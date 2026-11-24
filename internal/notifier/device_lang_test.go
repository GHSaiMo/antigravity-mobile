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
	"unicode"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/i18n"
	"antigravity-mobile/internal/proxy"
)

func hasHanRune(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func TestPushDeviceStore_LanguagePersistence(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "push_devices.json")
	store, err := NewPushDeviceStore(storePath)
	if err != nil {
		t.Fatalf("NewPushDeviceStore: %v", err)
	}
	tok := sampleHexToken("aa11")
	if err := store.UpsertAlertToken("dev-ja", "ios", tok, "production", nil, nil); err != nil {
		t.Fatalf("UpsertAlertToken: %v", err)
	}
	if err := store.UpdateLanguage("dev-ja", "ja-JP"); err != nil {
		t.Fatalf("UpdateLanguage: %v", err)
	}

	reloaded, err := NewPushDeviceStore(storePath)
	if err != nil {
		t.Fatalf("reload NewPushDeviceStore: %v", err)
	}
	targets := reloaded.ListAlertTargets()
	if len(targets) != 1 || targets[0].Lang != i18n.LangJa {
		t.Fatalf("expected reloaded Lang=ja, got %+v", targets)
	}
}

func TestAPNsPerDeviceLanguageRouting(t *testing.T) {
	prevLang := i18n.GetLang()
	i18n.SetLang(i18n.LangZh)
	defer i18n.SetLang(prevLang)

	var (
		mu       sync.Mutex
		byToken  = make(map[string]APNsRelayAlertRequest)
	)
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req APNsRelayAlertRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		byToken[req.DeviceToken] = req
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(APNsRelayResponse{OK: true, Status: 200})
	}))
	defer relay.Close()

	store, _ := NewPushDeviceStore("")
	tokJa := sampleHexToken("11aa")
	tokEn := sampleHexToken("22bb")
	tokLegacy := sampleHexToken("33cc")

	_ = store.UpsertAlertToken("dev-ja", "ios", tokJa, "production", nil, nil)
	_ = store.UpdateLanguage("dev-ja", "ja")
	_ = store.UpsertAlertToken("dev-en", "ios", tokEn, "production", nil, nil)
	_ = store.UpdateLanguage("dev-en", "en-US")
	_ = store.UpsertAlertToken("dev-legacy", "ios", tokLegacy, "production", nil, nil)

	apns := NewAPNsRelayClient(relay.URL, store)
	notif := NewNotifier(config.NotificationConfig{Enabled: false})
	notif.AttachAPNsClient(apns, true)

	if err := notif.NotifyAction("cid-lang-1", "Deploy", &proxy.PendingInteraction{
		Type:      "run_command",
		StepIndex: 1,
		Target:    "npm run build",
	}); err != nil {
		t.Fatalf("NotifyAction failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(byToken) != 3 {
		t.Fatalf("expected 3 device alerts, got %d", len(byToken))
	}
	if !strings.Contains(byToken[tokJa].Title, "コマンド実行の確認") || !strings.Contains(byToken[tokJa].Body, "Agent がコマンド実行を要求") {
		t.Errorf("expected Japanese APNs alert for ja device, got title=%q body=%q", byToken[tokJa].Title, byToken[tokJa].Body)
	}
	if hasHanRune(byToken[tokEn].Title) || hasHanRune(byToken[tokEn].Body) || !strings.Contains(byToken[tokEn].Title, "Confirm Terminal Command") {
		t.Errorf("expected English APNs alert with zero Han chars for en device, got title=%q body=%q", byToken[tokEn].Title, byToken[tokEn].Body)
	}
	if !strings.Contains(byToken[tokLegacy].Title, "确认执行终端命令") {
		t.Errorf("expected legacy unset-lang device to fall back to host zh, got title=%q", byToken[tokLegacy].Title)
	}
}

