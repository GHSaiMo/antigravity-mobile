//go:build windows

package inspector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"antigravity-mobile/internal/procutil"
)

type winProcessInfo struct {
	ProcessID   int    `json:"ProcessId"`
	Name        string `json:"Name"`
	CommandLine string `json:"CommandLine"`
}

// findProcessCandidates uses PowerShell and Get-CimInstance to locate all matching language_server /
// agy processes on Windows (sorted by descending priority then PID), while also noting whether
// Antigravity.exe is running or whether UAC elevation hid CommandLine from a non-elevated gateway.
func (i *Inspector) findProcessCandidates(ctx context.Context) processScanResult {
	cmdCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	psScript := `Get-CimInstance Win32_Process | Where-Object { $_.Name -like '*language_server*' -or $_.Name -eq 'agy.exe' -or $_.Name -eq 'antigravity-cli.exe' -or $_.Name -like 'Antigravity*' } | Select-Object ProcessId,Name,CommandLine | ConvertTo-Json -Compress`
	cmd := procutil.HideWindow(exec.CommandContext(cmdCtx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", psScript))
	out, err := cmd.Output()
	if err != nil {
		return processScanResult{Err: fmt.Errorf("failed to query language_server via PowerShell: %w", err)}
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "null" {
		return processScanResult{Err: fmt.Errorf("language_server process not found")}
	}

	var procs []winProcessInfo
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &procs); err != nil {
			return processScanResult{Err: fmt.Errorf("failed to parse process list JSON: %w", err)}
		}
	} else if strings.HasPrefix(trimmed, "{") {
		var single winProcessInfo
		if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
			return processScanResult{Err: fmt.Errorf("failed to parse process JSON: %w", err)}
		}
		procs = append(procs, single)
	} else {
		return processScanResult{Err: fmt.Errorf("unexpected output from PowerShell: %s", trimmed)}
	}

	var res processScanResult
	seenPID := make(map[int]bool)

	for _, p := range procs {
		if p.ProcessID <= 0 {
			continue
		}
		nameLower := strings.ToLower(strings.TrimSpace(p.Name))
		if strings.HasPrefix(nameLower, "antigravity") && nameLower != "antigravity-cli.exe" {
			res.AppRunning = true
			continue
		}

		isLSName := strings.Contains(nameLower, "language_server") ||
			nameLower == "agy.exe" ||
			nameLower == "antigravity-cli.exe"
		if !isLSName {
			continue
		}

		cmdline := strings.TrimSpace(p.CommandLine)
		if cmdline == "" {
			// On Windows, querying a process started with Administrator privileges from a normal
			// user token yields an empty CommandLine. Record AccessDenied for clear diagnostics and
			// still try probing its listening ports at lowest priority in case CSRF is empty.
			res.AccessDenied = true
			if !seenPID[p.ProcessID] {
				seenPID[p.ProcessID] = true
				res.Candidates = append(res.Candidates, processCandidate{
					PID:       p.ProcessID,
					CSRFToken: "",
					Priority:  0,
				})
			}
			continue
		}

		ok, priority := ClassifyLanguageServerCmdline(cmdline)
		if !ok || seenPID[p.ProcessID] {
			continue
		}
		var csrfToken string
		matches := csrfRegex.FindStringSubmatch(cmdline)
		if len(matches) > 1 {
			csrfToken = matches[1]
		}
		seenPID[p.ProcessID] = true
		res.Candidates = append(res.Candidates, processCandidate{
			PID:       p.ProcessID,
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

func parseWindowsNetstatPorts(out []byte, pidStr string, seen map[int]bool, ports *[]int) {
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
			if (strings.EqualFold(state, "LISTENING") || strings.Contains(strings.ToUpper(state), "LISTEN")) && rowPID == pidStr {
				localAddr := fields[1]
				if idx := strings.LastIndex(localAddr, ":"); idx != -1 {
					portStr := localAddr[idx+1:]
					if port, err := strconv.Atoi(portStr); err == nil && port > 0 && !seen[port] {
						seen[port] = true
						*ports = append(*ports, port)
					}
				}
			}
		}
	}
}

// findListeningPorts uses netstat -ano -p tcp (and tcpv6 fallback) to query TCP LISTEN ports for a given PID on Windows.
func (i *Inspector) findListeningPorts(ctx context.Context, pid int) ([]int, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	pidStr := strconv.Itoa(pid)
	var ports []int
	seen := make(map[int]bool)

	out, err := procutil.HideWindow(exec.CommandContext(cmdCtx, "netstat", "-ano", "-p", "tcp")).Output()
	if err == nil {
		parseWindowsNetstatPorts(out, pidStr, seen, &ports)
	}
	if len(ports) == 0 {
		if outV6, errV6 := procutil.HideWindow(exec.CommandContext(cmdCtx, "netstat", "-ano", "-p", "tcpv6")).Output(); errV6 == nil {
			parseWindowsNetstatPorts(outV6, pidStr, seen, &ports)
		} else if err != nil {
			return nil, fmt.Errorf("failed to run netstat: %w", err)
		}
	}

	return ports, nil
}
