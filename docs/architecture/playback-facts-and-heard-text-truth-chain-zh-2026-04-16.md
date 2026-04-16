# 播放事实回传与 heard-text 真相链（2026-04-16）

## 文档性质

- 本文是研究讨论材料，不是最终实施方案。
- 目标不是单纯讨论“要不要加一个 playback ack 事件”，而是回答：
  - 在语音 agent 里，服务侧究竟需要知道哪些“播放事实”；
  - `generated text`、`sent audio`、`played audio`、`heard text` 之间到底是什么关系；
  - 为什么 `heard-text` 会成为 interruption、resume、memory、自然感的地基。

## 一句话结论

**`heard-text` 不是一个原始事实，而是一条真相链的推断结果。**

更准确地说，系统内部至少要区分：

1. `generated_text`：模型或 planner 生成了什么
2. `sent_audio / delivered_text`：服务侧发出了什么
3. `playback_started`：端侧是否真的开始播了
4. `playback_progress / playback_mark`：端侧播到了哪里
5. `playback_cleared / interrupted / completed`：哪些内容实际上没播完
6. `heard_text`：基于上面的 playback facts 反推出“用户大概率听到了多少”

所以：

- **生成出来** 不等于 **发出去了**
- **发出去了** 不等于 **播放了**
- **播放了** 不等于 **用户真的听见并理解了**

对当前项目而言，最重要的不是追求“绝对精确的 heard truth”，而是：

- 尽快建立一条**可靠、可分层、带置信度的 playback truth chain**；
- 避免再把“完整生成回复”直接当成“用户已经听完回复”。

## 为什么这件事是语音体验的地基

### 1. interruption 后，如果不知道用户听到了哪里，就没法自然续说

如果系统只知道：

- “我本来打算说这整段话”

却不知道：

- “用户实际上只听到了前半句”

那么后果通常是：

- resume 时从错误位置继续
- 后续澄清时默认“这件事我已经说过了”
- memory 记录成系统说过完整回复，但用户并没有听完

### 2. `duck_only` / `backchannel` 的价值，也依赖 playback facts

如果 speaking 时用户插话：

- 进入了 `duck_only`
- 或最终升级成 `hard_interrupt`

服务侧需要知道的不是“我有没有尝试停播”，而是：

- 当前输出到底已经播了多少
- 哪些缓冲已经被清掉
- 哪一段是用户真正听到的 assistant text

否则：

- `duck_only` 的效果没法被评估
- false interruption 很难恢复
- continue/resume 策略会很机械

### 3. 记忆和多轮一致性会被“假 heard”污染

如果 memory 用的是：

- 完整生成文本

而不是：

- 已播到哪、已听到哪

那么系统会出现典型错位：

- 用户问“刚刚最后一句你说什么”
- 系统却以为自己已经说完了全部内容

这会直接破坏人性化与可信度。

## 当前仓库已经走到哪一步

本仓库其实已经明确承认了这个问题，而且边界方向是对的。

### ADR 0024 已经把 preview / playout / heard-text 放进 voice runtime

`docs/adr/0024-voice-runtime-owns-preview-playout-and-heard-text.md` 已经明确：

- memory 过去只知道生成文本，不知道用户真正听到了什么；
- `internal/voice.SessionOrchestrator` 应拥有 preview、playout、interruption、completion 与 heard-text persistence；
- runtime memory 应记录：
  - `delivered`
  - `heard`
  - `interrupted`
  - `truncated`
  - `playback_completed`

这说明当前项目的架构方向是正确的：

- **网关只回报事实**
- **共享 voice runtime 解释事实并形成 heard-text**

参考：

- `docs/adr/0024-voice-runtime-owns-preview-playout-and-heard-text.md`

### 但 ADR 也清楚承认了一个现实：当前 playback progress 仍偏启发式

ADR 同时指出：

- 当前 slice 里 playback progress 仍是 heuristic；
- 原因是并非每种 adapter 都已经有精确 client playout ack。

这意味着：

- 当前项目已经有“真相链的结构”，但还缺“更高置信的事实输入”；
- 现在最该研究的不是重写边界，而是**如何补强 adapter -> runtime 的播放事实回传精度**。

### Overview 里的边界表述也很清晰

`docs/architecture/overview.md` 也已写明：

- `internal/voice` 是 preview session、playback lifecycle callback、heard-text persistence 的 owner；
- adapter 负责把 transport events 报给 orchestrator，而不是自行解释。

这与本轮研究结论高度一致：

- 真相链必须属于共享 runtime，不能散在各 adapter。

## 更准确的概念：heard-text 是“最佳可得估计”，不是绝对真相

这里需要一个非常重要的概念校准。

### 严格意义上，“用户听到了什么”并不总是可观测

哪怕端侧回报了：

- 音频开始播放
- 已播到 1.5s
- 缓冲已清空

