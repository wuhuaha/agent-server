# Issues And Resolutions

## 2026-04-13

### Hidden Preview And Playout Ownership Was Still Split Between Gateway And Voice

- Problem: even after native realtime and `xiaozhi` shared one gateway turn flow, hidden preview polling, auto-commit suggestions, playback lifecycle callbacks, and memory writeback were still split between gateway helpers and `internal/voice`. That was already blocking the next full-duplex step and prevented memory from knowing what the user actually heard after interruption.
- Resolution: added `internal/voice.SessionOrchestrator`, moved preview and playout ownership behind that shared boundary, and wired playback start or progress or interrupt or complete callbacks through it. Runtime memory now persists delivered or heard or truncated state instead of only the generated reply.
- Status: resolved.

### Invalid Runtime Configurations Still Surfaced Too Late

- Problem: app config had already grown across realtime, agent, voice, TTS, and `xiaozhi`, but invalid combinations such as explicit `deepseek_chat` without a key or hidden preview on a non-streaming voice provider still surfaced only during later request handling.
- Resolution: split `internal/app` config by domain, added `Config.Validate()`, and made `NewServer(...)` fail fast before handler wiring. Added regression coverage for the new validation paths.
- Status: resolved.

### Channel Adapters Still Had No Shared Runtime Handoff Path

- Problem: `internal/channel` had only basic adapter contracts. The first real external channel would have been tempted to open-code normalize, runtime handoff, delivery, and retry metadata differently per adapter, or even call providers directly.
- Resolution: added `internal/channel.RuntimeBridge`, extended the channel contract with message or thread or idempotency metadata, and added delivery-status reporting primitives so future adapters stay on normalize -> runtime -> deliver instead of learning provider APIs.
- Status: resolved.

## 2026-04-14

### Standalone Browser Client Was Filed Under `tools` Instead Of `clients`

- Problem: the repository taxonomy had drifted. `clients/python-desktop-client` already established `clients/` as the home for reusable protocol-facing validation endpoints, but the standalone browser realtime client still lived under `tools/`, which made the client-vs-helper boundary inconsistent.
- Resolution: moved the browser client to `clients/web-realtime-client`, updated scripts and docs that scaffold or serve it, and recorded the taxonomy rule in ADR `0027`.
- Status: resolved.

### Websocket Write Paths Could Block Indefinitely Behind One Shared Mutex

- Problem: both native realtime and `xiaozhi` websocket peers wrote JSON and binary frames without a write deadline. A slow or stalled client could therefore block one write forever while holding the shared write mutex, which in turn would stall audio downlink, interruption feedback, and session-close events.
- Resolution: added a shared websocket write helper that applies a per-write deadline and closes the connection on write failure. Both peer implementations now route JSON and binary writes through that helper.
- Status: resolved.

### Recoverable `session_not_started` Audio Error Still Closed The Native Socket

- Problem: native realtime treated binary audio before `session.start` as `Recoverable: true`, but `handleBinary` still returned the underlying `ErrNoActiveSession`, so `ServeHTTP` exited and closed the socket immediately afterward.
- Resolution: keep the error event, but swallow `ErrNoActiveSession` after the recoverable error is emitted so the connection remains usable for a later `session.start`.
- Status: resolved.

### Audio Hot Path Still Did Extra Copies And Playback-Progress Writes

- Problem: gateway audio still paid for repeated session-buffer growth copies, commit-time full-buffer copy, buffered streaming ASR chunk copies, and per-playback-chunk memory-store writes.
- Resolution: introduced owned-frame ingest for gateway paths, flatten turn audio only at commit boundaries, stream buffered ASR through subslices instead of copied chunks, defer playback persistence to stable interrupt or completion boundaries, and stop cloning existing memory-store slices on every upsert.
- Status: resolved for the first hot-path trim slice.

### Imported Root Agent And Skill Packs Still Carried Irrelevant Upstream Noise

- Problem: the root `agents/` and `skills/` directories still contained many upstream ECC references for unrelated language stacks or workflows, such as C++, Java, Kotlin, Rust, Flutter, PostgreSQL, and office-communication roles. That made the repository context heavier and left stale references in kept skill docs.
- Resolution: trimmed those directories to the current `agent-server` stack, kept only Go or Python or voice-agent or deployment or security or harness-relevant references, and cleaned broken references from `skills/README.md` and `skills/prompt-optimizer/SKILL.md`.
- Status: resolved.

### Test Files Needed Better Layering But Not A Full Top-Level Relocation

- Problem: Go `*_test.go` files were spread across package directories, which made the test surface look unstructured. But moving them wholesale into `tests/ut` or `tests/st` would have broken package-local testing ergonomics and pushed internal-only behavior behind wider exported APIs.
- Resolution: kept Go unit/package tests colocated, introduced build-tagged `integration` and `system` tiers for higher-level gateway tests plus listener-backed voice adapter tests, added a documented top-level `tests/` taxonomy for future black-box suites, and exposed the split through `make test-go`, `make test-go-integration`, and `make test-go-system`.
- Environment note: `make test-go-integration` needs local loopback bind permission because the tagged cases use `httptest` and websocket listeners. In restricted sandboxes, validate that tier outside the sandbox.
- Status: resolved.

### Local Open-Source GPU TTS Was Still Missing From The Shared Voice Runtime

- Problem: the project already had a local open-source ASR path through `FunASR`, but TTS remained cloud-only or disabled. Adding a local GPU TTS by wiring browser pages, websocket adapters, or external channels directly to a model server would have broken the voice-runtime boundary.
- Resolution: added `cosyvoice_http` under `internal/voice`, targeting the official CosyVoice FastAPI service as a local GPU-side dependency. App bootstrap now selects that provider through the shared TTS config, audio is normalized before it reaches adapters, and the repository also carries Linux bring-up plus layered Docker overlay guidance for the external GPU service.
- Status: resolved.

