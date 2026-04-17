# 流式 ASR 与语义端点融合研究（2026-04-17）

## 文档定位

- 性质：针对当前项目的专题研究文档
- 目标：回答两个问题
  1. `Hugging Face`、`ModelScope`、GitHub 等开源生态里，是否已经有“流式识别 + 端点检测结合”的开源模型，可以在流式识别同时输出更快、更准、更符合语义的端点
  2. 如果没有成熟到可直接替换主链的现成方案，当前项目应如何结合“声学 + 流式识别 + 标点 + 规则 + 快速小模型/小 LLM”来做更好的端点识别
- 范围边界：
  - 以当前 `agent-server` 的服务侧语音主链为落点
  - 不讨论把项目直接改成端到端 speech-to-speech 大一统架构
  - 不把某个 seed domain 的业务词表硬编码回 shared runtime
- 相关上位文档：
  - `docs/architecture/project-status-and-voice-flow-review-zh-2026-04-17.md`
  - `docs/architecture/voice-architecture-blueprint-zh-2026-04-16.md`
  - `docs/architecture/streaming-final-asr-semantic-early-processing-zh-2026-04-16.md`
  - `docs/architecture/unified-early-processing-threshold-zh-2026-04-16.md`
  - `docs/architecture/voice-multi-llm-and-funasr-strategy-zh-2026-04-17.md`

---

## 1. 先把概念说清：本项目里“端点”至少有三层

当前讨论里的“端点检测”不能混成一个词，否则会误导架构判断。

### 1.1 声学端点 / 话音结束

这是最底层的“用户这一小段语音可能停下来了”：

- 典型依据是 `VAD`、能量下降、尾静音时长、blank / no-update stall
- 价值是：快、稳定、可做安全底座
- 缺点是：不懂语义，容易把“我想问一下那个……”这种未完句错判成结束

### 1.2 语句端点 / `end-of-utterance (EOU)`

这是更接近“这句话看起来说完了”的层次：

- 可以由联合建模的 ASR/EOU 模型给出
- 也可以由流式文本、标点、规则、语义模型融合得出
- 它比纯 VAD 更接近“语义结束”，但仍不等于“可以立刻提交执行”

### 1.3 Turn accept / 可真正提交到 Agent Runtime 的结束

这是当前项目最重要的一层：

- 对问答类请求，语句完整往往已经足够
- 对命令类请求，还需要看 `slot completeness`
- 对 speaking-time interruption，还要看这是不是 `backchannel / duck_only / hard takeover`

因此，哪怕未来接入真正的“ASR + EOU 一体模型”，它最多也只是：

- 更好的 `endpoint candidate` 信号
- 更好的 `early processing` 触发器

而不是天然等于：

- `turn accept`
- `tool-ready`
- `interrupt-now`

一句话：

> `EOU != final accept`。对当前项目，端点模型可以加速，但不能替代服务侧语音编排。

---

## 2. 结论先行

### 2.1 截至 2026-04-17 的总体判断

1. **开源生态并非完全没有“流式识别 + 端点检测”结合的模型。**
   - 我能明确确认到的一个代表，是 Hugging Face 上的 `nvidia/parakeet_realtime_eou_120m-v1`：官方 model card 明确写了它同时做流式语音识别与 `end-of-utterance prediction`，并通过特殊 token `<EOU>` 来输出结束点。  
   - 这说明“联合输出 ASR + EOU”已经开始从论文进入公开 checkpoint。

2. **但这类模型目前仍是少数，而且离“本项目可直接拿来替换主链”还有明显距离。**
   - 我目前没有找到一个成熟、中文优先、ModelScope/FunASR 主生态普遍采用、并以稳定对外 API 明确提供“语义端点判定”的开源成品。  
   - 这一点是根据本轮核查到的官方模型卡、FunASR 官方仓库、FunASR 官方讨论、以及 ModelScope 主生态现状做出的**推断**，不是数学意义上的“全网不存在”。

3. **中文开源主线，尤其是 FunASR / ModelScope 主生态，当前仍然明显是“模块化组合”路线。**
   - `streaming ASR`
   - `VAD`
   - `punctuation`
   - runtime 侧 endpoint / turn logic

