package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type trajectoryCacheEntry struct {
	fetchedAt time.Time
	data      *upstreamTrajectoryResp
}

const maxTrajCacheSize = 50 // Maximum number of cached trajectory entries to prevent unbounded memory growth

// metadataCacheEntry caches the result of reading a .metadata.json file.
type metadataCacheEntry struct {
	requestFeedback bool
	fetchedAt       time.Time
}

// annotationTitleCacheEntry caches the parsed title from an annotation .pbtxt file.
type annotationTitleCacheEntry struct {
	title     string
	fetchedAt time.Time
}

const (
	maxMetadataMapSize = 256
	annotationCacheTTL = 5 * time.Second
)

// TrajectoryCache encapsulates all mutable trajectory-related state with proper synchronization.
// It replaces scattered package-level vars, enabling clean lifecycle management and testability.
type TrajectoryCache struct {
	trajCache   map[string]*trajectoryCacheEntry
	trajCacheMu sync.RWMutex

	cascadeTitles       map[string]string
	cascadeTitlesMu     sync.RWMutex
	lastTitlesFetchTime time.Time

	lastKnownConfig   json.RawMessage
	lastKnownConfigMu sync.RWMutex

	cascadeConfigs   map[string]json.RawMessage
	cascadeConfigsMu sync.RWMutex

	cascadeModels   map[string]string
	cascadeModelsMu sync.RWMutex

	loadedCascades   map[string]bool
	loadedCascadesMu sync.RWMutex
	lastSyncedPort   int

	deletedCascades   map[string]time.Time
	deletedCascadesMu sync.RWMutex

	// metadataCache caches .metadata.json requestFeedback results (TTL: 10s) to avoid
	// repeated os.ReadFile calls during the 250ms WebSocket stream polling loop.
	metadataCache   map[string]*metadataCacheEntry
	metadataCacheMu sync.RWMutex

	// annotationTitleCache caches parsed .pbtxt annotation titles (TTL: 5s) to eliminate
	// hot-path disk I/O during stream polling and status inquiries.
	annotationTitleCache   map[string]*annotationTitleCacheEntry
	annotationTitleCacheMu sync.RWMutex

	// inFlightTraj deduplicates concurrent requests for the same cascade trajectory
	inFlightTraj   map[string]*inFlightTraj
	inFlightTrajMu sync.Mutex
}

type inFlightTraj struct {
	wg  sync.WaitGroup
	res *upstreamTrajectoryResp
	err error
}

// NewTrajectoryCache creates a new TrajectoryCache with initialized maps.
func NewTrajectoryCache() *TrajectoryCache {
	return &TrajectoryCache{
		trajCache:            make(map[string]*trajectoryCacheEntry),
		cascadeTitles:        make(map[string]string),
		cascadeConfigs:       make(map[string]json.RawMessage),
		cascadeModels:        make(map[string]string),
		loadedCascades:       make(map[string]bool),
		deletedCascades:      make(map[string]time.Time),
		metadataCache:        make(map[string]*metadataCacheEntry),
		annotationTitleCache: make(map[string]*annotationTitleCacheEntry),
		inFlightTraj:         make(map[string]*inFlightTraj),
	}
}

// maxDeletedCascades is the upper bound on in-memory deletion tombstones.
const maxDeletedCascades = 1024

// RecordDeletedCascade marks a cascade as recently deleted with a TTL.
func RecordDeletedCascade(cascadeID string) {
	if cascadeID == "" {
		return
	}
	InvalidateCascadeValidCache(cascadeID)

	defaultTrajCache.cascadeModelsMu.Lock()
	if defaultTrajCache.cascadeModels != nil {
		delete(defaultTrajCache.cascadeModels, cascadeID)
	}
	defaultTrajCache.cascadeModelsMu.Unlock()

	defaultTrajCache.cascadeConfigsMu.Lock()
	if defaultTrajCache.cascadeConfigs != nil {
		delete(defaultTrajCache.cascadeConfigs, cascadeID)
	}
	defaultTrajCache.cascadeConfigsMu.Unlock()

	defaultTrajCache.deletedCascadesMu.Lock()
	defer defaultTrajCache.deletedCascadesMu.Unlock()
	// PERF-2: evict expired tombstones when map grows too large.
	if len(defaultTrajCache.deletedCascades) >= maxDeletedCascades {
		cutoff := time.Now().Add(-10 * time.Minute)
		for k, t := range defaultTrajCache.deletedCascades {
			if t.Before(cutoff) {
				delete(defaultTrajCache.deletedCascades, k)
			}
		}
	}
	defaultTrajCache.deletedCascades[cascadeID] = time.Now()
}

