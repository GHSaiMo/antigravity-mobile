package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"antigravity-mobile/internal/auth"
	"antigravity-mobile/internal/hub"
	"antigravity-mobile/internal/inspector"
	"antigravity-mobile/internal/proxy"
	"antigravity-mobile/web"
)

type noopInspector struct{}

func (noopInspector) Current() *inspector.InstanceInfo     { return nil }
func (noopInspector) Scan() *inspector.InstanceInfo        { return nil }
func (noopInspector) Start()                               {}
func (noopInspector) Stop()                                {}
func (noopInspector) OnUpdate(func(inspector.InstanceInfo)) {}

func TestBuildRouterWithHubAuthAndRegistration(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	authStore, err := auth.NewAuthStore(filepath.Join(tmp, "devices.json"))
	if err != nil {
		t.Fatalf("NewAuthStore: %v", err)
	}
	defer authStore.Close()

	insp := noopInspector{}
	p := proxy.NewProxy(insp)
	pairingMgr := auth.NewPairingManager()
	authHandler := auth.NewAuthHandler(authStore, pairingMgr, "127.0.0.1", 58900, false)
	defer authHandler.Close()

	hubMgr, err := hub.NewHub(hub.Options{
		HubID:            "hub-router-test",
		HubName:          "HubTest",
		Headless:         true,
		DataDir:          tmp,
		Secret:           "router-hub-secret",
		DisableDiscovery: true,
		Inspector:        insp,
		Proxy:            p,
	})
	if err != nil {
		t.Fatalf("NewHub: %v", err)
	}
	defer hubMgr.Stop()

	policy := auth.AuthPolicy{
		TrustLAN:          false,
		ListenLoopback:    false,
		HubTokenValidator: hubMgr.ValidateHubAuthToken,
	}
	router := buildRouterWithHub(authStore, authHandler, p, insp, time.Now(), web.Handler(), policy, nil, hubMgr)

	// 1. Remote LAN caller without token cannot access /gateway/hub/status (401)
	unauthReq := httptest.NewRequest(http.MethodGet, "/gateway/hub/status", nil)
	unauthReq.RemoteAddr = "192.168.50.55:51234"
	unauthRec := httptest.NewRecorder()
	router.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated LAN /gateway/hub/status, got %d", unauthRec.Code)
	}

	// 2. Remote LAN node CAN reach /api/v1/hub/register (whitelisted like /api/v1/auth/pair)
	//    Bad secret -> 401; valid secret -> 200 and returns node_token.
	badRegReq := httptest.NewRequest(http.MethodPost, "/api/v1/hub/register", strings.NewReader(`{"name":"M1","port":58900,"secret":"wrong"}`))
	badRegReq.RemoteAddr = "192.168.50.55:51234"
	badRegRec := httptest.NewRecorder()
	router.ServeHTTP(badRegRec, badRegReq)
	if badRegRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad hub register secret, got %d", badRegRec.Code)
	}

	goodRegReq := httptest.NewRequest(http.MethodPost, "/api/v1/hub/register", strings.NewReader(`{"node_id":"m1","name":"M1","port":58900,"secret":"router-hub-secret"}`))
	goodRegReq.RemoteAddr = "192.168.50.55:51234"
	goodRegRec := httptest.NewRecorder()
	router.ServeHTTP(goodRegRec, goodRegReq)
	if goodRegRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for valid hub register, got %d: %s", goodRegRec.Code, goodRegRec.Body.String())
	}

	// 3. Registered node can call /api/v1/hub/internal/event using its derived token even with TrustLAN=false
	nodeTok := hub.DeriveNodeToken("router-hub-secret", "m1")
	evReq := httptest.NewRequest(http.MethodPost, "/api/v1/hub/internal/event", strings.NewReader(`{"type":"list_changed"}`))
	evReq.RemoteAddr = "192.168.50.55:51234"
	evReq.Header.Set(hub.HeaderHubToken, nodeTok)
	evRec := httptest.NewRecorder()
	router.ServeHTTP(evRec, evReq)
	if evRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for node internal event with valid token, got %d: %s", evRec.Code, evRec.Body.String())
	}
}

