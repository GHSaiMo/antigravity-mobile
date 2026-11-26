//go:build !windows

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"antigravity-mobile/internal/updater"
)

func detectSelfRestartMode(_ context.Context) string {
	return "exec"
}

// performSelfRestart replaces the current process image in-place with the newly installed
// binary while preserving PID, CLI flags, environment variables, and tmux/service attachment.
// All listeners and child processes must already be closed before calling this function.
func performSelfRestart(binaryPath string) {
	target := strings.TrimSpace(binaryPath)
	if target == "" {
		if resolved, err := updater.ResolveExecutablePath(""); err == nil {
			target = resolved
		}
	}
	if target == "" {
		slog.Error("❌ [SelfRestart] 无法解析新版本二进制路径，请手动重启 mgy")
		os.Exit(1)
	}

	signal.Reset(os.Interrupt, syscall.SIGTERM)
	argv := os.Args
	if len(argv) == 0 {
		argv = []string{target}
	}

	slog.Info("🚀 [SelfRestart] 正在原地切换至新版本进程镜像...", "binary", target)
	if err := syscall.Exec(target, argv, os.Environ()); err != nil {
		slog.Error("❌ [SelfRestart] 原地切换失败，进程退出以待服务管理器或用户拉起", "binary", target, "err", err)
		os.Exit(1)
	}
}
