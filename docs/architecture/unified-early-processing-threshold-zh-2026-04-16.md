# 统一“早处理门槛”研究：stable prefix + utterance completeness + slot completeness（2026-04-16）

## 文档性质

- 本文是研究讨论材料，不是最终实施方案。
- 目标是回答一个非常关键的问题：
  - `stable prefix + utterance completeness + slot completeness` 能不能形成一个统一的“早处理门槛”？
- 当前项目处于研究阶段，重点是提升实时性、自然性、智能性，而不是先把架构做重。

## 一句话结论

**可以形成“统一门槛”，但不应理解为一个单一硬阈值或单一分数。**

更合理的表达是：

- 它们可以形成一个统一的 `Early Processing Gate` 决策对象
- 但这个对象最好是 **分层门槛 / 向量门槛 / 策略门槛**，而不是一个简单的 scalar score

换句话说：

- `stable prefix` 解决“文本是否稳定到值得拿来用”
- `utterance completeness` 解决“语义上是否像是已经说完或足够成句”
- `slot completeness` 解决“对于命令/工具调用，执行参数是否真的齐了”

三者相关，但并不等价。

因此最值得借鉴的方向不是：

- `一个分数 >= 0.8 就全部提前提交`

而是：

- `一个统一门槛对象，根据不同动作类型触发不同层级的早处理动作`

## 为什么这三者可以统一，但不该硬合并

## 1. 三者分别回答的是三个不同问题

### `stable prefix`

它回答的是：

- 当前 partial 里，哪一部分已经足够稳定，不容易在下一次流式更新里被推翻？

本质是：

- **文本稳定性**
- 更偏 ASR / partial convergence 信号

### `utterance completeness`

它回答的是：

- 从语义或对话结构上看，这句话是否已经“足够完备”，值得开始下游处理？

本质是：

- **语义闭合度**
- 更偏 turn-taking / semantic endpointing / intent closure 信号

### `slot completeness`

它回答的是：

- 对于命令/工具/agent action，这轮输入是不是已经把关键参数说全了？

本质是：

- **可执行性**
- 更偏 NLU / tool planning / slot filling 信号

## 2. 它们彼此相关，但失配很常见

### 情况 A：`stable prefix` 高，但 `utterance completeness` 低

例如：

- `把客厅灯调到...`

前半句可能非常稳定，但用户显然还没说完。

### 情况 B：`utterance completeness` 高，但 `slot completeness` 低

例如：

- `帮我打开那个`

语调可能像已经说完，但实体槽缺失。

### 情况 C：`stable prefix` 和 `utterance completeness` 都高，但 `slot completeness` 仍需谨慎

例如：

- `提醒我明天早上`

意图清楚，但时间槽不完整；对 assistant 来说不宜直接提交。

### 情况 D：对于问答类任务，`slot completeness` 本来就不是核心约束

例如：

- `明天周几`
- `上海天气怎么样`

这类 query 更适合关注：

- 是否语义闭合
- 是否已经可回答

因此，`slot completeness` 应是 task-aware 的，而不应一刀切。

## 3. 所以最合理的统一方式，是“统一门槛对象 + 分层动作”

也就是说：

- 在表述上统一
- 在执行上分层

这是比“合成一个总分数”更稳、更现代的方式。

## 外部资料为什么支持这种判断

### OpenAI：语义完备优先于纯静音完备

OpenAI `semantic_vad` 的方向非常明确：

- 并不是只看静音长度
- 而是看“用户是否已经完成一个 thought”
- `eagerness` 还允许在不同场景下调得更快或更保守

这本质上说明：

- `utterance completeness` 本来就应该进入主门槛

参考：

- <https://platform.openai.com/docs/guides/realtime-vad>

### Google：partial hypothesis 可以提前驱动下游

Google 的 `Low Latency Speech Recognition using End-to-End Prefetching` 明确支持：

- 在 final 识别之前利用 partial hypothesis 预触发下游
- 系统级延迟可获得收益

这意味着：

- `stable prefix` 不应只用来显示字幕
- 它应该进入早处理门槛

参考：

- <https://research.google/pubs/low-latency-speech-recognition-using-end-to-end-prefetching/>

### Google：endpointing 与 ASR 表征应融合，而不是分裂

Google 的 unified endpointing 说明：

- 端点检测不该只是 ASR 外面的一个小阈值器
- 需要更深地利用识别过程本身的信息

这进一步支持：

- `stable prefix + utterance completeness` 应协同，而不是各自为政

参考：

- <https://research.google/pubs/unified-end-to-end-speech-recognition-and-endpointing-for-fast-and-efficient-speech-systems/>

### Deepgram / AssemblyAI：早猜测 + 后确认是主流，而不是最终式 one-shot

- Deepgram Flux 提供 `EagerEndOfTurn` 与 `TurnResumed`
- AssemblyAI turn detection 也是 semantic + acoustic 协同，而不是纯静音

这说明：

- 现代系统已经默认接受“先做可逆猜测，再做后确认”
- 这非常适合统一门槛对象的设计

参考：

