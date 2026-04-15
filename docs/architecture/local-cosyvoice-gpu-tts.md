# Local CosyVoice GPU TTS Bring-Up

## Goal

Add a directly usable local open-source GPU TTS path without changing the device-facing realtime protocol.

## Provider Boundary

- `agentd` selects `cosyvoice_http` at bootstrap
- provider-specific HTTP calls stay inside `internal/voice`
- gateways and channel adapters still consume only shared synthesized-audio contracts
- CosyVoice raw PCM output is normalized to the configured realtime output sample rate before downlink

## Current Integration Contract

- the current shared provider only targets the official FastAPI endpoints `/inference_sft` and `/inference_instruct`
- that means the safest default model family for this repository is still CosyVoice v1:
  - `iic/CosyVoice-300M-SFT`
  - `iic/CosyVoice-300M-Instruct`
- do not switch the default runtime to CosyVoice2 or CosyVoice3 until the shared `internal/voice` provider is expanded for their endpoint and prompt contract differences

## Current Supported Modes

- `sft`
- `instruct`

The current default mode is `sft`.

## Required `agentd` Environment

- `AGENT_SERVER_TTS_PROVIDER=cosyvoice_http`
- `AGENT_SERVER_TTS_COSYVOICE_BASE_URL=http://127.0.0.1:50000`
- `AGENT_SERVER_TTS_COSYVOICE_MODE=sft`
- `AGENT_SERVER_TTS_COSYVOICE_SPK_ID=中文女`

Optional:

- `AGENT_SERVER_TTS_COSYVOICE_INSTRUCT_TEXT`
- `AGENT_SERVER_TTS_COSYVOICE_SOURCE_SAMPLE_RATE=22050`

## Recommended Runtime Layout On This Host

This machine already runs GPU ASR on the data disk and has a `Tesla V100-SXM2-32GB` GPU. Keep TTS in the same operational shape:

- CosyVoice repo: `/home/ubuntu/kws-training/data/agent-server-runtime/CosyVoice`
- CosyVoice runtime env: `/home/ubuntu/kws-training/data/agent-server-runtime/cosyvoice-py310`
- shared caches:
  - `/home/ubuntu/kws-training/data/agent-server-cache/modelscope`
  - `/home/ubuntu/kws-training/data/agent-server-cache/hf`
  - `/home/ubuntu/kws-training/data/agent-server-cache/torch`

## Practical GPU Bring-Up

The helper script now supports either:

- a direct Python runtime via `COSYVOICE_PYTHON_BIN`, which is the preferred systemd path on this host
- or `conda run` if you still manage the env by conda name

It also performs a lightweight preflight check before starting the server:

- verifies the expected Python modules are installed
- verifies `torch.cuda.is_available()` when `COSYVOICE_REQUIRE_CUDA=true`
- when `COSYVOICE_PYTHON_BIN` points at a dedicated env, auto-prepends that env's `cusparselt/lib` path so V100 GPU torch import succeeds
- for `iic/CosyVoice-300M-SFT` and `iic/CosyVoice-300M-Instruct`, stages only the runtime-minimal file set by default instead of pulling the full upstream model repo
- for repo-id staging, preserves the upstream `org/model` directory shape under `COSYVOICE_MODEL_CACHE_DIR`, so repeated launches reuse the same staged path instead of creating parallel caches

### 1. Prepare the minimal runtime

The official `requirements.txt` is broader than the FastAPI inference path actually needs. For the realtime demo stage, start with a runtime-oriented subset around:

- `torch` + `torchaudio`
- `fastapi` + `uvicorn`
- `python-multipart`
- `modelscope`
- `hyperpyyaml`
- `onnxruntime-gpu`
- `transformers`
- `inflect`
- `wetext`
- `conformer`
- `x-transformers`
- `librosa`
- `soundfile`
- `scipy`
- `einops`
- `omegaconf`
- `pyarrow`
- `pyworld`
- `diffusers`
- `matplotlib`
- `tiktoken`
- `tqdm`
- `regex`
- `openai-whisper`

### Runtime model staging defaults

The launcher defaults to:

