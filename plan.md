# plan.md

## Working Goal

Create a directly usable foundation for `agent-server` with stable architecture boundaries, local development conventions, and the first implementation path toward RTOS realtime voice sessions.

## Confirmed Decisions

- Use a modular monolith first.
- Use Go for the main service runtime.
- Reserve Python for future voice and vision workers.
- Make `Realtime Session Core` the center of the system.
- Insert an `Agent Runtime Core` before channel adapters so transport layers do not own agent policy.
- Use `WebSocket + binary audio + JSON control events` for the first RTOS path.
- Keep authentication out of the critical path for v0.1, but reserve protocol fields for it.
- Model Feishu-like integrations as `channel skills`, not plain tools.
- Make the first real runtime memory and tool backends in-process so the boundary becomes useful before external services are introduced.

## Current Milestones

### M0 Foundation

- [x] Create the repository and baseline directories.
- [x] Create `.claude/`, `.codex/`, `AGENTS.md`, and `plan.md`.
- [x] Create first-pass architecture, protocol, and ADR documents.
- [x] Create project-local skills for future coding sessions.
- [x] Create the initial Go service skeleton.
- [x] Add a minimal schema for realtime envelopes.
- [x] Freeze the first RTOS device wire profile and runtime configuration defaults in docs.

### M1 RTOS Voice Fast Path

- [x] Accept realtime session start from device side.
- [x] Accept audio uplink frames.
- [x] Return streamed response events.
- [x] Allow both client and server to end the session.
- [x] Validate a directly usable local ASR + MiMo TTS voice path.
- [x] Define and implement barge-in and idle timeout policy.
- [x] Add an RTOS-oriented mock/reference client for protocol bring-up.
- [x] Add optional speech-oriented `opus` uplink support normalized to `pcm16le/16000/mono` before ASR.
- [x] Switch the realtime MiMo path to provider-streamed `pcm16`.
- [x] Add optional cloud voice providers behind the same voice runtime boundary for streaming ASR/TTS selection.
- [x] Add a `xiaozhi` compatibility adapter with OTA discovery, `/xiaozhi/v1/` WebSocket ingress, and protocol-version `1/2/3` binary framing support.

Bootstrap implementation for the WebSocket handler, session lifecycle, and first voice runtime path has been added in code. `go test ./...` now passes locally after the Go toolchain was installed and dependencies were resolved via `GOPROXY=https://goproxy.cn,direct`.

Live smoke validation has also succeeded against a local `agentd` process for:

- `GET /healthz`
- `GET /v1/realtime`
- `session.start -> audio uplink -> audio.in.commit -> streamed response -> session.end`
- scripted `full` validation via `agent-server-desktop-runner`, covering text turn, audio turn, and server-initiated close
- scripted `smoke-funasr.ps1` validation on clean ports with:
  - local `FunASR` ASR worker on CPU
  - `mimo-v2-tts` speech synthesis
  - real PCM16 audio chunks returned over `/v1/realtime/ws`
- runtime provider selection now also supports optional cloud voice backends without changing the device-facing websocket contract:
  - `iflytek_rtasr` for ASR
  - `iflytek_tts_ws` for websocket TTS
  - `volcengine_tts` for SSE TTS
- websocket runtime behaviour now includes:
  - paced `20 ms` audio downlink frames
  - barge-in by inbound audio or `session.update interrupt=true`
  - `idle_timeout` only while the session is `active`
  - `max_duration` enforced across the session lifetime
- live `rtos mock` validation on clean ports succeeded with:
  - one primary audio turn
  - one barge-in turn triggered during `speaking`
  - final client-initiated close after the second response completed

### M1.5 Agent Runtime Core

- [x] Define a transport-neutral `internal/agent` package with a shared `TurnExecutor` contract.
- [x] Add a bootstrap executor so text and ASR-completed turns flow through one agent-runtime boundary.
- [x] Move session-end policy for bootstrap turns out of the WebSocket gateway and into the runtime result.
- [x] Add streamed text/tool delta contracts for richer agent outputs.
- [x] Add memory and tool orchestration hooks behind the runtime boundary.
- [x] Add a true streaming executor interface so deltas can emit incrementally without pre-buffering.
- [x] Replace the no-op memory and tool implementations with real providers behind the same runtime contracts.
- [x] Add the first optional cloud LLM-backed executor behind the same runtime boundary.
- [x] Add a built-in household control-screen assistant prompt template and assistant-name runtime config for the LLM path.
- [ ] Route future channel adapters through this runtime boundary instead of responder-local logic.

### M2 Channel Skill Framework

- [ ] Finalize channel contract and registry around the `Agent Runtime Core` handoff.
- [ ] Add the first Feishu channel adapter skeleton.
- [ ] Connect channel messages into the shared runtime turn contract.

### M3 Security Backfill

- [ ] Device registration and token flow.
- [ ] User auth and operator auth.
- [ ] Multi-tenant policy boundaries.
- [ ] Audit and rate-limiting.

## Active Optimization Track

- `P0 Foundation Hardening`
  - fix turn-audio buffering and snapshot-copy overhead
  - align published `turn_mode` semantics with actual runtime behaviour
  - split assistant persona from execution mode
  - add baseline observability and quality metrics
- `P1 Runtime Intelligence`
  - add streaming chat plus real tool loop support behind `internal/agent`
  - add recent-message context plus layered memory
  - enrich ASR output into structured speech understanding metadata
  - introduce household context and deterministic control routing
- `P2 Companion Experience`
  - add context-aware turn detection and follow-up listening
  - add stronger TTS style control
  - add screen context, image path, and bounded proactive behaviour

The detailed task breakdown for this track now lives in:

- `docs/architecture/project-optimization-roadmap-zh-2026-04.md`
- `docs/architecture/full-duplex-voice-assessment-zh-2026-04-10.md`
- `docs/architecture/local-open-source-full-duplex-roadmap-zh-2026-04-10.md`

Current planning note:

- for the next full-duplex voice stage, the primary execution route is now explicitly `local / open-source first`
- hosted realtime speech providers remain comparison baselines, not the main implementation target
- the next `L2` hardening slice should strengthen local endpoint evidence inside the Python FunASR worker rather than widening the public realtime contract
- that slice should add an optional stronger acoustic endpoint hint path, preferably `Silero VAD`, while preserving the current tail-energy hint as the default and graceful fallback
- after the baseline Docker slice, the next deployment follow-up should add real compose validation on a Docker-installed machine, then separate GPU worker packaging and CI image smoke coverage without collapsing runtime boundaries

## Current Execution Log

### 2026-04-04 P0-1 Complete

- Scope:
  - decouple session snapshots from accumulated turn-audio buffers
  - stop cloning the full turn buffer on every inbound frame
  - export committed turn audio only at commit time for ASR/runtime handoff
  - add focused regression tests and at least one allocation-oriented benchmark around the session path
- Target files:
  - `internal/session/realtime_session.go`
  - `internal/session/realtime_session_test.go`
  - `internal/gateway/realtime_ws.go`
  - `internal/gateway/xiaozhi_ws.go`
- Acceptance for this execution step:
  - inbound audio frame ingestion no longer returns a snapshot containing a cloned accumulated turn buffer
  - commit path still hands the exact turn audio to the responder/runtime path
  - targeted tests pass before moving to `P0-2`

Validation recorded for `P0-1`:

- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/session`
- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/gateway`
- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/session -run ^$ -bench BenchmarkRealtimeSessionIngestAudioFrame -benchmem`

Observed benchmark snapshot:

- `BenchmarkRealtimeSessionIngestAudioFrame-16`: `638.7 ns/op`, `3724 B/op`, `0 allocs/op`

### 2026-04-04 P0-2 Complete

- Scope:
  - align published `turn_mode` with the real client-commit driven runtime behaviour
  - remove or narrow any publicly documented session states that are not exercised by the implementation
  - update discovery handlers, config defaults, protocol docs, schemas, and compatibility notes together
  - keep the session core transport-neutral while making the RTOS fast path easier to integrate correctly
- Target files:
  - `internal/app/config.go`
  - `internal/gateway/realtime.go`
  - `internal/gateway/realtime_test.go`
  - `internal/gateway/realtime_ws_test.go`
  - `internal/control/info.go`
  - `docs/protocols/realtime-session-v0.md`
  - `docs/protocols/rtos-device-ws-v0.md`
  - `schemas/realtime/session-envelope.schema.json`
- Acceptance for this execution step:
  - discovery and info endpoints no longer advertise a server-VAD mode that is not implemented
  - protocol docs clearly state that turn completion is client-side via `audio.in.commit`
  - public state semantics only describe states that the runtime actually uses today

Validation recorded for `P0-2`:

- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/app`
- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/session`
- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/gateway`

### 2026-04-04 Documentation And Durable Records Complete

- Scope:
  - record the advertised turn-mode decision in architecture docs and ADR
  - close or update the tracked issue entries for `P0-1` and `P0-2`
  - append change log, project memory, and implementation notes
  - inspect git status so the working tree remains understandable before any next step

Recorded follow-through:

- updated `docs/architecture/overview.md`
- added `docs/adr/0009-advertise-commit-driven-turn-semantics-until-server-vad-exists.md`
- updated `.codex/change-log.md`
- updated `.codex/issues-and-resolutions.md`
- updated `.codex/project-memory.md`

Git status note:

- the repository still contains a broad pre-existing dirty worktree from earlier milestones and bring-up work
- this execution specifically completed `P0-1` and `P0-2` plus the related docs, schema, ADR, and tracking updates

### 2026-04-05 P0-3 Complete

- Scope:
  - split assistant persona from execution mode inside the shared agent runtime
  - move debug-stage simulation rules out of the built-in persona template and into runtime-owned execution-mode policies
  - add config defaults for `persona` and `execution_mode` without breaking existing `{{assistant_name}}` prompt substitution
  - keep the transport and voice layers unaware of prompt-policy internals
- Target files:
  - `internal/agent/llm.go`
  - `internal/agent/llm_executor.go`
  - `internal/agent/llm_executor_test.go`
  - `internal/app/config.go`
  - `internal/app/app.go`
  - `internal/app/app_test.go`
  - `.env.example`
  - `docs/architecture/runtime-configuration.md`
  - `docs/architecture/overview.md`
  - `docs/architecture/agent-runtime-core.md`
- Acceptance for this execution step:
  - built-in persona no longer hardcodes simulation-only rules
  - runtime can switch among `simulation`, `dry_run`, and `live_control` without changing the persona template
  - live-control mode no longer inherits “pretend success” instructions from the debug persona path

Validation recorded for `P0-3`:

- `go test ./internal/agent`
- `go test ./internal/app`
- `go test ./...`

Recorded follow-through:

- updated `.env.example`
- updated `docs/architecture/runtime-configuration.md`
- updated `docs/architecture/overview.md`
- updated `docs/architecture/agent-runtime-core.md`
- updated `README.md`
- added `docs/adr/0010-separate-agent-persona-from-execution-mode.md`

### 2026-04-05 P0-4 Complete

- Scope:
  - upgrade the desktop scripted runner output into a comparable end-to-end quality report
  - add baseline latency and audio counters per scenario without changing the device-facing realtime protocol
  - include enough discovery metadata in the report to compare provider and mode combinations across runs
  - document the report shape so smoke scripts and future CI can archive it consistently
- Target files:
  - `clients/python-desktop-client/src/agent_server_desktop_client/protocol.py`
  - `clients/python-desktop-client/src/agent_server_desktop_client/runner.py`
  - `clients/python-desktop-client/tests/test_runner.py`
  - `clients/python-desktop-client/README.md`
  - `README.md`
- Acceptance for this execution step:
  - runner JSON output includes per-scenario latency metrics and aggregate quality summary
  - report payload includes enough discovery metadata to compare runs across providers or turn modes
  - unit tests cover the report summary logic

Validation recorded for `P0-4`:

- `PYTHONPATH=clients/python-desktop-client/src python3 -m unittest discover -s clients/python-desktop-client/tests -v`

Recorded follow-through:

- updated `clients/python-desktop-client/src/agent_server_desktop_client/protocol.py`
- updated `clients/python-desktop-client/src/agent_server_desktop_client/runner.py`
- updated `clients/python-desktop-client/tests/test_runner.py`
- updated `clients/python-desktop-client/README.md`
- updated `README.md`

### 2026-04-11 L2 Preview Config And Validation Slice Complete

- Scope:
  - make the hidden `L2` server-endpoint preview thresholds runtime-configurable instead of leaving them as implicit detector defaults
  - keep that tuning inside the shared `internal/voice` boundary so native realtime and `xiaozhi` continue to share one preview policy surface
  - add a dedicated desktop runner scenario for audio-without-commit validation, without widening the public discovery contract or polluting default regression suites
- Target files:
  - `internal/app/config.go`
  - `internal/app/app.go`
  - `internal/app/voice_runtime_test.go`
  - `internal/voice/asr_responder.go`
  - `internal/voice/asr_responder_test.go`
  - `clients/python-desktop-client/src/agent_server_desktop_client/runner.py`
  - `clients/python-desktop-client/tests/test_runner.py`
  - `clients/python-desktop-client/README.md`
  - `docs/architecture/runtime-configuration.md`
  - `docs/architecture/overview.md`
- Acceptance for this execution step:
  - `funasr_http` and `iflytek_rtasr` responder wiring both receive `server-endpoint` threshold config from `VoiceConfig`
  - preview-threshold behavior is covered by focused Go tests
  - the desktop runner can explicitly exercise audio upload without `audio.in.commit` through a dedicated scenario
  - the new runner scenario remains opt-in and undocumented as a public protocol capability

Validation recorded for this execution step:

- `go test ./internal/voice ./internal/app`
- `env PYTHONPATH=clients/python-desktop-client/src python3 -m unittest discover -s clients/python-desktop-client/tests -v`

### 2026-04-13 L2 Lexical False-Endpoint Guard Slice Complete

- Scope:
  - keep the hidden `L2` server-endpoint preview path conservative by default instead of relying only on a fixed silence window
  - add a first lexical false-endpoint guard inside shared `internal/voice` so obviously unfinished partials do not auto-commit on every short pause
  - preserve the existing hidden-preview rollout shape: no public discovery change and no provider-specific turn rules in adapters
- Target files:
  - `internal/voice/turn_detector.go`
  - `internal/voice/turn_detector_test.go`
  - `internal/voice/asr_responder.go`
  - `internal/voice/asr_responder_test.go`
  - `internal/app/config.go`
  - `internal/app/app.go`
  - `internal/app/app_test.go`
  - `internal/app/voice_runtime_test.go`
  - `docs/architecture/runtime-configuration.md`
  - `docs/architecture/overview.md`
  - `docs/adr/0021-local-open-source-first-full-duplex-roadmap-prioritizes-voice-orchestration.md`
- Acceptance for this execution step:
  - hidden preview mode can distinguish between lexically complete and obviously unfinished partials
  - incomplete partials wait an additional hold window before auto-commit, instead of using the base silence timeout immediately
  - the guard remains configurable from shared voice runtime config and applies uniformly to both `funasr_http` and `iflytek_rtasr`
  - public discovery and public protocol docs remain unchanged

Validation recorded for this execution step:

- `go test ./internal/voice ./internal/app`

### 2026-04-13 L2 Provider Endpoint Hint Slice Complete

- Scope:
  - make hidden server-endpoint preview consume real provider endpoint hints instead of relying only on silence windows plus local lexical heuristics
  - keep provider-specific logic behind shared `StreamingTranscriber` deltas and worker responses, with adapters still unaware of provider details
  - remain conservative by only shortening the silence window when the latest partial already looks complete
- Target files:
  - `workers/python/src/agent_server_workers/funasr_service.py`
  - `workers/python/tests/test_funasr_service.py`
  - `workers/python/README.md`
  - `internal/voice/http_transcriber.go`
  - `internal/voice/http_transcriber_test.go`
  - `internal/voice/turn_detector.go`
  - `internal/voice/turn_detector_test.go`
  - `internal/voice/asr_responder.go`
  - `internal/app/config.go`
  - `internal/app/app.go`
  - `internal/app/app_test.go`
  - `internal/app/voice_runtime_test.go`
  - `docs/architecture/runtime-configuration.md`
  - `docs/architecture/overview.md`
  - `docs/adr/0021-local-open-source-first-full-duplex-roadmap-prioritizes-voice-orchestration.md`
- Acceptance for this execution step:
  - local worker preview responses can expose a lightweight endpoint hint
  - `HTTPTranscriber` preserves that hint on partial deltas
  - shared turn detection can use the hint to shorten the endpoint wait for lexically complete partials, without changing public discovery or wire contracts
  - incomplete lexical partials still stay on the conservative hold path

Validation recorded for this execution step:

- `go test ./internal/voice ./internal/app`
- `env PYTHONPATH=workers/python/src python3 -m unittest discover -s workers/python/tests -v`

### 2026-04-13 L2 Optional Stronger Local Acoustic Endpoint Slice Complete

- Scope:
  - strengthen the hidden preview endpoint-hint path with a better local/open-source acoustic signal without widening public discovery or websocket contracts
  - keep the new logic inside the Python FunASR worker so adapters and the shared Go runtime still consume only normalized preview endpoint hints
  - preserve the existing tail-energy hint as the default and graceful fallback path
- Target files:
  - `workers/python/src/agent_server_workers/funasr_service.py`
  - `workers/python/tests/test_funasr_service.py`
  - `workers/python/pyproject.toml`
  - `workers/python/README.md`
  - `internal/voice/turn_detector_test.go`
  - `docs/architecture/runtime-configuration.md`
  - `docs/architecture/overview.md`
  - `plan.md`
- Acceptance for this execution step:
  - the worker can optionally use `Silero VAD` for internal preview endpoint hints
  - when `Silero VAD` is unavailable or unsupported, the worker falls back to the current `preview_tail_silence` path instead of changing behavior abruptly
  - the shared turn detector continues to treat alternate provider hints generically, without new adapter-specific code
  - public discovery and protocol docs remain unchanged

Validation recorded for this execution step:

- `env PYTHONPATH=workers/python/src python3 -m unittest discover -s workers/python/tests -v`
- `go test ./internal/voice ./internal/app ./internal/gateway`

### 2026-04-13 Linux Dependency Install Consolidation Complete

- Scope:
  - audit the repository's real dependency layers across Go service, desktop client, and FunASR worker
  - replace machine-history-only setup knowledge with a usable Linux install entrypoint
  - actually install `onnxruntime` and `silero-vad` into the worker conda env and verify the real preview-hint path
- Target files:
  - `scripts/install-linux-stack.sh`
  - `README.md`
  - `workers/python/pyproject.toml`
  - `workers/python/README.md`
  - `docs/architecture/local-funasr-asr.md`
  - `plan.md`
- Acceptance for this execution step:
  - Linux bring-up has one repository-local install entrypoint for Go deps, desktop client, and worker env preparation
  - worker packaging declares runtime extras (`funasr`, `modelscope`) and optional `stream-vad` extras (`onnxruntime`, `silero-vad`)
  - the install script handles the real local editable-install constraints discovered on this machine: `setuptools<82`, plus `hatchling` and `editables`
  - the `xiaozhi-esp32-server` env actually imports `onnxruntime` and `silero_vad`
  - a live worker smoke run returns `preview_silero_vad_silence` on the stream preview path

Validation recorded for this execution step:

- `./scripts/install-linux-stack.sh --with-stream-vad`
- `conda run -n xiaozhi-esp32-server python -c "import silero_vad, onnxruntime; print(silero_vad.__version__); print(onnxruntime.__version__)"`
- `conda run -n xiaozhi-esp32-server python -c "from agent_server_workers.funasr_service import FunASREngine, WorkerConfig; ... engine._ensure_silero_vad_runtime() ..."`
- live worker smoke on `127.0.0.1:8093` with `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_PROVIDER=silero`, using `artifacts/live-baseline/20260409/samples/qinyu_xiaoou_16k.wav` plus trailing silence:
  - preview text: `小欧管家。`
  - preview endpoint reason: `preview_silero_vad_silence`
  - final text: `小欧管家。`

### 2026-04-13 Dockerization P0 Slice Complete

- Scope:
  - turn the current ad hoc Docker assets into a first formal deployment slice
  - keep `agentd` and the local FunASR worker as separate containers so the shared runtime boundary stays intact
  - support two initial deployment shapes: `agentd` alone and `agentd + local CPU ASR worker`
- Target files:
  - `.dockerignore`
  - `deploy/docker/Dockerfile`
  - `deploy/docker/docker-compose.yml`
  - `deploy/docker/agentd.Dockerfile`
  - `deploy/docker/funasr-worker.cpu.Dockerfile`
  - `deploy/docker/compose.base.yml`
  - `deploy/docker/compose.local-asr.yml`
  - `deploy/docker/.env.docker.example`
  - `README.md`
  - `docs/architecture/runtime-configuration.md`
  - `plan.md`
- Acceptance for this execution step:
  - the `agentd` image uses a Go version aligned with `go.mod` and includes `go.sum`
  - there is a dedicated CPU worker image for the local FunASR HTTP service
  - compose files can express `agentd` alone or `agentd + funasr-worker` with service-name networking instead of `127.0.0.1`
  - Docker env examples and docs make the container-networking assumptions explicit
  - GPU-specific deployment remains a documented follow-up, not mixed into the first CPU slice

Validation recorded for this execution step:

- `python3 -c "import yaml,sys; [yaml.safe_load(open(f)) for f in ['deploy/docker/compose.base.yml','deploy/docker/compose.local-asr.yml','deploy/docker/docker-compose.yml']]; print('yaml-ok')"`
- `python3 -c "from pathlib import Path; import re; files=['deploy/docker/agentd.Dockerfile','deploy/docker/funasr-worker.cpu.Dockerfile','deploy/docker/Dockerfile']; ..."`
- `docker compose ...` validation could not run in this workspace because the `docker` CLI is not installed on this machine

Recorded follow-through:

- added `.dockerignore`
- added `deploy/docker/agentd.Dockerfile`
- added `deploy/docker/funasr-worker.cpu.Dockerfile`
- added `deploy/docker/compose.base.yml`
- added `deploy/docker/compose.local-asr.yml`
- added `deploy/docker/.env.docker.example`
- updated `deploy/docker/Dockerfile`
- updated `deploy/docker/docker-compose.yml`
- updated `docs/architecture/runtime-configuration.md`
- updated `README.md`
- updated `plan.md`
- updated `.codex/change-log.md`
- updated `.codex/issues-and-resolutions.md`
- updated `.codex/project-memory.md`
- updated `.claude/context.md`
- updated `.claude/logs/session-notes.md`

### 2026-04-13 Docker Validation Follow-up Complete With Network Caveat

- Scope:
  - install Docker Engine plus Compose on the current WSL2 machine and run real compose validation instead of static-only checks
  - fix any Docker asset issues surfaced by actual image resolution or build steps
  - keep the repository changes focused on reusable build robustness, not one-off machine hacks
- Target files:
  - `deploy/docker/Dockerfile`
  - `deploy/docker/agentd.Dockerfile`
  - `deploy/docker/funasr-worker.cpu.Dockerfile`
  - `deploy/docker/compose.base.yml`
  - `deploy/docker/compose.local-asr.yml`
  - `README.md`
  - `docs/architecture/runtime-configuration.md`
  - `plan.md`
- Acceptance for this execution step:
  - real `docker compose config` succeeds for both `agentd` only and `agentd + local-asr`
  - `agentd` image can be built on this machine without depending on `gcr.io/distroless`
  - Docker build paths honor the same constrained-network assumptions already proven on this machine
  - any remaining worker-image blockers are identified as repository issues or external-network caveats with clear follow-up

Validation recorded for this execution step:

- installed `docker.io 29.1.3` and `docker-compose-v2 2.40.3` on the current WSL2 Ubuntu machine
- `docker version`
- `docker compose -f deploy/docker/compose.base.yml config`
- `docker compose -f deploy/docker/compose.base.yml -f deploy/docker/compose.local-asr.yml config`
- `docker pull golang:1.24.4`
- `docker compose -f deploy/docker/compose.base.yml build agentd`
- `docker pull python:3.11-slim-bookworm`
- `docker compose -f deploy/docker/compose.base.yml -f deploy/docker/compose.local-asr.yml build funasr-worker`

Observed outcome:

- `agentd` image now builds successfully on this machine
- CPU `funasr-worker` image now gets past base-image resolution and bootstrap pip setup, but the current network still times out on large `torch` CPU wheel downloads from `download-r2.pytorch.org`
- the remaining worker-image failure is therefore an external network caveat on this machine, not a protocol or layering issue in the repository design

Recorded follow-through:

- switched `agentd` runtime images from `gcr.io/distroless` to `scratch` with copied CA bundle and non-root user
- added Docker build defaults for `GOPROXY=https://goproxy.cn,direct` and `GOSUMDB=sum.golang.google.cn`
- removed the unused worker-side apt system-package layer from the CPU FunASR image
- added proxy build-arg passthrough in compose and worker Dockerfile so constrained-network hosts can reuse standard `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY`
- updated Docker deployment documentation with build-network notes and remaining worker-network caveat

### 2026-04-05 P1-1 Complete

- Scope:
  - add a streaming-capable chat-model contract behind `internal/agent`
  - upgrade the LLM executor from single-shot completion to an iterative model/tool loop
  - keep bootstrap commands such as `/memory` and `/tool ...` on their existing direct path
  - preserve current transport-facing delta semantics while allowing provider-specific streaming internally
- Target files:
  - `internal/agent/contracts.go`
  - `internal/agent/llm.go`
  - `internal/agent/llm_executor.go`
  - `internal/agent/deepseek_chat.go`
  - `internal/agent/llm_executor_test.go`
  - `docs/architecture/agent-runtime-core.md`
  - `docs/architecture/overview.md`
  - `docs/adr/*`
- Acceptance for this execution step:
  - `LLMTurnExecutor` can stream text deltas from the model path without pre-buffering the full reply
  - `LLMTurnExecutor` can execute registered tools proposed by the model and reinject their results into follow-up model steps
  - the runtime enforces a bounded tool/model step loop so it cannot recurse indefinitely
  - existing bootstrap debug commands still bypass the model/tool loop
  - focused `internal/agent` tests pass before moving on to `P1-2`

Validation recorded for `P1-1`:

- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/agent`
- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/app`
- `env GOCACHE=/tmp/agent-server-go-build go test ./...`

Recorded follow-through:

- updated `internal/agent/contracts.go`
- updated `internal/agent/llm.go`
- updated `internal/agent/llm_executor.go`
- updated `internal/agent/deepseek_chat.go`
- updated `internal/agent/runtime_backends.go`
- updated `internal/agent/llm_executor_test.go`
- added `internal/agent/deepseek_chat_test.go`
- updated `docs/architecture/agent-runtime-core.md`
- updated `docs/architecture/overview.md`
- added `docs/adr/0011-runtime-tool-loop-stays-inside-agent-runtime.md`

### 2026-04-05 P1-2 Complete

