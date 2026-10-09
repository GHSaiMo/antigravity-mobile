package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 动态模型列表。
//
// language_server 的 GetAvailableModels 返回当前账号可用的全部模型（含内部占位、已改名的旧 id）。
// 过去网关用两张手写表（modelEnumMap / enumToCanonicalMap）做 id ↔ 枚举映射，官方一换模型就过期。
// 这里把实时列表缓存成注册表：
//   - /gateway/models 给客户端「默认模型」选择用，只返回去重后的推荐模型，按 Gemini / Claude 分组；
//   - resolveModelEnum 优先用实时列表把 id 解析成枚举，canonicalModelName 在静态表未命中时用它反查，
//     所以新模型无需改代码即可被选择、发送和在界面上显示。

const (
	liveModelsTTL     = 5 * time.Minute
	liveModelsTimeout = 4 * time.Second
	liveModelsRetry   = 30 * time.Second // 失败后至少间隔多久再试，避免每条消息都等 4 秒超时
)

// ModelOption is one selectable model as exposed to clients.
type ModelOption struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Provider       string `json:"provider"` // "gemini" | "claude" | "other"
	Enum           string `json:"enum,omitempty"`
	SupportsImages bool   `json:"supportsImages,omitempty"`
	Thinking       bool   `json:"thinking,omitempty"`
}

// ModelsResponse is the response of /gateway/models.
type ModelsResponse struct {
	Models []ModelOption `json:"models"`
	// Defaults are the gateway's suggested defaults per provider; clients may override them in settings.
	Defaults map[string]string `json:"defaults"`
	// Live is false when the list is a built-in fallback because language_server could not be queried.
	Live bool `json:"live"`
}

// 兜底默认值：实时列表里存在时才会被采用。
var preferredDefaults = map[string]string{
	"gemini": "gemini-3.8-flash-high",
	"claude": "claude-opus-4-6-thinking",
}

