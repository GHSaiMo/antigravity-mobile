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

func TestDetermineViewMode(t *testing.T) {
	// 1. Explicit query parameter
	reqDesktopQ := httptest.NewRequest(http.MethodGet, "/?view=desktop", nil)
	if mode := determineViewMode(reqDesktopQ); mode != "desktop" {
		t.Errorf("expected desktop from ?view=desktop, got %q", mode)
	}

	reqMobileQ := httptest.NewRequest(http.MethodGet, "/?view=mobile", nil)
	if mode := determineViewMode(reqMobileQ); mode != "mobile" {
		t.Errorf("expected mobile from ?view=mobile, got %q", mode)
	}

	// 2. Cookie preference
	reqCookie := httptest.NewRequest(http.MethodGet, "/", nil)
	reqCookie.AddCookie(&http.Cookie{Name: "agy_view_mode", Value: "desktop"})
	if mode := determineViewMode(reqCookie); mode != "desktop" {
		t.Errorf("expected desktop from cookie, got %q", mode)
	}

	// 3. User-Agent auto-detection
	reqIPhone := httptest.NewRequest(http.MethodGet, "/", nil)
	reqIPhone.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148")
	if mode := determineViewMode(reqIPhone); mode != "mobile" {
		t.Errorf("expected mobile from iPhone UA, got %q", mode)
	}

	reqIPad := httptest.NewRequest(http.MethodGet, "/", nil)
	reqIPad.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15")
	if mode := determineViewMode(reqIPad); mode != "desktop" {
		t.Errorf("expected desktop from Mac/iPad UA, got %q", mode)
	}
}

func TestIsDesktopStaticPath(t *testing.T) {
	if !isDesktopStaticPath("/main.js") {
		t.Errorf("expected /main.js to be desktop static")
	}
	if !isDesktopStaticPath("/jetbox.css") {
		t.Errorf("expected /jetbox.css to be desktop static")
	}
	if !isDesktopStaticPath("/symbols-icons/folder.svg") {
		t.Errorf("expected /symbols-icons/ to be desktop static")
	}
	if isDesktopStaticPath("/app.js") {
		t.Errorf("/app.js should not be desktop static")
	}
}

func TestStaticWebAssetsAccessible(t *testing.T) {
	router := setupTestRouter(t)

	for _, p := range []string{"/zh-CN.js", "/view-switcher.js", "/view-switcher.css"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for %s, got %d", p, rr.Code)
		}
	}
}

