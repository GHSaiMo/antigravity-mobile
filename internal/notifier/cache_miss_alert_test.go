package notifier

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/i18n"
	"antigravity-mobile/internal/inspector"
	"antigravity-mobile/internal/proxy"
)

func TestCacheMissAlertDefaultsToShadowAndSuppressesPush(t *testing.T) {
	t.Setenv("CACHE_MISS_ALERT_MODE", "")
	t.Setenv("CACHE_MISS_ALERT_ENABLED", "")

	if got := CacheMissAlertMode(); got != CacheMissModeShadow {
		t.Fatalf("expected default mode %q, got %q", CacheMissModeShadow, got)
	}

	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "audit.json")
	reportPath := filepath.Join(tmpDir, "private-docs", "cache_miss_shadow_audit_report.md")

	n, s := newQuotaTestNotifier()
	aud := NewCacheMissShadowAuditor(statePath, reportPath)
	n.SetCacheMissAuditor(aud)

	if err := n.NotifyCacheMissAlert("cas-shadow", "Neon Refactor", 3, 180000); err != nil {
		t.Fatal(err)
	}
	if len(s.sent) != 0 {
		t.Fatalf("shadow mode must NOT send push notifications, got %d: %+v", len(s.sent), s.sent)
	}

	sum := aud.Summary()
	if sum.TotalEpisodes != 1 || sum.TotalWouldNotify != 1 || sum.OngoingEpisodes != 1 {
		t.Fatalf("unexpected shadow summary after trigger: %+v", sum)
	}

	// Second trigger within 1h cooldown after resolution should mark would_have_notified=false
	hr := 92.5
	if err := n.ObserveCacheMiss(CacheMissObservation{
		CascadeID:       "cas-shadow",
		SessionTitle:    "Neon Refactor",
		Model:           "Gemini 4 Neon (XHigh)",
		Source:          "watcher",
		Streak:          0,
		InputTokens:     3000,
		CacheReadTokens: 37000,
		CacheHitRate:    &hr,
	}); err != nil {
		t.Fatal(err)
	}

	sum = aud.Summary()
	if sum.SelfRecoveredCount != 1 || sum.OngoingEpisodes != 0 || sum.SelfRecoveryRatePct != 100.0 {
		t.Fatalf("expected 1 self-recovered episode (100%%), got %+v", sum)
	}

	// Verify Markdown report written cleanly to target path
	reportRaw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("expected shadow Markdown report at %s: %v", reportPath, err)
	}
	report := string(reportRaw)
	for _, want := range []string{
		"影子审计模式",
		"前端胶囊未命中角标",
		"观察期已静默",
		"Neon Refactor",
		"自动恢复",
		"阈值反事实模拟",
		"原本会推送打扰次数",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("shadow report missing %q:\n%s", want, report)
		}
	}

	// Verify JSON state reloads across gateway restarts
	reloaded := NewCacheMissShadowAuditor(statePath, reportPath)
	rSum := reloaded.Summary()
	if rSum.TotalEpisodes != 1 || rSum.SelfRecoveredCount != 1 || rSum.TotalWouldNotify != 1 {
		t.Fatalf("reloaded shadow audit state mismatch: %+v", rSum)
	}
}

func TestCacheMissAlertModeAndPreferenceOff(t *testing.T) {
	t.Setenv("CACHE_MISS_ALERT_MODE", "off")
	if got := CacheMissAlertMode(); got != CacheMissModeOff {
		t.Fatalf("expected mode=off, got %q", got)
	}
	nOff, sOff := newQuotaTestNotifier()
	if err := nOff.NotifyCacheMissAlert("cas-off", "Demo", 4, 200000); err != nil {
		t.Fatal(err)
	}
	if len(sOff.sent) != 0 || nOff.CacheMissAuditor().Summary().TotalEpisodes != 0 {
		t.Fatalf("mode=off must suppress both push and shadow audit")
	}

	// Legacy CACHE_MISS_ALERT_ENABLED=false also disables
	t.Setenv("CACHE_MISS_ALERT_MODE", "")
	t.Setenv("CACHE_MISS_ALERT_ENABLED", "false")
	if got := CacheMissAlertMode(); got != CacheMissModeOff {
		t.Fatalf("expected legacy false -> mode=off, got %q", got)
	}

	// AlertPreferences.CacheMiss=false pauses shadow auditing even in shadow mode
	t.Setenv("CACHE_MISS_ALERT_ENABLED", "")
	nPaused, sPaused := newQuotaTestNotifier()
	_, _ = nPaused.UpdateAlertPreferences(AlertPreferencesPatch{CacheMiss: boolPtr(false)})
	if err := nPaused.NotifyCacheMissAlert("cas-paused", "Demo", 4, 200000); err != nil {
		t.Fatal(err)
	}
	if len(sPaused.sent) != 0 || nPaused.CacheMissAuditor().Summary().TotalEpisodes != 0 {
		t.Fatalf("disabling AlertPreferences.CacheMiss should pause shadow auditing")
	}
}

