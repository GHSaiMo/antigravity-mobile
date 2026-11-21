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

	var rpcName string
	switch r.URL.Path {
	case "/gateway/mcp/app/launch":
		rpcName = "LaunchMcpApp"
	case "/gateway/mcp/app/resource":
		rpcName = "GetMcpAppResource"
	case "/gateway/mcp/app/call":
		rpcName = "CallMcpAppTool"
	default:
		http.NotFound(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
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
func (p *Proxy) HandleSidecarUI(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/gateway/sidecar-ui/")
	parts := strings.SplitN(trimmed, "/", 2)
	sidecarID := strings.TrimSpace(parts[0])
	if sidecarID == "" {
		http.Error(w, "Missing sidecar ID", http.StatusBadRequest)
		return
	}
	subPath := "/"
	if len(parts) == 2 && parts[1] != "" {
		subPath = "/" + parts[1]
	}

	targetBase := p.resolveSidecarUIURL(r.Context(), sidecarID)
	if targetBase == nil {
		http.Error(w, "Sidecar web UI not running or not found", http.StatusBadGateway)
		return
	}

	rp := httputil.NewSingleHostReverseProxy(targetBase)
	origDirector := rp.Director
	rp.Director = func(req *http.Request) {
		origDirector(req)
		req.URL.Path = subPath
		req.Host = targetBase.Host
	}
	rp.ServeHTTP(w, r)
}

func (p *Proxy) resolveSidecarUIURL(ctx context.Context, sidecarID string) *url.URL {
	if port, _ := p.ActiveUpstream(); port > 0 {
		c, cancel := context.WithTimeout(ctx, 2*time.Second)
		body, status, err := p.callUpstreamRPC(c, "GetSidecars", map[string]interface{}{})
		cancel()
		if err == nil && status == http.StatusOK {
			var up struct {
				Sidecars []struct {
					ID      string `json:"id"`
					WebPort int    `json:"webPort"`
					BaseURL string `json:"baseUrl"`
				} `json:"sidecars"`
			}
			if json.Unmarshal(body, &up) == nil {
				for _, sc := range up.Sidecars {
					if sc.ID == sidecarID {
						if sc.BaseURL != "" {
							if u, err := url.Parse(sc.BaseURL); err == nil && isLoopbackHost(u.Hostname()) {
								return u
							}
						}
						if sc.WebPort > 0 {
							if u, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", sc.WebPort)); err == nil {
								return u
							}
						}
					}
				}
			}
		}
	}
	return nil
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
