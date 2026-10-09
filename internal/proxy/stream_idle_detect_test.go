package proxy

import (
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

// An idle stream only refetches the full trajectory every 15s; a run started elsewhere must still show
// up within a couple of idle ticks because the cascade's list entry changes.
func TestCascadeStreamIdleNoticesExternalChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const cid = "cascade-idle-detect"
	var version atomic.Int32
	version.Store(1)

	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		v := version.Load()
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			w.Write([]byte(`{"trajectorySummaries":{"` + cid + `":{"status":"CASCADE_RUN_STATUS_IDLE","stepCount":` + map[int32]string{1: "2", 2: "4"}[v] + `,"summary":"t"}}}`))
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			steps := `{"type":"CORTEX_STEP_TYPE_USER_INPUT","status":"CORTEX_STEP_STATUS_DONE","userInput":{"items":[{"text":"first"}]}},` +
				`{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","status":"CORTEX_STEP_STATUS_DONE","plannerResponse":{"response":"one"}}`
			if v == 2 {
				steps += `,{"type":"CORTEX_STEP_TYPE_USER_INPUT","status":"CORTEX_STEP_STATUS_DONE","userInput":{"items":[{"text":"from desktop"}]}},` +
					`{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","status":"CORTEX_STEP_STATUS_DONE","plannerResponse":{"response":"two"}}`
			}
			w.Write([]byte(`{"status":"CASCADE_RUN_STATUS_IDLE","trajectory":{"cascadeId":"` + cid + `","summary":"t","steps":[` + steps + `]}}`))
		default:
			w.Write([]byte("{}"))
		}
	}))
	defer upstream.Close()
	defer ClearTrajectoryCache(cid)

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
	init, err := read(5 * time.Second)
	if err != nil || init.Type != "init" || init.TotalMessages != 2 {
		t.Fatalf("init frame: %+v err=%v", init, err)
	}

	// The desktop adds a turn while the phone sits idle on this conversation.
	version.Store(2)
	start := time.Now()
	upd, err := read(6 * time.Second)
	if err != nil {
		t.Fatalf("no update within 6s of an external change (idle cache is 15s): %v", err)
	}
	if upd.TotalMessages != 4 {
		t.Fatalf("expected the new turn in the update, got %d messages", upd.TotalMessages)
	}
	t.Logf("external change pushed after %v", time.Since(start).Round(100*time.Millisecond))
}
