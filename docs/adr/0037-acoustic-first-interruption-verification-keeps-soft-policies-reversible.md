# ADR 0037: Acoustic-First Interruption Verification Keeps Soft Policies Reversible

## Status

Accepted

## Context

The repository already has shared interruption policies inside `internal/voice` and the native realtime gateway can now apply `duck_only` and `backchannel` as real playout effects instead of log-only labels.

However, the previous `EvaluateBargeIn(...)` path still behaved too much like a transcript shortcut:

- it relied primarily on `audioMs + looksLikeBackchannel + looksLexicallyComplete`
- it did not promote audio-only intrusion into a reversible soft-output state early enough
- it did not expose enough evidence for observability and later policy refinement
- it could not clearly separate fast acoustic intrusion detection from slower semantic takeover confirmation

The current phase-1 voice-demo goal is perceived realtime quality first, so interruption should become more natural without widening the wire contract or pushing more complexity into RTOS clients.

## Decision

The shared interruption verifier now follows an acoustic-first, semantic-confirmation shape while keeping the existing public policy names unchanged:

1. use a lower-latency acoustic gate to recognize speaking-time intrusion before full transcript evidence is ready
2. keep `duck_only` as the default reversible holding state when acoustic intrusion exists but takeover is not yet confirmed
3. let stronger semantic evidence such as lexical completeness, explicit takeover lexicon, or preview accept-candidate state upgrade to `hard_interrupt`
4. keep short acknowledgements as `backchannel` even when they are lexically complete
5. expose structured verifier evidence in runtime logs so later tuning can use real traces instead of guessing from final policy labels alone

The shared runtime contract remains internal:

- transports still consume the same `ignore / backchannel / duck_only / hard_interrupt` policy set
- no new realtime wire events are required
- `internal/gateway` stays an adapter that applies a runtime-owned `PlaybackDirective`

## Consequences

Positive:

- audio-only intrusion can now duck output earlier instead of waiting for transcript text
- explicit takeover phrasing can interrupt sooner without forcing transcript-only heuristics
- logs now carry acoustic and semantic evidence fields that make later tuning much easier
- the current architecture stays aligned with `internal/voice` owning interruption arbitration

Tradeoffs:

- verifier behavior becomes richer and needs stronger regression coverage
- some signals are still heuristic because full acoustic features are not yet available from the local workers
- `duck_only` timing and escalation thresholds still need later refinement from real-device traces

## Follow-Up

- use the new evidence fields to tune `duck_only` hold and escalation windows from archived runs
- extend the same runtime-owned verifier shape beyond native realtime into the remaining adapters
- keep the next slice focused on clause-aware output orchestration instead of widening protocol surface
