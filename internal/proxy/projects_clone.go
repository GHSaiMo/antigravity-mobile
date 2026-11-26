package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/procutil"
)

const (
	cloneJobTimeout         = 5 * time.Minute
	cloneJobRetention       = 30 * time.Minute
	maxListedLocalDirEntry  = 300
	maxRecommendedLocalDirs = 6
)

// LocalDirQuickItem describes a recommended parent folder (e.g. existing project root or home dir).
type LocalDirQuickItem struct {
	Path         string `json:"path"`
	Name         string `json:"name"`
	DisplayPath  string `json:"displayPath"`
	ProjectCount int    `json:"projectCount"`
	IsDefault    bool   `json:"isDefault"`
	Exists       bool   `json:"exists"`
}

// LocalDirEntry describes a subdirectory inside the currently browsed folder.
type LocalDirEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	IsGitRepo bool   `json:"isGitRepo"`
	IsProject bool   `json:"isProject"`
	IsHidden  bool   `json:"isHidden"`
}

// LocalDirsResponse is returned by GET /gateway/fs/dirs.
type LocalDirsResponse struct {
	OK              bool                `json:"ok"`
	Platform        string              `json:"platform"`
	HomePath        string              `json:"homePath"`
	DefaultDir      string              `json:"defaultDir"`
	CurrentPath     string              `json:"currentPath"`
	CurrentDisplay  string              `json:"currentDisplay"`
	ParentPath      string              `json:"parentPath,omitempty"`
	CanGoUp         bool                `json:"canGoUp"`
	RecommendedDirs []LocalDirQuickItem `json:"recommendedDirs"`
	Entries         []LocalDirEntry     `json:"entries"`
	Truncated       bool                `json:"truncated,omitempty"`
	Error           string              `json:"error,omitempty"`
}

// CloneJobStatusResponse is returned by POST /gateway/projects/clone (async) and GET /gateway/projects/clone/status.
type CloneJobStatusResponse struct {
	OK            bool         `json:"ok"`
	Async         bool         `json:"async,omitempty"`
	JobID         string       `json:"jobId,omitempty"`
	RepoSlug      string       `json:"repoSlug,omitempty"`
	Status        string       `json:"status"` // "cloning" | "registering" | "done" | "failed" | "cancelled"
	Phase         string       `json:"phase,omitempty"`
	Progress      int          `json:"progress"`
	Message       string       `json:"message,omitempty"`
	TargetPath    string       `json:"targetPath,omitempty"`
	FolderURI     string       `json:"folderUri,omitempty"`
	Path          string       `json:"path,omitempty"`
	Project       *ProjectItem `json:"project,omitempty"`
	AlreadyExists bool         `json:"alreadyExists,omitempty"`
	CascadeID     string       `json:"cascadeId,omitempty"`
	Code          string       `json:"code,omitempty"`
	ExistingPath  string       `json:"existingPath,omitempty"`
	Error         string       `json:"error,omitempty"`
}

type cloneRepoJob struct {
	mu           sync.RWMutex
	id           string
	repoSlug     string
	cloneURL     string
	parentDir    string
	folderName   string
	targetDir    string
	preExisted   bool
	startSession bool

	status        string
	phase         string
	progress      int
	message       string
	folderURI     string
	path          string
	project       *ProjectItem
	alreadyExists bool
	cascadeID     string
	errMsg        string

	createdAt  time.Time
	finishedAt time.Time
	cancel     context.CancelFunc
	done       chan struct{}
}

func (j *cloneRepoJob) snapshot(asyncFlag bool) CloneJobStatusResponse {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var projCopy *ProjectItem
	if j.project != nil {
		cp := *j.project
		projCopy = &cp
	}
	ok := j.status != "failed" && j.status != "cancelled"
	return CloneJobStatusResponse{
		OK:            ok,
		Async:         asyncFlag,
		JobID:         j.id,
		RepoSlug:      j.repoSlug,
		Status:        j.status,
		Phase:         j.phase,
		Progress:      j.progress,
		Message:       j.message,
		TargetPath:    j.targetDir,
		FolderURI:     j.folderURI,
		Path:          j.path,
		Project:       projCopy,
		AlreadyExists: j.alreadyExists,
		CascadeID:     j.cascadeID,
		Error:         j.errMsg,
	}
}

func (j *cloneRepoJob) updateProgress(phase string, progress int, message string) {
	j.mu.Lock()
	if j.status == "cancelled" {
		j.mu.Unlock()
		return
	}
	j.status = phase
	j.phase = phase
	if progress >= 0 && progress <= 100 {
		j.progress = progress
	}
	if message != "" {
		j.message = message
	}
	j.mu.Unlock()
}

