package proxy

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func muxFrameBytes(typ byte, id uint32, payload []byte) []byte {
	b := make([]byte, 5+len(payload))
	b[0] = typ
	binary.BigEndian.PutUint32(b[1:5], id)
	copy(b[5:], payload)
	return b
}

func muxOpenFrame(t *testing.T, id uint32, path string, body []byte) []byte {
	t.Helper()
	meta, _ := json.Marshal(muxOpenMeta{Path: path, Headers: map[string]string{"Content-Type": "application/connect+json"}})
	payload := make([]byte, 4, 4+len(meta)+len(body))
	binary.BigEndian.PutUint32(payload, uint32(len(meta)))
	payload = append(payload, meta...)
	payload = append(payload, body...)
	return muxFrameBytes(muxOpen, id, payload)
}

func TestHandleRPCMuxStreamsResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/connect+json")
		w.WriteHeader(http.StatusOK)
		fl := w.(http.Flusher)
		_, _ = w.Write([]byte("echo:" + string(b)))
		fl.Flush()
		_, _ = w.Write([]byte("|second"))
		fl.Flush()
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)

	p := &Proxy{wsConns: make(map[*websocket.Conn]struct{})}
	p.activeProxy = httputil.NewSingleHostReverseProxy(u)

	gw := httptest.NewServer(http.HandlerFunc(p.HandleRPCMux))
	defer gw.Close()

	wsURL := "ws" + strings.TrimPrefix(gw.URL, "http") + "/gateway/rpc-mux"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.BinaryMessage, muxOpenFrame(t, 7, "/exa.language_server_pb.LanguageServerService/SubscribeToPlugins", []byte("hi"))); err != nil {
		t.Fatalf("write: %v", err)
	}

	var (
		status int
		body   bytes.Buffer
		ended  bool
	)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for !ended {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if binary.BigEndian.Uint32(msg[1:5]) != 7 {
			t.Fatalf("unexpected stream id in frame")
		}
		switch msg[0] {
		case muxHead:
			var h struct {
				Status int `json:"status"`
			}
			if err := json.Unmarshal(msg[5:], &h); err != nil {
				t.Fatalf("head json: %v", err)
			}
			status = h.Status
		case muxChunk:
			body.Write(msg[5:])
		case muxEnd:
			ended = true
		case muxErr:
			t.Fatalf("mux error: %s", msg[5:])
		}
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
	if got := body.String(); got != "echo:hi|second" {
		t.Errorf("body = %q", got)
	}
}

func TestHandleRPCMuxRejectsForeignPath(t *testing.T) {
	p := &Proxy{wsConns: make(map[*websocket.Conn]struct{})}
	gw := httptest.NewServer(http.HandlerFunc(p.HandleRPCMux))
	defer gw.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(gw.URL, "http")+"/gateway/rpc-mux", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_ = conn.WriteMessage(websocket.BinaryMessage, muxOpenFrame(t, 1, "/gateway/projects", nil))
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if msg[0] != muxErr {
		t.Errorf("expected error frame, got type %d", msg[0])
	}
}
