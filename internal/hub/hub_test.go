package hub

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/inspector"
	"antigravity-mobile/internal/notifier"
	"antigravity-mobile/internal/proxy"

	"github.com/gorilla/websocket"
)

type stubInspector struct {
	healthy bool
	port    int
}

func (s *stubInspector) Current() *inspector.InstanceInfo {
	if !s.healthy {
		return nil
	}
	return &inspector.InstanceInfo{PID: 1234, Port: s.port, IsHealthy: true}
}
func (s *stubInspector) Scan() *inspector.InstanceInfo     { return s.Current() }
func (s *stubInspector) Start()                          {}
func (s *stubInspector) Stop()                           {}
func (s *stubInspector) OnUpdate(func(inspector.InstanceInfo)) {}

type captureSender struct {
	mu       sync.Mutex
	payloads []notifier.PushPayload
	ch       chan notifier.PushPayload
}

func newCaptureSender() *captureSender {
	return &captureSender{ch: make(chan notifier.PushPayload, 16)}
}

func (c *captureSender) Send(_ context.Context, p notifier.PushPayload) error {
	c.mu.Lock()
	c.payloads = append(c.payloads, p)
	c.mu.Unlock()
	select {
	case c.ch <- p:
	default:
	}
	return nil
}

func TestStoreAndDiscoveryPacket(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, "top-secret-cluster")
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	if store.Secret() != "top-secret-cluster" {
		t.Fatalf("unexpected secret: %q", store.Secret())
	}
	if !store.AuthorizeJoin("top-secret-cluster", "") {
		t.Fatalf("expected secret authorization to succeed")
	}
	if !store.AuthorizeJoin("", store.JoinCode()) {
		t.Fatalf("expected join code authorization to succeed")
	}
	if store.AuthorizeJoin("wrong", "BADCOD") {
		t.Fatalf("expected wrong secret/code to be rejected")
	}

	fp := ClusterFingerprint("top-secret-cluster")
	if len(fp) != 16 || strings.Contains(fp, "top-secret") {
		t.Fatalf("invalid cluster fingerprint: %q", fp)
	}

	nc, err := ParseStaticNodeSpec("M1=http://192.168.50.12:58900/|tok123")
	if err != nil {
		t.Fatalf("ParseStaticNodeSpec failed: %v", err)
	}
	if nc.Name != "M1" || nc.BaseURL != "http://192.168.50.12:58900" || nc.Token != "tok123" {
		t.Fatalf("unexpected parsed node: %+v", nc)
	}

	// Verify discovery packet handling: matching cluster hash auto-joins, non-matching stays in discovered.
	h, err := NewHub(Options{
		HubID:            "hub-1",
		HubName:          "HubMac",
		DataDir:          dir,
		Secret:           "top-secret-cluster",
		DisableDiscovery: true,
	})
	if err != nil {
		t.Fatalf("NewHub: %v", err)
	}
	defer h.Stop()

	rawGood, _ := json.Marshal(DiscoveryPacket{
		Type:        pktTypeNodeAnnounce,
		NodeID:      "mac-m1",
		Name:        "M1",
		OS:          "darwin",
		Port:        58900,
		LSHealthy:   true,
		ClusterHash: fp,
	})
	pktGood, ok := ParseDiscoveryPacket(rawGood)
	if !ok {
		t.Fatalf("ParseDiscoveryPacket rejected valid packet")
	}
	h.handleDiscoveryPacket(pktGood, &net.UDPAddr{IP: net.IPv4(192, 168, 50, 12), Port: 58901})

	joined, ok := h.Store().GetNode("mac-m1")
	if !ok || joined.BaseURL != "http://192.168.50.12:58900" {
		t.Fatalf("expected auto-joined node mac-m1, got ok=%v node=%+v", ok, joined)
	}
	if joined.Token != DeriveNodeToken("top-secret-cluster", "mac-m1") {
		t.Fatalf("expected derived node token, got %q", joined.Token)
	}

	// Non-matching cluster hash should stay pending in Discovered, NOT auto-joined.
	h.handleDiscoveryPacket(DiscoveryPacket{
		Type:        pktTypeNodeAnnounce,
		NodeID:      "stranger-box",
		Name:        "Stranger",
		Port:        58900,
		ClusterHash: "0000000000000000",
	}, &net.UDPAddr{IP: net.IPv4(192, 168, 50, 99), Port: 58901})

	if _, exists := h.Store().GetNode("stranger-box"); exists {
		t.Fatalf("stranger node must NOT auto-join without matching secret")
	}
	status := h.BuildStatusResponse(false)
	if len(status.Discovered) != 1 || status.Discovered[0].NodeID != "stranger-box" {
		t.Fatalf("expected stranger-box in discovered list, got %+v", status.Discovered)
	}
}

