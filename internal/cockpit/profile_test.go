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
}

func TestClearStaleAntigravityIdentity(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not available")
	}

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

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
