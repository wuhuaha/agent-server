# 2026-04-17 15:00 左右 RTOS 真机语音链路日志复盘

## 1. 复盘范围

- 时间窗口：`2026-04-17 15:00:25 ~ 15:01:53`
- 主服务：`agent-server-agentd.service`
- ASR worker：`agent-server-funasr-worker.service`
- TTS worker：`agent-server-cosyvoice-fastapi.service`
- 主会话：
  - `session_id=sess_1776409229257780085`
  - `device_id=8c:bd:37:49:a6:3c`
  - `remote_addr=218.17.73.193:8752`

本文目标不是泛泛讨论体验，而是基于真实联调日志，复盘这条会话在会话建立、上行音频、ASR、LLM、TTS、打断、播放回执、连接关闭各环节的实际表现与耗时。

## 2. 会话协商态

`15:00:29.257` 的 `gateway session.start negotiated` 明确给出本轮协商结果：

- `client_type=rtos`
- `wake_reason=keyword`
- `input_codec=pcm16le`
- `input_sample_rate_hz=16000`
- `playback_ack_enabled=true`
- `preview_events_requested=false`
- `preview_events_enabled=false`
- `server_endpoint_enabled=false`

这说明当前真机链路仍然是：

- 端侧唤醒后建连；
- 端侧负责 `end_of_speech` 后 `audio.in.commit`；
- 服务端内部虽然已经存在 preview / barge-in / playback-ack runtime，但当前公网协商上还不是“服务端 endpoint 主导”模式；
- preview 没有作为外显事件下发给端侧，更多还是服务侧内部仲裁与日志能力。

## 3. 总体结论

这一轮日志有四个非常明确的结论：

1. 首轮体验差的首因不是 LLM，而是前段输入链路：
   - 前两轮短语音都进了 ASR，但最终 `text_len=0`、`partials=0`，直接回退成“未识别到有效语音。”
2. 当前已经不是“完全没有实时能力”：
   - speaking 中的 preview / duck_only / hard_interrupt / playback_ack 都在工作；
   - 第 6 轮已经打通 `preview finalize fast path + early speaking + playback ack`。
3. 当前体感的主瓶颈仍然是首包音频慢：
   - 即便第 6 轮 LLM 首字只用了 `180ms`，真正首个音频字节仍在 `2254ms` 才到。
4. 下行链路还存在稳定性问题：
   - 端侧已经开始播放并连续上报 `playback ack mark`；
   - 但随后出现 `playback ack cleared reason=write_failed`，几秒后 websocket 以 `1005` 关闭。

换言之，这条链路已经具备“部分实时协作”雏形，但仍未达到流畅、自然、稳定的实时语音体验。

## 4. 整体时间线

### 4.1 会话建立

- `15:00:29.257`
  - `gateway session.start negotiated`
  - 会话建立完成

### 4.2 第 1 轮：短语音进入，但 ASR 完全空识别

- `15:00:31.797`
  - `gateway audio commit received`
  - `audio_bytes=40960`，约 `1.28s` 音频
  - `commit_reason=end_of_speech`
- `15:00:31.797`
  - `gateway turn accepted`
- `15:00:31.797`
  - `asr transcription stream started`
- `15:00:33.741`
  - `asr transcription stream completed`
  - `stream_elapsed_ms=1942`
  - `text_len=0`
  - `partials=0`
  - `endpoint_reason=stream_finish`
- `15:00:33.741`
  - `gateway turn first text delta`
  - `delta_text=未识别到有效语音。`
  - `first_text_delta_latency_ms=1944`
- `15:00:33.744`
  - `tts stream started`
- `15:00:35.845`
  - `gateway turn speaking`
  - `speaking_latency_ms=4048`
- `15:00:35.846`
  - `gateway turn first audio chunk`
  - `first_audio_chunk_latency_ms=4048`

配套 CosyVoice 日志：

- `15:00:33.743` 收到 `POST /inference_sft`
- `15:00:35.844` `yield speech len 1.811s, rtf 1.1588`

结论：

- 上行音频和 commit 都是正常的；
- 但这段约 `1.28s` 的短语音没有任何 partial，也没有 final text；
- 用户首轮感知会是：说完后等待近 `2s` 才收到“未识别到有效语音”，再等约 `2.1s` 才听到播报。

### 4.3 第 1 轮播报期间：用户再次开口，系统先 duck，不立即硬打断

