//go:build !windows

package inspector

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// findProcess uses ps to find the language_server process and extract PID & CSRF token on POSIX systems.
func (i *Inspector) findProcess(ctx context.Context) (int, string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "ps", "-eo", "pid,command")
	out, err := cmd.Output()
	if err != nil {
		return 0, "", fmt.Errorf("failed to run ps: %w", err)
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "grep") || strings.Contains(line, "<defunct>") || strings.Contains(line, "multicall") {
			continue
		}

		if strings.Contains(line, "language_server") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			pid, err := strconv.Atoi(fields[0])
			if err != nil {
				continue
			}

			// Verify the command actually executes language_server (not a shell wrapper or python script)
			cmdPath := fields[1]
			base := filepath.Base(cmdPath)
			if !strings.Contains(base, "language_server") {
				continue
			}

			var csrfToken string
			matches := csrfRegex.FindStringSubmatch(line)
			if len(matches) > 1 {
				csrfToken = matches[1]
			}
			return pid, csrfToken, nil
		}
	}

	return 0, "", fmt.Errorf("language_server process not found")
}

// findListeningPorts uses lsof and ss to query TCP LISTEN ports for a given PID on POSIX systems.
func (i *Inspector) findListeningPorts(ctx context.Context, pid int) ([]int, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var ports []int
	seen := make(map[int]bool)

	// 1. Primary method: lsof
	cmd := exec.CommandContext(cmdCtx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-a", "-p", strconv.Itoa(pid))
	out, lsofErr := cmd.Output()
	if lsofErr == nil {
		matches := lsofRegex.FindAllStringSubmatch(string(out), -1)
		for _, m := range matches {
			if len(m) > 1 {
				port, err := strconv.Atoi(m[1])
				if err == nil && !seen[port] {
					seen[port] = true
					ports = append(ports, port)
				}
			}
		}
	}

	// 2. Linux fallback: ss -H -tlnp
	if len(ports) == 0 {
		ssCmd := exec.CommandContext(cmdCtx, "ss", "-H", "-tlnp")
		if ssOut, ssErr := ssCmd.Output(); ssErr == nil {
			pidMarkerComma := fmt.Sprintf("pid=%d,", pid)
			pidMarkerClose := fmt.Sprintf("pid=%d)", pid)
			for _, line := range strings.Split(string(ssOut), "\n") {
				if !strings.Contains(line, pidMarkerComma) && !strings.Contains(line, pidMarkerClose) {
					continue
				}
				fields := strings.Fields(line)
				for _, f := range fields {
					idx := strings.LastIndex(f, ":")
					if idx != -1 {
						if port, err := strconv.Atoi(f[idx+1:]); err == nil && port > 0 && !seen[port] {
							seen[port] = true
							ports = append(ports, port)
						}
					}
				}
			}
		}
	}

	if len(ports) == 0 && lsofErr != nil {
		return nil, fmt.Errorf("failed to run lsof: %w", lsofErr)
	}

	return ports, nil
}
