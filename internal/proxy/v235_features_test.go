package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestV235_CapabilitiesAndBrowserWhitelistWithoutRemovedRPCs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// Verify capabilities.go no longer requires any of the 5 RPCs removed in Antigravity 2.35.0.
	removed235 := map[string]bool{
		"GetBrowserOpenConversation":   true,
		"GetAllBrowserWhitelistedUrls": true,
		"AddToBrowserWhitelist":        true,
		"UpdateCascadeMemory":          true,
		"GetKnowledgeItems":            true,
	}
	for _, f := range Features {
		for _, rpc := range f.RPCs {
			if removed235[rpc] {
				t.Errorf("feature %q still depends on removed Antigravity 2.35.0 RPC %q", f.ID, rpc)
			}
		}
	}

	// Browser whitelist endpoint persists locally without calling removed AddToBrowserWhitelist RPC.
	var upstreamCalled bool
	p := setupMockUpstreamProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/AddToBrowserWhitelist") {
			upstreamCalled = true
		}
		http.NotFound(w, r)
	})

	req := httptest.NewRequest(http.MethodPost, "/gateway/subagent/browser/whitelist", strings.NewReader(`{"url":"https://example.com"}`))
	rec := httptest.NewRecorder()
	p.HandleBrowserSubagent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if upstreamCalled {
		t.Fatalf("must not call removed AddToBrowserWhitelist upstream RPC")
	}
	rawCfg, err := os.ReadFile(resolveLocalGeminiConfigPath())
	if err != nil {
		t.Fatalf("expected local config.json written: %v", err)
	}
	if !strings.Contains(string(rawCfg), "https://example.com") {
		t.Fatalf("expected whitelisted url persisted in config.json, got %s", rawCfg)
	}

	// Built-in 2.35.0 `conversations` API skill stays hidden from the mobile "/" menu.
	opts, byName := buildSlashCommandTables([]upstreamSlashCommand{
		{Info: json.RawMessage(`{"name":"conversations","type":"SLASH_COMMAND_TYPE_SKILL"}`), Title: "conversations", Description: "Manage local conversations"},
		{Info: json.RawMessage(`{"name":"plan","type":"SLASH_COMMAND_TYPE_SYSTEM"}`), Title: "plan", Description: "Plan"},
	})
	for _, o := range opts {
		if o.Name == "conversations" {
			t.Errorf("conversations skill must be hidden from mobile slash menu")
		}
	}
	if _, ok := byName["conversations"]; !ok {
		t.Errorf("expected conversations authoritative info retained in lookup map")
	}
}

