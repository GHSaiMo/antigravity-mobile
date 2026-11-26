package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func zeroTime() time.Time { return time.Time{} }

// resetLiveModels isolates the process-wide registry between tests.
func resetLiveModels(t *testing.T) {
	t.Helper()
	liveModels.mu.Lock()
	savedOptions, savedByID, savedByEnum := liveModels.options, liveModels.byID, liveModels.byEnum
	savedOfficialDef := liveModels.officialDef
	savedOrchSpecs, savedOrchByEnum := liveModels.orchSpecs, liveModels.orchByPlannerEnum
	savedFetched, savedAttempt, savedInflight := liveModels.fetchedAt, liveModels.lastAttempt, liveModels.inflight
	liveModels.options, liveModels.byID, liveModels.byEnum = nil, nil, nil
	liveModels.officialDef = ""
	liveModels.orchSpecs, liveModels.orchByPlannerEnum = nil, nil
	liveModels.fetchedAt, liveModels.lastAttempt = zeroTime(), zeroTime()
	liveModels.inflight = false
	liveModels.mu.Unlock()
	t.Cleanup(func() {
		liveModels.mu.Lock()
		liveModels.options, liveModels.byID, liveModels.byEnum = savedOptions, savedByID, savedByEnum
		liveModels.officialDef = savedOfficialDef
		liveModels.orchSpecs, liveModels.orchByPlannerEnum = savedOrchSpecs, savedOrchByEnum
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
		"gemini-3.8-flash-high":    {DisplayName: "Gemini 3.8 Flash (High)", Model: "MODEL_PLACEHOLDER_M318", ModelProvider: g, Recommended: true, TagTitle: "Leaving Soon"},
		"gemini-3.8-flash-medium":  {DisplayName: "Gemini 3.8 Flash (Medium)", Model: "MODEL_PLACEHOLDER_M319", ModelProvider: g, Recommended: true, TagTitle: "Leaving Soon"},
		"gemini-3.8-flash-low":     {DisplayName: "Gemini 3.8 Flash (Low)", Model: "MODEL_PLACEHOLDER_M320", ModelProvider: g, Recommended: true, TagTitle: "Leaving Soon"},
		"gemini-3.7-flash-high":    {DisplayName: "Gemini 3.7 Flash (High)", Model: "MODEL_PLACEHOLDER_M298", ModelProvider: g, Recommended: true, TagTitle: "Unavailable"},
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

	// 「Unavailable」的 3.7 不提供选择；「Leaving Soon」的 3.8 Flash 仍保留可选且剥离徽章；
	// 同名去重：Pro (High) 选非 Leaving Soon 的 gemini-3.1-pro-high；Flash Lite 选 3.5 而不是 2.5
	gemini := idsOf(options, "gemini")
	want := []string{
		"gemini-3.8-flash-high", "gemini-3.8-flash-medium", "gemini-3.8-flash-low", // 版本新→旧，同版本 High→Low
		"gemini-3.5-flash-lite",
		"gemini-3.1-pro-high", // 与 -agent 同名，-agent 带「Leaving Soon」且为 -agent 别名，输给 gemini-3.1-pro-high
	}
	if strings.Join(gemini, ",") != strings.Join(want, ",") {
		t.Fatalf("gemini order/dedupe:\n got  %v\n want %v", gemini, want)
	}
	for _, o := range options {
		if strings.HasPrefix(o.ID, "gemini-3.8-flash-") && o.Tag != "" {
			t.Errorf("expected Leaving Soon tag stripped on %s, got %q", o.ID, o.Tag)
		}
	}
	if got := idsOf(options, "claude"); len(got) != 2 || got[0] != "claude-opus-4-6-thinking" && got[1] != "claude-opus-4-6-thinking" {
		t.Fatalf("claude options = %v", got)
	}
	if got := idsOf(options, "other"); len(got) != 0 {
		t.Fatalf("other options = %v, want excluded (no gpt-oss)", got)
	}
	if _, ok := byID["gpt-oss-120b-medium"]; !ok {
		t.Error("excluded gpt-oss model must stay resolvable by id")
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
	if res.Defaults["gemini"] != "gemini-4-neon-high" || res.Defaults["claude"] != "claude-opus-4-6-thinking" || res.Defaults["orchestrator"] != "gemini-pro-orchestrator-neon" {
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
	a1.PlannerResponse = &TrajectoryPlannerResponse{Response: "first"}
	a1.Metadata.GeneratorModel = "MODEL_PLACEHOLDER_M318"
	a2 := a1
	a2.PlannerResponse = &TrajectoryPlannerResponse{Response: "second"}
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
	// 3.7 带「Unavailable」、gpt-oss-120b-medium 已屏蔽：即使官方列表里有也不提供选择；
	// 3.8 Flash 带「Leaving Soon」但在下线前仍保留可选（且剥离 tag）
	want := []string{"gemini-3.8-flash-high", "claude-sonnet-4-6", "claude-opus-4-6-thinking"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("curated list:\n got  %v\n want %v", got, want)
	}
	if options[0].Tag != "" {
		t.Errorf("expected Leaving Soon tag stripped on curated model, got %q", options[0].Tag)
	}
	// 被排除的 gpt-oss-120b-medium 仍可按 id 解析，避免历史会话显示异常
	if _, ok := byID["gpt-oss-120b-medium"]; !ok {
		t.Error("excluded gpt-oss-120b-medium must stay resolvable")
	}
	// 官方列表之外的推荐模型（如 3.5 Flash Lite、3.8 Medium/Low）不再出现，但仍可按 id 解析
	if _, ok := byID["gemini-3.5-flash-lite"]; !ok {
		t.Error("non-listed models must stay resolvable")
	}
	// 被排除的 Unavailable 模型仍可解析，旧会话还在用它时不会丢失
	if _, ok := byID["gemini-3.7-flash-high"]; !ok {
		t.Errorf("gemini-3.7-flash-high must stay resolvable")
	}
}

func TestBuildModelTables_JioLeavingSoonFlashRetainedAndUnavailableExcluded(t *testing.T) {
	resetLiveModels(t)
	g := "MODEL_PROVIDER_GOOGLE"
	a := "MODEL_PROVIDER_ANTHROPIC"
	raw := map[string]upstreamModel{
		"gemini-3.8-flash-high":    {DisplayName: "Gemini 3.8 Flash (High)", Model: "MODEL_PLACEHOLDER_M318", ModelProvider: g, Recommended: true, Thinking: true, TagTitle: "Leaving Soon"},
		"gemini-3.8-flash-medium":  {DisplayName: "Gemini 3.8 Flash (Medium)", Model: "MODEL_PLACEHOLDER_M319", ModelProvider: g, Recommended: true, Thinking: true, TagTitle: "Leaving Soon"},
		"gemini-3.8-flash-low":     {DisplayName: "Gemini 3.8 Flash (Low)", Model: "MODEL_PLACEHOLDER_M320", ModelProvider: g, Recommended: true, Thinking: true, TagTitle: "Leaving Soon"},
		"gemini-3.1-pro-high":      {DisplayName: "Gemini 3.1 Pro (High)", Model: "MODEL_PLACEHOLDER_M37", ModelProvider: g, Recommended: true, TagTitle: "Unavailable"},
		"claude-sonnet-4-6":        {DisplayName: "Claude Sonnet 4.6 (Thinking)", Model: "MODEL_PLACEHOLDER_M35", ModelProvider: a, Recommended: true, Thinking: true},
		"claude-opus-4-6-thinking": {DisplayName: "Claude Opus 4.6 (Thinking)", Model: "MODEL_PLACEHOLDER_M26", ModelProvider: a, Recommended: true, Thinking: true},
		"gpt-oss-120b-medium":      {DisplayName: "GPT-OSS 120B (Medium)", Model: "MODEL_OPENAI_GPT_OSS_120B_MEDIUM", ModelProvider: "MODEL_PROVIDER_OPENAI", Recommended: true, TagTitle: "Notice"},
	}
	curated := []string{
		"gemini-3.8-flash-high",
		"gemini-3.8-flash-medium",
		"gemini-3.8-flash-low",
		"gemini-3.1-pro-high",
		"claude-sonnet-4-6",
		"claude-opus-4-6-thinking",
		"gpt-oss-120b-medium",
	}

	for _, tc := range []struct {
		name    string
		curated []string
	}{
		{name: "curated", curated: curated},
		{name: "heuristic", curated: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, byID, byEnum := buildModelTables(raw, tc.curated)
			gemini := idsOf(opts, "gemini")
			wantGemini := "gemini-3.8-flash-high,gemini-3.8-flash-medium,gemini-3.8-flash-low"
			if strings.Join(gemini, ",") != wantGemini {
				t.Fatalf("gemini options = %v, want %s", gemini, wantGemini)
			}
			for _, o := range opts {
				if o.Tag != "" {
					t.Errorf("unexpected tag %q on %s", o.Tag, o.ID)
				}
			}
			if _, ok := byID["gemini-3.1-pro-high"]; !ok {
				t.Errorf("unavailable model must remain resolvable in byID")
			}
			liveModels.set(opts, byID, byEnum, "")
			res, live := liveModels.snapshot()
			if !live || res.Defaults["gemini"] != "gemini-3.8-flash-high" {
				t.Fatalf("expected gemini default gemini-3.8-flash-high, got live=%v defaults=%v", live, res.Defaults)
			}
		})
	}
}

func TestBuildModelTables_ExcludesDisabledPlanGatedModels(t *testing.T) {
	resetLiveModels(t)
	g := "MODEL_PROVIDER_GOOGLE"
	a := "MODEL_PROVIDER_ANTHROPIC"
	raw := map[string]upstreamModel{
		"gemini-4-hydrogen-high":    {DisplayName: "Gemini 4 Hydrogen (High)", Model: "MODEL_PLACEHOLDER_M525", ModelProvider: g, Recommended: true, Thinking: true, TagTitle: "New"},
		"gemini-4-hydrogen-xhigh":   {DisplayName: "Gemini 4 Hydrogen (XHigh)", Model: "MODEL_PLACEHOLDER_M528", ModelProvider: g, Recommended: true, Thinking: true, TagTitle: "New"},
		"gemini-4-hydrogen-medium":  {DisplayName: "Gemini 4 Hydrogen (Medium)", Model: "MODEL_PLACEHOLDER_M526", ModelProvider: g, Recommended: true, Thinking: true, TagTitle: "New"},
		"gemini-4-hydrogen-low":     {DisplayName: "Gemini 4 Hydrogen (Low)", Model: "MODEL_PLACEHOLDER_M527", ModelProvider: g, Recommended: true, Thinking: true, TagTitle: "New"},
		"gemini-3.8-flash-high":     {DisplayName: "Gemini 3.8 Flash (High)", Model: "MODEL_PLACEHOLDER_M318", ModelProvider: g, Recommended: true, Thinking: true, TagTitle: "Leaving Soon"},
		"gemini-3.8-flash-medium":   {DisplayName: "Gemini 3.8 Flash (Medium)", Model: "MODEL_PLACEHOLDER_M319", ModelProvider: g, Recommended: true, Thinking: true, TagTitle: "Leaving Soon"},
		"gemini-3.8-flash-low":      {DisplayName: "Gemini 3.8 Flash (Low)", Model: "MODEL_PLACEHOLDER_M320", ModelProvider: g, Recommended: true, Thinking: true, TagTitle: "Leaving Soon"},
		"gemini-4-carbon-disabled":  {DisplayName: "Gemini 4 Carbon", Model: "MODEL_PLACEHOLDER_M670", ModelProvider: g, Recommended: true, Disabled: true, TagTitle: "Paid Pro"},
		"gemini-4-argon-disabled":   {DisplayName: "Gemini 4 Argon", Model: "MODEL_PLACEHOLDER_M421", ModelProvider: g, Recommended: true, Disabled: true, TagTitle: "Paid Pro"},
		"gemini-4-neon-disabled":    {DisplayName: "Gemini 4 Neon", Model: "MODEL_PLACEHOLDER_M536", ModelProvider: g, Recommended: true, Disabled: true, TagTitle: "Paid Pro"},
		"gemini-4-flag-only":        {DisplayName: "Gemini 4 Flag Only", Model: "MODEL_PLACEHOLDER_M991", ModelProvider: g, Recommended: true, Disabled: true, TagTitle: "Paid Pro"},
		"gemini-4-suffix-disabled":  {DisplayName: "Gemini 4 Suffix Disabled", Model: "MODEL_PLACEHOLDER_M992", ModelProvider: g, Recommended: true, Disabled: false, TagTitle: "Paid Pro"},
		"claude-sonnet-4-6":         {DisplayName: "Claude Sonnet 4.6 (Thinking)", Model: "MODEL_PLACEHOLDER_M35", ModelProvider: a, Recommended: true, Thinking: true, TagTitle: "Notice"},
		"claude-opus-4-6-thinking":  {DisplayName: "Claude Opus 4.6 (Thinking)", Model: "MODEL_PLACEHOLDER_M26", ModelProvider: a, Recommended: true, Thinking: true, TagTitle: "Notice"},
		"gpt-oss-120b-medium":       {DisplayName: "GPT-OSS 120B (Medium)", Model: "MODEL_OPENAI_GPT_OSS_120B_MEDIUM", ModelProvider: "MODEL_PROVIDER_OPENAI", Recommended: true, TagTitle: "Notice"},
	}
	curated := []string{
		"gemini-4-hydrogen-high",
		"gemini-4-hydrogen-xhigh",
		"gemini-4-hydrogen-medium",
		"gemini-4-hydrogen-low",
		"gemini-3.8-flash-high",
		"gemini-3.8-flash-medium",
		"gemini-3.8-flash-low",
		"claude-sonnet-4-6",
		"claude-opus-4-6-thinking",
		"gpt-oss-120b-medium",
		"gemini-4-carbon-disabled",
		"gemini-4-argon-disabled",
		"gemini-4-neon-disabled",
		"gemini-4-flag-only",
		"gemini-4-suffix-disabled",
	}

	for _, tc := range []struct {
		name       string
		curated    []string
		wantGemini string
	}{
		{
			name:       "curated",
			curated:    curated,
			wantGemini: "gemini-4-hydrogen-high,gemini-4-hydrogen-xhigh,gemini-4-hydrogen-medium,gemini-4-hydrogen-low,gemini-3.8-flash-high,gemini-3.8-flash-medium,gemini-3.8-flash-low",
		},
		{
			name:       "heuristic",
			curated:    nil,
			wantGemini: "gemini-4-hydrogen-xhigh,gemini-4-hydrogen-high,gemini-4-hydrogen-medium,gemini-4-hydrogen-low,gemini-3.8-flash-high,gemini-3.8-flash-medium,gemini-3.8-flash-low",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, byID, _ := buildModelTables(raw, tc.curated)
			gemini := idsOf(opts, "gemini")
			if strings.Join(gemini, ",") != tc.wantGemini {
				t.Fatalf("gemini options = %v, want %s", gemini, tc.wantGemini)
			}
			for _, id := range []string{"gemini-4-carbon-disabled", "gemini-4-argon-disabled", "gemini-4-neon-disabled", "gemini-4-flag-only", "gemini-4-suffix-disabled"} {
				if _, ok := byID[id]; !ok {
					t.Errorf("disabled model %s must stay resolvable in byID", id)
				}
			}
		})
	}
}

func TestHandleModels_ExcludesDisabledJSONModels(t *testing.T) {
	resetLiveModels(t)
	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/GetAvailableModels") {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{
			"response": {
				"defaultAgentModelId": "gemini-4-hydrogen-high",
				"models": {
					"gemini-4-hydrogen-high": {"displayName":"Gemini 4 Hydrogen (High)","model":"MODEL_PLACEHOLDER_M525","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"supportsThinking":true,"tagTitle":"New"},
					"gemini-3.8-flash-high": {"displayName":"Gemini 3.8 Flash (High)","model":"MODEL_PLACEHOLDER_M318","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"supportsThinking":true,"tagTitle":"Leaving Soon"},
					"gemini-4-neon-disabled": {"displayName":"Gemini 4 Neon","model":"MODEL_PLACEHOLDER_M536","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"disabled":true,"tagTitle":"Paid Pro"}
				},
				"agentModelSorts": [{
					"displayName": "Recommended",
					"groups": [{"modelIds": ["gemini-4-hydrogen-high", "gemini-3.8-flash-high", "gemini-4-neon-disabled"]}]
				}]
			}
		}`))
	})

	w := httptest.NewRecorder()
	p.HandleModels(w, httptest.NewRequest(http.MethodGet, "/gateway/models?refresh=1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	var res ModelsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := idsOf(res.Models, "gemini")
	if strings.Join(got, ",") != "gemini-4-hydrogen-high,gemini-3.8-flash-high" {
		t.Fatalf("got=%v, want [gemini-4-hydrogen-high gemini-3.8-flash-high]", got)
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

func TestBuildModelTables_Antigravity230ModelsAndTags(t *testing.T) {
	resetLiveModels(t)
	g := "MODEL_PROVIDER_GOOGLE"
	a := "MODEL_PROVIDER_ANTHROPIC"
	raw := map[string]upstreamModel{
		"gemini-4-argon-high":   {DisplayName: "Gemini 4 Argon (High)", Model: "MODEL_PLACEHOLDER_M417", ModelProvider: g, Recommended: true, Thinking: true, SupportsImages: true, TagTitle: "New"},
		"gemini-4-argon-medium": {DisplayName: "Gemini 4 Argon (Medium)", Model: "MODEL_PLACEHOLDER_M418", ModelProvider: g, Recommended: true, Thinking: true, SupportsImages: true, TagTitle: "New"},
		"claude-sonnet-5-5-high": {DisplayName: "Claude Sonnet 5.5 (High)", Model: "MODEL_PLACEHOLDER_M405", ModelProvider: a, Recommended: true, Thinking: true, SupportsImages: true, TagTitle: "New"},
		"claude-opus-5-5-high":   {DisplayName: "Claude Opus 5.5 (High)", Model: "MODEL_PLACEHOLDER_M402", ModelProvider: a, Recommended: true, Thinking: true, SupportsImages: true, TagTitle: "New"},
	}
	curated := []string{"gemini-4-argon-high", "gemini-4-argon-medium", "claude-sonnet-5-5-high", "claude-opus-5-5-high"}
	o, bi, be := buildModelTables(raw, curated)
	liveModels.set(o, bi, be, "gemini-4-argon-medium")

	res, live := liveModels.snapshot()
	if !live || res.Defaults["gemini"] != "gemini-4-argon-medium" || res.Defaults["claude"] != "claude-opus-5-5-high" {
		t.Fatalf("2.30 defaults = %v (live=%v)", res.Defaults, live)
	}
	for _, m := range res.Models {
		if m.Tag != "New" {
			t.Errorf("expected Tag='New' on %s, got %q", m.ID, m.Tag)
		}
	}
}

func TestBuildModelTables_StripsNoticeTag(t *testing.T) {
	raw := map[string]upstreamModel{
		"claude-sonnet-4-6": {
			DisplayName:    "Claude Sonnet 4.6 (Thinking)",
			Model:          "MODEL_PLACEHOLDER_M35",
			ModelProvider:  "MODEL_PROVIDER_ANTHROPIC",
			Recommended:    true,
			Thinking:       true,
			SupportsImages: true,
			TagTitle:       "Notice",
		},
		"gemini-3.8-flash-high": {
			DisplayName:   "Gemini 3.8 Flash (High)",
			Model:         "MODEL_PLACEHOLDER_M318",
			ModelProvider: "MODEL_PROVIDER_GOOGLE",
			Recommended:   true,
			TagTitle:      "Fast",
		},
	}
	options, _, _ := buildModelTables(raw, []string{"gemini-3.8-flash-high", "claude-sonnet-4-6"})
	for _, m := range options {
		if m.ID == "claude-sonnet-4-6" && m.Tag != "" {
			t.Errorf("expected Notice tag stripped, got %q", m.Tag)
		}
	}
}

func TestRefreshLiveModelsAfterSwitch_ReplacesOldAccountCache(t *testing.T) {
	resetLiveModels(t)
	// First upstream returns account A models
	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/GetAvailableModels") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"response": map[string]interface{}{
			"models": map[string]upstreamModel{
				"claude-opus-5-5-high": {DisplayName: "Claude Opus 5.5 (High)", Model: "MODEL_PLACEHOLDER_M402", ModelProvider: "MODEL_PROVIDER_ANTHROPIC", Recommended: true},
			},
		}})
	})
	p.HandleModels(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/gateway/models", nil))
	res, live := liveModels.snapshot()
	if !live || len(res.Models) != 1 || res.Models[0].ID != "claude-opus-5-5-high" {
		t.Fatalf("expected account A cache, got live=%v models=%+v", live, res.Models)
	}

	// Switch upstream to account B (only has claude-sonnet-4-6)
	pB := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/GetAvailableModels") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"response": map[string]interface{}{
			"models": map[string]upstreamModel{
				"claude-sonnet-4-6": {DisplayName: "Claude Sonnet 4.6 (Thinking)", Model: "MODEL_PLACEHOLDER_M35", ModelProvider: "MODEL_PROVIDER_ANTHROPIC", Recommended: true},
			},
		}})
	})
	pB.RefreshLiveModelsAfterSwitch(t.Context())
	res, live = liveModels.snapshot()
	if !live || len(res.Models) != 1 || res.Models[0].ID != "claude-sonnet-4-6" {
		t.Fatalf("expected account B models after switch refresh, got live=%v models=%+v", live, res.Models)
	}
}

func TestBuildModelTables_OrchestratorProviderAndDefaults(t *testing.T) {
	resetLiveModels(t)
	g := "MODEL_PROVIDER_GOOGLE"
	raw := map[string]upstreamModel{
		"gemini-4-carbon-medium":        {DisplayName: "Gemini 4 Carbon (Medium)", Model: "MODEL_PLACEHOLDER_M437", ModelProvider: g, Recommended: true},
		"gemini-pro-orchestrator":       {DisplayName: "Orchestrator (Carbon)", Model: "MODEL_PLACEHOLDER_M667", ModelProvider: g, Recommended: true, TagTitle: "New"},
		"gemini-pro-orchestrator-argon": {DisplayName: "Orchestrator (Argon)", Model: "MODEL_PLACEHOLDER_M681", ModelProvider: g, Recommended: true, TagTitle: "New"},
		"claude-opus-5-5-high":          {DisplayName: "Claude Opus 5.5 (High)", Model: "MODEL_PLACEHOLDER_M402", ModelProvider: "MODEL_PROVIDER_ANTHROPIC", Recommended: true},
	}
	curated := []string{"gemini-4-carbon-medium", "gemini-pro-orchestrator", "gemini-pro-orchestrator-argon", "claude-opus-5-5-high"}
	o, bi, be := buildModelTables(raw, curated)
	liveModels.set(o, bi, be, "gemini-4-carbon-medium")

	res, live := liveModels.snapshot()
	if !live {
		t.Fatal("expected live models")
	}
	if res.Defaults["orchestrator"] != "gemini-pro-orchestrator" {
		t.Fatalf("expected orchestrator default gemini-pro-orchestrator, got %v", res.Defaults)
	}
	orch := idsOf(res.Models, "orchestrator")
	if strings.Join(orch, ",") != "gemini-pro-orchestrator,gemini-pro-orchestrator-argon" {
		t.Fatalf("expected orchestrator models separated from gemini, got %v", orch)
	}
	if canonicalModelName("MODEL_PLACEHOLDER_M667") != "gemini-pro-orchestrator" {
		t.Errorf("M667 canonical = %q", canonicalModelName("MODEL_PLACEHOLDER_M667"))
	}
	if canonicalModelName("MODEL_PLACEHOLDER_M681") != "gemini-pro-orchestrator-argon" {
		t.Errorf("M681 canonical = %q", canonicalModelName("MODEL_PLACEHOLDER_M681"))
	}
}

func TestReadDesktopSelectedModel_AndHandleModels(t *testing.T) {
	resetLiveModels(t)
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	if id, name := readDesktopSelectedModel(); id != "" || name != "" {
		t.Fatalf("expected empty desktop model when file missing, got (%q, %q)", id, name)
	}

	stateDir := filepath.Join(tmpHome, ".gemini", "antigravity")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stateFile := filepath.Join(stateDir, "antigravity_state.pbtxt")
	if err := os.WriteFile(stateFile, []byte("agent_onboarding_completed: AGENT_ONBOARDING_STATE_COMPLETED\nlast_selected_agent_model: MODEL_PLACEHOLDER_M525\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/GetAvailableModels") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"response": map[string]interface{}{
			"models": map[string]upstreamModel{
				"gemini-4-hydrogen-high": {DisplayName: "Gemini 4 Hydrogen (High)", Model: "MODEL_PLACEHOLDER_M525", ModelProvider: "MODEL_PROVIDER_GOOGLE", Recommended: true},
			},
		}})
	})

	rec := httptest.NewRecorder()
	p.HandleModels(rec, httptest.NewRequest(http.MethodGet, "/gateway/models", nil))
	var res ModelsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.DesktopActiveModel != "gemini-4-hydrogen-high" {
		t.Fatalf("expected desktopActiveModel gemini-4-hydrogen-high, got %q", res.DesktopActiveModel)
	}
	if res.DesktopActiveModelName != "Gemini 4 Hydrogen (High)" {
		t.Fatalf("expected desktopActiveModelName Gemini 4 Hydrogen (High), got %q", res.DesktopActiveModelName)
	}

	if err := os.WriteFile(stateFile, []byte("last_selected_agent_model: MODEL_UNSPECIFIED\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if id, name := readDesktopSelectedModel(); id != "" || name != "" {
		t.Fatalf("expected MODEL_UNSPECIFIED ignored, got (%q, %q)", id, name)
	}
}

func TestReadDesktopSelectedModel_CarbonEnums(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	pbPath := filepath.Join(home, ".gemini", "antigravity", "antigravity_state.pbtxt")
	if err := os.MkdirAll(filepath.Dir(pbPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pbPath, []byte("last_selected_agent_model: MODEL_PLACEHOLDER_M427\n"), 0644); err != nil {
		t.Fatal(err)
	}
	id, name := readDesktopSelectedModel()
	if id != "gemini-4-carbon-high" {
		t.Fatalf("expected gemini-4-carbon-high, got %q", id)
	}
	if name != "Gemini 4 Carbon (High)" {
		t.Fatalf("expected Gemini 4 Carbon (High), got %q", name)
	}
}

func TestBuildModelTables_OrchestratorThinkingEffortsAndHighDefault(t *testing.T) {
	resetLiveModels(t)
	g := "MODEL_PROVIDER_GOOGLE"
	raw := map[string]upstreamModel{
		"gemini-pro-orchestrator-neon-low":    {DisplayName: "Orchestrator (Neon) (Low)", Model: "MODEL_PLACEHOLDER_M701", ModelProvider: g, Recommended: true, Thinking: true, TagTitle: "New"},
		"gemini-pro-orchestrator-neon-medium": {DisplayName: "Orchestrator (Neon) (Medium)", Model: "MODEL_PLACEHOLDER_M702", ModelProvider: g, Recommended: true, Thinking: true, TagTitle: "New"},
		"gemini-pro-orchestrator-neon-high":   {DisplayName: "Orchestrator (Neon) (High)", Model: "MODEL_PLACEHOLDER_M703", ModelProvider: g, Recommended: true, Thinking: true, TagTitle: "New"},
		"gemini-pro-orchestrator-neon-xhigh":  {DisplayName: "Orchestrator (Neon) (XHigh)", Model: "MODEL_PLACEHOLDER_M704", ModelProvider: g, Recommended: true, Thinking: true, TagTitle: "New"},
		"gemini-pro-orchestrator-argon-low":   {DisplayName: "Orchestrator (Argon, Low)", Model: "MODEL_PLACEHOLDER_M705", ModelProvider: g, Recommended: true, Thinking: true, TagTitle: "New"},
		"gemini-pro-orchestrator-argon-xhigh": {DisplayName: "Orchestrator (Argon, Extra High)", Model: "MODEL_PLACEHOLDER_M706", ModelProvider: g, Recommended: true, Thinking: true, TagTitle: "New"},
		"custom-multi-agent-high":             {DisplayName: "Orchestrator (Carbon - High)", Model: "MODEL_PLACEHOLDER_M707", ModelProvider: g, Recommended: true, Thinking: true},
	}
	curated := []string{
		"gemini-pro-orchestrator-neon-low",
		"gemini-pro-orchestrator-neon-medium",
		"gemini-pro-orchestrator-neon-high",
		"gemini-pro-orchestrator-neon-xhigh",
		"gemini-pro-orchestrator-argon-low",
		"gemini-pro-orchestrator-argon-xhigh",
		"custom-multi-agent-high",
	}
	o, bi, be := buildModelTables(raw, curated)
	liveModels.set(o, bi, be, "")

	res, live := liveModels.snapshot()
	if !live {
		t.Fatal("expected live models")
	}
	if res.Defaults["orchestrator"] != "gemini-pro-orchestrator-neon-high" {
		t.Fatalf("orchestrator default = %q, want gemini-pro-orchestrator-neon-high", res.Defaults["orchestrator"])
	}
	orch := idsOf(res.Models, "orchestrator")
	if len(orch) != len(curated) {
		t.Fatalf("expected all %d orchestrator options classified as orchestrator, got %v", len(curated), orch)
	}
	byID := map[string]ModelOption{}
	for _, opt := range res.Models {
		byID[opt.ID] = opt
	}
	if got := byID["gemini-pro-orchestrator-neon-xhigh"].Name; got != "Orchestrator (Neon) (XHigh)" {
		t.Errorf("xhigh display name = %q, want Orchestrator (Neon) (XHigh)", got)
	}
	if got := byID["gemini-pro-orchestrator-argon-xhigh"].Name; got != "Orchestrator (Argon, XHigh)" {
		t.Errorf("argon extra high display name = %q, want Orchestrator (Argon, XHigh)", got)
	}

	// Unknown orchestrator slug without preferredDefaults hit still prefers High over Low/Extra
	o2, bi2, be2 := buildModelTables(map[string]upstreamModel{
		"custom-orch-low":   {DisplayName: "Orchestrator (Titanium) (Low)", Model: "MODEL_PLACEHOLDER_M711", ModelProvider: g, Recommended: true},
		"custom-orch-xhigh": {DisplayName: "Orchestrator (Titanium) (XHigh)", Model: "MODEL_PLACEHOLDER_M712", ModelProvider: g, Recommended: true},
		"custom-orch-high":  {DisplayName: "Orchestrator (Titanium) (High)", Model: "MODEL_PLACEHOLDER_M713", ModelProvider: g, Recommended: true},
	}, []string{"custom-orch-low", "custom-orch-xhigh", "custom-orch-high"})
	liveModels.set(o2, bi2, be2, "")
	res2, _ := liveModels.snapshot()
	if res2.Defaults["orchestrator"] != "custom-orch-high" {
		t.Fatalf("unknown orchestrator default = %q, want custom-orch-high", res2.Defaults["orchestrator"])
	}
}

func TestHandleModels_ExpandsOrchestratorFromMendelRoles(t *testing.T) {
	resetLiveModels(t)
	t.Setenv("HOME", t.TempDir())

	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/GetAvailableModels"):
			_, _ = w.Write([]byte(`{
				"response": {
					"defaultAgentModelId": "gemini-4-neon-high",
					"models": {
						"gemini-4-neon-high": {"displayName":"Gemini 4 Neon (High)","model":"MODEL_PLACEHOLDER_M532","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"supportsImages":true,"supportsThinking":true,"tagTitle":"New","maxTokens":1048576},
						"gemini-4-neon-xhigh": {"displayName":"Gemini 4 Neon (XHigh)","model":"MODEL_PLACEHOLDER_M535","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"supportsImages":true,"supportsThinking":true,"tagTitle":"New","maxTokens":1048576},
						"gemini-4-neon-medium": {"displayName":"Gemini 4 Neon (Medium)","model":"MODEL_PLACEHOLDER_M533","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"supportsImages":true,"supportsThinking":true,"tagTitle":"New","maxTokens":1048576},
						"gemini-4-neon-low": {"displayName":"Gemini 4 Neon (Low)","model":"MODEL_PLACEHOLDER_M534","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"supportsImages":true,"supportsThinking":true,"tagTitle":"New","maxTokens":1048576},
						"gemini-4-carbon-high": {"displayName":"Gemini 4 Carbon (High)","model":"MODEL_PLACEHOLDER_M427","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"supportsImages":true,"supportsThinking":true,"tagTitle":"New","maxTokens":1048576},
						"gemini-4-carbon-medium": {"displayName":"Gemini 4 Carbon (Medium)","model":"MODEL_PLACEHOLDER_M437","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"supportsImages":true,"supportsThinking":true,"tagTitle":"New","maxTokens":1048576},
						"gemini-4-carbon-low": {"displayName":"Gemini 4 Carbon (Low)","model":"MODEL_PLACEHOLDER_M438","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"supportsImages":true,"supportsThinking":true,"tagTitle":"New","maxTokens":1048576},
						"gemini-4-argon-high": {"displayName":"Gemini 4 Argon (High)","model":"MODEL_PLACEHOLDER_M417","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"supportsImages":true,"supportsThinking":true,"maxTokens":1048576},
						"gemini-4-argon-low": {"displayName":"Gemini 4 Argon (Low)","model":"MODEL_PLACEHOLDER_M419","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"supportsImages":true,"supportsThinking":true,"maxTokens":1048576},
						"gemini-4-hydrogen-high": {"displayName":"Gemini 4 Hydrogen (High)","model":"MODEL_PLACEHOLDER_M525","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"supportsImages":true,"supportsThinking":true,"maxTokens":1048576},
						"gemini-4-hydrogen-xhigh": {"displayName":"Gemini 4 Hydrogen (XHigh)","model":"MODEL_PLACEHOLDER_M528","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"supportsImages":true,"supportsThinking":true,"maxTokens":1048576},
						"claude-opus-5-5-low": {"displayName":"Claude Opus 5.5 (Low)","model":"MODEL_PLACEHOLDER_M400","modelProvider":"MODEL_PROVIDER_ANTHROPIC","recommended":true,"supportsImages":true,"supportsThinking":true,"maxTokens":250000},
						"claude-opus-5-5-medium": {"displayName":"Claude Opus 5.5 (Medium)","model":"MODEL_PLACEHOLDER_M401","modelProvider":"MODEL_PROVIDER_ANTHROPIC","recommended":true,"supportsImages":true,"supportsThinking":true,"maxTokens":250000},
						"claude-opus-5-5-high": {"displayName":"Claude Opus 5.5 (High)","model":"MODEL_PLACEHOLDER_M402","modelProvider":"MODEL_PROVIDER_ANTHROPIC","recommended":true,"supportsImages":true,"supportsThinking":true,"maxTokens":250000},
						"claude-sonnet-5-5-high": {"displayName":"Claude Sonnet 5.5 (High)","model":"MODEL_PLACEHOLDER_M405","modelProvider":"MODEL_PROVIDER_ANTHROPIC","recommended":true,"supportsImages":true,"supportsThinking":true,"maxTokens":250000},
						"gemini-pro-orchestrator-neon": {"displayName":"Orchestrator (Neon)","model":"MODEL_PLACEHOLDER_M537","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"supportsImages":true,"supportsThinking":true,"tagTitle":"New"},
						"gemini-pro-orchestrator": {"displayName":"Orchestrator (Carbon)","model":"MODEL_PLACEHOLDER_M667","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"supportsImages":true,"supportsThinking":true,"tagTitle":"New"},
						"gemini-pro-orchestrator-argon": {"displayName":"Orchestrator (Argon)","model":"MODEL_PLACEHOLDER_M681","modelProvider":"MODEL_PROVIDER_GOOGLE","recommended":true,"supportsImages":true,"supportsThinking":true,"tagTitle":"New"}
					},
					"agentModelSorts": [{
						"displayName": "Recommended",
						"groups": [{"modelIds": [
							"gemini-4-neon-high","gemini-4-neon-xhigh","gemini-4-neon-medium","gemini-4-neon-low",
							"gemini-4-carbon-medium","gemini-4-carbon-high","gemini-4-carbon-low",
							"gemini-4-argon-high","gemini-4-argon-low",
							"gemini-4-hydrogen-high","gemini-4-hydrogen-xhigh",
							"gemini-pro-orchestrator-neon","gemini-pro-orchestrator","gemini-pro-orchestrator-argon",
							"claude-opus-5-5-low","claude-opus-5-5-medium","claude-opus-5-5-high",
							"claude-sonnet-5-5-high"
						]}]
					}]
				}
			}`))
		case strings.HasSuffix(r.URL.Path, "/GetMendelFlags"):
			_, _ = w.Write([]byte(`{
				"experimentConfig": {
					"experiments": [
						{"keyString": "enable-orchestrator-model-selection"},
						{"keyString": "orchestrator-model-roles", "string": "{\"orchestrators\":[\"gemini-4-neon\",\"gemini-4-carbon\",\"gemini-4-argon\",\"claude-opus-5-5\"],\"executors\":[\"gemini-4-hydrogen\",\"claude-sonnet-5-5\"]}"}
					]
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	})

	w := httptest.NewRecorder()
	p.HandleModels(w, httptest.NewRequest(http.MethodGet, "/gateway/models?refresh=1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var res ModelsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !res.Live {
		t.Fatal("expected live=true")
	}
	if res.Defaults["orchestrator"] != "gemini-pro-orchestrator-neon-high" {
		t.Fatalf("orchestrator default = %q, want gemini-pro-orchestrator-neon-high", res.Defaults["orchestrator"])
	}

	orch := idsOf(res.Models, "orchestrator")
	wantOrch := []string{
		"gemini-pro-orchestrator-neon-high",
		"gemini-pro-orchestrator-neon-xhigh",
		"gemini-pro-orchestrator-neon-medium",
		"gemini-pro-orchestrator-neon-low",
		"gemini-pro-orchestrator-carbon-medium",
		"gemini-pro-orchestrator-carbon-high",
		"gemini-pro-orchestrator-carbon-low",
		"gemini-pro-orchestrator-argon-high",
		"gemini-pro-orchestrator-argon-low",
		"gemini-pro-orchestrator-opus-5-5-low",
		"gemini-pro-orchestrator-opus-5-5-medium",
		"gemini-pro-orchestrator-opus-5-5-high",
	}
	if strings.Join(orch, ",") != strings.Join(wantOrch, ",") {
		t.Fatalf("expanded orchestrators:\n got  %v\n want %v", orch, wantOrch)
	}

	byID := map[string]ModelOption{}
	for _, m := range res.Models {
		byID[m.ID] = m
		if m.Provider == "orchestrator" {
			if m.Enum != "" {
				t.Errorf("synthetic orchestrator option %s must omit exported Enum to avoid client enum collisions, got %q", m.ID, m.Enum)
			}
			if strings.Contains(strings.ToLower(m.ID), "claude") {
				t.Errorf("synthetic orchestrator ID %s must not contain 'claude'", m.ID)
			}
		}
	}
	if got := byID["gemini-pro-orchestrator-neon-xhigh"].Name; got != "Orchestrator (Neon) (XHigh)" {
		t.Errorf("neon xhigh name = %q", got)
	}
	if got := byID["gemini-pro-orchestrator-neon-xhigh"].QuotaPool; got != "gemini" {
		t.Errorf("neon xhigh quotaPool = %q, want gemini", got)
	}
	if got := byID["gemini-pro-orchestrator-opus-5-5-high"].Name; got != "Orchestrator (Opus 5.5) (High)" {
		t.Errorf("opus 5.5 high name = %q", got)
	}
	if got := byID["gemini-pro-orchestrator-opus-5-5-high"].QuotaPool; got != "claude" {
		t.Errorf("opus 5.5 high quotaPool = %q, want claude", got)
	}

	// Resolution and reverse-lookup isolation
	if got := resolveModelEnum("gemini-pro-orchestrator-neon-xhigh"); got != "MODEL_PLACEHOLDER_M535" {
		t.Errorf("resolveModelEnum(neon-xhigh) = %q, want MODEL_PLACEHOLDER_M535", got)
	}
	if got := resolveModelEnum("gemini-pro-orchestrator-opus-5-5-high"); got != "MODEL_PLACEHOLDER_M402" {
		t.Errorf("resolveModelEnum(opus-5-5-high) = %q, want MODEL_PLACEHOLDER_M402", got)
	}
	if got := resolveModelEnum("gemini-pro-orchestrator-neon"); got != "MODEL_PLACEHOLDER_M537" {
		t.Errorf("legacy orchestrator anchor must stay resolvable, got %q", got)
	}
	if got := canonicalModelName("gemini-pro-orchestrator-neon-xhigh"); got != "gemini-pro-orchestrator-neon-xhigh" {
		t.Errorf("canonicalModelName(neon-xhigh) = %q, must preserve orchestrator ID", got)
	}
	if got := canonicalModelName("MODEL_PLACEHOLDER_M535"); got != "gemini-4-neon-xhigh" {
		t.Errorf("canonicalModelName(M535) = %q, must stay regular gemini-4-neon-xhigh", got)
	}

	// applyModelToCascadeConfig + applyOrchestratorAgentSpec for Gemini planner
	cfg := applyModelToCascadeConfig(nil, "MODEL_PLACEHOLDER_M535", "gemini-pro-orchestrator-neon-xhigh")
	cfgMap := cfg.(map[string]interface{})
	pCfg := cfgMap["plannerConfig"].(map[string]interface{})
	if pCfg["planModel"] != "MODEL_PLACEHOLDER_M535" {
		t.Errorf("planModel = %v, want MODEL_PLACEHOLDER_M535", pCfg["planModel"])
	}
	pec, ok := pCfg["planExecuteConfig"].(map[string]interface{})
	if !ok || pec["executionModel"] != "MODEL_PLACEHOLDER_M525" {
		t.Fatalf("planExecuteConfig = %v, want executionModel=MODEL_PLACEHOLDER_M525", pCfg["planExecuteConfig"])
	}
	msgPayload := map[string]interface{}{}
	applyOrchestratorAgentSpec(msgPayload, cfg)
	agentSpec, _ := msgPayload["customAgentSpec"].(map[string]interface{})
	builtin, _ := agentSpec["builtinAgent"].(map[string]interface{})
	defAgent, _ := builtin["defaultAgent"].(map[string]interface{})
	if defAgent["planExecute"] != true || defAgent["isInteractive"] != true || defAgent["surface"] != "AGENT_SURFACE_HUB" {
		t.Fatalf("unexpected customAgentSpec: %+v", msgPayload["customAgentSpec"])
	}

	// applyModelToCascadeConfig for Claude Opus 5.5 planner must clamp Claude checkpoint limits
	claudeOrchCfg := applyModelToCascadeConfig(cfg, "MODEL_PLACEHOLDER_M402", "gemini-pro-orchestrator-opus-5-5-high").(map[string]interface{})
	cpCfg := claudeOrchCfg["checkpointConfig"].(map[string]interface{})
	if cpCfg["maxTokenLimit"] != 160000 {
		t.Errorf("claude orchestrator maxTokenLimit = %v, want 160000", cpCfg["maxTokenLimit"])
	}
	if claudeOrchCfg["plannerConfig"].(map[string]interface{})["planExecuteConfig"].(map[string]interface{})["executionModel"] != "MODEL_PLACEHOLDER_M525" {
		t.Errorf("claude orchestrator executionModel unexpected: %v", claudeOrchCfg["plannerConfig"])
	}

	// Switching back to normal Gemini model must clear planExecuteConfig and restore checkpoint limits
	normalCfg := applyModelToCascadeConfig(claudeOrchCfg, "MODEL_PLACEHOLDER_M532", "gemini-4-neon-high").(map[string]interface{})
	normalPCfg := normalCfg["plannerConfig"].(map[string]interface{})
	if _, stillHasPEC := normalPCfg["planExecuteConfig"]; stillHasPEC {
		t.Errorf("planExecuteConfig must be removed when switching back to normal model, got %v", normalPCfg["planExecuteConfig"])
	}
	if normalCfg["checkpointConfig"].(map[string]interface{})["maxTokenLimit"] != 256000 {
		t.Errorf("normal gemini maxTokenLimit = %v, want 256000", normalCfg["checkpointConfig"])
	}

	// Desktop-initiated planExecute trajectory (no modelName) reverse-maps to synthetic orchestrator ID
	desktopOrchResp := &upstreamTrajectoryResp{}
	desktopOrchResp.Trajectory.ExecutorMetadatas = []struct {
		CascadeConfig json.RawMessage `json:"cascadeConfig"`
	}{
		{CascadeConfig: json.RawMessage(`{"plannerConfig":{"planModel":"MODEL_PLACEHOLDER_M535","planExecuteConfig":{"executionModel":"MODEL_PLACEHOLDER_M525"}}}`)},
	}
	if activeModel, _ := resolveActiveModel(desktopOrchResp); activeModel != "gemini-pro-orchestrator-neon-xhigh" {
		t.Errorf("resolveActiveModel(desktop planExecute) = %q, want gemini-pro-orchestrator-neon-xhigh", activeModel)
	}

	// Normal trajectory with same planModel but no planExecuteConfig resolves to regular Gemini model
	normalResp := &upstreamTrajectoryResp{}
	normalResp.Trajectory.ExecutorMetadatas = []struct {
		CascadeConfig json.RawMessage `json:"cascadeConfig"`
	}{
		{CascadeConfig: json.RawMessage(`{"plannerConfig":{"planModel":"MODEL_PLACEHOLDER_M535"}}`)},
	}
	if activeModel, _ := resolveActiveModel(normalResp); activeModel != "gemini-4-neon-xhigh" {
		t.Errorf("resolveActiveModel(normal) = %q, want gemini-4-neon-xhigh", activeModel)
	}
}

func TestExpandOrchestratorOptions_GatesAndDesktopExecutorPref(t *testing.T) {
	g := "MODEL_PROVIDER_GOOGLE"
	a := "MODEL_PROVIDER_ANTHROPIC"
	raw := map[string]upstreamModel{
		"gemini-4-neon-high":     {DisplayName: "Gemini 4 Neon (High)", Model: "MODEL_PLACEHOLDER_M532", ModelProvider: g, Recommended: true},
		"gemini-4-neon-low":      {DisplayName: "Gemini 4 Neon (Low)", Model: "MODEL_PLACEHOLDER_M534", ModelProvider: g, Recommended: true},
		"gemini-4-hydrogen-high": {DisplayName: "Gemini 4 Hydrogen (High)", Model: "MODEL_PLACEHOLDER_M525", ModelProvider: g, Recommended: true},
		"claude-sonnet-5-5-high": {DisplayName: "Claude Sonnet 5.5 (High)", Model: "MODEL_PLACEHOLDER_M405", ModelProvider: a, Recommended: true},
	}
	roles := &orchestratorRoles{
		Enabled:       true,
		Orchestrators: []string{"gemini-4-neon"},
		Executors:     []string{"gemini-4-hydrogen", "claude-sonnet-5-5"},
	}

	// Non-Ultra account without any orchestrator anchor must NOT get synthetic orchestrator rows
	optsNoAnchor, byIDNoAnchor, _ := buildModelTables(raw, []string{"gemini-4-neon-high", "gemini-4-neon-low", "gemini-4-hydrogen-high", "claude-sonnet-5-5-high"})
	merged, specs, _ := expandOrchestratorOptions(optsNoAnchor, byIDNoAnchor, roles, "")
	if len(idsOf(merged, "orchestrator")) != 0 || len(specs) != 0 {
		t.Fatalf("non-anchor account must not expand orchestrators, got opts=%v specs=%v", idsOf(merged, "orchestrator"), specs)
	}

	// With orchestrator anchor + userSettings.orchestratorExecutorModel pointing to Claude Sonnet 5.5 High
	raw["gemini-pro-orchestrator-neon"] = upstreamModel{DisplayName: "Orchestrator (Neon)", Model: "MODEL_PLACEHOLDER_M537", ModelProvider: g, Recommended: true}
	optsWithAnchor, byIDWithAnchor, _ := buildModelTables(raw, []string{"gemini-4-neon-high", "gemini-4-neon-low", "gemini-4-hydrogen-high", "claude-sonnet-5-5-high", "gemini-pro-orchestrator-neon"})
	_, specsPref, _ := expandOrchestratorOptions(optsWithAnchor, byIDWithAnchor, roles, "MODEL_PLACEHOLDER_M405")
	if got := specsPref["gemini-pro-orchestrator-neon-high"].ExecutionEnum; got != "MODEL_PLACEHOLDER_M405" {
		t.Fatalf("preferred desktop executor = %q, want MODEL_PLACEHOLDER_M405", got)
	}
}

func TestHandleJetboxWriteState_PreservesOrchestratorEffortOnAnchorEnum(t *testing.T) {
	resetLiveModels(t)
	cid := "test-jetbox-orch-preserve"
	SetCascadeModel(cid, "gemini-pro-orchestrator-carbon-medium", []byte(`{"plannerConfig":{"planModel":"MODEL_PLACEHOLDER_M437","modelName":"gemini-pro-orchestrator-carbon-medium","planExecuteConfig":{"executionModel":"MODEL_PLACEHOLDER_M525"}}}`))
	t.Cleanup(func() {
		defaultTrajCache.cascadeModelsMu.Lock()
		delete(defaultTrajCache.cascadeModels, cid)
		defaultTrajCache.cascadeModelsMu.Unlock()
	})

	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	noopRP := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	req := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/JetboxWriteState",
		strings.NewReader(`{"appState":{"lastSelectedAgentModel":"MODEL_PLACEHOLDER_M537"}}`))
	req.Header.Set("X-Cascade-Id", cid)
	rec := httptest.NewRecorder()
	p.handleJetboxWriteState(rec, req, noopRP, "/exa.language_server_pb.LanguageServerService/JetboxWriteState")

	if got, _ := GetCascadeModel(cid); got != "gemini-pro-orchestrator-carbon-medium" {
		t.Fatalf("JetboxWriteState anchor enum clobbered cached orchestrator effort: got %q, want gemini-pro-orchestrator-carbon-medium", got)
	}

	// Explicit switch to a non-orchestrator model must still update normally
	req2 := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/JetboxWriteState",
		strings.NewReader(`{"appState":{"lastSelectedAgentModel":"MODEL_PLACEHOLDER_M532"}}`))
	req2.Header.Set("X-Cascade-Id", cid)
	rec2 := httptest.NewRecorder()
	p.handleJetboxWriteState(rec2, req2, noopRP, "/exa.language_server_pb.LanguageServerService/JetboxWriteState")

	if got, _ := GetCascadeModel(cid); got != "gemini-4-neon-high" {
		t.Fatalf("JetboxWriteState switch to regular model failed: got %q, want gemini-4-neon-high", got)
	}
}

func TestOrchestratorBareAnchorResolution_NeverDropsPlanExecuteConfig(t *testing.T) {
	resetLiveModels(t)

	cases := []struct {
		inputModel     string
		inputEnum      string
		wantID         string
		wantPlanner    string
		wantExecNonNil bool
	}{
		{"MODEL_PLACEHOLDER_M537", "MODEL_PLACEHOLDER_M537", "gemini-pro-orchestrator-neon-high", "MODEL_PLACEHOLDER_M532", true},
		{"gemini-pro-orchestrator-neon", "MODEL_PLACEHOLDER_M537", "gemini-pro-orchestrator-neon-high", "MODEL_PLACEHOLDER_M532", true},
		{"MODEL_PLACEHOLDER_M667", "MODEL_PLACEHOLDER_M667", "gemini-pro-orchestrator-carbon-high", "MODEL_PLACEHOLDER_M420", true},
		{"gemini-pro-orchestrator", "MODEL_PLACEHOLDER_M667", "gemini-pro-orchestrator-carbon-high", "MODEL_PLACEHOLDER_M420", true},
		{"MODEL_PLACEHOLDER_M681", "MODEL_PLACEHOLDER_M681", "gemini-pro-orchestrator-argon-high", "MODEL_PLACEHOLDER_M417", true},
		{"gemini-pro-orchestrator-argon", "MODEL_PLACEHOLDER_M681", "gemini-pro-orchestrator-argon-high", "MODEL_PLACEHOLDER_M417", true},
	}

	for _, tc := range cases {
		cfg := applyModelToCascadeConfig(nil, tc.inputEnum, tc.inputModel)
		cfgMap, ok := cfg.(map[string]interface{})
		if !ok {
			t.Fatalf("%s: expected map cascadeConfig, got %T", tc.inputModel, cfg)
		}
		pCfg, _ := cfgMap["plannerConfig"].(map[string]interface{})
		if pCfg["modelName"] != tc.wantID {
			t.Errorf("%s: modelName = %v, want %q", tc.inputModel, pCfg["modelName"], tc.wantID)
		}
		if pCfg["planModel"] != tc.wantPlanner {
			t.Errorf("%s: planModel = %v, want %q", tc.inputModel, pCfg["planModel"], tc.wantPlanner)
		}
		if !cascadeConfigHasPlanExecute(cfgMap) {
			t.Errorf("%s: expected non-empty planExecuteConfig.executionModel, got %v", tc.inputModel, pCfg["planExecuteConfig"])
		}
	}
}

func TestResolveActiveModel_PlanExecuteColdStartAndExecutorMetadata(t *testing.T) {
	resetLiveModels(t)

	// 1. Cold start (liveModels empty): planner M535 + planExecuteConfig must NOT collapse to regular gemini-4-neon-xhigh
	coldStartResp := &upstreamTrajectoryResp{}
	coldStartResp.Trajectory.ExecutorMetadatas = []struct {
		CascadeConfig json.RawMessage `json:"cascadeConfig"`
	}{
		{CascadeConfig: json.RawMessage(`{"plannerConfig":{"planModel":"MODEL_PLACEHOLDER_M535","planExecuteConfig":{"executionModel":"MODEL_PLACEHOLDER_M525"}}}`)},
	}
	if got, _ := resolveActiveModel(coldStartResp); got != "gemini-pro-orchestrator-neon-xhigh" {
		t.Fatalf("cold-start planExecute resolveActiveModel = %q, want gemini-pro-orchestrator-neon-xhigh", got)
	}

	// 2. Alternating planner + executor metadata where the newest entry is the executor worker (M525)
	alternatingResp := &upstreamTrajectoryResp{}
	alternatingResp.Trajectory.Annotations = &struct {
		Title                  string            `json:"title"`
		LastUserViewTime       string            `json:"lastUserViewTime"`
		PlanExecute            bool              `json:"planExecute,omitempty"`
		SideQuestionCascadeIDs []string          `json:"sideQuestionCascadeIds,omitempty"`
		SideQuestionTitles     map[string]string `json:"sideQuestionTitles,omitempty"`
	}{PlanExecute: true}
	alternatingResp.Trajectory.ExecutorMetadatas = []struct {
		CascadeConfig json.RawMessage `json:"cascadeConfig"`
	}{
		{CascadeConfig: json.RawMessage(`{"plannerConfig":{"planModel":"MODEL_PLACEHOLDER_M402","planExecuteConfig":{"executionModel":"MODEL_PLACEHOLDER_M525"}}}`)},
		{CascadeConfig: json.RawMessage(`{"plannerConfig":{"planModel":"MODEL_PLACEHOLDER_M525"}}`)},
	}
	if got, _ := resolveActiveModel(alternatingResp); got != "gemini-pro-orchestrator-opus-5-5-high" {
		t.Fatalf("alternating planner/executor resolveActiveModel = %q, want gemini-pro-orchestrator-opus-5-5-high", got)
	}

	// 3. PlanExecute annotation true with unmappable enum must still stay on a synthetic Orchestrator ID
	unknownPlanExecResp := &upstreamTrajectoryResp{}
	unknownPlanExecResp.Trajectory.Annotations = &struct {
		Title                  string            `json:"title"`
		LastUserViewTime       string            `json:"lastUserViewTime"`
		PlanExecute            bool              `json:"planExecute,omitempty"`
		SideQuestionCascadeIDs []string          `json:"sideQuestionCascadeIds,omitempty"`
		SideQuestionTitles     map[string]string `json:"sideQuestionTitles,omitempty"`
	}{PlanExecute: true}
	unknownPlanExecResp.Trajectory.ExecutorMetadatas = []struct {
		CascadeConfig json.RawMessage `json:"cascadeConfig"`
	}{
		{CascadeConfig: json.RawMessage(`{"plannerConfig":{"planModel":"MODEL_PLACEHOLDER_M999","planExecuteConfig":{"executionModel":"MODEL_PLACEHOLDER_M525"}}}`)},
	}
	if got, _ := resolveActiveModel(unknownPlanExecResp); !isSyntheticOrchestratorID(got) {
		t.Fatalf("unknown planExecute resolveActiveModel = %q, expected synthetic orchestrator ID", got)
	}
}

func TestHandleSendUserCascadeMessage_PlanExecuteConsistency(t *testing.T) {
	resetLiveModels(t)
	cid := "test-send-plan-exec-consistency"
	SetCascadeModel(cid, "gemini-pro-orchestrator-opus-5-5-high", []byte(`{"plannerConfig":{"planModel":"MODEL_PLACEHOLDER_M402","modelName":"gemini-pro-orchestrator-opus-5-5-high","planExecuteConfig":{"executionModel":"MODEL_PLACEHOLDER_M525"}}}`))
	SetCascadePlanExecute(cid, true)
	t.Cleanup(func() {
		defaultTrajCache.cascadeModelsMu.Lock()
		delete(defaultTrajCache.cascadeModels, cid)
		delete(defaultTrajCache.cascadePlanExecute, cid)
		defaultTrajCache.cascadeModelsMu.Unlock()
		defaultTrajCache.cascadeConfigsMu.Lock()
		delete(defaultTrajCache.cascadeConfigs, cid)
		defaultTrajCache.cascadeConfigsMu.Unlock()
	})

	var eventsMu sync.Mutex
	var annotationEvents []map[string]interface{}
	var eventOrder []string
	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/UpdateConversationAnnotations") {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			eventsMu.Lock()
			annotationEvents = append(annotationEvents, body)
			eventOrder = append(eventOrder, "annotations")
			eventsMu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})
	port, token := p.ActiveUpstream()

	var forwardedSend map[string]interface{}
	captureRP := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eventsMu.Lock()
		eventOrder = append(eventOrder, "send")
		eventsMu.Unlock()
		forwardedSend = nil
		_ = json.NewDecoder(r.Body).Decode(&forwardedSend)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	// 1. Bare anchor M537 from an older client on an existing Opus 5.5 Orchestrator cascade
	//    must preserve Opus 5.5 + planExecuteConfig + planExecute:true.
	req1 := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage",
		strings.NewReader(`{"cascadeId":"`+cid+`","text":"continue"}`))
	req1.Header.Set("X-Antigravity-Model", "MODEL_PLACEHOLDER_M537")
	rec1 := httptest.NewRecorder()
	p.handleSendUserCascadeMessage(rec1, req1, captureRP, "/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", port, token)

	if !cascadeConfigHasPlanExecute(forwardedSend["cascadeConfig"]) {
		t.Fatalf("bare anchor stripped planExecuteConfig: %+v", forwardedSend["cascadeConfig"])
	}
	pCfg1 := forwardedSend["cascadeConfig"].(map[string]interface{})["plannerConfig"].(map[string]interface{})
	if pCfg1["modelName"] != "gemini-pro-orchestrator-opus-5-5-high" || pCfg1["planModel"] != "MODEL_PLACEHOLDER_M402" {
		t.Fatalf("bare anchor failed to preserve cached orchestrator variant: %+v", pCfg1)
	}
	defAgent1 := forwardedSend["customAgentSpec"].(map[string]interface{})["builtinAgent"].(map[string]interface{})["defaultAgent"].(map[string]interface{})
	if defAgent1["planExecute"] != true {
		t.Fatalf("expected customAgentSpec...planExecute=true, got %+v", defAgent1)
	}

	// 2. Switching away to a regular Gemini model must remove planExecuteConfig, explicitly set
	//    customAgentSpec...planExecute=false, and synchronously clear annotations.planExecute=false
	//    before forwarding SendUserCascadeMessage.
	annotationEvents = nil
	eventOrder = nil
	req2 := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage",
		strings.NewReader(`{"cascadeId":"`+cid+`","text":"switch to regular","customAgentSpec":{"builtinAgent":{"defaultAgent":{"isInteractive":true,"surface":"AGENT_SURFACE_HUB","planExecute":true}}}}`))
	req2.Header.Set("X-Antigravity-Model", "gemini-4-neon-high")
	rec2 := httptest.NewRecorder()
	p.handleSendUserCascadeMessage(rec2, req2, captureRP, "/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", port, token)

	if cascadeConfigHasPlanExecute(forwardedSend["cascadeConfig"]) {
		t.Fatalf("switching away did not remove planExecuteConfig: %+v", forwardedSend["cascadeConfig"])
	}
	defAgent2 := forwardedSend["customAgentSpec"].(map[string]interface{})["builtinAgent"].(map[string]interface{})["defaultAgent"].(map[string]interface{})
	if defAgent2["planExecute"] != false {
		t.Fatalf("switching away did not clear customAgentSpec...planExecute=false: %+v", defAgent2)
	}
	if len(annotationEvents) == 0 {
		t.Fatal("expected UpdateConversationAnnotations to clear planExecute=false on switch away")
	}
	annMap, _ := annotationEvents[0]["annotations"].(map[string]interface{})
	if annMap["planExecute"] != false || annotationEvents[0]["mergeAnnotations"] != true {
		t.Fatalf("unexpected UpdateConversationAnnotations payload: %+v", annotationEvents[0])
	}
	if len(eventOrder) < 2 || eventOrder[0] != "annotations" || eventOrder[1] != "send" {
		t.Fatalf("expected annotations clear before send, got order %v", eventOrder)
	}
	if IsCascadePlanExecute(cid) {
		t.Fatal("expected IsCascadePlanExecute to be cleared after switching away")
	}

	// 3. Switching back from a regular model to an Orchestrator model must synchronously set
	//    annotations.planExecute=true before sending and restore planExecuteConfig + customAgentSpec.
	eventsMu.Lock()
	annotationEvents = nil
	eventOrder = nil
	eventsMu.Unlock()
	req3 := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage",
		strings.NewReader(`{"cascadeId":"`+cid+`","text":"switch back to orch"}`))
	req3.Header.Set("X-Antigravity-Model", "gemini-pro-orchestrator-neon-xhigh")
	rec3 := httptest.NewRecorder()
	p.handleSendUserCascadeMessage(rec3, req3, captureRP, "/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", port, token)

	if !cascadeConfigHasPlanExecute(forwardedSend["cascadeConfig"]) {
		t.Fatalf("switching back to orchestrator missing planExecuteConfig: %+v", forwardedSend["cascadeConfig"])
	}
	defAgent3 := forwardedSend["customAgentSpec"].(map[string]interface{})["builtinAgent"].(map[string]interface{})["defaultAgent"].(map[string]interface{})
	if defAgent3["planExecute"] != true {
		t.Fatalf("switching back to orchestrator did not restore customAgentSpec...planExecute=true: %+v", defAgent3)
	}
	eventsMu.Lock()
	ann3Count := len(annotationEvents)
	var ann3 map[string]interface{}
	if ann3Count > 0 {
		ann3 = annotationEvents[0]
	}
	order3 := append([]string(nil), eventOrder...)
	eventsMu.Unlock()
	if ann3Count == 0 {
		t.Fatal("expected UpdateConversationAnnotations to set planExecute=true when re-entering Orchestrator mode")
	}
	if ann3["annotations"].(map[string]interface{})["planExecute"] != true || ann3["mergeAnnotations"] != true {
		t.Fatalf("unexpected re-enter UpdateConversationAnnotations payload: %+v", ann3)
	}
	if len(order3) < 2 || order3[0] != "annotations" || order3[1] != "send" {
		t.Fatalf("expected annotations enable before send, got order %v", order3)
	}
	if !IsCascadePlanExecute(cid) {
		t.Fatal("expected IsCascadePlanExecute to be true after re-entering Orchestrator mode")
	}
}

func TestHandleJetboxWriteState_SyntheticIDAndColdStartAnchor(t *testing.T) {
	resetLiveModels(t)
	cid1 := "test-jetbox-synth-id"
	cid2 := "test-jetbox-cold-anchor"
	t.Cleanup(func() {
		defaultTrajCache.cascadeModelsMu.Lock()
		delete(defaultTrajCache.cascadeModels, cid1)
		delete(defaultTrajCache.cascadeModels, cid2)
		defaultTrajCache.cascadeModelsMu.Unlock()
		defaultTrajCache.cascadeConfigsMu.Lock()
		delete(defaultTrajCache.cascadeConfigs, cid1)
		delete(defaultTrajCache.cascadeConfigs, cid2)
		defaultTrajCache.cascadeConfigsMu.Unlock()
	})

	p := newFakeLSProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	var upstreamModel string
	captureRP := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AppState struct {
				LastSelectedAgentModel string `json:"lastSelectedAgentModel"`
			} `json:"appState"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		upstreamModel = body.AppState.LastSelectedAgentModel
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	// 1. Synthetic Orchestrator ID sent by iOS/Android/Web:
	//    local cache preserves exact synthetic ID + planExecuteConfig, while upstream receives valid enum M402.
	req1 := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/JetboxWriteState",
		strings.NewReader(`{"appState":{"lastSelectedAgentModel":"gemini-pro-orchestrator-opus-5-5-high"}}`))
	req1.Header.Set("X-Cascade-Id", cid1)
	p.handleJetboxWriteState(httptest.NewRecorder(), req1, captureRP, "/exa.language_server_pb.LanguageServerService/JetboxWriteState")

	gotModel1, gotCfg1 := GetCascadeModel(cid1)
	if gotModel1 != "gemini-pro-orchestrator-opus-5-5-high" {
		t.Fatalf("cached model = %q, want gemini-pro-orchestrator-opus-5-5-high", gotModel1)
	}
	var cfg1Obj interface{}
	_ = json.Unmarshal(gotCfg1, &cfg1Obj)
	if !cascadeConfigHasPlanExecute(cfg1Obj) {
		t.Fatalf("cached config missing planExecuteConfig: %s", string(gotCfg1))
	}
	if upstreamModel != "MODEL_PLACEHOLDER_M402" {
		t.Fatalf("upstream JetboxWriteState lastSelectedAgentModel = %q, want MODEL_PLACEHOLDER_M402", upstreamModel)
	}

	// 2. Cold-start cascade receiving bare anchor MODEL_PLACEHOLDER_M537 must cache a valid
	//    synthetic Orchestrator spec with planExecuteConfig instead of stripping it.
	req2 := httptest.NewRequest(http.MethodPost, "/api/exa.language_server_pb.LanguageServerService/JetboxWriteState",
		strings.NewReader(`{"appState":{"lastSelectedAgentModel":"MODEL_PLACEHOLDER_M537"}}`))
	req2.Header.Set("X-Cascade-Id", cid2)
	p.handleJetboxWriteState(httptest.NewRecorder(), req2, captureRP, "/exa.language_server_pb.LanguageServerService/JetboxWriteState")

	gotModel2, gotCfg2 := GetCascadeModel(cid2)
	if gotModel2 != "gemini-pro-orchestrator-neon-high" {
		t.Fatalf("cold-start anchor cached model = %q, want gemini-pro-orchestrator-neon-high", gotModel2)
	}
	var cfg2Obj interface{}
	_ = json.Unmarshal(gotCfg2, &cfg2Obj)
	if !cascadeConfigHasPlanExecute(cfg2Obj) {
		t.Fatalf("cold-start anchor cached config missing planExecuteConfig: %s", string(gotCfg2))
	}
}
