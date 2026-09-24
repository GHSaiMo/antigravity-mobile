package cockpit

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)


// RunCockpitCmd handles the "mgy cockpit" CLI command and subcommands.
func RunCockpitCmd(args []string) {
	if len(args) == 0 {
		runCockpitInteractiveWizard()
		return
	}

	switch args[0] {
	case "status", "info":
		runCockpitStatusCmd()
	case "token", "set-token":
		runCockpitTokenCmd(args[1:])
	case "enable":
		runCockpitEnableCmd()
	case "restart":
		runCockpitRestartCmd()
	case "test":
		runCockpitTestCmd()
	case "wizard", "config":
		runCockpitInteractiveWizard()
	case "help", "-h", "--help":
		runCockpitHelpCmd()
	default:
		fmt.Fprintf(os.Stderr, "❌ 未知子命令: mgy cockpit %s\n\n", args[0])
		runCockpitHelpCmd()
		os.Exit(1)
	}
}

func runCockpitHelpCmd() {
	fmt.Print(`🛸 Multigravity (mgy) - Cockpit Tools 联动配置工具

用法:
  mgy cockpit [子命令] [参数]

常用命令:
  mgy cockpit                  进入交互式向导，一键配置 HTTP 报表服务与访问 Token 并重启生效
  mgy cockpit status           检查 Cockpit Tools 运行状态、端口监听与 Token 配置健康度
  mgy cockpit token <新Token>  快速设置访问 Token 并自动开启 HTTP 报表服务
  mgy cockpit token -g         自动生成 32 位高强度安全 Token 并配置生效
  mgy cockpit enable           一键开启 HTTP 报表服务 (report_enabled: true)
  mgy cockpit restart          重启 Cockpit Tools 桌面进程以加载最新配置
  mgy cockpit test             测试 HTTP 报表接口连通性并拉取最新配额数据
  mgy cockpit help             显示此帮助信息
`)
}