- `15:00:35.847`
  - `gateway barge-in speech started`
- `15:00:36.004` 起连续出现：
  - `gateway barge-in soft directive applied`
  - `barge_in_policy=duck_only`
  - `barge_in_reason=duck_pending_audio_only`
  - 初始 `barge_in_audio_ms=60`
- 后续在 `80ms / 100ms / 120ms / 140ms / 160ms ...` 阶段持续维持 `duck_only`

结论：

- 系统没有把用户一开口就当作强打断；
- 这说明 `duck_only` 中间态确实在线；
- 但当时 preview 还没有成熟文本与语义证据，所以只能先压低输出，不能稳定升级为 `hard_interrupt`。

### 4.4 第 2 轮：再次短语音，仍然空识别

- `15:00:39.814`
  - `gateway audio commit received`
  - `audio_bytes=35840`，约 `1.12s`
- `15:00:39.814`
  - `gateway turn request carries previous playback context`
  - `previous_heard_boundary=prefix`
  - `previous_resume_anchor=未`
- `15:00:41.179`
  - `asr transcription stream completed`
  - `stream_elapsed_ms=1363`
  - `text_len=0`
  - `partials=0`
- `15:00:41.179`
  - `gateway turn first text delta`
  - 仍然是 `未识别到有效语音。`
- `15:00:43.461`
  - `gateway turn speaking`
  - `speaking_latency_ms=3647`

结论：

- 第二次短语音仍然没有识别出来；
- `previous_heard_*` 已经开始记录“上一轮实际听到了前缀 `未`”，说明 playback-truth / interruption 这条链路已经开始产生真实上下文，而不是只看生成全文。

### 4.5 第 3 轮：首次识别成功，但整体链路明显偏慢

- `15:00:50.835`
  - `gateway audio commit received`
  - `audio_bytes=78720`，约 `2.46s`
- `15:00:54.193`
  - `asr transcription stream completed`
  - `stream_elapsed_ms=3357`
  - `text_len=33`
  - `partials=4`
- `15:00:54.354`
  - `gateway turn first text delta`
  - `first_text_delta_latency_ms=3519`
  - 首字为 `深圳`
- `15:00:55.000`
  - `gateway turn speaking`
  - `speaking_latency_ms=4164`
- `15:00:57.895`
  - `gateway turn first audio chunk`
  - `first_audio_chunk_latency_ms=7059`
- `15:01:00.045`
  - `agent turn completed`
  - `elapsed_ms=5851`
  - `text_len=330`
  - `first_text_delta_ms=160`

结论：

- 这轮一旦 ASR 识别成功，LLM 本身其实并不慢，首字只有 `160ms`；
- 真正慢的是：
  - commit 后 ASR 花了 `3.36s`
  - speaking 后到首音频又等了近 `2.9s`
- 这说明当轮体感慢的主要责任不在 LLM，而在 `ASR endpoint/完成时机 + TTS 首包产出`。

### 4.6 第 4 轮：ASR 好一些，但首音频依旧慢

- `15:01:14.070`
  - commit，`audio_bytes=56320`，约 `1.76s`
- `15:01:16.123`
  - ASR 完成，`stream_elapsed_ms=2051`
  - `text_len=15`
- `15:01:16.306`
  - 首文本 delta，`2235ms`
- `15:01:17.049`
  - speaking，`2978ms`
- `15:01:18.207`
  - `agent turn completed`
  - `elapsed_ms=2084`
  - `first_text_delta_ms=182`
- `15:01:20.482`
  - 首音频，`6411ms`

结论：

- LLM 仍然不慢；
- `speaking -> first_audio` 又拉出约 `3.43s` 缝隙；
- 也就是说，服务端虽然已经能更早宣布“开始说”，但端侧真正听到声音仍明显滞后。

### 4.7 第 5 轮：长输入造成超长等待，随后被打断

- `15:01:34.260`
  - commit，`audio_bytes=216320`，约 `6.76s`
- `15:01:41.971`
  - ASR 完成
  - `stream_elapsed_ms=7709`
  - `text_len=27`
  - `partials=7`
- `15:01:42.140`
  - 首文本 delta，`7880ms`
- `15:01:42.826`
  - speaking，`8565ms`
- `15:01:42.827`
  - `agent turn completed`
  - `elapsed_ms=856`
  - `first_text_delta_ms=169`

