# 语音架构完整方案（2026-04-16）

## 文档状态

- 性质：架构方案 / 主线设计蓝图
- 状态：当前项目语音方向的正式参考基线
- 适用阶段：研究阶段到可落地演进阶段
- 目标：在不破坏当前 `agent-server` 核心边界的前提下，形成一套尽可能先进、自然、智能、可落地、可持续演进的语音架构

## 一句话结论

当前项目最合适的语音方向，不是“继续停留在 client commit 半双工 + 批式回复”的渐进修补，也不是“立刻切到一套全新重型框架”，而是：

**以 `Realtime Session Core` 为中心，采用 `server-primary hybrid` 语音架构：会话建立后由服务侧主导 turn-taking、early processing、interruption arbitration、incremental output 与 playback truth reconciliation；端侧保留音频前端、播放控制、快速反射与兜底能力。**

在这个架构里，`internal/voice` 不再只是一个“ASR/TTS 提供者集合”，而是项目的 **Voice Orchestration Runtime**，围绕四条主循环协同工作：

1. `Input Preview Loop`：在线预览识别、声学/语义端点候选、输入里程碑感知
2. `Early Processing Loop`：统一早处理门槛（UEPG）、draft/prewarm/speculation、低风险前推
3. `Output Orchestration Loop`：增量文本、意群规划、早起播、duck/backchannel/interruption 输出仲裁
4. `Playback Truth Loop`：播放事实回传、heard-text 估计、memory 写回、resume/continue 对齐

## 目录

1. 背景与现状
2. 设计目标与非目标
3. 核心设计原则
4. 总体架构
5. 服务侧与端侧职责边界
6. 输入链路架构
7. turn-taking 与统一早处理门槛
8. 输出链路架构
9. interruption / 全双工仲裁
10. playback truth chain 与 heard-text
11. 领域智能与语音理解增强
12. 观测、评测与质量闭环
13. 运行时与部署架构
14. 渐进式落地路线
15. 与当前仓库的模块映射
16. 成功标准
17. 参考资料

## 1. 背景与现状

### 1.1 当前项目已经具备的关键基础

结合仓库当前实现与近几轮研究，项目已经具备如下重要基础：

- `Realtime Session Core` 已存在，并已引入内部 `input_state / output_state` 双轨表达
- `server_endpoint` 已经从隐藏实验演进为 discovery-advertised main-path candidate
- `internal/voice.SessionOrchestrator` 已开始拥有 preview、playout、heard-text persistence 的共享边界
- speaking 期间的 `ignore / backchannel / duck_only / hard_interrupt` 已具备初步共享运行时路径
- 输出侧已具备早起播雏形：speech planner + early audio hook
- 本地开源优先的 GPU 语音栈已成形：FunASR worker、CosyVoice GPU TTS、本地 LLM worker 候选
- 观测侧已具备预览和 turn milestone tracing：preview first partial、endpoint candidate、first text delta、first audio chunk 等

### 1.2 当前仍未彻底解决的关键问题

尽管已有基础，但项目还存在几个决定体验上限的结构性问题：

- `server endpoint` 已能工作，但还未成为完整会话编排中心
- speaking-time preview、interruption、resume 与 output orchestration 还未完全收束成一套稳定行为模型
- `preview partial` 仍未完全成为用户可感知的主路径信号
- `heard-text` 虽已进入共享 runtime 边界，但底层 playback facts 仍偏启发式
- 现有语音路径在“快、自然、智能、像人”之间的平衡仍偏 demo 级，而非 architecture-grade

### 1.3 当前架构演进的正确方向

过去几轮研究已经收敛出一组很稳定的结论：

- 会话建立后，turn-taking 应由服务侧主导，但端侧不能被降级为纯采音器
- 最值得投入的中间层不是更激进的硬中断，而是 `duck_only`
- 实时语音体验必须由 milestone latency 驱动，而不是单一总时延驱动
- `preview partial + utterance completeness + slot completeness` 可以形成统一早处理对象，但不应被压成单一阈值
- `heard-text` 必须来源于 playback truth chain，而不是直接拿完整生成文本当作“用户已听到”

## 2. 设计目标与非目标

### 2.1 设计目标

本方案目标是让项目的语音能力在下列维度上长期收敛：

#### A. 流畅性

