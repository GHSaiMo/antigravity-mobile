package auth

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PairRequest is the payload sent by clients to pair.
type PairRequest struct {
	PairingCode string `json:"pairing_code"`
	DeviceName  string `json:"device_name"`
	Platform    string `json:"platform"`
}

// EndpointInfo represents an accessible network endpoint of the gateway.
type EndpointInfo struct {
	Type string `json:"type"` // "lan", "ipv6", "ddns", "primary"
	URL  string `json:"url"`  // e.g. "http://192.168.1.50:58900"
}

// PairResponse is the response returned upon successful pairing.
type PairResponse struct {
	DeviceID    string         `json:"device_id"`
	DeviceToken string         `json:"device_token"`
	Endpoints   []EndpointInfo `json:"endpoints,omitempty"`
}

// AuthHandler handles authentication and device management routes.
type AuthHandler struct {
	store      *AuthStore
	pairingMgr *PairingManager
	host       string
	port       int
	ssl        bool
	lanHost    string
	ipv6Host   string
	ddnsHost   string
	relayURL   string
	policy     AuthPolicy
	limiter    *RateLimiter
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(store *AuthStore, pairingMgr *PairingManager, host string, port int, ssl bool) *AuthHandler {
	return &AuthHandler{
		store:      store,
		pairingMgr: pairingMgr,
		limiter:    NewRateLimiter(),
		host:       host,
		port:       port,
		ssl:        ssl,
	}
}

// SetEndpoints sets discovered network hosts (LAN IPv4, IPv6, DDNS) for pairing responses.
func (h *AuthHandler) SetEndpoints(lanHost, ipv6Host, ddnsHost string) {
	h.lanHost = strings.TrimSpace(lanHost)
	h.ipv6Host = strings.TrimSpace(ipv6Host)
	h.ddnsHost = strings.TrimSpace(ddnsHost)
}

// SetRelayURL sets the cloud relay URL (e.g. from embedded FRP tunnel) for pairing responses.
func (h *AuthHandler) SetRelayURL(relayURL string) {
	h.relayURL = strings.TrimSpace(relayURL)
}

// relayHost extracts the hostname from the configured cloud relay URL.
func (h *AuthHandler) relayHost() string {
	raw := strings.TrimSpace(h.relayURL)
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
	h.policy = policy
}

// GetEndpoints returns candidate endpoint URLs for clients.
func (h *AuthHandler) GetEndpoints() []EndpointInfo {
	var endpoints []EndpointInfo
	scheme := "http://"
	if h.ssl {
		scheme = "https://"
	}

	// LAN / public IPv6 literals are only advertised for cleartext HTTP.
	// GATEWAY_SSL certs are issued for the domain (DDNS_HOST), not RFC1918 or raw IPv6.
	if !h.ssl {
		lan := h.lanHost
		if lan == "" && !strings.Contains(h.host, ":") && h.host != "" && h.host != "127.0.0.1" && h.host != "localhost" {
			lan = h.host
		}
		if lan != "" {
			endpoints = append(endpoints, EndpointInfo{
				Type: "lan",
				URL:  fmt.Sprintf("%s%s:%d", scheme, lan, h.port),
			})
		}

		ipv6 := h.ipv6Host
		if ipv6 == "" && strings.Contains(h.host, ":") {
			ipv6 = h.host
		}
		if ipv6 != "" {
			cleanV6 := strings.Trim(ipv6, "[]")
			endpoints = append(endpoints, EndpointInfo{
				Type: "ipv6",
				URL:  fmt.Sprintf("%s[%s]:%d", scheme, cleanV6, h.port),
			})
		}
	}

	// 3. DDNS / Custom Domain
	if h.ddnsHost != "" {
		endpoints = append(endpoints, EndpointInfo{
			Type: "ddns",
			URL:  fmt.Sprintf("%s%s:%d", scheme, h.ddnsHost, h.port),
		})
	}

	// 4. Cloud Relay (embedded FRP tunnel)
	if h.relayURL != "" {
		dup := false
		for _, ep := range endpoints {
			if ep.URL == h.relayURL {
				dup = true
				break
			}
		}
		if !dup {
			endpoints = append(endpoints, EndpointInfo{
				Type: "relay",
				URL:  h.relayURL,
			})
		}
	}

	// 5. Fallback primary if no other endpoints detected
	if len(endpoints) == 0 {
		hStr := h.host
		if hStr == "" {
			hStr = "127.0.0.1"
		}
		if strings.Contains(hStr, ":") && !strings.HasPrefix(hStr, "[") {
			hStr = fmt.Sprintf("[%s]", hStr)
		}
		endpoints = append(endpoints, EndpointInfo{
			Type: "primary",
			URL:  fmt.Sprintf("%s%s:%d", scheme, hStr, h.port),
		})
	}

	return endpoints
}

// HandlePair handles POST /api/v1/auth/pair.
func (h *AuthHandler) HandlePair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req PairRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid json body"})
		return
	}

	clientIP := ExtractClientIP(r)
	rateKey := RateLimitKeyIP(clientIP)
	if h.limiter != nil {
		if !h.limiter.Allow("pair:global", 40, time.Minute) || !h.limiter.Allow("pair:"+rateKey, 8, time.Minute) {
			log.Printf("[AUDIT:RATE_LIMIT] action=pair ip=%s subnet=%s", clientIP, rateKey)
			w.Header().Set("Retry-After", "60")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]string{"error": "too many pairing attempts"})
			return
		}
	}

	req.PairingCode = strings.TrimSpace(req.PairingCode)
	if req.PairingCode == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "pairing_code is required"})
		return
	}

	// Validate and consume code immediately (prevent replay)
	if !h.pairingMgr.ValidateAndConsume(req.PairingCode) {
		log.Printf("[AUDIT:PAIR_FAILURE] reason=invalid_or_expired_code ip=%s", CleanIP(r.RemoteAddr))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid or expired pairing code"})
		return
	}

	deviceID, deviceToken, err := GenerateDeviceCredentials()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to generate credentials"})
		return
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
	device := PairedDevice{
		DeviceID:   deviceID,
		DeviceName: deviceName,
		Platform:   platform,
		TokenHash:  HashToken(deviceToken),
		CreatedAt:  now,
		LastSeenAt: now,
		LastSeenIP: cleanIP,
	}

	if err := h.store.AddDevice(device); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to persist device"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     DeviceCookieName,
		Value:    deviceToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   h.ssl,
		MaxAge:   86400 * 400,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(PairResponse{
		DeviceID:    deviceID,
		DeviceToken: deviceToken,
		Endpoints:   h.GetEndpoints(),
	})
}

