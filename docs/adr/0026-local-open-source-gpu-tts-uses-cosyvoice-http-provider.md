# ADR 0026: Local Open-Source GPU TTS Uses CosyVoice HTTP Provider Behind Voice Runtime

## Status

Accepted

## Context

`agent-server` already had a local open-source ASR path through `FunASR`, but TTS options in the shared runtime were still cloud-only or disabled.

The project needs one directly usable local open-source TTS option for GPU deployments without violating the current architecture constraints:

- websocket gateways stay transport-only
- device and Web/H5 adapters must not learn provider protocols
- TTS provider details should remain replaceable behind `internal/voice`

Vendoring a full local TTS model runtime directly into `agent-server` would also make the repository heavier and couple the service too tightly to one Python model stack.

## Decision

Add `cosyvoice_http` as a new shared TTS provider under `internal/voice`, targeting the official CosyVoice FastAPI service as a local GPU-side dependency.

This keeps the boundary shape consistent with the existing local ASR pattern:

- `agentd` selects the provider at bootstrap
- `internal/voice` owns provider request construction, audio normalization, and streaming or buffered synthesis behavior
- gateways continue to consume only shared responder and audio-stream contracts

The first supported CosyVoice modes are:

- `sft`
- `instruct`

The provider normalizes CosyVoice raw PCM output to the configured realtime output profile before audio reaches any websocket adapter.

## Consequences

- `agent-server` now has one local open-source GPU TTS option without changing the public realtime or `xiaozhi` compatibility protocols
- GPU deployment of the TTS model is delegated to the official CosyVoice runtime or image, while `agent-server` keeps a stable local HTTP integration boundary
- future local TTS providers can follow the same `internal/voice` contract without leaking model-specific details into adapters
