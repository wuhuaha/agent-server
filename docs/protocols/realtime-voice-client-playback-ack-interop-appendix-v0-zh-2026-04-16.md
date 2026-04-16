# Realtime 语音协作协议联调用例附录 v0（Embedded Playback ACK + Multi-Segment）

## 文档定位

- 性质：`segment_mark_v1` 联调附录
- 目标读者：嵌入式 / RTOS client、音频播放驱动、设备联调同事
- 配套主文档：`docs/protocols/realtime-voice-client-implementation-guide-v0-zh-2026-04-16.md`
- 关注重点：多 segment `audio.out.meta`、`started/mark/cleared/completed` 时机、异常 / 重入 / clear 后继续上行

## 1. 适用范围与核心结论

- 本附录只解释当前 `playback_ack.mode=segment_mark_v1` 的实现规则，不新增 wire 字段。
- `audio.out.meta` 现在可以在同一 `response_id/playback_id` 下重复出现多次，每次对应一个服务端内部 output segment。
- `audio.out.started` / `audio.out.mark` 是 segment 级事实；`audio.out.completed` 是 playback 级终态事实。
- `audio.out.cleared` 在 v1 中表达的是“最后一个完整听到的 segment 边界”，不是“当前 segment 中间某个 sample 停住”的精确事实。
- 如果 clear 发生在当前 segment 中途，端侧应该先发一条最终 `audio.out.mark`，再用 `cleared_after_segment_id` 指向上一个完整听到的 segment。
- 如果整个 playback 在第一帧真正播出前就被清空，当前 v1 没有 `none_heard` 哨兵；端侧不要伪造 `audio.out.cleared`。

## 2. 端侧本地最小数据模型

建议把“会话状态”和“播放状态”拆成两个轻量对象，不要用一个单字段强行表示所有语义。

### 2.1 会话侧最小字段

| 字段 | 说明 | 何时更新 |
| --- | --- | --- |
| `session_id` | 当前活跃会话 ID | `session.start` 成功后 |
| `turn_id` | 当前 turn 关联 ID | 收到 `session.update` / `response.start` |
| `input_state` | 当前输入轨状态 | 收到 `session.update` |
| `output_state` | 当前输出轨状态 | 收到 `session.update` |
| `accept_reason` | 当前 turn accept 原因 | 收到 `session.update` |

### 2.2 播放侧最小字段

| 字段 | 说明 | 何时更新 |
| --- | --- | --- |
| `response_id` | 当前 playback 所属响应 | 收到首个 `audio.out.meta` |
| `playback_id` | 当前 playback 实例 | 收到首个 `audio.out.meta` |
| `latest_announced_segment_id` | 最新收到 `audio.out.meta` 的 segment | 每次收到 `audio.out.meta` |
| `currently_playing_segment_id` | 当前真正出 DAC 的 segment | 该 segment 触发本地播放开始时 |
| `last_started_segment_id` | 最近发过 `audio.out.started` 的 segment | 发送 `started` 后 |
| `last_fully_heard_segment_id` | 最近一个已完整听到的 segment | 某 segment 自然播完，或切到下一个 segment 且前一个已播完 |
| `last_mark_ms_by_segment` | 每个 segment 最近一次上报的 `played_duration_ms` | 发送 `mark` 后 |
| `terminal_ack` | `none` / `cleared` / `completed` | 发送终态 ACK 后 |
| `playback_open` | 当前 playback 是否仍可继续 ACK | 首个 `meta` 置 `true`，终态后置 `false` |

### 2.3 本地硬规则

- `latest_announced_segment_id` 不等于 `currently_playing_segment_id`；后者只能在真正播放开始时更新。
- `last_fully_heard_segment_id` 只在“某 segment 已完整播完”时更新，不能在收到 `audio.out.meta` 时提前更新。
- `terminal_ack != none` 后，该 playback 不再发送任何新的 `started/mark/cleared/completed`。
- clear 后如果还继续采音，那是“旧 playback 已终态 + 新输入继续上行”的并存状态，不是旧 playback 还在继续。

## 3. ACK 时机与归属矩阵

