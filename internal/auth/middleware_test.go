package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestAuthMiddleware_And_Handler(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "auth_store.json")
	store, err := NewAuthStore(storePath)
	if err != nil {
		t.Fatalf("failed to create auth store: %v", err)
	}

	pm := NewPairingManager()
	authHandler := NewAuthHandler(store, pm, "mac.local", 58900, false)

	// Setup a dummy protected handler wrapped in AuthMiddleware
	protectedHit := false
	protectedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protectedHit = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/pair", authHandler.HandlePair)
	mux.HandleFunc("/api/v1/devices", authHandler.HandleDevices)
	mux.HandleFunc("/api/v1/devices/", authHandler.HandleDevices)
	mux.Handle("/codeium.cascade.test", protectedHandler)

	wrappedRouter := AuthMiddleware(store, mux)

	// 1. Access protected route without token -> 401
	req1 := httptest.NewRequest(http.MethodGet, "/codeium.cascade.test", nil)
	rr1 := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr1.Code)
	}

	// 2. Generate pairing code and pair
	session, err := pm.GenerateSession(5 * time.Minute)
	if err != nil {
		t.Fatalf("failed to generate session: %v", err)
	}

	pairBody, _ := json.Marshal(PairRequest{
		PairingCode: session.Code,
		DeviceName:  "Test Phone",
		Platform:    "ios",
	})
	pairReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/pair", bytes.NewReader(pairBody))
	pairReq.Header.Set("Content-Type", "application/json")
	pairRR := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(pairRR, pairReq)

	if pairRR.Code != http.StatusOK {
		t.Fatalf("expected 200 from pair, got %d (body: %s)", pairRR.Code, pairRR.Body.String())
	}

	var pairResp PairResponse
	if err := json.NewDecoder(pairRR.Body).Decode(&pairResp); err != nil {
		t.Fatalf("failed to parse pair response: %v", err)
	}
	if pairResp.DeviceToken == "" || pairResp.DeviceID == "" {
		t.Fatalf("empty token or device id in response: %+v", pairResp)
	}

	// 3. Re-using pairing code should fail
	pairReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/pair", bytes.NewReader(pairBody))
	pairReq2.Header.Set("Content-Type", "application/json")
	pairRR2 := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(pairRR2, pairReq2)
	if pairRR2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on re-used code, got %d", pairRR2.Code)
	}

	// 4. Access protected route with Bearer Header -> 200
	req2 := httptest.NewRequest(http.MethodGet, "/codeium.cascade.test", nil)
	req2.Header.Set("Authorization", "Bearer "+pairResp.DeviceToken)
	rr2 := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 with Bearer token, got %d", rr2.Code)
	}
	if !protectedHit {
		t.Errorf("expected protected handler to be hit")
	}

	// 5. Access with Query Param on standard API endpoint -> 401 Unauthorized (CWE-598 mitigation)
	req3 := httptest.NewRequest(http.MethodGet, "/codeium.cascade.test?auth_token="+pairResp.DeviceToken, nil)
	rr3 := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for query token on standard HTTP endpoint, got %d", rr3.Code)
	}

	// 5b. Access with Query Param on WebSocket Upgrade -> 200 OK (permitted for browser WS)
	reqWS := httptest.NewRequest(http.MethodGet, "/codeium.cascade.test?auth_token="+pairResp.DeviceToken, nil)
	reqWS.Header.Set("Upgrade", "websocket")
	rrWS := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rrWS, reqWS)
	if rrWS.Code != http.StatusOK {
		t.Fatalf("expected 200 for query token on WebSocket upgrade, got %d", rrWS.Code)
	}

	// 5c. Access with Query Param on /connect-websocket -> 200 OK
	mux.HandleFunc("/connect-websocket", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	reqWSConnect := httptest.NewRequest(http.MethodGet, "/connect-websocket?token="+pairResp.DeviceToken, nil)
	rrWSConnect := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rrWSConnect, reqWSConnect)
	if rrWSConnect.Code != http.StatusOK {
		t.Fatalf("expected 200 for query token on /connect-websocket, got %d", rrWSConnect.Code)
	}

	// 6. Whitelisted path (/gateway/status) without token -> 200
	statusHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/gateway/status", statusHandler)
	reqStatus := httptest.NewRequest(http.MethodGet, "/gateway/status", nil)
	rrStatus := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rrStatus, reqStatus)
	if rrStatus.Code != http.StatusOK {
		t.Fatalf("expected 200 for whitelisted /gateway/status, got %d", rrStatus.Code)
	}

	// 7. Device list via Loopback
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	reqList.RemoteAddr = "127.0.0.1:12345"
	rrList := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rrList, reqList)
	if rrList.Code != http.StatusOK {
		t.Fatalf("expected 200 from devices list, got %d", rrList.Code)
	}

	var devices []PairedDevice
	if err := json.NewDecoder(rrList.Body).Decode(&devices); err != nil {
		t.Fatalf("failed to decode devices: %v", err)
	}
	if len(devices) != 1 || devices[0].DeviceID != pairResp.DeviceID {
		t.Fatalf("expected 1 device with id %s, got %+v", pairResp.DeviceID, devices)
	}

	// 8. Delete device
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/"+pairResp.DeviceID, nil)
	reqDel.RemoteAddr = "127.0.0.1:12345"
	rrDel := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rrDel, reqDel)
	if rrDel.Code != http.StatusOK {
		t.Fatalf("expected 200 on delete, got %d", rrDel.Code)
	}

	// 9. Token revoked after delete -> 401
	reqRevoked := httptest.NewRequest(http.MethodGet, "/codeium.cascade.test", nil)
	reqRevoked.Header.Set("Authorization", "Bearer "+pairResp.DeviceToken)
	rrRevoked := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rrRevoked, reqRevoked)
	if rrRevoked.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after revocation, got %d", rrRevoked.Code)
	}
}