4. **因此，对当前项目最好的方向不是等待单个“神奇端点模型”，而是继续把 endpointing 做成 runtime-owned layered fusion。**
   - 声学层给安全底座
   - 流式 ASR 给文本稳定性
   - 标点和 clause 给语句闭合线索
   - 规则层兜住 continuation / correction / slot-tail
   - 小模型/小 LLM 给语义加速与错误抑制

5. **如果未来引入一体化 `ASR + EOU` 模型，最合理的位置是“额外证据源”，而不是直接取代当前 `internal/voice` 的 turn orchestration。**

---

## 3. 当前开源现状：有哪些能力，哪些还缺

## 3.1 Hugging Face：已经出现少量“一体化 ASR + EOU”模型，但仍不普遍

### 明确存在的代表：`nvidia/parakeet_realtime_eou_120m-v1`

从该模型的官方 model card 可以确认：

- 它是 `streaming ASR model`
- 同时做 `end-of-utterance prediction`
- 通过输出一个特殊 `<EOU>` token 来标记结束
- 语言当前标注为 `English`

这说明：

- “ASR 解码过程中顺带输出 EOU”这件事已经不只是论文概念
- 这类模型对于 `endpoint candidate` 和更早的服务端预处理很有价值

但对本项目而言，它的局限也很明显：

- 当前官方描述是英语模型，不是中文主线
- 它解决的是 `EOU`，不是命令可执行性、`slot completeness`、也不是 speaking-time interruption policy
- 其生态成熟度与当前项目已经打通的 FunASR/中文路径相比，还不够成为默认替代

### 对 Hugging Face 生态的综合判断

当前 HF 上能见到很多 streaming ASR 模型，但“同时把端点语义建成正式输出头”的模型并不多。一个对照例子是 `speechbrain/asr-streaming-conformer-gigaspeech`：它明确是 streaming ASR，但 model card 并没有把“联合端点输出”作为主要公开能力。

这说明：

- HF 上“streaming ASR checkpoint 很多”
- 但“streaming ASR + 可直接用于服务侧 turn accept 的 EOU/semantic endpoint checkpoint”依然稀缺

---

## 3.2 ModelScope / FunASR：当前主流仍是模块化栈，而不是一体化 semantic endpoint model

对当前项目最相关的仍然是 FunASR 体系，因为它与本仓库现有本地 GPU 路径最贴合。

从 FunASR 官方 Hugging Face 页面与官方 GitHub 仓库可确认：

- `funasr/paraformer-zh-streaming` 的页面同时列出了：
  - `Speech Recognition Paraformer-large`
  - `Voice Activity Detection FSMN-VAD`
  - `Punctuation Restoration CT-Punc`
- FunASR 官方仓库明确支持 `2pass` / `2pass-offline`
- FunASR 官方讨论里也明确提到过 `online punctuation model`，其做法是在 VAD 判断“语音缺席”的时刻输出伪流式标点

这几个点很关键：

1. **FunASR 确实高度重视实时性。**
   - 有 streaming ASR
   - 有 2pass
   - 有 VAD
   - 有在线/伪在线标点

2. **但它的主流工程形态依然是“多个模块协同”，而不是一个统一 semantic endpoint head。**

3. **这和本项目当前的 runtime 方向其实是高度一致的。**
   - 当前项目已经有：
     - `preview ASR`
     - `server endpoint`
     - `stable_prefix`
     - `SemanticTurnJudge`
     - `SemanticSlotParser`
   - 差的不是“有没有一个神奇 checkpoint”，而是这些已有信号融合得是否足够快、足够稳、足够可解释

### 本轮对 ModelScope 生态的推断结论

截至 2026-04-17，本轮基于官方仓库、官方模型卡、公开模型命名与官方讨论的核查，**我没有找到一个被广泛采用、明确宣称“中文流式 ASR + 语义端点检测统一输出”的 ModelScope/FunASR 主流模型卡**。

这是一条**推断**：

- 不是说未来不会出现
- 也不是说社区里绝对没有个人实验模型
- 而是说：**当前最成熟、最可依赖、最适合接到本项目主线的，仍是模块化组合**

---

