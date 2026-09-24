package proxy

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CursorSource indicates which client device currently holds focus.
type CursorSource string

const (
	CursorSourceMobile  CursorSource = "mobile"
	CursorSourceDesktop CursorSource = "desktop"

	DefaultMobileStickyDuration = 30 * time.Minute
	AntiReflectionDuration      = 1500 * time.Millisecond
)

var (
	lastViewTimeRegex = regexp.MustCompile(`last_user_view_time:\s*\{\s*seconds:\s*(\d+)(?:\s+nanos:\s*(\d+))?`)
)

// UnifiedCursor represents the globally arbitrated active cascade session.
type UnifiedCursor struct {
	CascadeID  string       `json:"cascade_id"`
	Title      string       `json:"title"`
	Source     CursorSource `json:"source"`
	UpdatedAt  time.Time    `json:"updated_at"`
	IsSticky   bool         `json:"is_sticky"`
	TimeSkewMs int64        `json:"time_skew_ms"`
}

// SetMobileFocus records the mobile client's active session and triggers anti-reflection suppression.
func (p *Proxy) SetMobileFocus(cascadeID, title string) {
	if cascadeID == "" {
		return
	}
	now := time.Now()
	p.cursorMu.Lock()
	defer p.cursorMu.Unlock()

	p.mobileCascadeID = cascadeID
	if title != "" && title != "未命名会话" {
		p.mobileTitle = title
	} else if p.mobileCascadeID != cascadeID || p.mobileTitle == "" {
		p.mobileTitle = title
	}
	p.mobileFocusedAt = now
	p.suppressDesktopFocusUntil = now.Add(AntiReflectionDuration)
}

// SetDesktopFocus explicitly updates the desktop focus cursor if viewTime is newer.
func (p *Proxy) SetDesktopFocus(cascadeID, title string, viewTime time.Time) {
	if cascadeID == "" {
		return
	}
	p.cursorMu.Lock()
	defer p.cursorMu.Unlock()

	if viewTime.After(p.desktopFocusedAt) || p.desktopCascadeID == "" {
		p.desktopCascadeID = cascadeID
		if title != "" {
			p.desktopTitle = title
		}
		p.desktopFocusedAt = viewTime
	}
}

// SuppressDesktopFocus pauses desktop focus updates for the given duration to prevent self-reflection.
func (p *Proxy) SuppressDesktopFocus(d time.Duration) {
	p.cursorMu.Lock()
	defer p.cursorMu.Unlock()
	p.suppressDesktopFocusUntil = time.Now().Add(d)
}