// RemoveDeletedCascadeTombstone removes a tombstone if deletion failed upstream.
func RemoveDeletedCascadeTombstone(cascadeID string) {
	if cascadeID == "" {
		return
	}
	defaultTrajCache.deletedCascadesMu.Lock()
	defer defaultTrajCache.deletedCascadesMu.Unlock()
	delete(defaultTrajCache.deletedCascades, cascadeID)
}

// IsDeletedCascade returns true if the cascade was recently deleted within tombstone TTL (10m).
func IsDeletedCascade(cascadeID string) bool {
	if cascadeID == "" {
		return false
	}
	defaultTrajCache.deletedCascadesMu.RLock()
	deletedAt, exists := defaultTrajCache.deletedCascades[cascadeID]
	defaultTrajCache.deletedCascadesMu.RUnlock()
	if !exists {
		return false
	}
	if time.Since(deletedAt) < 10*time.Minute {
		return true
	}
	// Expired tombstone, clean it up
	defaultTrajCache.deletedCascadesMu.Lock()
	delete(defaultTrajCache.deletedCascades, cascadeID)
	defaultTrajCache.deletedCascadesMu.Unlock()
	return false
}

// ResetHistoricalSyncState clears the loaded cascades map and resets the last synced port,
// allowing a fresh sync of all historical sessions from disk.
func ResetHistoricalSyncState() {
	defaultTrajCache.loadedCascadesMu.Lock()
	defer defaultTrajCache.loadedCascadesMu.Unlock()
	defaultTrajCache.loadedCascades = make(map[string]bool)
	defaultTrajCache.lastSyncedPort = 0
}

// HasSyncedHistoricalTrajectories returns whether historical trajectories have already been synced for this port.
func HasSyncedHistoricalTrajectories(port int) bool {
	defaultTrajCache.loadedCascadesMu.RLock()
	defer defaultTrajCache.loadedCascadesMu.RUnlock()
	return port > 0 && port == defaultTrajCache.lastSyncedPort
}

// evictTrajCacheLocked removes the oldest entries when cache exceeds maxTrajCacheSize.
// MUST be called while holding tc.trajCacheMu.
// Uses sort-based batch removal: O(n log n) instead of O(n²).
func (tc *TrajectoryCache) evictTrajCacheLocked() {
	if len(tc.trajCache) <= maxTrajCacheSize {
		return
	}
	type keyTime struct {
		key string
		t   time.Time
	}
	items := make([]keyTime, 0, len(tc.trajCache))
	for k, v := range tc.trajCache {
		items = append(items, keyTime{key: k, t: v.fetchedAt})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].t.Before(items[j].t)
	})
	// Remove oldest entries until we're at the limit
	removeCount := len(tc.trajCache) - maxTrajCacheSize
	for i := 0; i < removeCount; i++ {
		delete(tc.trajCache, items[i].key)
	}
}

// defaultTrajCache is the package-level TrajectoryCache instance used by Proxy.
// It is initialized here and assigned as a field of Proxy in NewProxy.
var defaultTrajCache = NewTrajectoryCache()

// readMetadataRequestFeedback reads the requestFeedback field from a .metadata.json file,
// caching results for metadataCacheTTL to avoid repeated os.ReadFile calls during stream polling.
const metadataCacheTTL = 10 * time.Second

func readMetadataRequestFeedback(filePath string) bool {
	defaultTrajCache.metadataCacheMu.RLock()
	if entry, ok := defaultTrajCache.metadataCache[filePath]; ok {
		if time.Since(entry.fetchedAt) < metadataCacheTTL {
			defaultTrajCache.metadataCacheMu.RUnlock()
			return entry.requestFeedback
		}
	}
	defaultTrajCache.metadataCacheMu.RUnlock()

	result := false
	if metaData, err := os.ReadFile(filePath); err == nil {
		var meta struct {
			RequestFeedback bool `json:"requestFeedback"`
		}
		if err := json.Unmarshal(metaData, &meta); err == nil {
			result = meta.RequestFeedback
		}
	}
	defaultTrajCache.metadataCacheMu.Lock()
	if defaultTrajCache.metadataCache == nil {
		defaultTrajCache.metadataCache = make(map[string]*metadataCacheEntry)
	}
	// Evict stale entries if map is large
	if len(defaultTrajCache.metadataCache) > 256 {
		now := time.Now()
		for k, v := range defaultTrajCache.metadataCache {
			if now.Sub(v.fetchedAt) > metadataCacheTTL {
				delete(defaultTrajCache.metadataCache, k)
			}
		}
	}
	defaultTrajCache.metadataCache[filePath] = &metadataCacheEntry{
		requestFeedback: result,
		fetchedAt:       time.Now(),
	}
	defaultTrajCache.metadataCacheMu.Unlock()
	return result
}

