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
		if line == "" || strings.Contains(line, "grep") || strings.Contains(line, "<defunct>") {
			continue
		}

		if strings.Contains(line, "language_server") && strings.Contains(line, "--csrf_token") {
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

			matches := csrfRegex.FindStringSubmatch(line)
			if len(matches) > 1 {
				return pid, matches[1], nil
			}
		}
	}

	return 0, "", fmt.Errorf("language_server process not found")
}

// findListeningPorts uses lsof to query TCP LISTEN ports for a given PID on POSIX systems.
func (i *Inspector) findListeningPorts(ctx context.Context, pid int) ([]int, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-a", "-p", strconv.Itoa(pid))
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run lsof: %w", err)
	}

	var ports []int
	seen := make(map[int]bool)

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

	return ports, nil
}
