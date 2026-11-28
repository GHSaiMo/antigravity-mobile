package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
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
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)
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

	if !IsSafeFilePath(filepath.Join(home, "Downloads/sample.png")) {
		t.Errorf("expected Downloads files to be safe by default")
	}

	if IsSafeFilePath(filepath.Join(home, "Downloads/.env")) {
		t.Errorf("expected Downloads .env to be blocked")
	}

	if !IsSafeFilePath(filepath.Join(home, ".agents/skills/xueqiu-radar/data/discovered_cubes_full.json")) {
		t.Errorf("expected .agents skill output to be safe")
	}
	if !IsSafeFilePath(filepath.Join(home, "Websites/Persia-Live/report.pdf")) {
		t.Errorf("expected home project documents to be safe")
	}
	if !IsSafeFilePath(filepath.Join(home, "Desktop/客户清单.xlsx")) {
		t.Errorf("expected Desktop documents to be safe")
	}
	if !IsSafeFilePath(filepath.Join(home, "Projects/demo/deck.key")) {
		t.Errorf("expected Keynote .key under Projects to be safe")
	}
	if IsSafeFilePath(filepath.Join(home, "Library/Application Support/Antigravity/User/globalStorage/storage.json")) {
		t.Errorf("expected Application Support to be blocked")
	}
	if !IsSafeFilePath(filepath.Join(home, ".gemini/config/skills/my-skill/SKILL.md")) {
		t.Errorf("expected .gemini/config skill file to be safe")
	}
	if !IsSafeFilePath(filepath.Join(home, "Projects/demo/readme.md")) {
		t.Errorf("expected Projects files to be safe")
	}

	// M-2: Check server.key, domain.key, and acme.sh
	if IsSafeFilePath(filepath.Join(home, "Projects/demo/server.key")) {
		t.Errorf("expected server.key to be rejected")
	}
	if IsSafeFilePath(filepath.Join(home, "Projects/demo/domain.key")) {
		t.Errorf("expected domain.key to be rejected")
	}
	if IsSafeFilePath(filepath.Join(home, "Projects/demo/tls.key")) {
		t.Errorf("expected tls.key to be rejected")
	}
	if IsSafeFilePath(filepath.Join(home, ".acme.sh/example.com/fullchain.cer")) {
		t.Errorf("expected .acme.sh directory to be rejected")
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
	if IsSafeFilePath(filepath.Join(home, ".multigravity/auth_store.json")) {
		t.Errorf("expected .multigravity/auth_store.json to be blocked")
	}
}

func TestResolveLocalFilePathUnsafePathsRejected(t *testing.T) {
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)
	home, _ := os.UserHomeDir()

	// M-1: ResolveLocalFilePath must reject /etc/passwd and ~/.ssh/id_rsa directly
	if _, err := ResolveLocalFilePath("/etc/passwd", ""); err == nil {
		t.Errorf("expected error resolving /etc/passwd")
	}
	if _, err := ResolveLocalFilePath(filepath.Join(home, ".ssh/id_rsa"), ""); err == nil {
		t.Errorf("expected error resolving ~/.ssh/id_rsa")
	}
	if _, err := ResolveLocalFilePath(filepath.Join(home, ".antigravity-mobile/auth_store.json"), ""); err == nil {
		t.Errorf("expected error resolving auth_store.json")
	}
	if _, err := ResolveLocalFilePath(filepath.Join(home, ".multigravity/auth_store.json"), ""); err == nil {
		t.Errorf("expected error resolving .multigravity/auth_store.json")
	}
}

func TestPrivateKeyHeaderDetection(t *testing.T) {
	// Create a temporary .key file containing private key banner
	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "presentation.key")
	if err := os.WriteFile(keyFile, []byte("-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA...\n"), 0600); err != nil {
		t.Fatalf("failed to write dummy key file: %v", err)
	}

	f, err := os.Open(keyFile)
	if err != nil {
		t.Fatalf("failed to open dummy key file: %v", err)
	}
	defer f.Close()

	if !isPrivateKeyFileHeader(f) {
		t.Errorf("expected isPrivateKeyFileHeader to detect PEM private key banner")
	}

	// Normal text / binary should not trigger
	normFile := filepath.Join(tmpDir, "normal.key")
	if err := os.WriteFile(normFile, []byte("Keynote dummy slide data"), 0600); err != nil {
		t.Fatalf("failed to write dummy file: %v", err)
	}
	fNorm, err := os.Open(normFile)
	if err != nil {
		t.Fatalf("failed to open dummy file: %v", err)
	}
	defer fNorm.Close()

	if isPrivateKeyFileHeader(fNorm) {
		t.Errorf("expected isPrivateKeyFileHeader to be false for non-key data")
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
	if rawRR.Header().Get("Cache-Control") != "private, max-age=604800, must-revalidate" {
		t.Errorf("expected Cache-Control header, got: %s", rawRR.Header().Get("Cache-Control"))
	}
	etag := rawRR.Header().Get("ETag")
	if etag == "" {
		t.Errorf("expected non-empty ETag header")
	}
	if rawRR.Body.String() != "# Test Plan\nHello world" {
		t.Errorf("expected raw body '# Test Plan\\nHello world', got %s", rawRR.Body.String())
	}

	// Test 304 Not Modified with If-None-Match
	req304 := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+testMD, nil)
	req304.Header.Set("If-None-Match", etag)
	rr304 := httptest.NewRecorder()
	proxy.HandleFileRaw(rr304, req304)
	if rr304.Code != http.StatusNotModified {
		t.Errorf("expected 304 Not Modified, got: %d", rr304.Code)
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

func TestIsSafeFilePath_CaseInsensitiveSensitiveDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, p := range []string{".aws/credentials", ".AWS/credentials", ".SSH/config", "Library/Application Support/x", "library/application support/x"} {
		if IsSafeFilePath(filepath.Join(home, p)) {
			t.Errorf("%s must be restricted", p)
		}
	}
}

