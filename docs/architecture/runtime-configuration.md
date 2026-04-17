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
- discovery now also exposes a structured `server_endpoint` object so tooling can see whether shared server endpointing is unsupported, available-but-disabled, or enabled as the current main-path candidate
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
- `AGENT_SERVER_AGENT_SKILLS`: comma-separated runtime skill set injected on top of the shared core; default empty, current built-in option is `household_control`
- `AGENT_SERVER_AGENT_LLM_PROVIDER`
  - `auto`: default behaviour; prefer `deepseek_chat` when a DeepSeek key is present, otherwise fall back to `bootstrap`
  - `bootstrap`: existing placeholder or bring-up executor
  - `deepseek_chat`: shared chat-completions-backed executor; it can target DeepSeek itself or a local OpenAI-compatible `/chat/completions` service
- `AGENT_SERVER_AGENT_LLM_TIMEOUT_MS`: timeout for one LLM request
- `AGENT_SERVER_AGENT_PERSONA`: built-in persona profile selector, one of `general_assistant` or `household_control_screen`, default `general_assistant`
- `AGENT_SERVER_AGENT_EXECUTION_MODE`: runtime execution policy, one of `simulation`, `dry_run`, or `live_control`, default `dry_run`
- `AGENT_SERVER_AGENT_ASSISTANT_NAME`: assistant display or speaking name used by the built-in prompt template, default `小欧助手`
- `AGENT_SERVER_AGENT_LLM_SYSTEM_PROMPT`: optional persona-template override; execution-mode policy is still appended by the runtime
- `AGENT_SERVER_AGENT_DEEPSEEK_BASE_URL`: default `https://api.deepseek.com`
- `AGENT_SERVER_AGENT_DEEPSEEK_API_KEY` or `DEEPSEEK_API_KEY`
- `AGENT_SERVER_AGENT_DEEPSEEK_MODEL`: default `deepseek-chat`
- `AGENT_SERVER_AGENT_DEEPSEEK_TEMPERATURE`: default `0.2`
- `AGENT_SERVER_AGENT_DEEPSEEK_MAX_TOKENS`: optional max token cap, `0` means provider default

Current runtime note:

- the current `deepseek_chat` integration stays inside `internal/agent`; device gateways, channel adapters, and the voice runtime still depend only on the shared `TurnExecutor`
- when `AGENT_SERVER_AGENT_LLM_PROVIDER` is unset or `auto`, the runtime chooses `deepseek_chat` if a DeepSeek key is present; otherwise it stays on `bootstrap`
- when `AGENT_SERVER_AGENT_LLM_SYSTEM_PROMPT` is empty, the runtime injects a built-in assistant persona selected by `AGENT_SERVER_AGENT_PERSONA`
- the repository defaults are intentionally generic: no built-in vertical skill is enabled by default, and household behavior must be opted into explicitly through `AGENT_SERVER_AGENT_SKILLS` and, if desired, `AGENT_SERVER_AGENT_PERSONA=household_control_screen`
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

### Local OpenAI-Compatible LLM Bring-Up

The current agent runtime does not require a separate provider type for a local model server as long as that server exposes an OpenAI-compatible `POST /chat/completions` or `POST /v1/chat/completions` endpoint.

That means the existing `deepseek_chat` path can already front a local GPU model service without widening the shared runtime boundary again.

Current machine-level example for a local Qwen3 worker:

```bash
AGENT_SERVER_AGENT_LLM_PROVIDER=deepseek_chat
AGENT_SERVER_AGENT_DEEPSEEK_BASE_URL=http://127.0.0.1:8012/v1
AGENT_SERVER_AGENT_DEEPSEEK_API_KEY=local-llm
AGENT_SERVER_AGENT_DEEPSEEK_MODEL=Qwen/Qwen3-4B-Instruct-2507
AGENT_SERVER_AGENT_LLM_TIMEOUT_MS=60000
```

Current host recommendation for the phase-1 voice demo:

- prefer `Qwen/Qwen3-4B-Instruct-2507` first on the single V100 host because the same GPU is already shared with local FunASR ASR and CosyVoice TTS
- keep `Qwen/Qwen3-8B` as a follow-up upgrade target once the voice round-trip and memory headroom are revalidated under real traffic
- keep `force no-think` behavior enabled in the local worker so the assistant does not emit hidden reasoning text or burn extra latency on a speech-first path

## Voice Runtime

- `AGENT_SERVER_VOICE_PROVIDER`
  - `bootstrap`: existing placeholder responder
  - `funasr_http`: local HTTP ASR worker backed by FunASR
  - `iflytek_rtasr`: iFlytek RTASR websocket ASR provider
- `AGENT_SERVER_VOICE_ASR_URL`: FunASR worker transcription endpoint
- `AGENT_SERVER_VOICE_ASR_TIMEOUT_MS`: timeout for one ASR request
- `AGENT_SERVER_VOICE_ASR_LANGUAGE`: language hint, default `auto`
- `AGENT_SERVER_VOICE_ASR_READY_URL`: optional health endpoint override used by `scripts/run-agentd-local.sh` before `agentd` starts
- `AGENT_SERVER_VOICE_ASR_READY_TIMEOUT_SEC`: how long the local launcher waits for FunASR readiness, default `180`
- `AGENT_SERVER_VOICE_ASR_READY_POLL_INTERVAL_SEC`: local launcher poll interval for worker readiness, default `2`
- `AGENT_SERVER_VOICE_SERVER_ENDPOINT_ENABLED`: shared server-endpoint candidate switch, default `false`
- `AGENT_SERVER_VOICE_SERVER_ENDPOINT_MIN_AUDIO_MS`: minimum accumulated audio before the shared candidate path may suggest auto-commit, default `320`
- `AGENT_SERVER_VOICE_SERVER_ENDPOINT_SILENCE_MS`: trailing local silence window before the shared candidate path may suggest auto-commit, default `480`
- `AGENT_SERVER_VOICE_SERVER_ENDPOINT_LEXICAL_MODE`: lexical false-endpoint guard mode for the shared candidate path, default `conservative`
- `AGENT_SERVER_VOICE_SERVER_ENDPOINT_INCOMPLETE_HOLD_MS`: extra hold window applied when the latest partial still looks unfinished, default `720`
- `AGENT_SERVER_VOICE_SERVER_ENDPOINT_HINT_SILENCE_MS`: shortened silence window used when a provider preview carries an explicit endpoint hint and the latest partial already looks complete, default `160`
- `AGENT_SERVER_VOICE_BARGE_IN_MIN_AUDIO_MS`: minimum staged interrupt audio before lexically complete barge-in is accepted, default `120`
- `AGENT_SERVER_VOICE_BARGE_IN_HOLD_AUDIO_MS`: extra staged audio hold applied when the current interrupt preview still looks incomplete, default `240`
- `AGENT_SERVER_VOICE_LLM_SEMANTIC_JUDGE_ENABLED`: enable the runtime-owned semantic judge capability, default `true`
- `AGENT_SERVER_VOICE_SEMANTIC_JUDGE_PROVIDER`: semantic judge model provider (`openai_compat`, `deepseek_chat`, or empty)
- `AGENT_SERVER_VOICE_SEMANTIC_JUDGE_BASE_URL`: semantic judge model base URL
- `AGENT_SERVER_VOICE_SEMANTIC_JUDGE_API_KEY`: semantic judge model API key when required
- `AGENT_SERVER_VOICE_SEMANTIC_JUDGE_MODEL`: semantic judge model name
- `AGENT_SERVER_VOICE_LLM_SEMANTIC_JUDGE_ROLLOUT_MODE`: semantic judge rollout mode, one of `control`, `semantic`, or `sticky_percent`; default `control`
- `AGENT_SERVER_VOICE_LLM_SEMANTIC_JUDGE_ROLLOUT_PERCENT`: sticky-percent rollout bucket threshold, default `0`
- `AGENT_SERVER_VOICE_LLM_SEMANTIC_JUDGE_TIMEOUT_MS`: semantic judge timeout, default `220`
- `AGENT_SERVER_VOICE_LLM_SEMANTIC_JUDGE_MIN_RUNES`: minimum mature preview length before judge launch, default `2`
- `AGENT_SERVER_VOICE_LLM_SEMANTIC_JUDGE_MIN_STABLE_FOR_MS`: minimum stable dwell before judge launch unless other gates already promote the preview, default `120`
- `AGENT_SERVER_VOICE_LLM_SLOT_PARSER_ENABLED`: enable the runtime-owned slot parser capability, default `true`
- `AGENT_SERVER_VOICE_LLM_SLOT_PARSER_PROVIDER`: slot parser model provider
- `AGENT_SERVER_VOICE_LLM_SLOT_PARSER_BASE_URL`: slot parser model base URL
- `AGENT_SERVER_VOICE_LLM_SLOT_PARSER_API_KEY`: slot parser model API key when required
- `AGENT_SERVER_VOICE_LLM_SLOT_PARSER_MODEL`: slot parser model name
- `AGENT_SERVER_VOICE_LLM_SLOT_PARSER_TIMEOUT_MS`: slot parser timeout, default `280`
- `AGENT_SERVER_VOICE_LLM_SLOT_PARSER_MIN_RUNES`: minimum mature preview length before slot parsing, default `4`
- `AGENT_SERVER_VOICE_LLM_SLOT_PARSER_MIN_STABLE_FOR_MS`: minimum stable dwell before slot parsing, default `160`
- `AGENT_SERVER_VOICE_ENTITY_CATALOG_PROFILE`: built-in grounding profile (`off` default, or `seed_companion`)
- `AGENT_SERVER_VOICE_SPEECH_PLANNER_ENABLED`: enable shared clause-level incremental TTS planning when a synthesizer is configured, default `true`
- `AGENT_SERVER_VOICE_SPEECH_PLANNER_MIN_CHUNK_RUNES`: minimum stable clause size before the speech planner emits an early segment, default `6`
- `AGENT_SERVER_VOICE_SPEECH_PLANNER_TARGET_CHUNK_RUNES`: preferred clause size for early speech-planner chunking, default `24`
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

