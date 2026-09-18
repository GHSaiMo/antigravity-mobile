package cockpit

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, "Library", "Application Support", "Antigravity", "User", "globalStorage", "state.vscdb"),
		filepath.Join(home, "Library", "Application Support", "Antigravity IDE", "User", "globalStorage", "state.vscdb"),
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
func prepareAntigravityProfileForSwitch(accountID string) {
	syncLegacyBindAccount(accountID)
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
	cmd := exec.Command("sqlite3", cleanDB, sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
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
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		log.Printf("[Cockpit] failed to parse %s: %v", path, err)
		return
	}
	ds, _ := doc["defaultSettings"].(map[string]any)
	if ds == nil {
		ds = map[string]any{}
		doc["defaultSettings"] = ds
	}
	prev, _ := ds["bindAccountId"].(string)
	if prev == accountID {
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
