package proxy

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func setupMockUpstreamProxy(t *testing.T, handler http.HandlerFunc) *Proxy {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)
	_, portStr, _ := net.SplitHostPort(u.Host)
	port, _ := strconv.Atoi(portStr)

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
	p := &Proxy{
		shortClient:  client,
		mediumClient: client,
		longClient:   client,
		streamClient: client,
	}
	p.SetTestUpstream(port, "test-token")
	return p
}

func TestParseTrajectoryDetails_ThinkingAndSuggestedResponses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	resetTrajectoryGlobals()

	rawJSON := `{
		"status": "CASCADE_RUN_STATUS_IDLE",
		"trajectory": {
			"cascadeId": "phase3-traj",
			"steps": [
				{
					"type": "CORTEX_STEP_TYPE_USER_INPUT",
					"userInput": {"userResponse": "analyze repo"}
				},
				{
					"type": "CORTEX_STEP_TYPE_TASK_BOUNDARY",
					"status": "CORTEX_STEP_STATUS_DONE",
					"metadata": {"startedAt": "2026-11-15T10:00:00Z", "completedAt": "2026-11-15T10:00:01Z", "toolAction": "Analyzing"},
					"taskBoundary": {"taskName": "Scan", "taskStatus": "Scanning files", "taskSummary": "Read codebase", "mode": "EXECUTION"}
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": {"thinking": "Need to inspect gateway routes first.", "thinkingDuration": "1.4s", "response": ""}
				},
				{
					"type": "CORTEX_STEP_TYPE_RUN_COMMAND",
					"status": "CORTEX_STEP_STATUS_DONE",
					"metadata": {"startedAt": "2026-11-15T10:00:01Z", "completedAt": "2026-11-15T10:00:03.5Z", "toolSummary": "List files"},
					"runCommand": {"commandLine": "ls -la"}
				},
				{
					"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
					"status": "CORTEX_STEP_STATUS_DONE",
					"plannerResponse": {"thinking": "Everything looks ready.", "thinkingDuration": "2.1s", "response": "Here is the analysis."}
				},
				{
					"type": "CORTEX_STEP_TYPE_SUGGESTED_RESPONSES",
					"status": "CORTEX_STEP_STATUS_DONE",
					"suggestedResponses": {"suggestions": ["Run full tests", "Explain architecture"]}
				}
			]
		}
	}`
	var raw upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		t.Fatal(err)
	}
	p := &Proxy{}
	d := p.ParseTrajectoryDetails(&raw)

	if len(d.SuggestedResponses) != 2 || d.SuggestedResponses[0] != "Run full tests" {
		t.Fatalf("unexpected suggestedResponses: %#v", d.SuggestedResponses)
	}
	if d.TaskProgress == nil || d.TaskProgress.TaskName != "Scan" || d.TaskProgress.Mode != "EXECUTION" {
		t.Fatalf("unexpected taskProgress: %#v", d.TaskProgress)
	}

	var foundTools *CascadeMessageItem
	for i := range d.AllMessages {
		if d.AllMessages[i].Type == "tools" {
			foundTools = &d.AllMessages[i]
			break
		}
	}
	if foundTools == nil {
		t.Fatalf("expected folded tools capsule, got %#v", d.AllMessages)
	}
	if foundTools.ToolCount != 2 || len(foundTools.ToolDetails) != 2 {
		t.Fatalf("expected 2 tools & toolDetails, got %d / %d", foundTools.ToolCount, len(foundTools.ToolDetails))
	}
	if foundTools.ToolDetails[1].Duration != "2.5s" || foundTools.ToolDetails[1].Summary != "List files" {
		t.Fatalf("unexpected tool detail: %#v", foundTools.ToolDetails[1])
	}
	if !strings.Contains(foundTools.Thinking, "inspect gateway routes") || !strings.Contains(foundTools.Thinking, "looks ready") {
		t.Fatalf("expected merged thinking, got %q", foundTools.Thinking)
	}
	if foundTools.ThinkingDuration != "2.1s" {
		t.Fatalf("expected thinkingDuration 2.1s, got %q", foundTools.ThinkingDuration)
	}
}

