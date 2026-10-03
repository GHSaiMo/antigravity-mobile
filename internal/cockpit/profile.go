package cockpit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var antigravityIdentityKeys = []string{
	"antigravityAuthStatus",
	"antigravityUnifiedStateSync.userStatus",
}

var validSQLiteKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

// validateSQLiteKey ensures a key string only contains standard alphanumeric, dot, underscore, or hyphen characters.
func validateSQLiteKey(key string) error {
	if !validSQLiteKeyPattern.MatchString(key) {
		return fmt.Errorf("invalid sqlite key: %q", key)
	}
	return nil
}

func antigravityStateDBPaths() []string {
	var paths []string
	if appData := os.Getenv("APPDATA"); appData != "" {
		paths = append(paths,
			filepath.Join(appData, "Antigravity", "User", "globalStorage", "state.vscdb"),
			filepath.Join(appData, "Antigravity IDE", "User", "globalStorage", "state.vscdb"),
		)
	}
	if home, err := os.UserHomeDir(); err == nil {
		if runtime.GOOS == "darwin" {
			paths = append(paths,
				filepath.Join(home, "Library", "Application Support", "Antigravity", "User", "globalStorage", "state.vscdb"),
				filepath.Join(home, "Library", "Application Support", "Antigravity IDE", "User", "globalStorage", "state.vscdb"),
			)
		} else if runtime.GOOS == "windows" {
			paths = append(paths,
				filepath.Join(home, "AppData", "Roaming", "Antigravity", "User", "globalStorage", "state.vscdb"),
				filepath.Join(home, "AppData", "Roaming", "Antigravity IDE", "User", "globalStorage", "state.vscdb"),
			)
		} else {
			configDir := os.Getenv("XDG_CONFIG_HOME")
			if configDir == "" {
				configDir = filepath.Join(home, ".config")
			}
			paths = append(paths,
				filepath.Join(configDir, "Antigravity", "User", "globalStorage", "state.vscdb"),
				filepath.Join(configDir, "Antigravity IDE", "User", "globalStorage", "state.vscdb"),
			)
		}
	}
	return paths
}

// ensureAntigravityStateDBs initializes the directories and SQLite state.vscdb
// with ItemTable if missing, ensuring Cockpit Tools' profile injection succeeds.
func ensureAntigravityStateDBs() {
	home, homeErr := os.UserHomeDir()
	if homeErr == nil && runtime.GOOS == "darwin" {
		agDir := filepath.Join(home, "Library", "Application Support", "Antigravity")
		agIdeDir := filepath.Join(home, "Library", "Application Support", "Antigravity IDE")
		if _, statErr := os.Stat(agDir); statErr == nil {
			if _, ideErr := os.Lstat(agIdeDir); os.IsNotExist(ideErr) {
				_ = os.Symlink("Antigravity", agIdeDir)
			}
		}
	}

	for _, dbPath := range antigravityStateDBPaths() {
		parentDir := filepath.Dir(dbPath)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			continue
		}
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			const initSQL = "CREATE TABLE IF NOT EXISTS ItemTable (key TEXT PRIMARY KEY, value TEXT);"
			if err := execSQLite(dbPath, initSQL); err != nil {
				log.Printf("[Cockpit] failed to initialize state.vscdb at %s: %v", dbPath, err)
			} else {
				log.Printf("[Cockpit] initialized state.vscdb at %s", dbPath)
			}
		}
	}
}

func sqliteQuote(s string) string {
	// SEC: Strip NULL bytes to prevent premature string truncation in SQLite
	cleaned := strings.ReplaceAll(s, "\x00", "")
	return "'" + strings.ReplaceAll(cleaned, "'", "''") + "'"
}

// prepareAntigravityProfileForSwitch runs after Antigravity has been quit and
// before Cockpit injects the new account. Do not wipe userStatus/authStatus:
// Cockpit's own UI switch leaves those keys in place, and deleting them leaves
// the workbench without a hydrated identity (black window). Only align the
// legacy Antigravity.app bind slot, which Cockpit otherwise leaves stale.
func prepareAntigravityProfileForSwitch(accountID string) (prevAccountID string) {
	prevAccountID = getLegacyBindAccount()
	syncLegacyBindAccount(accountID)
	return prevAccountID
}