| ACK | 作用域 | 本地触发点 | 前置条件 | 不该发送的时机 | 去重策略 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| `audio.out.started` | 每 segment 一次 | 该 segment 第一帧真正进入 DAC / 驱动回调 | 已收到该 segment 的 `audio.out.meta`；`terminal_ack=none` | 仅收到 `audio.out.meta` 但还没播；旧 playback 已 clear/completed | `response_id+playback_id+segment_id` | 不要把“入缓冲”当作“started” |
| `audio.out.mark` | 每 segment 多次 | 周期定时器、DMA 进度、关键边界 | 对应 segment 已 started；`terminal_ack=none` | `played_duration_ms` 回退；晚于 terminal；没有已播进度 | 同 segment 只保留单调递增值 | 40~120 ms 一次即可 |
| `audio.out.cleared` | 每 playback 最多一次 | 本地 stop / clear 实际完成后 | 已知 `last_fully_heard_segment_id`；`terminal_ack=none` | playback 尚未有任何 segment 真正开始；已发 completed | `response_id+playback_id` | v1 只能表达完整 segment 边界 |
| `audio.out.completed` | 每 playback 一次 | 最后一帧真实播完 | 当前 playback 自然结束；`terminal_ack=none` | 已发 cleared；还存在未播尾部被 clear | `response_id+playback_id` | 不是每个 segment 一次 |

### 3.1 mid-segment clear 的推荐动作

当当前正在播放 `seg_0002`，但用户插话导致本地清空：

1. 先对 `seg_0002` 发送一条最终 `audio.out.mark(played_duration_ms=x)`。
2. 把 `cleared_after_segment_id` 设为 `seg_0001`，即“最后完整听到的 segment”。
3. 立即停止旧 playback 的后续 ACK，并开始新一轮上行 / preview。

这样做的含义是：

- `mark(seg_0002, x)` 给服务端一个“当前 segment 已听到了一部分”的局部事实。
- `cleared_after_segment_id=seg_0001` 保证服务端不会把 `seg_0002` 未播出的尾部误判为 heard。
- 这是 `segment_mark_v1` 下最保守也最不容易错账的实现。

### 3.2 clear-before-start 的推荐动作

如果端侧收到 `audio.out.meta` 和部分音频字节，但在第一帧真正出 DAC 前就被本地清空：

- 不发送 `audio.out.started`。
- 不发送伪造的 `audio.out.cleared`。
- 直接结束本地旧 playback，并继续上行或等待下一轮服务端状态。

原因：当前 v1 没有表达“零已听内容的清空边界”的专用字段，硬塞一个 `cleared_after_segment_id` 会造成过度记账。

## 4. 多 segment 联调用例矩阵

### 4.1 正常自然播完类

| Case | 场景 | 服务端下行 | 端侧本地动作 | 端侧上行 ACK | 联调检查点 |
| --- | --- | --- | --- | --- | --- |
| C1 | 单 segment 自然播完 | `meta(seg1)` -> bytes(seg1) | seg1 开始播，持续播到尾 | `started(seg1)` -> `mark(seg1,...)` -> `completed()` | `completed` 只发一次 |
| C2 | 双 segment 自然播完 | `meta(seg1)` -> bytes(seg1) -> `meta(seg2)` -> bytes(seg2) | seg1 播完后接 seg2 | `started(seg1)` -> `mark(seg1,...)` -> `started(seg2)` -> `mark(seg2,...)` -> `completed()` | 同一 `playback_id` 下多次 `meta`，但仍只一条 `completed` |
| C3 | 提前宣布下一个 segment | `meta(seg1)` -> bytes(seg1) -> `meta(seg2)` 提前到达 | seg1 仍在播，seg2 仅入队 | seg1 继续 `mark(seg1,...)`；直到 seg2 真播才 `started(seg2)` | `latest_announced_segment_id` 与 `currently_playing_segment_id` 不同也要工作正常 |
| C4 | 新 segment 刚切换但尚未推进 | seg1 已播完，刚收到 seg2 | seg2 尚未积累有效播放时长 | 可先发 `started(seg2)`，必要时允许 `mark(seg2,0)` | `mark=0` 只表示 seg2 还没推进，不表示整轮没播 |