func (j *cloneRepoJob) markFailed(errMsg string) {
	j.mu.Lock()
	if j.status != "cancelled" {
		j.status = "failed"
		j.phase = "failed"
		j.errMsg = errMsg
		j.message = errMsg
		j.finishedAt = time.Now()
	}
	j.mu.Unlock()
}

func (j *cloneRepoJob) markCancelled() {
	j.mu.Lock()
	if j.status != "done" && j.status != "failed" {
		j.status = "cancelled"
		j.phase = "cancelled"
		j.errMsg = "已取消克隆"
		j.message = "已取消克隆"
		j.finishedAt = time.Now()
	}
	j.mu.Unlock()
}

func (j *cloneRepoJob) markDone(prj ProjectItem, alreadyExists bool, cascadeID string) {
	j.mu.Lock()
	if j.status == "cancelled" {
		j.mu.Unlock()
		return
	}
	cp := prj
	j.status = "done"
	j.phase = "done"
	j.progress = 100
	if alreadyExists {
		j.message = "仓库已在 Antigravity 项目列表中"
	} else {
		j.message = "克隆完成，已加入 Antigravity 项目列表"
	}
	j.project = &cp
	j.alreadyExists = alreadyExists
	j.folderURI = prj.URI
	j.path = prj.Path
	j.cascadeID = cascadeID
	j.finishedAt = time.Now()
	j.mu.Unlock()
}

func (p *Proxy) cancelAllCloneJobs() {
	p.cloneJobsMu.Lock()
	defer p.cloneJobsMu.Unlock()
	for _, job := range p.cloneJobs {
		if job != nil && job.cancel != nil {
			job.cancel()
		}
	}
}

func (p *Proxy) pruneExpiredCloneJobsLocked(now time.Time) {
	if p.cloneJobs == nil {
		return
	}
	for id, job := range p.cloneJobs {
		job.mu.RLock()
		finished := job.finishedAt
		job.mu.RUnlock()
		if !finished.IsZero() && now.Sub(finished) > cloneJobRetention {
			delete(p.cloneJobs, id)
		}
	}
}

func formatDisplayPath(fullPath, home string) string {
	clean := filepath.Clean(fullPath)
	if home == "" {
		return clean
	}
	cleanHome := filepath.Clean(home)
	if clean == cleanHome {
		return "~"
	}
	sep := string(filepath.Separator)
	if strings.HasPrefix(clean, cleanHome+sep) {
		return "~" + clean[len(cleanHome):]
	}
	return clean
}

func validateSingleDirName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", fmt.Errorf("请输入文件夹名称")
	}
	if len(name) > 120 {
		return "", fmt.Errorf("文件夹名称过长（最多 120 个字符）")
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("非法的文件夹名称")
	}
	if strings.HasPrefix(name, "-") {
		return "", fmt.Errorf("文件夹名称不能以 '-' 开头")
	}
	if strings.ContainsAny(name, `/\:`+"\x00\r\n") {
		return "", fmt.Errorf("文件夹名称不能包含路径分隔符或特殊字符")
	}
	return name, nil
}

