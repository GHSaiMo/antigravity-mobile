package proxy

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
	"github.com/gorilla/websocket"
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

func TestMarkCascadeMutatedBypassesIdleTrajectoryTTL(t *testing.T) {
	const cid = "cascade-mutated-ttl-test"
	var calls atomic.Int32
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		steps := `{"type":"CORTEX_STEP_TYPE_USER_INPUT","status":"CORTEX_STEP_STATUS_DONE","userInput":{"items":[{"text":"first"}]}}`
		if n >= 2 {
			steps += `,{"type":"CORTEX_STEP_TYPE_USER_INPUT","status":"CORTEX_STEP_STATUS_DONE","userInput":{"items":[{"text":"second"}]}}`
		}
		w.Write([]byte(`{"status":"CASCADE_RUN_STATUS_IDLE","trajectory":{"cascadeId":"` + cid + `","summary":"Has Title","steps":[` + steps + `]}}`))
	}))
	defer upstream.Close()
	defer func() {
		ClearTrajectoryCache(cid)
		clearCascadeMutated(cid)
	}()

	p := NewProxy(inspector.NewInspector(5 * time.Second))
	port := upstream.Listener.Addr().(*net.TCPAddr).Port

	// Prime the cache with an IDLE trajectory that has a title and steps (normally 60s TTL).
	resp1, err := p.fetchUpstreamTrajectoryWithContext(context.Background(), cid, port, "")
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}
	if len(resp1.Trajectory.Steps) != 1 {
		t.Fatalf("expected 1 step initially, got %d", len(resp1.Trajectory.Steps))
	}

	MarkCascadeMutated(cid, 4*time.Second)
	time.Sleep(270 * time.Millisecond)

	resp2, err := p.fetchUpstreamTrajectoryWithContext(context.Background(), cid, port, "")
	if err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}
	if len(resp2.Trajectory.Steps) != 2 {
		t.Fatalf("expected mutated cascade to bypass 60s idle TTL and fetch 2 steps, got %d (calls=%d)", len(resp2.Trajectory.Steps), calls.Load())
	}
}

func TestStreamTouchForcesFreshFetchOnIdleStream(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const cid = "cascade-touch-fresh-test"
	var version atomic.Int32
	version.Store(1)

	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		v := version.Load()
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			// Intentionally return an unchanged summary so only touchCh + forceFreshNext can trigger the refetch.
			w.Write([]byte(`{"trajectorySummaries":{"` + cid + `":{"status":"CASCADE_RUN_STATUS_IDLE","stepCount":2,"summary":"t"}}}`))
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			steps := `{"type":"CORTEX_STEP_TYPE_USER_INPUT","status":"CORTEX_STEP_STATUS_DONE","userInput":{"items":[{"text":"first"}]}},` +
				`{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","status":"CORTEX_STEP_STATUS_DONE","plannerResponse":{"response":"one"}}`
			if v == 2 {
				steps += `,{"type":"CORTEX_STEP_TYPE_USER_INPUT","status":"CORTEX_STEP_STATUS_DONE","userInput":{"items":[{"text":"just sent"}]}}`
			}
			w.Write([]byte(`{"status":"CASCADE_RUN_STATUS_IDLE","trajectory":{"cascadeId":"` + cid + `","summary":"t","steps":[` + steps + `]}}`))
		default:
			w.Write([]byte("{}"))
		}
	}))
	defer upstream.Close()
	defer func() {
		ClearTrajectoryCache(cid)
		clearCascadeMutated(cid)
	}()

	p := NewProxy(inspector.NewInspector(5 * time.Second))
	p.updateUpstream(inspector.InstanceInfo{Port: upstream.Listener.Addr().(*net.TCPAddr).Port, CSRFToken: "t", IsHealthy: true})
	server := httptest.NewServer(http.HandlerFunc(p.ServeHTTP))
	defer server.Close()

	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/gateway/cascade/stream?cascadeId="+cid+"&client=ios&format=messages", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	read := func(deadline time.Duration) (StreamUpdatePayload, error) {
		_ = c.SetReadDeadline(time.Now().Add(deadline))
		_, data, err := c.ReadMessage()
		var pl StreamUpdatePayload
		if err == nil {
			err = json.Unmarshal(data, &pl)
		}
		return pl, err
	}

	initFrame, err := read(3 * time.Second)
	if err != nil || initFrame.Type != "init" || initFrame.TotalMessages != 2 {
		t.Fatalf("init frame: %+v err=%v", initFrame, err)
	}

	// Simulate an idle cache entry being populated right before upstream commits step 3,
	// followed by touch wakeup (without clearing cache again).
	version.Store(2)
	MarkCascadeMutated(cid, 4*time.Second)
	p.notifyStreamTouch(cid)

	upd, err := read(1200 * time.Millisecond)
	if err != nil {
		t.Fatalf("expected touch wakeup to force fresh fetch promptly (<1.2s), got err: %v", err)
	}
	if upd.TotalMessages != 3 {
		t.Fatalf("expected 3 messages after touch wakeup, got %d", upd.TotalMessages)
	}
}
