package config

import (
	"path/filepath"
	"testing"
	"time"
)

// initEmptyConfig spins up a fresh config singleton backed by a temp file.
func initEmptyConfig(t *testing.T) {
	t.Helper()
	cfgFile := filepath.Join(t.TempDir(), "config.json")
	if err := Init(cfgFile); err != nil {
		t.Fatalf("init: %v", err)
	}
}

// TestAddApiKeyArmsLifetimeClock verifies the deadline is set only when a key is
// created enabled with a TTL, and stays stopped otherwise.
func TestAddApiKeyArmsLifetimeClock(t *testing.T) {
	initEmptyConfig(t)

	// Enabled + TTL -> clock armed.
	now := time.Now().Unix()
	armed, err := AddApiKey(ApiKeyEntry{Name: "armed", Key: "sk-armed", Enabled: true, LifetimeSeconds: 86400})
	if err != nil {
		t.Fatalf("add armed: %v", err)
	}
	if armed.ExpiresAt < now+86400-2 || armed.ExpiresAt > now+86400+2 {
		t.Fatalf("expected ExpiresAt ~now+86400, got %d (now=%d)", armed.ExpiresAt, now)
	}

	// Disabled + TTL -> clock stopped.
	off, err := AddApiKey(ApiKeyEntry{Name: "off", Key: "sk-off", Enabled: false, LifetimeSeconds: 86400})
	if err != nil {
		t.Fatalf("add off: %v", err)
	}
	if off.ExpiresAt != 0 {
		t.Fatalf("expected disabled key clock stopped, got ExpiresAt=%d", off.ExpiresAt)
	}

	// Enabled, no TTL -> permanent, clock stopped.
	perm, err := AddApiKey(ApiKeyEntry{Name: "perm", Key: "sk-perm", Enabled: true})
	if err != nil {
		t.Fatalf("add perm: %v", err)
	}
	if perm.ExpiresAt != 0 {
		t.Fatalf("expected permanent key clock stopped, got ExpiresAt=%d", perm.ExpiresAt)
	}
}

