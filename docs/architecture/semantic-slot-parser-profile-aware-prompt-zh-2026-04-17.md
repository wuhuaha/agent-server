# semantic_slot_parser 的 generic/profile-aware prompt 收敛说明（2026-04-17）

## 本轮目标

把 `internal/voice/semantic_slot_parser.go` 从研究期的 raw-domain prompt 继续收敛为：

- shared default prompt 保持 generic
- `task_family` 继续作为 early gate 的共享政策中心
- 垂直场景提示只通过显式 profile / prompt hints 注入

本轮不改 gateway/session/agent 的运行时边界，但已经把显式 profile 注入一路接到了现有 voice app bootstrap。

## 为什么要改

此前 slot parser 的 shared prompt 里直接写了：

- `smart_home 关注 ...`
- `desktop_assistant 关注 ...`

这会让 LLM slot parser 在没有显式 profile 的情况下，也倾向把共享 runtime 当成“默认自带智能家居 / 桌面助理场景”的系统。

这与当前仓库已经收敛出的方向不一致：

1. shared runtime 默认值应该 generic
2. `task_family` 才是早处理门槛的共享政策中心
3. vertical demo 只能通过显式 overlay/profile 接入

## 本轮改动

### 1. shared prompt 改为 generic

现在 shared prompt 明确强调：

- `task_family` 是默认政策中心
- `domain` 只是兼容性的 coarse scope label
- 默认只使用通用槽位概念：
  - `action`
  - `target`
  - `location`
  - `attribute`
  - `value`
  - `mode`
  - `duration`
  - `query`
  - `window_name`
  - `system_setting`

同时也明确要求：

- 没有显式 profile / hint 时，不要因为零散设备词或应用词就强行套入某个垂直 domain

### 2. 新增显式 prompt profile / prompt hints 注入层

本轮新增了一个很轻的 prompt 注入机制：

- `SemanticSlotParseRequest.PromptProfile`
- `SemanticSlotParseRequest.PromptHints`
- `NewProfileAwareSemanticSlotParser(...)`

它的定位是：

- 共享默认保持 generic
- 真正需要垂直提示时，再由外层显式注入

当前内建的示例 profile 只有：

- `seed_companion`

这个 profile 会补充说明：

- 何时可以把 domain 映射到 `smart_home`
- 何时可以把 domain 映射到 `desktop_assistant`
- 这些映射只是显式 opt-in 的 research/demo hints
- 它们不能覆盖 `task_family + slot completeness` 这条共享政策主线

### 3. task_family 路径不变

本轮没有改动：

- `task_family`
- `slot_constraint_required`
- `structured_command` 先 prewarm 再等 slot completeness 的策略

也就是说，本轮做的是“prompt 政策中心去 raw-domain 化”，而不是重新设计 early gate。

## 当前边界

这次收敛后，职责更清楚了：

- `semantic_slot_parser` shared prompt：
  - generic
  - task-family-aware
  - 不再默认内嵌 smart-home / desktop policy
- vertical prompt hints：
  - 必须显式注入
  - 只是补充证据
  - 不能改变 shared runtime 默认身份

## 当前已接入的装配点

当前 app bootstrap 已会把：

- `voice.entity_catalog_profile`

显式接到：

- `NewProfileAwareSemanticSlotParser(...)`

这意味着：

- `off` 仍保持 generic shared prompt
- `seed_companion` 会显式注入对应的 research/demo hints
- 这些 hints 与 entity grounder 继续一起留在 `internal/voice` / `internal/app` 的 shared runtime 边界内，而不会流入 gateway 或 protocol

## 结论

本轮之后，`semantic_slot_parser` 的默认语义已经不再把 `smart_home / desktop_assistant` 当作共享 runtime 的默认政策中心。

如果部署确实想保留这些提示，应该显式走：

- profile
- prompt hints
- 或后续 overlay/assembly glue

而不是继续依赖 shared default prompt 自带垂直偏置。
