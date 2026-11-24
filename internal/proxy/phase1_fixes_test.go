package proxy

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
)

func newMockTLSProxy(t *testing.T, handler http.HandlerFunc) (*Proxy, *httptest.Server) {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	p := NewProxy(inspector.NewInspector(10 * time.Second))
	tr := srv.Client().Transport.(*http.Transport)
	p.transport = tr
	p.shortClient = &http.Client{Transport: tr, Timeout: 5 * time.Second}
	p.mediumClient = &http.Client{Transport: tr, Timeout: 5 * time.Second}
	p.updateUpstream(inspector.InstanceInfo{
		PID:       4321,
		Port:      port,
		CSRFToken: "test-csrf",
		IsHealthy: true,
	})
	return p, srv
}

func TestHandleCascadeInteraction_FilePermissionMcpAndElicitation(t *testing.T) {
	var captured []map[string]interface{}

	p, _ := newMockTLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"status": "CASCADE_RUN_STATUS_RUNNING",
				"trajectory": {
					"cascadeId": "c-elicit",
					"trajectoryId": "t-elicit",
					"steps": [{
						"type": "CORTEX_STEP_TYPE_MCP_TOOL",
						"status": "CORTEX_STEP_STATUS_WAITING",
						"requestedInteraction": {
							"elicitation": {
								"serverName": "github",
								"message": "Provide PR settings",
								"requestedSchemaJson": "{\"properties\":{\"draft\":{\"type\":\"boolean\"},\"title\":{\"type\":\"string\"}},\"required\":[\"title\",\"draft\"]}"
							}
						}
					}]
				}
			}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/HandleCascadeUserInteraction") {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			captured = append(captured, body)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		http.NotFound(w, r)
	})

	// 1. file_permission -> must emit interaction.filePermission (NOT interaction.permission)
	fpBody := `{"cascadeId":"c-fp","trajectoryId":"t-fp","stepIndex":3,"type":"file_permission","allow":true,"scope":2,"target":"file:///etc/hosts"}`
	rec := httptest.NewRecorder()
	p.HandleCascadeInteraction(rec, httptest.NewRequest(http.MethodPost, "/gateway/interaction/submit", strings.NewReader(fpBody)))
	if rec.Code != http.StatusOK || len(captured) != 1 {
		t.Fatalf("file_permission submit failed: code=%d body=%s", rec.Code, rec.Body.String())
	}
	inter0 := captured[0]["interaction"].(map[string]interface{})
	if inter0["permission"] != nil {
		t.Errorf("file_permission must not populate interaction.permission: %+v", inter0)
	}
	fpMap, ok := inter0["filePermission"].(map[string]interface{})
	if !ok || fpMap["allow"] != true || fpMap["absolutePathUri"] != "file:///etc/hosts" || fpMap["scope"] != float64(2) {
		t.Errorf("unexpected filePermission payload: %+v", inter0)
	}

	// 2. mcp -> must emit interaction.mcp = {"confirm": true}
	mcpBody := `{"cascadeId":"c-mcp","trajectoryId":"t-mcp","stepIndex":4,"type":"mcp","allow":true,"optionId":"1"}`
	rec = httptest.NewRecorder()
	p.HandleCascadeInteraction(rec, httptest.NewRequest(http.MethodPost, "/gateway/interaction/submit", strings.NewReader(mcpBody)))
	if rec.Code != http.StatusOK || len(captured) != 2 {
		t.Fatalf("mcp submit failed: code=%d", rec.Code)
	}
	inter1 := captured[1]["interaction"].(map[string]interface{})
	mcpMap, ok := inter1["mcp"].(map[string]interface{})
	if !ok || mcpMap["confirm"] != true {
		t.Errorf("unexpected mcp payload: %+v", inter1)
	}

	// 3. elicitation -> must emit interaction.elicitation = {"action": "accept", "contentJson": ...}
	elicitBody := `{
		"cascadeId":"c-elicit",
		"trajectoryId":"t-elicit",
		"stepIndex":0,
		"type":"elicitation",
		"allow":true,
		"questionResponses":[
			{"questionIndex":0,"selectedOptionIds":[],"writeInResponse":"Fix race condition"},
			{"questionIndex":1,"selectedOptionIds":["true"]}
		]
	}`
	rec = httptest.NewRecorder()
	p.HandleCascadeInteraction(rec, httptest.NewRequest(http.MethodPost, "/gateway/interaction/submit", strings.NewReader(elicitBody)))
	if rec.Code != http.StatusOK || len(captured) != 3 {
		t.Fatalf("elicitation submit failed: code=%d body=%s", rec.Code, rec.Body.String())
	}
	inter2 := captured[2]["interaction"].(map[string]interface{})
	elMap, ok := inter2["elicitation"].(map[string]interface{})
	if !ok || elMap["action"] != "accept" {
		t.Fatalf("unexpected elicitation payload: %+v", inter2)
	}
	var contentObj map[string]interface{}
	if err := json.Unmarshal([]byte(elMap["contentJson"].(string)), &contentObj); err != nil {
		t.Fatalf("invalid contentJson: %v", err)
	}
	if contentObj["title"] != "Fix race condition" || contentObj["draft"] != true {
		t.Errorf("unexpected elicitation contentJson object: %+v", contentObj)
	}
}

