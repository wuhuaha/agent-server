# ADR 0005: Stream Agent Deltas Through Runtime Sinks

- Status: accepted
- Date: 2026-03-31

## Context

The first `Agent Runtime Core` introduced `TurnOutput.Deltas`, but that still required the runtime to materialize every delta before the gateway could emit `response.chunk`. That shape would delay tool progress, intermediate text, and future model output until the whole turn completed, which weakens the RTOS fast path and would force channel adapters to choose between waiting or re-implementing runtime orchestration.

## Decision

- Add a sink-based `StreamingTurnExecutor` interface under `internal/agent`.
- Add a matching `StreamingResponder` path under `internal/voice` so ASR and TTS remain adapters over the same runtime boundary.
- Keep the existing `TurnExecutor` and `Responder` contracts as compatibility wrappers over the streaming path where useful.
- Let the realtime gateway emit `response.chunk` events as streamed deltas arrive instead of waiting for the final response object to be fully materialized.

## Consequences

- Runtime tool progress and text deltas can reach RTOS clients before final response assembly completes.
- Device and channel adapters still do not own orchestration; they only consume streamed runtime output.
- The next architecture step is to route the first channel adapter through this boundary and then swap no-op memory or tool providers for real backends.
