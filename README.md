# agent-server

`agent-server` is a general-purpose multimodal agent service framework for RTOS devices, desktops, and external messaging channels.

Current priorities:

1. Keep the architecture stable and easy to evolve.
2. Land the RTOS voice fast path as early as possible.
3. Reserve first-class extension points for channel skills such as Feishu.
4. Backfill authentication, tenancy, and policy without breaking the realtime contract.

## Repository Layout

- `cmd/agentd`: main service entrypoint.
- `internal/gateway`: transport adapters such as realtime device ingress.
- `internal/session`: session state machine and turn lifecycle.
- `internal/voice`: voice pipeline contracts and future provider adapters.
- `internal/channel`: channel skill contracts for Feishu and similar platforms.
- `internal/control`: health, info, and later admin APIs.
- `pkg/events`: shared event envelope types.
- `clients/python-desktop-client`: desktop debug client for realtime protocol bring-up.
- `docs/architecture`: architecture notes and boundaries.
- `docs/protocols`: protocol contracts and compatibility notes.
- `docs/adr`: architecture decision records.
- `.codex`: Codex-facing memory, logs, and project-local skills.
- `.claude`: Claude-facing context, commands, and review roles.

## Quick Start

```bash
go test ./...
go run ./cmd/agentd
```

Then open:

- `GET /healthz`
- `GET /v1/info`
- `GET /v1/realtime`

For manual protocol bring-up and RTOS-side debugging:

```bash
cd clients/python-desktop-client
python -m pip install -e .
agent-server-desktop-client
```

For directly usable local ASR bring-up with FunASR:

```powershell
cd E:\agent-server
.\scripts\start-funasr-worker.ps1
```

In another terminal:

```powershell
cd E:\agent-server
.\scripts\dev-funasr-mimo.ps1
```

For repeatable scripted validation of discovery, text, audio, and server-initiated close:

```bash
cd clients/python-desktop-client
python -m pip install -e .
agent-server-desktop-runner --scenario full --http-base http://127.0.0.1:8080
agent-server-rtos-mock --http-base http://127.0.0.1:8080 --wav .\sample.wav
```

For a one-command local smoke test that starts the worker and server, generates a WAV sample, runs the scripted validation, and writes a JSON report:

```powershell
cd E:\agent-server
.\scripts\smoke-funasr.ps1
.\scripts\smoke-rtos-mock.ps1 -EnableBargeIn
```

## Current Status

This repository is in bootstrap stage. The service now includes foundation endpoints, realtime discovery, a first WebSocket session handler for the `rtos-ws-v0` profile, implemented `barge-in` and timeout policy for realtime voice turns, optional speech-oriented `opus` uplink support normalized to `pcm16le/16000/mono` before ASR, a Python desktop debug client for text/audio/session bring-up, a scripted validation runner, an RTOS-oriented reference CLI, a directly usable local FunASR-backed ASR path, and a MiMo-backed streaming `pcm16` TTS path. VLM and channel adapters are still placeholder or planned follow-up work.
