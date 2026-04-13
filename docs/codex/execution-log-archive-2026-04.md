# Execution Log Archive (2026-04)

This archive keeps completed execution slices that no longer need to stay in the root `plan.md`.
Use `plan.md` for active direction and the recent execution window.
Use this file when you need older implementation context, validation notes, or milestone history.

## 2026-04-04 Foundation Hardening

- `P0-1` removed turn-audio snapshot copy overhead in the session path and added focused gateway and session validation.
- `P0-2` aligned advertised `turn_mode` semantics with the real client-commit flow and updated discovery, docs, and schemas together.
- Durable follow-through for that slice updated architecture docs, ADR `0009`, and the `.codex` records.

## 2026-04-05 Runtime Hardening And Runtime Intelligence

- `P0-3` separated assistant persona from execution mode so simulation, dry-run, and live-control policy stay runtime-owned.
- `P0-4` upgraded the desktop runner into a comparable quality-report surface with latency and audio counters.
- `P1-1` added a streaming-capable LLM contract plus bounded model or tool loop inside `internal/agent`.
- `P1-2` layered recent-message context over summary memory and kept memory scope ownership inside the runtime boundary.
- `P1-3` normalized speech-understanding metadata before it entered the shared agent runtime.
- `P1-4` added a first bounded household-context routing layer inside `internal/agent`.
- `P1-5` improved `xiaozhi` compatibility with ASR-derived `stt` echo while keeping the adapter as a thin shim.

Representative validation:

- `go test ./internal/agent`
- `go test ./internal/app`
- `go test ./internal/voice`
- `go test ./internal/gateway`
- `PYTHONPATH=clients/python-desktop-client/src python3 -m unittest discover -s clients/python-desktop-client/tests -v`

## 2026-04-05 Web/H5 And Standalone Browser Bring-Up

- Added a first native Web/H5 direct client against `/v1/realtime/ws` without creating a browser-only protocol.
- Added the standalone `tools/web-client` debug tool for text turns, mic turns, interrupts, and raw envelope inspection.
- Documented browser constraints and the shared native realtime contract in `docs/protocols/web-h5-realtime-adaptation.md`.

Representative validation:

- `go test ./internal/app ./internal/control`
- `node --check internal/control/webh5_assets/app.js`
- `node --check tools/web-client/app.js`
- `python3 -m py_compile tools/web-client/serve.py`

## 2026-04-07 Local Integration And Frontend Stabilization

- Restored local loopback validation with `funasr_http + tts=none + bootstrap` and recorded the silence-rejection caveat on the local FunASR reference path.
- Fixed the websocket timeout panic so native realtime and `xiaozhi` handlers do not read again from a failed websocket connection.
- Refreshed the browser debug surfaces, split settings and debug pages, surfaced TTS diagnostics, and fixed empty-stream fallback on the MiMo TTS path.
- Refined the browser interaction flow to feel more like a voice console, inspired by `py-xiaozhi`.
- Changed the default LLM selection so DeepSeek becomes the effective provider when a key is present, and exposed the effective provider through discovery.

Representative validation:

- `go test ./internal/gateway`
- `go test ./internal/app ./internal/gateway`
- `go test ./internal/voice ./internal/app ./internal/control`
- `env PYTHONPATH=clients/python-desktop-client/src python3 -m agent_server_desktop_client.runner --scenario full --http-base http://127.0.0.1:8080 ...`

## 2026-04-08 Runtime Skill Boundary And Architecture Research

- Moved household-control behavior behind runtime skills so domain behavior enters through prompt fragments and tools instead of hardcoded executor branches.
- Recorded a modern framework review that concluded the session-centric architecture should stay and be strengthened rather than replaced.
- Added a next-framework proposal and a staged migration plan (`F0` through `F6`) on top of the existing session core.

Resulting durable direction:

- keep the current session-centric architecture
- strengthen voice orchestration, evals, capability fabric, memory, and workflow support inside that architecture
- keep public realtime and compatibility contracts stable during migration

## 2026-04-08 To 2026-04-09 F0 Traceability And Baseline Artifacts

- `F0-1` introduced shared `turn_id` and `trace_id` across gateway, voice, runtime, and client-visible response events.
- `F0-2` added structured turn-phase logs and richer desktop runner artifacts.
- `F0-3` expanded the runner with `tool`, `barge-in`, `timeout`, and `regression` scenarios.
- `F0-4` aligned RTOS mock artifacts with the desktop runner vocabulary.
- `F0-5` restored the live native baseline stack and archived canonical desktop and RTOS baseline artifacts.

