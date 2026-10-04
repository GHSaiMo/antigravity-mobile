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
	p.mu.RLock()
	rp := p.activeProxy
	port := p.activePort
	token := p.activeToken
	p.mu.RUnlock()

	if rp == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		if err := json.NewEncoder(w).Encode(map[string]string{
			"code":    "unavailable",
			"message": "Antigravity language_server is not connected",
		}); err != nil {
			slog.Warn("[Proxy] handleRpcProxy: failed to encode error response", "err", err)
		}
		return
	}

	reqPath := strings.TrimPrefix(r.URL.Path, "/api")
	if verboseRPC {
		slog.Info(fmt.Sprintf("[Proxy] RPC: %s %s", r.Method, reqPath))
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

	var req struct {
		CascadeID string `json:"cascadeId"`
	}
	if len(bodyBytes) > 0 {
		_ = json.Unmarshal(bodyBytes, &req)
	}

	cascadeID := strings.TrimSpace(req.CascadeID)
	if cascadeID == "" || port == 0 {
		// Missing cascadeId or no upstream — fall through to raw proxy
		p.mu.RLock()
		rp := p.activeProxy
		p.mu.RUnlock()
		if rp == nil {
			writeJSONError(w, "Antigravity language_server is not connected", http.StatusServiceUnavailable)
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
	rawResp, fetchErr := p.fetchUpstreamTrajectoryWithMaxAge(cascadeID, port, token, maxAge)
	if fetchErr != nil {
		// Forward raw to upstream on cache miss / error
		p.mu.RLock()
		rp := p.activeProxy
		p.mu.RUnlock()
		if rp == nil {
			writeJSONError(w, "Antigravity language_server is not connected", http.StatusServiceUnavailable)
			return
		}
		fwdReq := r.Clone(r.Context())
		fwdReq.URL.Path = "/exa.language_server_pb.LanguageServerService/GetCascadeTrajectory"
		fwdReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		fwdReq.ContentLength = int64(len(bodyBytes))
		rp.ServeHTTP(w, fwdReq)
		return
	}

	// Re-encode as JSON for the client (same wire format as upstream Connect-RPC response)
	respBytes, encErr := json.Marshal(rawResp)
	if encErr != nil {
		writeJSONError(w, "Failed to encode trajectory response", http.StatusInternalServerError)
		return
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

	var payload struct {
		CascadeID      string `json:"cascadeId"`
		ConversationID string `json:"conversationId"`
	}
	_ = json.Unmarshal(bodyBytes, &payload)
	targetID := strings.TrimSpace(payload.CascadeID)
	if targetID == "" {
		targetID = strings.TrimSpace(payload.ConversationID)
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

	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", port)
	bodyBytes, cleanup, _ := readBodyToPool(r.Body, 5*1024*1024)
	defer cleanup()
	if len(bodyBytes) == 0 {
		bodyBytes = []byte("{}")
	}

	// P4: reuse p.mediumClient; enforce the 4s budget via a context deadline
	// instead of allocating a new http.Client struct on every request.
	listCtx, listCancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer listCancel()
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
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		return
	}

	var rawMap map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&rawMap); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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

	// Enrich missing titles, filter out subagent sessions, deleted sessions, and stale abandoned drafts
	for id, s := range summaries {
		// Filter out recently deleted sessions (tombstone protection against upstream sync delays)
		if IsDeletedCascade(id) {
			delete(summaries, id)
			continue
		}

		// Filter out internal subagent sessions completely
		if isSubagentTrajectoryMap(s, id) {
			delete(summaries, id)
			continue
		}

		// Enrich missing title
		hasTitle := false
		if ann, ok := s["annotations"].(map[string]interface{}); ok {
			if t, ok := ann["title"].(string); ok && strings.TrimSpace(t) != "" && t != "未命名会话" {
				cleanTitle := SanitizeTitle(t)
				if cleanTitle != "" {
					ann["title"] = cleanTitle
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
					hasTitle = true
				}
			}
		}

		// Backfill projectId if missing or outside-of-project
		meta, _ := s["trajectoryMetadata"].(map[string]interface{})
		curPID, _ := meta["projectId"].(string)
		if curPID == "" || curPID == "outside-of-project" {
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
			if len(wsURIs) > 0 {
				if matchedPID := p.FindProjectIDForWorkspaces(wsURIs); matchedPID != "" {
					if meta == nil {
						meta = make(map[string]interface{})
					}
					meta["projectId"] = matchedPID
					s["trajectoryMetadata"] = meta
				}
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

	// 1. In-memory fast inspection: check sessions already cached in trajCache (instant, zero network cost)
	defaultTrajCache.trajCacheMu.RLock()
	type snapEntry struct {
		cid  string
		data *upstreamTrajectoryResp
	}
	var snapshots []snapEntry
	for cid, entry := range defaultTrajCache.trajCache {
		if entry != nil && entry.data != nil {
			snapshots = append(snapshots, snapEntry{cid: cid, data: entry.data})
		}
	}
	defaultTrajCache.trajCacheMu.RUnlock()

	for _, sn := range snapshots {
		if s := summaries[sn.cid]; s != nil {
			details := p.ParseTrajectoryDetails(sn.data)
			if details.PendingInteraction != nil || details.CanProceed {
				s["needsInput"] = true
			}
			if details.HasError {
				s["hasError"] = true
				s["errorMessage"] = details.ErrorMessage
			}
		}
	}

	// 2. Fast disk inspection: If implementation_plan.md.metadata.json has requestFeedback == true
	// and walkthrough.md does not yet exist (plan not yet delivered).
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		for cid, s := range summaries {
			if s["needsInput"] == true {
				continue
			}
			walkthroughFile := filepath.Join(home, ".gemini/antigravity/brain", cid, "walkthrough.md")
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
			metaFile := filepath.Join(home, ".gemini/antigravity/brain", cid, "implementation_plan.md.metadata.json")
			if readMetadataRequestFeedback(metaFile) {
				s["needsInput"] = true
			}
		}
	}

	// 3. Candidates that might need live upstream probing:
	// Only probe for mobile/API clients (/api/...) that actively consume needsInput/hasError.
	// Desktop web workbench (/exa.language_server_pb...) does not use these flags.
	isApiClient := strings.HasPrefix(r.URL.Path, "/api/")
	candidates := make(map[string]bool)

	if isApiClient {
		for id, s := range summaries {
			if s["needsInput"] == true {
				continue
			}
			status, _ := s["status"].(string)
			if status == "CASCADE_RUN_STATUS_RUNNING" {
				candidates[id] = true
			}
		}

		// Also check top recent items if not already cached in trajCache
		type recentItem struct {
			id string
			t  time.Time
		}
		var recentItems []recentItem
		for id, s := range summaries {
			if s["needsInput"] == true || candidates[id] {
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
				cid := recentItems[i].id
				defaultTrajCache.trajCacheMu.RLock()
				cached, cachedOk := defaultTrajCache.trajCache[cid]
				var isCachedNonRunning bool
				if cachedOk && cached != nil && cached.data != nil {
					if cached.data.Status != "" && cached.data.Status != "CASCADE_RUN_STATUS_RUNNING" && time.Since(cached.fetchedAt) < 60*time.Second {
						isCachedNonRunning = true
					}
				}
				defaultTrajCache.trajCacheMu.RUnlock()
				if !isCachedNonRunning {
					candidates[cid] = true
				}
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
					details := p.ParseTrajectoryDetails(raw)
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
