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

func TestProxyStatusAndRpc(t *testing.T) {
	insp := inspector.NewInspector(5 * time.Second)
	info := insp.Scan()
	if info == nil {
		t.Skip("Antigravity instance not available, skipping proxy test")
	}

	p := NewProxy(insp)

	// Test 1: /gateway/status
	req := httptest.NewRequest(http.MethodGet, "/gateway/status", nil)
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var status GatewayStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to decode status: %v", err)
	}
	if status.Status != "connected" {
		t.Errorf("expected status connected, got %s", status.Status)
	}

	// Test 2: /api/exa.language_server_pb.LanguageServerService/GetStatus
	rpcReq := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/GetStatus", strings.NewReader("{}"))
	rpcReq.Header.Set("Content-Type", "application/json")
	rpcRec := httptest.NewRecorder()
	p.ServeHTTP(rpcRec, rpcReq)

	if rpcRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from proxied RPC, got %d: %s", rpcRec.Code, rpcRec.Body.String())
	}
	t.Logf("Proxied GetStatus returned: %s", rpcRec.Body.String())
}

func TestGetAllCascadeTrajectoriesNeedsInput(t *testing.T) {
	// Setup mock upstream server
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories") {
			w.Write([]byte(`{
				"trajectorySummaries": {
					"traj-waiting": {
						"summary": "Needs user permission",
						"status": "CASCADE_RUN_STATUS_RUNNING",
						"stepCount": 5,
						"lastModifiedTime": "2026-09-10T00:00:00Z"
					},
					"traj-normal": {
						"summary": "Normal running task",
						"status": "CASCADE_RUN_STATUS_RUNNING",
						"stepCount": 3,
						"lastModifiedTime": "2026-09-10T00:00:00Z"
					}
				}
			}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/LoadTrajectory") {
			w.Write([]byte("{}"))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") {
			var body struct {
				CascadeID string `json:"cascadeId"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if body.CascadeID == "traj-waiting" {
				w.Write([]byte(`{
					"status": "CASCADE_RUN_STATUS_RUNNING",
					"trajectory": {
						"trajectoryId": "traj-waiting",
						"cascadeId": "traj-waiting",
						"steps": [
							{
								"type": "CORTEX_STEP_TYPE_USER_INPUT",
								"status": "CORTEX_STEP_STATUS_DONE"
							},
							{
								"type": "CORTEX_STEP_TYPE_GENERIC",
								"status": "CORTEX_STEP_STATUS_WAITING",
								"requestedInteraction": {
									"permission": {
										"resource": {
											"action": "run_command",
											"target": "npm run build"
										}
									}
								}
							}
						]
					}
				}`))
			} else {
				w.Write([]byte(`{
					"status": "CASCADE_RUN_STATUS_RUNNING",
					"trajectory": {
						"trajectoryId": "traj-normal",
						"cascadeId": "traj-normal",
						"steps": [
							{
								"type": "CORTEX_STEP_TYPE_USER_INPUT",
								"status": "CORTEX_STEP_STATUS_DONE"
							},
							{
								"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
								"status": "CORTEX_STEP_STATUS_GENERATING",
								"plannerResponse": { "response": "Thinking..." }
							}
						]
					}
				}`))
			}
			return
		}
		http.NotFound(w, r)
	}))
	defer mockUpstream.Close()

	// Parse mock port
	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port

	p := NewProxy(inspector.NewInspector(10 * time.Second))
	p.updateUpstream(inspector.InstanceInfo{
		PID:       1234,
		Port:      port,
		CSRFToken: "test-token",
		IsHealthy: true,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		TrajectorySummaries map[string]struct {
			Summary    string `json:"summary"`
			Status     string `json:"status"`
			NeedsInput *bool  `json:"needsInput"`
		} `json:"trajectorySummaries"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	waiting, ok := resp.TrajectorySummaries["traj-waiting"]
	if !ok {
		t.Fatalf("missing traj-waiting in response")
	}
	if waiting.NeedsInput == nil || !*waiting.NeedsInput {
		t.Errorf("expected traj-waiting to have needsInput=true, got %+v", waiting.NeedsInput)
	}

	normal, ok := resp.TrajectorySummaries["traj-normal"]
	if !ok {
		t.Fatalf("missing traj-normal in response")
	}
	if normal.NeedsInput != nil && *normal.NeedsInput {
		t.Errorf("expected traj-normal to NOT have needsInput=true, got %+v", normal.NeedsInput)
	}
}

func TestGetAllCascadeTrajectories_CanProceedNeedsInput(t *testing.T) {
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories") {
			w.Write([]byte(`{
				"trajectorySummaries": {
					"traj-proceed": {
						"summary": "Plan awaiting user approval",
						"status": "CASCADE_RUN_STATUS_IDLE",
						"stepCount": 10,
						"lastModifiedTime": "2026-09-09T10:00:00Z"
					},
					"traj-done": {
						"summary": "Completed session",
						"status": "CASCADE_RUN_STATUS_IDLE",
						"stepCount": 15,
						"lastModifiedTime": "2026-09-08T00:00:00Z"
					}
				}
			}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") {
			var body struct {
				CascadeID string `json:"cascadeId"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if body.CascadeID == "traj-proceed" {
				w.Write([]byte(`{
					"status": "CASCADE_RUN_STATUS_IDLE",
					"trajectory": {
						"trajectoryId": "traj-proceed",
						"cascadeId": "traj-proceed",
						"steps": [
							{
								"type": "CORTEX_STEP_TYPE_USER_INPUT",
								"status": "CORTEX_STEP_STATUS_DONE"
							},
							{
								"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
								"status": "CORTEX_STEP_STATUS_DONE"
							},
							{
								"type": "CORTEX_STEP_TYPE_CODE_ACTION",
								"status": "CORTEX_STEP_STATUS_DONE",
								"codeAction": {
									"isArtifactFile": true,
									"artifactMetadata": {
										"summary": "Plan",
										"requestFeedback": true,
										"userFacing": true
									},
									"actionResult": {
										"edit": {
											"absoluteUri": "file:///path/to/implementation_plan.md"
										}
									}
								}
							}
						]
					}
				}`))
			} else {
				w.Write([]byte(`{
					"status": "CASCADE_RUN_STATUS_IDLE",
					"trajectory": {
						"trajectoryId": "traj-done",
						"cascadeId": "traj-done",
						"steps": [
							{
								"type": "CORTEX_STEP_TYPE_USER_INPUT",
								"status": "CORTEX_STEP_STATUS_DONE"
							},
							{
								"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
								"status": "CORTEX_STEP_STATUS_DONE"
							}
						]
					}
				}`))
			}
			return
		}
		http.NotFound(w, r)
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	p := NewProxy(inspector.NewInspector(10 * time.Second))
	p.updateUpstream(inspector.InstanceInfo{
		PID:       1234,
		Port:      port,
		CSRFToken: "test-token",
		IsHealthy: true,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		TrajectorySummaries map[string]struct {
			Summary    string `json:"summary"`
			Status     string `json:"status"`
			NeedsInput *bool  `json:"needsInput"`
		} `json:"trajectorySummaries"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	proceedItem, ok := resp.TrajectorySummaries["traj-proceed"]
	if !ok {
		t.Fatalf("missing traj-proceed in response")
	}
	if proceedItem.NeedsInput == nil || !*proceedItem.NeedsInput {
		t.Errorf("expected traj-proceed to have needsInput=true, got %+v", proceedItem.NeedsInput)
	}

	doneItem, ok := resp.TrajectorySummaries["traj-done"]
	if !ok {
		t.Fatalf("missing traj-done in response")
	}
	if doneItem.NeedsInput != nil && *doneItem.NeedsInput {
		t.Errorf("expected traj-done to NOT have needsInput=true, got %+v", doneItem.NeedsInput)
	}
}

func TestFilterSubagentTrajectories(t *testing.T) {
	// 1. Direct unit test of isSubagentTrajectoryMap
	tests := []struct {
		name     string
		id       string
		summary  map[string]interface{}
		expected bool
	}{
		{
			name: "Normal user session with matching root ID",
			id:   "user-session-1",
			summary: map[string]interface{}{
				"summary": "Optimize mobile UI",
				"trajectoryMetadata": map[string]interface{}{
					"rootConversationId": "user-session-1",
					"workspaceUris":      []interface{}{"file:///Users/hal9000/Projects/app"},
				},
			},
			expected: false,
		},
		{
			name: "Legitimate user session with pure English title (Code Review and Architecture)",
			id:   "user-session-en-1",
			summary: map[string]interface{}{
				"summary": "Code Review and Architecture Analysis",
				"trajectoryMetadata": map[string]interface{}{
					"rootConversationId": "user-session-en-1",
					"workspaceUris":      []interface{}{"file:///Users/hal9000/Projects/app"},
				},
			},
			expected: false,
		},
		{
			name: "Legitimate user session with pure English title (Fix crash bug)",
			id:   "user-session-en-2",
			summary: map[string]interface{}{
				"summary": "Fix auth token refresh crash bug",
				"trajectoryMetadata": map[string]interface{}{
					"rootConversationId": "user-session-en-2",
					"workspaceUris":      []interface{}{"file:///Users/hal9000/Projects/app"},
				},
			},
			expected: false,
		},
		{
			name: "Subagent with parentConversationId",
			id:   "sub-1",
			summary: map[string]interface{}{
				"summary": "Application Log File Analysis",
				"trajectoryMetadata": map[string]interface{}{
					"parentConversationId": "user-session-1",
					"rootConversationId":   "user-session-1",
				},
			},
			expected: true,
		},
		{
			name: "Subagent with subagentSpec",
			id:   "sub-2",
			summary: map[string]interface{}{
				"summary": "Frontend Codebase Analysis",
				"trajectoryMetadata": map[string]interface{}{
					"subagentSpec": map[string]interface{}{
						"role":     "Frontend Code Researcher",
						"typeName": "research",
					},
				},
			},
			expected: true,
		},
		{
			name: "Subagent with agentScript",
			id:   "sub-3",
			summary: map[string]interface{}{
				"summary": "Python Backend Code Review",
				"trajectoryMetadata": map[string]interface{}{
					"agentScript": map[string]interface{}{
						"name": "research",
					},
				},
			},
			expected: true,
		},
		{
			name: "Subagent with nestingDepth > 0",
			id:   "sub-4",
			summary: map[string]interface{}{
				"summary": "Runtime LLM Fallback Mechanisms",
				"trajectoryMetadata": map[string]interface{}{
					"nestingDepth": float64(1),
				},
			},
			expected: true,
		},
		{
			name: "Battle mode fork session",
			id:   "fork-1",
			summary: map[string]interface{}{
				"summary": "Battle mode fork candidate",
				"trajectoryMetadata": map[string]interface{}{
					"isBattleModeFork": true,
				},
			},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isSubagentTrajectoryMap(tc.summary, tc.id)
			if got != tc.expected {
				t.Errorf("%s: expected isSubagent=%v, got %v", tc.name, tc.expected, got)
			}
		})
	}

	// 2. Integration test through proxy /api/.../GetAllCascadeTrajectories
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories") {
			w.Write([]byte(`{
				"trajectorySummaries": {
					"real-user-session": {
						"summary": "真实用户发起的会话",
						"status": "CASCADE_RUN_STATUS_IDLE",
						"stepCount": 20,
						"lastModifiedTime": "2026-09-10T00:00:00Z",
						"trajectoryMetadata": {
							"rootConversationId": "real-user-session",
							"workspaceUris": ["file:///path/to/project"]
						}
					},
					"subagent-with-parent": {
						"summary": "Application Log File Analysis",
						"status": "CASCADE_RUN_STATUS_IDLE",
						"stepCount": 10,
						"lastModifiedTime": "2026-09-10T00:00:00Z",
						"trajectoryMetadata": {
							"parentConversationId": "real-user-session",
							"rootConversationId": "real-user-session",
							"nestingDepth": 1
						}
					},
					"subagent-with-spec": {
						"summary": "Frontend Codebase Analysis",
						"status": "CASCADE_RUN_STATUS_IDLE",
						"stepCount": 15,
						"lastModifiedTime": "2026-09-10T00:00:00Z",
						"trajectoryMetadata": {
							"subagentSpec": {
								"role": "Frontend Code Researcher"
							}
						}
					}
				}
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	p := NewProxy(inspector.NewInspector(10 * time.Second))
	p.updateUpstream(inspector.InstanceInfo{
		PID:       1234,
		Port:      port,
		CSRFToken: "test-token",
		IsHealthy: true,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		TrajectorySummaries map[string]interface{} `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json: %v", err)
	}

	if _, ok := resp.TrajectorySummaries["real-user-session"]; !ok {
		t.Errorf("expected real-user-session to be present in summaries")
	}
	if _, ok := resp.TrajectorySummaries["subagent-with-parent"]; ok {
		t.Errorf("expected subagent-with-parent to be filtered out")
	}
	if _, ok := resp.TrajectorySummaries["subagent-with-spec"]; ok {
		t.Errorf("expected subagent-with-spec to be filtered out")
	}
}

func TestSendUserCascadeMessageDeduplication(t *testing.T) {
	upstreamCallCount := 0
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/SendUserCascadeMessage") {
			upstreamCallCount++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{}"))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer mockUpstream.Close()

	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	p.updateUpstream(inspector.InstanceInfo{
		Port:      port,
		CSRFToken: "test-token",
		IsHealthy: true,
	})

	msgPayload := `{"cascadeId":"cascade-dup-test","items":[{"text":"Deploy the fix"}]}`

	// 1. First send -> should reach upstream
	req1 := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", strings.NewReader(msgPayload))
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	p.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("first request expected 200, got %d", rec1.Code)
	}
	if upstreamCallCount != 1 {
		t.Fatalf("expected upstreamCallCount to be 1, got %d", upstreamCallCount)
	}

	// 2. Immediate second send with same content -> should be deduplicated by gateway
	req2 := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", strings.NewReader(msgPayload))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	p.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("second duplicate request expected 200, got %d", rec2.Code)
	}
	if rec2.Body.String() != "{}" {
		t.Fatalf("expected response body '{}', got %q", rec2.Body.String())
	}
	if upstreamCallCount != 1 {
		t.Fatalf("expected upstreamCallCount to remain 1 (deduplicated), but got %d", upstreamCallCount)
	}

	// 3. Different content -> should pass through and reach upstream
	diffPayload := `{"cascadeId":"cascade-dup-test","items":[{"text":"Different instruction"}]}`
	req3 := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", strings.NewReader(diffPayload))
	req3.Header.Set("Content-Type", "application/json")
	rec3 := httptest.NewRecorder()
	p.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusOK {
		t.Fatalf("third request expected 200, got %d", rec3.Code)
	}
	if upstreamCallCount != 2 {
		t.Fatalf("expected upstreamCallCount to be 2 for different text, got %d", upstreamCallCount)
	}

	// 4. ClientMessageId header deduplication test
	clientMsgID := "msg-unique-uuid-123"
	req4 := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", strings.NewReader(`{"cascadeId":"cascade-dup-test","items":[{"text":"Command with header"}]}`))
	req4.Header.Set("Content-Type", "application/json")
	req4.Header.Set("X-Client-Message-Id", clientMsgID)
	rec4 := httptest.NewRecorder()
	p.ServeHTTP(rec4, req4)

	if rec4.Code != http.StatusOK {
		t.Fatalf("fourth request expected 200, got %d", rec4.Code)
	}
	if upstreamCallCount != 3 {
		t.Fatalf("expected upstreamCallCount to be 3, got %d", upstreamCallCount)
	}

	// 5. Repeat with same X-Client-Message-Id -> must be deduplicated
	req5 := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", strings.NewReader(`{"cascadeId":"cascade-dup-test","items":[{"text":"Command with header"}]}`))
	req5.Header.Set("Content-Type", "application/json")
	req5.Header.Set("X-Client-Message-Id", clientMsgID)
	rec5 := httptest.NewRecorder()
	p.ServeHTTP(rec5, req5)

	if rec5.Code != http.StatusOK {
		t.Fatalf("fifth duplicate request expected 200, got %d", rec5.Code)
	}
	if rec5.Body.String() != "{}" {
		t.Fatalf("expected response body '{}', got %q", rec5.Body.String())
	}
	if upstreamCallCount != 3 {
		t.Fatalf("expected upstreamCallCount to remain 3 (deduplicated by clientMsgID), got %d", upstreamCallCount)
	}
}