- 用户尽早感知“系统在听”
- 用户尽早感知“系统开始懂了”
- 系统尽早起播首句，减少空等
- 插话时系统快速让话，不再死等完整状态切换

#### B. 自然性

- endpointing 不只靠静音阈值
- interruption 不再只有硬 cancel 一种手感
- TTS 能按意群更早起播，并保留更自然的停顿与回应节奏

#### C. 智能性

- 语义完备时尽早处理，但不可逆动作保守提交
- 面向智能家居 / 桌面助理场景，支持 alias、entity catalog、dynamic bias、slot completeness
- 低置信/歧义时优雅澄清，而不是机械误执行

#### D. 可落地

- 不引入第二套实时协议家族
- 尽量复用当前 `internal/session`、`internal/voice`、`internal/agent`、`internal/gateway` 边界
- 本地 / 开源优先，兼容当前 GPU 运行现实

#### E. 可持续演进

- 允许当前 v0 设备协议继续存在
- 保持 discovery / additive capability / compatibility-first 的演进节奏
- 支持未来接入更强 streaming ASR、turn detector、incremental TTS、realtime speech model，而不重写上层结构

### 2.2 非目标

当前方案明确不追求：

- 一步切成“纯端到端语音大模型 + 单体黑盒”
- 为了研究完整性而引入过重的控制平面或多服务编排层
- 把 channel adapter、web adapter、device adapter 各自做成第二套语音编排器
- 在还没有 playback truth 与 eval 闭环之前，过早追求复杂多代理语音 workflow

## 3. 核心设计原则

### 3.1 Session Core 仍是中心

- `Realtime Session Core` 继续是全系统中心
- 语音只是在其上形成一套共享 runtime capability
- 所有 turn、interruption、response lifecycle 最终都要回到 session core 统一表达

### 3.2 Voice Runtime 是“编排层”，不是“模型适配层”

`internal/voice` 的长期定位应是：

- preview / endpoint / early processing / output / playback truth 的共享编排层
- 而不是只负责：
  - 调某个 ASR
  - 调某个 TTS
  - 转一下 provider payload

### 3.3 设备与渠道都是 adapter

- device adapter 只负责 ingress/egress 与本地执行能力
- channel adapter 只负责 transport 和 message adaptation
- 它们都不应拥有独立的会话策略、模型调用逻辑或 interruption policy

### 3.4 公开协议保持加法演进

- 当前公开 `turn_mode=client_wakeup_client_commit` 兼容基线继续保留
- `server_endpoint`、更丰富的 lane state、future playback facts 应通过 additive capability 逐步公开
- 不在研究期引入第二套完全不同的 wire protocol

### 3.5 先做“像真人的节奏”，再做更大能力面

对 phase-1 / phase-2 的 ROI 来说，最优先的不是：

- 更复杂多模态工具链
- 更大模型单点升级

而是：

- preview 更早
- endpoint 更准
- interruption 更自然
- TTS 更早起播
- heard-text 更可信

## 4. 总体架构

## 4.1 目标架构概览

```text
[Device / Browser / RTOS]
  |- wake / local AEC-NS-AGC / local reflex VAD / playback control / playback telemetry
  v
[Gateway Adapter Layer]
  |- realtime ws / xiaozhi ws / browser page adapter
  |- transport normalization only
  v
[Realtime Session Core]
  |- session identity
  |- input lane / output lane
  |- compatibility state view
  |- turn ids / trace ids / response lifecycle
  v
[Voice Runtime]
  |- Input Preview Loop
  |   |- streaming ingest
  |   |- optional KWS
  |   |- online preview ASR
  |   |- endpoint evidence
  |
  |- Early Processing Loop
  |   |- UEPG
  |   |- draft-ready / commit-ready gates
  |   |- prewarm / speculative planning
  |
  |- Output Orchestration Loop
  |   |- text stream / speech planner
  |   |- incremental TTS
  |   |- duck/backchannel/hard interrupt policies
  |
  |- Playback Truth Loop
      |- playback facts
      |- delivered/heard/truncated state
      |- memory writeback / resume anchors
  v
[Agent Runtime Core]
  |- streaming llm/tool loop
  |- memory / skill / tool invocation
  |- risk-based action policy
```

## 4.2 架构中的四个关键“闭环”

### A. Input Preview Loop

职责：

