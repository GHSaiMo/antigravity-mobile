package inspector

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestRegexParsing(t *testing.T) {
	samplePs := `89479 /Applications/Antigravity.app/Contents/Resources/bin/language_server --standalone --override_ide_name antigravity --csrf_token 5aacbcc0-cf40-4b94-8f17-986485a8d0d5 --app_data_dir antigravity`
	
	matches := csrfRegex.FindStringSubmatch(samplePs)
	if len(matches) < 2 {
		t.Fatalf("expected csrf token match, got none")
	}
	expected := "5aacbcc0-cf40-4b94-8f17-986485a8d0d5"
	if matches[1] != expected {
		t.Errorf("expected %s, got %s", expected, matches[1])
	}

	sampleLsof := `COMMAND     PID    USER   FD   TYPE             DEVICE SIZE/OFF NODE NAME
language_ 89479 user        7u  IPv4 0xe146a107dbd3a758      0t0  TCP 127.0.0.1:62226 (LISTEN)
language_ 89479 user        8u  IPv4 0x43b49ab6df4a4122      0t0  TCP 127.0.0.1:62227 (LISTEN)
`
	portMatches := lsofRegex.FindAllStringSubmatch(sampleLsof, -1)
	if len(portMatches) != 2 {
		t.Fatalf("expected 2 port matches, got %d", len(portMatches))
	}
	if portMatches[0][1] != "62226" {
		t.Errorf("expected port 62226, got %s", portMatches[0][1])
	}
	if portMatches[1][1] != "62227" {
		t.Errorf("expected port 62227, got %s", portMatches[1][1])
	}
}

func TestLiveScan(t *testing.T) {
	insp := NewInspector(10 * time.Second)
	info := insp.Scan()
	if info == nil {
		t.Skip("Antigravity language_server not running, skipping live scan test")
	}
	if info.Port == 0 {
		t.Fatalf("invalid scanned info: %+v", info)
	}
	t.Logf("Live discovery succeeded: Port=%d, CSRF=%s, PID=%d", info.Port, info.CSRFToken, info.PID)
}

func TestDaemonDiscovery(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/exa.language_server_pb.LanguageServerService/GetStatus" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse url: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("failed to parse port: %v", err)
	}

	tmpDir := t.TempDir()
	daemonDirOverride = tmpDir
	t.Cleanup(func() {
		daemonDirOverride = ""
	})

	jsonContent := fmt.Sprintf(`{
  "pid": 99999,
  "httpsPort": %d,
  "httpPort": 50005,
  "lspPort": 41067,
  "lsVersion": "1.11.0",
  "csrfToken": ""
}`, port)

	jsonPath := filepath.Join(tmpDir, "ls_e3b0c44298fc1c14.json")
	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0600); err != nil {
		t.Fatalf("failed to write mock discovery json: %v", err)
	}

	insp := NewInspector(10 * time.Second)
	info := insp.Scan()
	if info == nil {
		t.Fatalf("expected discovery info, got nil")
	}
	if info.PID != 99999 {
		t.Errorf("expected PID 99999, got %d", info.PID)
	}
	if info.Port != port {
		t.Errorf("expected port %d, got %d", port, info.Port)
	}
	if !info.IsHealthy {
		t.Errorf("expected healthy instance")
	}
}


