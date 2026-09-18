package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureAdminTokenNoopWhenEnvSet(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "from-env")
	path, generated, err := EnsureAdminToken(true)
	if err != nil {
		t.Fatal(err)
	}
	if path != "" || generated {
		t.Fatalf("expected env token to win, path=%q generated=%v", path, generated)
	}
	if os.Getenv("ADMIN_TOKEN") != "from-env" {
		t.Fatalf("env token mutated")
	}
}

func TestEnsureAdminTokenGeneratesWhenTunnelOn(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "")
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
	tok := os.Getenv("ADMIN_TOKEN")
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

	t.Setenv("ADMIN_TOKEN", "")
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
	if os.Getenv("ADMIN_TOKEN") != tok {
		t.Fatalf("reloaded token mismatch")
	}
}

func TestEnsureAdminTokenLegacyFallback(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Create legacy token in ~/.antigravity-mobile/admin_token
	legacyDir := filepath.Join(home, ".antigravity-mobile")
	if err := os.MkdirAll(legacyDir, 0700); err != nil {
		t.Fatal(err)
	}
	legacyToken := "legacy-admin-token-1234567890123456"
	legacyPath := filepath.Join(legacyDir, "admin_token")
	if err := os.WriteFile(legacyPath, []byte(legacyToken+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	path, generated, err := EnsureAdminToken(true)
	if err != nil {
		t.Fatal(err)
	}
	if generated {
		t.Fatal("expected legacy token to be reused, not generated")
	}
	if path != legacyPath {
		t.Fatalf("path = %q, want %q", path, legacyPath)
	}
	if os.Getenv("ADMIN_TOKEN") != legacyToken {
		t.Fatalf("env ADMIN_TOKEN = %q, want %q", os.Getenv("ADMIN_TOKEN"), legacyToken)
	}
}
