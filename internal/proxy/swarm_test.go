package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSwarmParsingAndEndpoints(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	coordID := "cascade-swarm-coord"
	swarmDir := filepath.Join(home, ".gemini", "antigravity", "brain", coordID, ".system_generated", "swarm")
	if err := os.MkdirAll(swarmDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	membersJSON := `{
		"title": "测试蜂群",
		"members": {
			"worker-1": {
				"conversation_id": "conv-w1",
				"model": "gemini-4-carbon-high",
				"role": "worker",
				"wake_mode": "mentions",
				"last_read_seq": 1,
				"killed": false
			},
			"narrator": {
				"conversation_id": "conv-narrator",
				"model": "gemini-3.8-flash-high",
				"role": "narrator",
				"wake_mode": "any",
				"last_read_seq": 2,
				"killed": false
			}
		}
	}`
	_ = os.WriteFile(filepath.Join(swarmDir, "members.json"), []byte(membersJSON), 0644)

	boardJSONL := `{"seq":1,"agent":"coordinator","text":"@worker-1 请开始任务","time":"2026-11-12T04:00:00Z"}
{"seq":2,"agent":"worker-1","text":"收到，开始分析代码","time":"2026-11-12T04:01:00Z"}`
	_ = os.WriteFile(filepath.Join(swarmDir, "board.jsonl"), []byte(boardJSONL), 0644)

	viewJSON := `{
		"version": "1.0",
		"as_of_seq": 2,
		"headline": "蜂群初始化完成",
		"phase": {"label": "执行阶段", "index": 1, "total": 3},
		"attention": [{"id": "att-1", "kind": "fyi", "severity": "low", "title": "任务正常推进中"}]
	}`
	_ = os.WriteFile(filepath.Join(swarmDir, "view.json"), []byte(viewJSON), 0644)

	// 1. Test GetSwarmBrief
	brief := GetSwarmBrief(coordID)
	if brief == nil {
		t.Fatalf("expected non-nil SwarmBrief")
	}
	if brief.Title != "测试蜂群" {
		t.Errorf("expected Title '测试蜂群', got %q", brief.Title)
	}
	if brief.TotalMembers != 2 || brief.WorkingCount != 2 {
		t.Errorf("expected 2 total and 2 working members, got %d/%d", brief.TotalMembers, brief.WorkingCount)
	}
	if brief.LatestSeq != 2 {
		t.Errorf("expected latestSeq 2, got %d", brief.LatestSeq)
	}
	if brief.Headline != "蜂群初始化完成" {
		t.Errorf("expected headline '蜂群初始化完成', got %q", brief.Headline)
	}

	// 2. Test GetSwarmBrief from worker conversation ID
	wBrief := GetSwarmBrief("conv-w1")
	if wBrief == nil || wBrief.CoordinatorID != coordID {
		t.Errorf("expected worker conv-w1 to resolve coordinator %s, got %+v", coordID, wBrief)
	}

	// 3. Test GetSwarmSnapshot
	snap, notMod, err := GetSwarmSnapshot(coordID, 0, "")
	if err != nil || notMod || snap == nil {
		t.Fatalf("expected valid snapshot, err=%v, notMod=%v", err, notMod)
	}
	if len(snap.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(snap.Messages))
	}
	if len(snap.Messages[0].Mentions) != 1 || snap.Messages[0].Mentions[0] != "worker-1" {
		t.Errorf("expected mention of worker-1, got %+v", snap.Messages[0].Mentions)
	}

	// Test 304 with matching ETag
	_, notMod2, _ := GetSwarmSnapshot(coordID, 0, snap.ETag)
	if !notMod2 {
		t.Errorf("expected 304 Not Modified with matching ETag")
	}

	// 4. Test HTTP Handler
	p := &Proxy{}
	req := httptest.NewRequest(http.MethodGet, "/gateway/swarm?cascadeId="+coordID, nil)
	w := httptest.NewRecorder()
	p.HandleGetSwarm(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected HTTP 200, got %d: %s", w.Code, w.Body.String())
	}
	var resSnap SwarmSnapshot
	if err := json.Unmarshal(w.Body.Bytes(), &resSnap); err != nil {
		t.Errorf("failed to unmarshal HTTP response: %v", err)
	}
}

