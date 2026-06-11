param(
    [string]$SiteName = "apiunlimit.escs.vn",
    [string]$PhysicalPath = "F:\website\apiunlimit.escs.vn",
    [string]$AppPoolName = "apiunlimit.escs.vn",
    [string]$HostName = "apiunlimit.escs.vn",
    [int]$Port = 80,
    [string]$CertThumbprint,
    [string]$SslStore = "My"
)

$ErrorActionPreference = "Stop"

Import-Module WebAdministration

if (-not (Test-Path $PhysicalPath)) { throw "Physical path missing: $PhysicalPath" }

if (-not (Get-WebAppPoolState -Name $AppPoolName -ErrorAction SilentlyContinue)) {
    New-WebAppPool -Name $AppPoolName | Out-Null
}
Set-ItemProperty "IIS:\AppPools\$AppPoolName" -Name managedRuntimeVersion -Value ""
Set-ItemProperty "IIS:\AppPools\$AppPoolName" -Name enable32BitAppOnWin64 -Value $false

$site = Get-Website -Name $SiteName -ErrorAction SilentlyContinue
if (-not $site) {
    New-Website -Name $SiteName -Port $Port -HostHeader $HostName -PhysicalPath $PhysicalPath -ApplicationPool $AppPoolName | Out-Null
} else {
    Set-ItemProperty "IIS:\Sites\$SiteName" -Name physicalPath -Value $PhysicalPath
    Set-ItemProperty "IIS:\Sites\$SiteName" -Name applicationPool -Value $AppPoolName
}

if ($CertThumbprint) {
    New-WebBinding -Name $SiteName -Protocol https -Port $Port -HostHeader $HostName -ErrorAction SilentlyContinue | Out-Null
    $binding = Get-WebBinding -Name $SiteName -Protocol https -Port $Port -HostHeader $HostName
    $binding.AddSslCertificate($CertThumbprint, $SslStore)
}

Write-Host "Created/updated site: $SiteName"
