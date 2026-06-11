param(
    [int]$ResponseBufferThresholdKB = 0,
    [int]$RequestTimeoutSeconds = 600
)

$ErrorActionPreference = "Stop"

$appcmd = Join-Path $env:SystemRoot "System32\inetsrv\appcmd.exe"
if (-not (Test-Path $appcmd)) { throw "appcmd.exe not found. Install IIS first." }

& $appcmd set config -section:system.webServer/proxy /enabled:"True" /commit:apphost | Out-Null
& $appcmd set config -section:system.webServer/proxy /preserveHostHeader:"True" /commit:apphost | Out-Null
& $appcmd set config -section:system.webServer/proxy /timeout:"00:10:00" /commit:apphost | Out-Null

& $appcmd set config -section:system.webServer/proxy /responseBufferLimit:0 /commit:apphost 2>$null | Out-Null

$proxyConfig = Get-WebConfiguration -Filter "system.webServer/proxy" -PSPath "MACHINE/WEBROOT/APPHOST"
if ($proxyConfig) {
    Set-WebConfigurationProperty -Filter "system.webServer/proxy" -PSPath "MACHINE/WEBROOT/APPHOST" -Name "enabled" -Value $true
    Set-WebConfigurationProperty -Filter "system.webServer/proxy" -PSPath "MACHINE/WEBROOT/APPHOST" -Name "preserveHostHeader" -Value $true
    Set-WebConfigurationProperty -Filter "system.webServer/proxy" -PSPath "MACHINE/WEBROOT/APPHOST" -Name "timeout" -Value "00:10:00"
    try {
        Set-WebConfigurationProperty -Filter "system.webServer/proxy" -PSPath "MACHINE/WEBROOT/APPHOST" -Name "responseBufferLimit" -Value 0
    } catch {
        Write-Warning "responseBufferLimit not settable on this ARR version: $($_.Exception.Message)"
    }
}

Write-Host "ARR configured: proxy=enabled, timeout=00:10:00, responseBufferLimit=0 (SSE streaming safe)"
Write-Host "Verify in IIS Manager > Server > Application Request Routing Cache > Server Proxy Settings"
