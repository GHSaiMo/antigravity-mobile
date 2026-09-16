package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/inspector"
	"antigravity-mobile/internal/localtls"
)

// GatewayStatus represents the public status of the gateway.
type GatewayStatus struct {
	Status                string                  `json:"status"`
	Upstream              *inspector.InstanceInfo `json:"upstream,omitempty"`
	ActiveStreamCascadeID string                  `json:"active_stream_cascade_id,omitempty"`
	ActiveStreamTitle     string                  `json:"active_stream_title,omitempty"`
	Timestamp             time.Time               `json:"timestamp"`
	UnifiedCursor         *UnifiedCursor          `json:"unified_cursor,omitempty"`
}

// NotificationSink receives real-time trajectory status updates.
type NotificationSink interface {
	OnTrajectoryUpdate(details *TrajectoryDetails)
}

// Proxy routes and proxies HTTP/RPC requests to the Antigravity language_server.
type Proxy struct {
	insp      inspector.UpstreamDiscoverer
	transport *http.Transport
	startTime time.Time

	// Pre-built HTTP clients with different timeout tiers to avoid repeated construction (H-6)
	shortClient  *http.Client // 2-5s timeout, for quick status checks
	mediumClient *http.Client // 10s timeout, for standard RPC calls
	longClient   *http.Client // 60s timeout, for large data transfers

	mu          sync.RWMutex
	activeProxy *httputil.ReverseProxy
	activePort  int
	activeToken string
	notifier    NotificationSink

	activeStreamMu        sync.RWMutex
	activeStreamCascadeID string
	activeStreamTitle     string

	streamListenersMu sync.Mutex
	streamListeners   map[string][]chan struct{}

	msgDedupMu     sync.Mutex
	msgDedup       map[string]time.Time
	cascadeDedupMu sync.Mutex
	cascadeDedup   map[string]cascadeDedupEntry

	cursorMu                  sync.RWMutex
	mobileCascadeID           string
	mobileTitle               string
	mobileFocusedAt           time.Time
	desktopCascadeID          string
	desktopTitle              string
	desktopFocusedAt          time.Time
	suppressDesktopFocusUntil time.Time
	mobileStickyDuration      time.Duration
}

type cascadeDedupEntry struct {
	cascadeID string
	createdAt time.Time
}

// cascadeIDRe is a strict allowlist for cascade IDs used in filesystem operations.
// Cascade IDs are UUID-like strings: alphanumeric, hyphens, and underscores only.
var (
	cascadeIDRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)
	verboseRPC  = os.Getenv("GATEWAY_VERBOSE_RPC") != "" || os.Getenv("GATEWAY_LOG_RPC") != ""
)

// shortCascadeID truncates a cascade ID for log redaction/sanitization.
func shortCascadeID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 12 {
		return id
	}
	return id[:8] + "..."
}

// SetNotificationSink registers a sink to receive real-time trajectory updates.
func (p *Proxy) SetNotificationSink(sink NotificationSink) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.notifier = sink
}

// NotificationSink returns the registered notification sink, if any.
func (p *Proxy) NotificationSink() NotificationSink {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.notifier
}

// ActiveUpstream returns the current active upstream port and CSRF token.
func (p *Proxy) ActiveUpstream() (int, string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.activePort, p.activeToken
}

// SetTestUpstream configures active port and token for testing.
func (p *Proxy) SetTestUpstream(port int, token string) {
	p.updateUpstream(inspector.InstanceInfo{
		Port:      port,
		CSRFToken: token,
		IsHealthy: true,
	})
}

// GetActiveUserStatus queries the running Antigravity instance for the currently logged-in user email and name.
func (p *Proxy) GetActiveUserStatus() (email string, name string, err error) {
	port, token := p.ActiveUpstream()
	if port == 0 {
		return "", "", fmt.Errorf("no active Antigravity upstream")
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetUserStatus", port)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		return "", "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.shortClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("GetUserStatus returned status %d", resp.StatusCode)
	}

	var data struct {
		UserStatus struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"userStatus"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", err
	}

	return strings.TrimSpace(data.UserStatus.Email), strings.TrimSpace(data.UserStatus.Name), nil
}

// NewProxy creates a new reverse proxy backed by the inspector.
func NewProxy(insp inspector.UpstreamDiscoverer) *Proxy {
	tr := &http.Transport{
		TLSClientConfig:       localtls.ClientConfig(),
		DialTLSContext:        localtls.DialTLSContext,
		DisableCompression:    true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   50,
		MaxConnsPerHost:       100,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	p := &Proxy{
		insp:         insp,
		transport:    tr,
		startTime:    time.Now(),
		msgDedup:     make(map[string]time.Time),
		cascadeDedup: make(map[string]cascadeDedupEntry),
		shortClient: &http.Client{
			Timeout:   2 * time.Second,
			Transport: tr,
		},
		mediumClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: tr,
		},
		longClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: tr,
		},
		mobileStickyDuration: DefaultMobileStickyDuration,
	}

	insp.OnUpdate(func(info inspector.InstanceInfo) {
		p.updateUpstream(info)
	})

	if cur := insp.Current(); cur != nil && cur.IsHealthy {
		p.updateUpstream(*cur)
	}

	return p
}