服务侧仍然无法完全知道：

- 用户是否把音量关了
- 当前是不是切到了蓝牙耳机但没戴上
- 周围噪声是否盖住了播报
- 用户是否注意力已转移

所以，`heard-text` 更精确的含义应是：

- **基于端侧播放事实推断的“用户大概率已听到内容”**

### 因此我更建议把真相链拆成两层

#### 第一层：硬事实（high-trust facts）

例如：

- 服务侧发送了哪些音频 chunk
- 端侧是否确认开始播放
- 端侧确认播放完成的 mark / clause
- 端侧是否 clear 了缓冲
- 当前播放 cursor 在哪里

这些是尽量可观测、可记录、可追责的。

#### 第二层：推断事实（derived facts）

例如：

- `heard_text`
- `heard_ratio`
- `resume_anchor`
- `what user likely missed`

这些应由 runtime 根据第一层硬事实推导，并且最好带上：

- `source`
- `confidence`
- `precision_tier`

## 外部实践为什么支持这条方向

### OpenAI Realtime：WebRTC/SIP 里服务端知道播放进度；WebSocket 里客户端必须显式告知截断点

OpenAI Realtime 的官方文档很直接地把这个问题讲透了：

- 在 WebRTC / SIP 连接里，服务端维护输出音频缓冲，因此知道已经播了多少；
- 在 WebSocket 连接里，客户端自行管理播放，因此客户端必须：
  - 收到用户开口信号后立即停播
  - 记录上一个 response 已经播到了多少
  - 发送 `conversation.item.truncate`，带 `audio_end_ms`
- 文档还特别指出：模型并没有足够信息把 transcript 精确对齐到 audio，因此 truncate 可以精确裁音频，但未必能精确给出“截断后的文本”。

这非常有价值，因为它说明：

1. **播放真相链是连接模式相关的**；
2. **WebSocket 模式下，客户端 playback facts 是必需品，而不是锦上添花**；
3. **即使是顶级语音 API，也承认 text/audio 对齐并不天然精确。**

参考：

- <https://platform.openai.com/docs/guides/realtime-conversations>
- <https://platform.openai.com/docs/api-reference/realtime-client-events/conversation/item/truncate>

### LiveKit：中断后 conversation history 应截断到“用户实际听到的部分”

LiveKit 文档也明确强调：

- 当用户打断时，agent 应停止说话；
- 对话历史应只保留用户在中断前真正听到的那部分；
- interruption handling 还应结合最小语音时长、最小词数、false interruption timeout、resume false interruption 等策略。

这说明：

- “保留用户听到的部分”已经是现代语音 agent 编排层的主流要求；
- `heard-text` 不是附加功能，而是 interruption policy 的一部分；
- playback truth chain 与 interruption arbitration 必须联动设计。

参考：

- <https://docs.livekit.io/agents/logic/turns/>
- <https://docs.livekit.io/reference/agents/turn-handling-options/>
- <https://docs.livekit.io/agents/logic/turns/adaptive-interruption-handling>

### Twilio：`mark` / `clear` 是非常实用的播放事实回传范式

Twilio Media Streams 给出了一个极具工程价值的模式：

- 服务端发送 `media` 后，再发送 `mark`
- 当对应音频真正播完，Twilio 会回一个同名 `mark`
- 若服务端发送 `clear` 清空缓冲，Twilio 也会回传相关 `mark`
- 这样应用层就能知道：
  - 哪些 media 真播完了
  - 哪些只是进过缓冲、但后来被清掉，不会再播放

这套机制对当前项目很有启发：

- 不一定非要做毫秒级 cursor 才开始；
- **分段 mark ack 本身就是高价值的 MVP**；
- clause/segment 级 ack 往往已经足够改善 heard-text 可靠性。

参考：

- <https://www.twilio.com/docs/voice/media-streams/websocket-messages>

## 我更建议当前项目把真相链分成 4 个精度层级

### `Tier 0`：server heuristic only

服务侧只知道：

- 自己发送了多少字节
- 这些字节理论时长是多少
- interruption 发生在什么时候

优点：

- 最容易实现

问题：

- 只能做很粗的 heard estimate
- 不适合精确 resume / continue
- 不适合当 memory 事实强写入

### `Tier 1`：segment / mark acknowledged

端侧回报：

- `playback_started`
- `segment_mark_played`
- `buffer_cleared`
- `playback_completed`

优点：

- 成本低
- 很适合研究阶段 MVP
- 已经足够支持 clause 级 heard-text persistence

问题：

- 对 segment 内部的精确截断仍较粗

### `Tier 2`：playout cursor aware

端侧回报：

- 已播毫秒位置
- 当前播放 item / chunk / clause 序号
- clear 时的精确 cursor

优点：

- 可做更精确的 resume anchor
- 更适合 interruption 后“从哪儿继续”
- 能更稳地估计 heard ratio

