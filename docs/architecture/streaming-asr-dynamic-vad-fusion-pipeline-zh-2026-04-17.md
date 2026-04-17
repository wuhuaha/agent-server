# `streaming ASR + dynamic VAD + punctuation + runtime endpoint fusion + 小模型语义裁判` 融合策略方案（2026-04-17）

## 文档定位

- 性质：面向当前项目的详细架构方案
- 目标：回答一个非常具体的问题——在当前 `agent-server` 里，`streaming ASR + dynamic VAD + punctuation + runtime endpoint fusion + 小模型语义裁判` 应该如何融合；典型决策流水线应该是什么；有哪些可选方案；推荐哪一种
- 适用对象：服务端语音 runtime、ASR/算法、端侧、后续实现与评测同学
- 相关文档：
  - `docs/architecture/streaming-asr-and-semantic-endpointing-research-zh-2026-04-17.md`
  - `docs/architecture/llm-assisted-semantic-completeness-and-dynamic-vad-zh-2026-04-17.md`
  - `docs/architecture/llm-semantic-turn-taking-and-interruption-zh-2026-04-17.md`
  - `docs/architecture/unified-early-processing-threshold-zh-2026-04-16.md`
  - `docs/architecture/voice-architecture-blueprint-zh-2026-04-16.md`

---

## 1. 一页结论

当前项目最适合采用的不是：

- 纯 `VAD-only` 阈值器
- 纯 `LLM-only` 语义端点器
- 单一 global score 黑箱
- 或把端点决策散到 gateway / provider / client 各层

而是一个**runtime-owned、分层分阶段、带动态等待时间控制的融合式端点控制器**。

一句话概括推荐项：

> **Acoustic floor + preview maturity gate + semantic wait-time controller + punctuation/clause evidence + slot/risk guard + runtime orchestrator**

换成当前项目里的语言，就是：

1. `dynamic VAD` 负责给出**底层停顿事实**与 base wait
2. `streaming ASR` 负责给出**稳定文本与修正轨迹**
3. `punctuation` 负责给出**clause closure / syntactic completeness**
4. `SemanticTurnJudge` 负责给出**utterance completeness / continuation / correction / backchannel / takeover**
5. `SemanticSlotParser` 负责给出**slot readiness / clarify_needed**
6. `internal/voice` 里的 endpoint controller 统一决定：
   - 继续等
   - endpoint candidate
   - prewarm / draft_allowed
   - accept_now
   - speaking-time interruption outcome

最推荐的方案不是“单步拍板”，而是：

- `candidate_ready`
- `draft_ready`
- `accept_ready`

三个层级推进。

---

## 2. 为什么必须这样设计

## 2.1 外部实践共同指向“分层融合”

### OpenAI

OpenAI 官方 `semantic_vad` 的核心思想不是“用语义替代 VAD”，而是：

- 由 semantic classifier 判断用户是否说完
- 动态控制等待时间
- 明确句子更早收尾，犹豫句更久等待

这说明 dynamic endpointing 的核心不是单阈值，而是**语义驱动的等待时间控制**。

### LiveKit

LiveKit 官方 turn detector 明确写到：

- 它是一个 open-weights 语言模型
- 作用是把 conversational context 作为 VAD 的附加信号
- 基于 `Qwen2.5-0.5B-Instruct`
- 单轮延迟约 `50~160 ms`
- 推荐仍同时搭配 VAD/STT endpointing

这说明小模型语义裁判的最佳位置是：

- **叠加在 VAD 之上**
- **做 end-of-turn 的 contextual boost**
- **而不是直接替代底层语音前端**

### Google

Google 的 `Unified End-to-End Speech Recognition and Endpointing for Fast and Efficient Speech Systems` 说明：

- ASR 与 endpointing 联合建模，可以显著降端点延迟
- 相比独立端点器，中位数端点延迟可降 `120 ms`，90 分位可降 `170 ms`
- 且 WER 不退化

这说明“端点应该吃到 ASR 表征”是正确方向。

### Semantic VAD

`Semantic VAD` 进一步证明：

- 把 punctuation / endpoint category / ASR-related semantic loss 引入 VAD
- 平均 latency 可降约 `53.3%`
- 且后端 ASR 不显著退化

这说明 punctuation 与 semantic completeness 进入端点控制有明确价值。

### Amazon

Amazon 的 `Adaptive endpointing with deep contextual multi-armed bandits` 说明：

- endpointing 不应只靠静态 grid search
- 更好的方向是根据 utterance-level context 动态选配置
- 可以降低 early cutoff error，同时维持低延迟

