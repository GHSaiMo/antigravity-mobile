package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// upstreamClient returns an HTTP client suitable for unary RPC calls even in minimal test fixtures.
func (p *Proxy) upstreamClient() *http.Client {
	if p.mediumClient != nil {
		return p.mediumClient
	}
	if p.shortClient != nil {
		return p.shortClient
	}
	return p.streamingHTTPClient()
}

// callUpstreamRPC performs a unary LanguageServerService POST request and returns raw response bytes.
func (p *Proxy) callUpstreamRPC(ctx context.Context, method string, payload interface{}) ([]byte, int, error) {
	port, token := p.ActiveUpstream()
	if port <= 0 {
		return nil, http.StatusServiceUnavailable, fmt.Errorf("no active Antigravity upstream")
	}

	var bodyBytes []byte
	switch v := payload.(type) {
	case nil:
		bodyBytes = []byte("{}")
	case []byte:
		bodyBytes = v
	case json.RawMessage:
		bodyBytes = v
	default:
		var err error
		bodyBytes, err = json.Marshal(v)
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
	}

	endpoint := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/%s", port, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.upstreamClient().Do(req)
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	return respBody, resp.StatusCode, nil
}

func writeJSONStatus(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeRPCError(w http.ResponseWriter, status int, msg string) {
	if status < 400 {
		status = http.StatusBadGateway
	}
	writeJSONStatus(w, status, map[string]interface{}{
		"ok":    false,
		"error": msg,
	})
}

func filePathToURI(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "file://") {
		return p
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(p)}
	return u.String()
}

func uriToFilePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "file://") {
		return raw
	}
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		return filepath.FromSlash(u.Path)
	}
	return strings.TrimPrefix(raw, "file://")
}

// ContextMentionItem is returned by POST /gateway/context/mentions for `@` search across all clients.
type ContextMentionItem struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"` // "file", "directory", "conversation", "rule"
	Label       string                 `json:"label"`
	Detail      string                 `json:"detail,omitempty"`
	Path        string                 `json:"path,omitempty"`
	URI         string                 `json:"uri,omitempty"`
	CascadeID   string                 `json:"cascadeId,omitempty"`
	ScopeItem   map[string]interface{} `json:"scopeItem,omitempty"`
}

// appendNormalizedContextItems injects client-provided `contextItems` into `rawMap["items"]` using
// LanguageServer's TextOrScopeItem contract so `@file` / `@dir` / `@conversation` attach natively.
func appendNormalizedContextItems(rawMap map[string]interface{}) {
	rawContext, ok := rawMap["contextItems"].([]interface{})
	if !ok || len(rawContext) == 0 {
		return
	}
	items, _ := rawMap["items"].([]interface{})
	for _, rc := range rawContext {
		cm, ok := rc.(map[string]interface{})
		if !ok {
			continue
		}
		if scopeItem, ok := cm["scopeItem"].(map[string]interface{}); ok && len(scopeItem) > 0 {
			items = append(items, map[string]interface{}{"item": scopeItem})
			continue
		}
		typ, _ := cm["type"].(string)
		uri, _ := cm["uri"].(string)
		path, _ := cm["path"].(string)
		if uri == "" && path != "" {
			uri = filePathToURI(path)
		}
		switch strings.ToLower(typ) {
		case "directory", "dir", "folder":
			if uri != "" {
				items = append(items, map[string]interface{}{
					"item": map[string]interface{}{
						"directory": map[string]interface{}{"absoluteUri": uri},
					},
				})
			}
		case "conversation", "cascade", "chat":
			cid, _ := cm["cascadeId"].(string)
			title, _ := cm["label"].(string)
			if cid != "" {
				conv := map[string]interface{}{"id": cid}
				if title != "" {
					conv["title"] = title
				}
				items = append(items, map[string]interface{}{
					"item": map[string]interface{}{"conversation": conv},
				})
			}
		default:
			if uri != "" {
				items = append(items, map[string]interface{}{
					"item": map[string]interface{}{
						"file": map[string]interface{}{"absoluteUri": uri},
					},
				})
			}
		}
	}
	rawMap["items"] = items
	delete(rawMap, "contextItems")
}

