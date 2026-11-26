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
)

func TestScheduledTaskPromptTagsOnlyFirstPrompt(t *testing.T) {
	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)

	var raw upstreamTrajectoryResp
	raw.Status = "CASCADE_RUN_STATUS_IDLE"
	raw.Trajectory.CascadeID = "sched-conv-1"
	raw.Trajectory.Metadata = &struct {
		ParentConversationID string                 `json:"parentConversationId"`
		ProjectID            string                 `json:"projectId,omitempty"`
		SourceMetadata       map[string]interface{} `json:"sourceMetadata,omitempty"`
		SubagentSpec         *struct {
			TypeName string `json:"typeName"`
			Role     string `json:"role"`
		} `json:"subagentSpec"`
	}{
		SourceMetadata: map[string]interface{}{
			"sidecar": map[string]interface{}{
				"sidecarId": "daily-ops-patrol",
				"config":    map[string]interface{}{"builtin": "schedule"},
			},
		},
	}
	raw.Trajectory.Steps = []TrajectoryStep{
		{
			Type:      "CORTEX_STEP_TYPE_USER_INPUT",
			Status:    "CORTEX_STEP_STATUS_DONE",
			UserInput: &TrajectoryUserInput{UserResponse: "Run the full daily growth ops checklist and output dashboard."},
		},
		{
			Type:            "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
			Status:          "CORTEX_STEP_STATUS_DONE",
			PlannerResponse: &TrajectoryPlannerResponse{Response: "Done, here is the dashboard summary."},
		},
		{
			Type:      "CORTEX_STEP_TYPE_USER_INPUT",
			Status:    "CORTEX_STEP_STATUS_DONE",
			UserInput: &TrajectoryUserInput{UserResponse: "Can you expand on item 2?"},
		},
		{
			Type:            "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
			Status:          "CORTEX_STEP_STATUS_DONE",
			PlannerResponse: &TrajectoryPlannerResponse{Response: "Sure, item 2 details..."},
		},
	}

	details := p.ParseTrajectoryDetails(&raw)
	if !details.IsScheduledTask {
		t.Fatalf("expected details.IsScheduledTask=true")
	}

	var userMsgs []CascadeMessageItem
	for _, m := range details.AllMessages {
		if m.Type == "user" {
			userMsgs = append(userMsgs, m)
		}
	}
	if len(userMsgs) != 2 {
		t.Fatalf("expected 2 user messages, got %d", len(userMsgs))
	}
	if !userMsgs[0].IsScheduledTask {
		t.Fatalf("expected first user message to have IsScheduledTask=true")
	}
	if userMsgs[1].IsScheduledTask {
		t.Fatalf("expected manual follow-up user message to have IsScheduledTask=false")
	}
	if len(details.TurnOutline) != 2 {
		t.Fatalf("expected 2 turn outline entries, got %d", len(details.TurnOutline))
	}
	if !details.TurnOutline[0].IsScheduledTask {
		t.Fatalf("expected first TurnOutlineItem to have IsScheduledTask=true")
	}
	if details.TurnOutline[1].IsScheduledTask {
		t.Fatalf("expected second TurnOutlineItem to have IsScheduledTask=false")
	}

	// Clean up cache entry
	RecordDeletedCascade("sched-conv-1")
	RemoveDeletedCascadeTombstone("sched-conv-1")
}

