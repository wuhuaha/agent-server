#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/smoke-funasr.sh [options]

Options:
  --output-dir PATH         Artifact root. Default: artifacts/live-smoke/YYYYMMDD/desktop-full
  --wav PATH                Optional speech WAV. If omitted, a silence WAV is generated.
  --desktop-python CMD      Python executable for the desktop runner. Default: python3
  --conda-env NAME          Conda env for the FunASR worker. Default: xiaozhi-esp32-server
  --server-host HOST        agentd bind host. Default: 127.0.0.1
  --server-port PORT        agentd bind port. Default: 8080
  --worker-host HOST        FunASR worker bind host. Default: 127.0.0.1
  --worker-port PORT        FunASR worker bind port. Default: 8091
  --worker-model MODEL      FunASR model. Default: iic/SenseVoiceSmall
  --worker-device DEVICE    FunASR device. Default: cpu
  --worker-language LANG    FunASR language. Default: auto
  --voice-language LANG     agentd ASR language. Default: auto
  --scenario NAME           Runner scenario. Default: full
  --timeout-sec SEC         Runner timeout. Default: 60
  --llm-provider NAME       agentd LLM provider. Default: bootstrap
  --tts-provider NAME       agentd TTS provider. Default: none
  --help                    Show this message
EOF
}

log() {
  printf '[smoke-funasr] %s\n' "$*"
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

wait_for_http() {
  local url="$1"
  local timeout_sec="$2"
  local deadline=$((SECONDS + timeout_sec))
  while (( SECONDS < deadline )); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  return 1
}

write_silence_wav() {
  local output_path="$1"
  local python_cmd="$2"
  "$python_cmd" -c 'import pathlib, struct, wave, sys; path = pathlib.Path(sys.argv[1]); path.parent.mkdir(parents=True, exist_ok=True); frames = 16000; data = struct.pack("<%dh" % frames, *([0] * frames)); wf = wave.open(str(path), "wb"); wf.setnchannels(1); wf.setsampwidth(2); wf.setframerate(16000); wf.writeframes(data); wf.close()' "$output_path"
}

stop_group() {
  local pid="${1:-}"
  if [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1; then
    kill -- "-$pid" >/dev/null 2>&1 || true
    wait "$pid" 2>/dev/null || true
  fi
}

OUTPUT_DIR=""
WAV_INPUT=""
DESKTOP_PYTHON="python3"
CONDA_ENV="xiaozhi-esp32-server"
SERVER_HOST="127.0.0.1"
SERVER_PORT="8080"
WORKER_HOST="127.0.0.1"
WORKER_PORT="8091"
WORKER_MODEL="iic/SenseVoiceSmall"
WORKER_DEVICE="cpu"
WORKER_LANGUAGE="auto"
VOICE_LANGUAGE="auto"
SCENARIO="full"
TIMEOUT_SEC="60"
LLM_PROVIDER="bootstrap"
TTS_PROVIDER="none"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output-dir)
      OUTPUT_DIR="$2"
      shift 2
      ;;
    --wav)
      WAV_INPUT="$2"
      shift 2
      ;;
    --desktop-python)
      DESKTOP_PYTHON="$2"
      shift 2
      ;;
    --conda-env)
      CONDA_ENV="$2"
      shift 2
      ;;
    --server-host)
      SERVER_HOST="$2"
      shift 2
      ;;
    --server-port)
      SERVER_PORT="$2"
      shift 2
      ;;
    --worker-host)
      WORKER_HOST="$2"
      shift 2
      ;;
    --worker-port)
      WORKER_PORT="$2"
      shift 2
      ;;
    --worker-model)
      WORKER_MODEL="$2"
      shift 2
      ;;
    --worker-device)
      WORKER_DEVICE="$2"
      shift 2
      ;;
    --worker-language)
      WORKER_LANGUAGE="$2"
      shift 2
      ;;
    --voice-language)
      VOICE_LANGUAGE="$2"
      shift 2
      ;;
    --scenario)
      SCENARIO="$2"
      shift 2
      ;;
    --timeout-sec)
      TIMEOUT_SEC="$2"
      shift 2
      ;;
    --llm-provider)
      LLM_PROVIDER="$2"
      shift 2
      ;;
    --tts-provider)
      TTS_PROVIDER="$2"
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

