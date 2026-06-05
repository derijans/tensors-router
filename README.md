# tensors-router

Router for `.kcpps` configs.

It exposes text configs as `/v1/models`, exposes image configs through image model endpoints, loads the selected config, and forwards requests to the active backend.

`backend.mode: "kobold"` uses one KoboldCpp process. `backend.mode: "llama_sdcpp"` uses `llama-server` for LLM, embeddings, and multimodal requests, and `sd-server` for image requests.

## Build

```bash
go build -o tensors-router ./cmd/tensors-router
go build -o tensor-reuter-webui ./cmd/tensor-reuter-webui
```

Linux amd64:

```bash
make build-linux
```

Equivalent commands:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags "-s -w" -o dist/tensors-router-linux-amd64 ./cmd/tensors-router
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags "-s -w" -o dist/tensor-reuter-webui-linux-amd64 ./cmd/tensor-reuter-webui
```

## Configure

```bash
cp config.example.yaml config.yaml
mkdir -p kcpps bin data
```

Put `.kcpps` files in `kcpps`.

Filename mapping:

```text
kcpps/hermes-8k.kcpps -> model id hermes-8k
kcpps/sdxl.kcpps with sdmodel C:\models\juggernautXL.safetensors -> image model id sdxl-juggernautXL
```

Main config fields:

```yaml
server:
  bind: "127.0.0.1:8080"
  allowed_cidrs:
    - "127.0.0.0/8"
    - "::1/128"

auth:
  bearer_keys: []

models:
  config_dir: "./kcpps"
  startup_model: ""
  file_roots:
    - "./models"

backend:
  mode: "kobold"

kobold:
  backend_url: "http://127.0.0.1:5001"
  binary_path: "./bin/kobold/koboldcpp"
  data_dir: "./data"
  no_model: true

llama:
  backend_url: "http://127.0.0.1:5002"
  binary_path: "./bin/llama/llama-b9495/llama-server"
  data_dir: "./data/llama"
  hide_window: true
  extra_args: []

sdcpp:
  backend_url: "http://127.0.0.1:7860"
  binary_path: "./bin/stable-diffusion/build/bin/sd-server"
  data_dir: "./data/sdcpp"
  hide_window: true
  extra_args: []

logging:
  enabled: true

updates:
  enabled: false
  check_interval: "168h"
  binary_url: "https://koboldai.org/cpplinuxrocm"
  binary_sha256: ""
  llama_binary_url: "https://github.com/ggml-org/llama.cpp/releases/download/b9495/llama-b9495-bin-ubuntu-rocm-7.2-x64.tar.gz"
  llama_binary_sha256: "49275ee2df07cd227dbf573470955222d1f0c43cf4a3da52a9f2ee98924ac9e0"
  sdcpp_binary_url: "https://github.com/leejet/stable-diffusion.cpp/releases/download/master-672-1f9ee88/sd-master-1f9ee88-bin-Linux-Ubuntu-24.04-x86_64-rocm-7.2.1.zip"
  sdcpp_binary_sha256: "4e680acbba39a994147cc05c35fc355f164340376a90c04790ad6f48caaa05c7"

cluster:
  role: "standalone"
  node_id: "local"
  public_url: ""
  master_url: ""
  slave_urls: []
  token: ""
  store_dir: "./router-store"
  sync_interval: "60s"
  health_interval: "15s"
