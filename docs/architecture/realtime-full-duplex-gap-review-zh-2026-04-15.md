# 当前 realtime 全双工差距复核（2026-04-15）

## 目的

本文用于复核端侧给出的 `S1` 到 `S4` 四条建议，判断它们与当前仓库实际状态的吻合度，并明确：

- 当前实时性差、主观不够流畅的主要结构性原因是什么
- 本项目现在到底处于“哪一步”，不要高估也不要低估
- 下一阶段最该优先补的是哪一层

本文结论基于 2026-04-15 对当前代码、测试和已有架构文档的交叉检查，不是对未来目标态的想象性描述。

## 本次复核覆盖

本次重点复核了以下实现与边界：

- `internal/app/config_voice.go`
- `internal/session/realtime_session.go`
- `internal/session/types.go`
- `internal/gateway/realtime_ws.go`
- `internal/gateway/xiaozhi_ws.go`
- `internal/gateway/output_flow.go`
- `internal/gateway/turn_flow.go`
- `internal/voice/turn_detector.go`
- `internal/voice/asr_responder.go`
- `internal/voice/speech_planner.go`
- `internal/voice/synthesis_audio.go`
- `internal/voice/barge_in.go`
- `internal/voice/session_orchestrator.go`

同时复跑了端侧建议中的验证命令：

```bash
go test ./internal/gateway ./internal/voice
go test ./internal/session ./internal/gateway
go test ./internal/voice ./internal/gateway -run 'BargeIn|SessionOrchestrator|Realtime'
```

当前三组测试均通过。

## 一句话结论

端侧对方向的判断基本是对的。

当前仓库已经不再是“纯半双工原型”，而是：

- 已具备 `server endpoint` 候选能力
- 已具备 preview 驱动的 auto-commit 基础
- 已具备早于最终全文收口的增量 TTS 规划能力
- 已具备 barge-in 与 heard-text persistence 的基础

但它仍然不是严格意义上的“真全双工语音会话”。

当前更准确的描述应该是：

`较强的 half-duplex + adaptive interruption + partial early TTS`，
而不是 `true full-duplex session orchestration`。

## 完成度判断

结合当前代码现状，可将端侧四项建议粗略映射为：

| 建议 | 当前状态 | 粗略完成度 | 结论 |
| --- | --- | --- | --- |
| `S1` server endpoint 升级为主路径候选 | 已有较完整候选实现 | `~70%` | 作为候选主路径成立，但还不是最终结构中心 |
| `S2` session core 升级为输入/输出双轨 | 基本未完成 | `~20%` | 当前最核心的结构性短板 |
| `S3` 边生成边说 | 已有实质进展 | `~55% - 65%` | 已明显改善，但还未完全解除生命周期耦合 |
| `S4` interruption 多策略仲裁 | 有硬打断和 heard-text 基础 | `~45% - 55%` | 还没有真正的多策略裁决层 |

这意味着：

- `S1` 不是“还没开始”，而是“已经走到候选主路径阶段”
- `S3` 不是“完全没有”，而是“已经有一半以上的关键骨架”
- `S4` 不是“空白”，而是“还停留在 accept / reject + hard interrupt 层”
- 真正最拖后腿的是 `S2`

## S1 复核：server endpoint 已是候选主路径，但还不是真正的主编排中心

### 已经落地的部分

当前 `server endpoint` 路径已经明显超出“debug experiment”阶段：

- `internal/app/config_voice.go` 已经有 `ServerEndpointEnabled` 及相关阈值配置
- `internal/gateway/realtime_ws.go` 与 `internal/gateway/xiaozhi_ws.go` 都已经：
  - 启动 preview session
  - 推送输入音频进入 preview
  - 轮询 preview 状态
  - 记录 `preview updated` / `commit suggested` 日志
  - 在满足条件时走 `session.CommitTurn()`
  - 输出 `gateway turn accepted`，并带上 `endpoint_reason`
