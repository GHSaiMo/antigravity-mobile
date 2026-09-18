//go:build darwin

package netutil

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// IPv6Status holds macOS network service IPv6 configuration status.
type IPv6Status struct {
	ServiceName string
	Device      string
	IsAutomatic bool
	CurrentMode string // e.g. "Automatic", "Off", "Link-local only"
}

var (
	reServiceOrder = regexp.MustCompile(`(?m)^\((\d+)\)\s+(.+)$`)
	reDevice       = regexp.MustCompile(`(?i)Device:\s*([a-z0-9]+)`)
	reDefaultRoute = regexp.MustCompile(`(?i)interface:\s*([a-z0-9]+)`)
	reIPv6Mode     = regexp.MustCompile(`(?im)^IPv6:\s*(.+)$`)
)

// GetDefaultInterface returns the default routing network interface on macOS (e.g. "en0").
func GetDefaultInterface() string {
	out, err := exec.Command("route", "-n", "get", "default").Output()
	if err == nil {
		if match := reDefaultRoute.FindSubmatch(out); len(match) > 1 {
			return strings.TrimSpace(string(match[1]))
		}
	}
	return "en0" // Fallback to en0
}

// FindServiceForDevice finds the macOS network service name (e.g. "Ethernet", "Wi-Fi")
// corresponding to the given BSD interface name (e.g. "en0").
func FindServiceForDevice(device string) string {
	out, err := exec.Command("networksetup", "-listnetworkserviceorder").Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(string(out), "\n")
	currentService := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "An asterisk") {
			continue
		}
		if match := reServiceOrder.FindStringSubmatch(trimmed); len(match) > 2 {
			currentService = match[2]
			continue
		}
		if currentService != "" {
			if match := reDevice.FindStringSubmatch(trimmed); len(match) > 1 {
				dev := match[1]
				if strings.EqualFold(dev, device) {
					// Check that this service is not disabled
					if !strings.HasPrefix(currentService, "*") {
						return strings.TrimPrefix(currentService, "*")
					}
				}
			}
		}
	}
	return ""
}

// CheckMacOSIPv6Status detects the active network service on macOS and checks whether
// IPv6 is configured as Automatic.
func CheckMacOSIPv6Status() (*IPv6Status, error) {
	iface := GetDefaultInterface()
	service := FindServiceForDevice(iface)
	if service == "" {
		// Fallback: try common service names
		for _, tryName := range []string{"Ethernet", "Wi-Fi"} {
			if out, err := exec.Command("networksetup", "-getinfo", tryName).Output(); err == nil && len(out) > 0 {
				service = tryName
				break
			}
		}
	}
	if service == "" {
		return nil, fmt.Errorf("could not identify active macOS network service")
	}

	out, err := exec.Command("networksetup", "-getinfo", service).Output()
	if err != nil {
		return nil, fmt.Errorf("networksetup -getinfo %s: %w", service, err)
	}

	mode := "Unknown"
	if match := reIPv6Mode.FindSubmatch(out); len(match) > 1 {
		mode = strings.TrimSpace(string(match[1]))
	}

	isAuto := strings.EqualFold(mode, "Automatic")
	return &IPv6Status{
		ServiceName: service,
		Device:      iface,
		IsAutomatic: isAuto,
		CurrentMode: mode,
	}, nil
}

// EnsureMacOSIPv6Automatic checks if the active network service has IPv6 disabled/non-automatic,
// and if so, executes `networksetup -setv6automatic <service>` to enable it.
// Returns the status, whether a change was made, and any execution error.
func EnsureMacOSIPv6Automatic() (*IPv6Status, bool, error) {
	status, err := CheckMacOSIPv6Status()
	if err != nil || status == nil {
		return nil, false, err
	}

	if status.IsAutomatic {
		return status, false, nil
	}

	// Attempt to set to automatic
	cmd := exec.Command("networksetup", "-setv6automatic", status.ServiceName)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if runErr := cmd.Run(); runErr != nil {
		return status, false, fmt.Errorf("failed to set IPv6 automatic on %q: %v (%s)", status.ServiceName, runErr, strings.TrimSpace(stderr.String()))
	}

	status.IsAutomatic = true
	status.CurrentMode = "Automatic"
	return status, true, nil
}
