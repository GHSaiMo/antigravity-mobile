//go:build windows

package inspector

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

func lookupProcessDetails(pid int) ProcessDetails {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	script := fmt.Sprintf(`Get-CimInstance Win32_Process -Filter "ProcessId=%d" | Select-Object ExecutablePath,CommandLine | ConvertTo-Json -Compress`, pid)
	out, err := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		return ProcessDetails{}
	}
	var info struct {
		ExecutablePath string `json:"ExecutablePath"`
		CommandLine    string `json:"CommandLine"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(string(out))), &info) != nil {
		return ProcessDetails{}
	}
	return ProcessDetails{ExecutablePath: info.ExecutablePath, Version: VersionFromCommandLine(info.CommandLine)}
}