## 3.3 另一条开源主线：runtime 级 endpointing，而不是模型直接输出 endpoint

`sherpa-onnx` 的官方 endpointing 文档代表了另一类很典型的开源实践：

- 它的 endpointing 是 runtime 侧规则
- 规则会综合“是否包含非静音音频”“尾静音时长”“当前已解码文本长度”等条件
- 也就是 Kaldi 风格的 rule-based endpointing 现代化延续

这条路线的特点是：

- 工程上成熟
- 可解释性强
- 便于边缘部署
- 但不天然具备“语义结束”理解能力

对当前项目的启发是：

- 即使不接入小 LLM，靠 `VAD + ASR stability + endpoint rules` 也可以先把一版很强的端点器做出来
- 小模型/小 LLM 更适合放在上层做“加速 accept / suppress false accept”，而不是替代所有低层规则

---

## 3.4 论文与厂商实践：联合建模是方向，但生产可用形态仍多为分层融合

### Google：联合建模 ASR 与 endpointing 是有效方向

Google 的 `Unified End-to-End Speech Recognition and Endpointing for Fast and Efficient Speech Systems` 明确说明：

- 端点检测与 ASR 共享信息、联合优化，可以同时降低延迟并保持识别质量

这说明：

- 从研究方向看，“单模型更懂什么时候说完了”是成立的
- 所以像 `Parakeet Realtime EOU` 这样的模型是值得关注的

### 两阶段 / 两遍式分段研究仍在继续

`E2E Segmentation in a Two-Pass Cascaded Encoder ASR Model` 这类研究进一步表明：

- 即使是两阶段或两遍式系统，也可以把“分段 / 端点”纳入 encoder 或 decoding 过程里联合优化
- 这非常契合当前项目的现实：我们并不需要一步跳到 speech-to-speech，仍可以在 cascade 主线里持续吸收联合分段能力

### OpenAI / LiveKit：系统级 turn detection 正在从 `VAD-only` 升级到语义增强

虽然它们不等于“开源 checkpoint”，但它们提供了很强的系统设计参考：

- OpenAI Realtime 官方文档已经把 turn detection 区分为 `server_vad` 与 `semantic_vad`
- LiveKit 的 turn detector 文档明确给出一个 open-weights、基于 `Qwen2.5-0.5B` 的 turn detector，并强调其目的是判断“是否应该等待更多用户语音”

这说明当前业界主流并不是：

- 只调 VAD 阈值
- 或者只等一个最终句号

而是：

- 低层仍然靠声学/VAD 保底
- 上层通过文本/语义模型来判断“该不该等一下、是不是话还没说完”

这和当前项目的 `server endpoint + preview + semantic judge + slot completeness` 路线高度一致。

---

## 4. 直接回答问题 1：有没有“流式识别 + 端点检测结合”的开源模型？

### 4.1 回答

**有，但很少，而且对当前项目最关键的中文主线来说，仍不足以直接替代现有架构。**

更细一点说：

- **有少量新出现的一体化例子**：例如 `nvidia/parakeet_realtime_eou_120m-v1`
- **但主流中文开源路径仍不是这个形态**：当前更成熟的是 FunASR 这类模块化路线
- **我没有找到成熟、中文优先、在 ModelScope/FunASR 主生态中被广泛采用的一体化 semantic endpoint model**

### 4.2 对本项目意味着什么

1. 不应为了追“一体化模型”而打断当前主线
2. 可以把一体化 `ASR + EOU` 视为后续 `A/B` 能力源
3. 当前最应该做的是把 runtime 里的多信号融合做厚

---

## 5. 如果没有现成银弹：当前项目最值得采用的端点融合方案

## 5.1 总体原则

推荐把 endpointing 做成一个**五层融合控制器**，并继续放在 `internal/voice` 内部，而不是让 gateway 或 provider adapter 接管。

### 核心原则

1. **Acoustic hard floor，semantic soft accelerator**
   - 声学层是安全底座
   - 语义层负责加速或抑制
   - 不让小 LLM 单独制造最终 accept

2. **先 candidate，再 prewarm，再 accept**
   - `endpoint candidate`：可能说完了
   - `draft/prewarm`：可以提前准备，但必须可撤销
   - `accept_now`：真正把这一轮提交给 agent runtime

