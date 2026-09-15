package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"antigravity-mobile/internal/auth"
	"antigravity-mobile/internal/inspector"
	"antigravity-mobile/internal/proxy"
	"antigravity-mobile/web"
)

func setupTestRouter(t *testing.T) http.Handler {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	authStore, err := auth.NewAuthStore(storePath)
	if err != nil {
		t.Fatalf("failed to create auth store: %v", err)
	}

	pm := auth.NewPairingManager()
	authHandler := auth.NewAuthHandler(authStore, pm, "127.0.0.1", 58900, false)
	insp := inspector.NewInspector(10 * time.Second)
	p := proxy.NewProxy(insp)
	startTime := time.Now()
	webHandler := web.Handler()

	return buildRouter(authStore, authHandler, p, insp, startTime, webHandler, auth.AuthPolicy{})
}

func TestHealthzEndpoint(t *testing.T) {
	router := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 from /healthz, got %d", rr.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %q", resp["status"])
	}
}

func TestReadyzEndpoint(t *testing.T) {
	router := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	// Since inspector has no active language_server, status should be not_ready or 200 if active
	if rr.Code != http.StatusOK && rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 200 or 503 from /readyz, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if _, ok := resp["status"]; !ok {
		t.Errorf("missing 'status' in readyz response: %+v", resp)
	}
}

func TestSecurityHeaders(t *testing.T) {
	router := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("expected X-Content-Type-Options: nosniff, got %q", rr.Header().Get("X-Content-Type-Options"))
	}
	if rr.Header().Get("X-Frame-Options") != "DENY" {
		t.Errorf("expected X-Frame-Options: DENY, got %q", rr.Header().Get("X-Frame-Options"))
	}
	if rr.Header().Get("Content-Security-Policy") == "" {
		t.Errorf("expected Content-Security-Policy header")
	}
	if rr.Header().Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
		t.Errorf("expected Referrer-Policy: strict-origin-when-cross-origin, got %q", rr.Header().Get("Referrer-Policy"))
	}
	if rr.Header().Get("Strict-Transport-Security") != "" {
		t.Errorf("HSTS must not be set on plaintext HTTP, got %q", rr.Header().Get("Strict-Transport-Security"))
	}

	httpsReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	httpsReq.Header.Set("X-Forwarded-Proto", "https")
	httpsRR := httptest.NewRecorder()
	router.ServeHTTP(httpsRR, httpsReq)
	if httpsRR.Header().Get("Strict-Transport-Security") == "" {
		t.Errorf("expected HSTS when X-Forwarded-Proto is https")
	}
}

func TestGatewayStatusRequiresAuth(t *testing.T) {
	router := setupTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/gateway/status", nil)
	req.RemoteAddr = "192.168.1.50:12345"
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 from unauthenticated /gateway/status, got %d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "csrf_token") {
		t.Errorf("unauthenticated status must not leak csrf_token: %s", rr.Body.String())
	}
}

func TestUnauthorizedAPIRejected(t *testing.T) {
	router := setupTestRouter(t)

	// A non-whitelisted path should return 401 without auth token
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/content?uri=test", nil)
	// Non-loopback remote address
	req.RemoteAddr = "192.168.1.100:12345"
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for remote request without token, got %d", rr.Code)
	}
}