- 尽早感知用户开始说话
- 产出 preview partial
- 构造 endpoint evidence
- 为 turn-taking、interruption、UI cue、LLM/TTS 预热提供前置输入

### B. Early Processing Loop

职责：

- 在 final 前做可逆前推
- 基于 `UEPG` 决定：
  - 是否显示 preview
  - 是否开始 draft
  - 是否开始 prewarm
  - 是否允许低风险回答提前起播

### C. Output Orchestration Loop

职责：

- 让输出不再等 `TurnResponse` 完全收口再开始
- 管理 text delta、speech planner、TTS、duck/backchannel/interruption
- 保证 `response.start`、text delta、audio stream 真正重叠

### D. Playback Truth Loop

职责：

- 聚合 playback_started / mark / clear / complete 等事实
- 推导 delivered/heard/truncated
- 维护 heard-text、resume anchor、memory truth

## 5. 服务侧与端侧职责边界

## 5.1 目标边界：server-primary hybrid

### 服务侧负责

- turn-taking 主裁决
- endpoint candidate / turn accept
- preview / stable prefix / completeness 评估
- interruption arbitration
- response.start 决策
- incremental output orchestration
- playback truth interpretation
- heard-text persistence

### 端侧保留

- wake word 触发 / 会话唤起
- AEC / NS / AGC / beamforming / render reference 等音频前端能力
- local speech hint / reflex VAD
- 本地 playback stop / clear / duck / unduck
- playback telemetry 回传
- 网络异常/弱网 fallback
- 手动 stop / force commit / push-to-talk 等 override

## 5.2 为什么不做“纯服务侧 everything”

因为端侧仍然最接近：

- 真实扬声器参考信号
- 麦克风原始声学环境
- 本地播放缓冲与急停能力
- 用户已明显开口时的最快反射点

所以更合理的是：

- **服务侧拥有判断权**
- **端侧保留反射权、事实回传权、兜底权**

## 5.3 为什么不继续长期停留在 client commit 主路径

因为长期只靠显式 commit 会导致：

- endpoint 不自然
- preview 无法成为主链输入
- interruption 只能靠显式重开下一轮
- 真实全双工体验很难建立

因此当前兼容基线继续保留，但主架构目标应明确指向：

- `server endpoint + preview-driven orchestration`

## 6. 输入链路架构

## 6.1 输入链路的目标形态

```text
device audio
  -> local frontend (AEC/NS/AGC/...)
  -> gateway ingest
  -> StreamingTranscriber session
  -> optional KWS
  -> online preview ASR
  -> preview endpoint hints
  -> final ASR correction
  -> normalized speech metadata
  -> turn candidate / agent input
```

## 6.2 模块分层

### A. Audio Ingest Layer

职责：

- 接收 `pcm16le` 或受支持 `opus`
- 做轻量缓存、preroll、frame ownership 管理
- 为 preview / final / playback correlation 保留 trace 信息

### B. Acoustic Front-End Contract

说明：

- 端侧前处理仍是主要来源
- 服务侧不复写端侧 AEC 等职责
- 服务侧主要消费端侧输出与少量 speech hints

### C. StreamingTranscriber Contract

职责：

- 对上提供统一 streaming ASR session
- 对下屏蔽 `funasr_http`、兼容 buffer provider、未来 cloud provider 差异

### D. Worker-Internal Speech Pipeline

当前最适合项目的 worker 内部路径是：

- `optional KWS`（默认关闭）
- `online preview ASR`
- `preview endpoint hints`
- `final ASR correction`

对当前项目而言，`KWS` 的地位应是：

- 作为可配置可插拔能力
- 默认不接入主路径
- 仅在需要“会话内二次唤醒 / 局部热启动 / 特定产品形态”时启用

## 6.3 输入链路的推荐逻辑角色划分

### `optional KWS`

- 用于会话前唤醒或会话内特定模式的强触发
- 不能替代 endpointing
- 不能替代 speaking-time interruption arbitration

### `online preview ASR`

- 负责尽早产出 partial / stable prefix
- 是 preview、UEPG、duck_only、prewarm 的主要输入
- 应优先追求：
  - 低首包延迟
  - 较稳定前缀
  - 持续更新能力

### `final ASR`

- 负责 authoritative correction
- 负责更强的 normalization / punctuation / domain correction
- 是高风险 action 与持久事实写入的更高置信来源

### `preview endpoint hints`

