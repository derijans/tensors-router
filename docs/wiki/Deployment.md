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
bash scripts/install-systemd-user.sh "$PWD" "$PWD/tensors-router" "$PWD/config.yaml"
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

The installer enables the service and writes it under the current user's systemd configuration.

## Container images

The repository publishes separate images:

- `ghcr.io/derijans/tensors-router/tensors-router-node:{tag}`
- `ghcr.io/derijans/tensors-router/tensors-router-webui:{tag}`

The node image contains the router and downloader. The WebUI image also contains the WebUI and can run a managed router.

Both images run as UID and GID `10001`. `/data` and `/models` must be writable by that identity when the downloader, cooking, or configuration management writes there. Configuration and backend executable mounts can remain read-only.

## Docker Compose

Run the node image:

```sh
docker compose -f deploy/compose.base.yaml --profile node up --build router-node
```

Run the managed WebUI image:

```sh
docker compose -f deploy/compose.base.yaml --profile webui up --build router-webui
```

Add NVIDIA access:

```sh
docker compose -f deploy/compose.base.yaml -f deploy/compose.nvidia.yaml --profile webui up router-webui
```

Add AMD access:

```sh
docker compose -f deploy/compose.base.yaml -f deploy/compose.amd.yaml --profile webui up router-webui
```

The sample configurations use `trusted_lan` so they can start without bundled credentials. Restrict them to a trusted private network or replace that profile and configure credentials before using an exposed bind.

The sample stop grace period exceeds the router drain timeout so active requests can finish before the container runtime terminates the process.

## Podman

Build both targets:

```sh
podman build --target node -t localhost/tensors-router-node -f Containerfile .
podman build --target webui -t localhost/tensors-router-webui -f Containerfile .
```

Run the node image:

```sh
podman run --rm --name tensors-router-node --stop-timeout 960 --read-only --cap-drop all --security-opt no-new-privileges --tmpfs /tmp:size=64m,mode=1777 -p 8080:8080 -v ./deploy/config/node.yaml:/config/config.yaml:ro -v ./deploy/config/downloader.yaml:/config/downloader.yaml:ro -v ./deploy/models:/models -v ./deploy/bin:/bin:ro -v ./deploy/data/node:/data localhost/tensors-router-node
```

Run the WebUI image:

```sh
podman run --rm --name tensors-router-webui --stop-timeout 960 --read-only --cap-drop all --security-opt no-new-privileges --tmpfs /tmp:size=64m,mode=1777 -p 8080:8080 -p 8443:8443 -p 8444:8444 -v ./deploy/config/router-managed.yaml:/config/config.yaml:ro -v ./deploy/config/downloader.yaml:/config/downloader.yaml:ro -v ./deploy/config/webui.yaml:/config/webui.yaml:ro -v ./deploy/config/certs:/config/certs:ro -v ./deploy/models:/models -v ./deploy/bin:/bin:ro -v ./deploy/data/webui:/data localhost/tensors-router-webui
```

Use the vendor's container runtime instructions for GPU device access. The repository Compose overlays show the required device declarations for supported Linux hosts.

## Local Linux Compose files

`deploy/linux-local` contains separate router, WebUI, NVIDIA, and AMD Compose fragments for an existing host directory layout.

Update their host paths for the deployment machine. Paths written into `config.yaml` and `webui.yaml` must resolve inside the container, not on the host.

Ensure router, WebUI, and backend binaries are executable and make writable state directories owned by UID and GID `10001`.

## Listener mapping

The WebUI container exposes:

- router API on `8080`
- management HTTPS on `8443`
- backend UI HTTPS on `8444`

When NAT or proxying changes the visible backend UI origin, set `server.backend_ui_public_url` to the browser-visible HTTPS origin for the backend UI listener.
