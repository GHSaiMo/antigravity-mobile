package inspector

import (
	"regexp"
	"strings"
)

// ProcessDetails describes the running language_server process as far as the OS lets us see it.
type ProcessDetails struct {
	// ExecutablePath is the absolute path of the language_server binary ("" when it cannot be determined).
	ExecutablePath string
	// Version is the Antigravity version the process was started with (--override_ide_version), "" when unknown.
	Version string
}

var (
	versionArgRegex  = regexp.MustCompile(`--override_ide_version[=\s]+([0-9][0-9A-Za-z._-]*)`)
	headlessArgRegex = regexp.MustCompile(`(?:^|\s)--headless(?:$|\s|=)`)
)

// VersionFromCommandLine extracts the Antigravity version from a language_server command line.
func VersionFromCommandLine(cmdline string) string {
	if m := versionArgRegex.FindStringSubmatch(cmdline); len(m) > 1 {
		return m[1]
	}
	return ""
}

// ClassifyLanguageServerCmdline evaluates a process command line for fallback process-table discovery.
// It returns (ok, priority) where higher priority prefers the official Desktop GUI language_server
// (--host_bridge_url / --csrf_token / Antigravity.app bundle) and rejects isolated headless/bot
// instances (--headless, groupchat-runtime) that do not belong to the user's main desktop session.
func ClassifyLanguageServerCmdline(cmdline string) (bool, int) {
	if cmdline == "" ||
		!strings.Contains(cmdline, "language_server") ||
		strings.Contains(cmdline, "grep") ||
		strings.Contains(cmdline, "<defunct>") ||
		strings.Contains(cmdline, "multicall") {
		return false, 0
	}
	// Legitimate headless instances under $HOME already write ~/.gemini/antigravity/daemon/ls_*.json
	// and are discovered via findFromDaemon(). Any --headless process reached via raw ps fallback
	// (such as groupchat-runtime with an isolated HOME) must be excluded.
	if headlessArgRegex.MatchString(cmdline) || strings.Contains(cmdline, "groupchat-runtime") {
		return false, 0
	}

	priority := 1
	if strings.Contains(cmdline, "--host_bridge_url") {
		priority += 4
	}
	if csrfRegex.MatchString(cmdline) {
		priority += 2
	}
	if strings.Contains(cmdline, "Antigravity.app/") || strings.Contains(cmdline, "Antigravity IDE.app/") {
		priority += 2
	}
	return true, priority
}

// LookupProcessDetails returns the executable path and version of the process with the given PID.
// It never fails: whatever cannot be determined is left empty, and callers treat that as "unknown".
func LookupProcessDetails(pid int) ProcessDetails {
	if pid <= 0 {
		return ProcessDetails{}
	}
	return lookupProcessDetails(pid)
}