// HandleDevices handles GET /api/v1/devices and DELETE /api/v1/devices/{id}.
func (h *AuthHandler) HandleDevices(w http.ResponseWriter, r *http.Request) {
	if !h.isAuthorizedAdmin(r) {
		log.Printf("[AUDIT:AUTH_FAILURE] action=devices_management ip=%s path=%s", CleanIP(r.RemoteAddr), r.URL.Path)
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

	clientIP := ExtractClientIP(r)
	rateKey := RateLimitKeyIP(clientIP)
	if h.limiter != nil && !h.limiter.Allow("session:"+rateKey, 5, time.Minute) {
		log.Printf("[AUDIT:RATE_LIMIT] action=session ip=%s subnet=%s", clientIP, rateKey)
		w.Header().Set("Retry-After", "60")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "too many pairing session requests"})
		return
	}

	if !h.isAuthorizedAdmin(r) {
		log.Printf("[AUDIT:AUTH_FAILURE] action=create_pairing_session ip=%s", CleanIP(r.RemoteAddr))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		msg := "unauthorized"
		if h.policy.TunnelEnabled {
			msg = "unauthorized: FRP tunnel is enabled, send Authorization: Bearer <MULTIGRAVITY_ADMIN_TOKEN> (see ~/.multigravity/admin_token)"
		}
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

	relayHost := h.relayHost()
	lanHost, ipv6Host := h.lanHost, h.ipv6Host
	if h.ssl {
		// Cert is issued for DDNS_HOST only; IP literals fail iOS ATS/trust.
		lanHost, ipv6Host = "", ""
	}
	primaryHost := h.host
	if r.URL.Query().Get("prefer") == "ipv6" && ipv6Host != "" {
		primaryHost = ipv6Host
	}

	uri := GenerateMultiHostPairingURI(MultiHostPairingParams{
		PrimaryHost: primaryHost,
		Port:        h.port,
		Code:        session.Code,
		SSL:         h.ssl,
		LANHost:     lanHost,
		IPv6Host:    ipv6Host,
		DDNSHost:    h.ddnsHost,
		RelayHost:   relayHost,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":       session.Code,
		"expires_at": session.ExpiresAt,
		"uri":        uri,
	})

	// Also print QR code to gateway console/log
	var extraHosts []string
	if !h.ssl {
		if h.lanHost != "" && h.lanHost != h.host {
			extraHosts = append(extraHosts, h.lanHost)
		}
		if h.ipv6Host != "" && h.ipv6Host != h.host {
			extraHosts = append(extraHosts, h.ipv6Host)
		}
	}
	if h.ddnsHost != "" && h.ddnsHost != h.host {
		extraHosts = append(extraHosts, h.ddnsHost)
	}
	if relayHost != "" && relayHost != h.host {
		extraHosts = append(extraHosts, relayHost)
	}
	PrintPairingQRCode(h.host, h.port, session.Code, h.ssl, extraHosts...)
}

