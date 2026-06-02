# tensors-router

Router for KoboldCpp `.kcpps` configs.

It runs a KoboldCpp process, exposes LLM `.kcpps` filenames as `/v1/models`, exposes image-capable configs through image model endpoints, reloads the matching config through KoboldCpp admin mode, then forwards the request to KoboldCpp.

## Build

```bash
go build -o tensors-router ./cmd/tensors-router
```

Linux amd64:

```bash
make build-linux
```

Equivalent command:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags "-s -w" -o dist/tensors-router-linux-amd64 ./cmd/tensors-router
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

kobold:
  backend_url: "http://127.0.0.1:5001"
  binary_path: "./bin/koboldcpp"
  data_dir: "./data"
  no_model: true

logging:
  enabled: true

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

## Download KoboldCpp

```bash
./tensors-router download --config config.yaml
```

Set `updates.binary_url` for the KoboldCpp build you want.

## Run

```bash
./tensors-router serve --config config.yaml
```

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

## Config Types

The router only reads enough `.kcpps` JSON to detect `sdmodel` and text model fields. KoboldCpp still receives the whole config, so Flux, SD3, WAN, Qwen Image, VAE, LoRA, CLIP/T5, GPU, and other KoboldCpp image settings stay in the `.kcpps`.

Usable config types depend on the KoboldCpp endpoint being called:

- Text config: use with `/v1/chat/completions` or `/v1/completions`; listed on `/v1/models`.
- Embeddings config: use with `/v1/embeddings` and include `model`.
- Image-only config: use with `/v1/images/...`, `/sdapi/v1/...`, or supported ComfyUI-style image routes; listed on `/sdapi/v1/sd-models` as `<kcpps name>-<sdmodel filename stem>`.
- Combined LLM+image config: use one `.kcpps` containing the text model plus `sdmodel`; its image id is available only while the combined config is loaded, and selecting it does not reload or unload the active LLM.

Only one KoboldCpp config is active at a time. LLM and image requests share one model gate: requests using the active config can run together, while a config switch waits for in-flight requests to finish.

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

## Cluster Routing

Set one router to `cluster.role: "master"` and slave routers to `cluster.role: "slave"`. Use the same `cluster.token` on all nodes. Slaves register with `cluster.master_url`; masters also query configured `cluster.slave_urls` at startup and on the health interval.

The master hashes referenced model files and path-normalized `.kcpps` configs into `cluster.store_dir`. If master and slaves have the same public model id with identical hashes, clients see one normal model id. If a slave has the same id with different hashes, the master exposes an indexed id such as `model-2`. Requested models are rewritten to the selected slave local id while forwarding, then responses are rewritten back to the public id.

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
