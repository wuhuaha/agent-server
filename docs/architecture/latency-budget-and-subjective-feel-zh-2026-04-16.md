# 端到端时延预算与主观体感映射（2026-04-16）

## 文档性质

- 本文是研究讨论材料，不是最终实施方案。
- 目标不是简单回答“总时延要多低”，而是回答：
  - 在实时语音交互里，**哪些时延节点决定用户主观体感**；
  - 当前项目应该把优化预算优先压在哪些节点；
  - 为什么“平均总耗时更短”并不一定等价于“用户感觉更快”。
- 当前阶段仍以：
  - 流畅性
  - 自然性
  - 人性化
  - 智能性
  为主要目标。

## 一句话结论

**实时语音体验不该只盯“总响应时延”，而应该拆成一张“里程碑时延预算表”。**

对用户体感影响最大的，通常不是单一的 `end_to_end_ms`，而是这几个分段时刻：

1. `speech_start visible`：系统有没有立刻表现出“我在听”
2. `first preview partial`：系统有没有尽早表现出“我开始懂了”
3. `endpoint accept`：系统有没有自然地判断“你说完了”
4. `response.start / first text draft`：系统有没有快速表现出“我开始想了”
5. `first audio byte / first audible syllable`：系统有没有及时“开口”
6. `barge-in cutoff`：系统会不会在你插话时快速让话

因此，当前项目更该建立的是：

- **分阶段预算**，而不是一个总时延 KPI
- **体感映射**，而不是只看 ASR/LLM/TTS 单模块 benchmark
- **场景化目标**，而不是所有 query 用同一阈值

## 为什么这比“总时延”更关键

### 1. 用户不会直接感知系统内部模块边界

用户感知到的不是：

- ASR 用了多少毫秒
- LLM 首 token 用了多少毫秒
- TTS 合成耗时是多少

用户感知到的是：

- “它是不是马上在听我”
- “它是不是已经懂了我大概在说什么”
- “它是不是知道我说完了”
- “它是不是迟迟不开口”
- “我插话时它是不是还在硬说”

这意味着：

- 某些 150ms 的优化几乎无感
- 某些 120ms 的优化会显著改变主观体验

### 2. 同样的总时延，不同分布，体感完全不同

举例：

- 方案 A：前 700ms 完全无反馈，然后一次性给完整回复
- 方案 B：100ms 内有 listening cue，300ms 内有 preview，700ms 内起播首句

即使两者最终“完整答完”的总时间相近，方案 B 也通常更像在“对话”，而不是在“等系统算完”。

### 3. 语音交互天然受人类 turn-taking 节奏约束

人类对话里的 turn gap 极短。多篇 turn-taking 研究和综述都反复指出：

- 自然人际对话中，turn 间隙的众数大约只有 `200ms` 左右；
- 明显更长的响应停顿会被感知为：犹豫、关系疏离、系统笨重，或者没听懂。

这不意味着语音 agent 必须稳定做到 `200ms` 内回答完整内容；但它意味着：

- **至少要在更短时间里给出“在听/在理解/在准备回答”的信号**；
- 若要做到更像真人，系统需要把长耗时隐藏在“可见、可听、可逆的中间反馈”之后。

## 外部资料给出的关键启发

### Google：真正影响用户感知的，不只是模型算得多快，而是 token emission 和 endpointing

`Dissecting User-Perceived Latency of On-Device E2E Speech Recognition` 讨论了 `UPL`（user-perceived latency）问题。其核心启发非常重要：

- 模型大小、FLOPS、RTF，并不总能很好预测用户真正感到的延迟；
- 更直接影响体感的，是：
  - token emission latency
  - endpointing behavior

这与当前项目的直觉完全一致：

- 如果 preview partial 出不来，用户会觉得“系统没开始懂”
- 如果 endpoint accept 过慢，用户会觉得“系统不知道我说完了”
- 如果 TTS 起播太晚，用户会觉得“系统想太久”

参考：

- <https://www.isca-archive.org/interspeech_2021/shangguan21_interspeech.html>
- <https://arxiv.org/abs/2104.02207>

