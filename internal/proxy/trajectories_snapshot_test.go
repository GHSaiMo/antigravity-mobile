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

func TestCompactSummariesForWidget(t *testing.T) {
	in := map[string]map[string]interface{}{
		"active-1": {
			"summary":             "Fix widget skeleton",
			"stepCount":           12,
			"status":              "CASCADE_RUN_STATUS_RUNNING",
			"workspaceName":       "antigravity-mobile",
			"needsInput":          true,
			"trajectoryMetadata":  map[string]interface{}{"huge": "payload"},
			"latestNotifyUserStep": map[string]interface{}{"markdown": "big step"},
			"annotations": map[string]interface{}{
				"title":          "Widget Fix",
				"markedAsUnread": true,
				"extraBlob":      "drop-me",
			},
		},
		"archived-1": {
			"summary": "Old archived",
			"annotations": map[string]interface{}{
				"archived": true,
			},
		},
	}
	got := compactSummariesForWidget(in)
	if _, exists := got["archived-1"]; exists {
		t.Fatalf("expected archived session to be excluded")
	}
	item, ok := got["active-1"]
	if !ok {
		t.Fatalf("expected active-1 to be kept")
	}
	if _, leaked := item["trajectoryMetadata"]; leaked {
		t.Fatalf("expected trajectoryMetadata to be stripped")
	}
	if _, leaked := item["latestNotifyUserStep"]; leaked {
		t.Fatalf("expected latestNotifyUserStep to be stripped")
	}
	ann, _ := item["annotations"].(map[string]interface{})
	if ann["title"] != "Widget Fix" {
		t.Fatalf("expected title preserved, got %v", ann["title"])
	}
	if _, leaked := ann["extraBlob"]; leaked {
		t.Fatalf("expected extraBlob stripped from annotations")
	}
}
