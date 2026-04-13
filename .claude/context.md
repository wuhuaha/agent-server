# Claude Context

## Project Focus

- Architecture stability first.
- RTOS realtime voice fast path second.
- External channel skills are first-class extension points.

## Collaboration Rules

- Keep root decisions aligned with `AGENTS.md`.
- Use `plan.md` for the active execution ledger and `docs/codex/execution-log-archive-2026-04.md` for older completed slices.
- Use `docs/codex/live-validation-runbook.md` when a task needs archived live-stack evidence instead of fast local checks.
- Prefer repository-local `scripts/smoke-funasr.sh`, `scripts/smoke-rtos-mock.sh`, or their PowerShell counterparts over handwritten live-smoke sequences when those helpers fit the task.
- Prefer `scripts/web-h5-manual-capture.sh` before manual browser validation so server snapshots, page snapshots, and checklist files land in the canonical artifact root.
- Mirror durable implementation changes into `.codex` logs.
- Prefer the shared `Makefile` and repository scripts over ad hoc shell sequences when a repeatable command surface exists.
- Use `make test-py-workers` when a change affects `workers/python`; `make verify-fast` intentionally stays narrower.
- Keep repository issue and PR templates aligned with the shared command surface and required protocol or ADR follow-through.

## Current Constraints

- Authentication is intentionally deferred for the first voice fast path.
- Protocol fields must still leave room for future auth and capability negotiation.
- Realtime session control must support both client-driven and server-driven end-of-dialog.
- The baseline Docker deployment path must keep `agentd` and the local FunASR worker in separate containers instead of collapsing runtime boundaries for convenience.
- Hidden preview, playout lifecycle, and heard-text persistence now live behind `internal/voice.SessionOrchestrator`; gateway adapters should report transport events into that boundary instead of rebuilding orchestration locally.
- Future external messaging adapters should build on `internal/channel.RuntimeBridge` so they stay normalize -> runtime handoff -> deliver adapters over the shared `Agent Runtime Core`.
