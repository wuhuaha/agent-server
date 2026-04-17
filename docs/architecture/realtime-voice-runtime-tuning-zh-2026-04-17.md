# Realtime Voice Runtime Tuning（2026-04-17）

## 背景

本轮联调聚焦 4 个直接影响端侧体验的问题：

1. 真正把 `server_endpoint.enabled=true` 与 `preview_events.enabled=true` 开出来。
2. 开始压 `preview_first_partial_latency_ms=840` 这段时延。
3. 分析 TTS 首包为什么慢。
4. 补 `write_failed` 的根因日志，便于继续联调。

本文只记录本轮已经落地或已被当前代码/运行态验证的结论，不把临时聊天分析留在对话里。

## 当前运行态确认

本轮完成后，服务侧当前 discovery 已经明确 advertize：

- `server_endpoint.enabled=true`
- `server_endpoint.main_path_candidate=true`
- `voice_collaboration.preview_events.enabled=true`
- `voice_collaboration.playback_ack.enabled=true`

本机验证命令：

```bash
curl -sf http://127.0.0.1:8080/healthz
curl -sf http://127.0.0.1:8080/v1/realtime
curl -sf http://127.0.0.1:8091/healthz
systemctl is-active agent-server-agentd.service agent-server-funasr-worker.service agent-server-cosyvoice-fastapi.service
```

本轮验证时的关键运行参数：

- `AGENT_SERVER_VOICE_SERVER_ENDPOINT_ENABLED=true`
- `AGENT_SERVER_FUNASR_STREAM_CHUNK_SIZE=0,6,3`
- `AGENT_SERVER_FUNASR_STREAM_PREVIEW_MIN_AUDIO_MS=240`
- `AGENT_SERVER_VOICE_SPEECH_PLANNER_MIN_CHUNK_RUNES=4`
- `AGENT_SERVER_VOICE_SPEECH_PLANNER_TARGET_CHUNK_RUNES=12`

注意：

- discovery 开出 preview 能力，不代表单个会话就一定收到 preview 事件。
- 单会话仍需 client 在 `session.start.capabilities.preview_events=true` 中主动请求，当前协议仍保持 discovery + capability 双向协商。

## Preview 首 partial 时延分析与优化

### 1. 原始 840ms 的主因

旧日志里 `preview_first_partial_latency_ms=840` 的根因并不在 Go 网关的 40ms 分片，而在 FunASR worker 的 online preview 配置。

worker 中 online preview 的实际块长计算为：

```python
chunk_ms = max(self.config.stream_chunk_size[1], 1) * 60
```

因此旧默认值 `stream_chunk_size=0,10,5` 实际对应：

- online preview 主块长约 `600ms`

这意味着首个 partial 至少要先攒够约 600ms 的音频，再叠加：

- HTTP push/response 往返
- online 模型本次推理时间
- 网关日志/事件发送开销

所以 `840ms` 与旧配置是相符的。

### 2. 已落地的第一步运行时压缩

本轮已把 worker 运行参数收敛到：

- `stream_chunk_size=0,6,3`
- `stream_preview_min_audio_ms=240`

对应效果：

- online preview 主块长从 `600ms` 压到 `360ms`
- preview 最小起算音频门槛从 `320ms` 压到 `240ms`

这一步的目标不是一次性做到极限，而是先把最确定、最稳定的 600ms 大块延迟砍掉。

### 3. 已落地的第二步：预热 online preview 冷启动

仅 preload 模型权重并不能消除第一轮真实用户请求的 CUDA kernel / graph 冷启动开销。

为此，本轮在 `workers/python/src/agent_server_workers/funasr_service.py` 中新增了后台预热：

- preload 完成后，主动对 online preview 路径执行一次静音 warmup
- warmup 复用真实运行时的 chunk 尺寸，而不是随便构造一个不相关的 shape
- 目标是把第一轮真实 preview partial 的 cold-start 抖动前置到服务启动期

这一步主要解决的问题是：

- 第一轮真实会话的 preview latency 明显慢于后续 warm run
- 端侧容易误以为 preview 主链本身不稳定，实际很大一部分是首轮 GPU/模型冷启动抖动

### 4. 当前结论

当前对 `preview_first_partial_latency` 的判断是：

- 第一优先级仍然是 online preview chunk 配置，而不是继续在网关层做更细碎的音频分片。
- 第二优先级是 worker 级 warmup，降低第一轮真实用户的冷启动抖动。
- 只有在这两步仍不够时，才考虑继续把 `stream_chunk_size` 从 `0,6,3` 下探到更激进的配置（例如更短 chunk），因为那会更明显地牺牲识别稳定性与吞吐。

## TTS 首包慢的分析与优化

### 1. 直接现象

本地对 CosyVoice `stream=true` 的直接首包测试表明，首包时延与首段文本长度强相关。

一组本机实测样例：

| 文本 | 首包时延（约） |
| --- | --- |
| `好` | `~923ms` |
| `好的` | `~969ms` |
| `请稍等` | `~1162ms` |
| `明天周几` | `~1512ms` |
| `好的，我来看看` | `~1473ms` |
| `好的，我来帮你看一下明天是周几` | `~2617ms` |

结论：

- CosyVoice 当前 `stream=true` 路径并不是“文本一来就极早吐第一小段音频”。
- 它更像是先把首段可播音频准备到一定程度，再一次性开始吐流。
- 因此首段文本越长，首包越慢。

### 2. 旧 speech planner 的关键问题

旧 planner 在“没有逗号/句号的流式文本”场景下会过于保守：

