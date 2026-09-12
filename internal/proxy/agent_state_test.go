package proxy

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEncodeAndReadConnectEnvelope(t *testing.T) {
	data := []byte(`{"test":"connect_protocol_payload"}`)
	env := encodeConnectEnvelope(data)

	if len(env) != 5+len(data) {
		t.Fatalf("expected envelope length %d, got %d", 5+len(data), len(env))
	}
	if env[0] != 0 {
		t.Fatalf("expected flag 0, got %d", env[0])
	}

	flag, body, err := readConnectEnvelope(bytes.NewReader(env))
	if err != nil {
		t.Fatalf("readConnectEnvelope failed: %v", err)
	}
	if flag != 0 {
		t.Errorf("expected flag 0, got %d", flag)
	}
	if string(body) != string(data) {
		t.Errorf("expected body %s, got %s", string(data), string(body))
	}

	// Test malformed header
	_, _, err = readConnectEnvelope(bytes.NewReader([]byte{0, 1, 2}))
	if err == nil {
		t.Errorf("expected error reading short header, got nil")
	}

	// Test truncated body
	shortBuf := make([]byte, 8)
	shortBuf[0] = 0
	shortBuf[4] = 20 // says 20 bytes body, but only 3 bytes follow
	_, _, err = readConnectEnvelope(bytes.NewReader(shortBuf))
	if err == nil {
		t.Errorf("expected error reading truncated body, got nil")
	}
}

func TestIsQueuedDeliveryStrategy(t *testing.T) {
	tests := []struct {
		val      interface{}
		expected bool
	}{
		{nil, true},
		{2, true},
		{0, true},
		{1, false},
		{float64(2), true},
		{float64(0), true},
		{float64(1), false},
		{"WHEN_IDLE", true},
		{"NEXT_INVOCATION", false},
		{"DELIVERY_STRATEGY_NEXT_INVOCATION", false},
		{"DELIVERY_STRATEGY_WHEN_IDLE", true},
	}

	for i, tc := range tests {
		res := isQueuedDeliveryStrategy(tc.val)
		if res != tc.expected {
			t.Errorf("[%d] val=%v: expected %v, got %v", i, tc.val, tc.expected, res)
		}
	}
}

func TestParseAgentMessageTimestamp(t *testing.T) {
	// String format
	s := "2026-09-12T17:00:00Z"
	if got := parseAgentMessageTimestamp(s); got != s {
		t.Errorf("expected %s, got %s", s, got)
	}

	// Protobuf timestamp map
	tsMap := map[string]interface{}{
		"seconds": float64(1700000000),
		"nanos":   float64(0),
	}
	got := parseAgentMessageTimestamp(tsMap)
	if !strings.HasPrefix(got, "2023-11-14T22:13:20") {
		t.Errorf("unexpected parsed protobuf timestamp: %s", got)
	}

	// Nil format fallback
	nilRes := parseAgentMessageTimestamp(nil)
	if nilRes == "" {
		t.Errorf("expected non-empty timestamp for nil")
	}
}

func TestExtractQueuedMessageText(t *testing.T) {
	// Case 1: Plain content
	msg1 := upstreamAgentMessage{
		ID:      "1",
		Content: "Plain direct text",
	}
	if txt := extractQueuedMessageText(msg1); txt != "Plain direct text" {
		t.Errorf("expected 'Plain direct text', got '%s'", txt)
	}

	// Case 2: Payload string content
	msg2 := upstreamAgentMessage{
		ID: "2",
		Payload: &struct {
			Case  string          `json:"case"`
			Value json.RawMessage `json:"value"`
		}{
			Case:  "content",
			Value: json.RawMessage(`"String in payload value"`),
		},
	}
	if txt := extractQueuedMessageText(msg2); txt != "String in payload value" {
		t.Errorf("expected 'String in payload value', got '%s'", txt)
	}

	// Case 3: Protobuf-ES Step schema with userInput items
	msg3 := upstreamAgentMessage{
		ID: "3",
		StepPayload: json.RawMessage(`{
			"step": {
				"case": "userInput",
				"value": {
					"items": [
						{"text": "Hello, "},
						{"text": "Agent!"}
					]
				}
			}
		}`),
	}
	if txt := extractQueuedMessageText(msg3); txt != "Hello, Agent!" {
		t.Errorf("expected 'Hello, Agent!', got '%s'", txt)
	}

	// Case 4: ProtoJSON userResponse
	msg4 := upstreamAgentMessage{
		ID: "4",
		StepPayload: json.RawMessage(`{
			"userInput": {
				"userResponse": "Task to execute next"
			}
		}`),
	}
	if txt := extractQueuedMessageText(msg4); txt != "Task to execute next" {
		t.Errorf("expected 'Task to execute next', got '%s'", txt)
	}

	// Case 5: Generic nested items
	msg5 := upstreamAgentMessage{
		ID: "5",
		StepPayload: json.RawMessage(`{
			"custom": {
				"items": [
					{"text": "Nested generic prompt"}
				]
			}
		}`),
	}
	if txt := extractQueuedMessageText(msg5); txt != "Nested generic prompt" {
		t.Errorf("expected 'Nested generic prompt', got '%s'", txt)
	}
}

