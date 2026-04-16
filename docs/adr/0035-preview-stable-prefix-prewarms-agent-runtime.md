# ADR 0035: Preview Stable Prefix Prewarms The Agent Runtime

## Status

Accepted

## Context

The previous slice already reused preview finalization at accept time:

- streaming preview ASR no longer always had to replay the whole buffered PCM after accept
- explicit commit and server-endpoint accept could both benefit from that fast path

That still left another latency pocket on the accepted-turn critical path:

- `internal/agent` often waited until accept time to load turn memory
- prompt sections and tool definitions were still assembled only after the turn was fully accepted
- preview text already contained useful evidence about a converged user prefix, but that evidence was not yet used by the runtime

Promoting preview directly into a new public accept semantic would blur boundaries:

- preview events are still observational
- gateway adapters must not become a second turn-orchestration layer
- speculative early work must remain reversible until the runtime actually accepts the turn

## Decision

Keep the new slice runtime-owned and reversible.

For this slice:

- `internal/voice.SilenceTurnDetector` derives a `stable_prefix` and `utterance_complete` hint from preview deltas
- preview-aware websocket paths may expose that `stable_prefix` and a derived `stability` ratio as observational fields only
- `ASRResponder` may trigger a bounded `TurnPrewarmer` call when preview text is stable enough, lexically complete enough, and long enough to justify prewarm
- `internal/agent.LLMTurnExecutor` may cache prewarmed system prompt, memory context, and tool catalog per session
- accepted turns may reuse that cache only when the final accepted user text and non-preview metadata match exactly

Matching intentionally ignores transient `voice.preview.*` metadata so speculative preview bookkeeping does not prevent reuse, but it still requires the same session, device, client type, accepted text, and stable metadata signature.

## Consequences

Positive:

- prompt, memory, and tool preparation can start before turn acceptance without widening the public protocol
- preview-aware clients gain more faithful `stable_prefix` and `stability` observations for UI and debug
- speculative work stays bounded and reversible because reuse is exact-match only
- ownership remains clean: `internal/voice` produces preview evidence, `internal/agent` consumes it through a shared runtime interface, and adapters still only relay observations

Tradeoffs:

- some prewarm work may be wasted when the user keeps speaking or the final accepted text diverges from the preview
- exact-match reuse is intentionally conservative, so it will miss some near-match opportunities
- this slice improves accepted-turn startup latency, but it still does not authorize irreversible early tool execution or public early-accept semantics

## Follow-Up

- add explicit observability for prewarm hit versus miss on accepted turns
- later consider richer early-planning tiers beyond exact-match reuse, but keep them runtime-internal and reversible
- continue treating `accept_reason`, not preview events, as the public accepted-turn signal
