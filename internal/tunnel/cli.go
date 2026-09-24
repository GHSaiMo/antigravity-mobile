package tunnel

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"antigravity-mobile/internal/config"
)


// RunCloudflareCmd handles the "mgy cloudflare" and "mgy cf" CLI commands and subcommands.
func RunCloudflareCmd(args []string) {
	if len(args) == 0 {
		runCloudflareInteractiveWizard()
		return
	}

	switch args[0] {
	case "status", "info":
		runCloudflareStatusCmd()
	case "enable":
		runCloudflareEnableCmd()
	case "disable":
		runCloudflareDisableCmd()
	case "reset":
		runCloudflareResetCmd()
	case "set-worker":
		runCloudflareSetWorkerCmd(args[1:])
	case "set-code":
		runCloudflareSetCodeCmd(args[1:])
	case "set-token":
		runCloudflareSetTokenCmd(args[1:])
	case "wizard", "config":
		runCloudflareInteractiveWizard()
	case "help", "-h", "--help":
		runCloudflareHelpCmd()
	default:
		fmt.Fprintf(os.Stderr, "❌ 未知子命令: mgy cloudflare %s\n\n", args[0])
		runCloudflareHelpCmd()
		os.Exit(1)
	}
}

func runCloudflareHelpCmd() {
	fmt.Print(`☁️ Multigravity (mgy) - Cloudflare 专属公网穿透配置工具

用于管理全自动 Cloudflare 隧道 (自动分配永久专属 HTTPS 域名，扫码即用)。

用法:
  mgy cloudflare [子命令] [参数]
  mgy cf         [子命令] [参数]   (快捷别名)

常用命令:
  mgy cloudflare             进入交互式向导，配置公网穿透开关、Worker 调度器与暗号
  mgy cloudflare status      查看穿透状态、专属域名、缓存凭据与 cloudflared 引擎
  mgy cloudflare enable      一键开启 Cloudflare 专属穿透隧道
  mgy cloudflare disable     关闭穿透隧道 (仅限局域网内网直连)
  mgy cloudflare reset       清除本地隧道凭据缓存，下次启动时重新申请全新域名
  mgy cloudflare set-worker  设置自定义 Cloudflare Worker 调度器地址
  mgy cloudflare set-code    设置 Worker 调度器专属暗号/邀请码
  mgy cloudflare set-token   手动指定已有专属 Tunnel Token
  mgy cloudflare help        显示此帮助信息
`)
}

func runCloudflareStatusCmd() {
	fmt.Println("================================================================================")
	fmt.Println("☁️ Cloudflare 专属公网穿透配置与状态检查")
	fmt.Println("================================================================================")

	cfCfg := config.GetCloudflareConfig()
	envPath := config.GetActiveEnvPath()
	fmt.Printf("📄 配置文件路径: %s\n", envPath)

	cachePath := filepath.Join(config.GetDataDir(), "cf_tunnel.json")
	var cached CFTunnelResult
	hasCache := false
	if data, err := os.ReadFile(cachePath); err == nil {
		if json.Unmarshal(data, &cached) == nil && cached.URL != "" {
			hasCache = true
		}
	}

	binPath := FindCloudflaredBinary()
	hasBin := binPath != ""


	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "配置组件\t当前状态\t说明")
	fmt.Fprintln(w, "--------\t--------\t----")

	// Enabled status
	enabledStr := "✅ 已开启"
	if !cfCfg.Enabled {
		enabledStr = "❌ 已关闭"
	}
	fmt.Fprintf(w, "穿透开关\t%s\t启动时是否拉起 cloudflared 穿透\n", enabledStr)

	// Assigned Domain
	domainStr := "(尚未分配，启动网关时自动申请)"
	if hasCache {
		domainStr = fmt.Sprintf("✅ %s", cached.URL)
	} else if cfCfg.Token != "" {
		domainStr = "✅ 手动 Token 模式 (未通过 Worker 申请)"
	}
	fmt.Fprintf(w, "专属 HTTPS 域名\t%s\t移动端扫码连接的永久专属外网地址\n", domainStr)

	// Worker URL
	workerStr := cfCfg.WorkerURL
	if workerStr == config.DefaultCloudflareWorkerURL {
		workerStr = fmt.Sprintf("%s (官方公共调度器)", workerStr)
	}
	fmt.Fprintf(w, "Worker 调度器\t%s\t负责自动注册与绑定域名的调度节点\n", workerStr)

	// Invite Code
	codeStr := "(留空，公开申请)"
	if cfCfg.InviteCode != "" {
		codeStr = "******** (已配置专属暗号)"
	}
	fmt.Fprintf(w, "调度器暗号\t%s\t私有 Worker 准入鉴权\n", codeStr)

	// Tunnel Token
	tokenDesc := "(由 Worker 自动管理)"
	if cfCfg.Token != "" {
		tokenDesc = "已手动指定自定义 Token"
	} else if hasCache && cached.TunnelName != "" {
		tokenDesc = fmt.Sprintf("已分配隧道: %s", cached.TunnelName)
	}
	fmt.Fprintf(w, "隧道凭据\t%s\tCloudflare Tunnel 鉴权信息\n", tokenDesc)

	// Binary
	binDesc := "❌ 未就绪 (启动网关时自动下载)"
	if hasBin {
		binDesc = fmt.Sprintf("✅ 已就绪 (%s)", binPath)
	}
	fmt.Fprintf(w, "穿透引擎 (cloudflared)\t%s\t官方穿透核心程序\n", binDesc)

	w.Flush()
	fmt.Println("================================================================================")

	if !cfCfg.Enabled {
		fmt.Println("💡 提示: 穿透功能当前处于关闭状态，外部设备无法通过专属 HTTPS 域名直连。")
		fmt.Println("   -> 执行 `mgy cloudflare enable` 可快速开启。")
	} else if hasCache {
		fmt.Println("🎉 专属域名已就绪！启动网关 (执行 `mgy`) 后即可通过此域名在外网随时连回家中。")
	} else {
		fmt.Println("💡 提示: 首次执行 `mgy` 启动网关时，将全自动完成域名分配与引擎预载，开箱即用。")
	}
	fmt.Println("================================================================================")
}

