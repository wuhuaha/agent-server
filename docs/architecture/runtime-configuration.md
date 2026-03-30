# Runtime Configuration

This document explains the environment variables currently reserved for the first RTOS realtime profile.

## Core Service

- `AGENT_SERVER_ADDR`: HTTP listen address
- `AGENT_SERVER_ENV`: environment label such as `dev` or `prod`
- `AGENT_SERVER_NAME`: service name
- `AGENT_SERVER_VERSION`: build or release version

## Realtime Transport

- `AGENT_SERVER_REALTIME_WS_PATH`: WebSocket path for RTOS profile
- `AGENT_SERVER_REALTIME_PROTOCOL_VERSION`: wire profile identifier, currently `rtos-ws-v0`
- `AGENT_SERVER_REALTIME_SUBPROTOCOL`: WebSocket subprotocol string
- `AGENT_SERVER_REALTIME_AUTH_MODE`: currently `disabled` for bootstrap
- `AGENT_SERVER_REALTIME_TURN_MODE`: turn-taking policy label
- `AGENT_SERVER_REALTIME_IDLE_TIMEOUT_MS`: server idle timeout
- `AGENT_SERVER_REALTIME_MAX_SESSION_MS`: hard session duration cap
- `AGENT_SERVER_REALTIME_MAX_FRAME_BYTES`: maximum accepted binary audio frame size

Current server behaviour:

- idle timeout is enforced only when the session state is `active`
- max session duration is enforced across the full session lifetime
- realtime audio downlink is paced at `20 ms` PCM frames
- a speaking response can be interrupted by:
  - new inbound binary audio
  - or `session.update` with `payload.interrupt = true`

## Audio Defaults

- `AGENT_SERVER_REALTIME_INPUT_CODEC`
- `AGENT_SERVER_REALTIME_INPUT_SAMPLE_RATE`
- `AGENT_SERVER_REALTIME_INPUT_CHANNELS`
- `AGENT_SERVER_REALTIME_OUTPUT_CODEC`
- `AGENT_SERVER_REALTIME_OUTPUT_SAMPLE_RATE`
- `AGENT_SERVER_REALTIME_OUTPUT_CHANNELS`

Bootstrap baseline:

- input codec: `pcm16le`
- input sample rate: `16000`
- input channels: `1`
- output codec: `pcm16le`
- output sample rate: `16000`
- output channels: `1`

## Optional Capabilities

- `AGENT_SERVER_REALTIME_ALLOW_OPUS`
- `AGENT_SERVER_REALTIME_ALLOW_TEXT_INPUT`
- `AGENT_SERVER_REALTIME_ALLOW_IMAGE_INPUT`

The first RTOS bring-up path should not depend on image input. Text input is optional. When `AGENT_SERVER_REALTIME_ALLOW_OPUS=true`, the gateway now accepts supported speech-oriented `opus` uplink packets and normalizes them to `pcm16le/16000/mono` before ASR.

## Voice Runtime

- `AGENT_SERVER_VOICE_PROVIDER`
  - `bootstrap`: existing placeholder responder
  - `funasr_http`: local HTTP ASR worker backed by FunASR
- `AGENT_SERVER_VOICE_ASR_URL`: FunASR worker transcription endpoint
- `AGENT_SERVER_VOICE_ASR_TIMEOUT_MS`: timeout for one ASR request
- `AGENT_SERVER_VOICE_ASR_LANGUAGE`: language hint, default `auto`
- `AGENT_SERVER_VOICE_EMIT_PLACEHOLDER_AUDIO`: whether to keep emitting silent audio chunks until real TTS is added

Current directly usable local setup:

1. start the worker with `scripts/start-funasr-worker.ps1`
2. start the server with `scripts/dev-funasr.ps1` or `scripts/dev-funasr-mimo.ps1`
3. connect with the Python desktop client or the scripted runner

For `opus` device uplink, the current server path supports mono speech-oriented `SILK-only` packets and normalizes them in Go before calling the worker. The Python worker still receives `pcm16le` JSON payloads.

Current caveat on this machine:

- `FunASR` inference works locally on CPU.
- The existing `xiaozhi-esp32-server` conda env uses `torch 2.2.2+cu121`, which is not compatible with the local RTX 5060 for `SenseVoiceSmall`, so `device=cpu` is currently the stable path.

## TTS Runtime

- `AGENT_SERVER_TTS_PROVIDER`
  - `none`: disable real TTS and keep current fallback behavior
  - `mimo_v2_tts`: call Xiaomi MiMo TTS through its OpenAI-compatible API
- `MIMO_API_KEY`: MiMo API key, recommended as a user environment variable
- `AGENT_SERVER_TTS_MIMO_BASE_URL`: default `https://api.xiaomimimo.com/v1`
- `AGENT_SERVER_TTS_MIMO_MODEL`: default `mimo-v2-tts`
- `AGENT_SERVER_TTS_MIMO_VOICE`: built-in voice such as `mimo_default`, `default_zh`, or `default_en`
- `AGENT_SERVER_TTS_MIMO_STYLE`: optional style prefix inserted as `<style>...</style>`
- `AGENT_SERVER_TTS_TIMEOUT_MS`: request timeout

Current implementation details:

- the Go server now calls MiMo with streaming `pcm16` output for the realtime path
- the SSE stream is decoded incrementally and the first device frames can be forwarded without waiting for a full synthesis result
- the streamed PCM is emitted to the device in `20 ms` paced frames so barge-in can preempt the current response
- the older non-streaming `wav` decode path remains as a compatibility fallback inside the synthesizer implementation
