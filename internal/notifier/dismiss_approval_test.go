package notifier

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/inspector"
	"antigravity-mobile/internal/proxy"
)

func TestAPNsRelayClient_SilentBackgroundDismiss(t *testing.T) {
	var (
		mu         sync.Mutex
		bgRequests []APNsRelayBackgroundRequest
		statusCode = http.StatusOK
	)

	relaySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/push/background" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		var req APNsRelayBackgroundRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		bgRequests = append(bgRequests, req)
		code := statusCode
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		switch code {
		case http.StatusOK:
			_ = json.NewEncoder(w).Encode(APNsRelayResponse{OK: true, Status: 200, APNsID: "bg-1"})
		case http.StatusNotFound:
			_ = json.NewEncoder(w).Encode(APNsRelayResponse{OK: false, Status: 404, Error: "not found"})
		case http.StatusGone:
			_ = json.NewEncoder(w).Encode(APNsRelayResponse{OK: false, Status: 410, Reason: "Unregistered", Unregistered: true})
		}
	}))
	defer relaySrv.Close()

	store, err := NewPushDeviceStore(filepath.Join(t.TempDir(), "push_devices.json"))
	if err != nil {
		t.Fatalf("NewPushDeviceStore: %v", err)
	}
	tok := sampleHexToken("beef")
	if err := store.UpsertAlertToken("dev-ios", "ios", tok, "production", nil, nil); err != nil {
		t.Fatalf("UpsertAlertToken: %v", err)
	}

	apns := NewAPNsRelayClient(relaySrv.URL, store)
	notif := NewNotifier(config.NotificationConfig{Enabled: true})
	notif.AttachAPNsClient(apns, false)

	// 1. Normal silent background dismiss push
	if err := notif.NotifyDismissApproval("casc-123", "pi:4:run_command"); err != nil {
		t.Fatalf("NotifyDismissApproval failed: %v", err)
	}
	// Duplicate with same key must be deduplicated
	if err := notif.NotifyDismissApproval("casc-123", "pi:4:run_command"); err != nil {
		t.Fatalf("duplicate NotifyDismissApproval failed: %v", err)
	}

	mu.Lock()
	if len(bgRequests) != 1 {
		t.Fatalf("expected 1 background request (deduped), got %d", len(bgRequests))
	}
	got := bgRequests[0]
	mu.Unlock()

	if got.DeviceToken != tok || got.Action != "dismiss_approval" || got.CascadeID != "casc-123" || got.CollapseID != "dismiss-casc-123" {
		t.Fatalf("unexpected background payload: %+v", got)
	}

	// 2. Older relay returning 404 degrades gracefully without returning error
	mu.Lock()
	statusCode = http.StatusNotFound
	mu.Unlock()
	if err := notif.NotifyDismissApproval("casc-123", "pi:7:permission"); err != nil {
		t.Fatalf("expected 404 graceful degradation, got err: %v", err)
	}

	// 3. 410 Unregistered clears alert token
	mu.Lock()
	statusCode = http.StatusGone
	mu.Unlock()
	if err := notif.NotifyDismissApproval("casc-123", "pi:9:permission"); err != nil {
		t.Fatalf("expected nil on 410 cleanup, got err: %v", err)
	}
	if store.HasAlertTargets() {
		t.Fatalf("expected alert token cleared on 410 Unregistered")
	}
}

func TestFCMClient_SilentBackgroundDismiss(t *testing.T) {
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

	fcm := NewFCMClient(config.NotificationConfig{
		FCMEnabled:     true,
		FCMEndpoint:    fcmSrv.URL,
		FCMServerKey:   "srv-key",
		FCMDeviceToken: "android-tok",
	})

	err := fcm.Send(context.Background(), PushPayload{
		Silent:    true,
		Action:    "dismiss_approval",
		Category:  "dismiss_approval",
		CascadeID: "casc-and-1",
		URL:       "antigravity://cascade/casc-and-1",
	})
	if err != nil {
		t.Fatalf("silent FCM send failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 FCM payload, got %d", len(received))
	}
	msg := received[0]
	if msg.Notification != nil {
		t.Fatalf("silent FCM push must be data-only (Notification == nil), got %+v", msg.Notification)
	}
	if msg.Priority != "high" {
		t.Errorf("expected high priority for silent FCM dismiss, got %q", msg.Priority)
	}
	if msg.Data["action"] != "dismiss_approval" || msg.Data["cascadeId"] != "casc-and-1" || msg.Data["silent"] != "true" {
		t.Errorf("unexpected silent FCM data map: %+v", msg.Data)
	}
}

type recordingSender struct {
	mu       sync.Mutex
	payloads []PushPayload
}

func (r *recordingSender) Send(_ context.Context, p PushPayload) error {
	r.mu.Lock()
	r.payloads = append(r.payloads, p)
	r.mu.Unlock()
	return nil
}

func (r *recordingSender) snapshot() []PushPayload {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]PushPayload, len(r.payloads))
	copy(cp, r.payloads)
	return cp
}

