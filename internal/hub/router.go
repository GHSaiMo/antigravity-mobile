package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/notifier"
	"antigravity-mobile/internal/proxy"

	"github.com/gorilla/websocket"
)

type nodeWatcherEntry struct {
	baseURL string
	token   string
	cancel  context.CancelFunc
}

type localAttachmentEntry struct {
	local      proxy.Attachment
	replicated map[string]proxy.Attachment // nodeID -> remote attachment metadata
}

// Hub aggregates a local optional Antigravity gateway and any number of LAN mgy nodes
// behind one unified gateway endpoint (single QR pairing, auth, E2EE, tunnel, and push).
type Hub struct {
	opts    Options
	store   *Store
	hubID   string
	hubName string

	rootCtx    context.Context
	rootCancel context.CancelFunc

	transport    *http.Transport
	probeClient  *http.Client
	listClient   *http.Client
	rpcClient    *http.Client
	streamClient *http.Client
	probeTimeout time.Duration
	listTimeout  time.Duration

	stateMu    sync.RWMutex
	nodeStates map[string]NodeStatus

	discMu       sync.RWMutex
	discovered   map[string]DiscoveredNode
	probeTrigger chan struct{}

	verifyMu       sync.Mutex
	verifyCooldown map[string]time.Time

	indexMu        sync.RWMutex
	cascadeIndex   map[string]string            // cascadeID -> nodeID ("local" or remote node ID)
	workspaceIndex map[string]map[string]bool   // normalized workspace path/URI -> set of nodeIDs
	projectIDIndex map[string]string            // raw projectID -> nodeID
	taskIndex      map[string]string            // taskID/logURI -> nodeID
	attachments    map[string]*localAttachmentEntry

	events hubEventsBroadcaster

	watchMu      sync.Mutex
	nodeWatchers map[string]nodeWatcherEntry

	wsMu    sync.Mutex
	wsConns map[*websocket.Conn]struct{}
}

// NewHub creates a configured Hub instance. Call Start() to begin discovery, health probing, and event fan-in.
func NewHub(opts Options) (*Hub, error) {
	store, err := NewStore(opts.DataDir, opts.Secret)
	if err != nil {
		return nil, err
	}

	probeTimeout := opts.ProbeTimeout
	if probeTimeout <= 0 {
		probeTimeout = DefaultProbeTimeout
	}
	listTimeout := opts.ListTimeout
	if listTimeout <= 0 {
		listTimeout = DefaultListTimeout
	}
	if opts.ProbeInterval <= 0 {
		opts.ProbeInterval = DefaultProbeInterval
	}

	hubID := strings.TrimSpace(opts.HubID)
	if hubID == "" {
		hubID = "hub-" + randomHex(6)
	}
	hubName := strings.TrimSpace(opts.HubName)
	if hubName == "" {
		hubName = "Multigravity Hub"
	}

	tr := newNodeTransport()
	rootCtx, rootCancel := context.WithCancel(context.Background())

	h := &Hub{
		opts:           opts,
		store:          store,
		hubID:          hubID,
		hubName:        hubName,
		rootCtx:        rootCtx,
		rootCancel:     rootCancel,
		transport:      tr,
		probeClient:    &http.Client{Transport: tr, Timeout: probeTimeout + 200*time.Millisecond},
		listClient:     &http.Client{Transport: tr, Timeout: listTimeout + 300*time.Millisecond},
		rpcClient:      &http.Client{Transport: tr, Timeout: 30 * time.Second},
		streamClient:   &http.Client{Transport: tr, Timeout: 0},
		probeTimeout:   probeTimeout,
		listTimeout:    listTimeout,
		nodeStates:     make(map[string]NodeStatus),
		discovered:     make(map[string]DiscoveredNode),
		probeTrigger:   make(chan struct{}, 1),
		verifyCooldown: make(map[string]time.Time),
		cascadeIndex:   make(map[string]string),
		workspaceIndex: make(map[string]map[string]bool),
		projectIDIndex: make(map[string]string),
		taskIndex:      make(map[string]string),
		attachments:    make(map[string]*localAttachmentEntry),
		nodeWatchers:   make(map[string]nodeWatcherEntry),
		wsConns:        make(map[*websocket.Conn]struct{}),
	}

	// Register CLI-supplied static nodes (idempotent upsert).
	for _, rawSpec := range opts.StaticNodes {
		for _, part := range strings.Split(rawSpec, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			nc, parseErr := ParseStaticNodeSpec(part)
			if parseErr != nil {
				slog.Warn("[Hub] Ignoring invalid static node spec", "spec", part, "err", parseErr)
				continue
			}
			if _, err := store.UpsertNode(nc); err != nil {
				slog.Warn("[Hub] Failed to store static node", "spec", part, "err", err)
			}
		}
	}

	return h, nil
}

// Store returns the underlying persistent node store.
func (h *Hub) Store() *Store { return h.store }

// Secret returns the Hub cluster shared secret.
func (h *Hub) Secret() string { return h.store.Secret() }

// JoinCode returns the current interactive LAN join code.
func (h *Hub) JoinCode() string { return h.store.JoinCode() }

// HubName returns the Hub's display name.
func (h *Hub) HubName() string { return h.hubName }

// Headless reports whether local Antigravity LS is disabled.
func (h *Hub) Headless() bool { return h.opts.Headless }

// Start launches LAN UDP discovery, periodic node health probes, and event fan-in watchers.
func (h *Hub) Start(ctx context.Context) {
	if ctx != nil {
		go func() {
			<-ctx.Done()
			h.Stop()
		}()
	}

	h.startHubDiscovery(h.rootCtx)

	// Bridge local proxy summary updates into unified Hub events (if not headless).
	if !h.opts.Headless && h.opts.Proxy != nil {
		go func() {
			wake, unsub := h.opts.Proxy.SubscribeSummaryUpdates()
			defer unsub()
			for {
				select {
				case <-h.rootCtx.Done():
					return
				case <-wake:
					h.BumpEvents()
				}
			}
		}()
	}

	// Initial probe + background health probe loop.
	go func() {
		h.ProbeAllNodes(h.rootCtx)
		ticker := time.NewTicker(h.opts.ProbeInterval)
		defer ticker.Stop()
		for {
			select {
			case <-h.rootCtx.Done():
				return
			case <-ticker.C:
				h.ProbeAllNodes(h.rootCtx)
			}
		}
	}()
}

