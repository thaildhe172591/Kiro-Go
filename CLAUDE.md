# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Kiro-Go is a Go reverse proxy that translates the AWS Kiro / CodeWhisperer / Amazon Q upstream into OpenAI- and Anthropic-compatible HTTP APIs. It manages a pool of Kiro accounts (multiple auth flavors, OAuth refresh, weighted round-robin, cooldowns) and ships a single-binary admin UI for account/key/setting management. There is **no framework** — net/http only, with the single external dependency being `github.com/google/uuid`.

## Common Commands

```bash
# Build the server binary
go build -o kiro-go .

# Run locally (creates data/config.json on first start)
./kiro-go                       # default config path data/config.json
CONFIG_PATH=/tmp/cfg.json ./kiro-go
ADMIN_PASSWORD=secret ./kiro-go # overrides persisted password

# Run all tests
go test ./...

# Run tests for one package
go test ./proxy
go test ./config

# Run a single test by name
go test ./proxy -run TestTranslator_Truncate

# Verbose output for one test
go test ./proxy -run TestHandler -v

# Static checks
go vet ./...

# Docker
docker-compose up -d            # builds + runs on :8080, mounts ./data
```

The Dockerfile is multi-stage and uses `--platform=$BUILDPLATFORM` cross-compilation; the runtime image only contains the binary plus `web/` (the admin UI is served from disk, not embedded).

## Architecture

The codebase is organized into five packages plus the embedded web UI. **`proxy/handler.go` (~110 KB) and `proxy/translator.go` (~64 KB) are the two giant files** — most behavior lives in them.

```
main.go                  Wires config → logger → pool → http.Server (WriteTimeout=0 for SSE)
config/                  JSON-backed config singleton; Account, ApiKeyEntry, PromptFilterRule schemas
auth/                    Four upstream auth flows + token refresh dispatch
pool/                    Weighted round-robin account pool with cooldowns
proxy/                   HTTP handlers + Claude/OpenAI ↔ Kiro translation + Kiro REST client
web/                     Static admin SPA (vanilla HTML/CSS/JS, locales/{en,zh}.json)
```

### Request flow

