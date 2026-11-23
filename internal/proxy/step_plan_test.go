package proxy

import (
	"encoding/json"
	"testing"
)

func TestStepPlanParsingAndPlacement(t *testing.T) {
	rawJSON := `{
		"status": "CASCADE_RUN_STATUS_RUNNING",
		"trajectory": {
			"cascadeId": "cid-step-plan",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"metadata": {"createdAt": "2026-11-23T01:00:00Z"},
					"userInput": {"userResponse": "Implement feature"}
				},
				{
					"type": "CORTEX_STEP_TYPE_GENERIC",
					"status": "CORTEX_STEP_STATUS_CLEARED",
					"metadata": {
						"createdAt": "2026-11-23T01:00:01Z",
						"toolCall": {"id": "tc-1", "name": "update_steps"}
					},
					"generic": {
						"result": {
							"stepRenderInfo": {
								"markdown": "- [>] Inspect orchestrator, executor, and tests\n- [ ] Add deterministic spawn gates\n- [ ] Run verification"
							}
						}
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_GENERIC",
					"status": "CORTEX_STEP_STATUS_DONE",
					"metadata": {
						"createdAt": "2026-11-23T01:00:02Z",
						"toolCall": {"id": "tc-2", "name": "update_focus"}
					},
					"generic": {
						"result": {
							"stepRenderInfo": {"title": "Inspecting orchestrator files"}
						}
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_VIEW_FILE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"metadata": {
						"createdAt": "2026-11-23T01:00:03Z",
						"toolCall": {"id": "tc-3", "name": "read_file"}
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"metadata": {"createdAt": "2026-11-23T01:01:00Z"},
					"plannerResponse": {"response": "First pass done, moving to tests."}
				},
				{
					"type": "CORTEX_STEP_TYPE_GENERIC",
					"status": "CORTEX_STEP_STATUS_DONE",
					"metadata": {
						"createdAt": "2026-11-23T01:01:05Z",
						"toolCall": {
							"id": "tc-4",
							"name": "update_steps",
							"argumentsJson": "{\"steps\":[{\"title\":\"Inspect orchestrator, executor, and tests\",\"status\":\"done\"},{\"title\":\"Add deterministic spawn gates\",\"status\":\"done\"},{\"title\":\"Skipped extra\",\"status\":\"skipped\"},{\"title\":\"Add regression tests and run verification\",\"status\":\"in_progress\"}]}"
						}
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_GENERIC",
					"status": "CORTEX_STEP_STATUS_DONE",
					"metadata": {
						"createdAt": "2026-11-23T01:01:06Z",
						"toolCall": {
							"id": "tc-5",
							"name": "update_focus",
							"argumentsJson": "{\"focus\":\"Running full unit tests, config checks, and offline replay evals\"}"
						}
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_RUN_COMMAND",
					"status": "CORTEX_STEP_STATUS_RUNNING",
					"metadata": {
						"createdAt": "2026-11-23T01:01:07Z",
						"toolCall": {"id": "tc-6", "name": "run_command"}
					},
					"runCommand": {"commandLine": "go test ./..."}
				}
			]
		}
	}`

	var raw upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	p := &Proxy{}
	d := p.ParseTrajectoryDetails(&raw)

	// update_steps and update_focus must NOT inflate TotalTools (only read_file + run_command = 2)
	if d.TotalTools != 2 {
		t.Fatalf("expected TotalTools=2, got %d", d.TotalTools)
	}

	// Expected message sequence:
	// [0] user
	// [1] tools (read_file, FocusText="Inspecting orchestrator files")
	// [2] agent ("First pass done, moving to tests.")
	// [3] step_plan (anchored at step 1, latest 4 items, right above trailing tools!)
	// [4] tools (run_command, FocusText="Running full unit tests...", StartedAt="2026-11-23T01:01:06Z")
	if len(d.AllMessages) != 5 {
		t.Fatalf("expected 5 messages, got %d: %#v", len(d.AllMessages), d.AllMessages)
	}
	if d.AllMessages[0].Type != "user" ||
		d.AllMessages[1].Type != "tools" ||
		d.AllMessages[2].Type != "agent" ||
		d.AllMessages[3].Type != "step_plan" ||
		d.AllMessages[4].Type != "tools" {
		t.Fatalf("unexpected message order: %s, %s, %s, %s, %s",
			d.AllMessages[0].Type, d.AllMessages[1].Type, d.AllMessages[2].Type,
			d.AllMessages[3].Type, d.AllMessages[4].Type)
	}

	if d.AllMessages[1].FocusText != "Inspecting orchestrator files" {
		t.Fatalf("expected first tools FocusText, got %q", d.AllMessages[1].FocusText)
	}
	if d.AllMessages[4].FocusText != "Running full unit tests, config checks, and offline replay evals" {
		t.Fatalf("expected trailing tools FocusText, got %q", d.AllMessages[4].FocusText)
	}
	if d.AllMessages[4].StartedAt != "2026-11-23T01:01:06Z" {
		t.Fatalf("expected trailing tools StartedAt=2026-11-23T01:01:06Z, got %q", d.AllMessages[4].StartedAt)
	}

	planMsg := d.AllMessages[3]
	if planMsg.ID != "step-plan-1" {
		t.Fatalf("expected stable anchor id step-plan-1, got %q", planMsg.ID)
	}
	if planMsg.StepIndex != nil {
		t.Fatalf("expected step_plan StepIndex to be nil to avoid reorder traps, got %v", *planMsg.StepIndex)
	}
	if planMsg.StepPlan == nil {
		t.Fatalf("expected StepPlan payload")
	}
	sp := planMsg.StepPlan
	if sp.AnchorStepIndex != 1 || sp.DoneCount != 2 || sp.TotalCount != 3 || sp.SkippedCount != 1 || len(sp.Items) != 4 {
		t.Fatalf("unexpected StepPlan stats: %+v", sp)
	}
	if sp.Items[3].Status != "in_progress" || sp.Items[3].Title != "Add regression tests and run verification" {
		t.Fatalf("unexpected 4th step item: %+v", sp.Items[3])
	}

	if d.ActiveFocus != "Running full unit tests, config checks, and offline replay evals" {
		t.Fatalf("unexpected ActiveFocus: %q", d.ActiveFocus)
	}
	if d.ActivePlan == nil || d.ActivePlan.DoneCount != 2 || d.ActivePlan.TotalCount != 3 {
		t.Fatalf("unexpected ActivePlan: %+v", d.ActivePlan)
	}
}

