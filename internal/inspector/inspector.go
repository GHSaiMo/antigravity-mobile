package inspector

import (
	"log/slog"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"antigravity-mobile/internal/localtls"
)

// InstanceInfo contains discovered runtime information about Antigravity language_server.
type InstanceInfo struct {
	PID          int       `json:"pid"`
	Port         int       `json:"port"`
	CSRFToken    string    `json:"csrf_token"`
	DiscoveredAt time.Time `json:"discovered_at"`
	IsHealthy    bool      `json:"is_healthy"`
}

// UpstreamDiscoverer defines the contract for discovering and monitoring Antigravity language_server instances.
type UpstreamDiscoverer interface {
	Current() *InstanceInfo
	Scan() *InstanceInfo
	Start()
	Stop()
	OnUpdate(fn func(InstanceInfo))
}

// Inspector monitors and discovers Antigravity language_server instances.
type Inspector struct {
	mu           sync.RWMutex
	current      *InstanceInfo
	pollInterval time.Duration
	httpClient   *http.Client
	stopCh       chan struct{}
	stopOnce     sync.Once
	listeners    []func(InstanceInfo)

	// PERF-3: exponential backoff for the expensive ps+lsof full-scan path.
	failedScanMu   sync.Mutex
	failedScanAt   time.Time  // time of last failed full-scan
	failedBackoff  time.Duration // current backoff duration
}

// NewInspector creates a new instance inspector.
func NewInspector(pollInterval time.Duration) *Inspector {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}

	tr := &http.Transport{
		TLSClientConfig: localtls.ClientConfig(),
		DialTLSContext:  localtls.DialTLSContext,
	}

	return &Inspector{
		pollInterval: pollInterval,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   2 * time.Second,
		},
		stopCh: make(chan struct{}),
	}
}

// OnUpdate registers a listener callback called when instance info changes.
func (i *Inspector) OnUpdate(fn func(InstanceInfo)) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.listeners = append(i.listeners, fn)
}

// Current returns the current instance info (copy).
func (i *Inspector) Current() *InstanceInfo {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.current == nil {
		return nil
	}
	cp := *i.current
	return &cp
}

// Start begins periodic background discovery.
func (i *Inspector) Start() {
	// Immediate first discovery
	i.Scan()

	go func() {
		ticker := time.NewTicker(i.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-i.stopCh:
				return
			case <-ticker.C:
				i.Scan()
			}
		}
	}()
}

// Stop halts the inspector polling loop. Safe to call multiple times.
func (i *Inspector) Stop() {
	i.stopOnce.Do(func() {
		close(i.stopCh)
	})
}

var (
	csrfRegex = regexp.MustCompile(`--csrf_token\s+([0-9a-fA-F-]+)`)
	lsofRegex = regexp.MustCompile(`:(\d+)\s+\(LISTEN\)`)
)

// Scan performs one inspection pass to detect and verify the language_server instance.
// If an active instance is already known and healthy, a lightweight HTTP verification is
// attempted first, avoiding unnecessary subprocess forks of ps and lsof.
func (i *Inspector) Scan() *InstanceInfo {
	i.mu.RLock()
	curr := i.current
	i.mu.RUnlock()

	if curr != nil && curr.IsHealthy && curr.Port > 0 {
		if i.verifyPort(curr.Port, curr.CSRFToken) {
			return curr
		}
	}

	// Fast path: check daemon discovery files (~/.gemini/antigravity/daemon/ls_*.json)
	if dInfo := i.findFromDaemon(); dInfo != nil {
		i.failedScanMu.Lock()
		i.failedBackoff = 0
		i.failedScanMu.Unlock()

		i.update(dInfo)
		return dInfo
	}

	// PERF-3: exponential backoff — skip expensive ps+lsof if we recently failed.
	i.failedScanMu.Lock()
	if i.failedBackoff > 0 && time.Since(i.failedScanAt) < i.failedBackoff {
		i.failedScanMu.Unlock()
		return nil
	}
	i.failedScanMu.Unlock()

	pid, csrfToken, err := i.findProcess(context.Background())
	if err != nil {
		i.markUnhealthy()
		i.recordFailedScan()
		return nil
	}

	ports, err := i.findListeningPorts(context.Background(), pid)
	if err != nil || len(ports) == 0 {
		i.markUnhealthy()
		i.recordFailedScan()
		return nil
	}

	// Verify candidate ports with health check
	var activePort int
	for _, port := range ports {
		if i.verifyPort(port, csrfToken) {
			activePort = port
			break
		}
	}

	if activePort == 0 {
		i.markUnhealthy()
		i.recordFailedScan()
		return nil
	}

	// Success — reset backoff.
	i.failedScanMu.Lock()
	i.failedBackoff = 0
	i.failedScanMu.Unlock()

	info := &InstanceInfo{
		PID:          pid,
		Port:         activePort,
		CSRFToken:    csrfToken,
		DiscoveredAt: time.Now(),
		IsHealthy:    true,
	}

	i.update(info)
	return info
}

