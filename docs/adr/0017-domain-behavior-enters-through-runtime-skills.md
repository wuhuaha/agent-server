# ADR 0017: Domain Behavior Enters Through Runtime Skills

## Status

Accepted

## Context

The shared `Agent Runtime Core` already owns model execution, memory recall, tool orchestration, and streamed turn output. However, the repository had started to accumulate domain-specific household-control behavior directly in the executor path:

- deterministic household routing inside `internal/agent`
- home-control interpretation rules mixed into the core assistant persona

That direction weakens the architecture in two ways:

1. it turns the runtime core into a pile of product-vertical rule branches
2. it makes future AI-native behavior harder because the model gets bypassed before the normal tool loop has a chance to run

At the same time, household-control semantics still need a home inside the runtime, not inside transports or browser pages.

## Decision

Move domain-specific household behavior out of the core executor path and into runtime skills.

For the first built-in runtime-skill slice:

- the core executor no longer deterministically short-circuits common household requests
- `BuiltinToolBackend` may contribute runtime-skill prompt fragments in addition to tool definitions
- the first built-in skill is `household_control`
- that skill contributes:
  - a prompt fragment that tells the model how to handle smart-home control and risky household domains
  - a tool definition `home.control.simulate`
  - tool execution logic that normalizes structured household requests into a shared result payload

The shared core still owns:

- persona baseline
- runtime output contract
- execution-mode policy
- model-tool loop
- memory and tool boundaries

But household-control interpretation now enters through the runtime-skill layer rather than a hardcoded branch in `LLMTurnExecutor` or `BootstrapTurnExecutor`.

## Consequences

- The project stays more AI-native because household utterances now reach the model and can use the normal tool loop.
- TTS remains a shared `internal/voice` output capability and is unaffected by channel choice.
- Domain behavior is still runtime-owned, but now it is pluggable and configurable through `AGENT_SERVER_AGENT_SKILLS`.
- Future smart-home, calendar, reminder, or knowledge behavior can follow the same pattern:
  - prompt fragment
  - tool surface
  - tool execution logic

## Rejected Alternatives

### Keep deterministic home-control routing inside the executor

Rejected because it keeps expanding the core runtime with domain-specific branching and bypasses the model-tool loop.

### Move smart-home rules into browsers, RTOS gateways, or channel adapters

Rejected because domain behavior must remain transport-neutral and reusable across RTOS, Web/H5, desktop, and future channels.
