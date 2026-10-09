package proxy

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"antigravity-mobile/internal/inspector"
)

func TestBuildSlashCommandTables(t *testing.T) {
	mk := func(name, typ, icon string) upstreamSlashCommand {
		info, _ := json.Marshal(map[string]string{"name": name, "type": typ, "icon": icon, "modelFacingText": "TEXT-" + name})
		return upstreamSlashCommand{Info: info, Title: name, Description: "d-" + name}
	}
	raw := []upstreamSlashCommand{
		mk("plan", "SLASH_COMMAND_TYPE_SYSTEM", "ballot"),
		mk("zeta-skill", "SLASH_COMMAND_TYPE_SKILL", "z"),
		mk("schedule", "SLASH_COMMAND_TYPE_SYSTEM", "schedule"), // 隐藏
		mk("alpha-skill", "SLASH_COMMAND_TYPE_SKILL", "a"),
		mk("goal", "SLASH_COMMAND_TYPE_SYSTEM", "timer"),
		mk("plan", "SLASH_COMMAND_TYPE_SYSTEM", "dup"), // 重复名：忽略
		{Info: json.RawMessage(`{"type":"x"}`)},        // 没有 name：忽略
		{Info: json.RawMessage(`not json`)},
	}
	options, info := buildSlashCommandTables(raw)

	var names []string
	for _, o := range options {
		names = append(names, o.Kind+":"+o.Name)
	}
	want := "system:goal,system:plan,skill:alpha-skill,skill:zeta-skill"
	if strings.Join(names, ",") != want {
		t.Fatalf("menu = %v, want %s", names, want)
	}
	// 隐藏的命令不进菜单，但仍可按名称解析（桌面端/旧入口发来时不至于被拒）
	if _, ok := info["schedule"]; !ok {
		t.Error("hidden commands must remain resolvable")
	}
	if !strings.Contains(string(info["plan"]), `"icon":"ballot"`) {
		t.Errorf("duplicate must not overwrite the first entry: %s", info["plan"])
	}
	menuJSON, _ := json.Marshal(options)
	if strings.Contains(string(menuJSON), "TEXT-") || strings.Contains(string(menuJSON), "modelFacingText") {
		t.Errorf("menu must not carry the (large) injected text: %s", menuJSON)
	}
}

