# plan.md

## Working Goal

Create a directly usable foundation for `agent-server` with stable architecture boundaries, local development conventions, and the first implementation path toward RTOS realtime voice sessions.

## Confirmed Decisions

- Use a modular monolith first.
- Use Go for the main service runtime.
- Reserve Python for future voice and vision workers.
- Make `Realtime Session Core` the center of the system.
- Use `WebSocket + binary audio + JSON control events` for the first RTOS path.
- Keep authentication out of the critical path for v0.1, but reserve protocol fields for it.
- Model Feishu-like integrations as `channel skills`, not plain tools.

## Current Milestones

### M0 Foundation

- [x] Create the repository and baseline directories.
- [x] Create `.claude/`, `.codex/`, `AGENTS.md`, and `plan.md`.
- [x] Create first-pass architecture, protocol, and ADR documents.
- [x] Create project-local skills for future coding sessions.
- [x] Create the initial Go service skeleton.
- [x] Add a minimal schema for realtime envelopes.
- [x] Freeze the first RTOS device wire profile and runtime configuration defaults in docs.

### M1 RTOS Voice Fast Path

- [x] Accept realtime session start from device side.
- [x] Accept audio uplink frames.
- [x] Return streamed response events.
- [x] Allow both client and server to end the session.
- [x] Validate a directly usable local ASR + MiMo TTS voice path.
- [x] Define and implement barge-in and idle timeout policy.
- [x] Add an RTOS-oriented mock/reference client for protocol bring-up.

Bootstrap implementation for the WebSocket handler, session lifecycle, and placeholder response path has been added in code. `go test ./...` now passes locally after the Go toolchain was installed and dependencies were resolved via `GOPROXY=https://goproxy.cn,direct`.

Live smoke validation has also succeeded against a local `agentd` process for:

- `GET /healthz`
- `GET /v1/realtime`
- `session.start -> audio uplink -> audio.in.commit -> streamed response -> session.end`
- scripted `full` validation via `agent-server-desktop-runner`, covering text turn, audio turn, and server-initiated close
- scripted `smoke-funasr.ps1` validation on clean ports with:
  - local `FunASR` ASR worker on CPU
  - `mimo-v2-tts` speech synthesis
  - real PCM16 audio chunks returned over `/v1/realtime/ws`
- websocket runtime behaviour now includes:
  - paced `20 ms` audio downlink frames
  - barge-in by inbound audio or `session.update interrupt=true`
  - `idle_timeout` only while the session is `active`
  - `max_duration` enforced across the session lifetime
- live `rtos mock` validation on clean ports succeeded with:
  - one primary audio turn
  - one barge-in turn triggered during `speaking`
  - final client-initiated close after the second response completed

Recent hardening updates on top of that baseline include:

- optional speech-oriented `opus` uplink support, normalized to `pcm16le/16000/mono` before ASR
- MiMo realtime TTS now requesting streaming `pcm16` from the provider and forwarding the first `20 ms` frames without waiting for a full `wav`
- schema and docs now aligned with the current `session.start` contract and local validation clients

### M2 Channel Skill Framework

- [ ] Finalize channel contract and registry.
- [ ] Add the first Feishu channel adapter skeleton.
- [ ] Connect channel messages into the shared session core.

### M3 Security Backfill

- [ ] Device registration and token flow.
- [ ] User auth and operator auth.
- [ ] Multi-tenant policy boundaries.
- [ ] Audit and rate-limiting.

## Immediate Next Step

Start the first Feishu channel adapter skeleton on top of the now-stable realtime session core, then connect it through the shared session contract before beginning security backfill.