func TestParseTrajectoryDetails_ThinkingOnlyNoToolsCapsule(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	resetTrajectoryGlobals()

	rawJSON := `{
		"status": "CASCADE_RUN_STATUS_RUNNING",
		"trajectory": {
			"cascadeId": "phase3-thinking-only",
			"steps": [
				{"type": "CORTEX_STEP_TYPE_USER_INPUT", "userInput": {"userResponse": "hi"}},
				{"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE", "status": "CORTEX_STEP_STATUS_RUNNING", "plannerResponse": {"thinking": "Thinking deeply...", "response": ""}}
			]
		}
	}`
	var raw upstreamTrajectoryResp
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		t.Fatal(err)
	}
	d := (&Proxy{}).ParseTrajectoryDetails(&raw)
	if len(d.AllMessages) != 2 || d.AllMessages[1].Type != "tools" || !strings.Contains(d.AllMessages[1].Text, "已深度思考") {
		t.Fatalf("expected thinking-only capsule, got %#v", d.AllMessages)
	}
	if d.AllMessages[1].Thinking != "Thinking deeply..." {
		t.Fatalf("expected streaming thinking text, got %q", d.AllMessages[1].Thinking)
	}
}

func TestHandlePermissions_LocalAndPresets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Seed config.json with unrelated keys that must be preserved.
	cfgPath := resolveLocalGeminiConfigPath()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(`{"theme":"dark","userSettings":{"foo":"bar"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	p := &Proxy{}
	// Switch to sandbox
	req := httptest.NewRequest(http.MethodPost, "/gateway/permissions", strings.NewReader(`{"preset":"sandbox"}`))
	w := httptest.NewRecorder()
	p.HandlePermissions(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var st PermissionState
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil || st.Preset != "sandbox" || !st.IsUnlocked || !st.EnableTerminalSandbox {
		t.Fatalf("unexpected sandbox state: %+v err=%v", st, err)
	}

	// Verify sibling keys preserved in config.json
	raw, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(raw), `"theme": "dark"`) || !strings.Contains(string(raw), `"foo": "bar"`) {
		t.Fatalf("lost unrelated keys in config.json: %s", raw)
	}

	// GET reads back sandbox
	reqGet := httptest.NewRequest(http.MethodGet, "/gateway/permissions", nil)
	wGet := httptest.NewRecorder()
	p.HandlePermissions(wGet, reqGet)
	var st2 PermissionState
	_ = json.Unmarshal(wGet.Body.Bytes(), &st2)
	if st2.Preset != "sandbox" || st2.Label != "离席沙箱放行" {
		t.Fatalf("GET mismatch: %+v", st2)
	}
}

func TestHandleMemories_CRUDAndPathGuard(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ws := t.TempDir()
	p := &Proxy{}

	// Upsert workspace rule
	reqBody, _ := json.Marshal(map[string]string{
		"action":        "upsert",
		"scope":         "workspace",
		"workspacePath": ws,
		"title":         "Style Guide",
		"content":       "# Keep functions short\n",
	})
	req := httptest.NewRequest(http.MethodPost, "/gateway/memories", bytes.NewReader(reqBody))
	w := httptest.NewRecorder()
	p.HandleMemories(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("upsert failed: %d %s", w.Code, w.Body.String())
	}

	// Reject path outside memory roots
	badBody, _ := json.Marshal(map[string]string{
		"action":  "upsert",
		"path":    "/etc/passwd",
		"content": "nope",
	})
	reqBad := httptest.NewRequest(http.MethodPost, "/gateway/memories", bytes.NewReader(badBody))
	wBad := httptest.NewRecorder()
	p.HandleMemories(wBad, reqBad)
	if wBad.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for outside path, got %d", wBad.Code)
	}

	// GET lists the rule
	reqGet := httptest.NewRequest(http.MethodGet, "/gateway/memories?workspacePath="+url.QueryEscape(ws), nil)
	wGet := httptest.NewRecorder()
	p.HandleMemories(wGet, reqGet)
	if !strings.Contains(wGet.Body.String(), "Keep functions short") {
		t.Fatalf("expected created rule in GET list: %s", wGet.Body.String())
	}

	// Delete archives a backup before removing original file
	delBody, _ := json.Marshal(map[string]string{
		"action":        "delete",
		"workspacePath": ws,
		"path":          filepath.Join(ws, ".agents", "rules", "style-guide.md"),
	})
	reqDel := httptest.NewRequest(http.MethodPost, "/gateway/memories", bytes.NewReader(delBody))
	wDel := httptest.NewRecorder()
	p.HandleMemories(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("delete failed: %d %s", wDel.Code, wDel.Body.String())
	}
	if _, err := os.Stat(filepath.Join(ws, ".agents", "rules", "style-guide.md")); !os.IsNotExist(err) {
		t.Fatalf("expected original file to be deleted")
	}
	backupMatches, err := filepath.Glob(filepath.Join(home, ".gemini", "backups", "deleted_memories", "*_style-guide.md"))
	if err != nil || len(backupMatches) != 1 {
		t.Fatalf("expected backup copy in deleted_memories, got %v (err=%v)", backupMatches, err)
	}
}

func TestHandleAutomationCreateDelete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	p := &Proxy{}

	req := httptest.NewRequest(http.MethodPost, "/gateway/automations/create", strings.NewReader(`{
		"name": "Morning Brief",
		"description": "Daily digest",
		"cron": "0 9 * * *",
		"prompt": "Summarize overnight PRs"
	}`))
	w := httptest.NewRecorder()
	p.HandleAutomationCreateDelete(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create failed: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK        bool   `json:"ok"`
		SidecarID string `json:"sidecarId"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.OK || resp.SidecarID == "" {
		t.Fatalf("unexpected create resp: %+v", resp)
	}

	schedules := readLocalSidecars()
	if sc, ok := schedules[resp.SidecarID]; !ok || sc.CronExpression != "0 9 * * *" || sc.Prompt != "Summarize overnight PRs" {
		t.Fatalf("sidecar not readable via readLocalSidecars: %+v", schedules)
	}

	delBody, _ := json.Marshal(map[string]string{"sidecarId": resp.SidecarID})
	reqDel := httptest.NewRequest(http.MethodPost, "/gateway/automations/delete", bytes.NewReader(delBody))
	wDel := httptest.NewRecorder()
	p.HandleAutomationCreateDelete(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("delete failed: %d %s", wDel.Code, wDel.Body.String())
	}
	if _, ok := readLocalSidecars()[resp.SidecarID]; ok {
		t.Fatalf("expected deleted sidecar to be gone")
	}
}

