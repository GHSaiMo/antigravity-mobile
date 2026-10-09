package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleCascadeChanges_ForwardDiffAndRevertTarget(t *testing.T) {
	var gotStepIndex float64 = 999
	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/GetRevertPreview") {
			http.NotFound(w, r)
			return
		}
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotStepIndex, _ = body["stepIndex"].(float64)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"codeEditPreviews": []map[string]interface{}{{
				"fileUri":    "file:///w/new.md",
				"actionType": "CODE_REVERT_ACTION_TYPE_DELETE",
				"diff": map[string]interface{}{"lines": []map[string]interface{}{
					{"text": "# Hi", "type": "UNIFIED_DIFF_LINE_TYPE_DELETE"},
				}},
			}},
		})
	})

	w := postJSON(t, p.HandleCascadeChanges, "/gateway/cascade/changes", `{"cascadeId":"abc-123"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if gotStepIndex != -1 {
		t.Errorf("whole-conversation request must revert to step -1, got %v", gotStepIndex)
	}
	var res CascadeChangesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.HasChanges || len(res.Files) != 1 {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
	f := res.Files[0]
	if f.ActionType != "CREATE" || f.FileName != "new.md" || f.Additions != 1 || f.DiffLines[0].Type != "INSERT" {
		t.Fatalf("unexpected file: %+v", f)
	}
	if res.Additions != 1 || res.Deletions != 0 {
		t.Errorf("totals = +%d -%d", res.Additions, res.Deletions)
	}

	// 从第 12 步起的改动：回滚目标是 11
	postJSON(t, p.HandleCascadeChanges, "/gateway/cascade/changes", `{"cascadeId":"abc-123","fromStepIndex":12}`)
	if gotStepIndex != 11 {
		t.Errorf("fromStepIndex 12 must revert to 11, got %v", gotStepIndex)
	}
}

func TestHandleCascadeChanges_Validation(t *testing.T) {
	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	for _, body := range []string{`{}`, `{"cascadeId":"../etc"}`, `not json`} {
		if w := postJSON(t, p.HandleCascadeChanges, "/x", body); w.Code != http.StatusBadRequest {
			t.Errorf("body %q => %d, want 400", body, w.Code)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	p.HandleCascadeChanges(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET => %d, want 405", w.Code)
	}
	noUpstream := &Proxy{}
	if w := postJSON(t, noUpstream.HandleCascadeChanges, "/x", `{"cascadeId":"abc"}`); w.Code != http.StatusServiceUnavailable {
		t.Errorf("no upstream => %d, want 503", w.Code)
	}
}
