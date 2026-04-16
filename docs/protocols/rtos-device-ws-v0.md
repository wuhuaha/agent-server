# RTOS Device WebSocket Profile v0

## Status

This is the first concrete device-facing wire profile for RTOS and lightweight native clients.

- Profile ID: `rtos-ws-v0`
- Transport: `WebSocket`
- WebSocket subprotocol: `agent-server.realtime.v0`
- Auth mode for bootstrap: `disabled`
- Session concurrency per connection: exactly one active session

This profile is intentionally simple enough for ESP32-class or similar devices to implement first, while still leaving room for future auth and richer capabilities.

The bootstrap WebSocket handler is implemented in the repository for the first bring-up path. Local ASR can now be backed by the Python `FunASR` worker without changing the device-facing protocol. The current directly usable stack is:

- ASR: local `FunASR` worker
- uplink codecs: baseline `pcm16le`, optional speech-oriented `opus` normalized to `pcm16le/16000/mono` before ASR
- TTS: Xiaomi `mimo-v2-tts` with streaming `pcm16` for the realtime path
- output pacing: `20 ms` PCM frames over the same WebSocket

For browser or H5 direct bring-up against the same native profile, the service also exposes a built-in debug page at `/debug/realtime-h5/`. That page still talks to `GET /v1/realtime` and `/v1/realtime/ws`; it does not introduce a second browser-only wire contract.

Client collaboration note:

- for a concrete capability-gated proposal that embedded clients can implement in parallel, see `docs/protocols/realtime-voice-client-collaboration-proposal-v0-zh-2026-04-16.md`
- for the embedded implementation tables, retry strategy, and ACK timing guidance, see `docs/protocols/realtime-voice-client-implementation-guide-v0-zh-2026-04-16.md`
- the draft schema for those additive collaboration events lives at `schemas/realtime/voice-collaboration-v0-draft.schema.json`

## Goals

- Wake word hit on the client can immediately open a session.
- The client can stream microphone audio with minimal framing overhead.
- The server can stream TTS audio back on the same socket.
- Either side can end the session at any time.
- The same semantic session model can later be reused by non-RTOS clients.

## Discovery

Before opening the WebSocket, a client may fetch:

- `GET /v1/realtime`

The response publishes the current wire profile, WebSocket path, subprotocol, audio defaults, capability flags such as `allow_opus`, timeout policy, and links to the current schema and protocol documents.

Current discovery note:

- `turn_mode` is currently advertised as `client_wakeup_client_commit`
- this means the device starts the session after local wakeup or explicit action, then ends each user audio turn with `audio.in.commit`
- discovery may now also publish a `server_endpoint` object that marks shared server endpointing as a main-path candidate
- discovery may now also publish a `voice_collaboration` object that advertises preview-aware events and playback ACK support
- if that candidate object reports `enabled=true`, the server may auto-accept a spoken turn after shared preview, silence, and lexical-hold detection, but devices may still keep explicit `audio.in.commit` for compatibility
- the collaboration events remain additive and capability-gated: the device must both read discovery and opt in through `session.start.payload.capabilities`
- when negotiated, preview speech-start, partial-update, and endpoint-candidate observations may be sent as `input.speech.start`, `input.preview`, and `input.endpoint`
- playback-truth-aware devices may also receive `audio.out.meta` and send back `audio.out.started`, `audio.out.mark`, `audio.out.cleared`, and `audio.out.completed`

## Connection

### Endpoint

- `ws://<host>:<port>/v1/realtime/ws`
- `wss://<host>:<port>/v1/realtime/ws`

### Required Headers

- `Sec-WebSocket-Protocol: agent-server.realtime.v0`

### Authentication

For the bootstrap phase, no token is required.

Reserved for future versions:

- `Authorization`
- device proof or nonce fields inside `session.start`

## Framing Model

This profile uses two WebSocket message types:

- text frames: UTF-8 JSON control events
- binary frames: raw PCM audio chunks or encoded `opus` packets, depending on `session.start.audio.codec`

Rule:

- client binary frame => semantic event `audio.in.append`
- server binary frame => semantic event `audio.out.chunk`

There is no extra binary header in `rtos-ws-v0`. Each binary WebSocket message belongs to the current active session on that socket.

## Session Lifecycle

### 1. Idle Connection

The client may keep the socket open before wakeup. No audio is sent while idle.

### 2. Session Start

When the local wake word or explicit user action fires, the client sends `session.start`.

Current capability note:

- devices that support preview-aware collaboration may additionally declare `capabilities.preview_events=true`
- devices that support playback-truth-aware collaboration may additionally declare `capabilities.playback_ack.mode=segment_mark_v1`

### 3. Audio Uplink

The client streams binary audio frames immediately after `session.start`. For `pcm16le`, each binary frame carries raw PCM bytes. For `opus`, each binary frame carries one encoded packet or packet bundle and the server normalizes it before ASR.

### 4. Turn Accept Or Client Commit

Baseline compatibility path:

- when local VAD or UI logic decides the user turn is complete, the client sends `audio.in.commit`

Server-endpoint candidate path:

- if discovery reports `server_endpoint.enabled=true`, the server may accept the current audio turn without waiting for `audio.in.commit`
- the accepted-turn `session.update` then carries the usual `state` compatibility field and may additionally include `accept_reason=server_endpoint`
- devices should still support explicit `audio.in.commit` until the candidate path graduates further

### 5. Server Response

The server may emit:

- `session.update`
- `input.speech.start` when preview-aware collaboration is negotiated
- `input.preview` when preview-aware collaboration is negotiated
- `input.endpoint` when preview-aware collaboration is negotiated
- `response.start`
- `response.chunk`
- `audio.out.meta` when playback ACK collaboration is negotiated
- binary audio frames

The device may additionally emit during local playback:

- `audio.out.started`
- `audio.out.mark`
- `audio.out.cleared`
- `audio.out.completed`

### 6. Session End

Either side may send `session.end` with a reason. After sending `session.end`, no new turn starts on that socket until a new `session.start`.

## Control Event Envelope

All JSON control events use the shared envelope:

```json
{
  "type": "session.start",
  "session_id": "sess_01HQ...",
  "seq": 1,
  "ts": "2026-03-25T08:00:00Z",
  "payload": {}
}
```

### Required Fields

- `type`: event name
- `seq`: sender-local monotonically increasing sequence number
- `ts`: RFC 3339 UTC timestamp

### Recommended Fields

- `session_id`: required after session creation; may be omitted by the client on the first `session.start`
- `device`: device metadata
- `trace`: trace or correlation information

## Client To Server Events

### `session.start`

Purpose:

- opens a new realtime dialog
- declares device and audio capabilities
- communicates wakeup context

Minimum payload:

```json
{
  "type": "session.start",
  "seq": 1,
  "ts": "2026-03-25T08:00:00Z",
  "payload": {
    "protocol_version": "rtos-ws-v0",
    "device": {
      "device_id": "esp32-dev-001",
      "client_type": "rtos",
      "firmware_version": "0.1.0"
    },
    "audio": {
      "codec": "pcm16le",
      "sample_rate_hz": 16000,
      "channels": 1
    },
    "session": {
      "mode": "voice",
      "wake_reason": "keyword",
      "client_can_end": true,
      "server_can_end": true
    },
    "capabilities": {
      "text_input": false,
      "image_input": false,
      "half_duplex": true,
      "local_wake_word": true,
      "preview_events": true,
      "playback_ack": {
        "mode": "segment_mark_v1"
      }
    }
  }
}
```

Compatibility note:

- `preview_events` and `playback_ack` are optional capability-gated extensions
- devices must send them only after discovery reports the corresponding `voice_collaboration` support

### `audio.in.commit`

Purpose:

- marks the end of the current user turn
- asks the server to flush ASR and begin reply generation

Example:

```json
{
  "type": "audio.in.commit",
  "session_id": "sess_01HQ...",
  "seq": 18,
  "ts": "2026-03-25T08:00:03Z",
  "payload": {
    "reason": "end_of_speech"
  }
}
```

### `text.in`

Optional fallback for hybrid devices:

```json
{
  "type": "text.in",
  "session_id": "sess_01HQ...",
  "seq": 19,
  "ts": "2026-03-25T08:00:03Z",
  "payload": {
    "text": "tell me today's schedule"
  }
}
```

Bootstrap debug behaviour:

- if the text input is `/end`, `bye`, `goodbye`, `结束`, or `结束对话`, the bootstrap handler may reply once and then send a server-initiated `session.end(reason=completed)`.

### `session.end`

Purpose:

- terminates the dialog from the client side

Required payload:

- `reason`

Allowed client reasons:

- `client_stop`
- `wake_cancelled`
- `device_sleep`
- `network_shutdown`
- `error`

Example:

```json
{
  "type": "session.end",
  "session_id": "sess_01HQ...",
  "seq": 25,
  "ts": "2026-03-25T08:00:10Z",
  "payload": {
    "reason": "client_stop",
    "message": "user pressed stop"
  }
}
```