## 2026-03-25

### Writing to E Drive from the Current Workspace

- Problem: the active writable workspace was under `C:\Users\wangt\Documents\New project`, while the new project had to live at `E:\agent-server`.
- Resolution: created `E:\agent-server` and attached it into the workspace via a junction at `C:\Users\wangt\Documents\New project\agent-server`.
- Status: resolved.

### Local Go Toolchain Missing

- Problem: `go` and `gofmt` are not available on the current machine PATH, so local compile-time verification could not run during initialization.
- Resolution: Go was installed at `C:\Program Files\Go\bin`. The current Codex terminal PATH still does not include it, so verification currently uses the absolute tool path or a session-local PATH prefix.
- Status: resolved with environment caveat.

### Go Proxy Reachability

- Problem: `go mod tidy` could not reach `https://proxy.golang.org` from the current network environment, which blocked dependency resolution for `github.com/gorilla/websocket`.
- Resolution: persisted `GOPROXY=https://goproxy.cn,direct` and `GOSUMDB=sum.golang.google.cn` through `go env -w`, then revalidated repository-wide `go test ./...` against a fresh temporary module cache.
- Status: resolved on this machine.

### FunASR GPU Compatibility On RTX 5060

- Problem: the existing `xiaozhi-esp32-server` conda environment uses `torch 2.2.2+cu121`, and `SenseVoiceSmall` failed on the local RTX 5060 with `CUDA error: no kernel image is available for execution on the device`.
- Resolution: upgraded the environment to CUDA-enabled `torch 2.11.0+cu128` and `torchaudio 2.11.0+cu128`, then revalidated low-level CUDA init, direct `torch` tensor placement, direct `FunASR AutoModel` inference, and the HTTP worker path on `device=cuda:0`.
- Status: resolved on this machine.

### CUDA Driver Probes Failed Inside The Default Codex Sandbox

- Problem: the first post-upgrade `torch.cuda` probe still failed with `cudaGetDeviceCount error 304`, but the failure was coming from the sandboxed execution context rather than the machine GPU stack itself.
- Resolution: reran the same low-level `cuInit(0)` and `torch.cuda` checks outside the sandbox, confirmed successful CUDA initialization plus real GPU tensor execution, and used the same unrestricted context for GPU FunASR validation.
- Status: resolved as a tooling caveat.

### Git Safe Directory Warning On E Drive

- Problem: earlier sessions reported `E:\agent-server` as a dubious ownership repository, which blocked normal `git` inspection.
- Resolution: rechecked on 2026-03-30 and `git status` now runs cleanly from `E:\agent-server`, so the repository is no longer blocked on `safe.directory`.
- Status: resolved.

## 2026-03-30

### Pion Opus Decoder Output Sizing

- Problem: the first Go-side `opus` normalization attempt failed with the decoder error `out isn't large enough`, which blocked `opus` uplink support.
- Resolution: sized the decode buffer for the library's internally upsampled output, then normalized the decoded samples to `pcm16le/16000/mono` and added regression coverage with `testdata/opus-tiny.ogg`.
- Status: resolved.

### Windows Sandbox Temp Directory For Python Audio Tests

- Problem: `clients/python-desktop-client/tests/test_audio.py` can still fail in this Windows sandbox because `tempfile.TemporaryDirectory()` does not always get a writable location outside the workspace.
- Resolution: not changed in this task; repository-level Go verification remains the authoritative check here, and the Python audio test caveat stays environment-specific.
- Status: open environment caveat.

### Transport-Owned Bootstrap Policy

- Problem: the realtime websocket handler still owned bootstrap end-of-dialog policy, which would have pushed channel adapters toward transport-specific orchestration instead of one shared agent execution boundary.
- Resolution: introduced `internal/agent` with a `TurnExecutor` contract and moved bootstrap close directives behind responder/runtime output so transports stay adapters.
- Status: resolved.

### Runtime Output Had No Structured Delta Contract

- Problem: the new `Agent Runtime Core` could return text, but the realtime transport still effectively treated `response.chunk` as a single plain-text field, leaving no shared wire contract for tool progress or other ordered runtime deltas.
- Resolution: added shared delta kinds across `internal/agent` and `internal/voice`, taught the realtime gateway to emit ordered `response.chunk` events with `delta_type` plus tool metadata, and documented the contract in both protocol docs and schema.
- Status: resolved.

### Memory And Tool Hooks Had No Runtime-Owned Injection Point

- Problem: after adding the `TurnExecutor`, the service still had no canonical place to inject memory backends or tool providers without leaking that orchestration into transports or future channel adapters.
- Resolution: added `MemoryStore`, `ToolRegistry`, and `ToolInvoker` contracts under `internal/agent`, wired default no-op implementations from app bootstrap, and kept the first hook orchestration inside the bootstrap executor.
- Status: resolved.

### Materialized Delta Lists Blocked True Streaming

- Problem: the first runtime delta contract still required collecting all deltas into `TurnOutput.Deltas` before the gateway could emit them, which blocked early tool progress and future model streaming.
- Resolution: added sink-based `StreamingTurnExecutor` and `StreamingResponder` interfaces, then wired the realtime gateway to forward streamed deltas immediately as `response.chunk` events.
- Status: resolved.

## 2026-03-31

### Runtime Hook Boundary Was Real But Still Functionally Empty

