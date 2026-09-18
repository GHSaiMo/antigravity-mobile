package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuthMiddleware_And_Handler(t *testing.T) {
	t.Setenv("MULTIGRAVITY_ADMIN_TOKEN", "")
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

	// 6. Public health probe (/healthz) without token -> 200
	healthHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/healthz", healthHandler)
	reqHealth := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rrHealth := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rrHealth, reqHealth)
	if rrHealth.Code != http.StatusOK {
		t.Fatalf("expected 200 for whitelisted /healthz, got %d", rrHealth.Code)
	}

	// 6b. /gateway/status is no longer public
	mux.Handle("/gateway/status", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	reqStatus := httptest.NewRequest(http.MethodGet, "/gateway/status", nil)
	reqStatus.RemoteAddr = "192.168.1.50:12345"
	rrStatus := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rrStatus, reqStatus)
	if rrStatus.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated /gateway/status, got %d", rrStatus.Code)
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
		{"/gateway/status", false},
		{"/healthz", true},
		{"/readyz", true},
		{"/gateway/cascade/touch", false},
		{"/gateway/cascade/invalidate", false},
		{"/api/v1/auth/pair", true},
		{"/api/v1/auth/session", true},
		{"/api/v1/auth/ws-ticket", true},
		{"/api/v1/devices", true},
		{"/api/v1/devices/", true},
		{"/api/v1/devices/dev-123", true},
		// Paths that should NOT match (M-5 fix)
		{"/api/v1/auth/fake", false},
		{"/api/v1/auth/bypass", false},
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
	t.Setenv("MULTIGRAVITY_ADMIN_TOKEN", "")
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

	// Case 5: MULTIGRAVITY_ADMIN_TOKEN via Bearer header
	t.Setenv("MULTIGRAVITY_ADMIN_TOKEN", "secret-admin-123")
	reqAdminBearer := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	reqAdminBearer.RemoteAddr = "192.168.1.100:12345"
	reqAdminBearer.Header.Set("Authorization", "Bearer secret-admin-123")
	if !authHandler.isAuthorizedAdmin(reqAdminBearer) {
		t.Errorf("expected MULTIGRAVITY_ADMIN_TOKEN via Bearer header to be authorized")
	}

	// Case 6: MULTIGRAVITY_ADMIN_TOKEN via query param is rejected (leaks in logs/Referer)
	reqAdminQuery := httptest.NewRequest(http.MethodGet, "/api/v1/devices?admin_token=secret-admin-123", nil)
	reqAdminQuery.RemoteAddr = "192.168.1.100:12345"
	if authHandler.isAuthorizedAdmin(reqAdminQuery) {
		t.Errorf("expected MULTIGRAVITY_ADMIN_TOKEN via query parameter to be rejected")
	}

	// Case 7: Invalid MULTIGRAVITY_ADMIN_TOKEN
	reqAdminBad := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	reqAdminBad.RemoteAddr = "192.168.1.100:12345"
	reqAdminBad.Header.Set("Authorization", "Bearer wrong-token")
	if authHandler.isAuthorizedAdmin(reqAdminBad) {
		t.Errorf("expected invalid admin token to be rejected")
	}

	// Case 8: MULTIGRAVITY_ADMIN_TOKEN configured — loopback fallback is disabled
	reqLoopbackWithAdminEnv := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	reqLoopbackWithAdminEnv.RemoteAddr = "127.0.0.1:12345"
	if authHandler.isAuthorizedAdmin(reqLoopbackWithAdminEnv) {
		t.Errorf("expected loopback fallback to be disabled when MULTIGRAVITY_ADMIN_TOKEN is set")
	}
}

func TestAdminAuthorization_TunnelDisablesLoopback(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewAuthStore(filepath.Join(tempDir, "auth_store.json"))
	if err != nil {
		t.Fatalf("failed to create auth store: %v", err)
	}
	pm := NewPairingManager()
	authHandler := NewAuthHandler(store, pm, "127.0.0.1", 58900, false)
	authHandler.SetAuthPolicy(AuthPolicy{TunnelEnabled: true, ListenLoopback: true})

	reqLoopback := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", nil)
	reqLoopback.RemoteAddr = "127.0.0.1:12345"
	if authHandler.isAuthorizedAdmin(reqLoopback) {
		t.Errorf("expected loopback admin to be denied when tunnel is enabled")
	}

	t.Setenv("MULTIGRAVITY_ADMIN_TOKEN", "frp-admin-token")
	reqWithToken := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", nil)
	reqWithToken.RemoteAddr = "127.0.0.1:12345"
	reqWithToken.Header.Set("Authorization", "Bearer frp-admin-token")
	if !authHandler.isAuthorizedAdmin(reqWithToken) {
		t.Errorf("expected MULTIGRAVITY_ADMIN_TOKEN bearer to work even when tunnel is enabled")
	}
}

