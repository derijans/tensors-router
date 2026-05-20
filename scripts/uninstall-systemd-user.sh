#!/usr/bin/env bash
set -euo pipefail

SERVICE_PATH="$HOME/.config/systemd/user/tensors-router.service"

if ! command -v systemctl >/dev/null 2>&1; then
  rm -f "$SERVICE_PATH"
  echo "removed $SERVICE_PATH"
  exit 0
fi

systemctl --user disable --now tensors-router.service 2>/dev/null || true
rm -f "$SERVICE_PATH"
systemctl --user daemon-reload

echo "removed $SERVICE_PATH"
