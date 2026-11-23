package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoolProvider_SelectsFastestNodeAndFailsOver(t *testing.T) {
	secret := "pool-test-secret"

	// Slow node A (120ms healthz delay) vs Fast node B (no delay)
	slowSrv, _ := NewServer(ServerConfig{SigningSecret: secret, ControlHosts: []string{"slow.jiuge.space"}, AllowedSuffixes: []string{".jiuge.space"}})
	defer slowSrv.Close()
	slowHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			time.Sleep(120 * time.Millisecond)
		}
		slowSrv.ServeHTTP(w, r)
	}))
	defer slowHTTP.Close()

	fastSrv, _ := NewServer(ServerConfig{SigningSecret: secret, ControlHosts: []string{"fast.jiuge.space"}, AllowedSuffixes: []string{".jiuge.space"}})
	defer fastSrv.Close()
	fastHTTP := httptest.NewServer(fastSrv)
	defer fastHTTP.Close()

	tokSlow, _ := SignToken(secret, TokenClaims{ShortID: "abcd1234", OwnerHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Domain: "r-abcd1234.slow.jiuge.space"})
	tokFast, _ := SignToken(secret, TokenClaims{ShortID: "abcd1234", OwnerHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Domain: "r-abcd1234.fast.jiuge.space"})

	switched := make(chan string, 4)
	ln := NewStreamListener()
	defer ln.Close()

	pool, err := NewPoolProvider(PoolConfig{
		Nodes: []NodeConfig{
			{
				ID:        "slow",
				Region:    "us",
				Priority:  10,
				Endpoint:  "ws" + strings.TrimPrefix(slowHTTP.URL, "http") + TunnelUpgradePath,
				HealthURL: slowHTTP.URL + "/healthz",
				Subdomain: "r-abcd1234.slow.jiuge.space",
				URL:       "https://r-abcd1234.slow.jiuge.space",
				Token:     tokSlow,
			},
			{
				ID:        "fast",
				Region:    "ap",
				Priority:  20,
				Endpoint:  "ws" + strings.TrimPrefix(fastHTTP.URL, "http") + TunnelUpgradePath,
				HealthURL: fastHTTP.URL + "/healthz",
				Subdomain: "r-abcd1234.fast.jiuge.space",
				URL:       "https://r-abcd1234.fast.jiuge.space",
				Token:     tokFast,
			},
		},
		Listener: ln,
		OnEndpointChanged: func(n NodeConfig) {
			switched <- n.ID
		},
	})
	if err != nil {
		t.Fatalf("NewPoolProvider: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := pool.Start(ctx); err != nil {
		t.Fatalf("pool.Start: %v", err)
	}
	defer pool.Stop()

	// Should pick "fast" despite "slow" having lower priority number!
	if got := pool.ActiveNode().ID; got != "fast" {
		t.Fatalf("expected fastest node 'fast', got %q", got)
	}

	deadline := time.Now().Add(3 * time.Second)
	for fastSrv.ActiveTunnels() < 1 && time.Now().Before(deadline) {
		time.Sleep(15 * time.Millisecond)
	}
	if fastSrv.ActiveTunnels() != 1 {
		t.Fatalf("expected fastSrv to have 1 active tunnel, got %d", fastSrv.ActiveTunnels())
	}

	// Shut down fast node -> pool must fail over to "slow" automatically!
	fastSrv.Close()
	fastHTTP.Close()

	failoverDeadline := time.Now().Add(10 * time.Second)
	for slowSrv.ActiveTunnels() < 1 && time.Now().Before(failoverDeadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if slowSrv.ActiveTunnels() != 1 {
		t.Fatalf("expected pool to fail over to slowSrv, active=%d (current=%s)", slowSrv.ActiveTunnels(), pool.ActiveNode().ID)
	}
	if got := pool.ActiveNode().ID; got != "slow" {
		t.Fatalf("expected active node switched to 'slow', got %q", got)
	}
}

func TestPoolProvider_ProbeRetriesTransientFailure(t *testing.T) {
	var flakyCalls atomic.Int32
	fastFlaky := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if flakyCalls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer fastFlaky.Close()

	slowStable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer slowStable.Close()

	ln := NewStreamListener()
	defer ln.Close()

	pool, err := NewPoolProvider(PoolConfig{
		Nodes: []NodeConfig{
			{
				ID:        "jp1-direct",
				Region:    "ap",
				Priority:  10,
				Endpoint:  "ws://127.0.0.1:1" + TunnelUpgradePath,
				HealthURL: fastFlaky.URL + "/healthz",
				Subdomain: "r-abcd1234.jp1.jiuge.space",
				URL:       "https://r-abcd1234.jp1.jiuge.space",
				Token:     "tok",
			},
			{
				ID:        "lax-cf",
				Region:    "us",
				Priority:  100,
				Endpoint:  "ws://127.0.0.1:2" + TunnelUpgradePath,
				HealthURL: slowStable.URL + "/healthz",
				Subdomain: "r-abcd1234.jiuge.space",
				URL:       "https://r-abcd1234.jiuge.space",
				Token:     "tok",
			},
		},
		Listener: ln,
	})
	if err != nil {
		t.Fatalf("NewPoolProvider: %v", err)
	}

	best, rtt := pool.selectFastestNode(context.Background(), pool.nodes)
	if best.ID != "jp1-direct" {
		t.Fatalf("expected jp1-direct to win after retrying transient 1st probe failure, got %q (rtt=%v)", best.ID, rtt)
	}
	if flakyCalls.Load() < 2 {
		t.Fatalf("expected at least 2 probe attempts on jp1-direct, got %d", flakyCalls.Load())
	}
}
