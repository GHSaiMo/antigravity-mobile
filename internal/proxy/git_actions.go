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
	gitStatusTimeout = 15 * time.Second
	gitCommitTimeout = 60 * time.Second
	gitPushTimeout   = 120 * time.Second
	gitMaxOutput     = 64 * 1024
	gitMaxMessageLen = 8000
)

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

// GitStatusResponse is the response of POST /gateway/git/status.
type GitStatusResponse struct {
	RepoName string          `json:"repoName"`
	Branch   string          `json:"branch"`
	Upstream string          `json:"upstream,omitempty"`
	Ahead    int             `json:"ahead"`
	Behind   int             `json:"behind"`
	Detached bool            `json:"detached"`
	Files    []GitFileStatus `json:"files"`
	Clean    bool            `json:"clean"`
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
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "file" || u.Path == "" {
		return "", fmt.Errorf("不支持的工作区地址: %q", raw)
	}
	p := u.Path
	if runtime.GOOS == "windows" && len(p) > 2 && p[0] == '/' && isWindowsDriveLetter(p[1]) && p[2] == ':' {
		p = p[1:]
	}
	return filepath.Clean(filepath.FromSlash(p)), nil
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
	apiURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader("{}"))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}
	resp, err := p.mediumClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("查询会话工作区失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("查询会话工作区失败: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return "", err
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
	st, err := gitStatus(r.Context(), repo)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
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
	slog.Info(fmt.Sprintf("[Git] committed %s for cascade %s", res.CommitID, shortCascadeID(cascadeID)))
	if req.Push {
		out, pushErr := gitPush(r.Context(), repo)
		if pushErr != nil {
			res.PushError = pushErr.Error()
		} else {
			res.Pushed = true
			res.Output = strings.TrimSpace(res.Output + "\n" + out)
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
	writeGitJSON(w, GitCommitResponse{Pushed: true, Output: out})
}
