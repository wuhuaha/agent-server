# 会话建立后由服务侧主导 turn-taking 的取舍分析（2026-04-16）

## 问题

讨论的目标方案是：

- 端侧负责唤醒、建连、采音、回放、AEC/NS 等基础能力
- 一旦会话建立，turn-taking、endpointing、barge-in / interruption、何时起播等主决策尽量由服务侧统一完成
- 端侧 VAD 等能力不再作为主决策源，而是作为兜底、即时本地反射、异常回退能力

本文对比当前项目的公开主路径与上述目标方向的利弊，并结合开源项目与厂商公开资料做归纳。

## 当前项目的公开主路径

当前仓库对外仍以 `client_wakeup_client_commit` 作为兼容主路径：

- `docs/protocols/realtime-session-v0.md:119` 说明当前发现面仍以 `turn_mode = client_wakeup_client_commit` 对外发布。
- `docs/protocols/realtime-session-v0.md:122` 明确当前基线仍由 client 用 `audio.in.commit` 结束一轮输入。
- `docs/protocols/realtime-session-v0.md:127` 到 `docs/protocols/realtime-session-v0.md:138` 说明服务侧 `server_endpoint` 目前还是 additive candidate，而 preview / partial 仍主要停留在 runtime-internal 层。
- `docs/protocols/rtos-device-ws-v0.md:42` 到 `docs/protocols/rtos-device-ws-v0.md:47` 也表达了同样的兼容边界。

因此，当前项目更接近：

- 会话开始：端侧主导
- turn 最终提交：仍以端侧 commit 为兼容基线
- 服务侧：已有 preview / endpoint 候选能力，但尚未完全成为公开主导者

## 三种常见边界

### 1. 端侧主导

- 端侧负责 VAD、turn end、打断判断
- 服务侧更像被动处理已经切好的音频段

优点：

- 本地反应快
- 网络波动时更稳
- 省带宽、端到端链路简单

缺点：

- 看不到服务侧真实输出状态、ASR partial、语义上下文
- 很难做真正自然的全双工
- 端侧逻辑一旦写重，迭代成本高

### 2. 混合主导

- 端侧负责唤醒、局部 VAD、即时回放控制
- 服务侧负责 richer ASR / semantic endpoint / barge-in arbitration
- 两边都有信号，但主次边界需要清楚

优点：

- 同时兼顾低延迟本地反射和高质量服务侧判断
- 适合逐步演进和兼容旧客户端

缺点：

- 最怕“双主脑”：同一决策被两边同时拍板
- 若协议不清楚，容易出现端侧觉得该结束、服务侧觉得还没结束的冲突

### 3. 服务侧主导，端侧兜底

- 会话建立后，turn-taking / endpointing / interruption / response start 由服务侧统一决策
- 端侧只保留 wake、AEC、即时 duck、异常 fallback、手动控制等能力

优点：

- 最容易做真正统一的 turn orchestration
- 服务侧能同时看到：
  - partial / stable prefix
  - 当前 output playback 生命周期
  - 语义上下文
  - 历史 turn 状态
  - richer endpoint / barge-in signals
- 最适合做全双工、早起播、backchannel 区分、preview 驱动 prewarm
- 策略升级快，不必频繁升级 RTOS 固件
- 可观测性、A/B、线上调参更容易

缺点：

- 更依赖网络连续性和时延稳定性
- 若端侧完全不保留本地反射，会损失本地即时响应能力
- 服务侧看不到设备播放参考信号时，单靠云端更难做最早期的回声/自播音误判抑制
- 带宽和服务侧算力压力更高

## 相对当前方案，是否更有优势？

### 结论

对当前 `agent-server` 这个阶段来说：

- **是，更有优势，但前提不是“纯服务侧一刀切”，而是“服务侧拥有主决策权，端侧保留反射层和兜底层”。**

也就是说，我更赞成：

- `server-owned orchestration`
- `device-owned reflexes`

而不是：

- `server-only everything`

### 为什么相对当前方案更有优势

#### 1. 更适合真正的全双工

当前如果仍以 client commit 为主，服务侧即使已经 preview 到了用户在说什么，也很难完全自然地决定：

- 现在是 backchannel 还是 hard interrupt
- 要不要 duck
- 何时 accept 新 turn
- 何时开始 TTS 首段

而服务侧主导后，input lane 与 output lane 可以由同一编排器仲裁，更接近现代 realtime voice agent 的主路径。

#### 2. 更适合利用 richer signals

服务侧天然更容易把以下信号揉在一起：

- streaming partial
- stable prefix
- endpoint candidate
- punctuation / lexical completion
- 当前 response 是否已起播、已播到哪里
- LLM 是否已经形成首个稳定意群

端侧如果只看 VAD，很难拥有这一整套视角。