func runCockpitStatusCmd() {
	fmt.Println("================================================================================")
	fmt.Println("🛸 Cockpit Tools 运行与配置状态检查")
	fmt.Println("================================================================================")

	dataDir, err := GetCockpitDataDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 无法获取 Cockpit 数据目录: %v\n", err)
		return
	}
	fmt.Printf("📁 数据目录: %s\n", dataDir)

	// 1. Process status
	isRunning := isCockpitProcessRunning()
	var serverInfo *CockpitServerInfo
	if sInfo, sErr := ReadRawCockpitServerInfo(); sErr == nil {
		serverInfo = sInfo
	}

	cfg, _ := getCockpitConfig()
	reportPort := 18081
	reportEnabled := false
	reportToken := ""
	if cfg != nil {
		if cfg.ReportPort > 0 {
			reportPort = cfg.ReportPort
		}
		reportEnabled = cfg.ReportEnabled
		reportToken = cfg.ReportToken
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "服务组件\t配置状态\t网络探测与健康度")
	fmt.Fprintln(w, "--------\t--------\t----------------")

	// Process row
	if isRunning {
		pidDesc := "运行中"
		if serverInfo != nil && serverInfo.PID > 0 {
			pidDesc = fmt.Sprintf("运行中 (PID: %d)", serverInfo.PID)
		}
		fmt.Fprintf(w, "Cockpit 进程\t已安装\t✅ %s\n", pidDesc)
	} else {
		fmt.Fprintf(w, "Cockpit 进程\t未检测到运行\t⚠️ 进程未启动 (可通过 `mgy cockpit restart` 启动)\n")
	}

	// WebSocket row
	wsPort := 19528
	if serverInfo != nil && serverInfo.WsPort > 0 {
		wsPort = serverInfo.WsPort
	}
	wsListening := IsCockpitListening(wsPort, 400*time.Millisecond)
	if wsListening {
		fmt.Fprintf(w, "WebSocket 服务\t端口: %d\t✅ ws://127.0.0.1:%d 正常连接\n", wsPort, wsPort)
	} else {
		fmt.Fprintf(w, "WebSocket 服务\t端口: %d\t❌ 端口未响应\n", wsPort)
	}

	// HTTP Report service row
	reportListening := IsCockpitListening(reportPort, 400*time.Millisecond)
	var reportStatusStr string
	if reportEnabled {
		if reportListening {
			reportStatusStr = fmt.Sprintf("✅ 端口 %d 监听正常", reportPort)
		} else {
			reportStatusStr = fmt.Sprintf("⚠️ 端口 %d 未监听 (需重启 Cockpit Tools 生效)", reportPort)
		}
	} else {
		reportStatusStr = fmt.Sprintf("❌ 未开启 (report_enabled: false，端口 %d 未监听)", reportPort)
	}
	enabledDesc := "已开启"
	if !reportEnabled {
		enabledDesc = "已关闭"
	}
	fmt.Fprintf(w, "HTTP 报表服务\t开关: %s (端口: %d)\t%s\n", enabledDesc, reportPort, reportStatusStr)

	// Token row
	var tokenDesc string
	if reportToken == "" {
		tokenDesc = "❌ 未配置 (Token 为空，接口无法访问)"
	} else if reportToken == "change-this-token" {
		tokenDesc = "⚠️ 默认占位符 (change-this-token，未激活)"
	} else {
		masked := reportToken
		if len(masked) > 8 {
			masked = masked[:4] + "...." + masked[len(masked)-4:]
		}
		tokenDesc = fmt.Sprintf("✅ 已配置专属 Token (%s)", masked)
	}
	fmt.Fprintf(w, "访问 Token\t%s\t%s\n", redactTokenDisplay(reportToken), tokenDesc)

	w.Flush()
	fmt.Println("================================================================================")

	// Summary and actionable advice
	if !reportEnabled || reportToken == "" || reportToken == "change-this-token" || !reportListening {
		fmt.Println("💡 诊断建议:")
		if !reportEnabled {
			fmt.Println("   • HTTP 报表服务当前处于关闭状态，手机端无法直接触发账号配额刷新。")
			fmt.Println("     -> 运行 `mgy cockpit enable` 可快速开启。")
		}
		if reportToken == "" || reportToken == "change-this-token" {
			fmt.Println("   • 访问 Token 尚未配置或仍为默认值 'change-this-token'。")
			fmt.Println("     -> 运行 `mgy cockpit token -g` 可一键自动生成并配置安全 Token。")
		}
		if reportEnabled && !reportListening {
			fmt.Println("   • HTTP 报表服务已开启但端口未监听，这是因为 Cockpit Tools 需要重启后才能载入新配置。")
			fmt.Println("     -> 运行 `mgy cockpit restart` 可一键重启应用生效。")
		}
		fmt.Println("   • 或者直接运行 `mgy cockpit` 进入全流程交互式向导完成一键配置！")
	} else {
		// Try verifying endpoint
		if err := QueryReport(reportPort, reportToken); err == nil {
			fmt.Println("🎉 状态完美！HTTP 报表服务与 Token 校验全部通过，移动端已可全功能无缝同步。")
		} else {
			fmt.Printf("⚠️  HTTP 报表服务响应异常: %v\n", err)
			fmt.Println("   请确认 Token 是否与 Cockpit Tools 界面上显示的完全一致。")
		}
	}
	fmt.Println("================================================================================")
}

func redactTokenDisplay(t string) string {
	if t == "" {
		return "(空)"
	}
	if t == "change-this-token" {
		return "change-this-token"
	}
	if len(t) <= 8 {
		return "********"
	}
	return t[:3] + "..." + t[len(t)-3:]
}

