# ADR 0024: Voice Runtime Owns Preview, Playout, And Heard-Text Persistence

## Status

Accepted

## Context

After `internal/gateway` shared the native realtime and `xiaozhi` turn flow, the next architecture problem was still visible:

- hidden preview polling and auto-commit suggestions were partly in `internal/voice` and partly in gateway helpers
- playback start, interruption, and completion were still observed mainly from websocket handlers
- memory writeback knew the generated response text, but not what the user actually heard after interruption

That shape made full-duplex follow-up work harder because preview, playout, and memory were split across two layers.

## Decision

Move shared preview and playout orchestration ownership into `internal/voice`.

For the current slice:

- add `internal/voice.SessionOrchestrator`
- let gateway adapters create and feed that orchestrator, but not own preview or playout state themselves
- route hidden preview polling and auto-commit suggestions through the orchestrator
- route playback start, chunk progress, interruption, and completion through the orchestrator
- persist `delivered`, `heard`, `interrupted`, `truncated`, and `playback_completed` state through the shared runtime memory path

Gateway adapters still report transport events, but the interpretation of those events now belongs to the shared voice runtime.

## Consequences

Positive:

- preview and playout logic stop drifting between native realtime and `xiaozhi`
- the runtime can now remember what the user actually heard instead of only the full generated reply
- future full-duplex work can evolve inside `internal/voice` without widening device protocols first

Tradeoffs:

- `internal/voice` now owns another orchestration object that must stay transport-neutral
- playback progress is still heuristic in the current slice because exact client playout acknowledgements do not yet exist for every adapter

## Follow-Up

- improve playout-progress fidelity when adapters can provide stronger playback acknowledgements
- extend future channel or desktop adapters to reuse the same heard-text persistence model when they gain spoken-output paths
