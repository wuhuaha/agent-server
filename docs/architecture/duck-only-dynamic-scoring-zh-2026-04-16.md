# `duck_only` 动态打分函数研究（2026-04-16）

## 文档性质

- 本文是研究讨论材料，不是最终实施方案。
- 它建立在 `docs/architecture/duck-only-timing-and-escalation-zh-2026-04-16.md` 的时间窗框架之上，进一步讨论：
  - `duck_only` 的动态打分函数该怎么设计
  - 哪些信号适合先验快速进入
  - 哪些信号适合后验确认

## 一句话结论

`duck_only` 最适合的不是“一个总分直接判四分类”，而是：

- **先验快速入场分**：决定是否进入 `duck_only`
- **后验确认分**：决定最终收敛为 `ignore / backchannel / hard_interrupt`

更具体地说：

- `duck_only` 不是最终类别
- `duck_only` 是一个 **短时间可逆中间态**
- 它最适合由“两段动态打分”驱动，而不是由 VAD 或单一阈值驱动

## 为什么不建议一开始就做“四分类单模型”

### 1. 可用信号的时间到达顺序不同

在 speaking 期间出现新语音时：

- 最早到的是：
  - 近端语音 onset
  - 本地 VAD hint
  - overlap 持续时间
  - 当前 output state
- 更晚到的是：
  - stable partial
  - takeover lexicon
  - correction cue
  - directedness proxy
  - slot/intent 变化

如果把这些硬塞成一个同时刻可用的总分，会导致：

- 早期时刻只能靠缺失特征乱判
- 或者为了等特征齐全而牺牲响应性

### 2. `duck_only` 的核心价值就是“先让一点，再确认” 

因此它最自然的表达不是：

- `class = duck_only`

而是：

- `intrusion_prior enough -> enter duck_only`
- `confirmation evidence enough -> upgrade or release`

## 推荐的整体框架：两段打分 + 一个时间窗状态机

## 第一段：`intrusion_prior`

### 目标

决定：

- 当前是否值得进入 `duck_only`
- 是否值得对当前 output 进行轻微 duck

### 更适合进入的信号：先验快速分

#### A. `acoustic_intrusion_score`

构成建议：

- 近端 VAD / speech probability
- onset energy
- 近端持续时间
- SNR
- 是否像 burst 还是持续说话

#### B. `overlap_conflict_score`

构成建议：

- 当前是否仍在 speaking
- 当前 output 剩余是否较长
- 当前是否已进入 `previewing`
- 用户发声是否与播放明显重叠

#### C. `local_reflex_score`

构成建议：

- 端侧本地 speech hint
- 本地 playback duck trigger
- 端侧 render-reference / echo residual 判据

#### D. `anti_false_trigger_penalty`

构成建议：

- echo likelihood
- low SNR
- noise/non-speech likelihood
- off-device / low directedness proxy

### 研究阶段建议表达

```text
intrusion_prior =
  + w1 * acoustic_intrusion_score
  + w2 * overlap_conflict_score
  + w3 * local_reflex_score
  - w4 * anti_false_trigger_penalty
```

### 作用

- 若 `intrusion_prior < T_enter`：不进 `duck_only`
- 若 `intrusion_prior >= T_enter`：进入 `duck_only`

注意：

- 这一步 **不直接** 判 `hard_interrupt`
- 它只负责让系统“先表现出我注意到你了”

## 第二段：`takeover_confirmation`

### 目标

决定 `duck_only` 之后最终收敛方向：

- `hard_interrupt`
- `backchannel`
- `ignore`

### 更适合进入的信号：后验确认分

#### A. `prefix_stability_score`

构成建议：

- stable prefix 长度
- 连续多次 partial 更新保持不变的前缀比例
- partial edit distance 是否持续收敛
- token/字级 final-ish 标记（若模型可提供）

#### B. `lexical_takeover_score`

构成建议：

