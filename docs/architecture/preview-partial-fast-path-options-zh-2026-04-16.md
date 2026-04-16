# Preview Partial 快路径方案复核（2026-04-16）

## 目的

本文聚焦一个非常具体的当前主线问题：

- 真实设备已经在 `agent-server` 上跑起来
- 当前端侧主观上觉得 "`说完语音后很久才看到 STT`"
- 当前最优先的优化目标不是再解释现象，而是尽快把 "`preview partial 尽早下发给端侧`" 这条链路打通

本文基于 2026-04-16 对当前日志、协议、网关、voice runtime、桌面客户端代码的交叉检查，给出：

- 当前 preview partial 被卡在哪一层
- 可以怎么做
- 哪条方案最适合当前仓库
- 推荐的分阶段落地顺序

## 一句话结论

当前项目里，`preview partial` 其实已经在 worker / voice 内部产生了，但还没有穿透到公网会话协议。

更准确地说：

- `FunASR HTTP streaming session` 已经能实时产出 `speech_start` 和 `partial`
- `ASRResponder` 已经把这些 partial 用于 `turn detector`
- `SessionOrchestrator` 已经能判断 `PartialChanged` / `CommitSuggested`
- `realtime_ws` 目前只把这些信号写进日志和内部状态，没有下发给端侧

所以当前的主要堵点不是 "`模型没有 partial`"，而是：

`preview partial 仍停留在 internal runtime signal，没有成为 client-visible signal`

## 当前链路复核

### 1. worker 层已经产生 preview partial

当前 `internal/voice/http_transcriber.go` 的 `httpStreamingSession.PushAudio(...)` 已经在每次 `/v1/asr/stream/push` 返回后：

- 发出 `TranscriptionDeltaKindSpeechStart`
- 发出 `TranscriptionDeltaKindPartial`
- 在 `Finish(...)` 时发出 `SpeechEnd`
- 最后再发 `Final`

这意味着当前 worker 本身并不是 "`只能 final，不能 partial`"。

### 2. voice 层把 partial 吃掉了，但只用作 detector 输入

当前 `internal/voice/asr_responder.go` 的 `StartInputPreview(...)` 里：

- `StreamingTranscriber.StartStream(...)` 接收了一个 `TranscriptionDeltaSink`
- 这个 sink 只做一件事：`detector.ObserveTranscriptionDelta(...)`

所以当前 partial 的去向是：

- 进入 `SilenceTurnDetector`
- 转成 `InputPreview.PartialText`
- 再变成 `CommitSuggested` / `EndpointReason` / `SpeechStarted`

但它没有继续往外走到端侧。

### 3. orchestrator 只保留 snapshot，不暴露独立 preview 事件

当前 `internal/voice/session_orchestrator.go` 的：

- `PushInputPreviewAudio(...)`
- `PollInputPreview(...)`

只返回：

- `Preview`
- `PartialChanged`
- `CommitSuggested`

这已经足够让网关知道 "`preview 变了`"，但还不足以形成一套稳定的、面向端侧的 preview 事件流。

### 4. gateway 目前只记日志，不下发 preview partial

当前 `internal/gateway/realtime_ws.go` 在 preview 更新时会记录：

- `gateway input preview updated`
- `gateway input preview speech started`
- `gateway input preview endpoint candidate`
- `gateway input preview commit suggested`

但不会向端侧发送：

- `response.chunk`
- `session.update` 附带 preview text
- 或任何专门的 preview 事件

所以端侧可见文本仍然要等：

- `audio_commit`
- turn 被 accept
- `response.start`
- `response.chunk`

这就是当前 "`说话很久后才看到 STT`" 的直接原因。

## 当前日志证明了什么

2026-04-16 的真实设备会话 `sess_1776303205420016700` 是一个很典型的例子：

- `09:33:36.300`：日志已经出现 `gateway barge-in speech started`
- `09:33:42.173`：才出现 `gateway turn accepted`
- `09:33:49.656`：端侧可见的 `gateway turn first text delta` 才出现

这说明当前存在两个独立问题：

1. preview 已经开始，但 preview 本身没有对端侧可见
2. 最终 turn 还是要等 accept + final-ASR 才能看到正式文本

因此，"`preview partial 端侧可见`" 是当前最应该优先打通的链路。

## 方案选型

下面四种方案都可以考虑，但优先级和适配性不同。

### 方案 A：把 preview partial 伪装成 `response.chunk`

做法：

- 在 preview 阶段就提前发 `response.start`
- 然后用 `response.chunk delta_type=transcription_partial` 把 partial 发给端侧

优点：