- 来自 worker 的尾静音、VAD 或更强 acoustic clue
- 只作为 shared runtime 的输入之一
- 不能让 gateway 学会 provider-specific endpoint 逻辑

## 6.4 输入链路中的结构化语音元数据

`internal/voice` 应持续把 provider-specific 结果归一化为 shared speech metadata，例如：

- language
- speaker
- endpoint_reason
- partial hypothesis
- event: speech_start / speech_stop / preview_update
- confidence / stability-like hints

这些元数据的意义不仅是日志展示，更是：

- turn-taking
- interruption arbitration
- domain correction
- memory 与 evaluation

## 7. turn-taking 与统一早处理门槛

## 7.1 turn-taking 的核心判断：不再只靠 silence window

当前目标架构下，turn accept 应来自多信号融合，而不是单一静音阈值：

- acoustic evidence
- lexical completeness
- semantic completeness
- slot completeness
- dialogue context
- provider endpoint hints
- current output state

## 7.2 统一早处理门槛：UEPG

### 结论

`stable prefix + utterance completeness + slot completeness` 可以形成统一对象，但不应压成单一分数。

更合理的对象是：

```text
UEPG {
  prefix_stability
  utterance_completeness
  slot_completeness
  correction_risk
  action_risk
}
```

### 解释

- `prefix_stability`：当前文本前缀是否足够稳定
- `utterance_completeness`：语义上是否像一个已完成 thought
- `slot_completeness`：对命令/工具来说所需槽位是否足够可执行
- `correction_risk`：后续被追加语音/否定/补充推翻的风险
- `action_risk`：若下游动作不可逆，其提交门槛应显著更高

## 7.3 建议的三层 gate

### Gate A：`preview-ready`

允许：

- 展示 preview
- 进入 `previewing`
- 输出 listening / understanding cue
- 预热 LLM / TTS / planner
- 为 speaking-time duck_only 准备判断输入

### Gate B：`draft-ready`

允许：

- draft response
- tool planning candidate
- 低风险问答首句草拟
- speculative execution planning（但不真正执行）

### Gate C：`commit-ready`

允许：

- accepted turn
- response.start
- 低风险回复真正起播
- 低风险工具调用提交
- 高风险动作进入澄清/确认或更强 final 校验

## 7.4 slot completeness 的正式表达

对槽位 `s` 的可执行完备度，应使用分解对象，而不是 `filled=true/false`：

```text
SC(s,t|I,ctx) = Req * Fill * Normalize * Disambiguate * Stable
```

工程理解为：

- `Req`：当前意图是否真的需要该槽
- `Fill`：是否已听见候选值
- `Normalize`：是否可结构化
- `Disambiguate`：是否可映射到真实对象
- `Stable`：是否不容易被后续 correction 推翻

这对于：

- 智能家居设备控制
- 桌面对象操作
- 时间/数字类参数
- 高风险实体操作

尤其关键。

## 7.5 endpoint evidence 的推荐组成

```text
endpoint_evidence =
  acoustic_silence_or_pause
+ provider_endpoint_hint
+ lexical_complete_signal
+ semantic_complete_signal
+ slot_ready_signal
- correction_or_continuation_risk
```

但注意：

- 这更适合看作分层信号组合，不必急于做成单一大模型
- 当前阶段先让 runtime 能解释这些信号，并把 accept_reason 打出来，比追求完美单模型更重要

## 8. 输出链路架构

## 8.1 输出链路的目标

目标不是“拿到完整文字后再一次性 TTS”，而是：

- 文本增量、语义意群、音频起播三者真正重叠
- 第一意群出现时就可以启动 TTS
- output lane 能在 speaking 中持续接受 preview / interruption 信号

## 8.2 输出链路的推荐结构

```text
agent runtime streamed deltas
  -> response intent / style planner
  -> speech planner (sense-group / clause planner)
  -> incremental TTS
  -> playout controller
  -> playback facts / heard-text reconciliation
```

## 8.3 Speech Planner 的长期定位

`speech_planner` 不只是“把长文本切段”，而应承担：

- clause / sense-group 切分
- 首句优先策略
- 确认语 / 回答骨架 / 结论优先的口语化编排
- TTS 合成的 chunk 大小控制
- 中断后 restart/resume 的锚点管理

## 8.4 输出的三层内容形态

### A. `text draft`

