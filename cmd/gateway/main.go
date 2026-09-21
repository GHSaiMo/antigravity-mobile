package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
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

// Version represents the Multigravity Gateway release version.
var Version = "1.0.2"

func main() {
	// 0. Initialize console output synchronization so concurrent logs don't tear terminal output
	auth.InitConsoleSync()

	// 0.1. Load .env configuration
	config.LoadDotEnv()

	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "pair":
			runPairCmd(args[1:])
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

func isSSLEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("MULTIGRAVITY_SSL")))
	return v == "1" || v == "true" || v == "yes"
}

func runGatewayServer(args []string) {
	// ==============================================================================
	// 启动项配置参数定义与中文说明
	// 1. host: 监听主机/IP 地址。默认 "" 双栈监听本机所有 IPv4 与 IPv6 接口；设为 127.0.0.1 则仅限本机访问
	defaultHost := defaultHost()

	// 2. port: 网关服务 HTTP/WebSocket 监听端口，默认 58900 (可通过 MULTIGRAVITY_PORT 环境变量覆盖)
	defaultPort := defaultPort()

	// 3. qr: 是否在启动时在终端默认打印一次扫码配对二维码，默认 true (可通过 MULTIGRAVITY_QR 环境变量或 -qr=false 控制)
	defaultQR := true
	if envQR := os.Getenv("MULTIGRAVITY_QR"); envQR != "" {
		if envQR == "0" || strings.ToLower(envQR) == "false" || strings.ToLower(envQR) == "no" {
			defaultQR = false
		}
	}

	// 命令行 Flags 定义与中文说明
	fs := flag.NewFlagSet("mgy", flag.ExitOnError)
	host := fs.String("host", defaultHost, "网关监听的主机/IP 地址（默认 \"\" 双栈绑定所有 IPv4/IPv6 网卡，设为 127.0.0.1 仅限本机访问）")
	port := fs.Int("port", defaultPort, "网关 HTTP/WebSocket 监听端口（默认 58900）")
	printQR := fs.Bool("qr", defaultQR, "启动时是否在终端默认打印一次配对二维码（默认 true）")
	pollSec := fs.Int("poll", 5, "探测本地 Antigravity 实例与健康检查的轮询间隔秒数（默认 5 秒）")
	ddnsHost := fs.String("ddns", os.Getenv("DDNS_HOST"), "公网 DDNS 域名或固定 IPv6 地址，用于生成扫码配对链接及外部直连")
	enableSSL := fs.Bool("ssl", isSSLEnabled(), "是否开启 SSL/HTTPS 模式（默认 false，开启需配合 -tls-cert 与 -tls-key）")
	tlsCertEnv := os.Getenv("MULTIGRAVITY_TLS_CERT")
	if tlsCertEnv == "" {
		tlsCertEnv = os.Getenv("TLS_CERT_FILE")
	}
	tlsCert := fs.String("tls-cert", tlsCertEnv, "HTTPS 服务 TLS 证书文件路径 (.cer/.crt/.pem)")
	tlsKeyEnv := os.Getenv("MULTIGRAVITY_TLS_KEY")
	if tlsKeyEnv == "" {
		tlsKeyEnv = os.Getenv("TLS_KEY_FILE")
	}
	tlsKey := fs.String("tls-key", tlsKeyEnv, "HTTPS 服务 TLS 私钥文件路径 (.key)")
	_ = fs.Parse(args)

	if tokPath, _, err := auth.EnsureAdminToken(true); err != nil {
		log.Fatalf("❌ Failed to initialize MULTIGRAVITY_ADMIN_TOKEN: %v", err)
	} else if tokPath != "" {
		log.Printf("🔐 Loaded MULTIGRAVITY_ADMIN_TOKEN from %s", tokPath)
	}
	hasTLSFiles := *tlsCert != "" && *tlsKey != ""
	if *enableSSL && !hasTLSFiles {
		log.Fatalf("MULTIGRAVITY_SSL=1 requires MULTIGRAVITY_TLS_CERT and MULTIGRAVITY_TLS_KEY")
	}
	if *enableSSL && strings.TrimSpace(*ddnsHost) == "" {
		log.Printf("⚠️  MULTIGRAVITY_SSL=1 without DDNS_HOST: pairing will advertise https:// to an IP and iOS certificate checks will fail. Set DDNS_HOST=agy.example.com")
	}
	if !auth.IsListenAddrLoopback(*host) && !hasTLSFiles {
		log.Printf("⚠️  Gateway listening on a non-loopback address without TLS. LAN HTTP is supported; do not advertise this port on the public Internet. Set MULTIGRAVITY_TLS_CERT/MULTIGRAVITY_TLS_KEY or MULTIGRAVITY_SSL=1 for public access.")
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
	authHandler := auth.NewAuthHandler(authStore, pairingMgr, qrHost, *port, *enableSSL)
	authHandler.SetEndpoints(netAddrs.LANIPv4, "", "")

	qrPort := *port

	// 3.5. Initialize Automated Cloudflare Tunnel (Exclusive HTTPS Domain)
	cfCfg := config.GetCloudflareConfig()
	var cfTunnel *tunnel.CloudflareTunnel
	if cfCfg.Enabled {
		cfCtx, cancelCF := context.WithCancel(context.Background())
		defer cancelCF()

		binPath, err := tunnel.EnsureCloudflaredBinary(cfCtx)
		if err != nil {
			log.Printf("⚠️  Cloudflare 穿透引擎准备失败: %v", err)
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
				log.Printf("⚠️  Cloudflare 隧道注册失败: %v", err)
			} else if cfRes != nil {
				cfTunnel = tunnel.NewCloudflareTunnel(cfRes, &cfCfg)
				if err := cfTunnel.Start(cfCtx, binPath); err != nil {
					log.Printf("⚠️  启动 cloudflared 失败: %v", err)
				} else {
					defer cfTunnel.Stop()
					authHandler.SetCloudflareURL(cfRes.URL)
					authHandler.SetPrimary(cfRes.Subdomain, 443, true)
					log.Printf("☁️  Cloudflare 专属域名已生成")

					// 将专属 HTTPS 域名设为二维码主地址，强制走 HTTPS 443！
					qrHost = cfRes.Subdomain
					qrPort = 443
					*enableSSL = true
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
		TunnelEnabled:  cfTunnel != nil,
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
		if *enableSSL && *tlsCert != "" && *tlsKey != "" {
			log.Printf("🔒 TLS enabled with cert=%s key=%s", *tlsCert, *tlsKey)
			err = server.ServeTLS(listener, *tlsCert, *tlsKey)
		} else {
			err = server.Serve(listener)
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	localScheme := "http"
	if *tlsCert != "" && *tlsKey != "" {
		localScheme = "https"
	}
	lanDisplay := netAddrs.LANIPv4
	if lanDisplay == "" && qrHost != "127.0.0.1" && !strings.Contains(qrHost, ":") {
		lanDisplay = qrHost
	}
	if lanDisplay != "" {
		log.Printf("📱 Mobile Web UI ready at: %s://%s:%d (LAN) | %s://127.0.0.1:%d (Local)", localScheme, lanDisplay, *port, localScheme, *port)
	} else if qrHost != "127.0.0.1" {
		log.Printf("📱 Mobile Web UI ready at: %s://%s:%d (WAN/IPv6) | %s://127.0.0.1:%d (Local)", localScheme, qrHost, *port, localScheme, *port)
	} else {
		log.Printf("📱 Mobile Web UI ready at: %s://127.0.0.1:%d", localScheme, *port)
	}

	// Settle briefly so asynchronous startup logs (e.g. Cloudflare tunnel connect, baseline sync)
	// are printed in the boot logs area before rendering the QR code.
	time.Sleep(150 * time.Millisecond)

	// 默认打印配对二维码（网关已确认启动就绪，输出配对二维码供新客户端接入）
	if *printQR {
		if initialSession, err := pairingMgr.GenerateSession(5 * time.Minute); err == nil {
			if authStore.HasDevices() {
				log.Printf("ℹ️  检测到已有 %d 台已配对设备，打印一次新配对二维码供新客户端接入（可通过 -qr=false 关闭）", len(authStore.ListDevices()))
			}
			auth.PrintPairingQRCode(qrHost, qrPort, initialSession.Code, *enableSSL, extraHosts...)
		} else {
			log.Printf("⚠️  无法生成初始配对二维码: %v", err)
		}
	} else {
		log.Printf("ℹ️  已跳过启动配对二维码打印（已指定 -qr=false；如需配对可执行 `make pair`）")
	}

	<-stopCh
	log.Println("Shutting down gateway...")

	if cfTunnel != nil {
		cfTunnel.Stop()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("Gateway stopped gracefully.")
}

func runVersionCmd() {
	fmt.Printf("Multigravity (mgy) %s\n", Version)
}

func runHelpCmd() {
	fmt.Printf(`Multigravity (mgy) %s - Unified Mobile Gateway for Antigravity

用法:
  mgy [子命令] [参数]

常用子命令:
  run (默认)        启动网关服务 (双栈监听 + 自动识别局域网与公网 IPv6 复合配对)
  pair              向正在运行的网关申请并打印新配对二维码与链接 (支持 -ipv6)
  list              查看所有已配对授权的移动设备 (支持在线与离线查看)
  clear [all|id]    清除已配对的设备授权 (支持: mgy clear all 或 mgy clear <device-id>)
  version           查看当前版本信息
  help              显示帮助信息

网关运行参数 (用于 mgy 或 mgy run):
  -port <端口号>    HTTP/WebSocket 监听端口 (默认: 58900, 环境变量: MULTIGRAVITY_PORT)
  -host <主机/IP>   监听地址 (默认: "" 双栈全网卡监听; 设为 127.0.0.1 仅限本机)
  -ipv6             优先使用公网 IPv6 作为配对二维码主地址 (默认包含 IPv6 复合码)
  -qr=<true|false>  启动时是否打印配对二维码 (默认: true)
  -poll <秒数>      Antigravity 实例轮询间隔 (默认: 5秒)
  -ddns <域名/IP>   公网 DDNS 域名或固定 IPv6 地址
  -ssl              启用 HTTPS 模式 (需配置 -tls-cert 与 -tls-key)
`, Version)
}

func runPairCmd(args []string) {
	fs := flag.NewFlagSet("pair", flag.ExitOnError)
	port := defaultPort()
	portFlag := fs.Int("port", port, "网关端口")
	ipv6Flag := fs.Bool("ipv6", false, "是否优先生成纯公网 IPv6 配对二维码与链接")
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

	sslOn := isSSLEnabled()
	schemes := []string{"http", "https"}
	if sslOn {
		schemes = []string{"https", "http"}
	}

	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	var resp *http.Response
	var lastErr error

	for _, s := range schemes {
		u := fmt.Sprintf("%s://127.0.0.1:%d/api/v1/auth/session", s, targetPort)
		if *ipv6Flag {
			u += "?prefer=ipv6"
		}
		req, err := http.NewRequest(http.MethodPost, u, nil)
		if err != nil {
			continue
		}
		if adminToken != "" {
			req.Header.Set("Authorization", "Bearer "+adminToken)
		}
		r, err := client.Do(req)
		if err == nil {
			resp = r
			break
		}
		lastErr = err
	}

	if resp == nil {
		fmt.Fprintf(os.Stderr, "❌ 无法连接到网关 (端口 %d): %v\n", targetPort, lastErr)
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

	scheme := "http"
	if isSSLEnabled() {
		scheme = "https"
	}
	urlStr := fmt.Sprintf("%s://127.0.0.1:%d/api/v1/devices", scheme, targetPort)

	var devices []auth.PairedDevice
	mode := "离线模式 (直接读取本地凭据)"

	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
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

	scheme := "http"
	if isSSLEnabled() {
		scheme = "https"
	}
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	urlStr := fmt.Sprintf("%s://127.0.0.1:%d/api/v1/devices/%s", scheme, targetPort, target)
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
			path == "/manifest.json" || path == "/sw.js" || path == "/favicon.ico" || strings.HasPrefix(path, "/icons/") {
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
