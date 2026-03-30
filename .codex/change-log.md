# Change Log

## 2026-03-25

- Created the `agent-server` repository on `E:\agent-server`.
- Linked the project into the current workspace for stable editing.
- Added collaboration folders, architecture documents, protocol drafts, and local skills scaffolding.
- Added the initial Go service skeleton, schema, Docker bootstrap files, and local development scripts.
- Added project-local skills for bootstrap, architecture guard, protocol guard, RTOS fast path, and channel skill design.
- Imported the `agents/` role pack from `affaan-m/everything-claude-code`.
- Reworked root `AGENTS.md` and added `SECURITY.md` using `everything-claude-code` as the baseline while preserving `agent-server` specific constraints.
- Imported 30 curated skills from `everything-claude-code/skills` into the repository root `skills/` directory for local reference and reuse.
- Formalized the first RTOS device-facing wire profile in `docs/protocols/rtos-device-ws-v0.md`.
- Expanded runtime config defaults in `.env.example` and surfaced realtime profile details via the bootstrap server config and discovery endpoints.
- Implemented the bootstrap WebSocket handler for `/v1/realtime/ws` using the `rtos-ws-v0` profile.
- Added a minimal session state machine, placeholder voice responder, and unit tests for discovery, session lifecycle, and bootstrap response behaviour.
- Added `clients/python-desktop-client`, a Tk desktop debug client for discovery, session control, text input, PCM WAV uplink, silence generation, raw JSON injection, and received-audio save/playback.
- Added Python protocol/audio tests for the desktop client and verified them locally with `python -m unittest discover -s tests -v`.
- Added `go.sum`, resolved the websocket dependency, formatted the Go sources with `gofmt`, and verified the Go codebase with `go test ./...`.

## 2026-03-26

- Confirmed the restarted Codex terminal now picks up `go` and `gofmt` directly from PATH.
- Performed live smoke validation against a real `agentd` process:
  - `GET /healthz` returned `status=ok`
  - `GET /v1/realtime` returned the expected `rtos-ws-v0` discovery payload
  - WebSocket flow `session.start -> binary audio -> audio.in.commit -> response.start/response.chunk/audio.out.chunk -> session.end` succeeded end-to-end
- Added `agent_server_desktop_client.runner`, a headless scripted validation runner with `text`, `audio`, `server-end`, and `full` scenarios.
- Verified the scripted runner locally with Python unit tests and executed the `full` live scenario successfully against a local `agentd` process.
- Saved the latest scripted validation artifacts at `.codex/artifacts/desktop-runner-full.json` and `.codex/artifacts/desktop-runner/audio-scenario-rx.wav`.
- Added a directly usable local ASR path:
  - Go-side turn-audio buffering
  - configurable `funasr_http` voice provider
  - Python `FunASR` worker service under `workers/python`
  - PowerShell start scripts for the worker, the server, and one-command smoke validation
- Verified `FunASR` transcription locally with a generated 16k mono WAV sample and confirmed the recognized text `Hello from agent server.` through the realtime WebSocket path.
- Saved the latest FunASR validation artifacts at `.codex/artifacts/funasr-runner-full.json` and `.codex/artifacts/smoke-funasr-script/runner-report.json`.
- Added MiMo TTS integration for the realtime voice path:
  - configurable `mimo_v2_tts` provider in server runtime config
  - Go-side MiMo synthesizer using the OpenAI-compatible `/v1/chat/completions` API
  - PCM16 audio adaptation to the RTOS wire profile output format
  - PowerShell bring-up script `scripts/dev-funasr-mimo.ps1`
- Added runtime logging around TTS synthesis and threaded `session_id` / `device_id` through synthesis requests for live debugging.
- Fixed `scripts/smoke-funasr.ps1` to:
  - launch `dev-funasr-mimo.ps1` instead of the non-TTS bootstrap script
  - honor the requested `-ServerPort` by exporting `AGENT_SERVER_ADDR` before starting `agentd`
- Revalidated the end-to-end stack on clean ports with `smoke-funasr.ps1 -ServerPort 18080 -WorkerPort 18091` and confirmed real MiMo audio was streamed over `/v1/realtime/ws`.
- Saved the latest ASR+TTS validation artifacts at `.codex/artifacts/smoke-funasr-mimo4/runner-report.json` and `.codex/artifacts/smoke-funasr-mimo4/audio-scenario-rx.wav`.
- Reworked the realtime WebSocket handler to support:
  - write-serialized websocket output
  - paced `20 ms` audio downlink streaming
  - barge-in cancellation during `speaking`
  - idle timeout and max session duration enforcement
- Added websocket integration tests for:
  - `idle_timeout`
  - `max_duration`
  - barge-in interrupting an in-progress spoken response
- Added `agent_server_desktop_client.rtos_mock`, a Python RTOS-oriented reference CLI that:
  - streams audio in realtime frame cadence
  - optionally sends `session.update {interrupt:true}`
  - can trigger a second interrupting turn
  - saves received audio and a JSON event summary
- Added `scripts/smoke-rtos-mock.ps1` for one-command live validation of the RTOS mock path against local FunASR + MiMo TTS.
- Verified the RTOS mock path live with barge-in enabled and saved artifacts at `.codex/artifacts/smoke-rtos-mock-barge2/rtos-mock-report.json` and `.codex/artifacts/smoke-rtos-mock-barge2/rtos-mock-rx.wav`.

## 2026-03-30

- Added speech-oriented `opus` uplink support in the Go realtime gateway by normalizing supported mono `SILK-only` packets to `pcm16le/16000/mono` before ASR.
- Added streaming audio contracts in `internal/voice` and switched the MiMo realtime path to provider-streamed `pcm16`, while keeping buffered `wav` decoding as a compatibility fallback.
- Updated the realtime WebSocket runtime to build per-session input normalizers, stream normalized uplink audio into the existing ASR path, and forward streamed TTS chunks incrementally to the device.
- Added regression coverage for Opus normalization and MiMo streaming, plus repository test data under `testdata/opus-tiny.ogg`.
- Aligned protocol/schema/runtime docs with the current `session.start` contract, flexible `client_type` handling, optional `opus` bring-up rules, and the MiMo streaming TTS path.