## Server To Client Events

### `session.update`

Used for session state changes or interaction hints.

Example:

```json
{
  "type": "session.update",
  "session_id": "sess_01HQ...",
  "seq": 2,
  "ts": "2026-03-25T08:00:00Z",
  "payload": {
    "state": "active"
  }
}
```

Common server states:

- `active`
- `thinking`
- `speaking`

Compatibility rules:

- `state` remains the required top-level compatibility field
- newer servers may additionally include `input_state` and `output_state`
- older clients may ignore those lane fields and continue to drive UX from `state` only
- `turn_id` is an optional correlation field and must not be treated by itself as proof that a new user turn was accepted

Optional fields:

- `input_state`
- `output_state`
- `barge_in_enabled`
- `turn_id`
- `accept_reason`

Current lane meanings:

- `input_state=active`: the server is accepting user input
- `input_state=previewing`: the server is previewing speech for server-endpoint or interruption decisions
- `input_state=committed`: the current user turn has been accepted
- `output_state=idle`: no active reply output is streaming
- `output_state=thinking`: the accepted turn is being prepared for reply generation
- `output_state=speaking`: output is being streamed

Current `accept_reason` usage:

- `audio_commit`: explicit `audio.in.commit`
- `server_endpoint`: shared server endpointing accepted the turn
- `text_input`: `text.in` created the turn
- a runtime-specific or client-supplied reason string may also appear

Accepted-turn example:

```json
{
  "type": "session.update",
  "session_id": "sess_01HQ...",
  "seq": 9,
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

Speaking-time preview example:

```json
{
  "type": "session.update",
  "session_id": "sess_01HQ...",
  "seq": 14,
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

That means the server is still speaking for compatibility, but it has already started previewing new inbound speech. The turn is not accepted until a later update includes `accept_reason` or otherwise moves into a committed lane.

### `response.start`

Marks the beginning of one server reply turn. During streamed runtime execution, `modalities` is an early hint and clients must tolerate the server ultimately sending only a subset of those modalities.

```json
{
  "type": "response.start",
  "session_id": "sess_01HQ...",
  "seq": 3,
  "ts": "2026-03-25T08:00:03Z",
  "payload": {
    "response_id": "resp_01HQ...",
    "modalities": ["audio", "text"]
  }
}
```

### `response.chunk`

Streams partial text or structured deltas.

Text delta example:

```json
{
  "type": "response.chunk",
  "session_id": "sess_01HQ...",
  "seq": 4,
  "ts": "2026-03-25T08:00:03Z",
  "payload": {
    "response_id": "resp_01HQ...",
    "delta_type": "text",
    "text": "好的，我来为你查询"
  }
}
```

Tool call delta example:

```json
{
  "type": "response.chunk",
  "session_id": "sess_01HQ...",
  "seq": 5,
  "ts": "2026-03-25T08:00:03Z",
  "payload": {
    "response_id": "resp_01HQ...",
    "delta_type": "tool_call",
    "tool_call_id": "tool_01HQ...",
    "tool_name": "calendar.lookup",
    "tool_status": "started",
    "tool_input": "{\"date\":\"2026-03-31\"}"
  }
}
```

Tool result delta example:

```json
{
  "type": "response.chunk",
  "session_id": "sess_01HQ...",
  "seq": 6,
  "ts": "2026-03-25T08:00:03Z",
  "payload": {
    "response_id": "resp_01HQ...",
    "delta_type": "tool_result",
    "tool_call_id": "tool_01HQ...",
    "tool_name": "calendar.lookup",
    "tool_status": "completed",
    "tool_output": "{\"events\":1}"
  }
}
```

`tool_input` and `tool_output` currently carry serialized JSON strings when structured tool arguments or results need to cross the wire without changing the shared envelope shape.

Current delta kinds:

- `text`
- `tool_call`
- `tool_result`

Clients should ignore unknown `delta_type` values so newer servers can add more structured deltas without breaking the socket contract.

### `session.end`

Required payload:

- `reason`

Allowed server reasons:

- `completed`
- `idle_timeout`
- `max_duration`
- `server_stop`
- `error`

Bootstrap debug policy:

- the current placeholder implementation may use `reason=completed` after a text debug turn that explicitly asks to end the dialog.

Example:

```json
{
  "type": "session.end",
  "session_id": "sess_01HQ...",
  "seq": 12,
  "ts": "2026-03-25T08:00:12Z",
  "payload": {
    "reason": "completed",
    "message": "dialog finished"
  }
}
```

### `error`

Used for protocol or runtime failures.

Example:

```json
{
  "type": "error",
  "session_id": "sess_01HQ...",
  "seq": 13,
  "ts": "2026-03-25T08:00:12Z",
  "payload": {
    "code": "unsupported_codec",
    "message": "codec opus is not enabled on this server",
    "recoverable": false
  }
}
```

## Binary Audio Rules

### Baseline Required Profile

The first mandatory baseline profile for device bring-up is:

- codec: `pcm16le`
- sample rate: `16000`
- channels: `1`

Recommended chunking:

- 20 ms per binary WebSocket message
- 640 bytes per frame for `pcm16le/16k/mono`

### Optional Profile

Optional when enabled by server config:

- codec: `opus`

For `opus`, one binary frame should contain one encoded packet or one packet bundle. The current server implementation accepts mono speech-oriented `SILK-only` packets, decodes them in Go, and normalizes them to `pcm16le/16000/mono` before calling ASR. Packets up to `120 ms` are accepted by the decoder, but `20-60 ms` remains the recommended realtime range. Hybrid, `CELT-only`, or stereo packets are currently rejected.

## Barge-In

When the server is in `speaking`, the client may interrupt.

Client behaviour:

1. start sending new inbound binary audio frames immediately
2. optionally send `session.update` with `payload.interrupt = true`
3. send `audio.in.commit` when the interrupting user turn ends

Server behaviour:

1. start previewing the new input while deciding whether to ignore, duck, or hard-interrupt the current output
2. if a hard interrupt is accepted, stop the current TTS downlink and return the output lane to `idle`
3. process the new accepted user turn through the same `session.update -> response.start -> response.chunk -> audio` flow

Current compatibility note:

- the public client contract is still the same: new audio and optional `session.update { "interrupt": true }`
- the server may internally classify an interruption attempt as `ignore`, `backchannel`, `duck_only`, or `hard_interrupt`
- those policy names are not yet required wire fields in `rtos-ws-v0`
- clients therefore must not assume that the first interrupting frame always stops TTS immediately
- preview-related observations may advance on the server before any accepted-turn update is emitted

Current implemented policy:

- first interrupt signal can be either:
  - the first new inbound binary audio frame while the session is `speaking`
  - `session.update` with `payload.interrupt = true`
- before a hard interrupt is accepted, the server may emit `session.update(state=speaking, input_state=previewing, output_state=speaking)` while it previews the new speech
- after a hard interrupt is accepted, the server sends `session.update(state=active, output_state=idle, barge_in_enabled=true)`; `input_state` may still be `previewing` briefly if the interrupting audio is still staged, or `active` once buffered audio has been ingested back into the main input lane
- a later accepted-turn update may then move to `state=thinking` with `accept_reason=audio_commit`, `accept_reason=server_endpoint`, or another runtime-defined reason

## Timeout Policy

Bootstrap default policy:

- idle timeout: 15 seconds without turn activity
- max session duration: 5 minutes

These values are runtime-configurable and published by `GET /v1/realtime`.

Current implemented policy details:

- idle timeout is enforced only while the session state is `active`
- `thinking` and `speaking` do not trigger idle timeout
- max session duration applies across the whole session lifetime
- on timeout, the server sends `session.end` with:
  - `reason=idle_timeout`
  - or `reason=max_duration`

## Minimal RTOS Client Checklist

To qualify as `rtos-ws-v0` compatible, a client must:

1. open a WebSocket with subprotocol `agent-server.realtime.v0`
2. send `session.start` after wakeup
3. stream `pcm16le/16k/mono` audio as binary frames, or stream supported `opus` packets after advertising `audio.codec=opus` in `session.start`
4. send `audio.in.commit` after end-of-speech as the baseline compatibility path, even if discovery also reports `server_endpoint.enabled=true`
5. receive `response.start`, `response.chunk`, and binary TTS audio
6. support both client-initiated and server-initiated `session.end`

## Example Sequence

```text
Client -> Server : session.start
Client -> Server : binary audio frame
Client -> Server : binary audio frame
Client -> Server : audio.in.commit
Server -> Client : session.update(state=thinking, input_state=committed, output_state=thinking, accept_reason=audio_commit)
Server -> Client : response.start
Server -> Client : response.chunk("好的")
Server -> Client : binary audio frame
Server -> Client : binary audio frame
Server -> Client : session.end(reason=completed)
```
