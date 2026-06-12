package proxy

import (
	"kiro-go/config"
	"net/http"
	"strings"
	"testing"
	"time"
)

// A key whose ExpiresAt deadline has passed must be rejected synchronously by
// authenticate, before the background expiry loop has a chance to flip Enabled.
func TestAuthenticateRejectsExpiredKey(t *testing.T) {
	mustInitConfig(t)
	created, err := config.AddApiKey(config.ApiKeyEntry{Name: "exp", Key: "sk-exp", Enabled: true})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Force a deadline in the past while keeping the key enabled.
	if err := config.SetApiKeyExpiresAt(created.ID, time.Now().Unix()-1); err != nil {
		t.Fatalf("set expiry: %v", err)
	}
	requireAuth(t)

	h := &Handler{}
	r := newAuthTestRequest(t, "Authorization", "Bearer sk-exp")
	entry, err := h.authenticate(r)
	if err == nil {
		t.Fatalf("expected expired key to be rejected, got entry=%v", entry)
	}
	ae, ok := err.(*authError)
	if !ok || ae.status != http.StatusUnauthorized {
		t.Fatalf("expected 401 authError, got %v", err)
	}
	if !strings.Contains(ae.message, "expired") {
		t.Fatalf("expected expired message, got %q", ae.message)
	}
}

// A key with a future deadline still authenticates normally.
func TestAuthenticateAcceptsNotYetExpiredKey(t *testing.T) {
	mustInitConfig(t)
	created, err := config.AddApiKey(config.ApiKeyEntry{Name: "live", Key: "sk-live", Enabled: true})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := config.SetApiKeyExpiresAt(created.ID, time.Now().Unix()+3600); err != nil {
		t.Fatalf("set expiry: %v", err)
	}
	requireAuth(t)

	h := &Handler{}
	r := newAuthTestRequest(t, "Authorization", "Bearer sk-live")
	entry, err := h.authenticate(r)
	if err != nil {
		t.Fatalf("expected live key to authenticate, got err=%v", err)
	}
	if entry == nil || entry.ID != created.ID {
		t.Fatalf("expected entry to match seeded key, got %v", entry)
	}
}
