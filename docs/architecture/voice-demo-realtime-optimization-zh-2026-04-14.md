# 第一阶段语音 Agent Demo 实时体验优化研究（2026-04-14）

## 目的

本文聚焦当前阶段一个非常具体的问题：

在第一阶段语音 Agent demo 中，如果目标是尽可能提升实时语音交互的`流畅性`、`自然性`、`生动性`，当前项目最该优先做什么，为什么，顺序应该怎样安排？

这里的判断基于两部分信息：

- 当前仓库实现与已有本地研究文档
- 2026-04-14 可查的公开资料与开源语音 Agent / 实时语音框架实践

本文中的优先级排序，是结合本仓库现状与外部公开资料得到的项目内推断，不是泛化到所有语音系统的统一答案。

## 核心结论

对于当前 `agent-server`，第一阶段 demo 最该补的不是更大的模型面，也不是更多接入端能力，而是把 `internal/voice` 做成更厚的 `Voice Orchestration Core`。

更具体地说，当前最值得投入的方向依次是：

1. 真正的 streaming ASR，而不是 buffered compatibility streaming
2. 更聪明的 turn detection 与 endpointing，而不是只看静音
3. 更早启动的增量 TTS，而不是等完整回复文本生成后才开口
4. 更成熟的 interruption policy，而不是“检测到用户有声就立即硬打断”
5. 更口语化、更有节奏感的 spoken response planning，而不是只追求更像人的音色

一句话总结：

第一阶段最该优化的是`说话链路的编排厚度`，不是能力广度。

## 当前仓库已经具备的有利基础

当前仓库并不是从零开始，它已经具备了几项对 demo 非常关键的底座：

- `StreamingTranscriber`、`InputPreview`、`StreamingSynthesizer` 这些共享语音契约已经存在：
  - [`internal/voice/contracts.go`](../../internal/voice/contracts.go)
- `SessionOrchestrator` 已经统一接管 hidden preview、playback、interrupt、heard-text 持久化：
  - [`internal/voice/session_orchestrator.go`](../../internal/voice/session_orchestrator.go)
- `xiaozhi` 与 native realtime 网关都已经能消费 preview 并触发服务端自动收尾：
  - [`internal/gateway/xiaozhi_ws.go`](../../internal/gateway/xiaozhi_ws.go)
  - [`internal/gateway/realtime_ws.go`](../../internal/gateway/realtime_ws.go)
- 当前共享 turn detector 已经具备“静音 + 词法保守收尾”的第一层能力：
  - [`internal/voice/turn_detector.go`](../../internal/voice/turn_detector.go)
- 当前 TTS 路径已经支持 provider-streamed 音频首包直出：
  - [`internal/voice/synthesis_audio.go`](../../internal/voice/synthesis_audio.go)
  - [`internal/voice/mimo_tts.go`](../../internal/voice/mimo_tts.go)
- 当前仓库已经开始把“用户实际听到的 assistant 内容”写回内存，而不是只记录完整回复：
  - [`internal/agent/runtime_backends.go`](../../internal/agent/runtime_backends.go)

这意味着当前项目的主问题不是“架构方向错了”，而是“语音编排层还不够厚”。

## 当前最关键的体验短板

结合当前实现，第一阶段 demo 最影响主观体验的短板主要有五个：

### 1. 默认 streaming ASR 仍然偏“假流式”

当前 `BufferedStreamingTranscriber` 只是把音频先缓存起来，最后再调用一次性识别：

- [`internal/voice/streaming_transcriber.go`](../../internal/voice/streaming_transcriber.go)

这会直接限制：

- partial hypothesis 的及时性
- endpoint hint 的质量
- 提前规划回复的能力
- 打断判断的精度

### 2. turn detection 仍然偏“静音收尾”

当前 `SilenceTurnDetector` 已经支持 lexical hold，但仍主要围绕“有 partial 之后等待一段静音再 commit”：

- [`internal/voice/turn_detector.go`](../../internal/voice/turn_detector.go)

这对 bring-up 很友好，但对自然对话仍不够：

- 容易在用户只是短暂停顿时抢答
- 容易把“然后”“还有”“我想一下”这种未完成尾巴判断过早
- 对 backchannel、语气词、补充句、改口的处理仍然不够细

