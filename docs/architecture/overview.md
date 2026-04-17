# Architecture Overview

## Primary Layers

### 1. Realtime Session Core

Owns session state, turn management, interruption, response streaming, and shared event semantics.

The currently published RTOS turn-taking contract is `client_wakeup_client_commit`: devices or local UI logic start the session after wakeup, then explicitly close each user audio turn with `audio.in.commit`. Server-side VAD may arrive later, but it is not part of the advertised v0 contract today.

The shared session core now has separate internal input and output lanes, while transport-facing `session.update` still keeps `state` as a compatibility-derived top-level view and may optionally expose `input_state` plus `output_state` for richer clients. That gives the runtime a foundation for speaking-time preview and accepted-turn attribution without forcing older clients to change.

That said, the repository is still not full-duplex end-to-end yet. The dual-track state skeleton exists, but interruption arbitration, planner overlap, and gateway behavior still need to converge on that model before realtime voice feels truly duplex in practice.

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

The shared TTS boundary now also covers one local open-source GPU deployment path: `cosyvoice_http` targets the official CosyVoice FastAPI runtime as a local dependency, while `agent-server` still owns provider selection, output normalization, and pacing inside `internal/voice`.

TTS is part of this shared voice-runtime output layer rather than a browser-only, RTOS-only, or channel-specific feature. Once the runtime has produced final user-facing text, the same synthesized audio path can be reused by native RTOS devices, browser debug pages, desktop clients, and future channel adapters that need spoken output.

The voice runtime now also normalizes structured speech-understanding metadata before handing a turn to `internal/agent`. ASR providers may expose different fields, but the shared runtime path only sees normalized metadata keys such as language, emotion, speaker, endpoint reason, audio events, partial hypotheses, and punctuation-derived hints.

The shared agent prompt path may consume those normalized speech hints for reply shaping and clarification quality, but adapters still must not reason about provider-specific ASR payloads themselves.

The current baseline observability path also lives here: ASR and TTS requests now carry the same internal `turn_id` and `trace_id`, so transcriber logs, runtime logs, TTS setup logs, and playback-complete logs can all be correlated under one turn without widening the device-facing protocols.

The local FunASR reference path now follows the same boundary: `internal/voice` owns a shared `StreamingTranscriber` contract, the local HTTP worker exposes `/v1/asr/stream/*` only behind that contract, and app bootstrap chooses whether a provider is true-streaming or wrapped by the buffered compatibility adapter.

That worker boundary now also carries the first modular local speech pipeline without widening the public protocols: the worker may stay on backward-compatible buffered preview, or switch internally to a 2pass path of `optional KWS + online preview + final-ASR correction`. Worker-side `fsmn-vad`, `Silero VAD`, KWS prefix stripping, and preview endpoint hints all remain runtime-owned details behind `internal/voice`; device adapters still consume only normalized preview/final deltas and audio events.

The next turn-detection slice also stays inside the same layer: `internal/voice` now owns an internal input-preview boundary for partial ASR plus silence-based turn suggestions, while websocket adapters only consume preview snapshots and optional commit suggestions. The adapters still do not call ASR providers directly, and the advertised public turn mode remains unchanged until the server-endpoint path is mature enough to publish.

That path is no longer treated only as a hidden experiment. It is now advertised through discovery as a `server_endpoint` main-path candidate while still keeping `turn_mode=client_wakeup_client_commit` as the default public v0 contract. In other words: discovery may now tell clients that shared server endpointing is available and enabled, but explicit client commit remains the compatibility baseline until the candidate path graduates further.

That discovery-advertised candidate path is explicitly runtime-configurable through shared voice-runtime thresholds instead of adapter-local constants, and bring-up validation is expected to happen through an explicit non-default runner scenario rather than by widening the public event contract early.

The preview path now also has a lower-latency accept fast path inside the same ownership boundary: when an active preview session can finalize its streaming ASR state, the shared runtime may carry that finalized transcription into the accepted audio turn instead of replaying the same buffered PCM from scratch. Gateways only trigger the finalize-or-fallback step; they still do not own ASR provider logic.

That preview boundary now also emits stronger internal early-processing evidence without widening accepted-turn semantics. `internal/voice` derives a runtime-owned `stable_prefix` plus `utterance_complete` hint from preview deltas, may expose the observational prefix to preview-aware clients, and may use the same signal to prewarm the shared agent runtime. The later accepted turn only reuses that prewarm when the final accepted text matches exactly, so speculative work stays reversible and adapters still do not decide turn acceptance.

The latest `L2` hardening slice also keeps false-endpoint protection inside the same shared layer: the hidden preview path can now hold back auto-commit when the most recent partial still looks lexically unfinished, then fall back to a longer timeout instead of splitting the turn on every short pause.

