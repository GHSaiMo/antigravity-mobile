package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
)

func TestAIAssistPrefs_DefaultsAndPersistence(t *testing.T) {
	t.Setenv("MULTIGRAVITY_DATA_DIR", t.TempDir())
	t.Setenv("MGY_AI_TITLE_TRANSLATE", "")
	t.Setenv("MGY_AI_NEXT_PROMPT", "")

	p := NewProxy(inspector.NewInspector(time.Second))

	// GET default preferences
	req := httptest.NewRequest(http.MethodGet, "/gateway/ai-assist/prefs", nil)
	rec := httptest.NewRecorder()
	p.HandleAIAssistPrefs(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET prefs status = %d: %s", rec.Code, rec.Body.String())
	}
	var got AIAssistStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal GET prefs: %v", err)
	}
	if got.Prefs.TitleTranslateEnabled || got.Prefs.StepPlanTranslateEnabled || got.Prefs.FocusTranslateEnabled || got.Prefs.NextPromptEnabled {
		t.Fatalf("expected opt-in defaults false, got %+v", got.Prefs)
	}
	if got.ModelID == "" || got.ModelName == "" {
		t.Fatalf("expected resolved lightweight model, got %+v", got)
	}

	// POST partial patch (also verifies deprecated legacy nextPromptTapAction field is ignored cleanly)
	patchBody := `{"titleTranslateEnabled":true,"titleTranslateLang":"pt-BR","titleTranslateLangName":"Portuguese","stepPlanTranslateEnabled":true,"focusTranslateEnabled":true,"nextPromptEnabled":true,"nextPromptTapAction":"send"}`
	req2 := httptest.NewRequest(http.MethodPost, "/gateway/ai-assist/prefs", strings.NewReader(patchBody))
	rec2 := httptest.NewRecorder()
	p.HandleAIAssistPrefs(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("POST prefs status = %d: %s", rec2.Code, rec2.Body.String())
	}

	// Reload from a fresh Proxy instance pointing at the same data dir
	p2 := NewProxy(inspector.NewInspector(time.Second))
	reloaded := p2.AIAssistPrefs()
	if !reloaded.TitleTranslateEnabled || !reloaded.StepPlanTranslateEnabled || !reloaded.FocusTranslateEnabled || !reloaded.NextPromptEnabled {
		t.Fatalf("expected persisted enabled flags, got %+v", reloaded)
	}
	if reloaded.TitleTranslateEnabledAt == "" {
		t.Fatalf("expected persisted titleTranslateEnabledAt watermark when enabled, got %+v", reloaded)
	}
	if reloaded.TitleTranslateLang != "pt-BR" || reloaded.TitleTranslateLangName != "Portuguese" {
		t.Fatalf("expected persisted target language pt-BR/Portuguese, got %+v", reloaded)
	}

	// Disabling title translation clears the watermark
	req3 := httptest.NewRequest(http.MethodPost, "/gateway/ai-assist/prefs", strings.NewReader(`{"titleTranslateEnabled":false}`))
	rec3 := httptest.NewRecorder()
	p2.HandleAIAssistPrefs(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("POST disable status = %d: %s", rec3.Code, rec3.Body.String())
	}
	if p2.AIAssistPrefs().TitleTranslateEnabledAt != "" {
		t.Fatalf("expected titleTranslateEnabledAt cleared on disable, got %+v", p2.AIAssistPrefs())
	}
}

