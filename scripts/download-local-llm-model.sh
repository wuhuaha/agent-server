#!/usr/bin/env bash
set -euo pipefail

PYTHON_BIN="${AGENT_SERVER_LOCAL_LLM_PYTHON_BIN:-/home/ubuntu/kws-training/data/agent-server-runtime/cosyvoice-py310/bin/python}"
MODEL_ID="${AGENT_SERVER_LOCAL_LLM_MODEL_ID:-Qwen/Qwen3-4B-Instruct-2507}"
MODEL_DIR="${AGENT_SERVER_LOCAL_LLM_MODEL_DIR:-/home/ubuntu/kws-training/data/agent-server-cache/local-llm/Qwen3-4B-Instruct-2507}"
CACHE_DIR="${AGENT_SERVER_LOCAL_LLM_CACHE_DIR:-/home/ubuntu/kws-training/data/agent-server-cache/modelscope}"
DOWNLOAD_RETRIES="${AGENT_SERVER_LOCAL_LLM_DOWNLOAD_RETRIES:-3}"

mkdir -p "$MODEL_DIR" "$CACHE_DIR"
export MODEL_ID MODEL_DIR CACHE_DIR DOWNLOAD_RETRIES

exec "$PYTHON_BIN" - <<'PY'
import os
import re
import sys
import time
from pathlib import Path

from modelscope import snapshot_download

model_id = os.environ["MODEL_ID"]
model_dir = os.environ["MODEL_DIR"]
cache_dir = os.environ["CACHE_DIR"]
download_retries = max(int(os.environ.get("DOWNLOAD_RETRIES", "3")), 1)


def cleanup_corrupt_temp(error_text: str, target_dir: str) -> str:
    match = re.search(r"File ([^\\n]+?) integrity check failed", error_text)
    if match:
        path = Path(match.group(1).strip())
        if path.exists():
            path.unlink()
            return str(path)
    temp_dir = Path(target_dir) / "._____temp"
    if not temp_dir.exists():
        return ""
    removed = []
    for shard in temp_dir.glob("model-*.safetensors"):
        try:
            shard.unlink()
            removed.append(str(shard))
        except OSError:
            continue
    return ", ".join(removed)


for attempt in range(1, download_retries + 1):
    print(f"[local-llm] downloading {model_id} -> {model_dir} (attempt {attempt}/{download_retries})")
    try:
        path = snapshot_download(model_id=model_id, cache_dir=cache_dir, local_dir=model_dir)
        print(f"[local-llm] ready: {path}")
        raise SystemExit(0)
    except Exception as exc:  # pragma: no cover - exercised only in live provisioning
        message = str(exc)
        print(f"[local-llm] snapshot_download failed: {message}", file=sys.stderr)
        cleaned = ""
        if "integrity check failed" in message:
            cleaned = cleanup_corrupt_temp(message, model_dir)
            if cleaned:
                print(f"[local-llm] removed corrupt temp shard(s): {cleaned}", file=sys.stderr)
        if attempt >= download_retries:
            raise
        time.sleep(min(2 * attempt, 10))
PY
