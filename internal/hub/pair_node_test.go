package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"antigravity-mobile/internal/e2ee"
)

func freeUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	_ = conn.Close()
	return port
}

func TestStandalonePassiveDiscoveryAndSecondaryPairCodeFlow(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeDir := t.TempDir()
	hubDir := t.TempDir()

	var (
		promptMu   sync.Mutex
		promptCode string
		promptHub  string
	)

	// Start a mock ordinary standalone `mgy` server protected by NodeAgent token validation.
	var nodeAgent *NodeAgent
	var nodeServer *httptest.Server

	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := strings.TrimSpace(r.Header.Get(HeaderHubToken))
		if _, ok := nodeAgent.ValidateHubRequestToken(tok); !ok {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if r.URL.Path == "/gateway/status" {
			writeJSON(w, http.StatusOK, map[string]any{
				"status":       "connected",
				"gateway_name": "MacMini-Standalone",
				"os":           "darwin",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"ok": "1"})
	})

	var err error
	nodeServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nodeAgent.WrapHandler(innerHandler).ServeHTTP(w, r)
	}))
	defer nodeServer.Close()

	nodeURL, _ := url.Parse(nodeServer.URL)
	nodePort, _ := strconv.Atoi(nodeURL.Port())

	relayActivated := false
	nodeAgent, err = NewNodeAgent(NodeAgentOptions{
		DataDir:           nodeDir,
		NodeName:          "MacMini-Standalone",
		HTTPPort:          nodePort,
		AdvertiseBaseURL:  nodeServer.URL,
		Version:           "1.2.8-test",
		DisableDiscovery:  true,
		PassiveStandalone: true,
		OnRelayActivated:  func() { relayActivated = true },
		OnPairPrompt: func(hubName, requesterIP, code string) {
			promptMu.Lock()
			promptHub = hubName
			promptCode = code
			promptMu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("NewNodeAgent: %v", err)
	}

	// Start Hub manager
	hubMgr, err := NewHub(Options{
		HubID:            "hub-home-01",
		HubName:          "HomeHub",
		Headless:         true,
		DataDir:          hubDir,
		Secret:           "hub-cluster-secret-not-shared-with-standalone",
		DisableDiscovery: true,
		AdvertiseURL:     "http://127.0.0.1:58900",
	})
	if err != nil {
		t.Fatalf("NewHub: %v", err)
	}
	defer hubMgr.Stop()

	// Simulate UDP discovery packet received from standalone mgy (no ClusterHash match).
	hubMgr.handleDiscoveryPacket(DiscoveryPacket{
		Type:      pktTypeNodeAnnounce,
		NodeID:    nodeAgent.NodeID(),
		Name:      nodeAgent.NodeName(),
		OS:        "darwin",
		Mode:      "standalone",
		Port:      nodePort,
		Version:   "1.2.8-test",
		LSHealthy: true,
	}, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 58901})

	// 1. Standalone host MUST NOT auto-join without confirmation code.
	if nodes := hubMgr.Store().ListNodes(); len(nodes) != 0 {
		t.Fatalf("expected standalone mgy NOT to auto-join without pair code, got %d nodes", len(nodes))
	}
	disc := hubMgr.DiscoverableNodes()
	if len(disc) != 1 || disc[0].Mode != "standalone" || !disc[0].RequiresPairCode || disc[0].HasSecretMatch {
		t.Fatalf("unexpected discovered node state: %+v", disc)
	}

	// 2. Hub initiates pairing by discovered name ("MacMini-Standalone") -> triggers 6-char prompt on target.
	started, err := hubMgr.StartPairWithTarget(ctx, "MacMini-Standalone")
	if err != nil {
		t.Fatalf("StartPairWithTarget: %v", err)
	}
	if started.ChallengeID == "" || started.NodeID != nodeAgent.NodeID() {
		t.Fatalf("unexpected start result: %+v", started)
	}

	promptMu.Lock()
	gotCode := promptCode
	gotHub := promptHub
	promptMu.Unlock()
	if len(gotCode) != 6 || gotHub != "HomeHub" {
		t.Fatalf("expected 6-char code prompt for HomeHub, got hub=%q code=%q", gotHub, gotCode)
	}

	// 3. Wrong code burns challenge after 3 failed attempts.
	for i := 1; i <= 3; i++ {
		_, cErr := hubMgr.ConfirmPairWithTarget(ctx, HubPairConfirmInput{
			BaseURL:     started.BaseURL,
			ChallengeID: started.ChallengeID,
			Code:        "WRONG1",
		})
		if cErr == nil {
			t.Fatalf("attempt %d: expected error for wrong confirmation code", i)
		}
		pe, ok := cErr.(*HubPairError)
		if !ok || pe.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401 HubPairError, got %v", i, cErr)
		}
		if i < 3 && pe.AttemptsRemaining != 3-i {
			t.Fatalf("attempt %d: expected %d remaining, got %d", i, 3-i, pe.AttemptsRemaining)
		}
	}
	// Even submitting the real code now MUST fail because the challenge was burned!
	if _, err := hubMgr.ConfirmPairWithTarget(ctx, HubPairConfirmInput{
		BaseURL:     started.BaseURL,
		ChallengeID: started.ChallengeID,
		Code:        gotCode,
	}); err == nil {
		t.Fatalf("expected burned challenge to reject even the previously valid code")
	}

	// 4. Another Hub cannot hijack a challenge created by HomeHub.
	started2, err := hubMgr.StartPairWithTarget(ctx, "MacMini-Standalone")
	if err != nil {
		t.Fatalf("StartPairWithTarget #2: %v", err)
	}
	promptMu.Lock()
	gotCode2 := promptCode
	promptMu.Unlock()

	rogueHub, err := NewHub(Options{
		HubID:            "hub-rogue-99",
		HubName:          "RogueHub",
		Headless:         true,
		DataDir:          t.TempDir(),
		DisableDiscovery: true,
	})
	if err != nil {
		t.Fatalf("NewHub rogue: %v", err)
	}
	defer rogueHub.Stop()
	if _, err := rogueHub.ConfirmPairWithTarget(ctx, HubPairConfirmInput{
		BaseURL:     started2.BaseURL,
		ChallengeID: started2.ChallengeID,
		Code:        gotCode2,
	}); err == nil {
		t.Fatalf("expected rogue Hub to be blocked from confirming another Hub's challenge")
	}

	// 5. Genuine HomeHub confirms using lowercase + space-formatted code ("xxx xxx").
	formattedLower := strings.ToLower(FormatPairCode(gotCode2))
	cfg, err := hubMgr.ConfirmPairWithTarget(ctx, HubPairConfirmInput{
		BaseURL:     started2.BaseURL,
		ChallengeID: started2.ChallengeID,
		Code:        formattedLower,
	})
	if err != nil {
		t.Fatalf("ConfirmPairWithTarget valid code: %v", err)
	}
	if cfg.ID != nodeAgent.NodeID() || cfg.Token == "" {
		t.Fatalf("unexpected paired NodeConfig: %+v", cfg)
	}

	// Hub should immediately see the node online & healthy.
	st := hubMgr.BuildStatusResponse(false)
	if st.OnlineNodeCount != 1 || len(st.Discovered) != 0 {
		t.Fatalf("expected 1 online node and 0 pending discoveries, got online=%d disc=%d", st.OnlineNodeCount, len(st.Discovered))
	}

	// Persisted hub_links.json should contain HomeHub and survive reload.
	links, err := LoadHubLinks(nodeDir)
	if err != nil || len(links) != 1 || links[0].HubID != "hub-home-01" {
		t.Fatalf("expected persisted hub_links.json with hub-home-01, got links=%+v err=%v", links, err)
	}
	_ = relayActivated // notifier was nil in this unit test; relay activation tested separately below.

	// 6. Revoking Hub link on the target machine immediately invalidates the Hub's token.
	if _, ok := nodeAgent.RevokeHubLink("HomeHub"); !ok {
		t.Fatalf("expected RevokeHubLink(HomeHub) to succeed")
	}
	if _, valid := nodeAgent.ValidateHubRequestToken(cfg.Token); valid {
		t.Fatalf("expected revoked Hub token to be rejected immediately")
	}
	if remaining, _ := LoadHubLinks(nodeDir); len(remaining) != 0 {
		t.Fatalf("expected empty hub_links.json after revoke, got %+v", remaining)
	}
}

