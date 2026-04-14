# Python Workers

This package hosts Python-side workers for `agent-server`.

## Current Worker

- `agent_server_workers.funasr_service`
  - local HTTP ASR worker
  - designed for the existing `xiaozhi-esp32-server` conda environment
  - accepts normalized PCM16LE audio from the Go server and returns text transcription
  - supports batch `/v1/asr/transcribe` plus the local streaming lifecycle under `/v1/asr/stream/*`
  - keeps backward-compatible `stream_preview_batch` as the default stream mode
  - can switch to an internal 2pass path with `online preview + final ASR correction`
  - can optionally run KWS inside the worker, but `KWS` stays disabled by default and does not widen the public websocket contract

## Linux Install

From the repository root:

```bash
./scripts/install-linux-stack.sh --skip-desktop-client
```

To also install the optional local/open-source stream VAD runtime:

```bash
./scripts/install-linux-stack.sh --skip-desktop-client --with-stream-vad
```

## Start From Existing Conda Env

```powershell
cd E:\agent-server
.\scripts\start-funasr-worker.ps1
```

The default script targets:

- conda env: `xiaozhi-esp32-server`
- host: `127.0.0.1`
- port: `8091`
- model: `iic/SenseVoiceSmall`
- device: `cpu`
- `trust_remote_code`: `false`

CPU remains the default bring-up target so local runs do not claim the GPU unless requested.
The current `xiaozhi-esp32-server` env on this machine has been upgraded to `torch 2.11.0+cu128` / `torchaudio 2.11.0+cu128`, and `SenseVoiceSmall` has been validated on `cuda:0` with the local RTX 5060.
`trust_remote_code` stays disabled for the local `SenseVoiceSmall` path because the downloaded model bundle does not include remote code files and local load fails when it is enabled.

## Manual Start

```powershell
$env:PYTHONPATH='E:\agent-server\workers\python\src'
conda run -n xiaozhi-esp32-server python -m agent_server_workers.funasr_service --host 127.0.0.1 --port 8091 --device cpu
```

For GPU validation on this machine, switch the device argument to `cuda:0`.

## 2pass / Preview / KWS Tuning

The worker now supports two internal stream modes:

- default `stream_preview_batch`
  - keeps the previous compatibility behavior
  - worker preview comes from re-running the final ASR model on the buffered audio
- optional `stream_2pass_online_final`
  - enabled when `AGENT_SERVER_FUNASR_ONLINE_MODEL` is non-empty
  - worker preview comes from a true streaming online ASR model
  - turn-final text still comes from the configured final ASR model

### Online Preview / 2pass

- `AGENT_SERVER_FUNASR_ONLINE_MODEL`
  - empty by default, which keeps `stream_preview_batch`
  - set this to an online ASR model such as `paraformer-zh-streaming` to enable `stream_2pass_online_final`
- `AGENT_SERVER_FUNASR_STREAM_CHUNK_SIZE`
  - FunASR online chunk tuple, default `0,10,5`
- `AGENT_SERVER_FUNASR_STREAM_ENCODER_CHUNK_LOOK_BACK`
  - default `4`
- `AGENT_SERVER_FUNASR_STREAM_DECODER_CHUNK_LOOK_BACK`
  - default `1`
- `AGENT_SERVER_FUNASR_STREAM_PREVIEW_MIN_AUDIO_MS`
  - minimum buffered audio before the worker emits the first preview attempt
  - default: `320`
- `AGENT_SERVER_FUNASR_STREAM_PREVIEW_MIN_INTERVAL_MS`
  - batch-preview only; minimum interval between repeated buffered preview attempts
  - default: `240`

### Final ASR / Final VAD / Final Punctuation

- `AGENT_SERVER_FUNASR_MODEL`
  - final-ASR model, default `iic/SenseVoiceSmall`
- `AGENT_SERVER_FUNASR_FINAL_VAD_MODEL`
  - optional final-path FunASR VAD model, default empty
- `AGENT_SERVER_FUNASR_FINAL_PUNC_MODEL`
  - optional final-path punctuation model, default empty
