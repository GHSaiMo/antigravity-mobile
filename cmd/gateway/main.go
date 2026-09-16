package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"antigravity-mobile/internal/auth"
	"antigravity-mobile/internal/cockpit"
	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/inspector"
	"antigravity-mobile/internal/notifier"
	"antigravity-mobile/internal/proxy"
	"antigravity-mobile/internal/tunnel"
	"antigravity-mobile/web"
)

func main() {
	// 0. Load .env configuration
	config.LoadDotEnv()

	// Default to dual-stack socket binding (default "" binds to all IPv4 and IPv6 interfaces); override with -host flag or GATEWAY_HOST env var
	defaultHost := ""
	if envHost := os.Getenv("GATEWAY_HOST"); envHost != "" {
		defaultHost = envHost
	}
	defaultPort := 58900
	if envPort := os.Getenv("GATEWAY_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil && p > 0 {
			defaultPort = p
		}
	}
	host := flag.String("host", defaultHost, "Host/IP for Mobile Gateway to listen on (default \"\" binds to all IPv4 and IPv6 interfaces)")
	port := flag.Int("port", defaultPort, "Port for Mobile Gateway to listen on")
	pollSec := flag.Int("poll", 5, "Polling interval in seconds for Antigravity instance discovery")
	ddnsHost := flag.String("ddns", os.Getenv("DDNS_HOST"), "Public DDNS domain or IPv6 address for pairing QR code")
	enableSSL := flag.Bool("ssl", os.Getenv("GATEWAY_SSL") == "1" || os.Getenv("GATEWAY_SSL") == "true", "Indicate SSL mode in pairing QR code")
	tlsCert := flag.String("tls-cert", os.Getenv("TLS_CERT_FILE"), "Path to TLS certificate file for HTTPS (optional)")
	tlsKey := flag.String("tls-key", os.Getenv("TLS_KEY_FILE"), "Path to TLS private key file for HTTPS (optional)")
	flag.Parse()

	tunnelCfg := config.GetTunnelConfig()
	tunnelOn := tunnelCfg.Enabled && tunnelCfg.ServerAddr != ""
	if tokPath, generated, err := auth.EnsureAdminToken(tunnelOn); err != nil {
		log.Fatalf("❌ Failed to initialize ADMIN_TOKEN: %v", err)
	} else if tokPath != "" {
		if generated {
			log.Printf("🔐 Generated ADMIN_TOKEN at %s (required because FRP is enabled). `make pair` reads this file.", tokPath)
		} else {
			log.Printf("🔐 Loaded ADMIN_TOKEN from %s", tokPath)
		}
	}
	includePublicIPv6 := config.AdvertisePublicIPv6(*enableSSL)
	if tunnelOn && strings.TrimSpace(*host) == "" && !includePublicIPv6 {
		*host = "127.0.0.1"
		log.Printf("🔒 FRP tunnel enabled with empty GATEWAY_HOST — binding 127.0.0.1 (set GATEWAY_HOST or INCLUDE_PUBLIC_IPV6=1 to keep dual-stack / IPv6 pairing)")
	}
	if includePublicIPv6 && tunnelOn && strings.TrimSpace(*host) == "" {
		log.Printf("📱 INCLUDE_PUBLIC_IPV6=1: keeping dual-stack listen so phones can pair over public IPv6 (FRP still dials 127.0.0.1)")
	}
	hasTLSFiles := *tlsCert != "" && *tlsKey != ""
	if *enableSSL && !hasTLSFiles {
		log.Fatalf("GATEWAY_SSL=1 requires TLS_CERT_FILE and TLS_KEY_FILE (see docs/https_cloud_relay_guide.md)")
	}
	if *enableSSL && strings.TrimSpace(*ddnsHost) == "" {
		log.Printf("⚠️  GATEWAY_SSL=1 without DDNS_HOST: pairing will advertise https:// to an IP and iOS certificate checks will fail. Set DDNS_HOST=agy.jiuge.space")
	}
	if !auth.IsListenAddrLoopback(*host) && !hasTLSFiles {
		log.Printf("⚠️  Gateway listening on a non-loopback address without TLS. LAN HTTP is supported; do not advertise this port on the public Internet. Set TLS_CERT_FILE/TLS_KEY_FILE or GATEWAY_SSL=1 for public access.")
	}

	log.Printf("==================================================")
	log.Printf("🚀 Antigravity starting on %s:%d", *host, *port)
	log.Printf("==================================================")

	// 1. Initialize Inspector
	insp := inspector.NewInspector(time.Duration(*pollSec) * time.Second)
	insp.Start()
	defer insp.Stop()

	// 2. Initialize Reverse Proxy & WebSocket handler
	p := proxy.NewProxy(insp)

	// 3. Initialize Auth Store & Pairing Manager
	authStorePath := os.Getenv("AUTH_STORE_PATH")
	authStore, err := auth.NewAuthStore(authStorePath)
	if err != nil {
		log.Fatalf("❌ Failed to initialize auth store: %v", err)
	}

	netAddrs := auth.DetectNetworkAddresses()
	qrHost := *ddnsHost
	var extraHosts []string
	publicIPv6 := ""
	if includePublicIPv6 {
		publicIPv6 = netAddrs.PublicIPv6
	}

	if qrHost == "" {
		if *host != "" && *host != "0.0.0.0" && *host != "::" && *host != "[::]" {
			qrHost = *host
		} else if netAddrs.LANIPv4 != "" {
			qrHost = netAddrs.LANIPv4
			if publicIPv6 != "" {
				extraHosts = append(extraHosts, publicIPv6)
			}
		} else if publicIPv6 != "" {
			qrHost = publicIPv6
		} else {
			qrHost = "127.0.0.1"
		}
	} else {
		if netAddrs.LANIPv4 != "" && netAddrs.LANIPv4 != qrHost {
			extraHosts = append(extraHosts, netAddrs.LANIPv4)
		}
		if publicIPv6 != "" && publicIPv6 != qrHost {
			extraHosts = append(extraHosts, publicIPv6)
		}
	}

	pairingMgr := auth.NewPairingManager()
	authHandler := auth.NewAuthHandler(authStore, pairingMgr, qrHost, *port, *enableSSL)
	authHandler.SetEndpoints(netAddrs.LANIPv4, publicIPv6, *ddnsHost)
	if publicIPv6 != "" {
		scheme := "http"
		if *enableSSL {
			scheme = "https"
		}
		log.Printf("📱 IPv6 pairing endpoint: %s://[%s]:%d  (phone cellular should reach this address)", scheme, publicIPv6, *port)
	} else if includePublicIPv6 {
		log.Printf("⚠️  INCLUDE_PUBLIC_IPV6 is set but no global unicast IPv6 was found on this Mac")
	}

	// 3.5. Initialize Embedded FRP Cloud Relay Tunnel
	var tun *tunnel.Tunnel
	if tunnelOn && strings.TrimSpace(tunnelCfg.Token) == "" {
		log.Fatalf("FRP_TOKEN is required when the cloud relay tunnel is enabled")
	}

	if tunnelOn {
		tun = tunnel.New(tunnel.Config{
			Enabled:    true,
			ServerAddr: tunnelCfg.ServerAddr,
			ServerPort: tunnelCfg.ServerPort,
			Token:      tunnelCfg.Token,
			LocalPort:  *port,
			RemotePort: tunnelCfg.RemotePort,
			ProxyName:  fmt.Sprintf("antigravity-%d", tunnelCfg.RemotePort),
			TLSEnable:  tunnelCfg.TLSEnable,
		})
		tun.Start(context.Background())
		defer tun.Stop()

		relayHost := tunnelCfg.ServerAddr
		if d := strings.TrimSpace(*ddnsHost); d != "" {
			relayHost = d
		}
		relayURL := tun.RemoteURLFor(relayHost, *enableSSL)
		authHandler.SetRelayURL(relayURL)
		if !*enableSSL {
			extraHosts = append(extraHosts, tunnelCfg.ServerAddr)
		}
		log.Printf("☁️  Cloud Relay Tunnel ENABLED: %s (via %s:%d)", relayURL, tunnelCfg.ServerAddr, tunnelCfg.ServerPort)
	} else {
		log.Printf("ℹ️  Cloud Relay Tunnel disabled (set FRP_SERVER_ADDR in .env to enable)")
	}

	if *enableSSL {
		// HTTPS cert matches DDNS_HOST only; drop LAN / IPv6 / raw IP from the QR.
		extraHosts = nil
		if d := strings.TrimSpace(*ddnsHost); d != "" && d != qrHost {
			extraHosts = append(extraHosts, d)
		}
	}

	if !authStore.HasDevices() {
		if initialSession, err := pairingMgr.GenerateSession(5 * time.Minute); err == nil {
			auth.PrintPairingQRCode(qrHost, *port, initialSession.Code, *enableSSL, extraHosts...)
		}
	} else {
		log.Printf("ℹ️  Devices already paired — skipping startup QR. Run `make pair` or POST /api/v1/auth/session to mint a new code.")
	}

	// 4. Initialize Push Notification & Background Watcher
	notifCfg := config.GetNotificationConfig()
	watcherCtx, cancelWatcher := context.WithCancel(context.Background())
	defer cancelWatcher()

	// Start desktop focus watcher (annotations filesystem poller)
	go p.StartDesktopFocusWatcher(watcherCtx)

	// Periodically cleanup expired pairing sessions
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				pairingMgr.CleanupExpired()
			case <-watcherCtx.Done():
				return
			}
		}
	}()

	var notif *notifier.Notifier
	if notifCfg.Enabled {
		notif = notifier.NewNotifier(notifCfg)
		p.SetNotificationSink(notif)

		watcher := notifier.NewWatcher(p, notif)
		watcher.Start(watcherCtx)

		log.Printf("🔔 Bark notifications ENABLED")
		log.Printf("   🎯 Target: %s", config.RedactBarkEndpoint(notifCfg.BarkEndpoint))
		log.Printf("   🎨 Icon:   %s", notifCfg.IconURL)
		log.Printf("   📁 Group:  %s", notifCfg.Group)
	} else {
		log.Printf("ℹ️  Bark notifications disabled (set BARK_URL in .env to enable)")
	}

	// 5. Web frontend handler
	webHandler := web.Handler()

	// Start Cockpit quota auto-refresher (every 10 minutes, with auto-launch self-healing & Bark alert)
	var cockpitAlertFn func(title, body string)
	if notif != nil {
		cockpitAlertFn = func(title, body string) {
			_ = notif.NotifyCockpitAlert(title, body)
		}
	}
	cockpit.StartQuotaAutoRefresher(watcherCtx, 10*time.Minute, cockpitAlertFn)

	listenLoopback := auth.IsListenAddrLoopback(*host)
	authPolicy := auth.AuthPolicy{
		TunnelEnabled:  tun != nil,
		ListenLoopback: listenLoopback,
	}
	authHandler.SetAuthPolicy(authPolicy)

	if auth.AuthDisabledRequested() && (authPolicy.TunnelEnabled || !authPolicy.ListenLoopback) {
		log.Fatalf("AUTH_DISABLED is not allowed when a tunnel is enabled or the gateway is not loopback-only")
	}

	// 6 & 7. Build combined Root Router with Middleware
	router := buildRouter(authStore, authHandler, p, insp, time.Now(), webHandler, authPolicy)

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", *host, *port),
		Handler: router,
		// SEC-6: ReadHeaderTimeout prevents Slowloris attacks on the public-facing gateway.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		// WriteTimeout is intentionally 0 (disabled) to avoid cutting off
		// WebSocket and SSE long-lived connections. Each handler manages
		// its own response timeouts via context.WithTimeout.
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	// Print initial status
	if cur := insp.Current(); cur != nil && cur.IsHealthy {
		log.Printf("✅ Upstream connected: 127.0.0.1:%d (PID %d)", cur.Port, cur.PID)
	} else {
		log.Printf("⚠️  Upstream Antigravity instance not detected yet, waiting...")
	}

	// Graceful shutdown channel
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		var err error
		if *tlsCert != "" && *tlsKey != "" {
			log.Printf("🔒 TLS enabled with cert=%s key=%s", *tlsCert, *tlsKey)
			err = server.ListenAndServeTLS(*tlsCert, *tlsKey)
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	if qrHost != "127.0.0.1" {
		log.Printf("📱 Mobile Web UI ready at: http://%s:%d (LAN) | http://127.0.0.1:%d (Local)", qrHost, *port, *port)
	} else {
		log.Printf("📱 Mobile Web UI ready at: http://127.0.0.1:%d", *port)
	}

	<-stopCh
	log.Println("Shutting down gateway...")

	if tun != nil {
		tun.Stop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("Gateway stopped gracefully.")
}

// buildRouter constructs and wraps the HTTP routing mux with middleware.
func buildRouter(
	authStore *auth.AuthStore,
	authHandler *auth.AuthHandler,
	p *proxy.Proxy,
	insp inspector.UpstreamDiscoverer,
	startTime time.Time,
	webHandler http.Handler,
	authPolicy auth.AuthPolicy,
) http.Handler {
	rootMux := http.NewServeMux()

	// Auth endpoints
	rootMux.HandleFunc("/api/v1/auth/pair", authHandler.HandlePair)
	rootMux.HandleFunc("/api/v1/auth/session", authHandler.HandleNewPairingSession)
	rootMux.HandleFunc("POST /api/v1/auth/ws-ticket", authHandler.HandleWSTicket)
	rootMux.HandleFunc("/api/v1/devices/", authHandler.HandleDevices)
	rootMux.HandleFunc("/api/v1/devices", authHandler.HandleDevices)

	// Cockpit endpoints
	rootMux.HandleFunc("GET /api/v1/cockpit/quotas", func(w http.ResponseWriter, r *http.Request) {
		liveEmail, _, _ := p.GetActiveUserStatus()
		quotas, err := cockpit.GetQuotas(liveEmail)
		if err != nil {
			log.Printf("[Cockpit] GetQuotas failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, quotas)
	})
	rootMux.HandleFunc("POST /api/v1/cockpit/refresh", func(w http.ResponseWriter, r *http.Request) {
		log.Println("[Cockpit] Triggering quota refresh...")
		liveEmail, _, _ := p.GetActiveUserStatus()
		quotas, err := cockpit.RefreshQuotas(liveEmail)
		if err != nil {
			log.Printf("[Cockpit] RefreshQuotas failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		log.Println("[Cockpit] Quota refresh completed successfully")
		writeJSON(w, http.StatusOK, quotas)
	})
	rootMux.HandleFunc("POST /api/v1/cockpit/switch", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			AccountID string `json:"account_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.AccountID) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "account_id is required"})
			return
		}
		targetID := strings.TrimSpace(req.AccountID)
		log.Printf("[Cockpit] Switching account to: %s (quit Antigravity first, then Cockpit inject+relaunch)", targetID)
		if err := cockpit.SwitchAccount(targetID); err != nil {
			log.Printf("[Cockpit] SwitchAccount failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		log.Printf("[Cockpit] Account switched successfully to: %s", targetID)
		writeJSON(w, http.StatusOK, map[string]any{
			"status":     "ok",
			"message":    "account switched successfully",
			"account_id": targetID,
		})
	})

	// File / Artifact reading endpoint
	rootMux.HandleFunc("GET /api/v1/files/content", p.HandleFileContent)
	rootMux.HandleFunc("GET /api/v1/files/raw", p.HandleFileRaw)

	// Proxy routes: APIs, WebSocket, Artifacts, Gateway status
	rootMux.Handle("/api/", p)
	rootMux.Handle("/gateway/", p)
	rootMux.Handle("/static/artifacts/", p)
	rootMux.Handle("/connect-websocket", p)

	// Health and readiness probes
	rootMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	rootMux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		cur := insp.Current()
		isReady := cur != nil && cur.IsHealthy && cur.Port > 0
		status := "ready"
		httpCode := http.StatusOK
		if !isReady {
			status = "not_ready"
			httpCode = http.StatusServiceUnavailable
		}
		writeJSON(w, httpCode, map[string]any{
			"status":         status,
			"uptime_seconds": int(time.Since(startTime).Seconds()),
		})
	})

	// Web frontend (catch-all)
	rootMux.Handle("/", webHandler)

	return auth.SecurityHeadersMiddleware(auth.AuthMiddlewareWithPolicy(authStore, rootMux, authPolicy))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("[HTTP] Failed to encode JSON response: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(status)
	w.Write(data)
}
