package notifier

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/proxy"
)

type mockSender struct {
	mu       sync.Mutex
	payloads []PushPayload
	fail     bool
	count    atomic.Int32
}

func (m *mockSender) Send(_ context.Context, p PushPayload) error {
	m.count.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("mock send failure")
	}
	m.payloads = append(m.payloads, p)
	return nil
}

func (m *mockSender) setFail(v bool) {
	m.mu.Lock()
	m.fail = v
	m.mu.Unlock()
}

func (m *mockSender) snapshot() []PushPayload {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]PushPayload, len(m.payloads))
	copy(out, m.payloads)
	return out
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
	sender := &mockSender{}
	cfg := config.NotificationConfig{
		Enabled:       true,
		IconURL:       "https://example.com/icon.png",
		Group:         "Antigravity",
		SoundAction:   "alarm",
		SoundComplete: "glass",
	}

	n := NewNotifierWithSender(cfg, sender)

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
	if sender.count.Load() != 1 {
		t.Errorf("expected 1 request, got %d", sender.count.Load())
	}

	// Duplicate action should be deduped
	_ = n.NotifyAction("cas_abc", "My Project", pi)
	if sender.count.Load() != 1 {
		t.Errorf("duplicate action should have been deduped, got %d requests", sender.count.Load())
	}

	// 2. CanProceed notification
	err = n.NotifyProceed("cas_abc", "My Plan", 10)
	if err != nil {
		t.Fatalf("NotifyProceed failed: %v", err)
	}
	if sender.count.Load() != 2 {
		t.Errorf("expected 2 requests, got %d", sender.count.Load())
	}

	// 3. Completed notification
	err = n.NotifyCompleted("cas_abc", "My Plan", 15)
	if err != nil {
		t.Fatalf("NotifyCompleted failed: %v", err)
	}
	if sender.count.Load() != 3 {
		t.Errorf("expected 3 requests, got %d", sender.count.Load())
	}
}

func TestNotifierDedupRollbackOnFailure(t *testing.T) {
	sender := &mockSender{fail: true}
	cfg := config.NotificationConfig{Enabled: true}
	n := NewNotifierWithSender(cfg, sender)

	// Cause failure on first attempt
	err := n.NotifyCompleted("cas_fail", "Test Fail", 5)
	if err == nil {
		t.Fatalf("expected error on failure, got nil")
	}

	// Because of dedup rollback, calling it again (after sender recovered) should succeed!
	sender.setFail(false)
	err = n.NotifyCompleted("cas_fail", "Test Fail", 5)
	if err != nil {
		t.Fatalf("expected retry after failure to succeed, got %v", err)
	}
}

func TestNotifierOnTrajectoryUpdate_IgnoresTerminalStates(t *testing.T) {
	sender := &mockSender{}
	cfg := config.NotificationConfig{Enabled: true}
	n := NewNotifierWithSender(cfg, sender)

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

	if sender.count.Load() != 0 {
		t.Errorf("expected 0 requests for terminal/idle snapshots in OnTrajectoryUpdate, got %d", sender.count.Load())
	}
}

func TestNotifierOnTrajectoryUpdate_RunningInteraction(t *testing.T) {
	sender := &mockSender{}
	cfg := config.NotificationConfig{Enabled: true}
	n := NewNotifierWithSender(cfg, sender)

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
		if sender.count.Load() == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if sender.count.Load() != 1 {
		t.Errorf("expected 1 request for running pending interaction, got %d", sender.count.Load())
	}
}

func TestNotifier_NotifyFailed(t *testing.T) {
	sender := &mockSender{}
	cfg := config.NotificationConfig{Enabled: true}
	n := NewNotifierWithSender(cfg, sender)

	err := n.NotifyFailed("cas_fail_1", "Failed task", 5)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	got := sender.snapshot()
	if len(got) != 1 {
		t.Fatalf("expected 1 payload, got %d", len(got))
	}
	if got[0].Title != MsgTitleFailed {
		t.Errorf("expected title %q, got %q", MsgTitleFailed, got[0].Title)
	}
	if got[0].Level != "timeSensitive" {
		t.Errorf("expected level timeSensitive, got %q", got[0].Level)
	}
}

func TestNotifier_NotifyCockpitAlert(t *testing.T) {
	sender := &mockSender{}
	cfg := config.NotificationConfig{Enabled: true}
	n := NewNotifierWithSender(cfg, sender)

	err := n.NotifyCockpitAlert("自定义报警", "Cockpit 进程离线")
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	got := sender.snapshot()
	if len(got) != 1 || got[0].Title != "自定义报警" || got[0].Body != "Cockpit 进程离线" {
		t.Errorf("unexpected alert content: %+v", got)
	}
}

func TestNotifier_NotifyAction_Branches(t *testing.T) {
	sender := &mockSender{}
	cfg := config.NotificationConfig{Enabled: true}
	n := NewNotifierWithSender(cfg, sender)

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

	if count := len(sender.snapshot()); count != 2 {
		t.Fatalf("expected 2 notification payloads, got %d", count)
	}
}
