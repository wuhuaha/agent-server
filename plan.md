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
- `docs/architecture/voice-demo-realtime-optimization-zh-2026-04-14.md`
- `docs/architecture/realtime-full-duplex-gap-review-zh-2026-04-15.md`
- `docs/architecture/voice-architecture-blueprint-zh-2026-04-16.md`
- `docs/architecture/voice-architecture-execution-roadmap-zh-2026-04-16.md`
- `docs/architecture/full-duplex-voice-assessment-zh-2026-04-10.md`
- `docs/architecture/local-open-source-full-duplex-roadmap-zh-2026-04-10.md`
- `docs/protocols/realtime-voice-client-collaboration-proposal-v0-zh-2026-04-16.md`

Current planning note:

- the current phase-1 voice demo focus is perceived realtime quality first: true streaming ASR, stronger endpointing, adaptive interruption handling, earlier incremental TTS start, and spoken-style response planning should outrank broader capability surface
- the latest S1/S2/S3/S4 review now makes the main structural bottleneck explicit: current realtime quality is limited more by the single-track session state machine than by missing endpoint heuristics, so the next architecture slice should prioritize an internal input/output dual-track session model before more endpoint or planner tuning is treated as the primary fix
- for the next full-duplex voice stage, the primary execution route is now explicitly `local / open-source first`
- hosted realtime speech providers remain comparison baselines, not the main implementation target
- the first public collaboration slice is now landing on native realtime as an additive capability-gated contract: discovery exposes `voice_collaboration`, `session.start` can negotiate `preview_events` plus `playback_ack`, and the public wire can now optionally carry `input.*` preview observations plus `audio.out.meta` and playback-fact ACKs without replacing the v0 baseline
- the initial modular local FunASR worker slice is now landed: the worker can stay on backward-compatible buffered preview or switch internally to `online preview + final-ASR correction`, and worker-side `KWS` remains optional plus default-off
- the next voice-demo follow-up should benchmark concrete local model combinations and live latency/accuracy trade-offs on top of that worker boundary rather than widening the public realtime contract again
- the repository now has a formal voice architecture baseline in `docs/architecture/voice-architecture-blueprint-zh-2026-04-16.md`; future voice changes should align with its server-primary hybrid boundary, layered early-processing gate, output orchestration, playback-truth chain, and milestone-latency quality model
- the latest archived CPU benchmark now shows that `SenseVoiceSmall + paraformer-zh-streaming + fsmn-vad` is viable only after worker preload plus readiness gating, but it is not yet the best default for the CPU demo path: it kept command-only accuracy, failed to improve the wake-word-prefixed sample, and increased response-start latency from about `2.05 s` to about `3.5 s`
- the current local FunASR `1.3.1` runtime rejects the short KWS alias `fsmn-kws` during preload (`fsmn-kws is not registered`), but the calibrated enabled-KWS baseline is now `iic/speech_charctc_kws_phone-xiaoyun`; that worker path also needs `keywords` plus `output_dir` during `AutoModel(...)` init and still stays default-off
- the current V100 production host cannot use the newer `torch 2.11.0+cu128` wheel family because the official wheel omits `sm_70` kernels; the long-running GPU FunASR path is now validated with a dedicated data-volume runtime on `torch 2.7.1+cu126` plus `torchaudio 2.7.1+cu126`
- the current long-running GPU voice path is now `SenseVoiceSmall + paraformer-zh-streaming + fsmn-vad` on `cuda:0`, with model preload plus readiness gating enabled and KWS still default-off
- the current machine now also runs a long-running local GPU TTS runtime through `agent-server-cosyvoice-fastapi.service`, backed by `/home/ubuntu/kws-training/data/agent-server-runtime/cosyvoice-py310` plus the staged model at `/home/ubuntu/kws-training/data/agent-server-cache/modelscope/models/iic/CosyVoice-300M-SFT-runtime`
- the long-running `agentd` path is now cut over to `AGENT_SERVER_TTS_PROVIDER=cosyvoice_http`; direct `inference_sft` curl plus local realtime text/audio smoke runs both return audio with `tts_provider=cosyvoice_http`
- the current machine has revalidated its public edge after the GPU cutover: `101.33.235.154` now serves healthy HTTP and HTTPS through `nginx`, and public-IP realtime text smoke also returns audio with `tts_provider=cosyvoice_http`
- the shared realtime TTS regression discovered after the CosyVoice cutover is now fixed: the gateway no longer cancels responder-owned audio streams immediately after `RespondStream(...)` returns, and speech-planner turns no longer launch a second redundant full-response TTS request when a planned stream already exists
- the repaired service is now validated on the same committed-WAV audio sample both locally and through the public edge, with archived success runs at:
  - `artifacts/live-smoke/20260415/local-systemd-cosyvoice-audio-postfix/run_120700_3677/run_935960053343`
  - `artifacts/live-smoke/20260415/public-edge-cosyvoice-audio-postfix/run_120710_10469/run_846a70c2f15d`
  - `artifacts/live-smoke/20260415/public-edge-cosyvoice-text-postfix/run_120720_31537/run_3db52dc40d16`