- Problem: the runtime-owned memory and tool interfaces existed, but app bootstrap still defaulted to no-op providers, so the boundary could not yet prove real recall or tool behavior without transport-specific hacks.
- Resolution: added an in-process recent-turn memory store plus a builtin tool backend, made them the default app-wired providers, and surfaced recall through runtime commands instead of transport-specific code.
- Status: resolved.

### Existing `xiaozhi` Firmware Could Not Reuse The New Server Directly

- Problem: the native `rtos-ws-v0` transport was intentionally cleaner than `xiaozhi-esp32-server`, but current firmware still expected `/xiaozhi/ota/`, `/xiaozhi/v1/`, `hello` / `listen` / `abort`, and legacy binary frame wrappers.
- Resolution: added a compatibility adapter that mounts the legacy paths, accepts `hello` without strict `audio_params`, bridges firmware binary protocol versions `1` / `2` / `3`, and maps spoken replies back into `tts.start` / `tts.sentence_start` / `tts.stop` plus wrapped binary audio.
- Status: resolved for first voice bring-up; transcript echo for audio ASR turns and `llm.emotion` mapping remain follow-up polish items.

## 2026-04-02

### SenseVoiceSmall Local Load Failed With `trust_remote_code=true`

- Problem: after the local `SenseVoiceSmall` model cache completed, the FunASR worker still failed at first model load when `AGENT_SERVER_FUNASR_TRUST_REMOTE_CODE=true`, raising `No module named 'model'`.
- Resolution: verified that the cached model bundle loads successfully with `trust_remote_code=false`, then changed the worker default, the PowerShell start script, and the runtime docs to keep the local reference path on that setting.
- Status: resolved.

## 2026-04-03

### Xiaozhi Compatibility Opus Downlink Hung Before First Packet

- Problem: live `xiaozhi` compatibility smoke reached `tts stream started`, but the device side never received `tts.start` or the first binary audio frame because the Opus encoder path blocked before producing initial Ogg headers.
- Resolution: started the PCM feed goroutine before waiting on `oggreader.NewWith(...)`, added a regression test for first-packet production, then revalidated live `hello`, `listen.detect`, and protocol-version `3` audio-turn flows against `127.0.0.1:18080`.
- Status: resolved.

## 2026-04-03

### Validated Cloud Voice Providers Lived Only In External Debug Tools

- Problem: the verified iFlytek RTASR, iFlytek websocket TTS, and Volcengine SSE TTS flows existed only in external debug tooling, so the main `agent-server` runtime could not select them as first-class ASR/TTS backends.
- Resolution: migrated those providers into `internal/voice`, kept them behind the shared `Transcriber` and `StreamingSynthesizer` interfaces, wired selection through app config, and confirmed the repository still passes `go test ./...`.
- Status: resolved.

## 2026-04-04

### The Agent Runtime Had No Real LLM Provider Boundary

- Problem: `internal/agent` had a transport-neutral turn executor, memory hooks, tool hooks, and streamed deltas, but there was still no real cloud LLM path. Wiring DeepSeek directly into gateways or the voice runtime would have broken the runtime boundary before the first channel adapter landed.
- Resolution: added a shared `ChatModel` contract plus `LLMTurnExecutor` under `internal/agent`, implemented `deepseek_chat` against DeepSeek's OpenAI-compatible chat completions API, and selected it only from app bootstrap config.
- Status: resolved.

### The First Cloud LLM Path Still Used A Generic Assistant Prompt

- Problem: after wiring DeepSeek into `internal/agent`, the runtime still defaulted to a generic assistant prompt that did not match the household control-screen product role or the debug-stage requirement to confirm control naturally without exposing simulation details.
- Resolution: replaced the default prompt with a built-in Chinese home-control assistant template, added assistant-name runtime config, and made custom prompt overrides support `{{assistant_name}}` substitution so the persona remains configurable without moving prompt logic into transports.
- Status: resolved.

### Realtime Turn Buffer Copies Accumulated Audio On Every Frame

- Problem: the current `RealtimeSession` snapshot path still clones the full accumulated `TurnAudio` buffer whenever audio frames are ingested, which will inflate copy cost and memory pressure as turns get longer.
- Resolution: `RealtimeSession` now keeps accumulated turn audio in a private buffer, leaves `Snapshot` as metadata-only state, and exports one copied `AudioPCM` buffer only at `CommitTurn`. The native realtime gateway and `xiaozhi` compatibility gateway were both updated to consume the committed-turn boundary instead of reading turn audio from snapshots. Session regression coverage and a benchmark were also added.
- Status: resolved.

### Published `turn_mode` Suggests Server VAD But Runtime Still Depends On Client Commit

- Problem: discovery and config defaults still publish `client_wakeup_server_vad`, but the current realtime turn flow is still driven by explicit client-side `audio.in.commit` or text input rather than a true server-side VAD pipeline.
- Resolution: aligned the public contract to the current implementation by changing the default advertised mode to `client_wakeup_client_commit`, removing the unused public `armed` state, and updating discovery notes, protocol docs, RTOS adaptation docs, config defaults, and the realtime envelope schema. The decision is recorded in `docs/adr/0009-advertise-commit-driven-turn-semantics-until-server-vad-exists.md`.
- Status: resolved.

## 2026-04-05

### Silence On The Local FunASR Reference Path Still Produces False Positives

- Problem: during the 2026-04-07 local loopback validation, the full runner used a silence-based audio turn against `funasr_http`, but the ASR result still came back as `그.` instead of an empty utterance. That means the current local reference stack still lacks a robust silence-rejection gate before or after ASR.
- Resolution: not fixed yet. The live run was otherwise successful, and the issue has been recorded for follow-up tuning around silence detection, VAD, or post-ASR empty-utterance filtering.
- Status: open.

### Standalone Static Web Tool Could Not Assume Same-Origin Discovery

