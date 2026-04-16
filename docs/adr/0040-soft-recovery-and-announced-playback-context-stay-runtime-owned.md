# ADR 0040: Soft Recovery And Announced Playback Context Stay Runtime-Owned

## Status

Accepted

## Context

The repository already had:

- runtime-owned playback truth entering the next turn through `voice.previous.*`
- real playout ducking for `backchannel` and `duck_only`
- segment-level playback ACK facts such as `audio.out.mark` and `audio.out.cleared`
- early audio startup before final response settlement

Two behavior gaps still remained on that same boundary:

1. `duck_only` or `backchannel` could overlap the tail of a spoken reply, but if transport playback later completed naturally the runtime still tended to collapse the outcome back to “the user heard the full reply”.
2. on early-audio or multi-segment turns, later `audio.out.meta` events could announce more playback text before the final response settled, but the runtime playback context did not always consume that newer announced tail soon enough for later ACK facts to use it.

That left continue or recap behavior too optimistic after soft overlap, and it meant segment ACK truth could still lag behind the latest announced tail.

## Decision

The shared runtime now deepens playback truth in two places without widening the public websocket schema:

1. `internal/voice.SessionOrchestrator` captures soft-overlap snapshots for `duck_only` and `backchannel`, then may preserve a recoverable `heard_text` or `missed_text` boundary even when `playback_completed=true`.
2. native realtime synchronizes the latest announced `audio.out.meta` segment text and cumulative duration back into runtime playback context while output is still speaking.
3. later `audio.out.mark` / `audio.out.cleared` / `audio.out.completed` continue to supply the heard boundary facts, but they now reconcile against the newest runtime-owned announced tail instead of waiting for final response settlement.
4. `internal/agent` continues to consume only `voice.previous.*`; adapters still must not invent their own resume heuristics.

This remains an internal ownership rule:

- no new public wire field is required
- device adapters still only forward transport facts
- `internal/voice` stays responsible for delivered-text, heard-text, missed-tail, and resume-anchor truth

## Consequences

Positive:

- soft overlap no longer always disappears from later continue or recap behavior
- `playback_completed=true` is no longer treated as proof that the user heard the full spoken reply
- segment ACK truth can combine exact heard boundaries with newer announced tail text before final response settlement
- the playback-truth chain becomes more useful to both next-turn agent behavior and early-audio output orchestration

Tradeoffs:

- soft-recovery retention still uses heuristics based on remaining tail size and overlap timing
- runtime playback state becomes more stateful during speaking and needs stronger regression coverage
- transport completion and user-heard completion are now intentionally decoupled, which requires documentation discipline

## Follow-Up

- keep tuning soft-recovery thresholds from real-device traces rather than moving the logic into adapters
- extend the same announced-playback-context rule to any future adapters that negotiate playback ACK
- consider finer cursor- or audibility-aware playback truth only after the current segment-mark contract is well validated
