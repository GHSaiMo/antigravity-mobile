package proxy

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/localtls"

	"github.com/gorilla/websocket"
)

// allowedOrigins holds trusted origins loaded from MULTIGRAVITY_ALLOWED_ORIGINS (or ALLOWED_ORIGINS).
// Format: comma-separated list of origins, e.g. "https://my-ddns.example.com,https://[2001:db8::1]:58900"
var allowedOrigins []string

func init() {
	origins := os.Getenv("MULTIGRAVITY_ALLOWED_ORIGINS")
	if origins == "" {
		origins = os.Getenv("ALLOWED_ORIGINS")
	}
	if origins != "" {
		for _, o := range strings.Split(origins, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				allowedOrigins = append(allowedOrigins, strings.ToLower(o))
			}
		}
	}
}

// IsAllowedOrigin validates that an Origin header represents a trusted caller.
// It blocks Cross-Site WebSocket Hijacking (CSWSH) from malicious third-party websites.
func IsAllowedOrigin(origin string, requestHost string) bool {
	// Allow requests with no Origin header (native mobile apps, CLI, curl)
	if origin == "" {
		return true
	}

	u, err := url.Parse(origin)
	if err != nil {
		return false
	}

	originHost := strings.Trim(u.Hostname(), "[]")
	lowerOrigin := strings.ToLower(origin)

	// 1. Loopback addresses
	if originHost == "127.0.0.1" || originHost == "localhost" || originHost == "::1" {
		return true
	}

	// 2. Configured host / DDNS domains from environment
	var trustedHosts []string
	if dh := strings.TrimSpace(os.Getenv("DDNS_HOST")); dh != "" {
		trustedHosts = append(trustedHosts, strings.ToLower(dh))
	}
	if gh := strings.TrimSpace(os.Getenv("MULTIGRAVITY_HOST")); gh != "" {
		trustedHosts = append(trustedHosts, strings.ToLower(gh))
	} else if gh := strings.TrimSpace(os.Getenv("GATEWAY_HOST")); gh != "" {
		trustedHosts = append(trustedHosts, strings.ToLower(gh))
	}
	// S5: FRP_SERVER_ADDR intentionally excluded — it is a tunnel server address, not a
	// browser origin. Including it would allow CSWSH from any page on that domain.
	// Add it to ALLOWED_ORIGINS if truly needed.

	for _, th := range trustedHosts {
		if originHost == th || strings.HasSuffix(originHost, "."+th) {
			return true
		}
	}

	// 4. Check explicitly configured allowed origins (ALLOWED_ORIGINS env var)
	for _, allowed := range allowedOrigins {
		if lowerOrigin == allowed || strings.HasPrefix(lowerOrigin, allowed+":") {
			return true
		}
	}

	// 5. Match requestHost ONLY IF requestHost itself is a verified private/loopback IP or configured domain
	if requestHost != "" {
		reqHostname, _, err := net.SplitHostPort(requestHost)
		if err != nil {
			reqHostname = requestHost
		}
		reqHostname = strings.Trim(reqHostname, "[]")

		isSafeReqHost := false
		if reqHostname == "localhost" || reqHostname == "127.0.0.1" || reqHostname == "::1" {
			isSafeReqHost = true
		} else if ip := net.ParseIP(reqHostname); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
			isSafeReqHost = true
		} else {
			for _, th := range trustedHosts {
				if reqHostname == th || strings.HasSuffix(reqHostname, "."+th) {
					isSafeReqHost = true
					break
				}
			}
		}

		if isSafeReqHost {
			for _, scheme := range []string{"http://", "https://"} {
				if lowerOrigin == scheme+strings.ToLower(requestHost) {
					return true
				}
			}
		}
	}

	return false
}

// HasBrowserFingerprint detects whether an incoming HTTP request originates from a standard web browser.
// Standard web browsers compliant with RFC 6455 MUST include an Origin header during WebSocket handshakes.
func HasBrowserFingerprint(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") != "" || r.Header.Get("Sec-Fetch-Mode") != "" || r.Header.Get("Sec-Fetch-Dest") != "" {
		return true
	}
	ua := r.Header.Get("User-Agent")
	if strings.Contains(ua, "Mozilla/") && !strings.Contains(ua, "Antigravity") && !strings.Contains(ua, "CFNetwork") && !strings.Contains(ua, "Darwin") {
		return true
	}
	return false
}

// CheckWebSocketOrigin validates the Origin of an incoming WebSocket upgrade request.
func CheckWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		if HasBrowserFingerprint(r) {
			log.Printf("[WS] Rejected WebSocket connection: missing Origin header from browser client (host: %s, UA: %s)", r.Host, r.Header.Get("User-Agent"))
			return false
		}
		return true
	}
	allowed := IsAllowedOrigin(origin, r.Host)
	if !allowed {
		log.Printf("[WS] Rejected WebSocket connection from untrusted origin: %s (host: %s)", origin, r.Host)
	}
	return allowed
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  32768,
	WriteBufferSize: 32768,
	CheckOrigin:     CheckWebSocketOrigin,
}

