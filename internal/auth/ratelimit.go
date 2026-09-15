package auth

import (
	"sync"
	"time"
)

// maxRateLimitWindow is the largest window we ever use; keys silent longer than
// this can be safely evicted from the hits map.
const maxRateLimitWindow = 5 * time.Minute

// RateLimiter is a small in-memory sliding-window limiter keyed by string (usually IP + route).
type RateLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func NewRateLimiter() *RateLimiter {
	l := &RateLimiter{hits: make(map[string][]time.Time)}
	go l.gc()
	return l
}

// gc periodically removes keys whose last hit is older than maxRateLimitWindow,
// preventing unbounded memory growth under IP-rotation or high-cardinality key attacks.
func (l *RateLimiter) gc() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-maxRateLimitWindow)
		l.mu.Lock()
		for k, ts := range l.hits {
			// If the most recent hit is older than the max window, the key is fully expired.
			if len(ts) == 0 || ts[len(ts)-1].Before(cutoff) {
				delete(l.hits, k)
			}
		}
		l.mu.Unlock()
	}
}

// Allow reports whether key may proceed, keeping at most max events in window.
func (l *RateLimiter) Allow(key string, max int, window time.Duration) bool {
	if l == nil || max <= 0 {
		return true
	}
	now := time.Now()
	cutoff := now.Add(-window)

	l.mu.Lock()
	defer l.mu.Unlock()

	q := l.hits[key]
	kept := q[:0]
	for _, ts := range q {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= max {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}
