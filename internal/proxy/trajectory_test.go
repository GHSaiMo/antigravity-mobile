package proxy

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
)

func TestParseTrajectoryDetails_CanProceed(t *testing.T) {
	rawJSON := `{
		"status": "CASCADE_RUN_STATUS_IDLE",
		"trajectory": {
			"cascadeId": "test-cascade-123",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": {
						"userResponse": "Please generate a plan"
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": {
						"response": "Here is the implementation plan"
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_CODE_ACTION",
					"status": "CORTEX_STEP_STATUS_DONE",
					"codeAction": {
						"isArtifactFile": true,
						"artifactMetadata": {
							"summary": "Test plan",
							"requestFeedback": true,
							"userFacing": true
						},
						"actionResult": {
							"edit": {
								"absoluteUri": "file:///path/to/implementation_plan.md",
								"createFile": true
							}
						}
					}
				}
			]
		}
	}`

	var rawResp upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSON), &rawResp); err != nil {
		t.Fatalf("failed to unmarshal test JSON: %v", err)
	}

	p := &Proxy{}
	details := p.ParseTrajectoryDetails(&rawResp)

	if !details.CanProceed {
		t.Fatalf("expected CanProceed to be true, got false")
	}
	if details.ProceedArtifactURI != "file:///path/to/implementation_plan.md" {
		t.Fatalf("expected ProceedArtifactURI to be 'file:///path/to/implementation_plan.md', got %q", details.ProceedArtifactURI)
	}

	// Test 2: If subsequent user input exists, CanProceed should be false
	rawJSONAfterUserMessage := `{
		"status": "CASCADE_RUN_STATUS_IDLE",
		"trajectory": {
			"cascadeId": "test-cascade-123",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": { "userResponse": "Please generate a plan" }
				},
				{
					"type": "CORTEX_STEP_TYPE_CODE_ACTION",
					"status": "CORTEX_STEP_STATUS_DONE",
					"codeAction": {
						"isArtifactFile": true,
						"artifactMetadata": { "requestFeedback": true },
						"actionResult": {
							"edit": { "absoluteUri": "file:///path/to/implementation_plan.md" }
						}
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": { "userResponse": "I have reviewed it" }
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": { "response": "Proceeding with changes..." }
				}
			]
		}
	}`

	var rawResp2 upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSONAfterUserMessage), &rawResp2); err != nil {
		t.Fatalf("failed to unmarshal test JSON 2: %v", err)
	}

	details2 := p.ParseTrajectoryDetails(&rawResp2)
	if details2.CanProceed {
		t.Fatalf("expected CanProceed to be false after user response, got true")
	}

	// Test 3: If non-artifact code files were modified in the same turn, CanProceed should be false
	rawJSONWithCodeEdits := `{
		"status": "CASCADE_RUN_STATUS_IDLE",
		"trajectory": {
			"cascadeId": "test-cascade-123",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": { "userResponse": "Please generate a plan" }
				},
				{
					"type": "CORTEX_STEP_TYPE_CODE_ACTION",
					"status": "CORTEX_STEP_STATUS_DONE",
					"codeAction": {
						"isArtifactFile": true,
						"artifactMetadata": { "requestFeedback": true },
						"actionResult": {
							"edit": { "absoluteUri": "file:///path/to/implementation_plan.md" }
						}
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_CODE_ACTION",
					"status": "CORTEX_STEP_STATUS_DONE",
					"codeAction": {
						"isArtifactFile": false,
						"actionResult": {
							"edit": { "absoluteUri": "file:///path/to/main.go" }
						}
					}
				}
			]
		}
	}`

	var rawResp3 upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSONWithCodeEdits), &rawResp3); err != nil {
		t.Fatalf("failed to unmarshal test JSON 3: %v", err)
	}

	details3 := p.ParseTrajectoryDetails(&rawResp3)
	if details3.CanProceed {
		t.Fatalf("expected CanProceed to be false when code files were modified in same turn, got true")
	}

	// Test 4: Walkthrough document in the current turn must never trigger CanProceed
	rawJSONWalkthrough := `{
		"status": "CASCADE_RUN_STATUS_IDLE",
		"trajectory": {
			"cascadeId": "test-cascade-123",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": { "userResponse": "Remove animations and verify" }
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": { "response": "Completed task, updating walkthrough" }
				},
				{
					"type": "CORTEX_STEP_TYPE_CODE_ACTION",
					"status": "CORTEX_STEP_STATUS_DONE",
					"codeAction": {
						"isArtifactFile": true,
						"artifactMetadata": {
							"summary": "Walkthrough delivery report",
							"requestFeedback": false,
							"userFacing": true
						},
						"actionResult": {
							"edit": { "absoluteUri": "file:///path/to/.gemini/antigravity/brain/test-cascade-123/walkthrough.md" }
						}
					}
				}
			]
		}
	}`
	var rawResp4 upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSONWalkthrough), &rawResp4); err != nil {
		t.Fatalf("failed to unmarshal test JSON 4: %v", err)
	}
	details4 := p.ParseTrajectoryDetails(&rawResp4)
	if details4.CanProceed {
		t.Fatalf("expected CanProceed to be false for walkthrough.md, got true")
	}

	// Test 5: Scratch scripts in brain/scratch/ should count as code actions, not feedback artifacts
	rawJSONScratch := `{
		"status": "CASCADE_RUN_STATUS_IDLE",
		"trajectory": {
			"cascadeId": "test-cascade-123",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": { "userResponse": "Run scratch script" }
				},
				{
					"type": "CORTEX_STEP_TYPE_CODE_ACTION",
					"status": "CORTEX_STEP_STATUS_DONE",
					"codeAction": {
						"actionResult": {
							"edit": { "absoluteUri": "file:///path/to/.gemini/antigravity/brain/test-cascade-123/scratch/build.py" }
						}
					}
				}
			]
		}
	}`
	var rawResp5 upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSONScratch), &rawResp5); err != nil {
		t.Fatalf("failed to unmarshal test JSON 5: %v", err)
	}
	details5 := p.ParseTrajectoryDetails(&rawResp5)
	if details5.CanProceed {
		t.Fatalf("expected CanProceed to be false when scratch script was modified, got true")
	}
}

