# ADR 0044: slot post-processing 保持机制通用，seed 数据通过 profile 接入

- Status: accepted
- Date: 2026-04-17

## Context

在 `SemanticSlotParser -> EntityCatalogGrounder` 之后，shared voice runtime 开始具备更强的语义能力：

- canonical entity grounding
- recent-context disambiguation
- runtime-owned ASR hints
- canonical value normalization
- risk-aware clarification

但随着这些能力落地，也出现了新的边界风险：

- 智能家居 seed entity 很容易被误写成 runtime 的永久业务逻辑
- `门锁`、`删除`、`关机` 这类词面规则如果直接散落在 `internal/voice`，会让通用 agent server 腐化成某个 demo 的业务层
- 一旦 risk gating 依赖词面硬编码，后续扩到桌面助理、车载、客服、企业协作等场景时，会很快失控

本仓库的目标是通用 `ai agent server`，语音 agent 只是其中一种 runtime capability，因此必须明确：

- `internal/voice` 可以拥有机制
- 但不应把某个 seed app 的具体业务词表当成 runtime 主语

## Decision

我们决定：

1. `internal/voice` 保留以下 runtime-owned 通用机制：
   - entity grounding orchestration
   - session recent-context ranking
   - provider-neutral ASR hints (`hotwords` / `hint_phrases`)
   - slot value normalization
   - risk gating / confirm-required policy
2. 具体 seed entity 数据不再被视为架构默认真理，而是放在 optional built-in profile 中。
3. 当前仓库只内建一个 seed profile：`seed_companion`。
   - 该 profile 仅用于当前研究阶段的 smart-home / desktop-assistant 高频 demo
   - 未来可增加更多 profile，或替换成外部 catalog source
4. runtime 风险机制只消费抽象注解，例如：
   - `risk_level`
   - `risk_reason`
   - `confirm_required`
   这些注解可来自 catalog、policy 或后续外部配置。
5. runtime 不再依赖散落的业务词面规则来直接把某些文本判成高风险。
6. 若未启用 entity catalog profile，slot parser 仍可单独运行；grounding / recent-context hints 只是可选增强，而不是语音 runtime 的硬前提。

## Consequences

### Positive

- 通用 server 的架构边界更清晰：机制留在 runtime，seed data 留在 profile。
- 当前 smart-home demo 仍可复用现有能力，但不会继续污染 shared runtime。
- recent-context ASR hints 继续保留高 ROI，同时仍是 provider-neutral contract。
- 风险机制以后可平滑演进到 catalog annotations、policy engine 或 tenancy-aware config，而不需要先清理一批文本硬编码。

### Negative

- 当前 built-in profile 仍然是 seed 版本，覆盖面有限。
- value normalization 里仍保留少量 seed-domain 规则，用于研究阶段验证收益；后续最好继续向 profile data 或 policy annotations 收敛。
- 若部署方关闭 catalog profile，当前 demo 的 canonical grounding / recent-context bias 能力会同步下降，需要显式按场景开启。

## Follow-up

- 当前实现说明记录在：
  - `docs/architecture/voice-runtime-slot-post-processing-boundary-zh-2026-04-17.md`
- 若 profile shape、runtime risk annotations、或 public protocol 发生变化，需要同步更新：
  - `docs/architecture/overview.md`
  - `plan.md`
  - `.codex/project-memory.md`
