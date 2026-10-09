package proxy

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
)

// 结构取自真实抓包：父会话 invoke_subagent 步骤（2 个子代理）。
const invokeSubagentStepJSON = `{
  "type": "CORTEX_STEP_TYPE_INVOKE_SUBAGENT",
  "status": "CORTEX_STEP_STATUS_DONE",
  "metadata": {"sourceTrajectoryStepInfo": {"stepIndex": 6}},
  "invokeSubagent": {
    "subagents": [
      {"typeName": "research", "role": "Embodied AI Researcher", "initialPrompt": "调研具身智能", "inherit": true, "modelTier": "MODEL_TIER_FLASH"},
      {"typeName": "hardware_researcher", "role": "AI Hardware Analyst", "initialPrompt": "调研算力芯片", "modelTier": "MODEL_TIER_FLASH"}
    ],
    "results": [
      {"conversationId": "child-a", "logAbsoluteUri": "file:///x/transcript.jsonl"},
      {"conversationId": "child-b", "logAbsoluteUri": "file:///y/transcript.jsonl"}
    ]
  }
}`

func parseStep(t *testing.T, raw string) TrajectoryStep {
	t.Helper()
	var s TrajectoryStep
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestBuildSubagents_PairsSpecsWithResults(t *testing.T) {
	p := &Proxy{}
	items := p.buildSubagents([]TrajectoryStep{{Type: "CORTEX_STEP_TYPE_USER_INPUT"}, parseStep(t, invokeSubagentStepJSON)})
	if len(items) != 2 {
		t.Fatalf("want 2 subagents, got %d: %+v", len(items), items)
	}
	if items[0].ConversationID != "child-a" || items[0].Role != "Embodied AI Researcher" || items[0].TypeName != "research" || items[0].StepIndex != 6 {
		t.Fatalf("first item wrong: %+v", items[0])
	}
	if items[1].ConversationID != "child-b" || items[1].TypeName != "hardware_researcher" || items[1].Prompt != "调研算力芯片" {
		t.Fatalf("second item wrong: %+v", items[1])
	}
}

func TestBuildSubagents_SkipsPendingAndUnrelated(t *testing.T) {
	p := &Proxy{}
	pending := parseStep(t, `{"type":"CORTEX_STEP_TYPE_INVOKE_SUBAGENT","invokeSubagent":{"subagents":[{"typeName":"self"}],"results":[]}}`)
	noID := parseStep(t, `{"type":"CORTEX_STEP_TYPE_INVOKE_SUBAGENT","invokeSubagent":{"subagents":[{"typeName":"self"}],"results":[{"conversationId":"  "}]}}`)
	if got := p.buildSubagents([]TrajectoryStep{pending, noID}); len(got) != 0 {
		t.Fatalf("expected none, got %+v", got)
	}
	if got := p.buildSubagents(nil); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestBuildSubagents_TruncatesLongPrompt(t *testing.T) {
	p := &Proxy{}
	long := strings.Repeat("长", subagentPromptMaxRunes+50)
	step := parseStep(t, `{"type":"CORTEX_STEP_TYPE_INVOKE_SUBAGENT","invokeSubagent":{"subagents":[{"initialPrompt":"`+long+`"}],"results":[{"conversationId":"c"}]}}`)
	items := p.buildSubagents([]TrajectoryStep{step})
	if got := len([]rune(items[0].Prompt)); got != subagentPromptMaxRunes+1 { // +1 for the ellipsis
		t.Fatalf("prompt runes = %d", got)
	}
}

func TestSubagentIdentity(t *testing.T) {
	var raw upstreamTrajectoryResp
	body := `{"trajectory":{"cascadeId":"c","metadata":{"parentConversationId":"parent-1","subagentSpec":{"typeName":"research","role":"Web UI investigator"}}}}`
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatal(err)
	}
	parent, role := subagentIdentity(&raw)
	if parent != "parent-1" || role != "Web UI investigator" {
		t.Fatalf("got %q %q", parent, role)
	}

	// role 缺失时退回 typeName
	raw = upstreamTrajectoryResp{}
	_ = json.Unmarshal([]byte(`{"trajectory":{"metadata":{"parentConversationId":"p","subagentSpec":{"typeName":"self"}}}}`), &raw)
	if _, role := subagentIdentity(&raw); role != "self" {
		t.Fatalf("fallback role = %q", role)
	}

	// 普通会话
	raw = upstreamTrajectoryResp{}
	_ = json.Unmarshal([]byte(`{"trajectory":{"metadata":{"createdAt":"x"}}}`), &raw)
	if parent, role := subagentIdentity(&raw); parent != "" || role != "" {
		t.Fatalf("main conversation reported as subagent: %q %q", parent, role)
	}
	if parent, role := subagentIdentity(nil); parent != "" || role != "" {
		t.Fatal("nil must be a no-op")
	}
}

// fakeSubagentUpstream serves a list snapshot and records ForceStopCascadeTree calls.
type fakeSubagentUpstream struct {
	mu      sync.Mutex
	list    string
	stopped []string
	srv     *httptest.Server
}

func newFakeSubagentUpstream(t *testing.T, list string) (*fakeSubagentUpstream, *Proxy) {
	t.Helper()
	f := &fakeSubagentUpstream{list: list}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetAllCascadeTrajectories"):
			w.Write([]byte(f.list))
		case strings.HasSuffix(r.URL.Path, "/ForceStopCascadeTree"):
			var req struct {
				ConversationID string `json:"conversationId"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			f.stopped = append(f.stopped, req.ConversationID)
			w.Write([]byte(`{"stoppedConversationIds":["` + req.ConversationID + `"]}`))
		default:
			w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(f.srv.Close)
	p := NewProxy(inspector.NewInspector(5 * time.Second))
	p.activePort = f.srv.Listener.Addr().(*net.TCPAddr).Port
	p.activeToken = "t"
	return f, p
}

func (f *fakeSubagentUpstream) setList(list string) {
	f.mu.Lock()
	f.list = list
	f.mu.Unlock()
}

const subagentListFixture = `{"trajectorySummaries":{
 "main-1":  {"status":"CASCADE_RUN_STATUS_IDLE","stepCount":8,"summary":"Main task","trajectoryMetadata":{"createdAt":"x"}},
 "child-a": {"status":"CASCADE_RUN_STATUS_RUNNING","stepCount":12,"summary":"Embodied AI","trajectoryMetadata":{"parentConversationId":"main-1"}},
 "child-b": {"status":"CASCADE_RUN_STATUS_IDLE","stepCount":30,"summary":"Hardware","trajectoryMetadata":{"parentConversationId":"main-1"}}
}}`

func TestEnrichSubagents_LiveStatus(t *testing.T) {
	_, p := newFakeSubagentUpstream(t, subagentListFixture)
	in := []SubagentItem{{ConversationID: "child-a"}, {ConversationID: "child-b"}, {ConversationID: "deleted-c"}}
	out := p.enrichSubagents(in)
	want := []struct {
		status string
		steps  int
		title  string
	}{{"running", 12, "Embodied AI"}, {"done", 30, "Hardware"}, {"gone", 0, ""}}
	for i, w := range want {
		if out[i].Status != w.status || out[i].StepCount != w.steps || out[i].Title != w.title {
			t.Fatalf("item %d = %+v, want %+v", i, out[i], w)
		}
	}
	if in[0].Status != "" {
		t.Fatal("input must not be mutated")
	}
	if got := p.enrichSubagents(nil); got != nil {
		t.Fatalf("nil in, nil out; got %+v", got)
	}
}

func TestSubagentLiveSignature_ChangesWithChildState(t *testing.T) {
	f, p := newFakeSubagentUpstream(t, subagentListFixture)
	raw := &upstreamTrajectoryResp{}
	raw.Trajectory.Steps = []TrajectoryStep{parseStep(t, invokeSubagentStepJSON)}

	first := p.subagentLiveSignature(raw)
	if first == "" {
		t.Fatal("expected a signature when subagents exist")
	}
	if again := p.subagentLiveSignature(raw); again != first {
		t.Fatalf("signature must be stable: %q vs %q", first, again)
	}

	f.setList(strings.Replace(subagentListFixture, `"status":"CASCADE_RUN_STATUS_RUNNING","stepCount":12`, `"status":"CASCADE_RUN_STATUS_IDLE","stepCount":13`, 1))
	p.invalidateAllTrajectories()
	if changed := p.subagentLiveSignature(raw); changed == first {
		t.Fatal("signature must change when a child finishes")
	}

	if got := p.subagentLiveSignature(&upstreamTrajectoryResp{}); got != "" {
		t.Fatalf("no subagents → empty signature, got %q", got)
	}
	if got := p.subagentLiveSignature(nil); got != "" {
		t.Fatalf("nil → empty signature, got %q", got)
	}
}

func postStop(p *Proxy, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/gateway/subagent/stop", strings.NewReader(body))
	rec := httptest.NewRecorder()
	p.HandleSubagentStop(rec, req)
	return rec
}

func TestHandleSubagentStop(t *testing.T) {
	f, p := newFakeSubagentUpstream(t, subagentListFixture)

	rec := postStop(p, `{"conversationId":"child-a"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop child: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Success bool     `json:"success"`
		Stopped []string `json:"stopped"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || !resp.Success || len(resp.Stopped) != 1 || resp.Stopped[0] != "child-a" {
		t.Fatalf("bad response: %s (%v)", rec.Body.String(), err)
	}
	f.mu.Lock()
	got := append([]string(nil), f.stopped...)
	f.mu.Unlock()
	if len(got) != 1 || got[0] != "child-a" {
		t.Fatalf("upstream stop calls = %v", got)
	}

	// 主会话绝不能通过这个接口被停掉
	if rec := postStop(p, `{"conversationId":"main-1"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("main conversation: want 400, got %d", rec.Code)
	}
	if rec := postStop(p, `{"conversationId":"nope"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown: want 404, got %d", rec.Code)
	}
	if rec := postStop(p, `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty: want 400, got %d", rec.Code)
	}
	if rec := postStop(p, `not json`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json: want 400, got %d", rec.Code)
	}
	f.mu.Lock()
	n := len(f.stopped)
	f.mu.Unlock()
	if n != 1 {
		t.Fatalf("rejected requests must not reach upstream; stop calls = %d", n)
	}

	req := httptest.NewRequest(http.MethodGet, "/gateway/subagent/stop", nil)
	rr := httptest.NewRecorder()
	p.HandleSubagentStop(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: want 405, got %d", rr.Code)
	}
}

func TestHandleSubagentStop_NoUpstream(t *testing.T) {
	p := NewProxy(inspector.NewInspector(time.Second))
	if rec := postStop(p, `{"conversationId":"x"}`); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
}