func TestParseTrajectoryDetails_PendingInteraction(t *testing.T) {
	rawJSON := `{
		"status": "CASCADE_RUN_STATUS_RUNNING",
		"trajectory": {
			"trajectoryId": "traj-test-999",
			"cascadeId": "cascade-test-999",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": {
						"userResponse": "Read the file"
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_GENERIC",
					"status": "CORTEX_STEP_STATUS_WAITING",
					"requestedInteraction": {
						"permission": {
							"resource": {
								"action": "read_file",
								"target": "/Applications/ego lite.app/SKILL.md"
							},
							"actionDescription": "View ego-browser skill documentation"
						}
					}
				}
			]
		}
	}`

	var rawResp upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSON), &rawResp); err != nil {
		t.Fatalf("failed to unmarshal test JSON: %v", err)
	}

	p := &Proxy{}
	details := p.ParseTrajectoryDetails(&rawResp)

	if details.PendingInteraction == nil {
		t.Fatalf("expected PendingInteraction to be non-nil, got nil")
	}
	pi := details.PendingInteraction
	if pi.Type != "permission" {
		t.Errorf("expected type 'permission', got %q", pi.Type)
	}
	if pi.Title != "Allow read access to this path?" {
		t.Errorf("expected title 'Allow read access to this path?', got %q", pi.Title)
	}
	if pi.Target != "/Applications/ego lite.app/SKILL.md" {
		t.Errorf("expected target '/Applications/ego lite.app/SKILL.md', got %q", pi.Target)
	}
	if len(pi.Options) != 5 {
		t.Errorf("expected 5 options, got %d", len(pi.Options))
	}
	if pi.Options[0].Text != "Yes, allow this time" {
		t.Errorf("expected first option 'Yes, allow this time', got %q", pi.Options[0].Text)
	}
}

func TestActualCascade_51dbc1ee(t *testing.T) {
	insp := inspector.NewInspector(5 * time.Second)
	info := insp.Scan()
	if info == nil {
		t.Skip("Antigravity instance not available")
	}
	p := NewProxy(insp)
	rawResp, err := p.fetchUpstreamTrajectory("51dbc1ee-33a5-4272-a5a6-468901235e0c", info.Port, info.CSRFToken)
	if err != nil {
		t.Skipf("debug cascade not found on running instance: %v", err)
	}
	details := p.ParseTrajectoryDetails(rawResp)
	t.Logf("Status: %s, PendingInteraction: %+v", details.Status, details.PendingInteraction)
	if details.Status == "CASCADE_RUN_STATUS_RUNNING" {
		if details.PendingInteraction == nil {
			t.Errorf("expected 51dbc1ee to have active PendingInteraction while waiting")
		} else {
			t.Logf("Extracted pending interaction: title=%s, target=%s, options=%d",
				details.PendingInteraction.Title, details.PendingInteraction.Target, len(details.PendingInteraction.Options))
		}
	}
}

