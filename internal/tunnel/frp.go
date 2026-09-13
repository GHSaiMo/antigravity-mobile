package tunnel

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fatedier/frp/client"
	"github.com/fatedier/frp/pkg/config"
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
	ProxyName  string
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
	tmpFile, err := os.CreateTemp("", "agy-frpc-*.toml")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	var tokenLine string
	if t.cfg.Token != "" {
		tokenLine = fmt.Sprintf("auth.token = %q\n", t.cfg.Token)
	}

	tomlConfig := fmt.Sprintf(`serverAddr = %q
serverPort = %d
%s
[[proxies]]
name = %q
type = "tcp"
localIP = %q
localPort = %d
remotePort = %d
`, t.cfg.ServerAddr, t.cfg.ServerPort, tokenLine, t.cfg.ProxyName, t.cfg.LocalIP, t.cfg.LocalPort, t.cfg.RemotePort)

	if _, err := tmpFile.WriteString(tomlConfig); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	tmpFile.Close()

	common, proxyCfgs, visitorCfgs, _, err := config.LoadClientConfig(tmpPath, true)
	if err != nil {
		return fmt.Errorf("load client config: %w", err)
	}

	svr, err := client.NewService(client.ServiceOptions{
		Common:      common,
		ProxyCfgs:   proxyCfgs,
		VisitorCfgs: visitorCfgs,
	})
	if err != nil {
		return fmt.Errorf("initialize frp service: %w", err)
	}

	// Close service if context is cancelled
	go func() {
		<-ctx.Done()
		svr.Close()
	}()

	log.Printf("☁️ [Tunnel] Connecting to FRP relay %s:%d -> local 127.0.0.1:%d",
		t.cfg.ServerAddr, t.cfg.RemotePort, t.cfg.LocalPort)

	return svr.Run(ctx)
}

// RemoteURL returns the public HTTP URL for accessing this tunnel if configured.
func (t *Tunnel) RemoteURL(ssl bool) string {
	if t.cfg.ServerAddr == "" || t.cfg.RemotePort <= 0 {
		return ""
	}
	scheme := "http://"
	if ssl {
		scheme = "https://"
	}
	addr := t.cfg.ServerAddr
	if strings.Contains(addr, ":") && !strings.HasPrefix(addr, "[") {
		addr = "[" + addr + "]"
	}
	return fmt.Sprintf("%s%s:%d", scheme, addr, t.cfg.RemotePort)
}