func TestV235_SyncHistoricalTrajectoriesCappedNewestFirst(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	resetTrajectoryGlobals()

	convDir := filepath.Join(home, ".gemini", "antigravity", "conversations")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create 6 non-empty conversation DBs (> 49152 bytes) with staggered mod times.
	payload := make([]byte, 64*1024)
	baseTime := time.Now().Add(-10 * time.Hour)
	ids := []string{"conv-01", "conv-02", "conv-03", "conv-04", "conv-05", "conv-06"}
	for i, id := range ids {
		dbPath := filepath.Join(convDir, id+".db")
		if err := os.WriteFile(dbPath, payload, 0o600); err != nil {
			t.Fatal(err)
		}
		mt := baseTime.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(dbPath, mt, mt); err != nil {
			t.Fatal(err)
		}
	}

	// Set cap BEFORE setupMockUpstreamProxy so the automatic background sync triggered by
	// SetTestUpstream uses the same cap and coalesces cleanly with the explicit call below.
	t.Setenv("MGY_MAX_HISTORICAL_SYNC", "3")

	var mu sync.Mutex
	var loaded []string
	p := setupMockUpstreamProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/LoadTrajectory") {
			var body struct {
				CascadeID string `json:"cascadeId"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			loaded = append(loaded, body.CascadeID)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
			return
		}
		http.NotFound(w, r)
	})
	port, token := p.ActiveUpstream()

	// Cap eager preloading to the 3 newest conversations (coalescing with SetTestUpstream's goroutine).
	if err := p.SyncHistoricalTrajectories(port, token); err != nil {
		t.Fatalf("SyncHistoricalTrajectories failed: %v", err)
	}
	// A second call on the same port must not load deferred older batches.
	if err := p.SyncHistoricalTrajectories(port, token); err != nil {
		t.Fatalf("second SyncHistoricalTrajectories failed: %v", err)
	}
	mu.Lock()
	gotLoaded := append([]string(nil), loaded...)
	mu.Unlock()
	sort.Strings(gotLoaded)
	want := []string{"conv-04", "conv-05", "conv-06"}
	if strings.Join(gotLoaded, ",") != strings.Join(want, ",") {
		t.Fatalf("loaded = %v, want newest 3 %v", gotLoaded, want)
	}
	if !HasSyncedHistoricalTrajectories(port) {
		t.Fatalf("expected HasSyncedHistoricalTrajectories(%d) = true", port)
	}

	// MGY_MAX_HISTORICAL_SYNC=0 disables eager preloading while marking port synced.
	mu.Lock()
	loaded = nil
	mu.Unlock()
	t.Setenv("MGY_MAX_HISTORICAL_SYNC", "0")
	if err := p.SyncHistoricalTrajectories(port+1, token); err != nil {
		t.Fatalf("SyncHistoricalTrajectories(0) failed: %v", err)
	}
	mu.Lock()
	zeroLoaded := append([]string(nil), loaded...)
	mu.Unlock()
	if len(zeroLoaded) != 0 {
		t.Fatalf("expected 0 eager loads when MGY_MAX_HISTORICAL_SYNC=0, got %v", zeroLoaded)
	}
	if !HasSyncedHistoricalTrajectories(port + 1) {
		t.Fatalf("expected HasSyncedHistoricalTrajectories(%d) = true", port+1)
	}
}

func TestV235_InternalSessionConnectionFilesBlocked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	agyDir := filepath.Join(home, ".gemini", "antigravity")
	for _, rel := range []string{
		"daemon/hub_50402.json",
		"presence/03428bf8-18ee-4b6a-b2fc-3ec71196a8df.lock",
		"antigravity_state.pbtxt",
		"agyhub_summaries_proto.pb",
		"mcp_config.json.bak_wind_tools",
		"brain/test-casc/implementation_plan.md",
		"knowledge/notes.md",
	} {
		full := filepath.Join(agyDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("dummy"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	blocked := []string{
		filepath.Join(agyDir, "daemon", "hub_50402.json"),
		filepath.Join(agyDir, "presence", "03428bf8-18ee-4b6a-b2fc-3ec71196a8df.lock"),
		filepath.Join(agyDir, "antigravity_state.pbtxt"),
		filepath.Join(agyDir, "agyhub_summaries_proto.pb"),
		filepath.Join(agyDir, "mcp_config.json.bak_wind_tools"),
	}
	for _, p := range blocked {
		if IsSafeFilePath(p) || IsSafeReadableFilePath(p) {
			t.Errorf("expected internal session/secret file to be blocked: %s", p)
		}
		if _, err := ResolveLocalFilePath(p, ""); err == nil {
			t.Errorf("expected ResolveLocalFilePath to reject: %s", p)
		}
	}

	// Legitimate brain artifacts and knowledge files must remain accessible.
	allowed := []string{
		filepath.Join(agyDir, "brain", "test-casc", "implementation_plan.md"),
		filepath.Join(agyDir, "knowledge", "notes.md"),
	}
	for _, p := range allowed {
		if !IsSafeFilePath(p) || !IsSafeReadableFilePath(p) {
			t.Errorf("expected legitimate artifact/knowledge file to remain readable: %s", p)
		}
	}

	// Symlink inside brain/ pointing to daemon/hub_50402.json must be rejected.
	if runtime.GOOS != "windows" {
		linkPath := filepath.Join(agyDir, "brain", "test-casc", "escape.json")
		targetPath := filepath.Join(agyDir, "daemon", "hub_50402.json")
		if err := os.Symlink(targetPath, linkPath); err != nil {
			t.Fatal(err)
		}
		if IsSafeReadableFilePath(linkPath) {
			t.Errorf("expected symlink escaping into daemon/ to be rejected")
		}
		if _, err := GetFileContent("/static/artifacts/test-casc/escape.json", "test-casc"); err == nil {
			t.Errorf("expected GetFileContent to reject symlink into daemon/")
		}
	}
}

func TestV235_PermissionPresetsDecoupleAutoAndTurbo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// 1. Explicit mode="auto" writes AGENT_PERMISSION_PRESET_AUTO with terminal sandbox ON (not Turbo).
	p := &Proxy{}
	recAuto := httptest.NewRecorder()
	p.HandlePermissions(recAuto, httptest.NewRequest(http.MethodPost, "/gateway/permissions", strings.NewReader(`{"mode":"auto"}`)))
	if recAuto.Code != http.StatusOK {
		t.Fatalf("POST mode=auto status=%d body=%s", recAuto.Code, recAuto.Body.String())
	}
	var stAuto PermissionState
	if err := json.Unmarshal(recAuto.Body.Bytes(), &stAuto); err != nil {
		t.Fatal(err)
	}
	if stAuto.Preset != "auto" || stAuto.Mode != "auto" || stAuto.PermissionPreset != "AGENT_PERMISSION_PRESET_AUTO" ||
		!stAuto.EnableTerminalSandbox || stAuto.NonWorkspaceFilePolicy != "FILE_ACCESS_POLICY_ASK" || !stAuto.IsUnlocked {
		t.Fatalf("unexpected sandboxed Auto state: %+v", stAuto)
	}

	// 2. Explicit preset="agent_permission_preset_auto" also stays sandboxed Auto.
	recAuto2 := httptest.NewRecorder()
	p.HandlePermissions(recAuto2, httptest.NewRequest(http.MethodPost, "/gateway/permissions", strings.NewReader(`{"preset":"AGENT_PERMISSION_PRESET_AUTO"}`)))
	var stAuto2 PermissionState
	_ = json.Unmarshal(recAuto2.Body.Bytes(), &stAuto2)
	if stAuto2.Mode != "auto" || !stAuto2.EnableTerminalSandbox || stAuto2.PermissionPreset != "AGENT_PERMISSION_PRESET_AUTO" {
		t.Fatalf("explicit AGENT_PERMISSION_PRESET_AUTO must not downgrade to Turbo: %+v", stAuto2)
	}

	// 3. Legacy preset="auto" / mode="turbo" keeps Turbo behavior for shipped 3-tier clients.
	recTurbo := httptest.NewRecorder()
	p.HandlePermissions(recTurbo, httptest.NewRequest(http.MethodPost, "/gateway/permissions", strings.NewReader(`{"preset":"auto"}`)))
	var stTurbo PermissionState
	_ = json.Unmarshal(recTurbo.Body.Bytes(), &stTurbo)
	if stTurbo.Preset != "auto" || stTurbo.Mode != "turbo" || stTurbo.PermissionPreset != "AGENT_PERMISSION_PRESET_TURBO" ||
		stTurbo.EnableTerminalSandbox || stTurbo.NonWorkspaceFilePolicy != "FILE_ACCESS_POLICY_ALLOW" {
		t.Fatalf("unexpected Turbo state: %+v", stTurbo)
	}

	// 4. Upstream reporting AGENT_PERMISSION_PRESET_AUTO without explicit enableTerminalSandbox
	// must NOT be misclassified as Turbo even when local config.json has stale TURBO/ALLOW values.
	pUp := setupMockUpstreamProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/GetPermissionSettings") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"settings":{"permissionPreset":"AGENT_PERMISSION_PRESET_AUTO"}}`))
			return
		}
		http.NotFound(w, r)
	})
	recGet := httptest.NewRecorder()
	pUp.HandlePermissions(recGet, httptest.NewRequest(http.MethodGet, "/gateway/permissions", nil))
	if recGet.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", recGet.Code, recGet.Body.String())
	}
	var stGet PermissionState
	_ = json.Unmarshal(recGet.Body.Bytes(), &stGet)
	if stGet.Preset != "auto" || stGet.Mode != "auto" || !stGet.EnableTerminalSandbox || stGet.PermissionPreset != "AGENT_PERMISSION_PRESET_AUTO" {
		t.Fatalf("upstream AGENT_PERMISSION_PRESET_AUTO misclassified: %+v", stGet)
	}
}

