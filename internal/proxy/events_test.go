package proxy

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestListSignature(t *testing.T) {
	a := []byte(`{"trajectorySummaries":{"a":{"status":"IDLE","stepCount":1},"b":{"status":"IDLE","stepCount":2}}}`)
	reordered := []byte(`{"trajectorySummaries":{"b":{"status":"IDLE","stepCount":2},"a":{"status":"IDLE","stepCount":1}}}`)
	changed := []byte(`{"trajectorySummaries":{"a":{"status":"RUNNING","stepCount":1},"b":{"status":"IDLE","stepCount":2}}}`)

	sa, ok := listSignature(a)
	if !ok {
		t.Fatal("signature failed")
	}
	if sb, _ := listSignature(reordered); sb != sa {
		t.Fatal("signature must not depend on key order")
	}
	if sc, _ := listSignature(changed); sc == sa {
		t.Fatal("signature must change when a status changes")
	}
	if _, ok := listSignature([]byte(`not json`)); ok {
		t.Fatal("invalid body must not produce a signature")
	}
}

func TestListSignatureIgnoresDeletedCascades(t *testing.T) {
	RecordDeletedCascade("evt-deleted")
	with := []byte(`{"trajectorySummaries":{"keep":{"status":"IDLE"},"evt-deleted":{"status":"IDLE"}}}`)
	without := []byte(`{"trajectorySummaries":{"keep":{"status":"IDLE"}}}`)
	s1, _ := listSignature(with)
	s2, _ := listSignature(without)
	if s1 != s2 {
		t.Fatal("tombstoned cascades must not affect the signature")
	}
}

func TestEventsHubBaselineAndCoalescing(t *testing.T) {
	var h eventsHub
	sub, unsub := h.subscribe(func(ctx context.Context, _ <-chan struct{}) { <-ctx.Done() })
	defer unsub()

	h.observe(1) // baseline only
	select {
	case <-sub.wake:
		t.Fatal("first observation must only set the baseline")
	default:
	}
	h.observe(1)
	select {
	case <-sub.wake:
		t.Fatal("unchanged signature must not notify")
	default:
	}

	// Two changes before the subscriber drains coalesce into one wake-up, with the latest revision.
	h.observe(2)
	h.observe(3)
	<-sub.wake
	select {
	case <-sub.wake:
		t.Fatal("changes must coalesce into a single wake-up")
	default:
	}
	if got := h.currentRev(); got != 2 {
		t.Fatalf("rev = %d, want 2", got)
	}
}

func TestEventsHubDetectorLifecycle(t *testing.T) {
	var h eventsHub
	running := make(chan struct{})
	stopped := make(chan struct{})
	_, unsub := h.subscribe(func(ctx context.Context, _ <-chan struct{}) {
		close(running)
		<-ctx.Done()
		close(stopped)
	})
	<-running
	_, unsub2 := h.subscribe(func(context.Context, <-chan struct{}) { t.Error("detector must start once, for the first subscriber") })
	unsub()
	select {
	case <-stopped:
		t.Fatal("detector stopped while a subscriber remained")
	case <-time.After(50 * time.Millisecond):
	}
	unsub2()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("detector must stop with the last subscriber")
	}
}

// End to end against a fake upstream: hello on connect, one "changed" frame when the list changes,
// and silence while it does not.
func TestHandleEventsEndToEnd(t *testing.T) {
	oldPoll, oldNudge := eventsPollInterval, eventsNudgeSpacing
	eventsPollInterval, eventsNudgeSpacing = 40*time.Millisecond, 0
	defer func() { eventsPollInterval, eventsNudgeSpacing = oldPoll, oldNudge }()

	var mu sync.Mutex
	status := "IDLE"
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Write([]byte(`{"trajectorySummaries":{"evt-e2e":{"status":"` + status + `","stepCount":1}}}`))
	}))
	defer upstream.Close()
	_, portStr, _ := net.SplitHostPort(upstream.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	p := &Proxy{mediumClient: upstream.Client(), wsConns: map[*websocket.Conn]struct{}{}, activePort: port}
	gw := httptest.NewServer(http.HandlerFunc(p.HandleEvents))
	defer gw.Close()

	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(gw.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// gorilla connections are unusable after a read timeout, so frames are pumped into a channel.
	frames := make(chan eventsFrame, 8)
	go func() {
		defer close(frames)
		for {
			var f eventsFrame
			if err := c.ReadJSON(&f); err != nil {
				return
			}
			frames <- f
		}
	}()
	next := func(d time.Duration) (eventsFrame, bool) {
		select {
		case f, ok := <-frames:
			return f, ok
		case <-time.After(d):
			return eventsFrame{}, false
		}
	}

	hello, ok := next(2 * time.Second)
	if !ok || hello.Type != "hello" {
		t.Fatalf("hello: %+v ok=%v", hello, ok)
	}

	// Unchanged upstream (several poll cycles; the snapshot TTL is 1s) -> no frame.
	if f, got := next(1300 * time.Millisecond); got {
		t.Fatalf("unexpected frame while nothing changed: %+v", f)
	}

	mu.Lock()
	status = "RUNNING"
	mu.Unlock()
	f, ok := next(4 * time.Second)
	if !ok || f.Type != "changed" || f.Rev != hello.Rev+1 {
		t.Fatalf("changed: %+v ok=%v (hello rev %d)", f, ok, hello.Rev)
	}
}