func TestFCMDeviceLanguageRouting(t *testing.T) {
	prevLang := i18n.GetLang()
	i18n.SetLang(i18n.LangZh)
	defer i18n.SetLang(prevLang)

	var (
		mu       sync.Mutex
		received []FCMPayload
	)
	fcmSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p FCMPayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		received = append(received, p)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":1}`))
	}))
	defer fcmSrv.Close()

	notif := NewNotifier(config.NotificationConfig{
		Enabled:     true,
		FCMEnabled:  true,
		FCMEndpoint: fcmSrv.URL,
	})
	notif.UpdateFCMDeviceTokenWithLang("fcm-token-ja", "ja_JP")

	if err := notif.NotifyCompleted("cid-fcm-1", "Build App", 4); err != nil {
		t.Fatalf("NotifyCompleted failed: %v", err)
	}

	// Switch language to English and verify quota localized callback too
	notif.UpdateFCMDeviceLang("en")
	if err := notif.NotifyQuotaAlertLocalized("warn", "k-en", "中文标题", "中文内容", func(lang string) (string, string) {
		return i18n.T3For(lang, "中文标题", "Quota Low", "クォータ低下"), i18n.T3For(lang, "中文内容", "20% left", "残り20%")
	}); err != nil {
		t.Fatalf("NotifyQuotaAlertLocalized failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 FCM requests, got %d", len(received))
	}
	if received[0].Notification == nil || !strings.Contains(received[0].Notification.Title, "タスク完了") || !strings.Contains(received[0].Data["body"], "完了しました") {
		t.Errorf("expected Japanese FCM completion push, got notif=%+v data=%+v", received[0].Notification, received[0].Data)
	}
	if received[1].Notification == nil || received[1].Notification.Title != "Quota Low" || received[1].Data["body"] != "20% left" {
		t.Errorf("expected English FCM quota push, got notif=%+v data=%+v", received[1].Notification, received[1].Data)
	}
}

