# task-family aware 的早处理门槛实现说明（2026-04-17）

## 文档定位

- 性质：当前实现切片说明
- 目标：解释为什么在 `slot completeness` 之外，还需要一个更通用的 `task_family`
- 关联文档：
  - `docs/architecture/unified-early-processing-threshold-zh-2026-04-16.md`
  - `docs/architecture/slot-completeness-computable-object-zh-2026-04-16.md`
  - `docs/adr/0049-task-family-aware-early-gate-keeps-slot-constraints-generic.md`

## 一页结论

当前仓库已经把“统一早处理门槛”的第一版研究，落进了一个更可执行的 runtime 形态：

- `task_family` 用来表达“这一轮更像什么交互模式”
- `slot_constraint_required` 用来表达“这一轮 draft 是否必须等 slot completeness”

这意味着：

- `knowledge_query` / `dialogue` 可以继续尽早 draft
- `structured_command` 不再只因为 utterance complete 就直接 draft
- `structured_command` 会先 prewarm，再等 slot parser 给出更强后验

## 为什么不是直接拿 domain 做政策中心

当前 `SemanticSlotParser` 的 raw domain 仍是：

- `smart_home`
- `desktop_assistant`
- `general_chat`

这对研究阶段够用，但它并不等于 early-processing policy：

- 一个 `smart_home` 请求可能是控制命令，也可能是状态查询
- 一个 `general_chat` 请求可能是开放闲聊，也可能是知识问答

所以更适合 runtime 的，是一层更接近交互模式的抽象：

- `structured_command`
- `structured_query`
- `knowledge_query`
- `dialogue`
- `correction`
- `backchannel`

## 本轮代码落点

### 1. 新增 generic task-family 抽象

文件：

- `internal/voice/semantic_task_family.go`

当前能力：

- 定义了 `task_family` 常量与 normalize helper
- 提供 lexical family floor
- 提供 semantic / slot 结果到 family 的推断函数
- 提供 `taskFamilyRequiresSlotReadiness(...)`

### 2. `TurnArbitration` 现在显式暴露 family 与 slot guard 需求

文件：

- `internal/voice/contracts.go`

新增字段：

- `task_family`
- `slot_constraint_required`

这让 preview trace、prewarm metadata、以及后续调参时能直接看到“为什么这一轮 draft 被压住”。

### 3. lexical complete 的结构化命令不再默认直接 draft

文件：

- `internal/voice/turn_detector.go`

当前策略：

- `structured_command`：
  - lexical complete 时仍可 `candidate_ready`
  - 仍可 `accept_ready`
  - 仍可 `prewarm_allowed`
  - 但默认不直接 `draft_allowed`
- `knowledge_query` / `dialogue`：
  - 保持更早的 `draft_allowed`

这比此前“只要 utterance complete 就 draft”更符合统一早处理门槛的研究结论。

### 4. slot parser 会对结构化命令更早启动

文件：

- `internal/voice/semantic_slot_parser.go`

当前策略：

- 如果当前 preview 已经被判断为 `structured_command` 且 `candidate_ready=true`
- 那么即使 stable dwell 还没达到常规门槛，也允许更早拉起 slot parser

这样做的目的是：

- 避免“命令不让 draft，但 slot parser 又起得太慢”
- 让 structured command 能尽快拿到 `draft_ok / clarify_needed / act_candidate` 这类结构化后验

### 5. semantic judge 现在只对非 slot-constrained family 更激进地放 draft

文件：

- `internal/voice/semantic_judge.go`

当前策略：

- 若 `task_family` 需要 slot guard，semantic complete 先只推动 `prewarm_allowed`
- 若 `slot parser` 结果已到，且给出可前推 actionability，再进入 `draft_allowed`
- 对 `knowledge_query` 等 family，semantic complete 仍可直接帮助 draft

## 当前收益

### 1. 命令类 preview 更稳

现在像：

- `打开客厅灯`
- `把灯调亮一点`

这种句子，不会再仅因为 lexical complete 就被过早当成 draft-ready。

### 2. 问答类 preview 不被无意义拖慢

现在像：

- `明天周几`
- `上海天气怎么样`

仍会保持更积极的 early draft / wait shortening。

### 3. slot completeness 不再被一刀切地强加给所有任务

这更符合之前研究文档的判断：

- slot completeness 是 task-aware 的，而不是 universal hard gate

## 已有测试覆盖

- `internal/voice/turn_detector_test.go`
- `internal/voice/semantic_judge_test.go`
- `internal/voice/fused_endpoint_controller_test.go`

当前新增覆盖点包括：

- `knowledge_query` 仍可早 draft
- `structured_command` 在 slot 到来前只 prewarm 不 draft
- slot parser 会对结构化命令更早启动
- slot parser 返回 `task_family` 后会把 slot guard 需求带进 arbitration

## 剩余改进空间

### 1. family 仍是第一版 taxonomy

后续可继续扩展，但应谨慎，避免重新长成一套垂直业务分类树。

### 2. 当前 lexical family floor 仍偏保守启发式

它的职责只是：

- 给 runtime 一个低延迟 floor
- 真正的修正仍交给 semantic judge / slot parser

### 3. 后续可把 family 进一步带到 trace 与 offline replay

当前主链已经有 arbitration 字段和 prewarm metadata，但后续如果要做更系统的质量分析，可以进一步把 `task_family` 纳入 replay / trace 统计。
