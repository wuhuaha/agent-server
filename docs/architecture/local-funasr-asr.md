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
- KWS model default when enabled: `iic/speech_charctc_kws_phone-xiaoyun`
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
- the worker now background-preloads its configured models by default through `AGENT_SERVER_FUNASR_PRELOAD_MODELS=true`, so `/healthz` only reports `status=ok` after the configured final, online, KWS, and preview-VAD components are ready
- the local `run-agentd-local.sh` entrypoint now waits for the FunASR worker health endpoint to reach `status=ok` before starting `agentd` when `AGENT_SERVER_VOICE_PROVIDER=funasr_http`
- when `AGENT_SERVER_FUNASR_ONLINE_MODEL` is configured, the same worker routes preview through a true online ASR model while keeping turn-final text on the configured final-ASR model
- when `AGENT_SERVER_FUNASR_KWS_ENABLED=true`, the worker now treats missing `AGENT_SERVER_FUNASR_KWS_MODEL` or `AGENT_SERVER_FUNASR_KWS_KEYWORDS` as a health/config error, and the validated runtime path initializes `AutoModel(...)` with both `keywords` and `output_dir`
- the current calibrated KWS baseline on this machine is `iic/speech_charctc_kws_phone-xiaoyun`; it stays default-off, may emit `kws_detected` audio events when enabled, and still does not change the public realtime contract
- the local `SenseVoiceSmall` reference path on this machine now loads successfully only with `trust_remote_code=false`; enabling remote code causes model initialization to fail because the cached model bundle does not ship a `model` module
- the latest archived CPU benchmark on this machine shows that `SenseVoiceSmall + paraformer-zh-streaming + fsmn-vad` is operational after preload, but it is not yet the best default for a CPU demo:
  - command-only sample kept the correct text, but response-start latency rose from about `2058 ms` to about `3485 ms`
  - wake-word-prefixed sample still misrecognized `小欧管家 + 打开客厅灯` as `调管家。`
  - the short KWS alias `fsmn-kws` currently fails preload in the local FunASR `1.3.1` runtime with `fsmn-kws is not registered`, while the validated replacement `iic/speech_charctc_kws_phone-xiaoyun` only works reliably after passing `keywords` and `output_dir` during `AutoModel(...)` initialization

## 2026-04-15 GPU Deployment Note

- current production GPU host:
  - GPU: `Tesla V100-SXM2-32GB`
  - driver: `570.158.01`
  - reported CUDA runtime: `12.8`
  - hardware capability: `sm_70`
- the previous `xiaozhi-esp32-server` conda env should not be reused as the GPU runtime baseline on this machine:
  - it was found in a CPU-only or partially broken state (`torch 2.11.0+cpu`, `torchaudio` import failure) after an interrupted reinstall
  - the official `torch 2.11.0+cu128` wheel family also fails on V100 with `CUDA error: no kernel image is available for execution on the device`, because that wheel no longer contains `sm_70` kernels
- the validated replacement runtime is a dedicated data-volume worker Python at `/home/ubuntu/kws-training/data/agent-server-runtime/funasr-gpu-py311/bin/python` with:
  - `torch 2.7.1+cu126`
  - `torchaudio 2.7.1+cu126`
  - model and wheel caches rooted under `/home/ubuntu/kws-training/data/agent-server-cache/{modelscope,hf,torch}`
- the validated long-running GPU pipeline on this host is:
  - final ASR: `iic/SenseVoiceSmall`
  - online preview: `paraformer-zh-streaming`
  - preview/final VAD: `fsmn-vad`
  - device: `cuda:0`
  - pipeline mode: `stream_2pass_online_final`
  - KWS: still `disabled` by default
- live validation completed on the long-running GPU service after the cutover:
  - `GET http://127.0.0.1:8091/healthz` returned `status=ok`
  - `GET http://127.0.0.1:8091/v1/asr/info` reported `device=cuda:0`, `online_model_loaded=true`, and `stream_endpoint_fsmn_vad_loaded=true`
  - `POST http://127.0.0.1:8091/v1/asr/transcribe` recognized the cached sample `zh.mp3` as `开放时间早上9点至下午5点。` on `device=cuda:0` with worker elapsed time about `430 ms`
- local TTS remains `none` on this host for now. The current environment priority is low-latency ASR and turn detection on GPU first; CosyVoice or another local GPU TTS runtime should be installed as a separate follow-up.
