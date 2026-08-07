# Deployment

## Release binaries

[GitHub Releases](https://github.com/derijans/tensors-router/releases) contain router, WebUI, and downloader binaries for the published operating systems and architectures. Archives and individual binaries include checksum files.

Keep the three companion binaries together when automatic discovery is required.

## Native process layout

The router needs:

- `config.yaml`
- an existing `.kcpps` configuration directory
- backend executables selected by the configuration
- writable data and cluster store directories

The WebUI needs `webui.yaml`, a writable state directory, and either an external router URL or access to the router executable and configuration.

Keep all managed backend listeners on loopback addresses.

## systemd user service

Install the user service:

```sh
bash scripts/install-systemd-user.sh "$PWD" "$PWD/tensors-router" "$PWD/config.yaml" 16m30s
systemctl --user start tensors-router.service
```

Inspect it with:

```sh
systemctl --user status tensors-router.service
journalctl --user -u tensors-router.service
```

Remove it with:

```sh
bash scripts/uninstall-systemd-user.sh
```

The installer enables the service and writes it under the current user's systemd configuration. Its optional fourth argument is a strict positive `h`/`m`/`s` duration. The default `16m30s` stop deadline exceeds the router's 15-minute drain timeout.

## Container images

The repository publishes separate images:

- `ghcr.io/derijans/tensors-router/tensors-router-node:{tag}`
- `ghcr.io/derijans/tensors-router/tensors-router-webui:{tag}`

The node image contains the router and downloader. The WebUI image also contains the WebUI and can run a managed router. Both images run as UID and GID `10001`.

## Docker storage layout

Set two absolute host paths:

| Variable | Purpose | Container path |
| --- | --- | --- |
| `TENSORS_ROUTER_ROOT` | Holds `config`, `kcpps`, `logs`, and `backends` | Multiple mounts |
| `TENSORS_ROUTER_MODELS_PATH` | Holds model files independently of the router root | `/models` |

`TENSORS_ROUTER_ROOT` has this layout:

| Host directory | Container path | Access |
| --- | --- | --- |
| `config` | `/config` | Read-only, except the nested downloader log mount |
| `kcpps` | `/kcpps` | Read-write |
| `logs` | `/logs` | Read-write |
| `logs/downloader` | `/config/data` | Read-write |
| `backends` | `/backends` | Read-write |

Each stack creates a named `/data` volume. It contains downloader state and SQLite data, analytics, WebUI certificates and state, cluster registry, recipes, benchmarks, and model asset indexes. Back up this volume separately from the host folders.

Prepare a WebUI host layout before deployment:

```sh
export TENSORS_ROUTER_ROOT=/srv/tensors-router
export TENSORS_ROUTER_MODELS_PATH=/srv/tensors-models
install -d -m 0750 "$TENSORS_ROUTER_ROOT"/{config,kcpps,logs/downloader,backends} "$TENSORS_ROUTER_MODELS_PATH"
bash scripts/bootstrap-secrets.sh "$TENSORS_ROUTER_ROOT"
cp deploy/config/router-managed.yaml "$TENSORS_ROUTER_ROOT/config/"
cp deploy/config/webui.yaml "$TENSORS_ROUTER_ROOT/config/"
cp deploy/config/downloader.yaml "$TENSORS_ROUTER_ROOT/config/"
chown -R 10001:10001 "$TENSORS_ROUTER_ROOT/kcpps" "$TENSORS_ROUTER_ROOT/logs" "$TENSORS_ROUTER_ROOT/backends" "$TENSORS_ROUTER_MODELS_PATH"
```

For a node deployment, copy `deploy/config/node.yaml` instead of `router-managed.yaml`. Place backend installations below `backends/kobold`, `backends/llama`, `backends/sdcpp`, and `backends/whisper`; keep each executable name used by the supplied configuration.

The backend directory is intentionally writable so router-managed backend updates can replace binaries. Restrict its host permissions to the deployment administrator and container UID.

## Docker Compose

Run a CPU node:

```sh
docker compose -f deploy/compose.base.yaml --profile node up -d router-node
```

Run the managed WebUI:

```sh
docker compose -f deploy/compose.base.yaml --profile webui up -d router-webui
```

Add NVIDIA access:

```sh
docker compose -f deploy/compose.base.yaml -f deploy/compose.nvidia.yaml --profile webui up -d router-webui
```

Add AMD access:

```sh
docker compose -f deploy/compose.base.yaml -f deploy/compose.amd.yaml --profile webui up -d router-webui
```

The supplied configurations use `secure`. The bootstrap helper creates separate 256-bit inference, router-admin, cluster, and WebUI-admin credentials below `secrets/`, with a `0700` directory and `0600` files. Compose mounts them through `/run/secrets`; it never places credential values in the stack environment.

Direct environment values remain available for Portainer and other orchestrators. Set exactly one value or file variable for each role; value/file pairs are mutually exclusive, and values cannot be reused across security roles.

`trusted_lan` disables these authentication requirements and is intentionally an explicit opt-in. Use it only on an isolated network:

```sh
docker compose -f deploy/compose.base.yaml -f deploy/compose.trusted-lan.yaml --profile node up -d router-node
```

## Portainer

Use the application-template catalog at:

```text
https://raw.githubusercontent.com/derijans/tensors-router/main/deploy/portainer/templates.json
```

Portainer reads custom app-template catalogs from a reachable URL. Configure that URL in Portainer's application-template settings, then select one of the six entries:

- WebUI or Node
- CPU, NVIDIA, or AMD

Each template prompts for `TENSORS_ROUTER_ROOT`, `TENSORS_ROUTER_MODELS_PATH`, and distinct inference, router-admin, and cluster credentials. WebUI templates also require a separate WebUI-admin credential. Portainer passes these direct values to the container; do not also configure the corresponding `_FILE` variables. Use absolute host paths. The six standalone stack files are also available under `deploy/portainer`; deploy one directly from Git when a custom app-template catalog is not wanted.

## Logs and runtime state

When `logging.backend_logs_to_disk` is enabled, backend output is appended to the host log tree:

- `/logs/kobold/koboldcpp.log`
- `/logs/llama/llama-server.log`
- `/logs/sdcpp/sd-server.log`
- `/logs/whisper/whisper-server.log`
- `/config/data/downloader.log`, mapped to `logs/downloader/downloader.log` on the host

Router and WebUI process logs use stdout and stderr, so inspect them through `docker compose logs` or Portainer's container-log view.

The sample stop grace period exceeds the router drain timeout so active requests can finish before the container runtime terminates the process.

## Podman

Build both targets:

```sh
podman build --target node -t localhost/tensors-router-node -f Containerfile .
podman build --target webui -t localhost/tensors-router-webui -f Containerfile .
```

Use the same paths, mounts, named `/data` volume, and device configuration as the Compose definitions. The published Compose files are the canonical mount contract.

## Listener mapping

The WebUI container exposes:

- management HTTPS on `8443`
- backend UI HTTPS on `8444`

Its managed router binds to loopback inside the container and port `8080` is not published. The node stacks publish router inference and administration on `8080`, protected by their separate credentials.

When NAT or proxying changes the visible backend UI origin, set `server.backend_ui_public_url` to the browser-visible HTTPS origin for the backend UI listener.
