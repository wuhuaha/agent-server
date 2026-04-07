# AGENTS.md

## Mission

Build a reusable agent service framework for multimodal voice, image, and text interaction across RTOS devices, desktops, and external channels.

## Current Priority Order

1. Architecture first.
2. RTOS voice fast path second.
3. Agent Runtime Core stabilization third.
4. Channel skill extensibility fourth.
5. Authentication, tenancy, and policy after the fast path and runtime boundary are stable.

## Guardrails

- Treat `Realtime Session Core` as the center of the system.
- Keep `device adapters` and `channel adapters` as ingress and egress layers only.
- Do not let channel-specific code call model providers directly.
- Keep `voice-core` as a built-in runtime capability, not an optional afterthought.
- Preserve protocol fields for future auth even when auth is temporarily disabled.
- Prefer `WebSocket + binary audio + JSON control events` for the first RTOS path.
- Document every protocol shape change in `docs/protocols/` and `schemas/`.
- Record any architecture-level choice in `docs/adr/`.

## Required Updates When Making Changes

- Update `plan.md` when scope, priority, or milestone state changes.
- Update `.codex/change-log.md` for meaningful repository changes.
- Update `.codex/issues-and-resolutions.md` when a blocker is found or closed.
- Update `.codex/project-memory.md` when a durable decision is made.
- If protocol events or state transitions change, update:
  - `docs/protocols/realtime-session-v0.md`
  - `schemas/realtime/session-envelope.schema.json`
- If channel skill boundaries change, update `docs/protocols/channel-skill-contract-v0.md`.

## Implementation Direction

- Go is the primary language for the service, transport, orchestration, and control APIs.
- Python workers are reserved for future ASR/TTS/VLM adapters and algorithm-heavy tasks.
- Start as a modular monolith. Split only after real pressure proves it is necessary.
- Keep interfaces narrow and versioned.

## Review Standard

- Prefer behavioural correctness over clever abstractions.
- If a shortcut risks protocol churn later, do not take it.
- If a change helps only one transport but weakens the shared session core, reject it.
