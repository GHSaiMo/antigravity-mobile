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
