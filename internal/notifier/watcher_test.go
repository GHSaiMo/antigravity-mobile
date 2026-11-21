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

	// 2. Setup Mock Push Sender
	sender := &mockSender{}

	// 3. Initialize Proxy and Notifier
	p := proxy.NewProxy(inspector.NewInspector(10 * time.Second))
	p.SetTestUpstream(port, "test-token")

	notif := NewNotifierWithSender(config.NotificationConfig{Enabled: true}, sender)
	watcher := NewWatcher(p, notif)

	// Step 1: Initial baseline sync with existing historical IDLE session
	running := watcher.scanOnce()
	if running != 0 {
		t.Errorf("expected 0 running tasks on baseline sync, got %d", running)
	}

	if sender.count.Load() != 0 {
		t.Errorf("expected 0 notifications on initial baseline sync, got %d", sender.count.Load())
	}

	// Step 2: Session transitions to RUNNING
	mu.Lock()
	statusVal = "CASCADE_RUN_STATUS_RUNNING"
	stepCountVal = 12
	mu.Unlock()

	running = watcher.scanOnce()
	if running != 1 {
		t.Errorf("expected 1 running task, got %d", running)
	}

	if sender.count.Load() != 0 {
		t.Errorf("expected 0 notifications while running without action, got %d", sender.count.Load())
	}

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

	if sender.count.Load() != 1 {
		t.Errorf("expected exactly 1 completion notification on RUNNING->IDLE transition, got %d", sender.count.Load())
	}

	// Step 4: Subsequent scan with session still IDLE (must not re-trigger!)
	running = watcher.scanOnce()
	if running != 0 {
		t.Errorf("expected 0 running tasks, got %d", running)
	}

	if sender.count.Load() != 1 {
		t.Errorf("expected still 1 notification after second idle scan, got %d", sender.count.Load())
	}
}

func TestWatcherTransitionalAndBackgroundTaskSuppression(t *testing.T) {
	var mu sync.Mutex
	statusVal := "CASCADE_RUN_STATUS_RUNNING"
	stepCountVal := 20
	currentResp := "正在调用 ego-browser 刷新雪球有效会话并加载最新 WAF 凭证..."
	hasRunningTask := true

	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		curStatus := statusVal
		curSteps := stepCountVal
		curResp := currentResp
		curRunning := hasRunningTask
		mu.Unlock()

		if strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories") {
			w.Write([]byte(`{
				"trajectorySummaries": {
					"cas_test_2": {
						"summary": "Transitional suppression test",
						"status": "` + curStatus + `",
						"stepCount": ` + jsonNumber(curSteps) + `,
						"lastModifiedTime": "2026-09-12T00:00:00Z"
					}
				}
			}`))
			return
		}

		if strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") {
			stepStatus := "CORTEX_STEP_STATUS_DONE"
			taskDetailsJSON := ""
			if curRunning {
				stepStatus = "CORTEX_STEP_STATUS_RUNNING"
				taskDetailsJSON = `, "taskDetails": {"id": "task-238", "description": "ego-browser nodejs -e '...'"} `
			}

			w.Write([]byte(`{
				"status": "` + curStatus + `",
				"trajectory": {
					"trajectoryId": "cas_test_2",
					"cascadeId": "cas_test_2",
					"steps": [
						{"type": "CORTEX_STEP_TYPE_USER_INPUT", "status": "CORTEX_STEP_STATUS_DONE"},
						{"type": "CORTEX_STEP_TYPE_RUN_COMMAND", "status": "` + stepStatus + `"` + taskDetailsJSON + `},
						{"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE", "status": "CORTEX_STEP_STATUS_DONE", "plannerResponse": {"response": "` + curResp + `"}}
					]
				}
			}`))
			return
		}

		http.NotFound(w, r)
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port

	sender := &mockSender{}

	p := proxy.NewProxy(inspector.NewInspector(10 * time.Second))
	p.SetTestUpstream(port, "test-token")

	notif := NewNotifierWithSender(config.NotificationConfig{Enabled: true}, sender)
	watcher := NewWatcher(p, notif)

	// Step 1: Baseline sync
	watcher.scanOnce()

	// Step 2: Transition from RUNNING -> IDLE while background task is still RUNNING and message is transitional
	time.Sleep(300 * time.Millisecond) // expire cache
	mu.Lock()
	statusVal = "CASCADE_RUN_STATUS_IDLE"
	stepCountVal = 21
	mu.Unlock()

	running := watcher.scanOnce()
	if running == 0 {
		t.Errorf("expected runningCount > 0 due to active background task, got %d", running)
	}

	// Verify notification was SUPPRESSED!
	if sender.count.Load() != 0 {
		t.Errorf("expected 0 notifications while background task is running, got %d", sender.count.Load())
	}

	// Step 3: Background task completes! Agent wakes up and provides final response
	time.Sleep(300 * time.Millisecond) // expire cache
	mu.Lock()
	hasRunningTask = false
	currentResp = "数据已更新完毕，共扫描 50 个组合，其中 3 个净值发生变化。"
	stepCountVal = 23
	mu.Unlock()

	running = watcher.scanOnce()
	if running != 0 {
		t.Errorf("expected runningCount == 0 after real completion, got %d", running)
	}

	// Verify notification was DELIVERED upon genuine completion!
	if sender.count.Load() != 1 {
		t.Errorf("expected exactly 1 notification upon genuine completion, got %d", sender.count.Load())
	}

	// Step 4: Next scan tick (session still IDLE) must not duplicate
	running = watcher.scanOnce()
	if sender.count.Load() != 1 {
		t.Errorf("expected still 1 notification, got %d", sender.count.Load())
	}
}