func runCockpitTokenCmd(args []string) {
	fs := flag.NewFlagSet("token", flag.ExitOnError)
	genFlag := fs.Bool("g", false, "自动生成 32 位高强度安全随机 Token")
	genLongFlag := fs.Bool("generate", false, "自动生成 32 位高强度安全随机 Token")
	restartFlag := fs.Bool("restart", true, "修改后是否自动重启 Cockpit Tools 应用以生效")
	_ = fs.Parse(args)

	var token string
	if *genFlag || *genLongFlag {
		token = GenerateSecureToken()
		fmt.Printf("🔑 已生成加密随机 Token: %s\n", token)
	} else if len(fs.Args()) > 0 {
		token = strings.TrimSpace(fs.Args()[0])
	} else {
		// Prompt user interactively
		fmt.Print("请输入新的 Cockpit 访问 Token (输入 'g' 自动生成随机密钥): ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "g" || input == "gen" || input == "" {
			token = GenerateSecureToken()
			fmt.Printf("🔑 已自动生成随机 Token: %s\n", token)
		} else {
			token = input
		}
	}

	if token == "" {
		fmt.Fprintf(os.Stderr, "❌ Token 不能为空\n")
		os.Exit(1)
	}

	cfg, _ := getCockpitConfig()
	port := 18081
	if cfg != nil && cfg.ReportPort > 0 {
		port = cfg.ReportPort
	}

	if err := SaveCockpitReportSettings(true, port, token); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 保存配置失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ 已成功将 Token 保存至 ~/.antigravity_cockpit/config.json 并开启 HTTP 报表服务。")
	fmt.Println("   • 报表端口: ", port)
	fmt.Println("   • 访问 Token: ", token)

	if *restartFlag && isCockpitProcessRunning() {
		fmt.Println("🔄 正在自动重启 Cockpit Tools 应用以载入新配置...")
		if err := RestartCockpitApp(); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  重启应用失败: %v。请手动关闭并重新打开 Cockpit Tools 应用。\n", err)
		} else {
			fmt.Println("⏳ 等待服务端口就绪...")
			if waitForPortListening(port, 6*time.Second) {
				fmt.Printf("✅ Cockpit Tools 已成功重启，端口 %d 开始监听！\n", port)
				if qErr := QueryReport(port, token); qErr == nil {
					fmt.Println("🎉 HTTP 报表连通性测试通过 (HTTP 200 OK)！")
				}
			} else {
				fmt.Println("ℹ️  应用已重启，若端口未立即监听，请在 Cockpit Tools 界面确认设置。")
			}
		}
	} else {
		fmt.Println("💡 提示: Cockpit Tools 修改配置后需重启应用方可生效。可执行 `mgy cockpit restart` 完成重启。")
	}
}

func runCockpitEnableCmd() {
	cfg, err := getCockpitConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 无法读取 Cockpit 配置: %v\n", err)
		os.Exit(1)
	}

	port := 18081
	if cfg.ReportPort > 0 {
		port = cfg.ReportPort
	}
	token := strings.TrimSpace(cfg.ReportToken)
	if token == "" || token == "change-this-token" {
		token = GenerateSecureToken()
		fmt.Printf("🔑 检测到 Token 处于默认值，已自动为您生成专属安全 Token: %s\n", token)
	}

	if err := SaveCockpitReportSettings(true, port, token); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 开启 HTTP 服务配置失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ 已开启 HTTP 报表服务配置 (report_enabled: true)。")
	if isCockpitProcessRunning() {
		fmt.Println("🔄 正在重启 Cockpit Tools 应用以生效配置...")
		if err := RestartCockpitApp(); err == nil && waitForPortListening(port, 6*time.Second) {
			fmt.Printf("✅ Cockpit Tools 重启成功，HTTP 报表端口 %d 已上线！\n", port)
		} else {
			fmt.Println("💡 提示: 请执行 `mgy cockpit restart` 或手动重启 Cockpit Tools 桌面应用。")
		}
	}
}

func runCockpitRestartCmd() {
	fmt.Println("🔄 正在重启 Cockpit Tools 桌面应用...")
	if err := RestartCockpitApp(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 重启失败: %v\n", err)
		os.Exit(1)
	}

	cfg, _ := getCockpitConfig()
	port := 18081
	if cfg != nil && cfg.ReportPort > 0 {
		port = cfg.ReportPort
	}

	fmt.Printf("⏳ 正在等待 Cockpit Tools 初始化并监听端口 %d...\n", port)
	if waitForPortListening(port, 7*time.Second) {
		fmt.Printf("✅ Cockpit Tools 启动成功，端口 %d 监听正常！\n", port)
		if cfg != nil && cfg.ReportToken != "" && cfg.ReportToken != "change-this-token" {
			if qErr := QueryReport(port, cfg.ReportToken); qErr == nil {
				fmt.Println("🎉 HTTP 报表服务连通性验证通过！")
			}
		}
	} else {
		fmt.Println("ℹ️  Cockpit Tools 进程已启动。如端口未监听，请在 Cockpit Tools 应用设置中确认 HTTP 报表服务已开启。")
	}
}

