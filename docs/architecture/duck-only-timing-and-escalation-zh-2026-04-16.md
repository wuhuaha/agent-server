# `duck_only` 时间窗与升级条件研究（2026-04-16）

## 文档性质

- 本文是研究讨论材料，不是最终实施方案。
- 目标是为当前 `agent-server` 的 speaking 期重叠语音仲裁，建立一个足够现代、但不过度架构化的讨论框架。

## 一句话结论

`duck_only` 的核心价值，不是“更复杂”，而是给实时语音系统一个 **可逆的犹豫层**。

没有这一层，系统通常只剩两种糟糕体验：

- 一听到有人声就 `hard_interrupt`
- 或者一直强播，直到很晚才反应

更合理的链路是：

- `speech_start`
- `previewing`
- 先进入 `duck_only`
- 再在一个短时间证据窗里收敛为：
  - `ignore`
  - `backchannel`
  - `hard_interrupt`

## 为什么 `duck_only` 需要独立时间窗

### 1. `speech_start` 只能证明“有近端语音样信号”，不能证明“用户要夺回话轮”

它天然分不清：

- 附和词
- 咳嗽 / 笑声 / 叹气
- 回声 / 自播音泄露
- 旁边人说话
- 犹豫开头
- 真正的纠正、打断、新请求

因此，`speech_start` 更适合做快速反射信号，而不适合直接做 `hard_interrupt` 终判。

### 2. 公开实践基本都在做“先感知、后确认”

- OpenAI `semantic_vad` 不是固定静音门限，而是根据“用户是否可能说完”动态决定等待时间。参考：<https://platform.openai.com/docs/guides/realtime-vad>
- LiveKit 明确暴露了 `min_duration`、`min_words`、`false_interruption_timeout` 和自适应 interruption。参考：<https://docs.livekit.io/agents/build/turns/>
- Pipecat 明确把 `Start` 和 `Stop` 分开：前者可由 VAD/转写触发，后者由 AI turn detection 确认。参考：<https://docs.pipecat.ai/api-reference/server/utilities/turn-management/user-turn-strategies>
- Deepgram Flux 用 `EagerEndOfTurn -> TurnResumed -> EndOfTurn` 区分早猜测与最终确认。参考：<https://developers.deepgram.com/docs/flux/voice-agent-eager-eot>
- Amazon 将 barge-in 流程拆为 detection / verification / recovery。参考：<https://assets.amazon.science/56/4a/d81efc094934a2fdafdbe03d63f0/contextual-acoustic-barge-in-classification-for-spoken-dialog-systems.pdf>
- Apple `Talking Turns` 指出很多系统过于激进地打断，同时又缺少合适的附和行为。参考：<https://machinelearning.apple.com/research/talking-turns>

## 推荐的时间窗拆法

下面的数值不是某一家官方统一标准，而是基于公开资料与当前项目需求做的研究阶段推断。

### 1. `initial reflex window`

- 目标：尽快表现出“我注意到你可能在说话了”
- 推荐范围：`0-120ms`
- 推荐动作：
  - 触发 `speech_start candidate`
  - 暂停继续扩张新的 TTS chunk 排队
  - 做很轻的 duck，例如 `-3dB` 到 `-6dB`
  - 不立刻清空已缓冲输出
  - 开始缓存重叠语音并观察 partial

### 2. `evidence accumulation window`

- 目标：在很短时间内判定更像 `ignore / backchannel / duck_only继续 / hard_interrupt`
- 推荐范围：`120-450ms`
- 主要观察：
  - 语音是否持续
  - partial / interim transcript 是否出现稳定前缀
  - 是否出现高 takeover 意图词
  - 是否像 echo / 噪声 / 旁谈
  - 当前 agent 输出是在句首、中段还是收尾
- 推荐动作：
  - 保持 duck
  - 暂缓最终停播
  - 允许做轻量 prewarm，但不做不可逆提交

### 3. `escalation window`

- 目标：对“持续说话且证据增强”的情况做最终升级
- 推荐范围：`450-900ms`
- 主要动作：
  - 升级到 `hard_interrupt`
  - 清空后续待播音频
  - 停止或截断当前响应生成
  - 切换 input 主导权
  - 记录 heard-text 边界
- 注意：
  - 这不是“必须等到 900ms 才能停”
  - 若提前出现强中断词或高 directedness，可提前升级

### 4. `release window`

- 目标：如果证据不足，优雅回到继续播音
- 推荐范围：`150-500ms`
- 主要动作：
  - 从 duck 状态恢复音量
  - 恢复 TTS chunk 下发
  - 不把这次短声当作 turn switch

### 5. `false interruption recovery window`

- 目标：已经做了较强中断动作后，保留恢复机会
- 推荐范围：`1.0-2.0s`
- 参考：LiveKit 默认 `false_interruption_timeout = 2.0`
- 主要动作：
  - 若确认误触发，恢复之前的 assistant 输出
  - 但更建议做轻量 resume 或 recap，而不是机械续播

## 更适合当前项目的动态窗表达

比起固定毫秒，更推荐讨论成“带上下界的动态确认窗”：

- `W_reflex = 80~120ms`
- `W_accumulate = 200~450ms`
- `W_escalate_max = 700~900ms`
- `W_release = 150~400ms`
- `W_false_resume = 1~2s`

并按场景调偏置：

