package inspector

import (
	"log/slog"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/localtls"
)

// InstanceInfo contains discovered runtime information about Antigravity language_server.
type InstanceInfo struct {
	PID          int       `json:"pid"`
	Port         int       `json:"port"`
	CSRFToken    string    `json:"-"`
	DiscoveredAt time.Time `json:"discovered_at"`
	IsHealthy    bool      `json:"is_healthy"`
	// Version is the Antigravity version when the discovery source reports it (daemon files); otherwise
	// callers can look it up from the process with LookupProcessDetails.
	Version string `json:"version,omitempty"`
	// AppDataDir is the discovered Antigravity data directory (e.g. ~/.gemini/antigravity or
	// ~/.gemini/antigravity-cli) when known from daemon discovery files. Never serialized to JSON.
	AppDataDir string `json:"-"`
}

// Diagnostic failure reasons reported by Inspector when no healthy language_server is connected.
const (
	ReasonNone                  = ""
	ReasonAntigravityNotRunning = "antigravity_not_running"
	ReasonLSNotReady            = "ls_not_ready"
	ReasonLSAccessDenied        = "ls_access_denied"
)

// UpstreamDiscoverer defines the contract for discovering and monitoring Antigravity language_server instances.
type UpstreamDiscoverer interface {
	Current() *InstanceInfo
	Scan() *InstanceInfo
	Start()
	Stop()
	OnUpdate(fn func(InstanceInfo))
}

// ForceScanner is optionally implemented by UpstreamDiscoverer to bypass exponential backoff
// on user-initiated rescan or on-demand request recovery.
type ForceScanner interface {
	ForceScan() *InstanceInfo
}

// FailureReasonReporter is optionally implemented by UpstreamDiscoverer to expose the latest
// sanitized discovery failure reason (never contains paths, PIDs, or tokens).
type FailureReasonReporter interface {
	LastFailureReason() string
}

type processCandidate struct {
	PID       int
	CSRFToken string
	Priority  int
}

type processScanResult struct {
	Candidates   []processCandidate
	AppRunning   bool
	AccessDenied bool
	Err          error
}

// Inspector monitors and discovers Antigravity language_server instances.
type Inspector struct {
	scanMu       sync.Mutex
	mu           sync.RWMutex
	current      *InstanceInfo
	pollInterval time.Duration
	httpClient   *http.Client
	stopCh       chan struct{}
	stopOnce     sync.Once
	listeners    []func(InstanceInfo)

	// PERF-3: exponential backoff for the expensive ps+lsof full-scan path.
	failedScanMu      sync.Mutex
	failedScanAt      time.Time     // time of last failed full-scan
	failedBackoff     time.Duration // current backoff duration
	lastForceScanAt   time.Time     // cooldown timestamp for ForceScan
	forceScanCooldown time.Duration // minimum gap between forced full-scans (default 1.5s)

	initialScanDone   bool
	lastFailureReason string
	lastLoggedReason  string

	// Optional test seams; nil in production (falls back to OS implementations).
	findCandidatesFn func(context.Context) processScanResult
	findPortsFn      func(context.Context, int) ([]int, error)
	verifyPortFn     func(int, string) bool
}

// NewInspector creates a new instance inspector.
func NewInspector(pollInterval time.Duration) *Inspector {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}

	tr := &http.Transport{
		TLSClientConfig:     localtls.ClientConfig(),
		DialContext:         localtls.DialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     30 * time.Second,
	}

	return &Inspector{
		pollInterval: pollInterval,
		httpClient: &http.Client{
			Transport: tr,
			Timeout:   2 * time.Second,
		},
		stopCh:            make(chan struct{}),
		forceScanCooldown: 1500 * time.Millisecond,
	}
}

// LastFailureReason returns the latest sanitized failure reason when disconnected ("" when healthy).
func (i *Inspector) LastFailureReason() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.current != nil && i.current.IsHealthy && i.current.Port > 0 {
		return ReasonNone
	}
	return i.lastFailureReason
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
	i.mu.Lock()
	i.initialScanDone = true
	i.mu.Unlock()

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
	csrfRegex = regexp.MustCompile(`--csrf_token[=\s]+([0-9a-fA-F-]+)`)
	lsofRegex = regexp.MustCompile(`:(\d+)\s+\(LISTEN\)`)
)

// Scan performs one inspection pass to detect and verify the language_server instance.
// Daemon discovery files (~/.gemini/antigravity/daemon/{hub,ls}_*.json) are checked first so
// that an authoritative desktop/daemon instance under $HOME always takes precedence and can
// immediately preempt any fallback process discovered during a desktop restart window.
func (i *Inspector) Scan() *InstanceInfo {
	return i.scan(false)
}

