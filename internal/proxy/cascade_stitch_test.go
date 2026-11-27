package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

func TestStripCompactHandoverPrefix_CurrentAndLegacyMarkers(t *testing.T) {
	wrapped := WrapUserPromptWithCompactHandover("[Summary]\nline 1", "hello world")
	if strings.Contains(wrapped, "MGY_COMPACT_HANDOVER") {
		t.Fatalf("WrapUserPromptWithCompactHandover should not emit legacy tool fingerprint: %s", wrapped)
	}
	if !strings.Contains(wrapped, stitchPromptHeaderMarker) {
		t.Fatalf("expected neutral header marker %q in wrapped prompt: %s", stitchPromptHeaderMarker, wrapped)
	}
	if got := StripCompactHandoverPrefix(wrapped); got != "hello world" {
		t.Fatalf("StripCompactHandoverPrefix(current) = %q, want %q", got, "hello world")
	}

	legacy := legacyStitchPromptHeaderMarker + "\n[Old Summary]\n" + legacyStitchPromptEndMarker + "\n\nlegacy prompt"
	if got := StripCompactHandoverPrefix(legacy); got != "legacy prompt" {
		t.Fatalf("StripCompactHandoverPrefix(legacy) = %q, want %q", got, "legacy prompt")
	}
}

