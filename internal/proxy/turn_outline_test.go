package proxy

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
)

func TestBuildTurnOutline(t *testing.T) {
	intPtr := func(v int) *int { return &v }
	msgs := []CascadeMessageItem{
		{ID: "step-0", Type: "user", Role: "user", Text: "第一轮提问\n第二行补充", StepIndex: intPtr(0), Timestamp: "2026-11-22T10:00:00Z"},
		{ID: "tools-1", Type: "tools", Role: "tools", ToolCount: 3, ToolNames: []string{"read_file"}},
		{ID: "step-3", Type: "agent", Role: "agent", Text: "第一轮回复", StepIndex: intPtr(3), ModelName: "Gemini 4 Neon", Artifacts: []ArtifactItem{{URI: "file:///a.md", Title: "Plan", Summary: "very long summary that should be omitted"}}},
		{ID: "step-4", Type: "user", Role: "user", Text: "<talker_agent_context>\nuser: 帮我查一下今天的天气\ntalker: 好的\n</talker_agent_context>", StepIndex: intPtr(4), Timestamp: "2026-11-22T10:05:00Z"},
		{ID: "step-5", Type: "error", Role: "error", Text: "rate limited", StepIndex: intPtr(5)},
		{ID: "step-6", Type: "user", Role: "user", Text: attachmentHeader + "\n- /tmp/spec.pdf (pdf, 1.2 MB)", StepIndex: intPtr(6)},
		{ID: "step-7", Type: "agent", Role: "agent", Text: "已读取附件", StepIndex: intPtr(7)},
	}

	outline := buildTurnOutline(msgs)
	if len(outline) != 3 {
		t.Fatalf("expected 3 outline turns, got %d: %+v", len(outline), outline)
	}

	// Turn 1
	if outline[0].TurnIndex != 1 || outline[0].FirstMessageID != "step-0" || outline[0].MessageOffset != 0 || outline[0].MessageCount != 3 {
		t.Fatalf("unexpected turn 1 metadata: %+v", outline[0])
	}
	if outline[0].PromptSummary != "第一轮提问 第二行补充" {
		t.Fatalf("unexpected turn 1 summary: %q", outline[0].PromptSummary)
	}
	if outline[0].TotalTools != 3 || outline[0].ModelName != "Gemini 4 Neon" || len(outline[0].Artifacts) != 1 || outline[0].Artifacts[0].Summary != "" {
		t.Fatalf("unexpected turn 1 enriched fields: %+v", outline[0])
	}
	if outline[0].StartStepIndex == nil || *outline[0].StartStepIndex != 0 || outline[0].EndStepIndex == nil || *outline[0].EndStepIndex != 3 {
		t.Fatalf("unexpected turn 1 step bounds: start=%v end=%v", outline[0].StartStepIndex, outline[0].EndStepIndex)
	}

	// Turn 2 (voice + error)
	if !outline[1].IsVoiceTurn || outline[1].PromptSummary != "帮我查一下今天的天气" || !outline[1].HasError || outline[1].MessageOffset != 3 || outline[1].MessageCount != 2 {
		t.Fatalf("unexpected turn 2: %+v", outline[1])
	}

	// Turn 3 (attachment-only)
	if outline[2].PromptSummary != "[附件: spec.pdf]" || outline[2].MessageOffset != 5 || outline[2].MessageCount != 2 {
		t.Fatalf("unexpected turn 3: %+v", outline[2])
	}
}

