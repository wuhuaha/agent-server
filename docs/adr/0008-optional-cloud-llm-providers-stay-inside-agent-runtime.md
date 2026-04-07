# ADR 0008: Optional Cloud LLM Providers Stay Inside Agent Runtime

## Status

Accepted

## Context

The shared `Agent Runtime Core` already owns turn execution, memory recall or persist, tool invocation, and streamed delta emission. The next step is adding a real chat model without teaching RTOS gateways, desktop clients, or future channel adapters how to call a specific provider API.

DeepSeek offers an OpenAI-compatible chat completions interface, and the current runtime can benefit from a first real cloud LLM path. The architecture risk is letting that API call leak into `internal/voice`, websocket handlers, or future channel adapters.

## Decision

Keep optional cloud LLM integrations inside `internal/agent` behind shared `ChatModel` and `TurnExecutor` contracts.

App bootstrap may select among:

- `bootstrap`
- `deepseek_chat`

The first cloud-backed executor is `deepseek_chat`, which calls DeepSeek's chat completions API from inside `internal/agent` while preserving the same memory, tool, streamed-delta, and session-directive boundaries.

## Consequences

- Device and channel adapters continue to depend only on shared runtime contracts, not provider SDKs or HTTP shapes.
- Memory and tool debugging commands such as `/memory` and `/tool ...` remain runtime-owned and can bypass the LLM path cleanly.
- Future cloud or local LLM providers can replace or extend the runtime through the same injected model boundary.
- Provider-specific retries, auth, and response parsing now live in one replaceable runtime layer instead of spreading across transports.
