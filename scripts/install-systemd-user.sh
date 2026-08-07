#!/usr/bin/env bash
set -euo pipefail

APP_DIR="${1:-$PWD}"
BIN_PATH="${2:-$APP_DIR/tensors-router}"
CONFIG_PATH="${3:-$APP_DIR/config.yaml}"
STOP_DEADLINE="${4:-16m30s}"
SERVICE_DIR="$HOME/.config/systemd/user"
SERVICE_PATH="$SERVICE_DIR/tensors-router.service"

if [[ ! "$STOP_DEADLINE" =~ ^([0-9]+h)?([0-9]+m)?([0-9]+s)?$ ]] || [ -z "$STOP_DEADLINE" ]; then
  echo "stop deadline must use a strict duration such as 16m30s" >&2
  exit 1
fi

stop_hours="${BASH_REMATCH[1]%h}"
stop_minutes="${BASH_REMATCH[2]%m}"
stop_seconds="${BASH_REMATCH[3]%s}"
stop_hours="${stop_hours:-0}"
stop_minutes="${stop_minutes:-0}"
stop_seconds="${stop_seconds:-0}"
if [ "$stop_minutes" -ge 60 ] || [ "$stop_seconds" -ge 60 ]; then
  echo "stop deadline minutes and seconds must be below 60" >&2
  exit 1
fi
stop_deadline_seconds=$((10#$stop_hours * 3600 + 10#$stop_minutes * 60 + 10#$stop_seconds))
if [ "$stop_deadline_seconds" -le 0 ]; then
  echo "stop deadline must be positive" >&2
  exit 1
fi
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
KillSignal=SIGTERM
KillMode=mixed
TimeoutStopSec=${stop_deadline_seconds}s
SendSIGKILL=yes
FinalKillSignal=SIGKILL

[Install]
WantedBy=default.target
SERVICE

systemctl --user daemon-reload
systemctl --user enable tensors-router.service

echo "installed $SERVICE_PATH"