// Stop shuts down background discovery, health probes, event watchers, and open bridged WebSockets.
func (h *Hub) Stop() {
	h.rootCancel()

	h.watchMu.Lock()
	for id, entry := range h.nodeWatchers {
		if entry.cancel != nil {
			entry.cancel()
		}
		delete(h.nodeWatchers, id)
	}
	h.watchMu.Unlock()

	h.wsMu.Lock()
	conns := make([]*websocket.Conn, 0, len(h.wsConns))
	for c := range h.wsConns {
		conns = append(conns, c)
	}
	h.wsConns = make(map[*websocket.Conn]struct{})
	h.wsMu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}

	if h.transport != nil {
		h.transport.CloseIdleConnections()
	}
}

func (h *Hub) trackWS(c *websocket.Conn) func() {
	h.wsMu.Lock()
	h.wsConns[c] = struct{}{}
	h.wsMu.Unlock()
	return func() {
		h.wsMu.Lock()
		delete(h.wsConns, c)
		h.wsMu.Unlock()
	}
}

func (h *Hub) localLSHealthy() bool {
	if h.opts.Headless || h.opts.Inspector == nil {
		return false
	}
	cur := h.opts.Inspector.Current()
	return cur != nil && cur.IsHealthy && cur.Port > 0
}

// HasAnyHealthyNode reports whether local LS (non-headless) or any enabled remote node has a healthy LS.
func (h *Hub) HasAnyHealthyNode() bool {
	if h.localLSHealthy() {
		return true
	}
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	for _, st := range h.nodeStates {
		if st.Enabled && st.Online && st.LSHealthy {
			return true
		}
	}
	return false
}

// ProbeAllNodes concurrently probes all registered remote nodes and ensures event watchers for online nodes.
func (h *Hub) ProbeAllNodes(ctx context.Context) {
	nodes := h.store.ListNodes()
	if len(nodes) == 0 {
		return
	}
	var wg sync.WaitGroup
	changed := false
	var changedMu sync.Mutex

	for _, n := range nodes {
		wg.Add(1)
		go func(node NodeConfig) {
			defer wg.Done()
			st := h.probeNode(ctx, node)

			h.stateMu.Lock()
			prev, hadPrev := h.nodeStates[node.ID]
			st.CascadeCount = prev.CascadeCount
			h.nodeStates[node.ID] = st
			h.stateMu.Unlock()

			if st.Online {
				h.ensureRemoteEventWatcher(node)
			}
			if !hadPrev || prev.Online != st.Online || prev.LSHealthy != st.LSHealthy {
				changedMu.Lock()
				changed = true
				changedMu.Unlock()
			}
		}(n)
	}
	wg.Wait()

	if changed {
		h.BumpEvents()
	}
}

func (h *Hub) probeNodeAndRefresh(node NodeConfig) {
	st := h.probeNode(h.rootCtx, node)
	h.stateMu.Lock()
	h.nodeStates[node.ID] = st
	h.stateMu.Unlock()
	if st.Online {
		h.ensureRemoteEventWatcher(node)
	}
	h.BumpEvents()
}

func (h *Hub) markNodeError(nodeID string, err error) {
	h.stateMu.Lock()
	st := h.nodeStates[nodeID]
	wasOnline := st.Online
	st.ID = nodeID
	st.Online = false
	st.LSHealthy = false
	if err != nil {
		st.LastError = err.Error()
	}
	h.nodeStates[nodeID] = st
	h.stateMu.Unlock()
	if wasOnline {
		h.BumpEvents()
	}
}

func (h *Hub) updateNodeDesktopState(nodeID string, lsHealthy bool) {
	h.stateMu.Lock()
	st, ok := h.nodeStates[nodeID]
	if ok && st.LSHealthy != lsHealthy {
		st.LSHealthy = lsHealthy
		st.Online = true
		st.LastSeen = time.Now()
		h.nodeStates[nodeID] = st
		h.stateMu.Unlock()
		h.BumpEvents()
		return
	}
	h.stateMu.Unlock()
}

// ValidateHubAuthToken checks whether an incoming token belongs to a registered node (used by AuthPolicy).
func (h *Hub) ValidateHubAuthToken(token string) (string, bool) {
	if node, ok := h.store.ValidateNodeToken(token); ok {
		return "hub-node:" + node.ID, true
	}
	return "", false
}

// indexCascade records cascadeId -> nodeID.
func (h *Hub) indexCascade(cascadeID, nodeID string) {
	cid := strings.TrimSpace(cascadeID)
	nid := strings.TrimSpace(nodeID)
	if cid == "" || nid == "" {
		return
	}
	h.indexMu.Lock()
	h.cascadeIndex[cid] = nid
	h.indexMu.Unlock()
}

func (h *Hub) removeCascade(cascadeID string) {
	cid := strings.TrimSpace(cascadeID)
	if cid == "" {
		return
	}
	h.indexMu.Lock()
	delete(h.cascadeIndex, cid)
	h.indexMu.Unlock()
}

func (h *Hub) lookupCascadeNode(cascadeID string) (string, bool) {
	cid := strings.TrimSpace(cascadeID)
	if cid == "" {
		return "", false
	}
	h.indexMu.RLock()
	nid, ok := h.cascadeIndex[cid]
	h.indexMu.RUnlock()
	return nid, ok
}

func (h *Hub) indexWorkspace(rawURI, nodeID string) {
	norm := normalizeWorkspaceKey(rawURI)
	if norm == "" || nodeID == "" {
		return
	}
	h.indexMu.Lock()
	set := h.workspaceIndex[norm]
	if set == nil {
		set = make(map[string]bool)
		h.workspaceIndex[norm] = set
	}
	set[nodeID] = true
	h.indexMu.Unlock()
}

func (h *Hub) lookupUniqueWorkspaceNode(rawURI string) (string, bool) {
	norm := normalizeWorkspaceKey(rawURI)
	if norm == "" {
		return "", false
	}
	h.indexMu.RLock()
	defer h.indexMu.RUnlock()
	if set, ok := h.workspaceIndex[norm]; ok && len(set) == 1 {
		for nid := range set {
			return nid, true
		}
	}
	// Prefix match for files inside a uniquely owned workspace directory.
	var bestNode string
	bestLen := 0
	ambiguous := false
	for root, set := range h.workspaceIndex {
		if len(root) < 4 {
			continue
		}
		if norm == root || strings.HasPrefix(norm, root+"/") {
			if len(root) > bestLen {
				if len(set) == 1 {
					for nid := range set {
						bestNode = nid
					}
					bestLen = len(root)
					ambiguous = false
				} else {
					ambiguous = true
					bestLen = len(root)
				}
			}
		}
	}
	if !ambiguous && bestNode != "" {
		return bestNode, true
	}
	return "", false
}