// HandleContextMentions resolves `@` mentions across LanguageServer, workspace files, conversations, and rules.
func (p *Proxy) HandleContextMentions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query        string `json:"query"`
		CascadeID    string `json:"cascadeId"`
		WorkspaceURI string `json:"workspaceUri"`
		Limit        int    `json:"limit"`
	}
	if r.Method == http.MethodPost {
		_ = json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req)
	} else {
		q := r.URL.Query()
		req.Query = q.Get("query")
		req.CascadeID = q.Get("cascadeId")
		req.WorkspaceURI = q.Get("workspaceUri")
	}
	if req.Limit <= 0 || req.Limit > 60 {
		req.Limit = 30
	}

	query := strings.TrimSpace(req.Query)
	queryLower := strings.ToLower(query)

	results := make([]ContextMentionItem, 0, req.Limit)
	seen := make(map[string]bool)
	addResult := func(it ContextMentionItem) {
		if len(results) >= req.Limit {
			return
		}
		key := it.Type + ":" + it.ID + ":" + it.URI + ":" + it.CascadeID
		if seen[key] {
			return
		}
		seen[key] = true
		results = append(results, it)
	}

	// 1. Upstream GetMatchingContextScopeItems (works when IDE has open buffers/repo context).
	if port, _ := p.ActiveUpstream(); port > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		body, status, err := p.callUpstreamRPC(ctx, "GetMatchingContextScopeItems", map[string]interface{}{
			"currentTrajectoryId": req.CascadeID,
			"query":               query,
		})
		cancel()
		if err == nil && status == http.StatusOK {
			var up struct {
				Items []struct {
					File *struct {
						AbsoluteURI string `json:"absoluteUri"`
					} `json:"file"`
					Directory *struct {
						AbsoluteURI string `json:"absoluteUri"`
					} `json:"directory"`
					Conversation *struct {
						ID    string `json:"id"`
						Title string `json:"title"`
					} `json:"conversation"`
				} `json:"items"`
			}
			if json.Unmarshal(body, &up) == nil {
				for _, item := range up.Items {
					if item.File != nil && item.File.AbsoluteURI != "" {
						fp := uriToFilePath(item.File.AbsoluteURI)
						addResult(ContextMentionItem{
							ID:        item.File.AbsoluteURI,
							Type:      "file",
							Label:     filepath.Base(fp),
							Detail:    fp,
							Path:      fp,
							URI:       item.File.AbsoluteURI,
							ScopeItem: map[string]interface{}{"file": map[string]interface{}{"absoluteUri": item.File.AbsoluteURI}},
						})
					} else if item.Directory != nil && item.Directory.AbsoluteURI != "" {
						fp := uriToFilePath(item.Directory.AbsoluteURI)
						addResult(ContextMentionItem{
							ID:        item.Directory.AbsoluteURI,
							Type:      "directory",
							Label:     filepath.Base(fp) + "/",
							Detail:    fp,
							Path:      fp,
							URI:       item.Directory.AbsoluteURI,
							ScopeItem: map[string]interface{}{"directory": map[string]interface{}{"absoluteUri": item.Directory.AbsoluteURI}},
						})
					} else if item.Conversation != nil && item.Conversation.ID != "" {
						addResult(ContextMentionItem{
							ID:        item.Conversation.ID,
							Type:      "conversation",
							Label:     firstNonEmpty(item.Conversation.Title, item.Conversation.ID),
							Detail:    "会话引用",
							CascadeID: item.Conversation.ID,
							ScopeItem: map[string]interface{}{"conversation": map[string]interface{}{"id": item.Conversation.ID, "title": item.Conversation.Title}},
						})
					}
				}
			}
		}
	}

	// 2. Local workspace files & directories scan.
	wsPath := p.resolveMentionWorkspacePath(r.Context(), req.CascadeID, req.WorkspaceURI)
	if wsPath != "" {
		for _, item := range scanWorkspaceMentions(r.Context(), wsPath, queryLower, req.Limit-len(results)) {
			addResult(item)
		}
	}

	// 3. Matching recent conversations so user can reference another session via `@`.
	if len(results) < req.Limit {
		if port, token := p.ActiveUpstream(); port > 0 {
			if titles, err := p.fetchTrajectoriesSummaryWithTitles(port, token); err == nil {
				for cid, title := range titles {
					if cid == "" || cid == req.CascadeID {
						continue
					}
					if queryLower != "" && !strings.Contains(strings.ToLower(title), queryLower) && !strings.Contains(strings.ToLower(cid), queryLower) {
						continue
					}
					addResult(ContextMentionItem{
						ID:        cid,
						Type:      "conversation",
						Label:     firstNonEmpty(title, cid),
						Detail:    "历史会话",
						CascadeID: cid,
						ScopeItem: map[string]interface{}{"conversation": map[string]interface{}{"id": cid, "title": title}},
					})
					if len(results) >= req.Limit {
						break
					}
				}
			}
		}
	}

	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"ok":    true,
		"items": results,
	})
}