#### 3. 更容易把“打断”做得像人

真正自然的打断判断不是“有声音就停”，而是要区分：

- 嗯 / 对 / 好的 这类附和
- 想插一句但并不要求你立刻闭嘴
- 真正改口或打断

这种判断越来越依赖 preview 文本、上下文和当前回复状态，明显更适合服务侧。

#### 4. 更利于快速实验

当前项目处于研究阶段，目标是尽快迭代语音体验。若主决策在服务侧：

- 可以快速调 endpoint policy
- 可以快速改 interruption policy
- 可以快速测试 preview-based prewarm / eager TTS
- 可以快速加日志和评估指标

而无需频繁刷固件。

## 为什么又不建议“端侧完全退场”

即便服务侧拥有主决策权，端侧仍有几个不可替代的职责。

### 1. 唤醒、AEC、回放参考、音频前处理仍应主要在端侧

Apple 的 Siri 公开资料显示，唤醒前的低功耗持续监听、ring buffer、多阶段 trigger 检查等都是强端侧能力；这不是服务侧能替代的。

另外，在设备正在外放 TTS 时：

- 端侧最接近扬声器参考信号
- 端侧最适合做 AEC / NS / 本地音频门控
- 端侧最适合做最早期的“疑似用户插话”软 duck

如果这些都推迟到服务侧，容易让自播音、回声和远场噪声先污染上行。

### 2. 本地即时反射很重要

端侧即使不拍板 turn end，也仍然适合承担一些即时 UX 行为：

- 亮灯 / 波形 / “我在听”
- 本地先轻微 duck 当前播放
- 网络迟滞时先短暂停播，等待服务侧仲裁

这类动作不一定意味着端侧拥有最终决策权，但会显著提升主观灵敏度。

### 3. 异常兜底必须留在端侧

例如：

- 服务侧长时间无响应
- WebSocket 抖动
- 上行链路卡住
- 设备主动 push-to-talk 或手动 stop

此时端侧必须还能：

- 终止回放
- 强制 commit / clear
- 回退到简单半双工或手动模式

## 其他开源实践 / 厂商通常怎样做

## 总体结论

几乎没有成熟系统真的走“纯端侧 VAD 决定一切”。

更常见的是：

- **唤醒与最低功耗监听：端侧**
- **会话内 richer turn-taking / endpointing / interruption：服务侧或共享 runtime 主导**
- **端侧保留即时反射与 fallback**

### OpenAI Realtime：明显偏服务侧主导

OpenAI 官方文档显示：

- Realtime 会话默认启用 VAD
- 默认模式就是 `server_vad`
- 也支持 `semantic_vad`
- 还可以把 `create_response` / `interrupt_response` 打开或关闭
- 若要完全手动，也可以把 `turn_detection` 设为 `null`

这说明 OpenAI 的默认产品取向是：

- **服务侧主导 turn detection 是默认值**
- 但保留手动与混合控制接口

这和当前你提出的方向非常接近。

参考：

- OpenAI Realtime VAD：<https://platform.openai.com/docs/guides/realtime-vad>
- OpenAI Realtime conversations：<https://platform.openai.com/docs/guides/realtime-conversations>
- OpenAI Agents SDK Realtime：<https://openai.github.io/openai-agents-python/realtime/quickstart/>

### Gemini Live：同样明显偏服务侧主导

Google 的 Gemini Live API 官方文档显示：

- 默认支持 `automaticActivityDetection`
- 若关闭自动活动检测，则客户端才需要显式发送 `activityStart` / `activityEnd`
- 实时输入会在 end-of-turn 之前就被增量处理，以争取更快起播
- Live API 还支持在服务侧检测 interruption 并取消当前生成

这说明 Gemini Live 的默认范式与 OpenAI Realtime 很像：

- **默认由服务侧管理实时 turn 与 interruption**
- **客户端可以退化到手动模式，但那不是默认最强交互路径**

参考：

- Gemini Live API：<https://ai.google.dev/api/live>
- Gemini Live capabilities：<https://ai.google.dev/gemini-api/docs/live-api/capabilities>

### LiveKit Agents：典型混合派，但主张服务侧拥有更强 turn intelligence

LiveKit 文档把 turn handling 明确拆成：

- realtime model built-in turn detection
- VAD only
- STT endpointing
- manual turn control

尤其关键的是：

- 即便使用 STT endpointing，LiveKit 仍建议保留 VAD 来提升 interruption responsiveness
- 对 realtime model，又推荐优先使用模型提供方内建的服务侧 turn detection

这说明 LiveKit 的经验是：

- **turn 完成判断可以服务侧更强地负责**
- **但本地/附加 VAD 对中断响应仍有价值**

参考：

- LiveKit Turns overview：<https://docs.livekit.io/agents/logic/turns/>

