package cockpit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// accountFixtureRow mirrors one row of the Cockpit Tools v1.4+ summaries table.
type accountFixtureRow struct {
	ID    string
	Email string
	Name  string
}

// writeModernIndexFixture builds a real antigravity_accounts.sqlite file so the
// production read path (sqlite3 CLI / python fallback) is exercised end to end
// instead of being mocked out.
func writeModernIndexFixture(t *testing.T, dir string, rows []accountFixtureRow) {
	t.Helper()

	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 CLI not available; cannot build a v1.4 index fixture")
	}

	dbPath := filepath.Join(dir, modernAccountsIndexName)

	// Schema copied from a real Cockpit Tools v1.4.2 database.
	schema := `CREATE TABLE IF NOT EXISTS summaries (
		source INTEGER NOT NULL, position INTEGER NOT NULL,
		account_id TEXT NOT NULL, email TEXT NOT NULL, name TEXT,
		created_at INTEGER NOT NULL, last_used INTEGER NOT NULL,
		plan_type TEXT, subscription_active_until TEXT,
		provider_login TEXT, provider_user_id TEXT, payload TEXT,
		PRIMARY KEY(source,position)
	);`

	var inserts strings.Builder
	for i, r := range rows {
		fmt.Fprintf(&inserts,
			"INSERT INTO summaries (source,position,account_id,email,name,created_at,last_used) VALUES (%d,%d,'%s','%s','%s',0,0);",
			antigravitySummarySource, i, r.ID, r.Email, r.Name)
	}

	stmt := schema + "\n" + inserts.String()
	if out, err := exec.Command("sqlite3", dbPath, stmt).CombinedOutput(); err != nil {
		t.Fatalf("build sqlite fixture: %v (%s)", err, strings.TrimSpace(string(out)))
	}
}

func writeCurrentAccountFixture(t *testing.T, dir, email string) {
	t.Helper()
	doc := fmt.Sprintf(`{"email":%q,"updated_at":1795077590}`, email)
	if err := os.WriteFile(filepath.Join(dir, currentAccountName), []byte(doc), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestLoadAccountIndex_ModernSQLite is the regression this whole change exists for:
// Cockpit Tools v1.4 has no accounts.json at all, so the index must come from SQLite.
func TestLoadAccountIndex_ModernSQLite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", dir)

	writeModernIndexFixture(t, dir, []accountFixtureRow{
		{ID: "antigravity_aaa", Email: "first@example.com", Name: "First"},
		{ID: "antigravity_bbb", Email: "second@example.com", Name: "Second"},
	})
	writeCurrentAccountFixture(t, dir, "second@example.com")

	if _, err := os.Stat(filepath.Join(dir, legacyAccountsIndexName)); err == nil {
		t.Fatal("fixture must not contain accounts.json")
	}

	idx, err := LoadAccountIndex(dir)
	if err != nil {
		t.Fatalf("LoadAccountIndex: %v", err)
	}
	if len(idx.Accounts) != 2 {
		t.Fatalf("expected 2 accounts from sqlite index, got %d", len(idx.Accounts))
	}
	if idx.Accounts[0].ID != "antigravity_aaa" || idx.Accounts[0].Email != "first@example.com" {
		t.Errorf("unexpected first row: %+v", idx.Accounts[0])
	}
	// ORDER BY position must be respected, not SQLite's incidental row order.
	if idx.Accounts[1].ID != "antigravity_bbb" {
		t.Errorf("expected position order to be preserved, got %+v", idx.Accounts[1])
	}
	// current_account.json only carries an email; the id must be resolved from it.
	if idx.CurrentAccountID != "antigravity_bbb" {
		t.Errorf("CurrentAccountID = %q, want antigravity_bbb", idx.CurrentAccountID)
	}
}

func TestIsCockpitInstalled_ModernSQLite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", dir)

	if IsCockpitInstalled() {
		t.Fatal("empty dir must not report cockpit as installed")
	}

	writeModernIndexFixture(t, dir, []accountFixtureRow{
		{ID: "antigravity_aaa", Email: "first@example.com", Name: "First"},
	})

	if !IsCockpitInstalled() {
		t.Fatal("v1.4 sqlite index with an account must report cockpit as installed")
	}
}

