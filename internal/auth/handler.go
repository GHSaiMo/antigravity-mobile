package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/e2ee"
)

const jsonBodyReadTimeout = 30 * time.Second

func setJSONReadDeadline(w http.ResponseWriter) {
	_ = http.NewResponseController(w).SetReadDeadline(time.Now().Add(jsonBodyReadTimeout))
}

// DefaultDeviceCookieMaxAge is the session cookie validity duration (30 days, reduced from 400 days - H-3).
const DefaultDeviceCookieMaxAge = 86400 * 30

// PairRequest is the payload sent by clients to pair.
type PairRequest struct {
	PairingCode string `json:"pairing_code"`
	DeviceName  string `json:"device_name"`
	Platform    string `json:"platform"`
	DeviceKey   string `json:"device_key,omitempty"`
	E2EEPubKey  string `json:"e2ee_pub,omitempty"`
}

// EndpointInfo represents an accessible network endpoint of the gateway.
type EndpointInfo struct {
	Type string `json:"type"` // "lan", "ddns", "primary", "cloudflare", "relay"
	URL  string `json:"url"`  // e.g. "http://192.168.1.50:58900"
}

// PairResponse is the response returned upon successful pairing.
type PairResponse struct {
	DeviceID       string         `json:"device_id"`
	DeviceToken    string         `json:"device_token"`
	Endpoints      []EndpointInfo `json:"endpoints,omitempty"`
	OS             string         `json:"os,omitempty"`
	Platform       string         `json:"platform,omitempty"`
	E2EEGatewayPub string         `json:"e2ee_gateway_pub,omitempty"`
	E2EESalt       string         `json:"e2ee_salt,omitempty"`
	RelayCloudURL  string         `json:"relay_cloud_url,omitempty"`
}

// AuthHandler handles authentication and device management routes.
type AuthHandler struct {
	mu                   sync.RWMutex
	store                *AuthStore
	pairingMgr           *PairingManager
	host                 string
	port                 int
	ssl                  bool
	lanHost              string
	ddnsHost             string
	relayURL             string
	cfURL                string
	cloudMode            string // "cloudflare" (default) | "relay_v1" | "relay"
	relayCloudURL        string // Dual-track E2EE relay URL when running alongside legacy CF tunnel
	e2eeIdentity         *e2ee.Identity
	onDeviceE2EEUpgraded func(allUpgraded bool)
	policy               AuthPolicy
	limiter              *RateLimiter // unauthenticated & admin IP rate limiter (endpoints, pair, session)
	wsTicketLimiter      *RateLimiter // isolated limiter for authenticated /api/v1/auth/ws-ticket
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(store *AuthStore, pairingMgr *PairingManager, host string, port int, ssl bool) *AuthHandler {
	return &AuthHandler{
		store:           store,
		pairingMgr:      pairingMgr,
		limiter:         NewRateLimiter(),
		wsTicketLimiter: NewRateLimiter(),
		host:            host,
		port:            port,
		ssl:             ssl,
	}
}

// Close releases background rate-limiter cleanup goroutines.
func (h *AuthHandler) Close() {
	if h == nil {
		return
	}
	if h.limiter != nil {
		h.limiter.Close()
	}
	if h.wsTicketLimiter != nil {
		h.wsTicketLimiter.Close()
	}
}

// SetEndpoints sets discovered network hosts (LAN IPv4, DDNS) for pairing responses.
func (h *AuthHandler) SetEndpoints(lanHost, ddnsHost string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lanHost = strings.TrimSpace(lanHost)
	h.ddnsHost = strings.TrimSpace(ddnsHost)
}

// SetRelayURL sets the cloud relay URL (e.g. from embedded FRP tunnel) for pairing responses.
func (h *AuthHandler) SetRelayURL(relayURL string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.relayURL = strings.TrimSpace(relayURL)
}

// SetPrimary sets the primary network host, port, and SSL status.
func (h *AuthHandler) SetPrimary(host string, port int, ssl bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.host = strings.TrimSpace(host)
	h.port = port
	h.ssl = ssl
}

// SetCloudflareURL sets the Cloudflare Tunnel HTTPS endpoint URL for pairing responses.
func (h *AuthHandler) SetCloudflareURL(cfURL string) {
	h.SetCloudEndpoint(cfURL, "cloudflare")
}

// SetCloudEndpoint sets the active managed cloud tunnel HTTPS endpoint URL and transport mode ("cloudflare" | "relay_v1").
func (h *AuthHandler) SetCloudEndpoint(cloudURL, mode string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cfURL = strings.TrimSpace(cloudURL)
	m := strings.ToLower(strings.TrimSpace(mode))
	if m == "" {
		m = "cloudflare"
	}
	h.cloudMode = m
}

// SetE2EEIdentity attaches the gateway's persistent P-256 ECDH identity for E2EE pairing and upgrades.
func (h *AuthHandler) SetE2EEIdentity(id *e2ee.Identity) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.e2eeIdentity = id
}

