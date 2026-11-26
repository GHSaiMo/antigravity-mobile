package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/i18n"
	"antigravity-mobile/internal/service"
)

func runServiceCmd(args []string) {
	if len(args) == 0 {
		runServiceStatus(nil)
		return
	}

	sub := strings.ToLower(strings.TrimSpace(args[0]))
	rest := args[1:]
	switch sub {
	case "install", "enable":
		runServiceInstall(rest)
	case "uninstall", "remove", "disable":
		runServiceUninstall(rest)
	case "start":
		runServiceAction(sub, rest)
	case "stop":
		runServiceAction(sub, rest)
	case "restart":
		runServiceAction(sub, rest)
	case "status", "info":
		runServiceStatus(rest)
	case "logs", "log":
		runServiceLogs(rest)
	case "help", "-h", "--help":
		runServiceHelp()
	default:
		fmt.Fprint(os.Stderr, i18n.Sprintf3(
			"❌ 未知 service 子命令: %s\n\n",
			"❌ Unknown service subcommand: %s\n\n",
			"❌ 不明な service サブコマンド: %s\n\n",
			args[0],
		))
		runServiceHelp()
		os.Exit(1)
	}
}

func newServiceManagerOrExit() service.Manager {
	mgr, err := service.NewManager()
	if err != nil {
		fmt.Fprint(os.Stderr, i18n.Sprintf3(
			"❌ 当前系统不支持原生后台服务管理: %v\n",
			"❌ Native background service management is unavailable on this OS: %v\n",
			"❌ この OS ではネイティブバックグラウンドサービス管理を利用できません: %v\n",
			err,
		))
		os.Exit(1)
	}
	return mgr
}

func printServiceError(action string, err error) {
	if errors.Is(err, service.ErrNotInstalled) {
		fmt.Fprintln(os.Stderr, i18n.T3(
			"⚠️  尚未安装 Multigravity 后台自启服务。请先执行: mgy service install",
			"⚠️  Multigravity background service is not installed yet. Run: mgy service install",
			"⚠️  Multigravity バックグラウンドサービスは未登録です。先に実行してください: mgy service install",
		))
		os.Exit(1)
	}
	if errors.Is(err, service.ErrElevationRequired) {
		fmt.Fprintln(os.Stderr, i18n.T3(
			"❌ 权限不足：Windows 注册/管理开机自启任务需要「管理员权限」。",
			"❌ Administrator privileges required to register or manage the Windows startup task.",
			"❌ 権限不足：Windows の自動起動タスクを登録・管理するには管理者権限が必要です。",
		))
		fmt.Fprintln(os.Stderr, i18n.T3(
			"💡 请右键「Windows 终端 / PowerShell」选择「以管理员身份运行」，或直接执行以下提权命令：",
			"💡 Open PowerShell as Administrator, or run this one-liner to elevate:",
			"💡 管理者として PowerShell を開くか、以下のコマンドで昇格実行してください：",
		))
		fmt.Fprintf(os.Stderr, "   Start-Process powershell -Verb RunAs -ArgumentList \"-NoExit\",\"-Command\",\"mgy service %s\"\n", action)
		os.Exit(1)
	}
	fmt.Fprint(os.Stderr, i18n.Sprintf3(
		"❌ 执行 `mgy service %s` 失败: %v\n",
		"❌ Failed to run `mgy service %s`: %v\n",
		"❌ `mgy service %s` の実行に失敗しました: %v\n",
		action, err,
	))
	os.Exit(1)
}