- 可用于日志、字幕、debug 页面
- 不是直接等于最终 spoken text

### B. `speech plan`

- 将文字转成更适合说的话
- 例如把长句改成短句、先给结论再展开、加入短确认语

### C. `audio realization`

- 由 TTS 真正合成并下发
- 与 playback truth loop 绑定

## 8.5 输出链路的关键要求

### 1. 首句优先

优先快速形成一个：

- 安全
- 有用
- 自然
- 可继续扩展

的首句，而不是等完整长回答写完再开口。

### 2. 意群级起播

适合起播的单位更应是：

- clause
- sense-group
- short ack + answer lead

而不是 token 级碎片或整段全文。

### 3. 输出文本与 spoken text 可分离

对语音 agent 来说，spoken form 应允许和 text form 不完全一致，例如：

- 文本里保留更多细节
- spoken form 更口语、更分段、更适合人耳

## 9. interruption / 全双工仲裁

## 9.1 目标

真正的全双工不是“用户一开口就硬打断”，而是：

- 系统 speaking 时仍持续监听
- 系统先进入可逆的输出仲裁态
- 再根据更多证据收敛为：
  - `ignore`
  - `backchannel`
  - `duck_only`
  - `hard_interrupt`

## 9.2 四类输出策略定义

### `ignore`

- 噪声
- echo leakage
- 非 device-directed 声音
- 证据不足

### `backchannel`

- 短附和
- 不要求夺回话轮
- 系统可继续当前输出，必要时轻微调整节奏

### `duck_only`

- 短时间让话
- 不立即丢弃当前响应
- 等待更多 transcript / semantic 证据
- 是当前项目提升自然感的关键中间态

### `hard_interrupt`

- 明确 stop / correction / new request / takeover
- 立刻中止当前输出并切入下一轮

## 9.3 `duck_only` 的推荐判断模型

更推荐两段式判断，而不是一步四分类：

### 第一段：`intrusion_prior`

作用：决定是否先进入 `duck_only`

信号：

- acoustic intrusion
- overlap conflict
- local reflex hint
- anti-false-trigger penalty

### 第二段：`takeover_confirmation`

作用：决定收敛为 `hard_interrupt / backchannel / ignore`

信号：

- stable prefix
- takeover lexicon
- directedness
- output conflict
- backchannel likelihood
- ignore likelihood

这套设计与当前项目的实现复杂度、数据可得性、响应速度要求更匹配。

## 9.4 interruption 与 output lane 的关系

输出轨应显式支持以下状态：

- `idle`
- `planning`
- `speaking`
- `ducking`
- `interrupted`
- `completed`

输入轨与输出轨并行存在，意味着：

- speaking 中可以继续 preview 新输入
- preview 不必等 `CommitTurn()` 统一总闸门
- interruption 不再只是“清空一切后重开下一轮”

## 10. playback truth chain 与 heard-text

## 10.1 为什么这是架构级核心

如果系统不知道用户到底听到了什么，就无法自然做到：

- interruption 后继续说
- 纠错和补充不重复
- memory 准确写回
- 说过的话与听到的话保持一致

因此 `heard-text` 必须成为一等架构对象。

## 10.2 真相链的推荐表达

```text
generated_text
  -> delivered_text / sent_audio
  -> playback_started
  -> playback_progress / playback_mark
  -> playback_cleared / interrupted / completed
  -> heard_text_estimate
```

## 10.3 三类概念必须区分

### `generated`

模型/规划器打算说什么。

### `delivered`

服务端真的发给端侧什么。

### `heard`

基于播放事实推导的“用户大概率听到什么”。

任何把三者混为一谈的实现，都会在中断、resume、memory 上长期出错。

## 10.4 播放事实的分层精度

### Tier 0：纯服务侧启发式

- 只知道发了多少字节/时长
- 精度最低

### Tier 1：segment/mark ack

- `playback_started`
- `segment_mark_played`
- `playback_cleared`
- `playback_completed`

这是当前项目最值得优先落地的精度层。

### Tier 2：playout cursor aware

- `played_ms`
- `current_chunk_id`
- `clear_at_cursor`

### Tier 3：audibility aware

- mute / pause / route change / underrun / low-volume 等

## 10.5 当前项目的最小必须事实

我建议最小必须回传：

- `playback_started`
- `playback_cleared`
- `playback_completed`
- `segment_mark_played`

