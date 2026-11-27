package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/i18n"
)

// Phase 4: Marketplace search/featured, plugin uninstall, plugin auth account lifecycle,
// MCP OAuth sign-in/complete/disconnect, and safe loopback OAuth callback forwarding.

// MarketplaceItem is a normalized Marketplace catalog entry for mobile clients.
type MarketplaceItem struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Author       string `json:"author,omitempty"`
	Version      string `json:"version,omitempty"`
	Kind         string `json:"kind,omitempty"` // "plugin" | "skill" | "bundle"
	InstallCount int64  `json:"installCount,omitempty"`
	UserCount    int64  `json:"userCount,omitempty"`
	Homepage     string `json:"homepage,omitempty"`
	Repository   string `json:"repository,omitempty"`
	Logo         string `json:"logo,omitempty"`
	Marketplace  string `json:"marketplace,omitempty"`
	Installed    bool   `json:"installed"`
}

func isLoopbackOAuthURL(rawAuthURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawAuthURL))
	if err != nil {
		return false
	}
	q := u.Query()
	for _, key := range []string{"redirect_uri", "redirect_url", "callback", "return_to"} {
		val := strings.TrimSpace(q.Get(key))
		if val == "" {
			continue
		}
		ru, rerr := url.Parse(val)
		if rerr != nil {
			continue
		}
		h := strings.ToLower(ru.Hostname())
		if h == "127.0.0.1" || h == "localhost" || h == "::1" {
			return true
		}
	}
	return false
}

func hasNonNullJSON(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s != "" && s != "null"
}

func extractMarketplaceAuthor(raw json.RawMessage) string {
	if !hasNonNullJSON(raw) {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var obj struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		URL         string `json:"url"`
	}
	if json.Unmarshal(raw, &obj) == nil {
		return firstNonEmpty(strings.TrimSpace(obj.Name), strings.TrimSpace(obj.DisplayName), strings.TrimSpace(obj.URL))
	}
	return ""
}

func parseFlexibleInt64(raw json.RawMessage) int64 {
	if !hasNonNullJSON(raw) {
		return 0
	}
	var n int64
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
			return parsed
		}
	}
	return 0
}

func detectMarketplaceKind(id, rawKind string, pluginRaw, skillRaw, bundleRaw json.RawMessage) string {
	if k := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(rawKind), "CUSTOMIZATION_KIND_")); k != "" {
		return k
	}
	switch {
	case hasNonNullJSON(skillRaw):
		return "skill"
	case hasNonNullJSON(bundleRaw):
		return "bundle"
	case hasNonNullJSON(pluginRaw):
		return "plugin"
	case strings.Contains(id, "/skill/"):
		return "skill"
	case strings.Contains(id, "/bundle/"):
		return "bundle"
	default:
		return "plugin"
	}
}

