# Architecture

This page maps the code to its responsibilities. For operating the router, start with the [wiki](wiki/Home.md).

## Binaries (`cmd/`)

| Binary | Role |
|---|---|
| `tensors-router` | The router process. Also runs the `download` and `benchmark` subcommands. |
| `tensor-router-webui` | Management UI server. Serves the Vite bundle embedded from `internal/webui/assets`, terminates TLS, and can supervise the router process. |
| `tensor-router-downloader` | Standalone model downloader. |
| `tensor-router-vllm` | Resident vLLM companion. The router talks to it over a line protocol (`internal/vllm/protocol.go`). Built only for linux/amd64, linux/arm64, and darwin/arm64. |
| `tensor-router-tuf-*` | TUF tooling that publishes, validates, and rotates signed release and vLLM runtime metadata. |

The WebUI binary embeds the built frontend, so run `cd webui && npm ci && npm run build` before building Go.

## Request path

Every request enters `proxy.Service.ServeHTTP` (`internal/proxy/dispatch.go`). There is no `http.ServeMux`. Ollama paths get an error writer that answers in Ollama's error shape. The order is:

1. vLLM realtime and response operations, then the Ollama method check.
2. Transport admission: reserve working memory for inference bodies and pick the path (see below), then the control-body limit.
3. `/router/mcp`, then `/router/v1/*` admin and cluster endpoints through the route table (`internal/proxy/routes.go`). Unmatched paths answer 404, and `/router/v1/node/*` routes require the cluster token.
4. The site WebUI proxy, a 404 for `/api/admin/`, Ollama paths, `/v1/models`, `/ping`, and `/sdapi/v1/*`.
5. Capability lanes: voice and music, then ComfyUI paths (`/prompt`, `/history/`, `/view`, `/upload/image`), then image, then text.

### Buffered and streaming bodies

`internal/transportbody` decides how an inference body travels.

- A body up to the replay buffer (64 MiB by default) is buffered and replayable. It takes the buffered path, which can retry, reload a failed backend, wait for a backend that is still loading, and route across the cluster.
- A larger body streams. Except on embeddings paths, it needs a model selector outside the body (`?model=`, `?sd_model_checkpoint=`, or `X-Tensors-Model`), it is rewritten on the fly by the streaming JSON processor (`internal/transportbody/json.go`), and it retries only if no bytes reached the backend.

Both paths restore the public model ID in JSON, SSE, and NDJSON responses (`writeProxyResponse` and `writeModelProxyResponse` in `internal/proxy/proxy_response.go`), using the same JSON processor.

## Packages (`internal/`)

### Routing core

| Package | Responsibility |
|---|---|
| `proxy` | The HTTP service: dispatch, lanes, model load, switch and unload (`lane.go`), cluster forwarding, and the components below. |
| `transportbody` | Memory budget for inference bodies, replayable and streaming bodies, the streaming JSON and multipart rewriters. |
| `catalog` | Discovers `.kcpps` model configs and classifies their capabilities. The filename stem is the model ID. |
| `cluster` | Node registry, per-lane route acquisition, and the authenticated client for the `/router/v1/node/*` peer API. |
| `backendmode` | The backend families: `kobold`, `llama_sdcpp`, `vllm`. |
| `unloadpolicy` | How a config's `router_unload_policy` decides between reuse, reload, and restart. |
| `schedulingcost` | Fits load and token costs from history to price queues and offload decisions. |
| `routinggroups` | Operator-declared links that let one node lend work to another. |
| `offloaddecisions` | The lending decision log: every plan, dispatch, and helper decision, kept in the router database. |
| `openai`, `ollama` | Wire formats and error shapes for the OpenAI and Ollama APIs. |

### `proxy` components

`proxy.Service` is the composition root. It owns dispatch, lifecycle, the lane core, and cluster, recipe, site, and node glue. Each feature below is a component that owns its own fields, locks, and background work. None of them holds `*Service`. The downloads, benchmark, asset, WebUI, and scheduler components reach the service through a small dependency interface. `requestAnalytics` gets only a load-section callback.