func TestSwarmNumericWaitSinceMtimeSortAndTranscript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Clear cache between test runs
	swarmCoordMu.Lock()
	memberToCoordCache = make(map[string]string)
	swarmCoordMu.Unlock()

	brainBase := filepath.Join(home, ".gemini", "antigravity", "brain")

	// Create 30 older directories whose names sort AFTER "0000-coord" alphabetically
	olderTime := time.Now().Add(-2 * time.Hour)
	for i := 0; i < 30; i++ {
		d := filepath.Join(brainBase, fmt.Sprintf("ffff-old-%02d", i))
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(d, olderTime, olderTime)
	}

	coordID := "0000-coord-newest"
	workerID := "aaaa-worker-1"
	swarmDir := filepath.Join(brainBase, coordID, ".system_generated", "swarm")
	if err := os.MkdirAll(swarmDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Real Antigravity 2.31+ members.json uses integer wait_since
	membersJSON := `{
		"title": "数字 wait_since 测试",
		"members": {
			"coordinator": {
				"conversation_id": "0000-coord-newest",
				"model": "gemini-3.8-flash-high",
				"role": "coordinator",
				"wake_mode": "mentions",
				"wait_since": 6,
				"last_read_seq": 6,
				"last_nudged_seq": 6,
				"killed": false
			},
			"worker-1": {
				"conversation_id": "aaaa-worker-1",
				"model": "gemini-3.8-flash-high",
				"role": "worker",
				"wake_mode": "working",
				"wait_since": 3,
				"last_read_seq": 4,
				"killed": false
			}
		}
	}`
	_ = os.WriteFile(filepath.Join(swarmDir, "members.json"), []byte(membersJSON), 0644)
	_ = os.WriteFile(filepath.Join(swarmDir, "board.jsonl"), []byte(`{"seq":6,"agent":"coordinator","text":"@worker-1 check","time":"2026-11-13 00:05:00"}`+"\n"), 0644)

	// Coordinator transcript parked in swarm_wait; worker transcript actively calling read_file
	coordLogDir := filepath.Join(brainBase, coordID, ".system_generated", "logs")
	workerLogDir := filepath.Join(brainBase, workerID, ".system_generated", "logs")
	_ = os.MkdirAll(coordLogDir, 0755)
	_ = os.MkdirAll(workerLogDir, 0755)
	_ = os.WriteFile(filepath.Join(coordLogDir, "transcript.jsonl"), []byte(
		`{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE","tool_calls":[{"name":"swarm_wait","args":{"since":"6"}}]}`+"\n"+
			`{"step_index":2,"type":"GENERIC","status":"DONE","content":"Waiting for a message that calls on you by name"}`+"\n",
	), 0644)
	_ = os.WriteFile(filepath.Join(workerLogDir, "transcript.jsonl"), []byte(
		`{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE","tool_calls":[{"name":"read_file"}]}`+"\n",
	), 0644)

	// Worker should resolve coordinator despite 30 alphabetically larger directories
	snap, notMod, err := GetSwarmSnapshot(workerID, 0, "")
	if err != nil || notMod || snap == nil {
		t.Fatalf("expected worker to resolve swarm snapshot, err=%v notMod=%v", err, notMod)
	}
	if snap.CoordinatorID != coordID {
		t.Fatalf("expected coordinator %s, got %s", coordID, snap.CoordinatorID)
	}
	if snap.Brief.WorkingCount != 1 {
		t.Errorf("expected 1 working member (worker-1), got %d", snap.Brief.WorkingCount)
	}

	byName := map[string]SwarmMemberInfo{}
	for _, m := range snap.Members {
		byName[m.Name] = m
	}
	if byName["coordinator"].WaitSince != "6" || byName["coordinator"].Status != "waiting" {
		t.Errorf("unexpected coordinator info: %+v", byName["coordinator"])
	}
	if byName["worker-1"].WaitSince != "3" || byName["worker-1"].Status != "working" || !byName["worker-1"].IsCurrent {
		t.Errorf("unexpected worker-1 info: %+v", byName["worker-1"])
	}
	if sig := GetSwarmSignature(workerID); sig == "" {
		t.Errorf("expected non-empty swarm signature for worker")
	}
}