func TestV235_SwarmTailWindowAndBeforeSeqPaging(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	coordID := "cascade-swarm-paging-235"
	swarmDir := filepath.Join(home, ".gemini", "antigravity", "brain", coordID, ".system_generated", "swarm")
	if err := os.MkdirAll(swarmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	membersJSON := `{
		"title": "分页测试蜂群",
		"members": {
			"worker-1": {"conversation_id": "conv-pg-w1", "model": "gemini-4-carbon", "role": "worker", "wake_mode": "mentions", "last_read_seq": 5, "killed": false}
		}
	}`
	if err := os.WriteFile(filepath.Join(swarmDir, "members.json"), []byte(membersJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	var lines []string
	for i := 1; i <= 10; i++ {
		agent := "worker-1"
		if i%2 == 1 {
			agent = "coordinator"
		}
		lines = append(lines, fmt.Sprintf(`{"seq":%d,"agent":"%s","text":"@worker-1 message #%d","time":"2026-11-23T04:%02d:00Z"}`, i, agent, i, i))
	}
	if err := os.WriteFile(filepath.Join(swarmDir, "board.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &Proxy{}

	// 1. Initial tail page: limit=3 returns seqs [8, 9, 10], TotalMessages=10, HasMoreBefore=true, OldestSeq=8.
	rec1 := httptest.NewRecorder()
	p.HandleGetSwarm(rec1, httptest.NewRequest(http.MethodGet, "/gateway/swarm?cascadeId="+coordID+"&limit=3", nil))
	if rec1.Code != http.StatusOK {
		t.Fatalf("tail page status=%d body=%s", rec1.Code, rec1.Body.String())
	}
	etag1 := rec1.Header().Get("ETag")
	if etag1 == "" {
		t.Fatalf("expected non-empty ETag on swarm response")
	}
	var snap1 SwarmSnapshot
	if err := json.Unmarshal(rec1.Body.Bytes(), &snap1); err != nil {
		t.Fatal(err)
	}
	if snap1.TotalMessages != 10 || !snap1.HasMoreBefore || snap1.OldestSeq != 8 || len(snap1.Messages) != 3 {
		t.Fatalf("unexpected tail snapshot paging: total=%d hasMore=%v oldest=%d len=%d",
			snap1.TotalMessages, snap1.HasMoreBefore, snap1.OldestSeq, len(snap1.Messages))
	}
	if snap1.Messages[0].Seq != 8 || snap1.Messages[2].Seq != 10 {
		t.Fatalf("expected seqs [8..10], got [%d..%d]", snap1.Messages[0].Seq, snap1.Messages[2].Seq)
	}
	// Member post count must reflect ALL 10 board messages (worker-1 posted evens: 2,4,6,8,10 -> 5 posts).
	if len(snap1.Members) != 1 || snap1.Members[0].Posts != 5 {
		t.Fatalf("expected worker-1 Posts=5 across full board, got %+v", snap1.Members)
	}
	// Mentions must be populated on the returned window.
	if len(snap1.Messages[0].Mentions) == 0 || snap1.Messages[0].Mentions[0] != "worker-1" {
		t.Fatalf("expected mention extraction on retained window, got %+v", snap1.Messages[0].Mentions)
	}

	// 2. ETag 304 Not Modified short-circuit.
	req304 := httptest.NewRequest(http.MethodGet, "/gateway/swarm?cascadeId="+coordID+"&limit=3", nil)
	req304.Header.Set("If-None-Match", etag1)
	rec304 := httptest.NewRecorder()
	p.HandleGetSwarm(rec304, req304)
	if rec304.Code != http.StatusNotModified {
		t.Fatalf("expected 304 Not Modified for matching ETag, got %d", rec304.Code)
	}

	// 3. Load older page: beforeSeq=8&limit=3 returns [5, 6, 7], HasMoreBefore=true, OldestSeq=5.
	recOlder := httptest.NewRecorder()
	p.HandleGetSwarm(recOlder, httptest.NewRequest(http.MethodGet, "/gateway/swarm?cascadeId="+coordID+"&beforeSeq=8&limit=3", nil))
	if recOlder.Code != http.StatusOK {
		t.Fatalf("older page status=%d body=%s", recOlder.Code, recOlder.Body.String())
	}
	var snapOlder SwarmSnapshot
	_ = json.Unmarshal(recOlder.Body.Bytes(), &snapOlder)
	if len(snapOlder.Messages) != 3 || snapOlder.Messages[0].Seq != 5 || snapOlder.Messages[2].Seq != 7 || !snapOlder.HasMoreBefore || snapOlder.OldestSeq != 5 {
		t.Fatalf("unexpected older page: len=%d seqs=[%d..%d] hasMore=%v oldest=%d",
			len(snapOlder.Messages), snapOlder.Messages[0].Seq, snapOlder.Messages[len(snapOlder.Messages)-1].Seq, snapOlder.HasMoreBefore, snapOlder.OldestSeq)
	}

	// 4. Incremental poll: afterSeq=8 returns [9, 10] with different ETag.
	recInc := httptest.NewRecorder()
	p.HandleGetSwarm(recInc, httptest.NewRequest(http.MethodGet, "/gateway/swarm?cascadeId="+coordID+"&afterSeq=8", nil))
	if recInc.Code != http.StatusOK {
		t.Fatalf("incremental poll status=%d", recInc.Code)
	}
	var snapInc SwarmSnapshot
	_ = json.Unmarshal(recInc.Body.Bytes(), &snapInc)
	if len(snapInc.Messages) != 2 || snapInc.Messages[0].Seq != 9 || snapInc.Messages[1].Seq != 10 {
		t.Fatalf("unexpected incremental messages: %+v", snapInc.Messages)
	}
}

func TestV235_OutsideProjectWriteDirectoryScopeSuggestion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	workspaceDir := filepath.Join(home, "Projects", "demo")
	outsideFile := filepath.Join(home, "Notes", "drafts", "summary.md")
	insideFile := filepath.Join(workspaceDir, "src", "main.go")

	// 1. Outside-workspace file_permission prompt gets directory-scoped suggestion "~/Notes/drafts/*".
	rawOutside := fmt.Sprintf(`{
		"status": "CASCADE_RUN_STATUS_RUNNING",
		"trajectory": {
			"cascadeId": "casc-outside-perm",
			"workspaceUris": [%q],
			"steps": [{
				"type": "CORTEX_STEP_TYPE_FILE_PERMISSION",
				"status": "CORTEX_STEP_STATUS_WAITING",
				"requestedInteraction": {
					"filePermission": {
						"absolutePathUri": %q
					}
				}
			}]
		}
	}`, "file://"+filepath.ToSlash(workspaceDir), "file://"+filepath.ToSlash(outsideFile))

	var respOutside upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawOutside), &respOutside); err != nil {
		t.Fatal(err)
	}
	piOutside := buildPendingInteraction(&respOutside, respOutside.Trajectory.Steps)
	if piOutside == nil {
		t.Fatalf("expected pending interaction for file_permission")
	}
	if piOutside.SuggestedDirectoryScope != "~/Notes/drafts/*" {
		t.Fatalf("SuggestedDirectoryScope = %q, want ~/Notes/drafts/*", piOutside.SuggestedDirectoryScope)
	}
	if len(piOutside.Options) < 4 || !strings.Contains(piOutside.Options[1].Text, "~/Notes/drafts/*") {
		t.Fatalf("expected directory-scoped option text, got %+v", piOutside.Options)
	}

	// 2. Inside-workspace write permission keeps standard options (no outside directory scope).
	rawInside := fmt.Sprintf(`{
		"status": "CASCADE_RUN_STATUS_RUNNING",
		"trajectory": {
			"cascadeId": "casc-inside-perm",
			"workspaceUris": [%q],
			"steps": [{
				"type": "CORTEX_STEP_TYPE_PERMISSION",
				"status": "CORTEX_STEP_STATUS_WAITING",
				"requestedInteraction": {
					"permission": {
						"actionDescription": "Write file",
						"resource": {
							"action": "write_file",
							"target": %q
						}
					}
				}
			}]
		}
	}`, "file://"+filepath.ToSlash(workspaceDir), insideFile)

	var respInside upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawInside), &respInside); err != nil {
		t.Fatal(err)
	}
	piInside := buildPendingInteraction(&respInside, respInside.Trajectory.Steps)
	if piInside == nil {
		t.Fatalf("expected pending interaction for inside permission")
	}
	if piInside.SuggestedDirectoryScope != "" {
		t.Fatalf("inside-workspace write must not suggest outside directory scope, got %q", piInside.SuggestedDirectoryScope)
	}

	// 3. Read permission must NOT trigger write directory-scope suggestion.
	rawRead := fmt.Sprintf(`{
		"status": "CASCADE_RUN_STATUS_RUNNING",
		"trajectory": {
			"cascadeId": "casc-outside-read",
			"workspaceUris": [%q],
			"steps": [{
				"type": "CORTEX_STEP_TYPE_PERMISSION",
				"status": "CORTEX_STEP_STATUS_WAITING",
				"requestedInteraction": {
					"permission": {
						"actionDescription": "Read file",
						"resource": {
							"action": "read_file",
							"target": %q
						}
					}
				}
			}]
		}
	}`, "file://"+filepath.ToSlash(workspaceDir), outsideFile)
	var respRead upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawRead), &respRead); err != nil {
		t.Fatal(err)
	}
	piRead := buildPendingInteraction(&respRead, respRead.Trajectory.Steps)
	if piRead == nil || piRead.SuggestedDirectoryScope != "" {
		t.Fatalf("read_file permission must not suggest write directory scope, got %+v", piRead)
	}
}