// fallbackModels is used until the first successful query, or when language_server is unreachable.
var fallbackModels = []ModelOption{
	{ID: "gemini-3.8-flash-high", Name: "Gemini 3.8 Flash (High)", Provider: "gemini", Enum: "MODEL_PLACEHOLDER_M318"},
	{ID: "gemini-3.8-flash-medium", Name: "Gemini 3.8 Flash (Medium)", Provider: "gemini", Enum: "MODEL_PLACEHOLDER_M319"},
	{ID: "gemini-3.8-flash-low", Name: "Gemini 3.8 Flash (Low)", Provider: "gemini", Enum: "MODEL_PLACEHOLDER_M320"},
	{ID: "gemini-3.1-pro-high", Name: "Gemini 3.1 Pro (High)", Provider: "gemini", Enum: "MODEL_PLACEHOLDER_M37"},
	{ID: "gemini-3.1-pro-low", Name: "Gemini 3.1 Pro (Low)", Provider: "gemini", Enum: "MODEL_PLACEHOLDER_M36"},
	{ID: "claude-opus-4-6-thinking", Name: "Claude Opus 4.6 (Thinking)", Provider: "claude", Enum: "MODEL_PLACEHOLDER_M26"},
	{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6 (Thinking)", Provider: "claude", Enum: "MODEL_PLACEHOLDER_M35"},
}

// upstreamModel mirrors the fields of GetAvailableModels we care about.
type upstreamModel struct {
	DisplayName    string `json:"displayName"`
	Model          string `json:"model"`
	ModelProvider  string `json:"modelProvider"`
	Recommended    bool   `json:"recommended"`
	IsInternal     bool   `json:"isInternal"`
	SupportsImages bool   `json:"supportsImages"`
	Thinking       bool   `json:"supportsThinking"`
	TagTitle       string `json:"tagTitle"`
}

// upstreamModelList is the whole GetAvailableModels payload: the model map plus the vendor's own
// picker curation (agentModelSorts) and default (defaultAgentModelId).
type upstreamModelList struct {
	Models         map[string]upstreamModel
	CuratedIDs     []string // ids of the vendor's "Recommended" agent picker, in display order
	DefaultAgentID string
}

// modelRegistry caches the curated and the full model tables.
type modelRegistry struct {
	mu          sync.RWMutex
	fetchedAt   time.Time // last successful refresh
	lastAttempt time.Time // last refresh attempt, successful or not (failure back-off)
	inflight    bool
	options     []ModelOption          // curated: the vendor's picker list (or a heuristic fallback)
	officialDef string                 // vendor's defaultAgentModelId
	byID        map[string]ModelOption // every named model, keyed by lower-case id
	byEnum      map[string]ModelOption // enum -> preferred option
}

var liveModels = &modelRegistry{}

func providerOf(modelProvider, id string) string {
	lower := strings.ToLower(modelProvider + " " + id)
	switch {
	case strings.Contains(lower, "anthropic") || strings.Contains(lower, "claude"):
		return "claude"
	case strings.Contains(lower, "google") || strings.Contains(lower, "gemini"):
		return "gemini"
	default:
		return "other"
	}
}

// isLeavingSoon reports the vendor's "about to be removed" label.
func isLeavingSoon(tag string) bool {
	return strings.EqualFold(strings.TrimSpace(tag), "Leaving Soon")
}

// isRetiredEnum reports enums that language_server still lists but the gateway remaps elsewhere.
func isRetiredEnum(enum string) bool {
	return strings.HasPrefix(enum, "MODEL_GOOGLE_GEMINI_2_5_")
}

// legacyRank orders duplicate ids that share one display name: lower is preferred.
// Legacy 2.5 aliases and "-agent" aliases lose to the id that carries the real version.
func legacyRank(id string) int {
	switch {
	case strings.Contains(id, "2.5"):
		return 2
	case strings.HasSuffix(id, "-agent"):
		return 1
	default:
		return 0
	}
}

var (
	versionRe  = regexp.MustCompile(`\d+(?:\.\d+)*`)
	effortRank = map[string]int{"high": 0, "medium": 1, "low": 2}
)

// versionKey extracts the numeric version from a display name ("Gemini 3.8 Flash (High)" -> [3 8]).
func versionKey(name string) []int {
	m := versionRe.FindString(name)
	if m == "" {
		return nil
	}
	var out []int
	for _, part := range strings.Split(m, ".") {
		n, _ := strconv.Atoi(part)
		out = append(out, n)
	}
	return out
}

func effortOf(name string) int {
	lower := strings.ToLower(name)
	for k, v := range effortRank {
		if strings.Contains(lower, "("+k+")") {
			return v
		}
	}
	return 3
}

func lessVersionDesc(a, b []int) (less, decided bool) {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] > b[i], true
		}
	}
	if len(a) != len(b) {
		return len(a) > len(b), true
	}
	return false, false
}

