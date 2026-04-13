# ADR 0023: Gateway Adapters Share Turn Flow Before Voice Migration

## Status

Accepted

## Context

The native realtime websocket adapter and the `xiaozhi` compatibility adapter had both grown their own copies of the same turn lifecycle:

- accept input
- commit turn
- enter `thinking`
- start response streaming
- enter `speaking`
- return to `active` or end the session

That duplication already made bug fixes drift-prone, especially around response start timing, streamed deltas, interruption, and playback completion. At the same time, the repository is not ready to move all preview, interrupt arbitration, and playout ownership into `internal/voice` in one step yet.

## Decision

Add a shared gateway-side turn-flow layer first, then migrate ownership deeper into `internal/voice` later.

For the current slice:

- `internal/gateway` owns shared helpers for turn response execution, streamed delta emission, speaking/active completion, and interruption return-to-active handling
- native realtime and `xiaozhi` adapters call those helpers instead of keeping separate copies of the same lifecycle
- protocol shapes stay unchanged
- provider access still stays behind `internal/voice` and `internal/agent`

This is explicitly an intermediate refactor, not the final architecture target. The final ownership for preview, interruption arbitration, and playout callbacks is still expected to move into `internal/voice`.

## Consequences

Positive:

- gateway adapters become thinner without changing the published protocol
- bug fixes in shared turn progression now land in one place
- the next migration stage can move shared orchestration responsibilities out of gateway from a single helper layer instead of two divergent handlers

Tradeoffs:

- `internal/gateway` still temporarily owns more lifecycle logic than the target architecture wants
- the repository now carries a conscious intermediate layer that should eventually shrink again once `internal/voice` takes over more orchestration

## Follow-Up

- move preview polling, auto-commit, interruption arbitration, and playout completion ownership from gateway helpers into `internal/voice`
- extend memory and playout reporting so the runtime can distinguish generated text from text the user actually heard