// TestIsCockpitInstalled_RejectsEmptyModernDatabase is the reverse assertion: the
// sqlite file exists for every client (codex/cursor/...), so its mere presence must
// never be mistaken for "Cockpit Tools has Antigravity accounts".
func TestIsCockpitInstalled_RejectsEmptyModernDatabase(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", dir)

	writeModernIndexFixture(t, dir, nil) // database exists, zero summaries

	if _, err := os.Stat(filepath.Join(dir, modernAccountsIndexName)); err != nil {
		t.Fatalf("fixture database should exist: %v", err)
	}
	if IsCockpitInstalled() {
		t.Fatal("database without Antigravity rows must NOT report cockpit as installed")
	}

	idx, err := LoadAccountIndex(dir)
	if err != nil {
		t.Fatalf("LoadAccountIndex on empty database: %v", err)
	}
	if len(idx.Accounts) != 0 {
		t.Fatalf("expected 0 accounts, got %d", len(idx.Accounts))
	}
}

// TestGetQuotas_ModernSQLite covers the user-visible path: the quota cache is keyed
// by sha256(email) and never changed format, so a v1.4 index alone must be enough.
func TestGetQuotas_ModernSQLite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", dir)
	InvalidateQuotaCache()

	writeModernIndexFixture(t, dir, []accountFixtureRow{
		{ID: "antigravity_aaa", Email: "first@example.com", Name: "First"},
		{ID: "antigravity_bbb", Email: "second@example.com", Name: "Second"},
	})
	writeCurrentAccountFixture(t, dir, "first@example.com")

	// Quota cache entry for the first account only; the second stays quota-less but
	// must still show up in the list.
	cacheDir := filepath.Join(dir, "cache", "quota_api_v1_desktop", "authorized")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(cacheDir, quotaCacheFileName("first@example.com"))
	if err := os.WriteFile(cachePath, []byte(`{
		"updatedAt": 1720000000,
		"payload": {
			"quota_summary": {
				"groups": [
					{"buckets": [
						{"bucketId": "gemini-5h", "remainingFraction": 0.85, "resetTime": "2026-09-14T20:00:00Z"},
						{"bucketId": "3p-5h", "remainingFraction": 0.42, "resetTime": "2026-09-14T21:00:00Z"}
					]}
				]
			}
		}
	}`), 0644); err != nil {
		t.Fatal(err)
	}

	resp, err := BuildQuotaResponse(nil, "")
	if err != nil {
		t.Fatalf("BuildQuotaResponse: %v", err)
	}
	if !resp.CockpitInstalled {
		t.Fatal("cockpit_installed must be true when the v1.4 index has accounts")
	}
	if resp.Source != "cockpit" {
		t.Errorf("source = %q, want cockpit", resp.Source)
	}
	if len(resp.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(resp.Accounts))
	}
	if resp.CurrentAccount == nil || resp.CurrentAccount.Email != "first@example.com" {
		t.Fatalf("current account = %+v, want first@example.com", resp.CurrentAccount)
	}
	if resp.CurrentAccount.Gemini5h == nil || resp.CurrentAccount.Gemini5h.RemainingPercent != 85.0 {
		t.Errorf("expected Gemini5h 85%%, got %+v", resp.CurrentAccount.Gemini5h)
	}
}

