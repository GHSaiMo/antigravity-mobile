package proxy

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
)

func newSnapshotProxy(t *testing.T) (*Proxy, int, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte(`{"trajectorySummaries":{"a":{"summary":"x"}}}`))
	}))
	t.Cleanup(srv.Close)
	_, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)
	p := &Proxy{mediumClient: srv.Client()}
	return p, port, &hits
}

func TestFetchAllTrajectoriesRawSharesAndInvalidates(t *testing.T) {
	p, port, hits := newSnapshotProxy(t)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := p.fetchAllTrajectoriesRaw(context.Background(), port, "tok"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if _, err := p.fetchAllTrajectoriesRaw(context.Background(), port, "tok"); err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(hits); n != 1 {
		t.Fatalf("expected 1 upstream call for concurrent+cached reads, got %d", n)
	}

	p.invalidateAllTrajectories()
	if _, err := p.fetchAllTrajectoriesRaw(context.Background(), port, "tok"); err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(hits); n != 2 {
		t.Fatalf("expected refetch after invalidate, got %d calls", n)
	}
}

func TestIsReadOnlyRPC(t *testing.T) {
	for path, want := range map[string]bool{
		"/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories": true,
		"/api/exa.language_server_pb.LanguageServerService/StartCascade":              false,
		"/api/exa.language_server_pb.LanguageServerService/DeleteCascadeTrajectory":   false,
	} {
		if got := isReadOnlyRPC(path); got != want {
			t.Errorf("%s: got %v want %v", path, got, want)
		}
	}
}
