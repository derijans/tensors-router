#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
CGO_ENABLED="${CGO_ENABLED:-0}"
ROUTER_OUTPUT="${ROUTER_OUTPUT:-$ROOT_DIR/dist/tensors-router-$GOOS-$GOARCH}"
WEBUI_OUTPUT="${WEBUI_OUTPUT:-$ROOT_DIR/dist/tensor-reuter-webui-$GOOS-$GOARCH}"

mkdir -p "$(dirname "$ROUTER_OUTPUT")"
cd "$ROOT_DIR"

GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED="$CGO_ENABLED" go build -buildvcs=false -trimpath -ldflags "-s -w" -o "$ROUTER_OUTPUT" ./cmd/tensors-router
GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED="$CGO_ENABLED" go build -buildvcs=false -trimpath -ldflags "-s -w" -o "$WEBUI_OUTPUT" ./cmd/tensor-reuter-webui

echo "$ROUTER_OUTPUT"
echo "$WEBUI_OUTPUT"
