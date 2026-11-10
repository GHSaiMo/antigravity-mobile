//go:build darwin

package cockpit

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"
)

const (
	defaultClashProxyPort = 7890
)

// IsProxyPortListening checks if the specified local port is actively accepting TCP connections.
func IsProxyPortListening(port int, timeout time.Duration) bool {
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

// GetPrimaryNetworkService detects the primary active network service name (e.g. "Ethernet", "Wi-Fi").
func GetPrimaryNetworkService() (string, error) {
	// 1. Try scutil to resolve the PrimaryService ID
	if out, err := runCmdStdinTimeout(3*time.Second, "show State:/Network/Global/IPv4\nquit\n", "scutil"); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "PrimaryService") {
				parts := strings.Split(line, ":")
				if len(parts) >= 2 {
					serviceID := strings.TrimSpace(parts[1])
					if nameOut, nErr := runCmdStdinTimeout(3*time.Second, fmt.Sprintf("show Setup:/Network/Service/%s\nquit\n", serviceID), "scutil"); nErr == nil {
						for _, nLine := range strings.Split(string(nameOut), "\n") {
							if strings.Contains(nLine, "UserDefinedName") {
								nParts := strings.Split(nLine, ":")
								if len(nParts) >= 2 {
									name := strings.TrimSpace(nParts[1])
									if name != "" {
										return name, nil
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// 2. Fallback: inspect default route device and match via networksetup -listnetworkserviceorder
	if rOut, rErr := runCmdTimeout(3*time.Second, "route", "-n", "get", "default"); rErr == nil {
		var iface string
		for _, line := range strings.Split(string(rOut), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "interface:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					iface = fields[1]
					break
				}
			}
		}
		if iface != "" {
			if oOut, oErr := runCmdTimeout(3*time.Second, "networksetup", "-listnetworkserviceorder"); oErr == nil {
				lines := strings.Split(string(oOut), "\n")
				for i, l := range lines {
					if strings.Contains(l, fmt.Sprintf("Device: %s)", iface)) && i > 0 {
						prev := lines[i-1]
						if idx := strings.Index(prev, ") "); idx != -1 {
							name := strings.TrimSpace(prev[idx+2:])
							if name != "" {
								return name, nil
							}
						}
					}
				}
			}
		}
	}

	// 3. Fallback: try standard service names
	for _, candidate := range []string{"Ethernet", "Wi-Fi"} {
		if out, err := runCmdTimeout(3*time.Second, "networksetup", "-getwebproxy", candidate); err == nil && len(out) > 0 {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no primary active network service found")
}

// EnsureSystemProxy verifies whether local proxy port is running and safely restores
// the system proxy if it was configured but disabled.
func EnsureSystemProxy(targetPort int) (bool, error) {
	if targetPort <= 0 {
		targetPort = defaultClashProxyPort
	}

	// Safety check 1: Target port must be alive and accepting connections
	if !IsProxyPortListening(targetPort, 1*time.Second) {
		return false, nil
	}

	service, err := GetPrimaryNetworkService()
	if err != nil || service == "" {
		return false, err
	}

	// Safety check 2: Check current status
	out, err := runCmdTimeout(3*time.Second, "networksetup", "-getwebproxy", service)
	if err != nil {
		return false, err
	}

	strOut := string(out)
	// Only auto-enable if the server is already set to 127.0.0.1 or localhost,
	// indicating the user explicitly uses a local proxy client but it dropped/was disabled.
	isConfigured := strings.Contains(strOut, "127.0.0.1") || strings.Contains(strOut, "localhost")
	isDisabled := strings.Contains(strOut, "Enabled: No")

	if isConfigured && isDisabled {
		slog.Info(fmt.Sprintf("[Cockpit] 🔧 Detected local proxy %d is active but system proxy on '%s' is disabled. Re-enabling...", targetPort, service))
		if err := runCmdRunTimeout(3*time.Second, "networksetup", "-setwebproxystate", service, "on"); err != nil {
			slog.Warn("[Cockpit] Failed to enable webproxy", "service", service, "err", err)
		}
		if err := runCmdRunTimeout(3*time.Second, "networksetup", "-setsecurewebproxystate", service, "on"); err != nil {
			slog.Warn("[Cockpit] Failed to enable securewebproxy", "service", service, "err", err)
		}
		return true, nil
	}

	return false, nil
}
