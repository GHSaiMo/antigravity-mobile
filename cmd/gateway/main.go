package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"antigravity-mobile/internal/auth"
	"antigravity-mobile/internal/cockpit"
	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/inspector"
	"antigravity-mobile/internal/localtls"
	"antigravity-mobile/internal/logx"
	"antigravity-mobile/internal/netutil"
	"antigravity-mobile/internal/notifier"
	"antigravity-mobile/internal/proxy"
	"antigravity-mobile/internal/tunnel"
	"antigravity-mobile/web"
)

// Version represents the Multigravity Gateway release version.
var Version = "1.0.5"

func main() {
	// 0. Initialize console output synchronization so concurrent logs don't tear terminal output
	auth.InitConsoleSync()
	logx.Init(log.Writer())

	// 0.1. Load .env configuration
	config.LoadDotEnv()

	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "pair":
			runPairCmd(args[1:])
			return
		case "cockpit":
			cockpit.RunCockpitCmd(args[1:])
			return
		case "bark":
			notifier.RunBarkCmd(args[1:])
			return
		case "cloudflare", "cf":
			tunnel.RunCloudflareCmd(args[1:])
			return
		case "lan":
			runLanCmd(args[1:])
			return
		case "list":
			runListCmd(args[1:])
			return
		case "clear":
			runClearCmd(args[1:])
			return
		case "version", "-v", "--version":
			runVersionCmd()
			return
		case "help", "-h", "--help":
			runHelpCmd()
			return
		case "run":
			runGatewayServer(args[1:])
			return
		default:
			if strings.HasPrefix(args[0], "-") {
				runGatewayServer(args)
				return
			}
			fmt.Fprintf(os.Stderr, "❌ 未知子命令: %s\n\n", args[0])
			runHelpCmd()
			os.Exit(1)
		}
	} else {
		runGatewayServer(nil)
	}
}

func defaultHost() string {
	return os.Getenv("MULTIGRAVITY_HOST")
}

func defaultPort() int {
	defaultPort := 58900
	envPort := os.Getenv("MULTIGRAVITY_PORT")
	if envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil && p > 0 {
			defaultPort = p
		}
	}
	return defaultPort
}