func runServiceInstall(args []string) {
	fs := flag.NewFlagSet("service install", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = runServiceHelp

	modeFlag := fs.String("mode", "run", i18n.T("运行模式: run / hub / node", "Gateway mode: run / hub / node"))
	portFlag := fs.Int("port", 0, i18n.T("固定监听端口（默认 0，启动时动态读取 ~/.multigravity/.env）", "Fixed listen port (default 0: read dynamically from ~/.multigravity/.env)"))
	hostFlag := fs.String("host", "", i18n.T("固定监听地址（默认留空动态读取 .env）", "Fixed bind host (default empty: read dynamically from .env)"))
	headlessFlag := fs.Bool("headless", false, i18n.T("无头 Hub 模式（仅 -mode hub 时生效）", "Headless Hub mode (only for -mode hub)"))
	nameFlag := fs.String("name", "", i18n.T("Hub 或 Node 显示名称", "Hub or Node display name"))
	hubURLFlag := fs.String("hub", "", i18n.T("Node 模式目标 Hub 地址", "Target Hub URL for node mode"))
	noDiscFlag := fs.Bool("no-discovery", false, i18n.T("关闭局域网 UDP 自动发现", "Disable LAN UDP auto-discovery"))
	logonOnlyFlag := fs.Bool("logon-only", false, i18n.T("仅 Windows：改为登录后静默启动（Interactive + conhost --headless），而非开机前 S4U", "Windows only: run silently on user logon (Interactive + conhost --headless) instead of S4U boot trigger"))
	noStartFlag := fs.Bool("no-start", false, i18n.T("仅注册开机自启，不立即启动服务", "Register auto-start without starting immediately"))
	binaryFlag := fs.String("binary", "", i18n.T("自定义 mgy 可执行文件路径（默认自动识别当前二进制）", "Custom path to mgy binary (defaults to current executable)"))

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return
		}
		os.Exit(1)
	}

	binPath, err := service.ResolveBinaryPath(*binaryFlag)
	if err != nil {
		fmt.Fprint(os.Stderr, i18n.Sprintf3(
			"❌ 无法定位 mgy 可执行文件: %v\n",
			"❌ Failed to locate mgy binary: %v\n",
			"❌ mgy 実行ファイルを特定できませんでした: %v\n",
			err,
		))
		os.Exit(1)
	}

	mgr := newServiceManagerOrExit()
	dataDir := config.GetDataDir()
	cfg := service.InstallConfig{
		BinaryPath:       binPath,
		DataDir:          dataDir,
		LogPath:          service.DefaultLogPath(dataDir),
		Mode:             *modeFlag,
		Port:             *portFlag,
		Host:             strings.TrimSpace(*hostFlag),
		Headless:         *headlessFlag,
		Name:             strings.TrimSpace(*nameFlag),
		HubURL:           strings.TrimSpace(*hubURLFlag),
		NoDiscovery:      *noDiscFlag,
		WindowsLogonOnly: *logonOnlyFlag,
		NoStart:          *noStartFlag,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := mgr.Install(ctx, cfg)
	if err != nil {
		printServiceError("install", err)
	}

	fmt.Println()
	fmt.Println(i18n.T3(
		"✅ Multigravity (mgy) 开机自动接管服务已安装就绪！",
		"✅ Multigravity (mgy) auto-start background service installed!",
		"✅ Multigravity (mgy) の自動起動バックグラウンドサービスを登録しました！",
	))
	fmt.Println("--------------------------------------------------")
	fmt.Printf("  %-14s %s\n", i18n.T3("托管引擎:", "Backend:", "バックエンド:"), res.Backend)
	fmt.Printf("  %-14s %s\n", i18n.T3("服务名称:", "Service:", "サービス名:"), res.Name)
	fmt.Printf("  %-14s %s\n", i18n.T3("配置位置:", "Config:", "設定先:"), res.ConfigPath)
	fmt.Printf("  %-14s %s\n", i18n.T3("执行程序:", "Binary:", "バイナリ:"), res.BinaryPath)
	fmt.Printf("  %-14s %s\n", i18n.T3("启动参数:", "Args:", "引数:"), strings.Join(res.Args, " "))
	fmt.Printf("  %-14s %s\n", i18n.T3("日志文件:", "Log file:", "ログファイル:"), res.LogPath)
	if res.Note != "" {
		fmt.Printf("  %-14s %s\n", i18n.T3("提示:", "Note:", "補足:"), res.Note)
	}
	fmt.Println("--------------------------------------------------")

	if res.Started {
		probePort := *portFlag
		if probePort <= 0 {
			probePort = defaultPort()
		}
		healthy := waitForGatewayHealth(probePort, 3*time.Second)
		if healthy {
			fmt.Print(i18n.Sprintf3(
				"🚀 后台网关已启动并响应健康检查 (http://127.0.0.1:%d)\n",
				"🚀 Background gateway is running and healthy (http://127.0.0.1:%d)\n",
				"🚀 バックグラウンドゲートウェイが起動し正常応答しています (http://127.0.0.1:%d)\n",
				probePort,
			))
		} else {
			fmt.Println(i18n.T3(
				"⏳ 已触发后台启动，可稍后运行 `mgy service status` 或 `mgy service logs` 查看状态。",
				"⏳ Background start triggered; run `mgy service status` or `mgy service logs` to inspect.",
				"⏳ バックグラウンド起動を開始しました。`mgy service status` または `mgy service logs` で確認できます。",
			))
		}
	}

	fmt.Println(i18n.T3(
		"💡 常用管理: `mgy pair` 随时获取扫码配对码 · `mgy service status` 查状态 · `mgy service logs -f` 看日志 · `mgy service uninstall` 取消接管",
		"💡 Quick tips: `mgy pair` to pair devices · `mgy service status` · `mgy service logs -f` · `mgy service uninstall` to remove",
		"💡 操作ヒント: `mgy pair` でペアリング · `mgy service status` · `mgy service logs -f` · `mgy service uninstall` で解除",
	))
	fmt.Println()
}

