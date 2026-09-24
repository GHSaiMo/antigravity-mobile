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
			input:    "https://api.day.app/mock_test_device_key_abc123/自定义推送图标（需iOS15或以上）?icon=https://day.app/assets/images/avatar.jpg",
			expected: "https://api.day.app/mock_test_device_key_abc123",
		},
		{
			name:     "Standard URL with trailing slash",
			input:    "https://api.day.app/mock_test_device_key_abc123/",
			expected: "https://api.day.app/mock_test_device_key_abc123",
		},
		{
			name:     "Standard URL without trailing slash",
			input:    "https://api.day.app/mock_test_device_key_abc123",
			expected: "https://api.day.app/mock_test_device_key_abc123",
		},
		{
			name:     "Bare key only",
			input:    "mock_test_device_key_abc123",
			expected: "https://api.day.app/mock_test_device_key_abc123",
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
		{
			name:     "Placeholder key from env.example",
			input:    "https://api.day.app/YOUR_DEVICE_KEY/",
			expected: "",
		},
		{
			name:     "Bare placeholder key",
			input:    "YOUR_DEVICE_KEY",
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
BARK_URL="https://api.day.app/mock_test_device_key_abc123/"
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
	if cfg.BarkEndpoint != "https://api.day.app/mock_test_device_key_abc123" {
		t.Errorf("unexpected endpoint: %s", cfg.BarkEndpoint)
	}
	if cfg.Group != "MyAntigravity" {
		t.Errorf("unexpected group: %s", cfg.Group)
	}
	if cfg.IconURL != DefaultAntigravityIcon {
		t.Errorf("unexpected icon: %s", cfg.IconURL)
	}
}


func TestAdvertisePublicIPv6(t *testing.T) {
	t.Setenv("INCLUDE_PUBLIC_IPV6", "")
	if !AdvertisePublicIPv6(false) {
		t.Errorf("expected true when unset by default")
	}
	if !AdvertisePublicIPv6(true) {
		t.Errorf("expected true when ssl enabled")
	}
	t.Setenv("INCLUDE_PUBLIC_IPV6", "0")
	if AdvertisePublicIPv6(false) {
		t.Errorf("expected false when INCLUDE_PUBLIC_IPV6=0")
	}
	t.Setenv("INCLUDE_PUBLIC_IPV6", "false")
	if AdvertisePublicIPv6(false) {
		t.Errorf("expected false when INCLUDE_PUBLIC_IPV6=false")
	}
	t.Setenv("INCLUDE_PUBLIC_IPV6", "1")
	if !AdvertisePublicIPv6(false) {
		t.Errorf("expected true when INCLUDE_PUBLIC_IPV6=1")
	}
}

func TestRedactBarkEndpoint(t *testing.T) {
	got := RedactBarkEndpoint("https://api.day.app/supersecretkey123/")
	if got != "https://api.day.app/***" {
		t.Errorf("unexpected redaction: %s", got)
	}
	if RedactBarkEndpoint("") != "" {
		t.Errorf("empty should stay empty")
	}
}



func TestGetCloudflareConfig(t *testing.T) {
	// 1. Test defaults
	t.Setenv("CF_EDGE_IP_VERSION", "")
	t.Setenv("TUNNEL_EDGE_IP_VERSION", "")
	t.Setenv("CF_PROTOCOL", "")
	t.Setenv("TUNNEL_TRANSPORT_PROTOCOL", "")
	t.Setenv("CF_REGION", "")
	t.Setenv("TUNNEL_REGION", "")

	cfg := GetCloudflareConfig()
	if cfg.EdgeIPVersion != "4" {
		t.Errorf("expected default EdgeIPVersion = 4, got %s", cfg.EdgeIPVersion)
	}
	if cfg.Protocol != "http2" {
		t.Errorf("expected default Protocol = http2, got %s", cfg.Protocol)
	}
	if !cfg.Enabled {
		t.Errorf("expected default Enabled = true")
	}

	// 2. Test overrides
	t.Setenv("CF_EDGE_IP_VERSION", "6")
	t.Setenv("CF_PROTOCOL", "quic")
	t.Setenv("CF_REGION", "us")

	cfgOverridden := GetCloudflareConfig()
	if cfgOverridden.EdgeIPVersion != "6" {
		t.Errorf("expected overridden EdgeIPVersion = 6, got %s", cfgOverridden.EdgeIPVersion)
	}
	if cfgOverridden.Protocol != "quic" {
		t.Errorf("expected overridden Protocol = quic, got %s", cfgOverridden.Protocol)
	}
	if cfgOverridden.Region != "us" {
		t.Errorf("expected overridden Region = us, got %s", cfgOverridden.Region)
	}
}