### Google：partial-driven prefetch 可以直接减少大约 200ms 级系统时延

`Low Latency Speech Recognition using End-to-End Prefetching` 的关键点是：

- 不要等 final 结果才启动下游；
- 可以基于 partial hypothesis 提前预取后续响应；
- 在实验中可带来约 `200ms` 的系统级收益；
- 与 silence-only 的预取相比，end-to-end prefetching 在固定预取率下还能再快约 `100ms`。

这非常支持当前项目继续把：

- `preview partial`
- `stable prefix`
- `early processing`

从“显示层优化”升级为“主链路时延优化手段”。

参考：

- <https://research.google/pubs/low-latency-speech-recognition-using-end-to-end-prefetching/>

### Google：endpointing 如果和 ASR 表征联合建模，可以明显减少尾部等待

`Unified End-to-End Speech Recognition and Endpointing for Fast and Efficient Speech Systems` 表明：

- endpointing 不应只是外接的小阈值器；
- 联合建模可把 median endpoint latency 降低约 `130ms`，90 分位降低约 `160ms`，同时不牺牲 WER。

这说明：

- 对语音 agent 而言，“我什么时候认为你说完了”本身就是时延主战场；
- 把预算只压在 LLM/TTS 上是不够的；
- 当前项目继续深挖 `server endpoint + semantic/lexical completeness` 是高 ROI 的。

参考：

- <https://research.google/pubs/unified-end-to-end-speech-recognition-and-endpointing-for-fast-and-efficient-speech-systems/>

### Amazon：predictive ASR 的思想说明“结束前准备”是值得的

Amazon 的 `Personalized Predictive ASR for Latency Reduction in Voice Assistants` 继续证明了另一个方向：

- 某些 query 可以在用户真正说完之前，预测完整语句并提前准备响应；
- 这类“结束前准备”在一部分 utterances 上能够带来实实在在的 latency reduction；
- 个体化/场景化建模对这类提早预测很重要。

这与我们之前讨论的：

- `stable prefix`
- `utterance completeness`
- `slot completeness`
- `UEPG`

之间是同一条主线：**越早、越稳、越可逆地开始后续处理，体感越好。**

参考：

- <https://www.amazon.science/publications/personalized-predictive-asr-for-latency-reduction-in-voice-assistants>
- <https://assets.amazon.science/7f/99/6ecd06364b04b00110b8d601febd/personalized-predictive-asr-for-latency-reduction-in-voice-assistants.pdf>

### Deepgram：Eager EOT 本质上就是“先准备，再确认”

Deepgram Flux 把这一思路做得更显式：

- `EagerEndOfTurn`：中等置信度地认为用户大概率说完了，先开始准备下游
- `TurnResumed`：如果用户继续说，就撤销前面的准备
- `EndOfTurn`：高置信最终确认

Deepgram 还明确指出：

- 这种 eager 处理适合 latency-sensitive voice agent；
- 常见收益是再压掉最后 `100-200ms` 的体感时延；
- 代价是更多的 LLM 调用和更多中途撤销。

这与当前项目研究阶段非常契合，因为我们关心的不是“零错误的最慢提交”，而是“快且可修正”。

参考：

- <https://developers.deepgram.com/docs/flux/voice-agent-eager-eot>
- <https://developers.deepgram.com/docs/flux/agent>

### 人类 turn-taking：200ms 级自然 gap 说明“及时反馈”比“完整正确后再说”更重要

turn-taking 研究广泛指出：

- 自然对话中 turn gap 常见在 `0-300ms`，众数约 `200ms`；
- 这要求说话人一边听，一边提前计划下一轮；
- 更慢的响应常常被感知为犹豫、迟疑、关系疏离，或者理解出了问题。

对于 agent，这个结论最重要的含义不是“必须 200ms 内答完”，而是：

- 必须尽快把系统内部的后续工作隐藏到用户能感知到的连续交互节奏之后。

参考：

