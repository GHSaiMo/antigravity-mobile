package hub

import (
	"compress/flate"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/proxy"

	"github.com/gorilla/websocket"
)

const (
	hubWSReadTimeout     = 60 * time.Second
	hubWSWriteTimeout    = 10 * time.Second
	hubWSPingInterval    = 20 * time.Second
	hubWSMaxMessageBytes = 32 * 1024 * 1024
)

var hubWSUpgrader = websocket.Upgrader{
	ReadBufferSize:    65536,
	WriteBufferSize:   65536,
	EnableCompression: true,
	CheckOrigin: func(r *http.Request) bool {
		return proxy.IsAllowedOrigin(r.Header.Get("Origin"), r.Host)
	},
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Upgrade")), "websocket")
}

// buildNodeWSURL converts an http(s) node base URL into a ws(s) URL, stripping client auth query parameters.
func buildNodeWSURL(baseURL, reqPath, rawQuery string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		u.Scheme = "wss"
	default:
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + reqPath

	if rawQuery != "" {
		q, err := url.ParseQuery(rawQuery)
		if err == nil {
			q.Del("ticket")
			q.Del("auth_token")
			q.Del("token")
			q.Del("mgy_node")
			u.RawQuery = q.Encode()
		} else {
			u.RawQuery = rawQuery
		}
	}
	return u.String(), nil
}

// bridgeWSToNode upgrades a client WebSocket connection and transparently bridges all frames
// bidirectionally to the target remote LAN node.
func (h *Hub) bridgeWSToNode(w http.ResponseWriter, r *http.Request, node NodeConfig, overridePath string) {
	reqPath := r.URL.Path
	if overridePath != "" {
		reqPath = overridePath
	}
	upstreamURL, err := buildNodeWSURL(node.BaseURL, reqPath, r.URL.RawQuery)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "invalid node ws url")
		return
	}

	dialHeader := make(http.Header)
	dialHeader.Set(HeaderHubForwarded, "1")
	if node.Token != "" {
		dialHeader.Set(HeaderHubToken, node.Token)
		dialHeader.Set("Authorization", "Bearer "+node.Token)
	}
	if proto := r.Header.Get("Sec-WebSocket-Protocol"); proto != "" {
		dialHeader.Set("Sec-WebSocket-Protocol", proto)
	}
	if al := strings.TrimSpace(r.Header.Get("Accept-Language")); al != "" {
		dialHeader.Set("Accept-Language", al)
	}

	dialer := websocket.Dialer{
		Proxy:             nil, // Direct LAN dial
		HandshakeTimeout:  5 * time.Second,
		ReadBufferSize:    65536,
		WriteBufferSize:   65536,
		EnableCompression: false,
	}

	upstreamConn, resp, err := dialer.DialContext(r.Context(), upstreamURL, dialHeader)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		h.markNodeError(node.ID, err)
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("node %s websocket dial failed: %v", node.Name, err))
		return
	}

	clientConn, err := hubWSUpgrader.Upgrade(w, r, nil)
	if err != nil {
		_ = upstreamConn.Close()
		slog.Warn("[HubWS] Client upgrade failed", "node", node.ID, "path", reqPath, "err", err)
		return
	}
	_ = clientConn.SetCompressionLevel(flate.BestSpeed)

	untrackClient := h.trackWS(clientConn)
	untrackUpstream := h.trackWS(upstreamConn)
	defer untrackClient()
	defer untrackUpstream()

	clientConn.SetReadLimit(hubWSMaxMessageBytes)
	upstreamConn.SetReadLimit(hubWSMaxMessageBytes)
	_ = clientConn.SetReadDeadline(time.Now().Add(hubWSReadTimeout))
	_ = upstreamConn.SetReadDeadline(time.Now().Add(hubWSReadTimeout))
	clientConn.SetPongHandler(func(string) error {
		return clientConn.SetReadDeadline(time.Now().Add(hubWSReadTimeout))
	})
	upstreamConn.SetPongHandler(func(string) error {
		return upstreamConn.SetReadDeadline(time.Now().Add(hubWSReadTimeout))
	})

	var clientWriteMu, upstreamWriteMu sync.Mutex
	stopPing := make(chan struct{})
	var closeOnce sync.Once
	closeBoth := func() {
		closeOnce.Do(func() {
			close(stopPing)
			_ = clientConn.Close()
			_ = upstreamConn.Close()
		})
	}
	defer closeBoth()

	go func() {
		ticker := time.NewTicker(hubWSPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopPing:
				return
			case <-ticker.C:
				clientWriteMu.Lock()
				_ = clientConn.SetWriteDeadline(time.Now().Add(hubWSWriteTimeout))
				err1 := clientConn.WriteMessage(websocket.PingMessage, nil)
				clientWriteMu.Unlock()

				upstreamWriteMu.Lock()
				_ = upstreamConn.SetWriteDeadline(time.Now().Add(hubWSWriteTimeout))
				err2 := upstreamConn.WriteMessage(websocket.PingMessage, nil)
				upstreamWriteMu.Unlock()

				if err1 != nil || err2 != nil {
					closeBoth()
					return
				}
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	// client -> upstream
	go func() {
		defer wg.Done()
		defer closeBoth()
		for {
			mt, msg, err := clientConn.ReadMessage()
			if err != nil {
				return
			}
			_ = clientConn.SetReadDeadline(time.Now().Add(hubWSReadTimeout))
			upstreamWriteMu.Lock()
			_ = upstreamConn.SetWriteDeadline(time.Now().Add(hubWSWriteTimeout))
			wErr := upstreamConn.WriteMessage(mt, msg)
			upstreamWriteMu.Unlock()
			if wErr != nil {
				return
			}
		}
	}()

	// upstream -> client
	go func() {
		defer wg.Done()
		defer closeBoth()
		for {
			mt, msg, err := upstreamConn.ReadMessage()
			if err != nil {
				return
			}
			_ = upstreamConn.SetReadDeadline(time.Now().Add(hubWSReadTimeout))
			clientWriteMu.Lock()
			_ = clientConn.SetWriteDeadline(time.Now().Add(hubWSWriteTimeout))
			wErr := clientConn.WriteMessage(mt, msg)
			clientWriteMu.Unlock()
			if wErr != nil {
				return
			}
		}
	}()

	wg.Wait()
}