func jsonNumber(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestWatcherTransientFetchFailureRetry(t *testing.T) {
	var mu sync.Mutex
	statusVal := "CASCADE_RUN_STATUS_RUNNING"
	stepCountVal := 5
	fetchFailCount := 1 // Simulate 1 transient failure on trajectory fetch

	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		curStatus := statusVal
		curSteps := stepCountVal
		fail := fetchFailCount > 0
		if strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") && fail {
			fetchFailCount--
			mu.Unlock()
			http.Error(w, "transient error", http.StatusInternalServerError)
			return
		}
		mu.Unlock()

		if strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories") {
			w.Write([]byte(`{
				"trajectorySummaries": {
					"cas_retry_1": {
						"summary": "Retry test session",
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
					"trajectoryId": "cas_retry_1",
					"cascadeId": "cas_retry_1",
					"steps": [
						{"type": "CORTEX_STEP_TYPE_USER_INPUT", "status": "CORTEX_STEP_STATUS_DONE"},
						{"type": "CORTEX_STEP_TYPE_PLANNER_RESPONSE", "status": "CORTEX_STEP_STATUS_DONE", "plannerResponse": {"response": "Success after retry!"}}
					]
				}
			}`))
			return
		}

		http.NotFound(w, r)
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port

	sender := &mockSender{}

	p := proxy.NewProxy(inspector.NewInspector(10 * time.Second))
	p.SetTestUpstream(port, "test-token")

	notif := NewNotifierWithSender(config.NotificationConfig{Enabled: true}, sender)
	watcher := NewWatcher(p, notif)

	// Step 1: Baseline sync with RUNNING
	watcher.scanOnce()

	// Step 2: Transition from RUNNING -> IDLE, but first fetch will fail
	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	statusVal = "CASCADE_RUN_STATUS_IDLE"
	stepCountVal = 6
	mu.Unlock()

	// First scan tick: fetch fails!
	watcher.scanOnce()

	if sender.count.Load() != 0 {
		t.Errorf("expected 0 notifications on failed fetch, got %d", sender.count.Load())
	}

	// Second scan tick: fetch succeeds! Notification MUST be delivered!
	time.Sleep(300 * time.Millisecond)
	watcher.scanOnce()

	if sender.count.Load() != 1 {
		t.Errorf("expected 1 notification delivered after retry, got %d", sender.count.Load())
	}
}

func TestWatcherQueueFullRetainsStateForRetry(t *testing.T) {
	var mu sync.Mutex
	statusVal := "CASCADE_RUN_STATUS_RUNNING"
	stepCountVal := 4

	mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mu.Lock()
		curStatus := statusVal
		curSteps := stepCountVal
		mu.Unlock()

		if strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories") {
			_, _ = w.Write([]byte(`{"trajectorySummaries":{"cas_qfull":{"summary":"Queue Full Test","status":"` + curStatus + `","stepCount":` + jsonNumber(curSteps) + `}}}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") {
			_, _ = w.Write([]byte(`{"status":"` + curStatus + `","trajectory":{"trajectoryId":"cas_qfull","cascadeId":"cas_qfull","steps":[{"type":"CORTEX_STEP_TYPE_PLANNER_RESPONSE","status":"CORTEX_STEP_STATUS_DONE","plannerResponse":{"response":"Finished!"}}]}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer mockUpstream.Close()

	port := mockUpstream.Listener.Addr().(*net.TCPAddr).Port
	sender := &mockSender{}
	p := proxy.NewProxy(inspector.NewInspector(10 * time.Second))
	p.SetTestUpstream(port, "test-token")
	notif := NewNotifierWithSender(config.NotificationConfig{Enabled: true}, sender)
	watcher := NewWatcher(p, notif)

	// Baseline sync while RUNNING
	watcher.scanOnce()

	// Simulate alertCh saturated (capacity 1, already occupied)
	fullCh := make(chan func(), 1)
	fullCh <- func() {}
	watcher.mu.Lock()
	watcher.alertCh = fullCh
	watcher.mu.Unlock()

	time.Sleep(300 * time.Millisecond)
	mu.Lock()
	statusVal = "CASCADE_RUN_STATUS_IDLE"
	stepCountVal = 6
	mu.Unlock()

	// Scan while queue is full -> must keep runningCount > 0 and NOT lose the transition!
	if running := watcher.scanOnce(); running == 0 {
		t.Fatalf("expected runningCount > 0 when transition alert was deferred due to full queue")
	}

	// Drain queue and run next tick -> completion alert must now enqueue and deliver!
	<-fullCh
	watcher.scanOnce()
	fn := <-fullCh
	fn()

	if sender.count.Load() != 1 {
		t.Fatalf("expected deferred completion notification to succeed on retry tick, got %d", sender.count.Load())
	}
}

