#!/usr/bin/env bash
set -euo pipefail

PYTHON_BIN="${1:-python3}"
CONTEXT="${2:-python entrypoint}"

if ! command -v "${PYTHON_BIN}" >/dev/null 2>&1; then
  echo "${CONTEXT} requires ${PYTHON_BIN}, but it is not available in PATH" >&2
  exit 1
fi

if ! "${PYTHON_BIN}" -c 'import sys; raise SystemExit(0 if sys.version_info >= (3, 11) else 1)'; then
  echo "${CONTEXT} requires Python 3.11+, but ${PYTHON_BIN} reports $("${PYTHON_BIN}" --version 2>&1)" >&2
  exit 1
fi