func TestV235_MergeAdjacentToolsWithoutInterveningOutput(t *testing.T) {
	raw := `{
		"status": "CASCADE_RUN_STATUS_IDLE",
		"trajectory": {
			"cascadeId": "casc-merge-tools",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"userInput": {"userResponse": "inspect repo"}
				},
				{
					"type": "CORTEX_STEP_TYPE_VIEW_FILE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"viewFile": {"absolutePathUri": "file:///tmp/a.go"}
				},
				{
					"type": "CORTEX_STEP_TYPE_INVOKE_SUBAGENT",
					"status": "CORTEX_STEP_STATUS_DONE",
					"invokeSubagent": {"subagents": [], "results": []}
				},
				{
					"type": "CORTEX_STEP_TYPE_GREP_SEARCH",
					"status": "CORTEX_STEP_STATUS_DONE",
					"grepSearch": {"query": "TODO"}
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": {"response": "Done checking."}
				}
			]
		}
	}`
	var resp upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	msgs, totalTools := buildTrajectoryMessages(resp.Trajectory.Steps)
	if totalTools != 2 {
		t.Fatalf("expected 2 tools total, got %d", totalTools)
	}
	toolCapsules := 0
	for _, m := range msgs {
		if m.Type == "tools" {
			toolCapsules++
			if m.ToolCount != 2 {
				t.Fatalf("expected merged tools capsule ToolCount=2, got %d", m.ToolCount)
			}
		}
	}
	if toolCapsules != 1 {
		t.Fatalf("expected adjacent tool sections to merge into 1 capsule, got %d (msgs=%+v)", toolCapsules, msgs)
	}
}

