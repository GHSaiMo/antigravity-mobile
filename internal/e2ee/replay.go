package e2ee

import (
	"container/list"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultReplayWindow is the allowed absolute clock skew between client and gateway timestamps.
	DefaultReplayWindow = 5 * time.Minute
	// DefaultReplayMaxEntries bounds memory usage of the sliding replay deduplication LRU cache.
	DefaultReplayMaxEntries = 16384

	ErrReplayRequired  = "e2ee_replay_required"
	ErrReplayInvalidID = "e2ee_replay_invalid_id"
	ErrReplayClockSkew = "e2ee_replay_clock_skew"
	ErrReplayDetected  = "e2ee_replay_detected"
)

type replayEntry struct {
	key       string
	expiresAt time.Time
}

// ReplayGuard enforces a timestamp acceptance window plus bounded sliding-window LRU deduplication
// over authenticated (deviceID, reqID) pairs to block active ciphertext replay attacks.
type ReplayGuard struct {
	window     time.Duration
	ttl        time.Duration
	maxEntries int
	nowFunc    func() time.Time

	mu    sync.Mutex
	order *list.List               // front = oldest, back = newest
	index map[string]*list.Element // key -> element holding replayEntry
}

// NewReplayGuard creates a ReplayGuard with sensible defaults (±5m window, 10m TTL, 16384 max entries).
func NewReplayGuard() *ReplayGuard {
	return NewReplayGuardWithOptions(DefaultReplayWindow, DefaultReplayMaxEntries, nil)
}

// NewReplayGuardWithOptions creates a ReplayGuard with custom window, capacity, and optional clock.
func NewReplayGuardWithOptions(window time.Duration, maxEntries int, nowFunc func() time.Time) *ReplayGuard {
	if window <= 0 {
		window = DefaultReplayWindow
	}
	if maxEntries <= 0 {
		maxEntries = DefaultReplayMaxEntries
	}
	if nowFunc == nil {
		nowFunc = time.Now
	}
	return &ReplayGuard{
		window:     window,
		ttl:        window * 2,
		maxEntries: maxEntries,
		nowFunc:    nowFunc,
		order:      list.New(),
		index:      make(map[string]*list.Element, 256),
	}
}

// Check validates tsMs within ±window and atomically records (deviceID, reqID) if unseen.
// Returns ("", serverTsMs) when accepted, or an error code + current serverTsMs when rejected.
func (g *ReplayGuard) Check(deviceID, reqID string, tsMs int64) (string, int64) {
	now := g.nowFunc()
	nowMs := now.UnixMilli()

	reqID = strings.TrimSpace(reqID)
	if len(reqID) < 8 || len(reqID) > 128 {
		return ErrReplayInvalidID, nowMs
	}
	if tsMs <= 0 {
		return ErrReplayClockSkew, nowMs
	}

	diff := nowMs - tsMs
	if diff < 0 {
		diff = -diff
	}
	if diff > g.window.Milliseconds() {
		return ErrReplayClockSkew, nowMs
	}

	key := strings.TrimSpace(deviceID) + "\x00" + reqID

	g.mu.Lock()
	defer g.mu.Unlock()

	g.pruneExpiredLocked(now)

	if elem, exists := g.index[key]; exists {
		ent := elem.Value.(replayEntry)
		if now.Before(ent.expiresAt) {
			return ErrReplayDetected, nowMs
		}
		g.order.Remove(elem)
		delete(g.index, key)
	}

	for g.order.Len() >= g.maxEntries {
		front := g.order.Front()
		if front == nil {
			break
		}
		old := front.Value.(replayEntry)
		g.order.Remove(front)
		delete(g.index, old.key)
	}

	elem := g.order.PushBack(replayEntry{
		key:       key,
		expiresAt: now.Add(g.ttl),
	})
	g.index[key] = elem
	return "", nowMs
}

func (g *ReplayGuard) pruneExpiredLocked(now time.Time) {
	// Because TTL is fixed, entries expire strictly in FIFO order from the front.
	for {
		front := g.order.Front()
		if front == nil {
			return
		}
		ent := front.Value.(replayEntry)
		if now.Before(ent.expiresAt) {
			return
		}
		g.order.Remove(front)
		delete(g.index, ent.key)
	}
}