func TestDeleteCascadeTrajectoryProxy(t *testing.T) {
	var receivedCascadeID string
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/DeleteCascadeTrajectory") {
			var body struct {
				CascadeID string `json:"cascadeId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			receivedCascadeID = body.CascadeID
			w.Write([]byte("{}"))
			return
		}
		http.NotFound(w, r)
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)
	p.updateUpstream(inspector.InstanceInfo{
		PID:       1234,
		Port:      port,
		CSRFToken: "test-token",
		IsHealthy: true,
	})

	deleteReq := `{"cascadeId":"cascade-delete-test-id"}`
	req := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/DeleteCascadeTrajectory", strings.NewReader(deleteReq))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if receivedCascadeID != "cascade-delete-test-id" {
		t.Fatalf("expected upstream to receive cascadeId 'cascade-delete-test-id', got %q", receivedCascadeID)
	}
	if !IsDeletedCascade("cascade-delete-test-id") {
		t.Fatalf("expected 'cascade-delete-test-id' to be marked as deleted tombstone")
	}
}

func TestDeleteCascadeTrajectoryTombstonePreventsReflow(t *testing.T) {
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/DeleteCascadeTrajectory") {
			w.Write([]byte("{}"))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories") {
			// Simulate upstream still returning the deleted session due to delayed purge
			resp := map[string]interface{}{
				"trajectorySummaries": map[string]interface{}{
					"session-to-delete": map[string]interface{}{
						"status":    "CASCADE_RUN_STATUS_DONE",
						"stepCount": 5,
						"summary":   "Deleted conversation",
					},
					"session-alive": map[string]interface{}{
						"status":    "CASCADE_RUN_STATUS_DONE",
						"stepCount": 3,
						"summary":   "Active conversation",
					},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)
	p.updateUpstream(inspector.InstanceInfo{
		PID:       1234,
		Port:      port,
		CSRFToken: "test-token",
		IsHealthy: true,
	})

	// 1. Delete session-to-delete
	deleteReq := `{"cascadeId":"session-to-delete"}`
	delReq := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/DeleteCascadeTrajectory", strings.NewReader(deleteReq))
	delReq.Header.Set("Content-Type", "application/json")
	delRec := httptest.NewRecorder()
	p.ServeHTTP(delRec, delReq)

	if delRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", delRec.Code, delRec.Body.String())
	}

	// 2. Query GetAllCascadeTrajectories - session-to-delete must be intercepted by tombstone
	listReq := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", strings.NewReader("{}"))
	listReq.Header.Set("Content-Type", "application/json")
	listRec := httptest.NewRecorder()
	p.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRec.Code, listRec.Body.String())
	}

	var listResp struct {
		TrajectorySummaries map[string]interface{} `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, exists := listResp.TrajectorySummaries["session-to-delete"]; exists {
		t.Fatalf("expected 'session-to-delete' to be filtered out by tombstone, but it was returned!")
	}
	if _, exists := listResp.TrajectorySummaries["session-alive"]; !exists {
		t.Fatalf("expected 'session-alive' to be present in response")
	}
}

func TestSendUserCascadeMessageModelSwitching(t *testing.T) {
	var forwardedPayload map[string]interface{}
	upstreamServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/SendUserCascadeMessage") {
			json.NewDecoder(r.Body).Decode(&forwardedPayload)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer upstreamServer.Close()

	port := upstreamServer.Listener.Addr().(*net.TCPAddr).Port
	insp := inspector.NewInspector(5 * time.Second)
	p := NewProxy(insp)
	p.updateUpstream(inspector.InstanceInfo{
		PID:       1234,
		Port:      port,
		CSRFToken: "test-token",
		IsHealthy: true,
	})

	// Case 1: Switching via X-Antigravity-Model header
	req1 := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage",
		strings.NewReader(`{"cascadeId":"cascade-switch-test","items":[{"text":"Run with Claude"}],"cascadeConfig":{"plannerConfig":{"planModel":"MODEL_PLACEHOLDER_M318","modelName":"gemini-3.8-flash-high"}}}`))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Antigravity-Model", "claude-opus-4-6-thinking")
	rec1 := httptest.NewRecorder()
	p.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Fatalf("Case 1 expected 200, got %d: %s", rec1.Code, rec1.Body.String())
	}

	cfg1, ok := forwardedPayload["cascadeConfig"].(map[string]interface{})
	if !ok {
		t.Fatalf("Case 1 expected cascadeConfig in forwarded payload")
	}
	pCfg1 := cfg1["plannerConfig"].(map[string]interface{})
	if pCfg1["planModel"] != "MODEL_PLACEHOLDER_M26" {
		t.Errorf("Case 1 expected planModel MODEL_PLACEHOLDER_M26, got %v", pCfg1["planModel"])
	}
	if pCfg1["modelName"] != "claude-opus-4-6-thinking" {
		t.Errorf("Case 1 expected modelName claude-opus-4-6-thinking, got %v", pCfg1["modelName"])
	}

	// Case 2: Switching via body field "model"
	forwardedPayload = nil
	req2 := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage",
		strings.NewReader(`{"cascadeId":"cascade-switch-test","model":"claude","items":[{"text":"Run with Claude again"}],"cascadeConfig":{"plannerConfig":{"planModel":"MODEL_PLACEHOLDER_M318","modelName":"gemini-3.8-flash-high"}}}`))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	p.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("Case 2 expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}

	// Verify "model" was stripped from forwarded payload
	if _, hasModel := forwardedPayload["model"]; hasModel {
		t.Errorf("Case 2 expected 'model' field to be stripped from forwarded payload")
	}

	cfg2, ok := forwardedPayload["cascadeConfig"].(map[string]interface{})
	if !ok {
		t.Fatalf("Case 2 expected cascadeConfig in forwarded payload")
	}
	pCfg2 := cfg2["plannerConfig"].(map[string]interface{})
	if pCfg2["planModel"] != "MODEL_PLACEHOLDER_M26" {
		t.Errorf("Case 2 expected planModel MODEL_PLACEHOLDER_M26, got %v", pCfg2["planModel"])
	}
}