func TestBuildRouterWithStandalonePassiveNodePairing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	authStore, err := auth.NewAuthStore(filepath.Join(tmp, "devices.json"))
	if err != nil {
		t.Fatalf("NewAuthStore: %v", err)
	}
	defer authStore.Close()

	insp := noopInspector{}
	p := proxy.NewProxy(insp)
	pairingMgr := auth.NewPairingManager()
	authHandler := auth.NewAuthHandler(authStore, pairingMgr, "127.0.0.1", 58900, false)
	defer authHandler.Close()

	var issuedCode string
	nodeAgent, err := hub.NewNodeAgent(hub.NodeAgentOptions{
		DataDir:           tmp,
		NodeName:          "StandaloneMac",
		HTTPPort:          58900,
		DisableDiscovery:  true,
		PassiveStandalone: true,
		OnPairPrompt: func(_, _, code string) {
			issuedCode = code
		},
	})
	if err != nil {
		t.Fatalf("NewNodeAgent: %v", err)
	}

	policy := auth.AuthPolicy{
		TrustLAN:          false,
		ListenLoopback:    false,
		HubTokenValidator: nodeAgent.ValidateHubRequestToken,
	}
	router := buildRouterWithOptions(authStore, authHandler, p, insp, time.Now(), web.Handler(), policy, routerOptions{
		NodeAgent: nodeAgent,
	})

	// 1. Unauthenticated LAN access to /gateway/status is rejected with 401.
	stReq := httptest.NewRequest(http.MethodGet, "/gateway/status", nil)
	stReq.RemoteAddr = "192.168.50.10:51234"
	stRec := httptest.NewRecorder()
	router.ServeHTTP(stRec, stReq)
	if stRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 before pairing, got %d", stRec.Code)
	}

	// 2. Proxied pair-request is rejected with 403.
	proxReq := httptest.NewRequest(http.MethodPost, "/api/v1/hub/pair-request", strings.NewReader(`{"hub_id":"hub-1","hub_name":"HomeHub"}`))
	proxReq.RemoteAddr = "192.168.50.10:51234"
	proxReq.Header.Set("X-Forwarded-For", "1.2.3.4")
	proxRec := httptest.NewRecorder()
	router.ServeHTTP(proxRec, proxReq)
	if proxRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for proxied pair-request, got %d", proxRec.Code)
	}

	// 3. Direct LAN pair-request -> triggers 6-char code -> pair-confirm issues node_token -> immediate 200 on /gateway/status!
	prReq := httptest.NewRequest(http.MethodPost, "/api/v1/hub/pair-request", strings.NewReader(`{"hub_id":"hub-1","hub_name":"HomeHub","hub_url":"http://192.168.50.10:58900"}`))
	prReq.RemoteAddr = "192.168.50.10:51234"
	prRec := httptest.NewRecorder()
	router.ServeHTTP(prRec, prReq)
	if prRec.Code != http.StatusOK || len(issuedCode) != 6 {
		t.Fatalf("pair-request failed: code=%d issuedCode=%q body=%s", prRec.Code, issuedCode, prRec.Body.String())
	}

	confirmBody := `{"hub_id":"hub-1","hub_name":"HomeHub","hub_url":"http://192.168.50.10:58900","code":"` + issuedCode + `"}`
	// Extract challenge_id from pair-request response for exact binding
	if idx := strings.Index(prRec.Body.String(), `"challenge_id":"`); idx >= 0 {
		rest := prRec.Body.String()[idx+len(`"challenge_id":"`):]
		if end := strings.Index(rest, `"`); end > 0 {
			cid := rest[:end]
			confirmBody = `{"challenge_id":"` + cid + `","hub_id":"hub-1","hub_name":"HomeHub","hub_url":"http://192.168.50.10:58900","code":"` + issuedCode + `"}`
		}
	}
	pcReq := httptest.NewRequest(http.MethodPost, "/api/v1/hub/pair-confirm", strings.NewReader(confirmBody))
	pcReq.RemoteAddr = "192.168.50.10:51234"
	pcRec := httptest.NewRecorder()
	router.ServeHTTP(pcRec, pcReq)
	if pcRec.Code != http.StatusOK || !strings.Contains(pcRec.Body.String(), `"node_token":"mgy_node_`) {
		t.Fatalf("pair-confirm failed: %d %s", pcRec.Code, pcRec.Body.String())
	}

	// Extract token and hit /gateway/status immediately without restarting router.
	tokIdx := strings.Index(pcRec.Body.String(), `"node_token":"`)
	rest := pcRec.Body.String()[tokIdx+len(`"node_token":"`):]
	nodeToken := rest[:strings.Index(rest, `"`)]

	authedReq := httptest.NewRequest(http.MethodGet, "/gateway/status", nil)
	authedReq.RemoteAddr = "192.168.50.10:51234"
	authedReq.Header.Set(hub.HeaderHubToken, nodeToken)
	authedRec := httptest.NewRecorder()
	router.ServeHTTP(authedRec, authedReq)
	if authedRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on /gateway/status after confirmation-code pairing, got %d: %s", authedRec.Code, authedRec.Body.String())
	}
}
