package cockpit

import (
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCalculateCostUSD(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		input     int64
		cacheRead int64
		output    int64
		wantMin   float64
		wantMax   float64
	}{
		{
			name:      "Claude Sonnet 1M tokens",
			model:     "claude-3-5-sonnet",
			input:     1000000,
			cacheRead: 500000,
			output:    200000,
			// 1M * 3.0 + 0.5M * 0.3 + 0.2M * 15.0 = 3.0 + 0.15 + 3.0 = 6.15
			wantMin: 6.14,
			wantMax: 6.16,
		},
		{
			name:      "Gemini Flash 1M tokens",
			model:     "gemini-2.5-flash",
			input:     1000000,
			cacheRead: 1000000,
			output:    100000,
			// 1M * 0.075 + 1M * 0.01875 + 0.1M * 0.3 = 0.075 + 0.01875 + 0.03 = 0.12375
			wantMin: 0.12,
			wantMax: 0.13,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateCostUSD(tt.model, tt.input, tt.cacheRead, tt.output)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("CalculateCostUSD() = %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestParseTranscriptFile(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "transcript.jsonl")

	content := `{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-11-13T10:00:00Z","content":"hello"}
{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-13T10:00:05Z","input_tokens":1000,"cache_read_tokens":200,"output_tokens":300,"callId":"call-1"}
{"step_index":2,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-13T10:05:00Z","input_tokens":500,"cache_read_tokens":0,"output_tokens":100,"callId":"call-2"}
`
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	records := parseTranscriptFile(logFile, "desktop")
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}

	if records[0].total != 1500 {
		t.Errorf("record[0] total = %d, want 1500", records[0].total)
	}
	if records[0].dateKey != "2026-11-13" {
		t.Errorf("record[0] dateKey = %s, want 2026-11-13", records[0].dateKey)
	}
	if records[1].total != 600 {
		t.Errorf("record[1] total = %d, want 600", records[1].total)
	}
}

func TestParseTranscriptFile_TimezoneLocal(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "transcript.jsonl")

	// 2026-11-13T23:30:00Z is 2026-11-14 07:30:00 in UTC+8
	content := `{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-13T23:30:00Z","input_tokens":100,"output_tokens":50,"callId":"c-1"}
`
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	records := parseTranscriptFile(logFile, "desktop")
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	// Verify that dateKey matches t.Local().Format("2006-01-02")
	parsed, _ := time.Parse(time.RFC3339, "2026-11-13T23:30:00Z")
	expectedDate := parsed.Local().Format("2006-01-02")
	if records[0].dateKey != expectedDate {
		t.Errorf("record[0] dateKey = %s, want %s", records[0].dateKey, expectedDate)
	}
}

func TestParseModelFromUserSettings(t *testing.T) {
	tests := []struct {
		name     string
		raw      map[string]interface{}
		expected string
	}{
		{
			name: "Gemini 3.8 Flash (High) with version dot",
			raw: map[string]interface{}{
				"type": "USER_INPUT",
				"content": "<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection` from None to Gemini 3.8 Flash (High). No need to comment on this change if the user doesn't ask about it. If reporting what model you are, please use a human readable name instead of the exact string.\n</USER_SETTINGS_CHANGE>",
			},
			expected: "Gemini 3.8 Flash (High)",
		},
		{
			name: "Gemini 4 Hydrogen (Medium) without version dot",
			raw: map[string]interface{}{
				"type": "USER_INPUT",
				"content": "<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection` from None to Gemini 4 Hydrogen (Medium). No need to comment on this change if the user doesn't ask about it.\n</USER_SETTINGS_CHANGE>",
			},
			expected: "Gemini 4 Hydrogen (Medium)",
		},
		{
			name: "Claude Sonnet 4.6 (Thinking)",
			raw: map[string]interface{}{
				"type": "USER_INPUT",
				"content": "<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection` from Gemini 3.8 Flash (High) to Claude Sonnet 4.6 (Thinking). No need to comment on this change.\n</USER_SETTINGS_CHANGE>",
			},
			expected: "Claude Sonnet 4.6 (Thinking)",
		},
		{
			name: "Prevent code leakage in PLANNER_RESPONSE",
			raw: map[string]interface{}{
				"type": "PLANNER_RESPONSE",
				"content": `if idx := strings.Index(content, "Model Selection` + "` from None to \"); idx != -1 {\n rem := content[idx+len(\"Model Selection`" + ` from None to \"):];`,
			},
			expected: "",
		},
		{
			name: "Prevent code leakage in GENERIC tool output",
			raw: map[string]interface{}{
				"type": "GENERIC",
				"content": `<USER_SETTINGS_CHANGE>\nThe user changed setting ` + "`Model Selection` from None to \"); idx != -1 { foo(); };\n</USER_SETTINGS_CHANGE>",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseModelFromUserSettings(tt.raw)
			if got != tt.expected {
				t.Errorf("parseModelFromUserSettings() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGetTokenHistory_PreservesDeletedSessionsAfterScan(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("COCKPIT_TOOLS_DATA_DIR", filepath.Join(tempHome, ".cockpit_tools"))
	t.Setenv("COCKPIT_DATA_DIR", "")

	conv1Dir := filepath.Join(tempHome, ".gemini", "antigravity", "brain", "conv-1", ".system_generated", "logs")
	conv2Dir := filepath.Join(tempHome, ".gemini", "antigravity-cli", "brain", "conv-2", ".system_generated", "logs")
	if err := os.MkdirAll(conv1Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(conv2Dir, 0755); err != nil {
		t.Fatal(err)
	}

	log1 := `{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-14T10:00:00Z","input_tokens":1000,"cache_read_tokens":500,"output_tokens":200,"callId":"c-1","chatModel":{"model":"MODEL_PLACEHOLDER_M525"}}` + "\n"
	log2 := `{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-14T10:05:00Z","input_tokens":300,"cache_read_tokens":0,"output_tokens":100,"callId":"c-2","chatModel":{"model":"MODEL_PLACEHOLDER_M318"}}` + "\n"

	if err := os.WriteFile(filepath.Join(conv1Dir, "transcript.jsonl"), []byte(log1), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conv2Dir, "transcript.jsonl"), []byte(log2), 0644); err != nil {
		t.Fatal(err)
	}

	resp, err := GetTokenHistory(true)
	if err != nil {
		t.Fatalf("GetTokenHistory() error = %v", err)
	}
	if resp.Summary.TotalTokens != 2100 {
		t.Fatalf("initial TotalTokens = %d, want 2100", resp.Summary.TotalTokens)
	}
	if resp.Summary.DesktopTokens != 1700 || resp.Summary.CLITokens != 400 {
		t.Fatalf("initial breakdown desktop=%d cli=%d, want 1700/400", resp.Summary.DesktopTokens, resp.Summary.CLITokens)
	}

	// Delete conv-1 from disk completely
	if err := os.RemoveAll(filepath.Join(tempHome, ".gemini", "antigravity", "brain", "conv-1")); err != nil {
		t.Fatal(err)
	}

	// Append a new step to conv-2 to verify live updates don't double-count existing steps
	log2Updated := log2 + `{"step_index":2,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-14T10:10:00Z","input_tokens":200,"cache_read_tokens":0,"output_tokens":50,"callId":"c-3"}` + "\n"
	if err := os.WriteFile(filepath.Join(conv2Dir, "transcript.jsonl"), []byte(log2Updated), 0644); err != nil {
		t.Fatal(err)
	}

	respAfter, err := GetTokenHistory(true)
	if err != nil {
		t.Fatalf("GetTokenHistory(after delete) error = %v", err)
	}
	// 1700 (archived conv-1) + 650 (updated conv-2) = 2350
	if respAfter.Summary.TotalTokens != 2350 {
		t.Fatalf("after delete TotalTokens = %d, want 2350", respAfter.Summary.TotalTokens)
	}
	if respAfter.Summary.DesktopTokens != 1700 || respAfter.Summary.CLITokens != 650 {
		t.Fatalf("after delete breakdown desktop=%d cli=%d, want 1700/650", respAfter.Summary.DesktopTokens, respAfter.Summary.CLITokens)
	}
}

func TestArchiveConversationTokens_BeforeDelete(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("COCKPIT_TOOLS_DATA_DIR", filepath.Join(tempHome, ".cockpit_tools"))
	t.Setenv("COCKPIT_DATA_DIR", "")

	convDir := filepath.Join(tempHome, ".gemini", "antigravity", "brain", "conv-arch-1", ".system_generated", "logs")
	if err := os.MkdirAll(convDir, 0755); err != nil {
		t.Fatal(err)
	}

	logContent := `{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-14T11:00:00Z","input_tokens":800,"cache_read_tokens":100,"output_tokens":100,"callId":"arch-1","chatModel":{"model":"MODEL_PLACEHOLDER_M427"}}` + "\n"
	if err := os.WriteFile(filepath.Join(convDir, "transcript.jsonl"), []byte(logContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Archive directly without prior GetTokenHistory scan, then delete from disk
	ArchiveConversationTokens("conv-arch-1")
	if err := os.RemoveAll(filepath.Join(tempHome, ".gemini", "antigravity", "brain", "conv-arch-1")); err != nil {
		t.Fatal(err)
	}

	resp, err := GetTokenHistory(false)
	if err != nil {
		t.Fatalf("GetTokenHistory() error = %v", err)
	}
	if resp.Summary.TotalTokens != 1000 {
		t.Fatalf("TotalTokens = %d, want 1000", resp.Summary.TotalTokens)
	}
	if resp.Summary.DesktopTokens != 1000 {
		t.Fatalf("DesktopTokens = %d, want 1000", resp.Summary.DesktopTokens)
	}
}

func TestGetTokenHistory_LargeJSONLLineOver64KB(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("COCKPIT_TOOLS_DATA_DIR", filepath.Join(tempHome, ".cockpit_tools"))
	t.Setenv("COCKPIT_DATA_DIR", "")

	convDir := filepath.Join(tempHome, ".gemini", "antigravity", "brain", "conv-huge-line", ".system_generated", "logs")
	if err := os.MkdirAll(convDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Build a single JSONL line > 120KB (nearly 2x the old 64KB ReadLine limit)
	padding := make([]byte, 120*1024)
	for i := range padding {
		padding[i] = 'a'
	}
	hugeLine := `{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-15T01:00:00Z","content":"` +
		string(padding) +
		`","input_tokens":5000,"cache_read_tokens":2000,"output_tokens":3000,"callId":"huge-1","chatModel":{"model":"MODEL_PLACEHOLDER_M525"}}` + "\n"

	if err := os.WriteFile(filepath.Join(convDir, "transcript.jsonl"), []byte(hugeLine), 0644); err != nil {
		t.Fatal(err)
	}

	resp, err := GetTokenHistory(true)
	if err != nil {
		t.Fatalf("GetTokenHistory() error = %v", err)
	}
	if resp.Summary.TotalTokens != 10000 {
		t.Fatalf("TotalTokens for >64KB JSONL line = %d, want 10000", resp.Summary.TotalTokens)
	}
}

func encodeTestPBString(fieldNum int, s string) []byte {
	return encodeTestPBBytes(fieldNum, []byte(s))
}

func encodeTestVarint(v uint64) []byte {
	var out []byte
	for {
		if v < 0x80 {
			out = append(out, byte(v))
			break
		}
		out = append(out, byte((v&0x7f)|0x80))
		v >>= 7
	}
	return out
}

func encodeTestPBBytes(fieldNum int, b []byte) []byte {
	var out []byte
	out = append(out, encodeTestVarint(uint64((fieldNum<<3)|2))...)
	out = append(out, encodeTestVarint(uint64(len(b)))...)
	out = append(out, b...)
	return out
}

func encodeTestGenMetadataBlob(slug, modelEnum, displayName, lastStepIndex string) []byte {
	var inner []byte
	if slug != "" {
		inner = append(inner, encodeTestPBString(19, slug)...)
	}
	if modelEnum != "" {
		kv := append(encodeTestPBString(1, "model_enum"), encodeTestPBString(2, modelEnum)...)
		inner = append(inner, encodeTestPBBytes(20, kv)...)
	}
	if lastStepIndex != "" {
		kv := append(encodeTestPBString(1, "last_step_index"), encodeTestPBString(2, lastStepIndex)...)
		inner = append(inner, encodeTestPBBytes(20, kv)...)
	}
	if displayName != "" {
		inner = append(inner, encodeTestPBString(21, displayName)...)
	}
	return encodeTestPBBytes(1, inner)
}

func TestParseGenMetadataBlob(t *testing.T) {
	blob1 := encodeTestGenMetadataBlob("gemini-4-carbon", "MODEL_PLACEHOLDER_M427", "Gemini 4 Carbon (High)", "0")
	step1, name1 := parseGenMetadataBlob(blob1)
	if step1 != 0 || name1 != "Gemini 4 Carbon (High)" {
		t.Fatalf("blob1 = (%d, %q), want (0, %q)", step1, name1, "Gemini 4 Carbon (High)")
	}

	blob2 := encodeTestGenMetadataBlob("claude-opus-4-6-thinking", "MODEL_PLACEHOLDER_M26", "", "12")
	step2, name2 := parseGenMetadataBlob(blob2)
	if step2 != 12 || name2 != "Claude Opus 4.6 (Thinking)" {
		t.Fatalf("blob2 = (%d, %q), want (12, %q)", step2, name2, "Claude Opus 4.6 (Thinking)")
	}

	blob3 := encodeTestGenMetadataBlob("gemini-3.8-flash-tiered", "MODEL_PLACEHOLDER_M322", "", "4")
	step3, name3 := parseGenMetadataBlob(blob3)
	if step3 != 4 || name3 != "Gemini 3.8 Flash" {
		t.Fatalf("blob3 = (%d, %q), want (4, %q)", step3, name3, "Gemini 3.8 Flash")
	}
}

func TestGetTokenHistory_ResolvesSubagentModelFromConversationDB(t *testing.T) {
	sqliteBin, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 not available")
	}

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("COCKPIT_TOOLS_DATA_DIR", filepath.Join(tempHome, ".cockpit_tools"))
	t.Setenv("COCKPIT_DATA_DIR", "")

	convID := "subagent-db-1"
	logDir := filepath.Join(tempHome, ".gemini", "antigravity", "brain", convID, ".system_generated", "logs")
	convDBDir := filepath.Join(tempHome, ".gemini", "antigravity", "conversations")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(convDBDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Subagent transcript starts with SYSTEM_MESSAGE (no USER_SETTINGS_CHANGE / chatModel).
	transcript := `{"step_index":0,"source":"SYSTEM","type":"SYSTEM_MESSAGE","status":"DONE","created_at":"2026-11-15T10:00:00Z","content":"[Message] sender=parent-conv-1234567890"}` + "\n" +
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-15T10:00:01Z","input_tokens":1000,"cache_read_tokens":500,"output_tokens":200,"callId":"sub-c1"}` + "\n" +
		`{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-15T10:00:05Z","input_tokens":400,"cache_read_tokens":0,"output_tokens":100,"callId":"sub-c2"}` + "\n"
	transcriptPath := filepath.Join(logDir, "transcript.jsonl")
	if err := os.WriteFile(transcriptPath, []byte(transcript), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a stale v1 ledger with "default" to verify automatic backfill when Version < 2.
	dataDir := filepath.Join(tempHome, ".cockpit_tools")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(transcriptPath)
	if err != nil {
		t.Fatal(err)
	}
	staleLedger := `{"version":1,"sessions":{"desktop:subagent-db-1":{"source":"desktop","size":` +
		strconvFormatInt(st.Size()) + `,"mod_time_nano":` + strconvFormatInt(st.ModTime().UnixNano()) +
		`,"days":{"2026-11-15":{"date":"2026-11-15","total":2200,"input":1400,"output":300,"cache_read":500,"thinking":0,"steps":2,"cost_usd":0.003,"models":{"default":2200}}}}}}`
	if err := os.WriteFile(filepath.Join(dataDir, "token_history_ledger.json"), []byte(staleLedger), 0600); err != nil {
		t.Fatal(err)
	}
	// Reset in-memory ledger cache for clean test isolation.
	historyMu.Lock()
	cachedLedger = nil
	cachedLedgerPath = ""
	cachedHistoryResp = nil
	historyMu.Unlock()

	blob0 := hex.EncodeToString(encodeTestGenMetadataBlob("gemini-4-carbon", "MODEL_PLACEHOLDER_M427", "Gemini 4 Carbon (High)", "0"))
	blob1 := hex.EncodeToString(encodeTestGenMetadataBlob("gemini-4-hydrogen", "MODEL_PLACEHOLDER_M525", "Gemini 4 Hydrogen (High)", "2"))
	dbPath := filepath.Join(convDBDir, convID+".db")
	sql := "CREATE TABLE gen_metadata (idx integer PRIMARY KEY, data blob, size integer NOT NULL DEFAULT 0);" +
		"INSERT INTO gen_metadata (idx, data, size) VALUES (0, X'" + blob0 + "', 10);" +
		"INSERT INTO gen_metadata (idx, data, size) VALUES (1, X'" + blob1 + "', 10);"
	if out, err := exec.Command(sqliteBin, dbPath, sql).CombinedOutput(); err != nil {
		t.Fatalf("sqlite3 init failed: %v (%s)", err, string(out))
	}

	resp, err := GetTokenHistory(false)
	if err != nil {
		t.Fatalf("GetTokenHistory() error = %v", err)
	}
	if len(resp.DailyMatrix) != 1 {
		t.Fatalf("DailyMatrix len = %d, want 1", len(resp.DailyMatrix))
	}
	models := resp.DailyMatrix[0].Models
	if models["default"] != 0 {
		t.Fatalf("expected no default model, got %v", models)
	}
	if models["Gemini 4 Carbon (High)"] != 1700 {
		t.Errorf("Gemini 4 Carbon (High) = %d, want 1700 (models=%v)", models["Gemini 4 Carbon (High)"], models)
	}
	if models["Gemini 4 Hydrogen (High)"] != 500 {
		t.Errorf("Gemini 4 Hydrogen (High) = %d, want 500 (models=%v)", models["Gemini 4 Hydrogen (High)"], models)
	}
}

func TestGetTokenHistory_ResolvesSubagentModelFromParentFallback(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("COCKPIT_TOOLS_DATA_DIR", filepath.Join(tempHome, ".cockpit_tools"))
	t.Setenv("COCKPIT_DATA_DIR", "")

	parentID := "parent-conv-abcdef0123456789"
	childID := "child-conv-abcdef0123456789"

	parentLogDir := filepath.Join(tempHome, ".gemini", "antigravity", "brain", parentID, ".system_generated", "logs")
	childLogDir := filepath.Join(tempHome, ".gemini", "antigravity", "brain", childID, ".system_generated", "logs")
	if err := os.MkdirAll(parentLogDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(childLogDir, 0755); err != nil {
		t.Fatal(err)
	}

	parentFull := `{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","content":"<USER_SETTINGS_CHANGE>\nThe user changed setting ` + "`Model Selection`" + ` from None to Gemini 4 Carbon (High).\n</USER_SETTINGS_CHANGE>"}` + "\n" +
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","tool_calls":[{"name":"invoke_subagent","args":{"Model":"flash","Role":"Browser","TypeName":"browser"}}]}` + "\n" +
		`{"step_index":2,"source":"SYSTEM","type":"GENERIC","content":"Created the following subagent:\n{\"conversationId\":\"` + childID + `\"}"}` + "\n"
	if err := os.WriteFile(filepath.Join(parentLogDir, "transcript_full.jsonl"), []byte(parentFull), 0644); err != nil {
		t.Fatal(err)
	}

	childLog := `{"step_index":0,"source":"SYSTEM","type":"SYSTEM_MESSAGE","status":"DONE","created_at":"2026-11-15T10:00:00Z","content":"[Message] sender=` + parentID + ` content=check"}` + "\n" +
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-15T10:00:01Z","input_tokens":800,"cache_read_tokens":0,"output_tokens":200,"callId":"ch-1"}` + "\n"
	if err := os.WriteFile(filepath.Join(childLogDir, "transcript.jsonl"), []byte(childLog), 0644); err != nil {
		t.Fatal(err)
	}

	historyMu.Lock()
	cachedLedger = nil
	cachedLedgerPath = ""
	cachedHistoryResp = nil
	historyMu.Unlock()

	resp, err := GetTokenHistory(true)
	if err != nil {
		t.Fatalf("GetTokenHistory() error = %v", err)
	}
	if len(resp.DailyMatrix) != 1 {
		t.Fatalf("DailyMatrix len = %d, want 1", len(resp.DailyMatrix))
	}
	models := resp.DailyMatrix[0].Models
	if models["default"] != 0 || models["Gemini 3.8 Flash"] != 1000 {
		t.Fatalf("expected Gemini 3.8 Flash=1000 without default, got %v", models)
	}
}

func TestGetTokenHistory_NeverEmitsDefaultAndCleansLegacyDefaultLedger(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	cockpitDir := filepath.Join(tempHome, ".cockpit_tools")
	t.Setenv("COCKPIT_TOOLS_DATA_DIR", cockpitDir)
	t.Setenv("COCKPIT_DATA_DIR", "")

	// 1. Seed a legacy ledger with:
	//    - deleted session whose .db still exists -> should reclassify "default" to real model
	//    - deleted session whose files are completely gone -> should drop "default"
	dbConvID := "11111111-1111-1111-1111-111111111111"
	goneConvID := "22222222-2222-2222-2222-222222222222"

	sqliteBin, lookErr := exec.LookPath("sqlite3")
	if lookErr != nil {
		t.Skip("sqlite3 not available")
	}

	convsDir := filepath.Join(tempHome, ".gemini", "antigravity", "conversations")
	if err := os.MkdirAll(convsDir, 0755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(convsDir, dbConvID+".db")
	blob := hex.EncodeToString(encodeTestGenMetadataBlob("gemini-4-carbon", "MODEL_PLACEHOLDER_M427", "Gemini 4 Carbon (High)", "0"))
	initSQL := "CREATE TABLE gen_metadata (idx integer PRIMARY KEY, data blob, size integer NOT NULL DEFAULT 0);" +
		"INSERT INTO gen_metadata (idx, data, size) VALUES (0, X'" + blob + "', 10);"
	if out, err := exec.Command(sqliteBin, dbPath, initSQL).CombinedOutput(); err != nil {
		t.Fatalf("sqlite3 init failed: %v (%s)", err, string(out))
	}

	// Live transcript with mid-stream Model Selection (turns before selection should backfill)
	liveConvID := "33333333-3333-3333-3333-333333333333"
	liveDir := filepath.Join(tempHome, ".gemini", "antigravity", "brain", liveConvID, ".system_generated", "logs")
	if err := os.MkdirAll(liveDir, 0755); err != nil {
		t.Fatal(err)
	}
	liveLog := `{"step_index":0,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-15T10:00:00Z","input_tokens":500,"cache_read_tokens":0,"output_tokens":100,"callId":"l-1"}` + "\n" +
		`{"step_index":1,"source":"USER_EXPLICIT","type":"USER_INPUT","content":"<USER_SETTINGS_CHANGE>\nThe user changed setting ` + "`Model Selection`" + ` from None to Gemini 4 Hydrogen (High).\n</USER_SETTINGS_CHANGE>"}` + "\n"

	if err := os.WriteFile(filepath.Join(liveDir, "transcript.jsonl"), []byte(liveLog), 0644); err != nil {
		t.Fatal(err)
	}

	// Live transcript with zero model info anywhere -> must NOT produce "default"
	orphanConvID := "44444444-4444-4444-4444-444444444444"
	orphanDir := filepath.Join(tempHome, ".gemini", "antigravity", "brain", orphanConvID, ".system_generated", "logs")
	if err := os.MkdirAll(orphanDir, 0755); err != nil {
		t.Fatal(err)
	}
	orphanLog := `{"step_index":0,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-15T10:05:00Z","input_tokens":900,"cache_read_tokens":0,"output_tokens":100,"callId":"o-1"}` + "\n"
	if err := os.WriteFile(filepath.Join(orphanDir, "transcript.jsonl"), []byte(orphanLog), 0644); err != nil {
		t.Fatal(err)
	}

	legacyLedger := tokenHistoryLedger{
		Version: 2,
		Sessions: map[string]ledgerSession{
			"desktop:" + dbConvID: {
				Source: "desktop",
				Days: map[string]DailyTokenRecord{
					"2026-11-15": {
						Date:   "2026-11-15",
						Total:  1200,
						Input:  1000,
						Output: 200,
						Steps:  1,
						Models: map[string]int64{"default": 1200},
					},
				},
			},
			"desktop:" + goneConvID: {
				Source: "desktop",
				Days: map[string]DailyTokenRecord{
					"2026-11-15": {
						Date:   "2026-11-15",
						Total:  5000,
						Input:  4000,
						Output: 1000,
						Steps:  2,
						Models: map[string]int64{"default": 5000},
					},
				},
			},
		},
	}
	if err := saveTokenHistoryLedger(filepath.Join(cockpitDir, "token_history_ledger.json"), &legacyLedger); err != nil {
		t.Fatal(err)
	}

	historyMu.Lock()
	cachedLedger = nil
	cachedLedgerPath = ""
	cachedHistoryResp = nil
	historyMu.Unlock()

	resp, err := GetTokenHistory(true)
	if err != nil {
		t.Fatalf("GetTokenHistory() error = %v", err)
	}
	if len(resp.DailyMatrix) != 1 {
		t.Fatalf("DailyMatrix len = %d, want 1", len(resp.DailyMatrix))
	}
	models := resp.DailyMatrix[0].Models
	if _, hasDefault := models["default"]; hasDefault {
		t.Fatalf("expected no default bucket, got %v", models)
	}
	if models["Gemini 4 Carbon (High)"] != 1200 {
		t.Fatalf("expected reclassified dbConvID=1200, got %v", models)
	}
	if models["Gemini 4 Hydrogen (High)"] != 600 {
		t.Fatalf("expected backfilled liveConvID=600, got %v", models)
	}
	if resp.Summary.TotalTokens != 1800 {
		t.Fatalf("expected TotalTokens=1800 (dropped goneConvID & orphanConvID), got %d", resp.Summary.TotalTokens)
	}
}

func TestTokenHistory_RejectsUnsafeConversationIdentifiers(t *testing.T) {
	for _, bad := range []string{
		"",
		"../outside-1234567890",
		"..\\outside-1234567890",
		"a\\..\\outside-1234567890",
		"-init-flag-1234567890",
		"conv' OR 1=1--123456",
	} {
		if isSafeConvID(bad) {
			t.Fatalf("expected isSafeConvID(%q) = false", bad)
		}
		got := parseParentConvIDFromSystemMessage(map[string]interface{}{
			"type":    "SYSTEM_MESSAGE",
			"content": "[Message] sender=" + bad + " content=hello",
		})
		if got != "" {
			t.Fatalf("expected parseParentConvIDFromSystemMessage(%q) = empty, got %q", bad, got)
		}
	}

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	cockpitDir := filepath.Join(tempHome, ".cockpit_tools")
	t.Setenv("COCKPIT_TOOLS_DATA_DIR", cockpitDir)
	t.Setenv("COCKPIT_DATA_DIR", "")

	// Seed a transcript outside brain/ (at ~/.gemini/antigravity/outside/transcript.jsonl)
	outsideDir := filepath.Join(tempHome, ".gemini", "antigravity", "outside")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logLine := `{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-15T12:00:00Z","input_tokens":500,"output_tokens":100,"callId":"bad-1","chatModel":{"model":"MODEL_PLACEHOLDER_M525"}}` + "\n"
	if err := os.WriteFile(filepath.Join(outsideDir, "transcript.jsonl"), []byte(logLine), 0o644); err != nil {
		t.Fatal(err)
	}

	historyMu.Lock()
	cachedLedger = nil
	cachedLedgerPath = ""
	cachedHistoryResp = nil
	historyMu.Unlock()

	ArchiveConversationTokens("../outside")
	if _, err := os.Stat(filepath.Join(cockpitDir, "token_history_ledger.json")); err == nil {
		t.Fatalf("expected traversal cascadeID not to create token_history_ledger.json")
	}
}

func TestGetTokenHistory_StreamingMemoizesSiblingSubagentsAndBatchSummariesDB(t *testing.T) {
	sqliteBin, lookErr := exec.LookPath("sqlite3")
	if lookErr != nil {
		t.Skip("sqlite3 not available")
	}

	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("COCKPIT_TOOLS_DATA_DIR", filepath.Join(tempHome, ".cockpit_tools"))
	t.Setenv("COCKPIT_DATA_DIR", "")

	parentID := "parent-swarm-abcdef0123456789"
	childFlash := "child-flash-abcdef0123456789"
	childInherit := "child-inherit-abcdef01234567"
	childDBLinked := "child-dblink-abcdef012345678"

	for _, cid := range []string{parentID, childFlash, childInherit, childDBLinked} {
		logDir := filepath.Join(tempHome, ".gemini", "antigravity", "brain", cid, ".system_generated", "logs")
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	parentFull := `{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","content":"<USER_SETTINGS_CHANGE>\nThe user changed setting ` + "`Model Selection`" + ` from None to Gemini 4 Carbon (High).\n</USER_SETTINGS_CHANGE>"}` + "\n" +
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","tool_calls":[{"name":"invoke_subagent","args":{"Model":"flash"}}]}` + "\n" +
		`{"step_index":2,"source":"SYSTEM","type":"GENERIC","content":"Created the following subagent:\n{\"conversationId\":\"` + childFlash + `\"}"}` + "\n" +
		`{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","tool_calls":[{"name":"invoke_subagent","args":{"Model":"inherit"}}]}` + "\n" +
		`{"step_index":4,"source":"SYSTEM","type":"GENERIC","content":"Created the following subagent:\n{\"conversationId\":\"` + childInherit + `\"}"}` + "\n" +
		`{"step_index":5,"source":"MODEL","type":"PLANNER_RESPONSE","tool_calls":[{"name":"invoke_subagent","args":{"Model":"claude-sonnet-4-6"}}]}` + "\n" +
		`{"step_index":6,"source":"SYSTEM","type":"GENERIC","content":"Created the following subagent:\n{\"conversationId\":\"` + childDBLinked + `\"}"}` + "\n"

	parentLogDir := filepath.Join(tempHome, ".gemini", "antigravity", "brain", parentID, ".system_generated", "logs")
	if err := os.WriteFile(filepath.Join(parentLogDir, "transcript_full.jsonl"), []byte(parentFull), 0o644); err != nil {
		t.Fatal(err)
	}

	// childFlash & childInherit declare sender=parentID via SYSTEM_MESSAGE; childDBLinked relies on conversation_summaries.db.
	writeChild := func(cid, callID string, includeSender bool, inputTok, outTok string) {
		var content string
		if includeSender {
			content += `{"step_index":0,"source":"SYSTEM","type":"SYSTEM_MESSAGE","status":"DONE","created_at":"2026-11-15T10:00:00Z","content":"[Message] sender=` + parentID + `"}` + "\n"
		}
		content += `{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-15T10:00:02Z","input_tokens":` + inputTok + `,"cache_read_tokens":0,"output_tokens":` + outTok + `,"callId":"` + callID + `"}` + "\n"
		p := filepath.Join(tempHome, ".gemini", "antigravity", "brain", cid, ".system_generated", "logs", "transcript.jsonl")
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeChild(childFlash, "cf-1", true, "800", "200")
	writeChild(childInherit, "ci-1", true, "1500", "500")
	writeChild(childDBLinked, "cd-1", false, "2400", "600")

	summariesDB := filepath.Join(tempHome, ".gemini", "antigravity", "conversation_summaries.db")
	initSQL := "CREATE TABLE conversation_parents (conversation_id TEXT PRIMARY KEY, parent_id TEXT);" +
		"INSERT INTO conversation_parents (conversation_id, parent_id) VALUES ('" + childDBLinked + "', '" + parentID + "');"
	if out, err := exec.Command(sqliteBin, summariesDB, initSQL).CombinedOutput(); err != nil {
		t.Fatalf("sqlite3 init summaries db failed: %v (%s)", err, string(out))
	}

	historyMu.Lock()
	cachedLedger = nil
	cachedLedgerPath = ""
	cachedHistoryResp = nil
	historyMu.Unlock()

	resp, err := GetTokenHistory(true)
	if err != nil {
		t.Fatalf("GetTokenHistory() error = %v", err)
	}
	if len(resp.DailyMatrix) != 1 {
		t.Fatalf("DailyMatrix len = %d, want 1", len(resp.DailyMatrix))
	}
	models := resp.DailyMatrix[0].Models
	if models["Gemini 3.8 Flash"] != 1000 {
		t.Errorf("Gemini 3.8 Flash = %d, want 1000 (models=%v)", models["Gemini 3.8 Flash"], models)
	}
	if models["Gemini 4 Carbon (High)"] != 2000 {
		t.Errorf("Gemini 4 Carbon (High) = %d, want 2000 (models=%v)", models["Gemini 4 Carbon (High)"], models)
	}
	if models["Claude Sonnet 4.6 (Thinking)"] != 3000 {
		t.Errorf("Claude Sonnet 4.6 (Thinking) = %d, want 3000 (models=%v)", models["Claude Sonnet 4.6 (Thinking)"], models)
	}
}

func strconvFormatInt(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [24]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