// ForceScan performs an on-demand inspection pass that bypasses the exponential failed-scan
// backoff (subject to a short cooldown to prevent subprocess storms under request bursts).
func (i *Inspector) ForceScan() *InstanceInfo {
	return i.scan(true)
}

func (i *Inspector) verify(port int, csrfToken string) bool {
	if i.verifyPortFn != nil {
		return i.verifyPortFn(port, csrfToken)
	}
	return i.verifyPort(port, csrfToken)
}

func (i *Inspector) scan(force bool) *InstanceInfo {
	i.scanMu.Lock()
	defer i.scanMu.Unlock()

	// 1. Authoritative fast path: check daemon discovery files (~/.gemini/antigravity/daemon/{hub,ls}_*.json)
	if dInfo := i.findFromDaemon(); dInfo != nil {
		i.failedScanMu.Lock()
		i.failedBackoff = 0
		i.failedScanMu.Unlock()

		i.update(dInfo)
		return dInfo
	}

	i.mu.RLock()
	curr := i.current
	i.mu.RUnlock()

	if curr != nil && curr.IsHealthy && curr.Port > 0 {
		if i.verify(curr.Port, curr.CSRFToken) {
			return curr
		}
	}

	// PERF-3: exponential backoff for periodic scans; ForceScan bypasses the long 40s backoff
	// while still enforcing a short cooldown to prevent concurrent request storms.
	i.failedScanMu.Lock()
	if force {
		cooldown := i.forceScanCooldown
		if cooldown <= 0 {
			cooldown = 1500 * time.Millisecond
		}
		if !i.lastForceScanAt.IsZero() && time.Since(i.lastForceScanAt) < cooldown {
			i.failedScanMu.Unlock()
			return nil
		}
		i.lastForceScanAt = time.Now()
	} else if i.failedBackoff > 0 && time.Since(i.failedScanAt) < i.failedBackoff {
		i.failedScanMu.Unlock()
		return nil
	}
	i.failedScanMu.Unlock()

	var procRes processScanResult
	if i.findCandidatesFn != nil {
		procRes = i.findCandidatesFn(context.Background())
	} else {
		procRes = i.findProcessCandidates(context.Background())
	}

	findPorts := i.findListeningPorts
	if i.findPortsFn != nil {
		findPorts = i.findPortsFn
	}

	for _, cand := range procRes.Candidates {
		ports, err := findPorts(context.Background(), cand.PID)
		if err != nil || len(ports) == 0 {
			continue
		}
		for _, port := range ports {
			if i.verify(port, cand.CSRFToken) {
				i.failedScanMu.Lock()
				i.failedBackoff = 0
				i.failedScanMu.Unlock()

				info := &InstanceInfo{
					PID:          cand.PID,
					Port:         port,
					CSRFToken:    cand.CSRFToken,
					DiscoveredAt: time.Now(),
					IsHealthy:    true,
				}
				i.update(info)
				return info
			}
		}
	}

	reason := ReasonAntigravityNotRunning
	switch {
	case procRes.AccessDenied:
		reason = ReasonLSAccessDenied
	case len(procRes.Candidates) > 0 || procRes.AppRunning:
		reason = ReasonLSNotReady
	}
	i.markUnhealthyWithReason(reason)
	i.recordFailedScan()
	return nil
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
	i.markUnhealthyWithReason(ReasonAntigravityNotRunning)
}