func runServiceUninstall(args []string) {
	if hasHelpArg(args) {
		runServiceHelp()
		return
	}
	mgr := newServiceManagerOrExit()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	removed, err := mgr.Uninstall(ctx)
	if err != nil {
		printServiceError("uninstall", err)
	}
	if !removed {
		fmt.Println(i18n.T3(
			"ℹ️  未检测到已安装的 Multigravity 后台自启服务，无需卸载。",
			"ℹ️  Multigravity background service is not installed; nothing to remove.",
			"ℹ️  Multigravity バックグラウンドサービスは登録されていません。",
		))
		return
	}
	fmt.Print(i18n.Sprintf3(
		"✅ 已停止并移除后台自启服务 (%s: %s)\n",
		"✅ Stopped and removed background service (%s: %s)\n",
		"✅ バックグラウンドサービスを停止・削除しました (%s: %s)\n",
		mgr.BackendName(), mgr.ServiceName(),
	))
}

func runServiceAction(action string, args []string) {
	if hasHelpArg(args) {
		runServiceHelp()
		return
	}
	mgr := newServiceManagerOrExit()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var err error
	switch action {
	case "start":
		err = mgr.Start(ctx)
	case "stop":
		err = mgr.Stop(ctx)
	case "restart":
		err = mgr.Restart(ctx)
	}
	if err != nil {
		printServiceError(action, err)
	}

	switch action {
	case "start":
		fmt.Println(i18n.T3("✅ 已启动 Multigravity 后台服务。", "✅ Started Multigravity background service.", "✅ Multigravity バックグラウンドサービスを起動しました。"))
	case "stop":
		fmt.Println(i18n.T3("⏹️  已停止 Multigravity 后台服务。", "⏹️  Stopped Multigravity background service.", "⏹️  Multigravity バックグラウンドサービスを停止しました。"))
	case "restart":
		fmt.Println(i18n.T3("🔄 已重启 Multigravity 后台服务。", "🔄 Restarted Multigravity background service.", "🔄 Multigravity バックグラウンドサービスを再起動しました。"))
	}
}

