package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/i18n"
)

// 斜杠命令。
//
// language_server 的 GetSlashCommands 返回桌面端 "/" 菜单的全部内容：系统命令（plan、goal……）
// 加所有可调用的技能（含用户自己安装的）。发送时把命令作为 items 里的一个 ContextScopeItem 条目：
//
//	{"item": {"slashCommand": {"info": {name, modelFacingText, type, definitionPath, ...}}}}
//
// 实测 language_server 会把它记进会话（显示文本自动生成为 "/name 用户输入"），模型也确实收到了
// modelFacingText。为了让客户端轻量且不能伪造注入文本，客户端只传 {"info":{"name":"plan"}}，
// 网关发送前用这里缓存的权威 info 替换（expandSlashCommands）。

const (
	slashCommandsTTL     = 2 * time.Minute
	slashCommandsTimeout = 6 * time.Second
)

// hiddenSlashCommands are desktop-oriented commands that make no sense on a phone.
var hiddenSlashCommands = map[string]bool{
	"schedule":           true, // 会话内临时定时器，非长期定时任务
	"browser":            true, // 需要桌面端 Chrome
	"generative_ui":      true, // 生成内联 HTML 控件，手机端无法渲染
	"ui-extension":       true,
	"plugin":             true,
	"migrate-workflows":  true,
	"agy-customizations": true,
	"antigravity-guide":  true,
	"conversations":      true, // Antigravity 2.35 内置程序化对话管理 API 技能，非手机交互斜杠命令
}

// SlashCommandOption is one entry of the phone's "/" menu. modelFacingText is deliberately omitted
// (it can be 15 KB); the gateway re-attaches the authoritative info when the message is sent.
type SlashCommandOption struct {
	Name           string `json:"name"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Icon           string `json:"icon,omitempty"`
	Kind           string `json:"kind"` // "system" | "skill"
	DefinitionPath string `json:"definitionPath,omitempty"`
}

// SlashCommandsResponse is the response of /gateway/slash-commands.
type SlashCommandsResponse struct {
	Commands []SlashCommandOption `json:"commands"`
}

type slashCommandCache struct {
	mu               sync.Mutex
	fetchedAt        time.Time
	options          []SlashCommandOption
	infoByName       map[string]json.RawMessage // authoritative SlashCommandInfo per command name (hidden ones included)
	systemInfoByName map[string]json.RawMessage // visible upstream system commands (lowercased key) eligible for plain-text leading /cmd upgrade
}

func (c *slashCommandCache) fresh() bool {
	return c.infoByName != nil && time.Since(c.fetchedAt) < slashCommandsTTL
}

// upstreamSlashCommand mirrors one GetSlashCommands entry.
type upstreamSlashCommand struct {
	Info        json.RawMessage `json:"info"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
}

// buildSlashCommandTables turns the raw upstream commands into the visible menu and the info lookup.
func buildSlashCommandTables(raw []upstreamSlashCommand) ([]SlashCommandOption, map[string]json.RawMessage) {
	options, infoByName, _ := buildSlashCommandTablesWithSystem(raw)
	return options, infoByName
}

