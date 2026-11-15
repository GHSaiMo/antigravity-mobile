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
