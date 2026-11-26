package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
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

func parseMarketplaceCustomizations(raw []byte, installedIDs map[string]bool) ([]MarketplaceItem, string) {
	var doc struct {
		Customizations []struct {
			ID           string `json:"id"`
			DisplayName  string `json:"displayName"`
			Name         string `json:"name"`
			Description  string `json:"description"`
			Author       string `json:"author"`
			Version      string `json:"version"`
			Kind         string `json:"kind"`
			InstallCount int64  `json:"installCount"`
			UserCount    int64  `json:"userCount"`
			Homepage     string `json:"homepage"`
			Repository   string `json:"repository"`
			Logo         string `json:"logo"`
			Marketplace  string `json:"marketplace"`
		} `json:"customizations"`
		Featured []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Description string `json:"description"`
			Author      string `json:"author"`
			Version     string `json:"version"`
			Kind        string `json:"kind"`
			Homepage    string `json:"homepage"`
			Repository  string `json:"repository"`
			Logo        string `json:"logo"`
		} `json:"featured"`
		NextPageToken string `json:"nextPageToken"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil, ""
	}
	out := make([]MarketplaceItem, 0, len(doc.Customizations)+len(doc.Featured))
	seen := make(map[string]bool)
	for _, c := range doc.Customizations {
		id := strings.TrimSpace(c.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		name := firstNonEmpty(strings.TrimSpace(c.DisplayName), strings.TrimSpace(c.Name), id)
		kind := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(c.Kind), "CUSTOMIZATION_KIND_"))
		if kind == "" {
			kind = "plugin"
		}
		out = append(out, MarketplaceItem{
			ID:           id,
			Name:         name,
			Description:  strings.TrimSpace(c.Description),
			Author:       strings.TrimSpace(c.Author),
			Version:      strings.TrimSpace(c.Version),
			Kind:         kind,
			InstallCount: c.InstallCount,
			UserCount:    c.UserCount,
			Homepage:     strings.TrimSpace(c.Homepage),
			Repository:   strings.TrimSpace(c.Repository),
			Logo:         strings.TrimSpace(c.Logo),
			Marketplace:  strings.TrimSpace(c.Marketplace),
			Installed:    installedIDs[id] || installedIDs[name],
		})
	}
	for _, f := range doc.Featured {
		id := strings.TrimSpace(f.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		name := firstNonEmpty(strings.TrimSpace(f.DisplayName), id)
		kind := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(f.Kind), "CUSTOMIZATION_KIND_"))
		if kind == "" {
			kind = "plugin"
		}
		out = append(out, MarketplaceItem{
			ID:          id,
			Name:        name,
			Description: strings.TrimSpace(f.Description),
			Author:      strings.TrimSpace(f.Author),
			Version:     strings.TrimSpace(f.Version),
			Kind:        kind,
			Homepage:    strings.TrimSpace(f.Homepage),
			Repository:  strings.TrimSpace(f.Repository),
			Logo:        strings.TrimSpace(f.Logo),
			Installed:   installedIDs[id] || installedIDs[name],
		})
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
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"plugins"`
	}
	if json.Unmarshal(body, &raw) == nil {
		for _, pl := range raw.Plugins {
			if s := strings.TrimSpace(pl.ID); s != "" {
				res[s] = true
			}
			if s := strings.TrimSpace(pl.Name); s != "" {
				res[s] = true
			}
			if s := strings.TrimSpace(pl.DisplayName); s != "" {
				res[s] = true
			}
		}
	}
	return res
}

