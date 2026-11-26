package proxy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAntigravityPathsCLIFallback(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("ANTIGRAVITY_APP_DATA_DIR", "")
	setActiveAntigravityDataDir("")
	t.Cleanup(func() { setActiveAntigravityDataDir("") })

	cliRoot := filepath.Join(tmpHome, ".gemini", "antigravity-cli")
	if err := os.MkdirAll(filepath.Join(cliRoot, "brain", "cid-cli-1"), 0700); err != nil {
		t.Fatalf("mkdir cli brain: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(cliRoot, "annotations"), 0700); err != nil {
		t.Fatalf("mkdir cli annotations: %v", err)
	}
	cliDB := filepath.Join(cliRoot, "conversation_summaries.db")
	if err := os.WriteFile(cliDB, []byte("mock-db"), 0600); err != nil {
		t.Fatalf("write cli db: %v", err)
	}
	cliPlan := filepath.Join(cliRoot, "brain", "cid-cli-1", "implementation_plan.md")
	if err := os.WriteFile(cliPlan, []byte("# Plan"), 0600); err != nil {
		t.Fatalf("write cli plan: %v", err)
	}
	cliAnn := filepath.Join(cliRoot, "annotations", "cid-cli-1.pbtxt")
	if err := os.WriteFile(cliAnn, []byte("title: \"CLI Session\"\n"), 0600); err != nil {
		t.Fatalf("write cli annotation: %v", err)
	}

	// Only ~/.gemini/antigravity-cli exists -> all resolvers fall back to it.
	if got := resolveAntigravityHomeDir(); got != cliRoot {
		t.Fatalf("resolveAntigravityHomeDir() = %q, want %q", got, cliRoot)
	}
	if got := getAntigravityDBPath(); got != cliDB {
		t.Fatalf("getAntigravityDBPath() = %q, want %q", got, cliDB)
	}
	if got := readAnnotationTitle("cid-cli-1"); got != "CLI Session" {
		t.Fatalf("readAnnotationTitle(cid-cli-1) = %q, want %q", got, "CLI Session")
	}

	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)
	if got, err := resolveLocalFilePathExact("/static/artifacts/cid-cli-1/implementation_plan.md", "cid-cli-1"); err != nil || got != cliPlan {
		t.Fatalf("resolveLocalFilePathExact(static artifact) = (%q, %v), want (%q, nil)", got, err, cliPlan)
	}
	if got, err := resolveLocalFilePathExact("implementation_plan.md", "cid-cli-1"); err != nil || got != cliPlan {
		t.Fatalf("resolveLocalFilePathExact(bare filename) = (%q, %v), want (%q, nil)", got, err, cliPlan)
	}

	// When empty ~/.gemini/antigravity also exists, resolveAntigravityHomeDir prefers it,
	// while resolveAntigravitySubPath still finds existing CLI files seamlessly.
	desktopRoot := filepath.Join(tmpHome, ".gemini", "antigravity")
	if err := os.MkdirAll(desktopRoot, 0700); err != nil {
		t.Fatalf("mkdir desktop root: %v", err)
	}
	if got := resolveAntigravityHomeDir(); got != desktopRoot {
		t.Fatalf("resolveAntigravityHomeDir() with both dirs = %q, want %q", got, desktopRoot)
	}
	if got := getAntigravityDBPath(); got != cliDB {
		t.Fatalf("getAntigravityDBPath() with empty desktop dir = %q, want CLI fallback %q", got, cliDB)
	}

	// When both have conversation_summaries.db, active discovered AppDataDir takes precedence.
	desktopDB := filepath.Join(desktopRoot, "conversation_summaries.db")
	if err := os.WriteFile(desktopDB, []byte("desktop-db"), 0600); err != nil {
		t.Fatalf("write desktop db: %v", err)
	}
	if got := getAntigravityDBPath(); got != desktopDB {
		t.Fatalf("getAntigravityDBPath() with both dbs = %q, want desktop %q", got, desktopDB)
	}
	setActiveAntigravityDataDir(cliRoot)
	if got := getAntigravityDBPath(); got != cliDB {
		t.Fatalf("getAntigravityDBPath() with active CLI upstream = %q, want CLI %q", got, cliDB)
	}

	// Explicit ANTIGRAVITY_APP_DATA_DIR under $HOME overrides desktop/CLI defaults,
	// while an inherited ANTIGRAVITY_APP_DATA_DIR outside $HOME is ignored.
	outsideRoot := t.TempDir()
	t.Setenv("ANTIGRAVITY_APP_DATA_DIR", outsideRoot)
	if got := resolveAntigravityHomeDir(); got != cliRoot {
		t.Fatalf("resolveAntigravityHomeDir() with outside ANTIGRAVITY_APP_DATA_DIR = %q, want active CLI %q", got, cliRoot)
	}
	customRoot := filepath.Join(tmpHome, "custom-agy-data")
	if err := os.MkdirAll(customRoot, 0700); err != nil {
		t.Fatalf("mkdir custom root: %v", err)
	}
	t.Setenv("ANTIGRAVITY_APP_DATA_DIR", customRoot)
	if got := resolveAntigravityHomeDir(); got != customRoot {
		t.Fatalf("resolveAntigravityHomeDir() with $HOME-scoped env = %q, want %q", got, customRoot)
	}

	// MGY_ANTIGRAVITY_DATA_DIR overrides unconditionally (even outside $HOME).
	t.Setenv("MGY_ANTIGRAVITY_DATA_DIR", outsideRoot)
	if got := resolveAntigravityHomeDir(); got != outsideRoot {
		t.Fatalf("resolveAntigravityHomeDir() with MGY_ANTIGRAVITY_DATA_DIR = %q, want %q", got, outsideRoot)
	}
}

