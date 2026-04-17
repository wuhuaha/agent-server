# ADR 0047：通用 runtime 默认值必须避免 domain lock-in

- Status: Accepted
- Date: 2026-04-17

## Context

近几轮收敛后，仓库已经明确把：

- `internal/gateway` 作为 adapter
- `internal/session` 作为 realtime session core
- `internal/voice` 作为 voice runtime
- `internal/agent` 作为 transport-neutral runtime core

但代码审视发现，仍有一组“默认值层面的腐化”残留在主链里：

1. agent 默认 persona 仍是 `household_control_screen`
2. agent 默认 execution mode 仍是 `simulation`
3. agent 默认 assistant name 与 built-in skill 仍直接指向智能家居 demo
4. `voice.entity_catalog_profile` 仍会通过默认化逻辑隐式开启 `seed_companion`
5. `persona` / builtin `skill` 配错时存在静默回退或静默忽略

这会产生一个与仓库使命冲突的结果：

- 架构文档强调“通用 agent server，场景能力通过 runtime skill/profile 接入”
- 但进程一启动，默认行为却已经带着 household persona、simulation 话术和 seed catalog 偏置

这会让：

- 新接入方误以为 household 是架构内建主语
- 语音 runtime 在没有显式场景配置时就带有 seed bias
- 配置漂移难以及时暴露，后续越改越像 demo backend

## Decision

我们决定：

1. agent 默认值收敛为通用形态：
   - `persona = general_assistant`
   - `execution_mode = dry_run`
   - `assistant_name = 小欧助手`
   - `skills = ""`
2. `household_control_screen` 与 `household_control` 继续保留，但只作为显式 opt-in 的 vertical capability。
3. `voice.entity_catalog_profile` 默认值改为 `off`，`seed_companion` 仅在显式配置时开启。
4. `persona` 与 builtin `skill` 的有效值必须在配置校验阶段暴露，不再依赖静默回退或静默忽略。
5. shared runtime 的归一化 helper 应归属真正拥有该能力的 package：
   - agent persona / execution mode 归 `internal/agent`
   - semantic rollout mode 归 `internal/voice`
   app/config 层只做装配，不再手抄第二套规则。

## Consequences

### Positive

- 默认启动形态与仓库使命一致：它首先是通用 agent server，而不是 household demo server。
- vertical behavior 继续可用，但必须通过 runtime skill / profile 显式接入。
- seed smart-home / desktop bias 不再在未配置时污染共享语音 runtime。
- 配置错误更早暴露，减少 silent drift。
- 归一化逻辑收口后，后续扩 persona、execution mode、semantic rollout 时更不容易发生 drift。

### Negative

- 现有依赖 household 默认值的本地脚本或人工习惯，需要显式设置 persona / skill / profile。
- `simulation` 不再是默认执行模式，调试时若需要“仿真完成式反馈”，必须显式配置。
- 文档、测试和部署样例必须同步更新，否则容易出现“代码与说明不一致”。

## Follow-up

- 更新：
  - `docs/architecture/overview.md`
  - `docs/architecture/runtime-configuration.md`
  - `docs/architecture/project-status-and-voice-flow-review-zh-2026-04-17.md`
  - `.codex/project-memory.md`
- 若后续引入更多 runtime skill、persona 或 catalog profile，需要继续遵守：
  - 默认值保持通用
  - 垂直能力显式 opt-in
  - adapter 不得以默认值形式重新接管业务语义
