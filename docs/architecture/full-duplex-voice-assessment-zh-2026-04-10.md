# 当前项目“流畅、自然、全双工”语音交互能力评估（2026-04-10）

## 目的

本文聚焦一个具体问题：

当前 `agent-server` 是否已经具备流畅、自然、全双工语音交互能力？如果还没有，差距主要在哪里？业内较优秀的现代方案通常怎么做？结合当前项目，最合理的演进路线是什么？

本文结论基于两部分信息：

- 仓库当前实现与协议、运行时、网关代码的本地审视
- 2026-04-10 可查的官方公开资料与成熟开源方案对比研究

## 核心结论

截至 2026-04-10，当前项目还不能算已经实现了真正意义上的“流畅、自然、全双工”语音交互。

更准确地说，当前项目已经具备：

- 双向实时传输
- 文本流式输出
- 流式 TTS 输出接口
- 说话中断与抢话
- 多接入端共享同一会话核心

但整体形态仍更接近：

- 可打断的流式回合制
- 准全双工
- `interruptible turn-based voice interaction`

它距离现代高质量全双工语音 Agent 仍然缺少一层关键能力：`Voice Orchestration Core`，也就是把 turn detection、连续收音理解、增量回复规划、增量 TTS、打断恢复、输出截断、媒体工程放进同一条共享运行时主干，而不是分散在 ASR/TTS 工具调用和设备页面逻辑中。

一句话总结：

当前项目的主架构方向是对的，但语音运行时还不够“厚”。

## 评估标准

为了避免把“能实时传音频”误判成“全双工语音 Agent”，这里把目标拆成三层：

### 1. 传输层全双工

要求：

- 双向实时传输音频和控制事件
- 边上传边下发
- 支持中途取消或打断

当前项目：基本具备。

### 2. 交互层全双工

要求：

- 用户和系统都能在会话中持续存在
- 系统输出时用户插话能被自然处理
- turn 的结束不完全依赖显式提交
- 打断后能平滑衔接下一轮，而不是简单粗暴地 cancel 当前输出

当前项目：部分具备，但不完整。

### 3. 认知层全双工

要求：

- 用户还在说时，系统就能持续理解
- 系统可以在理解未完全结束时开始规划回复
- TTS 可以跟随增量文本或语义单元启动
- 被打断后，只把用户“实际听到的内容”进入后续上下文

当前项目：尚未形成。

## 当前项目已具备的基础能力

当前仓库已经打下了几个非常重要的基础，这些基础并不弱：

- 有清晰的 `Realtime Session Core` 边界，且设备适配层没有直接调用模型提供方
- 有统一的 `Agent Runtime Core`
- 有统一的 `Voice Runtime`
- 有共享的流式 turn delta 通道
- 有统一的 TTS 输出边界，而不是浏览器独享能力
- 有 turn trace / trace id 的可观测性基础

相关文档与实现：

- [架构概览](overview.md)
- [实时会话协议](../protocols/realtime-session-v0.md)
- [运行时配置](runtime-configuration.md)
- [语音响应器](../../internal/voice/asr_responder.go)
- [流式 turn 执行桥接](../../internal/voice/turn_executor.go)
- [实时网关](../../internal/gateway/realtime_ws.go)

这些基础说明：

当前项目并不是“架构不对所以做不了”，而是“语音编排能力还没有真正长成”。

## 为什么当前实现还不算真正全双工

### 1. 当前公开契约仍然是 commit 驱动

当前协议和公开文档明确发布的是 `client_wakeup_client_commit`，而不是 server-side turn detection。

相关证据：

- [docs/architecture/overview.md](overview.md)
- [docs/protocols/realtime-session-v0.md](../protocols/realtime-session-v0.md)
- [docs/architecture/runtime-configuration.md](runtime-configuration.md)

这意味着：

- 唤醒由客户端触发
- 每轮音频结束由客户端显式 `audio.in.commit`
- 服务端尚未把 VAD 或语义收尾作为公开能力发布

这对 bring-up 非常友好，但会限制自然对话体验。

### 2. 输入侧主路径仍然是“整轮音频 -> ASR -> LLM”

原生实时网关在收到 `audio.in.commit` 之后，才真正提交 turn 并进入推理流程。

相关实现：

