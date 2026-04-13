# Python Workers

This package hosts Python-side workers for `agent-server`.

## Current Worker

- `agent_server_workers.funasr_service`
  - local HTTP ASR worker
  - designed for the existing `xiaozhi-esp32-server` conda environment
  - accepts normalized PCM16LE audio from the Go server and returns text transcription
  - supports both batch `/v1/asr/transcribe` and the local streaming preview lifecycle under `/v1/asr/stream/*`

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

## Stream Preview Tuning

The worker can emit local preview partials by repeatedly re-running FunASR on the buffered audio during an active stream.

- `AGENT_SERVER_FUNASR_STREAM_PREVIEW_MIN_AUDIO_MS`
  - minimum buffered audio before the worker attempts the first preview
  - default: `320`
- `AGENT_SERVER_FUNASR_STREAM_PREVIEW_MIN_INTERVAL_MS`
  - minimum interval between preview attempts on the same stream
  - default: `240`
- `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_TAIL_MS`
  - tail-audio window used for the worker's lightweight preview endpoint hint
  - default: `160`
- `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_MEAN_ABS_THRESHOLD`
  - mean-absolute PCM threshold below which the tail window is treated as silence
  - default: `180`
- `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_PROVIDER`
  - preview endpoint hint provider selection
  - `energy`: default lightweight tail-energy hint
  - `silero`: prefer `Silero VAD`; if the runtime is unavailable or the audio format is unsupported, fall back to `energy`
  - `auto`: try `Silero VAD` first, otherwise fall back to `energy`
  - `none`: disable worker-side preview endpoint hints
  - default: `energy`
- `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_THRESHOLD`
  - VAD threshold passed to `Silero VAD`
  - default: `0.5`
- `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_MIN_SILENCE_MS`
  - minimum trailing silence required before the worker emits `preview_silero_vad_silence`
  - default: `160`
- `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_SPEECH_PAD_MS`
  - speech padding passed to `Silero VAD`
  - default: `30`

Optional local/open-source VAD install:

```bash
./scripts/install-linux-stack.sh --skip-desktop-client --with-stream-vad
```

The current stream preview path stays conservative by default:

- `energy` remains the default endpoint-hint source, so existing bring-up behavior does not change unexpectedly
- `Silero VAD` only strengthens worker-side hinting; it does not widen the public websocket or `xiaozhi` compatibility protocol
- `/healthz` and `/v1/asr/info` now expose the configured VAD provider plus lazy runtime status and any fallback error string

## Health Check

- `GET http://127.0.0.1:8091/healthz`
- `GET http://127.0.0.1:8091/v1/asr/info`

`/v1/asr/info` now advertises the currently enabled batch and streaming routes.
