package relay

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
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
	defaultConnsPerNode = 3
	maxConnsPerNode     = 4
)

// NodeConfig represents a single VPS relay node dispatched by the Worker pool.
type NodeConfig struct {
	ID        string `json:"id"`
	Region    string `json:"region"`
	Priority  int    `json:"priority"`
	Endpoint  string `json:"endpoint"`
	HealthURL string `json:"health_url"`
	Subdomain string `json:"subdomain"`
	URL       string `json:"url"`
	Token     string `json:"token"`
}

// PoolConfig configures a multi-VPS relay pool client with concurrent RTT race,
// hysteresis pinning, automatic failover, and periodic pool refresh.
type PoolConfig struct {
	Nodes             []NodeConfig
	Listener          *StreamListener // preferred: direct in-process E2EE-guarded HTTP listener
	LocalAddr         string          // fallback if Listener is nil
	ConnsPerNode      int             // parallel yamux connections per node (default 3, max 4; negotiated with server)
	RefreshInterval   time.Duration   // default 6h
	RefreshFunc       func(ctx context.Context) ([]NodeConfig, error)
	OnEndpointChanged func(node NodeConfig)
	OnAllFailed       func()
	ProbeClient       *http.Client
	Logger            *slog.Logger
}

// PoolProvider implements tunnel.Provider across a dynamic pool of mgy-relay VPS nodes.
type PoolProvider struct {
	cfg          PoolConfig
	logger       *slog.Logger
	connsPerNode int

	mu          sync.RWMutex
	nodes       []NodeConfig
	activeNode  NodeConfig
	activeRTT   time.Duration
	running     bool
	cancel      context.CancelFunc
	epochCancel context.CancelFunc
	done        chan struct{}
	sessions    map[*yamux.Session]struct{}
}

// NewPoolProvider initializes a multi-node relay pool provider.
func NewPoolProvider(cfg PoolConfig) (*PoolProvider, error) {
	var valid []NodeConfig
	for _, n := range cfg.Nodes {
		if strings.TrimSpace(n.Endpoint) != "" && strings.TrimSpace(n.Token) != "" && strings.TrimSpace(n.URL) != "" {
			valid = append(valid, n)
		}
	}
	if len(valid) == 0 {
		return nil, fmt.Errorf("relay pool has no valid nodes")
	}
	if cfg.RefreshInterval <= 0 {
		cfg.RefreshInterval = 6 * time.Hour
	}
	if cfg.LocalAddr == "" {
		cfg.LocalAddr = "127.0.0.1:58900"
	}
	connsPerNode := cfg.ConnsPerNode
	if connsPerNode <= 0 {
		if envVal := strings.TrimSpace(os.Getenv("MGY_RELAY_CONNS")); envVal != "" {
			if n, err := strconv.Atoi(envVal); err == nil && n > 0 {
				connsPerNode = n
			}
		}
	}
	if connsPerNode <= 0 {
		connsPerNode = defaultConnsPerNode
	}
	if connsPerNode > maxConnsPerNode {
		connsPerNode = maxConnsPerNode
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &PoolProvider{
		cfg:          cfg,
		logger:       logger,
		connsPerNode: connsPerNode,
		nodes:        valid,
		activeNode:   valid[0],
		sessions:     make(map[*yamux.Session]struct{}),
	}, nil
}

func (p *PoolProvider) PublicURL() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.activeNode.URL
}

func (p *PoolProvider) Subdomain() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.activeNode.Subdomain
}

func (p *PoolProvider) Mode() string {
	return "relay_v1"
}

// ActiveNode returns a snapshot of the currently selected VPS node.
func (p *PoolProvider) ActiveNode() NodeConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.activeNode
}

// Start begins concurrent node RTT probing and launches the supervised failover loop.
func (p *PoolProvider) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	p.cancel = cancel
	p.done = make(chan struct{})
	p.running = true
	nodesSnap := append([]NodeConfig(nil), p.nodes...)
	p.mu.Unlock()

	// Pick lowest-RTT healthy node before establishing initial tunnel
	bestNode, bestRTT := p.selectFastestNode(runCtx, nodesSnap)
	p.setActiveNode(bestNode, bestRTT)

	go p.supervise(runCtx)
	if p.cfg.RefreshFunc != nil {
		go p.refreshLoop(runCtx)
	}
	return nil
}

