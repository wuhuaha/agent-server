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
- `AGENT_SERVER_REALTIME_TURN_MODE`: turn-taking policy label, currently `client_wakeup_client_commit`
- `AGENT_SERVER_REALTIME_IDLE_TIMEOUT_MS`: server idle timeout
- `AGENT_SERVER_REALTIME_MAX_SESSION_MS`: hard session duration cap
- `AGENT_SERVER_REALTIME_MAX_FRAME_BYTES`: maximum accepted binary audio frame size

Current server behaviour:

- the discovery default `turn_mode` is `client_wakeup_client_commit`
- client wakeup or explicit action starts the session, and each audio turn completes only after explicit `audio.in.commit`
- the current bootstrap profile does not yet advertise a server-side VAD commit path
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

## Agent Runtime

- `AGENT_SERVER_AGENT_MEMORY_PROVIDER`
  - `in_memory`: default recent-turn memory backend
  - `noop`: disable runtime memory persistence or recall
- `AGENT_SERVER_AGENT_MEMORY_MAX_TURNS`: bounded recent-turn window size for `in_memory`
- `AGENT_SERVER_AGENT_TOOL_PROVIDER`
  - `builtin`: default local runtime tool backend
  - `noop`: disable runtime tools
- `AGENT_SERVER_AGENT_LLM_PROVIDER`
  - `bootstrap`: existing placeholder or bring-up executor
  - `deepseek_chat`: optional DeepSeek chat-completions-backed executor
- `AGENT_SERVER_AGENT_LLM_TIMEOUT_MS`: timeout for one LLM request
- `AGENT_SERVER_AGENT_PERSONA`: built-in persona profile selector, currently `household_control_screen`
- `AGENT_SERVER_AGENT_EXECUTION_MODE`: runtime execution policy, one of `simulation`, `dry_run`, or `live_control`
- `AGENT_SERVER_AGENT_ASSISTANT_NAME`: assistant display or speaking name used by the built-in household prompt template, default `小欧管家`
- `AGENT_SERVER_AGENT_LLM_SYSTEM_PROMPT`: optional persona-template override; execution-mode policy is still appended by the runtime
- `AGENT_SERVER_AGENT_DEEPSEEK_BASE_URL`: default `https://api.deepseek.com`
- `AGENT_SERVER_AGENT_DEEPSEEK_API_KEY` or `DEEPSEEK_API_KEY`
- `AGENT_SERVER_AGENT_DEEPSEEK_MODEL`: default `deepseek-chat`
- `AGENT_SERVER_AGENT_DEEPSEEK_TEMPERATURE`: default `0.2`
- `AGENT_SERVER_AGENT_DEEPSEEK_MAX_TOKENS`: optional max token cap, `0` means provider default

Current runtime note:

- the DeepSeek integration stays inside `internal/agent`; device gateways, channel adapters, and the voice runtime still depend only on the shared `TurnExecutor`
- when `AGENT_SERVER_AGENT_LLM_SYSTEM_PROMPT` is empty, the runtime injects a built-in Chinese smart-home-control persona selected by `AGENT_SERVER_AGENT_PERSONA`
- the runtime always appends an execution-mode policy selected by `AGENT_SERVER_AGENT_EXECUTION_MODE`
- custom prompt overrides may include `{{assistant_name}}`, which is replaced at runtime from `AGENT_SERVER_AGENT_ASSISTANT_NAME`
- custom prompt overrides replace the persona template, but do not disable the runtime-owned execution-mode policy
- current mode semantics are:
  - `simulation`: debug-stage simulated success feedback for control requests, without exposing simulation details to end users
  - `dry_run`: natural-language description of understood target action and expected effect, without claiming real execution
  - `live_control`: completion-style confirmation only when real execution results are available
- reserved bring-up commands `/memory` and `/tool <name> <json>` still bypass the LLM path so runtime backends can be debugged independently of model responses

## Voice Runtime