func (p *Proxy) resolveMentionWorkspacePath(ctx context.Context, cascadeID, workspaceURI string) string {
	if workspaceURI != "" {
		if path := uriToFilePath(workspaceURI); path != "" {
			if st, err := os.Stat(path); err == nil && st.IsDir() {
				return path
			}
		}
	}
	port, token := p.ActiveUpstream()
	if cascadeID != "" && port > 0 {
		if root, err := p.resolveCascadeRepo(ctx, cascadeID, port, token); err == nil && root != "" {
			return root
		}
		if raw, err := p.fetchUpstreamTrajectoryWithMaxAgeContext(ctx, cascadeID, port, token, 5*time.Second); err == nil && raw != nil {
			if len(raw.Trajectory.WorkspaceUris) > 0 {
				if path := uriToFilePath(raw.Trajectory.WorkspaceUris[0]); path != "" {
					if st, err := os.Stat(path); err == nil && st.IsDir() {
						return path
					}
				}
			}
		}
	}
	return ""
}

func scanWorkspaceMentions(ctx context.Context, root, queryLower string, limit int) []ContextMentionItem {
	if root == "" || limit <= 0 {
		return nil
	}
	out := make([]ContextMentionItem, 0, limit)
	seen := make(map[string]bool)

	addPath := func(rel string, isDir bool) {
		if len(out) >= limit {
			return
		}
		rel = filepath.ToSlash(rel)
		if rel == "." || rel == "" {
			return
		}
		if queryLower != "" && !strings.Contains(strings.ToLower(rel), queryLower) {
			return
		}
		abs := filepath.Join(root, filepath.FromSlash(rel))
		uri := filePathToURI(abs)
		if seen[uri] {
			return
		}
		seen[uri] = true
		if isDir {
			out = append(out, ContextMentionItem{
				ID:        uri,
				Type:      "directory",
				Label:     filepath.Base(abs) + "/",
				Detail:    rel,
				Path:      abs,
				URI:       uri,
				ScopeItem: map[string]interface{}{"directory": map[string]interface{}{"absoluteUri": uri}},
			})
			return
		}
		out = append(out, ContextMentionItem{
			ID:        uri,
			Type:      "file",
			Label:     filepath.Base(abs),
			Detail:    rel,
			Path:      abs,
			URI:       uri,
			ScopeItem: map[string]interface{}{"file": map[string]interface{}{"absoluteUri": uri}},
		})
	}

	// Fast path via `git ls-files` inside git repositories.
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		cmdCtx, cancel := context.WithTimeout(ctx, 1200*time.Millisecond)
		cmd := exec.CommandContext(cmdCtx, "git", "-C", root, "ls-files", "-z")
		outBytes, err := cmd.Output()
		cancel()
		if err == nil && len(outBytes) > 0 {
			dirSeen := make(map[string]bool)
			parts := bytes.Split(outBytes, []byte{0})
			var files []string
			for _, raw := range parts {
				if len(raw) == 0 {
					continue
				}
				rel := string(raw)
				files = append(files, rel)
				dir := filepath.ToSlash(filepath.Dir(rel))
				if dir != "." && !dirSeen[dir] {
					dirSeen[dir] = true
				}
			}
			// Prefer prefix/filename matches first.
			sort.SliceStable(files, func(i, j int) bool {
				bi := strings.ToLower(filepath.Base(files[i]))
				bj := strings.ToLower(filepath.Base(files[j]))
				pi := queryLower != "" && strings.HasPrefix(bi, queryLower)
				pj := queryLower != "" && strings.HasPrefix(bj, queryLower)
				if pi != pj {
					return pi
				}
				return len(files[i]) < len(files[j])
			})
			for d := range dirSeen {
				if len(out) >= limit/3 && queryLower == "" {
					break
				}
				addPath(d, true)
			}
			for _, f := range files {
				if len(out) >= limit {
					break
				}
				addPath(f, false)
			}
			return out
		}
	}

	skipDirs := map[string]bool{
		".git": true, "node_modules": true, "build": true, ".gradle": true,
		"DerivedData": true, "dist": true, ".next": true, "__pycache__": true,
	}
	visited := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if len(out) >= limit || visited > 1500 {
			return filepath.SkipAll
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			if strings.Count(filepath.ToSlash(rel), "/") > 4 {
				return filepath.SkipDir
			}
		}
		visited++
		addPath(rel, d.IsDir())
		return nil
	})
	return out
}

