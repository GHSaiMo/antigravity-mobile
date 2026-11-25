package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/inspector"
	"antigravity-mobile/internal/localtls"

	"github.com/gorilla/websocket"
)

// GatewaySummary provides lightweight cross-gateway conversation counters for multi-host status peek.
type GatewaySummary struct {
	RunningCount int `json:"running_count"`
	ActionCount  int `json:"action_count"`
	UnreadCount  int `json:"unread_count"`
	TotalActive  int `json:"total_active"`
}

// GatewayStatus represents the public status of the gateway.
type GatewayStatus struct {
	Status                string                  `json:"status"`
	OS                    string                  `json:"os,omitempty"`
	Platform              string                  `json:"platform,omitempty"`
	GatewayID             string                  `json:"gateway_id,omitempty"`
	GatewayName           string                  `json:"gateway_name,omitempty"`
	Summary               *GatewaySummary         `json:"summary,omitempty"`
	Upstream              *inspector.InstanceInfo `json:"upstream,omitempty"`
	ActiveStreamCascadeID string                  `json:"active_stream_cascade_id,omitempty"`
	ActiveStreamTitle     string                  `json:"active_stream_title,omitempty"`
	Timestamp             time.Time               `json:"timestamp"`
	UnifiedCursor         *UnifiedCursor          `json:"unified_cursor,omitempty"`
	// Compat tells the apps which features the running Antigravity still supports.
	Compat *Compatibility `json:"compat,omitempty"`
}

// NotificationSink receives real-time trajectory status updates.
type NotificationSink interface {
	OnTrajectoryUpdate(details *TrajectoryDetails)
}

type flightCall[T any] struct {
	done chan struct{}
	val  T
	err  error
}

// flightGroup is a zero-value-ready generic singleflight helper for coalescing concurrent cache misses.
type flightGroup[T any] struct {
	mu    sync.Mutex
	calls map[string]*flightCall[T]
}

func (g *flightGroup[T]) do(key string, fn func() (T, error)) (T, error) {
	g.mu.Lock()
	if g.calls == nil {
		g.calls = make(map[string]*flightCall[T])
	}
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		<-c.done
		return c.val, c.err
	}
	c := &flightCall[T]{done: make(chan struct{})}
	g.calls[key] = c
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		delete(g.calls, key)
		g.mu.Unlock()
		close(c.done)
	}()

	c.val, c.err = fn()
	return c.val, c.err
}

