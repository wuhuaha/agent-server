#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

export COSYVOICE_REPO_DIR="${COSYVOICE_REPO_DIR:-/home/ubuntu/kws-training/data/agent-server-runtime/CosyVoice}"
export COSYVOICE_PYTHON_BIN="${COSYVOICE_PYTHON_BIN:-/home/ubuntu/kws-training/data/agent-server-runtime/cosyvoice-py310/bin/python}"
export COSYVOICE_PORT="${COSYVOICE_PORT:-50000}"
export COSYVOICE_MODEL_DIR="${COSYVOICE_MODEL_DIR:-iic/CosyVoice-300M-SFT}"
export COSYVOICE_MODEL_DOWNLOAD_MODE="${COSYVOICE_MODEL_DOWNLOAD_MODE:-runtime_minimal}"
export COSYVOICE_MODEL_CACHE_DIR="${COSYVOICE_MODEL_CACHE_DIR:-/home/ubuntu/kws-training/data/agent-server-cache/modelscope/models}"
export COSYVOICE_MODEL_DOWNLOAD_MAX_WORKERS="${COSYVOICE_MODEL_DOWNLOAD_MAX_WORKERS:-4}"
export COSYVOICE_REQUIRE_CUDA="${COSYVOICE_REQUIRE_CUDA:-true}"
export MODELSCOPE_CACHE="${MODELSCOPE_CACHE:-/home/ubuntu/kws-training/data/agent-server-cache/modelscope}"
export HF_HOME="${HF_HOME:-/home/ubuntu/kws-training/data/agent-server-cache/hf}"
export TORCH_HOME="${TORCH_HOME:-/home/ubuntu/kws-training/data/agent-server-cache/torch}"
export CUDA_VISIBLE_DEVICES="${CUDA_VISIBLE_DEVICES:-0}"

cd "${REPO_ROOT}"
exec "${REPO_ROOT}/scripts/start-cosyvoice-fastapi.sh" \
  --repo-dir "${COSYVOICE_REPO_DIR}" \
  --python-bin "${COSYVOICE_PYTHON_BIN}" \
  --port "${COSYVOICE_PORT}" \
  --download-mode "${COSYVOICE_MODEL_DOWNLOAD_MODE}" \
  --model-cache-dir "${COSYVOICE_MODEL_CACHE_DIR}" \
  --model-dir "${COSYVOICE_MODEL_DIR}"
