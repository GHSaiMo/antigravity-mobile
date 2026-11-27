package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/cockpit"
	"antigravity-mobile/internal/config"
)

// CancelCascadeStep invokes Antigravity LanguageServer's CancelCascadeSteps RPC to terminate a specific background step.
func (p *Proxy) CancelCascadeStep(cascadeID string, stepIndex int, port int, token string) error {
	if port == 0 {
		return fmt.Errorf("no active Antigravity upstream")
	}

	buf := GetSmallBuffer()
	defer PutSmallBuffer(buf)
	buf.WriteString(`{"cascadeId":`)
	buf.WriteString(strconv.Quote(cascadeID))
	buf.WriteString(`,"stepIndices":[`)
	buf.WriteString(strconv.Itoa(stepIndex))
	buf.WriteString(`]}`)

	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/CancelCascadeSteps", port)
	req, err := http.NewRequest(http.MethodPost, url, buf)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.mediumClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("CancelCascadeSteps returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (p *Proxy) fetchUpstreamTrajectory(cascadeID string, port int, token string) (*upstreamTrajectoryResp, error) {
	return p.fetchUpstreamTrajectoryWithContext(context.Background(), cascadeID, port, token)
}

func (p *Proxy) fetchUpstreamTrajectoryWithContext(ctx context.Context, cascadeID string, port int, token string) (*upstreamTrajectoryResp, error) {
	if IsDeletedCascade(cascadeID) {
		return nil, fmt.Errorf("cascade trajectory %s has been deleted", cascadeID)
	}

	tc := defaultTrajCache
	// Status-aware TTL: completed sessions rarely change, so cache them longer.
	// But if title is missing or session has few/no steps, keep TTL short (1.5s)
	// so newly generated titles/summaries are quickly discovered.
	maxAge := 800 * time.Millisecond
	activeID := ResolveActiveCascadeID(cascadeID)
	if activeID == "" {
		activeID = cascadeID
	}
	tc.trajCacheMu.RLock()
	if cached, ok := tc.trajCache[activeID]; ok && cached.port == port {
		if cached.data.Status != "" && cached.data.Status != "CASCADE_RUN_STATUS_RUNNING" {
			hasTitle := (cached.data.Trajectory.Annotations != nil && cached.data.Trajectory.Annotations.Title != "") ||
				cached.data.Trajectory.Summary != ""
			if hasTitle && len(cached.data.Trajectory.Steps) > 0 {
				maxAge = 60 * time.Second
			} else {
				maxAge = 5 * time.Second
			}
		}
	}
	tc.trajCacheMu.RUnlock()

	resp, err := p.fetchUpstreamTrajectoryWithMaxAgeContext(ctx, cascadeID, port, token, maxAge)
	// Fallback: If not found or empty steps, try loading from disk via LoadTrajectory and retry once
	if (err != nil || (resp != nil && len(resp.Trajectory.Steps) == 0)) && port > 0 {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if loadErr := p.LoadTrajectory(cascadeID, port, token); loadErr == nil {
			tc.trajCacheMu.Lock()
			delete(tc.trajCache, cascadeID)
			tc.trajCacheMu.Unlock()
			trajFlights.forget(cascadeID)
			if retryResp, retryErr := p.fetchUpstreamTrajectoryWithMaxAgeContext(ctx, cascadeID, port, token, 0); retryErr == nil && retryResp != nil {
				return retryResp, nil
			}
		}
	}
	return resp, err
}

func (p *Proxy) lookupCascadeTitle(cascadeID string, port int, token string) string {
	if cascadeID == "" {
		return ""
	}

	defaultTrajCache.cascadeTitlesMu.RLock()
	cachedTitle, ok := defaultTrajCache.cascadeTitles[cascadeID]
	isFallback := defaultTrajCache.cascadeTitleFallback[cascadeID]
	defaultTrajCache.cascadeTitlesMu.RUnlock()

	if ok && !isFallback && cachedTitle != "" && cachedTitle != "未命名会话" {
		return cachedTitle
	}

	if t := readAnnotationTitle(cascadeID); t != "" && t != "未命名会话" {
		setCachedCascadeTitle(cascadeID, t)
		return t
	}

	if port > 0 {
		defaultTrajCache.cascadeTitlesMu.RLock()
		lastFetch := defaultTrajCache.lastTitlesFetchTime
		defaultTrajCache.cascadeTitlesMu.RUnlock()

		// Rate limit: do not re-scan all trajectories if checked within the last 5 seconds
		if time.Since(lastFetch) >= 5*time.Second {
			summaries, err := p.fetchTrajectoriesSummaryWithTitles(port, token)
			defaultTrajCache.cascadeTitlesMu.Lock()
			defaultTrajCache.lastTitlesFetchTime = time.Now()
			if err == nil && len(summaries) > 0 {
				if defaultTrajCache.cascadeTitleFallback == nil {
					defaultTrajCache.cascadeTitleFallback = make(map[string]bool)
				}
				if len(defaultTrajCache.cascadeTitles) > maxMetadataMapSize {
					count := 0
					for k := range defaultTrajCache.cascadeTitles {
						delete(defaultTrajCache.cascadeTitles, k)
						delete(defaultTrajCache.cascadeTitleFallback, k)
						count++
						if count >= maxMetadataMapSize/2 {
							break
						}
					}
				}
				for cid, t := range summaries {
					if t != "" && t != "未命名会话" {
						defaultTrajCache.cascadeTitles[cid] = t
						delete(defaultTrajCache.cascadeTitleFallback, cid)
					}
				}
				newTitle := defaultTrajCache.cascadeTitles[cascadeID]
				newIsFallback := defaultTrajCache.cascadeTitleFallback[cascadeID]
				defaultTrajCache.cascadeTitlesMu.Unlock()
				if !newIsFallback && newTitle != "" && newTitle != "未命名会话" {
					return newTitle
				}
			} else {
				defaultTrajCache.cascadeTitlesMu.Unlock()
			}
		}
	}

	// Fallback to cached trajectory first user prompt (never overwrites an authoritative title).
	defaultTrajCache.trajCacheMu.RLock()
	entry, hasEntry := defaultTrajCache.trajCache[cascadeID]
	defaultTrajCache.trajCacheMu.RUnlock()
	if hasEntry && entry != nil && entry.data != nil {
		for _, s := range entry.data.Trajectory.Steps {
			if s.Type == "CORTEX_STEP_TYPE_USER_INPUT" {
				if t := extractTitleFromUserInput(s); t != "" && t != "未命名会话" {
					setCachedCascadeTitleFallback(cascadeID, t)
					return t
				}
			}
		}
	}

	if ok && cachedTitle != "" && cachedTitle != "未命名会话" {
		return cachedTitle
	}
	return ""
}

// LoadTrajectory asks upstream language_server to load a historical cascade into memory.
func (p *Proxy) LoadTrajectory(cascadeID string, port int, token string) error {
	if cascadeID == "" || port == 0 {
		return fmt.Errorf("invalid cascadeId or port")
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/LoadTrajectory", port)
	payload, _ := json.Marshal(map[string]string{"cascadeId": cascadeID})
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	client := p.shortClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upstream LoadTrajectory returned %d: %s", resp.StatusCode, string(b))
	}

	return nil
}

// defaultMaxHistoricalSync bounds how many historical conversation DBs are eagerly
// loaded into language_server memory on startup/reconnect. Antigravity 2.35.0 keeps
// unlimited local history by default, so preloading only the newest sessions avoids
// unbounded memory bloat and startup stalls; older sessions still load lazily on demand
// via FetchTrajectory -> LoadTrajectory.
const defaultMaxHistoricalSync = 200

func maxHistoricalSyncLimit() int {
	raw := strings.TrimSpace(os.Getenv("MGY_MAX_HISTORICAL_SYNC"))
	if raw == "" {
		return defaultMaxHistoricalSync
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return defaultMaxHistoricalSync
	}
	return n
}

// SyncHistoricalTrajectories scans ~/.gemini/antigravity/conversations for historical session DBs
// and eagerly preloads the most recent sessions (capped by MGY_MAX_HISTORICAL_SYNC, default 200)
// into upstream language_server memory. Older sessions remain lazily loaded on demand via LoadTrajectory.
func (p *Proxy) SyncHistoricalTrajectories(port int, token string) error {
	if port == 0 {
		return fmt.Errorf("upstream port not set")
	}

	// Single-flight coalescing per upstream port: if another goroutine is already syncing this port
	// (e.g., updateUpstream + handleGetAllCascadeTrajectories racing on reconnect), wait for it.
	defaultTrajCache.loadedCascadesMu.Lock()
	if defaultTrajCache.syncingPort == port && defaultTrajCache.syncingDone != nil {
		waitCh := defaultTrajCache.syncingDone
		defaultTrajCache.loadedCascadesMu.Unlock()
		<-waitCh
		return nil
	}
	if port != defaultTrajCache.lastSyncedPort {
		defaultTrajCache.loadedCascades = make(map[string]bool)
		defaultTrajCache.lastSyncedPort = port
	}
	done := make(chan struct{})
	defaultTrajCache.syncingPort = port
	defaultTrajCache.syncingDone = done
	defaultTrajCache.loadedCascadesMu.Unlock()

	defer func() {
		defaultTrajCache.loadedCascadesMu.Lock()
		if defaultTrajCache.syncingDone == done {
			defaultTrajCache.syncingPort = 0
			defaultTrajCache.syncingDone = nil
		}
		defaultTrajCache.loadedCascadesMu.Unlock()
		close(done)
	}()

	limit := maxHistoricalSyncLimit()
	if limit == 0 {
		return nil
	}

	convDir := resolveAntigravitySubPath("conversations")
	if convDir == "" {
		return fmt.Errorf("cannot resolve antigravity conversations directory")
	}
	entries, err := os.ReadDir(convDir)
	if err != nil {
		return err
	}

	type historicalCandidate struct {
		cascadeID string
		modTime   time.Time
	}
	var candidates []historicalCandidate
	defaultTrajCache.loadedCascadesMu.Lock()
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".db") {
			continue
		}
		cascadeID := strings.TrimSuffix(name, ".db")
		if defaultTrajCache.loadedCascades[cascadeID] || IsDeletedCascade(cascadeID) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.Size() == 0 {
			continue
		}

		// Empty session schemas are exactly 48KB (49152 bytes) with 0 steps.
		// If older than 15 minutes and <= 49152 bytes, skip loading old empty drafts.
		if info.Size() <= 49152 && time.Since(info.ModTime()) > 15*time.Minute {
			defaultTrajCache.loadedCascades[cascadeID] = true
			continue
		}

		candidates = append(candidates, historicalCandidate{
			cascadeID: cascadeID,
			modTime:   info.ModTime(),
		})
	}
	defaultTrajCache.loadedCascadesMu.Unlock()

	if len(candidates) == 0 {
		return nil
	}

	sort.Slice(candidates, func(i, j int) bool {
		if !candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].modTime.After(candidates[j].modTime)
		}
		return candidates[i].cascadeID < candidates[j].cascadeID
	})

	totalCandidates := len(candidates)
	if totalCandidates > limit {
		skipped := candidates[limit:]
		candidates = candidates[:limit]
		// Mark deferred older sessions as processed for this port so repeated calls do not keep
		// loading older batches; opening any of them still triggers on-demand LoadTrajectory.
		defaultTrajCache.loadedCascadesMu.Lock()
		for _, s := range skipped {
			defaultTrajCache.loadedCascades[s.cascadeID] = true
		}
		defaultTrajCache.loadedCascadesMu.Unlock()
		slog.Info("[Proxy] Capping eager historical trajectory sync to newest sessions; older sessions load on demand",
			"syncing", limit, "total", totalCandidates)
	} else if verboseRPC {
		slog.Info(fmt.Sprintf("[Proxy] Syncing %d historical trajectories into upstream language_server...", totalCandidates))
	}

	concurrency := 2
	if concurrency > len(candidates) {
		concurrency = len(candidates)
	}

	workCh := make(chan string, len(candidates))
	for _, cand := range candidates {
		workCh <- cand.cascadeID
	}
	close(workCh)

	// Collect successful loads via a buffered channel to batch-write under a single lock.
	successCh := make(chan string, len(candidates))
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for cid := range workCh {
				err := p.LoadTrajectory(cid, port, token)
				if err == nil {
					successCh <- cid
				} else if verboseRPC {
					slog.Warn(fmt.Sprintf("[Proxy] Failed to load historical trajectory %s", cid), "err", err)
				}
			}
		}()
	}
	wg.Wait()
	close(successCh)

	// Batch-write all successes under a single lock acquisition instead of per-item locking.
	defaultTrajCache.loadedCascadesMu.Lock()
	for cid := range successCh {
		defaultTrajCache.loadedCascades[cid] = true
	}
	defaultTrajCache.loadedCascadesMu.Unlock()

	if verboseRPC {
		slog.Info("[Proxy] Finished syncing historical trajectories")
	}
	return nil
}

