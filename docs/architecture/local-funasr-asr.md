# Local FunASR ASR Bring-Up

## Goal

Replace the bootstrap audio-byte-count reply with real local speech recognition while keeping the existing Go realtime gateway and RTOS protocol unchanged.

## Runtime Topology

- `agentd` remains the main realtime service
- a local Python worker exposes `POST /v1/asr/transcribe`
- Go sends normalized `pcm16le` turn audio to the worker after `audio.in.commit`
- the worker uses `FunASR` to return recognized text
- `agentd` sends `response.start` and `response.chunk` back on the realtime socket

## Why This Split

- Go stays responsible for device ingress, session state, and protocol stability
- Python stays responsible for model loading and inference
- the ASR boundary is simple enough to replace later with streaming ASR, cloud ASR, or a GPU-upgraded local stack

## Current Local Reference

- worker env: `xiaozhi-esp32-server`
- model: `iic/SenseVoiceSmall`
- device: `cpu`
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
- the server now has a directly usable MiMo streaming `pcm16` TTS path for spoken responses
- the local `SenseVoiceSmall` reference path on this machine now loads successfully only with `trust_remote_code=false`; enabling remote code causes model initialization to fail because the cached model bundle does not ship a `model` module
- the remaining ASR limitation is that the worker boundary is still turn-based HTTP rather than streaming incremental ASR