```

`models.file_roots` is optional and used only by the management web UI model-file inventory and cooking APIs. The scanner is limited to those roots.

## Download Backends

```bash
./tensors-router download --config config.yaml
```

In `kobold` mode this downloads `kobold.binary_path` from `updates.binary_url` and verifies the downloaded payload against `updates.binary_sha256`.

In `llama_sdcpp` mode this downloads both `llama.binary_path` and `sdcpp.binary_path` from `updates.llama_binary_url` and `updates.sdcpp_binary_url`, then verifies the downloaded payloads against `updates.llama_binary_sha256` and `updates.sdcpp_binary_sha256`. For archive URLs, these hashes are archive SHA256 values. URLs must use HTTPS. Direct binaries, `.zip`, `.tar.gz`, and `.tgz` downloads are supported. Archives are extracted as-is into their backend folder, so `binary_path` must include the executable path inside that archive. The previous backend folder is removed before an archive update is installed. The example config pins Linux x64 ROCm archives for llama.cpp b9495 and stable-diffusion.cpp master-672-1f9ee88.

## Run

```bash
./tensors-router serve --config config.yaml
```

The router does not require the web UI.

Optional web UI:

```bash
cp webui.example.yaml webui.yaml
./tensor-reuter-webui --config webui.yaml
```

If `router.url` is empty, `tensor-reuter-webui` looks beside itself for `tensors-router`, launches it with `router.config_path`, and stops that managed router process when the web UI exits. If `router.url` is set, the router is treated as external and launch/restart/kill controls are disabled. The web UI serves HTTPS with a self-signed certificate from `server.state_dir` unless cert files are configured.

List LLM models:

```bash
curl http://127.0.0.1:8080/v1/models
```

Chat request:

```bash
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"hermes-8k","messages":[{"role":"user","content":"hello"}],"stream":true}'
```

## Routing Behavior

The router reloads the whole `.kcpps` file. It does not patch selected fields.

Set `models.startup_model` to preload one LLM config before the router starts listening. Leave it empty to keep lazy loading.

Set `backend.mode: "kobold"` to keep the original single KoboldCpp process. Set `backend.mode: "llama_sdcpp"` to route LLM, embeddings, and multimodal requests to `llama-server`, and image requests to `sd-server`. In split mode, the router starts each native process lazily from the selected `.kcpps` file and stops the previous process in that lane after in-flight requests finish.

Backend differences: [llama-sdcpp-differences.md](llama-sdcpp-differences.md).

Requests to `/v1/chat/completions` and `/v1/completions` require a `model` field. The model id must match a `.kcpps` filename stem.

Other `/v1/...` endpoints are forwarded too. If the JSON body has a `model` field, the matching `.kcpps` config is loaded first. If there is no `model` field, the request is forwarded to the currently loaded KoboldCpp config.

Examples:

```json
{"model":"embed","input":"hello"}
```

loads `embed.kcpps` before forwarding to `/v1/embeddings`.

Non-image KoboldCpp paths outside `/v1` are not proxied.

Image requests:

```bash
curl http://127.0.0.1:8080/sdapi/v1/sd-models
```

returns image model ids. Image-only configs are always listed. Combined LLM+image configs are listed only when that `.kcpps` is currently loaded.

```json
{"model":"sdxl-juggernautXL","prompt":"cat"}
```

loads `sdxl.kcpps` before forwarding to `/v1/images/generations`.

Image selection can come from JSON `model`, JSON `sd_model_checkpoint`, JSON `override_settings.sd_model_checkpoint`, query `model`, query `sd_model_checkpoint`, or `X-Tensors-Model`.

Stable Diffusion routes under `/sdapi/v1/...` and common ComfyUI-style image routes such as `/prompt`, `/queue`, `/history`, `/view`, and `/object_info` are proxied to KoboldCpp.

In `llama_sdcpp` mode, image routing targets stable-diffusion.cpp server APIs: `/v1/images/...`, `/sdapi/v1/...`, and `/sdcpp/v1/...`. Legacy ComfyUI-style routes such as `/prompt`, `/queue`, `/history`, `/view`, and `/object_info` are still recognized as image paths by the router but require a backend that implements them; `sd-server` does not provide those ComfyUI endpoints.

## Config Types

The router only reads enough `.kcpps` JSON to detect `sdmodel` and text model fields. KoboldCpp still receives the whole config, so Flux, SD3, WAN, Qwen Image, VAE, LoRA, CLIP/T5, GPU, and other KoboldCpp image settings stay in the `.kcpps`.

Usable config types depend on the KoboldCpp endpoint being called:

- Text config: use with `/v1/chat/completions` or `/v1/completions`; listed on `/v1/models`.
- Embeddings config: use with `/v1/embeddings` and include `model`.
- Image-only config: use with `/v1/images/...`, `/sdapi/v1/...`, or supported ComfyUI-style image routes; listed on `/sdapi/v1/sd-models` as `<kcpps name>-<sdmodel filename stem>`.
- Combined LLM+image config: use one `.kcpps` containing the text model plus `sdmodel`; its image id is available only while the combined config is loaded, and selecting it does not reload or unload the active LLM.

Only one KoboldCpp config is active at a time. LLM and image requests share one model gate: requests using the active config can run together, while a config switch waits for in-flight requests to finish.

In `llama_sdcpp` mode, combined LLM+image configs expose both lanes independently. Selecting the text model starts `llama-server`; selecting the image model starts `sd-server`; explicit `/router/v1/load` on a combined config starts both. The two lanes have separate gates, so a text model switch does not block an unrelated image request.

## Router Registry

Rich model list:

```bash
curl http://127.0.0.1:8080/router/v1/models
```

This returns local and clustered model records with capabilities, hashes, source, node id, and availability. The OpenAI `/v1/models` endpoint stays user-facing and does not expose node ids.

Load or unload a model explicitly:

```bash
curl http://127.0.0.1:8080/router/v1/load \
  -H "Content-Type: application/json" \
  -d '{"model":"hermes-8k"}'

