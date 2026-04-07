# ADR 0012: Layer Recent Messages Over Summary Memory

## Status

Accepted

## Context

The first in-process memory backend under `internal/agent` only returned one summary string plus facts. That proved the runtime memory boundary existed, but it was still too weak for real multi-turn continuity:

- the model could not see explicit recent dialogue turns
- all recall was compressed into one textual summary
- scope selection was effectively limited to `device` first and `session` fallback

That made follow-up phrases such as “再柔和一点” or “跟刚才一样” harder to resolve consistently, especially on shared household devices.

## Decision

Keep one shared `MemoryStore` boundary, but make the returned `MemoryContext` explicitly layered:

1. `RecentMessages`: a bounded recent dialogue window for immediate conversational continuity
2. `Summary` and `Facts`: compact recall for longer-lived context hints

The default in-process `InMemoryMemoryStore` now stores each turn under multiple runtime scopes when identifiers are available:

- `session`
- `user`
- `device`
- `room`
- `household`

At load time, it selects the most relevant available scope in that order and also reports available scope counts through memory facts.

## Consequences

- The LLM executor can inject recent history without teaching transports how memory is stored.
- Multi-turn continuity improves without abandoning compact summary recall.
- The runtime is prepared for future user, room, and household grounding without requiring protocol churn today.
- More durable or remote memory backends can reuse the same `MemoryStore` contract as long as they return both recent-message context and summary/fact recall.
