# Kiro-Go Production Runbook

## Goal

Run Kiro-Go live behind IIS on Windows Server. IIS terminates TLS via Let's Encrypt and reverse-proxies public routes to Kiro-Go on `127.0.0.1:9180`. Backend port is firewall-blocked from public.

## Layout

Combined backend + IIS site root in one folder:

```text
F:\website\apiunlimit.escs.vn\
  kiro-go.exe                  Backend binary (NSSM runs this on 127.0.0.1:9180)
  kiro.cmd                     Operations wrapper (start/stop/install/test/...)
  web\                         Static admin assets (served by backend)
  data\                        config.json, runtime state (NTFS-locked)
  logs\                        NSSM stdout/stderr logs
  releases\                    Release history for rollback
  deploy\
    iis\web.config             IIS reverse-proxy rules — also lives at site physical path root
    windows\*.ps1              Setup/install scripts
```

The IIS site `apiunlimit.escs.vn` points its physical path to `F:\website\apiunlimit.escs.vn`, so `web.config` is read from there. Static-extension lockdown in `web.config` blocks public access to `.exe`, `.cmd`, `.ps1`, `.config`, `.json`, plus the `data`/`logs`/`releases`/`deploy` folders.

## Required components

- Windows Server with IIS role installed (Web-Server feature)
- IIS URL Rewrite Module 2.x ([download](https://www.iis.net/downloads/microsoft/url-rewrite))
- IIS Application Request Routing 3.x ([download](https://www.iis.net/downloads/microsoft/application-request-routing))
- NSSM at `C:\Users\thaild\Downloads\nssm-2.24\win64\nssm.exe` (override with `KIRO_NSSM` / `-NssmPath` if elsewhere)
- win-acme at `C:\win-acme\wacs.exe` ([win-acme.com](https://www.win-acme.com))
- DNS A record for `apiunlimit.escs.vn` pointing to server public IP
- Inbound 80/443 reachable from public Internet (for ACME http-01 challenge)
- Go toolchain on build machine (dev box, not server)

## Repository artifacts

| File | Purpose |
|---|---|
| `scripts/release/build-package.ps1` | Builds release zip on dev machine |
| `kiro.cmd` | Ops wrapper (mirrors `tskt.cmd`); copied into package and onto server |
| `deploy/iis/web.config` | Allowlist reverse-proxy rules + extension/path lockdown |
| `deploy/windows/setup-preflight.ps1` | Verifies prerequisites before deploy |
| `deploy/windows/configure-arr.ps1` | Enables ARR proxy, sets timeout + responseBufferLimit=0 (SSE) |
| `deploy/windows/install-service.ps1` | Installs NSSM service, writes default 127.0.0.1 config |
| `deploy/windows/lockdown-firewall.ps1` | Blocks public 9180, allows 80/443 |
| `deploy/windows/create-iis-site.ps1` | Creates IIS site + app pool (HTTP-only first) |
| `deploy/windows/install-letsencrypt.ps1` | Issues cert via win-acme + binds HTTPS + auto-renew |
| `deploy/windows/verify-live.ps1` | End-to-end smoke test |
| `deploy/windows/backup-release.ps1` | Snapshots current install before upgrade |
| `deploy/windows/deploy-release.ps1` | Deploys new zip onto running server |
| `deploy/windows/rollback-release.ps1` | Restores previous release |
| `deploy/windows/HARDENING-CHECKLIST.md` | Pre-launch hardening review |

## Step 1 — Build on dev box

```powershell
cd <repo-root>
Set-ExecutionPolicy Bypass -Scope Process -Force
.\scripts\release\build-package.ps1 -Version "prod-001"
# Output: dist\kiro-go-prod-001.zip
```

Verify `kiro.cmd` is present in the package:

```powershell
Get-ChildItem dist\kiro-go-prod-001\kiro.cmd
```

## Step 2 — Transfer to server

Copy to server:

- `dist\kiro-go-prod-001.zip` → `F:\website\kiro-go-prod-001.zip` (any temp location)
- NSSM zip extracted (already at `C:\Users\thaild\Downloads\nssm-2.24\win64\nssm.exe`)
- win-acme zip extracted to `C:\win-acme\`

## Step 3 — Install IIS prerequisites

On server, Admin PowerShell:

```powershell
Install-WindowsFeature Web-Server, Web-Asp-Net45, Web-Net-Ext45 -IncludeManagementTools
```

Then install via the MSIs (download links above):

- URL Rewrite Module 2.x
- Application Request Routing 3.x

## Step 4 — Expand release and run preflight

```powershell
New-Item -ItemType Directory -Force -Path F:\website\apiunlimit.escs.vn | Out-Null
Expand-Archive F:\website\kiro-go-prod-001.zip -DestinationPath F:\website\apiunlimit.escs.vn -Force
F:\website\apiunlimit.escs.vn\deploy\windows\setup-preflight.ps1
```

Expected: all checks except `Service exists` and `Site exists` should be `True`.

## Step 5 — Configure ARR (CRITICAL for SSE streaming)

```powershell
F:\website\apiunlimit.escs.vn\deploy\windows\configure-arr.ps1
```

This enables the proxy globally and sets `responseBufferLimit=0`. Without this, SSE responses buffer until the entire stream completes, breaking Claude/OpenAI streaming clients.

Verify in IIS Manager → Server node → Application Request Routing Cache → Server Proxy Settings:
- Enable proxy: checked
- Response buffer threshold (KB): 0
- Time-out (seconds): 600

## Step 6 — Install Kiro-Go as Windows service

Two options. Use the wrapper for daily ops:

```cmd
cd /d F:\website\apiunlimit.escs.vn
set KIRO_ADMIN_PASSWORD=<STRONG_PASSWORD_HERE>
kiro install
```

Or run the PowerShell script directly:

```powershell
F:\website\apiunlimit.escs.vn\deploy\windows\install-service.ps1 `
  -AdminPassword "<STRONG_PASSWORD_HERE>"
```

Default config writes `host: "127.0.0.1"`, `port: 9180`. Verify backend:

```powershell
Invoke-WebRequest http://127.0.0.1:9180/admin -UseBasicParsing
# Expect 200 OK
# Or: kiro local
```

## Step 7 — Lockdown firewall

```powershell
F:\website\apiunlimit.escs.vn\deploy\windows\lockdown-firewall.ps1
```

Blocks public 9180, allows public 80/443.

Verify from outside the server: `curl http://<server-public-ip>:9180/admin` should fail (timeout or refused).

## Step 8 — Create IIS site (HTTP only first)

```powershell
F:\website\apiunlimit.escs.vn\deploy\windows\create-iis-site.ps1 -Port 80
```

Defaults: site `apiunlimit.escs.vn`, app pool `apiunlimit.escs.vn`, host `apiunlimit.escs.vn`, physical path `F:\website\apiunlimit.escs.vn`. Omit `-CertThumbprint` for now — site listens on HTTP:80 to satisfy the ACME http-01 challenge.

Verify:

```powershell
Invoke-WebRequest http://apiunlimit.escs.vn/admin -UseBasicParsing
# Expect 200 OK
```

## Step 9 — Issue Let's Encrypt cert + bind HTTPS

```powershell
F:\website\apiunlimit.escs.vn\deploy\windows\install-letsencrypt.ps1 `
  -Domain apiunlimit.escs.vn `
  -Email admin@escs.vn
```

win-acme:
1. Validates http-01 challenge via the site's HTTP:80 binding
2. Installs cert into `Cert:\LocalMachine\WebHosting`
3. Adds HTTPS:443 binding to `apiunlimit.escs.vn`
4. Registers a Windows Scheduled Task for auto-renewal (every 60 days)

Verify renewal task:

```powershell
schtasks /query /tn "win-acme*" /fo LIST
```

## Step 10 — Live verification

```powershell
F:\website\apiunlimit.escs.vn\deploy\windows\verify-live.ps1 -PublicBaseUrl https://apiunlimit.escs.vn
```

All probes must pass:
- `backend admin` → 200
- `public admin` → 200
- `public locales` → 200 (regression check for the `.json` rule fix)
- `public messages route` → 200/400/401/403/405/422 (any auth-shaped response)
- `public chat route` → same
- `public models route` → 200/401
- `unknown route` → 404

Or via the wrapper:

```cmd
kiro test
```

## Step 11 — Configure accounts and API keys

1. Open `https://apiunlimit.escs.vn/admin` (or `kiro admin`)
2. Login with `ADMIN_PASSWORD`
3. Add Kiro accounts (Builder ID / IdC SSO / SSO Token / API key)
4. Create API keys for clients under `API Keys` tab
5. Test real client streaming:

```powershell
$key = "<created-api-key>"
curl https://apiunlimit.escs.vn/v1/messages `
  -H "Content-Type: application/json" `
  -H "x-api-key: $key" `
  -H "anthropic-version: 2023-06-01" `
  -d '{"model":"claude-sonnet-4.5","max_tokens":256,"stream":true,"messages":[{"role":"user","content":"hi"}]}'
```

Look for `event: message_start` then `content_block_delta` chunks streaming in real-time. If the response only arrives at the end as one block, ARR buffering is still on — re-run Step 5 and restart `W3SVC`.

## Step 12 — Pre-launch hardening review

Walk through `deploy/windows/HARDENING-CHECKLIST.md` before opening to public traffic.

## Daily operations (kiro.cmd wrapper)

```cmd
kiro config       Show current paths/service/site/port
kiro status       NSSM service status
kiro start        Start service
kiro stop         Stop service
kiro restart      Restart service
kiro test         Probe backend + public site
kiro local        Probe http://127.0.0.1:9180/admin
kiro site         Probe https://apiunlimit.escs.vn/admin
kiro admin        Open admin panel in browser
kiro port         Show process bound to 9180
kiro logs         Print stdout/stderr logs
kiro iis          Restart IIS site
```

Override defaults via env vars before running:
`KIRO_NSSM`, `KIRO_SERVICE`, `KIRO_SITE`, `KIRO_PORT`, `KIRO_SITE_DIR`.

## Upgrade

```powershell
F:\website\apiunlimit.escs.vn\deploy\windows\backup-release.ps1 -Version before-upgrade
F:\website\apiunlimit.escs.vn\deploy\windows\deploy-release.ps1 `
  -PackagePath F:\website\kiro-go-prod-002.zip `
  -AdminPassword "<existing password>"
F:\website\apiunlimit.escs.vn\deploy\windows\verify-live.ps1 -PublicBaseUrl https://apiunlimit.escs.vn
```

## Rollback

```powershell
F:\website\apiunlimit.escs.vn\deploy\windows\rollback-release.ps1 `
  -ReleaseDir F:\website\apiunlimit.escs.vn\releases\kiro-go-before-upgrade
F:\website\apiunlimit.escs.vn\deploy\windows\verify-live.ps1 -PublicBaseUrl https://apiunlimit.escs.vn
```

## Operational rules

- Backend stays on `127.0.0.1:9180`. Never flip to `0.0.0.0`.
- Public 9180 must be blocked at firewall (Step 7).
- `F:\website\apiunlimit.escs.vn\data\config.json` holds account tokens. NTFS-ACL it to Administrators + service identity only.
- `ADMIN_PASSWORD` is set as a service env var by `install-service.ps1` and overrides `data\config.json`. Rotate via re-running install-service with a new password (or `kiro install` with a new `KIRO_ADMIN_PASSWORD`).
- For `/admin`, prefer IP allowlist or VPN. If exposed publicly, monitor 401 spikes.
- IIS access logs at `C:\inetpub\logs\LogFiles\W3SVC<id>\`; Kiro-Go logs at `F:\website\apiunlimit.escs.vn\logs\`.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| Streaming clients hang until completion | ARR buffering still on | Step 5; verify `responseBufferLimit=0` in `applicationHost.config` |
| `502 Bad Gateway` on all routes | Backend service down | `kiro status`; check `F:\website\apiunlimit.escs.vn\logs\kiro-go.err.log` |
| `502.3 Gateway Timeout` on long requests | ARR proxy timeout too low | Step 5; raise `timeout` to `00:10:00` or higher |
| `404` on `/admin/locales/en.json` | Old `BlockSensitiveFiles` rule still in `web.config` | Re-deploy; the rule was removed |
| `/admin` works but API keys "invalid" | Master switch off but expecting auth | Admin → Settings → enable `Require API Key`; or check `RequireApiKey` in config |
| Cert expired, HTTPS broken | win-acme renewal task disabled or failed | `schtasks /query /tn "win-acme*"`; re-run `install-letsencrypt.ps1` |
| `400 Bad Request` from Kiro upstream "Input is too long" | Payload exceeds 900 KiB | Translator already truncates; if seen, check for very large tool results |