- `COSYVOICE_MODEL_DOWNLOAD_MODE=runtime_minimal`
- `COSYVOICE_MODEL_CACHE_DIR=/home/ubuntu/kws-training/data/agent-server-cache/modelscope/models`

For the current voice demo this is the right default, because upstream `snapshot_download()` on the raw repo id also fetches large JIT/TRT attachments that the FastAPI `sft` runtime does not need.

The runtime-minimal stage currently keeps:

- `campplus.onnx`
- `configuration.json`
- `cosyvoice.yaml`
- `flow.pt`
- `hift.pt`
- `llm.pt`
- `speech_tokenizer_v1.onnx`
- `spk2info.pt`

If you deliberately want the original full-repo behavior, set:

```bash
COSYVOICE_MODEL_DOWNLOAD_MODE=full_repo
```

### 2. Validate the runtime only

```bash
scripts/start-cosyvoice-fastapi.sh \
  --repo-dir /home/ubuntu/kws-training/data/agent-server-runtime/CosyVoice \
  --python-bin /home/ubuntu/kws-training/data/agent-server-runtime/cosyvoice-py310/bin/python \
  --download-mode runtime_minimal \
  --model-dir iic/CosyVoice-300M-SFT \
  --check
```

On this host, the first working runtime also needed `diffusers`, `pyarrow`, `pyworld`, `matplotlib`, and `openai-whisper` in the dedicated Python env before the FastAPI server could really load the model.

### 3. Start the local GPU TTS runtime

```bash
scripts/run-cosyvoice-fastapi-local.sh
```

Or directly:

```bash
scripts/start-cosyvoice-fastapi.sh \
  --repo-dir /home/ubuntu/kws-training/data/agent-server-runtime/CosyVoice \
  --python-bin /home/ubuntu/kws-training/data/agent-server-runtime/cosyvoice-py310/bin/python \
  --port 50000 \
  --download-mode runtime_minimal \
  --model-dir iic/CosyVoice-300M-SFT
```

### 4. Verify the FastAPI output

```bash
curl -fsS http://127.0.0.1:50000/inference_sft \
  -X POST \
  --data-urlencode 'tts_text=你好，我是本地GPU语音助手。' \
  --data-urlencode 'spk_id=中文女' \
  --output /tmp/cosyvoice-test.pcm
```

The response body is raw `pcm16` audio. A non-empty output file plus active GPU memory on `nvidia-smi` is the quickest first-pass validation.

### 5. Switch `agentd` to local TTS

After FastAPI is stable, set:

```bash
AGENT_SERVER_TTS_PROVIDER=cosyvoice_http
AGENT_SERVER_TTS_COSYVOICE_BASE_URL=http://127.0.0.1:50000
AGENT_SERVER_TTS_COSYVOICE_MODE=sft
AGENT_SERVER_TTS_COSYVOICE_SPK_ID=中文女
```

## Systemd Shape

This repository now includes a matching service scaffold:

- `deploy/systemd/agent-server-cosyvoice-fastapi.service`
- `deploy/systemd/agent-server-cosyvoice-fastapi.env.example`

Recommended install flow:

```bash
sudo install -d -m 0755 /etc/agent-server
sudo install -m 0644 deploy/systemd/agent-server-cosyvoice-fastapi.env.example /etc/agent-server/cosyvoice-fastapi.env
sudo install -m 0644 deploy/systemd/agent-server-cosyvoice-fastapi.service /etc/systemd/system/agent-server-cosyvoice-fastapi.service
sudo systemctl daemon-reload
sudo systemctl enable --now agent-server-cosyvoice-fastapi.service
```

## Notes

- this slice adds one local open-source GPU TTS option; it does not yet replace existing cloud TTS providers
- the current provider integrates against the official CosyVoice FastAPI service instead of vendoring the full model stack into `agent-server`
- the current implementation keeps the shared realtime output profile unchanged, so adapters still receive normalized `pcm16le`
- if the runtime is not ready yet, keep `AGENT_SERVER_TTS_PROVIDER=none` in `agentd` so the realtime session core stays stable while TTS is being prepared
- `openai-whisper` is still required for the current CosyVoice v1 runtime because `cosyvoice.yaml` references `whisper.tokenizer.get_tokenizer`, even when the live demo only uses `sft`
