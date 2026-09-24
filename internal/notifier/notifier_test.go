package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/proxy"
)

func TestBarkClientSend(t *testing.T) {
	var receivedPayload BarkPayload
	var receivedContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&receivedPayload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":200,"message":"success"}`))
	}))
	defer server.Close()

	cfg := config.NotificationConfig{
		Enabled:       true,
		BarkEndpoint:  server.URL,
		IconURL:       "https://example.com/icon.png",
		Group:         "Antigravity",
		SoundAction:   "alarm",
		SoundComplete: "glass",
	}

	client := NewBarkClient(cfg)
	err := client.Send(context.Background(), BarkPayload{
		Title: "测试标题",
		Body:  "测试正文",
		URL:   "antigravity://cascade/cas_123",
		Level: "timeSensitive",
		Sound: "alarm",
	})

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if receivedContentType != "application/json; charset=utf-8" {
		t.Errorf("expected Content-Type application/json; charset=utf-8, got %q", receivedContentType)
	}

	if receivedPayload.Title != "测试标题" || receivedPayload.Body != "测试正文" {
		t.Errorf("unexpected payload content: %+v", receivedPayload)
	}
	if receivedPayload.Icon != "https://example.com/icon.png" {
		t.Errorf("expected default icon to be applied, got: %s", receivedPayload.Icon)
	}
	if receivedPayload.Group != "Antigravity" {
		t.Errorf("expected default group, got: %s", receivedPayload.Group)
	}
}

func TestDedupCache(t *testing.T) {
	cache := NewDedupCache()
	key := "test:event:1"

	// First attempt should succeed
	if !cache.TryNotify(key, 100*time.Millisecond) {
		t.Errorf("first TryNotify should return true")
	}

	// Immediate second attempt should be blocked
	if cache.TryNotify(key, 100*time.Millisecond) {
		t.Errorf("immediate second TryNotify should return false")
	}

	// Test Remove method
	cache.Remove(key)
	if !cache.TryNotify(key, 100*time.Millisecond) {
		t.Errorf("TryNotify after Remove should return true immediately")
	}

	// After TTL expires, it should succeed again
	time.Sleep(120 * time.Millisecond)
	if !cache.TryNotify(key, 100*time.Millisecond) {
		t.Errorf("TryNotify after TTL should return true")
	}
}

func TestNotifierEvents(t *testing.T) {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":200,"message":"success"}`))
	}))
	defer server.Close()

	cfg := config.NotificationConfig{
		Enabled:       true,
		BarkEndpoint:  server.URL,
		IconURL:       "https://example.com/icon.png",
		Group:         "Antigravity",
		SoundAction:   "alarm",
		SoundComplete: "glass",
	}

	n := NewNotifier(cfg)

	// 1. Action notification
	pi := &proxy.PendingInteraction{
		Type:      "run_command",
		Target:    "npm test",
		StepIndex: 3,
	}
	err := n.NotifyAction("cas_abc", "My Project", pi)
	if err != nil {
		t.Fatalf("NotifyAction failed: %v", err)
	}
	if requestCount != 1 {
		t.Errorf("expected 1 request, got %d", requestCount)
	}

	// Duplicate action should be deduped
	_ = n.NotifyAction("cas_abc", "My Project", pi)
	if requestCount != 1 {
		t.Errorf("duplicate action should have been deduped, got %d requests", requestCount)
	}

	// 2. CanProceed notification
	err = n.NotifyProceed("cas_abc", "My Plan", 10)
	if err != nil {
		t.Fatalf("NotifyProceed failed: %v", err)
	}
	if requestCount != 2 {
		t.Errorf("expected 2 requests, got %d", requestCount)
	}

	// 3. Completed notification
	err = n.NotifyCompleted("cas_abc", "My Plan", 15)
	if err != nil {
		t.Fatalf("NotifyCompleted failed: %v", err)
	}
	if requestCount != 3 {
		t.Errorf("expected 3 requests, got %d", requestCount)
	}
}

func TestNotifierDedupRollbackOnFailure(t *testing.T) {
	var shouldFail bool
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if shouldFail {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"code":500,"message":"internal error"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":200,"message":"success"}`))
	}))
	defer server.Close()

	cfg := config.NotificationConfig{
		Enabled:      true,
		BarkEndpoint: server.URL,
	}
	n := NewNotifier(cfg)

	// Cause failure on first attempt
	shouldFail = true
	err := n.NotifyCompleted("cas_fail", "Test Fail", 5)
	if err == nil {
		t.Fatalf("expected error on failure, got nil")
	}

	// Because of dedup rollback, calling it again (after server recovered) should succeed!
	shouldFail = false
	err = n.NotifyCompleted("cas_fail", "Test Fail", 5)
	if err != nil {
		t.Fatalf("expected retry after failure to succeed, got %v", err)
	}
}

