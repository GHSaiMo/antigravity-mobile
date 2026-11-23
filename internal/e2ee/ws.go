package e2ee

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSOpenFrame is the encrypted Frame 0 sent by an E2EE WebSocket client after connecting to `/api/v1/e2ee/ws`.
type WSOpenFrame struct {
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Enc     string            `json:"enc,omitempty"` // optional "flate" capability for compressed s2c WS frames
}

var e2eeWSUpgrader = websocket.Upgrader{
	ReadBufferSize:  32 << 10,
	WriteBufferSize: 32 << 10,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// HandleWS terminates an outer E2EE WebSocket (`GET /api/v1/e2ee/ws?d=<device_id>`),
// decrypts Frame 0 to authenticate & locate the real internal WebSocket route (`/gateway/events` or `/gateway/stream`),
// and pumps AES-256-GCM encrypted binary frames transparently in both directions.
func (h *Handler) HandleWS(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimSpace(r.URL.Query().Get("d"))
	if deviceID == "" {
		deviceID = strings.TrimSpace(r.Header.Get(HeaderDeviceID))
	}
	if deviceID == "" {
		writeE2EEError(w, http.StatusUnauthorized, "missing_e2ee_device")
		return
	}
	key, ok := h.LookupKey(deviceID)
	if !ok || len(key) != KeySize {
		writeE2EEError(w, http.StatusUnauthorized, "unknown_e2ee_device")
		return
	}
	if h.LocalWSAddr == nil {
		writeE2EEError(w, http.StatusServiceUnavailable, "ws_bridge_unconfigured")
		return
	}

	outerWS, err := e2eeWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer outerWS.Close()

	// 1. Read encrypted Frame 0 (open control frame) within 10s
	_ = outerWS.SetReadDeadline(time.Now().Add(10 * time.Second))
	mt, frame0, err := outerWS.ReadMessage()
	if err != nil || mt != websocket.BinaryMessage {
		_ = outerWS.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "missing_open_frame"))
		return
	}
	_ = outerWS.SetReadDeadline(time.Time{})

	openPlain, err := Decrypt(key, frame0, FrameAAD(deviceID, "ws-open"))
	if err != nil {
		_ = outerWS.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "e2ee_open_decrypt_failed"))
		return
	}

	var openReq WSOpenFrame
	if err := json.Unmarshal(openPlain, &openReq); err != nil ||
		!strings.HasPrefix(openReq.Path, "/") || strings.HasPrefix(openReq.Path, "//") {
		_ = outerWS.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "invalid_open_path"))
		return
	}

	// 2. Explicitly verify Bearer token inside decrypted Frame 0 BEFORE dialing loopback
	// (prevents a revoked device from abusing loopback implicit trust).
	// If the client authenticates via a short-lived one-time `?ticket=` query parameter,
	// loopback AuthMiddleware strictly validates and consumes it because X-Forwarded-Proto: https
	// disables loopback implicit trust.
	hasTicket := strings.Contains(openReq.Path, "ticket=")
	if h.ValidateToken != nil {
		rawToken := extractBearer(openReq.Headers)
		if rawToken != "" {
			tokDevID, valid := h.ValidateToken(rawToken)
			if !valid || tokDevID != deviceID {
				_ = outerWS.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "invalid_auth"))
				return
			}
		} else if !hasTicket {
			_ = outerWS.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "missing_auth"))
			return
		}
	}

	// 3. Dial internal gateway WebSocket route on loopback
	localAddr := strings.TrimSpace(h.LocalWSAddr())
	if localAddr == "" {
		_ = outerWS.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "no_local_addr"))
		return
	}
	innerHeaders := http.Header{}
	var subprotocols []string
	for k, v := range openReq.Headers {
		lower := strings.ToLower(strings.TrimSpace(k))
		if lower == "sec-websocket-protocol" {
			for _, sp := range strings.Split(v, ",") {
				if trimmed := strings.TrimSpace(sp); trimmed != "" {
					subprotocols = append(subprotocols, trimmed)
				}
			}
			continue
		}
		if lower == "host" || lower == "upgrade" || lower == "connection" ||
			strings.HasPrefix(lower, "sec-websocket-") {
			continue
		}
		innerHeaders.Set(k, v)
	}
	if cfIP := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cfIP != "" {
		innerHeaders.Set("CF-Connecting-IP", cfIP)
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		innerHeaders.Set("X-Forwarded-For", xff)
	}
	innerHeaders.Set("X-Forwarded-Proto", "https")

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Subprotocols: subprotocols}
	innerWS, resp, err := dialer.Dial("ws://"+localAddr+openReq.Path, innerHeaders)
	if err != nil {
		code := websocket.CloseInternalServerErr
		if resp != nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			code = websocket.ClosePolicyViolation
		}
		_ = outerWS.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(code, "upstream_ws_failed"))
		return
	}
	defer innerWS.Close()

	var outerWriteMu sync.Mutex
	var innerWriteMu sync.Mutex

	// Propagate WebSocket Ping keepalive to survive Cloudflare Edge timeouts
	stopPing := make(chan struct{})
	defer close(stopPing)
	go func() {
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopPing:
				return
			case <-ticker.C:
				outerWriteMu.Lock()
				_ = outerWS.SetWriteDeadline(time.Now().Add(10 * time.Second))
				pErr := outerWS.WriteMessage(websocket.PingMessage, nil)
				outerWriteMu.Unlock()
				if pErr != nil {
					_ = outerWS.Close()
					return
				}
			}
		}
	}()

	errCh := make(chan error, 2)

	// Client -> Local Gateway (decrypt)
	go func() {
		for {
			msgType, encData, rErr := outerWS.ReadMessage()
			if rErr != nil {
				errCh <- rErr
				return
			}
			if msgType != websocket.BinaryMessage {
				continue
			}
			plain, dErr := Decrypt(key, encData, FrameAAD(deviceID, "ws-c2s"))
			if dErr != nil || len(plain) < 1 {
				errCh <- dErr
				return
			}
			innerType := int(plain[0])
			if innerType != websocket.TextMessage && innerType != websocket.BinaryMessage {
				innerType = websocket.TextMessage
			}
			innerWriteMu.Lock()
			wErr := innerWS.WriteMessage(innerType, plain[1:])
			innerWriteMu.Unlock()
			if wErr != nil {
				errCh <- wErr
				return
			}
		}
	}()

	// Local Gateway -> Client (encrypt, with optional RFC 1951 raw deflate when negotiated in Frame 0)
	wsAllowFlate := headerContainsToken(openReq.Enc, "flate")
	go func() {
		for {
			msgType, rawData, rErr := innerWS.ReadMessage()
			if rErr != nil {
				errCh <- rErr
				return
			}
			var buf []byte
			if wsAllowFlate && len(rawData) >= compressMinBytes {
				if comp, ok := deflateRaw(rawData); ok {
					buf = make([]byte, 1+len(comp))
					buf[0] = byte(msgType) | wsCompressedFlag
					copy(buf[1:], comp)
				}
			}
			if buf == nil {
				buf = make([]byte, 1+len(rawData))
				buf[0] = byte(msgType)
				copy(buf[1:], rawData)
			}
			enc, eErr := Encrypt(key, buf, FrameAAD(deviceID, "ws-s2c"))
			if eErr != nil {
				errCh <- eErr
				return
			}
			outerWriteMu.Lock()
			_ = outerWS.SetWriteDeadline(time.Now().Add(15 * time.Second))
			wErr := outerWS.WriteMessage(websocket.BinaryMessage, enc)
			outerWriteMu.Unlock()
			if wErr != nil {
				errCh <- wErr
				return
			}
		}
	}()

	<-errCh
}

func extractBearer(headers map[string]string) string {
	for k, v := range headers {
		if strings.EqualFold(strings.TrimSpace(k), "Authorization") {
			v = strings.TrimSpace(v)
			if len(v) > 7 && strings.EqualFold(v[:7], "Bearer ") {
				return strings.TrimSpace(v[7:])
			}
		}
	}
	return ""
}
