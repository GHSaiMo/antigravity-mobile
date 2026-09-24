package notifier

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"antigravity-mobile/internal/config"
)

// RunBarkCmd handles the "mgy bark" CLI command and subcommands for iOS push notifications.
func RunBarkCmd(args []string) {
	if len(args) == 0 {
		runBarkInteractiveWizard()
		return
	}

	switch args[0] {
	case "status", "info":
		runBarkStatusCmd()
	case "test":
		runBarkTestCmd()
	case "set":
		runBarkSetCmd(args[1:])
	case "enable":
		runBarkEnableCmd()
	case "disable":
		runBarkDisableCmd()
	case "wizard", "config":
		runBarkInteractiveWizard()
	case "help", "-h", "--help":
		runBarkHelpCmd()
	default:
		fmt.Fprintf(os.Stderr, "❌ 未知子命令: mgy bark %s\n\n", args[0])
		runBarkHelpCmd()
		os.Exit(1)
	}
}

func runBarkHelpCmd() {
	fmt.Print(`🔔 Multigravity (mgy) - Bark 实时推送配置工具 (iOS 专用)

Bark 是 iOS 平台的极简推送通知客户端，用于在 iPhone / iPad 接收 Agent 状态提醒。

用法:
  mgy bark [子命令] [参数]

常用命令:
  mgy bark                  进入交互式向导，配置 iOS Bark 推送链接、提示音并发送测试
  mgy bark status           查看当前 Bark 推送配置、端点健康度与参数状态
  mgy bark test             向已配置的 iPhone 即时发送一条测试推送
  mgy bark set <Key或URL>   快速设置 Bark Device Key 或完整链接并开启推送
  mgy bark enable           开启 Bark 推送功能
  mgy bark disable          禁用 Bark 推送功能
  mgy bark help             显示此帮助信息
`)
}

func runBarkStatusCmd() {
	fmt.Println("================================================================================")
	fmt.Println("🔔 Bark 实时推送配置与状态检查 (iOS 专用)")
	fmt.Println("================================================================================")

	envPath := config.GetActiveEnvPath()
	fmt.Printf("📄 配置文件路径: %s\n", envPath)

	cfg := config.GetNotificationConfig()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "配置项\t状态/参数\t说明")
	fmt.Fprintln(w, "------\t---------\t----")

	// Enabled status
	enabledStr := "❌ 已禁用"
	if cfg.Enabled && cfg.BarkEndpoint != "" {
		enabledStr = "✅ 已启用"
	} else if cfg.Enabled {
		enabledStr = "⚠️  已开启但未配置 Key"
	}
	fmt.Fprintf(w, "推送功能状态\t%s\tiOS 锁屏与横幅通知开关\n", enabledStr)

	// Endpoint
	endpointStr := "(未配置)"
	if cfg.BarkEndpoint != "" {
		endpointStr = config.RedactBarkEndpoint(cfg.BarkEndpoint)
	}
	fmt.Fprintf(w, "Bark 设备端点\t%s\tiPhone 接收推送的专属地址\n", endpointStr)

	// Group
	fmt.Fprintf(w, "通知折叠分组\t%s\tiOS 通知中心归类名称\n", cfg.Group)

	// Sound
	fmt.Fprintf(w, "审批警报音\t%s\tAgent 等待人工审批时的提示音\n", cfg.SoundAction)
	fmt.Fprintf(w, "完成提示音\t%s\t任务全部执行完成时的提示音\n", cfg.SoundComplete)

	w.Flush()
	fmt.Println("================================================================================")

	if cfg.BarkEndpoint == "" {
		fmt.Println("💡 提示:")
		fmt.Println("   • 当前尚未配置 Bark Device Key，无法向 iPhone 发送推送。")
		fmt.Println("   • 请在 iPhone 上下载「Bark - 给你的手机发推送」App，")
		fmt.Println("     获取首页顶部的专属链接后，运行 `mgy bark` 进入一键配置向导。")
	} else {
		fmt.Println("💡 提示: 执行 `mgy bark test` 可立即向您的 iPhone 发送一条测试推送验证连通性。")
	}
	fmt.Println("================================================================================")
}

