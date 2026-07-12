# Linux Compose deployment

The Compose files use the deployment directory at `/media/main/VM/cobold/router` and run its Linux AMD64 executables inside the published container images.

Prepare permissions:

```sh
cd /media/main/VM/cobold/router
chmod 755 tensors-router-linux-amd64 tensor-router-webui-linux-amd64
chmod 755 bin/koboldcpp bin/llama-server bin/sd-server
sudo chown -R 10001:10001 data router-store webui-state
```

Run `chmod 755` only for backend executable paths that actually exist in your `bin` or `kcpps` directories.

Copy the four `compose.*.yaml` files into `/media/main/VM/cobold/router`, or run them using their existing paths.

Run the WebUI with its managed router on NVIDIA:

```sh
docker compose -f compose.webui.yaml -f compose.nvidia.yaml up -d
```

Run the WebUI with its managed router on AMD:

```sh
docker compose -f compose.webui.yaml -f compose.amd.yaml up -d
```

Run only the router on NVIDIA:

```sh
docker compose -f compose.router.yaml -f compose.nvidia.yaml up -d
```

Run only the router on AMD:

```sh
docker compose -f compose.router.yaml -f compose.amd.yaml up -d
```

Run without GPU passthrough by omitting the second Compose file.

Inspect startup and shutdown:

```sh
docker compose -f compose.webui.yaml logs -f
docker compose -f compose.webui.yaml down
```

The paths in `webui.yaml` and `config.yaml` must resolve below `/opt/router` inside the container. Relative paths work because the container working directory is `/opt/router`. Host paths beginning with `/media/main/VM/cobold/router` must be changed to the corresponding `/opt/router` paths.
