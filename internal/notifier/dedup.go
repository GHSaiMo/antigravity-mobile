package notifier

import (
	"sync"
	"time"
)

type dedupEntry struct {
	notifiedAt time.Time
}

// DedupCache tracks recently sent notification event keys to prevent duplicates.
type DedupCache struct {
	mu      sync.RWMutex
	entries map[string]dedupEntry
}

// NewDedupCache creates an empty DedupCache.
func NewDedupCache() *DedupCache {
	return &DedupCache{
		entries: make(map[string]dedupEntry),
	}
}

// TryNotify checks if key has been notified within ttl.
// If it has not, it records the notification time and returns true.
// If it has already been notified within ttl, it returns false.
func (d *DedupCache) TryNotify(key string, ttl time.Duration) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	if entry, exists := d.entries[key]; exists {
		if now.Sub(entry.notifiedAt) < ttl {
			return false
		}
	}

	d.entries[key] = dedupEntry{notifiedAt: now}
	return true
}

// Record marks a key as notified at the current time without checking.
func (d *DedupCache) Record(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.entries[key] = dedupEntry{notifiedAt: time.Now()}
}

// Remove deletes a key from the dedup cache, allowing subsequent retries.
func (d *DedupCache) Remove(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.entries, key)
}

// IsNotified returns true if key was notified within ttl.
func (d *DedupCache) IsNotified(key string, ttl time.Duration) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if entry, exists := d.entries[key]; exists {
		return time.Since(entry.notifiedAt) < ttl
	}
	return false
}

// Cleanup removes entries older than maxAge.
func (d *DedupCache) Cleanup(maxAge time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	for k, v := range d.entries {
		if now.Sub(v.notifiedAt) > maxAge {
			delete(d.entries, k)
		}
	}
}
