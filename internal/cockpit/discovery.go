package cockpit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	portCacheMutex     sync.RWMutex
	cachedReportPort   int
	cachedReportExpiry time.Time
	cachedWsPort       int
	cachedWsExpiry     time.Time

	// portCacheTTL is how long an auto-detected port remains valid before re-verifying.
	portCacheTTL = 5 * time.Minute

	// listenPortRegex matches TCP listening port numbers from lsof / netstat output.
	listenPortRegex = regexp.MustCompile(`(?i):([0-9]{4,5})\s+.*(?:LISTEN|LISTENING)`)
)

// InvalidatePortCache clears the in-memory cached active ports.
func InvalidatePortCache() {
	portCacheMutex.Lock()
	defer portCacheMutex.Unlock()
	cachedReportPort = 0
	cachedReportExpiry = time.Time{}
	cachedWsPort = 0
	cachedWsExpiry = time.Time{}
}

// VerifyReportPort tests whether a given TCP port responds to Cockpit's /report HTTP query.
// SEC (MED-08): Probes without sending the secret report token so blind port scans never leak credentials
// to arbitrary local listeners, and requires a Cockpit signature when HTTP 200 is returned.
func VerifyReportPort(port int, _ string, timeout time.Duration) bool {
	if port <= 0 {
		return false
	}
	if !IsCockpitListening(port, timeout) {
		return false
	}

	reqURL := fmt.Sprintf("http://127.0.0.1:%d/report?format=yaml", port)
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, reqURL, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Cockpit Tools responds with 401 Unauthorized when token is omitted/invalid
	if resp.StatusCode == http.StatusUnauthorized {
		return true
	}

	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	content := string(buf)
	hasSignature := strings.Contains(content, "service:") || strings.Contains(content, "metric:") || strings.Contains(content, "Cockpit")
	if resp.StatusCode == http.StatusOK {
		return hasSignature
	}
	return hasSignature
}

// VerifyWsPort tests whether a given TCP port accepts WebSocket connections.
func VerifyWsPort(port int, timeout time.Duration) bool {
	if port <= 0 {
		return false
	}
	if !IsCockpitListening(port, timeout) {
		return false
	}

	wsURL := url.URL{Scheme: "ws", Host: fmt.Sprintf("127.0.0.1:%d", port), Path: "/"}
	dialer := websocket.Dialer{HandshakeTimeout: timeout}
	conn, resp, err := dialer.Dial(wsURL.String(), nil)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return false
	}
	_ = conn.Close()
	return true
}

// GetListeningPortsForPID extracts all TCP listening ports for a given process PID.
func GetListeningPortsForPID(pid int) []int {
	if pid <= 0 {
		return nil
	}

	var output []byte
	var err error

	if runtime.GOOS == "windows" {
		out, runErr := runCmdTimeout(5*time.Second, "netstat", "-ano")
		if runErr == nil {
			pidStr := strconv.Itoa(pid)
			var matchedLines []string
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, pidStr) && strings.Contains(strings.ToUpper(line), "LISTENING") {
					matchedLines = append(matchedLines, line)
				}
			}
			output = []byte(strings.Join(matchedLines, "\n"))
		}
	} else {
		// macOS / Linux: use lsof
		output, err = runCmdTimeout(5*time.Second, "lsof", "-nP", "-a", "-iTCP", "-sTCP:LISTEN", "-p", strconv.Itoa(pid))
		if err != nil || len(output) == 0 {
			return nil
		}
	}

	matches := listenPortRegex.FindAllStringSubmatch(string(output), -1)
	if len(matches) == 0 {
		return nil
	}

	var ports []int
	seen := make(map[int]bool)
	for _, m := range matches {
		if len(m) > 1 {
			if p, convErr := strconv.Atoi(m[1]); convErr == nil && p > 0 && !seen[p] {
				seen[p] = true
				ports = append(ports, p)
			}
		}
	}
	return ports
}

func isCockpitCommandLine(cmdline string) bool {
	lower := strings.ToLower(strings.TrimSpace(cmdline))
	if lower == "" || strings.Contains(lower, "<defunct>") {
		return false
	}
	fields := strings.Fields(cmdline)
	if len(fields) > 0 {
		base := strings.ToLower(filepath.Base(fields[0]))
		if base == "cockpit-tools" || base == "cockpit-tools.exe" {
			return true
		}
	}
	return strings.Contains(cmdline, "Cockpit Tools.app/Contents/MacOS/") ||
		strings.Contains(lower, "/cockpit-tools") ||
		strings.Contains(lower, `\cockpit-tools.exe`)
}

func isTrustedCockpitPID(pid int) bool {
	if pid <= 0 {
		return false
	}
	if pid == os.Getpid() {
		return true
	}
	if runtime.GOOS == "windows" {
		out, err := runCmdTimeout(3*time.Second, "tasklist", "/fi", fmt.Sprintf("PID eq %d", pid), "/nh")
		if err != nil {
			return false
		}
		return strings.Contains(strings.ToLower(string(out)), "cockpit-tools")
	}
	args := []string{"-p", strconv.Itoa(pid), "-o", "uid=,command="}
	out, err := runCmdTimeout(3*time.Second, "ps", args...)
	if err != nil {
		return false
	}
	line := strings.TrimSpace(string(out))
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return false
	}
	if uid, convErr := strconv.Atoi(fields[0]); convErr == nil && os.Getuid() >= 0 && uid != os.Getuid() {
		return false
	}
	cmdPart := strings.TrimSpace(line[len(fields[0]):])
	return isCockpitCommandLine(cmdPart)
}

