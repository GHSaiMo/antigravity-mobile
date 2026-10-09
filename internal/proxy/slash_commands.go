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
	"schedule":           true, // 定时任务：手机端不做
	"automation":         true,
	"browser":            true, // 需要桌面端 Chrome
	"generative_ui":      true, // 生成内联 HTML 控件，手机端无法渲染
	"ui-extension":       true,
	"plugin":             true,
	"migrate-workflows":  true,
	"agy-customizations": true,
	"antigravity-guide":  true,
}

// SlashCommandOption is one entry of the phone's "/" menu. modelFacingText is deliberately omitted
// (it can be 15 KB); the gateway re-attaches the authoritative info when the message is sent.
type SlashCommandOption struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Icon        string `json:"icon,omitempty"`
	Kind        string `json:"kind"` // "system" | "skill"
}

// SlashCommandsResponse is the response of /gateway/slash-commands.
type SlashCommandsResponse struct {
	Commands []SlashCommandOption `json:"commands"`
}

type slashCommandCache struct {
	mu         sync.Mutex
	fetchedAt  time.Time
	options    []SlashCommandOption
	infoByName map[string]json.RawMessage // authoritative SlashCommandInfo per command name (hidden ones included)
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
	infoByName := map[string]json.RawMessage{}
	var options []SlashCommandOption
	for _, c := range raw {
		var info struct {
			Name string `json:"name"`
			Type string `json:"type"`
			Icon string `json:"icon"`
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
		}
		title := c.Title
		if title == "" {
			title = info.Name
		}
		options = append(options, SlashCommandOption{Name: info.Name, Title: title, Description: c.Description, Icon: info.Icon, Kind: kind})
	}
	// 系统命令在前，技能在后；组内按名称排序，菜单稳定
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].Kind != options[j].Kind {
			return options[i].Kind == "system"
		}
		return options[i].Name < options[j].Name
	})
	return options, infoByName
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
	p.slashCache.mu.Lock()
	defer p.slashCache.mu.Unlock()
	if !force && p.slashCache.fresh() {
		return p.slashCache.options, p.slashCache.infoByName, nil
	}
	port, token := p.ActiveUpstream()
	if port == 0 {
		if p.slashCache.infoByName != nil {
			return p.slashCache.options, p.slashCache.infoByName, nil // 上游暂时不可用：继续用旧缓存
		}
		return nil, nil, fmt.Errorf("no active Antigravity upstream")
	}
	ctx, cancel := context.WithTimeout(ctx, slashCommandsTimeout)
	defer cancel()
	raw, err := p.fetchSlashCommands(ctx, port, token)
	if err != nil {
		if p.slashCache.infoByName != nil {
			slog.Warn("[Slash] refresh failed, serving cached commands", "err", err)
			return p.slashCache.options, p.slashCache.infoByName, nil
		}
		return nil, nil, err
	}
	p.slashCache.options, p.slashCache.infoByName = buildSlashCommandTables(raw)
	p.slashCache.fetchedAt = time.Now()
	return p.slashCache.options, p.slashCache.infoByName, nil
}

// HandleSlashCommands handles GET/POST /gateway/slash-commands.
func (p *Proxy) HandleSlashCommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	options, _, err := p.slashCommandsSnapshot(r.Context(), r.URL.Query().Get("refresh") == "1")
	if err != nil {
		status := http.StatusBadGateway
		if strings.Contains(err.Error(), "no active") {
			status = http.StatusServiceUnavailable
		}
		writeJSONError(w, err.Error(), status)
		return
	}
	if options == nil {
		options = []SlashCommandOption{}
	}
	data, _ := json.Marshal(SlashCommandsResponse{Commands: options})
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

// expandSlashCommands replaces every slash-command item's info with the authoritative one.
// It reports whether anything was changed and rejects unknown command names.
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
		return false, nil
	}

	_, infoByName, err := p.slashCommandsSnapshot(ctx, false)
	if err != nil {
		return false, fmt.Errorf("斜杠命令列表不可用: %w", err)
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
