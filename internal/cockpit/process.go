package cockpit

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"antigravity-mobile/internal/inspector"
	"antigravity-mobile/internal/procutil"
)

func runCmdTimeout(d time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return procutil.HideWindow(exec.CommandContext(ctx, name, args...)).Output()
}

func runCmdCombinedTimeout(d time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return procutil.HideWindow(exec.CommandContext(ctx, name, args...)).CombinedOutput()
}

func runCmdRunTimeout(d time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	return procutil.HideWindow(exec.CommandContext(ctx, name, args...)).Run()
}

func runCmdStdinTimeout(d time.Duration, stdin string, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := procutil.HideWindow(exec.CommandContext(ctx, name, args...))
	cmd.Stdin = strings.NewReader(stdin)
	return cmd.Output()
}

func runCmdCombinedStdinTimeout(d time.Duration, stdin string, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	cmd := procutil.HideWindow(exec.CommandContext(ctx, name, args...))
	cmd.Stdin = strings.NewReader(stdin)
	return cmd.CombinedOutput()
}

// linuxAntigravitySystemdServices lists the supported systemd user services managing
// Antigravity backends on Linux (Antigravity CLI daemon and standalone language_server).
var linuxAntigravitySystemdServices = []string{
	"antigravity-cli-daemon.service",
	"antigravity-ls.service",
}

// Test seams for Linux systemd/process management; nil-safe defaults point at real commands.
var (
	linuxSystemctlRunner = func(d time.Duration, args ...string) error {
		fullArgs := append([]string{"--user"}, args...)
		return runCmdRunTimeout(d, "systemctl", fullArgs...)
	}
	linuxLookPath          = exec.LookPath
	linuxStartStandaloneLS = func(lsPath string) error {
		cmd := exec.Command(lsPath, "--standalone", "--headless", "--persistent_mode", "--http_server_port=50005")
		return cmd.Start()
	}
	linuxFindPIDsFn   = findLinuxAntigravityPIDs
	linuxWaitExitedFn = waitUntilAntigravityExited
	linuxSignalFn     = signalAntigravity

	stoppedLinuxServicesMu sync.Mutex
	stoppedLinuxServices   []string
)

func recordStoppedLinuxServices(svcs []string) {
	if len(svcs) == 0 {
		return
	}
	stoppedLinuxServicesMu.Lock()
	defer stoppedLinuxServicesMu.Unlock()
	seen := make(map[string]struct{}, len(stoppedLinuxServices)+len(svcs))
	for _, s := range stoppedLinuxServices {
		seen[s] = struct{}{}
	}
	for _, s := range svcs {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			stoppedLinuxServices = append(stoppedLinuxServices, s)
		}
	}
}

func takeStoppedLinuxServices() []string {
	stoppedLinuxServicesMu.Lock()
	defer stoppedLinuxServicesMu.Unlock()
	if len(stoppedLinuxServices) == 0 {
		return nil
	}
	out := append([]string(nil), stoppedLinuxServices...)
	stoppedLinuxServices = nil
	return out
}

func clearStoppedLinuxServices() {
	stoppedLinuxServicesMu.Lock()
	stoppedLinuxServices = nil
	stoppedLinuxServicesMu.Unlock()
}

func activeLinuxAntigravityServices() []string {
	var active []string
	for _, svc := range linuxAntigravitySystemdServices {
		if linuxSystemctlRunner(3*time.Second, "is-active", "--quiet", svc) == nil {
			active = append(active, svc)
		}
	}
	return active
}