// TestUpdateApiKeyLifetimeTransitions exercises the enable/disable/TTL-change
// transitions of the lifetime clock.
func TestUpdateApiKeyLifetimeTransitions(t *testing.T) {
	initEmptyConfig(t)

	created, err := AddApiKey(ApiKeyEntry{Name: "k", Key: "sk-k", Enabled: false, LifetimeSeconds: 86400})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if created.ExpiresAt != 0 {
		t.Fatalf("expected stopped clock on disabled create, got %d", created.ExpiresAt)
	}

	// disabled -> enabled: arm a fresh full TTL from now.
	now := time.Now().Unix()
	if err := UpdateApiKey(created.ID, ApiKeyEntry{Enabled: true, LifetimeSeconds: 86400}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	got := GetApiKeyEntry(created.ID)
	if got.ExpiresAt < now+86400-2 || got.ExpiresAt > now+86400+2 {
		t.Fatalf("expected armed deadline ~now+86400, got %d", got.ExpiresAt)
	}
	armed := got.ExpiresAt

	// staying enabled, TTL unchanged: deadline left as-is.
	if err := UpdateApiKey(created.ID, ApiKeyEntry{Enabled: true, LifetimeSeconds: 86400, Name: "renamed"}); err != nil {
		t.Fatalf("noop update: %v", err)
	}
	got = GetApiKeyEntry(created.ID)
	if got.ExpiresAt != armed {
		t.Fatalf("expected deadline unchanged when TTL stable, got %d want %d", got.ExpiresAt, armed)
	}

	// staying enabled, TTL changed: re-arm from now with the new TTL.
	now = time.Now().Unix()
	if err := UpdateApiKey(created.ID, ApiKeyEntry{Enabled: true, LifetimeSeconds: 7 * 86400}); err != nil {
		t.Fatalf("ttl change: %v", err)
	}
	got = GetApiKeyEntry(created.ID)
	if got.ExpiresAt < now+7*86400-2 || got.ExpiresAt > now+7*86400+2 {
		t.Fatalf("expected re-armed deadline ~now+7d, got %d", got.ExpiresAt)
	}

	// enabled -> disabled: stop the clock and reset to 0.
	if err := UpdateApiKey(created.ID, ApiKeyEntry{Enabled: false, LifetimeSeconds: 7 * 86400}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	got = GetApiKeyEntry(created.ID)
	if got.ExpiresAt != 0 {
		t.Fatalf("expected stopped clock after disable, got %d", got.ExpiresAt)
	}
}

// TestRestartApiKeyLifetime confirms restart re-enables and counts a fresh full TTL.
func TestRestartApiKeyLifetime(t *testing.T) {
	initEmptyConfig(t)

	created, err := AddApiKey(ApiKeyEntry{Name: "r", Key: "sk-r", Enabled: false, LifetimeSeconds: 86400})
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	now := time.Now().Unix()
	if err := RestartApiKeyLifetime(created.ID); err != nil {
		t.Fatalf("restart: %v", err)
	}
	got := GetApiKeyEntry(created.ID)
	if !got.Enabled {
		t.Fatalf("expected key re-enabled after restart")
	}
	if got.ExpiresAt < now+86400-2 || got.ExpiresAt > now+86400+2 {
		t.Fatalf("expected fresh deadline ~now+86400, got %d", got.ExpiresAt)
	}

	if err := RestartApiKeyLifetime("nope"); err == nil {
		t.Fatalf("expected error for unknown id")
	}
}

// TestRestartApiKeyLifetimeNoTTL: restarting a permanent key just enables it.
func TestRestartApiKeyLifetimeNoTTL(t *testing.T) {
	initEmptyConfig(t)
	created, err := AddApiKey(ApiKeyEntry{Name: "p", Key: "sk-p", Enabled: false})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := RestartApiKeyLifetime(created.ID); err != nil {
		t.Fatalf("restart: %v", err)
	}
	got := GetApiKeyEntry(created.ID)
	if !got.Enabled || got.ExpiresAt != 0 {
		t.Fatalf("expected enabled with stopped clock, got enabled=%v expiresAt=%d", got.Enabled, got.ExpiresAt)
	}
}

// TestExtendApiKeyLifetime checks deadline extension from a running and a stopped clock.
func TestExtendApiKeyLifetime(t *testing.T) {
	initEmptyConfig(t)

	created, err := AddApiKey(ApiKeyEntry{Name: "e", Key: "sk-e", Enabled: true, LifetimeSeconds: 86400})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	base := GetApiKeyEntry(created.ID).ExpiresAt

	// Extend a running clock: deadline moves out by exactly N days.
	if err := ExtendApiKeyLifetime(created.ID, 3); err != nil {
		t.Fatalf("extend: %v", err)
	}
	got := GetApiKeyEntry(created.ID)
	if got.ExpiresAt != base+3*86400 {
		t.Fatalf("expected deadline +3d, got %d want %d", got.ExpiresAt, base+3*86400)
	}

	// Non-positive days rejected.
	if err := ExtendApiKeyLifetime(created.ID, 0); err == nil {
		t.Fatalf("expected error for zero days")
	}
	if err := ExtendApiKeyLifetime(created.ID, -1); err == nil {
		t.Fatalf("expected error for negative days")
	}

	// Extend from a stopped clock counts from now.
	stopped, err := AddApiKey(ApiKeyEntry{Name: "s", Key: "sk-s", Enabled: false})
	if err != nil {
		t.Fatalf("add stopped: %v", err)
	}
	now := time.Now().Unix()
	if err := ExtendApiKeyLifetime(stopped.ID, 2); err != nil {
		t.Fatalf("extend stopped: %v", err)
	}
	got = GetApiKeyEntry(stopped.ID)
	if got.ExpiresAt < now+2*86400-2 || got.ExpiresAt > now+2*86400+2 {
		t.Fatalf("expected deadline ~now+2d from stopped clock, got %d", got.ExpiresAt)
	}
}

// TestExtendApiKeyLifetimeSeconds checks minute-level extension and that the
// day wrapper delegates to it.
func TestExtendApiKeyLifetimeSeconds(t *testing.T) {
	initEmptyConfig(t)

	created, err := AddApiKey(ApiKeyEntry{Name: "es", Key: "sk-es", Enabled: true, LifetimeSeconds: 3600})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	base := GetApiKeyEntry(created.ID).ExpiresAt

	// Extend by 120 seconds (2 minutes).
	if err := ExtendApiKeyLifetimeSeconds(created.ID, 120); err != nil {
		t.Fatalf("extend seconds: %v", err)
	}
	if got := GetApiKeyEntry(created.ID).ExpiresAt; got != base+120 {
		t.Fatalf("expected deadline +120s, got %d want %d", got, base+120)
	}

	// Non-positive seconds rejected.
	if err := ExtendApiKeyLifetimeSeconds(created.ID, 0); err == nil {
		t.Fatalf("expected error for zero seconds")
	}

	// The day wrapper delegates: +1 day == +86400 seconds.
	base = GetApiKeyEntry(created.ID).ExpiresAt
	if err := ExtendApiKeyLifetime(created.ID, 1); err != nil {
		t.Fatalf("extend day wrapper: %v", err)
	}
	if got := GetApiKeyEntry(created.ID).ExpiresAt; got != base+86400 {
		t.Fatalf("expected day wrapper to add 86400s, got %d want %d", got, base+86400)
	}
}

// TestDisableExpiredApiKeys verifies only past-deadline enabled keys are disabled.
func TestDisableExpiredApiKeys(t *testing.T) {
	initEmptyConfig(t)

	// Expired enabled key (deadline in the past).
	expired, err := AddApiKey(ApiKeyEntry{Name: "exp", Key: "sk-exp", Enabled: true, LifetimeSeconds: 86400})
	if err != nil {
		t.Fatalf("add exp: %v", err)
	}
	// Force its deadline into the past.
	if err := UpdateApiKey(expired.ID, ApiKeyEntry{Enabled: true, LifetimeSeconds: 86400}); err != nil {
		t.Fatalf("prep: %v", err)
	}
	forceExpiresAt(t, expired.ID, time.Now().Unix()-10)

	// Live enabled key (deadline in the future).
	live, err := AddApiKey(ApiKeyEntry{Name: "live", Key: "sk-live", Enabled: true, LifetimeSeconds: 86400})
	if err != nil {
		t.Fatalf("add live: %v", err)
	}

	// Permanent enabled key (no deadline).
	perm, err := AddApiKey(ApiKeyEntry{Name: "perm", Key: "sk-perm", Enabled: true})
	if err != nil {
		t.Fatalf("add perm: %v", err)
	}

	disabled := DisableExpiredApiKeys()
	if len(disabled) != 1 || disabled[0] != expired.ID {
		t.Fatalf("expected only the expired key disabled, got %v", disabled)
	}

	if e := GetApiKeyEntry(expired.ID); e.Enabled || e.ExpiresAt != 0 {
		t.Fatalf("expired key should be disabled with stopped clock, got enabled=%v expiresAt=%d", e.Enabled, e.ExpiresAt)
	}
	if GetApiKeyEntry(live.ID).Enabled != true {
		t.Fatalf("live key should stay enabled")
	}
	if GetApiKeyEntry(perm.ID).Enabled != true {
		t.Fatalf("permanent key should stay enabled")
	}

	// Idempotent: a second sweep disables nothing.
	if again := DisableExpiredApiKeys(); len(again) != 0 {
		t.Fatalf("expected no further disables, got %v", again)
	}
}

// forceExpiresAt rewrites a key's deadline directly under lock, bypassing the
// normal transition logic so tests can simulate an elapsed clock.
func forceExpiresAt(t *testing.T, id string, expiresAt int64) {
	t.Helper()
	cfgLock.Lock()
	defer cfgLock.Unlock()
	for i := range cfg.ApiKeys {
		if cfg.ApiKeys[i].ID == id {
			cfg.ApiKeys[i].ExpiresAt = expiresAt
			return
		}
	}
	t.Fatalf("forceExpiresAt: id %q not found", id)
}