func newSlashTestProxy(t *testing.T, slashCalls *int32, sent *map[string]interface{}) *Proxy {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetSlashCommands"):
			atomic.AddInt32(slashCalls, 1)
			_, _ = w.Write([]byte(`{"commands":[
			  {"info":{"name":"plan","type":"SLASH_COMMAND_TYPE_SYSTEM","icon":"ballot","modelFacingText":"<PLAN>think first</PLAN>"},"title":"plan","description":"Plan carefully"},
			  {"info":{"name":"bird-twitter","type":"SLASH_COMMAND_TYPE_SKILL","definitionPath":"/x/SKILL.md","modelFacingText":"<SKILL>use bird</SKILL>"},"title":"bird-twitter","description":"Twitter"},
			  {"info":{"name":"schedule","type":"SLASH_COMMAND_TYPE_SYSTEM","modelFacingText":"S"},"title":"schedule","description":"hidden"}]}`))
		case strings.HasSuffix(r.URL.Path, "/SendUserCascadeMessage"):
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			*sent = body
			_, _ = w.Write([]byte(`{}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	p := NewProxy(inspector.NewInspector(5 * time.Second))
	p.updateUpstream(inspector.InstanceInfo{PID: 1, Port: srv.Listener.Addr().(*net.TCPAddr).Port, CSRFToken: "t", IsHealthy: true})
	return p
}

func doSend(p *Proxy, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	return rec
}

func TestSlashCommandsEndpointAndCache(t *testing.T) {
	var calls int32
	var sent map[string]interface{}
	p := newSlashTestProxy(t, &calls, &sent)

	get := func(path string) SlashCommandsResponse {
		rec := httptest.NewRecorder()
		p.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s => %d %s", path, rec.Code, rec.Body.String())
		}
		var res SlashCommandsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		return res
	}
	res := get("/gateway/slash-commands")
	if len(res.Commands) != 2 || res.Commands[0].Name != "plan" || res.Commands[1].Name != "bird-twitter" {
		t.Fatalf("commands = %+v", res.Commands)
	}
	get("/gateway/slash-commands")
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("second call must hit the cache, upstream calls = %d", calls)
	}
	get("/gateway/slash-commands?refresh=1")
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("refresh=1 must refetch, upstream calls = %d", calls)
	}

	// 没有上游：503，而不是挂起
	rec := httptest.NewRecorder()
	(&Proxy{}).HandleSlashCommands(rec, httptest.NewRequest(http.MethodGet, "/gateway/slash-commands", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no upstream => %d", rec.Code)
	}
}

func TestSendExpandsSlashCommandToAuthoritativeInfo(t *testing.T) {
	var calls int32
	var sent map[string]interface{}
	p := newSlashTestProxy(t, &calls, &sent)

	// 客户端只传名称；网关补上权威 info（含 modelFacingText），并保留后面的文本条目
	rec := doSend(p, `{"cascadeId":"c1","text":"hi","items":[{"item":{"slashCommand":{"info":{"name":"plan","modelFacingText":"FORGED"}}}},{"text":"帮我规划"}]}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	items, _ := sent["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("forwarded items = %v", sent["items"])
	}
	info := items[0].(map[string]interface{})["item"].(map[string]interface{})["slashCommand"].(map[string]interface{})["info"].(map[string]interface{})
	if info["modelFacingText"] != "<PLAN>think first</PLAN>" || info["name"] != "plan" {
		t.Errorf("client-supplied text must be replaced by the authoritative one: %v", info)
	}
	if items[1].(map[string]interface{})["text"] != "帮我规划" {
		t.Errorf("text item lost: %v", items[1])
	}

	// 技能命令带 definitionPath
	rec = doSend(p, `{"cascadeId":"c1","items":[{"item":{"slashCommand":{"info":{"name":"bird-twitter"}}}}]}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("slash-only message => %d %s", rec.Code, rec.Body.String())
	}
	info = sent["items"].([]interface{})[0].(map[string]interface{})["item"].(map[string]interface{})["slashCommand"].(map[string]interface{})["info"].(map[string]interface{})
	if info["definitionPath"] != "/x/SKILL.md" {
		t.Errorf("skill info = %v", info)
	}

	// 隐藏命令仍可被解析
	if rec = doSend(p, `{"cascadeId":"c1","items":[{"item":{"slashCommand":{"info":{"name":"schedule"}}}}]}`, nil); rec.Code != http.StatusOK {
		t.Errorf("hidden command must still resolve, got %d", rec.Code)
	}
}

func TestSendRejectsUnknownOrEmptySlashCommand(t *testing.T) {
	var calls int32
	var sent map[string]interface{}
	p := newSlashTestProxy(t, &calls, &sent)

	sent = map[string]interface{}{"sentinel": true}
	rec := doSend(p, `{"cascadeId":"c1","items":[{"item":{"slashCommand":{"info":{"name":"no-such-command"}}}}]}`, nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "unknown slash command") {
		t.Fatalf("unknown => %d %s", rec.Code, rec.Body.String())
	}
	rec = doSend(p, `{"cascadeId":"c1","items":[{"item":{"slashCommand":{"info":{}}}}]}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty name => %d", rec.Code)
	}
	if sent["sentinel"] != true {
		t.Error("rejected messages must never reach language_server")
	}
}

func TestSendLeavesPlainMessagesUntouched(t *testing.T) {
	var calls int32
	var sent map[string]interface{}
	p := newSlashTestProxy(t, &calls, &sent)

	rec := doSend(p, `{"cascadeId":"c1","items":[{"text":"普通消息"}]}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Error("plain messages must not trigger a GetSlashCommands round trip")
	}
	if _, has := sent["tags"]; has {
		t.Errorf("messages without deliveryStrategy must not be tagged: %v", sent["tags"])
	}
}

func TestSlashDedupeDistinguishesCommands(t *testing.T) {
	var calls int32
	var sent map[string]interface{}
	p := newSlashTestProxy(t, &calls, &sent)

	body := func(cmd string) string {
		return `{"cascadeId":"dedupe-1","items":[{"item":{"slashCommand":{"info":{"name":"` + cmd + `"}}}},{"text":"同样的文字"}]}`
	}
	doSend(p, body("plan"), nil)
	first := sent
	sent = nil
	// 同样文字、不同命令：不能被当成重复消息吞掉
	doSend(p, body("bird-twitter"), nil)
	if sent == nil {
		t.Fatal("a different slash command with identical text was wrongly deduplicated")
	}
	// 完全相同的重复发送仍被去重
	sent = nil
	doSend(p, body("bird-twitter"), nil)
	if sent != nil {
		t.Error("identical repeat within the window should still be deduplicated")
	}
	_ = first
}