### Pipecat：强调可插拔，但默认不是“单阈值纯 VAD”

Pipecat 文档显示：

- 默认可使用 VAD + AI-powered turn detection
- 有 local smart turn analyzer
- 对 OpenAI realtime 接入时，可选 server-side VAD，也可禁用并手动控制
- 官方明确区分 local VAD 与 server-side VAD 模式

这说明 Pipecat 站在工程框架角度的共识是：

- turn detection 往往需要比 VAD 更强的智能层
- server-side 与 local-side 可以切换，但必须明确主导者

参考：

- Pipecat User Turn Strategies：<https://docs.pipecat.ai/server/utilities/turn-management/user-turn-strategies>
- Pipecat Smart Turn：<https://docs.pipecat.ai/server/utilities/smart-turn>
- Pipecat OpenAI Realtime：<https://docs.pipecat.ai/server/services/s2s/openai>

### Vocode：偏服务端编排，提供“对话机械性拨盘”

Vocode 的公开文档强调：

- 它提供 streaming / turn-based conversation orchestration
- 有 endpointing 和 handling interruptions
- 还直接暴露 `interrupt_sensitivity`、`endpointing_sensitivity`、`conversation_speed` 这类对话拨盘

这本质上说明：

- 他们把“如何结束对方一轮话、如何响应插话”视为服务端编排能力
- 并且已经承认 backchannel / interruption 不能简单地用一个静音阈值解决

参考：

- Vocode Overview：<https://docs.vocode.dev/open-source/what-is-vocode>
- Vocode Conversational Dials：<https://docs.vocode.dev/conversational-dials>

### FunASR：更像服务侧语音能力底座，而不是完整对话决策框架

FunASR 官方仓库公开提供：

- streaming ASR（如 `paraformer-zh-streaming`）
- VAD（如 `fsmn-vad`）
- KWS（如 `fsmn_kws`）
- 2pass 实时转写能力

它非常适合作为服务侧 voice runtime 的底座，但它本身并不强规定：

- 对话谁来拍板 turn-taking
- interruption policy 怎么写
- backchannel 怎么区分

也就是说：

- FunASR 更偏“能力组件”
- 你的 turn orchestration 仍需上层自己设计

参考：

- FunASR 官方仓库：<https://github.com/modelscope/FunASR>

### Google：越来越偏“ASR-aware / semantic-aware 的服务侧 endpointing”

Google 公开论文里反复强调：

- endpointing 不该只靠外置小 endpointer
- 应该与 ASR 共享表征或直接联合建模
- partial hypothesis 可以提前触发 downstream prefetch

这说明 Google 的路线是：

- **turn end 和 response prefetch 越来越依赖服务侧 richer recognition context**
- 而不是只靠设备上的静音阈值

参考：

- Unified endpointing：<https://research.google/pubs/unified-end-to-end-speech-recognition-and-endpointing-for-fast-and-efficient-speech-systems/>
- End-to-end prefetching：<https://research.google/pubs/low-latency-speech-recognition-using-end-to-end-prefetching/>
- Project Astra：<https://deepmind.google/technologies/project-astra/>

### Amazon Alexa：典型“服务侧融合识别 + 上下文打断分类”

Amazon 公开论文显示：

- endpointing 与识别过程结合，用来区分句中停顿和句末停顿
- barge-in verification 可以作为专门分类任务做，而不是检测到用户说话就一律 hard interrupt
- 还会用 predictive ASR 做响应预取，以减少等待

这说明 Alexa 方向非常接近：

- **服务侧拥有更完整的语音与上下文信息，因此更适合拍板 endpoint 和 true interruption**
- 但不会把每次说话都机械视为中断

参考：

- Amazon endpointing：<https://www.amazon.science/publications/accurate-endpointing-with-expected-pause-duration>
- Amazon barge-in：<https://www.amazon.science/publications/contextual-acoustic-barge-in-classification-for-spoken-dialog-systems>
- Amazon predictive ASR：<https://www.amazon.science/publications/personalized-predictive-asr-for-latency-reduction-in-voice-assistants>

### Apple Siri：强端侧唤醒 + 多阶段判别，不是“端侧 VAD 决定一切”

Apple 的公开资料最值得借鉴的一点是：

- 他们的低功耗唤醒与第一阶段 trigger 是强端侧的
- 但后续还会做高精度 checker、speakerID、device-directed speech detection、ASR lattice-based false trigger mitigation
- 也就是说，真正决定“这是不是有效对话输入”的过程是多阶段的，而不是只靠一个本地阈值

这说明对于设备型语音产品：

- **端侧非常适合做低功耗持续监听与第一时间过滤**
- **但 richer 决策一定会向更高上下文的一侧演进**

参考：

