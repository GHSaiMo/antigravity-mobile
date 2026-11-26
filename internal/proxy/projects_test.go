package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		t.Logf("[%d] ID=%s Name=%s -> %s (sessions: %d, ws: %v)", idx, prj.ID, prj.Name, prj.Path, prj.SessionCount, prj.IsWorkspace)
		if idx >= 5 {
			break
		}
	}
}

func TestURIHelpers(t *testing.T) {
	raw := "file:///Users/hal9000/Projects/test%20dir"
	path := uriToPath(raw)
	expected := filepath.FromSlash("/Users/hal9000/Projects/test dir")
	if path != expected {
		t.Errorf("expected decoded path, got: %s", path)
	}

	norm := normalizeURI("/Users/hal9000/Projects/foo/")
	if norm != "file:///Users/hal9000/Projects/foo" {
		t.Errorf("expected normalized URI, got: %s", norm)
	}

	// Test Chinese URI decoding
	chineseURI := "file:///d%3A/OneDrive/%E9%A1%B9%E7%9B%AE/Antigravity"
	normChinese := normalizeURI(chineseURI)
	if normChinese != "file:///d:/OneDrive/项目/Antigravity" {
		t.Errorf("expected normalized Chinese URI, got: %s", normChinese)
	}

	// Test 2-slash Windows file URI
	twoSlash := "file://d:/Projects/test-app"
	normTwoSlash := normalizeURI(twoSlash)
	if normTwoSlash != "file:///d:/Projects/test-app" {
		t.Errorf("expected normalized 3-slash URI, got: %s", normTwoSlash)
	}

	// Test remote workspace URI
	remoteURI := "vscode-remote://ssh-remote%2B7b22686f73744e616d65223a224e4153227d/home/jiuzai/projects"
	normRemote := normalizeURI(remoteURI)
	if normRemote != remoteURI {
		t.Errorf("expected remote URI preserved, got: %s", normRemote)
	}
	remotePath := uriToPath(remoteURI)
	if remotePath != "/home/jiuzai/projects" {
		t.Errorf("expected remote path extracted, got: %s", remotePath)
	}
}

