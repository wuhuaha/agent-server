# Local CosyVoice GPU TTS Bring-Up

## Goal

Add a directly usable local open-source GPU TTS path without changing the device-facing realtime protocol.

## Provider Boundary

- `agentd` selects `cosyvoice_http` at bootstrap
- provider-specific HTTP calls stay inside `internal/voice`
- gateways and channel adapters still consume only shared synthesized-audio contracts
- CosyVoice raw PCM output is normalized to the configured realtime output sample rate before downlink

## Current Supported Modes

- `sft`
- `instruct`

The current default mode is `sft`.

## Required Environment

- `AGENT_SERVER_TTS_PROVIDER=cosyvoice_http`
- `AGENT_SERVER_TTS_COSYVOICE_BASE_URL=http://127.0.0.1:50000`
- `AGENT_SERVER_TTS_COSYVOICE_MODE=sft`
- `AGENT_SERVER_TTS_COSYVOICE_SPK_ID=中文女`

Optional:

- `AGENT_SERVER_TTS_COSYVOICE_INSTRUCT_TEXT`
- `AGENT_SERVER_TTS_COSYVOICE_SOURCE_SAMPLE_RATE=22050`

## Recommended GPU Bring-Up

### Option 1: Official CosyVoice Runtime

Prepare the official CosyVoice repository and GPU conda environment, then start its FastAPI server through the helper script:

```bash
./scripts/start-cosyvoice-fastapi.sh \
  --repo-dir /path/to/CosyVoice \
  --conda-env cosyvoice \
  --port 50000 \
  --model-dir iic/CosyVoice-300M-SFT
```

Then start `agentd` against local FunASR + local CosyVoice:

```bash
./scripts/dev-funasr-cosyvoice.sh
```

### Option 2: Docker GPU Overlay

The repository now provides a layered Docker overlay for `agentd + cosyvoice-tts`:

- [deploy/docker/compose.local-tts-gpu.yml](/root/agent-server/deploy/docker/compose.local-tts-gpu.yml)

Expected flow:

1. build the official CosyVoice runtime image locally as `cosyvoice:v1.0`
2. copy `deploy/docker/.env.docker.example` to `deploy/docker/.env.docker`
3. run:

```bash
docker compose -f deploy/docker/compose.base.yml -f deploy/docker/compose.local-tts-gpu.yml up
```

## Notes

- this slice adds one local open-source GPU TTS option; it does not yet replace existing cloud TTS providers
- the current provider integrates against the official CosyVoice FastAPI service instead of vendoring the full model stack into `agent-server`
- the current implementation keeps the shared realtime output profile unchanged, so adapters still receive normalized `pcm16le`