func (p *Proxy) fetchTrajectoriesSummaryWithTitles(port int, token string) (map[string]string, error) {
	raw, err := p.fetchAllTrajectoriesRaw(context.Background(), port, token)
	if err != nil {
		return nil, err
	}

	var data struct {
		TrajectorySummaries map[string]struct {
			Summary     string `json:"summary"`
			Annotations *struct {
				Title string `json:"title"`
			} `json:"annotations"`
		} `json:"trajectorySummaries"`
	}

	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for id, sum := range data.TrajectorySummaries {
		t := ""
		if sum.Annotations != nil && sum.Annotations.Title != "" {
			t = sum.Annotations.Title
		} else if sum.Summary != "" {
			t = sum.Summary
		}
		if t != "" {
			result[id] = t
		}
	}
	return result, nil
}

func (p *Proxy) fetchUpstreamTrajectoryWithMaxAge(cascadeID string, port int, token string, maxAge time.Duration) (*upstreamTrajectoryResp, error) {
	return p.fetchUpstreamTrajectoryWithMaxAgeContext(context.Background(), cascadeID, port, token, maxAge)
}

func stitchedTrajectoryCacheKey(rootID string) string {
	rootID = strings.TrimSpace(rootID)
	if rootID == "" {
		return ""
	}
	return "stitched:" + rootID
}