func runBarkTestCmd() {
	cfg := config.GetNotificationConfig()
	if cfg.BarkEndpoint == "" {
		fmt.Fprintf(os.Stderr, "❌ 未配置 Bark Device Key 或链接！请先运行 `mgy bark` 完成配置。\n")
		os.Exit(1)
	}

	fmt.Printf("🚀 正在向 iPhone 发送测试推送: %s ...\n", config.RedactBarkEndpoint(cfg.BarkEndpoint))
	client := NewBarkClient(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	err := client.Send(ctx, BarkPayload{
		Title: "🎉 Multigravity 测试推送 (iOS)",
		Body:  fmt.Sprintf("您的 iPhone 已成功连接到本地网关！测试时间: %s", time.Now().Format("15:04:05")),
		Sound: cfg.SoundComplete,
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 测试推送发送失败: %v\n", err)
		fmt.Println("   请检查网络连接以及填写的 Bark Device Key 是否准确无误。")
		os.Exit(1)
	}

	fmt.Printf("✅ 测试推送已成功送达！响应耗时: %v\n", time.Since(start).Round(time.Millisecond))
	fmt.Println("📱 请查看您的 iPhone 锁屏或通知中心是否有收到横幅提醒。")
}

func runBarkSetCmd(args []string) {
	fs := flag.NewFlagSet("set", flag.ExitOnError)
	testFlag := fs.Bool("test", true, "保存后是否自动发送测试推送验证")
	_ = fs.Parse(args)

	if len(fs.Args()) == 0 {
		fmt.Fprintf(os.Stderr, "❌ 请提供 Bark Device Key 或完整链接\n用法: mgy bark set <Key或URL>\n")
		os.Exit(1)
	}

	rawInput := strings.TrimSpace(fs.Args()[0])
	endpoint := config.NormalizeBarkEndpoint(rawInput)
	if endpoint == "" {
		fmt.Fprintf(os.Stderr, "❌ 无法识别的 Bark 格式: %s\n", rawInput)
		os.Exit(1)
	}

	updates := map[string]string{
		"BARK_URL":    endpoint,
		"BARK_ENABLE": "1",
	}

	path, err := config.UpdateEnvVariables(updates)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 保存配置失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✅ 已成功将 Bark 配置保存至:", path)
	fmt.Println("   • 设备端点:", config.RedactBarkEndpoint(endpoint))
	fmt.Println("   • 推送开关: 已开启")

	if *testFlag {
		fmt.Println("\n🚀 正在发送即时测试推送验证连通性...")
		cfg := config.GetNotificationConfig()
		client := NewBarkClient(cfg)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := client.Send(ctx, BarkPayload{
			Title: "🎉 Bark 配置成功 (iOS)",
			Body:  "您的 iPhone 已成功与 Multigravity 建立通知联动！",
			Sound: cfg.SoundComplete,
		}); err == nil {
			fmt.Println("🎉 测试推送已成功送达您的 iPhone！")
		} else {
			fmt.Printf("⚠️  测试发送反馈: %v\n", err)
		}
	}
}

func runBarkEnableCmd() {
	cfg := config.GetNotificationConfig()
	if cfg.BarkEndpoint == "" {
		fmt.Println("⚠️  当前尚未配置 Bark Device Key。请先运行 `mgy bark` 完成配置。")
		return
	}

	updates := map[string]string{
		"BARK_ENABLE": "1",
	}
	path, err := config.UpdateEnvVariables(updates)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 更新配置失败: %v\n", err)
		return
	}

	fmt.Println("✅ 已启用 Bark 实时推送 (iOS)。配置文件已同步:", path)
}

func runBarkDisableCmd() {
	updates := map[string]string{
		"BARK_ENABLE": "0",
	}
	path, err := config.UpdateEnvVariables(updates)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 更新配置失败: %v\n", err)
		return
	}

	fmt.Println("⏸️  已静默关闭 Bark 实时推送。配置文件已同步:", path)
}

