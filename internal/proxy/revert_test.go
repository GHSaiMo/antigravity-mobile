package proxy

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleCascadeRevertPreview_Success(t *testing.T) {
	// Mock upstream language_server responding to GetRevertPreview
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/exa.language_server_pb.LanguageServerService/GetRevertPreview" {
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]interface{}{
				"codeEditPreviews": []map[string]interface{}{
					{
						"fileUri":    "file:///workspace/src/app.js",
						"actionType": "CODE_REVERT_ACTION_TYPE_MODIFY",
						"diff": map[string]interface{}{
							"lines": []map[string]interface{}{
								{"text": "const a = 1;", "type": "UNIFIED_DIFF_LINE_TYPE_INSERT"},
								{"text": "const a = 2;", "type": "UNIFIED_DIFF_LINE_TYPE_DELETE"},
								{"text": "const b = 3;", "type": "UNIFIED_DIFF_LINE_TYPE_UNCHANGED"},
							},
						},
					},
					{
						"fileUri":    "file:///workspace/docs/readme.md",
						"actionType": "CODE_REVERT_ACTION_TYPE_DELETE",
						"diff": map[string]interface{}{
							"lines": []map[string]interface{}{
								{"text": "# Old Title", "type": "UNIFIED_DIFF_LINE_TYPE_DELETE"},
							},
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer mockUpstream.Close()

	p := &Proxy{
		shortClient:  mockUpstream.Client(),
		mediumClient: mockUpstream.Client(),
		longClient:   mockUpstream.Client(),
	}
	p.SetTestUpstream(mockUpstream.Listener.Addr().(*net.TCPAddr).Port, "test-token")

	// Call HandleCascadeRevertPreview
	payload := `{"cascadeId":"test-cascade","stepIndex":4}`
	req := httptest.NewRequest(http.MethodPost, "/gateway/cascade/revert/preview", bytes.NewBufferString(payload))
	w := httptest.NewRecorder()

	p.HandleCascadeRevertPreview(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var res RevertPreviewResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if res.CascadeID != "test-cascade" {
		t.Errorf("expected cascadeId 'test-cascade', got '%s'", res.CascadeID)
	}
	if res.StepIndex != 4 {
		t.Errorf("expected stepIndex 4, got %d", res.StepIndex)
	}
	if res.TargetStepIndex != 3 {
		t.Errorf("expected targetStepIndex 3 (4-1), got %d", res.TargetStepIndex)
	}
	if !res.HasCodeChanges {
		t.Errorf("expected hasCodeChanges true, got false")
	}
	if len(res.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(res.Files))
	}

	file1 := res.Files[0]
	if file1.FileName != "app.js" {
		t.Errorf("expected fileName 'app.js', got '%s'", file1.FileName)
	}
	if file1.ActionType != "MODIFY" {
		t.Errorf("expected actionType 'MODIFY', got '%s'", file1.ActionType)
	}
	if file1.Additions != 1 {
		t.Errorf("expected 1 addition, got %d", file1.Additions)
	}
	if file1.Deletions != 1 {
		t.Errorf("expected 1 deletion, got %d", file1.Deletions)
	}
	if len(file1.DiffLines) != 3 {
		t.Errorf("expected 3 diff lines, got %d", len(file1.DiffLines))
	}

	file2 := res.Files[1]
	if file2.FileName != "readme.md" {
		t.Errorf("expected fileName 'readme.md', got '%s'", file2.FileName)
	}
	if file2.ActionType != "DELETE" {
		t.Errorf("expected actionType 'DELETE', got '%s'", file2.ActionType)
	}
	if file2.Deletions != 1 {
		t.Errorf("expected 1 deletion, got %d", file2.Deletions)
	}
}

func TestHandleCascadeRevertExecute_Success(t *testing.T) {
	revertCalled := false
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/exa.language_server_pb.LanguageServerService/RevertToCascadeStep" {
			revertCalled = true
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["cascadeId"] != "test-cascade" || int(body["stepIndex"].(float64)) != -1 {
				t.Errorf("unexpected body: %v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer mockUpstream.Close()

	p := &Proxy{
		shortClient:  mockUpstream.Client(),
		mediumClient: mockUpstream.Client(),
		longClient:   mockUpstream.Client(),
	}
	p.SetTestUpstream(mockUpstream.Listener.Addr().(*net.TCPAddr).Port, "test-token")

	// Call HandleCascadeRevertExecute for step 0 (target should be -1)
	payload := `{"cascadeId":"test-cascade","stepIndex":0}`
	req := httptest.NewRequest(http.MethodPost, "/gateway/cascade/revert/execute", bytes.NewBufferString(payload))
	w := httptest.NewRecorder()

	p.HandleCascadeRevertExecute(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	if !revertCalled {
		t.Errorf("expected upstream RevertToCascadeStep to be called")
	}

	var res map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if res["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", res["status"])
	}
	if int(res["targetStepIndex"].(float64)) != -1 {
		t.Errorf("expected targetStepIndex -1, got %v", res["targetStepIndex"])
	}
}
