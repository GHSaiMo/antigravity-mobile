package cockpit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gorilla/websocket"
)

func TestSwitchAccountMockWS(t *testing.T) {
	upgrader := websocket.Upgrader{}

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// 1. Send ready
		_ = conn.WriteJSON(map[string]any{
			"type":    "event.ready",
			"payload": map[string]any{"version": "1.3.47"},
		})

		// 2. Read request
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var req wsMessage
		_ = json.Unmarshal(msg, &req)
		if req.Type != "request.switch_account" {
			t.Errorf("unexpected req type: %s", req.Type)
			return
		}

		var p switchAccountPayload
		_ = json.Unmarshal(req.Payload, &p)

		if p.AccountID == "valid-id" {
			_ = conn.WriteJSON(map[string]any{
				"type": "event.account_switched",
				"payload": map[string]any{
					"account_id": "valid-id",
					"email":      "test@example.com",
				},
			})
			_ = conn.WriteJSON(map[string]any{
				"type": "response.success",
				"payload": map[string]any{
					"request_id": p.RequestID,
					"message":    "切换账号成功",
				},
			})
		} else {
			_ = conn.WriteJSON(map[string]any{
				"type": "event.switch_error",
				"payload": map[string]any{
					"message": "ACCOUNT_NOT_FOUND",
				},
			})
			_ = conn.WriteJSON(map[string]any{
				"type": "response.error",
				"payload": map[string]any{
					"request_id": p.RequestID,
					"error":      "ACCOUNT_NOT_FOUND",
				},
			})
		}
	}))
	defer s.Close()

	u, _ := url.Parse(s.URL)
	port, _ := strconv.Atoi(u.Port())

	// Create temp home dir for server.json
	tmpDir := t.TempDir()
	cockpitDir := filepath.Join(tmpDir, ".antigravity_cockpit")
	_ = os.MkdirAll(cockpitDir, 0755)

	serverInfo := CockpitServerInfo{
		WsPort:    port,
		Version:   "1.3.47",
		PID:       1234,
		AuthToken: "test",
	}
	bytes, _ := json.Marshal(serverInfo)
	_ = os.WriteFile(filepath.Join(cockpitDir, "server.json"), bytes, 0644)

	// Override HOME
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	// Test success
	err := SwitchAccount("valid-id")
	if err != nil {
		t.Fatalf("expected nil error on valid switch, got: %v", err)
	}

	// Test failure
	err = SwitchAccount("invalid-id")
	if err == nil || err.Error() != "ACCOUNT_NOT_FOUND" {
		t.Fatalf("expected ACCOUNT_NOT_FOUND, got: %v", err)
	}
}
