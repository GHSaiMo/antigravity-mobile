package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/proxy"
)

func sampleHexToken(prefix string) string {
	return prefix + strings.Repeat("a", 64-len(prefix))
}

func TestPushDeviceStore_PersistenceAndLifecycle(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "push_devices.json")
	store, err := NewPushDeviceStore(storePath)
	if err != nil {
		t.Fatalf("NewPushDeviceStore failed: %v", err)
	}

	alertTok := sampleHexToken("1111")
	startTok := sampleHexToken("2222")
	actTok := sampleHexToken("3333")

	if err := store.UpsertAlertToken("dev-1", "ios", alertTok, "sandbox", nil, nil); err != nil {
		t.Fatalf("UpsertAlertToken failed: %v", err)
	}
	if err := store.UpsertLiveActivityStartToken("dev-1", "ios", startTok, "sandbox", nil); err != nil {
		t.Fatalf("UpsertLiveActivityStartToken failed: %v", err)
	}
	if err := store.UpsertLiveActivityToken("dev-1", "ios", "casc-1", actTok, "sandbox"); err != nil {
		t.Fatalf("UpsertLiveActivityToken failed: %v", err)
	}

	// Verify 0600 file permissions
	fi, err := os.Stat(storePath)
	if err != nil {
		t.Fatalf("stat storePath: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 perms, got %o", fi.Mode().Perm())
	}

	// Reload from disk
	store2, err := NewPushDeviceStore(storePath)
	if err != nil {
		t.Fatalf("reload NewPushDeviceStore failed: %v", err)
	}
	alerts := store2.ListAlertTargets()
	if len(alerts) != 1 || alerts[0].Token != alertTok || alerts[0].Env != "sandbox" {
		t.Fatalf("unexpected reloaded alert targets: %+v", alerts)
	}

	// Cascade with active token returns update target (IsStart=false); new cascade returns push-to-start (IsStart=true)
	t1 := store2.ListLiveActivityTargets("casc-1", true)
	if len(t1) != 1 || t1[0].IsStart || t1[0].Token != actTok {
		t.Fatalf("expected active update token for casc-1, got %+v", t1)
	}
	t2 := store2.ListLiveActivityTargets("casc-2", true)
	if len(t2) != 1 || !t2[0].IsStart || t2[0].Token != startTok {
		t.Fatalf("expected push-to-start token for casc-2, got %+v", t2)
	}

	// Disable notifications via preferences
	off := false
	if err := store2.UpdatePreferences("dev-1", &off, nil); err != nil {
		t.Fatalf("UpdatePreferences failed: %v", err)
	}
	if store2.HasAlertTargets() {
		t.Errorf("expected HasAlertTargets=false after disabling notifications")
	}

	// RemoveDevice purges record
	store2.RemoveDevice("dev-1")
	if store2.ActiveDeviceCount() != 0 {
		t.Errorf("expected 0 devices after RemoveDevice, got %d", store2.ActiveDeviceCount())
	}
}

func TestAPNsRelayClient_AlertAnd410Cleanup(t *testing.T) {
	var (
		mu       sync.Mutex
		received []APNsRelayAlertRequest
		fail410  bool
	)

	relaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/push/alert" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req APNsRelayAlertRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		received = append(received, req)
		should410 := fail410
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if should410 {
			w.WriteHeader(http.StatusGone)
			_ = json.NewEncoder(w).Encode(APNsRelayResponse{
				OK:           false,
				Status:       410,
				Reason:       "Unregistered",
				Unregistered: true,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(APNsRelayResponse{OK: true, Status: 200, APNsID: "apns-123"})
	}))
	defer relaySrv.Close()

	store, _ := NewPushDeviceStore("")
	tok := sampleHexToken("abcd")
	_ = store.UpsertAlertToken("dev-ios", "ios", tok, "production", nil, nil)

	apns := NewAPNsRelayClient(relaySrv.URL, store)
	notif := NewNotifier(config.NotificationConfig{Enabled: false}) // Neither Bark nor FCM configured
	notif.AttachAPNsClient(apns, true)

	if !notif.IsEnabled() {
		t.Fatalf("expected Notifier.IsEnabled() == true when APNs alert device is registered")
	}

	err := notif.NotifyAction("casc-999", "Fix Build", &proxy.PendingInteraction{
		Type:      "run_command",
		StepIndex: 5,
		Target:    "go test ./...",
	})
	if err != nil {
		t.Fatalf("NotifyAction failed: %v", err)
	}

	mu.Lock()
	if len(received) != 1 {
		mu.Unlock()
		t.Fatalf("expected 1 APNs alert request, got %d", len(received))
	}
	got := received[0]
	mu.Unlock()

	if got.DeviceToken != tok || got.Env != "production" {
		t.Errorf("unexpected target: %+v", got)
	}
	if got.InterruptionLevel != "time-sensitive" {
		t.Errorf("expected time-sensitive interruption level, got %q", got.InterruptionLevel)
	}
	if got.CascadeID != "casc-999" || got.CollapseID != "cascade-casc-999" {
		t.Errorf("unexpected cascade/collapse ids: %+v", got)
	}

	// Simulate 410 Unregistered -> token must be automatically cleared
	mu.Lock()
	fail410 = true
	mu.Unlock()

	if err := notif.NotifyCompleted("casc-999", "Fix Build", 6); err != nil {
		t.Fatalf("NotifyCompleted returned unexpected error on 410 cleanup: %v", err)
	}
	if store.HasAlertTargets() {
		t.Errorf("expected stale alert token to be cleared after 410 Unregistered")
	}
}