func (i *Inspector) markUnhealthyWithReason(reason string) {
	i.mu.Lock()
	var notify bool
	var listeners []func(InstanceInfo)
	var disconnected InstanceInfo

	if i.current != nil && i.current.IsHealthy {
		i.current.IsHealthy = false
		disconnected = *i.current
		notify = true
		listeners = append([]func(InstanceInfo){}, i.listeners...)
		slog.Warn("[Inspector] ⚠️  Antigravity 实例已断开")
	}

	if reason != "" {
		i.lastFailureReason = reason
		if reason != i.lastLoggedReason {
			i.lastLoggedReason = reason
			switch reason {
			case ReasonLSAccessDenied:
				slog.Warn("[Inspector] ⚠️  检测到 language_server 进程但无权读取命令行（Windows 管理员权限隔离）；请以管理员身份运行 mgy 或普通权限启动 Antigravity")
			case ReasonLSNotReady:
				slog.Info("[Inspector] ⏳ 检测到 Antigravity 已启动，等待 language_server 本地服务就绪...")
			}
		}
	}
	i.mu.Unlock()

	if notify {
		for _, fn := range listeners {
			fn(disconnected)
		}
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
		if i.current != nil || i.initialScanDone {
			slog.Info(fmt.Sprintf("[Inspector] ✅ 已连接 Antigravity 实例 (PID %d, 端口 %d)", newInfo.PID, newInfo.Port))
		}
		notify = true
		listeners = append([]func(InstanceInfo){}, i.listeners...)
	}
	i.current = newInfo
	i.lastFailureReason = ReasonNone
	i.lastLoggedReason = ReasonNone
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
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

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

func isPathWithinDir(parent, child string) bool {
	if parent == "" || child == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// DaemonDiscoveryDirs returns the ordered, symlink-deduplicated list of daemon discovery directories
// to scan for {hub,ls}_*.json files. Precedence:
//  1. MGY_ANTIGRAVITY_DATA_DIR / MULTIGRAVITY_ANTIGRAVITY_DATA_DIR (explicit user override)
//  2. ANTIGRAVITY_APP_DATA_DIR/daemon (when within the current $HOME)
//  3. ~/.gemini/antigravity/daemon (Desktop / primary)
//  4. ~/.gemini/antigravity-cli/daemon (Antigravity CLI / agy fallback)
func DaemonDiscoveryDirs() []string {
	if daemonDirOverride != "" {
		return []string{daemonDirOverride}
	}

	home, _ := os.UserHomeDir()
	var candidates []string
	if customDir := strings.TrimSpace(os.Getenv("MGY_ANTIGRAVITY_DATA_DIR")); customDir != "" {
		candidates = append(candidates, filepath.Join(filepath.Clean(customDir), "daemon"))
	} else if customDir := strings.TrimSpace(os.Getenv("MULTIGRAVITY_ANTIGRAVITY_DATA_DIR")); customDir != "" {
		candidates = append(candidates, filepath.Join(filepath.Clean(customDir), "daemon"))
	}
	if envDir := strings.TrimSpace(os.Getenv("ANTIGRAVITY_APP_DATA_DIR")); envDir != "" && (home == "" || isPathWithinDir(home, envDir)) {
		candidates = append(candidates, filepath.Join(filepath.Clean(envDir), "daemon"))
	}
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".gemini", "antigravity", "daemon"),
			filepath.Join(home, ".gemini", "antigravity-cli", "daemon"),
		)
	}

	seen := make(map[string]struct{}, len(candidates))
	dirs := make([]string, 0, len(candidates))
	for _, dir := range candidates {
		key := filepath.Clean(dir)
		if resolved, err := filepath.EvalSymlinks(dir); err == nil && resolved != "" {
			key = filepath.Clean(resolved)
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		dirs = append(dirs, dir)
	}
	return dirs
}

// findFromDaemon inspects {hub,ls}_*.json written under ~/.gemini/antigravity/daemon and
// ~/.gemini/antigravity-cli/daemon by Antigravity Desktop, daemon, or CLI (agy) modes.
func (i *Inspector) findFromDaemon() *InstanceInfo {
	daemonDirs := DaemonDiscoveryDirs()
	if len(daemonDirs) == 0 {
		return nil
	}

	type fileEntry struct {
		path       string
		dir        string
		sourceRank int
		isHub      bool
		modTime    time.Time
	}

	seenFiles := make(map[string]struct{})
	var entries []fileEntry
	for rank, daemonDir := range daemonDirs {
		for _, pattern := range []string{"hub_*.json", "ls_*.json"} {
			matches, err := filepath.Glob(filepath.Join(daemonDir, pattern))
			if err != nil {
				continue
			}
			for _, f := range matches {
				key := filepath.Clean(f)
				if resolved, err := filepath.EvalSymlinks(f); err == nil && resolved != "" {
					key = filepath.Clean(resolved)
				}
				if _, dup := seenFiles[key]; dup {
					continue
				}
				seenFiles[key] = struct{}{}

				st, err := os.Stat(f)
				if err != nil || st.IsDir() {
					continue
				}
				entries = append(entries, fileEntry{
					path:       f,
					dir:        daemonDir,
					sourceRank: rank,
					isHub:      strings.HasPrefix(filepath.Base(f), "hub_"),
					modTime:    st.ModTime(),
				})
			}
		}
	}
	if len(entries) == 0 {
		return nil
	}

	sort.Slice(entries, func(a, b int) bool {
		if entries[a].sourceRank != entries[b].sourceRank {
			return entries[a].sourceRank < entries[b].sourceRank
		}
		if entries[a].isHub != entries[b].isHub {
			return entries[a].isHub
		}
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
		if d.HTTPSPort > 0 && i.verify(d.HTTPSPort, d.CSRFToken) {
			appDataDir := ""
			if daemonDirOverride == "" {
				appDataDir = filepath.Dir(entry.dir)
			}
			return &InstanceInfo{
				PID:          d.PID,
				Port:         d.HTTPSPort,
				CSRFToken:    d.CSRFToken,
				DiscoveredAt: time.Now(),
				IsHealthy:    true,
				Version:      d.LSVersion,
				AppDataDir:   appDataDir,
			}
		}
	}
	return nil
}

