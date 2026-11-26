package proxy

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func newPhase4MockProxy(t *testing.T, handler http.HandlerFunc) (*Proxy, *httptest.Server) {
	t.Helper()
	ts := httptest.NewTLSServer(handler)
	t.Cleanup(ts.Close)
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	p := &Proxy{
		shortClient:  ts.Client(),
		mediumClient: ts.Client(),
		longClient:   ts.Client(),
		streamClient: ts.Client(),
	}
	p.SetTestUpstream(port, "test-csrf")
	return p, ts
}

func TestPhase4BrowserViewerEndpoints(t *testing.T) {
	var mu sync.Mutex
	var gotPaths []string
	var lastControlBody map[string]interface{}

	p, _ := newPhase4MockProxy(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPaths = append(gotPaths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/exa.browser_viewer_pb.BrowserViewerService/ListBrowserTabs":
			_, _ = w.Write([]byte(`{"hasBrowserHost":true,"tabs":[{"tabId":"tab-1","title":"Login","url":"https://example.com","widthPx":1280,"heightPx":800,"agentOwned":true}]}`))
		case "/exa.browser_viewer_pb.BrowserViewerService/OpenBrowserView":
			_, _ = w.Write([]byte(`{"sessionId":"sess-1","expiresUnixMs":1999999999000,"tab":{"tabId":"tab-1","url":"https://example.com"}}`))
		case "/exa.browser_viewer_pb.BrowserViewerService/ReadBrowserFrame":
			_, _ = w.Write([]byte(`{"imageData":"iVBORw0KGgo=","mimeType":"image/png","widthPx":960,"heightPx":600}`))
		case "/exa.browser_viewer_pb.BrowserViewerService/TakeBrowserControl":
			_, _ = w.Write([]byte(`{"controlToken":"tok-abc","controlExpiresUnixMs":1999999999000}`))
		case "/exa.browser_viewer_pb.BrowserViewerService/ControlBrowserTab":
			mu.Lock()
			_ = json.NewDecoder(r.Body).Decode(&lastControlBody)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"tab":{"tabId":"tab-1","url":"https://example.com/ok"}}`))
		case "/exa.browser_viewer_pb.BrowserViewerService/ReleaseBrowserControl":
			_, _ = w.Write([]byte(`{}`))
		case "/exa.browser_viewer_pb.BrowserViewerService/CloseBrowserView":
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	})

	// 1. List tabs
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/gateway/browser/tabs?cascadeId=cid-1", nil)
	p.HandleBrowserViewer(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"tab-1"`) {
		t.Fatalf("tabs failed: %d %s", rec.Code, rec.Body.String())
	}

	// 2. Open + Frame + Takeover
	for _, tc := range []struct {
		path string
		body string
		want string
	}{
		{"/gateway/browser/open", `{"cascadeId":"cid-1","tabId":"tab-1"}`, `"sess-1"`},
		{"/gateway/browser/frame", `{"cascadeId":"cid-1","sessionId":"sess-1","tabId":"tab-1"}`, `"image/png"`},
		{"/gateway/browser/takeover", `{"cascadeId":"cid-1","sessionId":"sess-1","tabId":"tab-1"}`, `"tok-abc"`},
	} {
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		p.HandleBrowserViewer(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s failed: %d %s", tc.path, rec.Code, rec.Body.String())
		}
	}

	// 3. Batched tap input events
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/gateway/browser/control", strings.NewReader(`{
		"cascadeId":"cid-1","sessionId":"sess-1","tabId":"tab-1","controlToken":"tok-abc",
		"inputEvents":[{"kind":"pointer_down","x":100,"y":200,"buttons":1},{"kind":"pointer_up","x":100,"y":200,"buttons":0}]
	}`))
	p.HandleBrowserViewer(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("control failed: %d %s", rec.Code, rec.Body.String())
	}
	mu.Lock()
	ev, _ := lastControlBody["inputEvent"].(map[string]interface{})
	mu.Unlock()
	if ev["kind"] != "KIND_POINTER_UP" {
		t.Fatalf("expected normalized KIND_POINTER_UP, got %#v", ev)
	}
}

func TestPhase4RemotesConnectAndPrompt(t *testing.T) {
	promptAnswered := make(chan string, 1)

	p, _ := newPhase4MockProxy(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/exa.remoting.RemotingService/ListRemotes":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"remotes":[{"name":"dev-linux","description":"Ubuntu dev"}]}`))
		case "/exa.remoting.RemotingService/ListConnectedRemotes":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"remotes":[]}`))
		case "/exa.language_server_pb.LanguageServerService/ListEnvironments":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"environments":[{"id":"env-1","name":"default"}]}`))
		case "/exa.remoting.RemotingService/ConnectRemote":
			w.Header().Set("Content-Type", "application/connect+json")
			writeEnv := func(flag byte, payload string) {
				var hdr [5]byte
				hdr[0] = flag
				binary.BigEndian.PutUint32(hdr[1:5], uint32(len(payload)))
				_, _ = w.Write(hdr[:])
				_, _ = w.Write([]byte(payload))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
			writeEnv(0, `{"remoteId":"rem-42","prompt":{"message":"Enter passphrase:","isSecret":true}}`)
			select {
			case <-promptAnswered:
			case <-time.After(3 * time.Second):
			}
			writeEnv(0, `{"remoteId":"rem-42","status":"CONNECTED"}`)
			writeEnv(2, `{}`)
		case "/exa.remoting.RemotingService/RespondToPrompt":
			var req struct {
				RemoteID string `json:"remoteId"`
				Response string `json:"response"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			promptAnswered <- req.RemoteID + ":" + req.Response
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	})
	defer p.cancelAllRemoteJobs()

	// List remotes
	rec := httptest.NewRecorder()
	p.HandleRemotes(rec, httptest.NewRequest(http.MethodGet, "/gateway/remotes", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"dev-linux"`) {
		t.Fatalf("list remotes failed: %d %s", rec.Code, rec.Body.String())
	}

	// Start connect
	rec = httptest.NewRecorder()
	p.HandleRemotes(rec, httptest.NewRequest(http.MethodPost, "/gateway/remotes/connect", strings.NewReader(`{"name":"dev-linux","host":"10.0.0.5"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("connect failed: %d %s", rec.Code, rec.Body.String())
	}
	var startResp struct {
		Job RemoteConnectJobSnapshot `json:"job"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &startResp)

	// Wait for prompt
	deadline := time.Now().Add(2 * time.Second)
	var snap RemoteConnectJobSnapshot
	for time.Now().Before(deadline) {
		rec = httptest.NewRecorder()
		p.HandleRemotes(rec, httptest.NewRequest(http.MethodGet, "/gateway/remotes/connect/status?jobId="+startResp.Job.JobID, nil))
		var st struct {
			Job RemoteConnectJobSnapshot `json:"job"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &st)
		snap = st.Job
		if snap.Status == "prompt" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if snap.Status != "prompt" || snap.Prompt == nil || !snap.Prompt.IsSecret || snap.RemoteID != "rem-42" {
		t.Fatalf("expected secret prompt for rem-42, got %+v", snap)
	}

	// Answer prompt
	rec = httptest.NewRecorder()
	p.HandleRemotes(rec, httptest.NewRequest(http.MethodPost, "/gateway/remotes/prompt", strings.NewReader(`{"jobId":"`+startResp.Job.JobID+`","response":"s3cr3t"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("prompt answer failed: %d %s", rec.Code, rec.Body.String())
	}
	// Wait for connected
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec = httptest.NewRecorder()
		p.HandleRemotes(rec, httptest.NewRequest(http.MethodGet, "/gateway/remotes/connect/status?jobId="+startResp.Job.JobID, nil))
		var st struct {
			Job RemoteConnectJobSnapshot `json:"job"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &st)
		if st.Job.Status == "connected" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("remote connect job did not reach connected state")
}

func TestPhase4LoopbackOAuthCallbackForwardSecurity(t *testing.T) {
	// Spin up a local OAuth callback listener
	cbHit := false
	cbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/cb" && r.URL.Query().Get("code") == "xyz" {
			cbHit = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer cbServer.Close()

	p, _ := newPhase4MockProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Reject external host
	rec := httptest.NewRecorder()
	p.HandleMcpOAuth(rec, httptest.NewRequest(http.MethodPost, "/gateway/oauth/callback-forward", strings.NewReader(`{"callbackUrl":"http://evil.com:8080/cb"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for external host, got %d", rec.Code)
	}

	// Reject language_server port itself
	lsPort, _ := p.ActiveUpstream()
	rec = httptest.NewRecorder()
	p.HandleMcpOAuth(rec, httptest.NewRequest(http.MethodPost, "/gateway/oauth/callback-forward", strings.NewReader(`{"callbackUrl":"http://127.0.0.1:`+strconv.Itoa(lsPort)+`/cb"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for lsPort, got %d", rec.Code)
	}

	// Accept valid loopback OAuth listener
	rec = httptest.NewRecorder()
	body := `{"callbackUrl":"` + cbServer.URL + `/oauth/cb?code=xyz"}`
	req := httptest.NewRequest(http.MethodPost, "/gateway/oauth/callback-forward", strings.NewReader(body))
	req.Host = "127.0.0.1:58900"
	p.HandleMcpOAuth(rec, req)
	if rec.Code != http.StatusOK || !cbHit {
		t.Fatalf("expected valid loopback callback forward, code=%d hit=%v body=%s", rec.Code, cbHit, rec.Body.String())
	}
}

func TestPhase4MarketplaceUpdateSearchSnapshotsStar(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Prepare a mock workspace and changelog file inside tmpHome
	wsDir := filepath.Join(tmpHome, "demo-ws")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "hello.go"), []byte("package main\nfunc GreetingToken() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clPath := filepath.Join(tmpHome, "changelog.md")
	if err := os.WriteFile(clPath, []byte("# v2.35.2\n- Switch models while running\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var starredUpstream bool
	p, _ := newPhase4MockProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/exa.language_server_pb.LanguageServerService/GetAllPlugins":
			_, _ = w.Write([]byte(`{"plugins":[{"id":"p-installed","name":"Installed"}]}`))
		case "/exa.language_server_pb.LanguageServerService/SearchMarketplace":
			_, _ = w.Write([]byte(`{"customizations":[{"id":"p-new","displayName":"Linear","description":"Track issues","kind":"CUSTOMIZATION_KIND_PLUGIN"}]}`))
		case "/exa.language_server_pb.LanguageServerService/DeletePlugin":
			_, _ = w.Write([]byte(`{"success":true,"message":"deleted"}`))
		case "/exa.language_server_pb.LanguageServerService/StartPluginAuthSignIn":
			_, _ = w.Write([]byte(`{"authUrl":"https://auth.example.com/o?redirect_uri=http%3A%2F%2F127.0.0.1%3A9123%2Fcb","accountId":"acc-1"}`))
		case "/exa.language_server_pb.LanguageServerService/GetUpdateStatus":
			_, _ = w.Write([]byte(`{"status":{"currentVersion":"2.35.1","latestVersion":"2.35.2","updateAvailable":true}}`))
		case "/exa.language_server_pb.LanguageServerService/GetChangelog":
			raw, _ := json.Marshal(map[string]string{"path": clPath})
			_, _ = w.Write(raw)
		case "/exa.language_server_pb.LanguageServerService/ApplyUpdate":
			_, _ = w.Write([]byte(`{"notifiedHost":true}`))
		case "/exa.language_server_pb.LanguageServerService/SearchCode":
			_, _ = w.Write([]byte(`{"results":[]}`)) // force local fallback verification
		case "/exa.language_server_pb.LanguageServerService/SearchFiles":
			_, _ = w.Write([]byte(`{"results":[]}`)) // force local fallback verification
		case "/exa.language_server_pb.LanguageServerService/GetArtifactSnapshots":
			_, _ = w.Write([]byte(`{"artifactSnapshots":[{"artifactName":"plan.md","artifactAbsoluteUri":"file:///tmp/plan.md","content":"# v1","lastEdited":"2026-11-26T08:00:00Z"},{"artifactName":"plan.md","artifactAbsoluteUri":"file:///tmp/plan.md","content":"# v2","lastEdited":"2026-11-26T08:10:00Z"}]}`))
		case "/exa.language_server_pb.LanguageServerService/UpdateConversationAnnotations":
			starredUpstream = true
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	})

	// Marketplace search
	rec := httptest.NewRecorder()
	p.HandleMarketplace(rec, httptest.NewRequest(http.MethodGet, "/gateway/marketplace/search?q=linear", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"Linear"`) {
		t.Fatalf("marketplace search failed: %d %s", rec.Code, rec.Body.String())
	}

	// Plugin auth start detects loopback redirect_uri
	rec = httptest.NewRecorder()
	p.HandlePluginManageExtra(rec, httptest.NewRequest(http.MethodPost, "/gateway/plugins/auth/start", strings.NewReader(`{"plugin":"linear","provider":"oauth"}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"isLoopbackCallback":true`) {
		t.Fatalf("plugin auth start failed: %d %s", rec.Code, rec.Body.String())
	}

	// Desktop update + changelog + apply guard
	rec = httptest.NewRecorder()
	p.HandleDesktopUpdate(rec, httptest.NewRequest(http.MethodGet, "/gateway/desktop/update", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"updateAvailable":true`) {
		t.Fatalf("desktop update failed: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	p.HandleDesktopUpdate(rec, httptest.NewRequest(http.MethodGet, "/gateway/desktop/changelog?version=2.35.2", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Switch models while running") {
		t.Fatalf("changelog failed: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	p.HandleDesktopUpdate(rec, httptest.NewRequest(http.MethodPost, "/gateway/desktop/update/apply", strings.NewReader(`{"confirm":false}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when confirm=false, got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	p.HandleDesktopUpdate(rec, httptest.NewRequest(http.MethodPost, "/gateway/desktop/update/apply", strings.NewReader(`{"confirm":true}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"notifiedHost":true`) {
		t.Fatalf("apply update failed: %d %s", rec.Code, rec.Body.String())
	}

	// Code & File search with local fallback
	wsURI := filePathToURI(wsDir)
	rec = httptest.NewRecorder()
	bodyBytes, _ := json.Marshal(map[string]interface{}{"workspaceUri": wsURI, "query": "GreetingToken"})
	p.HandleWorkspaceSearch(rec, httptest.NewRequest(http.MethodPost, "/gateway/search/code", bytes.NewReader(bodyBytes)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "hello.go") {
		t.Fatalf("code search failed: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	bodyBytes, _ = json.Marshal(map[string]interface{}{"workspaceUri": wsURI, "query": "hello"})
	p.HandleWorkspaceSearch(rec, httptest.NewRequest(http.MethodPost, "/gateway/search/files", bytes.NewReader(bodyBytes)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "hello.go") {
		t.Fatalf("file search failed: %d %s", rec.Code, rec.Body.String())
	}

	// Artifact snapshots
	rec = httptest.NewRecorder()
	p.HandleArtifactSnapshots(rec, httptest.NewRequest(http.MethodGet, "/gateway/artifacts/snapshots?cascadeId=cid-snap&artifactName=plan.md", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "# v2") {
		t.Fatalf("artifact snapshots failed: %d %s", rec.Code, rec.Body.String())
	}

	// Star cascade persists local mirror and forwards upstream
	rec = httptest.NewRecorder()
	p.HandleStarCascade(rec, httptest.NewRequest(http.MethodPost, "/gateway/cascade/star", strings.NewReader(`{"cascadeId":"cid-star-1","starred":true}`)))
	if rec.Code != http.StatusOK || !starredUpstream {
		t.Fatalf("star cascade failed: %d upstream=%v body=%s", rec.Code, starredUpstream, rec.Body.String())
	}
	if starred, found := readAnnotationStarred("cid-star-1"); !found || !starred {
		t.Fatalf("expected starred=true in disk .pbtxt mirror, got found=%v starred=%v", found, starred)
	}
}