// ClearTrajectoryCache invalidates cached trajectory for an updated cascade.
func ClearTrajectoryCache(cascadeID string) {
	if cascadeID == "" {
		return
	}
	defaultTrajCache.trajCacheMu.Lock()
	delete(defaultTrajCache.trajCache, cascadeID)
	defaultTrajCache.trajCacheMu.Unlock()

	defaultTrajCache.cascadeTitlesMu.Lock()
	delete(defaultTrajCache.cascadeTitles, cascadeID)
	defaultTrajCache.lastTitlesFetchTime = time.Time{}
	defaultTrajCache.cascadeTitlesMu.Unlock()

	defaultTrajCache.loadedCascadesMu.Lock()
	delete(defaultTrajCache.loadedCascades, cascadeID)
	defaultTrajCache.loadedCascadesMu.Unlock()

	defaultTrajCache.annotationTitleCacheMu.Lock()
	if defaultTrajCache.annotationTitleCache != nil {
		delete(defaultTrajCache.annotationTitleCache, cascadeID)
	}
	defaultTrajCache.annotationTitleCacheMu.Unlock()

	InvalidateCascadeValidCache(cascadeID)
}

func readAnnotationTitle(cascadeID string) string {
	if cascadeID == "" {
		return ""
	}

	// 1. Check in-memory annotation title cache (TTL: 5s)
	defaultTrajCache.annotationTitleCacheMu.RLock()
	if entry, ok := defaultTrajCache.annotationTitleCache[cascadeID]; ok {
		if time.Since(entry.fetchedAt) < annotationCacheTTL {
			defaultTrajCache.annotationTitleCacheMu.RUnlock()
			return entry.title
		}
	}
	defaultTrajCache.annotationTitleCacheMu.RUnlock()

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	p := filepath.Join(home, ".gemini", "antigravity", "annotations", cascadeID+".pbtxt")
	b, err := os.ReadFile(p)
	title := ""
	if err == nil {
		m := titleRegex.FindSubmatch(b)
		if len(m) > 1 {
			title = string(m[1])
		}
	}

	// Update cache
	defaultTrajCache.annotationTitleCacheMu.Lock()
	if defaultTrajCache.annotationTitleCache == nil {
		defaultTrajCache.annotationTitleCache = make(map[string]*annotationTitleCacheEntry)
	}
	if len(defaultTrajCache.annotationTitleCache) > maxMetadataMapSize {
		cutoff := time.Now().Add(-annotationCacheTTL)
		for k, v := range defaultTrajCache.annotationTitleCache {
			if v.fetchedAt.Before(cutoff) {
				delete(defaultTrajCache.annotationTitleCache, k)
			}
		}
	}
	defaultTrajCache.annotationTitleCache[cascadeID] = &annotationTitleCacheEntry{
		title:     title,
		fetchedAt: time.Now(),
	}
	defaultTrajCache.annotationTitleCacheMu.Unlock()

	return title
}

func writeAnnotationTitle(cascadeID, title string) {
	if cascadeID == "" || strings.TrimSpace(title) == "" || title == "未命名会话" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".gemini", "antigravity", "annotations")
	_ = os.MkdirAll(dir, 0700)
	p := filepath.Join(dir, cascadeID+".pbtxt")
	b, err := os.ReadFile(p)
	if err != nil {
		content := fmt.Sprintf("title: %q\n", title)
		// SEC-8: 0600 — annotation files contain conversation titles, owner-only.
		_ = os.WriteFile(p, []byte(content), 0600)
	} else {
		s := string(b)
		if titleRegex.MatchString(s) {
			newContent := titleRegex.ReplaceAllString(s, fmt.Sprintf("title: %q", title))
			_ = os.WriteFile(p, []byte(newContent), 0600)
		} else {
			newContent := fmt.Sprintf("title: %q\n%s", title, s)
			_ = os.WriteFile(p, []byte(newContent), 0600)
		}
	}

	// Immediately update annotationTitleCache
	defaultTrajCache.annotationTitleCacheMu.Lock()
	if defaultTrajCache.annotationTitleCache != nil {
		defaultTrajCache.annotationTitleCache[cascadeID] = &annotationTitleCacheEntry{
			title:     title,
			fetchedAt: time.Now(),
		}
	}
	defaultTrajCache.annotationTitleCacheMu.Unlock()
}