// ArbitrateCursor computes the current active cursor based on mobile and desktop focus states.
func (p *Proxy) ArbitrateCursor() *UnifiedCursor {
	now := time.Now()

	// 1. Highest priority: Mobile active stream WebSocket is connected
	activeID, activeTitle := p.ActiveStream()
	if activeID != "" && isValidCascade(activeID) {
		p.cursorMu.RLock()
		updatedAt := p.mobileFocusedAt
		if updatedAt.IsZero() {
			updatedAt = now
		}
		p.cursorMu.RUnlock()
		return &UnifiedCursor{
			CascadeID:  activeID,
			Title:      activeTitle,
			Source:     CursorSourceMobile,
			UpdatedAt:  updatedAt,
			IsSticky:   false,
			TimeSkewMs: 0,
		}
	}

	p.cursorMu.RLock()
	mobileID := p.mobileCascadeID
	mobileTitle := p.mobileTitle
	mobileTime := p.mobileFocusedAt
	desktopID := p.desktopCascadeID
	desktopTitle := p.desktopTitle
	desktopTime := p.desktopFocusedAt
	stickyDuration := p.mobileStickyDuration
	if stickyDuration <= 0 {
		stickyDuration = DefaultMobileStickyDuration
	}
	p.cursorMu.RUnlock()

	mobileValid := mobileID != "" && isValidCascade(mobileID)
	desktopValid := desktopID != "" && isValidCascade(desktopID)

	// 2. Mobile sticky focus: within sticky duration and newer than desktop
	isMobileSticky := mobileValid && !mobileTime.IsZero() && now.Sub(mobileTime) < stickyDuration
	if isMobileSticky {
		// If mobile focus is newer than desktop focus, mobile retains focus in sticky mode
		if desktopTime.IsZero() || mobileTime.After(desktopTime) {
			return &UnifiedCursor{
				CascadeID:  mobileID,
				Title:      mobileTitle,
				Source:     CursorSourceMobile,
				UpdatedAt:  mobileTime,
				IsSticky:   true,
				TimeSkewMs: now.Sub(mobileTime).Milliseconds(),
			}
		}
	}

	// 3. Desktop active focus: desktop is newer or mobile has expired
	if desktopValid && !desktopTime.IsZero() {
		return &UnifiedCursor{
			CascadeID:  desktopID,
			Title:      desktopTitle,
			Source:     CursorSourceDesktop,
			UpdatedAt:  desktopTime,
			IsSticky:   false,
			TimeSkewMs: now.Sub(desktopTime).Milliseconds(),
		}
	}

	// 4. Fallback to mobile if valid (even if expired past sticky duration)
	if mobileValid {
		return &UnifiedCursor{
			CascadeID:  mobileID,
			Title:      mobileTitle,
			Source:     CursorSourceMobile,
			UpdatedAt:  mobileTime,
			IsSticky:   true,
			TimeSkewMs: now.Sub(mobileTime).Milliseconds(),
		}
	}

	// 5. Fallback: Scan disk annotations for newest valid cascade
	if bestID, bestTitle, bestTime := scanNewestValidAnnotation(); bestID != "" {
		p.SetDesktopFocus(bestID, bestTitle, bestTime)
		return &UnifiedCursor{
			CascadeID:  bestID,
			Title:      bestTitle,
			Source:     CursorSourceDesktop,
			UpdatedAt:  bestTime,
			IsSticky:   false,
			TimeSkewMs: now.Sub(bestTime).Milliseconds(),
		}
	}

	return nil
}

type cascadeValidCacheEntry struct {
	valid     bool
	checkedAt time.Time
}

var (
	cascadeValidCacheMu sync.RWMutex
	cascadeValidCache   = make(map[string]cascadeValidCacheEntry)
)

const (
	validCascadeTTL   = 30 * time.Second
	invalidCascadeTTL = 5 * time.Second
)

// InvalidateCascadeValidCache evicts a cascade ID from the validation cache.
func InvalidateCascadeValidCache(cascadeID string) {
	if cascadeID == "" {
		return
	}
	cascadeValidCacheMu.Lock()
	delete(cascadeValidCache, cascadeID)
	cascadeValidCacheMu.Unlock()
}

// isValidCascade verifies that a cascade ID is not deleted, has a brain directory, and is not a ghost.
func isValidCascade(cascadeID string) bool {
	if cascadeID == "" || IsDeletedCascade(cascadeID) {
		return false
	}

	cascadeValidCacheMu.RLock()
	if entry, ok := cascadeValidCache[cascadeID]; ok {
		ttl := validCascadeTTL
		if !entry.valid {
			ttl = invalidCascadeTTL
		}
		if time.Since(entry.checkedAt) < ttl {
			cascadeValidCacheMu.RUnlock()
			return entry.valid
		}
	}
	cascadeValidCacheMu.RUnlock()

	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	brainDir := filepath.Join(home, ".gemini", "antigravity", "brain", cascadeID)
	fi, err := os.Stat(brainDir)
	valid := err == nil && fi.IsDir()

	cascadeValidCacheMu.Lock()
	if len(cascadeValidCache) > 512 {
		now := time.Now()
		for k, v := range cascadeValidCache {
			ttl := validCascadeTTL
			if !v.valid {
				ttl = invalidCascadeTTL
			}
			if now.Sub(v.checkedAt) > ttl {
				delete(cascadeValidCache, k)
			}
		}
	}
	cascadeValidCache[cascadeID] = cascadeValidCacheEntry{
		valid:     valid,
		checkedAt: time.Now(),
	}
	cascadeValidCacheMu.Unlock()

	return valid
}

