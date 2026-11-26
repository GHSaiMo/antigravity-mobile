package inspector

import (
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ProcessDetails describes the running language_server process as far as the OS lets us see it.
type ProcessDetails struct {
	// ExecutablePath is the absolute path of the language_server binary ("" when it cannot be determined).
	ExecutablePath string
	// Version is the Antigravity version the process was started with (--override_ide_version), "" when unknown.
	Version string
}

var (
	versionArgRegex         = regexp.MustCompile(`--override_ide_version[=\s]+([0-9][0-9A-Za-z._-]*)`)
	headlessArgRegex        = regexp.MustCompile(`(?:^|\s)--headless(?:$|\s|=)`)
	remoteControlServeRegex = regexp.MustCompile(`(?i)(?:^|\s)remote-control\s+serve(?:$|\s)`)
)

// VersionFromCommandLine extracts the Antigravity version from a language_server command line.
func VersionFromCommandLine(cmdline string) string {
	if m := versionArgRegex.FindStringSubmatch(cmdline); len(m) > 1 {
		return m[1]
	}
	return ""
}

func excludeHeadlessProcesses() bool {
	raw := strings.TrimSpace(os.Getenv("MGY_EXCLUDE_HEADLESS"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("MULTIGRAVITY_EXCLUDE_HEADLESS"))
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func excludeProcessPatterns() []string {
	raw := strings.TrimSpace(os.Getenv("MGY_EXCLUDE_PROCESS_PATTERNS"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("MULTIGRAVITY_EXCLUDE_PROCESS_PATTERNS"))
	}
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	patterns := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			patterns = append(patterns, p)
		}
	}
	return patterns
}

// ShouldExcludeProcessCmdline reports whether a process command line should be excluded from
// Antigravity discovery and lifecycle management. Generic OS/helper noise (grep, <defunct>,
// multicall) is always skipped; optional user-specific exclusions are controlled via
// MGY_EXCLUDE_HEADLESS (or MULTIGRAVITY_EXCLUDE_HEADLESS) and MGY_EXCLUDE_PROCESS_PATTERNS
// (or MULTIGRAVITY_EXCLUDE_PROCESS_PATTERNS, comma-separated case-insensitive substrings).
func ShouldExcludeProcessCmdline(cmdline string) bool {
	if cmdline == "" ||
		strings.Contains(cmdline, "grep") ||
		strings.Contains(cmdline, "<defunct>") ||
		strings.Contains(cmdline, "multicall") {
		return true
	}
	if excludeHeadlessProcesses() && headlessArgRegex.MatchString(cmdline) {
		return true
	}
	if patterns := excludeProcessPatterns(); len(patterns) > 0 {
		lower := strings.ToLower(cmdline)
		for _, pat := range patterns {
			if strings.Contains(lower, pat) {
				return true
			}
		}
	}
	return false
}

func baseFromAnyOSPath(p string) string {
	p = strings.Trim(strings.TrimSpace(p), `"'`)
	if p == "" {
		return ""
	}
	if idx := strings.LastIndexAny(p, `/\`); idx != -1 {
		return p[idx+1:]
	}
	return p
}

func isAntigravityCLIBase(base string) bool {
	switch strings.ToLower(strings.Trim(strings.TrimSpace(base), `"'`)) {
	case "agy", "agy.exe", "antigravity-cli", "antigravity-cli.exe":
		return true
	default:
		return false
	}
}

// IsAntigravityProcessBase reports whether an executable basename belongs to an Antigravity
// language_server binary or the Antigravity CLI (agy / antigravity-cli) binary.
func IsAntigravityProcessBase(base string) bool {
	lower := strings.ToLower(strings.Trim(strings.TrimSpace(base), `"'`))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "language_server") || isAntigravityCLIBase(lower)
}

