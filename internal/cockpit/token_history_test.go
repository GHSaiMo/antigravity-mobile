package cockpit

import (
	"os"
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

	log1 := `{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-14T10:00:00Z","input_tokens":1000,"cache_read_tokens":500,"output_tokens":200,"callId":"c-1"}` + "\n"
	log2 := `{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-14T10:05:00Z","input_tokens":300,"cache_read_tokens":0,"output_tokens":100,"callId":"c-2"}` + "\n"

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

	logContent := `{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-11-14T11:00:00Z","input_tokens":800,"cache_read_tokens":100,"output_tokens":100,"callId":"arch-1"}` + "\n"
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
