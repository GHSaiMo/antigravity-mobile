package auth

import (
	"strings"
	"testing"
	"time"
)

func TestPairingManager_GenerateAndConsume(t *testing.T) {
	pm := NewPairingManager()

	session, err := pm.GenerateSession(5 * time.Minute)
	if err != nil {
		t.Fatalf("failed to generate session: %v", err)
	}

	if len(session.Code) != 64 {
		t.Errorf("expected 64 hex characters, got %d", len(session.Code))
	}

	// Validate and consume
	if !pm.ValidateAndConsume(session.Code) {
		t.Fatalf("expected pairing code to be valid and consumed")
	}

	// Double consume should fail (anti-replay)
	if pm.ValidateAndConsume(session.Code) {
		t.Fatalf("expected pairing code to be consumed already")
	}
}

func TestPairingManager_Expiration(t *testing.T) {
	pm := NewPairingManager()

	// Create already expired session
	session, err := pm.GenerateSession(-1 * time.Second)
	if err != nil {
		t.Fatalf("failed to generate session: %v", err)
	}

	if !session.IsExpired() {
		t.Errorf("expected session to be expired")
	}

	if pm.ValidateAndConsume(session.Code) {
		t.Errorf("expected expired session code validation to fail")
	}
}

func TestGenerateDeviceCredentials(t *testing.T) {
	devID, devToken, err := GenerateDeviceCredentials()
	if err != nil {
		t.Fatalf("failed to generate credentials: %v", err)
	}

	if !strings.HasPrefix(devID, "dev_") {
		t.Errorf("expected device ID to start with dev_, got %s", devID)
	}
	if !strings.HasPrefix(devToken, "tok_") {
		t.Errorf("expected device token to start with tok_, got %s", devToken)
	}
}

func TestGeneratePairingURI(t *testing.T) {
	uri := GeneratePairingURI("mac.example.com", 58900, "abc123code", true)
	if !strings.HasPrefix(uri, "agy://pair?") {
		t.Errorf("expected agy://pair? prefix, got %s", uri)
	}
	if !strings.Contains(uri, "host=mac.example.com") {
		t.Errorf("expected host in uri, got %s", uri)
	}
	if !strings.Contains(uri, "port=58900") {
		t.Errorf("expected port in uri, got %s", uri)
	}
	if !strings.Contains(uri, "ssl=1") {
		t.Errorf("expected ssl=1 in uri, got %s", uri)
	}
	if !strings.Contains(uri, "code=abc123code") {
		t.Errorf("expected code in uri, got %s", uri)
	}
}

func TestGenerateQRCodePNG(t *testing.T) {
	pngData, err := GenerateQRCodePNG("mac.example.com", 58900, "abc123code", true, 128)
	if err != nil {
		t.Fatalf("failed to generate qr png: %v", err)
	}
	if len(pngData) == 0 {
		t.Errorf("expected non-empty png bytes")
	}
}

func TestPrintPairingQRCode(t *testing.T) {
	// Ensure print does not panic with single or multiple hosts
	PrintPairingQRCode("192.168.50.9", 58900, "abc123code", false, "240e:3a1:1c3:4e80:818:6ec5:22d7:477b")
}

func TestPairingManager_CleanupAndLatest(t *testing.T) {
	pm := NewPairingManager()
	s1, err := pm.GenerateSession(10 * time.Minute)
	if err != nil {
		t.Fatalf("failed to generate session: %v", err)
	}

	latest := pm.LatestSession()
	if latest == nil || latest.Code != s1.Code {
		t.Errorf("expected latest session to match s1")
	}

	pm.CleanupExpired()
	if pm.LatestSession() == nil {
		t.Errorf("expected s1 still present after cleanup")
	}
}

func TestDetectNetworkAddresses(t *testing.T) {
	addrs := DetectNetworkAddresses()
	t.Logf("Detected LAN IPv4: %s, Public IPv6: %s", addrs.LANIPv4, addrs.PublicIPv6)
}


