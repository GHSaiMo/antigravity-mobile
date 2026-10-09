package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func zeroTime() time.Time { return time.Time{} }

// resetLiveModels isolates the process-wide registry between tests.
func resetLiveModels(t *testing.T) {
	t.Helper()
	liveModels.mu.Lock()
	savedOptions, savedByID, savedByEnum := liveModels.options, liveModels.byID, liveModels.byEnum
	savedFetched, savedAttempt, savedInflight := liveModels.fetchedAt, liveModels.lastAttempt, liveModels.inflight
	liveModels.options, liveModels.byID, liveModels.byEnum = nil, nil, nil
	liveModels.fetchedAt, liveModels.lastAttempt = zeroTime(), zeroTime()
	liveModels.inflight = false
	liveModels.mu.Unlock()
	t.Cleanup(func() {
		liveModels.mu.Lock()
		liveModels.options, liveModels.byID, liveModels.byEnum = savedOptions, savedByID, savedByEnum
		liveModels.fetchedAt, liveModels.lastAttempt, liveModels.inflight = savedFetched, savedAttempt, savedInflight
		liveModels.mu.Unlock()
	})
}

// realisticModels reproduces the shapes seen from a real GetAvailableModels response:
// internal placeholders, duplicate display names behind legacy ids, and a non-recommended model.
func realisticModels() map[string]upstreamModel {
	g := "MODEL_PROVIDER_GOOGLE"
	a := "MODEL_PROVIDER_ANTHROPIC"
	return map[string]upstreamModel{
		"chat_20706":               {Model: "MODEL_CHAT_20706", IsInternal: true},
		"claude-opus-4-6-thinking": {DisplayName: "Claude Opus 4.6 (Thinking)", Model: "MODEL_PLACEHOLDER_M26", ModelProvider: a, Recommended: true, Thinking: true, SupportsImages: true},
		"claude-sonnet-4-6":        {DisplayName: "Claude Sonnet 4.6 (Thinking)", Model: "MODEL_PLACEHOLDER_M35", ModelProvider: a, Recommended: true},
		"gemini-3.8-flash-high":    {DisplayName: "Gemini 3.8 Flash (High)", Model: "MODEL_PLACEHOLDER_M318", ModelProvider: g, Recommended: true},
		"gemini-3.8-flash-medium":  {DisplayName: "Gemini 3.8 Flash (Medium)", Model: "MODEL_PLACEHOLDER_M319", ModelProvider: g, Recommended: true},
		"gemini-3.8-flash-low":     {DisplayName: "Gemini 3.8 Flash (Low)", Model: "MODEL_PLACEHOLDER_M320", ModelProvider: g, Recommended: true},
		"gemini-3.7-flash-high":    {DisplayName: "Gemini 3.7 Flash (High)", Model: "MODEL_PLACEHOLDER_M298", ModelProvider: g, Recommended: true, TagTitle: "Leaving Soon"},
		"gemini-3.1-pro-high":      {DisplayName: "Gemini 3.1 Pro (High)", Model: "MODEL_PLACEHOLDER_M37", ModelProvider: g, Recommended: true},
		"gemini-pro-agent":         {DisplayName: "Gemini 3.1 Pro (High)", Model: "MODEL_PLACEHOLDER_M16", ModelProvider: g, Recommended: true, TagTitle: "Leaving Soon"},
		// 同一展示名下的旧 id：应该输给带真实版本号的 id
		"gemini-2.5-flash":      {DisplayName: "Gemini 3.5 Flash Lite", Model: "MODEL_GOOGLE_GEMINI_2_5_FLASH", ModelProvider: g, Recommended: true},
		"gemini-3.5-flash-lite": {DisplayName: "Gemini 3.5 Flash Lite", Model: "MODEL_PLACEHOLDER_M198", ModelProvider: g, Recommended: true},
		// 退役枚举：网关会把它改映射到别的模型，不能出现在候选里
		"gemini-2.5-pro":         {DisplayName: "Gemini 2.5 Pro", Model: "MODEL_GOOGLE_GEMINI_2_5_PRO", ModelProvider: g, Recommended: true},
		"gemini-3.1-flash-image": {DisplayName: "Gemini 3.1 Flash Image", Model: "MODEL_PLACEHOLDER_M21", ModelProvider: g, Recommended: false},
		"gpt-oss-120b-medium":    {DisplayName: "GPT-OSS 120B (Medium)", Model: "MODEL_OPENAI_GPT_OSS_120B_MEDIUM", ModelProvider: "MODEL_PROVIDER_OPENAI", Recommended: true},
	}
}