这说明“同一句话的最佳 endpoint 配置并不固定”，动态策略是值得做的。

### TurnGPT / response timing estimation

`TurnGPT` 和 Interspeech 2023 上关于 response timing estimation 的研究共同说明：

- syntactic completeness
- pragmatic completeness
- 低延迟 ASR

三者结合起来，对“什么时候该回”有直接增益。

### 本项目结论

所有这些外部实践汇总到当前项目，得到的是同一个答案：

> **不要把端点控制理解成单模型问题，而要把它理解成“多信号编排问题”。**

---

## 3. 当前项目里的角色划分

## 3.1 保持 runtime-owned ownership

本项目中，这条融合链必须继续收敛在 `internal/voice`，而不是下放到：

- `internal/gateway`
- provider adapter
- RTOS client

原因：

- gateway 只应做 transport adapter
- provider 只应提供 normalized evidence
- client 只保留 fallback / playback telemetry / 最小本地保护
- 真正的 turn-taking / endpoint / interruption policy 应继续由 server-side runtime 拥有

## 3.2 各层职责

### Layer 0：Acoustic Floor

典型信号：

- `speech_active`
- `speech_start_seen`
- `trailing_silence_ms`
- `audio_ms`
- `speech_ratio`
- VAD onset/offset
- provider endpoint hint

职责：

- 发现开始说话
- 给出 base endpoint candidate 倾向
- 作为 interruption 最快的入侵证据

### Layer 1：Preview Maturity Gate

典型信号：

- `partial_text`
- `stable_prefix`
- `revision_rate`
- `stable_for_ms`
- `no_update_ms`
- `correction_tail_chars`
- final/preview diff trend

职责：

- 决定什么时候这段 partial 已经“值得被语义分析”
- 过滤噪声与高抖动 partial

### Layer 2：Punctuation / Clause Layer

典型信号：

- online punctuation
- pseudo-online punctuation
- final punctuation hints
- clause count
- terminal punctuation

职责：

- 判断当前 stable prefix 是否形成 clause closure
- 区分“句末停顿”与“逗号停顿 / 列举停顿”

### Layer 3：Semantic Judge

典型输出：

- `utterance_status = complete | continue | correction | unclear`
- `interruption_intent = backchannel | duck_only | takeover | correction | ignore`
- `intent_family = question | command | answer | other`
- `dynamic_wait_policy = shorten | keep | extend`
- `wait_delta_ms`
- `confidence`

职责：

- 动态控制等待时间
- 为 prewarm / draft / interruption 提供语义证据

### Layer 4：Slot / Risk Guard

典型输出：

- `slot_readiness = unknown | partial | ready`
- `clarify_needed`
- `actionability`
- `missing_slots`
- `risk_level`

职责：

- 防止命令型 turn 在槽位未齐时过早 accept
- 为 clarify / act / hold 提供可执行语义

### Layer 5：Runtime Orchestrator

统一决定：

- `continue_waiting`
- `endpoint_candidate`
- `prewarm_allowed`
- `draft_allowed`
- `accept_candidate`
- `accept_now`
- `duck_only`
- `hard_interrupt`

---

## 4. 四种可选融合方案

## 4.1 方案 A：规则主导 + 语义微调

### 核心思路

- 以当前 `turn_detector` 规则为主
- 语义裁判只做 very light promotion / suppression
- punctuation 仅作 bonus feature

### 优点

- 改动小
- 风险低
- 最容易先上线

### 缺点

- 仍然容易受启发式上限限制
- `dynamic wait` 能力不够强
- pause / continue / correction 的自然度提升有限

### 适用阶段

- 适合最早期 MVP 或快速保守上线

---

## 4.2 方案 B：分阶段门控 + 动态等待时间控制器

### 核心思路

不做一个黑箱 global score，而是明确分成：

1. `candidate_ready`
2. `draft_ready`
3. `accept_ready`

并且让 semantic judge 主要输出 `dynamic_wait_policy + wait_delta_ms`，由 runtime 在 acoustic base wait 上做动态调节。

### 典型逻辑

- acoustic 层先给 `base_wait_ms`
- semantic judge 给 `wait_delta_ms`
- punctuation/clause 给 closure bonus 或 hold penalty
- slot/risk guard 决定是否允许 accept
- orchestrator 在不同 readiness stage 做推进或回退

### 优点

- 可解释性强
- 最适合逐步调参
- 与当前项目已有 `draft_allowed / accept_candidate / accept_now` 高度兼容
- 便于保留 rollback / late-tail merge

