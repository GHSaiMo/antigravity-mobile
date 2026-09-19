package cockpit

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
)

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
		out, err := exec.Command("tasklist", "/fi", "IMAGENAME eq Antigravity.exe").Output()
		if err == nil && strings.Contains(string(out), "Antigravity.exe") {
			return true
		}
		return false
	}
	if runtime.GOOS != "darwin" {
		cmd := exec.Command("pgrep", "-f", "Antigravity.app")
		return cmd.Run() == nil
	}
	for _, pat := range []string{antigravityMainPattern, antigravityBundlePattern, antigravityIDEBundlePattern} {
		if exec.Command("pgrep", "-f", pat).Run() == nil {
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
		_ = exec.Command("taskkill", "/F", "/IM", "Antigravity.exe", "/T").Run()
		_ = exec.Command("taskkill", "/F", "/IM", "language_server.exe", "/T").Run()
		return
	}
	patterns := []string{antigravityBundlePattern, antigravityIDEBundlePattern}
	for _, pat := range patterns {
		_ = exec.Command("pkill", fmt.Sprintf("-%d", sig), "-f", pat).Run()
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
	return exec.Command("osascript", "-e", script).Run()
}

// quitRunningAntigravity stops the live Antigravity app so Cockpit can inject
// tokens into a cold profile. Cockpit's own closer looks at
// "Application Support/Antigravity IDE", which misses the running
// "Application Support/Antigravity" instance.
func quitRunningAntigravity() error {
	if !antigravityStillRunning() {
		log.Printf("[Cockpit] Antigravity is not running; skip pre-switch quit")
		return nil
	}

	log.Printf("[Cockpit] Quitting Antigravity before account switch")
	if runtime.GOOS == "windows" {
		// 1. Graceful close on Windows (sends WM_CLOSE)
		_ = exec.Command("taskkill", "/IM", "Antigravity.exe").Run()
		if waitUntilAntigravityExited(6 * time.Second) {
			log.Printf("[Cockpit] Antigravity quit cleanly on Windows")
			return nil
		}

		log.Printf("[Cockpit] Antigravity still running; force killing tree on Windows")
		_ = exec.Command("taskkill", "/F", "/IM", "Antigravity.exe", "/T").Run()
		_ = exec.Command("taskkill", "/F", "/IM", "language_server.exe", "/T").Run()
		if waitUntilAntigravityExited(3 * time.Second) {
			return nil
		}
		return fmt.Errorf("Antigravity still running after force kill on Windows")
	}

	if runtime.GOOS == "darwin" {
		for _, name := range []string{antigravityAppName, antigravityIDEAppName} {
			_ = quitAppDarwin(name)
		}
	}
	if waitUntilAntigravityExited(8 * time.Second) {
		log.Printf("[Cockpit] Antigravity quit cleanly")
		return nil
	}

	log.Printf("[Cockpit] Antigravity still running; sending SIGTERM")
	signalAntigravity(syscall.SIGTERM)
	if waitUntilAntigravityExited(4 * time.Second) {
		return nil
	}

	log.Printf("[Cockpit] Antigravity still running; sending SIGKILL")
	signalAntigravity(syscall.SIGKILL)
	if waitUntilAntigravityExited(3 * time.Second) {
		return nil
	}
	return fmt.Errorf("Antigravity still running after quit")
}