func idsOf(opts []ModelOption, provider string) []string {
	var out []string
	for _, o := range opts {
		if o.Provider == provider {
			out = append(out, o.ID)
		}
	}
	return out
}

func TestBuildModelTables_CurationDedupeAndOrder(t *testing.T) {
	options, byID, byEnum := buildModelTables(realisticModels(), nil)

	// 内部占位、未推荐的模型不出现在候选里，但仍可被查到（用于解析会话里实际用到的模型）
	for _, o := range options {
		if strings.HasPrefix(o.ID, "chat_") || o.ID == "gemini-3.1-flash-image" || o.ID == "gemini-2.5-pro" {
			t.Errorf("%s must not be offered", o.ID)
		}
	}
	if _, ok := byID["gemini-2.5-pro"]; !ok {
		t.Error("retired models stay resolvable by id")
	}
	if _, ok := byID["gemini-3.1-flash-image"]; !ok {
		t.Error("non-recommended models must still resolve by id")
	}
	if _, ok := byID["chat_20706"]; ok {
		t.Error("internal placeholder must not be resolvable")
	}

	// 「Leaving Soon」的 3.7 不提供选择；同名去重：Pro (High) 选 gemini-3.1-pro-high；Flash Lite 选 3.5 而不是 2.5
	gemini := idsOf(options, "gemini")
	want := []string{
		"gemini-3.8-flash-high", "gemini-3.8-flash-medium", "gemini-3.8-flash-low", // 版本新→旧，同版本 High→Low
		"gemini-3.5-flash-lite",
		"gemini-3.1-pro-high", // 与 -agent 同名，-agent 带「Leaving Soon」被排除后留下这个
	}
	if strings.Join(gemini, ",") != strings.Join(want, ",") {
		t.Fatalf("gemini order/dedupe:\n got  %v\n want %v", gemini, want)
	}
	if got := idsOf(options, "claude"); len(got) != 2 || got[0] != "claude-opus-4-6-thinking" && got[1] != "claude-opus-4-6-thinking" {
		t.Fatalf("claude options = %v", got)
	}
	if got := idsOf(options, "other"); len(got) != 1 || got[0] != "gpt-oss-120b-medium" {
		t.Fatalf("other options = %v", got)
	}

	// 枚举反查：别名共用枚举时选真实 id
	if byEnum["MODEL_PLACEHOLDER_M16"].ID != "gemini-pro-agent" || byEnum["MODEL_PLACEHOLDER_M37"].ID != "gemini-3.1-pro-high" {
		t.Errorf("enum reverse map wrong: %+v / %+v", byEnum["MODEL_PLACEHOLDER_M16"], byEnum["MODEL_PLACEHOLDER_M37"])
	}
}

func TestRegistrySnapshotDefaults(t *testing.T) {
	resetLiveModels(t)

	// 未取到实时列表：用内置兜底，且标记 Live=false
	res, live := liveModels.snapshot()
	if live || res.Live || len(res.Models) == 0 {
		t.Fatalf("fallback expected, got live=%v models=%d", live, len(res.Models))
	}
	if res.Defaults["gemini"] != "gemini-3.8-flash-high" || res.Defaults["claude"] != "claude-opus-4-6-thinking" {
		t.Fatalf("fallback defaults = %v", res.Defaults)
	}

	// 实时列表里没有首选默认值时，退化为该厂商的第一个候选
	raw := realisticModels()
	delete(raw, "gemini-3.8-flash-high")
	delete(raw, "claude-opus-4-6-thinking")
	o, bi, be := buildModelTables(raw, nil)
	liveModels.set(o, bi, be, "")
	res, live = liveModels.snapshot()
	if !live || !res.Live {
		t.Fatal("live list expected")
	}
	if res.Defaults["gemini"] != "gemini-3.8-flash-medium" {
		t.Errorf("gemini default = %q, want first remaining", res.Defaults["gemini"])
	}
	if res.Defaults["claude"] != "claude-sonnet-4-6" {
		t.Errorf("claude default = %q", res.Defaults["claude"])
	}
}

