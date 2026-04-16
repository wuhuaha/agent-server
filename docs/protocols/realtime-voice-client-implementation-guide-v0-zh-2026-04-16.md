# Realtime 语音协作协议实现指引 v0（Embedded Client）

## 文档定位

- 性质：嵌入式 / RTOS client 实现手册
- 状态：v0 实施草案，可直接支撑端侧联调
- 上位文档：
  - `docs/protocols/realtime-session-v0.md`
  - `docs/protocols/rtos-device-ws-v0.md`
  - `docs/protocols/realtime-voice-client-collaboration-proposal-v0-zh-2026-04-16.md`
- 配套 schema：
  - 稳定 envelope：`schemas/realtime/session-envelope.schema.json`
  - 协作草案：`schemas/realtime/voice-collaboration-v0-draft.schema.json`

## 1. 实现总原则

- 端侧不是第二编排层；端侧负责采音、播放、UI / 灯效、本地兜底与事实回报。
- turn accept、endpoint、interrupt 主裁决在服务端。
- 所有新增能力都必须按 discovery + `session.start.capabilities` 双向协商启用。
- `accept_reason` 才是 accepted-turn 的主信号；`input.preview` / `input.endpoint` 只是观察事件。
- `audio.out.started` / `mark` / `cleared` / `completed` 是播放事实，不是策略命令。

## 2. 协商字段表

### 2.1 `GET /v1/realtime` -> `voice_collaboration`

| 字段 | 类型 | 必填 | 说明 | 端侧动作 |
| --- | --- | --- | --- | --- |
| `voice_collaboration.preview_events.enabled` | bool | 是 | 服务端是否支持 preview-aware 事件 | `true` 时才允许声明 `preview_events=true` |
| `voice_collaboration.preview_events.speech_start` | bool | 是 | 是否支持 `input.speech.start` | 决定是否做“开始说话”灯效 / UI |
| `voice_collaboration.preview_events.partial` | bool | 是 | 是否支持 `input.preview` | 决定是否实时显示 partial |
| `voice_collaboration.preview_events.endpoint_candidate` | bool | 是 | 是否支持 `input.endpoint` | 决定是否显示“即将收尾”观察态 |
| `voice_collaboration.preview_events.mode` | string | 是 | 当前预览事件版本，现为 `preview_v1` | 必须按版本兼容 |
| `voice_collaboration.playback_ack.enabled` | bool | 是 | 服务端是否接收播放事实回报 | `true` 时才允许声明 `playback_ack` |
| `voice_collaboration.playback_ack.started` | bool | 是 | 是否接收 `audio.out.started` | 用于首播事实回传 |
| `voice_collaboration.playback_ack.mark` | bool | 是 | 是否接收 `audio.out.mark` | 用于进度回传 |
| `voice_collaboration.playback_ack.cleared` | bool | 是 | 是否接收 `audio.out.cleared` | 用于打断 / 清空缓冲事实 |
| `voice_collaboration.playback_ack.completed` | bool | 是 | 是否接收 `audio.out.completed` | 用于本地播完事实 |
| `voice_collaboration.playback_ack.mode` | string | 是 | 当前播放 ACK 版本，现为 `segment_mark_v1` | 必须按版本兼容 |

### 2.2 `session.start.payload.capabilities`

| 字段 | 类型 | 必填 | 说明 | 推荐值 |
| --- | --- | --- | --- | --- |
| `text_input` | bool | 否 | 是否支持 `text.in` | `false` 或按设备能力 |
| `image_input` | bool | 否 | 是否支持 `image.in` | `false` |
| `half_duplex` | bool | 否 | 是否默认半双工 | 设备为语音 demo 时可 `false` |
| `local_wake_word` | bool | 否 | 是否端侧本地唤醒 | 按实际 |
| `preview_events` | bool | 否 | 是否愿意接收 preview-aware 事件 | 支持则 `true` |
| `playback_ack.mode` | string | 否 | 是否上报播放事实；当前仅 `segment_mark_v1` | 支持则填 `segment_mark_v1` |

### 2.3 协商判定规则

| 条件 | 结果 |
| --- | --- |
| discovery 不支持 + client 声明支持 | 不启用 |
| discovery 支持 + client 未声明 | 不启用 |
| discovery 支持 + client 正确声明 | 启用 |
| mode 未识别 | 回退到兼容基线 |

## 3. 服务端 -> 端侧字段表

### 3.1 `session.update.payload` 关键增强字段

| 字段 | 类型 | 必填 | 说明 | 端侧解释 |
| --- | --- | --- | --- | --- |
| `state` | string | 是 | 兼容主状态 | 旧逻辑继续依赖 |
| `input_state` | string | 否 | 输入轨状态 | 新 client 应优先读取 |
| `output_state` | string | 否 | 输出轨状态 | 新 client 应优先读取 |
| `turn_id` | string | 否 | 当前 turn 相关 ID | 仅用于关联，不代表 accept |
| `accept_reason` | string | 否 | turn 被接受原因，如 `audio_commit` / `server_endpoint` / `text_input` | accepted-turn 主信号 |

