# Xiaozhi Compatibility WebSocket Adapter v0

## Purpose

This document describes the `xiaozhi` compatibility adapter exposed by `agent-server` for existing firmware that already speaks the `xiaozhi-esp32` WebSocket and OTA shape.

Use this adapter when you want current `xiaozhi` firmware to connect to `agent-server` with minimal changes.

Use the native `rtos-ws-v0` contract instead when you are building a new RTOS client directly against `agent-server`.

Related references:

- `docs/protocols/rtos-client-adaptation-checklist.md`
- `docs/protocols/realtime-session-v0.md`
- `schemas/xiaozhi/compat-message.schema.json`

## Endpoints

When `AGENT_SERVER_XIAOZHI_ENABLED=true`, the server mounts:

- OTA discovery: `POST /xiaozhi/ota/`
- WebSocket session: `/xiaozhi/v1/`

Default environment values:

```bash
AGENT_SERVER_XIAOZHI_ENABLED=true
AGENT_SERVER_XIAOZHI_WS_PATH=/xiaozhi/v1/
AGENT_SERVER_XIAOZHI_OTA_PATH=/xiaozhi/ota/
AGENT_SERVER_XIAOZHI_INPUT_CODEC=opus
AGENT_SERVER_XIAOZHI_INPUT_SAMPLE_RATE=16000
AGENT_SERVER_XIAOZHI_INPUT_CHANNELS=1
AGENT_SERVER_XIAOZHI_INPUT_FRAME_DURATION_MS=60
AGENT_SERVER_XIAOZHI_OUTPUT_CODEC=opus
AGENT_SERVER_XIAOZHI_OUTPUT_SAMPLE_RATE=24000
AGENT_SERVER_XIAOZHI_OUTPUT_CHANNELS=1
AGENT_SERVER_XIAOZHI_OUTPUT_FRAME_DURATION_MS=60
```

The adapter still forwards turns through the shared realtime session core and runtime-owned responder path. It is only a device compatibility layer, not a second orchestration stack.

## Recommended Bring-Up Path

For an existing `xiaozhi` firmware port, use this exact order:

1. enable the compatibility adapter and keep shared realtime output on `pcm16le`
2. verify OTA discovery returns the expected websocket URL
3. connect websocket with the existing `xiaozhi` headers
4. send `hello`
5. open a turn with `listen.start`
6. stream binary audio frames
7. end the turn with `listen.stop`
8. wait for `tts.start -> optional tts.sentence_start -> binary audio -> tts.stop`

Do not start by modifying the native `rtos-ws-v0` client checklist if the goal is minimum firmware churn. This adapter exists specifically so existing `hello / listen / abort` clients can reach the shared session core first.

## Server-Side Preparation

At minimum, enable:

```bash
AGENT_SERVER_XIAOZHI_ENABLED=true
AGENT_SERVER_XIAOZHI_WS_PATH=/xiaozhi/v1/
AGENT_SERVER_XIAOZHI_OTA_PATH=/xiaozhi/ota/

AGENT_SERVER_REALTIME_OUTPUT_CODEC=pcm16le
AGENT_SERVER_REALTIME_OUTPUT_SAMPLE_RATE=16000
AGENT_SERVER_REALTIME_OUTPUT_CHANNELS=1
```

Why `AGENT_SERVER_REALTIME_OUTPUT_CODEC=pcm16le` matters:

- the compatibility adapter currently expects source runtime speech to be `pcm16le`
- it then re-encodes that source audio into the configured `xiaozhi` downlink profile
- if shared runtime output is not `pcm16le`, spoken replies will fail at the compatibility encoder boundary

Recommended first usable voice combinations:

- local path:
  - `AGENT_SERVER_VOICE_PROVIDER=funasr_http`
  - `AGENT_SERVER_TTS_PROVIDER=mimo_v2_tts`
- cloud path:
  - `AGENT_SERVER_VOICE_PROVIDER=iflytek_rtasr`
  - `AGENT_SERVER_TTS_PROVIDER=iflytek_tts_ws`
- mixed path:
  - `AGENT_SERVER_VOICE_PROVIDER=funasr_http`
  - `AGENT_SERVER_TTS_PROVIDER=volcengine_tts`