func TestLocalPairCodeAndSecurityGuards(t *testing.T) {
	nodeDir := t.TempDir()
	nodeAgent, err := NewNodeAgent(NodeAgentOptions{
		DataDir:           nodeDir,
		NodeName:          "BgHost",
		HTTPPort:          58920,
		AdvertiseBaseURL:  "http://127.0.0.1:58920",
		DisableDiscovery:  true,
		PassiveStandalone: true,
		OnPairPrompt:      func(string, string, string) {},
	})
	if err != nil {
		t.Fatalf("NewNodeAgent: %v", err)
	}

	handler := nodeAgent.WrapHandler(http.NotFoundHandler())

	// 1. Proxied (X-Forwarded-For), Relay transport, and Public IP MUST be rejected with 403.
	for _, tc := range []struct {
		name  string
		path  string
		setup func(*http.Request)
	}{
		{
			name: "proxied pair-request",
			path: "/api/v1/hub/pair-request",
			setup: func(r *http.Request) {
				r.RemoteAddr = "192.168.50.10:12345"
				r.Header.Set("X-Forwarded-For", "1.2.3.4")
			},
		},
		{
			name: "public IP pair-request",
			path: "/api/v1/hub/pair-request",
			setup: func(r *http.Request) {
				r.RemoteAddr = "8.8.8.8:12345"
			},
		},
		{
			name: "relay transport pair-request",
			path: "/api/v1/hub/pair-request",
			setup: func(r *http.Request) {
				r.RemoteAddr = "127.0.0.1:12345"
				*r = *r.WithContext(e2ee.WithRelayTransport(r.Context()))
			},
		},
		{
			name: "proxied pair-confirm",
			path: "/api/v1/hub/pair-confirm",
			setup: func(r *http.Request) {
				r.RemoteAddr = "192.168.50.10:12345"
				r.Header.Set("CF-Connecting-IP", "1.2.3.4")
			},
		},
		{
			name: "public IP verify",
			path: "/api/v1/hub/verify",
			setup: func(r *http.Request) {
				r.RemoteAddr = "203.0.113.5:5555"
			},
		},
		{
			name: "LAN non-admin pair-code",
			path: "/api/v1/hub/pair-code",
			setup: func(r *http.Request) {
				r.RemoteAddr = "192.168.50.99:5555"
			},
		},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(`{}`))
		tc.setup(req)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: expected 403 Forbidden, got %d (%s)", tc.name, rec.Code, rec.Body.String())
		}
	}

	// 2. Per-IP rate limit on pair-request: 2 allowed per minute, 3rd returns 429.
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/hub/pair-request", strings.NewReader(`{"hub_id":"h1","hub_name":"H1"}`))
		req.RemoteAddr = "192.168.50.25:40000"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if i <= 2 && rec.Code != http.StatusOK {
			t.Fatalf("pair-request #%d: expected 200, got %d (%s)", i, rec.Code, rec.Body.String())
		}
		if i == 3 && rec.Code != http.StatusTooManyRequests {
			t.Fatalf("pair-request #%d: expected 429 Too Many Requests, got %d", i, rec.Code)
		}
	}

	// 3. Background-host `mgy pair -hub` flow: local loopback issues code -> Hub confirms without challenge_id.
	codeReq := httptest.NewRequest(http.MethodPost, "/api/v1/hub/pair-code", nil)
	codeReq.RemoteAddr = "127.0.0.1:41000"
	codeRec := httptest.NewRecorder()
	handler.ServeHTTP(codeRec, codeReq)
	if codeRec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/hub/pair-code: expected 200, got %d (%s)", codeRec.Code, codeRec.Body.String())
	}
	var codeResp HubPairCodeResponse
	if err := json.Unmarshal(codeRec.Body.Bytes(), &codeResp); err != nil || len(codeResp.Code) != 6 {
		t.Fatalf("invalid pair-code response: %s (err=%v)", codeRec.Body.String(), err)
	}

	confirmBody, _ := json.Marshal(HubPairConfirmPayload{
		Code:    codeResp.FormattedCode,
		HubID:   "hub-bg-01",
		HubName: "NAS-Hub",
		HubURL:  "http://192.168.50.39:58900",
	})
	confirmReq := httptest.NewRequest(http.MethodPost, "/api/v1/hub/pair-confirm", bytes.NewReader(confirmBody))
	confirmReq.RemoteAddr = "192.168.50.39:42000"
	confirmRec := httptest.NewRecorder()
	handler.ServeHTTP(confirmRec, confirmReq)
	if confirmRec.Code != http.StatusOK {
		t.Fatalf("pair-confirm via local code failed: %d (%s)", confirmRec.Code, confirmRec.Body.String())
	}

	// GET /api/v1/hub/links returns redacted view (never leaks node_token).
	linksReq := httptest.NewRequest(http.MethodGet, "/api/v1/hub/links", nil)
	linksReq.RemoteAddr = "127.0.0.1:43000"
	linksRec := httptest.NewRecorder()
	handler.ServeHTTP(linksRec, linksReq)
	if linksRec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/hub/links: expected 200, got %d", linksRec.Code)
	}
	if strings.Contains(linksRec.Body.String(), "mgy_node_") {
		t.Fatalf("GET /api/v1/hub/links must NOT leak node_token: %s", linksRec.Body.String())
	}
}

