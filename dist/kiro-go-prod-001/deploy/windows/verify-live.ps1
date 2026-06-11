param(
    [Parameter(Mandatory=$true)][string]$PublicBaseUrl,
    [string]$BackendUrl = "http://127.0.0.1:9180"
)

$ErrorActionPreference = "Stop"

function Test-Status {
    param(
        [string]$Name,
        [string]$Url,
        [string]$Method = "GET",
        [string]$Body,
        [int[]]$AllowedStatus
    )

    try {
        $params = @{ Uri = $Url; UseBasicParsing = $true; TimeoutSec = 15; Method = $Method }
        if ($Body) { $params.Body = $Body; $params.ContentType = "application/json" }
        $res = Invoke-WebRequest @params
        $code = [int]$res.StatusCode
    } catch {
        if ($_.Exception.Response) {
            $code = [int]$_.Exception.Response.StatusCode.value__
        } else {
            throw $_
        }
    }

    if ($AllowedStatus -notcontains $code) {
        throw "$Name failed: $Url returned $code; expected one of $($AllowedStatus -join ',')"
    }
    Write-Host "${Name}: OK ($code)"
}

Test-Status -Name "backend admin" -Url "$BackendUrl/admin" -AllowedStatus @(200)
Test-Status -Name "public admin" -Url "$PublicBaseUrl/admin" -AllowedStatus @(200)
Test-Status -Name "public locales" -Url "$PublicBaseUrl/admin/locales/en.json" -AllowedStatus @(200)
Test-Status -Name "public messages route" -Url "$PublicBaseUrl/v1/messages" -Method "POST" -Body '{}' -AllowedStatus @(200,400,401,403,405,422)
Test-Status -Name "public chat route" -Url "$PublicBaseUrl/v1/chat/completions" -Method "POST" -Body '{}' -AllowedStatus @(200,400,401,403,405,422)
Test-Status -Name "public models route" -Url "$PublicBaseUrl/v1/models" -AllowedStatus @(200,401)
Test-Status -Name "unknown route" -Url "$PublicBaseUrl/does-not-exist" -AllowedStatus @(404)