func TestAntigravityCLISensitivePathsAndAllowlist(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("ALLOWED_WORKSPACE_ROOTS", filepath.Join(tmpHome, "only-this-workspace"))
	t.Setenv("MULTIGRAVITY_ALLOWED_WORKSPACE_ROOTS", "")
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	// Artifact under ~/.gemini/antigravity-cli/brain must remain readable even with explicit ALLOWED_WORKSPACE_ROOTS.
	safeArtifact := filepath.Join(tmpHome, ".gemini", "antigravity-cli", "brain", "cid-1", "walkthrough.md")
	if err := os.MkdirAll(filepath.Dir(safeArtifact), 0700); err != nil {
		t.Fatalf("mkdir safe artifact: %v", err)
	}
	if err := os.WriteFile(safeArtifact, []byte("ok"), 0600); err != nil {
		t.Fatalf("write safe artifact: %v", err)
	}
	if !IsSafeReadableFilePath(safeArtifact) {
		t.Fatalf("expected %q to be readable under ~/.gemini/antigravity-cli", safeArtifact)
	}

	// Sensitive CLI files and directories must always be blocked.
	blocked := []string{
		filepath.Join(tmpHome, ".gemini", "antigravity-cli", "daemon", "ls_123.json"),
		filepath.Join(tmpHome, ".gemini", "antigravity-cli", "presence", "session.lock"),
		filepath.Join(tmpHome, ".gemini", "antigravity-cli", "mcp_config.json"),
		filepath.Join(tmpHome, ".gemini", "antigravity-cli", "settings.json"),
		filepath.Join(tmpHome, ".gemini", "antigravity-cli", "antigravity_state.pbtxt"),
		filepath.Join(tmpHome, ".gemini", "antigravity-cli", "agyhub_summaries_proto.pb"),
		filepath.Join(tmpHome, ".gemini", "antigravity-cli", "antigravity-oauth-token"),
		filepath.Join(tmpHome, ".gemini", "antigravity-oauth-token"),
	}
	for _, p := range blocked {
		if IsSafeFilePath(p) || IsSafeReadableFilePath(p) {
			t.Errorf("expected sensitive CLI path %q to be blocked", p)
		}
	}
}
