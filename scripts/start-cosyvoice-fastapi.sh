#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/start-cosyvoice-fastapi.sh [options]

Options:
  --repo-dir PATH     Local CosyVoice repository path. Default: $COSYVOICE_REPO_DIR
  --python-bin PATH   Python binary that already has the runtime deps installed.
  --conda-env NAME    Conda environment name. Default: $COSYVOICE_CONDA_ENV or cosyvoice
  --port PORT         FastAPI listen port. Default: $COSYVOICE_PORT or 50000
  --model-dir PATH    Model dir argument passed to server.py. Default: $COSYVOICE_MODEL_DIR or iic/CosyVoice-300M-SFT
  --download-mode X   Model staging mode: runtime_minimal|full_repo|off. Default: $COSYVOICE_MODEL_DOWNLOAD_MODE
  --model-cache-dir P Local cache dir used for runtime_minimal staging.
  --check             Only validate the runtime and exit.
  --help              Show this message
EOF
}

COSYVOICE_REPO_DIR="${COSYVOICE_REPO_DIR:-}"
COSYVOICE_PYTHON_BIN="${COSYVOICE_PYTHON_BIN:-}"
COSYVOICE_CONDA_ENV="${COSYVOICE_CONDA_ENV:-cosyvoice}"
COSYVOICE_PORT="${COSYVOICE_PORT:-50000}"
COSYVOICE_MODEL_DIR="${COSYVOICE_MODEL_DIR:-iic/CosyVoice-300M-SFT}"
COSYVOICE_MODEL_DOWNLOAD_MODE="${COSYVOICE_MODEL_DOWNLOAD_MODE:-runtime_minimal}"
COSYVOICE_MODEL_CACHE_DIR="${COSYVOICE_MODEL_CACHE_DIR:-${MODELSCOPE_CACHE:-$HOME/.cache/modelscope}/models}"
COSYVOICE_MODEL_DOWNLOAD_MAX_WORKERS="${COSYVOICE_MODEL_DOWNLOAD_MAX_WORKERS:-4}"
COSYVOICE_REQUIRE_CUDA="${COSYVOICE_REQUIRE_CUDA:-true}"
CHECK_ONLY=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-dir)
      COSYVOICE_REPO_DIR="$2"
      shift 2
      ;;
    --python-bin)
      COSYVOICE_PYTHON_BIN="$2"
      shift 2
      ;;
    --conda-env)
      COSYVOICE_CONDA_ENV="$2"
      shift 2
      ;;
    --port)
      COSYVOICE_PORT="$2"
      shift 2
      ;;
    --model-dir)
      COSYVOICE_MODEL_DIR="$2"
      shift 2
      ;;
    --download-mode)
      COSYVOICE_MODEL_DOWNLOAD_MODE="$2"
      shift 2
      ;;
    --model-cache-dir)
      COSYVOICE_MODEL_CACHE_DIR="$2"
      shift 2
      ;;
    --check)
      CHECK_ONLY=true
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      printf 'unknown option: %s\n' "$1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [[ -z "${COSYVOICE_REPO_DIR}" ]]; then
  printf 'COSYVOICE_REPO_DIR or --repo-dir is required\n' >&2
  exit 1
fi

case "${COSYVOICE_MODEL_DOWNLOAD_MODE}" in
  runtime_minimal|full_repo|off)
    ;;
  *)
    printf 'unsupported COSYVOICE_MODEL_DOWNLOAD_MODE: %s\n' "${COSYVOICE_MODEL_DOWNLOAD_MODE}" >&2
    exit 1
    ;;
esac

SERVER_DIR="${COSYVOICE_REPO_DIR%/}/runtime/python/fastapi"
if [[ ! -f "${SERVER_DIR}/server.py" ]]; then
  printf 'expected CosyVoice FastAPI server at %s/server.py\n' "${SERVER_DIR}" >&2
  exit 1
fi

PYTHONPATH_PREFIX="${COSYVOICE_REPO_DIR%/}"
if [[ -n "${PYTHONPATH:-}" ]]; then
  PYTHONPATH_PREFIX="${PYTHONPATH_PREFIX}:${PYTHONPATH}"
fi

if [[ -n "${COSYVOICE_PYTHON_BIN}" ]]; then
  if [[ ! -x "${COSYVOICE_PYTHON_BIN}" ]]; then
    printf 'COSYVOICE_PYTHON_BIN is not executable: %s\n' "${COSYVOICE_PYTHON_BIN}" >&2
    exit 1
  fi
  RUNNER=("${COSYVOICE_PYTHON_BIN}")
  COSYVOICE_PYTHON_ENV_ROOT="$(cd "$(dirname "${COSYVOICE_PYTHON_BIN}")/.." && pwd)"
  COSYVOICE_CUSPARSELT_LIB_DIR="$(compgen -G "${COSYVOICE_PYTHON_ENV_ROOT}/lib/python*/site-packages/cusparselt/lib" | head -n 1 || true)"
else
  if ! command -v conda >/dev/null 2>&1; then
    printf 'either COSYVOICE_PYTHON_BIN or conda is required to start the CosyVoice FastAPI server\n' >&2
    exit 1
  fi
  RUNNER=(conda run --no-capture-output -n "${COSYVOICE_CONDA_ENV}" python)
  COSYVOICE_CUSPARSELT_LIB_DIR=""
fi