func TestNotifyCacheMissAlertLiveModeThresholdsDedupAndI18n(t *testing.T) {
	t.Setenv("CACHE_MISS_ALERT_MODE", "live")
	prevLang := i18n.GetLang()
	t.Cleanup(func() { i18n.SetLang(prevLang) })

	// Below thresholds -> silent no-op
	n, s := newQuotaTestNotifier()
	if err := n.NotifyCacheMissAlert("cas-low", "Demo", 2, 150000); err != nil {
		t.Fatal(err)
	}
	if err := n.NotifyCacheMissAlert("cas-low", "Demo", 3, 90000); err != nil {
		t.Fatal(err)
	}
	if len(s.sent) != 0 {
		t.Fatalf("expected 0 alerts below thresholds, got %d", len(s.sent))
	}

	// Trilingual check + dedup in explicit live mode
	for _, tc := range []struct {
		lang      string
		wantTitle string
		wantSub   string
	}{
		{"zh", "⚠️ 缓存连续未命中预警", "连续 3 步未命中缓存"},
		{"en", "⚠️ Prompt Cache Miss Alert", "3 consecutive cache misses (135K uncached input)"},
		{"ja", "⚠️ キャッシュ連続ミス警告", "3 ステップ連続でキャッシュ未命中"},
	} {
		i18n.SetLang(tc.lang)
		tn, ts := newQuotaTestNotifier()
		if err := tn.NotifyCacheMissAlert("cas-1", "Refactor API", 3, 135000); err != nil {
			t.Fatal(err)
		}
		// Duplicate within 1h cooldown must be suppressed
		if err := tn.NotifyCacheMissAlert("cas-1", "Refactor API", 4, 180000); err != nil {
			t.Fatal(err)
		}
		if len(ts.sent) != 1 {
			t.Fatalf("[%s] expected 1 deduplicated alert, got %d", tc.lang, len(ts.sent))
		}
		p := ts.sent[0]
		if p.Title != tc.wantTitle {
			t.Errorf("[%s] title=%q, want %q", tc.lang, p.Title, tc.wantTitle)
		}
		if !strings.Contains(p.Body, tc.wantSub) || !strings.Contains(p.Body, "Refactor API") {
			t.Errorf("[%s] unexpected body=%q", tc.lang, p.Body)
		}
		if p.Category != "cache_miss_alert" || p.Level != "timeSensitive" || p.URL != "antigravity://cascade/cas-1" {
			t.Errorf("[%s] unexpected payload metadata: %+v", tc.lang, p)
		}
	}
}

