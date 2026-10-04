package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
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
	tc.inFlightTrajMu.Lock()
	if flight, exists := tc.inFlightTraj[cascadeID]; exists {
		tc.inFlightTrajMu.Unlock()
		flight.wg.Wait()
		return flight.res, flight.err
	}
	flight := &inFlightTraj{}
	flight.wg.Add(1)
	tc.inFlightTraj[cascadeID] = flight
	tc.inFlightTrajMu.Unlock()

	defer func() {
		tc.inFlightTrajMu.Lock()
		delete(tc.inFlightTraj, cascadeID)
		tc.inFlightTrajMu.Unlock()
		flight.wg.Done()
	}()

	// Status-aware TTL: completed sessions rarely change, so cache them longer.
	// But if title is missing or session has few/no steps, keep TTL short (1.5s)
	// so newly generated titles/summaries are quickly discovered.
	maxAge := 800 * time.Millisecond
	tc.trajCacheMu.RLock()
	if cached, ok := tc.trajCache[cascadeID]; ok {
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
			flight.err = ctx.Err()
			return nil, ctx.Err()
		}
		if loadErr := p.LoadTrajectory(cascadeID, port, token); loadErr == nil {
			tc.trajCacheMu.Lock()
			delete(tc.trajCache, cascadeID)
			tc.trajCacheMu.Unlock()
			if retryResp, retryErr := p.fetchUpstreamTrajectoryWithMaxAgeContext(ctx, cascadeID, port, token, 0); retryErr == nil && retryResp != nil {
				flight.res = retryResp
				return retryResp, nil
			}
		}
	}
	flight.res = resp
	flight.err = err
	return resp, err
}

