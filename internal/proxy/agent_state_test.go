package proxy

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
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

	// Case 6: Base64-encoded protobuf binary (ProtoJSON bytes field)
	// Construct minimal Step message with field 19 (UserInput) -> field 2 (user_response)
	targetText := "Direct protobuf binary prompt"
	userInputBytes := append([]byte{18, byte(len(targetText))}, []byte(targetText)...)
	// Varint for field 19, wire type 2: (19 << 3) | 2 = 154 = 0x9A, 0x01
	stepBytes := append([]byte{0x9A, 0x01, byte(len(userInputBytes))}, userInputBytes...)
	b64Data := base64.StdEncoding.EncodeToString(stepBytes)
	rawJSON, _ := json.Marshal(b64Data)

	msg6 := upstreamAgentMessage{
		ID:          "6",
		StepPayload: json.RawMessage(rawJSON),
	}
	if txt := extractQueuedMessageText(msg6); txt != targetText {
		t.Errorf("expected '%s', got '%s'", targetText, txt)
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

func TestParseAgentStateQueuedMessages_FiltersTaskCancellation(t *testing.T) {
	// Replicating actual message structure observed in conversation 466ce6f7-bd84-4a3c-b1f6-642fadeb550e
	updateJSON := `{
		"update": {
			"conversationId": "466ce6f7-bd84-4a3c-b1f6-642fadeb550e",
			"status": "CASCADE_RUN_STATUS_IDLE",
			"pendingAgentMessagesUpdate": {
				"indices": [0],
				"pendingAgentMessages": [
					{
						"id": "8f758039-a64b-46cb-ac6f-a34614acb5f8",
						"recipient": "466ce6f7-bd84-4a3c-b1f6-642fadeb550e",
						"sender": "466ce6f7-bd84-4a3c-b1f6-642fadeb550e/task-475",
						"priority": "MESSAGE_PRIORITY_LOW",
						"timestamp": "2026-09-12T10:59:59.998485Z",
						"renderDetails": {
							"messageTitle": "Start JiugeSpace server.py was canceled"
						},
						"hideFromUser": true,
						"content": "Task id \"466ce6f7-bd84-4a3c-b1f6-642fadeb550e/task-475\" was canceled with result:\nTool execution was canceled",
						"sourceMetadata": {
							"tool": {
								"conversationId": "466ce6f7-bd84-4a3c-b1f6-642fadeb550e",
								"stepIndex": 475
							}
						}
					},
					{
						"id": "user-valid-msg",
						"content": "Valid user follow-up",
						"deliveryStrategy": 2,
						"timestamp": "2026-09-12T11:00:00Z"
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
	if len(items) != 1 {
		t.Fatalf("expected exactly 1 valid user queued message, got %d: %+v", len(items), items)
	}
	if items[0].ID != "user-valid-msg" || items[0].Text != "Valid user follow-up" {
		t.Errorf("unexpected queued message: %+v", items[0])
	}
}

func TestIsInternalAgentMessage(t *testing.T) {
	tests := []struct {
		name           string
		hideFromUser   bool
		sender         string
		sourceMetadata string
		content        string
		expected       bool
	}{
		{
			name:         "hideFromUser true",
			hideFromUser: true,
			expected:     true,
		},
		{
			name:     "sender is task",
			sender:   "conv-123/task-475",
			expected: true,
		},
		{
			name:     "sender is subagent",
			sender:   "subagent-research",
			expected: true,
		},
		{
			name:     "sender is system",
			sender:   "system",
			expected: true,
		},
		{
			name:           "sourceMetadata with tool",
			sourceMetadata: `{"tool":{"conversationId":"123"}}`,
			expected:       true,
		},
		{
			name:     "content starts with Task id",
			content:  `Task id "123/task-1" was canceled with result:\nCanceled`,
			expected: true,
		},
		{
			name:     "genuine user message",
			sender:   "",
			content:  "Please help me refactor the function",
			expected: false,
		},
		{
			name:     "user message with explicit sender user",
			sender:   "user",
			content:  "Check git status",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isInternalAgentMessage(tc.hideFromUser, tc.sender, json.RawMessage(tc.sourceMetadata), tc.content)
			if got != tc.expected {
				t.Errorf("[%s] expected %v, got %v", tc.name, tc.expected, got)
			}
		})
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

func TestDeletedMessageTombstonesAndCacheEviction(t *testing.T) {
	cascadeID := "test-cascade-tombstone"
	msgID := "msg-to-delete-123"

	// 1. Initial state: not deleted
	if IsMessageDeleted(cascadeID, msgID) {
		t.Fatalf("expected message not deleted initially")
	}

	// 2. Record deleted
	RecordDeletedMessage(cascadeID, msgID)
	if !IsMessageDeleted(cascadeID, msgID) {
		t.Fatalf("expected message to be marked as deleted")
	}

	// 3. Test filterTombstonedMessages
	items := []QueuedMessageItem{
		{ID: msgID, Text: "Will be deleted"},
		{ID: "msg-keep-456", Text: "Keep me"},
	}
	filtered := filterTombstonedMessages(cascadeID, items)
	if len(filtered) != 1 || filtered[0].ID != "msg-keep-456" {
		t.Fatalf("expected only msg-keep-456, got: %+v", filtered)
	}

	// 4. Test RemovePendingMessageFromCache
	pendingCacheMu.Lock()
	pendingCache[cascadeID] = &pendingMessagesCacheEntry{
		fetchedAt: time.Now(),
		messages: []QueuedMessageItem{
			{ID: msgID, Text: "Cached to delete"},
			{ID: "msg-keep-456", Text: "Cached keep"},
		},
	}
	pendingCacheMu.Unlock()

	RemovePendingMessageFromCache(cascadeID, msgID)

	pendingCacheMu.RLock()
	cached := pendingCache[cascadeID]
	pendingCacheMu.RUnlock()

	if len(cached.messages) != 1 || cached.messages[0].ID != "msg-keep-456" {
		t.Fatalf("expected msg-to-delete-123 evicted from cache, got: %+v", cached.messages)
	}

	// 5. Test RemoveDeletedMessageTombstone
	RemoveDeletedMessageTombstone(cascadeID, msgID)
	if IsMessageDeleted(cascadeID, msgID) {
		t.Fatalf("expected tombstone removed")
	}
}
