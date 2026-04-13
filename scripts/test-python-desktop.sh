#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

PYTHON_BIN="${PYTHON_BIN:-python3}"

bash "${ROOT_DIR}/scripts/require-python-3-11.sh" "${PYTHON_BIN}" "desktop-client tests"

echo "==> python desktop-client tests (${PYTHON_BIN})"
PYTHONPATH=clients/python-desktop-client/src "${PYTHON_BIN}" -m unittest discover -s clients/python-desktop-client/tests -v