func TestHandleGitExtra_BranchAndDiscard(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-b", "main")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "a.txt")
	runGit("commit", "-m", "init")

	p := &Proxy{}
	// Create & checkout branch
	bodyCheckout, _ := json.Marshal(map[string]interface{}{
		"repoPath": repo,
		"branch":   "feat/phase3",
		"create":   true,
	})
	w := httptest.NewRecorder()
	p.HandleGitExtra(w, httptest.NewRequest(http.MethodPost, "/gateway/git/checkout", bytes.NewReader(bodyCheckout)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"current":"feat/phase3"`) {
		t.Fatalf("checkout failed: %d %s", w.Code, w.Body.String())
	}

	// Modify tracked file & create untracked file, then discard both
	_ = os.WriteFile(filepath.Join(repo, "a.txt"), []byte("dirty\n"), 0o644)
	_ = os.WriteFile(filepath.Join(repo, "untracked.tmp"), []byte("tmp\n"), 0o644)
	bodyDiscard, _ := json.Marshal(map[string]interface{}{
		"repoPath": repo,
		"paths":    []string{"a.txt", "untracked.tmp"},
	})
	wDisc := httptest.NewRecorder()
	p.HandleGitExtra(wDisc, httptest.NewRequest(http.MethodPost, "/gateway/git/discard", bytes.NewReader(bodyDiscard)))
	if wDisc.Code != http.StatusOK {
		t.Fatalf("discard failed: %d %s", wDisc.Code, wDisc.Body.String())
	}
	got, _ := os.ReadFile(filepath.Join(repo, "a.txt"))
	if string(got) != "hello\n" {
		t.Fatalf("expected restored a.txt, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(repo, "untracked.tmp")); !os.IsNotExist(err) {
		t.Fatalf("expected untracked.tmp removed")
	}
}

func TestHandleContextMentionsAndNormalization(t *testing.T) {
	ws := t.TempDir()
	_ = os.MkdirAll(filepath.Join(ws, "pkg"), 0o755)
	_ = os.WriteFile(filepath.Join(ws, "pkg", "server.go"), []byte("package pkg\n"), 0o644)

	p := setupMockUpstreamProxy(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetMatchingContextScopeItems"):
			_, _ = w.Write([]byte(`{"items":[]}`))
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			_, _ = w.Write([]byte(`{"trajectorySummaries":{}}`))
		case strings.HasSuffix(r.URL.Path, "/ForkConversation"):
			_, _ = w.Write([]byte(`{"cascadeId":"forked-123"}`))
		case strings.HasSuffix(r.URL.Path, "/ToggleMcpServer"):
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/SendAgentMessage"):
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/SendTerminalInput"):
			_, _ = w.Write([]byte(`{}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	})

	// Mentions scan workspace
	reqBody, _ := json.Marshal(map[string]string{
		"query":        "server",
		"workspaceUri": filePathToURI(ws),
	})
	w := httptest.NewRecorder()
	p.HandleContextMentions(w, httptest.NewRequest(http.MethodPost, "/gateway/context/mentions", bytes.NewReader(reqBody)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "server.go") {
		t.Fatalf("expected server.go in mentions, got %d %s", w.Code, w.Body.String())
	}

	// Normalization into upstream items[]
	rawMap := map[string]interface{}{
		"items": []interface{}{map[string]interface{}{"text": "check this"}},
		"contextItems": []interface{}{
			map[string]interface{}{"type": "file", "path": filepath.Join(ws, "pkg", "server.go")},
			map[string]interface{}{"type": "conversation", "cascadeId": "c-prev", "label": "Prev Chat"},
		},
	}
	appendNormalizedContextItems(rawMap)
	items := rawMap["items"].([]interface{})
	if len(items) != 3 {
		t.Fatalf("expected 3 items after normalization, got %d", len(items))
	}
	if _, exists := rawMap["contextItems"]; exists {
		t.Fatalf("contextItems should be deleted after normalization")
	}

	// ForkConversation
	wFork := httptest.NewRecorder()
	p.HandleCascadeFork(wFork, httptest.NewRequest(http.MethodPost, "/gateway/cascade/fork", strings.NewReader(`{"cascadeId":"c-1","stepIndex":3,"createWorktree":true}`)))
	if wFork.Code != http.StatusOK || !strings.Contains(wFork.Body.String(), `"forked-123"`) {
		t.Fatalf("fork failed: %d %s", wFork.Code, wFork.Body.String())
	}

	// ToggleMcpServer, SendAgentMessage, SendTerminalInput
	wMcp := httptest.NewRecorder()
	p.HandleMcpManage(wMcp, httptest.NewRequest(http.MethodPost, "/gateway/mcp/toggle", strings.NewReader(`{"serverName":"github","enabled":false}`)))
	if wMcp.Code != http.StatusOK {
		t.Fatalf("mcp toggle failed: %d %s", wMcp.Code, wMcp.Body.String())
	}
	wMsg := httptest.NewRecorder()
	p.HandleSubagentMessage(wMsg, httptest.NewRequest(http.MethodPost, "/gateway/subagent/message", strings.NewReader(`{"recipient":"sub-1","content":"focus on tests"}`)))
	if wMsg.Code != http.StatusOK {
		t.Fatalf("subagent msg failed: %d %s", wMsg.Code, wMsg.Body.String())
	}
	wTerm := httptest.NewRecorder()
	p.HandleTerminals(wTerm, httptest.NewRequest(http.MethodPost, "/gateway/terminal/input", strings.NewReader(`{"terminalId":"t-1","input":"y\n"}`)))
	if wTerm.Code != http.StatusOK {
		t.Fatalf("terminal input failed: %d %s", wTerm.Code, wTerm.Body.String())
	}
}

func TestHandleCreateProject_UpstreamAndOfflineAndValidation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", home)
	t.Setenv("ANTIGRAVITY_APP_DATA_DIR", filepath.Join(home, ".gemini", "antigravity"))
	t.Setenv("MULTIGRAVITY_DATA_DIR", filepath.Join(home, "mgy-data"))

	gitDir := filepath.Join(home, "Projects", "my-git-app")
	normalDir := filepath.Join(home, "Projects", "notes")
	filePath := filepath.Join(home, "Projects", "readme.txt")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(normalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	var createPayloads []map[string]interface{}
	p := setupMockUpstreamProxy(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/ResolveFolder"):
			var body struct {
				FolderURI string `json:"folderUri"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if strings.Contains(body.FolderURI, "my-git-app") {
				_, _ = w.Write([]byte(`{"resourceType":"FOLDER_TYPE_GIT_FOLDER"}`))
			} else {
				_, _ = w.Write([]byte(`{"resourceType":"FOLDER_TYPE_NORMAL_FOLDER"}`))
			}
		case strings.HasSuffix(r.URL.Path, "/GetRepoInfos"):
			_, _ = w.Write([]byte(`{"repos":[{"branches":[{"name":"main","isCurrentBranch":true}]}]}`))
		case strings.HasSuffix(r.URL.Path, "/CreateProject"):
			var payload map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			createPayloads = append(createPayloads, payload)
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			_, _ = w.Write([]byte(`{"trajectorySummaries":{}}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	})

	// 1. Validation errors
	for _, bad := range []string{`{"path":""}`, `{"path":"relative/dir"}`, `{"path":"` + filePath + `"}`, `{"path":"` + filepath.Join(home, "missing") + `"}`} {
		wBad := httptest.NewRecorder()
		p.ServeHTTP(wBad, httptest.NewRequest(http.MethodPost, "/gateway/projects/create", strings.NewReader(bad)))
		if wBad.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d: %s", bad, wBad.Code, wBad.Body.String())
		}
	}

	// 2. Git folder via ~/Projects/my-git-app
	wGit := httptest.NewRecorder()
	p.ServeHTTP(wGit, httptest.NewRequest(http.MethodPost, "/gateway/projects/create", strings.NewReader(`{"path":"~/Projects/my-git-app"}`)))
	if wGit.Code != http.StatusOK {
		t.Fatalf("expected 200 for git dir, got %d: %s", wGit.Code, wGit.Body.String())
	}
	var respGit struct {
		OK            bool        `json:"ok"`
		AlreadyExists bool        `json:"alreadyExists"`
		Project       ProjectItem `json:"project"`
	}
	if err := json.Unmarshal(wGit.Body.Bytes(), &respGit); err != nil {
		t.Fatal(err)
	}
	if !respGit.OK || respGit.AlreadyExists || respGit.Project.Name != "my-git-app" || respGit.Project.Path != gitDir {
		t.Fatalf("unexpected git create resp: %+v", respGit)
	}
	if len(createPayloads) != 1 {
		t.Fatalf("expected 1 CreateProject RPC, got %d", len(createPayloads))
	}
	projObj := createPayloads[0]["project"].(map[string]interface{})
	resources := projObj["projectResources"].(map[string]interface{})["resources"].([]interface{})
	gitRes := resources[0].(map[string]interface{})["gitFolder"].(map[string]interface{})
	if gitRes["defaultBranch"] != "main" {
		t.Fatalf("expected defaultBranch main, got %#v", gitRes)
	}

	// 3. Normal folder via offline fallback (port == 0)
	pOffline := &Proxy{}
	bodyNorm, _ := json.Marshal(map[string]string{"path": normalDir})
	wNorm := httptest.NewRecorder()
	pOffline.ServeHTTP(wNorm, httptest.NewRequest(http.MethodPost, "/gateway/projects/create", bytes.NewReader(bodyNorm)))
	if wNorm.Code != http.StatusOK {
		t.Fatalf("expected 200 for offline normal dir, got %d: %s", wNorm.Code, wNorm.Body.String())
	}
	projects, err := pOffline.GetProjects()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range projects {
		if item.Path == normalDir && item.Name == "notes" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected offline created project %s in GetProjects(), got %+v", normalDir, projects)
	}

	// 4. Duplicate path returns alreadyExists: true without extra CreateProject
	wDup := httptest.NewRecorder()
	pOffline.ServeHTTP(wDup, httptest.NewRequest(http.MethodPost, "/gateway/projects/create", bytes.NewReader(bodyNorm)))
	if wDup.Code != http.StatusOK || !strings.Contains(wDup.Body.String(), `"alreadyExists":true`) {
		t.Fatalf("expected alreadyExists:true on duplicate, got %d: %s", wDup.Code, wDup.Body.String())
	}
}

