#!/usr/bin/env bash

set -euo pipefail

if [[ $EUID -ne 0 ]]; then
  echo "Run as root."
  exit 1
fi

: "${AGENT_BIN_URL:?AGENT_BIN_URL is required}"
: "${COORDINATOR_URL:?COORDINATOR_URL is required}"
: "${COORDINATOR_ACCESS_TOKEN:?COORDINATOR_ACCESS_TOKEN is required}"
: "${AGENT_ID:?AGENT_ID is required}"
: "${AGENT_PUBLIC_URL:?AGENT_PUBLIC_URL is required}"

AGENT_USER="${AGENT_USER:-agentsvc}"
AGENT_GROUP="${AGENT_GROUP:-$AGENT_USER}"
AGENT_HOME="${AGENT_HOME:-/opt/mytonstorage-agent}"
AGENT_PORT="${AGENT_PORT:-9091}"
AGENT_LOG_LEVEL="${AGENT_LOG_LEVEL:-1}"
AGENT_ACCESS_TOKEN="${AGENT_ACCESS_TOKEN:-}"
AGENT_REGISTRATION_INTERVAL_SEC="${AGENT_REGISTRATION_INTERVAL_SEC:-15}"
AGENT_COORDINATOR_TIMEOUT_SEC="${AGENT_COORDINATOR_TIMEOUT_SEC:-10}"

echo "[1/6] Installing runtime dependencies..."
apt-get update
apt-get install -y curl ca-certificates

echo "[2/6] Creating service user..."
if ! id "$AGENT_USER" >/dev/null 2>&1; then
  useradd --system --home "$AGENT_HOME" --shell /usr/sbin/nologin "$AGENT_USER"
fi

mkdir -p "$AGENT_HOME"/bin "$AGENT_HOME"/etc
chown -R "$AGENT_USER":"$AGENT_GROUP" "$AGENT_HOME"

echo "[3/6] Downloading agent binary..."
curl -fsSL "$AGENT_BIN_URL" -o "$AGENT_HOME/bin/agent"
chmod +x "$AGENT_HOME/bin/agent"
chown "$AGENT_USER":"$AGENT_GROUP" "$AGENT_HOME/bin/agent"

echo "[4/6] Writing environment file..."
cat > "$AGENT_HOME/etc/agent.env" <<EOF
AGENT_PORT=$AGENT_PORT
AGENT_ACCESS_TOKEN=$AGENT_ACCESS_TOKEN
AGENT_LOG_LEVEL=$AGENT_LOG_LEVEL
COORDINATOR_URL=$COORDINATOR_URL
COORDINATOR_ACCESS_TOKEN=$COORDINATOR_ACCESS_TOKEN
AGENT_ID=$AGENT_ID
AGENT_PUBLIC_URL=$AGENT_PUBLIC_URL
AGENT_REGISTRATION_INTERVAL_SEC=$AGENT_REGISTRATION_INTERVAL_SEC
AGENT_COORDINATOR_TIMEOUT_SEC=$AGENT_COORDINATOR_TIMEOUT_SEC
EOF
chmod 640 "$AGENT_HOME/etc/agent.env"
chown "$AGENT_USER":"$AGENT_GROUP" "$AGENT_HOME/etc/agent.env"

echo "[5/6] Installing systemd service..."
cat > /etc/systemd/system/mytonstorage-agent.service <<EOF
[Unit]
Description=MyTonStorage Agent Service
After=network.target

[Service]
Type=simple
User=$AGENT_USER
Group=$AGENT_GROUP
WorkingDirectory=$AGENT_HOME
EnvironmentFile=$AGENT_HOME/etc/agent.env
ExecStart=$AGENT_HOME/bin/agent
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

echo "[6/6] Enabling service..."
systemctl daemon-reload
systemctl enable --now mytonstorage-agent

echo "Done. Check service status:"
echo "  systemctl status mytonstorage-agent --no-pager"
echo "Healthcheck:"
echo "  curl -s http://127.0.0.1:${AGENT_PORT}/health"
