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

// NodeAgentOptions configures a LAN mgy node participating in a Multigravity Hub cluster.
type NodeAgentOptions struct {
	DataDir          string
	NodeName         string
	HTTPPort         int
	AdvertiseBaseURL string // optional explicit LAN URL (e.g. "http://192.168.50.12:58900")
	Version          string
	HubURL           string // optional static Hub URL; if empty, auto-discovers via UDP
	HubSecret        string
	JoinCode         string
	DiscoveryPort    int
	DisableDiscovery bool
	IsLSHealthy      func() bool
	Notifier         *notifier.Notifier
}

// NodeAgent runs on a LAN mgy node: it authenticates incoming Hub requests via a dedicated node token,
// advertises itself on the LAN, registers with a Hub, and relays local push/Live Activity events to the Hub.
type NodeAgent struct {
	opts   NodeAgentOptions
	client *http.Client

	mu        sync.RWMutex
	creds     NodeCredentials
	hubURL    string
	nodeToken string
	hubName   string
	joined    bool
}

// NewNodeAgent initializes a NodeAgent, loading or creating persistent node credentials.
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
	if changed {
		_ = SaveNodeCredentials(opts.DataDir, creds)
	}

	a := &NodeAgent{
		opts:      opts,
		client:    &http.Client{Transport: newNodeTransport(), Timeout: 4 * time.Second},
		creds:     creds,
		hubURL:    creds.HubURL,
		nodeToken: creds.NodeToken,
	}

	// Hook node's Notifier so alerts & Live Activities relay transparently to the Hub.
	if opts.Notifier != nil {
		opts.Notifier.EnableHubRelay(
			&nodePushRelaySender{agent: a},
			func(details *proxy.TrajectoryDetails) {
				a.forwardLiveActivity(details)
			},
			func(cascadeID, title, state string, totalSteps int) {
				a.forwardLiveActivityEnd(cascadeID, title, state, totalSteps)
			},
		)
	}
	return a, nil
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

// HubURL returns the currently configured or discovered Hub URL.
func (a *NodeAgent) HubURL() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.hubURL
}

// ValidateHubRequestToken checks whether an incoming token from a Hub matches this node's token.
func (a *NodeAgent) ValidateHubRequestToken(token string) (string, bool) {
	a.mu.RLock()
	want := a.nodeToken
	nodeID := a.creds.NodeID
	a.mu.RUnlock()
	if want == "" || !ConstantTimeEquals(strings.TrimSpace(token), want) {
		return "", false
	}
	return "hub-controller:" + nodeID, true
}

// Start begins LAN discovery announcements and Hub registration/heartbeat loop.
func (a *NodeAgent) Start(ctx context.Context) {
	if !a.opts.DisableDiscovery {
		StartNodeDiscoveryResponder(ctx, NodeAnnouncerConfig{
			NodeID:        a.NodeID(),
			NodeName:      a.NodeName(),
			HTTPPort:      a.opts.HTTPPort,
			Version:       a.opts.Version,
			DiscoveryPort: a.opts.DiscoveryPort,
			ClusterSecret: a.opts.HubSecret,
			IsLSHealthy:   a.opts.IsLSHealthy,
			OnHubFound: func(hubURL, hubName string) {
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
			},
		})
	}

	// If an explicit or persisted Hub URL is configured, register immediately and retry periodically until joined.
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

// RegisterWithHub registers this node with the specified Hub URL using HubSecret or JoinCode.
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

func (a *NodeAgent) postEvent(ctx context.Context, ev NodeEventPayload) error {
	a.mu.RLock()
	hubURL := a.hubURL
	token := a.nodeToken
	nodeID := a.creds.NodeID
	nodeName := a.creds.NodeName
	a.mu.RUnlock()

	if hubURL == "" || token == "" {
		return nil
	}
	ev.NodeID = nodeID
	ev.NodeName = nodeName

	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(pctx, http.MethodPost, hubURL+"/api/v1/hub/internal/event", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderHubToken, token)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("hub event relay HTTP %d", resp.StatusCode)
	}
	return nil
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
