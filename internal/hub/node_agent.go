package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/notifier"
	"antigravity-mobile/internal/proxy"
)

// NodeAgentOptions configures a LAN mgy gateway (standalone or dedicated `mgy node`) participating in Hub discovery/pairing.
type NodeAgentOptions struct {
	DataDir           string
	NodeName          string
	HTTPPort          int
	AdvertiseBaseURL  string // optional explicit LAN URL (e.g. "http://192.168.50.12:58900")
	Version           string
	HubURL            string // optional static Hub URL; if empty in node mode, auto-discovers via UDP
	HubSecret         string
	JoinCode          string
	DiscoveryPort     int
	DisableDiscovery  bool
	ListenLoopback    bool // when true in PassiveStandalone mode, skip LAN UDP discovery
	PassiveStandalone bool // true for ordinary `mgy`: passive discovery only, requires secondary pair code
	IsLSHealthy       func() bool
	Notifier          *notifier.Notifier
	OnRelayActivated  func()                                   // optional hook invoked when Hub push relay is hot-enabled
	OnPairPrompt      func(hubName, requesterIP, code string)  // optional override for terminal prompt (useful in tests)
}

// NodeAgent runs on a LAN mgy gateway: it authenticates incoming Hub requests via dedicated node tokens,
// responds to LAN discovery probes, supports secondary confirmation-code pairing from Hubs, and relays
// local push/Live Activity events to authorized Hubs.
type NodeAgent struct {
	opts   NodeAgentOptions
	client *http.Client

	mu           sync.RWMutex
	creds        NodeCredentials
	hubURL       string
	nodeToken    string
	hubName      string
	joined       bool
	links        map[string]HubLink
	relayEnabled bool

	pairMu      sync.Mutex
	challenges  map[string]*hubPairChallenge
	pairLimiter *slidingRateLimiter
}

// NewNodeAgent initializes a NodeAgent, loading persistent node identity and any previously authorized Hub links.
func NewNodeAgent(opts NodeAgentOptions) (*NodeAgent, error) {
	creds, err := LoadOrCreateNodeCredentials(opts.DataDir, opts.NodeName)
	if err != nil {
		return nil, err
	}
	changed := false
	if name := strings.TrimSpace(opts.NodeName); name != "" && creds.NodeName != name {
		creds.NodeName = name
		changed = true
	}
	if !opts.PassiveStandalone {
		if hubURL := NormalizeBaseURL(opts.HubURL); hubURL != "" && creds.HubURL != hubURL {
			creds.HubURL = hubURL
			changed = true
		}
		if sec := strings.TrimSpace(opts.HubSecret); sec != "" {
			if creds.HubSecret != sec {
				creds.HubSecret = sec
				changed = true
			}
			derived := DeriveNodeToken(sec, creds.NodeID)
			if creds.NodeToken == "" || creds.NodeToken != derived {
				creds.NodeToken = derived
				changed = true
			}
		}
	}
	if changed {
		_ = SaveNodeCredentials(opts.DataDir, creds)
	}

	persistedLinks, err := LoadHubLinks(opts.DataDir)
	if err != nil {
		slog.Warn("[HubNode] Failed to load hub_links.json (starting with empty links)", "err", err)
	}
	linkMap := make(map[string]HubLink, len(persistedLinks))
	for _, lk := range persistedLinks {
		linkMap[lk.HubID] = lk
	}

	a := &NodeAgent{
		opts:        opts,
		client:      &http.Client{Transport: newNodeTransport(), Timeout: 4 * time.Second},
		creds:       creds,
		links:       linkMap,
		challenges:  make(map[string]*hubPairChallenge),
		pairLimiter: newSlidingRateLimiter(),
	}
	if !opts.PassiveStandalone {
		a.hubURL = creds.HubURL
		a.nodeToken = creds.NodeToken
	}

	// Hook Notifier immediately for dedicated `mgy node` mode, or for standalone `mgy` that already has linked Hubs.
	// Standalone hosts with zero Hub links hot-attach EnableHubRelay only once a Hub is authorized.
	if !opts.PassiveStandalone || len(linkMap) > 0 {
		a.ensureHubRelayEnabled()
	}
	return a, nil
}

func (a *NodeAgent) ensureHubRelayEnabled() {
	a.mu.Lock()
	if a.relayEnabled || a.opts.Notifier == nil {
		a.mu.Unlock()
		return
	}
	a.relayEnabled = true
	notif := a.opts.Notifier
	onActivated := a.opts.OnRelayActivated
	a.mu.Unlock()

	notif.EnableHubRelay(
		&nodePushRelaySender{agent: a},
		func(details *proxy.TrajectoryDetails) {
			a.forwardLiveActivity(details)
		},
		func(cascadeID, title, state string, totalSteps int) {
			a.forwardLiveActivityEnd(cascadeID, title, state, totalSteps)
		},
	)
	if onActivated != nil {
		onActivated()
	}
}