// parseAnnotationFile parses title and last_user_view_time from a .pbtxt file.
func parseAnnotationFile(path string) (string, time.Time) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", time.Time{}
	}
	title := ""
	m := titleRegex.FindSubmatch(b)
	if len(m) > 1 {
		title = strings.TrimSpace(string(m[1]))
	}

	var viewTime time.Time
	mTime := lastViewTimeRegex.FindSubmatch(b)
	if len(mTime) > 1 {
		sec, err := strconv.ParseInt(string(mTime[1]), 10, 64)
		if err == nil {
			var nanos int64
			if len(mTime) > 2 && len(mTime[2]) > 0 {
				nanos, _ = strconv.ParseInt(string(mTime[2]), 10, 64)
			}
			viewTime = time.Unix(sec, nanos)
		}
	}
	if viewTime.IsZero() {
		if fi, err := os.Stat(path); err == nil {
			viewTime = fi.ModTime()
		}
	}
	return title, viewTime
}

var (
	newestAnnotationCacheMu sync.RWMutex
	cachedHomeDir           string
	cachedNewestID          string
	cachedNewestTitle       string
	cachedNewestTime        time.Time
	newestAnnotationFetched time.Time
)

// ClearNewestAnnotationCache invalidates the scanNewestValidAnnotation TTL cache.
func ClearNewestAnnotationCache() {
	newestAnnotationCacheMu.Lock()
	cachedHomeDir = ""
	cachedNewestID = ""
	cachedNewestTitle = ""
	cachedNewestTime = time.Time{}
	newestAnnotationFetched = time.Time{}
	newestAnnotationCacheMu.Unlock()
}

// scanNewestValidAnnotation scans annotations directory for the latest non-ghost cascade with a 5s TTL cache.
func scanNewestValidAnnotation() (string, string, time.Time) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", time.Time{}
	}

	newestAnnotationCacheMu.RLock()
	if home == cachedHomeDir && time.Since(newestAnnotationFetched) < 5*time.Second {
		id, title, vt := cachedNewestID, cachedNewestTitle, cachedNewestTime
		newestAnnotationCacheMu.RUnlock()
		if id == "" || isValidCascade(id) {
			return id, title, vt
		}
	} else {
		newestAnnotationCacheMu.RUnlock()
	}

	annDir := filepath.Join(home, ".gemini", "antigravity", "annotations")
	entries, err := os.ReadDir(annDir)
	if err != nil {
		newestAnnotationCacheMu.Lock()
		cachedHomeDir = home
		cachedNewestID = ""
		cachedNewestTitle = ""
		cachedNewestTime = time.Time{}
		newestAnnotationFetched = time.Now()
		newestAnnotationCacheMu.Unlock()
		return "", "", time.Time{}
	}

	var bestID, bestTitle string
	var bestTime time.Time

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".pbtxt") {
			continue
		}
		cid := strings.TrimSuffix(e.Name(), ".pbtxt")
		if !isValidCascade(cid) {
			continue
		}
		title, vTime := parseAnnotationFile(filepath.Join(annDir, e.Name()))
		if title == "" || title == "未命名会话" {
			continue
		}
		if vTime.After(bestTime) {
			bestID = cid
			bestTitle = title
			bestTime = vTime
		}
	}

	newestAnnotationCacheMu.Lock()
	cachedHomeDir = home
	cachedNewestID = bestID
	cachedNewestTitle = bestTitle
	cachedNewestTime = bestTime
	newestAnnotationFetched = time.Now()
	newestAnnotationCacheMu.Unlock()

	return bestID, bestTitle, bestTime
}

