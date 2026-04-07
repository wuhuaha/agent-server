# Agent Runtime Core

## Goal

Insert one transport-neutral execution boundary between session management and future channel adapters so `agent-server` can grow from a voice bring-up stack into a real agent service without teaching every transport how to make agent decisions.

## Current Package Shape

- `internal/agent/contracts.go`: shared turn input, streamed turn delta, memory or tool hook contracts, `TurnExecutor`, and `StreamingTurnExecutor`
- `internal/agent/hooks.go`: compatibility no-op memory store, tool registry, and tool invoker implementations
- `internal/agent/runtime_backends.go`: first real in-process memory store and builtin tool backend implementations
- `internal/agent/bootstrap_executor.go`: bootstrap executor that implements both materialized and sink-based streaming turn execution
- `internal/agent/llm_executor.go`: shared LLM-backed executor that preserves the same memory, tool, and streaming turn boundaries
- `internal/agent/deepseek_chat.go`: DeepSeek OpenAI-compatible chat provider behind the shared `ChatModel` and `StreamingChatModel` contracts
- `internal/voice`: adapts runtime turn output into `Responder` or `StreamingResponder` paths and keeps ASR or TTS as built-in runtime capabilities
- `internal/gateway`: streams runtime deltas directly into realtime `response.chunk` events when a streaming responder is available

## Responsibilities

- accept normalized turn input after transport decoding and optional ASR
- return transport-neutral response text, streamed deltas, and session directives
- own the hook boundary for memory recall or persist and tool catalog or invocation
- own the LLM provider boundary for optional cloud chat execution
- own iterative model or tool execution loops, including provider-streamed text deltas, tool-call reinjection, and loop step budgets
- own the default assistant persona templates, execution-mode policy rendering, and custom prompt composition used by cloud LLM executors
- expose both materialized and true streaming execution paths without changing transport contracts
- stay reusable across RTOS websocket sessions, desktop debug clients, and future channel adapters

## Non-Responsibilities

- websocket upgrades, frame pacing, or binary audio framing
- direct device or channel API calls
- raw ASR or TTS provider integration
- raw model-provider HTTP or SDK calls inside transports
- long-term storage implementation details inside transport adapters

## Current Bootstrap Behavior

The first executor is intentionally simple:

- text turns echo the normalized user text
- audio turns can still produce the existing bootstrap summary when no richer agent logic exists yet
- bootstrap replies are now represented as runtime deltas so the gateway can stream multiple `response.chunk` events in order
- bootstrap end-of-dialog decisions are returned as executor output instead of being hard-coded in the websocket gateway
- every turn now passes through runtime-owned memory load or save hooks, with `in_memory` as the default app-wired backend and `noop` still available as an explicit fallback
- the default memory backend keeps a bounded recent-turn window in process, keyed by device first and then session when no device identifier exists
- the default tool backend exposes builtin local tools (`time.now`, `session.describe`, `memory.recall`) through the same registry or invoker contracts that future remote providers will implement
- the runtime can now also select an optional LLM-backed executor; `deepseek_chat` is the first cloud model provider and still sits entirely behind `internal/agent`
- when no custom prompt override is provided, the LLM path now uses a built-in household control-screen assistant persona template with assistant-name substitution
- the runtime now appends execution-mode policy separately from persona selection, so `simulation`, `dry_run`, and `live_control` can change behavior without rewriting the assistant persona
- custom prompt overrides replace the persona template only; execution-mode policy still comes from runtime config
- the LLM path now carries explicit chat messages and tool definitions inside the shared runtime contract, so provider-specific request shapes stay hidden from transports
- `StreamingChatModel` allows provider-streamed text deltas to flow through `TurnDeltaKindText` immediately, while the shared executor still keeps tool progress on the same ordered runtime delta channel
- the shared LLM executor now runs a bounded model or tool loop: assistant tool calls are emitted as runtime deltas, invoked through the injected `ToolInvoker`, then reinserted as tool messages for the next model step
- provider-specific tool-name constraints stay inside the runtime adapter: runtime tool names such as `session.describe` are mapped to model-safe aliases only when constructing model requests, then mapped back before invocation
- runtime memory is now explicitly layered:
  - `RecentMessages` carries a bounded conversational window for immediate multi-turn continuity
  - `Summary` plus `Facts` remains the longer-lived recall layer for compact memory hints
- the default in-memory backend now stores the same turn under multiple runtime scopes (`session`, `user`, `device`, `room`, `household`) and selects the most relevant available scope at load time without teaching transports about memory topology
- the voice runtime can now enrich agent turns with normalized speech-understanding metadata such as language, emotion, speaker, endpoint reason, audio events, and partials without exposing provider-specific ASR payloads to transports or to the agent runtime contract itself
- the runtime now contains a first bounded household-control routing slice before the open-ended model path:
  - obvious lights, curtains, air-conditioner, and simple scene requests can be handled deterministically
  - room hints can come from the current turn text or runtime metadata such as `room_name`
  - sensitive device domains such as locks, gas, and security stay on a conservative clarification path
- reserved bootstrap commands now exist for runtime bring-up:
  - `/tool <name> <json>` exercises the shared tool registry or invoker path and emits ordered `tool_call`, `tool_result`, and text deltas
  - `/memory` surfaces the currently remembered summary without teaching the transport about memory backends
- bootstrap control commands are intentionally not persisted into turn memory so debug operations do not overwrite conversation recall
- `ExecuteTurn` remains available for compatibility, but the runtime can now stream deltas through `StreamTurn` and flush them out through realtime before the final response object is fully assembled

## Next Extension Steps

1. deepen the household context layer from bounded room or intent hints into a real household graph and deterministic execution path
2. route the first Feishu channel adapter through the same turn contract
3. replace or extend cloud LLM providers only through the shared agent-runtime model boundary
4. replace the in-process runtime backends only when a real persistence or remote tool requirement appears
