package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 手机端 Git 提交：直接在会话所属工作区执行 git status / add / commit / push，
// 不再让 Agent 用一轮对话（消耗额度与时间）去跑 git。
//
// language_server 的 GitCommit 可用，但 GetVersionControlState / GenerateCommitMessage 对未登记的仓库
// 返回空或 "repository does not exist"，拿不到变更列表，因此网关自己调用本机 git。
//
// 安全边界：
//   - 客户端只传 cascadeId，工作区目录由网关从会话的 workspaces 解析，不接受任意路径；
//   - 待提交路径必须出现在当前 git status 里，且不能以 "-" 开头，参数前统一加 "--"；
//   - push 不带 --force，不接受自定义 remote / refspec；
//   - 不读取、不转发任何凭据，认证沿用本机 git 的现有配置。

const (
	gitStatusTimeout  = 15 * time.Second
	gitStatusCacheTTL = 4 * time.Second
	gitCommitTimeout  = 60 * time.Second
	gitPushTimeout    = 120 * time.Second
	gitMaxOutput      = 64 * 1024
	gitMaxMessageLen  = 8000
)

type gitStatusCacheEntry struct {
	res       GitStatusResponse
	fetchedAt time.Time
}

type gitStatusInflight struct {
	done chan struct{}
	res  *GitStatusResponse
	err  error
}

var (
	gitStatusCacheMu sync.Mutex
	gitStatusCache   = map[string]gitStatusCacheEntry{}
	gitStatusCalls   = map[string]*gitStatusInflight{}
)

func cloneGitStatusResponse(src GitStatusResponse) *GitStatusResponse {
	cp := src
	if len(src.Files) > 0 {
		cp.Files = append([]GitFileStatus(nil), src.Files...)
	}
	return &cp
}

func invalidateGitStatusCache(repoRoot string) {
	key := filepath.Clean(strings.TrimSpace(repoRoot))
	if key == "" || key == "." {
		return
	}
	gitStatusCacheMu.Lock()
	delete(gitStatusCache, key)
	gitStatusCacheMu.Unlock()
}

