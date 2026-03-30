---
name: agent-architecture-guard
description: Use when changing architecture boundaries, service layering, runtime capability ownership, or any decision that could blur the line between session core, device adapters, channel skills, and control plane.
---

# Agent Architecture Guard

## When To Use

Use this skill for architecture decisions, major refactors, component moves, or feature additions that affect the service boundaries.

## Review Lens

- Keep `Realtime Session Core` transport-neutral.
- Keep device and channel layers as adapters.
- Keep provider integrations behind interfaces.
- Preserve the path to future auth and policy.

## Required Follow-Through

- Update `docs/architecture/overview.md`.
- Add or update an ADR under `docs/adr/`.
- Reflect durable choices in `.codex/project-memory.md`.