- <https://pmc.ncbi.nlm.nih.gov/articles/PMC10321606/>
- <https://pmc.ncbi.nlm.nih.gov/articles/PMC4357202/>
- <https://pmc.ncbi.nlm.nih.gov/articles/PMC11132552/>

## 当前项目最该建立的，不是单指标，而是一张里程碑预算表

下面这张表是我基于：

- 上述外部资料
- 当前项目现状
- 研究阶段的 demo 目标

做出的**研究性预算建议**。

注意：这些数字不是最终 SLA，而是“体感导向”的优先级参考。

| 里程碑 | 用户主观感受 | 研究阶段建议目标 | 备注 |
| --- | --- | --- | --- |
| `speech_start visible` | “它开始听我了” | `<= 120ms` 自首帧近端语音起 | 可以是 preview speech start、UI/listening cue、soft duck |
| `first preview partial` | “它开始懂我了” | `<= 300-450ms` 自起说 | 对短命令最好压到更前；长句允许稍慢 |
| `stable preview / intent cue` | “它基本知道我要说什么” | `<= 500-800ms` 自起说 | 对 early processing 很关键 |
| `endpoint candidate` | “它知道我说完了” | `<= 150-300ms` 自真实句末后（短命令） | 问答、长句可略放宽 |
| `turn accept / response.start` | “它开始处理了” | `<= 80-180ms` 自 endpoint candidate 后 | 若走 eager 路径，可在 final 前先预热 |
| `first audio byte` | “它要开口了” | `<= 250-450ms` 自 accept 后 | 若已有 planner/TTS 预热，可继续压缩 |
| `first audible syllable` | “它真的在回答我” | `<= 450-800ms` 自句末后 | 这是最核心的体感点之一 |
| `barge-in cutoff` | “它会让话” | `<= 120-220ms` 自确认插话后 | 对全双工自然性极关键 |

## 为什么这些预算是分场景的，而不是统一阈值

### 1. 短命令 / 确定性问答

例如：

- `明天周几`
- `打开客厅灯`
- `现在几点`

特点：

- 句子通常短
- 完整度判断更容易
- 允许更激进地 endpoint 与 early processing

因此最该压的是：

- `endpoint candidate`
- `turn accept`
- `first audible syllable`

### 2. 开放问答 / 长解释

例如：

- `你帮我分析一下这个问题`
- `为什么最近老是感觉很累`

特点：

- 输入可能更长
- 句末不那么容易预测
- 回答本身也长

因此最该压的是：

- `speech_start visible`
- `first preview partial`
- `response.start`
- `TTS 首句起播`

在这类场景中，用户更容易接受“完整答完较晚”，但不接受“前面长时间毫无反馈”。

### 3. 纠错 / 插话 / 打断

例如：

- `不是，我是说明天`
- `等一下`
- `停`

特点：

- 用户对系统让话能力极敏感
- 错过 200ms 往往体感就明显变差

因此最该压的是：

- `speech_start during speaking`
- `barge-in cutoff`
- `duck_only enter`
- `hard interrupt decision`

## 更适合当前项目的“体感映射表”

### 用户说“系统像没在听”

优先怀疑：

- `speech_start visible` 太晚
- `first preview partial` 没下发到端侧
- 端侧本地 listening cue 不够早

### 用户说“系统反应慢，但看日志总耗时不算太离谱”

优先怀疑：

- `endpoint accept` 太晚
- `first audio byte` 太晚
- `first audible syllable` 被播放缓冲或 TTS 起播拖住

### 用户说“它老是等我说完很久才接话”

优先怀疑：

- endpoint policy 太保守
- lexical/semantic completeness 没进入主判据
- 还在过度依赖 silence window

### 用户说“它不让我插话 / 抢话很严重”

优先怀疑：

- `duck_only` 入场太慢
- `hard interrupt` 判定太晚
- 输出轨和输入轨还没有足够并行

## 对当前项目的最重要结论

### 1. 当前主线最值得优先压的，不是“完整回答结束时间”

更值得优先压的是：

- `preview first partial`
- `endpoint accept`
- `first audio byte`
- `first audible syllable`
- `barge-in cutoff`