func buildSlashCommandTablesWithSystem(raw []upstreamSlashCommand) ([]SlashCommandOption, map[string]json.RawMessage, map[string]json.RawMessage) {
	infoByName := map[string]json.RawMessage{}
	systemInfoByName := map[string]json.RawMessage{}
	var options []SlashCommandOption
	for _, c := range raw {
		var info struct {
			Name           string `json:"name"`
			Type           string `json:"type"`
			Icon           string `json:"icon"`
			DefinitionPath string `json:"definitionPath"`
		}
		if err := json.Unmarshal(c.Info, &info); err != nil || info.Name == "" {
			continue
		}
		if _, dup := infoByName[info.Name]; dup {
			continue
		}
		infoByName[info.Name] = c.Info
		if hiddenSlashCommands[info.Name] {
			continue
		}
		kind := "system"
		if strings.Contains(info.Type, "SKILL") {
			kind = "skill"
		} else {
			systemInfoByName[strings.ToLower(info.Name)] = c.Info
		}
		title := c.Title
		if title == "" {
			title = info.Name
		}
		options = append(options, SlashCommandOption{
			Name:           info.Name,
			Title:          title,
			Description:    c.Description,
			Icon:           info.Icon,
			Kind:           kind,
			DefinitionPath: strings.TrimSpace(info.DefinitionPath),
		})
	}

	// 注入移动端首选动态多智能体工作流 (/workflow) 与旁路快问 (/btw) 快捷入口
	hasWorkflow := false
	hasBtw := false
	for _, opt := range options {
		if opt.Name == "workflow" {
			hasWorkflow = true
		} else if opt.Name == "btw" {
			hasBtw = true
		}
	}
	if !hasWorkflow {
		options = append(options, SlashCommandOption{
			Name:        "workflow",
			Title:       "Workflow",
			Description: "Orchestrate a Python multi-agent workflow for parallel staged tasks",
			Icon:        "point.3.connected.trianglepath.dotted",
			Kind:        "system",
		})
		infoByName["workflow"] = json.RawMessage(`{"name":"workflow","type":"COMMAND_TYPE_SYSTEM","icon":"point.3.connected.trianglepath.dotted"}`)
	}
	if !hasBtw {
		options = append(options, SlashCommandOption{
			Name:        "btw",
			Title:       "Side Question",
			Description: "Ask a quick side question from current context without interrupting the main chat",
			Icon:        "chat_add_on",
			Kind:        "system",
		})
		infoByName["btw"] = json.RawMessage(`{"name":"btw","type":"COMMAND_TYPE_SYSTEM","icon":"chat_add_on"}`)
	}

	// 系统命令在前，技能在后；组内按名称排序，菜单稳定
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].Kind != options[j].Kind {
			return options[i].Kind == "system"
		}
		return options[i].Name < options[j].Name
	})
	return options, infoByName, systemInfoByName
}