func TestHandleCascadeInteraction_AskQuestionMultiSelectAndOption5(t *testing.T) {
	var captured []map[string]interface{}
	p, _ := newMockTLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/HandleCascadeUserInteraction") {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			captured = append(captured, body)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		http.NotFound(w, r)
	})

	// 1. Single-question multi-select submitted via questionResponses: must preserve "5", deduplicate, filter "__write_in__", and keep writeInResponse
	multiBody := `{
		"cascadeId":"c-multi",
		"trajectoryId":"t-multi",
		"stepIndex":2,
		"type":"ask_question",
		"allow":true,
		"questionResponses":[
			{"questionIndex":0,"selectedOptionIds":["1","3","5","3","__write_in__"],"writeInResponse":"  补充第 4 项  ","skipped":false}
		]
	}`
	rec := httptest.NewRecorder()
	p.HandleCascadeInteraction(rec, httptest.NewRequest(http.MethodPost, "/gateway/cascade/interaction", strings.NewReader(multiBody)))
	if rec.Code != http.StatusOK || len(captured) != 1 {
		t.Fatalf("multi-select ask_question failed: code=%d body=%s", rec.Code, rec.Body.String())
	}
	inter0 := captured[0]["interaction"].(map[string]interface{})
	aq0 := inter0["askQuestion"].(map[string]interface{})
	if aq0["cancelled"] != false {
		t.Errorf("expected cancelled=false, got %v", aq0["cancelled"])
	}
	resps0 := aq0["responses"].([]interface{})
	if len(resps0) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps0))
	}
	r0 := resps0[0].(map[string]interface{})
	gotIDs := r0["selectedOptionIds"].([]interface{})
	wantIDs := []string{"1", "3", "5"}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("selectedOptionIds = %v, want %v", gotIDs, wantIDs)
	}
	for i, wID := range wantIDs {
		if gotIDs[i].(string) != wID {
			t.Errorf("selectedOptionIds[%d] = %q, want %q", i, gotIDs[i], wID)
		}
	}
	if r0["writeInResponse"] != "补充第 4 项" {
		t.Errorf("writeInResponse = %q, want %q", r0["writeInResponse"], "补充第 4 项")
	}

	// 2. Single-question legacy path with optionId="5": must NOT drop "5"
	singleOpt5 := `{
		"cascadeId":"c-opt5",
		"trajectoryId":"t-opt5",
		"stepIndex":3,
		"type":"ask_question",
		"optionId":"5",
		"allow":true
	}`
	rec = httptest.NewRecorder()
	p.HandleCascadeInteraction(rec, httptest.NewRequest(http.MethodPost, "/gateway/cascade/interaction", strings.NewReader(singleOpt5)))
	if rec.Code != http.StatusOK || len(captured) != 2 {
		t.Fatalf("single ask_question opt 5 failed: code=%d", rec.Code)
	}
	inter1 := captured[1]["interaction"].(map[string]interface{})
	aq1 := inter1["askQuestion"].(map[string]interface{})
	r1 := aq1["responses"].([]interface{})[0].(map[string]interface{})
	gotIDs1 := r1["selectedOptionIds"].([]interface{})
	if len(gotIDs1) != 1 || gotIDs1[0].(string) != "5" {
		t.Errorf("expected optionId '5' preserved, got %v", gotIDs1)
	}
}

func TestHandleCascadeTaskStop_DoesNotCancelStepZeroOnMiss(t *testing.T) {
	var cancelCalls atomic.Int32
	p, _ := newMockTLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"CASCADE_RUN_STATUS_IDLE","trajectory":{"cascadeId":"c-idle","steps":[{"type":"CORTEX_STEP_TYPE_USER_INPUT","status":"CORTEX_STEP_STATUS_DONE","userInput":{"userResponse":"hi"}}]}}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/CancelCascadeSteps") {
			cancelCalls.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
			return
		}
		http.NotFound(w, r)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/gateway/cascade/task/stop", strings.NewReader(`{"cascadeId":"c-idle","taskId":"nonexistent-task"}`))
	p.handleCascadeTaskStop(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when no running task matches, got %d: %s", rec.Code, rec.Body.String())
	}
	if cancelCalls.Load() != 0 {
		t.Fatalf("CancelCascadeSteps must NOT be invoked when task lookup misses (called %d times)", cancelCalls.Load())
	}
}

