[CmdletBinding()]
param(
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._-]*$')]
    [string]$NodeId = 'local',
    [ValidateSet('standalone', 'master', 'slave')]
    [string]$Role = 'standalone',
    [string]$NodeRoot,
    [string]$AssetRoot,
    [string]$SharedDir,
    [ValidateRange(1, 65535)]
    [int]$RouterPort = 18080,
    [ValidateRange(1, 65535)]
    [int]$BackendPort = 15001,
    [ValidateRange(1, 65535)]
    [int]$EmbeddingsBackendPort = 15004,
    [ValidateRange(1, 65535)]
    [int]$LlamaBackendPort = 15002,
    [ValidateRange(1, 65535)]
    [int]$LlamaEmbeddingsBackendPort = 15005,
    [ValidateRange(1, 65535)]
    [int]$WebUIPort = 18443,
    [ValidateRange(1, 65535)]
    [int]$BackendUIPort = 18444,
    [string]$BindAddress = '127.0.0.1',
    [string]$PublicURL,
    [string]$MasterURL,
    [string[]]$SlaveURL = @(),
    [string]$ClusterToken,
    [string]$RouterPath,
    [string]$WebUIPath,
    [string]$DownloaderPath,
    [string]$KoboldPath,
    [string]$LlamaPath,
    [string]$DownloaderStorageRoot,
    [string]$DownloaderStateDir,
    [switch]$NoPublicURL,
    [switch]$IncludeDownloader,
    [switch]$Wait,
    [switch]$Detach
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'router-process-lifecycle.ps1')

function Assert-PortAvailable {
    param([int]$Port, [string]$ServiceName)

    if (Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue) {
        throw "$ServiceName port $Port is already in use."
    }
}

function Wait-HTTPStatus {
    param([string]$Uri, [TimeSpan]$Timeout)

    $deadline = [DateTime]::UtcNow.Add($Timeout)
    do {
        try {
            $response = Invoke-WebRequest -Uri $Uri -UseBasicParsing -TimeoutSec 3
            if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 300) {
                return
            }
        }
        catch {
        }
        Start-Sleep -Milliseconds 500
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "Timed out waiting for $Uri."
}

function Write-UTF8File {
    param([string]$Path, [string]$Content)

    $encoding = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, $Content, $encoding)
}

function ConvertTo-YAMLScalar {
    param([string]$Value)

    "'" + $Value.Replace("'", "''") + "'"
}

function ConvertTo-YAMLList {
    param([string[]]$Values, [int]$Indent = 4)

    if ($Values.Count -eq 0) {
        return ' []'
    }
    $prefix = ' ' * $Indent
    "`n" + (($Values | ForEach-Object { "$prefix- $(ConvertTo-YAMLScalar $_)" }) -join "`n")
}

if ($Wait -and $Detach) {
    throw 'Wait and Detach cannot be used together.'
}

if ([string]::IsNullOrWhiteSpace($AssetRoot)) {
    $AssetRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
}
$assetRoot = [System.IO.Path]::GetFullPath($AssetRoot)
if ([string]::IsNullOrWhiteSpace($NodeRoot)) {
    $NodeRoot = Join-Path $assetRoot "data-manual\$NodeId"
}
$nodeRoot = [System.IO.Path]::GetFullPath($NodeRoot)
if ([string]::IsNullOrWhiteSpace($RouterPath)) {
    $RouterPath = Join-Path $assetRoot 'dist\tensors-router-windows-amd64.exe'
}
if ([string]::IsNullOrWhiteSpace($WebUIPath)) {
    $WebUIPath = Join-Path $assetRoot 'dist\tensor-router-webui-windows-amd64.exe'
}
if ([string]::IsNullOrWhiteSpace($DownloaderPath)) {
    $DownloaderPath = Join-Path $assetRoot 'dist\tensor-router-downloader-windows-amd64.exe'
}
if ([string]::IsNullOrWhiteSpace($KoboldPath)) {
    $KoboldPath = Join-Path $assetRoot 'bin\koboldcpp-nocuda.exe'
}
if ([string]::IsNullOrWhiteSpace($LlamaPath)) {
    $LlamaPath = Join-Path $assetRoot 'bin\llama-b10295-bin-win-cpu-x64\llama-server.exe'
}
if ($NoPublicURL) {
    if (-not [string]::IsNullOrWhiteSpace($PublicURL)) {
        throw 'NoPublicURL and PublicURL cannot be used together.'
    }
    if ($Role -eq 'slave') {
        throw 'NoPublicURL cannot be used for a slave because cluster.public_url is required there.'
    }
}
elseif ([string]::IsNullOrWhiteSpace($PublicURL)) {
    $PublicURL = "http://127.0.0.1:$RouterPort"
}
if ($Role -ne 'standalone' -and [string]::IsNullOrWhiteSpace($ClusterToken)) {
    throw 'ClusterToken is required for master and slave nodes.'
}
if ($Role -eq 'slave' -and [string]::IsNullOrWhiteSpace($MasterURL)) {
    throw 'MasterURL is required for a slave node.'
}