func TestCascadeStitch_SealedHistoricalCachePreservedAcrossClearAndPortSwitch(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MGY_STITCH_STATE_PATH", filepath.Join(tmpDir, "stitches.json"))
	t.Setenv("MGY_SWITCH_COMPACT_HANDOVER", "true")
	ResetCascadeStitchStoreForTest()
	t.Cleanup(ResetCascadeStitchStoreForTest)

	const (
		rootID  = "33333333-3333-3333-3333-333333333333"
		childID = "44444444-4444-4444-4444-444444444444"
	)
	ClearStitchedSegmentCaches(rootID, []string{rootID, childID})
	t.Cleanup(func() { ClearStitchedSegmentCaches(rootID, []string{rootID, childID}) })

	var rootFetchesPort1 atomic.Int32
	var rootFetchesPort2 atomic.Int32
	var childFetchesPort2 atomic.Int32

	makeServer := func(isPort2 bool) *httptest.Server {
		return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if !strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") {
				w.Write([]byte(`{}`))
				return
			}
			var req struct {
				CascadeID string `json:"cascadeId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			switch req.CascadeID {
			case rootID:
				if isPort2 {
					rootFetchesPort2.Add(1)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				rootFetchesPort1.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"status": "CASCADE_RUN_STATUS_IDLE",
					"trajectory": map[string]interface{}{
						"trajectoryId": "traj-root",
						"cascadeId":    rootID,
						"steps": []map[string]interface{}{
							{
								"type":      "CORTEX_STEP_TYPE_USER_INPUT",
								"status":    "CORTEX_STEP_STATUS_DONE",
								"userInput": map[string]interface{}{"userResponse": "sealed root turn"},
							},
						},
					},
				})
			case childID:
				if isPort2 {
					childFetchesPort2.Add(1)
				}
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"status": "CASCADE_RUN_STATUS_IDLE",
					"trajectory": map[string]interface{}{
						"trajectoryId": "traj-child",
						"cascadeId":    childID,
						"steps": []map[string]interface{}{
							{
								"type":      "CORTEX_STEP_TYPE_USER_INPUT",
								"status":    "CORTEX_STEP_STATUS_DONE",
								"userInput": map[string]interface{}{"userResponse": "active leaf turn"},
							},
						},
					},
				})
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	}

	srv1 := makeServer(false)
	defer srv1.Close()
	srv2 := makeServer(true)
	defer srv2.Close()

	p := NewProxy(&inspector.Inspector{})
	p.SetTestUpstream(extractTestPort(t, srv1.URL), "tok1")
	p.NotifyAccountSwitched("acc-b")
	defaultStitchStore.appendSegment(rootID, rootID, childID, "acc-b")

	// Initial fetch on port 1 populates sealed rootID and active childID.
	req1 := httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId="+rootID+"&limit=50", nil)
	rec1 := httptest.NewRecorder()
	p.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("initial fetch returned %d: %s", rec1.Code, rec1.Body.String())
	}
	if rootFetchesPort1.Load() != 1 {
		t.Fatalf("expected 1 root fetch on port 1, got %d", rootFetchesPort1.Load())
	}

	// Simulate send invalidation + account/port switch to port 2 (where rootID would 404).
	ClearTrajectoryCache(rootID)
	p.SetTestUpstream(extractTestPort(t, srv2.URL), "tok2")

	req2 := httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId="+rootID+"&limit=50", nil)
	rec2 := httptest.NewRecorder()
	p.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("post-clear port2 fetch returned %d: %s", rec2.Code, rec2.Body.String())
	}
	if rootFetchesPort2.Load() != 0 {
		t.Fatalf("expected sealed historical segment %s to be reused across ClearTrajectoryCache & port switch, got %d upstream fetches", rootID, rootFetchesPort2.Load())
	}
	if childFetchesPort2.Load() == 0 {
		t.Fatalf("expected active leaf %s to be refetched after ClearTrajectoryCache", childID)
	}
	if !strings.Contains(rec2.Body.String(), "sealed root turn") || !strings.Contains(rec2.Body.String(), "active leaf turn") {
		t.Fatalf("expected both sealed root and active leaf content in merged response: %s", rec2.Body.String())
	}
}

func TestCascadeStitch_StartCascadeRetryAndNoCrossAccountFallback(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MGY_STITCH_STATE_PATH", filepath.Join(tmpDir, "stitches.json"))
	t.Setenv("MGY_SWITCH_COMPACT_HANDOVER", "true")
	ResetCascadeStitchStoreForTest()
	t.Cleanup(ResetCascadeStitchStoreForTest)

	const (
		rootID  = "55555555-5555-5555-5555-555555555555"
		childID = "66666666-6666-6666-6666-666666666666"
	)
	ClearStitchedSegmentCaches(rootID, []string{rootID, childID})
	t.Cleanup(func() { ClearStitchedSegmentCaches(rootID, []string{rootID, childID}) })

	var (
		mu             sync.Mutex
		failStartCount int // fail the first N StartCascade calls
		startCalls     int
		sentTargets    []string
	)

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "CASCADE_RUN_STATUS_IDLE",
				"trajectory": map[string]interface{}{
					"cascadeId": rootID,
					"steps": []map[string]interface{}{
						{
							"type":      "CORTEX_STEP_TYPE_USER_INPUT",
							"status":    "CORTEX_STEP_STATUS_DONE",
							"userInput": map[string]interface{}{"userResponse": "initial work on account A"},
						},
					},
				},
			})
		case strings.HasSuffix(r.URL.Path, "/StartCascade"):
			mu.Lock()
			startCalls++
			shouldFail := startCalls <= failStartCount
			mu.Unlock()
			if shouldFail {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(`{"code":"unavailable"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"cascadeId": childID})
		case strings.HasSuffix(r.URL.Path, "/SendUserCascadeMessage"):
			var req map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			cid, _ := req["cascadeId"].(string)
			sentTargets = append(sentTargets, cid)
			mu.Unlock()
			w.Write([]byte(`{}`))
		default:
			w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	p := NewProxy(&inspector.Inspector{})
	p.SetTestUpstream(extractTestPort(t, srv.URL), "tok")
	p.NotifyAccountSwitched("acc-b")

	// Case 1: All 3 StartCascade attempts fail -> must return 503 and NEVER forward to old rootID.
	mu.Lock()
	failStartCount = 3
	mu.Unlock()
	body1, _ := json.Marshal(map[string]interface{}{
		"cascadeId": rootID,
		"items":     []interface{}{map[string]interface{}{"text": "first attempt during LS warmup"}},
	})
	rec1 := httptest.NewRecorder()
	p.ServeHTTP(rec1, httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", bytes.NewReader(body1)))
	if rec1.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when StartCascade fails all retries, got %d: %s", rec1.Code, rec1.Body.String())
	}
	mu.Lock()
	if len(sentTargets) != 0 {
		t.Fatalf("expected ZERO SendUserCascadeMessage calls on handover failure (no cross-account fallback), got %v", sentTargets)
	}
	if startCalls != 3 {
		t.Fatalf("expected 3 StartCascade attempts (initial + 2 retries), got %d", startCalls)
	}
	// Case 2: Next send fails StartCascade once (call #4) then succeeds on retry (call #5).
	failStartCount = 4
	mu.Unlock()

	body2, _ := json.Marshal(map[string]interface{}{
		"cascadeId": rootID,
		"items":     []interface{}{map[string]interface{}{"text": "retry after LS ready"}},
	})
	rec2 := httptest.NewRecorder()
	p.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", bytes.NewReader(body2)))
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 after StartCascade retry succeeded, got %d: %s", rec2.Code, rec2.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(sentTargets) != 1 || sentTargets[0] != childID {
		t.Fatalf("expected SendUserCascadeMessage to route to %s after retry, got %v", childID, sentTargets)
	}
	if startCalls != 5 {
		t.Fatalf("expected 5 total StartCascade calls, got %d", startCalls)
	}
}