func TestParseAgentStateQueuedMessages(t *testing.T) {
	updateJSON := `{
		"update": {
			"conversationId": "casc-123",
			"status": "CASCADE_RUN_STATUS_RUNNING",
			"pendingAgentMessagesUpdate": {
				"pendingAgentMessages": [
					{
						"id": "msg-1",
						"content": "First in queue",
						"deliveryStrategy": 2,
						"timestamp": "2026-09-12T17:10:00Z"
					},
					{
						"id": "msg-2",
						"content": "Skip this non-idle message",
						"deliveryStrategy": 1,
						"timestamp": "2026-09-12T17:10:05Z"
					},
					{
						"id": "msg-3",
						"stepPayload": {
							"step": {
								"case": "userInput",
								"value": {
									"items": [{"text": "Second in queue"}]
								}
							}
						},
						"deliveryStrategy": "DELIVERY_STRATEGY_WHEN_IDLE",
						"timestamp": "2026-09-12T17:10:10Z"
					}
				]
			}
		}
	}`

	var state upstreamAgentStateUpdate
	if err := json.Unmarshal([]byte(updateJSON), &state); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	items := parseAgentStateQueuedMessages(&state)
	if len(items) != 2 {
		t.Fatalf("expected 2 queued messages, got %d", len(items))
	}
	if items[0].ID != "msg-1" || items[0].Text != "First in queue" {
		t.Errorf("item[0] mismatch: %+v", items[0])
	}
	if items[1].ID != "msg-3" || items[1].Text != "Second in queue" {
		t.Errorf("item[1] mismatch: %+v", items[1])
	}
}

func TestGetCachedOrFetchPendingMessages(t *testing.T) {
	ClearPendingMessagesCache("")

	// Create a mock ConnectRPC test server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "StreamAgentStateUpdates") {
			http.NotFound(w, r)
			return
		}

		// Verify Connect protocol headers
		if r.Header.Get("Content-Type") != "application/connect+json" {
			http.Error(w, "invalid content type", http.StatusBadRequest)
			return
		}

		respObj := upstreamAgentStateUpdate{}
		respObj.Update.ConversationID = "test-cascade"
		respObj.Update.PendingAgentMessagesUpdate = &struct {
			Indices              []uint32               `json:"indices"`
			PendingAgentMessages []upstreamAgentMessage `json:"pendingAgentMessages"`
			TotalLength          uint32                 `json:"totalLength"`
		}{
			PendingAgentMessages: []upstreamAgentMessage{
				{
					ID:        "server-msg-1",
					Content:   "Queued on desktop",
					Timestamp: "2026-09-12T17:20:00Z",
				},
			},
		}

		respBytes, _ := json.Marshal(respObj)
		envelope := encodeConnectEnvelope(respBytes)

		w.Header().Set("Content-Type", "application/connect+json")
		w.WriteHeader(http.StatusOK)
		w.Write(envelope)
	}))
	defer server.Close()

	// Parse test server port
	parts := strings.Split(server.Listener.Addr().String(), ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])

	proxy := &Proxy{
		shortClient: &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
			Timeout: 2 * time.Second,
		},
	}

	// 1. Fetch from mock server
	items := proxy.GetCachedOrFetchPendingMessages("test-cascade", port, "token-123")
	if len(items) != 1 {
		t.Fatalf("expected 1 message from upstream, got %d", len(items))
	}
	if items[0].ID != "server-msg-1" || items[0].Text != "Queued on desktop" {
		t.Fatalf("unexpected message: %+v", items[0])
	}

	// 2. Cache hit test (server closed should still return cached result within TTL)
	server.Close()
	cached := proxy.GetCachedOrFetchPendingMessages("test-cascade", port, "token-123")
	if len(cached) != 1 || cached[0].ID != "server-msg-1" {
		t.Fatalf("expected cached message, got %+v", cached)
	}

	// 3. Clear cache
	ClearPendingMessagesCache("test-cascade")
	cleared := proxy.GetCachedOrFetchPendingMessages("test-cascade", port, "token-123")
	// Since server is closed and cache is cleared, should return empty
	if len(cleared) != 0 {
		t.Fatalf("expected empty after clear cache and server down, got %d", len(cleared))
	}
}

func TestStreamUpdatePayloadFingerprintQueuedMessages(t *testing.T) {
	p1 := StreamUpdatePayload{
		Status: "CASCADE_RUN_STATUS_RUNNING",
		QueuedMessages: []QueuedMessageItem{
			{ID: "q1", Text: "Hello"},
		},
	}
	fp1 := p1.Fingerprint()

	// Same content should yield same fingerprint
	p2 := StreamUpdatePayload{
		Status: "CASCADE_RUN_STATUS_RUNNING",
		QueuedMessages: []QueuedMessageItem{
			{ID: "q1", Text: "Hello"},
		},
	}
	if p2.Fingerprint() != fp1 {
		t.Errorf("expected fingerprints to match for identical queues")
	}

	// Adding message alters fingerprint
	p3 := StreamUpdatePayload{
		Status: "CASCADE_RUN_STATUS_RUNNING",
		QueuedMessages: []QueuedMessageItem{
			{ID: "q1", Text: "Hello"},
			{ID: "q2", Text: "World"},
		},
	}
	if p3.Fingerprint() == fp1 {
		t.Errorf("expected fingerprint to differ after adding message")
	}

	// Changing text length alters fingerprint
	p4 := StreamUpdatePayload{
		Status: "CASCADE_RUN_STATUS_RUNNING",
		QueuedMessages: []QueuedMessageItem{
			{ID: "q1", Text: "Hello altered length"},
		},
	}
	if p4.Fingerprint() == fp1 {
		t.Errorf("expected fingerprint to differ after modifying message text")
	}
}