func TestSwarmNarratorNumericVersionFallbackAndTranscriptETag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	brainBase := filepath.Join(home, ".gemini", "antigravity", "brain")
	coordID := "coord-2321"
	narratorID := "conv-narrator-2321"
	workerID := "conv-worker-2321"
	swarmDir := filepath.Join(brainBase, coordID, ".system_generated", "swarm")
	if err := os.MkdirAll(swarmDir, 0755); err != nil {
		t.Fatal(err)
	}

	membersJSON := `{
		"title": "2.32.1 蜂群",
		"members": {
			"worker-1": {
				"conversation_id": "conv-worker-2321",
				"model": "gemini-4-carbon-high",
				"role": "worker",
				"wake_mode": "working",
				"last_read_seq": 1,
				"killed": false
			},
			"narrator": {
				"conversation_id": "conv-narrator-2321",
				"model": "gemini-3.8-flash-high",
				"role": "narrator",
				"wake_mode": "any",
				"last_read_seq": 1,
				"killed": false
			}
		}
	}`
	_ = os.WriteFile(filepath.Join(swarmDir, "members.json"), []byte(membersJSON), 0644)
	_ = os.WriteFile(filepath.Join(swarmDir, "board.jsonl"), []byte(`{"seq":1,"agent":"coordinator","text":"@worker-1 go","time":"2026-11-15T00:00:00Z"}`+"\n"), 0644)

	// Place view.json ONLY in <brainBase>/<narratorID>/swarm_view/view.json with numeric "version": 1
	narratorViewDir := filepath.Join(brainBase, narratorID, "swarm_view")
	if err := os.MkdirAll(narratorViewDir, 0755); err != nil {
		t.Fatal(err)
	}
	narratorView := `{"version":1,"as_of_seq":1,"headline":"Narrator 回退路径与数字版本解析成功","phase":{"label":"审计阶段","index":1,"total":2}}`
	_ = os.WriteFile(filepath.Join(narratorViewDir, "view.json"), []byte(narratorView), 0644)

	workerLogDir := filepath.Join(brainBase, workerID, ".system_generated", "logs")
	_ = os.MkdirAll(workerLogDir, 0755)
	workerTranscript := filepath.Join(workerLogDir, "transcript.jsonl")
	_ = os.WriteFile(workerTranscript, []byte(`{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE","tool_calls":[{"name":"grep_search"}]}`+"\n"), 0644)

	brief := GetSwarmBrief(coordID)
	if brief == nil || brief.Headline != "Narrator 回退路径与数字版本解析成功" {
		t.Fatalf("expected narrator fallback headline, got %+v", brief)
	}

	snap1, notMod, err := GetSwarmSnapshot(coordID, 0, "")
	if err != nil || notMod || snap1 == nil || snap1.View == nil {
		t.Fatalf("expected valid snapshot with narrator view, err=%v snap=%+v", err, snap1)
	}
	if string(snap1.View.Version) != "1" {
		t.Errorf("expected version '1', got %q", snap1.View.Version)
	}

	// Update ONLY worker transcript (into swarm_wait) without touching board.jsonl or members.json:
	// ETag must change so client does not get stuck on 304 Not Modified!
	_ = os.WriteFile(workerTranscript, []byte(
		`{"step_index":1,"type":"PLANNER_RESPONSE","status":"DONE","tool_calls":[{"name":"swarm_wait"}]}`+"\n"+
			`{"step_index":2,"type":"GENERIC","status":"DONE","content":"Waiting for a message"}`+"\n",
	), 0644)

	snap2, notModAfterTranscript, err := GetSwarmSnapshot(coordID, 0, snap1.ETag)
	if err != nil || notModAfterTranscript || snap2 == nil {
		t.Fatalf("expected ETag invalidation after transcript update, err=%v notMod=%v", err, notModAfterTranscript)
	}
	if snap1.ETag == snap2.ETag {
		t.Errorf("expected distinct ETags after transcript growth: %s", snap1.ETag)
	}

	// Non-swarm conversation negative cache check
	if nonSwarm := GetSwarmBrief("ordinary-non-swarm-cascade"); nonSwarm != nil {
		t.Errorf("expected nil brief for non-swarm conversation, got %+v", nonSwarm)
	}
}

