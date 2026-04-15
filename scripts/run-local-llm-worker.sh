#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PYTHON_BIN="${AGENT_SERVER_LOCAL_LLM_PYTHON_BIN:-/home/ubuntu/kws-training/data/agent-server-runtime/cosyvoice-py310/bin/python}"
export PYTHONPATH="$REPO_ROOT/workers/python/src${PYTHONPATH:+:$PYTHONPATH}"

exec "$PYTHON_BIN" -m agent_server_workers.local_llm_service "$@"