func isAllDigitsASCII(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func nextShellToken(s string) (string, string) {
	s = strings.TrimLeft(s, " \t")
	if s == "" {
		return "", ""
	}
	if s[0] == '"' || s[0] == '\'' {
		quote := s[0]
		if end := strings.IndexByte(s[1:], quote); end != -1 {
			tok := s[1 : 1+end]
			rest := strings.TrimLeft(s[1+end+1:], " \t")
			return tok, rest
		}
	}
	if sp := strings.IndexAny(s, " \t"); sp != -1 {
		return s[:sp], strings.TrimLeft(s[sp+1:], " \t")
	}
	return s, ""
}

func isEnvAssignmentToken(tok string) bool {
	eq := strings.IndexByte(tok, '=')
	if eq <= 0 {
		return false
	}
	name := tok[:eq]
	for i := 0; i < len(name); i++ {
		c := name[i]
		isAlpha := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
		isDigit := c >= '0' && c <= '9'
		if (i == 0 && !isAlpha) || (i > 0 && !isAlpha && !isDigit) {
			return false
		}
	}
	return true
}

// stripAllowedWrapperPrefix strips an explicit, narrow allowlist of transparent proxy / env
// launcher prefixes (proxychains, proxychains4, proxychains-ng, graftcp, env) so that
// Antigravity language_server processes launched via proxy-injection scripts remain discoverable,
// while arbitrary script wrappers (python3, sh, node, etc.) continue to be rejected.
func stripAllowedWrapperPrefix(s string) string {
	cur := strings.TrimSpace(s)
	for depth := 0; depth < 4 && cur != ""; depth++ {
		tok, rest := nextShellToken(cur)
		if tok == "" {
			break
		}
		base := strings.ToLower(baseFromAnyOSPath(tok))
		switch base {
		case "proxychains", "proxychains4", "proxychains-ng":
			next := rest
			for next != "" {
				flagTok, after := nextShellToken(next)
				if !strings.HasPrefix(flagTok, "-") {
					break
				}
				next = after
				if flagTok == "--" {
					break
				}
				if flagTok == "-f" {
					_, next = nextShellToken(next)
				}
			}
			cur = next
		case "graftcp":
			next := rest
			for next != "" {
				flagTok, after := nextShellToken(next)
				if !strings.HasPrefix(flagTok, "-") {
					break
				}
				next = after
				if flagTok == "--" {
					break
				}
				if !strings.Contains(flagTok, "=") {
					switch flagTok {
					case "-a", "-p", "-c", "-f", "-l", "-s", "--config", "--listen", "--socks5", "--http_proxy":
						_, next = nextShellToken(next)
					}
				}
			}
			cur = next
		case "env":
			next := rest
			for next != "" {
				t, after := nextShellToken(next)
				if t == "" {
					break
				}
				if t == "--" {
					next = after
					break
				}
				if t == "-u" || t == "--unset" {
					_, next = nextShellToken(after)
					continue
				}
				if t == "-i" || t == "-0" || t == "--ignore-environment" || strings.HasPrefix(t, "--unset=") {
					next = after
					continue
				}
				if isEnvAssignmentToken(t) {
					next = after
					continue
				}
				break
			}
			cur = next
		default:
			return cur
		}
	}
	return cur
}

// ExtractCmdExecutableBase extracts the executable basename from either a raw command line
// or a `ps -eo pid,command` line (which starts with a numeric PID column).
func ExtractCmdExecutableBase(cmdline string) string {
	s := strings.TrimSpace(cmdline)
	if s == "" {
		return ""
	}
	// Strip leading numeric PID column when invoked on `ps -eo pid,command` output.
	if sp := strings.IndexAny(s, " \t"); sp > 0 && isAllDigitsASCII(s[:sp]) {
		s = strings.TrimSpace(s[sp+1:])
	}
	s = stripAllowedWrapperPrefix(s)
	if s == "" {
		return ""
	}
	// Handle quoted executable paths (e.g. Windows `"C:\Program Files\agy\agy.exe" --csrf_token=...`).
	if s[0] == '"' || s[0] == '\'' {
		quote := s[0]
		if end := strings.IndexByte(s[1:], quote); end != -1 {
			return baseFromAnyOSPath(s[1 : 1+end])
		}
	}
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	firstBase := baseFromAnyOSPath(fields[0])
	if IsAntigravityProcessBase(firstBase) {
		return firstBase
	}
	// Fallback for unquoted paths containing spaces (e.g. `/Users/my user/bin/agy --csrf_token=...`):
	// inspect the prefix before the first CLI flag or `remote-control` subcommand only when the
	// command starts with a path separator or drive letter AND the prefix does not contain separate
	// arguments (such as ` -c`, ` /other/path`, or embedded quotes from wrappers like `python3` or `sh`).
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, `\`) || (len(s) >= 3 && s[1] == ':' && (s[2] == '\\' || s[2] == '/')) {
		cut := len(s)
		for _, marker := range []string{" --", "\t--", " remote-control", "\tremote-control"} {
			if idx := strings.Index(strings.ToLower(s), marker); idx != -1 && idx < cut {
				cut = idx
			}
		}
		prefix := s[:cut]
		if !strings.Contains(prefix, " /") &&
			!strings.Contains(prefix, "\t/") &&
			!strings.Contains(prefix, ` \`) &&
			!strings.Contains(prefix, "\t\\") &&
			!strings.Contains(prefix, " -") &&
			!strings.Contains(prefix, "\t-") &&
			!strings.ContainsAny(prefix, `"'`) {
			if candidate := baseFromAnyOSPath(prefix); IsAntigravityProcessBase(candidate) {
				return candidate
			}
		}
	}
	return firstBase
}

