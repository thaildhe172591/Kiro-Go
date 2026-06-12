package proxy

import (
	"encoding/json"
	"kiro-go/config"
	"net/http"
)

// apiKeyView is the response payload for listing/inspecting API keys. The Key field
// is masked so admins can identify entries without exposing the secret.
type apiKeyView struct {
	ID              string  `json:"id"`
	Name            string  `json:"name,omitempty"`
	KeyMasked       string  `json:"keyMasked"`
	Enabled         bool    `json:"enabled"`
	Migrated        bool    `json:"migrated,omitempty"`
	CreatedAt       int64   `json:"createdAt"`
	LastUsedAt      int64   `json:"lastUsedAt,omitempty"`
	LifetimeSeconds int64   `json:"lifetimeSeconds,omitempty"`
	ExpiresAt       int64   `json:"expiresAt,omitempty"`
	TokenLimit      int64   `json:"tokenLimit,omitempty"`
	CreditLimit     float64 `json:"creditLimit,omitempty"`
	TokensUsed      int64   `json:"tokensUsed"`
	CreditsUsed     float64 `json:"creditsUsed"`
	RequestsCount   int64   `json:"requestsCount"`

	// Rate limits (0 = unlimited) and their live in-memory usage counters.
	// RequestsThisMinute/RequestsToday are not persisted; they come from the
	// rate limiter and reset on calendar boundaries.
	RequestsPerMinute  int64 `json:"requestsPerMinute,omitempty"`
	RequestsPerDay     int64 `json:"requestsPerDay,omitempty"`
	RequestsThisMinute int64 `json:"requestsThisMinute"`
	RequestsToday      int64 `json:"requestsToday"`
}

func toApiKeyView(e config.ApiKeyEntry) apiKeyView {
	return apiKeyView{
		ID:                e.ID,
		Name:              e.Name,
		KeyMasked:         config.MaskApiKey(e.Key),
		Enabled:           e.Enabled,
		Migrated:          e.Migrated,
		CreatedAt:         e.CreatedAt,
		LastUsedAt:        e.LastUsedAt,
		LifetimeSeconds:   e.LifetimeSeconds,
		ExpiresAt:         e.ExpiresAt,
		TokenLimit:        e.TokenLimit,
		CreditLimit:       e.CreditLimit,
		TokensUsed:        e.TokensUsed,
		CreditsUsed:       e.CreditsUsed,
		RequestsCount:     e.RequestsCount,
		RequestsPerMinute: e.RequestsPerMinute,
		RequestsPerDay:    e.RequestsPerDay,
	}
}

// withLiveRateUsage fills the live (non-persisted) rate counters from the limiter.
func (h *Handler) withLiveRateUsage(v apiKeyView) apiKeyView {
	if h.apiKeyLimiter != nil {
		v.RequestsThisMinute, v.RequestsToday = h.apiKeyLimiter.Snapshot(v.ID)
	}
	return v
}

func (h *Handler) apiListApiKeys(w http.ResponseWriter, r *http.Request) {
	entries := config.ListApiKeys()
	out := make([]apiKeyView, len(entries))
	for i, e := range entries {
		out[i] = h.withLiveRateUsage(toApiKeyView(e))
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"apiKeys": out})
}

func (h *Handler) apiGetApiKey(w http.ResponseWriter, r *http.Request, id string) {
	entry := config.GetApiKeyEntry(id)
	if entry == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "API key not found"})
		return
	}
	json.NewEncoder(w).Encode(h.withLiveRateUsage(toApiKeyView(*entry)))
}

type apiKeyCreateRequest struct {
	Name              string  `json:"name,omitempty"`
	Key               string  `json:"key,omitempty"`
	Enabled           *bool   `json:"enabled,omitempty"`
	LifetimeSeconds   int64   `json:"lifetimeSeconds,omitempty"`
	TokenLimit        int64   `json:"tokenLimit,omitempty"`
	CreditLimit       float64 `json:"creditLimit,omitempty"`
	RequestsPerMinute int64   `json:"requestsPerMinute,omitempty"`
	RequestsPerDay    int64   `json:"requestsPerDay,omitempty"`
}