// HandleCascadeFork forks an existing conversation from a step index, optionally in a new git worktree.
func (p *Proxy) HandleCascadeFork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		CascadeID      string `json:"cascadeId"`
		StepIndex      int    `json:"stepIndex"`
		CreateWorktree bool   `json:"createWorktree"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	if strings.TrimSpace(req.CascadeID) == "" {
		writeRPCError(w, http.StatusBadRequest, "缺少 cascadeId")
		return
	}

	target := "FORK_TARGET_CURRENT_WORKSPACE"
	if req.CreateWorktree {
		target = "FORK_TARGET_NEW_WORKTREE"
	}

	payload := map[string]interface{}{
		"sourceCascadeId":     req.CascadeID,
		"forkAtStepIndex":     req.StepIndex,
		"targetForkWorkspace": target,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	body, status, err := p.callUpstreamRPC(ctx, "ForkConversation", payload)
	if err != nil {
		writeRPCError(w, status, err.Error())
		return
	}
	if status != http.StatusOK {
		writeRPCError(w, status, strings.TrimSpace(string(body)))
		return
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		writeRPCError(w, http.StatusBadGateway, "上游分叉响应解析失败")
		return
	}
	cid, _ := resp["cascadeId"].(string)
	if cid == "" {
		cid, _ = resp["cascade_id"].(string)
	}
	p.notifyStreamTouch(cid)
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"cascadeId": cid,
		"raw":       resp,
	})
}

// HandleWorktree manages Git worktrees via CreateWorktree / CheckoutWorktree / GetWorktreeDiff / DeleteWorktree.
func (p *Proxy) HandleWorktree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	action := strings.TrimPrefix(r.URL.Path, "/gateway/worktree/")
	var req struct {
		RepoURI                   string `json:"repoUri"`
		WorkspacePath             string `json:"workspacePath"`
		BaseBranchName            string `json:"baseBranchName"`
		WorktreeName              string `json:"worktreeName"`
		WorktreeDirURI            string `json:"worktreeDirUri"`
		TargetWorkspaceURI        string `json:"targetWorkspaceUri"`
		DeleteWorktreeAfterCheck  bool   `json:"deleteWorktreeAfterCheckout"`
		StartSession              bool   `json:"startSession"`
		Prompt                    string `json:"prompt"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "无效的请求体")
		return
	}

	if req.RepoURI == "" && req.WorkspacePath != "" {
		req.RepoURI = filePathToURI(req.WorkspacePath)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	switch action {
	case "create":
		if req.RepoURI == "" {
			writeRPCError(w, http.StatusBadRequest, "缺少 repoUri")
			return
		}
		if req.WorktreeName == "" {
			req.WorktreeName = fmt.Sprintf("mgy-%d", time.Now().Unix()%100000)
		}
		payload := map[string]interface{}{
			"repoUri":        req.RepoURI,
			"baseBranchName": req.BaseBranchName,
			"worktreeName":   req.WorktreeName,
		}
		body, status, err := p.callUpstreamRPC(ctx, "CreateWorktree", payload)
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, strings.TrimSpace(string(body)))
			return
		}
		var wt struct {
			WorktreeDirURI string `json:"worktreeDirUri"`
			WorktreeName   string `json:"worktreeName"`
		}
		_ = json.Unmarshal(body, &wt)
		cascadeID := ""
		if req.StartSession && wt.WorktreeDirURI != "" {
			cBody, cStatus, cErr := p.callUpstreamRPC(ctx, "StartCascade", map[string]interface{}{
				"workspaceUris": []string{wt.WorktreeDirURI},
			})
			if cErr == nil && cStatus == http.StatusOK {
				var started struct {
					CascadeID string `json:"cascadeId"`
				}
				if json.Unmarshal(cBody, &started) == nil {
					cascadeID = started.CascadeID
				}
			}
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":             true,
			"worktreeDirUri": wt.WorktreeDirURI,
			"worktreeName":   firstNonEmpty(wt.WorktreeName, req.WorktreeName),
			"cascadeId":      cascadeID,
		})
	case "checkout":
		body, status, err := p.callUpstreamRPC(ctx, "CheckoutWorktree", map[string]interface{}{
			"worktreeDirUri":              req.WorktreeDirURI,
			"targetWorkspaceUri":          req.TargetWorkspaceURI,
			"deleteWorktreeAfterCheckout": req.DeleteWorktreeAfterCheck,
		})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, strings.TrimSpace(string(body)))
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true, "raw": json.RawMessage(body)})
	case "diff":
		body, status, err := p.callUpstreamRPC(ctx, "GetWorktreeDiff", map[string]interface{}{
			"worktreeDirUri": req.WorktreeDirURI,
		})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, strings.TrimSpace(string(body)))
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	case "delete":
		body, status, err := p.callUpstreamRPC(ctx, "DeleteWorktree", map[string]interface{}{
			"worktreeDirUri": req.WorktreeDirURI,
		})
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