// Stop terminates all active relay tunnel sessions and halts background supervision.
func (p *PoolProvider) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	cancel := p.cancel
	epochCancel := p.epochCancel
	done := p.done
	p.running = false
	p.mu.Unlock()

	if epochCancel != nil {
		epochCancel()
	}
	if cancel != nil {
		cancel()
	}
	p.closeAllSessions()
	if done != nil {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	}
}

func (p *PoolProvider) addSession(s *yamux.Session) {
	p.mu.Lock()
	p.sessions[s] = struct{}{}
	p.mu.Unlock()
}

func (p *PoolProvider) removeSession(s *yamux.Session) {
	p.mu.Lock()
	delete(p.sessions, s)
	p.mu.Unlock()
}

func (p *PoolProvider) activeSessionCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	n := 0
	for s := range p.sessions {
		if !s.IsClosed() {
			n++
		}
	}
	return n
}

func (p *PoolProvider) totalNumStreams() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	total := 0
	for s := range p.sessions {
		if !s.IsClosed() {
			total += s.NumStreams()
		}
	}
	return total
}

func (p *PoolProvider) closeAllSessions() {
	p.mu.Lock()
	list := make([]*yamux.Session, 0, len(p.sessions))
	for s := range p.sessions {
		list = append(list, s)
	}
	epochCancel := p.epochCancel
	p.mu.Unlock()

	if epochCancel != nil {
		epochCancel()
	}
	for _, s := range list {
		_ = s.Close()
	}
}

func (p *PoolProvider) setActiveNode(node NodeConfig, rtt time.Duration) {
	p.mu.Lock()
	changed := p.activeNode.URL != node.URL || p.activeNode.Endpoint != node.Endpoint
	p.activeNode = node
	if rtt > 0 {
		p.activeRTT = rtt
	}
	p.mu.Unlock()
	if changed && p.cfg.OnEndpointChanged != nil {
		p.cfg.OnEndpointChanged(node)
	}
}

type nodeProbeResult struct {
	node NodeConfig
	rtt  time.Duration
	ok   bool
	idx  int
}

func (p *PoolProvider) selectFastestNode(ctx context.Context, nodes []NodeConfig) (NodeConfig, time.Duration) {
	if len(nodes) == 1 {
		rtt, ok := p.probeSingle(ctx, nodes[0])
		if ok {
			return nodes[0], rtt
		}
		return nodes[0], 0
	}

	probeCtx, cancel := context.WithTimeout(ctx, 4500*time.Millisecond)
	defer cancel()

	resCh := make(chan nodeProbeResult, len(nodes))
	for i, n := range nodes {
		go func(idx int, node NodeConfig) {
			rtt, ok := p.probeSingle(probeCtx, node)
			resCh <- nodeProbeResult{node: node, rtt: rtt, ok: ok, idx: idx}
		}(i, n)
	}

	var results []nodeProbeResult
	for range nodes {
		results = append(results, <-resCh)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].ok != results[j].ok {
			return results[i].ok
		}
		if results[i].ok && results[j].ok {
			// Within 15ms tie, respect Dispatcher's region/priority order
			diff := results[i].rtt - results[j].rtt
			if diff < -15*time.Millisecond {
				return true
			}
			if diff > 15*time.Millisecond {
				return false
			}
		}
		return results[i].idx < results[j].idx
	})

	best := results[0]
	if best.ok {
		p.logger.Info("🌐 [RelayPool] 测速优选完成", "node", best.node.ID, "region", best.node.Region, "rtt_ms", best.rtt.Milliseconds())
		return best.node, best.rtt
	}
	return nodes[0], 0
}

func (p *PoolProvider) probeSingle(ctx context.Context, node NodeConfig) (time.Duration, bool) {
	healthURL := strings.TrimSpace(node.HealthURL)
	if healthURL == "" {
		return 0, false
	}

	const (
		attemptTimeout   = 2000 * time.Millisecond
		fastAcceptCutoff = 800 * time.Millisecond
	)

	var bestRTT time.Duration
	var gotOK bool

	// Prefer direct (Proxy: nil) probes so a developer's local Clash/VPN proxy
	// (often routed via US/EU nodes) does not distort physical RTT selection.
	for attempt := 0; attempt < 2; attempt++ {
		if ctx.Err() != nil {
			break
		}
		attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
		rtt, ok := p.probeOnce(attemptCtx, healthURL, false)
		cancel()
		if ok {
			if !gotOK || rtt < bestRTT {
				bestRTT = rtt
				gotOK = true
			}
			// Fast healthy response needs no second attempt; only retry on failure or
			// high-RTT spike (e.g., single-packet TCP SYN drop/retransmit on cold start).
			if bestRTT <= fastAcceptCutoff {
				break
			}
		}
	}
	if gotOK || p.cfg.ProbeClient != nil || ctx.Err() != nil {
		return bestRTT, gotOK
	}

	// Corporate-proxy fallback: if direct outbound 443 is blocked and env proxy is set, try proxy once.
	if hasEnvProxyForURL(healthURL) {
		attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
		rtt, ok := p.probeOnce(attemptCtx, healthURL, true)
		cancel()
		if ok {
			return rtt, true
		}
	}
	return 0, false
}

