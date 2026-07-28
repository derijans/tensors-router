# Windows one-machine cluster testing

Use the local launcher to run a master router and one or more slave routers on the same Windows machine. Each router manages its own `koboldcpp-nocuda.exe` process. The master also starts the WebUI; slaves run without it.

## Prerequisites

- `bin\koboldcpp-nocuda.exe`
- At least one `.kcpps` file in `.kcpps`
- Each `.kcpps` must reference an available model file
- Windows AMD64 executables in `dist`

Build the executables when they are not already present:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build-windows.ps1
```

## Start a master

Start the master first. It listens for model API requests on port `18080`, runs KoboldCpp on `15001`, and serves the management WebUI on `18443`.

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-koboldcpp-router.ps1 `
  -NodeId master `
  -Role master `
  -RouterPort 18080 `
  -BackendPort 15001 `
  -WebUIPort 18443 `
  -BackendUIPort 18444 `
  -ClusterToken local-cluster-token `
  -IncludeDownloader
```

Open `https://127.0.0.1:18443` and accept the generated local certificate. The master WebUI connects to the router already started by the launcher, so it does not create a second router process.

`-IncludeDownloader` generates `downloader.yaml` under the master runtime directory, enables downloader routes, and makes the downloader panel available in the master WebUI. The downloader is managed by the router; it is not a persistent fourth server process.

## Start a slave

Use different router and backend ports for every slave. The cluster token must exactly match the master token.

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-koboldcpp-router.ps1 `
  -NodeId slave-1 `
  -Role slave `
  -RouterPort 18081 `
  -BackendPort 15002 `
  -ClusterToken local-cluster-token `
  -MasterURL http://127.0.0.1:18080
```

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\start-koboldcpp-router.ps1 `
  -NodeId slave-2 `
  -Role slave `
  -RouterPort 18082 `
  -BackendPort 15003 `
  -ClusterToken local-cluster-token `
  -MasterURL http://127.0.0.1:18080
```

Slaves register with the master themselves. `-SlaveURL` is optional on the master; use it when the master should also poll a known slave URL at startup and every health interval.

## Test the cluster

Wait a few seconds for the slave registration loop, then query the master:

```powershell
Invoke-RestMethod http://127.0.0.1:18080/v1/models
```

Use the master endpoint for manual OpenAI-compatible requests. For example:

```powershell
$request = @{
  model = 'gemma-4-E2B-it-low'
  messages = @(@{ role = 'user'; content = 'Reply with ready.' })
  max_tokens = 8
} | ConvertTo-Json -Depth 5

Invoke-RestMethod -Method Post `
  -Uri http://127.0.0.1:18080/v1/chat/completions `
  -ContentType application/json `
  -Body $request
```

## Runtime files and shutdown

Every node keeps its generated files under `data-manual\<node-id>`:

- `router.yaml` and `router.pid`
- `webui.yaml` and `webui.pid` for the master
- `koboldcpp.log`
- `router-store`
- `downloader.yaml`, models, and downloader state when enabled

The launcher prints a router shutdown command. Run it for each node:

```powershell
Invoke-WebRequest -Method Post http://127.0.0.1:18080/router/v1/shutdown
```

For the master, also stop its WebUI process using the `StopWebUI` command printed by the launcher:

```powershell
Stop-Process -Id (Get-Content .\data-manual\master\webui.pid)
```

Use `-Wait` when the launcher's terminal should remain attached to the router process. Without `-Wait`, nodes stay running after the launcher returns, which makes it convenient to start several nodes from one terminal.
