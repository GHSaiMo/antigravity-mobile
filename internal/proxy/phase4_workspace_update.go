package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"antigravity-mobile/internal/i18n"
)

// Phase 4: Desktop self-update & changelog, workspace code/file search,
// artifact version snapshots, worktree PR create/update, and conversation starring.

// ===========================================================================
// 1. Desktop Update Status, Changelog & Remote Hot-Update
// ===========================================================================

func readSafeChangelogFile(rawPath string) (string, error) {
	return readSafeChangelogFileForLang(rawPath, i18n.GetLang())
}

func readSafeChangelogFileForLang(rawPath, lang string) (string, error) {
	l := i18n.ResolveLang(lang)
	clean := filepath.Clean(strings.TrimSpace(rawPath))
	if clean == "" || !filepath.IsAbs(clean) || strings.ContainsRune(clean, 0) {
		return "", fmt.Errorf("%s", i18n.T3For(l, "无效的更新日志路径", "Invalid changelog path", "無効な更新履歴パスです"))
	}
	fi, err := os.Lstat(clean)
	if err != nil {
		return "", err
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%s", i18n.T3For(l, "更新日志路径不是常规文件", "Changelog path is not a regular file", "更新履歴のパスは通常ファイルではありません"))
	}
	ext := strings.ToLower(filepath.Ext(clean))
	if ext != ".md" && ext != ".txt" && ext != ".markdown" && ext != "" {
		return "", fmt.Errorf("%s", i18n.T3For(l, "不支持的更新日志文件格式", "Unsupported changelog file format", "未対応の更新履歴ファイル形式です"))
	}
	f, err := os.Open(clean)
	if err != nil {
		return "", err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 512*1024))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// HandleDesktopUpdate serves /gateway/desktop/{update,update/apply,changelog}.
