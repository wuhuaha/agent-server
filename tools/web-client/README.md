# tools/web-client

Standalone browser client for the native `agent-server` realtime websocket contract.

It is intended for:

- manual use against `/v1/realtime/ws`
- protocol bring-up
- text-turn and microphone-turn debugging
- raw JSON control-event testing

Unlike the built-in `/debug/realtime-h5/` page served by `agent-server`, this tool is designed to work as a separate static site.

That means:

- it connects directly to the native websocket contract
- it does not depend on same-origin `GET /v1/realtime`
- it now splits setup and live debugging into separate pages
- it uses manual profile fields by default
- it can also import pasted discovery JSON from `GET /v1/realtime`

## Run

From the repository root:

```bash
cd tools/web-client
python3 serve.py --port 18081
```

Then open:

```text
http://127.0.0.1:18081/settings.html
```

Page split:

- `settings.html`: endpoint, audio profile, device preset, and optional discovery sync
- `index.html`: live debug console for connect, text turn, mic turn, raw JSON, TTS playback, and logs

Point it at your server, for example:

- `HTTP base`: `http://127.0.0.1:8080`
- `WS path`: `/v1/realtime/ws`
- `Subprotocol`: `agent-server.realtime.v0`
- `Protocol version`: `rtos-ws-v0`

Current happy-path defaults already match the repository runtime defaults:

- input: `pcm16le / 16000 / mono`
- output: `pcm16le / 16000 / mono`

## Discovery JSON Import

If you want the tool fields to match a running server exactly, fetch discovery separately and paste the JSON into the page:

```bash
curl http://127.0.0.1:8080/v1/realtime
```

Then use `Apply Discovery JSON`.

This avoids requiring CORS on the server for standalone static hosting.

For archived manual browser-validation evidence, scaffold a bundle before testing:

```bash
./scripts/web-h5-manual-capture.sh --mode standalone --standalone-base http://127.0.0.1:18081
```

That creates a `web-h5-manual` artifact root with server snapshots, page snapshots, and a manual checklist for screenshots, console logs, and exported WAV files.

## Features

- dedicated settings page for profile save and discovery sync
- dedicated debug page for live websocket work
- connect and disconnect websocket
- `session.start` and `session.end`
- `text.in`
- microphone capture with `audio.in.commit`
- `session.update { "interrupt": true }`
- raw JSON envelope send for protocol debugging
- assistant text log
- raw event log
- TTS chunk count, byte count, playback state, replay, and WAV export

## Current Limits

- microphone turns currently require input audio `pcm16le` and mono
- binary playback currently requires output audio `pcm16le` and mono
- raw browser `opus` uplink is not implemented here
- remote browsers typically need HTTPS and WSS for microphone permission
