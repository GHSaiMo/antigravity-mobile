package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"antigravity-mobile/internal/auth"
	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/hub"
	"antigravity-mobile/internal/i18n"
	"antigravity-mobile/internal/localtls"
)

type gatewayMode string

const (
	modeStandalone gatewayMode = "standalone"
	modeHub        gatewayMode = "hub"
	modeNode       gatewayMode = "node"
)

type hubLaunchConfig struct {
	Mode             gatewayMode
	Headless         bool
	HubName          string
	NodeName         string
	Secret           string
	JoinCode         string
	HubURL           string
	StaticNodes      []string
	DiscoveryPort    int
	DisableDiscovery bool
}

func runHubCmd(args []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		switch strings.ToLower(args[0]) {
		case "status", "nodes", "ls", "list":
			runHubStatusCmd(args[1:], false)
			return
		case "scan", "discover":
			runHubStatusCmd(args[1:], true)
			return
		case "pair":
			runHubPairCmd(args[1:])
			return
		case "links":
			runHubLinksCmd(args[1:])
			return
		case "unlink":
			runHubUnlinkCmd(args[1:])
			return
		case "add":
			runHubAddCmd(args[1:])
			return
		case "remove", "rm", "del", "delete":
			runHubRemoveCmd(args[1:])
			return
		case "filter", "use", "switch":
			runHubFilterCmd(args[1:])
			return
		case "secret":
			runHubSecretCmd(args[1:])
			return
		case "help", "-h", "--help":
			printHubHelp()
			return
		case "run", "start":
			runGatewayServerWithMode(modeHub, args[1:])
			return
		}
	}
	runGatewayServerWithMode(modeHub, args)
}

func runNodeCmd(args []string) {
	if len(args) > 0 && (args[0] == "help" || args[0] == "-h" || args[0] == "--help") {
		printHubHelp()
		return
	}
	runGatewayServerWithMode(modeNode, args)
}