- Problem: the built-in `/debug/realtime-h5/` page can call `GET /v1/realtime` because it is served by the same Go process, but a separate static tool under `tools/` would often run on another origin and therefore could not safely depend on browser-side discovery fetches.
- Resolution: the standalone client under `clients/web-realtime-client` now treats manual realtime-profile entry as the primary path and supports pasted discovery JSON as an optional sync aid, while still connecting to the same native `/v1/realtime/ws` contract.
- Status: resolved.

### Web Or H5 Direct Access Risked Becoming A Second Protocol Surface

- Problem: the repository already had a native realtime websocket contract and a `xiaozhi` compatibility adapter. Adding browser access by inventing a third browser-only websocket dialect would have duplicated transport behavior and weakened the shared session core boundary.
- Resolution: added a built-in debug page at `/debug/realtime-h5/`, but kept it on the existing `GET /v1/realtime` plus `/v1/realtime/ws` contract. Browser-specific microphone capture and `pcm16le` playback adaptation stay inside the page instead of creating a new gateway protocol.
- Status: resolved for the first browser slice; raw browser `opus` uplink and richer multimodal browser input remain follow-up work.

### The First LLM Runtime Path Was Still Single-Shot And Could Not Run Real Tools

- Problem: the first `deepseek_chat` integration stayed behind the correct `internal/agent` boundary, but `LLMTurnExecutor` still reduced it to one blocking completion call. That meant no provider-streamed text deltas, no model-proposed tool calls, and no shared runtime loop for tool execution or reinjection.
- Resolution: added `StreamingChatModel`, explicit chat-message and tool-definition contracts, a bounded model-tool loop inside `LLMTurnExecutor`, and a DeepSeek adapter that now parses both non-stream and streamed tool-call responses. Tool-name sanitization for provider requests now also stays inside `internal/agent` instead of renaming the runtime tool catalog globally.
- Status: resolved.

### Runtime Memory Was Still Summary-Only And Too Device-Centric

- Problem: the first in-process memory backend proved the `MemoryStore` boundary existed, but it still returned only a summary string plus facts and primarily keyed recall by device. That limited multi-turn continuity and made shared-device recall too dependent on `device_id`.
- Resolution: extended `MemoryContext` with a bounded `RecentMessages` window, taught the default in-memory backend to store turns under `session`, `user`, `device`, `room`, and `household` scopes, and injected recent-message history into `LLMTurnExecutor` ahead of the current user turn. Metadata-derived scope hints now stay inside `internal/agent`.
- Status: resolved.

### The Runtime Lost Most ASR Semantics Beyond Final Transcript Text

- Problem: the ASR path only forwarded final transcript text into `internal/agent`, which discarded useful signals such as detected language, endpoint reason, speaker, audio events, and partial hypotheses. Passing provider-native ASR payloads through transports would have broken the runtime boundary.
- Resolution: extended the shared transcription result with optional structured fields, normalized them into `speech.*` metadata inside `internal/voice`, and injected that metadata into the shared agent turn input without changing the websocket protocols.
- Status: resolved.

### Common Household Control Was Still Too Dependent On Open-Ended Generation

- Problem: even after adding richer runtime context, obvious household commands such as lights, curtains, and air conditioning still depended on the open-ended model path, which kept simple home-control behavior less predictable than it should be.
- Resolution: added a first bounded deterministic household-routing slice inside `internal/agent`, using room hints from text or metadata and keeping sensitive domains on a conservative clarification path instead of pushing control parsing into transports.
- Status: resolved for the first bounded slice.

### Xiaozhi Audio Turns Still Had No Transcript Echo

- Problem: the `xiaozhi` compatibility adapter echoed `stt` for `listen.detect` text turns, but audio turns still had no transcript echo, which made RTOS audio-turn debugging and UI feedback weaker than the text path.
- Resolution: extended the shared voice response contract with normalized input text and had the `xiaozhi` adapter emit audio-turn `stt` from that transport-neutral responder output instead of parsing ASR provider results itself.
- Status: resolved for the first compatibility slice.

## 2026-04-07

### Persona And Execution-Mode Policy Were Still Hidden Inside One Monolithic Prompt Builder

- Problem: even after moving household control into runtime skills, the shared runtime still assembled persona, output contract, and execution-mode policy through one hardcoded prompt builder. That kept policy boundaries implicit and made it harder to reason about what belonged to the core runtime versus what should remain pluggable.
- Resolution: introduced `PromptSectionProvider` and moved persona, output contract, and execution-mode policy into explicit runtime-owned prompt sections. Runtime skills still append their own prompt fragments afterward instead of replacing core policy.
- Status: resolved.

### Household-Control Product Rules Were Leaking Into The Core Executor Path

- Problem: the repository had started to hardwire household-control behavior directly inside `BootstrapTurnExecutor`, `LLMTurnExecutor`, and the default assistant prompt. That made the core runtime less AI-native, bypassed the normal model-tool loop for smart-home requests, and mixed product-vertical rules into a boundary that should stay generic.
- Resolution: removed the deterministic household short-circuit from the executor path, added a runtime-skill prompt contribution interface, and moved the current household semantics into a built-in runtime skill `household_control` with tool `home.control.simulate`.
- Status: resolved.

### TTS Spoke Bootstrap Echo Text Instead Of LLM Output

- Problem: live turns could synthesize speech for `agent-server received text input: ...` because the runtime was still on the bootstrap executor whenever `AGENT_SERVER_AGENT_LLM_PROVIDER` was unset, even if a DeepSeek key had already been configured elsewhere in the environment. From the client side this looked like a TTS or prompt failure, but the actual text source was the bootstrap placeholder reply.
- Resolution: changed the default LLM provider behavior to `auto`, which now prefers `deepseek_chat` when a DeepSeek key is present and otherwise stays on bootstrap. Discovery and info endpoints now also expose the effective `llm_provider`, and browser settings pages warn explicitly when they detect `bootstrap`.
- Status: resolved.

