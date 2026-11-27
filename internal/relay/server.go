package relay

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
)

const (
	TunnelUpgradePath        = "/_mgy_tunnel"
	RelayVersion             = "1.1.0"
	HeaderPoolMaxConns       = "X-Mgy-Relay-Pool-Max"
	defaultMaxConnsPerTunnel = 4
)

// ServerConfig configures the mgy-relay reverse tunnel server.
type ServerConfig struct {
	SigningSecret       string
	ControlHosts        []string      // hosts that serve /healthz and /_mgy_tunnel (e.g. "relay.jiuge.space")
	AllowedSuffixes     []string      // allowed tunnel domain suffixes (e.g. ".jiuge.space")
	ReservedHosts       []string      // domains forbidden from being claimed as user tunnels
	MaxTunnels          int           // global concurrent tunnel ceiling (default 10000)
	MaxStreamsPerTunnel int           // concurrent HTTP/WS streams per tunnel (default 64)
	MaxConnsPerTunnel   int           // max parallel yamux connections per gateway domain (default 4)
	TokenMaxAge         time.Duration // 0 = no expiry (tokens are long-lived until rotated)
	Logger              *slog.Logger
}

var errGatewayOffline = errors.New("gateway_offline")

type tunnelConn struct {
	slotID      string
	gen         string
	session     *yamux.Session
	connectedAt time.Time
	active      atomic.Int64
}

type trackedStreamConn struct {
	net.Conn
	tc        *tunnelConn
	closeOnce sync.Once
}

func (c *trackedStreamConn) Close() error {
	err := c.Conn.Close()
	c.closeOnce.Do(func() {
		c.tc.active.Add(-1)
	})
	return err
}

type tunnelGroup struct {
	domain    string
	shortID   string
	ownerHash string
	sem       chan struct{}
	proxy     *httputil.ReverseProxy

	mu    sync.RWMutex
	conns []*tunnelConn
	rr    atomic.Uint64
}

func (g *tunnelGroup) hasLiveConn() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	for _, c := range g.conns {
		if !c.session.IsClosed() {
			return true
		}
	}
	return false
}

// openStream selects live yamux connections in least-loaded order (rotating ties across slots)
// and falls back across sibling slots automatically if a slot dies right before OpenStream.
func (g *tunnelGroup) openStream(ctx context.Context) (net.Conn, error) {
	type candidate struct {
		tc   *tunnelConn
		load int64
		ord  int
	}

	g.mu.RLock()
	n := len(g.conns)
	if n == 0 {
		g.mu.RUnlock()
		return nil, errGatewayOffline
	}
	start := int(g.rr.Add(1) % uint64(n))
	cands := make([]candidate, 0, n)
	for i := 0; i < n; i++ {
		c := g.conns[(start+i)%n]
		if c.session.IsClosed() {
			continue
		}
		cands = append(cands, candidate{
			tc:   c,
			load: c.active.Load() + int64(c.session.NumStreams()),
			ord:  i,
		})
	}
	g.mu.RUnlock()

	if len(cands) == 0 {
		return nil, errGatewayOffline
	}
	if len(cands) > 1 {
		sort.Slice(cands, func(i, j int) bool {
			if cands[i].load != cands[j].load {
				return cands[i].load < cands[j].load
			}
			return cands[i].ord < cands[j].ord
		})
	}

	var lastErr error
	for _, cand := range cands {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		stream, err := cand.tc.session.OpenStream()
		if err == nil {
			cand.tc.active.Add(1)
			return &trackedStreamConn{Conn: stream, tc: cand.tc}, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errGatewayOffline
}

// Server terminates inbound gateway WSS tunnels and routes public HTTPS/WSS traffic
// by Host header to the corresponding gateway's multiplexed yamux session pool.
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
	tunnels       map[string]*tunnelGroup // domain -> active tunnel group (1..N yamux conns)
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
	if cfg.MaxConnsPerTunnel <= 0 {
		cfg.MaxConnsPerTunnel = defaultMaxConnsPerTunnel
	}
	if cfg.MaxConnsPerTunnel > 8 {
		cfg.MaxConnsPerTunnel = 8
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

	var suffixes []string
	usesOfficialSpace := false
	usesOfficialApp := false
	for _, s := range cfg.AllowedSuffixes {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		if !strings.HasPrefix(s, ".") {
			s = "." + s
		}
		if s == ".jiuge.space" || strings.HasSuffix(s, ".jiuge.space") {
			usesOfficialSpace = true
		}
		if s == ".multigravity.app" || strings.HasSuffix(s, ".multigravity.app") {
			usesOfficialApp = true
		}
		suffixes = append(suffixes, s)
	}
	for h := range ctrl {
		if h == "jiuge.space" || strings.HasSuffix(h, ".jiuge.space") {
			usesOfficialSpace = true
		}
		if h == "multigravity.app" || strings.HasSuffix(h, ".multigravity.app") {
			usesOfficialApp = true
		}
	}

	reserved := make(map[string]bool)
	if usesOfficialSpace {
		for _, official := range []string{
			"jiuge.space",
			"www.jiuge.space",
			"mgy.jiuge.space",
			"dispatcher.jiuge.space",
			"push.jiuge.space",
			"relay.jiuge.space",
			"hal9000.jiuge.space",
		} {
			reserved[official] = true
		}
	}
	if usesOfficialApp {
		for _, official := range []string{
			"multigravity.app",
			"www.multigravity.app",
			"dispatcher.multigravity.app",
			"push.multigravity.app",
			"relay.multigravity.app",
		} {
			reserved[official] = true
		}
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

	return &Server{
		cfg:           cfg,
		logger:        logger,
		startedAt:     time.Now(),
		controlHosts:  ctrl,
		reservedHosts: reserved,
		suffixes:      suffixes,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  64 << 10,
			WriteBufferSize: 64 << 10,
			CheckOrigin: func(r *http.Request) bool {
				// Tunnel ingress is authenticated by Bearer HMAC token, never cookies.
				return true
			},
		},
		tunnels: make(map[string]*tunnelGroup),
		ipRates: make(map[string]*ipBucket),
	}, nil
}

// ActiveTunnels returns the number of currently connected gateways.
func (s *Server) ActiveTunnels() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tunnels)
}

