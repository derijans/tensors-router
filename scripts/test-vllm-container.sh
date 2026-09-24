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
#   cpu:  vllm from https://wheels.vllm.ai/0.30.0/cpu, plus
#         https://download.pytorch.org/whl/cpu for torch (uv accepts a space-separated
#         --extra-index-url list, which the companion passes through verbatim). Left
#         unpinned by VLLM_VERSION - see the note below.
#   cuda: vllm==0.30.0 from PyPI (PyPI's vllm wheel is CUDA-built)
#   rocm: vllm from https://wheels.vllm.ai/rocm/. Also left unpinned - see below.
#
# Known issue this script works around: internal/vllm's CommandSmokeTester asserts
# vllm.__version__ == the exact pinned version string, but vllm.__version__ never
# carries a local-version segment (a wheel built as "0.30.0+cpu" still reports
# "0.30.0"). Passing --unverified-vllm-version with a +cpu/+rocm... suffix therefore
# always fails that assertion, even on a fully correct install (verified against a
# real vllm-node-cuda-based container running the CPU wheel end to end). cpu/rocm are
# left unpinned here so the default run actually succeeds; VLLM_VERSION overrides this
# if you want to reproduce the bug or a fix for it is in place.
#
# Known limitation this script does NOT work around: `cpu`'s vllm-node target is built
# on debian:bookworm-slim (glibc 2.36). vLLM's own CPU and ROCm wheels require glibc
# >= 2.39 (manylinux_2_39), so `cpu` fails at the "creating isolated Python environment"
# / dependency-resolution step on that base image no matter which index is used -
# confirmed by running this exact install against an Ubuntu 24.04 (glibc 2.39) vLLM
# image instead, where it succeeds and generates text. `rocm`'s vllm-node-rocm target is
# already Ubuntu 24.04, so it is unaffected. Fixing `cpu` means rebasing runtime-vllm
# in the Containerfile onto a glibc >= 2.39 distro - a real product change, not
# something this script can paper over.
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
    default_version=""
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
    default_version=""
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