// buildModelTables turns raw upstream models into client options plus lookup tables.
//
// When curatedIDs (the vendor's own "Recommended" picker list) is present it decides both membership
// and order, so the phone shows exactly what the desktop app offers. Otherwise a heuristic is used:
// recommended models, de-duplicated by display name, newest version first.
func buildModelTables(raw map[string]upstreamModel, curatedIDs []string) (options []ModelOption, byID, byEnum map[string]ModelOption) {
	byID = map[string]ModelOption{}
	byEnum = map[string]ModelOption{}

	// 稳定遍历顺序，保证去重结果可复现
	ids := make([]string, 0, len(raw))
	for id := range raw {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	bestByName := map[string]ModelOption{}
	leavingSoon := map[string]bool{} // 厂商标了「Leaving Soon」的模型：仍可按 id 解析（旧会话还在用），但不再提供选择
	for _, id := range ids {
		m := raw[id]
		if m.DisplayName == "" || m.IsInternal || strings.HasPrefix(id, "chat_") {
			continue
		}
		opt := ModelOption{
			ID:             id,
			Name:           m.DisplayName,
			Provider:       providerOf(m.ModelProvider, id),
			Enum:           m.Model,
			SupportsImages: m.SupportsImages,
			Thinking:       m.Thinking,
		}
		byID[strings.ToLower(id)] = opt
		if isLeavingSoon(m.TagTitle) {
			leavingSoon[opt.ID] = true
		}

		if prev, ok := byEnum[opt.Enum]; !ok || legacyRank(opt.ID) < legacyRank(prev.ID) {
			if opt.Enum != "" {
				byEnum[opt.Enum] = opt
			}
		}
		// 退役的 2.5 枚举会被 normalizeLegacyEnum 改映射到别的模型，列出来只会让用户选 A 跑 B
		if !m.Recommended || isRetiredEnum(opt.Enum) || isLeavingSoon(m.TagTitle) {
			continue
		}
		if prev, ok := bestByName[opt.Name]; !ok || legacyRank(opt.ID) < legacyRank(prev.ID) {
			bestByName[opt.Name] = opt
		}
	}

	if len(curatedIDs) > 0 {
		seen := map[string]bool{}
		for _, id := range curatedIDs {
			opt, ok := byID[strings.ToLower(id)]
			if !ok || seen[opt.ID] || isRetiredEnum(opt.Enum) || leavingSoon[opt.ID] {
				continue
			}
			seen[opt.ID] = true
			options = append(options, opt)
		}
		if len(options) > 0 {
			// 厂商分组（gemini / claude / other），组内保持官方顺序
			rank := map[string]int{"gemini": 0, "claude": 1, "other": 2}
			sort.SliceStable(options, func(i, j int) bool { return rank[options[i].Provider] < rank[options[j].Provider] })
			return options, byID, byEnum
		}
	}

	for _, opt := range bestByName {
		options = append(options, opt)
	}
	sort.SliceStable(options, func(i, j int) bool {
		a, b := options[i], options[j]
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		if less, decided := lessVersionDesc(versionKey(a.Name), versionKey(b.Name)); decided {
			return less
		}
		if ea, eb := effortOf(a.Name), effortOf(b.Name); ea != eb {
			return ea < eb
		}
		return a.Name < b.Name
	})
	return options, byID, byEnum
}

func (r *modelRegistry) set(options []ModelOption, byID, byEnum map[string]ModelOption, officialDefault string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.options, r.byID, r.byEnum, r.officialDef = options, byID, byEnum, officialDefault
	r.fetchedAt = time.Now()
}

func (r *modelRegistry) enumForID(id string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byID[strings.ToLower(strings.TrimSpace(id))].Enum
}

func (r *modelRegistry) idForEnum(enum string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.byEnum[enum].ID
}

// displayName resolves an id or enum to its human-readable name ("" when unknown).
func (r *modelRegistry) displayName(idOrEnum string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := strings.TrimSpace(idOrEnum)
	if o, ok := r.byID[strings.ToLower(key)]; ok {
		return o.Name
	}
	if o, ok := r.byEnum[key]; ok {
		return o.Name
	}
	for _, o := range fallbackModels {
		if o.ID == key || o.Enum == key {
			return o.Name
		}
	}
	return ""
}

func (r *modelRegistry) snapshot() (ModelsResponse, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	options := r.options
	live := len(options) > 0
	if !live {
		options = fallbackModels
	}
	out := ModelsResponse{Models: append([]ModelOption(nil), options...), Defaults: map[string]string{}, Live: live}
	// 厂商给的默认 agent 模型优先，其次是网关内置的偏好，最后是该厂商列表里的第一个
	wanted := map[string]string{}
	for provider, id := range preferredDefaults {
		wanted[provider] = id
	}
	if r.officialDef != "" {
		for _, o := range out.Models {
			if o.ID == r.officialDef && o.Provider != "other" {
				wanted[o.Provider] = o.ID
			}
		}
	}
	for provider, want := range wanted {
		chosen := ""
		for _, o := range out.Models {
			if o.Provider == provider && o.ID == want {
				chosen = o.ID
			}
		}
		if chosen == "" {
			for _, o := range out.Models {
				if o.Provider == provider {
					chosen = o.ID
					break
				}
			}
		}
		if chosen != "" {
			out.Defaults[provider] = chosen
		}
	}
	return out, live
}

// beginRefresh reserves a refresh slot. It refuses when the cache is fresh, another refresh is
// running, or the last attempt failed less than liveModelsRetry ago.
func (r *modelRegistry) beginRefresh(force bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.inflight {
		return false
	}
	fresh := len(r.options) > 0 && time.Since(r.fetchedAt) <= liveModelsTTL
	if fresh && !force {
		return false
	}
	if !force && time.Since(r.lastAttempt) < liveModelsRetry {
		return false
	}
	r.inflight = true
	r.lastAttempt = time.Now()
	return true
}

// needsRefresh is a cheap read-only check so hot paths can skip spawning a goroutine.
func (r *modelRegistry) needsRefresh() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.inflight {
		return false
	}
	if len(r.options) > 0 && time.Since(r.fetchedAt) <= liveModelsTTL {
		return false
	}
	return time.Since(r.lastAttempt) >= liveModelsRetry
}