// HandleCloneGitHubRepo clones a remote GitHub repository (`owner/repo` or URL) and optionally starts a session.
func (p *Proxy) HandleCloneGitHubRepo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		RepoSlug     string `json:"repoSlug"`
		RepoURL      string `json:"repoUrl"`
		ParentDir    string `json:"parentDir"`
		ParentDirURI string `json:"parentDirUri"`
		StartSession bool   `json:"startSession"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	slug := normalizeGitHubSlug(firstNonEmpty(req.RepoSlug, req.RepoURL))
	if slug == "" {
		writeRPCError(w, http.StatusBadRequest, "请输入有效的 GitHub 仓库地址或 owner/repo")
		return
	}

	parentDir := strings.TrimSpace(req.ParentDir)
	if parentDir == "" && req.ParentDirURI != "" {
		parentDir = uriToFilePath(req.ParentDirURI)
	}
	if parentDir == "" {
		home, _ := os.UserHomeDir()
		parentDir = filepath.Join(home, "Projects")
		_ = os.MkdirAll(parentDir, 0o755)
	}
	parentURI := firstNonEmpty(req.ParentDirURI, filePathToURI(parentDir))

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	body, status, err := p.callUpstreamRPC(ctx, "CloneGitHubRepo", map[string]interface{}{
		"repoSlug":     slug,
		"parentDirUri": parentURI,
	})
	if err != nil {
		writeRPCError(w, status, err.Error())
		return
	}
	if status != http.StatusOK {
		writeRPCError(w, status, strings.TrimSpace(string(body)))
		return
	}
	var cloned struct {
		FolderURI string `json:"folderUri"`
	}
	_ = json.Unmarshal(body, &cloned)

	p.invalidateProjectsCache()

	cascadeID := ""
	if req.StartSession && cloned.FolderURI != "" {
		cBody, cStatus, cErr := p.callUpstreamRPC(ctx, "StartCascade", map[string]interface{}{
			"workspaceUris": []string{cloned.FolderURI},
		})
		if cErr == nil && cStatus == http.StatusOK {
			var started struct {
				CascadeID string `json:"cascadeId"`
			}
			if json.Unmarshal(cBody, &started) == nil {
				cascadeID = started.CascadeID
			}
		}
	}

	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"folderUri": cloned.FolderURI,
		"path":      uriToFilePath(cloned.FolderURI),
		"cascadeId": cascadeID,
	})
}

// HandleCreateProject registers a local folder path as an Antigravity workspace project
// using ResolveFolder + CreateProject RPC (falling back to local ~/.gemini/config/projects JSON if offline).
func (p *Proxy) HandleCreateProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Path         string `json:"path"`
		Name         string `json:"name"`
		StartSession bool   `json:"startSession"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "无效的请求体")
		return
	}

	cleanPath, err := normalizeLocalWorkspacePath(req.Path)
	if err != nil {
		writeRPCError(w, http.StatusBadRequest, err.Error())
		return
	}

	folderURI := normalizeURI(filePathToURI(cleanPath))
	if folderURI == "" {
		writeRPCError(w, http.StatusBadRequest, "无法解析该路径的 URI")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = filepath.Base(cleanPath)
	}
	if name == "" || name == "/" || name == "\\" || name == "." {
		writeRPCError(w, http.StatusBadRequest, "无效的文件夹名称")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	// Avoid creating duplicate entries if this folder is already registered.
	if existing, err := p.GetProjects(); err == nil {
		for _, prj := range existing {
			if normalizeURI(prj.URI) == folderURI || (prj.Path != "" && filepath.Clean(prj.Path) == cleanPath) {
				cascadeID := ""
				if req.StartSession {
					cascadeID = p.startCascadeForProject(ctx, prj.ID, folderURI)
				}
				writeJSONStatus(w, http.StatusOK, map[string]interface{}{
					"ok":            true,
					"alreadyExists": true,
					"project":       prj,
					"cascadeId":     cascadeID,
				})
				return
			}
		}
	}

	port, _ := p.ActiveUpstream()
	isGit, defaultBranch := p.detectProjectFolderType(ctx, port, cleanPath, folderURI)

	projectID := newUUIDv4()
	var resourceObj map[string]interface{}
	if isGit {
		resourceObj = map[string]interface{}{
			"gitFolder": map[string]interface{}{
				"folderUri":     folderURI,
				"defaultBranch": defaultBranch,
			},
		}
	} else {
		resourceObj = map[string]interface{}{
			"folderUri": folderURI,
		}
	}

	if port > 0 {
		payload := map[string]interface{}{
			"project": map[string]interface{}{
				"id":   projectID,
				"name": name,
				"projectResources": map[string]interface{}{
					"resources": []interface{}{resourceObj},
				},
				"isWorkspaceOnly": false,
				"settings":        map[string]interface{}{},
			},
		}
		body, status, err := p.callUpstreamRPC(ctx, "CreateProject", payload)
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, firstNonEmpty(strings.TrimSpace(string(body)), fmt.Sprintf("CreateProject failed (%d)", status)))
			return
		}
	} else {
		if err := writeFallbackGeminiProjectFile(projectID, name, resourceObj); err != nil {
			writeRPCError(w, http.StatusInternalServerError, fmt.Sprintf("保存工作区配置失败: %v", err))
			return
		}
	}

	p.invalidateProjectsCache()

	cascadeID := ""
	if req.StartSession && port > 0 {
		cascadeID = p.startCascadeForProject(ctx, projectID, folderURI)
	}

	now := time.Now()
	created := ProjectItem{
		ID:          projectID,
		Name:        name,
		URI:         folderURI,
		Path:        cleanPath,
		IsWorkspace: false,
		LastActive:  &now,
	}

	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"ok":            true,
		"alreadyExists": false,
		"project":       created,
		"cascadeId":     cascadeID,
	})
}