func TestResolveModelEnumPrefersLiveList(t *testing.T) {
	resetLiveModels(t)

	// 注册表为空：沿用静态表
	if got := resolveModelEnum("gemini-3.8-flash-high"); got != "MODEL_PLACEHOLDER_M318" {
		t.Fatalf("static fallback = %q", got)
	}
	if got := resolveModelEnum("gemini-3.5-flash-lite"); got != "" {
		t.Fatalf("unknown id without live list must not resolve, got %q", got)
	}

	o, bi, be := buildModelTables(realisticModels(), nil)
	liveModels.set(o, bi, be, "")

	// 静态表里没有的新模型：靠实时列表解析
	if got := resolveModelEnum("gemini-3.5-flash-lite"); got != "MODEL_PLACEHOLDER_M198" {
		t.Errorf("live-only id = %q", got)
	}
	// 大小写不敏感
	if got := resolveModelEnum("Claude-Sonnet-4-6"); got != "MODEL_PLACEHOLDER_M35" {
		t.Errorf("case-insensitive = %q", got)
	}
	// 退役的 2.5 枚举仍旧映射到 M318，保持历史行为
	if got := resolveModelEnum("gemini-2.5-flash"); got != "MODEL_PLACEHOLDER_M318" {
		t.Errorf("legacy remap = %q", got)
	}
	// 直接传枚举
	if got := resolveModelEnum("MODEL_PLACEHOLDER_M26"); got != "MODEL_PLACEHOLDER_M26" {
		t.Errorf("enum passthrough = %q", got)
	}
}

func TestCanonicalModelNameReverseLookup(t *testing.T) {
	resetLiveModels(t)
	if got := canonicalModelName("MODEL_PLACEHOLDER_M198"); got != "MODEL_PLACEHOLDER_M198" {
		t.Fatalf("unknown enum without live list should stay as is, got %q", got)
	}
	o, bi, be := buildModelTables(realisticModels(), nil)
	liveModels.set(o, bi, be, "")

	if got := canonicalModelName("MODEL_PLACEHOLDER_M198"); got != "gemini-3.5-flash-lite" {
		t.Errorf("new enum => %q", got)
	}
	// 静态表优先，保持既有行为
	if got := canonicalModelName("MODEL_PLACEHOLDER_M26"); got != "claude-opus-4-6-thinking" {
		t.Errorf("static enum => %q", got)
	}
}

func TestDisplayNameResolution(t *testing.T) {
	resetLiveModels(t)
	// 兜底表也能给出展示名
	if got := liveModels.displayName("claude-opus-4-6-thinking"); got != "Claude Opus 4.6 (Thinking)" {
		t.Errorf("fallback display = %q", got)
	}
	o, bi, be := buildModelTables(realisticModels(), nil)
	liveModels.set(o, bi, be, "")
	if got := liveModels.displayName("gemini-3.5-flash-lite"); got != "Gemini 3.5 Flash Lite" {
		t.Errorf("by id = %q", got)
	}
	if got := liveModels.displayName("MODEL_PLACEHOLDER_M319"); got != "Gemini 3.8 Flash (Medium)" {
		t.Errorf("by enum = %q", got)
	}
	if got := liveModels.displayName("nonexistent"); got != "" {
		t.Errorf("unknown => %q", got)
	}
}

func TestGeneratorModelOf(t *testing.T) {
	resetLiveModels(t)
	o, bi, be := buildModelTables(realisticModels(), nil)
	liveModels.set(o, bi, be, "")

	var step TrajectoryStep
	step.Metadata.GeneratorModel = "MODEL_PLACEHOLDER_M319"
	id, name := generatorModelOf(step)
	if id != "gemini-3.8-flash-medium" || name != "Gemini 3.8 Flash (Medium)" {
		t.Errorf("got %q / %q", id, name)
	}

	// 未知枚举不应把裸枚举名暴露给用户
	step.Metadata.GeneratorModel = "MODEL_PLACEHOLDER_M99999"
	if id, name := generatorModelOf(step); id != "" || name != "" {
		t.Errorf("unknown enum leaked: %q / %q", id, name)
	}
	step.Metadata.GeneratorModel = ""
	if id, name := generatorModelOf(step); id != "" || name != "" {
		t.Errorf("empty => %q / %q", id, name)
	}
}