问题：

- 端侧实现复杂度更高
- 不同 adapter 能力差异大

### `Tier 3`：audibility-aware / context-aware

进一步考虑：

- muted / paused / route changed
- 本地 duck 状态
- 设备音量极低
- 可能的渲染失败或 underrun

优点：

- 更接近真实可听性

问题：

- 复杂度最高
- 研究阶段暂时不必追求一步到位

## 当前项目最小必须回传哪些事实

我更建议区分“最小必须”和“强烈建议”。

### 最小必须（MVP）

1. `playback_started`
2. `playback_cleared`
3. `playback_completed`
4. `segment_mark_played` 或者等价的 clause/response chunk completed

只要这四类事实稳定，服务侧就已经能从“纯猜”升级到“有依据地推断 heard-text”。

### 强烈建议（下一步）

1. `played_ms` 或 playout cursor
2. `current_item_id / clause_id / chunk_id`
3. `duck_active on/off`
4. `pause / mute / route_change`
5. `playback_error / underrun`

这些会极大提升：

- duck_only 评估
- false interruption 恢复
- continue / resume 自然度
- latency tracing 的真实性

## 误差会具体伤害哪些体验

### 1. 继续说错位置

服务侧以为用户听到了 `A+B`，但实际上只播完了 `A`。

结果：

- 用户问“后半句是什么？”
- 系统却从 `C` 开始接着说，体验会非常怪。

### 2. memory 写错事实

系统把完整生成段落记成“已说过”。

结果：

- 后续认为“我已经解释过了”
- 实际用户只听到了半句

### 3. interruption policy 被错误奖励或惩罚

若没有真实 playback facts：

- `duck_only` 成功让了多少，无法量化
- `hard_interrupt` 是否太晚，也无法精确判断

### 4. latency tracing 出现“伪快”

如果只记录 `first_audio_chunk_sent`，但端侧实际缓冲很久才响：

- 日志会显示系统很快
- 用户却主观觉得系统很慢

所以，`first_audio_chunk` 不是终点；还需要更接近 `first_audible_playout` 的事实。

## 对当前项目最重要的结论

### 1. `heard-text` 应继续留在共享 voice runtime，而不是下沉到各 adapter 各自解释

这一点和当前 ADR/overview 完全一致，应继续坚持。

### 2. 研究阶段最值得优先补的是 `Tier 1` 级播放事实，而不是一步做满 `Tier 3`

也就是：

- start
- mark/segment played
- clear
- completed

这已经能显著改善：

- interruption 后 memory 正确性
- resume/continue 自然度
- 对 duck_only / hard interrupt 的评估

### 3. `heard-text` 写入 memory 时最好显式带来源与置信度

例如概念上区分：

- `heard_source=heuristic_bytes`
- `heard_source=segment_mark`
- `heard_source=playout_cursor`

以及：

- `heard_confidence=low|medium|high`

因为“已生成全文”与“高置信 heard”不是一回事。

### 4. 当前项目后续若继续做全双工与 resume，这条真相链会比更多 prompt 技巧更关键

很多“像人”的感觉并不是来自更会说，而是来自：

- 知道自己说到哪了
- 知道对方听到哪了
- 被打断后不会失忆
- 继续时不会从错误位置接上

## 当前研究阶段最建议记住的四句话

1. **generated 不等于 delivered，delivered 不等于 played，played 也不等于 truly heard。**
2. **`heard-text` 更像“最佳可得估计”，需要基于 playback facts 推断。**
3. **最小可行的升级，不是先做完美毫秒对齐，而是先拿到稳定的 playback start / mark / clear / complete。**
4. **若没有这条真相链，interruption、resume、memory、自然感都会长期漂。**

## 相关本地文档

- `docs/adr/0024-voice-runtime-owns-preview-playout-and-heard-text.md`
- `docs/architecture/overview.md`
- `docs/architecture/realtime-full-duplex-gap-review-zh-2026-04-15.md`
- `docs/architecture/server-primary-hybrid-min-device-capabilities-and-interruption-zh-2026-04-16.md`
- `docs/architecture/duck-only-timing-and-escalation-zh-2026-04-16.md`

## 参考资料

- OpenAI Realtime conversations / truncation:
  - <https://platform.openai.com/docs/guides/realtime-conversations>
  - <https://platform.openai.com/docs/api-reference/realtime-client-events/conversation/item/truncate>
- LiveKit turns / interruptions:
  - <https://docs.livekit.io/agents/logic/turns/>
  - <https://docs.livekit.io/reference/agents/turn-handling-options/>
  - <https://docs.livekit.io/agents/logic/turns/adaptive-interruption-handling>
- Twilio Media Streams websocket messages:
  - <https://www.twilio.com/docs/voice/media-streams/websocket-messages>
