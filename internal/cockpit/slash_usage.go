package cockpit

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	userInputStepType  = 14
	slashUsageScanTTL  = 2 * time.Minute
	maxSlashUsageFiles = 400
)

var (
	skillDefPathRe = regexp.MustCompile(`/skills/([a-zA-Z0-9_-]+)/SKILL\.md`)

	// Known Antigravity system slash commands eligible for history frequency tracking.
	knownSystemSlashCommands = []string{
		"plan",
		"grill-me",
		"boost",
		"goal",
		"teamwork",
		"swarm",
		"workflow",
		"learn",
		"automation",
		"compact",
		"btw",
	}

	// Map internal built-in skill directory names back to their user-facing slash command name.
	skillAliasToSlashName = map[string]string{
		"native_swarm": "swarm",
	}

	slashUsageMu        sync.Mutex
	slashUsageHome      string
	slashUsageLastScan  time.Time
	slashUsageFileCache = map[string]slashUsageFileEntry{}
	slashUsageLiveHits  = map[string]int{}
)

type slashUsageFileEntry struct {
	size    int64
	modNano int64
	walSize int64
	walMod  int64
	counts  map[string]int
}

// ResetSlashUsageCache clears the in-memory scan and live usage caches (used by tests).
func ResetSlashUsageCache() {
	slashUsageMu.Lock()
	defer slashUsageMu.Unlock()
	slashUsageHome = ""
	slashUsageLastScan = time.Time{}
	slashUsageFileCache = map[string]slashUsageFileEntry{}
	slashUsageLiveHits = map[string]int{}
}

// RecordSlashCommandUsage increments the live in-memory usage count for a slash command
// sent through the gateway so mobile/web clients see updated top-used rankings immediately.
func RecordSlashCommandUsage(name string) {
	clean := normalizeSlashUsageName(name)
	if clean == "" {
		return
	}
	slashUsageMu.Lock()
	defer slashUsageMu.Unlock()
	home, _ := os.UserHomeDir()
	if home != "" {
		if slashUsageHome == "" {
			slashUsageHome = home
		} else if slashUsageHome != home {
			slashUsageHome = home
			slashUsageLastScan = time.Time{}
			slashUsageFileCache = map[string]slashUsageFileEntry{}
			slashUsageLiveHits = map[string]int{}
		}
	}
	slashUsageLiveHits[clean]++
}

func normalizeSlashUsageName(raw string) string {
	name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(raw), "/"))
	if name == "" {
		return ""
	}
	if mapped, ok := skillAliasToSlashName[name]; ok {
		return mapped
	}
	return name
}

// GetSlashCommandUsageStats scans local Antigravity Desktop and CLI conversation SQLite DBs
// (pure-Go read-only, incrementally cached by file mtime/size) and merges live gateway hits.
func GetSlashCommandUsageStats() map[string]int {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return nil
	}

	slashUsageMu.Lock()
	defer slashUsageMu.Unlock()

	if slashUsageHome != home {
		slashUsageHome = home
		slashUsageLastScan = time.Time{}
		slashUsageFileCache = map[string]slashUsageFileEntry{}
		slashUsageLiveHits = map[string]int{}
	}

	if slashUsageLastScan.IsZero() || time.Since(slashUsageLastScan) >= slashUsageScanTTL {
		scanSlashUsageLocked(home)
		slashUsageLastScan = time.Now()
	}

	merged := make(map[string]int, len(slashUsageLiveHits)+16)
	for _, entry := range slashUsageFileCache {
		for k, v := range entry.counts {
			merged[k] += v
		}
	}
	for k, v := range slashUsageLiveHits {
		merged[k] += v
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func scanSlashUsageLocked(home string) {
	dirs := []string{
		filepath.Join(home, ".gemini", "antigravity", "conversations"),
		filepath.Join(home, ".gemini", "antigravity-cli", "conversations"),
	}

	seenPaths := make(map[string]bool)
	scanned := 0

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, ent := range entries {
			if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".db") {
				continue
			}
			if scanned >= maxSlashUsageFiles {
				break
			}
			scanned++
			dbPath := filepath.Join(dir, ent.Name())
			seenPaths[dbPath] = true

			info, err := ent.Info()
			if err != nil || info.Size() < 100 {
				continue
			}
			var walSize, walMod int64
			if walInfo, wErr := os.Stat(dbPath + "-wal"); wErr == nil {
				walSize = walInfo.Size()
				walMod = walInfo.ModTime().UnixNano()
			}

			modNano := info.ModTime().UnixNano()
			if prev, ok := slashUsageFileCache[dbPath]; ok &&
				prev.size == info.Size() &&
				prev.modNano == modNano &&
				prev.walSize == walSize &&
				prev.walMod == walMod {
				continue
			}

			counts, err := readUserStepSlashUsagePureGo(dbPath)
			if err != nil {
				continue
			}
			slashUsageFileCache[dbPath] = slashUsageFileEntry{
				size:    info.Size(),
				modNano: modNano,
				walSize: walSize,
				walMod:  walMod,
				counts:  counts,
			}
		}
	}

	// Evict removed DB files from cache.
	for p := range slashUsageFileCache {
		if !seenPaths[p] {
			delete(slashUsageFileCache, p)
		}
	}
}

