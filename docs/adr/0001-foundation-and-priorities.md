# ADR 0001: Foundation And Priorities

- Status: accepted
- Date: 2026-03-25

## Context

The project needs to support RTOS devices, desktops, and later external channels while keeping voice as a built-in capability. The user asked to prioritize architecture first, then the fastest path to RTOS realtime voice, while leaving room for platform integrations such as Feishu.

## Decision

- Start with a modular monolith.
- Make the shared realtime session core the central abstraction.
- Use Go for the main service.
- Defer full authentication from the first fast path.
- Use a single first transport shape based on WebSocket, binary audio, and JSON control events.
- Model Feishu-like integrations as channel skills.

## Consequences

- The first implementation can move quickly without locking the whole system into one device or one messaging platform.
- Protocol and architecture documents must stay synchronized with code changes.
- Authentication will need a deliberate backfill milestone, but the contract can preserve space for it from day one.
