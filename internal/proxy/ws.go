package proxy

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// allowedOrigins holds trusted origins loaded from the ALLOWED_ORIGINS environment variable.
// Format: comma-separated list of origins, e.g. "https://my-ddns.example.com,https://[2001:db8::1]:58900"
var allowedOrigins []string

func init() {
	if origins := os.Getenv("ALLOWED_ORIGINS"); origins != "" {
		for _, o := range strings.Split(origins, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				allowedOrigins = append(allowedOrigins, strings.ToLower(o))
			}
		}
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  32768,
	WriteBufferSize: 32768,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		// Allow requests with no Origin header (native apps, curl, same-origin navigations)
		if origin == "" {
			return true
		}
		// Validate origin against trusted loopback/localhost patterns
		lower := strings.ToLower(origin)
		for _, prefix := range []string{
			"http://127.0.0.1", "https://127.0.0.1",
			"http://[::1]", "https://[::1]",
			"http://localhost", "https://localhost",
		} {
			if lower == prefix || strings.HasPrefix(lower, prefix+":") {
				return true
			}
		}
		// Also allow if Origin matches the request's Host (same-origin via tunnel/proxy)
		if r.Host != "" {
			for _, scheme := range []string{"http://", "https://"} {
				if lower == scheme+strings.ToLower(r.Host) {
					return true
				}
			}
		}
		// Check against configured allowed origins (from ALLOWED_ORIGINS env var)
		for _, allowed := range allowedOrigins {
			if lower == allowed || strings.HasPrefix(lower, allowed+":") {
				return true
			}
		}
		log.Printf("[WS] Rejected WebSocket connection from untrusted origin: %s", origin)
		return false
	},
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
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
		HandshakeTimeout: 5 * time.Second,
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

	// Set up read deadlines and pong handlers for both sides
	clientConn.SetReadDeadline(time.Now().Add(wsTimeout))
	clientConn.SetPongHandler(func(string) error {
		clientConn.SetReadDeadline(time.Now().Add(wsTimeout))
		return nil
	})
	upstreamConn.SetReadDeadline(time.Now().Add(wsTimeout))
	upstreamConn.SetPongHandler(func(string) error {
		upstreamConn.SetReadDeadline(time.Now().Add(wsTimeout))
		return nil
	})

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
