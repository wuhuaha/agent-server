# 当前架构与代码健康审视（2026-04-17）

## 文档定位

- 性质：代码与架构健康 review / 腐化点收敛记录
- 目标：确认当前仓库是否仍在沿着“通用 AI agent server + 内建 voice runtime”主线演进
- 相关文档：
  - `docs/architecture/overview.md`
  - `docs/architecture/project-status-and-voice-flow-review-zh-2026-04-17.md`
  - `docs/adr/0044-slot-post-processing-keeps-mechanism-generic-and-seed-data-profiled.md`
  - `docs/adr/0047-generic-runtime-defaults-avoid-domain-lock-in.md`

## 一页结论

当前仓库的主干架构本身没有明显腐化：

- gateway 仍基本保持 adapter 角色
- session core 与 voice runtime 的边界整体清晰
- semantic judge / slot parser / endpoint fusion 仍保持 runtime-owned，而不是被网关或某个 provider 抢走
- 智能家居语义主要已经被收敛到 runtime skill 和 optional seed profile

真正仍然危险的腐化点，主要集中在 **默认值、静默回退、以及配置装配层的重复逻辑**，而不在主干编排本身。

本轮完成后的判断是：

> 当前项目的主架构仍然健康，但如果不及时收敛默认值与配置漂移，它会在使用层面继续表现得像一个 household demo server，而不是通用 agent server。

## 本轮重点审视范围

### 1. Agent runtime 默认值

检查文件：

- `internal/app/config_agent.go`
- `internal/agent/llm.go`
- `internal/agent/llm_executor.go`
- `internal/app/app.go`

重点问题：

- 默认 `persona` 仍是 `household_control_screen`
- 默认 `skills` 仍隐式开启 `household_control`
- 默认 `execution_mode` 仍是 `simulation`
- 默认 assistant name 仍然绑定到当前 smart-home demo 形态

### 2. Voice runtime 的 seed bias 默认值

检查文件：

- `internal/app/config_voice.go`
- `internal/app/app.go`
- `internal/voice/entity_catalog.go`

重点问题：

- `voice.entity_catalog_profile` 虽然已经被设计成 optional profile
- 但默认化逻辑仍会把空值收敛成 `seed_companion`
- 这会让共享语音 runtime 在未显式配置场景时仍带有 demo bias

### 3. 配置装配层重复逻辑

检查文件：

- `internal/app/config_voice.go`
- `internal/voice/semantic_judge_rollout.go`

重点问题：

- semantic rollout mode 的 normalize / support check 在 app 层和 voice 层存在两套实现
- 这会形成典型 drift 风险：一处新增 mode，另一处忘了同步

### 4. 静默回退与静默忽略

检查文件：

- `internal/agent/llm.go`
- `internal/agent/runtime_skill.go`
- `internal/app/config_agent.go`

重点问题：

- `persona` 写错时会静默退回默认 persona
- builtin `skills` 写错时可能直接被忽略
- 这类“配置看似生效、实际悄悄失效”的路径会明显拉高后续维护成本

## 发现的问题与评估

### P1：通用 runtime 的默认启动形态仍被智能家居 demo 绑架

表现：

- 默认 prompt、assistant name、skills、execution mode 都偏向 household demo
- 即使不显式开启 household skill，系统整体风格也仍像家庭中控 demo

风险：

- 新功能会不自觉沿着 household 方向继续堆砌
- 容易误导后续开发者把 vertical behavior 写进 shared runtime
- 与仓库使命和既有 ADR 已经出现不一致

结论：必须优先修复。

### P1：共享 voice runtime 默认仍带 seed entity profile

表现：

- `voice.entity_catalog_profile` 空值会被默认成 `seed_companion`

风险：

- ASR hint、grounding、slot clarification 会天然偏向 smart-home / desktop seed data
- 破坏“机制通用、seed 数据 opt-in”的已收敛边界

结论：必须修复。

### P2：语义 rollout helper 重复定义

表现：

- `normalizeSemanticJudgeRolloutMode` 与 supported-check 在两个 package 里重复存在

风险：

- 后续演进 `semantic` / `sticky_percent` 规则时容易 drift
- app/config 层开始拥有不属于自己的语义规则

结论：应尽快收口到 `internal/voice`。

### P2：配置 typo 会静默漂移

表现：

- `persona` 与 builtin `skills` 存在“写错但进程照样启动”的路径

风险：

- 表面上成功部署，实际运行行为已和预期不一致
- 排查成本高，尤其在多环境调参阶段更危险

结论：应在配置校验期失败，而不是运行期悄悄吞掉。

## 本轮已落地修复

### 1. agent 默认值去域化

已修改：

- `internal/app/config_agent.go`
- `internal/agent/llm.go`

当前默认值：

- `assistant_name = 小欧助手`
- `persona = general_assistant`
- `execution_mode = dry_run`
- `skills = ""`

影响：

- household 不再是隐式默认语义
- `simulation` 仍保留，但只作为显式配置模式
- `household_control_screen` 仍可用，但变成 opt-in

### 2. entity catalog profile 默认关闭

已修改：

- `internal/app/config_voice.go`

当前默认值：

- `voice.entity_catalog_profile = off`

影响：

- shared voice runtime 默认不再带 smart-home / desktop seed bias
- `seed_companion` 继续可用，但需要显式开启

### 3. rollout helper 收口到 voice runtime

已修改：

- `internal/voice/semantic_judge_rollout.go`
- `internal/app/config_voice.go`

结果：

- normalize / support-check 由 `internal/voice` 统一导出
- app/config 层只消费 runtime-owned helper

### 4. 配置校验补齐

已修改：

- `internal/app/config_agent.go`
- `internal/agent/runtime_skill.go`

新增行为：

- 未知 `agent.persona` 会在配置校验时报错
- 未知 builtin `agent.skills` 会在配置校验时报错

这比 silent fallback / silent ignore 更符合研究阶段的质量要求。

## 测试与验证

本轮已验证：

- `go test ./internal/agent ./internal/app`

后续完整回归建议至少覆盖：

- `go test ./...`
- `make verify-fast`

## 当前仍需继续关注的剩余风险

### 1. slot/domain taxonomy 仍偏研究期 MVP

当前 `SemanticSlotParser` 的 domain 仍以：

- `smart_home`
- `desktop_assistant`
- `general_chat`

为主，这对于当前研究阶段是可接受的，但距离真正通用 agent taxonomy 仍有距离。后续若接入更多 channel / tool 生态，需要把这层进一步抽象成更通用的 action family / workflow family。

### 2. runtime skill 生态仍只有一条 built-in vertical

当前 builtin runtime skill 只有 `household_control`。这没有违反边界，但意味着“通用 server 的可插拔 vertical”还处在第一条样例阶段。后续最好增加至少一条非智能家居 vertical，验证架构真的通用。

### 3. 文档与脚本示例仍需持续同步

虽然本轮已修复默认值，但旧的历史文档、脚本或人工认知仍可能残留“默认就是 household / simulation”的惯性。后续新增示例时，需要继续防止这种回流。

## 最终判断

当前仓库没有发生根本性的架构腐化，主干仍然健康；
真正的问题是：**默认值、配置装配和静默回退还在把系统往单一 demo 方向拉。**

这轮修复之后，仓库的实际启动形态终于和它宣称的架构方向重新对齐：

- 默认是通用 assistant
- vertical capability 显式 opt-in
- seed profile 默认关闭
- 运行时语义 helper 回到真正拥有它们的 package
- 配置错误尽早暴露

这使得后续继续优化实时语音交互体验时，不必再背着一层隐式的 household 偏置前进。