COSYVOICE_EFFECTIVE_LD_LIBRARY_PATH="${LD_LIBRARY_PATH:-}"
if [[ -n "${COSYVOICE_CUSPARSELT_LIB_DIR}" ]]; then
  if [[ -n "${COSYVOICE_EFFECTIVE_LD_LIBRARY_PATH}" ]]; then
    COSYVOICE_EFFECTIVE_LD_LIBRARY_PATH="${COSYVOICE_CUSPARSELT_LIB_DIR}:${COSYVOICE_EFFECTIVE_LD_LIBRARY_PATH}"
  else
    COSYVOICE_EFFECTIVE_LD_LIBRARY_PATH="${COSYVOICE_CUSPARSELT_LIB_DIR}"
  fi
fi

run_python() {
  env PYTHONPATH="${PYTHONPATH_PREFIX}" LD_LIBRARY_PATH="${COSYVOICE_EFFECTIVE_LD_LIBRARY_PATH}" "${RUNNER[@]}" "$@"
}

looks_like_model_repo_id() {
  local value="$1"
  [[ "${value}" == */* && "${value}" != /* && "${value}" != ./* && "${value}" != ../* ]]
}

supports_runtime_minimal_download() {
  case "$1" in
    iic/CosyVoice-300M-SFT|iic/CosyVoice-300M-Instruct)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

resolve_model_dir() {
  local requested_model_dir="$1"
  local staged_model_dir
  local resolved_model_dir="$requested_model_dir"

  if [[ -d "${requested_model_dir}" ]]; then
    printf '%s\n' "${requested_model_dir}"
    return 0
  fi

  if [[ "${COSYVOICE_MODEL_DOWNLOAD_MODE}" == "runtime_minimal" ]] \
    && looks_like_model_repo_id "${requested_model_dir}" \
    && supports_runtime_minimal_download "${requested_model_dir}"; then
    staged_model_dir="${COSYVOICE_MODEL_CACHE_DIR%/}/${requested_model_dir}-runtime"
    mkdir -p "${staged_model_dir}"
    printf '[cosyvoice-fastapi] staging runtime-minimal model files into %s\n' "${staged_model_dir}" >&2
    resolved_model_dir="$(
      COSYVOICE_REQUESTED_MODEL_ID="${requested_model_dir}" \
      COSYVOICE_STAGED_MODEL_DIR="${staged_model_dir}" \
      COSYVOICE_MODEL_DOWNLOAD_MAX_WORKERS="${COSYVOICE_MODEL_DOWNLOAD_MAX_WORKERS}" \
      run_python - <<'PY' | tail -n 1
from modelscope import snapshot_download
import os

requested_model_id = os.environ["COSYVOICE_REQUESTED_MODEL_ID"]
staged_model_dir = os.environ["COSYVOICE_STAGED_MODEL_DIR"]
max_workers = int(os.environ["COSYVOICE_MODEL_DOWNLOAD_MAX_WORKERS"])
allow_patterns = [
    "campplus.onnx",
    "configuration.json",
    "cosyvoice.yaml",
    "flow.pt",
    "hift.pt",
    "llm.pt",
    "speech_tokenizer_v1.onnx",
    "spk2info.pt",
]
resolved = snapshot_download(
    requested_model_id,
    local_dir=staged_model_dir,
    allow_patterns=allow_patterns,
    allow_file_pattern=allow_patterns,
    max_workers=max_workers,
)
print(resolved)
PY
    )"
  elif [[ "${COSYVOICE_MODEL_DOWNLOAD_MODE}" == "runtime_minimal" ]] \
    && looks_like_model_repo_id "${requested_model_dir}"; then
    printf '[cosyvoice-fastapi] runtime_minimal is not defined for %s, falling back to upstream repo download behavior\n' "${requested_model_dir}" >&2
  fi

  printf '%s\n' "${resolved_model_dir}"
}

run_python - <<'PY'
import importlib.util as importlib_util
import os

required_modules = [
    "torch",
    "torchaudio",
    "fastapi",
    "uvicorn",
    "modelscope",
    "hyperpyyaml",
    "multipart",
    "onnxruntime",
    "whisper",
]
missing = [name for name in required_modules if importlib_util.find_spec(name) is None]
if missing:
    raise SystemExit("missing python modules: " + ", ".join(missing))

import torch

require_cuda = os.environ.get("COSYVOICE_REQUIRE_CUDA", "true").lower() not in {"0", "false", "no"}
cuda_available = torch.cuda.is_available()
print(f"[cosyvoice-fastapi] torch={torch.__version__} cuda_available={cuda_available}")
if require_cuda and not cuda_available:
    raise SystemExit("cuda is required for CosyVoice realtime TTS on this host")
if cuda_available:
    print(f"[cosyvoice-fastapi] gpu={torch.cuda.get_device_name(0)}")
PY

if [[ "${CHECK_ONLY}" == "true" ]]; then
  printf '[cosyvoice-fastapi] runtime check passed\n'
  exit 0
fi

COSYVOICE_RESOLVED_MODEL_DIR="$(resolve_model_dir "${COSYVOICE_MODEL_DIR}")"

cd "${SERVER_DIR}"
exec env \
  PYTHONPATH="${PYTHONPATH_PREFIX}" \
  LD_LIBRARY_PATH="${COSYVOICE_EFFECTIVE_LD_LIBRARY_PATH}" \
  COSYVOICE_PORT="${COSYVOICE_PORT}" \
  COSYVOICE_MODEL_DIR="${COSYVOICE_RESOLVED_MODEL_DIR}" \
  "${RUNNER[@]}" - <<'PY'
import os
import runpy
import sys

sys.argv = [
    "server.py",
    "--port",
    os.environ["COSYVOICE_PORT"],
    "--model_dir",
    os.environ["COSYVOICE_MODEL_DIR"],
]
runpy.run_path("server.py", run_name="__main__")
PY
