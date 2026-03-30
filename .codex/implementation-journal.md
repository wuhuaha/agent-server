# Implementation Journal

## 2026-03-25

- Repository initialization started.
- Baseline documents and logs established before service code to reduce architectural churn.
- Next focus: minimal Go service and protocol-shaped endpoints.
- Minimal Go entrypoint, control endpoints, contracts, and schema were added.
- Validation is currently limited by the absence of the local Go toolchain.
- Imported ECC-style root agent roles and adapted AGENTS/security guidance for this repository.
- Imported a curated ECC skill subset focused on architecture, Go/Python engineering, MCP, security, testing, and evaluation workflows.
- Clarified the RTOS-side WebSocket contract before handshake implementation so device and server work can proceed in parallel.
- Implemented the bootstrap RTOS WebSocket path in code, while keeping real ASR/TTS/LLM integration deferred behind the `voice` boundary.
- Added a separate Python desktop client package instead of reusing `workers/python`, so debugging tools stay isolated from future inference workers.
- Implemented a Tk-based debug console with discovery, connect/disconnect, session start/end, text send, PCM WAV streaming, silence generation, raw JSON injection, and received-audio capture.
- Verified the desktop client's protocol/audio helpers with local unit tests and confirmed the package modules compile under Python 3.13.
- Verified the Go toolchain after installation, resolved `github.com/gorilla/websocket` into `go.sum`, formatted the Go files with `gofmt`, and got `go test ./...` passing locally.
- Started a real `agentd` process locally, confirmed `/healthz` and `/v1/realtime`, and exercised the WebSocket bootstrap path with a scripted smoke client that sent `session.start`, one 640-byte PCM frame, `audio.in.commit`, and `session.end`.
- Added a reusable headless runner to the Python desktop client package instead of keeping scripted validation as one-off shell snippets.
- Executed the runner's `full` scenario against a live `agentd`, covering text input, 1-second silence-as-audio input, and server-initiated close via `/end`, and saved the resulting JSON report and received-audio WAV artifact.
- Reworked the Go realtime session so each turn now buffers raw PCM bytes instead of only counting frame sizes, which made real local ASR integration possible without changing the device protocol.
- Added the `funasr_http` responder path, a Python FunASR HTTP worker, worker/server startup scripts, and a one-command `smoke-funasr.ps1` script.
- Generated a 16k mono WAV sample with Windows SAPI, used it to verify raw FunASR inference locally, and then exercised the same sample through the full `worker -> agentd -> websocket runner` path.