// gitStatusCached reuses recent git status checks for large repos and deduplicates concurrent refreshes.
func gitStatusCached(ctx context.Context, repoRoot string, forceRefresh bool) (*GitStatusResponse, error) {
	key := filepath.Clean(strings.TrimSpace(repoRoot))
	if key == "" {
		return gitStatus(ctx, repoRoot)
	}
	now := time.Now()
	gitStatusCacheMu.Lock()
	if !forceRefresh {
		if entry, ok := gitStatusCache[key]; ok && now.Sub(entry.fetchedAt) < gitStatusCacheTTL {
			gitStatusCacheMu.Unlock()
			return cloneGitStatusResponse(entry.res), nil
		}
		if inflight, ok := gitStatusCalls[key]; ok {
			gitStatusCacheMu.Unlock()
			select {
			case <-inflight.done:
				if inflight.err != nil {
					return nil, inflight.err
				}
				return cloneGitStatusResponse(*inflight.res), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	inflight := &gitStatusInflight{done: make(chan struct{})}
	gitStatusCalls[key] = inflight
	gitStatusCacheMu.Unlock()

	res, err := gitStatus(ctx, key)

	gitStatusCacheMu.Lock()
	if err == nil && res != nil {
		gitStatusCache[key] = gitStatusCacheEntry{
			res:       *cloneGitStatusResponse(*res),
			fetchedAt: time.Now(),
		}
	}
	if gitStatusCalls[key] == inflight {
		delete(gitStatusCalls, key)
	}
	inflight.res = res
	inflight.err = err
	close(inflight.done)
	gitStatusCacheMu.Unlock()

	if err != nil {
		return nil, err
	}
	return cloneGitStatusResponse(*res), nil
}

var errNotGitRepo = errors.New("该会话的工作区不是 Git 仓库")

// GitFileStatus is one changed path in the working tree.
type GitFileStatus struct {
	Path     string `json:"path"`
	OrigPath string `json:"origPath,omitempty"` // rename source
	Index    string `json:"index"`              // porcelain X column: staged state
	Worktree string `json:"worktree"`           // porcelain Y column
	Staged   bool   `json:"staged"`
	Status   string `json:"status"` // MODIFIED / ADDED / DELETED / RENAMED / UNTRACKED / CONFLICT
}

// GitHubBranchTarget identifies the GitHub repository and branch associated with the workspace.
type GitHubBranchTarget struct {
	Host   string `json:"host,omitempty"`
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Branch string `json:"branch,omitempty"`
}

// GitPullRequestItem represents a GitHub Pull Request associated with the workspace branch.
type GitPullRequestItem struct {
	Host        string `json:"host,omitempty"`
	Owner       string `json:"owner,omitempty"`
	Repo        string `json:"repo,omitempty"`
	Number      int    `json:"number"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	State       string `json:"state"` // OPEN / DRAFT / MERGED / CLOSED
	AuthorLogin string `json:"authorLogin,omitempty"`
	HeadRefName string `json:"headRefName,omitempty"`
	BaseRefName string `json:"baseRefName,omitempty"`
	Body        string `json:"body,omitempty"`
	Error       string `json:"error,omitempty"`
}

type GitHubPullRequestRef struct {
	Host   string `json:"host,omitempty"`
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}

// GitStatusResponse is the response of POST /gateway/git/status.
type GitStatusResponse struct {
	RepoName     string               `json:"repoName"`
	Branch       string               `json:"branch"`
	Upstream     string               `json:"upstream,omitempty"`
	Ahead        int                  `json:"ahead"`
	Behind       int                  `json:"behind"`
	Detached     bool                 `json:"detached"`
	Files        []GitFileStatus      `json:"files"`
	Clean        bool                 `json:"clean"`
	GitHubTarget *GitHubBranchTarget  `json:"githubTarget,omitempty"`
	PullRequests []GitPullRequestItem `json:"pullRequests,omitempty"`
}

// GitCommitRequest is the request of POST /gateway/git/commit.
type GitCommitRequest struct {
	CascadeID string   `json:"cascadeId"`
	Message   string   `json:"message"`
	Paths     []string `json:"paths"` // empty = every changed file
	Push      bool     `json:"push"`
}

// GitCommitResponse is the response of commit and push.
type GitCommitResponse struct {
	Committed bool   `json:"committed"`
	Pushed    bool   `json:"pushed"`
	CommitID  string `json:"commitId,omitempty"`
	Output    string `json:"output,omitempty"`
	// PushError is set when the commit succeeded but the push failed, so the client can offer a retry.
	PushError string `json:"pushError,omitempty"`
}

type gitCascadeRequest struct {
	CascadeID string `json:"cascadeId"`
}

// ---------------------------------------------------------------------------
// git execution
// ---------------------------------------------------------------------------

type limitedBuffer struct {
	buf bytes.Buffer
	max int
}

func (l *limitedBuffer) Write(b []byte) (int, error) {
	if room := l.max - l.buf.Len(); room > 0 {
		if len(b) > room {
			l.buf.Write(b[:room])
		} else {
			l.buf.Write(b)
		}
	}
	return len(b), nil
}

func runGit(parent context.Context, dir string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", // 绝不在无人值守的网关上等待用户名密码
		"GIT_OPTIONAL_LOCKS=0",
		"LC_ALL=C",
	)
	out := &limitedBuffer{max: gitMaxOutput}
	cmd.Stdout = out
	cmd.Stderr = out
	err := cmd.Run()
	text := strings.TrimSpace(out.buf.String())
	if ctx.Err() == context.DeadlineExceeded {
		return text, fmt.Errorf("git %s 超时（%s）", args[0], timeout)
	}
	if err != nil {
		if text == "" {
			text = err.Error()
		}
		return text, fmt.Errorf("git %s 失败: %s", args[0], text)
	}
	return text, nil
}

// gitRepoRoot returns the repository top-level for dir, or errNotGitRepo.
func gitRepoRoot(ctx context.Context, dir string) (string, error) {
	out, err := runGit(ctx, dir, gitStatusTimeout, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errNotGitRepo
	}
	root := strings.TrimSpace(out)
	if root == "" {
		return "", errNotGitRepo
	}
	return filepath.Clean(root), nil
}

// parseGitStatus parses `git status --porcelain=v1 -z --branch` output.
func parseGitStatus(raw string) GitStatusResponse {
	res := GitStatusResponse{Files: []GitFileStatus{}}
	parts := strings.Split(raw, "\x00")
	for i := 0; i < len(parts); i++ {
		entry := parts[i]
		if entry == "" {
			continue
		}
		if strings.HasPrefix(entry, "## ") {
			parseGitBranchLine(strings.TrimPrefix(entry, "## "), &res)
			continue
		}
		if len(entry) < 4 {
			continue
		}
		x, y := string(entry[0]), string(entry[1])
		f := GitFileStatus{Path: entry[3:], Index: x, Worktree: y}
		if x == "R" || x == "C" || y == "R" || y == "C" {
			// -z 下重命名的源路径紧随其后作为独立条目
			if i+1 < len(parts) {
				f.OrigPath = parts[i+1]
				i++
			}
		}
		f.Staged = x != " " && x != "?" && x != "!"
		f.Status = classifyGitStatus(x, y)
		if x == "!" {
			continue
		}
		res.Files = append(res.Files, f)
	}
	res.Clean = len(res.Files) == 0
	return res
}

func classifyGitStatus(x, y string) string {
	switch {
	case x == "?" && y == "?":
		return "UNTRACKED"
	case x == "U" || y == "U" || (x == "A" && y == "A") || (x == "D" && y == "D"):
		return "CONFLICT"
	case x == "R" || y == "R" || x == "C" || y == "C":
		return "RENAMED"
	case x == "A" || y == "A":
		return "ADDED"
	case x == "D" || y == "D":
		return "DELETED"
	default:
		return "MODIFIED"
	}
}

// parseGitBranchLine handles "main...origin/main [ahead 1, behind 2]", "main", "HEAD (no branch)",
// "No commits yet on main" and "main...origin/main [gone]".
func parseGitBranchLine(line string, res *GitStatusResponse) {
	if strings.HasPrefix(line, "No commits yet on ") {
		res.Branch = strings.TrimPrefix(line, "No commits yet on ")
		return
	}
	if strings.HasPrefix(line, "HEAD (no branch)") {
		res.Branch = "HEAD"
		res.Detached = true
		return
	}
	if idx := strings.Index(line, " ["); idx >= 0 && strings.HasSuffix(line, "]") {
		for _, seg := range strings.Split(strings.TrimSuffix(line[idx+2:], "]"), ",") {
			seg = strings.TrimSpace(seg)
			switch {
			case strings.HasPrefix(seg, "ahead "):
				res.Ahead, _ = strconv.Atoi(strings.TrimPrefix(seg, "ahead "))
			case strings.HasPrefix(seg, "behind "):
				res.Behind, _ = strconv.Atoi(strings.TrimPrefix(seg, "behind "))
			}
		}
		line = line[:idx]
	}
	if i := strings.Index(line, "..."); i >= 0 {
		res.Branch = line[:i]
		res.Upstream = line[i+3:]
	} else {
		res.Branch = line
	}
}

func gitStatus(ctx context.Context, repoRoot string) (*GitStatusResponse, error) {
	out, err := runGit(ctx, repoRoot, gitStatusTimeout, "status", "--porcelain=v1", "-z", "--branch", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	res := parseGitStatus(out)
	res.RepoName = filepath.Base(repoRoot)
	return &res, nil
}

// validateGitPaths makes sure every requested path is a currently changed file.
func validateGitPaths(paths []string, status *GitStatusResponse) ([]string, error) {
	allowed := make(map[string]bool, len(status.Files)*2)
	for _, f := range status.Files {
		allowed[f.Path] = true
		if f.OrigPath != "" {
			allowed[f.OrigPath] = true
		}
	}
	out := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, raw := range paths {
		p := strings.TrimSpace(raw)
		if p == "" || strings.HasPrefix(p, "-") || filepath.IsAbs(p) || strings.Contains(p, "\x00") {
			return nil, fmt.Errorf("非法路径: %q", raw)
		}
		if !allowed[p] {
			return nil, fmt.Errorf("路径不在当前变更列表中: %q", raw)
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out, nil
}

// gitCommit stages (selected or all) changes and commits them.
func gitCommit(ctx context.Context, repoRoot, message string, paths []string) (*GitCommitResponse, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, errors.New("提交信息不能为空")
	}
	if len(message) > gitMaxMessageLen {
		return nil, errors.New("提交信息过长")
	}

	status, err := gitStatus(ctx, repoRoot)
	if err != nil {
		return nil, err
	}
	if status.Clean {
		return nil, errors.New("没有可提交的改动")
	}
	for _, f := range status.Files {
		if f.Status == "CONFLICT" {
			return nil, fmt.Errorf("存在未解决的冲突: %s", f.Path)
		}
	}

	selected, err := validateGitPaths(paths, status)
	if err != nil {
		return nil, err
	}

	// 重命名要连同源路径一起提交，否则会留下一个孤立的删除。
	// 源路径已不在工作区，git add 会报 pathspec 不匹配，改用 rm --cached 暂存其删除。
	var renameSources []string
	for _, f := range status.Files {
		if f.OrigPath == "" {
			continue
		}
		for _, sp := range selected {
			if sp == f.Path {
				renameSources = append(renameSources, f.OrigPath)
				break
			}
		}
	}

	var addOut string
	if len(selected) == 0 {
		addOut, err = runGit(ctx, repoRoot, gitCommitTimeout, "add", "-A")
	} else {
		addOut, err = runGit(ctx, repoRoot, gitCommitTimeout, append([]string{"add", "-A", "--"}, selected...)...)
	}
	if err != nil {
		return nil, err
	}
	if len(renameSources) > 0 {
		if _, err = runGit(ctx, repoRoot, gitCommitTimeout, append([]string{"rm", "--cached", "-q", "--ignore-unmatch", "--"}, renameSources...)...); err != nil {
			return nil, err
		}
		selected = append(selected, renameSources...)
	}

	commitArgs := []string{"commit", "-m", message}
	if len(selected) > 0 {
		// 只提交所选文件，忽略用户之前可能已暂存的其它文件
		commitArgs = append(commitArgs, "--")
		commitArgs = append(commitArgs, selected...)
	}
	out, err := runGit(ctx, repoRoot, gitCommitTimeout, commitArgs...)
	if err != nil {
		return nil, err
	}

	res := &GitCommitResponse{Committed: true, Output: strings.TrimSpace(addOut + "\n" + out)}
	if id, idErr := runGit(ctx, repoRoot, gitStatusTimeout, "rev-parse", "--short", "HEAD"); idErr == nil {
		res.CommitID = strings.TrimSpace(id)
	}
	return res, nil
}

func gitPush(ctx context.Context, repoRoot string) (string, error) {
	status, err := gitStatus(ctx, repoRoot)
	if err != nil {
		return "", err
	}
	if status.Detached {
		return "", errors.New("当前处于游离 HEAD，无法推送")
	}
	if status.Upstream == "" {
		// 首次推送：只设置 upstream 到同名分支，不接受自定义 remote/refspec
		return runGit(ctx, repoRoot, gitPushTimeout, "push", "--set-upstream", "origin", status.Branch)
	}
	return runGit(ctx, repoRoot, gitPushTimeout, "push")
}

// ---------------------------------------------------------------------------
// workspace resolution
// ---------------------------------------------------------------------------

// fileURIToLocalPath converts a file:// workspace URI into a local directory path.
func fileURIToLocalPath(raw string) (string, error) {
	norm := filepath.ToSlash(strings.TrimSpace(raw))
	u, err := url.Parse(norm)
	if err != nil || u.Scheme != "file" {
		return "", fmt.Errorf("不支持的工作区地址: %q", raw)
	}
	p := u.Path
	if runtime.GOOS == "windows" {
		if u.Host != "" && len(u.Host) == 2 && isWindowsDriveLetter(u.Host[0]) && u.Host[1] == ':' {
			p = u.Host + p
		} else if len(p) > 2 && p[0] == '/' && isWindowsDriveLetter(p[1]) && p[2] == ':' {
			p = p[1:]
		}
	}
	if p == "" {
		return "", fmt.Errorf("不支持的工作区地址: %q", raw)
	}
	return filepath.Clean(filepath.FromSlash(p)), nil
}

// localPathToFileURI converts a local directory path into a file:// URI.
func localPathToFileURI(path string) string {
	slash := filepath.ToSlash(filepath.Clean(path))
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	u := url.URL{Scheme: "file", Path: slash}
	return u.String()
}

type gitPRCacheEntry struct {
	target    *GitHubBranchTarget
	prs       []GitPullRequestItem
	fetchedAt time.Time
}

var (
	gitPRCacheMu sync.Mutex
	gitPRCache   = make(map[string]gitPRCacheEntry)
	gitPRTTL     = 30 * time.Second
)

func invalidateGitPRCache(repoRoot string) {
	gitPRCacheMu.Lock()
	defer gitPRCacheMu.Unlock()
	clean := filepath.Clean(repoRoot)
	for k := range gitPRCache {
		if strings.Contains(k, "|"+clean+"|") {
			delete(gitPRCache, k)
		}
	}
}

func normalizeGitHubPRState(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		s := strings.TrimSpace(strings.ToUpper(v))
		s = strings.TrimPrefix(s, "GITHUB_PULL_REQUEST_STATE_")
		switch s {
		case "OPEN", "DRAFT", "MERGED", "CLOSED":
			return s
		}
	case float64:
		switch int(v) {
		case 1:
			return "OPEN"
		case 2:
			return "DRAFT"
		case 3:
			return "MERGED"
		case 4:
			return "CLOSED"
		}
	}
	return "OPEN"
}

// parseGitHubRemoteURL extracts host, owner, and repository name from a git remote URL.
// Supports both SSH (git@github.com:owner/repo.git) and HTTPS (https://github.com/owner/repo.git).
func parseGitHubRemoteURL(raw string) (host, owner, repo string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", false
	}
	// SCP style: [user@]host:owner/repo[.git]
	if strings.Contains(raw, ":") && !strings.Contains(raw, "://") {
		parts := strings.SplitN(raw, ":", 2)
		hostPart := parts[0]
		if at := strings.LastIndex(hostPart, "@"); at >= 0 {
			hostPart = hostPart[at+1:]
		}
		pathPart := strings.TrimPrefix(parts[1], "/")
		pathPart = strings.TrimSuffix(pathPart, ".git")
		segs := strings.Split(pathPart, "/")
		if len(segs) == 2 && segs[0] != "" && segs[1] != "" {
			return hostPart, segs[0], segs[1], true
		}
	}
	// URL style: https://... or ssh://...
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		host := u.Hostname()
		path := strings.TrimPrefix(u.Path, "/")
		path = strings.TrimSuffix(path, ".git")
		segs := strings.Split(path, "/")
		if len(segs) == 2 && segs[0] != "" && segs[1] != "" {
			return host, segs[0], segs[1], true
		}
	}
	return "", "", "", false
}

// fetchGitHubPRsFromGH invokes local `gh pr list` CLI to retrieve PRs when available.
func fetchGitHubPRsFromGH(ctx context.Context, repoRoot string) []GitPullRequestItem {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "list", "--state", "all", "--limit", "20",
		"--json", "number,title,url,state,author,headRefName,baseRefName")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	var rawItems []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		URL    string `json:"url"`
		State  string `json:"state"`
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		HeadRefName string `json:"headRefName"`
		BaseRefName string `json:"baseRefName"`
	}
	if err := json.Unmarshal(out, &rawItems); err != nil {
		return nil
	}
	prs := make([]GitPullRequestItem, 0, len(rawItems))
	for _, it := range rawItems {
		prs = append(prs, GitPullRequestItem{
			Number:      it.Number,
			Title:       strings.TrimSpace(it.Title),
			URL:         strings.TrimSpace(it.URL),
			State:       normalizeGitHubPRState(it.State),
			AuthorLogin: strings.TrimSpace(it.Author.Login),
			HeadRefName: strings.TrimSpace(it.HeadRefName),
			BaseRefName: strings.TrimSpace(it.BaseRefName),
		})
	}
	return prs
}

// fetchWorkspaceGitHubPullRequests queries Antigravity GetGitHubPullRequests
// or local `gh pr list` CLI for the workspace's GitHub branch target and associated pull requests.
// Best-effort with a 2s timeout and 30s per-(port, repoRoot, branch) cache so git status is never blocked.
func (p *Proxy) fetchWorkspaceGitHubPullRequests(parent context.Context, repoRoot, branch string, port int, token string) (*GitHubBranchTarget, []GitPullRequestItem) {
	if port <= 0 || repoRoot == "" {
		return nil, nil
	}
	cacheKey := fmt.Sprintf("%d|%s|%s", port, filepath.Clean(repoRoot), branch)
	gitPRCacheMu.Lock()
	if entry, ok := gitPRCache[cacheKey]; ok && time.Since(entry.fetchedAt) < gitPRTTL {
		gitPRCacheMu.Unlock()
		return entry.target, entry.prs
	}
	gitPRCacheMu.Unlock()

	var target *GitHubBranchTarget
	remoteURL, _ := runGit(parent, repoRoot, gitStatusTimeout, "remote", "get-url", "origin")
	if host, owner, repo, ok := parseGitHubRemoteURL(remoteURL); ok {
		target = &GitHubBranchTarget{
			Host:   host,
			Owner:  owner,
			Repo:   repo,
			Branch: branch,
		}
	}

	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()

	payload, err := json.Marshal(map[string]interface{}{
		"workspaceUris": []string{localPathToFileURI(repoRoot)},
	})
	if err != nil {
		return target, nil
	}

	rpcURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetGitHubPullRequests", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewReader(payload))
	if err != nil {
		return target, nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	client := p.shortClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)

	var prs []GitPullRequestItem
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
			var parsed struct {
				Statuses []struct {
					Ref struct {
						Host   string `json:"host"`
						Owner  string `json:"owner"`
						Repo   string `json:"repo"`
						Number int    `json:"number"`
					} `json:"ref"`
					Number      int         `json:"number"`
					URL         string      `json:"url"`
					Title       string      `json:"title"`
					State       interface{} `json:"state"`
					AuthorLogin string      `json:"authorLogin"`
					HeadRefName string      `json:"headRefName"`
					BaseRefName string      `json:"baseRefName"`
				} `json:"statuses"`
				Workspaces []struct {
					Target *struct {
						Host   string `json:"host"`
						Owner  string `json:"owner"`
						Repo   string `json:"repo"`
						Branch string `json:"branch"`
					} `json:"target"`
					PullRequests []struct {
						Ref struct {
							Number int `json:"number"`
						} `json:"ref"`
						Number      int         `json:"number"`
						URL         string      `json:"url"`
						Title       string      `json:"title"`
						State       interface{} `json:"state"`
						AuthorLogin string      `json:"authorLogin"`
						HeadRefName string      `json:"headRefName"`
						BaseRefName string      `json:"baseRefName"`
					} `json:"pullRequests"`
				} `json:"workspaces"`
			}
			if json.Unmarshal(body, &parsed) == nil {
				if len(parsed.Statuses) > 0 {
					for _, s := range parsed.Statuses {
						num := s.Ref.Number
						if num == 0 {
							num = s.Number
						}
						if target == nil && (s.Ref.Owner != "" || s.Ref.Repo != "") {
							target = &GitHubBranchTarget{
								Host:   s.Ref.Host,
								Owner:  s.Ref.Owner,
								Repo:   s.Ref.Repo,
								Branch: branch,
							}
						}
						prs = append(prs, GitPullRequestItem{
							Number:      num,
							Title:       strings.TrimSpace(s.Title),
							URL:         strings.TrimSpace(s.URL),
							State:       normalizeGitHubPRState(s.State),
							AuthorLogin: strings.TrimSpace(s.AuthorLogin),
							HeadRefName: strings.TrimSpace(s.HeadRefName),
							BaseRefName: strings.TrimSpace(s.BaseRefName),
						})
					}
				} else if len(parsed.Workspaces) > 0 {
					ws := parsed.Workspaces[0]
					if target == nil && ws.Target != nil && (ws.Target.Owner != "" || ws.Target.Repo != "") {
						target = &GitHubBranchTarget{
							Host:   ws.Target.Host,
							Owner:  ws.Target.Owner,
							Repo:   ws.Target.Repo,
							Branch: ws.Target.Branch,
						}
					}
					for _, pr := range ws.PullRequests {
						num := pr.Ref.Number
						if num == 0 {
							num = pr.Number
						}
						if num <= 0 && pr.URL == "" && pr.Title == "" {
							continue
						}
						prs = append(prs, GitPullRequestItem{
							Number:      num,
							Title:       strings.TrimSpace(pr.Title),
							URL:         strings.TrimSpace(pr.URL),
							State:       normalizeGitHubPRState(pr.State),
							AuthorLogin: strings.TrimSpace(pr.AuthorLogin),
							HeadRefName: strings.TrimSpace(pr.HeadRefName),
							BaseRefName: strings.TrimSpace(pr.BaseRefName),
						})
					}
				}
			}
		}
	}

	// 降级：若上游 RPC 未返回 PR 列表，尝试调用本机 gh CLI 补齐
	if len(prs) == 0 {
		if cliPRs := fetchGitHubPRsFromGH(parent, repoRoot); len(cliPRs) > 0 {
			prs = cliPRs
		}
	}

	gitPRCacheMu.Lock()
	if len(gitPRCache) >= 256 {
		now := time.Now()
		for k, v := range gitPRCache {
			if now.Sub(v.fetchedAt) >= gitPRTTL {
				delete(gitPRCache, k)
			}
		}
		if len(gitPRCache) >= 256 {
			for k := range gitPRCache {
				delete(gitPRCache, k)
				break
			}
		}
	}
	gitPRCache[cacheKey] = gitPRCacheEntry{
		target:    target,
		prs:       prs,
		fetchedAt: time.Now(),
	}
	gitPRCacheMu.Unlock()

	return target, prs
}

// workspaceURIsFromSummaries extracts workspace folder URIs of one cascade from a GetAllCascadeTrajectories body.
func workspaceURIsFromSummaries(body []byte, cascadeID string) []string {
	var parsed struct {
		Summaries map[string]struct {
			Workspaces []struct {
				URI string `json:"workspaceFolderAbsoluteUri"`
			} `json:"workspaces"`
		} `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	var uris []string
	for _, w := range parsed.Summaries[cascadeID].Workspaces {
		if w.URI != "" {
			uris = append(uris, w.URI)
		}
	}
	return uris
}

// resolveCascadeRepo finds the git repository that belongs to a cascade's workspace.
func (p *Proxy) resolveCascadeRepo(ctx context.Context, cascadeID string, port int, token string) (string, error) {
	body, err := p.fetchAllTrajectoriesRaw(ctx, port, token)
	if err != nil {
		return "", fmt.Errorf("查询会话工作区失败: %w", err)
	}

	uris := workspaceURIsFromSummaries(body, cascadeID)
	if len(uris) == 0 {
		return "", errors.New("该会话没有关联的工作区（纯对话）")
	}
	for _, raw := range uris {
		dir, err := fileURIToLocalPath(raw)
		if err != nil {
			continue
		}
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			continue
		}
		if root, err := gitRepoRoot(ctx, dir); err == nil {
			return root, nil
		}
	}
	return "", errNotGitRepo
}

// ---------------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------------

func (p *Proxy) decodeGitCascadeRequest(w http.ResponseWriter, r *http.Request, v interface{}) (string, int, string, bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return "", 0, "", false
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(v); err != nil {
		writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return "", 0, "", false
	}
	var cascadeID string
	switch req := v.(type) {
	case *gitCascadeRequest:
		cascadeID = req.CascadeID
	case *GitCommitRequest:
		cascadeID = req.CascadeID
	}
	cascadeID = strings.TrimSpace(cascadeID)
	if cascadeID == "" || !cascadeIDRe.MatchString(cascadeID) {
		writeJSONError(w, "cascadeId is required", http.StatusBadRequest)
		return "", 0, "", false
	}
	port, token := p.ActiveUpstream()
	if port == 0 {
		writeJSONError(w, "No active Antigravity upstream", http.StatusServiceUnavailable)
		return "", 0, "", false
	}
	return cascadeID, port, token, true
}

func writeGitJSON(w http.ResponseWriter, v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func gitErrorStatus(err error) int {
	if errors.Is(err, errNotGitRepo) {
		return http.StatusUnprocessableEntity
	}
	return http.StatusInternalServerError
}

// HandleGitStatus handles POST /gateway/git/status.
func (p *Proxy) HandleGitStatus(w http.ResponseWriter, r *http.Request) {
	var req gitCascadeRequest
	cascadeID, port, token, ok := p.decodeGitCascadeRequest(w, r, &req)
	if !ok {
		return
	}
	repo, err := p.resolveCascadeRepo(r.Context(), cascadeID, port, token)
	if err != nil {
		writeJSONError(w, err.Error(), gitErrorStatus(err))
		return
	}
	forceRefresh := r.URL.Query().Get("refresh") == "1"
	st, err := gitStatusCached(r.Context(), repo, forceRefresh)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if target, prs := p.fetchWorkspaceGitHubPullRequests(r.Context(), repo, st.Branch, port, token); target != nil || len(prs) > 0 {
		st.GitHubTarget = target
		st.PullRequests = prs
	}
	writeGitJSON(w, st)
}

// HandleGitCommit handles POST /gateway/git/commit (optionally followed by a push).
func (p *Proxy) HandleGitCommit(w http.ResponseWriter, r *http.Request) {
	var req GitCommitRequest
	cascadeID, port, token, ok := p.decodeGitCascadeRequest(w, r, &req)
	if !ok {
		return
	}
	repo, err := p.resolveCascadeRepo(r.Context(), cascadeID, port, token)
	if err != nil {
		writeJSONError(w, err.Error(), gitErrorStatus(err))
		return
	}
	res, err := gitCommit(r.Context(), repo, req.Message, req.Paths)
	if err != nil {
		slog.Warn(fmt.Sprintf("[Git] commit failed for cascade %s", shortCascadeID(cascadeID)), "err", err)
		writeJSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	invalidateGitStatusCache(repo)
	slog.Info(fmt.Sprintf("[Git] committed %s for cascade %s", res.CommitID, shortCascadeID(cascadeID)))
	if req.Push {
		out, pushErr := gitPush(r.Context(), repo)
		if pushErr != nil {
			res.PushError = pushErr.Error()
		} else {
			res.Pushed = true
			res.Output = strings.TrimSpace(res.Output + "\n" + out)
			invalidateGitStatusCache(repo)
			invalidateGitPRCache(repo)
		}
	}
	writeGitJSON(w, res)
}

// HandleGitPush handles POST /gateway/git/push.
func (p *Proxy) HandleGitPush(w http.ResponseWriter, r *http.Request) {
	var req gitCascadeRequest
	cascadeID, port, token, ok := p.decodeGitCascadeRequest(w, r, &req)
	if !ok {
		return
	}
	repo, err := p.resolveCascadeRepo(r.Context(), cascadeID, port, token)
	if err != nil {
		writeJSONError(w, err.Error(), gitErrorStatus(err))
		return
	}
	out, err := gitPush(r.Context(), repo)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	invalidateGitStatusCache(repo)
	invalidateGitPRCache(repo)
	writeGitJSON(w, GitCommitResponse{Pushed: true, Output: out})
}

type gitPRRefCacheEntry struct {
	item      GitPullRequestItem
	fetchedAt time.Time
}

var (
	gitPRRefCacheMu   sync.Mutex
	gitPRRefCache     = make(map[string]gitPRRefCacheEntry)
	gitPRWarmupMu     sync.Mutex
	gitPRWarmupLastAt = make(map[string]time.Time)
)

// ParseGitHubPRURL extracts host, owner, repo, and PR number from a GitHub pull request URL.
func ParseGitHubPRURL(raw string) (GitHubPullRequestRef, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return GitHubPullRequestRef{}, false
	}
	host := strings.ToLower(u.Hostname())
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) < 4 || segs[2] != "pull" {
		return GitHubPullRequestRef{}, false
	}
	num, err := strconv.Atoi(segs[3])
	if err != nil || num <= 0 || segs[0] == "" || segs[1] == "" {
		return GitHubPullRequestRef{}, false
	}
	return GitHubPullRequestRef{
		Host:   host,
		Owner:  segs[0],
		Repo:   segs[1],
		Number: num,
	}, true
}

func prRefCacheKey(ref GitHubPullRequestRef) string {
	h := strings.ToLower(strings.TrimSpace(ref.Host))
	if h == "" {
		h = "github.com"
	}
	return fmt.Sprintf("%s/%s/%s#%d", h, strings.ToLower(strings.TrimSpace(ref.Owner)), strings.ToLower(strings.TrimSpace(ref.Repo)), ref.Number)
}

func truncatePRBody(raw string, maxRunes int) string {
	s := strings.TrimSpace(raw)
	if s == "" || maxRunes <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "..."
}

func lookupGitHubPRViaGH(ctx context.Context, ref GitHubPullRequestRef) (GitPullRequestItem, bool) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	repoSlug := ref.Owner + "/" + ref.Repo
	if ref.Host != "" && !strings.EqualFold(ref.Host, "github.com") {
		repoSlug = ref.Host + "/" + repoSlug
	}
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", strconv.Itoa(ref.Number),
		"--repo", repoSlug,
		"--json", "number,title,url,state,isDraft,author,headRefName,baseRefName,body")
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return GitPullRequestItem{}, false
	}
	var parsed struct {
		Number      int    `json:"number"`
		Title       string `json:"title"`
		URL         string `json:"url"`
		State       string `json:"state"`
		IsDraft     bool   `json:"isDraft"`
		HeadRefName string `json:"headRefName"`
		BaseRefName string `json:"baseRefName"`
		Body        string `json:"body"`
		Author      struct {
			Login string `json:"login"`
		} `json:"author"`
	}
	if json.Unmarshal(out, &parsed) != nil || parsed.Number <= 0 {
		return GitPullRequestItem{}, false
	}
	state := normalizeGitHubPRState(parsed.State)
	if state == "OPEN" && parsed.IsDraft {
		state = "DRAFT"
	}
	return GitPullRequestItem{
		Host:        firstNonEmpty(ref.Host, "github.com"),
		Owner:       ref.Owner,
		Repo:        ref.Repo,
		Number:      parsed.Number,
		Title:       strings.TrimSpace(parsed.Title),
		URL:         strings.TrimSpace(parsed.URL),
		State:       state,
		AuthorLogin: strings.TrimSpace(parsed.Author.Login),
		HeadRefName: strings.TrimSpace(parsed.HeadRefName),
		BaseRefName: strings.TrimSpace(parsed.BaseRefName),
		Body:        truncatePRBody(parsed.Body, 400),
	}, true
}