// fetchSlashCommands queries GetSlashCommands, which requires a model in the cascade config.
func (p *Proxy) fetchSlashCommands(ctx context.Context, port int, token string) ([]upstreamSlashCommand, error) {
	enum, name := "MODEL_PLACEHOLDER_M318", "gemini-3.8-flash-high"
	if snap, live := liveModels.snapshot(); live {
		if id := snap.Defaults["gemini"]; id != "" {
			if e := liveModels.enumForID(id); e != "" {
				enum, name = e, id
			}
		}
	}
	cfg := applyModelToCascadeConfig(nil, enum, name)
	body, _ := json.Marshal(map[string]interface{}{"cascadeConfig": cfg})

	apiURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetSlashCommands", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(string(body)))
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
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GetSlashCommands returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var parsed struct {
		Commands []upstreamSlashCommand `json:"commands"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	return parsed.Commands, nil
}

// slashCommandsSnapshot returns the cached menu and info table, refreshing when stale (or when force).
func (p *Proxy) slashCommandsSnapshot(ctx context.Context, force bool) ([]SlashCommandOption, map[string]json.RawMessage, error) {
	options, infoByName, _, err := p.slashCommandsSnapshotFull(ctx, force)
	return options, infoByName, err
}

func (p *Proxy) slashCommandsSnapshotFull(ctx context.Context, force bool) ([]SlashCommandOption, map[string]json.RawMessage, map[string]json.RawMessage, error) {
	p.slashCache.mu.Lock()
	defer p.slashCache.mu.Unlock()
	if !force && p.slashCache.fresh() {
		return p.slashCache.options, p.slashCache.infoByName, p.slashCache.systemInfoByName, nil
	}
	port, token := p.ActiveUpstream()
	if port == 0 {
		if p.slashCache.infoByName != nil {
			return p.slashCache.options, p.slashCache.infoByName, p.slashCache.systemInfoByName, nil // 上游暂时不可用：继续用旧缓存
		}
		return nil, nil, nil, fmt.Errorf("no active Antigravity upstream")
	}
	ctx, cancel := context.WithTimeout(ctx, slashCommandsTimeout)
	defer cancel()
	raw, err := p.fetchSlashCommands(ctx, port, token)
	if err != nil {
		if p.slashCache.infoByName != nil {
			slog.Warn("[Slash] refresh failed, serving cached commands", "err", err)
			return p.slashCache.options, p.slashCache.infoByName, p.slashCache.systemInfoByName, nil
		}
		return nil, nil, nil, err
	}
	p.slashCache.options, p.slashCache.infoByName, p.slashCache.systemInfoByName = buildSlashCommandTablesWithSystem(raw)
	p.slashCache.fetchedAt = time.Now()
	return p.slashCache.options, p.slashCache.infoByName, p.slashCache.systemInfoByName, nil
}

func localizeSlashCommandsForLang(options []SlashCommandOption, lang string) []SlashCommandOption {
	if len(options) == 0 {
		return []SlashCommandOption{}
	}
	l := i18n.ResolveLang(lang)
	out := make([]SlashCommandOption, len(options))
	copy(out, options)
	for i := range out {
		switch out[i].Name {
		case "workflow":
			if out[i].Kind == "system" && out[i].DefinitionPath == "" {
				out[i].Title = i18n.T3For(l, "Workflow", "Workflow", "Workflow")
				out[i].Description = i18n.T3For(l,
					"编排 Python 多智能体工作流，并行执行多阶段任务",
					"Orchestrate a Python multi-agent workflow for parallel staged tasks",
					"Python マルチエージェントワークフローで複数タスクを並列実行",
				)
			}
		case "btw":
			if out[i].Kind == "system" && out[i].DefinitionPath == "" {
				out[i].Title = i18n.T3For(l, "旁路快问", "Side Question", "サイド質問")
				out[i].Description = i18n.T3For(l,
					"基于当前上下文分支快问，不打断主对话",
					"Ask a quick side question from current context without interrupting the main chat",
					"現在のコンテキストからメイン会話を止めずに分岐質問",
				)
			}
		}
	}
	return out
}

// HandleSlashCommands handles GET/POST /gateway/slash-commands.
func (p *Proxy) HandleSlashCommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uiLang := resolveRequestUILang(r)
	options, _, err := p.slashCommandsSnapshot(r.Context(), r.URL.Query().Get("refresh") == "1")
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "no active") {
			status = http.StatusServiceUnavailable
		}
		writeJSONError(w, err.Error(), status)
		return
	}
	localized := localizeSlashCommandsForLang(options, uiLang)
	data, _ := json.Marshal(SlashCommandsResponse{Commands: localized})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// slashCommandName extracts the command name from an items[] element, if it is a slash-command item.
func slashCommandName(itemMap map[string]interface{}) (name string, slash map[string]interface{}, ok bool) {
	inner, _ := itemMap["item"].(map[string]interface{})
	slash, _ = inner["slashCommand"].(map[string]interface{})
	if slash == nil {
		return "", nil, false
	}
	info, _ := slash["info"].(map[string]interface{})
	name, _ = info["name"].(string)
	return strings.TrimSpace(name), slash, true
}

// extractLeadingSlashCommand parses a leading "/<cmd>" token from plain user text.
// Only strict ASCII command tokens ([a-zA-Z0-9_-]+) followed by whitespace or end-of-string
// match, so Unix file paths (e.g. "/Users/foo") and mid-sentence mentions never match.
func extractLeadingSlashCommand(text string) (name string, rest string, ok bool) {
	trimmedLeft := strings.TrimLeft(text, " \t")
	if !strings.HasPrefix(trimmedLeft, "/") {
		return "", "", false
	}
	afterSlash := trimmedLeft[1:]
	if afterSlash == "" {
		return "", "", false
	}
	end := strings.IndexAny(afterSlash, " \t\n\r")
	var cand string
	if end == -1 {
		cand = afterSlash
		rest = ""
	} else {
		cand = afterSlash[:end]
		rest = afterSlash[end:]
	}
	if cand == "" {
		return "", "", false
	}
	for i := 0; i < len(cand); i++ {
		c := cand[i]
		isAlpha := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		isDigit := c >= '0' && c <= '9'
		if !isAlpha && !isDigit && c != '-' && c != '_' {
			return "", "", false
		}
	}
	return strings.ToLower(cand), rest, true
}

// expandSlashCommands replaces every slash-command item's info with the authoritative one.
// When no structured slash-command item is present, it also upgrades plain-text messages
// starting with a known upstream system command (e.g. "/compact", "/plan ...") so older
// clients and manually typed system commands trigger native upstream behavior.
func (p *Proxy) expandSlashCommands(ctx context.Context, rawMap map[string]interface{}) (bool, error) {
	items, _ := rawMap["items"].([]interface{})
	hasSlash := false
	for _, it := range items {
		if m, ok := it.(map[string]interface{}); ok {
			if _, _, isSlash := slashCommandName(m); isSlash {
				hasSlash = true
				break
			}
		}
	}
	if !hasSlash {
		return p.upgradeLeadingPlainTextSystemSlash(ctx, rawMap, items)
	}

	_, infoByName, err := p.slashCommandsSnapshot(ctx, false)
	if err != nil {
		return false, fmt.Errorf("%s: %w", i18n.T3("斜杠命令列表不可用", "slash commands unavailable", "スラッシュコマンド一覧を利用できません"), err)
	}
	refreshed := false
	for _, it := range items {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		name, slash, isSlash := slashCommandName(m)
		if !isSlash {
			continue
		}
		if name == "" {
			return false, fmt.Errorf("invalid_argument: slash command name is required")
		}
		info, known := infoByName[name]
		if !known && !refreshed {
			// 刚装的技能：强制刷新一次再判断
			refreshed = true
			if _, fresh, rerr := p.slashCommandsSnapshot(ctx, true); rerr == nil {
				infoByName = fresh
				info, known = infoByName[name]
			}
		}
		if !known {
			return false, fmt.Errorf("invalid_argument: unknown slash command: %s", name)
		}
		var decoded interface{}
		if err := json.Unmarshal(info, &decoded); err != nil {
			return false, err
		}
		slash["info"] = decoded
	}
	return true, nil
}

// upgradeLeadingPlainTextSystemSlash upgrades a leading plain-text "/<system-cmd>"
// (such as "/compact" from older clients or manually typed "/plan ...") into a structured
// slashCommand item. Unknown commands, skills, paths, and upstream errors silently fall
// back to leaving the original plain text untouched.
func (p *Proxy) upgradeLeadingPlainTextSystemSlash(ctx context.Context, rawMap map[string]interface{}, items []interface{}) (bool, error) {
	if len(items) == 0 {
		return false, nil
	}
	firstMap, ok := items[0].(map[string]interface{})
	if !ok {
		return false, nil
	}
	firstText, ok := firstMap["text"].(string)
	if !ok {
		return false, nil
	}
	cand, rest, matched := extractLeadingSlashCommand(firstText)
	if !matched {
		return false, nil
	}

	_, _, systemInfoByName, err := p.slashCommandsSnapshotFull(ctx, false)
	if err != nil || len(systemInfoByName) == 0 {
		return false, nil
	}
	info, known := systemInfoByName[cand]
	if !known {
		return false, nil
	}
	var decoded interface{}
	if err := json.Unmarshal(info, &decoded); err != nil {
		return false, nil
	}

	upgraded := make([]interface{}, 0, len(items)+1)
	upgraded = append(upgraded, map[string]interface{}{
		"item": map[string]interface{}{
			"slashCommand": map[string]interface{}{
				"info": decoded,
			},
		},
	})
	if strings.TrimSpace(rest) != "" {
		upgraded = append(upgraded, map[string]interface{}{
			"text": rest,
		})
	}
	if len(items) > 1 {
		upgraded = append(upgraded, items[1:]...)
	}
	rawMap["items"] = upgraded
	slog.Info("[Slash] Upgraded leading plain-text system command", "command", cand)
	return true, nil
}