func (p *Proxy) updateUpstream(info inspector.InstanceInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()

	targetURL, _ := url.Parse(fmt.Sprintf("https://127.0.0.1:%d", info.Port))
	rp := httputil.NewSingleHostReverseProxy(targetURL)
	rp.Transport = p.transport

	originalDirector := rp.Director
	token := info.CSRFToken
	port := info.Port

	rp.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = fmt.Sprintf("127.0.0.1:%d", port)
		req.Header.Set("x-codeium-csrf-token", token)
		if req.Header.Get("Connect-Protocol-Version") == "" {
			req.Header.Set("Connect-Protocol-Version", "1")
		}
		// Ask upstream not to compress (best-effort; HTTP/2 may ignore this)
		req.Header.Del("Accept-Encoding")
	}

	// Decompress gzip responses from upstream so browsers can parse JSON directly.
	// The Antigravity language_server (HTTP/2) may compress regardless of Accept-Encoding.
	rp.ModifyResponse = func(resp *http.Response) error {
		if resp.Header.Get("Content-Encoding") == "gzip" {
			gzReader, err := gzip.NewReader(resp.Body)
			if err != nil {
				return err
			}
			resp.Body = gzReader
			resp.Header.Del("Content-Encoding")
			resp.Header.Del("Content-Length") // length is now unknown
			resp.ContentLength = -1
		}
		return nil
	}

	p.activeProxy = rp
	p.activePort = port
	p.activeToken = token
	log.Printf("[Proxy] Updated upstream proxy to 127.0.0.1:%d", port)

	// Reset historical sync state and asynchronously sync historical trajectories
	ResetHistoricalSyncState()
	go func(prt int, tok string) {
		_ = p.SyncHistoricalTrajectories(prt, tok)
	}(port, token)
}

