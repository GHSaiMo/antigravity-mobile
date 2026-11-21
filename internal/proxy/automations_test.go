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

func TestParseAndBuildScheduleArgs(t *testing.T) {
	args := []string{
		"CRON_TZ=Asia/Shanghai 0 * * * *",
		"agentapi",
		"new-conversation",
		"--model=gemini-3.8-flash-high",
		"--extra=1",
		"--",
		"Run hourly audit",
	}
	cron, tz, model, prompt, flags := parseScheduleArgs(args)
	if cron != "CRON_TZ=Asia/Shanghai 0 * * * *" {
		t.Fatalf("unexpected cron: %q", cron)
	}
	if tz != "Asia/Shanghai" {
		t.Fatalf("unexpected tz: %q", tz)
	}
	if model != "gemini-3.8-flash-high" {
		t.Fatalf("unexpected model: %q", model)
	}
	if prompt != "Run hourly audit" {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
	if len(flags) != 2 {
		t.Fatalf("expected 2 flags, got %v", flags)
	}

	newModel := "gemini-4-neon"
	rebuilt := buildScheduleArgs(args, "0 9 * * *", &newModel, "Updated prompt")
	cron2, tz2, model2, prompt2, flags2 := parseScheduleArgs(rebuilt)
	if cron2 != "0 9 * * *" || tz2 != "" || model2 != "gemini-4-neon" || prompt2 != "Updated prompt" {
		t.Fatalf("rebuilt parse mismatch: cron=%q tz=%q model=%q prompt=%q", cron2, tz2, model2, prompt2)
	}
	if len(flags2) != 2 || flags2[0] != "--model=gemini-4-neon" || flags2[1] != "--extra=1" {
		t.Fatalf("expected preserved non-model flag, got %v", flags2)
	}

	emptyModel := ""
	cleared := buildScheduleArgs(rebuilt, "0 9 * * *", &emptyModel, "Updated prompt")
	_, _, model3, _, flags3 := parseScheduleArgs(cleared)
	if model3 != "" || len(flags3) != 1 || flags3[0] != "--extra=1" {
		t.Fatalf("expected model cleared and --extra=1 preserved, got model=%q flags=%v", model3, flags3)
	}
}

func TestHandleGetAutomationsDisabledClearsNextRun(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	sidecarsDir := filepath.Join(tmpHome, ".gemini", "config", "sidecars", "x-outreach")
	if err := os.MkdirAll(sidecarsDir, 0755); err != nil {
		t.Fatal(err)
	}
	scJSON := `{
		"builtin": "schedule",
		"args": ["CRON_TZ=Asia/Shanghai 0 * * * *", "agentapi", "new-conversation", "--model=gemini-3.8-flash-high", "--", "Real prompt text"],
		"display_name": "X Outreach",
		"agent_permissions": {"workspace_uris": ["file:///tmp/repo"], "access_grants": ["command(gh api)"]}
	}`
	if err := os.WriteFile(filepath.Join(sidecarsDir, "sidecar.json"), []byte(scJSON), 0644); err != nil {
		t.Fatal(err)
	}
	cfgJSON := `{"sidecars":{"x-outreach":{"enabled":false,"projectId":"proj-1"}}}`
	if err := os.WriteFile(filepath.Join(tmpHome, ".gemini", "config", "config.json"), []byte(cfgJSON), 0644); err != nil {
		t.Fatal(err)
	}
	logsDir := filepath.Join(tmpHome, ".gemini", "antigravity", "sidecar_data", "x-outreach", "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		t.Fatal(err)
	}
	logLine := "2026-11-20 23:00:08 [schedule] Next execution scheduled in 59m (at 2026-11-21 00:00:00 UTC), will run by 2026-11-21 00:01:00 UTC\n"
	if err := os.WriteFile(filepath.Join(logsDir, "sidecar.log"), []byte(logLine), 0644); err != nil {
		t.Fatal(err)
	}

	p := &Proxy{}
	req := httptest.NewRequest(http.MethodGet, "/gateway/automations", nil)
	w := httptest.NewRecorder()
	p.HandleGetAutomations(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", w.Code, w.Body.String())
	}
	var resp AutomationsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Automations) != 1 {
		t.Fatalf("expected 1 automation, got %d", len(resp.Automations))
	}
	got := resp.Automations[0]
	if got.Prompt != "Real prompt text" {
		t.Errorf("expected real prompt, got %q", got.Prompt)
	}
	if got.Model != "gemini-3.8-flash-high" {
		t.Errorf("expected model, got %q", got.Model)
	}
	if got.TimeZone != "Asia/Shanghai" {
		t.Errorf("expected timezone, got %q", got.TimeZone)
	}
	if got.NextRunAt != "" {
		t.Errorf("expected empty NextRunAt when disabled, got %q", got.NextRunAt)
	}
	if got.WorkspaceURI != "file:///tmp/repo" {
		t.Errorf("expected workspaceUri, got %q", got.WorkspaceURI)
	}
}