### Browser Debug Pages Still Felt Like Dense Test Forms Instead Of A Voice Console

- Problem: even after splitting browser bring-up into separate settings and debug pages, the debug surface still presented connect buttons, text input, microphone controls, TTS playback, and protocol diagnostics as mostly flat neighboring blocks. That made the page feel more like a raw test harness than a voice console and did not reflect the clearer stage-driven interaction style seen in `py-xiaozhi`.
- Resolution: reorganized both built-in and standalone debug pages around a primary stage card with a visible phase rail (`idle / connect / listen / speak`), a current-phase headline, and a latest-event summary. Transcript, playback diagnostics, and raw protocol tools remain present, but are visually secondary.
- Status: resolved.

### Browser Debug Pages Could Render But Stay Completely Non-Interactive On Older Browsers

- Problem: the built-in `/debug/realtime-h5/` page and the standalone `clients/web-realtime-client` pages were served as raw browser scripts without any build step, but they still used `type="module"`, optional chaining, nullish coalescing, and `String.prototype.replaceAll`. On older browsers or embedded WebViews, the scripts could fail during parse or be skipped entirely, leaving a page that looked loaded but did not react to clicks.
- Resolution: switched all browser pages to classic deferred scripts and removed those unsupported syntax features from the shipped frontend code while keeping the same runtime behavior.
- Status: resolved.

### Historical Collaboration Noise Obscured The Real Repository State

- Problem: the worktree had accumulated long-lived dirty changes from prior sessions, especially `.claude/` files, `.codex/skills/*`, and `.codex/mimo-*`. Those diffs were only CRLF line-ending drift or other no-op formatting changes, which made `git status` and review output much harder to trust.
- Resolution: reverted the no-op collaboration and vendor-note diffs, then normalized the remaining changed text files back to LF so the kept worktree now reflects semantic product work only.
- Status: resolved.

### Websocket Timeout Retry Panic In Gateway Read Loops

- Problem: live browser-side usage hit `http: panic serving ... repeated read on failed websocket connection` because the native realtime handler continued into another `ReadMessage()` after a timeout-triggered read failure. The `xiaozhi` compatibility websocket loop used the same recoverable-timeout pattern and carried the same risk.
- Resolution: changed both gateway handlers so any timeout-triggered websocket read failure becomes terminal for that connection. The native realtime path still emits `session.end` when the timeout maps to an idle or max-duration close, the `xiaozhi` path still emits compat `tts stop`, and both handlers now return instead of re-entering `ReadMessage()`. Added regression tests that capture server logs and assert timeout-driven teardown no longer logs `panic serving` or `repeated read on failed websocket connection`.
- Status: resolved.

### Browser Bring-Up Had Too Much Configuration Mixed Into The Live Debug Page

- Problem: after the first browser bring-up slice, both the standalone client and the built-in `/debug/realtime-h5/` page still mixed endpoint setup, discovery sync, device preset, session control, TTS playback, and protocol logs into one dense surface. That made the page harder to scan during real debugging and made the intended bring-up flow less obvious.
- Resolution: split both browser paths into dedicated `settings` and `debug` pages. Settings now owns endpoint/audio profile and device preset work, while the debug page focuses on websocket turns, TTS playback, and logs.
- Status: resolved.

### MiMo Streaming TTS Could Close Successfully With Zero Audio On Normal Turns

- Problem: live validation on 2026-04-07 showed `mimo_v2_tts` frequently returning a stream that completed successfully with `0` chunks and `0` bytes for normal text and audio turns. The websocket path therefore entered `speaking` and returned to `active` without ever delivering binary audio to browser or desktop debug clients.
- Resolution: changed the shared synthesis path to prefetch the first non-empty streaming chunk before committing to stream mode, and to fall back immediately to buffered synthesis when the provider closes the stream without audio. After restarting `agentd`, the latest live runner report at `/tmp/agent-server-web-tts-runner.json` showed `response_with_audio_ratio = 1.0`.
- Status: resolved.

## 2026-04-08

### Realtime Turns Had No Stable Cross-Layer Trace Identifiers

- Problem: the shared turn path still lacked stable per-turn identifiers across gateway, voice responder, agent runtime input, and client-visible response events. That made the first migration stage harder to measure and made it difficult to correlate one user turn across logs, runner reports, and streamed response events.
- Resolution: added gateway-generated `turn_id` and `trace_id` for each committed turn, threaded them through `voice.TurnRequest` and `agent.TurnInput`, exposed optional `turn_id` and `trace_id` on native realtime `response.start`, and exposed optional `turn_id` on server-emitted turn-state `session.update` events. The desktop runner now captures those IDs and records additional phase timings.
- Status: resolved for the first `F0` traceability slice.

### Turn Traces Still Stopped At Identifiers Instead Of Becoming Useful Observability

- Problem: after the first `F0` slice, the repository had `turn_id` and `trace_id`, but server logs still could not explain where a turn spent time across gateway phases, runtime execution, ASR, TTS, and playback. The desktop runner also lacked run-level metadata and replay-friendly saved artifacts, so archived reports were still awkward to compare.
- Resolution: added structured gateway turn-phase logs, wrapped the shared runtime and voice providers with correlated logging decorators, propagated `turn_id` and `trace_id` into ASR/TTS request objects, and upgraded the desktop runner report with `generated_at`, `run_id`, `llm_provider`, per-scenario `issues`, `artifact_dir`, and saved replay artifacts.
- Status: resolved for the second `F0` traceability slice.

