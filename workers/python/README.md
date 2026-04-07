# Python Workers

This package hosts Python-side workers for `agent-server`.

## Current Worker

- `agent_server_workers.funasr_service`
  - local HTTP ASR worker
  - designed for the existing `xiaozhi-esp32-server` conda environment
  - accepts normalized PCM16LE audio from the Go server and returns text transcription

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

## Health Check

- `GET http://127.0.0.1:8091/healthz`
- `GET http://127.0.0.1:8091/v1/asr/info`