func runServiceStatus(args []string) {
	if hasHelpArg(args) {
		runServiceHelp()
		return
	}
	fs := flag.NewFlagSet("service status", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = runServiceHelp
	portFlag := fs.Int("port", defaultPort(), i18n.T("健康检查探测端口", "Port for local health probe"))
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return
		}
		os.Exit(1)
	}

	mgr := newServiceManagerOrExit()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()

	st, err := mgr.Status(ctx)
	if err != nil {
		printServiceError("status", err)
	}

	healthy, liveVer := service.ProbeHealth(*portFlag)

	fmt.Println()
	fmt.Println(i18n.T3(
		"🛡️  Multigravity (mgy) 后台自启服务状态",
		"🛡️  Multigravity (mgy) Background Service Status",
		"🛡️  Multigravity (mgy) バックグラウンドサービス状態",
	))
	fmt.Println("--------------------------------------------------")
	fmt.Printf("  %-14s %s\n", i18n.T3("托管引擎:", "Backend:", "バックエンド:"), st.Backend)
	fmt.Printf("  %-14s %s\n", i18n.T3("服务名称:", "Service:", "サービス名:"), st.Name)
	if st.Installed {
		fmt.Printf("  %-14s %s\n", i18n.T3("安装状态:", "Installed:", "登録状態:"), i18n.T3("✅ 已注册开机自启", "✅ Installed (auto-start enabled)", "✅ 登録済み（自動起動有効）"))
	} else {
		fmt.Printf("  %-14s %s\n", i18n.T3("安装状态:", "Installed:", "登録状態:"), i18n.T3("⚪ 未安装", "⚪ Not installed", "⚪ 未登録"))
	}
	if st.Running {
		pidText := ""
		if st.PID > 0 {
			pidText = fmt.Sprintf(" (PID %d)", st.PID)
		}
		fmt.Printf("  %-14s %s%s\n", i18n.T3("运行状态:", "Running:", "稼働状態:"), i18n.T3("🟢 运行中", "🟢 Running", "🟢 実行中"), pidText)
	} else {
		fmt.Printf("  %-14s %s (%s)\n", i18n.T3("运行状态:", "Running:", "稼働状態:"), i18n.T3("🔴 未运行", "🔴 Stopped", "🔴 停止中"), st.State)
	}
	if healthy {
		verText := ""
		if liveVer != "" {
			verText = " v" + strings.TrimPrefix(liveVer, "v")
		}
		fmt.Printf("  %-14s %s (127.0.0.1:%d%s)\n", i18n.T3("网关探活:", "Gateway:", "ゲートウェイ:"), i18n.T3("🟢 在线", "🟢 Healthy", "🟢 正常"), *portFlag, verText)
	} else {
		fmt.Printf("  %-14s %s (127.0.0.1:%d)\n", i18n.T3("网关探活:", "Gateway:", "ゲートウェイ:"), i18n.T3("⚪ 未响应", "⚪ Unreachable", "⚪ 未応答"), *portFlag)
	}
	if st.Detail != "" {
		fmt.Printf("  %-14s %s\n", i18n.T3("策略详情:", "Policy:", "設定詳細:"), st.Detail)
	}
	if st.BinaryPath != "" {
		fmt.Printf("  %-14s %s\n", i18n.T3("执行程序:", "Binary:", "バイナリ:"), st.BinaryPath)
	}
	fmt.Printf("  %-14s %s\n", i18n.T3("配置位置:", "Config:", "設定先:"), st.ConfigPath)
	fmt.Printf("  %-14s %s\n", i18n.T3("日志文件:", "Log file:", "ログファイル:"), st.LogPath)
	fmt.Println("--------------------------------------------------")
	if !st.Installed {
		if runtime.GOOS == "windows" {
			fmt.Println(i18n.T3(
				"💡 提示: 以管理员身份运行 PowerShell 执行 `mgy service install` 即可开启开机无黑框后台接管。",
				"💡 Tip: Run `mgy service install` in an Administrator PowerShell to enable headless startup takeover.",
				"💡 ヒント: 管理者 PowerShell で `mgy service install` を実行すると自動起動を登録できます。",
			))
		} else {
			fmt.Println(i18n.T3(
				"💡 提示: 执行 `mgy service install` 即可注册开机自动接管后台守护服务。",
				"💡 Tip: Run `mgy service install` to register the native auto-start background daemon.",
				"💡 ヒント: `mgy service install` で自動起動バックグラウンドサービスを登録できます。",
			))
		}
	}
	fmt.Println()
}