### 3. TTS 启动时机偏晚

当前共享语音输出已经能消费流式 TTS，但通常仍在完整回复文本形成之后才真正启动：

- [`internal/voice/synthesis_audio.go`](../../internal/voice/synthesis_audio.go)

这会让用户感受到：

- 系统“想很久才开口”
- 首句反应慢
- 输出虽然可流式，但主观节奏仍偏回合制

### 4. interruption 还是偏“硬 cancel”

当前仓库已经有抢话、中断、heard-text reconciliation 的基础：

- [`internal/voice/session_orchestrator.go`](../../internal/voice/session_orchestrator.go)

但更像“先停掉当前播报，再进入下一轮”，还没有充分形成：

- false interruption 过滤
- backchannel 识别
- “认真插话”和“嗯/对/好”的区分
- 打断后的更平滑恢复

### 5. 缺少面向 demo 体验的 latency budget 与评测闭环

如果没有统一指标，团队很容易把“ASR 准确率提高了”误判成“体验变自然了”。

第一阶段更应该追踪的是：

- 从用户开口到首个 partial 的时间
- 从用户说完到服务端收尾的时间
- 从 commit 到首个 text delta 的时间
- 从首个稳定意群到首个音频字节的时间
- 打断后停播的时间
- false interruption 比例

## 第一阶段应该怎样提升“流畅性”

### 目标判断

对 demo 来说，`流畅性` 的关键不是全链路都追求最小延迟，而是尽快给用户“第一段有意义反馈”。

最有效的组合通常是：

- 真 streaming ASR
- 更早但不过度冒进的 endpointing
- 在第一句或第一个稳定意群出现后立刻启动 TTS
- 小帧音频与低抖动播放
- 在用户还没完全说完时就开始做轻量预判和预热

### 建议时延预算

以下数值是针对当前项目第一阶段 demo 的建议目标，不是行业统一标准：

- `speech_start_detect_ms`：`<= 150 ms`
- `first_partial_ms`：`200 - 350 ms`
- `endpoint_commit_ms`：用户真正说完后 `250 - 500 ms`
- `first_text_delta_ms`：commit 后 `200 - 400 ms`
- `first_audio_byte_ms`：第一句稳定后约 `250 ms` 起步，尽量避免拖到 `700 ms` 以后
- `barge_in_cutoff_ms`：真打断发生后约 `100 ms`

### 具体做法

- 用真正的 streaming worker 替换 buffered fallback，让 partial、endpoint hint、segment metadata 成为实时信号，而不是日志副产品
- 在 preview 稳定后提前预热轻量路径，例如意图分类、RAG 检索预热、TTS 会话准备，但真正执行仍等 turn commit
- 在 `ResponseDeltaKindText` 上增加一层内部 `Speech Planner`，按稳定意群切块启动 TTS，而不是只吃最终完整文本
- 保持公开协议冻结优先，把所有新增优化尽量沉到 `internal/voice`

## 第一阶段应该怎样提升“自然性”

### 目标判断

`自然性` 的核心不是“模型更大”，而是 turn-taking 更像真人。

用户通常会从以下行为判断系统自然不自然：

- 我只是停一下，它会不会太早接话
- 我说“嗯”“对”“等一下”的时候，它会不会反应过度
- 我打断它以后，它会不会机械地把上一段逻辑继续往后念
- 它记住的是我真正听到的内容，还是它本来想说完的完整稿子

### 具体做法

- 把 endpointing 从“静音收尾”升级到“声学 + 词法 + 语义收尾”
- 对未完成尾巴、改口、补充句增加 hold 策略
- 区分：
  - 真正用户插话
  - backchannel
  - 回声和环境噪音
  - 仅表示继续聆听的短附和音
- 继续强化 heard-text persistence，让下一轮上下文依赖“用户实际听到的 assistant 内容”

### 对当前仓库最直接的含义

最值得优先强化的不是网关，而是这些共享边界：

- [`internal/voice/asr_responder.go`](../../internal/voice/asr_responder.go)
- [`internal/voice/turn_detector.go`](../../internal/voice/turn_detector.go)
- [`internal/voice/session_orchestrator.go`](../../internal/voice/session_orchestrator.go)

## 第一阶段应该怎样提升“生动性”

### 目标判断