只要拿到这四类事实，heard-text 的可靠性就能从“纯猜”提升到“可用”。

## 10.6 memory 写回策略

memory 不应只写 assistant full text，应同时区分：

- generated text
- delivered text
- heard text
- interrupted/truncated state
- playback_completed state

对中断后的多轮一致性，应优先信任：

- `heard_text`
- 而不是完整 `generated_text`

## 11. 领域智能与语音理解增强

## 11.1 目标场景

当前项目的语音主场景更偏：

- 智能家居
- 桌面助理
- 设备/对象控制
- 信息问答 + 轻量工具调用

因此语音理解增强不应只靠通用 ASR，而应加入场景结构。

## 11.2 `dynamic bias list + alias + entity catalog`

推荐的最小结构：

```text
EntityCatalogItem {
  entity_id
  canonical_name
  entity_type
  namespace

  aliases_zh[]
  aliases_en[]
  abbreviations[]
  pinyin_aliases[]
  common_misrecognitions[]

  room_scope
  device_group
  action_compatibility[]
  risk_level

  frequency_prior
  recency_prior
  preview_bias_weight
  final_bias_weight
}
```

## 11.3 前通道与后通道使用方式不同

### preview 前通道

只吃：

- top-K dynamic bias
- 高频短 alias
- 当前 room / device group / session focus
- 轻量 session boost

目标：

- 快
- 稳
- 低误吸附

### final 后通道

可以吃：

- 更全 alias
- 拼音 / 常见误识别
- 更大 catalog
- 更强 normalization 与 disambiguation

目标：

- 准
- 可校正
- 支撑 action-grade 结构化理解

## 11.4 低置信与高风险动作的处理原则

### 低置信

系统应支持：

- 继续听
- 轻澄清
- 给候选
- 保守默认
- 明确拒绝执行

### 高风险动作

例如：

- 删除
- 支付
- 门锁
- 发消息给特定对象
- 危险家居控制

必须要求：

- 更高 `slot completeness`
- 更高 `final confidence`
- 明确确认或双重确认

## 11.5 口语化理解与 spoken-style 响应

长期来看，`internal/agent` 与 `internal/voice` 之间应形成以下协作：

- `agent runtime` 负责事实和推理
- `voice runtime` 负责说法和节奏

也就是：

- 智能性不等于直接把文字念出来
- 人性化依赖 `speech planner` 把回答变成适合“说出口”的形式

## 12. 观测、评测与质量闭环

## 12.1 语音质量不能只看单模块 benchmark

必须建立 milestone-oriented 观测体系。

### 核心时延里程碑

- `speech_start_visible_latency_ms`
- `preview_first_partial_latency_ms`
- `preview_stable_prefix_latency_ms`
- `endpoint_candidate_latency_ms`
- `turn_accept_latency_ms`
- `first_text_delta_latency_ms`
- `first_audio_chunk_latency_ms`
- `first_audible_playout_latency_ms`
- `barge_in_cutoff_latency_ms`

### 真相链指标

- `heard_text_ratio`
- `delivered_vs_heard_gap`
- `playback_clear_after_mark_count`
- `resume_anchor_accuracy`

### interruption 指标

- `duck_only_enter_rate`
- `duck_only_to_interrupt_rate`
- `false_interrupt_rate`
- `backchannel_misclassified_rate`

## 12.2 质量评测必须覆盖真实场景

### 场景桶

- 短命令问答
- 设备控制
- 长解释
- 纠错/否定
- 插话/打断
- 背景噪声
- 回声环境
- 远场/近场

### 声学桶

- 安静近讲
- 轻噪声
- 电视背景
- 空调/风扇
- 房间混响
- 双讲
- 自播音残留

### 任务桶

- 低风险信息问答
- 中风险控制
- 高风险操作
- 多实体歧义
- 长尾 alias

## 12.3 评测闭环的建议原则

- 不只看平均值，要看 p50 / p90 / p95
- 不只看总响应完成，要看里程碑前移
- 不只看生成文本，要看 heard-text 质量
- 不只看命中率，要看 false endpoint / false interrupt / false action

## 13. 运行时与部署架构

## 13.1 provider-neutral，profile-specific

逻辑架构应稳定，但运行配置可以按 profile 切换：

### Profile A：本地开源 GPU 主路径