func clearStaleAntigravityIdentity() {
	inList := make([]string, 0, len(antigravityIdentityKeys))
	for _, key := range antigravityIdentityKeys {
		if err := validateSQLiteKey(key); err != nil {
			log.Printf("[Cockpit] skipping invalid key %q: %v", key, err)
			continue
		}
		inList = append(inList, sqliteQuote(key))
	}
	if len(inList) == 0 {
		return
	}
	sql := "DELETE FROM ItemTable WHERE key IN (" + strings.Join(inList, ",") + ");"

	for _, dbPath := range antigravityStateDBPaths() {
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		if err := execSQLite(dbPath, sql); err != nil {
			log.Printf("[Cockpit] failed to clear identity keys in %s: %v", dbPath, err)
			continue
		}
		log.Printf("[Cockpit] cleared stale Antigravity identity keys in %s", dbPath)
	}
}

func execSQLite(dbPath, sql string) error {
	cleanDB := filepath.Clean(dbPath)
	if !strings.HasSuffix(cleanDB, ".vscdb") && !strings.HasSuffix(cleanDB, ".db") {
		return fmt.Errorf("invalid sqlite database path: %q", dbPath)
	}
	if _, lookErr := exec.LookPath("sqlite3"); lookErr == nil {
		cmd := exec.Command("sqlite3", cleanDB, sql)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	for _, py := range []string{"python", "python3"} {
		if _, lookErr := exec.LookPath(py); lookErr == nil {
			pyScript := "import sqlite3, sys; conn = sqlite3.connect(sys.argv[1]); conn.executescript(sys.argv[2]); conn.commit()"
			cmd := exec.Command(py, "-c", pyScript, cleanDB, sql)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
			}
			return nil
		}
	}
	return fmt.Errorf("neither sqlite3 nor python found to execute sqlite query")
}

func getLegacyBindAccount() string {
	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(dataDir, "antigravity_legacy_instances.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ""
	}
	ds, _ := doc["defaultSettings"].(map[string]any)
	if ds == nil {
		return ""
	}
	val, _ := ds["bindAccountId"].(string)
	return strings.TrimSpace(val)
}

func syncLegacyBindAccount(accountID string) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return
	}
	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return
	}
	path := filepath.Join(dataDir, "antigravity_legacy_instances.json")
	var doc map[string]any
	fileExists := false
	if raw, err := os.ReadFile(path); err == nil {
		fileExists = true
		if err := json.Unmarshal(raw, &doc); err != nil {
			log.Printf("[Cockpit] failed to parse %s: %v", path, err)
			return
		}
	}
	if doc == nil {
		doc = map[string]any{
			"instances": []any{},
			"defaultSettings": map[string]any{
				"bindAccountId": accountID,
			},
		}
	}
	ds, _ := doc["defaultSettings"].(map[string]any)
	if ds == nil {
		ds = map[string]any{}
		doc["defaultSettings"] = ds
	}
	prev, _ := ds["bindAccountId"].(string)
	if fileExists && prev == accountID {
		return
	}
	ds["bindAccountId"] = accountID
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0644); err != nil {
		log.Printf("[Cockpit] failed to write %s: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("[Cockpit] failed to replace %s: %v", path, err)
		return
	}
	log.Printf("[Cockpit] synced antigravity_legacy_instances bindAccountId %s -> %s", prev, accountID)
}

// wait is kept tiny so tests can override if needed.
var afterProfilePrepare = func() { time.Sleep(50 * time.Millisecond) }

func querySQLite(dbPath, sql string) (string, error) {
	cleanDB := filepath.Clean(dbPath)
	if _, lookErr := exec.LookPath("sqlite3"); lookErr == nil {
		cmd := exec.Command("sqlite3", "-batch", "-noheader", cleanDB, sql)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(out)), nil
	}
	pyScript := "import sqlite3, sys; conn = sqlite3.connect(sys.argv[1]); cur = conn.cursor(); cur.execute(sys.argv[2]); row = cur.fetchone(); sys.stdout.write(str(row[0]) if row and row[0] is not None else '')"
	for _, py := range []string{"python", "python3"} {
		if _, lookErr := exec.LookPath(py); lookErr == nil {
			cmd := exec.Command(py, "-c", pyScript, cleanDB, sql)
			out, err := cmd.CombinedOutput()
			if err == nil {
				return strings.TrimSpace(string(out)), nil
			}
		}
	}
	return "", fmt.Errorf("sqlite query runner not found")
}

