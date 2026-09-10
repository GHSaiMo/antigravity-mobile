package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeBarkEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "User provided full example URL with query and Chinese path",
			input:    "https://api.day.app/8CTWiePgCJqiZNsJBaaDX9/自定义推送图标（需iOS15或以上）?icon=https://day.app/assets/images/avatar.jpg",
			expected: "https://api.day.app/8CTWiePgCJqiZNsJBaaDX9",
		},
		{
			name:     "Standard URL with trailing slash",
			input:    "https://api.day.app/8CTWiePgCJqiZNsJBaaDX9/",
			expected: "https://api.day.app/8CTWiePgCJqiZNsJBaaDX9",
		},
		{
			name:     "Standard URL without trailing slash",
			input:    "https://api.day.app/8CTWiePgCJqiZNsJBaaDX9",
			expected: "https://api.day.app/8CTWiePgCJqiZNsJBaaDX9",
		},
		{
			name:     "Bare key only",
			input:    "8CTWiePgCJqiZNsJBaaDX9",
			expected: "https://api.day.app/8CTWiePgCJqiZNsJBaaDX9",
		},
		{
			name:     "Self-hosted server with key",
			input:    "https://bark.example.com/customkey123",
			expected: "https://bark.example.com/customkey123",
		},
		{
			name:     "Empty input",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeBarkEndpoint(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeBarkEndpoint(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLoadDotEnv(t *testing.T) {
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env")
	content := `
# Comment line
BARK_URL="https://api.day.app/8CTWiePgCJqiZNsJBaaDX9/"
BARK_GROUP='MyAntigravity'
BARK_SOUND_ACTION=alarm
`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp .env: %v", err)
	}

	os.Unsetenv("BARK_URL")
	os.Unsetenv("BARK_GROUP")
	os.Unsetenv("BARK_SOUND_ACTION")

	LoadDotEnv(envPath)

	cfg := GetNotificationConfig()
	if !cfg.Enabled {
		t.Errorf("expected cfg.Enabled = true, got false")
	}
	if cfg.BarkEndpoint != "https://api.day.app/8CTWiePgCJqiZNsJBaaDX9" {
		t.Errorf("unexpected endpoint: %s", cfg.BarkEndpoint)
	}
	if cfg.Group != "MyAntigravity" {
		t.Errorf("unexpected group: %s", cfg.Group)
	}
	if cfg.IconURL != DefaultAntigravityIcon {
		t.Errorf("unexpected icon: %s", cfg.IconURL)
	}
}