func runCockpitTestCmd() {
	cfg, err := getCockpitConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 无法读取 Cockpit 配置: %v\n", err)
		os.Exit(1)
	}

	port := 18081
	if cfg.ReportPort > 0 {
		port = cfg.ReportPort
	}
	token := cfg.ReportToken
	if token == "" {
		fmt.Fprintf(os.Stderr, "❌ Token 未配置，请先运行 `mgy cockpit token <token>` 或 `mgy cockpit` 进行配置。\n")
		os.Exit(1)
	}

	fmt.Printf("🧪 正在测试连接: http://127.0.0.1:%d/report?token=%s ...\n", port, redactTokenDisplay(token))
	start := time.Now()
	err = QueryReport(port, token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 测试失败: %v\n", err)
		if strings.Contains(err.Error(), "connection refused") || !IsCockpitListening(port, time.Second) {
			fmt.Println("   • 原因: 端口未在监听。若已修改设置，请执行 `mgy cockpit restart` 重启应用。")
		} else if strings.Contains(err.Error(), "401") {
			fmt.Println("   • 原因: Token 鉴权失败。请检查 Token 是否与 Cockpit Tools 界面上的设置一致。")
		}
		os.Exit(1)
	}

	fmt.Printf("✅ 测试成功！响应耗时: %v\n", time.Since(start).Round(time.Millisecond))
	fmt.Println("📱 移动端与网关现可顺畅拉取最新配额数据。")
}

