# ADR 0043: 运行时实体 grounding 作为 `SemanticSlotParser` 之后的独立层

- Status: accepted
- Date: 2026-04-17

## Context

仓库已经落地：

- `SemanticTurnJudge`
- `SemanticSlotParser`
- FunASR punctuation / emotion / audio-events 的 runtime 消费

但在只接入 `SemanticSlotParser` 的情况下，shared voice runtime 仍缺少一层关键能力：

- 能知道缺少 `target` / `location` / `target_app`
- 但还不能稳定判断这些槽位是否已经映射到 canonical entity
- 也不能把“缺槽”和“命中了多个候选、现在更该澄清”严格区分

与此同时，项目当前仍处于研究阶段，seed catalog 不完整。如果把 catalog 命中失败也当成负证据，会导致：

- 运行时把本来合理的 parser 结果错误拉回 `missing`
- catalog 完整性反过来卡死 preview 早处理主链

因此需要一个边界清晰、研究期高 ROI 的 grounding MVP。

## Decision

我们决定：

1. 在 `internal/voice` 内增加 runtime-owned 的 `EntityCatalogGrounder`。
2. grounding 层放在 `SemanticSlotParser` 之后、`TurnArbitration` 之前。
3. grounding 只消费：
   - 当前 preview 文本
   - slot parser 产生的结构化摘要
4. grounding 只输出：
   - additive arbitration summary
   - 如 `slot_grounded`、`slot_canonical_target`、`slot_canonical_location`
   - 以及对 `slot_status` / `slot_actionability` / `missing_slots` / `ambiguous_slots` 的保守修正
5. grounding 遵循**正向证据优先**原则：
   - 命中唯一 canonical entity，可以 promotion / clear missing
   - 命中明确多候选歧义，可以 downgrade 到 `clarify_needed`
   - **catalog miss 不能单独否定 slot parser 结果**
6. adapter、gateway、channel layer 不拥有 entity catalog，也不直接做 canonical grounding。
7. LLM 仍不直接输出最终 canonical id 作为主判；canonicalization 由 runtime-owned catalog 完成。

## Consequences

### Positive

- `slot completeness` 从“知道有没有槽位”推进到“槽位能否映射到真实对象”。
- preview 仲裁可以更早地区分：
  - `wait_more`
  - `clarify_needed`
  - `act_candidate`
- alias / canonical grounding 仍留在 shared voice runtime，不污染 gateway 与协议层。
- 研究阶段可以先用 seed catalog 验证主链价值，再决定是否演进到动态 catalog 服务。

### Negative

- 由于 catalog 仍是 seed 版本，grounding 只能覆盖一小部分高频实体。
- 正向证据优先意味着：未命中 catalog 的长尾实体暂时不会得到显式 canonical summary。
- 仍需后续继续补：
  - session-scoped recent entity ranking
  - dynamic bias / hotword 回灌
  - canonical value normalization
  - risk-aware entity policy

## Follow-up

- 当前实现记录在：
  - `docs/architecture/entity-catalog-grounding-runtime-mvp-zh-2026-04-17.md`
- 后续如 catalog source、dynamic bias、或协议字段发生变化，需要同步更新：
  - `docs/architecture/overview.md`
  - `plan.md`
  - `.codex/project-memory.md`
