package proxy

import (
	"encoding/json"
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
