package auth

import (
	"testing"
	"time"
)

func TestRateLimiterAllow(t *testing.T) {
	l := NewRateLimiter()
	key := "pair:127.0.0.1"
	for i := 0; i < 3; i++ {
		if !l.Allow(key, 3, time.Minute) {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if l.Allow(key, 3, time.Minute) {
		t.Fatal("4th request in window should be denied")
	}
}

func TestRateLimiterWindowExpiry(t *testing.T) {
	l := NewRateLimiter()
	key := "session:10.0.0.1"
	if !l.Allow(key, 1, 20*time.Millisecond) {
		t.Fatal("first should be allowed")
	}
	time.Sleep(30 * time.Millisecond)
	if !l.Allow(key, 1, 20*time.Millisecond) {
		t.Fatal("after window should be allowed again")
	}
}

func TestRateLimiterMaxEntriesCapacity(t *testing.T) {
	l := NewRateLimiter()
	l.maxEntries = 5 // low limit for testing

	// Add 5 distinct keys
	for i := 0; i < 5; i++ {
		key := "ip:" + string(rune('a'+i))
		if !l.Allow(key, 10, time.Minute) {
			t.Fatalf("key %s should be allowed under capacity", key)
		}
	}

	// 6th key exceeds capacity when no keys expired -> should be denied
	if l.Allow("ip:z_exceed", 10, time.Minute) {
		t.Fatal("new key exceeding maxEntries should be denied")
	}

	// Existing keys within capacity should still be allowed if quota permits
	if !l.Allow("ip:a", 10, time.Minute) {
		t.Fatal("existing key should still proceed")
	}
}

