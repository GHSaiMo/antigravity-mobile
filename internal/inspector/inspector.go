package inspector

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// InstanceInfo contains discovered runtime information about Antigravity language_server.
type InstanceInfo struct {
	PID          int       `json:"pid"`
	Port         int       `json:"port"`
	CSRFToken    string    `json:"csrf_token"`
	DiscoveredAt time.Time `json:"discovered_at"`
	IsHealthy    bool      `json:"is_healthy"`
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
}

// NewInspector creates a new instance inspector.
func NewInspector(pollInterval time.Duration) *Inspector {
	if pollInterval <= 0 {
		pollInterval = 5 * time.Second
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
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
func (i *Inspector) Scan() *InstanceInfo {
	pid, csrfToken, err := i.findProcess()
	if err != nil {
		i.markUnhealthy()
		return nil
	}

	ports, err := i.findListeningPorts(pid)
	if err != nil || len(ports) == 0 {
		i.markUnhealthy()
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
		return nil
	}

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

func (i *Inspector) markUnhealthy() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.current != nil && i.current.IsHealthy {
		i.current.IsHealthy = false
		log.Println("[Inspector] Antigravity instance became unhealthy or stopped")
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
		log.Printf("[Inspector] Discovered active Antigravity instance: PID=%d Port=%d CSRF=%s",
			newInfo.PID, newInfo.Port, newInfo.CSRFToken)
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

// findProcess uses ps to find the language_server process and extract PID & CSRF token.
func (i *Inspector) findProcess() (int, string, error) {
	cmd := exec.Command("ps", "-eo", "pid,command")
	out, err := cmd.Output()
	if err != nil {
		return 0, "", fmt.Errorf("failed to run ps: %w", err)
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "language_server") && strings.Contains(line, "--csrf_token") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			pid, err := strconv.Atoi(fields[0])
			if err != nil {
				continue
			}

			matches := csrfRegex.FindStringSubmatch(line)
			if len(matches) > 1 {
				return pid, matches[1], nil
			}
		}
	}

	return 0, "", fmt.Errorf("language_server process not found")
}

// findListeningPorts uses lsof to query TCP LISTEN ports for a given PID.
func (i *Inspector) findListeningPorts(pid int) ([]int, error) {
	cmd := exec.Command("lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-a", "-p", strconv.Itoa(pid))
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run lsof: %w", err)
	}

	var ports []int
	seen := make(map[int]bool)

	matches := lsofRegex.FindAllStringSubmatch(string(out), -1)
	for _, m := range matches {
		if len(m) > 1 {
			port, err := strconv.Atoi(m[1])
			if err == nil && !seen[port] {
				seen[port] = true
				ports = append(ports, port)
			}
		}
	}

	return ports, nil
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