func TestDHCPMigrationHMACVerificationBlocksSpoofing(t *testing.T) {
	nodeDir := t.TempDir()
	hubDir := t.TempDir()

	// Genuine node starts at old URL (simulated) and later moves to newServer's URL.
	nodeAgent, err := NewNodeAgent(NodeAgentOptions{
		DataDir:           nodeDir,
		NodeName:          "MacBook",
		HTTPPort:          58930,
		DisableDiscovery:  true,
		PassiveStandalone: true,
		OnPairPrompt:      func(string, string, string) {},
	})
	if err != nil {
		t.Fatalf("NewNodeAgent: %v", err)
	}

	const hubID = "hub-verify-01"
	const sharedToken = "mgy_node_0123456789abcdef0123456789abcdef0123456789abcdef01234567"
	if err := nodeAgent.UpsertHubLink(HubLink{
		HubID:     hubID,
		HubName:   "HomeHub",
		HubURL:    "http://127.0.0.1:58900",
		NodeToken: sharedToken,
	}); err != nil {
		t.Fatalf("UpsertHubLink: %v", err)
	}

	hubMgr, err := NewHub(Options{
		HubID:            hubID,
		HubName:          "HomeHub",
		Headless:         true,
		DataDir:          hubDir,
		DisableDiscovery: true,
	})
	if err != nil {
		t.Fatalf("NewHub: %v", err)
	}
	defer hubMgr.Stop()

	oldURL := "http://127.0.0.1:59991"
	if _, err := hubMgr.Store().UpsertNode(NodeConfig{
		ID:      nodeAgent.NodeID(),
		Name:    "MacBook",
		BaseURL: oldURL,
		Token:   sharedToken,
		Enabled: true,
	}); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	// 1. Rogue server spoofing the node's ID: MUST NOT receive nodeToken and MUST NOT hijack BaseURL.
	rogueSawToken := false
	rogueServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(HeaderHubToken) != "" || r.Header.Get("Authorization") != "" {
			rogueSawToken = true
		}
		writeJSON(w, http.StatusOK, HubVerifyResponse{
			NodeID: nodeAgent.NodeID(),
			Proof:  "deadbeef-forged-proof",
		})
	}))
	defer rogueServer.Close()

	if ok := hubMgr.verifyAndMigrateNodeIP(NodeConfig{
		ID:      nodeAgent.NodeID(),
		Name:    "MacBook",
		BaseURL: oldURL,
		Token:   sharedToken,
		Enabled: true,
	}, rogueServer.URL, "MacBook"); ok {
		t.Fatalf("expected rogue server with forged proof to be REJECTED")
	}
	if rogueSawToken {
		t.Fatalf("SECURITY VIOLATION: nodeToken was transmitted to an unverified IP!")
	}
	cur, _ := hubMgr.Store().GetNode(nodeAgent.NodeID())
	if cur.BaseURL != oldURL {
		t.Fatalf("expected BaseURL to remain %s after spoofing attempt, got %s", oldURL, cur.BaseURL)
	}

	// 2. Genuine node at new DHCP address passes HMAC proof and updates BaseURL cleanly.
	genuineNewServer := httptest.NewServer(nodeAgent.WrapHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gateway/status" {
			writeJSON(w, http.StatusOK, map[string]any{"status": "connected", "gateway_name": "MacBook"})
			return
		}
		http.NotFound(w, r)
	})))
	defer genuineNewServer.Close()

	if ok := hubMgr.verifyAndMigrateNodeIP(cur, genuineNewServer.URL, "MacBook"); !ok {
		t.Fatalf("expected genuine node at new IP to pass HMAC verification")
	}
	updated, _ := hubMgr.Store().GetNode(nodeAgent.NodeID())
	if updated.BaseURL != genuineNewServer.URL {
		t.Fatalf("expected BaseURL to migrate to %s, got %s", genuineNewServer.URL, updated.BaseURL)
	}
}

