#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

echo "==> go test ./..."
go test ./...

echo "==> python desktop-client tests"
PYTHONPATH=clients/python-desktop-client/src python3 -m unittest discover -s clients/python-desktop-client/tests -v

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  echo "==> docker compose config"
  "${ROOT_DIR}/scripts/docker-config-check.sh"
else
  echo "==> skipping docker compose config (docker not available)"
fi