func TestAgentMessagesCarryGeneratorModel(t *testing.T) {
	resetLiveModels(t)
	o, bi, be := buildModelTables(realisticModels(), nil)
	liveModels.set(o, bi, be, "")

	mk := func(typ string) TrajectoryStep { return TrajectoryStep{Type: typ} }
	user := mk("CORTEX_STEP_TYPE_USER_INPUT")
	user.UserInput = &TrajectoryUserInput{UserResponse: "hi"}
	a1 := mk("CORTEX_STEP_TYPE_PLANNER_RESPONSE")
	a1.PlannerResponse = &struct {
		Response string `json:"response"`
		Thinking string `json:"thinking"`
	}{Response: "first"}
	a1.Metadata.GeneratorModel = "MODEL_PLACEHOLDER_M318"
	a2 := a1
	a2.PlannerResponse = &struct {
		Response string `json:"response"`
		Thinking string `json:"thinking"`
	}{Response: "second"}
	a2.Metadata.GeneratorModel = "MODEL_PLACEHOLDER_M26"

	msgs, _ := buildTrajectoryMessages([]TrajectoryStep{user, a1, user, a2})
	var agents []CascadeMessageItem
	for _, m := range msgs {
		if m.Type == "agent" {
			agents = append(agents, m)
		}
	}
	if len(agents) != 2 {
		t.Fatalf("got %d agent messages: %+v", len(agents), msgs)
	}
	if agents[0].Model != "gemini-3.8-flash-high" || agents[0].ModelName != "Gemini 3.8 Flash (High)" {
		t.Errorf("first reply model = %q / %q", agents[0].Model, agents[0].ModelName)
	}
	if agents[1].Model != "claude-opus-4-6-thinking" || agents[1].ModelName != "Claude Opus 4.6 (Thinking)" {
		t.Errorf("second reply model = %q / %q (each reply keeps its own model)", agents[1].Model, agents[1].ModelName)
	}
}

func TestTrajectoryStartedAt(t *testing.T) {
	mk := func(ts string) TrajectoryStep {
		var s TrajectoryStep
		s.Metadata.CreatedAt = ts
		return s
	}
	if got := trajectoryStartedAt(nil); got != "" {
		t.Errorf("nil => %q", got)
	}
	steps := []TrajectoryStep{mk(""), mk("not-a-time"), mk("2026-10-08T04:39:13.864926Z"), mk("2026-10-09T00:00:00Z")}
	if got := trajectoryStartedAt(steps); got != "2026-10-08T04:39:13.864926Z" {
		t.Errorf("first valid timestamp expected, got %q", got)
	}
}

func fakeModelsLS(t *testing.T, calls *int, fail bool) *Proxy {
	return newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/GetAvailableModels") {
			http.NotFound(w, r)
			return
		}
		*calls++
		if fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"response": map[string]interface{}{"models": realisticModels()}})
	})
}