func TestWatcherDismissApprovalOnDesktopResolution(t *testing.T) {
	var mu sync.Mutex
	curStatus := "CASCADE_RUN_STATUS_IDLE"
	curSteps := 5
	hasPending := false
	hasCanProceed := false
	resolvedProceed := false

	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		st := curStatus
		steps := curSteps
		pending := hasPending
		proceed := hasCanProceed
		procResolved := resolvedProceed
		mu.Unlock()

		if strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories") {
			_, _ = w.Write([]byte(`{
				"trajectorySummaries": {
					"cas_dismiss_1": {
						"summary": "Desktop approval dismiss test",
						"status": "` + st + `",
						"stepCount": ` + jsonNumber(steps) + `,
						"lastModifiedTime": "2026-11-23T01:00:00Z"
					}
				}
			}`))
			return
		}

		if strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") {
			stepList := []string{
				`{"type": "CORTEX_STEP_TYPE_USER_INPUT", "status": "CORTEX_STEP_STATUS_DONE"}`,
			}
			if pending {
				stepList = append(stepList, `{"type": "CORTEX_STEP_TYPE_RUN_COMMAND", "status": "CORTEX_STEP_STATUS_WAITING", "requestedInteraction": {"runCommand": {"commandLine": "rm -rf build"}}}`)
			} else if proceed {
				stepList = append(stepList, `{"type": "CORTEX_STEP_TYPE_CODE_ACTION", "status": "CORTEX_STEP_STATUS_DONE", "codeAction": {"actionResult": {"edit": {"absoluteUri": "file:///tmp/implementation_plan.md", "createFile": true}}, "artifactMetadata": {"requestFeedback": true}}}`)
				if procResolved {
					stepList = append(stepList, `{"type": "CORTEX_STEP_TYPE_USER_INPUT", "status": "CORTEX_STEP_STATUS_DONE"}`)
				}
			} else if st == "CASCADE_RUN_STATUS_RUNNING" {
				stepList = append(stepList, `{"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE", "status": "CORTEX_STEP_STATUS_RUNNING", "plannerResponse": {"response": "Working..."}}`)
			} else {
				stepList = append(stepList, `{"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE", "status": "CORTEX_STEP_STATUS_DONE", "plannerResponse": {"response": "Finished"}}`)
			}
			_, _ = w.Write([]byte(`{
				"status": "` + st + `",
				"trajectory": {
					"trajectoryId": "cas_dismiss_1",
					"cascadeId": "cas_dismiss_1",
					"steps": [` + strings.Join(stepList, ",") + `]
				}
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	p := proxy.NewProxy(inspector.NewInspector(10 * time.Second))
	p.SetTestUpstream(port, "test-token")

	rec := &recordingSender{}
	notif := NewNotifierWithSender(config.NotificationConfig{Enabled: true}, rec)
	watcher := NewWatcher(p, notif)

	// 1. Baseline sync
	watcher.scanOnce()

	// 2. Session enters RUNNING with a pending approval
	mu.Lock()
	curStatus = "CASCADE_RUN_STATUS_RUNNING"
	curSteps = 6
	hasPending = true
	mu.Unlock()
	watcher.scanOnce()

	got := rec.snapshot()
	if len(got) != 1 || got[0].Category != "antigravity_action" || got[0].Silent {
		t.Fatalf("expected 1 approval alert, got %+v", got)
	}

	// Expire trajectory cache (250ms TTL)
	time.Sleep(300 * time.Millisecond)

	// 3. User approves on desktop -> still RUNNING, no longer pending
	mu.Lock()
	curSteps = 7
	hasPending = false
	mu.Unlock()
	watcher.scanOnce()

	got = rec.snapshot()
	if len(got) != 2 {
		t.Fatalf("expected approval alert + silent dismiss, got %d payloads: %+v", len(got), got)
	}
	if !got[1].Silent || got[1].Action != "dismiss_approval" || got[1].CascadeID != "cas_dismiss_1" {
		t.Fatalf("expected silent dismiss_approval payload, got %+v", got[1])
	}

	// 4. Subsequent tick while still running must NOT re-fire dismiss
	time.Sleep(300 * time.Millisecond)
	watcher.scanOnce()
	if len(rec.snapshot()) != 2 {
		t.Fatalf("expected no duplicate dismiss on subsequent tick, got %d", len(rec.snapshot()))
	}

	// 5. Implementation plan ready (CanProceed=true) on RUNNING -> IDLE transition,
	//    then resolved externally on desktop while staying IDLE.
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	curStatus = "CASCADE_RUN_STATUS_IDLE"
	curSteps = 8
	hasPending = false
	hasCanProceed = true
	resolvedProceed = false
	mu.Unlock()
	watcher.scanOnce()

	got = rec.snapshot()
	if len(got) != 3 || got[2].Category != "antigravity_proceed" || got[2].Silent {
		t.Fatalf("expected proceed alert on RUNNING->IDLE, got %d payloads: %+v", len(got), got)
	}

	// Resolved externally while staying IDLE -> must fire silent dismiss
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	curStatus = "CASCADE_RUN_STATUS_IDLE"
	curSteps = 9
	resolvedProceed = true
	mu.Unlock()
	watcher.scanOnce()

	got = rec.snapshot()
	if len(got) != 4 || !got[3].Silent || got[3].Action != "dismiss_approval" {
		t.Fatalf("expected silent dismiss when IDLE proceed resolved externally, got %d payloads: %+v", len(got), got)
	}
}