- 是否出现强 takeover/correction/stop 语义：
  - 等一下
  - 不是
  - 停
  - 重新说
  - 你先别说
  - 我是说
  - 改成
- 是否出现明确新请求动词

#### C. `directedness_score`

构成建议：

- 是否像对设备说话
- 是否与当前任务语义相关
- 是否像命令句/纠错句/追问句
- 是否处于唤醒后 follow-up 窗

#### D. `output_conflict_score`

构成建议：

- 当前系统输出是否处于长解释中段
- 当前输出是否即将自然收尾
- 用户是否已经多次试图插入
- 当前已 heard 的内容是否足够

#### E. `backchannel_likelihood`

构成建议：

- transcript 是否是短附和词
- 时长是否很短
- 没有新 slot / 新 intent 扩展
- 与当前 agent 提问 / 确认节奏匹配

#### F. `ignore_likelihood`

构成建议：

- 无稳定 transcript
- 非语言声
- 低 directedness
- echo / noise 解释更强

### 研究阶段建议表达

```text
takeover_confirmation =
  + a1 * prefix_stability_score
  + a2 * lexical_takeover_score
  + a3 * directedness_score
  + a4 * output_conflict_score
  - a5 * backchannel_likelihood
  - a6 * ignore_likelihood
```

### 收敛逻辑建议

- 若 `takeover_confirmation >= T_interrupt`：`hard_interrupt`
- 否则若 `backchannel_likelihood >= T_backchannel`：`backchannel`
- 否则若 `ignore_likelihood >= T_ignore`：`ignore`
- 否则保持 `duck_only`，直到：
  - 新证据到达
  - 或时间窗上限到期

## 一个更像当前项目的状态表达

更适合当前 `agent-server` 的不是“永远算分”，而是配合时间窗：

### `Phase 1: reflex`

- 计算 `intrusion_prior`
- 决定是否进入 `duck_only`

### `Phase 2: accumulate`

- 保持 `duck_only`
- 逐步计算 `takeover_confirmation`
- 同时跟踪 `backchannel_likelihood`、`ignore_likelihood`

### `Phase 3: resolve`

在 `W_escalate_max` 内收敛为：

- `hard_interrupt`
- `backchannel`
- `ignore`

### `Phase 4: recover`

- `duck_only -> 恢复`
- 或 `hard_interrupt -> 新 turn`

## 我更推荐的最小打分字段集

如果只保留研究阶段最小必要字段，我更建议下面这一组：

```text
DuckOnlyScoringInputs {
  acoustic_intrusion_score
  overlap_conflict_score
  anti_false_trigger_penalty
  prefix_stability_score
  lexical_takeover_score
  directedness_score
  backchannel_likelihood
  ignore_likelihood
  output_phase
  elapsed_in_duck_window_ms
}
```

说明：

- `output_phase`：句首/中段/收尾
- `elapsed_in_duck_window_ms`：用于动态调阈值

## 为什么这比“一个 hard_interrupt_score”更好

因为 `backchannel` 和 `ignore` 不是 `hard_interrupt` 的负例那么简单。

例如：

- `嗯` 可能是：
  - backchannel
  - 也可能是 correction 开头前的犹豫
- 一个短促近端 burst 可能是：
  - ignore
  - 也可能是 hard interrupt 的开始

所以更合理的是：

- 同时维护几种假设
- 再用时间和新证据让它们收敛

## 不同场景如何调权重

## 1. 命令型场景

例如：

- 智能家居
- 桌面助理
- 设备控制

建议：

- 提高 `lexical_takeover_score` 权重
- 提高 `prefix_stability_score` 对短稳定前缀的敏感度
- 缩短 `W_accumulate`
- 降低 `backchannel_likelihood` 默认权重

原因：

- 中文命令往往短而强
- `停`、`不对`、`不是` 这类一两个词就足够说明 takeover

## 2. 解释型场景

例如：

- 长回答
- 解释说明
- 闲聊问答

建议：