func TestCascadeMessagesTurnOutlineAndFromOffset(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const cid = "cascade-outline-range-test"

	// Build a 25-turn (50-message) trajectory.
	var stepParts []string
	for i := 1; i <= 25; i++ {
		stepParts = append(stepParts,
			fmt.Sprintf(`{"type":"CORTEX_STEP_TYPE_USER_INPUT","status":"CORTEX_STEP_STATUS_DONE","userInput":{"items":[{"text":"第 %d 轮测试提问"}]}}`, i),
			fmt.Sprintf(`{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","status":"CORTEX_STEP_STATUS_DONE","plannerResponse":{"response":"第 %d 轮回复"}}`, i),
		)
	}
	rawTrajJSON := `{"status":"CASCADE_RUN_STATUS_IDLE","trajectory":{"cascadeId":"` + cid + `","summary":"long-session","steps":[` + strings.Join(stepParts, ",") + `]}}`

	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") {
			_, _ = w.Write([]byte(rawTrajJSON))
			return
		}
		_, _ = w.Write([]byte("{}"))
	}))
	defer upstream.Close()
	defer ClearTrajectoryCache(cid)

	port := upstream.Listener.Addr().(*net.TCPAddr).Port
	p := NewProxy(&stubDiscoverer{info: &inspector.InstanceInfo{
		PID:          2002,
		Port:         port,
		CSRFToken:    "tok",
		DiscoveredAt: time.Now(),
		IsHealthy:    true,
	}})

	// 1. Initial tail fetch: only 15 messages returned, but full 25-turn outline is present.
	req1 := httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId="+cid+"&limit=15", nil)
	rec1 := httptest.NewRecorder()
	p.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200 on initial fetch, got %d", rec1.Code)
	}
	var resp1 CascadeMessagesResponse
	if err := json.Unmarshal(rec1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("decode resp1: %v", err)
	}
	if len(resp1.Messages) != 15 || !resp1.HasMore || resp1.NextOffset != 35 || resp1.TotalMessages != 50 {
		t.Fatalf("unexpected tail slice: len=%d hasMore=%v nextOffset=%d total=%d", len(resp1.Messages), resp1.HasMore, resp1.NextOffset, resp1.TotalMessages)
	}
	if len(resp1.TurnOutline) != 25 {
		t.Fatalf("expected full 25-turn outline, got %d", len(resp1.TurnOutline))
	}
	if resp1.TurnOutline[0].TurnIndex != 1 || resp1.TurnOutline[0].MessageOffset != 0 || resp1.TurnOutline[0].FirstMessageID != "step-0" {
		t.Fatalf("unexpected turn 1 outline: %+v", resp1.TurnOutline[0])
	}

	// 2. Range jump fetch: fromOffset=0&offset=35 returns all 35 missing earlier messages in 1 request.
	req2 := httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId="+cid+"&fromOffset=0&offset=35&limit=50", nil)
	rec2 := httptest.NewRecorder()
	p.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 on range fetch, got %d", rec2.Code)
	}
	var resp2 CascadeMessagesResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("decode resp2: %v", err)
	}
	if len(resp2.Messages) != 35 || resp2.HasMore || resp2.NextOffset != 0 {
		t.Fatalf("unexpected range slice: len=%d hasMore=%v nextOffset=%d", len(resp2.Messages), resp2.HasMore, resp2.NextOffset)
	}
	if resp2.Messages[0].ID != "step-0" {
		t.Fatalf("expected first message to be step-0, got %s", resp2.Messages[0].ID)
	}

	// 3. Mid-session range jump: fromOffset=20&offset=35 returns 15 messages with hasMore=true, nextOffset=20.
	req3 := httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId="+cid+"&fromOffset=20&offset=35", nil)
	rec3 := httptest.NewRecorder()
	p.ServeHTTP(rec3, req3)
	var resp3 CascadeMessagesResponse
	if err := json.Unmarshal(rec3.Body.Bytes(), &resp3); err != nil {
		t.Fatalf("decode resp3: %v", err)
	}
	if len(resp3.Messages) != 15 || !resp3.HasMore || resp3.NextOffset != 20 {
		t.Fatalf("unexpected mid range slice: len=%d hasMore=%v nextOffset=%d", len(resp3.Messages), resp3.HasMore, resp3.NextOffset)
	}
}

func TestStreamDeltaTurnOutlineDeduplication(t *testing.T) {
	var d streamDeltaState
	outline := []TurnOutlineItem{
		{TurnIndex: 1, FirstMessageID: "step-0", MessageOffset: 0, MessageCount: 2, PromptSummary: "hello"},
	}

	// 1. Init frame always retains TurnOutline.
	initPayload := StreamUpdatePayload{
		Type:        "init",
		Messages:    []CascadeMessageItem{{ID: "step-0", Type: "user", Text: "hello"}, {ID: "step-1", Type: "agent", Text: "hi"}},
		TurnOutline: outline,
	}
	d.apply(&initPayload)
	if len(initPayload.TurnOutline) != 1 {
		t.Fatalf("expected init frame to keep TurnOutline, got %d", len(initPayload.TurnOutline))
	}

	// 2. Streaming text delta where TurnOutline did not change -> TurnOutline omitted.
	delta1 := StreamUpdatePayload{
		Type:        "update",
		Messages:    []CascadeMessageItem{{ID: "step-0", Type: "user", Text: "hello"}, {ID: "step-1", Type: "agent", Text: "hi there streaming..."}},
		TurnOutline: outline,
	}
	d.apply(&delta1)
	if delta1.TurnOutline != nil {
		t.Fatalf("expected unchanged delta frame to omit TurnOutline, got %+v", delta1.TurnOutline)
	}

	// 3. New turn added -> TurnOutline re-emitted.
	outline2 := append(outline, TurnOutlineItem{TurnIndex: 2, FirstMessageID: "step-2", MessageOffset: 2, MessageCount: 1, PromptSummary: "next"})
	delta2 := StreamUpdatePayload{
		Type:        "update",
		Messages:    []CascadeMessageItem{{ID: "step-0", Type: "user", Text: "hello"}, {ID: "step-1", Type: "agent", Text: "hi there streaming..."}, {ID: "step-2", Type: "user", Text: "next"}},
		TurnOutline: outline2,
	}
	d.apply(&delta2)
	if len(delta2.TurnOutline) != 2 {
		t.Fatalf("expected changed delta frame to include updated TurnOutline (len 2), got %d", len(delta2.TurnOutline))
	}
}
