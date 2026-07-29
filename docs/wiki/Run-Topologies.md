# Run Topologies

## Headless standalone

Run `tensors-router serve` directly. Clients connect to the router listener and no management WebUI is required.

Use this topology when configuration files are managed outside the WebUI or when only API access is needed.

## WebUI-managed standalone

Run `tensor-router-webui` with an empty `router.url`. The WebUI locates `tensors-router` beside its own executable, starts it with `router.config_path`, and stops the managed router when the WebUI exits.

The management interface and backend interfaces use separate HTTPS listeners. Do not configure `server.bind` and `server.backend_ui_bind` to the same address.

## External router with WebUI

Run both processes separately and set `router.url` in `webui.yaml`. The WebUI connects to that router but does not expose router launch, restart, or kill controls.

Use this topology when a process supervisor manages the router or when the WebUI and router use different hosts.

## Cluster master with local inference

Set `cluster.role: "master"` and configure one or more slaves. The master advertises local and remote models, selects an available node for each request, and provides the management API.

Local requests use the master's configured backend. Remote requests are forwarded to the selected slave with cluster authentication.

## Routing-only master

A master can route all inference to slaves without loading local models.

Configure the master with:

- `cluster.role: "master"`
- an existing but empty `models.config_dir`
- an empty `models.startup_model`
- `backend.mode: "llama_sdcpp"`
- no local recipes that select master-hosted components

The split backend managers are lazy. With no local models selected, the master does not start `llama-server` or `sd-server`. It still needs valid loopback backend URLs because those values are validated at startup.

Configure slaves with the same `cluster.token`, unique node IDs, reachable public URLs, and their local model configurations. See [Cluster Routing](Cluster-Routing) for registration and model identity rules.

## Cluster worker

Set `cluster.role: "slave"` and `cluster.master_url` on each worker. A slave advertises its local models to the master and accepts authenticated worker requests under `/router/v1/node/...`.

Slaves do not expose browser-facing management routes.