- <https://developers.deepgram.com/docs/flux/voice-agent-eager-eot>
- <https://www.assemblyai.com/docs/universal-streaming/turn-detection>

### 增量 SLU 研究：intent 往往能比 slot 更早稳定

增量 SLU / partial ASR NLU 相关研究长期都在指出：

- 部分语音上就能形成初步 intent
- 但 slot，尤其是实体槽、数字槽、时间槽，往往更晚稳定

这恰好说明：

- `stable prefix`
- `utterance completeness`
- `slot completeness`

很适合统一到一个对象里，但不适合压成一个单值。

参考：

- <https://aclanthology.org/N09-2014/>
- <https://www.ijcai.org/proceedings/2021/538>

### 当前项目本地架构也支持这条方向

当前仓库已经具备很好的研究土壤：

- `workers/python/README.md:95` 起：支持 `stream_2pass_online_final`
- `docs/architecture/local-funasr-asr.md:18` 起：已有 `online preview + final-ASR correction` 明确表达
- `docs/architecture/funasr-model-selection-zh-2026-04-14.md:151` 起：已经把真 streaming preview + final correction 视为最契合当前主线的方向

这意味着：

- 当前项目已经具备把“统一早处理门槛”做成 voice orchestration 中心信号的基础

## 更合理的结论：不是“单一分数”，而是“分层门槛”

我建议把统一门槛写成一个对象，而不是一个数。

例如：

```text
EarlyProcessingGate {
  prefix_stability        // 文本稳定度
  utterance_completeness  // 语义/句法/turn 完备度
  slot_completeness       // 动作所需槽位完备度
  correction_risk         // 后续自我纠正/补充的风险
  action_risk             // 当前动作的风险等级
}
```

然后用这个对象决定不同层级动作。

## 推荐的三层门槛

### Gate A：`preview-ready`

作用：

- 允许端侧展示 preview
- 允许进入 `previewing`
- 允许做 prewarm
- 允许 `duck_only` 的前置信号进入仲裁器

主要依赖：

- 中低门槛的 `stable prefix`
- 初步的 `utterance completeness`
- 几乎不依赖 `slot completeness`

### Gate B：`draft-ready`

作用：

- 允许开始 draft response
- 允许开始 tool planning candidate
- 允许为低风险问答生成首句草稿
- 允许 speculative execution planning，但不真正提交

主要依赖：

- 更强的 `stable prefix`
- 中高门槛的 `utterance completeness`
- 命令类场景下，需要至少基本可用的 `slot completeness`

### Gate C：`commit-ready`

作用：

- 允许高置信 early response start
- 允许真正工具调用或设备执行
- 允许高风险动作提交

主要依赖：

- 高 `stable prefix`
- 高 `utterance completeness`
- 高 `slot completeness`
- 低 `correction_risk`
- 并受 `action_risk` 策略额外约束

## 这三个指标分别应该如何理解

## A. `stable prefix`

### 更适合前通道

前通道最适合用的信号包括：

- 连续多次 partial 更新中保持不变的前缀长度
- 尾部 edit distance 是否快速收敛
- partial token / word 是否被标成 final-ish / stable-ish
- 是否已经跨过几个 chunk 仍不变

### 不适合直接当 final truth

因为它仍会受到：

- 句尾补充
- 自我纠正
- 中英混说
- 实体尾部替换

的影响。

## B. `utterance completeness`

### 更适合前通道参与“早处理”

它最适合用来决定：

- 是否开始草拟 response
- 是否开始工具候选推理
- 是否开始 TTS 首句规划
- 是否从 `duck_only` 向更强仲裁推进

适合的信号包括：

- 问句/命令句是否闭合
- 语调/停顿是否像句末
- 是否出现明显 continuation cue
- 是否已经能形成 clear intent hypothesis

### 不应单独决定执行

因为语义看似闭合，不代表参数已经全。

## C. `slot completeness`

### 更适合后确认或命令型场景强约束

它非常适合用来限制：

- 智能家居动作执行
- 桌面控制
- 外部工具调用
- 任何不可逆提交

典型信号：

- 必填槽是否齐全
- 值槽是否已归一化
- 实体是否已完成 alias / catalog 对齐
- 数字/时间/单位是否稳定

### 对问答类场景可以弱化

例如：

- `明天周几`
- `上海天气怎么样`

slot completeness 不是主门槛，更多看 `utterance completeness` 是否足够。

## 所以到底能不能形成一个统一“早处理门槛”？

我的最终答案是：

- **可以，但应统一成“决策对象”而不是“硬阈值公式”。**

如果一定要进一步压缩，我更推荐：

- `单一策略对象`
- `多层动作门槛`
- `任务类型感知`
- `风险等级感知`

而不是：

- `alpha * stable_prefix + beta * utterance + gamma * slot >= T`

因为这种纯线性单分数太容易掩盖失配：

- 高 prefix + 低 slot
- 高 utterance + 高 correction risk
- 低 prefix 但强 takeover / stop 语义

都可能被简单加权搞错。

## 我更推荐的研究阶段表达：`UEPG` 模型

