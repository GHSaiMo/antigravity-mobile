package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveLocalFilePath(t *testing.T) {
	home, _ := os.UserHomeDir()

	// 1. Static artifact path
	p, err := ResolveLocalFilePath("/static/artifacts/test-casc-1/plan.md", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(home, ".gemini/antigravity/brain/test-casc-1/plan.md")
	if p != expected {
		t.Errorf("expected %s, got %s", expected, p)
	}

	// 2. file:// protocol
	p, err = ResolveLocalFilePath("file://"+expected, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p != expected {
		t.Errorf("expected %s, got %s", expected, p)
	}

	// 3. Bare filename with cascadeID
	p, err = ResolveLocalFilePath("implementation_plan.md", "my-casc-42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected = filepath.Join(home, ".gemini/antigravity/brain/my-casc-42/implementation_plan.md")
	if p != expected {
		t.Errorf("expected %s, got %s", expected, p)
	}
}

func TestIsSafeFilePath(t *testing.T) {
	home, _ := os.UserHomeDir()

	if IsSafeFilePath(filepath.Join(home, ".ssh/id_rsa")) {
		t.Errorf("expected sensitive ssh key to be rejected")
	}

	if IsSafeFilePath("/etc/passwd") {
		t.Errorf("expected /etc/passwd to be rejected")
	}

	if !IsSafeFilePath(filepath.Join(home, ".gemini/antigravity/brain/abc/plan.md")) {
		t.Errorf("expected artifact file to be safe")
	}

	if IsSafeFilePath(filepath.Join(home, "Downloads/sample.png")) {
		t.Errorf("expected Downloads to be outside the default allowlist")
	}

	if IsSafeFilePath(filepath.Join(home, "Downloads/.env")) {
		t.Errorf("expected Downloads .env to be blocked")
	}

	if IsSafeFilePath(filepath.Join(home, ".agents/skills/xueqiu-radar/data/discovered_cubes_full.json")) {
		t.Errorf("expected .agents to be outside the default allowlist")
	}
	if !IsSafeFilePath(filepath.Join(home, ".gemini/config/skills/my-skill/SKILL.md")) {
		t.Errorf("expected .gemini/config skill file to be safe")
	}
	if !IsSafeFilePath(filepath.Join(home, "Projects/demo/readme.md")) {
		t.Errorf("expected Projects files to be safe")
	}

	// Verify sensitive auth files are blocked even in allowed dirs
	if IsSafeFilePath(filepath.Join(home, ".gemini/oauth_creds.json")) {
		t.Errorf("expected oauth_creds.json to be blocked")
	}
	if IsSafeFilePath(filepath.Join(home, ".gemini/jetski-standalone-oauth-token")) {
		t.Errorf("expected jetski-standalone-oauth-token to be blocked")
	}
	if IsSafeFilePath(filepath.Join(home, ".antigravity-mobile/auth_store.json")) {
		t.Errorf("expected auth_store.json to be blocked")
	}
}

func TestGetFileContentAndHandler(t *testing.T) {
	// Create temporary test artifact within the whitelisted brain directory
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	brainBase := filepath.Join(home, ".gemini", "antigravity", "brain")
	if err := os.MkdirAll(brainBase, 0755); err != nil {
		t.Fatal(err)
	}
	tmpDir, err := os.MkdirTemp(brainBase, "antigravity-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	testMD := filepath.Join(tmpDir, "implementation_plan.md")
	testMeta := filepath.Join(tmpDir, "implementation_plan.md.metadata.json")

	if err := os.WriteFile(testMD, []byte("# Test Plan\nHello world"), 0644); err != nil {
		t.Fatal(err)
	}
	metaData := `{"summary":"Test Plan Summary","requestFeedback":true,"userFacing":true}`
	if err := os.WriteFile(testMeta, []byte(metaData), 0644); err != nil {
		t.Fatal(err)
	}

	// Test GetFileContent
	res, err := GetFileContent(testMD, "")
	if err != nil {
		t.Fatalf("GetFileContent failed: %v", err)
	}
	if res.Filename != "implementation_plan.md" {
		t.Errorf("expected filename implementation_plan.md, got %s", res.Filename)
	}
	if res.Summary != "Test Plan Summary" {
		t.Errorf("expected summary 'Test Plan Summary', got %s", res.Summary)
	}
	if !res.RequestFeedback {
		t.Errorf("expected RequestFeedback true")
	}

	// Test HTTP Handler
	proxy := &Proxy{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/content?uri="+testMD, nil)
	rr := httptest.NewRecorder()
	proxy.HandleFileContent(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var jsonRes FileContentResult
	if err := json.Unmarshal(rr.Body.Bytes(), &jsonRes); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if jsonRes.Content != "# Test Plan\nHello world" {
		t.Errorf("expected content match, got %s", jsonRes.Content)
	}

	// Test HandleFileRaw
	rawReq := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+testMD, nil)
	rawRR := httptest.NewRecorder()
	proxy.HandleFileRaw(rawRR, rawReq)

	if rawRR.Code != http.StatusOK {
		t.Fatalf("expected 200 for raw file, got %d: %s", rawRR.Code, rawRR.Body.String())
	}
	if !strings.Contains(rawRR.Header().Get("Content-Disposition"), "implementation_plan.md") {
		t.Errorf("expected Content-Disposition header with filename, got: %s", rawRR.Header().Get("Content-Disposition"))
	}
	if rawRR.Body.String() != "# Test Plan\nHello world" {
		t.Errorf("expected raw body '# Test Plan\\nHello world', got %s", rawRR.Body.String())
	}
}

func TestHandleFileRawChineseFilename(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	// Use ~/Projects/ which is in the whitelist
	projectsBase := filepath.Join(home, "Projects")
	if err := os.MkdirAll(projectsBase, 0755); err != nil {
		t.Fatal(err)
	}
	tempDir, err := os.MkdirTemp(projectsBase, "antigravity-filetest-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	chineseFileName := "五矿证券转训课件测试.pptx"
	filePath := filepath.Join(tempDir, chineseFileName)
	content := []byte("PK\x03\x04test_zip_content")
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	proxy := &Proxy{}
	// Test with percent-encoded URI
	uri := "file://" + filepath.ToSlash(filepath.Join(tempDir, "五矿证券转训课件测试.pptx"))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+uri, nil)
	rr := httptest.NewRecorder()
	proxy.HandleFileRaw(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	disp := rr.Header().Get("Content-Disposition")
	if !strings.Contains(disp, "filename*=UTF-8''") {
		t.Errorf("expected RFC 5987 UTF-8 Content-Disposition header, got: %s", disp)
	}
	if rr.Body.String() != string(content) {
		t.Errorf("expected body to match written content")
	}
}

func TestIsSafeFilePath_SymlinkAndSensitiveFiles(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	// 1. Check direct sensitive files rejection
	sensitiveFiles := []string{
		filepath.Join(home, ".bash_history"),
		filepath.Join(home, ".zsh_history"),
		filepath.Join(home, ".git-credentials"),
		filepath.Join(home, ".netrc"),
		filepath.Join(home, ".docker", "config.json"),
		filepath.Join(home, ".config", "gcloud", "credentials.db"),
		filepath.Join(home, ".ssh", "id_ed25519"),
		"/etc/passwd",
		"/etc/shadow",
	}

	for _, sf := range sensitiveFiles {
		if IsSafeFilePath(sf) {
			t.Errorf("expected sensitive file to be rejected: %s", sf)
		}
	}

	// 2. Test Symlink escape attempt from inside whitelisted dir
	projectsBase := filepath.Join(home, "Projects")
	if err := os.MkdirAll(projectsBase, 0755); err == nil {
		tempDir, err := os.MkdirTemp(projectsBase, "symlink-test-*")
		if err == nil {
			defer os.RemoveAll(tempDir)
			targetFile := "/etc/hosts"
			linkPath := filepath.Join(tempDir, "evil_symlink_hosts")
			if os.Symlink(targetFile, linkPath) == nil {
				if IsSafeFilePath(linkPath) {
					t.Errorf("expected symlink pointing outside whitelist to be rejected: %s -> %s", linkPath, targetFile)
				}
			}
		}
	}
}

func TestJoinUnderRejectsDotDot(t *testing.T) {
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".gemini", "antigravity", "brain")
	if _, err := joinUnder(root, "../../../.ssh/id_rsa"); err == nil {
		t.Fatalf("expected joinUnder to reject .. escape")
	}
}

func TestAllowedWorkspaceRootsEnv(t *testing.T) {
	home, _ := os.UserHomeDir()
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", filepath.Join(home, "Downloads"))
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	if !IsSafeFilePath(filepath.Join(home, "Downloads/sample.png")) {
		t.Errorf("expected Downloads to be allowed when listed in ALLOWED_WORKSPACE_ROOTS")
	}
	if IsSafeFilePath(filepath.Join(home, "Movies/secret.mov")) {
		t.Errorf("expected Movies to remain blocked")
	}
}

func TestHandleFileRaw_JSONAndLog(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	projectsBase := filepath.Join(home, "Projects")
	tempDir, err := os.MkdirTemp(projectsBase, "antigravity-jsonlogtest-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// 1. Test JSON file
	jsonFile := filepath.Join(tempDir, "discovered_cubes_full.json")
	jsonContent := `[{"symbol":"ZH123","name":"Test Cube"}]`
	if err := os.WriteFile(jsonFile, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("failed to write json: %v", err)
	}

	proxy := &Proxy{}
	uriJSON := "file://" + filepath.ToSlash(jsonFile)
	reqJSON := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+uriJSON, nil)
	rrJSON := httptest.NewRecorder()
	proxy.HandleFileRaw(rrJSON, reqJSON)

	if rrJSON.Code != http.StatusOK {
		t.Fatalf("expected 200 for json download, got %d: %s", rrJSON.Code, rrJSON.Body.String())
	}
	if ct := rrJSON.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json, got: %s", ct)
	}
	disp := rrJSON.Header().Get("Content-Disposition")
	if !strings.Contains(disp, `filename="discovered_cubes_full.json"`) {
		t.Errorf("expected filename in Content-Disposition, got: %s", disp)
	}

	// 2. Test Log file
	logFile := filepath.Join(tempDir, "scanner.log")
	logContent := "2026-09-15 00:00:01 INFO: scan progress 92%"
	if err := os.WriteFile(logFile, []byte(logContent), 0644); err != nil {
		t.Fatalf("failed to write log: %v", err)
	}

	uriLog := "file://" + filepath.ToSlash(logFile)
	reqLog := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+uriLog, nil)
	rrLog := httptest.NewRecorder()
	proxy.HandleFileRaw(rrLog, reqLog)

	if rrLog.Code != http.StatusOK {
		t.Fatalf("expected 200 for log download, got %d: %s", rrLog.Code, rrLog.Body.String())
	}
	if ct := rrLog.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Errorf("expected text/plain, got: %s", ct)
	}
}
