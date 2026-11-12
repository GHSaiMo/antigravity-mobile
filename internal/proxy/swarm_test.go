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
