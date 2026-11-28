package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleLaunchpadShortcutsCRUD(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	p := &Proxy{}

	// 1. GET before saving returns uncustomized empty list.
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/gateway/launchpad-shortcuts", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET initial status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var initial LaunchpadShortcutsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &initial); err != nil {
		t.Fatalf("unmarshal initial: %v", err)
	}
	if initial.Customized || len(initial.Items) != 0 {
		t.Fatalf("initial = %+v, want customized=false items=[]", initial)
	}

	// 2. PUT saves normalized, deduplicated, non-magenta items.
	payload := map[string]any{
		"items": []map[string]any{
			{"kind": "slash", "ref": "/Plan", "tint": "purple"},
			{"kind": "skill", "ref": "///mgy-growth-ops", "title": "MGY Growth"},
			{"kind": "workflow", "ref": "code_review", "title": "Code Review"},
			{"kind": "prompt", "ref": "chat_fact_check"},
		},
	}
	body, _ := json.Marshal(payload)
	putRec := httptest.NewRecorder()
	p.ServeHTTP(putRec, httptest.NewRequest(http.MethodPut, "/gateway/launchpad-shortcuts", bytes.NewReader(body)))
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200; body=%s", putRec.Code, putRec.Body.String())
	}
	var saved LaunchpadShortcutsResponse
	if err := json.Unmarshal(putRec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("unmarshal saved: %v", err)
	}
	if !saved.Customized || len(saved.Items) != 4 {
		t.Fatalf("saved = %+v, want customized=true len=4", saved)
	}
	if saved.Items[0].Ref != "plan" || saved.Items[0].Tint != "indigo" {
		t.Fatalf("item[0] = %+v, want ref=plan tint=indigo", saved.Items[0])
	}
	if saved.Items[1].Ref != "mgy-growth-ops" {
		t.Fatalf("item[1] = %+v, want ref=mgy-growth-ops", saved.Items[1])
	}

	// 3. GET reads persisted items back.
	getRec := httptest.NewRecorder()
	p.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/gateway/launchpad-shortcuts", nil))
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET persisted status = %d", getRec.Code)
	}
	var loaded LaunchpadShortcutsResponse
	if err := json.Unmarshal(getRec.Body.Bytes(), &loaded); err != nil {
		t.Fatalf("unmarshal loaded: %v", err)
	}
	if !loaded.Customized || len(loaded.Items) != 4 || loaded.UpdatedAt == "" {
		t.Fatalf("loaded = %+v, want customized=true len=4 non-empty updatedAt", loaded)
	}

	// 4. DELETE resets back to uncustomized.
	delRec := httptest.NewRecorder()
	p.ServeHTTP(delRec, httptest.NewRequest(http.MethodDelete, "/gateway/launchpad-shortcuts", nil))
	if delRec.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d", delRec.Code)
	}
	var reset LaunchpadShortcutsResponse
	if err := json.Unmarshal(delRec.Body.Bytes(), &reset); err != nil {
		t.Fatalf("unmarshal reset: %v", err)
	}
	if reset.Customized || len(reset.Items) != 0 {
		t.Fatalf("reset = %+v, want customized=false items=[]", reset)
	}
}

func TestHandleLaunchpadShortcutsValidation(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	p := &Proxy{}

	cases := []struct {
		name string
		body string
	}{
		{"missing items", `{}`},
		{"too many items", `{"items":[{"kind":"slash","ref":"a"},{"kind":"slash","ref":"b"},{"kind":"slash","ref":"c"},{"kind":"slash","ref":"d"},{"kind":"slash","ref":"e"}]}`},
		{"invalid kind", `{"items":[{"kind":"unknown","ref":"a"}]}`},
		{"empty ref", `{"items":[{"kind":"slash","ref":"///"}]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			p.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/gateway/launchpad-shortcuts", bytes.NewBufferString(tc.body)))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}
