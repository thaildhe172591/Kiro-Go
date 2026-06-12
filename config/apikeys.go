package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// ListApiKeys returns a snapshot of all configured API key entries.
func ListApiKeys() []ApiKeyEntry {
	cfgLock.RLock()
	defer cfgLock.RUnlock()
	if cfg == nil {
		return nil
	}
	out := make([]ApiKeyEntry, len(cfg.ApiKeys))
	copy(out, cfg.ApiKeys)
	return out
}

// GetApiKeyEntry returns a copy of the entry with the given ID, or nil if not found.
func GetApiKeyEntry(id string) *ApiKeyEntry {
	cfgLock.RLock()
	defer cfgLock.RUnlock()
	if cfg == nil {
		return nil
	}
	for i := range cfg.ApiKeys {
		if cfg.ApiKeys[i].ID == id {
			cp := cfg.ApiKeys[i]
			return &cp
		}
	}
	return nil
}

// AddApiKey appends a new API key entry. Generates ID and CreatedAt if missing,
// rejects empty Key values, and refuses duplicates of an existing Key.
func AddApiKey(entry ApiKeyEntry) (ApiKeyEntry, error) {
	cfgLock.Lock()
	defer cfgLock.Unlock()
	if cfg == nil {
		return ApiKeyEntry{}, errors.New("config not initialized")
	}
	entry.Key = strings.TrimSpace(entry.Key)
	if entry.Key == "" {
		return ApiKeyEntry{}, errors.New("api key value must not be empty")
	}
	for _, existing := range cfg.ApiKeys {
		if existing.Key == entry.Key {
			return ApiKeyEntry{}, errors.New("api key already exists")
		}
	}
	if entry.ID == "" {
		entry.ID = newUUID()
	}
	if entry.CreatedAt == 0 {
		entry.CreatedAt = time.Now().Unix()
	}
	// Arm the lifetime clock when created enabled with a TTL; otherwise the clock
	// stays stopped (ExpiresAt = 0) until the key is enabled.
	if entry.Enabled && entry.LifetimeSeconds > 0 {
		entry.ExpiresAt = time.Now().Unix() + entry.LifetimeSeconds
	} else {
		entry.ExpiresAt = 0
	}
	cfg.ApiKeys = append(cfg.ApiKeys, entry)
	if err := saveLocked(); err != nil {
		// Roll back the in-memory append so we don't leave inconsistent state.
		cfg.ApiKeys = cfg.ApiKeys[:len(cfg.ApiKeys)-1]
		return ApiKeyEntry{}, err
	}
	return entry, nil
}

