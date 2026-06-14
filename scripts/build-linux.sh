#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
CGO_ENABLED="${CGO_ENABLED:-0}"
ROUTER_OUTPUT="${ROUTER_OUTPUT:-$ROOT_DIR/dist/tensors-router-$GOOS-$GOARCH}"
WEBUI_OUTPUT="${WEBUI_OUTPUT:-$ROOT_DIR/dist/tensor-router-webui-$GOOS-$GOARCH}"
VERSION="${VERSION:-$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || true)}"
COMMIT="${COMMIT:-$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || true)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
LDFLAGS="-s -w -X tensors-router/internal/buildinfo.Version=$VERSION -X tensors-router/internal/buildinfo.Commit=$COMMIT -X tensors-router/internal/buildinfo.Date=$BUILD_DATE"

mkdir -p "$(dirname "$ROUTER_OUTPUT")"
cd "$ROOT_DIR"

GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED="$CGO_ENABLED" go build -buildvcs=false -trimpath -ldflags "$LDFLAGS" -o "$ROUTER_OUTPUT" ./cmd/tensors-router
GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED="$CGO_ENABLED" go build -buildvcs=false -trimpath -ldflags "$LDFLAGS" -o "$WEBUI_OUTPUT" ./cmd/tensor-router-webui

echo "$ROUTER_OUTPUT"
echo "$WEBUI_OUTPUT"
