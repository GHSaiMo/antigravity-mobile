package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestV234_NeonAndOrchestratorModelMappings(t *testing.T) {
	resetLiveModels(t)

	cases := []struct {
		id   string
		enum string
	}{
		{"gemini-4-neon-xhigh", "MODEL_PLACEHOLDER_M535"},
		{"gemini-4-neon-high", "MODEL_PLACEHOLDER_M532"},
		{"gemini-4-neon-medium", "MODEL_PLACEHOLDER_M533"},
		{"gemini-4-neon-low", "MODEL_PLACEHOLDER_M534"},
		{"gemini-pro-orchestrator-neon", "MODEL_PLACEHOLDER_M537"},
		{"gemini-flash-worker-agy", "MODEL_PLACEHOLDER_M669"},
	}
	for _, tc := range cases {
		if got := resolveModelEnum(tc.id); got != tc.enum {
			t.Errorf("resolveModelEnum(%q) = %q, want %q", tc.id, got, tc.enum)
		}
		if got := canonicalModelName(tc.enum); got != tc.id {
			t.Errorf("canonicalModelName(%q) = %q, want %q", tc.enum, got, tc.id)
		}
	}

	raw := map[string]upstreamModel{
		"gemini-4-neon-high":           {DisplayName: "Gemini 4 Neon (High)", Model: "MODEL_PLACEHOLDER_M532", ModelProvider: "MODEL_PROVIDER_GOOGLE", Recommended: true, TagTitle: "New"},
		"gemini-4-neon-xhigh":          {DisplayName: "Gemini 4 Neon (XHigh)", Model: "MODEL_PLACEHOLDER_M535", ModelProvider: "MODEL_PROVIDER_GOOGLE", Recommended: true, TagTitle: "New"},
		"gemini-pro-orchestrator-neon": {DisplayName: "Orchestrator (Neon)", Model: "MODEL_PLACEHOLDER_M537", ModelProvider: "MODEL_PROVIDER_GOOGLE", Recommended: true, TagTitle: "New"},
	}
	curated := []string{"gemini-4-neon-high", "gemini-4-neon-xhigh", "gemini-pro-orchestrator-neon"}
	o, bi, be := buildModelTables(raw, curated)
	liveModels.set(o, bi, be, "gemini-4-neon-high")

	res, live := liveModels.snapshot()
	if !live {
		t.Fatal("expected live snapshot")
	}
	if res.Defaults["gemini"] != "gemini-4-neon-high" {
		t.Fatalf("gemini default = %q, want gemini-4-neon-high", res.Defaults["gemini"])
	}
	if res.Defaults["orchestrator"] != "gemini-pro-orchestrator-neon" {
		t.Fatalf("orchestrator default = %q, want gemini-pro-orchestrator-neon", res.Defaults["orchestrator"])
	}
	if liveModels.displayName("gemini-4-neon-xhigh") != "Gemini 4 Neon (XHigh)" {
		t.Fatalf("xhigh displayName = %q, want Gemini 4 Neon (XHigh)", liveModels.displayName("gemini-4-neon-xhigh"))
	}
}

func TestV234_SlashCommandDefinitionPathPropagation(t *testing.T) {
	raw := []upstreamSlashCommand{
		{
			Info:        json.RawMessage(`{"name":"demo-skill","type":"SLASH_COMMAND_TYPE_SKILL","icon":"code_blocks","definitionPath":"/tmp/skills/demo-skill/SKILL.md"}`),
			Title:       "demo-skill",
			Description: "Demo skill",
		},
	}
	opts, _ := buildSlashCommandTables(raw)
	var found *SlashCommandOption
	for i := range opts {
		if opts[i].Name == "demo-skill" {
			found = &opts[i]
			break
		}
	}
	if found == nil {
		t.Fatal("demo-skill missing from options")
	}
	if found.DefinitionPath != "/tmp/skills/demo-skill/SKILL.md" {
		t.Fatalf("DefinitionPath = %q, want /tmp/skills/demo-skill/SKILL.md", found.DefinitionPath)
	}

	var ui TrajectoryUserInput
	inputJSON := `{"items":[{"item":{"slashCommand":{"info":{"name":"demo-skill","type":"SLASH_COMMAND_TYPE_SKILL","definitionPath":"/tmp/skills/demo-skill/SKILL.md"}}}},{"text":" hello"}]}`
	if err := json.Unmarshal([]byte(inputJSON), &ui); err != nil {
		t.Fatalf("unmarshal TrajectoryUserInput: %v", err)
	}
	_, mentions := ui.ExtractTextAndSlashCommands()
	if len(mentions) != 1 || mentions[0].DefinitionPath != "/tmp/skills/demo-skill/SKILL.md" {
		t.Fatalf("mentions = %+v, want definitionPath preserved", mentions)
	}
}

