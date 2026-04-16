# 服务侧语音优化建议（深度研究，2026-04-16）

## 文档定位

- 性质：研究结论 / 服务侧优化建议
- 目标：结合当前 `agent-server` 现状，参考 OpenAI、Google、Amazon、Apple、FunASR 以及主流开源实时语音实践，给出尽可能贴合本项目的服务侧优化建议
- 适用阶段：当前研究阶段到后续实现阶段
- 当前前提：
  - 继续坚持 `Realtime Session Core` 为中心
  - 继续坚持 `server-primary hybrid`
  - 继续坚持本地 / 开源优先的 `STT -> Agent Runtime -> TTS` 级联主路径
  - 暂不为追求“理论最先进”而切到全新重型语音大模型黑盒架构

## 一句话结论

当前项目下一阶段最该做的，不是继续横向接更多模型或继续堆更多协议细节，而是把服务侧语音主链从“已经有能力”推进到“行为足够成熟”：

1. 把 **turn-taking** 从“静音 + 词法补丁”升级为 **多信号、分层、可学习的服务侧裁决**；
2. 把 **interruption** 从“partial 文本驱动”升级为 **声学优先 + 语义确认 + playback truth 收口**；
3. 把 **early processing** 从“prewarm 零散点状前推”升级为 **分层可撤销前推链**；
4. 把 **speech planner** 从“按字数切块”升级为 **按意群 / 句法 / 风格约束的起播编排器**；
5. 把 **heard-text** 从“近似可用”升级为 **带时间轴与对齐粒度的播放真相链**；
6. 把 **ASR domain bias** 从研究结论升级为 **动态 bias list + alias + entity catalog 的运行时服务**；
7. 把 **优化目标** 从“总时延”升级为 **分里程碑体验优化**。

如果这 7 点做扎实，当前仓库会比单纯换一个更大的 ASR / TTS / LLM 模型更明显地提升“流畅、自然、人性化、智能性”。

## 1. 当前项目的真实位置

结合当前代码与既有文档，本项目服务侧已经不再是一个简单半双工 demo：

- `internal/session` 已有 `input_state / output_state` 双轨
- `internal/voice.SessionOrchestrator` 已经接管 preview、playout、heard-text persistence
- `server_endpoint` 已经是 main-path candidate，而非纯 debug 开关
- `duck_only / backchannel / hard_interrupt` 已有共享 runtime 路径
- `TurnResponseFuture + speech planner` 已开始支持更早起播
- `playback_ack -> heard-text -> voice.previous.*` 已经打通到下一轮 runtime context
- `stable_prefix -> TurnPrewarmer` 已经开始把 preview 变成 runtime-owned 早处理信号

但从服务侧体验上限看，当前仍有 5 个核心瓶颈：

1. **turn accept 仍偏规则型**：`SilenceTurnDetector` 目前主要还是 `audio + silence + lexical completeness + endpoint hint` 的启发式组合，缺少更强的 pause/context/semantic modeling。
2. **barge-in 仍偏 transcript 近似**：`EvaluateBargeIn(...)` 主要依赖 `audioMs + looksLikeBackchannel + looksLexicallyComplete`，还没有真正的声学级 interrupt verifier。
3. **speech planner 仍偏 chunk heuristic**：当前 `SpeechPlanner` 本质上还是围绕 rune 数和基本边界在切块，还不是完整的 clause/prosody planner。
4. **early processing 还没有统一分层门槛**：虽然已有 preview finalize、stable prefix prewarm，但“何时 preview / 何时 prewarm / 何时 speculative / 何时 accept / 何时可做高风险动作”仍未统一进一个共享 gate object。
5. **domain bias 还没有进入主运行时**：现有研究已经充分，但实体目录、alias、session-scoped bias list、slot completeness 仍未真正喂给实时 ASR / early processing / risk gating 主链。

## 2. 外部实践给出的关键启发

## 2.1 OpenAI：turn-taking 不能只靠静音，服务端要有语义级 stop 判断

OpenAI 的 Realtime VAD 文档把 turn detection 明确分成两类：

