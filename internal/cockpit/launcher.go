package cockpit

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	execLaunchCockpit     = defaultLaunchCockpit
	cockpitProcessChecker = isCockpitProcessRunning
)

// isCockpitProcessRunning checks if the Cockpit Tools executable process is already running.
func isCockpitProcessRunning() bool {
	if runtime.GOOS == "windows" {
		out, err := exec.Command("tasklist", "/fi", "IMAGENAME eq cockpit-tools.exe").Output()
		if err == nil && strings.Contains(strings.ToLower(string(out)), "cockpit-tools.exe") {
			return true
		}
		return false
	}
	if runtime.GOOS != "darwin" {
		cmd := exec.Command("pgrep", "-f", "cockpit-tools")
		return cmd.Run() == nil
	}
	// macOS: check exact binary name or bundle path
	if exec.Command("pgrep", "-x", "cockpit-tools").Run() == nil {
		return true
	}
	if exec.Command("pgrep", "-f", "Cockpit Tools.app").Run() == nil {
		return true
	}
	return false
}

func sanitizeBundleID(id string) string {
	var clean strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			clean.WriteRune(r)
		}
	}
	return clean.String()
}

func getFrontmostAppBundleID() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	out, err := exec.Command("osascript", "-e", `tell application "System Events" to get bundle identifier of first application process whose frontmost is true`).Output()
	if err != nil {
		return ""
	}
	return sanitizeBundleID(strings.TrimSpace(string(out)))
}

func guardMacOSFocus(prevBundleID string) {
	if runtime.GOOS != "darwin" {
		return
	}
	go func() {
		checkDelays := []time.Duration{200 * time.Millisecond, 600 * time.Millisecond, 1500 * time.Millisecond}
		for _, delay := range checkDelays {
			time.Sleep(delay)

			// 1. Ensure Cockpit Tools window remains hidden
			script := `tell application "System Events"
	if exists (process "Cockpit Tools") then
		if visible of process "Cockpit Tools" is true then
			set visible of process "Cockpit Tools" to false
		end if
	end if
end tell`
			_ = exec.Command("osascript", "-e", script).Run()

			// 2. If the user had an active application and Cockpit stole focus, restore it
			if prevBundleID != "" && prevBundleID != "com.jlcodes.cockpit-tools" {
				restoreScript := fmt.Sprintf(`tell application "System Events"
	set frontApp to bundle identifier of first application process whose frontmost is true
	if frontApp is "com.jlcodes.cockpit-tools" then
		tell application id "%s" to activate
	end if
end tell`, prevBundleID)
				_ = exec.Command("osascript", "-e", restoreScript).Run()
			}
		}
	}()
}

func defaultLaunchCockpit() error {
	// Guard: if Cockpit Tools is already running, skip launching to avoid sending
	// a reopen event that activates the window and steals foreground focus.
	if cockpitProcessChecker() {
		log.Println("[Cockpit] Cockpit Tools process is already running; skipping launch to prevent focus stealing")
		return nil
	}

	if runtime.GOOS == "windows" {
		var candidatePaths []string
		if localApp := os.Getenv("LOCALAPPDATA"); localApp != "" {
			candidatePaths = append(candidatePaths, filepath.Join(localApp, "Cockpit Tools", "cockpit-tools.exe"))
		}
		if progFiles := os.Getenv("ProgramFiles"); progFiles != "" {
			candidatePaths = append(candidatePaths, filepath.Join(progFiles, "Cockpit Tools", "cockpit-tools.exe"))
		}
		if home, err := os.UserHomeDir(); err == nil {
			candidatePaths = append(candidatePaths, filepath.Join(home, "AppData", "Local", "Cockpit Tools", "cockpit-tools.exe"))
		}
		for _, path := range candidatePaths {
			if _, err := os.Stat(path); err == nil {
				cmd := exec.Command(path)
				setHideWindow(cmd)
				return cmd.Start()
			}
		}
		return fmt.Errorf("Cockpit Tools executable not found in AppData or ProgramFiles")
	}

	if runtime.GOOS != "darwin" {
		return fmt.Errorf("auto-launching Cockpit Tools is only supported on macOS and Windows (current: %s)", runtime.GOOS)
	}

	// Capture currently active app before launching to restore focus if needed
	prevAppID := getFrontmostAppBundleID()

	// Priority 1: Launch via app bundle name silently in background (-j -g)
	cmd := exec.Command("open", "-j", "-g", "-a", "Cockpit Tools")
	if err := cmd.Run(); err == nil {
		guardMacOSFocus(prevAppID)
		return nil
	}

	// Priority 2: Fallback to standard /Applications path
	appPath := "/Applications/Cockpit Tools.app"
	if _, err := os.Stat(appPath); err == nil {
		cmdFallback := exec.Command("open", "-j", "-g", appPath)
		if err := cmdFallback.Run(); err == nil {
			guardMacOSFocus(prevAppID)
			return nil
		}
		return err
	}

	return fmt.Errorf("Cockpit Tools application not found in /Applications")
}

// LaunchCockpitApp launches Cockpit Tools silently in the background without stealing focus.
func LaunchCockpitApp() error {
	return execLaunchCockpit()
}

// IsCockpitListening tests if the Cockpit Tools HTTP/TCP port is accepting connections.
func IsCockpitListening(port int, timeout time.Duration) bool {
	if port <= 0 {
		return false
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