第一阶段的`生动性`，不应该先理解为“更复杂的情感克隆”。

对 demo 更有效的是：

- 回答更像说话，而不是像文档朗读
- 第一句先给答案，再补细节
- 有明确的停顿、节奏和轻微口语感
- 回复长度与情境匹配
- 风格能跟场景走，但不要每轮都戏剧化

### 具体做法

- 在 runtime 内增加 spoken response policy，而不是只让 LLM 自由发挥
- 把回复风格收敛成少量内部 style，如：
  - `calm`
  - `warm`
  - `urgent`
  - `playful`
- 把 style 同时映射到：
  - 首句写法
  - 句长
  - 停顿密度
  - TTS style hint / prosody
- 对工具等待、检索等待、执行等待设计极短的 spoken filler，但要低频、克制、可中断

## 最该补的前 5 个短板

以下排序是结合当前仓库基础与 demo 主观体验收益做的项目内优先级推断：

1. **真 streaming ASR**  
   这是其余所有实时优化的地基。没有持续 partial 与稳定 endpoint evidence，后续 turn taking、预热、提前 TTS 都做不厚。
2. **更强的 endpointing 与 turn detection**  
   这是“自然不自然”的第一关键。很多系统的问题不是答得不对，而是接话时机不对。
3. **增量 TTS 提前启动**  
   这是“流畅不流畅”的第一感知增强器。用户很在意系统多久开口，不只在意系统多久说完整。
4. **自适应 interruption policy**  
   这是把“能打断”升级成“打断不别扭”的关键层。
5. **spoken-style response planning**  
   这是第一阶段“生动性”的最高 ROI 做法，比先追更复杂的声音克隆更值得。

## 适合当前仓库的实施顺序

建议按下面顺序推进，尽量不改公网协议：

### D0：先建立 demo 体验基线

- 固化 latency budget
- 固化 scripted scenario
- 固化 false interruption 与 endpoint 误判统计

2026-04-15 这条主线已经补了一轮共享 debug/timing 日志，且保持公网协议不变，主要落在 `internal/gateway`：

- `input preview` 生命周期现在会记录：
  - preview 更新
  - preview 首次 partial 延迟
  - preview commit suggestion 延迟
  - preview 累计音频字节数
  - preview endpoint reason
- turn 生命周期现在会额外记录：
  - 首个 text delta 延迟
  - 首个 audio chunk 延迟
- speaking 态下的 barge-in 现在会额外记录：
  - barge-in preview 更新
  - barge-in 放行原因
  - 候选音频时长
  - 候选 partial 是否 lexically complete

这轮实现的目的不是“日志越多越好”，而是先把后续最常用的诊断问题变成可直接从日志回答的问题，例如：

- partial 是不是出来得太晚
- commit suggestion 是不是来得太早或太晚
- text delta 到了，但 audio 首包为什么慢
- barge-in 是因为完整插话被放行，还是因为不完整插话过了 hold 窗口才放行

因此，后续第一阶段实时体验优化应优先复用这批共享字段做对比分析，而不是继续在 adapter 侧临时插散乱日志。

### D1：替换默认 streaming ASR 主路径

- 让 `StreamingTranscriber` 真正输出 partial、stable partial、endpoint hint、segment
- 保持适配层不感知 provider 细节
- 让 preview 真正成为 turn-control 输入，而不是附带日志

### D2：升级 turn detection 与 interruption policy

- 从 silence-only 走向 acoustic + lexical + semantic
- 增加 incomplete-turn hold
- 增加 backchannel / false interruption 过滤

当前已落地的第一刀偏向这一层：

- shared `turn detector` 现在对独立犹豫词 / 附和音更保守，例如 `嗯`、`呃`、`那个`、`uh`、`um` 这类 partial 不再被当作可立即收尾的完整话语
- 这类短语默认继续走 lexical hold 路径，而不是走最短静音收尾路径
- 目的不是让系统更慢，而是减少“用户还没组织好下一句，系统却先抢答”的违和感

### D3：在 text delta 与 TTS 之间插入 shared speech planner

- 从“完整文本再播”升级到“稳定意群就播”
- 保持 TTS 仍归 `internal/voice` 统一管理
- 不把提前说话逻辑散落进各个 adapter

### 2026-04-14 第一轮落地结果