func TestPhase1TrajectoriesAggregationOfflineIsolationAndFilter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Remote node M1
	m1Token := DeriveNodeToken("cluster-sec", "m1")
	m1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(HeaderHubToken) != m1Token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/gateway/status":
			writeJSON(w, http.StatusOK, proxy.GatewayStatus{
				Status:      "connected",
				GatewayName: "M1",
				Summary:     &proxy.GatewaySummary{RunningCount: 2, ActionCount: 1, UnreadCount: 3, TotalActive: 5},
			})
		case "/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories":
			writeJSON(w, http.StatusOK, map[string]any{
				"trajectorySummaries": map[string]any{
					"cid-m1-repo": map[string]any{"summary": "M1 Repo Task", "workspaceName": "repo-b"},
					"cid-m1-chat": map[string]any{"summary": "M1 Chat", "workspaceName": ""},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer m1.Close()

	// Slow/sleeping dead node that blocks until test finishes — must NOT stall Hub!
	stopSlow := make(chan struct{})
	slowNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-stopSlow:
		case <-time.After(5 * time.Second):
		}
	}))
	defer func() {
		close(stopSlow)
		slowNode.Close()
	}()

	// Local handler wrapped with gzip when Accept-Encoding: gzip is present (verifies Hub strips Accept-Encoding for local capture!)
	localMux := http.NewServeMux()
	localMux.HandleFunc("/gateway/status", func(w http.ResponseWriter, r *http.Request) {
		writeMaybeGzip(w, r, proxy.GatewayStatus{
			Status:      "connected",
			GatewayName: "HubMac",
			Summary:     &proxy.GatewaySummary{RunningCount: 1, ActionCount: 2, UnreadCount: 0, TotalActive: 3},
		})
	})
	localMux.HandleFunc("/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", func(w http.ResponseWriter, r *http.Request) {
		writeMaybeGzip(w, r, map[string]any{
			"trajectorySummaries": map[string]any{
				"cid-local-1": map[string]any{"summary": "Local Repo Task", "workspaceName": "repo-a"},
				"cid-local-2": map[string]any{"summary": "Local Chat", "workspaceName": "Chat"},
			},
		})
	})

	h, err := NewHub(Options{
		HubID:            "hub-test",
		HubName:          "HubMac",
		DataDir:          t.TempDir(),
		Secret:           "cluster-sec",
		StaticNodes:      []string{"m1=" + m1.URL + "|" + m1Token, "slow=" + slowNode.URL},
		DisableDiscovery: true,
		ProbeTimeout:     250 * time.Millisecond,
		ListTimeout:      350 * time.Millisecond,
		Inspector:        &stubInspector{healthy: true, port: 44444},
	})
	if err != nil {
		t.Fatalf("NewHub: %v", err)
	}
	defer h.Stop()
	handler := h.WrapHandler(localMux)

	// 1. Aggregated GetAllCascadeTrajectories must complete fast despite slowNode hanging
	start := time.Now()
	req := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", strings.NewReader("{}"))
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if elapsed > 2*time.Second {
		t.Fatalf("slow node stalled Hub aggregation: took %v", elapsed)
	}

	var listResp struct {
		TrajectorySummaries map[string]map[string]any `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal trajectories: %v", err)
	}
	if len(listResp.TrajectorySummaries) != 4 {
		t.Fatalf("expected 4 merged trajectories, got %d: %+v", len(listResp.TrajectorySummaries), listResp.TrajectorySummaries)
	}
	if got := listResp.TrajectorySummaries["cid-local-1"]["workspaceName"]; got != "[HubMac] repo-a" {
		t.Fatalf("expected [HubMac] repo-a, got %v", got)
	}
	if got := listResp.TrajectorySummaries["cid-local-2"]["workspaceName"]; got != "[HubMac] Chat" {
		t.Fatalf("expected [HubMac] Chat, got %v", got)
	}
	if got := listResp.TrajectorySummaries["cid-m1-repo"]["workspaceName"]; got != "[m1] repo-b" {
		t.Fatalf("expected [m1] repo-b, got %v", got)
	}
	if got := listResp.TrajectorySummaries["cid-m1-chat"]["workspaceName"]; got != "[m1] Chat" {
		t.Fatalf("expected [m1] Chat, got %v", got)
	}

	// 2. Aggregated /gateway/status?summary=1 sums counters across Local + M1
	stReq := httptest.NewRequest(http.MethodGet, "/gateway/status?summary=1", nil)
	stReq.Header.Set("Accept-Encoding", "gzip")
	stRec := httptest.NewRecorder()
	handler.ServeHTTP(stRec, stReq)
	if stRec.Code != http.StatusOK {
		t.Fatalf("status HTTP %d", stRec.Code)
	}
	var st proxy.GatewayStatus
	if err := json.Unmarshal(stRec.Body.Bytes(), &st); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if st.Status != "connected" || st.Summary == nil || st.Summary.RunningCount != 3 || st.Summary.ActionCount != 3 || st.Summary.TotalActive != 8 {
		t.Fatalf("unexpected aggregated status: %+v summary=%+v", st, st.Summary)
	}

	// 3. Hub filter switches view to M1 only
	fReq := httptest.NewRequest(http.MethodPost, "/gateway/hub/filter", strings.NewReader(`{"node_id":"m1"}`))
	fRec := httptest.NewRecorder()
	handler.ServeHTTP(fRec, fReq)
	if fRec.Code != http.StatusOK {
		t.Fatalf("filter HTTP %d: %s", fRec.Code, fRec.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", strings.NewReader("{}"))
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	var listResp2 struct {
		TrajectorySummaries map[string]map[string]any `json:"trajectorySummaries"`
	}
	_ = json.Unmarshal(rec2.Body.Bytes(), &listResp2)
	if len(listResp2.TrajectorySummaries) != 2 {
		t.Fatalf("expected 2 M1-only trajectories after filter, got %d", len(listResp2.TrajectorySummaries))
	}
}

func TestPhase1CascadeHTTPRoutingAndWSBridge(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	m1Token := "node-tok-m1"
	var remoteGotForwarded bool

	m1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(HeaderHubToken) != m1Token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get(HeaderHubForwarded) == "1" {
			remoteGotForwarded = true
		}
		switch r.URL.Path {
		case "/api/exa.language_server_pb.LanguageServerService/GetCascadeTrajectory":
			writeJSON(w, http.StatusOK, map[string]any{"source": "remote-m1"})
		case "/gateway/cascade/stream":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			mt, msg, err := conn.ReadMessage()
			if err == nil {
				_ = conn.WriteMessage(mt, append([]byte("m1-echo:"), msg...))
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer m1.Close()

	localMux := http.NewServeMux()
	localMux.HandleFunc("/api/exa.language_server_pb.LanguageServerService/GetCascadeTrajectory", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"source": "local-hub"})
	})

	h, err := NewHub(Options{
		HubID:            "hub-ws",
		HubName:          "HubMac",
		DataDir:          t.TempDir(),
		StaticNodes:      []string{"m1=" + m1.URL + "|" + m1Token},
		DisableDiscovery: true,
		Inspector:        &stubInspector{healthy: true, port: 44444},
	})
	if err != nil {
		t.Fatalf("NewHub: %v", err)
	}
	defer h.Stop()
	h.indexCascade("cid-remote-1", "m1")
	h.indexCascade("cid-local-1", LocalNodeID)

	hubSrv := httptest.NewServer(h.WrapHandler(localMux))
	defer hubSrv.Close()

	// 1. Remote cascade routes to M1
	resp, err := http.Post(hubSrv.URL+"/api/exa.language_server_pb.LanguageServerService/GetCascadeTrajectory", "application/json", strings.NewReader(`{"cascadeId":"cid-remote-1"}`))
	if err != nil {
		t.Fatalf("POST remote cascade: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "remote-m1") || !remoteGotForwarded {
		t.Fatalf("expected remote-m1 with X-Mgy-Hub-Forwarded, got body=%s forwarded=%v", body, remoteGotForwarded)
	}

	// 2. Loop prevention: X-Mgy-Hub-Forwarded: 1 stays strictly local even for a remote cascadeId
	loopReq, _ := http.NewRequest(http.MethodPost, hubSrv.URL+"/api/exa.language_server_pb.LanguageServerService/GetCascadeTrajectory", strings.NewReader(`{"cascadeId":"cid-remote-1"}`))
	loopReq.Header.Set(HeaderHubForwarded, "1")
	loopResp, err := http.DefaultClient.Do(loopReq)
	if err != nil {
		t.Fatalf("loop guard req: %v", err)
	}
	loopBody, _ := io.ReadAll(loopResp.Body)
	loopResp.Body.Close()
	if !strings.Contains(string(loopBody), "local-hub") {
		t.Fatalf("expected loop guard to route locally, got %s", loopBody)
	}

	// 3. Bidirectional WebSocket bridge to M1
	wsURL := "ws" + strings.TrimPrefix(hubSrv.URL, "http") + "/gateway/cascade/stream?cascadeId=cid-remote-1"
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WS dial through Hub failed: %v", err)
	}
	defer wsConn.Close()
	if err := wsConn.WriteMessage(websocket.TextMessage, []byte("hello-hub")); err != nil {
		t.Fatalf("WS write failed: %v", err)
	}
	_ = wsConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, reply, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("WS read failed: %v", err)
	}
	if string(reply) != "m1-echo:hello-hub" {
		t.Fatalf("unexpected WS bridged reply: %q", string(reply))
	}
}

func TestPhase1EventsFanInAndPushRelay(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Mock remote node with /gateway/events WS that emits a "changed" frame on trigger.
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	remoteChangeCh := make(chan struct{}, 4)
	remoteEventsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gateway/status":
			writeJSON(w, http.StatusOK, proxy.GatewayStatus{Status: "connected", GatewayName: "M1"})
		case "/gateway/events":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			_ = conn.WriteJSON(hubEventsFrame{Type: "hello", Rev: 1, Desktop: "ok"})
			for range remoteChangeCh {
				_ = conn.WriteJSON(hubEventsFrame{Type: "changed", Rev: 2})
			}
		}
	}))
	defer remoteEventsSrv.Close()

	// Setup Hub notifier with captureSender
	sender := newCaptureSender()
	hubNotif := notifier.NewNotifierWithSender(config.NotificationConfig{Enabled: true}, sender)
	hubNotif.SetGatewayIdentity("hub-gw-id", "MyHub")

	h, err := NewHub(Options{
		HubID:            "hub-gw-id",
		HubName:          "MyHub",
		DataDir:          t.TempDir(),
		Secret:           "shared-secret-xyz",
		DisableDiscovery: true,
		Notifier:         hubNotif,
	})
	if err != nil {
		t.Fatalf("NewHub: %v", err)
	}
	defer h.Stop()

	hubSrv := httptest.NewServer(h.WrapHandler(http.NewServeMux()))
	defer hubSrv.Close()

	// 1. Reject unauthorized registration
	badReg, _ := http.Post(hubSrv.URL+"/api/v1/hub/register", "application/json", strings.NewReader(`{"name":"Evil","port":58900,"secret":"wrong"}`))
	if badReg.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad secret, got %d", badReg.StatusCode)
	}
	badReg.Body.Close()

	// 2. Reject unauthorized internal event
	badEv, _ := http.Post(hubSrv.URL+"/api/v1/hub/internal/event", "application/json", strings.NewReader(`{"type":"list_changed"}`))
	if badEv.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthorized internal event, got %d", badEv.StatusCode)
	}
	badEv.Body.Close()

	// 3. NodeAgent registers cleanly with Hub using remoteEventsSrv as its advertised URL
	nodeNotif := notifier.NewNotifier(config.NotificationConfig{})
	nodeAgent, err := NewNodeAgent(NodeAgentOptions{
		DataDir:          t.TempDir(),
		NodeName:         "M1-Studio",
		AdvertiseBaseURL: remoteEventsSrv.URL,
		HubURL:           hubSrv.URL,
		HubSecret:        "shared-secret-xyz",
		DisableDiscovery: true,
		Notifier:         nodeNotif,
	})
	if err != nil {
		t.Fatalf("NewNodeAgent: %v", err)
	}
	if err := nodeAgent.RegisterWithHub(context.Background(), hubSrv.URL); err != nil {
		t.Fatalf("RegisterWithHub failed: %v", err)
	}

	// Connect client to Hub /gateway/events
	wsURL := "ws" + strings.TrimPrefix(hubSrv.URL, "http") + "/gateway/events"
	wsConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Hub /gateway/events dial failed: %v", err)
	}
	defer wsConn.Close()

	var hello hubEventsFrame
	_ = wsConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := wsConn.ReadJSON(&hello); err != nil || hello.Type != "hello" {
		t.Fatalf("expected hello frame, got %+v err=%v", hello, err)
	}

	// Ensure remote event watcher is attached, then fire remote change
	time.Sleep(150 * time.Millisecond)
	remoteChangeCh <- struct{}{}

	var changed hubEventsFrame
	_ = wsConn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err := wsConn.ReadJSON(&changed); err != nil || changed.Type != "changed" || changed.Rev <= hello.Rev {
		t.Fatalf("expected changed frame with rev > %d, got %+v err=%v", hello.Rev, changed, err)
	}

	// 4. Node's Notifier triggers NotifyCompleted -> relayed to Hub -> Hub's captureSender receives it!
	if err := nodeNotif.NotifyCompleted("cid-m1-done", "Build Backend", 12); err != nil {
		t.Fatalf("node NotifyCompleted failed: %v", err)
	}
	select {
	case p := <-sender.ch:
		if p.CascadeID != "cid-m1-done" {
			t.Fatalf("unexpected cascade_id: %q", p.CascadeID)
		}
		if p.GatewayID != "hub-gw-id" || !strings.Contains(p.URL, "gw=hub-gw-id") {
			t.Fatalf("expected Hub gateway ID in relayed push, got gw=%q url=%q", p.GatewayID, p.URL)
		}
		if !strings.Contains(p.Body, "[M1-Studio]") {
			t.Fatalf("expected [M1-Studio] prefix in relayed push body, got %q", p.Body)
		}
		if p.Localize != nil {
			_, enBody := p.Localize("en")
			if !strings.Contains(enBody, "[M1-Studio]") {
				t.Fatalf("expected [M1-Studio] prefix in localized body, got %q", enBody)
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for relayed push notification on Hub")
	}

	// Verify cascadeId from push was indexed on Hub -> routes to M1!
	if nid, ok := h.lookupCascadeNode("cid-m1-done"); !ok || nid != nodeAgent.NodeID() {
		t.Fatalf("expected cid-m1-done indexed to %s, got ok=%v nid=%q", nodeAgent.NodeID(), ok, nid)
	}
}

func TestPhase2ProjectsAndStartCascadeRouting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	const collidingPath = "/Users/dev/Projects/shared-app"
	const collidingURI = "file:///Users/dev/Projects/shared-app"

	var remoteStartReq map[string]any
	var remoteSentMessage bool

	m1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gateway/projects":
			writeJSON(w, http.StatusOK, []proxy.ProjectItem{
				{ID: "proj-same", Name: "shared-app", URI: collidingURI, Path: collidingPath, IsWorkspace: true, SessionCount: 5},
			})
		case "/gateway/cascade/new":
			_ = json.NewDecoder(r.Body).Decode(&remoteStartReq)
			writeJSON(w, http.StatusOK, proxy.CreateCascadeResponse{CascadeID: "cid-new-m1", Status: "ok"})
		case "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage":
			remoteSentMessage = true
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer m1.Close()

	localMux := http.NewServeMux()
	localMux.HandleFunc("/gateway/projects", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, []proxy.ProjectItem{
			{ID: "proj-same", Name: "shared-app", URI: collidingURI, Path: collidingPath, IsWorkspace: true, SessionCount: 3},
		})
	})

	h, err := NewHub(Options{
		HubID:            "hub-p2",
		HubName:          "HubMac",
		DataDir:          t.TempDir(),
		StaticNodes:      []string{"m1=" + m1.URL},
		DisableDiscovery: true,
		Inspector:        &stubInspector{healthy: true, port: 44444},
	})
	if err != nil {
		t.Fatalf("NewHub: %v", err)
	}
	defer h.Stop()
	handler := h.WrapHandler(localMux)

	// 1. GET /gateway/projects merges both colliding workspaces cleanly with distinct node hints
	pRec := httptest.NewRecorder()
	handler.ServeHTTP(pRec, httptest.NewRequest(http.MethodGet, "/gateway/projects", nil))
	if pRec.Code != http.StatusOK {
		t.Fatalf("projects HTTP %d", pRec.Code)
	}
	var projects []proxy.ProjectItem
	if err := json.Unmarshal(pRec.Body.Bytes(), &projects); err != nil {
		t.Fatalf("unmarshal projects: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 merged projects, got %d", len(projects))
	}
	var m1Proj proxy.ProjectItem
	for _, prj := range projects {
		if strings.HasPrefix(prj.Name, "[m1] ") {
			m1Proj = prj
		}
	}
	if m1Proj.URI == "" || !strings.Contains(m1Proj.URI, "mgy_node=m1") || m1Proj.ID != "mgy-node:m1:proj-same" {
		t.Fatalf("expected node-tagged M1 project, got %+v", m1Proj)
	}

	// 2. POST /gateway/cascade/new with M1's project URI/ID routes to M1 and strips Hub hints
	createPayload, _ := json.Marshal(map[string]any{
		"workspaceUri": m1Proj.URI,
		"projectId":    m1Proj.ID,
		"prompt":       "hello remote workspace",
	})
	cRec := httptest.NewRecorder()
	handler.ServeHTTP(cRec, httptest.NewRequest(http.MethodPost, "/gateway/cascade/new", bytes.NewReader(createPayload)))
	if cRec.Code != http.StatusOK {
		t.Fatalf("cascade/new HTTP %d: %s", cRec.Code, cRec.Body.String())
	}
	if remoteStartReq["workspaceUri"] != collidingURI {
		t.Fatalf("expected stripped workspaceUri %q forwarded to M1, got %v", collidingURI, remoteStartReq["workspaceUri"])
	}
	if remoteStartReq["projectId"] != "proj-same" {
		t.Fatalf("expected stripped projectId proj-same forwarded to M1, got %v", remoteStartReq["projectId"])
	}

	// 3. Subsequent SendUserCascadeMessage for cid-new-m1 routes straight to M1!
	msgRec := httptest.NewRecorder()
	handler.ServeHTTP(msgRec, httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage",
		strings.NewReader(`{"cascadeId":"cid-new-m1","text":"follow up"}`)))
	if msgRec.Code != http.StatusOK || !remoteSentMessage {
		t.Fatalf("expected follow-up message routed to M1, code=%d sent=%v", msgRec.Code, remoteSentMessage)
	}
}

func TestPhase2AttachmentLazyReplication(t *testing.T) {
	hubInbox := filepath.Join(t.TempDir(), "hub-inbox")
	t.Setenv("MGY_INBOX_DIR", hubInbox)
	t.Setenv("HOME", t.TempDir())

	// Remote node M1 receives replicated attachment upload and SendUserCascadeMessage
	var remoteUploadedBytes []byte
	var remoteUploadedName string
	var remoteMessageBody map[string]any

	m1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/attachments":
			rawName := r.Header.Get("X-File-Name")
			if unescaped, err := url.PathUnescape(rawName); err == nil {
				rawName = unescaped
			}
			remoteUploadedName = rawName
			remoteUploadedBytes, _ = io.ReadAll(r.Body)
			// Mirror real gateway content-addressed path on remote node
			writeJSON(w, http.StatusOK, proxy.Attachment{
				ID:   "replicated-id",
				Name: remoteUploadedName,
				Path: "/remote/m1/Inbox/2026-11/replicated-" + remoteUploadedName,
				Size: int64(len(remoteUploadedBytes)),
			})
		case "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage":
			_ = json.NewDecoder(r.Body).Decode(&remoteMessageBody)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer m1.Close()

	// Real proxy.HandleAttachmentUpload for Hub's local inbox!
	localProxy := proxy.NewProxy(&stubInspector{healthy: true, port: 44444})
	localMux := http.NewServeMux()
	localMux.HandleFunc("POST /api/v1/attachments", localProxy.HandleAttachmentUpload)

	h, err := NewHub(Options{
		HubID:            "hub-att",
		HubName:          "HubMac",
		DataDir:          t.TempDir(),
		StaticNodes:      []string{"m1=" + m1.URL},
		DisableDiscovery: true,
		Inspector:        &stubInspector{healthy: true, port: 44444},
	})
	if err != nil {
		t.Fatalf("NewHub: %v", err)
	}
	defer h.Stop()
	h.indexCascade("cid-remote-att", "m1")
	handler := h.WrapHandler(localMux)

	// 1. Mobile uploads file (with CJK filename) to Hub before targeting M1
	fileContent := []byte("hello cross-node attachment content!")
	upReq := httptest.NewRequest(http.MethodPost, "/api/v1/attachments", bytes.NewReader(fileContent))
	upReq.Header.Set("X-File-Name", url.PathEscape("研究报告.txt"))
	upReq.Header.Set("Content-Type", "text/plain")
	upRec := httptest.NewRecorder()
	handler.ServeHTTP(upRec, upReq)
	if upRec.Code != http.StatusOK {
		t.Fatalf("hub attachment upload HTTP %d: %s", upRec.Code, upRec.Body.String())
	}
	var hubAtt proxy.Attachment
	if err := json.Unmarshal(upRec.Body.Bytes(), &hubAtt); err != nil || hubAtt.ID == "" {
		t.Fatalf("invalid hub attachment response: %s", upRec.Body.String())
	}

	// 2. Mobile sends message referencing hubAtt.ID (and embedded Hub path) to remote M1 cascade
	msgPayload, _ := json.Marshal(map[string]any{
		"cascadeId":   "cid-remote-att",
		"attachments": []string{hubAtt.ID},
		"text":        "please read " + hubAtt.Path,
	})
	msgRec := httptest.NewRecorder()
	handler.ServeHTTP(msgRec, httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", bytes.NewReader(msgPayload)))
	if msgRec.Code != http.StatusOK {
		t.Fatalf("send message HTTP %d: %s", msgRec.Code, msgRec.Body.String())
	}

	if string(remoteUploadedBytes) != string(fileContent) || remoteUploadedName != "研究报告.txt" {
		t.Fatalf("remote node did not receive replicated attachment: name=%q bytes=%q", remoteUploadedName, string(remoteUploadedBytes))
	}
	if txt, _ := remoteMessageBody["text"].(string); !strings.Contains(txt, "/remote/m1/Inbox/2026-11/replicated-研究报告.txt") {
		t.Fatalf("expected Hub path rewritten to remote node path, got %q", txt)
	}
}

func TestPhase2CrossNodeFilesSearchTasksAndPermissions(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var remotePermPreset string
	m1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/files/content":
			uri := r.URL.Query().Get("uri")
			if strings.Contains(uri, "only-on-m1.md") || strings.Contains(uri, "aaaaaaaa-1111-2222-3333-444444444444") {
				writeJSON(w, http.StatusOK, map[string]any{"uri": uri, "filename": "plan.md", "content": "from-m1"})
				return
			}
			w.WriteHeader(http.StatusNotFound)
		case "/api/exa.language_server_pb.LanguageServerService/SearchConversations":
			writeJSON(w, http.StatusOK, proxy.ConversationSearchResponse{
				Results: []proxy.ConversationSearchResult{
					{CascadeID: "cid-search-m1", Title: "M1 Hit", WorkspaceName: "backend", LastModifiedTime: "2026-11-26T10:00:00Z"},
				},
			})
		case "/gateway/tasks/background":
			writeJSON(w, http.StatusOK, map[string]any{
				"tasks": []proxy.GlobalBackgroundTaskItem{
					{ConversationID: "cid-bg-m1", TaskID: "task-m1-9", WorkspaceName: "backend", LogURI: "file:///tmp/m1.log", StartTime: "2026-11-26T10:05:00Z"},
				},
			})
		case "/gateway/tasks/background/log":
			writeJSON(w, http.StatusOK, map[string]any{"taskId": "task-m1-9", "content": "remote-log-tail"})
		case "/gateway/permissions":
			var req map[string]string
			_ = json.NewDecoder(r.Body).Decode(&req)
			remotePermPreset = req["mode"]
			writeJSON(w, http.StatusOK, map[string]any{"mode": remotePermPreset})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer m1.Close()

	var localPermPreset string
	localMux := http.NewServeMux()
	localMux.HandleFunc("/api/v1/files/content", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	localMux.HandleFunc("/api/exa.language_server_pb.LanguageServerService/SearchConversations", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, proxy.ConversationSearchResponse{
			Results: []proxy.ConversationSearchResult{
				{CascadeID: "cid-search-local", Title: "Local Hit", WorkspaceName: "frontend", LastModifiedTime: "2026-11-26T09:00:00Z"},
			},
		})
	})
	localMux.HandleFunc("/gateway/tasks/background", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"tasks": []proxy.GlobalBackgroundTaskItem{}})
	})
	localMux.HandleFunc("/gateway/permissions", func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		_ = json.NewDecoder(r.Body).Decode(&req)
		localPermPreset = req["mode"]
		writeJSON(w, http.StatusOK, map[string]any{"mode": localPermPreset})
	})

	h, err := NewHub(Options{
		HubID:            "hub-p2-ops",
		HubName:          "HubMac",
		DataDir:          t.TempDir(),
		StaticNodes:      []string{"m1=" + m1.URL},
		DisableDiscovery: true,
		Inspector:        &stubInspector{healthy: true, port: 44444},
	})
	if err != nil {
		t.Fatalf("NewHub: %v", err)
	}
	defer h.Stop()
	h.indexCascade("aaaaaaaa-1111-2222-3333-444444444444", "m1")
	handler := h.WrapHandler(localMux)

	// 1. File reading via brain/<cascadeId>
	f1 := httptest.NewRecorder()
	handler.ServeHTTP(f1, httptest.NewRequest(http.MethodGet, "/api/v1/files/content?uri="+url.QueryEscape("file:///Users/m1/.gemini/antigravity/brain/aaaaaaaa-1111-2222-3333-444444444444/plan.md"), nil))
	if f1.Code != http.StatusOK || !strings.Contains(f1.Body.String(), "from-m1") {
		t.Fatalf("expected file routed via brain/<cid>, code=%d body=%s", f1.Code, f1.Body.String())
	}

	// 2. File reading via parallel fallback probe when missing locally
	f2 := httptest.NewRecorder()
	handler.ServeHTTP(f2, httptest.NewRequest(http.MethodGet, "/api/v1/files/content?uri="+url.QueryEscape("file:///Users/m1/only-on-m1.md"), nil))
	if f2.Code != http.StatusOK || !strings.Contains(f2.Body.String(), "from-m1") {
		t.Fatalf("expected fallback probe hit on M1, code=%d body=%s", f2.Code, f2.Body.String())
	}

	// 3. Cross-node SearchConversations merges & sorts
	sRec := httptest.NewRecorder()
	handler.ServeHTTP(sRec, httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SearchConversations", strings.NewReader(`{"query":"hit","limit":10}`)))
	var searchResp proxy.ConversationSearchResponse
	if err := json.Unmarshal(sRec.Body.Bytes(), &searchResp); err != nil || len(searchResp.Results) != 2 {
		t.Fatalf("expected 2 merged search hits, got %+v", searchResp.Results)
	}
	if searchResp.Results[0].CascadeID != "cid-search-m1" || searchResp.Results[0].WorkspaceName != "[m1] backend" {
		t.Fatalf("unexpected top search result: %+v", searchResp.Results[0])
	}

	// 4. Cross-node background tasks aggregation + log routing
	tRec := httptest.NewRecorder()
	handler.ServeHTTP(tRec, httptest.NewRequest(http.MethodGet, "/gateway/tasks/background", nil))
	if tRec.Code != http.StatusOK || !strings.Contains(tRec.Body.String(), "[m1] backend") {
		t.Fatalf("unexpected background tasks response: %d %s", tRec.Code, tRec.Body.String())
	}
	logRec := httptest.NewRecorder()
	handler.ServeHTTP(logRec, httptest.NewRequest(http.MethodPost, "/gateway/tasks/background/log", strings.NewReader(`{"taskId":"task-m1-9"}`)))
	if logRec.Code != http.StatusOK || !strings.Contains(logRec.Body.String(), "remote-log-tail") {
		t.Fatalf("expected background task log routed to M1, code=%d body=%s", logRec.Code, logRec.Body.String())
	}

	// 5. Permission preset broadcast to both Local and M1
	permRec := httptest.NewRecorder()
	handler.ServeHTTP(permRec, httptest.NewRequest(http.MethodPost, "/gateway/permissions", strings.NewReader(`{"mode":"turbo"}`)))
	if permRec.Code != http.StatusOK || localPermPreset != "turbo" || remotePermPreset != "turbo" {
		t.Fatalf("expected permissions synced to both local and M1, code=%d local=%q remote=%q", permRec.Code, localPermPreset, remotePermPreset)
	}
}

func TestHeadlessHubMode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gateway/status":
			writeJSON(w, http.StatusOK, proxy.GatewayStatus{
				Status:      "connected",
				GatewayName: "M1",
				Summary:     &proxy.GatewaySummary{RunningCount: 1, TotalActive: 2},
			})
		case "/api/exa.language_server_pb.LanguageServerService/StartCascade":
			writeJSON(w, http.StatusOK, map[string]any{"cascadeId": "cid-headless-chat"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer m1.Close()

	h, err := NewHub(Options{
		HubID:            "hub-nas",
		HubName:          "NAS-Hub",
		Headless:         true,
		DataDir:          t.TempDir(),
		StaticNodes:      []string{"m1=" + m1.URL},
		DisableDiscovery: true,
		Inspector:        &stubInspector{healthy: false},
	})
	if err != nil {
		t.Fatalf("NewHub: %v", err)
	}
	defer h.Stop()
	h.ProbeAllNodes(context.Background())

	localMux := http.NewServeMux()
	localMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	handler := h.WrapHandler(localMux)

	// /readyz returns 200 because M1 is healthy even though Hub has no local LS
	rRec := httptest.NewRecorder()
	handler.ServeHTTP(rRec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rRec.Code != http.StatusOK {
		t.Fatalf("expected headless Hub /readyz 200, got %d: %s", rRec.Code, rRec.Body.String())
	}

	// /gateway/status reports connected
	sRec := httptest.NewRecorder()
	handler.ServeHTTP(sRec, httptest.NewRequest(http.MethodGet, "/gateway/status?summary=1", nil))
	var st proxy.GatewayStatus
	_ = json.Unmarshal(sRec.Body.Bytes(), &st)
	if st.Status != "connected" || st.Summary == nil || st.Summary.RunningCount != 1 {
		t.Fatalf("expected connected headless status, got %+v summary=%+v", st, st.Summary)
	}

	// Pure chat StartCascade on headless Hub routes to M1 and indexes the new cascadeId
	cRec := httptest.NewRecorder()
	handler.ServeHTTP(cRec, httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/StartCascade", strings.NewReader(`{}`)))
	if cRec.Code != http.StatusOK || !strings.Contains(cRec.Body.String(), "cid-headless-chat") {
		t.Fatalf("expected headless StartCascade routed to M1, code=%d body=%s", cRec.Code, cRec.Body.String())
	}
	if nid, ok := h.lookupCascadeNode("cid-headless-chat"); !ok || nid != "m1" {
		t.Fatalf("expected cid-headless-chat indexed to m1, got ok=%v nid=%q", ok, nid)
	}
}

func writeMaybeGzip(w http.ResponseWriter, r *http.Request, v any) {
	data, _ := json.Marshal(v)
	w.Header().Set("Content-Type", "application/json")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = gz.Write(data)
		_ = gz.Close()
		return
	}
	_, _ = w.Write(data)
}

func TestHub_AIAssistPrefsBroadcastAndWSAcceptLanguage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var mu sync.Mutex
	var gotPrefsBody, gotPrefsAL, gotWSAL string
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	remoteNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gateway/status":
			writeJSON(w, http.StatusOK, proxy.GatewayStatus{Status: "connected", GatewayName: "Remote1"})
		case "/gateway/ai-assist/prefs":
			b, _ := io.ReadAll(r.Body)
			mu.Lock()
			gotPrefsBody = string(b)
			gotPrefsAL = r.Header.Get("Accept-Language")
			mu.Unlock()
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "resolvedLang": "fr-FR"})
		case "/gateway/stream":
			mu.Lock()
			gotWSAL = r.Header.Get("Accept-Language")
			mu.Unlock()
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"noop"}`))
			time.Sleep(50 * time.Millisecond)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer remoteNode.Close()

	h, err := NewHub(Options{
		HubID:            "hub-locale-test",
		HubName:          "HubLocale",
		DataDir:          t.TempDir(),
		StaticNodes:      []string{"n1=" + remoteNode.URL},
		DisableDiscovery: true,
		Inspector:        &stubInspector{healthy: true, port: 59001},
	})
	if err != nil {
		t.Fatalf("NewHub: %v", err)
	}
	defer h.Stop()
	h.ProbeAllNodes(context.Background())
	h.indexCascade("cid-ws-fr", "n1")

	localMux := http.NewServeMux()
	localMux.HandleFunc("/gateway/ai-assist/prefs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "local": true})
	})
	handler := h.WrapHandler(localMux)

	// 1. POST /gateway/ai-assist/prefs broadcasts to enabled remote nodes preserving Accept-Language
	req := httptest.NewRequest(http.MethodPost, "/gateway/ai-assist/prefs", strings.NewReader(`{"titleTranslateEnabled":true}`))
	req.Header.Set("Accept-Language", "fr-FR,fr;q=0.9")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /gateway/ai-assist/prefs status=%d body=%s", rec.Code, rec.Body.String())
	}
	mu.Lock()
	pb, pal := gotPrefsBody, gotPrefsAL
	mu.Unlock()
	if !strings.Contains(pb, `"titleTranslateEnabled":true`) || pal != "fr-FR,fr;q=0.9" {
		t.Fatalf("remote node did not receive AI Assist broadcast with Accept-Language: body=%q al=%q", pb, pal)
	}

	// 2. WebSocket bridge forwards Accept-Language to remote node
	hubSrv := httptest.NewServer(handler)
	defer hubSrv.Close()
	wsURL := "ws" + strings.TrimPrefix(hubSrv.URL, "http") + "/gateway/stream?cascadeId=cid-ws-fr"
	hdr := http.Header{"Accept-Language": []string{"fr-FR,fr;q=0.9"}}
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("dial hub ws: %v", err)
	}
	_, _, _ = conn.ReadMessage()
	_ = conn.Close()
	mu.Lock()
	wsal := gotWSAL
	mu.Unlock()
	if wsal != "fr-FR,fr;q=0.9" {
		t.Fatalf("WS bridge did not forward Accept-Language: got %q", wsal)
	}
}
