package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
)

func TestProjectsEndpoint(t *testing.T) {
	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)

	req := httptest.NewRequest(http.MethodGet, "/gateway/projects", nil)
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /gateway/projects, got %d: %s", rec.Code, rec.Body.String())
	}

	var projects []ProjectItem
	if err := json.NewDecoder(rec.Body).Decode(&projects); err != nil {
		t.Fatalf("failed to decode projects json: %v", err)
	}

	t.Logf("Successfully fetched %d projects", len(projects))
	for idx, prj := range projects {
		t.Logf("[%d] %s -> %s (sessions: %d, ws: %v)", idx, prj.Name, prj.Path, prj.SessionCount, prj.IsWorkspace)
		if idx >= 5 {
			break
		}
	}
}

func TestURIHelpers(t *testing.T) {
	raw := "file:///Users/hal9000/Projects/test%20dir"
	path := uriToPath(raw)
	if path != "/Users/hal9000/Projects/test dir" {
		t.Errorf("expected decoded path, got: %s", path)
	}

	norm := normalizeURI("/Users/hal9000/Projects/foo/")
	if norm != "file:///Users/hal9000/Projects/foo" {
		t.Errorf("expected normalized URI, got: %s", norm)
	}
}
