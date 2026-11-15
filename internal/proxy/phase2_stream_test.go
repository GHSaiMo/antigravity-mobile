package proxy

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func writeConnectFrame(w http.ResponseWriter, flusher http.Flusher, payload interface{}) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	env := encodeConnectEnvelope(raw)
	if _, err := w.Write(env); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

func TestPhase2StreamSinceRevResume(t *testing.T) {
	oldLinger := agentStateStreamLinger
	agentStateStreamLinger = 0
	defer func() { agentStateStreamLinger = oldLinger }()

	var mu sync.Mutex
	respText := "Hello world"
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory") {
			mu.Lock()
			txt := respText
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "CASCADE_RUN_STATUS_IDLE",
				"trajectory": map[string]interface{}{
					"cascadeId": "resume-cid",
					"steps": []map[string]interface{}{
						{
							"type":   "CORTEX_STEP_TYPE_USER_INPUT",
							"status": "CORTEX_STEP_STATUS_DONE",
							"userInput": map[string]interface{}{
								"userResponse": "Hi",
							},
						},
						{
							"type":   "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
							"status": "CORTEX_STEP_STATUS_DONE",
							"plannerResponse": map[string]interface{}{
								"response": txt,
							},
						},
					},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	_, portStr, _ := net.SplitHostPort(upstream.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	p := &Proxy{
		shortClient:  upstream.Client(),
		mediumClient: upstream.Client(),
		streamClient: upstream.Client(),
		wsConns:      map[*websocket.Conn]struct{}{},
		activePort:   port,
	}
	defer p.Shutdown()

	gw := httptest.NewServer(http.HandlerFunc(p.HandleCascadeStream))
	defer gw.Close()
	wsBase := "ws" + strings.TrimPrefix(gw.URL, "http")

	// 1. Initial connection with delta=1 -> receives full "init" with Rev > 0
	c1, _, err := websocket.DefaultDialer.Dial(wsBase+"?cascadeId=resume-cid&format=messages&delta=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	var initFrame StreamUpdatePayload
	if err := c1.ReadJSON(&initFrame); err != nil {
		t.Fatal(err)
	}
	_ = c1.Close()

	if initFrame.Type != "init" || initFrame.Rev == 0 || len(initFrame.Messages) != 2 {
		t.Fatalf("expected full init with rev>0 and 2 messages, got type=%q rev=%d msgs=%d", initFrame.Type, initFrame.Rev, len(initFrame.Messages))
	}

	// 2. Reconnect with matching sinceRev while unchanged -> receives tiny "noop" frame
	c2, _, err := websocket.DefaultDialer.Dial(
		wsBase+"?cascadeId=resume-cid&format=messages&delta=1&sinceRev="+strconv.FormatUint(initFrame.Rev, 10),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	var noopFrame StreamUpdatePayload
	if err := c2.ReadJSON(&noopFrame); err != nil {
		t.Fatal(err)
	}
	_ = c2.Close()

	if noopFrame.Type != "noop" || noopFrame.Rev != initFrame.Rev || len(noopFrame.Messages) != 0 {
		t.Fatalf("expected noop frame with rev=%d and 0 messages, got %+v", initFrame.Rev, noopFrame)
	}

	// 3. Mutate trajectory while disconnected, then reconnect with sinceRev -> receives incremental delta "update"
	mu.Lock()
	respText = "Hello world — updated while offline!"
	mu.Unlock()
	expireTrajectoryCacheFreshness("resume-cid")

	c3, _, err := websocket.DefaultDialer.Dial(
		wsBase+"?cascadeId=resume-cid&format=messages&delta=1&sinceRev="+strconv.FormatUint(noopFrame.Rev, 10),
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	var deltaFrame StreamUpdatePayload
	if err := c3.ReadJSON(&deltaFrame); err != nil {
		t.Fatal(err)
	}
	_ = c3.Close()

	if deltaFrame.Type != "update" || !deltaFrame.Delta || deltaFrame.Rev <= noopFrame.Rev {
		t.Fatalf("expected delta update frame after offline mutation, got type=%q delta=%v rev=%d", deltaFrame.Type, deltaFrame.Delta, deltaFrame.Rev)
	}
	if len(deltaFrame.Messages) != 1 || !strings.Contains(deltaFrame.Messages[0].Text, "updated while offline") {
		t.Fatalf("expected only the 1 changed assistant message in delta frame, got %d messages: %+v", len(deltaFrame.Messages), deltaFrame.Messages)
	}

	// 4. Reconnect with unknown/expired sinceRev -> falls back cleanly to full "init"
	c4, _, err := websocket.DefaultDialer.Dial(
		wsBase+"?cascadeId=resume-cid&format=messages&delta=1&sinceRev=99999999",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	var fallbackInit StreamUpdatePayload
	if err := c4.ReadJSON(&fallbackInit); err != nil {
		t.Fatal(err)
	}
	_ = c4.Close()

	if fallbackInit.Type != "init" || fallbackInit.Delta || len(fallbackInit.Messages) != 2 {
		t.Fatalf("expected full init fallback for unknown sinceRev, got type=%q delta=%v msgs=%d", fallbackInit.Type, fallbackInit.Delta, len(fallbackInit.Messages))
	}
}

func TestPhase2ReactiveAgentStateStreamAndLiveContext(t *testing.T) {
	oldLinger := agentStateStreamLinger
	agentStateStreamLinger = 0
	defer func() { agentStateStreamLinger = oldLinger }()

	var mu sync.Mutex
	stepText := "Thinking step 1"
	agentPushCh := make(chan map[string]interface{}, 4)
	streamConnected := make(chan struct{})
	var connectOnce sync.Once

	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/StreamAgentStateUpdates"):
			flusher, _ := w.(http.Flusher)
			w.Header().Set("Content-Type", "application/connect+json")
			w.WriteHeader(http.StatusOK)
			_ = writeConnectFrame(w, flusher, map[string]interface{}{
				"update": map[string]interface{}{
					"contextWindowUsage": map[string]interface{}{
						"estimatedTokensUsed": 42000,
						"maxContextTokens":    200000,
					},
					"pendingAgentMessages": []map[string]interface{}{
						{"id": "qm-1", "content": "queued follow-up"},
					},
				},
			})
			connectOnce.Do(func() { close(streamConnected) })
			for {
				select {
				case <-r.Context().Done():
					return
				case frame := <-agentPushCh:
					if err := writeConnectFrame(w, flusher, frame); err != nil {
						return
					}
				}
			}
		case strings.HasSuffix(r.URL.Path, "/GetCascadeTrajectory"):
			mu.Lock()
			txt := stepText
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "CASCADE_RUN_STATUS_RUNNING",
				"trajectory": map[string]interface{}{
					"cascadeId": "live-cid",
					"steps": []map[string]interface{}{
						{
							"type":   "CORTEX_STEP_TYPE_PLANNER_RESPONSE",
							"status": "CORTEX_STEP_STATUS_GENERATING",
							"plannerResponse": map[string]interface{}{
								"response": txt,
							},
						},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	_, portStr, _ := net.SplitHostPort(upstream.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	p := &Proxy{
		shortClient:  upstream.Client(),
		mediumClient: upstream.Client(),
		streamClient: upstream.Client(),
		wsConns:      map[*websocket.Conn]struct{}{},
		activePort:   port,
	}
	defer p.Shutdown()

	gw := httptest.NewServer(http.HandlerFunc(p.HandleCascadeStream))
	defer gw.Close()
	wsURL := "ws" + strings.TrimPrefix(gw.URL, "http") + "?cascadeId=live-cid&format=messages&delta=1"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	var initFrame StreamUpdatePayload
	if err := conn.ReadJSON(&initFrame); err != nil {
		t.Fatal(err)
	}

	select {
	case <-streamConnected:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for StreamAgentStateUpdates connection")
	}

	// Push a live Connect delta frame and assert WebSocket wakes up well below the 900ms fallback poll interval!
	mu.Lock()
	stepText = "Thinking step 2 — pushed via reactive Connect stream!"
	mu.Unlock()
	pushStart := time.Now()
	agentPushCh <- map[string]interface{}{
		"update": map[string]interface{}{
			"contextWindowUsage": map[string]interface{}{
				"estimatedTokensUsed": 64000,
				"maxContextTokens":    200000,
			},
			"pendingAgentMessagesUpdate": map[string]interface{}{
				"items": []interface{}{},
			},
		},
	}

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var updateFrame StreamUpdatePayload
	if err := conn.ReadJSON(&updateFrame); err != nil {
		t.Fatalf("expected sub-500ms reactive wakeup frame, got err: %v", err)
	}
	if elapsed := time.Since(pushStart); elapsed > 450*time.Millisecond {
		t.Fatalf("reactive push took %v, expected immediate wakeup (<450ms)", elapsed)
	}
	if updateFrame.ContextMeter == nil || updateFrame.ContextMeter.UsedTokens != 64000 || updateFrame.ContextMeter.MaxTokens != 200000 {
		t.Fatalf("expected live ContextWindowUsage (64000/200000), got %+v", updateFrame.ContextMeter)
	}
	if len(updateFrame.QueuedMessages) != 0 {
		t.Fatalf("expected live pendingAgentMessagesUpdate to clear QueuedMessages, got %+v", updateFrame.QueuedMessages)
	}
}

func TestPhase2ReactiveSummariesAndEventsSinceRev(t *testing.T) {
	summaryPushCh := make(chan map[string]interface{}, 4)
	subConnected := make(chan struct{})
	var subOnce sync.Once

	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/JetboxSubscribeToSummaries"):
			flusher, _ := w.(http.Flusher)
			w.Header().Set("Content-Type", "application/connect+json")
			w.WriteHeader(http.StatusOK)
			_ = writeConnectFrame(w, flusher, map[string]interface{}{
				"updates": map[string]interface{}{
					"conv-1": map[string]interface{}{
						"status":    "CASCADE_RUN_STATUS_IDLE",
						"stepCount": 3,
						"summary":   "Initial Title",
					},
				},
			})
			subOnce.Do(func() { close(subConnected) })
			for {
				select {
				case <-r.Context().Done():
					return
				case frame := <-summaryPushCh:
					if err := writeConnectFrame(w, flusher, frame); err != nil {
						return
					}
				}
			}
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"trajectorySummaries":{"conv-1":{"status":"CASCADE_RUN_STATUS_IDLE","stepCount":3,"summary":"Initial Title"}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()

	_, portStr, _ := net.SplitHostPort(upstream.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)

	p := &Proxy{
		shortClient:  upstream.Client(),
		mediumClient: upstream.Client(),
		streamClient: upstream.Client(),
		wsConns:      map[*websocket.Conn]struct{}{},
		activePort:   port,
	}
	p.events.rev = 100
	defer p.Shutdown()

	gw := httptest.NewServer(http.HandlerFunc(p.HandleEvents))
	defer gw.Close()
	wsBase := "ws" + strings.TrimPrefix(gw.URL, "http")

	c1, _, err := websocket.DefaultDialer.Dial(wsBase, nil)
	if err != nil {
		t.Fatal(err)
	}
	var hello1 eventsFrame
	if err := c1.ReadJSON(&hello1); err != nil {
		t.Fatal(err)
	}
	if hello1.Type != "hello" || hello1.Unchanged {
		t.Fatalf("expected fresh hello (unchanged=false), got %+v", hello1)
	}

	select {
	case <-subConnected:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for JetboxSubscribeToSummaries connection")
	}

	// Push live summary update via Connect stream -> client should get "changed" immediately without polling!
	summaryPushCh <- map[string]interface{}{
		"updates": map[string]interface{}{
			"conv-1": map[string]interface{}{
				"status":    "CASCADE_RUN_STATUS_RUNNING",
				"stepCount": 4,
				"summary":   "Running Now",
			},
		},
	}

	_ = c1.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	var changedFrame eventsFrame
	if err := c1.ReadJSON(&changedFrame); err != nil {
		t.Fatalf("expected immediate changed frame from JetboxSubscribeToSummaries, got %v", err)
	}
	if changedFrame.Type != "changed" || changedFrame.Rev != hello1.Rev+1 {
		t.Fatalf("unexpected changed frame: %+v (hello rev=%d)", changedFrame, hello1.Rev)
	}

	// Reconnect second client while c1 keeps snapshot alive with matching sinceRev -> hello.Unchanged == true!
	c2, _, err := websocket.DefaultDialer.Dial(wsBase+"?sinceRev="+strconv.FormatUint(changedFrame.Rev, 10), nil)
	if err != nil {
		t.Fatal(err)
	}
	var hello2 eventsFrame
	if err := c2.ReadJSON(&hello2); err != nil {
		t.Fatal(err)
	}
	_ = c2.Close()
	_ = c1.Close()

	if hello2.Type != "hello" || !hello2.Unchanged || hello2.Rev != changedFrame.Rev {
		t.Fatalf("expected hello with unchanged=true and rev=%d, got %+v", changedFrame.Rev, hello2)
	}
}

func TestPhase2SQLiteHierarchyFastPath(t *testing.T) {
	if _, _, err := findSQLiteExecutable(); err != nil {
		t.Logf("sqlite3 CLI without -json in this environment (%v); verifying embedded promotion path only", err)
		summaries := map[string]map[string]interface{}{
			"sub-inline": {
				"status": "CASCADE_RUN_STATUS_RUNNING",
				"trajectoryMetadata": map[string]interface{}{
					"parentConversationId": "root-inline",
				},
			},
		}
		EnrichSummariesHierarchy(t.Context(), summaries)
		if got := summaries["sub-inline"]["parentConversationId"]; got != "root-inline" {
			t.Fatalf("expected parentConversationId=root-inline, got %v", got)
		}
		return
	}

	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	dbDir := filepath.Join(tmpHome, ".gemini", "antigravity")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(dbDir, "conversation_summaries.db")

	initSQL := `
CREATE TABLE conversation_summaries (
	conversation_id TEXT PRIMARY KEY,
	title TEXT,
	preview TEXT,
	step_count INTEGER,
	last_modified_time TEXT,
	workspace_uris TEXT,
	status TEXT,
	parent_conversation_id TEXT,
	nesting_depth INTEGER
);
INSERT INTO conversation_summaries VALUES
	('root-db-1', 'Root Task', 'preview 1', 12, '2026-11-15T00:00:00Z', 'file:///ws', 'running', '', 0),
	('child-db-1', 'Subagent Worker', 'preview 2', 5, '2026-11-15T00:00:01Z', 'file:///ws', 'completed', 'root-db-1', 1);
`
	if out, err := exec.Command("sqlite3", dbPath, initSQL).CombinedOutput(); err != nil {
		t.Fatalf("failed to seed test sqlite db: %v (%s)", err, string(out))
	}

	// Reset package caches for isolated test run
	localStoreMu.Lock()
	cachedSummariesMap = nil
	cachedSummariesStamp = ""
	localStoreMu.Unlock()
	hierarchyMu.Lock()
	delete(hierarchyCache, "root-db-1")
	delete(hierarchyCache, "child-db-1")
	hierarchyMu.Unlock()

	summaries := map[string]map[string]interface{}{
		"root-db-1":  {"status": "CASCADE_RUN_STATUS_RUNNING"},
		"child-db-1": {"status": "CASCADE_RUN_STATUS_COMPLETED"},
	}
	EnrichSummariesHierarchy(t.Context(), summaries)

	if got := summaries["child-db-1"]["parentConversationId"]; got != "root-db-1" {
		t.Fatalf("expected child-db-1 parentConversationId=root-db-1, got %v", got)
	}
	if got := summaries["child-db-1"]["nestingDepth"]; got != 1 {
		t.Fatalf("expected child-db-1 nestingDepth=1, got %v", got)
	}

	localMap, err := FetchLocalConversationSummaries(t.Context())
	if err != nil {
		t.Fatalf("FetchLocalConversationSummaries failed: %v", err)
	}
	if localMap["root-db-1"]["status"] != "CASCADE_RUN_STATUS_RUNNING" {
		t.Fatalf("expected root-db-1 status RUNNING, got %v", localMap["root-db-1"]["status"])
	}
}