func (p *Proxy) HandleDesktopUpdate(w http.ResponseWriter, r *http.Request) {
	uiLang := resolveRequestUILang(r)
	switch r.URL.Path {
	case "/gateway/desktop/update":
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()

		var currentVersion, latestVersion string
		var updateAvailable bool
		if p.insp != nil {
			if cur := p.insp.Current(); cur != nil && cur.Version != "" {
				currentVersion = cur.Version
			}
		}

		body, status, err := p.callUpstreamRPC(ctx, "GetUpdateStatus", map[string]interface{}{})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "检查桌面端更新失败", "Failed to check desktop update", "デスクトップ版の更新確認に失敗しました")))
			return
		}
		var up struct {
			Status *struct {
				CurrentVersion  string `json:"currentVersion"`
				LatestVersion   string `json:"latestVersion"`
				UpdateAvailable bool   `json:"updateAvailable"`
			} `json:"status"`
			CurrentVersion  string `json:"currentVersion"`
			LatestVersion   string `json:"latestVersion"`
			UpdateAvailable bool   `json:"updateAvailable"`
		}
		if json.Unmarshal(body, &up) == nil {
			if up.Status != nil {
				currentVersion = firstNonEmpty(strings.TrimSpace(up.Status.CurrentVersion), currentVersion)
				latestVersion = strings.TrimSpace(up.Status.LatestVersion)
				updateAvailable = up.Status.UpdateAvailable
			} else {
				currentVersion = firstNonEmpty(strings.TrimSpace(up.CurrentVersion), currentVersion)
				latestVersion = strings.TrimSpace(up.LatestVersion)
				updateAvailable = up.UpdateAvailable
			}
		}

		runningCount := 0
		if summary := p.computeGatewaySummary(ctx, false); summary != nil {
			runningCount = summary.RunningCount
		}

		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":              true,
			"currentVersion":  currentVersion,
			"latestVersion":   latestVersion,
			"updateAvailable": updateAvailable,
			"runningCount":    runningCount,
		})

	case "/gateway/desktop/changelog":
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		version := strings.TrimSpace(r.URL.Query().Get("version"))
		if version == "" && r.Method == http.MethodPost && r.Body != nil {
			var req struct {
				Version string `json:"version"`
			}
			_ = json.NewDecoder(io.LimitReader(r.Body, 16*1024)).Decode(&req)
			version = strings.TrimSpace(req.Version)
		}
		payload := map[string]interface{}{}
		if version != "" {
			payload["version"] = version
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		body, status, err := p.callUpstreamRPC(ctx, "GetChangelog", payload)
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "获取更新日志失败", "Failed to load changelog", "更新履歴の取得に失敗しました")))
			return
		}
		var up struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(body, &up)
		markdown := ""
		if pth := strings.TrimSpace(up.Path); pth != "" {
			if txt, rerr := readSafeChangelogFileForLang(pth, uiLang); rerr == nil {
				markdown = txt
			}
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":       true,
			"version":  version,
			"path":     strings.TrimSpace(up.Path),
			"markdown": markdown,
		})

	case "/gateway/desktop/update/apply":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Confirm bool `json:"confirm"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 16*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		if !req.Confirm {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "请确认后再执行桌面端更新重启", "Please confirm before updating and restarting Desktop", "デスクトップ版の更新と再起動の前に確認してください"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		body, status, err := p.callUpstreamRPC(ctx, "ApplyUpdate", map[string]interface{}{})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "触发桌面端更新失败", "Failed to trigger desktop update", "デスクトップ版の更新開始に失敗しました")))
			return
		}
		var up struct {
			NotifiedHost bool `json:"notifiedHost"`
			Accepted     bool `json:"accepted"`
		}
		_ = json.Unmarshal(body, &up)
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":           true,
			"notifiedHost": up.NotifiedHost || up.Accepted,
			"message": i18n.T3For(uiLang,
				"已通知桌面端应用更新并重启",
				"Notified Desktop to apply the update and restart",
				"デスクトップ版に更新の適用と再起動を通知しました",
			),
		})

	default:
		http.NotFound(w, r)
	}
}

// ===========================================================================
// 2. Project Code & File Search (SearchCode / SearchFiles + Safe Local Fallback)
// ===========================================================================

type CodeSearchHit struct {
	Path          string   `json:"path"`
	RelativePath  string   `json:"relativePath"`
	URI           string   `json:"uri"`
	LineNumber    int      `json:"lineNumber"`
	Snippet       string   `json:"snippet"`
	MatchStartCol int      `json:"matchStartCol,omitempty"`
	MatchEndCol   int      `json:"matchEndCol,omitempty"`
	ContextBefore []string `json:"contextBefore,omitempty"`
	ContextAfter  []string `json:"contextAfter,omitempty"`
}

type FileSearchHit struct {
	Path         string `json:"path"`
	RelativePath string `json:"relativePath"`
	URI          string `json:"uri"`
	IsDirectory  bool   `json:"isDirectory,omitempty"`
}

func (p *Proxy) resolveWorkspaceTarget(ctx context.Context, cascadeID, workspaceURI string) (wsURI string, localDir string, err error) {
	return p.resolveWorkspaceTargetForLang(ctx, cascadeID, workspaceURI, i18n.GetLang())
}

func (p *Proxy) resolveWorkspaceTargetForLang(ctx context.Context, cascadeID, workspaceURI, lang string) (wsURI string, localDir string, err error) {
	l := i18n.ResolveLang(lang)
	if raw := strings.TrimSpace(workspaceURI); raw != "" {
		if isRemoteURI(raw) {
			return raw, "", nil
		}
		dir, verr := validateWorkspaceDir(raw, false)
		if verr != nil {
			return "", "", verr
		}
		return filePathToURI(dir), dir, nil
	}
	cid := strings.TrimSpace(cascadeID)
	if cid == "" || !cascadeIDRe.MatchString(cid) {
		return "", "", fmt.Errorf("%s", i18n.T3For(l, "缺少有效的工作区或会话 ID", "Missing valid workspace or conversation ID", "有効なワークスペースまたは会話 ID が指定されていません"))
	}
	port, token := p.ActiveUpstream()
	if port <= 0 {
		return "", "", fmt.Errorf("upstream language_server unavailable")
	}
	body, err := p.fetchAllTrajectoriesRaw(ctx, port, token)
	if err != nil {
		return "", "", err
	}
	uris := workspaceURIsFromSummaries(body, cid)
	for _, u := range uris {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if isRemoteURI(u) {
			return u, "", nil
		}
		if dir, perr := fileURIToLocalPath(u); perr == nil {
			if st, serr := os.Stat(dir); serr == nil && st.IsDir() {
				return filePathToURI(dir), dir, nil
			}
		}
	}
	return "", "", fmt.Errorf("%s", i18n.T3For(l, "该会话没有关联本地工作区", "This conversation has no associated local workspace", "この会話にはローカルワークスペースが関連付けられていません"))
}

func shouldSkipSearchDir(name string) bool {
	switch name {
	case ".git", "node_modules", "DerivedData", "build", "dist", ".next", ".gradle", "target", ".venv", "__pycache__", ".idea":
		return true
	}
	return false
}

func localFallbackCodeSearch(ctx context.Context, rootDir, query string, isRegex, caseSensitive bool, maxResults int) []CodeSearchHit {
	if rootDir == "" || query == "" {
		return nil
	}
	if maxResults <= 0 || maxResults > 100 {
		maxResults = 60
	}
	var re *regexp.Regexp
	if isRegex {
		pat := query
		if !caseSensitive {
			pat = "(?i)" + pat
		}
		var err error
		re, err = regexp.Compile(pat)
		if err != nil {
			return nil
		}
	}
	needle := query
	if !caseSensitive {
		needle = strings.ToLower(query)
	}
	hits := make([]CodeSearchHit, 0, 32)
	scannedFiles := 0
	const maxFiles = 2500

	_ = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil || len(hits) >= maxResults || scannedFiles >= maxFiles {
			return filepath.SkipAll
		}
		if d.IsDir() {
			if path != rootDir && shouldSkipSearchDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil || info.Size() == 0 || info.Size() > 512*1024 {
			return nil
		}
		scannedFiles++
		f, ferr := os.Open(path)
		if ferr != nil {
			return nil
		}
		defer f.Close()

		// Quick binary check on first 512 bytes
		var head [512]byte
		n, _ := f.Read(head[:])
		for i := 0; i < n; i++ {
			if head[i] == 0 {
				return nil
			}
		}
		_, _ = f.Seek(0, io.SeekStart)

		rel, _ := filepath.Rel(rootDir, path)
		rel = filepath.ToSlash(rel)
		uri := filePathToURI(path)

		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 32*1024), 256*1024)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			line := sc.Text()
			startCol, endCol := -1, -1
			if re != nil {
				if loc := re.FindStringIndex(line); loc != nil {
					startCol, endCol = loc[0], loc[1]
				}
			} else {
				hay := line
				if !caseSensitive {
					hay = strings.ToLower(line)
				}
				if idx := strings.Index(hay, needle); idx >= 0 {
					startCol, endCol = idx, idx+len(needle)
				}
			}
			if startCol >= 0 {
				snip := line
				if len(snip) > 240 {
					snip = snip[:240]
				}
				hits = append(hits, CodeSearchHit{
					Path:          path,
					RelativePath:  rel,
					URI:           uri,
					LineNumber:    lineNo,
					Snippet:       snip,
					MatchStartCol: startCol,
					MatchEndCol:   endCol,
				})
				if len(hits) >= maxResults {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	return hits
}

func localFallbackFileSearch(ctx context.Context, rootDir, query string, includeDirs bool, maxResults int) []FileSearchHit {
	if rootDir == "" {
		return nil
	}
	if maxResults <= 0 || maxResults > 100 {
		maxResults = 60
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	hits := make([]FileSearchHit, 0, 32)
	scanned := 0
	const maxEntries = 8000

	_ = filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil || len(hits) >= maxResults || scanned >= maxEntries {
			return filepath.SkipAll
		}
		if path == rootDir {
			return nil
		}
		if d.IsDir() && shouldSkipSearchDir(d.Name()) {
			return filepath.SkipDir
		}
		scanned++
		if d.IsDir() && !includeDirs {
			return nil
		}
		rel, _ := filepath.Rel(rootDir, path)
		rel = filepath.ToSlash(rel)
		if needle != "" && !strings.Contains(strings.ToLower(rel), needle) {
			return nil
		}
		hits = append(hits, FileSearchHit{
			Path:         path,
			RelativePath: rel,
			URI:          filePathToURI(path),
			IsDirectory:  d.IsDir(),
		})
		return nil
	})
	return hits
}

// HandleWorkspaceSearch serves POST /gateway/search/code and POST /gateway/search/files.
func (p *Proxy) HandleWorkspaceSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uiLang := resolveRequestUILang(r)
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	switch r.URL.Path {
	case "/gateway/search/code":
		var req struct {
			CascadeID       string `json:"cascadeId"`
			WorkspaceURI    string `json:"workspaceUri"`
			Query           string `json:"query"`
			IsRegex         bool   `json:"isRegex"`
			CaseSensitive   bool   `json:"caseSensitive"`
			CaseSensitivity string `json:"caseSensitivity"`
			MaxResults      int    `json:"maxResults"`
			LinesContext    int    `json:"linesContext"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		query := strings.TrimSpace(req.Query)
		if query == "" {
			writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true, "results": []CodeSearchHit{}})
			return
		}
		wsURI, localDir, err := p.resolveWorkspaceTargetForLang(ctx, req.CascadeID, req.WorkspaceURI, uiLang)
		if err != nil {
			writeRPCError(w, http.StatusBadRequest, err.Error())
			return
		}
		maxRes := req.MaxResults
		if maxRes <= 0 || maxRes > 100 {
			maxRes = 60
		}
		cs := strings.TrimSpace(req.CaseSensitivity)
		if cs == "" {
			if req.CaseSensitive {
				cs = "CASE_SENSITIVITY_YES"
			} else {
				cs = "CASE_SENSITIVITY_AUTO"
			}
		}
		payload := map[string]interface{}{
			"workspaceUri":    wsURI,
			"query":           query,
			"isRegex":         req.IsRegex,
			"caseSensitivity": cs,
			"maxResults":      maxRes,
		}
		if req.LinesContext > 0 && req.LinesContext <= 5 {
			payload["linesContext"] = req.LinesContext
		}
		body, status, rpcErr := p.callUpstreamRPC(ctx, "SearchCode", payload)
		results := make([]CodeSearchHit, 0)
		source := "upstream"
		if rpcErr == nil && status == http.StatusOK {
			var up struct {
				Results []struct {
					AbsoluteURI                string            `json:"absoluteUri"`
					LineNumber                 int               `json:"lineNumber"`
					Snippet                    string            `json:"snippet"`
					MatchStartCol              int               `json:"matchStartCol"`
					MatchEndCol                int               `json:"matchEndCol"`
					ContextBefore              []string          `json:"contextBefore"`
					ContextAfter               []string          `json:"contextAfter"`
					WorkspaceUrisToRelativeMap map[string]string `json:"workspaceUrisToRelativePaths"`
				} `json:"results"`
			}
			if json.Unmarshal(body, &up) == nil {
				for _, r := range up.Results {
					uri := strings.TrimSpace(r.AbsoluteURI)
					pth := uriToFilePath(uri)
					rel := ""
					for _, v := range r.WorkspaceUrisToRelativeMap {
						if strings.TrimSpace(v) != "" {
							rel = filepath.ToSlash(strings.TrimSpace(v))
							break
						}
					}
					if rel == "" && localDir != "" && pth != "" {
						if computed, rerr := filepath.Rel(localDir, pth); rerr == nil {
							rel = filepath.ToSlash(computed)
						}
					}
					if rel == "" {
						rel = filepath.Base(pth)
					}
					results = append(results, CodeSearchHit{
						Path:          pth,
						RelativePath:  rel,
						URI:           uri,
						LineNumber:    r.LineNumber,
						Snippet:       r.Snippet,
						MatchStartCol: r.MatchStartCol,
						MatchEndCol:   r.MatchEndCol,
						ContextBefore: r.ContextBefore,
						ContextAfter:  r.ContextAfter,
					})
				}
			}
		}
		if len(results) == 0 && localDir != "" {
			if fb := localFallbackCodeSearch(ctx, localDir, query, req.IsRegex, req.CaseSensitive, maxRes); len(fb) > 0 {
				results = fb
				source = "local"
			}
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":           true,
			"workspaceUri": wsURI,
			"source":       source,
			"results":      results,
		})

	case "/gateway/search/files":
		var req struct {
			CascadeID          string `json:"cascadeId"`
			WorkspaceURI       string `json:"workspaceUri"`
			Query              string `json:"query"`
			MaxResults         int    `json:"maxResults"`
			IncludeDirectories bool   `json:"includeDirectories"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		wsURI, localDir, err := p.resolveWorkspaceTargetForLang(ctx, req.CascadeID, req.WorkspaceURI, uiLang)
		if err != nil {
			writeRPCError(w, http.StatusBadRequest, err.Error())
			return
		}
		maxRes := req.MaxResults
		if maxRes <= 0 || maxRes > 100 {
			maxRes = 60
		}
		payload := map[string]interface{}{
			"workspaceUri":       wsURI,
			"query":              strings.TrimSpace(req.Query),
			"maxResults":         maxRes,
			"includeDirectories": req.IncludeDirectories,
		}
		if cid := strings.TrimSpace(req.CascadeID); cid != "" {
			payload["conversationId"] = cid
		}
		body, status, rpcErr := p.callUpstreamRPC(ctx, "SearchFiles", payload)
		results := make([]FileSearchHit, 0)
		source := "upstream"
		if rpcErr == nil && status == http.StatusOK {
			var up struct {
				Results []json.RawMessage `json:"results"`
				Matches []struct {
					Path         string `json:"path"`
					RelativePath string `json:"relativePath"`
					IsDirectory  bool   `json:"isDirectory"`
				} `json:"matches"`
			}
			if json.Unmarshal(body, &up) == nil {
				for _, m := range up.Matches {
					pth := strings.TrimSpace(m.Path)
					rel := filepath.ToSlash(strings.TrimSpace(m.RelativePath))
					if pth == "" && localDir != "" && rel != "" {
						pth = filepath.Join(localDir, filepath.FromSlash(rel))
					}
					if rel == "" && localDir != "" && pth != "" {
						if r, rerr := filepath.Rel(localDir, pth); rerr == nil {
							rel = filepath.ToSlash(r)
						}
					}
					if pth != "" {
						results = append(results, FileSearchHit{
							Path:         pth,
							RelativePath: firstNonEmpty(rel, filepath.Base(pth)),
							URI:          filePathToURI(pth),
							IsDirectory:  m.IsDirectory,
						})
					}
				}
				for _, rawItem := range up.Results {
					var s string
					if json.Unmarshal(rawItem, &s) == nil && strings.TrimSpace(s) != "" {
						pth := uriToFilePath(strings.TrimSpace(s))
						rel := pth
						if localDir != "" {
							if r, rerr := filepath.Rel(localDir, pth); rerr == nil {
								rel = filepath.ToSlash(r)
							}
						}
						results = append(results, FileSearchHit{
							Path:         pth,
							RelativePath: rel,
							URI:          filePathToURI(pth),
						})
						continue
					}
					var obj struct {
						AbsoluteURI  string `json:"absoluteUri"`
						URI          string `json:"uri"`
						Path         string `json:"path"`
						RelativePath string `json:"relativePath"`
						IsDirectory  bool   `json:"isDirectory"`
					}
					if json.Unmarshal(rawItem, &obj) == nil {
						uri := firstNonEmpty(strings.TrimSpace(obj.AbsoluteURI), strings.TrimSpace(obj.URI))
						pth := firstNonEmpty(strings.TrimSpace(obj.Path), uriToFilePath(uri))
						if uri == "" && pth != "" {
							uri = filePathToURI(pth)
						}
						rel := filepath.ToSlash(strings.TrimSpace(obj.RelativePath))
						if rel == "" && localDir != "" && pth != "" {
							if r, rerr := filepath.Rel(localDir, pth); rerr == nil {
								rel = filepath.ToSlash(r)
							}
						}
						if pth != "" || uri != "" {
							results = append(results, FileSearchHit{
								Path:         pth,
								RelativePath: firstNonEmpty(rel, filepath.Base(pth)),
								URI:          uri,
								IsDirectory:  obj.IsDirectory,
							})
						}
					}
				}
			}
		}
		if len(results) == 0 && localDir != "" {
			if fb := localFallbackFileSearch(ctx, localDir, req.Query, req.IncludeDirectories, maxRes); len(fb) > 0 {
				results = fb
				source = "local"
			}
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":           true,
			"workspaceUri": wsURI,
			"source":       source,
			"results":      results,
		})

	default:
		http.NotFound(w, r)
	}
}

// ===========================================================================
// 3. Artifact Version Snapshots (GetArtifactSnapshots)
// ===========================================================================

type ArtifactSnapshotItem struct {
	Index        int    `json:"index"`
	ArtifactName string `json:"artifactName"`
	ArtifactURI  string `json:"artifactUri"`
	Content      string `json:"content"`
	LastEdited   string `json:"lastEdited,omitempty"`
	ReviewState  string `json:"reviewState,omitempty"`
	Summary      string `json:"summary,omitempty"`
}

// HandleArtifactSnapshots serves GET/POST /gateway/artifacts/snapshots.
func (p *Proxy) HandleArtifactSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uiLang := resolveRequestUILang(r)
	var cascadeID, filterURI, filterName string
	if r.Method == http.MethodPost && r.Body != nil {
		var req struct {
			CascadeID    string `json:"cascadeId"`
			URI          string `json:"uri"`
			ArtifactURI  string `json:"artifactUri"`
			ArtifactName string `json:"artifactName"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req)
		cascadeID = strings.TrimSpace(req.CascadeID)
		filterURI = firstNonEmpty(strings.TrimSpace(req.ArtifactURI), strings.TrimSpace(req.URI))
		filterName = strings.TrimSpace(req.ArtifactName)
	} else {
		q := r.URL.Query()
		cascadeID = strings.TrimSpace(q.Get("cascadeId"))
		filterURI = firstNonEmpty(strings.TrimSpace(q.Get("artifactUri")), strings.TrimSpace(q.Get("uri")))
		filterName = strings.TrimSpace(q.Get("artifactName"))
	}
	if cascadeID == "" || !cascadeIDRe.MatchString(cascadeID) {
		writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "缺少有效的 cascadeId", "Missing valid cascadeId", "有効な cascadeId が指定されていません"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	body, status, err := p.callUpstreamRPC(ctx, "GetArtifactSnapshots", map[string]interface{}{
		"cascadeId": cascadeID,
	})
	if err != nil {
		writeRPCError(w, status, err.Error())
		return
	}
	if status != http.StatusOK {
		writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "获取文档历史版本失败", "Failed to load artifact history", "ドキュメント履歴の取得に失敗しました")))
		return
	}

	var up struct {
		ArtifactSnapshots []struct {
			ArtifactName        string `json:"artifactName"`
			ArtifactAbsoluteURI string `json:"artifactAbsoluteUri"`
			Content             string `json:"content"`
			LastEdited          string `json:"lastEdited"`
			ReviewState         string `json:"reviewState"`
			ArtifactMetadata    *struct {
				Summary string `json:"summary"`
			} `json:"artifactMetadata"`
		} `json:"artifactSnapshots"`
	}
	if err := json.Unmarshal(body, &up); err != nil {
		writeRPCError(w, http.StatusBadGateway, i18n.T3For(uiLang, "解析文档快照失败", "Failed to parse artifact snapshots", "ドキュメントスナップショットの解析に失敗しました"))
		return
	}

	normPath := func(raw string) string {
		return strings.ToLower(filepath.ToSlash(filepath.Clean(uriToFilePath(raw))))
	}
	wantPath := ""
	if filterURI != "" {
		wantPath = normPath(filterURI)
	}
	wantBase := strings.ToLower(strings.TrimSpace(filterName))
	if wantBase == "" && filterURI != "" {
		wantBase = strings.ToLower(filepath.Base(uriToFilePath(filterURI)))
	}

	out := make([]ArtifactSnapshotItem, 0, len(up.ArtifactSnapshots))
	for i, s := range up.ArtifactSnapshots {
		itemURI := strings.TrimSpace(s.ArtifactAbsoluteURI)
		itemName := firstNonEmpty(strings.TrimSpace(s.ArtifactName), filepath.Base(uriToFilePath(itemURI)))
		if wantPath != "" {
			gotPath := normPath(itemURI)
			if gotPath != wantPath && !strings.EqualFold(filepath.Base(gotPath), wantBase) && !strings.EqualFold(itemName, wantBase) {
				continue
			}
		} else if wantBase != "" && !strings.EqualFold(itemName, wantBase) {
			continue
		}
		summary := ""
		if s.ArtifactMetadata != nil {
			summary = strings.TrimSpace(s.ArtifactMetadata.Summary)
		}
		out = append(out, ArtifactSnapshotItem{
			Index:        i + 1,
			ArtifactName: itemName,
			ArtifactURI:  itemURI,
			Content:      s.Content,
			LastEdited:   strings.TrimSpace(s.LastEdited),
			ReviewState:  strings.TrimSpace(s.ReviewState),
			Summary:      summary,
		})
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"cascadeId": cascadeID,
		"snapshots": out,
	})
}

