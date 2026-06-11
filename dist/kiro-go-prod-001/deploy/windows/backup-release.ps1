param(
    [string]$AppDir = "F:\website\apiunlimit.escs.vn",
    [string]$ReleaseName = "kiro-go",
    [string]$Version
)

$ErrorActionPreference = "Stop"

$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$versionLabel = if ($Version) { $Version } else { $timestamp }
$releaseDir = Join-Path $AppDir "releases\$ReleaseName-$versionLabel"
New-Item -ItemType Directory -Force -Path $releaseDir | Out-Null
Copy-Item (Join-Path $AppDir "kiro-go.exe") $releaseDir -Force
Copy-Item (Join-Path $AppDir "web") $releaseDir -Recurse -Force
Copy-Item (Join-Path $AppDir "deploy") $releaseDir -Recurse -Force
if (Test-Path (Join-Path $AppDir "data\config.json")) {
    Copy-Item (Join-Path $AppDir "data\config.json") (Join-Path $releaseDir "config.json.bak") -Force
}
Write-Host $releaseDir