// Proxy routes and proxies HTTP/RPC requests to the Antigravity language_server.
type Proxy struct {
	insp      inspector.UpstreamDiscoverer
	transport *http.Transport
	startTime time.Time

	// Pre-built HTTP clients with different timeout tiers to avoid repeated construction (H-6)
	shortClient  *http.Client // 2-5s timeout, for quick status checks
	mediumClient *http.Client // 10s timeout, for standard RPC calls
	longClient   *http.Client // 60s timeout, for large data transfers
	streamClient *http.Client // 0 timeout, for long-lived ConnectRPC streaming subscriptions

	mu          sync.RWMutex
	activeProxy *httputil.ReverseProxy
	activePort  int
	activeToken string
	notifier    NotificationSink
	gatewayID   string
	gatewayName string

	slashCache slashCommandCache
	compat     compatState

	// allTrajectories shares one GetAllCascadeTrajectories response across Watcher, list polls and lookups.
	allTrajectories allTrajectoriesSnapshot

	// events fans conversation-list change notifications out to /gateway/events subscribers.
	events eventsHub

	// Reactive Antigravity 2.32+ Connect streaming hubs & WS cursor resume store.
	agentStateHub reactiveAgentStateHub
	summariesHub  reactiveSummariesHub
	streamResume  streamResumeStore

	activeStreamMu        sync.RWMutex
	activeStreamCascadeID string
	activeStreamTitle     string

	streamListenersMu sync.Mutex
	streamListeners   map[string][]chan struct{}

	msgDedupMu     sync.Mutex
	msgDedup       map[string]time.Time
	cascadeDedupMu sync.Mutex
	cascadeDedup   map[string]cascadeDedupEntry

	cursorMu                  sync.RWMutex
	mobileCascadeID           string
	mobileTitle               string
	mobileFocusedAt           time.Time
	desktopCascadeID          string
	desktopTitle              string
	desktopFocusedAt          time.Time
	suppressDesktopFocusUntil time.Time
	mobileStickyDuration      time.Duration

	projectsCacheMu      sync.RWMutex
	lastReadProjectsResp []byte
	cachedProjectsKey    string
	cachedProjectsList   []ProjectItem
	cachedProjectsAt     time.Time
	projectsFlights      flightGroup[[]ProjectItem]

	cloneJobsMu sync.Mutex
	cloneJobs   map[string]*cloneRepoJob

	// nuxCache caches GetCascadeNuxes responses for nuxCacheTTL to avoid
	// 300-900ms upstream latency on every desktop workbench initialization.
	nuxCacheMu      sync.RWMutex
	nuxCacheBody    []byte
	nuxCacheHeaders http.Header
	nuxCachedAt     time.Time

	// authStatusCache caches GetAuthStatus responses for authStatusCacheTTL to avoid
	// 3-6s upstream Google Auth latency on every desktop workbench initialization.
	authStatusCacheMu      sync.RWMutex
	authStatusCacheBody    []byte
	authStatusCacheHeaders http.Header
	authStatusCachedAt     time.Time

	// userStatusMu caches GetActiveUserStatus (email/name) for activeUserStatusTTL.
	userStatusMu    sync.RWMutex
	cachedUserEmail string
	cachedUserName  string
	cachedUserPort  int
	cachedUserAt    time.Time

	// Active WebSocket connection tracking for graceful shutdown
	wsConnsMu sync.Mutex
	wsConns   map[*websocket.Conn]struct{}
}

const activeUserStatusTTL = 30 * time.Second

// nuxCacheTTL is the duration to cache GetCascadeNuxes responses.
// NUX (New User Experience) data changes at most once per feature release; 60s is safe.
const nuxCacheTTL = 60 * time.Second

// authStatusCacheTTL is the duration to cache GetAuthStatus responses.
// Auth state rarely fluctuates within a session; 10 minutes eliminates recurring 3-6s lags.
const authStatusCacheTTL = 10 * time.Minute

// desktopAssetVersion provides cache-busting versioning for localized desktop assets.
var desktopAssetVersion = fmt.Sprintf("1.0.7-%d", time.Now().Unix())

type cascadeDedupEntry struct {
	cascadeID string
	createdAt time.Time
}

// cascadeIDRe is a strict allowlist for cascade IDs used in filesystem operations.
// Cascade IDs are UUID-like strings: alphanumeric, hyphens, and underscores only.
var (
	cascadeIDRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)
	verboseRPC  = os.Getenv("MULTIGRAVITY_VERBOSE") != "" || os.Getenv("MULTIGRAVITY_LOG_RPC") != "" || os.Getenv("MULTIGRAVITY_VERBOSE_RPC") != "" || os.Getenv("GATEWAY_VERBOSE_RPC") != "" || os.Getenv("GATEWAY_LOG_RPC") != ""
)

// shortCascadeID truncates a cascade ID for log redaction/sanitization.
func shortCascadeID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 12 {
		return id
	}
	return id[:8] + "..."
}

// SetNotificationSink registers a sink to receive real-time trajectory updates.
func (p *Proxy) SetNotificationSink(sink NotificationSink) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.notifier = sink
}

// NotificationSink returns the registered notification sink, if any.
func (p *Proxy) NotificationSink() NotificationSink {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.notifier
}

// ActiveUpstream returns the current active upstream port and CSRF token.
func (p *Proxy) ActiveUpstream() (int, string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.activePort, p.activeToken
}

