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
	"net/url"
	"sort"
	"strings"
	"time"

	"antigravity-mobile/internal/auth"
)

// HubPairStartResult is returned to the Hub CLI after initiating a pairing prompt on a target LAN mgy host.
type HubPairStartResult struct {
	Status      string `json:"status"`
	ChallengeID string `json:"challenge_id"`
	BaseURL     string `json:"base_url"`
	NodeID      string `json:"node_id"`
	NodeName    string `json:"node_name"`
	ExpiresIn   int    `json:"expires_in"`
}

// HubPairConfirmInput is the request sent to POST /gateway/hub/pair/confirm on the Hub.
type HubPairConfirmInput struct {
	Target      string `json:"target,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
	ChallengeID string `json:"challenge_id,omitempty"`
	Code        string `json:"code"`
	Name        string `json:"name,omitempty"`
}

// HubPairError preserves upstream HTTP status and remaining attempt count for interactive CLI retry UX.
type HubPairError struct {
	StatusCode        int
	Message           string
	AttemptsRemaining int
}

func (e *HubPairError) Error() string {
	return e.Message
}

// DiscoverableNodes returns a sorted snapshot of freshly heard LAN mgy hosts not yet registered with this Hub.
func (h *Hub) DiscoverableNodes() []DiscoveredNode {
	h.discMu.RLock()
	var disc []DiscoveredNode
	cutoff := time.Now().Add(-2 * time.Minute)
	for _, d := range h.discovered {
		if d.LastSeen.Before(cutoff) {
			continue
		}
		if _, already := h.store.GetNode(d.NodeID); already {
			continue
		}
		disc = append(disc, d)
	}
	h.discMu.RUnlock()
	sort.Slice(disc, func(i, j int) bool {
		if disc[i].Name != disc[j].Name {
			return disc[i].Name < disc[j].Name
		}
		return disc[i].BaseURL < disc[j].BaseURL
	})
	return disc
}

// ScanLAN broadcasts two closely-spaced UDP probes and waits up to `wait` for LAN mgy hosts to reply.
// Used both on `mgy hub` startup and on explicit `mgy hub scan`.
func (h *Hub) ScanLAN(ctx context.Context, wait time.Duration) []DiscoveredNode {
	if h == nil {
		return nil
	}
	if h.opts.DisableDiscovery || wait <= 0 {
		return h.DiscoverableNodes()
	}
	h.triggerDiscoveryProbe()

	secondDelay := wait / 3
	if secondDelay < 150*time.Millisecond {
		secondDelay = 150 * time.Millisecond
	}
	if secondDelay < wait {
		t1 := time.NewTimer(secondDelay)
		select {
		case <-ctx.Done():
			t1.Stop()
			return h.DiscoverableNodes()
		case <-t1.C:
			h.triggerDiscoveryProbe()
		}
		rem := wait - secondDelay
		t2 := time.NewTimer(rem)
		select {
		case <-ctx.Done():
			t2.Stop()
		case <-t2.C:
		}
	} else {
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
		case <-t.C:
		}
	}
	return h.DiscoverableNodes()
}

// ResolvePairTarget resolves a user-supplied target (discovered node Name/ID, bare LAN IP, host:port, or http URL)
// into a normalized private-LAN or loopback BaseURL.
func (h *Hub) ResolvePairTarget(target string) (baseURL, name, nodeID string, err error) {
	raw := strings.TrimSpace(target)
	if raw == "" {
		return "", "", "", fmt.Errorf("target node name, ID, or LAN IP/URL is required")
	}

	// 1. Check freshly discovered LAN devices by NodeID, Name, BaseURL, or IP.
	h.discMu.RLock()
	cutoff := time.Now().Add(-5 * time.Minute)
	for _, d := range h.discovered {
		if d.LastSeen.Before(cutoff) {
			continue
		}
		if strings.EqualFold(d.NodeID, raw) || strings.EqualFold(d.Name, raw) || strings.EqualFold(d.BaseURL, NormalizeBaseURL(raw)) {
			baseURL, name, nodeID = d.BaseURL, d.Name, d.NodeID
			break
		}
		if u, uErr := url.Parse(d.BaseURL); uErr == nil && strings.EqualFold(u.Hostname(), raw) {
			baseURL, name, nodeID = d.BaseURL, d.Name, d.NodeID
			break
		}
	}
	h.discMu.RUnlock()

	if baseURL == "" {
		candidate := raw
		if !strings.Contains(candidate, "://") {
			// Accept bare IP ("192.168.50.99") or IP:port ("192.168.50.99:58910").
			if _, _, splitErr := net.SplitHostPort(candidate); splitErr != nil {
				if ip := net.ParseIP(candidate); ip != nil || strings.EqualFold(candidate, "localhost") {
					candidate = fmt.Sprintf("%s:58900", candidate)
				} else if strings.HasSuffix(strings.ToLower(candidate), ".local") {
					candidate = fmt.Sprintf("%s:58900", candidate)
				} else {
					return "", "", "", fmt.Errorf("could not find discovered LAN device %q (run `mgy hub scan` or pass http://<LAN-IP>:58900)", raw)
				}
			}
			candidate = "http://" + candidate
		}
		baseURL = NormalizeBaseURL(candidate)
	}

	if baseURL == "" {
		return "", "", "", fmt.Errorf("invalid target address: %q", raw)
	}
	if err := validateLANTargetURL(baseURL); err != nil {
		return "", "", "", err
	}
	return baseURL, name, nodeID, nil
}

func validateLANTargetURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return fmt.Errorf("invalid target url: %q", rawURL)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q (expected http or https)", u.Scheme)
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".local") {
		return nil
	}
	if auth.IsLoopbackAddr(host) || auth.IsPrivateLANAddr(host) {
		return nil
	}
	return fmt.Errorf("hub pairing target %q must be a private LAN or loopback address", host)
}

// StartPairWithTarget sends POST /api/v1/hub/pair-request to a target LAN mgy host,
// triggering a 6-char confirmation code prompt on the target host's terminal.
func (h *Hub) StartPairWithTarget(ctx context.Context, target string) (*HubPairStartResult, error) {
	baseURL, discName, _, err := h.ResolvePairTarget(target)
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(HubPairRequestPayload{
		HubID:   h.hubID,
		HubName: h.hubName,
		HubURL:  NormalizeBaseURL(h.opts.AdvertiseURL),
	})
	if err != nil {
		return nil, err
	}

	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(pctx, http.MethodPost, baseURL+"/api/v1/hub/pair-request", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.rpcClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return nil, parseUpstreamPairError(resp.StatusCode, body)
	}

	var parsed HubPairRequestResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("invalid pair-request response from %s: %w", baseURL, err)
	}
	nodeName := strings.TrimSpace(parsed.NodeName)
	if nodeName == "" {
		nodeName = discName
	}
	return &HubPairStartResult{
		Status:      "pending",
		ChallengeID: parsed.ChallengeID,
		BaseURL:     baseURL,
		NodeID:      parsed.NodeID,
		NodeName:    nodeName,
		ExpiresIn:   parsed.ExpiresIn,
	}, nil
}

// ConfirmPairWithTarget submits the 6-char confirmation code to POST /api/v1/hub/pair-confirm on the target host,
// saves the resulting nodeToken into the Hub store, and probes the newly paired node immediately.
func (h *Hub) ConfirmPairWithTarget(ctx context.Context, in HubPairConfirmInput) (NodeConfig, error) {
	target := strings.TrimSpace(in.BaseURL)
	if target == "" {
		target = strings.TrimSpace(in.Target)
	}
	baseURL, discName, _, err := h.ResolvePairTarget(target)
	if err != nil {
		return NodeConfig{}, err
	}
	code := NormalizePairCode(in.Code)
	if code == "" {
		return NodeConfig{}, &HubPairError{StatusCode: http.StatusBadRequest, Message: "confirmation code is required"}
	}

	payload, err := json.Marshal(HubPairConfirmPayload{
		ChallengeID: strings.TrimSpace(in.ChallengeID),
		Code:        code,
		HubID:       h.hubID,
		HubName:     h.hubName,
		HubURL:      NormalizeBaseURL(h.opts.AdvertiseURL),
	})
	if err != nil {
		return NodeConfig{}, err
	}

	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(pctx, http.MethodPost, baseURL+"/api/v1/hub/pair-confirm", bytes.NewReader(payload))
	if err != nil {
		return NodeConfig{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.rpcClient.Do(req)
	if err != nil {
		return NodeConfig{}, fmt.Errorf("failed to reach %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return NodeConfig{}, parseUpstreamPairError(resp.StatusCode, body)
	}

	var parsed HubPairConfirmResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return NodeConfig{}, fmt.Errorf("invalid pair-confirm response from %s: %w", baseURL, err)
	}
	if strings.TrimSpace(parsed.NodeToken) == "" {
		return NodeConfig{}, fmt.Errorf("target node did not return an auth token")
	}

	displayName := strings.TrimSpace(in.Name)
	if displayName == "" {
		displayName = strings.TrimSpace(parsed.NodeName)
	}
	if displayName == "" {
		displayName = discName
	}

	cfg, err := h.store.UpsertNode(NodeConfig{
		ID:      parsed.NodeID,
		Name:    displayName,
		BaseURL: baseURL,
		Token:   parsed.NodeToken,
		OS:      strings.TrimSpace(parsed.OS),
		Version: strings.TrimSpace(parsed.Version),
		Static:  false,
		Enabled: true,
	})
	if err != nil {
		return NodeConfig{}, err
	}

	slog.Info("[Hub] Paired LAN mgy host via confirmation code", "node_id", cfg.ID, "name", cfg.Name, "url", cfg.BaseURL)
	h.probeNodeAndRefresh(cfg)
	return cfg, nil
}

func parseUpstreamPairError(statusCode int, body []byte) *HubPairError {
	var parsed struct {
		Error             string `json:"error"`
		AttemptsRemaining int    `json:"attempts_remaining"`
	}
	msg := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &parsed) == nil && parsed.Error != "" {
		msg = parsed.Error
	}
	if msg == "" {
		msg = fmt.Sprintf("HTTP %d", statusCode)
	}
	return &HubPairError{
		StatusCode:        statusCode,
		Message:           msg,
		AttemptsRemaining: parsed.AttemptsRemaining,
	}
}

// verifyAndMigrateNodeIP verifies that a registered node announcing from a new LAN IP genuinely
// possesses its paired nodeToken via HMAC-SHA256 challenge-response before updating BaseURL.
// This prevents a rogue LAN peer spoofing a node_id from tricking the Hub into leaking nodeToken.
func (h *Hub) verifyAndMigrateNodeIP(existing NodeConfig, newBaseURL, newName string) bool {
	newBaseURL = NormalizeBaseURL(newBaseURL)
	if existing.ID == "" || existing.Token == "" || newBaseURL == "" || existing.BaseURL == newBaseURL {
		return false
	}
	if err := validateLANTargetURL(newBaseURL); err != nil {
		return false
	}

	cooldownKey := existing.ID + "|" + newBaseURL
	now := time.Now()
	h.verifyMu.Lock()
	if h.verifyCooldown == nil {
		h.verifyCooldown = make(map[string]time.Time)
	}
	if last, ok := h.verifyCooldown[cooldownKey]; ok && now.Sub(last) < 10*time.Second {
		h.verifyMu.Unlock()
		return false
	}
	h.verifyCooldown[cooldownKey] = now
	h.verifyMu.Unlock()

	nonce := randomHex(16)
	payload, err := json.Marshal(HubVerifyRequest{
		HubID: h.hubID,
		Nonce: nonce,
	})
	if err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(h.rootCtx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, newBaseURL+"/api/v1/hub/verify", bytes.NewReader(payload))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.probeClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	if resp.StatusCode != http.StatusOK {
		slog.Warn("[Hub] Rejected unverified IP change for registered node (HTTP status mismatch)",
			"node_id", existing.ID, "old_url", existing.BaseURL, "new_url", newBaseURL, "status", resp.StatusCode)
		return false
	}

	var vResp HubVerifyResponse
	if err := json.Unmarshal(body, &vResp); err != nil {
		return false
	}
	expected := ComputeVerifyProof(existing.Token, h.hubID, existing.ID, nonce)
	if vResp.NodeID != existing.ID || !ConstantTimeEquals(vResp.Proof, expected) {
		slog.Warn("[Hub] Rejected unverified IP change for registered node (HMAC mismatch — possible LAN spoofing)",
			"node_id", existing.ID, "old_url", existing.BaseURL, "new_url", newBaseURL)
		return false
	}

	updated := existing
	updated.BaseURL = newBaseURL
	if strings.TrimSpace(updated.Name) == "" && strings.TrimSpace(newName) != "" {
		updated.Name = strings.TrimSpace(newName)
	}
	saved, err := h.store.UpsertNode(updated)
	if err != nil {
		slog.Warn("[Hub] Failed to save verified IP update for node", "node_id", existing.ID, "err", err)
		return false
	}
	slog.Info("[Hub] Verified & updated LAN IP for paired node",
		"node_id", saved.ID, "name", saved.Name, "old_url", existing.BaseURL, "new_url", saved.BaseURL)
	h.probeNodeAndRefresh(saved)
	return true
}

func (h *Hub) handleHubPairStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !isLocalAdminCaller(r) {
		writeJSONError(w, http.StatusForbidden, "forbidden: local admin access required")
		return
	}
	var req struct {
		Target  string `json:"target"`
		BaseURL string `json:"base_url"`
		URL     string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16*1024)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	target := strings.TrimSpace(req.Target)
	if target == "" {
		target = strings.TrimSpace(req.BaseURL)
	}
	if target == "" {
		target = strings.TrimSpace(req.URL)
	}
	res, err := h.StartPairWithTarget(r.Context(), target)
	if err != nil {
		if pe, ok := err.(*HubPairError); ok {
			writeJSON(w, pe.StatusCode, map[string]any{"error": pe.Message})
			return
		}
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Hub) handleHubPairConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !isLocalAdminCaller(r) {
		writeJSONError(w, http.StatusForbidden, "forbidden: local admin access required")
		return
	}
	var req HubPairConfirmInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 16*1024)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	cfg, err := h.ConfirmPairWithTarget(r.Context(), req)
	if err != nil {
		if pe, ok := err.(*HubPairError); ok {
			writeJSON(w, pe.StatusCode, map[string]any{
				"error":              pe.Message,
				"attempts_remaining": pe.AttemptsRemaining,
			})
			return
		}
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	// Never echo raw node token back to CLI unless needed; redact token from response view.
	safeCfg := cfg
	safeCfg.Token = ""
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"node":   safeCfg,
	})
}
