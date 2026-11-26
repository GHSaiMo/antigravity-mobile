package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"antigravity-mobile/internal/auth"
	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/e2ee"
	"antigravity-mobile/internal/inspector"
	"antigravity-mobile/internal/proxy"
	"antigravity-mobile/internal/relay"
	"antigravity-mobile/web"

	"github.com/gorilla/websocket"
)

func setupTestRouter(t *testing.T) http.Handler {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "auth.json")
	authStore, err := auth.NewAuthStore(storePath)
	if err != nil {
		t.Fatalf("failed to create auth store: %v", err)
	}
	t.Cleanup(authStore.Close)

	pm := auth.NewPairingManager()
	authHandler := auth.NewAuthHandler(authStore, pm, "127.0.0.1", 58900, false)
	t.Cleanup(authHandler.Close)
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
	if resp["os"] != runtime.GOOS {
		t.Errorf("expected os %q, got %q", runtime.GOOS, resp["os"])
	}
	if resp["platform"] != runtime.GOOS {
		t.Errorf("expected platform %q, got %q", runtime.GOOS, resp["platform"])
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
	if resp["os"] != runtime.GOOS {
		t.Errorf("expected os %q, got %v", runtime.GOOS, resp["os"])
	}
	if resp["platform"] != runtime.GOOS {
		t.Errorf("expected platform %q, got %v", runtime.GOOS, resp["platform"])
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

func TestStaticWebAssetsAccessible(t *testing.T) {
	router := setupTestRouter(t)

	for _, p := range []string{"/", "/js/core.js", "/js/cockpit.js", "/style.css", "/manifest.json", "/favicon.ico"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for %s, got %d", p, rr.Code)
		}
	}
}

func TestDetermineViewMode(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		userAgent  string
		cookie     *http.Cookie
		expectMode string
	}{
		{
			name:       "PC Mac Chrome default",
			url:        "http://localhost:58900/",
			userAgent:  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			expectMode: "desktop",
		},
		{
			name:       "PC Windows Edge default",
			url:        "http://localhost:58900/",
			userAgent:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
			expectMode: "desktop",
		},
		{
			name:       "PC Linux Firefox default",
			url:        "http://localhost:58900/",
			userAgent:  "Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/119.0",
			expectMode: "desktop",
		},
		{
			name:       "iPhone Safari -> PWA",
			url:        "http://localhost:58900/",
			userAgent:  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			expectMode: "pwa",
		},
		{
			name:       "iPad Safari -> PWA",
			url:        "http://localhost:58900/",
			userAgent:  "Mozilla/5.0 (iPad; CPU OS 16_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.5 Mobile/15E148 Safari/604.1",
			expectMode: "pwa",
		},
		{
			name:       "Android Phone -> PWA",
			url:        "http://localhost:58900/",
			userAgent:  "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
			expectMode: "pwa",
		},
		{
			name:       "Android Tablet -> PWA",
			url:        "http://localhost:58900/",
			userAgent:  "Mozilla/5.0 (Linux; Android 13; SM-X900) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			expectMode: "pwa",
		},
		{
			name:       "Query override view=desktop on mobile",
			url:        "http://localhost:58900/?view=desktop",
			userAgent:  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)",
			expectMode: "desktop",
		},
		{
			name:       "Query override view=pwa on desktop",
			url:        "http://localhost:58900/?view=pwa",
			userAgent:  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
			expectMode: "pwa",
		},
		{
			name:       "Cookie override desktop",
			url:        "http://localhost:58900/",
			userAgent:  "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)",
			cookie:     &http.Cookie{Name: "agy_view_mode", Value: "desktop"},
			expectMode: "desktop",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			if tc.userAgent != "" {
				req.Header.Set("User-Agent", tc.userAgent)
			}
			if tc.cookie != nil {
				req.AddCookie(tc.cookie)
			}
			actual := determineViewMode(req)
			if actual != tc.expectMode {
				t.Errorf("expected view mode %q, got %q", tc.expectMode, actual)
			}
		})
	}
}

