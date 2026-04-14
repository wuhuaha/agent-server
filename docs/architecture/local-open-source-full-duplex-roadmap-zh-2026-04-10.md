# 本地 / 开源优先的全双工语音改造任务清单（2026-04-10）

## 文档目的

本文基于当前仓库实现和 [当前项目“流畅、自然、全双工”语音交互能力评估（2026-04-10）](full-duplex-voice-assessment-zh-2026-04-10.md)，给出一份以“本地 / 开源优先”为约束的可执行改造任务清单。

这里的目标不是泛泛而谈“以后支持全双工”，而是把当前项目从：

- `client_commit` 驱动的可打断流式回合制

推进到：

- 本地优先
- 开源组件为主
- 协议尽量不变
- 会话核心不推翻
- 接近自然、连续、可打断的全双工语音 Agent

## 总体原则

### 1. 保持现有中心不变

继续保持：

- `Realtime Session Core` 是中心
- `internal/voice` 是共享语音运行时
- `internal/agent` 是共享推理与技能运行时
- RTOS / Web / `xiaozhi` 兼容层只做适配，不直连模型

### 2. 本地 / 开源优先，不等于纯端到端语音模型优先

短中期更务实的路线不是直接追求纯 `speech-to-speech`，而是先把本地可控的语音编排链路做厚：

- 本地 streaming ASR
- 本地 VAD / endpointing
- 本地 turn detection
- 本地增量 TTS
- 本地 interruption / truncation / memory reconciliation

### 3. 优先补“语音编排层”，不要把问题误判成单一模型问题

当前系统离自然全双工的差距，主要不是：

- ASR 模型太差
- LLM 不够强
- TTS 音色不够像人

而是缺少一条连续工作的 `Voice Orchestration Core`。

### 4. 保持协议冻结优先

第一阶段尽量不修改以下公共面：

- `docs/protocols/realtime-session-v0.md`
- `docs/protocols/rtos-device-ws-v0.md`
- `docs/protocols/xiaozhi-compat-ws-v0.md`

允许新增服务端内部模式、内部事件、内部指标，但尽量不让 RTOS 接入方跟着改协议。

## 建议目标栈

本地 / 开源优先路线下，建议优先以“可落地、可调试、可迭代”为标准选型。

### 语音输入

- ASR 主线：
  - `FunASR`
  - `SenseVoiceSmall` 继续作为当前本地参考模型
- 第一阶段目标：
  - 先从“批式 HTTP ASR”升级到“本地 streaming ASR worker”
  - 先拿到 partial hypotheses、endpoint signals、segments

### 端点检测与打断

- 第一阶段：
  - `Silero VAD` 或 `WebRTC VAD`
  - 配合本地语音能量和静音窗口
- 第二阶段：
  - 在 partial transcript 之上增加 lexical / semantic endpointing
- 第三阶段：
  - 做 false interruption 与短附和音识别

### 语音输出

- 本地 TTS 主线建议优先验证：
  - `CosyVoice` 系列
  - 轻量 fallback 可评估 `Piper`
- 第一阶段目标：
  - 先接入一个可本地部署、可流式首包返回的 TTS
  - 不要求第一版音色最佳，但要求能增量输出

### 编排与运行时

- 不引入新的外部编排框架替换核心
- 借鉴 `LiveKit Agents` / `Pipecat` 的 turn management 思路
- 所有能力沉淀到 `internal/voice` 的共享接口后面

## 当前项目与目标之间的关键缺口

当前最关键的缺口有六个：

1. `Transcriber` 仍是一次性接口，没有 streaming ASR 通道
2. `audio.in.commit` 仍是主 turn 边界，server-side endpointing 尚未形成
3. partial / endpoint metadata 已存在，但没有 turn-control 价值
4. TTS 启动时机仍在完整回复文本之后
5. 打断是硬 cancel，没有 false interruption / resume / truncation 机制
6. memory 记录的是完整 assistant 文本，而不是用户实际听到的文本

这些缺口决定了本次改造必须先落在 `internal/voice`，其次才是 `internal/agent` 与评测链路。

## 里程碑拆分