本轮已按 `1 -> 2 -> 3` 的顺序完成第一阶段 demo 的三项收敛：

1. **自适应 barge-in 中断门槛**
   - 新增共享 `internal/voice/barge_in.go`
   - speaking 态下不再因为首个入站音频帧就立刻打断
   - 当前策略变为：
     - 先暂存候选打断音频
     - 对 lexically complete preview 只要求基础最小时长
     - 对 `嗯 / 呃 / 那个 / uh / um` 这类不完整插话要求额外 hold
     - 如果用户在 speaking 态明确发出 `audio.in.commit`，即使插话较短，也可把已暂存音频作为一次“有意图”的打断提交
2. **增量 TTS speech planner**
   - 新增共享 `internal/voice/speech_planner.go`
   - 在 streaming text delta 与 TTS 之间增加 clause-level planner
   - 当前实现是“稳定意群预合成”而不是“协议层提前开口”：
     - 对流式文本按逗号、句号、换行和 chunk 目标长度切稳定意群
     - 在 LLM / runtime 仍在继续产出后续文本时，提前在后台合成前序稳定意群
     - 网关和协议不变，但 turn 完成后首段音频可更早就绪
3. **真实语音样本跑 `server-endpoint-preview`**
   - 为了让该场景可稳定回归，本轮还修复了一个 live-only 问题：
     - hidden preview 过去依赖 websocket read timeout 做非终态轮询
     - 一次 auto-commit 后连接会被底层 timeout 状态污染，随后客户端再发 `session.end` 时会异常断开
     - 现在改成 shared preview ticker + websocket read pump，native realtime 与 `xiaozhi` 都不再依赖 read timeout 做 preview 轮询

### 本轮真实样本验证

本轮使用了两份 2026-04-14 本机真实 WAV 样本做 `server-endpoint-preview` 验证：

- 主验证样本：`artifacts/live-baseline/20260414/samples/input-command-only.wav`
- 对比样本：`artifacts/live-baseline/20260414/samples/input-wake-command.wav`

归档结果：

- 主验证通过：
  - `artifacts/live-baseline/20260414/desktop-server-endpoint-preview-command-only-final/report.json`
- 对比样本通过：
  - `artifacts/live-baseline/20260414/desktop-server-endpoint-preview-wake-command-v1/report.json`

主验证样本的关键观察：

- hidden preview 在**没有客户端 `audio.in.commit`** 的情况下完成了 turn auto-commit
- 服务器在 turn 完成后保持连接可继续复用，客户端 `session.end` 收到正常确认
- 这轮主验证质量摘要为：
  - `thinking_latency_ms`: 约 `1281 ms`
  - `response_start_latency_ms`: 约 `2058 ms`
  - `first_text_latency_ms`: 约 `2058 ms`
  - 返回文本：`agent-server received text input: 打开客厅灯。`

对比样本的 caveat：

- `input-wake-command.wav` 这份 `小欧管家 + 打开客厅灯` 样本在当前 `FunASR + cpu` 路径下仍出现了明显识别偏差，返回文本是 `调管家。`
- 这说明当前阶段 hidden preview 的时延链路已可用，但 wake-word 前缀样本在本地 ASR 路径上的鲁棒性仍需继续补强
- 这个问题更偏向 `ASR / endpoint / speech understanding` 质量，不是本轮 websocket 生命周期或 preview 机制的问题

### 2026-04-14 主线追加对照：CPU 上的 2pass + `fsmn-vad` + KWS readiness

为了回到当前主线“提升实时语音 demo 主观体验”，本轮又补做了一个更贴近真实部署的问题验证：

- 如果把 worker 切到 `SenseVoiceSmall + paraformer-zh-streaming + fsmn-vad`
- 并保持当前 hidden `server-endpoint-preview` 场景不变
- 在这台机器的 `cpu` 路径上，它到底是更适合作为默认主链路，还是更适合作为后续 GPU / 更小 online 模型的候选

这轮对照首先暴露了一个真实阻塞：

- 过去 worker 是按需懒加载模型
- 第一次进入 `online preview` 时会把在线模型下载/加载时间压到首轮流式请求里
- 在 `agentd` 当前默认 `30s` ASR HTTP 超时下，这会直接把首轮 turn 打挂，而不是只“慢一点”