// E2EEIdentity returns the configured E2EE identity, if any.
func (h *AuthHandler) E2EEIdentity() *e2ee.Identity {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.e2eeIdentity
}

// E2EEPubKey returns the gateway's Base64URL P-256 public key, or "" if E2EE is not configured.
func (h *AuthHandler) E2EEPubKey() string {
	h.mu.RLock()
	id := h.e2eeIdentity
	h.mu.RUnlock()
	if id == nil {
		return ""
	}
	return id.PublicKeyB64()
}

// SetDualTrackRelayURL registers an E2EE relay_v1 HTTPS URL running in parallel with a legacy Cloudflare Tunnel.
// Only E2EE-ready devices receive this URL; legacy un-upgraded clients keep receiving the legacy Cloudflare URL.
func (h *AuthHandler) SetDualTrackRelayURL(u string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.relayCloudURL = strings.TrimSpace(u)
}

// DualTrackRelayURL returns the active parallel relay_v1 URL, if running in dual-track migration mode.
func (h *AuthHandler) DualTrackRelayURL() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.relayCloudURL
}

// SetOnDeviceE2EEUpgraded registers a callback fired whenever a paired device completes E2EE key negotiation.
// allUpgraded is true when zero legacy non-E2EE paired devices remain in AuthStore.
func (h *AuthHandler) SetOnDeviceE2EEUpgraded(fn func(allUpgraded bool)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onDeviceE2EEUpgraded = fn
}

// CloudflareURL returns the configured Cloudflare Tunnel HTTPS endpoint URL.
func (h *AuthHandler) CloudflareURL() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfURL
}

// CloudURL returns the active public HTTPS tunnel URL (managed Cloudflare or relay endpoint)
// for X-Antigravity-Cloud-URL advertisement and external access redaction.
func (h *AuthHandler) CloudURL() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.cfURL != "" {
		return h.cfURL
	}
	if strings.HasPrefix(strings.ToLower(h.relayURL), "https://") {
		return h.relayURL
	}
	return ""
}

// requestDeviceE2EEReady reports whether the authenticated caller has negotiated an E2EE session key.
func (h *AuthHandler) requestDeviceE2EEReady(r *http.Request) bool {
	if r == nil {
		return false
	}
	if e2ee.IsRelayTransport(r.Context()) {
		return true
	}
	if dev, ok := DeviceFromContext(r.Context()); ok && dev != nil && dev.E2EEReady {
		return true
	}
	if token := ExtractToken(r); token != "" && h.store != nil {
		if dev, ok := h.store.ValidateToken(token); ok && dev != nil && dev.E2EEReady {
			return true
		}
	}
	return false
}

// CloudURLForRequest returns the appropriate managed HTTPS tunnel URL for r.
// In dual-track mode (legacy CF + parallel E2EE relay), E2EE-ready devices receive the relay_v1 URL
// while un-upgraded legacy clients continue receiving the legacy Cloudflare URL so they never hit HTTP 426.
func (h *AuthHandler) CloudURLForRequest(r *http.Request) string {
	h.mu.RLock()
	dualRelay := h.relayCloudURL
	h.mu.RUnlock()
	if dualRelay != "" && h.requestDeviceE2EEReady(r) {
		return dualRelay
	}
	return h.CloudURL()
}

// relayHost extracts the hostname from the configured cloud relay URL.
func (h *AuthHandler) relayHost() string {
	h.mu.RLock()
	relay := h.relayURL
	cf := h.cfURL
	h.mu.RUnlock()
	return parseRelayHostname(relay, cf)
}

func parseRelayHostname(relayURL, cfURL string) string {
	raw := strings.TrimSpace(relayURL)
	if raw == "" {
		raw = strings.TrimSpace(cfURL)
	}
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(u.Hostname())
}

// SetAuthPolicy sets loopback-trust / tunnel policy used by admin authorization.
func (h *AuthHandler) SetAuthPolicy(policy AuthPolicy) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.policy = policy
}

func (h *AuthHandler) getPolicy() AuthPolicy {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.policy
}

// GetEndpoints returns candidate endpoint URLs for legacy or unauthenticated clients.
func (h *AuthHandler) GetEndpoints() []EndpointInfo {
	return h.GetEndpointsForDevice(false)
}