- 即使已经攒到了 `TargetChunkRunes`
- 只要没有 soft/strong boundary
- 它就会继续等待
- 直到最终 flush，或者攒到非常长的超限长度

这会导致两种连锁问题：

1. `gateway turn speaking` 可能已经开始，但真正的第一段 TTS 仍然没有启动。
2. 第一个送给 CosyVoice 的 clause 往往太长，进一步放大首包时延。

### 3. 本轮已落地的修复

本轮在 `internal/voice/speech_planner.go` 中补了一个更符合 realtime 语音 demo 阶段目标的策略：

- 当流式文本已经达到 `TargetChunkRunes`
- 但仍然没有逗号/句号/空格等自然边界
- 且当前还不是 finalize flush
- 则直接按 `TargetChunkRunes` 强制切出首段

这不是“完美语言学切分”，但在当前阶段是合理的 realtime trade-off：

- 优先保证尽早起播
- 避免把一整句都憋到 finalize 才送 TTS
- 避免首个 clause 过长导致 CosyVoice 首包继续放大

并新增回归测试：

- `TestSpeechPlannerForceCutsAtTargetWithoutPunctuationForEarlyAudioStart`

### 4. 当前对 TTS 首包慢的判断

当前 TTS 首包慢通常不是单点问题，而是两段叠加：

1. `speech planner` 起播太晚：首段 clause 攒得太长。
2. `CosyVoice` 本身对长 clause 的首包准备时间就更长。

所以服务侧当前最有效、最可控的优化顺序是：

1. 先缩短首段 clause，让 TTS 更早启动。
2. 再通过 `tts first audio chunk ready` 日志确认实际首包收益。
3. 如果仍然不够，再进一步评估替换或并行保留更低首包延迟的 TTS 路径。

## `write_failed` 根因日志补充

为便于继续定位端侧“写音频失败后到底听到了哪里”，本轮在 playback ACK 路径补了更完整的日志。

新增点：

- `internal/gateway/realtime_ws.go`
- `internal/gateway/voice_collaboration.go`

现在当 client 回传：

- `audio.out.cleared.reason=write_failed`

服务端会额外输出：

- `gateway playback ack write_failed observed`

并带上以下根因辅助字段：

- `response_id`
- `playback_id`
- `cleared_after_segment_id`
- `playback_total_played_ms`
- `playback_heard_text`
- `playback_segment_text`
- `playback_announced_text`
- `playback_expected_duration_ms`
- `playback_duration_before_ms`
- `playback_duration_after_ms`
- `playback_last_mark_ms`
- `session_state`
- `input_state`
- `output_state`

这组信息的目的不是单纯“看到 write_failed”，而是直接回答联调时最关键的 3 个问题：

1. 服务端原本正在播哪一段？
2. 端侧实际上已经听到了哪里？
3. 清理发生时，session 是在 speaking、return-active，还是已经转入下一轮 preview？

## 当前可直接用于联调的关键日志

### Preview/ASR

- `gateway session.start negotiated`
- `gateway input preview updated`
- `gateway input preview candidate ready`
- `gateway input preview endpoint candidate`
- `gateway input preview commit suggested`
- `gateway turn accepted`

重点字段：

- `preview_first_partial_latency_ms`
- `preview_candidate_ready_latency_ms`
- `preview_commit_suggest_latency_ms`
- `preview_partial_text`
- `preview_accept_reason`

### TTS/播报

- `tts stream started`
- `tts first audio chunk ready`
- `gateway turn first audio chunk`

重点字段：

- `text_len`
- `tts_first_chunk_latency_ms`
- `first_audio_chunk_latency_ms`

### 播放失败/中断

- `gateway playback ack cleared`
- `gateway playback ack write_failed observed`
- `gateway turn interrupted`

## 本轮代码改动

- `internal/voice/speech_planner.go`
  - 为无标点流式文本补充按 `TargetChunkRunes` 强制切段能力，尽早起播 TTS。
- `internal/voice/speech_planner_test.go`
  - 新增无标点 forced-breath 早起播回归测试。
- `workers/python/src/agent_server_workers/funasr_service.py`
  - preload 后新增 online preview 静音 warmup，降低首轮 partial 冷启动抖动。
- `internal/voice/logging_synthesizer.go`
  - 新增 `tts first audio chunk ready` 日志。
- `internal/gateway/voice_collaboration.go`
- `internal/gateway/realtime_ws.go`
  - 补齐 `write_failed` 的 playback 根因日志字段。

## 本轮验证

已完成：

```bash
go test ./internal/voice ./internal/gateway ./internal/app
python3 -m py_compile workers/python/src/agent_server_workers/funasr_service.py
```

同时验证通过：

```bash
curl -sf http://127.0.0.1:8091/healthz
curl -sf http://127.0.0.1:8080/healthz
curl -sf http://127.0.0.1:8080/v1/realtime
```

## 下一轮联调建议

1. 让端侧重新发起一轮 `session.start`，确认 `preview_events_requested=true` 与 `preview_events_enabled=true` 同时成立。
2. 重点观察新会话的：
   - `preview_first_partial_latency_ms`
   - `tts_first_chunk_latency_ms`
   - `first_audio_chunk_latency_ms`
3. 如果首轮 preview 仍显著慢于第二轮，要继续区分：
   - worker cold-start 是否已明显收敛
   - 网络/端侧送帧节奏是否仍然拖慢到达 worker 的 360ms 音频积累时间
4. 如果 TTS 首包仍然明显偏慢，要优先看：
   - `tts stream started.text_len`
   - `tts first audio chunk ready.tts_first_chunk_latency_ms`
   - 是否仍存在“planner 首段过长”或“首段直到 finalize 才开始”的情况