- 提高 `backchannel_likelihood` 权重
- 提高 `output_phase` 的作用
- 延长 `W_accumulate`
- 不要太早升级 `hard_interrupt`

原因：

- 用户更可能出现附和
- 一有声就停会显得很不自然

## 3. 中文短命令场景

建议：

- 弱化 `min_words` 逻辑
- 强化字级/短前缀稳定度
- 强化 correction lexicon
- 缩短 `duck_only` 到 `hard_interrupt` 的升级路径

原因：

- 中文里很多 takeover 只要 1 个词甚至 1 个字节级稳定前缀就够了

## 4. 高风险操作场景

例如：

- 删除
- 付款
- 桌面高风险自动化

建议：

- 对 `hard_interrupt` 更敏感
- 对 `execute` 更保守
- 允许更早停止当前输出
- 但不允许仅凭 preview 就执行新动作

## 研究阶段最不该过早引入的信号

### 1. 过重的多模态 directedness 模型

原因：

- 训练/标注成本高
- 当前项目研究阶段 ROI 不高

### 2. 复杂的个性化用户建模

例如：

- 用户个人 backchannel 习惯长期建模
- 个性化 prosody/user-profile 驱动 interruption

原因：

- 很容易把研究范围拖大

### 3. 自由 LLM 直接参与 interruption 最终裁决

原因：

- 不稳定
- 延迟和可解释性都较差
- 当前阶段更适合把 LLM 放在后验解释或草案层，而不是主裁判层

### 4. 过度依赖 `min_words`

特别对中文：

- `停`
- `不是`
- `等等`

这类都不能靠英文式 `min_words` 思维来处理

## 对当前 `agent-server` 最值得借鉴的边界

### 1. 让 `duck_only` 由“入场分 + 确认分”驱动

这是最适合当前项目的表达，原因是：

- 与现有 `previewing`、`duck_only`、`hard_interrupt` runtime-internal 设计相容
- 不要求端侧协议立刻扩张
- 能把 preview partial 真正纳入仲裁主链

### 2. 优先把 stable prefix 纳入确认分，而不是把整个 partial 尾巴直接纳入

- 这样更稳
- 也更适合中文短命令场景

### 3. `duck_only` 应是短时态，不应无限停留

- 到达 `W_escalate_max` 后应强制收敛
- 否则系统会显得犹豫、拖沓

## 参考资料

- OpenAI Realtime VAD：<https://platform.openai.com/docs/guides/realtime-vad>
- LiveKit Turns：<https://docs.livekit.io/agents/build/turns/>
- Pipecat User Turn Strategies：<https://docs.pipecat.ai/api-reference/server/utilities/turn-management/user-turn-strategies>
- Pipecat Interruption Strategies：<https://docs.pipecat.ai/api-reference/server/utilities/turn-management/interruption-strategies>
- Deepgram Flux configuration：<https://developers.deepgram.com/docs/flux/configuration>
- Deepgram Eager End of Turn：<https://developers.deepgram.com/docs/flux/voice-agent-eager-eot>
- AssemblyAI Turn Detection：<https://www.assemblyai.com/docs/universal-streaming/turn-detection>
- Apple Talking Turns：<https://machinelearning.apple.com/research/talking-turns>
- Amazon Contextual Acoustic Barge-In Classification：<https://assets.amazon.science/56/4a/d81efc094934a2fdafdbe03d63f0/contextual-acoustic-barge-in-classification-for-spoken-dialog-systems.pdf>
- Amazon Accurate Endpointing with Expected Pause Duration：<https://www.amazon.science/publications/accurate-endpointing-with-expected-pause-duration>
- Google Unified End-to-End Speech Recognition and Endpointing：<https://research.google/pubs/unified-end-to-end-speech-recognition-and-endpointing-for-fast-and-efficient-speech-systems/>
- Google Low Latency Speech Recognition using End-to-End Prefetching：<https://research.google/pubs/low-latency-speech-recognition-using-end-to-end-prefetching/>