// HandleMarketplace serves /gateway/marketplace/{search,featured,bundles}.
func (p *Proxy) HandleMarketplace(w http.ResponseWriter, r *http.Request) {
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
			writeRPCError(w, status, extractConnectErrorMessage(body, "查询插件市场失败"))
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
			writeRPCError(w, status, extractConnectErrorMessage(body, "获取插件合集失败"))
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
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	switch r.URL.Path {
	case "/gateway/plugins/delete":
		var req struct {
			PluginID string `json:"pluginId"`
			ID       string `json:"id"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, "无效的请求体")
			return
		}
		pid := firstNonEmpty(strings.TrimSpace(req.PluginID), strings.TrimSpace(req.ID))
		if pid == "" {
			writeRPCError(w, http.StatusBadRequest, "缺少 pluginId")
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
			writeRPCError(w, status, extractConnectErrorMessage(body, "卸载插件失败"))
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
			"message":  firstNonEmpty(up.Message, "插件已卸载"),
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
			writeRPCError(w, http.StatusBadRequest, "无效的请求体")
			return
		}
		if strings.TrimSpace(req.Plugin) == "" && strings.TrimSpace(req.Provider) == "" {
			writeRPCError(w, http.StatusBadRequest, "缺少 plugin 或 provider")
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
			writeRPCError(w, status, extractConnectErrorMessage(body, "启动插件登录失败"))
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
			writeRPCError(w, http.StatusBadRequest, "无效的请求体")
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
			writeRPCError(w, status, extractConnectErrorMessage(body, "退出插件账号失败"))
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
			writeRPCError(w, http.StatusBadRequest, "无效的请求体")
			return
		}
		if strings.TrimSpace(req.AccountID) == "" {
			writeRPCError(w, http.StatusBadRequest, "缺少 accountId")
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
			writeRPCError(w, status, extractConnectErrorMessage(body, "切换默认账号失败"))
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
			writeRPCError(w, http.StatusBadRequest, "无效的请求体")
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
			writeRPCError(w, status, extractConnectErrorMessage(body, "修改账号备注失败"))
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
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	switch r.URL.Path {
	case "/gateway/mcp/oauth/start":
		var req struct {
			ServerName string `json:"serverName"`
			Mode       string `json:"mode"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, "无效的请求体")
			return
		}
		srv := strings.TrimSpace(req.ServerName)
		if srv == "" {
			writeRPCError(w, http.StatusBadRequest, "缺少 serverName")
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
			writeRPCError(w, status, extractConnectErrorMessage(body, "启动 MCP OAuth 授权失败"))
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
			writeRPCError(w, http.StatusBadRequest, "无效的请求体")
			return
		}
		if strings.TrimSpace(req.ServerName) == "" || strings.TrimSpace(req.AuthorizationCode) == "" {
			writeRPCError(w, http.StatusBadRequest, "缺少 serverName 或 authorizationCode")
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
			writeRPCError(w, status, extractConnectErrorMessage(body, "提交 MCP OAuth 授权码失败"))
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})

	case "/gateway/mcp/oauth/disconnect":
		var req struct {
			ServerName string `json:"serverName"`
			Account    string `json:"account"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, "无效的请求体")
			return
		}
		if strings.TrimSpace(req.ServerName) == "" {
			writeRPCError(w, http.StatusBadRequest, "缺少 serverName")
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
			writeRPCError(w, status, extractConnectErrorMessage(body, "断开 MCP OAuth 失败"))
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
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" || len(trimmed) > 8192 {
		return nil, fmt.Errorf("回调链接为空或过长")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("无法解析回调链接: %v", err)
	}
	if u.Scheme != "http" {
		return nil, fmt.Errorf("仅允许转发 http://127.0.0.1 或 http://localhost 本机回调链接")
	}
	if u.User != nil {
		return nil, fmt.Errorf("不允许包含凭据信息")
	}
	host := strings.ToLower(u.Hostname())
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return nil, fmt.Errorf("出于安全限制，仅允许转发指向本机回环地址 (127.0.0.1 / localhost) 的 OAuth 回调")
	}
	portStr := u.Port()
	if portStr == "" {
		return nil, fmt.Errorf("回调链接必须包含明确端口")
	}
	portNum, err := strconv.Atoi(portStr)
	if err != nil || portNum < 1024 || portNum > 65535 {
		return nil, fmt.Errorf("回调端口超出允许范围 (1024-65535)")
	}
	// Block forwarding to Antigravity language_server port or the gateway's own listening port.
	if lsPort, _ := p.ActiveUpstream(); lsPort > 0 && portNum == lsPort {
		return nil, fmt.Errorf("禁止向受保护内部服务端口转发")
	}
	if _, reqPortStr, splitErr := net.SplitHostPort(reqHost); splitErr == nil {
		if gwPort, perr := strconv.Atoi(reqPortStr); perr == nil && gwPort > 0 && portNum == gwPort {
			return nil, fmt.Errorf("禁止向网关自身端口转发")
		}
	}
	if strings.Contains(u.Path, "..") {
		return nil, fmt.Errorf("无效的回调路径")
	}
	// Normalize host to 127.0.0.1 so DNS cannot be abused.
	if host == "localhost" {
		u.Host = net.JoinHostPort("127.0.0.1", portStr)
	}
	return u, nil
}

func (p *Proxy) handleLoopbackOAuthCallbackForward(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CallbackURL string `json:"callbackUrl"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	target, err := p.validateLoopbackCallbackURL(req.CallbackURL, r.Host)
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
		writeRPCError(w, http.StatusBadGateway, fmt.Sprintf("转发至本机 OAuth 监听端口失败: %v", err))
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32*1024))
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"ok":         resp.StatusCode >= 200 && resp.StatusCode < 400,
		"statusCode": resp.StatusCode,
		"message":    "已将 OAuth 回调转发至电脑端监听端口，请刷新插件/MCP 登录状态",
	})
}