- `AGENT_SERVER_FUNASR_FINAL_MERGE_VAD`
  - whether to merge adjacent VAD clips before final ASR when a final VAD model is configured
  - default: `true`
- `AGENT_SERVER_FUNASR_FINAL_MERGE_LENGTH_S`
  - final VAD merge window in seconds
  - default: `15`

### KWS

- `AGENT_SERVER_FUNASR_KWS_ENABLED`
  - whether to enable KWS inside the worker
  - default: `false`
- `AGENT_SERVER_FUNASR_KWS_MODEL`
  - KWS model id used only when `KWS` is enabled
  - default: `fsmn-kws`
- `AGENT_SERVER_FUNASR_KWS_KEYWORDS`
  - comma-separated keyword list
  - default: empty
- `AGENT_SERVER_FUNASR_KWS_STRIP_MATCHED_PREFIX`
  - when `true`, strips the detected wake-word prefix from preview/final transcript text
  - default: `true`
- `AGENT_SERVER_FUNASR_KWS_MIN_AUDIO_MS`
  - minimum buffered audio before the worker attempts KWS
  - default: `480`
- `AGENT_SERVER_FUNASR_KWS_MIN_INTERVAL_MS`
  - minimum interval between repeated KWS checks on one stream
  - default: `400`

KWS remains worker-internal:

- it is off by default
- it only annotates worker results with `audio_events` and optional prefix stripping
- it does not change the public realtime or `xiaozhi` protocol shapes

### Endpoint Hints / VAD

- `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_TAIL_MS`
  - tail-audio window used for the worker's lightweight preview endpoint hint
  - default: `160`
- `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_MEAN_ABS_THRESHOLD`
  - mean-absolute PCM threshold below which the tail window is treated as silence
  - default: `180`
- `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_PROVIDER`
  - preview endpoint hint provider selection
  - `energy`: default lightweight tail-energy hint
  - `fsmn_vad`: use the configured `AGENT_SERVER_FUNASR_FINAL_VAD_MODEL` as the worker-side endpoint hint source
  - `silero`: prefer `Silero VAD`; if the runtime is unavailable or the audio format is unsupported, fall back to `energy`
  - `auto`: prefer `fsmn_vad` when `AGENT_SERVER_FUNASR_FINAL_VAD_MODEL` is configured, otherwise try `Silero VAD`, then fall back to `energy`
  - `none`: disable worker-side preview endpoint hints
  - default: `energy`
- `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_THRESHOLD`
  - VAD threshold passed to `Silero VAD`
  - default: `0.5`
- `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_MIN_SILENCE_MS`
  - minimum trailing silence required before the worker emits `preview_fsmn_vad_silence` or `preview_silero_vad_silence`
  - default: `160`
- `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_SPEECH_PAD_MS`
  - speech padding passed to `Silero VAD`
  - default: `30`

Optional local/open-source `Silero VAD` install:

```bash
./scripts/install-linux-stack.sh --skip-desktop-client --with-stream-vad
```

The current stream path stays conservative by default:

- `AGENT_SERVER_FUNASR_ONLINE_MODEL` stays empty, so existing bring-up still uses `stream_preview_batch`
- `AGENT_SERVER_FUNASR_KWS_ENABLED=false`, so wake-word detection is opt-in
- `energy` remains the default endpoint-hint source, so existing bring-up behavior does not change unexpectedly
- worker-side `fsmn-vad`, `Silero VAD`, and `KWS` all stay behind the same HTTP worker boundary; they do not widen the public websocket or `xiaozhi` compatibility protocol
- `/healthz` and `/v1/asr/info` now expose the active pipeline mode plus online/final/KWS/VAD config and runtime status

## Health Check

- `GET http://127.0.0.1:8091/healthz`
- `GET http://127.0.0.1:8091/v1/asr/info`

`/v1/asr/info` now advertises the currently enabled batch and streaming routes together with:

- `pipeline_mode`
- `online_model`
- `final_vad_model`
- `final_punc_model`
- `kws_enabled`
- `kws_keywords`
- worker-side endpoint-hint provider and lazy runtime status
