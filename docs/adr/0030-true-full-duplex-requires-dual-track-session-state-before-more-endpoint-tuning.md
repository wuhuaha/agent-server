# ADR 0030: True Full Duplex Requires Dual-Track Session State Before More Endpoint Tuning

## Status

Accepted

## Context

The phase-1 voice demo has already moved beyond the earliest half-duplex baseline:

- shared server-endpoint preview sessions exist behind `internal/voice`
- websocket adapters can already auto-commit on preview suggestions
- the shared speech planner can already start incremental TTS work before the full text response is fully closed
- barge-in gating and heard-text persistence already exist

However, the internal session model still exposes one session-wide state machine:

- `CommitTurn()` moves the whole session into `thinking`
- `SetState(StateSpeaking)` moves the whole session into `speaking`
- speaking-time input is currently modeled mostly as staged barge-in plus interrupt-then-next-turn

That means the repository still behaves more like strong half-duplex with adaptive interruption than true full-duplex session orchestration.

## Decision

Prioritize an internal dual-track session model before further treating endpoint tuning or planner tuning as the main path to full duplex.

Specifically:

- keep the public websocket contract stable during this refactor unless a concrete protocol gap is proven
- introduce distinct internal input and output lanes inside the realtime session core
- allow speaking output and live input preview to coexist as first-class internal states
- treat `CommitTurn()` and speaking lifecycle as separate orchestration boundaries instead of one total gate
- evolve interruption from accept-or-reject hard interrupt gating toward strategy-based arbitration on top of that dual-track model

## Consequences

- the next major full-duplex step belongs in the session core, not mainly in adapter-local endpoint heuristics
- `server_endpoint` remains important, but it is not the primary remaining bottleneck
- early incremental TTS remains important, but it should be reattached to a richer output-lane lifecycle instead of growing as another finalize-stage workaround
- interruption policy can later add `backchannel`, `duck_only`, `hard_interrupt`, and `ignore` without forcing transports to own that policy
- the external device/channel adapters remain adapters over shared session and voice-runtime boundaries rather than becoming a second orchestration layer
