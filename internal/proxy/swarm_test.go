package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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