$routerPath = [System.IO.Path]::GetFullPath($RouterPath)
$webuiPath = [System.IO.Path]::GetFullPath($WebUIPath)
$downloaderPath = [System.IO.Path]::GetFullPath($DownloaderPath)
$koboldPath = [System.IO.Path]::GetFullPath($KoboldPath)
$llamaPath = [System.IO.Path]::GetFullPath($LlamaPath)
$configDirectory = Join-Path $nodeRoot '.kcpps'
foreach ($requiredPath in @($routerPath, $koboldPath, $llamaPath, $configDirectory)) {
    if (-not (Test-Path -LiteralPath $requiredPath)) {
        throw "Required router asset was not found: $requiredPath"
    }
}
if ($Role -eq 'master' -and -not (Test-Path -LiteralPath $webuiPath)) {
    throw "The master WebUI executable was not found: $webuiPath"
}
if ($IncludeDownloader -and -not (Test-Path -LiteralPath $downloaderPath)) {
    throw "The downloader executable was not found: $downloaderPath"
}
if (-not (Get-ChildItem -LiteralPath $configDirectory -Filter '*.kcpps' -File | Select-Object -First 1)) {
    throw "No .kcpps files were found in $configDirectory"
}

$runtimePath = Join-Path $nodeRoot 'data'
$configPath = Join-Path $runtimePath 'router.yaml'
$pidPath = Join-Path $runtimePath 'router.pid'
$webuiConfigPath = Join-Path $runtimePath 'webui.yaml'
$webuiPIDPath = Join-Path $runtimePath 'webui.pid'
foreach ($existingPIDPath in @($pidPath, $webuiPIDPath)) {
    if (Test-Path -LiteralPath $existingPIDPath) {
        $existingProcessId = Get-Content -LiteralPath $existingPIDPath -Raw
        if (Get-Process -Id $existingProcessId -ErrorAction SilentlyContinue) {
            throw "Node $NodeId is already running with process ID $existingProcessId."
        }
        Remove-Item -LiteralPath $existingPIDPath -Force
    }
}

$servicePorts = @($RouterPort, $BackendPort, $EmbeddingsBackendPort, $LlamaBackendPort, $LlamaEmbeddingsBackendPort)
if ($Role -eq 'master') {
    $servicePorts += @($WebUIPort, $BackendUIPort)
}
if (($servicePorts | Sort-Object -Unique).Count -ne $servicePorts.Count) {
    throw 'Router, KoboldCpp, llama.cpp, and active WebUI ports must be distinct.'
}
Assert-PortAvailable -Port $RouterPort -ServiceName 'Router'
Assert-PortAvailable -Port $BackendPort -ServiceName 'KoboldCpp'
Assert-PortAvailable -Port $EmbeddingsBackendPort -ServiceName 'KoboldCpp embeddings'
Assert-PortAvailable -Port $LlamaBackendPort -ServiceName 'llama.cpp'
Assert-PortAvailable -Port $LlamaEmbeddingsBackendPort -ServiceName 'llama.cpp embeddings'
if ($Role -eq 'master') {
    Assert-PortAvailable -Port $WebUIPort -ServiceName 'WebUI'
    Assert-PortAvailable -Port $BackendUIPort -ServiceName 'WebUI backend'
}
New-Item -ItemType Directory -Force -Path $runtimePath | Out-Null

