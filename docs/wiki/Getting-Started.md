# Getting Started

## Install a release

Download the archive for the required operating system and architecture from [GitHub Releases](https://github.com/derijans/tensors-router/releases). Each release includes checksum files.

The main executables are:

- `tensors-router` for routing and backend process management
- `tensor-router-webui` for the optional management interface
- `tensor-router-downloader` for model downloads used by the management interface

Keep companion executables in the same directory when the WebUI or downloader should be discovered automatically.

## Build from source

Requirements:

- Go matching the version used by the build workflow
- Node.js and npm for the WebUI bundle

Build on the current platform:

```sh
cd webui
npm ci
npm run build
cd ..
go build -o tensors-router ./cmd/tensors-router
go build -o tensor-router-webui ./cmd/tensor-router-webui
go build -o tensor-router-downloader ./cmd/tensor-router-downloader
```

Build Linux binaries with the Makefile:

```sh
cd webui
npm ci
cd ..
make build-linux
```

Build Windows binaries:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1
```

Generated cross-platform binaries are written to `dist`.

## Configure the router

Copy the canonical example and create the required directories:

```sh
cp config.example.yaml config.yaml
mkdir -p kcpps bin data router-store
```

Put `.kcpps` files in the directory selected by `models.config_dir`. Each file name stem becomes its default public model ID.

Set the backend executable paths for the selected [backend mode](Backends). Keep managed backend listeners on loopback addresses.

## Run the router

```sh
./tensors-router serve --config config.yaml
```

Confirm model discovery:

```sh
curl http://127.0.0.1:8080/v1/models
```

Send a chat request:

```sh
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"model-id","messages":[{"role":"user","content":"hello"}],"stream":true}'
```

## Add the WebUI

Copy the WebUI example and start it:

```sh
cp webui.example.yaml webui.yaml
./tensor-router-webui --config webui.yaml
```

With an empty `router.url`, the WebUI starts the router executable beside it and stops that managed process when the WebUI exits. Set `router.url` when the router is already running.

The WebUI uses HTTPS and generates a local certificate when certificate files are not configured. See [WebUI](WebUI) for listener and authentication details.

## Next steps

- Choose a [run topology](Run-Topologies).
- Review [security settings](Security-and-Reverse-Proxy) before binding beyond loopback.
- Use [Deployment](Deployment) for containers, GPU access, and system services.