func TestLiveActivityDispatcher_StartThrottleCriticalAndEnd(t *testing.T) {
	var (
		mu       sync.Mutex
		requests []APNsLiveActivityRequest
	)

	relaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/push/live-activity" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req APNsLiveActivityRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Ensure payload stays well under Apple's 4 KB APNs ceiling
		raw, _ := json.Marshal(req)
		if len(raw) >= 4096 {
			t.Errorf("live activity payload exceeded 4KB: %d bytes", len(raw))
		}
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(APNsRelayResponse{OK: true, Status: 200})
	}))
	defer relaySrv.Close()

	store, _ := NewPushDeviceStore("")
	startTok := sampleHexToken("5555")
	updateTok := sampleHexToken("6666")
	_ = store.UpsertLiveActivityStartToken("dev-1", "ios", startTok, "production", nil)

	apns := NewAPNsRelayClient(relaySrv.URL, store)
	disp := NewLiveActivityDispatcher(apns, true)

	curTime := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	disp.nowFunc = func() time.Time { return curTime }

	// 1. Push-to-Start when no active activity token exists yet
	details := &proxy.TrajectoryDetails{
		CascadeID:  "casc-live-1",
		Title:      strings.Repeat("超长会话标题测试", 20),
		Status:     "CASCADE_RUN_STATUS_RUNNING",
		TotalSteps: 2,
		RunningTasks: []proxy.RunningTaskItem{
			{ID: "t1", ToolSummary: "编译测试", CommandLine: strings.Repeat("go test ", 40)},
		},
	}
	disp.SyncFromDetails(context.Background(), details)

	mu.Lock()
	if len(requests) != 1 || requests[0].Event != "start" || requests[0].AttributesType != "AgentActivityAttributes" || !requests[0].InputPushToken {
		mu.Unlock()
		t.Fatalf("expected push-to-start event, got %+v", requests)
	}
	if requests[0].Alert == nil || requests[0].Alert.Title == "" || requests[0].Alert.Body == "" {
		mu.Unlock()
		t.Fatalf("expected non-empty aps.alert on start event (required by iOS liveactivitiesd), got %+v", requests[0].Alert)
	}
	if requests[0].ContentState.LastUpdated != float64(curTime.Unix()) {
		mu.Unlock()
		t.Fatalf("expected Unix epoch LastUpdated %.0f, got %.0f", float64(curTime.Unix()), requests[0].ContentState.LastUpdated)
	}
	mu.Unlock()

	// 2. App reports the newly minted activity update token
	_ = store.UpsertLiveActivityToken("dev-1", "ios", "casc-live-1", updateTok, "production")

	// 3. Routine step change within 5 seconds -> must be throttled!
	curTime = curTime.Add(5 * time.Second)
	details.TotalSteps = 3
	disp.SyncFromDetails(context.Background(), details)
	mu.Lock()
	if len(requests) != 1 {
		mu.Unlock()
		t.Fatalf("expected routine update within throttle window to be skipped, got %d requests", len(requests))
	}
	mu.Unlock()

	// 4. Critical transition to WAITING_APPROVAL -> must bypass throttle with priority 10 immediately!
	curTime = curTime.Add(2 * time.Second)
	details.PendingInteraction = &proxy.PendingInteraction{
		Type:      "run_command",
		StepIndex: 3,
		Target:    "git status",
	}
	disp.SyncFromDetails(context.Background(), details)
	mu.Lock()
	if len(requests) != 2 || requests[1].Event != "update" || requests[1].Priority != 10 || !requests[1].ContentState.HasPendingAction {
		mu.Unlock()
		t.Fatalf("expected immediate priority=10 approval update, got %+v", requests)
	}
	mu.Unlock()

	// 5. Routine update after throttle interval (20s) -> sent with budget-friendly priority 5!
	curTime = curTime.Add(20 * time.Second)
	details.PendingInteraction = nil
	details.TotalSteps = 4
	disp.SyncFromDetails(context.Background(), details) // status WAITING_APPROVAL -> RUNNING (critical, p=10)
	curTime = curTime.Add(20 * time.Second)
	details.TotalSteps = 5
	disp.SyncFromDetails(context.Background(), details) // routine step 4 -> 5 (p=5)

	mu.Lock()
	if len(requests) != 4 || requests[3].Priority != 5 || requests[3].ContentState.StepCount != 5 {
		mu.Unlock()
		t.Fatalf("expected routine priority=5 update at step 5, got %+v", requests)
	}
	mu.Unlock()

	// 6. EndSession sends event="end" with dismissal_date and removes active cascade token
	curTime = curTime.Add(10 * time.Second)
	disp.EndSession(context.Background(), "casc-live-1", "Fix Build", "COMPLETED", 5)

	mu.Lock()
	if len(requests) != 5 || requests[4].Event != "end" || requests[4].DismissalDate <= curTime.Unix() {
		mu.Unlock()
		t.Fatalf("expected end event with dismissal_date, got %+v", requests)
	}
	mu.Unlock()

	if remaining := store.ListLiveActivityTargets("casc-live-1", false); len(remaining) != 0 {
		t.Errorf("expected active cascade token to be cleared after EndSession, got %+v", remaining)
	}
}
