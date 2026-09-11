package notifier

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/inspector"
	"antigravity-mobile/internal/proxy"
)

func TestWatcherStateTransitions(t *testing.T) {
	var mu sync.Mutex
	statusVal := "CASCADE_RUN_STATUS_IDLE"
	stepCountVal := 10

	// 1. Mock Antigravity Language Server upstream
	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		curStatus := statusVal
		curSteps := stepCountVal
		mu.Unlock()

		if strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories") {
			w.Write([]byte(`{
				"trajectorySummaries": {
					"cas_test_1": {
						"summary": "State transition test session",
						"status": "` + curStatus + `",
						"stepCount": ` + jsonNumber(curSteps) + `,
						"lastModifiedTime": "2026-09-12T00:00:00Z"
					}
				}
			}`))
			return
		}

		if strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") {
			w.Write([]byte(`{
				"status": "` + curStatus + `",
				"trajectory": {
					"trajectoryId": "cas_test_1",
					"cascadeId": "cas_test_1",
					"steps": [
						{"type": "CORTEX_STEP_TYPE_USER_INPUT", "status": "CORTEX_STEP_STATUS_DONE"},
						{"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE", "status": "CORTEX_STEP_STATUS_DONE", "plannerResponse": {"response": "All done!"}}
					]
				}
			}`))
			return
		}

		http.NotFound(w, r)
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port

	// 2. Setup Mock Bark Server
	var barkRequestCount int
	var barkMu sync.Mutex
	mockBark := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		barkMu.Lock()
		barkRequestCount++
		barkMu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":200,"message":"success"}`))
	}))
	defer mockBark.Close()

	// 3. Initialize Proxy and Notifier
	p := proxy.NewProxy(inspector.NewInspector(10 * time.Second))
	p.SetTestUpstream(port, "test-token")

	notifCfg := config.NotificationConfig{
		Enabled:      true,
		BarkEndpoint: mockBark.URL,
	}
	notif := NewNotifier(notifCfg)
	watcher := NewWatcher(p, notif)

	// Step 1: Initial baseline sync with existing historical IDLE session
	running := watcher.scanOnce()
	if running != 0 {
		t.Errorf("expected 0 running tasks on baseline sync, got %d", running)
	}

	barkMu.Lock()
	if barkRequestCount != 0 {
		t.Errorf("expected 0 bark notifications on initial baseline sync, got %d", barkRequestCount)
	}
	barkMu.Unlock()

	// Step 2: Session transitions to RUNNING
	mu.Lock()
	statusVal = "CASCADE_RUN_STATUS_RUNNING"
	stepCountVal = 12
	mu.Unlock()

	running = watcher.scanOnce()
	if running != 1 {
		t.Errorf("expected 1 running task, got %d", running)
	}

	barkMu.Lock()
	if barkRequestCount != 0 {
		t.Errorf("expected 0 bark notifications while running without action, got %d", barkRequestCount)
	}
	barkMu.Unlock()

	// Wait for trajectory cache (250ms TTL) to expire so FetchTrajectoryDetails fetches the new IDLE state
	time.Sleep(300 * time.Millisecond)

	// Step 3: Session transitions from RUNNING -> IDLE (Task Completed!)
	mu.Lock()
	statusVal = "CASCADE_RUN_STATUS_IDLE"
	stepCountVal = 15
	mu.Unlock()

	running = watcher.scanOnce()
	if running != 0 {
		t.Errorf("expected 0 running tasks after completion, got %d", running)
	}

	barkMu.Lock()
	if barkRequestCount != 1 {
		t.Errorf("expected exactly 1 completion notification on RUNNING->IDLE transition, got %d", barkRequestCount)
	}
	barkMu.Unlock()

	// Step 4: Subsequent scan with session still IDLE (must not re-trigger!)
	running = watcher.scanOnce()
	if running != 0 {
		t.Errorf("expected 0 running tasks, got %d", running)
	}

	barkMu.Lock()
	if barkRequestCount != 1 {
		t.Errorf("expected still 1 notification after second idle scan, got %d", barkRequestCount)
	}
	barkMu.Unlock()
}

func jsonNumber(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