- `server_vad`：基于静音阈值
- `semantic_vad`：基于“用户这句话是否已经说完”的语义判断

这对本项目的启发非常直接：

- 当前 `SilenceTurnDetector` 已经从纯静音推进到“静音 + lexical completeness + endpoint hint”
- 但离真正的 `semantic_vad` 还有距离
- 下一步最应该补的不是更多固定阈值，而是一个 **语义/上下文 aware 的 turn completion scorer**

同时，OpenAI 在音频指南中仍保留了 `STT -> LLM -> TTS` 级联路径，原因是它在可控性、文本工具调用、业务约束、调试可解释性方面更强。这与当前项目现状高度一致：

- 本项目当前不应急于切到端到端 speech-to-speech 黑盒
- 更适合把服务侧 orchestration 做强，在现有 cascade 边界内继续压时延和自然度

## 2.2 Google：终止点判断是独立问题；two-pass 与 prefetch 仍然是高 ROI 路线

Google 在 end-of-query / endpointing 研究里持续强调：

- “用户是否停止发声”与“系统是否应该接受本轮 turn”不是一回事
- pause、hesitation、filler、语速、上下文都应参与 endpoint 判断

同时，Google 的多篇 two-pass 研究说明了一条非常契合本项目的路线：

- 第一阶段保持 **流式、快速、可前推**
- 第二阶段用 **更强但稍慢的纠错 / rescoring / deliberation** 提升准确率
- 可以通过 **prefetch** 提前触发下游，减少 accepted-turn 后的空等

这和当前项目已经做出的两步高度一致：

- `preview finalize fast path`
- `stable_prefix prewarm`

但当前项目仍缺三层能力：

1. prewarm 还太保守，只做 exact match reuse；
2. 没有对 ambiguity turn 做单独的 speculative tier；
3. 还没有把 early semantic prediction 明确变成独立模块。

换句话说，**当前项目不缺 two-pass 思路，缺的是把 two-pass 思路从“ASR 层”继续推广到“会话编排层”。**

## 2.3 Amazon：voice assistant 的 latency 优化，本质是 endpointing、prefetch、streamable SLU、context biasing 的组合

Amazon 在 Alexa / voice assistant 方向给了几类很强的启发：

### A. endpointing 不是单阈值问题

Amazon 的 endpointing / EOS 研究明确区分：

- 更早结束可降低 latency
- 但过早 cut-off 会明显伤害真实体验
- 因此应该让 first-pass detector 与 second-pass arbitrator 组合，而不是只调一个 silence 值

这和本项目当前 `server_endpoint` 路径极其相关：

- 当前已经有 `server_endpoint + incomplete_hold + endpoint_hint`
- 下一步最该补的，是一个 **EP Arbitrator / Turn Arbitrator**，而不是继续单点调 `silence_ms`

### B. prefetch 是 voice assistant 的核心 latency 武器

Amazon 的 predictive ASR 工作强调：

- 可以把 preliminary hypothesis 甚至 predicted full utterance 提前交给下游系统
- 如果最终结果匹配，就能把后段 latency 部分隐藏掉

这与当前项目的 `TurnPrewarmer` 十分契合。区别只是：

- 现在本项目只 prewarm prompt/memory/tools
- 后续可以扩展到 **tool shortlist / response policy draft / domain bias narrowing / TTS voice style pre-selection**

### C. streamable SLU 比“等 final transcript 后再做 NLU”更适合实时语音

Amazon 的 streamable SLU / semantic decoder 研究很重要的一点是：

- 语义理解不一定要等最终全文确定后才开始
- 可以在流式识别阶段就逐步形成 intent/slot hypothesis

这与本项目之前讨论的 `stable prefix + utterance completeness + slot completeness` 几乎完全对齐。

结论是：**本项目后续的 early processing，最值得补的不是“更快 commit”，而是“更早的可撤销语义假设”。**

### D. contextual biasing 不能只做静态 hotwords

Amazon 在 contextual biasing / slot-triggered biasing 上的研究说明：