- `AGENT_SERVER_VOICE_PROVIDER`
  - `bootstrap`: existing placeholder responder
  - `funasr_http`: local HTTP ASR worker backed by FunASR
  - `iflytek_rtasr`: iFlytek RTASR websocket ASR provider
- `AGENT_SERVER_VOICE_ASR_URL`: FunASR worker transcription endpoint
- `AGENT_SERVER_VOICE_ASR_TIMEOUT_MS`: timeout for one ASR request
- `AGENT_SERVER_VOICE_ASR_LANGUAGE`: language hint, default `auto`
- `AGENT_SERVER_VOICE_EMIT_PLACEHOLDER_AUDIO`: whether to keep emitting silent audio chunks until real TTS is added
- `AGENT_SERVER_VOICE_IFLYTEK_RTASR_APP_ID`
- `AGENT_SERVER_VOICE_IFLYTEK_RTASR_ACCESS_KEY_ID`
- `AGENT_SERVER_VOICE_IFLYTEK_RTASR_ACCESS_KEY_SECRET`
- `AGENT_SERVER_VOICE_IFLYTEK_RTASR_SCHEME`: default `ws`
- `AGENT_SERVER_VOICE_IFLYTEK_RTASR_HOST`: default `office-api-ast-dx.iflyaisol.com`
- `AGENT_SERVER_VOICE_IFLYTEK_RTASR_PORT`: default `80`
- `AGENT_SERVER_VOICE_IFLYTEK_RTASR_PATH`: default `/ast/communicate/v1`
- `AGENT_SERVER_VOICE_IFLYTEK_RTASR_AUDIO_ENCODE`: default `pcm_s16le`
- `AGENT_SERVER_VOICE_IFLYTEK_RTASR_LANGUAGE`: default `autodialect`
- `AGENT_SERVER_VOICE_IFLYTEK_RTASR_SAMPLE_RATE`: default `16000`
- `AGENT_SERVER_VOICE_IFLYTEK_RTASR_FRAME_BYTES`: default `1280`
- `AGENT_SERVER_VOICE_IFLYTEK_RTASR_FRAME_INTERVAL_MS`: default `40`

Current directly usable local setup:

1. start the worker with `scripts/start-funasr-worker.ps1`
2. start the server with `scripts/dev-funasr.ps1` or `scripts/dev-funasr-mimo.ps1`
3. connect with the Python desktop client or the scripted runner

For `opus` device uplink, the current server path supports mono speech-oriented `SILK-only` packets and normalizes them in Go before calling the worker. The Python worker still receives `pcm16le` JSON payloads.

Current note on this machine:

- `FunASR` inference has been validated locally on both CPU and GPU.
- The `xiaozhi-esp32-server` conda env now carries `torch 2.11.0+cu128` and `torchaudio 2.11.0+cu128`, and `SenseVoiceSmall` has been verified on the local RTX 5060 with `device=cuda:0`.
- The default worker scripts still use `device=cpu` for predictable local bring-up and easy fallback.
- The local `SenseVoiceSmall` worker path must keep `AGENT_SERVER_FUNASR_TRUST_REMOTE_CODE=false`; the downloaded model bundle loads correctly from cache, but enabling remote code fails with `No module named 'model'`.

Current cloud ASR note:

- `iflytek_rtasr` still plugs into the existing turn-based `audio.in.commit` boundary, but the provider call itself uses websocket-framed audio upload under `internal/voice`.
- The device-facing realtime contract does not change when switching between `funasr_http` and `iflytek_rtasr`.

## TTS Runtime

- `AGENT_SERVER_TTS_PROVIDER`
  - `none`: disable real TTS and keep current fallback behavior
  - `mimo_v2_tts`: call Xiaomi MiMo TTS through its OpenAI-compatible API
  - `iflytek_tts_ws`: call iFlytek TTS over websocket
  - `volcengine_tts`: call Volcengine TTS over SSE