// ClassifyLanguageServerCmdline evaluates a process command line for fallback process-table discovery.
// It returns (ok, priority) where higher priority prefers the official Desktop GUI language_server
// (--host_bridge_url / --csrf_token / Antigravity.app bundle) over Antigravity CLI (agy /
// antigravity-cli) and headless instances. User-configured exclusions (MGY_EXCLUDE_HEADLESS /
// MGY_EXCLUDE_PROCESS_PATTERNS) are rejected immediately.
func ClassifyLanguageServerCmdline(cmdline string) (bool, int) {
	if ShouldExcludeProcessCmdline(cmdline) {
		return false, 0
	}

	isHeadless := headlessArgRegex.MatchString(cmdline)

	if strings.Contains(cmdline, "language_server") {
		if isHeadless {
			priority := 1
			if csrfRegex.MatchString(cmdline) {
				priority++
			}
			return true, priority
		}
		priority := 3
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

	// Antigravity CLI (agy / antigravity-cli) embeds language_server inside a single binary.
	// Require --csrf_token or `remote-control serve` so short-lived CLI helper commands are skipped.
	base := ExtractCmdExecutableBase(cmdline)
	if !isAntigravityCLIBase(base) {
		return false, 0
	}
	hasCSRF := csrfRegex.MatchString(cmdline)
	hasRemoteServe := remoteControlServeRegex.MatchString(cmdline)
	if !hasCSRF && !hasRemoteServe {
		return false, 0
	}
	if isHeadless {
		return true, 1
	}
	priority := 2
	if hasCSRF {
		priority++
	}
	if hasRemoteServe {
		priority++
	}
	return true, priority
}

type cachedProcessDetails struct {
	details   ProcessDetails
	expiresAt time.Time
}

var (
	procDetailsCacheMu sync.RWMutex
	procDetailsCache   = make(map[int]cachedProcessDetails)
)

// LookupProcessDetails returns the executable path and version of the process with the given PID.
// It never fails: whatever cannot be determined is left empty, and callers treat that as "unknown".
func LookupProcessDetails(pid int) ProcessDetails {
	if pid <= 0 {
		return ProcessDetails{}
	}
	now := time.Now()
	procDetailsCacheMu.RLock()
	if entry, ok := procDetailsCache[pid]; ok && now.Before(entry.expiresAt) {
		procDetailsCacheMu.RUnlock()
		return entry.details
	}
	procDetailsCacheMu.RUnlock()

	d := lookupProcessDetails(pid)
	if d.ExecutablePath != "" || d.Version != "" {
		procDetailsCacheMu.Lock()
		if len(procDetailsCache) >= 64 {
			procDetailsCache = make(map[int]cachedProcessDetails)
		}
		procDetailsCache[pid] = cachedProcessDetails{
			details:   d,
			expiresAt: now.Add(30 * time.Second),
		}
		procDetailsCacheMu.Unlock()
	}
	return d
}
