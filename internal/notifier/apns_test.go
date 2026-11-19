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
	notif := NewNotifier(config.NotificationConfig{Enabled: false}) // Neither FCM nor other senders configured
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
	if requests[0].Alert.Sound != "" {
		mu.Unlock()
		t.Fatalf("expected empty sound on start event so Live Activity push-to-start is silent, got %q", requests[0].Alert.Sound)
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

func TestAPNsRelayClient_SoundPreferences(t *testing.T) {
	var (
		mu       sync.Mutex
		received []APNsRelayAlertRequest
	)

	relaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req APNsRelayAlertRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		received = append(received, req)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(APNsRelayResponse{OK: true, Status: 200, APNsID: "apns-sound"})
	}))
	defer relaySrv.Close()

	storePath := filepath.Join(t.TempDir(), "push_devices.json")
	store, err := NewPushDeviceStore(storePath)
	if err != nil {
		t.Fatalf("NewPushDeviceStore failed: %v", err)
	}
	tok1 := sampleHexToken("1111")
	tok2 := sampleHexToken("2222")
	_ = store.UpsertAlertToken("dev-default", "ios", tok1, "production", nil, nil)
	_ = store.UpsertAlertToken("dev-custom", "ios", tok2, "production", nil, nil)
	if err := store.UpdateSoundPreferences("dev-custom", "chime", "electronic"); err != nil {
		t.Fatalf("UpdateSoundPreferences failed: %v", err)
	}

	// Reload from disk to verify persistence of sound preferences
	reloaded, err := NewPushDeviceStore(storePath)
	if err != nil {
		t.Fatalf("reload NewPushDeviceStore failed: %v", err)
	}

	apns := NewAPNsRelayClient(relaySrv.URL, reloaded)
	notif := NewNotifier(config.NotificationConfig{
		SoundAction:   "alarm",
		SoundComplete: "glass",
	})
	notif.AttachAPNsClient(apns, true)

	if err := notif.NotifyAction("casc-1", "Task 1", &proxy.PendingInteraction{Type: "run_command", StepIndex: 1, Target: "ls"}); err != nil {
		t.Fatalf("NotifyAction failed: %v", err)
	}
	if err := notif.NotifyCompleted("casc-2", "Task 2", 3); err != nil {
		t.Fatalf("NotifyCompleted failed: %v", err)
	}
	if err := notif.NotifyFailed("casc-3", "Task 3", 4); err != nil {
		t.Fatalf("NotifyFailed failed: %v", err)
	}

	mu.Lock()
	snap1 := append([]APNsRelayAlertRequest(nil), received...)
	mu.Unlock()
	if len(snap1) != 6 {
		t.Fatalf("expected 6 requests (3 notifications x 2 devices), got %d", len(snap1))
	}

	soundByDeviceAndCat := make(map[string]string)
	for _, r := range snap1 {
		soundByDeviceAndCat[r.DeviceToken+"|"+r.Category] = r.Sound
	}

	if got := soundByDeviceAndCat[tok1+"|antigravity_action"]; got != "alarm.caf" {
		t.Errorf("dev-default action sound = %q, want alarm.caf", got)
	}
	if got := soundByDeviceAndCat[tok1+"|antigravity_complete"]; got != "glass.caf" {
		t.Errorf("dev-default complete sound = %q, want glass.caf", got)
	}
	if got := soundByDeviceAndCat[tok1+"|antigravity_error"]; got != "suspense.caf" {
		t.Errorf("dev-default error sound = %q, want suspense.caf", got)
	}
	if got := soundByDeviceAndCat[tok2+"|antigravity_action"]; got != "electronic.caf" {
		t.Errorf("dev-custom action sound = %q, want electronic.caf", got)
	}
	if got := soundByDeviceAndCat[tok2+"|antigravity_complete"]; got != "chime.caf" {
		t.Errorf("dev-custom complete sound = %q, want chime.caf", got)
	}

	// Verify silence & default normalization
	if err := reloaded.UpdateSoundPreferences("dev-custom", "silence", "default"); err != nil {
		t.Fatalf("UpdateSoundPreferences silence/default failed: %v", err)
	}
	mu.Lock()
	received = nil
	mu.Unlock()
	_ = notif.NotifyAction("casc-4", "Task 4", &proxy.PendingInteraction{Type: "run_command", StepIndex: 2, Target: "pwd"})
	_ = notif.NotifyCompleted("casc-5", "Task 5", 5)

	mu.Lock()
	snap2 := append([]APNsRelayAlertRequest(nil), received...)
	mu.Unlock()
	soundByDeviceAndCat = make(map[string]string)
	for _, r := range snap2 {
		soundByDeviceAndCat[r.DeviceToken+"|"+r.Category] = r.Sound
	}
	if got := soundByDeviceAndCat[tok2+"|antigravity_action"]; got != "default" {
		t.Errorf("dev-custom default action sound = %q, want default", got)
	}
	if got := soundByDeviceAndCat[tok2+"|antigravity_complete"]; got != "silence.caf" {
		t.Errorf("dev-custom silence complete sound = %q, want silence.caf", got)
	}
}

