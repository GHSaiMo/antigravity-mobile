package proxy

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// RPC multiplexing over a single WebSocket.
//
// The desktop workbench keeps ~6 long-lived streaming RPCs (Subscribe*/Watch*/*Stream) open. Over
// plain HTTP/1.1 a browser allows only 6 connections per host, so those streams alone exhaust the
// pool and every further request (opening the settings page, analytics, ...) queues forever.
// The injected desktop shim therefore tunnels streaming RPCs through this endpoint instead.
//
// Frame layout (binary): [type u8][stream id u32 BE][payload]
//
//	client -> server: muxOpen   payload = [json len u32 BE][json {"path","headers"}][request body]
//	                  muxCancel payload = empty
//	server -> client: muxHead   payload = json {"status","headers"}
//	                  muxChunk  payload = response bytes
//	                  muxEnd    payload = empty
//	                  muxErr    payload = error text
const (
	muxOpen   byte = 1
	muxCancel byte = 3
	muxHead   byte = 4
	muxChunk  byte = 5
	muxEnd    byte = 6
	muxErr    byte = 7

	muxWriteTimeout = 15 * time.Second
	muxMaxFrame     = 8 * 1024 * 1024
)

type muxOpenMeta struct {
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
}

type muxConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func (m *muxConn) send(typ byte, id uint32, payload []byte) error {
	buf := make([]byte, 5+len(payload))
	buf[0] = typ
	binary.BigEndian.PutUint32(buf[1:5], id)
	copy(buf[5:], payload)

	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	_ = m.conn.SetWriteDeadline(time.Now().Add(muxWriteTimeout))
	return m.conn.WriteMessage(websocket.BinaryMessage, buf)
}

// muxResponseWriter adapts a proxied RPC response onto mux frames.
type muxResponseWriter struct {
	mc     *muxConn
	id     uint32
	header http.Header
	wrote  bool
	failed bool
}

func (w *muxResponseWriter) Header() http.Header { return w.header }

func (w *muxResponseWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.wrote = true
	hdrs := make(map[string]string, len(w.header))
	for k, vv := range w.header {
		switch strings.ToLower(k) {
		case "content-length", "content-encoding", "transfer-encoding", "connection", "set-cookie":
			continue
		}
		hdrs[k] = strings.Join(vv, ", ")
	}
	payload, _ := json.Marshal(map[string]any{"status": status, "headers": hdrs})
	if err := w.mc.send(muxHead, w.id, payload); err != nil {
		w.failed = true
	}
}

func (w *muxResponseWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if w.failed {
		return 0, context.Canceled
	}
	if len(b) == 0 {
		return 0, nil
	}
	if err := w.mc.send(muxChunk, w.id, b); err != nil {
		w.failed = true
		return 0, err
	}
	return len(b), nil
}

// Flush is a no-op: every Write is already sent as its own frame.
func (w *muxResponseWriter) Flush() {}

// HandleRPCMux serves GET /gateway/rpc-mux (WebSocket). Authentication is enforced by the
// gateway auth middleware before the request reaches this handler.
func (p *Proxy) HandleRPCMux(w http.ResponseWriter, r *http.Request) {
	sanitizeWebSocketHeaders(r)
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("[RPCMux] WebSocket upgrade failed", "err", err)
		return
	}
	defer p.trackWSConn(conn)()
	defer conn.Close()
	conn.SetReadLimit(muxMaxFrame)

	mc := &muxConn{conn: conn}
	ctx, cancelAll := context.WithCancel(context.Background())
	defer cancelAll()

	var mu sync.Mutex
	cancels := make(map[uint32]context.CancelFunc)

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if len(msg) < 5 {
			continue
		}
		typ := msg[0]
		id := binary.BigEndian.Uint32(msg[1:5])
		payload := msg[5:]

		switch typ {
		case muxCancel:
			mu.Lock()
			if c, ok := cancels[id]; ok {
				c()
			}
			mu.Unlock()

		case muxOpen:
			if len(payload) < 4 {
				_ = mc.send(muxErr, id, []byte("bad open frame"))
				continue
			}
			jl := int(binary.BigEndian.Uint32(payload[:4]))
			if jl < 0 || 4+jl > len(payload) {
				_ = mc.send(muxErr, id, []byte("bad open frame"))
				continue
			}
			var meta muxOpenMeta
			if err := json.Unmarshal(payload[4:4+jl], &meta); err != nil {
				_ = mc.send(muxErr, id, []byte("bad open metadata"))
				continue
			}
			// Only the language-server RPC surface may be tunnelled.
			if !strings.HasPrefix(meta.Path, "/exa.language_server_pb.") &&
				!strings.HasPrefix(meta.Path, "/api/exa.language_server_pb.") {
				_ = mc.send(muxErr, id, []byte("path not allowed"))
				continue
			}
			body := append([]byte(nil), payload[4+jl:]...)

			reqCtx, cancel := context.WithCancel(r.Context())
			go func() {
				select {
				case <-ctx.Done():
					cancel()
				case <-reqCtx.Done():
				}
			}()
			mu.Lock()
			cancels[id] = cancel
			mu.Unlock()

			go func() {
				defer func() {
					// ReverseProxy aborts with http.ErrAbortHandler when the stream is cancelled
					// mid-copy; net/http normally swallows it, but this goroutine is ours.
					if rec := recover(); rec != nil && rec != http.ErrAbortHandler {
						slog.Error("[RPCMux] stream handler panic", "panic", rec)
					}
					cancel()
					mu.Lock()
					delete(cancels, id)
					mu.Unlock()
				}()
				req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, meta.Path, bytes.NewReader(body))
				if err != nil {
					_ = mc.send(muxErr, id, []byte(err.Error()))
					return
				}
				for k, v := range meta.Headers {
					switch strings.ToLower(k) {
					case "host", "cookie", "authorization", "content-length", "connection", "upgrade", "accept-encoding":
						continue
					}
					req.Header.Set(k, v)
				}
				req.RemoteAddr = r.RemoteAddr
				rw := &muxResponseWriter{mc: mc, id: id, header: make(http.Header)}
				p.ServeHTTP(rw, req)
				if !rw.wrote {
					rw.WriteHeader(http.StatusOK)
				}
				if reqCtx.Err() == nil {
					_ = mc.send(muxEnd, id, nil)
				}
			}()
		}
	}
}