- 重用现有 `response.chunk` 渲染链路
- desktop runner 已经能识别 `speech_partial` / `input_partial` / `transcription_partial`

缺点：

- 语义不干净：`response.start` 表示 server response turn 开始，而 preview partial 实际是用户输入理解，不是 assistant response
- 当前协议文档明确要求 `response.start` 先于 `response.chunk`，如果 preview 期就发 `response.chunk`，要么违背协议，要么伪造一个 preview response
- 当前桌面 GUI 和 Web/H5 客户端对 `response.chunk` 文本基本是“直接累加到 assistant 文本区”，这样会把用户 preview transcript 和 assistant reply 混在一起

结论：

- **不推荐作为当前主路径**
- 可以作为临时 debug hack，但不适合作为当前仓库的长期协议方向

### 方案 B：在 `session.update` 上增加 preview 字段

做法：

- 保持事件名不变，仍用 `session.update`
- 当 `input_state=previewing` 时，附加例如：
  - `preview_text`
  - `preview_seq`
  - `preview_audio_bytes`
  - `preview_endpoint_reason`
  - `preview_speech_started`
  - `preview_commit_suggested`

优点：

- 不引入新 event type，对较严格的 RTOS 端更友好
- preview 本来就是输入轨状态的一部分，挂在 `session.update` 语义上成立
- 不会污染 assistant 的 `response.start / response.chunk` 流
- 当前双轨 session 模型已经有 `input_state=previewing`，协议延展方向自然

缺点：

- `session.update` 会从“状态变化事件”扩展成“状态 + 高频 preview 更新事件”
- 需要做好节流和去重，否则过于频繁
- 长期看，`session.update` 会承载过多 preview 细节

结论：

- **最适合作为当前第一阶段主推方案**
- 兼容性风险最低
- 协议扩展量最小
- 最适合先把端侧感知收益做出来

### 方案 C：新增专门的 `input.preview` 或 `transcription.preview` 事件

做法：

- 新增独立 server->client 事件，例如：
  - `input.preview`
- payload 可带：
  - `preview_id`
  - `kind=speech_start|partial|endpoint_candidate|commit_suggested`
  - `text`
  - `audio_bytes`
  - `endpoint_reason`
  - `seq`

优点：

- 协议语义最干净
- preview 和 final response 完全解耦
- 更适合未来做 richer tracing / duplex UX / 端侧字幕区

缺点：

- 需要端侧明确适配新 event type
- 需要更新 protocol docs、schema、客户端与设备解析器
- 对当前主线来说，工程面比方案 B 更重

结论：

- **长期最干净**
- 但不是当前最快见效的第一刀

### 方案 D：先不发 preview partial，只开启 `server_endpoint`

做法：

- 不把 preview 公开给端侧
- 直接启用 `server_endpoint.enabled=true`
- 让 turn 更早 auto-accept

优点：

- 服务端改动集中
- 可以缩短 final 结果出现时间

缺点：

- 解决的是“更早 final”，不是“更早 preview visible”
- 端侧依然拿不到真正的实时 partial 反馈
- 若 endpoint 误判未足够稳，会把 turn-taking 体验问题暴露得更明显

结论：

- **只能作为第二阶段配套，不应替代 preview partial 下发**

## 推荐路径

### 推荐结论

当前仓库最合适的顺序是：

1. **先走方案 B**
   - 用 `session.update + preview_* fields` 把 preview partial 安全暴露给端侧
2. **协议与端侧稳定后，再考虑演进到方案 C**
   - 把 preview 从状态更新里拆成专门事件
3. **在 preview visible 稳住后，再扩大 `server_endpoint` 默认范围**

一句话就是：

`先把 preview partial 作为输入轨状态公开，再决定是否把它升级成独立事件族。`

## 推荐落地方案

### P0：最小可用快路径

目标：

- 不动现有 `response.start / response.chunk` 语义
- 尽快让端侧看到实时 preview 文本

做法：

- 扩展 `session.update.payload`，增加可选字段：
  - `preview_text`
  - `preview_seq`
  - `preview_audio_bytes`
  - `preview_endpoint_reason`
  - `preview_commit_suggested`
- 仅在以下条件满足时发送：
  - `input_state=previewing`
  - 文本发生变化
  - 或 `speech_started / endpoint_candidate / commit_suggested` 首次出现
- 发送节流建议：
  - `min_interval_ms = 120 ~ 180`
  - 只发去重后的文本
  - 仅在文本长度达到最小阈值后对外发送，例如 `>= 2` rune

这一步的关键收益是：

