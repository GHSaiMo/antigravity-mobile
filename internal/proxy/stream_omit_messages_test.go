package proxy

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
	"github.com/gorilla/websocket"
)

// readStreamInit dials the cascade stream with the given extra query and returns the raw init frame.
func readStreamInit(t *testing.T, serverURL, cascadeID, extra string) map[string]json.RawMessage {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/gateway/cascade/stream?cascadeId=" + cascadeID + extra
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WS dial failed: %v", err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read init frame: %v", err)
	}
	var frame map[string]json.RawMessage
	if err := json.Unmarshal(data, &frame); err != nil {
		t.Fatalf("decode init frame: %v", err)
	}
	return frame
}

func TestCascadeStreamMessagesZeroOmitsMessagesWindow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const cid = "cascade-omit-messages"
	trajectory := `{"status":"CASCADE_RUN_STATUS_IDLE","trajectory":{"cascadeId":"` + cid + `","steps":[` +
		`{"type":"CORTEX_STEP_TYPE_USER_INPUT","status":"CORTEX_STEP_STATUS_DONE","userInput":{"items":[{"text":"hello"}]}},` +
		`{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","status":"CORTEX_STEP_STATUS_DONE","plannerResponse":{"response":"hi there"}}]}}`
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") {
			w.Write([]byte(trajectory))
			return
		}
		w.Write([]byte("{}"))
	}))
	defer upstream.Close()
	defer ClearTrajectoryCache(cid)

	p := NewProxy(inspector.NewInspector(5 * time.Second))
	p.updateUpstream(inspector.InstanceInfo{
		Port:      upstream.Listener.Addr().(*net.TCPAddr).Port,
		CSRFToken: "test-token",
		IsHealthy: true,
	})
	server := httptest.NewServer(http.HandlerFunc(p.ServeHTTP))
	defer server.Close()

	full := readStreamInit(t, server.URL, cid, "&delta=1")
	if _, ok := full["messages"]; !ok {
		t.Fatal("default web stream should still carry the messages window")
	}
	if _, ok := full["steps"]; !ok {
		t.Fatal("web stream should carry steps")
	}

	lite := readStreamInit(t, server.URL, cid, "&delta=1&messages=0")
	if _, ok := lite["messages"]; ok {
		t.Fatal("messages=0 should drop the messages window")
	}
	var steps []json.RawMessage
	if err := json.Unmarshal(lite["steps"], &steps); err != nil || len(steps) != 2 {
		t.Fatalf("messages=0 should keep all steps, got %d (err %v)", len(steps), err)
	}

	// Messages-only clients ignore the flag: they have nothing else to render.
	native := readStreamInit(t, server.URL, cid, "&client=ios&format=messages&delta=1&messages=0")
	if _, ok := native["messages"]; !ok {
		t.Fatal("messages-only clients must keep their messages")
	}
}