因此本轮先补了两个工程性兜底，再继续跑归档验证：

- worker 现在会在启动后后台 preload 已配置的 final / online / preview-VAD / KWS 模型，并把 `/healthz` 的 `status` 从“final 模型存在”收紧成“当前链路所需模型全部 ready”
- 本地 `run-agentd-local.sh` 现在会在 `funasr_http` 模式下等待 worker `/healthz` 到 `status=ok` 再拉起 `agentd`

归档结果：

- 2pass 命令样本：
  - `artifacts/live-baseline/20260414/desktop-server-endpoint-preview-2pass-command-only-v2/report.json`
- 2pass 唤醒词前缀样本：
  - `artifacts/live-baseline/20260414/desktop-server-endpoint-preview-2pass-wake-command-v2/report.json`
- 2pass + KWS 预热失败记录：
  - `artifacts/live-baseline/20260414/desktop-server-endpoint-preview-2pass-kws-wake-command-v1/worker-health.json`

关键观察：

- `2pass + fsmn-vad` 在命令样本上**没有提升最终文本质量**：
  - baseline：`打开客厅灯。`
  - 2pass：`打开客厅灯。`
- 但在这台机器的 `cpu` 路径上，它让时延明显变差：
  - baseline `response_start_latency_ms`：约 `2058 ms`
  - 2pass `response_start_latency_ms`：约 `3485 ms`
  - agentd 侧同轮日志记录：
    - `stream_elapsed_ms=2041`
    - `result_elapsed_ms=477`
    - `partials=3`
    - `mode=stream_2pass_online_final`
- `2pass + fsmn-vad` 在唤醒词前缀样本上也**没有修复当前识别短板**：
  - baseline：`调管家。`
  - 2pass：仍然是 `调管家。`
  - 对应 `response_start_latency_ms` 还从约 `2052 ms` 升到了约 `3572 ms`
- 这说明：
  - 2pass 架构作为后续演进方向是对的
  - 但“当前 CPU demo 默认就切 2pass”这件事并不成立
  - 当前这条链路更适合作为：
    - `GPU` 路径 benchmark
    - 更小 online 模型对照 benchmark
    - 更强 final-ASR 的第二阶段 benchmark 底座

关于 KWS，这轮还有一个比识别效果更靠前的现实问题：

- 当按当前设计直接启用 `AGENT_SERVER_FUNASR_KWS_ENABLED=true` 且沿用短模型名 `fsmn-kws` 时
- worker preload 会直接报错：
  - `fsmn-kws is not registered`
- 也就是说，在这台机器当前 FunASR `1.3.1` runtime 里：
  - `KWS` 的接口边界和开关已经接好
  - 但默认短模型名还**没有完成可运行性闭环**

因此，这轮主线结论要更务实一些：

1. **工程主线先保住 ready-to-serve**
   - preload + readiness gate 是必须项，不是锦上添花
2. **CPU demo 默认先不要切 2pass**
   - 现阶段收益没有覆盖代价
3. **2pass 先保留为内部可选路径**
   - 下一步优先去跑 `GPU` 或更小 online 模型
4. **wake-word 问题仍然是当前最该继续补的质量缺口**
   - 单靠当前 `SenseVoiceSmall + cpu` 或这轮 2pass 还不够
5. **KWS 的下一步不是先调阈值，而是先把模型 id / runtime 对齐跑通**
   - 否则它还不能进入当前阶段的主推荐组合

### D4：做 preview-driven shadow planning

- partial 足够稳定时先做预取和预热
- 真正的工具动作仍以 turn commit 为边界
- 优先减小主观等待，而不是过早执行副作用

### D5：最后再补 richer speech understanding

- 增加情绪、语气、音频事件等 speech metadata
- 把这些 metadata 用于 style、策略、路由、记忆，而不是只作为展示字段

## 补充：当前真机“无 ASR / 无音频”排障结论

2026-04-15 的一次真机日志对后续排障优先级很关键：表面现象虽然是“没看到 ASR 结果，也没听到音频”，但服务端日志已经证明这不应再被优先归因到 ASR。

同一轮链路里，服务端先后出现了：