### 4.2 clear / interruption 类

| Case | 场景 | 服务端下行 | 端侧本地动作 | 端侧上行 ACK | 联调检查点 |
| --- | --- | --- | --- | --- | --- |
| C5 | seg1 已播完，seg2 还未开始，本地 clear | `meta(seg1)` -> bytes(seg1) -> `meta(seg2)` -> bytes(seg2) | seg1 已完整听到；seg2 还在缓冲 | `started(seg1)` -> `mark(seg1,...)` -> `cleared(after=seg1)` | `cleared_after_segment_id` 指向 seg1 |
| C6 | seg2 播放到一半被 clear | `meta(seg1)` -> bytes(seg1) -> `meta(seg2)` -> bytes(seg2 partial) | seg2 已部分播出后被 stop | `started(seg1)` -> `mark(seg1,...)` -> `started(seg2)` -> `mark(seg2,x)` -> `cleared(after=seg1)` | `segment_mark_v1` 下不能把 seg2 直接当作 fully heard |
| C7 | 第一帧未出 DAC 前 clear | `meta(seg1)` -> bytes(seg1 queued) | 本地还没真正开播就清空 | 无 ACK，直接清本地 playback | 当前 v1 没有 `none_heard` 边界 |
| C8 | 服务端仍 speaking，但端侧本地 stop 后继续采音 | 正在播旧 response | stop / clear 旧播放后继续采音上行 | 旧 playback: `cleared(...)`；新输入：继续 `audio.in.append` | clear 后旧 playback 不再发 ACK，但新输入可继续 preview/accept |

### 4.3 异常 / 重入 / 去重类

| Case | 场景 | 端侧要求 | 预期行为 |
| --- | --- | --- | --- |
| C9 | 同一 segment 收到重复播放开始回调 | 抑制重复 `audio.out.started` | 服务端不应看到重复 started 洪泛 |
| C10 | 同一 segment 的 `played_duration_ms` 回退 | 丢弃回退 mark | 同一 segment 的 mark 必须单调 |
| C11 | 已发送 `cleared` 后，本地还有晚到播放回调 | 丢弃所有晚到 `mark/completed` | 旧 playback 已终态 |
| C12 | 已发送 `completed` 后，驱动又回调 segment done | 丢弃重复 completed | 完成 ACK 只能一条 |
| C13 | ACK 发送失败但 socket 仍在 | 异步短窗口重试，不阻塞 DAC | 播放继续，关键 ACK 尽量补发 |
| C14 | socket 已断开 | 不续发旧 playback ACK | 重连后重新 discovery + `session.start` |

## 5. 关键时序图

### 5.1 双 segment 自然播完

```text
Server -> Client: audio.out.meta(seg1)
Server -> Client: audio bytes(seg1)
Client -> Server: audio.out.started(seg1)
Client -> Server: audio.out.mark(seg1, 120)
Server -> Client: audio.out.meta(seg2)
Server -> Client: audio bytes(seg2)
Client -> Server: audio.out.started(seg2)
Client -> Server: audio.out.mark(seg2, 80)
Client -> Server: audio.out.completed(playback)
```

联调要点：

- `audio.out.completed` 只出现一次。
- `seg2` 的 `started/mark` 必须等到 seg2 真正进入播放后才发送。

### 5.2 mid-segment clear

```text
Server -> Client: audio.out.meta(seg1)
Server -> Client: audio bytes(seg1)
Client -> Server: audio.out.started(seg1)
Client -> Server: audio.out.mark(seg1, 180)
Server -> Client: audio.out.meta(seg2)
Server -> Client: audio bytes(seg2)
Client -> Server: audio.out.started(seg2)
Client -> Server: audio.out.mark(seg2, 70)
Client local: user barges in, clear playback queue
Client -> Server: audio.out.cleared(cleared_after_segment_id=seg1, reason=barge_in_clear)
Client -> Server: binary audio(new user speech)
```

联调要点：

- `mark(seg2,70)` 先发，给出 seg2 已播的局部事实。
- `cleared_after_segment_id=seg1` 表示只把 seg1 作为完整 heard 边界。
- clear 后继续上行，不代表旧 playback 还没终态。

