package power

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

type fakeBackend struct {
	mu           sync.Mutex
	available    bool
	acquireErr   error
	acquireCalls int
	releaseCalls int
	held         bool
}

func (f *fakeBackend) Name() string { return "fake" }

func (f *fakeBackend) Available() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.available
}

func (f *fakeBackend) Acquire() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acquireCalls++
	if f.acquireErr != nil {
		return f.acquireErr
	}
	f.held = true
	return nil
}

func (f *fakeBackend) Release() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.releaseCalls++
	f.held = false
	return nil
}

func (f *fakeBackend) stats() (acq, rel int, held bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.acquireCalls, f.releaseCalls, f.held
}

func TestGuardAcquiresOnActiveAndReleasesAfterGrace(t *testing.T) {
	fb := &fakeBackend{available: true}
	g := NewGuard(fb, 60*time.Millisecond)
	defer g.Close()

	// Idle initially: must NOT acquire.
	g.SetActive(false)
	acq, rel, held := fb.stats()
	if acq != 0 || rel != 0 || held || g.IsHeld() {
		t.Fatalf("expected no acquire while idle, got acq=%d rel=%d held=%v", acq, rel, held)
	}

	// Active: acquires once; repeated SetActive(true) is idempotent.
	g.SetActive(true)
	g.SetActive(true)
	g.SetActive(true)
	acq, rel, held = fb.stats()
	if acq != 1 || rel != 0 || !held || !g.IsHeld() {
		t.Fatalf("expected exactly 1 acquire while active, got acq=%d rel=%d held=%v", acq, rel, held)
	}

	// Inactive: stays held during grace period, releases after grace expires.
	g.SetActive(false)
	time.Sleep(20 * time.Millisecond)
	if !g.IsHeld() {
		t.Fatal("guard must remain held during release grace window")
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for g.IsHeld() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	acq, rel, held = fb.stats()
	if acq != 1 || rel != 1 || held || g.IsHeld() {
		t.Fatalf("expected release after grace expired, got acq=%d rel=%d held=%v", acq, rel, held)
	}
}

func TestGuardCancelsReleaseWhenActivityResumesDuringGrace(t *testing.T) {
	fb := &fakeBackend{available: true}
	g := NewGuard(fb, 70*time.Millisecond)
	defer g.Close()

	g.SetActive(true)
	g.SetActive(false)
	time.Sleep(25 * time.Millisecond)

	// New task begins before grace expires: must cancel release timer cleanly.
	g.SetActive(true)
	time.Sleep(80 * time.Millisecond)

	acq, rel, held := fb.stats()
	if acq != 1 || rel != 0 || !held || !g.IsHeld() {
		t.Fatalf("resumed activity must cancel scheduled release, got acq=%d rel=%d held=%v", acq, rel, held)
	}

	// Now go idle for real -> releases after grace.
	g.SetActive(false)
	deadline := time.Now().Add(500 * time.Millisecond)
	for g.IsHeld() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	acq, rel, held = fb.stats()
	if acq != 1 || rel != 1 || held {
		t.Fatalf("expected release after second idle grace, got acq=%d rel=%d held=%v", acq, rel, held)
	}
}

func TestGuardWatchContextAndCloseReleaseImmediately(t *testing.T) {
	fb := &fakeBackend{available: true}
	g := NewGuard(fb, 10*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	g.WatchContext(ctx)

	g.SetActive(true)
	if !g.IsHeld() {
		t.Fatal("expected guard to be held")
	}

	cancel()
	deadline := time.Now().Add(500 * time.Millisecond)
	for g.IsHeld() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	acq, rel, held := fb.stats()
	if acq != 1 || rel != 1 || held {
		t.Fatalf("context cancel must immediately release guard, got acq=%d rel=%d held=%v", acq, rel, held)
	}

	// Post-close SetActive(true) must be a safe no-op.
	g.SetActive(true)
	acq2, _, _ := fb.stats()
	if acq2 != 1 {
		t.Fatalf("closed guard must not re-acquire, got acq=%d", acq2)
	}
}

func TestGuardUnavailableBackendIsNoop(t *testing.T) {
	fb := &fakeBackend{available: false}
	g := NewGuard(fb, 20*time.Millisecond)
	defer g.Close()

	if g.Available() {
		t.Fatal("expected guard.Available() == false")
	}
	g.SetActive(true)
	g.SetActive(false)
	acq, rel, held := fb.stats()
	if acq != 0 || rel != 0 || held {
		t.Fatalf("unavailable backend must never be invoked, got acq=%d rel=%d held=%v", acq, rel, held)
	}
}

func TestGuardAcquireErrorRetriesOnNextActive(t *testing.T) {
	fb := &fakeBackend{available: true, acquireErr: errors.New("transient")}
	g := NewGuard(fb, 50*time.Millisecond)
	defer g.Close()

	g.SetActive(true)
	if g.IsHeld() {
		t.Fatal("failed Acquire must not mark guard held")
	}

	fb.mu.Lock()
	fb.acquireErr = nil
	fb.mu.Unlock()

	g.SetActive(true)
	if !g.IsHeld() {
		t.Fatal("subsequent SetActive(true) should retry and succeed")
	}
}

func TestPlatformBackendSmokeDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only caffeinate smoke test")
	}
	b := newPlatformBackend()
	if !b.Available() {
		t.Skip("caffeinate not found on host")
	}
	if err := b.Acquire(); err != nil {
		t.Fatalf("darwin caffeinate Acquire failed: %v", err)
	}
	if err := b.Release(); err != nil {
		t.Fatalf("darwin caffeinate Release failed: %v", err)
	}
}