// sanitizeWebSocketHeaders normalizes HTTP headers required for WebSocket upgrade.
// Reverse proxies (e.g. Cloudflare Tunnel, Nginx, ALB) or HTTP/2 gateways often:
// 1. Join duplicate Sec-WebSocket-Key headers into a comma-separated list (e.g. "key1, key2")
// 2. Omit or strip Sec-WebSocket-Key when converting from HTTP/2 Extended CONNECT (RFC 8441)
// 3. Add surrounding quotes or spaces
// This function ensures RFC 6455 compliance before passing the request to Gorilla WebSocket.
func sanitizeWebSocketHeaders(r *http.Request) {
	// 1. Normalize 'Connection' header to ensure it contains 'upgrade'
	conn := r.Header.Get("Connection")
	if !strings.Contains(strings.ToLower(conn), "upgrade") {
		r.Header.Set("Connection", "Upgrade")
	}

	// 2. Normalize 'Upgrade' header to ensure it contains 'websocket'
	up := r.Header.Get("Upgrade")
	if !strings.Contains(strings.ToLower(up), "websocket") {
		r.Header.Set("Upgrade", "websocket")
	}

	// 3. Normalize 'Sec-WebSocket-Version'
	if r.Header.Get("Sec-Websocket-Version") == "" {
		r.Header.Set("Sec-Websocket-Version", "13")
	}

	// 4. Normalize 'Sec-WebSocket-Key'
	// Extract candidate keys from all header entries and comma-separated tokens
	var validKey string
	for k, vv := range r.Header {
		if strings.EqualFold(k, "Sec-WebSocket-Key") {
			for _, rawHeader := range vv {
				for _, part := range strings.Split(rawHeader, ",") {
					candidate := strings.TrimSpace(part)
					candidate = strings.Trim(candidate, "\"")
					if decoded, err := base64.StdEncoding.DecodeString(candidate); err == nil && len(decoded) == 16 {
						validKey = candidate
						break
					}
				}
				if validKey != "" {
					break
				}
			}
		}
		if validKey != "" {
			break
		}
	}

	// If no valid 16-byte base64 key was found (e.g. stripped by an HTTP/2 proxy),
	// generate a compliant RFC 6455 16-byte nonce so the handshake completes cleanly.
	if validKey == "" {
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); err == nil {
			validKey = base64.StdEncoding.EncodeToString(nonce)
		} else {
			validKey = "dGhlIHNhbXBsZSBub25jZQ==" // fallback RFC 6455 example nonce
		}
	}

	// Remove any duplicate or unconventional cased keys and set standard canonical key
	var toDelete []string
	for k := range r.Header {
		if strings.EqualFold(k, "Sec-WebSocket-Key") {
			toDelete = append(toDelete, k)
		}
	}
	for _, k := range toDelete {
		delete(r.Header, k)
	}
	r.Header.Set("Sec-Websocket-Key", validKey)
}

// HandleWebSocket handles client WebSocket connections and proxies to upstream language_server.
func (p *Proxy) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	cur := p.insp.Current()
	if cur == nil || !cur.IsHealthy {
		http.Error(w, "Antigravity language_server unavailable", http.StatusServiceUnavailable)
		return
	}

	sanitizeWebSocketHeaders(r)

	// Upgrade client connection
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] Upgrade failed: %v", err)
		return
	}
	defer clientConn.Close()

	// Dial upstream language_server
	upstreamURL := fmt.Sprintf("wss://127.0.0.1:%d/connect-websocket", cur.Port)
	dialer := websocket.Dialer{
		TLSClientConfig:   localtls.ClientConfig(),
		NetDialTLSContext: localtls.DialTLSContext,
		HandshakeTimeout:  5 * time.Second,
	}

	reqHeader := make(http.Header)
	reqHeader.Set("x-codeium-csrf-token", cur.CSRFToken)

	upstreamConn, resp, err := dialer.Dial(upstreamURL, reqHeader)
	if err != nil {
		log.Printf("[WS] Upstream dial failed (%s): %v", upstreamURL, err)
		if resp != nil {
			log.Printf("[WS] Upstream response status: %d", resp.StatusCode)
		}
		clientConn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Upstream connection failed"))
		return
	}
	defer upstreamConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	const wsTimeout = 60 * time.Second
	const maxWSMessageSize = 32 * 1024 * 1024 // 32MB safety ceiling against memory exhaustion (M-3)

	// Set up read limits, deadlines and pong handlers for both sides
	clientConn.SetReadLimit(maxWSMessageSize)
	clientConn.SetReadDeadline(time.Now().Add(wsTimeout))
	clientConn.SetPongHandler(func(string) error {
		clientConn.SetReadDeadline(time.Now().Add(wsTimeout))
		return nil
	})
	upstreamConn.SetReadLimit(maxWSMessageSize)
	upstreamConn.SetReadDeadline(time.Now().Add(wsTimeout))
	upstreamConn.SetPongHandler(func(string) error {
		upstreamConn.SetReadDeadline(time.Now().Add(wsTimeout))
		return nil
	})

	// Active server-side heartbeat ping to keep connections alive through reverse proxies
	stopHeartbeat := make(chan struct{})
	defer close(stopHeartbeat)
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopHeartbeat:
				return
			case <-ticker.C:
				_ = clientConn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second))
				_ = upstreamConn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(5*time.Second))
			}
		}
	}()

	// Pump: Client -> Upstream
	go func() {
		defer wg.Done()
		defer upstreamConn.Close()
		for {
			msgType, data, err := clientConn.ReadMessage()
			if err != nil {
				break
			}
			clientConn.SetReadDeadline(time.Now().Add(wsTimeout))
			upstreamConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := upstreamConn.WriteMessage(msgType, data); err != nil {
				break
			}
		}
	}()

	// Pump: Upstream -> Client
	go func() {
		defer wg.Done()
		defer clientConn.Close()
		for {
			msgType, data, err := upstreamConn.ReadMessage()
			if err != nil {
				break
			}
			upstreamConn.SetReadDeadline(time.Now().Add(wsTimeout))
			clientConn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := clientConn.WriteMessage(msgType, data); err != nil {
				break
			}
		}
	}()

	wg.Wait()
}
