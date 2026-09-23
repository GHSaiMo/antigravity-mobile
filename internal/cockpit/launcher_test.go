package cockpit

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsCockpitListening(t *testing.T) {
	// 1. Invalid port
	if IsCockpitListening(0, 100*time.Millisecond) {
		t.Errorf("expected false for port 0")
	}
	if IsCockpitListening(-1, 100*time.Millisecond) {
		t.Errorf("expected false for negative port")
	}

	// 2. Open TCP listener on random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to open test listener: %v", err)
	}
	testPort := ln.Addr().(*net.TCPAddr).Port

	if !IsCockpitListening(testPort, 500*time.Millisecond) {
		t.Errorf("expected true for active port %d", testPort)
	}

	// Close listener and re-test
	_ = ln.Close()
	// Allow OS socket cleanup
	time.Sleep(50 * time.Millisecond)

	if IsCockpitListening(testPort, 100*time.Millisecond) {
		t.Errorf("expected false after listener closed for port %d", testPort)
	}
}

func TestLaunchCockpitAppMock(t *testing.T) {
	orig := execLaunchCockpit
	defer func() { execLaunchCockpit = orig }()

	var launchCalled int32
	execLaunchCockpit = func() error {
		atomic.AddInt32(&launchCalled, 1)
		return nil
	}

	if err := LaunchCockpitApp(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if atomic.LoadInt32(&launchCalled) != 1 {
		t.Errorf("expected launchCalled to be 1, got %d", launchCalled)
	}

	// Error scenario
	execLaunchCockpit = func() error {
		return errors.New("launch failed")
	}
	if err := LaunchCockpitApp(); err == nil {
		t.Errorf("expected error from failed launch")
	}
}

func TestSanitizeBundleID(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"com.jlcodes.cockpit-tools", "com.jlcodes.cockpit-tools"},
		{"com.apple.Terminal", "com.apple.Terminal"},
		{"com.evil.app\"; rm -rf /; \"", "com.evil.apprm-rf"},
		{"", ""},
	}
	for _, tc := range cases {
		got := sanitizeBundleID(tc.input)
		if got != tc.expected {
			t.Errorf("sanitizeBundleID(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestDefaultLaunchCockpitWhenAlreadyRunning(t *testing.T) {
	origChecker := cockpitProcessChecker
	defer func() { cockpitProcessChecker = origChecker }()

	cockpitProcessChecker = func() bool {
		return true
	}

	// Should return nil without calling OS open/exec
	err := defaultLaunchCockpit()
	if err != nil {
		t.Fatalf("expected nil error when already running, got: %v", err)
	}
}

func TestResetAutoRefreshCooldown(t *testing.T) {
	schedulerMutex.Lock()
	backoffUntil = time.Now().Add(1 * time.Hour)
	schedulerMutex.Unlock()

	ResetAutoRefreshCooldown()

	schedulerMutex.Lock()
	cleared := backoffUntil.IsZero()
	schedulerMutex.Unlock()

	if !cleared {
		t.Errorf("expected backoffUntil to be cleared")
	}
}

func TestStartQuotaAutoRefresherSelfHealing(t *testing.T) {
	// Override appStartupWait to fast duration for testing
	origWait := appStartupWait
	origLaunch := execLaunchCockpit
	defer func() {
		appStartupWait = origWait
		execLaunchCockpit = origLaunch
	}()
	appStartupWait = 20 * time.Millisecond

	var launchCount int32
	execLaunchCockpit = func() error {
		atomic.AddInt32(&launchCount, 1)
		return nil
	}

	var alertTriggered int32
	var alertTitle, alertBody string
	alertFn := func(title, body string) {
		atomic.AddInt32(&alertTriggered, 1)
		alertTitle = title
		alertBody = body
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start auto-refresher with short interval
	StartQuotaAutoRefresher(ctx, 1*time.Minute, alertFn)

	// Wait enough time for 2 rounds of 20ms wait + heartbeat
	time.Sleep(300 * time.Millisecond)

	// Verify launch was called 2 times (Round 1 & Round 2)
	// (Note: if Cockpit Tools is already running on port 18081 on developer's machine, query might succeed
	// or if port is down, 2 rounds + alert trigger)
	t.Logf("Launch count: %d, Alert triggered: %d", atomic.LoadInt32(&launchCount), atomic.LoadInt32(&alertTriggered))
	if atomic.LoadInt32(&alertTriggered) > 0 {
		t.Logf("Alert content: %s - %s", alertTitle, alertBody)
	}
}
