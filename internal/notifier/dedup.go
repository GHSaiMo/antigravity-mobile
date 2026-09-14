package notifier

import (
	"sort"
	"sync"
	"time"
)

const defaultMaxDedupEntries = 2000

type dedupEntry struct {
	notifiedAt time.Time
}

// DedupCache tracks recently sent notification event keys to prevent duplicates.
type DedupCache struct {
	mu         sync.RWMutex
	entries    map[string]dedupEntry
	maxEntries int
}

// NewDedupCache creates an empty DedupCache with default max capacity.
func NewDedupCache() *DedupCache {
	return NewDedupCacheWithCapacity(defaultMaxDedupEntries)
}

// NewDedupCacheWithCapacity creates an empty DedupCache with custom max capacity.
func NewDedupCacheWithCapacity(maxEntries int) *DedupCache {
	if maxEntries <= 0 {
		maxEntries = defaultMaxDedupEntries
	}
	return &DedupCache{
		entries:    make(map[string]dedupEntry),
		maxEntries: maxEntries,
	}
}

func (d *DedupCache) evictIfFullLocked() {
	if len(d.entries) < d.maxEntries {
		return
	}
	// First pass: remove entries older than 2 hours
	now := time.Now()
	for k, v := range d.entries {
		if now.Sub(v.notifiedAt) > 2*time.Hour {
			delete(d.entries, k)
		}
	}
	if len(d.entries) < d.maxEntries {
		return
	}

	// Second pass: remove oldest entries until we are under 80% capacity
	type keyTime struct {
		key string
		t   time.Time
	}
	items := make([]keyTime, 0, len(d.entries))
	for k, v := range d.entries {
		items = append(items, keyTime{key: k, t: v.notifiedAt})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].t.Before(items[j].t)
	})
	target := d.maxEntries * 4 / 5
	removeCount := len(d.entries) - target
	for i := 0; i < removeCount && i < len(items); i++ {
		delete(d.entries, items[i].key)
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

	d.evictIfFullLocked()
	d.entries[key] = dedupEntry{notifiedAt: now}
	return true
}

// Record marks a key as notified at the current time without checking.
func (d *DedupCache) Record(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.evictIfFullLocked()
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

// Len returns the current number of entries in the dedup cache.
func (d *DedupCache) Len() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.entries)
}

