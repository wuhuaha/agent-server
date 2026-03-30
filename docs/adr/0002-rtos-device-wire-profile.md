# ADR 0002: RTOS Device Wire Profile

- Status: accepted
- Date: 2026-03-25

## Context

Before implementing the first RTOS voice fast path, the device-facing contract must be concrete enough that server and firmware work can proceed in parallel. The previous protocol document only described transport-neutral event names and states.

## Decision

- Define the first device-facing wire profile as `rtos-ws-v0`.
- Use WebSocket text frames for control events and binary frames for audio.
- Keep one active session per socket in the bootstrap profile.
- Make `pcm16le/16k/mono` the required baseline audio profile.
- Allow both client and server to terminate the session.
- Publish runtime defaults through `GET /v1/realtime` and `.env.example`.

## Consequences

- RTOS client teams can start implementing against a stable handshake and session flow.
- The initial server implementation stays simple because it does not need multi-session multiplexing on one socket.
- Future auth, richer codecs, or browser-oriented transports must preserve the same semantic session model.
