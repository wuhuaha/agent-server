# ADR 0046：融合式 streaming endpoint controller 保持 stage-based 且 runtime-owned

- Status: Accepted
- Date: 2026-04-17

## Context

当前仓库已经形成：

- `streaming ASR`
- `server endpoint`
- `stable_prefix`
- `SemanticTurnJudge`
- `SemanticSlotParser`
- speaking-time interruption / playback truth

但接下来的关键问题不再是“是否要再加一个新模型”，而是：

- 这些信号应如何融合
- 决策流水线应如何组织
- 是否应采用单一 score、单一 learned controller、还是显式阶段门控

同时，仓库仍需遵守既有边界：

- gateway 只做 adapter
- provider 只提供 normalized evidence
- `internal/voice` 保持 turn-taking / interruption / endpoint orchestration ownership

## Decision

我们决定：

1. 当前主线采用 **stage-based fused endpoint controller**，而不是单一黑箱 score 或单一 semantic endpointer。
2. 该 controller 继续保持 runtime-owned，落在 `internal/voice`，至少拥有：
   - `candidate_ready`
   - `draft_ready`
   - `accept_ready`
   - speaking-time interruption escalation
3. `dynamic VAD` / acoustic floor 提供底层停顿事实与 `base_wait_ms`。
4. `streaming ASR` 提供 preview maturity，包括：
   - `stable_prefix`
   - `revision_rate`
   - `no_update_ms`
   - correction evidence
5. punctuation / clause 层提供 closure bonus 或 hold penalty。
6. `SemanticTurnJudge` 主要用于输出：
   - `utterance_status`
   - `interruption_intent`
   - `dynamic_wait_policy`
   - `wait_delta_ms`
7. `SemanticSlotParser` 与 risk guard 主要用于命令型 accept 的 hard constraint。
8. 最终 accept 不由任一单层单独制造；必须经过 runtime orchestrator 的分阶段推进。

## Consequences

### Positive

- 融合策略更可解释，便于 trace、A/B 和回滚。
- 与当前已有 `draft_allowed / accept_candidate / accept_now` 结构天然兼容。
- 有利于未来把部分 evidence 进一步学成 score 或 bandit policy，而不需要推翻主线。
- 保持 `EOU != turn accept`、`semantic judge != main dialogue LLM` 的关键边界。

### Negative

- runtime 内部状态与配置项会增加。
- 需要更好的 tracing 和 offline replay 支持，否则调参成本会上升。
- 初期仍需手工设计部分阈值与 gate。

## Follow-up

- 新增方案文档：`docs/architecture/streaming-asr-dynamic-vad-fusion-pipeline-zh-2026-04-17.md`
- 更新 `docs/architecture/overview.md`
- 在 `.codex/project-memory.md` 记录 durable decision