func runBarkInteractiveWizard() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("================================================================================")
	fmt.Println("🔔 Multigravity - Bark 实时推送通知配置向导 (iOS 专用)")
	fmt.Println("================================================================================")
	fmt.Println("Bark 是 iOS 平台的极简通知推送客户端，专为 iPhone / iPad 用户打造。")
	fmt.Println("当反重力 Agent 执行任务需要人工审批操作 (Action) 或任务全部执行完成时，")
	fmt.Println("网关会自动向您的 iPhone 推送锁屏通知并播放专属提示音，彻底避免切屏漏消息。")
	fmt.Println("================================================================================")

	cfg := config.GetNotificationConfig()

	// Step 0: Display current status
	fmt.Println("📋 当前检测状态:")
	statusDesc := "⚠️  未配置 (推送未开启)"
	if cfg.BarkEndpoint != "" && cfg.Enabled {
		statusDesc = fmt.Sprintf("✅ 已配置 (%s)", config.RedactBarkEndpoint(cfg.BarkEndpoint))
	}
	fmt.Printf("   • Bark 推送端点: %s\n", statusDesc)
	fmt.Printf("   • 审批警报音:   %s\n", cfg.SoundAction)
	fmt.Printf("   • 任务完成音:   %s\n", cfg.SoundComplete)
	fmt.Printf("   • 配置文件位置: %s\n", config.GetActiveEnvPath())
	fmt.Println("--------------------------------------------------------------------------------")

	// Step 1: Prompt how to get key
	if cfg.BarkEndpoint == "" {
		fmt.Println("📱 【新手快速指引 - 如何获取 iPhone 的专属 Bark 链接】:")
		fmt.Println("   1. 打开 iPhone 上的 App Store，搜索「Bark - 给你的手机发推送」并安装；")
		fmt.Println("   2. 打开 Bark App，首页顶部会展示您的专属服务器链接 (形式如: https://api.day.app/YOUR_KEY/)；")
		fmt.Println("   3. 点击复制即可将完整链接或末尾的 Device Key 贴入下方。")
		fmt.Println("--------------------------------------------------------------------------------")
	}

	// Step 2: Input Key or URL
	fmt.Print("\n[1/3] 请输入您的 Bark 专属链接或 Device Key\n")
	if cfg.BarkEndpoint != "" {
		fmt.Printf("      (直接回车保持当前设置 [%s]): ", config.RedactBarkEndpoint(cfg.BarkEndpoint))
	} else {
		fmt.Print("      请输入: ")
	}

	inputKey, _ := reader.ReadString('\n')
	inputKey = strings.TrimSpace(inputKey)

	targetEndpoint := cfg.BarkEndpoint
	if inputKey != "" {
		normalized := config.NormalizeBarkEndpoint(inputKey)
		if normalized == "" {
			fmt.Printf("❌ 无法解析该链接或 Key: %s，配置已中止。\n", inputKey)
			return
		}
		targetEndpoint = normalized
	}

	if targetEndpoint == "" {
		fmt.Println("⚠️  未填入有效 Bark Key，将保留静默关闭状态。")
		return
	}

	// Step 3: Action Sound
	fmt.Printf("\n[2/3] 需要审批/等待人工操作时的提示音 [回车保持: %s]: ", cfg.SoundAction)
	soundActionInput, _ := reader.ReadString('\n')
	soundActionInput = strings.TrimSpace(soundActionInput)
	targetSoundAction := cfg.SoundAction
	if soundActionInput != "" {
		targetSoundAction = soundActionInput
	}

	// Step 4: Complete Sound
	fmt.Printf("[3/3] 任务全部执行完成时的提示音 [回车保持: %s]: ", cfg.SoundComplete)
	soundCompleteInput, _ := reader.ReadString('\n')
	soundCompleteInput = strings.TrimSpace(soundCompleteInput)
	targetSoundComplete := cfg.SoundComplete
	if soundCompleteInput != "" {
		targetSoundComplete = soundCompleteInput
	}

	// Save updates to .env
	updates := map[string]string{
		"BARK_URL":            targetEndpoint,
		"BARK_ENABLE":         "1",
		"BARK_SOUND_ACTION":   targetSoundAction,
		"BARK_SOUND_COMPLETE": targetSoundComplete,
		"BARK_GROUP":          cfg.Group,
	}

	fmt.Println("\n💾 正在保存配置...")
	path, err := config.UpdateEnvVariables(updates)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 保存配置失败: %v\n", err)
		return
	}
	fmt.Printf("✅ 配置已成功保存至 %s！\n", path)
	fmt.Println("--------------------------------------------------------------------------------")

	// Step 5: Instant test
	fmt.Print("🚀 是否立即向您的 iPhone 发送一条测试推送验证连通性？[Y/n] (默认: Y): ")
	testInput, _ := reader.ReadString('\n')
	testInput = strings.TrimSpace(strings.ToLower(testInput))

	if testInput != "n" && testInput != "no" {
		fmt.Println("📡 正在向 iPhone 发送测试通知...")
		newCfg := config.GetNotificationConfig()
		client := NewBarkClient(newCfg)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		start := time.Now()
		err := client.Send(ctx, BarkPayload{
			Title: "🎉 Bark 推送配置成功 (iOS)",
			Body:  fmt.Sprintf("您的 iPhone 已成功与 Multigravity 连接！耗时: %v", time.Since(start).Round(time.Millisecond)),
			Sound: targetSoundComplete,
		})

		if err == nil {
			fmt.Printf("🎉 测试推送发送成功 (耗时: %v)！请查看 iPhone 是否已亮屏或弹出横幅通知。\n", time.Since(start).Round(time.Millisecond))
		} else {
			fmt.Printf("⚠️  测试推送发送失败: %v\n", err)
			fmt.Println("   请检查网络连接及 Device Key 是否填写正确。")
		}
	}

	fmt.Println("================================================================================")
	fmt.Println("💡 配置完成！后续使用建议:")
	fmt.Println("   • 启动网关主服务:   mgy")
	fmt.Println("   • 查看推送状态:     mgy bark status")
	fmt.Println("   • 再次发送测试:     mgy bark test")
	fmt.Println("================================================================================")
}
