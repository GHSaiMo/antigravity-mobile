package proxy

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestAutomationsLocalReading(t *testing.T) {
	// Create temporary HOME
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Create ~/.gemini/config/sidecars/test-task/sidecar.json
	sidecarsDir := filepath.Join(tmpHome, ".gemini", "config", "sidecars", "test-task")
	if err := os.MkdirAll(sidecarsDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	sidecarJSON := `{
		"builtin": "schedule",
		"args": ["0 12 * * *", "agentapi", "new-conversation", "Test Prompt"],
		"display_name": "Test Scheduled Task"
	}`
	if err := os.WriteFile(filepath.Join(sidecarsDir, "sidecar.json"), []byte(sidecarJSON), 0644); err != nil {
		t.Fatalf("WriteFile sidecar.json failed: %v", err)
	}

	// Create config.json with enabled = true
	configJSON := `{
		"sidecars": {
			"test-task": {
				"enabled": true
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(tmpHome, ".gemini", "config", "config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatalf("WriteFile config.json failed: %v", err)
	}

	// Create events directory with a test event
	eventsDir := filepath.Join(tmpHome, ".gemini", "antigravity", "sidecar_data", "test-task", "events")
	if err := os.MkdirAll(eventsDir, 0755); err != nil {
		t.Fatalf("MkdirAll events failed: %v", err)
	}

	eventJSON := `{
		"timestampMs": "1794495650000",
		"error": "",
		"payload": {
			"newConversation": {
				"conversationId": "cid-12345",
				"prompt": "Test Prompt Content"
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(eventsDir, "20261112_230050.json"), []byte(eventJSON), 0644); err != nil {
		t.Fatalf("WriteFile event failed: %v", err)
	}

	// Create logs with Next execution scheduled line
	logsDir := filepath.Join(tmpHome, ".gemini", "antigravity", "sidecar_data", "test-task", "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		t.Fatalf("MkdirAll logs failed: %v", err)
	}
	logContent := `2026-11-12 23:11:33 [schedule] Next execution scheduled in 12h (at 2026-11-13 12:00:00 UTC), will run by 2026-11-13 12:01:00 UTC`
	if err := os.WriteFile(filepath.Join(logsDir, "sidecar.log"), []byte(logContent), 0644); err != nil {
		t.Fatalf("WriteFile log failed: %v", err)
	}

	// Test readLocalSidecars
	sidecars := readLocalSidecars()
	if len(sidecars) != 1 {
		t.Fatalf("expected 1 sidecar, got %d", len(sidecars))
	}
	sc, ok := sidecars["test-task"]
	if !ok {
		t.Fatalf("expected test-task sidecar")
	}
	if sc.Name != "Test Scheduled Task" {
		t.Errorf("expected Name 'Test Scheduled Task', got %q", sc.Name)
	}
	if sc.CronExpression != "0 12 * * *" {
		t.Errorf("expected Cron '0 12 * * *', got %q", sc.CronExpression)
	}
	if sc.Prompt != "Test Prompt" {
		t.Errorf("expected Prompt 'Test Prompt', got %q", sc.Prompt)
	}
	if !sc.Enabled {
		t.Errorf("expected Enabled true")
	}

	// Test readLocalSidecarEvents
	events := readLocalSidecarEvents("test-task", sc.Name, 10)
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.ConversationID != "cid-12345" {
		t.Errorf("expected ConversationID 'cid-12345', got %q", ev.ConversationID)
	}
	if ev.Status != "success" {
		t.Errorf("expected Status 'success', got %q", ev.Status)
	}

	// Test extractNextRunFromLog
	nextRun := extractNextRunFromLog("test-task")
	if nextRun == "" {
		t.Errorf("expected nextRun to not be empty")
	}

	// Test updateLocalSidecarEnabled
	if err := updateLocalSidecarEnabled("test-task", false); err != nil {
		t.Fatalf("updateLocalSidecarEnabled failed: %v", err)
	}
	sidecarsAfter := readLocalSidecars()
	if sidecarsAfter["test-task"].Enabled {
		t.Errorf("expected Enabled false after update")
	}
}

func TestHandleManageAutomationActionMapping(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	var mu sync.Mutex
	var lastActionReceived string
	var lastSidecarIDReceived string

	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		lastSidecarIDReceived, _ = body["sidecarId"].(string)
		lastActionReceived, _ = body["action"].(string)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer mockUpstream.Close()

	tcpAddr := mockUpstream.Listener.Addr().(*net.TCPAddr)

	p := &Proxy{
		mediumClient: mockUpstream.Client(),
		activePort:   tcpAddr.Port,
		activeToken:  "mock-token",
	}

	testCases := []struct {
		inputAction    string
		expectedAction string
	}{
		{"start", "SIDECAR_ACTION_ENABLE"},
		{"enable", "SIDECAR_ACTION_ENABLE"},
		{"stop", "SIDECAR_ACTION_DISABLE"},
		{"disable", "SIDECAR_ACTION_DISABLE"},
		{"restart", "SIDECAR_ACTION_RESTART"},
		{"run_now", "SIDECAR_ACTION_RUN_NOW"},
		{"trigger", "SIDECAR_ACTION_RUN_NOW"},
	}

	for _, tc := range testCases {
		reqBody, _ := json.Marshal(map[string]string{
			"sidecarId": "task-abc",
			"action":    tc.inputAction,
		})
		req := httptest.NewRequest(http.MethodPost, "/gateway/automations/manage", bytes.NewReader(reqBody))
		w := httptest.NewRecorder()

		p.HandleManageAutomation(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("unexpected code for action %s: %d, body: %s", tc.inputAction, w.Code, w.Body.String())
		}

		mu.Lock()
		gotAction := lastActionReceived
		gotSidecarID := lastSidecarIDReceived
		mu.Unlock()

		if gotAction != tc.expectedAction {
			t.Errorf("for input action %q: expected upstream %q, got %q", tc.inputAction, tc.expectedAction, gotAction)
		}
		if gotSidecarID != "task-abc" {
			t.Errorf("expected sidecarId 'task-abc', got %q", gotSidecarID)
		}
	}
}
