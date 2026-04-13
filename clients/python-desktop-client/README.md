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

- `--scenario text|audio|server-end|server-endpoint-preview|tool|barge-in|timeout|full|regression`
- `--text "hello from scripted desktop client"`
- `--silence-ms 1000`
- `--frame-ms 20`
- `--wav path\to\pcm16_mono_16k.wav`
- `--output .\runner-report.json`
- `--save-rx-dir .\artifacts`

Recommended live artifact root:

- quick smoke: `artifacts/live-smoke/YYYYMMDD/desktop-full`
- archived comparison run: `artifacts/live-baseline/YYYYMMDD/desktop-regression`

The JSON report now includes:

- run metadata such as `generated_at`, `run_id`, and optional `artifact_dir`
- discovery metadata such as `turn_mode`, `llm_provider`, `voice_provider`, and `tts_provider`
- per-scenario identifiers and diagnostics such as `turn_id`, `trace_id`, `issues`, and `artifacts`
- per-scenario phase and latency metrics such as `thinking_latency_ms`, `speaking_latency_ms`, `active_return_latency_ms`, `response_start_latency_ms`, `first_partial_latency_ms`, `first_text_latency_ms`, `first_audio_latency_ms`, `barge_in_cutoff_latency_ms`, `response_complete_latency_ms`, and `playout_complete_latency_ms`
- top-level `quality_summary` aggregates for quick cross-run comparison, including audio-byte totals, text-volume totals, partial-response ratio, heard-text totals, and issue counts

When `--save-rx-dir` is set, the runner now creates one run directory under that path and saves replay-friendly artifacts per scenario:

- `events.json`
- `response.txt`
- `scenario.json`
- `received-audio.wav` when binary audio arrived

The `full` scenario runs the fast smoke trio:

1. a text turn and client-side close
2. an audio turn and client-side close
3. a `/end` text turn that expects the server to close the session

The `regression` scenario runs the broader migration baseline:

1. `text`
2. `audio`
3. `server-end`
4. `tool`
5. `barge-in`
6. `timeout`

For the canonical artifact-root and profile naming convention used across the repository, see [../../docs/codex/live-validation-runbook.md](../../docs/codex/live-validation-runbook.md).

Current intent of the additional scenarios:

- `server-endpoint-preview`: uploads audio without sending `audio.in.commit` and expects hidden server endpointing to auto-close the turn; this requires `AGENT_SERVER_VOICE_SERVER_ENDPOINT_ENABLED=true` and a speech-like `--wav` input
- `tool`: exercises the shared tool-call and tool-result delta path with `/tool time.now {}`
- `barge-in`: verifies that one spoken response starts, receives audio, gets interrupted, and a second turn completes afterward
- `timeout`: verifies server-driven `idle_timeout` session closure using the discovery-advertised idle timeout

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
- `--save-rx .\rtos-rx.wav`
- `--save-rx-dir .\rtos-artifacts`

Recommended live artifact root:

- quick smoke: `artifacts/live-smoke/YYYYMMDD/rtos-mock`
- archived comparison run: `artifacts/live-baseline/YYYYMMDD/rtos-mock`

The RTOS mock JSON report now uses the same baseline vocabulary as the desktop runner for:

- run metadata such as `generated_at`, `run_id`, and `artifact_dir`
- discovery metadata such as `turn_mode`, `llm_provider`, `voice_provider`, and `tts_provider`
- identifier capture such as `turn_id`, `trace_id`, `turn_ids`, and `trace_ids`
- replay-friendly artifact references under `artifacts`

When `--save-rx-dir` is set, the RTOS mock archives:

- `events.json`
- `response.txt`
- `run.json`
- `received-audio.wav` when audio arrived

Typical barge-in validation:

```powershell
agent-server-rtos-mock `
  --http-base http://127.0.0.1:8080 `
  --wav .\sample.wav `
  --interrupt-wav .\interrupt.wav `
  --save-rx-dir .\rtos-artifacts `
  --output .\rtos-mock-report.json
```