func runCloudflareEnableCmd() {
	updates := map[string]string{
		"CF_TUNNEL_ENABLED": "1",
	}
	path, err := config.UpdateEnvVariables(updates)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 更新配置失败: %v\n", err)
		return
	}
	fmt.Println("✅ 已开启 Cloudflare 专属公网穿透。配置文件已更新:", path)
}

func runCloudflareDisableCmd() {
	updates := map[string]string{
		"CF_TUNNEL_ENABLED": "0",
	}
	path, err := config.UpdateEnvVariables(updates)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 更新配置失败: %v\n", err)
		return
	}
	fmt.Println("⏸️  已关闭 Cloudflare 专属穿透 (仅允许局域网或内网直连)。配置文件已更新:", path)
}

func runCloudflareResetCmd() {
	cachePath := filepath.Join(config.GetDataDir(), "cf_tunnel.json")
	if _, err := os.Stat(cachePath); err == nil {
		_ = os.Remove(cachePath)
		fmt.Println("✅ 已清除本地专属隧道凭据缓存 (cf_tunnel.json)。")
		fmt.Println("   下次启动网关 (`mgy`) 时，系统将自动向 Worker 申请一个全新的专属域名！")
	} else {
		fmt.Println("ℹ️  本地当前暂无隧道凭据缓存，无需清除。")
	}
}

func runCloudflareSetWorkerCmd(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "❌ 请提供 Worker 调度器 URL\n用法: mgy cloudflare set-worker <https://your-worker.dev>\n")
		os.Exit(1)
	}
	urlStr := strings.TrimRight(strings.TrimSpace(args[0]), "/")
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		fmt.Fprintf(os.Stderr, "❌ URL 格式无效，必须以 http:// 或 https:// 开头\n")
		os.Exit(1)
	}

	updates := map[string]string{
		"CF_WORKER_URL": urlStr,
	}
	path, err := config.UpdateEnvVariables(updates)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 更新配置失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ 已设置 Worker 调度器地址为:", urlStr)
	fmt.Println("   配置文件已更新:", path)
}

func runCloudflareSetCodeCmd(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "❌ 请提供调度器邀请码/暗号\n用法: mgy cloudflare set-code <暗号>\n")
		os.Exit(1)
	}
	code := strings.TrimSpace(args[0])
	updates := map[string]string{
		"CF_INVITE_CODE": code,
	}
	path, err := config.UpdateEnvVariables(updates)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 更新配置失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ 已成功设置 Worker 调度器邀请码。配置文件已更新:", path)
}

func runCloudflareSetTokenCmd(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "❌ 请提供 Tunnel Token\n用法: mgy cloudflare set-token <Token>\n")
		os.Exit(1)
	}
	token := strings.TrimSpace(args[0])
	updates := map[string]string{
		"CF_TUNNEL_TOKEN": token,
	}
	path, err := config.UpdateEnvVariables(updates)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 更新配置失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ 已成功配置手动 Tunnel Token。配置文件已更新:", path)
}