- bias list 不应该始终全开
- 应该按 domain / slot / semantic context 动态收窄
- 否则很容易 false biasing

这对当前项目尤为关键，因为我们正处于“智能家居 / 桌面助理”场景：

- 设备名、房间名、App 名、联系人、别名都非常适合 bias
- 但一旦全局硬 bias，误识别副作用会很明显

## 2.4 Apple：自然交互体验的关键，不只是识别准确率，而是 talking turns / device-directedness / follow-up context

Apple 在 `Talking Turns`、DDSD、follow-up speech 等研究里有一个非常一致的方向：

- 真实 spoken dialogue 的问题，不只是 ASR WER
- 而是系统是否知道：
  - 这句话是不是对设备说的
  - 这次停顿是不是想继续
  - 这轮 follow-up 是新问题还是承接上一轮
  - 什么时候该 backchannel，什么时候该继续等待

这对本项目的核心启发是：

- 当前 `server_endpoint` 不能只被理解成“服务端替客户端 commit”
- 它实际上应演进成一个 **turn-taking runtime**
- 其中至少要显式建模：
  - `turn_complete_likelihood`
  - `device_directedness / addressness`
  - `follow_up_continuation_likelihood`
  - `barge_in_intent_likelihood`

Apple 的 `ChipChat` 也说明：经过足够精细的 streaming redesign 后，cascaded voice system 依然能做到很低延迟，而且在理解与控制上依然很有竞争力。

这再次支持当前项目：**不要急着推翻 cascade，先把 cascade 做对。**

## 2.5 开源框架：LiveKit / Pipecat 都在把“turn detection”从 VAD-only 升级为独立层

LiveKit 和 Pipecat 的共同趋势非常清晰：

- VAD 仍然保留
- 但 turn detection 已经独立成一个可插拔层
- interruption 也不再完全等 transcript，而是尽量 acoustic-first
- 系统会记录更细的 latency 和 interruption/turn lifecycle 指标

对本项目最值得借鉴的不是它们的协议，而是它们的运行时思想：

1. **turn detector 是独立组件**，不是塞在 STT provider 的附属功能里；
2. **adaptive interruption** 应优先走声学信号；
3. **transcript 与 playback 应尽量对齐**，被打断时要截断到“用户真正听到”的部分；
4. **latency 需要拆到 TTFS / first speech / interruption cutoff / per-service breakdown**。

这和本项目现有边界其实非常兼容：

- `internal/voice` 正好就是承载这些能力的地方
- 不需要再引入第二套 orchestration core

## 2.6 FunASR：2pass、VAD、KWS、domain model 的组合仍是当前阶段最契合本项目的本地开源路线

FunASR 官方实践清楚说明：

- streaming 模型、VAD、punc、KWS、hotword、domain model 是可组合的
- Paraformer streaming + final correction 是一个很自然的 2pass 方向
- 流式与非流式并不是二选一，而是典型的配合关系

这与本项目当前 worker 边界完全匹配。

但真正要注意的是：

- FunASR 的 2pass 思路应该成为 **上游信号体系** 的一部分
- 而不应被限制为“worker 里做完就结束”
- 服务侧 runtime 要消费更多结构化信号，而不是只拿 `partial text` 和 `final text`

## 3. 对本项目最重要的服务侧优化建议

下面是结合当前项目状态，我认为最值得依次推进的 8 个服务侧优化建议。

## 3.1 把 `SilenceTurnDetector` 升级为 `MultiSignalTurnArbitrator`

### 为什么最重要

当前 turn accept 仍然是体验上限最大的瓶颈之一。

### 建议方向

新增一个共享服务侧对象，用统一 evidence 评分而不是只看单一阈值：

- acoustic pause / tail energy / VAD stop
- streaming ASR endpoint hint
- stable prefix stability
- utterance completeness
- slot completeness
- device-directedness / follow-up likelihood
- dialogue state（当前在听、在说、刚被打断、是否上一轮未播完）

### 推荐输出

不要只输出 `CommitSuggested bool`，而应输出分层结果：