func TestLiveActivityDispatcher_NoDuplicateStartAcrossRestartAndConcurrentSessions(t *testing.T) {
	var (
		mu       sync.Mutex
		requests []APNsLiveActivityRequest
	)

	relaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req APNsLiveActivityRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(APNsRelayResponse{OK: true, Status: 200})
	}))
	defer relaySrv.Close()

	storePath := filepath.Join(t.TempDir(), "push_devices.json")
	store, err := NewPushDeviceStore(storePath)
	if err != nil {
		t.Fatalf("NewPushDeviceStore failed: %v", err)
	}
	startTok := sampleHexToken("7777")
	updateTokB := sampleHexToken("8888")
	_ = store.UpsertLiveActivityStartToken("dev-1", "ios", startTok, "production", nil)

	curTime := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	disp1 := NewLiveActivityDispatcher(NewAPNsRelayClient(relaySrv.URL, store), true)
	disp1.nowFunc = func() time.Time { return curTime }

	// 1. Two concurrent working sessions A and B -> each gets exactly 1 Push-to-Start
	detailsA := &proxy.TrajectoryDetails{
		CascadeID:  "casc-A",
		Title:      "Session A",
		Status:     "CASCADE_RUN_STATUS_RUNNING",
		TotalSteps: 2,
	}
	detailsB := &proxy.TrajectoryDetails{
		CascadeID:  "casc-B",
		Title:      "Session B",
		Status:     "CASCADE_RUN_STATUS_RUNNING",
		TotalSteps: 1,
	}
	// Subagent should be ignored
	detailsSub := &proxy.TrajectoryDetails{
		CascadeID:          "casc-sub",
		Title:              "Subagent",
		Status:             "CASCADE_RUN_STATUS_RUNNING",
		TotalSteps:         1,
		ParentConversation: "casc-A",
	}

	disp1.SyncFromDetails(context.Background(), detailsA)
	disp1.SyncFromDetails(context.Background(), detailsB)
	disp1.SyncFromDetails(context.Background(), detailsSub)

	mu.Lock()
	if len(requests) != 2 {
		mu.Unlock()
		t.Fatalf("expected exactly 2 start requests (A and B, not subagent), got %d", len(requests))
	}
	mu.Unlock()

	// 2. Simulate gateway restart 15 minutes later (past old 10m cooldown), with A still lacking update token and B having update token
	storeReloaded, err := NewPushDeviceStore(storePath)
	if err != nil {
		t.Fatalf("reload store failed: %v", err)
	}
	_ = storeReloaded.UpsertLiveActivityToken("dev-1", "ios", "casc-B", updateTokB, "production")

	curTime = curTime.Add(15 * time.Minute)
	disp2 := NewLiveActivityDispatcher(NewAPNsRelayClient(relaySrv.URL, storeReloaded), true)
	disp2.nowFunc = func() time.Time { return curTime }

	detailsA.TotalSteps = 5
	detailsB.TotalSteps = 4
	disp2.SyncFromDetails(context.Background(), detailsA) // MUST NOT send duplicate start!
	disp2.SyncFromDetails(context.Background(), detailsB) // MUST send update on updateTokB

	mu.Lock()
	if len(requests) != 3 {
		mu.Unlock()
		t.Fatalf("expected 3 total requests (no duplicate start for A, 1 update for B), got %d: %+v", len(requests), requests)
	}
	if requests[2].Event != "update" || requests[2].PushToken != updateTokB {
		mu.Unlock()
		t.Fatalf("expected update for casc-B on updateTokB, got %+v", requests[2])
	}
	mu.Unlock()

	// 3. EndSession for A clears A's persisted start marker even without an update token; B remains intact
	curTime = curTime.Add(1 * time.Minute)
	disp2.EndSession(context.Background(), "casc-A", "Session A", "COMPLETED", 5)

	if storeReloaded.HasLiveActivityStateForCascade("casc-A") {
		t.Errorf("expected casc-A live activity state to be cleared after EndSession")
	}
	if !storeReloaded.HasLiveActivityStateForCascade("casc-B") {
		t.Errorf("expected casc-B live activity state to remain active")
	}

	// 4. Subsequent working run of A can Push-to-Start cleanly again
	curTime = curTime.Add(10 * time.Second)
	detailsA.TotalSteps = 6
	disp2.SyncFromDetails(context.Background(), detailsA)

	mu.Lock()
	if len(requests) != 4 || requests[3].Event != "start" || requests[3].Attributes == nil || requests[3].Attributes.CascadeID != "casc-A" {
		mu.Unlock()
		t.Fatalf("expected fresh start for new run of casc-A, got %+v", requests)
	}
	mu.Unlock()
}