### 5.3 clear-before-start

```text
Server -> Client: audio.out.meta(seg1)
Server -> Client: audio bytes(seg1 queued)
Client local: clear before first frame reaches DAC
Client local: drop playback context without sending cleared
Client -> Server: binary audio(new user speech) or wait for next server turn
```

联调要点：

- 不发伪造 `cleared_after_segment_id`。
- 若服务端短时间内仍未回到 active，属于当前 v1 的兼容回退窗口；不应因此阻塞端侧新一轮上行。

## 6. RTOS Client 推荐状态机

建议把端侧实现拆成“会话 lane”与“播放 lane”，不要试图用一个状态复制服务端所有内部策略。

### 6.1 会话 lane

```text
SocketClosed
  -> SocketOpenIdle
  -> SessionStarting
  -> InputStreaming
  -> TurnAccepted
  -> InputStreaming / SessionEnding
```

解释：

- `TurnAccepted` 只能由 `session.update.accept_reason` 驱动。
- `input.preview` / `input.endpoint` 只能让 UI 进入观察态，不能让业务进入 accepted 态。

### 6.2 播放 lane

```text
NoPlayback
  -> PlaybackContextReady       (receive first audio.out.meta)
  -> SegmentQueued             (segment bytes buffered but not yet started)
  -> SegmentPlaying            (send audio.out.started)
  -> SegmentPlaying            (send periodic audio.out.mark)
  -> SegmentQueued             (next audio.out.meta arrives while current segment still playing)
  -> PlaybackCleared           (send audio.out.cleared)
  -> PlaybackCompleted         (send audio.out.completed)
  -> NoPlayback
```

### 6.3 状态转移硬规则

| 规则 | 说明 |
| --- | --- |
| R1 | `PlaybackContextReady` 不等于 `SegmentPlaying`；收到 `meta` 只代表上下文建立成功 |
| R2 | `SegmentPlaying` 只能在真实播放开始后进入，并立刻触发 `audio.out.started` |
| R3 | `PlaybackCleared` 与 `PlaybackCompleted` 互斥；任何一个发生后都回到 `NoPlayback` |
| R4 | clear 后若继续采音，会话 lane 可保持 `InputStreaming`，播放 lane 已经回到 `NoPlayback` |
| R5 | 端侧不要根据本地现象自行声明 `duck_only` / `hard_interrupt`；这些是服务端策略判断 |

## 7. 联调日志建议

建议端侧日志至少打印以下字段，便于和服务端 `turn_id/trace_id` 对齐：

| 字段 | 用途 |
| --- | --- |
| `session_id` | 会话关联 |
| `turn_id` | turn 关联 |
| `trace_id` | 端到端排障 |
| `response_id` | 响应关联 |
| `playback_id` | 一次播放实例关联 |
| `segment_id` | segment 级排障 |
| `played_duration_ms` | mark / clear 精度分析 |
| `terminal_ack` | 确认是否已 clear/completed |
| `local_reason` | 本地 clear / stop / reset 原因 |

## 8. 当前 v1 明确不做的事

- 不提供 sample-accurate、字节级的播放真相协议。
- 不提供 `none_heard` 的 `audio.out.cleared` 哨兵写法。
- 不提供“当前 segment 播到一半后 clear”的单事件精确表达；当前必须靠 `mark + cleared_after_previous_segment` 组合近似。
- 不要求端侧复刻服务端的 interruption policy、endpoint policy 或 turn-taking 评分函数。

## 9. 嵌入式同事联调检查单

- 能否正确处理同一 `playback_id` 下多次 `audio.out.meta`。
- 能否区分 `latest_announced_segment_id` 与 `currently_playing_segment_id`。
- `audio.out.started` 是否基于真实出 DAC 时刻，而不是入缓冲时刻。
- `audio.out.mark` 是否对同 segment 单调不减。
- `audio.out.completed` 是否保证“一轮 playback 只发一次”。
- clear 后是否停止旧 playback ACK，并允许继续上行新输入。
- mid-segment clear 时，是否先发最终 `mark`，再用上一个完整 heard 的 segment 发 `cleared`。
