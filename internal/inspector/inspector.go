package inspector

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
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
		log.Println("[Inspector] ⚠️  Antigravity 实例已断开")
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
			log.Printf("[Inspector] ✅ 已连接 Antigravity 实例 (PID %d, 端口 %d)", newInfo.PID, newInfo.Port)
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
	req.Header.Set("x-codeium-csrf-token", csrfToken)

	resp, err := i.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
