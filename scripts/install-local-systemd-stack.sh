#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SERVICE_USER="${SERVICE_USER:-ubuntu}"
SYSTEMD_DIR="/etc/systemd/system"
ETC_DIR="/etc/agent-server"
mkdir -p "$ETC_DIR"

if [[ ! -f "$ETC_DIR/funasr-worker.env" ]]; then
  install -m 0644 "$REPO_ROOT/deploy/systemd/agent-server-funasr-worker.env.example" "$ETC_DIR/funasr-worker.env"
fi
if [[ ! -f "$ETC_DIR/agentd.env" ]]; then
  install -m 0644 "$REPO_ROOT/deploy/systemd/agent-server-agentd.env.example" "$ETC_DIR/agentd.env"
fi

mkdir -p "$REPO_ROOT/bin"
go build -o "$REPO_ROOT/bin/agentd" ./cmd/agentd
chown "$SERVICE_USER":"$SERVICE_USER" "$REPO_ROOT/bin/agentd"
chmod 0755 "$REPO_ROOT/bin/agentd"
chmod 0755 "$REPO_ROOT/scripts/run-funasr-worker-local.sh" "$REPO_ROOT/scripts/run-agentd-local.sh"

install -m 0644 "$REPO_ROOT/deploy/systemd/agent-server-funasr-worker.service" "$SYSTEMD_DIR/agent-server-funasr-worker.service"
install -m 0644 "$REPO_ROOT/deploy/systemd/agent-server-agentd.service" "$SYSTEMD_DIR/agent-server-agentd.service"

systemctl daemon-reload
systemctl enable --now agent-server-funasr-worker.service
systemctl enable --now agent-server-agentd.service
