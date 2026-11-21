package cockpit

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Account index resolution that works across Cockpit Tools storage generations.
//
// Cockpit Tools <= v1.3 kept one JSON index at <dataDir>/accounts.json holding
// { current_account_id, accounts[] }. Since v1.4 the index moved into a SQLite
// database (<dataDir>/antigravity_accounts.sqlite, table `summaries` +
// `account_index_state`). Upgrading from <= v1.3 to v1.4+ migrates accounts
// into SQLite while often leaving the legacy accounts.json frozen on disk.
//
// Everything else kept its shape, which is why the blast radius here is small:
//   - encrypted per-account files <dataDir>/accounts/<id>.json  (AES-256-GCM, unchanged)
//   - quota cache <dataDir>/cache/quota_api_v1_desktop/authorized/<sha256(email)>.json (unchanged)
//   - current account <dataDir>/current_account.json  (active email hint written by both Cockpit Tools and mgy)

const (
	legacyAccountsIndexName = "accounts.json"
	modernAccountsIndexName = "antigravity_accounts.sqlite"
	currentAccountName      = "current_account.json"

	// antigravitySummarySource is the `summaries.source` discriminator Cockpit
	// Tools uses for Antigravity accounts (0). Other clients (codex/cursor/...)
	// live in their own source buckets and are out of scope here.
	antigravitySummarySource = 0

	modernIndexCacheTTL = 10 * time.Minute
)

// accountSummariesSQL selects the Antigravity bucket in stable display order.
// COALESCE keeps nullable columns out of the JSON payload as empty strings.
var accountSummariesSQL = fmt.Sprintf(
	"SELECT account_id, email, COALESCE(name, '') AS name FROM summaries WHERE source = %d ORDER BY position;",
	antigravitySummarySource,
)

const accountIndexStateSQL = "SELECT state FROM account_index_state WHERE singleton = 1 LIMIT 1;"

// accountIndexLookPath allows unit tests to exercise sqlite3/python fallback
// behavior deterministically without mutating process-wide PATH.
var accountIndexLookPath = exec.LookPath

type modernSQLiteStat struct {
	dbModNano  int64
	dbSize     int64
	walExists  bool
	walModNano int64
	walSize    int64
}

type modernIndexCacheEntry struct {
	stat               modernSQLiteStat
	accounts           []indexedAccount
	stateCurrentLoaded bool
	stateCurrentID     string
	expiresAt          time.Time
}

var (
	modernIndexCacheMu sync.RWMutex
	modernIndexCache   = make(map[string]*modernIndexCacheEntry)
)

func invalidateModernAccountIndexCache() {
	modernIndexCacheMu.Lock()
	modernIndexCache = make(map[string]*modernIndexCacheEntry)
	modernIndexCacheMu.Unlock()
}

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

