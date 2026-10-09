package inspector

import "regexp"

// ProcessDetails describes the running language_server process as far as the OS lets us see it.
type ProcessDetails struct {
	// ExecutablePath is the absolute path of the language_server binary ("" when it cannot be determined).
	ExecutablePath string
	// Version is the Antigravity version the process was started with (--override_ide_version), "" when unknown.
	Version string
}

var versionArgRegex = regexp.MustCompile(`--override_ide_version[=\s]+([0-9][0-9A-Za-z._-]*)`)

// VersionFromCommandLine extracts the Antigravity version from a language_server command line.
func VersionFromCommandLine(cmdline string) string {
	if m := versionArgRegex.FindStringSubmatch(cmdline); len(m) > 1 {
		return m[1]
	}
	return ""
}

// LookupProcessDetails returns the executable path and version of the process with the given PID.
// It never fails: whatever cannot be determined is left empty, and callers treat that as "unknown".
func LookupProcessDetails(pid int) ProcessDetails {
	if pid <= 0 {
		return ProcessDetails{}
	}
	return lookupProcessDetails(pid)
}