func TestIsSafeFilePath_MoreCredentialStores(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)
	for _, p := range []string{".config/gh/hosts.yml", ".zshrc", ".pgpass", ".vault-token", ".azure/accessTokens.json", ".cargo/credentials.toml", ".mozilla/firefox/x/logins.json"} {
		if IsSafeFilePath(filepath.Join(home, p)) {
			t.Errorf("%s must be restricted", p)
		}
	}
	if !IsSafeFilePath(filepath.Join(home, "Desktop", "report.pdf")) {
		t.Error("ordinary Desktop file must stay readable")
	}
}

func TestGetFileContent_RejectsBinaryFiles(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	brainBase := filepath.Join(home, ".gemini", "antigravity", "brain")
	if err := os.MkdirAll(brainBase, 0755); err != nil {
		t.Fatal(err)
	}
	tmpDir, err := os.MkdirTemp(brainBase, "antigravity-bin-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	jpgFile := filepath.Join(tmpDir, "sample.jpg")
	// Write dummy JPEG magic bytes
	if err := os.WriteFile(jpgFile, []byte("\xFF\xD8\xFF\xE0\x00\x10JFIF\x00\x01\x01"), 0644); err != nil {
		t.Fatal(err)
	}

	// 1. GetFileContent must reject .jpg
	_, err = GetFileContent(jpgFile, "")
	if err == nil {
		t.Fatalf("expected GetFileContent to reject binary .jpg file, got nil error")
	}
	if !strings.Contains(err.Error(), "binary file") {
		t.Errorf("expected error mentioning binary file, got: %v", err)
	}

	// 2. HandleFileContent HTTP handler must return 404/error instead of 200
	proxy := &Proxy{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/content?uri="+jpgFile, nil)
	rr := httptest.NewRecorder()
	proxy.HandleFileContent(rr, req)
	if rr.Code == http.StatusOK {
		t.Fatalf("expected HTTP error for binary file content request, got 200: %s", rr.Body.String())
	}

	// 3. HandleFileRaw must still succeed and stream the image
	rawReq := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+jpgFile, nil)
	rawRR := httptest.NewRecorder()
	proxy.HandleFileRaw(rawRR, rawReq)
	if rawRR.Code != http.StatusOK {
		t.Fatalf("expected 200 for HandleFileRaw on .jpg, got %d", rawRR.Code)
	}
	if ct := rawRR.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("expected Content-Type image/jpeg, got: %s", ct)
	}
}