// UpdateApiKey applies a patch to an existing API key. Patch semantics:
//   - Name, Key are overwritten when non-empty in patch.
//   - Enabled, TokenLimit, CreditLimit are always overwritten (zero values are valid).
//   - Counters (TokensUsed/CreditsUsed/RequestsCount) are not touched here; use
//     RecordApiKeyUsage or ResetApiKeyUsage instead.
//   - Migrated stays as-is once true; only flips when explicitly set in patch.
func UpdateApiKey(id string, patch ApiKeyEntry) error {
	cfgLock.Lock()
	defer cfgLock.Unlock()
	if cfg == nil {
		return errors.New("config not initialized")
	}
	idx := -1
	for i := range cfg.ApiKeys {
		if cfg.ApiKeys[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return errors.New("api key not found")
	}
	if patch.Name != "" {
		cfg.ApiKeys[idx].Name = patch.Name
	}
	if patch.Key != "" {
		newKey := strings.TrimSpace(patch.Key)
		// Reject duplicates against any other entry.
		for j := range cfg.ApiKeys {
			if j != idx && cfg.ApiKeys[j].Key == newKey {
				return errors.New("api key value collides with existing entry")
			}
		}
		cfg.ApiKeys[idx].Key = newKey
	}
	wasEnabled := cfg.ApiKeys[idx].Enabled
	oldLifetime := cfg.ApiKeys[idx].LifetimeSeconds

	cfg.ApiKeys[idx].Enabled = patch.Enabled
	cfg.ApiKeys[idx].TokenLimit = patch.TokenLimit
	cfg.ApiKeys[idx].CreditLimit = patch.CreditLimit
	cfg.ApiKeys[idx].LifetimeSeconds = patch.LifetimeSeconds
	if patch.Migrated {
		cfg.ApiKeys[idx].Migrated = true
	}

	// Lifetime clock transitions:
	//   - disabled (manual or staying off) -> stop the clock and reset to 0.
	//   - disabled->enabled                -> arm a fresh full TTL from now.
	//   - staying enabled, TTL changed     -> re-arm from now with the new TTL.
	//   - staying enabled, TTL unchanged   -> leave the running deadline as-is.
	now := time.Now().Unix()
	switch {
	case !cfg.ApiKeys[idx].Enabled:
		cfg.ApiKeys[idx].ExpiresAt = 0
	case !wasEnabled || cfg.ApiKeys[idx].LifetimeSeconds != oldLifetime:
		if cfg.ApiKeys[idx].LifetimeSeconds > 0 {
			cfg.ApiKeys[idx].ExpiresAt = now + cfg.ApiKeys[idx].LifetimeSeconds
		} else {
			cfg.ApiKeys[idx].ExpiresAt = 0
		}
	}

	return saveLocked()
}

// RestartApiKeyLifetime re-enables the key (if needed) and arms a fresh full TTL
// counting from now. Used by the "restart" admin action. If the key has no
// LifetimeSeconds configured, it is simply enabled with the clock stopped.
func RestartApiKeyLifetime(id string) error {
	cfgLock.Lock()
	defer cfgLock.Unlock()
	if cfg == nil {
		return errors.New("config not initialized")
	}
	for i := range cfg.ApiKeys {
		if cfg.ApiKeys[i].ID == id {
			cfg.ApiKeys[i].Enabled = true
			if cfg.ApiKeys[i].LifetimeSeconds > 0 {
				cfg.ApiKeys[i].ExpiresAt = time.Now().Unix() + cfg.ApiKeys[i].LifetimeSeconds
			} else {
				cfg.ApiKeys[i].ExpiresAt = 0
			}
			return saveLocked()
		}
	}
	return errors.New("api key not found")
}

// ExtendApiKeyLifetime pushes the deadline out by the given number of days.
// Thin wrapper over ExtendApiKeyLifetimeSeconds kept for callers that work in days.
func ExtendApiKeyLifetime(id string, days int64) error {
	return ExtendApiKeyLifetimeSeconds(id, days*86400)
}

// ExtendApiKeyLifetimeSeconds pushes the running deadline out by the given number
// of seconds. When the clock is currently stopped (ExpiresAt == 0), the extension
// is counted from now so the key gets a live deadline. The configured
// LifetimeSeconds is left unchanged — this only moves the running deadline.
// Rejects non-positive durations.
func ExtendApiKeyLifetimeSeconds(id string, seconds int64) error {
	if seconds <= 0 {
		return errors.New("duration must be positive")
	}
	cfgLock.Lock()
	defer cfgLock.Unlock()
	if cfg == nil {
		return errors.New("config not initialized")
	}
	for i := range cfg.ApiKeys {
		if cfg.ApiKeys[i].ID == id {
			base := cfg.ApiKeys[i].ExpiresAt
			if base == 0 {
				base = time.Now().Unix()
			}
			cfg.ApiKeys[i].ExpiresAt = base + seconds
			return saveLocked()
		}
	}
	return errors.New("api key not found")
}

// SetApiKeyExpiresAt overwrites the running deadline (Unix seconds) for the entry
// without touching LifetimeSeconds or Enabled. A value of 0 stops the clock.
// Primarily used to seed deterministic deadlines in tests.
func SetApiKeyExpiresAt(id string, expiresAt int64) error {
	cfgLock.Lock()
	defer cfgLock.Unlock()
	if cfg == nil {
		return errors.New("config not initialized")
	}
	for i := range cfg.ApiKeys {
		if cfg.ApiKeys[i].ID == id {
			cfg.ApiKeys[i].ExpiresAt = expiresAt
			return saveLocked()
		}
	}
	return errors.New("api key not found")
}

// DisableExpiredApiKeys scans for enabled keys whose deadline has passed, disables
// them, stops their clock (ExpiresAt = 0), and persists once if anything changed.
// Returns the IDs that were disabled so the caller can log them. Auth already
// rejects expired keys synchronously; this loop flips the persisted flag so the
// admin UI reflects reality and the change survives restarts.
func DisableExpiredApiKeys() []string {
	cfgLock.Lock()
	defer cfgLock.Unlock()
	if cfg == nil {
		return nil
	}
	now := time.Now().Unix()
	var disabled []string
	for i := range cfg.ApiKeys {
		e := &cfg.ApiKeys[i]
		if e.Enabled && e.ExpiresAt > 0 && now >= e.ExpiresAt {
			e.Enabled = false
			e.ExpiresAt = 0
			disabled = append(disabled, e.ID)
		}
	}
	if len(disabled) > 0 {
		if err := saveLocked(); err != nil {
			// Persist failed; the in-memory flip stands but won't survive restart.
			// Caller logs the IDs regardless.
			return disabled
		}
	}
	return disabled
}

// DeleteApiKey removes the API key entry with the given ID. Returns nil even if
// the ID is unknown (idempotent), matching the existing DeleteAccount style.
func DeleteApiKey(id string) error {
	cfgLock.Lock()
	defer cfgLock.Unlock()
	if cfg == nil {
		return errors.New("config not initialized")
	}
	for i, e := range cfg.ApiKeys {
		if e.ID == id {
			cfg.ApiKeys = append(cfg.ApiKeys[:i], cfg.ApiKeys[i+1:]...)
			return saveLocked()
		}
	}
	return nil
}

// FindApiKeyByValue returns a copy of the entry whose Key matches the given value,
// or nil if no match. O(n) linear scan.
func FindApiKeyByValue(key string) *ApiKeyEntry {
	cfgLock.RLock()
	defer cfgLock.RUnlock()
	if cfg == nil || key == "" {
		return nil
	}
	for i := range cfg.ApiKeys {
		if cfg.ApiKeys[i].Key == key {
			cp := cfg.ApiKeys[i]
			return &cp
		}
	}
	return nil
}

// HasApiKeys returns true when at least one API key entry is configured.
func HasApiKeys() bool {
	cfgLock.RLock()
	defer cfgLock.RUnlock()
	if cfg == nil {
		return false
	}
	return len(cfg.ApiKeys) > 0
}

// RecordApiKeyUsage atomically adds tokens and credits to the entry's counters,
// updates LastUsedAt, increments RequestsCount, and persists.
func RecordApiKeyUsage(id string, tokens int64, credits float64) error {
	cfgLock.Lock()
	defer cfgLock.Unlock()
	if cfg == nil {
		return errors.New("config not initialized")
	}
	for i := range cfg.ApiKeys {
		if cfg.ApiKeys[i].ID == id {
			if tokens > 0 {
				cfg.ApiKeys[i].TokensUsed += tokens
			}
			if credits > 0 {
				cfg.ApiKeys[i].CreditsUsed += credits
			}
			cfg.ApiKeys[i].RequestsCount++
			cfg.ApiKeys[i].LastUsedAt = time.Now().Unix()
			return saveLocked()
		}
	}
	return errors.New("api key not found")
}

// ResetApiKeyUsage clears TokensUsed/CreditsUsed/RequestsCount for the entry.
// LastUsedAt is preserved so operators can still see when the key was last used.
func ResetApiKeyUsage(id string) error {
	cfgLock.Lock()
	defer cfgLock.Unlock()
	if cfg == nil {
		return errors.New("config not initialized")
	}
	for i := range cfg.ApiKeys {
		if cfg.ApiKeys[i].ID == id {
			cfg.ApiKeys[i].TokensUsed = 0
			cfg.ApiKeys[i].CreditsUsed = 0
			cfg.ApiKeys[i].RequestsCount = 0
			return saveLocked()
		}
	}
	return errors.New("api key not found")
}

// GenerateApiKeyValue returns a new random 32-byte hex API key prefixed with "sk-".
func GenerateApiKeyValue() string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	return "sk-" + hex.EncodeToString(buf)
}

// MaskApiKey produces a display-friendly masked version: keeps first 6 and last 4
// characters, replaces the middle with "****". Returns "" for empty input and
// the original string if it's too short to mask meaningfully.
func MaskApiKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 10 {
		return key
	}
	return key[:6] + "****" + key[len(key)-4:]
}

// ApiKeyOverLimit returns (overToken, overCredit) for the entry. Limits with value 0
// are ignored. The function does not lock; callers should pass a copied entry.
func ApiKeyOverLimit(e ApiKeyEntry) (overToken bool, overCredit bool) {
	if e.TokenLimit > 0 && e.TokensUsed >= e.TokenLimit {
		overToken = true
	}
	if e.CreditLimit > 0 && e.CreditsUsed >= e.CreditLimit {
		overCredit = true
	}
	return
}