// SetTestUpstream configures active port and token for testing.
func (p *Proxy) SetTestUpstream(port int, token string) {
	p.updateUpstream(inspector.InstanceInfo{
		Port:      port,
		CSRFToken: token,
		IsHealthy: true,
	})
}

// GetActiveUserStatus queries the running Antigravity instance for the currently logged-in user email and name.
func (p *Proxy) GetActiveUserStatus() (email string, name string, err error) {
	port, token := p.ActiveUpstream()
	if port == 0 {
		return "", "", fmt.Errorf("no active Antigravity upstream")
	}

	p.userStatusMu.RLock()
	if p.cachedUserPort == port && p.cachedUserEmail != "" && time.Since(p.cachedUserAt) < activeUserStatusTTL {
		email, name = p.cachedUserEmail, p.cachedUserName
		p.userStatusMu.RUnlock()
		return email, name, nil
	}
	p.userStatusMu.RUnlock()

	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetUserStatus", port)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		return "", "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.shortClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", "", fmt.Errorf("GetUserStatus returned status %d", resp.StatusCode)
	}

	var data struct {
		UserStatus struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"userStatus"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", "", err
	}

	email = strings.TrimSpace(data.UserStatus.Email)
	name = strings.TrimSpace(data.UserStatus.Name)
	if email != "" {
		p.userStatusMu.Lock()
		p.cachedUserEmail = email
		p.cachedUserName = name
		p.cachedUserPort = port
		p.cachedUserAt = time.Now()
		p.userStatusMu.Unlock()
	}
	return email, name, nil
}

// NewProxy creates a new reverse proxy backed by the inspector.
func NewProxy(insp inspector.UpstreamDiscoverer) *Proxy {
	tr := &http.Transport{
		TLSClientConfig:       localtls.ClientConfig(),
		DialContext:           localtls.DialContext,
		ForceAttemptHTTP2:     true,
		DisableCompression:    true,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   100,
		MaxConnsPerHost:       200,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 0, // 0 allows long-polling and streaming ConnectRPC endpoints without dropping
		ExpectContinueTimeout: 1 * time.Second,
	}

	p := &Proxy{
		insp:         insp,
		transport:    tr,
		startTime:    time.Now(),
		msgDedup:     make(map[string]time.Time),
		cascadeDedup: make(map[string]cascadeDedupEntry),
		wsConns:      make(map[*websocket.Conn]struct{}),
		shortClient: &http.Client{
			Timeout:   2 * time.Second,
			Transport: tr,
		},
		mediumClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: tr,
		},
		longClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: tr,
		},
		streamClient: &http.Client{
			Timeout:   0,
			Transport: tr,
		},
		mobileStickyDuration: DefaultMobileStickyDuration,
	}
	p.events.rev = uint64(time.Now().UnixNano()&0xfffffff) + 1

	insp.OnUpdate(func(info inspector.InstanceInfo) {
		p.updateUpstream(info)
	})

	if cur := insp.Current(); cur != nil && cur.IsHealthy {
		p.updateUpstream(*cur)
	}

	return p
}

// trackWSConn registers a WebSocket connection for lifecycle tracking.
// The returned cleanup function must be deferred by the caller.
func (p *Proxy) trackWSConn(conn *websocket.Conn) func() {
	p.wsConnsMu.Lock()
	if p.wsConns == nil {
		p.wsConns = make(map[*websocket.Conn]struct{})
	}
	p.wsConns[conn] = struct{}{}
	p.wsConnsMu.Unlock()
	return func() {
		p.wsConnsMu.Lock()
		delete(p.wsConns, conn)
		p.wsConnsMu.Unlock()
	}
}

