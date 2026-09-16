package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	// Test with extra hosts (LAN & IPv6)
	multiPngData, err := GenerateQRCodePNG("192.168.1.100", 58900, "abc123code", false, 128, "2001:db8::1", "mac.example.com")
	if err != nil {
		t.Fatalf("failed to generate multi-host qr png: %v", err)
	}
	if len(multiPngData) == 0 {
		t.Errorf("expected non-empty multi-host png bytes")
	}

	// Test GenerateMultiHostQRCodePNG directly
	directPng, err := GenerateMultiHostQRCodePNG(MultiHostPairingParams{
		PrimaryHost: "192.168.1.100",
		Port:        58900,
		Code:        "abc123code",
		LANHost:     "192.168.1.100",
		IPv6Host:    "2001:db8::1",
	}, 128)
	if err != nil {
		t.Fatalf("failed to generate direct multi-host qr png: %v", err)
	}
	if len(directPng) == 0 {
		t.Errorf("expected non-empty direct multi-host png bytes")
	}
}

func TestPrintPairingQRCode(t *testing.T) {
	// Ensure print does not panic with single or multiple hosts
	PrintPairingQRCode("192.168.50.9", 58900, "abc123code", false, "2001:db8::1")
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

func TestGenerateMultiHostPairingURI(t *testing.T) {
	uri := GenerateMultiHostPairingURI(MultiHostPairingParams{
		PrimaryHost: "192.168.1.100",
		Port:        58900,
		Code:        "testcode123",
		SSL:         false,
		LANHost:     "192.168.1.100",
		IPv6Host:    "2001:db8:abcd::1",
		DDNSHost:    "mac.example.com",
	})

	if !strings.HasPrefix(uri, "agy://pair?") {
		t.Fatalf("expected agy://pair scheme, got %s", uri)
	}
	if !strings.Contains(uri, "host=192.168.1.100") {
		t.Errorf("missing primary host in %s", uri)
	}
	if !strings.Contains(uri, "ipv6=2001%3Adb8%3Aabcd%3A%3A1") && !strings.Contains(uri, "ipv6=2001:db8:abcd::1") {
		t.Errorf("missing ipv6 in %s", uri)
	}
	if !strings.Contains(uri, "ddns=mac.example.com") {
		t.Errorf("missing ddns in %s", uri)
	}
}

func TestGenerateMultiHostPairingURI_IncludesRelay(t *testing.T) {
	uri := GenerateMultiHostPairingURI(MultiHostPairingParams{
		PrimaryHost: "192.168.50.9",
		Port:        58900,
		Code:        "testcode123",
		SSL:         false,
		LANHost:     "192.168.50.9",
		IPv6Host:    "2001:db8:abcd::1",
		RelayHost:   "124.222.226.143",
	})
	if !strings.Contains(uri, "relay=124.222.226.143") {
		t.Fatalf("expected relay host in pairing URI, got %s", uri)
	}
}

func TestAuthHandler_NewPairingSessionIncludesRelay(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "pair-admin-token")
	store, err := NewAuthStore(t.TempDir() + "/auth.json")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	pm := NewPairingManager()
	h := NewAuthHandler(store, pm, "192.168.50.9", 58900, false)
	h.SetEndpoints("192.168.50.9", "2001:db8:abcd::1", "")
	h.SetRelayURL("http://124.222.226.143:58900")

	if got := h.relayHost(); got != "124.222.226.143" {
		t.Fatalf("relayHost() = %q", got)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", nil)
	req.Header.Set("Authorization", "Bearer pair-admin-token")
	rr := httptest.NewRecorder()
	h.HandleNewPairingSession(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("session status %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(resp.URI, "relay=124.222.226.143") {
		t.Fatalf("session URI missing relay: %s", resp.URI)
	}
	if !strings.Contains(resp.URI, "ipv6=") {
		t.Fatalf("session URI missing ipv6: %s", resp.URI)
	}
}

func TestAuthHandler_GetEndpoints(t *testing.T) {
	store, _ := NewAuthStore(t.TempDir() + "/auth.json")
	pm := NewPairingManager()
	h := NewAuthHandler(store, pm, "192.168.1.50", 58900, false)
	h.SetEndpoints("192.168.1.50", "2001:db8:abcd::1", "mac.example.com")
	h.SetRelayURL("http://relay.example.com:58900")

	endpoints := h.GetEndpoints()
	if len(endpoints) < 4 {
		t.Fatalf("expected at least 4 endpoints, got %d", len(endpoints))
	}

	foundLAN := false
	foundV6 := false
	foundDDNS := false
	foundRelay := false

	for _, ep := range endpoints {
		if ep.Type == "lan" && strings.Contains(ep.URL, "192.168.1.50:58900") {
			foundLAN = true
		}
		if ep.Type == "ipv6" && strings.Contains(ep.URL, "[2001:db8:abcd::1]:58900") {
			foundV6 = true
		}
		if ep.Type == "ddns" && strings.Contains(ep.URL, "mac.example.com:58900") {
			foundDDNS = true
		}
		if ep.Type == "relay" && strings.Contains(ep.URL, "relay.example.com:58900") {
			foundRelay = true
		}
	}

	if !foundLAN {
		t.Errorf("LAN endpoint missing")
	}
	if !foundV6 {
		t.Errorf("IPv6 endpoint missing")
	}
	if !foundDDNS {
		t.Errorf("DDNS endpoint missing")
	}
	if !foundRelay {
		t.Errorf("Relay endpoint missing")
	}
}

func TestAuthHandler_NewPairingSession_SSLOmitsIPLiterals(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "pair-admin-token")
	store, err := NewAuthStore(t.TempDir() + "/auth.json")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	pm := NewPairingManager()
	h := NewAuthHandler(store, pm, "agy.jiuge.space", 58900, true)
	h.SetEndpoints("192.168.50.9", "2001:db8:abcd::1", "agy.jiuge.space")
	h.SetRelayURL("https://agy.jiuge.space:58900")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", nil)
	req.Header.Set("Authorization", "Bearer pair-admin-token")
	rr := httptest.NewRecorder()
	h.HandleNewPairingSession(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("session status %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(resp.URI, "ssl=1") {
		t.Fatalf("expected ssl=1 in %s", resp.URI)
	}
	if !strings.Contains(resp.URI, "host=agy.jiuge.space") {
		t.Fatalf("expected domain host in %s", resp.URI)
	}
	if strings.Contains(resp.URI, "lan=") || strings.Contains(resp.URI, "ipv6=") {
		t.Fatalf("TLS pairing URI must not include IP literals: %s", resp.URI)
	}
}

func TestAuthHandler_GetEndpoints_SSLOmitsIPLiterals(t *testing.T) {
	store, _ := NewAuthStore(t.TempDir() + "/auth.json")
	pm := NewPairingManager()
	h := NewAuthHandler(store, pm, "agy.jiuge.space", 58900, true)
	h.SetEndpoints("192.168.50.9", "2001:db8:abcd::1", "agy.jiuge.space")
	h.SetRelayURL("https://agy.jiuge.space:58900")

	endpoints := h.GetEndpoints()
	if len(endpoints) != 1 {
		t.Fatalf("expected only the domain endpoint under TLS, got %+v", endpoints)
	}
	if endpoints[0].URL != "https://agy.jiuge.space:58900" {
		t.Fatalf("unexpected endpoint %s", endpoints[0].URL)
	}
}
