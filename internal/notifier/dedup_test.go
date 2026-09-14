package notifier

import (
	"fmt"
	"testing"
	"time"
)

func TestDedupCache_BasicAndTTL(t *testing.T) {
	cache := NewDedupCache()

	if !cache.TryNotify("key1", 50*time.Millisecond) {
		t.Fatalf("expected first TryNotify to succeed")
	}

	if cache.TryNotify("key1", 50*time.Millisecond) {
		t.Fatalf("expected immediate second TryNotify to be suppressed")
	}

	if !cache.IsNotified("key1", 50*time.Millisecond) {
		t.Fatalf("expected IsNotified to be true within TTL")
	}

	time.Sleep(60 * time.Millisecond)

	if !cache.TryNotify("key1", 50*time.Millisecond) {
		t.Fatalf("expected TryNotify to succeed after TTL expiration")
	}
}

func TestDedupCache_RecordAndRemove(t *testing.T) {
	cache := NewDedupCache()
	cache.Record("key2")

	if !cache.IsNotified("key2", 1*time.Hour) {
		t.Fatalf("expected key2 to be marked as notified")
	}

	cache.Remove("key2")
	if cache.IsNotified("key2", 1*time.Hour) {
		t.Fatalf("expected key2 to be removed")
	}
}

func TestDedupCache_CapacityEviction(t *testing.T) {
	const capLimit = 10
	cache := NewDedupCacheWithCapacity(capLimit)

	for i := 0; i < capLimit; i++ {
		cache.Record(fmt.Sprintf("item-%d", i))
	}

	if cache.Len() != capLimit {
		t.Fatalf("expected len %d, got %d", capLimit, cache.Len())
	}

	// Adding one more item should trigger eviction
	cache.Record("item-overflow")

	if cache.Len() >= capLimit {
		t.Fatalf("expected cache to be pruned below capLimit, got %d", cache.Len())
	}

	// Latest item should exist
	if !cache.IsNotified("item-overflow", 1*time.Minute) {
		t.Fatalf("expected item-overflow to exist in cache")
	}
}