### Live Regression Validation Depends On A Running Local Agentd

- Problem: during the `F0-3` scripted-regression expansion, the local `http://127.0.0.1:8080/v1/realtime` endpoint was not available, so the new `regression` suite could not be exercised against a live server in the same turn.
- Resolution: restored the local stack with the FunASR worker on `127.0.0.1:8091` plus `agentd` on `127.0.0.1:8080`, then ran the live desktop `regression` suite and the live RTOS mock session successfully. Canonical artifacts are now archived under `artifacts/live-baseline/20260409/desktop-regression` and `artifacts/live-baseline/20260409/rtos-mock`.
- Status: resolved.

### Live Service Probes Required The Same Escalated Network Context As The Listeners

- Problem: after starting the local worker and `agentd` outside the default sandbox so they could bind `8091` and `8080`, sandboxed `curl` probes to `127.0.0.1` still failed because the listeners were not reachable from the default sandbox network context.
- Resolution: ran the live bring-up and health probes in the same escalated context as the listeners. Repository code verification remains safe to run inside the default sandbox, but live network validation on this machine should assume the worker, server, and probes all need the same unrestricted context.
- Status: resolved as a tooling caveat.

### RTOS Mock Reports Were Not Comparable To Desktop Baseline Artifacts

- Problem: the RTOS mock still emitted a minimal one-off JSON payload (`session_id`, `ok`, `interrupt_sent`, `close_reason`) and a single optional WAV file path, which made device-style bring-up artifacts hard to compare against the richer desktop runner baseline reports introduced in the `F0` migration slices.
- Resolution: upgraded `RTOSMockClient` to emit run metadata, discovery metadata, identifier capture, checks/issues, and replay-friendly artifact references; added `--save-rx-dir` to archive events, response text, run summary, and received audio while preserving the existing `--save-rx` quick WAV path.
- Status: resolved.

## 2026-04-10

### Local Full-Duplex Path Still Lacks A Shared Voice-Orchestration Layer

- Problem: the repository can already stream text, stream audio, and interrupt speaking output, but it still does not have a complete shared local voice-orchestration layer for server-side endpointing, incremental TTS scheduling, interruption arbitration, and heard-text reconciliation. Before the first roadmap slice landed, the local reference ASR path was also still effectively batch-driven.
- Resolution: documented the gap in `docs/architecture/full-duplex-voice-assessment-zh-2026-04-10.md` and recorded the local/open-source-first execution path in `docs/architecture/local-open-source-full-duplex-roadmap-zh-2026-04-10.md` plus `docs/adr/0021-local-open-source-first-full-duplex-roadmap-prioritizes-voice-orchestration.md`. Then implemented the first `L0/L1` slices: added a shared `StreamingTranscriber` boundary in `internal/voice`, kept non-streaming providers on a buffered compatibility adapter, added the local FunASR worker `/v1/asr/stream/*` lifecycle, upgraded `HTTPTranscriber` to use it as a real streaming session, switched `funasr_http` to that real path, and expanded runner metrics for partial-latency and barge-in quality measurement.
- Status: partially resolved.

### Server-Side Endpointing Needed To Arrive Without Breaking The Advertised Turn Contract

- Problem: the roadmap called for `L2` server-side endpointing, but publicly advertised turn-taking still had to remain `client_wakeup_client_commit`. Implementing endpointing directly inside websocket adapters or immediately widening discovery would have broken the current architecture boundary and forced clients to reason about an unstable interim mode.
- Resolution: added a shared internal input-preview boundary in `internal/voice` (`InputPreviewer`, `InputPreviewSession`, `InputPreview`) plus a default silence-based turn detector, then made `ASRResponder` expose preview sessions when streaming ASR is available. Native realtime and `xiaozhi` websocket handlers now consume that shared preview capability behind the hidden `AGENT_SERVER_VOICE_SERVER_ENDPOINT_ENABLED` switch and can auto-commit a turn after local silence, while the public discovery mode and wire schemas remain unchanged.
- Status: resolved for the first internal preview slice; broader endpoint policy and public rollout remain open.

### Hidden Preview Mode Needed Explicit Tuning And A Safe Validation Entry Point

- Problem: after the first internal `L2` slice landed, the hidden server-endpoint path still relied on implicit detector defaults and there was no dedicated scripted validation scenario for “send audio but do not send commit”. That made tuning harder and increased the chance that people would infer a public protocol change from ad hoc tests.
- Resolution: exposed `AGENT_SERVER_VOICE_SERVER_ENDPOINT_MIN_AUDIO_MS` and `AGENT_SERVER_VOICE_SERVER_ENDPOINT_SILENCE_MS` as shared voice-runtime config, wired them through both `funasr_http` and `iflytek_rtasr` responder bootstrap paths, and added an opt-in desktop runner scenario `server-endpoint-preview` for explicit hidden-mode validation. The default public discovery mode and default runner suites remain unchanged.
- Status: resolved for the current hidden-preview stage.

### Hidden Endpoint Preview Still Risked False Turn Ends On Short Pauses

- Problem: after the first hidden preview slices, auto-commit still depended only on “enough audio + silence window + at least one partial”. That meant a partial like `帮我把` or `还有` could still be cut into a turn if the speaker paused briefly, even though the phrase was obviously unfinished.
- Resolution: kept the fix inside `internal/voice` by extending the shared turn detector with a conservative lexical false-endpoint guard plus a configurable extra hold window. The hidden preview mode now delays auto-commit for obviously unfinished partials, while still falling back to a longer timeout so turns are not held forever.
- Status: resolved for the current hidden-preview stage.

### Hidden Endpoint Preview Still Ignored Provider Endpoint Signals

