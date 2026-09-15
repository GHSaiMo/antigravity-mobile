package auth

import (
	"sync"
	"time"
)

// RateLimiter is a small in-memory sliding-window limiter keyed by string (usually IP + route).
type RateLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{hits: make(map[string][]time.Time)}
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
