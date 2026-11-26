package hub

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/auth"
	"antigravity-mobile/internal/e2ee"
	"antigravity-mobile/internal/i18n"
)

const (
	hubLinksFilename       = "hub_links.json"
	hubPairCodeTTL         = 2 * time.Minute
	hubPairMaxAttempts     = 3
	hubPairMaxPending      = 16
	hubPairReqPerIPPerMin  = 2
	hubPairReqGlobalPerMin = 6
	hubPairConfirmPerMin   = 10
	hubVerifyPerIPPerMin   = 30
)

type persistedHubLinks struct {
	Links []HubLink `json:"links"`
}

type hubPairChallenge struct {
	ChallengeID    string
	Code           string
	HubID          string
	HubName        string
	HubURL         string
	RequesterIP    string
	LocalPreIssued bool
	ExpiresAt      time.Time
	Attempts       int
}

type slidingRateLimiter struct {
	mu      sync.Mutex
	buckets map[string][]time.Time
}

func newSlidingRateLimiter() *slidingRateLimiter {
	return &slidingRateLimiter{
		buckets: make(map[string][]time.Time),
	}
}

func (l *slidingRateLimiter) Allow(key string, limit int, window time.Duration) bool {
	if limit <= 0 {
		return true
	}
	now := time.Now()
	cutoff := now.Add(-window)

	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.buckets) > 256 {
		for k, ts := range l.buckets {
			if len(ts) == 0 || ts[len(ts)-1].Before(cutoff) {
				delete(l.buckets, k)
			}
		}
	}

	ts := l.buckets[key]
	kept := ts[:0]
	for _, t := range ts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		l.buckets[key] = kept
		return false
	}
	kept = append(kept, now)
	l.buckets[key] = kept
	return true
}

// FormatPairCode formats a 6-char confirmation code as "XXX XXX" for terminal readability.
func FormatPairCode(code string) string {
	norm := NormalizePairCode(code)
	if len(norm) == 6 {
		return norm[:3] + " " + norm[3:]
	}
	return norm
}

// NormalizePairCode trims spaces/hyphens and uppercases a Hub pairing confirmation code.
func NormalizePairCode(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	return strings.ToUpper(s)
}

// ComputeVerifyProof computes the HMAC-SHA256 challenge-response proof used to verify
// a paired node at a new LAN IP without ever transmitting nodeToken over the wire.
func ComputeVerifyProof(nodeToken, hubID, nodeID, nonce string) string {
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(nodeToken)))
	_, _ = mac.Write([]byte("mgy-hub-verify-v1:"))
	_, _ = mac.Write([]byte(strings.TrimSpace(hubID)))
	_, _ = mac.Write([]byte(":"))
	_, _ = mac.Write([]byte(strings.TrimSpace(nodeID)))
	_, _ = mac.Write([]byte(":"))
	_, _ = mac.Write([]byte(strings.TrimSpace(nonce)))
	return hex.EncodeToString(mac.Sum(nil))
}

func resolveHubLinksPath(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataDir = filepath.Join(home, ".multigravity")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dataDir, hubLinksFilename), nil
}

// LoadHubLinks reads persisted Hub links from <dataDir>/hub_links.json.
func LoadHubLinks(dataDir string) ([]HubLink, error) {
	path, err := resolveHubLinksPath(dataDir)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var doc persistedHubLinks
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := make([]HubLink, 0, len(doc.Links))
	seen := make(map[string]bool)
	for _, lk := range doc.Links {
		lk.HubID = strings.TrimSpace(lk.HubID)
		lk.NodeToken = strings.TrimSpace(lk.NodeToken)
		lk.HubURL = NormalizeBaseURL(lk.HubURL)
		if lk.HubID == "" || lk.NodeToken == "" || seen[lk.HubID] {
			continue
		}
		if strings.TrimSpace(lk.HubName) == "" {
			lk.HubName = lk.HubID
		}
		seen[lk.HubID] = true
		out = append(out, lk)
	}
	return out, nil
}

