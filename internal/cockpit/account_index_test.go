package cockpit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// accountFixtureRow mirrors one row of the Cockpit Tools v1.4+ summaries table.
type accountFixtureRow struct {
	Source   int
	Position *int
	ID       string
	Email    string
	Name     string
}

func intPtr(v int) *int { return &v }

func setupAccountIndexTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("COCKPIT_TOOLS_DATA_DIR", "")
	t.Setenv("COCKPIT_DATA_DIR", dir)
	InvalidateQuotaCache()
	t.Cleanup(InvalidateQuotaCache)
	return dir
}

// writeModernIndexFixture builds a real antigravity_accounts.sqlite file so the
// production read path (sqlite3 CLI / python fallback) is exercised end to end
// instead of being mocked out.
func writeModernIndexFixture(t *testing.T, dir string, rows []accountFixtureRow) {
	t.Helper()
	writeModernIndexFixtureWithState(t, dir, rows, "")
}

func writeModernIndexFixtureWithState(t *testing.T, dir string, rows []accountFixtureRow, stateJSON string) {
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
	);
	CREATE TABLE IF NOT EXISTS account_index_state (
		singleton INTEGER PRIMARY KEY CHECK(singleton=1),
		state TEXT NOT NULL
	);`

	var inserts strings.Builder
	for i, r := range rows {
		pos := i
		if r.Position != nil {
			pos = *r.Position
		}
		fmt.Fprintf(&inserts,
			"INSERT INTO summaries (source,position,account_id,email,name,created_at,last_used) VALUES (%d,%d,'%s','%s','%s',0,0);",
			r.Source, pos, r.ID, r.Email, r.Name)
	}
	if stateJSON != "" {
		fmt.Fprintf(&inserts,
			"INSERT OR REPLACE INTO account_index_state (singleton, state) VALUES (1, '%s');",
			strings.ReplaceAll(stateJSON, "'", "''"))
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

// TestLoadAccountIndex_ModernSQLite verifies v1.4 SQLite reading, out-of-order
// position sorting, source=0 filtering, and current_account.json resolution.
func TestLoadAccountIndex_ModernSQLite(t *testing.T) {
	dir := setupAccountIndexTestDir(t)

	// Insert rows intentionally out of position order and mix in a non-Antigravity
	// source row (source=1) to verify both ORDER BY position and WHERE source=0.
	writeModernIndexFixture(t, dir, []accountFixtureRow{
		{Source: antigravitySummarySource, Position: intPtr(2), ID: "antigravity_ccc", Email: "third@example.com", Name: "Third"},
		{Source: 1, Position: intPtr(0), ID: "codex_other", Email: "codex@example.com", Name: "Codex"},
		{Source: antigravitySummarySource, Position: intPtr(0), ID: "antigravity_aaa", Email: "first@example.com", Name: "First"},
		{Source: antigravitySummarySource, Position: intPtr(1), ID: "antigravity_bbb", Email: "second@example.com", Name: "Second"},
	})
	writeCurrentAccountFixture(t, dir, "second@example.com")

	if _, err := os.Stat(filepath.Join(dir, legacyAccountsIndexName)); err == nil {
		t.Fatal("fixture must not contain accounts.json")
	}

	idx, err := LoadAccountIndex(dir)
	if err != nil {
		t.Fatalf("LoadAccountIndex: %v", err)
	}
	if len(idx.Accounts) != 3 {
		t.Fatalf("expected 3 Antigravity accounts from sqlite index, got %d: %+v", len(idx.Accounts), idx.Accounts)
	}
	if idx.Accounts[0].ID != "antigravity_aaa" || idx.Accounts[1].ID != "antigravity_bbb" || idx.Accounts[2].ID != "antigravity_ccc" {
		t.Errorf("expected ORDER BY position [aaa, bbb, ccc], got %+v", idx.Accounts)
	}
	// current_account.json only carries an email; the id must be resolved from it.
	if idx.CurrentAccountID != "antigravity_bbb" {
		t.Errorf("CurrentAccountID = %q, want antigravity_bbb", idx.CurrentAccountID)
	}
}

// TestLoadAccountIndex_ModernStateFallbackCurrentID verifies account_index_state
// fallback when current_account.json is missing, and immediate freshness once
// current_account.json is written even while the SQLite summary cache is warm.
func TestLoadAccountIndex_ModernStateFallbackCurrentID(t *testing.T) {
	dir := setupAccountIndexTestDir(t)

	writeModernIndexFixtureWithState(t, dir, []accountFixtureRow{
		{ID: "antigravity_aaa", Email: "first@example.com", Name: "First"},
		{ID: "antigravity_bbb", Email: "second@example.com", Name: "Second"},
	}, `{"source":0,"version":2.0,"current_id":"antigravity_bbb","complete":true}`)

	idx, err := LoadAccountIndex(dir)
	if err != nil {
		t.Fatalf("LoadAccountIndex: %v", err)
	}
	if idx.CurrentAccountID != "antigravity_bbb" {
		t.Fatalf("CurrentAccountID = %q, want antigravity_bbb from account_index_state", idx.CurrentAccountID)
	}

	// Writing current_account.json must take effect immediately even without
	// touching the SQLite file or clearing the summary cache.
	writeCurrentAccountFixture(t, dir, "first@example.com")
	idx, err = LoadAccountIndex(dir)
	if err != nil {
		t.Fatalf("LoadAccountIndex after current_account.json update: %v", err)
	}
	if idx.CurrentAccountID != "antigravity_aaa" {
		t.Fatalf("CurrentAccountID = %q, want antigravity_aaa from current_account.json", idx.CurrentAccountID)
	}
}

func TestIsCockpitInstalled_ModernSQLite(t *testing.T) {
	dir := setupAccountIndexTestDir(t)

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
// sqlite file may exist with zero Antigravity rows or only other clients' rows
// (source != 0), which must never be mistaken for Antigravity accounts.
func TestIsCockpitInstalled_RejectsEmptyModernDatabase(t *testing.T) {
	dir := setupAccountIndexTestDir(t)

	writeModernIndexFixture(t, dir, []accountFixtureRow{
		{Source: 1, ID: "codex_only", Email: "codex@example.com", Name: "Codex"},
	})

	if _, err := os.Stat(filepath.Join(dir, modernAccountsIndexName)); err != nil {
		t.Fatalf("fixture database should exist: %v", err)
	}
	if IsCockpitInstalled() {
		t.Fatal("database without Antigravity rows (source=0) must NOT report cockpit as installed")
	}

	idx, err := LoadAccountIndex(dir)
	if err != nil {
		t.Fatalf("LoadAccountIndex on non-Antigravity database: %v", err)
	}
	if len(idx.Accounts) != 0 {
		t.Fatalf("expected 0 accounts, got %d", len(idx.Accounts))
	}
}

// TestGetQuotas_ModernSQLite covers the user-visible path: the quota cache is keyed
// by sha256(email) and never changed format, so a v1.4 index alone must be enough.
func TestGetQuotas_ModernSQLite(t *testing.T) {
	dir := setupAccountIndexTestDir(t)

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

// TestLoadAccountIndex_ModernWinsOverStaleLegacy guards the <= v1.3 -> v1.4+
// upgrade path: Cockpit Tools migrates accounts into antigravity_accounts.sqlite
// while leaving the old accounts.json frozen on disk, so SQLite must win.
func TestLoadAccountIndex_ModernWinsOverStaleLegacy(t *testing.T) {
	dir := setupAccountIndexTestDir(t)

	legacy := `{"current_account_id":"acc-legacy","accounts":[{"id":"acc-legacy","email":"legacy@example.com","name":"Legacy"}]}`
	if err := os.WriteFile(filepath.Join(dir, legacyAccountsIndexName), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	writeModernIndexFixture(t, dir, []accountFixtureRow{
		{ID: "antigravity_new", Email: "new@example.com", Name: "New"},
	})
	writeCurrentAccountFixture(t, dir, "new@example.com")

	idx, err := LoadAccountIndex(dir)
	if err != nil {
		t.Fatalf("LoadAccountIndex: %v", err)
	}
	if len(idx.Accounts) != 1 || idx.Accounts[0].ID != "antigravity_new" {
		t.Fatalf("modern SQLite index must win over frozen legacy accounts.json, got %+v", idx.Accounts)
	}
	if idx.CurrentAccountID != "antigravity_new" {
		t.Errorf("CurrentAccountID = %q, want antigravity_new", idx.CurrentAccountID)
	}
}

// TestLoadAccountIndex_LegacyOnly guards <= v1.3 installs that have no SQLite DB.
func TestLoadAccountIndex_LegacyOnly(t *testing.T) {
	dir := setupAccountIndexTestDir(t)

	legacy := `{"current_account_id":"acc-legacy","accounts":[{"id":"acc-legacy","email":"legacy@example.com","name":"Legacy"}]}`
	if err := os.WriteFile(filepath.Join(dir, legacyAccountsIndexName), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}

	idx, err := LoadAccountIndex(dir)
	if err != nil {
		t.Fatalf("LoadAccountIndex: %v", err)
	}
	if len(idx.Accounts) != 1 || idx.Accounts[0].ID != "acc-legacy" {
		t.Fatalf("legacy index must load when sqlite is absent, got %+v", idx.Accounts)
	}
	if idx.CurrentAccountID != "acc-legacy" {
		t.Errorf("CurrentAccountID = %q, want acc-legacy", idx.CurrentAccountID)
	}
}

// TestLoadAccountIndex_EmptyModernFallsBackToLegacy ensures an empty or
// other-client-only SQLite database does not shadow a populated legacy accounts.json.
func TestLoadAccountIndex_EmptyModernFallsBackToLegacy(t *testing.T) {
	dir := setupAccountIndexTestDir(t)

	legacy := `{"current_account_id":"acc-legacy","accounts":[{"id":"acc-legacy","email":"legacy@example.com","name":"Legacy"}]}`
	if err := os.WriteFile(filepath.Join(dir, legacyAccountsIndexName), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	writeModernIndexFixture(t, dir, []accountFixtureRow{
		{Source: 1, ID: "codex_only", Email: "codex@example.com", Name: "Codex"},
	})

	idx, err := LoadAccountIndex(dir)
	if err != nil {
		t.Fatalf("LoadAccountIndex: %v", err)
	}
	if len(idx.Accounts) != 1 || idx.Accounts[0].ID != "acc-legacy" {
		t.Fatalf("empty modern sqlite should fall back to legacy accounts.json, got %+v", idx.Accounts)
	}
}

// TestLoadAccountIndex_EmptyLegacyFallsThrough covers the migration leftover: a
// zero-account accounts.json must not mask a populated v1.4 database.
func TestLoadAccountIndex_EmptyLegacyFallsThrough(t *testing.T) {
	dir := setupAccountIndexTestDir(t)

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

// TestLoadAccountIndex_CorruptedLegacyErrors is a reverse assertion: a broken
// accounts.json (when SQLite has no accounts) must surface as an error rather
// than silently degrading into "no accounts".
func TestLoadAccountIndex_CorruptedLegacyErrors(t *testing.T) {
	dir := setupAccountIndexTestDir(t)

	if err := os.WriteFile(filepath.Join(dir, legacyAccountsIndexName), []byte(`{"accounts":[{"id":`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAccountIndex(dir); err == nil {
		t.Fatal("truncated accounts.json must return an error, not an empty index")
	}
}

func TestLoadAccountIndex_NoIndexAtAll(t *testing.T) {
	dir := setupAccountIndexTestDir(t)

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
	dir := setupAccountIndexTestDir(t)

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

// TestQueryAccountSummaries_PythonFallback forces the sqlite3 lookup to miss so
// the read-only Python fallback is exercised end to end.
func TestQueryAccountSummaries_PythonFallback(t *testing.T) {
	hasPython := false
	for _, candidate := range pythonCandidates() {
		if p, err := exec.LookPath(candidate); err == nil && isExecutableValid(p) {
			hasPython = true
			break
		}
	}
	if !hasPython {
		t.Skip("python runtime not available; cannot exercise python fallback")
	}

	dir := setupAccountIndexTestDir(t)
	writeModernIndexFixtureWithState(t, dir, []accountFixtureRow{
		{Position: intPtr(1), ID: "antigravity_bbb", Email: "second@example.com", Name: "Second"},
		{Position: intPtr(0), ID: "antigravity_aaa", Email: "first@example.com", Name: "First"},
	}, `{"current_id":"antigravity_bbb"}`)

	origLookPath := accountIndexLookPath
	accountIndexLookPath = func(file string) (string, error) {
		if file == "sqlite3" {
			return "", exec.ErrNotFound
		}
		return origLookPath(file)
	}
	t.Cleanup(func() { accountIndexLookPath = origLookPath })

	idx, err := LoadAccountIndex(dir)
	if err != nil {
		t.Fatalf("LoadAccountIndex via python fallback: %v", err)
	}
	if len(idx.Accounts) != 2 || idx.Accounts[0].ID != "antigravity_aaa" || idx.Accounts[1].ID != "antigravity_bbb" {
		t.Fatalf("unexpected python fallback rows: %+v", idx.Accounts)
	}
	if idx.CurrentAccountID != "antigravity_bbb" {
		t.Errorf("CurrentAccountID via python fallback = %q, want antigravity_bbb", idx.CurrentAccountID)
	}
}

// TestQueryAccountSummaries_SQLiteFailureFallsBackToPython guards against the
// bug where sqlite3 writes an error to stderr (e.g. older sqlite3 without -json)
// and exits non-zero: the stderr output must not block Python fallback.
func TestQueryAccountSummaries_SQLiteFailureFallsBackToPython(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell shim test skipped on Windows")
	}
	hasPython := false
	for _, candidate := range pythonCandidates() {
		if p, err := exec.LookPath(candidate); err == nil && isExecutableValid(p) {
			hasPython = true
			break
		}
	}
	if !hasPython {
		t.Skip("python runtime not available; cannot exercise python fallback")
	}

	dir := setupAccountIndexTestDir(t)
	writeModernIndexFixture(t, dir, []accountFixtureRow{
		{ID: "antigravity_aaa", Email: "first@example.com", Name: "First"},
	})

	fakeSQLite := filepath.Join(t.TempDir(), "sqlite3")
	if err := os.WriteFile(fakeSQLite, []byte("#!/bin/sh\necho 'sqlite3: Error: unknown option: -json' >&2\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}

	origLookPath := accountIndexLookPath
	accountIndexLookPath = func(file string) (string, error) {
		if file == "sqlite3" {
			return fakeSQLite, nil
		}
		return origLookPath(file)
	}
	t.Cleanup(func() { accountIndexLookPath = origLookPath })

	idx, err := LoadAccountIndex(dir)
	if err != nil {
		t.Fatalf("LoadAccountIndex should fall back to python on sqlite3 failure: %v", err)
	}
	if len(idx.Accounts) != 1 || idx.Accounts[0].ID != "antigravity_aaa" {
		t.Fatalf("unexpected fallback accounts: %+v", idx.Accounts)
	}
}

// TestQueryAccountSummaries_ErrorWhenNoRunnerAvailable verifies we do not
// silently return an empty index when neither sqlite3 nor python is available.
func TestQueryAccountSummaries_ErrorWhenNoRunnerAvailable(t *testing.T) {
	dir := setupAccountIndexTestDir(t)
	writeModernIndexFixture(t, dir, []accountFixtureRow{
		{ID: "antigravity_aaa", Email: "first@example.com", Name: "First"},
	})

	origLookPath := accountIndexLookPath
	accountIndexLookPath = func(string) (string, error) {
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { accountIndexLookPath = origLookPath })

	if _, err := LoadAccountIndex(dir); err == nil {
		t.Fatal("expected error when neither sqlite3 nor python3 is available")
	}
}

// TestResolveAccountIDAndListStorage_ModernSQLite verifies account switching
// email->ID lookup and storage listing work against v1.4 SQLite indexes.
func TestResolveAccountIDAndListStorage_ModernSQLite(t *testing.T) {
	dir := setupAccountIndexTestDir(t)
	writeModernIndexFixture(t, dir, []accountFixtureRow{
		{ID: "antigravity_aaa", Email: "first@example.com", Name: "First"},
	})

	accountsDir := filepath.Join(dir, "accounts")
	if err := os.MkdirAll(accountsDir, 0700); err != nil {
		t.Fatal(err)
	}
	plainAcc := `{"id":"antigravity_aaa","email":"first@example.com","name":"First","token":{"access_token":"at","refresh_token":"rt"}}`
	if err := os.WriteFile(filepath.Join(accountsDir, "antigravity_aaa.json"), []byte(plainAcc), 0600); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveAccountID("FIRST@EXAMPLE.COM")
	if err != nil {
		t.Fatalf("ResolveAccountID: %v", err)
	}
	if resolved != "antigravity_aaa" {
		t.Fatalf("ResolveAccountID = %q, want antigravity_aaa", resolved)
	}

	if email := resolveAccountEmail("antigravity_aaa"); email != "first@example.com" {
		t.Fatalf("resolveAccountEmail = %q, want first@example.com", email)
	}

	list, err := ListAccountsFromStorage()
	if err != nil {
		t.Fatalf("ListAccountsFromStorage: %v", err)
	}
	if len(list) != 1 || list[0].ID != "antigravity_aaa" || list[0].Email != "first@example.com" {
		t.Fatalf("unexpected ListAccountsFromStorage result: %+v", list)
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