// GetEndpointsForDevice returns candidate endpoint URLs tailored to the client's E2EE readiness.
// When dual-track relay_v1 is active, E2EE-ready devices receive the relay_v1 endpoint first.
func (h *AuthHandler) GetEndpointsForDevice(e2eeReady bool) []EndpointInfo {
	h.mu.RLock()
	host := h.host
	port := h.port
	ssl := h.ssl
	lanHost := h.lanHost
	ddnsHost := h.ddnsHost
	relayURL := h.relayURL
	cfURL := h.cfURL
	cloudMode := h.cloudMode
	dualRelayURL := h.relayCloudURL
	h.mu.RUnlock()

	if e2eeReady && dualRelayURL != "" {
		cfURL = dualRelayURL
		cloudMode = "relay_v1"
	}

	var endpoints []EndpointInfo
	scheme := "http://"
	if ssl {
		scheme = "https://"
	}

	// 1. Managed Cloud Tunnel (Cloudflare or future relay_v1 HTTPS domain) - Top Priority
	if cfURL != "" {
		epType := "cloudflare"
		if cloudMode == "relay_v1" || cloudMode == "relay" {
			epType = "relay"
		}
		endpoints = append(endpoints, EndpointInfo{
			Type: epType,
			URL:  cfURL,
		})
		lan := lanHost
		if lan == "" && !strings.Contains(host, ":") && host != "" && host != "127.0.0.1" && host != "localhost" && !strings.Contains(host, ".") {
			lan = host
		}
		if lan != "" {
			lanPort := port
			if ssl && port == 443 {
				lanPort = 58900
			}
			endpoints = append(endpoints, EndpointInfo{
				Type: "lan",
				URL:  fmt.Sprintf("http://%s:%d", lan, lanPort),
			})
		}
		// On the Cloudflare branch, DDNS and legacy FRP relays are retired.
		return endpoints
	}

	// 2. LAN IPv4 - Advertised when local gateway is running cleartext HTTP (i.e. not self-TLS, or Cloudflare termination)
	if !ssl {
		lan := lanHost
		if lan == "" && !strings.Contains(host, ":") && host != "" && host != "127.0.0.1" && host != "localhost" && !strings.Contains(host, ".") {
			lan = host
		}
		if lan != "" {
			lanPort := port
			if ssl && port == 443 {
				lanPort = 58900
			}
			endpoints = append(endpoints, EndpointInfo{
				Type: "lan",
				URL:  fmt.Sprintf("http://%s:%d", lan, lanPort),
			})
		}
	}

	// 3. DDNS / Custom Domain
	if ddnsHost != "" {
		endpoints = append(endpoints, EndpointInfo{
			Type: "ddns",
			URL:  fmt.Sprintf("%s%s:%d", scheme, ddnsHost, port),
		})
	}

	// 4. Cloud Relay (embedded FRP tunnel)
	if relayURL != "" {
		dup := false
		for _, ep := range endpoints {
			if ep.URL == relayURL {
				dup = true
				break
			}
		}
		if !dup {
			endpoints = append(endpoints, EndpointInfo{
				Type: "relay",
				URL:  relayURL,
			})
		}
	}

	// 5. Fallback primary if no other endpoints detected
	if len(endpoints) == 0 {
		hStr := host
		if hStr == "" {
			hStr = "127.0.0.1"
		}
		if strings.Contains(hStr, ":") && !strings.HasPrefix(hStr, "[") {
			hStr = fmt.Sprintf("[%s]", hStr)
		}
		endpoints = append(endpoints, EndpointInfo{
			Type: "primary",
			URL:  fmt.Sprintf("%s%s:%d", scheme, hStr, port),
		})
	}

	return endpoints
}

// HandleEndpoints handles GET /api/v1/auth/endpoints to return candidate connection endpoints.
func (h *AuthHandler) HandleEndpoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// SEC-AUDIT M-5: Rate limit endpoints polling to prevent network scanning
	clientIP := ExtractClientIP(r)
	rateKey := RateLimitKeyIP(clientIP)
	if h.limiter != nil && !h.limiter.Allow("endpoints:"+rateKey, 60, time.Minute) {
		w.Header().Set("Retry-After", "60")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "too many requests"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	authed := h.isAuthenticated(r)
	e2eeReady := h.requestDeviceE2EEReady(r)
	cloudURL := h.CloudURLForRequest(r)
	if cloudURL != "" && authed {
		w.Header().Set("X-Antigravity-Cloud-URL", cloudURL)
	}

	allEndpoints := h.GetEndpointsForDevice(e2eeReady)
	// SEC-AUDIT M-5: If the request comes from an external public IP (via proxy/tunnel)
	// and caller is not authenticated, redact private LAN endpoints to avoid leaking internal topology.
	isExternal := IsProxiedRequest(r)
	if isExternal && cloudURL != "" && !authed {
		var safeEndpoints []EndpointInfo
		for _, ep := range allEndpoints {
			if ep.Type != "lan" {
				safeEndpoints = append(safeEndpoints, ep)
			}
		}
		if len(safeEndpoints) > 0 {
			allEndpoints = safeEndpoints
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"endpoints": allEndpoints,
		"os":        runtime.GOOS,
		"platform":  runtime.GOOS,
	})
}

