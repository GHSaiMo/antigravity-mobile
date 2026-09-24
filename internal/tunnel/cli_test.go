package tunnel

import (
	"os"
	"path/filepath"
	"testing"

	"antigravity-mobile/internal/config"
)

func TestCloudflareCLIStatusAndReset(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("MULTIGRAVITY_DATA_DIR", tmpDir)

	// Test runCloudflareStatusCmd without crash
	runCloudflareStatusCmd()

	// Write mock tunnel cache
	cachePath := filepath.Join(tmpDir, "cf_tunnel.json")
	if err := os.WriteFile(cachePath, []byte(`{"url":"https://test.mgy","token":"mock"}`), 0600); err != nil {
		t.Fatalf("failed to write mock cache: %v", err)
	}

	// Status should show cache
	runCloudflareStatusCmd()

	// Reset cache
	runCloudflareResetCmd()

	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("expected cache file to be removed after reset")
	}

	// Test enable/disable
	runCloudflareDisableCmd()
	cfg := config.GetCloudflareConfig()
	if cfg.Enabled {
		t.Fatalf("expected cloudflare tunnel to be disabled")
	}

	runCloudflareEnableCmd()
	cfg = config.GetCloudflareConfig()
	if !cfg.Enabled {
		t.Fatalf("expected cloudflare tunnel to be enabled")
	}
}
