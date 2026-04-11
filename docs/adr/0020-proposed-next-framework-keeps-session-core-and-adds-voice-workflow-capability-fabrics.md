# ADR 0020: Proposed Next Framework Keeps The Session Core And Adds Voice, Workflow, And Capability Fabrics

## Status

Proposed

## Context

The repository has already established a sound top-level architecture around:

- `Realtime Session Core`
- `Agent Runtime Core`
- `Voice Runtime`
- device and channel adapters

However, deeper architecture review against current voice-agent and agent-framework patterns shows that the next bottlenecks are no longer transport boundaries. They are capability density gaps inside the runtime:

- voice orchestration
- workflow and handoff
- skill and tool standardization
- durable context and memory
- observability and evaluation

The project therefore needs a next-stage target framework that preserves the current center while clarifying how those missing capabilities should be layered.

## Proposal

Adopt the following target shape as the proposed next architecture direction:

1. keep `Realtime Session Core` as the center
2. evolve `Voice Runtime` into a stronger `Voice Orchestration Core`
3. evolve `Agent Runtime Core` into an `Agent Workflow Core`
4. introduce a `Capability Fabric` for skills, tools, MCP, and capability policy
5. introduce a `Context & Memory Fabric` for conversation, household, identity, and screen context
6. introduce a stronger `Policy & Safety Fabric`
7. separate `Control Plane` from `Eval Plane`

This proposal is documented in:

- `docs/architecture/agent-server-next-framework-zh-2026-04-08.md`

## Consequences

- The repository keeps its RTOS-friendly session-centric shape.
- Voice interaction becomes a first-class orchestration domain instead of remaining a thinner provider wrapper layer.
- Domain growth remains skill-based instead of leaking into gateways or executor branches.
- Durable memory, MCP, workflow, and observability gain clear homes in the architecture.

## Rejected Alternatives

### Replace the whole runtime with an external framework

Rejected because the current top-level decomposition is already sound and the main remaining gaps are internal runtime capabilities.

### Keep adding capabilities directly into `internal/agent` and `internal/voice` without a clearer target shape

Rejected because that would blur boundaries again and make the repository harder to evolve over the next stages.