// streamingHTTPClient returns a timeout-free HTTP client suitable for long-lived Connect streams,
// falling back to a cloned mediumClient/shortClient transport in struct-literal unit tests.
func (p *Proxy) streamingHTTPClient() *http.Client {
	if p.streamClient != nil {
		return p.streamClient
	}
	if p.mediumClient != nil {
		cp := *p.mediumClient
		cp.Timeout = 0
		return &cp
	}
	if p.shortClient != nil {
		cp := *p.shortClient
		cp.Timeout = 0
		return &cp
	}
	return nil
}

// Shutdown gracefully closes all tracked WebSocket connections and releases upstream resources.
// Must be called before http.Server.Shutdown to allow hijacked connections to drain.
func (p *Proxy) Shutdown() {
	p.agentStateHub.StopAll()
	p.summariesHub.StopAll()
	p.cancelAllCloneJobs()

	p.wsConnsMu.Lock()
	conns := make([]*websocket.Conn, 0, len(p.wsConns))
	for c := range p.wsConns {
		conns = append(conns, c)
	}
	p.wsConnsMu.Unlock()

	for _, c := range conns {
		_ = c.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "gateway shutting down"),
			time.Now().Add(1*time.Second),
		)
		_ = c.Close()
	}

	if p.transport != nil {
		p.transport.CloseIdleConnections()
	}
}

func (p *Proxy) updateUpstream(info inspector.InstanceInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()

	targetURL, _ := url.Parse(fmt.Sprintf("https://127.0.0.1:%d", info.Port))
	rp := httputil.NewSingleHostReverseProxy(targetURL)
	rp.Transport = p.transport
	rp.FlushInterval = -1 // Flush immediately to deliver streaming responses without buffering
	rp.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		if errors.Is(err, context.Canceled) || errors.Is(req.Context().Err(), context.Canceled) {
			// Client disconnected or canceled the request (e.g. page navigation or switching tabs)
			return
		}
		slog.Warn(fmt.Sprintf("[Proxy] Upstream proxy error for %s %s", req.Method, req.URL.Path), "err", err)
		rw.WriteHeader(http.StatusBadGateway)
	}

	originalDirector := rp.Director
	token := info.CSRFToken
	port := info.Port

	rp.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = fmt.Sprintf("127.0.0.1:%d", port)
		req.Header.Set("x-codeium-csrf-token", token)
		if req.Header.Get("Connect-Protocol-Version") == "" {
			req.Header.Set("Connect-Protocol-Version", "1")
		}
		// Ask upstream not to compress (best-effort; HTTP/2 may ignore this)
		req.Header.Del("Accept-Encoding")
	}

	// Decompress gzip responses from upstream so browsers can parse JSON directly.
	// The Antigravity language_server (HTTP/2) may compress regardless of Accept-Encoding.
	rp.ModifyResponse = func(resp *http.Response) error {
		if resp.Request != nil && resp.Request.URL != nil {
			path := resp.Request.URL.Path

			needsModification := resp.StatusCode == http.StatusOK &&
				(strings.HasSuffix(path, "/ReadDir") || strings.HasSuffix(path, "/StatUri") ||
					strings.HasSuffix(path, "/GetFileDetails") || strings.HasSuffix(path, "/FindFiles"))

			if resp.StatusCode == http.StatusOK && isL10nRPCPath(path) &&
				strings.Contains(resp.Header.Get("Content-Type"), "json") {
				if err := localizeRPCResponse(resp); err != nil {
					return err
				}
			}

			if needsModification {
				if resp.Header.Get("Content-Encoding") == "gzip" {
					gzReader, err := GetGzipReader(resp.Body)
					if err != nil {
						return err
					}
					resp.Body = &pooledGzipReadCloser{gz: gzReader, body: resp.Body}
					resp.Header.Del("Content-Encoding")
					resp.Header.Del("Content-Length")
					resp.ContentLength = -1
				}

				raw, err := io.ReadAll(resp.Body)
				if err == nil {
					_ = resp.Body.Close()
					norm := normalizeFileTypes(raw)
					resp.Body = io.NopCloser(bytes.NewReader(norm))
					resp.ContentLength = int64(len(norm))
					resp.Header.Set("Content-Length", strconv.Itoa(len(norm)))
				}
			}
		}
		return nil
	}

	prevPort, prevToken := p.activePort, p.activeToken
	upstreamSwitched := prevPort != 0 && (prevPort != port || prevToken != token)

	p.activeProxy = rp
	p.activePort = port
	p.activeToken = token
	p.userStatusMu.Lock()
	p.cachedUserEmail = ""
	p.cachedUserName = ""
	p.cachedUserPort = 0
	p.cachedUserAt = time.Time{}
	p.userStatusMu.Unlock()
	p.invalidateAllTrajectories()
	p.invalidateProjectsCache()
	if upstreamSwitched {
		liveModels.invalidate()
		if p.mediumClient != nil {
			go p.ensureLiveModels(context.Background())
		}
		go func(prt int) {
			p.agentStateHub.OnUpstreamChanged(p, prt)
			p.summariesHub.OnUpstreamChanged(p, prt)
		}(port)
	}
	if verboseRPC {
		slog.Info(fmt.Sprintf("[Proxy] Updated upstream proxy to 127.0.0.1:%d", port))
	}

	// 升级自检：读取这个实例的二进制方法清单，判断哪些功能可用（后台执行，不阻塞切换）
	go p.refreshCompatibility(info)

	// Reset historical sync state and asynchronously sync historical trajectories
	ResetHistoricalSyncState()
	if port > 0 && p.shortClient != nil {
		go func(prt int, tok string) {
			_ = p.SyncHistoricalTrajectories(prt, tok)
		}(port, token)
	}

	// Clear and warm up desktop static cache in background
	ClearDesktopStaticCache()
	if port > 0 && p.mediumClient != nil {
		go p.WarmupDesktopStatic(port, token)
	}
}