- `internal/voice/turn_detector.go` 已经不是裸静音收尾，而是同时使用：
  - 最小音频时长
  - 静音窗口
  - lexical completeness guard
  - provider endpoint hint 缩短窗口

从工程状态看，`server_endpoint` 现在确实可以被称为“主路径候选”。

### 仍然没做完的部分

问题在于，这条链路虽然已经能触发 turn accept，但它还没有成为 session core 内部真正的一等输入轨。

当前 preview 的地位更像：

- 帮助网关更早决定何时 `CommitTurn()`
- 帮助共享 turn detector 提高自动收尾稳定性

而不是：

- 在 session core 内形成独立的、持续存在的 `input lane`
- 与输出播放状态并存，成为 turn orchestration 的长期主输入

换句话说，`InputPreview` 现在已经是重要信号，但仍主要服务于旧的 `CommitTurn()` 总闸门。

### 结论

端侧对 `S1` 的判断只说对了一半：

- 说对的部分：它确实应该从实验能力升级为主路径候选
- 没说完的部分：即使 `S1` 再做厚，也不能替代 `S2`

当前项目的实时性瓶颈，已经不主要是“有没有 server endpoint”，而是“server endpoint 进入系统后，是否还被单轨状态机重新压扁成老式回合制”。

## S2 复核：当前 session core 仍是单轨状态机，这是最主要结构瓶颈

### 代码事实

`internal/session/types.go` 当前只有一个共享状态枚举：

- `idle`
- `active`
- `thinking`
- `speaking`
- `closing`

`internal/session/realtime_session.go` 里：

- `CommitTurn()` 会直接把整个 session 切到 `thinking`
- `SetState(StateSpeaking)` 再把整个 session 切到 `speaking`

这意味着系统当前内部只有一个“全局回合状态”，没有显式区分：

- `input state`
- `output state`

### 当前 speaking 态下到底发生了什么

网关层确实已经做了一些增强：

- `internal/gateway/realtime_ws.go` 在 `speaking` 态下仍会继续吃入音频
- 这些音频会被暂存为 `pendingBargeIn`
- 也会继续生成 preview，并做 `EvaluateBargeIn(...)`
- 一旦放行，就先中断当前输出，再把音频冲回新一轮 turn

这比传统“speaking 时完全拒绝输入”已经好很多。

但它本质上仍然是：

`interrupt current output -> return active -> open next turn`

而不是：

`speaking output + live preview input` 的稳定并存。

### 为什么它会直接影响主观实时性

在真实语音交互里，用户会把下面几类现象都感知为“不够全双工”：

1. assistant 一开口，系统内部就默认“输入这轮已经结束了”
2. speaking 期间虽然还能说话，但系统只把它当作“是否打断”的候选，而不是持续输入
3. preview 只要想进入主流程，就必须重新穿过 `CommitTurn()`
4. thinking / speaking / active 之间仍然是串行翻门，而不是双轨并行推进

所以当前体验不好的根因，不是某一个阈值不对，而是编排模型本身仍是单轨。

### 结论

`S2` 是当前最该优先补的部分，而且优先级明显高于 `S1`。

如果不先把 session core 升级为“输入轨 / 输出轨”双轨模型，那么：

- `S1` 只会继续增强 auto-commit
- `S3` 只会继续增强首包音频时延
- `S4` 只会继续增强打断判断

但系统仍然会给人一种“本质上还是一轮一轮说”的感觉。

## S3 复核：边生成边说已有实质进展，但还没有完全从 turn finalize 脱钩

### 已经落地的部分

这部分当前仓库其实做得比端侧描述中“从零开始”更靠前。

当前链路已经具备：

- `internal/gateway/turn_flow.go`
  - 支持流式 `ResponseDelta` 先行发送
  - `response.start` 可以在 delta 出现时尽早发出