func TestLiveActivityDispatcher_CapThreeCardsApprovalPriority(t *testing.T) {
	var (
		mu       sync.Mutex
		requests []APNsLiveActivityRequest
	)
	relaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req APNsLiveActivityRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(APNsRelayResponse{OK: true, Status: 200})
	}))
	defer relaySrv.Close()

	store, err := NewPushDeviceStore(filepath.Join(t.TempDir(), "push_devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	_ = store.UpsertLiveActivityStartToken("dev-1", "ios", sampleHexToken("7777"), "production", nil)

	curTime := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	disp := NewLiveActivityDispatcher(NewAPNsRelayClient(relaySrv.URL, store), true)
	disp.nowFunc = func() time.Time { return curTime }

	mk := func(id string) *proxy.TrajectoryDetails {
		return &proxy.TrajectoryDetails{CascadeID: id, Title: id, Status: "CASCADE_RUN_STATUS_RUNNING", TotalSteps: 3}
	}
	starts := func() map[string]bool {
		mu.Lock()
		defer mu.Unlock()
		m := map[string]bool{}
		for _, r := range requests {
			if r.Event == "start" && r.Attributes != nil {
				m[r.Attributes.CascadeID] = true
			}
		}
		return m
	}

	// 5 working sessions -> only first 3 get a card
	for _, id := range []string{"c1", "c2", "c3", "c4", "c5"} {
		disp.SyncFromDetails(context.Background(), mk(id))
		curTime = curTime.Add(time.Second)
	}
	if got := starts(); len(got) != 3 || !got["c1"] || !got["c2"] || !got["c3"] {
		t.Fatalf("expected starts for c1..c3 only, got %v", got)
	}

	// c4 needs approval -> evicts least-recent non-pending (c1), c1 gets an end
	pend := mk("c4")
	pend.CanProceed = true
	disp.SyncFromDetails(context.Background(), pend)
	if got := starts(); len(got) != 4 || !got["c4"] {
		t.Fatalf("expected c4 started after eviction, got %v", got)
	}
	if store.HasLiveActivityStateForCascade("c1") {
		t.Errorf("evicted c1 state should be cleared")
	}
	// c1 stays denied while full
	curTime = curTime.Add(10 * time.Second)
	disp.SyncFromDetails(context.Background(), mk("c1"))
	if len(starts()) != 4 {
		t.Fatalf("c1 must not be re-started while cap is full")
	}
}