func (h *AuthHandler) isAuthenticated(r *http.Request) bool {
	if h.isAuthorizedAdmin(r) {
		return true
	}
	token := ExtractToken(r)
	if token == "" {
		return false
	}
	device, ok := h.store.ValidateToken(token)
	return ok && device != nil
}

// requestIsHTTPS reports whether the client reached us over HTTPS (directly or via a TLS-terminating
// proxy/tunnel). The Secure cookie flag must follow the request, not the gateway's primary endpoint:
// browsers silently drop Secure cookies received over plain-HTTP LAN access.
func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return strings.Contains(strings.ToLower(r.Header.Get("CF-Visitor")), `"scheme":"https"`)
}

// RequestAuthorized reports whether r would pass AuthMiddleware on a protected route:
// genuine localhost, trusted LAN (when enabled), the admin token, or a valid paired-device token.
// It lets whitelisted HTML entry points decide between serving the app and a pairing page.
func (h *AuthHandler) RequestAuthorized(r *http.Request) bool {
	policy := h.getPolicy()
	proxied := IsProxiedRequest(r)
	if AuthDisabledRequested() && !policy.TunnelEnabled && policy.ListenLoopback && IsLoopbackAddr(r.RemoteAddr) && !proxied {
		return true
	}
	if proxied {
		return h.hasValidToken(r)
	}
	if ImplicitTrustRequestOK(r) && (IsLoopbackAddr(r.RemoteAddr) || (policy.TrustLAN && IsPrivateLANAddr(r.RemoteAddr))) {
		return true
	}
	return h.hasValidToken(r)
}

func (h *AuthHandler) hasValidToken(r *http.Request) bool {
	token := ExtractToken(r)
	if token == "" {
		return false
	}
	if adminTok := GetAdminToken(); adminTok != "" && ConstantTimeTokenEquals(token, adminTok) {
		return true
	}
	device, ok := h.store.ValidateToken(token)
	return ok && device != nil
}

// HandlePair handles POST /api/v1/auth/pair.
func (h *AuthHandler) HandlePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	setJSONReadDeadline(w)

	var req PairRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid json body"})
		return
	}

	resp, status, errMsg, rateLimited := h.executePair(req, "", r)
	if rateLimited {
		w.Header().Set("Retry-After", "60")
	}
	if status != http.StatusOK {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": errMsg})
		return
	}

	isSSL := requestIsHTTPS(r)
	http.SetCookie(w, &http.Cookie{
		Name:     DeviceCookieName,
		Value:    resp.DeviceToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSSL,
		MaxAge:   DefaultDeviceCookieMaxAge,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// PairFromEncryptedPayload serves as e2ee.Handler.PairDispatcher for `POST /api/v1/e2ee/pair`.
// It runs the exact same rate limiting, pairing code validation, and device credential issuance
// as HandlePair, while deriving the persistent per-device E2EE key directly from clientPubB64.
func (h *AuthHandler) PairFromEncryptedPayload(rawBody []byte, clientPubB64 string, r *http.Request) ([]byte, string, int, error) {
	var req PairRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		errJSON, _ := json.Marshal(map[string]string{"error": "invalid json body"})
		return errJSON, "", http.StatusBadRequest, nil
	}
	resp, status, errMsg, _ := h.executePair(req, clientPubB64, r)
	if status != http.StatusOK {
		errJSON, _ := json.Marshal(map[string]string{"error": errMsg})
		return errJSON, "", status, nil
	}
	respJSON, err := json.Marshal(resp)
	if err != nil {
		return nil, "", http.StatusInternalServerError, err
	}
	return respJSON, resp.DeviceID, http.StatusOK, nil
}

func (h *AuthHandler) executePair(req PairRequest, envelopeClientPub string, r *http.Request) (PairResponse, int, string, bool) {
	clientIP := ExtractClientIP(r)
	rateKey := RateLimitKeyIP(clientIP)
	if h.limiter != nil {
		if !h.limiter.Allow("pair:global", 40, time.Minute) || !h.limiter.Allow("pair:"+rateKey, 8, time.Minute) {
			auditWarn("RATE_LIMIT", "action", "pair", "ip", clientIP, "subnet", rateKey)
			return PairResponse{}, http.StatusTooManyRequests, "too many pairing attempts", true
		}
	}

	req.PairingCode = strings.TrimSpace(req.PairingCode)
	if req.PairingCode == "" {
		return PairResponse{}, http.StatusBadRequest, "pairing_code is required", false
	}

	// Validate and consume code immediately (prevent replay)
	if !h.pairingMgr.ValidateAndConsume(req.PairingCode) {
		auditWarn("PAIR_FAILURE", "reason", "invalid_or_expired_code", "ip", CleanIP(r.RemoteAddr))
		return PairResponse{}, http.StatusUnauthorized, "invalid or expired pairing code", false
	}

	deviceID, deviceToken, err := GenerateDeviceCredentials()
	if err != nil {
		return PairResponse{}, http.StatusInternalServerError, "failed to generate credentials", false
	}

	deviceName := strings.TrimSpace(req.DeviceName)
	if deviceName == "" {
		deviceName = "Mobile Device"
	}
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = "unknown"
	}

	now := time.Now()
	cleanIP := ExtractClientIP(r)
	deviceKey := strings.TrimSpace(req.DeviceKey)
	if len(deviceKey) < 16 || len(deviceKey) > 128 {
		deviceKey = ""
	}
	createdAt := now
	// Same physical client pairing again: keep its identity and rotate the credential.
	if existing, ok := h.store.FindByDeviceKey(deviceKey); ok {
		deviceID = existing.DeviceID
		createdAt = existing.CreatedAt
	}

	device := PairedDevice{
		DeviceID:   deviceID,
		DeviceName: deviceName,
		Platform:   platform,
		TokenHash:  HashToken(deviceToken),
		CreatedAt:  createdAt,
		LastSeenAt: now,
		LastSeenIP: cleanIP,
		DeviceKey:  deviceKey,
	}

	resp := PairResponse{
		DeviceID:    deviceID,
		DeviceToken: deviceToken,
		OS:          runtime.GOOS,
		Platform:    runtime.GOOS,
	}

	// Negotiate persistent E2EE session key if client supplied its P-256 public key.
	clientPub := strings.TrimSpace(envelopeClientPub)
	if clientPub == "" {
		clientPub = strings.TrimSpace(req.E2EEPubKey)
	}
	h.mu.RLock()
	id := h.e2eeIdentity
	dualRelayURL := h.relayCloudURL
	upgradeCb := h.onDeviceE2EEUpgraded
	h.mu.RUnlock()

	if clientPub != "" && id != nil {
		if shared, eErr := id.ECDH(clientPub); eErr == nil {
			if salt, rErr := e2ee.RandomBytes(16); rErr == nil {
				devE2EEKey := e2ee.DeriveDeviceKey(shared, salt, deviceID)
				device.E2EEKeyB64 = base64.RawURLEncoding.EncodeToString(devE2EEKey)
				device.E2EEReady = true
				resp.E2EEGatewayPub = id.PublicKeyB64()
				resp.E2EESalt = base64.RawURLEncoding.EncodeToString(salt)
				if dualRelayURL != "" {
					resp.RelayCloudURL = dualRelayURL
				}
			}
		}
	}

	if err := h.store.AddDevice(device); err != nil {
		return PairResponse{}, http.StatusInternalServerError, "failed to persist device", false
	}

	resp.Endpoints = h.GetEndpointsForDevice(device.E2EEReady)
	if device.E2EEReady && upgradeCb != nil {
		allUpgraded := !h.store.HasLegacyNonE2EEDevices()
		go upgradeCb(allUpgraded)
	}
	return resp, http.StatusOK, "", false
}

