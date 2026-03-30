# Local MiMo TTS Bring-Up

## Goal

Add directly usable speech output on top of the local FunASR ASR path without changing the device-facing realtime protocol.

## Provider Boundary

- Go realtime gateway owns device output framing and pacing
- MiMo TTS is called from Go through the OpenAI-compatible `chat/completions` API
- the realtime path requests `stream=true` with `audio.format=pcm16`
- streamed provider chunks are forwarded incrementally as `pcm16le` realtime output frames
- the older non-streaming `wav` decode path remains as a compatibility fallback inside the synthesizer

## Required Environment

- `MIMO_API_KEY`
- `AGENT_SERVER_TTS_PROVIDER=mimo_v2_tts`

Recommended local startup:

1. `scripts/start-funasr-worker.ps1`
2. `scripts/dev-funasr-mimo.ps1`
3. `scripts/smoke-funasr.ps1`

## Current Default Voice

- `mimo_default`

Other documented built-in voices:

- `default_zh`
- `default_en`

## Current Notes

- MiMo official examples still show `24kHz` mono output.
- The current server implementation adapts provider audio to the configured realtime output sample rate before pushing binary audio frames to the client.
- The realtime gateway can now forward the first spoken chunk without waiting for a full buffered `wav` response.
