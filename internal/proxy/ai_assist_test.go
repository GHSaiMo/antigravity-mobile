package proxy

import (
	"bytes"
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
	t.Setenv("MGY_AI_NEXT_PROMPT_TAP", "")

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
	if got.Prefs.NextPromptTapAction != NextPromptTapFill {
		t.Fatalf("expected default tap action 'fill', got %q", got.Prefs.NextPromptTapAction)
	}
	if got.ModelID == "" || got.ModelName == "" {
		t.Fatalf("expected resolved lightweight model, got %+v", got)
	}

	// POST partial patch
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
	if reloaded.TitleTranslateLang != "pt-BR" || reloaded.TitleTranslateLangName != "Portuguese" {
		t.Fatalf("expected persisted target language pt-BR/Portuguese, got %+v", reloaded)
	}
	if reloaded.NextPromptTapAction != NextPromptTapSend {
		t.Fatalf("expected persisted tap action 'send', got %q", reloaded.NextPromptTapAction)
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

func TestTitleTranslator_BatchTranslatePreserveUserRenameAndRestore(t *testing.T) {
	t.Setenv("MULTIGRAVITY_DATA_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	p := NewProxy(inspector.NewInspector(time.Second))

	var mu sync.Mutex
	written := map[string]string{}
	p.titleWriteHook = func(ctx context.Context, cascadeID, title string) error {
		mu.Lock()
		written[cascadeID] = title
		mu.Unlock()
		return nil
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	p.summariesFetchHook = func(ctx context.Context) (map[string]map[string]interface{}, error) {
		return map[string]map[string]interface{}{
			"conv-en-1": {
				"summary":          "Antigravity Context Viewer Prompt Principles",
				"annotations":      map[string]interface{}{"title": "Antigravity Context Viewer Prompt Principles"},
				"lastModifiedTime": nowStr,
			},
			"conv-en-archived": {
				"summary":          "Old Archived English Chat",
				"annotations":      map[string]interface{}{"title": "Old Archived English Chat", "archived": true},
				"lastModifiedTime": nowStr,
			},
			"conv-user-renamed": {
				"summary":          "Auto Generated English Title",
				"annotations":      map[string]interface{}{"title": "My Custom English Name"},
				"lastModifiedTime": nowStr,
			},
			"conv-zh-already": {
				"summary":          "中东地缘局势分析",
				"annotations":      map[string]interface{}{"title": "中东地缘局势分析"},
				"lastModifiedTime": nowStr,
			},
			"conv-subagent": {
				"summary":            "Research subagent task",
				"trajectoryMetadata": map[string]interface{}{"parentConversationId": "conv-en-1"},
				"lastModifiedTime":   nowStr,
			},
			"conv-en-2": {
				"summary":          "Debug Xcode Wireless Connection",
				"annotations":      map[string]interface{}{"title": "Debug Xcode Wireless Connection"},
				"lastModifiedTime": nowStr,
			},
			"conv-en-3": {
				"summary":          "Optimize Stream Delta Cache",
				"annotations":      map[string]interface{}{"title": "Optimize Stream Delta Cache"},
				"lastModifiedTime": nowStr,
			},
		}, nil
	}

	modelCalls := 0
	p.lsModelResponseHook = func(ctx context.Context, prompt, modelEnum string) (string, error) {
		modelCalls++
		if !strings.Contains(prompt, "Simplified Chinese") {
			t.Errorf("expected Simplified Chinese in prompt, got: %s", prompt)
		}
		if strings.Contains(prompt, "Input JSON:") {
			// Batch prompt: return mapped translations keyed by 1-based index matching input order
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

	// Mark conv-en-2 as manually edited first -> should be skipped!
	p.MarkConversationTitleUserEdited("conv-en-2", "Debug Xcode Wireless Connection")

	// Run batch translation (default includeArchived=false)
	req := httptest.NewRequest(http.MethodPost, "/gateway/ai-assist/translate-titles",
		bytes.NewBufferString(`{"targetLang":"zh-Hans","targetLangName":"Simplified Chinese"}`))
	rec := httptest.NewRecorder()
	p.HandleTranslateTitles(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("translate-titles status = %d: %s", rec.Code, rec.Body.String())
	}

	var res struct {
		OK              bool `json:"ok"`
		Scanned         int  `json:"scanned"`
		Translated      int  `json:"translated"`
		Skipped         int  `json:"skipped"`
		Failed          int  `json:"failed"`
		TranslatedCount int  `json:"translatedCount"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal translate-titles: %v", err)
	}
	if !res.OK || res.Translated != 2 || res.Failed != 0 || res.TranslatedCount != 2 {
		t.Fatalf("unexpected translate result: %+v (body=%s)", res, rec.Body.String())
	}
	if modelCalls != 1 {
		t.Fatalf("expected 1 batched model call for 2 eligible items, got %d", modelCalls)
	}
	mu.Lock()
	gotTitle := written["conv-en-1"]
	gotTitle3 := written["conv-en-3"]
	_, wroteArchived := written["conv-en-archived"]
	_, wroteCustom := written["conv-user-renamed"]
	_, wroteEdited := written["conv-en-2"]
	mu.Unlock()
	if gotTitle != "Antigravity Context Viewer 提示词原则" || gotTitle3 != "优化 Stream Delta 缓存" {
		t.Fatalf("unexpected translated titles: conv-en-1=%q conv-en-3=%q", gotTitle, gotTitle3)
	}
	if wroteArchived || wroteCustom || wroteEdited {
		t.Fatalf("must not translate archived/user-renamed/user-edited conversations: %+v", written)
	}

	// Re-running batch translation must be idempotent (0 model calls)
	prevCalls := modelCalls
	// Update mock summaries to reflect translated conv-en-1 & conv-en-3
	p.summariesFetchHook = func(ctx context.Context) (map[string]map[string]interface{}, error) {
		return map[string]map[string]interface{}{
			"conv-en-1": {
				"summary":          "Antigravity Context Viewer Prompt Principles",
				"annotations":      map[string]interface{}{"title": gotTitle},
				"lastModifiedTime": nowStr,
			},
			"conv-en-3": {
				"summary":          "Optimize Stream Delta Cache",
				"annotations":      map[string]interface{}{"title": gotTitle3},
				"lastModifiedTime": nowStr,
			},
		}, nil
	}
	req2 := httptest.NewRequest(http.MethodPost, "/gateway/ai-assist/translate-titles",
		bytes.NewBufferString(`{"targetLang":"zh-Hans"}`))
	rec2 := httptest.NewRecorder()
	p.HandleTranslateTitles(rec2, req2)
	if modelCalls != prevCalls {
		t.Fatalf("expected idempotent 0 additional model calls, got %d", modelCalls-prevCalls)
	}

	// Restore titles must put original English title back
	reqRestore := httptest.NewRequest(http.MethodPost, "/gateway/ai-assist/restore-titles", nil)
	recRestore := httptest.NewRecorder()
	p.HandleRestoreTitles(recRestore, reqRestore)
	if recRestore.Code != http.StatusOK {
		t.Fatalf("restore-titles status = %d: %s", recRestore.Code, recRestore.Body.String())
	}
	mu.Lock()
	restoredTitle := written["conv-en-1"]
	mu.Unlock()
	if restoredTitle != "Antigravity Context Viewer Prompt Principles" {
		t.Fatalf("expected restored original title, got %q", restoredTitle)
	}
	if p.titleTranslator.RestorableCount() != 0 {
		t.Fatalf("expected 0 restorable entries after restore, got %d", p.titleTranslator.RestorableCount())
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
