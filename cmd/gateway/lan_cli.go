package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"antigravity-mobile/internal/auth"
	"antigravity-mobile/internal/config"
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
		fmt.Fprintf(os.Stderr, "❌ 未知参数: %s\n\n", args[0])
		runLanHelp()
		os.Exit(1)
	}
}

func runLanInteractiveWizard() {
	currentTrust := config.GetTrustLAN()
	netAddrs := auth.DetectNetworkAddresses()
	lanIP := netAddrs.LANIPv4
	if lanIP == "" {
		lanIP = "未探测到 (请检查局域网连接)"
	}

	fmt.Println()
	fmt.Println("🌐 Multigravity 局域网配对安全策略配置")
	fmt.Println("--------------------------------------------------")
	fmt.Printf("➜  当前局域网 IP:   %s\n", lanIP)
	if currentTrust {
		fmt.Println("➜  当前安全策略:   局域网已信任免密放行 (Trust LAN: 免配对码)")
	} else {
		fmt.Println("➜  当前安全策略:   局域网标准安全配对 (Standard Pair: 需扫码/配对码)")
	}
	fmt.Println("--------------------------------------------------")
	fmt.Println("请选择要设定的局域网访问策略:")
	fmt.Println("  [1] 局域网已信任免密放行 (Trust LAN) - 同一 Wi-Fi/以太网设备免配对直接访问")
	fmt.Println("  [2] 局域网标准安全配对 (Standard Pair) - 首次连接必须通过二维码或配对码授权 (推荐)")
	fmt.Println("  [q] 退出 (保持当前配置不变)")
	fmt.Print("\n请输入选项 [1/2/q]: ")

	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	switch input {
	case "1":
		applyLanPolicy(true)
	case "2":
		applyLanPolicy(false)
	case "q", "Q", "exit", "":
		fmt.Println("已取消操作，配置保持不变。")
	default:
		fmt.Println("❌ 无效选项，已退出。")
	}
}

func applyLanPolicy(trust bool) {
	val := "0"
	desc := "局域网标准安全配对 (需配对码)"
	if trust {
		val = "1"
		desc = "局域网已信任免密放行 (免配对)"
	}

	path, err := config.UpdateEnvVariables(map[string]string{
		"MULTIGRAVITY_TRUST_LAN": val,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 保存配置失败: %v\n", err)
		return
	}

	fmt.Printf("\n✅ 已成功将局域网策略更新为: %s\n", desc)
	fmt.Printf("📁 配置文件: %s\n", path)
	fmt.Println("💡 提示: 下次启动 `mgy` 将默认使用此策略；若网关已在运行，重启后立即生效。")
}

func runLanStatus() {
	currentTrust := config.GetTrustLAN()
	netAddrs := auth.DetectNetworkAddresses()
	lanIP := netAddrs.LANIPv4
	if lanIP == "" {
		lanIP = "-"
	}

	fmt.Println()
	fmt.Println("🌐 Multigravity 局域网安全策略状态")
	fmt.Println("--------------------------------------------------")
	fmt.Printf("  局域网 IP:      %s\n", lanIP)
	if currentTrust {
		fmt.Println("  配对模式:       已信任免密放行 (MULTIGRAVITY_TRUST_LAN=1)")
		fmt.Println("  说明:           局域网内任意客户端均可直接接入控制台，无需扫码配对。")
	} else {
		fmt.Println("  配对模式:       标准安全配对 (MULTIGRAVITY_TRUST_LAN=0)")
		fmt.Println("  说明:           局域网设备首次访问必须在终端执行 `mgy pair` 扫码或输入配对码。")
	}
	fmt.Println("--------------------------------------------------")
	fmt.Println("  💡 切换策略: 执行 `mgy lan` 可进入交互式切换。")
	fmt.Println()
}

func runLanHelp() {
	fmt.Print(`🌐 Multigravity (mgy) - 局域网访问与配对安全策略配置

用法:
  mgy lan               交互式选择并切换局域网安全策略 (Trust LAN / Standard Pair)
  mgy lan status        查看当前局域网 IP 与配对策略状态
  mgy lan trust (或 on) 切换为局域网免配对信任放行
  mgy lan pair (或 off) 切换为局域网标准安全配对 (需配对码)
  mgy lan help          显示此帮助信息
`)
}
