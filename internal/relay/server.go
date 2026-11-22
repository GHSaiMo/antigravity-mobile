package relay

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
)

const (
	TunnelUpgradePath = "/_mgy_tunnel"
	RelayVersion      = "1.0.0"
)

// ServerConfig configures the mgy-relay reverse tunnel server.
type ServerConfig struct {
	SigningSecret       string
	ControlHosts        []string      // hosts that serve /healthz and /_mgy_tunnel (e.g. "relay.jiuge.space")
	AllowedSuffixes     []string      // allowed tunnel domain suffixes (e.g. ".jiuge.space")
	ReservedHosts       []string      // domains forbidden from being claimed as user tunnels
	MaxTunnels          int           // global concurrent tunnel ceiling (default 10000)
	MaxStreamsPerTunnel int           // concurrent HTTP/WS streams per tunnel (default 64)
	TokenMaxAge         time.Duration // 0 = no expiry (tokens are long-lived until rotated)
	Logger              *slog.Logger
}

type tunnelSession struct {
	domain      string
	shortID     string
	ownerHash   string
	session     *yamux.Session
	proxy       *httputil.ReverseProxy
	sem         chan struct{}
	connectedAt time.Time
	active      atomic.Int64
}

// Server terminates inbound gateway WSS tunnels and routes public HTTPS/WSS traffic
// by Host header to the corresponding gateway's multiplexed yamux session.
// Strict zero-logging: request paths, headers, tokens, and bodies are never logged.
type Server struct {
	cfg           ServerConfig
	logger        *slog.Logger
	startedAt     time.Time
	controlHosts  map[string]bool
	reservedHosts map[string]bool
	suffixes      []string
	upgrader      websocket.Upgrader

	mu            sync.RWMutex
	tunnels       map[string]*tunnelSession // domain -> active session
	activeStreams atomic.Int64
	totalStreams  atomic.Int64

	rateMu  sync.Mutex
	ipRates map[string]*ipBucket
}

type ipBucket struct {
	windowStart time.Time
	count       int
}

// NewServer constructs a new mgy-relay HTTP/WebSocket handler.
func NewServer(cfg ServerConfig) (*Server, error) {
	if strings.TrimSpace(cfg.SigningSecret) == "" {
		return nil, fmt.Errorf("SigningSecret is required")
	}
	if cfg.MaxTunnels <= 0 {
		cfg.MaxTunnels = 10000
	}
	if cfg.MaxStreamsPerTunnel <= 0 {
		cfg.MaxStreamsPerTunnel = 64
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	ctrl := make(map[string]bool, len(cfg.ControlHosts))
	for _, h := range cfg.ControlHosts {
		h = normalizeHost(h)
		if h != "" {
			ctrl[h] = true
		}
	}

	reserved := map[string]bool{
		"jiuge.space":            true,
		"www.jiuge.space":        true,
		"mgy.jiuge.space":        true,
		"dispatcher.jiuge.space": true,
		"push.jiuge.space":       true,
		"relay.jiuge.space":      true,
		"hal9000.jiuge.space":    true,
	}
	for h := range ctrl {
		reserved[h] = true
	}
	for _, h := range cfg.ReservedHosts {
		h = normalizeHost(h)
		if h != "" {
			reserved[h] = true
		}
	}

	var suffixes []string
	for _, s := range cfg.AllowedSuffixes {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if !strings.HasPrefix(s, ".") {
			s = "." + s
		}
		suffixes = append(suffixes, s)
	}

	return &Server{
		cfg:           cfg,
		logger:        logger,
		startedAt:     time.Now(),
		controlHosts:  ctrl,
		reservedHosts: reserved,
		suffixes:      suffixes,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  32 << 10,
			WriteBufferSize: 32 << 10,
			CheckOrigin: func(r *http.Request) bool {
				// Tunnel ingress is authenticated by Bearer HMAC token, never cookies.
				return true
			},
		},
		tunnels: make(map[string]*tunnelSession),
		ipRates: make(map[string]*ipBucket),
	}, nil
}

// ActiveTunnels returns the number of currently connected gateways.
func (s *Server) ActiveTunnels() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tunnels)
}

