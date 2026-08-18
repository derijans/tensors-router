# Testing and Troubleshooting

## Project checks

Run the Go test suite:

```sh
go test ./...
```

Run the WebUI checks:

```sh
cd webui
npm ci
npm run check
```

The WebUI check runs the package audit, lint, tests, and production build.

Release CI also runs race tests, `govulncheck`, container manifest checks, and vulnerability scans for every Alpine and glibc image on amd64 and arm64. Archive upload and multi-architecture image publication depend on those scan jobs. A signed vLLM hardware profile has additional platform-gated checks: exact dependency installation, import and serving smoke tests, Python dependency audit, runtime vulnerability scan, and an OCI container scan where applicable on the matching platform or vendor runner. A failed, stale, partial, or unavailable hardware gate prevents profile publication.

## vLLM initialization fails

Inspect the selected node's backend lifecycle and initialization job. `companion_missing` means the supported archive's `tensor-router-vllm` executable is not beside the router or at `vllm.binary_location`. `unsupported` means the host platform has no companion release. `failed` includes a sanitized prerequisite or verification failure and permits retry when appropriate.

An accelerator detected without its required host driver, SDK, device permission, compiler, container engine, or privileged OS package does not fall back to CPU. Satisfy the reported prerequisite and retry initialization. The companion never installs host prerequisites. On Windows, run the Linux suite in WSL 2; native Windows is unsupported.

Initialization is never automatic. Start it from the selected Nodes panel or the administrator API. Model load remains offline and returns `backend_not_initialized` until a profile is ready.

## Local KoboldCpp smoke test on Windows

Requirements:

- `bin\koboldcpp-nocuda.exe`
- at least one `.kcpps` configuration in `.kcpps`
- the model files referenced by that configuration

Build and run one request:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1
powershell -ExecutionPolicy Bypass -File .\scripts\test-koboldcpp.ps1 -Model model-id
```

The script validates the selected configuration, creates an isolated runtime under `data-smoke`, starts the router and CPU-only KoboldCpp, checks `/v1/models`, sends one chat request, and stops both processes.

Use `-KeepRuntime` to retain the generated configuration and backend log.

## Manual Windows standalone node

Start one standalone node:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-koboldcpp-router.ps1 -NodeId local -RouterPort 18080 -BackendPort 15001 -EmbeddingsBackendPort 15004
Invoke-RestMethod http://127.0.0.1:18080/v1/models
```

The launcher prints the matching shutdown commands. Use `-Wait` to keep its terminal attached.

## One-machine Windows cluster

The local launcher can run a master and multiple slaves on one Windows machine. Every node gets a separate router port, backend port, PID, log, and store under `data-manual\{node-id}`.

Start the master:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-koboldcpp-router.ps1 `
  -NodeId master `
  -Role master `
  -RouterPort 18080 `
  -BackendPort 15001 `
  -EmbeddingsBackendPort 15004 `
  -WebUIPort 18443 `
  -BackendUIPort 18444 `
  -ClusterToken local-cluster-token `
  -IncludeDownloader
```

Start a slave with different ports:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-koboldcpp-router.ps1 `
  -NodeId slave-1 `
  -Role slave `
  -RouterPort 18081 `
  -BackendPort 15002 `
  -EmbeddingsBackendPort 15005 `
  -ClusterToken local-cluster-token `
  -MasterURL http://127.0.0.1:18080
```

Verify the master registry:

```powershell
Invoke-RestMethod http://127.0.0.1:18080/router/v1/models
```

Confirm that the slave appears in the returned model records and reports an available node before sending inference through the master. The launcher prints shutdown commands and supports `-Wait` when the terminal should remain attached.

Use `-DownloaderStorageRoot` with `-IncludeDownloader` to test a mounted or remote destination. `-DownloaderStateDir` can keep temporary staging local; relative values resolve from the generated node configuration directory.

## Basic API checks

List text models:

```sh
curl http://127.0.0.1:8080/v1/models
```

List image models:

```sh
curl http://127.0.0.1:8080/sdapi/v1/sd-models
```

Inspect the full registry:

```sh
curl http://127.0.0.1:8080/router/v1/models
```

For a cluster, send these requests to the master and confirm remote model records report an available node.

## WebUI reports `Scheme missing.`

`router.url` is not a complete URL. Use an HTTP or HTTPS origin, such as:

```yaml
router:
  url: "http://127.0.0.1:8080"
```

Leave `router.url` empty when the WebUI should manage the router process.

## WebUI certificate warning

The generated certificate is self-signed. Accept it for local testing or configure trusted certificate and key files.

If the browser-facing name or address is missing from the certificate, add it to `server.cert_hosts` and regenerate the local certificate state.

## Backend does not start

Check:

- the configured executable path
- execute permission on Linux
- archive extraction layout relative to `binary_path`
- backend log files when `logging.backend_logs_to_disk` is enabled
- whether an `extra_args` value conflicts with managed host or port settings
- whether the error names a backend port as already in use — the router now probes the port immediately before spawning and fails fast with that message, instead of the process silently failing to bind and the router waiting out the full health-check timeout

Kobold mode starts its process during router startup. Split backend processes start only after a matching model is selected.

During a model load, the router keeps the final 256 KiB of combined backend stdout and stderr in memory. If the backend exits or never becomes ready, `tensor-router-webui` writes this sanitized diagnostic to its stderr, including the node, backend, and exit status. The diagnostic is forwarded through cluster nodes for that load only, then discarded after a successful load; it is never returned to the browser. Disk logging remains optional and, when enabled, receives the same backend output.

## Model is missing

Check that:

- `models.config_dir` exists
- the file has a `.kcpps` extension
- the file name stem matches the requested model ID
- the configuration has fields for the requested capability
- referenced assets are available on the selected node

Open the Models tab or request a full inventory when file hashes and asset resolution need to be refreshed.

## Streaming stalls behind a proxy

Disable proxy response buffering and increase read and send timeouts. Streaming uses server-sent events rather than WebSockets.

## Cluster node is unavailable

Check the shared cluster token, the slave's `public_url`, the master's `slave_urls`, and network access in both directions.

The master rejects registration from node URLs that are not configured. Lane-specific backend health also determines whether a model can receive a request.