func TestCascadeStitch_DeleteCleansStitchedChildrenAndPreservesChainOnFailure(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("MGY_STITCH_STATE_PATH", filepath.Join(tmpHome, ".multigravity", "stitches.json"))
	t.Setenv("MGY_SWITCH_COMPACT_HANDOVER", "true")
	ResetCascadeStitchStoreForTest()
	t.Cleanup(ResetCascadeStitchStoreForTest)

	const (
		rootID  = "77777777-7777-7777-7777-777777777777"
		childID = "88888888-8888-8888-8888-888888888888"
	)
	t.Cleanup(func() {
		RemoveDeletedCascadeTombstone(rootID)
		RemoveDeletedCascadeTombstone(childID)
		ClearStitchedSegmentCaches(rootID, []string{rootID, childID})
	})

	// Create fake local files for both rootID and childID under ~/.gemini/antigravity-cli
	agRoot := filepath.Join(tmpHome, ".gemini", "antigravity-cli")
	for _, cid := range []string{rootID, childID} {
		for _, pth := range []string{
			filepath.Join(agRoot, "conversations", cid+".db"),
			filepath.Join(agRoot, "conversations", cid+".db-wal"),
			filepath.Join(agRoot, "annotations", cid+".pbtxt"),
			filepath.Join(agRoot, "brain", cid, "artifact.md"),
		} {
			if err := os.MkdirAll(filepath.Dir(pth), 0o700); err != nil {
				t.Fatalf("mkdir %s: %v", pth, err)
			}
			if err := os.WriteFile(pth, []byte("test"), 0o600); err != nil {
				t.Fatalf("write %s: %v", pth, err)
			}
		}
	}

	var (
		mu             sync.Mutex
		deleteFailMode bool
		deletedIDs     []string
	)
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/DeleteCascadeTrajectory") {
			var req struct {
				CascadeID string `json:"cascadeId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			fail := deleteFailMode
			if !fail {
				deletedIDs = append(deletedIDs, req.CascadeID)
			}
			mu.Unlock()
			if fail {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error":"upstream busy"}`))
				return
			}
			w.Write([]byte(`{}`))
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	p := NewProxy(&inspector.Inspector{})
	p.SetTestUpstream(extractTestPort(t, srv.URL), "tok")
	p.NotifyAccountSwitched("acc-b")
	defaultStitchStore.appendSegment(rootID, rootID, childID, "acc-b")

	// Case A: Upstream delete fails (500) -> stitch chain and local files MUST remain intact, tombstones cleared.
	mu.Lock()
	deleteFailMode = true
	mu.Unlock()
	delBody, _ := json.Marshal(map[string]string{"cascadeId": rootID})
	recFail := httptest.NewRecorder()
	p.ServeHTTP(recFail, httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/DeleteCascadeTrajectory", bytes.NewReader(delBody)))
	if recFail.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on failed delete, got %d", recFail.Code)
	}
	if segs := GetStitchChainSegmentIDs(rootID); len(segs) != 2 {
		t.Fatalf("expected stitch chain to be preserved after failed upstream delete, got %v", segs)
	}
	if IsDeletedCascade(rootID) || IsDeletedCascade(childID) {
		t.Fatalf("expected tombstones to be released after failed delete")
	}
	if _, err := os.Stat(filepath.Join(agRoot, "conversations", childID+".db")); err != nil {
		t.Fatalf("expected child .db to remain on disk after failed delete: %v", err)
	}

	// Case B: Upstream delete succeeds (200) -> chain removed, child deleted upstream, all local files purged.
	mu.Lock()
	deleteFailMode = false
	mu.Unlock()
	recOK := httptest.NewRecorder()
	p.ServeHTTP(recOK, httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/DeleteCascadeTrajectory", bytes.NewReader(delBody)))
	if recOK.Code != http.StatusOK {
		t.Fatalf("expected 200 on successful delete, got %d: %s", recOK.Code, recOK.Body.String())
	}
	if segs := GetStitchChainSegmentIDs(rootID); len(segs) != 0 {
		t.Fatalf("expected stitch chain to be removed after successful delete, got %v", segs)
	}
	mu.Lock()
	gotDeleted := append([]string(nil), deletedIDs...)
	mu.Unlock()
	joinedDeleted := strings.Join(gotDeleted, ",")
	if !strings.Contains(joinedDeleted, rootID) || !strings.Contains(joinedDeleted, childID) {
		t.Fatalf("expected upstream DeleteCascadeTrajectory for both root and child, got %v", gotDeleted)
	}
	for _, cid := range []string{rootID, childID} {
		for _, pth := range []string{
			filepath.Join(agRoot, "conversations", cid+".db"),
			filepath.Join(agRoot, "conversations", cid+".db-wal"),
			filepath.Join(agRoot, "annotations", cid+".pbtxt"),
			filepath.Join(agRoot, "brain", cid),
		} {
			if _, err := os.Stat(pth); !os.IsNotExist(err) {
				t.Fatalf("expected local path %s to be cleaned up after delete, err=%v", pth, err)
			}
		}
	}
}

