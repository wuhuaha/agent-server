# ADR 0025: Channel Adapters Use A Shared Runtime Bridge

## Status

Accepted

## Context

The repository already had channel adapter contracts, but no shared handoff path. Without one, the first Feishu or Slack adapter would have been tempted to:

- normalize inbound messages in one file
- call `internal/agent` or even model providers directly in another file
- open-code delivery status, retry metadata, and thread mapping differently per adapter

That would have recreated the same drift risk that the repository already removed from the websocket adapters.

## Decision

Add a shared bridge under `internal/channel` that fixes the channel adapter lifecycle as:

1. normalize inbound message
2. hand the normalized turn to the shared `Agent Runtime Core`
3. deliver the runtime response through the adapter
4. report delivery outcome

For the current slice:

- adapters still implement `Normalize(...)` and `Deliver(...)`
- `internal/channel.RuntimeBridge` owns the shared normalize -> runtime -> deliver flow
- the bridge maps message ids, thread ids, session ids, idempotency keys, attachments, and delivery status into one shared handoff path
- channel adapters continue to depend only on `agent.TurnExecutor`, not on provider-specific APIs

## Consequences

Positive:

- the first external channel can stay a true adapter over the shared runtime
- thread mapping, idempotency metadata, and delivery status gain one reusable shape
- adding a second channel should reuse the same bridge instead of copying the first adapter's orchestration

Tradeoffs:

- the first bridge covers the text-response baseline only; richer channel-native actions may need additive adapter hooks later
- channel-specific retry persistence is still adapter-specific until the project adds a deeper channel delivery store

## Follow-Up

- add the first Feishu adapter skeleton on top of the shared bridge
- decide whether future channel adapters need streaming turn output or whether turn-complete delivery remains enough for the first channel milestone