func TestActualCascade_d363164b(t *testing.T) {
	insp := inspector.NewInspector(5 * time.Second)
	info := insp.Scan()
	if info == nil {
		t.Skip("Antigravity instance not available")
	}
	p := NewProxy(insp)
	rawResp, err := p.fetchUpstreamTrajectory("d363164b-e572-4de3-b4ad-05029eb6a629", info.Port, info.CSRFToken)
	if err != nil {
		t.Skipf("debug cascade not found on running instance: %v", err)
	}
	details := p.ParseTrajectoryDetails(rawResp)
	t.Logf("CanProceed: %t, ProceedArtifactURI: %s", details.CanProceed, details.ProceedArtifactURI)
	if details.CanProceed {
		t.Errorf("expected completed cascade d363164b to have CanProceed == false, got true")
	}
	if details.ProceedArtifactURI != "" {
		t.Errorf("expected completed cascade d363164b to have empty ProceedArtifactURI, got %q", details.ProceedArtifactURI)
	}
}

func TestActualCascade_def5e25a(t *testing.T) {
	insp := inspector.NewInspector(5 * time.Second)
	info := insp.Scan()
	if info == nil {
		t.Skip("Antigravity instance not available")
	}
	p := NewProxy(insp)
	rawResp, err := p.fetchUpstreamTrajectory("def5e25a-1718-4fd5-9c25-e3d3789fad36", info.Port, info.CSRFToken)
	if err != nil {
		t.Skipf("debug cascade def5e25a not found on running instance: %v", err)
	}
	details := p.ParseTrajectoryDetails(rawResp)
	t.Logf("def5e25a details: Status=%s, HasError=%t, TotalTools=%d, TotalMessages=%d",
		details.Status, details.HasError, details.TotalTools, len(details.AllMessages))
	for i, msg := range details.AllMessages {
		t.Logf("  Msg %d: type=%s, text=%q", i, msg.Type, msg.Text)
	}

	if details.TotalTools != 0 {
		t.Errorf("expected TotalTools to be 0, got %d", details.TotalTools)
	}
	if len(details.AllMessages) != 2 {
		t.Fatalf("expected exactly 2 messages (1 user, 1 final 503 error), got %d", len(details.AllMessages))
	}
	if details.AllMessages[0].Type != "user" || details.AllMessages[0].Text != "你是什么模型" {
		t.Errorf("expected first message to be '你是什么模型', got %+v", details.AllMessages[0])
	}
	if details.AllMessages[1].Type != "error" || !strings.Contains(details.AllMessages[1].Text, "503") {
		t.Errorf("expected second message to be 503 error, got %+v", details.AllMessages[1])
	}
}

func TestParseTrajectoryDetails_RunningTasks(t *testing.T) {
	rawJSON := `{
		"status": "CASCADE_RUN_STATUS_RUNNING",
		"trajectory": {
			"cascadeId": "test-task-cascade",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": {
						"userResponse": "Run browser tests"
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_RUN_COMMAND",
					"status": "CORTEX_STEP_STATUS_RUNNING",
					"metadata": {
						"createdAt": "2026-09-10T14:46:24Z",
						"toolSummary": "Check dashboard in browser",
						"toolAction": "Inspecting dashboard with browser",
						"toolCall": {
							"name": "run_command"
						},
						"sourceTrajectoryStepInfo": {
							"stepIndex": 1
						}
					},
					"taskDetails": {
						"id": "test-task-cascade/task-148",
						"logUri": "file:///path/to/task-148.log",
						"description": "ego-browser nodejs test.js"
					},
					"runCommand": {
						"commandLine": "ego-browser nodejs test.js",
						"cwd": "/Users/test/workspace"
					}
				}
			]
		}
	}`

	var rawResp upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSON), &rawResp); err != nil {
		t.Fatalf("failed to unmarshal test JSON: %v", err)
	}

	p := &Proxy{}
	details := p.ParseTrajectoryDetails(&rawResp)

	if len(details.RunningTasks) != 1 {
		t.Fatalf("expected 1 running task, got %d", len(details.RunningTasks))
	}
	task := details.RunningTasks[0]
	if task.ID != "test-task-cascade/task-148" {
		t.Errorf("expected task ID 'test-task-cascade/task-148', got %q", task.ID)
	}
	if task.StepIndex != 1 {
		t.Errorf("expected stepIndex 1, got %d", task.StepIndex)
	}
	if task.CommandLine != "ego-browser nodejs test.js" {
		t.Errorf("expected commandLine 'ego-browser nodejs test.js', got %q", task.CommandLine)
	}
	if task.ToolSummary != "Check dashboard in browser" {
		t.Errorf("expected toolSummary 'Check dashboard in browser', got %q", task.ToolSummary)
	}
}

