package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleListBackgroundTasks_AndLogTail(t *testing.T) {
	tmpDir := t.TempDir()
	tasksDir := filepath.Join(tmpDir, "brain", "conv-1", ".system_generated", "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(tasksDir, "task-42.log")
	if err := os.WriteFile(logFile, []byte("line 1: starting server\nline 2: listening on :8080\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	logURI := localPathToFileURI(logFile)

	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/exa.language_server_pb.LanguageServerService/ListBackgroundTasks":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"tasks": []map[string]interface{}{
					{
						"conversationId": "conv-1",
						"task": map[string]interface{}{
							"taskId":      "conv-1/task-42",
							"toolName":    "run_command",
							"toolSummary": "Dev server startup",
							"description": "npm run dev",
							"startTime":   "2026-11-10T05:35:25Z",
							"stepIndex":   42,
							"logUri":      logURI,
							"isDaemon":    true,
						},
					},
				},
			})
		case "/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"trajectorySummaries": map[string]interface{}{
					"conv-1": map[string]interface{}{
						"summary": "重构后台任务中心",
						"workspaces": []map[string]interface{}{
							{"workspaceFolderAbsoluteUri": "file:///Users/test/Projects/multigravity"},
						},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	// 1. Test GET /gateway/tasks/background
	req := httptest.NewRequest(http.MethodGet, "/gateway/tasks/background", nil)
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var listResp struct {
		Tasks []GlobalBackgroundTaskItem `json:"tasks"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if len(listResp.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(listResp.Tasks))
	}
	got := listResp.Tasks[0]
	if got.ConversationID != "conv-1" || got.ConversationTitle != "重构后台任务中心" || got.WorkspaceName != "multigravity" {
		t.Errorf("unexpected metadata enrichment: %+v", got)
	}
	if got.TaskID != "conv-1/task-42" || got.StepIndex != 42 || !got.IsDaemon || got.CommandLine != "npm run dev" {
		t.Errorf("unexpected task fields: %+v", got)
	}

	// 2. Test POST /gateway/tasks/background/log
	logReqBody, _ := json.Marshal(map[string]interface{}{
		"conversationId": "conv-1",
		"taskId":         "conv-1/task-42",
		"logUri":         logURI,
	})
	logReq := httptest.NewRequest(http.MethodPost, "/gateway/tasks/background/log", bytes.NewReader(logReqBody))
	logRec := httptest.NewRecorder()
	p.ServeHTTP(logRec, logReq)

	if logRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for log tail, got %d: %s", logRec.Code, logRec.Body.String())
	}
	var logResp struct {
		Content   string `json:"content"`
		Truncated bool   `json:"truncated"`
		SizeBytes int64  `json:"sizeBytes"`
	}
	if err := json.Unmarshal(logRec.Body.Bytes(), &logResp); err != nil {
		t.Fatalf("unmarshal log response: %v", err)
	}
	if !strings.Contains(logResp.Content, "listening on :8080") {
		t.Errorf("expected log content to contain 'listening on :8080', got %q", logResp.Content)
	}
}
