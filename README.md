# agent-server

`agent-server` is a general-purpose multimodal agent service framework for RTOS devices, desktops, and external messaging channels.

Current priorities:

1. Keep the architecture stable and easy to evolve.
2. Land the RTOS voice fast path as early as possible.
3. Insert a reusable `Agent Runtime Core` before channel-specific orchestration grows.
4. Backfill authentication, tenancy, and policy without breaking the realtime contract.

## Repository Layout

- `cmd/agentd`: main service entrypoint.
- `internal/agent`: transport-neutral agent turn execution contracts, bootstrap runtime skeleton, and optional LLM runtime providers.
- `internal/gateway`: transport adapters such as realtime device ingress.
- `internal/session`: session state machine and turn lifecycle.
- `internal/voice`: voice pipeline contracts and future provider adapters.
- `internal/channel`: channel skill contracts for Feishu and similar platforms.
- `internal/control`: health, info, admin-oriented APIs, and built-in debug surfaces such as the Web/H5 realtime page.
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
- `GET /debug/realtime-h5/`
- `GET /debug/realtime-h5/settings.html`

For manual protocol bring-up and RTOS-side debugging:

```bash
cd clients/python-desktop-client
python -m pip install -e .
agent-server-desktop-client
```

For browser or H5 direct bring-up against the same native realtime websocket contract, open:

- `http://127.0.0.1:8080/debug/realtime-h5/settings.html`
- `http://127.0.0.1:8080/debug/realtime-h5/`
- [Web/H5 direct realtime adaptation guide](docs/protocols/web-h5-realtime-adaptation.md)

For a standalone repository tool that you can serve separately and use for manual test/debug:

```bash
cd tools/web-client
python3 serve.py --port 18081
```

Then open:

- `http://127.0.0.1:18081/settings.html`
- `http://127.0.0.1:18081/`
- [tools/web-client/README.md](tools/web-client/README.md)

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

The desktop runner report is now also a baseline quality artifact: it records discovery metadata plus per-scenario latency metrics and a top-level `quality_summary`, so different ASR/TTS/LLM configurations can be compared across archived JSON runs.

For firmware-side RTOS adaptation against the current native websocket and audio contract, see:

- [RTOS client adaptation checklist](docs/protocols/rtos-client-adaptation-checklist.md)

For existing `xiaozhi` firmware compatibility with minimal protocol churn, enable the adapter and use:

- [Xiaozhi compatibility WebSocket adapter](docs/protocols/xiaozhi-compat-ws-v0.md)
  This is now the detailed bring-up guide for OTA discovery, `hello`, `listen.start/stop/detect`, binary frame versions `1/2/3`, and troubleshooting.

For architecture-direction research on making the voice stack more companion-like instead of tool-like, see:

- [项目优化路线图（2026-04-04）](docs/architecture/project-optimization-roadmap-zh-2026-04.md)
- [语音 Agent 伙伴化研究（2026-04-04）](docs/architecture/voice-agent-companion-research-zh-2026-04.md)
- [Voice agent companion research (2026-04-04)](docs/architecture/voice-agent-companion-research-2026-04.md)

Compatibility adapter env switches live in `.env.example` under `AGENT_SERVER_XIAOZHI_*`.

For a one-command local smoke test that starts the worker and server, generates a WAV sample, runs the scripted validation, and writes a JSON report:

```powershell
cd E:\agent-server
.\scripts\smoke-funasr.ps1
.\scripts\smoke-rtos-mock.ps1 -EnableBargeIn
```

## Runtime Bring-Up Notes

The bootstrap runtime now ships with first real in-process backends behind the shared `internal/agent` contracts:

- memory provider `in_memory`: keeps a bounded recent-turn window per device, falling back to session scope when no device id exists
- tool provider `builtin`: exposes `time.now`, `session.describe`, and `memory.recall`
- compatibility fallback `noop`: still available through env config for isolated bring-up

Useful debug turns over the existing runtime boundary:

- `/memory` returns the currently remembered turn summary for the active device or session
- `/tool time.now {}` exercises builtin time lookup
- `/tool session.describe {}` returns the active runtime identifiers
- `/tool memory.recall {"query":"recent"}` returns structured memory recall JSON