// ===========================================================================
// 4. Worktree Create / Update Pull Request (UpdatePRForWorktree)
// ===========================================================================

// HandleWorktreePR serves POST /gateway/worktree/pr.
func (p *Proxy) HandleWorktreePR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uiLang := resolveRequestUILang(r)
	var req struct {
		CascadeID     string `json:"cascadeId"`
		Title         string `json:"title"`
		Body          string `json:"body"`
		CommitMessage string `json:"commitMessage"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 128*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
		return
	}
	cid := strings.TrimSpace(req.CascadeID)
	if cid == "" || !cascadeIDRe.MatchString(cid) {
		writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "缺少有效的 cascadeId", "Missing valid cascadeId", "有効な cascadeId が指定されていません"))
		return
	}
	port, token := p.ActiveUpstream()
	if port <= 0 {
		writeRPCError(w, http.StatusServiceUnavailable, "no active Antigravity upstream")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	repoRoot, err := p.resolveCascadeRepo(ctx, cid, port, token)
	if err != nil {
		writeRPCError(w, http.StatusBadRequest, err.Error())
		return
	}
	payload := map[string]interface{}{
		"worktreeFsPath": repoRoot,
	}
	if t := strings.TrimSpace(req.Title); t != "" {
		payload["title"] = t
	}
	if b := strings.TrimSpace(req.Body); b != "" {
		payload["body"] = b
	}
	if cm := strings.TrimSpace(req.CommitMessage); cm != "" {
		payload["commitMessage"] = cm
	}
	body, status, rpcErr := p.callUpstreamRPCLong(ctx, "UpdatePRForWorktree", payload)
	if rpcErr != nil {
		writeRPCError(w, status, rpcErr.Error())
		return
	}
	if status != http.StatusOK {
		writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "创建或更新 Worktree PR 失败", "Failed to create or update Worktree PR", "Worktree PR の作成・更新に失敗しました")))
		return
	}
	var up struct {
		PrURL string `json:"prUrl"`
	}
	_ = json.Unmarshal(body, &up)
	invalidateGitStatusCache(repoRoot)
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"ok":    true,
		"prUrl": strings.TrimSpace(up.PrURL),
	})
}

// ===========================================================================
// 5. Conversation Starring (UpdateConversationAnnotations + local .pbtxt mirror)
// ===========================================================================

// HandleStarCascade serves POST /gateway/cascade/star.
func (p *Proxy) HandleStarCascade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	uiLang := resolveRequestUILang(r)
	var req struct {
		CascadeID string `json:"cascadeId"`
		Starred   *bool  `json:"starred"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
		return
	}
	cid := strings.TrimSpace(req.CascadeID)
	if cid == "" || !cascadeIDRe.MatchString(cid) {
		writeRPCError(w, http.StatusBadRequest, "invalid cascadeId")
		return
	}
	starred := true
	if req.Starred != nil {
		starred = *req.Starred
	}
	writeAnnotationStarred(cid, starred)

	if port, _ := p.ActiveUpstream(); port > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		_, status, err := p.callUpstreamRPC(ctx, "UpdateConversationAnnotations", map[string]interface{}{
			"cascadeIds":       []string{cid},
			"annotations":      map[string]interface{}{"starred": starred},
			"mergeAnnotations": true,
		})
		if err != nil {
			writeRPCError(w, http.StatusBadGateway, "upstream star failed: "+err.Error())
			return
		}
		if status >= 400 {
			writeRPCError(w, status, fmt.Sprintf("upstream star returned HTTP %d", status))
			return
		}
	}

	p.SuppressDesktopFocus(AntiReflectionDuration)
	p.invalidateAllTrajectories()
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"cascadeId": cid,
		"starred":   starred,
	})
}