// Close terminates all active gateway sessions gracefully.
func (s *Server) Close() {
	s.mu.Lock()
	sessions := make([]*tunnelSession, 0, len(s.tunnels))
	for _, ts := range s.tunnels {
		sessions = append(sessions, ts)
	}
	s.tunnels = make(map[string]*tunnelSession)
	s.mu.Unlock()

	for _, ts := range sessions {
		_ = ts.session.Close()
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := normalizeHost(r.Host)

	// 1. Gateway reverse-tunnel registration endpoint
	if r.URL.Path == TunnelUpgradePath && (s.isControlHostOrIP(host) || len(s.controlHosts) == 0) {
		s.handleTunnelConnect(w, r)
		return
	}

	// 2. Relay server health check (only on control hosts, loopback, or raw IP — NEVER hijack user tunnel /healthz!)
	if (r.URL.Path == "/healthz" || r.URL.Path == "/") && s.isControlHostOrIP(host) {
		s.handleHealthz(w, r)
		return
	}

	// 3. Forward public traffic to the registered gateway session for this Host
	s.handlePublicProxy(w, r, host)
}

func (s *Server) isControlHost(host string) bool {
	return s.controlHosts[host]
}

func (s *Server) isControlHostOrIP(host string) bool {
	if s.controlHosts[host] || host == "" || host == "localhost" {
		return true
	}
	return net.ParseIP(host) != nil
}

func (s *Server) isDomainAllowed(domain string) bool {
	if s.reservedHosts[domain] {
		return false
	}
	if len(s.suffixes) == 0 {
		return true
	}
	for _, suf := range s.suffixes {
		if strings.HasSuffix(domain, suf) && len(domain) > len(suf) {
			return true
		}
	}
	return false
}

func (s *Server) allowRegisterIP(ip string) bool {
	now := time.Now()
	s.rateMu.Lock()
	defer s.rateMu.Unlock()

	if len(s.ipRates) > 4096 {
		for k, b := range s.ipRates {
			if now.Sub(b.windowStart) > time.Minute {
				delete(s.ipRates, k)
			}
		}
	}

	b, ok := s.ipRates[ip]
	if !ok || now.Sub(b.windowStart) > time.Minute {
		s.ipRates[ip] = &ipBucket{windowStart: now, count: 1}
		return true
	}
	b.count++
	return b.count <= 30
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeRelayJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"service":        "mgy-relay",
		"version":        RelayVersion,
		"active_tunnels": s.ActiveTunnels(),
		"active_streams": s.activeStreams.Load(),
		"total_streams":  s.totalStreams.Load(),
		"uptime_sec":     int64(time.Since(s.startedAt).Seconds()),
	})
}

func (s *Server) handleTunnelConnect(w http.ResponseWriter, r *http.Request) {
	clientIP := extractClientIP(r)
	if !s.allowRegisterIP(clientIP) {
		writeRelayJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
		return
	}

	token := extractBearerToken(r)
	if token == "" {
		writeRelayJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing_token"})
		return
	}

	claims, err := VerifyToken(s.cfg.SigningSecret, token, time.Now(), s.cfg.TokenMaxAge)
	if err != nil {
		writeRelayJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_token"})
		return
	}

	if !s.isDomainAllowed(claims.Domain) {
		writeRelayJSON(w, http.StatusForbidden, map[string]string{"error": "domain_not_allowed"})
		return
	}

	// Verify capacity and owner binding BEFORE upgrading to WebSocket
	s.mu.RLock()
	existing := s.tunnels[claims.Domain]
	curCount := len(s.tunnels)
	s.mu.RUnlock()

	if existing != nil {
		if subtle.ConstantTimeCompare([]byte(existing.ownerHash), []byte(claims.OwnerHash)) != 1 {
			writeRelayJSON(w, http.StatusForbidden, map[string]string{"error": "domain_owner_mismatch"})
			return
		}
	} else if curCount >= s.cfg.MaxTunnels {
		writeRelayJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "relay_capacity_full"})
		return
	}

	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	wsc := newWSNetConn(ws)
	ycfg := defaultYamuxConfig()
	sess, err := yamux.Server(wsc, ycfg)
	if err != nil {
		_ = wsc.Close()
		return
	}

	ts := s.newTunnelSession(claims, sess)

	// Register atomically, replacing any previous session owned by the same machine
	s.mu.Lock()
	prev := s.tunnels[claims.Domain]
	if prev != nil && subtle.ConstantTimeCompare([]byte(prev.ownerHash), []byte(claims.OwnerHash)) != 1 {
		s.mu.Unlock()
		_ = sess.Close()
		return
	}
	if prev == nil && len(s.tunnels) >= s.cfg.MaxTunnels {
		s.mu.Unlock()
		_ = sess.Close()
		return
	}
	s.tunnels[claims.Domain] = ts
	total := len(s.tunnels)
	s.mu.Unlock()

	if prev != nil {
		_ = prev.session.Close()
	}

	s.logger.Info("tunnel connected", "sid", claims.ShortID, "region", claims.Region, "active_tunnels", total)

	// Block until the yamux session closes, then clean up only if still the registered entry
	<-sess.CloseChan()
	_ = wsc.Close()

	s.mu.Lock()
	if s.tunnels[claims.Domain] == ts {
		delete(s.tunnels, claims.Domain)
	}
	remaining := len(s.tunnels)
	s.mu.Unlock()

	s.logger.Info("tunnel disconnected", "sid", claims.ShortID, "active_tunnels", remaining)
}

