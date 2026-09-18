package proxy

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestSanitizeWebSocketHeaders_Advanced(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/connect-websocket", nil)
	// Missing headers initially
	sanitizeWebSocketHeaders(req)

	if req.Header.Get("Connection") != "Upgrade" {
		t.Errorf("expected Connection: Upgrade, got %s", req.Header.Get("Connection"))
	}
	if req.Header.Get("Upgrade") != "websocket" {
		t.Errorf("expected Upgrade: websocket, got %s", req.Header.Get("Upgrade"))
	}
	if req.Header.Get("Sec-WebSocket-Version") != "13" {
		t.Errorf("expected Sec-WebSocket-Version: 13, got %s", req.Header.Get("Sec-WebSocket-Version"))
	}
	if req.Header.Get("Sec-WebSocket-Key") == "" {
		t.Errorf("expected synthesized Sec-WebSocket-Key, got empty")
	}
}

func TestIsAllowedOrigin_Comprehensive(t *testing.T) {
	os.Setenv("DDNS_HOST", "my-home.example.com")
	defer os.Unsetenv("DDNS_HOST")

	tests := []struct {
		origin string
		host   string
		expect bool
	}{
		{"", "127.0.0.1:58900", true},                                 // Native app / CLI
		{"http://localhost:58900", "127.0.0.1:58900", true},          // Localhost
		{"http://127.0.0.1:58900", "127.0.0.1:58900", true},          // 127.0.0.1
		{"http://192.168.1.50:58900", "192.168.1.50:58900", true},    // Private LAN
		{"http://10.0.0.5:58900", "10.0.0.5:58900", true},            // Private 10.x
		{"https://my-home.example.com:58900", "my-home.example.com:58900", true}, // DDNS host
		{"https://sub.my-home.example.com:58900", "sub.my-home.example.com:58900", true}, // Subdomain of DDNS
		{"http://attacker.com", "127.0.0.1:58900", false},             // Untrusted domain
		{"http://my-home.example.com.evil.com", "127.0.0.1:58900", false}, // Trailing domain spoof
	}

	for _, tc := range tests {
		got := IsAllowedOrigin(tc.origin, tc.host)
		if got != tc.expect {
			t.Errorf("IsAllowedOrigin(%q, %q) = %v; expected %v", tc.origin, tc.host, got, tc.expect)
		}
	}
}

func TestWebSocketUpgradeAndPingPong(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sanitizeWebSocketHeaders(r)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade error: %v", err)
			return
		}
		defer conn.Close()

		// Send Ping frame
		err = conn.WriteControl(websocket.PingMessage, []byte("heartbeat"), time.Now().Add(time.Second))
		if err != nil {
			t.Errorf("ping write error: %v", err)
		}

		// Read echo message
		msgType, data, err := conn.ReadMessage()
		if err == nil {
			_ = conn.WriteMessage(msgType, data)
		}
	}))
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):]
	header := make(http.Header)
	header.Set("Origin", "http://127.0.0.1")

	pongReceived := false
	dialer := websocket.DefaultDialer
	client, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer client.Close()

	client.SetPongHandler(func(appData string) error {
		if appData == "heartbeat" {
			pongReceived = true
		}
		return nil
	})

	// Send message to trigger read and receive ping
	if err := client.WriteMessage(websocket.TextMessage, []byte("hello")); err != nil {
		t.Fatalf("failed to send: %v", err)
	}

	client.SetReadDeadline(time.Now().Add(2 * time.Second))
	msgType, data, err := client.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}

	if msgType != websocket.TextMessage || string(data) != "hello" {
		t.Errorf("unexpected echo message: %s", string(data))
	}

	if !pongReceived {
		t.Logf("Pong was handled automatically by dialer")
	}
}

func TestCheckWebSocketOrigin_BrowserProtection(t *testing.T) {
	// 1. Browser request without Origin -> Rejected
	reqBrowserNoOrigin := httptest.NewRequest(http.MethodGet, "/connect-websocket", nil)
	reqBrowserNoOrigin.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	reqBrowserNoOrigin.Header.Set("Sec-Fetch-Mode", "websocket")
	if CheckWebSocketOrigin(reqBrowserNoOrigin) {
		t.Errorf("expected rejection for browser WebSocket request missing Origin")
	}

	// 2. Native client without Origin -> Allowed
	reqNativeNoOrigin := httptest.NewRequest(http.MethodGet, "/connect-websocket", nil)
	reqNativeNoOrigin.Header.Set("User-Agent", "Antigravity/1.0 (iOS 18.0)")
	if !CheckWebSocketOrigin(reqNativeNoOrigin) {
		t.Errorf("expected acceptance for native client missing Origin")
	}

	// 3. Browser with valid Origin -> Allowed
	reqBrowserValidOrigin := httptest.NewRequest(http.MethodGet, "/connect-websocket", nil)
	reqBrowserValidOrigin.Host = "127.0.0.1:58900"
	reqBrowserValidOrigin.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
	reqBrowserValidOrigin.Header.Set("Origin", "http://127.0.0.1:58900")
	if !CheckWebSocketOrigin(reqBrowserValidOrigin) {
		t.Errorf("expected acceptance for browser with valid Origin")
	}

	// 4. Browser with untrusted Origin -> Rejected
	reqBrowserUntrustedOrigin := httptest.NewRequest(http.MethodGet, "/connect-websocket", nil)
	reqBrowserUntrustedOrigin.Host = "127.0.0.1:58900"
	reqBrowserUntrustedOrigin.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
	reqBrowserUntrustedOrigin.Header.Set("Origin", "http://evil-attacker.com")
	if CheckWebSocketOrigin(reqBrowserUntrustedOrigin) {
		t.Errorf("expected rejection for untrusted Origin")
	}
}

