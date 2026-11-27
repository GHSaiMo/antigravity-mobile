package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"antigravity-mobile/internal/inspector"
)

func extractTestPort(t *testing.T, rawURL string) int {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return p
}

func TestCascadeStitch_AccountSwitchHandoverAndTrajectoryMerge(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MGY_STITCH_STATE_PATH", filepath.Join(tmpDir, "stitches.json"))
	t.Setenv("MGY_SWITCH_COMPACT_HANDOVER", "true")
	ResetCascadeStitchStoreForTest()
	t.Cleanup(ResetCascadeStitchStoreForTest)

	var (
		mu              sync.Mutex
		startedCount    int
		sentPayloads    []map[string]interface{}
		canceledTargets []string
	)

	const (
		rootID  = "11111111-1111-1111-1111-111111111111"
		childID = "22222222-2222-2222-2222-222222222222"
	)

	childSteps := []map[string]interface{}{}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			var req struct {
				CascadeID string `json:"cascadeId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			switch req.CascadeID {
			case rootID:
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"status": "CASCADE_RUN_STATUS_IDLE",
					"trajectory": map[string]interface{}{
						"trajectoryId":  "traj-root",
						"cascadeId":     rootID,
						"workspaceUris": []string{"file:///workspace/demo"},
						"steps": []map[string]interface{}{
							{
								"type":   "CORTEX_STEP_TYPE_USER_INPUT",
								"status": "CORTEX_STEP_STATUS_DONE",
								"userInput": map[string]interface{}{
									"userResponse": "Build a tmux monitor script",
								},
							},
							{
								"type":   "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
								"status": "CORTEX_STEP_STATUS_DONE",
								"plannerResponse": map[string]interface{}{
									"response": "I have inspected the tmux panes and designed agy_monitor.py.",
								},
							},
						},
					},
				})
			case childID:
				mu.Lock()
				stepsCopy := append([]map[string]interface{}(nil), childSteps...)
				mu.Unlock()
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"status": "CASCADE_RUN_STATUS_IDLE",
					"trajectory": map[string]interface{}{
						"trajectoryId":  "traj-child",
						"cascadeId":     childID,
						"workspaceUris": []string{"file:///workspace/demo"},
						"steps":         stepsCopy,
					},
				})
			default:
				w.WriteHeader(http.StatusNotFound)
			}

		case strings.HasSuffix(r.URL.Path, "/StartCascade"):
			mu.Lock()
			startedCount++
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"cascadeId": childID,
			})

		case strings.HasSuffix(r.URL.Path, "/SendUserCascadeMessage"):
			var payload map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			mu.Lock()
			sentPayloads = append(sentPayloads, payload)
			var sentText string
			if items, ok := payload["items"].([]interface{}); ok && len(items) > 0 {
				if im, ok := items[0].(map[string]interface{}); ok {
					sentText, _ = im["text"].(string)
				}
			}
			childSteps = append(childSteps,
				map[string]interface{}{
					"type":   "CORTEX_STEP_TYPE_USER_INPUT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"userInput": map[string]interface{}{
						"userResponse": sentText,
					},
				},
				map[string]interface{}{
					"type":   "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": map[string]interface{}{
						"response": "Continuing with the deployment on the backup account.",
					},
				},
			)
			mu.Unlock()
			w.Write([]byte(`{}`))

		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"trajectorySummaries": map[string]interface{}{
					rootID: map[string]interface{}{
						"summary":          "Tmux Monitor Task",
						"status":           "CASCADE_RUN_STATUS_IDLE",
						"stepCount":        2,
						"lastModifiedTime": "2026-11-27T08:00:00Z",
					},
					childID: map[string]interface{}{
						"summary":          "Compacted Continuation",
						"status":           "CASCADE_RUN_STATUS_RUNNING",
						"stepCount":        2,
						"lastModifiedTime": "2026-11-27T09:00:00Z",
					},
				},
			})

		case strings.HasSuffix(r.URL.Path, "/CancelCascadeInvocation"):
			var req map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&req)
			cid, _ := req["cascadeId"].(string)
			mu.Lock()
			canceledTargets = append(canceledTargets, cid)
			mu.Unlock()
			w.Write([]byte(`{}`))

		default:
			w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	port := extractTestPort(t, srv.URL)
	p := NewProxy(&inspector.Inspector{})
	p.SetTestUpstream(port, "test-csrf")
	ClearTrajectoryCache(rootID)
	ClearTrajectoryCache(childID)

	// 1. Trigger account switch notification
	p.NotifyAccountSwitched("backup@example.com")

	// 2. Client sends a message to the original rootID ("继续部署")
	sendBody, _ := json.Marshal(map[string]interface{}{
		"cascadeId": rootID,
		"items":     []interface{}{map[string]interface{}{"text": "继续部署"}},
	})
	sendReq := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", bytes.NewReader(sendBody))
	sendRec := httptest.NewRecorder()
	p.ServeHTTP(sendRec, sendReq)
	if sendRec.Code != http.StatusOK {
		t.Fatalf("SendUserCascadeMessage returned %d: %s", sendRec.Code, sendRec.Body.String())
	}

	mu.Lock()
	if startedCount != 1 {
		t.Fatalf("expected 1 StartCascade call on account switch rollover, got %d", startedCount)
	}
	if len(sentPayloads) != 1 {
		t.Fatalf("expected 1 SendUserCascadeMessage upstream call, got %d", len(sentPayloads))
	}
	gotTargetID, _ := sentPayloads[0]["cascadeId"].(string)
	if gotTargetID != childID {
		t.Fatalf("expected upstream SendUserCascadeMessage target to be rewritten to %s, got %s", childID, gotTargetID)
	}
	var upstreamPrompt string
	if items, ok := sentPayloads[0]["items"].([]interface{}); ok && len(items) > 0 {
		if im, ok := items[0].(map[string]interface{}); ok {
			upstreamPrompt, _ = im["text"].(string)
		}
	}
	mu.Unlock()

	if !strings.Contains(upstreamPrompt, "Build a tmux monitor script") || !strings.Contains(upstreamPrompt, "继续部署") {
		t.Fatalf("expected upstream prompt to contain compacted history and user message, got:\n%s", upstreamPrompt)
	}

	// 3. Subsequent message under the SAME account should reuse childID without calling StartCascade again
	sendBody2, _ := json.Marshal(map[string]interface{}{
		"cascadeId": rootID,
		"items":     []interface{}{map[string]interface{}{"text": "第二步验证"}},
	})
	sendReq2 := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", bytes.NewReader(sendBody2))
	sendRec2 := httptest.NewRecorder()
	p.ServeHTTP(sendRec2, sendReq2)
	if sendRec2.Code != http.StatusOK {
		t.Fatalf("second SendUserCascadeMessage returned %d", sendRec2.Code)
	}
	mu.Lock()
	if startedCount != 1 {
		t.Fatalf("expected no additional StartCascade on same account, got %d", startedCount)
	}
	mu.Unlock()

	// 4. Client fetches messages for rootID: should see seamless merged history (root + divider + child)
	// and the internal compact handover block must be stripped from the user message!
	ClearTrajectoryCache(rootID)
	ClearTrajectoryCache(childID)
	msgReq := httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId="+rootID+"&limit=50", nil)
	msgRec := httptest.NewRecorder()
	p.ServeHTTP(msgRec, msgReq)
	if msgRec.Code != http.StatusOK {
		t.Fatalf("/gateway/cascade/messages returned %d: %s", msgRec.Code, msgRec.Body.String())
	}
	var msgResp CascadeMessagesResponse
	if err := json.Unmarshal(msgRec.Body.Bytes(), &msgResp); err != nil {
		t.Fatalf("unmarshal CascadeMessagesResponse: %v", err)
	}
	if msgResp.CascadeID != rootID {
		t.Fatalf("expected response cascadeId=%s, got %s", rootID, msgResp.CascadeID)
	}

	var texts []string
	for _, m := range msgResp.Messages {
		if strings.Contains(m.Text, stitchPromptHeaderMarker) {
			t.Fatalf("internal compact handover marker leaked to client message: %s", m.Text)
		}
		texts = append(texts, m.Text)
	}
	joined := strings.Join(texts, "\n")
	for _, want := range []string{
		"Build a tmux monitor script",
		"I have inspected the tmux panes",
		"Compact Handover",
		"继续部署",
		"Continuing with the deployment on the backup account.",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("merged client messages missing %q; got:\n%s", want, joined)
		}
	}

	// Regression check: repeated trajectory fetches across cache hits/misses must NOT
	// overwrite segment 0 (rootID) in trajCache with the combined slice and duplicate steps!
	firstCount := len(msgResp.Messages)
	for poll := 0; poll < 4; poll++ {
		expireTrajectoryCacheFreshness(rootID)
		msgReqPoll := httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId="+rootID+"&limit=50", nil)
		msgRecPoll := httptest.NewRecorder()
		p.ServeHTTP(msgRecPoll, msgReqPoll)
		var pollResp CascadeMessagesResponse
		if err := json.Unmarshal(msgRecPoll.Body.Bytes(), &pollResp); err != nil {
			t.Fatalf("poll %d unmarshal: %v", poll, err)
		}
		if len(pollResp.Messages) != firstCount {
			t.Fatalf("poll %d duplicated messages: got %d messages, want %d", poll, len(pollResp.Messages), firstCount)
		}
		dividerCount := 0
		for _, m := range pollResp.Messages {
			if strings.Contains(m.Text, "Compact Handover") {
				dividerCount++
			}
		}
		if dividerCount != 1 {
			t.Fatalf("poll %d expected exactly 1 Compact Handover divider, got %d", poll, dividerCount)
		}
	}

	// 5. Client fetches conversation list: childID must be hidden and its RUNNING status propagated to rootID
	listReq := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", strings.NewReader(`{}`))
	listRec := httptest.NewRecorder()
	p.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("GetAllCascadeTrajectories returned %d", listRec.Code)
	}
	var listResp struct {
		TrajectorySummaries map[string]map[string]interface{} `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal listResp: %v", err)
	}
	if _, leaked := listResp.TrajectorySummaries[childID]; leaked {
		t.Fatalf("expected stitched child cascade %s to be hidden from conversation list", childID)
	}
	rootSummary := listResp.TrajectorySummaries[rootID]
	if rootSummary == nil {
		t.Fatalf("expected root cascade %s in conversation list", rootID)
	}
	if st, _ := rootSummary["status"].(string); st != "CASCADE_RUN_STATUS_RUNNING" {
		t.Errorf("expected root summary status to reflect active child RUNNING status, got %q", st)
	}

	// 6. CancelCascadeInvocation on rootID should transparently route to childID
	cancelBody, _ := json.Marshal(map[string]interface{}{"cascadeId": rootID})
	cancelReq := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/CancelCascadeInvocation", bytes.NewReader(cancelBody))
	cancelRec := httptest.NewRecorder()
	p.ServeHTTP(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("CancelCascadeInvocation returned %d", cancelRec.Code)
	}
	mu.Lock()
	if len(canceledTargets) != 1 || canceledTargets[0] != childID {
		t.Fatalf("expected CancelCascadeInvocation to target active child %s, got %v", childID, canceledTargets)
	}
	mu.Unlock()
}