func (p *PoolProvider) probeOnce(ctx context.Context, healthURL string, useProxy bool) (time.Duration, bool) {
	client := p.cfg.ProbeClient
	if client == nil {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.DisableKeepAlives = true
		if useProxy {
			tr.Proxy = http.ProxyFromEnvironment
		} else {
			tr.Proxy = nil
		}
		client = &http.Client{Transport: tr}
		defer tr.CloseIdleConnections()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return 0, false
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, false
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, false
	}
	return time.Since(start), true
}

func hasEnvProxyForURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	// http.ProxyFromEnvironment checks http/https schemes; normalize wss/ws for lookup.
	switch strings.ToLower(u.Scheme) {
	case "wss":
		u.Scheme = "https"
	case "ws":
		u.Scheme = "http"
	}
	proxyURL, err := http.ProxyFromEnvironment(&http.Request{URL: u})
	return err == nil && proxyURL != nil
}

func (p *PoolProvider) supervise(ctx context.Context) {
	defer close(p.done)

	consecutiveOnNode := 0
	allFailedNotified := false
	backoffStep := 0

	for {
		if ctx.Err() != nil {
			return
		}
		node := p.ActiveNode()
		startedAt := time.Now()
		allDownOutage, err := p.runNodeEpoch(ctx, node)
		if ctx.Err() != nil {
			return
		}

		// If refreshLoop explicitly switched activeNode while this epoch was running, reconnect immediately.
		if p.ActiveNode().Endpoint != node.Endpoint {
			consecutiveOnNode = 0
			backoffStep = 0
			continue
		}

		if allDownOutage {
			consecutiveOnNode = 3
			backoffStep++
		} else if time.Since(startedAt) >= 30*time.Second {
			consecutiveOnNode = 0
			backoffStep = 0
			allFailedNotified = false
		} else {
			consecutiveOnNode++
			backoffStep++
		}

		// Failover to next healthy node in pool after 3 consecutive failures on current node
		if consecutiveOnNode >= 3 {
			p.mu.RLock()
			allNodes := append([]NodeConfig(nil), p.nodes...)
			p.mu.RUnlock()

			if len(allNodes) > 1 {
				var alternatives []NodeConfig
				for _, cand := range allNodes {
					if cand.ID != node.ID || cand.Endpoint != node.Endpoint {
						alternatives = append(alternatives, cand)
					}
				}
				nextNode, nextRTT := p.selectFastestNode(ctx, alternatives)
				if nextRTT > 0 {
					p.logger.Warn("🔄 [RelayPool] 当前节点不可达，自动故障转移至备用节点",
						"from", node.ID, "to", nextNode.ID, "rtt_ms", nextRTT.Milliseconds())
					p.setActiveNode(nextNode, nextRTT)
					consecutiveOnNode = 0
					backoffStep = 0
					continue
				}
			}

			// All pool nodes are currently down; notify fallback hook once so gateway can activate Cloudflare fallback
			if !allFailedNotified && p.cfg.OnAllFailed != nil {
				allFailedNotified = true
				go p.cfg.OnAllFailed()
			}
		}

		delay := nextClientBackoff(backoffStep)
		p.logger.Warn("⚠️  [RelayPool] 中继节点连接断开，准备重连",
			"node", node.ID, "err", err, "retry_in", delay.Round(100*time.Millisecond))

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (p *PoolProvider) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(p.cfg.RefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fresh, err := p.cfg.RefreshFunc(ctx)
			if err != nil || len(fresh) == 0 {
				continue
			}
			p.mu.Lock()
			p.nodes = fresh
			curNode := p.activeNode
			p.mu.Unlock()

			// Check if current node was removed from pool
			stillPresent := false
			for _, n := range fresh {
				if n.ID == curNode.ID && n.Endpoint == curNode.Endpoint {
					stillPresent = true
					break
				}
			}

			bestNode, bestRTT := p.selectFastestNode(ctx, fresh)
			if !stillPresent {
				p.logger.Info("🔄 [RelayPool] 当前节点已从池中下线，平滑迁移至新节点", "from", curNode.ID, "to", bestNode.ID)
				p.setActiveNode(bestNode, bestRTT)
				p.closeAllSessions()
				continue
			}

			// Hysteresis: only migrate if new node is >40% faster AND at least 50ms lower RTT, and no active streams
			if bestNode.ID != curNode.ID && bestRTT > 0 {
				curRTT, curOK := p.probeSingle(ctx, curNode)
				noActiveStreams := p.totalNumStreams() == 0
				if noActiveStreams && (!curOK || (curRTT-bestRTT >= 50*time.Millisecond && float64(bestRTT) < float64(curRTT)*0.6)) {
					p.logger.Info("🚀 [RelayPool] 发现更优节点，空闲时无感切换",
						"from", curNode.ID, "from_ms", curRTT.Milliseconds(),
						"to", bestNode.ID, "to_ms", bestRTT.Milliseconds())
					p.setActiveNode(bestNode, bestRTT)
					p.closeAllSessions()
				}
			}
		}
	}
}