Operational prerequisites:

- `ffmpeg` must be available on the server for `xiaozhi` opus downlink encoding
- if using `opus` uplink, keep the firmware on the current mono speech path
- if using raw `pcm16le` uplink, keep the sample rate exactly aligned with `AGENT_SERVER_XIAOZHI_INPUT_SAMPLE_RATE`

## Detailed Bring-Up Checklist

### 1. Verify Base Service Health

Before touching firmware, verify:

- `GET /healthz` returns `200`
- `POST /xiaozhi/ota/` returns a websocket URL pointing at the expected host and path
- the service is started with `AGENT_SERVER_XIAOZHI_ENABLED=true`

Example:

```bash
curl -X POST http://127.0.0.1:8080/xiaozhi/ota/ \
  -H 'Content-Type: application/json' \
  -d '{"application":{"version":"test-fw"}}'
```

You should confirm:

- `websocket.url` is the URL your device can really reach
- `firmware.version` echoes the version your device sent
- reverse proxy deployments pass the correct `X-Forwarded-Host` and `X-Forwarded-Proto`

### 2. Connect WebSocket With Existing Firmware Headers

Recommended request headers:

- `Protocol-Version: 1|2|3`
- `Device-Id: <stable device id>`
- `Client-Id: <optional client id>`
- optional `Authorization: Bearer <token>`

Current compatibility behavior:

- handshake version falls back to `AGENT_SERVER_XIAOZHI_WELCOME_VERSION` if the header is absent
- `Device-Id` is preferred as the server-side device key
- `hello.device_id` or `hello.device_mac` can still update the device identity after connect

### 3. Send `hello` First

Do not rely on binary audio before `hello`.

Recommended first message:

```json
{
  "type": "hello",
  "version": 3,
  "transport": "websocket",
  "audio_params": {
    "format": "opus",
    "sample_rate": 16000,
    "channels": 1,
    "frame_duration": 60
  }
}
```

The adapter response confirms:

- negotiated protocol version
- `session_id`
- expected downlink audio format

If `audio_params` is omitted, the adapter falls back to:

- format: `opus`
- sample rate: `16000`
- channels: `1`
- frame duration: `60`

### 4. Open A Voice Turn Explicitly

Recommended sequence:

1. send `listen.state=start`
2. start pushing binary audio frames
3. send `listen.state=stop` when local VAD or button logic decides the turn is complete

Example:

```json
{
  "type": "listen",
  "session_id": "sess_xiaozhi_123",
  "state": "start",
  "mode": "auto"
}
```

Then later:

```json
{
  "type": "listen",
  "session_id": "sess_xiaozhi_123",
  "state": "stop"
}
```

Important implementation notes:

- `listen.start` lazily creates the shared realtime session if needed
- binary audio without a started turn should not be treated as the primary happy path
- `listen.stop` is the commit point that pushes the buffered turn into ASR/runtime execution

### 5. Expect This Spoken Reply Sequence

For a normal spoken reply, the current adapter emits:

1. `tts.start`
2. optional `tts.sentence_start`
3. one or more binary audio frames
4. `tts.stop`

If the runtime returned text but no audio:

- the adapter sends one fallback `llm` text message
- no binary audio is sent for that turn

### 6. Use `listen.detect` For Text Shortcut Or Wake Debug

`listen.detect` is the compatibility text-turn shortcut.

Example:

```json
{
  "type": "listen",
  "session_id": "sess_xiaozhi_123",
  "state": "detect",
  "text": "打开客厅灯"
}
```

Current response behavior:

- the adapter immediately echoes one `stt` event with the same text
- the request then flows through the same runtime path as a speech-completed turn
- spoken output still returns through `tts.* + binary audio` if TTS is available

### 7. Use `abort` Or New Audio For Barge-In

Current barge-in signals:

- `abort`
- new inbound binary audio while the server is speaking

When barge-in succeeds:

- the current output stream is interrupted
- the adapter emits `tts.stop`
- the session returns to `active`
- the next turn can begin immediately

## Binary Audio Framing Guide

Use binary frame type `0` only.

### Version 1

- websocket binary payload is raw audio only

### Version 2

