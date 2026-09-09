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

	// Strip /api prefix
	r.URL.Path = reqPath
	rp.ServeHTTP(w, r)
}

type responseObserver struct {
	http.ResponseWriter
	statusCode int
	body       bytes.Buffer
}

func (ro *responseObserver) WriteHeader(statusCode int) {
	ro.statusCode = statusCode
	ro.ResponseWriter.WriteHeader(statusCode)
}

func (ro *responseObserver) Write(b []byte) (int, error) {
	if ro.statusCode >= 400 && ro.body.Len() < 2048 {
		ro.body.Write(b)
	}
	return ro.ResponseWriter.Write(b)
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

	obs := &responseObserver{ResponseWriter: w, statusCode: http.StatusOK}
	rp.ServeHTTP(obs, r)

	if obs.statusCode >= 400 {
		log.Printf("[Proxy] SendUserCascadeMessage upstream error: status=%d body=%s", obs.statusCode, obs.body.String())
	} else {
		log.Printf("[Proxy] SendUserCascadeMessage upstream success: status=%d", obs.statusCode)
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
