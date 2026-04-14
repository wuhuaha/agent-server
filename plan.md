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
- [x] Route future channel adapters through this runtime boundary instead of responder-local logic.

### M2 Channel Skill Framework

- [x] Finalize the shared channel contract and runtime bridge around the `Agent Runtime Core` handoff.
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
- the Codex harness now uses a thin root `plan.md`, an execution-log archive under `docs/codex/`, and shared GitHub templates that require boundary plus validation context
- the Codex harness now also has a canonical live-validation runbook plus artifact naming convention under `docs/codex/live-validation-runbook.md`
- Linux-side archived-output live-smoke helpers now exist alongside the PowerShell scripts
- the Codex harness now also has a Web/H5 manual-evidence scaffolding helper under `scripts/web-h5-manual-capture.sh`
- the next Codex harness follow-up should decide whether browser console export and screenshot naming should stay manual or gain another lightweight helper layer
- the voice-runtime ownership migration is now landed: hidden preview, playout callbacks, and heard-text persistence live behind `internal/voice.SessionOrchestrator`
- external channel follow-up should build the first Feishu adapter on top of `internal/channel.RuntimeBridge` instead of adding another adapter-local orchestration path
- startup config is now split by runtime domain and validated before handler wiring, so future provider additions should extend `Config.Validate()` instead of relying on request-time failures

## Current Execution Log

Detailed historical execution history now lives in:

- `docs/codex/execution-log-archive-2026-04.md`

Keep this root ledger focused on active direction, recent execution context, and next-step decisions. When a completed slice stops affecting immediate work, summarize it in the archive instead of extending the root plan.

### 2026-04-13 Codex Planning Context And Collaboration Template Slice Complete

- Scope:
  - reduce top-level planning context weight by moving older completed execution detail out of `plan.md`
  - add repository-level issue and PR templates so future work proposals and submissions carry boundary, protocol, and validation context by default
  - keep those templates aligned with the shared `Makefile` command surface and the repo's required follow-through files
- Target files:
  - `plan.md`
  - `docs/codex/execution-log-archive-2026-04.md`
  - `docs/codex/harness-workflow.md`
  - `.github/ISSUE_TEMPLATE/bug-report.md`
  - `.github/ISSUE_TEMPLATE/architecture-task.md`
  - `.github/ISSUE_TEMPLATE/config.yml`
  - `.github/pull_request_template.md`
  - `.claude/context.md`
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - `plan.md` keeps only active planning state and a small recent execution window
  - older completed slices remain accessible through a dedicated archive doc instead of inflating the root context
  - GitHub issue and PR templates ask for affected boundaries, protocol or ADR impact, and validation using the shared command surface
  - Codex-facing workflow docs point contributors at the archive and templates explicitly

Validation recorded for this execution step:

- `git diff --check`
- `make doctor`
- `make verify-fast`

Observed outcome:

- `plan.md` now stays focused on current direction while older completed slices are summarized in a dedicated archive document
- GitHub templates now reinforce the repository's architecture-first workflow, protocol follow-through, and shared validation commands
- `docs/codex/harness-workflow.md` and `.claude/context.md` now point coding agents at the planning archive and template-based collaboration path instead of letting root context grow again

Recorded follow-through:

- archived older execution history into `docs/codex/execution-log-archive-2026-04.md`
- added repository issue templates for bug reports and architecture or feature tasks
- added a pull-request template aligned with the repo guardrails and standard command surface
- updated `plan.md`, `docs/codex/harness-workflow.md`, `.claude/context.md`, and `.codex/` durable records

### 2026-04-13 Codex Live Validation Runbook Slice Complete

- Scope:
  - standardize how live local validation is described and archived without widening the fast CI surface
  - define stable artifact roots, profile names, and canonical top-level filenames for smoke runs versus comparison-worthy baselines
  - align the existing Windows smoke scripts with that artifact convention so the runbook is reflected in real tooling
- Target files:
  - `docs/codex/live-validation-runbook.md`
  - `docs/codex/harness-workflow.md`
  - `README.md`
  - `clients/python-desktop-client/README.md`
  - `scripts/smoke-funasr.ps1`
  - `scripts/smoke-rtos-mock.ps1`
  - `plan.md`
  - `.claude/context.md`
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - the repository has one canonical doc for live-validation run selection, artifact roots, profile names, and file layout
  - the root harness docs point at that runbook explicitly
  - smoke scripts default to repository-local `artifacts/live-smoke/YYYYMMDD/<profile>/` paths and canonical `report.json` naming
  - desktop-runner and RTOS-mock docs now point users at the same artifact convention