func TestOnboardingRedirect(t *testing.T) {
	router := setupTestRouter(t)

	// Test 1: Direct /onboarding?login=true redirects to /
	req := httptest.NewRequest(http.MethodGet, "/onboarding?login=true", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 redirect, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/" {
		t.Errorf("expected Location '/', got %q", loc)
	}

	// Test 2: /onboarding with custom redirect parameter
	req2 := httptest.NewRequest(http.MethodGet, "/onboarding?login=true&redirect=/c/123", nil)
	rr2 := httptest.NewRecorder()
	router.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307 redirect, got %d", rr2.Code)
	}
	if loc := rr2.Header().Get("Location"); loc != "/c/123" {
		t.Errorf("expected Location '/c/123', got %q", loc)
	}

	// Test 3: Open redirect payloads must be sanitized to '/' (SEC-06)
	for _, evil := range []string{
		"https://evil.example.com",
		"//evil.example.com",
		`/\\evil.example.com`,
		`/\evil.example.com`,
		"/onboarding?login=true",
	} {
		reqEvil := httptest.NewRequest(http.MethodGet, "/onboarding?login=true&redirect="+evil, nil)
		rrEvil := httptest.NewRecorder()
		router.ServeHTTP(rrEvil, reqEvil)
		if rrEvil.Code != http.StatusTemporaryRedirect {
			t.Fatalf("expected 307 for redirect=%q, got %d", evil, rrEvil.Code)
		}
		if loc := rrEvil.Header().Get("Location"); loc != "/" {
			t.Errorf("open redirect payload %q escaped sanitizeLocalRedirect: Location=%q", evil, loc)
		}
	}
}

