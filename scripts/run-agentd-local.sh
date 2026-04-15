#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEFAULT_BINARY_PATH="$REPO_ROOT/bin/agentd"
USER_OVERRIDE_BINARY_PATH="$REPO_ROOT/.runtime/bin/agentd"
BINARY_PATH="${AGENT_SERVER_BINARY:-}"

if [[ "$BINARY_PATH" == "$DEFAULT_BINARY_PATH" && -x "$USER_OVERRIDE_BINARY_PATH" ]]; then
  BINARY_PATH="$USER_OVERRIDE_BINARY_PATH"
fi

if [[ -z "$BINARY_PATH" ]]; then
  if [[ -x "$USER_OVERRIDE_BINARY_PATH" ]]; then
    BINARY_PATH="$USER_OVERRIDE_BINARY_PATH"
  else
    BINARY_PATH="$DEFAULT_BINARY_PATH"
  fi
fi

if [[ ! -x "$BINARY_PATH" ]]; then
  echo "agentd binary not found at $BINARY_PATH; build it first" >&2
  exit 1
fi

export AGENT_SERVER_ADDR="${AGENT_SERVER_ADDR:-0.0.0.0:8080}"
export AGENT_SERVER_VOICE_PROVIDER="${AGENT_SERVER_VOICE_PROVIDER:-funasr_http}"
export AGENT_SERVER_VOICE_ASR_URL="${AGENT_SERVER_VOICE_ASR_URL:-http://127.0.0.1:8091/v1/asr/transcribe}"
export AGENT_SERVER_VOICE_ASR_LANGUAGE="${AGENT_SERVER_VOICE_ASR_LANGUAGE:-auto}"
export AGENT_SERVER_VOICE_EMIT_PLACEHOLDER_AUDIO="${AGENT_SERVER_VOICE_EMIT_PLACEHOLDER_AUDIO:-false}"
export AGENT_SERVER_VOICE_BARGE_IN_MIN_AUDIO_MS="${AGENT_SERVER_VOICE_BARGE_IN_MIN_AUDIO_MS:-120}"
export AGENT_SERVER_VOICE_BARGE_IN_HOLD_AUDIO_MS="${AGENT_SERVER_VOICE_BARGE_IN_HOLD_AUDIO_MS:-240}"
export AGENT_SERVER_VOICE_SPEECH_PLANNER_ENABLED="${AGENT_SERVER_VOICE_SPEECH_PLANNER_ENABLED:-true}"
export AGENT_SERVER_VOICE_SPEECH_PLANNER_MIN_CHUNK_RUNES="${AGENT_SERVER_VOICE_SPEECH_PLANNER_MIN_CHUNK_RUNES:-6}"
export AGENT_SERVER_VOICE_SPEECH_PLANNER_TARGET_CHUNK_RUNES="${AGENT_SERVER_VOICE_SPEECH_PLANNER_TARGET_CHUNK_RUNES:-24}"
export AGENT_SERVER_TTS_PROVIDER="${AGENT_SERVER_TTS_PROVIDER:-none}"
export AGENT_SERVER_AGENT_LLM_PROVIDER="${AGENT_SERVER_AGENT_LLM_PROVIDER:-bootstrap}"
export AGENT_SERVER_AGENT_DEEPSEEK_BASE_URL="${AGENT_SERVER_AGENT_DEEPSEEK_BASE_URL:-https://api.deepseek.com}"

derive_asr_ready_url() {
  local endpoint="$1"
  if [[ "$endpoint" == *"/v1/asr/transcribe" ]]; then
    printf '%s/healthz' "${endpoint%/v1/asr/transcribe}"
    return
  fi
  printf '%s' "$endpoint"
}

wait_for_asr_ready() {
  if [[ "${AGENT_SERVER_VOICE_PROVIDER}" != "funasr_http" ]]; then
    return 0
  fi
  if [[ -z "${AGENT_SERVER_VOICE_ASR_URL}" ]]; then
    return 0
  fi

  local timeout_sec="${AGENT_SERVER_VOICE_ASR_READY_TIMEOUT_SEC:-180}"
  if (( timeout_sec <= 0 )); then
    return 0
  fi

  local poll_sec="${AGENT_SERVER_VOICE_ASR_READY_POLL_INTERVAL_SEC:-2}"
  local health_url="${AGENT_SERVER_VOICE_ASR_READY_URL:-$(derive_asr_ready_url "${AGENT_SERVER_VOICE_ASR_URL}")}"
  local deadline=$((SECONDS + timeout_sec))
  local payload=""

  echo "[agentd] waiting for FunASR worker readiness: ${health_url}"
  while (( SECONDS < deadline )); do
    payload="$(curl -fsS "${health_url}" 2>/dev/null || true)"
    if [[ -n "${payload}" ]] && grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"' <<<"${payload}"; then
      echo "[agentd] FunASR worker ready"
      return 0
    fi
    sleep "${poll_sec}"
  done

  echo "[agentd] FunASR worker did not become ready within ${timeout_sec}s: ${health_url}" >&2
  if [[ -n "${payload}" ]]; then
    echo "[agentd] last health payload: ${payload}" >&2
  fi
  return 1
}

derive_llm_ready_url() {
  local base_url="$1"
  if [[ "$base_url" == */v1 ]]; then
    printf '%s/healthz' "${base_url%/v1}"
    return
  fi
  printf '%s/healthz' "$base_url"
}

llm_base_url_is_local() {
  local base_url="$1"
  [[ "$base_url" == http://127.0.0.1* || "$base_url" == http://localhost* ]]
}

wait_for_llm_ready() {
  if [[ "${AGENT_SERVER_AGENT_LLM_PROVIDER}" != "deepseek_chat" && "${AGENT_SERVER_AGENT_LLM_PROVIDER}" != "deepseek" ]]; then
    return 0
  fi
  if ! llm_base_url_is_local "${AGENT_SERVER_AGENT_DEEPSEEK_BASE_URL}"; then
    return 0
  fi

  local timeout_sec="${AGENT_SERVER_AGENT_LLM_READY_TIMEOUT_SEC:-180}"
  if (( timeout_sec <= 0 )); then
    return 0
  fi

  local poll_sec="${AGENT_SERVER_AGENT_LLM_READY_POLL_INTERVAL_SEC:-2}"
  local health_url="${AGENT_SERVER_AGENT_LLM_READY_URL:-$(derive_llm_ready_url "${AGENT_SERVER_AGENT_DEEPSEEK_BASE_URL}")}"
  local deadline=$((SECONDS + timeout_sec))
  local payload=""

  echo "[agentd] waiting for local LLM worker readiness: ${health_url}"
  while (( SECONDS < deadline )); do
    payload="$(curl -fsS "${health_url}" 2>/dev/null || true)"
    if [[ -n "${payload}" ]] && grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"' <<<"${payload}"; then
      echo "[agentd] local LLM worker ready"
      return 0
    fi
    sleep "${poll_sec}"
  done

  echo "[agentd] local LLM worker did not become ready within ${timeout_sec}s: ${health_url}" >&2
  if [[ -n "${payload}" ]]; then
    echo "[agentd] last health payload: ${payload}" >&2
  fi
  return 1
}

wait_for_asr_ready
wait_for_llm_ready

cd "$REPO_ROOT"
exec "$BINARY_PATH"