结论：

- 这一轮进一步证明：长时等待的主导项不是 LLM，而是前段输入完成时机；
- `6.76s` 音频 + `7.7s` ASR stream elapsed，导致用户几乎要等 `8s` 才看到首文字；
- 对实时对话来说，这已经明显不可接受。

### 4.8 第 5 轮 speaking 期间：preview 先成熟，再升级为 hard interrupt

- `15:01:43.669`
  - `gateway barge-in accept ready`
  - `preview_audio_bytes=19200`，约 `0.60s`
  - `preview_first_partial_latency_ms=840`
  - `preview_candidate_ready_latency_ms=840`
  - `preview_accept_ready_latency_ms=840`
  - `preview_partial_text=能听到`
  - `preview_turn_stage=wait_for_more`
  - `preview_effective_wait_ms=900`
  - `preview_hold_reason=server_lexical_hold_timeout`
- 同时：
  - `gateway barge-in accepted`
  - `barge_in_policy=hard_interrupt`
  - `barge_in_reason=accepted_incomplete_after_hold`
  - `barge_in_audio_ms=600`

结论：

- speaking-time preview + barge-in 升级链路是通的；
- 系统先等了 `duck_only` / lexical hold，再在约 `600ms` 用户音频后接受强打断；
- 但 preview 首 partial 要到 `840ms` 才出现，这对于“用户打断助手”的主观体验仍偏慢。

### 4.9 第 6 轮：preview finalize fast path 生效，LLM 很快，但首音频仍受 TTS 限制

- `15:01:45.678`
  - commit
  - `preview_active=true`
  - `preview_audio_bytes=19200`
  - `preview_partial_text=能听到`
- `15:01:46.455`
  - preview ASR 完成
  - `audio_bytes=19200`
  - `stream_elapsed_ms=3627`
  - `text_len=21`
  - `partials=3`
- `15:01:46.456`
  - `gateway turn accepted`
  - `preview_finalize_fast_path=true`
  - `preview_finalize_text_len=21`
- `15:01:46.456`
  - `gateway turn request carries previous playback context`
  - `previous_heard_boundary=none`
  - `previous_missed_text=我不清楚你指的是什么情况，可以再具体说明一下吗？`
- `15:01:46.636`
  - `agent turn first text delta`
  - `elapsed_ms=180`
  - `gateway turn first text delta=可以`
- `15:01:46.838`
  - `gateway turn speaking`
  - `speaking_latency_ms=382`
  - `audio_start_source=speech_planner`
  - `audio_start_incremental=true`
- `15:01:46.840`
  - `tts stream started`
- `15:01:47.853`
  - `agent turn completed`
  - `elapsed_ms=1397`
- `15:01:48.710`
  - `gateway turn first audio chunk`
  - `first_audio_chunk_latency_ms=2254`

CosyVoice 对应：

- `15:01:46.839` 收到 TTS 请求
- `15:01:48.709` 开始 `yield speech len 0.917s, rtf 2.036`

结论：

- 这是本轮表现最好的一次：
  - preview finalize fast path 已经在工作；
  - LLM 首字 `180ms`；
  - speaking `382ms`；
  - 已经出现“边生成边说”的雏形。
- 但首音频仍要 `2254ms`，说明第一听感仍主要受 TTS 首段可播放音频生成速度约束。

### 4.10 播放回执与连接关闭

- `15:01:48.753 ~ 15:01:49.752`
  - 连续收到 `gateway playback ack mark`
  - `played_duration_ms` 从 `1` 增长到 `990`
- `15:01:49.753`
  - `gateway playback ack cleared`
  - `cleared_reason=write_failed`
- 同一时刻：
  - `gateway turn interrupted`
  - `tts synthesis failed error=context canceled`
- `15:01:52.995`
  - `gateway websocket inbound closed`
  - `ws_close_code=1005`

结论：

- 端侧实际上已经收到并播放了音频，否则不会持续上报 `played_duration_ms`；
- 但下行链路随后发生了写失败或连接异常；
- 当前日志只能看到 `write_failed` 这个归因标签，看不到更底层的 websocket 写错误原因，这是现有可观测性缺口。

## 5. 关键耗时表