### 缺点

- 规则与状态较多，需要良好 tracing
- 初期配置面会增加

### 适用阶段

- **最适合当前项目**

---

## 4.3 方案 C：单一 evidence score + 少量硬阈值

### 核心思路

把多层信号压成一个分数，例如：

`endpoint_score = acoustic + stable_prefix + punctuation + semantic - correction_penalty - slot_penalty`

然后再加：

- `score >= T1` -> candidate
- `score >= T2` -> draft
- `score >= T3` -> accept

### 优点

- 调参形式统一
- 便于后续 learned scorer / bandit 化

### 缺点

- 容易过早变成黑箱
- 单分数难表达 slot/risk 这种 hard constraint
- 初期容易因为权重设置不稳而“看起来合理、实际不好调”

### 适用阶段

- 适合作为方案 B 的第二阶段，而不是当前第一选择

---

## 4.4 方案 D：统一 semantic endpointer / learned controller 主导

### 核心思路

- 强依赖统一小模型或 learned controller
- acoustic / punctuation / slot 只是辅助特征
- 最终由 learned policy 主导 accept

### 优点

- 长期潜力大
- 若数据和评测体系成熟，可能进一步优化自然度

### 缺点

- 当前项目训练数据与 online reward 体系不足
- 错误更难解释
- 不适合研究阶段立刻主线采用

### 适用阶段

- 适合 P3/P4 以后基于大量 trace 做 learned policy / bandit / policy tuning

---

## 5. 推荐项

### 5.1 当前强烈推荐：方案 B

即：

**分阶段门控 + 动态等待时间控制器**

这是当前项目最合适的折中点：

- 比纯规则更智能
- 比单分数黑箱更可控
- 比统一 learned controller 更现实
- 与当前架构、代码和研究结论最一致

### 5.2 推荐的升级路线

- `P1`: 先落地方案 B
- `P2`: 在 B 的 tracing 基础上，把部分 evidence 转成轻量 score 辅助，即 B + C
- `P3`: 如果数据积累足够，再尝试 bandit / learned wait-policy，即向 D 局部演进

---

## 6. 推荐项的具体融合策略

## 6.1 核心对象

建议在 runtime 内显式维护一个对象：

```json
{
  "acoustic": {
    "speech_active": true,
    "trailing_silence_ms": 220,
    "audio_ms": 1480,
    "base_wait_ms": 420,
    "endpoint_hint": "preview_tail_silence"
  },
  "preview": {
    "text": "明天周几",
    "stable_prefix": "明天周几",
    "revision_rate": 0.06,
    "stable_for_ms": 210,
    "no_update_ms": 180,
    "punctuation_hint": "question_like",
    "clause_closed": true
  },
  "semantic": {
    "utterance_status": "complete",
    "intent_family": "question",
    "dynamic_wait_policy": "shorten",
    "wait_delta_ms": -160,
    "confidence": 0.92
  },
  "slot": {
    "slot_readiness": "unknown",
    "clarify_needed": false,
    "risk_level": "low"
  },
  "decision": {
    "candidate_ready": true,
    "draft_ready": true,
    "accept_ready": true,
    "effective_wait_ms": 240,
    "accept_reason": "semantic_complete_question"
  }
}
```

---

## 6.2 决策公式

### 第一步：计算 `base_wait_ms`

由 acoustic + session pause statistics 决定：

```text
base_wait_ms = clamp(
  session_pause_ema_ms
  + acoustic_tail_adjustment
  + endpoint_hint_adjustment,
  min_wait_ms,
  max_wait_ms
)
```

建议初始默认：

- `min_wait_ms = 180`
- `max_wait_ms = 900`
- 短问句/短命令倾向于较低 base
- speaking-time interruption 可走更激进的 base

### 第二步：计算 `semantic_wait_delta_ms`

由 semantic judge 产生：

- `complete + command + slot_ready` -> `-120 ~ -260`
- `complete + question` -> `-80 ~ -180`
- `continue + hesitation` -> `+120 ~ +350`
- `correction` -> `+220 ~ +500`
- `unclear` -> `0 ~ +180`

### 第三步：叠加 punctuation / clause 修正

- clause closed / terminal punctuation / question-like closure -> `-40 ~ -120`
- comma / list / conjunction tail -> `+60 ~ +180`

### 第四步：叠加 slot / risk guard

- 命令型 `slot_readiness=partial` -> `+150 ~ +400`
- `clarify_needed=true` -> 禁止直接 accept
- `risk_level=high` -> 禁止 act-ready accept，必要时进入 clarify

