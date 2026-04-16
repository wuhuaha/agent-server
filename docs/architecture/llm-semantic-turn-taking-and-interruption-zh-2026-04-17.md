# LLM 参与实时 turn-taking 与 interruption 的落地方案（2026-04-17）

## 文档定位

- 性质：研究结论 + 当前项目落地方案
- 目标：回答一个非常具体的问题——如何让本项目的“语句是否完成”“是否应该 hard interrupt / duck_only / backchannel”不再主要依赖规则，而是开始真正利用 LLM 的语义能力
- 范围：仅讨论服务侧 shared voice runtime，不把决策权重新下放到 client / adapter

## 一句话结论

本项目不适合把 turn-taking / interruption 直接改成“纯 LLM 判定”，但非常适合改成：

- **规则底座保实时安全**
- **LLM 做语义裁判**
- **LLM 结果只做 advisory / 可撤销加权**

也就是：

`acoustic + timing heuristics` 负责快、稳、可兜底，`LLM semantic judge` 负责补“是否像一句完整话”“这是 backchannel 还是 takeover”“这是补充还是改口”这些规则最薄弱的部分。

## 1. 当前项目现状复核

结合当前代码，当前两条核心链路都仍然主要是规则型：

### 1.1 语句是否完成

当前主判断链在：

- `internal/voice/turn_detector.go`

核心依据仍然是：

- `stable_prefix`
- `stability`
- `stable_for_ms`
- `audio_ms`
- `silence_ms`
- `endpoint_hint`
- `looksLexicallyComplete(...)`
- `looksCorrectionPending(...)`

当前项目虽然已经有 `prewarm_allowed / draft_allowed / accept_candidate / accept_now` 分层，但这些层仍主要建立在启发式仲裁上。

### 1.2 是否打断

当前主判断链在：

- `internal/voice/barge_in.go`

核心依据仍然是：

- `audioMs`
- `looksLikeBackchannel(...)`
- `looksLikeTakeoverLexicon(...)`
- `looksLexicallyComplete(...)`
- `AcceptCandidate / AcceptNow`
- 若干 score-based heuristic

这条链已经比最早版本强很多，但“semantic”仍主要是规则拼装，不是模型语义判断。

## 2. 外部实践给出的直接启发

下面只列与本项目最契合、且能直接影响落地设计的结论。

### 2.1 OpenAI：turn detection 不应只有静音版，还应有 semantic_vad

OpenAI Realtime 官方文档已经把 turn detection 区分为：

- `server_vad`
- `semantic_vad`

这件事对本项目的含义非常直接：

- “用户停没停”与“这句话该不该开始处理”不是同一个问题
- 当前项目已经有 `server_vad_assisted` 的共享 runtime 路径
- 下一步不是再堆更多尾词规则，而是补一个真正的 semantic judge

参考：

- OpenAI Realtime VAD guide：<https://platform.openai.com/docs/guides/realtime-vad>

### 2.2 Google：endpointing 是独立建模问题，不能只调 silence threshold

Google 的统一识别+endpointing工作说明：

- endpointing 应与语音识别一起建模
- 更早结束与更准结束之间需要平衡
- 这件事本质上不是“静音阈值调多少”

而 turn-taking 研究也在强调：

- 实时系统需要利用更丰富的上下文信号，而不是只用一个局部 pause

这与本项目当前 `stable_prefix + silence + endpoint_hint` 的路线一致，但也说明仅靠规则会很快到上限。

参考：

- Unified End-to-End Speech Recognition and Endpointing for Fast and Efficient Speech Systems：<https://research.google/pubs/unified-end-to-end-speech-recognition-and-endpointing-for-fast-and-efficient-speech-systems/>
- A Turn-Taking Model for Spoken Dialog Systems Using Multiplexed Prediction：<https://www.isca-archive.org/interspeech_2022/chang22_interspeech.html>