func normalizeLocalWorkspacePath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	p = strings.Trim(p, `"'`)
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("请输入电脑本地路径")
	}
	if strings.HasPrefix(p, "file://") {
		p = uriToPath(p)
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", fmt.Errorf("无法解析用户主目录 ~")
		}
		if p == "~" {
			p = home
		} else {
			p = filepath.Join(home, p[2:])
		}
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("请输入绝对路径，例如 /Users/... 或 ~/...")
	}
	cleanPath := filepath.Clean(p)
	if isDriveRoot(cleanPath) {
		return "", fmt.Errorf("不能将系统根目录作为工作区")
	}
	fi, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("本地路径不存在: %s", cleanPath)
		}
		return "", fmt.Errorf("无法访问该路径: %v", err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("该路径不是文件夹: %s", cleanPath)
	}
	return cleanPath, nil
}

func (p *Proxy) detectProjectFolderType(ctx context.Context, port int, cleanPath, folderURI string) (bool, string) {
	isGit := false
	defaultBranch := ""
	if port > 0 {
		rCtx, rCancel := context.WithTimeout(ctx, 3*time.Second)
		body, status, err := p.callUpstreamRPC(rCtx, "ResolveFolder", map[string]interface{}{
			"folderUri": folderURI,
		})
		rCancel()
		if err == nil && status == http.StatusOK {
			var resolved struct {
				ResourceType string `json:"resourceType"`
			}
			if json.Unmarshal(body, &resolved) == nil && resolved.ResourceType == "FOLDER_TYPE_GIT_FOLDER" {
				isGit = true
			}
		} else if _, statErr := os.Stat(filepath.Join(cleanPath, ".git")); statErr == nil {
			isGit = true
		}

		if isGit {
			gCtx, gCancel := context.WithTimeout(ctx, 2*time.Second)
			gBody, gStatus, gErr := p.callUpstreamRPC(gCtx, "GetRepoInfos", map[string]interface{}{
				"repoUris": []string{folderURI},
			})
			gCancel()
			if gErr == nil && gStatus == http.StatusOK {
				var repoResp struct {
					Repos []struct {
						Branches []struct {
							Name            string `json:"name"`
							IsCurrentBranch bool   `json:"isCurrentBranch"`
						} `json:"branches"`
					} `json:"repos"`
				}
				if json.Unmarshal(gBody, &repoResp) == nil && len(repoResp.Repos) > 0 {
					for _, b := range repoResp.Repos[0].Branches {
						if b.IsCurrentBranch && b.Name != "" {
							defaultBranch = b.Name
							break
						}
					}
				}
			}
		}
		return isGit, defaultBranch
	}

	if _, statErr := os.Stat(filepath.Join(cleanPath, ".git")); statErr == nil {
		isGit = true
	}
	return isGit, defaultBranch
}