- `preview_only`
- `prewarm_allowed`
- `draft_allowed`
- `accept_candidate`
- `accept_now`
- `wait_for_more`

### 与当前代码的贴合点

- `internal/voice/turn_detector.go`
- `internal/voice/asr_responder.go`
- `internal/voice/session_orchestrator.go`

当前 `SilenceTurnDetector` 可以保留为 fast path evidence provider，但不应继续承担全部 turn 决策职责。

## 3.2 把 barge-in 升级为“声学优先、语义确认”的两阶段仲裁

### 当前问题

`EvaluateBargeIn(...)` 目前主要根据：

- audio duration
- backchannel token
- lexical completeness

这足以做 demo，但还不够支撑更自然的真实设备体验。

### 建议方向

第一阶段：超低时延 acoustic gate

- 是否有人声侵入
- 能量、持续时间、说话速率突变
- 与当前 TTS 的重叠关系
- 是否更像附和音 / laughter / side talk / true take-over

第二阶段：语义确认

- stable prefix
- completeness
- slot trigger
- 语义是否新意图 / 澄清 / 继续 / 纠正

### 推荐结果状态

- `ignore`
- `backchannel`
- `duck_only_enter`
- `duck_only_hold`
- `duck_only_release`
- `hard_interrupt`
- `resume_previous_output`

### 关键点

`duck_only` 应继续被视为 **短时、可逆中间态**，不要把它做成稳定终态。

## 3.3 把 early processing 正式升级成“分层可撤销前推链”

### 当前问题

现在已有：

- preview finalization reuse
- stable prefix prewarm

但还缺统一分层语义。

### 建议方向

把当前服务侧前推分成 5 层：

1. `visible preview`：仅给端侧 / 日志可见
2. `runtime prewarm`：prompt/memory/tools 准备
3. `semantic draft`：intent/slot/entity/response policy 草案
4. `tool shortlist`：只做候选，不执行不可逆工具
5. `accept + commit`：真正形成正式 turn

### 这样做的收益

- 能继续压 `endpoint_accept -> first_text_delta`
- 能继续压 `first_text_delta -> first_audio_chunk`
- 又不会把误判代价提前放大

### 对本项目的具体意义

- 当前 `TurnPrewarmer` 是对的，但还太窄
- 下一步不应立刻执行工具，而应先补 `semantic draft / tool shortlist`

## 3.4 把 `SpeechPlanner` 从字数切块升级为 clause / prosody planner

### 当前问题

当前 planner 已经能更早起播，但仍偏“按文本长度和基本边界切块”。

### 建议方向

将 planner 输入从纯 `text delta` 扩展为：

- punctuation / phrase boundary
- LLM delta type
- intent class（回答 / 澄清 / 确认 / 反问 / 列举）
- output style（简短答复 / 家居命令确认 / 情感回应）
- TTS provider feedback（可选）

### planner 应输出的不只是文本片段

还应输出：

- `clause_text`
- `speech_act`
- `urgency`
- `interruptibility`
- `prosody_hint`
- `can_start_before_turn_finalized`

### 实际目标

让首段 TTS 更像“意群”而不是“切开的字符段”。

## 3.5 把 playback truth 从“字符串边界”推进到“时间轴 + 对齐粒度”

### 当前问题

本项目已经把 playback facts 真正接进了 heard-text 主链，这是很大的进步。

但下一步如果想继续提升自然度，还需要更细一层：

- segment timeline
- 可选 word / clause 对齐
- `played / cleared / interrupted / completed` 的统一时序视图

### 建议方向

在服务侧维护一个 playback timeline：

- `generated_text`
- `planned_clause`
- `synthesized_segment`
- `sent_audio`
- `started`
- `marked`
- `cleared`
- `completed`
- `heard_cursor`

### 直接收益

- interruption 后 resume 更自然
- `继续 / 后面呢 / 你刚才说到哪了` 更可信
- memory writeback 更不容易污染
- 可以开始做 “false interruption 后恢复继续说”

## 3.6 把 domain bias / alias / entity catalog 正式接进实时主链