// resolveRecommendedDirs builds the sorted quick-access directory list from existing Antigravity
// projects' parent directories, upstream GetDefaultProjectDir, ~/Projects, and user home.
func (p *Proxy) resolveRecommendedDirs(ctx context.Context) (home string, defaultDir string, quickDirs []LocalDirQuickItem, projectPathSet map[string]bool) {
	home, _ = os.UserHomeDir()
	home = filepath.Clean(home)
	projectPathSet = make(map[string]bool)

	type dirAgg struct {
		path      string
		count     int
		isDefault bool
		exists    bool
	}
	aggMap := make(map[string]*dirAgg)
	addCandidate := func(raw string, countDelta int, markDefault bool) {
		if strings.TrimSpace(raw) == "" {
			return
		}
		clean := filepath.Clean(raw)
		if isDriveRoot(clean) {
			return
		}
		resolved := evalSymlinksExistingPrefix(clean)
		if !IsSafeFilePath(resolved) {
			return
		}
		fi, err := os.Stat(resolved)
		exists := err == nil && fi.IsDir()
		if err == nil && !fi.IsDir() {
			return
		}
		if existing, ok := aggMap[resolved]; ok {
			existing.count += countDelta
			if markDefault {
				existing.isDefault = true
			}
			if exists {
				existing.exists = true
			}
			return
		}
		aggMap[resolved] = &dirAgg{
			path:      resolved,
			count:     countDelta,
			isDefault: markDefault,
			exists:    exists,
		}
	}

	if projects, err := p.GetProjects(); err == nil {
		for _, prj := range projects {
			if prj.Path == "" || isRemoteURI(prj.URI) {
				continue
			}
			cleanProj := filepath.Clean(prj.Path)
			resolvedProj := evalSymlinksExistingPrefix(cleanProj)
			projectPathSet[cleanProj] = true
			projectPathSet[resolvedProj] = true

			parent := filepath.Dir(resolvedProj)
			if parent != "" && parent != resolvedProj && !isDriveRoot(parent) {
				addCandidate(parent, 1, false)
			}
		}
	}

	// Best-effort query of upstream GetDefaultProjectDir
	if port, _ := p.ActiveUpstream(); port > 0 {
		dCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		body, status, err := p.callUpstreamRPC(dCtx, "GetDefaultProjectDir", map[string]interface{}{})
		cancel()
		if err == nil && status == http.StatusOK {
			var resp struct {
				DefaultProjectDirURI string `json:"defaultProjectDirUri"`
			}
			if json.Unmarshal(body, &resp) == nil && resp.DefaultProjectDirURI != "" {
				if pth := uriToPath(resp.DefaultProjectDirURI); pth != "" {
					addCandidate(pth, 0, true)
				}
			}
		}
	}

	if home != "" {
		projectsDir := filepath.Join(home, "Projects")
		if fi, err := os.Stat(projectsDir); err == nil && fi.IsDir() {
			addCandidate(projectsDir, 0, false)
		}
		addCandidate(home, 0, false)
	}

	list := make([]*dirAgg, 0, len(aggMap))
	for _, item := range aggMap {
		list = append(list, item)
	}
	homeResolved := ""
	if home != "" {
		homeResolved = evalSymlinksExistingPrefix(home)
	}
	sort.Slice(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.count != b.count {
			return a.count > b.count
		}
		if a.isDefault != b.isDefault {
			return a.isDefault
		}
		if a.exists != b.exists {
			return a.exists
		}
		// Keep home after dedicated project parent folders when tied at count 0
		aIsHome := homeResolved != "" && a.path == homeResolved
		bIsHome := homeResolved != "" && b.path == homeResolved
		if aIsHome != bIsHome {
			return !aIsHome
		}
		return strings.ToLower(a.path) < strings.ToLower(b.path)
	})

	// Pick defaultDir: prefer highest-project-count existing parent, then upstream default, then ~/Projects, then home.
	for _, item := range list {
		if item.exists && item.count > 0 {
			defaultDir = item.path
			break
		}
	}
	if defaultDir == "" {
		for _, item := range list {
			if item.exists && item.isDefault {
				defaultDir = item.path
				break
			}
		}
	}
	if defaultDir == "" {
		for _, item := range list {
			if item.exists {
				defaultDir = item.path
				break
			}
		}
	}
	if defaultDir == "" && homeResolved != "" {
		defaultDir = homeResolved
	}

	if len(list) > maxRecommendedLocalDirs {
		// Ensure home is preserved if it was pushed past the limit.
		trimmed := list[:maxRecommendedLocalDirs]
		hasHome := false
		for _, it := range trimmed {
			if it.path == homeResolved {
				hasHome = true
				break
			}
		}
		if !hasHome && homeResolved != "" {
			if homeAgg, ok := aggMap[homeResolved]; ok {
				trimmed[len(trimmed)-1] = homeAgg
			}
		}
		list = trimmed
	}

	quickDirs = make([]LocalDirQuickItem, 0, len(list))
	for _, item := range list {
		name := filepath.Base(item.path)
		if item.path == homeResolved {
			name = "主目录 (~)"
		}
		quickDirs = append(quickDirs, LocalDirQuickItem{
			Path:         item.path,
			Name:         name,
			DisplayPath:  formatDisplayPath(item.path, homeResolved),
			ProjectCount: item.count,
			IsDefault:    item.path == defaultDir,
			Exists:       item.exists,
		})
	}
	return homeResolved, defaultDir, quickDirs, projectPathSet
}

