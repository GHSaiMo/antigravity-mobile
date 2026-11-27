package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"antigravity-mobile/internal/i18n"
	"antigravity-mobile/internal/service"
	"antigravity-mobile/internal/updater"
)

func runUpdateCmd(args []string) {
	if len(args) > 0 {
		switch strings.ToLower(strings.TrimSpace(args[0])) {
		case "help", "-h", "--help":
			runUpdateHelp()
			return
		}
	}

	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = runUpdateHelp

	checkOnly := fs.Bool("check", false, i18n.T3(
		"仅检查是否有新版本，不下载替换",
		"Check for a newer version without downloading or replacing",
		"新しいバージョンの有無のみ確認（ダウンロード・置換しない）",
	))
	force := fs.Bool("force", false, i18n.T3(
		"即使已是最新版本也强制重新下载安装",
		"Force re-download and reinstall even if already up to date",
		"最新バージョンでも強制的に再ダウンロードしてインストール",
	))
	targetVersion := fs.String("version", "", i18n.T3(
		"指定要安装或回滚的版本号 (例如 v1.2.0)",
		"Install or roll back to a specific version (e.g. v1.2.0)",
		"インストールまたはロールバックするバージョンを指定（例: v1.2.0）",
	))
	port := fs.Int("port", defaultPort(), i18n.T3(
		"用于探测本地网关是否正在运行的端口",
		"Port used to probe whether the local gateway is running",
		"ローカルゲートウェイの稼働確認に使うポート",
	))

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return
		}
		os.Exit(1)
	}

	// Support optional positional argument: `mgy update check` or `mgy update v1.2.0`
	if fs.NArg() > 0 {
		arg := strings.TrimSpace(fs.Arg(0))
		switch strings.ToLower(arg) {
		case "check":
			*checkOnly = true
		case "help":
			runUpdateHelp()
			return
		default:
			if *targetVersion == "" {
				*targetVersion = arg
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	res, err := updater.Run(ctx, updater.Options{
		CurrentVersion: Version,
		TargetVersion:  *targetVersion,
		CheckOnly:      *checkOnly,
		Force:          *force,
		Out:            os.Stdout,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, i18n.T3("❌ 更新失败: %v\n", "❌ Update failed: %v\n", "❌ 更新に失敗しました: %v\n"), err)
		os.Exit(1)
	}

	if *checkOnly {
		if res.UpToDate {
			fmt.Printf(i18n.T3("✅ 当前已是最新版本: v%s\n", "✅ Already up to date: v%s\n", "✅ すでに最新バージョンです: v%s\n"), res.CurrentVersion)
		} else {
			fmt.Printf(i18n.T3("🆕 发现新版本: v%s (当前: v%s)\n", "🆕 New version available: v%s (current: v%s)\n", "🆕 新しいバージョンが利用可能です: v%s（現在: v%s）\n"), res.LatestVersion, res.CurrentVersion)
			fmt.Println(i18n.T3("💡 执行 `mgy update` 即可一键升级。", "💡 Run `mgy update` to upgrade.", "💡 `mgy update` を実行してアップグレードできます。"))
		}
		return
	}

	if !res.Updated && res.UpToDate {
		fmt.Printf(i18n.T3(
			"✅ 当前已是最新版本: v%s (如需强制覆盖，可执行 `mgy update -force`)\n",
			"✅ Already up to date: v%s (run `mgy update -force` to reinstall)\n",
			"✅ すでに最新バージョンです: v%s（強制再インストールは `mgy update -force`）\n",
		), res.CurrentVersion)
		return
	}

	fmt.Printf(i18n.T3(
		"\n✅ Multigravity (mgy) 已成功更新: v%s -> v%s\n",
		"\n✅ Multigravity (mgy) updated: v%s -> v%s\n",
		"\n✅ Multigravity (mgy) を更新しました: v%s -> v%s\n",
	), res.CurrentVersion, res.LatestVersion)
	fmt.Printf(i18n.T3("📁 可执行文件: %s\n", "📁 Binary: %s\n", "📁 実行ファイル: %s\n"), res.TargetPath)
	if res.ChecksumVerified {
		fmt.Println(i18n.T3(
			"🔒 校验状态:   SHA-256 校验通过 + 二进制自检通过",
			"🔒 Verification: SHA-256 verified + binary self-check passed",
			"🔒 検証状態:   SHA-256 検証済み + バイナリ自己診断OK",
		))
	} else {
		fmt.Println(i18n.T3(
			"🛡️  校验状态:   归档完整 + 二进制自检通过",
			"🛡️  Verification: Archive intact + binary self-check passed",
			"🛡️  検証状態:   アーカイブ検証済み + バイナリ自己診断OK",
		))
	}

	if runningVer, ok := updater.ProbeRunningGatewayVersion(*port); ok {
		svcRestartHint := i18n.T3("请重启网关以使新版本生效", "please restart the gateway to apply the new version", "新しいバージョンを有効にするためゲートウェイを再起動してください")
		if service.IsInstalled(ctx) {
			svcRestartHint = i18n.T3("可执行 `mgy service restart` 重启后台服务使新版本生效", "run `mgy service restart` to restart the background service", "`mgy service restart` でバックグラウンドサービスを再起動してください")
		}
		if runningVer != "" {
			fmt.Printf(i18n.T3(
				"💡 提示: 检测到本地网关仍在运行 (v%s, 端口 %d)，%s。\n",
				"💡 Tip: Local gateway is still running (v%s, port %d); %s.\n",
				"💡 ヒント: ローカルゲートウェイが稼働中です (v%s, ポート %d)。%s。\n",
			), updater.NormalizeVersion(runningVer), *port, svcRestartHint)
		} else {
			fmt.Printf(i18n.T3(
				"💡 提示: 检测到本地网关仍在运行 (端口 %d)，%s。\n",
				"💡 Tip: Local gateway is still running (port %d); %s.\n",
				"💡 ヒント: ローカルゲートウェイが稼働中です (ポート %d)。%s。\n",
			), *port, svcRestartHint)
		}
	}
}

func runUpdateHelp() {
	fmt.Printf(i18n.T3(`Multigravity (mgy) %s - 检查并一键更新网关二进制

用法:
  mgy update [参数]

常用示例:
  mgy update                  检查并自动升级到最新正式版
  mgy update -check           仅检查是否有新版本，不执行下载替换
  mgy update -force           强制重新下载并覆盖当前版本
  mgy update -version v1.2.0  安装或回滚到指定版本

参数说明:
  -check              仅检查更新
  -force              强制重新安装
  -version <版本号>   指定目标版本 (如 v1.2.0 或 1.2.0)
  -port <端口号>      探测本机运行中网关的端口 (默认: %d)

环境变量:
  MULTIGRAVITY_REPO            自定义 GitHub 仓库 (默认: %s)
  MULTIGRAVITY_UPDATE_CHECK    设为 0/false 可关闭启动时的静默版本检查
`, `Multigravity (mgy) %s - Check and update the gateway binary

Usage:
  mgy update [flags]

Examples:
  mgy update                  Check and upgrade to the latest release
  mgy update -check           Check for updates without installing
  mgy update -force           Force re-download and reinstall
  mgy update -version v1.2.0  Install or roll back to a specific version

Flags:
  -check              Check for updates only
  -force              Force reinstall
  -version <ver>      Target version (e.g. v1.2.0 or 1.2.0)
  -port <port>        Local gateway port to probe (default: %d)

Environment:
  MULTIGRAVITY_REPO            Custom GitHub repository (default: %s)
  MULTIGRAVITY_UPDATE_CHECK    Set to 0/false to disable startup update checks
`, `Multigravity (mgy) %s - ゲートウェイバイナリの確認と更新

使い方:
  mgy update [オプション]

使用例:
  mgy update                  最新リリースを確認して自動アップグレード
  mgy update -check           更新の有無のみ確認（インストールしない）
  mgy update -force           強制的に再ダウンロードして上書き
  mgy update -version v1.2.0  指定バージョンへ更新またはロールバック

オプション:
  -check              更新確認のみ
  -force              強制再インストール
  -version <ver>      対象バージョン (例: v1.2.0 または 1.2.0)
  -port <port>        ローカルゲートウェイ確認ポート (既定: %d)

環境変数:
  MULTIGRAVITY_REPO            カスタム GitHub リポジトリ (既定: %s)
  MULTIGRAVITY_UPDATE_CHECK    0/false で起動時の更新チェックを無効化
`), Version, defaultPort(), updater.DefaultRepo)
}
