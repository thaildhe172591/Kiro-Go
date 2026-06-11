# Kiro-Go on IIS / Windows Server

This file is kept for backwards links only. The authoritative deployment
sequence is in [PROD-RUNBOOK.md](PROD-RUNBOOK.md).

## Quick reference

- Site name: `apiunlimit.escs.vn`
- Site physical path: `F:\website\apiunlimit.escs.vn`
- Backend: `127.0.0.1:9180` (NSSM service `kiro-go`)
- Config: `F:\website\apiunlimit.escs.vn\data\config.json`
- Logs: `F:\website\apiunlimit.escs.vn\logs\`
- Wrapper script: `kiro.cmd` at site root (run `kiro install`, `kiro start`, `kiro test`, ...)

For the full step-by-step (preflight, ARR, NSSM service, firewall lockdown,
IIS site, Let's Encrypt, verification, upgrade, rollback) see
[PROD-RUNBOOK.md](PROD-RUNBOOK.md). For the pre-launch checklist see
[HARDENING-CHECKLIST.md](HARDENING-CHECKLIST.md).