- 命令型场景：更短 `W_accumulate`，更早升级
- 长解释场景：更长 `W_accumulate`，更依赖语义和 output phase
- 慢节奏用户：更长 `W_release`，更高 `hard_interrupt` 门槛
- 最近误打断频繁：拉长 `W_accumulate`，降低 VAD 敏感度
- 出现强中断词：直接跳过部分窗，提前升级

## 升级为 `hard_interrupt` 的关键条件

### A. 声学持续性

- 近端语音持续时间超过最短中断时长
- 能量 / SNR 持续稳定，而非瞬时 burst
- 不是 echo 或环境旁谈

### B. partial / stable prefix 已出现

- 有稳定词串，而不是空转写
- 中文里哪怕只是很短的稳定高意图前缀，也可能足以升级

### C. 明确的 takeover / correction / stop 语义

典型中文词：

- 等一下
- 不是
- 停
- 重新说
- 你先别说
- 我问的是
- 不用了

### D. device-directedness 足够高

- 更像在对设备说，而不是对旁人说
- 即便研究阶段暂不做重型多模态模型，也至少应纳入：
  - 是否像命令句
  - 是否与当前任务上下文相关
  - 是否包含典型设备指向语气

### E. output state 不利于继续强播

- 当前正在长解释
- 当前处于句中，不适合持续与用户重叠
- 已经发生多次用户尝试插入
- 当前 chunk 虽未播完，但 takeover 证据已足够

## 回落为 `backchannel` 或 `ignore` 的关键条件

### 回落为 `backchannel`

- 语音很短
- transcript 是短附和词
- 没有后续内容扩展
- 没有强 takeover 语义
- 用户更像在“跟着你听”，不是在“接管话轮”

### 回落为 `ignore`

- 没有稳定 transcript
- 低 SNR / 回声 / 旁谈 / 非语言声音
- 持续时间极短
- 与当前对话上下文无关
- 被后续声学或语义证据否定

## 中文语音 agent 特别要注意的点

### 1. 中文里“词数阈值”比英文更不可靠

很多关键打断信号可能只有 1 个词甚至 1 个音节：

- 停
- 不对
- 等等
- 不是

因此中文更适合依赖：

- 意图词类
- 稳定字串
- 字级或音节级稳定前缀

### 2. 中文 backchannel 极短、极轻、极频繁

- 嗯
- 对
- 哦
- 好
- 行
- 啊
- 是

这正是中文比英文更需要 `duck_only` 的重要原因。

### 3. 中文句中停顿很多，不能见停就收

- 列举
- 自我修正
- 话题前置
- 想词停顿

这与 Amazon endpointing、Google unified endpointing 的结论一致：句中 pause 和句末 pause 必须区分。

### 4. 中文里“纠错式打断”很常见

- 不是，我是说
- 不对，应该是
- 等等，我换个说法

这类一旦出现，应显著缩短升级路径。

### 5. 智能家居 / 桌面助理的中文命令很短

- 关灯
- 打开空调
- 静音
- 暂停
- 别播了

所以在这类域里：

- `duck_only` 应更短
- `hard_interrupt` 升级应更快

## 对当前 `agent-server` 最值得借鉴的边界

### 1. `duck_only` 只做中间态，不无限停留

若超过 `W_escalate_max` 还不收敛，系统会显得犹豫。因此研究阶段建议在上限内强制收敛为：

- `hard_interrupt`
- `backchannel`
- `ignore`

### 2. 最适合当前项目的渐进链路

- `speech_start`
- `input_state=previewing`
- 先 `duck_only`
- 200~450ms 内快速看：
  - 持续时长
  - stable partial
  - takeover lexicon
  - directedness
  - output progress
- 最终收敛为：
  - `hard_interrupt`
  - `backchannel`
  - `ignore`

### 3. 当前最值得继续深挖的问题

如果只沿这条线继续推进，最值得继续讨论的是：

- 哪些信号最先进入 `duck_only` 判定器
- 哪些信号只用于后验确认
- 以及中文场景下的动态时间窗打分函数应如何设计

## 参考资料

- OpenAI Realtime VAD：<https://developers.openai.com/api/docs/guides/realtime-vad>
- LiveKit Turns overview：<https://docs.livekit.io/agents/logic/turns/>
- LiveKit Adaptive interruption handling：<https://docs.livekit.io/agents/logic/turns/adaptive-interruption-handling/>
- Pipecat User Turn Strategies：<https://docs.pipecat.ai/api-reference/server/utilities/turn-management/user-turn-strategies>
- Deepgram Flux configuration：<https://developers.deepgram.com/docs/flux/configuration>
- Deepgram Eager End of Turn：<https://developers.deepgram.com/docs/flux/voice-agent-eager-eot>
- AssemblyAI Turn Detection：<https://www.assemblyai.com/docs/streaming/universal-streaming/turn-detection>
- Apple Talking Turns：<https://machinelearning.apple.com/research/talking-turns>
- Amazon Contextual Acoustic Barge-In Classification：<https://assets.amazon.science/56/4a/d81efc094934a2fdafdbe03d63f0/contextual-acoustic-barge-in-classification-for-spoken-dialog-systems.pdf>
- Amazon Accurate Endpointing with Expected Pause Duration：<https://www.amazon.science/publications/accurate-endpointing-with-expected-pause-duration>
- Google Unified End-to-End Speech Recognition and Endpointing：<https://research.google/pubs/unified-end-to-end-speech-recognition-and-endpointing-for-fast-and-efficient-speech-systems/>
- Google Low Latency Speech Recognition using End-to-End Prefetching：<https://research.google/pubs/low-latency-speech-recognition-using-end-to-end-prefetching/>