```text
uint16 version        = 2
uint16 type           = 0
uint32 reserved       = 0
uint32 timestamp      = device-defined
uint32 payload_size   = N
bytes  payload        = audio bytes
```

### Version 3

```text
uint8  type           = 0
uint8  reserved       = 0
uint16 payload_size   = N
bytes  payload        = audio bytes
```

Current input expectations:

- default uplink: `opus / 16000 / mono / 60 ms`
- optional raw uplink: `pcm16le / 16000 / mono`

Current output expectations:

- default downlink: `opus / 24000 / mono / 60 ms`
- the server sends the same binary framing family as the client negotiated

## Firmware Work Items

To call a port `xiaozhi`-compat ready against current `agent-server`, the device should:

- call `POST /xiaozhi/ota/` or otherwise learn `/xiaozhi/v1/`
- send websocket headers `Protocol-Version` and `Device-Id`
- send `hello` before the first normal turn
- implement `listen.start`, `listen.stop`, and optionally `listen.detect`
- accept `tts.start`, `tts.sentence_start`, `tts.stop`
- play binary downlink audio in the negotiated `audio_params`
- support interruption via `abort` or new uplink audio

## Troubleshooting

### No `hello` response

Check:

- websocket path is really `/xiaozhi/v1/`
- proxy allows websocket upgrade headers through
- `AGENT_SERVER_XIAOZHI_ENABLED=true`

### `server` error right after `hello`

Common causes:

- unsupported `audio_params.format`
- non-mono input
- `pcm16le` sample rate does not match the configured input sample rate

### `listen.stop` produces no reply

Check:

- audio bytes actually arrived before `listen.stop`
- binary protocol version matches the framing format you sent
- `payload_size` inside version `2/3` headers is correct
- ASR provider is healthy and configured

### `tts.start` arrives but no usable audio plays

Check:

- server has `ffmpeg`
- `AGENT_SERVER_REALTIME_OUTPUT_CODEC=pcm16le`
- downlink decoder on the device matches the `hello` response `audio_params`

### Only `llm` text arrives, no spoken reply

This usually means one of:

- TTS provider is disabled
- TTS provider failed and the runtime fell back to text-only output
- the turn result itself contained no audio-capable response

## Validation Recommendation

Before claiming the firmware is integrated, confirm all of these in order:

1. OTA discovery returns the correct websocket URL
2. websocket connect succeeds
3. `hello` round-trip succeeds
4. one `listen.start -> binary audio -> listen.stop` turn returns `tts.start -> binary -> tts.stop`
5. one `listen.detect` text turn returns `stt` plus a spoken or text reply
6. one `abort` or barge-in test interrupts current speech cleanly

## OTA Discovery

`POST /xiaozhi/ota/` returns the currently configured WebSocket URL and echoes the firmware version the device reported.

Example request:

```json
{
  "application": {
    "version": "1.0.0"
  }
}
```

Example response:

```json
{
  "server_time": {
    "timestamp": 1774918800000,
    "timezone_offset": 480
  },
  "firmware": {
    "version": "1.0.0",
    "url": ""
  },
  "websocket": {
    "url": "ws://127.0.0.1:8080/xiaozhi/v1/",
    "token": ""
  }
}
```

Notes:

- the adapter currently does not implement firmware download distribution
- the OTA route is only used to advertise the WebSocket endpoint
- `GET /xiaozhi/ota/` returns a plain-text health hint for manual debugging

## WebSocket Connect

The client connects to `/xiaozhi/v1/` with the same headers current firmware already sends.

Common request headers:

- `Protocol-Version: 1|2|3`
- `Device-Id: <device id>`
- `Client-Id: <client id>`
- optional `Authorization: Bearer <token>`

The adapter accepts the protocol version from the handshake header and also honors `hello.version` when provided.

## Hello Handshake

Client to server:

```json
{
  "type": "hello",
  "version": 1,
  "transport": "websocket",
  "audio_params": {
    "format": "opus",
    "sample_rate": 16000,
    "channels": 1,
    "frame_duration": 60
  }
}
```

Server to client:

```json
{
  "type": "hello",
  "version": 1,
  "transport": "websocket",
  "session_id": "sess_xiaozhi_...",
  "audio_params": {
    "format": "opus",
    "sample_rate": 24000,
    "channels": 1,
    "frame_duration": 60
  }
}
```

