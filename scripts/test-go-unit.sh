#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

export GOCACHE="${GOCACHE:-/tmp/agent-server-go-build}"
mkdir -p "${GOCACHE}"

echo "==> go unit/package tests"
go test ./...
