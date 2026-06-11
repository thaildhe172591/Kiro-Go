param(
    [string]$OutDir = "dist",
    [string]$Version = "dev",
    [string]$Goos = "windows",
    [string]$Goarch = "amd64"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path "go.mod")) {
    throw "Run from repo root (go.mod not found in $(Get-Location))"
}

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go toolchain not on PATH. Install Go and retry."
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$packageDir = Join-Path $OutDir "kiro-go-$Version"
if (Test-Path $packageDir) { Remove-Item $packageDir -Recurse -Force }
New-Item -ItemType Directory -Force -Path $packageDir | Out-Null

$env:GOOS = $Goos
$env:GOARCH = $Goarch
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-s -w" -o (Join-Path $packageDir "kiro-go.exe") .
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

Copy-Item web $packageDir -Recurse -Force
Copy-Item deploy $packageDir -Recurse -Force
Copy-Item kiro.cmd $packageDir -Force
Copy-Item deploy\iis\web.config (Join-Path $packageDir "web.config") -Force
@{
    version = $Version
    goos    = $Goos
    goarch  = $Goarch
    builtAt = (Get-Date).ToString('o')
    commit  = (git rev-parse --short HEAD 2>$null)
} | ConvertTo-Json | Set-Content (Join-Path $packageDir "release.json")

$zipPath = Join-Path $OutDir "kiro-go-$Version.zip"
if (Test-Path $zipPath) { Remove-Item $zipPath -Force }
Compress-Archive -Path (Join-Path $packageDir '*') -DestinationPath $zipPath -Force
Write-Host "Built: $zipPath"
Write-Host "Size: $([math]::Round((Get-Item $zipPath).Length / 1MB, 2)) MB"