func TestNewPairingSessionMethodNotAllowed(t *testing.T) {
	tempDir := t.TempDir()
	store, err := NewAuthStore(filepath.Join(tempDir, "auth_store.json"))
	if err != nil {
		t.Fatalf("failed to create auth store: %v", err)
	}
	pm := NewPairingManager()
	authHandler := NewAuthHandler(store, pm, "127.0.0.1", 58900, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	req.RemoteAddr = "127.0.0.1:1"
	rr := httptest.NewRecorder()
	authHandler.HandleNewPairingSession(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET /api/v1/auth/session, got %d", rr.Code)
	}
}

func TestAuthDisabledIgnoredWhenTunnelEnabled(t *testing.T) {
	t.Setenv("AUTH_DISABLED", "1")
	tempDir := t.TempDir()
	store, err := NewAuthStore(filepath.Join(tempDir, "auth_store.json"))
	if err != nil {
		t.Fatalf("failed to create auth store: %v", err)
	}

	hit := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	})
	wrapped := AuthMiddlewareWithPolicy(store, next, AuthPolicy{TunnelEnabled: true, ListenLoopback: true})

	req := httptest.NewRequest(http.MethodGet, "/api/secret", nil)
	req.RemoteAddr = "127.0.0.1:9"
	rr := httptest.NewRecorder()
	wrapped.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when AUTH_DISABLED is ignored due to tunnel, got %d", rr.Code)
	}
	if hit {
		t.Fatalf("protected handler must not run")
	}
}

func TestIsListenAddrLoopback(t *testing.T) {
	if IsListenAddrLoopback("") || IsListenAddrLoopback("0.0.0.0") || IsListenAddrLoopback("::") {
		t.Errorf("wildcard/empty listen addrs must not be treated as loopback")
	}
	if !IsListenAddrLoopback("127.0.0.1") || !IsListenAddrLoopback("localhost") || !IsListenAddrLoopback("[::1]") {
		t.Errorf("loopback listen addrs must be detected")
	}
}

func TestSecurityHeadersMiddlewarePermissionsPolicy(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := SecurityHeadersMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	policy := rr.Header().Get("Permissions-Policy")
	if policy == "" {
		t.Errorf("expected Permissions-Policy header to be present")
	}
	if policy != "camera=(), microphone=(), geolocation=()" {
		t.Errorf("unexpected Permissions-Policy: %q", policy)
	}
}

func TestIsQueryTokenDisabled(t *testing.T) {
	t.Setenv("MULTIGRAVITY_DISABLE_QUERY_TOKEN", "")
	if IsQueryTokenDisabled() {
		t.Errorf("expected false when unset")
	}
	t.Setenv("MULTIGRAVITY_DISABLE_QUERY_TOKEN", "1")
	if !IsQueryTokenDisabled() {
		t.Errorf("expected true when set to 1")
	}

	// When disabled, ExtractToken on WS path with query param should return empty
	req := httptest.NewRequest(http.MethodGet, "/connect-websocket?token=some_token", nil)
	if tok := ExtractToken(req); tok != "" {
		t.Errorf("expected empty token when query tokens are disabled, got %q", tok)
	}
}

func TestSecurityHeadersMiddlewareHSTS_SSL(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := SecurityHeadersMiddleware(next)

	// 1. Without SSL env -> No HSTS
	t.Setenv("MULTIGRAVITY_SSL", "0")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Header().Get("Strict-Transport-Security") != "" {
		t.Errorf("expected no HSTS on plain HTTP without SSL env")
	}

	// 2. With MULTIGRAVITY_SSL=1 -> HSTS present
	t.Setenv("MULTIGRAVITY_SSL", "1")
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Header().Get("Strict-Transport-Security") == "" {
		t.Errorf("expected HSTS header when MULTIGRAVITY_SSL=1")
	}
}

func TestMaxBytesMiddleware(t *testing.T) {
	// Handler that reads the body
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with 100-byte limit
	handler := MaxBytesMiddleware(100, next)

	// Small payload (< 100 bytes) -> 200 OK
	smallReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("small payload"))
	smallRR := httptest.NewRecorder()
	handler.ServeHTTP(smallRR, smallReq)
	if smallRR.Code != http.StatusOK {
		t.Errorf("expected 200 OK for small payload, got %d", smallRR.Code)
	}

	// Large payload (> 100 bytes) -> 413 Request Entity Too Large
	largeReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("A", 200)))
	largeRR := httptest.NewRecorder()
	handler.ServeHTTP(largeRR, largeReq)
	if largeRR.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413 RequestEntityTooLarge for oversized payload, got %d", largeRR.Code)
	}
}