func TestDesktopGetCascadeTrajectoryRPC_PreservesUnknownProtoFields(t *testing.T) {
	ClearTrajectoryCache("c-desktop-full")
	p, _ := newMockTLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"CASCADE_RUN_STATUS_IDLE","futureDesktopField":{"nested":42},"trajectory":{"cascadeId":"c-desktop-full","chapters":[{"title":"Part 1"}],"steps":[]}}`))
			return
		}
		http.NotFound(w, r)
	})

	port, token := p.ActiveUpstream()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/exa.language_server_pb.LanguageServerService/GetCascadeTrajectory", bytes.NewBufferString(`{"cascadeId":"c-desktop-full"}`))
	p.handleGetCascadeTrajectoryRPC(rec, req, port, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	bodyStr := rec.Body.String()
	if !strings.Contains(bodyStr, `"futureDesktopField":{"nested":42}`) || !strings.Contains(bodyStr, `"chapters":[{"title":"Part 1"}]`) {
		t.Errorf("desktop GetCascadeTrajectory stripped unknown fields: %s", bodyStr)
	}
}

func TestContextMeter_IncrementalOffsetAndLiveModelMaxTokens(t *testing.T) {
	saved := saveLiveModelsSnapshot()
	defer restoreLiveModelsSnapshot(saved)

	opts, byID, byEnum := buildModelTables(map[string]upstreamModel{
		"gemini-4-carbon-high": {
			DisplayName: "Gemini 4 Carbon (High)",
			Model:       "MODEL_PLACEHOLDER_M420",
			Recommended: true,
			MaxTokens:   1048576,
		},
	}, nil)
	liveModels.set(opts, byID, byEnum, "gemini-4-carbon-high")

	contextMeterCacheMu.Lock()
	delete(contextMeterCache, "c-meter-inc")
	contextMeterCacheMu.Unlock()

	var observedOffsets []int
	totalItems := 10

	p, _ := newMockTLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectoryGeneratorMetadata") {
			var req struct {
				Offset int `json:"generatorMetadataOffset"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			observedOffsets = append(observedOffsets, req.Offset)

			var items []map[string]interface{}
			for i := req.Offset; i < totalItems; i++ {
				items = append(items, map[string]interface{}{
					"chatModel": map[string]interface{}{
						"model": "MODEL_PLACEHOLDER_M420",
						"usage": map[string]interface{}{
							"inputTokens":  (i + 1) * 10000,
							"outputTokens": 500,
						},
					},
				})
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"generatorMetadata": items})
			return
		}
		http.NotFound(w, r)
	})

	port, token := p.ActiveUpstream()
	m1 := p.fetchUpstreamGeneratorMetadata("c-meter-inc", port, token)
	if m1 == nil || m1.MaxTokens != 1048576 {
		t.Fatalf("expected live MaxTokens 1048576, got %+v", m1)
	}

	// Expire cache entry and grow upstream metadata to 12 items
	contextMeterCacheMu.Lock()
	e := contextMeterCache["c-meter-inc"]
	e.at = time.Now().Add(-2 * time.Second)
	contextMeterCache["c-meter-inc"] = e
	contextMeterCacheMu.Unlock()
	totalItems = 12

	m2 := p.fetchUpstreamGeneratorMetadata("c-meter-inc", port, token)
	if m2 == nil || m2.InputTokens != 120000 {
		t.Fatalf("expected latest item inputTokens 120000, got %+v", m2)
	}
	if len(observedOffsets) != 2 || observedOffsets[0] != 0 || observedOffsets[1] != 5 {
		t.Errorf("expected offsets [0, 5], got %v", observedOffsets)
	}
}

type liveModelsBackup struct {
	options   []ModelOption
	byID      map[string]ModelOption
	byEnum    map[string]ModelOption
	fetchedAt time.Time
}

func saveLiveModelsSnapshot() liveModelsBackup {
	liveModels.mu.RLock()
	defer liveModels.mu.RUnlock()
	return liveModelsBackup{
		options:   liveModels.options,
		byID:      liveModels.byID,
		byEnum:    liveModels.byEnum,
		fetchedAt: liveModels.fetchedAt,
	}
}

func restoreLiveModelsSnapshot(b liveModelsBackup) {
	liveModels.mu.Lock()
	defer liveModels.mu.Unlock()
	liveModels.options = b.options
	liveModels.byID = b.byID
	liveModels.byEnum = b.byEnum
	liveModels.fetchedAt = b.fetchedAt
}