func parseMarketplaceCustomizations(raw []byte, installedIDs map[string]bool) ([]MarketplaceItem, string) {
	var doc struct {
		Customizations []struct {
			ID           string          `json:"id"`
			DisplayName  string          `json:"displayName"`
			Name         string          `json:"name"`
			Description  string          `json:"description"`
			Author       json.RawMessage `json:"author"`
			Version      string          `json:"version"`
			Kind         string          `json:"kind"`
			Plugin       json.RawMessage `json:"plugin"`
			Skill        json.RawMessage `json:"skill"`
			Bundle       json.RawMessage `json:"bundle"`
			InstallCount json.RawMessage `json:"installCount"`
			UserCount    json.RawMessage `json:"userCount"`
			Homepage     string          `json:"homepage"`
			Repository   string          `json:"repository"`
			Logo         string          `json:"logo"`
			Marketplace  string          `json:"marketplace"`
		} `json:"customizations"`
		Featured []struct {
			ID          string          `json:"id"`
			MemberIDs   []string        `json:"memberIds"`
			DisplayName string          `json:"displayName"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Author      json.RawMessage `json:"author"`
			Version     string          `json:"version"`
			Kind        string          `json:"kind"`
			Plugin      json.RawMessage `json:"plugin"`
			Skill       json.RawMessage `json:"skill"`
			Bundle      json.RawMessage `json:"bundle"`
			Homepage    string          `json:"homepage"`
			Repository  string          `json:"repository"`
			Logo        string          `json:"logo"`
			Marketplace string          `json:"marketplace"`
		} `json:"featured"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		slog.Warn("[Marketplace] failed to decode upstream response", "err", err)
		return nil, ""
	}

	byID := make(map[string]MarketplaceItem, len(doc.Customizations))
	customOrder := make([]string, 0, len(doc.Customizations))
	for _, c := range doc.Customizations {
		id := strings.TrimSpace(c.ID)
		if id == "" {
			continue
		}
		if _, exists := byID[id]; exists {
			continue
		}
		name := firstNonEmpty(strings.TrimSpace(c.DisplayName), strings.TrimSpace(c.Name), id)
		kind := detectMarketplaceKind(id, c.Kind, c.Plugin, c.Skill, c.Bundle)
		byID[id] = MarketplaceItem{
			ID:           id,
			Name:         name,
			Description:  strings.TrimSpace(c.Description),
			Author:       extractMarketplaceAuthor(c.Author),
			Version:      strings.TrimSpace(c.Version),
			Kind:         kind,
			InstallCount: parseFlexibleInt64(c.InstallCount),
			UserCount:    parseFlexibleInt64(c.UserCount),
			Homepage:     strings.TrimSpace(c.Homepage),
			Repository:   strings.TrimSpace(c.Repository),
			Logo:         strings.TrimSpace(c.Logo),
			Marketplace:  strings.TrimSpace(c.Marketplace),
			Installed:    installedIDs[id] || installedIDs[name],
		}
		customOrder = append(customOrder, id)
	}

	// Upstream GetMarketplaceFeatured encodes `featured` as shelf descriptors
	// (`{"id":"featured","memberIds":[...]}`) alongside full entries in `customizations`.
	// Respect memberIds ordering without turning shelf headers into bogus plugin items,
	// while retaining fallback support for legacy flat featured item payloads.
	featuredOrder := make([]string, 0, len(doc.Customizations))
	legacyFeatured := make([]MarketplaceItem, 0)
	for _, f := range doc.Featured {
		if len(f.MemberIDs) > 0 {
			for _, mid := range f.MemberIDs {
				if mid = strings.TrimSpace(mid); mid != "" {
					featuredOrder = append(featuredOrder, mid)
				}
			}
			continue
		}
		id := strings.TrimSpace(f.ID)
		disp := firstNonEmpty(strings.TrimSpace(f.DisplayName), strings.TrimSpace(f.Name))
		desc := strings.TrimSpace(f.Description)
		if id == "" || (disp == "" && desc == "") {
			continue
		}
		name := firstNonEmpty(disp, id)
		kind := detectMarketplaceKind(id, f.Kind, f.Plugin, f.Skill, f.Bundle)
		legacyFeatured = append(legacyFeatured, MarketplaceItem{
			ID:          id,
			Name:        name,
			Description: desc,
			Author:      extractMarketplaceAuthor(f.Author),
			Version:     strings.TrimSpace(f.Version),
			Kind:        kind,
			Homepage:    strings.TrimSpace(f.Homepage),
			Repository:  strings.TrimSpace(f.Repository),
			Logo:        strings.TrimSpace(f.Logo),
			Marketplace: strings.TrimSpace(f.Marketplace),
			Installed:   installedIDs[id] || installedIDs[name],
		})
	}

	out := make([]MarketplaceItem, 0, len(byID)+len(legacyFeatured))
	seen := make(map[string]bool, len(byID)+len(legacyFeatured))
	for _, id := range featuredOrder {
		if seen[id] {
			continue
		}
		if item, ok := byID[id]; ok {
			seen[id] = true
			out = append(out, item)
		}
	}
	for _, id := range customOrder {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, byID[id])
	}
	for _, item := range legacyFeatured {
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		out = append(out, item)
	}
	return out, strings.TrimSpace(doc.NextPageToken)
}

