#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/start-cosyvoice-fastapi.sh [options]

Options:
  --repo-dir PATH     Local CosyVoice repository path. Default: $COSYVOICE_REPO_DIR
  --conda-env NAME    Conda environment name. Default: $COSYVOICE_CONDA_ENV or cosyvoice
  --port PORT         FastAPI listen port. Default: $COSYVOICE_PORT or 50000
  --model-dir PATH    Model dir argument passed to server.py. Default: $COSYVOICE_MODEL_DIR or iic/CosyVoice-300M-SFT
  --help              Show this message
EOF
}

COSYVOICE_REPO_DIR="${COSYVOICE_REPO_DIR:-}"
COSYVOICE_CONDA_ENV="${COSYVOICE_CONDA_ENV:-cosyvoice}"
COSYVOICE_PORT="${COSYVOICE_PORT:-50000}"
COSYVOICE_MODEL_DIR="${COSYVOICE_MODEL_DIR:-iic/CosyVoice-300M-SFT}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-dir)
      COSYVOICE_REPO_DIR="$2"
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

if ! command -v conda >/dev/null 2>&1; then
  printf 'conda is required to start the CosyVoice FastAPI server\n' >&2
  exit 1
fi

SERVER_DIR="${COSYVOICE_REPO_DIR%/}/runtime/python/fastapi"
if [[ ! -f "${SERVER_DIR}/server.py" ]]; then
  printf 'expected CosyVoice FastAPI server at %s/server.py\n' "${SERVER_DIR}" >&2
  exit 1
fi

PYTHONPATH_PREFIX="${COSYVOICE_REPO_DIR%/}"
if [[ -n "${PYTHONPATH:-}" ]]; then
  PYTHONPATH_PREFIX="${PYTHONPATH_PREFIX}:${PYTHONPATH}"
fi

cd "${SERVER_DIR}"
exec conda run --no-capture-output -n "${COSYVOICE_CONDA_ENV}" \
  env PYTHONPATH="${PYTHONPATH_PREFIX}" \
  python server.py --port "${COSYVOICE_PORT}" --model_dir "${COSYVOICE_MODEL_DIR}"
