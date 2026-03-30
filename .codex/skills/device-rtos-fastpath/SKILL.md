---
name: device-rtos-fastpath
description: Use when implementing or reviewing the first RTOS voice path, including wake-to-session start, audio uplink, streamed output, barge-in, and client or server initiated session end.
---

# Device RTOS Fast Path

## When To Use

Use this skill for RTOS-facing transport handlers, capability negotiation, audio framing, and low-latency voice session behaviour.

## Design Priorities

- Minimize device complexity.
- Prefer one stable event envelope.
- Support wakeup-driven immediate session start.
- Support both client and server initiated session end.
- Allow half-duplex fallback without a separate protocol family.

## Implementation Checklist

1. Confirm the event names against `docs/protocols/realtime-session-v0.md`.
2. Keep audio framing simple and explicit.
3. Preserve room for future auth fields even if unused.
4. Document any device assumptions in `plan.md` or `docs/`.