func (p *Proxy) fetchInstalledPluginIDSet(ctx context.Context) map[string]bool {
	res := make(map[string]bool)
	body, status, err := p.callUpstreamRPC(ctx, "GetAllPlugins", map[string]interface{}{})
	if err != nil || status != http.StatusOK {
		return res
	}
	var raw struct {
		Plugins []struct {
			ID          string `json:"id"`
			CatalogID   string `json:"catalogId"`
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"plugins"`
	}
	if json.Unmarshal(body, &raw) == nil {
		for _, pl := range raw.Plugins {
			for _, candidate := range []string{pl.ID, pl.CatalogID, pl.Name, pl.DisplayName} {
				if s := strings.TrimSpace(candidate); s != "" {
					res[s] = true
				}
			}
		}
	}
	return res
}

// HandleMarketplace serves /gateway/marketplace/{search,featured,bundles}.
func (p *Proxy) HandleMarketplace(w http.ResponseWriter, r *http.Request) {
	uiLang := resolveRequestUILang(r)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	switch r.URL.Path {
	case "/gateway/marketplace/search", "/gateway/marketplace/featured":
		var query, groupID, sortBy, pageToken string
		if r.Method == http.MethodPost && r.Body != nil {
			var req struct {
				Query     string `json:"query"`
				GroupID   string `json:"groupId"`
				Sort      string `json:"sort"`
				PageToken string `json:"pageToken"`
			}
			_ = json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req)
			query = strings.TrimSpace(req.Query)
			groupID = strings.TrimSpace(req.GroupID)
			sortBy = strings.TrimSpace(req.Sort)
			pageToken = strings.TrimSpace(req.PageToken)
		} else {
			q := r.URL.Query()
			query = strings.TrimSpace(firstNonEmpty(q.Get("q"), q.Get("query")))
			groupID = strings.TrimSpace(q.Get("groupId"))
			sortBy = strings.TrimSpace(q.Get("sort"))
			pageToken = strings.TrimSpace(q.Get("pageToken"))
		}

		var (
			installed map[string]bool
			body      []byte
			status    int
			rpcErr    error
			wg        sync.WaitGroup
		)
		wg.Add(2)
		go func() {
			defer wg.Done()
			installed = p.fetchInstalledPluginIDSet(ctx)
		}()
		go func() {
			defer wg.Done()
			if r.URL.Path == "/gateway/marketplace/featured" || (query == "" && groupID == "" && pageToken == "") {
				body, status, rpcErr = p.callUpstreamRPC(ctx, "GetMarketplaceFeatured", map[string]interface{}{})
				if rpcErr == nil && status == http.StatusOK {
					return
				}
			}
			payload := map[string]interface{}{}
			if query != "" {
				payload["query"] = query
			}
			if groupID != "" {
				payload["groupId"] = groupID
			}
			if sortBy != "" {
				payload["sort"] = sortBy
			}
			if pageToken != "" {
				payload["pageToken"] = pageToken
			}
			body, status, rpcErr = p.callUpstreamRPC(ctx, "SearchMarketplace", payload)
		}()
		wg.Wait()

		if rpcErr != nil {
			writeRPCError(w, status, rpcErr.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "查询插件市场失败", "Failed to search marketplace", "マーケットプレイスの検索に失敗しました")))
			return
		}
		items, nextToken := parseMarketplaceCustomizations(body, installed)
		if items == nil {
			items = []MarketplaceItem{}
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":            true,
			"items":         items,
			"nextPageToken": nextToken,
		})

	case "/gateway/marketplace/bundles":
		var ids []string
		if r.Method == http.MethodPost && r.Body != nil {
			var req struct {
				IDs []string `json:"ids"`
			}
			_ = json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req)
			ids = req.IDs
		}
		body, status, err := p.callUpstreamRPC(ctx, "GetMarketplaceBundles", map[string]interface{}{"ids": ids})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "获取插件合集失败", "Failed to load plugin bundles", "プラグインバンドルの取得に失敗しました")))
			return
		}
		var up map[string]interface{}
		if json.Unmarshal(body, &up) != nil {
			up = map[string]interface{}{}
		}
		up["ok"] = true
		writeJSONStatus(w, http.StatusOK, up)

	default:
		http.NotFound(w, r)
	}
}

// HandlePluginManageExtra handles plugin uninstall and plugin auth account actions.
func (p *Proxy) HandlePluginManageExtra(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uiLang := resolveRequestUILang(r)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	switch r.URL.Path {
	case "/gateway/plugins/delete":
		var req struct {
			PluginID string `json:"pluginId"`
			ID       string `json:"id"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		pid := firstNonEmpty(strings.TrimSpace(req.PluginID), strings.TrimSpace(req.ID))
		if pid == "" {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "缺少 pluginId", "Missing pluginId", "pluginId が指定されていません"))
			return
		}
		body, status, err := p.callUpstreamRPC(ctx, "DeletePlugin", map[string]interface{}{
			"pluginId": pid,
		})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "卸载插件失败", "Failed to uninstall plugin", "プラグインのアンインストールに失敗しました")))
			return
		}
		var up struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &up)
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":       true,
			"pluginId": pid,
			"message":  firstNonEmpty(up.Message, i18n.T3For(uiLang, "插件已卸载", "Plugin uninstalled", "プラグインをアンインストールしました")),
		})

	case "/gateway/plugins/auth/start":
		var req struct {
			Plugin     string `json:"plugin"`
			Provider   string `json:"provider"`
			AccountID  string `json:"accountId"`
			AddAccount bool   `json:"addAccount"`
			Mode       string `json:"mode"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		if strings.TrimSpace(req.Plugin) == "" && strings.TrimSpace(req.Provider) == "" {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "缺少 plugin 或 provider", "Missing plugin or provider", "plugin または provider が指定されていません"))
			return
		}
		payload := map[string]interface{}{
			"plugin":   strings.TrimSpace(req.Plugin),
			"provider": strings.TrimSpace(req.Provider),
		}
		if aid := strings.TrimSpace(req.AccountID); aid != "" {
			payload["accountId"] = aid
		}
		if req.AddAccount {
			payload["addAccount"] = true
		}
		if m := strings.TrimSpace(req.Mode); m != "" {
			payload["mode"] = m
		}
		body, status, err := p.callUpstreamRPC(ctx, "StartPluginAuthSignIn", payload)
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "启动插件登录失败", "Failed to start plugin sign-in", "プラグインのサインイン開始に失敗しました")))
			return
		}
		var up struct {
			AuthURL   string `json:"authUrl"`
			AccountID string `json:"accountId"`
			Mode      string `json:"mode"`
		}
		_ = json.Unmarshal(body, &up)
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":                 true,
			"authUrl":            strings.TrimSpace(up.AuthURL),
			"accountId":          strings.TrimSpace(up.AccountID),
			"mode":               strings.TrimSpace(up.Mode),
			"isLoopbackCallback": isLoopbackOAuthURL(up.AuthURL),
		})

	case "/gateway/plugins/auth/signout":
		var req struct {
			Plugin    string `json:"plugin"`
			Provider  string `json:"provider"`
			AccountID string `json:"accountId"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		payload := map[string]interface{}{
			"plugin":   strings.TrimSpace(req.Plugin),
			"provider": strings.TrimSpace(req.Provider),
		}
		if aid := strings.TrimSpace(req.AccountID); aid != "" {
			payload["accountId"] = aid
		}
		body, status, err := p.callUpstreamRPC(ctx, "SignOutPluginAuthProvider", payload)
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "退出插件账号失败", "Failed to sign out plugin account", "プラグインアカウントのサインアウトに失敗しました")))
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})

	case "/gateway/plugins/auth/default":
		var req struct {
			Plugin    string `json:"plugin"`
			Provider  string `json:"provider"`
			AccountID string `json:"accountId"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		if strings.TrimSpace(req.AccountID) == "" {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "缺少 accountId", "Missing accountId", "accountId が指定されていません"))
			return
		}
		body, status, err := p.callUpstreamRPC(ctx, "SetDefaultPluginAuthAccount", map[string]interface{}{
			"plugin":    strings.TrimSpace(req.Plugin),
			"provider":  strings.TrimSpace(req.Provider),
			"accountId": strings.TrimSpace(req.AccountID),
		})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "切换默认账号失败", "Failed to set default account", "デフォルトアカウントの設定に失敗しました")))
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})

	case "/gateway/plugins/auth/label":
		var req struct {
			Plugin    string `json:"plugin"`
			Provider  string `json:"provider"`
			AccountID string `json:"accountId"`
			Label     string `json:"label"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		body, status, err := p.callUpstreamRPC(ctx, "SetPluginAuthAccountLabel", map[string]interface{}{
			"plugin":    strings.TrimSpace(req.Plugin),
			"provider":  strings.TrimSpace(req.Provider),
			"accountId": strings.TrimSpace(req.AccountID),
			"label":     strings.TrimSpace(req.Label),
		})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "修改账号备注失败", "Failed to update account label", "アカウントラベルの更新に失敗しました")))
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})

	default:
		http.NotFound(w, r)
	}
}

