//go:build !windows

package inspector

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
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

	var (
		bestPID      int
		bestCSRF     string
		bestPriority int
	)

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		ok, priority := ClassifyLanguageServerCmdline(line)
		if !ok {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			continue
		}

		// Verify the command actually executes language_server or agy / antigravity-cli (not a shell wrapper or python script)
		base := ExtractCmdExecutableBase(line)
		if !IsAntigravityProcessBase(base) {
			continue
		}

		var csrfToken string
		matches := csrfRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			csrfToken = matches[1]
		}

		if priority > bestPriority || (priority == bestPriority && pid > bestPID) {
			bestPID = pid
			bestCSRF = csrfToken
			bestPriority = priority
		}
	}

	if bestPID > 0 {
		return bestPID, bestCSRF, nil
	}

	return 0, "", fmt.Errorf("language_server process not found")
}

// findListeningPorts uses ss (Linux primary) and lsof (macOS primary / Linux fallback)
// to query TCP LISTEN ports for a given PID on POSIX systems.
func (i *Inspector) findListeningPorts(ctx context.Context, pid int) ([]int, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var ports []int
	seen := make(map[int]bool)

	trySS := func() {
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

	var lsofErr error
	tryLsof := func() {
		cmd := exec.CommandContext(cmdCtx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-a", "-p", strconv.Itoa(pid))
		out, err := cmd.Output()
		lsofErr = err
		if err == nil {
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
	}

	if runtime.GOOS == "linux" {
		trySS()
		if len(ports) == 0 {
			tryLsof()
		}
	} else {
		tryLsof()
		if len(ports) == 0 {
			trySS()
		}
	}

	if len(ports) == 0 && lsofErr != nil {
		return nil, fmt.Errorf("failed to run lsof: %w", lsofErr)
	}

	return ports, nil
}