func printHubHelp() {
	if i18n.IsZh() {
		fmt.Printf(`Multigravity Hub 多机聚合模式 (mgy hub / mgy node)

用法:
  mgy hub [参数]                            启动统一 Hub 网关（启动时自动扫描局域网 mgy 设备）
  mgy hub scan [-port N]                    主动扫描局域网中运行中的普通 mgy 与节点设备
  mgy hub pair <名称|IP|URL> [-code 6位码]  向局域网普通 mgy 发起二次配对码接入（首次需输入 6 位确认码）
  mgy hub status [-port N]                  查看 Hub 集群状态、加入码与所有已接入/待配对设备
  mgy hub nodes [-port N]                   同 status，列出已接入节点与局域网发现的待配对设备
  mgy hub add <url> [-name 名称]            手动添加静态局域网节点
  mgy hub remove <id|名称>                  从 Hub 移除指定节点
  mgy hub filter <all|local|节点>           切换移动端看到的节点视图（立即同步刷新）
  mgy hub secret                            查看集群共享密钥与免交互加入命令

  被接入主机管理 (在普通 mgy 机器上执行):
  mgy pair -hub [-port N]                   签发 6 位局域网 Hub 确认码（适合后台/tmux 运行场景）
  mgy hub links [-port N]                   查看已授权接入本机的 Hub 列表
  mgy hub unlink <hub_id|名称> [-port N]    吊销指定 Hub 对本机的接入授权

  mgy node [参数]                           以纯子节点模式启动 mgy 并接入 Hub（默认关闭自身公网隧道与二维码）

Hub 启动参数 (mgy hub):
  -headless                 无头模式（不要求本机运行 Antigravity，适合 NAS / 常驻主机）
  -name <名称>              Hub 显示名称（默认取本机主机名）
  -secret <密钥>            集群共享密钥（局域网同密钥 mgy node 自动免密加入，环境变量: MGY_HUB_SECRET）
  -nodes <列表>             逗号分隔静态节点列表，如 M1=http://192.168.50.12:58900,M2=http://192.168.50.13:58900
  -discovery-port <端口>    局域网 UDP 发现端口（默认: 58901）
  -no-discovery             关闭局域网 UDP 广播发现（环境变量: MGY_DISABLE_DISCOVERY=1）

Node 启动参数 (mgy node):
  -hub <URL>                Hub 局域网地址，如 http://192.168.50.10:58900（同密钥时可省略自动发现）
  -code <加入码>            Hub 启动看板打印的 6 位加入码
  -secret <密钥>            与 Hub 一致的集群共享密钥（环境变量: MGY_HUB_SECRET）
  -name <名称>              节点显示名称，会话与项目将自动标注为 [名称]（默认取主机名）
`)
		return
	}
	fmt.Printf(`Multigravity Hub Multi-Host Aggregation (mgy hub / mgy node)

Usage:
  mgy hub [flags]                           Start unified Hub gateway (auto-scans LAN mgy hosts on startup)
  mgy hub scan [-port N]                    Actively scan LAN for running mgy hosts and nodes
  mgy hub pair <name|IP|URL> [-code CODE]   Pair a discovered LAN mgy host using a 6-char confirmation code
  mgy hub status [-port N]                  Show Hub status, join code, registered nodes, and LAN discoveries
  mgy hub nodes [-port N]                   Alias for status
  mgy hub add <url> [-name NAME]            Add a static LAN node to the Hub
  mgy hub remove <id|name>                  Remove a node from the Hub
  mgy hub filter <all|local|node>           Switch active node filter (instantly pushes refresh)
  mgy hub secret                            Print cluster secret and one-line node join command

  Host-Side Hub Authorization (run on an ordinary mgy machine):
  mgy pair -hub [-port N]                   Issue a 6-char Hub confirmation code (ideal for background/tmux hosts)
  mgy hub links [-port N]                   List Hubs authorized to connect to this host
  mgy hub unlink <hub_id|name> [-port N]    Revoke a Hub's authorization on this host

  mgy node [flags]                          Start mgy as a dedicated LAN node and join a Hub

Hub Flags (mgy hub):
  -headless                 Run without local Antigravity LS (ideal for NAS / always-on host)
  -name <name>              Hub display name (default: hostname)
  -secret <secret>          Cluster secret for zero-touch LAN mgy node auto-join (env: MGY_HUB_SECRET)
  -nodes <list>             Comma-separated static nodes, e.g. M1=http://192.168.50.12:58900
  -discovery-port <port>    LAN UDP discovery port (default: 58901)
  -no-discovery             Disable UDP LAN discovery (env: MGY_DISABLE_DISCOVERY=1)

Node Flags (mgy node):
  -hub <url>                Hub LAN URL, e.g. http://192.168.50.10:58900 (auto-discovered if secret matches)
  -code <join-code>         6-char join code shown on Hub startup banner
  -secret <secret>          Shared cluster secret matching Hub (env: MGY_HUB_SECRET)
  -name <name>              Node display name prefixed as [name] in lists (default: hostname)
`)
}

func loadAdminTokenForCLI() string {
	tok := auth.GetAdminToken()
	if tok == "" {
		if path, _, err := auth.EnsureAdminToken(false); err == nil && path != "" {
			if b, err := os.ReadFile(path); err == nil {
				tok = strings.TrimSpace(string(b))
			}
		}
	}
	return tok
}

func hubAdminHTTPClient() *http.Client {
	return &http.Client{
		Timeout:   8 * time.Second,
		Transport: localtls.NewLoopbackTransport(),
	}
}

