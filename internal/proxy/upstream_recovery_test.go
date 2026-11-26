package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
)

type dynamicDiscoverer struct {
	mu          sync.Mutex
	cur         *inspector.InstanceInfo
	nextOnForce *inspector.InstanceInfo
	reason      string
	scanCalls   int
	forceCalls  int
	listeners   []func(inspector.InstanceInfo)
}

func (d *dynamicDiscoverer) Current() *inspector.InstanceInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cur == nil {
		return nil
	}
	cp := *d.cur
	return &cp
}

func (d *dynamicDiscoverer) Scan() *inspector.InstanceInfo {
	d.mu.Lock()
	d.scanCalls++
	cur := d.cur
	d.mu.Unlock()
	return cur
}

func (d *dynamicDiscoverer) ForceScan() *inspector.InstanceInfo {
	d.mu.Lock()
	d.forceCalls++
	if d.nextOnForce != nil {
		info := *d.nextOnForce
		d.cur = &info
		d.reason = inspector.ReasonNone
		listeners := append([]func(inspector.InstanceInfo){}, d.listeners...)
		d.mu.Unlock()
		for _, fn := range listeners {
			fn(info)
		}
		return &info
	}
	d.mu.Unlock()
	return nil
}

func (d *dynamicDiscoverer) LastFailureReason() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.reason
}

func (d *dynamicDiscoverer) Start() {}
func (d *dynamicDiscoverer) Stop()  {}
func (d *dynamicDiscoverer) OnUpdate(fn func(inspector.InstanceInfo)) {
	d.mu.Lock()
	d.listeners = append(d.listeners, fn)
	d.mu.Unlock()
}

func TestCascadeMessages_Structured503AndOnDemandForceScanRecovery(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"trajectory":{"cascadeId":"cid-recover","steps":[]},"status":"CASCADE_RUN_STATUS_IDLE"}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer upstream.Close()
	u, _ := url.Parse(upstream.URL)
	port, _ := strconv.Atoi(u.Port())

	disc := &dynamicDiscoverer{reason: inspector.ReasonLSAccessDenied}
	p := NewProxy(disc)

	// 1. Disconnected: /gateway/cascade/messages invokes ForceScan and returns structured 503 with X-Mgy-Error + reason.
	req := httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId=cid-recover&limit=10", nil)
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("disconnected status = %d, want 503", rec.Code)
	}
	if got := rec.Header().Get(HeaderMgyError); got != ErrCodeUpstreamUnavailable {
		t.Fatalf("%s header = %q, want %q", HeaderMgyError, got, ErrCodeUpstreamUnavailable)
	}
	var errBody map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode 503 body: %v (%s)", err, rec.Body.String())
	}
	if errBody["code"] != ErrCodeUpstreamUnavailable || errBody["reason"] != inspector.ReasonLSAccessDenied {
		t.Fatalf("unexpected 503 payload: %+v", errBody)
	}
	if disc.forceCalls == 0 {
		t.Fatalf("expected on-demand ForceScan to be called when disconnected")
	}

	// 2. /gateway/status also reports upstream_reason while disconnected.
	stRec := httptest.NewRecorder()
	p.ServeHTTP(stRec, httptest.NewRequest(http.MethodGet, "/gateway/status", nil))
	var st GatewayStatus
	if err := json.Unmarshal(stRec.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode /gateway/status: %v", err)
	}
	if st.Status != "disconnected" || st.UpstreamReason != inspector.ReasonLSAccessDenied {
		t.Fatalf("unexpected /gateway/status: %+v", st)
	}

	// 3. Antigravity launches; next request triggers on-demand ForceScan and succeeds with 200 immediately.
	disc.mu.Lock()
	disc.nextOnForce = &inspector.InstanceInfo{
		PID:          12345,
		Port:         port,
		CSRFToken:    "tok",
		DiscoveredAt: time.Now(),
		IsHealthy:    true,
	}
	disc.mu.Unlock()

	rec2 := httptest.NewRecorder()
	p.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/gateway/cascade/messages?cascadeId=cid-recover&limit=10", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("recovered status = %d, want 200 (body=%s)", rec2.Code, rec2.Body.String())
	}

	// 4. When upstream later becomes unhealthy, updateUpstream clears activePort cleanly.
	p.updateUpstream(inspector.InstanceInfo{PID: 12345, Port: port, IsHealthy: false})
	if activePort, _ := p.ActiveUpstream(); activePort != 0 {
		t.Fatalf("expected activePort=0 after unhealthy update, got %d", activePort)
	}
}

func TestHandleRescanUsesForceScan(t *testing.T) {
	disc := &dynamicDiscoverer{reason: inspector.ReasonLSNotReady}
	p := NewProxy(disc)

	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/gateway/rescan", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("rescan HTTP status = %d, want 200", rec.Code)
	}
	if disc.forceCalls != 1 || disc.scanCalls != 0 {
		t.Fatalf("rescan should call ForceScan once (got force=%d scan=%d)", disc.forceCalls, disc.scanCalls)
	}
	var st GatewayStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode rescan response: %v", err)
	}
	if st.Status != "failed" || st.UpstreamReason != inspector.ReasonLSNotReady {
		t.Fatalf("unexpected rescan response: %+v", st)
	}
}