Current directly usable machine-local long-running setup:

1. install and enable the worker plus `agentd` services with `sudo env PATH="$PATH" bash scripts/install-local-systemd-stack.sh`
2. optionally expose `80/443` through `nginx` with `sudo PUBLIC_IP=<your-public-ip> bash scripts/install-local-nginx-proxy.sh`
3. adjust `/etc/agent-server/funasr-worker.env` or `/etc/agent-server/agentd.env` when local runtime defaults need to persist across reboots

When `AGENT_SERVER_VOICE_PROVIDER=funasr_http`, the local launcher now waits for the worker health endpoint to reach `status=ok` before it execs `agentd`.
That readiness gate uses:

- `AGENT_SERVER_VOICE_ASR_READY_URL` when explicitly set
- otherwise a derived `/healthz` URL from `AGENT_SERVER_VOICE_ASR_URL`

This readiness wait is currently a launcher-side behavior in `scripts/run-agentd-local.sh`; it is not a new public API surface.

For the current machine-local `systemd` path, `scripts/run-agentd-local.sh` now also supports a repo-local user override binary at `.runtime/bin/agentd`.
If `/etc/agent-server/agentd.env` still points at the default `/home/ubuntu/agent-server/bin/agentd`, the launcher may transparently prefer that repo-local override when it exists. This keeps unprivileged iteration possible on machines where the root-owned `bin/` directory cannot be updated directly.

For `opus` device uplink, the current server path supports mono speech-oriented `SILK-only` packets and normalizes them in Go before calling the worker. The Python worker still receives `pcm16le` JSON payloads.

Current server-endpoint candidate note:

- when `AGENT_SERVER_VOICE_SERVER_ENDPOINT_ENABLED=true`, websocket adapters may start an internal input-preview session behind the shared `internal/voice` boundary and auto-commit an audio turn after a local silence window
- preview polling, auto-commit suggestions, playout interruption, playout completion, and heard-text persistence now all flow through one shared `internal/voice.SessionOrchestrator` boundary instead of being split across multiple gateway handlers
- the shared candidate endpoint policy is runtime-configurable through `AGENT_SERVER_VOICE_SERVER_ENDPOINT_MIN_AUDIO_MS`, `AGENT_SERVER_VOICE_SERVER_ENDPOINT_SILENCE_MS`, `AGENT_SERVER_VOICE_SERVER_ENDPOINT_LEXICAL_MODE`, and `AGENT_SERVER_VOICE_SERVER_ENDPOINT_INCOMPLETE_HOLD_MS`, and those settings are applied uniformly to both `funasr_http` and `iflytek_rtasr` responder wiring
- `AGENT_SERVER_VOICE_SERVER_ENDPOINT_HINT_SILENCE_MS` lets the shared detector use a shorter silence window when a provider preview already carries an explicit endpoint hint for a lexically complete partial
- the current default candidate policy is intentionally conservative: if the latest partial still looks lexically unfinished, the preview path waits an additional hold window before suggesting auto-commit
- the current online trace baseline now also exposes fused endpoint-controller timing and wait-budget evidence in gateway logs, including:
  - `preview_candidate_ready_latency_ms`
  - `preview_draft_ready_latency_ms`
  - `preview_accept_ready_latency_ms`
  - `preview_base_wait_ms`
  - `preview_semantic_wait_delta_ms`
  - `preview_slot_guard_adjust_ms`
  - `preview_effective_wait_ms`
  - `preview_hold_reason`
  - `preview_accept_reason`
- discovery and `/v1/info` now expose this path as `server_endpoint.main_path_candidate=true` when the selected voice provider supports shared preview-driven endpointing, even if the instance keeps the candidate disabled
- preview polling no longer relies on websocket read timeouts for the non-terminal preview loop; native realtime and `xiaozhi` now keep the socket reusable after hidden auto-commit and a later client-driven close
- the semantic judge now also has a runtime-owned rollout switch instead of only a global on/off toggle:
  - `control`: never launch the semantic judge, but still expose `variant=control`
  - `semantic`: always use the semantic judge when configured
  - `sticky_percent`: choose once per preview session using a stable `session/device` hash and keep that variant fixed for the whole session
- inbound audio barge-in now stages candidate interrupt audio and applies one shared adaptive threshold instead of interrupting on the first frame:
  - lexically complete interrupt previews may cut in after `AGENT_SERVER_VOICE_BARGE_IN_MIN_AUDIO_MS`
  - incomplete interrupt previews must clear an additional `AGENT_SERVER_VOICE_BARGE_IN_HOLD_AUDIO_MS`
  - explicit `audio.in.commit` while speaking can still accept a short but intentional interruption if staged interrupt audio already exists
- the local FunASR worker now supports two internal stream modes behind the same shared `StreamingTranscriber` boundary:
  - `stream_preview_batch`: backward-compatible buffered preview, still the default while `AGENT_SERVER_FUNASR_ONLINE_MODEL` stays empty
  - `stream_2pass_online_final`: optional 2pass mode enabled by `AGENT_SERVER_FUNASR_ONLINE_MODEL`, where preview comes from a true online ASR model and turn-final text still comes from the configured final-ASR model
- the worker-side 2pass knobs are:
  - `AGENT_SERVER_FUNASR_ONLINE_MODEL`
  - `AGENT_SERVER_FUNASR_PRELOAD_MODELS`
  - `AGENT_SERVER_FUNASR_STREAM_CHUNK_SIZE`
  - `AGENT_SERVER_FUNASR_STREAM_ENCODER_CHUNK_LOOK_BACK`
  - `AGENT_SERVER_FUNASR_STREAM_DECODER_CHUNK_LOOK_BACK`
  - `AGENT_SERVER_FUNASR_FINAL_VAD_MODEL`
  - `AGENT_SERVER_FUNASR_FINAL_PUNC_MODEL`
  - `AGENT_SERVER_FUNASR_FINAL_MERGE_VAD`
  - `AGENT_SERVER_FUNASR_FINAL_MERGE_LENGTH_S`