func TestNotifierOnTrajectoryUpdate_IgnoresTerminalStates(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":200,"message":"success"}`))
	}))
	defer server.Close()

	cfg := config.NotificationConfig{
		Enabled:      true,
		BarkEndpoint: server.URL,
	}
	n := NewNotifier(cfg)

	// 1. IDLE status should not trigger completion notification
	idleDetails := &proxy.TrajectoryDetails{
		CascadeID:  "cas_idle_test",
		Title:      "IDLE Test",
		Status:     "CASCADE_RUN_STATUS_IDLE",
		TotalSteps: 12,
	}
	n.OnTrajectoryUpdate(idleDetails)

	// 2. COMPLETED status should not trigger completion notification
	completedDetails := &proxy.TrajectoryDetails{
		CascadeID:  "cas_completed_test",
		Title:      "Completed Test",
		Status:     "CASCADE_RUN_STATUS_COMPLETED",
		TotalSteps: 10,
	}
	n.OnTrajectoryUpdate(completedDetails)

	// 3. FAILED status should not trigger failure notification
	failedDetails := &proxy.TrajectoryDetails{
		CascadeID:  "cas_failed_test",
		Title:      "Failed Test",
		Status:     "CASCADE_RUN_STATUS_FAILED",
		TotalSteps: 5,
	}
	n.OnTrajectoryUpdate(failedDetails)

	// 4. IDLE with stale pending interaction should not trigger action notification
	idleWithPIDetails := &proxy.TrajectoryDetails{
		CascadeID:  "cas_idle_pi",
		Title:      "Idle with Stale PI",
		Status:     "CASCADE_RUN_STATUS_IDLE",
		TotalSteps: 8,
		PendingInteraction: &proxy.PendingInteraction{
			Type:   "run_command",
			Target: "ls",
		},
	}
	n.OnTrajectoryUpdate(idleWithPIDetails)

	if requestCount.Load() != 0 {
		t.Errorf("expected 0 requests for terminal/idle snapshots in OnTrajectoryUpdate, got %d", requestCount.Load())
	}
}

func TestNotifierOnTrajectoryUpdate_RunningInteraction(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":200,"message":"success"}`))
	}))
	defer server.Close()

	cfg := config.NotificationConfig{
		Enabled:      true,
		BarkEndpoint: server.URL,
	}
	n := NewNotifier(cfg)

	runningPIDetails := &proxy.TrajectoryDetails{
		CascadeID:  "cas_running_pi",
		Title:      "Running Action",
		Status:     "CASCADE_RUN_STATUS_RUNNING",
		TotalSteps: 4,
		PendingInteraction: &proxy.PendingInteraction{
			Type:      "run_command",
			Target:    "go test ./...",
			StepIndex: 4,
		},
	}
	n.OnTrajectoryUpdate(runningPIDetails)

	// Await async notification dispatch
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if requestCount.Load() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if requestCount.Load() != 1 {
		t.Errorf("expected 1 request for running pending interaction, got %d", requestCount.Load())
	}
}

func TestNotifier_NotifyFailed(t *testing.T) {
	var received BarkPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":200,"message":"success"}`))
	}))
	defer server.Close()

	cfg := config.NotificationConfig{
		Enabled:      true,
		BarkEndpoint: server.URL,
	}
	n := NewNotifier(cfg)

	err := n.NotifyFailed("cas_fail_1", "Failed task", 5)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if received.Title != MsgTitleFailed {
		t.Errorf("expected title %q, got %q", MsgTitleFailed, received.Title)
	}
	if received.Level != "timeSensitive" {
		t.Errorf("expected level timeSensitive, got %q", received.Level)
	}
}

func TestNotifier_NotifyCockpitAlert(t *testing.T) {
	var received BarkPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":200,"message":"success"}`))
	}))
	defer server.Close()

	cfg := config.NotificationConfig{
		Enabled:      true,
		BarkEndpoint: server.URL,
	}
	n := NewNotifier(cfg)

	err := n.NotifyCockpitAlert("自定义报警", "Cockpit 进程离线")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	if received.Title != "自定义报警" || received.Body != "Cockpit 进程离线" {
		t.Errorf("unexpected alert content: %+v", received)
	}
}

func TestNotifier_NotifyAction_Branches(t *testing.T) {
	var mu sync.Mutex
	var payloads []BarkPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p BarkPayload
		json.NewDecoder(r.Body).Decode(&p)
		mu.Lock()
		payloads = append(payloads, p)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":200,"message":"success"}`))
	}))
	defer server.Close()

	cfg := config.NotificationConfig{
		Enabled:      true,
		BarkEndpoint: server.URL,
	}
	n := NewNotifier(cfg)

	// 1. Permission for write/edit file
	piFile := &proxy.PendingInteraction{
		Type:      "permission",
		Action:    "write_to_file",
		Target:    "/path/to/main.go",
		StepIndex: 1,
	}
	if err := n.NotifyAction("cas_act_1", "File Edit", piFile); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2. Question type
	piQuestion := &proxy.PendingInteraction{
		Type:      "question",
		Target:    "Should we proceed with migration?",
		StepIndex: 2,
	}
	if err := n.NotifyAction("cas_act_1", "Question", piQuestion); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	count := len(payloads)
	mu.Unlock()
	if count != 2 {
		t.Fatalf("expected 2 notification payloads, got %d", count)
	}
}