func TestCloudURLHeaderGatedOnAuth(t *testing.T) {
	tmpDir := t.TempDir()
	authStore, err := auth.NewAuthStore(filepath.Join(tmpDir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(authStore.Close)
	devToken := "paired-device-secret-token"
	if err := authStore.AddDevice(auth.PairedDevice{
		DeviceID:   "dev-cf-test",
		DeviceName: "iPhone",
		Platform:   "ios",
		TokenHash:  auth.HashToken(devToken),
		CreatedAt:  time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	pm := auth.NewPairingManager()
	authHandler := auth.NewAuthHandler(authStore, pm, "127.0.0.1", 58900, false)
	t.Cleanup(authHandler.Close)
	authHandler.SetCloudflareURL("https://secret-tunnel.example.com")
	insp := inspector.NewInspector(10 * time.Second)
	p := proxy.NewProxy(insp)
	router := buildRouter(authStore, authHandler, p, insp, time.Now(), web.Handler(), auth.AuthPolicy{})

	// 1. Unauthenticated request to whitelisted /healthz must NOT leak X-Antigravity-Cloud-URL (SEC-03)
	unauthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	unauthReq.RemoteAddr = "203.0.113.10:54321"
	unauthRR := httptest.NewRecorder()
	router.ServeHTTP(unauthRR, unauthReq)
	if got := unauthRR.Header().Get("X-Antigravity-Cloud-URL"); got != "" {
		t.Errorf("unauthenticated response leaked X-Antigravity-Cloud-URL: %q", got)
	}

	// 2. Authenticated request MUST receive X-Antigravity-Cloud-URL for self-healing
	authReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	authReq.RemoteAddr = "203.0.113.10:54321"
	authReq.Header.Set("Authorization", "Bearer "+devToken)
	authRR := httptest.NewRecorder()
	router.ServeHTTP(authRR, authReq)
	if got := authRR.Header().Get("X-Antigravity-Cloud-URL"); got != "https://secret-tunnel.example.com" {
		t.Errorf("authenticated response missing X-Antigravity-Cloud-URL, got %q", got)
	}
}

func TestExtractLangFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedLang string
		expectedArgs []string
	}{
		{
			name:         "no flag",
			args:         []string{"pair", "-port", "58900"},
			expectedLang: "",
			expectedArgs: []string{"pair", "-port", "58900"},
		},
		{
			name:         "separated flag",
			args:         []string{"--lang", "en", "pair"},
			expectedLang: "en",
			expectedArgs: []string{"pair"},
		},
		{
			name:         "equals flag",
			args:         []string{"run", "--lang=zh"},
			expectedLang: "zh",
			expectedArgs: []string{"run"},
		},
		{
			name:         "single dash equals flag",
			args:         []string{"-lang=en", "help"},
			expectedLang: "en",
			expectedArgs: []string{"help"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := make([]string, len(tc.args))
			copy(args, tc.args)
			lang := extractLangFlag(&args)
			if lang != tc.expectedLang {
				t.Fatalf("expected lang %q, got %q", tc.expectedLang, lang)
			}
			if len(args) != len(tc.expectedArgs) {
				t.Fatalf("expected args %+v, got %+v", tc.expectedArgs, args)
			}
			for i := range args {
				if args[i] != tc.expectedArgs[i] {
					t.Errorf("arg[%d] expected %q, got %q", i, tc.expectedArgs[i], args[i])
				}
			}
		})
	}
}

func TestPushTokenCrossDeviceProtection(t *testing.T) {
	tmpDir := t.TempDir()
	authStore, err := auth.NewAuthStore(filepath.Join(tmpDir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(authStore.Close)

	tokA := "tok_device_a_secret"
	if err := authStore.AddDevice(auth.PairedDevice{
		DeviceID:   "dev_a",
		DeviceName: "Phone A",
		Platform:   "ios",
		TokenHash:  auth.HashToken(tokA),
		CreatedAt:  time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	pm := auth.NewPairingManager()
	authHandler := auth.NewAuthHandler(authStore, pm, "127.0.0.1", 58900, false)
	t.Cleanup(authHandler.Close)
	insp := inspector.NewInspector(10 * time.Second)
	p := proxy.NewProxy(insp)
	router := buildRouter(authStore, authHandler, p, insp, time.Now(), web.Handler(), auth.AuthPolicy{TrustLAN: true})

	// 1. Paired device dev_a attempting to modify dev_b's push token -> 403 Forbidden (LOW-01)
	postCross := httptest.NewRequest(http.MethodPost, "/api/v1/device/push-token", strings.NewReader(`{"platform":"ios","token_type":"alert","device_id":"dev_b","token":"abcd"}`))
	postCross.RemoteAddr = "203.0.113.10:12345"
	postCross.Header.Set("Authorization", "Bearer "+tokA)
	rrCross := httptest.NewRecorder()
	router.ServeHTTP(rrCross, postCross)
	if rrCross.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-device push token update, got %d: %s", rrCross.Code, rrCross.Body.String())
	}

	// 2. Unpaired TrustLAN synthetic caller attempting to delete dev_a's push token -> 403 Forbidden
	delLAN := httptest.NewRequest(http.MethodDelete, "/api/v1/device/push-token?device_id=dev_a", nil)
	delLAN.RemoteAddr = "192.168.1.88:5555"
	delLAN.Host = "192.168.1.1:58900"
	rrLAN := httptest.NewRecorder()
	router.ServeHTTP(rrLAN, delLAN)
	if rrLAN.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for TrustLAN push token deletion, got %d: %s", rrLAN.Code, rrLAN.Body.String())
	}
}

func TestNormalizeTopLevelCommand(t *testing.T) {
	tests := []struct {
		arg  string
		want string
	}{
		{"help", "help"},
		{"-h", "help"},
		{"-help", "help"},
		{"--help", "help"},
		{"version", "version"},
		{"-v", "version"},
		{"-version", "version"},
		{"--version", "version"},
		{"pair", "pair"},
		{"-port", "-port"},
	}
	for _, tc := range tests {
		if got := normalizeTopLevelCommand(tc.arg); got != tc.want {
			t.Errorf("normalizeTopLevelCommand(%q) = %q, want %q", tc.arg, got, tc.want)
		}
	}
}

func TestHealthz_HidesPubKeyOnlyWhenRelayAndDisableWeb(t *testing.T) {
	tmpDir := t.TempDir()
	authStore, err := auth.NewAuthStore(filepath.Join(tmpDir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(authStore.Close)

	e2eeID, err := e2ee.LoadOrCreateIdentity(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	pm := auth.NewPairingManager()
	authHandler := auth.NewAuthHandler(authStore, pm, "127.0.0.1", 58900, false)
	t.Cleanup(authHandler.Close)
	authHandler.SetE2EEIdentity(e2eeID)

	insp := inspector.NewInspector(10 * time.Second)
	p := proxy.NewProxy(insp)

	// 1. Default relay (web enabled): /healthz MUST keep e2ee_pub for Web/PWA pairing.
	routerWebOn := buildRouterWithOptions(authStore, authHandler, p, insp, time.Now(), web.Handler(), auth.AuthPolicy{}, routerOptions{
		RelayDisableWeb: false,
	})
	reqRelayOn := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	reqRelayOn = reqRelayOn.WithContext(e2ee.WithRelayTransport(reqRelayOn.Context()))
	rrOn := httptest.NewRecorder()
	routerWebOn.ServeHTTP(rrOn, reqRelayOn)
	if rrOn.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rrOn.Code)
	}
	var bodyOn map[string]any
	if err := json.NewDecoder(rrOn.Body).Decode(&bodyOn); err != nil {
		t.Fatal(err)
	}
	if bodyOn["e2ee_pub"] != e2eeID.PublicKeyB64() {
		t.Fatalf("expected e2ee_pub to remain visible when web enabled on relay, got %+v", bodyOn)
	}
	if bodyOn["e2ee_required"] != true {
		t.Fatalf("expected e2ee_required=true on relay transport, got %+v", bodyOn)
	}

	// 2. MGY_RELAY_DISABLE_WEB=1 on relay transport: hide e2ee_pub, version, os, platform; keep caps & e2ee=true.
	routerWebOff := buildRouterWithOptions(authStore, authHandler, p, insp, time.Now(), web.Handler(), auth.AuthPolicy{}, routerOptions{
		RelayDisableWeb: true,
	})
	reqRelayOff := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	reqRelayOff = reqRelayOff.WithContext(e2ee.WithRelayTransport(reqRelayOff.Context()))
	rrOff := httptest.NewRecorder()
	routerWebOff.ServeHTTP(rrOff, reqRelayOff)
	if rrOff.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rrOff.Code)
	}
	if hdr := rrOff.Header().Get(e2ee.HeaderE2EECaps); hdr != e2ee.SupportedCaps {
		t.Fatalf("expected %s=%q, got %q", e2ee.HeaderE2EECaps, e2ee.SupportedCaps, hdr)
	}
	var bodyOff map[string]any
	if err := json.NewDecoder(rrOff.Body).Decode(&bodyOff); err != nil {
		t.Fatal(err)
	}
	for _, hidden := range []string{"e2ee_pub", "version", "os", "platform"} {
		if _, exists := bodyOff[hidden]; exists {
			t.Fatalf("expected %q hidden on relay /healthz when MGY_RELAY_DISABLE_WEB=1, got %+v", hidden, bodyOff)
		}
	}
	if bodyOff["e2ee"] != true || bodyOff["e2ee_required"] != true {
		t.Fatalf("expected e2ee=true and e2ee_required=true, got %+v", bodyOff)
	}

	// 3. Direct LAN/loopback request with MGY_RELAY_DISABLE_WEB=1 still exposes normal /healthz fields.
	reqLocal := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rrLocal := httptest.NewRecorder()
	routerWebOff.ServeHTTP(rrLocal, reqLocal)
	var bodyLocal map[string]any
	if err := json.NewDecoder(rrLocal.Body).Decode(&bodyLocal); err != nil {
		t.Fatal(err)
	}
	if bodyLocal["e2ee_pub"] != e2eeID.PublicKeyB64() {
		t.Fatalf("local /healthz should keep e2ee_pub even when MGY_RELAY_DISABLE_WEB=1, got %+v", bodyLocal)
	}
}

func TestWrapForceE2EEHandler_AllowsLocalLanTailscale_BlocksProxiedAndSupportsWS(t *testing.T) {
	tmpDir := t.TempDir()
	authStore, err := auth.NewAuthStore(filepath.Join(tmpDir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(authStore.Close)

	e2eeID, err := e2ee.LoadOrCreateIdentity(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	devID := "dev-force-e2ee"
	devTok := "tok-force-e2ee-secret"
	devKey := bytes.Repeat([]byte{0x42}, 32)
	if err := authStore.AddDevice(auth.PairedDevice{
		DeviceID:   devID,
		DeviceName: "iPhone",
		Platform:   "ios",
		TokenHash:  auth.HashToken(devTok),
		E2EEKeyB64: base64.RawURLEncoding.EncodeToString(devKey),
		E2EEReady:  true,
		CreatedAt:  time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	pm := auth.NewPairingManager()
	authHandler := auth.NewAuthHandler(authStore, pm, "127.0.0.1", 58900, false)
	t.Cleanup(authHandler.Close)
	authHandler.SetE2EEIdentity(e2eeID)
	authHandler.SetForceE2EE(true)

	insp := inspector.NewInspector(10 * time.Second)
	p := proxy.NewProxy(insp)

	// Start isolated internal loopback WS listener just like runGatewayServerWithMode does when MGY_FORCE_E2EE=1.
	innerLn, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = innerLn.Close() })

	router := buildRouterWithOptions(authStore, authHandler, p, insp, time.Now(), web.Handler(), auth.AuthPolicy{TrustLAN: true}, routerOptions{
		LocalWSAddr:     func() string { return innerLn.Addr().String() },
		RelayDisableWeb: true,
	})

	// Serve a simple WS echo endpoint on innerLn alongside raw router to prove e2ee.HandleWS dials without 426.
	innerMux := http.NewServeMux()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	innerMux.HandleFunc("/gateway/events", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		mt, msg, err := conn.ReadMessage()
		if err == nil {
			_ = conn.WriteMessage(mt, append([]byte("echo:"), msg...))
		}
	})
	innerMux.Handle("/", router)
	innerSrv := &http.Server{Handler: innerMux}
	go innerSrv.Serve(innerLn)
	t.Cleanup(func() { _ = innerSrv.Close() })

	forced := wrapForceE2EEHandler(router, true)

	// 1. Direct loopback CLI request passes without 426.
	reqLoop := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	reqLoop.RemoteAddr = "127.0.0.1:54321"
	rrLoop := httptest.NewRecorder()
	forced.ServeHTTP(rrLoop, reqLoop)
	if rrLoop.Code != http.StatusOK {
		t.Fatalf("expected 200 for loopback request, got %d", rrLoop.Code)
	}

	// 2. Direct LAN and Tailscale CGNAT requests pass without 426.
	for _, addr := range []string{"192.168.50.10:12345", "100.100.42.5:12345"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
		req.RemoteAddr = addr
		rr := httptest.NewRecorder()
		forced.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 for direct private/Tailscale addr %s, got %d", addr, rr.Code)
		}
	}

	// 3. Proxied plaintext API request gets 426 Upgrade Required.
	reqProxied := httptest.NewRequest(http.MethodGet, "/api/v1/version", nil)
	reqProxied.RemoteAddr = "127.0.0.1:54321"
	reqProxied.Header.Set("X-Forwarded-For", "198.51.100.9")
	reqProxied.Header.Set("X-Forwarded-Proto", "https")
	rrProxied := httptest.NewRecorder()
	forced.ServeHTTP(rrProxied, reqProxied)
	if rrProxied.Code != http.StatusUpgradeRequired {
		t.Fatalf("expected 426 for proxied plaintext API under ForceE2EE, got %d body=%s", rrProxied.Code, rrProxied.Body.String())
	}

	// 4. Proxied static web shell gets 403 when RelayDisableWeb=true.
	reqWeb := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	reqWeb.RemoteAddr = "198.51.100.9:443"
	rrWeb := httptest.NewRecorder()
	forced.ServeHTTP(rrWeb, reqWeb)
	if rrWeb.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for public web shell when RelayDisableWeb=true, got %d", rrWeb.Code)
	}

	// 5. E2EE WS tunnel through the ForceE2EE outer server succeeds because HandleWS dials isolated innerWSServer.
	outerSrv := httptest.NewServer(forced)
	t.Cleanup(outerSrv.Close)
	wsURL := "ws" + strings.TrimPrefix(outerSrv.URL, "http") + "/api/v1/e2ee/ws?d=" + devID
	hdr := http.Header{
		"X-Forwarded-For":   []string{"198.51.100.9"},
		"X-Forwarded-Proto": []string{"https"},
	}
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial /api/v1/e2ee/ws under ForceE2EE failed: %v (resp=%v)", err, resp)
	}
	defer conn.Close()

	openPayload, _ := json.Marshal(e2ee.WSOpenFrame{
		Path:    "/gateway/events",
		Headers: map[string]string{"Authorization": "Bearer " + devTok},
		Seq:     "v1",
		TSMs:    time.Now().UnixMilli(),
		ReqID:   "ws-force-e2ee-test-01",
	})
	encOpen, err := e2ee.Encrypt(devKey, openPayload, e2ee.FrameAAD(devID, "ws-open"))
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, encOpen); err != nil {
		t.Fatal(err)
	}
	c2sPlain := append([]byte{byte(websocket.TextMessage)}, []byte("ping")...)
	c2sEnc, err := e2ee.Encrypt(devKey, c2sPlain, e2ee.FrameSeqAAD(devID, "ws-c2s", 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, c2sEnc); err != nil {
		t.Fatal(err)
	}
	_, encReply, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read encrypted ws reply failed: %v", err)
	}
	plainReply, err := e2ee.Decrypt(devKey, encReply, e2ee.FrameSeqAAD(devID, "ws-s2c", 0))
	if err != nil {
		t.Fatalf("decrypt ws reply with seq=0 failed: %v", err)
	}
	if got := string(plainReply[1:]); got != "echo:ping" {
		t.Fatalf("expected echo:ping, got %q", got)
	}
}

func TestResolveCustomRelayResult(t *testing.T) {
	tok, err := relay.SignToken("test-relay-secret-32-bytes-minimum", relay.TokenClaims{
		ShortID:   "a1b2c3d4",
		OwnerHash: "0123456789abcdef0123456789abcdef",
		Domain:    "dev.example.com",
		IssuedAt:  time.Now().Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// 1. Auto-derives PublicURL and Subdomain from signed token when PublicURL is omitted.
	res, err := resolveCustomRelayResult(config.CustomRelayConfig{
		Endpoint: "wss://relay.example.com/_mgy_tunnel",
		Token:    tok,
	})
	if err != nil {
		t.Fatalf("resolveCustomRelayResult auto-derive failed: %v", err)
	}
	if res.Subdomain != "dev.example.com" || res.URL != "https://dev.example.com" {
		t.Fatalf("unexpected auto-derived result: %+v", res)
	}

	// 2. Explicit PublicURL override takes precedence.
	res2, err := resolveCustomRelayResult(config.CustomRelayConfig{
		Endpoint:  "wss://relay.example.com/_mgy_tunnel",
		Token:     tok,
		PublicURL: "https://custom.example.org/",
	})
	if err != nil {
		t.Fatalf("resolveCustomRelayResult override failed: %v", err)
	}
	if res2.Subdomain != "custom.example.org" || res2.URL != "https://custom.example.org" {
		t.Fatalf("unexpected override result: %+v", res2)
	}
}