func (h *Handler) apiCreateApiKey(w http.ResponseWriter, r *http.Request) {
	var req apiKeyCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON"})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	keyValue := req.Key
	if keyValue == "" {
		keyValue = config.GenerateApiKeyValue()
	}

	lifetime := req.LifetimeSeconds
	if lifetime < 0 {
		lifetime = 0
	}

	entry, err := config.AddApiKey(config.ApiKeyEntry{
		Name:              req.Name,
		Key:               keyValue,
		Enabled:           enabled,
		LifetimeSeconds:   lifetime,
		TokenLimit:        req.TokenLimit,
		CreditLimit:       req.CreditLimit,
		RequestsPerMinute: req.RequestsPerMinute,
		RequestsPerDay:    req.RequestsPerDay,
	})
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Return the cleartext key exactly once on creation so the operator can copy it.
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"id":      entry.ID,
		"key":     entry.Key,
		"apiKey":  toApiKeyView(entry),
	})
}

type apiKeyUpdateRequest struct {
	Name              *string  `json:"name,omitempty"`
	Key               *string  `json:"key,omitempty"`
	Enabled           *bool    `json:"enabled,omitempty"`
	TokenLimit        *int64   `json:"tokenLimit,omitempty"`
	CreditLimit       *float64 `json:"creditLimit,omitempty"`
	LifetimeSeconds   *int64   `json:"lifetimeSeconds,omitempty"`
	RequestsPerMinute *int64   `json:"requestsPerMinute,omitempty"`
	RequestsPerDay    *int64   `json:"requestsPerDay,omitempty"`
}

func (h *Handler) apiUpdateApiKey(w http.ResponseWriter, r *http.Request, id string) {
	existing := config.GetApiKeyEntry(id)
	if existing == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "API key not found"})
		return
	}

	var req apiKeyUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON"})
		return
	}

	patch := *existing
	if req.Name != nil {
		patch.Name = *req.Name
	}
	if req.Key != nil {
		patch.Key = *req.Key
	}
	if req.Enabled != nil {
		patch.Enabled = *req.Enabled
	}
	if req.TokenLimit != nil {
		patch.TokenLimit = *req.TokenLimit
	}
	if req.CreditLimit != nil {
		patch.CreditLimit = *req.CreditLimit
	}
	if req.LifetimeSeconds != nil {
		patch.LifetimeSeconds = *req.LifetimeSeconds
	}
	if req.RequestsPerMinute != nil {
		patch.RequestsPerMinute = *req.RequestsPerMinute
	}
	if req.RequestsPerDay != nil {
		patch.RequestsPerDay = *req.RequestsPerDay
	}

	if err := config.UpdateApiKey(id, patch); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	updated := config.GetApiKeyEntry(id)
	if updated == nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to reload entry"})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"apiKey":  toApiKeyView(*updated),
	})
}

func (h *Handler) apiDeleteApiKey(w http.ResponseWriter, r *http.Request, id string) {
	if err := config.DeleteApiKey(id); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// apiRestartApiKeyLifetime re-enables the key and arms a fresh full TTL from now.
func (h *Handler) apiRestartApiKeyLifetime(w http.ResponseWriter, r *http.Request, id string) {
	if err := config.RestartApiKeyLifetime(id); err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	updated := config.GetApiKeyEntry(id)
	if updated == nil {
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"apiKey":  toApiKeyView(*updated),
	})
}

type apiKeyExtendRequest struct {
	Seconds int64 `json:"seconds,omitempty"` // Preferred: extension in seconds (enables minute-level granularity)
	Days    int64 `json:"days,omitempty"`    // Legacy: extension in whole days
}

// apiExtendApiKeyLifetime pushes the running deadline out. The client may send
// either "seconds" (preferred, minute-level granularity) or the legacy "days".
func (h *Handler) apiExtendApiKeyLifetime(w http.ResponseWriter, r *http.Request, id string) {
	var req apiKeyExtendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON"})
		return
	}
	seconds := req.Seconds
	if seconds == 0 {
		seconds = req.Days * 86400
	}
	if err := config.ExtendApiKeyLifetimeSeconds(id, seconds); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	updated := config.GetApiKeyEntry(id)
	if updated == nil {
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"apiKey":  toApiKeyView(*updated),
	})
}

func (h *Handler) apiResetApiKeyUsage(w http.ResponseWriter, r *http.Request, id string) {
	if err := config.ResetApiKeyUsage(id); err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	updated := config.GetApiKeyEntry(id)
	if updated == nil {
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"apiKey":  toApiKeyView(*updated),
	})
}
