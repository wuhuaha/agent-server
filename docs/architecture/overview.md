# Architecture Overview

## Primary Layers

### 1. Realtime Session Core

Owns session state, turn management, interruption, response streaming, and shared event semantics.

The currently published RTOS turn-taking contract is `client_wakeup_client_commit`: devices or local UI logic start the session after wakeup, then explicitly close each user audio turn with `audio.in.commit`. Server-side VAD may arrive later, but it is not part of the advertised v0 contract today.

### 2. Agent Runtime Core

Owns transport-neutral turn execution, streaming turn-delta emission, conversation policy, injected memory recall or persist hooks, and injected tool catalog or invocation hooks. It consumes normalized user turns and returns shared response directives such as text output or session-close intent.

Provider choice for LLM inference remains a runtime concern inside `internal/agent`: local bootstrap execution and optional cloud chat providers are selected at app bootstrap, while transports still depend only on the shared `TurnExecutor` contract.

The runtime now also owns the iterative model-tool loop for cloud chat execution. Provider-streamed text deltas, assistant tool-call proposals, tool invocation, tool-result reinjection, and loop step budgets all stay inside `internal/agent` instead of leaking into transports or the voice layer.

The first `F0` migration slices now also keep structured turn traceability inside this layer: `turn_id` and `trace_id` already enter `TurnInput`, and the runtime can emit correlated server logs through a wrapper without teaching transports or providers about logging policy.

The same runtime layer now owns both assistant persona selection and execution-mode policy, but no longer as one opaque hardcoded string. Prompt composition is split into core prompt sections:

- persona section
- runtime output-contract section
- execution-mode policy section

Those core sections are then composed with any runtime-skill prompt sections, so the executor owns prompt assembly while product-vertical behavior stays pluggable.

Runtime memory is now layered as well: a bounded recent-message window feeds immediate multi-turn continuity, while summary/fact recall remains a separate compact memory layer. The default in-process backend can already load from `session`, `user`, `device`, `room`, or `household` scopes without exposing that topology to transports.

Domain-specific behavior should enter this layer through runtime skills, not by hardwiring household rules into the executor path. A runtime skill may contribute prompt fragments, tool definitions, and tool execution logic while still staying behind the shared runtime interfaces.

### 3. Voice Runtime

Provides built-in voice capabilities such as turn detection, ASR, TTS, and stream control. It prepares spoken turns for the `Agent Runtime Core` and renders spoken output afterward.

Provider choice remains a runtime concern inside `internal/voice`: local workers and optional cloud ASR/TTS backends are selected at app bootstrap, while transports still consume one shared responder contract.

TTS is part of this shared voice-runtime output layer rather than a browser-only, RTOS-only, or channel-specific feature. Once the runtime has produced final user-facing text, the same synthesized audio path can be reused by native RTOS devices, browser debug pages, desktop clients, and future channel adapters that need spoken output.

The voice runtime now also normalizes structured speech-understanding metadata before handing a turn to `internal/agent`. ASR providers may expose different fields, but the shared runtime path only sees normalized metadata keys such as language, emotion, speaker, endpoint reason, audio events, and partial hypotheses.

The current baseline observability path also lives here: ASR and TTS requests now carry the same internal `turn_id` and `trace_id`, so transcriber logs, runtime logs, TTS setup logs, and playback-complete logs can all be correlated under one turn without widening the device-facing protocols.

The local FunASR reference path now follows the same boundary: `internal/voice` owns a shared `StreamingTranscriber` contract, the local HTTP worker exposes `/v1/asr/stream/*` only behind that contract, and app bootstrap chooses whether a provider is true-streaming or wrapped by the buffered compatibility adapter.

The next turn-detection slice also stays inside the same layer: `internal/voice` now owns an internal input-preview boundary for partial ASR plus silence-based turn suggestions, while websocket adapters only consume preview snapshots and optional commit suggestions. The adapters still do not call ASR providers directly, and the advertised public turn mode remains unchanged until the server-endpoint path is mature enough to publish.

That hidden preview path is now explicitly runtime-configurable through shared voice-runtime thresholds instead of adapter-local constants, and bring-up validation is expected to happen through an explicit non-default runner scenario rather than by widening the public discovery contract early.

The latest `L2` hardening slice also keeps false-endpoint protection inside the same shared layer: the hidden preview path can now hold back auto-commit when the most recent partial still looks lexically unfinished, then fall back to a longer timeout instead of splitting the turn on every short pause.

The next `L2` slice keeps that same ownership model while making provider endpoint signals useful: local worker preview hints now travel through `StreamingTranscriber` deltas into the shared turn detector, and only the shared voice runtime decides whether those hints justify a shorter endpoint window.

### 4. Device Adapters

Own ingress and egress for RTOS devices, desktops, and browsers. They translate transport details into core events and shared turn inputs.

The `xiaozhi` compatibility adapter still stays at the translation layer: even when it emits compat-only events such as `stt` transcript echo for audio turns, it derives them from the shared responder output instead of reaching back into provider-specific ASR logic.

