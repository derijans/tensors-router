#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
CGO_ENABLED="${CGO_ENABLED:-0}"
OUTPUT="${OUTPUT:-$ROOT_DIR/dist/tensors-router-$GOOS-$GOARCH}"

mkdir -p "$(dirname "$OUTPUT")"
cd "$ROOT_DIR"

GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED="$CGO_ENABLED" go build -buildvcs=false -trimpath -ldflags "-s -w" -o "$OUTPUT" ./cmd/tensors-router

echo "$OUTPUT"
