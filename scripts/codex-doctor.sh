#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

status=0

print_section() {
  printf '\n==> %s\n' "$1"
}

required_tool() {
  local tool="$1"
  local help_text="$2"
  if command -v "${tool}" >/dev/null 2>&1; then
    printf '[ok] %s\n' "${tool}"
  else
    printf '[missing] %s: %s\n' "${tool}" "${help_text}" >&2
    status=1
  fi
}

optional_tool() {
  local tool="$1"
  local help_text="$2"
  if command -v "${tool}" >/dev/null 2>&1; then
    printf '[ok] %s\n' "${tool}"
  else
    printf '[warn] %s: %s\n' "${tool}" "${help_text}"
  fi
}

print_section "Core Tools"
required_tool go "required for agentd build and Go test execution"
required_tool python3 "required for desktop-client tests and local tooling"
optional_tool docker "needed for compose validation and container workflows"
optional_tool conda "needed for the local FunASR worker environment on this machine"

print_section "Versions"
if command -v go >/dev/null 2>&1; then
  go version
  printf 'GOPROXY=%s\n' "$(go env GOPROXY)"
  printf 'GOSUMDB=%s\n' "$(go env GOSUMDB)"
fi
if command -v python3 >/dev/null 2>&1; then
  python3 --version
fi
if command -v docker >/dev/null 2>&1; then
  docker compose version || printf '[warn] docker compose not available through the current docker install\n'
fi

print_section "Repository Checks"
if [[ -f deploy/docker/.env.docker.example ]]; then
  printf '[ok] deploy/docker/.env.docker.example\n'
else
  printf '[missing] deploy/docker/.env.docker.example\n' >&2
  status=1
fi

git status --short

print_section "Suggested Next Commands"
printf '%s\n' \
  "make test-go" \
  "make test-py" \
  "make docker-config" \
  "make verify-fast"

exit "${status}"