Validation recorded for this execution step:

- `git diff --check`
- `make doctor`
- `make verify-fast`
- PowerShell parse check for:
  - `scripts/smoke-funasr.ps1`
  - `scripts/smoke-rtos-mock.ps1`

Observed outcome:

- live validation now has one canonical naming and archiving convention instead of being spread across historical notes and one-off artifact paths
- repository docs now distinguish clearly between `artifacts/live-smoke/` for quick reruns and `artifacts/live-baseline/` for comparison-worthy runs
- the Windows smoke scripts now default to repository-relative artifact roots rather than historical machine-specific paths

Recorded follow-through:

- added `docs/codex/live-validation-runbook.md`
- updated harness, root, and desktop-client docs to point at the same convention
- aligned the Windows smoke scripts with repository-local artifact roots and canonical top-level filenames

### 2026-04-13 Codex Linux Live-Smoke Helper Slice Complete

- Scope:
  - give Linux the same one-command archived-output smoke path that Windows already had through PowerShell scripts
  - keep the helper behavior aligned with the live-validation runbook instead of inventing a second artifact layout
  - avoid widening the fast CI surface; this slice should stay in the manual live-validation tier
- Target files:
  - `scripts/smoke-funasr.sh`
  - `scripts/smoke-rtos-mock.sh`
  - `docs/codex/live-validation-runbook.md`
  - `docs/codex/harness-workflow.md`
  - `README.md`
  - `clients/python-desktop-client/README.md`
  - `plan.md`
  - `.claude/context.md`
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - Linux has one-command smoke helpers for desktop runner and RTOS mock flows
  - those helpers default to `artifacts/live-smoke/YYYYMMDD/<profile>/` under the repository root
  - root docs and runbook clearly describe Linux and Windows parity for archived-output smoke runs
  - helper scripts pass syntax validation and the standard fast command surface still passes afterward

Validation recorded for this execution step:

- `bash -n scripts/smoke-funasr.sh`
- `bash -n scripts/smoke-rtos-mock.sh`
- `./scripts/smoke-funasr.sh --help`
- `./scripts/smoke-rtos-mock.sh --help`
- `git diff --check`
- `make doctor`
- `make verify-fast`

Observed outcome:

- Linux now has repository-local one-command smoke helpers for both desktop and RTOS mock paths
- the runbook's live-smoke artifact roots are now reflected in real tooling on both Windows and Linux
- when no speech sample is provided, the Linux helpers generate a silence `input.wav` so the end-to-end stack can still be exercised without external files

Recorded follow-through:

- added `scripts/smoke-funasr.sh`
- added `scripts/smoke-rtos-mock.sh`
- updated the runbook, harness docs, root README, desktop-client README, and durable repo records

### 2026-04-13 Codex Web/H5 Manual Evidence Slice Complete

- Scope:
  - standardize browser-side manual validation evidence so Web/H5 checks stop depending on ad hoc screenshots and temporary notes
  - create a repository-local helper that scaffolds the canonical `web-h5-manual` artifact root and captures server plus page snapshots before manual interaction
  - wire the helper into the browser bring-up docs and Codex runbook instead of leaving it as an undocumented side path
- Target files:
  - `scripts/web-h5-manual-capture.sh`
  - `docs/codex/live-validation-runbook.md`
  - `docs/codex/harness-workflow.md`
  - `docs/protocols/web-h5-realtime-adaptation.md`
  - `README.md`
  - `tools/web-client/README.md`
  - `plan.md`
  - `.claude/context.md`
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - the repository has one helper that scaffolds a `web-h5-manual` artifact root with server snapshots and manual checklist files
  - the live-validation runbook documents the expected Web/H5 evidence layout explicitly
  - built-in and standalone browser docs both point at the same helper and artifact convention
  - helper script syntax, `--help`, and the standard fast command surface all validate cleanly

Validation recorded for this execution step:

- `bash -n scripts/web-h5-manual-capture.sh`
- `./scripts/web-h5-manual-capture.sh --help`
- `./scripts/web-h5-manual-capture.sh --skip-fetch --output-dir /tmp/agent-server-web-h5-manual`
- `git diff --check`
- `make doctor`
- `make verify-fast`

Observed outcome:

- Web/H5 manual validation now has a canonical evidence bundle with `capture.json`, `manual-checklist.md`, server snapshots, page snapshots, and prepared attachment directories
- built-in and standalone browser bring-up docs now point at the same evidence flow instead of relying on historical notes
- browser-side screenshots, console exports, and WAV exports remain manual actions, but they now land in a predictable artifact structure

Recorded follow-through:

- added `scripts/web-h5-manual-capture.sh`
- updated the runbook, harness docs, Web/H5 protocol guide, root README, standalone tool README, and durable repo records
- updated `plan.md`, `.claude/context.md`, and `.codex/` durable records

### 2026-04-13 Iteration 1 Validation Surface And Gateway Shared Turn Flow Slice Complete

- Scope:
  - harden the shared command surface so Python version failures and worker-test scope are explicit
  - stop relying on script execute bits from `Makefile`
  - reduce duplicate realtime versus `xiaozhi` turn lifecycle code without changing published protocol shapes
- Target files:
  - `Makefile`
  - `scripts/require-python-3-11.sh`
  - `scripts/test-python-desktop.sh`
  - `scripts/test-python-workers.sh`
  - `scripts/codex-doctor.sh`
  - `scripts/verify-fast.sh`
  - `internal/gateway/turn_flow.go`
  - `internal/gateway/output_flow.go`
  - `internal/gateway/realtime_ws.go`
  - `internal/gateway/xiaozhi_ws.go`
  - `docs/architecture/overview.md`
  - `docs/adr/0022-websocket-read-failures-are-terminal.md`
  - `docs/adr/0023-gateway-adapters-share-turn-flow-before-voice-migration.md`
  - `AGENTS.md`
  - `README.md`
  - `docs/codex/harness-workflow.md`
  - `.claude/context.md`
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - `make doctor`, `make test-py`, and `make test-py-workers` fail fast with clear Python 3.11+ requirements
  - `make verify-fast` keeps the narrow fast path but no longer duplicates raw desktop test commands inline
  - native realtime and `xiaozhi` adapters share the same turn-response and output-lifecycle helper path
  - existing gateway websocket tests remain green with no protocol changes

Validation recorded for this execution step:

- `bash -n scripts/require-python-3-11.sh scripts/test-python-desktop.sh scripts/test-python-workers.sh scripts/codex-doctor.sh scripts/verify-fast.sh`
- `go test ./...`
- `make doctor`
- `make test-py`
- `make test-py-workers`
- `make verify-fast`

Observed outcome:

- Python entrypoints now validate `3.11+` explicitly and worker tests have their own stable make target
- the shared command surface no longer depends on script execute bits from `Makefile`
- gateway turn execution, interruption return-to-active, and active/end completion logic now live in shared helpers instead of separate native and `xiaozhi` copies

### 2026-04-13 Review-Driven Runtime Ownership Refactor Slice Complete

- Scope:
  - finish the review-driven phase 2 to 6 refactor after the earlier gateway turn-flow sharing slice
  - move hidden preview and playout memory ownership into `internal/voice`
  - add heard-text persistence for interrupted spoken replies
  - split app config by domain and fail invalid combinations at startup
  - add the first shared channel runtime bridge so future adapters stay normalize -> handoff -> deliver
- Target files:
  - `internal/voice/session_orchestrator.go`
  - `internal/agent/contracts.go`
  - `internal/agent/runtime_skill.go`
  - `internal/agent/runtime_skill_household.go`
  - `internal/app/config*.go`
  - `internal/channel/contracts.go`
  - `internal/channel/runtime_bridge.go`
  - `docs/architecture/overview.md`
  - `docs/architecture/runtime-configuration.md`
  - `docs/protocols/channel-skill-contract-v0.md`
  - `docs/adr/0024-voice-runtime-owns-preview-playout-and-heard-text.md`
  - `docs/adr/0025-channel-adapters-use-a-shared-runtime-bridge.md`
  - `.claude/context.md`
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - gateway adapters no longer own hidden preview or playout memory persistence
  - memory writeback can distinguish generated, delivered, and heard text for interrupted spoken replies
  - invalid provider combinations fail in `NewServer(...)` before handler wiring
  - `internal/channel` exposes a shared runtime bridge that keeps adapters off provider APIs

