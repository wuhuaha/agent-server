# Realtime Session Protocol v0

## Intent

This contract defines the first shared event model for realtime sessions across RTOS devices, desktops, and future channel-driven realtime adapters.

This document defines the transport-neutral semantic model. The first concrete RTOS wire profile is documented in `docs/protocols/rtos-device-ws-v0.md`.

Client collaboration note:

- for a concrete capability-gated proposal that embedded clients can implement in parallel, see `docs/protocols/realtime-voice-client-collaboration-proposal-v0-zh-2026-04-16.md`
- for the field tables, retry policy, and ACK timing guidance that embedded teams can implement directly, see `docs/protocols/realtime-voice-client-implementation-guide-v0-zh-2026-04-16.md`
- the draft schema for those additive collaboration events lives at `schemas/realtime/voice-collaboration-v0-draft.schema.json`

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

## Compatibility State And Lane Substates

`session.update.payload.state` remains the required compatibility field in v0.

Servers may now additionally include:

- `input_state`: optional fine-grained input-lane state
- `output_state`: optional fine-grained output-lane state
- `accept_reason`: optional reason string when the server has accepted a new turn

Compatibility rules:

- older clients may continue to look only at `state`
- newer clients may use `input_state` and `output_state` for richer UX, but must still tolerate them being absent
- `state` is the compatibility-derived top-level view, not a second independent state machine

Current lane values used by the shared session core:

- `input_state`:
  - `idle`: no active session input lane
  - `active`: input is being accepted
  - `previewing`: the server is actively previewing or staging speech for endpointing or interruption decisions
  - `committed`: the current input turn has been accepted and handed off downstream
  - `closing`: session shutdown is in progress
- `output_state`:
  - `idle`: no active server output lane
  - `thinking`: the server accepted a turn and is preparing response output
  - `speaking`: text and or audio output is being streamed
  - `closing`: session shutdown is in progress

Current compatibility derivation:

- if `output_state=speaking`, then `state=speaking`
- else if `output_state=thinking`, then `state=thinking`
- else if the session is active, then `state=active`
- else `state=idle` or `state=closing` for lifecycle edges

## Required Behaviours

- Wakeup on the client can trigger `session.start`.
- Both client and server may send `session.end`.
- A reason must be supplied when ending a session.
- User speech during server output may trigger interruption arbitration.
- RTOS devices may use half-duplex fallback without changing the event envelope.

## Core Events

- `session.start`
- `session.update`
- `audio.in.append`
- `audio.in.commit`
- `text.in`
- `image.in`
- `input.speech.start`
- `input.preview`
- `input.endpoint`
- `response.start`
- `response.chunk`
- `audio.out.meta`
- `audio.out.chunk`
- `audio.out.started`
- `audio.out.mark`
- `audio.out.cleared`
- `audio.out.completed`
- `session.end`
- `error`

## Event Semantics

- `session.start`: opens a new realtime dialog after local wakeup or explicit user action.
- `session.update`: currently serves two roles:
  - client to server: optional interrupt hint with `payload.interrupt = true`
  - server to client: non-terminal session state changes such as `active`, `thinking`, `speaking`, plus optional lane fields like `input_state`, `output_state`, and acceptance hints like `accept_reason`
- `audio.in.append`: semantic name for inbound audio chunks from the client; the concrete wire profile defines the actual codec and framing details.
- `audio.in.commit`: indicates end of the current user turn, not necessarily end of session. It remains the baseline v0 compatibility boundary for audio input, although a runtime that advertises `server_endpoint.enabled=true` may accept the turn earlier without waiting for it.
- `text.in`: sends text input on the same session when typing fallback is available.
- `image.in`: sends an image reference or attachment metadata on the same session.
- `input.speech.start`: optional server -> client observational event that indicates the shared preview path has detected speech start for the current preview window.
- `input.preview`: optional server -> client observational partial-update event for preview-aware clients.
- `input.endpoint`: optional server -> client observational endpoint-candidate event for preview-aware clients.
- `response.start`: begins a server response turn.
- `response.chunk`: streams partial text or structured deltas such as `text`, `tool_call`, or `tool_result`.
- `audio.out.meta`: optional server -> client playback-metadata event that establishes the IDs used by later playback ACK facts.
- `audio.out.chunk`: semantic name for outbound server audio chunks.
- `audio.out.started`: optional client -> server fact that local playback for the referenced segment actually started.
- `audio.out.mark`: optional client -> server progress fact for a referenced playback segment.
- `audio.out.cleared`: optional client -> server fact that queued audio after a segment was cleared locally.
- `audio.out.completed`: optional client -> server fact that referenced playback fully completed locally.
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
- the top-level `turn_mode` does not yet switch to a server-owned turn-finalization contract, even if discovery also exposes the additive `server_endpoint` candidate object

Current discovery compatibility note:

- discovery may additionally expose a structured `server_endpoint` object
- discovery may additionally expose a structured `voice_collaboration` object for preview-aware and playback-truth-aware collaboration
- that object advertises whether shared server endpointing is now a main-path candidate on the current runtime
- even when `server_endpoint.enabled=true`, explicit `audio.in.commit` remains a supported compatibility path in v0
- the optional collaboration events remain capability-gated: servers advertise them via discovery and clients opt into them through `session.start.payload.capabilities`
- preview observations still remain observational only; `accept_reason` stays the accepted-turn signal