// SaveHubLinks atomically persists authorized Hub links with 0600 permissions.
func SaveHubLinks(dataDir string, links []HubLink) error {
	path, err := resolveHubLinksPath(dataDir)
	if err != nil {
		return err
	}
	doc := persistedHubLinks{Links: links}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// RemoveHubLinkOffline deletes a Hub link directly from hub_links.json when the gateway is not running.
func RemoveHubLinkOffline(dataDir, idOrName string) (HubLink, bool, error) {
	links, err := LoadHubLinks(dataDir)
	if err != nil {
		return HubLink{}, false, err
	}
	key := strings.TrimSpace(idOrName)
	var removed HubLink
	kept := make([]HubLink, 0, len(links))
	found := false
	for _, lk := range links {
		if !found && (strings.EqualFold(lk.HubID, key) || strings.EqualFold(lk.HubName, key)) {
			removed = lk
			found = true
			continue
		}
		kept = append(kept, lk)
	}
	if !found {
		return HubLink{}, false, nil
	}
	if err := SaveHubLinks(dataDir, kept); err != nil {
		return HubLink{}, false, err
	}
	return removed, true, nil
}

// isDirectLANOrLoopback enforces that Hub secondary-code pairing and IP-verify endpoints
// are strictly reachable from direct private LAN or loopback callers, NEVER via public
// internet, reverse proxies, Cloudflare Tunnel, or E2EE relay transport.
func isDirectLANOrLoopback(r *http.Request) bool {
	if r == nil {
		return false
	}
	if e2ee.IsRelayTransport(r.Context()) || auth.IsProxiedRequest(r) {
		return false
	}
	return auth.IsLoopbackAddr(r.RemoteAddr) || auth.IsPrivateLANAddr(r.RemoteAddr)
}

func isLocalAdminCaller(r *http.Request) bool {
	if r == nil || e2ee.IsRelayTransport(r.Context()) || auth.IsProxiedRequest(r) {
		return false
	}
	if auth.IsLoopbackAddr(r.RemoteAddr) {
		return true
	}
	if adminTok := auth.GetAdminToken(); adminTok != "" {
		if tok := auth.BearerToken(r); tok != "" && auth.ConstantTimeTokenEquals(tok, adminTok) {
			return true
		}
	}
	return false
}

func defaultPrintHubPairPrompt(hubName, requesterIP, formattedCode string) {
	fmt.Println()
	fmt.Println("  ==================================================================")
	fmt.Print(i18n.Sprintf(
		"    🔐 局域网 Hub 请求接入本机\n    Hub 名称: %s (%s)\n    请在 Hub 端输入确认码:  %s   (2 分钟内有效，限试 3 次)\n",
		"    🔐 LAN Hub Pairing Request\n    Hub Name: %s (%s)\n    Enter confirmation code on Hub:  %s   (Valid 2 mins, max 3 tries)\n",
		hubName, requesterIP, formattedCode,
	))
	fmt.Println("  ==================================================================")
	fmt.Println()
}

func defaultPrintHubPairApproved(hubName, hubID string) {
	fmt.Print(i18n.Sprintf(
		"  ✅ 已授权 Hub「%s」(%s) 接入本机（可随时通过 `mgy hub links` 查看或 `mgy hub unlink` 吊销）\n",
		"  ✅ Authorized Hub %q (%s) to connect (inspect via `mgy hub links`, revoke via `mgy hub unlink`)\n",
		hubName, hubID,
	))
}

// WrapHandler mounts node-side Hub pairing, IP-verification, and link-management routes on a standalone/node gateway,
// and keeps authenticated Hub URLs refreshed across DHCP changes.
func (a *NodeAgent) WrapHandler(next http.Handler) http.Handler {
	if a == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/hub/pair-request":
			a.HandleHubPairRequest(w, r)
			return
		case "/api/v1/hub/pair-confirm":
			a.HandleHubPairConfirm(w, r)
			return
		case "/api/v1/hub/pair-code":
			a.HandleHubIssueLocalCode(w, r)
			return
		case "/api/v1/hub/verify":
			a.HandleHubVerify(w, r)
			return
		case "/api/v1/hub/links":
			a.HandleHubLinks(w, r)
			return
		default:
			if strings.HasPrefix(r.URL.Path, "/api/v1/hub/links/") {
				a.HandleHubLinks(w, r)
				return
			}
		}

		// If this request was authenticated with one of our Hub tokens and advertises a new Hub LAN URL,
		// refresh the stored link URL so push/Live Activity relay follows Hub DHCP changes automatically.
		if hubURL := NormalizeBaseURL(r.Header.Get(HeaderHubURL)); hubURL != "" {
			if tok := strings.TrimSpace(r.Header.Get(HeaderHubToken)); tok != "" {
				a.TouchHubLinkURLByToken(tok, hubURL)
			}
		}

		next.ServeHTTP(w, r)
	})
}

