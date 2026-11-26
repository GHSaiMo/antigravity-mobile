package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"antigravity-mobile/internal/updater"
)

func TestGatewaySelfUpdate_StatusAndRefresh(t *testing.T) {
	p := &Proxy{}
	p.SetGatewayVersion("1.2.7")
	p.SetSelfRestartFunc(func(string) {}, "exec")
	p.selfUpdateCapFn = func(curVer string) updater.SelfUpdateCapability {
		return updater.SelfUpdateCapability{
			CanUpdate:  true,
			TargetPath: "/usr/local/bin/mgy",
			Dir:        "/usr/local/bin",
		}
	}
	var gotForce bool
	p.selfUpdateCheckFn = func(ctx context.Context, curVer string, force bool) (string, error) {
		gotForce = force
		return "v1.2.8", nil
	}

	// Normal status check
	rec := httptest.NewRecorder()
	p.HandleGatewaySelfUpdate(rec, httptest.NewRequest(http.MethodGet, "/gateway/self/update", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if gotForce {
		t.Fatalf("expected force=false without refresh param")
	}
	var st GatewaySelfUpdateStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if !st.OK || st.CurrentVersion != "1.2.7" || st.LatestVersion != "1.2.8" || !st.UpdateAvailable || !st.CanSelfUpdate || st.RestartMode != "exec" || st.State != "idle" {
		t.Fatalf("unexpected status: %+v", st)
	}

	// Force refresh query parameter
	rec = httptest.NewRecorder()
	p.HandleGatewaySelfUpdate(rec, httptest.NewRequest(http.MethodGet, "/gateway/self/update?refresh=1", nil))
	if rec.Code != http.StatusOK || !gotForce {
		t.Fatalf("expected refresh=1 to pass force=true, code=%d gotForce=%v", rec.Code, gotForce)
	}
}

func TestGatewaySelfUpdate_OfflineDegradesGracefully(t *testing.T) {
	p := &Proxy{}
	p.SetGatewayVersion("v1.2.7")
	p.selfUpdateCapFn = func(string) updater.SelfUpdateCapability {
		return updater.SelfUpdateCapability{CanUpdate: false, Reason: "目录无写入权限"}
	}
	p.selfUpdateCheckFn = func(context.Context, string, bool) (string, error) {
		return "", errors.New("network unreachable")
	}

	rec := httptest.NewRecorder()
	p.HandleGatewaySelfUpdate(rec, httptest.NewRequest(http.MethodGet, "/gateway/self/update", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 graceful degrade, got %d: %s", rec.Code, rec.Body.String())
	}
	var st GatewaySelfUpdateStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !st.OK || st.CurrentVersion != "1.2.7" || st.UpdateAvailable || st.CanSelfUpdate || st.CannotUpdateReason == "" || st.CheckError == "" {
		t.Fatalf("unexpected degraded status: %+v", st)
	}
}

func TestGatewaySelfUpdate_ApplyGuardsAndLifecycle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	p := &Proxy{}
	p.SetGatewayVersion("1.2.7")
	p.selfRestartDelay = 10 * time.Millisecond

	// 1. confirm=false rejected with 400
	rec := httptest.NewRecorder()
	p.HandleGatewaySelfUpdate(rec, httptest.NewRequest(http.MethodPost, "/gateway/self/update/apply", strings.NewReader(`{"confirm":false}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for confirm=false, got %d", rec.Code)
	}

	// 2. Non-writable binary dir rejected with 400
	p.selfUpdateCapFn = func(string) updater.SelfUpdateCapability {
		return updater.SelfUpdateCapability{CanUpdate: false, Reason: "目录 /usr/local/bin 无写入权限"}
	}
	rec = httptest.NewRecorder()
	p.HandleGatewaySelfUpdate(rec, httptest.NewRequest(http.MethodPost, "/gateway/self/update/apply", strings.NewReader(`{"confirm":true}`)))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "无写入权限") {
		t.Fatalf("expected 400 for non-writable dir, got %d %s", rec.Code, rec.Body.String())
	}

	// 3. Successful update progresses and triggers selfRestartFn
	p.selfUpdateCapFn = func(string) updater.SelfUpdateCapability {
		return updater.SelfUpdateCapability{CanUpdate: true, TargetPath: "/tmp/fake-mgy", Dir: "/tmp"}
	}
	var (
		mu             sync.Mutex
		restartedPath  string
		restartDone    = make(chan struct{}, 1)
		releaseRunGate = make(chan struct{})
	)
	p.SetSelfRestartFunc(func(bin string) {
		mu.Lock()
		restartedPath = bin
		mu.Unlock()
		select {
		case restartDone <- struct{}{}:
		default:
		}
	}, "exec")

	p.selfUpdateRunFn = func(ctx context.Context, opts updater.Options) (*updater.Result, error) {
		if opts.OnProgress != nil {
			opts.OnProgress("downloading", "正在下载 v1.2.8...")
		}
		<-releaseRunGate
		if opts.OnProgress != nil {
			opts.OnProgress("replacing", "正在替换网关程序...")
		}
		return &updater.Result{
			CurrentVersion:   "1.2.7",
			LatestVersion:    "1.2.8",
			Tag:              "v1.2.8",
			TargetPath:       "/tmp/fake-mgy",
			Updated:          true,
			ChecksumVerified: true,
		}, nil
	}

	rec = httptest.NewRecorder()
	p.HandleGatewaySelfUpdate(rec, httptest.NewRequest(http.MethodPost, "/gateway/self/update/apply", strings.NewReader(`{"confirm":true}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"accepted":true`) {
		t.Fatalf("expected 200 accepted, got %d %s", rec.Code, rec.Body.String())
	}

	// Wait briefly for goroutine to enter downloading stage and verify GET reports active state without hitting checkFn
	p.selfUpdateCheckFn = func(context.Context, string, bool) (string, error) {
		t.Errorf("checkLatestSelfTag must not be called while update job is active")
		return "", nil
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec = httptest.NewRecorder()
		p.HandleGatewaySelfUpdate(rec, httptest.NewRequest(http.MethodGet, "/gateway/self/update", nil))
		var st GatewaySelfUpdateStatus
		_ = json.Unmarshal(rec.Body.Bytes(), &st)
		if st.State == "downloading" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Duplicate POST while active is idempotent (200 accepted, no double run)
	rec = httptest.NewRecorder()
	p.HandleGatewaySelfUpdate(rec, httptest.NewRequest(http.MethodPost, "/gateway/self/update/apply", strings.NewReader(`{"confirm":true}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"accepted":true`) {
		t.Fatalf("expected idempotent 200 for duplicate apply, got %d %s", rec.Code, rec.Body.String())
	}

	close(releaseRunGate)
	select {
	case <-restartDone:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for selfRestartFn callback")
	}

	mu.Lock()
	gotBin := restartedPath
	mu.Unlock()
	if gotBin != "/tmp/fake-mgy" {
		t.Fatalf("expected restart binary /tmp/fake-mgy, got %q", gotBin)
	}
}

func TestGatewaySelfUpdate_FailureDoesNotRestart(t *testing.T) {
	p := &Proxy{}
	p.SetGatewayVersion("1.2.7")
	p.selfRestartDelay = 5 * time.Millisecond
	p.selfUpdateCapFn = func(string) updater.SelfUpdateCapability {
		return updater.SelfUpdateCapability{CanUpdate: true, TargetPath: "/tmp/fake-mgy", Dir: "/tmp"}
	}
	restarted := false
	p.SetSelfRestartFunc(func(string) { restarted = true }, "exec")
	p.selfUpdateRunFn = func(ctx context.Context, opts updater.Options) (*updater.Result, error) {
		return nil, errors.New("SHA-256 校验失败")
	}
	p.selfUpdateCheckFn = func(context.Context, string, bool) (string, error) {
		return "v1.2.8", nil
	}

	rec := httptest.NewRecorder()
	p.HandleGatewaySelfUpdate(rec, httptest.NewRequest(http.MethodPost, "/gateway/self/update/apply", strings.NewReader(`{"confirm":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 accepted, got %d", rec.Code)
	}

	deadline := time.Now().Add(2 * time.Second)
	var st GatewaySelfUpdateStatus
	for time.Now().Before(deadline) {
		rec = httptest.NewRecorder()
		p.HandleGatewaySelfUpdate(rec, httptest.NewRequest(http.MethodGet, "/gateway/self/update", nil))
		_ = json.Unmarshal(rec.Body.Bytes(), &st)
		if st.State == "failed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st.State != "failed" || !strings.Contains(st.Error, "SHA-256") {
		t.Fatalf("expected failed state with SHA-256 error, got %+v", st)
	}
	if restarted {
		t.Fatalf("selfRestartFn must not be invoked when update fails")
	}
}