建议把本地 / 开源优先的改造拆成 `L0` 到 `L5` 六个阶段，按从低风险到高收益排序推进。

---

## L0：建立本地全双工基线与评测框架

### 目标

在不改变公共协议的前提下，把“现在到底差在哪里”量化出来，形成后续每一步都能对比的基线。

### 执行项

1. 补充语音全双工关键指标
   - `speech_start_detect_latency_ms`
   - `speech_end_detect_latency_ms`
   - `first_partial_latency_ms`
   - `first_text_delta_latency_ms`
   - `first_audio_byte_latency_ms`
   - `barge_in_cutoff_latency_ms`
   - `playout_complete_latency_ms`
   - `false_interrupt_count`
   - `empty_asr_ratio`
   - `heard_text_ratio`

2. 扩展现有 runner / mock 工具
   - 增加连续说话、短停顿、插话、附和音、假打断场景
   - 单独归档“被打断前实际播了多少文本 / 音频”

3. 建立本地参考测试集
   - 正常问答
   - 长句命令
   - 中途停顿
   - 用户打断
   - 背景噪声
   - 沉默音频

### 主要文件

- `clients/python-desktop-client/src/agent_server_desktop_client/runner.py`
- `clients/python-desktop-client/src/agent_server_desktop_client/rtos_mock.py`
- `internal/gateway/realtime_ws.go`
- `internal/gateway/xiaozhi_ws.go`
- `internal/voice/*`
- `artifacts/live-baseline/*`

### 验收标准

- 能稳定产出一份本地 baseline 报告
- 能看出 text/audio/barge-in/timeout 之外的连续对话缺口
- 后续每个阶段都能复用同一批指标

### 建议优先级

最高。没有这一层，后续优化只能凭体感。

---

## L1：把 ASR 从“整轮转写”升级到“本地流式理解”

### 目标

把 `internal/voice` 的输入能力从：

- `commit -> Transcribe() -> final text`

升级为：

- 连续音频输入
- 持续 partial hypotheses
- endpoint hints
- final transcript

### 执行项

1. 新增流式 ASR 边界
   - 在 `internal/voice` 增加 `StreamingTranscriber`
   - 支持会话级 `Start / PushAudio / Finish / Close`

2. 改造本地 FunASR worker
   - 从单次 HTTP 转写升级为本地流式 worker
   - 输出 partial、segment、endpoint reason
   - 保留现有 HTTP path 作为 fallback

3. 在网关和 voice runtime 中接入连续上行
   - speaking 期间用户插话时，不只是缓存下一轮音频
   - 允许语音 runtime 在会话中持续接收新音频

4. 将 partial 结果变成一等事件
   - 先作为内部事件
   - 不急着外露到公开协议

### 主要文件

- `internal/voice/contracts.go`
- `internal/voice/asr_responder.go`
- `internal/voice/http_transcriber.go`
- 新增 `internal/voice/streaming_transcriber*.go`
- `internal/gateway/realtime_ws.go`
- `internal/gateway/xiaozhi_ws.go`
- `workers/python/*` 或本地 worker 脚本

### 验收标准

- 连续音频输入时能得到 partial transcript
- 本地 worker 能返回 endpoint 信号
- 现有 `audio.in.commit` 仍可兼容工作
- 现有 RTOS 与 Web 调试工具无需立刻改协议

### 风险

- FunASR 本地流式 worker 稳定性
- GPU / CPU 切换下的 worker 时序
- partial 频率过高导致网关日志或内部队列拥塞

---

## L2：引入本地 turn detection 与 server-side endpointing

### 目标

让 turn 结束不再只依赖客户端显式 commit，而是先在服务端内部形成真正可用的 endpointing 能力。

### 执行项

1. 新增 `TurnDetector`
   - 支持基于：
     - VAD
     - 静音窗口
     - partial transcript
     - 标点 / 语义边界

2. 第一版采用双层策略
   - 第一层：声学 VAD
   - 第二层：基于 partial 的 lexical endpointing

3. 保留兼容模式
   - `client_commit` 继续可用
   - 增加内部 `server_endpoint_enabled` 开关

