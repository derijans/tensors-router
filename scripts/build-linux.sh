#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
CGO_ENABLED="${CGO_ENABLED:-0}"
ROUTER_OUTPUT="${ROUTER_OUTPUT:-$ROOT_DIR/dist/tensors-router-$GOOS-$GOARCH}"
WEBUI_OUTPUT="${WEBUI_OUTPUT:-$ROOT_DIR/dist/tensor-router-webui-$GOOS-$GOARCH}"
DOWNLOADER_OUTPUT="${DOWNLOADER_OUTPUT:-$ROOT_DIR/dist/tensor-router-downloader-$GOOS-$GOARCH}"
VLLM_OUTPUT="${VLLM_OUTPUT:-$ROOT_DIR/dist/tensor-router-vllm-$GOOS-$GOARCH}"
VERSION="${VERSION:-$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || true)}"
COMMIT="${COMMIT:-$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || true)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
LDFLAGS="-s -w -X tensors-router/internal/buildinfo.Version=$VERSION -X tensors-router/internal/buildinfo.Commit=$COMMIT -X tensors-router/internal/buildinfo.Date=$BUILD_DATE"

VLLM_SUPPORTED=false
if [[ "$GOOS" == "linux" && ( "$GOARCH" == "amd64" || "$GOARCH" == "arm64" ) ]]; then
  VLLM_SUPPORTED=true
fi

mkdir -p "$(dirname "$ROUTER_OUTPUT")"
cd "$ROOT_DIR"

if [[ "$VLLM_SUPPORTED" == true ]]; then
  HOST_GOOS="$(go env GOOS)"
  HOST_GOARCH="$(go env GOARCH)"
  if [[ "$HOST_GOOS/$HOST_GOARCH" != "$GOOS/$GOARCH" ]]; then
    echo "vLLM companion requires a matching native $GOOS/$GOARCH build host" >&2
    exit 1
  fi
  UV_VERSION="${UV_VERSION:-0.12.0}"
  if [[ "$(uv --version | awk '{print $2}')" != "$UV_VERSION" ]]; then
    echo "vLLM companion requires uv $UV_VERSION for its embedded bootstrap" >&2
    exit 1
  fi
  mkdir -p internal/vllm/assets
  cp -L "$(command -v uv)" internal/vllm/assets/uv
fi

GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED="$CGO_ENABLED" go build -buildvcs=false -trimpath -ldflags "$LDFLAGS" -o "$ROUTER_OUTPUT" ./cmd/tensors-router
GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED="$CGO_ENABLED" go build -buildvcs=false -trimpath -ldflags "$LDFLAGS" -o "$WEBUI_OUTPUT" ./cmd/tensor-router-webui
GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED="$CGO_ENABLED" go build -buildvcs=false -trimpath -ldflags "$LDFLAGS" -o "$DOWNLOADER_OUTPUT" ./cmd/tensor-router-downloader
if [[ "$VLLM_SUPPORTED" == true ]]; then
  GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED="$CGO_ENABLED" go build -tags vllm_embedded_uv -buildvcs=false -trimpath -ldflags "$LDFLAGS" -o "$VLLM_OUTPUT" ./cmd/tensor-router-vllm
  "$VLLM_OUTPUT" bootstrap-info
fi

echo "$ROUTER_OUTPUT"
echo "$WEBUI_OUTPUT"
echo "$DOWNLOADER_OUTPUT"
if [[ "$VLLM_SUPPORTED" == true ]]; then
  echo "$VLLM_OUTPUT"
fi