Runtime backend config lives in `.env.example`:

```bash
AGENT_SERVER_AGENT_MEMORY_PROVIDER=in_memory
AGENT_SERVER_AGENT_MEMORY_MAX_TURNS=8
AGENT_SERVER_AGENT_TOOL_PROVIDER=builtin
AGENT_SERVER_AGENT_SKILLS=household_control
AGENT_SERVER_AGENT_LLM_PROVIDER=auto
AGENT_SERVER_AGENT_PERSONA=household_control_screen
AGENT_SERVER_AGENT_EXECUTION_MODE=simulation
AGENT_SERVER_AGENT_ASSISTANT_NAME=小欧管家
AGENT_SERVER_AGENT_DEEPSEEK_BASE_URL=https://api.deepseek.com
AGENT_SERVER_AGENT_DEEPSEEK_MODEL=deepseek-chat
```

Optional LLM-backed runtime config also lives under `AGENT_SERVER_AGENT_*`:

- `AGENT_SERVER_AGENT_SKILLS`: comma-separated runtime skill set layered over the shared core; current built-in option is `household_control`
- `auto`: prefer `deepseek_chat` when a DeepSeek key is present, otherwise stay on `bootstrap`
- `bootstrap`: keep the current echo or bring-up executor
- `deepseek_chat`: call DeepSeek's OpenAI-compatible chat completions API from inside the shared runtime boundary

When no custom system prompt is supplied, the runtime now uses a built-in family-control-screen assistant persona:

- positioned as a premium household smart-home voice assistant
- replies only in natural language
- stays cautious for locks, gas, security, and other sensitive home scenarios

The runtime now also separates persona from execution mode:

- `AGENT_SERVER_AGENT_PERSONA=household_control_screen`: built-in household control-screen assistant persona
- `AGENT_SERVER_AGENT_EXECUTION_MODE=simulation`: current debug-stage mode that gives simulated-success feedback without exposing that internal detail
- `AGENT_SERVER_AGENT_EXECUTION_MODE=dry_run`: describes the understood target action and expected effect without claiming real execution
- `AGENT_SERVER_AGENT_EXECUTION_MODE=live_control`: only uses completion-style confirmation when real execution results exist

Recommended DeepSeek env shape:

```bash
export AGENT_SERVER_AGENT_LLM_PROVIDER=deepseek_chat
export AGENT_SERVER_AGENT_PERSONA=household_control_screen
export AGENT_SERVER_AGENT_EXECUTION_MODE=simulation
export AGENT_SERVER_AGENT_ASSISTANT_NAME=小欧管家
export AGENT_SERVER_AGENT_DEEPSEEK_API_KEY=...
export AGENT_SERVER_AGENT_DEEPSEEK_MODEL=deepseek-chat
```

If you want to override the built-in persona template, set `AGENT_SERVER_AGENT_LLM_SYSTEM_PROMPT`. The runtime will replace `{{assistant_name}}` with `AGENT_SERVER_AGENT_ASSISTANT_NAME`, and will still append the configured execution-mode policy.

## Current Status

This repository is in bootstrap stage. The service now includes foundation endpoints, realtime discovery, a first WebSocket session handler for the `rtos-ws-v0` profile, implemented `barge-in` and timeout policy for realtime voice turns, optional speech-oriented `opus` uplink support normalized to `pcm16le/16000/mono` before ASR, a Python desktop debug client for text/audio/session bring-up, a built-in Web/H5 debug page that also speaks the native realtime contract, a scripted validation runner, an RTOS-oriented reference CLI, a directly usable local FunASR-backed ASR path, a MiMo-backed streaming `pcm16` TTS path, a first `Agent Runtime Core` skeleton that bootstrap text and ASR-completed turns can flow through, an optional DeepSeek-backed chat executor behind the same runtime boundary, and a `xiaozhi` compatibility adapter with OTA discovery plus protocol-version `1/2/3` binary frame support. The runtime now supports both materialized and true streaming turn execution, ordered `response.chunk` text/tool deltas, an in-process recent-turn memory backend, builtin local runtime tools, and one optional cloud LLM provider while keeping transport adapters unaware of those backend details. Channel adapters and longer-lived runtime backends are still planned follow-up work.