3. **EOU 信号最多是强证据，不是总闸门**
   - 即使接入联合 `ASR + EOU` 模型，也只把它当额外 evidence

4. **slot completeness 是 task-aware 的**
   - 问答类 turn 不应被 slot completeness 卡死
   - 命令类 / tool 类 turn 则应显著受其约束

---

## 5.2 五层证据栈

### Layer 0：声学层（安全底座）

输入信号：

- `speech_active`
- `trailing_silence_ms`
- `speech_duration_ms`
- `last_voiced_ms`
- 能量下降 / 过零率 / blank-stall
- worker 侧 `fsmn-vad` / `silero-vad` / energy hint

职责：

- 尽快给出 `speech start`
- 给出最初的 `endpoint candidate` 倾向
- 为 interruption 提供最快的 onset 证据

要求：

- 永远保留
- 不依赖 LLM
- 可以在 provider 缺失或语义层超时时独立工作

### Layer 1：流式识别稳定性层

输入信号：

- `stable_prefix`
- 最近 `N` 次 partial 的 revision rate
- 文本无更新时长 `no_update_ms`
- 尾部改写长度 `correction_tail_chars`
- 是否出现 provider 原生 `EOU` / endpoint hint

职责：

- 区分“文本已经稳定”还是“尾巴还在抖”
- 判断是不是进入了可做语义判断的成熟 preview candidate

这里最重要的一点是：

> 不是所有 partial 都值得送去做语义判断；应优先让 `stable_prefix` 进入后续仲裁。

### Layer 2：标点 / clause 闭合层

输入信号：

- 流式/伪流式标点结果
- final punctuation
- 语气词与问句尾标记
- clause 数量与边界强弱

职责：

- 判断当前 stable prefix 是否已经形成较完整的 clause
- 帮助区分“句子结束”与“只是逗号停顿”

建议：

- 若 worker 侧可稳定提供 online punctuation，可将其作为 preview evidence
- 若当前 online punctuation 质量不稳定，则至少在 stable prefix stall 或短静音后触发一次轻量 punctuation 推断，而不是每帧都跑

### Layer 3：规则层（便宜、强约束、可解释）

规则层不是落后的做法，而是语义系统的保险丝。

应重点覆盖：

- continuation tail：`然后`、`再`、`还有`、`那个`、`就是`、`所以` 等
- correction intent：`不是`、`不对`、`改成`、`我是说`
- slot-tail protection：数值、时间、房间、目标对象、量词、单位是否半截悬空
- short backchannel：`嗯`、`哦`、`对`、`行`、`好吧`

职责：

- 在语义层之前，先快速压掉明显不该 accept 的情况
- 在语义层之后，作为 final safety check

### Layer 4：快速小模型 / 小 LLM 语义裁判层

这层的目标不是生成回复，而是给出一个结构化、短路径、低延迟的判断：

- `utterance_status`: `complete / continue / correction / unclear`
- `interruption_intent`: `ignore / backchannel / duck_only / takeover`
- `slot_status`: `unknown / partial / complete`
- `clarify_needed`: `true / false`
- `confidence`
- `reason`

#### 为什么这里适合小模型而不是主回复大模型

- 高频调用，必须低抖动
- 输出结构化短 JSON，天然更适合小模型
- 与主回复模型解耦后，实时链和长回复链不会争 GPU 资源

#### 什么模型形态更合适

优先级建议：

1. **专用 turn detector / 轻量语义分类器**
   - 例如 LiveKit 当前公开的 open-weights turn detector 就是基于 `Qwen2.5-0.5B`
   - 这类模型比通用聊天 LLM 更贴合“等不等 / 该不该打断”的任务

2. **严格结构化 prompt 的小 LLM**
   - 当前项目的本地首选仍可从 `Qwen3-1.7B` 级别开始
   - 输入只给 `stable_prefix + 最近不稳定尾巴 + 少量最近 partial 差异 + 轻量元数据`
   - 输出固定 schema，禁止长解释

3. **大模型只做主理解/回复，不参与高频端点仲裁**

---