$downloaderEnabled = $IncludeDownloader.ToString().ToLowerInvariant()
$downloaderBinaryLocation = ''
if ($IncludeDownloader) {
    $downloaderBinaryLocation = $downloaderPath
    $downloaderConfigPath = Join-Path $runtimePath 'downloader.yaml'
    if ([string]::IsNullOrWhiteSpace($DownloaderStorageRoot)) {
        $DownloaderStorageRoot = Join-Path $runtimePath 'models'
    }
    if ([string]::IsNullOrWhiteSpace($DownloaderStateDir)) {
        $DownloaderStateDir = Join-Path $runtimePath 'downloader-state'
    }
    $downloaderConfig = @"
storage:
  root: $(ConvertTo-YAMLScalar $DownloaderStorageRoot)
  state_dir: $(ConvertTo-YAMLScalar $DownloaderStateDir)
  database_path: $(ConvertTo-YAMLScalar (Join-Path $DownloaderStateDir 'downloads.sqlite'))
  free_space_reserve_gb: 0
downloads:
  concurrent_jobs: 2
  concurrent_files: 4
  retry_limit: 5
  timeout: "30s"
scanning:
  hash_workers: 1
  write_hash_sidecars: true
hardware:
  default_context: 8192
  vram_reserve_mb: 1024
  safety_margin_percent: 15
logging:
  mode: "normal"
"@
    Write-UTF8File -Path $downloaderConfigPath -Content $downloaderConfig
}

$yaml = @"
security:
  profile: "trusted_lan"
server:
  bind: "${BindAddress}:$RouterPort"
  allowed_cidrs:
    - "127.0.0.0/8"
    - "::1/128"
    - "10.0.0.0/8"
    - "172.16.0.0/12"
    - "192.168.0.0/16"
auth:
  inference_keys: []
  admin_keys: []
models:
  config_dir: $(ConvertTo-YAMLScalar $configDirectory)
  shared_dir: $(ConvertTo-YAMLScalar $SharedDir)
backend:
  mode: "kobold"
kobold:
  backend_url: "http://127.0.0.1:$BackendPort"
  embeddings_backend_url: "http://127.0.0.1:$EmbeddingsBackendPort"
  binary_path: $(ConvertTo-YAMLScalar $koboldPath)
  data_dir: $(ConvertTo-YAMLScalar $runtimePath)
  multiuser: 1
  quiet: true
  skip_launcher: true
  no_model: true
  hide_window: true
  extra_args: []
llama:
  backend_url: "http://127.0.0.1:$LlamaBackendPort"
  embeddings_backend_url: "http://127.0.0.1:$LlamaEmbeddingsBackendPort"
  binary_path: $(ConvertTo-YAMLScalar $llamaPath)
  data_dir: $(ConvertTo-YAMLScalar (Join-Path $runtimePath 'llama'))
  hide_window: true
  extra_args: []
logging:
  mode: "normal"
  backend_logs_to_disk: true
updates:
  enabled: false
downloader:
  enabled: $downloaderEnabled
  binary_location: $(ConvertTo-YAMLScalar $downloaderBinaryLocation)
analytics:
  enabled: false
cluster:
  role: "$Role"
  node_id: $(ConvertTo-YAMLScalar $NodeId)
  public_url: $(ConvertTo-YAMLScalar $PublicURL)
  master_url: $(ConvertTo-YAMLScalar $MasterURL)
  slave_urls:$(ConvertTo-YAMLList $SlaveURL)
  token: $(ConvertTo-YAMLScalar $ClusterToken)
  store_dir: $(ConvertTo-YAMLScalar (Join-Path $runtimePath 'router-store'))
  sync_interval: "5s"
  health_interval: "5s"