// HandleListLocalDirs handles GET /gateway/fs/dirs?path=...&showHidden=false.
// Returns recommended project parent directories and subdirectories inside the requested path.
func (p *Proxy) HandleListLocalDirs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	home, defaultDir, quickDirs, projectPathSet := p.resolveRecommendedDirs(r.Context())
	rawPath := strings.TrimSpace(r.URL.Query().Get("path"))
	showHidden := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("showHidden")), "true")

	var currentPath string
	if rawPath == "" {
		currentPath = defaultDir
		if currentPath == "" {
			writeRPCError(w, http.StatusInternalServerError, "无法定位本机目录")
			return
		}
		if _, err := os.Stat(currentPath); err != nil {
			if home != "" {
				currentPath = home
			}
		}
	} else {
		valid, err := validateWorkspaceDir(rawPath, true)
		if err != nil {
			status := http.StatusForbidden
			if strings.Contains(err.Error(), "不存在") || strings.Contains(err.Error(), "不是目录") || strings.Contains(err.Error(), "绝对路径") {
				status = http.StatusBadRequest
			}
			writeRPCError(w, status, err.Error())
			return
		}
		currentPath = valid
	}

	dirEntries, err := os.ReadDir(currentPath)
	if err != nil {
		writeRPCError(w, http.StatusForbidden, fmt.Sprintf("无法读取目录: %v", err))
		return
	}

	skipNames := map[string]bool{
		".git":         true,
		"node_modules": true,
		"__pycache__":  true,
		".Trash":       true,
	}

	entries := make([]LocalDirEntry, 0, len(dirEntries))
	truncated := false
	for _, de := range dirEntries {
		name := de.Name()
		if skipNames[name] {
			continue
		}
		isHidden := strings.HasPrefix(name, ".")
		if isHidden && !showHidden {
			continue
		}

		fullPath := filepath.Join(currentPath, name)
		isDir := de.IsDir()
		if !isDir && (de.Type()&os.ModeSymlink != 0) {
			if fi, sErr := os.Stat(fullPath); sErr == nil && fi.IsDir() {
				isDir = true
			}
		}
		if !isDir {
			continue
		}
		resolvedEntry := evalSymlinksExistingPrefix(fullPath)
		if !IsSafeFilePath(resolvedEntry) {
			continue
		}

		_, gitErr := os.Stat(filepath.Join(fullPath, ".git"))
		entries = append(entries, LocalDirEntry{
			Name:      name,
			Path:      fullPath,
			IsGitRepo: gitErr == nil,
			IsProject: projectPathSet[filepath.Clean(fullPath)] || projectPathSet[resolvedEntry],
			IsHidden:  isHidden,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	if len(entries) > maxListedLocalDirEntry {
		entries = entries[:maxListedLocalDirEntry]
		truncated = true
	}

	parentPath := ""
	canGoUp := false
	candParent := filepath.Dir(currentPath)
	if candParent != "" && candParent != currentPath && !isDriveRoot(candParent) && IsSafeFilePath(candParent) {
		parentPath = candParent
		canGoUp = true
	}

	writeJSONStatus(w, http.StatusOK, LocalDirsResponse{
		OK:              true,
		Platform:        runtime.GOOS,
		HomePath:        home,
		DefaultDir:      defaultDir,
		CurrentPath:     currentPath,
		CurrentDisplay:  formatDisplayPath(currentPath, home),
		ParentPath:      parentPath,
		CanGoUp:         canGoUp,
		RecommendedDirs: quickDirs,
		Entries:         entries,
		Truncated:       truncated,
	})
}

// HandleCreateLocalDir handles POST /gateway/fs/mkdir.
// Creates a new local directory inside AllowedWorkspaceRoots().
func (p *Proxy) HandleCreateLocalDir(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ParentPath string `json:"parentPath"`
		Name       string `json:"name"`
		Path       string `json:"path"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "无效的请求体")
		return
	}

	var target string
	if strings.TrimSpace(req.Path) != "" && strings.TrimSpace(req.ParentPath) == "" {
		valid, err := validateWorkspaceDir(req.Path, false)
		if err != nil {
			status := http.StatusForbidden
			if strings.Contains(err.Error(), "绝对路径") || strings.Contains(err.Error(), "根目录") || strings.Contains(err.Error(), "无效") || strings.Contains(err.Error(), "非法") {
				status = http.StatusBadRequest
			}
			writeRPCError(w, status, err.Error())
			return
		}
		target = valid
	} else {
		dirName, err := validateSingleDirName(req.Name)
		if err != nil {
			writeRPCError(w, http.StatusBadRequest, err.Error())
			return
		}
		parentRaw := strings.TrimSpace(req.ParentPath)
		if parentRaw == "" {
			_, defaultDir, _, _ := p.resolveRecommendedDirs(r.Context())
			parentRaw = defaultDir
		}
		validParent, err := validateWorkspaceDir(parentRaw, true)
		if err != nil {
			status := http.StatusForbidden
			if strings.Contains(err.Error(), "不存在") || strings.Contains(err.Error(), "不是目录") || strings.Contains(err.Error(), "绝对路径") {
				status = http.StatusBadRequest
			}
			writeRPCError(w, status, err.Error())
			return
		}
		joined := filepath.Join(validParent, dirName)
		validTarget, err := validateWorkspaceDir(joined, false)
		if err != nil {
			writeRPCError(w, http.StatusForbidden, err.Error())
			return
		}
		target = validTarget
	}

	alreadyExists := false
	if fi, err := os.Stat(target); err == nil {
		if !fi.IsDir() {
			writeRPCError(w, http.StatusConflict, "目标位置已存在同名文件")
			return
		}
		alreadyExists = true
	} else if !os.IsNotExist(err) {
		writeRPCError(w, http.StatusInternalServerError, fmt.Sprintf("无法检查目录状态: %v", err))
		return
	}

	if !alreadyExists {
		if err := os.MkdirAll(target, 0o755); err != nil {
			writeRPCError(w, http.StatusInternalServerError, fmt.Sprintf("创建目录失败: %v", err))
			return
		}
	}

	home, _ := os.UserHomeDir()
	homeResolved := ""
	if home != "" {
		homeResolved = evalSymlinksExistingPrefix(home)
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"ok":            true,
		"path":          target,
		"name":          filepath.Base(target),
		"displayPath":   formatDisplayPath(target, homeResolved),
		"alreadyExists": alreadyExists,
	})
}

func parseGitHubCloneInput(raw string) (slug, repoName, cloneURL string, ok bool) {
	trimmed := strings.TrimSpace(raw)
	slug = normalizeGitHubSlug(trimmed)
	if slug == "" {
		return "", "", "", false
	}
	parts := strings.SplitN(slug, "/", 2)
	repoName = parts[1]
	if strings.HasPrefix(trimmed, "git@github.com:") {
		cloneURL = "git@github.com:" + slug + ".git"
	} else {
		cloneURL = "https://github.com/" + slug + ".git"
	}
	return slug, repoName, cloneURL, true
}

func detectOriginGitHubSlug(ctx context.Context, dir string) string {
	gCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := procutil.HideWindow(exec.CommandContext(gCtx, "git", "-C", dir, "remote", "get-url", "origin"))
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return normalizeGitHubSlug(strings.TrimSpace(string(out)))
}

// HandleCloneGitHubRepo clones a GitHub repository into a chosen local parent directory
// and automatically registers the cloned folder into Antigravity's official project list.
// When `async: true`, returns a trackable `jobId` immediately to avoid short HTTP / Relay timeouts.
func (p *Proxy) HandleCloneGitHubRepo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		RepoSlug       string `json:"repoSlug"`
		RepoURL        string `json:"repoUrl"`
		ParentDir      string `json:"parentDir"`
		ParentDirURI   string `json:"parentDirUri"`
		FolderName     string `json:"folderName"`
		StartSession   bool   `json:"startSession"`
		Async          bool   `json:"async"`
		ImportExisting bool   `json:"importExisting"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "无效的请求体")
		return
	}

	slug, repoName, cloneURL, ok := parseGitHubCloneInput(firstNonEmpty(req.RepoSlug, req.RepoURL))
	if !ok {
		writeRPCError(w, http.StatusBadRequest, "请输入有效的 GitHub 仓库地址（如 owner/repo 或 https://github.com/owner/repo）")
		return
	}

	folderName := repoName
	if strings.TrimSpace(req.FolderName) != "" {
		validatedName, err := validateSingleDirName(req.FolderName)
		if err != nil {
			writeRPCError(w, http.StatusBadRequest, err.Error())
			return
		}
		folderName = validatedName
	}

	parentDir := strings.TrimSpace(req.ParentDir)
	if parentDir == "" && req.ParentDirURI != "" {
		parentDir = uriToFilePath(req.ParentDirURI)
	}
	if parentDir == "" {
		_, defaultDir, _, _ := p.resolveRecommendedDirs(r.Context())
		if defaultDir != "" {
			parentDir = defaultDir
		} else {
			home, _ := os.UserHomeDir()
			parentDir = filepath.Join(home, "Projects")
		}
	}

	validParent, wsErr := validateWorkspaceDir(parentDir, false)
	if wsErr != nil {
		writeRPCError(w, http.StatusForbidden, wsErr.Error())
		return
	}
	targetDir := filepath.Join(validParent, folderName)
	validTarget, tErr := validateWorkspaceDir(targetDir, false)
	if tErr != nil {
		writeRPCError(w, http.StatusForbidden, tErr.Error())
		return
	}

	// Pre-flight check on target directory
	preExisted := false
	if fi, statErr := os.Stat(validTarget); statErr == nil {
		if !fi.IsDir() {
			writeJSONStatus(w, http.StatusConflict, CloneJobStatusResponse{
				OK:           false,
				Status:       "failed",
				Code:         "path_is_file",
				ExistingPath: validTarget,
				Error:        fmt.Sprintf("目标位置已存在同名文件: %s", validTarget),
			})
			return
		}
		preExisted = true
		entries, readErr := os.ReadDir(validTarget)
		if readErr == nil && len(entries) > 0 {
			existingSlug := detectOriginGitHubSlug(r.Context(), validTarget)
			if existingSlug != "" && strings.EqualFold(existingSlug, slug) {
				if req.ImportExisting {
					regCtx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
					defer cancel()
					prj, alreadyExists, statusCode, regErr := p.registerProjectFolder(regCtx, validTarget, folderName)
					if regErr != nil {
						writeRPCError(w, statusCode, regErr.Error())
						return
					}
					cascadeID := ""
					if req.StartSession && !req.Async {
						if port, _ := p.ActiveUpstream(); port > 0 {
							cascadeID = p.startCascadeForProject(regCtx, prj.ID, prj.URI)
						}
					}
					writeJSONStatus(w, http.StatusOK, CloneJobStatusResponse{
						OK:            true,
						Async:         false,
						RepoSlug:      slug,
						Status:        "done",
						Phase:         "done",
						Progress:      100,
						Message:       "已将本地已有仓库导入 Antigravity 项目列表",
						TargetPath:    validTarget,
						FolderURI:     prj.URI,
						Path:          prj.Path,
						Project:       &prj,
						AlreadyExists: alreadyExists,
						CascadeID:     cascadeID,
					})
					return
				}
				writeJSONStatus(w, http.StatusConflict, CloneJobStatusResponse{
					OK:           false,
					Status:       "failed",
					RepoSlug:     slug,
					Code:         "already_cloned",
					ExistingPath: validTarget,
					TargetPath:   validTarget,
					Error:        "目标目录已存在该 GitHub 仓库，可直接导入项目列表，或更换文件夹名。",
				})
				return
			}
			writeJSONStatus(w, http.StatusConflict, CloneJobStatusResponse{
				OK:           false,
				Status:       "failed",
				RepoSlug:     slug,
				Code:         "target_exists",
				ExistingPath: validTarget,
				TargetPath:   validTarget,
				Error:        "目标目录已存在且非空，请更换存放目录或修改文件夹名。",
			})
			return
		}
	}

	jobID := "clone_" + strings.ReplaceAll(newUUIDv4(), "-", "")[:16]
	jobCtx, cancel := context.WithTimeout(context.Background(), cloneJobTimeout)
	job := &cloneRepoJob{
		id:           jobID,
		repoSlug:     slug,
		cloneURL:     cloneURL,
		parentDir:    validParent,
		folderName:   folderName,
		targetDir:    validTarget,
		preExisted:   preExisted,
		startSession: req.StartSession && !req.Async,
		status:       "cloning",
		phase:        "cloning",
		progress:     5,
		message:      "正在连接 GitHub 并准备克隆…",
		createdAt:    time.Now(),
		cancel:       cancel,
		done:         make(chan struct{}),
	}

	p.cloneJobsMu.Lock()
	if p.cloneJobs == nil {
		p.cloneJobs = make(map[string]*cloneRepoJob)
	}
	p.pruneExpiredCloneJobsLocked(time.Now())
	p.cloneJobs[jobID] = job
	p.cloneJobsMu.Unlock()

	go p.executeCloneJob(jobCtx, job, repoName)

	if req.Async {
		writeJSONStatus(w, http.StatusOK, job.snapshot(true))
		return
	}

	// Synchronous compatibility path for legacy clients (wait for job or request cancel)
	select {
	case <-job.done:
		snap := job.snapshot(false)
		if !snap.OK {
			writeJSONStatus(w, http.StatusBadGateway, snap)
			return
		}
		writeJSONStatus(w, http.StatusOK, snap)
	case <-r.Context().Done():
		cancel()
		writeRPCError(w, http.StatusRequestTimeout, "克隆请求已取消或超时")
	}
}

