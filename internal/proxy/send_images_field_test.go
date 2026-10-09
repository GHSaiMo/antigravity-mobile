package proxy

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
)

type sendRecorder struct {
	mu     sync.Mutex
	bodies []map[string]any
}

func newSendTestProxy(t *testing.T) (*Proxy, *sendRecorder) {
	t.Helper()
	rec := &sendRecorder{}
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/SendUserCascadeMessage") {
			b, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			rec.mu.Lock()
			rec.bodies = append(rec.bodies, m)
			rec.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{}"))
	}))
	t.Cleanup(upstream.Close)
	p := NewProxy(inspector.NewInspector(5 * time.Second))
	p.updateUpstream(inspector.InstanceInfo{Port: upstream.Listener.Addr().(*net.TCPAddr).Port, CSRFToken: "t", IsHealthy: true})
	return p, rec
}

func sendJSON(t *testing.T, p *Proxy, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	p.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("send failed: %d %s", rr.Code, rr.Body.String())
	}
}

func TestSendDropsLegacyImagesWhenMediaPresent(t *testing.T) {
	p, rec := newSendTestProxy(t)
	sendJSON(t, p, `{"cascadeId":"c-img-both","items":[{"text":"look"}],`+
		`"images":[{"base64Data":"QUFBQQ==","mimeType":"image/jpeg"}],`+
		`"media":[{"inlineData":"QUFBQQ==","mimeType":"image/jpeg"}]}`)
	if len(rec.bodies) != 1 {
		t.Fatalf("expected 1 upstream send, got %d", len(rec.bodies))
	}
	got := rec.bodies[0]
	if _, ok := got["images"]; ok {
		t.Error("legacy images must be dropped when media is present")
	}
	if media, ok := got["media"].([]any); !ok || len(media) != 1 {
		t.Errorf("media must be forwarded, got %v", got["media"])
	}
}

func TestSendKeepsLegacyImagesWithoutMedia(t *testing.T) {
	p, rec := newSendTestProxy(t)
	sendJSON(t, p, `{"cascadeId":"c-img-legacy","items":[{"text":"look"}],"images":[{"base64Data":"QkJCQg==","mimeType":"image/jpeg"}]}`)
	if len(rec.bodies) != 1 {
		t.Fatalf("expected 1 upstream send, got %d", len(rec.bodies))
	}
	if imgs, ok := rec.bodies[0]["images"].([]any); !ok || len(imgs) != 1 {
		t.Errorf("images-only payloads must be forwarded unchanged, got %v", rec.bodies[0]["images"])
	}
}

// The short-window duplicate check must tell different media-only images apart, and still
// collapse a genuine double tap.
func TestSendDedupConsidersMediaImages(t *testing.T) {
	p, rec := newSendTestProxy(t)
	first := `{"cascadeId":"c-img-dedup","items":[{"text":"same text"}],"media":[{"inlineData":"Q0NDQw==","mimeType":"image/jpeg"}]}`
	second := `{"cascadeId":"c-img-dedup","items":[{"text":"same text"}],"media":[{"inlineData":"RERERA==","mimeType":"image/jpeg"}]}`
	sendJSON(t, p, first)
	sendJSON(t, p, second)
	if len(rec.bodies) != 2 {
		t.Fatalf("two different images with the same text must both be sent, got %d", len(rec.bodies))
	}
	sendJSON(t, p, second)
	if len(rec.bodies) != 2 {
		t.Fatalf("an immediate repeat of the same image must be deduplicated, got %d sends", len(rec.bodies))
	}
}
