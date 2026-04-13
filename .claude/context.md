# Claude Context

## Project Focus

- Architecture stability first.
- RTOS realtime voice fast path second.
- External channel skills are first-class extension points.

## Collaboration Rules

- Keep root decisions aligned with `AGENTS.md`.
- Use `plan.md` as the current execution ledger.
- Mirror durable implementation changes into `.codex` logs.

## Current Constraints

- Authentication is intentionally deferred for the first voice fast path.
- Protocol fields must still leave room for future auth and capability negotiation.
- Realtime session control must support both client-driven and server-driven end-of-dialog.
- The baseline Docker deployment path must keep `agentd` and the local FunASR worker in separate containers instead of collapsing runtime boundaries for convenience.
