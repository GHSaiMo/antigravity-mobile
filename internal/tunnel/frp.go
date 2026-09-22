package tunnel

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fatedier/frp/client"
	"github.com/fatedier/frp/pkg/config"
	"github.com/fatedier/frp/pkg/config/source"
)

// Config defines the parameters for the embedded FRP tunnel client.
type Config struct {
	Enabled    bool
	ServerAddr string
	ServerPort int
	Token      string
	LocalIP    string
	LocalPort  int
	RemotePort int
	ProxyName            string
	TLSEnable            bool
	ProxyProtocolVersion string // e.g. "v2"
}

// Tunnel manages an embedded FRP client connecting to a remote FRP server.
type Tunnel struct {
	cfg        Config
	cancelFunc context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.Mutex
	running    bool
}

// New creates a new embedded FRP Tunnel.
func New(cfg Config) *Tunnel {
	if cfg.LocalIP == "" {
		cfg.LocalIP = "127.0.0.1"
	}
	if cfg.ServerPort <= 0 {
		cfg.ServerPort = 7000
	}
	if cfg.RemotePort <= 0 {
		cfg.RemotePort = cfg.LocalPort
	}
	if cfg.ProxyName == "" {
		cfg.ProxyName = fmt.Sprintf("antigravity-%d", cfg.RemotePort)
	}
	return &Tunnel{
		cfg: cfg,
	}
}

// Start launches the FRP client in a background goroutine with automatic reconnection.
func (t *Tunnel) Start(ctx context.Context) {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	if strings.TrimSpace(t.cfg.Token) == "" {
		t.mu.Unlock()
		log.Printf("❌ [Tunnel] FRP_TOKEN is required; refusing to start tunnel")
		return
	}
	t.running = true
	tunnelCtx, cancel := context.WithCancel(ctx)
	t.cancelFunc = cancel
	t.mu.Unlock()

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		log.Printf("☁️ [Tunnel] Starting embedded FRP tunnel to %s:%d (remote port %d)...",
			t.cfg.ServerAddr, t.cfg.ServerPort, t.cfg.RemotePort)

		for {
			select {
			case <-tunnelCtx.Done():
				log.Printf("☁️ [Tunnel] Tunnel stopped.")
				return
			default:
			}

			err := t.runSession(tunnelCtx)
			if err != nil {
				if tunnelCtx.Err() != nil {
					return
				}
				log.Printf("⚠️ [Tunnel] FRP tunnel session closed: %v. Reconnecting in 5s...", err)
			}

			select {
			case <-tunnelCtx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
}

// Stop stops the running tunnel and waits for the goroutine to finish.
func (t *Tunnel) Stop() {
	t.mu.Lock()
	if !t.running {
		t.mu.Unlock()
		return
	}
	t.running = false
	if t.cancelFunc != nil {
		t.cancelFunc()
	}
	t.mu.Unlock()

	t.wg.Wait()
}

// runSession generates a temporary TOML configuration and executes an FRP client service session.
func (t *Tunnel) runSession(ctx context.Context) error {
	cfgDir := os.TempDir()
	if home, err := os.UserHomeDir(); err == nil {
		cfgDir = filepath.Join(home, ".antigravity-mobile")
		_ = os.MkdirAll(cfgDir, 0700)
	}
	tmpFile, err := os.CreateTemp(cfgDir, "frpc-*.toml")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpPath := tmpFile.Name()
	_ = os.Chmod(tmpPath, 0600)
	defer os.Remove(tmpPath)

	tomlConfig := t.BuildConfigTOML()

	if _, err := tmpFile.WriteString(tomlConfig); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	tmpFile.Close()

	common, proxyCfgs, visitorCfgs, _, err := config.LoadClientConfig(tmpPath, true)
	// Purge temporary config file containing token from disk immediately after loading into memory
	_ = os.Remove(tmpPath)
	if err != nil {
		return fmt.Errorf("load client config: %w", err)
	}

	// SEC-AUDIT M-3: FRP v0.70+ API migration — use ConfigSource + Aggregator
	cfgSource := source.NewConfigSource()
	if err := cfgSource.ReplaceAll(proxyCfgs, visitorCfgs); err != nil {
		return fmt.Errorf("configure frp proxies: %w", err)
	}
	agg := source.NewAggregator(cfgSource)

	svr, err := client.NewService(client.ServiceOptions{
		Common:                 common,
		ConfigSourceAggregator: agg,
	})
	if err != nil {
		return fmt.Errorf("initialize frp service: %w", err)
	}

	// Close service if context is cancelled, preventing goroutine leak when session ends
	sessionDone := make(chan struct{})
	defer close(sessionDone)

	go func() {
		select {
		case <-ctx.Done():
			svr.Close()
		case <-sessionDone:
		}
	}()

	log.Printf("☁️ [Tunnel] Connecting to FRP relay %s:%d -> local 127.0.0.1:%d (TLS=%t)",
		t.cfg.ServerAddr, t.cfg.RemotePort, t.cfg.LocalPort, t.cfg.TLSEnable)

	return svr.Run(ctx)
}

// RemoteURL returns the public HTTP URL for accessing this tunnel if configured.
func (t *Tunnel) RemoteURL(ssl bool) string {
	return t.RemoteURLFor("", ssl)
}

// RemoteURLFor is RemoteURL with an optional public hostname (e.g. DDNS).
// When publicHost is empty, FRP_SERVER_ADDR is used.
func (t *Tunnel) RemoteURLFor(publicHost string, ssl bool) string {
	if t.cfg.RemotePort <= 0 {
		return ""
	}
	addr := strings.TrimSpace(publicHost)
	if addr == "" {
		addr = t.cfg.ServerAddr
	}
	if addr == "" {
		return ""
	}
	scheme := "http://"
	if ssl {
		scheme = "https://"
	}
	if strings.Contains(addr, ":") && !strings.HasPrefix(addr, "[") {
		addr = "[" + addr + "]"
	}
	return fmt.Sprintf("%s%s:%d", scheme, addr, t.cfg.RemotePort)
}

// BuildConfigTOML generates the TOML configuration for frpc.
func (t *Tunnel) BuildConfigTOML() string {
	var tokenLine string
	if t.cfg.Token != "" {
		tokenLine = fmt.Sprintf("auth.token = %q\n", t.cfg.Token)
	}

	var tlsLine string
	if t.cfg.TLSEnable {
		tlsLine = "transport.tls.enable = true\n"
	}

	var ppLine string
	if t.cfg.ProxyProtocolVersion != "" {
		ppLine = fmt.Sprintf("transport.proxyProtocolVersion = %q\n", t.cfg.ProxyProtocolVersion)
	}

	return fmt.Sprintf(`serverAddr = %q
serverPort = %d
%s%s
[[proxies]]
name = %q
type = "tcp"
localIP = %q
localPort = %d
remotePort = %d
%s`, t.cfg.ServerAddr, t.cfg.ServerPort, tokenLine, tlsLine, t.cfg.ProxyName, t.cfg.LocalIP, t.cfg.LocalPort, t.cfg.RemotePort, ppLine)
}