// isAuthorizedAdmin checks if the request carries a valid admin token,
// or is genuinely from localhost (not behind a reverse proxy or tunnel).
func (h *AuthHandler) isAuthorizedAdmin(r *http.Request) bool {
	adminToken := strings.TrimSpace(GetAdminToken())
	if adminToken != "" {
		// Query-string admin tokens are rejected (they leak via logs/Referer).
		return ConstantTimeTokenEquals(BearerToken(r), adminToken)
	}

	// Loopback fallback is incompatible with FRP/SSH tunnels: those dial 127.0.0.1,
	// so every remote client appears local. Never trust RemoteAddr when a tunnel is on.
	if h.policy.TunnelEnabled {
		return false
	}

	// Only trust RemoteAddr if NOT behind a reverse proxy.
	if r.Header.Get("X-Forwarded-For") == "" && r.Header.Get("X-Real-IP") == "" {
		if IsLoopbackAddr(r.RemoteAddr) {
			return true
		}
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

	// SEC-5 / M-4: Rate-limit ticket issuance to 60 per minute per IP / /64 subnet.
	clientIP := ExtractClientIP(r)
	rateKey := RateLimitKeyIP(clientIP)
	if h.limiter != nil && !h.limiter.Allow("wsticket:"+rateKey, 60, time.Minute) {
		log.Printf("[AUDIT:RATE_LIMIT] action=wsticket ip=%s subnet=%s", clientIP, rateKey)
		w.Header().Set("Retry-After", "60")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]string{"error": "too many ticket requests"})
		return
	}

	token := ExtractToken(r)
	if token == "" {
		log.Printf("[AUDIT:AUTH_FAILURE] action=issue_wsticket reason=missing_token ip=%s", CleanIP(r.RemoteAddr))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized: missing token"})
		return
	}

	dev, ok := h.store.ValidateToken(token)
	if !ok || dev == nil {
		log.Printf("[AUDIT:AUTH_FAILURE] action=issue_wsticket reason=invalid_token ip=%s", CleanIP(r.RemoteAddr))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized: invalid token"})
		return
	}

	ticket, err := h.store.IssueWSTicket(dev.DeviceID)
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
