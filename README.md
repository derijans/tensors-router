# tensors-router

Router for KoboldCpp `.kcpps` configs.

It runs a KoboldCpp process, exposes `.kcpps` filenames as `/v1/models`, reloads the matching config through KoboldCpp admin mode, then forwards the request to KoboldCpp.

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
kcpps/sdxl.kcpps -> model id sdxl
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

kobold:
  backend_url: "http://127.0.0.1:5001"
  binary_path: "./bin/koboldcpp"
  data_dir: "./data"
  no_model: true
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

List models:

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

Requests to `/v1/chat/completions` and `/v1/completions` require a `model` field. The model id must match a `.kcpps` filename stem.

Other `/v1/...` endpoints are forwarded too. If the JSON body has a `model` field, the matching `.kcpps` config is loaded first. If there is no `model` field, the request is forwarded to the currently loaded KoboldCpp config.

Examples:

```json
{"model":"embed","input":"hello"}
```

loads `embed.kcpps` before forwarding to `/v1/embeddings`.

```json
{"model":"sdxl","prompt":"cat"}
```

loads `sdxl.kcpps` before forwarding to `/v1/images/generations`.

Non-`/v1` KoboldCpp paths are not proxied.

## Config Types

The router does not validate `.kcpps` contents. It only maps filename to config reload.

Usable config types depend on the KoboldCpp endpoint being called:

- Text config: use with `/v1/chat/completions` or `/v1/completions`.
- Embeddings config: use with `/v1/embeddings` and include `model`.
- Stable Diffusion config: use with `/v1/images/...` and include `model`.
- Multimodal config: use one `.kcpps` containing the text model plus its multimodal settings.

Only one KoboldCpp config is active at a time.

## Auth

Optional bearer keys:

```yaml
auth:
  bearer_keys:
    - "change-me"
```

Requests must also match `server.allowed_cidrs`.

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