func (p *Proxy) startCascadeForProject(ctx context.Context, projectID, folderURI string) string {
	payload := map[string]interface{}{}
	if projectID != "" {
		payload["source"] = "CORTEX_TRAJECTORY_SOURCE_CASCADE_CLIENT"
		payload["workspaceUris"] = []string{}
		payload["projectEnvConfig"] = map[string]interface{}{
			"projectId":                 projectID,
			"defaultProjectEnvironment": map[string]interface{}{},
		}
	} else {
		payload["source"] = "CORTEX_TRAJECTORY_SOURCE_INTERACTIVE_CASCADE"
		payload["workspaceUris"] = []string{folderURI}
	}
	cBody, cStatus, cErr := p.callUpstreamRPC(ctx, "StartCascade", payload)
	if cErr == nil && cStatus == http.StatusOK {
		var started struct {
			CascadeID string `json:"cascadeId"`
		}
		if json.Unmarshal(cBody, &started) == nil {
			return started.CascadeID
		}
	}
	return ""
}

func writeFallbackGeminiProjectFile(projectID, name string, resourceObj map[string]interface{}) error {
	dirs := getGeminiConfigProjectsDirs()
	if len(dirs) == 0 {
		return fmt.Errorf("cannot resolve projects config directory")
	}
	projDir := filepath.Clean(dirs[0])
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		return err
	}
	doc := map[string]interface{}{
		"id":   projectID,
		"name": name,
		"projectResources": map[string]interface{}{
			"resources": []interface{}{resourceObj},
		},
		"permissionGrants": map[string]interface{}{
			"v2Migrated": true,
		},
		"settings":        map[string]interface{}{},
		"isWorkspaceOnly": false,
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(projDir, projectID+".json"), append(raw, '\n'), 0o644)
}

func newUUIDv4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func normalizeGitHubSlug(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimPrefix(s, "git@github.com:")
	if strings.HasPrefix(s, "https://github.com/") || strings.HasPrefix(s, "http://github.com/") {
		s = strings.TrimPrefix(strings.TrimPrefix(s, "https://github.com/"), "http://github.com/")
	}
	s = strings.Trim(s, "/")
	parts := strings.Split(s, "/")
	if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
		return parts[0] + "/" + parts[1]
	}
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