func runCockpitInteractiveWizard() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("================================================================================")
	fmt.Println("🛸 Multigravity - Cockpit Tools 交互式配置向导")
	fmt.Println("================================================================================")
	fmt.Println("本向导将指引您一键完成 Cockpit Tools 网页报表服务与安全 Token 的配置，")
	fmt.Println("使手机移动端可以实时查看与无缝刷新各账号的配额用量。")
	fmt.Println("================================================================================")

	// Step 0: inspect current environment
	cfg, _ := getCockpitConfig()
	currentPort := 18081
	currentEnabled := false
	currentToken := "change-this-token"
	if cfg != nil {
		if cfg.ReportPort > 0 {
			currentPort = cfg.ReportPort
		}
		currentEnabled = cfg.ReportEnabled
		if cfg.ReportToken != "" {
			currentToken = cfg.ReportToken
		}
	}

	isProcessRunning := isCockpitProcessRunning()
	portListening := IsCockpitListening(currentPort, 300*time.Millisecond)

	fmt.Println("📋 当前检测状态:")
	procStatus := "❌ 未运行"
	if isProcessRunning {
		procStatus = "✅ 运行中"
	}
	fmt.Printf("   • Cockpit Tools 桌面进程: %s\n", procStatus)

	enabledStatus := "⚠️  未开启 (report_enabled: false)"
	if currentEnabled {
		enabledStatus = "✅ 已开启"
	}
	fmt.Printf("   • HTTP 报表服务配置状态: %s\n", enabledStatus)

	listenStatus := "❌ 未在监听"
	if portListening {
		listenStatus = "✅ 监听中"
	}
	fmt.Printf("   • 报表端口 (127.0.0.1:%d): %s\n", currentPort, listenStatus)

	tokenStatus := "⚠️  默认占位符 (未激活)"
	if currentToken != "" && currentToken != "change-this-token" {
		tokenStatus = fmt.Sprintf("✅ 已配置 (%s)", redactTokenDisplay(currentToken))
	}
	fmt.Printf("   • 当前访问 Token:         %s\n", tokenStatus)
	fmt.Println("--------------------------------------------------------------------------------")

	// Step 1: Enable HTTP Report Service
	fmt.Printf("\n[1/3] 是否开启 HTTP 报表服务？(输入 Y 开启 / N 保持关闭) [Y]: ")
	enableInput, _ := reader.ReadString('\n')
	enableInput = strings.TrimSpace(strings.ToLower(enableInput))
	targetEnabled := true
	if enableInput == "n" || enableInput == "no" {
		targetEnabled = false
	}

	// Step 2: Configure Port
	fmt.Printf("[2/3] 请确认 HTTP 报表端口 [回车保持: %d]: ", currentPort)
	portInput, _ := reader.ReadString('\n')
	portInput = strings.TrimSpace(portInput)
	targetPort := currentPort
	if portInput != "" {
		if p, err := strconv.Atoi(portInput); err == nil && p > 0 && p < 65536 {
			targetPort = p
		} else {
			fmt.Printf("⚠️  无效端口输入，将沿用默认端口 %d\n", currentPort)
		}
	}

	// Step 3: Configure Token
	fmt.Printf("\n[3/3] 配置访问 Token:\n")
	if currentToken == "change-this-token" {
		fmt.Println("   💡 提示: 检测到当前 Token 为默认占位符 'change-this-token'。")
	}
	fmt.Println("   • 直接回车: 保留当前值")
	fmt.Println("   • 输入 'g' 或 'gen': 自动生成 32 位高强度安全 Token (推荐)")
	fmt.Println("   • 输入自定义密码/字符串: 作为新 Token")
	fmt.Print("   请选择或输入: ")
	tokenInput, _ := reader.ReadString('\n')
	tokenInput = strings.TrimSpace(tokenInput)

	targetToken := currentToken
	if tokenInput == "g" || tokenInput == "gen" {
		targetToken = GenerateSecureToken()
		fmt.Printf("   🔑 已为您生成专属安全 Token: %s\n", targetToken)
	} else if tokenInput != "" {
		targetToken = tokenInput
		fmt.Printf("   🔑 已设定自定义 Token: %s\n", redactTokenDisplay(targetToken))
	} else if targetToken == "change-this-token" {
		// If user pressed enter on default token, prompt whether to auto-generate
		fmt.Print("   ⚠️  当前仍为默认占位符，是否自动生成安全 Token 替代？[Y/n]: ")
		gConfirm, _ := reader.ReadString('\n')
		gConfirm = strings.TrimSpace(strings.ToLower(gConfirm))
		if gConfirm != "n" && gConfirm != "no" {
			targetToken = GenerateSecureToken()
			fmt.Printf("   🔑 已为您生成专属安全 Token: %s\n", targetToken)
		}
	}

	// Save configuration
	fmt.Println("\n💾 正在写入配置...")
	if err := SaveCockpitReportSettings(targetEnabled, targetPort, targetToken); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 保存配置失败: %v\n", err)
		return
	}
	fmt.Println("✅ 配置已成功保存至 ~/.antigravity_cockpit/config.json！")
	fmt.Println("--------------------------------------------------------------------------------")

	// Step 4: Restart application
	fmt.Println("⚠️  【关键提示】: Cockpit Tools 的 HTTP 报表服务与端口修改后，需重启应用方可生效。")
	fmt.Print("是否立即重启 Cockpit Tools 应用以载入新配置？[Y/n]: ")
	restartInput, _ := reader.ReadString('\n')
	restartInput = strings.TrimSpace(strings.ToLower(restartInput))

	if restartInput != "n" && restartInput != "no" {
		fmt.Println("\n🔄 正在优雅重启 Cockpit Tools 桌面应用...")
		if err := RestartCockpitApp(); err != nil {
			fmt.Printf("⚠️  自动重启触发受阻: %v\n", err)
			fmt.Println("   请在屏幕右上角托盘菜单或 Dock 中完全退出 Cockpit Tools 并重新打开。")
		} else {
			fmt.Print("⏳ 正在等待应用初始化与网络服务上线")
			online := false
			for i := 0; i < 8; i++ {
				time.Sleep(600 * time.Millisecond)
				fmt.Print(".")
				if IsCockpitListening(targetPort, 300*time.Millisecond) {
					online = true
					break
				}
			}
			fmt.Println()

			if online {
				fmt.Printf("✅ 端口 127.0.0.1:%d 已成功启动并接受连接！\n", targetPort)
				// Test report query
				fmt.Println("🧪 正在测试 HTTP 报表连通性与数据校验...")
				if err := QueryReport(targetPort, targetToken); err == nil {
					fmt.Println("🎉 HTTP 报表接口验证成功 (HTTP 200 OK)！用量数据拉取畅通无阻！")
				} else {
					fmt.Printf("ℹ️  接口连接成功，查询反馈: %v\n", err)
				}
			} else {
				fmt.Println("ℹ️  应用已重新拉起。若端口未立刻响应，请在 Cockpit Tools 窗口中点击确认应用设置。")
			}
		}
	} else {
		fmt.Println("\n💡 您选择暂不重启。修改已保存在配置文件中，下次启动或手动重启 Cockpit Tools 时即可生效。")
	}

	fmt.Println("================================================================================")
	fmt.Println("🚀 配置完成！后续使用建议:")
	fmt.Println("   • 启动网关主服务:   mgy (或 mgy run)")
	fmt.Println("   • 查看 Cockpit 状态: mgy cockpit status")
	fmt.Println("   • 移动端配对二维码: mgy pair")
	fmt.Println("================================================================================")
}

func waitForPortListening(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if IsCockpitListening(port, 300*time.Millisecond) {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return IsCockpitListening(port, 300*time.Millisecond)
}