func TestTitleNeedsTranslation_MultiLocaleRules(t *testing.T) {
	cases := []struct {
		title string
		lang  string
		want  bool
	}{
		// Chinese target
		{"Antigravity Context Viewer Prompt Principles", "zh-Hans", true},
		{"Fix iOS Dynamic Island Push", "zh-Hans", true},
		{"修复 iOS 灵动岛推送问题", "zh-Hans", false},
		{"中东地缘局势分析", "zh-Hans", false},
		{"アプリの設定画面を修正", "zh-Hans", true}, // Japanese Kana -> translate to Chinese
		{"/Users/hal9000/.gemini/config/skills/foo", "zh-Hans", false},
		{"未命名会话", "zh-Hans", false},

		// Japanese target (Chinese-only Han should still translate to Japanese; Kana means already Japanese)
		{"Fix iOS Dynamic Island Push", "ja", true},
		{"中东地缘局势分析", "ja", true},
		{"iOSプッシュ通知の修正", "ja", false},

		// Latin-script target (Portuguese): unseen titles go to model; ledger prevents repeats
		{"Fix iOS Dynamic Island Push", "pt-BR", true},

		// Korean target
		{"Fix Android Build", "ko", true},
		{"안드로이드 빌드 오류 수정", "ko", false},

		// English target
		{"Fix Android Build", "en", false},
		{"修复安卓构建", "en", true},
	}

	for _, tc := range cases {
		got := titleNeedsTranslation(tc.title, tc.lang)
		if got != tc.want {
			t.Errorf("titleNeedsTranslation(%q, %q) = %v, want %v", tc.title, tc.lang, got, tc.want)
		}
	}
}

func TestTitleTranslator_FutureOnlyOverwriteAndSkipExisting(t *testing.T) {
	t.Setenv("MULTIGRAVITY_DATA_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	oldDebounce := titleTranslateDebounce
	titleTranslateDebounce = 15 * time.Millisecond
	t.Cleanup(func() { titleTranslateDebounce = oldDebounce })

	p := NewProxy(inspector.NewInspector(time.Second))

	// Seed an existing cached conversation ID (without createdTime) before enabling title translation.
	p.allTrajectories.mu.Lock()
	p.allTrajectories.parsed = &allTrajectoriesParsed{
		sortedIDs: []string{"conv-baseline-no-created"},
	}
	p.allTrajectories.mu.Unlock()

	// Enable title translation via prefs handler (stamps enabledAt & captures baselineIDs).
	enableReq := httptest.NewRequest(http.MethodPost, "/gateway/ai-assist/prefs",
		strings.NewReader(`{"titleTranslateEnabled":true,"titleTranslateLang":"zh-Hans","titleTranslateLangName":"Simplified Chinese"}`))
	enableRec := httptest.NewRecorder()
	p.HandleAIAssistPrefs(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("enable prefs status = %d: %s", enableRec.Code, enableRec.Body.String())
	}

	var mu sync.Mutex
	written := map[string]string{}
	doneCh := make(chan struct{}, 2)
	p.titleWriteHook = func(ctx context.Context, cascadeID, title string) error {
		mu.Lock()
		written[cascadeID] = title
		mu.Unlock()
		return nil
	}

	modelCalls := 0
	p.lsModelResponseHook = func(ctx context.Context, prompt, modelEnum string) (string, error) {
		mu.Lock()
		modelCalls++
		mu.Unlock()
		defer func() {
			select {
			case doneCh <- struct{}{}:
			default:
			}
		}()
		if strings.Contains(prompt, "Input JSON:") {
			out := map[string]string{}
			var items []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			}
			idx := strings.Index(prompt, "[{")
			if idx >= 0 && json.Unmarshal([]byte(prompt[idx:]), &items) == nil {
				for _, it := range items {
					if strings.Contains(it.Title, "Prompt Principles") {
						out[it.ID] = "Antigravity Context Viewer 提示词原则"
					} else {
						out[it.ID] = "优化 Stream Delta 缓存"
					}
				}
			}
			b, _ := json.Marshal(out)
			return string(b), nil
		}
		return "Antigravity Context Viewer 提示词原则", nil
	}

	// Mark conv-future-edited as manually renamed -> must never be overwritten.
	p.MarkConversationTitleUserEdited("conv-future-edited", "Debug Xcode Wireless Connection")

	beforeStr := time.Now().Add(-5 * time.Minute).UTC().Format(time.RFC3339Nano)
	futureStr := time.Now().Add(5 * time.Second).UTC().Format(time.RFC3339Nano)

	summaries := map[string]map[string]interface{}{
		// Existing conversation created before toggle -> MUST NOT be touched even if lastModifiedTime is newer!
		"conv-old-active": {
			"summary":          "Old Conversation Updated After Enable",
			"createdTime":      beforeStr,
			"lastModifiedTime": futureStr,
		},
		// Existing baseline conversation without createdTime -> MUST NOT be touched!
		"conv-baseline-no-created": {
			"summary":          "Baseline Existing Conversation Without CreatedTime",
			"lastModifiedTime": futureStr,
		},
		// Future archived conversation -> skip
		"conv-future-archived": {
			"summary":          "Future Archived Chat",
			"annotations":      map[string]interface{}{"title": "Future Archived Chat", "archived": true},
			"createdTime":      futureStr,
			"lastModifiedTime": futureStr,
		},
		// Future manually renamed conversation -> skip
		"conv-future-renamed": {
			"summary":          "Auto Generated English Title",
			"annotations":      map[string]interface{}{"title": "My Custom English Name"},
			"createdTime":      futureStr,
			"lastModifiedTime": futureStr,
		},
		// Future user-edited via mobile -> skip
		"conv-future-edited": {
			"summary":          "Debug Xcode Wireless Connection",
			"createdTime":      futureStr,
			"lastModifiedTime": futureStr,
		},
		// Future already-Chinese conversation -> skip
		"conv-future-zh": {
			"summary":          "中东地缘局势分析",
			"createdTime":      futureStr,
			"lastModifiedTime": futureStr,
		},
		// Future subagent -> skip
		"conv-future-subagent": {
			"summary":            "Research subagent task",
			"trajectoryMetadata": map[string]interface{}{"parentConversationId": "conv-future-1"},
			"createdTime":        futureStr,
			"lastModifiedTime":   futureStr,
		},
		// Future eligible conversations -> MUST translate and overwrite!
		"conv-future-1": {
			"summary":          "Antigravity Context Viewer Prompt Principles",
			"createdTime":      futureStr,
			"lastModifiedTime": futureStr,
		},
		"conv-future-2": {
			"summary":          "Optimize Stream Delta Cache",
			"annotations":      map[string]interface{}{"title": "Optimize Stream Delta Cache"},
			"createdTime":      futureStr,
			"lastModifiedTime": futureStr,
		},
	}

	p.MaybeAutoTranslateSummaries(summaries)

	select {
	case <-doneCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for auto title translation")
	}

	// Wait briefly for writes to commit after model hook returns
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(written)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	got1 := written["conv-future-1"]
	got2 := written["conv-future-2"]
	totalWritten := len(written)
	callsAfterFirst := modelCalls
	mu.Unlock()

	if totalWritten != 2 || got1 != "Antigravity Context Viewer 提示词原则" || got2 != "优化 Stream Delta 缓存" {
		t.Fatalf("unexpected written titles (count=%d): %+v", totalWritten, written)
	}
	if callsAfterFirst != 1 {
		t.Fatalf("expected 1 batched model call for 2 future conversations, got %d", callsAfterFirst)
	}

	// Re-observing the same summaries (including with overwritten annotations.title) must issue 0 extra model calls.
	summaries["conv-future-1"]["annotations"] = map[string]interface{}{"title": got1}
	summaries["conv-future-2"]["annotations"] = map[string]interface{}{"title": got2}
	p.MaybeAutoTranslateSummaries(summaries)
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	callsAfterSecond := modelCalls
	mu.Unlock()
	if callsAfterSecond != callsAfterFirst {
		t.Fatalf("expected 0 extra model calls on re-observation, got %d", callsAfterSecond-callsAfterFirst)
	}
}