// FindCockpitPIDs locates running Cockpit Tools PIDs owned by the current user.
func FindCockpitPIDs() []int {
	var pids []int
	seen := make(map[int]bool)

	// Source 1: server.json (verified alive and owned by current user / Cockpit binary)
	if serverInfo, err := ReadRawCockpitServerInfo(); err == nil && serverInfo.PID > 0 {
		if isTrustedCockpitPID(serverInfo.PID) {
			seen[serverInfo.PID] = true
			pids = append(pids, serverInfo.PID)
		}
	}

	// Source 2: UID-scoped process table scan on POSIX (never cross-user broad pgrep -f)
	if runtime.GOOS != "windows" {
		var out []byte
		var err error
		if uid := os.Getuid(); uid >= 0 {
			out, err = runCmdTimeout(3*time.Second, "ps", "-u", strconv.Itoa(uid), "-o", "pid=,command=")
		} else {
			out, err = runCmdTimeout(3*time.Second, "ps", "-eo", "pid=,command=")
		}
		if err == nil {
			for _, rawLine := range strings.Split(string(out), "\n") {
				line := strings.TrimSpace(rawLine)
				fields := strings.Fields(line)
				if len(fields) < 2 {
					continue
				}
				p, convErr := strconv.Atoi(fields[0])
				if convErr != nil || p <= 0 || seen[p] {
					continue
				}
				cmdPart := strings.TrimSpace(line[len(fields[0]):])
				if isCockpitCommandLine(cmdPart) {
					seen[p] = true
					pids = append(pids, p)
				}
			}
		}
	}

	return pids
}

// isCandidatePortOwnedByTrustedProcess verifies that an unconfigured fallback port belongs to a verified
// Cockpit Tools process (or the current process in unit tests) before sending any credentials to it.
func isCandidatePortOwnedByTrustedProcess(port int, trustedPIDs []int) bool {
	if port <= 0 {
		return false
	}
	trustedSet := make(map[int]bool, len(trustedPIDs)+1)
	trustedSet[os.Getpid()] = true
	for _, p := range trustedPIDs {
		trustedSet[p] = true
		for _, lp := range GetListeningPortsForPID(p) {
			if lp == port {
				return true
			}
		}
	}
	if runtime.GOOS == "windows" {
		return false
	}
	out, err := runCmdTimeout(3*time.Second, "lsof", "-nP", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-Fp")
	if err != nil || len(out) == 0 {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "p") {
			if pid, convErr := strconv.Atoi(line[1:]); convErr == nil && (trustedSet[pid] || isTrustedCockpitPID(pid)) {
				return true
			}
		}
	}
	return false
}

// candidateReportPorts constructs an ordered list of candidate ports to probe for Cockpit Report.
func candidateReportPorts(configuredPort int) []int {
	var candidates []int
	seen := make(map[int]bool)

	add := func(p int) {
		if p > 0 && !seen[p] {
			seen[p] = true
			candidates = append(candidates, p)
		}
	}

	// 1. Configured port
	add(configuredPort)

	// 2. Standard defaults
	add(18081)

	// 3. Sequential fallback ports if 18081 was in use
	for p := 18082; p <= 18086; p++ {
		add(p)
	}

	// 4. WebSocket ports in case user flipped them
	add(19528)
	add(19529)

	return candidates
}

// candidateWsPorts constructs an ordered list of candidate ports to probe for Cockpit WebSocket API.
func candidateWsPorts(configuredWsPort, serverJsonWsPort int) []int {
	var candidates []int
	seen := make(map[int]bool)

	add := func(p int) {
		if p > 0 && !seen[p] {
			seen[p] = true
			candidates = append(candidates, p)
		}
	}

	add(serverJsonWsPort)
	add(configuredWsPort)
	add(19528)
	add(18081)
	add(19529)
	add(19530)

	return candidates
}

