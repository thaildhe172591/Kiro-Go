param(
    [string]$ServiceName = "kiro-go",
    [string]$AppDir = "F:\website\apiunlimit.escs.vn",
    [string]$BinaryName = "kiro-go.exe",
    [string]$ConfigPath = "F:\website\apiunlimit.escs.vn\data\config.json",
    [string]$NssmPath = "C:\Users\thaild\Downloads\nssm-2.24\win64\nssm.exe",
    [string]$AdminPassword,
    [string]$LogLevel = "info"
)

$ErrorActionPreference = "Stop"

if (-not $AdminPassword) {
    throw "Missing -AdminPassword"
}

$binaryPath = Join-Path $AppDir $BinaryName
if (-not (Test-Path $binaryPath)) {
    throw "Binary not found: $binaryPath"
}

if (-not (Test-Path $NssmPath)) {
    throw "NSSM not found: $NssmPath. Download NSSM and place nssm.exe there first."
}

New-Item -ItemType Directory -Force -Path $AppDir | Out-Null
New-Item -ItemType Directory -Force -Path (Split-Path $ConfigPath -Parent) | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $AppDir "logs") | Out-Null

if (-not (Test-Path $ConfigPath)) {
    $defaultConfig = @{
        password      = $AdminPassword
        port          = 9180
        host          = "127.0.0.1"
        requireApiKey = $false
        accounts      = @()
        logLevel      = $LogLevel
    } | ConvertTo-Json -Depth 5
    Set-Content -Path $ConfigPath -Value $defaultConfig -Encoding UTF8
    Write-Host "Wrote default config (host=127.0.0.1) to $ConfigPath"
} else {
    try {
        $existing = Get-Content $ConfigPath -Raw | ConvertFrom-Json
        if ($existing.host -eq "0.0.0.0") {
            Write-Warning "Config at $ConfigPath has host=0.0.0.0. For IIS reverse proxy, change to 127.0.0.1 to prevent direct public access on port $($existing.port)."
        }
    } catch {
        Write-Warning "Could not parse $ConfigPath to check host binding."
    }
}

$existing = & $NssmPath status $ServiceName 2>$null
if ($LASTEXITCODE -eq 0) {
    Write-Host "Service exists. Stopping and removing: $ServiceName"
    & $NssmPath stop $ServiceName | Out-Null
    & $NssmPath remove $ServiceName confirm | Out-Null
}

& $NssmPath install $ServiceName $binaryPath | Out-Null
& $NssmPath set $ServiceName AppDirectory $AppDir | Out-Null
& $NssmPath set $ServiceName AppEnvironmentExtra "CONFIG_PATH=$ConfigPath" "ADMIN_PASSWORD=$AdminPassword" "LOG_LEVEL=$LogLevel" | Out-Null
& $NssmPath set $ServiceName AppStdout (Join-Path $AppDir "logs\kiro-go.out.log") | Out-Null
& $NssmPath set $ServiceName AppStderr (Join-Path $AppDir "logs\kiro-go.err.log") | Out-Null
& $NssmPath set $ServiceName AppRotateFiles 1 | Out-Null
& $NssmPath set $ServiceName AppRotateOnline 1 | Out-Null
& $NssmPath set $ServiceName AppRotateBytes 10485760 | Out-Null
& $NssmPath set $ServiceName Start SERVICE_AUTO_START | Out-Null
& $NssmPath set $ServiceName AppThrottle 5000 | Out-Null
& $NssmPath set $ServiceName AppRestartDelay 5000 | Out-Null
& $NssmPath set $ServiceName Description "Kiro-Go reverse proxy service" | Out-Null

& $NssmPath start $ServiceName | Out-Null
Get-Service -Name $ServiceName