func generateEpochGen() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("%x", time.Now().UnixNano()&0xffffffff)
}

func appendTunnelSlotQuery(endpoint, gen string, slot int) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	q := u.Query()
	q.Set("gen", gen)
	q.Set("conn", strconv.Itoa(slot))
	u.RawQuery = q.Encode()
	return u.String()
}

func parsePoolMaxConns(resp *http.Response) int {
	if resp == nil {
		return 1
	}
	raw := strings.TrimSpace(resp.Header.Get(HeaderPoolMaxConns))
	if raw == "" {
		return 1
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 1
	}
	if n > maxConnsPerNode {
		return maxConnsPerNode
	}
	return n
}

func (p *PoolProvider) dialSlot(ctx context.Context, node NodeConfig, gen string, slot int) (*wsNetConn, *yamux.Session, int, error) {
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+node.Token)
	hdr.Set("User-Agent", "Multigravity-RelayPool/1.1")

	targetEndpoint := appendTunnelSlotQuery(node.Endpoint, gen, slot)

	// Try direct connection first (bypassing local dev proxies like Clash/V2Ray so the
	// tunnel follows the physical low-latency route and survives proxy restarts), falling
	// back to ProxyFromEnvironment only when direct dial fails and an env proxy is set.
	directDialer := websocket.Dialer{
		Proxy:             nil,
		HandshakeTimeout:  10 * time.Second,
		ReadBufferSize:    64 << 10,
		WriteBufferSize:   64 << 10,
		EnableCompression: false,
	}
	ws, resp, err := directDialer.DialContext(ctx, targetEndpoint, hdr)
	if err != nil && ctx.Err() == nil && hasEnvProxyForURL(node.Endpoint) {
		if resp != nil {
			_ = resp.Body.Close()
		}
		proxyDialer := websocket.Dialer{
			Proxy:             http.ProxyFromEnvironment,
			HandshakeTimeout:  15 * time.Second,
			ReadBufferSize:    64 << 10,
			WriteBufferSize:   64 << 10,
			EnableCompression: false,
		}
		ws, resp, err = proxyDialer.DialContext(ctx, targetEndpoint, hdr)
	}
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			return nil, nil, 1, fmt.Errorf("dial %s:slot%d (HTTP %d): %w (%s)", node.ID, slot, resp.StatusCode, err, strings.TrimSpace(string(body)))
		}
		return nil, nil, 1, fmt.Errorf("dial %s:slot%d: %w", node.ID, slot, err)
	}
	serverMax := parsePoolMaxConns(resp)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	wsc := newWSNetConn(ws)
	sess, err := yamux.Client(wsc, defaultYamuxConfig())
	if err != nil {
		_ = wsc.Close()
		return nil, nil, serverMax, fmt.Errorf("yamux init: %w", err)
	}
	return wsc, sess, serverMax, nil
}

func (p *PoolProvider) serveSession(ctx context.Context, wsc *wsNetConn, sess *yamux.Session) error {
	p.addSession(sess)
	doneCh := make(chan struct{})
	defer func() {
		close(doneCh)
		_ = sess.Close()
		_ = wsc.Close()
		p.removeSession(sess)
	}()

	go func() {
		select {
		case <-ctx.Done():
			_ = sess.Close()
		case <-doneCh:
		}
	}()

	for {
		stream, err := sess.AcceptStream()
		if err != nil {
			return err
		}
		if p.cfg.Listener != nil {
			p.cfg.Listener.Enqueue(stream)
		} else {
			go pipeStreamToTCP(ctx, stream, p.cfg.LocalAddr)
		}
	}
}