// recordFailedScan advances the exponential backoff after a full-scan failure.
// Backoff sequence: 5s → 10s → 20s → 40s (capped).
func (i *Inspector) recordFailedScan() {
	const maxBackoff = 40 * time.Second
	i.failedScanMu.Lock()
	defer i.failedScanMu.Unlock()
	i.failedScanAt = time.Now()
	if i.failedBackoff == 0 {
		i.failedBackoff = 5 * time.Second
	} else {
		i.failedBackoff *= 2
		if i.failedBackoff > maxBackoff {
			i.failedBackoff = maxBackoff
		}
	}
}

func (i *Inspector) markUnhealthy() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.current != nil && i.current.IsHealthy {
		i.current.IsHealthy = false
		slog.Warn("[Inspector] ⚠️  Antigravity 实例已断开")
	}
}

func (i *Inspector) update(newInfo *InstanceInfo) {
	i.mu.Lock()
	var notify bool
	var listeners []func(InstanceInfo)

	if i.current == nil ||
		i.current.PID != newInfo.PID ||
		i.current.Port != newInfo.Port ||
		i.current.CSRFToken != newInfo.CSRFToken ||
		!i.current.IsHealthy {
		if i.current != nil {
			slog.Info(fmt.Sprintf("[Inspector] ✅ 已连接 Antigravity 实例 (PID %d, 端口 %d)", newInfo.PID, newInfo.Port))
		}
		notify = true
		listeners = append([]func(InstanceInfo){}, i.listeners...)
	}
	i.current = newInfo
	i.mu.Unlock()

	if notify {
		for _, fn := range listeners {
			fn(*newInfo)
		}
	}
}

// verifyPort checks if the port responds positively to GetStatus RPC with the CSRF token.
func (i *Inspector) verifyPort(port int, csrfToken string) bool {
	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetStatus", port)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewBufferString("{}"))
	if err != nil {
		return false
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if csrfToken != "" {
		req.Header.Set("x-codeium-csrf-token", csrfToken)
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

type daemonDiscoveryInfo struct {
	PID       int    `json:"pid"`
	HTTPSPort int    `json:"httpsPort"`
	HTTPPort  int    `json:"httpPort"`
	LSPPort   int    `json:"lspPort"`
	LSVersion string `json:"lsVersion"`
	CSRFToken string `json:"csrfToken"`
}

var daemonDirOverride string

// findFromDaemon inspects ~/.gemini/antigravity/daemon/ls_*.json written by Antigravity daemon/headless modes.
func (i *Inspector) findFromDaemon() *InstanceInfo {
	var daemonDir string
	if daemonDirOverride != "" {
		daemonDir = daemonDirOverride
	} else {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return nil
		}
		daemonDir = filepath.Join(home, ".gemini", "antigravity", "daemon")
	}
	files, err := filepath.Glob(filepath.Join(daemonDir, "ls_*.json"))
	if err != nil || len(files) == 0 {
		return nil
	}

	type fileEntry struct {
		path    string
		modTime time.Time
	}
	var entries []fileEntry
	for _, f := range files {
		st, err := os.Stat(f)
		if err == nil {
			entries = append(entries, fileEntry{path: f, modTime: st.ModTime()})
		}
	}
	sort.Slice(entries, func(a, b int) bool {
		return entries[a].modTime.After(entries[b].modTime)
	})

	for _, entry := range entries {
		data, err := os.ReadFile(entry.path)
		if err != nil {
			continue
		}
		var d daemonDiscoveryInfo
		if err := json.Unmarshal(data, &d); err != nil {
			continue
		}
		if d.HTTPSPort > 0 && i.verifyPort(d.HTTPSPort, d.CSRFToken) {
			return &InstanceInfo{
				PID:          d.PID,
				Port:         d.HTTPSPort,
				CSRFToken:    d.CSRFToken,
				DiscoveredAt: time.Now(),
				IsHealthy:    true,
			}
		}
	}
	return nil
}

