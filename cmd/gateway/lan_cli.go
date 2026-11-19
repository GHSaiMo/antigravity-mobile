package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"antigravity-mobile/internal/auth"
	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/i18n"
)

// runLanCmd handles the interactive or direct CLI command to inspect and configure LAN trust policy.
func runLanCmd(args []string) {
	if len(args) == 0 {
		runLanInteractiveWizard()
		return
	}

	switch strings.ToLower(args[0]) {
	case "status", "info", "get":
		runLanStatus()
	case "trust", "enable", "on", "1":
		applyLanPolicy(true)
	case "pair", "disable", "off", "0":
		applyLanPolicy(false)
	case "help", "-h", "--help":
		runLanHelp()
	default:
		fmt.Fprint(os.Stderr, i18n.Sprintf("❌ 未知参数: %s\n\n", "❌ Unknown argument: %s\n\n", args[0]))
		runLanHelp()
		os.Exit(1)
	}
}

func runLanInteractiveWizard() {
	currentTrust := config.GetTrustLAN()
	netAddrs := auth.DetectNetworkAddresses()
	lanIP := netAddrs.LANIPv4
	if lanIP == "" {
		lanIP = i18n.T("未探测到 (请检查局域网连接)", "Not detected (check LAN connection)")
	}

	fmt.Println()
	fmt.Println(i18n.T("🌐 Multigravity 局域网配对安全策略配置", "🌐 Multigravity LAN Pairing Security Policy Configuration"))
	fmt.Println("--------------------------------------------------")
	fmt.Print(i18n.Sprintf("➜  当前局域网 IP:   %s\n", "➜  Current LAN IP:   %s\n", lanIP))
	if currentTrust {
		fmt.Println(i18n.T("➜  当前安全策略:   局域网已信任免密放行 (Trust LAN: 免配对码)",
			"➜  Current Policy:   LAN Trusted without pairing (Trust LAN: No pairing code)"))
	} else {
		fmt.Println(i18n.T("➜  当前安全策略:   局域网标准安全配对 (Standard Pair: 需扫码/配对码)",
			"➜  Current Policy:   LAN Standard Pairing (Standard Pair: QR code or pairing code required)"))
	}
	fmt.Println("--------------------------------------------------")
	fmt.Println(i18n.T("请选择要设定的局域网访问策略:", "Select LAN access security policy:"))
	fmt.Println(i18n.T("  [1] 局域网已信任免密放行 (Trust LAN) - 同一 Wi-Fi/以太网设备免配对直接访问",
		"  [1] Trust LAN without pairing (Trust LAN) - Direct access for devices on the same Wi-Fi/Ethernet"))
	fmt.Println(i18n.T("  [2] 局域网标准安全配对 (Standard Pair) - 首次连接必须通过二维码或配对码授权 (推荐)",
		"  [2] LAN Standard Pairing (Standard Pair) - Authorization via QR or pairing code required (Recommended)"))
	fmt.Println(i18n.T("  [q] 退出 (保持当前配置不变)",
		"  [q] Quit (Keep current configuration)"))
	fmt.Print(i18n.T("\n请输入选项 [1/2/q]: ", "\nEnter choice [1/2/q]: "))

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	switch input {
	case "1":
		applyLanPolicy(true)
	case "2":
		applyLanPolicy(false)
	case "q", "Q", "exit", "":
		fmt.Println(i18n.T("已取消操作，配置保持不变。", "Operation cancelled, configuration unchanged."))
	default:
		fmt.Println(i18n.T("❌ 无效选项，已退出。", "❌ Invalid choice, exited."))
	}
}

func applyLanPolicy(trust bool) {
	val := "0"
	desc := i18n.T("局域网标准安全配对 (需配对码)", "LAN Standard Pairing (Pairing code required)")
	if trust {
		val = "1"
		desc = i18n.T("局域网已信任免密放行 (免配对)", "Trust LAN without pairing (No pairing code)")
	}

	path, err := config.UpdateEnvVariables(map[string]string{
		"MULTIGRAVITY_TRUST_LAN": val,
	})
	if err != nil {
		fmt.Fprint(os.Stderr, i18n.Sprintf("❌ 保存配置失败: %v\n", "❌ Failed to save configuration: %v\n", err))
		return
	}

	fmt.Print(i18n.Sprintf("\n✅ 已成功将局域网策略更新为: %s\n", "\n✅ Successfully updated LAN policy to: %s\n", desc))
	fmt.Print(i18n.Sprintf("📁 配置文件: %s\n", "📁 Config file: %s\n", path))
	fmt.Println(i18n.T("💡 提示: 下次启动 `mgy` 将默认使用此策略；若网关已在运行，重启后立即生效。",
		"💡 Tip: Next launch of `mgy` will use this policy; if gateway is running, restart to apply."))
}

func runLanStatus() {
	currentTrust := config.GetTrustLAN()
	netAddrs := auth.DetectNetworkAddresses()
	lanIP := netAddrs.LANIPv4
	if lanIP == "" {
		lanIP = "-"
	}

	fmt.Println()
	fmt.Println(i18n.T("🌐 Multigravity 局域网安全策略状态", "🌐 Multigravity LAN Security Policy Status"))
	fmt.Println("--------------------------------------------------")
	fmt.Print(i18n.Sprintf("  局域网 IP:      %s\n", "  LAN IP:         %s\n", lanIP))
	if currentTrust {
		fmt.Println(i18n.T("  配对模式:       已信任免密放行 (MULTIGRAVITY_TRUST_LAN=1)",
			"  Pairing Mode:   Trusted without pairing (MULTIGRAVITY_TRUST_LAN=1)"))
		fmt.Println(i18n.T("  说明:           局域网内任意客户端均可直接接入控制台，无需扫码配对。",
			"  Description:    Any client in the LAN can access the console directly without pairing."))
	} else {
		fmt.Println(i18n.T("  配对模式:       标准安全配对 (MULTIGRAVITY_TRUST_LAN=0)",
			"  Pairing Mode:   Standard Pairing (MULTIGRAVITY_TRUST_LAN=0)"))
		fmt.Println(i18n.T("  说明:           局域网设备首次访问必须在终端执行 `mgy pair` 扫码或输入配对码。",
			"  Description:    LAN devices must run `mgy pair` on the terminal to pair on first access."))
	}
	fmt.Println("--------------------------------------------------")
	fmt.Println(i18n.T("  💡 切换策略: 执行 `mgy lan` 可进入交互式切换。",
		"  💡 Switch Policy: Run `mgy lan` to switch interactively."))
	fmt.Println()
}

func runLanHelp() {
	if i18n.IsZh() {
		fmt.Print(`🌐 Multigravity (mgy) - 局域网访问与配对安全策略配置

用法:
  mgy lan               交互式选择并切换局域网安全策略 (Trust LAN / Standard Pair)
  mgy lan status        查看当前局域网 IP 与配对策略状态
  mgy lan trust (或 on) 切换为局域网免配对信任放行
  mgy lan pair (或 off) 切换为局域网标准安全配对 (需配对码)
  mgy lan help          显示此帮助信息
`)
	} else {
		fmt.Print(`🌐 Multigravity (mgy) - LAN Access & Pairing Security Policy

Usage:
  mgy lan               Interactively choose and switch LAN security policy (Trust LAN / Standard Pair)
  mgy lan status        View current LAN IP and pairing policy status
  mgy lan trust (or on) Enable LAN trust without pairing
  mgy lan pair (or off) Enable standard secure pairing (pairing code required)
  mgy lan help          Show this help message
`)
	}
}
