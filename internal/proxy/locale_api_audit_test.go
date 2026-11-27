package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"antigravity-mobile/internal/i18n"
	"antigravity-mobile/internal/inspector"
)

func TestFourLocaleAPIResponses_ZeroChineseLeakForEnAndFr(t *testing.T) {
	prev := i18n.GetNativeLocale()
	t.Cleanup(func() { i18n.SetNativeLocale(prev) })
	// Simulate a Chinese host Mac to prove English and French clients never inherit Chinese chrome.
	i18n.SetNativeLocale("zh-CN")

	p := NewProxy(inspector.NewInspector(time.Second))
	p.SetGatewayVersion("dev")

	// 1. Self-update capability & apply confirm error across 4 locales
	for _, tc := range []struct {
		al        string
		wantNoHan bool
		wantSub   string
	}{
		{"zh-CN", false, "手动更新"},
		{"ja-JP", false, "手動更新"},
		{"en-US", true, "manually on your computer"},
		{"fr-FR,fr;q=0.9", true, "manually on your computer"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/gateway/self/update", nil)
		req.Header.Set("Accept-Language", tc.al)
		rec := httptest.NewRecorder()
		p.selfUpdateCheckFn = func(_ context.Context, _ string, _ bool) (string, error) { return "v1.2.8", nil }
		p.HandleGatewaySelfUpdate(rec, req)
		var st GatewaySelfUpdateStatus
		if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
			t.Fatalf("unmarshal self-update (%s): %v", tc.al, err)
		}
		if tc.wantNoHan && containsHan([]byte(st.CannotUpdateReason)) {
			t.Errorf("self-update reason leaked Han for %s: %q", tc.al, st.CannotUpdateReason)
		}
		if !strings.Contains(st.CannotUpdateReason, tc.wantSub) {
			t.Errorf("self-update reason (%s) = %q, want substring %q", tc.al, st.CannotUpdateReason, tc.wantSub)
		}

		reqApply := httptest.NewRequest(http.MethodPost, "/gateway/self/update/apply", strings.NewReader(`{"confirm":false}`))
		reqApply.Header.Set("Accept-Language", tc.al)
		recApply := httptest.NewRecorder()
		p.HandleGatewaySelfUpdate(recApply, reqApply)
		if tc.wantNoHan && containsHan(recApply.Body.Bytes()) {
			t.Errorf("self-update apply error leaked Han for %s: %s", tc.al, recApply.Body.String())
		}
	}

	// 2. Context Breakdown Top-3 labels across 4 locales
	longThinking := strings.Repeat("reasoning step detail ", 120)
	longCmd := strings.Repeat("go test ./... ", 120)
	steps := []TrajectoryStep{
		{PlannerResponse: &TrajectoryPlannerResponse{Thinking: longThinking, Response: "ok"}},
		{RunCommand: &TrajectoryRunCommand{CommandLine: longCmd}},
	}
	for _, tc := range []struct {
		lang      string
		wantNoHan bool
		wantThink string
		wantTerm  string
	}{
		{"zh", false, "深度思考", "终端命令"},
		{"ja", false, "思考プロセス", "ターミナル"},
		{"en", true, "Thinking", "Terminal"},
		{"fr-FR", true, "Thinking", "Terminal"},
	} {
		cm := applyTrajectoryContextEnrichmentForLang(&ContextMeterItem{UsedTokens: 5000, MaxTokens: 200000}, steps, nil, tc.lang)
		if cm == nil || cm.Breakdown == nil || len(cm.Breakdown.TopConsumers) < 2 {
			t.Fatalf("expected top consumers for %s, got %+v", tc.lang, cm)
		}
		joined := cm.Breakdown.TopConsumers[0].Label + " | " + cm.Breakdown.TopConsumers[1].Label
		if tc.wantNoHan && containsHan([]byte(joined)) {
			t.Errorf("ContextBreakdown leaked Han for %s: %q", tc.lang, joined)
		}
		if !strings.Contains(joined, tc.wantThink) || !strings.Contains(joined, tc.wantTerm) {
			t.Errorf("ContextBreakdown (%s) = %q, want %q and %q", tc.lang, joined, tc.wantThink, tc.wantTerm)
		}
	}

	// 3. Slash Commands (/workflow & /btw) per-request localization
	p.slashCache.mu.Lock()
	p.slashCache.options, p.slashCache.infoByName = buildSlashCommandTables(nil)
	p.slashCache.fetchedAt = time.Now()
	p.slashCache.mu.Unlock()
	for _, tc := range []struct {
		al        string
		wantNoHan bool
		wantBtw   string
	}{
		{"zh-CN", false, "旁路快问"},
		{"ja-JP", false, "サイド質問"},
		{"en-US", true, "Side Question"},
		{"fr-FR", true, "Side Question"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/gateway/slash-commands", nil)
		req.Header.Set("Accept-Language", tc.al)
		rec := httptest.NewRecorder()
		p.HandleSlashCommands(rec, req)
		if tc.wantNoHan && containsHan(rec.Body.Bytes()) {
			t.Errorf("/gateway/slash-commands leaked Han for %s: %s", tc.al, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), tc.wantBtw) {
			t.Errorf("/gateway/slash-commands (%s) missing %q: %s", tc.al, tc.wantBtw, rec.Body.String())
		}
	}

	// 4. BTW prompt formatting across 4 locales
	if got := formatBtwPromptForLang("", "some code", "en-US"); containsHan([]byte(got)) || !strings.Contains(got, "Please explain") {
		t.Errorf("formatBtwPromptForLang(en-US) = %q", got)
	}
	if got := formatBtwPromptForLang("why?", "some code", "fr-FR"); containsHan([]byte(got)) || !strings.Contains(got, "Regarding the following") {
		t.Errorf("formatBtwPromptForLang(fr-FR) = %q", got)
	}
	if got := formatBtwPromptForLang("", "code", "ja"); !strings.Contains(got, "説明してください") {
		t.Errorf("formatBtwPromptForLang(ja) = %q", got)
	}

	// 5. Phase 4 + Automations validation errors never leak Chinese to en-US or fr-FR
	for _, ep := range []struct {
		handler http.HandlerFunc
		path    string
		body    string
	}{
		{p.HandleBrowserViewer, "/gateway/browser/renew", `{}`},
		{p.HandleRemotes, "/gateway/remotes/connect", `{}`},
		{p.HandlePluginManageExtra, "/gateway/plugins/delete", `{}`},
		{p.HandleMcpOAuth, "/gateway/oauth/callback-forward", `{"callbackUrl":"https://evil.example/cb"}`},
		{p.HandleDesktopUpdate, "/gateway/desktop/update/apply", `{"confirm":false}`},
		{p.HandleArtifactSnapshots, "/gateway/artifacts/snapshots", `{}`},
		{p.HandleAutomationCreateDelete, "/gateway/automations/create", `{}`},
	} {
		for _, al := range []string{"en-US", "fr-FR", "ko-KR"} {
			req := httptest.NewRequest(http.MethodPost, ep.path, strings.NewReader(ep.body))
			req.Header.Set("Accept-Language", al)
			rec := httptest.NewRecorder()
			ep.handler(rec, req)
			if containsHan(rec.Body.Bytes()) {
				t.Errorf("%s (%s) leaked Chinese: %s", ep.path, al, rec.Body.String())
			}
		}
	}

	// 6. Local sidecar automation event default message across 4 locales
	home := t.TempDir()
	t.Setenv("HOME", home)
	evDir := filepath.Join(home, ".gemini", "antigravity", "sidecar_data", "sched-test", "events")
	if err := os.MkdirAll(evDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evDir, "20261127_120000.json"), []byte(`{"timestampMs":"1795665600000"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if evs := readLocalSidecarEventsForLang("sched-test", "Test", 5, "en"); len(evs) != 1 || evs[0].Message != "Automation completed" {
		t.Errorf("sidecar event (en) = %+v", evs)
	}
	if evs := readLocalSidecarEventsForLang("sched-test", "Test", 5, "fr-FR"); len(evs) != 1 || evs[0].Message != "Automation completed" {
		t.Errorf("sidecar event (fr-FR) = %+v", evs)
	}
	if evs := readLocalSidecarEventsForLang("sched-test", "Test", 5, "ja"); len(evs) != 1 || !strings.Contains(evs[0].Message, "オートメーション") {
		t.Errorf("sidecar event (ja) = %+v", evs)
	}
}