- the local FunASR worker now also emits a lightweight preview endpoint hint (`preview_tail_silence`) based on tail-audio energy, and the shared voice runtime consumes that hint without widening the public protocol
- the worker now background-preloads configured models by default through `AGENT_SERVER_FUNASR_PRELOAD_MODELS=true`
- `/healthz` and `/v1/asr/info` now report `status=ok` only after all configured worker-side model dependencies for the active path are ready, not merely after the final ASR model object exists
- the local FunASR worker can now optionally strengthen that internal hint path through `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_PROVIDER`:
  - `energy`: keep the existing lightweight tail-energy hint as the default behavior
  - `fsmn_vad`: use the configured `AGENT_SERVER_FUNASR_FINAL_VAD_MODEL` as the worker-side endpoint hint source
  - `silero`: try `Silero VAD` inside the worker and emit `preview_silero_vad_silence` when the buffered speech already ends in enough local silence
  - `auto`: prefer `fsmn_vad` when `AGENT_SERVER_FUNASR_FINAL_VAD_MODEL` is configured, otherwise try `Silero VAD`, then fall back to `energy`
  - `none`: disable worker-side preview endpoint hints entirely
- additional worker-side VAD knobs are `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_THRESHOLD`, `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_MIN_SILENCE_MS`, and `AGENT_SERVER_FUNASR_STREAM_ENDPOINT_VAD_SPEECH_PAD_MS`
- even when `silero` or `fsmn_vad` is configured, unsupported sample rates, missing local VAD dependencies, or an absent final VAD model still fall back to the existing `preview_tail_silence` path instead of changing the shared contract or breaking stream preview
- the worker can now also run optional KWS ahead of final transcript normalization:
  - `AGENT_SERVER_FUNASR_KWS_ENABLED=false` keeps KWS off by default
  - `AGENT_SERVER_FUNASR_KWS_MODEL` selects the KWS model when enabled
  - `AGENT_SERVER_FUNASR_KWS_KEYWORDS` provides a comma-separated keyword list
  - `AGENT_SERVER_FUNASR_KWS_STRIP_MATCHED_PREFIX=true` lets the worker strip a matched wake-word prefix from preview/final transcript text after KWS succeeds
  - `AGENT_SERVER_FUNASR_KWS_MIN_AUDIO_MS` and `AGENT_SERVER_FUNASR_KWS_MIN_INTERVAL_MS` gate repeated KWS checks on one stream
- KWS stays internal to the worker/runtime path: it may surface as worker `audio_events` such as `kws_detected:<keyword>`, but it does not widen the public realtime or `xiaozhi` contracts
- this path is now discovery-advertised as a structured candidate capability, but `turn_mode` still advertises `client_wakeup_client_commit`
- the first implementation slice uses a silence-based detector on top of streaming ASR partials; it is a stepping stone toward fuller server-side endpointing, not the final policy
- the Python desktop runner now exposes a non-default `server-endpoint-preview` scenario for this hidden mode; it intentionally stays out of `full` and `regression`, and it should be used together with `AGENT_SERVER_VOICE_SERVER_ENDPOINT_ENABLED=true` plus a speech-like `--wav` sample
- when a TTS provider is configured, the shared voice runtime can now pre-synthesize stable clauses incrementally behind `AGENT_SERVER_VOICE_SPEECH_PLANNER_*`; the public websocket contract still stays unchanged and playback ownership remains inside `internal/voice`

Current note on this machine:

- `FunASR` inference has been validated locally on both CPU and GPU.
- The current 2026-04-15 production host uses `Tesla V100-SXM2-32GB` (`sm_70`), and the original `xiaozhi-esp32-server` conda env should no longer be treated as the GPU runtime of record: it was found with `torch 2.11.0+cpu` plus a broken `torchaudio` import after an interrupted reinstall.
- The active long-running GPU worker now runs from `FUNASR_PYTHON_BIN=/home/ubuntu/kws-training/data/agent-server-runtime/funasr-gpu-py311/bin/python` with caches rooted under `/home/ubuntu/kws-training/data/agent-server-cache/{modelscope,hf,torch}` so large wheels and model downloads no longer consume the root disk.
- The validated GPU runtime on this V100 host is `torch 2.7.1+cu126` plus `torchaudio 2.7.1+cu126`; the newer `torch 2.11.0+cu128` wheel family was rejected because the official wheel omits `sm_70` kernels and fails with `CUDA error: no kernel image is available for execution on the device`.
- With that runtime, `SenseVoiceSmall + paraformer-zh-streaming + fsmn-vad` now preload successfully on `cuda:0`, and the long-running worker reports `pipeline_mode=stream_2pass_online_final` with worker-side endpoint VAD status `ready`.
- The calibrated enabled-KWS baseline on this host is now `iic/speech_charctc_kws_phone-xiaoyun`; the worker initializes `AutoModel(...)` with both `keywords` and `output_dir`, and `/healthz` reports `status=error` if `AGENT_SERVER_FUNASR_KWS_ENABLED=true` but `KWS_MODEL` or `KWS_KEYWORDS` is still missing.
- The default worker scripts still keep `device=cpu` for portable bring-up and easy fallback; GPU deployment should opt in explicitly through the worker env overrides above.
- The local `SenseVoiceSmall` worker path must keep `AGENT_SERVER_FUNASR_TRUST_REMOTE_CODE=false`; the downloaded model bundle loads correctly from cache, but enabling remote code fails with `No module named 'model'`.
- The same host now also runs a long-running local CosyVoice GPU TTS runtime through `agent-server-cosyvoice-fastapi.service`, backed by `/home/ubuntu/kws-training/data/agent-server-runtime/cosyvoice-py310` plus the staged model `/home/ubuntu/kws-training/data/agent-server-cache/modelscope/models/iic/CosyVoice-300M-SFT-runtime`.
- The long-running `agentd` path is now configured with `AGENT_SERVER_TTS_PROVIDER=cosyvoice_http`, `AGENT_SERVER_TTS_COSYVOICE_BASE_URL=http://127.0.0.1:50000`, `AGENT_SERVER_TTS_COSYVOICE_MODE=sft`, `AGENT_SERVER_TTS_COSYVOICE_SPK_ID=中文女`, and `AGENT_SERVER_TTS_COSYVOICE_SOURCE_SAMPLE_RATE=22050`.
- Direct `POST /inference_sft` validation on this host now returns non-empty raw PCM from the local FastAPI runtime, and local systemd-backed realtime text plus audio smoke runs both return audio.
- The current machine has also revalidated the public edge after the GPU cutover: `http://101.33.235.154/healthz` and `https://101.33.235.154/healthz` both return `status=ok`, and public-IP realtime text smoke now returns audio with `tts_provider=cosyvoice_http`.
- On 2026-04-15, the shared realtime TTS chain on this host was corrected in two places:
  - the gateway now keeps the `StreamingResponder` audio-stream context alive until the returned `AudioStream` closes instead of canceling it immediately after `RespondStream(...)` returns
  - speech-planner turns now avoid the old duplicate full-response TTS call when a planned incremental stream is already available
- After that fix, the same committed-WAV audio path now succeeds locally and through the public edge again:
  - local audio smoke: `artifacts/live-smoke/20260415/local-systemd-cosyvoice-audio-postfix/run_120700_3677/run_935960053343`
  - public-IP audio smoke: `artifacts/live-smoke/20260415/public-edge-cosyvoice-audio-postfix/run_120710_10469/run_846a70c2f15d`
  - public-IP text smoke recheck: `artifacts/live-smoke/20260415/public-edge-cosyvoice-text-postfix/run_120720_31537/run_3db52dc40d16`
- The repaired runs now show one `tts stream started` per turn, no `context canceled` TTS failure for the validated turns, and large non-zero returned audio (`122600` bytes locally, `123344` bytes on the public-IP audio smoke).

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