func TestIsSafeFilePath_CockpitAndToolCredentialsAndXmlSandbox(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	blocked := []string{
		".antigravity_cockpit/secure-account-storage.key",
		".cockpit-tools/accounts.json",
		".hal9000/cf_tunnel.json",
		".claude/credentials.json",
		".codex/auth.json",
		".cursor/state.vscdb",
		".gemini/antigravity/mcp_config.json",
		".gemini/settings.json",
		".gemini/config/config.json",
		".gitconfig",
		"Library/Cookies/Cookies.binarycookies",
		"Library/Safari/History.db",
		"Library/Messages/chat.db",
		"Desktop/release.keystore",
		"Desktop/admin_token",
		"Desktop/private.key",
	}
	for _, rel := range blocked {
		if IsSafeFilePath(filepath.Join(home, rel)) {
			t.Errorf("expected %s to be blocked by IsSafeFilePath", rel)
		}
	}

	// Ordinary presentation .key file without sensitive keywords must remain allowed.
	if !IsSafeFilePath(filepath.Join(home, "Desktop", "deck.key")) {
		t.Error("expected ordinary presentation Desktop/deck.key to remain allowed")
	}

	// Verify HandleFileRaw forces attachment + sandbox CSP on .xml files (SEC-04).
	desktopDir := filepath.Join(home, "Desktop")
	if err := os.MkdirAll(desktopDir, 0755); err != nil {
		t.Fatal(err)
	}
	xmlFile := filepath.Join(desktopDir, "payload.xml")
	if err := os.WriteFile(xmlFile, []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`), 0644); err != nil {
		t.Fatal(err)
	}

	p := &Proxy{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+xmlFile, nil)
	rr := httptest.NewRecorder()
	p.HandleFileRaw(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for HandleFileRaw on .xml, got %d", rr.Code)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") {
		t.Errorf("expected Content-Disposition attachment for .xml, got %q", cd)
	}
	if csp := rr.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") {
		t.Errorf("expected Content-Security-Policy containing 'sandbox' for .xml, got %q", csp)
	}
}

func TestTempDirReadableFilesAndWorkspaceConfinement(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", "")
	t.Setenv("MULTIGRAVITY_ALLOWED_WORKSPACE_ROOTS", "")
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	// Prepare temp directories to test: os.TempDir() (via t.TempDir()) and /tmp on Unix.
	tmpDirs := []string{t.TempDir()}
	if runtime.GOOS != "windows" {
		slashTmpDir, err := os.MkdirTemp("/tmp", "mgy-tmp-readable-*")
		if err != nil {
			t.Fatalf("failed to create dir in /tmp: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(slashTmpDir) })
		tmpDirs = append(tmpDirs, slashTmpDir)
	}

	p := &Proxy{}
	videoBytes := []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom")

	for _, dir := range tmpDirs {
		mp4Path := filepath.Join(dir, "minimax_h3_preview.mp4")
		if err := os.WriteFile(mp4Path, videoBytes, 0o644); err != nil {
			t.Fatalf("failed to write temp mp4: %v", err)
		}

		// Temp dirs remain outside workspace roots (validateWorkspaceDir / IsSafeFilePath must reject them).
		if IsSafeFilePath(mp4Path) {
			t.Errorf("expected temp file %s to remain outside workspace roots", mp4Path)
		}
		if _, err := validateWorkspaceDir(dir, true); err == nil {
			t.Errorf("expected validateWorkspaceDir to reject temp dir %s", dir)
		}

		// Read-only file preview/download must allow regular non-sensitive files in temp dirs.
		if !IsSafeReadableFilePath(mp4Path) {
			t.Fatalf("expected temp video %s to be readable", mp4Path)
		}

		for _, rawURI := range []string{mp4Path, "file://" + filepath.ToSlash(mp4Path)} {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(rawURI), nil)
			rr := httptest.NewRecorder()
			p.HandleFileRaw(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200 downloading %s, got %d: %s", rawURI, rr.Code, rr.Body.String())
			}
			if ct := rr.Header().Get("Content-Type"); ct != "video/mp4" {
				t.Errorf("expected Content-Type video/mp4 for %s, got %q", rawURI, ct)
			}
			if rr.Body.String() != string(videoBytes) {
				t.Errorf("downloaded bytes mismatch for %s", rawURI)
			}
		}

		// Verify HTTP Range requests work cleanly for video streaming.
		rangeReq := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(mp4Path), nil)
		rangeReq.Header.Set("Range", "bytes=4-11")
		rangeRR := httptest.NewRecorder()
		p.HandleFileRaw(rangeRR, rangeReq)
		if rangeRR.Code != http.StatusPartialContent {
			t.Fatalf("expected 206 Partial Content for range request on %s, got %d", mp4Path, rangeRR.Code)
		}
		if rangeRR.Body.String() != "ftypmp42" {
			t.Errorf("expected range body ftypmp42, got %q", rangeRR.Body.String())
		}

		// Text content preview should also work for non-sensitive files in temp dir.
		txtPath := filepath.Join(dir, "notes.txt")
		if err := os.WriteFile(txtPath, []byte("verify ok"), 0o644); err != nil {
			t.Fatal(err)
		}
		res, err := GetFileContent(txtPath, "")
		if err != nil {
			t.Fatalf("expected GetFileContent to allow temp text file %s: %v", txtPath, err)
		}
		if res.Content != "verify ok" {
			t.Errorf("unexpected GetFileContent body: %q", res.Content)
		}

		// Sensitive filenames inside temp dir must still be rejected.
		for _, badName := range []string{".env", "server.key", "id_rsa", "secrets.json", "cert.pem"} {
			badPath := filepath.Join(dir, badName)
			if err := os.WriteFile(badPath, []byte("secret"), 0o600); err != nil {
				t.Fatal(err)
			}
			if IsSafeReadableFilePath(badPath) {
				t.Errorf("expected sensitive temp file %s to be rejected", badPath)
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(badPath), nil)
			rr := httptest.NewRecorder()
			p.HandleFileRaw(rr, req)
			if rr.Code != http.StatusForbidden {
				t.Errorf("expected 403 for sensitive temp file %s, got %d", badPath, rr.Code)
			}
		}

		// Symlink inside temp dir pointing outside allowed roots must be rejected.
		if runtime.GOOS != "windows" {
			linkPath := filepath.Join(dir, "escape.mp4")
			if err := os.Symlink("/etc/hosts", linkPath); err == nil {
				if IsSafeReadableFilePath(linkPath) {
					t.Errorf("expected temp symlink %s -> /etc/hosts to be rejected", linkPath)
				}
				req := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(linkPath), nil)
				rr := httptest.NewRecorder()
				p.HandleFileRaw(rr, req)
				if rr.Code != http.StatusForbidden {
					t.Errorf("expected 403 for temp symlink escape %s, got %d", linkPath, rr.Code)
				}
			}
		}
	}

	// Explicit ALLOWED_WORKSPACE_ROOTS must override default temp dir access.
	downloadsDir := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(downloadsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", downloadsDir)
	resetWorkspaceRootsForTest()

	mp4InTmp := filepath.Join(tmpDirs[0], "minimax_h3_preview.mp4")
	if IsSafeReadableFilePath(mp4InTmp) {
		t.Errorf("expected temp file %s to be blocked when ALLOWED_WORKSPACE_ROOTS is explicitly set", mp4InTmp)
	}
	reqBlocked := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(mp4InTmp), nil)
	rrBlocked := httptest.NewRecorder()
	p.HandleFileRaw(rrBlocked, reqBlocked)
	if rrBlocked.Code != http.StatusForbidden {
		t.Errorf("expected 403 when ALLOWED_WORKSPACE_ROOTS excludes temp dir, got %d", rrBlocked.Code)
	}
}

func TestResolveLocalFilePathFragmentsQueriesAndLineLocators(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", "")
	t.Setenv("MULTIGRAVITY_ALLOWED_WORKSPACE_ROOTS", "")
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	docsDir := filepath.Join(home, "Projects", "demo", "private-docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	guidePath := filepath.Join(docsDir, "relay_pool_and_e2ee_ops_guide.md")
	const guideBody = "# Relay Pool & E2EE Ops Guide\nSection 9\n"
	if err := os.WriteFile(guidePath, []byte(guideBody), 0o644); err != nil {
		t.Fatal(err)
	}

	hashFilePath := filepath.Join(docsDir, "Report #1.md")
	const hashBody = "# Report #1\n"
	if err := os.WriteFile(hashFilePath, []byte(hashBody), 0o644); err != nil {
		t.Fatal(err)
	}

	envPath := filepath.Join(docsDir, ".env")
	if err := os.WriteFile(envPath, []byte("SECRET=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := &Proxy{}

	// 1. Standard file:// URI with GitHub-style line range fragment (#L748-L779), query, and :line locators
	guideCases := []string{
		"file://" + filepath.ToSlash(guidePath) + "#L748-L779",
		"file://" + filepath.ToSlash(guidePath) + "#section-9",
		"file://" + filepath.ToSlash(guidePath) + "?v=1#L748-L779",
		"file://" + filepath.ToSlash(guidePath) + "%23L748-L779",
		guidePath + "#L748-L779",
		guidePath + ":748",
		guidePath + ":748:12",
		guidePath + ":L748-L779",
	}
	for _, rawURI := range guideCases {
		got, err := ResolveLocalFilePath(rawURI, "test-cascade")
		if err != nil {
			t.Fatalf("ResolveLocalFilePath(%q) failed: %v", rawURI, err)
		}
		if got != guidePath {
			t.Errorf("ResolveLocalFilePath(%q) = %q, want %q", rawURI, got, guidePath)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/files/content?uri="+url.QueryEscape(rawURI)+"&cascade_id=test-cascade", nil)
		rr := httptest.NewRecorder()
		p.HandleFileContent(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("HandleFileContent(%q) returned %d: %s", rawURI, rr.Code, rr.Body.String())
		}
		var res FileContentResult
		if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
			t.Fatalf("HandleFileContent(%q) invalid json: %v", rawURI, err)
		}
		if res.Content != guideBody {
			t.Errorf("HandleFileContent(%q) content = %q, want %q", rawURI, res.Content, guideBody)
		}
		if res.Filename != "relay_pool_and_e2ee_ops_guide.md" {
			t.Errorf("HandleFileContent(%q) filename = %q", rawURI, res.Filename)
		}
	}

	// 2. Real filenames containing '#' (percent-encoded or pre-unescaped, with or without #L fragment)
	hashCases := []string{
		"file://" + filepath.ToSlash(filepath.Join(docsDir, "Report%20%231.md")),
		"file://" + filepath.ToSlash(filepath.Join(docsDir, "Report%20%231.md")) + "#L10-L20",
		"file://" + filepath.ToSlash(hashFilePath),
		"file://" + filepath.ToSlash(hashFilePath) + "#L10-L20",
		hashFilePath,
		hashFilePath + "#L10-L20",
	}
	for _, rawURI := range hashCases {
		got, err := ResolveLocalFilePath(rawURI, "")
		if err != nil {
			t.Fatalf("ResolveLocalFilePath(%q) failed: %v", rawURI, err)
		}
		if got != hashFilePath {
			t.Errorf("ResolveLocalFilePath(%q) = %q, want %q", rawURI, got, hashFilePath)
		}
		res, err := GetFileContent(rawURI, "")
		if err != nil {
			t.Fatalf("GetFileContent(%q) failed: %v", rawURI, err)
		}
		if res.Content != hashBody {
			t.Errorf("GetFileContent(%q) content = %q, want %q", rawURI, res.Content, hashBody)
		}
	}

	// 3. Static artifact & bare filename with cascadeID + fragment
	gotStatic, err := ResolveLocalFilePath("/static/artifacts/casc-99/plan.md#L1-L20", "")
	if err != nil {
		t.Fatalf("static artifact with fragment failed: %v", err)
	}
	wantStatic := filepath.Join(home, ".gemini", "antigravity", "brain", "casc-99", "plan.md")
	if gotStatic != wantStatic {
		t.Errorf("got %q, want %q", gotStatic, wantStatic)
	}

	gotBare, err := ResolveLocalFilePath("implementation_plan.md#L5-L12", "casc-99")
	if err != nil {
		t.Fatalf("bare filename with fragment failed: %v", err)
	}
	wantBare := filepath.Join(home, ".gemini", "antigravity", "brain", "casc-99", "implementation_plan.md")
	if gotBare != wantBare {
		t.Errorf("got %q, want %q", gotBare, wantBare)
	}

	// 4. Security: fragments must never bypass sensitive file restrictions
	unsafeCases := []string{
		"file://" + filepath.ToSlash(envPath) + "#L1-L5",
		"file://" + filepath.ToSlash(envPath) + "#safe.md",
		"file:///etc/passwd#L1-L5",
		"file://" + filepath.ToSlash(filepath.Join(home, ".ssh", "id_rsa")) + "#notes.md",
	}
	for _, rawURI := range unsafeCases {
		if got, err := ResolveLocalFilePath(rawURI, ""); err == nil {
			t.Errorf("expected ResolveLocalFilePath(%q) to reject sensitive path, got %q", rawURI, got)
		}
	}
}

func TestHasPathPrefixForOS_CaseSensitivity(t *testing.T) {
	cases := []struct {
		name   string
		goos   string
		path   string
		prefix string
		sep    string
		want   bool
	}{
		{"windows_drive_case_diff", "windows", `c:\users\alice\yiju_9000`, `C:\Users\Alice\`, `\`, true},
		{"windows_exact_dir_case_diff", "windows", `d:\yiju_9000`, `D:\Yiju_9000\`, `\`, true},
		{"windows_sibling_prefix_rejected", "windows", `d:\yiju_9000_evil`, `D:\yiju_9000\`, `\`, false},
		{"darwin_case_diff", "darwin", "/Users/Hal9000/yiju_9000", "/Users/hal9000/", "/", true},
		{"darwin_sibling_prefix_rejected", "darwin", "/Volumes/SSD/yiju_9000_other", "/Volumes/SSD/yiju_9000/", "/", false},
		{"linux_case_sensitive_reject", "linux", "/home/Alice/yiju_9000", "/home/alice/", "/", false},
		{"linux_exact_match", "linux", "/data/yiju_9000", "/data/yiju_9000/", "/", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasPathPrefixForOS(tc.goos, tc.path, tc.prefix, tc.sep); got != tc.want {
				t.Errorf("hasPathPrefixForOS(%q, %q, %q, %q) = %v, want %v", tc.goos, tc.path, tc.prefix, tc.sep, got, tc.want)
			}
		})
	}
}

func TestIsBlockedAutoWorkspaceRoot(t *testing.T) {
	blocked := []string{
		"/",
		"/etc",
		"/etc/nginx",
		"/private/etc",
		"/System/Library",
		"/proc",
		"/sys",
		"/dev",
		"/tmp",
		"/var",
		"/Users",
		"/Volumes",
		os.TempDir(),
		"C:\\",
		"D:\\",
		"C:\\Windows",
		"C:\\Windows\\System32",
		"C:\\Users",
		"/Users/alice/.ssh",
		"/Users/alice/.hal9000/yiju_9000",
	}
	for _, p := range blocked {
		if !isBlockedAutoWorkspaceRoot(p) {
			t.Errorf("expected %q to be blocked as auto workspace root", p)
		}
	}

	allowed := []string{
		"/Volumes/ExternalSSD/yiju_9000",
		"/data/yiju_9000",
		"/opt/apps/yiju_9000",
		"D:\\yiju_9000",
		"E:\\Projects\\yiju_9000",
	}
	for _, p := range allowed {
		if isBlockedAutoWorkspaceRoot(p) {
			t.Errorf("expected %q to be allowed as auto workspace root", p)
		}
	}
}

func TestFilePathToURI_WindowsAndUNC(t *testing.T) {
	if got := filePathToURI("D:/Projects/yiju_9000"); got != "file:///D:/Projects/yiju_9000" {
		t.Errorf("got %q, want file:///D:/Projects/yiju_9000", got)
	}
	if got := filePathToURI("//wsl.localhost/Ubuntu/home/alice/yiju_9000"); got != "file://wsl.localhost/Ubuntu/home/alice/yiju_9000" {
		t.Errorf("got %q, want file://wsl.localhost/Ubuntu/home/alice/yiju_9000", got)
	}
}

func TestAutoRegisteredWorkspaceRoots_GeminiConfigAndWorkspaceStorage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", "")
	t.Setenv("ANTIGRAVITY_APP_DATA_DIR", "")
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", "")
	t.Setenv("MULTIGRAVITY_ALLOWED_WORKSPACE_ROOTS", "")
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	// External directories completely outside $HOME (simulates D:\, /Volumes/SSD, /data)
	extRoot := t.TempDir()
	projA := filepath.Join(extRoot, "yiju_9000")
	projA2 := filepath.Join(extRoot, "yiju_9000_sub")
	projB := filepath.Join(extRoot, "legacy_ws_proj")
	projSymTarget := filepath.Join(extRoot, "symlink_target_proj")
	unregistered := filepath.Join(extRoot, "unregistered_secret")
	for _, d := range []string{projA, projA2, projB, projSymTarget, unregistered} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	readmeA := filepath.Join(projA, "README.md")
	envA := filepath.Join(projA, ".env")
	if err := os.WriteFile(readmeA, []byte("# yiju_9000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envA, []byte("SECRET=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Register multi-folder project (projA + projA2) and a malicious /etc entry in ~/.gemini/config/projects/
	cfgDir := filepath.Join(home, ".gemini", "config", "projects")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	projAJSON := `{
		"id": "yiju-project-id",
		"name": "yiju_9000",
		"projectResources": {
			"resources": [
				{"folderUri": "` + filePathToURI(projA) + `"},
				{"gitFolder": {"folderUri": "` + filePathToURI(projA2) + `"}}
			]
		}
	}`
	if err := os.WriteFile(filepath.Join(cfgDir, "yiju-project-id.json"), []byte(projAJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "bad-etc.json"), []byte(`{"id":"bad-etc","name":"etc","projectResources":{"resources":[{"folderUri":"file:///etc"}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Register symlink project inside $HOME pointing to external volume
	var linkedProj string
	if runtime.GOOS != "windows" {
		linkedProj = filepath.Join(home, "Projects", "linked-yiju")
		if err := os.MkdirAll(filepath.Dir(linkedProj), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(projSymTarget, linkedProj); err != nil {
			t.Fatal(err)
		}
		symJSON := `{"id":"sym-id","name":"linked-yiju","projectResources":{"resources":[{"folderUri":"` + filePathToURI(linkedProj) + `"}]}}`
		if err := os.WriteFile(filepath.Join(cfgDir, "sym-id.json"), []byte(symJSON), 0o644); err != nil {
			t.Fatal(err)
		}
		// Symlink inside projA escaping to unregistered directory must still be blocked
		if err := os.Symlink(unregistered, filepath.Join(projA, "escape_link")); err != nil {
			t.Fatal(err)
		}
	}

	// Register projB via workspaceStorage/*/workspace.json
	wsStorageDir := filepath.Join(home, ".config", "Antigravity", "User", "workspaceStorage", "hash-proj-b")
	if err := os.MkdirAll(wsStorageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsStorageDir, "workspace.json"), []byte(`{"folder":"`+filePathToURI(projB)+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	resetWorkspaceRootsForTest()

	// 1. Registered external projects must pass validateWorkspaceDir & file checks
	for _, d := range []string{projA, projA2, projB} {
		if _, err := validateWorkspaceDir(d, true); err != nil {
			t.Errorf("expected registered external project %q to pass validateWorkspaceDir, got: %v", d, err)
		}
		if _, err := validateWorkspaceDir(filePathToURI(d), true); err != nil {
			t.Errorf("expected registered external project URI %q to pass validateWorkspaceDir, got: %v", filePathToURI(d), err)
		}
	}
	if linkedProj != "" {
		if _, err := validateWorkspaceDir(linkedProj, true); err != nil {
			t.Errorf("expected registered symlinked project %q to pass validateWorkspaceDir, got: %v", linkedProj, err)
		}
	}

	// Regular files inside registered external project must be readable, while secrets remain blocked
	if !IsSafeFilePath(readmeA) || !IsSafeReadableFilePath(readmeA) {
		t.Errorf("expected regular file %q in registered external project to be allowed", readmeA)
	}
	if IsSafeFilePath(envA) || IsSafeReadableFilePath(envA) {
		t.Errorf("expected sensitive file %q in registered external project to stay blocked", envA)
	}
	if runtime.GOOS != "windows" {
		if IsSafeFilePath(filepath.Join(projA, "escape_link")) {
			t.Errorf("expected symlink inside registered project escaping to unregistered dir to be blocked")
		}
	}

	// 2. Unregistered external directory and blocked system roots must remain rejected
	if _, err := validateWorkspaceDir(unregistered, true); err == nil {
		t.Errorf("expected unregistered external dir %q to be rejected", unregistered)
	}
	if runtime.GOOS != "windows" {
		if _, err := validateWorkspaceDir("/etc", true); err == nil {
			t.Errorf("expected /etc to remain rejected even if listed in project config")
		}
	}

	// 3. Explicit ALLOWED_WORKSPACE_ROOTS must still strictly override auto-registered projects
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", filepath.Join(home, "OnlyHere"))
	resetWorkspaceRootsForTest()
	if _, err := validateWorkspaceDir(projA, true); err == nil {
		t.Errorf("expected explicit ALLOWED_WORKSPACE_ROOTS to override auto-registered external project %q", projA)
	}
}

func TestHandleFileContentAndRaw_CascadeRelativeAndRepoRootPaths(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", "")
	t.Setenv("ANTIGRAVITY_APP_DATA_DIR", "")
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", "")
	t.Setenv("MULTIGRAVITY_ALLOWED_WORKSPACE_ROOTS", "")
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	// Set up two distinct repos: one as process CWD (must NEVER be read for another chat's workspace)
	// and one as the conversation's actual workspace.
	cwdRepo := filepath.Join(home, "Projects", "wrong-cwd-repo")
	chatRepo := filepath.Join(home, "Projects", "my-app")
	docsDir := filepath.Join(chatRepo, "docs")
	for _, d := range []string{cwdRepo, docsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(cwdRepo, "AGENTS.md"), []byte("# WRONG CWD AGENTS\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const wantAgents = "# RIGHT CHAT AGENTS\n"
	const wantGuide = "# Guide\nSection 1\n"
	const wantBrainPlan = "# Brain Implementation Plan\n"
	if err := os.WriteFile(filepath.Join(chatRepo, "AGENTS.md"), []byte(wantAgents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "guide.md"), []byte(wantGuide), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chatRepo, ".env"), []byte("SECRET=topsecret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chatRepo, "implementation_plan.md"), []byte("# Repo Plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink("/etc/hosts", filepath.Join(chatRepo, "escape_hosts.md")); err != nil {
			t.Fatal(err)
		}
	}

	const cascadeID = "11111111-2222-3333-4444-555555555555"
	const metaOnlyCascadeID = "66666666-7777-8888-9999-aaaaaaaaaaaa"
	const noWorkspaceCascadeID = "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"

	brainDir := filepath.Join(home, ".gemini", "antigravity", "brain", cascadeID)
	if err := os.MkdirAll(brainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(brainDir, "implementation_plan.md"), []byte(wantBrainPlan), 0o644); err != nil {
		t.Fatal(err)
	}

	// Switch CWD to cwdRepo to prove conversation relative paths ignore gateway CWD.
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwdRepo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })

	chatRepoURI := filePathToURI(chatRepo)
	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"trajectorySummaries": map[string]interface{}{
				cascadeID: map[string]interface{}{
					"workspaces": []map[string]interface{}{{"workspaceFolderAbsoluteUri": chatRepoURI}},
				},
				metaOnlyCascadeID: map[string]interface{}{
					"trajectoryMetadata": map[string]interface{}{
						"workspaceUris": []string{chatRepoURI},
					},
				},
				noWorkspaceCascadeID: map[string]interface{}{},
			},
		})
	})

	// 1. All relative, bare, repo-root, file://, and fragment variants resolve to chatRepo (NOT cwdRepo).
	okCases := []struct {
		uri      string
		cid      string
		wantBody string
		wantName string
	}{
		{"AGENTS.md", cascadeID, wantAgents, "AGENTS.md"},
		{"AGENTS.md#L1-L5", cascadeID, wantAgents, "AGENTS.md"},
		{"AGENTS.md:1", cascadeID, wantAgents, "AGENTS.md"},
		{"./AGENTS.md", cascadeID, wantAgents, "AGENTS.md"},
		{"./AGENTS.md#L1-L5", cascadeID, wantAgents, "AGENTS.md"},
		{"/AGENTS.md", cascadeID, wantAgents, "AGENTS.md"},
		{"file:///AGENTS.md", cascadeID, wantAgents, "AGENTS.md"},
		{"file://AGENTS.md", cascadeID, wantAgents, "AGENTS.md"},
		{"file://./AGENTS.md", cascadeID, wantAgents, "AGENTS.md"},
		{"my-app/AGENTS.md", cascadeID, wantAgents, "AGENTS.md"},
		{"docs/guide.md", cascadeID, wantGuide, "guide.md"},
		{"./docs/guide.md#section-1", cascadeID, wantGuide, "guide.md"},
		{"/docs/guide.md#L1-L2", cascadeID, wantGuide, "guide.md"},
		{"file:///docs/guide.md", cascadeID, wantGuide, "guide.md"},
		// trajectoryMetadata.workspaceUris fallback works even when workspaces[] is omitted.
		{"./AGENTS.md", metaOnlyCascadeID, wantAgents, "AGENTS.md"},
		// Brain artifact still takes priority over same-named file in workspace.
		{"implementation_plan.md", cascadeID, wantBrainPlan, "implementation_plan.md"},
	}

	for _, tc := range okCases {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/files/content?uri="+url.QueryEscape(tc.uri)+"&cascade_id="+tc.cid, nil)
		rr := httptest.NewRecorder()
		p.HandleFileContent(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("HandleFileContent(%q, %s) status=%d body=%s", tc.uri, tc.cid, rr.Code, rr.Body.String())
		}
		var res FileContentResult
		if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
			t.Fatalf("HandleFileContent(%q) invalid json: %v", tc.uri, err)
		}
		if res.Content != tc.wantBody || res.Filename != tc.wantName {
			t.Errorf("HandleFileContent(%q) = (%q, %q), want (%q, %q)", tc.uri, res.Filename, res.Content, tc.wantName, tc.wantBody)
		}

		rawReq := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(tc.uri)+"&cascade_id="+tc.cid, nil)
		rawRR := httptest.NewRecorder()
		p.HandleFileRaw(rawRR, rawReq)
		if rawRR.Code != http.StatusOK {
			t.Fatalf("HandleFileRaw(%q, %s) status=%d body=%s", tc.uri, tc.cid, rawRR.Code, rawRR.Body.String())
		}
		if rawRR.Body.String() != tc.wantBody {
			t.Errorf("HandleFileRaw(%q) body=%q, want %q", tc.uri, rawRR.Body.String(), tc.wantBody)
		}
	}

	// 2. Non-existent benign relative files return 404 (not 403), even when CWD is "/" on Unix.
	if runtime.GOOS != "windows" {
		if err := os.Chdir("/"); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		uri string
		cid string
	}{
		{"./missing.md", cascadeID},
		{"./AGENTS.md", noWorkspaceCascadeID},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/files/content?uri="+url.QueryEscape(tc.uri)+"&cascade_id="+tc.cid, nil)
		rr := httptest.NewRecorder()
		p.HandleFileContent(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Errorf("HandleFileContent(%q, %s) expected 404 not found, got %d: %s", tc.uri, tc.cid, rr.Code, rr.Body.String())
		}
		if strings.Contains(rr.Body.String(), "access to file is restricted") {
			t.Errorf("HandleFileContent(%q, %s) should not report restricted for missing benign file: %s", tc.uri, tc.cid, rr.Body.String())
		}
	}

	// 3. Security: traversal, sensitive files, symlinks escaping workspace, and system paths must return 403.
	forbiddenURIs := []string{
		"../../etc/passwd",
		"../wrong-cwd-repo/AGENTS.md",
		".env",
		"./.env",
		"/.env",
		"file:///.env",
		"./.env.local",
		"/etc/passwd",
		"file:///etc/passwd",
		filepath.Join(home, ".codex", "AGENTS.md"),
	}
	if runtime.GOOS != "windows" {
		forbiddenURIs = append(forbiddenURIs, "./escape_hosts.md", "escape_hosts.md", "/escape_hosts.md")
	}
	for _, badURI := range forbiddenURIs {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/files/content?uri="+url.QueryEscape(badURI)+"&cascade_id="+cascadeID, nil)
		rr := httptest.NewRecorder()
		p.HandleFileContent(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("HandleFileContent(%q) expected 403 Forbidden, got %d: %s", badURI, rr.Code, rr.Body.String())
		}

		rawReq := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(badURI)+"&cascade_id="+cascadeID, nil)
		rawRR := httptest.NewRecorder()
		p.HandleFileRaw(rawRR, rawReq)
		if rawRR.Code != http.StatusForbidden {
			t.Errorf("HandleFileRaw(%q) expected 403 Forbidden, got %d: %s", badURI, rawRR.Code, rawRR.Body.String())
		}
	}

	// 4. External workspace outside $HOME (e.g. D:\ or /Volumes) bound to a cascade also resolves relative & absolute links.
	extDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	extRepo := filepath.Join(extDir, "ext-app")
	if err := os.MkdirAll(extRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	const wantExtAgents = "# EXTERNAL AGENTS\n"
	extAgentsPath := filepath.Join(extRepo, "AGENTS.md")
	if err := os.WriteFile(extAgentsPath, []byte(wantExtAgents), 0o644); err != nil {
		t.Fatal(err)
	}
	const extCascadeID = "77777777-8888-9999-aaaa-bbbbbbbbbbbb"
	extProxy := fakeLSWithWorkspace(t, extCascadeID, filePathToURI(extRepo))
	for _, u := range []string{"AGENTS.md", "./AGENTS.md", "/AGENTS.md", filePathToURI(extAgentsPath)} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/files/content?uri="+url.QueryEscape(u)+"&cascade_id="+extCascadeID, nil)
		rr := httptest.NewRecorder()
		extProxy.HandleFileContent(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("external workspace HandleFileContent(%q) status=%d body=%s", u, rr.Code, rr.Body.String())
		}
		var res FileContentResult
		if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
			t.Fatalf("external workspace invalid json: %v", err)
		}
		if res.Content != wantExtAgents {
			t.Errorf("external workspace HandleFileContent(%q) content=%q, want %q", u, res.Content, wantExtAgents)
		}
	}

	// 5. Explicit ALLOWED_WORKSPACE_ROOTS must still block workspaces outside it.
	onlyDir := filepath.Join(home, "OnlyHere")
	if err := os.MkdirAll(onlyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", onlyDir)
	resetWorkspaceRootsForTest()

	reqBlocked := httptest.NewRequest(http.MethodGet, "/api/v1/files/content?uri="+url.QueryEscape("/AGENTS.md")+"&cascade_id="+cascadeID, nil)
	rrBlocked := httptest.NewRecorder()
	p.HandleFileContent(rrBlocked, reqBlocked)
	if rrBlocked.Code != http.StatusForbidden {
		t.Errorf("expected 403 when ALLOWED_WORKSPACE_ROOTS excludes cascade workspace, got %d: %s", rrBlocked.Code, rrBlocked.Body.String())
	}
}

func TestParseLocalWorkspaceURIs(t *testing.T) {
	got := parseLocalWorkspaceURIs(`["file:///a/b", " file:///c/d "]`)
	if len(got) != 2 || got[0] != "file:///a/b" || got[1] != "file:///c/d" {
		t.Fatalf("unexpected JSON array parse: %#v", got)
	}
	gotSingle := parseLocalWorkspaceURIs(`"file:///only/one"`)
	if len(gotSingle) != 1 || gotSingle[0] != "file:///only/one" {
		t.Fatalf("unexpected JSON string parse: %#v", gotSingle)
	}
	gotPlain := parseLocalWorkspaceURIs("file:///plain/path")
	if len(gotPlain) != 1 || gotPlain[0] != "file:///plain/path" {
		t.Fatalf("unexpected plain parse: %#v", gotPlain)
	}
	if empty := parseLocalWorkspaceURIs("   "); len(empty) != 0 {
		t.Fatalf("expected empty slice, got %#v", empty)
	}
}
