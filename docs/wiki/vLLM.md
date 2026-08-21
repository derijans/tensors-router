# vLLM

The `vllm` backend uses the resident `tensor-router-vllm` companion. The router executable does not link Python, vLLM, hardware plugins, or their package managers. The companion owns isolated runtime environments and vLLM child processes.

## Supported hosts

Release archives include the companion on Linux amd64, Linux arm64, and macOS arm64. Native Windows and Linux armv7 archives remain unchanged. Run a Linux archive inside WSL 2 for a Windows-hosted vLLM deployment; this is a Linux deployment and not native Windows support.

Signed profiles may describe NVIDIA CUDA, AMD ROCm, Intel XPU, CPU, and Apple Metal installation methods for x86, AArch64, and Apple Silicon where upstream supports that combination. A profile is published only after its exact wheel, source-build, or OCI method passes import and serving smoke tests on the matching platform or an approved vendor runner.

`profile: auto` chooses the highest-priority compatible accelerated profile, then CPU when no accelerator is detected. If an accelerator is detected but required drivers, SDKs, container engine, compiler, or OS packages are missing, initialization fails with prerequisite guidance. The companion never installs those prerequisites.

## Initialization

No runtime is installed during router startup or model loading. An administrator starts initialization from the selected Nodes panel or calls:

```http
POST /router/v1/site/nodes/backends/init
Content-Type: application/json

{"node_id":"local","backend_id":"vllm","profile":"auto"}
```

Initialization returns `202 Accepted`. Repeated requests return the active persistent job instead of creating another. Cancellation leaves the previous promoted environment intact. A staged environment is promoted only after artifact verification, import validation, and a serving smoke test.

The companion resolves its manifest from one of four trust tiers. Each runtime artifact has an exact version, URL, byte size, SHA-256 digest, installation method, compatibility fields, and prerequisites. Release builds embed the platform-matched `uv` 0.12.0 bootstrap; the release workflow verifies the embedded bytes before packaging. Runtime initialization extracts those bytes into the staged environment and uses content-addressed per-profile directories without modifying system Python, `pip`, `PATH`, or user environments. Stable versions are pinned; nightlies and runtime-resolved `latest` packages are rejected everywhere except the `unverified` tier, which pins nothing at all.

### Manifest trust tiers

| Tier | When it applies | Guarantee |
|---|---|---|
| `tuf` | `vllm.tuf_repository_url` is set and `runtimes/vllm/<os>-<arch>.json` is published | Independently signed, expiring, revocable, rollback-protected |
| `operator-pinned` | `vllm.tuf_repository_url` is empty and `vllm.manifest_sha256` plus `vllm.manifest_size` pin a local `vllm.manifest_path` | Bytes fixed by an operator-supplied digest and length |
| `embedded-default` | TUF metadata verified but no runtime manifest has been published for this platform | Bytes fixed by the manifest compiled into the release binary |
| `unverified` | `vllm.allow_unverified_install` is `true` and neither of the above resolved a manifest | None. `uv pip install vllm` runs against PyPI with no artifact list to check bytes against |

The companion falls back to `embedded-default`, and from there to `unverified` if enabled, only after the TUF metadata chain refreshes successfully and the platform's target turns out not to exist. A signature, expiry, rollback, transport, or digest failure is never a reason to fall back: those still fail closed with the previous environment untouched. An operator-pinned manifest never falls back either, because a missing or mismatched pinned file is an error rather than an invitation to install something else. Nothing parsed from a TUF-signed or operator-pinned manifest can ever select the `unverified` tier; it is only reachable through the explicit config flag.

As of this writing, no `runtimes/vllm/*` TUF target has been published (it requires an evidence bundle from protected self-hosted GPU runners this deployment does not have) and `internal/vllm/defaults/` ships no embedded manifests, so `tuf` and `embedded-default` both currently fail for every platform. Until one of those is populated, `unverified` is the only tier that can actually install vLLM. Set it deliberately:

```yaml
vllm:
  allow_unverified_install: true
  # unverified_vllm_version pins the exact release; empty installs latest, which is
  # unpinned even by version. unverified_python_version defaults to 3.12 when empty.
  # unverified_extra_index_url reaches a CUDA/ROCm-specific torch wheel index.
  unverified_vllm_version: "0.6.3"
  unverified_python_version: "3.12"
  unverified_extra_index_url: "https://download.pytorch.org/whl/cu129"
```

The active tier is reported as `manifest_trust` on the vLLM state and logged as a warning whenever it is not `tuf`. Publishing a reviewed bundle under `tuf/profile-evidence/` is what moves a platform to the `tuf` tier.

### Initializing an unverified install per platform

Initialization is the same everywhere: set the options above, open the node in the WebUI, and press **backend needs init**. What differs is whether PyPI can supply a usable build.

| Host | Works | Notes |
| --- | --- | --- |
| CUDA | Yes | PyPI vLLM wheels are CUDA builds. Point `unverified_extra_index_url` at the torch index matching your driver, for example `https://download.pytorch.org/whl/cu129`. |
| CPU | Yes for import; serving depends on the release | The published wheels are built for CUDA, so a CPU-only host can install and import but may not serve. |
| ROCm | **No** | vLLM publishes no ROCm wheels, on PyPI or in AMD's ROCm wheel index. |

On ROCm the resolver has no good answer. Left on uv's default index strategy it cannot satisfy the dependency graph at all; widened, it silently prefers PyPI's newer **CUDA** torch and produces a complete, importable, entirely unusable stack. The smoke test therefore inspects the resolved torch build and fails with an explanation rather than letting a mismatched install look successful:

```
this host reports a ROCm accelerator but the resolved torch "2.13.0+cu130" is a CUDA
build; PyPI publishes only CUDA vLLM wheels, so an unverified install cannot target
ROCm - use an OCI profile built for ROCm instead
```

A ROCm deployment needs a vendor image such as `rocm/vllm`, which is the `oci` installation method rather than the unverified PyPI path. Note that an OCI runtime is launched through the local container engine, so the router has to run somewhere a container engine is reachable - the published router images deliberately contain none.

Vendor images that install their interpreter under `/root` cannot run as the host user. For those, set:

```yaml
vllm:
  oci_run_as_image_user: true
```

This drops only the forced host-user mapping. The read-only root filesystem, dropped capabilities, and `no-new-privileges` all still apply. It is off by default because anything the runtime writes then lands as the image's user rather than yours.

OCI profiles additionally pin the imported image by immutable `sha256:` image ID, require a preinstalled Docker or Podman engine, and run with read-only model mounts, a private socket mount, dropped capabilities, offline model-loading variables, and only the device mappings declared by the signed profile. The companion never installs the engine.

Model loading is offline and never installs or downloads packages or models. A vLLM inference request before successful initialization returns `503` with `backend_not_initialized`.

## Launch options

Three offline switches applied to every runtime process are selectable per node rather than fixed:

| Option | Environment variable |
| --- | --- |
| `hf_hub_offline` | `HF_HUB_OFFLINE` |
| `transformers_offline` | `TRANSFORMERS_OFFLINE` |
| `hf_datasets_offline` | `HF_DATASETS_OFFLINE` |

All three default to on, which keeps a running model from reaching Hugging Face so only the local pinned snapshot is used. Turning one off lets vLLM resolve missing files over the network, which is occasionally needed for a model whose snapshot is incomplete.

Select them in the vLLM section of the Nodes panel and press **Apply and reload**. The choice is stored in the companion's data directory and survives a router or companion restart. Applying unloads any running runtime, because the environment is fixed when the process starts; the next load uses the new selection. The same values are readable and writable through `/router/v1/site/nodes/backends/launch-options`.

## Model configuration

Set `backend_mode` and a `vllm` section in the `.kcpps` file:

```json
{
  "backend_mode": "vllm",
  "vllm": {
    "snapshot": {
      "repository": "organization/model",
      "revision": "full-immutable-revision",
      "path": "/models/snapshots/organization/model/full-immutable-revision",
      "tree_digest": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    },
    "runner": "generate",
    "task": "generate",
    "served_names": ["example-model"],
    "static_adapters": [],
    "settings": {
      "dtype": "auto",
      "max_model_length": 8192,
      "gpu_memory_utilization": 0.9,
      "tensor_parallel_size": 1,
      "pipeline_parallel_size": 1,
      "data_parallel_size": 1,
      "max_number_sequences": 64,
      "enable_chunked_prefill": true
    },
    "serve_args": [],
    "trust_remote_code": false,
    "external_tool_servers": []
  }
}
```

`runner` selects the lazy generation, pooling, or speech runtime. The local snapshot path and normalized tree digest identify immutable Hugging Face snapshot contents. Served names and static adapters become catalog aliases without changing snapshot identity.

Typed settings cover common controls. Ordered `serve_args` retain upstream features that are not router-owned, including architecture, quantization, loader, multimodal processor, speculative decoding, and local parallelism choices. Listener addresses, API keys, development mode, gRPC, Ray, remote scale-out, arbitrary middleware, unrestricted plugins, and similar router-bypassing options are rejected.

Remote code and external tool servers default to disabled. Both the router configuration and model configuration must explicitly enable their respective use. Tool servers use explicit `host:port` entries; URLs, credentials, paths, queries, and comma injection are rejected.

## API boundary

Stable vLLM online-serving inference is exposed through the router's inference authentication: OpenAI Completions, Chat, Chat Batch, Responses, Embeddings, Transcriptions, Translations, Realtime transcription, Anthropic Messages and token counting, Cohere Embed and Rerank, Classification, Score, Pooling, Generative Scoring, SageMaker invocation, tokenize, and detokenize.

Health, version, load, metrics, tokenizer information, dynamic LoRA, and Elastic Expert Parallelism are available only under the administrator-authenticated `/router/v1/vllm/...` namespace. Dynamic LoRA and EEP also require their separate configuration switches.

Development, profiler, arbitrary RPC, weight-transfer, sleep, offline Python, native multi-node, disaggregated, gRPC, Ray, renderer, and derenderer surfaces are not proxied. vLLM-Omni is a separate project and is outside this backend.

The router connects to vLLM over a private Unix-domain socket. It strips client and backend credentials and hop-by-hop headers, applies router limits, and allowlists every method and path. See the upstream [security guidance](https://docs.vllm.ai/en/stable/usage/security/), [online-serving surface](https://docs.vllm.ai/en/latest/serving/online_serving/), [Realtime API](https://docs.vllm.ai/en/stable/serving/online_serving/speech_to_text/), and [installation matrix](https://docs.vllm.ai/en/stable/getting_started/installation/index.html).

## Release profile gates

Every signed hardware profile must pass its installation method on the matching platform or vendor runner. Required gates are import and serving smoke tests, the Go suite and race tests, WebUI checks, `govulncheck`, npm audit, Python dependency audit of the resolved environment, and container/runtime vulnerability scans. Failure prevents that profile from entering signed TUF targets. A companion binary can therefore be present while a requested profile remains unavailable.
