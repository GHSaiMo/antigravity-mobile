package cockpit

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func runCmdTimeout(d time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}

func runCmdCombinedTimeout(d time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func runCmdRunTimeout(d time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Run()
}

func runCmdStdinTimeout(d time.Duration, stdin string, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	return cmd.Output()
}

const (
	antigravityAppName          = "Antigravity"
	antigravityIDEAppName       = "Antigravity IDE"
	antigravityMainPattern      = `/Applications/Antigravity.app/Contents/MacOS/Antigravity`
	antigravityBundlePattern    = `/Applications/Antigravity.app/`
	antigravityIDEBundlePattern = `/Applications/Antigravity IDE.app/`
)

// quitAntigravityBeforeSwitch is the pre-switch hook. Tests replace it with a no-op.
var quitAntigravityBeforeSwitch = quitRunningAntigravity

func antigravityStillRunning() bool {
	if runtime.GOOS == "windows" {
		out, err := runCmdTimeout(5*time.Second, "tasklist", "/fi", "IMAGENAME eq Antigravity.exe")
		if err == nil && strings.Contains(string(out), "Antigravity.exe") {
			return true
		}
		return false
	}
	if runtime.GOOS == "linux" {
		if runCmdRunTimeout(3*time.Second, "systemctl", "--user", "is-active", "--quiet", "antigravity-ls") == nil {
			return true
		}
		for _, pat := range []string{"language_server", "antigravity"} {
			if runCmdRunTimeout(3*time.Second, "pgrep", "-f", pat) == nil {
				return true
			}
		}
		return false
	}
	if runtime.GOOS != "darwin" {
		return runCmdRunTimeout(3*time.Second, "pgrep", "-f", "Antigravity.app") == nil
	}
	for _, pat := range []string{antigravityMainPattern, antigravityBundlePattern, antigravityIDEBundlePattern} {
		if runCmdRunTimeout(3*time.Second, "pgrep", "-f", pat) == nil {
			return true
		}
	}
	return false
}

func waitUntilAntigravityExited(d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if !antigravityStillRunning() {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return !antigravityStillRunning()
}

func signalAntigravity(sig syscall.Signal) {
	if runtime.GOOS == "windows" {
		_ = runCmdRunTimeout(5*time.Second, "taskkill", "/F", "/IM", "Antigravity.exe", "/T")
		_ = runCmdRunTimeout(5*time.Second, "taskkill", "/F", "/IM", "language_server.exe", "/T")
		return
	}
	if runtime.GOOS == "linux" {
		_ = runCmdRunTimeout(3*time.Second, "pkill", fmt.Sprintf("-%d", sig), "-f", "language_server")
		return
	}
	patterns := []string{antigravityBundlePattern, antigravityIDEBundlePattern}
	for _, pat := range patterns {
		_ = runCmdRunTimeout(3*time.Second, "pkill", fmt.Sprintf("-%d", sig), "-f", pat)
	}
}

var allowedQuitAppNames = map[string]bool{
	antigravityAppName:    true,
	antigravityIDEAppName: true,
}

// quitAppDarwin safely tells a whitelisted application to quit via osascript.
// It enforces an explicit allowlist to prevent arbitrary AppleScript injection.
func quitAppDarwin(name string) error {
	if !allowedQuitAppNames[name] {
		return fmt.Errorf("application %q is not in the allowed quit list", name)
	}
	script := fmt.Sprintf("tell application %q to quit", name)
	return runCmdRunTimeout(3*time.Second, "osascript", "-e", script)
}

// quitRunningAntigravity stops the live Antigravity app so Cockpit can inject
// tokens into a cold profile. Cockpit's own closer looks at
// "Application Support/Antigravity IDE", which misses the running
// "Application Support/Antigravity" instance.
func quitRunningAntigravity() error {
	if !antigravityStillRunning() {
		slog.Info("[Cockpit] Antigravity is not running; skip pre-switch quit")
		return nil
	}

	slog.Info("[Cockpit] Quitting Antigravity before account switch")
	if runtime.GOOS == "windows" {
		// 1. Graceful close on Windows (sends WM_CLOSE)
		_ = runCmdRunTimeout(5*time.Second, "taskkill", "/IM", "Antigravity.exe")
		if waitUntilAntigravityExited(6 * time.Second) {
			slog.Info("[Cockpit] Antigravity quit cleanly on Windows")
			return nil
		}

		slog.Info("[Cockpit] Antigravity still running; force killing tree on Windows")
		_ = runCmdRunTimeout(5*time.Second, "taskkill", "/F", "/IM", "Antigravity.exe", "/T")
		_ = runCmdRunTimeout(5*time.Second, "taskkill", "/F", "/IM", "language_server.exe", "/T")
		if waitUntilAntigravityExited(3 * time.Second) {
			return nil
		}
		return fmt.Errorf("Antigravity still running after force kill on Windows")
	}

	if runtime.GOOS == "linux" {
		if runCmdRunTimeout(3*time.Second, "systemctl", "--user", "is-active", "--quiet", "antigravity-ls") == nil {
			_ = runCmdRunTimeout(5*time.Second, "systemctl", "--user", "stop", "antigravity-ls")
		} else {
			_ = runCmdRunTimeout(3*time.Second, "pkill", "-f", "language_server")
		}
		if waitUntilAntigravityExited(5 * time.Second) {
			slog.Info("[Cockpit] Antigravity stopped cleanly on Linux")
			return nil
		}
		_ = runCmdRunTimeout(3*time.Second, "pkill", "-9", "-f", "language_server")
		return nil
	}

	if runtime.GOOS == "darwin" {
		for _, name := range []string{antigravityAppName, antigravityIDEAppName} {
			_ = quitAppDarwin(name)
		}
	}
	if waitUntilAntigravityExited(8 * time.Second) {
		slog.Info("[Cockpit] Antigravity quit cleanly")
		return nil
	}

	slog.Info("[Cockpit] Antigravity still running; sending SIGTERM")
	signalAntigravity(syscall.SIGTERM)
	if waitUntilAntigravityExited(4 * time.Second) {
		return nil
	}

	slog.Info("[Cockpit] Antigravity still running; sending SIGKILL")
	signalAntigravity(syscall.SIGKILL)
	if waitUntilAntigravityExited(3 * time.Second) {
		return nil
	}
	return fmt.Errorf("Antigravity still running after quit")
}