1. `proxy.Handler.ServeHTTP` ([proxy/handler.go:341](proxy/handler.go#L341)) is the single entrypoint, dispatching on `r.URL.Path` with a `switch` (no router library).
2. Public API paths (`/v1/messages`, `/v1/chat/completions`, `/v1/responses`, `/v1/messages/count_tokens`) call `authenticateForClaude` / `authenticateForOpenAI`, which delegate to `proxy/auth.go`'s `authenticate`. Auth has a master switch (`config.RequireApiKey`) and prefers the multi-key `ApiKeys` list over the legacy single `ApiKey`. **It fails closed** when the switch is on but no keys are configured.
3. Translators in `proxy/translator.go` (Claude/OpenAI) and `proxy/responses_*.go` (OpenAI Responses API) convert the request into the Kiro wire format. Note `maxPayloadBytes = 900 KiB` — older history turns are dropped with a placeholder when payloads exceed this.
4. `proxy/kiro.go` posts to one of three upstream endpoints (`kiroEndpoints`: Kiro IDE → CodeWhisperer → AmazonQ), parses the AWS Event Stream response, and streams SSE back. Endpoint selection respects `PreferredEndpoint` and `EndpointFallback`.
5. Token usage and credits are attributed back to (a) the account that served it and (b) the API key that authenticated the caller, via `withApiKeyContext` / `apiKeyIDFromContext`.

### Account pool ([pool/account.go](pool/account.go))

`pool.GetPool()` is a singleton lazily populated by `Reload()`. The pool keeps a *weighted* slice — accounts with `Weight >= 2` appear N times so round-robin naturally biases toward them. `GetNextExcluding` skips accounts that are: in cooldown, near token expiry (120 s skew), or quota-blocked. Quota gating uses `isQuotaBlocked`, which is suppressed when either the per-account upstream `OverageStatus=ENABLED` or the global `AllowOverUsage` is set. **Always call `pool.Reload()` after mutating accounts in config** — the pool caches a derived slice.

### Auth flows ([auth/](auth/))

`auth.RefreshToken(*config.Account)` ([auth/oidc.go:31](auth/oidc.go#L31)) is the dispatcher. It branches on `account.AuthMethod`:
- **`builderid`** — AWS Builder ID device-code login ([builderid.go](auth/builderid.go)); social token refresh path
- **`idc`** — IAM Identity Center / Enterprise SSO ([iam_sso.go](auth/iam_sso.go)); OIDC client credentials refresh
- **`sso_token`** — Imported SSO token ([sso_token.go](auth/sso_token.go))
- **`api_key`** — `account.IsApiKeyCredential()` returns true; the Kiro API key is used directly as a bearer token, no refresh needed

Per-account `AuthRegion` and `ApiRegion` (with global fallback) decouple where tokens are minted from where API calls go — useful for cross-region deployments.

### Background loops

`NewHandler()` spawns three goroutines:
- `backgroundRefresh` — every 30 min: refreshes models cache, then for each enabled account refreshes near-expiry tokens via `auth.RefreshToken` and updates usage info via `RefreshAccountInfo`. Calls `pool.Reload()` at the end.
- `backgroundStatsSaver` — every 30 s: persists running counters back to `config.json`.
- `purgeExpiredResponses(responsesDefaultTTL)` — cleans up stored responses older than 30 days (for `previous_response_id` continuity in the OpenAI Responses API).

### Config singleton ([config/config.go](config/config.go))

`config.Init(path)` loads JSON into a global `*Config` guarded by `cfgLock` (RWMutex). All accessors are top-level functions like `GetAccounts`, `GetEnabledAccounts`, `UpdateAccountToken`, `SetPassword`. There are migration paths for legacy fields (`allowOverage` → `OverageStatus`, single `ApiKey` → `ApiKeys`, `SanitizeClaudeCodePrompt` → `FilterClaudeCode`) — when adding new fields with deprecations, follow the same pattern: load old → write new → zero the old field on save so it doesn't reappear.

### Admin UI

`/admin` serves [web/index.html](web/index.html); admin JSON APIs live under `/admin/api/...` and are routed by a second `switch` in `handleAdminAPI` ([proxy/handler.go:2048](proxy/handler.go#L2048)). The big, mostly-unused [web/index-legacy.html](web/index-legacy.html) (170 KB) is kept for fallback — don't edit it unless explicitly working on the legacy UI. Localization is JSON-based: `web/locales/en.json` and `web/locales/zh.json`.

### Thinking mode

Triggered by either appending the configured suffix (default `-thinking`) to the model name, or by sending a top-level `thinking: {type, budget_tokens}` block on Claude requests. The response format is configurable per-API (`OpenAIThinkingFormat`, `ClaudeThinkingFormat`): `reasoning_content`, `thinking`, or `think`. The translator dual-tracks reasoning sources (`thinkingStreamSource`) to avoid mixing tag-block and event-based reasoning streams from the upstream.

### Prompt cache emulation ([proxy/cache_tracker.go](proxy/cache_tracker.go))

The Kiro upstream doesn't report Anthropic-style `cache_read`/`cache_creation` token counts, so `promptCacheTracker` synthesizes them. It hashes cacheable prefixes (system + message breakpoints), stores them with a 5-minute TTL (`defaultPromptCacheTTL`), and reports cache hits on subsequent requests that share a prefix. Breakpoints below `defaultMinCacheableTokens` (1024, per Anthropic's minimum) are excluded so short requests don't report unrealistic 100% hit rates. Per-model minimums come from `minCacheableTokensForModel`.

### Token estimation ([proxy/token_estimator.go](proxy/token_estimator.go))

Because the upstream usage numbers are unreliable for some endpoints, `estimateApproxTokens` and the `estimateClaude*` helpers compute approximate input/output token counts (including thinking content and tool-use blocks) used for usage attribution and the cache tracker. These are heuristics, not a real tokenizer — don't treat them as exact.

## Conventions to Follow

- **Errors are formatted with `fmt.Errorf` and surfaced via `logger.Warnf`/`Errorf`**; the logger has 4 levels (debug/info/warn/error) selected by `LOG_LEVEL` env var or `config.logLevel`.
- **Routing is plain `switch` in `ServeHTTP` and `handleAdminAPI`** — when adding a new endpoint, add a case in the right block (public APIs early, admin APIs in the `/admin/api` switch).
- **Streaming responses set `WriteTimeout: 0` on the http.Server** (see [main.go:73](main.go#L73)). Don't wrap responses in middlewares that buffer; SSE callers depend on flushed writes.
- **No new dependencies without asking** — the project intentionally stays on stdlib + `google/uuid`.
- Per-account proxy support uses `proxyClientCache` (sync.Map). When making outbound calls, route through `GetClientForProxy(ResolveAccountProxyURL(account))` rather than `http.DefaultClient`.

## Test Patterns

23 test files, all standard `go test`. Translator tests in `proxy/translator_*_test.go` cover compaction, truncation, and OpenAI tool format edge cases. `proxy/responses_*_test.go` covers the OpenAI Responses API store. `pool/account_failover_test.go` exercises cooldown transitions. There's a regional routing test (`auth/region_test.go`, `config/region_test.go`) — run these when touching `EffectiveAuthRegion` / `EffectiveApiRegion` logic. Auth tests use `auth/testhooks.go` to inject HTTP responses.

## Environment Variables

| Variable | Purpose |
|---|---|
| `CONFIG_PATH` | Override JSON config path (default `data/config.json`) |
| `ADMIN_PASSWORD` | Override admin panel password at startup |
| `LOG_LEVEL` | `debug` / `info` / `warn` / `error` (overrides config) |

## Notes

- Default admin password is `changeme`. Always override before exposing to any network.
- The `data/` directory is gitignored and contains all runtime state (config + tokens). Don't commit it.
- `.zeabur/context.json` is also gitignored (contains personal IDs) — leave it alone.
