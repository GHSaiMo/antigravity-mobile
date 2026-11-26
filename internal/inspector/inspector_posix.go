//go:build !windows

package inspector

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

func isAntigravityDesktopProcessLine(line string) bool {
	if ShouldExcludeProcessCmdline(line) {
		return false
	}
	if strings.Contains(line, "Antigravity.app/Contents/MacOS/Antigravity") ||
		strings.Contains(line, "Antigravity IDE.app/Contents/MacOS/") {
		return true
	}
	base := strings.ToLower(ExtractCmdExecutableBase(line))
	return base == "antigravity" || base == "antigravity-ide"
}

// findProcessCandidates uses ps to find all matching language_server / agy processes on POSIX
// systems, sorted by descending priority then PID so Scan() can fall back across multiple PIDs.
func (i *Inspector) findProcessCandidates(ctx context.Context) processScanResult {
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "ps", "-eo", "pid,command")
	out, err := cmd.Output()
	if err != nil {
		return processScanResult{Err: fmt.Errorf("failed to run ps: %w", err)}
	}

	var res processScanResult
	seenPID := make(map[int]bool)

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !res.AppRunning && isAntigravityDesktopProcessLine(line) {
			res.AppRunning = true
		}

		ok, priority := ClassifyLanguageServerCmdline(line)
		if !ok {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 || seenPID[pid] {
			continue
		}

		// Verify the command actually executes language_server or agy / antigravity-cli
		// (optionally under an allowlisted transparent proxy launcher like proxychains4/graftcp/env).
		base := ExtractCmdExecutableBase(line)
		if !IsAntigravityProcessBase(base) {
			continue
		}

		var csrfToken string
		matches := csrfRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			csrfToken = matches[1]
		}

		seenPID[pid] = true
		res.Candidates = append(res.Candidates, processCandidate{
			PID:       pid,
			CSRFToken: csrfToken,
			Priority:  priority,
		})
	}

	sort.Slice(res.Candidates, func(a, b int) bool {
		if res.Candidates[a].Priority != res.Candidates[b].Priority {
			return res.Candidates[a].Priority > res.Candidates[b].Priority
		}
		return res.Candidates[a].PID > res.Candidates[b].PID
	})

	if len(res.Candidates) == 0 {
		res.Err = fmt.Errorf("language_server process not found")
	}
	return res
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
