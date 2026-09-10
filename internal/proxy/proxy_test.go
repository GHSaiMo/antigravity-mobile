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
}