Representative artifact locations:

- `artifacts/live-baseline/20260409/desktop-regression/`
- `artifacts/live-baseline/20260409/rtos-mock/`

Representative validation:

- `go test ./internal/gateway ./internal/voice ./internal/agent ./internal/app`
- `PYTHONPATH=clients/python-desktop-client/src python3 -m unittest discover -s clients/python-desktop-client/tests -v`
- live local `curl`, desktop runner, and RTOS mock validation against `127.0.0.1:8080`

## 2026-04-10 Local/Open-Source-First Full-Duplex Roadmap

- Recorded the local/open-source-first roadmap for smoother full-duplex voice interaction.
- Landed the first `L0/L1` slices:
  - shared `StreamingTranscriber` contracts in `internal/voice`
  - worker-backed FunASR streaming lifecycle under `/v1/asr/stream/*`
  - runner metrics for partial latency and barge-in quality

Representative validation:

- `go test ./internal/voice ./internal/app`
- `PYTHONPATH=workers/python/src python3 -m unittest discover -s workers/python/tests -v`
- `PYTHONPATH=clients/python-desktop-client/src python3 -m unittest discover -s clients/python-desktop-client/tests -v`

## 2026-04-11 Hidden Preview Rollout

- Landed the minimal hidden `L2` server-endpoint preview slice without widening the public discovery contract.
- Added runtime-configurable preview thresholds and a dedicated desktop-runner scenario for audio upload without `audio.in.commit`.

Representative validation:

- `go test ./internal/voice ./internal/gateway ./internal/app`
- `go test ./internal/voice ./internal/app`
- desktop-client unit tests

## 2026-04-13 L2 Endpointing Hardening

- Added a conservative lexical false-endpoint guard to avoid cutting obviously unfinished phrases.
- Added worker-side provider endpoint hints and taught the shared turn detector to consume them conservatively.
- Added optional stronger local acoustic evidence through `Silero VAD`, with graceful fallback to the tail-energy hint path.

Representative validation:

- `go test ./internal/voice ./internal/app ./internal/gateway`
- `PYTHONPATH=workers/python/src python3 -m unittest discover -s workers/python/tests -v`

## 2026-04-13 Linux Bring-Up Consolidation

- Added `scripts/install-linux-stack.sh` as the repository-local Linux install entrypoint.
- Declared worker `runtime` and `stream-vad` extras explicitly.
- Validated `onnxruntime` and `silero-vad` in the `xiaozhi-esp32-server` conda env and confirmed a live preview hint of `preview_silero_vad_silence`.

Representative validation:

- `./scripts/install-linux-stack.sh --with-stream-vad`
- `conda run -n xiaozhi-esp32-server python -c "import silero_vad, onnxruntime; ..."`

## 2026-04-13 Dockerization And Docker Validation

- Formalized the first layered Docker deployment slice under `deploy/docker`.
- Kept `agentd` and the local FunASR worker split across containers.
- Validated layered compose config expansion and a real `agentd` image build on this WSL2 machine.
- Hardened Docker assets for the machine's constrained-network reality:
  - `scratch` runtime image for `agentd`
  - Docker build defaults for `GOPROXY` and `GOSUMDB`
  - removed the unused apt layer from the CPU worker image
  - added proxy passthrough for compose and worker builds

Current caveat retained from that work:

- the CPU FunASR worker image still depends on stable external access to the large PyTorch wheel from `download-r2.pytorch.org`

## 2026-04-13 Codex Harness P0

- Shortened `AGENTS.md` into a high-signal repo-specific instruction file.
- Added `docs/codex/harness-workflow.md` as the deeper Codex execution guide.
- Standardized the repository command surface through `Makefile` and helper scripts.
- Added fast GitHub Actions coverage for Go tests, Python desktop-client tests, and layered Docker compose config checks.

Representative validation:

- `make test-go`
- `make test-py`
- `make doctor`
- `make docker-config`
- `make verify-fast`

## Archive Maintenance Rule

- Keep `plan.md` focused on active direction and the recent execution window.
- When a slice is complete and no longer changes immediate implementation choices, summarize it here instead of expanding the root plan again.
