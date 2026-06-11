# Public Deployment Hardening Checklist

## Network

- Bind IIS public traffic to `443` with valid TLS certificate.
- Redirect `80` to `443` or close `80` if redirect is handled by CDN/load balancer.
- Keep Kiro-Go backend on `127.0.0.1:9180` only.
- Block inbound public access to `9180` in Windows Firewall.
- Allow inbound only `80/443` from Internet unless remote admin needs explicit source IPs.
- Put Cloudflare/WAF/reverse proxy in front if endpoint is public and high-traffic.
- Rate-limit API routes at CDN/WAF/IIS layer if many clients use same domain.

## IIS Site

- Use dedicated site `apiunlimit.escs.vn`.
- Use dedicated app pool with `.NET CLR version: No Managed Code`.
- Use least-privilege app pool identity.
- Disable directory browsing.
- Site physical path is `F:\website\apiunlimit.escs.vn` (combined backend + IIS site root).
- `web.config` at site root blocks public access to `data`, `logs`, `releases`, `deploy`, and to `.exe`/`.cmd`/`.ps1`/`.config`/`.bak`/`.zip` extensions. Do not weaken these rules.
- Keep `URL Rewrite` and `ARR` updated.
- Enable ARR proxy and preserve host header.
- Increase ARR proxy timeout for long streaming requests.
- Disable response buffering if ARR version exposes that setting.

## Public Routes

- Public allowlist should include only needed routes:
  - `/admin`
  - `/v1/messages`
  - `/messages`
  - `/anthropic/v1/messages`
  - `/v1/messages/count_tokens`
  - `/v1/chat/completions`
  - `/chat/completions`
  - `/v1/responses`
  - `/responses`
- Unknown routes should return `404`.
- Consider IP allowlist or VPN for `/admin`.
- If `/admin` must be public, use strong password and external WAF rules.

## TLS

- Use HTTPS only for public clients.
- Use automatic certificate renewal where possible.
- Disable obsolete TLS versions and weak cipher suites at OS/IIS level.
- Test with SSL Labs or equivalent before production exposure.

## Secrets

- Set strong `ADMIN_PASSWORD`; never use default `changeme`.
- Store config at `F:\website\apiunlimit.escs.vn\data\config.json`. `web.config` blocks the `data` segment from public.
- Restrict NTFS ACLs on `F:\website\apiunlimit.escs.vn\data` to Administrators + service identity only.
- Do not commit or upload `data\config.json`.
- Do not place account exports under `C:\inetpub` or any other IIS-served path.
- Encrypt server backups or store them in protected backup vault.
- Rotate Kiro/account/API credentials if backup, snapshot, or logs leak.

## Kiro-Go App

- Keep `Host` as `127.0.0.1` in config.
- Keep `Port` as `9180` unless conflict exists.
- Set `LOG_LEVEL=info` for normal production.
- Use API keys for external clients; do not run open proxy.
- Give each API key quota where practical.
- Review account overage settings before public launch.
- Review prompt filtering settings before multi-tenant usage.
- Avoid exposing admin account import/export features to untrusted operators.

## Windows Service

- Run through `NSSM`, not raw `sc.exe`, because app is console executable.
- Configure auto-start.
- Configure restart on crash.
- Rotate stdout/stderr logs.
- Verify service starts after reboot.
- Keep service binary path stable at `F:\website\apiunlimit.escs.vn\kiro-go.exe`.
- For upgrades, stop service, replace binary, start service, then verify health.

## Logging

- Monitor IIS access logs for 401/403/404 spikes and long request duration.
- Monitor Kiro-Go stdout/stderr logs under `F:\website\apiunlimit.escs.vn\logs`.
- Avoid debug logging in production unless actively troubleshooting.
- Redact tokens, cookies, authorization headers, and account exports before sharing logs.
- Configure log rotation or scheduled cleanup.

## Backup and Recovery

- Back up `F:\website\apiunlimit.escs.vn\data\config.json` securely.
- Back up IIS site config and certificate material if not managed externally.
- Test restore on non-production machine.
- Keep old working `kiro-go.exe` for rollback.
- Document exact deployed version or commit SHA.

## Monitoring

- Check service status with `Get-Service kiro-go` (or `kiro status`).
- Check backend with `Invoke-WebRequest http://127.0.0.1:9180/admin` (or `kiro local`).
- Check public endpoint with `Invoke-WebRequest https://apiunlimit.escs.vn/admin` (or `kiro site`).
- Add uptime monitor for `/admin` or a safe public route.
- Alert on service stopped, high 5xx, disk near full, and certificate expiry.

## Verification Before Public Launch

- `https://apiunlimit.escs.vn/admin` loads.
- Login works with configured admin password.
- Public `/admin` access policy matches intent.
- `/v1/messages` works with streaming client.
- `/v1/chat/completions` works with OpenAI-compatible client.
- `/v1/responses` works if client uses Responses API.
- Unknown routes return `404`.
- Windows Firewall blocks public `9180`.
- Restarting IIS does not stop Kiro-Go service.
- Reboot restores IIS site and `kiro-go` service.
- Backup restore path is known and tested.
