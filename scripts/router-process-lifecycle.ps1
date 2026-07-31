function Test-ProcessRunning {
    param([System.Diagnostics.Process]$Process)

    if ($null -eq $Process) {
        return $false
    }
    try {
        return -not $Process.HasExited
    }
    catch {
        return $false
    }
}

function Stop-ProcessTree {
    param([System.Diagnostics.Process]$Process)

    if (-not (Test-ProcessRunning -Process $Process)) {
        return
    }
    & taskkill.exe /PID $Process.Id /T /F | Out-Null
    $Process.WaitForExit()
}

function Stop-RouterProcess {
    param(
        [System.Diagnostics.Process]$Process,
        [string]$ShutdownURI
    )

    if (-not (Test-ProcessRunning -Process $Process)) {
        return
    }
    try {
        Invoke-WebRequest -Method Post -Uri $ShutdownURI -UseBasicParsing -TimeoutSec 3 | Out-Null
    }
    catch {
    }
    if (-not $Process.WaitForExit(35000)) {
        Stop-ProcessTree -Process $Process
    }
}

function Remove-OwnedPIDFile {
    param(
        [string]$Path,
        [System.Diagnostics.Process]$Process
    )

    if ($null -eq $Process -or -not (Test-Path -LiteralPath $Path)) {
        return
    }
    if ((Get-Content -LiteralPath $Path -Raw).Trim() -eq [string]$Process.Id) {
        Remove-Item -LiteralPath $Path -Force
    }
}

function Wait-LauncherProcesses {
    param(
        [System.Diagnostics.Process]$RouterProcess,
        [System.Diagnostics.Process]$WebUIProcess
    )

    while (Test-ProcessRunning -Process $RouterProcess) {
        if ($null -ne $WebUIProcess -and -not (Test-ProcessRunning -Process $WebUIProcess)) {
            return
        }
        Start-Sleep -Milliseconds 500
    }
}