// HandleE2EEUpgrade handles POST /api/v1/auth/e2ee/upgrade.
// It allows an already-paired device upgrading to v1.2.5+ to negotiate its persistent
// E2EE key silently over LAN or legacy Cloudflare Tunnel without re-scanning a QR code.
func (h *AuthHandler) HandleE2EEUpgrade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	setJSONReadDeadline(w)

	if BearerToken(r) == "" && !CookieWriteCSRFOK(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "forbidden: cross-origin request rejected"})
		return
	}

	// Resolve real paired device: even on TrustLAN or localhost, prefer the explicit Bearer/Cookie device token
	// so paired mobile apps upgrading over Wi-Fi get bound to their actual DeviceID.
	var targetDev *PairedDevice
	if tok := ExtractToken(r); tok != "" && h.store != nil {
		if d, ok := h.store.ValidateToken(tok); ok && d != nil {
			targetDev = d
		}
	}
	if targetDev == nil {
		if ctxDev, ok := DeviceFromContext(r.Context()); ok && ctxDev != nil &&
			ctxDev.DeviceID != "" && ctxDev.DeviceID != "admin-local" && !strings.HasPrefix(ctxDev.DeviceID, "lan-") {
			targetDev = ctxDev
		}
	}
	if targetDev == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "paired device token required for e2ee upgrade"})
		return
	}

	h.mu.RLock()
	id := h.e2eeIdentity
	dualRelayURL := h.relayCloudURL
	upgradeCb := h.onDeviceE2EEUpgraded
	h.mu.RUnlock()
	if id == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "e2ee not initialized"})
		return
	}

	var req struct {
		ClientPub string `json:"client_pub"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.ClientPub) == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "client_pub is required"})
		return
	}

	shared, err := id.ECDH(req.ClientPub)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid client_pub"})
		return
	}
	salt, err := e2ee.RandomBytes(16)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "rng failure"})
		return
	}
	devKey := e2ee.DeriveDeviceKey(shared, salt, targetDev.DeviceID)
	if err := h.store.SetDeviceE2EEKey(targetDev.DeviceID, devKey); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to persist e2ee key"})
		return
	}

	cloudURL := dualRelayURL
	if cloudURL == "" {
		cloudURL = h.CloudURL()
	}

	auditInfo("E2EE_UPGRADE_SUCCESS", "device_id", targetDev.DeviceID, "ip", ExtractClientIP(r))
	if upgradeCb != nil {
		allUpgraded := !h.store.HasLegacyNonE2EEDevices()
		go upgradeCb(allUpgraded)
	}

	w.Header().Set("Content-Type", "application/json")
	if cloudURL != "" {
		w.Header().Set("X-Antigravity-Cloud-URL", cloudURL)
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"device_id":   targetDev.DeviceID,
		"gateway_pub": id.PublicKeyB64(),
		"e2ee_salt":   base64.RawURLEncoding.EncodeToString(salt),
		"cloud_url":   cloudURL,
	})
}

func clearDeviceCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     DeviceCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsHTTPS(r),
		MaxAge:   -1,
	})
}

// HandleLogout handles POST /api/v1/auth/logout (SEC-08).
// It expires the HttpOnly session cookie and, if the session belongs to a paired device,
// revokes that device's credential in AuthStore.
func (h *AuthHandler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	setJSONReadDeadline(w)
	if BearerToken(r) == "" && !CookieWriteCSRFOK(r) {
		auditWarn("CSRF_REJECTED", "action", "logout", "ip", CleanIP(r.RemoteAddr), "origin", r.Header.Get("Origin"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "forbidden: cross-origin request rejected"})
		return
	}

	var removedID string
	if token := ExtractToken(r); token != "" {
		if dev, ok := h.store.ValidateToken(token); ok && dev != nil {
			removedID = dev.DeviceID
			_ = h.store.RemoveDevice(dev.DeviceID)
			auditInfo("LOGOUT_UNPAIR_SUCCESS", "device_id", dev.DeviceID, "ip", CleanIP(r.RemoteAddr))
		}
	}

	clearDeviceCookie(w, r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"status":    "logged_out",
		"device_id": removedID,
	})
}

// HandleUnpair handles POST /api/v1/auth/unpair.
// It allows a paired device to revoke its own pairing, or an admin to unpair a specified device.
func (h *AuthHandler) HandleUnpair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	setJSONReadDeadline(w)
	if BearerToken(r) == "" && !CookieWriteCSRFOK(r) {
		auditWarn("CSRF_REJECTED", "action", "unpair", "ip", CleanIP(r.RemoteAddr), "origin", r.Header.Get("Origin"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "forbidden: cross-origin request rejected"})
		return
	}

	_, cookieErr := r.Cookie(DeviceCookieName)
	hasCookie := cookieErr == nil
	token := ExtractToken(r)
	var req struct {
		DeviceID string `json:"device_id"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	targetID := strings.TrimSpace(req.DeviceID)
	if targetID == "" {
		targetID = strings.TrimSpace(r.URL.Query().Get("id"))
	}

	// 1. Admin path
	if h.isAuthorizedAdmin(r) {
		if targetID != "" {
			_ = h.store.RemoveDevice(targetID)
			auditInfo("UNPAIR_SUCCESS", "by", "admin", "device_id", targetID, "ip", CleanIP(r.RemoteAddr))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"status":    "unpaired",
				"device_id": targetID,
			})
			return
		}
	}

	// 2. Device token path
	if token == "" {
		if hasCookie {
			clearDeviceCookie(w, r)
		}
		auditWarn("AUTH_FAILURE", "action", "unpair", "reason", "missing_token", "ip", CleanIP(r.RemoteAddr))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized: missing authentication token"})
		return
	}

	if hasCookie && targetID == "" {
		if adminTok := GetAdminToken(); adminTok != "" && ConstantTimeTokenEquals(token, adminTok) {
			clearDeviceCookie(w, r)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"status":    "unpaired",
				"device_id": "admin-local",
			})
			return
		}
	}

	device, ok := h.store.ValidateToken(token)
	if !ok || device == nil {
		if hasCookie {
			clearDeviceCookie(w, r)
		}
		auditWarn("AUTH_FAILURE", "action", "unpair", "reason", "invalid_token", "ip", CleanIP(r.RemoteAddr))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid or expired token"})
		return
	}

	// If a specific targetID was provided, ensure it matches caller's deviceID unless caller is admin
	if targetID != "" && targetID != device.DeviceID && !h.isAuthorizedAdmin(r) {
		auditWarn("AUTH_FAILURE", "action", "unpair", "reason", "forbidden_target_mismatch",
			"device_id", device.DeviceID, "target", targetID, "ip", CleanIP(r.RemoteAddr))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "forbidden: cannot unpair another device"})
		return
	}

	deviceIDToRemove := device.DeviceID
	if err := h.store.RemoveDevice(deviceIDToRemove); err != nil {
		auditError("UNPAIR_ERROR", "device_id", deviceIDToRemove, "err", err)
	} else {
		auditInfo("UNPAIR_SUCCESS", "device_id", deviceIDToRemove, "ip", CleanIP(r.RemoteAddr))
	}

	// Clear session cookie if set
	clearDeviceCookie(w, r)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"status":    "unpaired",
		"device_id": deviceIDToRemove,
	})
}