"@
Write-UTF8File -Path $configPath -Content $yaml

$routerProcess = $null
$webuiProcess = $null
$leaveProcessesRunning = $false
try {
    $routerConfigArgument = '"{0}"' -f $configPath
    $routerProcess = Start-Process -FilePath $routerPath -ArgumentList @('serve', '--config', $routerConfigArgument) -PassThru -NoNewWindow
    Write-UTF8File -Path $pidPath -Content $routerProcess.Id
    Wait-HTTPStatus -Uri "http://127.0.0.1:$RouterPort/router/v1/models" -Timeout ([TimeSpan]::FromSeconds(90))

    $webuiURL = ''
    if ($Role -eq 'master') {
        if ($IncludeDownloader) {
            $webuiDownloaderPath = Join-Path (Split-Path -Parent $webuiPath) 'tensor-router-downloader.exe'
            if (-not (Test-Path -LiteralPath $webuiDownloaderPath)) {
                Copy-Item -LiteralPath $downloaderPath -Destination $webuiDownloaderPath
            }
        }
        $webuiConfig = @"
security:
  profile: "trusted_lan"
server:
  bind: "${BindAddress}:$WebUIPort"
  backend_ui_bind: "${BindAddress}:$BackendUIPort"
  backend_ui_public_url: ""
  state_dir: $(ConvertTo-YAMLScalar (Join-Path $runtimePath 'webui-state'))
  cert_file: ""
  key_file: ""
  cert_hosts: []
  admin_token: ""
router:
  url: "http://127.0.0.1:$RouterPort"
  token: ""
  binary_path: $(ConvertTo-YAMLScalar $routerPath)
  config_path: $(ConvertTo-YAMLScalar $configPath)
  start_when_missing: false
  shutdown_with_webui: false
  args: []
logging:
  mode: "normal"
"@
        Write-UTF8File -Path $webuiConfigPath -Content $webuiConfig
        $webuiConfigArgument = '"{0}"' -f $webuiConfigPath
        $webuiProcess = Start-Process -FilePath $webuiPath -ArgumentList @('--config', $webuiConfigArgument) -PassThru -NoNewWindow
        Write-UTF8File -Path $webuiPIDPath -Content $webuiProcess.Id
        $webuiURL = "https://$BindAddress`:$WebUIPort"
    }

    [pscustomobject]@{
        Node = $NodeId
        Role = $Role
        Router = "http://$BindAddress`:$RouterPort"
        Backend = "http://127.0.0.1:$BackendPort"
        EmbeddingsBackend = "http://127.0.0.1:$EmbeddingsBackendPort"
        LlamaBackend = "http://127.0.0.1:$LlamaBackendPort"
        LlamaEmbeddingsBackend = "http://127.0.0.1:$LlamaEmbeddingsBackendPort"
        ProcessId = $routerProcess.Id
        Config = $configPath
        WebUI = $webuiURL
        WebUIProcessId = if ($null -eq $webuiProcess) { '' } else { $webuiProcess.Id }
        Downloader = $IncludeDownloader.IsPresent
        Mode = if ($Detach) { 'detached' } else { 'attached' }
        Stop = "Invoke-WebRequest -Method Post http://$BindAddress`:$RouterPort/router/v1/shutdown"
        StopWebUI = if ($null -eq $webuiProcess) { '' } else { "Stop-Process -Id $($webuiProcess.Id)" }
    } | Format-List

    if ($Detach) {
        $leaveProcessesRunning = $true
        return
    }
    Wait-LauncherProcesses -RouterProcess $routerProcess -WebUIProcess $webuiProcess
}
finally {
    if (-not $leaveProcessesRunning) {
        Stop-ProcessTree -Process $webuiProcess
        Stop-RouterProcess -Process $routerProcess -ShutdownURI "http://127.0.0.1:$RouterPort/router/v1/shutdown"
        Remove-OwnedPIDFile -Path $webuiPIDPath -Process $webuiProcess
        Remove-OwnedPIDFile -Path $pidPath -Process $routerProcess
    }
}
