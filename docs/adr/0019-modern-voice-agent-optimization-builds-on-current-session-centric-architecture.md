# ADR 0019: Modern Voice-Agent Optimization Builds On The Current Session-Centric Architecture

## Status

Accepted

## Context

On 2026-04-08, the repository architecture was re-reviewed against current official documentation for modern AI-agent and voice-agent frameworks, including:

- LiveKit Agents
- Pipecat
- OpenAI Voice Agents / Realtime / Agents SDK
- LangGraph
- Model Context Protocol
- Home Assistant Voice

That review showed two things clearly:

1. the current `agent-server` top-level direction is still correct
2. the main gaps are now capability gaps inside the runtime stack, not a broken system decomposition

The repository already keeps:

- `Realtime Session Core` as the center
- `Agent Runtime Core` as the transport-neutral execution boundary
- `Voice Runtime` as a shared built-in capability
- device adapters and channel adapters as ingress/egress layers

Replacing the core with an external framework would create churn across the RTOS fast path, realtime contract, and existing runtime boundaries without directly solving the most important gaps.

## Decision

Keep the current session-centric architecture and optimize within it.

For the next architecture cycle, prioritize these directions over framework replacement:

1. strengthen voice-runtime orchestration
   - turn detection
   - endpointing
   - interruption and playout control
2. strengthen observability and evaluation
   - traceability
   - latency and quality metrics
   - comparable regression reports
3. strengthen skill and tool standardization
   - runtime skill registry
   - MCP-aligned tool integration
4. strengthen durable memory and identity context
   - `session / user / device / room / household`
5. add a lightweight workflow or handoff layer above the current runtime

External frameworks may still inform implementation choices, but they do not replace the shared session core, runtime boundaries, or realtime contract at this stage.

## Consequences

- The repository avoids high-churn framework migration while keeping the RTOS fast path stable.
- The optimization focus moves to the real bottlenecks: voice runtime quality, memory durability, tool standardization, and observability.
- Future Web/H5, RTOS, desktop, and channel adapters can continue to reuse one shared runtime core.
- The project stays aligned with the user's preference for a more AI-native system where domain behavior enters through skills instead of hardcoded executor branches.

## Rejected Alternatives

### Replace the core runtime with a third-party agent framework now

Rejected because the current decomposition is already sound, while the most important gaps are internal capability gaps rather than missing top-level layers.

### Reintroduce household-control rules directly into the executor or gateway

Rejected because it would weaken the runtime-skill direction and make the system less AI-native.

### Prioritize end-to-end speech-to-speech experimentation before observability and policy layers

Rejected because the current project needs stronger control, measurement, and extensibility first.