- [internal/gateway/realtime_ws.go#L381](../../internal/gateway/realtime_ws.go#L381)

语音 runtime 当前的 `Transcriber` 仍然是一次性 `Transcribe(...)` 接口：

- [internal/voice/contracts.go#L112](../../internal/voice/contracts.go#L112)
- [internal/voice/asr_responder.go#L72](../../internal/voice/asr_responder.go#L72)

这说明目前主流程不是：

- 连续上行音频
- 连续生成 partial hypothesis
- 连续判断 turn 是否结束
- 连续准备下一步回复

而仍是：

- 收完这一轮
- 做一次转写
- 再进入 LLM

### 3. ASR partial 和 endpoint 信息还不是 turn control 的一等信号

当前代码已经支持把 `endpoint_reason`、`partials`、`segments` 等数据归一化进 metadata：

- [internal/voice/speech_metadata.go](../../internal/voice/speech_metadata.go)

但这些信息目前只作为补充语义元数据传入 `internal/agent`，并没有真正驱动：

- turn 结束判定
- 是否继续等待用户补充
- 是否提前触发回复准备
- 中断和恢复策略

因此它们现在更像“有记录”，而不是“有控制力”。

### 4. 当前还不是“边生成边说”

当前 LLM 确实已经支持流式文本 delta：

- [internal/agent/llm_executor.go#L174](../../internal/agent/llm_executor.go#L174)
- [internal/voice/turn_executor.go#L17](../../internal/voice/turn_executor.go#L17)

但语音输出仍然是在拿到最终 `turn.Text` 后，才调用 TTS：

- [internal/voice/asr_responder.go#L98](../../internal/voice/asr_responder.go#L98)
- [internal/voice/synthesis_audio.go#L9](../../internal/voice/synthesis_audio.go#L9)

网关也要等 `TurnResponse` 汇总完，才进入最终音频播放阶段：

- [internal/gateway/realtime_ws.go#L747](../../internal/gateway/realtime_ws.go#L747)

这意味着当前系统虽然能：

- 文本先流出来
- 音频再尽快流出来

但还不是：

- LLM 一边生成短句
- 语音 runtime 一边切分可播子句
- TTS 一边启动
- 客户端几乎立即进入“对话中”

这正是自然感差异最大的地方之一。

### 5. 打断目前是硬 cancel，不是自然打断控制

当前 speaking 阶段收到新音频时，会优先调用 `interruptOutput(...)` 终止当前输出：

- [internal/gateway/realtime_ws.go#L248](../../internal/gateway/realtime_ws.go#L248)
- [internal/gateway/realtime_runtime.go#L61](../../internal/gateway/realtime_runtime.go#L61)

这已经比完全不可打断强很多，但还缺少几个现代语音系统常见能力：

- false interruption 识别
- backchannel 区分，例如“嗯”“对”“好”
- 被打断后是否恢复剩余输出
- 打断点之后如何修正 assistant 已说内容
- 输出被截断后如何与 memory 同步

### 6. 当前 memory 记录的是完整 assistant 文本，而不是“用户实际听到的文本”

当前 `LLMTurnExecutor` 在 turn 结束后保存的是完整 `output.Text`：

- [internal/agent/llm_executor.go#L95](../../internal/agent/llm_executor.go#L95)
- [internal/agent/runtime_backends.go#L101](../../internal/agent/runtime_backends.go#L101)

这在文字聊天里通常没问题，但在可打断语音对话里会产生典型偏差：

- 系统说了一长段
- 用户中途打断
- 用户其实没听完整段
- 但上下文记忆里仍然保存了完整回复

这会让后续追问、纠错、接话都变得不够自然。

### 7. 浏览器侧已有基础媒体处理，但还没有共享媒体工程层

当前 Web/H5 调试端已经启用了：

- `echoCancellation`
- `noiseSuppression`
- `autoGainControl`

相关实现：

- [clients/web-realtime-client/app.js#L1045](../../clients/web-realtime-client/app.js#L1045)
- [internal/control/webh5_assets/app.js#L865](../../internal/control/webh5_assets/app.js#L865)

但这些能力还停留在页面级，尚未形成统一的共享语音媒体工程体系，例如：

- playout-aware interruption
- 设备侧回声泄漏和误打断协同治理
- jitter / playout buffer 观测
- first-audio-byte 和 playout-complete 指标联动

## 当前项目的真实能力定位

综合判断，当前项目更适合被定位为：

- 有较好架构基础的会话型语音 Agent 服务
- 已具备流式文本和流式音频回复能力
- 已具备 barge-in 基础能力
- 已具备多接入方式共享同一会话核心的能力

但它还不是：

- 真正连续语音会话 runtime
- 现代自然全双工 spoken dialogue 系统
- 原生 speech-to-speech 对话引擎

## 当前优秀方案通常怎么做

### OpenAI Realtime

OpenAI 的 Realtime 能力已经明确把 turn detection 作为一等能力，支持 `server_vad` 和 `semantic_vad` 两类模式。

它体现出的设计方向是：

- turn detection 不是客户端页面临时逻辑，而是服务端会话能力
- VAD 和语义判定都可以进入 turn control
- 实时语音对话天然支持中断、自动回复、低延迟连贯体验

官方参考：

- [OpenAI Realtime VAD](https://developers.openai.com/api/docs/guides/realtime-vad)
- [OpenAI GPT Realtime](https://developers.openai.com/api/docs/models/gpt-realtime-1.5)

### Gemini Live API

Gemini Live 代表另一条很重要的现代路线：把 native audio conversation、tool use、情感表达、实时会话控制做成统一 Live API。

它体现出的方向是：

- 音频不只是输入输出介质，而是会话原生模态
- 打断和持续对话被视为产品默认能力
- “更像伙伴”来自端到端会话行为，而不只是更强文本模型

官方参考：

- [Gemini Live API](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/live-api)
- [Gemini 2.5 Flash Live](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/models/gemini/2-5-flash-live-api)

### LiveKit Agents

LiveKit 的价值不只是 transport，而是它把 turn handling 做成了清晰的 agent runtime 能力：

- context-aware turn detection
- adaptive interruption
- false interruption handling
- speech resume

这类系统非常值得借鉴，因为它们解决的是“主观体验像不像真的在对话”这个问题。

官方参考：

- [LiveKit Agents Turns](https://docs.livekit.io/agents/logic/turns/)

### Pipecat

Pipecat 在实时语音编排方面有很强的工程参考价值，尤其是：

- user turn strategy
- smart turn detection
- pipeline 中断与恢复
- 多 provider、多 transport 的统一实时编排

对当前项目最重要的启发是：turn management 不能只是 ASR 结束后的附属判断，而要成为共享 runtime 主干。

官方参考：

- [Pipecat User Turn Strategies](https://docs.pipecat.ai/api-reference/server/utilities/turn-management/user-turn-strategies)
- [Pipecat Smart Turn](https://docs.pipecat.ai/api-reference/server/utilities/turn-detection/smart-turn-overview)

### Moshi 等原生 spoken dialogue 路线

Kyutai Moshi 这类研究型系统代表的是更长期的 full-duplex speech-native 路线。

其价值在于提醒我们：

- 真正自然的 spoken dialogue 未必总要经过稳定的 “ASR -> text LLM -> TTS” 串联
- 短反馈、重叠、插话、接话是语音对话中的原生行为

但对当前项目而言，这更像长期技术观察方向，而不是最务实的下一步落地方案。

公开参考：

- [Moshi 论文](https://kyutai.org/Moshi.pdf)

## 外部优秀方案的共同点

从这些方案里可以提炼出几个高度一致的共性：

### 1. turn detection 是共享运行时能力

不是靠单一设备页面或单一端上逻辑临时判断。

### 2. partial ASR 必须有控制价值

不仅要看到 partial，还要让 partial 参与：

- turn 结束判断
- 是否继续等用户补充
- 是否提前准备回复

### 3. 打断不是布尔值，而是一套策略

需要处理：

- 真打断
- 误打断
- 附和音
- 是否恢复刚才没说完的内容

### 4. TTS 要尽量前移

优秀体验普遍追求：

- 更早的 first audio byte
- 子句级增量播报
- 边生成边说

### 5. 输出截断必须回写上下文

系统知道自己原本想说什么还不够，还要知道用户到底听到了什么。

### 6. 媒体工程是核心能力，不是附属细节

包括：

- 回声消除
- 降噪
- 音量控制
- 抖动和播放节奏
- 打断瞬时切断延迟

## 结合当前项目，最合理的演进判断

### 不建议推翻当前架构

当前项目的中心化会话架构仍然是合理的：

- `Realtime Session Core` 保持中心
- 设备和 channel adapter 仍是 ingress/egress
- provider 集成仍应隐藏在 runtime 接口之后

这套边界非常适合继续演进。

### 真正需要增强的是 `internal/voice`

当前最值得升级的不是换一个更强模型，而是把 `internal/voice` 从“ASR/TTS 工具层”提升成“语音编排核心”。

更理想的目标形态应该包含：

- `StreamingTranscriber`
- `TurnDetector`
- `InterruptionArbiter`
- `IncrementalResponderPlanner`
- `IncrementalSynthesizerScheduler`
- `OutputTruncator`
- `HeardStateStore`
- `MediaTelemetry`

其中最关键的不是名字，而是职责：

- 连续理解
- 连续判断
- 连续准备
- 连续播报
- 连续修正上下文

## 推荐路线

### 路线 A：在现有架构上补齐现代 Voice Orchestration Core

这是最符合当前仓库方向的路线，也是我最推荐的路线。

优点：

- 保留当前 session-centric 架构
- 不破坏现有 RTOS、Web/H5、xiaozhi 兼容路径
- 可以逐步落地
- 继续保持 provider-neutral

缺点：

- 研发复杂度高于“直接接一个托管式实时语音模型”
- 需要补齐较多 runtime 接口与评测指标

### 路线 B：在共享 voice boundary 后增加 provider-native realtime backend

例如接入：

- OpenAI Realtime
- Gemini Live
- 更成熟的语音原生云服务

但关键约束不应改变：

- 设备适配层不能直接调用 provider
- channel skill 不能直接接模型
- 仍应挂在共享 voice runtime 后面

优点：

- 最快提升主观体验
- 更快接近自然全双工

缺点：

- 成本和外部依赖更高
- provider lock-in 风险更明显

### 路线 C：走本地/开源全链路增强

例如：

- 流式 ASR
- semantic turn detector
- 增量 TTS
- 更强本地模型编排

优点：

- 可控性强
- 更符合长期本地化目标

缺点：

- 从工程投入和主观效果看，短期通常不如托管式 realtime provider 见效快

## 对当前项目的建议优先级

### P0

- 新增 server-side turn detection 模式，不再只有 `audio.in.commit`
- 在 `internal/voice` 中引入 streaming ASR 接口
- 让 `speech.partials` 与 `speech.endpoint_reason` 真正参与 turn control
- 增加关键实时语音指标：
  - first text delta
  - first audio byte
  - end-of-turn latency
  - interrupt cut-off latency
  - false interruption rate

### P1

- 把 TTS 从“最终文本合成”提升到“子句级增量合成”
- 引入 interruption arbiter
- 增加输出截断与 heard-text 同步
- 修正 memory 保存逻辑，只保存用户实际听到的 assistant 输出

### P2

- 评估 browser/desktop 可选 WebRTC transport
- 评估 provider-native realtime backend
- 持续跟踪 speech-native 路线，例如 Moshi 类模型或后续更成熟实现

## 最终判断

当前项目还不能说已经实现了流畅、自然、全双工的语音交互。

更准确的判断是：

- 现在已经具备了比较好的“可打断、可流式、可多端接入”的会话基础
- 但仍然属于回合制主导的语音 Agent
- 距离现代高质量全双工体验，最主要差距在语音编排层，而不只是在模型本身

如果只继续在当前 commit 驱动链路上做局部修修补补，体验提升会比较有限。

如果把 `internal/voice` 正式升级为共享的 `Voice Orchestration Core`，当前项目是有机会向高质量自然语音 Agent 演进的。

## 参考资料

- [OpenAI Realtime VAD](https://developers.openai.com/api/docs/guides/realtime-vad)
- [OpenAI GPT Realtime](https://developers.openai.com/api/docs/models/gpt-realtime-1.5)
- [Gemini Live API](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/live-api)
- [Gemini 2.5 Flash Live](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/models/gemini/2-5-flash-live-api)
- [LiveKit Agents Turns](https://docs.livekit.io/agents/logic/turns/)
- [Pipecat User Turn Strategies](https://docs.pipecat.ai/api-reference/server/utilities/turn-management/user-turn-strategies)
- [Pipecat Smart Turn](https://docs.pipecat.ai/api-reference/server/utilities/turn-detection/smart-turn-overview)
- [Moshi 论文](https://kyutai.org/Moshi.pdf)