- `internal/voice/asr_responder.go`
  - `executeTurnWithPlannedSpeech(...)` 会在执行 turn 时创建 `plannedSpeechSynthesis`
- `internal/voice/speech_planner.go`
  - 会从稳定文本 delta 中按意群切出 segment
  - 一边接收 delta，一边起 TTS worker
- `internal/voice/synthesis_audio.go`
  - 支持流式 synth，并做首个非空 chunk 预取

也就是说，系统已经不是“必须等完整 `TurnResponse` 文本全部结束以后才能准备 TTS”。

### 还卡在哪

当前主要还卡在两个点：

#### 1. speaking 生命周期仍然绑定在 finalize 阶段

`internal/gateway/output_flow.go` 里的 `finalizeTurnLifecycle(...)` 仍然在 `audioStream != nil` 时：

- 先把 session 设成 `speaking`
- 再启动音频输出

这里的问题不是“不能说”，而是：

系统对 speaking 的正式承认，仍然落在 turn finalize 后段，而不是更细粒度的 output lane 生命周期里。

#### 2. planned audio 的播放时长估计仍偏弱

`plannedPlaybackDurationForResponse(...)` 当前只对 `AudioChunks` 计算明确时长；
如果走的是 `AudioStream`，当前返回 `0`。

这会直接影响：

- heard-text 估计精度
- playback progress 的语义强度
- 中断后“用户到底听到了多少”的估算稳定性

### 结论

`S3` 当前不能算“没做”，它已经是本项目最有成效的实时性优化之一。

但它还没有完全进入“text delta / response.start / audio byte 三者真正稳定重叠”的成熟阶段。

下一步真正要做的，不是再堆一个 planner，而是把它放进 `S2` 之后的新双轨输出模型里。

## S4 复核：已经有硬打断基础，但还没有多策略仲裁层

### 已经落地的部分

当前仓库已经不是“检测到声音就一刀切”：

- `internal/voice/barge_in.go` 已经做了：
  - 最小时长门槛
  - incomplete hold
  - lexical completeness 判断
- `internal/gateway/realtime_ws.go` 会根据 `EvaluateBargeIn(...)` 的结果决定是否进入打断流
- `internal/voice/session_orchestrator.go` 已经会在 `InterruptPlayback()` 时持久化：
  - `heardText`
  - `ResponseInterrupted`
  - `ResponseTruncated`

也就是说：

- 真打断的放行条件已经比过去细
- 打断后记忆也不再只保存未播完全文

### 缺的关键层

但当前的策略空间仍然只有两类：

- 不接受
- 接受，并进入 hard interrupt

它还没有明确区分：

- `backchannel`
- `duck only`
- `hard interrupt`
- `ignore`

这会造成两个后果：

1. 用户短附和音只能被视为“拒绝打断”或“直接打断”  
   中间没有更自然的行为层。
2. 后续如果要做 `resume / continue / hold-and-yield`，当前决策模型不够承载

### 结论

`S4` 的方向判断是对的。

但当前更准确的说法应该是：

项目已经有一个“不错的 hard-interrupt gate”，还没有“多策略 interruption arbiter”。

## 为什么当前实时性还不够好

综合 `S1` 到 `S4`，当前实时性和全双工感不足，主要不是单点故障，而是下面四个结构叠加：

### 1. session core 仍把输入结束、thinking、speaking 绑在一条状态线上

这是最大的根因。

### 2. preview 虽然已经重要，但仍主要服务于 `CommitTurn()`

它还没有变成 session core 内部的一等输入轨。

### 3. 增量 TTS 已经存在，但输出生命周期还不够“独立成轨”

planner 已经提早了音频准备，但 speaking 仍更像 finalize 的后段阶段。

### 4. interruption 仍以 hard interrupt 为中心

这使系统更像“聪明一点的抢话”，而不是“真正会话式的同听同说编排”。

## 对主线优先级的结论

下一阶段如果目标是显著提升“实时性 / 双工感 / 自然度”，建议优先级为：

