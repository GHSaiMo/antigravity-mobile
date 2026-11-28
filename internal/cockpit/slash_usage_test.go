package cockpit

import (
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGetSlashCommandUsageStats_PureGoAndLiveRecord(t *testing.T) {
	sqliteBin, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 not available")
	}

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	ResetSlashUsageCache()
	t.Cleanup(ResetSlashUsageCache)

	convDir := filepath.Join(tempHome, ".gemini", "antigravity", "conversations")
	if err := os.MkdirAll(convDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Simulate protobuf-like USER_INPUT (step_type=14) payloads and a non-user step (step_type=15).
	p0 := append([]byte{0x12, 0x20}, []byte("/grill-me 帮我拆解新对话方案")...)
	p1 := []byte("...builtin/skills/native_swarm/SKILL.md</SKILL>\x12\x10/swarm 并发执行")
	p2 := []byte(".../skills/mgy-growth-ops/SKILL.md</SKILL>")
	p3 := []byte("https://example.com/plan/doc and /Users/foo/plan should not count")
	pNonUser := []byte("/plan 这一条不是用户输入不应该计入")

	dbPath := filepath.Join(convDir, "conv-slash-1.db")
	sql := "CREATE TABLE steps (idx integer PRIMARY KEY, step_type integer NOT NULL DEFAULT 0, step_payload blob);" +
		"INSERT INTO steps (idx, step_type, step_payload) VALUES (0, 14, X'" + hex.EncodeToString(p0) + "');" +
		"INSERT INTO steps (idx, step_type, step_payload) VALUES (1, 14, X'" + hex.EncodeToString(p1) + "');" +
		"INSERT INTO steps (idx, step_type, step_payload) VALUES (2, 14, X'" + hex.EncodeToString(p2) + "');" +
		"INSERT INTO steps (idx, step_type, step_payload) VALUES (3, 14, X'" + hex.EncodeToString(p3) + "');" +
		"INSERT INTO steps (idx, step_type, step_payload) VALUES (4, 15, X'" + hex.EncodeToString(pNonUser) + "');"
	if out, err := exec.Command(sqliteBin, dbPath, sql).CombinedOutput(); err != nil {
		t.Fatalf("sqlite3 init failed: %v (%s)", err, string(out))
	}

	stats := GetSlashCommandUsageStats()
	if stats["grill-me"] != 1 {
		t.Errorf("grill-me = %d, want 1 (stats=%v)", stats["grill-me"], stats)
	}
	// native_swarm and /swarm in the same user turn must deduplicate to 1 hit.
	if stats["swarm"] != 1 {
		t.Errorf("swarm = %d, want 1 (stats=%v)", stats["swarm"], stats)
	}
	if stats["mgy-growth-ops"] != 1 {
		t.Errorf("mgy-growth-ops = %d, want 1 (stats=%v)", stats["mgy-growth-ops"], stats)
	}
	if stats["plan"] != 0 {
		t.Errorf("plan = %d, want 0 (URL/path/non-user must not count; stats=%v)", stats["plan"], stats)
	}

	// Live recording via gateway must immediately appear in stats.
	RecordSlashCommandUsage("/boost")
	RecordSlashCommandUsage("boost")
	stats = GetSlashCommandUsageStats()
	if stats["boost"] != 2 {
		t.Errorf("boost after live record = %d, want 2", stats["boost"])
	}
}
