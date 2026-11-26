package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

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

	checkOnly := fs.Bool("check", false, "仅检查是否有新版本，不下载替换")
	force := fs.Bool("force", false, "即使已是最新版本也强制重新下载安装")
	targetVersion := fs.String("version", "", "指定要安装或回滚的版本号 (例如 v1.2.0)")
	port := fs.Int("port", defaultPort(), "用于探测本地网关是否正在运行的端口")

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
		fmt.Fprintf(os.Stderr, "❌ 更新失败: %v\n", err)
		os.Exit(1)
	}

	if *checkOnly {
		if res.UpToDate {
			fmt.Printf("✅ 当前已是最新版本: v%s\n", res.CurrentVersion)
		} else {
			fmt.Printf("🆕 发现新版本: v%s (当前: v%s)\n", res.LatestVersion, res.CurrentVersion)
			fmt.Println("💡 执行 `mgy update` 即可一键升级。")
		}
		return
	}

	if !res.Updated && res.UpToDate {
		fmt.Printf("✅ 当前已是最新版本: v%s (如需强制覆盖，可执行 `mgy update -force`)\n", res.CurrentVersion)
		return
	}

	fmt.Printf("\n✅ Multigravity (mgy) 已成功更新: v%s -> v%s\n", res.CurrentVersion, res.LatestVersion)
	fmt.Printf("📁 可执行文件: %s\n", res.TargetPath)
	if res.ChecksumVerified {
		fmt.Println("🔒 校验状态:   SHA-256 校验通过 + 二进制自检通过")
	} else {
		fmt.Println("🛡️  校验状态:   归档完整 + 二进制自检通过")
	}

	if runningVer, ok := updater.ProbeRunningGatewayVersion(*port); ok {
		svcRestartHint := "请重启网关以使新版本生效"
		if service.IsInstalled(ctx) {
			svcRestartHint = "可执行 `mgy service restart` 重启后台服务使新版本生效"
		}
		if runningVer != "" {
			fmt.Printf("💡 提示: 检测到本地网关仍在运行 (v%s, 端口 %d)，%s。\n", updater.NormalizeVersion(runningVer), *port, svcRestartHint)
		} else {
			fmt.Printf("💡 提示: 检测到本地网关仍在运行 (端口 %d)，%s。\n", *port, svcRestartHint)
		}
	}
}

func runUpdateHelp() {
	fmt.Printf(`Multigravity (mgy) %s - 检查并一键更新网关二进制

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
`, Version, defaultPort(), updater.DefaultRepo)
}