| 阶段 | 时间点 | 耗时 | 说明 |
| --- | --- | --- | --- |
| session.start -> 首轮 commit | `15:00:29.257 -> 15:00:31.797` | `2.540s` | 会话建立后到端侧 `end_of_speech` commit |
| 第 1 轮 commit -> ASR 完成 | `15:00:31.797 -> 15:00:33.741` | `1.944s` | 结果为空识别 |
| 第 1 轮 commit -> 首文本 | `15:00:31.797 -> 15:00:33.741` | `1.944s` | fallback 文案 |
| 第 1 轮 commit -> 首音频 | `15:00:31.797 -> 15:00:35.846` | `4.049s` | 体感很慢 |
| 第 3 轮 commit -> ASR 完成 | `15:00:50.835 -> 15:00:54.193` | `3.358s` | 2.46s 音频 |
| 第 3 轮 commit -> 首文本 | `15:00:50.835 -> 15:00:54.354` | `3.519s` | 首字 `深圳` |
| 第 3 轮 commit -> 首音频 | `15:00:50.835 -> 15:00:57.895` | `7.060s` | 当前最差主观点之一 |
| 第 4 轮 commit -> ASR 完成 | `15:01:14.070 -> 15:01:16.123` | `2.053s` | 1.76s 音频 |
| 第 4 轮 commit -> 首音频 | `15:01:14.070 -> 15:01:20.482` | `6.412s` | 仍明显偏慢 |
| 第 5 轮 commit -> ASR 完成 | `15:01:34.260 -> 15:01:41.971` | `7.711s` | 6.76s 长输入 |
| speaking-time preview 首 partial | `15:01:42.829 -> 15:01:43.669` | `840ms` | 仍偏慢 |
| 第 6 轮 accepted -> LLM 首文本 | `15:01:46.456 -> 15:01:46.636` | `180ms` | LLM 很快 |
| 第 6 轮 accepted -> speaking | `15:01:46.456 -> 15:01:46.838` | `382ms` | planner 已提前起播 |
| 第 6 轮 accepted -> 首音频 | `15:01:46.456 -> 15:01:48.710` | `2.254s` | 仍主要受 TTS 首包限制 |

## 6. 各阶段效果判断

### 6.1 会话与协商

- 正常
- 会话建立、codec 协商、playback_ack 协商都正常
- 但当前不是服务端 endpoint 主链，仍然依赖端侧 commit

### 6.2 上行音频与 commit

- 基本正常
- 所有轮次都能看到 `audio commit received`
- `audio_bytes`、`input_frames` 也基本合理
- 问题不在“音频根本没上来”，而在“短音频识别质量与完成时机”

### 6.3 ASR

- 能工作，但质量和时延非常不稳定
- 明显存在两类问题：
  1. 短语音空识别：第 1、2 轮完全空结果
  2. 长语音等待过久：第 5 轮 `7.7s`
- 第 6 轮 preview fast path 已有价值，但 speaking-time preview 首 partial 仍偏晚

### 6.4 LLM

- 当前不是主瓶颈
- 成功轮里首 token 基本都在 `160~182ms`
- `agent turn completed` 也普遍比 ASR 与首音频更快
- 说明当前“服务端感觉慢”的主要责任不在推理大模型

### 6.5 TTS

- 是当前体感慢的主要瓶颈之一
- CosyVoice 的第一段通常要 `1.8~2.9s` 才开始吐第一批可播音频
- 即使 `speech_planner` 已经让系统更早进入 `speaking`，真正首包音频仍晚

### 6.6 打断与双工体验

- 已具备初步能力，但还不够“自然”
- 优点：
  - `duck_only` 先行，不会一开口就硬打断
  - 成熟后可升级 `hard_interrupt`
- 问题：
  - speaking-time preview partial 要到 `840ms`
  - 对用户来说，这仍然偏慢，容易感知成“我插话了但它还在说”

### 6.7 下行播放与连接稳定性

- 已证明确实有音频播到端侧
- 但随后出现：
  - `write_failed`
  - `context canceled`
  - websocket `1005`
- 这条链路当前还不能视为稳定

## 7. 本轮暴露出的主瓶颈排序

### P0. 短语音空识别

证据：

- 第 1、2 轮都存在 `text_len=0`、`partials=0`
- 输入音频并非空，分别约 `1.28s`、`1.12s`

影响：

- 这是最直接伤害体验的问题；
- 用户会误以为系统“听不见”或“卡死”。

### P0. 首音频延迟仍然过高

证据：