func runHubStatusCmd(args []string, forceScan bool) {
	name := "hub status"
	if forceScan {
		name = "hub scan"
	}
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	portFlag := fs.Int("port", defaultPort(), i18n.T("Hub 监听端口", "Hub port"))
	probeFlag := fs.Bool("probe", true, i18n.T("实时探测节点健康状态", "Probe node health before printing"))
	scanFlag := fs.Bool("scan", forceScan, i18n.T("主动发起局域网 UDP 发现扫描", "Actively broadcast LAN UDP discovery scan"))
	_ = fs.Parse(args)

	params := url.Values{}
	if *probeFlag {
		params.Set("probe", "1")
	}
	if *scanFlag || forceScan {
		params.Set("scan", "1")
		fmt.Println(i18n.T("🔍 正在扫描局域网内的 mgy 设备...", "🔍 Scanning LAN for running mgy hosts..."))
	}
	q := ""
	if enc := params.Encode(); enc != "" {
		q = "?" + enc
	}
	u := fmt.Sprintf("http://127.0.0.1:%d/gateway/hub/status%s", *portFlag, q)
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	if tok := loadAdminTokenForCLI(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := hubAdminHTTPClient().Do(req)
	if err != nil {
		// Offline fallback: read persisted ~/.multigravity/hub_nodes.json
		store, sErr := hub.NewStore(config.GetDataDir(), "")
		if sErr != nil {
			fmt.Fprintf(os.Stderr, i18n.T("❌ 无法连接到运行中的 Hub (端口 %d): %v\n", "❌ Unable to connect to running Hub (port %d): %v\n"), *portFlag, err)
			os.Exit(1)
		}
		nodes := store.ListNodes()
		fmt.Printf(i18n.T("ℹ️  Hub 当前未在端口 %d 运行（离线配置，共 %d 个已保存节点）\n", "ℹ️  Hub is not running on port %d (offline config, %d saved node(s))\n"), *portFlag, len(nodes))
		for _, n := range nodes {
			fmt.Printf("  - %s (%s) -> %s\n", n.Name, n.ID, n.BaseURL)
		}
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, i18n.T("❌ Hub 状态查询失败 (HTTP %d): %s\n", "❌ Hub status query failed (HTTP %d): %s\n"), resp.StatusCode, strings.TrimSpace(string(body)))
		os.Exit(1)
	}

	var st hub.HubStatusResponse
	if err := json.Unmarshal(body, &st); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Invalid response: %v\n", err)
		os.Exit(1)
	}

	modeStr := "Local + LAN"
	if st.Headless {
		modeStr = "Headless"
	}
	fmt.Println("==========================================================================================")
	fmt.Printf("  Multigravity Hub | %s (%s) | Filter: %s | Join Code: %s\n", st.HubName, modeStr, st.ActiveFilter, st.JoinCode)
	fmt.Println("==========================================================================================")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NODE ID\tNAME\tSTATUS\tLS\tLATENCY\tURL")
	fmt.Fprintln(w, "-------\t----\t------\t--\t-------\t---")
	for _, n := range st.Nodes {
		status := "offline"
		if n.Online {
			status = "online"
		}
		ls := "down"
		if n.LSHealthy {
			ls = "healthy"
		}
		lat := "-"
		if n.Local {
			lat = "local"
		} else if n.Online {
			lat = fmt.Sprintf("%dms", n.LatencyMs)
		}
		uStr := n.BaseURL
		if n.Local {
			uStr = "(local)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", n.ID, n.Name, status, ls, lat, uStr)
	}
	_ = w.Flush()

	if len(st.Discovered) > 0 {
		fmt.Println(i18n.T("\n  局域网发现 (待二次确认码配对):", "\n  LAN Discovered (awaiting confirmation code pairing):"))
		for _, d := range st.Discovered {
			modeTag := d.Mode
			if modeTag == "" {
				modeTag = "mgy"
			}
			if d.RequiresPairCode || !d.HasSecretMatch {
				fmt.Printf("    • %s [%s | id=%s] -> %s  [pair: mgy hub pair %s]\n", d.Name, modeTag, d.NodeID, d.BaseURL, d.Name)
			} else {
				fmt.Printf("    • %s [%s | id=%s] -> %s  [approve: mgy hub add %s -name %s]\n", d.Name, modeTag, d.NodeID, d.BaseURL, d.BaseURL, d.Name)
			}
		}
	} else if *scanFlag || forceScan {
		fmt.Println(i18n.T("\n  未发现新的局域网 mgy 设备（请确认其他电脑已运行 mgy 且在同一局域网未开启 AP 隔离）。",
			"\n  No new LAN mgy hosts discovered (ensure other hosts run mgy on the same LAN without AP isolation)."))
	}
	fmt.Println("==========================================================================================")
}