// handleFocusSession handles POST /gateway/cascade/focus from mobile clients.
func (p *Proxy) handleFocusSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CascadeID string `json:"cascadeId"`
		Source    string `json:"source"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&req); err != nil || req.CascadeID == "" {
		writeJSONError(w, "Invalid payload or missing cascadeId", http.StatusBadRequest)
		return
	}

	cascadeID := strings.TrimSpace(req.CascadeID)
	port, token := p.ActiveUpstream()
	title := p.lookupCascadeTitle(cascadeID, port, token)
	if title == "" || title == "未命名会话" {
		title = readAnnotationTitle(cascadeID)
	}

	p.SetMobileFocus(cascadeID, title)

	// Pre-warm cache for the newly focused cascade
	go p.prewarmCascadeCache(cascadeID)

	now := time.Now()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "ok",
		"cascade_id": cascadeID,
		"title":      title,
		"focused_at": now.UTC().Format(time.RFC3339Nano),
	})
}

// prewarmCascadeCache triggers trajectory cache fetching in the background.
func (p *Proxy) prewarmCascadeCache(cascadeID string) {
	if cascadeID == "" || IsDeletedCascade(cascadeID) {
		return
	}
	port, token := p.ActiveUpstream()
	if port <= 0 {
		return
	}
	_, _ = p.fetchUpstreamTrajectoryWithMaxAge(cascadeID, port, token, 1*time.Second)
}

// StartDesktopFocusWatcher starts a lightweight poller monitoring annotations for desktop IDE tab switches.
func (p *Proxy) StartDesktopFocusWatcher(ctx context.Context) {
	p.startDesktopFocusWatcherWithInterval(ctx, 2*time.Second)
}

func (p *Proxy) startDesktopFocusWatcherWithInterval(ctx context.Context, interval time.Duration) {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("[Cursor] Cannot determine home directory for desktop watcher: %v", err)
		return
	}
	annDir := filepath.Join(home, ".gemini", "antigravity", "annotations")

	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lastSeenMtimes := make(map[string]time.Time)

	scanPass := func() {
		entries, err := os.ReadDir(annDir)
		if err != nil {
			return
		}

		now := time.Now()

		// Check anti-reflection suppression
		p.cursorMu.RLock()
		suppressUntil := p.suppressDesktopFocusUntil
		mobileID := p.mobileCascadeID
		mobileTime := p.mobileFocusedAt
		p.cursorMu.RUnlock()

		isSuppressed := now.Before(suppressUntil)

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".pbtxt") {
				continue
			}
			cid := strings.TrimSuffix(e.Name(), ".pbtxt")
			fullPath := filepath.Join(annDir, e.Name())

			info, err := e.Info()
			if err != nil {
				continue
			}
			mtime := info.ModTime()
			lastMtime, seen := lastSeenMtimes[cid]
			if seen && !mtime.After(lastMtime) {
				continue
			}
			lastSeenMtimes[cid] = mtime

			// Anti-reflection check: Ignore if within suppression window or matches mobile's recent focus
			if isSuppressed || (cid == mobileID && now.Sub(mobileTime) < 2*time.Second) {
				continue
			}

			// Triple check: Brain directory must exist
			if !isValidCascade(cid) {
				continue
			}

			title, vTime := parseAnnotationFile(fullPath)
			if title == "" || title == "未命名会话" {
				continue
			}

			p.cursorMu.Lock()
			if vTime.After(p.desktopFocusedAt) || p.desktopCascadeID == "" {
				p.desktopCascadeID = cid
				p.desktopTitle = title
				p.desktopFocusedAt = vTime
				if verboseRPC {
					log.Printf("[Cursor] Desktop focus migrated to %s (%s) at %v", cid, title, vTime)
				}
			}
			p.cursorMu.Unlock()
		}

		// Periodic cleanup of removed session mtimes when map grows
		if len(lastSeenMtimes) > 256 {
			activeCids := make(map[string]bool, len(entries))
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".pbtxt") {
					activeCids[strings.TrimSuffix(e.Name(), ".pbtxt")] = true
				}
			}
			for cid := range lastSeenMtimes {
				if !activeCids[cid] {
					delete(lastSeenMtimes, cid)
				}
			}
		}
	}

	// Immediate first pass upon start
	scanPass()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			scanPass()
		}
	}
}
