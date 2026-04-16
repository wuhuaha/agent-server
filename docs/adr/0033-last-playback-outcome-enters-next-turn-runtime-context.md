# ADR 0033: Last Playback Outcome Enters Next-Turn Runtime Context

## Status

Accepted

## Context

`internal/voice.SessionOrchestrator` already owns playback truth collection:

- what text was delivered
- what text was probably heard
- whether playback completed or was interrupted
- where a future resume should anchor

That solved persistence and observability, but it still left one practical gap:

- the next user turn could reach `internal/agent` without an explicit runtime signal about what part of the previous spoken reply actually reached the user

Without that bridge, the shared runtime could remember `heard_text` in memory, yet immediate follow-up behavior such as:

- "继续"
- "后面呢"
- "你刚刚最后一句说什么"
- interruption-aware recap after barge-in

would still depend on indirect recall instead of a fresh turn-scoped playback context.

## Decision

Keep playback-truth derivation inside `internal/voice`, and surface the latest finalized playback outcome into the next shared turn as additive metadata.

For this slice:

- `SessionOrchestrator` keeps the latest finalized playback outcome as structured runtime state
- gateway adapters may project that outcome into the next `voice.TurnRequest.Metadata` as additive `voice.previous.*` fields
- `internal/agent` may consume those fields when building prompt sections or other runtime policy, but adapters still do not interpret playback facts themselves

Representative fields include:

- `voice.previous.heard_text`
- `voice.previous.missed_text`
- `voice.previous.resume_anchor`
- `voice.previous.heard_source`
- `voice.previous.heard_confidence`
- `voice.previous.response_interrupted`

## Consequences

Positive:

- continue or recap behavior can use a fresh turn-local playback boundary instead of relying only on generic memory recall
- the gateway stays an adapter: it forwards runtime-owned metadata, but playback-truth reasoning remains inside `internal/voice`
- the change is additive and internal; no wire-level protocol expansion is required

Tradeoffs:

- next-turn behavior still depends on an estimate, not perfect knowledge of human perception
- metadata shape is now a shared internal contract between `internal/voice` and `internal/agent`, so naming stability matters
- richer resume policies are still follow-up work; this ADR only lands the context bridge

## Follow-Up

- teach more runtime behaviors to consume the structured playback outcome directly, not only prompt text
- keep improving playback-fact precision through negotiated client ACK and segment marks
- later decide whether explicit resume directives need a first-class runtime contract beyond metadata
