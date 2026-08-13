#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ "$(uname -s)" != "Darwin" || "$(uname -m)" != "arm64" ]]; then
  echo "macOS vLLM release builds require an Apple Silicon runner" >&2
  exit 1
fi

OUTPUT_DIR="${OUTPUT_DIR:-$ROOT_DIR/dist}"
VERSION="${VERSION:-$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || true)}"
COMMIT="${COMMIT:-$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || true)}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
LDFLAGS="-s -w -X tensors-router/internal/buildinfo.Version=$VERSION -X tensors-router/internal/buildinfo.Commit=$COMMIT -X tensors-router/internal/buildinfo.Date=$BUILD_DATE"

mkdir -p "$OUTPUT_DIR"
cd "$ROOT_DIR"
npm --prefix webui run build

UV_VERSION="${UV_VERSION:-0.12.0}"
if [[ "$(uv --version | awk '{print $2}')" != "$UV_VERSION" ]]; then
  echo "vLLM companion requires uv $UV_VERSION for its embedded bootstrap" >&2
  exit 1
fi
mkdir -p internal/vllm/assets
cp -L "$(command -v uv)" internal/vllm/assets/uv
go test -tags vllm_embedded_uv ./internal/vllm ./cmd/tensor-router-vllm

for target in \
  "tensors-router ./cmd/tensors-router" \
  "tensor-router-webui ./cmd/tensor-router-webui" \
  "tensor-router-downloader ./cmd/tensor-router-downloader" \
  "tensor-router-vllm ./cmd/tensor-router-vllm"; do
  read -r name package <<<"$target"
  build_tags=()
  if [[ "$name" == "tensor-router-vllm" ]]; then
    build_tags=(-tags vllm_embedded_uv)
  fi
  go build "${build_tags[@]}" -buildvcs=false -trimpath -ldflags "$LDFLAGS" -o "$OUTPUT_DIR/$name-darwin-arm64" "$package"
  echo "$OUTPUT_DIR/$name-darwin-arm64"
done
"$OUTPUT_DIR/tensor-router-vllm-darwin-arm64" bootstrap-info
