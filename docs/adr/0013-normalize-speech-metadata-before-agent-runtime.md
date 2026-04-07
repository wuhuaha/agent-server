# ADR 0013: Normalize Speech Metadata Before Agent Runtime

## Status

Accepted

## Context

The first ASR-backed runtime path only forwarded final transcript text into `internal/agent`. That kept the boundary simple, but it discarded useful speech-understanding signals such as:

- detected language
- speaker identity
- endpoint reason
- partial hypotheses
- coarse audio events

Different ASR providers expose different payload shapes for those signals. Passing provider-native result structures through transports or directly into `internal/agent` would have leaked provider coupling across the runtime boundary.

## Decision

Keep provider-native ASR parsing inside `internal/voice`, then normalize any available speech-understanding signals into shared turn metadata before calling `internal/agent`.

The first normalized metadata slice includes keys under the `speech.*` namespace, such as:

- `speech.language`
- `speech.emotion`
- `speech.speaker_id`
- `speech.endpoint_reason`
- `speech.audio_events`
- `speech.partials`
- `speech.model`
- `speech.device`

List-shaped values remain encoded inside metadata strings so the shared runtime input contract stays transport-neutral and provider-agnostic.

## Consequences

- The agent runtime can consume richer speech context without knowing provider payload formats.
- Gateway and device protocols do not need to change for this first structured-speech slice.
- Providers that do not expose a given field remain compatible; missing values simply do not appear in turn metadata.
- Future deterministic routing, memory policy, or TTS-style logic can inspect normalized `speech.*` metadata without importing ASR-specific packages.
