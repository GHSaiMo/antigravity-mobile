package relay

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
)

// ClientConfig configures the embedded mgy reverse-tunnel client.
type ClientConfig struct {
	RelayEndpoint string // e.g. "wss://relay.jiuge.space/_mgy_tunnel"
	Token         string // signed mgyr1 token
	LocalAddr     string // local gateway address, e.g. "127.0.0.1:58900"
	PublicURL     string
	Subdomain     string
	Logger        *slog.Logger
	OnConnected   func()
}

// Client maintains a persistent multiplexed WSS reverse tunnel to mgy-relay
// and forwards incoming streams to the local mgy HTTP/WebSocket gateway.
type Client struct {
	cfg    ClientConfig
	logger *slog.Logger

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
	sess    *yamux.Session
}

// NewClient constructs a managed relay client.
func NewClient(cfg ClientConfig) (*Client, error) {
	cfg.RelayEndpoint = strings.TrimSpace(cfg.RelayEndpoint)
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.LocalAddr = strings.TrimSpace(cfg.LocalAddr)
	if cfg.RelayEndpoint == "" {
		return nil, fmt.Errorf("missing relay endpoint")
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("missing relay token")
	}
	if cfg.LocalAddr == "" {
		cfg.LocalAddr = "127.0.0.1:58900"
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		cfg:    cfg,
		logger: logger,
	}, nil
}

// Start launches the supervised background relay connection loop.
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	c.done = make(chan struct{})
	c.running = true
	go c.runLoop(runCtx)
	return nil
}

// Stop terminates the active session and stops reconnection.
func (c *Client) Stop() {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return
	}
	cancel := c.cancel
	sess := c.sess
	done := c.done
	c.running = false
	c.mu.Unlock()

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

func (c *Client) PublicURL() string { return c.cfg.PublicURL }
func (c *Client) Subdomain() string { return c.cfg.Subdomain }
func (c *Client) Mode() string      { return "relay_v1" }

func (c *Client) runLoop(ctx context.Context) {
	defer close(c.done)

	failures := 0
	for {
		if ctx.Err() != nil {
			return
		}
		startedAt := time.Now()
		err := c.connectAndServe(ctx)
		if ctx.Err() != nil {
			return
		}
		if time.Since(startedAt) >= 30*time.Second {
			failures = 0
		}
		delay := nextClientBackoff(failures)
		failures++
		c.logger.Warn("⚠️  [Relay] 中继连接断开，准备自动重连", "err", err, "retry_in", delay.Round(100*time.Millisecond))

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (c *Client) connectAndServe(ctx context.Context) error {
	dialer := websocket.Dialer{
		Proxy:             http.ProxyFromEnvironment,
		HandshakeTimeout:  15 * time.Second,
		ReadBufferSize:    32 << 10,
		WriteBufferSize:   32 << 10,
		EnableCompression: false,
	}
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+c.cfg.Token)
	hdr.Set("User-Agent", "Multigravity-RelayClient/1.0")

	ws, resp, err := dialer.DialContext(ctx, c.cfg.RelayEndpoint, hdr)
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			return fmt.Errorf("dial relay (HTTP %d): %w (%s)", resp.StatusCode, err, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("dial relay: %w", err)
	}

	wsc := newWSNetConn(ws)
	sess, err := yamux.Client(wsc, defaultYamuxConfig())
	if err != nil {
		_ = wsc.Close()
		return fmt.Errorf("init yamux client: %w", err)
	}

	c.mu.Lock()
	c.sess = sess
	c.mu.Unlock()
	defer func() {
		_ = sess.Close()
		_ = wsc.Close()
		c.mu.Lock()
		if c.sess == sess {
			c.sess = nil
		}
		c.mu.Unlock()
	}()

	go func() {
		<-ctx.Done()
		_ = sess.Close()
	}()

	c.logger.Info("✅ [Relay] 专属中继隧道连接就绪", "url", c.cfg.PublicURL)
	if c.cfg.OnConnected != nil {
		c.cfg.OnConnected()
	}

	for {
		stream, err := sess.AcceptStream()
		if err != nil {
			return err
		}
		go c.pipeToLocal(ctx, stream)
	}
}

func (c *Client) pipeToLocal(ctx context.Context, stream *yamux.Stream) {
	defer stream.Close()

	dialer := &net.Dialer{Timeout: 5 * time.Second}
	localConn, err := dialer.DialContext(ctx, "tcp", c.cfg.LocalAddr)
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

func nextClientBackoff(failures int) time.Duration {
	if failures < 0 {
		failures = 0
	}
	if failures > 6 {
		failures = 6
	}
	base := 500 * time.Millisecond << failures
	if base > 30*time.Second {
		base = 30 * time.Second
	}
	var jb [1]byte
	if _, err := rand.Read(jb[:]); err == nil {
		delta := (int64(jb[0]) - 128) * int64(base) / 640
		base = time.Duration(int64(base) + delta)
	}
	if base < 300*time.Millisecond {
		base = 300 * time.Millisecond
	}
	return base
}