1. `S2`：先把 session core 升级为输入/输出双轨模型
2. `S4`：在双轨基础上引入 interruption arbitration
3. `S3`：把 planner / streaming TTS 放进更成熟的 output lane 生命周期
4. `S1`：继续硬化 server endpoint，但不要把它误判成主瓶颈

一句话说，就是：

先改“会话编排骨架”，再继续压“endpoint / planner / barge-in”的局部体验。

## 对后续设计的直接含义

如果沿当前主线继续演进，建议遵循下面几个约束：

### 1. 先做 session 内部双轨，不急着改公网协议

第一步应该是内部结构升级，而不是对外增加更多事件名。

建议先在 session core 内至少显式拆出：

- `InputState`
- `OutputState`

让以下状态可以真实并存：

- `output=speaking`
- `input=previewing`

### 2. 把 `CommitTurn()` 从“单一总闸门”拆成更细的 turn accept 流程

当前 `CommitTurn()` 同时承担了：

- 输入结束
- thinking 开始
- 回合切换

这对 half-duplex 足够，但对全双工不够。

下一步应把“输入被接受”和“输出进入 speaking”拆成两个不同生命周期节点。

### 3. interruption 决策应返回策略，而不只是 accepted bool

建议后续从 `EvaluateBargeIn(...)` 升级到显式策略结果，例如：

- `ignore`
- `backchannel`
- `duck_only`
- `hard_interrupt`

这样 `SessionOrchestrator` 才能继续演进到：

- heard-text reconciliation
- output resume / continue
- low-risk accompaniment handling

### 4. playback accounting 要补强 `AudioStream` 路径

否则后续即使编排模型升级了：

- heard-text
- truncation estimation
- interruption memory quality

仍会在流式音频路径上偏弱。

## 最终判断

端侧这次 `S1` 到 `S4` 的建议，整体方向是正确的。

如果一定要概括当前项目的真实状态，可以这样描述：

- `S1`：已经是主路径候选，不再只是实验
- `S2`：仍是当前最大结构缺口
- `S3`：已经有明显进展，但还未完全解耦
- `S4`：已有硬打断基础，但缺少多策略仲裁

因此，当前主线最该优先补的不是“更多 endpoint 调参”，而是：

`先把 realtime session core 从单轨状态机升级为输入/输出双轨模型。`

只有这一层先落地，后面的：

- server endpoint graduation
- true overlap TTS
- natural interruption policy

才会从“体验补丁”变成“稳定系统能力”。

## 2026-04-15 实现跟进

在本次复核完成后，仓库已经落下了第一轮对应 `S1` 到 `S4` 的实现切片，主要包括：

- `internal/session` 已升级为输入轨 / 输出轨双轨状态，`state` 改为兼容视图而不是唯一真相
- `session.update` 已补充 `input_state`、`output_state`、`accept_reason`
- `server_endpoint` 接受 turn 已走统一 accepted-turn 路径，且保留 `state` 兼容字段
- speaking 期间的 preview / staged audio 不再会因为当前输出自然播放完毕而被直接清空
- interruption policy 已形成 `ignore / backchannel / duck_only / hard_interrupt` 的共享运行时语义

因此，本文前面给出的粗略完成度评估应被理解为“实现前的复核判断”，不是当前仓库此刻的最新完成度刻度。

当前剩余的主缺口已经更集中到两类：

1. `duck_only` / `backchannel` 还没有真正落实为 playout ducking、resume、continue 之类的输出策略动作
2. `response.start`、text delta、audio byte 的重叠虽然已经明显改善，但 speaking 生命周期仍主要从 finalize / startAudioStream 边界进入，尚未完全升级成更成熟的 output-lane-first 编排模型

也就是说，当前主线已经不再是“先把双轨状态做出来”，而是：

`在双轨状态已经落地的前提下，把 softer interruption 行为和更早的 output orchestration 做实。`
