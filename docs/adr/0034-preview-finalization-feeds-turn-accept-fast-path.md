# ADR 0034: Preview Finalization Feeds The Accepted-Turn Fast Path

## Status

Accepted

## Context

The shared voice runtime already supported hidden preview streaming:

- device audio uplink fed an `InputPreviewSession`
- preview partials and server-endpoint suggestions came from that session
- once the turn was finally accepted, the normal audio-turn path still replayed the buffered PCM through ASR again

That shape preserved boundaries, but it wasted latency on a common path:

- the preview streaming ASR session had already consumed the audio incrementally
- explicit `audio.in.commit` and server-endpoint auto-commit still often paid for a second full transcription pass before the agent could think or speak

## Decision

Keep preview ownership inside `internal/voice`, and allow preview sessions to expose an optional finalize capability that can be reused at accept time.

For this slice:

- `FinalizingInputPreviewSession` is an internal shared runtime capability
- `internal/voice.SessionOrchestrator` may finalize the active preview session when a turn is accepted
- gateway adapters may consume that finalized preview result and project it into the accepted `voice.TurnRequest`
- `ASRResponder` may prefer that finalized preview transcription over replaying buffered audio from scratch

If a preview session does not support finalization, the runtime falls back to the existing replay-based ASR path.

## Consequences

Positive:

- accepted audio turns can skip redundant ASR replay when the preview stream already has the needed final result
- explicit client commit and server-endpoint auto-commit both benefit without widening the public websocket contract
- the gateway remains an adapter: it triggers finalize-or-fallback, but ASR and preview ownership stay inside `internal/voice`

Tradeoffs:

- preview-session lifecycle becomes more important, because accept-time cleanup now has a fast-path and a fallback path
- preview finalization quality still depends on the underlying streaming transcriber implementation
- this slice reduces commit-to-text latency, but it does not by itself solve all earlier-thinking or earlier-speaking opportunities

## Follow-Up

- add observability for when accepted turns use preview-finalize fast path versus replay fallback
- later consider whether stable-prefix evidence should also prewarm agent-planning work before full commit
- keep the public protocol unchanged until a concrete client-facing gap is proven
