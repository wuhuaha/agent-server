# ADR 0048：Demo profile 必须显式存在，不能回流为 shared default

- Status: Accepted
- Date: 2026-04-17

## Context

上一轮已经把 shared runtime 默认值收敛为 generic：

- `agent.skills = ""`
- `agent.persona = general_assistant`
- `agent.execution_mode = dry_run`
- `voice.entity_catalog_profile = off`

但如果仓库没有一个清晰的 household demo profile 入口，日常联调和部署操作仍容易出现两种回流：

1. 直接把 household 变量重新写回 `.env.example` 或 systemd 基线 env
2. 依赖口头约定，让每个人手工记忆要 export 哪几个 demo 变量

这会让 demo 配置重新变成“隐式默认值”，并继续腐化 shared runtime 的外观。

## Decision

我们决定：

1. 保持 `.env.example` 与 `deploy/systemd/agent-server-agentd.env.example` 作为 generic baseline。
2. 把 household demo 入口显式收敛到单独的 overlay/profile 文件，而不是 shared default。
3. 这个 household overlay 至少显式开启：
   - `AGENT_SERVER_AGENT_SKILLS=household_control`
4. 其余 vertical-demo 强相关配置继续保留为显式可选项：
   - `AGENT_SERVER_AGENT_PERSONA=household_control_screen`
   - `AGENT_SERVER_AGENT_ASSISTANT_NAME=小欧管家`
   - `AGENT_SERVER_AGENT_EXECUTION_MODE=simulation`
   - `AGENT_SERVER_VOICE_ENTITY_CATALOG_PROFILE=seed_companion`
5. README、runtime configuration 文档、以及部署样例必须同步指向这个显式 overlay 入口，避免文档再次引导用户修改 shared default。

## Consequences

### Positive

- 通用默认值与 demo 入口同时存在，但职责清晰。
- household demo 可以继续快速启用，不需要再次改 runtime 代码。
- systemd 与本地 shell 的 bring-up 路径都能复用同一个 profile 思路。
- 后续增加其他 vertical 时，也可以沿用“generic baseline + explicit overlay”的方式扩展。

### Negative

- 配置文件数量会增加一份，需要维护。
- 使用者需要理解“baseline + overlay”的两层关系，而不是只复制一个最终 env。

## Follow-up

- 更新：
  - `docs/architecture/overview.md`
  - `docs/architecture/runtime-configuration.md`
  - `docs/architecture/household-demo-profile-zh-2026-04-17.md`
  - `.codex/project-memory.md`
- 后续如果再引入其他 vertical demo，也应遵守同一原则：
  - generic baseline 不变
  - vertical capability 放到显式 profile/env overlay 中
