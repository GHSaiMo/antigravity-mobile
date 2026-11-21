package cockpit

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
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

func runCmdCombinedStdinTimeout(d time.Duration, stdin string, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	return cmd.CombinedOutput()
}

// findLinuxAntigravityPIDs returns verified Antigravity / language_server PIDs owned by the
// current user on Linux, excluding the gateway's own PID and isolated bot runtimes.
func findLinuxAntigravityPIDs() []int {
	var out []byte
	var err error
	if uid := os.Getuid(); uid >= 0 {
		out, err = runCmdTimeout(3*time.Second, "ps", "-u", strconv.Itoa(uid), "-o", "pid=,command=")
	}
	if err != nil || len(out) == 0 {
		out, err = runCmdTimeout(3*time.Second, "ps", "-eo", "pid=,command=")
		if err != nil {
			return nil
		}
	}

	selfPID := os.Getpid()
	var pids []int
	seen := make(map[int]bool)
	for _, rawLine := range strings.Split(string(out), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.Contains(line, "<defunct>") || strings.Contains(line, "multicall") || strings.Contains(line, "groupchat-runtime") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, convErr := strconv.Atoi(fields[0])
		if convErr != nil || pid <= 0 || pid == selfPID || seen[pid] {
			continue
		}
		base := filepath.Base(fields[1])
		if strings.HasPrefix(base, "language_server") || base == "antigravity" || base == "antigravity-ide" {
			seen[pid] = true
			pids = append(pids, pid)
		}
	}
	return pids
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
		return len(findLinuxAntigravityPIDs()) > 0
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
		for _, pid := range findLinuxAntigravityPIDs() {
			_ = runCmdRunTimeout(3*time.Second, "kill", fmt.Sprintf("-%d", sig), strconv.Itoa(pid))
		}
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
			signalAntigravity(syscall.SIGTERM)
		}
		if waitUntilAntigravityExited(5 * time.Second) {
			slog.Info("[Cockpit] Antigravity stopped cleanly on Linux")
			return nil
		}
		signalAntigravity(syscall.SIGKILL)
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