- Problem: after the lexical-guard slice, hidden endpoint preview was still driven almost entirely by local silence windows plus shared lexical heuristics. The local streaming worker could produce useful preview-side evidence, but that evidence still stopped at the worker boundary and never affected shared turn detection.
- Resolution: added a lightweight worker-side preview endpoint hint derived from tail-audio energy, propagated it through `HTTPTranscriber` partial deltas, and taught the shared turn detector to use that hint for a shorter endpoint wait on lexically complete partials. The hint path still remains internal to `internal/voice`, and incomplete partials remain on the conservative hold path.
- Status: resolved for the current hidden-preview stage.

### Hidden Endpoint Preview Still Relied On A Weak Tail-Energy Hint Only

- Problem: after the first provider-hint slice landed, the worker still had only one lightweight acoustic hint source: tail mean-absolute energy. That was useful as a minimal signal, but it was still weaker than a proper local/open-source VAD path and risked noisy false hints or missed endpoints on more varied speech.
- Resolution: kept the enhancement inside the Python worker boundary by adding an optional `Silero VAD` preview-hint path behind `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_PROVIDER`. The worker can now emit `preview_silero_vad_silence` when the local VAD runtime is available, while unsupported inputs or missing dependencies still fall back to the existing `preview_tail_silence` path. The shared Go voice runtime remains generic over endpoint hints, and the public realtime contract still does not change.
- Status: resolved for the current hidden-preview stage.

### Linux Bring-Up Still Relied On Historical Machine State Instead Of A Real Install Entry Point

- Problem: the repository had PowerShell bring-up scripts and scattered docs, but no single Linux install entrypoint that encoded the real dependency layers already assumed by this machine. The local FunASR worker especially depended on undeclared conda-env state such as `funasr`, `modelscope`, and later `onnxruntime` / `silero-vad`.
- Resolution: added `scripts/install-linux-stack.sh` as the repository-local Linux install entrypoint and updated docs to point at it. The worker package now declares `runtime` and `stream-vad` extras so the install path matches the actual runtime shape instead of hidden machine history.
- Status: resolved.

### Local Editable Install Flow Broke On Real Packaging Constraints

- Problem: once the Linux install flow was exercised for real, three packaging issues surfaced: upgrading `setuptools` too far conflicted with `torch 2.11.0(+cu128)`, local editable installs under hatchling still needed `hatchling` available without network, and `--no-build-isolation` also required `editables`.
- Resolution: hardened the install script to keep `setuptools<82`, preinstall `hatchling` and `editables`, and then install repository-local packages with `--no-build-isolation`. After that change, the full install flow completed successfully and the worker env still loaded `torch`, `onnxruntime`, and `silero_vad`.
- Status: resolved.

### Docker CLI Is Not Installed In This Workspace

- Problem: the current machine context for `/root/agent-server` does not have a `docker` executable, so this turn could not run `docker compose config`, image builds, or live container smoke validation for the new Docker deployment slice.
- Resolution: completed static validation instead by parsing the compose YAML files with `python3` + `yaml.safe_load`, checking the Dockerfiles for expected image and entrypoint directives, and documenting the runtime assumptions in `README.md` plus `docs/architecture/runtime-configuration.md`.
- Status: open environment caveat. Real compose validation should run after Docker is installed on the target machine.

### Docker Daemon Could Not Reach Registries Reliably On This WSL2 Machine

- Problem: after Docker was installed on the current WSL2 Ubuntu machine, the daemon still timed out or returned `EOF` while resolving `docker.io`, `gcr.io`, and `auth.docker.io` directly during image builds.
- Resolution: confirmed that the existing local proxy path (`127.0.0.1:7890`) could reach those registry endpoints, configured a systemd drop-in so the Docker daemon uses that proxy on this machine, and retried validation from the same unrestricted context.
- Status: resolved on this machine as an environment fix.

### `gcr.io/distroless` Was A Fragile Runtime Base For `agentd` In Real Docker Validation

- Problem: the first formal `agentd` image used `gcr.io/distroless/static-debian12:nonroot`, but real image resolution repeatedly failed through the current machine's constrained network path even after the Docker daemon proxy was configured.
- Resolution: changed both `deploy/docker/Dockerfile` and `deploy/docker/agentd.Dockerfile` to use `scratch`, copy the CA bundle from the Go build stage, and run as non-root `65532:65532`. That kept the image minimal while removing the unstable `gcr.io` dependency.
- Status: resolved.

### Docker Build Reintroduced The Historical Go Proxy Failure

- Problem: once `agentd` image resolution was fixed, the Docker build still failed at `go mod download` because the container defaulted back to `https://proxy.golang.org`, which had already been proven unreliable on this machine.
- Resolution: added Docker build defaults for `GOPROXY=https://goproxy.cn,direct` and `GOSUMDB=sum.golang.google.cn`, while still allowing overrides through build args.
- Status: resolved.

### CPU FunASR Worker Image Carried An Unneeded Apt Layer

- Problem: the CPU worker image installed `ca-certificates` and `libsndfile1` through `apt-get`, but the current worker implementation only uses the standard-library `wave` path and declared Python wheels. In practice that apt step became one of the noisiest failure points in real Docker validation.
- Resolution: removed the apt layer from `deploy/docker/funasr-worker.cpu.Dockerfile` and kept the image on `python:3.11-slim-bookworm`, which already provides the needed base runtime for the current worker code path.
- Status: resolved for the current worker implementation.

### CPU FunASR Worker Build Still Depends On A Stable PyTorch CDN Path