func quitLinuxAntigravity() error {
	activeServices := activeLinuxAntigravityServices()
	if len(activeServices) > 0 {
		recordStoppedLinuxServices(activeServices)
		for _, svc := range activeServices {
			slog.Info(fmt.Sprintf("[Cockpit] stopping systemd service %s", svc))
			_ = linuxSystemctlRunner(8*time.Second, "stop", svc)
		}
	} else {
		linuxSignalFn(syscall.SIGTERM)
	}
	if linuxWaitExitedFn(5 * time.Second) {
		slog.Info("[Cockpit] Antigravity stopped cleanly on Linux")
		return nil
	}
	if len(linuxFindPIDsFn()) > 0 {
		if len(activeServices) > 0 {
			linuxSignalFn(syscall.SIGTERM)
			if linuxWaitExitedFn(2 * time.Second) {
				slog.Info("[Cockpit] Antigravity stopped cleanly on Linux")
				return nil
			}
		}
		linuxSignalFn(syscall.SIGKILL)
	}
	return nil
}

func launchLinuxAntigravity() error {
	if stopped := takeStoppedLinuxServices(); len(stopped) > 0 {
		for _, svc := range stopped {
			slog.Info(fmt.Sprintf("[Cockpit] restarting systemd service %s", svc))
			if err := linuxSystemctlRunner(10*time.Second, "restart", svc); err != nil {
				return fmt.Errorf("restart %s: %w", svc, err)
			}
		}
		return nil
	}

	var existing []string
	for _, svc := range linuxAntigravitySystemdServices {
		if linuxSystemctlRunner(3*time.Second, "cat", svc) == nil {
			existing = append(existing, svc)
		}
	}
	if len(existing) > 0 {
		var toRestart []string
		for _, svc := range existing {
			if linuxSystemctlRunner(3*time.Second, "is-active", "--quiet", svc) == nil ||
				linuxSystemctlRunner(3*time.Second, "is-enabled", "--quiet", svc) == nil {
				toRestart = append(toRestart, svc)
			}
		}
		if len(toRestart) == 0 {
			toRestart = existing[:1]
		}
		for _, svc := range toRestart {
			slog.Info(fmt.Sprintf("[Cockpit] restarting systemd service %s", svc))
			if err := linuxSystemctlRunner(10*time.Second, "restart", svc); err != nil {
				return fmt.Errorf("restart %s: %w", svc, err)
			}
		}
		return nil
	}

	if lsPath, err := linuxLookPath("language_server"); err == nil {
		slog.Info("[Cockpit] launching standalone language_server")
		return linuxStartStandaloneLS(lsPath)
	}
	slog.Info("[Cockpit] no language_server, antigravity-ls, or antigravity-cli-daemon service found on Linux")
	return nil
}

// parseLinuxAntigravityPIDs parses `ps -o pid=,command=` output and returns verified
// Antigravity Desktop, language_server, and Antigravity CLI (agy / antigravity-cli) daemon PIDs,
// excluding selfPID, short-lived CLI helper commands, and excluded processes.
func parseLinuxAntigravityPIDs(psOutput string, selfPID int) []int {
	var pids []int
	seen := make(map[int]bool)
	for _, rawLine := range strings.Split(psOutput, "\n") {
		line := strings.TrimSpace(rawLine)
		if inspector.ShouldExcludeProcessCmdline(line) {
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
		base := strings.ToLower(inspector.ExtractCmdExecutableBase(line))
		isDesktopOrLS := strings.HasPrefix(base, "language_server") || base == "antigravity" || base == "antigravity-ide"
		isCLIDaemon := false
		if !isDesktopOrLS && inspector.IsAntigravityProcessBase(base) {
			ok, _ := inspector.ClassifyLanguageServerCmdline(line)
			isCLIDaemon = ok
		}
		if isDesktopOrLS || isCLIDaemon {
			seen[pid] = true
			pids = append(pids, pid)
		}
	}
	return pids
}

// findLinuxAntigravityPIDs returns verified Antigravity / language_server / agy daemon PIDs
// owned by the current user on Linux, excluding the gateway's own PID and isolated bot runtimes.
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

	return parseLinuxAntigravityPIDs(string(out), os.Getpid())
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
		if len(activeLinuxAntigravityServices()) > 0 {
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
		return quitLinuxAntigravity()
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
