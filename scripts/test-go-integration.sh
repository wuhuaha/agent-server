#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

export GOCACHE="${GOCACHE:-/tmp/agent-server-go-build}"
mkdir -p "${GOCACHE}"

echo "==> go integration tests (tag=integration)"
echo "note: this tier opens local loopback listeners via httptest/websocket servers and needs local bind permission"
go test -tags=integration ./internal/gateway ./internal/voice
