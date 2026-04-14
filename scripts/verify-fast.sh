#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

PYTHON_BIN="${PYTHON_BIN:-python3}"

bash "${ROOT_DIR}/scripts/test-go-unit.sh"

bash "${ROOT_DIR}/scripts/test-python-desktop.sh"

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  echo "==> docker compose config"
  "${ROOT_DIR}/scripts/docker-config-check.sh"
else
  echo "==> skipping docker compose config (docker not available)"
fi
