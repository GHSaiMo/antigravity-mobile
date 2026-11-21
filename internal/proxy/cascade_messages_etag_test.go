package proxy

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
)

func TestCascadeMessagesETagAnd304(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	const cid = "cascade-etag-test"
	var version atomic.Int32
	version.Store(1)

	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		v := version.Load()
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			steps := `{"type":"CORTEX_STEP_TYPE_USER_INPUT","status":"CORTEX_STEP_STATUS_DONE","userInput":{"items":[{"text":"hello"}]}},` +
				`{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","status":"CORTEX_STEP_STATUS_DONE","plannerResponse":{"response":"world"}}`
			if v == 2 {
				steps += `,{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","status":"CORTEX_STEP_STATUS_DONE","plannerResponse":{"response":"updated"}}`
			}
			w.Write([]byte(`{"status":"CASCADE_RUN_STATUS_IDLE","trajectory":{"cascadeId":"` + cid + `","summary":"etag","steps":[` + steps + `]}}`))
		default:
			w.Write([]byte("{}"))
		}
	}))
	defer upstream.Close()
	defer ClearTrajectoryCache(cid)

	port := upstream.Listener.Addr().(*net.TCPAddr).Port
	p := NewProxy(&stubDiscoverer{info: &inspector.InstanceInfo{
		PID:          1001,
		Port:         port,
		CSRFToken:    "tok",
		DiscoveredAt: time.Now(),
		IsHealthy:    true,
	}})

	// 1. Initial GET returns 200 with strong ETag
	req1 := httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId="+cid+"&limit=15", nil)
	rec1 := httptest.NewRecorder()
	p.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("expected 200 on initial fetch, got %d", rec1.Code)
	}
	etag1 := rec1.Header().Get("ETag")
	if !strings.HasPrefix(etag1, `"mgy-`) {
		t.Fatalf("expected quoted mgy ETag, got %q", etag1)
	}

	// 2. Subsequent GET with matching strong ETag OR Cloudflare weak W/"..." ETag returns 304 with 0-byte body
	for _, hdr := range []string{etag1, "W/" + etag1} {
		req2 := httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId="+cid+"&limit=15", nil)
		req2.Header.Set("If-None-Match", hdr)
		rec2 := httptest.NewRecorder()
		p.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusNotModified {
			t.Fatalf("expected 304 for If-None-Match %q, got %d", hdr, rec2.Code)
		}
		if rec2.Body.Len() != 0 {
			t.Fatalf("expected empty body on 304, got %d bytes", rec2.Body.Len())
		}
	}

	// 3. Trajectory advances -> must return 200 with updated ETag
	version.Store(2)
	ClearTrajectoryCache(cid)
	req3 := httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId="+cid+"&limit=15", nil)
	req3.Header.Set("If-None-Match", "W/"+etag1)
	rec3 := httptest.NewRecorder()
	p.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected 200 after trajectory update, got %d", rec3.Code)
	}
	etag2 := rec3.Header().Get("ETag")
	if etag2 == "" || etag2 == etag1 {
		t.Fatalf("expected changed ETag after trajectory update, got %q vs %q", etag2, etag1)
	}
}
