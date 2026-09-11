package proxy

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestResolveModelEnum(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"gemini-3.8-flash-high", "MODEL_PLACEHOLDER_M318"},
		{"claude-opus-4-6-thinking", "MODEL_PLACEHOLDER_M26"},
		{"gemini", "MODEL_PLACEHOLDER_M318"},
		{"claude", "MODEL_PLACEHOLDER_M26"},
		{"MODEL_PLACEHOLDER_M16", "MODEL_PLACEHOLDER_M16"},
		{"MODEL_CUSTOM_TEST", "MODEL_CUSTOM_TEST"},
		{"", ""},
		{"unknown-model-xyz", ""},
	}

	for _, tc := range tests {
		got := resolveModelEnum(tc.input)
		if got != tc.expected {
			t.Errorf("resolveModelEnum(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestHandleCreateCascadeRequestedModel(t *testing.T) {
	var lastReceivedPayload map[string]interface{}
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/StartCascade") {
			json.NewDecoder(r.Body).Decode(&lastReceivedPayload)
			w.Write([]byte(`{"cascadeId": "test-cascade-123"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/UpdateConversationAnnotations") {
			w.Write([]byte(`{}`))
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)
	p.activePort = port
	p.activeToken = "test-token"

	testCases := []struct {
		modelInput    string
		expectedEnum  string
		shouldHaveKey bool
	}{
		{"gemini-3.8-flash-high", "MODEL_PLACEHOLDER_M318", true},
		{"claude-opus-4-6-thinking", "MODEL_PLACEHOLDER_M26", true},
		{"MODEL_PLACEHOLDER_M37", "MODEL_PLACEHOLDER_M37", true},
		{"", "", false},
		{"unknown-model", "", false},
	}

	for _, tc := range testCases {
		lastReceivedPayload = nil
		body, _ := json.Marshal(CreateCascadeRequest{
			WorkspaceURI: "file:///test/ws",
			Model:        tc.modelInput,
		})
		req := httptest.NewRequest(http.MethodPost, "/gateway/cascade/new", strings.NewReader(string(body)))
		rec := httptest.NewRecorder()

		p.HandleCreateCascade(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("model %q: expected 200, got %d: %s", tc.modelInput, rec.Code, rec.Body.String())
		}

		if tc.shouldHaveKey {
			reqModel, ok := lastReceivedPayload["requestedModel"]
			if !ok {
				t.Errorf("model %q: expected requestedModel in payload, but not found", tc.modelInput)
			} else if reqModelStr, ok := reqModel.(string); !ok || reqModelStr != tc.expectedEnum {
				t.Errorf("model %q: expected string enum %q, got %v (%T)", tc.modelInput, tc.expectedEnum, reqModel, reqModel)
			}
		} else {
			if _, ok := lastReceivedPayload["requestedModel"]; ok {
				t.Errorf("model %q: expected requestedModel to be omitted, but found: %v", tc.modelInput, lastReceivedPayload["requestedModel"])
			}
		}
	}
}

func TestHandleCreateCascadeIdempotency(t *testing.T) {
	startCascadeCalls := 0
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/StartCascade") {
			startCascadeCalls++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"cascadeId":"cascade-idemp-123"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer mockUpstream.Close()

	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)
	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	p.updateUpstream(inspector.InstanceInfo{
		Port:      port,
		CSRFToken: "test-token",
		IsHealthy: true,
	})

	body, _ := json.Marshal(CreateCascadeRequest{
		WorkspaceURI: "file:///test/ws",
		Prompt:       "Initial prompt",
	})

	clientMsgID := "new-cascade-uuid-456"

	// First request -> should call StartCascade
	req1 := httptest.NewRequest(http.MethodPost, "/gateway/cascade/new", strings.NewReader(string(body)))
	req1.Header.Set("X-Client-Message-Id", clientMsgID)
	rec1 := httptest.NewRecorder()
	p.HandleCreateCascade(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec1.Code)
	}
	var resp1 CreateCascadeResponse
	_ = json.NewDecoder(rec1.Body).Decode(&resp1)
	if resp1.CascadeID != "cascade-idemp-123" {
		t.Fatalf("expected cascadeId 'cascade-idemp-123', got %q", resp1.CascadeID)
	}
	if startCascadeCalls != 1 {
		t.Fatalf("expected startCascadeCalls=1, got %d", startCascadeCalls)
	}

	// Second request with same X-Client-Message-Id -> should return cached response without calling StartCascade
	req2 := httptest.NewRequest(http.MethodPost, "/gateway/cascade/new", strings.NewReader(string(body)))
	req2.Header.Set("X-Client-Message-Id", clientMsgID)
	rec2 := httptest.NewRecorder()
	p.HandleCreateCascade(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 on repeat, got %d", rec2.Code)
	}
	var resp2 CreateCascadeResponse
	_ = json.NewDecoder(rec2.Body).Decode(&resp2)
	if resp2.CascadeID != "cascade-idemp-123" {
		t.Fatalf("expected cached cascadeId 'cascade-idemp-123', got %q", resp2.CascadeID)
	}
	if startCascadeCalls != 1 {
		t.Fatalf("expected startCascadeCalls to remain 1 (deduplicated), got %d", startCascadeCalls)
	}
}