func TestResolveModelEnum(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"gemini-3.8-flash-high", "MODEL_PLACEHOLDER_M318"},
		{"gemini-4-carbon-high", "MODEL_PLACEHOLDER_M420"},
		{"gemini-4-carbon", "MODEL_PLACEHOLDER_M420"},
		{"gemini-4-argon-high", "MODEL_PLACEHOLDER_M417"},
		{"gemini-4-argon-medium", "MODEL_PLACEHOLDER_M418"},
		{"gemini-4-argon-low", "MODEL_PLACEHOLDER_M419"},
		{"gemini-4-hydrogen-xhigh", "MODEL_PLACEHOLDER_M528"},
		{"gemini-4-hydrogen-high", "MODEL_PLACEHOLDER_M525"},
		{"gemini-4-hydrogen-medium", "MODEL_PLACEHOLDER_M526"},
		{"gemini-4-hydrogen-low", "MODEL_PLACEHOLDER_M527"},
		{"gemini-pro-orchestrator", "MODEL_PLACEHOLDER_M667"},
		{"gemini-pro-orchestrator-argon", "MODEL_PLACEHOLDER_M681"},
		{"claude-opus-5-5-high", "MODEL_PLACEHOLDER_M402"},
		{"claude-sonnet-5-5-high", "MODEL_PLACEHOLDER_M405"},
		{"claude-opus-4-6-thinking", "MODEL_PLACEHOLDER_M26"},
		{"gemini", "MODEL_PLACEHOLDER_M318"},
		{"claude", "MODEL_PLACEHOLDER_M26"},
		{"MODEL_PLACEHOLDER_M16", "MODEL_PLACEHOLDER_M16"},
		{"MODEL_CUSTOM_TEST", "MODEL_CUSTOM_TEST"},
		{"gemini-2.5-pro", "MODEL_PLACEHOLDER_M318"},
		{"MODEL_GOOGLE_GEMINI_2_5_PRO", "MODEL_PLACEHOLDER_M318"},
		{"MODEL_GOOGLE_GEMINI_2_5_FLASH", "MODEL_PLACEHOLDER_M318"},
		{"claude-3-7-sonnet", "MODEL_PLACEHOLDER_M26"},
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
		{"", "MODEL_PLACEHOLDER_M318", true},
		{"unknown-model", "", false},
	}

	wsDir := t.TempDir()
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", wsDir)
	wsURI := filePathToURI(wsDir)

	for _, tc := range testCases {
		lastReceivedPayload = nil
		body, _ := json.Marshal(CreateCascadeRequest{
			WorkspaceURI: wsURI,
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

	wsDir := t.TempDir()
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", wsDir)

	body, _ := json.Marshal(CreateCascadeRequest{
		WorkspaceURI: filePathToURI(wsDir),
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

func TestCanonicalModelName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"MODEL_PLACEHOLDER_M26", "claude-opus-4-6-thinking"},
		{"claude", "claude-opus-4-6-thinking"},
		{"claude-opus", "claude-opus-4-6-thinking"},
		{"claude-opus-4-6-thinking", "claude-opus-4-6-thinking"},
		{"claude-sonnet-4-6", "claude-sonnet-4-6"},
		{"MODEL_PLACEHOLDER_M318", "gemini-3.8-flash-high"},
		{"gemini", "gemini-3.8-flash-high"},
		{"gemini-3.8-flash-high", "gemini-3.8-flash-high"},
		{"", ""},
	}

	for _, tc := range tests {
		got := canonicalModelName(tc.input)
		if got != tc.expected {
			t.Errorf("canonicalModelName(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestApplyModelToCascadeConfig(t *testing.T) {
	// 1. Apply to existing config with different model
	origConfig := map[string]interface{}{
		"plannerConfig": map[string]interface{}{
			"planModel": "MODEL_PLACEHOLDER_M318",
			"requestedModel": map[string]interface{}{
				"model": "MODEL_PLACEHOLDER_M318",
			},
			"modelName": "gemini-3.8-flash-high",
		},
		"otherField": "keepMe",
	}

	updated := applyModelToCascadeConfig(origConfig, "MODEL_PLACEHOLDER_M26", "claude-opus-4-6-thinking")
	upMap, ok := updated.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", updated)
	}

	if upMap["otherField"] != "keepMe" {
		t.Errorf("expected otherField to be preserved")
	}

	pCfg, ok := upMap["plannerConfig"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected plannerConfig map, got %T", upMap["plannerConfig"])
	}

	if pCfg["planModel"] != "MODEL_PLACEHOLDER_M26" {
		t.Errorf("expected planModel MODEL_PLACEHOLDER_M26, got %v", pCfg["planModel"])
	}
	if pCfg["modelName"] != "claude-opus-4-6-thinking" {
		t.Errorf("expected modelName claude-opus-4-6-thinking, got %v", pCfg["modelName"])
	}
	reqM, ok := pCfg["requestedModel"].(map[string]interface{})
	if !ok || reqM["model"] != "MODEL_PLACEHOLDER_M26" {
		t.Errorf("expected requestedModel.model to be MODEL_PLACEHOLDER_M26, got %v", pCfg["requestedModel"])
	}

	// 2. Apply to nil config
	nilUpdated := applyModelToCascadeConfig(nil, "MODEL_PLACEHOLDER_M26", "claude-opus-4-6-thinking")
	nilMap, ok := nilUpdated.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map from nil config, got %T", nilUpdated)
	}
	pCfg2 := nilMap["plannerConfig"].(map[string]interface{})
	if pCfg2["planModel"] != "MODEL_PLACEHOLDER_M26" {
		t.Errorf("expected planModel MODEL_PLACEHOLDER_M26 from nil config, got %v", pCfg2["planModel"])
	}
}

func TestHandleCreateCascadeWithPromptModelSync(t *testing.T) {
	var startPayload map[string]interface{}
	var sendUserMsgPayload map[string]interface{}

	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/StartCascade") {
			json.NewDecoder(r.Body).Decode(&startPayload)
			w.Write([]byte(`{"cascadeId": "test-model-sync-123"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/UpdateConversationAnnotations") {
			w.Write([]byte(`{}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/SendUserCascadeMessage") {
			json.NewDecoder(r.Body).Decode(&sendUserMsgPayload)
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

	// Seed lastKnownConfig with Gemini model (simulating a prior session)
	staleConfig := []byte(`{"plannerConfig":{"planModel":"MODEL_PLACEHOLDER_M318","modelName":"gemini-3.8-flash-high"}}`)
	SetLastKnownCascadeConfig(staleConfig)

	wsDir := t.TempDir()
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", wsDir)

	body, _ := json.Marshal(CreateCascadeRequest{
		WorkspaceURI: filePathToURI(wsDir),
		Prompt:       "Analyze the codebase",
		Model:        "claude-opus-4-6-thinking",
	})
	req := httptest.NewRequest(http.MethodPost, "/gateway/cascade/new", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()

	p.HandleCreateCascade(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// 1. Verify StartCascade received MODEL_PLACEHOLDER_M26
	if startPayload["requestedModel"] != "MODEL_PLACEHOLDER_M26" {
		t.Errorf("StartCascade expected requestedModel MODEL_PLACEHOLDER_M26, got %v", startPayload["requestedModel"])
	}

	// 2. Verify SendUserCascadeMessage received cascadeConfig patched with Claude model instead of stale Gemini
	cfg, ok := sendUserMsgPayload["cascadeConfig"].(map[string]interface{})
	if !ok {
		t.Fatalf("SendUserCascadeMessage expected cascadeConfig map, got %T", sendUserMsgPayload["cascadeConfig"])
	}
	pCfg, ok := cfg["plannerConfig"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected plannerConfig, got %T", cfg["plannerConfig"])
	}

	if pCfg["planModel"] != "MODEL_PLACEHOLDER_M26" {
		t.Errorf("initial prompt expected planModel MODEL_PLACEHOLDER_M26, got %v", pCfg["planModel"])
	}
	if pCfg["modelName"] != "claude-opus-4-6-thinking" {
		t.Errorf("initial prompt expected modelName claude-opus-4-6-thinking, got %v", pCfg["modelName"])
	}
}

func TestApplyModelToCascadeConfig_ClaudeLimits(t *testing.T) {
	// 1. Initial Gemini config with 256000 maxTokenLimit
	initialGeminiConfig := map[string]interface{}{
		"plannerConfig": map[string]interface{}{
			"planModel": "MODEL_PLACEHOLDER_M318",
			"modelName": "gemini-3.8-flash-high",
		},
		"checkpointConfig": map[string]interface{}{
			"maxTokenLimit":       256000,
			"tokenThreshold":      140000,
			"isSync":              true,
			"useLastPlannerModel": true,
		},
	}

	// 2. Switch to Claude Opus 4.6 Thinking
	claudeConfig := applyModelToCascadeConfig(initialGeminiConfig, "MODEL_PLACEHOLDER_M26", "claude-opus-4-6-thinking")
	cfgMap, ok := claudeConfig.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", claudeConfig)
	}

	cpCfg, ok := cfgMap["checkpointConfig"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected checkpointConfig map, got %T", cfgMap["checkpointConfig"])
	}

	if limit, ok := cpCfg["maxTokenLimit"].(int); !ok || limit > 186000 || limit != 160000 {
		t.Errorf("expected Claude maxTokenLimit 160000 (must be <= 186000), got %v", cpCfg["maxTokenLimit"])
	}
	if thresh, ok := cpCfg["tokenThreshold"].(int); !ok || thresh != 50000 {
		t.Errorf("expected Claude tokenThreshold 50000, got %v", cpCfg["tokenThreshold"])
	}
	if sync, ok := cpCfg["isSync"].(bool); !ok || sync != false {
		t.Errorf("expected Claude isSync false, got %v", cpCfg["isSync"])
	}

	// 3. Switch back to Gemini
	geminiRestored := applyModelToCascadeConfig(claudeConfig, "MODEL_PLACEHOLDER_M318", "gemini-3.8-flash-high")
	geminiMap := geminiRestored.(map[string]interface{})
	geminiCp := geminiMap["checkpointConfig"].(map[string]interface{})

	if limit, ok := geminiCp["maxTokenLimit"].(int); !ok || limit != 256000 {
		t.Errorf("expected Gemini maxTokenLimit restored to 256000, got %v", geminiCp["maxTokenLimit"])
	}
	if thresh, ok := geminiCp["tokenThreshold"].(int); !ok || thresh != 140000 {
		t.Errorf("expected Gemini tokenThreshold restored to 140000, got %v", geminiCp["tokenThreshold"])
	}
}

func TestHandleCreateCascadePureChat(t *testing.T) {
	var lastPayload map[string]interface{}
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/StartCascade") {
			json.NewDecoder(r.Body).Decode(&lastPayload)
			w.Write([]byte(`{"cascadeId": "pure-chat-cascade-1"}`))
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

	// Case 1: Empty workspaceURI and empty projectID -> defaults to outside-of-project
	body1, _ := json.Marshal(CreateCascadeRequest{
		WorkspaceURI: "",
		ProjectID:    "",
	})
	req1 := httptest.NewRequest(http.MethodPost, "/gateway/cascade/new", strings.NewReader(string(body1)))
	rec1 := httptest.NewRecorder()
	p.HandleCreateCascade(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec1.Code, rec1.Body.String())
	}

	if src, ok := lastPayload["source"].(string); !ok || src != "CORTEX_TRAJECTORY_SOURCE_CASCADE_CLIENT" {
		t.Errorf("expected source CORTEX_TRAJECTORY_SOURCE_CASCADE_CLIENT, got %v", lastPayload["source"])
	}
	envCfg, ok := lastPayload["projectEnvConfig"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected projectEnvConfig in payload")
	}
	if pid, ok := envCfg["projectId"].(string); !ok || pid != "outside-of-project" {
		t.Errorf("expected projectId outside-of-project, got %v", envCfg["projectId"])
	}

	// Case 2: Explicit projectID "outside-of-project"
	lastPayload = nil
	body2, _ := json.Marshal(CreateCascadeRequest{
		WorkspaceURI: "",
		ProjectID:    "outside-of-project",
	})
	req2 := httptest.NewRequest(http.MethodPost, "/gateway/cascade/new", strings.NewReader(string(body2)))
	rec2 := httptest.NewRecorder()
	p.HandleCreateCascade(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
	envCfg2, ok := lastPayload["projectEnvConfig"].(map[string]interface{})
	if !ok || envCfg2["projectId"] != "outside-of-project" {
		t.Errorf("expected projectId outside-of-project, got %v", envCfg2)
	}
}

func TestLiveOfficialProjects(t *testing.T) {
	insp := inspector.NewInspector(5 * time.Second)
	info := insp.Scan()
	if info == nil {
		t.Skip("Antigravity instance not available, skipping test")
	}
	t.Logf("info: PID=%d Port=%d CSRF=%s IsHealthy=%v", info.PID, info.Port, info.CSRFToken, info.IsHealthy)

	p := NewProxy(insp)
	p.activePort = info.Port
	p.activeToken = info.CSRFToken

	items, err := p.fetchOfficialProjects(info.Port, info.CSRFToken, nil)
	t.Logf("fetchOfficialProjects err: %v, items count: %d", err, len(items))

	projects, err := p.GetProjects()
	t.Logf("GetProjects err: %v, count: %d", err, len(projects))
	for i, prj := range projects {
		if i < 3 || prj.Name == "antigravity-mobile" {
			t.Logf("[%d] ID=%q Name=%q URI=%q Path=%q", i, prj.ID, prj.Name, prj.URI, prj.Path)
		}
	}
}

func TestFetchProjectsFromWorkspaceStorage(t *testing.T) {
	tmpDir := t.TempDir()
	wsDir := filepath.Join(tmpDir, "Antigravity", "User", "workspaceStorage", "test-hash-123")
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		t.Fatalf("failed to create temp workspaceStorage: %v", err)
	}

	testProjectDir := filepath.Join(tmpDir, "MyTestProject")
	if err := os.MkdirAll(testProjectDir, 0755); err != nil {
		t.Fatalf("failed to create test project dir: %v", err)
	}

	wsJSON := fmt.Sprintf(`{"folder": "file://%s"}`, filepath.ToSlash(testProjectDir))
	if err := os.WriteFile(filepath.Join(wsDir, "workspace.json"), []byte(wsJSON), 0644); err != nil {
		t.Fatalf("failed to write workspace.json: %v", err)
	}

	origAppData := os.Getenv("APPDATA")
	os.Setenv("APPDATA", tmpDir)
	defer os.Setenv("APPDATA", origAppData)

	items := fetchProjectsFromWorkspaceStorage()
	found := false
	for _, item := range items {
		if item.Name == "MyTestProject" {
			found = true
			if item.LastActive == nil {
				t.Errorf("expected LastActive to be set from file ModTime")
			}
			break
		}
	}
	if !found {
		t.Errorf("expected MyTestProject to be discovered from workspaceStorage, got %d items", len(items))
	}
}

func TestFetchProjectsFromGeminiConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", tmpDir)
	cfgDir := filepath.Join(tmpDir, "config", "projects")
	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		t.Fatalf("failed to create temp config/projects: %v", err)
	}

	testProjectDir := filepath.Join(tmpDir, "DemoApp")
	if err := os.MkdirAll(testProjectDir, 0755); err != nil {
		t.Fatalf("failed to create test project dir: %v", err)
	}

	projJSON := fmt.Sprintf(`{
		"id": "project-uuid-1234-5678",
		"name": "DemoApp",
		"projectResources": {
			"resources": [
				{
					"gitFolder": {
						"folderUri": "file://%s",
						"defaultBranch": "main"
					}
				}
			]
		},
		"isWorkspaceOnly": false
	}`, filepath.ToSlash(testProjectDir))

	if err := os.WriteFile(filepath.Join(cfgDir, "project-uuid-1234-5678.json"), []byte(projJSON), 0644); err != nil {
		t.Fatalf("failed to write project config json: %v", err)
	}

	origAppDataDir := os.Getenv("ANTIGRAVITY_APP_DATA_DIR")
	os.Setenv("ANTIGRAVITY_APP_DATA_DIR", filepath.Join(tmpDir, "antigravity"))
	defer os.Setenv("ANTIGRAVITY_APP_DATA_DIR", origAppDataDir)

	items := fetchProjectsFromGeminiConfig()
	found := false
	for _, item := range items {
		if item.ID == "project-uuid-1234-5678" {
			found = true
			if item.Name != "DemoApp" {
				t.Errorf("expected DemoApp, got %s", item.Name)
			}
			if !strings.EqualFold(item.Path, testProjectDir) {
				t.Errorf("expected %s, got %s", testProjectDir, item.Path)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected project-uuid-1234-5678 to be discovered, got %d items", len(items))
	}
}

func TestHandleCreateCascade_AutoResolveProjectID(t *testing.T) {
	var lastReceivedPayload map[string]interface{}
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/StartCascade") {
			json.NewDecoder(r.Body).Decode(&lastReceivedPayload)
			w.Write([]byte(`{"cascadeId": "test-cascade-pid"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/UpdateConversationAnnotations") {
			w.Write([]byte(`{}`))
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer mockUpstream.Close()

	tmpDir := t.TempDir()
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", tmpDir)
	cfgDir := filepath.Join(tmpDir, "config", "projects")
	_ = os.MkdirAll(cfgDir, 0755)
	testProjectDir := filepath.Join(tmpDir, "TargetProject")
	_ = os.MkdirAll(testProjectDir, 0755)

	projJSON := fmt.Sprintf(`{
		"id": "target-pid-9999",
		"name": "TargetProject",
		"projectResources": {
			"resources": [
				{
					"gitFolder": {
						"folderUri": "file://%s"
					}
				}
			]
		}
	}`, filepath.ToSlash(testProjectDir))
	_ = os.WriteFile(filepath.Join(cfgDir, "target-pid-9999.json"), []byte(projJSON), 0644)

	origAppDataDir := os.Getenv("ANTIGRAVITY_APP_DATA_DIR")
	os.Setenv("ANTIGRAVITY_APP_DATA_DIR", filepath.Join(tmpDir, "antigravity"))
	defer os.Setenv("ANTIGRAVITY_APP_DATA_DIR", origAppDataDir)

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)
	p.activePort = port
	p.activeToken = "test-token"

	// Request from iOS without explicit projectId, only workspaceUri
	body, _ := json.Marshal(CreateCascadeRequest{
		WorkspaceURI: "file://" + filepath.ToSlash(testProjectDir),
		Prompt:       "hello from ios",
	})
	req := httptest.NewRequest(http.MethodPost, "/gateway/cascade/new", strings.NewReader(string(body)))
	w := httptest.NewRecorder()

	p.HandleCreateCascade(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify that upstream StartCascade received projectEnvConfig with target-pid-9999
	envCfg, ok := lastReceivedPayload["projectEnvConfig"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected projectEnvConfig in startPayload, got: %+v", lastReceivedPayload)
	}
	if envCfg["projectId"] != "target-pid-9999" {
		t.Errorf("expected projectId 'target-pid-9999', got: %v", envCfg["projectId"])
	}
	if lastReceivedPayload["source"] != "CORTEX_TRAJECTORY_SOURCE_CASCADE_CLIENT" {
		t.Errorf("expected CORTEX_TRAJECTORY_SOURCE_CASCADE_CLIENT, got: %v", lastReceivedPayload["source"])
	}
}

func TestProjectAliasEndpoint(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "multigravity-alias-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	origEnv := os.Getenv("MULTIGRAVITY_DATA_DIR")
	defer os.Setenv("MULTIGRAVITY_DATA_DIR", origEnv)
	os.Setenv("MULTIGRAVITY_DATA_DIR", tempDir)

	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)

	// 1. Set alias
	setBody, _ := json.Marshal(map[string]string{
		"path":  "/Users/test/my-cool-project",
		"alias": "我的酷项目",
	})
	req := httptest.NewRequest(http.MethodPost, "/gateway/projects/alias", bytes.NewReader(setBody))
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from POST /gateway/projects/alias, got %d: %s", rec.Code, rec.Body.String())
	}

	// 2. Verify stored in memory / disk
	aliases := loadWorkspaceAliases()
	normKey := normalizeWorkspaceKey("/Users/test/my-cool-project")
	if aliases[normKey] != "我的酷项目" {
		t.Errorf("expected '我的酷项目', got: %q", aliases[normKey])
	}

	// 3. Clear alias
	clearBody, _ := json.Marshal(map[string]string{
		"path":  "/Users/test/my-cool-project",
		"alias": "",
	})
	reqClear := httptest.NewRequest(http.MethodPost, "/gateway/projects/alias", bytes.NewReader(clearBody))
	recClear := httptest.NewRecorder()
	p.ServeHTTP(recClear, reqClear)

	if recClear.Code != http.StatusOK {
		t.Fatalf("expected 200 from clear alias, got %d: %s", recClear.Code, recClear.Body.String())
	}

	aliasesAfter := loadWorkspaceAliases()
	if _, exists := aliasesAfter[normKey]; exists {
		t.Errorf("expected alias to be deleted, but still exists: %v", aliasesAfter[normKey])
	}
}

func TestSidecarProjectsIgnoredAndNotOverwriting(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)
	t.Setenv("APPDATA", tmpHome)
	t.Setenv("ANTIGRAVITY_APP_DATA_DIR", filepath.Join(tmpHome, ".gemini", "antigravity"))
	t.Setenv("MULTIGRAVITY_DATA_DIR", filepath.Join(tmpHome, "mgy-data"))

	cfgRoot := filepath.Join(tmpHome, ".gemini", "config")
	projCfgDir := filepath.Join(cfgRoot, "projects")
	if err := os.MkdirAll(projCfgDir, 0755); err != nil {
		t.Fatalf("failed to create projects dir: %v", err)
	}

	wsDir := filepath.Join(tmpHome, "Websites", "hejvip-deep-research")
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		t.Fatalf("failed to create workspace dir: %v", err)
	}
	wsURI := "file://" + filepath.ToSlash(wsDir)

	realID := "0446b504-3d27-41e2-9a6b-ed8a5beff1df"
	sidecarID := "05621556-c9bc-4ea8-a876-58580b28555c"

	// Write config.json with sidecar mapping
	configJSON := fmt.Sprintf(`{
		"sidecars": {
			"deep-research-git": {
				"enabled": true,
				"projectId": %q,
				"schemaVersion": 1
			}
		}
	}`, sidecarID)
	if err := os.WriteFile(filepath.Join(cfgRoot, "config.json"), []byte(configJSON), 0644); err != nil {
		t.Fatalf("failed to write config.json: %v", err)
	}

	// Write both real project JSON and sidecar shadow project JSON pointing to the same folderUri
	realProjJSON := fmt.Sprintf(`{
		"id": %q,
		"name": "hejvip-deep-research",
		"projectResources": {
			"resources": [{"gitFolder": {"folderUri": %q}}]
		}
	}`, realID, wsURI)
	sidecarProjJSON := fmt.Sprintf(`{
		"id": %q,
		"name": "deep-research-git",
		"projectResources": {
			"resources": [{"gitFolder": {"folderUri": %q}}]
		}
	}`, sidecarID, wsURI)

	_ = os.WriteFile(filepath.Join(projCfgDir, realID+".json"), []byte(realProjJSON), 0644)
	_ = os.WriteFile(filepath.Join(projCfgDir, sidecarID+".json"), []byte(sidecarProjJSON), 0644)

	// Write app_storage.json where sidecarID appears after realID in projectsOrder
	appSupportDir := filepath.Join(tmpHome, "Antigravity")
	_ = os.MkdirAll(appSupportDir, 0755)
	appStorageJSON := fmt.Sprintf(`{
		"projectsOrder": "[%q,%q]"
	}`, realID, sidecarID)
	_ = os.WriteFile(filepath.Join(appSupportDir, "app_storage.json"), []byte(appStorageJSON), 0644)

	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/ReadProjects") {
			resp := fmt.Sprintf(`{
				"projects": [
					{
						"id": %q,
						"name": "hejvip-deep-research",
						"projectResources": {"resources": [{"gitFolder": {"folderUri": %q}}]}
					},
					{
						"id": %q,
						"name": "deep-research-git",
						"projectResources": {"resources": [{"gitFolder": {"folderUri": %q}}]}
					}
				]
			}`, realID, wsURI, sidecarID, wsURI)
			w.Write([]byte(resp))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories") {
			w.Write([]byte(`{"trajectorySummaries":{}}`))
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

	projects, err := p.GetProjects()
	if err != nil {
		t.Fatalf("GetProjects failed: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 deduplicated project, got %d: %+v", len(projects), projects)
	}
	if projects[0].ID != realID {
		t.Errorf("expected ID %q, got %q", realID, projects[0].ID)
	}
	if projects[0].Name != "hejvip-deep-research" {
		t.Errorf("expected Name 'hejvip-deep-research', got %q", projects[0].Name)
	}
}

func TestIsDriveRoot(t *testing.T) {
	driveRoots := []string{
		"C:\\", "c:\\", "D:\\", "d:\\", "Z:\\",
		"C:", "d:", "D:/", "/", "\\", ".", "",
	}
	for _, dr := range driveRoots {
		if !isDriveRoot(dr) {
			t.Errorf("expected isDriveRoot(%q) to be true, got false", dr)
		}
	}

	validPaths := []string{
		"C:\\Projects\\app",
		"D:\\Documents\\Work",
		"/home/user/project",
		"/var/www/site",
	}
	for _, vp := range validPaths {
		if isDriveRoot(vp) {
			t.Errorf("expected isDriveRoot(%q) to be false, got true", vp)
		}
	}
}

func TestGetProjects_StrictOfficialAlignment(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Official project config
	cfgDir := filepath.Join(tmpDir, "config", "projects")
	_ = os.MkdirAll(cfgDir, 0755)
	officialProjDir := filepath.Join(tmpDir, "OfficialApp")
	_ = os.MkdirAll(officialProjDir, 0755)

	officialJSON := fmt.Sprintf(`{
		"id": "proj-official-uuid-1",
		"name": "OfficialApp",
		"projectResources": {
			"resources": [
				{
					"gitFolder": {
						"folderUri": "file://%s",
						"defaultBranch": "main"
					}
				}
			]
		},
		"isWorkspaceOnly": false
	}`, filepath.ToSlash(officialProjDir))
	_ = os.WriteFile(filepath.Join(cfgDir, "proj-official-uuid-1.json"), []byte(officialJSON), 0644)

	// 2. Dirty workspaceStorage cache (with random folder and drive root)
	wsDir := filepath.Join(tmpDir, "Antigravity", "User", "workspaceStorage")
	dirtyFolder1 := filepath.Join(wsDir, "dirty-1")
	dirtyFolder2 := filepath.Join(wsDir, "dirty-2")
	_ = os.MkdirAll(dirtyFolder1, 0755)
	_ = os.MkdirAll(dirtyFolder2, 0755)

	dirtyDir := filepath.Join(tmpDir, "DirtyCachedFolder")
	_ = os.MkdirAll(dirtyDir, 0755)
	_ = os.WriteFile(filepath.Join(dirtyFolder1, "workspace.json"), []byte(fmt.Sprintf(`{"folder":"file://%s"}`, filepath.ToSlash(dirtyDir))), 0644)
	_ = os.WriteFile(filepath.Join(dirtyFolder2, "workspace.json"), []byte(`{"folder":"file:///d%3A/"}`), 0644)

	origAppData := os.Getenv("APPDATA")
	origAppDataDir := os.Getenv("ANTIGRAVITY_APP_DATA_DIR")
	origCacheEnv := os.Getenv("MGY_INCLUDE_WORKSPACE_CACHE")
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")

	os.Setenv("APPDATA", tmpDir)
	os.Setenv("ANTIGRAVITY_APP_DATA_DIR", filepath.Join(tmpDir, "antigravity"))
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)
	os.Unsetenv("MGY_INCLUDE_WORKSPACE_CACHE")
	defer func() {
		os.Setenv("APPDATA", origAppData)
		os.Setenv("ANTIGRAVITY_APP_DATA_DIR", origAppDataDir)
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUserProfile)
		if origCacheEnv != "" {
			os.Setenv("MGY_INCLUDE_WORKSPACE_CACHE", origCacheEnv)
		} else {
			os.Unsetenv("MGY_INCLUDE_WORKSPACE_CACHE")
		}
	}()

	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)

	// By default, strict alignment should only yield the official project
	projects, err := p.GetProjects()
	if err != nil {
		t.Fatalf("GetProjects failed: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected strictly 1 official project, got %d: %+v", len(projects), projects)
	}
	if projects[0].Name != "OfficialApp" {
		t.Errorf("expected project name 'OfficialApp', got %q", projects[0].Name)
	}
	if projects[0].ID != "proj-official-uuid-1" {
		t.Errorf("expected project ID 'proj-official-uuid-1', got %q", projects[0].ID)
	}

	// When MGY_INCLUDE_WORKSPACE_CACHE=true, dirty cache is allowed, but drive root is still filtered out
	os.Setenv("MGY_INCLUDE_WORKSPACE_CACHE", "true")
	projectsWithCache, err := p.GetProjects()
	if err != nil {
		t.Fatalf("GetProjects with cache failed: %v", err)
	}
	if len(projectsWithCache) != 2 {
		t.Fatalf("expected 2 projects (official + dirty folder, root filtered), got %d: %+v", len(projectsWithCache), projectsWithCache)
	}
	for _, prj := range projectsWithCache {
		if prj.Name == "\\" || prj.Name == "/" || prj.Path == "d:\\" || prj.Path == "d:/" {
			t.Errorf("drive root was not filtered: %+v", prj)
		}
	}
}

func TestBuildWorkspaceDisplayNameResolver(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("ANTIGRAVITY_APP_DATA_DIR", filepath.Join(tmpHome, ".gemini", "antigravity"))
	t.Setenv("MULTIGRAVITY_DATA_DIR", filepath.Join(tmpHome, ".multigravity"))
	t.Setenv("APPDATA", "")

	cfgDir := filepath.Join(tmpHome, ".gemini", "config")
	projDir := filepath.Join(cfgDir, "projects")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatal(err)
	}
	mkRepo := func(name string) string {
		dir := filepath.Join(tmpHome, "Projects", name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	fileURI := func(dir string) string { return "file://" + filepath.ToSlash(dir) }
	writeProject := func(id, name, dir string) {
		cfg := fmt.Sprintf(`{"id": %q, "name": %q, "projectResources": {"resources": [{"gitFolder": {"folderUri": %q}}]}}`,
			id, name, fileURI(dir))
		if err := os.WriteFile(filepath.Join(projDir, id+".json"), []byte(cfg), 0644); err != nil {
			t.Fatal(err)
		}
	}

	repoDir := mkRepo("wechat-bot")
	aliasDir := mkRepo("alias-repo")
	sidecarDir := mkRepo("sidecar-repo")
	writeProject("proj-truck-9000", "Truck9000", repoDir)
	writeProject("proj-alias", "AliasProj", aliasDir)

	// Sidecar shadow project: declared in config.json and only visible through the ReadProjects cache.
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"),
		[]byte(`{"sidecars": {"sc-1": {"projectId": "proj-sidecar"}}}`), 0644); err != nil {
		t.Fatal(err)
	}

	// Workspace alias for aliasDir must take priority over the project's official name.
	if err := saveWorkspaceAlias(aliasDir, "别名项目"); err != nil {
		t.Fatal(err)
	}

	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)
	p.projectsCacheMu.Lock()
	p.lastReadProjectsResp = []byte(fmt.Sprintf(
		`{"projects": [{"id": "proj-sidecar", "name": "Scheduled Task", "projectResources": {"resources": [{"folderUri": %q}]}}]}`,
		fileURI(sidecarDir)))
	p.projectsCacheMu.Unlock()
	resolve := p.BuildWorkspaceDisplayNameResolver()

	// 1. Resolve by projectId even when folder name differs ("wechat-bot" vs "Truck9000")
	if pid, disp := resolve("proj-truck-9000", []string{fileURI(repoDir)}); pid != "proj-truck-9000" || disp != "Truck9000" {
		t.Fatalf("expected (proj-truck-9000, Truck9000), got (%q, %q)", pid, disp)
	}

	// 2. Backfill projectId and display name from workspaceURI when projectId is missing
	if pid, disp := resolve("", []string{fileURI(repoDir)}); pid != "proj-truck-9000" || disp != "Truck9000" {
		t.Fatalf("expected backfilled (proj-truck-9000, Truck9000), got (%q, %q)", pid, disp)
	}

	// 3. Workspace alias wins over the official project name
	if pid, disp := resolve("proj-alias", nil); pid != "proj-alias" || disp != "别名项目" {
		t.Fatalf("expected (proj-alias, 别名项目), got (%q, %q)", pid, disp)
	}

	// 4. Sidecar shadow projects must never be back-filled onto regular conversations by folder URI,
	// while explicit scheduled-task sidecar projectIds retain their own task display name and flag.
	for _, inputPID := range []string{"", "outside-of-project"} {
		if pid, disp := resolve(inputPID, []string{fileURI(sidecarDir)}); pid != "" || disp != "" {
			t.Fatalf("input %q: expected sidecar workspace to stay unresolved for regular conversation, got (%q, %q)", inputPID, pid, disp)
		}
	}
	fullResolve := p.BuildWorkspaceResolver()
	if res := fullResolve("proj-sidecar", []string{fileURI(sidecarDir)}, nil); res.ProjectID != "proj-sidecar" || res.WorkspaceName != "Scheduled Task" || !res.IsScheduledTask {
		t.Fatalf("expected scheduled task sidecar to resolve to (proj-sidecar, Scheduled Task, true), got %+v", res)
	}

	// 5. A known-but-unregistered projectId must not borrow another project's name via its folder
	if pid, disp := resolve("proj-unknown", []string{fileURI(repoDir)}); pid != "proj-unknown" || disp == "Truck9000" {
		t.Fatalf("expected (proj-unknown, not Truck9000), got (%q, %q)", pid, disp)
	}
}

func TestScheduledTasksPreserveIndependentFolderNames(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)
	t.Setenv("APPDATA", "")
	t.Setenv("ANTIGRAVITY_APP_DATA_DIR", filepath.Join(tmpHome, ".gemini", "antigravity"))
	t.Setenv("MULTIGRAVITY_DATA_DIR", filepath.Join(tmpHome, ".multigravity"))
	invalidateGeminiConfigCaches()
	t.Cleanup(invalidateGeminiConfigCaches)

	cfgDir := filepath.Join(tmpHome, ".gemini", "config")
	projDir := filepath.Join(cfgDir, "projects")
	sidecarsDir := filepath.Join(cfgDir, "sidecars")
	repoDir := filepath.Join(tmpHome, "Projects", "antigravity-mobile")
	for _, d := range []string{projDir, repoDir, filepath.Join(sidecarsDir, "x-golden-slot-outreach"), filepath.Join(sidecarsDir, "non-x-channel-outreach")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	wsURI := "file://" + filepath.ToSlash(repoDir)

	const (
		realPID     = "6e8f5d1a-90eb-41d0-8480-be82d3ee635a"
		taskXPID    = "17985f0b-c095-4af3-b70e-0b475c9377f5"
		taskNonXPID = "4c96ad73-ff50-433c-970f-f3b5b61172f6"
		pluginPID   = "b7bfdd49-a721-4b43-83b6-95d2de6dd32e"
	)

	writeFile := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeFile(filepath.Join(cfgDir, "config.json"), fmt.Sprintf(`{
		"sidecars": {
			"x-golden-slot-outreach": {"enabled": true, "projectId": %q},
			"non-x-channel-outreach": {"enabled": true, "projectId": %q},
			"swarm/swarm_board": {"enabled": true, "projectId": %q}
		}
	}`, taskXPID, taskNonXPID, pluginPID))

	writeFile(filepath.Join(projDir, realPID+".json"), fmt.Sprintf(`{
		"id": %q,
		"name": "antigravity-mobile",
		"projectResources": {"resources": [{"gitFolder": {"folderUri": %q}}]}
	}`, realPID, wsURI))
	writeFile(filepath.Join(projDir, taskXPID+".json"), fmt.Sprintf(`{
		"id": %q,
		"name": "x-golden-slot-outreach",
		"projectResources": {"resources": [{"folderUri": %q}]},
		"sidecar": {"sidecarId": "x-golden-slot-outreach"}
	}`, taskXPID, wsURI))
	writeFile(filepath.Join(projDir, taskNonXPID+".json"), fmt.Sprintf(`{
		"id": %q,
		"name": "non-x-channel-outreach",
		"projectResources": {"resources": [{"folderUri": %q}]},
		"sidecar": {"sidecarId": "non-x-channel-outreach"}
	}`, taskNonXPID, wsURI))
	writeFile(filepath.Join(projDir, pluginPID+".json"), fmt.Sprintf(`{
		"id": %q,
		"name": "swarm/swarm_board",
		"projectResources": {},
		"sidecar": {"sidecarId": "swarm/swarm_board"}
	}`, pluginPID))

	writeFile(filepath.Join(sidecarsDir, "x-golden-slot-outreach", "sidecar.json"), `{"builtin":"schedule","display_name":"X Golden Slot Outreach & Review"}`)
	writeFile(filepath.Join(sidecarsDir, "non-x-channel-outreach", "sidecar.json"), `{"builtin":"schedule","display_name":"Non-X Channel Outreach & Review"}`)

	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories") {
			resp := fmt.Sprintf(`{
				"trajectorySummaries": {
					"conv-normal": {
						"summary": "Regular feature work",
						"stepCount": 5,
						"status": "CASCADE_RUN_STATUS_IDLE",
						"trajectoryMetadata": {
							"projectId": %q,
							"workspaceUris": [%q]
						}
					},
					"conv-task-x": {
						"summary": "X patrol run",
						"stepCount": 12,
						"status": "CASCADE_RUN_STATUS_IDLE",
						"trajectoryMetadata": {
							"projectId": %q,
							"workspaceUris": [%q],
							"sourceMetadata": {
								"sidecar": {
									"sidecarId": "x-golden-slot-outreach",
									"config": {"builtin": "schedule", "displayName": "X Golden Slot Outreach & Review"}
								}
							}
						}
					},
					"conv-task-nonx": {
						"summary": "Non-X outreach run",
						"stepCount": 8,
						"status": "CASCADE_RUN_STATUS_IDLE",
						"trajectoryMetadata": {
							"projectId": %q,
							"workspaceUris": [%q]
						}
					},
					"conv-source-only": {
						"summary": "Ad-hoc scheduled task without local project json",
						"stepCount": 4,
						"status": "CASCADE_RUN_STATUS_IDLE",
						"trajectoryMetadata": {
							"projectId": "outside-of-project",
							"workspaceUris": [%q],
							"sourceMetadata": {
								"sidecar": {
									"sidecarId": "mgy-community-daily-voc",
									"config": {"builtin": "schedule"}
								}
							}
						}
					},
					"conv-plugin": {
						"summary": "Plugin sidecar conversation",
						"stepCount": 3,
						"status": "CASCADE_RUN_STATUS_IDLE",
						"trajectoryMetadata": {
							"projectId": %q,
							"workspaceUris": [%q],
							"sourceMetadata": {
								"sidecar": {
									"sidecarId": "swarm/swarm_board",
									"config": {"hasWebUi": true}
								}
							}
						}
					}
				}
			}`, realPID, wsURI, taskXPID, wsURI, taskNonXPID, wsURI, wsURI, pluginPID, wsURI)
			_, _ = w.Write([]byte(resp))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)
	p.activePort = port
	p.activeToken = "test-token"

	// 1. GetProjects must still expose only the real project (never pollute New Conversation picker with sidecars).
	projects, err := p.GetProjects()
	if err != nil {
		t.Fatalf("GetProjects failed: %v", err)
	}
	if len(projects) != 1 || projects[0].ID != realPID || projects[0].Name != "antigravity-mobile" {
		t.Fatalf("expected only real project antigravity-mobile in GetProjects, got %+v", projects)
	}

	// 2. GetAllCascadeTrajectories must give each scheduled task its independent folder name, preserve its projectId, and set isScheduledTask=true.
	req := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	p.handleGetAllCascadeTrajectories(rec, req, port, "test-token")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var out struct {
		TrajectorySummaries map[string]struct {
			WorkspaceName      string `json:"workspaceName"`
			IsScheduledTask    bool   `json:"isScheduledTask"`
			TrajectoryMetadata struct {
				ProjectID string `json:"projectId"`
			} `json:"trajectoryMetadata"`
		} `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	check := func(id, wantWS, wantPID string, wantSched bool) {
		t.Helper()
		got, ok := out.TrajectorySummaries[id]
		if !ok {
			t.Fatalf("missing trajectory %s", id)
		}
		if got.WorkspaceName != wantWS || got.IsScheduledTask != wantSched || got.TrajectoryMetadata.ProjectID != wantPID {
			t.Errorf("%s: got (ws=%q, pid=%q, sched=%v), want (ws=%q, pid=%q, sched=%v)",
				id, got.WorkspaceName, got.TrajectoryMetadata.ProjectID, got.IsScheduledTask,
				wantWS, wantPID, wantSched)
		}
	}

	check("conv-normal", "antigravity-mobile", realPID, false)
	check("conv-task-x", "x-golden-slot-outreach", taskXPID, true)
	check("conv-task-nonx", "non-x-channel-outreach", taskNonXPID, true)
	check("conv-source-only", "mgy-community-daily-voc", "outside-of-project", true)
	check("conv-plugin", "antigravity-mobile", realPID, false)
}

func TestHandleCreateCascade_RegisteredExternalProjectOutsideHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", "")
	t.Setenv("ANTIGRAVITY_APP_DATA_DIR", "")
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", "")
	t.Setenv("MULTIGRAVITY_ALLOWED_WORKSPACE_ROOTS", "")
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	extRoot := t.TempDir()
	yijuDir := filepath.Join(extRoot, "yiju_9000")
	rpcOnlyDir := filepath.Join(extRoot, "rpc_only_external")
	unregDir := filepath.Join(extRoot, "unregistered_dir")
	for _, d := range []string{yijuDir, rpcOnlyDir, unregDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// 1. Register yiju_9000 in ~/.gemini/config/projects/
	cfgDir := filepath.Join(home, ".gemini", "config", "projects")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yijuJSON := fmt.Sprintf(`{
		"id": "yiju-pid-001",
		"name": "yiju_9000",
		"projectResources": {"resources": [{"folderUri": %q}]}
	}`, filePathToURI(yijuDir))
	if err := os.WriteFile(filepath.Join(cfgDir, "yiju-pid-001.json"), []byte(yijuJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Put rpcOnlyDir ONLY in app_storage.json + upstream ReadProjects RPC (not in ~/.gemini/config/projects/)
	appStorageDir := filepath.Join(home, "Library", "Application Support", "Antigravity")
	if err := os.MkdirAll(appStorageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appStorageDir, "app_storage.json"), []byte(`{"projectsOrder":"[\"rpc-only-pid-002\"]"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/ReadProjects"):
			resp := fmt.Sprintf(`{"projects":[{"id":"rpc-only-pid-002","name":"rpc_only_external","projectResources":{"resources":[{"folderUri":%q}]}}]}`, filePathToURI(rpcOnlyDir))
			_, _ = w.Write([]byte(resp))
		case strings.HasSuffix(r.URL.Path, "/StartCascade"):
			_, _ = w.Write([]byte(`{"cascadeId":"cascade-ext-ok"}`))
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			_, _ = w.Write([]byte(`{"trajectorySummaries":{}}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)
	p.activePort = port
	p.activeToken = "test-token"

	// Case A: Config-registered project outside $HOME succeeds without ALLOWED_WORKSPACE_ROOTS
	bodyA, _ := json.Marshal(CreateCascadeRequest{
		WorkspaceURI: filePathToURI(yijuDir),
		ProjectID:    "yiju-pid-001",
		Prompt:       "你好",
	})
	recA := httptest.NewRecorder()
	p.HandleCreateCascade(recA, httptest.NewRequest(http.MethodPost, "/gateway/cascade/new", strings.NewReader(string(bodyA))))
	if recA.Code != http.StatusOK {
		t.Fatalf("Case A (config-registered external workspace) expected 200, got %d: %s", recA.Code, recA.Body.String())
	}

	// Case B: Cold-start fallback discovers ReadProjects-only workspace outside $HOME
	bodyB, _ := json.Marshal(CreateCascadeRequest{
		WorkspaceURI: filePathToURI(rpcOnlyDir),
		ProjectID:    "rpc-only-pid-002",
		Prompt:       "你好",
	})
	recB := httptest.NewRecorder()
	p.HandleCreateCascade(recB, httptest.NewRequest(http.MethodPost, "/gateway/cascade/new", strings.NewReader(string(bodyB))))
	if recB.Code != http.StatusOK {
		t.Fatalf("Case B (ReadProjects cold-start fallback) expected 200, got %d: %s", recB.Code, recB.Body.String())
	}

	// Case C: Unregistered directory outside $HOME is still rejected with 403
	bodyC, _ := json.Marshal(CreateCascadeRequest{
		WorkspaceURI: filePathToURI(unregDir),
		Prompt:       "你好",
	})
	recC := httptest.NewRecorder()
	p.HandleCreateCascade(recC, httptest.NewRequest(http.MethodPost, "/gateway/cascade/new", strings.NewReader(string(bodyC))))
	if recC.Code != http.StatusForbidden {
		t.Fatalf("Case C (unregistered external workspace) expected 403, got %d: %s", recC.Code, recC.Body.String())
	}
}

func TestGetProjects_MultipleProjectsSameFolderURI(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)
	t.Setenv("APPDATA", tmpHome)
	t.Setenv("ANTIGRAVITY_APP_DATA_DIR", filepath.Join(tmpHome, ".gemini", "antigravity"))
	t.Setenv("MULTIGRAVITY_DATA_DIR", filepath.Join(tmpHome, ".multigravity"))
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", tmpHome)
	t.Setenv("MGY_INCLUDE_WORKSPACE_CACHE", "")
	invalidateGeminiConfigCaches()
	t.Cleanup(invalidateGeminiConfigCaches)

	sharedDir := filepath.Join(tmpHome, "workspace")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sharedURI := filePathToURI(sharedDir)

	projCfgDir := filepath.Join(tmpHome, ".gemini", "config", "projects")
	if err := os.MkdirAll(projCfgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	const (
		idA = "11111111-1111-4111-8111-111111111111"
		idB = "22222222-2222-4222-8222-222222222222"
		idC = "33333333-3333-4333-8333-333333333333"
	)

	writeCfg := func(id, name string, mod time.Time) {
		t.Helper()
		p := filepath.Join(projCfgDir, id+".json")
		body := fmt.Sprintf(`{
			"id": %q,
			"name": %q,
			"projectResources": {"resources": [{"folderUri": %q}]}
		}`, id, name, sharedURI)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
	}

	baseMod := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	writeCfg(idA, "ProjectA", baseMod)
	writeCfg(idB, "ProjectB", baseMod)

	// Also add an app_storage.json referencing idA, idB, and an RPC-only idC sharing the same folder.
	appSupportDir := filepath.Join(tmpHome, "Antigravity")
	if err := os.MkdirAll(appSupportDir, 0o755); err != nil {
		t.Fatal(err)
	}
	orderJSON, err := json.Marshal([]string{idA, idB, idC})
	if err != nil {
		t.Fatal(err)
	}
	appStorageJSON, err := json.Marshal(map[string]string{"projectsOrder": string(orderJSON)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appSupportDir, "app_storage.json"), appStorageJSON, 0o644); err != nil {
		t.Fatal(err)
	}

	// Add a legacy workspaceStorage entry for the exact same folder to verify MGY_INCLUDE_WORKSPACE_CACHE=true
	// neither spawns a duplicate ID-less item nor pollutes per-project counts.
	wsEntryDir := filepath.Join(tmpHome, "Antigravity", "User", "workspaceStorage", "shared-hash")
	if err := os.MkdirAll(wsEntryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsEntryDir, "workspace.json"), []byte(fmt.Sprintf(`{"folder":%q}`, sharedURI)), 0o644); err != nil {
		t.Fatal(err)
	}

	var lastStartPayload map[string]interface{}
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/ReadProjects"):
			resp := fmt.Sprintf(`{
				"projects": [
					{"id": %q, "name": "ProjectA", "projectResources": {"resources": [{"folderUri": %q}]}},
					{"id": %q, "name": "ProjectB", "projectResources": {"resources": [{"folderUri": %q}]}},
					{"id": %q, "name": "ProjectC", "projectResources": {"resources": [{"folderUri": %q}]}}
				]
			}`, idA, sharedURI, idB, sharedURI, idC, sharedURI)
			_, _ = w.Write([]byte(resp))
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			resp := fmt.Sprintf(`{
				"trajectorySummaries": {
					"conv-a-1": {
						"lastModifiedTime": "2026-11-26T10:00:00Z",
						"trajectoryMetadata": {"projectId": %q, "workspaceUris": [%q]}
					},
					"conv-a-2": {
						"lastModifiedTime": "2026-11-26T11:00:00Z",
						"trajectoryMetadata": {"projectId": %q, "workspaceUris": [%q]}
					},
					"conv-b-1": {
						"lastModifiedTime": "2026-11-26T12:00:00Z",
						"trajectoryMetadata": {"projectId": %q, "workspaceUris": [%q]}
					},
					"conv-unassigned": {
						"lastModifiedTime": "2026-11-26T13:00:00Z",
						"trajectoryMetadata": {"workspaceUris": [%q]}
					}
				}
			}`, idA, sharedURI, idA, sharedURI, idB, sharedURI, sharedURI)
			_, _ = w.Write([]byte(resp))
		case strings.HasSuffix(r.URL.Path, "/StartCascade"):
			lastStartPayload = nil
			_ = json.NewDecoder(r.Body).Decode(&lastStartPayload)
			_, _ = w.Write([]byte(`{"cascadeId":"cascade-multi-proj-b"}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer mockUpstream.Close()

	// 1. fetchProjectsFromGeminiConfig must retain both disk projects sharing the same folderUri.
	cfgItems := fetchProjectsFromGeminiConfig()
	if len(cfgItems) != 2 {
		t.Fatalf("expected 2 projects from gemini config, got %d: %+v", len(cfgItems), cfgItems)
	}

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)
	p.activePort = port
	p.activeToken = "test-token"

	assertProjects := func(label string, projects []ProjectItem) {
		t.Helper()
		if len(projects) != 3 {
			t.Fatalf("%s: expected 3 distinct projects, got %d: %+v", label, len(projects), projects)
		}
		byID := make(map[string]ProjectItem, len(projects))
		for _, item := range projects {
			byID[item.ID] = item
		}

		pA, okA := byID[idA]
		pB, okB := byID[idB]
		pC, okC := byID[idC]
		if !okA || !okB || !okC {
			t.Fatalf("%s: missing expected projects in %+v", label, projects)
		}
		if pA.Name != "ProjectA" || pA.SessionCount != 2 {
			t.Errorf("%s: ProjectA got Name=%q SessionCount=%d, want Name=ProjectA SessionCount=2", label, pA.Name, pA.SessionCount)
		}
		if pB.Name != "ProjectB" || pB.SessionCount != 1 {
			t.Errorf("%s: ProjectB got Name=%q SessionCount=%d, want Name=ProjectB SessionCount=1", label, pB.Name, pB.SessionCount)
		}
		if pC.Name != "ProjectC" || pC.SessionCount != 0 {
			t.Errorf("%s: ProjectC got Name=%q SessionCount=%d, want Name=ProjectC SessionCount=0", label, pC.Name, pC.SessionCount)
		}

		// Verify timestamps did not bleed across sibling projects: B (12:00) > A (11:00) > C (nil).
		if pA.LastActive == nil || !pA.LastActive.Equal(time.Date(2026, 11, 26, 11, 0, 0, 0, time.UTC)) {
			t.Errorf("%s: ProjectA LastActive = %v, want 2026-11-26T11:00:00Z", label, pA.LastActive)
		}
		if pB.LastActive == nil || !pB.LastActive.Equal(time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)) {
			t.Errorf("%s: ProjectB LastActive = %v, want 2026-11-26T12:00:00Z", label, pB.LastActive)
		}
		if pC.LastActive != nil {
			t.Errorf("%s: ProjectC LastActive = %v, want nil (no sessions, RPC-only)", label, pC.LastActive)
		}
		if projects[0].ID != idB || projects[1].ID != idA || projects[2].ID != idC {
			t.Errorf("%s: expected order [ProjectB, ProjectA, ProjectC], got [%s, %s, %s]",
				label, projects[0].Name, projects[1].Name, projects[2].Name)
		}
	}

	// 2. GET /gateway/projects returns all 3 projects with isolated per-projectId session counts.
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/gateway/projects", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /gateway/projects returned %d: %s", rec.Code, rec.Body.String())
	}
	var gotProjects []ProjectItem
	if err := json.Unmarshal(rec.Body.Bytes(), &gotProjects); err != nil {
		t.Fatalf("decode /gateway/projects: %v", err)
	}
	assertProjects("default", gotProjects)

	// 3. Even with MGY_INCLUDE_WORKSPACE_CACHE=true, legacy workspaceStorage does not duplicate the shared folder.
	t.Setenv("MGY_INCLUDE_WORKSPACE_CACHE", "true")
	p.invalidateProjectsCache()
	gotWithCache, err := p.GetProjects()
	if err != nil {
		t.Fatalf("GetProjects(cache=true) failed: %v", err)
	}
	assertProjects("with-workspace-cache", gotWithCache)

	// 4. Workspace resolver maps each projectId to its own project name despite sharing folderUri.
	resolve := p.BuildWorkspaceResolver()
	if resA := resolve(idA, []string{sharedURI}, nil); resA.ProjectID != idA || resA.WorkspaceName != "ProjectA" {
		t.Errorf("resolve(idA) = %+v, want (idA, ProjectA)", resA)
	}
	if resB := resolve(idB, []string{sharedURI}, nil); resB.ProjectID != idB || resB.WorkspaceName != "ProjectB" {
		t.Errorf("resolve(idB) = %+v, want (idB, ProjectB)", resB)
	}

	// 5. Creating a new cascade targeting ProjectB forwards ProjectB's projectId accurately.
	createBody, _ := json.Marshal(CreateCascadeRequest{
		WorkspaceURI: sharedURI,
		ProjectID:    idB,
		Prompt:       "hello project B",
	})
	recCreate := httptest.NewRecorder()
	p.HandleCreateCascade(recCreate, httptest.NewRequest(http.MethodPost, "/gateway/cascade/new", bytes.NewReader(createBody)))
	if recCreate.Code != http.StatusOK {
		t.Fatalf("HandleCreateCascade returned %d: %s", recCreate.Code, recCreate.Body.String())
	}
	envCfg, _ := lastStartPayload["projectEnvConfig"].(map[string]interface{})
	if envCfg["projectId"] != idB {
		t.Fatalf("expected StartCascade projectId %q, got %+v", idB, lastStartPayload)
	}
}

func TestGetProjects_SingleProjectFoldsLegacyUnassignedSessions(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)
	t.Setenv("APPDATA", tmpHome)
	t.Setenv("ANTIGRAVITY_APP_DATA_DIR", filepath.Join(tmpHome, ".gemini", "antigravity"))
	t.Setenv("MULTIGRAVITY_DATA_DIR", filepath.Join(tmpHome, ".multigravity"))
	t.Setenv("MGY_INCLUDE_WORKSPACE_CACHE", "")
	invalidateGeminiConfigCaches()
	t.Cleanup(invalidateGeminiConfigCaches)

	repoDir := filepath.Join(tmpHome, "solo-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	repoURI := filePathToURI(repoDir)

	cfgRoot := filepath.Join(tmpHome, ".gemini", "config")
	projCfgDir := filepath.Join(cfgRoot, "projects")
	if err := os.MkdirAll(projCfgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	const (
		soloID    = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		sidecarID = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	)

	if err := os.WriteFile(filepath.Join(cfgRoot, "config.json"),
		[]byte(fmt.Sprintf(`{"sidecars":{"nightly-job":{"enabled":true,"projectId":%q}}}`, sidecarID)), 0o644); err != nil {
		t.Fatal(err)
	}
	soloCfg := fmt.Sprintf(`{"id":%q,"name":"SoloRepo","projectResources":{"resources":[{"folderUri":%q}]}}`, soloID, repoURI)
	soloCfgPath := filepath.Join(projCfgDir, soloID+".json")
	if err := os.WriteFile(soloCfgPath, []byte(soloCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	baseMod := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	if err := os.Chtimes(soloCfgPath, baseMod, baseMod); err != nil {
		t.Fatal(err)
	}

	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories") {
			resp := fmt.Sprintf(`{
				"trajectorySummaries": {
					"conv-new": {
						"lastModifiedTime": "2026-11-26T09:00:00Z",
						"trajectoryMetadata": {"projectId": %q, "workspaceUris": [%q]}
					},
					"conv-legacy": {
						"lastModifiedTime": "2026-11-26T10:00:00Z",
						"trajectoryMetadata": {"workspaceUris": [%q]}
					},
					"conv-sidecar": {
						"lastModifiedTime": "2026-11-26T11:00:00Z",
						"trajectoryMetadata": {"projectId": %q, "workspaceUris": [%q]}
					}
				}
			}`, soloID, repoURI, repoURI, sidecarID, repoURI)
			_, _ = w.Write([]byte(resp))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)
	p.activePort = port
	p.activeToken = "test-token"

	projects, err := p.GetProjects()
	if err != nil {
		t.Fatalf("GetProjects failed: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d: %+v", len(projects), projects)
	}
	// Should fold conv-new (by soloID) + conv-legacy (unassigned URI) = 2, while excluding conv-sidecar.
	if projects[0].SessionCount != 2 {
		t.Errorf("expected SessionCount=2 (1 projectId + 1 legacy unassigned, excluding sidecar), got %d", projects[0].SessionCount)
	}
	wantActive := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	if projects[0].LastActive == nil || !projects[0].LastActive.Equal(wantActive) {
		t.Errorf("expected LastActive=%v, got %v", wantActive, projects[0].LastActive)
	}
}