- preview ASR：在线流式模型
- final ASR：更强准确率模型
- TTS：本地 GPU streaming TTS
- LLM：本地 OpenAI-compatible worker

### Profile B：本地语音 + 远端 LLM

- 保持语音链路本地低时延
- 把较重推理卸到远端

### Profile C：云 baseline / eval profile

- 作为对照验证
- 不作为主架构依赖

## 13.2 当前项目的现实约束

结合当前仓库和机器现状：

- 已有稳定 GPU FunASR worker 路径
- 已有 CosyVoice GPU TTS 路径
- LLM 路径正在本地 worker 化
- 当前硬件资源和 GPU 共享情况决定：语音实时性必须优先于“盲目上更大模型”

因此长期架构应明确：

- **语音主链路优先保障时延和稳定性**
- **LLM 模型大小按 GPU 余量 profile 化，而不是把语音链路挤爆**

## 13.3 预加载与 readiness gating

所有关键语音组件都应继续坚持：

- preload
- readiness health
- session start 前资源已就绪

避免第一次 turn 把下载/初始化时延暴露给用户。

## 13.4 为什么不建议把所有东西塞回一个进程

原因：

- 语音 worker 生命周期与 Go 主服务不同
- GPU 资源、模型缓存、Python 依赖更适合隔离
- 保持 provider-neutral boundary 更利于替换与回归

因此当前 `Go main + Python workers` 的分层仍是合理的。

## 14. 渐进式落地路线

## 14.1 第一阶段：把当前能力收束成“稳定主路径”

目标：

- `server_endpoint` 从 candidate 走向真正可用主路径候选
- preview partial 成为端侧可感知信号
- playback truth 至少具备 Tier 1 事实
- output orchestrator 真正实现更早起播

优先事项：

1. preview fast path
2. endpoint accept quality
3. first audible latency
4. playback mark/clear/completed telemetry
5. duck_only/backchannel 行为深度

## 14.2 第二阶段：把输入/输出双轨做成真实全双工

目标：

- speaking 中持续 preview
- interruption 不再只是 cancel-and-restart
- continue/resume 具备基本自然性
- backchannel 与 short interruption 不再全部硬停

优先事项：

1. dual-track behavior hardening
2. duck_only two-stage scoring
3. resume anchor from heard-text
4. speaking-time semantic preview

## 14.3 第三阶段：增强领域智能

目标：

- dynamic bias + alias + catalog 真正接入
- slot completeness 成为 action gating 的一部分
- 低置信澄清更自然
- 家居 / 桌面领域识别和执行更稳

优先事项：

1. dynamic top-K bias
2. alias recovery
3. action risk grading
4. clarification templates and policies

## 14.4 第四阶段：形成评测平台与数据闭环

目标：

- 里程碑时延与 heard-truth 指标有稳定归档
- 真实设备与真实环境回归可重复
- 调参与改模型不再靠主观印象

## 15. 与当前仓库的模块映射

## 15.1 `internal/session`

长期职责：

- session identity
- input/output lane state
- compatibility `state`
- accepted turn attribution
- response lifecycle ownership

## 15.2 `internal/voice`

长期职责：

- preview sessions
- turn detector
- UEPG / early processing
- interruption arbitration
- speech planner
- synthesizer orchestration
- playback truth reconciliation

这是本方案的主战场。

## 15.3 `internal/gateway`

长期职责：

- transport adaptation
- websocket lifecycle
- local transport-facing trace
- transport facts -> runtime callbacks

不应继续承载越来越多 adapter-local 语音策略。

## 15.4 `internal/agent`

长期职责：

- transport-neutral LLM/tool loop
- memory / skill / tool orchestration
- risk-based action policy
- structured response intent

## 15.5 `workers/python`

长期职责：

- speech worker internals
- model loading / GPU runtime / pipeline selection
- ASR/TTS implementation details
- optional KWS / streaming preview / endpoint hints

## 15.6 `docs/protocols`

长期职责：

- 对外协议仍由 shared runtime 边界驱动
- 任何 lane state、server_endpoint、playback facts 的公开化都必须 additive documented

## 16. 成功标准

## 16.1 体验成功标准

用户主观感知应出现明显变化：

- 更早知道系统在听
- 更早看到/听到系统开始理解
- 更少觉得系统“等很久才接话”
- 更少觉得系统“插话打不断”
- 更少觉得系统“说过了但我没听到”
- 更少出现误听后直接误执行