// ActiveConns returns the total number of live yamux connections across all gateways.
func (s *Server) ActiveConns() int {
	s.mu.RLock()
	groups := make([]*tunnelGroup, 0, len(s.tunnels))
	for _, g := range s.tunnels {
		groups = append(groups, g)
	}
	s.mu.RUnlock()

	total := 0
	for _, g := range groups {
		g.mu.RLock()
		for _, c := range g.conns {
			if !c.session.IsClosed() {
				total++
			}
		}
		g.mu.RUnlock()
	}
	return total
}

// Close terminates all active gateway sessions gracefully.
func (s *Server) Close() {
	s.mu.Lock()
	groups := make([]*tunnelGroup, 0, len(s.tunnels))
	for _, g := range s.tunnels {
		groups = append(groups, g)
	}
	s.tunnels = make(map[string]*tunnelGroup)
	s.mu.Unlock()

	for _, g := range groups {
		g.mu.Lock()
		conns := append([]*tunnelConn(nil), g.conns...)
		g.conns = nil
		g.mu.Unlock()
		for _, c := range conns {
			_ = c.session.Close()
		}
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
		"active_conns":   s.ActiveConns(),
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

	slotID := strings.TrimSpace(r.URL.Query().Get("conn"))
	if len(slotID) > 16 {
		slotID = slotID[:16]
	}
	gen := strings.TrimSpace(r.URL.Query().Get("gen"))
	if len(gen) > 32 {
		gen = gen[:32]
	}

	respHeader := http.Header{
		HeaderPoolMaxConns: []string{strconv.Itoa(s.cfg.MaxConnsPerTunnel)},
	}
	ws, err := s.upgrader.Upgrade(w, r, respHeader)
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

	tc := &tunnelConn{
		slotID:      slotID,
		gen:         gen,
		session:     sess,
		connectedAt: time.Now(),
	}

	// Register atomically into the domain's tunnelGroup.
	// - Legacy clients (no ?conn= slot) replace all existing connections (1:1 backwards-compatible).
	// - Multi-connection pool clients (?conn=0..N&gen=...) coexist up to MaxConnsPerTunnel,
	//   evicting same-slot reconnects and previous process generations automatically.
	var toClose []*tunnelConn
	s.mu.Lock()
	g := s.tunnels[claims.Domain]
	if g != nil && subtle.ConstantTimeCompare([]byte(g.ownerHash), []byte(claims.OwnerHash)) != 1 {
		s.mu.Unlock()
		_ = sess.Close()
		return
	}
	if g == nil {
		if len(s.tunnels) >= s.cfg.MaxTunnels {
			s.mu.Unlock()
			_ = sess.Close()
			return
		}
		g = s.newTunnelGroup(claims)
		s.tunnels[claims.Domain] = g
	}
	g.mu.Lock()
	if slotID == "" {
		toClose = append(toClose, g.conns...)
		g.conns = []*tunnelConn{tc}
	} else {
		kept := make([]*tunnelConn, 0, len(g.conns)+1)
		for _, prev := range g.conns {
			superseded := prev.session.IsClosed() ||
				prev.slotID == "" ||
				prev.slotID == slotID ||
				(gen != "" && prev.gen != "" && prev.gen != gen)
			if superseded {
				toClose = append(toClose, prev)
			} else {
				kept = append(kept, prev)
			}
		}
		kept = append(kept, tc)
		for len(kept) > s.cfg.MaxConnsPerTunnel {
			toClose = append(toClose, kept[0])
			kept = kept[1:]
		}
		g.conns = kept
	}
	groupConns := len(g.conns)
	g.mu.Unlock()
	total := len(s.tunnels)
	s.mu.Unlock()

	for _, prev := range toClose {
		_ = prev.session.Close()
	}

	s.logger.Info("tunnel connected", "sid", claims.ShortID, "region", claims.Region, "slot", slotID, "conns", groupConns, "active_tunnels", total)

	// Block until this yamux session closes, then remove only this connection from its group
	<-sess.CloseChan()
	_ = wsc.Close()

	s.mu.Lock()
	remainingConns := 0
	if curGroup := s.tunnels[claims.Domain]; curGroup == g {
		g.mu.Lock()
		filtered := g.conns[:0]
		for _, c := range g.conns {
			if c != tc {
				filtered = append(filtered, c)
			}
		}
		g.conns = filtered
		remainingConns = len(g.conns)
		g.mu.Unlock()
		if remainingConns == 0 {
			delete(s.tunnels, claims.Domain)
		}
	}
	remainingTunnels := len(s.tunnels)
	s.mu.Unlock()

	s.logger.Info("tunnel disconnected", "sid", claims.ShortID, "slot", slotID, "conns", remainingConns, "active_tunnels", remainingTunnels)
}

func (s *Server) newTunnelGroup(claims *TokenClaims) *tunnelGroup {
	g := &tunnelGroup{
		domain:    claims.Domain,
		shortID:   claims.ShortID,
		ownerHash: claims.OwnerHash,
		sem:       make(chan struct{}, s.cfg.MaxStreamsPerTunnel),
	}

	targetURL := &url.URL{Scheme: "http", Host: "mgy.local"}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return g.openStream(ctx)
		},
		// Opening a fresh yamux stream is 0-RTT over the persistent WSS tunnel; disabling HTTP/1.1
		// idle keep-alive pooling eliminates stale-stream EOF 502 errors on non-idempotent POSTs.
		DisableKeepAlives:     true,
		ResponseHeaderTimeout: 30 * time.Second,
		DisableCompression:    true, // preserve upstream gzip/br/ETag transparently
	}

	g.proxy = &httputil.ReverseProxy{
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
			if errors.Is(err, errGatewayOffline) {
				writeRelayJSON(rw, http.StatusServiceUnavailable, map[string]string{
					"error": "gateway_offline",
				})
				return
			}
			writeRelayJSON(rw, http.StatusBadGateway, map[string]string{
				"error": "gateway_unreachable",
			})
		},
	}

	return g
}

func (s *Server) handlePublicProxy(w http.ResponseWriter, r *http.Request, host string) {
	if !s.isDomainAllowed(host) {
		writeRelayJSON(w, http.StatusNotFound, map[string]string{"error": "unknown_host"})
		return
	}

	s.mu.RLock()
	g := s.tunnels[host]
	s.mu.RUnlock()

	if g == nil || !g.hasLiveConn() {
		writeRelayJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "gateway_offline",
		})
		return
	}

	select {
	case g.sem <- struct{}{}:
		defer func() { <-g.sem }()
	default:
		writeRelayJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "tunnel_busy",
		})
		return
	}

	s.activeStreams.Add(1)
	s.totalStreams.Add(1)
	defer func() {
		s.activeStreams.Add(-1)
	}()

	g.proxy.ServeHTTP(w, r)
}

func defaultYamuxConfig() *yamux.Config {
	cfg := yamux.DefaultConfig()
	cfg.LogOutput = io.Discard
	cfg.EnableKeepAlive = true
	cfg.KeepAliveInterval = 20 * time.Second
	cfg.ConnectionWriteTimeout = 20 * time.Second
	cfg.MaxStreamWindowSize = 1 << 20 // 1 MiB per stream to avoid window stalls on lossy WAN links
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
