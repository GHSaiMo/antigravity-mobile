package relay

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func TestPoolProvider_MultiConnPoolAndSingleSlotHealing(t *testing.T) {
	secret := "multiconn-test-secret"
	srv, err := NewServer(ServerConfig{
		SigningSecret:   secret,
		ControlHosts:    []string{"ctl.jp1.jiuge.space"},
		AllowedSuffixes: []string{".jiuge.space"},
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	defer srv.Close()

	relayHTTP := httptest.NewServer(srv)
	defer relayHTTP.Close()

	domain := "r-cafe1234.jp1.jiuge.space"
	tok, _ := SignToken(secret, TokenClaims{
		ShortID:   "cafe1234",
		OwnerHash: "cccccccccccccccccccccccccccccccc",
		Domain:    domain,
	})

	ln := NewStreamListener()
	defer ln.Close()

	localMux := http.NewServeMux()
	localMux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok:" + r.URL.Query().Get("n")))
	})
	localSrv := &http.Server{Handler: localMux}
	go func() { _ = localSrv.Serve(ln) }()
	defer localSrv.Close()

	pool, err := NewPoolProvider(PoolConfig{
		Nodes: []NodeConfig{{
			ID:        "jp1-direct",
			Region:    "ap",
			Priority:  10,
			Endpoint:  "ws" + strings.TrimPrefix(relayHTTP.URL, "http") + TunnelUpgradePath,
			HealthURL: relayHTTP.URL + "/healthz",
			Subdomain: domain,
			URL:       "https://" + domain,
			Token:     tok,
		}},
		Listener:     ln,
		ConnsPerNode: 3,
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

	deadline := time.Now().Add(4 * time.Second)
	for (srv.ActiveTunnels() < 1 || srv.ActiveConns() < 3) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if srv.ActiveTunnels() != 1 || srv.ActiveConns() != 3 {
		t.Fatalf("expected 1 active tunnel with 3 parallel conns, got tunnels=%d conns=%d", srv.ActiveTunnels(), srv.ActiveConns())
	}

	// Simulate dropping 1 physical TCP/yamux slot while sibling slots keep serving traffic cleanly
	srv.mu.RLock()
	grp := srv.tunnels[domain]
	srv.mu.RUnlock()
	grp.mu.RLock()
	victim := grp.conns[0].session
	grp.mu.RUnlock()
	_ = victim.Close()

	// Concurrent requests right after dropping slot 0 must succeed via slots 1 & 2 with zero errors
	var wg sync.WaitGroup
	errCh := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, relayHTTP.URL+"/echo?n=hi", nil)
			req.Host = domain
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				errCh <- err
				return
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK || string(body) != "ok:hi" {
				errCh <- io.ErrUnexpectedEOF
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		t.Fatalf("request failed during single-slot drop: %v", e)
	}

	// Dropped slot should self-heal back to 3 conns automatically
	healDeadline := time.Now().Add(5 * time.Second)
	for srv.ActiveConns() < 3 && time.Now().Before(healDeadline) {
		time.Sleep(30 * time.Millisecond)
	}
	if srv.ActiveConns() != 3 {
		t.Fatalf("expected dropped slot to heal back to 3 conns, got %d", srv.ActiveConns())
	}
}
