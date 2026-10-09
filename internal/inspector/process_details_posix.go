//go:build !windows

package inspector

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func lookupProcessDetails(pid int) ProcessDetails {
	var d ProcessDetails

	// Linux: /proc/<pid>/exe is the authoritative, space-safe path.
	if p, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid)); err == nil && p != "" {
		d.ExecutablePath = strings.TrimSuffix(p, " (deleted)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// macOS: `ps -o comm=` prints the full executable path (including spaces).
	if d.ExecutablePath == "" {
		if out, err := exec.CommandContext(ctx, "ps", "-p", fmt.Sprint(pid), "-o", "comm=").Output(); err == nil {
			if p := strings.TrimSpace(string(out)); strings.HasPrefix(p, "/") {
				d.ExecutablePath = p
			}
		}
	}

	if out, err := exec.CommandContext(ctx, "ps", "-p", fmt.Sprint(pid), "-o", "command=").Output(); err == nil {
		d.Version = VersionFromCommandLine(string(out))
	}
	return d
}