### 2.3 Amazon：endpointing / natural turn-taking 需要深上下文，不是单规则

Amazon 的官方研究和工程文章非常一致：

- endpointing 需要上下文
- 更自然的 turn-taking 不是单阈值问题
- 实时语音系统应该允许更智能的动态判定，而不是只靠固定静音窗口

这支持本项目把 LLM 放到“语义裁判”位置，而不是继续把所有语义判断塞进 backchannel token / takeover lexicon。

参考：

- Adaptive Endpointing with Deep Contextual Multi-Armed Bandits：<https://www.amazon.science/publications/adaptive-endpointing-with-deep-contextual-multi-armed-bandits>
- Change to Alexa wake-word process adds natural turn-taking：<https://www.amazon.science/blog/change-to-alexa-wake-word-process-adds-natural-turn-taking>

### 2.4 Apple：LLM 更适合作为受约束的辅助裁判，而不是放飞式替代器

Apple 在 ASR contextualization with LLM 的公开研究里强调：

- LLM 可以提供强语义补偿
- 但要通过受约束的检索/候选机制来使用
- 不要让 LLM 自由改写整条主链

虽然那篇工作更偏 ASR contextualization，但对本项目当前问题非常有借鉴意义：

- 我们要利用 LLM 的语义能力
- 但最好只让它输出结构化小结论，而不是直接替代 runtime

参考：

- Contextualization of ASR with LLM Using Phonetic Retrieval-Based Augmentation：<https://machinelearning.apple.com/research/asr-contextualization>
- Follow-up Voice Search with a Natural Language Understanding Augmented Multi-Turn Dialog System：<https://machinelearning.apple.com/research/follow-up-voice-search>

### 2.5 FunASR：2-pass 说明“快路径 + 强语义补偿”本来就是合理方向

FunASR 官方路线一直是：

- streaming fast path
- 2-pass correction
- VAD / KWS / hotword 等可组合

这与本项目当前结构高度兼容。换句话说，本项目现在最值得做的不是推翻 cascade，而是把 “2-pass 的思想” 从 ASR 层继续推广到 voice orchestration 层：

- first pass：声学/规则/preview
- second pass：LLM 语义裁判

参考：

- FunASR 官方 README（包含 `2pass` / `2pass-offline` 路线）：<https://github.com/modelscope/FunASR>

## 3. 结合当前项目的设计判断

## 3.1 不建议的方案

### A. 纯 LLM 判定语句完成

不建议直接让每次 partial 都走一次 LLM，然后由 LLM 决定 accept。

原因：

- latency 抖动大
- 容易受局部 partial 噪声影响
- 错误时没有安全底座
- 会把 realtime 控制链过度耦合到模型响应时间

### B. 只靠提示词让主回复模型顺便决定打断

也不建议把“打断判定”混进正常回复 LLM 的 prompt 里。

原因：

- 会把 control policy 和 response generation 搅在一起
- 更难缓存、限时、隔离失败
- 很难做到结构化小返回和快速兜底

## 3.2 推荐方案

推荐引入一个 runtime-owned 的 `SemanticTurnJudge`：

- 位置：`internal/voice`
- 依赖：只依赖 provider-neutral 的 `agent.ChatModel`
- 作用：输出一个小而结构化的语义判断对象
- 原则：**advisory, not authoritative**

Judge 输出的关键字段建议为：

- `utterance_status`: `incomplete | complete | correction`
- `interruption_intent`: `unknown | backchannel | takeover | correction | request | question | continue | other`
- `confidence`
- `reason`

然后把这份结果并回 `InputPreview.Arbitration`，供两条主链消费：

1. **preview / early processing**
   - 可把 `wait_for_more` 提升到 `draft_allowed`
   - 可在 correction 语义下压回保守状态
   - 可更早触发 prewarm
2. **barge-in**
   - 可把本来会被误判为 hard interrupt 的短句拉回 `backchannel`
   - 可把本来还会停在 `duck_only` 的语义 takeover 更早升级为 `hard_interrupt`

