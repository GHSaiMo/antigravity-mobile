package proxy

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:   32768,
	WriteBufferSize:  32768,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow mobile browsers & tunnel origins
	},
}

// HandleWebSocket handles client WebSocket connections and proxies to upstream language_server.
func (p *Proxy) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	cur := p.insp.Current()
	if cur == nil || !cur.IsHealthy {
		http.Error(w, "Antigravity language_server unavailable", http.StatusServiceUnavailable)
		return
	}

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

	// Pump: Client -> Upstream
	go func() {
		defer wg.Done()
		defer upstreamConn.Close()
		for {
			msgType, data, err := clientConn.ReadMessage()
			if err != nil {
				break
			}
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
			if err := clientConn.WriteMessage(msgType, data); err != nil {
				break
			}
		}
	}()

	wg.Wait()
}
