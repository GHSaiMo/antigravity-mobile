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
}

func TestGetFileContentAndHandler(t *testing.T) {
	// Create temporary test artifact with metadata
	tmpDir, err := os.MkdirTemp("", "antigravity-test-*")
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
	tempDir := t.TempDir()
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