func TestHandleUpdateAutomationUpstreamPreservesPermissions(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	var mu sync.Mutex
	var capturedManage map[string]interface{}

	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/exa.language_server_pb.LanguageServerService/GetSidecars":
			_, _ = w.Write([]byte(`{
				"sidecars": [{
					"sidecarId": "x-outreach",
					"status": "SIDECAR_STATUS_STOPPED",
					"config": {
						"builtin": "schedule",
						"restartPolicy": "always",
						"displayName": "Old Name",
						"description": "Old Desc",
						"args": ["CRON_TZ=Asia/Shanghai 0 * * * *", "agentapi", "new-conversation", "--model=gemini-3.8-flash-high", "--", "Old prompt"],
						"agentPermissions": {
							"workspaceUris": ["file:///repo"],
							"accessGrants": ["command(gh api)"]
						}
					}
				}]
			}`))
		case "/exa.language_server_pb.LanguageServerService/ManageSidecar":
			mu.Lock()
			_ = json.NewDecoder(r.Body).Decode(&capturedManage)
			mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockUpstream.Close()

	tcpAddr := mockUpstream.Listener.Addr().(*net.TCPAddr)
	p := &Proxy{
		mediumClient: mockUpstream.Client(),
		activePort:   tcpAddr.Port,
		activeToken:  "tok",
	}

	reqBody := `{"sidecarId":"x-outreach","name":"New Name","cron":"0 9 * * *","model":"gemini-4-neon","prompt":"New prompt"}`
	req := httptest.NewRequest(http.MethodPost, "/gateway/automations/update", bytes.NewBufferString(reqBody))
	w := httptest.NewRecorder()
	p.HandleUpdateAutomation(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update failed: %d %s", w.Code, w.Body.String())
	}

	mu.Lock()
	defer mu.Unlock()
	if capturedManage["action"] != "SIDECAR_ACTION_UPDATE" {
		t.Fatalf("expected SIDECAR_ACTION_UPDATE, got %v", capturedManage["action"])
	}
	cfg, _ := capturedManage["config"].(map[string]interface{})
	if cfg["displayName"] != "New Name" {
		t.Errorf("expected displayName New Name, got %v", cfg["displayName"])
	}
	if cfg["restartPolicy"] != "always" {
		t.Errorf("expected preserved restartPolicy, got %v", cfg["restartPolicy"])
	}
	if cfg["agentPermissions"] == nil {
		t.Errorf("expected preserved agentPermissions")
	}
}

func TestHandleGetAutomationLogs(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	logsDir := filepath.Join(tmpHome, ".gemini", "antigravity", "sidecar_data", "my-task", "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logsDir, "sidecar.log"), []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	p := &Proxy{}
	req := httptest.NewRequest(http.MethodGet, "/gateway/automations/logs?sidecarId=my-task&tail=2", nil)
	w := httptest.NewRecorder()
	p.HandleGetAutomationLogs(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("logs failed: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Logs      string   `json:"logs"`
		Files     []string `json:"files"`
		Truncated bool     `json:"truncated"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Logs != "line2\nline3" || !resp.Truncated {
		t.Fatalf("unexpected logs resp: %+v", resp)
	}

	// Reject path traversal
	reqBad := httptest.NewRequest(http.MethodGet, "/gateway/automations/logs?sidecarId=../etc", nil)
	wBad := httptest.NewRecorder()
	p.HandleGetAutomationLogs(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad sidecarId, got %d", wBad.Code)
	}
}