require_cmd curl
require_cmd conda
require_cmd go
require_cmd setsid
require_cmd "$DESKTOP_PYTHON"

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
cd "$REPO_ROOT"

if [[ -z "$OUTPUT_DIR" ]]; then
  OUTPUT_DIR="${REPO_ROOT}/artifacts/live-smoke/$(date +%Y%m%d)/desktop-full"
fi

mkdir -p "$OUTPUT_DIR"

INPUT_WAV="${OUTPUT_DIR}/input.wav"
REPORT_PATH="${OUTPUT_DIR}/report.json"
WORKER_LOG="${OUTPUT_DIR}/worker.log"
WORKER_ERR="${OUTPUT_DIR}/worker.err.log"
SERVER_LOG="${OUTPUT_DIR}/agentd.log"
SERVER_ERR="${OUTPUT_DIR}/agentd.err.log"
WORKER_PID=""
SERVER_PID=""

cleanup() {
  stop_group "$SERVER_PID"
  stop_group "$WORKER_PID"
}

trap cleanup EXIT INT TERM

if [[ -n "$WAV_INPUT" ]]; then
  cp "$WAV_INPUT" "$INPUT_WAV"
  log "copied input wav to ${INPUT_WAV}"
else
  write_silence_wav "$INPUT_WAV" "$DESKTOP_PYTHON"
  log "no wav provided; generated silence input at ${INPUT_WAV}"
fi

log "starting FunASR worker on ${WORKER_HOST}:${WORKER_PORT}"
setsid env \
  PYTHONPATH="${REPO_ROOT}/workers/python/src" \
  AGENT_SERVER_FUNASR_DISABLE_UPDATE="true" \
  AGENT_SERVER_FUNASR_TRUST_REMOTE_CODE="false" \
  AGENT_SERVER_FUNASR_USE_ITN="true" \
  conda run --no-capture-output -n "$CONDA_ENV" \
  python -m agent_server_workers.funasr_service \
    --host "$WORKER_HOST" \
    --port "$WORKER_PORT" \
    --model "$WORKER_MODEL" \
    --device "$WORKER_DEVICE" \
    --language "$WORKER_LANGUAGE" \
    --disable-update \
    --use-itn >"$WORKER_LOG" 2>"$WORKER_ERR" &
WORKER_PID=$!

if ! wait_for_http "http://${WORKER_HOST}:${WORKER_PORT}/healthz" 30; then
  printf 'FunASR worker failed to become healthy; see %s and %s\n' "$WORKER_LOG" "$WORKER_ERR" >&2
  exit 1
fi

log "starting agentd on ${SERVER_HOST}:${SERVER_PORT}"
setsid env \
  AGENT_SERVER_ADDR="${SERVER_HOST}:${SERVER_PORT}" \
  AGENT_SERVER_VOICE_PROVIDER="funasr_http" \
  AGENT_SERVER_VOICE_ASR_URL="http://${WORKER_HOST}:${WORKER_PORT}/v1/asr/transcribe" \
  AGENT_SERVER_VOICE_ASR_LANGUAGE="$VOICE_LANGUAGE" \
  AGENT_SERVER_VOICE_EMIT_PLACEHOLDER_AUDIO="false" \
  AGENT_SERVER_TTS_PROVIDER="$TTS_PROVIDER" \
  AGENT_SERVER_AGENT_LLM_PROVIDER="$LLM_PROVIDER" \
  go run ./cmd/agentd >"$SERVER_LOG" 2>"$SERVER_ERR" &
SERVER_PID=$!

if ! wait_for_http "http://${SERVER_HOST}:${SERVER_PORT}/healthz" 30; then
  printf 'agentd failed to become healthy; see %s and %s\n' "$SERVER_LOG" "$SERVER_ERR" >&2
  exit 1
fi

log "running desktop runner scenario ${SCENARIO}"
PYTHONPATH="${REPO_ROOT}/clients/python-desktop-client/src" \
  "$DESKTOP_PYTHON" -m agent_server_desktop_client.runner \
    --scenario "$SCENARIO" \
    --http-base "http://${SERVER_HOST}:${SERVER_PORT}" \
    --wav "$INPUT_WAV" \
    --timeout-sec "$TIMEOUT_SEC" \
    --output "$REPORT_PATH" \
    --save-rx-dir "$OUTPUT_DIR"

log "completed; artifacts written to ${OUTPUT_DIR}"