func runServiceLogs(args []string) {
	if hasHelpArg(args) {
		runServiceHelp()
		return
	}
	fs := flag.NewFlagSet("service logs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = runServiceHelp

	linesFlag := fs.Int("n", 80, i18n.T("显示末尾日志行数", "Number of trailing log lines to show"))
	followFlag := fs.Bool("f", false, i18n.T("持续实时跟踪日志输出 (Ctrl+C 退出)", "Follow live log output (Ctrl+C to exit)"))
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return
		}
		os.Exit(1)
	}

	mgr := newServiceManagerOrExit()
	logPath := mgr.LogPath()
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		fmt.Fprint(os.Stderr, i18n.Sprintf3(
			"ℹ️  尚未找到服务日志文件: %s\n   请先启动后台服务（mgy service install / start）。\n",
			"ℹ️  Service log file not found yet: %s\n   Start the background service first (mgy service install / start).\n",
			"ℹ️  サービスログファイルがまだありません: %s\n   先にサービスを起動してください (mgy service install / start)。\n",
			logPath,
		))
		return
	}

	if *followFlag {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := service.FollowLogs(ctx, logPath, *linesFlag, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			os.Exit(1)
		}
		return
	}

	lines, err := service.TailLogs(logPath, *linesFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
	for _, line := range lines {
		fmt.Println(line)
	}
}