func (h *Hub) indexProjectID(projectID, nodeID string) {
	pid := strings.TrimSpace(projectID)
	if pid == "" || nodeID == "" {
		return
	}
	h.indexMu.Lock()
	h.projectIDIndex[pid] = nodeID
	h.indexMu.Unlock()
}

func (h *Hub) lookupProjectIDNode(projectID string) (string, bool) {
	pid := strings.TrimSpace(projectID)
	if pid == "" {
		return "", false
	}
	h.indexMu.RLock()
	nid, ok := h.projectIDIndex[pid]
	h.indexMu.RUnlock()
	return nid, ok
}

func (h *Hub) indexTask(key, nodeID string) {
	key = strings.TrimSpace(key)
	if key == "" || nodeID == "" {
		return
	}
	h.indexMu.Lock()
	h.taskIndex[key] = nodeID
	h.indexMu.Unlock()
}

func (h *Hub) lookupTaskNode(key string) (string, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false
	}
	h.indexMu.RLock()
	nid, ok := h.taskIndex[key]
	h.indexMu.RUnlock()
	return nid, ok
}

func (h *Hub) isMultiNode() bool {
	if h.opts.Headless {
		return true
	}
	for _, n := range h.store.ListNodes() {
		if n.Enabled {
			return true
		}
	}
	return false
}

func (h *Hub) localNodeDisplayName() string {
	name := strings.TrimSpace(h.hubName)
	if name == "" {
		return "Local"
	}
	return name
}

func (h *Hub) activeFilterMatches(nodeID, nodeName string) bool {
	f := h.store.ActiveFilter()
	if f == "" {
		return true
	}
	return strings.EqualFold(f, nodeID) || strings.EqualFold(f, nodeName)
}