### 最终

```text
effective_wait_ms = clamp(
  base_wait_ms
  + semantic_wait_delta_ms
  + punctuation_adjust_ms
  + slot_guard_adjust_ms,
  min_wait_ms,
  max_wait_ms
)
```

但注意：

- `effective_wait_ms` 只控制“等多久”
- **不是**唯一 accept 条件
- accept 仍要经过 readiness gate

---

## 6.3 三层 readiness gate

### Gate A：`candidate_ready`

必须满足：

- 已出现 `speech_start`
- 有至少一条 partial
- `stable_prefix` 达到最小长度或已有 endpoint hint
- `audio_ms` 超过最小门槛

建议默认：

- `min_audio_ms >= 280`
- `stable_prefix_chars >= 2`（中文短句）
- 或 endpoint hint 已出现

动作：

- 可产生 `endpoint_candidate`
- 可触发 semantic judge
- 可打点 candidate trace

### Gate B：`draft_ready`

在 `candidate_ready` 基础上，还应满足：

- preview maturity 足够：
  - `stable_for_ms`
  - `revision_rate` 低于阈值
  - `no_update_ms` 达到最小门槛
- semantic 不反对：
  - `utterance_status != continue`
  - `correction` 不强
- punctuation/clause 支持，或问答语义已足够闭合

动作：

- `prewarm_allowed=true`
- `draft_allowed=true`
- 允许 early planning / retrieval / tool prep

### Gate C：`accept_ready`

在 `draft_ready` 基础上，还应满足：

- `elapsed_since_last_significant_update >= effective_wait_ms`
- acoustic floor 未反对
- correction risk 低
- 若命令型请求，则：
  - `slot_readiness == ready`
  - 或至少不是 `clarify_needed`

动作：

- `accept_candidate=true`
- `accept_now=true`
- 进入正式 turn accept

---

## 7. 典型决策流水线

## 7.1 主链：用户说一句短问句

示例：`明天周几`

### Step 0：音频 ingress

- gateway 收到 20~100ms 音频 chunk
- 交给 `StreamingTranscriber`
- worker 给出 VAD/energy 事实与 partial

### Step 1：speech start

- acoustic floor 检出 onset
- runtime 发出 `speech_start`

### Step 2：preview 建立

- streaming ASR 出现 partial：`明天周`
- 再更新为 `明天周几`
- `stable_prefix=明天周几`
- `revision_rate` 降低

### Step 3：candidate gate

- `candidate_ready=true`
- 触发 semantic judge
- punctuation 层判断 question-like closure

### Step 4：semantic wait control

- judge 输出：
  - `utterance_status=complete`
  - `intent_family=question`
  - `wait_delta_ms=-140`

### Step 5：draft gate

- `draft_ready=true`
- 允许 prewarm / early planning

### Step 6：effective wait 满足

- acoustic base wait 例如 `380ms`
- semantic/punctuation 调整后 `effective_wait_ms=220ms`
- 实际停顿达到后进入 accept

### Step 7：正式 accept

- `accept_reason=semantic_complete_question`
- 进入 agent runtime

---

## 7.2 主链：用户说命令，但槽位未齐

示例：`帮我把客厅的...`

### 典型行为

- ASR 已有 partial
- acoustic 层已有停顿
- 但：
  - `stable_prefix` 尾部未闭合
  - punctuation 不闭合
  - semantic judge 可能给 `continue`
  - slot parser 判断 `slot_readiness=partial`

### 决策

- `candidate_ready=true`
- `draft_ready=false` 或仅 very weak draft
- `effective_wait_ms` 被 slot guard 拉长
- 暂不 accept

结果：

- 系统继续等，而不是把半截命令送进 runtime

---

## 7.3 主链：用户改口

示例：`打开客厅灯，不对，打开卧室灯`

### 典型信号

- preview 有明显 revision
- `correction_tail_chars` 增大
- semantic judge 输出 `correction`
- punctuation 不稳定

### 决策

- `wait_delta_ms` 明显增加
- `draft_allowed` 被压回保守状态
- 已有 speculative prewarm 可撤销
- 最终以修正后的 stable prefix 再进入 accept

---

## 7.4 speaking-time interruption

示例：assistant 正在说话，用户插话 `不是，我是说明天`

### Step 0：acoustic intrusion

- acoustic floor 快速发现入侵
- 先进入 reversible `duck_only`

### Step 1：preview maturity

- 若 partial 还不成熟，仅 duck，不 hard interrupt