// HandleMcpOAuth handles /gateway/mcp/oauth/{start,complete,disconnect} and /gateway/oauth/callback-forward.
func (p *Proxy) HandleMcpOAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uiLang := resolveRequestUILang(r)
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	switch r.URL.Path {
	case "/gateway/mcp/oauth/start":
		var req struct {
			ServerName string `json:"serverName"`
			Mode       string `json:"mode"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		srv := strings.TrimSpace(req.ServerName)
		if srv == "" {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "缺少 serverName", "Missing serverName", "serverName が指定されていません"))
			return
		}
		payload := map[string]interface{}{"serverName": srv}
		if m := strings.TrimSpace(req.Mode); m != "" {
			payload["mode"] = m
		}
		body, status, err := p.callUpstreamRPC(ctx, "StartMcpOAuthSignIn", payload)
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "启动 MCP OAuth 授权失败", "Failed to start MCP OAuth sign-in", "MCP OAuth 認証の開始に失敗しました")))
			return
		}
		var up struct {
			AuthURL string `json:"authUrl"`
			Mode    string `json:"mode"`
		}
		_ = json.Unmarshal(body, &up)
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":                 true,
			"authUrl":            strings.TrimSpace(up.AuthURL),
			"mode":               strings.TrimSpace(up.Mode),
			"isLoopbackCallback": isLoopbackOAuthURL(up.AuthURL),
		})

	case "/gateway/mcp/oauth/complete":
		var req struct {
			ServerName        string `json:"serverName"`
			AuthorizationCode string `json:"authorizationCode"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		if strings.TrimSpace(req.ServerName) == "" || strings.TrimSpace(req.AuthorizationCode) == "" {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "缺少 serverName 或 authorizationCode", "Missing serverName or authorizationCode", "serverName または authorizationCode が不足しています"))
			return
		}
		body, status, err := p.callUpstreamRPC(ctx, "CompleteMcpOAuth", map[string]interface{}{
			"serverName":        strings.TrimSpace(req.ServerName),
			"authorizationCode": strings.TrimSpace(req.AuthorizationCode),
		})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "提交 MCP OAuth 授权码失败", "Failed to submit MCP OAuth code", "MCP OAuth 認証コードの送信に失敗しました")))
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})

	case "/gateway/mcp/oauth/disconnect":
		var req struct {
			ServerName string `json:"serverName"`
			Account    string `json:"account"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		if strings.TrimSpace(req.ServerName) == "" {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "缺少 serverName", "Missing serverName", "serverName が指定されていません"))
			return
		}
		payload := map[string]interface{}{"serverName": strings.TrimSpace(req.ServerName)}
		if acc := strings.TrimSpace(req.Account); acc != "" {
			payload["account"] = acc
		}
		body, status, err := p.callUpstreamRPC(ctx, "DisconnectMcpOAuth", payload)
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "断开 MCP OAuth 失败", "Failed to disconnect MCP OAuth", "MCP OAuth の切断に失敗しました")))
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})

	case "/gateway/oauth/callback-forward":
		p.handleLoopbackOAuthCallbackForward(w, r)

	default:
		http.NotFound(w, r)
	}
}

// validateLoopbackCallbackURL enforces strict SSRF bounds for forwarding a pasted OAuth callback URL
// from a remote mobile browser to a local OAuth callback listener on the host machine.
func (p *Proxy) validateLoopbackCallbackURL(rawURL string, reqHost string) (*url.URL, error) {
	return p.validateLoopbackCallbackURLForLang(rawURL, reqHost, i18n.GetLang())
}

func (p *Proxy) validateLoopbackCallbackURLForLang(rawURL string, reqHost string, lang string) (*url.URL, error) {
	l := i18n.ResolveLang(lang)
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" || len(trimmed) > 8192 {
		return nil, fmt.Errorf("%s", i18n.T3For(l, "回调链接为空或过长", "Callback URL is empty or too long", "コールバック URL が空か長すぎます"))
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf(i18n.T3For(l, "无法解析回调链接: %v", "Failed to parse callback URL: %v", "コールバック URL を解析できません: %v"), err)
	}
	if u.Scheme != "http" {
		return nil, fmt.Errorf("%s", i18n.T3For(l, "仅允许转发 http://127.0.0.1 或 http://localhost 本机回调链接", "Only http://127.0.0.1 or http://localhost loopback callbacks are allowed", "http://127.0.0.1 または http://localhost のローカルコールバックのみ許可されています"))
	}
	if u.User != nil {
		return nil, fmt.Errorf("%s", i18n.T3For(l, "不允许包含凭据信息", "Credentials are not allowed in callback URL", "コールバック URL に認証情報を含めることはできません"))
	}
	host := strings.ToLower(u.Hostname())
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return nil, fmt.Errorf("%s", i18n.T3For(l, "出于安全限制，仅允许转发指向本机回环地址 (127.0.0.1 / localhost) 的 OAuth 回调", "For security, only OAuth callbacks to loopback addresses (127.0.0.1 / localhost) may be forwarded", "セキュリティ上の制限により、ループバックアドレス (127.0.0.1 / localhost) 宛の OAuth コールバックのみ転送できます"))
	}
	portStr := u.Port()
	if portStr == "" {
		return nil, fmt.Errorf("%s", i18n.T3For(l, "回调链接必须包含明确端口", "Callback URL must specify an explicit port", "コールバック URL にはポート番号が必要です"))
	}
	portNum, err := strconv.Atoi(portStr)
	if err != nil || portNum < 1024 || portNum > 65535 {
		return nil, fmt.Errorf("%s", i18n.T3For(l, "回调端口超出允许范围 (1024-65535)", "Callback port out of allowed range (1024-65535)", "コールバックポートが許可範囲外です (1024-65535)"))
	}
	// Block forwarding to Antigravity language_server port or the gateway's own listening port.
	if lsPort, _ := p.ActiveUpstream(); lsPort > 0 && portNum == lsPort {
		return nil, fmt.Errorf("%s", i18n.T3For(l, "禁止向受保护内部服务端口转发", "Forwarding to protected internal service port is forbidden", "保護された内部サービスポートへの転送は禁止されています"))
	}
	if _, reqPortStr, splitErr := net.SplitHostPort(reqHost); splitErr == nil {
		if gwPort, perr := strconv.Atoi(reqPortStr); perr == nil && gwPort > 0 && portNum == gwPort {
			return nil, fmt.Errorf("%s", i18n.T3For(l, "禁止向网关自身端口转发", "Forwarding to the gateway's own port is forbidden", "ゲートウェイ自身のポートへの転送は禁止されています"))
		}
	}
	if strings.Contains(u.Path, "..") {
		return nil, fmt.Errorf("%s", i18n.T3For(l, "无效的回调路径", "Invalid callback path", "無効なコールバックパスです"))
	}
	// Normalize host to 127.0.0.1 so DNS cannot be abused.
	if host == "localhost" {
		u.Host = net.JoinHostPort("127.0.0.1", portStr)
	}
	return u, nil
}

func (p *Proxy) handleLoopbackOAuthCallbackForward(w http.ResponseWriter, r *http.Request) {
	uiLang := resolveRequestUILang(r)
	var req struct {
		CallbackURL string `json:"callbackUrl"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
		return
	}
	target, err := p.validateLoopbackCallbackURLForLang(req.CallbackURL, r.Host, uiLang)
	if err != nil {
		writeRPCError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	fwdReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		writeRPCError(w, http.StatusBadRequest, err.Error())
		return
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(fwdReq)
	if err != nil {
		writeRPCError(w, http.StatusBadGateway, fmt.Sprintf(i18n.T3For(uiLang, "转发至本机 OAuth 监听端口失败: %v", "Failed to forward to local OAuth listener: %v", "ローカル OAuth リスナーへの転送に失敗しました: %v"), err))
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32*1024))
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"ok":         resp.StatusCode >= 200 && resp.StatusCode < 400,
		"statusCode": resp.StatusCode,
		"message": i18n.T3For(uiLang,
			"已将 OAuth 回调转发至电脑端监听端口，请刷新插件/MCP 登录状态",
			"Forwarded OAuth callback to local listener; please refresh plugin/MCP sign-in status",
			"OAuth コールバックをPCのリスナーへ転送しました。プラグイン/MCP のログイン状態を更新してください",
		),
	})
}