- the current mainline observability slice for phase-1 realtime quality is now landed in shared gateway code instead of adapter-local patches:
  - preview lifecycle logs now emit `preview_id`, `preview_first_partial_latency_ms`, `preview_commit_suggest_latency_ms`, `preview_audio_bytes`, and `preview_endpoint_reason`
  - turn lifecycle logs now emit `first_text_delta_latency_ms` and `first_audio_chunk_latency_ms`
  - accepted speaking-state interruption now logs a structured barge-in decision including acceptance reason, candidate audio duration, lexical completeness, and the linked preview trace
- the current device-path debug baseline now also includes websocket downlink lifecycle markers:
  - inbound websocket termination logs now include `ws_close_code`, `ws_close_text`, and `remote_addr`
  - outbound lifecycle logs now explicitly report send success or failure for `response.start`, speaking updates, streamed text chunks, audio binary chunks, return-to-active updates, and `session.end`
  - when a real-device turn shows `asr transcription stream completed` and `agent turn completed` before `tts stream setup failed`, the next diagnosis focus should stay on downlink/TTS lifecycle rather than ASR
- the first S1/S2/S3/S4 convergence slice for realtime duplexing is now landed in the shared stack:
  - `internal/session` now keeps separate `input_state` and `output_state` lanes, while `state` stays as a derived compatibility view
  - server-emitted `session.update` may now carry `input_state`, `output_state`, and `accept_reason`
  - `server_endpoint` now drives accepted-turn updates through that same compatibility contract, with preview speech-start and endpoint-candidate observability kept as runtime logs instead of new public v0 events
  - speaking-time preview and staged overlap audio now survive natural playback completion instead of being dropped when the current output ends first
  - interruption strategy is now shared runtime behavior with `ignore`, `backchannel`, `duck_only`, and `hard_interrupt`, although only hard interrupt currently changes playout
- the next realtime duplex follow-up should now move from state-shape plumbing to behavior depth:
  - keep extending the new soft-output policy path beyond native realtime into the remaining adapters and resume/continue policy
  - keep pushing earlier audio start deeper into the output lane, with adapter coverage and latency measurement becoming the next bottleneck instead of basic hook shape
  - native realtime now already applies true soft ducking for `duck_only/backchannel` and can start planned audio before final turn settlement through the shared orchestrator hook
- the current local LLM cutover path keeps the shared runtime boundary unchanged:
  - the new local worker exposes an OpenAI-compatible `chat/completions` surface behind the existing `deepseek_chat` executor path
  - the current machine-first model target is `Qwen/Qwen3-4B-Instruct-2507`, not `8B`, because this V100 host already shares one GPU across ASR, TTS, and now LLM
  - the built-in prompt now includes current local time/date context so relative-date questions such as `明天周几` do not depend only on model priors