func TestHandleAudioDictate_Normalization(t *testing.T) {
	p := setupMockUpstreamProxy(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/exa.language_server_pb.LanguageServerService/StreamAudioTranscription":
			w.Header().Set("Content-Type", "application/connect+json")
			// Frame 1: ready
			f1, _ := buildConnectFrame(map[string]interface{}{
				"ready": map[string]interface{}{"sessionId": "test-sid-999"},
			})
			_, _ = w.Write(f1)
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
			time.Sleep(20 * time.Millisecond)

			// Frame 2: transcription proto JSON (upstream format)
			f2, _ := buildConnectFrame(map[string]interface{}{
				"transcription": map[string]interface{}{
					"text":    "测试语音识别",
					"isFinal": true,
				},
			})
			_, _ = w.Write(f2)
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}
			time.Sleep(20 * time.Millisecond)

			// Frame 3: Connect trailer (flag 2)
			trailer := []byte{2, 0, 0, 0, 2, '{', '}'}
			_, _ = w.Write(trailer)
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
			}

		case "/exa.language_server_pb.LanguageServerService/SendAudioChunk":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))

		case "/exa.language_server_pb.LanguageServerService/EndAudioSession":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{}"))

		default:
			http.NotFound(w, r)
		}
	})

	gwServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gateway/audio/dictate" {
			p.HandleAudioDictate(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	defer gwServer.Close()

	wsURL := "ws" + strings.TrimPrefix(gwServer.URL, "http") + "/gateway/audio/dictate?cascadeId=test-cid"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial audio dictate ws: %v", err)
	}
	defer conn.Close()

	// Wait for messages
	var receivedNormalized bool
	deadline := time.Now().Add(3 * time.Second)
	_ = conn.SetReadDeadline(deadline)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(msg, &parsed); err != nil {
			continue
		}
		// Check that the gateway normalized the message to include top-level text & transcript
		if parsed["text"] == "测试语音识别" && parsed["transcript"] == "测试语音识别" {
			receivedNormalized = true
			if isFinal, ok := parsed["isFinal"].(bool); !ok || !isFinal {
				t.Errorf("expected isFinal=true, got %v", parsed["isFinal"])
			}
			// And also preserves the original transcription block
			if tr, ok := parsed["transcription"].(map[string]interface{}); !ok || tr["text"] != "测试语音识别" {
				t.Errorf("expected original transcription block preserved, got %v", tr)
			}
			break
		}
	}

	if !receivedNormalized {
		t.Fatalf("did not receive normalized transcription message with text and transcript")
	}

	// Send an audio chunk and end message
	_ = conn.WriteMessage(websocket.BinaryMessage, make([]byte, 3200))
	_ = conn.WriteJSON(map[string]interface{}{"type": "end"})
}
