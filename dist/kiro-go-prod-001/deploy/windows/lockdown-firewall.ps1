param(
    [string]$BackendPort = "9180"
)

$ErrorActionPreference = "Stop"

$ruleName = "Kiro-Go Block Public Backend ($BackendPort)"
Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
New-NetFirewallRule -DisplayName $ruleName -Direction Inbound -Action Block -Protocol TCP -LocalPort $BackendPort -Profile Public,Domain,Private -RemoteAddress Any | Out-Null
Get-NetFirewallRule -DisplayName "Kiro-Go Allow Loopback ($BackendPort)" -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue
New-NetFirewallRule -DisplayName "Kiro-Go Allow Loopback ($BackendPort)" -Direction Inbound -Action Allow -Protocol TCP -LocalPort $BackendPort -RemoteAddress 127.0.0.1,::1 | Out-Null

$httpRule = Get-NetFirewallRule -DisplayName "Kiro-Go Public HTTP" -ErrorAction SilentlyContinue
if (-not $httpRule) {
    New-NetFirewallRule -DisplayName "Kiro-Go Public HTTP" -Direction Inbound -Action Allow -Protocol TCP -LocalPort 80 -Profile Any | Out-Null
}
$httpsRule = Get-NetFirewallRule -DisplayName "Kiro-Go Public HTTPS" -ErrorAction SilentlyContinue
if (-not $httpsRule) {
    New-NetFirewallRule -DisplayName "Kiro-Go Public HTTPS" -Direction Inbound -Action Allow -Protocol TCP -LocalPort 443 -Profile Any | Out-Null
}

Write-Host "Firewall locked down:"
Write-Host "  Block public inbound on TCP/$BackendPort"
Write-Host "  Allow loopback only on TCP/$BackendPort"
Write-Host "  Allow public TCP/80 and TCP/443"
Get-NetFirewallRule -DisplayName "Kiro-Go*" | Format-Table DisplayName, Direction, Action, Enabled -AutoSize