func (p *Proxy) fetchSingleSegmentTrajectory(ctx context.Context, cascadeID string, port int, token string, maxAge time.Duration) (*upstreamTrajectoryResp, error) {
	if IsDeletedCascade(cascadeID) {
		return nil, fmt.Errorf("cascade trajectory %s has been deleted", cascadeID)
	}

	defaultTrajCache.trajCacheMu.RLock()
	if cached, ok := defaultTrajCache.trajCache[cascadeID]; ok && cached != nil && cached.data != nil && cached.port == port {
		if time.Since(cached.fetchedAt) < maxAge {
			defaultTrajCache.trajCacheMu.RUnlock()
			return cached.data, nil
		}
	}
	defaultTrajCache.trajCacheMu.RUnlock()

	return trajFlights.doCtx(ctx, cascadeID, func(upCtx context.Context) (*upstreamTrajectoryResp, error) {
		return p.fetchUpstreamTrajectoryUncached(upCtx, cascadeID, port, token)
	})
}

// fetchHistoricalSegmentTrajectory returns a sealed prior segment (before the active leaf) in a stitched chain.
// Once a prior segment has been fetched with non-empty steps, it is immutable across account/port switches,
// so we reuse the in-memory cached steps without repeatedly polling or calling LoadTrajectory on upstream.
func (p *Proxy) fetchHistoricalSegmentTrajectory(ctx context.Context, cascadeID string, port int, token string) (*upstreamTrajectoryResp, error) {
	if IsDeletedCascade(cascadeID) {
		return nil, fmt.Errorf("cascade trajectory %s has been deleted", cascadeID)
	}

	defaultTrajCache.trajCacheMu.RLock()
	var staleData *upstreamTrajectoryResp
	if cached, ok := defaultTrajCache.trajCache[cascadeID]; ok && cached != nil && cached.data != nil {
		staleData = cached.data
		if len(cached.data.Trajectory.Steps) > 0 && cached.data.Status != "CASCADE_RUN_STATUS_RUNNING" {
			defaultTrajCache.trajCacheMu.RUnlock()
			return cached.data, nil
		}
	}
	defaultTrajCache.trajCacheMu.RUnlock()

	segResp, err := p.fetchSingleSegmentTrajectory(ctx, cascadeID, port, token, 24*time.Hour)
	if (err != nil || (segResp != nil && len(segResp.Trajectory.Steps) == 0)) && port > 0 && (ctx == nil || ctx.Err() == nil) {
		if loadErr := p.LoadTrajectory(cascadeID, port, token); loadErr == nil {
			defaultTrajCache.trajCacheMu.Lock()
			delete(defaultTrajCache.trajCache, cascadeID)
			defaultTrajCache.trajCacheMu.Unlock()
			trajFlights.forget(cascadeID)
			if retryResp, retryErr := p.fetchSingleSegmentTrajectory(ctx, cascadeID, port, token, 0); retryErr == nil && retryResp != nil {
				segResp, err = retryResp, nil
			}
		}
	}
	if (err != nil || segResp == nil || len(segResp.Trajectory.Steps) == 0) && staleData != nil && len(staleData.Trajectory.Steps) > 0 {
		return staleData, nil
	}
	return segResp, err
}

func (p *Proxy) fetchUpstreamTrajectoryWithMaxAgeContext(ctx context.Context, cascadeID string, port int, token string, maxAge time.Duration) (*upstreamTrajectoryResp, error) {
	if IsDeletedCascade(cascadeID) {
		return nil, fmt.Errorf("cascade trajectory %s has been deleted", cascadeID)
	}

	segments := GetStitchChainSegments(cascadeID)
	if len(segments) <= 1 {
		return p.fetchSingleSegmentTrajectory(ctx, cascadeID, port, token, maxAge)
	}

	rootID := ResolveVisibleRootCascadeID(cascadeID)
	stitchedKey := stitchedTrajectoryCacheKey(rootID)
	if stitchedKey != "" && maxAge > 0 {
		defaultTrajCache.trajCacheMu.RLock()
		if cached, ok := defaultTrajCache.trajCache[stitchedKey]; ok && cached != nil && cached.data != nil && cached.port == port {
			if time.Since(cached.fetchedAt) < maxAge {
				data := cached.data
				defaultTrajCache.trajCacheMu.RUnlock()
				return data, nil
			}
		}
		defaultTrajCache.trajCacheMu.RUnlock()
	}

	var combined *upstreamTrajectoryResp
	var allSteps []TrajectoryStep

	for i, seg := range segments {
		var segResp *upstreamTrajectoryResp
		var err error
		if i < len(segments)-1 {
			segResp, err = p.fetchHistoricalSegmentTrajectory(ctx, seg.CascadeID, port, token)
		} else {
			defaultTrajCache.trajCacheMu.RLock()
			var prevLeafData *upstreamTrajectoryResp
			if cached, ok := defaultTrajCache.trajCache[seg.CascadeID]; ok && cached != nil && cached.data != nil {
				prevLeafData = cached.data
			}
			defaultTrajCache.trajCacheMu.RUnlock()

			segResp, err = p.fetchSingleSegmentTrajectory(ctx, seg.CascadeID, port, token, maxAge)
			if err != nil && port > 0 && (ctx == nil || ctx.Err() == nil) {
				if loadErr := p.LoadTrajectory(seg.CascadeID, port, token); loadErr == nil {
					defaultTrajCache.trajCacheMu.Lock()
					delete(defaultTrajCache.trajCache, seg.CascadeID)
					defaultTrajCache.trajCacheMu.Unlock()
					trajFlights.forget(seg.CascadeID)
					if retryResp, retryErr := p.fetchSingleSegmentTrajectory(ctx, seg.CascadeID, port, token, 0); retryErr == nil && retryResp != nil {
						segResp, err = retryResp, nil
					}
				}
			}
			if (err != nil || segResp == nil) && prevLeafData != nil {
				segResp, err = prevLeafData, nil
			}
		}
		if err != nil || segResp == nil {
			// If any segment fails and has no cached fallback, return the last complete stitched snapshot
			// (or error) rather than silently omitting a segment and shifting all step indices backwards.
			if stitchedKey != "" {
				defaultTrajCache.trajCacheMu.RLock()
				if prevCombined, ok := defaultTrajCache.trajCache[stitchedKey]; ok && prevCombined != nil && prevCombined.data != nil {
					data := prevCombined.data
					defaultTrajCache.trajCacheMu.RUnlock()
					return data, nil
				}
				defaultTrajCache.trajCacheMu.RUnlock()
			}
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("segment trajectory %s unavailable", seg.CascadeID)
		}
		if combined == nil {
			cp := *segResp
			combined = &cp
		}
		if i > 0 {
			allSteps = append(allSteps, makeAccountSwitchMarkerStep(seg.AccountID, seg.CreatedAt))
		}
		allSteps = append(allSteps, segResp.Trajectory.Steps...)
		if i == len(segments)-1 {
			combined.Status = segResp.Status
			combined.PendingAgentMessages = segResp.PendingAgentMessages
			if len(segResp.Trajectory.ExecutorMetadatas) > 0 {
				combined.Trajectory.ExecutorMetadatas = segResp.Trajectory.ExecutorMetadatas
			}
		}
	}
	if combined == nil {
		return p.fetchSingleSegmentTrajectory(ctx, cascadeID, port, token, maxAge)
	}
	combined.Trajectory.CascadeID = rootID
	combined.Trajectory.Steps = allSteps

	if stitchedKey != "" {
		sig := rawTrajectorySignature(combined)
		defaultTrajCache.trajCacheMu.Lock()
		var prevParsed *TrajectoryDetails
		var prevSig string
		if prev, ok := defaultTrajCache.trajCache[stitchedKey]; ok && prev != nil && prev.parsed != nil && prev.parsedSig == sig {
			prevParsed = prev.parsed
			prevSig = prev.parsedSig
		}
		defaultTrajCache.trajCache[stitchedKey] = &trajectoryCacheEntry{
			fetchedAt: time.Now(),
			port:      port,
			data:      combined,
			parsedSig: prevSig,
			parsed:    prevParsed,
		}
		defaultTrajCache.evictTrajCacheLocked()
		defaultTrajCache.trajCacheMu.Unlock()
	}

	return combined, nil
}

