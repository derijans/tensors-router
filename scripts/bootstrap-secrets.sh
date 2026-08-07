#!/usr/bin/env bash
set -euo pipefail

DEPLOYMENT_ROOT="${1:-}"
if [ -z "$DEPLOYMENT_ROOT" ] || [ "$DEPLOYMENT_ROOT" = "/" ]; then
  echo "usage: $0 ABSOLUTE_TENSORS_ROUTER_ROOT" >&2
  exit 1
fi

case "$DEPLOYMENT_ROOT" in
  /*) ;;
  *)
    echo "deployment root must be absolute" >&2
    exit 1
    ;;
esac

SECRET_DIR="$DEPLOYMENT_ROOT/secrets"
install -d -m 0700 "$SECRET_DIR"
umask 077

for secret_name in router-inference router-admin cluster webui-admin; do
  secret_path="$SECRET_DIR/$secret_name.token"
  if [ -e "$secret_path" ] || [ -L "$secret_path" ]; then
    if [ ! -f "$secret_path" ] || [ -L "$secret_path" ]; then
      echo "refusing non-regular secret path: $secret_path" >&2
      exit 1
    fi
    chmod 0600 "$secret_path"
    continue
  fi
  secret_value="$(od -An -N32 -tx1 /dev/urandom | tr -d '[:space:]')"
  if [[ ! "$secret_value" =~ ^[0-9a-f]{64}$ ]]; then
    echo "secure random secret generation failed" >&2
    exit 1
  fi
  printf '%s\n' "$secret_value" > "$secret_path"
  chmod 0600 "$secret_path"
done

echo "credentials ready in $SECRET_DIR"
