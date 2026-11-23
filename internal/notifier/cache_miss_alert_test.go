package notifier

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/i18n"
	"antigravity-mobile/internal/inspector"
	"antigravity-mobile/internal/proxy"
)

func TestNotifyCacheMissAlertThresholdsDedupAndI18n(t *testing.T) {
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

	// Trilingual check + dedup
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

	// Env disable guard
	t.Setenv("CACHE_MISS_ALERT_ENABLED", "false")
	nOff, sOff := newQuotaTestNotifier()
	if err := nOff.NotifyCacheMissAlert("cas-off", "Demo", 4, 200000); err != nil {
		t.Fatal(err)
	}
	if len(sOff.sent) != 0 {
		t.Fatalf("expected 0 alerts when CACHE_MISS_ALERT_ENABLED=false, got %d", len(sOff.sent))
	}
}

func TestWatcherTriggersCacheMissAlertAndThrottlesChecks(t *testing.T) {
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
					"cas_cache_burn": {
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
					"trajectoryId": "cas_cache_burn",
					"cascadeId": "cas_cache_burn",
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

	// First active scan checks cache guard and dispatches alert
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
		t.Fatalf("expected 1 cache miss alert, got %d", gotCount)
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