func runHubPairCmd(args []string) {
	fs := flag.NewFlagSet("hub pair", flag.ExitOnError)
	portFlag := fs.Int("port", defaultPort(), i18n.T("Hub 监听端口", "Hub port"))
	codeFlag := fs.String("code", "", i18n.T("直接提交 6 位确认码（适合配合目标机 `mgy pair -hub` 非交互使用）", "Submit 6-char confirmation code directly (non-interactive)"))
	nameFlag := fs.String("name", "", i18n.T("自定义该节点在 Hub 中的显示名称（可选）", "Optional custom display name for this node"))
	_ = fs.Parse(args)

	target := strings.TrimSpace(fs.Arg(0))
	if target == "" {
		fmt.Fprintln(os.Stderr, i18n.T(
			"用法: mgy hub pair <名称|节点ID|局域网IP|URL> [-code 6位确认码] [-name 显示名]",
			"Usage: mgy hub pair <name|node-id|lan-ip|url> [-code 6-char-code] [-name DISPLAY-NAME]",
		))
		os.Exit(1)
	}

	adminTok := loadAdminTokenForCLI()
	client := hubAdminHTTPClient()

	// Fast path: user already obtained a 6-char code via `mgy pair -hub` on the target host.
	if presetCode := hub.NormalizePairCode(*codeFlag); presetCode != "" {
		cfg, _, err := confirmHubPairViaAPI(client, *portFlag, adminTok, hub.HubPairConfirmInput{
			Target: target,
			Code:   presetCode,
			Name:   strings.TrimSpace(*nameFlag),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, i18n.T("❌ 配对失败: %v\n", "❌ Pairing failed: %v\n"), err)
			os.Exit(1)
		}
		fmt.Printf(i18n.T("✅ 已成功配对并接入节点 %s (%s) -> %s\n", "✅ Paired and connected node %s (%s) -> %s\n"), cfg.Name, cfg.ID, cfg.BaseURL)
		return
	}

	// Default interactive AirPlay-style flow: trigger a 6-char confirmation prompt on the target host's terminal.
	startBody, _ := json.Marshal(map[string]string{"target": target})
	startURL := fmt.Sprintf("http://127.0.0.1:%d/gateway/hub/pair/start", *portFlag)
	startReq, _ := http.NewRequest(http.MethodPost, startURL, bytes.NewReader(startBody))
	startReq.Header.Set("Content-Type", "application/json")
	if adminTok != "" {
		startReq.Header.Set("Authorization", "Bearer "+adminTok)
	}
	startResp, err := client.Do(startReq)
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T("❌ 无法连接到运行中的 Hub (端口 %d): %v\n", "❌ Unable to connect to running Hub (port %d): %v\n"), *portFlag, err)
		os.Exit(1)
	}
	rawStart, _ := io.ReadAll(io.LimitReader(startResp.Body, 64*1024))
	startResp.Body.Close()
	if startResp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, i18n.T("❌ 发起局域网配对请求失败 (HTTP %d): %s\n", "❌ Failed to start LAN pairing request (HTTP %d): %s\n"), startResp.StatusCode, extractJSONError(rawStart))
		os.Exit(1)
	}

	var started hub.HubPairStartResult
	if err := json.Unmarshal(rawStart, &started); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Invalid response: %v\n", err)
		os.Exit(1)
	}

	displayTarget := started.NodeName
	if displayTarget == "" {
		displayTarget = target
	}
	fmt.Printf(i18n.T(
		"🔐 已向 %s (%s) 发送接入请求（有效期 %d 秒）\n   请查看对方 mgy 终端显示的 6 位确认码（若对方在后台运行，可在对方执行 `mgy pair -hub` 获取）。\n",
		"🔐 Pairing request sent to %s (%s) (valid %ds)\n   Check the 6-char confirmation code shown on the target mgy terminal (or run `mgy pair -hub` there if running in background).\n",
	), displayTarget, started.BaseURL, started.ExpiresIn)

	reader := bufio.NewReader(os.Stdin)
	for attempt := 1; attempt <= 3; attempt++ {
		fmt.Print(i18n.T("请输入 6 位确认码: ", "Enter 6-char confirmation code: "))
		line, readErr := reader.ReadString('\n')
		code := hub.NormalizePairCode(line)
		if code == "" {
			if readErr != nil {
				fmt.Fprintln(os.Stderr, i18n.T("\n❌ 已取消配对。", "\n❌ Pairing cancelled."))
				os.Exit(1)
			}
			continue
		}

		cfg, remaining, cErr := confirmHubPairViaAPI(client, *portFlag, adminTok, hub.HubPairConfirmInput{
			BaseURL:     started.BaseURL,
			ChallengeID: started.ChallengeID,
			Code:        code,
			Name:        strings.TrimSpace(*nameFlag),
		})
		if cErr == nil {
			fmt.Printf(i18n.T("✅ 已成功配对并接入节点 %s (%s) -> %s\n", "✅ Paired and connected node %s (%s) -> %s\n"), cfg.Name, cfg.ID, cfg.BaseURL)
			return
		}
		if remaining > 0 {
			fmt.Fprintf(os.Stderr, i18n.T("⚠️  确认码不正确，还剩 %d 次机会。\n", "⚠️  Incorrect confirmation code (%d attempt(s) remaining).\n"), remaining)
			continue
		}
		fmt.Fprintf(os.Stderr, i18n.T("❌ 配对失败: %v\n", "❌ Pairing failed: %v\n"), cErr)
		os.Exit(1)
	}
	os.Exit(1)
}

