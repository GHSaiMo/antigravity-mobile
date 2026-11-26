//go:build windows

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"antigravity-mobile/internal/service"
	"antigravity-mobile/internal/updater"
)

const (
	winCreateNewProcessGroup = 0x00000200
	winDetachedProcess       = 0x00000008
	winCreateNoWindow        = 0x08000000
)

func detectSelfRestartMode(ctx context.Context) string {
	if isRunningUnderWindowsService(ctx) {
		return "service"
	}
	return "spawn"
}

func isRunningUnderWindowsService(ctx context.Context) bool {
	cctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	mgr, err := service.NewManager()
	if err != nil {
		return false
	}
	st, err := mgr.Status(cctx)
	return err == nil && st.Installed && st.Running
}

func escapePowerShellSingleQuoted(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// performSelfRestart spawns a hidden detached helper that waits for the current PID to exit
// (releasing listening ports) and then restarts either the Windows scheduled service or mgy.exe.
func performSelfRestart(binaryPath string) {
	target := strings.TrimSpace(binaryPath)
	if target == "" {
		if resolved, err := updater.ResolveExecutablePath(""); err == nil {
			target = resolved
		}
	}
	if target == "" {
		slog.Error("❌ [SelfRestart] 无法解析新版本可执行文件路径，请手动重启 mgy.exe")
		return
	}

	pid := os.Getpid()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	useService := isRunningUnderWindowsService(ctx)
	cancel()

	var launchStmt string
	if useService {
		launchStmt = fmt.Sprintf("schtasks.exe /run /tn %s | Out-Null", escapePowerShellSingleQuoted(service.DefaultWindowsTaskName))
	} else {
		argsExpr := ""
		if len(os.Args) > 1 {
			quoted := make([]string, 0, len(os.Args)-1)
			for _, a := range os.Args[1:] {
				quoted = append(quoted, escapePowerShellSingleQuoted(a))
			}
			argsExpr = fmt.Sprintf(" -ArgumentList @(%s)", strings.Join(quoted, ", "))
		}
		launchStmt = fmt.Sprintf("Start-Process -FilePath %s%s -WindowStyle Hidden", escapePowerShellSingleQuoted(target), argsExpr)
	}

	script := fmt.Sprintf(
		"try { Wait-Process -Id %d -Timeout 12 -ErrorAction SilentlyContinue } catch {}; Start-Sleep -Milliseconds 450; %s",
		pid, launchStmt,
	)

	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: winCreateNewProcessGroup | winDetachedProcess | winCreateNoWindow,
	}
	if err := cmd.Start(); err != nil {
		slog.Error("❌ [SelfRestart] 拉起新版本重启助手失败，请手动重启 mgy.exe", "err", err)
		return
	}
	_ = cmd.Process.Release()
	slog.Info("🚀 [SelfRestart] 已交接给后台重启助手，当前旧进程即将退出...", "binary", target, "service", useService)
}
