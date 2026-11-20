package cockpit

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Account index resolution that works across Cockpit Tools storage generations.
//
// Cockpit Tools <= v1.3 kept one JSON index at <dataDir>/accounts.json holding
// { current_account_id, accounts[] }. Since v1.4 the index moved into a SQLite
// database (<dataDir>/antigravity_accounts.sqlite, table `summaries`) and
// accounts.json is no longer written at all.
//
// Everything else kept its shape, which is why the blast radius here is small:
//   - encrypted per-account files <dataDir>/accounts/<id>.json  (AES-256-GCM, unchanged)
//   - quota cache <dataDir>/cache/quota_api_v1_desktop/authorized/<sha256(email)>.json (unchanged)
//   - current account <dataDir>/current_account.json  (now the only "who is active" hint)
//
// Before this file existed, every caller stat/read accounts.json directly, so a
// v1.4 install looked "not installed" and /cockpit/quotas returned
// { accounts: [], cockpit_installed: false } even though the data was all there.

const (
	legacyAccountsIndexName = "accounts.json"
	modernAccountsIndexName = "antigravity_accounts.sqlite"
	currentAccountName      = "current_account.json"

	// antigravitySummarySource is the `summaries.source` discriminator Cockpit
	// Tools uses for Antigravity accounts (0). Other clients (codex/cursor/...)
	// live in their own source buckets and are out of scope here.
	antigravitySummarySource = 0
)

// accountSummariesSQL selects the Antigravity bucket in stable display order.
// COALESCE keeps nullable columns out of the JSON payload as empty strings.
// The source filter is inlined because it is a compile-time constant and the
// statement is handed to sqlite3 verbatim.
const accountSummariesSQL = "SELECT account_id, email, COALESCE(name, '') AS name " +
	"FROM summaries WHERE source = 0 ORDER BY position;"

// indexedAccount is the version-agnostic row shape used by quota resolution.
type indexedAccount struct {
	ID    string
	Email string
	Name  string
}

// cockpitAccountIndex is the version-agnostic replacement for accountsIndex.
type cockpitAccountIndex struct {
	CurrentAccountID string
	Accounts         []indexedAccount
}

// LoadAccountIndex returns the authoritative account list, tolerating both
// storage generations. A readable legacy index wins when it actually has rows;
// otherwise the SQLite summaries table is used.
//
// An existing but unparsable accounts.json is returned as an error instead of
// being silently skipped: a truncated legacy file should surface, not degrade
// into a misleading "no accounts" response.
func LoadAccountIndex(dataDir string) (*cockpitAccountIndex, error) {
	legacyPath := filepath.Join(dataDir, legacyAccountsIndexName)
	if _, err := os.Stat(legacyPath); err == nil {
		idx, parseErr := parseLegacyAccountsIndex(legacyPath)
		if parseErr != nil {
			return nil, parseErr
		}
		if len(idx.Accounts) > 0 {
			return idx, nil
		}
		// Empty legacy index (e.g. left behind during a v1.4 migration): fall
		// through and let the SQLite index answer.
	}

	return loadModernAccountIndex(dataDir)
}

func parseLegacyAccountsIndex(path string) (*cockpitAccountIndex, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", legacyAccountsIndexName, err)
	}
	var parsed accountsIndex
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", legacyAccountsIndexName, err)
	}

	idx := &cockpitAccountIndex{CurrentAccountID: strings.TrimSpace(parsed.CurrentAccountID)}
	for _, acc := range parsed.Accounts {
		idx.Accounts = append(idx.Accounts, indexedAccount{
			ID:    strings.TrimSpace(acc.ID),
			Email: strings.TrimSpace(acc.Email),
			Name:  strings.TrimSpace(acc.Name),
		})
	}
	if idx.CurrentAccountID == "" {
		idx.CurrentAccountID = resolveCurrentAccountID(idx.Accounts, readCurrentAccountEmail(filepath.Dir(path)))
	}
	return idx, nil
}