func (s *Server) newTunnelSession(claims *TokenClaims, sess *yamux.Session) *tunnelSession {
	ts := &tunnelSession{
		domain:      claims.Domain,
		shortID:     claims.ShortID,
		ownerHash:   claims.OwnerHash,
		session:     sess,
		sem:         make(chan struct{}, s.cfg.MaxStreamsPerTunnel),
		connectedAt: time.Now(),
	}

	targetURL := &url.URL{Scheme: "http", Host: "mgy.local"}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			stream, err := sess.OpenStream()
			if err != nil {
				return nil, err
			}
			return stream, nil
		},
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       60 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DisableCompression:    true, // preserve upstream gzip/br/ETag transparently
	}

	rp := &httputil.ReverseProxy{
		Transport:     transport,
		FlushInterval: -1, // immediate flush for SSE / streaming
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(targetURL)
			pr.Out.Host = pr.In.Host
			pr.SetXForwarded()
			pr.Out.Header.Set("X-Mgy-Relay", "1")
			if cfIP := strings.TrimSpace(pr.In.Header.Get("CF-Connecting-IP")); cfIP != "" {
				pr.Out.Header.Set("CF-Connecting-IP", cfIP)
			}
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			if req.Context().Err() != nil {
				return
			}
			writeRelayJSON(rw, http.StatusBadGateway, map[string]string{
				"error": "gateway_unreachable",
			})
		},
	}

	ts.proxy = rp
	return ts
}

func (s *Server) handlePublicProxy(w http.ResponseWriter, r *http.Request, host string) {
	if !s.isDomainAllowed(host) {
		writeRelayJSON(w, http.StatusNotFound, map[string]string{"error": "unknown_host"})
		return
	}

	s.mu.RLock()
	ts := s.tunnels[host]
	s.mu.RUnlock()

	if ts == nil || ts.session.IsClosed() {
		writeRelayJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "gateway_offline",
		})
		return
	}

	select {
	case ts.sem <- struct{}{}:
		defer func() { <-ts.sem }()
	default:
		writeRelayJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "tunnel_busy",
		})
		return
	}

	ts.active.Add(1)
	s.activeStreams.Add(1)
	s.totalStreams.Add(1)
	defer func() {
		ts.active.Add(-1)
		s.activeStreams.Add(-1)
	}()

	ts.proxy.ServeHTTP(w, r)
}

func defaultYamuxConfig() *yamux.Config {
	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = 20 * time.Second
	cfg.ConnectionWriteTimeout = 15 * time.Second
	cfg.MaxStreamWindowSize = 512 << 10 // 512 KiB per stream
	return cfg
}

func normalizeHost(h string) string {
	h = strings.TrimSpace(h)
	if h == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return strings.ToLower(strings.Trim(h, "[]"))
}

func extractBearerToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

func extractClientIP(r *http.Request) string {
	if cf := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cf != "" {
		return cf
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func writeRelayJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
