package cockpit

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestSQLiteQuote(t *testing.T) {
	if got := sqliteQuote("a'b"); got != "'a''b'" {
		t.Fatalf("sqliteQuote = %q", got)
	}
	if got := sqliteQuote("a\x00b'c"); got != "'ab''c'" {
		t.Fatalf("sqliteQuote with null byte = %q, want 'ab''c'", got)
	}
}

func TestIsSafeProxyURL(t *testing.T) {
	valid := []string{
		"http://127.0.0.1:7890",
		"https://proxy.example.com:8080",
		"socks5://127.0.0.1:1080",
		"socks5h://localhost:1080",
	}
	for _, v := range valid {
		if !isSafeProxyURL(v) {
			t.Errorf("expected %q to be safe proxy URL", v)
		}
	}
	invalid := []string{
		"",
		"ftp://127.0.0.1",
		"javascript:alert(1)",
		"http://127.0.0.1\nmalicious",
		"http://127.0.0.1\r\nmalicious",
		"http://",
		"just-a-string",
	}
	for _, inv := range invalid {
		if isSafeProxyURL(inv) {
			t.Errorf("expected %q to be unsafe proxy URL", inv)
		}
	}
}

func TestValidateSQLiteKey(t *testing.T) {
	validKeys := []string{
		"antigravityAuthStatus",
		"antigravityUnifiedStateSync.userStatus",
		"history.recentlyOpenedPathsList",
		"item_123",
		"my-key",
	}
	for _, k := range validKeys {
		if err := validateSQLiteKey(k); err != nil {
			t.Errorf("expected valid key %q, got error: %v", k, err)
		}
	}

	invalidKeys := []string{
		"key'; DROP TABLE ItemTable; --",
		"key OR 1=1",
		"key\"--",
		"key with spaces",
		"key\nnewline",
	}
	for _, k := range invalidKeys {
		if err := validateSQLiteKey(k); err == nil {
			t.Errorf("expected error for malicious key %q, got nil", k)
		}
	}
}

func TestExecSQLiteDatabaseValidation(t *testing.T) {
	// Attempting to run on arbitrary file like /etc/passwd or script.sh must be rejected
	if err := execSQLite("/etc/passwd", "SELECT 1;"); err == nil {
		t.Errorf("expected error for non-sqlite db extension, got nil")
	}
	if err := execSQLite("/tmp/script.sh", "SELECT 1;"); err == nil {
		t.Errorf("expected error for non-sqlite db extension, got nil")
	}
}

func TestClearStaleAntigravityIdentity(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("APPDATA", filepath.Join(tmp, "AppData", "Roaming"))

	dbPath := filepath.Join(tmp, "Library", "Application Support", "Antigravity", "User", "globalStorage", "state.vscdb")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}

	sql := `CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value BLOB);
INSERT INTO ItemTable(key, value) VALUES
 ('antigravityAuthStatus', '{"email":"old@example.com"}'),
 ('antigravityUnifiedStateSync.userStatus', 'stale-status'),
 ('history.recentlyOpenedPathsList', 'keep-me');`
	if out, err := exec.Command("sqlite3", dbPath, sql).CombinedOutput(); err != nil {
		t.Fatalf("seed sqlite: %v (%s)", err, out)
	}

	clearStaleAntigravityIdentity()

	out, err := exec.Command("sqlite3", dbPath, "SELECT key FROM ItemTable ORDER BY key;").Output()
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if got != "history.recentlyOpenedPathsList\n" {
		t.Fatalf("remaining keys = %q, want only history.recentlyOpenedPathsList", got)
	}
}

func TestSyncLegacyBindAccount(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", tmp)

	path := filepath.Join(tmp, "antigravity_legacy_instances.json")
	if err := os.WriteFile(path, []byte(`{
  "instances": [],
  "defaultSettings": {
    "bindAccountId": "old-id",
    "launchMode": "app"
  }
}`), 0644); err != nil {
		t.Fatal(err)
	}

	syncLegacyBindAccount("new-id")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	ds := doc["defaultSettings"].(map[string]any)
	if ds["bindAccountId"] != "new-id" {
		t.Fatalf("bindAccountId = %v", ds["bindAccountId"])
	}
	if ds["launchMode"] != "app" {
		t.Fatalf("launchMode should be preserved, got %v", ds["launchMode"])
	}
}