Compatibility behavior:

- `audio_params` is accepted as optional for browser or debug clients
- if omitted, the adapter defaults to `opus / 16000 / mono / 60 ms` uplink assumptions
- only mono uplink is supported right now
- raw `pcm16le` uplink is accepted only at the configured input sample rate
- `listen.stop` audio turns now emit `stt` after ASR completes and before the reply path starts

## Client To Server Messages

### `hello`

Starts capability negotiation and tells the server which binary protocol version and audio profile to expect.

### `listen`

Supported states:

- `start`: open or resume an audio turn
- `stop`: commit the current audio turn into the shared session core
- `detect`: send a direct text turn, typically wake text or a typed/browser debug input

For `detect`, the adapter forwards the text turn into the same runtime path as speech turns and emits `stt` back to the device before the reply.

### `abort`

Interrupts server speech output when the session is currently speaking.

### `mcp`

Accepted and ignored in the current compatibility phase. Voice interaction is the first goal.

### Binary audio frames

The adapter accepts all three firmware binary framing modes:

- version `1`: raw audio payload only
- version `2`: 16-byte header plus payload
- version `3`: 4-byte header plus payload

Binary payload expectations:

- default uplink codec: `opus / 16000 / mono / 60 ms`
- optional raw uplink codec: `pcm16le / 16000 / mono`
- the adapter unwraps the legacy binary header, normalizes audio as needed, and ingests the turn through the shared session core

Version 2 frame shape:

```text
uint16 version
uint16 type
uint32 reserved
uint32 timestamp
uint32 payload_size
bytes  payload
```

Version 3 frame shape:

```text
uint8  type
uint8  reserved
uint16 payload_size
bytes  payload
```

The current adapter only accepts audio binary frame type `0`.

## Server To Client Messages

### `hello`

Confirms the negotiated transport, returns the server session id, and advertises the expected downlink audio profile.

### `stt`

```json
{
  "type": "stt",
  "session_id": "sess_xiaozhi_...",
  "text": "wake text or text input"
}
```

Current behavior:

- emitted for `listen.state=detect` text turns
- emitted for audio turns after ASR completes when the shared responder returns normalized input text

### `tts`

Supported states:

- `start`
- `sentence_start`
- `stop`

The adapter sends:

1. `tts.start`
2. optional `tts.sentence_start` with aggregated reply text
3. one or more binary audio frames
4. `tts.stop`

### `llm`

Current compatibility behavior is intentionally narrow:

- no incremental `llm.text` streaming during spoken replies
- if a reply has no audio, the adapter sends one fallback `llm.text` message so debug clients still see text
- `llm.emotion` is reserved for future runtime mapping and is not populated yet

### `server`

Error surface for invalid JSON, unsupported frames, decode failures, and other compatibility-layer problems.

## Audio Path

The adapter is deliberately asymmetric:

- uplink can arrive as `opus` or configured `pcm16le`
- the shared runtime currently produces source speech as realtime output audio
- the compatibility adapter requires that source runtime audio be `pcm16le`
- the adapter then encodes downlink audio into the configured `xiaozhi` output profile, which defaults to `opus / 24000 / mono / 60 ms`

This keeps the shared runtime and session core unchanged while allowing `xiaozhi` devices to keep their expected downlink model.

## Session Behavior

Important behavior notes:

- there is still one logical session per websocket connection
- `listen.start` will lazily create the shared realtime session if needed
- `listen.stop` commits the buffered audio turn
- inbound audio or `abort` can barge into current speech output
- idle timeout and max-session timeout still come from the shared realtime session policy
- when the shared runtime requests session end after audio, the adapter closes the WebSocket after `tts.stop`

## Native vs Compatibility Paths

Choose one path per client implementation:

- native `agent-server` RTOS client: use `GET /v1/realtime` and `/v1/realtime/ws`
- existing `xiaozhi` firmware or browser page: use `/xiaozhi/ota/` and `/xiaozhi/v1/`

The compatibility adapter exists to reduce firmware churn, but the long-term protocol center of the system remains the shared realtime session contract.