func (h *Hub) filteredRemoteNodes() []NodeConfig {
	all := h.store.ListNodes()
	var out []NodeConfig
	for _, n := range all {
		if !n.Enabled {
			continue
		}
		if !h.activeFilterMatches(n.ID, n.Name) {
			continue
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (h *Hub) includeLocalInAggregation() bool {
	if h.opts.Headless {
		return false
	}
	return h.activeFilterMatches(LocalNodeID, h.localNodeDisplayName())
}

// pickDefaultRemoteOrLocal chooses the target node when no explicit cascade/workspace matches.
// Returns (node, true) for a remote node, or (zero, false) to handle locally.
func (h *Hub) pickDefaultTarget() (NodeConfig, bool) {
	// If single-node filter points to a specific remote node, prefer it.
	if f := h.store.ActiveFilter(); f != "" && !strings.EqualFold(f, LocalNodeID) && !strings.EqualFold(f, h.localNodeDisplayName()) {
		if n, ok := h.store.GetNode(f); ok && n.Enabled {
			return n, true
		}
	}
	// Non-headless with healthy local LS -> handle locally.
	if !h.opts.Headless && h.localLSHealthy() {
		return NodeConfig{}, false
	}
	// Headless or local LS down -> pick first online healthy remote node (or first online node).
	nodes := h.filteredRemoteNodes()
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	for _, n := range nodes {
		if st, ok := h.nodeStates[n.ID]; ok && st.Online && st.LSHealthy {
			return n, true
		}
	}
	for _, n := range nodes {
		if st, ok := h.nodeStates[n.ID]; ok && st.Online {
			return n, true
		}
	}
	if h.opts.Headless && len(nodes) > 0 {
		return nodes[0], true
	}
	return NodeConfig{}, false
}

// WrapHandler wraps the gateway's inner HTTP handler (mounted after AuthMiddleware and inside E2EE).
// Requests with X-Mgy-Hub-Forwarded: 1 bypass Hub logic to prevent forwarding loops.
func (h *Hub) WrapHandler(local http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strict loop prevention: requests forwarded from a Hub are served strictly locally.
		if r.Header.Get(HeaderHubForwarded) == "1" {
			local.ServeHTTP(w, r)
			return
		}

		path := r.URL.Path

		// 1. Hub cluster management & internal node-to-hub endpoints
		switch {
		case path == "/api/v1/hub/register":
			h.handleNodeRegister(w, r)
			return
		case path == "/api/v1/hub/internal/event":
			h.handleInternalNodeEvent(w, r)
			return
		case path == "/gateway/hub/status":
			h.handleHubStatus(w, r)
			return
		case path == "/gateway/hub/nodes" || strings.HasPrefix(path, "/gateway/hub/nodes/"):
			h.handleHubNodes(w, r)
			return
		case path == "/gateway/hub/filter":
			h.handleHubFilter(w, r)
			return
		case path == "/gateway/hub/pair/start":
			h.handleHubPairStart(w, r)
			return
		case path == "/gateway/hub/pair/confirm":
			h.handleHubPairConfirm(w, r)
			return
		}

		// 2. Readiness probe: ready if local LS is healthy OR any remote node is healthy.
		if path == "/readyz" && r.Method == http.MethodGet {
			if h.HasAnyHealthyNode() {
				writeJSON(w, http.StatusOK, map[string]any{
					"status":   "ready",
					"mode":     "hub",
					"headless": h.opts.Headless,
					"os":       runtime.GOOS,
					"platform": runtime.GOOS,
				})
				return
			}
			local.ServeHTTP(w, r)
			return
		}

		// 3. Unified Hub WebSocket event stream (/gateway/events)
		if path == "/gateway/events" {
			h.HandleEvents(w, r)
			return
		}

		// 4. Aggregated /gateway/status
		if path == "/gateway/status" && r.Method == http.MethodGet {
			h.handleAggregatedStatus(w, r, local)
			return
		}

		// 5. Aggregated GetAllCascadeTrajectories
		if isRPCMethod(path, "GetAllCascadeTrajectories") && r.Method == http.MethodPost {
			h.handleAggregatedTrajectories(w, r, local)
			return
		}

		// 6. Phase 2: Projects aggregation (GET /gateway/projects & ReadProjects)
		if path == "/gateway/projects" && r.Method == http.MethodGet {
			h.handleAggregatedProjects(w, r, local)
			return
		}
		if isRPCMethod(path, "ReadProjects") && r.Method == http.MethodPost {
			h.handleAggregatedReadProjects(w, r, local)
			return
		}

		// 7. Phase 2: SearchConversations fan-out & merge
		if isRPCMethod(path, "SearchConversations") && r.Method == http.MethodPost {
			h.handleAggregatedSearch(w, r, local)
			return
		}

		// 8. Phase 2: Global background tasks aggregation & task log/stop routing
		if path == "/gateway/tasks/background" {
			h.handleAggregatedBackgroundTasks(w, r, local)
			return
		}
		if path == "/gateway/tasks/background/log" && r.Method == http.MethodPost {
			h.handleBackgroundTaskLogRoute(w, r, local)
			return
		}

		// 9. Phase 2: Permission preset broadcast
		if path == "/gateway/permissions" {
			h.handleHubPermissions(w, r, local)
			return
		}

		// 10. Phase 2: Attachments upload (Hub inbox staging + direct routing if cascadeId/node known)
		if path == "/api/v1/attachments" && r.Method == http.MethodPost {
			h.handleHubAttachmentUpload(w, r, local)
			return
		}

		// 11. Phase 2: Cross-node file & artifact reading
		if path == "/api/v1/files/content" ||
			path == "/api/v1/files/raw" ||
			path == "/api/v1/files/wechat-chat" ||
			path == "/api/v1/files/zip-entry" ||
			strings.HasPrefix(path, "/static/artifacts/") {
			h.handleCrossNodeFile(w, r, local)
			return
		}

		// 12. Phase 2: New conversation & workspace-targeted creation routes
		if path == "/gateway/cascade/new" ||
			isRPCMethod(path, "StartCascade") ||
			strings.HasPrefix(path, "/gateway/worktree/") ||
			path == "/gateway/projects/alias" ||
			path == "/gateway/projects/trust" ||
			path == "/gateway/git/status" ||
			path == "/gateway/git/commit" ||
			path == "/gateway/git/push" ||
			path == "/gateway/git/branches" ||
			path == "/gateway/git/checkout" ||
			path == "/gateway/git/discard" ||
			path == "/gateway/search/code" ||
			path == "/gateway/search/files" {
			h.handleWorkspaceOrCreationRoute(w, r, local)
			return
		}

		// 13. Cascade-scoped WebSockets & HTTP/ConnectRPC routes
		if isHubRoutablePath(path) {
			h.handleCascadeRoutedRequest(w, r, local)
			return
		}

		// All other routes (auth, E2EE, push-token registration, static web UI, healthz, version) stay on Hub!
		local.ServeHTTP(w, r)
	})
}

func isRPCMethod(path, method string) bool {
	return strings.HasSuffix(path, "/"+method)
}

func isHubRoutablePath(path string) bool {
	if strings.HasPrefix(path, "/gateway/") ||
		strings.HasPrefix(path, "/exa.language_server_pb.") ||
		strings.HasPrefix(path, "/exa.browser_viewer_pb.") ||
		strings.HasPrefix(path, "/exa.remoting.") ||
		strings.HasPrefix(path, "/product.v1.") ||
		path == "/connect-websocket" ||
		path == "/voice/ws" ||
		path == "/voice/health" {
		return true
	}
	if strings.HasPrefix(path, "/api/exa.") || strings.HasPrefix(path, "/api/product.") {
		return true
	}
	if strings.HasPrefix(path, "/api/v1/cockpit/") {
		return true
	}
	return false
}

// =========================================================================
// Hub Cluster Status & Management Endpoints
// =========================================================================

// BuildStatusResponse builds the full HubStatusResponse snapshot.
func (h *Hub) BuildStatusResponse(includeSecrets bool) HubStatusResponse {
	localHealthy := h.localLSHealthy()
	nodes := h.store.ListNodes()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })

	h.stateMu.RLock()
	states := make([]NodeStatus, 0, len(nodes)+1)
	onlineCount := 0
	if !h.opts.Headless {
		localStatus := NodeStatus{
			ID:        LocalNodeID,
			Name:      h.localNodeDisplayName(),
			OS:        runtime.GOOS,
			Local:     true,
			Enabled:   true,
			Online:    true,
			LSHealthy: localHealthy,
			LastSeen:  time.Now(),
		}
		states = append(states, localStatus)
		onlineCount++
	}
	for _, n := range nodes {
		st, ok := h.nodeStates[n.ID]
		if !ok {
			st = NodeStatus{
				ID:      n.ID,
				Name:    n.Name,
				BaseURL: n.BaseURL,
				OS:      n.OS,
				Version: n.Version,
				Static:  n.Static,
				Enabled: n.Enabled,
			}
		} else {
			st.Name = n.Name
			st.BaseURL = n.BaseURL
			st.Static = n.Static
			st.Enabled = n.Enabled
		}
		if st.Online {
			onlineCount++
		}
		states = append(states, st)
	}
	h.stateMu.RUnlock()

	disc := h.DiscoverableNodes()

	filter := h.store.ActiveFilter()
	if filter == "" {
		filter = "all"
	}

	resp := HubStatusResponse{
		Mode:            "hub",
		Headless:        h.opts.Headless,
		HubID:           h.hubID,
		HubName:         h.hubName,
		ActiveFilter:    filter,
		SecretSet:       h.store.Secret() != "",
		LocalOnline:     !h.opts.Headless,
		LocalLSHealthy:  localHealthy,
		OnlineNodeCount: onlineCount,
		TotalNodeCount:  len(states),
		Nodes:           states,
		Discovered:      disc,
		Timestamp:       time.Now(),
	}
	if includeSecrets {
		resp.JoinCode = h.store.JoinCode()
	}
	return resp
}

func (h *Hub) handleHubStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if r.URL.Query().Get("scan") == "1" {
		h.ScanLAN(r.Context(), 1200*time.Millisecond)
	}
	if r.URL.Query().Get("probe") == "1" {
		h.ProbeAllNodes(r.Context())
	}
	writeJSON(w, http.StatusOK, h.BuildStatusResponse(true))
}

