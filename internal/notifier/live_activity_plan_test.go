package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"antigravity-mobile/internal/i18n"
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

func TestLiveActivity_PerDeviceLocaleAndHashIsolation(t *testing.T) {
	prev := i18n.GetNativeLocale()
	t.Cleanup(func() { i18n.SetNativeLocale(prev) })
	i18n.SetNativeLocale("zh-CN")

	store, err := NewPushDeviceStore(filepath.Join(t.TempDir(), "push.json"))
	if err != nil {
		t.Fatalf("NewPushDeviceStore: %v", err)
	}
	tokEn := sampleHexToken("1111")
	tokJa := sampleHexToken("2222")
	tokFr := sampleHexToken("3333")
	_ = store.UpsertLiveActivityToken("dev-en", "ios", "cid-multi-la", tokEn, "sandbox")
	_ = store.UpsertLiveActivityToken("dev-ja", "ios", "cid-multi-la", tokJa, "sandbox")
	_ = store.UpsertLiveActivityToken("dev-fr", "ios", "cid-multi-la", tokFr, "sandbox")
	_ = store.UpdateLanguage("dev-en", "en-US")
	_ = store.UpdateLanguage("dev-ja", "ja-JP")
	_ = store.UpdateLanguage("dev-fr", "fr-FR")

	// Verify PushDeviceStore preserves RawLocale ("fr-FR") while normalizing Lang ("en") for UI chrome.
	targets := store.ListLiveActivityTargets("cid-multi-la", false)
	rawByDev := map[string]string{}
	langByDev := map[string]string{}
	for _, tgt := range targets {
		rawByDev[tgt.DeviceID] = tgt.RawLocale
		langByDev[tgt.DeviceID] = tgt.Lang
	}
	if langByDev["dev-fr"] != "en" || rawByDev["dev-fr"] != "fr-FR" {
		t.Fatalf("dev-fr store locale = lang:%q raw:%q, want en / fr-FR", langByDev["dev-fr"], rawByDev["dev-fr"])
	}

	var mu sync.Mutex
	sentByToken := map[string][]LiveActivityContentState{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req APNsLiveActivityRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		tok := req.PushToken
		mu.Lock()
		sentByToken[tok] = append(sentByToken[tok], req.ContentState)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(APNsRelayResponse{OK: true, Status: 200})
	}))
	defer srv.Close()

	client := NewAPNsRelayClient(srv.URL, store)
	disp := NewLiveActivityDispatcher(client, true)
	fakeNow := time.Date(2026, 11, 27, 5, 0, 0, 0, time.UTC)
	disp.nowFunc = func() time.Time { return fakeNow }

	var preprocLocales []string
	disp.SetDetailsPreprocessor(func(ctx context.Context, details *proxy.TrajectoryDetails, fallbackLang string) {
		mu.Lock()
		preprocLocales = append(preprocLocales, fallbackLang)
		mu.Unlock()
		lower := strings.ToLower(fallbackLang)
		switch {
		case strings.HasPrefix(lower, "en"):
			// Explicit English MUST NOT translate
			return
		case strings.HasPrefix(lower, "ja"):
			details.ActiveFocus = "ユニットテストを実行中"
		case strings.HasPrefix(lower, "fr"):
			details.ActiveFocus = "Exécution des tests unitaires"
		}
	})

	details := &proxy.TrajectoryDetails{
		CascadeID:   "cid-multi-la",
		Title:       MsgUntitledSession, // "未命名会话" must NEVER leak to EN/JA/FR!
		Status:      "CASCADE_RUN_STATUS_RUNNING",
		TotalSteps:  4,
		ActiveFocus: "Running unit tests",
	}
	disp.SyncFromDetails(context.Background(), details)

	mu.Lock()
	enStates := sentByToken[tokEn]
	jaStates := sentByToken[tokJa]
	frStates := sentByToken[tokFr]
	mu.Unlock()

	if len(enStates) != 1 || len(jaStates) != 1 || len(frStates) != 1 {
		t.Fatalf("expected 1 push per device, got en=%d ja=%d fr=%d", len(enStates), len(jaStates), len(frStates))
	}
	if enStates[0].ConversationTitle != "Multigravity Agent" || enStates[0].LatestAction != "Running unit tests" {
		t.Fatalf("dev-en unexpected state: %+v", enStates[0])
	}
	if jaStates[0].ConversationTitle != "Multigravity Agent" || jaStates[0].LatestAction != "ユニットテストを実行中" {
		t.Fatalf("dev-ja unexpected state: %+v", jaStates[0])
	}
	if frStates[0].ConversationTitle != "Multigravity Agent" || frStates[0].LatestAction != "Exécution des tests unitaires" {
		t.Fatalf("dev-fr unexpected state: %+v", frStates[0])
	}

	// Second identical update after routine interval must dedup per device (0 extra sends).
	fakeNow = fakeNow.Add(30 * time.Second)
	disp.SyncFromDetails(context.Background(), details)
	mu.Lock()
	if len(sentByToken[tokEn]) != 1 || len(sentByToken[tokJa]) != 1 || len(sentByToken[tokFr]) != 1 {
		t.Fatalf("expected per-device dedup on identical state, got en=%d ja=%d fr=%d",
			len(sentByToken[tokEn]), len(sentByToken[tokJa]), len(sentByToken[tokFr]))
	}
	mu.Unlock()
}

func TestRedactedAutoSwitchQuotaCopy_FourLocales(t *testing.T) {
	for _, tc := range []struct {
		lang      string
		wantTitle string
	}{
		{"zh", "账号配额状态更新"},
		{"ja", "アカウント・クォータ更新"},
		{"en", "Account Quota Update"},
		{"fr-FR", "Account Quota Update"},
	} {
		title, body := msgRedactedQuotaCopyFor(tc.lang, "info")
		if !strings.Contains(title, tc.wantTitle) || body == "" {
			t.Errorf("msgRedactedQuotaCopyFor(%q, info) = %q / %q, want %q", tc.lang, title, body, tc.wantTitle)
		}
		if strings.Contains(title, "Low Quota") {
			t.Errorf("redacted info/auto_switch should not say Low Quota: %q", title)
		}
	}
}
