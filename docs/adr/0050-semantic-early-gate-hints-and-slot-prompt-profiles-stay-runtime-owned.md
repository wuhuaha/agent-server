# ADR 0050：语义早处理 hint 与 slot prompt profile 继续保持 runtime-owned 且 generic-by-default

- Status: Accepted
- Date: 2026-04-17

## Context

当前仓库已经完成：

- `task_family` 作为 early gate 的通用抽象
- `structured_command` 通过 `slot_constraint_required` 保持更保守的 draft 策略
- `SemanticTurnJudge` 与 `SemanticSlotParser` 都留在 `internal/voice`

但还存在两个缺口：

1. `SemanticTurnJudge` 仍主要输出 `utterance_status / interruption_intent / wait_delta`，对“这是知识问答还是结构化命令”“现在应 wait_slot 还是 clarify/ready”表达不够直接。
2. `SemanticSlotParser` 的 shared prompt 仍残留研究期 raw-domain 默认偏置，容易把 `smart_home / desktop_assistant` 误当成共享 runtime 的默认政策中心。

如果不补这两点：

- lexical `task_family` floor 的误判会继续拖累早处理门槛
- shared runtime 会继续在 prompt 层被垂直 demo 语义反向腐化

## Decision

我们决定继续把这两条能力都收在 shared voice runtime 内，但保持 generic-by-default：

1. `SemanticTurnJudge` 现在可以显式输出：
   - `task_family`
   - `slot_readiness_hint`
2. `slot_readiness_hint` 只作为 runtime-owned 的可撤销 hint，当前枚举为：
   - `unknown`
   - `not_applicable`
   - `wait_slot`
   - `clarify`
   - `ready`
3. `internal/voice` 可用这些 hint：
   - 覆盖 lexical `task_family` floor 的明显误判
   - 区分 `structured_command` 的 `wait_slot` / `clarify` / `ready`
   - 让语义裁判在 slot parser 结果回来前，也能更智能地决定是继续保守、先 prewarm，还是允许 draft
4. `SemanticSlotParser` shared prompt 必须保持 generic：
   - `task_family` 是默认政策中心
   - `domain` 降级为兼容性的 coarse scope label
   - `smart_home / desktop_assistant` 这类垂直提示只能通过显式 profile / prompt hints 注入
5. 上述 prompt profile / semantic hint 继续保持 runtime-owned：
   - gateway 不拥有这些政策
   - device/channel adapter 不调用模型
   - public realtime protocol 不因此扩大

## Consequences

### Positive

- 早处理门槛更依赖语义理解，而不只是 lexical prefix 规则。
- “帮我看看明天天气”这类 imperative 外观但本质是问答/查询的句子，更容易被拉回正确的 `task_family`。
- `structured_command` 可以在 slot parser 结果回来前，先由小模型判断是 `wait_slot`、`clarify` 还是 `ready`。
- `SemanticSlotParser` shared prompt 不再默认携带智能家居/桌面助理偏置，通用边界更清晰。

### Negative

- runtime 内部的语义对象更丰富，调参与 trace 解释成本会上升。
- `slot_readiness_hint` 与真正的 slot parser 结果可能暂时不一致，需要接受后验修正。
- 仍需要后续在 app/bootstrap 层谨慎接入 profile glue，避免重新把 vertical 逻辑塞回 shared default。

## Follow-up

- 更新：
  - `docs/architecture/overview.md`
  - `.codex/project-memory.md`
- 说明文档：
  - `docs/architecture/semantic-judge-early-gate-hints-zh-2026-04-17.md`
  - `docs/architecture/semantic-slot-parser-profile-aware-prompt-zh-2026-04-17.md`
