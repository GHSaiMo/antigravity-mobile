package cockpit

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestLookupLanguageServerAntigravityCLIDaemon(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("ANTIGRAVITY_APP_DATA_DIR", "")

	lsLookupMu.Lock()
	cachedLSPort = 0
	cachedLSCSRF = ""
	cachedLSUntil = cachedLSUntil.AddDate(-1, 0, 0)
	lastLSFailScan = lastLSFailScan.AddDate(-1, 0, 0)
	lsLookupMu.Unlock()

	cliDaemonDir := filepath.Join(tmpHome, ".gemini", "antigravity-cli", "daemon")
	if err := os.MkdirAll(cliDaemonDir, 0700); err != nil {
		t.Fatalf("mkdir cli daemon dir: %v", err)
	}
	payload := fmt.Sprintf(`{"httpsPort": %d, "csrfToken": "cli-csrf-123"}`, port)
	if err := os.WriteFile(filepath.Join(cliDaemonDir, "ls_cli.json"), []byte(payload), 0600); err != nil {
		t.Fatalf("write cli discovery json: %v", err)
	}

	gotPort, gotCSRF, err := lookupLanguageServer()
	if err != nil {
		t.Fatalf("lookupLanguageServer() error = %v", err)
	}
	if gotPort != port || gotCSRF != "cli-csrf-123" {
		t.Fatalf("lookupLanguageServer() = (%d, %q), want (%d, %q)", gotPort, gotCSRF, port, "cli-csrf-123")
	}
}

func TestArchiveConversationTokensCLIFallback(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Reset cached token ledger so it writes under tmpHome.
	historyMu.Lock()
	cachedLedger = nil
	cachedLedgerPath = ""
	historyMu.Unlock()
	t.Cleanup(func() {
		historyMu.Lock()
		cachedLedger = nil
		cachedLedgerPath = ""
		historyMu.Unlock()
	})

	cid := "cli-archive-session-001"
	logsDir := filepath.Join(tmpHome, ".gemini", "antigravity-cli", "brain", cid, ".system_generated", "logs")
	if err := os.MkdirAll(logsDir, 0700); err != nil {
		t.Fatalf("mkdir logs dir: %v", err)
	}
	line := `{"type":"PLANNER_RESPONSE","step_index":1,"created_at":"2026-11-26T06:00:00Z","model":"gemini-4-neon-high","input_tokens":100,"output_tokens":50}` + "\n"
	if err := os.WriteFile(filepath.Join(logsDir, "transcript.jsonl"), []byte(line), 0600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	ArchiveConversationTokens(cid)

	historyMu.RLock()
	defer historyMu.RUnlock()
	if cachedLedger == nil {
		t.Fatalf("expected token ledger to be populated for CLI conversation")
	}
	sess, ok := cachedLedger.Sessions["cli:"+cid]
	if !ok {
		t.Fatalf("expected archived session key %q in ledger, got keys: %v", "cli:"+cid, cachedLedger.Sessions)
	}
	if sess.Source != "cli" {
		t.Errorf("archived session source = %q, want %q", sess.Source, "cli")
	}
}