func TestIsWhitelistedPath(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/", true},
		{"/index.html", true},
		{"/web/index.html", true},
		{"/icons/icon.png", true},
		{"/gateway/status", true},
		{"/api/v1/auth/pair", true},
		{"/api/v1/devices", true},
		{"/api/v1/devices/", true},
		{"/api/v1/devices/dev-123", true},
		// Paths that should NOT match
		{"/api/v1/devices_bypass", false},
		{"/api/v1/devices_unauthorized", false},
		{"/codeium.cascade.test", false},
		{"/unknown", false},
	}

	for _, tt := range tests {
		got := IsWhitelistedPath(tt.path)
		if got != tt.expected {
			t.Errorf("IsWhitelistedPath(%q) = %v, expected %v", tt.path, got, tt.expected)
		}
	}
}

func TestAdminAuthorization(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "auth_store.json")
	store, err := NewAuthStore(storePath)
	if err != nil {
		t.Fatalf("failed to create auth store: %v", err)
	}
	deviceToken := "tok_secret1234567890abcdef"
	dev := PairedDevice{
		DeviceID:   "dev-1",
		DeviceName: "Test Device",
		Platform:   "ios",
		TokenHash:  HashToken(deviceToken),
		CreatedAt:  time.Now(),
		LastSeenAt: time.Now(),
	}
	if err := store.AddDevice(dev); err != nil {
		t.Fatalf("failed to add device: %v", err)
	}

	pm := NewPairingManager()
	authHandler := NewAuthHandler(store, pm, "mac.local", 58900, false)

	// Case 1: Device token does NOT grant admin access
	reqDevToken := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	reqDevToken.RemoteAddr = "192.168.1.100:12345"
	reqDevToken.Header.Set("Authorization", "Bearer "+deviceToken)
	if authHandler.isAuthorizedAdmin(reqDevToken) {
		t.Errorf("expected device token not to grant admin access")
	}

	// Case 2: Direct loopback without proxy headers grants admin
	reqLoopback := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	reqLoopback.RemoteAddr = "127.0.0.1:12345"
	if !authHandler.isAuthorizedAdmin(reqLoopback) {
		t.Errorf("expected loopback to grant admin when no proxy headers present")
	}

	// Case 3: Loopback with X-Forwarded-For should be rejected
	reqProxyXFF := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	reqProxyXFF.RemoteAddr = "127.0.0.1:12345"
	reqProxyXFF.Header.Set("X-Forwarded-For", "203.0.113.195")
	if authHandler.isAuthorizedAdmin(reqProxyXFF) {
		t.Errorf("expected loopback with X-Forwarded-For to be rejected")
	}

	// Case 4: Loopback with X-Real-IP should be rejected
	reqProxyRealIP := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	reqProxyRealIP.RemoteAddr = "127.0.0.1:12345"
	reqProxyRealIP.Header.Set("X-Real-IP", "203.0.113.195")
	if authHandler.isAuthorizedAdmin(reqProxyRealIP) {
		t.Errorf("expected loopback with X-Real-IP to be rejected")
	}

	// Case 5: ADMIN_TOKEN via Bearer header
	t.Setenv("ADMIN_TOKEN", "secret-admin-123")
	reqAdminBearer := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	reqAdminBearer.RemoteAddr = "192.168.1.100:12345"
	reqAdminBearer.Header.Set("Authorization", "Bearer secret-admin-123")
	if !authHandler.isAuthorizedAdmin(reqAdminBearer) {
		t.Errorf("expected ADMIN_TOKEN via Bearer header to be authorized")
	}

	// Case 6: ADMIN_TOKEN via query param
	reqAdminQuery := httptest.NewRequest(http.MethodGet, "/api/v1/devices?admin_token=secret-admin-123", nil)
	reqAdminQuery.RemoteAddr = "192.168.1.100:12345"
	if !authHandler.isAuthorizedAdmin(reqAdminQuery) {
		t.Errorf("expected ADMIN_TOKEN via query parameter to be authorized")
	}

	// Case 7: Invalid ADMIN_TOKEN
	reqAdminBad := httptest.NewRequest(http.MethodGet, "/api/v1/devices?admin_token=wrong-token", nil)
	reqAdminBad.RemoteAddr = "192.168.1.100:12345"
	if authHandler.isAuthorizedAdmin(reqAdminBad) {
		t.Errorf("expected invalid admin token to be rejected")
	}
}

