package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
)

func TestGatewayStatusIncludesIdentityAndSummary(t *testing.T) {
	insp := inspector.NewInspector(10 * time.Second)
	p := NewProxy(insp)
	p.SetGatewayIdentity("gw_0123456789abcdef", "Mac-mini")

	// Seed a warm trajectories snapshot for active port 12345 so handleStatus computes summary with zero network I/O.
	p.mu.Lock()
	p.activePort = 12345
	p.activeToken = "tok"
	p.mu.Unlock()

	raw := []byte(`{
		"trajectorySummaries": {
			"c-action": {"status":"CASCADE_RUN_STATUS_RUNNING","needsInput":true,"stepCount":4,"summary":"Approve cmd"},
			"c-run": {"status":"CASCADE_RUN_STATUS_RUNNING","stepCount":2,"summary":"Building"},
			"c-unread": {"status":"CASCADE_RUN_STATUS_IDLE","stepCount":3,"summary":"Finished","annotations":{"markedAsUnread":true}},
			"c-unread-ts": {"status":"CASCADE_RUN_STATUS_IDLE","stepCount":3,"summary":"Updated","lastModifiedTime":"2026-11-25T10:00:00Z","annotations":{"lastUserViewTime":"2026-11-25T09:00:00Z"}},
			"c-read": {"status":"CASCADE_RUN_STATUS_IDLE","stepCount":3,"summary":"Read","lastModifiedTime":"2026-11-25T08:00:00Z","annotations":{"lastUserViewTime":"2026-11-25T09:00:00Z"}},
			"c-arch": {"status":"CASCADE_RUN_STATUS_IDLE","stepCount":3,"summary":"Archived","annotations":{"archived":true,"markedAsUnread":true}}
		}
	}`)
	p.allTrajectories.mu.Lock()
	p.allTrajectories.port = 12345
	p.allTrajectories.token = "tok"
	p.allTrajectories.body = raw
	p.allTrajectories.fetchedAt = time.Now()
	p.allTrajectories.mu.Unlock()

	sum := p.computeGatewaySummary(t.Context(), false)
	if sum == nil {
		t.Fatal("expected non-nil GatewaySummary")
	}
	if sum.ActionCount != 1 {
		t.Errorf("expected ActionCount=1, got %d", sum.ActionCount)
	}
	if sum.RunningCount != 1 {
		t.Errorf("expected RunningCount=1, got %d", sum.RunningCount)
	}
	if sum.UnreadCount != 2 {
		t.Errorf("expected UnreadCount=2, got %d", sum.UnreadCount)
	}
	if sum.TotalActive != 5 {
		t.Errorf("expected TotalActive=5 (excluding archived), got %d", sum.TotalActive)
	}

	req := httptest.NewRequest(http.MethodGet, "/gateway/status", nil)
	rr := httptest.NewRecorder()
	p.handleStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var st GatewayStatus
	if err := json.Unmarshal(rr.Body.Bytes(), &st); err != nil {
		t.Fatalf("unmarshal GatewayStatus: %v", err)
	}
	if st.GatewayID != "gw_0123456789abcdef" || st.GatewayName != "Mac-mini" {
		t.Errorf("unexpected gateway identity: id=%q name=%q", st.GatewayID, st.GatewayName)
	}
}
