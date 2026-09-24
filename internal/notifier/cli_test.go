package notifier

import (
	"os"
	"path/filepath"
	"testing"

	"antigravity-mobile/internal/config"
)

func TestBarkCLIStatusAndSet(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MULTIGRAVITY_DATA_DIR", tmpDir)
	t.Setenv("BARK_URL", "")
	t.Setenv("BARK_ENABLE", "")

	// Test runBarkStatusCmd without crash
	runBarkStatusCmd()

	// Update bark key
	testKey := "abcdef123456"
	updates := map[string]string{
		"BARK_URL":    "https://api.day.app/" + testKey,
		"BARK_ENABLE": "1",
	}
	envPath, err := config.UpdateEnvVariables(updates)
	if err != nil {
		t.Fatalf("UpdateEnvVariables failed: %v", err)
	}

	if _, err := os.Stat(envPath); err != nil {
		t.Fatalf("expected env file to exist at %s", envPath)
	}

	cfg := config.GetNotificationConfig()
	if cfg.BarkEndpoint != "https://api.day.app/"+testKey {
		t.Fatalf("expected endpoint %s, got %s", "https://api.day.app/"+testKey, cfg.BarkEndpoint)
	}
	if !cfg.Enabled {
		t.Fatalf("expected enabled to be true")
	}

	// Read content
	content, _ := os.ReadFile(filepath.Join(tmpDir, ".env"))
	if len(content) == 0 {
		t.Fatalf("expected non-empty .env file")
	}
}