When `server_endpoint.enabled=true`:

- the server may accept a spoken turn after shared preview plus silence plus lexical-hold logic without waiting for `audio.in.commit`
- that acceptance is still reported through the existing `session.update` plus `response.start` flow
- the accepted-turn `session.update` may include `accept_reason=server_endpoint`
- if `voice_collaboration.preview_events.enabled=true` and the client also negotiated `preview_events=true`, the server may additionally emit `input.speech.start`, `input.preview`, and `input.endpoint`
- clients must not treat those preview events as accepted-turn confirmation

## First Transport Mapping

- Control plane: JSON frames.
- Audio plane: binary WebSocket frames.
- Session metadata: JSON envelope with stable event names.

`response.start` must still precede the first `response.chunk`, but clients should treat `payload.modalities` as an early hint during runtime streaming rather than as an exhaustive declaration of every later output piece.

Current implementation note:

- on the native realtime main path, shared runtime orchestration may now start audio from an internal early-output stream before the final `TurnResponse` envelope has fully settled
- on the native realtime main path, capability-gated `audio.out.meta` plus client playback ACK events may now coexist with the existing `response.start` plus streamed deltas plus binary audio surface

Current optional tracing fields:

- `response.start.payload.turn_id`: server-assigned identifier for the current response turn
- `response.start.payload.trace_id`: server-assigned identifier for correlating logs and metrics for that turn
- `session.update.payload.turn_id`: may appear on server-emitted turn-state updates such as `thinking`, `speaking`, or the return to `active`

Clients should treat these fields as optional and ignore them if absent.

Compatibility note:

- `turn_id` is a correlation hint, not a standalone acceptance signal
- on preview-only `session.update` frames during `speaking`, `turn_id` may still refer to the in-flight server output turn rather than a newly accepted user turn

## `session.update` Server Payload

Server-emitted `session.update` keeps `state` for compatibility and may additionally include:

- `input_state`
- `output_state`
- `barge_in_enabled`
- `turn_id`
- `accept_reason`

Common `accept_reason` values in the current runtime:

- `audio_commit`: explicit `audio.in.commit`
- `server_endpoint`: shared server-endpoint path accepted the turn
- `text_input`: `text.in` created the turn directly
- a client-supplied or runtime-specific reason string may also appear for observability

Compatibility notes:

- `accept_reason` explains why a turn was accepted; it does not replace `state`
- a missing `accept_reason` means "no new turn acceptance is being declared in this update"
- clients must not fail if they see an unknown `accept_reason`
- clients must not treat `turn_id` alone as proof that a new user turn was accepted

Examples:

Session started and ready for input:

```json
{
  "type": "session.update",
  "session_id": "sess_01HQ...",
  "seq": 2,
  "ts": "2026-04-15T08:00:00Z",
  "payload": {
    "state": "active",
    "input_state": "active",
    "output_state": "idle",
    "barge_in_enabled": true
  }
}
```

Server-endpoint accepted the current audio turn:

```json
{
  "type": "session.update",
  "session_id": "sess_01HQ...",
  "seq": 12,
  "ts": "2026-04-15T08:00:02Z",
  "payload": {
    "state": "thinking",
    "input_state": "committed",
    "output_state": "thinking",
    "barge_in_enabled": true,
    "turn_id": "turn_01HQ...",
    "accept_reason": "server_endpoint"
  }
}
```

The server is still speaking while previewing new user speech:

```json
{
  "type": "session.update",
  "session_id": "sess_01HQ...",
  "seq": 19,
  "ts": "2026-04-15T08:00:04Z",
  "payload": {
    "state": "speaking",
    "input_state": "previewing",
    "output_state": "speaking",
    "barge_in_enabled": true,
    "turn_id": "turn_01HQ..."
  }
}
```

That last shape means:

- compatibility-only clients still see `state=speaking`
- richer clients may infer that new user speech is being previewed while output is still in progress
- the turn has not been accepted yet unless `accept_reason` is present or a later update moves the session into a committed lane

## Interruption Compatibility

The public v0 interruption surface is unchanged:

- the client may start sending new audio during `speaking`
- the client may additionally send `session.update { "interrupt": true }`
- the client may still finish the interrupting turn with `audio.in.commit`

The server may now internally classify an interruption attempt into richer policies such as:

- `ignore`
- `backchannel`
- `duck_only`
- `hard_interrupt`

Current implementation note:

- on the native realtime main path, `backchannel` and `duck_only` now apply real shared-output ducking on the current PCM16 playout path instead of remaining log-only
- that ducking behavior is still runtime-internal in v0; clients do not receive a separate public `duck` event

Compatibility rules:

- these policy names are internal runtime behavior in v0 and are not yet a required wire field
- clients must not assume the first new audio frame always causes an immediate hard stop of TTS
- the server may continue `state=speaking` while `input_state=previewing` until it decides whether to ignore, duck, or hard-interrupt
- once a hard interrupt is accepted, the server returns to the regular `session.update(state=active|thinking)` path and continues with the next accepted turn
- immediately after hard interrupt acceptance, the server commonly reports `output_state=idle` while `input_state` may still be `previewing` for a short overlap window before later settling back to `active` or moving into a committed turn

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
