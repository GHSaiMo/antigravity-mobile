package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureAdminTokenNoopWhenEnvSet(t *testing.T) {
	t.Setenv("MULTIGRAVITY_ADMIN_TOKEN", "from-env")
	path, generated, err := EnsureAdminToken(true)
	if err != nil {
		t.Fatal(err)
	}
	if path != "" || generated {
		t.Fatalf("expected env token to win, path=%q generated=%v", path, generated)
	}
	if os.Getenv("MULTIGRAVITY_ADMIN_TOKEN") != "from-env" {
		t.Fatalf("env token mutated")
	}
}

func TestEnsureAdminTokenGeneratesWhenTunnelOn(t *testing.T) {
	t.Setenv("MULTIGRAVITY_ADMIN_TOKEN", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	// ResolvePath uses UserHomeDir which reads HOME on Unix.
	path, generated, err := EnsureAdminToken(true)
	if err != nil {
		t.Fatal(err)
	}
	if !generated {
		t.Fatal("expected a new token to be generated")
	}
	want := DefaultAdminTokenPath()
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	tok := os.Getenv("MULTIGRAVITY_ADMIN_TOKEN")
	if len(tok) < 32 {
		t.Fatalf("generated token too short: %q", tok)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); len(got) < 32 {
		t.Fatalf("file token missing: %q", got)
	}

	t.Setenv("MULTIGRAVITY_ADMIN_TOKEN", "")
	path2, generated2, err := EnsureAdminToken(true)
	if err != nil {
		t.Fatal(err)
	}
	if generated2 {
		t.Fatal("second call should load existing file, not generate")
	}
	if path2 != path {
		t.Fatalf("path2 = %q, want %q", path2, path)
	}
	if os.Getenv("MULTIGRAVITY_ADMIN_TOKEN") != tok {
		t.Fatalf("reloaded token mismatch")
	}
}

func TestEnsureAdminTokenStandardLocation(t *testing.T) {
	t.Setenv("MULTIGRAVITY_ADMIN_TOKEN", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create token in standard ~/.multigravity/admin_token
	multigravityDir := filepath.Join(home, ".multigravity")
	if err := os.MkdirAll(multigravityDir, 0700); err != nil {
		t.Fatal(err)
	}
	stdToken := "std-admin-token-1234567890123456"
	stdPath := filepath.Join(multigravityDir, "admin_token")
	if err := os.WriteFile(stdPath, []byte(stdToken+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	path, generated, err := EnsureAdminToken(true)
	if err != nil {
		t.Fatal(err)
	}
	if generated {
		t.Fatal("expected standard token to be reused, not generated")
	}
	if path != stdPath {
		t.Fatalf("path = %q, want %q", path, stdPath)
	}
	if os.Getenv("MULTIGRAVITY_ADMIN_TOKEN") != stdToken {
		t.Fatalf("env MULTIGRAVITY_ADMIN_TOKEN = %q, want %q", os.Getenv("MULTIGRAVITY_ADMIN_TOKEN"), stdToken)
	}
}
