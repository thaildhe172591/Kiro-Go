package proxy

import (
	"kiro-go/config"
	"net/http"
	"testing"
)

// A key with RequestsPerMinute=2 must authenticate twice then get a 429 on the
// third call within the same minute. Exercises the enforcement path wired into
// authenticate via the handler's rate limiter.
func TestAuthenticateEnforcesPerMinuteLimit(t *testing.T) {
	mustInitConfig(t)
	if _, err := config.AddApiKey(config.ApiKeyEntry{
		Name: "rpm", Key: "sk-rpm", Enabled: true, RequestsPerMinute: 2,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	requireAuth(t)

	h := &Handler{apiKeyLimiter: newAPIKeyRateLimiter()}

	for i := 0; i < 2; i++ {
		r := newAuthTestRequest(t, "Authorization", "Bearer sk-rpm")
		if _, err := h.authenticate(r); err != nil {
			t.Fatalf("request %d should pass, got %v", i+1, err)
		}
	}

	// Third request in the same minute must be rejected with 429.
	r := newAuthTestRequest(t, "Authorization", "Bearer sk-rpm")
	_, err := h.authenticate(r)
	if err == nil {
		t.Fatalf("expected third request to be rate limited")
	}
	ae, ok := err.(*authError)
	if !ok || ae.status != http.StatusTooManyRequests {
		t.Fatalf("expected 429 authError, got %T %v", err, err)
	}
}

// A nil limiter (the zero-value Handler used by many existing tests) must never
// rate-limit, so legacy tests keep passing.
func TestAuthenticateNilLimiterSkipsRateLimit(t *testing.T) {
	mustInitConfig(t)
	if _, err := config.AddApiKey(config.ApiKeyEntry{
		Name: "nil", Key: "sk-nil", Enabled: true, RequestsPerMinute: 1,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	requireAuth(t)

	h := &Handler{} // apiKeyLimiter is nil
	for i := 0; i < 5; i++ {
		r := newAuthTestRequest(t, "Authorization", "Bearer sk-nil")
		if _, err := h.authenticate(r); err != nil {
			t.Fatalf("nil limiter must not rate-limit, request %d got %v", i+1, err)
		}
	}
}