- for this host's unprivileged deployment loop, `scripts/run-agentd-local.sh` now supports a repo-local override binary at `.runtime/bin/agentd`, which lets `systemd` restart onto a new user-built binary even when `/etc/agent-server/agentd.env` still points at the root-owned default path
- after the baseline Docker slice, the next deployment follow-up should add real compose validation on a Docker-installed machine, then separate GPU worker packaging and CI image smoke coverage without collapsing runtime boundaries
- the Codex harness now uses a thin root `plan.md`, an execution-log archive under `docs/codex/`, and shared GitHub templates that require boundary plus validation context
- the Codex harness now also has a canonical live-validation runbook plus artifact naming convention under `docs/codex/live-validation-runbook.md`
- Linux-side archived-output live-smoke helpers now exist alongside the PowerShell scripts
- the Codex harness now also has a Web/H5 manual-evidence scaffolding helper under `scripts/web-h5-manual-capture.sh`
- the next Codex harness follow-up should decide whether browser console export and screenshot naming should stay manual or gain another lightweight helper layer
- the voice-runtime ownership migration is now landed: hidden preview, playout callbacks, and heard-text persistence live behind `internal/voice.SessionOrchestrator`
- external channel follow-up should build the first Feishu adapter on top of `internal/channel.RuntimeBridge` instead of adding another adapter-local orchestration path
- startup config is now split by runtime domain and validated before handler wiring, so future provider additions should extend `Config.Validate()` instead of relying on request-time failures
- reusable protocol-facing validation clients should now live under `clients/`; `tools/` stays reserved for helper scripts and diagnostics

## Current Execution Log

Detailed historical execution history now lives in:

- `docs/codex/execution-log-archive-2026-04.md`

Keep this root ledger focused on active direction, recent execution context, and next-step decisions. When a completed slice stops affecting immediate work, summarize it in the archive instead of extending the root plan.

### 2026-04-15 Realtime Dual-Track And Interruption Slice Complete

- Scope:
  - implement the first integrated `S1` to `S4` slice across shared session, gateway, voice runtime, protocol docs, and schema
  - keep the published v0 websocket contract backward compatible while making server-endpoint, speaking-time preview, and interruption orchestration materially more capable
- Target files:
  - `internal/session/realtime_session.go`
  - `internal/session/types.go`
  - `internal/gateway/realtime_ws.go`
  - `internal/gateway/output_flow.go`
  - `internal/gateway/barge_in_runtime.go`
  - `internal/gateway/xiaozhi_ws.go`
  - `internal/voice/barge_in.go`
  - `internal/voice/session_orchestrator.go`
  - `docs/protocols/realtime-session-v0.md`
  - `docs/protocols/rtos-device-ws-v0.md`
  - `schemas/realtime/session-envelope.schema.json`
- Acceptance for this execution step:
  - shared session snapshots expose separate input/output lanes while preserving compatibility `state`
  - `session.update` documents and emits lane fields plus accepted-turn attribution without breaking older clients
  - server-endpoint accepted turns, speaking-time preview, and interruption arbitration now share one runtime path instead of adapter-local special cases
  - natural playback completion no longer drops staged speaking-time preview or overlap audio
- Validation recorded for this execution step:
  - `go test ./internal/session ./internal/gateway ./internal/voice`
  - `go test ./internal/gateway ./internal/voice`
  - `go test ./internal/session ./internal/gateway`
  - `go test ./internal/voice ./internal/gateway -run 'BargeIn|SessionOrchestrator|Realtime'`
  - `go test -tags integration ./internal/gateway -run 'Realtime|Xiaozhi'`
- Observed outcome:
  - native realtime and `xiaozhi` now share the same dual-track-compatible output completion path
  - accepted turns can now explain whether they came from `audio_commit`, `server_endpoint`, or `text_input`
  - softer interruption policies remain visible to runtime memory and observability without forcing a wire-level protocol change
- Recorded follow-through:
  - updated shared protocol docs and schema for lane fields and accepted-turn attribution
  - expanded gateway/session/voice test coverage for speaking-time preview, preserved overlap input, and interruption-policy boundaries
  - updated `.codex` durable records with the new duplex-state baseline and remaining follow-up gaps

### 2026-04-14 Client Directory Taxonomy Slice Complete

- Scope:
  - align the repository structure so reusable protocol-facing validation clients live under `clients/` instead of `tools/`
  - move the standalone browser realtime client to a stable client-specific path
  - update scripts, docs, and durable repository records to the new location
