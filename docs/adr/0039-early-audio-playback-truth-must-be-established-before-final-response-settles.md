# ADR 0039: Early-Audio Playback Truth Must Be Established Before Final Response Settles

## Status

Accepted

## Context

The repository now supports earlier audio startup through `TurnResponseFuture` and clause-aware planning, but playback truth quality still depends on when the runtime begins tracking the assistant turn.

Before this slice, the native realtime path still had one important gap:

- early audio could begin before the final `TurnResponse` settled
- but `SessionOrchestrator` turn state was only fully refreshed after `executeTurnResponse(...)` returned
- later streamed text deltas were not guaranteed to extend the active playback text while the output was already speaking

That meant interruption, heard-text truncation, and next-turn resume context could lag behind the real spoken output on the earliest-audio path.

## Decision

The early-audio path now establishes and updates playback truth during output, not only after final response settlement:

1. the realtime gateway prepares the runtime-owned turn context before or at early audio startup instead of waiting for the final response envelope
2. while output is already `speaking`, later streamed text deltas may extend the active playback text through the same shared runtime boundary
3. the final settled response still refreshes the same turn record, but it is no longer the first moment that playback truth becomes available

This remains an internal runtime rule:

- no new wire event is required
- device adapters still do not compute heard-text themselves
- `internal/voice.SessionOrchestrator` remains the owner of delivered-text, heard-text, interruption metadata, and next-turn playback context

## Consequences

Positive:

- interruption during early audio can now truncate against a more current delivered-text view
- `voice.previous.*` resume context is less likely to lag one or more late text deltas behind the actually spoken answer
- early-audio and playback-truth paths now converge on the same runtime ownership boundary

Tradeoffs:

- playback-text updates during speaking are now more stateful and need regression coverage
- some residual mismatch may still exist until playback ACK grows more segment-aware on the server side

## Follow-Up

- continue refining playback ACK and segment-level truth so later marks can align to clause or segment boundaries more precisely
- keep this behavior shared across adapters instead of adding transport-local resume heuristics