func TestV235_SearchConversationsEnrichesSideQuestions(t *testing.T) {
	sideID := "casc-btw-side-235-test"
	parentID := "casc-btw-parent-235-test"
	defaultBtwTracker.record(BtwSideQuestionEntry{
		CascadeID:       sideID,
		ParentCascadeID: parentID,
		Title:           "解释一下 ETag",
		Question:        "解释一下 ETag",
	})
	t.Cleanup(func() {
		defaultBtwTracker.mu.Lock()
		delete(defaultBtwTracker.bySide, sideID)
		delete(defaultBtwTracker.byParent, parentID)
		defaultBtwTracker.mu.Unlock()
	})

	results := []ConversationSearchResult{
		{CascadeID: sideID, Title: "", Snippet: "ETag 304 缓存命中"},
		{CascadeID: "casc-normal-235-test", Title: "普通会话", Snippet: "正文"},
	}
	enrichSearchResultsWithSideQuestions(results)

	if !results[0].IsSideQuestion || results[0].ParentConversationID != parentID {
		t.Fatalf("expected side-question enrichment, got %+v", results[0])
	}
	if results[0].Title != "解释一下 ETag" {
		t.Fatalf("expected fallback title from btw tracker, got %q", results[0].Title)
	}
	if results[1].IsSideQuestion || results[1].ParentConversationID != "" {
		t.Fatalf("ordinary conversation must not be marked side-question: %+v", results[1])
	}
}
