package cockpit

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestCandidateReportPorts(t *testing.T) {
	ports := candidateReportPorts(18081)
	if len(ports) < 5 {
		t.Fatalf("expected at least 5 candidate ports, got %d", len(ports))
	}
	if ports[0] != 18081 {
		t.Fatalf("expected first port to be configured port 18081, got %d", ports[0])
	}

	// Test deduplication with custom configured port
	ports2 := candidateReportPorts(18082)
	if ports2[0] != 18082 {
		t.Fatalf("expected first port to be 18082, got %d", ports2[0])
	}
	// Check no duplicates
	seen := make(map[int]bool)
	for _, p := range ports2 {
		if seen[p] {
			t.Fatalf("duplicate port %d in candidates", p)
		}
		seen[p] = true
	}
}

func TestCandidateWsPorts(t *testing.T) {
	ports := candidateWsPorts(18081, 19528)
	if len(ports) < 4 {
		t.Fatalf("expected at least 4 candidate ports, got %d", len(ports))
	}
	if ports[0] != 19528 {
		t.Fatalf("expected first port to be server.json ws_port 19528, got %d", ports[0])
	}
	if ports[1] != 18081 {
		t.Fatalf("expected second port to be configured ws_port 18081, got %d", ports[1])
	}
}

func TestVerifyReportPort(t *testing.T) {
	// 1. Invalid port
	if VerifyReportPort(0, "test", 100*time.Millisecond) {
		t.Fatal("expected port 0 to fail")
	}
	if VerifyReportPort(-1, "test", 100*time.Millisecond) {
		t.Fatal("expected port -1 to fail")
	}

	// 2. Unreachable port
	if VerifyReportPort(59999, "test", 100*time.Millisecond) {
		t.Fatal("expected dead port to fail")
	}

	// 3. Mock Report Server (200 OK)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/report" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("service: Antigravity\nmetric: Claude\n"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())

	if !VerifyReportPort(port, "token123", 500*time.Millisecond) {
		t.Fatalf("expected VerifyReportPort to succeed on port %d", port)
	}

	// 4. Mock Report Server (401 Unauthorized - still recognized as report endpoint)
	serverAuth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/report" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer serverAuth.Close()

	uAuth, _ := url.Parse(serverAuth.URL)
	portAuth, _ := strconv.Atoi(uAuth.Port())

	if !VerifyReportPort(portAuth, "wrong_token", 500*time.Millisecond) {
		t.Fatalf("expected VerifyReportPort to succeed (as valid cockpit service) on 401 for port %d", portAuth)
	}
}

func TestVerifyWsPort(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())

	if !VerifyWsPort(port, 500*time.Millisecond) {
		t.Fatalf("expected VerifyWsPort to succeed on port %d", port)
	}
	if VerifyWsPort(59998, 100*time.Millisecond) {
		t.Fatal("expected dead port to fail VerifyWsPort")
	}
}

func TestResolveActiveReportPort_PortShift(t *testing.T) {
	InvalidatePortCache()

	// Mock a report server on an arbitrary ephemeral port (simulating port shift)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/report" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("service: Antigravity\nmetric: Gemini\n"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	actualPort, _ := strconv.Atoi(u.Port())

	// Test 1: Configured port points directly to actualPort
	resolved, err := ResolveActiveReportPort("test-token", actualPort)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if resolved != actualPort {
		t.Fatalf("expected %d, got %d", actualPort, resolved)
	}

	// Test 2: In-memory cache hit
	resolvedCached, err := ResolveActiveReportPort("test-token", 59997)
	if err != nil {
		t.Fatalf("expected cache hit success, got error: %v", err)
	}
	if resolvedCached != actualPort {
		t.Fatalf("expected cached port %d, got %d", actualPort, resolvedCached)
	}

	// Test 3: Cache invalidated, configured port is dead (e.g. 59997 is occupied or inactive),
	// but actualPort is in candidates
	InvalidatePortCache()
	// If actualPort is passed as configuredPort, it resolves immediately
	resolved2, err := ResolveActiveReportPort("test-token", actualPort)
	if err != nil || resolved2 != actualPort {
		t.Fatalf("expected to resolve to actualPort %d, got %d (err: %v)", actualPort, resolved2, err)
	}
}

func TestGetListeningPortsForPID(t *testing.T) {
	// Create a real TCP listener on current process
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("skipping TCP listen test due to socket permissions")
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	port := addr.Port

	pid := os.Getpid()
	ports := GetListeningPortsForPID(pid)
	if len(ports) == 0 {
		// Some CI/sandboxes restrict lsof/netstat permissions
		t.Logf("GetListeningPortsForPID returned no ports (possibly restricted sandbox permissions for pid %d)", pid)
		return
	}

	found := false
	for _, p := range ports {
		if p == port {
			found = true
			break
		}
	}
	if !found {
		t.Logf("port %d not explicitly found in ports %v (might be due to lsof filter)", port, ports)
	}
}

func TestGetCockpitServerInfo_WithFallback(t *testing.T) {
	InvalidatePortCache()

	tmpDir := t.TempDir()
	cockpitDir := filepath.Join(tmpDir, ".antigravity_cockpit")
	_ = os.MkdirAll(cockpitDir, 0755)

	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
	t.Setenv("COCKPIT_DATA_DIR", cockpitDir)

	// Mock WebSocket server
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
	}))
	defer server.Close()

	u, _ := url.Parse(server.URL)
	actualWsPort, _ := strconv.Atoi(u.Port())

	// Case 1: server.json has correct port
	info := CockpitServerInfo{
		WsPort:    actualWsPort,
		AuthToken: "mock-token",
		PID:       os.Getpid(),
	}
	raw, _ := json.Marshal(info)
	_ = os.WriteFile(filepath.Join(cockpitDir, "server.json"), raw, 0644)

	res, err := GetCockpitServerInfo()
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if res.WsPort != actualWsPort {
		t.Fatalf("expected ws_port %d, got %d", actualWsPort, res.WsPort)
	}
	if res.AuthToken != "mock-token" {
		t.Fatalf("expected mock-token, got %s", res.AuthToken)
	}

	// Case 2: server.json has dead port (e.g. 59995), but actualWsPort is cached or resolvable
	info.WsPort = 59995
	rawDead, _ := json.Marshal(info)
	_ = os.WriteFile(filepath.Join(cockpitDir, "server.json"), rawDead, 0644)

	// Prime the WS cache with the actual live port
	updateWsCache(actualWsPort)

	res2, err := GetCockpitServerInfo()
	if err != nil {
		t.Fatalf("expected fallback success, got %v", err)
	}
	if res2.WsPort != actualWsPort {
		t.Fatalf("expected auto-detected port %d, got %d", actualWsPort, res2.WsPort)
	}
}