// HandleDevices handles GET /api/v1/devices and DELETE /api/v1/devices/{id}.
func (h *AuthHandler) HandleDevices(w http.ResponseWriter, r *http.Request) {
	if !h.isAuthorizedAdmin(r) {
		auditWarn("AUTH_FAILURE", "action", "devices_management", "ip", CleanIP(r.RemoteAddr), "path", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}

	path := r.URL.Path
	prefix := "/api/v1/devices"
	subPath := strings.TrimPrefix(path, prefix)
	subPath = strings.TrimPrefix(subPath, "/")

	switch r.Method {
	case http.MethodGet:
		devices := h.store.ListDevices()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(devices)

	case http.MethodDelete:
		targetID := subPath
		if targetID == "" {
			targetID = r.URL.Query().Get("id")
		}

		if targetID == "all" || r.URL.Query().Get("all") == "true" {
			count, err := h.store.ClearAll()
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"status":  "cleared",
				"cleared": count,
			})
			return
		}

		if targetID == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "device_id is required, or use id=all"})
			return
		}

		if err := h.store.RemoveDevice(targetID); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status":    "deleted",
			"device_id": targetID,
		})

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// HandleNewPairingSession handles POST /api/v1/auth/session to generate a new pairing code.
func (h *AuthHandler) HandleNewPairingSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	setJSONReadDeadline(w)

	clientIP := ExtractClientIP(r)
	rateKey := RateLimitKeyIP(clientIP)
	if h.limiter != nil && !h.limiter.Allow("session:"+rateKey, 5, time.Minute) {
		auditWarn("RATE_LIMIT", "action", "session", "ip", clientIP, "subnet", rateKey)
		w.Header().Set("Retry-After", "60")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "too many pairing session requests"})
		return
	}

	if !h.isAuthorizedAdmin(r) {
		auditWarn("AUTH_FAILURE", "action", "create_pairing_session", "ip", CleanIP(r.RemoteAddr))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		msg := "unauthorized: administrator token required (see ~/.multigravity/admin_token)"
		json.NewEncoder(w).Encode(map[string]string{"error": msg})
		return
	}

	session, err := h.pairingMgr.GenerateSession(DefaultPairingTTL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	h.mu.RLock()
	lanHost := h.lanHost
	primaryHost := h.host
	port := h.port
	ssl := h.ssl
	cfURL := h.cfURL
	origDDNSHost := h.ddnsHost
	origRelayURL := h.relayURL
	h.mu.RUnlock()
	relayHost := parseRelayHostname(origRelayURL, cfURL)

	var ddnsHost string
	if cfURL != "" {
		if u, err := url.Parse(cfURL); err == nil && u.Hostname() != "" {
			primaryHost = u.Hostname()
			port = 443
			ssl = true
			if u.Port() != "" {
				if p, err := strconv.Atoi(u.Port()); err == nil {
					port = p
				}
			}
			// Retain LAN host for dual-routing URI so mobile can pair over Wi-Fi or Cloudflare
			relayHost = ""
			ddnsHost = ""
		}
	} else if ssl {
		// Cert is issued for DDNS_HOST only; IP literals fail iOS ATS/trust.
		lanHost = ""
		ddnsHost = origDDNSHost
	} else {
		ddnsHost = origDDNSHost
	}

	uri := GenerateMultiHostPairingURI(MultiHostPairingParams{
		PrimaryHost: primaryHost,
		Port:        port,
		Code:        session.Code,
		SSL:         ssl,
		LANHost:     lanHost,
		DDNSHost:    ddnsHost,
		RelayHost:   relayHost,
		E2EEPubKey:  h.E2EEPubKey(),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":       session.Code,
		"expires_at": session.ExpiresAt,
		"uri":        uri,
	})

	// Also print QR code to gateway console/log
	var extraHosts []string
	if lanHost != "" && lanHost != primaryHost {
		extraHosts = append(extraHosts, lanHost)
	}
	if cfURL == "" {
		if ddnsHost != "" && ddnsHost != primaryHost {
			extraHosts = append(extraHosts, ddnsHost)
		}
		if relayHost != "" && relayHost != primaryHost {
			extraHosts = append(extraHosts, relayHost)
		}
	}
	PrintPairingQRCode(primaryHost, port, session.Code, ssl, extraHosts...)
}