curl -X POST http://127.0.0.1:8080/router/v1/unload
```

Unload restarts KoboldCpp in no-model mode and clears the router active config.

## Management Web UI API

Standalone and master routers expose `/router/v1/site/...` for the optional web UI. Slaves do not expose those browser-facing endpoints; they only accept cluster-token worker calls under `/router/v1/node/site/...`.

The cooking flow can create normal `.kcpps` configs on one node, or store a master split recipe in `cluster.store_dir` when selected LLM, image, and embedding components live on different machines.

## Cluster Routing

Set one router to `cluster.role: "master"` and slave routers to `cluster.role: "slave"`. Use the same `cluster.token` on all nodes. Slaves register with `cluster.master_url`; masters also query configured `cluster.slave_urls` at startup and on the health interval.

The master hashes referenced model files and path-normalized `.kcpps` configs into `cluster.store_dir`. If master and slaves have the same public model id with identical hashes, clients see one normal model id. If a slave has the same id with different hashes, the master exposes an indexed id such as `model-2`. Requested models are rewritten to the selected slave local id while forwarding, then responses are rewritten back to the public id.

Cluster model records include `backend_mode`. Masters use the text lane health for LLM, embedding, and multimodal routing, and the image lane health for image routing. Split-mode nodes can advertise combined config image models without the text config being active.

## Auth

Optional bearer keys:

```yaml
auth:
  bearer_keys:
    - "change-me"
```

Requests must also match `server.allowed_cidrs`.

## Logging

Set `logging.enabled: false` to suppress router event logs and discard KoboldCpp stdout/stderr instead of writing `koboldcpp.log`.

## Reverse Proxy

Use a reverse proxy for TLS in Local Net. Not production grade software. Bind this service to localhost unless you have a reason not to.

Nginx example:

```nginx
location /v1/ {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_buffering off;
    proxy_read_timeout 600s;
    proxy_send_timeout 600s;
}
```

Notes:

- Streaming uses HTTP event streams, not WebSocket.
- Disable response buffering for streaming.
- `allowed_cidrs` checks the proxy IP from `RemoteAddr`. It does not read `X-Forwarded-For`.
- If mounting under a prefix, strip the prefix before forwarding.

## systemd User Service

```bash
bash scripts/install-systemd-user.sh "$PWD" "$PWD/tensors-router" "$PWD/config.yaml"
systemctl --user start tensors-router.service
```

Remove:

```bash
bash scripts/uninstall-systemd-user.sh
```

## Warning

This router is meant for intranet use only. Do not expose it directly to the public internet. Bind it to localhost or a private interface, use a reverse proxy or VPN for access, and keep `server.allowed_cidrs` restricted.