// HandleHubPairRequest handles POST /api/v1/hub/pair-request from a LAN Hub.
// It prints a 6-char confirmation code on this machine's terminal and returns only challenge metadata.
func (a *NodeAgent) HandleHubPairRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !isDirectLANOrLoopback(r) {
		writeJSONError(w, http.StatusForbidden, "forbidden: hub pairing is only allowed over direct private LAN")
		return
	}

	clientIP := auth.ExtractClientIP(r)
	if a.pairLimiter != nil {
		if !a.pairLimiter.Allow("req:global", hubPairReqGlobalPerMin, time.Minute) ||
			!a.pairLimiter.Allow("req:"+clientIP, hubPairReqPerIPPerMin, time.Minute) {
			w.Header().Set("Retry-After", "60")
			writeJSONError(w, http.StatusTooManyRequests, "too many hub pairing requests; please wait a minute")
			return
		}
	}

	var req HubPairRequestPayload
	if err := json.NewDecoder(io.LimitReader(r.Body, 16*1024)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	hubID := SanitizeNodeID(req.HubID)
	if hubID == "" {
		writeJSONError(w, http.StatusBadRequest, "hub_id is required")
		return
	}
	hubName := strings.TrimSpace(req.HubName)
	if hubName == "" {
		hubName = hubID
	}
	hubURL := NormalizeBaseURL(req.HubURL)

	code := generateJoinCode(6)
	challengeID := "hc_" + randomHex(8)
	now := time.Now()
	ch := &hubPairChallenge{
		ChallengeID: challengeID,
		Code:        code,
		HubID:       hubID,
		HubName:     hubName,
		HubURL:      hubURL,
		RequesterIP: clientIP,
		ExpiresAt:   now.Add(hubPairCodeTTL),
	}

	a.pairMu.Lock()
	a.pruneExpiredChallengesLocked(now)
	// Replace any previous pending challenge from the same HubID to avoid stale duplicates.
	for id, existing := range a.challenges {
		if !existing.LocalPreIssued && existing.HubID == hubID {
			delete(a.challenges, id)
		}
	}
	if len(a.challenges) >= hubPairMaxPending {
		a.evictOldestChallengeLocked()
	}
	a.challenges[challengeID] = ch
	a.pairMu.Unlock()

	formatted := FormatPairCode(code)
	if a.opts.OnPairPrompt != nil {
		a.opts.OnPairPrompt(hubName, clientIP, code)
	} else {
		defaultPrintHubPairPrompt(hubName, clientIP, formatted)
	}

	writeJSON(w, http.StatusOK, HubPairRequestResponse{
		Status:      "pending",
		ChallengeID: challengeID,
		NodeID:      a.NodeID(),
		NodeName:    a.NodeName(),
		ExpiresIn:   int(hubPairCodeTTL.Seconds()),
	})
}

// HandleHubIssueLocalCode handles POST /api/v1/hub/pair-code called locally by `mgy pair -hub`.
// Useful when the target mgy runs in background/tmux and the user cannot see foreground logs.
func (a *NodeAgent) HandleHubIssueLocalCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !isLocalAdminCaller(r) {
		writeJSONError(w, http.StatusForbidden, "forbidden: local admin access required")
		return
	}

	code := generateJoinCode(6)
	challengeID := "hl_" + randomHex(8)
	now := time.Now()
	ch := &hubPairChallenge{
		ChallengeID:    challengeID,
		Code:           code,
		LocalPreIssued: true,
		ExpiresAt:      now.Add(hubPairCodeTTL),
	}

	a.pairMu.Lock()
	a.pruneExpiredChallengesLocked(now)
	// Keep at most one active local pre-issued code at a time.
	for id, existing := range a.challenges {
		if existing.LocalPreIssued {
			delete(a.challenges, id)
		}
	}
	a.challenges[challengeID] = ch
	a.pairMu.Unlock()

	writeJSON(w, http.StatusOK, HubPairCodeResponse{
		Code:          code,
		FormattedCode: FormatPairCode(code),
		NodeID:        a.NodeID(),
		NodeName:      a.NodeName(),
		LANURL:        NormalizeBaseURL(a.opts.AdvertiseBaseURL),
		ExpiresIn:     int(hubPairCodeTTL.Seconds()),
	})
}