The latest `L2` hardening slice keeps that same ownership model while making provider endpoint signals stronger without widening the protocol: local worker preview hints may now come from the default tail-energy path or an optional worker-side `Silero VAD` path, but they still travel only through `StreamingTranscriber` deltas into the shared turn detector. Only the shared voice runtime decides whether those hints justify a shorter endpoint window.

The latest orchestration slice also makes `internal/voice` the owner of hidden preview sessions, playback lifecycle callbacks, and "what the user actually heard" persistence. `SessionOrchestrator` now keeps preview polling, auto-commit suggestions, playback start or interrupt or completion, and heard-text truncation under one shared runtime boundary. Gateway adapters report transport events into that orchestrator instead of persisting preview or playout state on their own.

That same ownership now extends one step farther into the next turn: once a spoken reply finishes or is interrupted, `internal/voice` keeps the latest playback outcome as structured runtime state and projects an additive `voice.previous.*` metadata view into the next shared `TurnRequest`. This keeps the gateway in an adapter role while giving `internal/agent` enough context to distinguish "the assistant generated this" from "the user actually heard up to here" for continue, recap, and interruption-aware follow-up behavior.

The shared agent runtime now also applies a bounded direct follow-up policy on top of that metadata. The playback truth still comes only from `internal/voice`, but `internal/agent` may now treat utterances such as `继续`, `后面呢`, or `你刚刚说到哪了` as playback-aware follow-ups instead of generic fresh turns. Bootstrap fallback and LLM prompt shaping both reuse the same runtime-owned `voice.previous.*` context rather than teaching adapters any resume heuristics.

The next behavior-depth slice stays on that same boundary instead of creating a second protocol family. Soft interruption outcomes such as `backchannel` and `duck_only` now flow through a runtime-owned `PlaybackDirective` and can duck shared PCM16 playout on the native realtime path without inventing new wire events. Earlier speech-planner audio start also stays internal: responders may expose a `TurnResponseFuture` so the gateway can start output before the final `TurnResponse` settles, while transports still emit the same public `response.start -> response.chunk -> audio` lifecycle.

The latest interruption-verification slice keeps that same ownership model but makes the decision path more acoustic-first. `internal/voice` now treats speaking-time intrusion as a two-step runtime problem: first recognize low-latency acoustic intrusion strongly enough to enter reversible `duck_only`, then use preview evidence such as `stable_prefix`, lexical completeness, accept-candidate state, and takeover lexicon to confirm whether the turn should escalate to `hard_interrupt` or remain `backchannel`. Gateways still apply only the runtime-owned `PlaybackDirective`, but observability now records acoustic-ready, semantic-ready, turn-stage, and score-like evidence so future tuning can happen from traces instead of policy labels alone.

The next refinement on that same boundary now also lets an optional LLM act as a runtime-owned semantic judge instead of keeping every decision inside handwritten heuristics. The shared voice runtime may asynchronously ask a provider-neutral `agent.ChatModel` for a small structured judgement over a mature preview candidate, then merge that result back into `InputPreview.Arbitration` as advisory evidence such as semantic completeness, correction-in-progress, backchannel intent, or takeover intent. This does not replace the heuristic safety floor: adapters still do not call models directly, `CommitSuggested` is still not created by the model alone, and acoustic/timing gates remain the final safeguard for realtime behavior.

The next slot-depth slice now extends that same runtime-owned path one step farther: `SemanticSlotParser` may first produce `domain / intent / slot completeness / actionability / clarify_needed`, and then a runtime-owned entity catalog grounder may turn positive alias evidence into canonical target or location summaries such as `客厅灯 -> 客厅灯` or `VS Code -> Visual Studio Code`. This grounding stays inside `internal/voice`, feeds only additive arbitration summaries such as `slot_grounded`, `slot_canonical_target`, and `slot_canonical_location`, and deliberately uses a positive-evidence rule: catalog hits may promote or clarify a parse, explicit multi-hit ambiguity may pull it back toward `clarify_needed`, but a catalog miss must not by itself negate the LLM slot parse because the seed catalog is intentionally incomplete during the research stage.

The latest convergence slice keeps that path generic instead of letting seed demo logic leak into the runtime core. `internal/voice` may still own recent-context ranking, provider-neutral ASR hint generation, slot value normalization, and risk gating, but concrete smart-home or desktop entities now live behind an optional built-in catalog profile (`seed_companion`) instead of being treated as a permanent architecture assumption. High-risk confirmation now consumes abstract annotations such as `risk_level` from runtime-owned catalog or policy data rather than lexical business-term lists scattered through the voice runtime.

