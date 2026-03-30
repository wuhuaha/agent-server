# Realtime Session Protocol v0

## Intent

This contract defines the first shared event model for realtime sessions across RTOS devices, desktops, and future channel-driven realtime adapters.

This document defines the transport-neutral semantic model. The first concrete RTOS wire profile is documented in `docs/protocols/rtos-device-ws-v0.md`.

## Session States

- `idle`: no active session.
- `armed`: device is awake and ready to start a session.
- `active`: audio or text input is being accepted.
- `thinking`: the server is preparing a reply.
- `speaking`: the server is streaming TTS or text output.
- `closing`: the session is ending and no new turn should start.

## Required Behaviours

- Wakeup on the client can trigger `session.start`.
- Both client and server may send `session.end`.
- A reason must be supplied when ending a session.
- User speech during server output may trigger `barge_in`.
- RTOS devices may use half-duplex fallback without changing the event envelope.

## Core Events

- `session.start`
- `session.update`
- `audio.in.append`
- `audio.in.commit`
- `text.in`
- `image.in`
- `response.start`
- `response.chunk`
- `audio.out.chunk`
- `session.end`
- `error`

## Event Semantics

- `session.start`: opens a new realtime dialog after local wakeup or explicit user action.
- `session.update`: communicates non-terminal session state changes such as `armed`, `thinking`, `speaking`, or barge-in hints.
- `audio.in.append`: semantic name for inbound audio chunks from the client; the concrete wire profile defines the actual codec and framing details.
- `audio.in.commit`: indicates end of the current user turn, not necessarily end of session.
- `text.in`: sends text input on the same session when typing fallback is available.
- `image.in`: sends an image reference or attachment metadata on the same session.
- `response.start`: begins a server response turn.
- `response.chunk`: streams partial text or structured response deltas.
- `audio.out.chunk`: semantic name for outbound server audio chunks.
- `session.end`: closes the session from either side and always includes a reason.
- `error`: reports recoverable or terminal protocol errors.

## Envelope

Every event uses a shared envelope:

- `type`
- `session_id`
- `seq`
- `ts`
- `payload`

Reserved for future compatibility:

- `auth`
- `device`
- `trace`

## Transport Direction

- Client to server: session lifecycle start, audio uplink, text, image, client end.
- Server to client: response lifecycle, streamed text, streamed audio, policy end, error.

## First Transport Mapping

- Control plane: JSON frames.
- Audio plane: binary WebSocket frames.
- Session metadata: JSON envelope with stable event names.

## Wire Profiles

- `rtos-ws-v0`: first RTOS-oriented profile over WebSocket.
- Future browser and channel profiles must preserve these event names even if their framing differs.