- Voice Trigger System for Siri：<https://machinelearning.apple.com/research/voice-trigger>
- Talking Turns：<https://machinelearning.apple.com/research/talking-turns>
- Device-directed speech detection：<https://machinelearning.apple.com/research/device-directed>

### 新一代语音 API / STT 厂商：也明显在往服务侧语义 endpointing 走

#### AssemblyAI

AssemblyAI 的 Universal Streaming turn detection 文档明确写到：

- 使用神经网络做 end-of-turn detection
- 同时结合 semantic detection 和 acoustic detection
- acoustic VAD 作为 backup
- 客户端还可以发送 `ForceEndpoint` 强制结束当前 turn

这几乎就是“服务侧主导 + 客户端 override/fallback”的标准范式。

参考：

- AssemblyAI Turn detection：<https://www.assemblyai.com/docs/universal-streaming/turn-detection>

#### Deepgram Flux

Deepgram Flux 的 voice-agent 文档公开了：

- `EndOfTurn`
- `EagerEndOfTurn`
- `TurnResumed`

这意味着它不只是判断“结束没结束”，还显式支持：

- 先中等置信度提前触发
- 如果用户又续上，则撤回
- 再等高置信度收尾

这正是现代 server-driven 语音编排的典型特征。

参考：

- Deepgram Flux Eager EOT：<https://developers.deepgram.com/docs/flux/voice-agent-eager-eot>
- Deepgram Flux voice agent：<https://developers.deepgram.com/docs/flux/agent>

## 对当前项目最值得借鉴的边界

如果把“研究阶段 + GPU 资源充足 + RTOS 端尽量轻薄 + 目标是语音体验优先”合起来看，最值得借鉴的不是极端纯服务侧，也不是继续长期维持 client commit 主路径，而是：

### 推荐边界

#### 服务侧拥有主决策权

服务侧主导：

- speech start 进入 preview
- stable partial / preview 更新
- endpoint candidate
- turn accept
- backchannel / duck / hard interrupt 仲裁
- 何时启动首段 TTS
- 何时 cancel / truncate 已生成输出

#### 端侧保留反射层和兜底层

端侧保留：

- wake word / session open
- AEC / NS / AGC / render reference
- 本地“疑似插话”软 duck
- UI 级立刻反馈
- 手动 push-to-talk / stop / cancel
- 网络异常或服务无响应时的 commit / clear fallback

## 一句话判断

- **相对当前 `client commit` 基线，建连后由服务侧主导 turn-taking 与 interruption，整体上更先进、也更适合你的目标。**
- **但不要把端侧降成“纯麦克风采集器”；更合理的是：服务侧拥有会话决策权，端侧保留本地反射权。**

## 参考资料

- OpenAI Realtime VAD：<https://platform.openai.com/docs/guides/realtime-vad>
- OpenAI Realtime conversations：<https://platform.openai.com/docs/guides/realtime-conversations>
- LiveKit Turns overview：<https://docs.livekit.io/agents/logic/turns/>
- Pipecat User Turn Strategies：<https://docs.pipecat.ai/server/utilities/turn-management/user-turn-strategies>
- Pipecat Smart Turn：<https://docs.pipecat.ai/server/utilities/smart-turn>
- Pipecat OpenAI Realtime：<https://docs.pipecat.ai/server/services/s2s/openai>
- Vocode Overview：<https://docs.vocode.dev/open-source/what-is-vocode>
- Vocode Conversational Dials：<https://docs.vocode.dev/conversational-dials>
- FunASR：<https://github.com/modelscope/FunASR>
- Google unified endpointing：<https://research.google/pubs/unified-end-to-end-speech-recognition-and-endpointing-for-fast-and-efficient-speech-systems/>
- Google end-to-end prefetching：<https://research.google/pubs/low-latency-speech-recognition-using-end-to-end-prefetching/>
- Google DeepMind Project Astra：<https://deepmind.google/technologies/project-astra/>
- Amazon endpointing：<https://www.amazon.science/publications/accurate-endpointing-with-expected-pause-duration>
- Amazon barge-in：<https://www.amazon.science/publications/contextual-acoustic-barge-in-classification-for-spoken-dialog-systems>
- Amazon predictive ASR：<https://www.amazon.science/publications/personalized-predictive-asr-for-latency-reduction-in-voice-assistants>
- Apple Voice Trigger System for Siri：<https://machinelearning.apple.com/research/voice-trigger>
- Apple Talking Turns：<https://machinelearning.apple.com/research/talking-turns>
- AssemblyAI Turn detection：<https://www.assemblyai.com/docs/universal-streaming/turn-detection>
- Deepgram Flux voice agent：<https://developers.deepgram.com/docs/flux/agent>
- Deepgram Eager EOT：<https://developers.deepgram.com/docs/flux/voice-agent-eager-eot>
