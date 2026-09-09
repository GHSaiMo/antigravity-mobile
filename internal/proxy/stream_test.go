package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
	"github.com/gorilla/websocket"
)

func TestCascadeStreamWebSocket(t *testing.T) {
	insp := inspector.NewInspector(5 * time.Second)
	info := insp.Scan()
	if info == nil {
		t.Skip("Antigravity instance not available, skipping test")
	}

	p := NewProxy(insp)
	server := httptest.NewServer(http.HandlerFunc(p.ServeHTTP))
	defer server.Close()

	// 1. First get a valid cascade ID by listing trajectories
	rpcReq := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", strings.NewReader("{}"))
	rpcReq.Header.Set("Content-Type", "application/json")
	rpcRec := httptest.NewRecorder()
	p.ServeHTTP(rpcRec, rpcReq)

	if rpcRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from GetAllCascadeTrajectories, got %d", rpcRec.Code)
	}

	var allResp struct {
		TrajectorySummaries map[string]interface{} `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(rpcRec.Body.Bytes(), &allResp); err != nil {
		t.Fatalf("failed to decode summaries: %v", err)
	}

	if len(allResp.TrajectorySummaries) == 0 {
		t.Skip("No active cascade trajectories found to test stream")
	}

	var targetCascadeID string
	for id := range allResp.TrajectorySummaries {
		targetCascadeID = id
		break
	}

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/gateway/cascade/stream?cascadeId=" + targetCascadeID
	t.Logf("Connecting to %s", wsURL)

	c, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WS dial failed: %v (resp: %+v)", err, resp)
	}
	defer c.Close()

	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	msgType, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read initial stream message: %v", err)
	}

	if msgType != websocket.TextMessage {
		t.Errorf("expected text message, got %d", msgType)
	}

	var payload StreamUpdatePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("Failed to decode stream payload: %v (data: %s)", err, string(data))
	}

	if payload.Type != "init" {
		t.Errorf("expected payload.Type init, got %s", payload.Type)
	}
	if payload.CascadeID != targetCascadeID {
		t.Errorf("expected cascadeId %s, got %s", targetCascadeID, payload.CascadeID)
	}
	t.Logf("Stream init received: steps=%d, tools=%d, status=%s", payload.TotalSteps, payload.TotalTools, payload.Status)
}
