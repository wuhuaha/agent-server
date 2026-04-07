# ADR 0004: Keep Memory And Tool Hooks Inside Agent Runtime Core

- Status: accepted
- Date: 2026-03-31

## Context

After introducing the transport-neutral `TurnExecutor` and streamed response deltas, the next architecture risk was letting transports or future channel adapters talk to memory stores or tool providers directly. That would recreate transport-owned orchestration logic and make RTOS, desktop, and channel paths diverge before the first real agent runtime existed.

## Decision

- Define `MemoryStore`, `ToolRegistry`, and `ToolInvoker` interfaces under `internal/agent`.
- Inject default no-op implementations from app bootstrap so the runtime boundary exists before real providers are added.
- Keep bootstrap memory recall or persist and tool orchestration inside the executor layer rather than in the websocket gateway or channel adapters.
- Allow the bootstrap executor to exercise the tool path with a reserved debug command so the delta contract can be validated without coupling transports to tool logic.

## Consequences

- Future LLM-backed executors can reuse one storage and tool boundary across RTOS and channel transports.
- Device and channel adapters still remain ingress or egress layers only.
- The next runtime milestone is a true streaming executor interface, not transport-specific tool wiring.