func (h *Hub) handleHubNodes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Query().Get("scan") == "1" {
			h.ScanLAN(r.Context(), 1200*time.Millisecond)
		}
		if r.URL.Query().Get("probe") == "1" {
			h.ProbeAllNodes(r.Context())
		}
		writeJSON(w, http.StatusOK, h.BuildStatusResponse(true))
		return

	case http.MethodPost:
		var req struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			BaseURL string `json:"base_url"`
			URL     string `json:"url"`
			Token   string `json:"token"`
			Enabled *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}
		baseURL := strings.TrimSpace(req.BaseURL)
		if baseURL == "" {
			baseURL = strings.TrimSpace(req.URL)
		}
		// Allow approving a LAN-discovered node by ID without retyping URL.
		if baseURL == "" && req.ID != "" {
			h.discMu.RLock()
			if d, ok := h.discovered[req.ID]; ok {
				baseURL = d.BaseURL
				if req.Name == "" {
					req.Name = d.Name
				}
			}
			h.discMu.RUnlock()
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		cfg, err := h.store.UpsertNode(NodeConfig{
			ID:      req.ID,
			Name:    req.Name,
			BaseURL: baseURL,
			Token:   strings.TrimSpace(req.Token),
			Static:  true,
			Enabled: enabled,
		})
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.probeNodeAndRefresh(cfg)
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "ok",
			"node":   cfg,
		})
		return

	case http.MethodDelete:
		id := strings.TrimPrefix(r.URL.Path, "/gateway/hub/nodes/")
		id = strings.TrimSpace(id)
		if id == "" {
			id = strings.TrimSpace(r.URL.Query().Get("id"))
		}
		if id == "" {
			writeJSONError(w, http.StatusBadRequest, "node id is required")
			return
		}
		if !h.store.RemoveNode(id) {
			writeJSONError(w, http.StatusNotFound, "node not found")
			return
		}
		h.stopRemoteEventWatcher(id)
		h.stateMu.Lock()
		delete(h.nodeStates, id)
		h.stateMu.Unlock()
		h.BumpEvents()
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "removed": id})
		return

	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Hub) handleHubFilter(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		f := h.store.ActiveFilter()
		if f == "" {
			f = "all"
		}
		writeJSON(w, http.StatusOK, map[string]string{"active_filter": f})
	case http.MethodPost:
		var req struct {
			NodeID string `json:"node_id"`
			Filter string `json:"filter"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 16*1024)).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid json")
			return
		}
		target := strings.TrimSpace(req.NodeID)
		if target == "" {
			target = strings.TrimSpace(req.Filter)
		}
		// Resolve by node name or ID (if not all/local)
		if target != "" && !strings.EqualFold(target, "all") && !strings.EqualFold(target, LocalNodeID) && !strings.EqualFold(target, h.localNodeDisplayName()) {
			if n, ok := h.store.GetNode(target); ok {
				target = n.ID
			} else {
				writeJSONError(w, http.StatusNotFound, fmt.Sprintf("unknown node: %s", target))
				return
			}
		} else if strings.EqualFold(target, h.localNodeDisplayName()) {
			target = LocalNodeID
		}
		saved := h.store.SetActiveFilter(target)
		if saved == "" {
			saved = "all"
		}
		h.BumpEvents()
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "active_filter": saved})
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleNodeRegister handles POST /api/v1/hub/register from LAN mgy nodes.
// Protected by constant-time cluster secret or interactive join code verification.
func (h *Hub) handleNodeRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req RegisterNodeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !h.store.AuthorizeJoin(req.Secret, req.JoinCode) {
		slog.Warn("[Hub] Rejected unauthorized node registration attempt", "remote", r.RemoteAddr, "name", req.Name)
		writeJSONError(w, http.StatusUnauthorized, "unauthorized: invalid hub secret or join code")
		return
	}

	baseURL := NormalizeBaseURL(req.BaseURL)
	if baseURL == "" {
		if req.Port <= 0 || req.Port > 65535 {
			writeJSONError(w, http.StatusBadRequest, "base_url or valid port is required")
			return
		}
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || host == "" {
			host = r.RemoteAddr
		}
		if host == "::1" {
			host = "127.0.0.1"
		}
		baseURL = fmt.Sprintf("http://%s:%d", host, req.Port)
	}

	nodeID := SanitizeNodeID(req.NodeID)
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = nodeID
	}
	if nodeID == "" || strings.EqualFold(nodeID, LocalNodeID) {
		nodeID = SanitizeNodeID(name)
	}
	if nodeID == "" || strings.EqualFold(nodeID, LocalNodeID) {
		nodeID = "node-" + randomHex(4)
	}

	token := strings.TrimSpace(req.NodeToken)
	if token == "" {
		token = DeriveNodeToken(h.store.Secret(), nodeID)
	}

	cfg, err := h.store.UpsertNode(NodeConfig{
		ID:      nodeID,
		Name:    name,
		BaseURL: baseURL,
		Token:   token,
		OS:      strings.TrimSpace(req.OS),
		Version: strings.TrimSpace(req.Version),
		Static:  false,
		Enabled: true,
	})
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	slog.Info("[Hub] Registered LAN node", "node_id", cfg.ID, "name", cfg.Name, "url", cfg.BaseURL)
	go h.probeNodeAndRefresh(cfg)

	writeJSON(w, http.StatusOK, RegisterNodeResponse{
		Status:    "ok",
		HubID:     h.hubID,
		HubName:   h.hubName,
		NodeID:    cfg.ID,
		NodeName:  cfg.Name,
		NodeToken: cfg.Token,
	})
}

// handleInternalNodeEvent handles POST /api/v1/hub/internal/event pushed by registered remote nodes.
func (h *Hub) handleInternalNodeEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	token := strings.TrimSpace(r.Header.Get(HeaderHubToken))
	if token == "" {
		if authHdr := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(authHdr), "bearer ") {
			token = strings.TrimSpace(authHdr[7:])
		}
	}
	node, ok := h.store.ValidateNodeToken(token)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized node token")
		return
	}

	var ev NodeEventPayload
	if err := json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&ev); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid event json")
		return
	}

	nodeName := node.Name
	if strings.TrimSpace(ev.NodeName) != "" && nodeName == "" {
		nodeName = strings.TrimSpace(ev.NodeName)
	}
	if ev.CascadeID != "" {
		h.indexCascade(ev.CascadeID, node.ID)
	}

	switch ev.Type {
	case "list_changed":
		h.BumpEvents()

	case "push":
		h.BumpEvents()
		if h.opts.Notifier != nil {
			prefix := "[" + nodeName + "] "
			title := ev.Title
			body := ev.Body
			if !ev.Silent && nodeName != "" && !strings.HasPrefix(body, prefix) && !strings.HasPrefix(title, prefix) {
				body = prefix + body
			}
			var localizeFn func(lang string) (string, string)
			if len(ev.Localized) > 0 {
				locCopy := make(map[string][2]string, len(ev.Localized))
				for k, v := range ev.Localized {
					lt, lb := v[0], v[1]
					if !ev.Silent && nodeName != "" && !strings.HasPrefix(lb, prefix) && !strings.HasPrefix(lt, prefix) {
						lb = prefix + lb
					}
					locCopy[k] = [2]string{lt, lb}
				}
				localizeFn = func(lang string) (string, string) {
					if pair, ok := locCopy[lang]; ok {
						return pair[0], pair[1]
					}
					if pair, ok := locCopy["en"]; ok {
						return pair[0], pair[1]
					}
					return title, body
				}
			}
			payload := notifier.PushPayload{
				Title:     title,
				Body:      body,
				URL:       ev.URL,
				Level:     ev.Level,
				Sound:     ev.Sound,
				Category:  ev.Category,
				CascadeID: ev.CascadeID,
				Silent:    ev.Silent,
				Action:    ev.Action,
				Localize:  localizeFn,
			}
			go func() {
				if err := h.opts.Notifier.DispatchForwardedPush(context.Background(), payload); err != nil {
					slog.Warn("[Hub] Failed to dispatch forwarded push from node", "node", node.ID, "err", err)
				}
			}()
		}

	case "live_activity":
		if ev.Details != nil {
			if ev.Details.CascadeID != "" {
				h.indexCascade(ev.Details.CascadeID, node.ID)
			}
			if h.opts.Notifier != nil && h.opts.Notifier.LiveActivity() != nil {
				d := *ev.Details
				prefix := "[" + nodeName + "] "
				if nodeName != "" && d.Title != "" && !strings.HasPrefix(d.Title, prefix) {
					d.Title = prefix + d.Title
				}
				h.opts.Notifier.LiveActivity().EnqueueLatest(&d)
			}
		}

	case "live_activity_end":
		if h.opts.Notifier != nil && h.opts.Notifier.LiveActivity() != nil && ev.CascadeID != "" {
			title := ev.Title
			prefix := "[" + nodeName + "] "
			if nodeName != "" && title != "" && !strings.HasPrefix(title, prefix) {
				title = prefix + title
			}
			go h.opts.Notifier.LiveActivity().EndSession(context.Background(), ev.CascadeID, title, ev.EndState, ev.TotalSteps)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// =========================================================================
// Phase 1 Core Aggregation: /gateway/status & GetAllCascadeTrajectories
// =========================================================================

func (h *Hub) handleAggregatedStatus(w http.ResponseWriter, r *http.Request, local http.Handler) {
	wantSummary := r.URL.Query().Get("summary") == "1" || strings.EqualFold(r.URL.Query().Get("summary"), "true")

	var localStatus proxy.GatewayStatus
	hasLocal := false
	if !h.opts.Headless {
		rec := httptest.NewRecorder()
		local.ServeHTTP(rec, cloneLocalRequest(r, nil))
		if rec.Code == http.StatusOK && json.Unmarshal(rec.Body.Bytes(), &localStatus) == nil {
			hasLocal = true
		}
	}

	remoteNodes := h.filteredRemoteNodes()
	type remoteRes struct {
		node   NodeConfig
		status *proxy.GatewayStatus
		ok     bool
	}
	results := make([]remoteRes, len(remoteNodes))
	if len(remoteNodes) > 0 {
		var wg sync.WaitGroup
		for i, n := range remoteNodes {
			wg.Add(1)
			go func(idx int, node NodeConfig) {
				defer wg.Done()
				// Fast path: reuse recent probe if !wantSummary and seen within 3s.
				if !wantSummary {
					h.stateMu.RLock()
					st, ok := h.nodeStates[node.ID]
					h.stateMu.RUnlock()
					if ok && st.Online && time.Since(st.LastSeen) < 3*time.Second {
						statusStr := "disconnected"
						if st.LSHealthy {
							statusStr = "connected"
						}
						results[idx] = remoteRes{
							node: node,
							ok:   true,
							status: &proxy.GatewayStatus{
								Status:  statusStr,
								Summary: st.Summary,
							},
						}
						return
					}
				}
				ctx, cancel := context.WithTimeout(r.Context(), h.probeTimeout)
				defer cancel()
				q := ""
				if wantSummary {
					q = "summary=1"
				}
				code, _, body, err := h.doNodeRequest(ctx, h.probeClient, node, http.MethodGet, "/gateway/status", q, nil, "")
				if err != nil || code != http.StatusOK {
					return
				}
				var gs proxy.GatewayStatus
				if json.Unmarshal(body, &gs) == nil {
					results[idx] = remoteRes{node: node, status: &gs, ok: true}
					h.stateMu.Lock()
					st := h.nodeStates[node.ID]
					st.ID = node.ID
					st.Name = node.Name
					st.BaseURL = node.BaseURL
					st.Enabled = node.Enabled
					st.Online = true
					st.LSHealthy = strings.EqualFold(gs.Status, "connected")
					st.LastSeen = time.Now()
					st.Summary = gs.Summary
					h.nodeStates[node.ID] = st
					h.stateMu.Unlock()
				}
			}(i, n)
		}
		wg.Wait()
	}

	out := localStatus
	if !hasLocal {
		out = proxy.GatewayStatus{
			Status:      "disconnected",
			OS:          runtime.GOOS,
			Platform:    runtime.GOOS,
			GatewayID:   h.hubID,
			GatewayName: h.hubName,
			Timestamp:   time.Now(),
		}
		if h.opts.Proxy != nil {
			if gwID, gwName := h.opts.Proxy.GatewayIdentity(); gwID != "" {
				out.GatewayID = gwID
				if gwName != "" {
					out.GatewayName = gwName
				}
			}
		}
	}

	anyConnected := !h.opts.Headless && strings.EqualFold(out.Status, "connected") && h.includeLocalInAggregation()
	aggSummary := &proxy.GatewaySummary{}
	hasSummary := false
	if hasLocal && out.Summary != nil && h.includeLocalInAggregation() {
		aggSummary.RunningCount += out.Summary.RunningCount
		aggSummary.ActionCount += out.Summary.ActionCount
		aggSummary.UnreadCount += out.Summary.UnreadCount
		aggSummary.TotalActive += out.Summary.TotalActive
		hasSummary = true
	}

	for _, res := range results {
		if !res.ok || res.status == nil {
			continue
		}
		if strings.EqualFold(res.status.Status, "connected") {
			anyConnected = true
		}
		if res.status.Summary != nil {
			aggSummary.RunningCount += res.status.Summary.RunningCount
			aggSummary.ActionCount += res.status.Summary.ActionCount
			aggSummary.UnreadCount += res.status.Summary.UnreadCount
			aggSummary.TotalActive += res.status.Summary.TotalActive
			hasSummary = true
		}
		if out.Compat == nil && res.status.Compat != nil {
			out.Compat = res.status.Compat
		}
		if out.Upstream == nil && res.status.Upstream != nil {
			out.Upstream = res.status.Upstream
		}
	}

	if anyConnected {
		out.Status = "connected"
	} else {
		out.Status = "disconnected"
	}
	if hasSummary || wantSummary {
		out.Summary = aggSummary
	}
	out.Timestamp = time.Now()

	writeJSON(w, http.StatusOK, out)
}

func (h *Hub) handleAggregatedTrajectories(w http.ResponseWriter, r *http.Request, local http.Handler) {
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 5<<20))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		bodyBytes = []byte("{}")
	}

	multiNode := h.isMultiNode()
	remoteNodes := h.filteredRemoteNodes()
	includeLocal := h.includeLocalInAggregation()

	type trajSourceResult struct {
		nodeID    string
		nodeName  string
		summaries map[string]map[string]any
		ok        bool
	}

	totalSources := len(remoteNodes)
	if includeLocal {
		totalSources++
	}
	results := make([]trajSourceResult, 0, totalSources)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Local source
	if includeLocal {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			local.ServeHTTP(rec, cloneLocalRequest(r, bodyBytes))
			if rec.Code != http.StatusOK {
				return
			}
			var env struct {
				TrajectorySummaries map[string]map[string]any `json:"trajectorySummaries"`
			}
			if json.Unmarshal(rec.Body.Bytes(), &env) == nil && env.TrajectorySummaries != nil {
				mu.Lock()
				results = append(results, trajSourceResult{
					nodeID:    LocalNodeID,
					nodeName:  h.localNodeDisplayName(),
					summaries: env.TrajectorySummaries,
					ok:        true,
				})
				mu.Unlock()
			}
		}()
	}

	// Remote nodes (concurrent, bounded LAN timeout so offline/sleeping nodes NEVER block!)
	for _, n := range remoteNodes {
		wg.Add(1)
		go func(node NodeConfig) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(r.Context(), h.listTimeout)
			defer cancel()
			code, _, respBody, reqErr := h.doNodeRequest(
				ctx, h.listClient, node, http.MethodPost,
				"/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories",
				"", bodyBytes, "application/json",
			)
			if reqErr != nil {
				h.markNodeError(node.ID, reqErr)
				return
			}
			if code != http.StatusOK {
				return
			}
			var env struct {
				TrajectorySummaries map[string]map[string]any `json:"trajectorySummaries"`
			}
			if json.Unmarshal(respBody, &env) != nil || env.TrajectorySummaries == nil {
				return
			}
			h.stateMu.Lock()
			st := h.nodeStates[node.ID]
			st.ID = node.ID
			st.Name = node.Name
			st.BaseURL = node.BaseURL
			st.Enabled = node.Enabled
			st.Online = true
			st.LSHealthy = true
			st.LastSeen = time.Now()
			st.CascadeCount = len(env.TrajectorySummaries)
			h.nodeStates[node.ID] = st
			h.stateMu.Unlock()

			mu.Lock()
			results = append(results, trajSourceResult{
				nodeID:    node.ID,
				nodeName:  node.Name,
				summaries: env.TrajectorySummaries,
				ok:        true,
			})
			mu.Unlock()
		}(n)
	}

	wg.Wait()

	merged := make(map[string]map[string]any)
	for _, res := range results {
		if !res.ok {
			continue
		}
		for cid, summary := range res.summaries {
			if summary == nil {
				continue
			}
			h.indexCascade(cid, res.nodeID)
			// Also index parent/root cascade IDs and workspaces for Phase 2 routing
			if meta, ok := summary["trajectoryMetadata"].(map[string]any); ok {
				if p, _ := meta["parentConversationId"].(string); p != "" {
					h.indexCascade(p, res.nodeID)
				}
				if root, _ := meta["rootConversationId"].(string); root != "" {
					h.indexCascade(root, res.nodeID)
				}
				if pid, _ := meta["projectId"].(string); pid != "" {
					h.indexProjectID(pid, res.nodeID)
				}
				if uris, ok := meta["workspaceUris"].([]any); ok {
					for _, u := range uris {
						if us, _ := u.(string); us != "" {
							h.indexWorkspace(us, res.nodeID)
						}
					}
				}
			}
			if workspaces, ok := summary["workspaces"].([]any); ok {
				for _, ws := range workspaces {
					if wm, ok := ws.(map[string]any); ok {
						if u, _ := wm["workspaceFolderAbsoluteUri"].(string); u != "" {
							h.indexWorkspace(u, res.nodeID)
						}
					}
				}
			}

			if multiNode {
				curWS, _ := summary["workspaceName"].(string)
				summary["workspaceName"] = prefixWorkspaceWithNode(res.nodeName, curWS)
			}
			merged[cid] = summary
		}
	}

	respBytes, err := json.Marshal(map[string]any{
		"trajectorySummaries": merged,
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Connect-Protocol-Version", "1")
	w.Header().Set("Content-Length", strconv.Itoa(len(respBytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBytes)
}

func prefixWorkspaceWithNode(nodeName, wsName string) string {
	nodeName = strings.TrimSpace(nodeName)
	wsName = strings.TrimSpace(wsName)
	if nodeName == "" {
		return wsName
	}
	prefix := "[" + nodeName + "] "
	if strings.HasPrefix(wsName, prefix) {
		return wsName
	}
	if wsName == "" || strings.EqualFold(wsName, "Chat") {
		return prefix + "Chat"
	}
	return prefix + wsName
}

// =========================================================================
// Phase 1 Core Cascade-Scoped HTTP / ConnectRPC / WebSocket Routing
// =========================================================================

func (h *Hub) handleCascadeRoutedRequest(w http.ResponseWriter, r *http.Request, local http.Handler) {
	// 1. Check explicit node override (?node= or ?mgy_node=)
	if explicit := firstNonEmpty(r.URL.Query().Get("node"), r.URL.Query().Get("mgy_node")); explicit != "" {
		if strings.EqualFold(explicit, LocalNodeID) && !h.opts.Headless {
			local.ServeHTTP(w, r)
			return
		}
		if node, ok := h.store.GetNode(explicit); ok && node.Enabled {
			if isWebSocketUpgrade(r) {
				h.bridgeWSToNode(w, r, node, "")
			} else {
				h.forwardHTTPToNode(w, r, node, "", nil, nil, nil)
			}
			return
		}
	}

	// 2. Extract cascadeId from query parameters first (works for GET + WebSocket upgrades without touching body)
	cascadeID := extractCascadeIDFromQuery(r.URL.Query())

	// 3. For JSON POST/PUT/PATCH/DELETE (non-WS), peek body for cascadeId / conversationId / taskId
	var bodyBytes []byte
	var rawMap map[string]any
	if !isWebSocketUpgrade(r) && r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete) {
		var err error
		bodyBytes, err = io.ReadAll(io.LimitReader(r.Body, 50<<20))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		r.ContentLength = int64(len(bodyBytes))
		if len(bodyBytes) > 0 && json.Unmarshal(bodyBytes, &rawMap) == nil && rawMap != nil {
			if cascadeID == "" {
				cascadeID = extractCascadeIDFromMap(rawMap)
			}
		}
	}

	// 4. Resolve target node from cascadeIndex (or taskIndex for task stop)
	var targetNodeID string
	var found bool
	if cascadeID != "" {
		targetNodeID, found = h.lookupCascadeNode(cascadeID)
	}
	if !found && rawMap != nil {
		if tid, _ := rawMap["taskId"].(string); tid != "" {
			targetNodeID, found = h.lookupTaskNode(tid)
		}
	}

	// If cascadeId is still unknown and we have remote nodes, do a single quick trajectory index refresh
	if !found && cascadeID != "" && len(h.filteredRemoteNodes()) > 0 {
		h.refreshCascadeIndexQuick(r.Context())
		targetNodeID, found = h.lookupCascadeNode(cascadeID)
	}

	var remoteNode NodeConfig
	useRemote := false
	if found {
		if targetNodeID != LocalNodeID {
			if n, ok := h.store.GetNode(targetNodeID); ok && n.Enabled {
				remoteNode = n
				useRemote = true
			}
		}
	} else {
		// Unscoped gateway endpoint (e.g. /gateway/models, /gateway/slash-commands, /connect-websocket without cascadeId):
		// route locally when local LS is healthy, else fall back to default remote node (critical for headless Hub!).
		remoteNode, useRemote = h.pickDefaultTarget()
	}

	if !useRemote {
		local.ServeHTTP(w, r)
		return
	}

	// WebSocket bridge
	if isWebSocketUpgrade(r) {
		h.bridgeWSToNode(w, r, remoteNode, "")
		return
	}

	// Phase 2: Before forwarding SendUserCascadeMessage or /gateway/btw/start to a remote node,
	// lazily replicate any Hub-staged file attachments onto that remote node!
	if rawMap != nil && (isRPCMethod(r.URL.Path, "SendUserCascadeMessage") || r.URL.Path == "/gateway/btw/start") {
		if rewritten, changed, repErr := h.replicateAttachmentsForRemoteNode(r.Context(), remoteNode, rawMap); repErr != nil {
			slog.Warn("[Hub] Failed to replicate attachments to remote node", "node", remoteNode.ID, "err", repErr)
			writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("failed to transfer attachment to node %s: %v", remoteNode.Name, repErr))
			return
		} else if changed {
			bodyBytes = rewritten
		}
	}

	// Inspect responses for creation/fork/btw/delete so cascadeIndex & events stay immediately up to date.
	var inspect func(statusCode int, respBody []byte)
	switch {
	case isRPCMethod(r.URL.Path, "DeleteCascadeTrajectory"):
		inspect = func(statusCode int, _ []byte) {
			if statusCode >= 200 && statusCode < 300 && cascadeID != "" {
				h.removeCascade(cascadeID)
				h.BumpEvents()
			}
		}
	case r.URL.Path == "/gateway/cascade/fork" || r.URL.Path == "/gateway/btw/start":
		inspect = func(statusCode int, respBody []byte) {
			if statusCode >= 200 && statusCode < 300 {
				if newCID := extractNewCascadeIDFromResponse(respBody); newCID != "" {
					h.indexCascade(newCID, remoteNode.ID)
				}
				h.BumpEvents()
			}
		}
	case r.Method == http.MethodPost && !isReadOnlyHubRPC(r.URL.Path):
		inspect = func(statusCode int, _ []byte) {
			if statusCode >= 200 && statusCode < 300 {
				h.BumpEvents()
			}
		}
	}

	h.forwardHTTPToNode(w, r, remoteNode, "", nil, bodyBytes, inspect)
}

func (h *Hub) refreshCascadeIndexQuick(ctx context.Context) {
	nodes := h.store.ListNodes()
	var wg sync.WaitGroup
	for _, n := range nodes {
		if !n.Enabled {
			continue
		}
		wg.Add(1)
		go func(node NodeConfig) {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, h.probeTimeout)
			defer cancel()
			code, _, body, err := h.doNodeRequest(
				pctx, h.probeClient, node, http.MethodPost,
				"/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories",
				"", []byte("{}"), "application/json",
			)
			if err != nil || code != http.StatusOK {
				return
			}
			var env struct {
				TrajectorySummaries map[string]json.RawMessage `json:"trajectorySummaries"`
			}
			if json.Unmarshal(body, &env) == nil {
				for cid := range env.TrajectorySummaries {
					h.indexCascade(cid, node.ID)
				}
			}
		}(n)
	}
	wg.Wait()
}

func isReadOnlyHubRPC(path string) bool {
	for _, suffix := range []string{
		"/GetAllCascadeTrajectories", "/GetCascadeTrajectory", "/GetCascadeNuxes",
		"/GetAuthStatus", "/ReadProjects", "/SearchConversations",
		"/ListBackgroundTasks",
		"/gateway/status", "/gateway/projects", "/gateway/models",
		"/gateway/cascade/messages", "/gateway/cascade/changes", "/gateway/cascade/chapters",
		"/gateway/swarm", "/gateway/btw/list", "/gateway/btw/status",
	} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

func extractCascadeIDFromQuery(q url.Values) string {
	for _, key := range []string{"cascadeId", "cascade_id", "conversationId", "conversation_id", "parentCascadeId", "parentConversationId", "id"} {
		if v := strings.TrimSpace(q.Get(key)); v != "" {
			return v
		}
	}
	return ""
}

func extractCascadeIDFromMap(m map[string]any) string {
	for _, key := range []string{
		"cascadeId", "cascade_id", "conversationId", "conversation_id",
		"parentCascadeId", "parentConversationId", "rootCascadeId", "rootConversationId",
		"sideQuestionId", "sourceCascadeId",
	} {
		if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	// Peek into /gateway/rpc-mux batched calls if present
	if calls, ok := m["calls"].([]any); ok {
		for _, c := range calls {
			if cm, ok := c.(map[string]any); ok {
				if body, ok := cm["body"].(map[string]any); ok {
					if cid := extractCascadeIDFromMap(body); cid != "" {
						return cid
					}
				}
			}
		}
	}
	return ""
}

func extractNewCascadeIDFromResponse(body []byte) string {
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return ""
	}
	for _, key := range []string{"cascadeId", "cascade_id", "sideQuestionId", "conversationId", "newCascadeId"} {
		if v, ok := m[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func cloneLocalRequest(r *http.Request, body []byte) *http.Request {
	req := r.Clone(r.Context())
	req.Header = r.Header.Clone()
	req.Header.Del("Accept-Encoding")
	req.Header.Set(HeaderHubForwarded, "1")
	if body != nil {
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
	}
	return req
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
