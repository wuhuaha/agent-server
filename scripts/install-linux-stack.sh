#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/install-linux-stack.sh [options]

Options:
  --desktop-python PATH          Python executable used for desktop client install. Default: python3
  --conda-env NAME               Conda env for the FunASR worker. Default: xiaozhi-esp32-server
  --worker-python-version VER    Python version when creating the worker env. Default: 3.11
  --with-stream-vad              Install optional stream-vad packages: onnxruntime + silero-vad
  --skip-go                      Skip go mod download
  --skip-desktop-client          Skip desktop client editable install
  --skip-worker                  Skip worker env creation/update
  --skip-worker-torch            Do not auto-install torch/torchaudio into the worker env
  --force-worker-torch           Force reinstall torch/torchaudio even when import checks pass
  --torch-index-url URL          PyTorch index URL used when torch install is needed.
                                 Default: https://download.pytorch.org/whl/cu128
  --torch-version VER            Optional torch version pin, for example 2.7.1
  --torchaudio-version VER       Optional torchaudio version pin, for example 2.7.1
  --help                         Show this message

What this script installs:
  1. Go module dependencies for agentd
  2. Python desktop client into the selected desktop Python environment
  3. FunASR worker package into the selected conda env
  4. Worker runtime extras: funasr==1.3.1, modelscope==1.24.1
  5. Optional worker stream-vad extras: onnxruntime, silero-vad
EOF
}

log() {
  printf '[install-linux-stack] %s\n' "$*"
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

worker_torch_healthy() {
  TORCH_INDEX_URL="$TORCH_INDEX_URL" conda run -n "$CONDA_ENV" python - <<'PY'
import os
import sys

try:
    import torch
    import torchaudio
except Exception as exc:  # noqa: BLE001
    print(f"worker torch import failed: {exc}", file=sys.stderr)
    raise SystemExit(1)

index_url = os.environ.get("TORCH_INDEX_URL", "")
need_cuda = "/whl/cu" in index_url
cuda_tag = getattr(getattr(torch, "version", None), "cuda", None)
if need_cuda and not cuda_tag:
    print("worker torch is CPU-only while a CUDA wheel index was requested", file=sys.stderr)
    raise SystemExit(2)

if cuda_tag and torch.cuda.is_available():
    try:
        torch.zeros(1, device="cuda")
    except Exception as exc:  # noqa: BLE001
        print(f"worker torch CUDA runtime check failed: {exc}", file=sys.stderr)
        raise SystemExit(3)

print("worker torch ok", torch.__version__, torchaudio.__version__, cuda_tag or "cpu")
PY
}

DESKTOP_PYTHON="python3"
CONDA_ENV="xiaozhi-esp32-server"
WORKER_PYTHON_VERSION="3.11"
WITH_STREAM_VAD=0
SKIP_GO=0
SKIP_DESKTOP_CLIENT=0
SKIP_WORKER=0
SKIP_WORKER_TORCH=0
FORCE_WORKER_TORCH=0
TORCH_INDEX_URL="https://download.pytorch.org/whl/cu128"
TORCH_VERSION=""
TORCHAUDIO_VERSION=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --desktop-python)
      DESKTOP_PYTHON="$2"
      shift 2
      ;;
    --conda-env)
      CONDA_ENV="$2"
      shift 2
      ;;
    --worker-python-version)
      WORKER_PYTHON_VERSION="$2"
      shift 2
      ;;
    --with-stream-vad)
      WITH_STREAM_VAD=1
      shift
      ;;
    --skip-go)
      SKIP_GO=1
      shift
      ;;
    --skip-desktop-client)
      SKIP_DESKTOP_CLIENT=1
      shift
      ;;
    --skip-worker)
      SKIP_WORKER=1
      shift
      ;;
    --skip-worker-torch)
      SKIP_WORKER_TORCH=1
      shift
      ;;
    --force-worker-torch)
      FORCE_WORKER_TORCH=1
      shift
      ;;
    --torch-index-url)
      TORCH_INDEX_URL="$2"
      shift 2
      ;;
    --torch-version)
      TORCH_VERSION="$2"
      shift 2
      ;;
    --torchaudio-version)
      TORCHAUDIO_VERSION="$2"
      shift 2
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

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
cd "$REPO_ROOT"

if [[ "$SKIP_GO" -eq 0 ]]; then
  require_cmd go
  log "downloading Go module dependencies"
  go mod download
fi

if [[ "$SKIP_DESKTOP_CLIENT" -eq 0 ]]; then
  require_cmd "$DESKTOP_PYTHON"
  log "installing desktop client into ${DESKTOP_PYTHON}"
  "$DESKTOP_PYTHON" -m pip install --upgrade pip "setuptools<82" wheel "hatchling>=1.25.0" "editables>=0.5"
  "$DESKTOP_PYTHON" -m pip install --no-build-isolation -e clients/python-desktop-client
fi

if [[ "$SKIP_WORKER" -eq 0 ]]; then
  require_cmd conda
  log "preparing worker conda env ${CONDA_ENV}"
  if ! conda env list | awk 'NF && $1 !~ /^#/ {print $1}' | grep -Fxq "$CONDA_ENV"; then
    log "creating conda env ${CONDA_ENV} with python ${WORKER_PYTHON_VERSION}"
    conda create -y -n "$CONDA_ENV" "python=${WORKER_PYTHON_VERSION}"
  fi

  log "upgrading pip bootstrap tooling inside ${CONDA_ENV}"
  conda run -n "$CONDA_ENV" python -m pip install --upgrade pip "setuptools<82" wheel "hatchling>=1.25.0" "editables>=0.5"

  if [[ "$SKIP_WORKER_TORCH" -eq 0 ]]; then
    TORCH_SPEC="torch"
    TORCHAUDIO_SPEC="torchaudio"
    if [[ -n "$TORCH_VERSION" ]]; then
      TORCH_SPEC="torch==${TORCH_VERSION}"
    fi
    if [[ -n "$TORCHAUDIO_VERSION" ]]; then
      TORCHAUDIO_SPEC="torchaudio==${TORCHAUDIO_VERSION}"
    fi

    if [[ "$FORCE_WORKER_TORCH" -eq 1 ]] || ! worker_torch_healthy; then
      log "installing torch and torchaudio into ${CONDA_ENV} from ${TORCH_INDEX_URL}"
      conda run -n "$CONDA_ENV" python -m pip install --upgrade --force-reinstall --index-url "$TORCH_INDEX_URL" "$TORCH_SPEC" "$TORCHAUDIO_SPEC"
    else
      log "torch and torchaudio already available and importable in ${CONDA_ENV}"
    fi
  fi

  log "installing FunASR worker package with runtime extras into ${CONDA_ENV}"
  conda run -n "$CONDA_ENV" python -m pip install --no-build-isolation -e "./workers/python[runtime]"

  if [[ "$WITH_STREAM_VAD" -eq 1 ]]; then
    log "installing optional stream-vad extras into ${CONDA_ENV}"
    conda run -n "$CONDA_ENV" python -m pip install --no-build-isolation -e "./workers/python[runtime,stream-vad]"
  fi

  log "verifying worker imports inside ${CONDA_ENV}"
  conda run -n "$CONDA_ENV" python -c "import importlib.util as u; print('funasr', bool(u.find_spec('funasr'))); print('modelscope', bool(u.find_spec('modelscope'))); print('torch', bool(u.find_spec('torch'))); print('torchaudio', bool(u.find_spec('torchaudio'))); print('onnxruntime', bool(u.find_spec('onnxruntime'))); print('silero_vad', bool(u.find_spec('silero_vad')))"
fi

log "completed"