func confirmHubPairViaAPI(client *http.Client, port int, adminTok string, in hub.HubPairConfirmInput) (hub.NodeConfig, int, error) {
	payload, _ := json.Marshal(in)
	u := fmt.Sprintf("http://127.0.0.1:%d/gateway/hub/pair/confirm", port)
	req, _ := http.NewRequest(http.MethodPost, u, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if adminTok != "" {
		req.Header.Set("Authorization", "Bearer "+adminTok)
	}
	resp, err := client.Do(req)
	if err != nil {
		return hub.NodeConfig{}, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		var errDoc struct {
			Error             string `json:"error"`
			AttemptsRemaining int    `json:"attempts_remaining"`
		}
		_ = json.Unmarshal(body, &errDoc)
		msg := errDoc.Error
		if msg == "" {
			msg = strings.TrimSpace(string(body))
		}
		return hub.NodeConfig{}, errDoc.AttemptsRemaining, fmt.Errorf("%s", msg)
	}
	var okDoc struct {
		Node hub.NodeConfig `json:"node"`
	}
	if err := json.Unmarshal(body, &okDoc); err != nil {
		return hub.NodeConfig{}, 0, err
	}
	return okDoc.Node, 0, nil
}

func extractJSONError(body []byte) string {
	var parsed struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Error != "" {
		return parsed.Error
	}
	return strings.TrimSpace(string(body))
}

func runHubLinksCmd(args []string) {
	fs := flag.NewFlagSet("hub links", flag.ExitOnError)
	portFlag := fs.Int("port", defaultPort(), i18n.T("本机网关端口", "Local gateway port"))
	_ = fs.Parse(args)

	u := fmt.Sprintf("http://127.0.0.1:%d/api/v1/hub/links", *portFlag)
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	if tok := loadAdminTokenForCLI(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := hubAdminHTTPClient().Do(req)
	if err != nil {
		// Offline fallback: read ~/.multigravity/hub_links.json
		links, lErr := hub.LoadHubLinks(config.GetDataDir())
		if lErr != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
		if len(links) == 0 {
			fmt.Println(i18n.T("ℹ️  [离线配置] 本机尚未授权任何 Hub 接入。", "ℹ️  [Offline] No Hubs are currently authorized on this host."))
			return
		}
		fmt.Printf(i18n.T("ℹ️  [离线配置] 本机已授权 %d 个 Hub 接入:\n", "ℹ️  [Offline] %d authorized Hub(s) on this host:\n"), len(links))
		printHubLinksTable(links)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "❌ HTTP %d: %s\n", resp.StatusCode, extractJSONError(body))
		os.Exit(1)
	}
	var out struct {
		NodeID   string            `json:"node_id"`
		NodeName string            `json:"node_name"`
		Links    []hub.HubLinkView `json:"links"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Invalid response: %v\n", err)
		os.Exit(1)
	}
	if len(out.Links) == 0 {
		fmt.Printf(i18n.T("ℹ️  本机 (%s) 尚未授权任何局域网 Hub 接入。\n", "ℹ️  Host (%s) has no authorized LAN Hubs.\n"), out.NodeName)
		return
	}
	links := make([]hub.HubLink, 0, len(out.Links))
	for _, v := range out.Links {
		links = append(links, hub.HubLink{
			HubID:      v.HubID,
			HubName:    v.HubName,
			HubURL:     v.HubURL,
			PairedAt:   v.PairedAt,
			LastSeenAt: v.LastSeenAt,
		})
	}
	fmt.Printf(i18n.T("本机 (%s / %s) 已授权接入的 Hub:\n", "Authorized Hubs for %s (%s):\n"), out.NodeName, out.NodeID)
	printHubLinksTable(links)
}

func printHubLinksTable(links []hub.HubLink) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "HUB ID\tNAME\tHUB URL\tPAIRED AT")
	fmt.Fprintln(w, "------\t----\t-------\t---------")
	for _, lk := range links {
		hubURL := lk.HubURL
		if hubURL == "" {
			hubURL = "-"
		}
		paired := "-"
		if !lk.PairedAt.IsZero() {
			paired = lk.PairedAt.Local().Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", lk.HubID, lk.HubName, hubURL, paired)
	}
	_ = w.Flush()
}

func runHubUnlinkCmd(args []string) {
	fs := flag.NewFlagSet("hub unlink", flag.ExitOnError)
	portFlag := fs.Int("port", defaultPort(), i18n.T("本机网关端口", "Local gateway port"))
	_ = fs.Parse(args)

	target := strings.TrimSpace(fs.Arg(0))
	if target == "" {
		fmt.Fprintln(os.Stderr, i18n.T("用法: mgy hub unlink <hub_id|名称>", "Usage: mgy hub unlink <hub-id-or-name>"))
		os.Exit(1)
	}

	u := fmt.Sprintf("http://127.0.0.1:%d/api/v1/hub/links/%s", *portFlag, url.PathEscape(target))
	req, _ := http.NewRequest(http.MethodDelete, u, nil)
	if tok := loadAdminTokenForCLI(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := hubAdminHTTPClient().Do(req)
	if err != nil {
		removed, ok, rErr := hub.RemoveHubLinkOffline(config.GetDataDir(), target)
		if rErr == nil && ok {
			fmt.Printf(i18n.T("✅ [离线配置] 已吊销 Hub「%s」(%s) 的接入授权。\n", "✅ [Offline] Revoked authorization for Hub %q (%s).\n"), removed.HubName, removed.HubID)
			return
		}
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "❌ HTTP %d: %s\n", resp.StatusCode, extractJSONError(body))
		os.Exit(1)
	}
	fmt.Printf(i18n.T("✅ 已吊销 Hub (%s) 对本机的接入授权（立即生效）。\n", "✅ Revoked authorization for Hub (%s) immediately.\n"), target)
}

func runHubAddCmd(args []string) {
	fs := flag.NewFlagSet("hub add", flag.ExitOnError)
	portFlag := fs.Int("port", defaultPort(), "Hub port")
	nameFlag := fs.String("name", "", "Node display name")
	idFlag := fs.String("id", "", "Node ID")
	tokenFlag := fs.String("token", "", "Node auth token (optional if sharing Hub secret)")
	_ = fs.Parse(args)

	targetURL := strings.TrimSpace(fs.Arg(0))
	if targetURL == "" && *idFlag == "" {
		fmt.Fprintln(os.Stderr, i18n.T("用法: mgy hub add <节点URL> [-name 名称] [-token 令牌]", "Usage: mgy hub add <node-url> [-name NAME] [-token TOKEN]"))
		os.Exit(1)
	}

	payload, _ := json.Marshal(map[string]any{
		"id":    *idFlag,
		"name":  *nameFlag,
		"url":   targetURL,
		"token": *tokenFlag,
	})
	u := fmt.Sprintf("http://127.0.0.1:%d/gateway/hub/nodes", *portFlag)
	req, _ := http.NewRequest(http.MethodPost, u, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if tok := loadAdminTokenForCLI(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := hubAdminHTTPClient().Do(req)
	if err != nil {
		// Offline fallback: write directly to hub_nodes.json
		store, sErr := hub.NewStore(config.GetDataDir(), "")
		if sErr != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
		cfg, uErr := store.UpsertNode(hub.NodeConfig{
			ID:      *idFlag,
			Name:    *nameFlag,
			BaseURL: targetURL,
			Token:   *tokenFlag,
			Static:  true,
			Enabled: true,
		})
		if uErr != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", uErr)
			os.Exit(1)
		}
		fmt.Printf(i18n.T("✅ [离线配置] 已保存节点 %s (%s) -> %s\n", "✅ [Offline] Saved node %s (%s) -> %s\n"), cfg.Name, cfg.ID, cfg.BaseURL)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "❌ HTTP %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		os.Exit(1)
	}
	fmt.Println(i18n.T("✅ 已向 Hub 添加节点并触发探测。", "✅ Node added to Hub and probed."))
}

func runHubRemoveCmd(args []string) {
	fs := flag.NewFlagSet("hub remove", flag.ExitOnError)
	portFlag := fs.Int("port", defaultPort(), "Hub port")
	_ = fs.Parse(args)

	target := strings.TrimSpace(fs.Arg(0))
	if target == "" {
		fmt.Fprintln(os.Stderr, i18n.T("用法: mgy hub remove <节点ID或名称>", "Usage: mgy hub remove <node-id-or-name>"))
		os.Exit(1)
	}

	u := fmt.Sprintf("http://127.0.0.1:%d/gateway/hub/nodes/%s", *portFlag, url.PathEscape(target))
	req, _ := http.NewRequest(http.MethodDelete, u, nil)
	if tok := loadAdminTokenForCLI(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := hubAdminHTTPClient().Do(req)
	if err != nil {
		store, sErr := hub.NewStore(config.GetDataDir(), "")
		if sErr == nil && store.RemoveNode(target) {
			fmt.Printf(i18n.T("✅ [离线配置] 已移除节点: %s\n", "✅ [Offline] Removed node: %s\n"), target)
			return
		}
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "❌ HTTP %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		os.Exit(1)
	}
	fmt.Printf(i18n.T("✅ 已从 Hub 移除节点: %s\n", "✅ Removed node from Hub: %s\n"), target)
}

func runHubFilterCmd(args []string) {
	fs := flag.NewFlagSet("hub filter", flag.ExitOnError)
	portFlag := fs.Int("port", defaultPort(), "Hub port")
	_ = fs.Parse(args)

	target := strings.TrimSpace(fs.Arg(0))
	if target == "" {
		// Show current filter
		u := fmt.Sprintf("http://127.0.0.1:%d/gateway/hub/filter", *portFlag)
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		if tok := loadAdminTokenForCLI(); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		resp, err := hubAdminHTTPClient().Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
		defer resp.Body.Close()
		var out map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&out)
		fmt.Printf("Active Hub Filter: %s\n", out["active_filter"])
		return
	}

	payload, _ := json.Marshal(map[string]string{"node_id": target})
	u := fmt.Sprintf("http://127.0.0.1:%d/gateway/hub/filter", *portFlag)
	req, _ := http.NewRequest(http.MethodPost, u, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if tok := loadAdminTokenForCLI(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := hubAdminHTTPClient().Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "❌ HTTP %d: %s\n", resp.StatusCode, strings.TrimSpace(string(body)))
		os.Exit(1)
	}
	var out map[string]string
	_ = json.Unmarshal(body, &out)
	fmt.Printf(i18n.T("✅ Hub 视图过滤已切换为: %s（已通知客户端刷新）\n", "✅ Hub filter set to: %s (clients notified)\n"), out["active_filter"])
}

func runHubSecretCmd(args []string) {
	store, err := hub.NewStore(config.GetDataDir(), "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
	secret := store.Secret()
	netAddrs := auth.DetectNetworkAddresses()
	lanIP := netAddrs.LANIPv4
	if lanIP == "" {
		lanIP = "<HUB_LAN_IP>"
	}
	fmt.Printf("Hub Cluster Secret: %s\n", secret)
	fmt.Printf("Node Auto-Join Cmd: mgy node -hub http://%s:%d -secret %s\n", lanIP, defaultPort(), secret)
}