The latest output-orchestration slice also keeps the same compatibility boundary while making the planner more explicit. `internal/voice` now treats early speech output as structured clause planning rather than only raw string chunking: each internal clause carries boundary strength, a lightweight prosody hint, launchability before final turn settlement, and an estimated duration. The clause queue is now buffered so one slow TTS startup does not back-pressure later text deltas as aggressively, and the gateway now uses `ResponseAudioStart.Text` as the earliest trustworthy speech text when audio starts before the first delta is consumed. That means `response.start` can still truthfully advertise `text,audio` in the audio-first race without inventing a second protocol family.

The latest playback-truth follow-up closes one more early-audio gap on that same boundary: once the native realtime path has started speaking, later streamed text deltas may now extend the active playback text inside `SessionOrchestrator` instead of waiting for final response settlement. This makes interruption truncation and next-turn `voice.previous.*` context less likely to lag behind what the assistant was already in the middle of saying, while still keeping delivered-text and heard-text ownership entirely inside `internal/voice`.

The next playback-truth depth slice now pushes that same ownership into two more corners without widening the public contract. First, native realtime now synchronizes the latest announced `audio.out.meta` segment text back into the runtime playback context while output is still speaking, so later `audio.out.mark` or `audio.out.cleared` can reconcile exact heard or missed boundaries against the most recently announced tail instead of waiting for final response settlement. Second, `duck_only` and `backchannel` no longer disappear into metadata-only history if transport playback later completes naturally: `internal/voice` may now preserve a recoverable prefix or missed-tail boundary even when `playback_completed=true`, which lets `internal/agent` continue or recap from runtime-owned `voice.previous.*` facts rather than assuming natural completion always means the user heard everything.

The current long-term voice direction now explicitly converges on a `server-primary hybrid` architecture after wake and session establishment: devices keep audio-front-end, local reflex, playback execution, playback telemetry, and fallback controls, while the shared voice runtime owns preview, early-processing gates, interruption arbitration, clause-level output orchestration, and playback-truth reconciliation. The public realtime contract still evolves additively and compatibility-first while that target graduates.

The next service-side optimization priority is now explicit on top of that same architecture: do not widen the transport or model surface first. Instead, keep the cascade boundary and strengthen `internal/voice` with a multi-signal turn arbitrator, acoustic-first interruption verification, layered reversible early-processing, clause-aware output planning, finer playback-truth alignment, and runtime-owned dynamic biasing for domain entities.

### 4. Device Adapters

Own ingress and egress for RTOS devices, desktops, and browsers. They translate transport details into core events and shared turn inputs.

The `xiaozhi` compatibility adapter still stays at the translation layer: even when it emits compat-only events such as `stt` transcript echo for audio turns, it derives them from the shared responder output instead of reaching back into provider-specific ASR logic.

The current browser or H5 direct path also stays on the native realtime contract. Browser-side PCM16 microphone and playback adaptation lives in the page itself, so the repository does not need a separate browser-only websocket protocol just to reach the same session core.

Inside `internal/gateway`, native realtime and `xiaozhi` compatibility adapters now share one turn-response and output-lifecycle path instead of each carrying their own copy of `commit -> thinking -> response.start -> speaking -> active/end`. That shared helper layer is an intermediate refactor step before preview, interrupt arbitration, and playout ownership move deeper into `internal/voice`.

On the current native realtime main path, that shared output layer now also owns two latency-sensitive behaviors:

- soft speaking-time output arbitration, where `backchannel` and `duck_only` duck current playout instead of forcing an immediate hard cancel
- earlier incremental output start, where the adapter can begin streaming planned audio from the shared response future before final turn settlement completes

### 5. Channel Skills

Own ingress and egress for external messaging platforms such as Feishu. A channel skill is a transport and message adapter, not a tool runner.

The first shared `internal/channel` bridge now fixes the adapter shape as:

- normalize inbound message, thread, attachment, and idempotency metadata
- hand the resulting turn to the shared `Agent Runtime Core`
- deliver the shared runtime response back through the channel adapter
- report delivery outcome without teaching the adapter about providers or core runtime policy

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
- One local open-source GPU TTS option behind the same shared `Synthesizer` boundary so CosyVoice deployment details still stop inside `internal/voice`.
- One provider-selected streaming ASR path behind shared `StreamingTranscriber` and `StreamingTranscriptionSession` interfaces so local preview workers and buffered compatibility adapters both terminate inside `internal/voice`.
- One worker-internal modular speech path so local FunASR can add `KWS`, worker-side VAD, online preview, and final-ASR correction without teaching device adapters or the public contracts about model-serving details.
- One voice-runtime-owned input-preview path behind shared `InputPreviewer` and `InputPreviewSession` interfaces so server-side endpoint preview can evolve without pushing provider logic into websocket adapters.
- One bounded preview-to-runtime prewarm bridge so stable complete preview text can prepare prompt or memory or tool context inside `internal/agent` without promoting preview observations into new wire-level acceptance semantics.
- One voice-runtime-owned session orchestrator behind shared preview and playback callbacks so auto-commit, interruption, playout completion, and heard-text persistence stop being split across multiple websocket handlers.
- One voice-runtime-owned playback-truth bridge into the next shared turn context so interruption or resume-aware agent behavior can reuse `heard_text`, `missed_text`, and `resume_anchor` without teaching gateways how to reason about playback facts.
- One runtime-owned hook layer for memory and tool orchestration.
- One provider-selected LLM runtime behind shared `ChatModel`, `StreamingChatModel`, and `TurnExecutor` interfaces so model providers do not leak into device or channel adapters.
- One tiered voice-intelligence path inside `internal/voice` so small semantic-judge models, medium slot/domain parsers, and larger dialogue models can evolve independently without giving realtime transport adapters policy ownership.
- One additive observability path that keeps `turn_id` and `trace_id` in gateway phase logs, runtime logs, voice-provider logs, and archived runner artifacts without changing the public websocket event shapes again.
- One runtime-skill path for domain behavior so smart-home semantics can be injected as prompt fragments plus tools without turning `internal/agent` into a pile of hardcoded vertical rules.
- One channel-runtime bridge so external messaging adapters normalize input, hand it to the shared runtime, and deliver results back without learning provider-specific APIs.
- One startup-time config validation layer split by runtime domain so invalid provider or credential combinations fail at process bring-up instead of later inside request handling.
- One first in-process runtime backend set:
  - `InMemoryMemoryStore` for layered recent-message plus summary recall across `session`, `user`, `device`, `room`, and `household` scopes
  - `BuiltinToolBackend` for local runtime tools such as `time.now`, `session.describe`, and `memory.recall`
- One runtime-owned tool-name adaptation layer so provider-safe function names do not force repository-wide renames of internal tools.
- One future plugin path for channels and runtime skills.

## Hard Boundaries

- Session logic must not depend on Feishu, Slack, or any specific device.
- Device and channel adapters must not own agent policy; they hand normalized turns to the `Agent Runtime Core`.
- Channel adapters should prefer the shared `internal/channel` runtime bridge for normalize -> handoff -> deliver flow instead of open-coding runtime or provider calls in each adapter.
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

## Repository Client Taxonomy

- Standalone reference and debug clients belong under `clients/`.
- `tools/` is reserved for auxiliary diagnostics, capture, conversion, bootstrap, or one-off lab helpers.
- A browser or desktop client does not move into `tools/` just because it is used for validation; if it is a reusable endpoint implementation over a shared protocol, keep it in `clients/`.

## Related Notes

- [服务侧语音优化建议（深度研究，2026-04-16）](service-side-voice-optimization-recommendations-zh-2026-04-16.md)
- [项目优化路线图（2026-04-04）](project-optimization-roadmap-zh-2026-04.md)
- [第一阶段语音 Agent Demo 实时体验优化研究（2026-04-14）](voice-demo-realtime-optimization-zh-2026-04-14.md)
- [当前 realtime 全双工差距复核（2026-04-15）](realtime-full-duplex-gap-review-zh-2026-04-15.md)
- [语音架构完整方案（2026-04-16）](voice-architecture-blueprint-zh-2026-04-16.md)
- [语音架构执行路线图（2026-04-16）](voice-architecture-execution-roadmap-zh-2026-04-16.md)
- [端到端时延预算与主观体感映射（2026-04-16）](latency-budget-and-subjective-feel-zh-2026-04-16.md)
- [播放事实回传与 heard-text 真相链（2026-04-16）](playback-facts-and-heard-text-truth-chain-zh-2026-04-16.md)
- [分层 LLM + FunASR 增强策略研究（2026-04-17）](voice-multi-llm-and-funasr-strategy-zh-2026-04-17.md)
- [当前项目“流畅、自然、全双工”语音交互能力评估（2026-04-10）](full-duplex-voice-assessment-zh-2026-04-10.md)
- [本地 / 开源优先的全双工语音改造任务清单（2026-04-10）](local-open-source-full-duplex-roadmap-zh-2026-04-10.md)
- [本地 CosyVoice GPU TTS 接入说明](local-cosyvoice-gpu-tts.md)
- [现代 AI Agent / 语音 Agent 框架复核与架构优化建议（2026-04-08）](modern-ai-agent-framework-review-zh-2026-04-08.md)
- [`agent-server` 新一代项目框架设计提案（2026-04-08）](agent-server-next-framework-zh-2026-04-08.md)
- [从当前实现迁移到新一代项目框架的分阶段实施方案（2026-04-08）](migration-plan-to-next-framework-zh-2026-04-08.md)
- [语音 Agent 伙伴化研究（2026-04-04）](voice-agent-companion-research-zh-2026-04.md)
- [Voice Agent Companion Research (2026-04-04)](voice-agent-companion-research-2026-04.md)