// LoadAccountIndex returns the authoritative account list across Cockpit Tools
// storage generations.
//
// Resolution order:
//  1. Modern SQLite index (antigravity_accounts.sqlite, summaries WHERE source=0):
//     Cockpit Tools <= v1.3 never creates this file, and upgrading to v1.4+
//     leaves the old accounts.json frozen on disk. Whenever SQLite contains
//     Antigravity accounts, it is the authoritative source.
//  2. Legacy JSON index (accounts.json): used for <= v1.3 installs, or when the
//     SQLite database is absent / has no Antigravity rows (e.g. only other
//     clients) / cannot be read while a valid legacy index exists.
//
// An existing but unparsable accounts.json (when SQLite did not provide
// accounts) is returned as an error instead of silently degrading into "no
// accounts".
func LoadAccountIndex(dataDir string) (*cockpitAccountIndex, error) {
	modernIdx, modernErr := loadModernAccountIndex(dataDir)
	if modernErr == nil && len(modernIdx.Accounts) > 0 {
		return modernIdx, nil
	}

	legacyPath := filepath.Join(dataDir, legacyAccountsIndexName)
	if _, err := os.Stat(legacyPath); err == nil {
		return parseLegacyAccountsIndex(legacyPath)
	}

	if modernErr != nil {
		return nil, modernErr
	}
	return modernIdx, nil
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

func statModernSQLite(dbPath string) (modernSQLiteStat, bool) {
	fi, err := os.Stat(dbPath)
	if err != nil || fi.IsDir() {
		return modernSQLiteStat{}, false
	}
	st := modernSQLiteStat{
		dbModNano: fi.ModTime().UnixNano(),
		dbSize:    fi.Size(),
	}
	if wfi, wErr := os.Stat(dbPath + "-wal"); wErr == nil && !wfi.IsDir() {
		st.walExists = true
		st.walModNano = wfi.ModTime().UnixNano()
		st.walSize = wfi.Size()
	}
	return st, true
}

// loadModernAccountIndex reads the v1.4+ SQLite index. A missing database is not
// an error: it simply means Cockpit Tools was never installed or has no accounts.
func loadModernAccountIndex(dataDir string) (*cockpitAccountIndex, error) {
	dbPath := filepath.Clean(filepath.Join(dataDir, modernAccountsIndexName))
	st, ok := statModernSQLite(dbPath)
	if !ok {
		return &cockpitAccountIndex{}, nil
	}

	now := time.Now()
	var rows []indexedAccount
	var stateCurrentLoaded bool
	var stateCurrentID string
	cacheHit := false

	modernIndexCacheMu.RLock()
	if entry, exists := modernIndexCache[dbPath]; exists && entry.stat == st && now.Before(entry.expiresAt) {
		rows = append([]indexedAccount(nil), entry.accounts...)
		stateCurrentLoaded = entry.stateCurrentLoaded
		stateCurrentID = entry.stateCurrentID
		cacheHit = true
	}
	modernIndexCacheMu.RUnlock()

	if !cacheHit {
		queried, err := queryAccountSummaries(dbPath)
		if err != nil {
			return nil, err
		}
		rows = queried
		modernIndexCacheMu.Lock()
		if len(modernIndexCache) > 16 {
			modernIndexCache = make(map[string]*modernIndexCacheEntry)
		}
		modernIndexCache[dbPath] = &modernIndexCacheEntry{
			stat:      st,
			accounts:  append([]indexedAccount(nil), rows...),
			expiresAt: now.Add(modernIndexCacheTTL),
		}
		modernIndexCacheMu.Unlock()
	}

	idx := &cockpitAccountIndex{Accounts: rows}
	idx.CurrentAccountID = resolveCurrentAccountID(rows, readCurrentAccountEmail(dataDir))
	if idx.CurrentAccountID == "" && len(rows) > 0 {
		if !stateCurrentLoaded {
			stateCurrentID = queryModernStateCurrentID(dbPath)
			modernIndexCacheMu.Lock()
			if entry, exists := modernIndexCache[dbPath]; exists && entry.stat == st {
				entry.stateCurrentLoaded = true
				entry.stateCurrentID = stateCurrentID
			}
			modernIndexCacheMu.Unlock()
		}
		idx.CurrentAccountID = matchAccountID(rows, stateCurrentID)
	}
	return idx, nil
}

// queryAccountSummaries shells out to sqlite3, with a python3 fallback. This
// mirrors the existing state.vscdb read path in jetski_oauth.go rather than
// pulling a cgo/pure-Go SQLite driver into a project that builds for
// darwin/linux/windows without cgo.
func queryAccountSummaries(dbPath string) ([]indexedAccount, error) {
	cleanDB, pathErr := safeSQLitePathArg(dbPath)
	if pathErr != nil {
		return nil, fmt.Errorf("invalid cockpit account index path")
	}

	var out []byte
	var lastErr error
	ranOK := false

	if p, lookErr := accountIndexLookPath("sqlite3"); lookErr == nil && isExecutableValid(p) {
		out, lastErr = runCmdTimeout(5*time.Second, p, "-readonly", "-batch", "-json", "-cmd", ".timeout 3000", cleanDB, accountSummariesSQL)
		if lastErr == nil {
			ranOK = true
		}
	}

	if !ranOK {
		pyOut, pyFound, pyErr := queryAccountSummariesPython(cleanDB)
		if pyErr == nil && pyFound {
			out = pyOut
			lastErr = nil
			ranOK = true
		} else if pyErr != nil {
			lastErr = pyErr
		}
	}

	if !ranOK {
		if lastErr != nil {
			return nil, fmt.Errorf("failed to read %s: %w", modernAccountsIndexName, lastErr)
		}
		return nil, fmt.Errorf("failed to read %s: no sqlite3 or python3 executable available", modernAccountsIndexName)
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		// sqlite3 -json prints nothing for an empty result set.
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

func queryAccountSummariesPython(cleanDB string) ([]byte, bool, error) {
	const pyScript = "import json,pathlib,sqlite3,sys\n" +
		"uri=pathlib.Path(sys.argv[1]).resolve().as_uri()+'?mode=ro'\n" +
		"conn=sqlite3.connect(uri,uri=True,timeout=3.0)\n" +
		"try:\n" +
		"    rows=conn.execute(sys.argv[2]).fetchall()\n" +
		"finally:\n" +
		"    conn.close()\n" +
		"sys.stdout.write(json.dumps([{'account_id':r[0],'email':r[1] or '','name':r[2] or ''} for r in rows]))\n"

	var lastErr error
	found := false
	for _, pyExe := range pythonCandidates() {
		p, lookErr := accountIndexLookPath(pyExe)
		if lookErr != nil || !isExecutableValid(p) {
			continue
		}
		found = true
		pyOut, pyErr := runCmdTimeout(5*time.Second, p, "-c", pyScript, cleanDB, accountSummariesSQL)
		if pyErr == nil {
			return pyOut, true, nil
		}
		lastErr = pyErr
	}
	return nil, found, lastErr
}

// queryModernStateCurrentID performs a best-effort read of `account_index_state`
// for the `current_id` field when current_account.json is missing or unmatched.
func queryModernStateCurrentID(rawDBPath string) string {
	cleanDB, pathErr := safeSQLitePathArg(rawDBPath)
	if pathErr != nil {
		return ""
	}

	var out []byte
	ranOK := false

	if p, lookErr := accountIndexLookPath("sqlite3"); lookErr == nil && isExecutableValid(p) {
		if raw, err := runCmdTimeout(5*time.Second, p, "-readonly", "-batch", "-noheader", "-cmd", ".timeout 3000", cleanDB, accountIndexStateSQL); err == nil {
			out = raw
			ranOK = true
		}
	}

	if !ranOK {
		const pyScript = "import pathlib,sqlite3,sys\n" +
			"uri=pathlib.Path(sys.argv[1]).resolve().as_uri()+'?mode=ro'\n" +
			"conn=sqlite3.connect(uri,uri=True,timeout=3.0)\n" +
			"try:\n" +
			"    row=conn.execute(sys.argv[2]).fetchone()\n" +
			"finally:\n" +
			"    conn.close()\n" +
			"if row and row[0]:\n" +
			"    sys.stdout.write(str(row[0]))\n"
		for _, pyExe := range pythonCandidates() {
			p, lookErr := accountIndexLookPath(pyExe)
			if lookErr != nil || !isExecutableValid(p) {
				continue
			}
			if raw, err := runCmdTimeout(5*time.Second, p, "-c", pyScript, cleanDB, accountIndexStateSQL); err == nil {
				out = raw
				break
			}
		}
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return ""
	}
	var doc struct {
		CurrentID string `json:"current_id"`
	}
	if json.Unmarshal([]byte(trimmed), &doc) != nil {
		return ""
	}
	return strings.TrimSpace(doc.CurrentID)
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

func matchAccountID(accounts []indexedAccount, id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	for _, acc := range accounts {
		if acc.ID == id {
			return acc.ID
		}
	}
	return ""
}
