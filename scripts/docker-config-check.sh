#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is not installed; cannot run compose config checks" >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "docker daemon is not reachable; cannot run compose config checks" >&2
  exit 1
fi

ENV_FILE="deploy/docker/.env.docker"
cleanup_env=0

if [[ ! -f "${ENV_FILE}" ]]; then
  cp deploy/docker/.env.docker.example "${ENV_FILE}"
  cleanup_env=1
fi

cleanup() {
  if [[ "${cleanup_env}" -eq 1 ]]; then
    rm -f "${ENV_FILE}"
  fi
}

trap cleanup EXIT

docker compose -f deploy/docker/compose.base.yml config >/dev/null
docker compose -f deploy/docker/compose.base.yml -f deploy/docker/compose.local-asr.yml config >/dev/null
docker compose -f deploy/docker/compose.base.yml -f deploy/docker/compose.local-tts-gpu.yml config >/dev/null

echo "docker compose config checks passed"
