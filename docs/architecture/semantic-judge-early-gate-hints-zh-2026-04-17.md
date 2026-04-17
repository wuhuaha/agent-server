# semantic judge 的 task-family / slot-readiness hint 早处理增强说明（2026-04-17）

## 本轮目标

在已有 `task_family + slot_constraint_required` 的基础上，继续把早处理门槛从“主要靠 lexical heuristics + slot parser 后验”推进到“更充分利用小 LLM 语义裁判”。

本轮重点不是改 public protocol，而是增强 `internal/voice` 内部这条 runtime-owned 早处理链：

- semantic judge 可以直接表达更像什么 `task_family`
- semantic judge 可以直接表达当前更适合：
  - `wait_slot`
  - `clarify`
  - `ready`
  - `not_applicable`

## 为什么要做

上一轮虽然已经有：

- `task_family`
- `slot_constraint_required`
- `structured_command` 先 prewarm 再等 slot guard

但仍有两个现实问题：

### 1. lexical floor 仍会误判

例如：

- `帮我看看明天上海天气`

这类句子在 lexical 层可能会被 `帮我` 一类前缀误拉向命令型，但从语义上其实更像：

- `knowledge_query`
- 或 `structured_query`

如果 runtime 只信 lexical floor，就会把本该尽早 draft 的查询类句子错误地拖进 slot guard。

### 2. structured command 不是只有“等 slot”这一种状态

对结构化命令而言，语义裁判至少还需要能区分：

- `wait_slot`：对象/参数明显还没说完
- `clarify`：这句话大致说完了，但应该立刻澄清
- `ready`：已经足够具体，可做可撤销 draft / planning

如果 semantic judge 只会说 `complete / incomplete`，那它对命令类 preview 的帮助仍然太弱。

## 本轮实现

### 1. semantic judge 新增两类结构化输出

文件：

- `internal/voice/semantic_judge.go`

新增输出：

- `task_family`
- `slot_readiness_hint`

其中 `slot_readiness_hint` 当前枚举为：

- `unknown`
- `not_applicable`
- `wait_slot`
- `clarify`
- `ready`

### 2. semantic request 现在会带上 runtime 当前的 floor

semantic judge 的 user message 现在会额外带上：

- `task_family_hint`
- `slot_constraint_required`

这样模型不是在“纯盲判”，而是在 shared runtime 当前状态之上做保守校正。

### 3. semantic judge 可纠正 lexical task-family floor

当前 merge 策略是：

- 若 semantic judge 高置信度给出了明确 `task_family`
- runtime 允许它覆盖当前 lexical floor

这让：

- imperative 外观但本质是查询/问答的 preview

更容易回到正确通道，而不是继续被错误 slot guard 拖慢。

### 4. structured command 的语义 hint 不再只有“等 slot”

当前合并策略：

- `wait_slot`：继续保守，只 prewarm 不 draft
- `clarify`：可提前进入 `draft_allowed`，并标记 clarify-needed 倾向
- `ready`：可提前进入 `draft_allowed`
- `not_applicable`：说明当前不是 slot-gated 请求，可按 query/dialogue 路径更早推进

也就是说，本轮把命令型 preview 的语义状态从二值化推进到了更像三段式：

- 继续等
- 先澄清
- 先推进

### 5. semantic judge 启动时机也更早了一点

当前 `shouldJudgeSemantic(...)` 已不再只看：

- stable dwell
- `draft_allowed`
- `accept_candidate`

它现在还会看：

- `candidate_ready`
- 当前 `task_family`

这样 `knowledge_query / structured_command / structured_query / correction` 在成为成熟 candidate 后，可以更早得到小模型语义裁判，而不是必须等更晚的保守门槛。

## 当前收益

### 1. 查询类 preview 更不容易被误拖慢

例如：

- `帮我看看明天上海天气`

现在可以通过 semantic judge 明确回到：

- `task_family=knowledge_query`
- `slot_readiness_hint=not_applicable`

从而解除错误的 slot guard。

### 2. 命令类 preview 的“先澄清”路径更自然

例如：

- `把灯调到舒服一点`

语义上更像“该马上澄清一下”，而不是继续空等 slot parser 或继续傻等更多音频。

现在 semantic judge 已能把这类情况表达成：

- `task_family=structured_command`
- `slot_readiness_hint=clarify`

### 3. 可观测性更强

当前新增的 runtime 字段与 metadata 已包括：

- `TurnArbitration.SemanticSlotReadiness`
- `voice.preview.semantic_slot_readiness`
- gateway trace 中的 `preview_semantic_slot_readiness`

后续 trace / replay 可以直接看到“为什么这轮没 draft”或“为什么这轮已经可以先澄清”。

## 与 slot parser 的关系

这不是要让 semantic judge 替代 slot parser。

当前分工仍然是：

- semantic judge：
  - 更轻、更早、更偏门槛裁判
  - 回答“像什么任务”“现在更该等/澄清/推进哪一种”
- slot parser：
  - 更细、更结构化、更偏槽位后验
  - 回答“缺哪几个槽位”“是否 grounded”“是否 risk confirm”

也就是说：

- semantic judge 更像 early gate 的快速语义裁判
- slot parser 更像结构化后验与执行前约束

## 验证

本轮新增/更新了：

- `internal/voice/semantic_judge_test.go`
- `internal/gateway/realtime_test.go`

覆盖点包括：

- semantic judge 解码 `task_family + slot_readiness_hint`
- semantic family 覆盖 lexical floor
- `clarify` hint 提前推动 draft
- `candidate_ready knowledge_query` 更早启动 semantic judge

## 结论

本轮之后，shared voice runtime 的早处理门槛已经开始从：

- lexical floor + slot parser 后验

推进到：

- lexical floor + semantic task-family override + semantic slot-readiness hint + slot parser 后验

这让实时语音链路在不扩大 protocol 的前提下，更接近“先用小模型做智能化 early gate，再由更结构化后验继续校正”的目标形态。
