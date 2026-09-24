package cockpit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
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
func VerifyReportPort(port int, token string, timeout time.Duration) bool {
	if port <= 0 {
		return false
	}
	if !IsCockpitListening(port, timeout) {
		return false
	}

	reqURL := fmt.Sprintf("http://127.0.0.1:%d/report?token=%s&format=yaml", port, token)
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

	// Cockpit Tools responds with 200 OK (valid token) or 401 Unauthorized (service exists, bad token)
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
		return true
	}

	// Also check if response content has Cockpit signature
	buf := make([]byte, 512)
	n, _ := io.ReadFull(resp.Body, buf)
	content := string(buf[:n])
	return strings.Contains(content, "service:") || strings.Contains(content, "metric:") || strings.Contains(content, "Cockpit")
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
	conn, _, err := dialer.Dial(wsURL.String(), nil)
	if err != nil {
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
		cmd := exec.Command("netstat", "-ano")
		out, runErr := cmd.Output()
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
		cmd := exec.Command("lsof", "-nP", "-a", "-iTCP", "-sTCP:LISTEN", "-p", strconv.Itoa(pid))
		output, err = cmd.Output()
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

// FindCockpitPIDs attempts to locate running Cockpit Tools PIDs across the system.
func FindCockpitPIDs() []int {
	var pids []int
	seen := make(map[int]bool)

	// Source 1: server.json
	if serverInfo, err := ReadRawCockpitServerInfo(); err == nil && serverInfo.PID > 0 {
		seen[serverInfo.PID] = true
		pids = append(pids, serverInfo.PID)
	}

	// Source 2: pgrep on unix
	if runtime.GOOS != "windows" {
		for _, pattern := range []string{"cockpit-tools", "Cockpit Tools"} {
			out, err := exec.Command("pgrep", "-f", pattern).Output()
			if err == nil {
				for _, line := range strings.Split(string(out), "\n") {
					line = strings.TrimSpace(line)
					if p, err := strconv.Atoi(line); err == nil && p > 0 && !seen[p] {
						seen[p] = true
						pids = append(pids, p)
					}
				}
			}
		}
	}

	return pids
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
// It checks cache -> configuredPort -> OS-level process listening ports -> candidate port pool,
// with protocol-level verification to ensure zero false positives.
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

	// 2. Check configured port directly
	if configuredPort > 0 && VerifyReportPort(configuredPort, token, 400*time.Millisecond) {
		updateReportCache(configuredPort)
		return configuredPort, nil
	}

	// 3. Process-level discovery: inspect ports listening by cockpit-tools PID
	pids := FindCockpitPIDs()
	for _, pid := range pids {
		ports := GetListeningPortsForPID(pid)
		for _, port := range ports {
			if VerifyReportPort(port, token, 400*time.Millisecond) {
				log.Printf("[Cockpit] 🎯 Auto-detected active Report port via process (PID %d): %d (configured was %d)",
					pid, port, configuredPort)
				updateReportCache(port)
				return port, nil
			}
		}
	}

	// 4. Scan candidate ports
	candidates := candidateReportPorts(configuredPort)
	for _, port := range candidates {
		if VerifyReportPort(port, token, 250*time.Millisecond) {
			log.Printf("[Cockpit] 🎯 Auto-detected active Report port via candidate scan: %d (configured was %d)",
				port, configuredPort)
			updateReportCache(port)
			return port, nil
		}
	}

	return 0, fmt.Errorf("cockpit report service not found on configured port %d or any candidate ports", configuredPort)
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

	// 3. Process-level discovery
	pids := FindCockpitPIDs()
	for _, pid := range pids {
		ports := GetListeningPortsForPID(pid)
		for _, port := range ports {
			if VerifyWsPort(port, 400*time.Millisecond) {
				log.Printf("[Cockpit] 🎯 Auto-detected active WebSocket port via process (PID %d): %d (server.json was %d)",
					pid, port, serverJsonPort)
				updateWsCache(port)
				return port, authToken, nil
			}
		}
	}

	// 4. Candidate port pool
	var configuredWsPort int
	if cfg, err := getCockpitConfig(); err == nil && cfg != nil {
		// config.json may have ws_port
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

	candidates := candidateWsPorts(configuredWsPort, serverJsonPort)
	for _, port := range candidates {
		if VerifyWsPort(port, 250*time.Millisecond) {
			log.Printf("[Cockpit] 🎯 Auto-detected active WebSocket port via candidate scan: %d", port)
			updateWsCache(port)
			return port, authToken, nil
		}
	}

	return 0, authToken, fmt.Errorf("cockpit websocket service not responding on any probed ports")
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
