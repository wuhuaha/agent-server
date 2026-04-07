# ADR 0011: Runtime Tool Loop Stays Inside Agent Runtime

## Status

Accepted

## Context

The first cloud LLM path under `internal/agent` already proved that model-provider HTTP code can stay out of transports. However, that path still behaved like a single-shot text generator:

- no provider-streamed text deltas
- no model-proposed tool calls
- no tool-result reinjection for follow-up reasoning

Implementing those behaviors in the voice responder or websocket gateway would have broken the intended architecture. There was also a provider-specific constraint to handle: DeepSeek's OpenAI-compatible tool names must follow a restricted identifier format, while the current runtime-safe tool names already use dotted names such as `session.describe` and `memory.recall`.

## Decision

Keep the full model-tool loop inside `internal/agent`.

The shared runtime now owns:

1. streaming model text deltas through `StreamingChatModel`
2. assistant tool-call proposal handling
3. injected `ToolInvoker` execution
4. tool-result reinjection as follow-up chat messages
5. bounded loop step budgets
6. provider-facing tool-name adaptation

Provider-facing tool-name aliases are generated only when constructing model requests. Internal tool identities remain unchanged, and the runtime maps model-safe aliases back to the actual tool names before invocation.

## Consequences

- Transports and the voice layer remain adapters over the shared runtime instead of becoming provider-aware orchestration code.
- Runtime deltas keep one transport-neutral shape: streamed text, tool-call progress, and tool-result progress all still flow through the existing turn-delta contract.
- Internal tool names do not need to change globally just because one provider has stricter naming rules.
- Future model providers can adopt different request or tool-call quirks under `internal/agent` without forcing protocol churn or device-side updates.