Validation recorded for this execution step:

- `go test ./internal/app ./internal/agent ./internal/voice ./internal/gateway`
- `go test ./internal/channel ./internal/app ./internal/agent ./internal/voice ./internal/gateway`

Observed outcome:

- hidden preview polling and playout writeback now flow through one shared `internal/voice` orchestrator
- runtime memory now persists heard-text state for interrupted spoken output
- runtime skills are now registry-backed instead of being hardwired into the core backend wiring
- startup config validation now fails fast on invalid provider or credential combinations
- the first channel handoff path is now shared and provider-neutral

Recorded follow-through:

- added `SessionOrchestrator` under `internal/voice`
- extended runtime memory records with delivered or heard or truncated playback state
- split `internal/app` config by domain and validated it at `NewServer(...)`
- added `internal/channel.RuntimeBridge` plus delivery-status reporting primitives
- updated architecture docs, runtime configuration docs, channel contract docs, and ADRs

### Recent Slices Still Relevant

- `2026-04-14 Root Agent And Skill Directory Pruning`
  - reviewed the imported ECC reference pack under root `agents/` and `skills/`, removed items unrelated to the current Go or Python or voice-agent or deployment stack, and cleaned broken residual references from the kept skill docs
  - validation: `rg` reference sweep across `agents/`, `skills/`, `AGENTS.md`, `.codex/`, `.claude/`, and docs
- `2026-04-14 Gateway Write-Path Hardening And Audio Hot-Path Trim`
  - added websocket write deadlines and close-on-write-error behavior for native realtime and `xiaozhi` peers, fixed the recoverable `session_not_started` audio error to actually keep the connection alive, and trimmed the first hot-path copy/write amplifiers in session ingest, buffered streaming ASR chunking, voice playback persistence, and in-memory turn upserts
  - validation: `go test ./internal/gateway ./internal/session ./internal/voice ./internal/agent`
- `2026-04-13 Codex Harness P0`
  - shortened `AGENTS.md`, introduced `docs/codex/harness-workflow.md`, standardized `Makefile` and `scripts` entrypoints, and added fast CI
  - validation: `make test-go`, `make test-py`, `make doctor`, `make docker-config`, `make verify-fast`
- `2026-04-13 Docker Validation Follow-up`
  - validated layered compose files and the `agentd` image on this WSL2 machine, while the CPU worker image remains externally gated by large PyTorch wheel downloads
  - validation: layered `docker compose ... config` plus `docker compose ... build agentd`
- `2026-04-13 Codex Live Validation Runbook`
  - standardized live-validation artifact roots, profile names, top-level filenames, and smoke-script defaults
  - validation: `git diff --check`, `make doctor`, `make verify-fast`, and PowerShell parse checks for the smoke scripts
- `2026-04-13 Codex Linux Live-Smoke Helpers`
  - added Linux one-command archived-output smoke helpers for desktop runner and RTOS mock flows
  - validation: `bash -n`, `--help`, `make doctor`, and `make verify-fast`
- `2026-04-13 Codex Web/H5 Manual Evidence`
  - added a helper for canonical browser-validation artifact bundles covering server snapshots, page snapshots, and manual checklists
  - validation: `bash -n`, `--help`, `--skip-fetch`, `make doctor`, and `make verify-fast`
- `2026-04-13 Linux Dependency Install Consolidation`
  - standardized Linux bring-up under `scripts/install-linux-stack.sh` and validated `silero-vad` plus `onnxruntime` in the worker env
  - validation: `./scripts/install-linux-stack.sh --with-stream-vad`
- `2026-04-11 L2 Preview Config And Validation`
  - exposed hidden server-endpoint preview thresholds through shared voice-runtime config and added explicit desktop-runner validation for audio without commit
  - validation: `go test ./internal/voice ./internal/app` plus desktop-client unit tests

For older completed slices covering `P0/P1` runtime work, Web/H5 bring-up, frontend iterations, `F0` traceability, and the local/open-source full-duplex roadmap, see the archive document above.
