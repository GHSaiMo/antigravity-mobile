//go:build windows

package inspector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type winProcessInfo struct {
	ProcessID   int    `json:"ProcessId"`
	CommandLine string `json:"CommandLine"`
}

// findProcess uses PowerShell and Get-CimInstance to locate language_server.exe and extract its PID & CSRF token on Windows.
func (i *Inspector) findProcess(ctx context.Context) (int, string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	psScript := `Get-CimInstance Win32_Process | Where-Object { $_.Name -like '*language_server*' } | Select-Object ProcessId,CommandLine | ConvertTo-Json -Compress`
	cmd := exec.CommandContext(cmdCtx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", psScript)
	out, err := cmd.Output()
	if err != nil {
		return 0, "", fmt.Errorf("failed to query language_server via PowerShell: %w", err)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "null" {
		return 0, "", fmt.Errorf("language_server process not found")
	}

	var procs []winProcessInfo
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &procs); err != nil {
			return 0, "", fmt.Errorf("failed to parse process list JSON: %w", err)
		}
	} else if strings.HasPrefix(trimmed, "{") {
		var single winProcessInfo
		if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
			return 0, "", fmt.Errorf("failed to parse process JSON: %w", err)
		}
		procs = append(procs, single)
	} else {
		return 0, "", fmt.Errorf("unexpected output from PowerShell: %s", trimmed)
	}

	for _, p := range procs {
		if strings.Contains(p.CommandLine, "language_server") && strings.Contains(p.CommandLine, "--csrf_token") {
			matches := csrfRegex.FindStringSubmatch(p.CommandLine)
			if len(matches) > 1 {
				return p.ProcessID, matches[1], nil
			}
		}
	}

	return 0, "", fmt.Errorf("language_server process found but no CSRF token in command line")
}

// findListeningPorts uses netstat -ano -p tcp to query TCP LISTEN ports for a given PID on Windows.
func (i *Inspector) findListeningPorts(ctx context.Context, pid int) ([]int, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "netstat", "-ano", "-p", "tcp")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run netstat: %w", err)
	}

	pidStr := strconv.Itoa(pid)
	var ports []int
	seen := make(map[int]bool)

	lines := bytes.Split(out, []byte("\n"))
	for _, rawLine := range lines {
		line := strings.TrimSpace(string(rawLine))
		if line == "" || !strings.HasPrefix(line, "TCP") {
			continue
		}

		fields := strings.Fields(line)
		// Expected netstat -ano output:
		// Proto  Local Address          Foreign Address        State           PID
		// TCP    127.0.0.1:57015        0.0.0.0:0              LISTENING       18788
		if len(fields) >= 5 {
			state := fields[3]
			rowPID := fields[4]
			if strings.EqualFold(state, "LISTENING") && rowPID == pidStr {
				localAddr := fields[1]
				if idx := strings.LastIndex(localAddr, ":"); idx != -1 {
					portStr := localAddr[idx+1:]
					if port, err := strconv.Atoi(portStr); err == nil && port > 0 && !seen[port] {
						seen[port] = true
						ports = append(ports, port)
					}
				}
			}
		}
	}

	return ports, nil
}
