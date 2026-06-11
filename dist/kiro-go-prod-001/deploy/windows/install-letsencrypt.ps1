param(
    [Parameter(Mandatory=$true)][string]$Domain,
    [Parameter(Mandatory=$true)][string]$Email,
    [string]$WacsPath = "C:\win-acme\wacs.exe",
    [string]$SiteName = "apiunlimit.escs.vn"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $WacsPath)) {
    Write-Host "win-acme not found at $WacsPath."
    Write-Host "Download from https://www.win-acme.com (pluggable .NET 4.7.2 zip)."
    Write-Host "Extract to C:\win-acme\ then re-run this script."
    throw "win-acme missing"
}

$site = Get-Website -Name $SiteName -ErrorAction SilentlyContinue
if (-not $site) { throw "IIS site not found: $SiteName. Run create-iis-site.ps1 first." }

$httpBinding = Get-WebBinding -Name $SiteName -Protocol http -ErrorAction SilentlyContinue
if (-not $httpBinding) {
    Write-Host "Adding temporary HTTP:80 binding for ACME http-01 challenge..."
    New-WebBinding -Name $SiteName -Protocol http -Port 80 -HostHeader $Domain | Out-Null
}

& $WacsPath --target iis --siteid $site.Id --host $Domain --emailaddress $Email --accepttos --installation iis --installationsiteid $site.Id

Write-Host "Cert issued and bound. win-acme registers a scheduled task for auto-renewal."
Write-Host "Verify renewal task: schtasks /query /tn win-acme*"
