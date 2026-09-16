package auth

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func TestFormatPairingQRCode(t *testing.T) {
	out := FormatPairingQRCode("192.168.50.9", 58900, "abc123code", false, "2001:db8::1", "124.222.226.143")
	if !strings.Contains(out, "Antigravity Mobile 客户端扫码一键配对") {
		t.Errorf("expected header banner in output")
	}
	if !strings.Contains(out, "agy://pair?code=abc123code") {
		t.Errorf("expected pairing URI in output")
	}
	if !strings.Contains(out, "局域网 Wi-Fi 直连 URI:") {
		t.Errorf("expected LAN URI in output")
	}
	if !strings.Contains(out, "外网 IPv6 直连 URI:") {
		t.Errorf("expected IPv6 URI in output")
	}
	if !strings.Contains(out, "云服务器中继 URI:") {
		t.Errorf("expected relay URI in output")
	}
	if !strings.Contains(out, "请使用 Antigravity 手机客户端扫描上方二维码") {
		t.Errorf("expected footer instructions in output")
	}
}

func TestInitConsoleSyncAndConcurrentLogging(t *testing.T) {
	InitConsoleSync()

	unlock := ConsoleLock()
	unlock()

	var wg sync.WaitGroup
	// Run concurrent log writers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				log.Printf("[Test] Concurrent log from worker %d-%d", idx, j)
			}
		}(i)
	}

	// Run concurrent QR code printers
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			PrintPairingQRCode("127.0.0.1", 58900, "testcode", false)
		}()
	}

	wg.Wait()
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
		RelayHost:   "198.51.100.1",
	})
	if !strings.Contains(uri, "relay=198.51.100.1") {
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
	h.SetRelayURL("http://198.51.100.1:58900")

	if got := h.relayHost(); got != "198.51.100.1" {
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
	if !strings.Contains(resp.URI, "relay=198.51.100.1") {
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
	h := NewAuthHandler(store, pm, "agy.example.com", 58900, true)
	h.SetEndpoints("192.168.50.9", "2001:db8:abcd::1", "agy.example.com")
	h.SetRelayURL("https://agy.example.com:58900")

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
	if !strings.Contains(resp.URI, "host=agy.example.com") {
		t.Fatalf("expected domain host in %s", resp.URI)
	}
	if strings.Contains(resp.URI, "lan=") || strings.Contains(resp.URI, "ipv6=") {
		t.Fatalf("TLS pairing URI must not include IP literals: %s", resp.URI)
	}
}

func TestAuthHandler_GetEndpoints_SSLOmitsIPLiterals(t *testing.T) {
	store, _ := NewAuthStore(t.TempDir() + "/auth.json")
	pm := NewPairingManager()
	h := NewAuthHandler(store, pm, "agy.example.com", 58900, true)
	h.SetEndpoints("192.168.50.9", "2001:db8:abcd::1", "agy.example.com")
	h.SetRelayURL("https://agy.example.com:58900")

	endpoints := h.GetEndpoints()
	if len(endpoints) != 1 {
		t.Fatalf("expected only the domain endpoint under TLS, got %+v", endpoints)
	}
	if endpoints[0].URL != "https://agy.example.com:58900" {
		t.Fatalf("unexpected endpoint %s", endpoints[0].URL)
	}
}

func TestAuthHandler_HandleDevices_ClearAll(t *testing.T) {
	t.Setenv("ADMIN_TOKEN", "test-admin-secret")
	store, err := NewAuthStore(t.TempDir() + "/auth.json")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	pm := NewPairingManager()
	h := NewAuthHandler(store, pm, "127.0.0.1", 58900, false)

	// Add 2 devices
	_ = store.AddDevice(PairedDevice{DeviceID: "dev_1", DeviceName: "Phone 1"})
	_ = store.AddDevice(PairedDevice{DeviceID: "dev_2", DeviceName: "Phone 2"})

	// 1. GET /api/v1/devices
	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	reqGet.Header.Set("Authorization", "Bearer test-admin-secret")
	rrGet := httptest.NewRecorder()
	h.HandleDevices(rrGet, reqGet)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rrGet.Code, rrGet.Body.String())
	}
	var devList []PairedDevice
	if err := json.Unmarshal(rrGet.Body.Bytes(), &devList); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(devList) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devList))
	}

	// 2. DELETE /api/v1/devices/all
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/all", nil)
	reqDel.Header.Set("Authorization", "Bearer test-admin-secret")
	rrDel := httptest.NewRecorder()
	h.HandleDevices(rrDel, reqDel)
	if rrDel.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rrDel.Code, rrDel.Body.String())
	}
	var delResp map[string]any
	if err := json.Unmarshal(rrDel.Body.Bytes(), &delResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if delResp["status"] != "cleared" || delResp["cleared"].(float64) != 2 {
		t.Fatalf("unexpected delete resp: %+v", delResp)
	}

	// 3. GET /api/v1/devices should now be empty
	rrGet2 := httptest.NewRecorder()
	h.HandleDevices(rrGet2, reqGet)
	var devList2 []PairedDevice
	_ = json.Unmarshal(rrGet2.Body.Bytes(), &devList2)
	if len(devList2) != 0 {
		t.Fatalf("expected 0 devices after clear, got %d", len(devList2))
	}
}

