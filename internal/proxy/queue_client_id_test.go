package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// 真实实验里从 language_server 读到的队列条目 stepPayload：消息「排队消息C」，tags=["mgy-client:test-C"]。
const realQueuedStepPayload = "CA4gAyoPCgsIxbqh1gYQ0LnsARgEmgGQARIN5o6S6Zif5raI5oGvQxoPCg3mjpLpmJ/mtojmga9DIgBiKwoaCKgK4gEUZ2VtaW5pLTMuOC1mbGFzaC1sb3cSDQjgxQgQASiA0A+4AQFqKwoaCKgK4gEUZ2VtaW5pLTMuOC1mbGFzaC1sb3cSDQjgxQgQASiA0A+4AQGCARFtZ3ktY2xpZW50OnRlc3QtQw=="

func TestApplyClientMessageTag(t *testing.T) {
	m := map[string]interface{}{}
	if !applyClientMessageTag(m, "abc-123_X") {
		t.Fatal("valid id must be applied")
	}
	tags, _ := m["tags"].([]interface{})
	if len(tags) != 1 || tags[0] != "mgy-client:abc-123_X" {
		t.Fatalf("tags = %v", m["tags"])
	}
	if applyClientMessageTag(m, "abc-123_X") {
		t.Error("applying twice must be a no-op")
	}
	if !applyClientMessageTag(m, "other") || len(m["tags"].([]interface{})) != 2 {
		t.Errorf("must append alongside existing tags: %v", m["tags"])
	}
	for _, bad := range []string{"", "   ", "has space", "a:b", strings.Repeat("x", 65), "中文"} {
		fresh := map[string]interface{}{}
		if applyClientMessageTag(fresh, bad) || len(fresh) != 0 {
			t.Errorf("invalid id %q must be ignored", bad)
		}
	}
	weird := map[string]interface{}{"tags": "not-a-list"}
	if applyClientMessageTag(weird, "id1") {
		t.Error("malformed tags must be left alone")
	}
}

func TestExtractQueuedClientMessageID_RealPayload(t *testing.T) {
	pam := upstreamAgentMessage{ID: "m1", StepPayload: json.RawMessage(`"` + realQueuedStepPayload + `"`)}
	if got := extractQueuedClientMessageID(pam); got != "test-C" {
		t.Fatalf("got %q, want test-C", got)
	}
	if got := extractQueuedMessageText(pam); got != "排队消息C" {
		t.Errorf("text extraction regressed: %q", got)
	}
	// 通过 Payload.Case 形式携带同样有效
	viaCase := upstreamAgentMessage{Payload: &struct {
		Case  string          `json:"case"`
		Value json.RawMessage `json:"value"`
	}{Case: "stepPayload", Value: json.RawMessage(`"` + realQueuedStepPayload + `"`)}}
	if got := extractQueuedClientMessageID(viaCase); got != "test-C" {
		t.Errorf("via payload case: %q", got)
	}
}

func TestExtractQueuedClientMessageID_NoTagAndGarbage(t *testing.T) {
	for _, raw := range []string{``, `""`, `"not base64!!"`, `"AAAA"`, `123`, `{"x":1}`} {
		pam := upstreamAgentMessage{}
		if raw != "" {
			pam.StepPayload = json.RawMessage(raw)
		}
		if got := extractQueuedClientMessageID(pam); got != "" {
			t.Errorf("payload %s => %q, want empty", raw, got)
		}
	}
}

func TestParseAgentStateQueuedMessages_ExposesClientMessageID(t *testing.T) {
	state := &upstreamAgentStateUpdate{}
	state.Update.PendingAgentMessages = []upstreamAgentMessage{{
		ID:               "server-id-1",
		Sender:           "user",
		StepPayload:      json.RawMessage(`"` + realQueuedStepPayload + `"`),
		DeliveryStrategy: "MESSAGE_DELIVERY_STRATEGY_WHEN_IDLE",
	}}
	items := parseAgentStateQueuedMessages(state)
	if len(items) != 1 || items[0].ID != "server-id-1" || items[0].ClientMessageID != "test-C" || items[0].Text != "排队消息C" {
		t.Fatalf("items = %+v", items)
	}
	data, _ := json.Marshal(items[0])
	if !strings.Contains(string(data), `"clientMessageId":"test-C"`) {
		t.Errorf("json = %s", data)
	}
}

func TestSendTagsQueuedMessagesWithClientID(t *testing.T) {
	var calls int32
	var sent map[string]interface{}
	p := newSlashTestProxy(t, &calls, &sent)

	rec := doSend(p, `{"cascadeId":"c1","items":[{"text":"排队"}],"deliveryStrategy":2}`, map[string]string{"X-Client-Message-Id": "0b8f-uuid"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	tags, _ := sent["tags"].([]interface{})
	if len(tags) != 1 || tags[0] != "mgy-client:0b8f-uuid" {
		t.Fatalf("queued message must carry the client id tag, got %v", sent["tags"])
	}

	// 没有 client id：不打标签
	sent = nil
	doSend(p, `{"cascadeId":"c1","items":[{"text":"排队2"}],"deliveryStrategy":2}`, nil)
	if _, has := sent["tags"]; has {
		t.Errorf("no client id => no tag, got %v", sent["tags"])
	}
	// 非法 client id（例如带冒号）：忽略，消息照常发送
	sent = nil
	rec = doSend(p, `{"cascadeId":"c1","items":[{"text":"排队3"}],"deliveryStrategy":2}`, map[string]string{"X-Client-Message-Id": "bad:id"})
	if rec.Code != http.StatusOK {
		t.Fatalf("invalid id must not fail the send: %d", rec.Code)
	}
	if _, has := sent["tags"]; has {
		t.Errorf("invalid id must not be tagged, got %v", sent["tags"])
	}
}
