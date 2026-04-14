# ADR 0028: Local FunASR Worker Keeps 2pass ASR And KWS Internal

## Status

Accepted

## Context

The phase-1 voice demo needs better realtime spoken interaction quality, especially for:

- faster and steadier preview partials
- stronger acoustic endpoint evidence
- wake-word-prefixed short commands
- higher-quality turn-final correction

The repository already has a stable shared voice-runtime boundary:

- `internal/voice` owns provider choice and turn interpretation
- device adapters consume normalized preview/final deltas only
- the public realtime and `xiaozhi` contracts should not widen early just to experiment with local speech-model composition

The earlier local FunASR worker shape overloaded one final-ASR model for buffered preview, final recognition, and much of the wake-word and endpoint burden. That made voice-quality improvements tightly coupled and harder to iterate independently.

## Decision

Keep the local FunASR worker as the place where a modular local speech pipeline is composed, while preserving the existing shared worker HTTP contract.

Specifically:

- keep `/v1/asr/transcribe` and `/v1/asr/stream/*` as the worker surface consumed by `internal/voice`
- keep `stream_preview_batch` as the default stream mode for backward compatibility
- allow an optional internal 2pass mode when an online preview model is configured:
  - online ASR model for preview partials
  - separate final-ASR model for authoritative turn-final text
  - optional final-path FunASR VAD and punctuation models
- allow optional KWS inside the worker, but keep it disabled by default
- keep KWS effects internal to worker/runtime output:
  - worker `audio_events`
  - optional wake-word prefix stripping in preview/final transcript text
- do not add public protocol fields that make device adapters aware of KWS, VAD implementation, or worker model composition

## Consequences

- realtime voice quality can improve through worker-internal model composition without forcing a websocket or adapter contract change first
- device adapters stay thin and continue to depend on normalized `internal/voice` behavior instead of provider-specific model details
- the worker now has more runtime knobs, so health and runtime-configuration docs must expose pipeline mode and worker-side model status clearly
- KWS is opt-in and defaults off, which preserves current bring-up behavior while keeping the wake-word path ready for targeted A/B work
- future model swaps such as `paraformer-zh-streaming`, `fsmn-vad`, or a stronger final-ASR candidate can be tested behind the same worker boundary
