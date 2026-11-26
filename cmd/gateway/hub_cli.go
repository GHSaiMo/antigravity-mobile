package main

import (
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
			runHubStatusCmd(args[1:])
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
  mgy hub [参数]                      启动统一 Hub 网关（聚合本机与局域网多台 mgy 节点）
  mgy hub status [-port N]            查看 Hub 集群状态、加入码与所有节点在线状态
  mgy hub nodes [-port N]             列出已加入节点与局域网自动发现的待加入节点
  mgy hub add <url> [-name 名称]      向运行中的 Hub 动态添加/批准一个局域网节点
  mgy hub remove <id|名称>            从 Hub 移除指定节点
  mgy hub filter <all|local|节点>     切换移动端看到的节点视图（立即同步刷新）
  mgy hub secret                      查看集群共享密钥与免交互加入命令

  mgy node [参数]                     以局域网节点模式启动 mgy，并注册到 Hub

Hub 启动参数 (mgy hub):
  -headless                 无头模式（不要求本机运行 Antigravity，适合 NAS / 常驻主机）
  -name <名称>              Hub 显示名称（默认取本机主机名）
  -secret <密钥>            集群共享密钥（局域网同密钥节点自动发现并免密加入，环境变量: MGY_HUB_SECRET）
  -nodes <列表>             逗号分隔静态节点列表，如 M1=http://192.168.50.12:58900,M2=http://192.168.50.13:58900
  -discovery-port <端口>    局域网 UDP 发现端口（默认: 58901）
  -no-discovery             关闭局域网 UDP 广播发现（仅使用静态节点或主动注册）

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
  mgy hub [flags]                     Start unified Hub gateway (aggregates local + LAN mgy nodes)
  mgy hub status [-port N]            Show Hub status, join code, and online/offline nodes
  mgy hub nodes [-port N]             List joined nodes and LAN-discovered nodes
  mgy hub add <url> [-name NAME]      Add or approve a LAN node on a running Hub
  mgy hub remove <id|name>            Remove a node from the Hub
  mgy hub filter <all|local|node>     Switch active node filter (instantly pushes refresh)
  mgy hub secret                      Print cluster secret and one-line node join command

  mgy node [flags]                    Start mgy as a LAN node and join a Hub

Hub Flags (mgy hub):
  -headless                 Run without local Antigravity LS (ideal for NAS / always-on host)
  -name <name>              Hub display name (default: hostname)
  -secret <secret>          Cluster secret for zero-touch LAN auto-join (env: MGY_HUB_SECRET)
  -nodes <list>             Comma-separated static nodes, e.g. M1=http://192.168.50.12:58900
  -discovery-port <port>    LAN UDP discovery port (default: 58901)
  -no-discovery             Disable UDP LAN discovery

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
		Timeout:   4 * time.Second,
		Transport: localtls.NewLoopbackTransport(),
	}
}

func runHubStatusCmd(args []string) {
	fs := flag.NewFlagSet("hub status", flag.ExitOnError)
	portFlag := fs.Int("port", defaultPort(), i18n.T("Hub 监听端口", "Hub port"))
	probeFlag := fs.Bool("probe", true, i18n.T("实时探测节点健康状态", "Probe node health before printing"))
	_ = fs.Parse(args)

	q := ""
	if *probeFlag {
		q = "?probe=1"
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
		fmt.Println("\n  LAN Discovered (pending approval):")
		for _, d := range st.Discovered {
			fmt.Printf("    • %s (id=%s) -> %s  [approve: mgy hub add %s -name %s]\n", d.Name, d.NodeID, d.BaseURL, d.BaseURL, d.Name)
		}
	}
	fmt.Println("==========================================================================================")
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