func runGatewayServer(args []string) {
	// ==============================================================================
	// 启动项配置参数定义与中文说明
	// 1. host: 监听主机/IP 地址。默认 "" 监听本机所有 IPv4 接口；设为 127.0.0.1 则仅限本机访问
	defaultHost := defaultHost()

	// 2. port: 网关服务 HTTP/WebSocket 监听端口，默认 58900 (可通过 MULTIGRAVITY_PORT 环境变量覆盖)
	defaultPort := defaultPort()

	fs := flag.NewFlagSet("mgy", flag.ExitOnError)
	host := fs.String("host", defaultHost, "网关监听的主机/IP 地址（默认 \"\" 绑定所有 IPv4 网卡，设为 127.0.0.1 仅限本机访问）")
	port := fs.Int("port", defaultPort, "网关 HTTP/WebSocket 监听端口（默认 58900）")
	printQR := fs.Bool("qr", false, "启动时是否输出配对二维码（默认: 未配对时自动输出，已配对时默认隐藏）")
	pollSec := fs.Int("poll", 5, "探测本地 Antigravity 实例与健康检查的轮询间隔秒数（默认 5 秒）")
	ddnsHost := fs.String("ddns", os.Getenv("DDNS_HOST"), "公网 DDNS 域名，用于生成扫码配对链接及外部直连")
	trustLAN := fs.Bool("trust-lan", config.GetTrustLAN(), "是否信任局域网访问（允许免配对直接使用，默认: 需配对码）")
	_ = fs.Parse(args)

	qrExplicitlySet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "qr" {
			qrExplicitlySet = true
		}
	})
	if envQR := os.Getenv("MULTIGRAVITY_QR"); envQR != "" && !qrExplicitlySet {
		if envQR == "1" || strings.ToLower(envQR) == "true" || strings.ToLower(envQR) == "yes" {
			*printQR = true
			qrExplicitlySet = true
		} else if envQR == "0" || strings.ToLower(envQR) == "false" || strings.ToLower(envQR) == "no" {
			*printQR = false
			qrExplicitlySet = true
		}
	}

	if _, _, err := auth.EnsureAdminToken(true); err != nil {
		log.Fatalf("❌ Failed to initialize MULTIGRAVITY_ADMIN_TOKEN: %v", err)
	}

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
	qrHost := netAddrs.LANIPv4
	if qrHost == "" {
		qrHost = "127.0.0.1"
	}
	if *host != "" && *host != "0.0.0.0" && *host != "::" && *host != "[::]" {
		qrHost = *host
	}
	var extraHosts []string
	if netAddrs.LANIPv4 != "" && netAddrs.LANIPv4 != qrHost {
		extraHosts = append(extraHosts, netAddrs.LANIPv4)
	}

	pairingMgr := auth.NewPairingManager()
	authHandler := auth.NewAuthHandler(authStore, pairingMgr, qrHost, *port, false)
	authHandler.SetEndpoints(netAddrs.LANIPv4, *ddnsHost)

	qrPort := *port

	// 3.5. Initialize Automated Cloudflare Tunnel (Exclusive HTTPS Domain)
	cfCfg := config.GetCloudflareConfig()
	var cfTunnel *tunnel.CloudflareTunnel
	var cfDomain string
	if cfCfg.Enabled {
		cfCtx, cancelCF := context.WithCancel(context.Background())
		defer cancelCF()

		binPath, err := tunnel.EnsureCloudflaredBinary(cfCtx)
		if err != nil {
			slog.Warn("⚠️  Cloudflare 穿透引擎准备失败", "err", err)
		} else {
			var cfRes *tunnel.CFTunnelResult
			if cfCfg.Token != "" {
				cfRes = &tunnel.CFTunnelResult{
					Success:   true,
					Token:     cfCfg.Token,
					Subdomain: "custom.mgy",
					URL:       "https://custom.mgy",
				}
			} else {
				cfRes, err = tunnel.RegisterOrFetchTunnel(cfCtx, cfCfg.WorkerURL, cfCfg.InviteCode)
			}

			if err != nil {
				slog.Warn("⚠️  Cloudflare 隧道注册失败", "err", err)
			} else if cfRes != nil {
				cfTunnel = tunnel.NewCloudflareTunnel(cfRes, &cfCfg)
				if err := cfTunnel.Start(cfCtx, binPath); err != nil {
					slog.Warn("⚠️  启动 cloudflared 失败", "err", err)
				} else {
					defer cfTunnel.Stop()
					authHandler.SetCloudflareURL(cfRes.URL)
					authHandler.SetPrimary(cfRes.Subdomain, 443, true)
					cfDomain = cfRes.URL

					// 将专属 HTTPS 域名设为二维码主地址，强制走 HTTPS 443！
					qrHost = cfRes.Subdomain
					qrPort = 443
					extraHosts = nil
					if netAddrs.LANIPv4 != "" {
						extraHosts = append(extraHosts, netAddrs.LANIPv4)
					}
				}
			}
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
	var pushSummary string
	if notifCfg.Enabled {
		notif = notifier.NewNotifier(notifCfg)
		p.SetNotificationSink(notif)

		watcher := notifier.NewWatcher(p, notif)
		watcher.Start(watcherCtx)

		var pushes []string
		if notifCfg.BarkEndpoint != "" {
			pushes = append(pushes, "Bark (iOS)")
		}
		if notifCfg.FCMEnabled {
			pushes = append(pushes, "FCM (Android)")
		}
		if len(pushes) > 0 {
			pushSummary = strings.Join(pushes, " + ") + " 已启用"
		}
	}
	if pushSummary == "" {
		pushSummary = "未配置 (支持 Bark / FCM)"
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

	// Align Cockpit configuration to prevent switch failures (APP_PATH_NOT_FOUND)
	if err := cockpit.EnsureCockpitAntigravityConfig(); err != nil {
		slog.Info("[Cockpit] Note: EnsureCockpitAntigravityConfig", "err", err)
	}

	cockpit.StartQuotaAutoRefresher(watcherCtx, 10*time.Minute, cockpitAlertFn)

	listenLoopback := auth.IsListenAddrLoopback(*host)
	authPolicy := auth.AuthPolicy{
		TunnelEnabled:  cfTunnel != nil,
		ListenLoopback: listenLoopback,
		TrustLAN:       *trustLAN,
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

	// Synchronously bind the network listener so we verify port availability immediately
	// Strictly listen on IPv4 only (tcp4) to prevent exposing IPv6 sockets or Bonjour/scanner characteristics.
	rawListener, err := net.Listen("tcp4", server.Addr)
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
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Brief pause to allow initial upstream detection
	time.Sleep(100 * time.Millisecond)

	upstreamDesc := "等待 Antigravity 启动..."
	if cur := insp.Current(); cur != nil && cur.IsHealthy {
		upstreamDesc = fmt.Sprintf("已连接 (PID %d, 端口 %d)", cur.PID, cur.Port)
	}


	lanDisplay := netAddrs.LANIPv4
	if lanDisplay == "" && qrHost != "127.0.0.1" && !strings.Contains(qrHost, ":") {
		lanDisplay = qrHost
	}
	lanURL := "-"
	if lanDisplay != "" {
		lanURL = fmt.Sprintf("http://%s:%d", lanDisplay, *port)
	}

	deviceCount := len(authStore.ListDevices())
	deviceDesc := "0 台"
	if deviceCount > 0 {
		deviceDesc = fmt.Sprintf("%d 台已配对", deviceCount)
	}

	// 打印清爽紧凑的启动看板
	fmt.Println()
	fmt.Printf("  Multigravity (mgy) v%s\n", Version)
	fmt.Println("  --------------------------------------------------")
	fmt.Printf("  ➜  本地访问:   http://127.0.0.1:%d (免配对)\n", *port)
	if lanURL != "-" {
		if authPolicy.TrustLAN {
			fmt.Printf("  ➜  局域网络:   %s (已信任免配对)\n", lanURL)
		} else {
			fmt.Printf("  ➜  局域网络:   %s (需配对码)\n", lanURL)
		}
	}
	if cfDomain != "" {
		fmt.Printf("  ➜  云端穿透:   Cloudflare 专属域名已生成 (强制配对)\n")
	} else if *ddnsHost != "" {
		fmt.Printf("  ➜  云端穿透:   DDNS 专属域名已配置 (强制配对)\n")
	}
	if authPolicy.TrustLAN {
		fmt.Printf("  ➜  安全策略:   局域网已信任免密放行 (可通过 mgy lan 切换策略)\n")
	} else {
		fmt.Printf("  ➜  安全策略:   局域网标准安全配对 (可通过 mgy lan 切换策略)\n")
	}
	fmt.Printf("  ➜  目标实例:   %s\n", upstreamDesc)
	fmt.Printf("  ➜  远程推送:   %s\n", pushSummary)
	fmt.Printf("  ➜  已配设备:   %s\n", deviceDesc)
	fmt.Println("  --------------------------------------------------")

	shouldPrintQR := false
	if qrExplicitlySet {
		shouldPrintQR = *printQR
	} else {
		// 未显式指定 -qr 时：未配对设备自动打印二维码；已有配对设备则保持界面清爽
		shouldPrintQR = deviceCount == 0
	}

	if shouldPrintQR {
		if initialSession, err := pairingMgr.GenerateSession(5 * time.Minute); err == nil {
			auth.PrintPairingQRCode(qrHost, qrPort, initialSession.Code, cfTunnel != nil, extraHosts...)
		} else {
			slog.Warn("⚠️  无法生成初始配对二维码", "err", err)
		}
	} else if deviceCount > 0 {
		fmt.Println("  💡 提示: 执行 `mgy pair` 可随时申请新设备配对二维码。")
		fmt.Println()
	}

	<-stopCh
	slog.Info("🛑 网关正在安全停止...")

	// 1. Stop background watchers and inspector polling first to prevent new requests
	cancelWatcher()
	insp.Stop()

	if cfTunnel != nil {
		cfTunnel.Stop()
	}

	// 2. Gracefully close all tracked WebSocket connections so server.Shutdown can drain
	p.Shutdown()

	// 3. Shutdown HTTP server with 10s timeout (hijacked WS conns already closed)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		slog.Warn("Server shutdown error", "err", err)
	}
	slog.Info("✅ 网关已完全退出。")
}

func runVersionCmd() {
	fmt.Printf("Multigravity (mgy) %s\n", Version)
}

func runHelpCmd() {
	fmt.Printf(`Multigravity (mgy) %s - Unified Mobile Gateway for Antigravity

用法:
  mgy [子命令] [参数]

常用子命令:
  run (默认)        启动网关服务 (局域网直连 + Cloudflare 专属 HTTPS 隧道)
  pair              向正在运行的网关申请并打印新配对二维码与链接
  cockpit           交互式配置 Cockpit 报表服务与安全 Token (支持 status/token/restart)
  bark              交互式配置 Bark 实时推送与提示音 (iOS 专用, 支持 status/test/set)
  cloudflare (cf)   交互式配置 Cloudflare 专属穿透隧道与域名 (支持 status/reset/enable)
  lan               交互式配置局域网安全配对策略 (支持 status/trust/pair)
  list              查看所有已配对授权的移动设备 (支持在线与离线查看)
  clear [all|id]    清除已配对的设备授权 (支持: mgy clear all 或 mgy clear <device-id>)
  version           查看当前版本信息
  help              显示帮助信息

网关运行参数 (用于 mgy 或 mgy run):
  -port <端口号>          HTTP/WebSocket 监听端口 (默认: 58900, 环境变量: MULTIGRAVITY_PORT)
  -host <主机/IP>         监听地址 (默认: "" 全网卡 IPv4 监听; 设为 127.0.0.1 仅限本机)
  -qr=<true|false>        启动时是否打印配对二维码 (默认: true)
  -poll <秒数>            Antigravity 实例轮询间隔 (默认: 5秒)
  -ddns <域名/IP>         公网 DDNS 域名或固定 IP 地址
  -trust-lan=<true|false> 是否信任局域网免配对访问 (默认: false, 环境变量: MULTIGRAVITY_TRUST_LAN)
  -open=<true|false>      启动时是否自动在默认浏览器打开主页 (默认: true, 无头系统自动跳过)
`, Version)
}

func runPairCmd(args []string) {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	port := defaultPort()
	portFlag := fs.Int("port", port, "网关端口")
	_ = fs.Parse(args)

	targetPort := *portFlag
	adminToken := auth.GetAdminToken()
	if adminToken == "" {
		if path, _, err := auth.EnsureAdminToken(false); err == nil && path != "" {
			if b, err := os.ReadFile(path); err == nil {
				adminToken = strings.TrimSpace(string(b))
			}
		}
	}

	client := &http.Client{
		Timeout:   3 * time.Second,
		Transport: localtls.NewLoopbackTransport(), // SEC-AUDIT M-2: gate InsecureSkipVerify to loopback only
	}

	u := fmt.Sprintf("http://127.0.0.1:%d/api/v1/auth/session", targetPort)
	req, err := http.NewRequest(http.MethodPost, u, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 创建请求失败: %v\n", err)
		os.Exit(1)
	}
	if adminToken != "" {
		req.Header.Set("Authorization", "Bearer "+adminToken)
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 无法连接到网关 (端口 %d): %v\n", targetPort, err)
		fmt.Fprintf(os.Stderr, "   请确认网关是否已在运行 (启动命令: mgy 或 mgy run)\n")
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		fmt.Fprintf(os.Stderr, "❌ 网关拒绝签发配对码 (HTTP %d): %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		if resp.StatusCode == http.StatusUnauthorized {
			fmt.Fprintf(os.Stderr, "   提示: 请确认管理员令牌已配置在环境变量 MULTIGRAVITY_ADMIN_TOKEN 或 ~/.multigravity/admin_token。\n")
		}
		os.Exit(1)
	}

	var sessionResp struct {
		Code string `json:"code"`
		URI  string `json:"uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sessionResp); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 解析网关返回失败: %v\n", err)
		os.Exit(1)
	}

	auth.PrintRawPairingQRCode(sessionResp.Code, sessionResp.URI)
}

func runListCmd(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	port := defaultPort()
	portFlag := fs.Int("port", port, "网关端口")
	_ = fs.Parse(args)

	targetPort := *portFlag
	adminToken := auth.GetAdminToken()
	if adminToken == "" {
		if path, _, err := auth.EnsureAdminToken(false); err == nil && path != "" {
			if b, err := os.ReadFile(path); err == nil {
				adminToken = strings.TrimSpace(string(b))
			}
		}
	}

	urlStr := fmt.Sprintf("http://127.0.0.1:%d/api/v1/devices", targetPort)

	var devices []auth.PairedDevice
	mode := "离线模式 (直接读取本地凭据)"

	client := &http.Client{
		Timeout:   2 * time.Second,
		Transport: localtls.NewLoopbackTransport(), // SEC-AUDIT M-2: gate InsecureSkipVerify to loopback only
	}
	req, _ := http.NewRequest(http.MethodGet, urlStr, nil)
	if adminToken != "" {
		req.Header.Set("Authorization", "Bearer "+adminToken)
	}

	if resp, err := client.Do(req); err == nil && resp.StatusCode == http.StatusOK {
		mode = "在线模式 (网关实时探测)"
		_ = json.NewDecoder(resp.Body).Decode(&devices)
		resp.Body.Close()
	} else {
		store, err := auth.NewAuthStore("")
		if err == nil {
			devices = store.ListDevices()
		}
	}

	printDeviceTable(devices, mode)
}

func printDeviceTable(devices []auth.PairedDevice, mode string) {
	fmt.Println("========================================================================================================")
	if len(devices) == 0 {
		fmt.Printf("ℹ️  当前暂无已配对设备 (%s)\n", mode)
		fmt.Println("💡 提示: 执行 mgy pair 可生成配对二维码与扫码链接。")
		fmt.Println("========================================================================================================")
		return
	}

	fmt.Printf("📱 Multigravity 已配对设备列表 (共 %d 台 | %s)\n", len(devices), mode)
	fmt.Println("========================================================================================================")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "设备 ID\t设备名称\t平台\t首次配对时间\t最后活跃时间\t最后 IP")
	fmt.Fprintln(w, "-------\t--------\t----\t------------\t------------\t-------")
	for _, dev := range devices {
		created := "-"
		if !dev.CreatedAt.IsZero() {
			created = dev.CreatedAt.Format("2006-01-02 15:04:05")
		}
		lastSeen := "-"
		if !dev.LastSeenAt.IsZero() {
			lastSeen = dev.LastSeenAt.Format("2006-01-02 15:04:05")
		}
		lastIP := dev.LastSeenIP
		if lastIP == "" {
			lastIP = "-"
		}
		name := dev.DeviceName
		if name == "" {
			name = "未知设备"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", dev.DeviceID, name, dev.Platform, created, lastSeen, lastIP)
	}
	w.Flush()
	fmt.Println("========================================================================================================")
	fmt.Println("💡 提示: 执行 mgy clear all 可清空所有设备授权；执行 mgy pair 可生成新配对二维码。")
}

func runClearCmd(args []string) {
	target := "all"
	if len(args) > 0 && args[0] != "" {
		if args[0] == "all" && len(args) > 1 {
			target = args[1]
		} else {
			target = args[0]
		}
	}

	targetPort := defaultPort()
	adminToken := auth.GetAdminToken()
	if adminToken == "" {
		if path, _, err := auth.EnsureAdminToken(false); err == nil && path != "" {
			if b, err := os.ReadFile(path); err == nil {
				adminToken = strings.TrimSpace(string(b))
			}
		}
	}

	client := &http.Client{
		Timeout:   3 * time.Second,
		Transport: localtls.NewLoopbackTransport(), // SEC-AUDIT M-2: gate InsecureSkipVerify to loopback only
	}

	urlStr := fmt.Sprintf("http://127.0.0.1:%d/api/v1/devices/%s", targetPort, target)
	req, _ := http.NewRequest(http.MethodDelete, urlStr, nil)
	if adminToken != "" {
		req.Header.Set("Authorization", "Bearer "+adminToken)
	}

	if resp, err := client.Do(req); err == nil && (resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent) {
		resp.Body.Close()
		if target == "all" {
			fmt.Println("✅ [在线网关] 已成功清除所有已配对设备授权。")
		} else {
			fmt.Printf("✅ [在线网关] 已成功清除设备 [%s] 的授权。\n", target)
		}
		return
	}

	// Offline fallback
	store, err := auth.NewAuthStore("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 无法打开设备凭据存储: %v\n", err)
		os.Exit(1)
	}
	if target == "all" {
		count := len(store.ListDevices())
		store.ClearAll()
		fmt.Printf("✅ [离线模式] 已清除全部 %d 台已配对设备授权。\n", count)
	} else {
		if err := store.RemoveDevice(target); err == nil {
			fmt.Printf("✅ [离线模式] 已成功清除设备 [%s] 的授权。\n", target)
		} else {
			fmt.Printf("⚠️  [离线模式] 清除设备失败: %v\n", err)
		}
	}
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
	rootMux.HandleFunc("/api/v1/auth/unpair", authHandler.HandleUnpair)
	rootMux.HandleFunc("/api/v1/auth/session", authHandler.HandleNewPairingSession)
	rootMux.HandleFunc("/api/v1/auth/endpoints", authHandler.HandleEndpoints)
	rootMux.HandleFunc("POST /api/v1/auth/ws-ticket", authHandler.HandleWSTicket)
	rootMux.HandleFunc("/api/v1/devices/", authHandler.HandleDevices)
	rootMux.HandleFunc("/api/v1/devices", authHandler.HandleDevices)

	// Mobile push token registration endpoint (for Android FCM and other clients)
	rootMux.HandleFunc("POST /api/v1/device/push-token", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Platform    string `json:"platform"`
			Token       string `json:"token"`
			FCMToken    string `json:"fcm_token"`
			DeviceToken string `json:"device_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		pushToken := strings.TrimSpace(req.Token)
		if pushToken == "" {
			pushToken = strings.TrimSpace(req.FCMToken)
		}
		if pushToken == "" {
			pushToken = strings.TrimSpace(req.DeviceToken)
		}
		if pushToken == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token is required"})
			return
		}

		if sink, ok := p.NotificationSink().(*notifier.Notifier); ok && sink != nil {
			sink.UpdateFCMDeviceToken(pushToken)
		} else {
			cfg := config.GetNotificationConfig()
			cfg.FCMDeviceToken = pushToken
			cfg.FCMEnabled = true
			cfg.Enabled = true
			newNotif := notifier.NewNotifier(cfg)
			p.SetNotificationSink(newNotif)
		}

		slog.Info(fmt.Sprintf("[PushToken] 📱 Registered %s push token: %s", req.Platform, config.RedactFCMKey(pushToken)))
		writeJSON(w, http.StatusOK, map[string]string{
			"status":   "ok",
			"platform": req.Platform,
			"message":  "push token registered successfully",
		})
	})

	// Cockpit endpoints
	rootMux.HandleFunc("GET /api/v1/cockpit/quotas", func(w http.ResponseWriter, r *http.Request) {
		liveEmail, _, _ := p.GetActiveUserStatus()
		quotas, err := cockpit.GetQuotas(liveEmail)
		if err != nil {
			slog.Warn("[Cockpit] GetQuotas failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, quotas)
	})
	rootMux.HandleFunc("POST /api/v1/cockpit/refresh", func(w http.ResponseWriter, r *http.Request) {
		slog.Info("[Cockpit] Triggering quota refresh...")
		liveEmail, _, _ := p.GetActiveUserStatus()
		quotas, err := cockpit.RefreshQuotas(liveEmail)
		if err != nil {
			slog.Warn("[Cockpit] RefreshQuotas failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		slog.Info("[Cockpit] Quota refresh completed successfully")
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
		slog.Info(fmt.Sprintf("[Cockpit] Switching account to: %s (quit Antigravity first, then Cockpit inject+relaunch)", targetID))
		if err := cockpit.SwitchAccount(targetID); err != nil {
			slog.Warn("[Cockpit] SwitchAccount failed", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		slog.Info(fmt.Sprintf("[Cockpit] Account switched successfully to: %s", targetID))
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
	rootMux.Handle("/api/", web.GzipHandler(p))
	rootMux.Handle("/gateway/", web.GzipHandler(p))
	rootMux.Handle("/static/artifacts/", web.GzipHandler(p))
	rootMux.Handle("/connect-websocket", p) // WebSocket: must NOT wrap with GzipHandler
	rootMux.Handle("/exa.language_server_pb.", web.GzipHandler(p))
	rootMux.Handle("/exa.language_server_pb.LanguageServerService/", web.GzipHandler(p))

	// Version, health and readiness probes
	rootMux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"version":  Version,
			"app_name": "Multigravity",
			"os":       runtime.GOOS,
		})
	})
	rootMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":   "ok",
			"version":  Version,
			"os":       runtime.GOOS,
			"platform": runtime.GOOS,
		})
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
			"version":        Version,
			"os":             runtime.GOOS,
			"platform":       runtime.GOOS,
			"uptime_seconds": int(time.Since(startTime).Seconds()),
		})
	})

	// Desktop static assets (direct endpoints)
	rootMux.HandleFunc("GET /main.js", p.HandleDesktopStatic)
	rootMux.HandleFunc("GET /jetbox.css", p.HandleDesktopStatic)
	rootMux.HandleFunc("GET /compiled_tailwind.css", p.HandleDesktopStatic)
	rootMux.HandleFunc("GET /prism_bundle.js", p.HandleDesktopStatic)
	rootMux.HandleFunc("GET /diff_worker.js", p.HandleDesktopStatic)
	rootMux.HandleFunc("GET /audio_processor.js", p.HandleDesktopStatic)
	rootMux.HandleFunc("GET /icon.png", p.HandleDesktopStatic)
	rootMux.Handle("/symbols-icons/", http.HandlerFunc(p.HandleDesktopStatic))

	// Dedicated direct endpoints for Desktop and PWA
	rootMux.HandleFunc("GET /desktop", func(w http.ResponseWriter, r *http.Request) {
		if !authHandler.RequestAuthorized(r) {
			serveDesktopPairingPage(w)
			return
		}
		p.HandleDesktopIndex(w, r)
	})
	rootMux.HandleFunc("GET /pwa", webHandler.ServeHTTP)

	// Proactively redirect /onboarding to root to prevent web clients getting trapped
	rootMux.HandleFunc("GET /onboarding", func(w http.ResponseWriter, r *http.Request) {
		redirectURL := r.URL.Query().Get("redirect")
		if redirectURL == "" || redirectURL == "/onboarding" || strings.HasPrefix(redirectURL, "/onboarding") {
			redirectURL = "/"
		}
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
	})

	// Adaptive Web frontend:
	// - PC / Computers (Mac, Windows, Linux) -> Desktop Workbench
	// - iPhone or Tablet (iPad, Android, Mobile) -> Mobile PWA
	// - Explicit ?view=desktop or ?view=pwa / cookie override
	adaptiveWebHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// ConnectRPC direct proto calls
		if strings.HasPrefix(path, "/exa.language_server_pb.") {
			web.GzipHandler(p).ServeHTTP(w, r)
			return
		}

		// Embedded web files for mobile PWA
		if path == "/style.css" || strings.HasPrefix(path, "/js/") || path == "/mermaid.min.js" ||
			path == "/manifest.json" || path == "/sw.js" || path == "/favicon.ico" || strings.HasPrefix(path, "/icons/") {
			webHandler.ServeHTTP(w, r)
			return
		}

		// Desktop static asset fallback
		if proxy.IsDesktopStaticPath(path) {
			web.GzipHandler(http.HandlerFunc(p.HandleDesktopStatic)).ServeHTTP(w, r)
			return
		}

		// Determine view mode (desktop vs pwa)
		viewMode := determineViewMode(r)
		qv := r.URL.Query().Get("view")
		if qv == "" {
			qv = r.URL.Query().Get("mode")
		}

		// Explicit ?view query parameter overrides viewMode and sets cookie
		if qv != "" {
			http.SetCookie(w, &http.Cookie{
				Name:     "agy_view_mode",
				Value:    viewMode,
				Path:     "/",
				MaxAge:   86400 * 365,
				SameSite: http.SameSiteLaxMode,
			})
		}

		if viewMode == "desktop" {
			if !authHandler.RequestAuthorized(r) {
				serveDesktopPairingPage(w)
				return
			}
			p.HandleDesktopIndex(w, r)
			return
		}

		// Mobile / tablet view
		webHandler.ServeHTTP(w, r)
	})

	rootMux.Handle("/", adaptiveWebHandler)

	// Inject Cloudflare tunnel domain in response headers for client auto-discovery and healing
	endpointHeadersMiddleware := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cf := authHandler.CloudflareURL(); cf != "" {
			w.Header().Set("X-Antigravity-Cloud-URL", cf)
		}
		rootMux.ServeHTTP(w, r)
	})

	// Wrap with security headers, body size ceiling (64MB), and authentication policy
	return auth.SecurityHeadersMiddleware(auth.MaxBytesMiddleware(64*1024*1024, auth.AuthMiddlewareWithPolicy(authStore, endpointHeadersMiddleware, authPolicy)))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		slog.Warn("[HTTP] Failed to encode JSON response", "err", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(status)
	w.Write(data)
}