func TestSwarmPlanJsonAndToolActivityLabels(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	brainBase := filepath.Join(home, ".gemini", "antigravity", "brain")
	coordID := "coord-hive-plan"
	wProxyID := "conv-proxy-hive"
	wWebID := "conv-web-hive"
	swarmDir := filepath.Join(brainBase, coordID, ".system_generated", "swarm")
	if err := os.MkdirAll(swarmDir, 0755); err != nil {
		t.Fatal(err)
	}

	membersJSON := `{
		"title": "Hive Plan Swarm",
		"members": {
			"coordinator": {"conversation_id": "coord-hive-plan", "role": "coordinator", "model": "gemini-4-carbon-high", "killed": false},
			"proxy": {"conversation_id": "conv-proxy-hive", "role": "worker", "model": "gemini-4-carbon-high", "killed": false},
			"web": {"conversation_id": "conv-web-hive", "role": "worker", "model": "gemini-4-carbon-high", "killed": false},
			"monitor": {"conversation_id": "conv-mon-hive", "role": "worker", "model": "gemini-4-carbon-high", "killed": false}
		}
	}`
	_ = os.WriteFile(filepath.Join(swarmDir, "members.json"), []byte(membersJSON), 0644)
	_ = os.WriteFile(filepath.Join(swarmDir, "board.jsonl"), []byte(
		`{"seq":1,"agent":"coordinator","text":"@proxy @web go","time":"2026-11-22 06:00:00"}`+"\n"+
			`{"seq":2,"agent":"web","text":"@coordinator [impl-ready] web done","time":"2026-11-22 06:10:00"}`+"\n",
	), 0644)

	planJSON := `{
		"goal": {"title": "Fix security and performance", "line": "Audit and verify all modules."},
		"tasks": {"proxy": "Fix proxy stream hotspots", "web": "Fix embed/XSS"},
		"threads": [{"id": "t-core", "title": "Gateway hardening", "goal": "Harden proxy.", "tasks": ["proxy", "web"]}]
	}`
	_ = os.WriteFile(filepath.Join(swarmDir, "plan.json"), []byte(planJSON), 0644)

	proxyLogDir := filepath.Join(brainBase, wProxyID, ".system_generated", "logs")
	webLogDir := filepath.Join(brainBase, wWebID, ".system_generated", "logs")
	_ = os.MkdirAll(proxyLogDir, 0755)
	_ = os.MkdirAll(webLogDir, 0755)

	// proxy is actively reading stream.go
	_ = os.WriteFile(filepath.Join(proxyLogDir, "transcript.jsonl"), []byte(
		`{"step_index":3,"type":"PLANNER_RESPONSE","status":"DONE","tool_calls":[{"name":"read_file","args":{"Path":"\"/repo/internal/proxy/stream.go\""}}]}`+"\n",
	), 0644)
	// web posted + called swarm_wait in the same multi-tool batch -> should be waiting!
	_ = os.WriteFile(filepath.Join(webLogDir, "transcript.jsonl"), []byte(
		`{"step_index":4,"type":"PLANNER_RESPONSE","status":"DONE","tool_calls":[{"name":"post_message","args":{}},{"name":"swarm_wait","args":{}}]}`+"\n"+
			`{"step_index":5,"type":"GENERIC","status":"DONE","content":"Posted #2"}`+"\n"+
			`{"step_index":6,"type":"GENERIC","status":"DONE","content":"Waiting for a message"}`+"\n",
	), 0644)

	brief := GetSwarmBrief(coordID)
	if brief == nil || brief.Headline != "Fix security and performance" {
		t.Fatalf("expected plan.goal.title fallback in SwarmBrief.Headline, got %+v", brief)
	}

	snap, notMod, err := GetSwarmSnapshot(coordID, 0, "")
	if err != nil || notMod || snap == nil || snap.Plan == nil || snap.Plan.Goal == nil {
		t.Fatalf("expected valid snapshot with Plan, err=%v snap=%+v", err, snap)
	}
	if len(snap.Plan.Threads) != 1 || snap.Plan.Threads[0].Title != "Gateway hardening" {
		t.Errorf("unexpected plan threads: %+v", snap.Plan.Threads)
	}

	byName := map[string]SwarmMemberInfo{}
	for _, m := range snap.Members {
		byName[m.Name] = m
	}
	if byName["proxy"].Status != "working" || byName["proxy"].LastActivity != "Read stream.go" {
		t.Errorf("expected proxy working on 'Read stream.go', got %+v", byName["proxy"])
	}
	if byName["web"].Status != "waiting" || byName["web"].Posts != 1 || byName["web"].LastPostSeq != 2 {
		t.Errorf("expected web waiting with 1 post (#2), got %+v", byName["web"])
	}
	if !byName["monitor"].RoleWorker {
		t.Errorf("expected monitor to be marked RoleWorker=true")
	}
}

