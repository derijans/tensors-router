[CmdletBinding()]
param(
    [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9._-]*$')]
    [string]$Model = 'gemma-4-E2B-it-low',
    [ValidateRange(1, 65535)]
    [int]$RouterPort = 18080,
    [ValidateRange(1, 65535)]
    [int]$BackendPort = 15001,
    [string]$RouterPath,
    [switch]$KeepRuntime
)

$ErrorActionPreference = 'Stop'

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
                return $response
            }
        }
        catch {
        }
        Start-Sleep -Milliseconds 500
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "Timed out waiting for $Uri."
}

function Get-KoboldProcessIds {
    param([string]$BinaryPath)

    @(Get-Process -Name 'koboldcpp-nocuda' -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $BinaryPath } | Select-Object -ExpandProperty Id)
}

$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
if ([string]::IsNullOrWhiteSpace($RouterPath)) {
    $RouterPath = Join-Path $repositoryRoot 'dist\tensors-router-windows-amd64.exe'
}
$koboldPath = Join-Path $repositoryRoot 'bin\koboldcpp-nocuda.exe'
$configDirectory = Join-Path $repositoryRoot '.kcpps'
$routerPath = [System.IO.Path]::GetFullPath($RouterPath)
$modelConfigPath = Join-Path $configDirectory "$Model.kcpps"

foreach ($requiredPath in @($koboldPath, $configDirectory, $routerPath, $modelConfigPath)) {
    if (-not (Test-Path -LiteralPath $requiredPath)) {
        throw "Required test asset was not found: $requiredPath"
    }
}

$modelConfig = Get-Content -LiteralPath $modelConfigPath -Raw | ConvertFrom-Json
$modelPath = [string]$modelConfig.model_param
if ([string]::IsNullOrWhiteSpace($modelPath) -or -not (Test-Path -LiteralPath $modelPath -PathType Leaf)) {
    throw "The model configured by $modelConfigPath was not found: $modelPath"
}

Assert-PortAvailable -Port $RouterPort -ServiceName 'Router'
Assert-PortAvailable -Port $BackendPort -ServiceName 'KoboldCpp'
$koboldProcessIdsBefore = Get-KoboldProcessIds -BinaryPath $koboldPath

$runtimePath = Join-Path $repositoryRoot ("data-smoke\koboldcpp-test-" + [guid]::NewGuid().ToString('N'))
$runtimeConfigPath = Join-Path $runtimePath 'config.yaml'
$routerProcess = $null
try {
New-Item -ItemType Directory -Force -Path $runtimePath | Out-Null

$yaml = @"
security:
  profile: "trusted_lan"
server:
  bind: "127.0.0.1:$RouterPort"
  allowed_cidrs:
    - "127.0.0.0/8"
    - "::1/128"
auth:
  inference_keys: []
  admin_keys: []
models:
  config_dir: '$configDirectory'
backend:
  mode: "kobold"
kobold:
  backend_url: "http://127.0.0.1:$BackendPort"
  binary_path: '$koboldPath'
  data_dir: '$runtimePath'
  multiuser: 1
  quiet: true
  skip_launcher: true
  no_model: true
  hide_window: true
  extra_args: []
logging:
  mode: "startup_only"
  backend_logs_to_disk: true
updates:
  enabled: false
downloader:
  enabled: false
analytics:
  enabled: false
"@
$utf8WithoutBom = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($runtimeConfigPath, $yaml, $utf8WithoutBom)

    $routerConfigArgument = '"{0}"' -f $runtimeConfigPath
    $routerProcess = Start-Process -FilePath $routerPath -ArgumentList @('serve', '--config', $routerConfigArgument) -PassThru -NoNewWindow
    $modelsResponse = Wait-HTTPStatus -Uri "http://127.0.0.1:$RouterPort/v1/models" -Timeout ([TimeSpan]::FromSeconds(90))
    $models = $modelsResponse.Content | ConvertFrom-Json
    if (-not ($models.data.id -contains $Model)) {
        throw "Router catalog does not contain model $Model."
    }

    $request = @{ model = $Model; messages = @(@{ role = 'user'; content = 'Reply with the single word ready.' }); temperature = 0; max_tokens = 4 } | ConvertTo-Json -Depth 5
    $response = Invoke-RestMethod -Method Post -Uri "http://127.0.0.1:$RouterPort/v1/chat/completions" -ContentType 'application/json' -Body $request -TimeoutSec 600
    if ($null -eq $response.choices -or $response.choices.Count -lt 1 -or [string]::IsNullOrWhiteSpace([string]$response.choices[0].message.content)) {
        throw 'Router returned no chat completion content.'
    }

    [pscustomobject]@{
        Model = $Model
        Router = "http://127.0.0.1:$RouterPort"
        Backend = "http://127.0.0.1:$BackendPort"
        Reply = $response.choices[0].message.content
    } | Format-List
}
finally {
    if ($null -ne $routerProcess -and -not $routerProcess.HasExited) {
        try {
            Invoke-WebRequest -Method Post -Uri "http://127.0.0.1:$RouterPort/router/v1/shutdown" -UseBasicParsing -TimeoutSec 3 | Out-Null
        }
        catch {
        }
        if (-not $routerProcess.WaitForExit(35000)) {
            & taskkill.exe /PID $routerProcess.Id /T /F | Out-Null
            $routerProcess.WaitForExit()
        }
    }
    $koboldProcessIdsAfter = Get-KoboldProcessIds -BinaryPath $koboldPath
    foreach ($processId in $koboldProcessIdsAfter) {
        if ($processId -notin $koboldProcessIdsBefore) {
            & taskkill.exe /PID $processId /T /F | Out-Null
        }
    }
    if (-not $KeepRuntime -and (Test-Path -LiteralPath $runtimePath)) {
        try {
            Remove-Item -LiteralPath $runtimePath -Recurse -Force
        }
        catch {
            Write-Warning "Test runtime could not be removed: $runtimePath"
        }
    }
}
