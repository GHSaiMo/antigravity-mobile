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
	defaultConnsPerNode          = 3
	maxConnsPerNode              = 4
	defaultRefreshInterval       = 6 * time.Hour
	defaultReprobeInterval       = 45 * time.Minute
	defaultHealthPingInterval    = 2 * time.Minute
	defaultDegradedPingThreshold = 450 * time.Millisecond
	defaultDegradedCooldown      = 10 * time.Minute
	probeSamplesPerNode          = 3
	probeAttemptTimeout          = 2000 * time.Millisecond
	probeSingleLossPenalty       = 100 * time.Millisecond
	probeSevereLossPenalty       = 1000 * time.Millisecond
	probeColdSpikeCutoff         = 900 * time.Millisecond
	probeColdSpikeMaxPenalty     = 200 * time.Millisecond
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
// loss-aware scoring, hysteresis pinning, automatic failover, periodic local reprobe,
// runtime ping degradation escape, and periodic pool refresh.
type PoolConfig struct {
	Nodes                 []NodeConfig
	Listener              *StreamListener // preferred: direct in-process E2EE-guarded HTTP listener
	LocalAddr             string          // fallback if Listener is nil
	ConnsPerNode          int             // parallel yamux connections per node (default 3, max 4; negotiated with server)
	RefreshInterval       time.Duration   // Worker pool/token refresh interval (default 6h)
	ReprobeInterval       time.Duration   // local RTT reprobe interval across cached nodes (default 45m)
	HealthPingInterval    time.Duration   // runtime yamux ping interval to detect congestion (default 2m)
	DegradedPingThreshold time.Duration   // yamux ping RTT treated as degraded (default 450ms)
	RefreshFunc           func(ctx context.Context) ([]NodeConfig, error)
	OnEndpointChanged     func(node NodeConfig)
	OnConnected           func(node NodeConfig)
	OnAllFailed           func()
	ProbeClient           *http.Client
	Logger                *slog.Logger
}

// PoolProvider implements tunnel.Provider across a dynamic pool of mgy-relay VPS nodes.
type PoolProvider struct {
	cfg                   PoolConfig
	logger                *slog.Logger
	connsPerNode          int
	reprobeInterval       time.Duration
	healthPingInterval    time.Duration
	degradedPingThreshold time.Duration

	mu          sync.RWMutex
	nodes       []NodeConfig
	activeNode  NodeConfig
	activeRTT   time.Duration
	running     bool
	cancel      context.CancelFunc
	epochCancel context.CancelFunc
	done        chan struct{}
	degradeCh   chan struct{}
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
		cfg.RefreshInterval = defaultRefreshInterval
	}
	reprobeInterval := cfg.ReprobeInterval
	if reprobeInterval <= 0 {
		reprobeInterval = defaultReprobeInterval
	}
	healthPingInterval := cfg.HealthPingInterval
	if healthPingInterval <= 0 {
		healthPingInterval = defaultHealthPingInterval
	}
	degradedPingThreshold := cfg.DegradedPingThreshold
	if degradedPingThreshold <= 0 {
		degradedPingThreshold = defaultDegradedPingThreshold
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
		cfg:                   cfg,
		logger:                logger,
		connsPerNode:          connsPerNode,
		reprobeInterval:       reprobeInterval,
		healthPingInterval:    healthPingInterval,
		degradedPingThreshold: degradedPingThreshold,
		nodes:                 valid,
		activeNode:            valid[0],
		degradeCh:             make(chan struct{}, 1),
		sessions:              make(map[*yamux.Session]struct{}),
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
	go p.reprobeLoop(runCtx)
	go p.healthWatchdogLoop(runCtx)
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
	node         NodeConfig
	rtt          time.Duration // loss- and jitter-adjusted effective score for ranking
	medianRTT    time.Duration // raw median of successful probes
	successes    int
	attempts     int
	ok           bool
	idx          int
}

func (r nodeProbeResult) hadLoss() bool {
	return r.attempts > 0 && r.successes < r.attempts
}

func (r nodeProbeResult) severeLoss() bool {
	return r.attempts >= 2 && r.successes <= 1
}

func (p *PoolProvider) selectFastestNode(ctx context.Context, nodes []NodeConfig) (NodeConfig, time.Duration) {
	results := p.probeAllNodes(ctx, nodes)
	if len(results) == 0 {
		return nodes[0], 0
	}
	best := results[0]
	if best.ok {
		p.logger.Info("🌐 [RelayPool] 测速优选完成",
			"node", best.node.ID,
			"region", best.node.Region,
			"rtt_ms", best.rtt.Milliseconds(),
			"med_ms", best.medianRTT.Milliseconds(),
			"ok", fmt.Sprintf("%d/%d", best.successes, best.attempts))
		return best.node, best.rtt
	}
	return nodes[0], 0
}