// NodeID returns this node's stable cluster identifier.
func (a *NodeAgent) NodeID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.creds.NodeID
}

// NodeName returns this node's display name.
func (a *NodeAgent) NodeName() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.creds.NodeName
}

// HubURL returns the currently configured or discovered Hub URL (used in `mgy node` mode).
func (a *NodeAgent) HubURL() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.hubURL
}

// ValidateHubRequestToken checks whether an incoming token from a Hub matches either
// this node's dedicated node token (`mgy node`) or any authorized Hub link (`hub_links.json`).
func (a *NodeAgent) ValidateHubRequestToken(token string) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", false
	}
	a.mu.RLock()
	want := a.nodeToken
	nodeID := a.creds.NodeID
	for _, lk := range a.links {
		if lk.NodeToken != "" && ConstantTimeEquals(token, lk.NodeToken) {
			hubID := lk.HubID
			a.mu.RUnlock()
			return "hub-controller:" + hubID, true
		}
	}
	a.mu.RUnlock()
	if want != "" && ConstantTimeEquals(token, want) {
		return "hub-controller:" + nodeID, true
	}
	return "", false
}

// Start begins LAN discovery responder and (for `mgy node` mode) Hub registration/heartbeat loop.
func (a *NodeAgent) Start(ctx context.Context) {
	if !a.opts.DisableDiscovery && !(a.opts.PassiveStandalone && a.opts.ListenLoopback) {
		modeStr := "node"
		var onHubFound func(hubURL, hubName string)
		clusterSecret := a.opts.HubSecret
		if a.opts.PassiveStandalone {
			modeStr = "standalone"
			clusterSecret = "" // standalone hosts require explicit secondary confirmation code; never auto-join
		} else {
			onHubFound = func(hubURL, hubName string) {
				if hubURL == "" {
					return
				}
				a.mu.Lock()
				needRegister := a.hubURL == "" || !a.joined
				if a.hubURL == "" {
					a.hubURL = hubURL
				}
				a.mu.Unlock()
				if needRegister {
					go func() {
						if err := a.RegisterWithHub(ctx, hubURL); err != nil {
							slog.Debug("[HubNode] Auto-register with discovered Hub deferred", "hub", hubURL, "err", err)
						}
					}()
				}
			}
		}

		StartNodeDiscoveryResponder(ctx, NodeAnnouncerConfig{
			NodeID:        a.NodeID(),
			NodeName:      a.NodeName(),
			Mode:          modeStr,
			HTTPPort:      a.opts.HTTPPort,
			Version:       a.opts.Version,
			DiscoveryPort: a.opts.DiscoveryPort,
			ClusterSecret: clusterSecret,
			PassiveOnly:   a.opts.PassiveStandalone,
			IsLSHealthy:   a.opts.IsLSHealthy,
			OnHubFound:    onHubFound,
		})
	}

	if a.opts.PassiveStandalone {
		return
	}

	// If an explicit or persisted Hub URL is configured in `mgy node` mode, register immediately and retry until joined.
	go func() {
		tryJoin := func() {
			a.mu.RLock()
			target := a.hubURL
			joined := a.joined
			a.mu.RUnlock()
			if target == "" || joined {
				return
			}
			if err := a.RegisterWithHub(ctx, target); err != nil {
				slog.Warn("[HubNode] Failed to register with Hub (will retry)", "hub", target, "err", err)
			}
		}

		tryJoin()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tryJoin()
			}
		}
	}()
}

