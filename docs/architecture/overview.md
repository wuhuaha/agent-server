# Architecture Overview

## Primary Layers

### 1. Realtime Session Core

Owns session state, turn management, interruption, response streaming, and shared event semantics.

The currently published RTOS turn-taking contract is `client_wakeup_client_commit`: devices or local UI logic start the session after wakeup, then explicitly close each user audio turn with `audio.in.commit`. Server-side VAD may arrive later, but it is not part of the advertised v0 contract today.

### 2. Agent Runtime Core

Owns transport-neutral turn execution, streaming turn-delta emission, conversation policy, injected memory recall or persist hooks, and injected tool catalog or invocation hooks. It consumes normalized user turns and returns shared response directives such as text output or session-close intent.

Provider choice for LLM inference remains a runtime concern inside `internal/agent`: local bootstrap execution and optional cloud chat providers are selected at app bootstrap, while transports still depend only on the shared `TurnExecutor` contract.

The runtime now also owns the iterative model-tool loop for cloud chat execution. Provider-streamed text deltas, assistant tool-call proposals, tool invocation, tool-result reinjection, and loop step budgets all stay inside `internal/agent` instead of leaking into transports or the voice layer.

The same runtime layer now owns both assistant persona selection and execution-mode policy. Built-in persona templates and custom prompt overrides stay inside `internal/agent`, while mode-specific behavior such as `simulation`, `dry_run`, or `live_control` is appended there instead of leaking debug-stage assumptions into transports.

Runtime memory is now layered as well: a bounded recent-message window feeds immediate multi-turn continuity, while summary/fact recall remains a separate compact memory layer. The default in-process backend can already load from `session`, `user`, `device`, `room`, or `household` scopes without exposing that topology to transports.

The agent runtime now also has a first deterministic household-routing slice ahead of the open-ended LLM path. For a bounded set of common home-control utterances, the runtime can respond directly using room hints from the current text or metadata, while sensitive domains such as locks, gas, and security stay on a more conservative clarification path.

### 3. Voice Runtime

Provides built-in voice capabilities such as turn detection, ASR, TTS, and stream control. It prepares spoken turns for the `Agent Runtime Core` and renders spoken output afterward.

Provider choice remains a runtime concern inside `internal/voice`: local workers and optional cloud ASR/TTS backends are selected at app bootstrap, while transports still consume one shared responder contract.

The voice runtime now also normalizes structured speech-understanding metadata before handing a turn to `internal/agent`. ASR providers may expose different fields, but the shared runtime path only sees normalized metadata keys such as language, emotion, speaker, endpoint reason, audio events, and partial hypotheses.

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
- One runtime-owned hook layer for memory and tool orchestration.
- One provider-selected LLM runtime behind shared `ChatModel`, `StreamingChatModel`, and `TurnExecutor` interfaces so model providers do not leak into device or channel adapters.
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
- Deterministic home-control routing must stay inside `internal/agent`; gateways and voice responders must not start owning their own device-intent logic.
- Voice provider logic must stay behind interfaces.
- Optional cloud ASR/TTS providers may be added only under the shared voice runtime interfaces; gateways and channel adapters must not branch on provider-specific protocols.
- Provider-specific ASR result payloads must be normalized inside `internal/voice` before any speech metadata reaches `internal/agent`.
- Channel skills must not call model providers directly.

## Related Notes

- [项目优化路线图（2026-04-04）](project-optimization-roadmap-zh-2026-04.md)
- [语音 Agent 伙伴化研究（2026-04-04）](voice-agent-companion-research-zh-2026-04.md)
- [Voice Agent Companion Research (2026-04-04)](voice-agent-companion-research-2026-04.md)