// HandleCloneGitHubRepoStatus handles GET /gateway/projects/clone/status?jobId=...
func (p *Proxy) HandleCloneGitHubRepoStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID := strings.TrimSpace(r.URL.Query().Get("jobId"))
	if jobID == "" {
		writeRPCError(w, http.StatusBadRequest, "缺少 jobId 参数")
		return
	}

	p.cloneJobsMu.Lock()
	p.pruneExpiredCloneJobsLocked(time.Now())
	job := p.cloneJobs[jobID]
	p.cloneJobsMu.Unlock()

	if job == nil {
		writeRPCError(w, http.StatusNotFound, "未找到对应的克隆任务（可能已过期或网关已重启）")
		return
	}
	writeJSONStatus(w, http.StatusOK, job.snapshot(true))
}

// HandleCloneGitHubRepoCancel handles POST /gateway/projects/clone/cancel.
func (p *Proxy) HandleCloneGitHubRepoCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		JobID string `json:"jobId"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	jobID := strings.TrimSpace(req.JobID)
	if jobID == "" {
		writeRPCError(w, http.StatusBadRequest, "缺少 jobId 参数")
		return
	}

	p.cloneJobsMu.Lock()
	job := p.cloneJobs[jobID]
	p.cloneJobsMu.Unlock()

	if job == nil {
		writeRPCError(w, http.StatusNotFound, "未找到对应的克隆任务")
		return
	}

	job.markCancelled()
	if job.cancel != nil {
		job.cancel()
	}
	writeJSONStatus(w, http.StatusOK, job.snapshot(true))
}

// localGitCloneRunner allows deterministic unit testing without hitting live GitHub.
var localGitCloneRunner = func(ctx context.Context, p *Proxy, job *cloneRepoJob) (error, string) {
	return p.runLocalGitClone(ctx, job)
}

func (p *Proxy) executeCloneJob(ctx context.Context, job *cloneRepoJob, repoName string) {
	defer close(job.done)
	defer job.cancel()

	cleanupPartial := func() {
		if !job.preExisted && job.targetDir != "" {
			_ = os.RemoveAll(job.targetDir)
		}
	}

	if err := os.MkdirAll(job.parentDir, 0o755); err != nil {
		job.markFailed(fmt.Sprintf("无法创建父目录 %s: %v", job.parentDir, err))
		return
	}

	clonedPath := job.targetDir
	gitErr, gitStderr := localGitCloneRunner(ctx, p, job)

	if ctx.Err() != nil {
		cleanupPartial()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			job.markFailed("克隆超时（超过 5 分钟），请检查网络连接或代理设置")
		} else {
			job.markCancelled()
		}
		return
	}

	if gitErr != nil {
		// Smart fallback to upstream Antigravity CloneGitHubRepo RPC if git binary is missing
		// or HTTPS auth failed (private repo authenticated inside Antigravity).
		port, _ := p.ActiveUpstream()
		canFallbackRPC := port > 0 && job.folderName == repoName && shouldFallbackToUpstreamClone(gitErr, gitStderr)
		if canFallbackRPC {
			cleanupPartial()
			job.updateProgress("cloning", 15, "本地 Git 无法拉取，正在尝试通过 Antigravity 凭据克隆…")
			body, status, rpcErr := p.callUpstreamRPCLong(ctx, "CloneGitHubRepo", map[string]interface{}{
				"repoSlug":     job.repoSlug,
				"parentDirUri": filePathToURI(job.parentDir),
			})
			if ctx.Err() != nil {
				cleanupPartial()
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					job.markFailed("克隆超时（超过 5 分钟），请检查网络连接或代理设置")
				} else {
					job.markCancelled()
				}
				return
			}
			if rpcErr == nil && status == http.StatusOK {
				var rpcRes struct {
					FolderURI string `json:"folderUri"`
				}
				_ = json.Unmarshal(body, &rpcRes)
				if rpcRes.FolderURI != "" {
					if resolved := uriToFilePath(rpcRes.FolderURI); resolved != "" {
						clonedPath = resolved
					}
				}
				gitErr = nil
			} else {
				// Keep the more descriptive error
				rpcDetail := strings.TrimSpace(string(body))
				if rpcErr != nil {
					rpcDetail = rpcErr.Error()
				}
				job.markFailed(formatFriendlyCloneError(gitErr, gitStderr, rpcDetail))
				cleanupPartial()
				return
			}
		} else {
			cleanupPartial()
			job.markFailed(formatFriendlyCloneError(gitErr, gitStderr, ""))
			return
		}
	}

	validCloned, vErr := validateWorkspaceDir(clonedPath, true)
	if vErr != nil {
		job.markFailed(fmt.Sprintf("克隆完成但未找到有效目录: %v", vErr))
		return
	}

	job.updateProgress("registering", 94, "克隆完成，正在注册到 Antigravity 项目列表…")
	regCtx, regCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer regCancel()

	prj, alreadyExists, _, regErr := p.registerProjectFolder(regCtx, validCloned, job.folderName)
	if regErr != nil {
		job.markFailed(fmt.Sprintf("仓库已克隆至 %s，但加入项目列表失败: %v", validCloned, regErr))
		return
	}

	cascadeID := ""
	if job.startSession {
		if port, _ := p.ActiveUpstream(); port > 0 {
			cascadeID = p.startCascadeForProject(regCtx, prj.ID, prj.URI)
		}
	}

	job.markDone(prj, alreadyExists, cascadeID)
}

var gitProgressPctRe = regexp.MustCompile(`(?i)(Counting objects|Compressing objects|Receiving objects|Resolving deltas|Updating files):\s*(\d{1,3})%`)

func (p *Proxy) runLocalGitClone(ctx context.Context, job *cloneRepoJob) (error, string) {
	if _, err := exec.LookPath("git"); err != nil {
		return err, "git not found"
	}

	cmd := procutil.HideWindow(exec.CommandContext(ctx, "git", "clone", "--progress", "--", job.cloneURL, job.targetDir))
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=never",
		"LC_ALL=C",
	)
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err, ""
	}
	cmd.Stdout = io.Discard

	if err := cmd.Start(); err != nil {
		return err, err.Error()
	}

	var stderrLines []string
	var lineBuf bytes.Buffer
	reader := bufio.NewReaderSize(stderrPipe, 4096)

	flushSegment := func(seg string) {
		trimmed := strings.TrimSpace(seg)
		if trimmed == "" {
			return
		}
		if len(stderrLines) >= 24 {
			stderrLines = stderrLines[1:]
		}
		stderrLines = append(stderrLines, trimmed)

		if m := gitProgressPctRe.FindStringSubmatch(trimmed); len(m) == 3 {
			pct, _ := strconv.Atoi(m[2])
			if pct > 100 {
				pct = 100
			}
			stage := strings.ToLower(m[1])
			var overall int
			var label string
			switch {
			case strings.Contains(stage, "counting"):
				overall = 8 + pct/10
				label = fmt.Sprintf("正在统计仓库对象 (%d%%)…", pct)
			case strings.Contains(stage, "compressing"):
				overall = 18 + (pct*12)/100
				label = fmt.Sprintf("正在压缩传输对象 (%d%%)…", pct)
			case strings.Contains(stage, "receiving"):
				overall = 30 + (pct*50)/100
				label = fmt.Sprintf("正在拉取仓库数据 (%d%%)…", pct)
			case strings.Contains(stage, "resolving"):
				overall = 80 + (pct*10)/100
				label = fmt.Sprintf("正在解析提交差异 (%d%%)…", pct)
			default:
				overall = 90 + (pct*3)/100
				label = fmt.Sprintf("正在检出工作区文件 (%d%%)…", pct)
			}
			job.updateProgress("cloning", overall, label)
		} else if strings.HasPrefix(trimmed, "Cloning into ") {
			job.updateProgress("cloning", 8, "已连接 GitHub，正在拉取仓库…")
		}
	}

	for {
		b, readErr := reader.ReadByte()
		if readErr != nil {
			if lineBuf.Len() > 0 {
				flushSegment(lineBuf.String())
				lineBuf.Reset()
			}
			break
		}
		if b == '\r' || b == '\n' {
			flushSegment(lineBuf.String())
			lineBuf.Reset()
		} else if lineBuf.Len() < 512 {
			lineBuf.WriteByte(b)
		}
	}

	waitErr := cmd.Wait()
	return waitErr, strings.Join(stderrLines, "\n")
}

func shouldFallbackToUpstreamClone(gitErr error, stderr string) bool {
	if gitErr == nil {
		return false
	}
	if errors.Is(gitErr, exec.ErrNotFound) || strings.Contains(strings.ToLower(stderr), "git not found") {
		return true
	}
	low := strings.ToLower(stderr)
	return strings.Contains(low, "could not read username") ||
		strings.Contains(low, "terminal prompts disabled") ||
		strings.Contains(low, "authentication failed") ||
		strings.Contains(low, "permission denied") ||
		strings.Contains(low, "repository not found")
}

func formatFriendlyCloneError(gitErr error, stderr, rpcDetail string) string {
	low := strings.ToLower(stderr + "\n" + rpcDetail)
	switch {
	case strings.Contains(low, "could not read username") ||
		strings.Contains(low, "authentication failed") ||
		strings.Contains(low, "terminal prompts disabled") ||
		strings.Contains(low, "permission denied (publickey)"):
		return "仓库需要鉴权（私有仓库或未配置 Git/SSH 凭据），请先在本机完成 GitHub 登录或改用已配置 SSH 的地址。"
	case strings.Contains(low, "repository not found") || strings.Contains(low, "not found"):
		return "未找到该 GitHub 仓库，请检查 owner/repo 是否拼写正确，或确认私有仓库访问权限。"
	case strings.Contains(low, "could not resolve host") ||
		strings.Contains(low, "failed to connect") ||
		strings.Contains(low, "connection refused") ||
		strings.Contains(low, "timed out") ||
		strings.Contains(low, "unable to access"):
		return "无法连接 GitHub，请检查本机网络或代理配置后重试。"
	case strings.Contains(low, "already exists and is not an empty directory"):
		return "目标目录已存在且非空，请更换存放位置或修改文件夹名称。"
	}
	if rpcDetail != "" {
		return fmt.Sprintf("克隆失败: %s", truncateCloneErr(rpcDetail))
	}
	// Extract the last fatal/error line from git stderr if present
	lines := strings.Split(stderr, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(strings.ToLower(line), "fatal:") || strings.HasPrefix(strings.ToLower(line), "error:") {
			return fmt.Sprintf("克隆失败: %s", truncateCloneErr(line))
		}
	}
	if gitErr != nil {
		return fmt.Sprintf("克隆失败: %s", truncateCloneErr(gitErr.Error()))
	}
	return "克隆失败，请稍后重试"
}

func truncateCloneErr(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 220 {
		return s[:220] + "…"
	}
	return s
}