// ResolveActiveReportPort detects the actual listening ReportPort for Cockpit Tools.
// It checks cache -> configuredPort -> OS-level process listening ports -> verified candidate port pool,
// using tokenless pre-probing and PID ownership verification to prevent token leakage.
func ResolveActiveReportPort(token string, configuredPort int) (int, error) {
	now := time.Now()

	// 1. Check in-memory cache
	portCacheMutex.RLock()
	cached := cachedReportPort
	expiry := cachedReportExpiry
	portCacheMutex.RUnlock()

	if cached > 0 && now.Before(expiry) {
		if VerifyReportPort(cached, token, 300*time.Millisecond) {
			return cached, nil
		}
	}

	// 2. Check configured port directly (tokenless VerifyReportPort)
	if configuredPort > 0 && VerifyReportPort(configuredPort, token, 400*time.Millisecond) {
		updateReportCache(configuredPort)
		return configuredPort, nil
	}

	// 3. Process-level discovery: inspect ports listening by verified cockpit-tools PIDs
	pids := FindCockpitPIDs()
	for _, pid := range pids {
		ports := GetListeningPortsForPID(pid)
		for _, port := range ports {
			if VerifyReportPort(port, token, 400*time.Millisecond) {
				slog.Info(fmt.Sprintf("[Cockpit] 🎯 Auto-detected active Report port via process (PID %d): %d (configured was %d)", pid, port, configuredPort))
				updateReportCache(port)
				return port, nil
			}
		}
	}

	// 4. Scan candidate ports only when verified to belong to a trusted Cockpit process
	if len(pids) > 0 {
		candidates := candidateReportPorts(configuredPort)
		for _, port := range candidates {
			if port == configuredPort {
				continue
			}
			if !isCandidatePortOwnedByTrustedProcess(port, pids) {
				continue
			}
			if VerifyReportPort(port, token, 250*time.Millisecond) {
				slog.Info(fmt.Sprintf("[Cockpit] 🎯 Auto-detected active Report port via verified candidate scan: %d (configured was %d)", port, configuredPort))
				updateReportCache(port)
				return port, nil
			}
		}
	}

	return 0, fmt.Errorf("cockpit report service not found on configured port %d or any verified candidate ports", configuredPort)
}

// ResolveActiveWsPort detects the actual listening WebSocket port for Cockpit Tools.
func ResolveActiveWsPort() (int, string, error) {
	now := time.Now()

	// 1. Check in-memory cache
	portCacheMutex.RLock()
	cached := cachedWsPort
	expiry := cachedWsExpiry
	portCacheMutex.RUnlock()

	serverInfo, serverErr := ReadRawCockpitServerInfo()
	authToken := ""
	serverJsonPort := 0
	if serverErr == nil && serverInfo != nil {
		authToken = serverInfo.AuthToken
		serverJsonPort = serverInfo.WsPort
	}

	if cached > 0 && now.Before(expiry) {
		if VerifyWsPort(cached, 300*time.Millisecond) {
			return cached, authToken, nil
		}
	}

	// 2. Check server.json port
	if serverJsonPort > 0 && VerifyWsPort(serverJsonPort, 400*time.Millisecond) {
		updateWsCache(serverJsonPort)
		return serverJsonPort, authToken, nil
	}

	// 3. Process-level discovery (verified Cockpit PIDs only)
	pids := FindCockpitPIDs()
	for _, pid := range pids {
		ports := GetListeningPortsForPID(pid)
		for _, port := range ports {
			if VerifyWsPort(port, 400*time.Millisecond) {
				slog.Info(fmt.Sprintf("[Cockpit] 🎯 Auto-detected active WebSocket port via process (PID %d): %d (server.json was %d)", pid, port, serverJsonPort))
				updateWsCache(port)
				return port, authToken, nil
			}
		}
	}

	// 4. Candidate port pool (configured port directly; unconfigured ports require trusted PID ownership)
	var configuredWsPort int
	if cfg, err := getCockpitConfig(); err == nil && cfg != nil {
		type cfgWithWs struct {
			WsPort int `json:"ws_port"`
		}
		dataDir, dErr := GetCockpitDataDir()
		if dErr == nil {
			if rawBytes, err := os.ReadFile(filepath.Join(dataDir, "config.json")); err == nil {
				var w cfgWithWs
				_ = json.Unmarshal(rawBytes, &w)
				configuredWsPort = w.WsPort
			}
		}
	}

	if configuredWsPort > 0 && VerifyWsPort(configuredWsPort, 250*time.Millisecond) {
		slog.Info(fmt.Sprintf("[Cockpit] 🎯 Auto-detected active WebSocket port via configured ws_port: %d", configuredWsPort))
		updateWsCache(configuredWsPort)
		return configuredWsPort, authToken, nil
	}

	if len(pids) > 0 {
		candidates := candidateWsPorts(configuredWsPort, serverJsonPort)
		for _, port := range candidates {
			if port == serverJsonPort || port == configuredWsPort {
				continue
			}
			if !isCandidatePortOwnedByTrustedProcess(port, pids) {
				continue
			}
			if VerifyWsPort(port, 250*time.Millisecond) {
				slog.Info(fmt.Sprintf("[Cockpit] 🎯 Auto-detected active WebSocket port via verified candidate scan: %d", port))
				updateWsCache(port)
				return port, authToken, nil
			}
		}
	}

	return 0, "", fmt.Errorf("cockpit websocket service not responding on any probed ports")
}

func updateReportCache(port int) {
	portCacheMutex.Lock()
	defer portCacheMutex.Unlock()
	cachedReportPort = port
	cachedReportExpiry = time.Now().Add(portCacheTTL)
}

func updateWsCache(port int) {
	portCacheMutex.Lock()
	defer portCacheMutex.Unlock()
	cachedWsPort = port
	cachedWsExpiry = time.Now().Add(portCacheTTL)
}