// HandleGitHubPullRequests handles POST /gateway/github/pull-requests to resolve live PR statuses.
func (p *Proxy) HandleGitHubPullRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Refs []GitHubPullRequestRef `json:"refs"`
		URLs []string               `json:"urls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var refs []GitHubPullRequestRef
	seen := make(map[string]bool)
	addRef := func(ref GitHubPullRequestRef) {
		if ref.Owner == "" || ref.Repo == "" || ref.Number <= 0 {
			return
		}
		if ref.Host == "" {
			ref.Host = "github.com"
		}
		key := prRefCacheKey(ref)
		if seen[key] {
			return
		}
		seen[key] = true
		refs = append(refs, ref)
	}
	for _, ref := range req.Refs {
		addRef(ref)
	}
	for _, u := range req.URLs {
		if ref, ok := ParseGitHubPRURL(u); ok {
			addRef(ref)
		}
	}
	if len(refs) == 0 {
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"pullRequests": []GitPullRequestItem{}})
		return
	}
	if len(refs) > 25 {
		refs = refs[:25]
	}

	resultsByKey := make(map[string]GitPullRequestItem, len(refs))
	var missing []GitHubPullRequestRef
	gitPRRefCacheMu.Lock()
	now := time.Now()
	for _, ref := range refs {
		key := prRefCacheKey(ref)
		if entry, ok := gitPRRefCache[key]; ok && now.Sub(entry.fetchedAt) < gitPRTTL {
			resultsByKey[key] = entry.item
		} else {
			missing = append(missing, ref)
		}
	}
	gitPRRefCacheMu.Unlock()

	if len(missing) > 0 {
		port, token := p.ActiveUpstream()
		if port > 0 {
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			rpcBody, _ := json.Marshal(map[string]interface{}{"pullRequests": missing})
			rpcURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetGitHubPullRequests", port)
			if httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewReader(rpcBody)); err == nil {
				httpReq.Header.Set("Content-Type", "application/json")
				httpReq.Header.Set("Connect-Protocol-Version", "1")
				if token != "" {
					httpReq.Header.Set("x-codeium-csrf-token", token)
				}
				client := p.shortClient
				if client == nil {
					client = p.upstreamClient()
				}
				if client != nil {
					if resp, err := client.Do(httpReq); err == nil {
						body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
						resp.Body.Close()
						if resp.StatusCode == http.StatusOK {
							var parsed struct {
								Statuses []struct {
									Ref struct {
										Host   string `json:"host"`
										Owner  string `json:"owner"`
										Repo   string `json:"repo"`
										Number int    `json:"number"`
									} `json:"ref"`
									URL         string      `json:"url"`
									Title       string      `json:"title"`
									State       interface{} `json:"state"`
									AuthorLogin string      `json:"authorLogin"`
									HeadRefName string      `json:"headRefName"`
									BaseRefName string      `json:"baseRefName"`
									Body        string      `json:"body"`
									Error       *struct {
										Message string `json:"message"`
									} `json:"error"`
								} `json:"statuses"`
							}
							if json.Unmarshal(body, &parsed) == nil {
								for _, s := range parsed.Statuses {
									ref := GitHubPullRequestRef{
										Host:   firstNonEmpty(s.Ref.Host, "github.com"),
										Owner:  s.Ref.Owner,
										Repo:   s.Ref.Repo,
										Number: s.Ref.Number,
									}
									if ref.Owner == "" && s.URL != "" {
										if uRef, ok := ParseGitHubPRURL(s.URL); ok {
											ref = uRef
										}
									}
									if ref.Owner == "" || ref.Number <= 0 {
										continue
									}
									errMsg := ""
									if s.Error != nil {
										errMsg = strings.TrimSpace(s.Error.Message)
									}
									prURL := strings.TrimSpace(s.URL)
									if prURL == "" {
										prURL = fmt.Sprintf("https://%s/%s/%s/pull/%d", ref.Host, ref.Owner, ref.Repo, ref.Number)
									}
									item := GitPullRequestItem{
										Host:        ref.Host,
										Owner:       ref.Owner,
										Repo:        ref.Repo,
										Number:      ref.Number,
										Title:       strings.TrimSpace(s.Title),
										URL:         prURL,
										State:       normalizeGitHubPRState(s.State),
										AuthorLogin: strings.TrimSpace(s.AuthorLogin),
										HeadRefName: strings.TrimSpace(s.HeadRefName),
										BaseRefName: strings.TrimSpace(s.BaseRefName),
										Body:        truncatePRBody(s.Body, 400),
										Error:       errMsg,
									}
									resultsByKey[prRefCacheKey(ref)] = item
								}
							}
						}
					}
				}
			}
		}

		// Best-effort `gh pr view` fallback for unresolved items when gh is available on host.
		if os.Getenv("MGY_DISABLE_GH_CLI") != "1" {
			if _, err := exec.LookPath("gh"); err == nil {
				for _, ref := range missing {
					key := prRefCacheKey(ref)
					existing, has := resultsByKey[key]
					if !has || (existing.Title == "" && existing.Error == "") {
						if ghItem, ok := lookupGitHubPRViaGH(r.Context(), ref); ok {
							resultsByKey[key] = ghItem
						}
					}
				}
			}
		}

		gitPRRefCacheMu.Lock()
		if len(gitPRRefCache) >= 256 {
			for k, v := range gitPRRefCache {
				if time.Since(v.fetchedAt) >= gitPRTTL {
					delete(gitPRRefCache, k)
				}
			}
		}
		storeAt := time.Now()
		for _, ref := range missing {
			key := prRefCacheKey(ref)
			if item, ok := resultsByKey[key]; ok {
				gitPRRefCache[key] = gitPRRefCacheEntry{item: item, fetchedAt: storeAt}
			}
		}
		gitPRRefCacheMu.Unlock()
	}

	out := make([]GitPullRequestItem, 0, len(refs))
	for _, ref := range refs {
		if item, ok := resultsByKey[prRefCacheKey(ref)]; ok {
			out = append(out, item)
		}
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{"pullRequests": out})
}

// findGitRepoRootFast walks up at most 4 parent directories looking for .git without spawning git.
func findGitRepoRootFast(dir string) (string, string) {
	cur := filepath.Clean(dir)
	for i := 0; i < 4 && cur != "" && cur != string(filepath.Separator); i++ {
		dotGit := filepath.Join(cur, ".git")
		fi, err := os.Stat(dotGit)
		if err == nil {
			gitDir := dotGit
			if !fi.IsDir() {
				if raw, err := os.ReadFile(dotGit); err == nil {
					line := strings.TrimSpace(string(raw))
					if strings.HasPrefix(line, "gitdir:") {
						target := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
						if !filepath.IsAbs(target) {
							target = filepath.Join(cur, target)
						}
						gitDir = filepath.Clean(target)
					}
				}
			}
			headBytes, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
			if err != nil {
				return cur, ""
			}
			head := strings.TrimSpace(string(headBytes))
			branch := ""
			if strings.HasPrefix(head, "ref: refs/heads/") {
				branch = strings.TrimPrefix(head, "ref: refs/heads/")
			}
			return cur, branch
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return "", ""
}

func selectPrimaryPRForBranch(prs []GitPullRequestItem, branch string) *GitPullRequestItem {
	if len(prs) == 0 {
		return nil
	}
	var matching []GitPullRequestItem
	for _, pr := range prs {
		if branch != "" && pr.HeadRefName == branch {
			matching = append(matching, pr)
		}
	}
	pool := matching
	if len(pool) == 0 {
		return nil
	}
	for _, wantState := range []string{"OPEN", "DRAFT", "MERGED", "CLOSED"} {
		for i := range pool {
			if strings.EqualFold(pool[i].State, wantState) {
				cp := pool[i]
				return &cp
			}
		}
	}
	cp := pool[0]
	return &cp
}

// cachedWorkspacePullRequest reads cached branch PR info synchronously (sub-millisecond) and returns
// whether a non-blocking background warmup should be scheduled.
func cachedWorkspacePullRequest(wsURIs []string, port int) (branch string, pr *GitPullRequestItem, repoRoot string, needsWarmup bool) {
	if port <= 0 || len(wsURIs) == 0 {
		return "", nil, "", false
	}
	for _, raw := range wsURIs {
		dir, err := fileURIToLocalPath(raw)
		if err != nil || dir == "" {
			continue
		}
		root, br := findGitRepoRootFast(dir)
		if root == "" {
			continue
		}
		cacheKey := fmt.Sprintf("%d|%s|%s", port, filepath.Clean(root), br)
		gitPRCacheMu.Lock()
		entry, ok := gitPRCache[cacheKey]
		fresh := ok && time.Since(entry.fetchedAt) < gitPRTTL
		gitPRCacheMu.Unlock()
		if fresh {
			return br, selectPrimaryPRForBranch(entry.prs, br), root, false
		}
		return br, nil, root, true
	}
	return "", nil, "", false
}

func (p *Proxy) scheduleWorkspacePRWarmup(repoRoot, branch string, port int, token string) {
	if p == nil || port <= 0 || repoRoot == "" {
		return
	}
	key := fmt.Sprintf("%d|%s|%s", port, filepath.Clean(repoRoot), branch)
	gitPRWarmupMu.Lock()
	if last, ok := gitPRWarmupLastAt[key]; ok && time.Since(last) < 60*time.Second {
		gitPRWarmupMu.Unlock()
		return
	}
	gitPRWarmupLastAt[key] = time.Now()
	gitPRWarmupMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, prs := p.fetchWorkspaceGitHubPullRequests(ctx, repoRoot, branch, port, token)
		if len(prs) > 0 {
			p.invalidateAllTrajectories()
			p.events.requestCheck()
		}
	}()
}