// runNodeEpoch establishes slot 0 to probe server multi-connection support (X-Mgy-Relay-Pool-Max).
// When supported (>=2), it runs up to connsPerNode parallel yamux tunnels under a shared epoch so
// individual TCP drops heal silently with zero user-visible downtime and zero Head-of-Line blocking.
func (p *PoolProvider) runNodeEpoch(parentCtx context.Context, node NodeConfig) (bool, error) {
	epochCtx, epochCancel := context.WithCancel(parentCtx)
	p.mu.Lock()
	p.epochCancel = epochCancel
	p.mu.Unlock()
	defer func() {
		epochCancel()
		p.mu.Lock()
		if p.epochCancel != nil {
			p.epochCancel = nil
		}
		p.mu.Unlock()
	}()

	gen := generateEpochGen()
	wsc0, sess0, serverMax, err := p.dialSlot(epochCtx, node, gen, 0)
	if err != nil {
		return false, err
	}

	targetConns := p.connsPerNode
	if serverMax < targetConns {
		targetConns = serverMax
	}
	if targetConns < 1 {
		targetConns = 1
	}

	if targetConns == 1 {
		p.logger.Info("✅ [RelayPool] E2EE 中继隧道就绪", "node", node.ID, "conns", 1, "url", node.URL)
		return false, p.serveSession(epochCtx, wsc0, sess0)
	}

	p.logger.Info("✅ [RelayPool] E2EE 并发中继隧道池就绪", "node", node.ID, "conns", targetConns, "url", node.URL)

	var (
		allDownFailures atomic.Int32
		outageTriggered atomic.Bool
		lastErrMu       sync.Mutex
		lastErr         error
		wg              sync.WaitGroup
	)
	recordErr := func(e error) {
		if e == nil {
			return
		}
		lastErrMu.Lock()
		lastErr = e
		lastErrMu.Unlock()
	}

	runSlot := func(slot int, initWSC *wsNetConn, initSess *yamux.Session) {
		defer wg.Done()
		if slot > 0 {
			stagger := time.NewTimer(time.Duration(slot) * 80 * time.Millisecond)
			select {
			case <-epochCtx.Done():
				stagger.Stop()
				return
			case <-stagger.C:
			}
		}

		backoff := 0
		wsc := initWSC
		sess := initSess

		for {
			if epochCtx.Err() != nil {
				return
			}
			if sess == nil {
				var dialErr error
				wsc, sess, _, dialErr = p.dialSlot(epochCtx, node, gen, slot)
				if dialErr != nil {
					if epochCtx.Err() != nil {
						return
					}
					recordErr(dialErr)
					backoff++
					if p.activeSessionCount() == 0 {
						if allDownFailures.Add(1) >= 3 {
							outageTriggered.Store(true)
							epochCancel()
							return
						}
					}
					delay := nextClientBackoff(backoff)
					timer := time.NewTimer(delay)
					select {
					case <-epochCtx.Done():
						timer.Stop()
						return
					case <-timer.C:
					}
					continue
				}
			}

			// Connected cleanly
			allDownFailures.Store(0)
			connectedAt := time.Now()
			serveErr := p.serveSession(epochCtx, wsc, sess)
			wsc = nil
			sess = nil
			if epochCtx.Err() != nil {
				return
			}
			recordErr(serveErr)
			if time.Since(connectedAt) >= 30*time.Second {
				backoff = 0
			} else {
				backoff++
			}
			// Brief jitter before reconnecting this slot while sibling slots keep serving traffic
			delay := nextClientBackoff(backoff)
			if delay > 5*time.Second {
				delay = 5 * time.Second
			}
			timer := time.NewTimer(delay)
			select {
			case <-epochCtx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}

	wg.Add(targetConns)
	go runSlot(0, wsc0, sess0)
	for slot := 1; slot < targetConns; slot++ {
		go runSlot(slot, nil, nil)
	}
	wg.Wait()

	lastErrMu.Lock()
	retErr := lastErr
	lastErrMu.Unlock()
	return outageTriggered.Load(), retErr
}

func pipeStreamToTCP(ctx context.Context, stream *yamux.Stream, localAddr string) {
	defer stream.Close()
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	localConn, err := dialer.DialContext(ctx, "tcp", localAddr)
	if err != nil {
		return
	}
	defer localConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(localConn, stream)
		if tc, ok := localConn.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(stream, localConn)
		_ = stream.Close()
	}()
	wg.Wait()
}