func TestCascadeStitch_ReloadPersistedSegmentEpochStillTriggersRollover(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MGY_STITCH_STATE_PATH", filepath.Join(tmpDir, "stitches.json"))
	t.Setenv("MGY_SWITCH_COMPACT_HANDOVER", "true")
	ResetCascadeStitchStoreForTest()
	t.Cleanup(ResetCascadeStitchStoreForTest)

	const (
		rootID  = "99999999-9999-9999-9999-999999999999"
		childID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	)

	p := NewProxy(&inspector.Inspector{})
	// Epoch 1: switch to acc-b and roll over to childID (persisted with Epoch=1)
	p.NotifyAccountSwitched("acc-b")
	defaultStitchStore.appendSegment(rootID, rootID, childID, "acc-b")
	// Epoch 2: switch back to acc-a, then simulate a gateway restart BEFORE user sends next message
	p.NotifyAccountSwitched("acc-a")

	defaultStitchStore.mu.Lock()
	defaultStitchStore.loaded = false
	defaultStitchStore.switchEpoch = 0
	defaultStitchStore.lastAccount = ""
	defaultStitchStore.chainsByRoot = make(map[string]*CascadeStitchChain)
	defaultStitchStore.rootByChild = make(map[string]string)
	defaultStitchStore.cascadeEpoch = make(map[string]uint64)
	defaultStitchStore.mu.Unlock()

	needsRollover, prevActive, account := defaultStitchStore.needsHandoverRollover(rootID)
	if !needsRollover {
		t.Fatalf("expected needsHandoverRollover=true after reload when switchEpoch(2) > child.Epoch(1)")
	}
	if prevActive != childID || account != "acc-a" {
		t.Fatalf("unexpected rollover state after reload: prevActive=%s account=%s", prevActive, account)
	}
}

