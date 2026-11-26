package proxy

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (p *Proxy) handleRpcProxy(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		dur := time.Since(start)
		if dur > 100*time.Millisecond &&
			!strings.Contains(r.URL.Path, "Stream") &&
			!strings.Contains(r.URL.Path, "Subscribe") &&
			!strings.Contains(r.URL.Path, "Watch") &&
			// Whitelist: these are known upstream-bound slow RPCs that are now cached at the
			// gateway layer or are inherently full-scan operations; do not spam the log.
			!strings.HasSuffix(r.URL.Path, "/GetCascadeNuxes") &&
			!strings.HasSuffix(r.URL.Path, "/GetAuthStatus") &&
			!strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories") &&
			!strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") {
			slog.Warn(fmt.Sprintf("[RPC] ⚠️ SLOW: %s in %v", r.URL.Path, dur))
		}
	}()
	if r.Method == http.MethodPost && !isReadOnlyRPC(r.URL.Path) {
		// Any non-read RPC may change the cascade list; drop the shared snapshot once it completes.
		defer p.invalidateAllTrajectories()
	}
	p.mu.RLock()
	rp := p.activeProxy
	port := p.activePort
	token := p.activeToken
	p.mu.RUnlock()

	reqPath := strings.TrimPrefix(r.URL.Path, "/api")
	if verboseRPC {
		slog.Info(fmt.Sprintf("[Proxy] RPC: %s %s", r.Method, reqPath))
	}
	if strings.HasSuffix(reqPath, "/SearchConversations") && r.Method == http.MethodPost {
		p.handleSearchConversations(w, r, rp, reqPath)
		return
	}
	if strings.HasSuffix(reqPath, "/GetAllCascadeTrajectories") && r.Method == http.MethodPost {
		if port == 0 {
			p.triggerBackgroundRescan()
		}
		p.handleGetAllCascadeTrajectories(w, r, port, token)
		return
	}
	if rp == nil {
		port, token, rp = p.ensureActiveUpstream(r.Context())
	}
	if rp == nil {
		p.writeUpstreamUnavailable(w)
		return
	}
	if strings.HasSuffix(reqPath, "/SendUserCascadeMessage") && r.Method == http.MethodPost {
		p.handleSendUserCascadeMessage(w, r, rp, reqPath, port, token)
		return
	}
	if strings.HasSuffix(reqPath, "/JetboxWriteState") && r.Method == http.MethodPost {
		p.handleJetboxWriteState(w, r, rp, reqPath)
		return
	}
	if strings.HasSuffix(reqPath, "/StartCascade") && r.Method == http.MethodPost {
		p.handleStartCascadeProxy(w, r, rp, reqPath)
		return
	}
	if strings.HasSuffix(reqPath, "/GetAllCascadeTrajectories") && r.Method == http.MethodPost {
		p.handleGetAllCascadeTrajectories(w, r, port, token)
		return
	}
	if strings.HasSuffix(reqPath, "/GetCascadeNuxes") && r.Method == http.MethodPost {
		p.handleGetCascadeNuxes(w, r, rp, reqPath)
		return
	}
	if strings.HasSuffix(reqPath, "/GetAuthStatus") && r.Method == http.MethodPost {
		p.handleGetAuthStatus(w, r, rp, reqPath)
		return
	}
	if strings.HasSuffix(reqPath, "/GetCascadeTrajectory") && r.Method == http.MethodPost {
		p.handleGetCascadeTrajectoryRPC(w, r, port, token)
		return
	}
	if (strings.HasSuffix(reqPath, "/UpdateConversationAnnotations") || strings.HasSuffix(reqPath, "/SetCascadeTrajectoryMetadata")) && r.Method == http.MethodPost {
		p.handleUpdateConversationAnnotations(w, r, rp, reqPath)
		return
	}
	if strings.HasSuffix(reqPath, "/DeleteCascadeTrajectory") && r.Method == http.MethodPost {
		p.handleDeleteCascadeTrajectory(w, r, rp, reqPath)
		return
	}
	if strings.HasSuffix(reqPath, "/DeleteAgentMessage") && r.Method == http.MethodPost {
		p.handleDeleteAgentMessage(w, r, rp, reqPath)
		return
	}
	if strings.HasSuffix(reqPath, "/ReadProjects") && r.Method == http.MethodPost {
		p.handleReadProjects(w, r, rp, reqPath)
		return
	}
	if (strings.HasSuffix(reqPath, "/CancelCascadeInvocation") || strings.HasSuffix(reqPath, "/ForceStopCascadeTree")) && r.Method == http.MethodPost {
		p.handleCancelCascadeInvocation(w, r, rp, reqPath)
		return
	}

	// Clone the request to avoid mutating the original before forwarding
	fwdReq := r.Clone(r.Context())
	fwdReq.URL.Path = reqPath
	rp.ServeHTTP(w, fwdReq)
}

// handleGetCascadeNuxes proxies GetCascadeNuxes with a 60s gateway-level cache.
// The upstream language_server takes 300-900ms to respond; NUX data is nearly static
// and caching it eliminates the stall on every desktop workbench initialization.
func (p *Proxy) handleGetCascadeNuxes(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string) {
	// Fast path: return cached response if still fresh
	p.nuxCacheMu.RLock()
	body := p.nuxCacheBody
	headers := p.nuxCacheHeaders
	cachedAt := p.nuxCachedAt
	p.nuxCacheMu.RUnlock()

	if len(body) > 0 && time.Since(cachedAt) < nuxCacheTTL {
		for k, v := range headers {
			w.Header()[k] = v
		}
		w.Header().Set("X-Gateway-Cache", "HIT")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
		return
	}

	// Slow path: fetch from upstream and cache the result
	rec := newBufferedResponseWriter()
	defer rec.release()

	fwdReq := r.Clone(r.Context())
	fwdReq.URL.Path = reqPath
	rp.ServeHTTP(rec, fwdReq)

	if rec.statusCode == http.StatusOK && rec.body.Len() > 0 {
		respBytes := make([]byte, rec.body.Len())
		copy(respBytes, rec.body.Bytes())
		savedHeaders := rec.header.Clone()

		p.nuxCacheMu.Lock()
		p.nuxCacheBody = respBytes
		p.nuxCacheHeaders = savedHeaders
		p.nuxCachedAt = time.Now()
		p.nuxCacheMu.Unlock()

		for k, v := range rec.header {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.statusCode)
		w.Write(respBytes)
		return
	}

	// Upstream error: serve cached stale data if available, otherwise pass through
	p.nuxCacheMu.RLock()
	staleBody := p.nuxCacheBody
	staleHeaders := p.nuxCacheHeaders
	p.nuxCacheMu.RUnlock()

	if len(staleBody) > 0 {
		for k, v := range staleHeaders {
			w.Header()[k] = v
		}
		w.Header().Set("X-Gateway-Cache", "STALE")
		w.WriteHeader(http.StatusOK)
		w.Write(staleBody)
		return
	}

	// No cache at all — pass through whatever upstream returned
	for k, v := range rec.header {
		w.Header()[k] = v
	}
	w.WriteHeader(rec.statusCode)
	w.Write(rec.body.Bytes())
}