4. 在 session core 之外实现 turn detection
   - 不把 provider 逻辑塞进 session
   - 由 `internal/voice` 输出“建议提交 turn”

5. 增加 false endpoint 保护
   - 短停顿不立刻结束
   - 连续句中逗号停顿不结束

### 主要文件

- 新增 `internal/voice/turn_detector*.go`
- `internal/voice/contracts.go`
- `internal/voice/asr_responder.go`
- `internal/gateway/realtime_ws.go`
- `internal/gateway/xiaozhi_ws.go`
- `docs/architecture/runtime-configuration.md`

### 验收标准

- 在本地连续说话场景中，不再必须手动 commit 才能触发响应
- 短停顿误收尾率显著低于当前 naive VAD
- 公开协议仍可继续声明 `client_wakeup_client_commit`，直到 server endpoint 模式成熟

### 注意

这一阶段先做“内部可用”，不急着对外改 discovery。

---

## L3：把 TTS 前移到“子句级增量播报”

### 目标

把语音输出从“等完整文本出来再播”升级到“按可播子句增量播报”。

### 执行项

1. 新增本地 `IncrementalSynthesizerScheduler`
   - 接收 LLM text delta
   - 基于子句边界、停顿、长度阈值切分待播文本

2. 接入本地流式 TTS
   - 首选验证 `CosyVoice`
   - 若首版资源占用过高，增加轻量 fallback

3. 保持统一输出接口
   - 所有本地 TTS 仍然只向网关输出 `pcm16le` 或统一流式音频边界

4. 避免“半个词就开始播”
   - 增量不等于逐 token TTS
   - 以子句或短句为最小播报单元

5. 建立 early audio 质量指标
   - 首音频时延
   - 音频中断响应时延
   - 子句切分平均长度

### 主要文件

- `internal/voice/synthesis_audio.go`
- `internal/voice/contracts.go`
- 新增 `internal/voice/incremental_tts*.go`
- 新增本地 TTS provider 文件
- `internal/gateway/realtime_ws.go`
- `internal/gateway/xiaozhi_ws.go`

### 验收标准

- LLM 回复还未完全结束时，客户端已开始收到语音
- 首音频时延明显优于当前“最终文本后再合成”
- 打断时不再总要丢掉整段尚未播出的文本

### 风险

- 本地 TTS 显存 / CPU 占用
- 子句切分不稳导致音频停顿生硬
- 长句下调度器队列堆积

---

## L4：实现自然打断、恢复与“用户已听到文本”同步

### 目标

把当前的硬 cancel 打断机制升级成更自然的 interruption runtime。

### 执行项

1. 新增 `InterruptionArbiter`
   - 区分：
     - 真打断
     - 背景噪声
     - 附和音
     - 误触发

2. 新增 `OutputTruncator`
   - 记录 assistant 原计划输出
   - 记录实际已播出的文本边界
   - 被打断时只保留用户真正听到的部分

3. 修正 memory 保存逻辑
   - 保存 `heard_assistant_text`
   - 保存 `planned_assistant_text`
   - 后续上下文默认使用前者

4. 增加 resume policy
   - 被用户短附和后是否继续播
   - 被明确新问题打断后不继续播

5. 增加打断质量评测
   - cutoff latency
   - false interruption rate
   - resume success rate

### 主要文件

- 新增 `internal/voice/interruption*.go`
- 新增 `internal/voice/output_truncation*.go`
- `internal/gateway/realtime_runtime.go`
- `internal/gateway/realtime_ws.go`
- `internal/agent/llm_executor.go`
- `internal/agent/runtime_backends.go`
- `internal/agent/contracts.go`

### 验收标准

- 打断后 memory 不再默认保存完整未播完回复
- 假打断和附和音场景下误中断率下降
- 同一轮中 assistant 被打断后，下一轮上下文更自然

---

## L5：形成“本地优先”的连续语音会话模式

### 目标

在前四阶段完成后，形成一套默认本地优先的连续语音模式，可用于 RTOS、Web、桌面共同复用。

### 执行项

1. 整理统一配置
   - `streaming_asr_provider`
   - `turn_detector_mode`
   - `local_vad_provider`
   - `local_tts_provider`
   - `interruption_mode`

