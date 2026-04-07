# Realtime Session Protocol v0

## Intent

This contract defines the first shared event model for realtime sessions across RTOS devices, desktops, and future channel-driven realtime adapters.

This document defines the transport-neutral semantic model. The first concrete RTOS wire profile is documented in `docs/protocols/rtos-device-ws-v0.md`.

## Session States

- `idle`: no active session.
- `active`: audio or text input is being accepted.
- `thinking`: the server is preparing a reply.
- `speaking`: the server is streaming TTS or text output.
- `closing`: the session is ending and no new turn should start.

Current implementation note:

- the current server emits `session.update` with `active`, `thinking`, and `speaking`
- `idle` and `closing` remain valid lifecycle states inside the session core, but are not currently emitted as steady-state server updates
- the bootstrap profile does not currently publish or emit an `armed` state

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
- `session.update`: currently serves two roles:
  - client to server: optional interrupt hint with `payload.interrupt = true`
  - server to client: non-terminal session state changes such as `active`, `thinking`, `speaking`, plus hints like `barge_in_enabled`
- `audio.in.append`: semantic name for inbound audio chunks from the client; the concrete wire profile defines the actual codec and framing details.
- `audio.in.commit`: indicates end of the current user turn, not necessarily end of session. In the current bootstrap implementation, this is the required turn-finalization boundary for audio input.
- `text.in`: sends text input on the same session when typing fallback is available.
- `image.in`: sends an image reference or attachment metadata on the same session.
- `response.start`: begins a server response turn.
- `response.chunk`: streams partial text or structured deltas such as `text`, `tool_call`, or `tool_result`.
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
- Server to client: response lifecycle, streamed text or tool deltas, streamed audio, policy end, error.

Current turn-taking mode advertised by discovery is `client_wakeup_client_commit`:

- the client wake word or explicit user action opens the session
- the client ends each audio turn with `audio.in.commit`
- the server does not yet advertise a server-side VAD turn-finalization path

## First Transport Mapping

- Control plane: JSON frames.
- Audio plane: binary WebSocket frames.
- Session metadata: JSON envelope with stable event names.

`response.start` must still precede the first `response.chunk`, but clients should treat `payload.modalities` as an early hint during runtime streaming rather than as an exhaustive declaration of every later output piece.

## `response.chunk` Delta Payload

The first protocol version keeps one `response.chunk` event name and uses payload fields to describe the streamed delta kind.

Common fields:

- `response_id`
- `delta_type`: one of `text`, `tool_call`, or `tool_result`

Text delta fields:

- `text`

Tool delta fields:

- `tool_call_id`
- `tool_name`
- `tool_status`
- `tool_input`
- `tool_output`

`tool_input` and `tool_output` currently carry serialized JSON strings when the runtime needs to expose structured tool arguments or results without changing the envelope shape.

A receiver should tolerate unknown `delta_type` values for forward compatibility.

## Wire Profiles

- `rtos-ws-v0`: first RTOS-oriented profile over WebSocket.
- The current browser or H5 direct reference client also reuses `rtos-ws-v0`; browser-side PCM16 capture and playback adaptation happens in the page instead of introducing a second browser-only event family.
- Future browser and channel profiles must preserve these event names even if their framing differs.
