package proxy

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
	"github.com/gorilla/websocket"
)

func TestCascadeStreamWebSocket(t *testing.T) {
	insp := inspector.NewInspector(5 * time.Second)
	info := insp.Scan()
	if info == nil {
		t.Skip("Antigravity instance not available, skipping test")
	}

	p := NewProxy(insp)
	server := httptest.NewServer(http.HandlerFunc(p.ServeHTTP))
	defer server.Close()

	// 1. First get a valid cascade ID by listing trajectories
	rpcReq := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", strings.NewReader("{}"))
	rpcReq.Header.Set("Content-Type", "application/json")
	rpcRec := httptest.NewRecorder()
	p.ServeHTTP(rpcRec, rpcReq)

	if rpcRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from GetAllCascadeTrajectories, got %d", rpcRec.Code)
	}

	var allResp struct {
		TrajectorySummaries map[string]interface{} `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(rpcRec.Body.Bytes(), &allResp); err != nil {
		t.Fatalf("failed to decode summaries: %v", err)
	}

	if len(allResp.TrajectorySummaries) == 0 {
		t.Skip("No active cascade trajectories found to test stream")
	}

	var targetCascadeID string
	for id := range allResp.TrajectorySummaries {
		targetCascadeID = id
		break
	}

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/gateway/cascade/stream?cascadeId=" + targetCascadeID
	t.Logf("Connecting to %s", wsURL)

	c, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WS dial failed: %v (resp: %+v)", err, resp)
	}
	defer c.Close()

	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	msgType, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read initial stream message: %v", err)
	}

	if msgType != websocket.TextMessage {
		t.Errorf("expected text message, got %d", msgType)
	}

	var payload StreamUpdatePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Failed to decode stream payload: %v (data: %s)", err, string(data))
	}

	if payload.Type != "init" {
		t.Errorf("expected payload.Type init, got %s", payload.Type)
	}
	if payload.CascadeID != targetCascadeID {
		t.Errorf("expected cascadeId %s, got %s", targetCascadeID, payload.CascadeID)
	}
	t.Logf("Stream init received: steps=%d, tools=%d, status=%s", payload.TotalSteps, payload.TotalTools, payload.Status)
}

func TestSanitizeWebSocketHeaders(t *testing.T) {
	t.Run("duplicate comma-separated keys from proxy", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/gateway/cascade/stream", nil)
		req.Header.Set("Connection", "keep-alive, Upgrade")
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==, dGhlIHNhbXBsZSBub25jZQ==")

		sanitizeWebSocketHeaders(req)

		gotKey := req.Header.Get("Sec-WebSocket-Key")
		if gotKey != "dGhlIHNhbXBsZSBub25jZQ==" {
			t.Errorf("expected single cleaned key, got: %q", gotKey)
		}
	})

	t.Run("missing Sec-WebSocket-Key synthesized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/gateway/cascade/stream", nil)
		sanitizeWebSocketHeaders(req)

		gotKey := req.Header.Get("Sec-WebSocket-Key")
		if gotKey == "" {
			t.Fatal("expected synthesized Sec-WebSocket-Key, got empty")
		}
		if req.Header.Get("Connection") != "Upgrade" {
			t.Errorf("expected Connection: Upgrade, got: %s", req.Header.Get("Connection"))
		}
		if req.Header.Get("Upgrade") != "websocket" {
			t.Errorf("expected Upgrade: websocket, got: %s", req.Header.Get("Upgrade"))
		}
		if req.Header.Get("Sec-WebSocket-Version") != "13" {
			t.Errorf("expected Sec-WebSocket-Version: 13, got: %s", req.Header.Get("Sec-WebSocket-Version"))
		}
	})

	t.Run("quoted and whitespace key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/gateway/cascade/stream", nil)
		req.Header.Set("Sec-WebSocket-Key", " \"dGhlIHNhbXBsZSBub25jZQ==\" ")

		sanitizeWebSocketHeaders(req)

		gotKey := req.Header.Get("Sec-WebSocket-Key")
		if gotKey != "dGhlIHNhbXBsZSBub25jZQ==" {
			t.Errorf("expected unquoted trimmed key, got: %q", gotKey)
		}
	})
}

func TestWebSocketUpgradeWithMangledHeaders(t *testing.T) {
	// Verify that a request with comma-separated Sec-WebSocket-Key (like Cloudflare Tunnel produces)
	// successfully upgrades to 101 Switching Protocols without error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sanitizeWebSocketHeaders(r)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrader.Upgrade failed: %v", err)
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte("ok"))
	}))
	defer server.Close()

	// Dial using raw TCP to simulate proxy forwarding duplicated comma-separated key
	rawConn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("TCP dial failed: %v", err)
	}
	defer rawConn.Close()

	rawReq := "GET / HTTP/1.1\r\n" +
		"Host: " + server.Listener.Addr().String() + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==, dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"

	if _, err := rawConn.Write([]byte(rawReq)); err != nil {
		t.Fatalf("Write raw request failed: %v", err)
	}

	reader := bufio.NewReader(rawConn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("Read status line failed: %v", err)
	}

	if !strings.Contains(statusLine, "101") {
		t.Errorf("expected status 101 Switching Protocols, got: %s", statusLine)
	}
}