func readUserStepSlashUsagePureGo(dbPath string) (map[string]int, error) {
	db, err := openPureSQLiteDB(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rootPage, createSQL, found, err := db.findTableSchema("steps")
	if err != nil || !found {
		return nil, err
	}

	stepTypeIdx := parseSQLiteTableColumnIndex(createSQL, "step_type")
	if stepTypeIdx < 0 {
		stepTypeIdx = 1
	}
	payloadIdx := parseSQLiteTableColumnIndex(createSQL, "step_payload")
	if payloadIdx < 0 {
		payloadIdx = 9
	}

	counts := make(map[string]int)
	visited := make(map[uint32]bool)
	err = db.walkTableBTree(rootPage, visited, func(row sqliteRow) error {
		if stepTypeIdx >= len(row.Values) || payloadIdx >= len(row.Values) {
			return nil
		}
		st, ok := asInt64(row.Values[stepTypeIdx])
		if !ok || st != userInputStepType {
			return nil
		}
		var payload []byte
		switch v := row.Values[payloadIdx].(type) {
		case []byte:
			payload = v
		case string:
			payload = []byte(v)
		default:
			return nil
		}
		if len(payload) == 0 {
			return nil
		}
		for _, cmd := range extractSlashCommandsFromUserPayload(payload) {
			counts[cmd]++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(counts) == 0 {
		return nil, nil
	}
	return counts, nil
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func isASCIIWordOrPathByte(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '_' || b == '-' || b == '.' || b == '/' || b == ':'
}

func isSlashTrailingBoundary(b byte) bool {
	return b == ' ' || b == '\n' || b == '\r' || b == '\t' || b < 0x20
}

// extractSlashCommandsFromUserPayload returns the deduplicated slash command / skill names
// invoked in a single USER_INPUT protobuf payload.
func extractSlashCommandsFromUserPayload(payload []byte) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(raw string) {
		name := normalizeSlashUsageName(raw)
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, name)
	}

	// 1. Structured skill definitionPath markers (.../skills/<name>/SKILL.md).
	for _, m := range skillDefPathRe.FindAllSubmatch(payload, -1) {
		if len(m) >= 2 {
			add(string(m[1]))
		}
	}

	// 2. Leading or whitespace-delimited /<system-cmd> tokens in userResponse/items.
	for _, cmd := range knownSystemSlashCommands {
		needle := []byte("/" + cmd)
		offset := 0
		for {
			idx := bytes.Index(payload[offset:], needle)
			if idx < 0 {
				break
			}
			absIdx := offset + idx
			endIdx := absIdx + len(needle)
			offset = endIdx

			// Ensure the byte preceding "/" is not a path/URL/identifier character.
			// Note: In protobuf wire format, a length-delimited string starts with a varint length byte,
			// which may be a printable ASCII byte (e.g. 0x62 'b' for len 98) right after tag 0x12/0x0a.
			prevOK := absIdx == 0 || !isASCIIWordOrPathByte(payload[absIdx-1])
			if !prevOK && absIdx >= 2 && (payload[absIdx-2] == 0x12 || payload[absIdx-2] == 0x0a || payload[absIdx-2] == 0x1a) {
				prevOK = true
			}
			if !prevOK {
				continue
			}
			if endIdx < len(payload) && !isSlashTrailingBoundary(payload[endIdx]) {
				continue
			}
			add(cmd)
			break
		}
	}
	return out
}