// ServeHTTP handles incoming HTTP requests.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Gateway meta APIs
	if r.URL.Path == "/gateway/status" {
		p.handleStatus(w, r)
		return
	}
	if r.URL.Path == "/gateway/rescan" {
		p.handleRescan(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/messages" {
		p.handleCascadeMessages(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/touch" || r.URL.Path == "/gateway/cascade/invalidate" {
		p.handleCascadeTouch(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/focus" {
		p.handleFocusSession(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/stream" {
		p.HandleCascadeStream(w, r)
		return
	}
	if r.URL.Path == "/gateway/projects" {
		p.HandleProjects(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/new" {
		p.HandleCreateCascade(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/interaction" {
		p.HandleCascadeInteraction(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/task/stop" {
		p.handleCascadeTaskStop(w, r)
		return
	}

	// WebSocket upgrade route
	if r.URL.Path == "/connect-websocket" {
		p.HandleWebSocket(w, r)
		return
	}

	// ConnectRPC proxy route (prefixed with /api/)
	if strings.HasPrefix(r.URL.Path, "/api/") {
		p.handleRpcProxy(w, r)
		return
	}

	// Artifacts proxy route
	if strings.HasPrefix(r.URL.Path, "/static/artifacts/") {
		p.handleArtifactProxy(w, r)
		return
	}

	http.NotFound(w, r)
}

// SetActiveStream records the currently connected active cascade stream on mobile.
func (p *Proxy) SetActiveStream(cascadeID, title string) {
	p.activeStreamMu.Lock()
	defer p.activeStreamMu.Unlock()
	p.activeStreamCascadeID = cascadeID
	p.activeStreamTitle = title
}

// ClearActiveStream clears the active cascade stream if matching the disconnecting cascade.
func (p *Proxy) ClearActiveStream(cascadeID string) {
	p.activeStreamMu.Lock()
	defer p.activeStreamMu.Unlock()
	if p.activeStreamCascadeID == cascadeID {
		p.activeStreamCascadeID = ""
		p.activeStreamTitle = ""
	}
}

// ActiveStream returns the currently active cascade ID and title, if any.
func (p *Proxy) ActiveStream() (string, string) {
	p.activeStreamMu.RLock()
	defer p.activeStreamMu.RUnlock()
	return p.activeStreamCascadeID, p.activeStreamTitle
}

func (p *Proxy) registerStreamTouchListener(cascadeID string) (<-chan struct{}, func()) {
	p.streamListenersMu.Lock()
	defer p.streamListenersMu.Unlock()
	if p.streamListeners == nil {
		p.streamListeners = make(map[string][]chan struct{})
	}
	ch := make(chan struct{}, 1)
	p.streamListeners[cascadeID] = append(p.streamListeners[cascadeID], ch)
	cleanup := func() {
		p.streamListenersMu.Lock()
		defer p.streamListenersMu.Unlock()
		listeners := p.streamListeners[cascadeID]
		for i, l := range listeners {
			if l == ch {
				p.streamListeners[cascadeID] = append(listeners[:i], listeners[i+1:]...)
				break
			}
		}
		if len(p.streamListeners[cascadeID]) == 0 {
			delete(p.streamListeners, cascadeID)
		}
	}
	return ch, cleanup
}

func (p *Proxy) notifyStreamTouch(cascadeID string) {
	p.streamListenersMu.Lock()
	defer p.streamListenersMu.Unlock()
	if p.streamListeners == nil {
		return
	}
	for _, ch := range p.streamListeners[cascadeID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func publicInstanceInfo(info *inspector.InstanceInfo) *inspector.InstanceInfo {
	if info == nil {
		return nil
	}
	cp := *info
	cp.CSRFToken = ""
	return &cp
}

func (p *Proxy) handleStatus(w http.ResponseWriter, r *http.Request) {
	cur := p.insp.Current()
	status := "disconnected"
	if cur != nil && cur.IsHealthy {
		status = "connected"
	}

	activeID, activeTitle := p.ActiveStream()

	data, err := json.Marshal(GatewayStatus{
		Status:                status,
		Upstream:              publicInstanceInfo(cur),
		ActiveStreamCascadeID: activeID,
		ActiveStreamTitle:     activeTitle,
		Timestamp:             time.Now(),
		UnifiedCursor:         p.ArbitrateCursor(),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (p *Proxy) handleCascadeTouch(w http.ResponseWriter, r *http.Request) {
	cascadeID := strings.TrimSpace(r.URL.Query().Get("cascadeId"))
	if cascadeID == "" && r.Body != nil {
		var body struct {
			CascadeID string `json:"cascadeId"`
		}
		// SEC-4: Limit body to prevent OOM (cascadeId is always short).
		_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)
		cascadeID = strings.TrimSpace(body.CascadeID)
	}
	if cascadeID != "" {
		ClearTrajectoryCache(cascadeID)
		ClearPendingMessagesCache(cascadeID)
		p.notifyStreamTouch(cascadeID)
		log.Printf("[Proxy] Cascade cache invalidated and stream notified via touch API: %s", shortCascadeID(cascadeID))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (p *Proxy) handleRescan(w http.ResponseWriter, r *http.Request) {
	info := p.insp.Scan()
	status := "failed"
	if info != nil && info.IsHealthy {
		status = "connected"
		ResetHistoricalSyncState()
		go func(prt int, tok string) {
			_ = p.SyncHistoricalTrajectories(prt, tok)
		}(info.Port, info.CSRFToken)
	}

	data, err := json.Marshal(GatewayStatus{
		Status:    status,
		Upstream:  publicInstanceInfo(info),
		Timestamp: time.Now(),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (p *Proxy) handleRpcProxy(w http.ResponseWriter, r *http.Request) {
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
			log.Printf("[Proxy] handleRpcProxy: failed to encode error response: %v", err)
		}
		return
	}

	reqPath := strings.TrimPrefix(r.URL.Path, "/api")
	if verboseRPC || (r.Method != http.MethodGet && (strings.HasSuffix(reqPath, "/SendUserCascadeMessage") ||
		strings.HasSuffix(reqPath, "/StartCascade") ||
		strings.HasSuffix(reqPath, "/DeleteCascadeTrajectory") ||
		strings.HasSuffix(reqPath, "/DeleteAgentMessage") ||
		strings.HasSuffix(reqPath, "/UpdateConversationAnnotations"))) {
		log.Printf("[Proxy] RPC: %s %s", r.Method, reqPath)
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
	if strings.HasSuffix(reqPath, "/UpdateConversationAnnotations") && r.Method == http.MethodPost {
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

	// Clone the request to avoid mutating the original before forwarding
	fwdReq := r.Clone(r.Context())
	fwdReq.URL.Path = reqPath
	rp.ServeHTTP(w, fwdReq)
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

func (p *Proxy) checkAndRecordMessageDedup(key string, ttl time.Duration) bool {
	p.msgDedupMu.Lock()
	defer p.msgDedupMu.Unlock()
	if p.msgDedup == nil {
		p.msgDedup = make(map[string]time.Time)
	}
	now := time.Now()
	if lastTime, exists := p.msgDedup[key]; exists {
		if now.Sub(lastTime) < ttl {
			return true // is duplicate within TTL
		}
		// Lazy expiry: remove stale key immediately
		delete(p.msgDedup, key)
	}
	// Periodic cleanup of stale entries if map expands
	if len(p.msgDedup) > 128 {
		for k, t := range p.msgDedup {
			if now.Sub(t) > 120*time.Second {
				delete(p.msgDedup, k)
			}
		}
	}
	p.msgDedup[key] = now
	return false
}

func (p *Proxy) getCascadeDedup(key string, ttl time.Duration) string {
	p.cascadeDedupMu.Lock()
	defer p.cascadeDedupMu.Unlock()
	if p.cascadeDedup == nil {
		p.cascadeDedup = make(map[string]cascadeDedupEntry)
		return ""
	}
	now := time.Now()
	if entry, exists := p.cascadeDedup[key]; exists {
		if now.Sub(entry.createdAt) < ttl {
			return entry.cascadeID
		}
		// Lazy expiry
		delete(p.cascadeDedup, key)
	}
	if len(p.cascadeDedup) > 64 {
		for k, v := range p.cascadeDedup {
			if now.Sub(v.createdAt) > 120*time.Second {
				delete(p.cascadeDedup, k)
			}
		}
	}
	return ""
}

func (p *Proxy) setCascadeDedup(key string, cascadeID string) {
	p.cascadeDedupMu.Lock()
	defer p.cascadeDedupMu.Unlock()
	if p.cascadeDedup == nil {
		p.cascadeDedup = make(map[string]cascadeDedupEntry)
	}
	p.cascadeDedup[key] = cascadeDedupEntry{
		cascadeID: cascadeID,
		createdAt: time.Now(),
	}
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

func (p *Proxy) handleSendUserCascadeMessage(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string, port int, token string) {
	clientMsgID := strings.TrimSpace(r.Header.Get("X-Client-Message-Id"))
	if clientMsgID == "" {
		clientMsgID = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	if clientMsgID != "" {
		dedupKey := "client_msg:" + clientMsgID
		if p.checkAndRecordMessageDedup(dedupKey, 60*time.Second) {
			log.Printf("[Proxy] Deduplicated repeat SendUserCascadeMessage via clientMsgID: %s", clientMsgID)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", "2")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{}"))
			return
		}
	}

	// P6: fast-reject oversized requests using Content-Length before reading any bytes.
	const maxBodySize = 50 * 1024 * 1024
	if cl := r.ContentLength; cl > maxBodySize {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Protect against OOM for extremely large requests by limiting to 50MB
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var rawMap map[string]interface{}
	cascadeID := ""
	if err := json.Unmarshal(bodyBytes, &rawMap); err == nil {
		cascadeID, _ = rawMap["cascadeId"].(string)

		// Short-window idempotency check: prevent duplicate triggers within 15 seconds
		var textContent strings.Builder
		if items, ok := rawMap["items"].([]interface{}); ok {
			for _, it := range items {
				if itemMap, ok := it.(map[string]interface{}); ok {
					if t, ok := itemMap["text"].(string); ok {
						textContent.WriteString(t)
					}
				}
			}
		}
		// If items is missing or empty, synthesize from "text" parameter
		if items, ok := rawMap["items"].([]interface{}); !ok || len(items) == 0 {
			if txt, ok := rawMap["text"].(string); ok && strings.TrimSpace(txt) != "" {
				rawMap["items"] = []interface{}{map[string]interface{}{"text": txt}}
				textContent.WriteString(txt)
			}
		}
		if comments, ok := rawMap["artifactComments"].([]interface{}); ok {
			for _, ac := range comments {
				if acMap, ok := ac.(map[string]interface{}); ok {
					if uri, ok := acMap["artifactUri"].(string); ok {
						textContent.WriteString(uri)
					}
				}
			}
		}
		if images, ok := rawMap["images"].([]interface{}); ok {
			for _, img := range images {
				if imgMap, ok := img.(map[string]interface{}); ok {
					if b64, ok := imgMap["base64Data"].(string); ok && len(b64) > 0 {
						prefix := b64
						if len(prefix) > 1024 {
							prefix = prefix[:1024]
						}
						h := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", len(b64), prefix)))
						textContent.WriteString(fmt.Sprintf(":img:%x", h[:8]))
					}
				}
			}
		}

		if clientMsgID == "" && cascadeID != "" && textContent.Len() > 0 {
			strategyKey := fmt.Sprintf("%v", rawMap["deliveryStrategy"])
			dedupKey := fmt.Sprintf("%s:%s:%x", cascadeID, strategyKey, sha256.Sum256([]byte(textContent.String())))
			if p.checkAndRecordMessageDedup(dedupKey, 15*time.Second) {
				log.Printf("[Proxy] Deduplicated repeat SendUserCascadeMessage for cascade %s (textLen=%d, hashDedup)", shortCascadeID(cascadeID), textContent.Len())
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Content-Length", "2")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("{}"))
				return
			}
		}

		targetModel := strings.TrimSpace(r.Header.Get("X-Antigravity-Model"))
		if targetModel == "" {
			targetModel = strings.TrimSpace(r.Header.Get("X-Model"))
		}
		if targetModel == "" {
			if m, ok := rawMap["model"].(string); ok {
				targetModel = strings.TrimSpace(m)
			}
		}

		// PERF-1: detect upfront whether the body actually needs modification.
		// If no model override and no cascadeConfigRaw cleanup needed, skip
		// unmarshal→modify→remarshal which is expensive for large payloads.
		_, hasModel := rawMap["model"]
		_, hasCfgRaw := rawMap["cascadeConfigRaw"]
		_, hasText := rawMap["text"]
		needsModification := targetModel != "" || hasModel || hasCfgRaw || hasText

		if !needsModification {
			// No changes required — forward original bytes as-is.
			if cascadeID != "" {
				ClearTrajectoryCache(cascadeID)
				ClearPendingMessagesCache(cascadeID)
			}
		} else {
			delete(rawMap, "model")

			var configToUse json.RawMessage
			// 1. Check if cascadeConfig already exists in payload
			if cfg, exists := rawMap["cascadeConfig"]; exists && cfg != nil {
				if cfgBytes, err := json.Marshal(cfg); err == nil && len(cfgBytes) > 2 && string(cfgBytes) != "{}" && string(cfgBytes) != "null" {
					configToUse = cfgBytes
				}
			}

			// 2. Check if cascadeConfigRaw was provided
			if len(configToUse) == 0 {
				if rawStr, ok := rawMap["cascadeConfigRaw"].(string); ok && len(rawStr) > 0 {
					configToUse = json.RawMessage(rawStr)
				}
			}

			// 3. Fallback to trajectory metadata or last known cascade config
			if len(configToUse) == 0 {
				configToUse = p.GetCascadeConfig(cascadeID, port, token)
			}

			modelEnum := resolveModelEnum(targetModel)
			canonicalName := canonicalModelName(targetModel)
			if canonicalName == "" {
				canonicalName = targetModel
			}

			if len(configToUse) > 0 {
				var cfgObj interface{}
				if err := json.Unmarshal(configToUse, &cfgObj); err == nil {
					if modelEnum != "" {
						cfgObj = applyModelToCascadeConfig(cfgObj, modelEnum, canonicalName)
						if updatedBytes, err := json.Marshal(cfgObj); err == nil {
							configToUse = updatedBytes
						}
						log.Printf("[Proxy] SendUserCascadeMessage: applied model %s (%s) to cascade %s", targetModel, modelEnum, shortCascadeID(cascadeID))
					}
					rawMap["cascadeConfig"] = cfgObj
				}
				SetLastKnownCascadeConfig(configToUse)
				if canonicalName != "" {
					SetCascadeModel(cascadeID, canonicalName, configToUse)
				}
			} else if modelEnum != "" {
				cfgObj := applyModelToCascadeConfig(nil, modelEnum, canonicalName)
				rawMap["cascadeConfig"] = cfgObj
				if updatedBytes, err := json.Marshal(cfgObj); err == nil {
					SetLastKnownCascadeConfig(updatedBytes)
					SetCascadeModel(cascadeID, canonicalName, updatedBytes)
				}
				log.Printf("[Proxy] SendUserCascadeMessage: synthesized cascadeConfig with model %s (%s) for cascade %s", targetModel, modelEnum, shortCascadeID(cascadeID))
			}

			delete(rawMap, "cascadeConfigRaw")

			if modifiedBytes, err := json.Marshal(rawMap); err == nil {
				bodyBytes = modifiedBytes
			}

			if cascadeID != "" {
				ClearTrajectoryCache(cascadeID)
				ClearPendingMessagesCache(cascadeID)
			}
		}
	}

	log.Printf("[Proxy] SendUserCascadeMessage: cascadeId=%s payloadLen=%d", shortCascadeID(cascadeID), len(bodyBytes))

	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}
	r.ContentLength = int64(len(bodyBytes))
	r.Header.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
	r.URL.Path = reqPath

	rw := newBufferedResponseWriter()
	defer rw.release()
	rp.ServeHTTP(rw, r)

	// Copy headers from upstream
	for k, vv := range rw.header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	respBody := rw.body.Bytes()
	if rw.statusCode >= 200 && rw.statusCode < 300 {
		// Ensure ConnectRPC empty responses always return valid JSON "{}"
		// to prevent any client JSONDecoder from crashing on 0-byte data
		if isSpaceOnly(respBody) {
			respBody = []byte("{}")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(respBody)))
		w.WriteHeader(rw.statusCode)
		w.Write(respBody)
		log.Printf("[Proxy] SendUserCascadeMessage upstream success: status=%d bodyLen=%d", rw.statusCode, len(respBody))
	} else {
		w.WriteHeader(rw.statusCode)
		w.Write(respBody)
		log.Printf("[Proxy] SendUserCascadeMessage upstream error: status=%d body=%s", rw.statusCode, string(respBody))
	}
}

func (p *Proxy) handleJetboxWriteState(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string) {
	// Protect against OOM for excessively large state requests by limiting to 5MB
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 5*1024*1024))
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	var stateReq struct {
		AppState struct {
			LastSelectedAgentModel string `json:"lastSelectedAgentModel"`
		} `json:"appState"`
	}
	if err := json.Unmarshal(bodyBytes, &stateReq); err == nil {
		model := stateReq.AppState.LastSelectedAgentModel
		if model != "" {
			modelEnum := resolveModelEnum(model)
			canonicalName := canonicalModelName(model)
			if canonicalName == "" {
				canonicalName = model
			}
			cascadeID := r.Header.Get("X-Cascade-Id")
			if cascadeID == "" {
				cascadeID = r.URL.Query().Get("cascade_id")
			}
			lastCfg := p.GetCascadeConfig(cascadeID, 0, "")
			patched := applyModelToCascadeConfig(lastCfg, modelEnum, canonicalName)
			if patchedBytes, err := json.Marshal(patched); err == nil {
				SetLastKnownCascadeConfig(patchedBytes)
				if cascadeID != "" {
					SetCascadeModel(cascadeID, canonicalName, patchedBytes)
					ClearTrajectoryCache(cascadeID)
				}
			}
			log.Printf("[Proxy] JetboxWriteState: cached active model %s (%s) cascadeId=%s", canonicalName, modelEnum, shortCascadeID(cascadeID))
		}
	}

	fwdReq := r.Clone(r.Context())
	fwdReq.URL.Path = reqPath
	fwdReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	rp.ServeHTTP(w, fwdReq)
}

func (p *Proxy) handleArtifactProxy(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	rp := p.activeProxy
	p.mu.RUnlock()

	if rp == nil {
		if fileRes, err := GetFileContent(r.URL.Path, ""); err == nil {
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fileRes.Content))
			return
		}
		http.Error(w, "Antigravity instance unavailable", http.StatusServiceUnavailable)
		return
	}

	rp.ServeHTTP(w, r)
}

func (p *Proxy) handleStartCascadeProxy(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string) {
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 5*1024*1024))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var rawMap map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &rawMap); err == nil {
		if src, ok := rawMap["source"].(string); !ok || src == "" || src == "CORTEX_TRAJECTORY_SOURCE_UNSPECIFIED" {
			rawMap["source"] = "CORTEX_TRAJECTORY_SOURCE_INTERACTIVE_CASCADE"
		}
		if modifiedBytes, err := json.Marshal(rawMap); err == nil {
			bodyBytes = modifiedBytes
		}
	}

	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}
	r.ContentLength = int64(len(bodyBytes))
	r.Header.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
	r.URL.Path = reqPath

	rp.ServeHTTP(w, r)
}

func (p *Proxy) handleDeleteCascadeTrajectory(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string) {
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 5*1024*1024))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var reqData struct {
		CascadeID string `json:"cascadeId"`
	}
	_ = json.Unmarshal(bodyBytes, &reqData)

	// SEC-7: Strict allowlist — cascade IDs are UUID-like alphanumeric strings.
	// Rejects any ID containing path separators, null bytes, or shell metacharacters.
	isSafeCascadeID := cascadeIDRe.MatchString(reqData.CascadeID)

	// S3 fix: only set the in-memory tombstone pre-emptively (so stream/list filters
	// hide the cascade immediately). File deletion is deferred to the success branch
	// below — doing it here would permanently destroy local files if upstream rejects.
	if isSafeCascadeID {
		RecordDeletedCascade(reqData.CascadeID)
	}

	fwdReq := r.Clone(r.Context())
	fwdReq.URL.Path = reqPath
	fwdReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	rec := newBufferedResponseWriter()
	defer rec.release()
	rp.ServeHTTP(rec, fwdReq)

	if rec.statusCode >= 200 && rec.statusCode < 300 {
		if isSafeCascadeID {
			RecordDeletedCascade(reqData.CascadeID)
			ClearTrajectoryCache(reqData.CascadeID)
			// Clean up local files only after upstream confirms deletion.
			if home, err := os.UserHomeDir(); err == nil {
				annPath := filepath.Join(home, ".gemini", "antigravity", "annotations", reqData.CascadeID+".pbtxt")
				_ = os.Remove(annPath)

				convDir := filepath.Join(home, ".gemini", "antigravity", "conversations")
				_ = os.Remove(filepath.Join(convDir, reqData.CascadeID+".db"))
				_ = os.Remove(filepath.Join(convDir, reqData.CascadeID+".db-wal"))
				_ = os.Remove(filepath.Join(convDir, reqData.CascadeID+".db-shm"))

				brainDir := filepath.Join(home, ".gemini", "antigravity", "brain", reqData.CascadeID)
				_ = os.RemoveAll(brainDir)
			}
			log.Printf("[Proxy] Deleted cascade trajectory: %s (cache, tombstone & files cleared)", shortCascadeID(reqData.CascadeID))
		}
	} else if rec.statusCode >= 400 {
		// Upstream explicitly rejected deletion; release the tombstone so the cascade reappears.
		if isSafeCascadeID {
			RemoveDeletedCascadeTombstone(reqData.CascadeID)
		}
	}

	for k, v := range rec.header {
		w.Header()[k] = v
	}
	w.WriteHeader(rec.statusCode)
	w.Write(rec.body.Bytes())
}

func (p *Proxy) handleDeleteAgentMessage(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string) {
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 5*1024*1024))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var reqData struct {
		MessageID string `json:"messageId"`
		Recipient string `json:"recipient"`
	}
	_ = json.Unmarshal(bodyBytes, &reqData)

	cascadeID := reqData.Recipient
	messageID := reqData.MessageID

	if messageID != "" {
		RecordDeletedMessage(cascadeID, messageID)
		RemovePendingMessageFromCache(cascadeID, messageID)
	}
	ClearPendingMessagesCache(cascadeID)

	fwdReq := r.Clone(r.Context())
	fwdReq.URL.Path = reqPath
	fwdReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	rec := newBufferedResponseWriter()
	defer rec.release()
	rp.ServeHTTP(rec, fwdReq)

	if rec.statusCode >= 200 && rec.statusCode < 300 {
		if messageID != "" {
			RemovePendingMessageFromCache(cascadeID, messageID)
		}
		ClearPendingMessagesCache(cascadeID)
	} else if rec.statusCode >= 400 {
		if messageID != "" {
			RemoveDeletedMessageTombstone(cascadeID, messageID)
		}
	}

	for k, v := range rec.header {
		w.Header()[k] = v
	}
	w.WriteHeader(rec.statusCode)
	w.Write(rec.body.Bytes())
}

func (p *Proxy) handleUpdateConversationAnnotations(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string) {
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 5*1024*1024))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var payload struct {
		CascadeIDs  []string `json:"cascadeIds"`
		Annotations struct {
			Title string `json:"title"`
		} `json:"annotations"`
	}
	if err := json.Unmarshal(bodyBytes, &payload); err == nil {
		var activeIDs []string
		for _, cid := range payload.CascadeIDs {
			if !IsDeletedCascade(cid) {
				activeIDs = append(activeIDs, cid)
			}
		}
		if len(activeIDs) == 0 && len(payload.CascadeIDs) > 0 {
			// All requested cascades are deleted tombstones; absorb without reviving upstream
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{}"))
			return
		}

		title := strings.TrimSpace(payload.Annotations.Title)
		if title != "" && title != "未命名会话" {
			defaultTrajCache.cascadeTitlesMu.Lock()
			for _, cid := range activeIDs {
				if cid != "" {
					defaultTrajCache.cascadeTitles[cid] = title
					writeAnnotationTitle(cid, title)
				}
			}
			defaultTrajCache.cascadeTitlesMu.Unlock()
		}
	}

	p.SuppressDesktopFocus(AntiReflectionDuration)

	fwdReq := r.Clone(r.Context())
	fwdReq.URL.Path = reqPath
	fwdReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	rp.ServeHTTP(w, fwdReq)
}

// InteractionSubmitRequest represents user decision submitted from mobile client.
type InteractionSubmitRequest struct {
	CascadeID       string `json:"cascadeId"`
	TrajectoryID    string `json:"trajectoryId"`
	StepIndex       int    `json:"stepIndex"`
	Type            string `json:"type"` // "permission", "ask_question", "file_permission", "run_command"
	OptionID        string `json:"optionId"`
	Scope           int    `json:"scope"`
	Allow           bool   `json:"allow"`
	WriteInResponse string `json:"writeInResponse"`
	Skipped         bool   `json:"skipped"`
	Target          string `json:"target,omitempty"`
}

// HandleCascadeInteraction submits user choice to upstream HandleCascadeUserInteraction.
func (p *Proxy) HandleCascadeInteraction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// SEC-3: Limit request body to prevent OOM from oversized payloads.
	body, err := io.ReadAll(io.LimitReader(r.Body, 1*1024*1024))
	if err != nil {
		writeJSONError(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	var req InteractionSubmitRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	p.mu.RLock()
	port := p.activePort
	token := p.activeToken
	p.mu.RUnlock()

	if port == 0 {
		http.Error(w, `{"error":"upstream not connected"}`, http.StatusServiceUnavailable)
		return
	}

	type userInteractionPayload struct {
		TrajectoryID string                 `json:"trajectoryId"`
		StepIndex    int                    `json:"stepIndex"`
		Permission   map[string]interface{} `json:"permission,omitempty"`
		AskQuestion  map[string]interface{} `json:"askQuestion,omitempty"`
		RunCommand   map[string]interface{} `json:"runCommand,omitempty"`
	}

	type rpcRequest struct {
		CascadeID   string                 `json:"cascadeId"`
		Interaction userInteractionPayload `json:"interaction"`
	}

	interaction := userInteractionPayload{
		TrajectoryID: req.TrajectoryID,
		StepIndex:    req.StepIndex,
	}

	switch req.Type {
	case "permission", "file_permission":
		allow := req.Allow
		scope := req.Scope
		denyReason := ""
		if req.Skipped {
			allow = false
		} else if req.OptionID == "5" || !req.Allow {
			allow = false
			denyReason = req.WriteInResponse
		} else {
			allow = true
			if scope <= 0 {
				scope = 1
			}
		}
		interaction.Permission = map[string]interface{}{
			"allow":               allow,
			"scope":               scope,
			"userDenyInstruction": denyReason,
			"editedTarget":        req.Target,
		}

	case "ask_question":
		if req.Skipped {
			interaction.AskQuestion = map[string]interface{}{
				"responses": []map[string]interface{}{
					{
						"selectedOptionIds": []string{},
						"writeInResponse":   "",
						"skipped":           true,
					},
				},
				"cancelled": true,
			}
		} else {
			opts := []string{}
			if req.OptionID != "" && req.OptionID != "5" && req.OptionID != "__write_in__" {
				opts = append(opts, req.OptionID)
			}
			interaction.AskQuestion = map[string]interface{}{
				"responses": []map[string]interface{}{
					{
						"selectedOptionIds": opts,
						"writeInResponse":   req.WriteInResponse,
						"skipped":           false,
					},
				},
				"cancelled": false,
			}
		}

	case "run_command":
		interaction.RunCommand = map[string]interface{}{
			"confirm":              req.Allow && !req.Skipped,
			"submittedCommandLine": req.Target,
		}

	default:
		interaction.Permission = map[string]interface{}{
			"allow":               req.Allow && !req.Skipped,
			"scope":               req.Scope,
			"userDenyInstruction": req.WriteInResponse,
		}
	}

	rpcReq := rpcRequest{
		CascadeID:   req.CascadeID,
		Interaction: interaction,
	}

	bodyBytes, err := json.Marshal(rpcReq)
	if err != nil {
		writeJSONError(w, "failed to encode: "+err.Error(), http.StatusInternalServerError)
		return
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/HandleCascadeUserInteraction", port)
	upstreamReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		writeJSONError(w, "failed to create upstream request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		upstreamReq.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.mediumClient.Do(upstreamReq)
	if err != nil {
		writeJSONError(w, "upstream call failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("[Proxy] HandleCascadeUserInteraction error (%d): %s", resp.StatusCode, string(respBody))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(respBody)
		return
	}

	// Invalidate cache immediately so next poll and stream capture the new active state
	ClearTrajectoryCache(req.CascadeID)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success":true}`))
}

// handleCascadeTaskStop terminates a specific running background step via CancelCascadeSteps.
func (p *Proxy) handleCascadeTaskStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CascadeID string `json:"cascadeId"`
		StepIndex int    `json:"stepIndex"`
		TaskID    string `json:"taskId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.CascadeID == "" {
		http.Error(w, `{"error":"missing cascadeId"}`, http.StatusBadRequest)
		return
	}

	p.mu.RLock()
	port := p.activePort
	token := p.activeToken
	p.mu.RUnlock()

	if port == 0 {
		http.Error(w, `{"error":"upstream not connected"}`, http.StatusServiceUnavailable)
		return
	}

	if err := p.CancelCascadeStep(req.CascadeID, req.StepIndex, port, token); err != nil {
		log.Printf("[Proxy] CancelCascadeStep failed (cascade: %s, step: %d): %v", req.CascadeID, req.StepIndex, err)
		writeJSONError(w, "cancel step failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Immediately invalidate trajectory cache so next stream tick picks up the changed status
	ClearTrajectoryCache(req.CascadeID)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success":true}`))
}

func (p *Proxy) handleGetAllCascadeTrajectories(w http.ResponseWriter, r *http.Request, port int, token string) {
	// 1. Ensure historical trajectories on disk are loaded into upstream memory for this port
	if port > 0 && !HasSyncedHistoricalTrajectories(port) {
		_ = p.SyncHistoricalTrajectories(port, token)
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", port)
	bodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, 5*1024*1024))
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
			if t := p.lookupCascadeTitle(id, port, token); t != "" && t != "未命名会话" {
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

	// Identify candidate cascades that might need user action (permissions or CanProceed plan feedback)
	candidates := make(map[string]bool)

	type recentItem struct {
		id string
		t  time.Time
	}
	var recentItems []recentItem

	for id, s := range summaries {
		status, _ := s["status"].(string)
		if status == "CASCADE_RUN_STATUS_RUNNING" {
			candidates[id] = true
		} else {
			// Extract timestamp with fallbacks across lastModifiedTime, lastUserViewTime, createdTime, lastUserInputTime
			var modTime time.Time
			foundTime := false

			if modStr, ok := s["lastModifiedTime"].(string); ok && modStr != "" {
				if t, err := parseTime(modStr); err == nil {
					modTime = t
					foundTime = true
				}
			}
			if !foundTime {
				if ann, ok := s["annotations"].(map[string]interface{}); ok {
					if uvStr, ok := ann["lastUserViewTime"].(string); ok && uvStr != "" {
						if t, err := parseTime(uvStr); err == nil {
							modTime = t
							foundTime = true
						}
					}
				}
			}
			if !foundTime {
				if ctStr, ok := s["createdTime"].(string); ok && ctStr != "" {
					if t, err := parseTime(ctStr); err == nil {
						modTime = t
						foundTime = true
					}
				}
			}
			if !foundTime {
				if uiStr, ok := s["lastUserInputTime"].(string); ok && uiStr != "" {
					if t, err := parseTime(uiStr); err == nil {
						modTime = t
						foundTime = true
					}
				}
			}

			if foundTime {
				recentItems = append(recentItems, recentItem{id: id, t: modTime})
			}
		}
	}

	// Check recent sessions for CanProceed / PendingInteraction:
	// Check up to 10 most recent sessions (within 48 hours) to prevent upstream N+1 RPC storms.
	// For any session already cached with terminal status and within 60s TTL, parse directly without RPC.
	if len(recentItems) > 0 {
		sort.Slice(recentItems, func(i, j int) bool {
			return recentItems[i].t.After(recentItems[j].t)
		})
		for i := 0; i < len(recentItems) && i < 10; i++ {
			if i < 5 || time.Since(recentItems[i].t) < 48*time.Hour {
				cid := recentItems[i].id
				// Fast path: if already cached, non-running, and within TTL, reuse directly.
				// P2 fix: copy the data pointer under a short RLock, then parse outside the lock.
				defaultTrajCache.trajCacheMu.RLock()
				cached, ok := defaultTrajCache.trajCache[cid]
				var cachedData *upstreamTrajectoryResp
				if ok && cached != nil && cached.data != nil {
					if cached.data.Status != "" && cached.data.Status != "CASCADE_RUN_STATUS_RUNNING" && time.Since(cached.fetchedAt) < 60*time.Second {
						cachedData = cached.data
					}
				}
				defaultTrajCache.trajCacheMu.RUnlock()

				if cachedData != nil {
					// ParseTrajectoryDetails does significant JSON work — keep it outside any lock.
					details := p.ParseTrajectoryDetails(cachedData)
					if details.PendingInteraction != nil || details.CanProceed {
						if summaries[cid] != nil {
							summaries[cid]["needsInput"] = true
						}
					}
					if details.HasError {
						if summaries[cid] != nil {
							summaries[cid]["hasError"] = true
							summaries[cid]["errorMessage"] = details.ErrorMessage
						}
					}
					continue
				}
				candidates[cid] = true
			}
		}
	}

	// Fast disk inspection: If ~/.gemini/antigravity/brain/<id>/implementation_plan.md.metadata.json has requestFeedback == true
	// and walkthrough.md does not yet exist (plan not yet delivered).
	// Both file reads are served from metadataCache (TTL: 10s) to avoid per-request syscalls.
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		for cid := range summaries {
			if candidates[cid] {
				continue
			}
			walkthroughFile := filepath.Join(home, ".gemini/antigravity/brain", cid, "walkthrough.md")
			// Cache os.Stat result via metadataCache using a "stat:" key prefix.
			walkthroughStatKey := "stat:" + walkthroughFile
			defaultTrajCache.metadataCacheMu.RLock()
			statEntry, statCached := defaultTrajCache.metadataCache[walkthroughStatKey]
			defaultTrajCache.metadataCacheMu.RUnlock()
			walkthroughExists := false
			if statCached && time.Since(statEntry.fetchedAt) < metadataCacheTTL {
				walkthroughExists = statEntry.requestFeedback // re-purposed: true = file exists
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
				candidates[cid] = true
			}
		}
	}

	// Also check any cascade in trajCache that has PendingInteraction, CanProceed, or HasError.
	// P2 fix: snapshot entries under RLock, then parse outside the lock.
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
		details := p.ParseTrajectoryDetails(sn.data)
		if details.PendingInteraction != nil || details.CanProceed || details.HasError {
			if _, exists := summaries[sn.cid]; exists {
				candidates[sn.cid] = true
			}
		}
	}

	if len(candidates) > 0 {
		var mu sync.Mutex
		actionMap := make(map[string]bool)
		errorMap := make(map[string]string)
		clearedErrorMap := make(map[string]string)
		sem := make(chan struct{}, 4) // Limit concurrent upstream RPCs to prevent hammering language_server

		// M-5: Add overall timeout to prevent blocking indefinitely on concurrent RPCs
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()

		var wg sync.WaitGroup
		for cid := range candidates {
			wg.Add(1)
			go func(cascadeID string) {
				defer wg.Done()
				select {
				case sem <- struct{}{}: // Acquire semaphore slot
					defer func() { <-sem }() // Release semaphore slot
				case <-ctx.Done():
					return
				}
				raw, err := p.fetchUpstreamTrajectory(cascadeID, port, token)
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

		// Wait for completion or timeout, whichever comes first
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-ctx.Done():
			log.Printf("[Proxy] GetAllCascadeTrajectories: timeout after 8s, returning partial results (%d/%d checked)", len(actionMap), len(candidates))
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
		log.Printf("[Proxy] GetAllCascadeTrajectories: failed to encode response: %v", err)
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