- Target files:
  - `clients/web-realtime-client/*`
  - `README.md`
  - `scripts/web-h5-manual-capture.sh`
  - `docs/protocols/web-h5-realtime-adaptation.md`
  - `docs/architecture/overview.md`
  - `docs/adr/0027-standalone-reference-clients-live-under-clients.md`
  - `plan.md`
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - the standalone browser realtime client lives under `clients/`
  - root and protocol docs point at the new path
  - repository memory now records the rule that reusable clients belong under `clients/`

Validation recorded for this execution step:

- `node --check clients/web-realtime-client/app.js`
- `node --check clients/web-realtime-client/settings.js`
- `python3 -m py_compile clients/web-realtime-client/serve.py`
- `git diff --check`

Observed outcome:

- the standalone browser realtime debug surface now sits alongside the Python desktop client under `clients/`
- the repository layout more clearly separates reusable endpoint clients from helper tooling

Recorded follow-through:

- moved `tools/web-client` to `clients/web-realtime-client`
- updated browser validation docs and repository durable records
- added ADR `0027-standalone-reference-clients-live-under-clients.md`

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
  - `clients/web-realtime-client/README.md`
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
- updated the runbook, harness docs, Web/H5 protocol guide, root README, standalone client README, and durable repo records
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

### 2026-04-14 Phase-1 Realtime Voice Demo Slice Complete

- Scope:
  - finish the requested `1 -> 2 -> 3` phase-1 voice-demo loop in order:
    - adaptive barge-in threshold
    - incremental TTS speech planner
    - real-sample `server-endpoint-preview` validation and recording
- Target files:
  - `internal/voice/barge_in.go`
  - `internal/voice/speech_planner.go`
  - `internal/gateway/realtime_ws.go`
  - `internal/gateway/xiaozhi_ws.go`
  - `internal/app/config_voice.go`
  - `docs/architecture/runtime-configuration.md`
  - `docs/architecture/voice-demo-realtime-optimization-zh-2026-04-14.md`
  - `.env.example`
  - `deploy/docker/.env.docker.example`
  - `.codex/change-log.md`
  - `.codex/issues-and-resolutions.md`
  - `.codex/project-memory.md`
- Acceptance for this execution step:
  - inbound audio barge-in no longer interrupts on the first frame and still accepts an explicit short interruption commit while speaking
  - shared responders can pre-synthesize stable text clauses behind the existing websocket contract
  - `server-endpoint-preview` succeeds on real speech samples and the websocket remains reusable for a later client `session.end`
- Validation recorded for this execution step:
  - `go test ./internal/voice ./internal/app ./internal/gateway`
  - `go test -tags=integration ./internal/gateway ./internal/voice`
  - `make verify-fast`
  - `python3 -m agent_server_desktop_client.runner --scenario server-endpoint-preview --http-base http://127.0.0.1:8081 --wav artifacts/live-baseline/20260414/samples/input-command-only.wav --output artifacts/live-baseline/20260414/desktop-server-endpoint-preview-command-only-final/report.json --save-rx-dir artifacts/live-baseline/20260414/desktop-server-endpoint-preview-command-only-final`
  - `python3 -m agent_server_desktop_client.runner --scenario server-endpoint-preview --http-base http://127.0.0.1:8081 --wav artifacts/live-baseline/20260414/samples/input-wake-command.wav --output artifacts/live-baseline/20260414/desktop-server-endpoint-preview-wake-command-v1/report.json --save-rx-dir artifacts/live-baseline/20260414/desktop-server-endpoint-preview-wake-command-v1`
- Observed outcome:
  - adaptive barge-in is now shared and staged instead of first-frame hard interrupt
  - the shared speech planner now pre-synthesizes stable clauses while streamed text is still arriving
  - native realtime and `xiaozhi` preview polling now use a non-terminal ticker path instead of read-timeout recovery, so auto-commit no longer breaks the next client close step
  - the real command-only sample passed `server-endpoint-preview` end to end and returned a normal client-driven `session.end`
  - the wake-word-prefixed comparison sample still exposed a local ASR quality caveat (`调管家。`), which is recorded separately for follow-up

### Recent Slices Still Relevant

