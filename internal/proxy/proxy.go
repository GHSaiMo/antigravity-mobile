package proxy

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/inspector"
)

// GatewayStatus represents the public status of the gateway.
type GatewayStatus struct {
	Status    string                  `json:"status"`
	Upstream  *inspector.InstanceInfo `json:"upstream,omitempty"`
	Timestamp time.Time               `json:"timestamp"`
}

// Proxy routes and proxies HTTP/RPC requests to the Antigravity language_server.
type Proxy struct {
	insp      *inspector.Inspector
	transport *http.Transport
	startTime time.Time

	mu          sync.RWMutex
	activeProxy *httputil.ReverseProxy
	activePort  int
	activeToken string
}

// NewProxy creates a new reverse proxy backed by the inspector.
func NewProxy(insp *inspector.Inspector) *Proxy {
	tr := &http.Transport{
		TLSClientConfig:    &tls.Config{InsecureSkipVerify: true},
		DisableCompression: true,
	}

	p := &Proxy{
		insp:      insp,
		transport: tr,
		startTime: time.Now(),
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

func (p *Proxy) handleStatus(w http.ResponseWriter, r *http.Request) {
	cur := p.insp.Current()
	status := "disconnected"
	if cur != nil && cur.IsHealthy {
		status = "connected"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GatewayStatus{
		Status:    status,
		Upstream:  cur,
		Timestamp: time.Now(),
	})
}

func (p *Proxy) handleRescan(w http.ResponseWriter, r *http.Request) {
	info := p.insp.Scan()
	status := "failed"
	if info != nil && info.IsHealthy {
		status = "connected"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GatewayStatus{
		Status:    status,
		Upstream:  info,
		Timestamp: time.Now(),
	})
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
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "unavailable",
			"message": "Antigravity language_server is not connected",
		})
		return
	}

	reqPath := strings.TrimPrefix(r.URL.Path, "/api")
	log.Printf("[Proxy] RPC: %s %s", r.Method, reqPath)
	if strings.HasSuffix(reqPath, "/SendUserCascadeMessage") && r.Method == http.MethodPost {
		p.handleSendUserCascadeMessage(w, r, rp, reqPath, port, token)
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

	// Strip /api prefix
	r.URL.Path = reqPath
	rp.ServeHTTP(w, r)
}

type bufferedResponseWriter struct {
	header     http.Header
	body       bytes.Buffer
	statusCode int
}

func newBufferedResponseWriter() *bufferedResponseWriter {
	return &bufferedResponseWriter{
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
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

func (p *Proxy) handleSendUserCascadeMessage(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string, port int, token string) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var rawMap map[string]interface{}
	cascadeID := ""
	if err := json.Unmarshal(bodyBytes, &rawMap); err == nil {
		cascadeID, _ = rawMap["cascadeId"].(string)

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

		if len(configToUse) > 0 {
			var cfgObj interface{}
			if err := json.Unmarshal(configToUse, &cfgObj); err == nil {
				rawMap["cascadeConfig"] = cfgObj
			}
			SetLastKnownCascadeConfig(configToUse)
		}

		delete(rawMap, "cascadeConfigRaw")

		if modifiedBytes, err := json.Marshal(rawMap); err == nil {
			bodyBytes = modifiedBytes
		}

		if cascadeID != "" {
			ClearTrajectoryCache(cascadeID)
		}
	}

	log.Printf("[Proxy] SendUserCascadeMessage: cascadeId=%s payloadLen=%d", cascadeID, len(bodyBytes))

	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}
	r.ContentLength = int64(len(bodyBytes))
	r.Header.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
	r.URL.Path = reqPath

	rw := newBufferedResponseWriter()
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
		if len(bytes.TrimSpace(respBody)) == 0 {
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

func (p *Proxy) handleArtifactProxy(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	rp := p.activeProxy
	token := p.activeToken
	p.mu.RUnlock()

	if rp == nil {
		http.Error(w, "Antigravity instance unavailable", http.StatusServiceUnavailable)
		return
	}

	// Inject csrf query parameter if not present
	q := r.URL.Query()
	if q.Get("csrf") == "" && token != "" {
		q.Set("csrf", token)
		r.URL.RawQuery = q.Encode()
	}

	rp.ServeHTTP(w, r)
}

func (p *Proxy) handleStartCascadeProxy(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string) {
	bodyBytes, err := io.ReadAll(r.Body)
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

	var req InteractionSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"invalid request: %s"}`, err.Error()), http.StatusBadRequest)
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
		http.Error(w, fmt.Sprintf(`{"error":"failed to encode: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/HandleCascadeUserInteraction", port)
	upstreamReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to create upstream request: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		upstreamReq.Header.Set("x-codeium-csrf-token", token)
	}

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: p.transport,
	}

	resp, err := client.Do(upstreamReq)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"upstream call failed: %s"}`, err.Error()), http.StatusBadGateway)
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

func (p *Proxy) handleGetAllCascadeTrajectories(w http.ResponseWriter, r *http.Request, port int, token string) {
	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", port)
	bodyBytes, _ := io.ReadAll(r.Body)
	if len(bodyBytes) == 0 {
		bodyBytes = []byte("{}")
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	client := &http.Client{
		Timeout:   4 * time.Second,
		Transport: p.transport,
	}
	resp, err := client.Do(req)
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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rawMap)
		return
	}

	var summaries map[string]map[string]interface{}
	if err := json.Unmarshal(summariesRaw, &summaries); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rawMap)
		return
	}

	// Identify candidate cascades that might need user action
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
		} else if modStr, ok := s["lastModifiedTime"].(string); ok {
			if t, err := time.Parse(time.RFC3339Nano, modStr); err == nil {
				recentItems = append(recentItems, recentItem{id: id, t: t})
			}
		}
	}

	// Check top 2 most recently modified sessions (if within 2 hours) for CanProceed (Proceed button)
	if len(recentItems) > 0 {
		sort.Slice(recentItems, func(i, j int) bool {
			return recentItems[i].t.After(recentItems[j].t)
		})
		for i := 0; i < len(recentItems) && i < 2; i++ {
			if time.Since(recentItems[i].t) < 2*time.Hour {
				candidates[recentItems[i].id] = true
			}
		}
	}

	// Also check any cascade in trajCache that has PendingInteraction or CanProceed
	trajCacheMu.Lock()
	for cid, entry := range trajCache {
		if entry != nil && entry.data != nil {
			details := p.ParseTrajectoryDetails(entry.data)
			if details.PendingInteraction != nil || details.CanProceed {
				if _, exists := summaries[cid]; exists {
					candidates[cid] = true
				}
			}
		}
	}
	trajCacheMu.Unlock()

	if len(candidates) > 0 {
		var wg sync.WaitGroup
		var mu sync.Mutex
		actionMap := make(map[string]bool)

		for cid := range candidates {
			wg.Add(1)
			go func(cascadeID string) {
				defer wg.Done()
				raw, err := p.fetchUpstreamTrajectory(cascadeID, port, token)
				if err == nil && raw != nil {
					details := p.ParseTrajectoryDetails(raw)
					if details.PendingInteraction != nil || details.CanProceed {
						mu.Lock()
						actionMap[cascadeID] = true
						mu.Unlock()
					}
				}
			}(cid)
		}
		wg.Wait()

		for cid, hasAction := range actionMap {
			if hasAction && summaries[cid] != nil {
				summaries[cid]["needsInput"] = true
			}
		}
	}

	rawMap["trajectorySummaries"], _ = json.Marshal(summaries)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rawMap)
}