func TestWatcherShadowAuditsCacheMissRecoveryAndSessionEnd(t *testing.T) {
	t.Setenv("CACHE_MISS_ALERT_MODE", "")
	t.Setenv("CACHE_MISS_ALERT_ENABLED", "")

	prevLang := i18n.GetLang()
	t.Cleanup(func() { i18n.SetLang(prevLang) })
	i18n.SetLang("zh")

	var mu sync.Mutex
	statusVal := "CASCADE_RUN_STATUS_RUNNING"
	stepVal := 8
	metaJSON := `{
		"generatorMetadata": [
			{"chatModel": {"modelDisplayName": "Gemini 4 Neon (XHigh)", "usage": {"inputTokens": "30000", "cacheReadTokens": "0", "outputTokens": "500"}}},
			{"chatModel": {"modelDisplayName": "Gemini 4 Neon (XHigh)", "usage": {"inputTokens": "42000", "cacheReadTokens": "0", "outputTokens": "500"}}},
			{"chatModel": {"modelDisplayName": "Gemini 4 Neon (XHigh)", "usage": {"inputTokens": "45000", "cacheReadTokens": "0", "outputTokens": "500"}}},
			{"chatModel": {"modelDisplayName": "Gemini 4 Neon (XHigh)", "usage": {"inputTokens": "48000", "cacheReadTokens": "0", "outputTokens": "500"}}}
		]
	}`

	var metaCalls atomic.Int32
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		st := statusVal
		steps := stepVal
		mj := metaJSON
		mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			_, _ = w.Write([]byte(`{
				"trajectorySummaries": {
					"cas_cache_shadow": {
						"summary": "Long Neon Refactor",
						"status": "` + st + `",
						"stepCount": ` + string(rune('0'+steps)) + `,
						"lastModifiedTime": "2026-11-24T01:00:00Z"
					}
				}
			}`))
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			stepStatus := "CORTEX_STEP_STATUS_RUNNING"
			if st != "CASCADE_RUN_STATUS_RUNNING" {
				stepStatus = "CORTEX_STEP_STATUS_DONE"
			}
			_, _ = w.Write([]byte(`{
				"status": "` + st + `",
				"trajectory": {
					"trajectoryId": "cas_cache_shadow",
					"cascadeId": "cas_cache_shadow",
					"steps": [
						{"type": "CORTEX_STEP_TYPE_USER_INPUT", "status": "CORTEX_STEP_STATUS_DONE"},
						{"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE", "status": "` + stepStatus + `"}
					]
				}
			}`))
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectoryGeneratorMetadata"):
			metaCalls.Add(1)
			_, _ = w.Write([]byte(mj))
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	p := proxy.NewProxy(inspector.NewInspector(10 * time.Second))
	p.SetTestUpstream(port, "test-token")

	tmpDir := t.TempDir()
	reportPath := filepath.Join(tmpDir, "report.md")
	statePath := filepath.Join(tmpDir, "state.json")

	sender := &captureSender{}
	notif := NewNotifierWithSender(config.NotificationConfig{Enabled: true, Group: "Antigravity", SoundAction: "alarm"}, sender)
	aud := NewCacheMissShadowAuditor(statePath, reportPath)
	notif.SetCacheMissAuditor(aud)

	w := NewWatcher(p, notif)
	w.cacheGuardInterval = 15 * time.Second
	curTime := time.Date(2026, 11, 24, 4, 30, 0, 0, time.UTC)
	w.nowFunc = func() time.Time { return curTime }

	// Baseline sync
	w.scanOnce()
	// First active scan records shadow trigger WITHOUT sending any push
	curTime = curTime.Add(5 * time.Second)
	w.scanOnce()
	if metaCalls.Load() != 1 {
		t.Fatalf("expected 1 generator metadata check, got %d", metaCalls.Load())
	}
	if len(sender.sent) != 0 {
		t.Fatalf("shadow mode watcher must NOT send push alert, got %d: %+v", len(sender.sent), sender.sent)
	}
	sum := aud.Summary()
	if sum.TotalEpisodes != 1 || sum.OngoingEpisodes != 1 || sum.TotalWouldNotify != 1 {
		t.Fatalf("expected 1 ongoing shadow episode, got %+v", sum)
	}
	if len(sum.RecentEpisodes) == 0 || sum.RecentEpisodes[0].Model != "Gemini 4 Neon (XHigh)" {
		t.Fatalf("expected model attribution Gemini 4 Neon (XHigh), got %+v", sum.RecentEpisodes)
	}

	// Immediate second scan must be throttled
	w.scanOnce()
	if metaCalls.Load() != 1 {
		t.Fatalf("expected throttled cache guard (still 1 RPC), got %d", metaCalls.Load())
	}

	// Session finishes without recovering -> episode closes as session_ended
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	statusVal = "CASCADE_RUN_STATUS_IDLE"
	stepVal = 9
	mu.Unlock()
	curTime = curTime.Add(20 * time.Second)
	w.scanOnce()

	sum = aud.Summary()
	if sum.OngoingEpisodes != 0 || sum.ResolvedCount != 1 || sum.RecentEpisodes[0].Status != "session_ended" {
		t.Fatalf("expected episode closed as session_ended, got %+v", sum)
	}
}

