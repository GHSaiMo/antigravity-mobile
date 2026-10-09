package proxy

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func dialWS(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	d := websocket.Dialer{EnableCompression: true, HandshakeTimeout: 5 * time.Second}
	c, _, err := d.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return c
}

// A payload far larger than the stream write buffer must arrive intact, with and without compression.
func TestStreamUpgraderLargeMessageRoundTrip(t *testing.T) {
	// Incompressible-ish and compressible payloads, both well over the 32KB write buffer.
	var rnd bytes.Buffer
	for i := 0; rnd.Len() < 1<<20; i++ {
		rnd.WriteString(strings.Repeat(string(rune('a'+i%26)), i%97+1))
		rnd.WriteString(time.Duration(i * 7919).String())
	}
	payloads := map[string][]byte{
		"compressible": bytes.Repeat([]byte(`{"type":"update","text":"hello"}`), 40000),
		"mixed":        rnd.Bytes(),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := streamUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for _, name := range []string{"compressible", "mixed"} {
			if err := c.WriteMessage(websocket.TextMessage, payloads[name]); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	c := dialWS(t, srv)
	defer c.Close()
	c.SetReadLimit(8 << 20)
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	for _, name := range []string{"compressible", "mixed"} {
		_, got, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("%s: read: %v", name, err)
		}
		if !bytes.Equal(got, payloads[name]) {
			t.Fatalf("%s: payload corrupted (got %d bytes, want %d)", name, len(got), len(payloads[name]))
		}
	}
}
