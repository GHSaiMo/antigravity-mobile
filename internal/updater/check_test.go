package updater

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckLatestWithCache_TTLForceAndOfflineFallback(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "update_check.json")
	t0 := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)

	calls := 0
	fetchOK := func(tag string) func(context.Context) (string, error) {
		return func(context.Context) (string, error) {
			calls++
			return tag, nil
		}
	}

	// 1. Cold cache fetches live
	tag, _, err := checkLatestWithCache(context.Background(), "1.2.7", cachePath, t0, 15*time.Minute, false, fetchOK("v1.2.8"))
	if err != nil || tag != "v1.2.8" || calls != 1 {
		t.Fatalf("cold fetch failed: tag=%q err=%v calls=%d", tag, err, calls)
	}

	// 2. Within TTL hits cache without calling fetch
	tag, _, err = checkLatestWithCache(context.Background(), "1.2.7", cachePath, t0.Add(5*time.Minute), 15*time.Minute, false, fetchOK("v1.2.9"))
	if err != nil || tag != "v1.2.8" || calls != 1 {
		t.Fatalf("cache hit expected: tag=%q err=%v calls=%d", tag, err, calls)
	}

	// 3. Force=true bypasses cache and updates it
	tag, _, err = checkLatestWithCache(context.Background(), "1.2.7", cachePath, t0.Add(6*time.Minute), 15*time.Minute, true, fetchOK("v1.2.9"))
	if err != nil || tag != "v1.2.9" || calls != 2 {
		t.Fatalf("force refresh expected: tag=%q err=%v calls=%d", tag, err, calls)
	}

	// 4. Offline error still returns previously cached tag alongside error
	tag, _, err = checkLatestWithCache(context.Background(), "1.2.7", cachePath, t0.Add(30*time.Minute), 15*time.Minute, false, func(context.Context) (string, error) {
		calls++
		return "", errors.New("dial tcp timeout")
	})
	if err == nil || tag != "v1.2.9" || calls != 3 {
		t.Fatalf("offline fallback expected cached v1.2.9 + error: tag=%q err=%v calls=%d", tag, err, calls)
	}
}

func TestInspectSelfUpdateCapability(t *testing.T) {
	writableDir := t.TempDir()
	binPath := filepath.Join(writableDir, "mgy")

	cap := InspectSelfUpdateCapability(binPath, "1.2.7")
	if !cap.CanUpdate {
		t.Fatalf("expected writable temp dir binary to be updatable, got %+v", cap)
	}

	devCap := InspectSelfUpdateCapability(binPath, "dev")
	if devCap.CanUpdate || devCap.Reason == "" {
		t.Fatalf("expected dev build to disable self-update, got %+v", devCap)
	}

	goBuildCap := InspectSelfUpdateCapability(filepath.Join(writableDir, "go-build12345", "b001", "exe", "gateway"), "1.2.7")
	if goBuildCap.CanUpdate || goBuildCap.Reason == "" {
		t.Fatalf("expected go-build path to disable self-update, got %+v", goBuildCap)
	}
}
