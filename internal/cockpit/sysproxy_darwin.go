//go:build darwin

package cockpit

import (
	"fmt"
	"log/slog"
	"net"
	"os/exec"
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
	cmd := exec.Command("scutil")
	cmd.Stdin = strings.NewReader("show State:/Network/Global/IPv4\nquit\n")
	if out, err := cmd.Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "PrimaryService") {
				parts := strings.Split(line, ":")
				if len(parts) >= 2 {
					serviceID := strings.TrimSpace(parts[1])
					nameCmd := exec.Command("scutil")
					nameCmd.Stdin = strings.NewReader(fmt.Sprintf("show Setup:/Network/Service/%s\nquit\n", serviceID))
					if nameOut, nErr := nameCmd.Output(); nErr == nil {
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
	routeCmd := exec.Command("route", "-n", "get", "default")
	if rOut, rErr := routeCmd.Output(); rErr == nil {
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
			orderCmd := exec.Command("networksetup", "-listnetworkserviceorder")
			if oOut, oErr := orderCmd.Output(); oErr == nil {
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
		if out, err := exec.Command("networksetup", "-getwebproxy", candidate).Output(); err == nil && len(out) > 0 {
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
	out, err := exec.Command("networksetup", "-getwebproxy", service).Output()
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
		if err := exec.Command("networksetup", "-setwebproxystate", service, "on").Run(); err != nil {
			slog.Warn("[Cockpit] Failed to enable webproxy", "service", service, "err", err)
		}
		if err := exec.Command("networksetup", "-setsecurewebproxystate", service, "on").Run(); err != nil {
			slog.Warn("[Cockpit] Failed to enable securewebproxy", "service", service, "err", err)
		}
		return true, nil
	}

	return false, nil
}