func TestV234_McpAppResourceBridgeAndFileContentRouting(t *testing.T) {
	var calls []string
	var resourceCalls int
	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetMcpAppResource"):
			calls = append(calls, "GetMcpAppResource:"+stringField(body["serverName"]))
			resourceCalls++
			if resourceCalls == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"code":"unavailable","message":"server not started"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"contents":[{"uri":"ui://weather-app/card","mimeType":"text/html","text":"<h1>Sunny</h1>"}]}`))
		case strings.HasSuffix(r.URL.Path, "/LaunchMcpApp"):
			calls = append(calls, "LaunchMcpApp:"+stringField(body["serverName"]))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	// 1. /api/v1/files/content?uri=ui://weather-app/card auto-infers serverName, auto-launches, and returns HTML content.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/content?uri=ui://weather-app/card", nil)
	rec := httptest.NewRecorder()
	p.HandleFileContent(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HandleFileContent ui:// status=%d body=%s", rec.Code, rec.Body.String())
	}
	var fc FileContentResult
	if err := json.Unmarshal(rec.Body.Bytes(), &fc); err != nil {
		t.Fatalf("unmarshal FileContentResult: %v", err)
	}
	if fc.Content != "<h1>Sunny</h1>" {
		t.Fatalf("content = %q, want <h1>Sunny</h1>", fc.Content)
	}
	if got := strings.Join(calls, ","); got != "GetMcpAppResource:weather-app,LaunchMcpApp:weather-app,GetMcpAppResource:weather-app" {
		t.Fatalf("unexpected RPC sequence: %s", got)
	}

	// 2. /gateway/mcp/app/resource also normalizes content field for direct callers.
	req2 := httptest.NewRequest(http.MethodPost, "/gateway/mcp/app/resource", strings.NewReader(`{"uri":"ui://weather-app/card"}`))
	rec2 := httptest.NewRecorder()
	p.HandleMcpApp(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("HandleMcpApp resource status=%d body=%s", rec2.Code, rec2.Body.String())
	}
	var m map[string]interface{}
	if err := json.Unmarshal(rec2.Body.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal resource resp: %v", err)
	}
	if m["content"] != "<h1>Sunny</h1>" {
		t.Fatalf("normalized content = %v, want <h1>Sunny</h1>", m["content"])
	}
}

func stringField(v interface{}) string {
	s, _ := v.(string)
	return s
}

func TestV234_GitStatusCacheAndInvalidation(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("init")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	run("commit", "-m", "init")

	invalidateGitStatusCache(dir)
	defer invalidateGitStatusCache(dir)

	st1, err := gitStatusCached(context.Background(), dir, false)
	if err != nil {
		t.Fatalf("gitStatusCached 1: %v", err)
	}
	if !st1.Clean {
		t.Fatalf("expected clean repo, got %+v", st1.Files)
	}

	// Create a new file on disk; cached status should still report clean within TTL.
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st2, err := gitStatusCached(context.Background(), dir, false)
	if err != nil {
		t.Fatalf("gitStatusCached 2: %v", err)
	}
	if !st2.Clean {
		t.Fatalf("expected cached clean response within TTL, got %+v", st2.Files)
	}

	// Invalidating cache immediately observes b.txt.
	invalidateGitStatusCache(dir)
	st3, err := gitStatusCached(context.Background(), dir, false)
	if err != nil {
		t.Fatalf("gitStatusCached 3: %v", err)
	}
	if st3.Clean || len(st3.Files) != 1 || st3.Files[0].Path != "b.txt" {
		t.Fatalf("expected b.txt after invalidation, got %+v", st3.Files)
	}
}
