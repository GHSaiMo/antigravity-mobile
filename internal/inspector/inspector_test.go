package inspector

import (
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
	if info.Port == 0 || info.CSRFToken == "" {
		t.Fatalf("invalid scanned info: %+v", info)
	}
	t.Logf("Live discovery succeeded: Port=%d, CSRF=%s, PID=%d", info.Port, info.CSRFToken, info.PID)
}