- Problem: after base-image and apt issues were resolved, the CPU worker image still failed intermittently while downloading the large `torch 2.11.0+cpu` wheel from `download-r2.pytorch.org`, ending with `incomplete-download` after repeated resume attempts.
- Resolution: added proxy-environment passthrough to the worker Dockerfile and compose build args, plus a higher `pip` default timeout. That improved the build path and got validation through bootstrap pip setup plus into the PyTorch stage, but the final wheel download still depends on external network quality on this machine.
- Status: open environment caveat.

### Top-Level Agent Instructions Had Become Too Heavy For Reliable Codex Use

- Problem: the repository-level `AGENTS.md` had accumulated a large imported baseline plus project-specific rules, which made the top-level instruction surface noisy and reduced the chance that coding agents would quickly load only the repo-critical guardrails before starting work.
- Resolution: rewrote `AGENTS.md` into a short high-signal file containing the repo mission, priority order, guardrails, required follow-through, standard command surface, and context map. Moved deeper Codex execution guidance into `docs/codex/harness-workflow.md`.
- Status: resolved.

### Validation Entry Points Were Fragmented Across Ad Hoc Commands

- Problem: common repository checks were spread across README snippets and one-off shell commands, so there was no stable command surface that Codex sessions and CI could reuse consistently for environment inspection, fast validation, or Docker compose config checks.
- Resolution: expanded `Makefile` with `doctor`, `test-go`, `test-py`, `docker-config`, `verify-fast`, and `run`; added `scripts/codex-doctor.sh`, `scripts/docker-config-check.sh`, and `scripts/verify-fast.sh`; and added a fast GitHub Actions workflow that runs the same Go, Python, and Docker-config checks. Local validation passed on this machine.
- Status: resolved.

### Root Planning Context Had Become Too Large For Fast Agent Loading

- Problem: after many implementation slices, `plan.md` had grown into a 1400+ line mixed active-plan and historical ledger. That made the root planning context heavier than necessary for everyday Codex work, even after `AGENTS.md` had already been shortened.
- Resolution: moved older completed slice history into `docs/codex/execution-log-archive-2026-04.md` and rewrote `plan.md` so it keeps only active direction, the recent execution window, and next-step notes.
- Status: resolved.

### Repository Collaboration Still Lacked Structured Issue And PR Scaffolding

- Problem: even after the command surface and harness docs were standardized, future work proposals and PRs still had no shared template requiring architecture boundaries, protocol or ADR impact, or validation against the common command surface.
- Resolution: added GitHub issue templates for bugs and architecture or feature tasks, plus a PR template that asks for boundary impact, docs follow-through, secret checks, and validation with the shared `make` entrypoints.
- Status: resolved.

### Live Validation Evidence Still Used Inconsistent Artifact Roots And Historical One-Off Names

- Problem: the repository had working live validation tools, but the archived outputs still mixed historical locations such as `.codex/artifacts/*`, ad hoc `report.json` paths, and handwritten references in docs. That made it harder to compare runs or tell which artifact directories were baseline-quality versus quick smoke output.
- Resolution: added `docs/codex/live-validation-runbook.md` and standardized two artifact roots: `artifacts/live-smoke/YYYYMMDD/<profile>/` for quick local reruns and `artifacts/live-baseline/YYYYMMDD/<profile>/` for comparison-worthy archived runs. Also aligned the Windows smoke scripts with repository-local paths plus canonical top-level names such as `input.wav` and `report.json`.
- Status: resolved.

### Linux Still Lacked A One-Command Archived-Output Live-Smoke Path

- Problem: after the runbook and artifact naming conventions were standardized, Linux still did not have repository-local helper scripts that started the local worker and `agentd`, ran the desktop or RTOS smoke flow, and archived outputs under the canonical live-smoke roots. Windows had that path already through PowerShell scripts.
- Resolution: added `scripts/smoke-funasr.sh` and `scripts/smoke-rtos-mock.sh`, both aligned with `artifacts/live-smoke/YYYYMMDD/<profile>/`, repository-local logging, and the existing desktop-runner or RTOS-mock archive formats.
- Status: resolved.

### Web/H5 Manual Validation Still Produced Unstructured Evidence

- Problem: desktop and RTOS validation already had replay-friendly artifact layouts, but browser-side validation still relied on ad hoc screenshots, temporary notes, and manually remembered URLs. That made Web/H5 checks harder to compare and harder to attach cleanly to roadmap or bug records.
- Resolution: added `scripts/web-h5-manual-capture.sh`, which scaffolds a canonical `web-h5-manual` artifact root with `capture.json`, `manual-checklist.md`, server snapshots, page snapshots, and prepared directories for screenshots, exports, and logs. Updated the browser docs and live-validation runbook to point at that helper.
- Status: resolved.

### Python Validation Entry Points Did Not Explain Version Or Scope Clearly

- Problem: `Makefile` still called raw shell scripts and desktop Python tests directly, so Python-version mismatches or worker-test omissions were harder to diagnose than they should be.
- Resolution: routed the command surface through `bash`, added `scripts/require-python-3-11.sh`, split worker tests into `make test-py-workers`, and made `make doctor`, `make test-py`, and `make verify-fast` validate Python 3.11+ explicitly before running.
- Status: resolved.

### Native Realtime And `xiaozhi` Gateway Lifecycles Were Drifting Apart

- Problem: both websocket adapters still carried separate copies of response execution, interruption return-to-active, and active/end completion logic. That made fixes prone to drift before the next architecture step can move more ownership into `internal/voice`.
- Resolution: added shared helper layers in `internal/gateway/turn_flow.go` and `internal/gateway/output_flow.go`, then rewired both adapters to use the same turn-response and output-lifecycle path without changing the published protocols.
- Status: resolved for iteration 1; preview and playout ownership migration remains follow-up work.