func (p *Proxy) lookupCascadeTitle(cascadeID string, port int, token string) string {
	if cascadeID == "" {
		return ""
	}

	defaultTrajCache.cascadeTitlesMu.RLock()
	cachedTitle, ok := defaultTrajCache.cascadeTitles[cascadeID]
	defaultTrajCache.cascadeTitlesMu.RUnlock()

	if ok && cachedTitle != "" && cachedTitle != "未命名会话" {
		return cachedTitle
	}

	if t := readAnnotationTitle(cascadeID); t != "" && t != "未命名会话" {
		defaultTrajCache.cascadeTitlesMu.Lock()
		if len(defaultTrajCache.cascadeTitles) > maxMetadataMapSize {
			count := 0
			for k := range defaultTrajCache.cascadeTitles {
				delete(defaultTrajCache.cascadeTitles, k)
				count++
				if count >= maxMetadataMapSize/2 {
					break
				}
			}
		}
		defaultTrajCache.cascadeTitles[cascadeID] = t
		defaultTrajCache.cascadeTitlesMu.Unlock()
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
				if len(defaultTrajCache.cascadeTitles) > maxMetadataMapSize {
					count := 0
					for k := range defaultTrajCache.cascadeTitles {
						delete(defaultTrajCache.cascadeTitles, k)
						count++
						if count >= maxMetadataMapSize/2 {
							break
						}
					}
				}
				for cid, t := range summaries {
					if t != "" && t != "未命名会话" {
						defaultTrajCache.cascadeTitles[cid] = t
					}
				}
				newTitle := defaultTrajCache.cascadeTitles[cascadeID]
				defaultTrajCache.cascadeTitlesMu.Unlock()
				if newTitle != "" && newTitle != "未命名会话" {
					return newTitle
				}
			} else {
				defaultTrajCache.cascadeTitlesMu.Unlock()
			}
		}
	}

	// Fallback to cached trajectory first user prompt
	defaultTrajCache.trajCacheMu.RLock()
	entry, hasEntry := defaultTrajCache.trajCache[cascadeID]
	defaultTrajCache.trajCacheMu.RUnlock()
	if hasEntry && entry != nil && entry.data != nil {
		for _, s := range entry.data.Trajectory.Steps {
			if s.Type == "CORTEX_STEP_TYPE_USER_INPUT" {
				if t := extractTitleFromUserInput(s); t != "" && t != "未命名会话" {
					defaultTrajCache.cascadeTitlesMu.Lock()
					defaultTrajCache.cascadeTitles[cascadeID] = t
					defaultTrajCache.cascadeTitlesMu.Unlock()
					return t
				}
			}
		}
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

// SyncHistoricalTrajectories scans ~/.gemini/antigravity/conversations for historical session DBs
// and loads valid sessions into upstream language_server memory so GetAllCascadeTrajectories returns them.
func (p *Proxy) SyncHistoricalTrajectories(port int, token string) error {
	if port == 0 {
		return fmt.Errorf("upstream port not set")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	convDir := filepath.Join(home, ".gemini", "antigravity", "conversations")
	entries, err := os.ReadDir(convDir)
	if err != nil {
		return err
	}

	var candidates []string
	defaultTrajCache.loadedCascadesMu.Lock()
	if port != defaultTrajCache.lastSyncedPort {
		defaultTrajCache.loadedCascades = make(map[string]bool)
		defaultTrajCache.lastSyncedPort = port
	}
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

		candidates = append(candidates, cascadeID)
	}
	defaultTrajCache.loadedCascadesMu.Unlock()

	if len(candidates) == 0 {
		return nil
	}

	if verboseRPC {
		slog.Info(fmt.Sprintf("[Proxy] Syncing %d historical trajectories into upstream language_server...", len(candidates)))
	}

	concurrency := 2
	if concurrency > len(candidates) {
		concurrency = len(candidates)
	}

	workCh := make(chan string, len(candidates))
	for _, cid := range candidates {
		workCh <- cid
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
	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", port)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.shortClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	var data struct {
		TrajectorySummaries map[string]struct {
			Summary     string `json:"summary"`
			Annotations *struct {
				Title string `json:"title"`
			} `json:"annotations"`
		} `json:"trajectorySummaries"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
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

func (p *Proxy) fetchUpstreamTrajectoryWithMaxAgeContext(ctx context.Context, cascadeID string, port int, token string, maxAge time.Duration) (*upstreamTrajectoryResp, error) {
	if IsDeletedCascade(cascadeID) {
		return nil, fmt.Errorf("cascade trajectory %s has been deleted", cascadeID)
	}

	defaultTrajCache.trajCacheMu.RLock()
	if cached, ok := defaultTrajCache.trajCache[cascadeID]; ok {
		if time.Since(cached.fetchedAt) < maxAge {
			defaultTrajCache.trajCacheMu.RUnlock()
			return cached.data, nil
		}
	}
	defaultTrajCache.trajCacheMu.RUnlock()

	// Coalesce concurrent cache misses for the same cascade into one upstream request,
	// so N stream subscribers expiring the cache together don't each hit language_server.
	return trajFlights.do(cascadeID, func() (*upstreamTrajectoryResp, error) {
		return p.fetchUpstreamTrajectoryUncached(ctx, cascadeID, port, token)
	})
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

func (g *trajFlightGroup) do(key string, fn func() (*upstreamTrajectoryResp, error)) (*upstreamTrajectoryResp, error) {
	g.mu.Lock()
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		<-c.done
		return c.data, c.err
	}
	c := &trajFlightCall{done: make(chan struct{})}
	g.calls[key] = c
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		delete(g.calls, key)
		g.mu.Unlock()
		close(c.done)
	}()
	c.data, c.err = fn()
	return c.data, c.err
}

func (p *Proxy) fetchUpstreamTrajectoryUncached(ctx context.Context, cascadeID string, port int, token string) (*upstreamTrajectoryResp, error) {
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

	var data upstreamTrajectoryResp
	if err := json.NewDecoder(reader).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode upstream response: %w", err)
	}

	defaultTrajCache.trajCacheMu.Lock()
	defaultTrajCache.trajCache[cascadeID] = &trajectoryCacheEntry{
		fetchedAt: time.Now(),
		data:      &data,
	}
	defaultTrajCache.evictTrajCacheLocked()
	defaultTrajCache.trajCacheMu.Unlock()

	return &data, nil
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
	details := p.ParseTrajectoryDetails(rawResp)
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
	port, token := p.ActiveUpstream()
	if port == 0 {
		return nil, nil, fmt.Errorf("antigravity upstream not connected")
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", port)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.mediumClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("upstream status %d", resp.StatusCode)
	}

	var envelope struct {
		TrajectorySummaries map[string]map[string]interface{} `json:"trajectorySummaries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, nil, err
	}

	summaries := envelope.TrajectorySummaries
	if summaries == nil {
		summaries = make(map[string]map[string]interface{})
	}

	runningSubagents := make(map[string]bool)
	// Filter out internal subagent sessions while tracking parents of actively running subagents
	for id, s := range summaries {
		if isSubagentTrajectoryMap(s, id) {
			status, _ := s["status"].(string)
			if status == "CASCADE_RUN_STATUS_RUNNING" {
				if meta, ok := s["trajectoryMetadata"].(map[string]interface{}); ok {
					if parent, ok := meta["parentConversationId"].(string); ok && strings.TrimSpace(parent) != "" {
						runningSubagents[strings.TrimSpace(parent)] = true
					}
				}
			}
			delete(summaries, id)
		}
	}

	return summaries, runningSubagents, nil
}