func TestGetAllCascadeTrajectoriesErrorStatus(t *testing.T) {
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories") {
			w.Write([]byte(`{
				"trajectorySummaries": {
					"traj-with-error": {
						"summary": "Errored session",
						"status": "CASCADE_RUN_STATUS_IDLE",
						"stepCount": 2,
						"lastModifiedTime": "2026-09-11T15:00:00Z"
					}
				}
			}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/LoadTrajectory") {
			w.Write([]byte("{}"))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") {
			w.Write([]byte(`{
				"status": "CASCADE_RUN_STATUS_IDLE",
				"trajectory": {
					"cascadeId": "traj-with-error",
					"steps": [
						{
							"type": "CORTEX_STEP_TYPE_USER_INPUT",
							"status": "CORTEX_STEP_STATUS_DONE",
							"userInput": {"userResponse": "test"}
						},
						{
							"type": "CORTEX_STEP_TYPE_ERROR_MESSAGE",
							"status": "CORTEX_STEP_STATUS_DONE",
							"errorMessage": {
								"error": {
									"userErrorMessage": "Agent execution terminated due to error.",
									"shortError": "checkpoint validation failed"
								}
							}
						}
					]
				}
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	p := NewProxy(inspector.NewInspector(10 * time.Second))
	p.transport = mockUpstream.Client().Transport.(*http.Transport)
	p.updateUpstream(inspector.InstanceInfo{
		PID:       1234,
		Port:      port,
		CSRFToken: "test-token",
		IsHealthy: true,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		TrajectorySummaries map[string]struct {
			Status       string `json:"status"`
			HasError     bool   `json:"hasError"`
			ErrorMessage string `json:"errorMessage"`
		} `json:"trajectorySummaries"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	sum, ok := resp.TrajectorySummaries["traj-with-error"]
	if !ok {
		t.Fatalf("expected traj-with-error in summaries")
	}
	if !sum.HasError {
		t.Errorf("expected HasError to be true, got false")
	}
	if sum.Status != "CASCADE_RUN_STATUS_ERROR" {
		t.Errorf("expected Status CASCADE_RUN_STATUS_ERROR, got %s", sum.Status)
	}
	if !strings.Contains(sum.ErrorMessage, "checkpoint validation failed") {
		t.Errorf("expected ErrorMessage to contain 'checkpoint validation failed', got %q", sum.ErrorMessage)
	}
}





