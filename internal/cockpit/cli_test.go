package cockpit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateSecureToken(t *testing.T) {
	tok1 := GenerateSecureToken()
	tok2 := GenerateSecureToken()
	if len(tok1) != 32 {
		t.Fatalf("expected token length 32, got %d", len(tok1))
	}
	if len(tok2) != 32 {
		t.Fatalf("expected token length 32, got %d", len(tok2))
	}
	if tok1 == tok2 {
		t.Fatalf("tokens should be random and distinct")
	}
}

func TestRedactTokenDisplay(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"", "(空)"},
		{"change-this-token", "change-this-token"},
		{"12345", "********"},
		{"abcdef1234567890", "abc...890"},
	}

	for _, c := range cases {
		got := redactTokenDisplay(c.input)
		if got != c.expected {
			t.Errorf("redactTokenDisplay(%q) = %q; want %q", c.input, got, c.expected)
		}
	}
}

func TestSaveCockpitReportSettingsAndStatus(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("COCKPIT_DATA_DIR", tmpDir)

	// Initially no config file
	status := CheckCockpitConfigStatus()
	if status.Configured {
		t.Fatalf("expected not configured when config.json is absent")
	}

	// Write empty config
	configFile := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configFile, []byte(`{"ws_port": 19528}`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Status with report_enabled = false
	status = CheckCockpitConfigStatus()
	if status.Configured {
		t.Fatalf("expected not configured when report_enabled is false")
	}

	// Save settings with custom token
	token := "my-secret-test-token-1234"
	if err := SaveCockpitReportSettings(true, 18081, token); err != nil {
		t.Fatalf("SaveCockpitReportSettings failed: %v", err)
	}

	cfg, err := getCockpitConfig()
	if err != nil {
		t.Fatalf("getCockpitConfig failed: %v", err)
	}
	if !cfg.ReportEnabled {
		t.Fatalf("expected ReportEnabled to be true")
	}
	if cfg.ReportPort != 18081 {
		t.Fatalf("expected ReportPort 18081, got %d", cfg.ReportPort)
	}
	if cfg.ReportToken != token {
		t.Fatalf("expected ReportToken %q, got %q", token, cfg.ReportToken)
	}

	// Status should now be configured
	status = CheckCockpitConfigStatus()
	if !status.Configured {
		t.Fatalf("expected configured to be true, got reason: %s", status.Reason)
	}
	if status.IsDefault {
		t.Fatalf("token should not be default")
	}

	// If token is change-this-token
	if err := SaveCockpitReportSettings(true, 18081, "change-this-token"); err != nil {
		t.Fatalf("save default token: %v", err)
	}
	status = CheckCockpitConfigStatus()
	if status.Configured {
		t.Fatalf("expected not configured when token is change-this-token")
	}
	if !status.IsDefault {
		t.Fatalf("expected IsDefault to be true")
	}
}