func TestNextPrompt_GatesSanitizationAndAsyncResolution(t *testing.T) {
	t.Setenv("MULTIGRAVITY_DATA_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	p := NewProxy(inspector.NewInspector(time.Second))
	enabled := true
	if _, err := p.aiAssistPrefs.Update(AIAssistPrefsPatch{NextPromptEnabled: &enabled}); err != nil {
		t.Fatalf("enable next prompt: %v", err)
	}

	// 1. Pre-abstention gates
	base := TrajectoryDetails{
		CascadeID:  "cid-next-1",
		Status:     "CASCADE_RUN_STATUS_IDLE",
		TotalSteps: 6,
		AllMessages: []CascadeMessageItem{
			{ID: "u1", Role: "user", Text: "修改代码后顺便跑一下单元测试。"},
			{ID: "a1", Role: "agent", Text: "已经修改完了核心逻辑并保存文件，还没执行测试。"},
		},
		ActivePlan: &StepPlanItem{
			Items: []StepPlanItemEntry{
				{Title: "修改核心逻辑", Status: "done"},
				{Title: "运行单元测试确认通过", Status: "pending"},
			},
		},
	}

	// Running -> skip
	running := base
	running.Status = "CASCADE_RUN_STATUS_RUNNING"
	if skip, _ := shouldSkipNextPromptPrediction(&running, nil); !skip {
		t.Fatal("expected running session to skip")
	}
	// Error -> skip (has Continue)
	errored := base
	errored.HasError = true
	if skip, _ := shouldSkipNextPromptPrediction(&errored, nil); !skip {
		t.Fatal("expected errored session to skip")
	}
	// CanProceed -> skip (has Proceed)
	proceed := base
	proceed.CanProceed = true
	if skip, _ := shouldSkipNextPromptPrediction(&proceed, nil); !skip {
		t.Fatal("expected canProceed session to skip")
	}
	// PendingInteraction -> skip
	pending := base
	pending.PendingInteraction = &PendingInteraction{Type: "run_command"}
	if skip, _ := shouldSkipNextPromptPrediction(&pending, nil); !skip {
		t.Fatal("expected pendingInteraction session to skip")
	}
	// Background notification user turn -> skip
	bg := base
	bg.AllMessages = []CascadeMessageItem{
		{ID: "u1", Role: "user", Text: "[Cron] nightly sync done"},
		{ID: "a1", Role: "agent", Text: "Acknowledged cron notification."},
	}
	if skip, _ := shouldSkipNextPromptPrediction(&bg, nil); !skip {
		t.Fatal("expected background notification turn to skip")
	}
	// Native suggestion always takes precedence
	withNative := base
	withNative.SuggestedResponses = []string{"Native Option"}
	sugs, src := p.ResolveOrScheduleNextPrompt(base.CascadeID, &withNative, nil, true)
	if len(sugs) != 1 || sugs[0] != "Native Option" || src != SuggestionSourceNative {
		t.Fatalf("expected native suggestion passthrough, got %v (%q)", sugs, src)
	}

	// 2. Sanitization tests (filler, abstention, duplicate completed item)
	checklist := nextPromptChecklist{Completed: []string{"修改核心逻辑并保存文件"}}
	for _, bad := range []string{"NO_SUGGESTION", "继续", "ok", "好的。", "修改核心逻辑并保存文件"} {
		if got := sanitizeNextPromptSuggestion(bad, checklist); got != "" {
			t.Errorf("sanitizeNextPromptSuggestion(%q) = %q, want empty", bad, got)
		}
	}
	if got := sanitizeNextPromptSuggestion(`"运行单元测试确认一下"`, checklist); got != "运行单元测试确认一下" {
		t.Errorf("sanitizeNextPromptSuggestion quoted = %q", got)
	}

	// 3. Async scheduling -> notifies stream listener and populates cache
	doneCh := make(chan struct{}, 1)
	p.lsModelResponseHook = func(ctx context.Context, prompt, modelEnum string) (string, error) {
		defer func() { doneCh <- struct{}{} }()
		if !strings.Contains(prompt, "运行单元测试确认通过") {
			t.Errorf("expected pending step in prompt")
		}
		return "跑一下单元测试确认通过", nil
	}

	sugs, src = p.ResolveOrScheduleNextPrompt(base.CascadeID, &base, nil, true)
	if len(sugs) != 0 || src != "" {
		t.Fatalf("expected immediate empty return while async prediction runs, got %v (%q)", sugs, src)
	}

	select {
	case <-doneCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for async next-prompt prediction")
	}

	// Wait briefly for cache commit after hook returns
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sugs, src = p.ResolveOrScheduleNextPrompt(base.CascadeID, &base, nil, false)
		if len(sugs) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(sugs) != 1 || sugs[0] != "跑一下单元测试确认通过" || src != SuggestionSourcePredicted {
		t.Fatalf("expected cached predicted suggestion, got %v (%q)", sugs, src)
	}
	if sig := p.nextPromptSignature(base.CascadeID); sig == "" {
		t.Fatal("expected non-empty nextPromptSignature after prediction cached")
	}
}

func TestLiveTextNeedsTranslation_MixedLanguageAndLocales(t *testing.T) {
	cases := []struct {
		text string
		lang string
		want bool
	}{
		// Chinese target: pure English & mixed English with CJK proper noun
		{"Update iOS layout, trackpad cursor, and HUD", "zh-Hans", true},
		{"Install to 九仔 and restart mgy", "zh-Hans", true},
		{"Deploy to 九仔", "zh-Hans", true},
		{"Updating Android layout, full model names, and virtual cursor", "zh-Hans", true},
		// Already Chinese (with tech acronyms)
		{"更新 iOS 布局、触控板光标与 HUD", "zh-Hans", false},
		{"安装到九仔并重启 mgy", "zh-Hans", false},
		{"iOS 端适配 HUD 与虚拟光标", "zh-Hans", false},
		// Paths & empty
		{"/Users/hal9000/Projects/antigravity-mobile", "zh-Hans", false},
		{"", "zh-Hans", false},
		// English target -> never translate
		{"Update iOS layout, trackpad cursor, and HUD", "en", false},
		{"更新 iOS 布局", "en-US", false},
		// Japanese target
		{"Update Android layout", "ja", true},
		{"Androidレイアウトを更新", "ja", false},
		// Korean target
		{"Verify iOS, Android, and Web builds", "ko", true},
		{"iOS, Android, Web 빌드 검증", "ko", false},
		// Latin-script non-English (Portuguese)
		{"Verify iOS, Android, and Web builds", "pt-BR", true},
	}
	for _, tc := range cases {
		if got := liveTextNeedsTranslation(tc.text, tc.lang); got != tc.want {
			t.Errorf("liveTextNeedsTranslation(%q, %q) = %v, want %v", tc.text, tc.lang, got, tc.want)
		}
	}
}

func TestLiveTextTranslator_SeparateTogglesCopyOnWriteAndLiveActivity(t *testing.T) {
	t.Setenv("MULTIGRAVITY_DATA_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	p := NewProxy(inspector.NewInspector(time.Second))
	lang := "zh-Hans"
	langName := "Simplified Chinese"
	stepOn := true
	focusOff := false
	if _, err := p.aiAssistPrefs.Update(AIAssistPrefsPatch{
		StepPlanTranslateEnabled: &stepOn,
		FocusTranslateEnabled:    &focusOff,
		TitleTranslateLang:       &lang,
		TitleTranslateLangName:   &langName,
	}); err != nil {
		t.Fatalf("update prefs: %v", err)
	}

	var mu sync.Mutex
	var calls int
	p.lsModelResponseHook = func(ctx context.Context, prompt, modelEnum string) (string, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		if strings.Contains(prompt, "Input JSON:") {
			return `{"1":"更新 iOS 布局与光标","2":"安装到九仔并重启 mgy"}`, nil
		}
		if strings.Contains(prompt, "Status:") {
			return "正在更新 Android 布局与虚拟光标", nil
		}
		return "", nil
	}

	// Build a shared TrajectoryDetails fixture to verify copy-on-write does NOT mutate original backing slices.
	origItems := []StepPlanItemEntry{
		{Key: "update ios layout, trackpad cursor, and hud", Title: "Update iOS layout, trackpad cursor, and HUD", Status: "done"},
		{Key: "install to 九仔 and restart mgy", Title: "Install to 九仔 and restart mgy", Status: "in_progress"},
	}
	origPlan := &StepPlanItem{AnchorStepIndex: 3, Items: origItems, DoneCount: 1, TotalCount: 2}
	origMsgs := []CascadeMessageItem{
		{ID: "step-plan-3", Type: "step_plan", StepPlan: origPlan},
		{ID: "tools-4", Type: "tools", FocusText: "Updating Android layout and virtual cursor"},
	}

	makeDetails := func() TrajectoryDetails {
		return TrajectoryDetails{
			CascadeID:   "cid-live-1",
			Status:      "CASCADE_RUN_STATUS_RUNNING",
			AllMessages: origMsgs,
			ActivePlan:  origPlan,
			ActiveFocus: "Updating Android layout and virtual cursor",
		}
	}

	touchCh, cleanup := p.registerStreamTouchListener("cid-live-1")
	defer cleanup()

	// 1. With only StepPlanTranslateEnabled=true, schedules step plan batch (NOT focus).
	d1 := makeDetails()
	if applied := p.ApplyLiveTextTranslations("cid-live-1", &d1, "", true); len(applied) != 0 {
		t.Fatalf("expected empty immediate overlay on cache miss, got %v", applied)
	}
	select {
	case <-touchCh:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for step-plan translation touch notification")
	}

	d2 := makeDetails()
	applied := p.ApplyLiveTextTranslations("cid-live-1", &d2, "", false)
	if len(applied) != 2 {
		t.Fatalf("expected 2 translated step items in applied map, got %v", applied)
	}
	// Verify translated titles + untouched original Keys!
	sp := d2.AllMessages[0].StepPlan
	if sp.Items[0].Title != "更新 iOS 布局与光标" || sp.Items[0].Key != "update ios layout, trackpad cursor, and hud" {
		t.Fatalf("unexpected translated item[0]: %+v", sp.Items[0])
	}
	if sp.Items[1].Title != "安装到九仔并重启 mgy" || sp.Items[1].Key != "install to 九仔 and restart mgy" {
		t.Fatalf("unexpected translated item[1]: %+v", sp.Items[1])
	}
	if d2.ActivePlan.Items[1].Title != "安装到九仔并重启 mgy" {
		t.Fatalf("expected ActivePlan translated, got %+v", d2.ActivePlan.Items[1])
	}
	// Focus switch is OFF -> ActiveFocus and tools.FocusText MUST remain original English.
	if d2.ActiveFocus != "Updating Android layout and virtual cursor" || d2.AllMessages[1].FocusText != "Updating Android layout and virtual cursor" {
		t.Fatalf("expected focus untouched when focusTranslateEnabled=false, got active=%q msg=%q", d2.ActiveFocus, d2.AllMessages[1].FocusText)
	}
	// Shared original backing slices MUST remain completely untouched (copy-on-write safety)!
	if origItems[0].Title != "Update iOS layout, trackpad cursor, and HUD" || origMsgs[0].StepPlan.Items[0].Title != "Update iOS layout, trackpad cursor, and HUD" {
		t.Fatalf("shared backing slice was mutated! origItems[0]=%q", origItems[0].Title)
	}

	// 2. Flipping step status (pending -> done) reuses cache with ZERO additional model calls!
	mu.Lock()
	callsAfterSteps := calls
	mu.Unlock()
	if callsAfterSteps != 1 {
		t.Fatalf("expected exactly 1 batched model call for 2 step items, got %d", callsAfterSteps)
	}
	d3 := makeDetails()
	p.ApplyLiveTextTranslations("cid-live-1", &d3, "", true)
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	callsAfterCacheHit := calls
	mu.Unlock()
	if callsAfterCacheHit != 1 {
		t.Fatalf("expected 0 extra model calls on cache hit, got %d", callsAfterCacheHit-callsAfterSteps)
	}

	// 3. Turn FocusTranslateEnabled=true and verify EnsureLiveTextTranslatedForLiveActivity translates synchronously!
	focusOn := true
	if _, err := p.aiAssistPrefs.Update(AIAssistPrefsPatch{FocusTranslateEnabled: &focusOn}); err != nil {
		t.Fatalf("enable focus translate: %v", err)
	}
	laDetails := makeDetails()
	p.EnsureLiveTextTranslatedForLiveActivity(context.Background(), &laDetails, "")
	if laDetails.ActiveFocus != "正在更新 Android 布局与虚拟光标" {
		t.Fatalf("expected Live Activity ActiveFocus translated synchronously, got %q", laDetails.ActiveFocus)
	}
	if laDetails.ActivePlan.Items[1].Title != "安装到九仔并重启 mgy" {
		t.Fatalf("expected Live Activity ActivePlan translated, got %q", laDetails.ActivePlan.Items[1].Title)
	}

	// 4. English target language must never translate or issue model calls.
	enLang := "en"
	enName := "English"
	if _, err := p.aiAssistPrefs.Update(AIAssistPrefsPatch{TitleTranslateLang: &enLang, TitleTranslateLangName: &enName}); err != nil {
		t.Fatalf("switch to en: %v", err)
	}
	dEn := makeDetails()
	if appliedEn := p.ApplyLiveTextTranslations("cid-live-1", &dEn, "en-US", true); len(appliedEn) != 0 {
		t.Fatalf("expected no translations for English user, got %v", appliedEn)
	}
	if dEn.ActiveFocus != "Updating Android layout and virtual cursor" {
		t.Fatalf("expected untouched focus for English user, got %q", dEn.ActiveFocus)
	}
}

func TestTitleTranslator_OutputValidationFallbackAndNoTruncation(t *testing.T) {
	t.Setenv("MULTIGRAVITY_DATA_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	// 1. cleanTranslatedTitleOutput must NOT truncate titles longer than 36 runes (up to 80 runes).
	longTranslated := "Antigravity Context Viewer 移动端功能设计与多端适配方案总结"
	if got := cleanTranslatedTitleOutput(longTranslated); got != longTranslated {
		t.Fatalf("cleanTranslatedTitleOutput truncated valid title: got %q, want %q", got, longTranslated)
	}

	// 2. isTranslationAcceptable rejects verbatim/truncated echoes and accepts valid translations with proper nouns.
	if isTranslationAcceptable("Antigravity Context Viewer Mobile Features", "Antigravity Context Viewer Mobile Fe...", "zh-Hans") {
		t.Fatal("expected truncated English echo to be rejected for zh-Hans")
	}
	if isTranslationAcceptable("Draft Message To Advisor", "Draft Message To Advisor", "zh-Hans") {
		t.Fatal("expected verbatim English echo to be rejected for zh-Hans")
	}
	if !isTranslationAcceptable("Antigravity Context Viewer Mobile Features", "Antigravity Context Viewer 移动端功能", "zh-Hans") {
		t.Fatal("expected proper-noun + Chinese title to be accepted for zh-Hans")
	}
	if titleNeedsTranslation("Antigravity Context Viewer 移动端功能", "zh-Hans") {
		t.Fatal("expected already-Chinese title with English proper noun not to require re-translation")
	}
	if !isTranslationAcceptable("Configurar Docker", "Configurar Docker", "pt-BR") {
		t.Fatal("expected unchanged Latin-script target title to remain accepted for pt-BR")
	}

	// 3. When primary model echoes English, translator automatically retries on fallback model and succeeds.
	p := NewProxy(inspector.NewInspector(time.Second))
	var mu sync.Mutex
	written := map[string]string{}
	p.titleWriteHook = func(ctx context.Context, cascadeID, title string) error {
		mu.Lock()
		written[cascadeID] = title
		mu.Unlock()
		return nil
	}

	var seenModels []string
	p.lsModelResponseHook = func(ctx context.Context, prompt, modelEnum string) (string, error) {
		mu.Lock()
		seenModels = append(seenModels, modelEnum)
		mu.Unlock()
		if modelEnum == "MODEL_PLACEHOLDER_M198" {
			var items []struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			}
			out := map[string]string{}
			if idx := strings.Index(prompt, "[{"); idx >= 0 && json.Unmarshal([]byte(prompt[idx:]), &items) == nil {
				for _, it := range items {
					out[it.ID] = it.Title
				}
			}
			b, _ := json.Marshal(out)
			return string(b), nil
		}
		var items []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}
		out := map[string]string{}
		if idx := strings.Index(prompt, "[{"); idx >= 0 && json.Unmarshal([]byte(prompt[idx:]), &items) == nil {
			for _, it := range items {
				if it.Title == "Antigravity Context Viewer Mobile Features" {
					out[it.ID] = "Antigravity Context Viewer 移动端功能"
				} else if it.Title == "Draft Message To Advisor" {
					out[it.ID] = "给导师的消息草稿"
				}
			}
		}
		b, _ := json.Marshal(out)
		return string(b), nil
	}

	candidates := []titleCandidate{
		{CascadeID: "conv-echo-1", SourceTitle: "Antigravity Context Viewer Mobile Features", CurrentTitle: "Antigravity Context Viewer Mobile Features"},
		{CascadeID: "conv-echo-2", SourceTitle: "Draft Message To Advisor", CurrentTitle: "Draft Message To Advisor"},
	}
	translated, failed, err := p.translateAndApplyCandidates(context.Background(), candidates, "zh-Hans", "Simplified Chinese")
	if err != nil || translated != 2 || failed != 0 {
		t.Fatalf("expected 2 fallback-translated titles, got translated=%d failed=%d err=%v", translated, failed, err)
	}
	mu.Lock()
	got1, got2 := written["conv-echo-1"], written["conv-echo-2"]
	modelsUsed := append([]string(nil), seenModels...)
	mu.Unlock()
	if got1 != "Antigravity Context Viewer 移动端功能" || got2 != "给导师的消息草稿" {
		t.Fatalf("unexpected written titles: conv-echo-1=%q conv-echo-2=%q", got1, got2)
	}
	if len(modelsUsed) < 2 || modelsUsed[0] != "MODEL_PLACEHOLDER_M198" || modelsUsed[1] == "MODEL_PLACEHOLDER_M198" {
		t.Fatalf("expected primary M198 followed by non-Lite fallback model, got %v", modelsUsed)
	}

	// 4. If ALL models echo untranslated English, must NOT overwrite titles.
	p2 := NewProxy(inspector.NewInspector(time.Second))
	wroteAny := false
	p2.titleWriteHook = func(ctx context.Context, cascadeID, title string) error {
		wroteAny = true
		return nil
	}
	p2.lsModelResponseHook = func(ctx context.Context, prompt, modelEnum string) (string, error) {
		return "Multigravity Cross Platform Routine Inspection And Review", nil
	}

	trFail, flFail, _ := p2.translateAndApplyCandidates(context.Background(), []titleCandidate{
		{CascadeID: "conv-fail-1", SourceTitle: "Multigravity Cross Platform Routine Inspection And Review", CurrentTitle: "Multigravity Cross Platform Routine Inspection And Review"},
	}, "zh-Hans", "Simplified Chinese")
	if wroteAny || trFail != 0 || flFail != 1 {
		t.Fatalf("expected untranslated echo to fail cleanly without writing, wroteAny=%v tr=%d fl=%d", wroteAny, trFail, flFail)
	}
}

func TestLiveTextTranslator_RejectsEchoAndRetriesFallbackModel(t *testing.T) {
	t.Setenv("MULTIGRAVITY_DATA_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	p := NewProxy(inspector.NewInspector(time.Second))
	lang := "zh-Hans"
	langName := "Simplified Chinese"
	stepOn := true
	focusOn := true
	if _, err := p.aiAssistPrefs.Update(AIAssistPrefsPatch{
		StepPlanTranslateEnabled: &stepOn,
		FocusTranslateEnabled:    &focusOn,
		TitleTranslateLang:       &lang,
		TitleTranslateLangName:   &langName,
	}); err != nil {
		t.Fatalf("update prefs: %v", err)
	}

	p.lsModelResponseHook = func(ctx context.Context, prompt, modelEnum string) (string, error) {
		if modelEnum == "MODEL_PLACEHOLDER_M198" {
			// Primary echoes English untranslated
			if strings.Contains(prompt, "Input JSON:") {
				return `{"1":"Inspect title translation logs"}`, nil
			}
			return "Checking model fallback", nil
		}
		// Fallback translates properly
		if strings.Contains(prompt, "Input JSON:") {
			return `{"1":"排查标题翻译日志"}`, nil
		}
		return "正在检查模型回退", nil
	}

	details := TrajectoryDetails{
		CascadeID:   "cid-live-fallback",
		Status:      "CASCADE_RUN_STATUS_RUNNING",
		ActiveFocus: "Checking model fallback",
		ActivePlan: &StepPlanItem{
			Items: []StepPlanItemEntry{{Key: "inspect title translation logs", Title: "Inspect title translation logs", Status: "in_progress"}},
		},
	}
	p.EnsureLiveTextTranslatedForLiveActivity(context.Background(), &details, "")
	if details.ActiveFocus != "正在检查模型回退" {
		t.Fatalf("expected fallback focus translation, got %q", details.ActiveFocus)
	}
	if details.ActivePlan.Items[0].Title != "排查标题翻译日志" {
		t.Fatalf("expected fallback step-plan translation, got %q", details.ActivePlan.Items[0].Title)
	}
}
