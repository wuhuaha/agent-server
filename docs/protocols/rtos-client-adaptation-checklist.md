# RTOS Client Adaptation Checklist

## Purpose

This checklist is for firmware or RTOS client teams that want to connect directly to `agent-server` using the current `rtos-ws-v0` protocol.

It is intentionally precise about:

- websocket endpoint and headers
- each client-to-server and server-to-client event
- exact audio formats on uplink and downlink
- differences from the current `xiaozhi-esp32-server` wire behavior

Use this document together with:

- `docs/protocols/realtime-session-v0.md`
- `docs/protocols/rtos-device-ws-v0.md`

If you want to preserve the legacy `xiaozhi` event family and endpoint layout instead of moving to the native `rtos-ws-v0` contract, use `docs/protocols/xiaozhi-compat-ws-v0.md`.

## Scope

This checklist answers one question only:

- what must an RTOS client implement so it can establish a connection to `agent-server` and complete realtime voice interaction

It does not cover:

- OTA
- device binding
- manager-api
- MQTT or UDP gateway compatibility
- `xiaozhi` legacy event compatibility

## 1. Discovery Before Connect

Recommended first step:

- call `GET /v1/realtime`

The client should read and cache at least these response fields:

- `ws_path`
- `subprotocol`
- `protocol_version`
- `turn_mode`
- `input_audio.codec`
- `input_audio.sample_rate_hz`
- `input_audio.channels`
- `output_audio.codec`
- `output_audio.sample_rate_hz`
- `output_audio.channels`
- `capabilities.allow_opus`
- `capabilities.allow_text_input`
- `capabilities.allow_image_input`
- `idle_timeout_ms`
- `max_session_ms`
- `max_frame_bytes`

Current expected bootstrap values are usually:

- websocket path: `/v1/realtime/ws`
- subprotocol: `agent-server.realtime.v0`
- protocol version: `rtos-ws-v0`
- turn mode: `client_wakeup_client_commit`
- baseline uplink audio: `pcm16le / 16000 / mono`
- current downlink audio: `pcm16le / 16000 / mono`

## 2. WebSocket Connect

The RTOS client must:

1. open a websocket to `ws://<host>:<port><ws_path>` or `wss://<host>:<port><ws_path>`
2. send the header `Sec-WebSocket-Protocol: agent-server.realtime.v0`
3. keep exactly one active realtime session per websocket connection

Bootstrap auth status:

- no `Authorization` header is required right now

Important compatibility note:

- for the native `rtos-ws-v0` path, this is not the `xiaozhi` websocket path
- native clients should not connect to `/xiaozhi/v1/`
- if you are intentionally using the compatibility adapter, connect to `/xiaozhi/v1/` and follow `docs/protocols/xiaozhi-compat-ws-v0.md`

## 3. Control Frame Envelope Rules

All control events are sent as UTF-8 JSON text frames with this envelope shape:

```json
{
  "type": "session.start",
  "session_id": "sess_01hq_example",
  "seq": 1,
  "ts": "2026-03-31T09:00:00Z",
  "payload": {}
}
```

The client must follow these rules:

- `type`: required
- `seq`: required, sender-local monotonically increasing integer
- `ts`: required, RFC 3339 UTC timestamp
- `payload`: required, may be empty object

Recommended client behavior:

- always generate a local `session_id` on session start and reuse it for every later control event in that session

Binary audio frames:

- are not wrapped in JSON
- carry only audio payload bytes
- do not include a custom binary header

## 4. Client To Server Event Checklist

### 4.1 `session.start`

When to send:

- immediately after wakeup or explicit user action
- before the first binary audio frame

Minimum required payload fields:

- `payload.protocol_version`
- `payload.device.device_id`
- `payload.device.client_type`
- `payload.audio.codec`
- `payload.audio.sample_rate_hz`
- `payload.audio.channels`

Recommended full example:

```json
{
  "type": "session.start",
  "session_id": "sess_rtos_001",
  "seq": 1,
  "ts": "2026-03-31T09:00:00Z",
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
      "local_wake_word": true
    }
  }
}
```

### 4.2 Binary Audio Uplink

When to send:

- immediately after `session.start`
- continuously during the user turn

Rules:

- one websocket binary frame carries one audio chunk
- audio bytes are raw payload only
- no websocket text wrapper
- no extra 16-byte `xiaozhi` header

Exact format options are in section 6.

### 4.3 `audio.in.commit`

When to send:

- when local VAD, push-to-talk logic, or UI decides the user turn is finished

Required behavior:

- send this after the last uplink audio frame of the turn

Recommended example:

```json
{
  "type": "audio.in.commit",
  "session_id": "sess_rtos_001",
  "seq": 18,
  "ts": "2026-03-31T09:00:03Z",
  "payload": {
    "reason": "end_of_speech"
  }
}
```

### 4.4 Optional `text.in`

Use only if:

- the device supports typed or prebuilt text input
- `GET /v1/realtime` says `allow_text_input=true`

Example:

```json
{
  "type": "text.in",
  "session_id": "sess_rtos_001",
  "seq": 19,
  "ts": "2026-03-31T09:00:03Z",
  "payload": {
    "text": "tell me today's schedule"
  }
}
```

### 4.5 Optional `session.update` For Interrupt

Use only during barge-in:

- the client may send `session.update` with `payload.interrupt=true`
- this is optional
- the first new inbound audio frame during `speaking` is also treated as an interrupt signal

Example:

```json
{
  "type": "session.update",
  "session_id": "sess_rtos_001",
  "seq": 20,
  "ts": "2026-03-31T09:00:05Z",
  "payload": {
    "interrupt": true
  }
}
```

### 4.6 `session.end`

When to send:

- user stops the dialog
- device is going to sleep
- network is shutting down cleanly

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
  "session_id": "sess_rtos_001",
  "seq": 25,
  "ts": "2026-03-31T09:00:10Z",
  "payload": {
    "reason": "client_stop",
    "message": "user pressed stop"
  }
}
```

## 5. Server To Client Event Checklist

### 5.1 `session.update`

The client must handle these state transitions:

- `active`
- `thinking`
- `speaking`

Optional hint fields the client should tolerate:

- `barge_in_enabled`
- `turn_id`
- future unknown fields

Minimum example:

```json
{
  "type": "session.update",
  "session_id": "sess_rtos_001",
  "seq": 2,
  "ts": "2026-03-31T09:00:00Z",
  "payload": {
    "state": "active"
  }
}
```

### 5.2 `response.start`

Meaning:

- one server reply turn is starting

Current required fields:

- `payload.response_id`
- `payload.modalities`

Important rule:

- `modalities` is an early hint, not a strict guarantee
- the client must tolerate the server finally sending only text, only audio, or both

### 5.3 `response.chunk`

The client must parse:

- `payload.response_id`
- `payload.delta_type`

Current supported `delta_type` values:

- `text`
- `tool_call`
- `tool_result`

For `delta_type=text`, read:

- `payload.text`

For `delta_type=tool_call` and `tool_result`, read when present:

- `payload.tool_call_id`
- `payload.tool_name`
- `payload.tool_status`
- `payload.tool_input`
- `payload.tool_output`

Forward-compatibility rule:

- unknown `delta_type` values must be ignored rather than treated as fatal errors

### 5.4 Binary Audio Downlink

Server binary websocket frames are the realtime audio downlink.

Current exact implementation:

- codec: `pcm16le`
- sample rate: `16000`
- channels: `1`
- typical frame duration: `20 ms`
- typical payload size: `640` bytes per binary frame
- no WAV header
- no Opus wrapper
- no custom binary header

This is the most important client-side adaptation point.

If the current RTOS firmware only supports:

- `opus` decode
- `24000 Hz` playback
- or `xiaozhi`-style framed binary payloads

then it is not yet compatible with current `agent-server` downlink without firmware work.

### 5.5 `session.end`

The client must handle both server-initiated and client-initiated end.

Current server reasons:

- `completed`
- `idle_timeout`
- `max_duration`
- `server_stop`
- `error`

The client should:

1. stop local playback or capture
2. mark session closed
3. avoid sending more turn events on that session

### 5.6 `error`

The client must surface:

- `payload.code`
- `payload.message`
- `payload.recoverable`

Recommended behavior:

- if `recoverable=false`, close the current session state locally
- if `recoverable=true`, allow retry according to product logic

## 6. Exact Audio Format Requirements

## 6.1 Uplink Baseline: `pcm16le / 16000 / mono`

The simplest supported path is:

- signed 16-bit little-endian PCM
- sample rate `16000 Hz`
- channel count `1`
- raw PCM bytes only

Recommended realtime chunking:

- `20 ms` per websocket binary frame

Exact sizing:

- samples per 20 ms = `16000 * 0.02 = 320`
- bytes per sample = `2`
- channels = `1`
- bytes per frame = `320 * 2 * 1 = 640`

So the recommended baseline uplink binary frame is:

- `640` bytes of raw `pcm16le`

## 6.2 Uplink Optional: `opus`

Use this only if `GET /v1/realtime` reports `allow_opus=true`.

The client must also advertise:

- `payload.audio.codec = "opus"` in `session.start`

Current accepted Opus constraints:

- mono only
- speech-oriented `SILK-only` packets only
- one binary frame contains one encoded packet or one packet bundle
- recommended frame duration `20-60 ms`
- maximum accepted packet duration `120 ms`

Currently rejected by the server:

- stereo packets
- `CELT-only`
- hybrid mode packets

Server-side behavior:

- accepted Opus uplink is normalized in Go to `pcm16le / 16000 / mono` before ASR

## 6.3 Downlink Current Implementation: `pcm16le / 16000 / mono`

Current server downlink is not Opus.

The RTOS client must be able to play:

- raw `pcm16le`
- `16000 Hz`
- `1` channel

Current pacing:

- approximately `20 ms` chunks

Typical size:

- `640` bytes per binary websocket frame

Compatibility warning:

- if the existing firmware playback pipeline assumes `opus / 24000 / mono`, adaptation is mandatory

## 7. Turn And State Handling

Recommended client-side state machine:

1. websocket connected, no active session
2. local wakeup or explicit user action
3. send `session.start`
4. wait for or tolerate early `session.update(state=active)`
5. stream uplink audio
6. send `audio.in.commit`
7. receive `session.update(state=thinking)`
8. receive `response.start`
9. receive one or more `response.chunk`
10. receive zero or more binary audio frames while `speaking`
11. either:
    - continue with another user turn on same session
    - or receive/send `session.end`

Current timeout behavior published by the server:

- idle timeout applies only while the session is `active`
- idle timeout does not run while the server is `thinking` or `speaking`
- max session duration applies to the full session lifetime

## 8. Barge-In And Stop Behavior

Current `agent-server` barge-in model:

1. while the server is `speaking`, the client starts sending new audio immediately
2. the client may also send `session.update { "interrupt": true }`
3. the client ends the new turn with `audio.in.commit`

Important compatibility note:

- there is no legacy `abort` event in the current `rtos-ws-v0` protocol

So a client adapted from `xiaozhi` must not rely on:

- `abort`
- `listen start`
- `listen stop`
- `listen detect`

for the first `agent-server` integration.

## 9. Differences From `xiaozhi-esp32-server`

This section applies only when you are adapting a client to the native `rtos-ws-v0` wire contract.

If you are using the `xiaozhi` compatibility adapter, these differences are intentionally narrowed by `docs/protocols/xiaozhi-compat-ws-v0.md`.

An RTOS client adapted from the current `xiaozhi` stack must account for these differences:

- websocket path is `/v1/realtime/ws`, not `/xiaozhi/v1/`
- session start event is `session.start`, not `hello`
- end-of-turn event is `audio.in.commit`, not `listen stop`
- interrupt uses new audio and optional `session.update interrupt=true`, not `abort`
- server text or tool progress is carried by `response.chunk`
- server audio downlink is currently raw `pcm16le/16000/mono`, not the `xiaozhi` default Opus downlink model
- websocket binary audio frames have no extra `xiaozhi` 16-byte header
- OTA and vision endpoints are not required to establish the realtime websocket session

## 10. Final Adaptation Checklist

If minimal firmware churn is more important than moving to the native contract immediately, stop here and use the compatibility adapter document instead.


- open `GET /v1/realtime` and honor advertised websocket path, subprotocol, audio defaults, and capability flags
- connect to `/v1/realtime/ws` with `Sec-WebSocket-Protocol: agent-server.realtime.v0`
- send JSON control events with `type`, `seq`, `ts`, and `payload`
- send `session.start` before the first binary audio frame
- uplink audio in either:
  - `pcm16le / 16000 / mono / 20 ms / 640 bytes`
  - or supported mono `opus` if `allow_opus=true`
- send `audio.in.commit` at end of every user turn
- parse `session.update`, `response.start`, `response.chunk`, `session.end`, and `error`
- ignore unknown `response.chunk.payload.delta_type` values safely
- play binary downlink audio as raw `pcm16le / 16000 / mono`
- support server-initiated close for `completed`, `idle_timeout`, `max_duration`, and `error`
- support barge-in by sending new uplink audio during `speaking`

## 11. Minimum Smoke Test Cases

Before claiming compatibility, the RTOS client should pass all of these:

1. normal voice turn
   - `session.start -> binary audio -> audio.in.commit -> response.start -> response.chunk -> binary audio`
2. server-initiated end
   - client receives `session.end(reason=completed)` and closes the local session cleanly
3. idle timeout
   - client receives `session.end(reason=idle_timeout)` after inactivity in `active`
4. max duration
   - client receives `session.end(reason=max_duration)` on long-lived sessions
5. barge-in
   - client interrupts server `speaking` with new audio and completes the new turn successfully
6. optional Opus uplink
   - if enabled by discovery, client sends mono speech-oriented Opus and gets a normal reply