- Scope:
  - add recent-message context to the shared chat-model request path
  - keep the current memory summary as a separate long-lived recall layer instead of flattening everything into one string
  - extend the in-memory runtime backend so it can return both a bounded recent window and summary/fact recall under one runtime-owned contract
  - prepare the memory boundary for future `session / device / user / room / household` scope expansion without pushing those concepts into transports
- Target files:
  - `internal/agent/contracts.go`
  - `internal/agent/runtime_backends.go`
  - `internal/agent/llm_executor.go`
  - `internal/agent/llm_executor_test.go`
  - `docs/architecture/agent-runtime-core.md`
  - `docs/architecture/overview.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - `ChatModelRequest` can carry a recent message window independently from summary memory
  - the default in-memory backend returns bounded recent turns as explicit message history plus summary/facts
  - the LLM executor injects recent context without teaching transports how memory is stored
  - focused `internal/agent` tests pass before moving on to the next P1 slice

Validation recorded for `P1-2`:

- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/agent`
- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/app`
- `env GOCACHE=/tmp/agent-server-go-build go test ./...`

Recorded follow-through:

- updated `internal/agent/contracts.go`
- added `internal/agent/memory_context.go`
- updated `internal/agent/bootstrap_executor.go`
- updated `internal/agent/runtime_backends.go`
- updated `internal/agent/llm_executor.go`
- updated `internal/agent/bootstrap_executor_test.go`
- updated `internal/agent/llm_executor_test.go`
- updated `internal/agent/runtime_backends_test.go`
- updated `docs/architecture/agent-runtime-core.md`
- updated `docs/architecture/overview.md`
- added `docs/adr/0012-layer-recent-messages-over-summary-memory.md`

### 2026-04-05 P1-3 Complete

- Scope:
  - extend the shared ASR result contract from plain transcript text into structured speech-understanding metadata
  - inject normalized speech metadata into `internal/agent` turn input without exposing provider-specific payloads to transports
  - keep provider capability gaps compatible so missing fields do not break the shared runtime path
  - preserve the current device-facing realtime and `xiaozhi` websocket protocols while enriching runtime context internally
- Target files:
  - `internal/voice/contracts.go`
  - `internal/voice/asr_responder.go`
  - `internal/voice/http_transcriber.go`
  - `internal/voice/iflytek_rtasr.go`
  - `internal/voice/turn_executor.go`
  - `internal/voice/*_test.go`
  - `docs/architecture/agent-runtime-core.md`
  - `docs/architecture/overview.md`
  - `docs/adr/*`
- Acceptance for this execution step:
  - `TranscriptionResult` can carry structured metadata such as language, endpoint reason, speaker, emotion, and partials without forcing every provider to populate all fields
  - `ASRResponder` injects normalized speech metadata into agent-turn metadata for both text and audio turns where available
  - the shared runtime path remains provider-agnostic and transport-agnostic
  - focused voice and app tests pass before moving on to `P1-4`

Validation recorded for `P1-3`:

- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/voice`
- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/app`
- `env GOCACHE=/tmp/agent-server-go-build go test ./...`

Recorded follow-through:

- updated `internal/voice/contracts.go`
- added `internal/voice/speech_metadata.go`
- updated `internal/voice/turn_executor.go`
- updated `internal/voice/asr_responder.go`
- updated `internal/voice/http_transcriber.go`
- updated `internal/voice/iflytek_rtasr.go`
- updated `internal/voice/asr_responder_test.go`
- updated `internal/voice/http_transcriber_test.go`
- updated `internal/voice/iflytek_rtasr_test.go`
- updated `docs/architecture/agent-runtime-core.md`
- updated `docs/architecture/overview.md`
- added `docs/adr/0013-normalize-speech-metadata-before-agent-runtime.md`

### 2026-04-05 P1-4 Complete

- Scope:
  - add a first household-context layer inside `internal/agent`
  - route obvious household control requests through a deterministic runtime path before the generative chat path
  - keep sensitive-device handling conservative and transport-neutral
  - preserve the current user-facing natural-language reply style without introducing device-facing control payloads
- Target files:
  - `internal/agent/contracts.go`
  - `internal/agent/runtime_backends.go`
  - `internal/agent/llm_executor.go`
  - `internal/agent/bootstrap_executor.go`
  - `internal/agent/*_test.go`
  - `docs/architecture/agent-runtime-core.md`
  - `docs/architecture/overview.md`
  - `docs/adr/*`
- Acceptance for this execution step:
  - the runtime can recognize at least a first bounded slice of household control requests deterministically
  - deterministic routing stays inside `internal/agent` and does not leak into transports or voice providers
  - user-facing replies remain natural language only and compatible with the current simulation-oriented runtime
  - focused tests pass before moving on to the next P1 slice

Validation recorded for `P1-4`:

- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/agent`
- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/app`
- `env GOCACHE=/tmp/agent-server-go-build go test ./...`

Recorded follow-through:

- added `internal/agent/household_control.go`
- updated `internal/agent/bootstrap_executor.go`
- updated `internal/agent/llm_executor.go`
- updated `internal/agent/bootstrap_executor_test.go`
- updated `internal/agent/llm_executor_test.go`
- updated `docs/architecture/agent-runtime-core.md`
- updated `docs/architecture/overview.md`
- added `docs/adr/0014-first-household-routing-stays-bounded-and-runtime-owned.md`

### 2026-04-05 P1-5 Complete

- Scope:
  - fill the most important missing `xiaozhi` compatibility capabilities needed for stronger runtime integration
  - prioritize audio-turn `stt` echo first so RTOS device bring-up can observe recognized user speech on the compat path
  - keep the `xiaozhi` adapter as an ingress/egress shim over the shared runtime instead of turning it into a second orchestration layer
  - avoid broad protocol churn while tightening the existing compatibility surface
- Target files:
  - `internal/gateway/xiaozhi_ws.go`
  - `internal/gateway/xiaozhi_protocol.go`
  - `internal/gateway/xiaozhi_ws_test.go`
  - `docs/protocols/xiaozhi-compat-ws-v0.md`
  - `docs/architecture/overview.md`
  - `docs/adr/*`
- Acceptance for this execution step:
  - audio turns on the `xiaozhi` compatibility path emit a useful `stt` echo after ASR completes
  - the compat adapter still delegates turn execution to the shared runtime instead of owning transcript policy itself
  - focused gateway tests pass before moving to the next compatibility item

Validation recorded for `P1-5`:

- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/voice`
- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/gateway`
- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/app`
- `env GOCACHE=/tmp/agent-server-go-build go test ./...`

Recorded follow-through:

- updated `internal/voice/contracts.go`
- updated `internal/voice/bootstrap_responder.go`
- updated `internal/voice/asr_responder.go`
- updated `internal/voice/asr_responder_test.go`
- updated `internal/gateway/xiaozhi_ws.go`
- updated `internal/gateway/xiaozhi_ws_test.go`
- updated `docs/protocols/xiaozhi-compat-ws-v0.md`
- updated `docs/architecture/overview.md`
- added `docs/adr/0015-xiaozhi-stt-echo-comes-from-shared-responder-output.md`

### 2026-04-05 Web/H5 Direct Realtime Adaptation Complete

- Scope:
  - add a first browser/H5 reference client that connects directly to the native `/v1/realtime/ws` path
  - keep the existing `rtos-ws-v0` protocol as the single direct realtime contract instead of creating a browser-specific variant
  - adapt browser microphone capture and speaker playback to the current `pcm16le / 16000 / mono` websocket audio path
  - provide a same-service debug page for quick bring-up without requiring a separate static-file host
  - document browser constraints explicitly so future Web/H5 work can stay transport-neutral
- Target files:
  - `internal/app/app.go`
  - `internal/app/app_test.go`
  - `internal/control/*`
  - `README.md`
  - `docs/protocols/*`
  - `docs/architecture/overview.md`
  - `docs/adr/*`
- Acceptance for this execution step:
  - a browser page can discover `/v1/realtime`, open `/v1/realtime/ws`, and complete at least one text turn and one microphone audio turn
  - the implementation does not introduce a second browser-only agent or voice protocol
  - browser-side limitations are documented, especially the current requirement that the native realtime path use `pcm16le` for Web/H5 audio capture and playback
  - focused tests pass before resuming the remaining `xiaozhi` compatibility backlog

Validation recorded for this execution step:

- `env GOCACHE=/tmp/agent-server-go-build go test ./internal/app ./internal/control`
- `node --check internal/control/webh5_assets/app.js`
- `env GOCACHE=/tmp/agent-server-go-build go test ./...`

Recorded follow-through:

- added `internal/control/webh5.go`
- added `internal/control/webh5_assets/index.html`
- added `internal/control/webh5_assets/styles.css`
- added `internal/control/webh5_assets/app.js`
- updated `internal/app/app.go`
- updated `internal/app/app_test.go`
- added `docs/protocols/web-h5-realtime-adaptation.md`
- updated `docs/protocols/realtime-session-v0.md`
- updated `docs/protocols/rtos-device-ws-v0.md`
- updated `docs/architecture/overview.md`
- added `docs/adr/0016-web-h5-direct-clients-reuse-native-realtime-contract.md`
- updated `README.md`

### 2026-04-05 Standalone Tools Web Client Complete

- Scope:
  - add a standalone repository tool under `tools/web-client`
  - keep it on the native `/v1/realtime/ws` contract instead of introducing a tool-only websocket dialect
  - make it usable from a separate static origin by supporting manual profile entry and pasted discovery JSON instead of assuming same-origin `GET /v1/realtime`
  - keep enough controls for real usage plus test/debug work, including text turns, microphone turns, interrupts, raw event logging, and ad hoc JSON control sends
- Target files:
  - `tools/web-client/*`
  - `README.md`
  - `docs/protocols/web-h5-realtime-adaptation.md`
  - `plan.md`
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - a user can serve `tools/web-client` as static files and connect it to the current server over `/v1/realtime/ws`
  - the tool can complete text turns and microphone turns against the native realtime contract
  - the tool remains usable when same-origin discovery is unavailable, by manual profile configuration or pasted discovery JSON
  - usage and limitations are documented clearly enough for test/debug bring-up

Validation recorded for this execution step:

- `node --check tools/web-client/app.js`
- `python3 -m py_compile tools/web-client/serve.py`

Recorded follow-through:

- added `tools/web-client/index.html`
- added `tools/web-client/styles.css`
- added `tools/web-client/app.js`
- added `tools/web-client/README.md`
- added `tools/web-client/serve.py`
- updated `docs/protocols/web-h5-realtime-adaptation.md`
- updated `README.md`
- updated `.codex/change-log.md`
- updated `.codex/issues-and-resolutions.md`
- updated `.codex/project-memory.md`

### 2026-04-07 Local Loopback Integration Validation Complete

- Scope:
  - start the local FunASR HTTP worker, `agentd`, and the standalone `tools/web-client` static server on this machine
  - verify the local zero-external-dependency validation stack using `funasr_http + tts=none + bootstrap`
  - confirm the standalone web-client page is reachable and the native realtime websocket contract still passes end-to-end text and audio validation
  - capture any runtime issues observed during this local loopback run
- Runtime configuration used for this execution step:
  - worker: `conda run -n xiaozhi-esp32-server ... python -m agent_server_workers.funasr_service --host 127.0.0.1 --port 8091 --device cpu`
  - server:
    - `AGENT_SERVER_ADDR=:8080`
    - `AGENT_SERVER_VOICE_PROVIDER=funasr_http`
    - `AGENT_SERVER_VOICE_ASR_URL=http://127.0.0.1:8091/v1/asr/transcribe`
    - `AGENT_SERVER_VOICE_EMIT_PLACEHOLDER_AUDIO=false`
    - `AGENT_SERVER_TTS_PROVIDER=none`
    - `AGENT_SERVER_AGENT_LLM_PROVIDER=bootstrap`
  - standalone web tool: `python3 tools/web-client/serve.py --port 18081`
- Acceptance for this execution step:
  - local worker, server, and static tool all become reachable
  - the realtime discovery contract advertises the expected native websocket path and local runtime providers
  - the end-to-end runner passes text, audio, and server-end scenarios against the live local stack
  - noteworthy runtime issues are recorded if found

Validation recorded for this execution step:

- local health and reachability:
  - `http://127.0.0.1:8091/v1/asr/info`
  - `http://127.0.0.1:8080/healthz`
  - `http://127.0.0.1:8080/v1/realtime`
  - `http://127.0.0.1:18081/`
- end-to-end runner:
  - `env PYTHONPATH=clients/python-desktop-client/src python3 -m agent_server_desktop_client.runner --scenario full --http-base http://127.0.0.1:8080 --timeout-sec 90 --output /tmp/agent-server-local-runner.json`

Observed result summary:

- worker state became `model_loaded=true`
- realtime discovery advertised:
  - `voice_provider=funasr_http`
  - `tts_provider=none`
  - `ws_path=/v1/realtime/ws`
  - `turn_mode=client_wakeup_client_commit`
- standalone tool index was reachable at `http://127.0.0.1:18081/`
- the full runner report returned `ok=true` for:
  - `text`
  - `audio`
  - `server-end`
- archived runner artifact:
  - `.codex/artifacts/local-loopback-full-2026-04-07.json`

Observed issue recorded for follow-up:

- the local silence-based audio scenario was transcribed as `그.` instead of an empty utterance, which indicates current silence rejection on the local FunASR reference path still needs tuning

### 2026-04-07 Websocket Timeout Panic Fix Complete

- Scope:
  - fix the live panic `repeated read on failed websocket connection` observed in the native realtime websocket handler during browser-side usage
  - apply the same timeout-read handling correction to the `xiaozhi` compatibility websocket handler so both transport adapters stay consistent
  - keep the session core and runtime boundaries unchanged; only transport timeout and close behavior should change
  - add regression coverage around timeout-triggered close handling
- Target files:
  - `internal/gateway/realtime_runtime.go`
  - `internal/gateway/realtime_ws.go`
  - `internal/gateway/realtime_ws_test.go`
  - `internal/gateway/xiaozhi_ws.go`
  - `internal/gateway/xiaozhi_ws_test.go`
  - `plan.md`
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - transport timeout handling never loops back into `ReadMessage()` on a websocket connection that has already failed
  - native realtime websocket usage no longer emits the observed panic during timeout-driven connection teardown
  - the `xiaozhi` compatibility websocket path follows the same corrected timeout-close semantics
  - focused gateway tests pass after the fix

Validation recorded for this execution step:

- focused regression verification:
  - `env GOCACHE=/tmp/agent-server-go-build go test ./internal/gateway`
  - `env GOCACHE=/tmp/agent-server-go-build go test ./internal/app ./internal/gateway`
- live local service sync:
  - restarted `agentd` on `127.0.0.1:8080` with `funasr_http + tts=none + bootstrap` after the fix

Observed implementation result:

- both websocket handlers now treat timeout-triggered `ReadMessage()` failures as terminal for the connection
- the native realtime path still emits `session.end` for deadline-based close reasons before the handler returns
- the `xiaozhi` compatibility path still emits compat `tts stop` before the handler returns and the websocket closes
- new regression tests assert timeout-driven close behavior does not produce `panic serving` or `repeated read on failed websocket connection` in the test server logs

### 2026-04-07 Web Frontend Refresh And TTS Debug Complete

- Scope:
  - install the requested `ui-ux-pro-max` frontend design skill for local use
  - redesign the standalone `tools/web-client` into a clearer voice-debug console instead of a plain form stack
  - bring the built-in `/debug/realtime-h5/` page onto the same visual language so browser bring-up does not feel like a second-class debug path
  - split browser bring-up into separate `settings` and `debug` pages so configuration no longer crowds the live debug surface
  - make Web debug TTS first-class by surfacing playback state, audio diagnostics, and reusable last-turn audio artifacts without changing the native realtime websocket contract
  - fix the current `mimo_v2_tts` empty-stream issue so normal Web/H5 turns actually receive audio again
- Target files:
  - `tools/web-client/index.html`
  - `tools/web-client/settings.html`
  - `tools/web-client/styles.css`
  - `tools/web-client/app.js`
  - `tools/web-client/settings.js`
  - `tools/web-client/README.md`
  - `internal/control/webh5_assets/index.html`
  - `internal/control/webh5_assets/settings.html`
  - `internal/control/webh5_assets/styles.css`
  - `internal/control/webh5_assets/app.js`
  - `internal/control/webh5_assets/settings.js`
  - `docs/protocols/web-h5-realtime-adaptation.md`
  - `README.md`
  - `plan.md`
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - the main standalone browser tool presents a stronger information hierarchy, clearer session/audio state, and mobile-safe controls
  - Web debug visibly exposes whether TTS is configured, whether audio was expected, whether audio chunks arrived, and whether playback is active
  - at least one browser path can export or replay the last received TTS audio without changing the websocket contract
  - built-in and standalone browser pages remain aligned with the same native `/v1/realtime/ws` session semantics
  - frontend asset checks and affected Go tests pass after the change

Validation recorded for this execution step:

- frontend asset verification:
  - `node --check tools/web-client/app.js`
  - `node --check tools/web-client/settings.js`
  - `node --check internal/control/webh5_assets/app.js`
  - `node --check internal/control/webh5_assets/settings.js`
- focused Go verification:
  - `env GOCACHE=/tmp/agent-server-go-build go test ./internal/voice -run 'TestSynthesizedAudio'`
  - `env GOCACHE=/tmp/agent-server-go-build go test ./internal/app -run TestWebH5DebugRouteServesIndex`
  - `env GOCACHE=/tmp/agent-server-go-build go test ./internal/voice ./internal/app ./internal/control`
- live local validation after restarting `agentd` with `funasr_http + mimo_v2_tts`:
  - `env PYTHONPATH=clients/python-desktop-client/src python3 -m agent_server_desktop_client.runner --scenario full --http-base http://127.0.0.1:8080 --timeout-sec 120 --output /tmp/agent-server-web-tts-runner.json`

Observed implementation result:

- both browser surfaces now use a two-page flow:
  - settings page for endpoint, discovery, and device preset
  - debug page for websocket connect, turns, TTS playback, and logs
- the built-in same-origin page and the standalone tool now share the same visual language and TTS diagnostics model
- the MiMo TTS runtime now prefetches the first non-empty streaming chunk and falls back to buffered synthesis when the provider closes a stream without audio
- the latest live runner report at `/tmp/agent-server-web-tts-runner.json` now shows:
  - `response_with_audio_ratio = 1.0`
  - audio present for `text`, `audio`, and `server-end` scenarios

### 2026-04-07 Py-Xiaozhi-Inspired Frontend Interaction Refinement

- Scope:
  - reshape the debug pages around a clearer voice-session flow inspired by `py-xiaozhi`
  - emphasize one primary interaction line: connect -> start session -> listen -> speak
  - reduce the feeling of a control dump by promoting stage state, current session, and latest event into a top-level workbench card
  - keep settings/debug split, but make the debug page feel like a voice console rather than a test form
- Target files:
  - `tools/web-client/index.html`
  - `tools/web-client/styles.css`
  - `tools/web-client/app.js`
  - `internal/control/webh5_assets/index.html`
  - `internal/control/webh5_assets/styles.css`
  - `internal/control/webh5_assets/app.js`
  - `plan.md`
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - both browser debug pages present a primary state stage with obvious next action
  - phase changes such as idle, connect, listen, and speak are visible without reading the raw event log
  - transcript, TTS diagnostics, and protocol log remain available but no longer dominate first glance
  - the built-in and standalone pages remain protocol-compatible with the existing realtime websocket path

Validation recorded for this execution step:

- frontend asset verification:
  - `node --check tools/web-client/app.js`
  - `node --check tools/web-client/settings.js`
  - `node --check internal/control/webh5_assets/app.js`
  - `node --check internal/control/webh5_assets/settings.js`
- structure and initialization verification:
  - DOM id parity check between each debug page and its script refs
  - lightweight VM bootstrap check for both debug pages
- live local validation after restarting `agentd` with `funasr_http + mimo_v2_tts`:
  - `GET /debug/realtime-h5/`
  - `GET /debug/realtime-h5/app.js`
  - `GET /v1/realtime`

Observed implementation result:

- both debug pages now center the session state machine instead of starting with dense form controls
- a new stage card now exposes:
  - current phase badge
  - flow rail for idle / connect / listen / speak
  - current session snapshot
  - latest event summary
- transcript, mic turn, raw envelope debug, TTS playback, and protocol log remain available in the same page but are visually secondary to the live voice workflow
- the built-in page still uses same-origin discovery, and the standalone page still uses the manually saved profile model

## Immediate Next Step

Continue from the now-restored `F0 Baseline And Traceability` stack by deciding whether the next archived comparison path should prioritize:

- `xiaozhi` compatibility replay artifacts against the same local reference stack
- or the first non-bootstrap comparison run (`deepseek_chat` and/or real TTS) against the same native runner and RTOS mock baselines

### 2026-04-07 LLM Default Selection And TTS Source Correction

- Scope:
  - fix the current runtime pitfall where TTS can legitimately speak bootstrap echo text such as `agent-server received text input: ...`
  - make the service prefer the configured DeepSeek runtime automatically when a DeepSeek API key is present and no explicit `AGENT_SERVER_AGENT_LLM_PROVIDER` override was given
  - expose the effective `llm_provider` through realtime discovery so browser and RTOS bring-up can detect bootstrap fallback immediately
  - surface a browser-side compatibility warning when discovery still reports `llm_provider=bootstrap`
- Target files:
  - `internal/app/config.go`
  - `internal/app/app.go`
  - `internal/control/info.go`
  - `internal/gateway/realtime.go`
  - `internal/app/app_test.go`
  - `internal/gateway/realtime_test.go`
  - `tools/web-client/settings.js`
  - `internal/control/webh5_assets/settings.js`
  - `.env.example`
  - `README.md`
  - `docs/architecture/runtime-configuration.md`
  - `docs/protocols/web-h5-realtime-adaptation.md`
  - `plan.md`
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - an unset `AGENT_SERVER_AGENT_LLM_PROVIDER` no longer forces bootstrap when a DeepSeek key is present
  - realtime discovery advertises the effective `llm_provider`
  - browser settings pages warn when the current server is still on bootstrap and TTS will likely speak placeholder echo text
  - focused tests cover the new config default and discovery field

### 2026-04-08 Runtime Skill Boundary For Household Control

- Scope:
  - remove hardcoded household-control short-circuit logic from the core executor path
  - keep TTS in the shared voice runtime output layer instead of letting any single channel own spoken rendering policy
  - add a first runtime-skill injection path so domain behavior can enter as prompt fragments plus tools
  - move the current household-control semantics behind a builtin runtime skill instead of executor-owned rule branches
- Target files:
  - `internal/agent/contracts.go`
  - `internal/agent/llm_executor.go`
  - `internal/agent/bootstrap_executor.go`
  - `internal/agent/household_control.go`
  - `internal/agent/runtime_backends.go`
  - `internal/app/config.go`
  - `internal/app/app.go`
  - `internal/agent/*_test.go`
  - `internal/app/app_test.go`
  - `docs/architecture/overview.md`
  - `docs/architecture/runtime-configuration.md`
  - `docs/adr/0017-domain-behavior-enters-through-runtime-skills.md`
  - `docs/adr/0014-first-household-routing-stays-bounded-and-runtime-owned.md`
  - `.env.example`
  - `README.md`
  - `plan.md`
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - household requests no longer bypass the shared model loop through a deterministic executor branch
  - builtin domain behavior can be injected through a runtime skill that contributes prompt fragments and tools
  - the first builtin skill is configurable through `AGENT_SERVER_AGENT_SKILLS`
  - architecture docs explicitly state that TTS belongs to the shared voice runtime layer across RTOS, Web/H5, desktop, and future channels
  - regression coverage proves the new runtime-skill tool loop and the removal of bootstrap household hard-routing

### 2026-04-08 Modern Framework Review Note Complete

- Scope:
  - re-review the repository against current official AI-agent and voice-agent framework guidance
  - capture which top-level architecture decisions still hold and which capability gaps now matter most
  - publish one detailed Chinese research note plus one ADR so the outcome is durable and easy to revisit
- Result:
  - the current session-centric architecture remains the correct base
  - the next optimization priority is not framework replacement, but stronger voice-runtime orchestration, observability, skill/tool standardization, durable memory, and lightweight workflow support
- Follow-through:
  - added `docs/architecture/modern-ai-agent-framework-review-zh-2026-04-08.md`
  - updated `docs/architecture/overview.md`
  - added `docs/adr/0019-modern-voice-agent-optimization-builds-on-current-session-centric-architecture.md`
  - updated `.codex/change-log.md`
  - updated `.codex/project-memory.md`
- Validation:
  - documentation-only change; no code-path verification required

### 2026-04-08 Next-Framework Proposal Note Complete

- Scope:
  - design a more detailed next-generation project framework on top of the current session-centric architecture
  - turn the prior research conclusions into a concrete target layering model for future implementation work
  - record the result as a proposal rather than as current implemented architecture
- Result:
  - the target framework keeps the current session core and adds a stronger `Voice Orchestration Core`, `Agent Workflow Core`, `Capability Fabric`, `Context & Memory Fabric`, `Policy & Safety Fabric`, and `Eval Plane`
  - the proposal also clarifies a recommended package evolution path and phased migration order
- Follow-through:
  - added `docs/architecture/agent-server-next-framework-zh-2026-04-08.md`
  - updated `docs/architecture/overview.md`
  - added `docs/adr/0020-proposed-next-framework-keeps-session-core-and-adds-voice-workflow-capability-fabrics.md`
  - updated `.codex/change-log.md`
  - updated `.codex/project-memory.md`
- Validation:
  - documentation-only change; no code-path verification required

### 2026-04-08 Next-Framework Migration Plan Note Complete

- Scope:
  - turn the target-framework proposal into a staged migration plan from the current implementation
  - define migration phases, frozen external contracts, phase exit gates, and risk controls
  - keep the result explicitly documented as a proposal rather than as already completed engineering work
- Result:
  - the migration plan now breaks the move into `F0` through `F6`: baseline/traceability, voice orchestration, capability fabric, context and memory, workflow core, policy and safety, and eval plane
  - the plan also maps those framework stages back to the existing `P0 / P1 / P2` optimization track
- Follow-through:
  - added `docs/architecture/migration-plan-to-next-framework-zh-2026-04-08.md`
  - updated `docs/architecture/agent-server-next-framework-zh-2026-04-08.md`
  - updated `docs/architecture/overview.md`
  - updated `.codex/change-log.md`
  - updated `.codex/project-memory.md`
- Validation:
  - documentation-only change; no code-path verification required

### 2026-04-08 F0-1 Traceability Slice Complete

- Scope:
  - start the actual migration from the current implementation by landing the first `F0` slice
  - add stable server-assigned `turn_id` and `trace_id` through the shared turn path without breaking the existing websocket contracts
  - enrich the desktop runner with earlier phase timings so migration work can be measured instead of judged only by subjective feel
- Result:
  - shared turn requests now carry `turn_id` and `trace_id` from the gateway into the shared voice and agent runtime path
  - native realtime `response.start` now includes optional `turn_id` and `trace_id`, and turn-state `session.update` events now include `turn_id`
  - the Python desktop runner now records additional phase metrics such as `thinking_latency_ms`, `speaking_latency_ms`, and `active_return_latency_ms`
  - the `xiaozhi` compatibility path now also generates internal per-turn identifiers for the shared responder request path
- Follow-through:
  - added `internal/gateway/turn_trace.go`
  - updated `internal/gateway/realtime_runtime.go`
  - updated `internal/gateway/realtime_ws.go`
  - updated `internal/gateway/xiaozhi_ws.go`
  - updated `internal/voice/contracts.go`
  - updated `internal/voice/turn_executor.go`
  - updated `internal/agent/contracts.go`
  - updated `clients/python-desktop-client/src/agent_server_desktop_client/runner.py`
  - updated `clients/python-desktop-client/tests/test_runner.py`
  - updated `clients/python-desktop-client/README.md`
  - updated `docs/protocols/realtime-session-v0.md`
  - updated `schemas/realtime/session-envelope.schema.json`
  - updated `.codex/change-log.md`
  - updated `.codex/issues-and-resolutions.md`
  - updated `.codex/project-memory.md`
- Validation:
  - `go test ./internal/gateway ./internal/voice ./internal/agent`
  - `go test ./internal/app ./internal/control`
  - `PYTHONPATH=clients/python-desktop-client/src python3 -m unittest discover -s clients/python-desktop-client/tests -v`

### 2026-04-09 F0-2 Structured Turn Trace And Baseline Artifact Slice Complete

- Scope:
  - add server-side structured turn trace logs around turn acceptance, response start, speaking, interruption, completion, and close paths without changing the device-facing websocket contract
  - extend internal trace identifiers into shared ASR/TTS request objects so logging can correlate gateway, voice, runtime, and playback phases under one turn
  - enrich the desktop runner report with replay-friendly artifacts and summary metadata so future migration slices can compare behavior instead of relying on ad hoc logs
- Target files:
  - `internal/gateway/realtime_runtime.go`
  - `internal/gateway/realtime_ws.go`
  - `internal/gateway/xiaozhi_ws.go`
  - `internal/gateway/turn_trace.go`
  - `internal/voice/contracts.go`
  - `internal/voice/asr_responder.go`
  - `internal/voice/synthesis_audio.go`
  - `internal/voice/logging_*.go`
  - `internal/agent/*logging*.go`
  - `internal/app/app.go`
  - `clients/python-desktop-client/src/agent_server_desktop_client/runner.py`
  - `clients/python-desktop-client/tests/test_runner.py`
  - `clients/python-desktop-client/README.md`
  - `docs/architecture/overview.md`
  - `docs/architecture/migration-plan-to-next-framework-zh-2026-04-08.md`
  - `.codex/change-log.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - logs can correlate one turn across gateway, runtime, ASR/TTS, and playback with the same `turn_id` and `trace_id`
  - realtime and `xiaozhi` websocket contracts stay additive-only or unchanged
  - runner JSON output includes replay-friendly artifact references and enough metadata to compare runs across providers and execution modes
  - focused Go and Python tests pass before the next migration slice
- Result:
  - gateway logs now emit structured turn-phase records for native realtime and `xiaozhi` turns, including accepted, response-started, speaking, interrupted, completed, and terminal-error paths
  - shared runtime and voice layers now keep `turn_id` and `trace_id` through `agent.LoggingTurnExecutor`, `voice.LoggingTranscriber`, and `voice.LoggingSynthesizer`
  - the desktop runner now emits `generated_at`, `run_id`, `llm_provider`, richer quality-summary fields, per-scenario issue lists, and replay-friendly artifact references under `artifact_dir`
- Follow-through:
  - updated `internal/gateway/turn_trace.go`
  - updated `internal/gateway/realtime.go`
  - updated `internal/gateway/realtime_ws.go`
  - updated `internal/gateway/xiaozhi.go`
  - updated `internal/gateway/xiaozhi_ws.go`
  - updated `internal/agent/logging_turn_executor.go`
  - updated `internal/voice/contracts.go`
  - updated `internal/voice/asr_responder.go`
  - updated `internal/voice/synthesis_audio.go`
  - updated `internal/voice/logging_synthesizer.go`
  - added `internal/voice/logging_transcriber.go`
  - updated `internal/app/app.go`
  - updated `internal/app/app_test.go`
  - updated `internal/app/voice_runtime_test.go`
  - updated `clients/python-desktop-client/src/agent_server_desktop_client/protocol.py`
  - updated `clients/python-desktop-client/src/agent_server_desktop_client/runner.py`
  - updated `clients/python-desktop-client/tests/test_runner.py`
- Validation:
  - `env GOCACHE=/tmp/agent-server-go-build go test ./internal/gateway ./internal/voice ./internal/agent ./internal/app`
  - `PYTHONPATH=clients/python-desktop-client/src python3 -m unittest discover -s clients/python-desktop-client/tests -v`

### 2026-04-09 F0-3 Scripted Regression Scenario Expansion Complete

- Scope:
  - expand the desktop scripted runner with additional regression scenarios for barge-in, timeout, and tool-path behavior
  - treat the barge-in scenario as the first interrupted-TTS baseline by verifying that one response is cut short and another turn completes afterward
  - keep using the native `/v1/realtime/ws` contract and existing debug commands rather than introducing a second regression protocol
  - archive the new scenarios through the same replay-friendly runner artifact model introduced in `F0-2`
- Target files:
  - `clients/python-desktop-client/src/agent_server_desktop_client/runner.py`
  - `clients/python-desktop-client/src/agent_server_desktop_client/rtos_mock.py`
  - `clients/python-desktop-client/tests/test_runner.py`
  - `clients/python-desktop-client/tests/test_rtos_mock.py`
  - `clients/python-desktop-client/README.md`
  - `plan.md`
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - the archived runner report can include at least one native barge-in scenario, one timeout scenario, and one tool-path scenario in addition to the existing text/audio/server-end coverage
  - the barge-in scenario verifies interrupted playout rather than only a second response
  - the new scenarios remain compatible with the current native realtime contract and do not require protocol changes
  - focused Python tests pass before the next migration slice
- Result:
  - the desktop runner now supports `tool`, `barge-in`, `timeout`, and `regression` modes in addition to the earlier `text`, `audio`, `server-end`, and `full` modes
  - the broader `regression` suite archives one report containing `text`, `audio`, `server-end`, `tool`, `barge-in`, and `timeout`
  - per-scenario report payloads now capture multi-turn identifiers (`turn_ids`, `trace_ids`), session end reason, tool-delta counters, and response-start counts, which makes the new scenarios comparable without widening the realtime protocol
  - the barge-in scenario now validates interrupted playout semantics by confirming first-response audio, an interrupt-triggered return to `active`, and a second response with audio
- Follow-through:
  - updated `clients/python-desktop-client/src/agent_server_desktop_client/runner.py`
  - updated `clients/python-desktop-client/tests/test_runner.py`
  - updated `clients/python-desktop-client/README.md`
- Validation:
  - `python3 -m py_compile clients/python-desktop-client/src/agent_server_desktop_client/runner.py clients/python-desktop-client/src/agent_server_desktop_client/rtos_mock.py clients/python-desktop-client/tests/test_runner.py clients/python-desktop-client/tests/test_rtos_mock.py`
  - `PYTHONPATH=clients/python-desktop-client/src python3 -m unittest discover -s clients/python-desktop-client/tests -v`

### 2026-04-09 F0-4 RTOS Mock Baseline Artifact Alignment Complete

- Scope:
  - align `RTOSMockClient` output with the same run metadata, discovery metadata, identifier capture, issue tracking, and replay-friendly artifact vocabulary used by the desktop runner
  - preserve the current CLI shape and existing `--save-rx` quick WAV output while adding a directory-based artifact archival path for richer comparison
  - keep the RTOS mock on the native `/v1/realtime/ws` contract instead of introducing any mock-only protocol branch
- Target files:
  - `clients/python-desktop-client/src/agent_server_desktop_client/rtos_mock.py`
  - `clients/python-desktop-client/tests/test_rtos_mock.py`
  - `clients/python-desktop-client/README.md`
  - `plan.md`
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - RTOS mock JSON output includes run-level metadata and discovery metadata compatible with the desktop runner baseline vocabulary
  - RTOS mock archives replay-friendly artifacts such as events, response text, run summary, and received audio when a directory is provided
  - existing quick usage with `--save-rx` remains valid
  - focused Python tests pass before the next migration slice
- Result:
  - `RTOSMockClient` now emits run metadata, discovery metadata, identifier capture, issue tracking, and artifact references that align with the desktop runner baseline vocabulary
  - the RTOS mock can now archive `events.json`, `response.txt`, `run.json`, and `received-audio.wav` under a run directory via `--save-rx-dir`
  - the existing `--save-rx` quick WAV export path remains supported for simple one-off barge-in bring-up
- Follow-through:
  - updated `clients/python-desktop-client/src/agent_server_desktop_client/rtos_mock.py`
  - updated `clients/python-desktop-client/tests/test_rtos_mock.py`
  - updated `clients/python-desktop-client/README.md`
- Validation:
  - `python3 -m py_compile clients/python-desktop-client/src/agent_server_desktop_client/rtos_mock.py clients/python-desktop-client/tests/test_rtos_mock.py clients/python-desktop-client/src/agent_server_desktop_client/runner.py clients/python-desktop-client/tests/test_runner.py`
  - `PYTHONPATH=clients/python-desktop-client/src python3 -m unittest discover -s clients/python-desktop-client/tests -v`

### 2026-04-09 F0-5 Live Native Baseline Validation Complete

- Scope:
  - restore the local native realtime reference stack so the broader archived baseline can run against a live server again
  - run the first live native `regression` suite and save replay-friendly artifacts
  - run the RTOS mock against the same live stack so device-style artifacts can be compared with the native desktop runner output
  - confirm the new `turn_id` and `trace_id` observability path is visible in live gateway and ASR logs, not only in unit tests
- Runtime used for this validation step:
  - FunASR worker on `127.0.0.1:8091` from conda env `xiaozhi-esp32-server`
  - model `iic/SenseVoiceSmall`
  - device `cpu`
  - `agentd` on `127.0.0.1:8080` with:
    - `AGENT_SERVER_AGENT_LLM_PROVIDER=bootstrap`
    - `AGENT_SERVER_VOICE_PROVIDER=funasr_http`
    - `AGENT_SERVER_TTS_PROVIDER=none`
  - validation sample normalized to `16k/mono/pcm16le` at `artifacts/live-baseline/20260409/samples/qinyu_xiaoou_16k.wav`
- Acceptance for this execution step:
  - local worker and server health endpoints respond again on `8091` and `8080`
  - desktop `regression` completes live and archives one report plus per-scenario artifacts
  - RTOS mock completes live and archives one comparable run bundle
  - live logs show one stable `turn_id` and `trace_id` continuing across gateway accept, ASR, runtime, and response phases
- Result:
  - local health and discovery were restored successfully for `http://127.0.0.1:8091` and `http://127.0.0.1:8080`
  - desktop native regression run `run_f70f2b7bba3e` completed with `ok=true` across `text`, `audio`, `server-end`, `tool`, `barge-in`, and `timeout`
  - desktop regression quality summary reported:
    - `scenario_count=6`
    - `ok_scenarios=6`
    - `response_with_audio_ratio=0.833`
    - `non_empty_response_ratio=0.833`
    - `received_audio_bytes_total=7040`
  - RTOS mock run `run_e4675e6b3cfb` completed with `ok=true`, `interrupt_sent=true`, and two archived turn ids under one session
  - live server logs confirmed the same `turn_id` and `trace_id` across:
    - gateway accepted / response started / speaking / interrupted / completed phases
    - FunASR transcription start and completion
    - shared runtime turn start and completion
- Follow-through:
  - updated `plan.md`
  - updated `.codex/change-log.md`
  - updated `.codex/issues-and-resolutions.md`
  - updated `.codex/project-memory.md`
- Canonical artifacts:
  - `artifacts/live-baseline/20260409/desktop-regression/report.json`
  - `artifacts/live-baseline/20260409/desktop-regression/run_f70f2b7bba3e`
  - `artifacts/live-baseline/20260409/rtos-mock/report.json`
  - `artifacts/live-baseline/20260409/rtos-mock/run_e4675e6b3cfb`
- Validation:
  - `curl -sS http://127.0.0.1:8091/healthz`
  - `curl -sS http://127.0.0.1:8091/v1/asr/info`
  - `curl -sS http://127.0.0.1:8080/healthz`
  - `curl -sS http://127.0.0.1:8080/v1/realtime`
  - `env PYTHONPATH=/root/agent-server/clients/python-desktop-client/src python3 -m agent_server_desktop_client.runner --scenario regression --http-base http://127.0.0.1:8080 --wav /root/agent-server/artifacts/live-baseline/20260409/samples/qinyu_xiaoou_16k.wav --save-rx-dir /root/agent-server/artifacts/live-baseline/20260409/desktop-regression --output /root/agent-server/artifacts/live-baseline/20260409/desktop-regression/report.json`
  - `env PYTHONPATH=/root/agent-server/clients/python-desktop-client/src python3 -m agent_server_desktop_client.rtos_mock --http-base http://127.0.0.1:8080 --wav /root/agent-server/artifacts/live-baseline/20260409/samples/qinyu_xiaoou_16k.wav --interrupt-wav /root/agent-server/artifacts/live-baseline/20260409/samples/qinyu_xiaoou_16k.wav --save-rx-dir /root/agent-server/artifacts/live-baseline/20260409/rtos-mock --output /root/agent-server/artifacts/live-baseline/20260409/rtos-mock/report.json`

### 2026-04-10 Local Open-Source-First Full-Duplex Roadmap Recorded

- Scope:
  - record a concrete implementation roadmap for smoother and more natural full-duplex voice interaction
  - keep the repository on a local and open-source-first route instead of switching the primary plan to hosted realtime speech providers
  - map the roadmap directly onto the current repository boundaries and implementation areas
- Result:
  - added a current-state assessment note focused on full-duplex voice quality
  - added a local/open-source-first executable task list split into `L0` through `L5`
  - recorded the durable architecture decision in a new ADR
- Follow-through:
  - updated `docs/architecture/overview.md`
  - updated `.codex/project-memory.md`
  - updated `.codex/issues-and-resolutions.md`
  - updated `.codex/change-log.md`

### 2026-04-10 L0 And L1 First Implementation Slices Landed

- Scope:
  - turn the new full-duplex roadmap into the first concrete local/open-source-first implementation slices
  - add a shared streaming ASR boundary in `internal/voice` without leaking provider details into adapters
  - move the local `funasr_http` path from buffered compatibility mode toward a real worker-backed streaming session
- Result:
  - added `StreamingTranscriber`, `StreamingTranscriptionSession`, and `TranscriptionDelta` contracts plus a `BufferedStreamingTranscriber` compatibility adapter in `internal/voice`
  - updated `ASRResponder` to prefer streaming transcribers automatically while keeping batch compatibility and normalized speech metadata (`speech.elapsed_ms`, `speech.transcriber_mode`, `speech.partial_count`)
  - extended the desktop runner quality metrics with `first_partial_latency_ms`, `barge_in_cutoff_latency_ms`, `response_text_chars`, `partial_response_count`, `partial_response_chars`, and `heard_text_chars`
  - implemented local FunASR worker streaming lifecycle routes under `/v1/asr/stream/start|push|finish|close`
  - upgraded `HTTPTranscriber` to use the worker streaming lifecycle as a real `StreamingTranscriber`
  - switched the `funasr_http` voice runtime wiring from buffered compatibility mode to the real streaming path, while `iflytek_rtasr` stays on the compatibility wrapper for now
- Verification:
  - `go test ./internal/voice ./internal/app`
  - `env PYTHONPATH=workers/python/src python3 -m unittest discover -s workers/python/tests -v`
  - `env PYTHONPATH=clients/python-desktop-client/src python3 -m unittest discover -s clients/python-desktop-client/tests -v`
- Follow-through:
  - updated `docs/architecture/overview.md`
  - updated `workers/python/README.md`
  - updated `.codex/project-memory.md`
  - updated `.codex/issues-and-resolutions.md`
  - updated `.codex/change-log.md`

### 2026-04-11 L2 Minimal Server-Endpoint Preview Slice Landed

- Scope:
  - start `L2` without changing the public websocket contracts or discovery turn-mode
  - add a shared voice-runtime input-preview capability that can observe streaming ASR partials and suggest internal auto-commit after silence
  - let native realtime and `xiaozhi` websocket adapters consume that shared capability instead of embedding provider-specific endpoint logic
- Result:
  - added `InputPreviewer`, `InputPreviewSession`, and `InputPreview` contracts in `internal/voice`
  - added a default `SilenceTurnDetector` and made `ASRResponder` expose preview sessions when its transcriber supports streaming
  - added a hidden runtime switch `AGENT_SERVER_VOICE_SERVER_ENDPOINT_ENABLED`
  - when that switch is enabled, native realtime and `xiaozhi` websocket handlers now keep an internal preview session, poll it on short read deadlines, and may auto-commit an audio turn after a local silence window
  - public discovery still reports `client_wakeup_client_commit`, and no public event schema was changed
- Verification:
  - `go test ./internal/voice ./internal/gateway ./internal/app`
  - `env PYTHONPATH=workers/python/src python3 -m unittest discover -s workers/python/tests -v`
  - `env PYTHONPATH=clients/python-desktop-client/src python3 -m unittest discover -s clients/python-desktop-client/tests -v`
- Follow-through:
  - updated `docs/architecture/overview.md`
  - updated `docs/architecture/runtime-configuration.md`
  - updated `docs/adr/0021-local-open-source-first-full-duplex-roadmap-prioritizes-voice-orchestration.md`
  - updated `.codex/project-memory.md`
  - updated `.codex/issues-and-resolutions.md`
  - updated `.codex/change-log.md`
