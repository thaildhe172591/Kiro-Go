package proxy

import "testing"

// A nil limiter (e.g. Handler{} in older tests) must never block.
func TestRateLimiterNilSafe(t *testing.T) {
	var l *apiKeyRateLimiter
	if scope, ok := l.Allow("x", 1, 1); !ok || scope != "" {
		t.Fatalf("nil limiter should allow, got scope=%q ok=%v", scope, ok)
	}
	if m, d := l.Snapshot("x"); m != 0 || d != 0 {
		t.Fatalf("nil limiter snapshot should be zero, got %d/%d", m, d)
	}
	l.Retain(map[string]struct{}{}) // must not panic
}

// Zero limits mean unlimited: Allow never blocks and never allocates a counter.
func TestRateLimiterUnlimited(t *testing.T) {
	l := newAPIKeyRateLimiter()
	for i := 0; i < 1000; i++ {
		if scope, ok := l.Allow("k", 0, 0); !ok {
			t.Fatalf("unlimited key blocked at i=%d scope=%q", i, scope)
		}
	}
	if m, d := l.Snapshot("k"); m != 0 || d != 0 {
		t.Fatalf("unlimited key should not accumulate counts, got %d/%d", m, d)
	}
}

// The per-minute limit blocks once the count reaches the cap within a bucket.
func TestRateLimiterPerMinuteBlocks(t *testing.T) {
	l := newAPIKeyRateLimiter()
	// Allow 3/min, unlimited/day.
	for i := 0; i < 3; i++ {
		if _, ok := l.Allow("k", 3, 0); !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	scope, ok := l.Allow("k", 3, 0)
	if ok {
		t.Fatalf("4th request should be blocked")
	}
	if scope != "per-minute" {
		t.Fatalf("expected per-minute scope, got %q", scope)
	}
}

// The per-day limit blocks independently of the minute limit.
func TestRateLimiterPerDayBlocks(t *testing.T) {
	l := newAPIKeyRateLimiter()
	for i := 0; i < 2; i++ {
		if _, ok := l.Allow("k", 0, 2); !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	scope, ok := l.Allow("k", 0, 2)
	if ok {
		t.Fatalf("3rd request should be blocked by daily cap")
	}
	if scope != "per-day" {
		t.Fatalf("expected per-day scope, got %q", scope)
	}
}

// A rejected request must not consume quota in the other dimension: when the
// minute cap is hit, the day counter stays where it was.
func TestRateLimiterRejectionDoesNotBurnOtherDimension(t *testing.T) {
	l := newAPIKeyRateLimiter()
	// minute cap 1, day cap 10.
	if _, ok := l.Allow("k", 1, 10); !ok {
		t.Fatalf("first request should pass")
	}
	if _, ok := l.Allow("k", 1, 10); ok {
		t.Fatalf("second request should be blocked by per-minute")
	}
	_, day := l.Snapshot("k")
	if day != 1 {
		t.Fatalf("day count should stay at 1 after a per-minute rejection, got %d", day)
	}
}

// A changed minute bucket resets the minute count (lazy calendar reset). We
// simulate this by mutating the stored bucket id to a stale value.
func TestRateLimiterMinuteBucketResets(t *testing.T) {
	l := newAPIKeyRateLimiter()
	if _, ok := l.Allow("k", 5, 0); !ok {
		t.Fatalf("first request should pass")
	}
	// Force the stored minute bucket into the past so the next Allow resets it.
	l.mu.Lock()
	l.counters["k"].minuteBucket -= 1
	l.mu.Unlock()

	if _, ok := l.Allow("k", 5, 0); !ok {
		t.Fatalf("request after bucket rollover should pass")
	}
	min, _ := l.Snapshot("k")
	if min != 1 {
		t.Fatalf("minute count should be 1 after reset+1, got %d", min)
	}
}

// Retain drops counters for keys no longer present.
func TestRateLimiterRetain(t *testing.T) {
	l := newAPIKeyRateLimiter()
	l.Allow("keep", 10, 0)
	l.Allow("drop", 10, 0)
	l.Retain(map[string]struct{}{"keep": {}})
	if _, ok := l.counters["drop"]; ok {
		t.Fatalf("expected 'drop' counter to be removed")
	}
	if _, ok := l.counters["keep"]; !ok {
		t.Fatalf("expected 'keep' counter to survive")
	}
}