### 3.2 `input.speech.start`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `preview_id` | string | 是 | 当前 preview 观察窗口 ID |
| `audio_offset_ms` | int | 是 | 截至检测到 speech start 的输入音频偏移 |
| `source` | string | 是 | 当前固定为 `server_preview` |

端侧建议动作：

- 立刻更新 listening cue / 灯效
- 不要因此自动停录或自动 commit

### 3.3 `input.preview`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `preview_id` | string | 是 | preview 观察窗口 ID |
| `text` | string | 是 | 当前 partial 文本 |
| `stable_prefix` | string | 否 | 当前稳定前缀；服务端有收敛证据时应尽量填真实稳定前缀，而不是机械复制 `text` |
| `is_final` | bool | 是 | 是否最终结果；当前阶段通常为 `false` |
| `stability` | float | 否 | 0~1 稳定度提示；仅供显示/调试，不能替代 accepted-turn 判断 |
| `audio_offset_ms` | int | 是 | 该 partial 对应的累计音频位置 |

端侧建议动作：

- 用于字幕 / 屏显 / 调试
- 不要把它当成 accepted-turn
- 即便 `stable_prefix` 很长或 `stability` 很高，也继续等待 `accept_reason`

### 3.4 `input.endpoint`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `preview_id` | string | 是 | preview 观察窗口 ID |
| `candidate` | bool | 是 | 当前是否为 endpoint candidate |
| `reason` | string | 是 | endpoint 候选原因，如 `preview_tail_silence` / `server_silence_timeout` |
| `audio_offset_ms` | int | 是 | 候选出现时的累计音频位置 |

端侧建议动作：

- 可以做“即将收尾”的轻提示
- 不要把它当成 accepted-turn 或 stop-recording 指令

### 3.5 `response.start`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `response_id` | string | 是 | 响应流 ID |
| `modalities` | array | 否 | 当前响应模态 |
| `turn_id` | string | 否 | 当前服务端 turn ID |
| `trace_id` | string | 否 | 链路追踪 ID |

### 3.6 `audio.out.meta`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `response_id` | string | 是 | 所属响应流 ID |
| `playback_id` | string | 是 | 本次播放实例 ID |
| `segment_id` | string | 是 | 当前音频片段 ID；初版通常一条响应一个 segment |
| `text` | string | 否 | 对应文本片段，便于端侧对齐字幕 |
| `expected_duration_ms` | int | 是 | 期望播放时长，便于端侧安排 mark / watchdog |
| `is_last_segment` | bool | 是 | 是否最后一段 |

端侧建议动作：

- 收到 `audio.out.meta` 后建立播放上下文
- 之后所有 ACK 事件必须带回同一组 `response_id/playback_id/segment_id`

## 4. 端侧 -> 服务端字段表

### 4.1 `audio.out.started`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `response_id` | string | 是 | 对应 `audio.out.meta.response_id` |
| `playback_id` | string | 是 | 对应 `audio.out.meta.playback_id` |
| `segment_id` | string | 是 | 对应 `audio.out.meta.segment_id` |

发送时机：音频真正开始从 DAC / 播放驱动播出时。

### 4.2 `audio.out.mark`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `response_id` | string | 是 | 对应 `audio.out.meta.response_id` |
| `playback_id` | string | 是 | 对应 `audio.out.meta.playback_id` |
| `segment_id` | string | 是 | 对应 `audio.out.meta.segment_id` |
| `played_duration_ms` | int | 否 | 当前已真实播放时长 |

发送时机：

- 建议按 40~120 ms 粗粒度上报
- 资源紧张设备可只在关键节点上报一到数次

### 4.3 `audio.out.cleared`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `response_id` | string | 是 | 对应 `audio.out.meta.response_id` |
| `playback_id` | string | 是 | 对应 `audio.out.meta.playback_id` |
| `cleared_after_segment_id` | string | 是 | 当前被清空到哪个 segment |
| `reason` | string | 是 | 清空原因，如 `barge_in_clear` / `session_end_clear` / `device_reset` |

发送时机：

- 本地清空未播缓冲时
- 不要求一定意味着已 hard interrupt，但一定代表“后续内容没被播出来”

当前服务端行为补充：

- 若服务端仍在 `speaking`，`audio.out.cleared` 可能触发服务端立刻停止当前输出并回到 `active`
- 因此该事件不只是“日志事实”，也会参与服务端 playback truth 收尾

### 4.4 `audio.out.completed`

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `response_id` | string | 是 | 对应 `audio.out.meta.response_id` |
| `playback_id` | string | 是 | 对应 `audio.out.meta.playback_id` |

发送时机：最后一个样本真实播完时。

## 5. ACK 时机表

| 事件 | 推荐触发点 | 强制性 | 备注 |
| --- | --- | --- | --- |
| `audio.out.started` | 第一帧真正开始播放 | 强烈建议 | 用于首播事实 |
| `audio.out.mark` | 每 40~120 ms 或关键进度点 | 可选但建议 | 低资源设备可降频 |
| `audio.out.cleared` | 本地缓冲被清空后立即发送 | 条件触发 | 与 interrupt / clear 动作配合 |
| `audio.out.completed` | 最后一帧真实播放完 | 强烈建议 | 用于最终听到边界 |