// isAuthorizedAdmin checks if the request carries a valid admin token,
// or is genuinely from localhost (not behind a reverse proxy or tunnel).
func (h *AuthHandler) isAuthorizedAdmin(r *http.Request) bool {
	adminToken := strings.TrimSpace(GetAdminToken())
	if adminToken != "" {
		// Query-string admin tokens are rejected (they leak via logs/Referer).
		return ConstantTimeTokenEquals(BearerToken(r), adminToken)
	}

	policy := h.getPolicy()
	// When a tunnel is enabled, the connector reaches us from loopback, so a loopback
	// source address proves nothing: require the admin token instead.
	if policy.TunnelEnabled {
		return false
	}

	// Check if genuinely from localhost (not behind any reverse proxy or tunnel).
	if !IsProxiedRequest(r) && IsLoopbackAddr(r.RemoteAddr) && ImplicitTrustRequestOK(r) {
		return true
	}

	return false
}

// HandleWSTicket issues a short-lived (30s) one-time ticket for WebSocket connections.
// S9: Authenticated clients exchange their device token (via Authorization: Bearer or Cookie)
// for a single-use ticket, so the real token never appears in query strings.
func (h *AuthHandler) HandleWSTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	setJSONReadDeadline(w)

	clientIP := ExtractClientIP(r)
	rateKey := RateLimitKeyIP(clientIP)
	policy := h.getPolicy()
	proxied := IsProxiedRequest(r)

	// Check if genuinely from localhost (no pairing required) or trusted LAN, gated by
	// ImplicitTrustRequestOK and !IsProxiedRequest to defeat DNS rebinding, CSRF, and reverse-proxy spoofing.
	implicitOK := ImplicitTrustRequestOK(r)
	isLocalhost := implicitOK && IsLoopbackAddr(r.RemoteAddr) && !proxied
	isTrustedLAN := implicitOK && policy.TrustLAN && IsPrivateLANAddr(r.RemoteAddr) && !proxied

	if isLocalhost || isTrustedLAN {
		deviceID := "admin-local"
		if !isLocalhost {
			deviceID = "lan-" + clientIP
		}
		if h.wsTicketLimiter != nil && !h.wsTicketLimiter.Allow("wsticket:"+deviceID, 60, time.Minute) {
			auditWarn("RATE_LIMIT", "action", "wsticket", "ip", clientIP, "device_id", deviceID)
			w.Header().Set("Retry-After", "60")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]string{"error": "too many ticket requests"})
			return
		}
		ticket, err := h.store.IssueWSTicket(deviceID)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to generate ticket"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"ticket":     ticket,
			"expires_in": 30,
		})
		return
	}

	if BearerToken(r) == "" && !CookieWriteCSRFOK(r) {
		auditWarn("CSRF_REJECTED", "action", "issue_wsticket", "ip", CleanIP(r.RemoteAddr), "origin", r.Header.Get("Origin"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "forbidden: cross-origin request rejected"})
		return
	}

	token := ExtractToken(r)
	if token == "" {
		// Unauthenticated callers use the public limiter so they can never fill wsTicketLimiter.
		if h.limiter != nil && !h.limiter.Allow("wsticket-unauth:"+rateKey, 30, time.Minute) {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]string{"error": "too many ticket requests"})
			return
		}
		auditWarn("AUTH_FAILURE", "action", "issue_wsticket", "reason", "missing_token", "ip", CleanIP(r.RemoteAddr))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized: missing token"})
		return
	}

	var issueDeviceID string
	if adminTok := GetAdminToken(); adminTok != "" && ConstantTimeTokenEquals(token, adminTok) {
		issueDeviceID = "admin-local"
	} else {
		dev, ok := h.store.ValidateToken(token)
		if !ok || dev == nil {
			if h.limiter != nil && !h.limiter.Allow("wsticket-unauth:"+rateKey, 30, time.Minute) {
				w.Header().Set("Retry-After", "60")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{"error": "too many ticket requests"})
				return
			}
			auditWarn("AUTH_FAILURE", "action", "issue_wsticket", "reason", "invalid_token", "ip", CleanIP(r.RemoteAddr))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized: invalid token"})
			return
		}
		issueDeviceID = dev.DeviceID
	}

	// MED-04: Rate-limit authenticated WS ticket issuance by deviceID in an isolated limiter
	// so unauthenticated endpoint/pairing floods never starve legitimate paired devices.
	if h.wsTicketLimiter != nil && !h.wsTicketLimiter.Allow("wsticket:dev:"+issueDeviceID, 60, time.Minute) {
		auditWarn("RATE_LIMIT", "action", "wsticket", "ip", clientIP, "device_id", issueDeviceID)
		w.Header().Set("Retry-After", "60")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "too many ticket requests"})
		return
	}

	ticket, err := h.store.IssueWSTicket(issueDeviceID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to generate ticket"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"ticket":     ticket,
		"expires_in": 30,
	})
}