// handleGetAuthStatus proxies GetAuthStatus with a 60s gateway-level cache.
// Upstream language_server makes a slow outbound call to Google Auth servers on every call,
// taking 3.5s - 6.5s. Caching it eliminates the huge initial workbench freeze.
func (p *Proxy) handleGetAuthStatus(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string) {
	// Fast path: return cached response if still fresh
	p.authStatusCacheMu.RLock()
	body := p.authStatusCacheBody
	headers := p.authStatusCacheHeaders
	cachedAt := p.authStatusCachedAt
	p.authStatusCacheMu.RUnlock()

	if len(body) > 0 && time.Since(cachedAt) < authStatusCacheTTL {
		for k, v := range headers {
			w.Header()[k] = v
		}
		w.Header().Set("X-Gateway-Cache", "HIT")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
		return
	}

	// Slow path: fetch from upstream and cache the result
	rec := newBufferedResponseWriter()
	defer rec.release()

	fwdReq := r.Clone(r.Context())
	fwdReq.URL.Path = reqPath
	rp.ServeHTTP(rec, fwdReq)

	if rec.statusCode == http.StatusOK && rec.body.Len() > 0 {
		respBytes := make([]byte, rec.body.Len())
		copy(respBytes, rec.body.Bytes())
		savedHeaders := rec.header.Clone()

		p.authStatusCacheMu.Lock()
		p.authStatusCacheBody = respBytes
		p.authStatusCacheHeaders = savedHeaders
		p.authStatusCachedAt = time.Now()
		p.authStatusCacheMu.Unlock()

		for k, v := range rec.header {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.statusCode)
		w.Write(respBytes)
		return
	}

	// Upstream error: serve cached stale data if available, otherwise pass through
	p.authStatusCacheMu.RLock()
	staleBody := p.authStatusCacheBody
	staleHeaders := p.authStatusCacheHeaders
	p.authStatusCacheMu.RUnlock()

	if len(staleBody) > 0 {
		for k, v := range staleHeaders {
			w.Header()[k] = v
		}
		w.Header().Set("X-Gateway-Cache", "STALE")
		w.WriteHeader(http.StatusOK)
		w.Write(staleBody)
		return
	}

	for k, v := range rec.header {
		w.Header()[k] = v
	}
	w.WriteHeader(rec.statusCode)
	w.Write(rec.body.Bytes())
}

