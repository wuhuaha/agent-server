# ADR 0049：task-family aware 的早处理门槛保持 generic，slot 约束只在结构化命令上变硬

- Status: Accepted
- Date: 2026-04-17

## Context

当前仓库已经具备：

- `stable_prefix`
- `SemanticTurnJudge`
- `SemanticSlotParser`
- stage-based `candidate_ready -> draft_ready -> accept_ready`

但在代码层仍存在一个关键偏差：

- 对完整 preview，runtime 往往会直接放行 `draft_allowed`
- 这对 `knowledge_query` 类问题通常是对的
- 但对结构化命令类请求，`utterance complete != slot ready`

也就是说，当前 runtime 虽然已经有 `slot completeness`，但“是否应该等 slot 再 draft”这件事仍然缺少一个更通用、跨 domain 的抽象层。

如果直接继续依赖 `smart_home / desktop_assistant / general_chat` 这些 raw domain 去驱动 early gate，会有两个问题：

1. domain taxonomy 仍偏研究期 MVP，不足以稳定代表 early-processing policy
2. shared runtime 会继续被垂直场景语义绑架，而不是围绕通用交互模式建模

## Decision

我们决定在 shared voice runtime 中引入一个更通用的中间抽象：`task_family`。

当前内建 family：

- `dialogue`
- `knowledge_query`
- `structured_command`
- `structured_query`
- `correction`
- `backchannel`
- `unknown`

并采用以下策略：

1. `SemanticSlotParser` 输出可以继续保留当前 raw `domain`，但同时补一个更通用的 `task_family`。
2. runtime 允许在 parser 结果到来前，先通过 lexical / semantic 线索推一个保守的 task-family floor。
3. `structured_command` 默认启用 `slot_constraint_required=true`：
   - lexical complete 只保证 `prewarm_allowed`
   - 不再因为“看起来像说完了”就直接放行 `draft_allowed`
   - 需要等 slot parser 给出 `draft_ok / clarify_needed / act_candidate` 之类的结构化结果后再前推
4. `knowledge_query` / `dialogue` 继续允许更早 draft，因为这类请求不应被 slot completeness 一刀切卡住。
5. 这套 family 与 slot-guard 逻辑继续保持 runtime-owned，仍落在 `internal/voice`，不外溢到 gateway、transport 或 public protocol。

## Consequences

### Positive

- 早处理门槛终于开始真正 task-aware，而不是只看 utterance completeness。
- shared runtime 不必直接拿 raw domain taxonomy 当成政策中心。
- 结构化命令的 early draft 更保守，降低“句子像说完但参数仍未稳”的误推进。
- 问答/闲聊仍保持低延迟，不会被 slot completeness 无意义拖慢。

### Negative

- runtime 内部多了一层抽象，需要持续校验 lexical family floor 与 parser family result 是否一致。
- 当前 task-family 仍是第一版，后续如果 channel/tool 生态扩展，仍可能继续细化。
- family 误判时，可能带来“命令被先当问答”或“问答被过于保守”的短暂偏差，因此仍需靠 semantic/slot parser 后验修正。

## Follow-up

- 更新：
  - `docs/architecture/overview.md`
  - `.codex/project-memory.md`
- 说明文档：
  - `docs/architecture/task-family-aware-early-processing-gate-zh-2026-04-17.md`
