package proxy

import (
	"sync"
	"time"
)

// apiKeyRateLimiter enforces per-API-key request rate limits (requests/minute and
// requests/day) using purely in-memory counters. Nothing is persisted: counts are
// reset on restart, which is acceptable for spam protection and keeps memory and
// disk overhead near zero (Fly.io 256MB target).
//
// Counters reset on calendar boundaries, not sliding windows:
//   - the minute counter resets at the top of each wall-clock minute,
//   - the day counter resets at local midnight (00:00).
//
// Resets are lazy: each Allow/Snapshot call compares the current bucket id with
// the stored one and zeroes the count when they differ. No background goroutine
// is needed. A counter is created only when a key actually receives a request, so
// idle keys cost nothing (~48 bytes per active key).
type apiKeyRateLimiter struct {
	mu       sync.Mutex
	counters map[string]*rateCounter
}

// rateCounter holds the live counts plus the bucket ids they belong to. When the
// computed bucket id no longer matches, the corresponding count is stale and gets
// reset to zero on next access.
type rateCounter struct {
	minuteBucket int64 // unix-minute (now/60) the minuteCount belongs to
	minuteCount  int64
	dayBucket    int64 // local YYYYMMDD the dayCount belongs to
	dayCount     int64
}

func newAPIKeyRateLimiter() *apiKeyRateLimiter {
	return &apiKeyRateLimiter{counters: make(map[string]*rateCounter)}
}

// minuteBucketID returns the wall-clock minute bucket for t (unix seconds / 60).
func minuteBucketID(t time.Time) int64 { return t.Unix() / 60 }

// dayBucketID returns the local-calendar day bucket for t as YYYYMMDD, so the day
// counter resets at local midnight regardless of timezone offset.
func dayBucketID(t time.Time) int64 {
	y, m, d := t.Date()
	return int64(y)*10000 + int64(m)*100 + int64(d)
}

// Allow reports whether a request for the given key may proceed under the supplied
// limits, and records the request when it does. A limit of 0 means "unlimited" for
// that dimension. The second return value is the scope that was exceeded
// ("per-minute" or "per-day") when ok is false, for the error message.
//
// Both limits are checked before either counter is incremented, so a request that
// would exceed a limit is rejected without consuming quota in the other dimension.
func (l *apiKeyRateLimiter) Allow(id string, perMinute, perDay int64) (scope string, ok bool) {
	if l == nil || id == "" || (perMinute <= 0 && perDay <= 0) {
		return "", true
	}
	now := time.Now()
	minID := minuteBucketID(now)
	dayID := dayBucketID(now)

	l.mu.Lock()
	defer l.mu.Unlock()

	c := l.counters[id]
	if c == nil {
		c = &rateCounter{minuteBucket: minID, dayBucket: dayID}
		l.counters[id] = c
	}
	// Lazy calendar reset.
	if c.minuteBucket != minID {
		c.minuteBucket = minID
		c.minuteCount = 0
	}
	if c.dayBucket != dayID {
		c.dayBucket = dayID
		c.dayCount = 0
	}

	// Check both dimensions before mutating, so a rejection in one does not burn
	// quota in the other.
	if perMinute > 0 && c.minuteCount >= perMinute {
		return "per-minute", false
	}
	if perDay > 0 && c.dayCount >= perDay {
		return "per-day", false
	}

	c.minuteCount++
	c.dayCount++
	return "", true
}

// Snapshot returns the current (already calendar-adjusted) minute and day counts
// for a key without incrementing them. Used by the admin UI to show live usage.
// Returns (0, 0) for keys that have never been seen.
func (l *apiKeyRateLimiter) Snapshot(id string) (minuteCount, dayCount int64) {
	if l == nil || id == "" {
		return 0, 0
	}
	now := time.Now()
	minID := minuteBucketID(now)
	dayID := dayBucketID(now)

	l.mu.Lock()
	defer l.mu.Unlock()

	c := l.counters[id]
	if c == nil {
		return 0, 0
	}
	if c.minuteBucket != minID {
		minuteCount = 0
	} else {
		minuteCount = c.minuteCount
	}
	if c.dayBucket != dayID {
		dayCount = 0
	} else {
		dayCount = c.dayCount
	}
	return minuteCount, dayCount
}

// Retain drops counters for keys whose IDs are no longer present, preventing the
// map from growing unbounded as keys are deleted over the process lifetime. Called
// periodically from the background loop with the set of currently-configured IDs.
func (l *apiKeyRateLimiter) Retain(validIDs map[string]struct{}) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for id := range l.counters {
		if _, ok := validIDs[id]; !ok {
			delete(l.counters, id)
		}
	}
}