func (r *modelRegistry) endRefresh() {
	r.mu.Lock()
	r.inflight = false
	r.mu.Unlock()
}

// fetchUpstreamModels queries GetAvailableModels.
func (p *Proxy) fetchUpstreamModels(ctx context.Context, port int, token string) (*upstreamModelList, error) {
	apiURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetAvailableModels", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}
	resp, err := p.mediumClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GetAvailableModels returned HTTP %d", resp.StatusCode)
	}
	var parsed struct {
		Response struct {
			Models              map[string]upstreamModel `json:"models"`
			DefaultAgentModelID string                   `json:"defaultAgentModelId"`
			AgentModelSorts     []struct {
				DisplayName string `json:"displayName"`
				Groups      []struct {
					ModelIDs []string `json:"modelIds"`
				} `json:"groups"`
			} `json:"agentModelSorts"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	out := &upstreamModelList{Models: parsed.Response.Models, DefaultAgentID: parsed.Response.DefaultAgentModelID}
	// 取第一个排序方案（"Recommended"）作为选择器列表
	if len(parsed.Response.AgentModelSorts) > 0 {
		for _, g := range parsed.Response.AgentModelSorts[0].Groups {
			out.CuratedIDs = append(out.CuratedIDs, g.ModelIDs...)
		}
	}
	return out, nil
}

// ensureLiveModels refreshes the registry when it is stale, blocking for at most liveModelsTimeout.
// Cheap when the cache is fresh; failures keep whatever was cached before. Call it before resolving
// a client-supplied model id.
func (p *Proxy) ensureLiveModels(ctx context.Context) {
	port, token := p.ActiveUpstream()
	if port == 0 || !liveModels.beginRefresh(false) {
		return
	}
	defer liveModels.endRefresh()
	ctx, cancel := context.WithTimeout(ctx, liveModelsTimeout)
	defer cancel()
	if err := p.refreshLiveModels(ctx, port, token); err != nil {
		slog.Warn("[Models] refresh failed, keeping previous list", "err", err)
	}
}

// warmLiveModels refreshes in the background; used on hot paths that must not block.
func (p *Proxy) warmLiveModels() {
	if !liveModels.needsRefresh() {
		return
	}
	go p.ensureLiveModels(context.Background())
}

// refreshLiveModels does the actual fetch + table build.
func (p *Proxy) refreshLiveModels(ctx context.Context, port int, token string) error {
	list, err := p.fetchUpstreamModels(ctx, port, token)
	if err != nil {
		return err
	}
	options, byID, byEnum := buildModelTables(list.Models, list.CuratedIDs)
	if len(byID) == 0 {
		return fmt.Errorf("GetAvailableModels returned no named models")
	}
	liveModels.set(options, byID, byEnum, list.DefaultAgentID)
	return nil
}

// HandleModels handles GET/POST /gateway/models.
func (p *Proxy) HandleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if port, token := p.ActiveUpstream(); port != 0 && liveModels.beginRefresh(r.URL.Query().Get("refresh") == "1") {
		ctx, cancel := context.WithTimeout(r.Context(), liveModelsTimeout)
		if err := p.refreshLiveModels(ctx, port, token); err != nil {
			slog.Warn("[Models] refresh failed, serving cached/fallback list", "err", err)
		}
		cancel()
		liveModels.endRefresh()
	}
	res, _ := liveModels.snapshot()
	data, err := json.Marshal(res)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