- 第 3 轮 `7059ms`
- 第 4 轮 `6411ms`
- 第 6 轮优化后仍是 `2254ms`

影响：

- 即使文本已经开始流式返回，用户真正听到声音还是晚；
- 这会直接破坏“像在对话”的感受。

### P0. speaking-time preview partial 太晚

证据：

- 第 5 -> 6 轮打断链路中，`preview_first_partial_latency_ms=840`

影响：

- 让用户插话时的“被理解感”仍然不足；
- `duck_only -> hard_interrupt` 的升级不够敏捷。

### P1. 长输入仍过度依赖 commit 后整段完成

证据：

- 第 5 轮 `audio_bytes=216320`，ASR `stream_elapsed_ms=7709`
- 首文本到 `7880ms`

影响：

- 当前仍然更像“说完一整句后处理”，而不是强实时口语交互。

### P1. 下行 write_failed 缺少根因日志

证据：

- 只看到 `cleared_reason=write_failed`
- 没有看到更具体的 outbound write error、peer reset、deadline exceeded 等细节

影响：

- 现在只能知道“写失败了”，但很难从日志直接区分：
  - 端侧主动关连接
  - 中间网络抖动
  - 服务端写超时
  - 某个状态流转提前 cancel

## 8. 对当前架构状态的判断

这条真实日志能说明一个重要事实：

- 当前系统已经不是“什么都没打通”的原型；
- 但它也还没有进入“成熟 realtime voice agent”阶段。

更准确地说，它处于一个中间状态：

1. 输入侧：
   - 已有流式 ASR、preview、barge-in、playback-aware context
   - 但短语音识别与 speaking-time preview 时延仍不稳定
2. 输出侧：
   - 已有 speech planner、增量 speaking、segment/playback ack
   - 但真实首音频时间仍受 TTS 首段生成强约束
3. 会话侧：
   - interruption / heard-text / previous playback context 已经串起来了
   - 但公共协商仍然是 `client commit`，不是 `server endpoint primary`

所以，这轮日志不支持“整条链路没工作”的判断；更接近：

- 主链已经通；
- 但实时性与稳定性还远没达到目标；
- 优化重点应继续落在输入短句识别、preview 提前、TTS 首音频、下行稳定性，而不是优先怀疑 LLM。

## 9. 推荐的下一轮排查/优化顺序

### 9.1 优先排查短语音空识别

建议直接对照第 1、2 轮日志与样本，重点看：

- 端侧首轮音频是否被 wake-word 残留、截断或幅度过低污染
- FunASR 流式 worker 在短 utterance 上是否因为 chunk 尺寸、finish 时机或 endpoint 触发导致没有有效解码
- 首轮短音频是否缺少 preview partial，导致 commit 后只有空 final

### 9.2 继续压 preview 首 partial 时延

目标不是先压整轮总耗时，而是先把 speaking-time 的“我已经听见你在打断我”变快：

- 争取把当前 `840ms` 压到更低；
- 让 `duck_only -> accept candidate` 的升级更早发生。

### 9.3 优先优化 TTS 首段首包

第 6 轮已经证明：

- LLM 首字 `180ms` 已足够好；
- 真正还慢的是 TTS 首包。

因此下一步应把重点放在：

- 更短的首段切分
- 更早的首段送入 TTS
- 降低 TTS 首段最短可播音频门槛

### 9.4 补齐 `write_failed` 根因日志

建议在 websocket 下行失败时补充至少以下字段：

- 实际写错误字符串
- 失败事件类型（json / binary / audio chunk / control event）
- 当前 response/segment/playback 上下文
- 是否命中写 deadline
- peer 是否已 close

否则后续端侧联调时，`write_failed` 的诊断价值仍然不够。

## 10. 复盘结论

如果只用一句话概括这轮 15:00 左右的真实联调日志：

当前链路已经打通到“可真实对话、可真实打断、可真实播放回执”，但实际体验仍主要卡在三个地方：

- 短语音 ASR 不稳，前两轮直接空识别；
- preview / turn-taking 对 speaking-time 输入的反应还不够快；
- TTS 首音频依旧慢，且下行链路在实际播放后仍出现 `write_failed` 与 websocket `1005` 关闭。

因此，下一阶段的主线应继续围绕“更快听见、更早理解、更快出声、更稳播完”推进，而不是优先怀疑 LLM 或认为整条链路尚未贯通。
