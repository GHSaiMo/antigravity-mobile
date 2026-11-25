package proxy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildToolDetailItem_DesktopParityAndNoRawJSON(t *testing.T) {
	rawJSON := `{
		"status": "CASCADE_RUN_STATUS_IDLE",
		"trajectory": {
			"cascadeId": "cid-desktop-tool-parity",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": {"userResponse": "Inspect orchestrator models"}
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": {"rawThinking": "First check git log and read StreamPayload.", "thinkingDuration": "4.2s", "response": ""}
				},
				{
					"type": "CORTEX_STEP_TYPE_GENERIC",
					"status": "CORTEX_STEP_STATUS_DONE",
					"metadata": {
						"toolCall": {
							"id": "tc-1",
							"name": "read_file",
							"argumentsJson": "{\"EndLine\":650,\"Path\":\"/Users/hal9000/Projects/antigravity-mobile/android/app/src/main/java/StreamPayload.kt\",\"StartLine\":1}"
						}
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_GENERIC",
					"status": "CORTEX_STEP_STATUS_DONE",
					"metadata": {
						"toolCall": {"id": "tc-2", "name": "edit_file"}
					},
					"generic": {
						"result": {
							"stepRenderInfo": {
								"titlePrefix": "Edited",
								"aggregationKey": "edits",
								"titleParts": [
									{"fileDiff": {"uri": "file:///repo/ios/StepPlanCardView.swift", "diffStats": {"additions": 42, "deletions": 18}}}
								]
							}
						}
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_GENERIC",
					"status": "CORTEX_STEP_STATUS_DONE",
					"metadata": {"toolCall": {"id": "tc-hidden", "name": "internal_noop"}},
					"generic": {"result": {"stepRenderInfo": {"hidden": true, "title": "secret"}}}
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": {"thinking": "Now run tests to verify.", "thinkingDuration": "2.0s", "response": ""}
				},
				{
					"type": "CORTEX_STEP_TYPE_GENERIC",
					"status": "CORTEX_STEP_STATUS_DONE",
					"metadata": {
						"toolSummary": "Unit test execution",
						"toolCall": {"id": "tc-3", "name": "run_command"}
					},
					"generic": {
						"result": {
							"stepRenderInfo": {
								"titlePrefix": "Running",
								"aggregationKey": "terminal",
								"titleParts": [{"text": "Unit test execution"}],
								"contentBlocks": [
									{"terminal": {"command": "go test ./internal/proxy", "cwd": "/Users/hal9000/Projects/antigravity-mobile", "exitCode": 0, "output": "ok  antigravity-mobile/internal/proxy 0.42s\n"}}
								]
							}
						}
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_GREP_SEARCH",
					"status": "CORTEX_STEP_STATUS_DONE",
					"grepSearch": {"query": "Orchestrator", "totalResults": 7}
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": {"response": "All checked."}
				}
			]
		}
	}`

	var raw upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	d := (&Proxy{}).ParseTrajectoryDetails(&raw)
	if len(d.AllMessages) != 3 {
		t.Fatalf("expected 3 messages (user, tools, agent), got %d", len(d.AllMessages))
	}
	toolsMsg := d.AllMessages[1]
	if toolsMsg.Type != "tools" {
		t.Fatalf("expected tools message, got %q", toolsMsg.Type)
	}

	// Hidden step must be filtered out: read_file, edit_file, run_command, grep_search => 4
	if toolsMsg.ToolCount != 4 || len(toolsMsg.ToolDetails) != 4 {
		t.Fatalf("expected 4 visible toolDetails, got count=%d len=%d", toolsMsg.ToolCount, len(toolsMsg.ToolDetails))
	}

	// 1) read_file fallback parses Path/StartLine/EndLine without raw JSON leakage
	readItem := toolsMsg.ToolDetails[0]
	if readItem.Kind != "read" || readItem.Prefix != "Analyzed" || readItem.FileName != "StreamPayload.kt" || readItem.StartLine != 1 || readItem.EndLine != 650 {
		t.Fatalf("unexpected read_file detail: %+v", readItem)
	}
	if strings.Contains(readItem.Summary, "{") || strings.Contains(readItem.Title, "{") {
		t.Fatalf("raw JSON leaked into read_file detail: %+v", readItem)
	}

	// 2) edit_file stepRenderInfo fileDiff
	editItem := toolsMsg.ToolDetails[1]
	if editItem.Kind != "edit" || editItem.Prefix != "Edited" || editItem.FileName != "StepPlanCardView.swift" || !editItem.HasDiffStats || editItem.Additions != 42 || editItem.Deletions != 18 {
		t.Fatalf("unexpected edit_file detail: %+v", editItem)
	}

	// 3) run_command stepRenderInfo terminal block ("Running" -> "Ran" when DONE)
	cmdItem := toolsMsg.ToolDetails[2]
	if cmdItem.Kind != "terminal" || cmdItem.Prefix != "Ran" || cmdItem.Title != "Unit test execution" || cmdItem.Command != "go test ./internal/proxy" || cmdItem.Cwd != "antigravity-mobile" {
		t.Fatalf("unexpected run_command detail: %+v", cmdItem)
	}
	if cmdItem.ExitCode == nil || *cmdItem.ExitCode != 0 || !strings.Contains(cmdItem.Output, "ok  antigravity-mobile") {
		t.Fatalf("unexpected terminal output/exitCode: %+v", cmdItem)
	}

	// 4) grep_search
	grepItem := toolsMsg.ToolDetails[3]
	if grepItem.Kind != "search" || grepItem.Prefix != "Searched" || grepItem.Title != "Orchestrator" || grepItem.NumResults != 7 {
		t.Fatalf("unexpected grep_search detail: %+v", grepItem)
	}

	// Chronological ThinkingBlocks preserved at exact step indices (step 1 and step 5)
	if len(toolsMsg.ThinkingBlocks) != 2 {
		t.Fatalf("expected 2 thinkingBlocks, got %d", len(toolsMsg.ThinkingBlocks))
	}
	if toolsMsg.ThinkingBlocks[0].StepIndex != 1 || toolsMsg.ThinkingBlocks[0].Duration != "4.2s" || !strings.Contains(toolsMsg.ThinkingBlocks[0].Text, "First check") {
		t.Fatalf("unexpected thinkingBlocks[0]: %+v", toolsMsg.ThinkingBlocks[0])
	}
	if toolsMsg.ThinkingBlocks[1].StepIndex != 5 || toolsMsg.ThinkingBlocks[1].Duration != "2.0s" {
		t.Fatalf("unexpected thinkingBlocks[1]: %+v", toolsMsg.ThinkingBlocks[1])
	}
}

func TestTruncateTailOutputAndDeltaHash(t *testing.T) {
	var lines []string
	for i := 1; i <= 120; i++ {
		lines = append(lines, "line-"+strings.Repeat("x", 10))
	}
	raw := strings.Join(lines, "\n")
	out, above := truncateTailOutput(raw, 5, 80, 4096)
	if above != 45 {
		t.Fatalf("expected 45 lines above (5 + 40), got %d", above)
	}
	if len(strings.Split(out, "\n")) != 80 {
		t.Fatalf("expected 80 retained lines, got %d", len(strings.Split(out, "\n")))
	}

	// Delta hash must detect terminal output updates on the same step
	m1 := CascadeMessageItem{
		ID:          "tools-1",
		Type:        "tools",
		ToolDetails: []ToolDetailItem{{Name: "run_command", Kind: "terminal", Command: "echo hi", Output: "a"}},
	}
	m2 := m1
	m2.ToolDetails = []ToolDetailItem{{Name: "run_command", Kind: "terminal", Command: "echo hi", Output: "a\nb"}}
	if hashMessageItem(&m1) == hashMessageItem(&m2) {
		t.Fatalf("hashMessageItem failed to detect terminal output update")
	}
}