// handleGetCascadeTrajectoryRPC intercepts the standard Connect-RPC GetCascadeTrajectory
// call from the desktop workbench and routes it through the gateway's TrajectoryCache,
// eliminating redundant upstream fetches when the same cascade is already cached.
func (p *Proxy) handleGetCascadeTrajectoryRPC(w http.ResponseWriter, r *http.Request, port int, token string) {
	bodyBytes, cleanup, err := readBodyToPool(r.Body, 64*1024)
	if err != nil {
		writeJSONError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer cleanup()

	var rawFields map[string]json.RawMessage
	var req struct {
		CascadeID string `json:"cascadeId"`
	}
	if len(bodyBytes) > 0 {
		_ = json.Unmarshal(bodyBytes, &rawFields)
		_ = json.Unmarshal(bodyBytes, &req)
	}

	cascadeID := strings.TrimSpace(req.CascadeID)
	hasExtraFields := len(rawFields) > 1
	if cascadeID == "" || port == 0 || hasExtraFields {
		// Missing cascadeId, extra request options, or no upstream — fall through to raw proxy
		p.mu.RLock()
		rp := p.activeProxy
		p.mu.RUnlock()
		if rp == nil {
			p.writeUpstreamUnavailable(w)
			return
		}
		fwdReq := r.Clone(r.Context())
		fwdReq.URL.Path = "/exa.language_server_pb.LanguageServerService/GetCascadeTrajectory"
		fwdReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		fwdReq.ContentLength = int64(len(bodyBytes))
		rp.ServeHTTP(w, fwdReq)
		return
	}

	// Use the same cache as the stream / messages endpoints.
	// For running cascades use a short 300ms maxAge so the desktop stays responsive;
	// for idle cascades 15s is fine since the content is static.
	maxAge := 300 * time.Millisecond
	defaultTrajCache.trajCacheMu.RLock()
	if cached, ok := defaultTrajCache.trajCache[cascadeID]; ok && cached != nil && cached.port == port && cached.data != nil {
		if cached.data.Status != "" && cached.data.Status != "CASCADE_RUN_STATUS_RUNNING" {
			maxAge = 15 * time.Second
		}
	}
	defaultTrajCache.trajCacheMu.RUnlock()

	rawResp, fetchErr := p.fetchUpstreamTrajectoryWithMaxAge(cascadeID, port, token, maxAge)
	if fetchErr != nil {
		// Forward raw to upstream on cache miss / error
		p.mu.RLock()
		rp := p.activeProxy
		p.mu.RUnlock()
		if rp == nil {
			p.writeUpstreamUnavailable(w)
			return
		}
		fwdReq := r.Clone(r.Context())
		fwdReq.URL.Path = "/exa.language_server_pb.LanguageServerService/GetCascadeTrajectory"
		fwdReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		fwdReq.ContentLength = int64(len(bodyBytes))
		rp.ServeHTTP(w, fwdReq)
		return
	}

	var respBytes []byte
	defaultTrajCache.trajCacheMu.RLock()
	if cached, ok := defaultTrajCache.trajCache[cascadeID]; ok && cached != nil && cached.data == rawResp && len(cached.rawJSON) > 0 {
		respBytes = cached.rawJSON
	}
	defaultTrajCache.trajCacheMu.RUnlock()

	if len(respBytes) == 0 {
		var encErr error
		respBytes, encErr = json.Marshal(rawResp)
		if encErr != nil {
			writeJSONError(w, "Failed to encode trajectory response", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Connect-Protocol-Version", "1")
	w.Header().Set("X-Gateway-Cache", "HIT")
	w.Header().Set("Content-Length", strconv.Itoa(len(respBytes)))
	w.WriteHeader(http.StatusOK)
	w.Write(respBytes)
}

func (p *Proxy) handleCancelCascadeInvocation(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string) {
	bodyBytes, cleanup, err := readBodyToPool(r.Body, 64*1024)
	if err != nil {
		writeJSONError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer cleanup()

	var rawMap map[string]interface{}
	targetID := ""
	if err := json.Unmarshal(bodyBytes, &rawMap); err == nil && rawMap != nil {
		if cid, ok := rawMap["cascadeId"].(string); ok {
			targetID = strings.TrimSpace(cid)
		}
		if targetID == "" {
			if cid, ok := rawMap["conversationId"].(string); ok {
				targetID = strings.TrimSpace(cid)
			}
		}
		// Antigravity 2.30+ supports killBackgroundTasks on CancelCascadeInvocation.
		// Default to true when the caller did not specify it, unless the session has a standing daemon (IsDaemon=true).
		if strings.HasSuffix(reqPath, "/CancelCascadeInvocation") {
			if _, hasKill := rawMap["killBackgroundTasks"]; !hasKill {
				hasDaemon := false
				if targetID != "" {
					defaultTrajCache.trajCacheMu.RLock()
					entry := defaultTrajCache.trajCache[targetID]
					defaultTrajCache.trajCacheMu.RUnlock()
					if entry != nil && entry.data != nil {
						for _, t := range buildRunningTasks(entry.data.Trajectory.Steps) {
							if t.IsDaemon {
								hasDaemon = true
								break
							}
						}
					}
				}
				if !hasDaemon {
					rawMap["killBackgroundTasks"] = true
					if patched, err := json.Marshal(rawMap); err == nil {
						bodyBytes = patched
					}
				}
			}
		}
	}

	fwdReq := r.Clone(r.Context())
	fwdReq.URL.Path = reqPath
	fwdReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	fwdReq.ContentLength = int64(len(bodyBytes))
	fwdReq.Header.Set("Content-Length", strconv.Itoa(len(bodyBytes)))

	rp.ServeHTTP(w, fwdReq)

	if targetID != "" {
		ClearTrajectoryCache(targetID)
		ClearPendingMessagesCache(targetID)
		p.notifyStreamTouch(targetID)
		if verboseRPC {
			slog.Info(fmt.Sprintf("[Proxy] CancelCascadeInvocation forwarded and cache invalidated for: %s", shortCascadeID(targetID)))
		}
	}
}

type bufferedResponseWriter struct {
	header     http.Header
	body       *bytes.Buffer
	statusCode int
}

func newBufferedResponseWriter() *bufferedResponseWriter {
	return &bufferedResponseWriter{
		header:     make(http.Header),
		body:       GetLargeBuffer(), // P3: reuse pooled buffer instead of heap-allocating
		statusCode: http.StatusOK,
	}
}

// release returns the body buffer to the pool. Must be called after the response body
// has been fully consumed (i.e. after w.Write(rec.body.Bytes())).
func (b *bufferedResponseWriter) release() {
	PutLargeBuffer(b.body)
	b.body = nil
}

func (b *bufferedResponseWriter) Header() http.Header {
	return b.header
}

func (b *bufferedResponseWriter) WriteHeader(code int) {
	b.statusCode = code
}

func (b *bufferedResponseWriter) Write(p []byte) (int, error) {
	return b.body.Write(p)
}

func (p *Proxy) handleReadProjects(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string) {
	bodyBytes, cleanup, err := readBodyToPool(r.Body, 64*1024)
	if err != nil {
		writeJSONError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer cleanup()

	rec := newBufferedResponseWriter()
	defer rec.release()

	fwdReq := r.Clone(r.Context())
	fwdReq.URL.Path = reqPath
	fwdReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	fwdReq.ContentLength = int64(len(bodyBytes))
	fwdReq.Header.Set("Content-Length", strconv.Itoa(len(bodyBytes)))

	rp.ServeHTTP(rec, fwdReq)

	if rec.statusCode == http.StatusOK && rec.body.Len() > 0 {
		respBytes := rec.body.Bytes()
		p.projectsCacheMu.Lock()
		p.lastReadProjectsResp = make([]byte, len(respBytes))
		copy(p.lastReadProjectsResp, respBytes)
		p.projectsCacheMu.Unlock()
		syncDiscoveredWorkspaceRootsFromReadProjectsJSON(respBytes)

		for k, v := range rec.header {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.statusCode)
		w.Write(respBytes)
		return
	}

	p.projectsCacheMu.RLock()
	cached := p.lastReadProjectsResp
	p.projectsCacheMu.RUnlock()

	if len(cached) > 0 {
		slog.Info(fmt.Sprintf("[Proxy] Upstream ReadProjects returned status %d; serving cached projects list (%d bytes)", rec.statusCode, len(cached)))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Connect-Protocol-Version", "1")
		w.Header().Set("Content-Length", strconv.Itoa(len(cached)))
		w.WriteHeader(http.StatusOK)
		w.Write(cached)
		return
	}

	for k, v := range rec.header {
		w.Header()[k] = v
	}
	w.WriteHeader(rec.statusCode)
	w.Write(rec.body.Bytes())
}

func isSpaceOnly(b []byte) bool {
	return len(bytes.TrimSpace(b)) == 0
}

// writeJSONError writes a properly JSON-encoded error response.
// S6: using json.Marshal prevents injection when err.Error() contains quotes or backslashes.
func writeJSONError(w http.ResponseWriter, msg string, status int) {
	body, _ := json.Marshal(map[string]string{"error": msg})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
}

func (p *Proxy) handleGetAllCascadeTrajectories(w http.ResponseWriter, r *http.Request, port int, token string) {
	// 1. Ensure historical trajectories on disk are loaded into upstream memory asynchronously
	if port > 0 && !HasSyncedHistoricalTrajectories(port) {
		go func(prt int, tok string) {
			_ = p.SyncHistoricalTrajectories(prt, tok)
		}(port, token)
	}

	bodyBytes, cleanup, _ := readBodyToPool(r.Body, 5*1024*1024)
	defer cleanup()

	// P4: enforce the 6s budget (raised from 4s for Antigravity 2.35+ unlimited local history)
	// via a context deadline instead of allocating a new http.Client per request.
	listCtx, listCancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer listCancel()

	var rawMap map[string]json.RawMessage
	if trimmed := bytes.TrimSpace(bodyBytes); len(trimmed) == 0 || string(trimmed) == "{}" {
		// Unfiltered list: share one upstream call with the Watcher and other clients.
		snap, err := p.fetchAllTrajectoriesRaw(listCtx, port, token)
		if err != nil {
			// Zero-RPC Fallback to Antigravity 2.26 local SQLite store
			if localSummaries, lerr := FetchLocalConversationSummaries(listCtx); lerr == nil && len(localSummaries) > 0 {
				localRaw, _ := json.Marshal(map[string]interface{}{"trajectorySummaries": localSummaries})
				_ = json.Unmarshal(localRaw, &rawMap)
				w.Header().Set("X-Gateway-Fallback", "sqlite")
			} else {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
		} else if err := json.Unmarshal(snap, &rawMap); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", port)
		req, err := http.NewRequestWithContext(listCtx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Connect-Protocol-Version", "1")
		if token != "" {
			req.Header.Set("x-codeium-csrf-token", token)
		}

		resp, err := p.mediumClient.Do(req)
		if err != nil {
			if localSummaries, lerr := FetchLocalConversationSummaries(listCtx); lerr == nil && len(localSummaries) > 0 {
				localRaw, _ := json.Marshal(map[string]interface{}{"trajectorySummaries": localSummaries})
				_ = json.Unmarshal(localRaw, &rawMap)
				w.Header().Set("X-Gateway-Fallback", "sqlite")
			} else {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
		} else {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				for k, v := range resp.Header {
					w.Header()[k] = v
				}
				w.WriteHeader(resp.StatusCode)
				io.Copy(w, resp.Body)
				return
			}
			if err := json.NewDecoder(resp.Body).Decode(&rawMap); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}

	summariesRaw, ok := rawMap["trajectorySummaries"]
	if !ok {
		respBytes, _ := json.Marshal(rawMap)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(respBytes)))
		w.WriteHeader(http.StatusOK)
		w.Write(respBytes)
		return
	}

	var summaries map[string]map[string]interface{}
	if err := json.Unmarshal(summariesRaw, &summaries); err != nil {
		respBytes, _ := json.Marshal(rawMap)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(respBytes)))
		w.WriteHeader(http.StatusOK)
		w.Write(respBytes)
		return
	}

	// Enrich hierarchy metadata (parent-child relationship) with zero-subprocess memory cache + sqlite3 fast path
	EnrichSummariesHierarchy(listCtx, summaries)

	// Filter out half-deleted in-memory zombies (e.g. desktop DeleteCascadeTrajectory failed
	// on "unlinkat brain/<id>: directory not empty" after already removing conversations/<id>.db).
	filterDeletedZombieSummaries(summaries)

	// Snapshot raw upstream statuses, modification times, and subagent lineage before filtering
	// so multi-level subagent chains and swarm members propagate accurately up to their visible
	// root conversation, while Step 1 cache freshness checks still compare against raw values.
	rawStatuses := make(map[string]string, len(summaries))
	rawModTimes := make(map[string]time.Time, len(summaries))
	subagentIDs := make(map[string]bool)
	subagentParents := make(map[string]string)
	subagentRoots := make(map[string]string)
	for id, s := range summaries {
		if s == nil {
			continue
		}
		if st, ok := s["status"].(string); ok {
			rawStatuses[id] = st
		}
		if modStr, ok := s["lastModifiedTime"].(string); ok && modStr != "" {
			if t, err := parseTime(modStr); err == nil {
				rawModTimes[id] = t
			}
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

	runningSubagentParents := make(map[string]bool)
	subagentLatestModTime := make(map[string]time.Time)
	sidecarProjectIDs := fetchSidecarProjectIDs()
	resolveWorkspace := p.BuildWorkspaceResolver()
	var (
		fallbackProjectsLoaded bool
		fallbackProjects       []ProjectItem
		fallbackCfgProjects    []ProjectItem
	)
	lazyMatchProjectID := func(wsURIs []string) string {
		if !fallbackProjectsLoaded {
			fallbackProjectsLoaded = true
			fallbackCfgProjects = fetchProjectsFromGeminiConfig()
			if prjs, err := p.GetProjects(); err == nil && len(prjs) > 0 {
				fallbackProjects = prjs
			} else {
				fallbackProjects = fallbackCfgProjects
			}
		}
		return matchProjectIDFromProjects(wsURIs, fallbackProjects, fallbackCfgProjects)
	}
	isApiClient := strings.HasPrefix(r.URL.Path, "/api/")
	isWidgetLite := r.Header.Get("X-Widget-Lite") == "1"

	// Pre-index side-question parentage and semantic titles from parent annotations and in-memory tracker.
	sideParentByID := make(map[string]string)
	sideTitleByID := make(map[string]string)
	sideForkStepByID := make(map[string]int)
	for parentID, s := range summaries {
		if IsDeletedCascade(parentID) || s == nil {
			continue
		}
		ann, ok := s["annotations"].(map[string]interface{})
		if !ok || ann == nil {
			continue
		}
		if rawIDs, ok := ann["sideQuestionCascadeIds"].([]interface{}); ok {
			for _, rawID := range rawIDs {
				if sid, ok := rawID.(string); ok && strings.TrimSpace(sid) != "" {
					sideParentByID[strings.TrimSpace(sid)] = parentID
				}
			}
		}
		if rawTitles, ok := ann["sideQuestionTitles"].(map[string]interface{}); ok {
			for sid, rawTitle := range rawTitles {
				if t, ok := rawTitle.(string); ok && strings.TrimSpace(t) != "" {
					sideTitleByID[strings.TrimSpace(sid)] = strings.TrimSpace(t)
				}
			}
		}
	}
	for sid, entry := range defaultBtwTracker.snapshotAll() {
		if entry.ParentCascadeID != "" && sideParentByID[sid] == "" {
			sideParentByID[sid] = entry.ParentCascadeID
		}
		if sideTitleByID[sid] == "" {
			if t := strings.TrimSpace(firstNonEmpty(entry.Title, entry.Question)); t != "" {
				sideTitleByID[sid] = t
			}
		}
		if entry.ForkStepIndex != nil && *entry.ForkStepIndex >= 0 {
			sideForkStepByID[sid] = *entry.ForkStepIndex
		}
	}

	var keptSideQuestionIDs []string
	// Enrich missing titles, filter out subagent sessions, deleted sessions, and stale abandoned drafts
	for id, s := range summaries {
		// Filter out recently deleted sessions (tombstone protection against upstream sync delays)
		if IsDeletedCascade(id) {
			delete(summaries, id)
			continue
		}

		// Filter out internal subagent sessions completely while recording running subagents
		// and propagating their latest modification timestamp up to the visible ancestor.
		if isSubagentTrajectoryMap(s, id) {
			ancestor := resolveVisibleAncestor(id)
			if ancestor != "" {
				status, _ := s["status"].(string)
				if status == "CASCADE_RUN_STATUS_RUNNING" {
					runningSubagentParents[ancestor] = true
				}
				if modT, ok := rawModTimes[id]; ok && !modT.IsZero() {
					if prev, exists := subagentLatestModTime[ancestor]; !exists || modT.After(prev) {
						subagentLatestModTime[ancestor] = modT
					}
				}
			}
			delete(summaries, id)
			continue
		}

		// For mobile/PWA /api/ clients (excluding compact widgets), keep /btw side-question branches
		// in the list with explicit branch metadata so clients render them indented under their parent.
		// Desktop workbench (/exa.language_server_pb...) and widgets keep them filtered out.
		isSideQ := isSideQuestionTrajectoryMap(s, id)
		if isSideQ {
			if !isApiClient || isWidgetLite {
				delete(summaries, id)
				continue
			}
			s["isSideQuestion"] = true
			if pid := sideParentByID[id]; pid != "" {
				s["branchParentId"] = pid
			}
			if fi, ok := sideForkStepByID[id]; ok {
				s["forkStepIndex"] = fi
			}
			if sqTitle := sideTitleByID[id]; sqTitle != "" {
				cleanSQ := SanitizeTitle(sqTitle)
				if cleanSQ != "" {
					ann, _ := s["annotations"].(map[string]interface{})
					if ann == nil {
						ann = make(map[string]interface{})
					}
					ann["title"] = cleanSQ
					s["annotations"] = ann
					s["summary"] = cleanSQ
					setCachedCascadeTitle(id, cleanSQ)
				}
			}
			keptSideQuestionIDs = append(keptSideQuestionIDs, id)
		}

		// Backfill archived state from disk .pbtxt when upstream omits the field.
		if ann, ok := s["annotations"].(map[string]interface{}); !ok || ann == nil || ann["archived"] == nil {
			if diskArchived, found := readAnnotationArchived(id); found && diskArchived {
				if ann == nil {
					ann = make(map[string]interface{})
				}
				ann["archived"] = true
				s["annotations"] = ann
			}
		}

		// Backfill starred state from disk .pbtxt when upstream omits the field.
		if ann, ok := s["annotations"].(map[string]interface{}); !ok || ann == nil || ann["starred"] == nil {
			if diskStarred, found := readAnnotationStarred(id); found && diskStarred {
				if ann == nil {
					ann = make(map[string]interface{})
				}
				ann["starred"] = true
				s["annotations"] = ann
			}
		}

		// Enrich missing title (authoritative sources first: annotations -> disk .pbtxt -> upstream summary;
		// in-memory cascadeTitles is last-resort fallback so a prompt-derived fallback never shadows upstream summary).
		hasTitle := false
		if ann, ok := s["annotations"].(map[string]interface{}); ok {
			if t, ok := ann["title"].(string); ok && strings.TrimSpace(t) != "" && t != "未命名会话" {
				cleanTitle := SanitizeTitle(t)
				if cleanTitle != "" {
					ann["title"] = cleanTitle
					setCachedCascadeTitle(id, cleanTitle)
					hasTitle = true
				}
			}
		}
		if !hasTitle {
			if t := readAnnotationTitle(id); t != "" && t != "未命名会话" {
				cleanTitle := SanitizeTitle(t)
				if cleanTitle != "" {
					ann, _ := s["annotations"].(map[string]interface{})
					if ann == nil {
						ann = make(map[string]interface{})
					}
					ann["title"] = cleanTitle
					s["annotations"] = ann
					s["summary"] = cleanTitle
					setCachedCascadeTitle(id, cleanTitle)
					hasTitle = true
				}
			}
		}
		if !hasTitle {
			if sm, ok := s["summary"].(string); ok && strings.TrimSpace(sm) != "" && sm != "未命名会话" {
				cleanTitle := SanitizeTitle(sm)
				if cleanTitle != "" {
					ann, _ := s["annotations"].(map[string]interface{})
					if ann == nil {
						ann = make(map[string]interface{})
					}
					ann["title"] = cleanTitle
					s["annotations"] = ann
					s["summary"] = cleanTitle
					setCachedCascadeTitle(id, cleanTitle)
					hasTitle = true
				}
			}
		}
		if !hasTitle {
			defaultTrajCache.cascadeTitlesMu.RLock()
			cachedT := defaultTrajCache.cascadeTitles[id]
			defaultTrajCache.cascadeTitlesMu.RUnlock()
			if cachedT != "" && cachedT != "未命名会话" {
				cleanTitle := SanitizeTitle(cachedT)
				if cleanTitle != "" {
					ann, _ := s["annotations"].(map[string]interface{})
					if ann == nil {
						ann = make(map[string]interface{})
					}
					ann["title"] = cleanTitle
					s["annotations"] = ann
					s["summary"] = cleanTitle
					hasTitle = true
				}
			}
		}

		// Resolve projectId and display name (workspaceName).
		// Scheduled-task trajectories retain their own independent task folder name & projectId
		// (matching Antigravity desktop) and are tagged with isScheduledTask=true.
		// Regular conversations backfill missing / outside-of-project projectIds from workspaceUris.
		meta, _ := s["trajectoryMetadata"].(map[string]interface{})
		curPID, _ := meta["projectId"].(string)
		var sourceMeta map[string]interface{}
		if meta != nil {
			sourceMeta, _ = meta["sourceMetadata"].(map[string]interface{})
		}
		var wsURIs []string
		if meta != nil {
			if uris, ok := meta["workspaceUris"].([]interface{}); ok {
				for _, u := range uris {
					if us, ok := u.(string); ok && us != "" {
						wsURIs = append(wsURIs, us)
					}
				}
			}
		}
		if len(wsURIs) == 0 {
			if wss, ok := s["workspaces"].([]interface{}); ok {
				for _, w := range wss {
					if wm, ok := w.(map[string]interface{}); ok {
						if uri, ok := wm["workspaceFolderAbsoluteUri"].(string); ok && uri != "" {
							wsURIs = append(wsURIs, uri)
						}
					}
				}
			}
		}
		wsRes := resolveWorkspace(curPID, wsURIs, sourceMeta)
		if wsRes.IsScheduledTask {
			s["isScheduledTask"] = true
			if (curPID == "" || curPID == "outside-of-project") && wsRes.ProjectID != "" {
				if meta == nil {
					meta = make(map[string]interface{})
				}
				meta["projectId"] = wsRes.ProjectID
				s["trajectoryMetadata"] = meta
			}
			if wsRes.WorkspaceName != "" {
				s["workspaceName"] = wsRes.WorkspaceName
			}
		} else {
			resolvedPID, resolvedWSName := wsRes.ProjectID, wsRes.WorkspaceName
			if (curPID == "" || curPID == "outside-of-project" || sidecarProjectIDs[curPID]) && len(wsURIs) > 0 {
				if resolvedPID == "" {
					resolvedPID = lazyMatchProjectID(wsURIs)
					if resolvedPID != "" && resolvedWSName == "" {
						resolvedWSName = resolveWorkspace(resolvedPID, wsURIs, nil).WorkspaceName
					}
				}
				if resolvedPID != "" {
					if meta == nil {
						meta = make(map[string]interface{})
					}
					meta["projectId"] = resolvedPID
					s["trajectoryMetadata"] = meta
				}
			}
			if resolvedWSName != "" {
				s["workspaceName"] = resolvedWSName
			}
		}
		if len(wsURIs) > 0 {
			br, pr, repoRoot, needsWarmup := cachedWorkspacePullRequest(wsURIs, port)
			if br != "" {
				s["currentBranch"] = br
			}
			if pr != nil {
				s["pullRequest"] = pr
			}
			if needsWarmup && p.insp != nil {
				p.scheduleWorkspacePRWarmup(repoRoot, br, port, token)
			}
		}

		// Filter out stale empty drafts (0 steps, not running, older than 15 minutes, no custom title)
		status, _ := s["status"].(string)
		stepCount := 0
		if sc, ok := s["stepCount"].(float64); ok {
			stepCount = int(sc)
		} else if sc, ok := s["stepCount"].(int); ok {
			stepCount = sc
		}

		if stepCount == 0 && status != "CASCADE_RUN_STATUS_RUNNING" && !hasTitle {
			isRecent := false
			if modStr, ok := s["lastModifiedTime"].(string); ok {
				if t, err := parseTime(modStr); err == nil && time.Since(t) < 15*time.Minute {
					isRecent = true
				}
			}
			if !isRecent {
				delete(summaries, id)
			}
		}
	}

	// Inherit resolved parent title, projectId, and workspaceName onto kept side-question branches
	// so project tabs and indented branch rows always stay attached to their parent conversation.
	for _, sid := range keptSideQuestionIDs {
		s := summaries[sid]
		if s == nil {
			continue
		}
		pid, _ := s["branchParentId"].(string)
		if pid == "" {
			continue
		}
		parent := summaries[pid]
		if parent == nil {
			continue
		}
		if ann, ok := parent["annotations"].(map[string]interface{}); ok {
			if pt, ok := ann["title"].(string); ok && strings.TrimSpace(pt) != "" {
				s["branchParentTitle"] = pt
			}
		}
		if _, hasPT := s["branchParentTitle"]; !hasPT {
			if pt, ok := parent["summary"].(string); ok && strings.TrimSpace(pt) != "" {
				s["branchParentTitle"] = pt
			}
		}
		if pws, ok := parent["workspaceName"].(string); ok && pws != "" {
			if cur, _ := s["workspaceName"].(string); cur == "" {
				s["workspaceName"] = pws
			}
		}
		if pSched, ok := parent["isScheduledTask"].(bool); ok && pSched {
			if _, exists := s["isScheduledTask"]; !exists {
				s["isScheduledTask"] = true
			}
		}
		if pMeta, ok := parent["trajectoryMetadata"].(map[string]interface{}); ok && pMeta != nil {
			if ppid, ok := pMeta["projectId"].(string); ok && ppid != "" {
				sMeta, _ := s["trajectoryMetadata"].(map[string]interface{})
				if sMeta == nil {
					sMeta = make(map[string]interface{})
				}
				if curPID, _ := sMeta["projectId"].(string); curPID == "" || curPID == "outside-of-project" {
					sMeta["projectId"] = ppid
					s["trajectoryMetadata"] = sMeta
				}
			}
		}
	}

	// Propagate running status from subagents up to parent conversations
	for parentID := range runningSubagentParents {
		if parent, ok := summaries[parentID]; ok {
			parent["status"] = "CASCADE_RUN_STATUS_RUNNING"
			parent["hasRunningSubagents"] = true
		}
	}

	// Non-blocking auto-translation check for newly generated recent conversation titles.
	p.MaybeAutoTranslateSummaries(summaries)

	// 1. In-memory fast inspection: check sessions already cached in trajCache (instant, zero network cost)
	defaultTrajCache.trajCacheMu.RLock()
	type snapEntry struct {
		cid       string
		fetchedAt time.Time
		data      *upstreamTrajectoryResp
	}
	var snapshots []snapEntry
	for cid, entry := range defaultTrajCache.trajCache {
		if entry != nil && entry.data != nil {
			snapshots = append(snapshots, snapEntry{
				cid:       cid,
				fetchedAt: entry.fetchedAt,
				data:      entry.data,
			})
		}
	}
	defaultTrajCache.trajCacheMu.RUnlock()

	inspectedFresh := make(map[string]bool, len(snapshots))
	for _, sn := range snapshots {
		if s := summaries[sn.cid]; s != nil {
			if !isTrajectorySnapshotFreshForSummary(s, rawStatuses[sn.cid], sn.fetchedAt, sn.data) {
				ClearTrajectoryCache(sn.cid)
				continue
			}
			inspectedFresh[sn.cid] = true
			details := p.ParseTrajectoryDetailsCached(sn.data)
			if details.PendingInteraction != nil || details.CanProceed {
				s["needsInput"] = true
			}
			if details.HasError {
				s["hasError"] = true
				s["errorMessage"] = details.ErrorMessage
			}
			if len(details.RunningTasks) > 0 {
				s["runningTaskCount"] = len(details.RunningTasks)
				// hasRunningTasks drives the "running" badge on native conversation lists, so it only
				// counts turn-blocking tasks; standing daemons (IsDaemon=true) stay in runningTaskCount.
				if HasActiveNonDaemonTasks(details.RunningTasks) {
					s["hasRunningTasks"] = true
					s["status"] = "CASCADE_RUN_STATUS_RUNNING"
				}
			}
			if details.HasRunningSubagents {
				s["hasRunningSubagents"] = true
				s["status"] = "CASCADE_RUN_STATUS_RUNNING"
			}
			if details.BranchOrigin != nil {
				if details.BranchOrigin.IsSideQuestion {
					s["isSideQuestion"] = true
				}
				if details.BranchOrigin.ParentCascadeID != "" {
					if cur, _ := s["branchParentId"].(string); cur == "" {
						s["branchParentId"] = details.BranchOrigin.ParentCascadeID
					}
				}
				if details.BranchOrigin.ParentTitle != "" {
					if cur, _ := s["branchParentTitle"].(string); cur == "" {
						s["branchParentTitle"] = details.BranchOrigin.ParentTitle
					}
				}
				if details.BranchOrigin.ForkStepIndex >= 0 {
					s["forkStepIndex"] = details.BranchOrigin.ForkStepIndex
				}
			}
		}
	}

	// Propagate subagent modification timestamps and swarm coordinator state AFTER Step 1's
	// trajCache freshness check so parent snapshots are never unnecessarily evicted.
	for parentID, subMod := range subagentLatestModTime {
		if parent := summaries[parentID]; parent != nil {
			bumpSummaryLastModifiedTime(parent, subMod)
		}
	}
	for id, s := range summaries {
		if s == nil {
			continue
		}
		if sw := InspectSwarmCoordinatorForList(id, rawStatuses, rawModTimes); sw != nil && sw.HasSwarm {
			s["hasSwarm"] = true
			if sw.TotalMembers > 0 {
				s["swarmTotalMembers"] = sw.TotalMembers
			}
			if sw.WorkingCount > 0 {
				s["swarmWorkingCount"] = sw.WorkingCount
				s["status"] = "CASCADE_RUN_STATUS_RUNNING"
				s["hasRunningSubagents"] = true
			}
			if !sw.LatestActivityTime.IsZero() {
				bumpSummaryLastModifiedTime(s, sw.LatestActivityTime)
			}
		}
	}

	// 2. Candidates that might need live upstream probing:
	// Only probe for mobile/API clients (/api/...) that actively consume needsInput/hasError.
	// Desktop web workbench (/exa.language_server_pb...) does not use these flags.
	candidates := make(map[string]bool)

	if isApiClient {
		// Fast disk inspection: If implementation_plan.md.metadata.json has requestFeedback == true
		// and walkthrough.md does not yet exist, add non-running uncached sessions to candidates
		// so Step 3 verifies via detectProceed whether the plan is actually awaiting approval in the
		// latest turn (Antigravity never clears requestFeedback in .metadata.json after Proceed).
		if resolveAntigravityHomeDir() != "" {
			for cid, s := range summaries {
				if s["needsInput"] == true || inspectedFresh[cid] {
					continue
				}
				if status, _ := s["status"].(string); status == "CASCADE_RUN_STATUS_RUNNING" {
					continue
				}
				walkthroughFile := resolveAntigravitySubPath("brain", cid, "walkthrough.md")
				walkthroughStatKey := "stat:" + walkthroughFile
				defaultTrajCache.metadataCacheMu.RLock()
				statEntry, statCached := defaultTrajCache.metadataCache[walkthroughStatKey]
				defaultTrajCache.metadataCacheMu.RUnlock()
				walkthroughExists := false
				if statCached && time.Since(statEntry.fetchedAt) < metadataCacheTTL {
					walkthroughExists = statEntry.requestFeedback
				} else {
					_, serr := os.Stat(walkthroughFile)
					walkthroughExists = serr == nil
					defaultTrajCache.metadataCacheMu.Lock()
					defaultTrajCache.metadataCache[walkthroughStatKey] = &metadataCacheEntry{
						requestFeedback: walkthroughExists,
						fetchedAt:       time.Now(),
					}
					defaultTrajCache.metadataCacheMu.Unlock()
				}
				if walkthroughExists {
					continue
				}
				metaFile := resolveAntigravitySubPath("brain", cid, "implementation_plan.md.metadata.json")
				if readMetadataRequestFeedback(metaFile) {
					candidates[cid] = true
				}
			}
		}

		for id, s := range summaries {
			if s["needsInput"] == true || inspectedFresh[id] {
				continue
			}
			status, _ := s["status"].(string)
			if status == "CASCADE_RUN_STATUS_RUNNING" {
				candidates[id] = true
			}
		}

		// Also check top recent items if not already inspected from fresh trajCache
		type recentItem struct {
			id string
			t  time.Time
		}
		var recentItems []recentItem
		for id, s := range summaries {
			if s["needsInput"] == true || inspectedFresh[id] || candidates[id] {
				continue
			}
			if modStr, ok := s["lastModifiedTime"].(string); ok && modStr != "" {
				if t, err := parseTime(modStr); err == nil {
					recentItems = append(recentItems, recentItem{id: id, t: t})
				}
			}
		}
		if len(recentItems) > 0 {
			sort.Slice(recentItems, func(i, j int) bool {
				return recentItems[i].t.After(recentItems[j].t)
			})
			for i := 0; i < len(recentItems) && i < 3; i++ {
				candidates[recentItems[i].id] = true
			}
		}
	}

	if len(candidates) > 0 {
		var mu sync.Mutex
		actionMap := make(map[string]bool)
		errorMap := make(map[string]string)
		clearedErrorMap := make(map[string]string)
		sem := make(chan struct{}, 3)

		// Responsive 500ms timeout budget with full context propagation
		ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
		defer cancel()

		var wg sync.WaitGroup
		for cid := range candidates {
			wg.Add(1)
			go func(cascadeID string) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					return
				}
				raw, err := p.fetchUpstreamTrajectoryWithContext(ctx, cascadeID, port, token)
				if err == nil && raw != nil {
					details := p.ParseTrajectoryDetailsCached(raw)
					if details.PendingInteraction != nil || details.CanProceed {
						mu.Lock()
						actionMap[cascadeID] = true
						mu.Unlock()
					}
					if details.HasError {
						mu.Lock()
						errorMap[cascadeID] = details.ErrorMessage
						mu.Unlock()
					} else {
						mu.Lock()
						clearedErrorMap[cascadeID] = details.Status
						mu.Unlock()
					}
				}
			}(cid)
		}

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
		}

		for cid, hasAction := range actionMap {
			if hasAction && summaries[cid] != nil {
				summaries[cid]["needsInput"] = true
			}
		}
		for cid, errMsg := range errorMap {
			if summaries[cid] != nil {
				summaries[cid]["hasError"] = true
				summaries[cid]["status"] = "CASCADE_RUN_STATUS_ERROR"
				summaries[cid]["errorMessage"] = errMsg
			}
		}
		for cid, cleanStatus := range clearedErrorMap {
			if summaries[cid] != nil && summaries[cid]["status"] == "CASCADE_RUN_STATUS_ERROR" {
				summaries[cid]["hasError"] = false
				summaries[cid]["status"] = cleanStatus
				delete(summaries[cid], "errorMessage")
			}
		}
	}

	if r.Header.Get("X-Widget-Lite") == "1" {
		summaries = compactSummariesForWidget(summaries)
		rawMap = map[string]json.RawMessage{}
	}

	rawMap["trajectorySummaries"], _ = json.Marshal(summaries)
	respBytes, err := json.Marshal(rawMap)
	if err != nil {
		slog.Warn("[Proxy] GetAllCascadeTrajectories: failed to encode response", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(respBytes)))
	w.WriteHeader(http.StatusOK)
	w.Write(respBytes)
}

// compactSummariesForWidget strips bulky step/metadata payloads and archived sessions
// when serving iOS WidgetKit background fetches (30 MB Jetsam memory cap).
func compactSummariesForWidget(summaries map[string]map[string]interface{}) map[string]map[string]interface{} {
	lite := make(map[string]map[string]interface{}, len(summaries))
	keepKeys := [...]string{
		"summary",
		"stepCount",
		"lastModifiedTime",
		"status",
		"workspaceName",
		"isScheduledTask",
		"workspaces",
		"needsInput",
		"hasError",
		"hasRunningTasks",
		"runningTaskCount",
		"hasRunningSubagents",
		"hasSwarm",
		"swarmWorkingCount",
		"swarmTotalMembers",
		"notFullyIdle",
		"backgroundTaskCount",
	}
	for id, s := range summaries {
		if s == nil {
			continue
		}
		if ann, ok := s["annotations"].(map[string]interface{}); ok {
			if archived, _ := ann["archived"].(bool); archived {
				continue
			}
		}
		item := make(map[string]interface{}, 10)
		for _, k := range keepKeys {
			if v, exists := s[k]; exists {
				item[k] = v
			}
		}
		if ann, ok := s["annotations"].(map[string]interface{}); ok {
			liteAnn := make(map[string]interface{}, 3)
			for _, ak := range [...]string{"title", "markedAsUnread", "pinned"} {
				if av, exists := ann[ak]; exists {
					liteAnn[ak] = av
				}
			}
			if len(liteAnn) > 0 {
				item["annotations"] = liteAnn
			}
		}
		lite[id] = item
	}
	return lite
}

// isTrajectorySnapshotFreshForSummary reports whether a cached GetCascadeTrajectory snapshot
// is still consistent with the live summary entry from GetAllCascadeTrajectories.
func isTrajectorySnapshotFreshForSummary(s map[string]interface{}, rawSummaryStatus string, fetchedAt time.Time, data *upstreamTrajectoryResp) bool {
	if data == nil {
		return false
	}
	cachedStatus := data.Status
	if rawSummaryStatus != "" && cachedStatus != "" && rawSummaryStatus != cachedStatus {
		return false
	}
	if rawSummaryStatus == "CASCADE_RUN_STATUS_RUNNING" || cachedStatus == "CASCADE_RUN_STATUS_RUNNING" {
		if time.Since(fetchedAt) > 1500*time.Millisecond {
			return false
		}
	} else if time.Since(fetchedAt) >= 60*time.Second {
		return false
	}
	if modStr, ok := s["lastModifiedTime"].(string); ok && modStr != "" {
		if modTime, err := parseTime(modStr); err == nil && modTime.After(fetchedAt) {
			return false
		}
	}
	if summarySteps := summaryStepCount(s["stepCount"]); summarySteps > 0 && len(data.Trajectory.Steps) > 0 && summarySteps != len(data.Trajectory.Steps) {
		return false
	}
	return true
}

func summaryStepCount(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case int64:
		return int(n)
	default:
		return 0
	}
}

func extractSummaryParentID(s map[string]interface{}) string {
	if s == nil {
		return ""
	}
	if pid, ok := s["parentConversationId"].(string); ok && strings.TrimSpace(pid) != "" {
		return strings.TrimSpace(pid)
	}
	if meta, ok := s["trajectoryMetadata"].(map[string]interface{}); ok && meta != nil {
		if pid, ok := meta["parentConversationId"].(string); ok && strings.TrimSpace(pid) != "" {
			return strings.TrimSpace(pid)
		}
	}
	return ""
}

func extractSummaryRootID(s map[string]interface{}) string {
	if s == nil {
		return ""
	}
	if rid, ok := s["rootConversationId"].(string); ok && strings.TrimSpace(rid) != "" {
		return strings.TrimSpace(rid)
	}
	if meta, ok := s["trajectoryMetadata"].(map[string]interface{}); ok && meta != nil {
		if rid, ok := meta["rootConversationId"].(string); ok && strings.TrimSpace(rid) != "" {
			return strings.TrimSpace(rid)
		}
	}
	return ""
}

func bumpSummaryLastModifiedTime(s map[string]interface{}, candidate time.Time) {
	if s == nil || candidate.IsZero() {
		return
	}
	if curStr, ok := s["lastModifiedTime"].(string); ok && curStr != "" {
		if curT, err := parseTime(curStr); err == nil && !candidate.After(curT) {
			return
		}
	}
	s["lastModifiedTime"] = candidate.UTC().Format(time.RFC3339Nano)
}

// isSubagentTrajectoryMap checks if a trajectory summary map belongs to an internal subagent.
func isSubagentTrajectoryMap(s map[string]interface{}, id string) bool {
	meta, ok := s["trajectoryMetadata"].(map[string]interface{})
	if !ok || meta == nil {
		return false
	}

	if parent, ok := meta["parentConversationId"].(string); ok && strings.TrimSpace(parent) != "" {
		return true
	}
	if isFork, ok := meta["isBattleModeFork"].(bool); ok && isFork {
		return true
	}
	if spec, ok := meta["subagentSpec"]; ok && spec != nil {
		return true
	}
	if script, ok := meta["agentScript"]; ok && script != nil {
		return true
	}
	if depth, ok := meta["nestingDepth"].(float64); ok && depth > 0 {
		return true
	}
	if depth, ok := meta["nestingDepth"].(int); ok && depth > 0 {
		return true
	}

	return false
}

const deletedZombieGracePeriod = 10 * time.Second

// filterDeletedZombieSummaries removes in-memory zombie trajectories that upstream
// language_server failed to evict after a partial DeleteCascadeTrajectory (for example
// when desktop Antigravity deletes a conversation and hits "unlinkat brain/<id>: directory
// not empty" after already removing conversations/<id>.db and conversation_summaries.db).
//
// Safety invariants to prevent false positives in tests, non-default app_data_dir setups,
// or brand-new conversations:
//   - Only activates when ~/.gemini/antigravity/conversations exists and at least one
//     entry in summaries is backed by a .db/.pb file there (or the missing entry itself
//     still has a brain/<id> or annotations/<id>.pbtxt residue under ~/.gemini/antigravity).
//   - Never filters 0-step drafts, currently running sessions, or sessions modified within
//     deletedZombieGracePeriod.
var (
	diskConvIDsMu      sync.RWMutex
	diskConvIDsDir     string
	diskConvIDsModNano int64
	diskConvIDsAt      time.Time
	diskConvIDsMap     map[string]struct{}
)

func loadCachedDiskConvIDs(convDir string) (map[string]struct{}, bool) {
	dirFi, err := os.Stat(convDir)
	if err != nil || !dirFi.IsDir() {
		return nil, false
	}
	modNano := dirFi.ModTime().UnixNano()
	now := time.Now()

	diskConvIDsMu.RLock()
	if diskConvIDsMap != nil && diskConvIDsDir == convDir && diskConvIDsModNano == modNano && now.Sub(diskConvIDsAt) < 3*time.Second {
		m := diskConvIDsMap
		diskConvIDsMu.RUnlock()
		return m, true
	}
	diskConvIDsMu.RUnlock()

	entries, err := os.ReadDir(convDir)
	if err != nil {
		return nil, false
	}
	ids := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".db") {
			ids[strings.TrimSuffix(name, ".db")] = struct{}{}
		} else if strings.HasSuffix(name, ".pb") {
			ids[strings.TrimSuffix(name, ".pb")] = struct{}{}
		}
	}

	diskConvIDsMu.Lock()
	diskConvIDsDir = convDir
	diskConvIDsModNano = modNano
	diskConvIDsAt = now
	diskConvIDsMap = ids
	diskConvIDsMu.Unlock()
	return ids, true
}

func filterDeletedZombieSummaries(summaries map[string]map[string]interface{}) {
	if len(summaries) == 0 {
		return
	}
	convDir := resolveAntigravitySubPath("conversations")
	if convDir == "" {
		return
	}
	agDir := filepath.Dir(convDir)
	diskConvIDs, ok := loadCachedDiskConvIDs(convDir)
	if !ok {
		return
	}

	matchedOnDisk := false
	for id := range summaries {
		if _, ok := diskConvIDs[id]; ok {
			matchedOnDisk = true
			break
		}
	}

	now := time.Now()
	for id, s := range summaries {
		if _, ok := diskConvIDs[id]; ok {
			continue
		}
		if !cascadeIDRe.MatchString(id) || s == nil {
			continue
		}
		if summaryStepCount(s["stepCount"]) <= 0 {
			continue
		}
		if status, _ := s["status"].(string); status == "CASCADE_RUN_STATUS_RUNNING" {
			continue
		}

		tsStr, _ := s["lastModifiedTime"].(string)
		if tsStr == "" {
			tsStr, _ = s["createdTime"].(string)
		}
		if tsStr == "" {
			continue
		}
		ts, err := parseTime(tsStr)
		if err != nil || now.Sub(ts) < deletedZombieGracePeriod {
			continue
		}

		if !matchedOnDisk {
			hasResidue := false
			if _, serr := os.Stat(filepath.Join(agDir, "brain", id)); serr == nil {
				hasResidue = true
			} else if _, serr := os.Stat(filepath.Join(agDir, "annotations", id+".pbtxt")); serr == nil {
				hasResidue = true
			}
			if !hasResidue {
				continue
			}
		}

		delete(summaries, id)
	}
}

// isGrpcWebFramed checks if data matches gRPC-Web / Connect framed streaming format:
// A sequence of frames where each frame has [1 byte flag][4 bytes big-endian length][payload bytes].
func isGrpcWebFramed(data []byte) bool {
	if len(data) < 5 {
		return false
	}
	idx := 0
	for idx < len(data) {
		if idx+5 > len(data) {
			return false
		}
		flag := data[idx]
		// Valid gRPC-Web flags: 0x00 (data), 0x01 (compressed data), 0x80 (trailers)
		if flag != 0x00 && flag != 0x01 && flag != 0x80 {
			return false
		}
		msgLen := int(binary.BigEndian.Uint32(data[idx+1 : idx+5]))
		if idx+5+msgLen > len(data) {
			return false
		}
		idx += 5 + msgLen
	}
	return idx == len(data)
}

func replaceFileTypeEnums(body []byte) []byte {
	body = bytes.ReplaceAll(body, []byte(`"FILE_TYPE_DIRECTORY"`), []byte(`2`))
	body = bytes.ReplaceAll(body, []byte(`"FILE_TYPE_FILE"`), []byte(`1`))
	body = bytes.ReplaceAll(body, []byte(`"FILE_TYPE_SYMLINK"`), []byte(`3`))
	body = bytes.ReplaceAll(body, []byte(`"FILE_TYPE_UNSPECIFIED"`), []byte(`0`))
	return body
}

// normalizeFileTypes transforms proto enum string values ("FILE_TYPE_DIRECTORY", etc.)
// into numeric enum values (2, 1, 3, 0) expected by the ConnectRPC client in main.js.
// It supports both raw JSON and gRPC-Web / Connect enveloped streams, adjusting
// the 4-byte big-endian frame lengths to prevent "protocol error: incomplete envelope".
func normalizeFileTypes(body []byte) []byte {
	if !bytes.Contains(body, []byte("FILE_TYPE_")) {
		return body
	}
	if !isGrpcWebFramed(body) {
		return replaceFileTypeEnums(body)
	}

	var out bytes.Buffer
	idx := 0
	for idx < len(body) {
		flag := body[idx]
		msgLen := int(binary.BigEndian.Uint32(body[idx+1 : idx+5]))
		payload := body[idx+5 : idx+5+msgLen]

		if flag == 0x00 || flag == 0x01 {
			normPayload := replaceFileTypeEnums(payload)
			out.WriteByte(flag)
			var lenBuf [4]byte
			binary.BigEndian.PutUint32(lenBuf[:], uint32(len(normPayload)))
			out.Write(lenBuf[:])
			out.Write(normPayload)
		} else {
			out.WriteByte(flag)
			var lenBuf [4]byte
			binary.BigEndian.PutUint32(lenBuf[:], uint32(len(payload)))
			out.Write(lenBuf[:])
			out.Write(payload)
		}
		idx += 5 + msgLen
	}
	return out.Bytes()
}

// isReadOnlyRPC reports whether an RPC path is a Get*/List* query that cannot change the cascade list.
func isReadOnlyRPC(path string) bool {
	name := path[strings.LastIndex(path, "/")+1:]
	return strings.HasPrefix(name, "Get") || strings.HasPrefix(name, "List")
}

// handleSearchConversations handles SearchConversations RPC with instant FTS5 SQLite query.
func (p *Proxy) handleSearchConversations(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string) {
	const maxSearchReqBytes = 64 * 1024
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxSearchReqBytes+1))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(bodyBytes) > maxSearchReqBytes {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	var req struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	_ = json.Unmarshal(bodyBytes, &req)

	query := strings.TrimSpace(req.Query)
	if query != "" {
		// Fast Path: Try local FTS5 SQLite query first (<10ms)
		if results, err := SearchConversationsFTS5(r.Context(), query, req.Limit); err == nil && len(results) > 0 {
			enrichSearchResultsWithSideQuestions(results)
			respBytes, _ := json.Marshal(ConversationSearchResponse{Results: results})
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Gateway-Search", "FTS5")
			w.Header().Set("Content-Length", strconv.Itoa(len(respBytes)))
			w.WriteHeader(http.StatusOK)
			w.Write(respBytes)
			return
		}
	}

	// Fallback to upstream if available
	if rp != nil {
		fwdReq := r.Clone(r.Context())
		fwdReq.URL.Path = reqPath
		fwdReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		rec := newBufferedResponseWriter()
		defer rec.release()
		rp.ServeHTTP(rec, fwdReq)
		body := rec.body.Bytes()
		if rec.statusCode == http.StatusOK && len(body) > 2 {
			var parsed ConversationSearchResponse
			if json.Unmarshal(body, &parsed) == nil && len(parsed.Results) > 0 {
				enrichSearchResultsWithSideQuestions(parsed.Results)
				if patched, err := json.Marshal(parsed); err == nil {
					body = patched
				}
			}
		}
		for k, vals := range rec.header {
			if strings.EqualFold(k, "Content-Length") {
				continue
			}
			for _, v := range vals {
				w.Header().Add(k, v)
			}
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(rec.statusCode)
		_, _ = w.Write(body)
		return
	}

	// If upstream not available and FTS5 returned empty, return empty results
	respBytes, _ := json.Marshal(ConversationSearchResponse{Results: []ConversationSearchResult{}})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBytes)
}

// enrichSearchResultsWithSideQuestions attaches side-question parentage metadata so clients
// selecting a /btw search hit can open both the parent conversation and its side-question directly.
func enrichSearchResultsWithSideQuestions(results []ConversationSearchResult) {
	if len(results) == 0 {
		return
	}
	btwSnap := defaultBtwTracker.snapshotAll()
	for i := range results {
		cid := strings.TrimSpace(results[i].CascadeID)
		if cid == "" {
			continue
		}
		if entry, ok := btwSnap[cid]; ok {
			results[i].IsSideQuestion = true
			if entry.ParentCascadeID != "" {
				results[i].ParentConversationID = entry.ParentCascadeID
			}
			if results[i].Title == "" {
				results[i].Title = strings.TrimSpace(firstNonEmpty(entry.Title, entry.Question))
			}
			continue
		}
		hierarchyMu.RLock()
		hm, hasHierarchy := hierarchyCache[cid]
		hierarchyMu.RUnlock()
		if hasHierarchy && hm.parentID != "" {
			if isSideQuestionTrajectoryMap(map[string]interface{}{}, cid) {
				results[i].IsSideQuestion = true
				results[i].ParentConversationID = hm.parentID
			}
		}
	}
}
