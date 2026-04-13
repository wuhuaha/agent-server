#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

PYTHON_BIN="${PYTHON_BIN:-python3}"

bash "${ROOT_DIR}/scripts/require-python-3-11.sh" "${PYTHON_BIN}" "python worker tests"

echo "==> python worker tests (${PYTHON_BIN})"
PYTHONPATH=workers/python/src "${PYTHON_BIN}" -m unittest discover -s workers/python/tests -v
