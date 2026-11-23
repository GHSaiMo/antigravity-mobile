package notifier

import (
	"testing"
	"time"

	"antigravity-mobile/internal/proxy"
)

func TestBuildContentStateUsesActivePlanAndFocus(t *testing.T) {
	now := time.Date(2026, 11, 23, 2, 0, 0, 0, time.UTC)

	details := &proxy.TrajectoryDetails{
		CascadeID:   "cid-1",
		Title:       "Refactor",
		Status:      "CASCADE_RUN_STATUS_RUNNING",
		TotalSteps:  12,
		ActiveFocus: "Running full unit tests, config checks, and offline replay evals",
		ActivePlan: &proxy.StepPlanItem{
			AnchorStepIndex: 1,
			DoneCount:       3,
			TotalCount:      4,
			Items: []proxy.StepPlanItemEntry{
				{Key: "a", Title: "Inspect", Status: "done"},
				{Key: "b", Title: "Gates", Status: "done"},
				{Key: "c", Title: "Guardrails", Status: "done"},
				{Key: "d", Title: "Add regression tests", Status: "in_progress"},
			},
		},
	}

	state := BuildContentState(details, now)
	want := "3/4 · Running full unit tests, config checks, and offline replay evals"
	if state.LatestAction != want {
		t.Fatalf("LatestAction = %q, want %q", state.LatestAction, want)
	}

	// Fallback to in_progress step title when ActiveFocus is empty
	details.ActiveFocus = ""
	state2 := BuildContentState(details, now)
	want2 := "3/4 · Add regression tests"
	if state2.LatestAction != want2 {
		t.Fatalf("LatestAction (no focus) = %q, want %q", state2.LatestAction, want2)
	}
}