// HandleHubPairConfirm handles POST /api/v1/hub/pair-confirm from a LAN Hub.
// Verifies the 6-char confirmation code (max 3 attempts), persists the Hub link, and hot-enables push relay.
func (a *NodeAgent) HandleHubPairConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !isDirectLANOrLoopback(r) {
		writeJSONError(w, http.StatusForbidden, "forbidden: hub pairing is only allowed over direct private LAN")
		return
	}

	clientIP := auth.ExtractClientIP(r)
	if a.pairLimiter != nil && !a.pairLimiter.Allow("confirm:"+clientIP, hubPairConfirmPerMin, time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeJSONError(w, http.StatusTooManyRequests, "too many confirmation attempts; please wait")
		return
	}

	var req HubPairConfirmPayload
	if err := json.NewDecoder(io.LimitReader(r.Body, 16*1024)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}

	code := NormalizePairCode(req.Code)
	if code == "" {
		writeJSONError(w, http.StatusBadRequest, "confirmation code is required")
		return
	}
	hubID := SanitizeNodeID(req.HubID)
	if hubID == "" {
		writeJSONError(w, http.StatusBadRequest, "hub_id is required")
		return
	}
	hubName := strings.TrimSpace(req.HubName)
	if hubName == "" {
		hubName = hubID
	}
	hubURL := NormalizeBaseURL(req.HubURL)

	now := time.Now()
	a.pairMu.Lock()
	a.pruneExpiredChallengesLocked(now)

	var matched *hubPairChallenge
	var target *hubPairChallenge

	cid := strings.TrimSpace(req.ChallengeID)
	if cid != "" {
		if ch, ok := a.challenges[cid]; ok {
			if ch.HubID != "" && ch.HubID != hubID {
				a.pairMu.Unlock()
				writeJSONError(w, http.StatusForbidden, "challenge does not belong to this hub")
				return
			}
			target = ch
			if ConstantTimeEquals(code, ch.Code) {
				matched = ch
			}
		}
	}

	// Also allow matching an active local pre-issued code (`mgy pair -hub`),
	// whether challenge_id was omitted OR the user used `mgy pair -hub` after starting an interactive request.
	if matched == nil {
		for _, ch := range a.challenges {
			if !ch.LocalPreIssued {
				continue
			}
			if target == nil {
				target = ch
			}
			if ConstantTimeEquals(code, ch.Code) {
				matched = ch
				break
			}
		}
	}

	if matched == nil {
		if target == nil {
			a.pairMu.Unlock()
			writeJSONError(w, http.StatusUnauthorized, "pairing session expired or not found; please request a new code")
			return
		}
		target.Attempts++
		remaining := hubPairMaxAttempts - target.Attempts
		if remaining <= 0 {
			delete(a.challenges, target.ChallengeID)
			a.pairMu.Unlock()
			writeJSONError(w, http.StatusUnauthorized, "invalid confirmation code; session invalidated after 3 failed attempts")
			return
		}
		a.pairMu.Unlock()
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error":              fmt.Sprintf("invalid confirmation code (%d attempt(s) remaining)", remaining),
			"attempts_remaining": remaining,
		})
		return
	}

	// Single-use: consume matched challenge (and any pending challenge bound to this HubID).
	delete(a.challenges, matched.ChallengeID)
	if cid != "" && cid != matched.ChallengeID {
		delete(a.challenges, cid)
	}
	if hubURL == "" {
		hubURL = matched.HubURL
	}
	if hubName == hubID && matched.HubName != "" {
		hubName = matched.HubName
	}
	a.pairMu.Unlock()

	nodeToken := "mgy_node_" + randomHex(32)
	link := HubLink{
		HubID:      hubID,
		HubName:    hubName,
		HubURL:     hubURL,
		NodeToken:  nodeToken,
		PairedAt:   now,
		LastSeenAt: now,
	}
	if err := a.UpsertHubLink(link); err != nil {
		slog.Warn("[HubNode] Failed to persist authorized Hub link", "hub_id", hubID, "err", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to persist hub authorization")
		return
	}

	if a.opts.OnPairPrompt == nil {
		defaultPrintHubPairApproved(hubName, hubID)
	}
	slog.Info("[HubNode] Authorized LAN Hub via confirmation code", "hub_id", hubID, "hub_name", hubName, "hub_url", hubURL)

	writeJSON(w, http.StatusOK, HubPairConfirmResponse{
		Status:    "ok",
		NodeID:    a.NodeID(),
		NodeName:  a.NodeName(),
		NodeToken: nodeToken,
		OS:        runtime.GOOS,
		Version:   a.opts.Version,
	})
}

