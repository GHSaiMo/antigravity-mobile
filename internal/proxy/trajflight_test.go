package proxy

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTrajFlightGroupCoalescesConcurrentCalls(t *testing.T) {
	g := &trajFlightGroup{calls: make(map[string]*trajFlightCall)}
	var calls int32
	release := make(chan struct{})
	want := &upstreamTrajectoryResp{}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := g.do("c1", func() (*upstreamTrajectoryResp, error) {
				atomic.AddInt32(&calls, 1)
				<-release
				return want, nil
			})
			if err != nil || got != want {
				t.Errorf("unexpected result: %v %v", got, err)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("expected 1 upstream call, got %d", n)
	}
	if len(g.calls) != 0 {
		t.Fatalf("flight map not cleaned up")
	}
}
