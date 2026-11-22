package relay

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
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
	RefreshInterval   time.Duration   // default 6h
	RefreshFunc       func(ctx context.Context) ([]NodeConfig, error)
	OnEndpointChanged func(node NodeConfig)
	OnAllFailed       func()
	ProbeClient       *http.Client
	Logger            *slog.Logger
}

// PoolProvider implements tunnel.Provider across a dynamic pool of mgy-relay VPS nodes.
type PoolProvider struct {
	cfg    PoolConfig
	logger *slog.Logger

	mu         sync.RWMutex
	nodes      []NodeConfig
	activeNode NodeConfig
	activeRTT  time.Duration
	running    bool
	cancel     context.CancelFunc
	done       chan struct{}
	sess       *yamux.Session
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
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &PoolProvider{
		cfg:        cfg,
		logger:     logger,
		nodes:      valid,
		activeNode: valid[0],
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

// Stop terminates the active relay tunnel and halts background supervision.
func (p *PoolProvider) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	cancel := p.cancel
	sess := p.sess
	done := p.done
	p.running = false
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if sess != nil {
		_ = sess.Close()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
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

	probeCtx, cancel := context.WithTimeout(ctx, 3500*time.Millisecond)
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
	client := p.cfg.ProbeClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
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
		err := p.connectAndServe(ctx, node)
		if ctx.Err() != nil {
			return
		}

		if time.Since(startedAt) >= 30*time.Second {
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
			sess := p.sess
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
				if sess != nil {
					_ = sess.Close()
				}
				continue
			}

			// Hysteresis: only migrate if new node is >40% faster AND at least 50ms lower RTT, and no active streams
			if bestNode.ID != curNode.ID && bestRTT > 0 {
				curRTT, curOK := p.probeSingle(ctx, curNode)
				noActiveStreams := sess == nil || sess.NumStreams() == 0
				if noActiveStreams && (!curOK || (curRTT-bestRTT >= 50*time.Millisecond && float64(bestRTT) < float64(curRTT)*0.6)) {
					p.logger.Info("🚀 [RelayPool] 发现更优节点，空闲时无感切换",
						"from", curNode.ID, "from_ms", curRTT.Milliseconds(),
						"to", bestNode.ID, "to_ms", bestRTT.Milliseconds())
					p.setActiveNode(bestNode, bestRTT)
					if sess != nil {
						_ = sess.Close()
					}
				}
			}
		}
	}
}

func (p *PoolProvider) connectAndServe(ctx context.Context, node NodeConfig) error {
	dialer := websocket.Dialer{
		Proxy:             http.ProxyFromEnvironment,
		HandshakeTimeout:  15 * time.Second,
		ReadBufferSize:    32 << 10,
		WriteBufferSize:   32 << 10,
		EnableCompression: false,
	}
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+node.Token)
	hdr.Set("User-Agent", "Multigravity-RelayPool/1.0")

	ws, resp, err := dialer.DialContext(ctx, node.Endpoint, hdr)
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			return fmt.Errorf("dial %s (HTTP %d): %w (%s)", node.ID, resp.StatusCode, err, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("dial %s: %w", node.ID, err)
	}

	wsc := newWSNetConn(ws)
	sess, err := yamux.Client(wsc, defaultYamuxConfig())
	if err != nil {
		_ = wsc.Close()
		return fmt.Errorf("yamux init: %w", err)
	}

	p.mu.Lock()
	p.sess = sess
	p.mu.Unlock()
	defer func() {
		_ = sess.Close()
		_ = wsc.Close()
		p.mu.Lock()
		if p.sess == sess {
			p.sess = nil
		}
		p.mu.Unlock()
	}()

	go func() {
		<-ctx.Done()
		_ = sess.Close()
	}()

	p.logger.Info("✅ [RelayPool] E2EE 中继隧道就绪", "node", node.ID, "url", node.URL)

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