## 5.3 一套更适合当前项目的“统一早处理门槛”

本项目前面已经研究过：

- `stable prefix`
- `utterance completeness`
- `slot completeness`

这三者完全可以收敛成一个统一但分层的门槛体系。

### 阶段 A：`candidate_ready`

满足以下条件中的大部分即可：

- 有最小尾静音或 provider EOU hint
- `stable_prefix` 达到最小长度
- 最近一次 partial 更新后已有短暂 stall
- correction risk 不高

动作：

- 允许打 `endpoint candidate`
- 允许触发轻量语义判断
- 允许预热 planner / memory / tools

### 阶段 B：`draft_ready`

在 `candidate_ready` 基础上，再满足：

- 语句看起来基本闭合
- 或小模型判断 `complete`
- 或问答类场景下虽无句号但语义已足够明确

动作：

- 允许启动可撤销的 early processing
- 可提前准备首句回复或工具候选
- 仍保留 rollback window

### 阶段 C：`accept_ready`

在 `draft_ready` 基础上，再满足：

- 声学底座确认不是误切
- continuation risk / correction risk 低
- 若是命令型请求，则 `slot completeness` 至少基本可用

动作：

- 真正 `turn accept`
- 输出 `accept_reason`
- 进入主 agent runtime

### 关键点

- `candidate` 和 `accept` 必须分开
- `semantic complete` 与 `slot complete` 必须分开
- `EOU` 和 `tool-ready` 必须分开

这也是当前项目避免“抢答”和“等太久”同时失控的关键。

---

## 5.4 推荐的融合逻辑：先离散门槛，后学习权重

在当前研究阶段，不建议一上来就做一个复杂、难调的 learned scorer。更实用的是：

### 第一步：离散门槛 + 明确 reason code

例如：

- `endpoint_candidate_reason=acoustic_eou`
- `endpoint_candidate_reason=stable_prefix_plus_punc`
- `hold_reason=continuation_tail`
- `hold_reason=slot_missing`
- `hold_reason=correction_in_progress`
- `accept_reason=semantic_complete_with_slot_ready`

这样最利于：

- trace
- replay
- A/B
- 端侧联调

### 第二步：再把这些离散特征学成一个轻量打分器

等日志积累后，再考虑把以下特征喂给更轻量的 learned ranker：

- acoustic score
- stable prefix score
- punctuation closure score
- semantic completeness score
- slot completeness score
- correction penalty
- continuation penalty

但这应是后续优化，不是当前研究阶段的第一优先级。

---

## 6. 推荐给当前项目的落地组合

## 6.1 近期主线组合

对于当前中文、本地 GPU、FunASR 已接通的项目现状，推荐仍以这组为主：

- preview ASR：`paraformer-zh-streaming`
- preview / final VAD：`fsmn-vad`
- final ASR：当前已接通的中文高质量 final-ASR 路径
- punctuation：优先接入 FunASR online/pseudo-online punctuation；若当前不稳定，则至少保留 `ct-punc` 于 stable-prefix stall / final path
- semantic judge：小模型/小 LLM，优先 `Qwen3-1.7B` 级别的结构化裁判
- slot parser：继续保留当前 `SemanticSlotParser` 路线

### 原因

1. 这条路线与当前项目已完成的 runtime 架构最一致
2. 对中文语音与本地 GPU 更现实
3. 可以持续吸收未来的一体化 `ASR + EOU` 模型，而不需要推翻现有边界

---

## 6.2 一体化 `ASR + EOU` 模型在本项目里的正确位置

如果后续要引入类似 `Parakeet Realtime EOU` 的能力，推荐做法是：

- 在 `StreamingTranscriber` 的归一化结果里增加可选的 provider hint
  - 例如 `provider_eou_seen`
  - 或 `provider_eou_score`
- 该 hint 只进入 `internal/voice` 的 endpoint fusion
- 不直接暴露成 gateway 的独立业务语义
- 不让它直接单独触发最终 `turn accept`

这样做的好处：

- 未来若出现中文、多语、效果更好的 EOU 模型，可以无缝接入
- 当前项目不会被某个 provider 的特定 token 机制绑死
- 仍然保留 `slot completeness`、`interruption policy`、`playback truth` 这些服务侧核心能力