可以先只把它作为研究阶段术语讨论：

```text
UEPG = Unified Early Processing Gate

Inputs:
- prefix_stability
- utterance_completeness
- slot_completeness
- correction_risk
- action_risk
- turn_type   // 问答型 / 命令型 / 控制型 / 澄清型

Outputs:
- prewarm_allowed
- draft_allowed
- early_tts_allowed
- execute_allowed
- needs_confirmation
```

这比“统一成一个分数”更像一个真正可落地的现代实时系统表达。

## 前通道与后确认，更适合放什么信号

## 更适合前通道的

- prefix stability
- semantic closure / utterance completeness
- 低风险 intent guess
- 初步 slot candidate
- 轻量 actionability 估计
- correction cue 的 early detection

这些适合驱动：

- preview
- prewarm
- draft response
- tool planning candidate
- duck_only / interruption arbitration

## 更适合后确认的

- 完整实体归一化
- 数字/时间/百分比最终 normalization
- 高风险动作执行判定
- 完整 catalog disambiguation
- final-ASR 纠正结果
- 需要强一致性的 memory 写入

这些更适合驱动：

- execute
- persistent commit
- external side effect

## 对智能家居 / 桌面助理特别要注意什么

### 1. `intent` 往往比 `slot` 更早稳定

例如：

- `打开客厅灯`
- `关闭投影`
- `把空调调到二十六度`

动作意图往往很早就能猜到，但实体和值槽经常在后半句。

所以：

- 可以早猜 intent
- 但不应早执行 action

### 2. 尾部 slot 经常决定执行是否正确

例如：

- `把客厅灯调到三十`
- `提醒我明天早上`
- `打开卧室灯，不对，客厅灯`

这些都说明：

- `slot completeness` 对命令型场景是硬约束
- 特别是数值槽、时间槽、实体槽

### 3. 中文短命令很容易“意图早、参数晚”

- `开灯`
- `静音`
- `暂停`
- `别播了`

这类短命令如果动作低风险，可更早进入 commit-ready；但一旦有实体或参数尾部，则应立即抬高门槛。

### 4. correction risk 必须单独进门槛对象

一旦出现：

- 不是
- 等等等等
- 我是说
- 改成
- 还有

即使 `prefix_stability` 和 `utterance_completeness` 已经很高，也应降低 early commit。

## 这条路最容易踩的坑

### 1. 把三者硬压成一个总分数

看起来简单，但很容易掩盖关键失配。

### 2. 把问答和命令共用一套门槛

问答类更适合早起播；命令类更适合早规划、晚执行。

### 3. 没有 `correction_risk`

这会导致系统太容易被尾部修正打脸。

### 4. 没有 task / risk aware policy

不同动作的提交门槛必须不同。

### 5. 让不稳定尾巴直接污染下游

应优先让 stable prefix 参与 deeper processing，而不是整个 partial 尾巴一股脑送进工具/回复。

## 对当前 `agent-server` 最值得借鉴的边界

## 1. 先把“统一门槛”作为 runtime 内部对象，而不是公开协议字段

这更符合当前阶段：

- 可快速试验
- 不会过早固化 wire contract
- 便于把 preview / endpoint / duck_only / prewarm / early draft 串起来

## 2. 先让它驱动“可逆动作”，不要一上来驱动不可逆动作

最适合先接入的动作为：

- preview visible
- prewarm
- draft response
- speculative tool planning
- early TTS first clause candidate

## 3. 真正高风险提交仍应晚一步

尤其是：

- 智能家居控制
- 桌面自动化
- 对外部系统写入

## 4. 更适合当前项目的最短表达

如果必须用一句最短的话概括：

- **可以统一，但应统一为“分层决策门槛”，而不是“一个总分数”。**

## 参考资料

- OpenAI Realtime VAD：<https://platform.openai.com/docs/guides/realtime-vad>
- Google Low Latency Speech Recognition using End-to-End Prefetching：<https://research.google/pubs/low-latency-speech-recognition-using-end-to-end-prefetching/>
- Google Unified End-to-End Speech Recognition and Endpointing：<https://research.google/pubs/unified-end-to-end-speech-recognition-and-endpointing-for-fast-and-efficient-speech-systems/>
- Deepgram Eager End of Turn：<https://developers.deepgram.com/docs/flux/voice-agent-eager-eot>
- Deepgram Flux Agent：<https://developers.deepgram.com/docs/flux/agent>
- AssemblyAI Turn Detection：<https://www.assemblyai.com/docs/universal-streaming/turn-detection>
- FunASR：<https://github.com/modelscope/FunASR>
- 当前项目 `workers/python/README.md`
- 当前项目 `docs/architecture/local-funasr-asr.md`
- 当前项目 `docs/architecture/funasr-model-selection-zh-2026-04-14.md`
- Towards Natural Language Understanding of Partial Speech Recognition Results in Dialogue Systems：<https://aclanthology.org/N09-2014/>
- A Streaming End-to-End Framework for Spoken Language Understanding：<https://www.ijcai.org/proceedings/2021/538>
