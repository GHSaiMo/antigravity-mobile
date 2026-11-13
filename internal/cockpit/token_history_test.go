package cockpit

import (
	"os"
	"path/filepath"
	"testing"
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