- `asr transcription stream completed`
- `agent turn completed`
- `gateway turn response started ... modalities=text has_audio=false`
- 随后才出现 `tts stream setup failed`
- 对应错误是：`Post "http://127.0.0.1:50000/inference_sft": context canceled`

这说明至少在这类失败样本里：

- `FunASR` 已经完成识别
- agent 文本生成也已经完成
- 真正需要优先观察的是 **response downlink / TTS 启动 / websocket 生命周期**

因此，本轮补日志的目标不是“多打一层通用 debug”，而是把这个链路里的关键分叉点一次打透。当前共享网关里已经补上了以下结构化日志：

- websocket 入站终止：
  - `gateway websocket inbound closed`
  - 重点字段：`ws_close_code`、`ws_close_text`、`remote_addr`
- 首次下行响应启动：
  - `gateway response.start sent`
  - `gateway response.start write failed`
- 说话态切换：
  - `gateway speaking update sent`
  - `gateway speaking update write failed`
- 文本/音频下行失败：
  - `gateway response.chunk write failed`
  - `gateway audio binary write failed`
- turn 收尾失败：
  - `gateway active update write failed`
  - `gateway barge-in active update write failed`
  - `gateway session.end write failed`

下一次真机复现时，应优先据此判断三类根因：

1. **设备先断开**
   - 会先看到 `gateway websocket inbound closed`
   - 后续 TTS 失败更可能只是后果
2. **服务端先下行写失败**
   - 会出现 `response.start` / `speaking` / `audio binary` 的 write failed
   - 这更像 transport/downlink 层问题
3. **严格端侧把早期 `text-only response.start` 误当成“本轮无音频”**
   - 如果 `response.start sent` 成功，但后续音频迟迟不起、或端侧直接不再期待音频
   - 就要继续检查当前 streamed turn 的 modality hint 是否过早暴露为 `text`

## 当前阶段不建议优先做的事

- 不建议为了 demo 先公开修改 realtime 协议
- 不建议把 turn orchestration 重新塞回 `xiaozhi`、Web/H5、RTOS 适配层
- 不建议把 `Moshi` 一类 native speech-to-speech 路线直接作为第一阶段主路径
- 不建议先花主要精力在 voice clone、复杂人格、多 Agent 编排上
- 不建议把“说很多”误判成“更像人”；第一阶段更需要短、准、快、可打断

## 与现有本地研究的关系

本文是对下列文档的阶段化、demo 导向补充：

- [当前项目“流畅、自然、全双工”语音交互能力评估（2026-04-10）](full-duplex-voice-assessment-zh-2026-04-10.md)
- [本地 / 开源优先的全双工语音改造任务清单（2026-04-10）](local-open-source-full-duplex-roadmap-zh-2026-04-10.md)
- [语音 Agent 伙伴化研究（2026-04-04）](voice-agent-companion-research-zh-2026-04.md)

三者的关系可以概括为：

- `full-duplex assessment`：回答“当前离真正自然全双工还有多远”
- `local open-source roadmap`：回答“按本地 / 开源优先应该怎么落地改造”
- `本篇`：回答“如果当前就是第一阶段 demo，要把主观体验拉起来，最该先做什么”

## 参考资料

### 官方 / 框架资料

- OpenAI Realtime VAD guide: <https://platform.openai.com/docs/guides/realtime-vad>
- LiveKit Agents turn detection and adaptive interruption handling:
  - <https://docs.livekit.io/agents/logic/turns/>
  - <https://docs.livekit.io/agents/logic/turns/adaptive-interruption-handling/>
- Pipecat turn-management references:
  - <https://docs.pipecat.ai/api-reference/server/utilities/turn-management/interruption-strategies>
  - <https://docs.pipecat.ai/server/utilities/turn-management/user-turn-strategies>
  - <https://docs.pipecat.ai/api-reference/server/utilities/turn-management/filter-incomplete-turns>
- Home Assistant voice pipelines:
  - <https://developers.home-assistant.io/docs/voice/pipelines/>
  - <https://developers.home-assistant.io/docs/core/llm/>

### 开源模型 / 项目

- FunASR: <https://github.com/modelscope/FunASR>
- SenseVoice: <https://github.com/FunAudioLLM/SenseVoice>
- CosyVoice: <https://github.com/FunAudioLLM/CosyVoice>
- Moshi: <https://github.com/kyutai-labs/moshi>
