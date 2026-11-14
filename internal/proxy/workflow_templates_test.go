package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleWorkflowTemplates_Lifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("MULTIGRAVITY_DATA_DIR", tmpDir)

	p := &Proxy{}

	// 1. Initial GET returns the 4 built-in defaults without creating a file.
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/gateway/workflow-templates", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got WorkflowTemplatesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if len(got.Templates) != 4 {
		t.Fatalf("default templates len = %d, want 4", len(got.Templates))
	}

	// 2. PUT reorders and adds a custom template (with default icon/color fallback).
	updated := []WorkflowTemplate{
		{
			ID:             "my_daily",
			Title:          "每日代码巡检",
			Subtitle:       "检查今日 git diff",
			TemplatePrompt: "请帮我检查今天所有的 git diff 并列出潜在风险。",
		},
		got.Templates[2], // research moved ahead
		got.Templates[0], // code_review
	}
	body, _ := json.Marshal(map[string]interface{}{"templates": updated})
	putReq := httptest.NewRequest(http.MethodPut, "/gateway/workflow-templates", bytes.NewReader(body))
	putRec := httptest.NewRecorder()
	p.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200: %s", putRec.Code, putRec.Body.String())
	}
	var saved WorkflowTemplatesResponse
	if err := json.Unmarshal(putRec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode PUT: %v", err)
	}
	if len(saved.Templates) != 3 {
		t.Fatalf("saved templates len = %d, want 3", len(saved.Templates))
	}
	if saved.Templates[0].ID != "my_daily" || saved.Templates[0].Icon != "workflow" || saved.Templates[0].Color != "indigo" {
		t.Fatalf("unexpected normalized first template: %+v", saved.Templates[0])
	}
	if saved.Templates[1].ID != "research" || saved.Templates[2].ID != "code_review" {
		t.Fatalf("order not preserved: %+v", saved.Templates)
	}

	// 3. Subsequent GET returns the persisted custom order.
	rec2 := httptest.NewRecorder()
	p.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/gateway/workflow-templates", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("GET after PUT status = %d, want 200", rec2.Code)
	}
	var got2 WorkflowTemplatesResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &got2); err != nil {
		t.Fatalf("decode GET2: %v", err)
	}
	if len(got2.Templates) != 3 || got2.Templates[0].ID != "my_daily" {
		t.Fatalf("persisted GET2 mismatch: %+v", got2.Templates)
	}

	// 4. PUT empty array explicitly persists 0 templates (distinct from uninitialized).
	emptyPut := httptest.NewRequest(http.MethodPut, "/gateway/workflow-templates", bytes.NewBufferString(`{"templates":[]}`))
	emptyRec := httptest.NewRecorder()
	p.ServeHTTP(emptyRec, emptyPut)
	if emptyRec.Code != http.StatusOK {
		t.Fatalf("empty PUT status = %d, want 200: %s", emptyRec.Code, emptyRec.Body.String())
	}
	recEmptyGet := httptest.NewRecorder()
	p.ServeHTTP(recEmptyGet, httptest.NewRequest(http.MethodGet, "/gateway/workflow-templates", nil))
	var gotEmpty WorkflowTemplatesResponse
	_ = json.Unmarshal(recEmptyGet.Body.Bytes(), &gotEmpty)
	if len(gotEmpty.Templates) != 0 {
		t.Fatalf("expected 0 persisted templates, got %d", len(gotEmpty.Templates))
	}

	// 5. DELETE resets to defaults.
	delRec := httptest.NewRecorder()
	p.ServeHTTP(delRec, httptest.NewRequest(http.MethodDelete, "/gateway/workflow-templates", nil))
	if delRec.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, want 200", delRec.Code)
	}
	var resetResp WorkflowTemplatesResponse
	_ = json.Unmarshal(delRec.Body.Bytes(), &resetResp)
	if len(resetResp.Templates) != 4 {
		t.Fatalf("expected 4 default templates after DELETE, got %d", len(resetResp.Templates))
	}
}

func TestHandleWorkflowTemplates_Validation(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("MULTIGRAVITY_DATA_DIR", tmpDir)

	p := &Proxy{}

	cases := []struct {
		name   string
		method string
		body   string
		want   int
	}{
		{"missing templates field", http.MethodPut, `{}`, http.StatusBadRequest},
		{"empty title", http.MethodPut, `{"templates":[{"id":"a","title":"  ","templatePrompt":"hi"}]}`, http.StatusBadRequest},
		{"empty prompt", http.MethodPut, `{"templates":[{"id":"a","title":"T","templatePrompt":"  "}]}`, http.StatusBadRequest},
		{"unsupported method", http.MethodPatch, `{}`, http.StatusMethodNotAllowed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/gateway/workflow-templates", bytes.NewBufferString(tc.body))
			rec := httptest.NewRecorder()
			p.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}
