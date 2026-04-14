# AGENTS.md

This repository keeps the top-level agent instructions intentionally short.
Use this file for hard guardrails and standard entrypoints only.
Load deeper context from the linked docs when the task needs it.

## Mission

Build a reusable agent service framework for multimodal voice, image, and text interaction across RTOS devices, desktops, Web/H5 clients, and external channels.

## Priority Order

1. Architecture first.
2. RTOS voice fast path second.
3. Agent Runtime Core stabilization third.
4. Channel skill extensibility fourth.
5. Authentication, tenancy, and policy after the fast path and runtime boundary are stable.

## Guardrails

- Treat `Realtime Session Core` as the center of the system.
- Keep device adapters and channel adapters as ingress and egress layers only.
- Do not let channel-specific code call model providers directly.
- Keep `internal/voice` as a built-in runtime capability, not an optional afterthought.
- Preserve protocol fields for future auth even when auth is temporarily disabled.
- Prefer `WebSocket + binary audio + JSON control events` for the first RTOS path.
- Document protocol-shape changes in `docs/protocols/` and `schemas/`.
- Record architecture-level choices in `docs/adr/`.

## Required Follow-through

- Update `plan.md` when scope, priority, or milestone state changes.
- Update `.codex/change-log.md` for meaningful repository changes.
- Update `.codex/issues-and-resolutions.md` when a blocker is found or closed.
- Update `.codex/project-memory.md` when a durable decision is made.
- If realtime session protocol or state transitions change, update:
  - `docs/protocols/realtime-session-v0.md`
  - `schemas/realtime/session-envelope.schema.json`
- If channel skill boundaries change, update `docs/protocols/channel-skill-contract-v0.md`.

## Standard Command Surface

Start here before inventing ad hoc commands:

- `make doctor`
- `make test-go`
- `make test-go-integration` for tagged listener-backed gateway and voice adapter tests
- `make test-go-system`
- `make test-py`
- `make test-py-workers`
- `make docker-config`
- `make verify-fast`
- `make run`

If Linux dependencies are not prepared yet:

- `./scripts/install-linux-stack.sh`
- `./scripts/install-linux-stack.sh --with-stream-vad`

## Context Map

Open these docs before large changes:

- Codex workflow and harness entrypoints:
  - `docs/codex/harness-workflow.md`
- Current goals and milestone ledger:
  - `plan.md`
- Architecture boundaries:
  - `docs/architecture/overview.md`
  - `docs/architecture/agent-runtime-core.md`
  - `docs/architecture/runtime-configuration.md`
- Protocol contracts:
  - `docs/protocols/realtime-session-v0.md`
  - `docs/protocols/rtos-device-ws-v0.md`
  - `docs/protocols/xiaozhi-compat-ws-v0.md`

## Local Agent Extensions

- Project-specific skills live under `.codex/skills/`.
- Imported reference roles and skills live under `agents/` and `skills/`.
- Those root reference directories are intentionally trimmed to the current `agent-server` stack: Go, Python, voice or agent runtime work, deployment, security, docs, testing, and harness workflows.
- Treat those directories as supporting references, not as permission to ignore the project guardrails above.