func runCloudflareInteractiveWizard() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("================================================================================")
	fmt.Println("☁️ Multigravity - Cloudflare 专属公网穿透配置向导")
	fmt.Println("================================================================================")
	fmt.Println("Multigravity 内置全自动 Cloudflare 隧道调度引擎，可为您的网关全自动签发")
	fmt.Println("专属永久 HTTPS 二级域名，无需公网 IP、无需路由器端口映射，开箱即用。")
	fmt.Println("================================================================================")

	cfCfg := config.GetCloudflareConfig()

	cachePath := filepath.Join(config.GetDataDir(), "cf_tunnel.json")
	var cached CFTunnelResult
	hasCache := false
	if data, err := os.ReadFile(cachePath); err == nil {
		if json.Unmarshal(data, &cached) == nil && cached.URL != "" {
			hasCache = true
		}
	}

	fmt.Println("📋 当前检测状态:")
	switchDesc := "❌ 已关闭"
	if cfCfg.Enabled {
		switchDesc = "✅ 已开启"
	}
	fmt.Printf("   • 穿透功能开关:   %s\n", switchDesc)
	domainDesc := "尚未分配 (首次启动自动申请)"
	if hasCache {
		domainDesc = fmt.Sprintf("✅ %s", cached.URL)
	}
	fmt.Printf("   • 专属 HTTPS 域名: %s\n", domainDesc)
	fmt.Printf("   • 当前调度器地址: %s\n", cfCfg.WorkerURL)
	fmt.Printf("   • 配置文件位置:   %s\n", config.GetActiveEnvPath())
	fmt.Println("--------------------------------------------------------------------------------")

	// Step 1: Enable Tunnel
	fmt.Printf("[1/3] 是否启用 Cloudflare 专属公网穿透？(输入 Y 开启 / N 保持关闭) [Y]: ")
	enableInput, _ := reader.ReadString('\n')
	enableInput = strings.TrimSpace(strings.ToLower(enableInput))
	targetEnabled := "1"
	if enableInput == "n" || enableInput == "no" {
		targetEnabled = "0"
	}

	// Step 2: Worker URL
	fmt.Printf("\n[2/3] Cloudflare Worker 调度器地址 [回车保持默认: %s]: ", cfCfg.WorkerURL)
	workerInput, _ := reader.ReadString('\n')
	workerInput = strings.TrimSpace(workerInput)
	targetWorker := cfCfg.WorkerURL
	if workerInput != "" {
		targetWorker = strings.TrimRight(workerInput, "/")
	}

	// Step 3: Invite Code
	codeHint := "(留空公开申请)"
	if cfCfg.InviteCode != "" {
		codeHint = fmt.Sprintf("(当前: %s，回车保持)", cfCfg.InviteCode)
	}
	fmt.Printf("\n[3/3] Worker 调度器暗号/邀请码 %s: ", codeHint)
	codeInput, _ := reader.ReadString('\n')
	codeInput = strings.TrimSpace(codeInput)
	targetCode := cfCfg.InviteCode
	if codeInput != "" {
		targetCode = codeInput
	}

	updates := map[string]string{
		"CF_TUNNEL_ENABLED": targetEnabled,
		"CF_WORKER_URL":     targetWorker,
		"CF_INVITE_CODE":    targetCode,
	}

	fmt.Println("\n💾 正在保存配置...")
	path, err := config.UpdateEnvVariables(updates)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 保存配置失败: %v\n", err)
		return
	}
	fmt.Printf("✅ 配置已成功保存至 %s！\n", path)
	fmt.Println("--------------------------------------------------------------------------------")

	// Optional Reset Domain
	if hasCache {
		fmt.Printf("🔄 当前已绑定专属域名: %s\n", cached.URL)
		fmt.Print("是否需要重置并更换为一个全新的专属域名？[y/N]: ")
		resetInput, _ := reader.ReadString('\n')
		resetInput = strings.TrimSpace(strings.ToLower(resetInput))
		if resetInput == "y" || resetInput == "yes" {
			_ = os.Remove(cachePath)
			fmt.Println("✅ 历史凭据已重置，下次启动网关时将自动分配新域名。")
		} else {
			fmt.Println("保留现有专属域名。")
		}
	} else if targetEnabled == "1" {
		fmt.Println("🚀 提示: 穿透已就绪！首次运行 `mgy` 时，调度引擎将自动为您分配专属域名并启动。")
	}

	fmt.Println("================================================================================")
	fmt.Println("💡 配置完成！后续使用建议:")
	fmt.Println("   • 启动网关主服务:   mgy")
	fmt.Println("   • 查看穿透状态:     mgy cloudflare status (或 mgy cf status)")
	fmt.Println("   • 移动端配对二维码: mgy pair")
	fmt.Println("================================================================================")
}
