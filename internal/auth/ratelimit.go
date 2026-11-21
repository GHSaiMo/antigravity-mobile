package auth

import (
	"sync"
	"time"
)

// maxRateLimiterEntries caps the number of keys held in memory to prevent memory
// exhaustion under IP rotation or high-cardinality distributed attacks.
const maxRateLimiterEntries = 10000

type rateBucket struct {
	hits      []time.Time
	expiresAt time.Time
}

// RateLimiter is a small in-memory sliding-window limiter keyed by string (usually IP + route or deviceID).
type RateLimiter struct {
	mu         sync.Mutex
	buckets    map[string]rateBucket
	maxEntries int
	stopCh     chan struct{}
	closeOnce  sync.Once
}

func NewRateLimiter() *RateLimiter {
	l := &RateLimiter{
		buckets:    make(map[string]rateBucket),
		maxEntries: maxRateLimiterEntries,
		stopCh:     make(chan struct{}),
	}
	go l.gc()
	return l
}

// Close stops the background cleanup goroutine and releases its ticker.
func (l *RateLimiter) Close() {
	if l == nil || l.stopCh == nil {
		return
	}
	l.closeOnce.Do(func() {
		close(l.stopCh)
	})
}

// gc periodically removes keys whose actual sliding window has expired,
// preventing unbounded memory growth or 5x key retention inflation.
func (l *RateLimiter) gc() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-l.stopCh:
			return
		case now := <-ticker.C:
			l.mu.Lock()
			for k, b := range l.buckets {
				if len(b.hits) == 0 || !now.Before(b.expiresAt) {
					delete(l.buckets, k)
				}
			}
			l.mu.Unlock()
		}
	}
}

// Allow reports whether key may proceed, keeping at most max events in window.
func (l *RateLimiter) Allow(key string, max int, window time.Duration) bool {
	if l == nil || max <= 0 {
		return true
	}
	if window <= 0 {
		window = time.Minute
	}
	now := time.Now()
	cutoff := now.Add(-window)

	l.mu.Lock()
	defer l.mu.Unlock()

	limitMax := l.maxEntries
	if limitMax <= 0 {
		limitMax = maxRateLimiterEntries
	}

	b, exists := l.buckets[key]
	if !exists && len(l.buckets) >= limitMax {
		// Capacity reached: evict entries whose actual window has already expired.
		for k, entry := range l.buckets {
			if len(entry.hits) == 0 || !now.Before(entry.expiresAt) {
				delete(l.buckets, k)
			}
		}
		// If still at or above capacity, deny new keys to prevent OOM.
		if len(l.buckets) >= limitMax {
			return false
		}
	}

	kept := b.hits[:0]
	for _, ts := range b.hits {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= max {
		b.hits = kept
		if len(kept) > 0 {
			b.expiresAt = kept[len(kept)-1].Add(window)
		}
		l.buckets[key] = b
		return false
	}
	kept = append(kept, now)
	l.buckets[key] = rateBucket{
		hits:      kept,
		expiresAt: now.Add(window),
	}
	return true
}