// ServeHTTP handles incoming HTTP requests.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Gateway meta APIs
	if r.URL.Path == "/gateway/status" {
		p.handleStatus(w, r)
		return
	}
	if r.URL.Path == "/gateway/rescan" {
		p.handleRescan(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/messages" {
		p.handleCascadeMessages(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/touch" || r.URL.Path == "/gateway/cascade/invalidate" {
		p.handleCascadeTouch(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/focus" {
		p.handleFocusSession(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/stream" {
		p.HandleCascadeStream(w, r)
		return
	}
	if r.URL.Path == "/gateway/events" {
		p.HandleEvents(w, r)
		return
	}
	if r.URL.Path == "/gateway/projects/alias" {
		p.HandleProjectAlias(w, r)
		return
	}
	if r.URL.Path == "/gateway/projects/create" {
		p.HandleCreateProject(w, r)
		return
	}
	if r.URL.Path == "/gateway/projects" {
		p.HandleProjects(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/new" {
		p.HandleCreateCascade(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/interaction" {
		p.HandleCascadeInteraction(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/task/stop" {
		p.handleCascadeTaskStop(w, r)
		return
	}
	if r.URL.Path == "/gateway/tasks/background" {
		p.HandleListBackgroundTasks(w, r)
		return
	}
	if r.URL.Path == "/gateway/tasks/background/log" {
		p.HandleBackgroundTaskLog(w, r)
		return
	}
	if r.URL.Path == "/gateway/subagent/stop" {
		p.HandleSubagentStop(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/revert/preview" {
		p.HandleCascadeRevertPreview(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/revert/execute" {
		p.HandleCascadeRevertExecute(w, r)
		return
	}
	if r.URL.Path == "/gateway/slash-commands" {
		p.HandleSlashCommands(w, r)
		return
	}
	if r.URL.Path == "/gateway/workflow-templates" {
		p.HandleWorkflowTemplates(w, r)
		return
	}
	if r.URL.Path == "/gateway/models" {
		p.HandleModels(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/changes" {
		p.HandleCascadeChanges(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/chapters" {
		p.HandleCascadeChapters(w, r)
		return
	}
	if r.URL.Path == "/gateway/git/status" {
		p.HandleGitStatus(w, r)
		return
	}
	if r.URL.Path == "/gateway/git/commit" {
		p.HandleGitCommit(w, r)
		return
	}
	if r.URL.Path == "/gateway/git/push" {
		p.HandleGitPush(w, r)
		return
	}

	// Swarm endpoints (Antigravity 2.31+)
	if r.URL.Path == "/gateway/swarm" {
		p.HandleGetSwarm(w, r)
		return
	}
	if r.URL.Path == "/gateway/swarm/feedback" {
		p.HandleSwarmFeedback(w, r)
		return
	}
	if r.URL.Path == "/gateway/swarm/nudge" {
		p.HandleSwarmNudge(w, r)
		return
	}

	// Plugins endpoints (Antigravity 2.31+ / 2.35+ permission grants)
	if r.URL.Path == "/gateway/plugins" {
		p.HandleGetPlugins(w, r)
		return
	}
	if r.URL.Path == "/gateway/plugins/install" {
		p.HandleInstallPlugin(w, r)
		return
	}
	if r.URL.Path == "/gateway/plugins/grants/revoke" {
		p.HandleRevokePluginPermissionGrant(w, r)
		return
	}
	if r.URL.Path == "/gateway/cascade/archive" {
		p.HandleArchiveCascade(w, r)
		return
	}

	// Automations endpoints (GetSidecars / schedule)
	if r.URL.Path == "/gateway/automations" {
		p.HandleGetAutomations(w, r)
		return
	}
	if r.URL.Path == "/gateway/automations/manage" {
		p.HandleManageAutomation(w, r)
		return
	}
	if r.URL.Path == "/gateway/automations/update" {
		p.HandleUpdateAutomation(w, r)
		return
	}
	if r.URL.Path == "/gateway/automations/logs" {
		p.HandleGetAutomationLogs(w, r)
		return
	}

	if r.URL.Path == "/gateway/rpc-mux" {
		p.HandleRPCMux(w, r)
		return
	}

	// Phase 3 native feature routes
	switch {
	case r.URL.Path == "/gateway/context/mentions":
		p.HandleContextMentions(w, r)
		return
	case r.URL.Path == "/gateway/cascade/fork":
		p.HandleCascadeFork(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/gateway/worktree/"):
		p.HandleWorktree(w, r)
		return
	case r.URL.Path == "/gateway/projects/clone":
		p.HandleCloneGitHubRepo(w, r)
		return
	case r.URL.Path == "/gateway/projects/clone/status":
		p.HandleCloneGitHubRepoStatus(w, r)
		return
	case r.URL.Path == "/gateway/projects/clone/cancel":
		p.HandleCloneGitHubRepoCancel(w, r)
		return
	case r.URL.Path == "/gateway/fs/dirs":
		p.HandleListLocalDirs(w, r)
		return
	case r.URL.Path == "/gateway/fs/mkdir":
		p.HandleCreateLocalDir(w, r)
		return
	case r.URL.Path == "/gateway/permissions":
		p.HandlePermissions(w, r)
		return
	case r.URL.Path == "/gateway/memories":
		p.HandleMemories(w, r)
		return
	case r.URL.Path == "/gateway/terminals",
		r.URL.Path == "/gateway/terminal/create",
		r.URL.Path == "/gateway/terminal/input",
		r.URL.Path == "/gateway/terminal/close":
		p.HandleTerminals(w, r)
		return
	case r.URL.Path == "/gateway/terminal/stream":
		p.HandleTerminalStream(w, r)
		return
	case r.URL.Path == "/gateway/cascade/task/background":
		p.HandleSendStepsToBackground(w, r)
		return
	case r.URL.Path == "/gateway/subagent/message":
		p.HandleSubagentMessage(w, r)
		return
	case r.URL.Path == "/gateway/subagent/browser",
		r.URL.Path == "/gateway/subagent/browser/skip",
		r.URL.Path == "/gateway/subagent/browser/whitelist":
		p.HandleBrowserSubagent(w, r)
		return
	case r.URL.Path == "/gateway/audio/dictate":
		p.HandleAudioDictate(w, r)
		return
	case r.URL.Path == "/gateway/narration":
		p.HandleNarration(w, r)
		return
	case r.URL.Path == "/gateway/mcp/toggle",
		r.URL.Path == "/gateway/mcp/refresh":
		p.HandleMcpManage(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/gateway/mcp/app/"):
		p.HandleMcpApp(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/gateway/sidecar-ui/"):
		p.HandleSidecarUI(w, r)
		return
	case r.URL.Path == "/gateway/automations/create",
		r.URL.Path == "/gateway/automations/delete":
		p.HandleAutomationCreateDelete(w, r)
		return
	case r.URL.Path == "/gateway/git/branches",
		r.URL.Path == "/gateway/git/checkout",
		r.URL.Path == "/gateway/git/discard":
		p.HandleGitExtra(w, r)
		return
	case r.URL.Path == "/gateway/projects/trust":
		p.HandleProjectTrust(w, r)
		return
	case r.URL.Path == "/gateway/github/pull-requests":
		p.HandleGitHubPullRequests(w, r)
		return
	case r.URL.Path == "/gateway/btw/start",
		r.URL.Path == "/gateway/btw/reply",
		r.URL.Path == "/gateway/btw/cancel",
		r.URL.Path == "/gateway/btw/list",
		r.URL.Path == "/gateway/btw/status":
		p.HandleBtw(w, r)
		return
	}

	// WebSocket upgrade routes
	if r.URL.Path == "/connect-websocket" {
		p.HandleWebSocket(w, r)
		return
	}
	if r.URL.Path == "/voice/ws" {
		p.HandleVoiceWebSocket(w, r)
		return
	}
	if r.URL.Path == "/voice/health" {
		p.handleVoiceHealth(w, r)
		return
	}

	// ConnectRPC proxy route (prefixed with /api/ or direct proto service prefix)
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/exa.language_server_pb.") {
		p.handleRpcProxy(w, r)
		return
	}

	// Artifacts proxy route
	if strings.HasPrefix(r.URL.Path, "/static/artifacts/") {
		p.handleArtifactProxy(w, r)
		return
	}

	// Desktop static asset fallback
	if IsDesktopStaticPath(r.URL.Path) {
		p.HandleDesktopStatic(w, r)
		return
	}

	http.NotFound(w, r)
}

// SetActiveStream records the currently connected active cascade stream on mobile.
func (p *Proxy) SetActiveStream(cascadeID, title string) {
	p.activeStreamMu.Lock()
	defer p.activeStreamMu.Unlock()
	p.activeStreamCascadeID = cascadeID
	p.activeStreamTitle = title
}

// ClearActiveStream clears the active cascade stream if matching the disconnecting cascade.
func (p *Proxy) ClearActiveStream(cascadeID string) {
	p.activeStreamMu.Lock()
	defer p.activeStreamMu.Unlock()
	if p.activeStreamCascadeID == cascadeID {
		p.activeStreamCascadeID = ""
		p.activeStreamTitle = ""
	}
}

// ActiveStream returns the currently active cascade ID and title, if any.
func (p *Proxy) ActiveStream() (string, string) {
	p.activeStreamMu.RLock()
	defer p.activeStreamMu.RUnlock()
	return p.activeStreamCascadeID, p.activeStreamTitle
}

func (p *Proxy) registerStreamTouchListener(cascadeID string) (<-chan struct{}, func()) {
	p.streamListenersMu.Lock()
	defer p.streamListenersMu.Unlock()
	if p.streamListeners == nil {
		p.streamListeners = make(map[string][]chan struct{})
	}
	ch := make(chan struct{}, 1)
	p.streamListeners[cascadeID] = append(p.streamListeners[cascadeID], ch)
	cleanup := func() {
		p.streamListenersMu.Lock()
		defer p.streamListenersMu.Unlock()
		listeners := p.streamListeners[cascadeID]
		for i, l := range listeners {
			if l == ch {
				p.streamListeners[cascadeID] = append(listeners[:i], listeners[i+1:]...)
				break
			}
		}
		if len(p.streamListeners[cascadeID]) == 0 {
			delete(p.streamListeners, cascadeID)
		}
	}
	return ch, cleanup
}

func (p *Proxy) notifyStreamTouch(cascadeID string) {
	p.streamListenersMu.Lock()
	defer p.streamListenersMu.Unlock()
	if p.streamListeners == nil {
		return
	}
	for _, ch := range p.streamListeners[cascadeID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func publicInstanceInfo(info *inspector.InstanceInfo) *inspector.InstanceInfo {
	if info == nil {
		return nil
	}
	cp := *info
	cp.CSRFToken = ""
	return &cp
}

// SetGatewayIdentity configures the stable gateway ID and display name exposed by /gateway/status.
func (p *Proxy) SetGatewayIdentity(id, name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gatewayID = strings.TrimSpace(id)
	p.gatewayName = strings.TrimSpace(name)
}

// GatewayIdentity returns the configured (gatewayID, gatewayName).
func (p *Proxy) GatewayIdentity() (string, string) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.gatewayID, p.gatewayName
}

func (p *Proxy) handleStatus(w http.ResponseWriter, r *http.Request) {
	cur := p.insp.Current()
	status := "disconnected"
	if cur != nil && cur.IsHealthy {
		status = "connected"
	}

	activeID, activeTitle := p.ActiveStream()
	gwID, gwName := p.GatewayIdentity()
	wantSummary := r != nil && (r.URL.Query().Get("summary") == "1" || strings.EqualFold(r.URL.Query().Get("summary"), "true"))
	var summary *GatewaySummary
	if status == "connected" {
		summary = p.computeGatewaySummary(r.Context(), wantSummary)
	}

	data, err := json.Marshal(GatewayStatus{
		Status:                status,
		OS:                    runtime.GOOS,
		Platform:              runtime.GOOS,
		GatewayID:             gwID,
		GatewayName:           gwName,
		Summary:               summary,
		Upstream:              publicInstanceInfo(cur),
		ActiveStreamCascadeID: activeID,
		ActiveStreamTitle:     activeTitle,
		Timestamp:             time.Now(),
		UnifiedCursor:         p.ArbitrateCursor(),
		Compat:                p.currentCompatibility(cur),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (p *Proxy) handleCascadeTouch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cascadeID := strings.TrimSpace(r.URL.Query().Get("cascadeId"))
	if cascadeID == "" && r.Body != nil {
		var body struct {
			CascadeID string `json:"cascadeId"`
		}
		// SEC-4: Limit body to prevent OOM (cascadeId is always short).
		_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body)
		cascadeID = strings.TrimSpace(body.CascadeID)
	}
	if cascadeID != "" {
		ClearTrajectoryCache(cascadeID)
		ClearPendingMessagesCache(cascadeID)
		p.notifyStreamTouch(cascadeID)
		if verboseRPC {
			slog.Info(fmt.Sprintf("[Proxy] Cascade cache invalidated and stream notified via touch API: %s", shortCascadeID(cascadeID)))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (p *Proxy) handleRescan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	info := p.insp.Scan()
	status := "failed"
	if info != nil && info.IsHealthy {
		status = "connected"
		ResetHistoricalSyncState()
		go func(prt int, tok string) {
			_ = p.SyncHistoricalTrajectories(prt, tok)
		}(info.Port, info.CSRFToken)
	}

	data, err := json.Marshal(GatewayStatus{
		Status:    status,
		Upstream:  publicInstanceInfo(info),
		Timestamp: time.Now(),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
