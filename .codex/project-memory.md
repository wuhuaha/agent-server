# Project Memory

## Durable Decisions

- `Realtime Session Core` is the long-term center of the system.
- RTOS voice fast path is the first functional target after repository initialization.
- Channel integrations such as Feishu are modeled as `channel skills`.
- First transport target is `WebSocket + binary audio + JSON control events`.
- Authentication is deferred, not ignored; reserved fields stay in the protocol.
- The repository now carries an imported root `agents/` pack inspired by `everything-claude-code`, while `agent-server` specific guardrails remain the top-priority section in root `AGENTS.md`.
- The repository also carries a curated subset of upstream ECC skills in root `skills/`, while project-specific execution skills continue to live under `.codex/skills/`.
- The first concrete device profile is now frozen as `rtos-ws-v0`: one active session per socket, text frames for control, binary frames for audio, baseline codec `pcm16le/16k/mono`.
- `GET /v1/realtime` is the discovery contract for device teams and must stay aligned with the protocol docs and runtime config defaults.
- The first bootstrap implementation now supports: WebSocket upgrade, `session.start`, binary audio uplink, `audio.in.commit`, placeholder streamed response events, and bidirectional `session.end`.
- A Python desktop debug client now exists under `clients/python-desktop-client` and is the primary manual validation tool for the bootstrap realtime protocol until a smaller RTOS reference client is added.
- The same Python client package now also provides a headless scripted runner for repeatable discovery/text/audio/server-end validation.
- The local Go toolchain is now installed and repository-level verification uses `GOPROXY=https://goproxy.cn,direct` in this environment because `proxy.golang.org` is not reachable.
- A live smoke run against `agentd` on `http://127.0.0.1:8080` has verified the bootstrap realtime contract, including binary audio uplink, placeholder streamed response events, and bidirectional session close semantics.
- The latest `full` scripted validation report is stored at `.codex/artifacts/desktop-runner-full.json` and confirms text turn, audio turn, and server-initiated close behaviour.
- The server now has a configurable `funasr_http` voice provider that sends committed turn audio to a local Python ASR worker and returns recognized text through the existing realtime contract.
- The directly usable local ASR reference path on this machine is:
  - worker env `xiaozhi-esp32-server`
  - model `iic/SenseVoiceSmall`
  - device `cpu`
  - server script `scripts/dev-funasr.ps1`
  - worker script `scripts/start-funasr-worker.ps1`
- The current RTX 5060 cannot run the existing `torch 2.2.2+cu121` FunASR stack for this model; CPU is the stable path until the env is upgraded.

## Working Defaults

- Main service language: Go.
- Voice and vision adapters: future Python workers where useful.
- Repo style: architecture docs and logs must evolve with the codebase.
- The realtime gateway now accepts optional speech-oriented `opus` uplink, but the supported path is intentionally narrow: mono `SILK-only` packets are decoded in Go and normalized to `pcm16le/16000/mono` before ASR.
- The realtime MiMo TTS path now prefers provider-streamed `pcm16` audio and forwards chunks as they arrive; buffered `wav` decoding remains only as a compatibility fallback inside the synthesizer.
- `session.start.payload.device.client_type` is intentionally modeled as a non-empty implementation identifier rather than a closed enum so `rtos`, `desktop-script`, `rtos-mock`, and future clients can reuse the same schema.