2. 增加可控会话模式
   - `strict_commit`
   - `server_endpoint_preview`
   - `continuous_voice`

3. 更新工具链与调试页面
   - Web 调试页显示：
     - 当前 turn detector 状态
     - partial transcript
     - interruption decision
     - heard text

4. 增加归档基线
   - 本地 CPU 基线
   - 本地 GPU 基线
   - 连续语音对话基线

### 主要文件

- `internal/app/config.go`
- `internal/app/app.go`
- `docs/architecture/runtime-configuration.md`
- `clients/web-realtime-client/*`
- `internal/control/webh5_assets/*`
- `clients/python-desktop-client/*`

### 验收标准

- Web / RTOS / 桌面都能复用同一套本地连续语音能力
- 能在本地 GPU 机器上稳定跑出可打断连续对话
- 仍不需要让 adapter 层直连模型

---

## 推荐执行顺序

严格建议按下面顺序做，不要跳步：

1. `L0` 先把评测和基线做出来
2. `L1` 先做 streaming ASR
3. `L2` 再做 server-side endpointing
4. `L3` 再做增量 TTS
5. `L4` 最后做自然打断、resume、heard-text 同步
6. `L5` 再把它们收束成稳定模式和工具链

原因很直接：

- 没有 `L1`，`L2` 做不稳
- 没有 `L2`，`L3` 会越来越像“会说话的回合制”
- 没有 `L4`，即使能边说边听，体验仍然不自然

## 不建议的路线

### 1. 直接把现有 commit 路径继续堆 patch

会得到更多特例，不会得到真正连续语音 runtime。

### 2. 先上复杂 speech-to-speech 模型，再补可观测性

会更难定位问题，也更难比较体验收益。

### 3. 把 turn detection 做进 RTOS adapter 或 Web 页面

会破坏共享运行时边界，后续所有接入方式都会重复踩坑。

### 4. 让 channel / device adapter 直接决定何时调用 ASR、TTS、LLM

会把系统重新做回多个分散的小应用，而不是共享会话核心。

## 当前仓库里的直接落点建议

如果马上开始实施，建议先开这几类任务：

### 第一批

- `internal/voice`: 新增 `StreamingTranscriber` 契约
- `internal/voice`: 新增 `TurnDetector` 契约
- `workers/python` 或等价本地 worker：提供 streaming FunASR path
- `clients/python-desktop-client`: 新增连续说话、短停顿、打断测试场景

### 第二批

- `internal/voice`: 新增本地增量 TTS provider
- `internal/voice`: 新增 clause-level scheduler
- `internal/gateway`: 新增 interruption / truncation 可观测性

### 第三批

- `internal/agent`: 保存 `heard_assistant_text`
- `clients/web-realtime-client`: 增加 continuous-voice 调试信息面板
- `docs`: 再决定是否升级公开协议语义

## 最终判断

对当前项目来说，坚持“本地 / 开源为主”是可行的，但必须接受一个事实：

真正决定体验上限的，不只是本地模型，而是语音编排层本身。

所以这份路线图的核心不是：

- 再接一个更强的开源 ASR
- 再换一个更像人的开源 TTS

而是：

- 先把 `internal/voice` 升级成真正的 `Voice Orchestration Core`
- 再让本地 / 开源模型各自填进正确的位置

## 参考资料

- [当前项目“流畅、自然、全双工”语音交互能力评估（2026-04-10）](full-duplex-voice-assessment-zh-2026-04-10.md)
- [项目优化路线图（2026-04-04）](project-optimization-roadmap-zh-2026-04.md)
- [OpenAI Realtime VAD](https://developers.openai.com/api/docs/guides/realtime-vad)
- [Gemini Live API](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/live-api)
- [LiveKit Agents Turns](https://docs.livekit.io/agents/logic/turns/)
- [Pipecat User Turn Strategies](https://docs.pipecat.ai/api-reference/server/utilities/turn-management/user-turn-strategies)
- [Pipecat Smart Turn](https://docs.pipecat.ai/api-reference/server/utilities/turn-detection/smart-turn-overview)