func TestScheduledTaskPromptGuards(t *testing.T) {
	step0 := 0
	step2 := 2

	t.Run("subagent is not marked scheduled", func(t *testing.T) {
		d := TrajectoryDetails{
			ParentConversation: "sched-parent",
			AllMessages: []CascadeMessageItem{
				{ID: "step-0", Type: "user", Role: "user", Text: "subagent brief", StepIndex: &step0},
			},
			TurnOutline: []TurnOutlineItem{{TurnIndex: 1, FirstMessageID: "step-0"}},
		}
		applyScheduledTaskFlag(&d, true)
		if d.IsScheduledTask || d.AllMessages[0].IsScheduledTask || d.TurnOutline[0].IsScheduledTask {
			t.Fatalf("subagent should not be marked scheduled: %+v", d)
		}
	})

	t.Run("voice message keeps voice precedence", func(t *testing.T) {
		d := TrajectoryDetails{
			AllMessages: []CascadeMessageItem{
				{ID: "step-0", Type: "user", Role: "user", Text: "<talker_agent_context>user: hi</talker_agent_context>", StepIndex: &step0},
			},
			TurnOutline: []TurnOutlineItem{{TurnIndex: 1, FirstMessageID: "step-0", IsVoiceTurn: true}},
		}
		applyScheduledTaskFlag(&d, true)
		if d.AllMessages[0].IsScheduledTask || d.TurnOutline[0].IsScheduledTask {
			t.Fatalf("voice message should not be marked scheduled: %+v", d.AllMessages[0])
		}
	})

	t.Run("btw side question does not mark question at or after forkStepIndex", func(t *testing.T) {
		d := TrajectoryDetails{
			BranchOrigin: &SideQuestionOrigin{
				ParentCascadeID: "sched-parent",
				ForkStepIndex:   2,
				IsSideQuestion:  true,
			},
			AllMessages: []CascadeMessageItem{
				{ID: "step-2", Type: "user", Role: "user", Text: "Quick side question", StepIndex: &step2},
			},
			TurnOutline: []TurnOutlineItem{{TurnIndex: 1, FirstMessageID: "step-2"}},
		}
		applyScheduledTaskFlag(&d, true)
		if d.AllMessages[0].IsScheduledTask {
			t.Fatalf("side question user message after forkStepIndex should not be marked scheduled")
		}
	})

	t.Run("hashMessageItem, hashTurnOutline, and StreamUpdatePayload Fingerprint detect flag change", func(t *testing.T) {
		m1 := CascadeMessageItem{ID: "step-0", Type: "user", Role: "user", Text: "prompt"}
		m2 := m1
		m2.IsScheduledTask = true
		if hashMessageItem(&m1) == hashMessageItem(&m2) {
			t.Fatalf("hashMessageItem did not change when IsScheduledTask flipped")
		}
		o1 := []TurnOutlineItem{{TurnIndex: 1, FirstMessageID: "step-0", PromptSummary: "prompt"}}
		o2 := []TurnOutlineItem{{TurnIndex: 1, FirstMessageID: "step-0", PromptSummary: "prompt", IsScheduledTask: true}}
		if hashTurnOutline(o1) == hashTurnOutline(o2) {
			t.Fatalf("hashTurnOutline did not change when IsScheduledTask flipped")
		}
		p1 := StreamUpdatePayload{CascadeID: "c1", Status: "IDLE", TotalMessages: 1, Messages: []CascadeMessageItem{m1}}
		p2 := StreamUpdatePayload{CascadeID: "c1", Status: "IDLE", TotalMessages: 1, Messages: []CascadeMessageItem{m2}, IsScheduledTask: true}
		if p1.Fingerprint() == p2.Fingerprint() {
			t.Fatalf("StreamUpdatePayload.Fingerprint did not change when IsScheduledTask flipped")
		}
	})
}

func TestHandleCascadeMessagesResolvesScheduledTaskFromSummariesSnapshot(t *testing.T) {
	const cid = "sched-cold-open-1"
	RecordDeletedCascade(cid)
	RemoveDeletedCascadeTombstone(cid)
	t.Cleanup(func() {
		RecordDeletedCascade(cid)
		RemoveDeletedCascadeTombstone(cid)
	})

	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			// Trajectory itself omits sourceMetadata, mimicking cold deep-link where only summaries have it.
			_, _ = w.Write([]byte(`{
				"status": "CASCADE_RUN_STATUS_IDLE",
				"trajectory": {
					"cascadeId": "` + cid + `",
					"steps": [
						{"type": "CORTEX_STEP_TYPE_USER_INPUT", "status": "CORTEX_STEP_STATUS_DONE", "userInput": {"userResponse": "Long scheduled patrol instructions..."}},
						{"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE", "status": "CORTEX_STEP_STATUS_DONE", "plannerResponse": {"response": "Patrol complete."}}
					]
				}
			}`))
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			_, _ = w.Write([]byte(`{
				"trajectorySummaries": {
					"` + cid + `": {
						"summary": "Growth Ops Patrol",
						"status": "CASCADE_RUN_STATUS_IDLE",
						"trajectoryMetadata": {
							"sourceMetadata": {
								"sidecar": {
									"sidecarId": "non-x-growth-ops",
									"config": {"builtin": "schedule"}
								}
							}
						}
					}
				}
			}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)
	p.activePort = port
	p.activeToken = "tok"

	req := httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId="+cid+"&limit=15", nil)
	rec := httptest.NewRecorder()
	p.handleCascadeMessages(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp CascadeMessagesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.IsScheduledTask {
		t.Fatalf("expected resp.IsScheduledTask=true")
	}
	if len(resp.Messages) < 1 || !resp.Messages[0].IsScheduledTask {
		t.Fatalf("expected first message to have IsScheduledTask=true, got %+v", resp.Messages)
	}
	if len(resp.TurnOutline) < 1 || !resp.TurnOutline[0].IsScheduledTask {
		t.Fatalf("expected first turn outline entry to have IsScheduledTask=true, got %+v", resp.TurnOutline)
	}
}