// determineViewMode detects whether the client should receive the desktop workbench or mobile PWA.
// - Computers/PC (Mac, Windows, Linux) -> "desktop"
// - iPhone or Tablets (iPad, Android, Mobile, Tablet) -> "pwa"
// - Explicit ?view=desktop or ?view=pwa (or cookie) overrides default.
func determineViewMode(r *http.Request) string {
	// 1. Explicit query parameter (?view=desktop / ?view=pwa / ?view=mobile)
	qView := strings.ToLower(r.URL.Query().Get("view"))
	if qView == "" {
		qView = strings.ToLower(r.URL.Query().Get("mode"))
	}
	if qView == "desktop" || qView == "pc" {
		return "desktop"
	}
	if qView == "pwa" || qView == "mobile" || qView == "phone" || qView == "tablet" || qView == "ipad" {
		return "pwa"
	}

	// 2. Explicit cookie (agy_view_mode)
	if c, err := r.Cookie("agy_view_mode"); err == nil {
		val := strings.ToLower(c.Value)
		if val == "desktop" {
			return "desktop"
		}
		if val == "pwa" || val == "mobile" {
			return "pwa"
		}
	}

	// 3. User-Agent detection:
	// iPhone or Tablet (iPad, Android, Mobile, Tablet) -> PWA
	ua := strings.ToLower(r.UserAgent())
	isPhoneOrTablet := strings.Contains(ua, "iphone") || strings.Contains(ua, "ipod") ||
		strings.Contains(ua, "ipad") || strings.Contains(ua, "tablet") ||
		strings.Contains(ua, "android") || strings.Contains(ua, "mobile")

	if isPhoneOrTablet {
		return "pwa"
	}

	// Default to desktop for PC / Computer (Mac, Windows, Linux)
	return "desktop"
}
