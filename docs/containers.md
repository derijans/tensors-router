# Containers

The repository publishes independent node and WebUI variants. The node image contains only `tensors-router`. The WebUI image contains `tensor-router-webui` and its managed router.

Published release tags are:

- `ghcr.io/<owner>/<repository>/tensors-router-node:<version>`
- `ghcr.io/<owner>/<repository>/tensors-router-webui:<version>`

The `Publish container images` GitHub Actions workflow runs for published releases and can also be dispatched manually with an explicit version. It publishes the version tag and `latest`.

The examples use `trusted_lan` so they start without bundled credentials. Use this only on a trusted private network. For any exposed deployment, change both configurations to `secure` and provide independent inference, admin, WebUI, and cluster credentials.

## Layout

Both variants use these paths:

- `/config` for configuration and certificates
- `/models` for read-only model files
- `/bin` for mounted backend executables
- `/data` for configuration mutations, backend state, analytics, recipes, benchmarks, and caches

The process runs as UID and GID `10001`. Make the host data directory writable by that identity. Model and backend binary mounts remain read-only.

## Compose

Run the node-only variant:

```sh
docker compose -f deploy/compose.base.yaml --profile node up --build router-node
```

Run the managed WebUI variant:

```sh
docker compose -f deploy/compose.base.yaml --profile webui up --build router-webui
```

Add NVIDIA GPU access:

```sh
docker compose -f deploy/compose.base.yaml -f deploy/compose.nvidia.yaml --profile webui up router-webui
```

Add AMD GPU access:

```sh
docker compose -f deploy/compose.base.yaml -f deploy/compose.amd.yaml --profile webui up router-webui
```

The 16-minute stop grace period is longer than the sample router's 15-minute drain timeout.

The WebUI keeps its management origin on `8443` and backend UI content on `8444`. For NAT or a reverse proxy, set `server.backend_ui_public_url` in `deploy/config/webui.yaml` to the browser-visible HTTPS origin for port `8444`; never route both listeners through the same external origin.

## Podman

Build both targets:

```sh
podman build --target node -t localhost/tensors-router-node -f Containerfile .
podman build --target webui -t localhost/tensors-router-webui -f Containerfile .
```

Run the node-only variant:

```sh
podman run --rm --name tensors-router-node --stop-timeout 960 --read-only --cap-drop all --security-opt no-new-privileges --tmpfs /tmp:size=64m,mode=1777 -p 8080:8080 -v ./deploy/config/node.yaml:/config/config.yaml:ro -v ./deploy/models:/models:ro -v ./deploy/bin:/bin:ro -v ./deploy/data/node:/data localhost/tensors-router-node
```

Run the WebUI variant:

```sh
podman run --rm --name tensors-router-webui --stop-timeout 960 --read-only --cap-drop all --security-opt no-new-privileges --tmpfs /tmp:size=64m,mode=1777 -p 8080:8080 -p 8443:8443 -p 8444:8444 -v ./deploy/config/router-managed.yaml:/config/config.yaml:ro -v ./deploy/config/webui.yaml:/config/webui.yaml:ro -v ./deploy/config/certs:/config/certs:ro -v ./deploy/models:/models:ro -v ./deploy/bin:/bin:ro -v ./deploy/data/webui:/data localhost/tensors-router-webui
```

With [NVIDIA Container Toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/overview.html) and CDI, add `--device nvidia.com/gpu=all`. For [AMD ROCm container access](https://rocm.docs.amd.com/_/downloads/install-on-linux/en/docs-6.4.1/pdf/), add `--device /dev/kfd --device /dev/dri --group-add keep-groups`.

[Windows container GPU acceleration is DirectX-oriented](https://learn.microsoft.com/en-us/virtualization/windowscontainers/deploy-containers/gpu-acceleration), so CUDA/ROCm deployments remain native loopback/firewall installations or Linux VM deployments. The Linux container GPU fragments target NVIDIA Container Toolkit and ROCm device passthrough.