func TestPassiveStandaloneUDPScanFromHub(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	discPort := freeUDPPort(t)
	nodeDir := t.TempDir()
	hubDir := t.TempDir()

	// Start passive standalone node listener on discPort.
	nodeAgent, err := NewNodeAgent(NodeAgentOptions{
		DataDir:           nodeDir,
		NodeName:          "PassiveMac",
		HTTPPort:          58944,
		Version:           "1.2.8-test",
		DiscoveryPort:     discPort,
		PassiveStandalone: true,
		IsLSHealthy:       func() bool { return true },
	})
	if err != nil {
		t.Fatalf("NewNodeAgent: %v", err)
	}
	nodeAgent.Start(ctx)
	time.Sleep(80 * time.Millisecond)

	// Start Hub on same host (falls back to ephemeral UDP socket, probes 127.0.0.1:discPort).
	hubMgr, err := NewHub(Options{
		HubID:         "hub-udp-scan-01",
		HubName:       "ScanHub",
		Headless:      true,
		DataDir:       hubDir,
		DiscoveryPort: discPort,
	})
	if err != nil {
		t.Fatalf("NewHub: %v", err)
	}
	defer hubMgr.Stop()
	hubMgr.Start(ctx)

	discovered := hubMgr.ScanLAN(ctx, 400*time.Millisecond)
	found := false
	for _, d := range discovered {
		if d.NodeID == nodeAgent.NodeID() && d.Name == "PassiveMac" && d.Port == 58944 && d.Mode == "standalone" && d.RequiresPairCode {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected Hub startup/on-demand ScanLAN to discover passive standalone node, got %+v", discovered)
	}
	// Must NOT auto-join without confirmation code.
	if len(hubMgr.Store().ListNodes()) != 0 {
		t.Fatalf("expected passive standalone node NOT to auto-join during ScanLAN")
	}
}
