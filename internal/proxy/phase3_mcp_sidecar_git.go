package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var validBranchNameRe = regexp.MustCompile(`^[A-Za-z0-9._/\-]+$`)

// HandleMcpManage handles toggling and refreshing MCP servers via ToggleMcpServer & RefreshMcpServers.
func (p *Proxy) HandleMcpManage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	switch r.URL.Path {
	case "/gateway/mcp/toggle":
		var req struct {
			ServerName string `json:"serverName"`
			Enabled    bool   `json:"enabled"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, "无效的请求体")
			return
		}
		if strings.TrimSpace(req.ServerName) == "" {
			writeRPCError(w, http.StatusBadRequest, "缺少 serverName")
			return
		}
		body, status, err := p.callUpstreamRPC(ctx, "ToggleMcpServer", map[string]interface{}{
			"serverName": req.ServerName,
			"enabled":    req.Enabled,
		})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, strings.TrimSpace(string(body)))
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true, "serverName": req.ServerName, "enabled": req.Enabled})
	case "/gateway/mcp/refresh":
		body, status, err := p.callUpstreamRPC(ctx, "RefreshMcpServers", map[string]interface{}{})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, strings.TrimSpace(string(body)))
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

// parseMcpAppURI extracts an optional serverName and cleaned ui:// URI.
func parseMcpAppURI(rawURI string) (serverName, cleanURI string) {
	cleanURI = strings.TrimSpace(rawURI)
	if !strings.HasPrefix(strings.ToLower(cleanURI), "ui://") {
		return "", cleanURI
	}
	u, err := url.Parse(cleanURI)
	if err != nil {
		return "", cleanURI
	}
	if sn := strings.TrimSpace(u.Query().Get("serverName")); sn != "" {
		serverName = sn
	} else if sn := strings.TrimSpace(u.Query().Get("server")); sn != "" {
		serverName = sn
	} else if h := strings.TrimSpace(u.Host); h != "" {
		serverName = h
	}
	return serverName, cleanURI
}

// extractMcpAppResourceHTML normalizes HTML/text payloads returned by GetMcpAppResource.
func extractMcpAppResourceHTML(body []byte) (content, mimeType string) {
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", ""
	}
	pick := func(m map[string]interface{}) (string, string) {
		if m == nil {
			return "", ""
		}
		mt, _ := m["mimeType"].(string)
		for _, k := range []string{"html", "content", "text"} {
			if s, ok := m[k].(string); ok && s != "" {
				return s, mt
			}
		}
		return "", mt
	}
	if c, mt := pick(raw); c != "" {
		return c, mt
	}
	if res, ok := raw["resource"].(map[string]interface{}); ok {
		if c, mt := pick(res); c != "" {
			return c, mt
		}
	}
	if contents, ok := raw["contents"].([]interface{}); ok {
		for _, item := range contents {
			if m, ok := item.(map[string]interface{}); ok {
				if c, mt := pick(m); c != "" {
					return c, mt
				}
			}
		}
	}
	return "", ""
}

// fetchMcpAppResourceHTML fetches an MCP App ui:// resource via GetMcpAppResource,
// launching the MCP App automatically once if the server instance is not yet active.
func (p *Proxy) fetchMcpAppResourceHTML(ctx context.Context, rawURI, serverName string) (content, mimeType string, rawBody []byte, status int, err error) {
	inferredServer, cleanURI := parseMcpAppURI(rawURI)
	if strings.TrimSpace(serverName) == "" {
		serverName = inferredServer
	}
	reqPayload := map[string]interface{}{
		"uri": cleanURI,
	}
	if serverName != "" {
		reqPayload["serverName"] = serverName
	}

	rawBody, status, err = p.callUpstreamRPC(ctx, "GetMcpAppResource", reqPayload)
	if err != nil {
		return "", "", rawBody, status, err
	}
	if status != http.StatusOK && serverName != "" {
		// Auto-launch MCP App once on first load when server instance isn't active yet.
		launchPayload := map[string]interface{}{
			"serverName": serverName,
			"uri":        cleanURI,
		}
		if _, lStatus, lErr := p.callUpstreamRPC(ctx, "LaunchMcpApp", launchPayload); lErr == nil && lStatus == http.StatusOK {
			rawBody, status, err = p.callUpstreamRPC(ctx, "GetMcpAppResource", reqPayload)
			if err != nil {
				return "", "", rawBody, status, err
			}
		}
	}
	if status != http.StatusOK {
		return "", "", rawBody, status, fmt.Errorf("GetMcpAppResource returned HTTP %d: %s", status, strings.TrimSpace(string(rawBody)))
	}
	content, mimeType = extractMcpAppResourceHTML(rawBody)
	return content, mimeType, rawBody, status, nil
}

// HandleMcpApp bridges interactive <agent-embed> / MCP App lifecycle calls.
func (p *Proxy) HandleMcpApp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload map[string]interface{}
	if r.Method == http.MethodPost {
		_ = json.NewDecoder(io.LimitReader(r.Body, 512*1024)).Decode(&payload)
	}
	if payload == nil {
		payload = map[string]interface{}{}
		for k, vv := range r.URL.Query() {
			if len(vv) > 0 {
				payload[k] = vv[0]
			}
		}
	}

	if uriStr, _ := payload["uri"].(string); uriStr != "" {
		inferredServer, cleanURI := parseMcpAppURI(uriStr)
		payload["uri"] = cleanURI
		if sn, _ := payload["serverName"].(string); strings.TrimSpace(sn) == "" && inferredServer != "" {
			payload["serverName"] = inferredServer
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	var rpcName string
	switch r.URL.Path {
	case "/gateway/mcp/app/launch":
		rpcName = "LaunchMcpApp"
	case "/gateway/mcp/app/resource":
		uriStr, _ := payload["uri"].(string)
		serverName, _ := payload["serverName"].(string)
		content, mimeType, body, status, err := p.fetchMcpAppResourceHTML(ctx, uriStr, serverName)
		if err != nil && len(body) == 0 {
			writeRPCError(w, status, err.Error())
			return
		}
		if status == http.StatusOK && content != "" {
			var merged map[string]interface{}
			if json.Unmarshal(body, &merged) == nil && merged != nil {
				if _, has := merged["content"]; !has {
					merged["content"] = content
				}
				if mimeType != "" {
					if _, has := merged["mimeType"]; !has {
						merged["mimeType"] = mimeType
					}
				}
				if out, mErr := json.Marshal(merged); mErr == nil {
					body = out
				}
			}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write(body)
		return
	case "/gateway/mcp/app/call":
		rpcName = "CallMcpAppTool"
	default:
		http.NotFound(w, r)
		return
	}

	body, status, err := p.callUpstreamRPC(ctx, rpcName, payload)
	if err != nil {
		writeRPCError(w, status, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// HandleSidecarUI reverse-proxies /gateway/sidecar-ui/<sidecarID>/* to the local sidecar web UI port.
// Supports both standalone sidecars ("my-sidecar") and namespaced plugin sidecars ("<plugin>/<folder>").
func (p *Proxy) HandleSidecarUI(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/gateway/sidecar-ui/")
	if strings.TrimSpace(trimmed) == "" {
		http.Error(w, "Missing sidecar ID", http.StatusBadRequest)
		return
	}

	targetBase, matchedID, rest, hasTrailingSlash := p.resolveSidecarUITarget(r.Context(), trimmed)
	if targetBase == nil || matchedID == "" {
		http.Error(w, "Sidecar web UI not running or not found", http.StatusBadGateway)
		return
	}

	// Ensure bare "/gateway/sidecar-ui/<matchedID>" (no trailing slash) redirects to "<matchedID>/"
	// on GET/HEAD so relative assets like <script src="app.js"> resolve inside the sidecar root.
	if rest == "" && !hasTrailingSlash && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		q := r.URL.Query()
		q.Del("ticket")
		redir := r.URL.Path + "/"
		if encoded := q.Encode(); encoded != "" {
			redir += "?" + encoded
		}
		http.Redirect(w, r, redir, http.StatusTemporaryRedirect)
		return
	}

	subPath := "/"
	if rest != "" {
		subPath = "/" + rest
	}

	rp := httputil.NewSingleHostReverseProxy(targetBase)
	origDirector := rp.Director
	rp.Director = func(req *http.Request) {
		origDirector(req)
		req.URL.Path = subPath
		req.URL.RawPath = ""
		if req.URL.RawQuery != "" {
			q := req.URL.Query()
			if q.Has("ticket") {
				q.Del("ticket")
				req.URL.RawQuery = q.Encode()
			}
		}
		// Strip gateway auth cookies before forwarding to local sidecar process.
		if cookies := req.Cookies(); len(cookies) > 0 {
			req.Header.Del("Cookie")
			for _, c := range cookies {
				if c.Name == "agy_dt" || c.Name == "agy_sidecar_st" {
					continue
				}
				req.AddCookie(c)
			}
		}
		req.Host = targetBase.Host
	}
	rp.ServeHTTP(w, r)
}

func (p *Proxy) resolveSidecarUITarget(ctx context.Context, trimmedPath string) (target *url.URL, matchedID string, rest string, hasTrailingSlash bool) {
	port, _ := p.ActiveUpstream()
	if port <= 0 {
		return nil, "", "", false
	}

	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	body, status, err := p.callUpstreamRPC(c, "GetSidecars", map[string]interface{}{})
	cancel()
	if err != nil || status != http.StatusOK {
		return nil, "", "", false
	}

	var up struct {
		Sidecars []struct {
			SidecarID string `json:"sidecarId"`
			ID        string `json:"id"`
			WebPort   int    `json:"webPort"`
			WebUIPort int    `json:"webUiPort"`
			BaseURL   string `json:"baseUrl"`
			WebUIURL  string `json:"webUiUrl"`
		} `json:"sidecars"`
	}
	if json.Unmarshal(body, &up) != nil {
		return nil, "", "", false
	}

	cleanTrimmed := strings.TrimLeft(trimmedPath, "/")
	for _, sc := range up.Sidecars {
		id := strings.TrimSpace(firstNonEmpty(sc.SidecarID, sc.ID))
		if id == "" {
			continue
		}
		var candidateRest string
		var candidateTrailing bool
		if cleanTrimmed == id {
			candidateRest = ""
			candidateTrailing = false
		} else if strings.HasPrefix(cleanTrimmed, id+"/") {
			candidateRest = strings.TrimPrefix(cleanTrimmed, id+"/")
			candidateTrailing = true
		} else {
			continue
		}

		// Prefer the longest matching sidecar ID (e.g. "plugin/panel" over "plugin").
		if len(id) <= len(matchedID) {
			continue
		}

		var candidateURL *url.URL
		if rawURL := strings.TrimSpace(firstNonEmpty(sc.BaseURL, sc.WebUIURL)); rawURL != "" {
			if u, pErr := url.Parse(rawURL); pErr == nil && isLoopbackHost(u.Hostname()) {
				candidateURL = u
			}
		}
		if candidateURL == nil {
			webPort := sc.WebPort
			if webPort <= 0 {
				webPort = sc.WebUIPort
			}
			if webPort > 0 {
				if u, pErr := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", webPort)); pErr == nil {
					candidateURL = u
				}
			}
		}
		if candidateURL != nil {
			target = candidateURL
			matchedID = id
			rest = candidateRest
			hasTrailingSlash = candidateTrailing
		}
	}
	return target, matchedID, rest, hasTrailingSlash
}

func isLoopbackHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

// HandleAutomationCreateDelete creates or removes scheduled sidecar definitions on disk + upstream ManageSidecar.
func (p *Proxy) HandleAutomationCreateDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		writeRPCError(w, http.StatusInternalServerError, "cannot determine user home")
		return
	}
	configDir := filepath.Join(home, ".gemini", "config")
	sidecarsDir := filepath.Join(configDir, "sidecars")

	switch r.URL.Path {
	case "/gateway/automations/create":
		var req struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Cron        string `json:"cron"`
			Schedule    string `json:"schedule"`
			Model       string `json:"model"`
			Prompt      string `json:"prompt"`
			Enabled     *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, "无效的请求体")
			return
		}
		cronExpr := firstNonEmpty(req.Cron, req.Schedule)
		if strings.TrimSpace(cronExpr) == "" || strings.TrimSpace(req.Prompt) == "" {
			writeRPCError(w, http.StatusBadRequest, "请填写 Cron 表达式和执行提示词")
			return
		}
		sid := sanitizeFileSlug(firstNonEmpty(req.ID, req.Name))
		if sid == "" {
			sid = fmt.Sprintf("schedule-%d", time.Now().Unix()%100000)
		}
		if !strings.Contains(sid, "schedule") {
			sid = "schedule-" + sid
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		var modelPtr *string
		if strings.TrimSpace(req.Model) != "" {
			m := strings.TrimSpace(req.Model)
			modelPtr = &m
		}
		args := buildScheduleArgs(nil, cronExpr, modelPtr, req.Prompt)
		displayName := firstNonEmpty(req.Name, sid)

		createdUpstream := false
		if port, _ := p.ActiveUpstream(); port > 0 {
			ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
			upCfg := map[string]interface{}{
				"builtin":       "schedule",
				"restartPolicy": "always",
				"args":          args,
				"displayName":   displayName,
				"description":   strings.TrimSpace(req.Description),
			}
			_, status, err := p.callUpstreamRPC(ctx, "ManageSidecar", map[string]interface{}{
				"sidecarId": sid,
				"action":    "SIDECAR_ACTION_CREATE",
				"config":    upCfg,
			})
			if err == nil && status == http.StatusOK {
				createdUpstream = true
				if enabled {
					_, _, _ = p.callUpstreamRPC(ctx, "ManageSidecar", map[string]interface{}{
						"sidecarId": sid,
						"action":    "SIDECAR_ACTION_ENABLE",
					})
				}
			}
			cancel()
		}

		if !createdUpstream {
			targetDir := filepath.Join(sidecarsDir, sid)
			if err := os.MkdirAll(targetDir, 0o755); err != nil {
				writeRPCError(w, http.StatusInternalServerError, err.Error())
				return
			}
			sidecarCfg := map[string]interface{}{
				"builtin":        "schedule",
				"restart_policy": "always",
				"args":           args,
				"display_name":   displayName,
				"description":    strings.TrimSpace(req.Description),
			}
			raw, _ := json.MarshalIndent(sidecarCfg, "", "  ")
			if err := os.WriteFile(filepath.Join(targetDir, "sidecar.json"), append(raw, '\n'), 0o644); err != nil {
				writeRPCError(w, http.StatusInternalServerError, err.Error())
				return
			}
			_ = updateLocalSidecarEnabled(sid, enabled)
		}

		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":        true,
			"sidecarId": sid,
		})
	case "/gateway/automations/delete":
		var req struct {
			SidecarID string `json:"sidecarId"`
			Name      string `json:"name"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, "无效的请求体")
			return
		}
		reqID := strings.TrimSpace(firstNonEmpty(req.SidecarID, req.Name))
		if reqID == "" {
			writeRPCError(w, http.StatusBadRequest, "缺少 sidecarId")
			return
		}
		if !isSafeSidecarID(reqID) {
			writeRPCError(w, http.StatusBadRequest, "非法的 sidecarId")
			return
		}

		deletedUpstream := false
		if port, _ := p.ActiveUpstream(); port > 0 {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			_, status, err := p.callUpstreamRPC(ctx, "ManageSidecar", map[string]interface{}{
				"sidecarId": reqID,
				"action":    "SIDECAR_ACTION_DELETE",
			})
			cancel()
			if err == nil && status == http.StatusOK {
				deletedUpstream = true
			}
		}
		if !deletedUpstream {
			_ = os.RemoveAll(filepath.Join(sidecarsDir, reqID))
			_ = removeLocalSidecarFromConfig(reqID)
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true, "sidecarId": reqID})
	default:
		http.NotFound(w, r)
	}
}

