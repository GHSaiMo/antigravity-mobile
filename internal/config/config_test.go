package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env")
	content := `
# Comment line
APNS_RELAY_URL="https://custom-relay.example.com/"
NOTIFICATION_GROUP='MyAntigravity'
FCM_SERVER_KEY=test_fcm_key
FCM_DEVICE_TOKEN=test_fcm_token
`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp .env: %v", err)
	}

	t.Setenv("APNS_RELAY_URL", "")
	t.Setenv("NOTIFICATION_GROUP", "")
	t.Setenv("FCM_SERVER_KEY", "")
	t.Setenv("FCM_DEVICE_TOKEN", "")
	t.Setenv("FCM_ENABLE", "")
	t.Setenv("NOTIFICATION_ENABLE", "")

	LoadDotEnv(envPath)

	cfg := GetNotificationConfig()
	if !cfg.Enabled {
		t.Errorf("expected cfg.Enabled = true, got false")
	}
	if cfg.APNSRelayURL != "https://custom-relay.example.com" {
		t.Errorf("unexpected APNSRelayURL: %s", cfg.APNSRelayURL)
	}
	if cfg.Group != "MyAntigravity" {
		t.Errorf("unexpected group: %s", cfg.Group)
	}
	if cfg.IconURL != DefaultAntigravityIcon {
		t.Errorf("unexpected icon: %s", cfg.IconURL)
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
	t.Setenv("CF_DNS_RESOLVERS", "")
	t.Setenv("TUNNEL_DNS_RESOLVER_ADDRS", "")

	cfg := GetCloudflareConfig()
	if cfg.EdgeIPVersion != "auto" {
		t.Errorf("expected default EdgeIPVersion = auto, got %s", cfg.EdgeIPVersion)
	}
	if cfg.Protocol != "http2" {
		t.Errorf("expected default Protocol = http2, got %s", cfg.Protocol)
	}
	if !cfg.Enabled {
		t.Errorf("expected default Enabled = true")
	}
	if cfg.DNSResolvers != "" {
		t.Errorf("expected default DNSResolvers = '', got %s", cfg.DNSResolvers)
	}

	// 2. Test overrides
	t.Setenv("CF_EDGE_IP_VERSION", "4")
	t.Setenv("CF_PROTOCOL", "quic")
	t.Setenv("CF_REGION", "us")
	t.Setenv("CF_DNS_RESOLVERS", "1.1.1.1:53")

	cfgOverridden := GetCloudflareConfig()
	if cfgOverridden.EdgeIPVersion != "4" {
		t.Errorf("expected overridden EdgeIPVersion = 4, got %s", cfgOverridden.EdgeIPVersion)
	}
	if cfgOverridden.Protocol != "quic" {
		t.Errorf("expected overridden Protocol = quic, got %s", cfgOverridden.Protocol)
	}
	if cfgOverridden.Region != "us" {
		t.Errorf("expected overridden Region = us, got %s", cfgOverridden.Region)
	}
	if cfgOverridden.DNSResolvers != "1.1.1.1:53" {
		t.Errorf("expected overridden DNSResolvers = 1.1.1.1:53, got %s", cfgOverridden.DNSResolvers)
	}
}

func TestGetEnableIPv6(t *testing.T) {
	t.Setenv("MULTIGRAVITY_ENABLE_IPV6", "")
	t.Setenv("MGY_ENABLE_IPV6", "")
	if GetEnableIPv6() {
		t.Fatalf("expected default GetEnableIPv6() = false")
	}

	t.Setenv("MGY_ENABLE_IPV6", "1")
	if !GetEnableIPv6() {
		t.Fatalf("expected MGY_ENABLE_IPV6=1 to enable IPv6")
	}

	t.Setenv("MULTIGRAVITY_ENABLE_IPV6", "false")
	if GetEnableIPv6() {
		t.Fatalf("expected MULTIGRAVITY_ENABLE_IPV6=false to take precedence over MGY_ENABLE_IPV6")
	}

	t.Setenv("MULTIGRAVITY_ENABLE_IPV6", "true")
	if !GetEnableIPv6() {
		t.Fatalf("expected MULTIGRAVITY_ENABLE_IPV6=true to enable IPv6")
	}
}

func TestGetTLSConfig(t *testing.T) {
	t.Setenv("MGY_TLS_CERT", "")
	t.Setenv("MULTIGRAVITY_TLS_CERT", "")
	t.Setenv("MGY_TLS_KEY", "")
	t.Setenv("MULTIGRAVITY_TLS_KEY", "")
	t.Setenv("MGY_TLS_PORT", "")
	t.Setenv("MULTIGRAVITY_TLS_PORT", "")

	def := GetTLSConfig()
	if def.Enabled() || def.Port != DefaultTLSPort {
		t.Fatalf("unexpected default TLSConfig: %+v", def)
	}

	t.Setenv("MULTIGRAVITY_TLS_CERT", "/etc/certs/fullchain.pem")
	t.Setenv("MULTIGRAVITY_TLS_KEY", "/etc/certs/privkey.pem")
	t.Setenv("MULTIGRAVITY_TLS_PORT", "59443")
	cfg := GetTLSConfig()
	if !cfg.Enabled() || cfg.CertFile != "/etc/certs/fullchain.pem" || cfg.KeyFile != "/etc/certs/privkey.pem" || cfg.Port != 59443 {
		t.Fatalf("unexpected TLSConfig from MULTIGRAVITY_*: %+v", cfg)
	}

	t.Setenv("MGY_TLS_CERT", "/tmp/a.crt")
	t.Setenv("MGY_TLS_KEY", "/tmp/a.key")
	t.Setenv("MGY_TLS_PORT", "443")
	cfg2 := GetTLSConfig()
	if cfg2.CertFile != "/tmp/a.crt" || cfg2.KeyFile != "/tmp/a.key" || cfg2.Port != 443 {
		t.Fatalf("expected MGY_TLS_* precedence, got: %+v", cfg2)
	}
}