## 4. 本次落地的最小可用实现

本次实现不做“大而全”的 semantic runtime，而是做一个当前项目最需要、可直接落地的 MVP：

### 4.1 新增 `SemanticTurnJudge`

- 文件：`internal/voice/semantic_judge.go`
- 默认实现：`LLMSemanticTurnJudge`
- 模型边界：通过 `agent.ChatModel`
- 返回：结构化 JSON 解析为 `SemanticTurnJudgement`

### 4.2 接入 preview session，而不是接入 gateway

原因：

- preview 语义判断本质上是 voice runtime 的能力
- 不该让 websocket adapter 自己决定何时调用 LLM
- 也不该让 adapter 自己缓存 candidate text

因此：

- `asrInputPreviewSession` 在 preview candidate 成熟后异步触发 semantic judge
- judgement 结果缓存在 preview session 内
- 后续 `PushAudio` / `Poll` 都会把结果 merge 回 `InputPreview`

### 4.3 只让 LLM 做可撤销 promotion，不直接做最终 accept

当前实现中，LLM judgement 主要做两件事：

- `complete`：可把 preview 提升到 `draft_allowed`
- `correction`：可把 preview 拉回保守状态

但它不会直接产生：

- `CommitSuggested=true`
- `AcceptNow=true`

这样可以保证：

- accept 仍然需要声学/静音/已有 runtime gate 收尾
- 不会因为一次模型误判就把 turn 直接截断

### 4.4 interruption 里让 LLM 直接影响策略，但仍保留 acoustic gate

当前实现里：

- `semantic backchannel` 可阻止误触发 hard interrupt
- `semantic takeover` 可在达到 acoustic gate 后更早升级为 hard interrupt

也就是说：

- **语义可以提权**
- **但仍要过最小声学门**

这符合当前项目“实时优先”的目标。

## 5. 配置建议

本次方案建议保持显式配置：

- `AGENT_SERVER_VOICE_LLM_SEMANTIC_JUDGE_ENABLED`
- `AGENT_SERVER_VOICE_LLM_SEMANTIC_JUDGE_TIMEOUT_MS`
- `AGENT_SERVER_VOICE_LLM_SEMANTIC_JUDGE_MIN_RUNES`
- `AGENT_SERVER_VOICE_LLM_SEMANTIC_JUDGE_MIN_STABLE_FOR_MS`

推荐默认值：

- enabled: `true`
- timeout: `220ms`
- min_runes: `2`
- min_stable_for_ms: `120`

原因：

- 当前机器已有本地 LLM worker
- 研究阶段目标是尽量把 LLM 真正用起来
- 但仍需要把调用范围限制在“成熟 preview candidate”上，避免每个 token 都打一遍模型

## 6. 后续建议

本次落地只是第一步。后续最值得继续推进的是：

1. 把 `slot completeness` 也并入 semantic judge，而不只看 `utterance complete`
2. 给 semantic judge 加 domain bias/context，例如智能家居实体目录
3. 记录 semantic judge disagreement 指标：
   - heuristic says complete, LLM says incomplete
   - heuristic says interrupt, LLM says backchannel
4. 后续再考虑把语义判断从单次判定扩展为 session-scoped judge state，而不是每次都无记忆

## 7. 与当前项目边界的一致性

本方案保持了当前项目最重要的架构边界：

- `internal/voice` 继续拥有 turn-taking / interruption
- adapter 不调用 provider
- provider API 仍在 `internal/agent` 的 model boundary 后面
- 协议不需要因为这次升级而新增一套 turn-taking 事件家族
- client 不需要理解 `duck_only` / `hard_interrupt` 的内部判定细节

也就是说，这次不是“改成 LLM 主导系统”，而是：

**把 LLM 安装到 shared voice runtime 的语义裁判位。**
