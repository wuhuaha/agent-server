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

CPU is the default because the currently installed `torch 2.2.2+cu121` in that env is not compatible with the local RTX 5060 GPU for this model.

## Manual Start

```powershell
$env:PYTHONPATH='E:\agent-server\workers\python\src'
conda run -n xiaozhi-esp32-server python -m agent_server_workers.funasr_service --host 127.0.0.1 --port 8091 --device cpu
```

## Health Check

- `GET http://127.0.0.1:8091/healthz`
- `GET http://127.0.0.1:8091/v1/asr/info`
