# Python Desktop Client

Desktop debug client for the `agent-server` `rtos-ws-v0` realtime profile.

It supports:

- Discovering realtime capabilities from `GET /v1/realtime`
- Connecting to `/v1/realtime/ws`
- Starting and ending sessions
- Sending text turns
- Streaming PCM WAV files as uplink audio
- Generating silence frames for transport debugging
- Sending `audio.in.commit`
- Viewing logs and accumulated response text
- Saving and replaying received PCM audio on Windows
- Injecting raw JSON control events

## Install

```powershell
cd clients/python-desktop-client
python -m pip install -e .
```

## Run

```powershell
agent-server-desktop-client
```

Or:

```powershell
python -m agent_server_desktop_client.app
```

Or:

```powershell
python -m agent_server_desktop_client
```

The first debug profile assumes `pcm16le / 16000 Hz / mono` for both directions unless the server discovery endpoint reports different values.

## Scripted Validation

For repeatable text and audio bring-up without the GUI:

```powershell
agent-server-desktop-runner --scenario full --http-base http://127.0.0.1:8080
```

Useful options:

- `--scenario text|audio|server-end|full`
- `--text "hello from scripted desktop client"`
- `--silence-ms 1000`
- `--frame-ms 20`
- `--wav path\to\pcm16_mono_16k.wav`
- `--output .\runner-report.json`
- `--save-rx-dir .\artifacts`

The JSON report now includes:

- discovery metadata such as `turn_mode`, `voice_provider`, and `tts_provider`
- per-scenario latency metrics such as `response_start_latency_ms`, `first_text_latency_ms`, `first_audio_latency_ms`, and `response_complete_latency_ms`
- top-level `quality_summary` aggregates for quick cross-run comparison

The `full` scenario runs:

1. a text turn and client-side close
2. an audio turn and client-side close
3. a `/end` text turn that expects the server to close the session

## RTOS Mock Client

For a CLI that behaves more like a device endpoint:

```powershell
agent-server-rtos-mock --http-base http://127.0.0.1:8080 --wav .\sample.wav --save-rx .\rtos-rx.wav
```

Useful options:

- `--interrupt-wav .\interrupt.wav`
- `--interrupt-silence-ms 600`
- `--no-interrupt-update`
- `--no-auto-end`
- `--output .\rtos-mock-report.json`

Typical barge-in validation:

```powershell
agent-server-rtos-mock `
  --http-base http://127.0.0.1:8080 `
  --wav .\sample.wav `
  --interrupt-wav .\interrupt.wav `
  --save-rx .\rtos-rx.wav `
  --output .\rtos-mock-report.json
```
