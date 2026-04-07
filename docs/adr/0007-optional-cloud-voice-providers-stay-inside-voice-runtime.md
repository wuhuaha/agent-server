# ADR 0007: Optional Cloud Voice Providers Stay Inside Voice Runtime

## Status

Accepted

## Context

The first directly usable voice path already exists with local `FunASR` ASR and MiMo TTS, but validated cloud providers also exist for:

- iFlytek RTASR websocket ASR
- iFlytek websocket TTS
- Volcengine SSE TTS

Those providers use different wire protocols and authentication schemes. If transports or channel adapters learn those protocol details directly, the shared realtime session core becomes provider-aware and future channel work repeats the same branching.

## Decision

Keep all optional cloud ASR and TTS integrations inside `internal/voice` behind the existing `Transcriber`, `Synthesizer`, and `StreamingSynthesizer` contracts.

App bootstrap may select among:

- `funasr_http`
- `iflytek_rtasr`
- `mimo_v2_tts`
- `iflytek_tts_ws`
- `volcengine_tts`

The realtime websocket gateways, session state machine, and channel adapters continue to depend only on the shared responder and audio-stream contracts.

## Consequences

- Device-facing realtime protocol remains unchanged when switching voice providers.
- Voice-provider authentication and stream parsing live in one replaceable runtime layer.
- Cloud ASR can be adopted now without forcing protocol churn or provider-specific code into transports.
- The current ASR handoff stays turn-based at the session boundary even when the provider itself uses websocket streaming internally.