func TestTrajectoryPagingDefaults(t *testing.T) {
	// Generate 35 mock messages
	var allMessages []CascadeMessageItem
	for i := 0; i < 35; i++ {
		allMessages = append(allMessages, CascadeMessageItem{
			ID:   string(rune('A' + i)),
			Type: "user",
			Text: "message",
		})
	}

	totalMsgs := len(allMessages)
	defaultLimit := 15

	// 1. Initial fetch (offset < 0)
	start := totalMsgs - defaultLimit
	if start < 0 {
		start = 0
	}
	sliced := allMessages[start:totalMsgs]
	hasMore := start > 0
	nextOffset := start

	if len(sliced) != 15 {
		t.Fatalf("expected 15 sliced messages, got %d", len(sliced))
	}
	if !hasMore {
		t.Fatalf("expected hasMore to be true")
	}
	if nextOffset != 20 {
		t.Fatalf("expected nextOffset 20, got %d", nextOffset)
	}

	// 2. Load older messages (offset = 20)
	targetEnd := nextOffset
	targetStart := targetEnd - defaultLimit
	if targetStart < 0 {
		targetStart = 0
	}
	slicedOlder := allMessages[targetStart:targetEnd]
	hasMoreOlder := targetStart > 0
	nextOffsetOlder := targetStart

	if len(slicedOlder) != 15 {
		t.Fatalf("expected 15 slicedOlder messages, got %d", len(slicedOlder))
	}
	if !hasMoreOlder {
		t.Fatalf("expected hasMoreOlder to be true")
	}
	if nextOffsetOlder != 5 {
		t.Fatalf("expected nextOffsetOlder 5, got %d", nextOffsetOlder)
	}
}

func TestParseTrajectoryDetails_ErrorMessage(t *testing.T) {
	rawJSON := `{
		"status": "CASCADE_RUN_STATUS_IDLE",
		"trajectory": {
			"cascadeId": "test-error-cascade",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": {
						"userResponse": "你是什么模型"
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_ERROR_MESSAGE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"errorMessage": {
						"error": {
							"userErrorMessage": "Agent execution terminated due to error.",
							"shortError": "checkpoint config validation failed: max token limit exceeded"
						},
						"shouldShowUser": true
					}
				}
			]
		}
	}`

	var rawResp upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSON), &rawResp); err != nil {
		t.Fatalf("failed to unmarshal test JSON: %v", err)
	}

	p := &Proxy{}
	details := p.ParseTrajectoryDetails(&rawResp)

	if !details.HasError {
		t.Errorf("expected HasError to be true, got false")
	}
	if details.Status != "CASCADE_RUN_STATUS_ERROR" {
		t.Errorf("expected Status to be CASCADE_RUN_STATUS_ERROR, got %s", details.Status)
	}
	if details.TotalTools != 0 {
		t.Errorf("expected TotalTools to be 0, got %d", details.TotalTools)
	}

	if len(details.AllMessages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(details.AllMessages))
	}

	if details.AllMessages[0].Type != "user" || details.AllMessages[0].Text != "你是什么模型" {
		t.Errorf("expected first message to be user '你是什么模型', got %+v", details.AllMessages[0])
	}

	errMsg := details.AllMessages[1]
	if errMsg.Type != "error" {
		t.Errorf("expected second message to be error type, got %s", errMsg.Type)
	}
	if errMsg.Text == "" || errMsg.Text == "error_message" {
		t.Errorf("expected descriptive error text, got %q", errMsg.Text)
	}
}

