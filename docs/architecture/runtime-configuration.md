# Runtime Configuration

This document explains the environment variables currently reserved for the first RTOS realtime profile.

## Core Service

- `AGENT_SERVER_ADDR`: HTTP listen address
- `AGENT_SERVER_ENV`: environment label such as `dev` or `prod`
- `AGENT_SERVER_NAME`: service name
- `AGENT_SERVER_VERSION`: build or release version

Startup validation note:

- `agentd` now validates the composed runtime configuration before mounting handlers
- invalid cross-domain combinations fail fast during process startup instead of surfacing later in request handling
- current startup-time checks include:
  - explicit `deepseek_chat` selection without a DeepSeek API key
  - selected TTS provider without its required credentials
  - `xiaozhi` enabled while shared realtime output is not `pcm16le`
  - hidden server-endpoint preview enabled on a non-streaming voice provider

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
- `AGENT_SERVER_AGENT_SKILLS`: comma-separated runtime skill set injected on top of the shared core; current built-in option is `household_control`
- `AGENT_SERVER_AGENT_LLM_PROVIDER`
  - `auto`: default behaviour; prefer `deepseek_chat` when a DeepSeek key is present, otherwise fall back to `bootstrap`
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
- when `AGENT_SERVER_AGENT_LLM_PROVIDER` is unset or `auto`, the runtime chooses `deepseek_chat` if a DeepSeek key is present; otherwise it stays on `bootstrap`
- when `AGENT_SERVER_AGENT_LLM_SYSTEM_PROMPT` is empty, the runtime injects a built-in assistant persona selected by `AGENT_SERVER_AGENT_PERSONA`
- prompt composition inside the shared runtime is now layered as:
  - core persona section
  - runtime output-contract section
  - execution-mode policy section
  - runtime-skill prompt sections
- the runtime always appends an execution-mode policy selected by `AGENT_SERVER_AGENT_EXECUTION_MODE`
- custom prompt overrides may include `{{assistant_name}}`, which is replaced at runtime from `AGENT_SERVER_AGENT_ASSISTANT_NAME`
- custom prompt overrides replace the persona section, but do not disable the runtime-owned output-contract or execution-mode policy sections
- domain behavior such as household-control semantics should be added through runtime skills, which can contribute prompt fragments and tools without hardcoding domain branches into the core executor path
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
- `AGENT_SERVER_VOICE_SERVER_ENDPOINT_ENABLED`: internal server-endpoint preview switch, default `false`
- `AGENT_SERVER_VOICE_SERVER_ENDPOINT_MIN_AUDIO_MS`: minimum accumulated audio before the hidden preview path may suggest auto-commit, default `320`
- `AGENT_SERVER_VOICE_SERVER_ENDPOINT_SILENCE_MS`: trailing local silence window before the hidden preview path may suggest auto-commit, default `480`
- `AGENT_SERVER_VOICE_SERVER_ENDPOINT_LEXICAL_MODE`: hidden lexical false-endpoint guard mode, default `conservative`
- `AGENT_SERVER_VOICE_SERVER_ENDPOINT_INCOMPLETE_HOLD_MS`: extra hold window applied when the latest partial still looks unfinished, default `720`
- `AGENT_SERVER_VOICE_SERVER_ENDPOINT_HINT_SILENCE_MS`: shortened silence window used when a provider preview carries an explicit endpoint hint and the latest partial already looks complete, default `160`
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

Current internal endpoint-preview note:

- when `AGENT_SERVER_VOICE_SERVER_ENDPOINT_ENABLED=true`, websocket adapters may start an internal input-preview session behind the shared `internal/voice` boundary and auto-commit an audio turn after a local silence window
- preview polling, auto-commit suggestions, playout interruption, playout completion, and heard-text persistence now all flow through one shared `internal/voice.SessionOrchestrator` boundary instead of being split across multiple gateway handlers
- the first hidden endpoint policy is runtime-configurable through `AGENT_SERVER_VOICE_SERVER_ENDPOINT_MIN_AUDIO_MS`, `AGENT_SERVER_VOICE_SERVER_ENDPOINT_SILENCE_MS`, `AGENT_SERVER_VOICE_SERVER_ENDPOINT_LEXICAL_MODE`, and `AGENT_SERVER_VOICE_SERVER_ENDPOINT_INCOMPLETE_HOLD_MS`, and those settings are applied uniformly to both `funasr_http` and `iflytek_rtasr` responder wiring
- `AGENT_SERVER_VOICE_SERVER_ENDPOINT_HINT_SILENCE_MS` lets the shared detector use a shorter silence window when a provider preview already carries an explicit endpoint hint for a lexically complete partial
- the current default hidden policy is intentionally conservative: if the latest partial still looks lexically unfinished, the preview path waits an additional hold window before suggesting auto-commit
- the local FunASR worker now also emits a lightweight preview endpoint hint (`preview_tail_silence`) based on tail-audio energy, and the shared voice runtime consumes that hint without widening the public protocol
- the local FunASR worker can now optionally strengthen that internal hint path through `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_PROVIDER`:
  - `energy`: keep the existing lightweight tail-energy hint as the default behavior
  - `silero`: try `Silero VAD` inside the worker and emit `preview_silero_vad_silence` when the buffered speech already ends in enough local silence
  - `auto`: prefer `Silero VAD` when its runtime is available, otherwise fall back to `energy`
  - `none`: disable worker-side preview endpoint hints entirely
