# Local FunASR ASR Bring-Up

## Goal

Replace the bootstrap audio-byte-count reply with real local speech recognition while keeping the existing Go realtime gateway and RTOS protocol unchanged.

## Runtime Topology

- `agentd` remains the main realtime service
- a local Python worker exposes:
  - `POST /v1/asr/transcribe`
  - `POST /v1/asr/stream/start`
  - `POST /v1/asr/stream/push`
  - `POST /v1/asr/stream/finish`
  - `POST /v1/asr/stream/close`
- Go still sends normalized `pcm16le` audio to the worker behind the shared `internal/voice` boundary
- hidden preview and server-side endpointing continue to consume the worker only through the shared `StreamingTranscriber` contract
- the worker now supports two internal stream modes:
  - `stream_preview_batch`: buffered compatibility mode, still the default
  - `stream_2pass_online_final`: optional 2pass mode with online preview plus final-ASR correction
- optional worker-side `KWS`, `fsmn-vad`, and `Silero VAD` all stay inside the worker/runtime boundary and do not change the public websocket or `xiaozhi` contracts

## Why This Split

- Go stays responsible for device ingress, session state, and protocol stability
- Python stays responsible for model loading and inference
- `internal/voice` stays responsible for selecting providers and interpreting preview/final output, instead of teaching device adapters about model-serving details
- KWS, preview, endpoint hints, and final-ASR correction can evolve inside the same worker boundary without widening the realtime protocol early

## Current Local Reference

- worker env: `xiaozhi-esp32-server`
- final-ASR model default: `iic/SenseVoiceSmall`
- online preview model default: empty, so the worker still defaults to `stream_preview_batch`
- final VAD model default: empty
- final punctuation model default: empty
- KWS default: disabled
- device default: `cpu`
- `trust_remote_code`: `false`
- input format: `pcm16le / 16000 Hz / mono`

## Install Entry Point

On Linux, repository-local dependency bring-up now goes through:

```bash
./scripts/install-linux-stack.sh --skip-desktop-client
```

Add `--with-stream-vad` when the worker should also install `onnxruntime` and `silero-vad` for stronger local preview endpoint hints.

## Current Status

- the gateway can now normalize supported speech-oriented `opus` uplink packets to `pcm16le/16000/mono` before calling the worker
- the worker now supports an internal modular path of `optional KWS + optional worker-side VAD + optional online preview + final ASR`
- the default worker configuration stays conservative and backward-compatible:
  - `AGENT_SERVER_FUNASR_ONLINE_MODEL` is empty by default, so streaming still starts in `stream_preview_batch`
  - `AGENT_SERVER_FUNASR_KWS_ENABLED=false`, so wake-word detection is opt-in
  - `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_PROVIDER=energy`, so the default endpoint hint remains lightweight and dependency-free
- when `AGENT_SERVER_FUNASR_ONLINE_MODEL` is configured, the same worker routes preview through a true online ASR model while keeping turn-final text on the configured final-ASR model
- when `AGENT_SERVER_FUNASR_KWS_ENABLED=true`, the worker may emit `kws_detected` audio events and optionally strip the matched wake-word prefix from preview/final transcript text, but the public realtime contract still stays unchanged
- the local `SenseVoiceSmall` reference path on this machine now loads successfully only with `trust_remote_code=false`; enabling remote code causes model initialization to fail because the cached model bundle does not ship a `model` module