func removeLocalSidecarFromConfig(sidecarID string) error {
	cfgPath := resolveLocalGeminiConfigPath()
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil
	}
	var doc map[string]interface{}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	if scMap, ok := doc["sidecars"].(map[string]interface{}); ok {
		delete(scMap, sidecarID)
		doc["sidecars"] = scMap
		if out, err := json.MarshalIndent(doc, "", "  "); err == nil {
			_ = os.WriteFile(cfgPath, append(out, '\n'), 0o600)
		}
	}
	return nil
}

// HandleGitExtra handles branch listing, checkout/create branch, and discarding file changes.
func (p *Proxy) HandleGitExtra(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		CascadeID string   `json:"cascadeId"`
		RepoPath  string   `json:"repoPath"`
		Branch    string   `json:"branch"`
		Create    bool     `json:"create"`
		Paths     []string `json:"paths"`
		Path      string   `json:"path"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "无效的请求体")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	repoRoot := strings.TrimSpace(req.RepoPath)
	if repoRoot == "" && req.CascadeID != "" {
		port, token := p.ActiveUpstream()
		if resolved, err := p.resolveCascadeRepo(ctx, req.CascadeID, port, token); err == nil {
			repoRoot = resolved
		}
	}
	if repoRoot == "" {
		writeRPCError(w, http.StatusBadRequest, "无法解析当前会话的 Git 仓库路径")
		return
	}

	switch r.URL.Path {
	case "/gateway/git/branches":
		branches, current, err := gitListBranches(ctx, repoRoot)
		if err != nil {
			writeRPCError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":       true,
			"current":  current,
			"branches": branches,
		})
	case "/gateway/git/checkout":
		branch := strings.TrimSpace(req.Branch)
		if branch == "" || !validBranchNameRe.MatchString(branch) || strings.HasPrefix(branch, "-") {
			writeRPCError(w, http.StatusBadRequest, "分支名不合法")
			return
		}
		args := []string{"-C", repoRoot, "checkout"}
		if req.Create {
			args = append(args, "-b", branch)
		} else {
			args = append(args, branch)
		}
		cmd := exec.CommandContext(ctx, "git", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			writeRPCError(w, http.StatusBadRequest, firstNonEmpty(strings.TrimSpace(string(out)), err.Error()))
			return
		}
		invalidateGitStatusCache(repoRoot)
		branches, current, _ := gitListBranches(ctx, repoRoot)
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":       true,
			"current":  current,
			"branches": branches,
		})
	case "/gateway/git/discard":
		targets := req.Paths
		if req.Path != "" {
			targets = append(targets, req.Path)
		}
		if len(targets) == 0 {
			writeRPCError(w, http.StatusBadRequest, "请指定要丢弃改动的文件路径")
			return
		}
		// Best-effort upstream DiscardFileChanges alongside safe local git restore/clean.
		uris := make([]string, 0, len(targets))
		for _, pth := range targets {
			if abs := resolveRepoSubpath(repoRoot, pth); abs != "" {
				uris = append(uris, filePathToURI(abs))
			}
		}
		if len(uris) > 0 {
			_, _, _ = p.callUpstreamRPC(ctx, "DiscardFileChanges", map[string]interface{}{
				"workspaceUri": filePathToURI(repoRoot),
				"uris":         uris,
			})
		}
		if err := gitDiscardPaths(ctx, repoRoot, targets); err != nil {
			writeRPCError(w, http.StatusBadRequest, err.Error())
			return
		}
		invalidateGitStatusCache(repoRoot)
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

func gitListBranches(ctx context.Context, repoRoot string) ([]string, string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "branch", "--format=%(HEAD)|%(refname:short)")
	out, err := cmd.Output()
	if err != nil {
		return nil, "", err
	}
	var branches []string
	var current string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[1])
		if name == "" {
			continue
		}
		if strings.TrimSpace(parts[0]) == "*" {
			current = name
		}
		branches = append(branches, name)
	}
	return branches, current, nil
}

func resolveRepoSubpath(repoRoot, rel string) string {
	cleanRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return ""
	}
	cand := rel
	if !filepath.IsAbs(cand) {
		cand = filepath.Join(cleanRoot, filepath.FromSlash(cand))
	}
	abs, err := filepath.Abs(filepath.Clean(cand))
	if err != nil {
		return ""
	}
	r, err := filepath.Rel(cleanRoot, abs)
	if err != nil || r == "." || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return ""
	}
	return abs
}

func gitDiscardPaths(ctx context.Context, repoRoot string, paths []string) error {
	cleanRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return err
	}
	// Check untracked status first so we only delete verified untracked files.
	untracked := make(map[string]bool)
	stCmd := exec.CommandContext(ctx, "git", "-C", cleanRoot, "status", "--porcelain", "-uall", "-z")
	if stOut, stErr := stCmd.Output(); stErr == nil {
		for _, rec := range strings.Split(string(stOut), "\x00") {
			if len(rec) > 3 && strings.HasPrefix(rec, "?? ") {
				untracked[filepath.ToSlash(strings.TrimSpace(rec[3:]))] = true
			}
		}
	}

	var tracked []string
	for _, raw := range paths {
		abs := resolveRepoSubpath(cleanRoot, raw)
		if abs == "" {
			return fmt.Errorf("非法路径: %s", raw)
		}
		rel, err := filepath.Rel(cleanRoot, abs)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		if untracked[relSlash] {
			if rmErr := os.RemoveAll(abs); rmErr != nil {
				return rmErr
			}
			continue
		}
		tracked = append(tracked, relSlash)
	}
	if len(tracked) > 0 {
		args := append([]string{"-C", cleanRoot, "checkout", "--"}, tracked...)
		cmd := exec.CommandContext(ctx, "git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
		}
	}
	return nil
}
