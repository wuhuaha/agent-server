# ADR 0015: Xiaozhi STT Echo Comes From Shared Responder Output

## Status

Accepted

## Context

The `xiaozhi` compatibility adapter already emitted `stt` for `listen.detect` text turns, but audio turns still had no transcript echo. Adding audio-turn `stt` was important for RTOS bring-up and user feedback, yet pulling transcript logic directly out of ASR providers inside the adapter would have broken the transport boundary.

## Decision

Keep transcript ownership in the shared voice runtime.

The `xiaozhi` adapter now emits audio-turn `stt` only when the shared responder returns normalized input text in its transport-neutral response object. The adapter remains responsible for compat event timing and JSON framing, but not for ASR result parsing.

## Consequences

- The compat path now provides audio-turn `stt` without becoming ASR-provider-aware.
- Transcript echo remains aligned with the shared responder path used by other transports.
- Future ASR providers can populate normalized input text once in the voice runtime and automatically benefit the compat adapter without adapter-specific parsing logic.
