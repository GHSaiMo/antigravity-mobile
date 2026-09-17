package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
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
	"antigravity-mobile/internal/netutil"
	"antigravity-mobile/internal/notifier"
	"antigravity-mobile/internal/proxy"
	"antigravity-mobile/internal/tunnel"
	"antigravity-mobile/web"
)

func main() {
	// 0. Initialize console output synchronization so concurrent logs don't tear terminal output
	auth.InitConsoleSync()

	// 0.1. Load .env configuration
	config.LoadDotEnv()

	// ==============================================================================
	// 启动项配置参数定义与中文说明
	// 1. host: 监听主机/IP 地址。默认 "" 双栈监听本机所有 IPv4 与 IPv6 接口；设为 127.0.0.1 则仅限本机访问
	defaultHost := ""
	if envHost := os.Getenv("GATEWAY_HOST"); envHost != "" {
		defaultHost = envHost
	}

	// 2. port: 网关服务 HTTP/WebSocket 监听端口，默认 58900 (可通过 GATEWAY_PORT 环境变量覆盖)
	defaultPort := 58900
	if envPort := os.Getenv("GATEWAY_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil && p > 0 {
			defaultPort = p
		}
	}

	// 3. qr: 是否在启动时在终端默认打印一次扫码配对二维码，默认 true (可通过 GATEWAY_QR 环境变量或 -qr=false 控制)
	defaultQR := true
	if envQR := os.Getenv("GATEWAY_QR"); envQR != "" {
		if envQR == "0" || strings.ToLower(envQR) == "false" || strings.ToLower(envQR) == "no" {
			defaultQR = false
		}
	}

	// 命令行 Flags 定义与中文说明
	host := flag.String("host", defaultHost, "网关监听的主机/IP 地址（默认 \"\" 双栈绑定所有 IPv4/IPv6 网卡，设为 127.0.0.1 仅限本机访问）")
	port := flag.Int("port", defaultPort, "网关 HTTP/WebSocket 监听端口（默认 58900）")
	printQR := flag.Bool("qr", defaultQR, "启动时是否在终端默认打印一次配对二维码（默认 true）")
	pollSec := flag.Int("poll", 5, "探测本地 Antigravity 实例与健康检查的轮询间隔秒数（默认 5 秒）")
	ddnsHost := flag.String("ddns", os.Getenv("DDNS_HOST"), "公网 DDNS 域名或固定 IPv6 地址，用于生成扫码配对链接及外部直连")
	enableSSL := flag.Bool("ssl", os.Getenv("GATEWAY_SSL") == "1" || os.Getenv("GATEWAY_SSL") == "true", "是否开启 SSL/HTTPS 模式（默认 false，开启需配合 -tls-cert 与 -tls-key）")
	tlsCert := flag.String("tls-cert", os.Getenv("TLS_CERT_FILE"), "HTTPS 服务 TLS 证书文件路径 (.cer/.crt/.pem)")
	tlsKey := flag.String("tls-key", os.Getenv("TLS_KEY_FILE"), "HTTPS 服务 TLS 私钥文件路径 (.key)")
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
		log.Fatalf("GATEWAY_SSL=1 requires TLS_CERT_FILE and TLS_KEY_FILE")
	}
	if *enableSSL && strings.TrimSpace(*ddnsHost) == "" {
		log.Printf("⚠️  GATEWAY_SSL=1 without DDNS_HOST: pairing will advertise https:// to an IP and iOS certificate checks will fail. Set DDNS_HOST=agy.example.com")
	}
	if !auth.IsListenAddrLoopback(*host) && !hasTLSFiles {
		log.Printf("⚠️  Gateway listening on a non-loopback address without TLS. LAN HTTP is supported; do not advertise this port on the public Internet. Set TLS_CERT_FILE/TLS_KEY_FILE or GATEWAY_SSL=1 for public access.")
	}

	listenDesc := *host
	if listenDesc == "" {
		listenDesc = "0.0.0.0 / [::] (双栈绑定所有网络接口)"
	}
	log.Printf("==================================================")
	log.Printf("🚀 Antigravity Mobile Gateway 启动中...")
	log.Printf("📋 启动项配置:")
	log.Printf("   • 监听地址 (-host)     : %s", listenDesc)
	log.Printf("   • 监听端口 (-port)     : %d", *port)
	log.Printf("   • 实例轮询 (-poll)     : %d 秒", *pollSec)
	log.Printf("   • 配对二维码 (-qr)     : %v", *printQR)
	if *ddnsHost != "" {
		log.Printf("   • 公网 DDNS (-ddns)    : %s", *ddnsHost)
	}
	if *enableSSL {
		log.Printf("   • SSL/TLS 模式 (-ssl)  : 已启用 (证书: %s)", *tlsCert)
	}
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
		proxyProtoVer := strings.TrimSpace(os.Getenv("FRP_PROXY_PROTOCOL_VERSION"))
		if proxyProtoVer == "" {
			proxyProtoVer = "v2"
		}
		tun = tunnel.New(tunnel.Config{
			Enabled:              true,
			ServerAddr:           tunnelCfg.ServerAddr,
			ServerPort:           tunnelCfg.ServerPort,
			Token:                tunnelCfg.Token,
			LocalPort:            *port,
			RemotePort:           tunnelCfg.RemotePort,
			ProxyName:            fmt.Sprintf("antigravity-%d", tunnelCfg.RemotePort),
			TLSEnable:            tunnelCfg.TLSEnable,
			ProxyProtocolVersion: proxyProtoVer,
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
		// ReadTimeout and WriteTimeout are intentionally 0 (disabled) to avoid cutting off
		// WebSocket and SSE long-lived connections. Each handler manages
		// its own request/response timeouts via context.WithTimeout and application-level heartbeats.
		ReadTimeout:  0,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	// Print initial status
	if cur := insp.Current(); cur != nil && cur.IsHealthy {
		log.Printf("✅ Upstream connected: 127.0.0.1:%d (PID %d)", cur.Port, cur.PID)
	} else {
		log.Printf("⚠️  Upstream Antigravity instance not detected yet, waiting...")
	}

	// Synchronously bind the network listener so we verify port availability immediately
	rawListener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		log.Fatalf("❌ Failed to bind server address %s: %v", server.Addr, err)
	}
	defer rawListener.Close()

	// Wrap with AdaptiveListener to support transparent PROXY protocol v1/v2 extraction
	// while maintaining complete compatibility with local curl and direct LAN connections.
	listener := netutil.NewAdaptiveListener(rawListener)
	defer listener.Close()

	// Graceful shutdown channel
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		var err error
		if *tlsCert != "" && *tlsKey != "" {
			log.Printf("🔒 TLS enabled with cert=%s key=%s", *tlsCert, *tlsKey)
			err = server.ServeTLS(listener, *tlsCert, *tlsKey)
		} else {
			err = server.Serve(listener)
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

	// Settle briefly so asynchronous startup logs (e.g. FRP tunnel connect, baseline sync)
	// are printed in the boot logs area before rendering the QR code.
	time.Sleep(150 * time.Millisecond)

	// 默认打印配对二维码（网关已确认启动就绪，输出配对二维码供新客户端接入）
	if *printQR {
		if initialSession, err := pairingMgr.GenerateSession(5 * time.Minute); err == nil {
			if authStore.HasDevices() {
				log.Printf("ℹ️  检测到已有 %d 台已配对设备，打印一次新配对二维码供新客户端接入（可通过 -qr=false 关闭）", len(authStore.ListDevices()))
			}
			auth.PrintPairingQRCode(qrHost, *port, initialSession.Code, *enableSSL, extraHosts...)
		} else {
			log.Printf("⚠️  无法生成初始配对二维码: %v", err)
		}
	} else {
		log.Printf("ℹ️  已跳过启动配对二维码打印（已指定 -qr=false；如需配对可执行 `make pair`）")
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
	rootMux.Handle("/exa.language_server_pb.", p)

	// Desktop static assets (direct endpoints)
	rootMux.HandleFunc("GET /main.js", p.HandleDesktopStatic)
	rootMux.HandleFunc("GET /jetbox.css", p.HandleDesktopStatic)
	rootMux.HandleFunc("GET /compiled_tailwind.css", p.HandleDesktopStatic)
	rootMux.HandleFunc("GET /prism_bundle.js", p.HandleDesktopStatic)
	rootMux.HandleFunc("GET /diff_worker.js", p.HandleDesktopStatic)
	rootMux.Handle("/symbols-icons/", p)

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

	// Adaptive Dual-Mode Web frontend (Desktop Workbench on iPad/PC, Lightweight PWA on Phones)
	adaptiveWebHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// ConnectRPC direct proto calls (e.g. /exa.language_server_pb.LanguageServerService/...)
		if strings.HasPrefix(path, "/exa.language_server_pb.") {
			p.ServeHTTP(w, r)
			return
		}

		// Embedded web files (localization, switcher, mobile app files)
		if path == "/zh-CN.js" || path == "/view-switcher.js" || path == "/view-switcher.css" ||
			path == "/style.css" || path == "/app.js" || path == "/mermaid.min.js" ||
			path == "/manifest.json" || path == "/sw.js" || strings.HasPrefix(path, "/icons/") {
			webHandler.ServeHTTP(w, r)
			return
		}

		// Desktop static asset fallback
		if isDesktopStaticPath(path) {
			p.HandleDesktopStatic(w, r)
			return
		}

		// Determine view mode (desktop vs mobile)
		viewMode := determineViewMode(r)
		qv := r.URL.Query().Get("view")

		// Persist if explicitly requested via query param
		if qv != "" {
			http.SetCookie(w, &http.Cookie{
				Name:     "agy_view_mode",
				Value:    viewMode,
				Path:     "/",
				MaxAge:   86400 * 365,
				SameSite: http.SameSiteLaxMode,
			})
		}

		// Check if request is authenticated before serving desktop workbench.
		// If unauthenticated and no explicit view parameter, show mobile view so user can pair.
		token := auth.ExtractToken(r)
		_, isAuthenticated := authStore.ValidateToken(token)
		if !isAuthenticated && viewMode == "desktop" && qv == "" {
			viewMode = "mobile"
		}

		if viewMode == "desktop" {
			p.HandleDesktopIndex(w, r)
			return
		}

		// Mobile view
		webHandler.ServeHTTP(w, r)
	})

	rootMux.Handle("/", adaptiveWebHandler)

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

func determineViewMode(r *http.Request) string {
	// 1. Explicit query parameter (?view=desktop / ?view=mobile, or ?mode=...)
	qView := strings.ToLower(r.URL.Query().Get("view"))
	if qView == "" {
		qView = strings.ToLower(r.URL.Query().Get("mode"))
	}
	if qView == "desktop" || qView == "ipad" || qView == "pc" {
		return "desktop"
	}
	if qView == "mobile" || qView == "phone" {
		return "mobile"
	}

	// 2. Explicit cookie (agy_view_mode)
	if c, err := r.Cookie("agy_view_mode"); err == nil {
		val := strings.ToLower(c.Value)
		if val == "desktop" || val == "mobile" {
			return val
		}
	}

	// 3. User-Agent auto-detection
	ua := strings.ToLower(r.UserAgent())

	// Mobile phones: iPhone, iPod, or Android with "Mobile"
	if strings.Contains(ua, "iphone") || strings.Contains(ua, "ipod") {
		return "mobile"
	}
	if strings.Contains(ua, "android") && strings.Contains(ua, "mobile") {
		return "mobile"
	}

	// Default to desktop for iPad, Mac, Windows, Linux, Tablets, and desktop browsers
	return "desktop"
}

func isDesktopStaticPath(path string) bool {
	return path == "/main.js" ||
		path == "/jetbox.css" ||
		path == "/compiled_tailwind.css" ||
		path == "/prism_bundle.js" ||
		path == "/diff_worker.js" ||
		path == "/icon.png" ||
		path == "/favicon.ico" ||
		strings.HasPrefix(path, "/symbols-icons/")
}
