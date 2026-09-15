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