- `MIMO_API_KEY`: MiMo API key, recommended as a user environment variable
- `AGENT_SERVER_TTS_MIMO_BASE_URL`: default `https://api.xiaomimimo.com/v1`
- `AGENT_SERVER_TTS_MIMO_MODEL`: default `mimo-v2-tts`
- `AGENT_SERVER_TTS_MIMO_VOICE`: built-in voice such as `mimo_default`, `default_zh`, or `default_en`
- `AGENT_SERVER_TTS_MIMO_STYLE`: optional style prefix inserted as `<style>...</style>`
- `AGENT_SERVER_TTS_TIMEOUT_MS`: request timeout
- `AGENT_SERVER_TTS_IFLYTEK_APP_ID` or `IFLYTEK_TTS_APP_ID`
- `AGENT_SERVER_TTS_IFLYTEK_API_KEY` or `IFLYTEK_TTS_API_KEY`
- `AGENT_SERVER_TTS_IFLYTEK_API_SECRET` or `IFLYTEK_TTS_API_SECRET`
- `AGENT_SERVER_TTS_IFLYTEK_SCHEME`: default `ws`
- `AGENT_SERVER_TTS_IFLYTEK_HOST`: default `tts-api.xfyun.cn`
- `AGENT_SERVER_TTS_IFLYTEK_PORT`: default `80`
- `AGENT_SERVER_TTS_IFLYTEK_PATH`: default `/v2/tts`
- `AGENT_SERVER_TTS_IFLYTEK_VOICE`: default `xiaoyan`
- `AGENT_SERVER_TTS_IFLYTEK_AUE`: default `raw`
- `AGENT_SERVER_TTS_IFLYTEK_AUF`: optional override for `audio/L16;rate=<output sample rate>`
- `AGENT_SERVER_TTS_IFLYTEK_TEXT_ENCODING`: default `UTF8`
- `AGENT_SERVER_TTS_IFLYTEK_SPEED`: default `50`
- `AGENT_SERVER_TTS_IFLYTEK_VOLUME`: default `50`
- `AGENT_SERVER_TTS_IFLYTEK_PITCH`: default `50`
- `AGENT_SERVER_TTS_VOLCENGINE_BASE_URL`: default `https://openspeech.bytedance.com`
- `AGENT_SERVER_TTS_VOLCENGINE_ACCESS_TOKEN` or `VOLCENGINE_TTS_ACCESS_TOKEN`
- `AGENT_SERVER_TTS_VOLCENGINE_APP_ID` or `VOLCENGINE_TTS_APPID`
- `AGENT_SERVER_TTS_VOLCENGINE_RESOURCE_ID`: default `seed-tts-2.0`
- `AGENT_SERVER_TTS_VOLCENGINE_VOICE_TYPE`: default `zh_female_vv_uranus_bigtts`
- `AGENT_SERVER_TTS_VOLCENGINE_SPEECH_RATE`
- `AGENT_SERVER_TTS_VOLCENGINE_LOUDNESS_RATE`
- `AGENT_SERVER_TTS_VOLCENGINE_EMOTION`
- `AGENT_SERVER_TTS_VOLCENGINE_EMOTION_SCALE`: default `4`
- `AGENT_SERVER_TTS_VOLCENGINE_MODEL`

Current implementation details:

- the Go server now calls MiMo with streaming `pcm16` output for the realtime path
- the SSE stream is decoded incrementally and the first device frames can be forwarded without waiting for a full synthesis result
- the streamed PCM is emitted to the device in `20 ms` paced frames so barge-in can preempt the current response
- the older non-streaming `wav` decode path remains as a compatibility fallback inside the synthesizer implementation
- `iflytek_tts_ws` and `volcengine_tts` now share the same `StreamingSynthesizer` boundary, so the gateway keeps one audio pacing path regardless of provider protocol differences
- all currently supported cloud TTS backends are normalized to realtime output `pcm16le` frames before they reach the websocket gateway