// InjectAccountToAntigravityStateDB writes unified OAuth token protobuf and flags into state.vscdb.
func InjectAccountToAntigravityStateDB(acc *CockpitAccountDetail) error {
	if acc == nil {
		return fmt.Errorf("cannot inject nil account")
	}

	ensureAntigravityStateDBs()

	isGCP := false
	if acc.Token.IsGCPToS != nil {
		isGCP = *acc.Token.IsGCPToS
	}

	expiry := acc.Token.ExpiryTimestamp
	if expiry <= 0 {
		expiry = time.Now().Unix() + 3600
	}

	oauthPayload := CreateOAuthInfoWithMetadata(
		acc.Token.AccessToken,
		acc.Token.RefreshToken,
		expiry,
		isGCP,
		acc.Token.IDToken,
	)

	dbs := antigravityStateDBPaths()
	for _, dbPath := range dbs {
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}

		var currentTopic []byte
		if val, err := querySQLite(dbPath, "SELECT value FROM ItemTable WHERE key = 'antigravityUnifiedStateSync.oauthToken';"); err == nil && val != "" {
			if b, bErr := base64.StdEncoding.DecodeString(val); bErr == nil {
				currentTopic = b
			}
		}

		topic, _ := RemoveUnifiedTopicEntry(currentTopic, "oauthTokenInfoSentinelKey")
		topic, _ = RemoveUnifiedTopicEntry(topic, "authStateWithContextSentinelKey")

		entry := CreateUnifiedTopicEntry("oauthTokenInfoSentinelKey", oauthPayload)
		topic = append(topic, entry...)
		topicB64 := base64.StdEncoding.EncodeToString(topic)

		sql := fmt.Sprintf("INSERT OR REPLACE INTO ItemTable (key, value) VALUES ('antigravityUnifiedStateSync.oauthToken', %s);", sqliteQuote(topicB64))
		if err := execSQLite(dbPath, sql); err != nil {
			log.Printf("[Cockpit] failed to write unified oauthToken to %s: %v", dbPath, err)
		}

		// Inject minimal userStatus if missing
		userStatusCount, _ := querySQLite(dbPath, "SELECT COUNT(*) FROM ItemTable WHERE key = 'antigravityUnifiedStateSync.userStatus';")
		if userStatusCount == "0" || userStatusCount == "" {
			minPayload := CreateMinimalUserStatusPayload(acc.Email)
			userTopic := CreateUnifiedTopicEntry("userStatusSentinelKey", minPayload)
			userTopicB64 := base64.StdEncoding.EncodeToString(userTopic)
			uSQL := fmt.Sprintf("INSERT OR REPLACE INTO ItemTable (key, value) VALUES ('antigravityUnifiedStateSync.userStatus', %s);", sqliteQuote(userTopicB64))
			_ = execSQLite(dbPath, uSQL)
		}

		// Inject Onboarding flag
		_ = execSQLite(dbPath, "INSERT OR REPLACE INTO ItemTable (key, value) VALUES ('antigravityOnboarding', 'true');")
		log.Printf("[Cockpit] Injected account %s tokens into %s", acc.Email, dbPath)
	}
	return nil
}

// SyncCockpitLocalState updates accounts.json, current_account.json, and legacy instance binding.
func SyncCockpitLocalState(accountID, email string) error {
	accountID = strings.TrimSpace(accountID)
	email = strings.TrimSpace(email)
	if accountID == "" && email == "" {
		return nil
	}

	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return err
	}

	now := time.Now().Unix()

	// 1. Update current_account.json
	if email != "" {
		curDoc := map[string]any{
			"email":      email,
			"updated_at": now,
		}
		if raw, err := json.MarshalIndent(curDoc, "", "  "); err == nil {
			curPath := filepath.Join(dataDir, "current_account.json")
			tmpPath := curPath + ".tmp"
			if os.WriteFile(tmpPath, append(raw, '\n'), 0644) == nil {
				_ = os.Rename(tmpPath, curPath)
			}
		}
	}

	// 2. Update accounts.json (current_account_id & last_used)
	accPath := filepath.Join(dataDir, "accounts.json")
	if raw, err := os.ReadFile(accPath); err == nil {
		var idx map[string]any
		if json.Unmarshal(raw, &idx) == nil {
			if accountID != "" {
				idx["current_account_id"] = accountID
			}
			if accList, ok := idx["accounts"].([]any); ok {
				for _, item := range accList {
					if m, ok := item.(map[string]any); ok {
						if (accountID != "" && m["id"] == accountID) || (email != "" && strings.EqualFold(fmt.Sprint(m["email"]), email)) {
							m["last_used"] = now
						}
					}
				}
			}
			if updated, err := json.MarshalIndent(idx, "", "  "); err == nil {
				tmpPath := accPath + ".tmp"
				if os.WriteFile(tmpPath, append(updated, '\n'), 0644) == nil {
					_ = os.Rename(tmpPath, accPath)
				}
			}
		}
	}

	// 3. Update legacy bind account
	if accountID != "" {
		syncLegacyBindAccount(accountID)
	}
	return nil
}