func TestStepPlanFinalizedPlacementAndClearedMarkdown(t *testing.T) {
	rawJSON := `{
		"status": "CASCADE_RUN_STATUS_IDLE",
		"trajectory": {
			"cascadeId": "cid-step-plan-done",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": {"userResponse": "Ship it"}
				},
				{
					"type": "CORTEX_STEP_TYPE_GENERIC",
					"status": "CORTEX_STEP_STATUS_CLEARED",
					"metadata": {"toolCall": {"name": "update_steps"}},
					"generic": {
						"result": {
							"stepRenderInfo": {
								"markdown": "- [x] Build\n- [!] Lint\n- [-] Optional\n- [x] Build"
							}
						}
					}
				},
				{
					"type": "CORTEX_STEP_TYPE_RUN_COMMAND",
					"status": "CORTEX_STEP_STATUS_DONE",
					"metadata": {"toolCall": {"name": "run_command"}}
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": {"response": "All done!"}
				}
			]
		}
	}`

	var raw upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	p := &Proxy{}
	d := p.ParseTrajectoryDetails(&raw)

	// [0] user, [1] step_plan, [2] tools, [3] agent
	if len(d.AllMessages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(d.AllMessages))
	}
	if d.AllMessages[1].Type != "step_plan" || d.AllMessages[2].Type != "tools" || d.AllMessages[3].Type != "agent" {
		t.Fatalf("unexpected order: %s, %s, %s", d.AllMessages[1].Type, d.AllMessages[2].Type, d.AllMessages[3].Type)
	}
	sp := d.AllMessages[1].StepPlan
	if sp == nil || sp.DoneCount != 2 || sp.FailedCount != 1 || sp.SkippedCount != 1 || sp.TotalCount != 3 {
		t.Fatalf("unexpected cleared markdown parse: %+v", sp)
	}
	if sp.Items[0].Key != "build" || sp.Items[3].Key != "build#2" {
		t.Fatalf("expected deduplicated keys build & build#2, got %q & %q", sp.Items[0].Key, sp.Items[3].Key)
	}
	if d.ActiveFocus != "" {
		t.Fatalf("expected ActiveFocus reset after final agent response, got %q", d.ActiveFocus)
	}
}

func TestStreamFingerprintDetectsInPlaceStepPlanAndFocusUpdates(t *testing.T) {
	base := StreamUpdatePayload{
		Status:        "CASCADE_RUN_STATUS_RUNNING",
		TotalMessages: 3,
		Messages: []CascadeMessageItem{
			{ID: "step-0", Type: "user", Text: "go"},
			{
				ID:   "step-plan-1",
				Type: "step_plan",
				Text: "1/2 steps",
				StepPlan: &StepPlanItem{
					AnchorStepIndex: 1,
					DoneCount:       1,
					TotalCount:      2,
					Items: []StepPlanItemEntry{
						{Key: "a", Title: "A", Status: "done"},
						{Key: "b", Title: "B", Status: "in_progress"},
					},
				},
			},
			{ID: "tools-1", Type: "tools", Text: "已思考并执行 1 项操作", ToolCount: 1, FocusText: "Running A"},
		},
		ActiveFocus: "Running A",
	}
	fp1 := base.Fingerprint()

	// Mutate only FocusText on the last message
	changedFocus := base
	changedFocus.Messages = append([]CascadeMessageItem(nil), base.Messages...)
	changedFocus.Messages[2].FocusText = "Running B"
	changedFocus.ActiveFocus = "Running B"
	if fp2 := changedFocus.Fingerprint(); fp2 == fp1 {
		t.Fatalf("Fingerprint failed to detect FocusText update")
	}

	// Mutate only non-last step_plan message status
	changedPlan := base
	changedPlan.Messages = append([]CascadeMessageItem(nil), base.Messages...)
	planCopy := *base.Messages[1].StepPlan
	planCopy.DoneCount = 2
	planCopy.Items = []StepPlanItemEntry{
		{Key: "a", Title: "A", Status: "done"},
		{Key: "b", Title: "B", Status: "done"},
	}
	changedPlan.Messages[1].StepPlan = &planCopy
	if fp3 := changedPlan.Fingerprint(); fp3 == fp1 {
		t.Fatalf("Fingerprint failed to detect in-place StepPlan status update")
	}

	// Delta hash must also change for step_plan and focus updates
	if hashMessageItem(&base.Messages[1]) == hashMessageItem(&changedPlan.Messages[1]) {
		t.Fatalf("hashMessageItem failed to detect StepPlan update")
	}
	if hashMessageItem(&base.Messages[2]) == hashMessageItem(&changedFocus.Messages[2]) {
		t.Fatalf("hashMessageItem failed to detect FocusText update")
	}
}