func TestGetAllCascadeTrajectoriesPropagatesSwarmAndSubagentActivity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	brainBase := filepath.Join(home, ".gemini", "antigravity", "brain")

	// 1. Active swarm coordinator whose own status is IDLE and lastModifiedTime is 45m old,
	// while its workers (including a depth-2 nested helper) are actively running right now.
	coordID := "coord-active-swarm-1"
	workerRunningID := "worker-running-1"
	workerHelperID := "worker-helper-2"
	activeSwarmDir := filepath.Join(brainBase, coordID, ".system_generated", "swarm")
	if err := os.MkdirAll(activeSwarmDir, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(activeSwarmDir, "members.json"), []byte(`{
		"title": "Active Swarm",
		"members": {
			"coordinator": {"conversation_id": "coord-active-swarm-1", "role": "coordinator", "wake_mode": "mentions", "killed": false},
			"fixer": {"conversation_id": "worker-running-1", "role": "worker", "wake_mode": "working", "killed": false}
		}
	}`), 0644)

	// 2. Historical finished swarm: all workers IDLE in upstream -> hasSwarm=true, status stays IDLE, no false running.
	idleCoordID := "coord-idle-swarm-2"
	idleWorkerID := "worker-idle-2"
	idleSwarmDir := filepath.Join(brainBase, idleCoordID, ".system_generated", "swarm")
	if err := os.MkdirAll(idleSwarmDir, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(idleSwarmDir, "members.json"), []byte(`{
		"title": "Finished Swarm",
		"members": {
			"coordinator": {"conversation_id": "coord-idle-swarm-2", "role": "coordinator", "wake_mode": "mentions", "killed": false},
			"done-worker": {"conversation_id": "worker-idle-2", "role": "worker", "wake_mode": "mentions", "killed": false}
		}
	}`), 0644)

	// 3. Freshly initialized swarm with only swarm.json (before members.json exists).
	freshCoordID := "coord-fresh-swarm-3"
	freshSwarmDir := filepath.Join(brainBase, freshCoordID, ".system_generated", "swarm")
	if err := os.MkdirAll(freshSwarmDir, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(freshSwarmDir, "swarm.json"), []byte(`{"version":1}`), 0644)

	// 4. Ordinary non-swarm conversation.
	plainID := "plain-idle-conv-4"

	now := time.Now().UTC()
	stale45m := now.Add(-45 * time.Minute).Format(time.RFC3339)
	idle10m := now.Add(-10 * time.Minute).Format(time.RFC3339)
	workerFresh := now.Add(-5 * time.Second).Format(time.RFC3339)
	helperFresh := now.Add(-2 * time.Second).Format(time.RFC3339)
	yesterday := now.Add(-24 * time.Hour).Format(time.RFC3339)
	yesterdayWorker := now.Add(-23 * time.Hour).Format(time.RFC3339)

	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories" {
			resp := map[string]interface{}{
				"trajectorySummaries": map[string]interface{}{
					coordID: map[string]interface{}{
						"summary":          "iPhone Duo 适配开发",
						"status":           "CASCADE_RUN_STATUS_IDLE",
						"stepCount":        569,
						"lastModifiedTime": stale45m,
					},
					workerRunningID: map[string]interface{}{
						"summary":          "",
						"status":           "CASCADE_RUN_STATUS_RUNNING",
						"stepCount":        120,
						"lastModifiedTime": workerFresh,
						"trajectoryMetadata": map[string]interface{}{
							"parentConversationId": coordID,
							"nestingDepth":         1,
						},
					},
					workerHelperID: map[string]interface{}{
						"summary":          "",
						"status":           "CASCADE_RUN_STATUS_RUNNING",
						"stepCount":        12,
						"lastModifiedTime": helperFresh,
						"trajectoryMetadata": map[string]interface{}{
							"parentConversationId": workerRunningID,
							"rootConversationId":   coordID,
							"nestingDepth":         2,
						},
					},
					idleCoordID: map[string]interface{}{
						"summary":          "Yesterday Swarm",
						"status":           "CASCADE_RUN_STATUS_IDLE",
						"stepCount":        80,
						"lastModifiedTime": yesterday,
					},
					idleWorkerID: map[string]interface{}{
						"summary":          "",
						"status":           "CASCADE_RUN_STATUS_IDLE",
						"stepCount":        25,
						"lastModifiedTime": yesterdayWorker,
						"trajectoryMetadata": map[string]interface{}{
							"parentConversationId": idleCoordID,
							"nestingDepth":         1,
						},
					},
					freshCoordID: map[string]interface{}{
						"summary":          "Fresh Swarm Init",
						"status":           "CASCADE_RUN_STATUS_RUNNING",
						"stepCount":        3,
						"lastModifiedTime": idle10m,
					},
					plainID: map[string]interface{}{
						"summary":          "Plain Idle Chat",
						"status":           "CASCADE_RUN_STATUS_IDLE",
						"stepCount":        10,
						"lastModifiedTime": idle10m,
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	var port int
	_, _ = fmt.Sscanf(upstream.Listener.Addr().String(), "127.0.0.1:%d", &port)
	p := &Proxy{
		shortClient:  upstream.Client(),
		mediumClient: upstream.Client(),
		longClient:   upstream.Client(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", nil)
	rec := httptest.NewRecorder()
	p.handleGetAllCascadeTrajectories(rec, req, port, "tok")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var parsed struct {
		TrajectorySummaries map[string]map[string]interface{} `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Subagents must still be filtered out.
	if _, exists := parsed.TrajectorySummaries[workerRunningID]; exists {
		t.Errorf("expected workerRunningID to be filtered out")
	}
	if _, exists := parsed.TrajectorySummaries[workerHelperID]; exists {
		t.Errorf("expected workerHelperID to be filtered out")
	}

	// Active swarm coordinator: promoted to RUNNING, hasSwarm=true, swarmWorkingCount=1, fresh lastModifiedTime.
	active := parsed.TrajectorySummaries[coordID]
	if active == nil {
		t.Fatalf("missing active swarm coordinator %s", coordID)
	}
	if active["status"] != "CASCADE_RUN_STATUS_RUNNING" {
		t.Errorf("expected active swarm status RUNNING, got %v", active["status"])
	}
	if active["hasSwarm"] != true {
		t.Errorf("expected active swarm hasSwarm=true, got %v", active["hasSwarm"])
	}
	if got, _ := active["swarmWorkingCount"].(float64); int(got) != 1 {
		t.Errorf("expected swarmWorkingCount=1, got %v", active["swarmWorkingCount"])
	}
	if got, _ := active["swarmTotalMembers"].(float64); int(got) != 2 {
		t.Errorf("expected swarmTotalMembers=2, got %v", active["swarmTotalMembers"])
	}
	gotMod, err := parseTime(fmt.Sprintf("%v", active["lastModifiedTime"]))
	if err != nil {
		t.Fatalf("invalid active lastModifiedTime %v: %v", active["lastModifiedTime"], err)
	}
	idle10mT, _ := parseTime(idle10m)
	if !gotMod.After(idle10mT) {
		t.Errorf("expected active swarm lastModifiedTime (%v) to surpass plain idle chat (%v)", gotMod, idle10mT)
	}

	// Finished historical swarm: hasSwarm=true, status stays IDLE, lastModifiedTime bumped to yesterdayWorker (not now!).
	idleSwarm := parsed.TrajectorySummaries[idleCoordID]
	if idleSwarm == nil {
		t.Fatalf("missing idle swarm coordinator %s", idleCoordID)
	}
	if idleSwarm["hasSwarm"] != true {
		t.Errorf("expected finished swarm hasSwarm=true, got %v", idleSwarm["hasSwarm"])
	}
	if idleSwarm["status"] != "CASCADE_RUN_STATUS_IDLE" {
		t.Errorf("expected finished swarm status IDLE, got %v", idleSwarm["status"])
	}
	if _, hasWorking := idleSwarm["swarmWorkingCount"]; hasWorking {
		t.Errorf("expected finished swarm to omit non-zero swarmWorkingCount, got %v", idleSwarm["swarmWorkingCount"])
	}
	idleSwarmMod, err := parseTime(fmt.Sprintf("%v", idleSwarm["lastModifiedTime"]))
	if err != nil {
		t.Fatalf("invalid idle swarm lastModifiedTime: %v", err)
	}
	if idleSwarmMod.After(idle10mT) {
		t.Errorf("finished historical swarm must not inherit today's file mtime: got %v", idleSwarmMod)
	}

	// Fresh swarm (swarm.json only): hasSwarm=true.
	fresh := parsed.TrajectorySummaries[freshCoordID]
	if fresh == nil || fresh["hasSwarm"] != true {
		t.Errorf("expected fresh swarm.json coordinator to have hasSwarm=true, got %+v", fresh)
	}

	// Plain non-swarm conversation: no hasSwarm.
	plain := parsed.TrajectorySummaries[plainID]
	if plain == nil {
		t.Fatalf("missing plain conversation")
	}
	if _, has := plain["hasSwarm"]; has {
		t.Errorf("expected plain conversation to not set hasSwarm, got %v", plain["hasSwarm"])
	}
}