func (p *PoolProvider) probeAllNodes(ctx context.Context, nodes []NodeConfig) []nodeProbeResult {
	if len(nodes) == 0 {
		return nil
	}
	if len(nodes) == 1 {
		res := p.probeNode(ctx, nodes[0], 0)
		return []nodeProbeResult{res}
	}

	probeCtx, cancel := context.WithTimeout(ctx, 5500*time.Millisecond)
	defer cancel()

	resCh := make(chan nodeProbeResult, len(nodes))
	for i, n := range nodes {
		go func(idx int, node NodeConfig) {
			resCh <- p.probeNode(probeCtx, node, idx)
		}(i, n)
	}

	results := make([]nodeProbeResult, 0, len(nodes))
	for range nodes {
		results = append(results, <-resCh)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].ok != results[j].ok {
			return results[i].ok
		}
		if results[i].ok && results[j].ok {
			// Severely lossy nodes (<=1/3 probes succeeded) always rank behind healthy peers.
			if results[i].severeLoss() != results[j].severeLoss() {
				return !results[i].severeLoss()
			}
			// Within 15ms tie, respect Dispatcher's region/priority order.
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
	return results
}

func (p *PoolProvider) probeSingle(ctx context.Context, node NodeConfig) (time.Duration, bool) {
	res := p.probeNode(ctx, node, 0)
	return res.rtt, res.ok
}

// probeNode executes up to 3 lightweight probes (1 cold + 2 keep-alive warm probes) to expose
// both cold-start SYN/TLS packet loss and evening-peak retransmit jitter, computing a
// loss- and jitter-adjusted effective RTT score.
func (p *PoolProvider) probeNode(ctx context.Context, node NodeConfig, idx int) nodeProbeResult {
	res := nodeProbeResult{node: node, idx: idx}
	healthURL := strings.TrimSpace(node.HealthURL)
	if healthURL == "" {
		return res
	}

	// Prefer direct (Proxy: nil) probes so a developer's local Clash/VPN proxy
	// (often routed via US/EU nodes) does not distort physical RTT selection.
	directRes := p.probeSeries(ctx, healthURL, false)
	directRes.node = node
	directRes.idx = idx
	if directRes.ok || p.cfg.ProbeClient != nil || ctx.Err() != nil {
		return directRes
	}

	// Corporate-proxy fallback: if direct outbound 443 is blocked and env proxy is set, try proxy once.
	if hasEnvProxyForURL(healthURL) {
		proxyRes := p.probeSeries(ctx, healthURL, true)
		proxyRes.node = node
		proxyRes.idx = idx
		if proxyRes.ok {
			return proxyRes
		}
	}
	return directRes
}

func (p *PoolProvider) probeSeries(ctx context.Context, healthURL string, useProxy bool) nodeProbeResult {
	client := p.cfg.ProbeClient
	var ownedTransport *http.Transport
	if client == nil {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		// Keep connections alive across the 3 samples of this node so sample 0 tests
		// cold TCP+TLS connect while samples 1-2 measure warm RTT and retransmit jitter.
		tr.DisableKeepAlives = false
		tr.MaxIdleConnsPerHost = 2
		if useProxy {
			tr.Proxy = http.ProxyFromEnvironment
		} else {
			tr.Proxy = nil
		}
		ownedTransport = tr
		client = &http.Client{Transport: tr}
		defer ownedTransport.CloseIdleConnections()
	}

	var (
		rtts             []time.Duration
		coldRTT          time.Duration
		attempts         int
		consecutiveFails int
	)

	for i := 0; i < probeSamplesPerNode; i++ {
		if ctx.Err() != nil {
			break
		}
		attempts++
		attemptCtx, cancel := context.WithTimeout(ctx, probeAttemptTimeout)
		rtt, ok := p.probeOnceWithClient(attemptCtx, client, healthURL)
		cancel()
		if ok {
			consecutiveFails = 0
			if len(rtts) == 0 {
				coldRTT = rtt
			}
			rtts = append(rtts, rtt)
		} else {
			consecutiveFails++
			// If 2 consecutive attempts failed and already consumed >=1.5s, stop early
			// so dead/heavily-congested nodes don't stall startup probing.
			if consecutiveFails >= 2 && rtt >= 1500*time.Millisecond {
				break
			}
		}
	}

	res := nodeProbeResult{
		attempts:  attempts,
		successes: len(rtts),
		ok:        len(rtts) > 0,
	}
	if !res.ok {
		return res
	}

	res.medianRTT = medianDuration(rtts)
	res.rtt = scoreProbeSamples(rtts, coldRTT, attempts)
	return res
}

// scoreProbeSamples turns raw probe RTTs into a loss- and jitter-aware score:
//   - 3/3 ok: uses warm samples (rtts[1:]) weighted 25% min + 75% max so retransmit spikes
//     (e.g. 49ms -> 302ms) are penalized instead of hidden by min(), plus a mild cold-connect
//     penalty if TCP+TLS handshake exceeded 900ms (SYN retransmit).
//   - 2/3 ok (1 transient failure): adds +100ms loss penalty on top of the post-recovery RTT.
//   - 1/3 ok (>=50% loss): adds +1000ms severe-loss penalty (and ranked behind healthy peers).
func scoreProbeSamples(rtts []time.Duration, coldRTT time.Duration, attempts int) time.Duration {
	if len(rtts) == 0 {
		return 0
	}
	var score time.Duration
	if len(rtts) >= 3 {
		warmMin, warmMax := rtts[1], rtts[1]
		for _, d := range rtts[2:] {
			if d < warmMin {
				warmMin = d
			}
			if d > warmMax {
				warmMax = d
			}
		}
		score = (warmMin + 3*warmMax) / 4
		if coldRTT > probeColdSpikeCutoff {
			extra := (coldRTT - probeColdSpikeCutoff) / 4
			if extra > probeColdSpikeMaxPenalty {
				extra = probeColdSpikeMaxPenalty
			}
			score += extra
		}
	} else if len(rtts) == 2 {
		// rtts[0] was cold (after 1st failed), rtts[1] was warm.
		// Penalize if warm jumped above cold or still spiky.
		warm := rtts[1]
		if warm > rtts[0] {
			score = (rtts[0] + 3*warm) / 4
		} else {
			score = warm
		}
		if rtts[0] > probeColdSpikeCutoff {
			extra := (rtts[0] - probeColdSpikeCutoff) / 4
			if extra > probeColdSpikeMaxPenalty {
				extra = probeColdSpikeMaxPenalty
			}
			score += extra
		}
	} else {
		score = rtts[0]
	}

	failures := attempts - len(rtts)
	if failures >= 2 || (attempts >= 2 && len(rtts) == 1) {
		score += probeSevereLossPenalty
	} else if failures == 1 {
		score += probeSingleLossPenalty
	}
	if score <= 0 {
		score = time.Millisecond
	}
	return score
}

func medianDuration(vals []time.Duration) time.Duration {
	if len(vals) == 0 {
		return 0
	}
	cp := append([]time.Duration(nil), vals...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	mid := len(cp) / 2
	if len(cp)%2 == 1 {
		return cp[mid]
	}
	return (cp[mid-1] + cp[mid]) / 2
}

func (p *PoolProvider) probeOnceWithClient(ctx context.Context, client *http.Client, healthURL string) (time.Duration, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return 0, false
	}
	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return elapsed, false
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return elapsed, false
	}
	return elapsed, true
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

func shouldMigrateIdle(cur, best nodeProbeResult, degraded bool) bool {
	if !best.ok || best.node.ID == cur.node.ID {
		return false
	}
	if !cur.ok {
		return true
	}
	// Always escape a lossy current node when a zero-loss faster alternative exists.
	if cur.hadLoss() && !best.hadLoss() && best.rtt < cur.rtt {
		return true
	}
	// If runtime ping watchdog flagged sustained congestion, use a lower escape threshold.
	if degraded {
		return cur.rtt-best.rtt >= 30*time.Millisecond && float64(best.rtt) < float64(cur.rtt)*0.75
	}
	// Normal hysteresis: require >=50ms improvement AND >40% faster to avoid route flapping.
	return cur.rtt-best.rtt >= 50*time.Millisecond && float64(best.rtt) < float64(cur.rtt)*0.6
}

func (p *PoolProvider) evaluateAndMigrateIdle(ctx context.Context, nodes []NodeConfig, curNode NodeConfig, reason string, degraded bool) {
	if len(nodes) <= 1 || p.totalNumStreams() != 0 {
		return
	}
	results := p.probeAllNodes(ctx, nodes)
	if len(results) == 0 || !results[0].ok {
		return
	}
	best := results[0]
	if best.node.ID == curNode.ID {
		return
	}
	var curRes nodeProbeResult
	foundCur := false
	for _, r := range results {
		if r.node.ID == curNode.ID && r.node.Endpoint == curNode.Endpoint {
			curRes = r
			foundCur = true
			break
		}
	}
	if !foundCur {
		curRes = p.probeNode(ctx, curNode, len(results))
	}
	// Re-check stream count right before closing sessions so we never interrupt newly opened streams.
	if p.totalNumStreams() != 0 {
		return
	}
	if shouldMigrateIdle(curRes, best, degraded) {
		p.logger.Info("🚀 [RelayPool] 发现更优节点，空闲时无感切换",
			"reason", reason,
			"from", curNode.ID, "from_ms", curRes.rtt.Milliseconds(), "from_ok", fmt.Sprintf("%d/%d", curRes.successes, curRes.attempts),
			"to", best.node.ID, "to_ms", best.rtt.Milliseconds(), "to_ok", fmt.Sprintf("%d/%d", best.successes, best.attempts))
		p.setActiveNode(best.node, best.rtt)
		p.closeAllSessions()
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

			if !stillPresent {
				bestNode, bestRTT := p.selectFastestNode(ctx, fresh)
				p.logger.Info("🔄 [RelayPool] 当前节点已从池中下线，平滑迁移至新节点", "from", curNode.ID, "to", bestNode.ID)
				p.setActiveNode(bestNode, bestRTT)
				p.closeAllSessions()
				continue
			}

			p.evaluateAndMigrateIdle(ctx, fresh, curNode, "pool_refresh", false)
		}
	}
}