- additional worker-side VAD knobs are `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_THRESHOLD`, `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_MIN_SILENCE_MS`, and `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_SPEECH_PAD_MS`
- even when `silero` is configured, unsupported sample rates or missing local VAD dependencies still fall back to the existing `preview_tail_silence` path instead of changing the shared contract or breaking stream preview
- this path currently stays intentionally undisclosed at the public discovery layer, so `turn_mode` still advertises `client_wakeup_client_commit`
- the first implementation slice uses a silence-based detector on top of streaming ASR partials; it is a stepping stone toward fuller server-side endpointing, not the final policy
- the Python desktop runner now exposes a non-default `server-endpoint-preview` scenario for this hidden mode; it intentionally stays out of `full` and `regression`, and it should be used together with `AGENT_SERVER_VOICE_SERVER_ENDPOINT_ENABLED=true` plus a speech-like `--wav` sample

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
  - `cosyvoice_http`: call a local GPU-side CosyVoice FastAPI service
  - `iflytek_tts_ws`: call iFlytek TTS over websocket
  - `volcengine_tts`: call Volcengine TTS over SSE
- `MIMO_API_KEY`: MiMo API key, recommended as a user environment variable
- `AGENT_SERVER_TTS_MIMO_BASE_URL`: default `https://api.xiaomimimo.com/v1`
- `AGENT_SERVER_TTS_MIMO_MODEL`: default `mimo-v2-tts`
- `AGENT_SERVER_TTS_MIMO_VOICE`: built-in voice such as `mimo_default`, `default_zh`, or `default_en`
- `AGENT_SERVER_TTS_MIMO_STYLE`: optional style prefix inserted as `<style>...</style>`
- `AGENT_SERVER_TTS_TIMEOUT_MS`: request timeout
- `AGENT_SERVER_TTS_COSYVOICE_BASE_URL`: default `http://127.0.0.1:50000`
- `AGENT_SERVER_TTS_COSYVOICE_MODE`: one of `sft` or `instruct`, default `sft`
- `AGENT_SERVER_TTS_COSYVOICE_SPK_ID`: CosyVoice speaker id, default `中文女`
- `AGENT_SERVER_TTS_COSYVOICE_INSTRUCT_TEXT`: required when mode is `instruct`
- `AGENT_SERVER_TTS_COSYVOICE_SOURCE_SAMPLE_RATE`: source PCM sample rate returned by CosyVoice, default `22050`
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

- TTS belongs to the shared voice runtime output layer, not to a specific browser, RTOS, or channel adapter.
- `cosyvoice_http` is the first local open-source GPU TTS option in the shared runtime, and it integrates against the official CosyVoice FastAPI service instead of teaching adapters about model-serving details.
- the current CosyVoice slice supports `sft` and `instruct` modes and normalizes CosyVoice raw PCM output to the configured realtime output sample rate before downlink.
- the Go server now calls MiMo with streaming `pcm16` output for the realtime path
- the SSE stream is decoded incrementally and the first device frames can be forwarded without waiting for a full synthesis result
- the streamed PCM is emitted to the device in `20 ms` paced frames so barge-in can preempt the current response
- the older non-streaming `wav` decode path remains as a compatibility fallback inside the synthesizer implementation
- `iflytek_tts_ws` and `volcengine_tts` now share the same `StreamingSynthesizer` boundary, so the gateway keeps one audio pacing path regardless of provider protocol differences
- all currently supported cloud TTS backends are normalized to realtime output `pcm16le` frames before they reach the websocket gateway
- playback completion and interruption now also feed a shared voice-runtime memory writeback path so the runtime can distinguish generated text, delivered text, and heard text

## Docker Deployment Notes

The first formal Docker deployment slice is intentionally layered and keeps the runtime boundary intact:

- `deploy/docker/agentd.Dockerfile`: production-oriented `agentd` image
- `deploy/docker/funasr-worker.cpu.Dockerfile`: separate CPU `FunASR` worker image
- `deploy/docker/compose.base.yml`: `agentd` only
- `deploy/docker/compose.local-asr.yml`: overlays the local CPU `funasr-worker`
- `deploy/docker/.env.docker.example`: docker-specific env template

Container-networking rules for this slice:

- when `agentd` talks to the local ASR worker inside compose, `AGENT_SERVER_VOICE_ASR_URL` must use the compose service DNS name `http://funasr-worker:8091/v1/asr/transcribe`
- do not use `127.0.0.1` for that worker URL unless the worker actually runs in the same container
- the worker image mounts named volumes for `MODELSCOPE_CACHE`, `HF_HOME`, and `TORCH_HOME` under `/models/...` so model downloads survive container replacement
- compose build definitions now also pass through standard proxy variables (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`, plus lowercase variants) so restricted-network hosts can reuse the same image assets
- the `agentd` image build defaults `GOPROXY` to `https://goproxy.cn,direct` and `GOSUMDB` to `sum.golang.google.cn`; override them through compose build args if your environment prefers different Go module infrastructure

Current scope limit:

- this slice covers `agentd` alone and `agentd + local CPU FunASR worker`
- an additional layered GPU TTS overlay now exists for `agentd + local CosyVoice FastAPI`
- GPU worker containerization remains a later follow-up so CUDA packaging, driver passthrough, and model-cache behavior can be validated separately without weakening the baseline deployment path
