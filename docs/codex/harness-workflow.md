# Codex Harness Workflow

This document is the high-signal entrypoint for coding agents working in `agent-server`.
It exists to keep `AGENTS.md` short while still giving Codex a predictable execution surface.

## Start Here

Use the standard command surface first:

```bash
make doctor
make test-go
make test-py
make test-py-workers
make docker-config
make verify-fast
```

For local service bring-up:

```bash
make run
```

For Linux dependency preparation:

```bash
./scripts/install-linux-stack.sh
./scripts/install-linux-stack.sh --with-stream-vad
```

## Repository Map

- `cmd/agentd`
  Main Go service entrypoint.
- `internal/agent`
  Shared runtime boundary for turn execution, memory, tools, and LLM providers.
- `internal/voice`
  Shared ASR and TTS runtime boundary.
- `internal/gateway`
  Realtime and compatibility adapters only.
- `internal/session`
  Session state and turn lifecycle.
- `clients/python-desktop-client`
  Scripted validation runner and RTOS mock tooling.
- `workers/python`
  FunASR worker and future Python-side voice adapters.
- `deploy/docker`
  Layered container assets.

## Canonical Docs

- Current milestone and execution ledger:
  - `plan.md`
- Historical execution archive:
  - `docs/codex/execution-log-archive-2026-04.md`
- Live validation runbook:
  - `docs/codex/live-validation-runbook.md`
- Architecture:
  - `docs/architecture/overview.md`
  - `docs/architecture/agent-runtime-core.md`
  - `docs/architecture/runtime-configuration.md`
- Protocols:
  - `docs/protocols/realtime-session-v0.md`
  - `docs/protocols/rtos-device-ws-v0.md`
  - `docs/protocols/xiaozhi-compat-ws-v0.md`
  - `docs/protocols/web-h5-realtime-adaptation.md`

## Validation Surface

Fast local validation:

```bash
make verify-fast
```

The current fast path includes:

- `go test ./...`
- Python desktop-client unit tests
- layered Docker compose config expansion

Additional Python worker coverage is available through:

- `make test-py-workers`

The richer manual and scripted validation assets live here:

- desktop runner guide:
  - `clients/python-desktop-client/README.md`
- archived live baselines:
  - `artifacts/live-baseline/`
- canonical live runbook:
  - `docs/codex/live-validation-runbook.md`
- Codex-local debugging records:
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`

## Collaboration Templates

- Use `.github/ISSUE_TEMPLATE/bug-report.md` for regressions, runtime failures, or deployment breakage.
- Use `.github/ISSUE_TEMPLATE/architecture-task.md` for planned feature, refactor, or migration slices.
- Use `.github/pull_request_template.md` to keep boundary, protocol, docs, and validation follow-through aligned with the repo guardrails.

## Web/H5 Evidence

- Use `scripts/web-h5-manual-capture.sh` to scaffold a canonical browser-validation artifact bundle before manual testing.
- Use `docs/codex/live-validation-runbook.md` for the expected `web-h5-manual` layout and attachment naming.

## Working Rules

- Do not bypass `internal/agent` or `internal/voice` from adapters.
- Prefer extending the standard command surface instead of inventing one-off shell sequences.
- When a task produces deep analysis, architecture comparison, or research conclusions, land the durable result in the appropriate `docs/` note in the same change and update the related indexes or durable logs instead of leaving the outcome only in chat.
- When you discover a durable environment or tooling caveat, record it in `.codex/issues-and-resolutions.md`.
- When you add a new repeatable verification path, expose it through `Makefile`, `scripts/`, or both.

## Current Harness Gaps

These are known next-step improvements:

- GitHub Actions currently covers only fast checks; live ASR/TTS smokes are still manual.
- Browser screenshots, console exports, and exported WAV attachments are now scaffolded, but still require manual browser actions to complete.
- CPU worker image builds still depend on stable access to large PyTorch wheels.