func TestCascadeStitch_DiskSealedSnapshotAndSyncSkipsSealedSegments(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("MGY_STITCH_STATE_PATH", filepath.Join(tmpHome, ".multigravity", "stitches.json"))
	t.Setenv("MGY_SWITCH_COMPACT_HANDOVER", "true")
	ResetCascadeStitchStoreForTest()
	t.Cleanup(ResetCascadeStitchStoreForTest)
	ResetHistoricalSyncState()
	t.Cleanup(ResetHistoricalSyncState)

	const (
		rootID  = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		childID = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	)
	ClearStitchedSegmentCaches(rootID, []string{rootID, childID})
	t.Cleanup(func() {
		RemoveDeletedCascadeTombstone(rootID)
		RemoveDeletedCascadeTombstone(childID)
		ClearStitchedSegmentCaches(rootID, []string{rootID, childID})
	})

	// Create dummy >48KB .db files for both rootID (sealed) and childID (active leaf)
	convDir := filepath.Join(tmpHome, ".gemini", "antigravity-cli", "conversations")
	if err := os.MkdirAll(convDir, 0o700); err != nil {
		t.Fatalf("mkdir convDir: %v", err)
	}
	dummyDB := bytes.Repeat([]byte("x"), 60000)
	for _, cid := range []string{rootID, childID} {
		if err := os.WriteFile(filepath.Join(convDir, cid+".db"), dummyDB, 0o600); err != nil {
			t.Fatalf("write %s.db: %v", cid, err)
		}
	}

	var (
		mu              sync.Mutex
		port2RootProbes int
		loadedIDs       []string
	)
	srv1 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") {
			var req struct {
				CascadeID string `json:"cascadeId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			txt := "sealed disk snapshot turn"
			if req.CascadeID == childID {
				txt = "active child turn"
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "CASCADE_RUN_STATUS_IDLE",
				"trajectory": map[string]interface{}{
					"cascadeId": req.CascadeID,
					"steps": []map[string]interface{}{
						{
							"type":      "CORTEX_STEP_TYPE_USER_INPUT",
							"status":    "CORTEX_STEP_STATUS_DONE",
							"userInput": map[string]interface{}{"userResponse": txt},
						},
					},
				},
			})
			return
		}
		w.Write([]byte(`{}`))
	}))
	defer srv1.Close()

	srv2 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/LoadTrajectory"):
			var req struct {
				CascadeID string `json:"cascadeId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			mu.Lock()
			loadedIDs = append(loadedIDs, req.CascadeID)
			if req.CascadeID == rootID {
				port2RootProbes++
			}
			mu.Unlock()
			w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			var req struct {
				CascadeID string `json:"cascadeId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.CascadeID == rootID {
				mu.Lock()
				port2RootProbes++
				mu.Unlock()
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "CASCADE_RUN_STATUS_IDLE",
				"trajectory": map[string]interface{}{
					"cascadeId": childID,
					"steps": []map[string]interface{}{
						{
							"type":      "CORTEX_STEP_TYPE_USER_INPUT",
							"status":    "CORTEX_STEP_STATUS_DONE",
							"userInput": map[string]interface{}{"userResponse": "active child turn on port 2"},
						},
					},
				},
			})
		default:
			w.Write([]byte(`{}`))
		}
	}))
	defer srv2.Close()

	p := NewProxy(&inspector.Inspector{})
	p.SetTestUpstream(extractTestPort(t, srv1.URL), "tok1")
	p.NotifyAccountSwitched("acc-b")
	defaultStitchStore.appendSegment(rootID, rootID, childID, "acc-b")

	// 1. First fetch persists sealed rootID snapshot to ~/.multigravity/stitch_cache/<rootID>.json.gz
	rec1 := httptest.NewRecorder()
	p.ServeHTTP(rec1, httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId="+rootID+"&limit=50", nil))
	if rec1.Code != http.StatusOK {
		t.Fatalf("first fetch failed: %d %s", rec1.Code, rec1.Body.String())
	}
	diskCachePath := filepath.Join(tmpHome, ".multigravity", "stitch_cache", rootID+".json.gz")
	if _, err := os.Stat(diskCachePath); err != nil {
		t.Fatalf("expected sealed disk snapshot at %s, got err=%v", diskCachePath, err)
	}

	// 2. Simulate full gateway restart (purge all in-memory trajectory caches) + switch to port 2
	ClearStitchedSegmentCaches(rootID, []string{rootID, childID})
	port2 := extractTestPort(t, srv2.URL)
	p.SetTestUpstream(port2, "tok2")

	// 2a. SyncHistoricalTrajectories on port 2 must skip sealed rootID and only LoadTrajectory childID
	if err := p.SyncHistoricalTrajectories(port2, "tok2"); err != nil {
		t.Fatalf("SyncHistoricalTrajectories failed: %v", err)
	}
	mu.Lock()
	gotLoaded := append([]string(nil), loadedIDs...)
	mu.Unlock()
	for _, cid := range gotLoaded {
		if cid == rootID {
			t.Fatalf("SyncHistoricalTrajectories must NOT call LoadTrajectory on sealed segment %s, got %v", rootID, gotLoaded)
		}
	}

	// 2b. Fetching messages on port 2 after in-memory cache wipe must load rootID from disk with ZERO upstream probes
	rec2 := httptest.NewRecorder()
	p.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId="+rootID+"&limit=50", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("post-restart fetch failed: %d %s", rec2.Code, rec2.Body.String())
	}
	mu.Lock()
	probes := port2RootProbes
	mu.Unlock()
	if probes != 0 {
		t.Fatalf("expected 0 upstream GetCascadeTrajectory/LoadTrajectory probes for sealed rootID after restart, got %d", probes)
	}
	if !strings.Contains(rec2.Body.String(), "sealed disk snapshot turn") || !strings.Contains(rec2.Body.String(), "active child turn on port 2") {
		t.Fatalf("expected merged disk root + live child content, got: %s", rec2.Body.String())
	}

	// 3. Deleting the stitched conversation must also remove the sealed disk snapshot
	delBody, _ := json.Marshal(map[string]string{"cascadeId": rootID})
	recDel := httptest.NewRecorder()
	p.ServeHTTP(recDel, httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/DeleteCascadeTrajectory", bytes.NewReader(delBody)))
	if recDel.Code != http.StatusOK {
		t.Fatalf("delete failed: %d", recDel.Code)
	}
	if _, err := os.Stat(diskCachePath); !os.IsNotExist(err) {
		t.Fatalf("expected sealed disk snapshot %s to be deleted on conversation delete, err=%v", diskCachePath, err)
	}
}