// trajFlightGroup is a minimal singleflight keyed by cascade ID.
type trajFlightGroup struct {
	mu    sync.Mutex
	calls map[string]*trajFlightCall
}

type trajFlightCall struct {
	done chan struct{}
	data *upstreamTrajectoryResp
	err  error
}

var trajFlights = &trajFlightGroup{calls: make(map[string]*trajFlightCall)}

// forget detaches any in-flight call for key so later callers start a fresh upstream request.
// Callers already waiting on the old call still receive its result.
func (g *trajFlightGroup) forget(key string) {
	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
}

func (g *trajFlightGroup) do(key string, fn func() (*upstreamTrajectoryResp, error)) (*upstreamTrajectoryResp, error) {
	return g.doCtx(context.Background(), key, func(context.Context) (*upstreamTrajectoryResp, error) {
		return fn()
	})
}

func (g *trajFlightGroup) doCtx(ctx context.Context, key string, fn func(context.Context) (*upstreamTrajectoryResp, error)) (*upstreamTrajectoryResp, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	g.mu.Lock()
	c, ok := g.calls[key]
	if !ok {
		c = &trajFlightCall{done: make(chan struct{})}
		g.calls[key] = c
		go func() {
			upCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer func() {
				cancel()
				g.mu.Lock()
				if g.calls[key] == c {
					delete(g.calls, key)
				}
				g.mu.Unlock()
				close(c.done)
			}()
			c.data, c.err = fn(upCtx)
		}()
	}
	g.mu.Unlock()

	if ctx == nil {
		<-c.done
		return c.data, c.err
	}
	select {
	case <-c.done:
		return c.data, c.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *Proxy) fetchUpstreamTrajectoryUncached(ctx context.Context, cascadeID string, port int, token string) (*upstreamTrajectoryResp, error) {
	defaultTrajCache.trajCacheMu.RLock()
	gen := defaultTrajCache.cascadeGenLocked(cascadeID)
	defaultTrajCache.trajCacheMu.RUnlock()

	buf := GetSmallBuffer()
	defer PutSmallBuffer(buf)
	buf.WriteString(`{"cascadeId":`)
	buf.WriteString(strconv.Quote(cascadeID))
	buf.WriteString(`}`)
	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetCascadeTrajectory", port)

	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.mediumClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upstream returned %d: %s", resp.StatusCode, string(b))
	}

	var reader io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gzReader, err := GetGzipReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip reader failed: %w", err)
		}
		defer PutGzipReader(gzReader)
		reader = gzReader
	}

	const maxRawCacheBytes = 8 * 1024 * 1024
	const maxUpstreamTrajectoryBytes = 64 * 1024 * 1024
	rawBuf := GetHugeBuffer()
	defer PutHugeBuffer(rawBuf)
	if _, err := rawBuf.ReadFrom(io.LimitReader(reader, maxUpstreamTrajectoryBytes)); err != nil {
		return nil, fmt.Errorf("failed to read upstream response: %w", err)
	}

	var data upstreamTrajectoryResp
	if err := json.Unmarshal(rawBuf.Bytes(), &data); err != nil {
		return nil, fmt.Errorf("failed to decode upstream response: %w", err)
	}
	var preservedRaw []byte
	if rawBuf.Len() > 0 && rawBuf.Len() <= maxRawCacheBytes {
		preservedRaw = make([]byte, rawBuf.Len())
		copy(preservedRaw, rawBuf.Bytes())
	}
	compactTrajectoryMedia(&data)

	defaultTrajCache.trajCacheMu.Lock()
	if defaultTrajCache.cascadeGenLocked(cascadeID) == gen {
		defaultTrajCache.trajCache[cascadeID] = &trajectoryCacheEntry{
			fetchedAt: time.Now(),
			port:      port,
			data:      &data,
			rawJSON:   preservedRaw,
		}
		defaultTrajCache.evictTrajCacheLocked()
	}
	defaultTrajCache.trajCacheMu.Unlock()

	return &data, nil
}