### Step 2：semantic judge

- 若 judge 输出 `correction/takeover`
- 且 partial mature + clause 足够 + wait 很短

### Step 3：升级

- 从 `duck_only` 升级到 `hard_interrupt`
- playback truth 截断到 heard text
- 新用户 turn 接管

### 关键点

- acoustic onset 负责“先躲一下”
- semantic layer 负责“要不要真打断”

---

## 8. 推荐的默认阈值与触发策略

## 8.1 触发 semantic judge 的时机

不建议每个 partial 都调一次。

推荐触发条件：

- `candidate_ready` 第一次成立时
- 或 `stable_prefix` 发生实质变化时
- 或每 `150~250ms` 最多重评一次
- 或 speaking-time intrusion 时 partial 达到最小成熟度时

## 8.2 preview maturity 默认门槛

可从以下默认值起步：

- `audio_ms >= 280`
- `stable_for_ms >= 120`
- `no_update_ms >= 80`
- `revision_rate <= 0.18`
- `stable_prefix_chars >= 2`（中文）

## 8.3 wait 范围

- `min_wait_ms = 180`
- `max_wait_ms = 900`
- 短命令 / 问句：多落在 `180~350ms`
- correction / hesitation / slot missing：可拉到 `450~900ms`

这些值不应在 gateway 常量里硬编码，而应是 voice runtime 配置。

---

## 9. 推荐的实现映射

当前仓库内最适合承接该方案的位置：

- `internal/voice/turn_detector.go`
  - base wait
  - candidate/draft/accept gate
  - punctuation + slot + semantic fusion
- `internal/voice/semantic_judge.go`
  - `dynamic_wait_policy`
  - `wait_delta_ms`
  - `utterance_status`
- `internal/voice/semantic_slot_parser.go`
  - `slot_readiness`
  - `clarify_needed`
- `internal/voice/session_orchestrator.go`
  - speaking-time interruption flow
  - playback truth / rollback / late-tail
- `internal/voice/asr_responder.go`
  - preview maturity tracing
- `internal/gateway/*`
  - 只消费 `input.preview` / `input.endpoint` / accept 结果，不自行做 endpoint policy

---

## 10. 建议的观测指标

必须至少记录：

- `preview_first_partial_latency_ms`
- `candidate_ready_latency_ms`
- `draft_ready_latency_ms`
- `accept_ready_latency_ms`
- `effective_wait_ms`
- `base_wait_ms`
- `semantic_wait_delta_ms`
- `punctuation_adjust_ms`
- `slot_guard_adjust_ms`
- `accept_reason`
- `hold_reason`
- `rollback_reason`
- `late_tail_merge_rate`
- `post_accept_correction_rate`
- `duck_only_to_hard_interrupt_rate`

这样后续才能做：

- 阈值调优
- A/B
- 语义 judge 模型替换
- online bandit / policy learning

---

## 11. 最终推荐

### 推荐方案

**方案 B：分阶段门控 + 动态等待时间控制器**

### 推荐理由

1. 最符合当前项目现有架构与代码成熟度
2. 最符合 OpenAI / LiveKit / Google / Semantic VAD / Amazon 共同显示的方向
3. 最有利于在研究阶段保持：
   - 可解释
   - 可观测
   - 可回滚
   - 可逐步进化

### 一句话的最终设计

> 让 `dynamic VAD` 给出底层停顿事实，让 `streaming ASR` 给出稳定文本，让 `punctuation` 给出 clause closure，让 `SemanticTurnJudge` 动态调节等待时间，让 `slot parser` 决定命令是否已够执行，最后由 `internal/voice` 的 stage-based orchestrator 统一决定是继续等、先 draft、还是 accept。

---

## 参考链接

- OpenAI Realtime VAD / `semantic_vad`：<https://developers.openai.com/api/docs/guides/realtime-vad>
- LiveKit turn detector：<https://docs.livekit.io/agents/logic/turns/turn-detector/>
- Semantic VAD（Interspeech 2023）：<https://www.isca-archive.org/interspeech_2023/shi23c_interspeech.html>
- Google unified endpointing：<https://arxiv.org/abs/2211.00786>
- Amazon adaptive endpointing：<https://www.amazon.science/publications/adaptive-endpointing-with-deep-contextual-multi-armed-bandits>
- TurnGPT：<https://aclanthology.org/2020.findings-emnlp.268/>
- Response timing estimation with low-latency ASR：<https://www.isca-archive.org/interspeech_2023/sakuma23_interspeech.html>