### 2. 现在的 tracing 已经有不错基础，但还差“更靠近体感”的最后几步

仓库当前已经具备较好的 phase tracing 基础，包括：

- `preview_speech_start_latency_ms`
- `preview_first_partial_latency_ms`
- `preview_endpoint_candidate_latency_ms`
- `preview_commit_suggest_latency_ms`
- `first_text_delta_latency_ms`
- `first_audio_chunk_latency_ms`
- `speaking_latency_ms`

这很好，但对体感来说仍建议后续继续补：

- `first_audible_playout_latency_ms`
- `barge_in_cutoff_latency_ms`
- `heard_text_ratio`
- `preview_to_accept_ms`
- `accept_to_first_audible_ms`

### 3. “总时延下降”不应成为唯一成功标准

更适合当前项目研究阶段的成功标准是：

- 用户是否更早感知到“系统在听”
- 用户是否更早看到/听到“系统开始理解”
- 用户是否更少觉得“系统不知道我说完了”
- 用户是否能在 speaking 时自然插话
- 用户是否主观觉得“对话节奏更像真人”

### 4. 早处理链路的价值，应该按里程碑拆开评估

也就是说后续如果继续优化：

- preview partial 早下发
- semantic early processing
- endpointing
- planner/TTS 早起播

不应只问“总共快了多少”，而应该问：

- 哪个里程碑前移了？
- 前移之后体感有没有提升？
- 是否引入了更多 false start / false endpoint / false interrupt？

## 当前研究阶段最建议记住的三句话

1. **用户先感知到的是连续对话节奏，不是模块 benchmark。**
2. **最该优化的是关键里程碑时刻，而不是平均总耗时。**
3. **语音 agent 的“快”，很多时候来自更早反馈、更早准备、更早起播，而不是等全部算完。**

## 相关本地文档

- `docs/architecture/remaining-critical-analysis-topics-zh-2026-04-16.md`
- `docs/architecture/streaming-final-asr-semantic-early-processing-zh-2026-04-16.md`
- `docs/architecture/unified-early-processing-threshold-zh-2026-04-16.md`
- `docs/architecture/server-driven-turn-taking-vs-client-commit-zh-2026-04-16.md`
- `docs/architecture/local-open-source-full-duplex-roadmap-zh-2026-04-10.md`

## 参考资料

- Google, `Low Latency Speech Recognition using End-to-End Prefetching`:
  - <https://research.google/pubs/low-latency-speech-recognition-using-end-to-end-prefetching/>
- Google, `Unified End-to-End Speech Recognition and Endpointing for Fast and Efficient Speech Systems`:
  - <https://research.google/pubs/unified-end-to-end-speech-recognition-and-endpointing-for-fast-and-efficient-speech-systems/>
- Amazon, `Personalized Predictive ASR for Latency Reduction in Voice Assistants`:
  - <https://www.amazon.science/publications/personalized-predictive-asr-for-latency-reduction-in-voice-assistants>
  - <https://assets.amazon.science/7f/99/6ecd06364b04b00110b8d601febd/personalized-predictive-asr-for-latency-reduction-in-voice-assistants.pdf>
- Deepgram, `Optimize Voice Agent Latency with Eager End of Turn`:
  - <https://developers.deepgram.com/docs/flux/voice-agent-eager-eot>
- Deepgram, `Build a Flux-enabled Voice Agent`:
  - <https://developers.deepgram.com/docs/flux/agent>
- Shangguan et al., `Dissecting User-Perceived Latency of On-Device E2E Speech Recognition`:
  - <https://www.isca-archive.org/interspeech_2021/shangguan21_interspeech.html>
  - <https://arxiv.org/abs/2104.02207>
- turn-taking / response timing studies:
  - <https://pmc.ncbi.nlm.nih.gov/articles/PMC10321606/>
  - <https://pmc.ncbi.nlm.nih.gov/articles/PMC4357202/>
  - <https://pmc.ncbi.nlm.nih.gov/articles/PMC11132552/>
