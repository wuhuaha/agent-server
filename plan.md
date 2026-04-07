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

## Immediate Next Step

Resume the remaining `P1-5` compatibility backlog from `docs/architecture/project-optimization-roadmap-zh-2026-04.md`: basic `mcp` capability negotiation, `iot` state uplink, and first auth/token checks on the `xiaozhi` compatibility path. Keep the new local loopback validation stack (`funasr_http + tts=none + bootstrap + tools/web-client`) as the first zero-external-dependency smoke path for future changes.