### 当前问题

当前这部分还主要停留在研究文档。

### 建议方向

服务侧新增一个轻量运行时服务，在每个 session / turn 上动态产出：

- 当前生效的 bias phrases
- alias -> canonical entity 映射
- domain slot schema
- risk tier（低风险信息查询 / 中风险控制 / 高风险动作）

### 关键原则

- bias list 不能全局硬开
- 应按当前场景和 slot trigger 缩窄
- 对高风险实体要降低误触发率优先级

### 与当前场景的贴合点

对于智能家居 / 桌面助理，这项优化的边际收益会非常高。

## 3.7 建立“里程碑体验指标”而不是只看总时延

### 建议新增或强化的指标

输入侧：

- `speech_start_visible_ms`
- `preview_first_partial_ms`
- `preview_stable_prefix_ms`
- `endpoint_candidate_ms`
- `turn_accept_ms`

输出侧：

- `first_text_delta_ms`
- `first_audio_chunk_ms`
- `first_audible_ms`
- `duck_enter_ms`
- `hard_interrupt_cutoff_ms`

真实性侧：

- `heard_cursor_error_ms`
- `resume_anchor_accuracy`
- `false_interrupt_rate`
- `false_endpoint_rate`
- `premature_response_rate`

### 原因

很多真实体验差，并不是因为总时延太高，而是某一个里程碑非常差。

## 3.8 在当前阶段，坚持 cascade 主线，不急于切纯 speech-to-speech 主架构

### 不是说不值得研究

speech-to-speech 模型当然值得作为评测基线和未来路线关注。

### 但当前项目更现实的结论是

- 当前最缺的是 orchestration 成熟度
- 不是主路径模型形式
- 当前已有 GPU FunASR + CosyVoice + local LLM worker + shared runtime boundary
- 继续把 cascade 做强，ROI 明显更高

### 更合适的策略

- 继续把 `STT -> LLM -> TTS` 作为主路径
- 把 speech-to-speech model 当作未来 benchmark / eval baseline / 局部替换候选
- 不在当前研究阶段推翻主架构

## 4. 面向本项目的优先级排序

如果只看当前项目，服务侧优化建议我建议按下面顺序推进：

### P0：最该立刻投入

1. `MultiSignalTurnArbitrator`
2. 声学优先的 `BargeInVerifier`
3. `SpeechPlanner` clause/prosody 升级
4. playback timeline / heard cursor 精细化

### P1：紧随其后

5. 统一 `EarlyProcessingGate`
6. dynamic bias / alias / entity catalog runtime service

### P2：之后再做

7. 领域风险 gating 深化
8. speech-to-speech baseline 对照评测

## 5. 对当前仓库的明确建议

### 最建议继续强化的文件 / 模块

- `internal/voice/turn_detector.go`
- `internal/voice/barge_in.go`
- `internal/voice/speech_planner.go`
- `internal/voice/session_orchestrator.go`
- `internal/voice/asr_responder.go`
- `internal/gateway/output_flow.go`
- `internal/gateway/turn_flow.go`
- `workers/python/src/agent_server_workers/funasr_service.py`

### 当前不建议做的事

- 不建议再把更多 turn / interruption 决策回推到端侧
- 不建议为了“更先进”而立即改成全新 speech-to-speech 主协议
- 不建议在 domain bias 还没 runtime 化之前就全局强开 hotwords
- 不建议继续只靠 `silence_ms`/`hold_ms` 微调来解决 turn-taking 上限

## 6. 本轮最终判断

对于当前 `agent-server`，最值得坚持的主线是：

**以服务侧为主导，把 turn-taking、early processing、output orchestration、playback truth 做成共享 runtime 基础设施；在这个基础上，再逐步接入 domain bias、slot completeness、risk gating 与更强模型。**

这条路线同时满足：

- 架构边界清晰
- 与当前代码高度兼容
- 对实时体验 ROI 高
- 可持续演进
- 不需要立刻重写公网协议

## 7. 参考资料

### OpenAI

