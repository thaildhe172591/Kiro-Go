param(
    [string]$AppDir = "F:\website\apiunlimit.escs.vn",
    [string]$ServiceName = "kiro-go",
    [string]$SiteName = "apiunlimit.escs.vn",
    [string]$BackendUrl = "http://127.0.0.1:9180",
    [string]$NssmPath = "C:\Users\thaild\Downloads\nssm-2.24\win64\nssm.exe"
)

$ErrorActionPreference = "Stop"

Import-Module ServerManager -ErrorAction SilentlyContinue
Import-Module WebAdministration -ErrorAction SilentlyContinue

$required = @(
    $AppDir,
    (Join-Path $AppDir "data"),
    (Join-Path $AppDir "logs"),
    (Join-Path $AppDir "releases")
)

foreach ($path in $required) {
    New-Item -ItemType Directory -Force -Path $path | Out-Null
}

$iisFeature = Get-WindowsFeature Web-Server -ErrorAction SilentlyContinue
$site = if (Get-Command Get-Website -ErrorAction SilentlyContinue) { Get-Website -Name $SiteName -ErrorAction SilentlyContinue } else { $null }

$backendOk = $false
try {
    $res = Invoke-WebRequest "$BackendUrl/admin" -UseBasicParsing -TimeoutSec 5
    $backendOk = ($res.StatusCode -eq 200)
} catch { }

$checks = @(
    [pscustomobject]@{ Name = "AppDir exists"; Ok = (Test-Path $AppDir) },
    [pscustomobject]@{ Name = "NSSM exists"; Ok = (Test-Path $NssmPath) },
    [pscustomobject]@{ Name = "IIS installed"; Ok = if ($iisFeature) { [bool]$iisFeature.Installed } else { $false } },
    [pscustomobject]@{ Name = "Service exists"; Ok = [bool](Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) },
    [pscustomobject]@{ Name = "Site exists"; Ok = [bool]$site },
    [pscustomobject]@{ Name = "Backend reachable"; Ok = $backendOk }
)

$checks | Format-Table -AutoSize

if (-not (Test-Path $NssmPath)) { throw "NSSM missing: $NssmPath" }