func TestHandleModels_LiveCachesAndRefreshes(t *testing.T) {
	resetLiveModels(t)
	calls := 0
	p := fakeModelsLS(t, &calls, false)

	get := func(path string) ModelsResponse {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		p.HandleModels(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		var res ModelsResponse
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		return res
	}

	res := get("/gateway/models")
	if !res.Live || calls != 1 {
		t.Fatalf("first call should fetch: live=%v calls=%d", res.Live, calls)
	}
	if res.Defaults["gemini"] != "gemini-3.8-flash-high" || res.Defaults["claude"] != "claude-opus-4-6-thinking" {
		t.Errorf("defaults = %v", res.Defaults)
	}
	get("/gateway/models")
	if calls != 1 {
		t.Errorf("fresh cache must not refetch, calls=%d", calls)
	}
	get("/gateway/models?refresh=1")
	if calls != 2 {
		t.Errorf("refresh=1 must refetch, calls=%d", calls)
	}
}

func TestHandleModels_UpstreamFailureFallsBackAndBacksOff(t *testing.T) {
	resetLiveModels(t)
	calls := 0
	p := fakeModelsLS(t, &calls, true)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/gateway/models", nil)
		w := httptest.NewRecorder()
		p.HandleModels(w, req)
		var res ModelsResponse
		_ = json.Unmarshal(w.Body.Bytes(), &res)
		if w.Code != http.StatusOK || res.Live || len(res.Models) == 0 {
			t.Fatalf("fallback list expected on failure: code=%d live=%v n=%d", w.Code, res.Live, len(res.Models))
		}
	}
	if calls != 1 {
		t.Errorf("failures must back off instead of hammering upstream, calls=%d", calls)
	}

	// 没有上游时同样给兜底
	noUpstream := &Proxy{}
	w := httptest.NewRecorder()
	noUpstream.HandleModels(w, httptest.NewRequest(http.MethodGet, "/gateway/models", nil))
	if w.Code != http.StatusOK {
		t.Errorf("no upstream => %d", w.Code)
	}
	w = httptest.NewRecorder()
	noUpstream.HandleModels(w, httptest.NewRequest(http.MethodDelete, "/gateway/models", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE => %d", w.Code)
	}
}

func TestBuildModelTables_UsesVendorCuratedList(t *testing.T) {
	curated := []string{
		"gemini-3.8-flash-high",
		"gemini-3.7-flash-high",
		"gemini-pro-agent",
		"claude-sonnet-4-6",
		"claude-opus-4-6-thinking",
		"gpt-oss-120b-medium",
		"no-such-model",         // 官方列表里有、但详细信息里没有：跳过
		"gemini-2.5-pro",        // 退役枚举：跳过
		"gemini-3.8-flash-high", // 重复：只留一个
	}
	options, byID, _ := buildModelTables(realisticModels(), curated)

	var got []string
	for _, o := range options {
		got = append(got, o.ID)
	}
	// 3.7 与 pro-agent 带「Leaving Soon」，即使官方列表里有也不提供选择
	want := []string{"gemini-3.8-flash-high", "claude-sonnet-4-6", "claude-opus-4-6-thinking", "gpt-oss-120b-medium"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("curated list:\n got  %v\n want %v", got, want)
	}
	// 官方列表之外的推荐模型（如 3.5 Flash Lite、3.8 Medium/Low）不再出现，但仍可按 id 解析
	if _, ok := byID["gemini-3.5-flash-lite"]; !ok {
		t.Error("non-listed models must stay resolvable")
	}
	// 被排除的 Leaving Soon 模型仍可解析，旧会话还在用它时不会丢失
	for _, id := range []string{"gemini-3.7-flash-high", "gemini-pro-agent"} {
		if _, ok := byID[id]; !ok {
			t.Errorf("%s must stay resolvable", id)
		}
	}
}

func TestBuildModelTables_EmptyCuratedFallsBackToHeuristic(t *testing.T) {
	// 官方列表为空，或其中没有一个可用 id：退回启发式筛选，而不是给出空列表
	for _, curated := range [][]string{nil, {}, {"no-such-model"}} {
		options, _, _ := buildModelTables(realisticModels(), curated)
		if len(options) == 0 {
			t.Fatalf("curated=%v must fall back to heuristic", curated)
		}
	}
}

func TestSnapshotPrefersVendorDefault(t *testing.T) {
	resetLiveModels(t)
	o, bi, be := buildModelTables(realisticModels(), []string{"gemini-3.8-flash-high", "gemini-3.8-flash-medium", "claude-sonnet-4-6", "claude-opus-4-6-thinking"})
	liveModels.set(o, bi, be, "gemini-3.8-flash-medium")
	res, _ := liveModels.snapshot()
	if res.Defaults["gemini"] != "gemini-3.8-flash-medium" {
		t.Errorf("vendor default should win: %v", res.Defaults)
	}
	if res.Defaults["claude"] != "claude-opus-4-6-thinking" {
		t.Errorf("claude keeps gateway preference: %v", res.Defaults)
	}

	// 官方默认不在列表里时忽略它
	liveModels.set(o, bi, be, "gemini-9-nonexistent")
	res, _ = liveModels.snapshot()
	if res.Defaults["gemini"] != "gemini-3.8-flash-high" {
		t.Errorf("unknown vendor default must be ignored: %v", res.Defaults)
	}
}

func TestHandleModels_ParsesVendorPickerList(t *testing.T) {
	resetLiveModels(t)
	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/GetAvailableModels") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"response": map[string]interface{}{
			"models":              realisticModels(),
			"defaultAgentModelId": "gemini-3.8-flash-medium",
			"agentModelSorts": []map[string]interface{}{{
				"displayName": "Recommended",
				"groups": []map[string]interface{}{{"modelIds": []string{
					"gemini-3.8-flash-high", "gemini-3.8-flash-medium", "claude-opus-4-6-thinking",
				}}},
			}},
		}})
	})
	w := httptest.NewRecorder()
	p.HandleModels(w, httptest.NewRequest(http.MethodGet, "/gateway/models", nil))
	var res ModelsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range res.Models {
		ids = append(ids, m.ID)
	}
	if strings.Join(ids, ",") != "gemini-3.8-flash-high,gemini-3.8-flash-medium,claude-opus-4-6-thinking" {
		t.Fatalf("ids = %v", ids)
	}
	if !res.Live || res.Defaults["gemini"] != "gemini-3.8-flash-medium" {
		t.Errorf("live=%v defaults=%v", res.Live, res.Defaults)
	}
}
