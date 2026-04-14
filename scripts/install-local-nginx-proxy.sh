#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PUBLIC_IP="${PUBLIC_IP:-101.33.235.154}"
SSL_DIR="/etc/ssl/agent-server"
NGINX_SITE="/etc/nginx/sites-available/agent-server.conf"
NGINX_ENABLED="/etc/nginx/sites-enabled/agent-server.conf"

export DEBIAN_FRONTEND=noninteractive
if ! command -v nginx >/dev/null 2>&1; then
  apt-get update
  apt-get install -y nginx
fi

mkdir -p "$SSL_DIR"
if [[ ! -f "$SSL_DIR/selfsigned.crt" || ! -f "$SSL_DIR/selfsigned.key" ]]; then
  openssl req -x509 -nodes -newkey rsa:2048 -days 365 \
    -keyout "$SSL_DIR/selfsigned.key" \
    -out "$SSL_DIR/selfsigned.crt" \
    -subj "/CN=$PUBLIC_IP" \
    -addext "subjectAltName = IP:$PUBLIC_IP,IP:127.0.0.1"
  chmod 600 "$SSL_DIR/selfsigned.key"
fi

install -m 0644 "$REPO_ROOT/deploy/nginx/agent-server.conf" "$NGINX_SITE"
ln -sfn "$NGINX_SITE" "$NGINX_ENABLED"
rm -f /etc/nginx/sites-enabled/default
nginx -t
systemctl enable --now nginx
systemctl reload nginx

if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then
  ufw allow 80/tcp
  ufw allow 443/tcp
  ufw allow 8080/tcp
fi
