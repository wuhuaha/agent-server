#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOCAL_LLM_ENV_FILE="${LOCAL_LLM_ENV_FILE:-/etc/agent-server/local-llm.env}"
AGENTD_ENV_FILE="${AGENTD_ENV_FILE:-/etc/agent-server/agentd.env}"
LOCAL_LLM_SERVICE="${LOCAL_LLM_SERVICE:-agent-server-local-llm.service}"
AGENTD_SERVICE="${AGENTD_SERVICE:-agent-server-agentd.service}"
LOCAL_LLM_BASE_URL="${LOCAL_LLM_BASE_URL:-http://127.0.0.1:8012/v1}"
LOCAL_LLM_API_KEY="${LOCAL_LLM_API_KEY:-local-llm}"
LOCAL_LLM_TIMEOUT_MS="${LOCAL_LLM_TIMEOUT_MS:-60000}"
TEXT_SMOKE_PROMPT="${TEXT_SMOKE_PROMPT:-明天周几}"

wait_for_health() {
  local url="$1"
  local label="$2"
  local timeout_sec="${3:-300}"
  local deadline=$((SECONDS + timeout_sec))
  local payload=""
  echo "[provision-local-qwen] waiting for ${label}: ${url}"
  while (( SECONDS < deadline )); do
    payload="$(curl -fsS "$url" 2>/dev/null || true)"
    if [[ -n "$payload" ]] && grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"' <<<"$payload"; then
      echo "[provision-local-qwen] ${label} ready"
      return 0
    fi
    sleep 2
  done
  echo "[provision-local-qwen] ${label} did not become ready: ${url}" >&2
  if [[ -n "$payload" ]]; then
    echo "[provision-local-qwen] last payload: $payload" >&2
  fi
  return 1
}

require_env_value() {
  local key="$1"
  local value
  value="$(sudo awk -F= -v target="$key" '$1 == target {print substr($0, index($0, "=") + 1)}' "$LOCAL_LLM_ENV_FILE" | tail -n 1)"
  if [[ -z "$value" ]]; then
    echo "[provision-local-qwen] missing $key in $LOCAL_LLM_ENV_FILE" >&2
    exit 1
  fi
  printf '%s' "$value"
}

cd "$REPO_ROOT"

sudo install -d /etc/agent-server
if [[ ! -f "$LOCAL_LLM_ENV_FILE" ]]; then
  sudo install -m 0644 "$REPO_ROOT/deploy/systemd/agent-server-local-llm.env.example" "$LOCAL_LLM_ENV_FILE"
fi
sudo install -m 0644 "$REPO_ROOT/deploy/systemd/agent-server-local-llm.service" /etc/systemd/system/agent-server-local-llm.service
sudo systemctl daemon-reload

MODEL_ID="$(require_env_value AGENT_SERVER_LOCAL_LLM_MODEL_ID)"
MODEL_DIR="$(require_env_value AGENT_SERVER_LOCAL_LLM_MODEL_DIR)"
PYTHON_BIN="$(require_env_value AGENT_SERVER_LOCAL_LLM_PYTHON_BIN)"

AGENT_SERVER_LOCAL_LLM_MODEL_ID="$MODEL_ID" \
AGENT_SERVER_LOCAL_LLM_MODEL_DIR="$MODEL_DIR" \
AGENT_SERVER_LOCAL_LLM_PYTHON_BIN="$PYTHON_BIN" \
./scripts/download-local-llm-model.sh

sudo systemctl enable --now "$LOCAL_LLM_SERVICE"
wait_for_health "${LOCAL_LLM_BASE_URL%/v1}/healthz" "local LLM worker" 900

sudo python3 - <<'PY'
from pathlib import Path

env_path = Path('/etc/agent-server/agentd.env')
updates = {
    'AGENT_SERVER_AGENT_LLM_PROVIDER': 'deepseek_chat',
    'AGENT_SERVER_AGENT_DEEPSEEK_BASE_URL': 'http://127.0.0.1:8012/v1',
    'AGENT_SERVER_AGENT_DEEPSEEK_API_KEY': 'local-llm',
    'AGENT_SERVER_AGENT_DEEPSEEK_MODEL': 'Qwen/Qwen3-4B-Instruct-2507',
    'AGENT_SERVER_AGENT_LLM_TIMEOUT_MS': '60000',
}
lines = []
if env_path.exists():
    lines = env_path.read_text(encoding='utf-8').splitlines()
seen = set()
out = []
for line in lines:
    if not line or line.lstrip().startswith('#') or '=' not in line:
        out.append(line)
        continue
    key = line.split('=', 1)[0]
    if key in updates:
        out.append(f'{key}={updates[key]}')
        seen.add(key)
    else:
        out.append(line)
for key, value in updates.items():
    if key not in seen:
        out.append(f'{key}={value}')
env_path.write_text('\n'.join(out) + '\n', encoding='utf-8')
PY

sudo systemctl restart "$AGENTD_SERVICE"
wait_for_health "http://127.0.0.1:8080/healthz" "agentd" 300

mkdir -p "$REPO_ROOT/artifacts/live-smoke/$(date +%Y%m%d)/local-qwen-text"
REPORT_PATH="$REPO_ROOT/artifacts/live-smoke/$(date +%Y%m%d)/local-qwen-text/report.json"
PYTHONPATH="$REPO_ROOT/clients/python-desktop-client/src" \
python3 -m agent_server_desktop_client.runner \
  --http-base http://127.0.0.1:8080 \
  --scenario text \
  --text "$TEXT_SMOKE_PROMPT" \
  --timeout-sec 90 \
  --output "$REPORT_PATH"

python3 - <<'PY' "$REPORT_PATH"
import json
import sys
from pathlib import Path

report_path = Path(sys.argv[1])
payload = json.loads(report_path.read_text(encoding="utf-8"))
texts = []
for scenario in payload.get("scenarios", []):
    texts.extend(str(item) for item in (scenario.get("response_texts") or []))
visible = "\n".join(part for part in texts if part.strip()).strip()
if not visible:
    raise SystemExit("[provision-local-qwen] no response_texts found in smoke report")
if "agent-server received text input:" in visible:
    raise SystemExit("[provision-local-qwen] bootstrap echo still present in smoke response")
print(f"[provision-local-qwen] smoke response: {visible[:400]}")
PY

echo "[provision-local-qwen] completed"
