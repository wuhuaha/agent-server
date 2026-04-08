# Web/H5 Direct Realtime Adaptation

## Purpose

This guide explains how a browser or H5 page can connect directly to the native `agent-server` realtime websocket path without going through the `xiaozhi` compatibility layer.

The current browser bring-up path intentionally reuses the same native contract as RTOS clients:

- discovery: `GET /v1/realtime`
- websocket: `/v1/realtime/ws`
- subprotocol: `agent-server.realtime.v0`

There is no separate browser-only agent protocol in this repository right now.

## Built-In Debug Page

The service now ships with a same-origin debug page:

- `GET /debug/realtime-h5/`
- `GET /debug/realtime-h5/settings.html`

Use it when you want the fastest possible browser bring-up against the current service process.

Recommended flow:

1. open `GET /debug/realtime-h5/settings.html`
2. confirm device preset and refresh `GET /v1/realtime`
3. switch to `GET /debug/realtime-h5/`
4. run live websocket debug turns

The built-in pages perform:

- settings page:
  - browser preset save for `device_id` and `wake_reason`
  - same-origin discovery refresh
  - current LLM/voice/TTS provider snapshot
- debug page:
  - realtime discovery during connect
  - websocket connect with the required subprotocol
  - `session.start`
  - `text.in`
  - microphone capture for binary audio uplink
  - `audio.in.commit`
  - binary `pcm16le` audio playback from the server
  - TTS replay and WAV export of the latest turn

## Standalone Repository Tool

The repository also now ships a separate static tool:

- `tools/web-client/`

Use that path when you want:

- a standalone browser client outside the main Go service
- manual test/debug work from a different static origin
- raw JSON control-event testing against the native realtime websocket

Because it is a separate static site, it does not assume same-origin discovery. Instead it supports:

- `settings.html` for manual realtime profile entry and discovery sync
- `index.html` for live websocket debug, microphone turns, and TTS playback
- pasted discovery JSON from `GET /v1/realtime`

## Current Compatibility Boundaries

The first browser slice is intentionally narrow.

Current supported browser audio shape:

- uplink: mono `pcm16le`
- downlink: mono `pcm16le`
- browser-side microphone adaptation: capture from Web Audio and convert to raw `pcm16le` frames before websocket send
- browser-side playback adaptation: decode server binary audio as `pcm16le` and queue it in Web Audio

Current non-goals for this slice:

- raw `opus` uplink from the browser
- browser-side `image.in`
- WebRTC transport
- a second browser-specific websocket event family

## Secure Context Requirement

Remote browsers need a secure context before microphone permission is available.

Practical rule:

- `http://127.0.0.1` and `http://localhost` are acceptable for local debugging
- remote phones, tablets, and H5 pages should use `https://...` and therefore `wss://...`

If you open the built-in page over a non-secure remote origin, the text path may still work, but microphone capture will usually be blocked by the browser.

The same browser rule applies to the standalone tool under `tools/web-client/`.

## Expected Server Profile

For the current browser reference page, discovery should advertise:

- `llm_provider` with the effective runtime selection so browser bring-up can detect bootstrap fallback quickly
- `input_audio.codec = pcm16le`
- `input_audio.channels = 1`
- `output_audio.codec = pcm16le`
- `output_audio.channels = 1`

The sample rate may vary, but the current page assumes the server stays on a mono `pcm16le` path and adapts capture or playback around the advertised sample rate.

The repository defaults already match the happy path:

- input: `pcm16le / 16000 / mono`
- output: `pcm16le / 16000 / mono`

## Browser Session Flow

The reference flow is the same as the native realtime profile:

1. `GET /v1/realtime`
2. open websocket to the advertised `ws_path`
3. send `session.start`
4. send either:
   - `text.in`
   - or binary mic frames followed by `audio.in.commit`
5. receive:
   - `session.update`
   - `response.start`
   - `response.chunk`
   - optional binary audio
6. send `session.end` or wait for server end

## When To Use Which Path

Choose one direct client shape per implementation:

- browser or H5 page built specifically for `agent-server`: use `GET /v1/realtime`, `/v1/realtime/ws`, start from `/debug/realtime-h5/settings.html`, then debug through `/debug/realtime-h5/`
- standalone repo-hosted browser debug tool: serve `tools/web-client/`, start from `settings.html`, then move to `index.html` for live websocket debug
- existing `xiaozhi` firmware or an old browser page written around `hello/listen/abort`: use `/xiaozhi/ota/` and `/xiaozhi/v1/`

## Implementation Notes

The built-in page currently lives under:

- `internal/control/webh5_assets/`

This placement is intentional:

- websocket session execution still belongs to the shared realtime and runtime layers
- the debug page is only a control-plane-hosted bring-up surface
- browser microphone and playback quirks stay in the page, not in the agent runtime