## 16.2 架构成功标准

- 语音主逻辑继续集中在 `internal/voice`
- gateway 不再膨胀成第二编排层
- device/channel adapter 不直接调用 provider
- 新模型、新 provider、新 adapter 接入时不重写会话语义

## 16.3 落地成功标准

- 当前协议兼容路径继续可用
- 新主路径以 capability/additive 方式逐步开放
- 有稳定日志与 artifact 能证明优化带来真实体感提升

## 17. 参考资料

### 项目内文档

- `docs/architecture/realtime-full-duplex-gap-review-zh-2026-04-15.md`
- `docs/architecture/server-driven-turn-taking-vs-client-commit-zh-2026-04-16.md`
- `docs/architecture/server-primary-hybrid-min-device-capabilities-and-interruption-zh-2026-04-16.md`
- `docs/architecture/unified-early-processing-threshold-zh-2026-04-16.md`
- `docs/architecture/latency-budget-and-subjective-feel-zh-2026-04-16.md`
- `docs/architecture/playback-facts-and-heard-text-truth-chain-zh-2026-04-16.md`
- `docs/architecture/duck-only-dynamic-scoring-zh-2026-04-16.md`
- `docs/architecture/dynamic-bias-alias-entity-catalog-mvp-zh-2026-04-16.md`
- `docs/architecture/slot-completeness-computable-object-zh-2026-04-16.md`
- `docs/adr/0024-voice-runtime-owns-preview-playout-and-heard-text.md`
- `docs/adr/0028-local-funasr-worker-keeps-2pass-asr-and-kws-internal.md`
- `docs/adr/0029-server-endpoint-candidate-is-discovery-advertised-before-default.md`
- `docs/adr/0030-true-full-duplex-requires-dual-track-session-state-before-more-endpoint-tuning.md`
- `docs/adr/0031-soft-output-arbitration-and-early-audio-stay-runtime-internal.md`

### 外部参考

- OpenAI Realtime VAD：
  - <https://platform.openai.com/docs/guides/realtime-vad>
- OpenAI Realtime conversations / truncate：
  - <https://platform.openai.com/docs/guides/realtime-conversations>
  - <https://platform.openai.com/docs/api-reference/realtime-client-events/conversation/item/truncate>
- Google：
  - `Low Latency Speech Recognition using End-to-End Prefetching`
    <https://research.google/pubs/low-latency-speech-recognition-using-end-to-end-prefetching/>
  - `Unified End-to-End Speech Recognition and Endpointing for Fast and Efficient Speech Systems`
    <https://research.google/pubs/unified-end-to-end-speech-recognition-and-endpointing-for-fast-and-efficient-speech-systems/>
  - `Dissecting User-Perceived Latency of On-Device E2E Speech Recognition`
    <https://www.isca-archive.org/interspeech_2021/shangguan21_interspeech.html>
- Amazon：
  - `Accurate Endpointing with Expected Pause Duration`
    <https://www.amazon.science/publications/accurate-endpointing-with-expected-pause-duration>
  - `Personalized Predictive ASR for Latency Reduction in Voice Assistants`
    <https://www.amazon.science/publications/personalized-predictive-asr-for-latency-reduction-in-voice-assistants>
- LiveKit：
  - <https://docs.livekit.io/agents/logic/turns/>
  - <https://docs.livekit.io/agents/logic/turns/adaptive-interruption-handling>
  - <https://docs.livekit.io/reference/agents/turn-handling-options/>
  - <https://docs.livekit.io/agents/logic-structure/turns/turn-detector/>
- Deepgram Flux：
  - <https://developers.deepgram.com/docs/flux/voice-agent-eager-eot>
  - <https://developers.deepgram.com/docs/flux/agent>
  - <https://developers.deepgram.com/docs/measuring-streaming-latency>
- AssemblyAI Universal Streaming：
  - <https://www.assemblyai.com/docs/universal-streaming/turn-detection>
  - <https://www.assemblyai.com/docs/streaming/universal-streaming>
- Twilio Media Streams：
  - <https://www.twilio.com/docs/voice/media-streams/websocket-messages>
- FunASR 官方仓库：
  - <https://github.com/modelscope/FunASR>
  - <https://github.com/FunAudioLLM/Fun-ASR>
