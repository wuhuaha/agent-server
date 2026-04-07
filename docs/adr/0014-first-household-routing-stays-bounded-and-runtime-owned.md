# ADR 0014: First Household Routing Stays Bounded And Runtime-Owned

## Status

Accepted

## Context

After adding streaming model execution, layered memory, and structured speech metadata, the runtime still treated most household requests as open-ended language generation. That left a gap for common smart-home commands where the assistant should feel more predictable than a generic chatbot.

At the same time, pushing home-control parsing into the voice layer or into websocket gateways would have violated the repository guardrails. The project is also still in a simulation-oriented stage, so this step should not pretend to be a complete device graph or a real execution engine.

## Decision

Add the first deterministic household-routing slice inside `internal/agent`, ahead of the open-ended model path.

This first slice stays intentionally bounded:

- recognize a small set of common intents such as lights, curtains, air conditioning, and simple household scenes
- use room hints from the current turn text or runtime metadata
- keep user-facing replies in natural language only
- treat sensitive domains such as locks, gas, and security conservatively with clarification-oriented replies

Both the bootstrap executor and the LLM-backed executor can use this same runtime-owned routing step before falling back to open-ended model generation.

## Consequences

- Common home-control requests become more predictable without moving orchestration into transports.
- The current device-facing protocols do not change.
- Sensitive domains stay conservative until a fuller household graph, policy layer, and real execution path exist.
- This is only the first slice of household context, not the final execution architecture; later work can replace or extend it with a richer household graph and deterministic action layer while keeping the routing boundary inside `internal/agent`.