// HandleHubVerify handles POST /api/v1/hub/verify.
// Proves possession of the shared nodeToken for the given hub_id via HMAC-SHA256 without revealing the token.
func (a *NodeAgent) HandleHubVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !isDirectLANOrLoopback(r) {
		writeJSONError(w, http.StatusForbidden, "forbidden: hub verification is only allowed over direct private LAN")
		return
	}
	clientIP := auth.ExtractClientIP(r)
	if a.pairLimiter != nil && !a.pairLimiter.Allow("verify:"+clientIP, hubVerifyPerIPPerMin, time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeJSONError(w, http.StatusTooManyRequests, "too many verify requests")
		return
	}

	var req HubVerifyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4*1024)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	hubID := strings.TrimSpace(req.HubID)
	nonce := strings.TrimSpace(req.Nonce)
	if hubID == "" || len(nonce) < 16 || len(nonce) > 128 {
		writeJSONError(w, http.StatusBadRequest, "hub_id and nonce (16..128 chars) are required")
		return
	}

	token := a.tokenForHubID(hubID)
	if token == "" {
		writeJSONError(w, http.StatusNotFound, "hub not paired with this node")
		return
	}
	nodeID := a.NodeID()
	writeJSON(w, http.StatusOK, HubVerifyResponse{
		NodeID:   nodeID,
		NodeName: a.NodeName(),
		Proof:    ComputeVerifyProof(token, hubID, nodeID, nonce),
	})
}

// HandleHubLinks handles GET /api/v1/hub/links and DELETE /api/v1/hub/links/{hubID} for local admin inspection/revocation.
func (a *NodeAgent) HandleHubLinks(w http.ResponseWriter, r *http.Request) {
	if !isLocalAdminCaller(r) {
		writeJSONError(w, http.StatusForbidden, "forbidden: local admin access required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		links := a.ListHubLinks()
		views := make([]HubLinkView, 0, len(links))
		for _, lk := range links {
			views = append(views, HubLinkView{
				HubID:      lk.HubID,
				HubName:    lk.HubName,
				HubURL:     lk.HubURL,
				PairedAt:   lk.PairedAt,
				LastSeenAt: lk.LastSeenAt,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"node_id":   a.NodeID(),
			"node_name": a.NodeName(),
			"links":     views,
		})

	case http.MethodDelete:
		target := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/v1/hub/links/"))
		if target == "" || target == "/api/v1/hub/links" {
			target = strings.TrimSpace(r.URL.Query().Get("id"))
		}
		if target == "" {
			writeJSONError(w, http.StatusBadRequest, "hub id or name is required")
			return
		}
		removed, ok := a.RevokeHubLink(target)
		if !ok {
			writeJSONError(w, http.StatusNotFound, "hub link not found")
			return
		}
		slog.Info("[HubNode] Revoked Hub authorization", "hub_id", removed.HubID, "hub_name", removed.HubName)
		writeJSON(w, http.StatusOK, map[string]any{
			"status":   "ok",
			"removed":  removed.HubID,
			"hub_name": removed.HubName,
		})

	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *NodeAgent) pruneExpiredChallengesLocked(now time.Time) {
	for id, ch := range a.challenges {
		if now.After(ch.ExpiresAt) {
			delete(a.challenges, id)
		}
	}
}

func (a *NodeAgent) evictOldestChallengeLocked() {
	var oldestID string
	var oldestExp time.Time
	for id, ch := range a.challenges {
		if oldestID == "" || ch.ExpiresAt.Before(oldestExp) {
			oldestID = id
			oldestExp = ch.ExpiresAt
		}
	}
	if oldestID != "" {
		delete(a.challenges, oldestID)
	}
}

func (a *NodeAgent) tokenForHubID(hubID string) string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if lk, ok := a.links[hubID]; ok && lk.NodeToken != "" {
		return lk.NodeToken
	}
	for _, lk := range a.links {
		if strings.EqualFold(lk.HubID, hubID) && lk.NodeToken != "" {
			return lk.NodeToken
		}
	}
	// Fallback to legacy single-node credential (mgy node mode)
	if a.nodeToken != "" {
		return a.nodeToken
	}
	return ""
}

// UpsertHubLink adds or updates an authorized Hub link, persists hub_links.json, and hot-enables push relay.
func (a *NodeAgent) UpsertHubLink(link HubLink) error {
	link.HubID = strings.TrimSpace(link.HubID)
	link.NodeToken = strings.TrimSpace(link.NodeToken)
	link.HubURL = NormalizeBaseURL(link.HubURL)
	if link.HubID == "" || link.NodeToken == "" {
		return fmt.Errorf("hub_id and node_token are required")
	}
	if strings.TrimSpace(link.HubName) == "" {
		link.HubName = link.HubID
	}
	if link.PairedAt.IsZero() {
		link.PairedAt = time.Now()
	}

	a.mu.Lock()
	if a.links == nil {
		a.links = make(map[string]HubLink)
	}
	a.links[link.HubID] = link
	snapshot := a.sortedLinksLocked()
	a.mu.Unlock()

	if err := SaveHubLinks(a.opts.DataDir, snapshot); err != nil {
		return err
	}
	a.ensureHubRelayEnabled()
	return nil
}

// ListHubLinks returns a sorted snapshot of all authorized Hub links on this machine.
func (a *NodeAgent) ListHubLinks() []HubLink {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sortedLinksLocked()
}

// HasHubLinks reports whether at least one Hub is authorized via hub_links.json or legacy nodeToken.
func (a *NodeAgent) HasHubLinks() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.links) > 0 || (a.hubURL != "" && a.nodeToken != "")
}