// reprobeLoop periodically re-evaluates cached pool nodes locally (without hitting Worker)
// and also reacts to sustained congestion signals from healthWatchdogLoop so evening-peak
// degradation triggers idle escape in minutes instead of waiting for the 6h Worker refresh.
func (p *PoolProvider) reprobeLoop(ctx context.Context) {
	ticker := time.NewTicker(p.reprobeInterval)
	defer ticker.Stop()
	var lastDegradedProbe time.Time
	for {
		degraded := false
		reason := "periodic_reprobe"
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-p.degradeCh:
			if !lastDegradedProbe.IsZero() && time.Since(lastDegradedProbe) < defaultDegradedCooldown {
				continue
			}
			lastDegradedProbe = time.Now()
			degraded = true
			reason = "runtime_degraded"
		}

		p.mu.RLock()
		nodesSnap := append([]NodeConfig(nil), p.nodes...)
		curNode := p.activeNode
		p.mu.RUnlock()

		p.evaluateAndMigrateIdle(ctx, nodesSnap, curNode, reason, degraded)
	}
}

func (p *PoolProvider) pickLiveSession() *yamux.Session {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for s := range p.sessions {
		if !s.IsClosed() {
			return s
		}
	}
	return nil
}

// healthWatchdogLoop pings one live yamux session periodically; 3 consecutive slow/failed
// pings (>= degradedPingThreshold) signal reprobeLoop to escape evening-peak congestion when idle.
func (p *PoolProvider) healthWatchdogLoop(ctx context.Context) {
	ticker := time.NewTicker(p.healthPingInterval)
	defer ticker.Stop()

	consecutiveBad := 0
	var lastNodeID string
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.mu.RLock()
			nodeCount := len(p.nodes)
			curID := p.activeNode.ID
			p.mu.RUnlock()
			if nodeCount <= 1 {
				consecutiveBad = 0
				continue
			}
			if curID != lastNodeID {
				lastNodeID = curID
				consecutiveBad = 0
			}
			sess := p.pickLiveSession()
			if sess == nil {
				continue
			}
			rtt, err := sess.Ping()
			if ctx.Err() != nil {
				return
			}
			if err != nil || rtt >= p.degradedPingThreshold {
				consecutiveBad++
				if consecutiveBad >= 3 {
					p.logger.Warn("⚠️  [RelayPool] 检测到当前节点持续高延迟/拥堵，触发空闲重选",
						"node", curID, "ping_ms", rtt.Milliseconds(), "err", err)
					consecutiveBad = 0
					select {
					case p.degradeCh <- struct{}{}:
					default:
					}
				}
			} else {
				consecutiveBad = 0
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
		if p.cfg.OnConnected != nil {
			go p.cfg.OnConnected(node)
		}
		return false, p.serveSession(epochCtx, wsc0, sess0)
	}

	p.logger.Info("✅ [RelayPool] E2EE 并发中继隧道池就绪", "node", node.ID, "conns", targetConns, "url", node.URL)
	if p.cfg.OnConnected != nil {
		go p.cfg.OnConnected(node)
	}

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
