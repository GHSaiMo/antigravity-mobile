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

func TestGetTunnelConfig(t *testing.T) {
	os.Setenv("FRP_SERVER_ADDR", "198.51.100.1")
	os.Setenv("FRP_SERVER_PORT", "7000")
	os.Setenv("FRP_TOKEN", "TestToken123")
	os.Setenv("FRP_REMOTE_PORT", "58900")
	os.Setenv("FRP_ENABLED", "true")
	defer func() {
		os.Unsetenv("FRP_SERVER_ADDR")
		os.Unsetenv("FRP_SERVER_PORT")
		os.Unsetenv("FRP_TOKEN")
		os.Unsetenv("FRP_REMOTE_PORT")
		os.Unsetenv("FRP_ENABLED")
	}()

	cfg := GetTunnelConfig()
	if !cfg.Enabled {
		t.Errorf("expected Enabled true, got false")
	}
	if cfg.ServerAddr != "198.51.100.1" {
		t.Errorf("expected 198.51.100.1, got %s", cfg.ServerAddr)
	}
	if cfg.ServerPort != 7000 {
		t.Errorf("expected 7000, got %d", cfg.ServerPort)
	}
	if cfg.Token != "TestToken123" {
		t.Errorf("expected TestToken123, got %s", cfg.Token)
	}
	if cfg.RemotePort != 58900 {
		t.Errorf("expected 58900, got %d", cfg.RemotePort)
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

func TestValidateFRPTokenStrength(t *testing.T) {
	if ValidateFRPTokenStrength("") == "" {
		t.Errorf("expected warning for empty token")
	}
	if ValidateFRPTokenStrength("admin") == "" {
		t.Errorf("expected warning for well-known weak token")
	}
	if ValidateFRPTokenStrength("your_frp_auth_token") == "" {
		t.Errorf("expected warning for placeholder token")
	}
	if ValidateFRPTokenStrength("short-token") == "" {
		t.Errorf("expected warning for short token (< 24 chars)")
	}
	if ValidateFRPTokenStrength("AgySecure2026Token") == "" {
		t.Errorf("expected warning for AgySecure2026Token (predictable words pattern)")
	}
	strongToken := "a1b2c3d4e5f60718293a4b5c6d7e8f90"
	if ValidateFRPTokenStrength(strongToken) != "" {
		t.Errorf("expected no warning for strong 32-hex token, got: %s", ValidateFRPTokenStrength(strongToken))
	}
}

