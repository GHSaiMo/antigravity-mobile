package cockpit

import (
	"fmt"
	"log"
	"os/exec"
	"runtime"
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
	patterns := []string{antigravityBundlePattern, antigravityIDEBundlePattern}
	for _, pat := range patterns {
		_ = exec.Command("pkill", fmt.Sprintf("-%d", sig), "-f", pat).Run()
	}
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
	if runtime.GOOS == "darwin" {
		for _, name := range []string{antigravityAppName, antigravityIDEAppName} {
			script := fmt.Sprintf("tell application %q to quit", name)
			_ = exec.Command("osascript", "-e", script).Run()
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