// TestLoadAccountIndex_LegacyStillWins guards the upgrade path: installs that still
// have a populated accounts.json must keep behaving exactly as before.
func TestLoadAccountIndex_LegacyStillWins(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", dir)

	legacy := `{"current_account_id":"acc-legacy","accounts":[{"id":"acc-legacy","email":"legacy@example.com","name":"Legacy"}]}`
	if err := os.WriteFile(filepath.Join(dir, legacyAccountsIndexName), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	// A stale v1.4 database must not shadow the legacy index.
	writeModernIndexFixture(t, dir, []accountFixtureRow{
		{ID: "antigravity_new", Email: "new@example.com", Name: "New"},
	})

	idx, err := LoadAccountIndex(dir)
	if err != nil {
		t.Fatalf("LoadAccountIndex: %v", err)
	}
	if len(idx.Accounts) != 1 || idx.Accounts[0].ID != "acc-legacy" {
		t.Fatalf("legacy index must win, got %+v", idx.Accounts)
	}
	if idx.CurrentAccountID != "acc-legacy" {
		t.Errorf("CurrentAccountID = %q, want acc-legacy", idx.CurrentAccountID)
	}
}

// TestLoadAccountIndex_EmptyLegacyFallsThrough covers the migration leftover: a
// zero-account accounts.json must not mask a populated v1.4 database.
func TestLoadAccountIndex_EmptyLegacyFallsThrough(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", dir)

	if err := os.WriteFile(filepath.Join(dir, legacyAccountsIndexName), []byte(`{"current_account_id":"","accounts":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	writeModernIndexFixture(t, dir, []accountFixtureRow{
		{ID: "antigravity_new", Email: "new@example.com", Name: "New"},
	})

	idx, err := LoadAccountIndex(dir)
	if err != nil {
		t.Fatalf("LoadAccountIndex: %v", err)
	}
	if len(idx.Accounts) != 1 || idx.Accounts[0].ID != "antigravity_new" {
		t.Fatalf("empty legacy index should fall through to sqlite, got %+v", idx.Accounts)
	}
}

// TestLoadAccountIndex_CorruptedLegacyErrors is the second reverse assertion: a
// broken accounts.json must surface as an error rather than silently degrading into
// "no accounts" (or silently switching to another source).
func TestLoadAccountIndex_CorruptedLegacyErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", dir)

	if err := os.WriteFile(filepath.Join(dir, legacyAccountsIndexName), []byte(`{"accounts":[{"id":`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAccountIndex(dir); err == nil {
		t.Fatal("truncated accounts.json must return an error, not an empty index")
	}
}

func TestLoadAccountIndex_NoIndexAtAll(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", dir)

	idx, err := LoadAccountIndex(dir)
	if err != nil {
		t.Fatalf("missing index must not error: %v", err)
	}
	if len(idx.Accounts) != 0 {
		t.Fatalf("expected 0 accounts, got %d", len(idx.Accounts))
	}

	resp, err := BuildQuotaResponse(nil, "")
	if err != nil {
		t.Fatalf("BuildQuotaResponse: %v", err)
	}
	if resp.CockpitInstalled {
		t.Error("no index present means cockpit_installed=false")
	}
	if resp.Source != "live" {
		t.Errorf("source = %q, want live fallback", resp.Source)
	}
	if len(resp.Accounts) != 0 {
		t.Errorf("expected no cockpit accounts, got %d", len(resp.Accounts))
	}
}

// TestLoadAccountIndex_ModernRowsWithoutIDAreDropped keeps the invariant the mobile
// clients rely on: every returned account carries a usable, unique id.
func TestLoadAccountIndex_ModernRowsWithoutIDAreDropped(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", dir)

	writeModernIndexFixture(t, dir, []accountFixtureRow{
		{ID: "", Email: "ghost@example.com", Name: "Ghost"},
		{ID: "antigravity_ok", Email: "ok@example.com", Name: "Ok"},
	})

	idx, err := LoadAccountIndex(dir)
	if err != nil {
		t.Fatalf("LoadAccountIndex: %v", err)
	}
	if len(idx.Accounts) != 1 || idx.Accounts[0].ID != "antigravity_ok" {
		t.Fatalf("rows without an id must be dropped, got %+v", idx.Accounts)
	}
}

// quotaCacheFileName mirrors the cache key derivation used by GetQuotas:
// hex(sha256(lower(trim(email)))) + ".json".
func quotaCacheFileName(email string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(h[:]) + ".json"
}

// TestParseLegacyAccountsIndex_NameAndEmailTrimmed keeps whitespace-padded legacy
// entries from breaking cache lookups.
func TestParseLegacyAccountsIndex_NameAndEmailTrimmed(t *testing.T) {
	dir := t.TempDir()
	doc := `{"accounts":[{"id":" acc-1 ","email":" spaced@example.com ","name":" Name "}]}`
	path := filepath.Join(dir, legacyAccountsIndexName)
	if err := os.WriteFile(path, []byte(doc), 0644); err != nil {
		t.Fatal(err)
	}
	idx, err := parseLegacyAccountsIndex(path)
	if err != nil {
		t.Fatalf("parseLegacyAccountsIndex: %v", err)
	}
	if len(idx.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(idx.Accounts))
	}
	if idx.Accounts[0].ID != "acc-1" || idx.Accounts[0].Email != "spaced@example.com" || idx.Accounts[0].Name != "Name" {
		t.Errorf("values not trimmed: %+v", idx.Accounts[0])
	}
}