The current browser or H5 direct path also stays on the native realtime contract. Browser-side PCM16 microphone and playback adaptation lives in the page itself, so the repository does not need a separate browser-only websocket protocol just to reach the same session core.

### 5. Channel Skills

Own ingress and egress for external messaging platforms such as Feishu. A channel skill is a transport and message adapter, not a tool runner.

### 6. Control Plane

Provides health, diagnostics, config, device management, auth, and policy APIs.

The control plane can also host same-service debug surfaces such as the built-in Web/H5 realtime bring-up page when that helps validate the shared transport contract without adding a second orchestration path.

## First Practical Shape

- One Go service process.
- One shared event envelope.
- One realtime contract.
- One transport-neutral `TurnExecutor` boundary under `internal/agent`.
- One sink-based streaming path from `StreamingTurnExecutor` through `internal/voice` into realtime `response.chunk` events.
- One provider-selected voice runtime behind shared `Transcriber` and `Synthesizer` interfaces so local and cloud voice backends do not leak into device or channel adapters.
- One provider-selected streaming ASR path behind shared `StreamingTranscriber` and `StreamingTranscriptionSession` interfaces so local preview workers and buffered compatibility adapters both terminate inside `internal/voice`.
- One voice-runtime-owned input-preview path behind shared `InputPreviewer` and `InputPreviewSession` interfaces so server-side endpoint preview can evolve without pushing provider logic into websocket adapters.
- One runtime-owned hook layer for memory and tool orchestration.
- One provider-selected LLM runtime behind shared `ChatModel`, `StreamingChatModel`, and `TurnExecutor` interfaces so model providers do not leak into device or channel adapters.
- One additive observability path that keeps `turn_id` and `trace_id` in gateway phase logs, runtime logs, voice-provider logs, and archived runner artifacts without changing the public websocket event shapes again.
- One runtime-skill path for domain behavior so smart-home semantics can be injected as prompt fragments plus tools without turning `internal/agent` into a pile of hardcoded vertical rules.
- One first in-process runtime backend set:
  - `InMemoryMemoryStore` for layered recent-message plus summary recall across `session`, `user`, `device`, `room`, and `household` scopes
  - `BuiltinToolBackend` for local runtime tools such as `time.now`, `session.describe`, and `memory.recall`
- One runtime-owned tool-name adaptation layer so provider-safe function names do not force repository-wide renames of internal tools.
- One future plugin path for channels and runtime skills.

## Hard Boundaries

- Session logic must not depend on Feishu, Slack, or any specific device.
- Device and channel adapters must not own agent policy; they hand normalized turns to the `Agent Runtime Core`.
- The `Agent Runtime Core` must not own transport framing or websocket lifecycle details.
- Gateway websocket adapters must treat any websocket read failure, including deadline timeouts, as terminal for that connection. They may emit one final timeout-close signal, but they must not re-enter `ReadMessage()` on a failed connection.
- Memory and tool backends must be injected into the `Agent Runtime Core`, not called directly from transports or channel adapters.
- The first real memory and tool backends stay in-process and ephemeral until shared persistence or remote tool execution is justified.
- Optional cloud LLM providers may be added only under the shared agent-runtime interfaces; gateways and channel adapters must not branch on provider-specific APIs.
- Provider-specific tool-call request or naming constraints must be handled inside `internal/agent`, not by changing transport contracts or runtime tool identities globally.
- Domain-specific household-control behavior must be injected as runtime skills or tools; gateways, voice responders, and the core executor path must not start owning their own device-intent rule tables.
- Voice provider logic must stay behind interfaces.
- Optional cloud ASR/TTS providers may be added only under the shared voice runtime interfaces; gateways and channel adapters must not branch on provider-specific protocols.
- Provider-specific ASR result payloads must be normalized inside `internal/voice` before any speech metadata reaches `internal/agent`.
- Channel skills must not call model providers directly.

## Related Notes

- [项目优化路线图（2026-04-04）](project-optimization-roadmap-zh-2026-04.md)
- [当前项目“流畅、自然、全双工”语音交互能力评估（2026-04-10）](full-duplex-voice-assessment-zh-2026-04-10.md)
- [本地 / 开源优先的全双工语音改造任务清单（2026-04-10）](local-open-source-full-duplex-roadmap-zh-2026-04-10.md)
- [现代 AI Agent / 语音 Agent 框架复核与架构优化建议（2026-04-08）](modern-ai-agent-framework-review-zh-2026-04-08.md)
- [`agent-server` 新一代项目框架设计提案（2026-04-08）](agent-server-next-framework-zh-2026-04-08.md)
- [从当前实现迁移到新一代项目框架的分阶段实施方案（2026-04-08）](migration-plan-to-next-framework-zh-2026-04-08.md)
- [语音 Agent 伙伴化研究（2026-04-04）](voice-agent-companion-research-zh-2026-04.md)
- [Voice Agent Companion Research (2026-04-04)](voice-agent-companion-research-2026-04.md)
