#!/usr/bin/env bash
set -euo pipefail

APP_DIR="${1:-$PWD}"
BIN_PATH="${2:-$APP_DIR/tensors-router}"
CONFIG_PATH="${3:-$APP_DIR/config.yaml}"
SERVICE_DIR="$HOME/.config/systemd/user"
SERVICE_PATH="$SERVICE_DIR/tensors-router.service"

if ! command -v systemctl >/dev/null 2>&1; then
  echo "systemctl is required" >&2
  exit 1
fi

if [ ! -f "$BIN_PATH" ]; then
  echo "binary not found: $BIN_PATH" >&2
  exit 1
fi

if [ ! -x "$BIN_PATH" ]; then
  chmod +x "$BIN_PATH"
fi

if [ ! -f "$CONFIG_PATH" ]; then
  echo "config not found: $CONFIG_PATH" >&2
  exit 1
fi

mkdir -p "$SERVICE_DIR"

cat > "$SERVICE_PATH" <<SERVICE
[Unit]
Description=KoboldCpp OpenAI Router
After=network-online.target

[Service]
Type=simple
WorkingDirectory="$APP_DIR"
ExecStart="$BIN_PATH" serve --config "$CONFIG_PATH"
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
SERVICE

systemctl --user daemon-reload
systemctl --user enable tensors-router.service

echo "installed $SERVICE_PATH"