补充语义：

- 当已协商 `playback_ack` 时，服务端可能不会在音频字节发送完的瞬间立刻回到 `session.update(state=active)`。
- 更准确的行为是：服务端优先等待 `audio.out.completed` 或 `audio.out.cleared`，再收口本轮 playback truth；若短时间内未等到，则回退到服务端启发式完成。
- 因此端侧若希望 turn 收尾、heard-text、resume 更自然，`audio.out.completed` 应尽量及时发送。

## 6. 错误码与重试策略

### 6.1 连接 / 会话类

| 错误码 | 含义 | 端侧处理 |
| --- | --- | --- |
| `invalid_json` | 控制帧 JSON 非法 | 修正本地编码问题，不自动重发同帧 |
| `unsupported_event` | 当前事件不支持 | 降级，停止发送该类扩展事件 |
| `session_not_started` | 会话尚未开始 | 先重建或重新发送 `session.start` |
| `session_already_active` | 重复开会话 | 清理本地状态，避免重复 start |
| `unsupported_session_start` | `session.start` 参数不被接受 | 回退参数，重新建连或重新 start |
| `turn_not_ready` | 当前状态不允许 `audio.in.commit` | 等待 `active` 或按服务端 turn-taking 继续 |

### 6.2 协作扩展类

| 场景 | 建议策略 |
| --- | --- |
| 服务端 discovery 未声明 `voice_collaboration.preview_events.enabled=true` | 不发送 `preview_events=true`，不等待 `input.*` |
| 服务端 discovery 未声明 `voice_collaboration.playback_ack.enabled=true` | 不发送 `playback_ack`，也不发送 `audio.out.* ACK` |
| 服务端返回未知 `accept_reason` | 仅做透传日志，不应判错 |
| 服务端返回未知 `input.endpoint.reason` | 仅做透传日志，不应判错 |
| 发送 ACK 后收到 `unsupported_event` | 关闭 ACK 扩展，回退兼容模式 |
| ACK 发送失败但 socket 仍在 | 不阻塞播放；下一关键 ACK 继续尝试 |
| socket 断开 | 重连后从 discovery + `session.start` 重新协商，不续发旧 playback ACK |

## 7. 重试策略表

| 场景 | 是否立即重试 | 上限 | 说明 |
| --- | --- | --- | --- |
| `session.start` 前的 HTTP discovery 失败 | 是 | 退避重试 | 端侧可指数退避 |
| `session.start` 被拒绝 | 否 | 0 | 先按错误码修正参数 |
| `audio.out.mark` 单次发送失败 | 是 | 1~2 次 | 不应阻塞播放线程 |
| `audio.out.completed` 发送失败 | 是 | 2~3 次 | 可异步重试短窗口 |
| `audio.out.cleared` 发送失败 | 是 | 1~2 次 | 如果 socket 已断则放弃 |
| unknown server event | 否 | 0 | 忽略即可 |

## 8. Embedded Client 最小实现清单

- 建 discovery 解析：读取 `server_endpoint` 与 `voice_collaboration`
- 建会话协商：`session.start.capabilities` 能声明 `preview_events` 与 `playback_ack.mode`
- 建 accepted-turn 判断：以 `session.update.accept_reason` 为准
- 建 preview 显示：能处理 `input.speech.start` / `input.preview` / `input.endpoint`
- 建播放上下文：收到 `audio.out.meta` 后缓存 `response_id/playback_id/segment_id`
- 建 ACK 上报：至少支持 `audio.out.started` 与 `audio.out.completed`
- 建降级路径：协商失败或扩展不支持时，仍能按兼容 v0 运行
- 建日志：打印 `turn_id/trace_id/preview_id/response_id/playback_id`

## 9. 推荐最小落地顺序

1. 先打通 discovery + `session.start` 能力协商
2. 再打通 `accept_reason` 与 `input.preview` 的 UI / 日志
3. 再接 `audio.out.meta` + `audio.out.started/completed`
4. 最后再补 `audio.out.mark` / `audio.out.cleared` 与更细腻的播放事实

## 10. 与 schema 的对应关系

| 文档对象 | 对应 schema |
| --- | --- |
| 通用 envelope | `schemas/realtime/session-envelope.schema.json` |
| preview / playback 协作扩展 | `schemas/realtime/voice-collaboration-v0-draft.schema.json` |
| `session.start` payload | `schemas/realtime/device-session-start.schema.json` |

## 11. 当前阶段的实现边界说明

- 当前服务端已经开始在 native realtime 路径上公开 capability-gated 的 `input.speech.start` / `input.preview` / `input.endpoint` 与 `audio.out.meta` / 播放 ACK 接口。
- 当前 `audio.out.* ACK` 首先作为“播放事实回传入口 + 观测日志”落地；后续再进一步把它接入更精确的 heard-text / resume 策略。
- 当前端侧仍应保留 local VAD / 本地 stop 作为兜底，但会话建立后的主裁决以服务端为准。
