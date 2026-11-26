package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
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

func TestLiveActivityDispatcher_DetailsPreprocessorAppliesTranslations(t *testing.T) {
	store, err := NewPushDeviceStore(filepath.Join(t.TempDir(), "push.json"))
	if err != nil {
		t.Fatalf("NewPushDeviceStore: %v", err)
	}
	if err := store.UpsertLiveActivityToken("dev-la-1", "ios", "cid-la-tr", sampleHexToken("4444"), "sandbox"); err != nil {
		t.Fatalf("UpsertLiveActivityToken: %v", err)
	}

	var mu sync.Mutex
	var gotStates []LiveActivityContentState
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req APNsLiveActivityRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		gotStates = append(gotStates, req.ContentState)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(APNsRelayResponse{OK: true, Status: 200})
	}))
	defer srv.Close()

	client := NewAPNsRelayClient(srv.URL, store)
	disp := NewLiveActivityDispatcher(client, true)
	disp.SetDetailsPreprocessor(func(ctx context.Context, details *proxy.TrajectoryDetails, fallbackLang string) {
		details.ActiveFocus = "正在更新 Android 布局"
		if details.ActivePlan != nil {
			cp := *details.ActivePlan
			cp.Items = append([]proxy.StepPlanItemEntry(nil), details.ActivePlan.Items...)
			cp.Items[0].Title = "更新 Android 布局"
			details.ActivePlan = &cp
		}
	})

	origDetails := &proxy.TrajectoryDetails{
		CascadeID:   "cid-la-tr",
		Title:       "Layout",
		Status:      "CASCADE_RUN_STATUS_RUNNING",
		TotalSteps:  5,
		ActiveFocus: "Updating Android layout",
		ActivePlan: &proxy.StepPlanItem{
			DoneCount:  1,
			TotalCount: 3,
			Items: []proxy.StepPlanItemEntry{
				{Key: "update android layout", Title: "Update Android layout", Status: "in_progress"},
			},
		},
	}

	disp.SyncFromDetails(context.Background(), origDetails)

	mu.Lock()
	defer mu.Unlock()
	if len(gotStates) != 1 {
		t.Fatalf("expected 1 Live Activity push, got %d", len(gotStates))
	}
	if gotStates[0].LatestAction != "1/3 · 正在更新 Android 布局" {
		t.Fatalf("expected translated LatestAction, got %q", gotStates[0].LatestAction)
	}
	// Ensure caller's original TrajectoryDetails was NOT mutated.
	if origDetails.ActiveFocus != "Updating Android layout" {
		t.Fatalf("caller details mutated: %q", origDetails.ActiveFocus)
	}
}
