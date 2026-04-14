#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/web-h5-manual-capture.sh [options]

Options:
  --output-dir PATH         Artifact root. Default: artifacts/live-smoke/YYYYMMDD/web-h5-manual
  --http-base URL           agentd HTTP base. Default: http://127.0.0.1:8080
  --standalone-base URL     Standalone web client base. Default: http://127.0.0.1:18081
  --mode MODE               built-in | standalone | both. Default: built-in
  --skip-fetch              Only scaffold the artifact bundle; do not fetch server or page snapshots
  --help                    Show this message
EOF
}

log() {
  printf '[web-h5-manual-capture] %s\n' "$*"
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

write_capture_manifest() {
  local output_dir="$1"
  local generated_at="$2"
  local http_base="$3"
  local standalone_base="$4"
  local mode="$5"
  local skip_fetch="$6"
  cat >"${output_dir}/capture.json" <<EOF
{
  "generated_at": "${generated_at}",
  "http_base": "${http_base}",
  "standalone_base": "${standalone_base}",
  "mode": "${mode}",
  "skip_fetch": ${skip_fetch}
}
EOF
}

write_manual_checklist() {
  local output_dir="$1"
  cat >"${output_dir}/manual-checklist.md" <<'EOF'
# Web/H5 Manual Validation Checklist

## Environment

- Browser:
- Browser version:
- Device or OS:
- Validation mode:
  - built-in `/debug/realtime-h5/`
  - standalone `clients/web-realtime-client`
- Reviewer:

## Basic Bring-Up

- [ ] `server/realtime.json` was captured
- [ ] settings page opened successfully
- [ ] debug page opened successfully
- [ ] websocket connected successfully
- [ ] `session.start` completed

## Turn Checks

- [ ] text turn succeeded
- [ ] microphone turn succeeded
- [ ] `interrupt` action behaved as expected
- [ ] TTS playback behaved as expected
- [ ] latest TTS WAV export was saved if audio was present

## Suggested Screenshot Names

- `screenshots/01-settings.png`
- `screenshots/02-connected.png`
- `screenshots/03-text-turn.png`
- `screenshots/04-mic-turn.png`
- `screenshots/05-tts.png`

## Suggested Manual Attachments

- Browser console export:
  - `logs/browser-console.txt`
- Exported WAV:
  - `exports/latest-tts.wav`
- Additional notes:
  - `logs/manual-notes.txt`

## Observed Result

- Summary:
- Issues:
- Follow-up:
EOF
}

fetch_to_file() {
  local url="$1"
  local output_path="$2"
  curl -fsS "$url" -o "$output_path"
}

OUTPUT_DIR=""
HTTP_BASE="http://127.0.0.1:8080"
STANDALONE_BASE="http://127.0.0.1:18081"
MODE="built-in"
SKIP_FETCH=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output-dir)
      OUTPUT_DIR="$2"
      shift 2
      ;;
    --http-base)
      HTTP_BASE="$2"
      shift 2
      ;;
    --standalone-base)
      STANDALONE_BASE="$2"
      shift 2
      ;;
    --mode)
      MODE="$2"
      shift 2
      ;;
    --skip-fetch)
      SKIP_FETCH=1
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      printf 'unknown option: %s\n' "$1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

case "$MODE" in
  built-in|standalone|both)
    ;;
  *)
    printf 'invalid mode: %s\n' "$MODE" >&2
    exit 1
    ;;
esac

require_cmd curl

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

if [[ -z "$OUTPUT_DIR" ]]; then
  OUTPUT_DIR="${REPO_ROOT}/artifacts/live-smoke/$(date +%Y%m%d)/web-h5-manual"
fi

mkdir -p \
  "${OUTPUT_DIR}/server" \
  "${OUTPUT_DIR}/pages/built-in" \
  "${OUTPUT_DIR}/pages/standalone" \
  "${OUTPUT_DIR}/screenshots" \
  "${OUTPUT_DIR}/exports" \
  "${OUTPUT_DIR}/logs"

GENERATED_AT="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
write_capture_manifest "$OUTPUT_DIR" "$GENERATED_AT" "$HTTP_BASE" "$STANDALONE_BASE" "$MODE" "$SKIP_FETCH"
write_manual_checklist "$OUTPUT_DIR"

if [[ "$SKIP_FETCH" -eq 1 ]]; then
  log "created manual evidence bundle without fetching live pages"
  log "artifact root: ${OUTPUT_DIR}"
  exit 0
fi

log "capturing server metadata from ${HTTP_BASE}"
fetch_to_file "${HTTP_BASE}/healthz" "${OUTPUT_DIR}/server/healthz.txt"
fetch_to_file "${HTTP_BASE}/v1/info" "${OUTPUT_DIR}/server/info.json"
fetch_to_file "${HTTP_BASE}/v1/realtime" "${OUTPUT_DIR}/server/realtime.json"

if [[ "$MODE" == "built-in" || "$MODE" == "both" ]]; then
  log "capturing built-in Web/H5 pages"
  fetch_to_file "${HTTP_BASE}/debug/realtime-h5/settings.html" "${OUTPUT_DIR}/pages/built-in/settings.html"
  fetch_to_file "${HTTP_BASE}/debug/realtime-h5/" "${OUTPUT_DIR}/pages/built-in/index.html"
  fetch_to_file "${HTTP_BASE}/debug/realtime-h5/app.js" "${OUTPUT_DIR}/pages/built-in/app.js"
  fetch_to_file "${HTTP_BASE}/debug/realtime-h5/settings.js" "${OUTPUT_DIR}/pages/built-in/settings.js"
  fetch_to_file "${HTTP_BASE}/debug/realtime-h5/styles.css" "${OUTPUT_DIR}/pages/built-in/styles.css"
fi

if [[ "$MODE" == "standalone" || "$MODE" == "both" ]]; then
  log "capturing standalone Web/H5 client pages"
  fetch_to_file "${STANDALONE_BASE}/settings.html" "${OUTPUT_DIR}/pages/standalone/settings.html"
  fetch_to_file "${STANDALONE_BASE}/" "${OUTPUT_DIR}/pages/standalone/index.html"
  fetch_to_file "${STANDALONE_BASE}/app.js" "${OUTPUT_DIR}/pages/standalone/app.js"
  fetch_to_file "${STANDALONE_BASE}/settings.js" "${OUTPUT_DIR}/pages/standalone/settings.js"
  fetch_to_file "${STANDALONE_BASE}/styles.css" "${OUTPUT_DIR}/pages/standalone/styles.css"
fi

log "created manual evidence bundle at ${OUTPUT_DIR}"
log "next step: complete manual-checklist.md and attach screenshots / logs / WAV exports into the prepared directories"