func waitForGatewayHealth(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ok, _ := service.ProbeHealth(port); ok {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

func hasHelpArg(args []string) bool {
	for _, a := range args {
		switch strings.ToLower(strings.TrimSpace(a)) {
		case "-h", "--help", "help":
			return true
		}
	}
	return false
}

func runServiceHelp() {
	if i18n.IsZh() {
		fmt.Print(`🛡️  Multigravity (mgy) - 开机自动接管与后台守护服务管理

支持平台原生托管引擎:
  • Windows: 任务计划程序 (Task Scheduler，默认 S4U 无黑框开机自启 + 最高权限 + 崩溃自动拉起)
  • macOS:   launchd (~/Library/LaunchAgents/com.multigravity.gateway.plist，RunAtLoad + KeepAlive)
  • Linux:   systemd --user (~/.config/systemd/user/mgy.service，Restart=always)

用法:
  mgy service [子命令] [参数]

子命令:
  install [参数]    安装并立即启动开机自动接管后台服务 (别名: enable)
  uninstall         停止并彻底移除开机自启服务 (别名: remove / disable)
  start             启动已安装的后台服务
  stop              停止后台服务
  restart           重启后台服务 (更新 mgy 或修改 .env 后推荐使用)
  status            查看后台服务注册与实时探活状态 (默认子命令)
  logs [-n 80] [-f] 查看后台服务日志 (-f 实时跟踪)
  help              显示此帮助信息

install 可选参数:
  -mode <run|hub|node>  运行模式 (默认: run)
  -port <端口号>        固定监听端口 (默认留空动态读取 ~/.multigravity/.env)
  -host <主机/IP>       固定监听地址 (默认留空动态读取 .env)
  -headless             无头 Hub 模式 (仅 -mode hub)
  -name <名称>          Hub / Node 显示名称
  -hub <URL>            Node 模式目标 Hub 地址
  -no-discovery         关闭局域网 UDP 自动发现
  -logon-only           仅 Windows：改为登录后静默启动 (Interactive + conhost --headless)
  -no-start             仅注册开机自启，不立即启动
  -binary <路径>        指定自定义 mgy 可执行文件路径

示例:
  mgy service install                      # 一键注册并启动常规后台网关
  mgy service install -mode hub -headless  # 在服务器/NAS 上注册无头 Hub 守护
  mgy service status                       # 查看守护与探活状态
  mgy service logs -f                      # 实时查看后台日志
  mgy service restart                      # 升级后重启后台服务
  mgy service uninstall                    # 取消开机自动接管
`)
	} else if i18n.IsJa() {
		fmt.Print(`🛡️  Multigravity (mgy) - 自動起動バックグラウンドサービス管理

対応ネイティブバックエンド:
  • Windows: タスク スケジューラ (既定 S4U ウィンドウ非表示 + 管理者権限 + 障害時自動再起動)
  • macOS:   launchd (~/Library/LaunchAgents/com.multigravity.gateway.plist, RunAtLoad + KeepAlive)
  • Linux:   systemd --user (~/.config/systemd/user/mgy.service, Restart=always)

使い方:
  mgy service [サブコマンド] [オプション]

サブコマンド:
  install [オプション]  自動起動サービスを登録して起動 (別名: enable)
  uninstall             自動起動サービスを停止・削除 (別名: remove / disable)
  start                 サービスを起動
  stop                  サービスを停止
  restart               サービスを再起動 (アップデートや .env 変更後に推奨)
  status                サービス状態とヘルスチェックを表示 (デフォルト)
  logs [-n 80] [-f]     サービスログを表示 (-f でリアルタイム追従)
  help                  このヘルプを表示

install オプション:
  -mode <run|hub|node>  動作モード (デフォルト: run)
  -port <番号>          待受ポートを固定 (既定では .env から動的読込)
  -host <ホスト/IP>     バインド先を固定 (既定では .env から動的読込)
  -headless             ヘッドレス Hub モード (-mode hub 用)
  -name <名前>          Hub / Node 表示名
  -hub <URL>            Node モード接続先 Hub URL
  -no-discovery         LAN UDP 自動検出を無効化
  -logon-only           Windows 専用: ログオン時ヘッドレス起動 (Interactive) に切替
  -no-start             登録のみ行い即時起動しない
  -binary <パス>        任意の mgy バイナリを指定
`)
	} else {
		fmt.Print(`🛡️  Multigravity (mgy) - Native Auto-Start Background Service Manager

Native OS backends:
  • Windows: Task Scheduler (default S4U headless boot+logon, HighestAvailable, auto-restart on failure)
  • macOS:   launchd (~/Library/LaunchAgents/com.multigravity.gateway.plist, RunAtLoad + KeepAlive)
  • Linux:   systemd --user (~/.config/systemd/user/mgy.service, Restart=always)

Usage:
  mgy service [subcommand] [flags]

Subcommands:
  install [flags]   Register and start the auto-start background service (alias: enable)
  uninstall         Stop and remove the background service (aliases: remove, disable)
  start             Start the installed background service
  stop              Stop the background service
  restart           Restart the background service (recommended after updates or .env edits)
  status            Show service registration and live health status (default)
  logs [-n 80] [-f] Show service logs (-f to follow live)
  help              Show this help message

Install Flags:
  -mode <run|hub|node>  Gateway mode (default: run)
  -port <number>        Fixed listen port (default: dynamically loaded from ~/.multigravity/.env)
  -host <host/IP>       Fixed bind host (default: dynamically loaded from .env)
  -headless             Headless Hub mode (with -mode hub)
  -name <name>          Hub or Node display name
  -hub <URL>            Target Hub URL (with -mode node)
  -no-discovery         Disable LAN UDP auto-discovery
  -logon-only           Windows only: run silently on user logon (Interactive + conhost --headless)
  -no-start             Register without starting immediately
  -binary <path>        Override path to mgy executable

Examples:
  mgy service install
  mgy service install -mode hub -headless
  mgy service status
  mgy service logs -f
  mgy service restart
  mgy service uninstall
`)
	}
}
