---
name: protocol-guard
description: Use when editing realtime event names, session state transitions, transport envelopes, schemas, or compatibility notes for agent-server protocols.
---

# Protocol Guard

## When To Use

Use this skill whenever the wire contract, schema, or state machine changes.

## Required Steps

1. Update the protocol document in `docs/protocols/`.
2. Update the schema in `schemas/`.
3. Check naming consistency across handlers and event types.
4. Record compatibility impact in `.codex/change-log.md`.

## Guardrails

- Avoid transport-specific event names in the shared core.
- Keep server-end and client-end semantics symmetric when possible.
- Preserve reserved fields for future auth, tracing, and capabilities.