// RevokeHubLink removes an authorized Hub by ID or name (case-insensitive) and persists hub_links.json.
func (a *NodeAgent) RevokeHubLink(idOrName string) (HubLink, bool) {
	key := strings.TrimSpace(idOrName)
	if key == "" {
		return HubLink{}, false
	}
	a.mu.Lock()
	var removed HubLink
	found := false
	if lk, ok := a.links[key]; ok {
		removed = lk
		delete(a.links, key)
		found = true
	} else {
		for id, lk := range a.links {
			if strings.EqualFold(id, key) || strings.EqualFold(lk.HubName, key) {
				removed = lk
				delete(a.links, id)
				found = true
				break
			}
		}
	}
	var snapshot []HubLink
	if found {
		snapshot = a.sortedLinksLocked()
	}
	a.mu.Unlock()

	if !found {
		return HubLink{}, false
	}
	_ = SaveHubLinks(a.opts.DataDir, snapshot)
	return removed, true
}

// TouchHubLinkURLByToken updates LastSeenAt and refreshes HubURL when an authenticated Hub advertises a new LAN URL.
func (a *NodeAgent) TouchHubLinkURLByToken(token, hubURL string) {
	token = strings.TrimSpace(token)
	hubURL = NormalizeBaseURL(hubURL)
	if token == "" || hubURL == "" {
		return
	}
	a.mu.Lock()
	changed := false
	for id, lk := range a.links {
		if ConstantTimeEquals(token, lk.NodeToken) {
			if lk.HubURL != hubURL {
				lk.HubURL = hubURL
				lk.LastSeenAt = time.Now()
				a.links[id] = lk
				changed = true
			}
			break
		}
	}
	var snapshot []HubLink
	if changed {
		snapshot = a.sortedLinksLocked()
	}
	a.mu.Unlock()

	if changed {
		_ = SaveHubLinks(a.opts.DataDir, snapshot)
	}
}

func (a *NodeAgent) sortedLinksLocked() []HubLink {
	out := make([]HubLink, 0, len(a.links))
	for _, lk := range a.links {
		out = append(out, lk)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].HubName != out[j].HubName {
			return out[i].HubName < out[j].HubName
		}
		return out[i].HubID < out[j].HubID
	})
	return out
}