| Component | Where | Responsibility |
|---|---|---|
| Route table | `routes.go`, `proxy/routing` | Exact and prefix routes for `/router/v1/*`. The downloads, benchmark, asset, and WebUI components contribute their own routes. The analytics and offload endpoints are `Service` handlers. |
| `downloads.Handlers` | `proxy/downloads` | Model download endpoints and fan-out to cluster nodes (`proxy/clusterfan`). |
| `benchmarkRunner` | `benchmark_*.go` | Benchmark runs, records, and the model list decoration. |
| `assetManager` | `model_asset*.go` | Model file hashing, lookup cache, peer transfer, Hugging Face resolution, and background asset jobs. `Close` waits for those jobs. |
| `requestAnalytics` | `analytics.go`, `vram_analytics.go` | Request events, load hooks, and VRAM sampling (`analytics.VRAMWorkSampler`). |
| `webUIProxy` | `webui_*.go` | Backend WebUI proxying and its route snapshot. `Service.onRuntimeChanged` invalidates the snapshot when a runtime loads or unloads. |
| `scheduler` | `scheduler.go`, `offload_*.go`, `scheduling_costs.go`, `text_work.go` | Image and text queues, offload leases, lent-request slots, queue events the master decides on, the fitted cost table, the refresh loop, and borrow restore. `Close` stops the loop, the event reports, and the restore timers. |

Every unload claims the runtime through one protocol in `lane.go` (`claimIdleRuntime`). The unload policy, the node UI unload with its generation check, the disabled-model unload, and backend stop all use it.

### Backends

| Package | Responsibility |
|---|---|
| `kobold` | Launches and supervises KoboldCpp. |
| `native` | Launches llama.cpp, stable-diffusion.cpp, and whisper.cpp and maps `.kcpps` fields to their CLI arguments. |
| `vllm` | Drives the vLLM companion, its runtime installs, and its manifests. |
| `backendendpoint`, `portalloc` | Loopback backend addresses and router-allocated ports. |
| `backendreadiness`, `backenddiagnostic` | Read backend output to tell loading from failure, and attach the decisive log line to errors. |
| `processcontrol` | Start and stop child processes per platform. |
| `companion` | Find sibling executables next to the router binary. |
| `ffmpeg`, `comfyvideo` | Optional media conversion and ComfyUI video workflow support. |
| `hardware` | GPU detection and VRAM sampling. |

### State and storage

| Package | Responsibility |
|---|---|
| `routerstore` | The single SQLite file shared by analytics, load capture, load errors, and routing groups. Owns pragmas, permissions, and schema versions. |
| `analytics`, `loadcapture`, `loaderrors` | Request analytics, captured backend load output, and pre-load failure records. |
| `modelstate` | Per-node model enablement and separate-runtime settings. |
| `modelassets`, `inventory` | Model file hashing, peer transfer, Hugging Face resolution, and file inventory. |
| `recipes`, `cook` | Composite model recipes and config authoring. |
| `benchmark` | Benchmark runs and history. |
| `atomicfile` | Crash-safe file replacement. |

### Configuration, security, and delivery

| Package | Responsibility |
|---|---|
| `config` | Loads the router YAML, applies the `secure` or `trusted_lan` profile, and reports deprecations and credential warnings. |
| `auth` | CIDR allowlist and the inference, admin, and cluster credential classes per route. |
| `credential` | Credential file resolution, placeholder rejection, and strength warnings. |
| `webui` | The management UI server and its config. |
| `mcp` | Per-model MCP servers and the gateway behind `/router/mcp`. |
| `downloader` | Hugging Face search, plans, and resumable downloads. |
| `update`, `tufpublish` | TUF-verified self and backend updates, and the publishing side. |
| `siteapi`, `jsonpath`, `buildinfo` | Shared request types, JSON path helpers, and build metadata. |

## WebUI (`webui/`)

Vanilla TypeScript and Vite. Markup is built only with the `html` tagged template from `webui/src/safe-html.ts`, which escapes every interpolated value, and reaches the DOM only through `setHTML`. ESLint rejects direct `innerHTML`, `outerHTML`, and `insertAdjacentHTML`, and rejects markup in untagged templates or string literals.
