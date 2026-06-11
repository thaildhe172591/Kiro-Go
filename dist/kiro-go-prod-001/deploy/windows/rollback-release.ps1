param(
    [string]$AppDir = "F:\website\apiunlimit.escs.vn",
    [string]$ServiceName = "kiro-go",
    [string]$ReleaseDir
)

$ErrorActionPreference = "Stop"

if (-not $ReleaseDir) { throw "Missing -ReleaseDir" }
if (-not (Test-Path $ReleaseDir)) { throw "Release not found: $ReleaseDir" }

Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
Copy-Item (Join-Path $ReleaseDir "kiro-go.exe") (Join-Path $AppDir "kiro-go.exe") -Force
if (Test-Path (Join-Path $ReleaseDir "web")) { Copy-Item (Join-Path $ReleaseDir "web") $AppDir -Recurse -Force }
if (Test-Path (Join-Path $ReleaseDir "deploy")) { Copy-Item (Join-Path $ReleaseDir "deploy") $AppDir -Recurse -Force }
Start-Service -Name $ServiceName
Get-Service -Name $ServiceName
