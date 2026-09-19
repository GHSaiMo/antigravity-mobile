package cockpit

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

var execLaunchCockpit = defaultLaunchCockpit

func defaultLaunchCockpit() error {
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
				return cmd.Start()
			}
		}
		return fmt.Errorf("Cockpit Tools executable not found in AppData or ProgramFiles")
	}

	if runtime.GOOS != "darwin" {
		return fmt.Errorf("auto-launching Cockpit Tools is only supported on macOS and Windows (current: %s)", runtime.GOOS)
	}

	// Priority 1: Launch via app bundle name silently in background (-g)
	cmd := exec.Command("open", "-g", "-a", "Cockpit Tools")
	if err := cmd.Run(); err == nil {
		return nil
	}

	// Priority 2: Fallback to standard /Applications path
	appPath := "/Applications/Cockpit Tools.app"
	if _, err := os.Stat(appPath); err == nil {
		cmdFallback := exec.Command("open", "-g", appPath)
		return cmdFallback.Run()
	}

	return fmt.Errorf("Cockpit Tools application not found in /Applications")
}

// LaunchCockpitApp launches Cockpit Tools in the background on macOS without stealing focus (-g).
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
