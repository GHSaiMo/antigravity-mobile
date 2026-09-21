package auth

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultDeviceCookieMaxAge is the session cookie validity duration (30 days, reduced from 400 days - H-3).
const DefaultDeviceCookieMaxAge = 86400 * 30

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
	cfURL      string
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

// SetPrimary sets the primary network host, port, and SSL status.
func (h *AuthHandler) SetPrimary(host string, port int, ssl bool) {
	h.host = strings.TrimSpace(host)
	h.port = port
	h.ssl = ssl
}

// SetCloudflareURL sets the Cloudflare Tunnel HTTPS endpoint URL for pairing responses.
func (h *AuthHandler) SetCloudflareURL(cfURL string) {
	h.cfURL = strings.TrimSpace(cfURL)
}

// CloudflareURL returns the configured Cloudflare Tunnel HTTPS endpoint URL.
func (h *AuthHandler) CloudflareURL() string {
	return h.cfURL
}

// relayHost extracts the hostname from the configured cloud relay URL.
func (h *AuthHandler) relayHost() string {
	raw := strings.TrimSpace(h.relayURL)
	if raw == "" {
		raw = strings.TrimSpace(h.cfURL)
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
	h.policy = policy
}

// GetEndpoints returns candidate endpoint URLs for clients.
func (h *AuthHandler) GetEndpoints() []EndpointInfo {
	var endpoints []EndpointInfo
	scheme := "http://"
	if h.ssl {
		scheme = "https://"
	}

	// 1. Cloudflare Tunnel (Assigned HTTPS domain) - Top Priority
	if h.cfURL != "" {
		endpoints = append(endpoints, EndpointInfo{
			Type: "cloudflare",
			URL:  h.cfURL,
		})
		lan := h.lanHost
		if lan == "" && !strings.Contains(h.host, ":") && h.host != "" && h.host != "127.0.0.1" && h.host != "localhost" && !strings.Contains(h.host, ".") {
			lan = h.host
		}
		if lan != "" {
			lanPort := h.port
			if h.ssl && h.port == 443 {
				lanPort = 58900
			}
			endpoints = append(endpoints, EndpointInfo{
				Type: "lan",
				URL:  fmt.Sprintf("http://%s:%d", lan, lanPort),
			})
		}
		// On the Cloudflare branch, IPv6 literals, DDNS, and legacy FRP relays are retired.
		return endpoints
	}

	// 2. LAN IPv4 - Advertised when local gateway is running cleartext HTTP (i.e. not self-TLS, or Cloudflare termination)
	if !h.ssl {
		lan := h.lanHost
		if lan == "" && !strings.Contains(h.host, ":") && h.host != "" && h.host != "127.0.0.1" && h.host != "localhost" && !strings.Contains(h.host, ".") {
			lan = h.host
		}
		if lan != "" {
			lanPort := h.port
			if h.ssl && h.port == 443 {
				lanPort = 58900
			}
			endpoints = append(endpoints, EndpointInfo{
				Type: "lan",
				URL:  fmt.Sprintf("http://%s:%d", lan, lanPort),
			})
		}
	}

	// LAN / public IPv6 literals are only advertised for cleartext HTTP.
	// GATEWAY_SSL certs are issued for the domain (DDNS_HOST), not RFC1918 or raw IPv6.
	if !h.ssl {
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

// HandleEndpoints handles GET /api/v1/auth/endpoints to return candidate connection endpoints.
func (h *AuthHandler) HandleEndpoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if cf := h.CloudflareURL(); cf != "" {
		w.Header().Set("X-Antigravity-Cloud-URL", cf)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"endpoints": h.GetEndpoints(),
	})
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
		MaxAge:   DefaultDeviceCookieMaxAge,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(PairResponse{
		DeviceID:    deviceID,
		DeviceToken: deviceToken,
		Endpoints:   h.GetEndpoints(),
	})
}

// HandleUnpair handles POST /api/v1/auth/unpair.
// It allows a paired device to revoke its own pairing, or an admin to unpair a specified device.
func (h *AuthHandler) HandleUnpair(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

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
			log.Printf("[AUDIT:UNPAIR_SUCCESS] admin unpair device_id=%s ip=%s", targetID, CleanIP(r.RemoteAddr))
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
		log.Printf("[AUDIT:AUTH_FAILURE] action=unpair reason=missing_token ip=%s", CleanIP(r.RemoteAddr))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized: missing authentication token"})
		return
	}

	device, ok := h.store.ValidateToken(token)
	if !ok || device == nil {
		log.Printf("[AUDIT:AUTH_FAILURE] action=unpair reason=invalid_token ip=%s", CleanIP(r.RemoteAddr))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid or expired token"})
		return
	}

	// If a specific targetID was provided, ensure it matches caller's deviceID unless caller is admin
	if targetID != "" && targetID != device.DeviceID && !h.isAuthorizedAdmin(r) {
		log.Printf("[AUDIT:AUTH_FAILURE] action=unpair reason=forbidden_target_mismatch device_id=%s target=%s ip=%s",
			device.DeviceID, targetID, CleanIP(r.RemoteAddr))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "forbidden: cannot unpair another device"})
		return
	}

	deviceIDToRemove := device.DeviceID
	if err := h.store.RemoveDevice(deviceIDToRemove); err != nil {
		log.Printf("[AUDIT:UNPAIR_ERROR] device_id=%s err=%v", deviceIDToRemove, err)
	} else {
		log.Printf("[AUDIT:UNPAIR_SUCCESS] device_id=%s ip=%s", deviceIDToRemove, CleanIP(r.RemoteAddr))
	}

	// Clear session cookie if set
	http.SetCookie(w, &http.Cookie{
		Name:     DeviceCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   h.ssl,
		MaxAge:   -1,
	})

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

	relayHost := h.relayHost()
	lanHost, ipv6Host := h.lanHost, h.ipv6Host
	primaryHost := h.host
	port := h.port
	ssl := h.ssl

	var ddnsHost string
	if h.cfURL != "" {
		if u, err := url.Parse(h.cfURL); err == nil && u.Hostname() != "" {
			primaryHost = u.Hostname()
			port = 443
			ssl = true
			if u.Port() != "" {
				if p, err := strconv.Atoi(u.Port()); err == nil {
					port = p
				}
			}
			// Retain LAN host for dual-routing URI so mobile can pair over Wi-Fi or Cloudflare
			if h.lanHost != "" {
				lanHost = h.lanHost
			}
			// In unified Cloudflare tunnel mode, IPv6 literals, FRP relays, and DDNS are retired.
			ipv6Host = ""
			relayHost = ""
			ddnsHost = ""
		}
	} else if h.ssl {
		// Cert is issued for DDNS_HOST only; IP literals fail iOS ATS/trust.
		lanHost, ipv6Host = "", ""
		ddnsHost = h.ddnsHost
	} else {
		ddnsHost = h.ddnsHost
		if (r.URL.Query().Get("prefer") == "ipv6" || primaryHost == "" || primaryHost == "127.0.0.1" || primaryHost == lanHost) && ipv6Host != "" {
			primaryHost = ipv6Host
		}
	}

	uri := GenerateMultiHostPairingURI(MultiHostPairingParams{
		PrimaryHost: primaryHost,
		Port:        port,
		Code:        session.Code,
		SSL:         ssl,
		LANHost:     lanHost,
		IPv6Host:    ipv6Host,
		DDNSHost:    ddnsHost,
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
	if lanHost != "" && lanHost != primaryHost {
		extraHosts = append(extraHosts, lanHost)
	}
	if h.cfURL == "" {
		if ipv6Host != "" && ipv6Host != primaryHost {
			extraHosts = append(extraHosts, ipv6Host)
		}
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

	// Loopback fallback is incompatible with tunnels: remote clients dial localhost via the tunnel daemon.
	// Never trust RemoteAddr when a tunnel is on without an explicit admin token.
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