- OpenAI Audio Guide: https://platform.openai.com/docs/guides/audio
- OpenAI Realtime VAD Guide: https://platform.openai.com/docs/guides/realtime-vad
- Introducing the Realtime API: https://openai.com/index/introducing-the-realtime-api/
- Introducing gpt-realtime: https://openai.com/index/introducing-gpt-realtime
- Introducing next-generation audio models: https://openai.com/index/introducing-our-next-generation-audio-models/

### Google

- Improved End-of-Query Detection for Streaming Speech Recognition: https://research.google/pubs/improved-end-of-query-detection-for-streaming-speech-recognition/
- Unified End-to-End Speech Recognition and Endpointing for Fast and Efficient Speech Systems: https://research.google/pubs/unified-end-to-end-speech-recognition-and-endpointing-for-fast-and-efficient-speech-systems/
- Two-Pass End-to-End Speech Recognition: https://research.google/pubs/two-pass-end-to-end-speech-recognition/
- Low Latency Speech Recognition using End-to-End Prefetching: https://research.google/pubs/low-latency-speech-recognition-using-end-to-end-prefetching/
- DEEP CONTEXT: End-to-End Contextual Speech Recognition: https://research.google/pubs/deep-context-end-to-end-contextual-speech-recognition/
- Contextual Speech Recognition in End-to-End Neural Network Systems Using Beam Search: https://research.google/pubs/contextual-speech-recognition-in-end-to-end-neural-network-systems-using-beam-search/

### Amazon

- Towards Accurate and Real-Time End-of-Speech Estimation: https://www.amazon.science/publications/towards-accurate-and-real-time-end-of-speech-estimation
- Two-pass Endpoint Detection for Speech Recognition: https://www.amazon.science/publications/two-pass-endpoint-detection-for-speech-recognition
- Personalized Predictive ASR for Latency Reduction in Voice Assistants: https://www.amazon.science/publications/personalized-predictive-asr-for-latency-reduction-in-voice-assistants
- Multi-task RNN-T with Semantic Decoder for Streamable Spoken Language Understanding: https://www.amazon.science/publications/multi-task-rnn-t-with-semantic-decoder-for-streamable-spoken-language-understanding
- Contextual Acoustic Barge-In Classification for Spoken Dialog Systems: https://assets.amazon.science/56/4a/d81efc094934a2fdafdbe03d63f0/contextual-acoustic-barge-in-classification-for-spoken-dialog-systems.pdf
- Robust Acoustic and Semantic Contextual Biasing in Neural Transducers for Speech Recognition: https://www.amazon.science/publications/robust-acoustic-and-semantic-contextual-biasing-in-neural-transducers-for-speech-recognition
- Model-internal Slot-triggered Biasing for Domain Expansion in Neural Transducer ASR Models: https://www.amazon.science/publications/model-internal-slot-triggered-biasing-for-domain-expansion-in-neural-transducer-asr-models

### Apple

- Talking Turns Benchmark: https://machinelearning.apple.com/research/talking-turns
- Device-Directed Speech Detection for Follow-up Conversations Using Large Language Models: https://machinelearning.apple.com/research/device-directed
- STEER: Semantic Turn Extension-Expansion Recognition for Voice Assistants: https://machinelearning.apple.com/research/steer
- ChipChat: Low-Latency Cascaded Conversational Agent in MLX: https://machinelearning.apple.com/research/chipchat

### 开源 / 本地语音栈

- FunASR Official Repository: https://github.com/modelscope/FunASR
- LiveKit Turn Detection and Interruptions: https://docs.livekit.io/agents/v1/build/turn-detection
- LiveKit Turn Handling Options: https://docs.livekit.io/reference/agents/turn-handling-options/
- LiveKit Text and Transcriptions: https://docs.livekit.io/agents/voice-agent/transcriptions/
- Pipecat Smart Turn Overview: https://docs.pipecat.ai/server/utilities/smart-turn
- Pipecat STT Latency Tuning: https://docs.pipecat.ai/guides/fundamentals/stt-latency-tuning