---

## 6.3 不建议当前阶段做的事

### 不建议 1：为了追单模型而替换整个中文主链

原因：

- 现有中文开源成熟度仍是 FunASR 组合栈更高
- 当前项目真正的瓶颈更多在 orchestration，而不是单一 checkpoint

### 不建议 2：让小 LLM 成为最终 accept 的唯一裁判

原因：

- 时延抖动不可控
- 容易误判口语修正与半截命令
- 会削弱服务侧语音 runtime 的可解释性

### 不建议 3：把端点规则写成 seed-domain 业务逻辑

原因：

- 当前项目目标是通用 agent server
- 智能家居只是一个应用方向，不应反向腐化 shared voice runtime

---

## 7. 验证指标：怎么判断这条路真的更好

推荐至少持续观测以下指标：

### 时延相关

- `preview_first_partial_latency_ms`
- `endpoint_candidate_latency_ms`
- `accept_latency_ms`
- `response_start_latency_ms`
- `first_audio_latency_ms`

### 质量相关

- `false_endpoint_rate`
- `late_tail_merge_rate`
- `post_accept_correction_rate`
- `preview_to_final_delta_chars`
- `slot_missing_after_accept_rate`

### 交互感知相关

- 用户主观“抢答”率
- 用户主观“等太久”率
- speaking-time interruption 误触发率
- `duck_only -> hard_interrupt` 升级精度

### 诊断相关

- `accept_reason` 分布
- `hold_reason` 分布
- semantic judge 超时率
- punctuation hint 可用率
- provider EOU hint 采纳率

---

## 8. 对当前项目的最终建议

### 结论 1

不要把希望押在“是否能找到一个单模型同时搞定 streaming + semantic endpointing”。

### 结论 2

当前最现实、最先进、也最符合项目现状的路径，是：

- 保持当前 cascade 主线
- 继续强化 `internal/voice` 里的 layered endpoint fusion
- 把一体化 `ASR + EOU` 模型当成未来可插拔证据源

### 结论 3

短期最值得优先增强的是：

1. 更稳定的 `stable_prefix` 与 revision-rate 度量
2. 更早但更稳的 punctuation / clause 闭合信号
3. 小模型 semantic judge 的高频、短 schema、低抖动推理
4. `endpoint candidate -> draft_ready -> accept_ready` 的显式分层
5. 命令型 turn 的 `slot completeness` 保护

### 结论 4

如果只用一句话概括当前项目下一阶段的端点策略：

> 用声学层保底，用流式识别与标点做结构，用小模型补语义，把真正的 accept 继续掌握在服务侧 runtime 手里。

---

## 参考链接

- NVIDIA Parakeet Realtime EOU 120M（Hugging Face）：<https://huggingface.co/nvidia/parakeet_realtime_eou_120m-v1>
- SpeechBrain Streaming ASR（Hugging Face）：<https://huggingface.co/speechbrain/asr-streaming-conformer-gigaspeech>
- FunASR `paraformer-zh-streaming`（Hugging Face）：<https://huggingface.co/funasr/paraformer-zh-streaming>
- FunASR 官方仓库（GitHub）：<https://github.com/modelscope/FunASR>
- FunASR 官方讨论：online punctuation（GitHub Discussions）：<https://github.com/modelscope/FunASR/discussions/238>
- sherpa endpointing 官方文档：<https://k2-fsa.github.io/sherpa/python/streaming_asr/endpointing.html>
- OpenAI Realtime VAD / `semantic_vad`：<https://platform.openai.com/docs/guides/realtime-vad>
- LiveKit turn detector：<https://docs.livekit.io/agents/logic/turns/turn-detector/>
- Google：`Unified End-to-End Speech Recognition and Endpointing for Fast and Efficient Speech Systems`：<https://arxiv.org/abs/2211.00786>
- `E2E Segmentation in a Two-Pass Cascaded Encoder ASR Model`：<https://arxiv.org/abs/2211.15432>
- Amazon：`Accurate Endpointing with Expected Pause Duration`：<https://www.amazon.science/publications/accurate-endpointing-with-expected-pause-duration>
