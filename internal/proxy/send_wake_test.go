package proxy

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
)

func TestSendUserCascadeMessageWakesStream(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer upstream.Close()

	p := NewProxy(inspector.NewInspector(5 * time.Second))
	p.updateUpstream(inspector.InstanceInfo{
		Port:      upstream.Listener.Addr().(*net.TCPAddr).Port,
		CSRFToken: "test-token",
		IsHealthy: true,
	})

	touch, cleanup := p.registerStreamTouchListener("cascade-wake-test")
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage",
		strings.NewReader(`{"cascadeId":"cascade-wake-test","items":[{"text":"wake the stream"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	select {
	case <-touch:
	default:
		t.Fatal("expected a successful send to wake the cascade stream")
	}
}

func TestSendUserCascadeMessageUpstreamErrorDoesNotWakeStream(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	p := NewProxy(inspector.NewInspector(5 * time.Second))
	p.updateUpstream(inspector.InstanceInfo{
		Port:      upstream.Listener.Addr().(*net.TCPAddr).Port,
		CSRFToken: "test-token",
		IsHealthy: true,
	})

	touch, cleanup := p.registerStreamTouchListener("cascade-wake-err")
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage",
		strings.NewReader(`{"cascadeId":"cascade-wake-err","items":[{"text":"rejected"}]}`))
	req.Header.Set("Content-Type", "application/json")
	p.ServeHTTP(httptest.NewRecorder(), req)

	select {
	case <-touch:
		t.Fatal("a failed send must not wake the stream")
	default:
	}
}

// A trajectory fetch that started before ClearTrajectoryCache must not write its (possibly stale)
// result back into the cache, and later callers must not join it.
func TestClearTrajectoryCacheDiscardsInFlightFetch(t *testing.T) {
	const cid = "cascade-inflight-test"
	release := make(chan struct{})
	calls := 0
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			<-release
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"CASCADE_RUN_STATUS_IDLE","trajectory":{"steps":[]}}`))
	}))
	defer upstream.Close()
	defer ClearTrajectoryCache(cid)

	p := NewProxy(inspector.NewInspector(5 * time.Second))
	port := upstream.Listener.Addr().(*net.TCPAddr).Port

	staleDone := make(chan struct{})
	go func() {
		defer close(staleDone)
		_, _ = p.fetchUpstreamTrajectoryWithMaxAge(cid, port, "", 0)
	}()

	// Wait until the first fetch is in flight, then invalidate.
	deadline := time.Now().Add(2 * time.Second)
	for {
		trajFlights.mu.Lock()
		_, inFlight := trajFlights.calls[cid]
		trajFlights.mu.Unlock()
		if inFlight {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first fetch never started")
		}
		time.Sleep(5 * time.Millisecond)
	}
	ClearTrajectoryCache(cid)

	trajFlights.mu.Lock()
	_, stillJoinable := trajFlights.calls[cid]
	trajFlights.mu.Unlock()
	if stillJoinable {
		t.Fatal("in-flight fetch should be forgotten after ClearTrajectoryCache")
	}

	close(release)
	<-staleDone

	defaultTrajCache.trajCacheMu.RLock()
	_, cached := defaultTrajCache.trajCache[cid]
	defaultTrajCache.trajCacheMu.RUnlock()
	if cached {
		t.Fatal("a fetch started before invalidation must not populate the cache")
	}

	// A fresh fetch after the invalidation is cached normally.
	if _, err := p.fetchUpstreamTrajectoryWithMaxAge(cid, port, "", 0); err != nil {
		t.Fatalf("fresh fetch failed: %v", err)
	}
	defaultTrajCache.trajCacheMu.RLock()
	_, cached = defaultTrajCache.trajCache[cid]
	defaultTrajCache.trajCacheMu.RUnlock()
	if !cached {
		t.Fatal("expected fresh fetch to be cached")
	}
}