- 端侧能尽早看到用户正在说什么
- 不需要等待 `audio_commit`
- 不需要伪造 `response.start`

### P1：把 preview 从“轮询快照”升级成“推式 delta”

当前 preview 还存在一个隐藏损耗：

- `httpStreamingSession.PushAudio(...)` 已经拿到了 partial
- 但 `ASRResponder.StartInputPreview(...)` 只把 partial 喂给 detector
- gateway 最后拿到的是 snapshot，而不是真正的 delta stream

建议把 preview 链路改成：

- `StartInputPreview(...)` 支持附加一个 preview sink
- sink 同时扇出到：
  - `SilenceTurnDetector`
  - `SessionOrchestrator`
  - gateway preview emitter

这样可以减少两类额外延迟：

- `InputPreviewPollInterval = 80ms` 带来的轮询损耗
- “只知道 snapshot changed，但不知道精确 partial arrival 时刻”的损耗

这一阶段的关键目标是：

- 让 preview partial 的对外下发时间尽量贴近 worker `push` 返回时刻

### P2：把 preview visible 与 server endpoint 结合

当端侧已经能稳定显示 preview partial 后，可以再推进：

- 让 `server_endpoint.enabled=true` 在特定 client / session 上灰度开启
- 当 preview 已稳定且进入 endpoint candidate 后，更早 accept turn

这样用户体验会从：

- “先看到字幕，再等系统回答”

进阶为：

- “先看到字幕，而且系统更早开始想和说”

### P3：基于 preview 做预热

当 preview 已经公开，且稳定性不错，可以进一步压缩：

- commit -> first_text_delta
- first_text_delta -> first_audio_chunk

可以做的预热包括：

- agent runtime prompt/context 预拼装
- memory recall / tool route 预热
- TTS 会话预热
- speech planner 预创建

注意：

- 这些预热应该是“可丢弃的轻量准备”
- 真正 turn accept 前，不要把它升级成正式 assistant response

## 当前不推荐的做法

### 1. 不要直接把 preview 文本塞进现有 assistant 文本区

当前桌面 GUI 和 Web/H5 都会把 `response.chunk.text` 直接当 assistant 内容追加。

如果 preview partial 复用这条路径，会造成：

- 用户自己的话和 assistant 的话混在一起
- partial / final 重复累积
- 端侧字幕和最终回复难以区分

### 2. 不要先把 `server_endpoint` 默认打开，再补 preview visible

否则用户可能会感受到：

- 系统更早截断
- 但还是没看到更早字幕

这会放大 turn-taking 的负反馈。

## 对当前仓库最值得优先修改的文件

### 第一批

- `internal/voice/asr_responder.go`
  - 让 preview partial 不只进入 detector，也能进入外部 preview sink
- `internal/voice/session_orchestrator.go`
  - 让 orchestrator 保存 preview delta / seq / last-sent state，而不是只有 snapshot changed
- `internal/gateway/realtime_ws.go`
  - 在 `session.update` 中发出 preview fields
- `clients/python-desktop-client/src/agent_server_desktop_client/app.py`
  - 单独渲染 preview transcript 区，不与 assistant response 混用
- `internal/control/webh5_assets/app.js`
  - 同步增加 preview 区与状态展示

### 第二批

- `docs/protocols/realtime-session-v0.md`
- `schemas/realtime/session-envelope.schema.json`
- `clients/python-desktop-client/src/agent_server_desktop_client/runner.py`
  - 增加 preview-first metrics

## 验证建议

至少新增以下验证：

### 单测

- preview partial 下发去重
- preview partial 节流
- `session.update` 在 `input_state=previewing` 时带 preview fields
- speaking 期间 preview 与 output 并存时不会污染 final response

### scripted validation

扩展 desktop runner：

- 新增 `preview_first_text_latency_ms`
- 新增 `preview_commit_suggest_latency_ms`
- 新增 `preview_to_accept_gap_ms`

### live 验证

至少记录两类会话：

1. `active -> preview -> commit -> final`
2. `speaking -> preview -> duck_only/backchannel -> hard_interrupt`

## 最终推荐

如果目标是：

- 先压端侧感知的 "`STT 出得慢`"
- 又不想一口气扩太多公网协议

那么当前最合适的执行顺序是：

1. `session.update` 增加 preview fields
2. preview partial 由推式 delta 驱动，而不是只靠 poll snapshot
3. 端侧单独显示 preview transcript
4. 再灰度打开 `server_endpoint`
5. 再基于 preview 做 agent / TTS 预热

换句话说，当前最值得做的不是直接“更早 final”，而是：

`先让 preview 变成端侧可见的一等信号。`