func TestParseTrajectoryDetails_HistoricalErrorResolved(t *testing.T) {
	// Scenario matching LoRA session:
	// Turn 1 had an error at step 1.
	// Turn 2 had a user input at step 2 and a successful planner response at step 3.
	// The session should NOT be marked as an error.
	rawJSON := `{
		"status": "CASCADE_RUN_STATUS_IDLE",
		"trajectory": {
			"cascadeId": "lora-session",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": {
						"userResponse": "旧指令"
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_ERROR_MESSAGE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"errorMessage": {
						"error": {
							"userErrorMessage": "LoRA 训练执行异常中断。",
							"shortError": "training stream interrupted"
						},
						"shouldShowUser": true
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": {
						"userResponse": "新指令"
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": {
						"response": "新指令已顺利完成，没有错误。"
					}
				}
			]
		}
	}`

	var rawResp upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSON), &rawResp); err != nil {
		t.Fatalf("failed to unmarshal test JSON: %v", err)
	}

	p := &Proxy{}
	details := p.ParseTrajectoryDetails(&rawResp)

	if details.HasError {
		t.Errorf("expected HasError to be false for resolved historical error, got true")
	}
	if details.Status != "CASCADE_RUN_STATUS_IDLE" {
		t.Errorf("expected Status to be CASCADE_RUN_STATUS_IDLE, got %s", details.Status)
	}
	if details.ErrorMessage != "" {
		t.Errorf("expected empty ErrorMessage, got %q", details.ErrorMessage)
	}

	// But historical messages should still include the user-visible error message
	var hasErrorMessage bool
	for _, msg := range details.AllMessages {
		if msg.Type == "error" && strings.Contains(msg.Text, "training stream interrupted") {
			hasErrorMessage = true
			break
		}
	}
	if !hasErrorMessage {
		t.Errorf("expected historical error message to be preserved in AllMessages")
	}
}

func TestParseTrajectoryDetails_InternalStreamInterruptedFiltered(t *testing.T) {
	// Replicates the exact scenario of "def5e25a-1718-4fd5-9c25-e3d3789fad36":
	// 1 User Input -> 2 transient stream interruptions with shouldShowModel: true -> final 503 error with shouldShowUser: true
	rawJSON := `{
		"status": "CASCADE_RUN_STATUS_ERROR",
		"trajectory": {
			"cascadeId": "def5e25a-1718-4fd5-9c25-e3d3789fad36",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": {
						"userResponse": "你是什么模型"
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": {
						"response": ""
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_ERROR_MESSAGE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"errorMessage": {
						"error": {
							"shortError": "The stream was interrupted. Please continue the task you were working on.",
							"errorId": "step-2-err"
						},
						"shouldShowModel": true
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": {
						"response": ""
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_ERROR_MESSAGE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"errorMessage": {
						"error": {
							"shortError": "The stream was interrupted. Please continue the task you were working on.",
							"errorId": "step-4-err"
						},
						"shouldShowModel": true
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": {
						"response": ""
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_ERROR_MESSAGE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"errorMessage": {
						"error": {
							"userErrorMessage": "Our servers are experiencing high traffic right now, please try again in a minute.",
							"shortError": "UNAVAILABLE (code 503): No capacity available for model claude-opus-4-6-thinking on the server",
							"errorId": "step-6-503"
						},
						"shouldShowUser": true
					}
				}
			]
		}
	}`

	var rawResp upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSON), &rawResp); err != nil {
		t.Fatalf("failed to unmarshal test JSON: %v", err)
	}

	p := &Proxy{}
	details := p.ParseTrajectoryDetails(&rawResp)

	if !details.HasError {
		t.Errorf("expected HasError to be true, got false")
	}
	if details.Status != "CASCADE_RUN_STATUS_ERROR" {
		t.Errorf("expected Status to be CASCADE_RUN_STATUS_ERROR, got %s", details.Status)
	}
	if details.TotalTools != 0 {
		t.Errorf("expected TotalTools to be 0 (no phantom tools from planner responses), got %d", details.TotalTools)
	}

	// Should contain EXACTLY 2 messages: 1 user, 1 error (the 503).
	// Transient errors and phantom tool batches must NOT appear!
	if len(details.AllMessages) != 2 {
		t.Fatalf("expected exactly 2 messages (1 user, 1 final error), got %d: %+v", len(details.AllMessages), details.AllMessages)
	}

	if details.AllMessages[0].Type != "user" || details.AllMessages[0].Text != "你是什么模型" {
		t.Errorf("unexpected first message: %+v", details.AllMessages[0])
	}
	if details.AllMessages[1].Type != "error" {
		t.Errorf("expected second message to be error, got %s", details.AllMessages[1].Type)
	}
	if !strings.Contains(details.AllMessages[1].Text, "503") {
		t.Errorf("expected second message to contain 503 error, got %s", details.AllMessages[1].Text)
	}
}