// RegisterWithHub registers this node with the specified Hub URL using HubSecret or JoinCode (`mgy node` mode).
func (a *NodeAgent) RegisterWithHub(ctx context.Context, hubURL string) error {
	hubURL = NormalizeBaseURL(hubURL)
	if hubURL == "" {
		return fmt.Errorf("hub url is required")
	}

	a.mu.RLock()
	creds := a.creds
	secret := strings.TrimSpace(a.opts.HubSecret)
	if secret == "" {
		secret = creds.HubSecret
	}
	code := strings.TrimSpace(a.opts.JoinCode)
	a.mu.RUnlock()

	if secret == "" && code == "" {
		return fmt.Errorf("hub secret or join code is required to register")
	}

	reqPayload := RegisterNodeRequest{
		NodeID:    creds.NodeID,
		Name:      creds.NodeName,
		BaseURL:   NormalizeBaseURL(a.opts.AdvertiseBaseURL),
		Port:      a.opts.HTTPPort,
		OS:        runtime.GOOS,
		Version:   a.opts.Version,
		Secret:    secret,
		JoinCode:  code,
		NodeToken: creds.NodeToken,
	}
	raw, err := json.Marshal(reqPayload)
	if err != nil {
		return err
	}

	rctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, hubURL+"/api/v1/hub/register", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hub register HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed RegisterNodeResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return err
	}

	a.mu.Lock()
	a.hubURL = hubURL
	a.hubName = parsed.HubName
	if parsed.NodeToken != "" {
		a.nodeToken = parsed.NodeToken
		a.creds.NodeToken = parsed.NodeToken
	}
	if parsed.NodeID != "" {
		a.creds.NodeID = parsed.NodeID
	}
	a.creds.HubURL = hubURL
	a.joined = true
	credsCopy := a.creds
	a.mu.Unlock()

	_ = SaveNodeCredentials(a.opts.DataDir, credsCopy)
	slog.Info("[HubNode] Connected to Multigravity Hub", "hub", hubURL, "hub_name", parsed.HubName, "node_id", credsCopy.NodeID)
	return nil
}

type hubEventTarget struct {
	hubURL string
	token  string
}

func (a *NodeAgent) eventTargets() (nodeID, nodeName string, targets []hubEventTarget) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	nodeID = a.creds.NodeID
	nodeName = a.creds.NodeName
	seen := make(map[string]bool)
	add := func(u, tok string) {
		u = NormalizeBaseURL(u)
		tok = strings.TrimSpace(tok)
		if u == "" || tok == "" {
			return
		}
		key := u + "|" + tok
		if seen[key] {
			return
		}
		seen[key] = true
		targets = append(targets, hubEventTarget{hubURL: u, token: tok})
	}
	add(a.hubURL, a.nodeToken)
	for _, lk := range a.links {
		add(lk.HubURL, lk.NodeToken)
	}
	return nodeID, nodeName, targets
}

func (a *NodeAgent) postEvent(ctx context.Context, ev NodeEventPayload) error {
	nodeID, nodeName, targets := a.eventTargets()
	if len(targets) == 0 {
		return nil
	}
	ev.NodeID = nodeID
	ev.NodeName = nodeName

	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}

	var firstErr error
	for _, tgt := range targets {
		pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		req, err := http.NewRequestWithContext(pctx, http.MethodPost, tgt.hubURL+"/api/v1/hub/internal/event", bytes.NewReader(body))
		if err != nil {
			cancel()
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(HeaderHubToken, tgt.token)
		req.Header.Set("Authorization", "Bearer "+tgt.token)

		resp, err := a.client.Do(req)
		cancel()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if (resp.StatusCode < 200 || resp.StatusCode >= 300) && firstErr == nil {
			firstErr = fmt.Errorf("hub event relay HTTP %d", resp.StatusCode)
		}
	}
	return firstErr
}

func (a *NodeAgent) forwardLiveActivity(details *proxy.TrajectoryDetails) {
	if details == nil || details.CascadeID == "" {
		return
	}
	// Strip heavy step & message history so LAN Live Activity relays stay tiny (<4KB).
	trimmed := *details
	trimmed.Steps = nil
	trimmed.AllMessages = nil
	trimmed.QueuedMessages = nil
	go func() {
		_ = a.postEvent(context.Background(), NodeEventPayload{
			Type:      "live_activity",
			CascadeID: trimmed.CascadeID,
			Details:   &trimmed,
		})
	}()
}

func (a *NodeAgent) forwardLiveActivityEnd(cascadeID, title, state string, totalSteps int) {
	if cascadeID == "" {
		return
	}
	go func() {
		_ = a.postEvent(context.Background(), NodeEventPayload{
			Type:       "live_activity_end",
			CascadeID:  cascadeID,
			Title:      title,
			EndState:   state,
			TotalSteps: totalSteps,
		})
	}()
}

type nodePushRelaySender struct {
	agent *NodeAgent
}

func (s *nodePushRelaySender) Send(ctx context.Context, payload notifier.PushPayload) error {
	ev := NodeEventPayload{
		Type:      "push",
		CascadeID: payload.CascadeID,
		Title:     payload.Title,
		Body:      payload.Body,
		URL:       payload.URL,
		Level:     payload.Level,
		Sound:     payload.Sound,
		Category:  payload.Category,
		Silent:    payload.Silent,
		Action:    payload.Action,
	}
	if payload.Localize != nil {
		ev.Localized = make(map[string][2]string, 3)
		for _, lang := range []string{"zh", "en", "ja"} {
			t, b := payload.Localize(lang)
			ev.Localized[lang] = [2]string{t, b}
		}
	}
	return s.agent.postEvent(ctx, ev)
}
