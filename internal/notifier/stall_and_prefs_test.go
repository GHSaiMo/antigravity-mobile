package notifier

import (
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/i18n"
	"antigravity-mobile/internal/inspector"
	"antigravity-mobile/internal/proxy"
)

func boolPtr(v bool) *bool { return &v }

func TestAlertPreferencesStorePersistenceAndPartialUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notification_prefs.json")
	store := NewAlertPreferencesStore(path)

	got := store.Get()
	if !got.TaskCompletion || !got.TaskStall || !got.CacheMiss || !got.QuotaLow || !got.QuotaBonusReset || !got.QuotaAnomaly {
		t.Fatalf("expected all alerts enabled by default, got %+v", got)
	}
	if got.StallMinutes != defaultStallAlertMinutes {
		t.Fatalf("expected default StallMinutes=%d, got %d", defaultStallAlertMinutes, got.StallMinutes)
	}

	updated, err := store.Update(AlertPreferencesPatch{
		TaskStall:       boolPtr(false),
		QuotaBonusReset: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.TaskStall || updated.QuotaBonusReset {
		t.Fatalf("expected TaskStall & QuotaBonusReset disabled, got %+v", updated)
	}
	if !updated.TaskCompletion || !updated.CacheMiss || !updated.QuotaLow || !updated.QuotaAnomaly {
		t.Fatalf("unrelated preferences should remain true, got %+v", updated)
	}

	// Reload from disk to verify persistence across gateway restarts
	reloaded := NewAlertPreferencesStore(path).Get()
	if reloaded.TaskStall || reloaded.QuotaBonusReset {
		t.Fatalf("expected persisted disabled flags after reload, got %+v", reloaded)
	}
	if !reloaded.TaskCompletion || !reloaded.CacheMiss || !reloaded.QuotaLow || !reloaded.QuotaAnomaly {
		t.Fatalf("expected persisted enabled flags after reload, got %+v", reloaded)
	}
}

func TestNotifierRespectsAlertPreferencesAcrossCategories(t *testing.T) {
	n, s := newQuotaTestNotifier()

	_, _ = n.UpdateAlertPreferences(AlertPreferencesPatch{
		TaskCompletion:  boolPtr(false),
		TaskStall:       boolPtr(false),
		CacheMiss:       boolPtr(false),
		QuotaLow:        boolPtr(false),
		QuotaBonusReset: boolPtr(false),
		QuotaAnomaly:    boolPtr(false),
	})

	_ = n.NotifyCompleted("c1", "Demo", 5)
	_ = n.NotifyFailed("c1", "Demo", 5)
	_ = n.NotifyStallAlert("c1", "Demo", 10, 5)
	_ = n.NotifyCacheMissAlert("c1", "Demo", 4, 180000)
	_ = n.NotifyQuotaAlertWithLevel("warn", "k-warn", "Low", "Body")
	_ = n.NotifyQuotaAlertWithLevel("critical", "k-crit", "Crit", "Body")
	_ = n.NotifyQuotaAlertWithLevel("bonus_reset", "k-bonus", "Reset", "Body")
	_ = n.NotifyQuotaAlertWithLevel("anomaly", "k-anom", "Drop", "Body")

	if len(s.sent) != 0 {
		t.Fatalf("expected 0 alerts when all optional categories disabled, got %d: %+v", len(s.sent), s.sent)
	}

	// Interactive approval request must STILL fire (always follows master push switch)
	_ = n.NotifyAction("c1", "Demo", &proxy.PendingInteraction{Type: "run_command", Target: "go test", StepIndex: 3})
	if len(s.sent) != 1 {
		t.Fatalf("expected approval alert to bypass optional category toggles, got %d", len(s.sent))
	}

	// Re-enable only bonus_reset and task_stall
	_, _ = n.UpdateAlertPreferences(AlertPreferencesPatch{
		QuotaBonusReset: boolPtr(true),
		TaskStall:       boolPtr(true),
	})
	_ = n.NotifyQuotaAlertWithLevel("warn", "k-warn-2", "Low", "Body")
	_ = n.NotifyQuotaAlertWithLevel("bonus_reset", "k-bonus-2", "Reset", "Body")
	_ = n.NotifyStallAlert("c2", "Long Task", 8, 12)

	if len(s.sent) != 3 {
		t.Fatalf("expected 3 total pushes (action + bonus_reset + stall), got %d: %+v", len(s.sent), s.sent)
	}
	if s.sent[1].Category != "quota_alert" || s.sent[2].Category != "task_stall_alert" {
		t.Fatalf("unexpected alert categories: %+v", s.sent)
	}
}

func TestNotifyStallAlertTrilingualAndDedup(t *testing.T) {
	prevLang := i18n.GetLang()
	t.Cleanup(func() { i18n.SetLang(prevLang) })

	for _, tc := range []struct {
		lang      string
		wantTitle string
		wantSub   string
	}{
		{"zh", "⏳ 任务长时间无进展", "已连续 8 分钟无新输出或步骤推进"},
		{"en", "⏳ Task Appears Stalled", "No new output or step progress for 8 min"},
		{"ja", "⏳ タスクが停止している可能性", "8 分間新しい出力やステップの進行がありません"},
	} {
		i18n.SetLang(tc.lang)
		n, s := newQuotaTestNotifier()
		_ = n.NotifyStallAlert("cas-stall", "Build Release", 8, 20)
		// Same step within 1h dedup window must not repeat
		_ = n.NotifyStallAlert("cas-stall", "Build Release", 12, 20)
		if len(s.sent) != 1 {
			t.Fatalf("[%s] expected 1 deduplicated stall alert, got %d", tc.lang, len(s.sent))
		}
		if s.sent[0].Title != tc.wantTitle || !strings.Contains(s.sent[0].Body, tc.wantSub) {
			t.Errorf("[%s] got (%q, %q), want title %q and body containing %q", tc.lang, s.sent[0].Title, s.sent[0].Body, tc.wantTitle, tc.wantSub)
		}
	}
}

func TestWatcherStallDetectionProgressResetAndExemptions(t *testing.T) {
	prevLang := i18n.GetLang()
	t.Cleanup(func() { i18n.SetLang(prevLang) })
	i18n.SetLang("zh")

	var mu sync.Mutex
	curThinking := "initial thought"
	hasPendingCmd := false
	hasBgTask := false

	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		thinking := curThinking
		pending := hasPendingCmd
		bg := hasBgTask
		mu.Unlock()

		switch {
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			_, _ = w.Write([]byte(`{
				"trajectorySummaries": {
					"cas_stall_watch": {
						"summary": "Deep Neon Refactor",
						"status": "CASCADE_RUN_STATUS_RUNNING",
						"stepCount": 6,
						"lastModifiedTime": "2026-11-24T01:00:00Z"
					}
				}
			}`))
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			extraStep := ""
			if pending {
				extraStep = `,{"type":"CORTEX_STEP_TYPE_RUN_COMMAND","status":"CORTEX_STEP_STATUS_WAITING","runCommand":{"commandLine":"rm -rf tmp"}}`
			} else if bg {
				extraStep = `,{"type":"CORTEX_STEP_TYPE_RUN_COMMAND","status":"CORTEX_STEP_STATUS_RUNNING","runCommand":{"commandLine":"go test ./..."}}`
			}
			_, _ = w.Write([]byte(`{
				"status": "CASCADE_RUN_STATUS_RUNNING",
				"trajectory": {
					"trajectoryId": "cas_stall_watch",
					"cascadeId": "cas_stall_watch",
					"steps": [
						{"type": "CORTEX_STEP_TYPE_USER_INPUT", "status": "CORTEX_STEP_STATUS_DONE"},
						{"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE", "status": "CORTEX_STEP_STATUS_RUNNING", "plannerResponse": {"thinking": "` + thinking + `"}}` + extraStep + `
					]
				}
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
	w.stallThreshold = 8 * time.Minute
	w.stallBgThreshold = 15 * time.Minute

	curTime := time.Date(2026, 11, 24, 4, 0, 0, 0, time.UTC)
	w.nowFunc = func() time.Time { return curTime }

	// Baseline sync
	w.scanOnce()
	// First RUNNING scan sets progress baseline
	curTime = curTime.Add(10 * time.Second)
	w.scanOnce()

	// 7 minutes with no change -> below 8m threshold, must NOT alert
	curTime = curTime.Add(7 * time.Minute)
	w.scanOnce()
	if len(sender.sent) != 0 {
		t.Fatalf("expected 0 alerts at 7m, got %d", len(sender.sent))
	}

	// Thinking grows -> progress signature changes, resets timer!
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	curThinking = "initial thought + more deep reasoning"
	mu.Unlock()
	curTime = curTime.Add(10 * time.Second)
	w.scanOnce()

	// Another 7 minutes -> still under 8m since last thinking growth
	curTime = curTime.Add(7 * time.Minute)
	w.scanOnce()
	if len(sender.sent) != 0 {
		t.Fatalf("expected thinking growth to reset stall timer, got %d alerts", len(sender.sent))
	}

	// Waiting for approval for 10 minutes -> must NOT trigger stall alert (only approval alert)
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	hasPendingCmd = true
	mu.Unlock()
	curTime = curTime.Add(10 * time.Minute)
	w.scanOnce()
	for _, msg := range sender.sent {
		if msg.Category == "task_stall_alert" {
			t.Fatalf("approval wait must not trigger stall alert: %+v", msg)
		}
	}

	// Clear approval, simulate active background command: 10m should NOT alert (bg threshold = 15m)
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	hasPendingCmd = false
	hasBgTask = true
	mu.Unlock()
	curTime = curTime.Add(5 * time.Second)
	w.scanOnce()
	curTime = curTime.Add(10 * time.Minute)
	w.scanOnce()
	for _, msg := range sender.sent {
		if msg.Category == "task_stall_alert" {
			t.Fatalf("active background command should relax threshold to 15m, got alert at 10m: %+v", msg)
		}
	}

	// Background command finishes, then pure silent stall for 8 minutes -> MUST trigger stall alert!
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	hasBgTask = false
	mu.Unlock()
	curTime = curTime.Add(5 * time.Second)
	w.scanOnce()
	curTime = curTime.Add(8 * time.Minute)
	w.scanOnce()

	stallCount := 0
	for _, msg := range sender.sent {
		if msg.Category == "task_stall_alert" {
			stallCount++
		}
	}
	if stallCount != 1 {
		st := w.knownSessions["cas_stall_watch"]
		t.Fatalf("expected exactly 1 stall alert after 8m silent stall, got %d (all=%+v, state=%+v, now=%v)", stallCount, sender.sent, st, curTime)
	}
}