// ParseTrajectoryDetailsCached returns a shallow copy of cached TrajectoryDetails if rawResp
// matches the cached trajectory entry, avoiding redundant regex/JSON scans across concurrent
// WebSocket stream subscribers, HTTP message polls, and the background Watcher.
func (p *Proxy) ParseTrajectoryDetailsCached(rawResp *upstreamTrajectoryResp) TrajectoryDetails {
	if rawResp == nil {
		return TrajectoryDetails{}
	}
	cid := rawResp.Trajectory.CascadeID
	sig := rawTrajectorySignature(rawResp)
	stitchedKey := stitchedTrajectoryCacheKey(cid)
	if cid != "" && sig != "" {
		defaultTrajCache.trajCacheMu.RLock()
		if cached, ok := defaultTrajCache.trajCache[cid]; ok && cached != nil && cached.data == rawResp && cached.parsed != nil && cached.parsedSig == sig {
			cp := *cached.parsed
			defaultTrajCache.trajCacheMu.RUnlock()
			return cp
		}
		if stitchedKey != "" {
			if cached, ok := defaultTrajCache.trajCache[stitchedKey]; ok && cached != nil && (cached.data == rawResp || cached.parsedSig == sig) && cached.parsed != nil && cached.parsedSig == sig {
				cp := *cached.parsed
				defaultTrajCache.trajCacheMu.RUnlock()
				return cp
			}
		}
		defaultTrajCache.trajCacheMu.RUnlock()
	}

	details := p.ParseTrajectoryDetails(rawResp)
	if cid != "" && sig != "" {
		stored := details
		defaultTrajCache.trajCacheMu.Lock()
		if cached, ok := defaultTrajCache.trajCache[cid]; ok && cached != nil && cached.data == rawResp {
			cached.parsedSig = sig
			cached.parsed = &stored
		} else if stitchedKey != "" {
			if cached, ok := defaultTrajCache.trajCache[stitchedKey]; ok && cached != nil {
				cached.parsedSig = sig
				cached.parsed = &stored
			}
		}
		defaultTrajCache.trajCacheMu.Unlock()
	}
	return details
}

// FetchTrajectoryDetails fetches and parses full trajectory details for a cascade.
func (p *Proxy) FetchTrajectoryDetails(cascadeID string, maxAge time.Duration) (*TrajectoryDetails, error) {
	port, token := p.ActiveUpstream()
	if port == 0 {
		return nil, fmt.Errorf("antigravity upstream not connected")
	}
	rawResp, err := p.fetchUpstreamTrajectoryWithMaxAge(cascadeID, port, token, maxAge)
	if err != nil {
		return nil, err
	}
	details := p.ParseTrajectoryDetailsCached(rawResp)
	if details.Title == "" || details.Title == "未命名会话" {
		if t := p.lookupCascadeTitle(cascadeID, port, token); t != "" {
			details.Title = t
		}
	}
	return &details, nil
}

// FetchRawCascadeSummaries queries upstream GetAllCascadeTrajectories and returns non-subagent summaries
// along with a map of parent conversation IDs that currently have active running subagents.
func (p *Proxy) FetchRawCascadeSummaries() (map[string]map[string]interface{}, map[string]bool, error) {
	return p.FetchRawCascadeSummariesWithMaxAge(0)
}