func TestWatcherLiveModeTriggersCacheMissAlertAndThrottlesChecks(t *testing.T) {
	t.Setenv("CACHE_MISS_ALERT_MODE", "live")
	prevLang := i18n.GetLang()
	t.Cleanup(func() { i18n.SetLang(prevLang) })
	i18n.SetLang("zh")

	var metaCalls atomic.Int32
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			_, _ = w.Write([]byte(`{
				"trajectorySummaries": {
					"cas_cache_burn_live": {
						"summary": "Long Neon Refactor",
						"status": "CASCADE_RUN_STATUS_RUNNING",
						"stepCount": 8,
						"lastModifiedTime": "2026-11-24T01:00:00Z"
					}
				}
			}`))
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			_, _ = w.Write([]byte(`{
				"status": "CASCADE_RUN_STATUS_RUNNING",
				"trajectory": {
					"trajectoryId": "cas_cache_burn_live",
					"cascadeId": "cas_cache_burn_live",
					"steps": [
						{"type": "CORTEX_STEP_TYPE_USER_INPUT", "status": "CORTEX_STEP_STATUS_DONE"},
						{"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE", "status": "CORTEX_STEP_STATUS_RUNNING"}
					]
				}
			}`))
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectoryGeneratorMetadata"):
			metaCalls.Add(1)
			_, _ = w.Write([]byte(`{
				"generatorMetadata": [
					{"chatModel": {"usage": {"inputTokens": "30000", "cacheReadTokens": "0", "outputTokens": "500"}}},
					{"chatModel": {"usage": {"inputTokens": "42000", "cacheReadTokens": "0", "outputTokens": "500"}}},
					{"chatModel": {"usage": {"inputTokens": "45000", "cacheReadTokens": "0", "outputTokens": "500"}}},
					{"chatModel": {"usage": {"inputTokens": "48000", "cacheReadTokens": "0", "outputTokens": "500"}}}
				]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	p := proxy.NewProxy(inspector.NewInspector(10 * time.Second))
	p.SetTestUpstream(port, "test-token")

	sender := &captureSender{}
	notif := NewNotifierWithSender(config.NotificationConfig{Enabled: true, Group: "Antigravity", SoundAction: "alarm"}, sender)
	w := NewWatcher(p, notif)
	w.cacheGuardInterval = 30 * time.Second

	// Baseline sync
	w.scanOnce()
	if len(sender.sent) != 0 {
		t.Fatalf("expected 0 alerts on baseline sync, got %d", len(sender.sent))
	}

	// First active scan checks cache guard and dispatches alert in live mode
	w.scanOnce()
	if metaCalls.Load() != 1 {
		t.Fatalf("expected 1 generator metadata check, got %d", metaCalls.Load())
	}
	sender.mu.Lock()
	gotCount := len(sender.sent)
	var first PushPayload
	if gotCount > 0 {
		first = sender.sent[0]
	}
	sender.mu.Unlock()
	if gotCount != 1 {
		t.Fatalf("expected 1 cache miss alert in live mode, got %d", gotCount)
	}
	if first.Category != "cache_miss_alert" || !strings.Contains(first.Body, "135K") {
		t.Fatalf("unexpected cache miss alert payload: %+v", first)
	}

	// Second immediate scan must be throttled (no extra generator metadata RPC)
	w.scanOnce()
	if metaCalls.Load() != 1 {
		t.Fatalf("expected throttled cache guard (still 1 RPC), got %d", metaCalls.Load())
	}
}

func TestResolveCacheMissAuditReportPath(t *testing.T) {
	custom := filepath.Join(t.TempDir(), "custom_report.md")
	t.Setenv("CACHE_MISS_AUDIT_REPORT_PATH", custom)
	if got := ResolveCacheMissAuditReportPath("/tmp/data"); got != custom {
		t.Fatalf("expected env override %q, got %q", custom, got)
	}

	t.Setenv("CACHE_MISS_AUDIT_REPORT_PATH", "")
	tmpRepo := t.TempDir()
	privDir := filepath.Join(tmpRepo, "private-docs")
	if err := os.MkdirAll(privDir, 0755); err != nil {
		t.Fatal(err)
	}
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(tmpRepo); err != nil {
		t.Fatal(err)
	}
	got := ResolveCacheMissAuditReportPath("/tmp/data")
	// Resolve symlinks (macOS /var -> /private/var)
	wantEval, _ := filepath.EvalSymlinks(privDir)
	gotDirEval, _ := filepath.EvalSymlinks(filepath.Dir(got))
	if gotDirEval != wantEval || filepath.Base(got) != cacheMissAuditReportFilename {
		t.Fatalf("expected report under %q, got %q", privDir, got)
	}
}