- `2026-04-14 Local systemd + nginx edge bring-up`
  - added reusable local service-management and edge-exposure assets for this machine:
    - `scripts/install-local-systemd-stack.sh`
    - `scripts/install-local-nginx-proxy.sh`
    - `scripts/run-agentd-local.sh`
    - `scripts/run-funasr-worker-local.sh`
    - `deploy/systemd/agent-server-agentd.service`
    - `deploy/systemd/agent-server-funasr-worker.service`
    - `deploy/nginx/agent-server.conf`
  - current local stack now runs under `systemd` with `agentd` on `0.0.0.0:8080`, the FunASR worker on `127.0.0.1:8091`, and `nginx` exposing `80/443`
  - current `443` exposure uses a self-signed certificate for the machine IP, which removes connection refusal and enables HTTPS/WSS reachability, but trusted-certificate rollout remains a later deployment step if strict client trust is required
  - validation: `systemctl status`, repeated `systemctl restart`, `curl` health checks on `8080/80/443`, and HTTP/HTTPS WebSocket upgrade checks through `nginx`

- `2026-04-14 Local FunASR 2pass Worker And Optional KWS`
  - the Python worker now supports a modular local speech path behind the same shared HTTP contract:
    - optional online preview model
    - separate final-ASR model
    - optional final-path `fsmn-vad`
    - optional final-path punctuation
    - optional worker-side KWS with default `off`
  - default bring-up remains conservative and backward-compatible: `stream_preview_batch`, `energy` endpoint hint, and `KWS` disabled
  - added worker tests, runtime/config docs, Docker env examples, and ADR `0028`
  - validation: worker `py_compile`, targeted Python worker unit tests, and live `/healthz` restart of `agentd + FunASR worker`

- `2026-04-15 Server Endpoint Promoted To Main-Path Candidate`
  - kept the shared runtime boundary unchanged while upgrading discovery and info surfaces to expose a structured `server_endpoint` candidate profile
  - `turn_mode` still stays `client_wakeup_client_commit`, but tooling can now see whether shared preview-driven auto-commit is unsupported, available-but-disabled, or enabled on the current instance
  - browser debug surfaces and desktop runner reports now surface the candidate state instead of relying only on hidden env knowledge
  - architecture/protocol follow-through: `docs/architecture/overview.md`, `docs/architecture/runtime-configuration.md`, `docs/protocols/realtime-session-v0.md`, `docs/protocols/rtos-device-ws-v0.md`, and ADR `0029`
  - validation: targeted Go tests for `/v1/realtime` and `/v1/info`, plus desktop-client protocol/runner unit tests

- `2026-04-14 FunASR Model Selection Research`
  - recorded the current repository truth: the main local FunASR path still uses one `SenseVoiceSmall` ASR worker plus heuristic or external endpoint signals instead of a modular FunASR stack
  - compared official model options across ASR, online streaming, VAD, punctuation, KWS, speaker, emotion, and CosyVoice TTS
  - recommended the next effect-first voice-demo direction as `fsmn-kws` + `fsmn-vad` + true online ASR preview + final-ASR correction rather than continuing to overload one batch-style ASR worker
  - durable note: `docs/architecture/funasr-model-selection-zh-2026-04-14.md`

- `2026-04-14 Local Open-Source GPU TTS Via CosyVoice`
  - added `cosyvoice_http` as a shared `internal/voice` TTS provider targeting the official CosyVoice FastAPI service, keeping local GPU deployment details behind the same runtime boundary as existing ASR/TTS providers
  - added config wiring, runtime validation, integration coverage, Linux bring-up scripts, a layered Docker GPU TTS overlay, architecture follow-through, and ADR `0026`
  - validation: `make test-go`, `make test-go-integration`, `make docker-config`, `make verify-fast`
  - environment note: the new Docker overlay assumes the official CosyVoice image has already been built locally as `cosyvoice:v1.0`; this slice validates compose shape, not a live model-serving run
- `2026-04-14 Test Layout And Command Surface Cleanup`
  - kept Go unit/package tests colocated with source, introduced tagged `integration` and `system` tiers for higher-level gateway tests plus listener-backed voice adapter tests, added top-level `tests/` docs, and exposed the split through `make test-go`, `make test-go-integration`, and `make test-go-system`
  - validation: `make test-go`, `make test-go-integration`, `make test-go-system`, `make verify-fast`
  - environment note: `make test-go-integration` requires local loopback bind permission because the tagged coverage uses `httptest` and websocket listeners
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
