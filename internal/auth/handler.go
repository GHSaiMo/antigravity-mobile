package auth

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
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
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(store *AuthStore, pairingMgr *PairingManager, host string, port int, ssl bool) *AuthHandler {
	return &AuthHandler{
		store:      store,
		pairingMgr: pairingMgr,
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

// GetEndpoints returns candidate endpoint URLs for clients.
func (h *AuthHandler) GetEndpoints() []EndpointInfo {
	var endpoints []EndpointInfo
	scheme := "http://"
	if h.ssl {
		scheme = "https://"
	}

	// 1. LAN IPv4
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

	// 2. Public IPv6
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

	// 3. DDNS / Custom Domain
	if h.ddnsHost != "" {
		endpoints = append(endpoints, EndpointInfo{
			Type: "ddns",
			URL:  fmt.Sprintf("%s%s:%d", scheme, h.ddnsHost, h.port),
		})
	}

	// 4. Fallback primary if no other endpoints detected
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

// SetNetworkInfo updates the host, port, and ssl settings used for QR generation.
func (h *AuthHandler) SetNetworkInfo(host string, port int, ssl bool) {
	h.host = host
	h.port = port
	h.ssl = ssl
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

	req.PairingCode = strings.TrimSpace(req.PairingCode)
	if req.PairingCode == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "pairing_code is required"})
		return
	}

	// Validate and consume code immediately (prevent replay)
	if !h.pairingMgr.ValidateAndConsume(req.PairingCode) {
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
	cleanIP := CleanIP(r.RemoteAddr)
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
		if targetID == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "device_id is required"})
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
	if !h.isAuthorizedAdmin(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}

	session, err := h.pairingMgr.GenerateSession(DefaultPairingTTL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	uri := GenerateMultiHostPairingURI(MultiHostPairingParams{
		PrimaryHost: h.host,
		Port:        h.port,
		Code:        session.Code,
		SSL:         h.ssl,
		LANHost:     h.lanHost,
		IPv6Host:    h.ipv6Host,
		DDNSHost:    h.ddnsHost,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":       session.Code,
		"expires_at": session.ExpiresAt,
		"uri":        uri,
	})
}

// isLoopback checks whether the incoming address is from localhost.
func isLoopback(remoteAddr string) bool {
	clean := CleanIP(remoteAddr)
	ip := net.ParseIP(clean)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

// isAuthorizedAdmin checks if the request is from localhost or carries a valid device token.
func (h *AuthHandler) isAuthorizedAdmin(r *http.Request) bool {
	if isLoopback(r.RemoteAddr) {
		return true
	}
	token := ExtractToken(r)
	if token != "" {
		if _, ok := h.store.ValidateToken(token); ok {
			return true
		}
	}
	return false
}
