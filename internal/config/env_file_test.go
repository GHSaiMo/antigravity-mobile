package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateEnvVariables(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MULTIGRAVITY_DATA_DIR", tmpDir)

	initialContent := `# Multigravity Config
MULTIGRAVITY_PORT=58900
# APNS_RELAY_URL=
# CF_INVITE_CODE=
`
	envFile := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(envFile, []byte(initialContent), 0600); err != nil {
		t.Fatalf("failed to write initial env: %v", err)
	}

	updates := map[string]string{
		"APNS_RELAY_URL":    "https://push.example.com",
		"MULTIGRAVITY_PORT": "59000",
		"CF_WORKER_URL":     "https://custom.worker.dev",
	}

	path, err := UpdateEnvVariables(updates)
	if err != nil {
		t.Fatalf("UpdateEnvVariables failed: %v", err)
	}
	if path != envFile {
		t.Fatalf("expected path %q, got %q", envFile, path)
	}

	updatedBytes, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("failed to read updated env: %v", err)
	}
	content := string(updatedBytes)

	// Check APNS_RELAY_URL was uncommented and updated
	if !strings.Contains(content, "APNS_RELAY_URL=https://push.example.com") {
		t.Errorf("APNS_RELAY_URL was not updated correctly:\n%s", content)
	}

	// Check MULTIGRAVITY_PORT was updated
	if !strings.Contains(content, "MULTIGRAVITY_PORT=59000") {
		t.Errorf("MULTIGRAVITY_PORT was not updated correctly:\n%s", content)
	}

	// Check CF_WORKER_URL was appended
	if !strings.Contains(content, "CF_WORKER_URL=https://custom.worker.dev") {
		t.Errorf("CF_WORKER_URL was not appended correctly:\n%s", content)
	}

	// Check runtime env was set
	if os.Getenv("APNS_RELAY_URL") != "https://push.example.com" {
		t.Errorf("runtime env APNS_RELAY_URL not set")
	}
}
