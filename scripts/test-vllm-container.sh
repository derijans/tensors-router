#!/usr/bin/env bash
# Builds a vllm-node* container image, has the real tensor-router-vllm companion
# install vLLM into it through its normal (unverified/PyPI) install path, and - unless
# INSTALL_ONLY=1 - loads a small model and checks that it generates non-empty text.
#
# Usage: scripts/test-vllm-container.sh <cpu|cuda|rocm>
#
# Env vars:
#   CACHE_DIR                 Cache root for the downloaded model and the companion's
#                              persistent /data/vllm (default: .tmp/vllm-smoke)
#   MODEL_REPO                Hugging Face repo to download (default: Qwen/Qwen2.5-0.5B-Instruct)
#   MODEL_REVISION             Pinned revision/commit to download (default: main)
#   VLLM_VERSION               Exact `vllm==<version>` to install (device-specific default below)
#   VLLM_PYTHON_VERSION         Interpreter uv provisions (default: 3.12)
#   VLLM_INDEX_URL / VLLM_EXTRA_INDEX_URL   Override the wheel index(es) used to install vLLM
#   INSTALL_ONLY=1              Only run generate-check --install-only; skip loading a model
#
# Default wheel sources per device (overridable above):
#   cpu:  vllm==0.30.0+cpu from https://wheels.vllm.ai/0.30.0/cpu, plus
#         https://download.pytorch.org/whl/cpu for torch (uv accepts a space-separated
#         --extra-index-url list, which the companion passes through verbatim).
#   cuda: vllm==0.30.0 from PyPI (PyPI's vllm wheel is CUDA-built)
#   rocm: vllm==0.30.0+rocm723 from https://wheels.vllm.ai/rocm/
#
# Two product bugs this script previously had to work around are now fixed:
#   - vllm-node's base image was debian:bookworm-slim (glibc 2.36), which cannot
#     install vLLM's own CPU/ROCm wheels (glibc >= 2.39 required). runtime-vllm in the
#     Containerfile is now ubuntu:24.04, matching runtime-cuda/runtime-rocm.
#   - CommandSmokeTester's post-install check compared vllm.__version__, which drops a
#     wheel's local version segment ("0.30.0+cpu" reports as "0.30.0"), against the
#     exact pinned version - so any +cpu/+rocm... pin always failed even on a correct
#     install. It now compares importlib.metadata.version('vllm') instead, which
#     matches the installed wheel's real version.
#
# Requires docker and, for cuda/rocm without INSTALL_ONLY, the matching accelerator.
# Disk note: the rocm image plus its torch wheels are large; prune
# (docker image prune, docker volume prune) between runs if disk is tight.
set -euo pipefail

device=${1:?usage: scripts/test-vllm-container.sh <cpu|cuda|rocm>}
case "$device" in
  cpu|cuda|rocm) ;;
  *) echo "unsupported device $device (expected cpu, cuda, or rocm)" >&2; exit 1 ;;
esac

cache_dir=${CACHE_DIR:-.tmp/vllm-smoke}
model_repo=${MODEL_REPO:-Qwen/Qwen2.5-0.5B-Instruct}
model_revision=${MODEL_REVISION:-main}
python_version=${VLLM_PYTHON_VERSION:-3.12}
install_only=${INSTALL_ONLY:-0}

case "$device" in
  cpu)
    target=vllm-node
    default_version="0.30.0+cpu"
    default_extra_index="https://wheels.vllm.ai/0.30.0/cpu https://download.pytorch.org/whl/cpu"
    default_index=""
    ;;
  cuda)
    target=vllm-node-cuda
    default_version="0.30.0"
    default_extra_index=""
    default_index=""
    ;;
  rocm)
    target=vllm-node-rocm
    default_version="0.30.0+rocm723"
    default_extra_index="https://wheels.vllm.ai/rocm/"
    default_index=""
    ;;
esac

vllm_version=${VLLM_VERSION:-$default_version}
index_url=${VLLM_INDEX_URL:-$default_index}
extra_index_url=${VLLM_EXTRA_INDEX_URL:-$default_extra_index}

image="tensors-router-vllm-smoke:$device"
model_dir="$cache_dir/models/$(basename "$model_repo")"
data_dir="$cache_dir/data-$device"
result_dir="$cache_dir/results"
log_path="$result_dir/$device.log"
result_path="$result_dir/$device.json"

echo "== building tensors-router $target ==" >&2
docker build -f Containerfile --target "$target" -t "$image" .

mkdir -p "$model_dir" "$data_dir" "$result_dir"

if [[ "$install_only" != "1" ]]; then
  if [[ ! -f "$model_dir/config.json" ]]; then
    echo "== downloading $model_repo@$model_revision ==" >&2
    # HF_HUB_DISABLE_XET=1: the xet transfer backend silently produced empty
    # downloads for large files (e.g. model.safetensors) under this host's egress
    # setup, with hf download exiting 0 regardless. The plain HTTP transfer does not.
    HF_HUB_DISABLE_XET=1 uvx --from huggingface_hub hf download "$model_repo" --revision "$model_revision" --local-dir "$model_dir"
  else
    echo "== reusing cached snapshot at $model_dir ==" >&2
  fi
fi

# The companion's runtime user is uid:gid 10001:10001 (see Containerfile). Host-created
# bind mounts otherwise belong to whoever ran this script, which the container user
# cannot write (data_dir) or, on some hosts, even read (model_dir).
chown -R 10001:10001 "$data_dir" "$model_dir" 2>/dev/null || true

device_arguments=()
case "$device" in
  cuda)
    device_arguments+=(--gpus all)
    ;;
  rocm)
    device_arguments+=(--device /dev/kfd --device /dev/dri --group-add video --group-add render)
    ;;
esac

command_arguments=(
  generate-check
  --data-dir /data/vllm
  --expected-device "$device"
  --unverified-python-version "$python_version"
)
[[ -z "$vllm_version" ]] || command_arguments+=(--unverified-vllm-version "$vllm_version")
[[ -z "$index_url" ]] || command_arguments+=(--unverified-index-url "$index_url")
[[ -z "$extra_index_url" ]] || command_arguments+=(--unverified-extra-index-url "$extra_index_url")
if [[ "$install_only" == "1" ]]; then
  command_arguments+=(--install-only true)
else
  command_arguments+=(--model /models/model)
fi

run_arguments=(
  run --rm
  --entrypoint tensor-router-vllm
  --shm-size "${TENSORS_ROUTER_VLLM_SHM_SIZE:-4g}"
  -v "$data_dir:/data/vllm"
  "${device_arguments[@]}"
)
[[ "$install_only" == "1" ]] || run_arguments+=(-v "$model_dir:/models/model:ro")

echo "== running generate-check ($device, vllm==${vllm_version:-unpinned}) ==" >&2
if docker "${run_arguments[@]}" "$image" "${command_arguments[@]}" >"$result_path" 2>"$log_path"; then
  cat "$result_path"
  echo "generate-check ($device) succeeded; result: $result_path log: $log_path" >&2
else
  status=$?
  echo "generate-check ($device) failed (exit $status); log: $log_path" >&2
  tail -n 60 "$log_path" >&2 || true
  exit "$status"
fi