func TestLiveActivityBuildContentStateAndEndLocalization(t *testing.T) {
	now := time.Date(2026, 11, 25, 1, 0, 0, 0, time.UTC)

	cases := []struct {
		name    string
		details *proxy.TrajectoryDetails
		wantEn  string
		wantJa  string
		wantZh  string
	}{
		{
			name: "pending_approval_no_target",
			details: &proxy.TrajectoryDetails{
				CascadeID:          "c1",
				Title:              "Session",
				Status:             "CASCADE_RUN_STATUS_RUNNING",
				PendingInteraction: &proxy.PendingInteraction{Type: "permission", StepIndex: 1},
			},
			wantEn: "Waiting for approval",
			wantJa: "承認待ちです",
			wantZh: "等待用户审批操作",
		},
		{
			name: "pending_approval_with_target",
			details: &proxy.TrajectoryDetails{
				CascadeID:          "c2",
				Title:              "Session",
				Status:             "CASCADE_RUN_STATUS_RUNNING",
				PendingInteraction: &proxy.PendingInteraction{Type: "run_command", Target: "go test", StepIndex: 2},
			},
			wantEn: "Approval: go test",
			wantJa: "承認待ち: go test",
			wantZh: "待审批: go test",
		},
		{
			name: "can_proceed",
			details: &proxy.TrajectoryDetails{
				CascadeID:  "c3",
				Title:      "Session",
				Status:     "CASCADE_RUN_STATUS_RUNNING",
				CanProceed: true,
				TotalSteps: 3,
			},
			wantEn: "Plan ready — tap Proceed",
			wantJa: "計画を確認して Proceed",
			wantZh: "方案待确认，可点击 Proceed",
		},
		{
			name: "running_step_n",
			details: &proxy.TrajectoryDetails{
				CascadeID:  "c4",
				Title:      "Session",
				Status:     "CASCADE_RUN_STATUS_RUNNING",
				TotalSteps: 7,
			},
			wantEn: "Running step 7...",
			wantJa: "ステップ 7 を実行中...",
			wantZh: "正在执行第 7 步...",
		},
		{
			name: "working_fallback",
			details: &proxy.TrajectoryDetails{
				CascadeID: "c5",
				Title:     "Session",
				Status:    "CASCADE_RUN_STATUS_RUNNING",
			},
			wantEn: "Agent is working...",
			wantJa: "Agent が実行中...",
			wantZh: "Agent 正在执行...",
		},
		{
			name: "unnamed_background_task",
			details: &proxy.TrajectoryDetails{
				CascadeID:    "c6",
				Title:        "Session",
				Status:       "CASCADE_RUN_STATUS_RUNNING",
				RunningTasks: []proxy.RunningTaskItem{{ID: "t1"}},
			},
			wantEn: "Background Task",
			wantJa: "バックグラウンドタスク",
			wantZh: "后台任务",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enState := BuildContentStateForLang(tc.details, now, i18n.LangEn)
			if enState.LatestAction != tc.wantEn || hasHanRune(enState.LatestAction) {
				t.Errorf("EN LatestAction = %q, want %q", enState.LatestAction, tc.wantEn)
			}
			jaState := BuildContentStateForLang(tc.details, now, i18n.LangJa)
			if jaState.LatestAction != tc.wantJa {
				t.Errorf("JA LatestAction = %q, want %q", jaState.LatestAction, tc.wantJa)
			}
			zhState := BuildContentStateForLang(tc.details, now, i18n.LangZh)
			if zhState.LatestAction != tc.wantZh {
				t.Errorf("ZH LatestAction = %q, want %q", zhState.LatestAction, tc.wantZh)
			}
		})
	}

	// Verify LiveActivityDispatcher sends per-device localized start and end payloads
	prevLang := i18n.GetLang()
	i18n.SetLang(i18n.LangZh)
	defer i18n.SetLang(prevLang)

	var (
		mu   sync.Mutex
		reqs []APNsLiveActivityRequest
	)
	relay := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req APNsLiveActivityRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		reqs = append(reqs, req)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(APNsRelayResponse{OK: true, Status: 200})
	}))
	defer relay.Close()

	store, _ := NewPushDeviceStore("")
	tokJa := sampleHexToken("7711")
	tokEn := sampleHexToken("8822")
	_ = store.UpsertLiveActivityToken("dev-ja", "ios", "cid-la", tokJa, "production")
	_ = store.UpdateLanguage("dev-ja", "ja")
	_ = store.UpsertLiveActivityToken("dev-en", "ios", "cid-la", tokEn, "production")
	_ = store.UpdateLanguage("dev-en", "en")

	apns := NewAPNsRelayClient(relay.URL, store)
	disp := NewLiveActivityDispatcher(apns, true)
	disp.nowFunc = func() time.Time { return now }

	disp.SyncFromDetails(context.Background(), &proxy.TrajectoryDetails{
		CascadeID:  "cid-la",
		Title:      "Refactor",
		Status:     "CASCADE_RUN_STATUS_RUNNING",
		TotalSteps: 4,
	})
	disp.EndSession(context.Background(), "cid-la", "Refactor", "COMPLETED", 4)

	mu.Lock()
	defer mu.Unlock()
	var sawJaUpdate, sawEnUpdate, sawJaEnd, sawEnEnd bool
	for _, r := range reqs {
		switch {
		case r.Event == "update" && r.PushToken == tokJa:
			sawJaUpdate = r.ContentState.LatestAction == "ステップ 4 を実行中..."
		case r.Event == "update" && r.PushToken == tokEn:
			sawEnUpdate = r.ContentState.LatestAction == "Running step 4..." && !hasHanRune(r.ContentState.LatestAction)
		case r.Event == "end" && r.PushToken == tokJa:
			sawJaEnd = r.ContentState.LatestAction == "タスク完了"
		case r.Event == "end" && r.PushToken == tokEn:
			sawEnEnd = r.ContentState.LatestAction == "Task completed" && !hasHanRune(r.ContentState.LatestAction)
		}
	}
	if !sawJaUpdate || !sawEnUpdate || !sawJaEnd || !sawEnEnd {
		t.Fatalf("expected localized Live Activity updates & ends, got jaUp=%v enUp=%v jaEnd=%v enEnd=%v reqs=%+v",
			sawJaUpdate, sawEnUpdate, sawJaEnd, sawEnEnd, reqs)
	}
}
