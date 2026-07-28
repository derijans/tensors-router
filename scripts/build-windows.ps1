[CmdletBinding()]
param(
    [ValidateSet('amd64', 'arm64')]
    [string]$Architecture = 'amd64',
    [string]$OutputDirectory
)

$ErrorActionPreference = 'Stop'

$repositoryRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    $OutputDirectory = Join-Path $repositoryRoot 'dist'
}
$outputDirectory = [System.IO.Path]::GetFullPath($OutputDirectory)
$buildDate = [DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
$version = (git -C $repositoryRoot describe --tags --always --dirty 2>$null)
if ([string]::IsNullOrWhiteSpace($version)) {
    $version = 'dev'
}
$commit = (git -C $repositoryRoot rev-parse HEAD 2>$null)
if ([string]::IsNullOrWhiteSpace($commit)) {
    $commit = 'unknown'
}

New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null
Push-Location $repositoryRoot
try {
    & npm.cmd --prefix webui run build
    if ($LASTEXITCODE -ne 0) {
        throw 'WebUI build failed.'
    }

    $ldflags = "-s -w -X tensors-router/internal/buildinfo.Version=$version -X tensors-router/internal/buildinfo.Commit=$commit -X tensors-router/internal/buildinfo.Date=$buildDate"
    $environment = @{ GOOS = 'windows'; GOARCH = $Architecture; CGO_ENABLED = '0' }
    $targets = @(
        @{ Package = './cmd/tensors-router'; Name = "tensors-router-windows-$Architecture.exe" },
        @{ Package = './cmd/tensor-router-webui'; Name = "tensor-router-webui-windows-$Architecture.exe" },
        @{ Package = './cmd/tensor-router-downloader'; Name = "tensor-router-downloader-windows-$Architecture.exe" }
    )

    foreach ($target in $targets) {
        $outputPath = Join-Path $outputDirectory $target.Name
        & go build -buildvcs=false -trimpath -ldflags $ldflags -o $outputPath $target.Package
        if ($LASTEXITCODE -ne 0) {
            throw "Build failed for $($target.Package)."
        }
        $outputPath
    }
}
finally {
    Pop-Location
}
