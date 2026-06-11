param(
    [Parameter(Mandatory=$true)][string]$PackagePath,
    [string]$AppDir = "F:\website\apiunlimit.escs.vn",
    [string]$ServiceName = "kiro-go",
    [string]$NssmPath = "C:\Users\thaild\Downloads\nssm-2.24\win64\nssm.exe",
    [string]$AdminPassword,
    [string]$LogLevel = "info"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $PackagePath)) { throw "Package missing: $PackagePath" }

$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
$releaseDir = Join-Path $AppDir "releases\$stamp"
New-Item -ItemType Directory -Force -Path $releaseDir | Out-Null
Expand-Archive -Path $PackagePath -DestinationPath $releaseDir -Force

$service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
if ($service) { Stop-Service -Name $ServiceName -Force }

Copy-Item (Join-Path $releaseDir "kiro-go.exe") (Join-Path $AppDir "kiro-go.exe") -Force
if (Test-Path (Join-Path $releaseDir "web")) { Copy-Item (Join-Path $releaseDir "web") $AppDir -Recurse -Force }
if (Test-Path (Join-Path $releaseDir "deploy")) { Copy-Item (Join-Path $releaseDir "deploy") $AppDir -Recurse -Force }

if (-not $AdminPassword) { throw "Missing -AdminPassword" }
[Environment]::SetEnvironmentVariable("CONFIG_PATH", (Join-Path $AppDir "data\config.json"), "Machine")
[Environment]::SetEnvironmentVariable("ADMIN_PASSWORD", $AdminPassword, "Machine")
[Environment]::SetEnvironmentVariable("LOG_LEVEL", $LogLevel, "Machine")

if (-not (Test-Path $NssmPath)) { throw "NSSM missing: $NssmPath" }
& $NssmPath restart $ServiceName | Out-Null
Start-Sleep -Seconds 5
Get-Service -Name $ServiceName