// loadModernAccountIndex reads the v1.4+ SQLite index. A missing database is not
// an error: it simply means Cockpit Tools was never installed or has no accounts.
func loadModernAccountIndex(dataDir string) (*cockpitAccountIndex, error) {
	dbPath := filepath.Join(dataDir, modernAccountsIndexName)
	if fi, err := os.Stat(dbPath); err != nil || fi.IsDir() {
		return &cockpitAccountIndex{}, nil
	}

	rows, err := queryAccountSummaries(dbPath)
	if err != nil {
		return nil, err
	}

	idx := &cockpitAccountIndex{Accounts: rows}
	idx.CurrentAccountID = resolveCurrentAccountID(rows, readCurrentAccountEmail(dataDir))
	return idx, nil
}

// queryAccountSummaries shells out to sqlite3, with a python3 fallback. This
// mirrors the existing state.vscdb read path in jetski_oauth.go rather than
// pulling a cgo/pure-Go SQLite driver into a project that builds for
// darwin/linux/windows without cgo.
func queryAccountSummaries(dbPath string) ([]indexedAccount, error) {
	cleanDB := filepath.Clean(dbPath)
	if cleanDB == "" || strings.ContainsAny(cleanDB, "\x00\r\n") {
		return nil, fmt.Errorf("invalid cockpit account index path")
	}

	var out []byte
	var lastErr error

	if p, lookErr := exec.LookPath("sqlite3"); lookErr == nil && isExecutableValid(p) {
		out, lastErr = runCmdCombinedTimeout(5*time.Second, p, "-batch", "-json", "-cmd", ".timeout 3000", cleanDB, accountSummariesSQL)
	}

	if len(out) == 0 {
		const pyScript = "import json,sqlite3,sys\n" +
			"conn=sqlite3.connect(sys.argv[1],timeout=3.0)\n" +
			"rows=conn.execute('SELECT account_id, email, COALESCE(name, \\'\\') FROM summaries WHERE source=0 ORDER BY position').fetchall()\n" +
			"sys.stdout.write(json.dumps([{'account_id':r[0],'email':r[1] or '','name':r[2] or ''} for r in rows]))\n"
		for _, pyExe := range pythonCandidates() {
			p, lookErr := exec.LookPath(pyExe)
			if lookErr != nil || !isExecutableValid(p) {
				continue
			}
			if pyOut, pyErr := runCmdTimeout(5*time.Second, p, "-c", pyScript, cleanDB); pyErr == nil && len(pyOut) > 0 {
				out = pyOut
				lastErr = nil
				break
			}
		}
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		// sqlite3 -json prints nothing for an empty result set. Keep the original
		// error (if any) out of the way: no rows is a valid, non-installed state.
		if lastErr != nil {
			return nil, fmt.Errorf("failed to read %s: %w", modernAccountsIndexName, lastErr)
		}
		return nil, nil
	}

	var rows []struct {
		AccountID string `json:"account_id"`
		Email     string `json:"email"`
		Name      string `json:"name"`
	}
	if err := json.Unmarshal([]byte(trimmed), &rows); err != nil {
		return nil, fmt.Errorf("failed to parse %s summaries: %w", modernAccountsIndexName, err)
	}

	accounts := make([]indexedAccount, 0, len(rows))
	for _, r := range rows {
		id := strings.TrimSpace(r.AccountID)
		if id == "" {
			continue // a row without an id cannot be decrypted or keyed
		}
		accounts = append(accounts, indexedAccount{
			ID:    id,
			Email: strings.TrimSpace(r.Email),
			Name:  strings.TrimSpace(r.Name),
		})
	}
	return accounts, nil
}

// readCurrentAccountEmail reads the "active account" hint written by both
// storage generations. It only carries an email, not an id.
func readCurrentAccountEmail(dataDir string) string {
	raw, err := os.ReadFile(filepath.Join(dataDir, currentAccountName))
	if err != nil {
		return ""
	}
	var doc struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(doc.Email))
}

func resolveCurrentAccountID(accounts []indexedAccount, email string) string {
	if email == "" {
		return ""
	}
	for _, acc := range accounts {
		if strings.EqualFold(strings.TrimSpace(acc.Email), email) {
			return acc.ID
		}
	}
	return ""
}