// FetchRawCascadeSummariesWithMaxAge queries GetAllCascadeTrajectories allowing reuse of recent list snapshots.
func (p *Proxy) FetchRawCascadeSummariesWithMaxAge(maxAge time.Duration) (map[string]map[string]interface{}, map[string]bool, error) {
	port, token := p.ActiveUpstream()
	if port == 0 {
		return nil, nil, fmt.Errorf("antigravity upstream not connected")
	}

	raw, err := p.fetchAllTrajectoriesRawMaxAge(context.Background(), port, token, maxAge)
	if err != nil {
		return nil, nil, err
	}

	var envelope struct {
		TrajectorySummaries map[string]map[string]interface{} `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, nil, err
	}

	summaries := envelope.TrajectorySummaries
	if summaries == nil {
		summaries = make(map[string]map[string]interface{})
	}

	filterDeletedZombieSummaries(summaries)

	subagentIDs := make(map[string]bool)
	subagentParents := make(map[string]string)
	subagentRoots := make(map[string]string)
	for id, s := range summaries {
		if s == nil || IsDeletedCascade(id) {
			continue
		}
		if isSubagentTrajectoryMap(s, id) {
			subagentIDs[id] = true
			if pid := extractSummaryParentID(s); pid != "" {
				subagentParents[id] = pid
			}
			if rid := extractSummaryRootID(s); rid != "" {
				subagentRoots[id] = rid
			}
		}
	}
	resolveVisibleAncestor := func(id string) string {
		cur := id
		for depth := 0; depth < 8; depth++ {
			next := subagentParents[cur]
			if next == "" || next == cur {
				break
			}
			if !subagentIDs[next] {
				if _, exists := summaries[next]; exists && !IsDeletedCascade(next) {
					return next
				}
				break
			}
			cur = next
		}
		if root := subagentRoots[id]; root != "" && !subagentIDs[root] {
			if _, exists := summaries[root]; exists && !IsDeletedCascade(root) {
				return root
			}
		}
		return ""
	}

	runningSubagents := make(map[string]bool)
	// Filter out internal subagent sessions while tracking parents of actively running subagents
	for id, s := range summaries {
		if IsDeletedCascade(id) || IsStitchedChildCascade(id) {
			if IsStitchedChildCascade(id) {
				rootID := ResolveVisibleRootCascadeID(id)
				if status, _ := s["status"].(string); status == "CASCADE_RUN_STATUS_RUNNING" && rootID != "" {
					runningSubagents[rootID] = true
				}
			}
			delete(summaries, id)
			continue
		}
		if isSubagentTrajectoryMap(s, id) {
			status, _ := s["status"].(string)
			if status == "CASCADE_RUN_STATUS_RUNNING" {
				if ancestor := resolveVisibleAncestor(id); ancestor != "" {
					runningSubagents[ancestor] = true
				}
			}
			delete(summaries, id)
		} else if isSideQuestionTrajectoryMap(s, id) {
			delete(summaries, id)
		}
	}

	return summaries, runningSubagents, nil
}

type sessionStepUsage struct {
	input      int64
	output     int64
	cacheRead  int64
	thinking   int64
	model      string
	responseID string
	hasUsage   bool
}

type contextMeterCacheEntry struct {
	item       *ContextMeterItem
	at         time.Time
	nextOffset int
	steps      []sessionStepUsage
}

var resolveSessionTokenUsage = cockpit.ResolveConversationSessionUsage

func applySessionUsageTotals(item *ContextMeterItem, steps []sessionStepUsage) {
	if item == nil || len(steps) == 0 {
		return
	}
	var count int
	var sumInp, sumOut, sumCache, sumThink int64
	var sumCost float64
	var lastModel string
	seenResp := make(map[string]bool, len(steps))
	for _, s := range steps {
		if m := strings.TrimSpace(s.model); m != "" {
			lastModel = m
		}
		if !s.hasUsage {
			continue
		}
		if s.responseID != "" {
			if seenResp[s.responseID] {
				continue
			}
			seenResp[s.responseID] = true
		}
		count++
		sumInp += s.input
		sumOut += s.output
		sumCache += s.cacheRead
		sumThink += s.thinking
		modelForCost := strings.TrimSpace(s.model)
		if modelForCost == "" {
			modelForCost = lastModel
		}
		if modelForCost == "" {
			modelForCost = strings.TrimSpace(item.ModelName)
		}
		sumCost += cockpit.CalculateCostUSD(modelForCost, s.input, s.cacheRead, s.output)
	}
	if count == 0 {
		return
	}
	item.SessionSteps = count
	item.SessionInputTokens = sumInp
	item.SessionOutputTokens = sumOut
	item.SessionCacheReadTokens = sumCache
	item.SessionThinkingTokens = sumThink
	item.SessionTotalTokens = sumInp + sumOut + sumCache
	item.SessionCostUSD = math.Round(sumCost*10000) / 10000
	if promptTotal := sumInp + sumCache; promptTotal > 0 {
		rate := float64(sumCache) / float64(promptTotal) * 100.0
		rounded := float64(int(rate*10+0.5)) / 10
		item.SessionCacheHitRate = &rounded
	} else {
		item.SessionCacheHitRate = nil
	}
}

func mergeSessionStepUsages(prev []sessionStepUsage, offset int, slice []sessionStepUsage) []sessionStepUsage {
	if offset <= 0 {
		out := make([]sessionStepUsage, len(slice))
		copy(out, slice)
		return out
	}
	totalLen := offset + len(slice)
	out := make([]sessionStepUsage, totalLen)
	if len(prev) > 0 {
		n := len(prev)
		if n > offset {
			n = offset
		}
		copy(out[:n], prev[:n])
	}
	copy(out[offset:], slice)
	return out
}

var (
	contextMeterCacheMu sync.Mutex
	contextMeterCache   = make(map[string]contextMeterCacheEntry)
)

const (
	// Prompt Cache miss streak guard thresholds (suppresses cold-start and tiny-context false positives).
	cacheMissMinPromptTokens int64   = 24000
	cacheMissHitRateCeiling  float64 = 20.0
	cacheMissStreakThreshold int     = 2
)

// FetchContextMeter returns the latest ContextMeterItem for cascadeID using active upstream credentials.
// Reuses the short in-memory cache so background watchers incur minimal RPC overhead.
func (p *Proxy) FetchContextMeter(cascadeID string) *ContextMeterItem {
	port, token := p.ActiveUpstream()
	if port <= 0 || strings.TrimSpace(cascadeID) == "" {
		return nil
	}
	return p.fetchUpstreamGeneratorMetadata(cascadeID, port, token)
}

// extractLastActivityAt finds the most recent valid RFC3339 timestamp across trajectory steps.
func extractLastActivityAt(steps []TrajectoryStep) string {
	for i := len(steps) - 1; i >= 0; i-- {
		m := steps[i].Metadata
		for _, raw := range []string{m.CompletedAt, m.StartedAt, m.CreatedAt} {
			s := strings.TrimSpace(raw)
			if s == "" {
				continue
			}
			if ts, err := time.Parse(time.RFC3339Nano, s); err == nil {
				return ts.UTC().Format(time.RFC3339)
			}
			if ts, err := time.Parse(time.RFC3339, s); err == nil {
				return ts.UTC().Format(time.RFC3339)
			}
		}
	}
	return ""
}

// withLastActivityAt returns a shallow copy of cm annotated with lastActivityAt so cached entries stay race-free.
func withLastActivityAt(cm *ContextMeterItem, lastActivityAt string) *ContextMeterItem {
	if cm == nil || lastActivityAt == "" {
		return cm
	}
	cp := *cm
	cp.LastActivityAt = lastActivityAt
	return &cp
}

// sanitizeContextMeterForClient strips miss_streak warning signals from client-facing payloads
// while the cache-miss guard is in shadow observation or disabled mode, preserving passive
// read-only metrics (token counts, cacheHitRate, stale lastActivityAt) without mutating cached entries.
func sanitizeContextMeterForClient(cm *ContextMeterItem) *ContextMeterItem {
	if cm == nil || config.IsCacheMissAlertLive() {
		return cm
	}
	if cm.CacheMissStreak == 0 && cm.CacheMissTokens == 0 && cm.CacheStatus != "miss_streak" {
		return cm
	}
	cp := *cm
	cp.CacheMissStreak = 0
	cp.CacheMissTokens = 0
	if cp.CacheStatus == "miss_streak" {
		cp.CacheStatus = "healthy"
	}
	return &cp
}

// fetchUpstreamGeneratorMetadata queries GetCascadeTrajectoryGeneratorMetadata from LanguageServer
func (p *Proxy) fetchUpstreamGeneratorMetadata(cascadeID string, port int, token string) *ContextMeterItem {
	if port <= 0 || cascadeID == "" {
		return nil
	}
	cascadeID = ResolveActiveCascadeID(cascadeID)

	reqOffset := 0
	var prevItem *ContextMeterItem
	var prevSteps []sessionStepUsage
	contextMeterCacheMu.Lock()
	if entry, ok := contextMeterCache[cascadeID]; ok {
		if time.Since(entry.at) < 1200*time.Millisecond {
			cached := entry.item
			contextMeterCacheMu.Unlock()
			return p.enrichContextMeter(cascadeID, cached)
		}
		reqOffset = entry.nextOffset
		prevItem = entry.item
		prevSteps = entry.steps
	}
	contextMeterCacheMu.Unlock()

	doQuery := func(offset int) (*ContextMeterItem, generatorMetaSummary, bool) {
		buf := GetSmallBuffer()
		defer PutSmallBuffer(buf)
		buf.WriteString(`{"cascadeId":`)
		buf.WriteString(strconv.Quote(cascadeID))
		buf.WriteString(`,"generatorMetadataOffset":`)
		buf.WriteString(strconv.Itoa(offset))
		buf.WriteString(`,"includeMessages":false}`)

		url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetCascadeTrajectoryGeneratorMetadata", port)
		req, err := http.NewRequest(http.MethodPost, url, buf)
		if err != nil {
			return nil, generatorMetaSummary{}, false
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Connect-Protocol-Version", "1")
		if token != "" {
			req.Header.Set("x-codeium-csrf-token", token)
		}

		resp, err := p.shortClient.Do(req)
		if err != nil {
			return nil, generatorMetaSummary{}, false
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, generatorMetaSummary{}, false
		}
		item, sum := parseGeneratorMetadataDetailedWithOffset(resp.Body, offset)
		return item, sum, true
	}

	item, sum, ok := doQuery(reqOffset)
	if !ok {
		return p.enrichContextMeter(cascadeID, prevItem)
	}
	// If trajectory reverted or tail slice had no chat model metrics, fall back to full offset=0 once.
	if reqOffset > 0 && (item == nil || (!sum.hasMetrics && prevItem != nil)) {
		reqOffset = 0
		item, sum, ok = doQuery(0)
		if !ok {
			return p.enrichContextMeter(cascadeID, prevItem)
		}
	}
	if item == nil {
		return p.enrichContextMeter(cascadeID, nil)
	}

	if !sum.explicitMax && sum.model != "" {
		if liveMax := liveModels.maxTokensFor(sum.model); liveMax > 0 {
			item.MaxTokens = liveMax
			pct := 0.0
			if item.UsedTokens > 0 {
				pct = float64(item.UsedTokens) / float64(liveMax) * 100.0
				if pct > 100.0 {
					pct = 100.0
				}
			}
			item.Percentage = float64(int(pct*10+0.5)) / 10
		}
	}

	mergedSteps := mergeSessionStepUsages(prevSteps, reqOffset, sum.stepUsages)
	applySessionUsageTotals(item, mergedSteps)

	totalCount := reqOffset + sum.count
	nextOffset := 0
	if totalCount > 5 {
		nextOffset = totalCount - 5
	}

	contextMeterCacheMu.Lock()
	if len(contextMeterCache) > 200 {
		contextMeterCache = make(map[string]contextMeterCacheEntry)
	}
	contextMeterCache[cascadeID] = contextMeterCacheEntry{
		item:       item,
		at:         time.Now(),
		nextOffset: nextOffset,
		steps:      mergedSteps,
	}
	contextMeterCacheMu.Unlock()

	return p.enrichContextMeter(cascadeID, item)
}

func (p *Proxy) enrichContextMeter(cascadeID string, base *ContextMeterItem) *ContextMeterItem {
	return overlaySessionTokenUsage(cascadeID, p.overlayLiveContextUsage(cascadeID, base))
}

func overlaySessionTokenUsage(cascadeID string, base *ContextMeterItem) *ContextMeterItem {
	if base == nil || base.SessionSteps > 0 || cascadeID == "" || resolveSessionTokenUsage == nil {
		return base
	}
	usage := resolveSessionTokenUsage(cascadeID)
	if usage == nil || usage.Steps <= 0 {
		return base
	}
	cp := *base
	cp.SessionSteps = usage.Steps
	cp.SessionInputTokens = usage.InputTokens
	cp.SessionOutputTokens = usage.OutputTokens
	cp.SessionCacheReadTokens = usage.CacheReadTokens
	cp.SessionThinkingTokens = usage.ThinkingTokens
	cp.SessionTotalTokens = usage.InputTokens + usage.OutputTokens + usage.CacheReadTokens
	cp.SessionCostUSD = usage.EstimatedCostUSD
	if usage.CacheHitRate != nil {
		rate := *usage.CacheHitRate
		cp.SessionCacheHitRate = &rate
	}
	return &cp
}

func (p *Proxy) overlayLiveContextUsage(cascadeID string, base *ContextMeterItem) *ContextMeterItem {
	used, maxTok, ok := p.agentStateHub.LiveContextUsage(cascadeID, 30*time.Second)
	if !ok {
		return base
	}
	var out ContextMeterItem
	if base != nil {
		out = *base
	}
	if used > 0 {
		out.UsedTokens = used
	}
	if maxTok > 0 {
		out.MaxTokens = maxTok
	}
	if out.MaxTokens > 0 && out.UsedTokens > 0 {
		pct := float64(out.UsedTokens) / float64(out.MaxTokens) * 100.0
		if pct > 100.0 {
			pct = 100.0
		}
		out.Percentage = float64(int(pct*10+0.5)) / 10
		out.IsOverflow = out.UsedTokens >= out.MaxTokens
	}
	return &out
}

// flexInt64 decodes ProtoJSON int64 fields encoded as either JSON numbers or quoted strings.
type flexInt64 int64

func (f *flexInt64) UnmarshalJSON(data []byte) error {
	s := strings.TrimSpace(string(data))
	if s == "" || s == "null" || s == `""` {
		*f = 0
		return nil
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		unquoted, err := strconv.Unquote(s)
		if err != nil {
			return err
		}
		s = strings.TrimSpace(unquoted)
		if s == "" {
			*f = 0
			return nil
		}
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*f = flexInt64(v)
	return nil
}

type generatorMetaSummary struct {
	count       int
	model       string
	explicitMax bool
	hasMetrics  bool
	stepUsages  []sessionStepUsage
}

func parseGeneratorMetadata(r io.Reader) *ContextMeterItem {
	item, _ := parseGeneratorMetadataDetailedWithOffset(r, 0)
	return item
}

func parseGeneratorMetadataDetailed(r io.Reader) (*ContextMeterItem, generatorMetaSummary) {
	return parseGeneratorMetadataDetailedWithOffset(r, 0)
}

func parseGeneratorMetadataDetailedWithOffset(r io.Reader, offset int) (*ContextMeterItem, generatorMetaSummary) {
	var metaResp struct {
		GeneratorMetadata []struct {
			ChatModel struct {
				Model             string `json:"model"`
				ModelDisplayName  string `json:"modelDisplayName"`
				TimeToFirstToken  string `json:"timeToFirstToken"`
				StreamingDuration string `json:"streamingDuration"`
				ChatStartMetadata *struct {
					ContextWindowMetadata *struct {
						MaxContextTokens    flexInt64 `json:"maxContextTokens"`
						EstimatedTokensUsed flexInt64 `json:"estimatedTokensUsed"`
					} `json:"contextWindowMetadata"`
				} `json:"chatStartMetadata"`
				Usage *struct {
					InputTokens              flexInt64 `json:"inputTokens"`
					OutputTokens             flexInt64 `json:"outputTokens"`
					CacheCreationTokens      flexInt64 `json:"cacheCreationTokens"`
					CacheCreationInputTokens flexInt64 `json:"cacheCreationInputTokens"`
					CacheReadTokens          flexInt64 `json:"cacheReadTokens"`
					ThinkingOutputTokens     flexInt64 `json:"thinkingOutputTokens"`
					ResponseID               string    `json:"responseId"`
				} `json:"usage"`
				RetryInfos []struct {
					Error string `json:"error"`
				} `json:"retryInfos"`
			} `json:"chatModel"`
		} `json:"generatorMetadata"`
	}

	if err := json.NewDecoder(r).Decode(&metaResp); err != nil || len(metaResp.GeneratorMetadata) == 0 {
		return nil, generatorMetaSummary{}
	}

	type usageStepSample struct {
		uncachedInput int64
		cacheRead     int64
		promptInput   int64
		hitRate       float64
	}
	usageSamples := make([]usageStepSample, 0, len(metaResp.GeneratorMetadata))
	stepUsages := make([]sessionStepUsage, len(metaResp.GeneratorMetadata))
	for i, entry := range metaResp.GeneratorMetadata {
		u := entry.ChatModel.Usage
		if u == nil {
			continue
		}
		stepInp := int64(u.InputTokens)
		stepCacheCreate := int64(u.CacheCreationTokens)
		if stepCacheCreate == 0 {
			stepCacheCreate = int64(u.CacheCreationInputTokens)
		}
		if stepCacheCreate > 0 {
			stepInp += stepCacheCreate
		}
		stepCacheRead := int64(u.CacheReadTokens)
		stepOut := int64(u.OutputTokens)
		stepThink := int64(u.ThinkingOutputTokens)
		stepModel := strings.TrimSpace(entry.ChatModel.ModelDisplayName)
		if stepModel == "" {
			stepModel = strings.TrimSpace(entry.ChatModel.Model)
		}
		if stepInp > 0 || stepOut > 0 || stepCacheRead > 0 || stepThink > 0 {
			stepUsages[i] = sessionStepUsage{
				input:      stepInp,
				output:     stepOut,
				cacheRead:  stepCacheRead,
				thinking:   stepThink,
				model:      stepModel,
				responseID: strings.TrimSpace(u.ResponseID),
				hasUsage:   true,
			}
		}
		promptInp := stepInp + stepCacheRead
		if promptInp <= 0 {
			continue
		}
		hitRate := float64(stepCacheRead) / float64(promptInp) * 100.0
		usageSamples = append(usageSamples, usageStepSample{
			uncachedInput: stepInp,
			cacheRead:     stepCacheRead,
			promptInput:   promptInp,
			hitRate:       hitRate,
		})
	}

	selected := metaResp.GeneratorMetadata[len(metaResp.GeneratorMetadata)-1]
	hasMetrics := false
	for i := len(metaResp.GeneratorMetadata) - 1; i >= 0; i-- {
		cm := metaResp.GeneratorMetadata[i].ChatModel
		if cm.Usage != nil || cm.ChatStartMetadata != nil || cm.Model != "" {
			selected = metaResp.GeneratorMetadata[i]
			hasMetrics = cm.Usage != nil || cm.ChatStartMetadata != nil
			break
		}
	}
	cm := selected.ChatModel

	var maxTokens, usedTokens int64
	if cm.ChatStartMetadata != nil && cm.ChatStartMetadata.ContextWindowMetadata != nil {
		maxTokens = int64(cm.ChatStartMetadata.ContextWindowMetadata.MaxContextTokens)
		usedTokens = int64(cm.ChatStartMetadata.ContextWindowMetadata.EstimatedTokensUsed)
	}
	explicitMax := maxTokens > 0

	var inp, outp, capTokens, thinking int64
	if cm.Usage != nil {
		inp = int64(cm.Usage.InputTokens)
		cacheCreate := int64(cm.Usage.CacheCreationTokens)
		if cacheCreate == 0 {
			cacheCreate = int64(cm.Usage.CacheCreationInputTokens)
		}
		if cacheCreate > 0 {
			inp += cacheCreate
		}
		outp = int64(cm.Usage.OutputTokens)
		capTokens = int64(cm.Usage.CacheReadTokens)
		thinking = int64(cm.Usage.ThinkingOutputTokens)
		if usedTokens == 0 {
			usedTokens = inp + capTokens + outp
		}
	}

	var cacheHitRate *float64
	cacheMissStreak := 0
	var cacheMissTokens int64
	cacheStatus := ""

	if len(usageSamples) > 0 {
		latest := usageSamples[len(usageSamples)-1]
		rounded := float64(int(latest.hitRate*10+0.5)) / 10
		cacheHitRate = &rounded

		for i := len(usageSamples) - 1; i >= 0; i-- {
			// Skip the very first prompt in a brand-new session (normal cold cache write).
			if offset == 0 && i == 0 {
				break
			}
			s := usageSamples[i]
			if s.promptInput >= cacheMissMinPromptTokens && s.hitRate < cacheMissHitRateCeiling {
				cacheMissStreak++
				cacheMissTokens += s.uncachedInput
			} else {
				break
			}
		}

		switch {
		case cacheMissStreak >= cacheMissStreakThreshold:
			cacheStatus = "miss_streak"
		case offset == 0 && len(usageSamples) == 1 && latest.cacheRead == 0:
			cacheStatus = "cold"
		default:
			cacheStatus = "healthy"
		}
	}

	if maxTokens <= 0 {
		maxTokens = 200000
	}

	percentage := 0.0
	if maxTokens > 0 && usedTokens > 0 {
		percentage = float64(usedTokens) / float64(maxTokens) * 100.0
		if percentage > 100.0 {
			percentage = 100.0
		}
	}

	ttftSec := parseDurationToSec(cm.TimeToFirstToken)
	streamSec := parseDurationToSec(cm.StreamingDuration)
	var tps float64
	if outp > 0 && streamSec > 0.05 {
		tps = float64(outp) / streamSec
	}

	lastErr := ""
	if len(cm.RetryInfos) > 0 {
		lastErr = cm.RetryInfos[len(cm.RetryInfos)-1].Error
	}
	errKind := ClassifyErrorKind(lastErr)

	modelName := strings.TrimSpace(cm.ModelDisplayName)
	if modelName == "" {
		modelName = strings.TrimSpace(cm.Model)
	}

	item := &ContextMeterItem{
		UsedTokens:        usedTokens,
		MaxTokens:         maxTokens,
		Percentage:        float64(int(percentage*10+0.5)) / 10,
		InputTokens:       inp,
		OutputTokens:      outp,
		CacheReadTokens:   capTokens,
		ThinkingTokens:    thinking,
		TimeToFirstToken:  ttftSec,
		StreamingDuration: streamSec,
		TokensPerSecond:   float64(int(tps*10+0.5)) / 10,
		IsOverflow:        errKind == "context_overflow",
		ErrorKind:         errKind,
		CacheHitRate:      cacheHitRate,
		CacheMissStreak:   cacheMissStreak,
		CacheMissTokens:   cacheMissTokens,
		CacheStatus:       cacheStatus,
		ModelName:         modelName,
	}
	if offset == 0 {
		applySessionUsageTotals(item, stepUsages)
	}

	return item, generatorMetaSummary{
		count:       len(metaResp.GeneratorMetadata),
		model:       cm.Model,
		explicitMax: explicitMax,
		hasMetrics:  hasMetrics,
		stepUsages:  stepUsages,
	}
}

func parseDurationToSec(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	s = strings.TrimSuffix(s, "s")
	if val, err := strconv.ParseFloat(s, 64); err == nil {
		return val
	}
	return 0
}

