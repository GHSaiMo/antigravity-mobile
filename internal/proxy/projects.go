package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ProjectItem represents a discovered upstream project or workspace.
type ProjectItem struct {
	ID           string     `json:"id,omitempty"`
	Name         string     `json:"name"`
	URI          string     `json:"uri"`
	Path         string     `json:"path"`
	IsWorkspace  bool       `json:"isWorkspace"`
	SessionCount int        `json:"sessionCount"`
	LastActive   *time.Time `json:"lastActive,omitempty"`
}

type vscdbHistoryEntry struct {
	FolderURI string `json:"folderUri"`
	Workspace *struct {
		ConfigPath string `json:"configPath"`
	} `json:"workspace"`
	FileURI string `json:"fileUri"`
}

type vscdbHistory struct {
	Entries []vscdbHistoryEntry `json:"entries"`
}

// GetProjects discovers all projects from local IDE database and trajectory history.
// Merges official projects, workspaceStorage folders, state.vscdb history, and active trajectories.
func (p *Proxy) GetProjects() ([]ProjectItem, error) {
	p.mu.RLock()
	port := p.activePort
	token := p.activeToken
	p.mu.RUnlock()

	// 1. Fetch active/historical sessions statistics if upstream is connected
	sessionStats := make(map[string]struct {
		count      int
		lastActive time.Time
	})

	if port > 0 {
		trajectories, err := p.fetchTrajectoriesSummary(port, token)
		if err == nil {
			for cid, sum := range trajectories {
				// Exclude internal subagents from project statistics
				if sum.TrajectoryMetadata != nil {
					meta := sum.TrajectoryMetadata
					if meta.ParentConversationID != "" ||
						meta.SubagentSpec != nil ||
						meta.AgentScript != nil ||
						meta.NestingDepth > 0 ||
						(meta.RootConversationID != "" && meta.RootConversationID != cid) {
						continue
					}
				}

				var uris []string
				if sum.TrajectoryMetadata != nil && len(sum.TrajectoryMetadata.WorkspaceUris) > 0 {
					uris = sum.TrajectoryMetadata.WorkspaceUris
				} else if len(sum.Workspaces) > 0 {
					for _, w := range sum.Workspaces {
						if w.WorkspaceFolderAbsoluteUri != "" {
							uris = append(uris, w.WorkspaceFolderAbsoluteUri)
						}
					}
				}

				var activeTime time.Time
				if sum.LastModifiedTime != "" {
					if t, err := parseTime(sum.LastModifiedTime); err == nil {
						activeTime = t
					}
				}

				for _, u := range uris {
					normalized := normalizeURI(u)
					if normalized == "" {
						continue
					}
					stat := sessionStats[normalized]
					stat.count++
					if activeTime.After(stat.lastActive) {
						stat.lastActive = activeTime
					}
					sessionStats[normalized] = stat
				}
			}
		}
	}

	projectMap := make(map[string]*ProjectItem)

	mergeProject := func(prj ProjectItem) {
		norm := normalizeURI(prj.URI)
		if norm == "" {
			return
		}
		if existing, exists := projectMap[norm]; exists {
			if existing.ID == "" && prj.ID != "" {
				existing.ID = prj.ID
			}
			if prj.ID != "" && prj.Name != "" {
				existing.Name = prj.Name
			}
			if prj.IsWorkspace {
				existing.IsWorkspace = true
			}
			if prj.Path != "" && (existing.Path == "" || !filepath.IsAbs(existing.Path)) {
				existing.Path = prj.Path
			}
			if prj.LastActive != nil && (existing.LastActive == nil || prj.LastActive.After(*existing.LastActive)) {
				existing.LastActive = prj.LastActive
			}
			if prj.SessionCount > existing.SessionCount {
				existing.SessionCount = prj.SessionCount
			}
			return
		}

		itemCopy := prj
		itemCopy.URI = norm
		if itemCopy.Path == "" {
			itemCopy.Path = uriToPath(norm)
		}
		if stat, ok := sessionStats[norm]; ok {
			itemCopy.SessionCount = stat.count
			if !stat.lastActive.IsZero() && (itemCopy.LastActive == nil || stat.lastActive.After(*itemCopy.LastActive)) {
				t := stat.lastActive
				itemCopy.LastActive = &t
			}
		}
		projectMap[norm] = &itemCopy
	}

	// 2. Load official ordered Projects from app_storage.json + ReadProjects RPC
	if officialProjects, err := p.fetchOfficialProjects(port, token, sessionStats); err == nil {
		for _, prj := range officialProjects {
			mergeProject(prj)
		}
	}

	// 3. Load workspace projects from workspaceStorage (plain JSON, works on all OSes without SQLite/Python)
	for _, prj := range fetchProjectsFromWorkspaceStorage() {
		mergeProject(prj)
	}

	// 4. Load from state.vscdb (history.recentlyOpenedPathsList)
	for _, prj := range fetchProjectsFromStateDB() {
		mergeProject(prj)
	}

	// 5. Load any remaining active workspace URIs from trajectories
	for norm, stat := range sessionStats {
		if _, exists := projectMap[norm]; !exists {
			parsedPath := uriToPath(norm)
			if parsedPath == "" {
				continue
			}
			if !isRemoteURI(norm) {
				if _, err := os.Stat(parsedPath); err != nil {
					continue
				}
			}

			isWs := strings.HasSuffix(parsedPath, ".code-workspace")
			name := filepath.Base(parsedPath)
			if isWs {
				name = strings.TrimSuffix(name, ".code-workspace")
			}

			var lastActive *time.Time
			if !stat.lastActive.IsZero() {
				t := stat.lastActive
				lastActive = &t
			}

			mergeProject(ProjectItem{
				Name:         name,
				URI:          norm,
				Path:         parsedPath,
				IsWorkspace:  isWs,
				SessionCount: stat.count,
				LastActive:   lastActive,
			})
		}
	}

	var result []ProjectItem
	for _, prj := range projectMap {
		result = append(result, *prj)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].LastActive != nil && result[j].LastActive != nil {
			return result[i].LastActive.After(*result[j].LastActive)
		}
		if result[i].LastActive != nil {
			return true
		}
		if result[j].LastActive != nil {
			return false
		}
		if result[i].SessionCount != result[j].SessionCount {
			return result[i].SessionCount > result[j].SessionCount
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	return result, nil
}

func getAntigravityAppStoragePaths() []string {
	var paths []string
	if appData := os.Getenv("APPDATA"); appData != "" {
		paths = append(paths,
			filepath.Join(appData, "Antigravity", "app_storage.json"),
			filepath.Join(appData, "Antigravity IDE", "app_storage.json"),
		)
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(home, "Library", "Application Support", "Antigravity", "app_storage.json"),
			filepath.Join(home, "Library", "Application Support", "Antigravity IDE", "app_storage.json"),
			filepath.Join(home, "AppData", "Roaming", "Antigravity", "app_storage.json"),
			filepath.Join(home, "AppData", "Roaming", "Antigravity IDE", "app_storage.json"),
		)
	}
	return paths
}

// fetchOfficialProjects loads projectsOrder and calls upstream ReadProjects to get the exact 21 projects in order.
func (p *Proxy) fetchOfficialProjects(port int, token string, sessionStats map[string]struct {
	count      int
	lastActive time.Time
}) ([]ProjectItem, error) {
	var storageBytes []byte
	for _, appStoragePath := range getAntigravityAppStoragePaths() {
		if b, err := os.ReadFile(appStoragePath); err == nil && len(b) > 0 {
			storageBytes = b
			break
		}
	}
	if len(storageBytes) == 0 {
		return nil, fmt.Errorf("app_storage.json not found")
	}

	var storageMap map[string]interface{}
	if err := json.Unmarshal(storageBytes, &storageMap); err != nil {
		return nil, err
	}

	rawOrder, ok := storageMap["projectsOrder"].(string)
	if !ok || rawOrder == "" {
		return nil, fmt.Errorf("projectsOrder not found in app_storage.json")
	}

	var order []string
	if err := json.Unmarshal([]byte(rawOrder), &order); err != nil || len(order) == 0 {
		return nil, fmt.Errorf("invalid projectsOrder json")
	}

	// If upstream connected, call ReadProjects RPC
	if port > 0 {
		readURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/ReadProjects", port)
		reqBody, _ := json.Marshal(map[string]interface{}{"ids": order})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, readURL, bytes.NewReader(reqBody))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Connect-Protocol-Version", "1")
			if token != "" {
				req.Header.Set("x-codeium-csrf-token", token)
			}
			resp, err := p.mediumClient.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				defer resp.Body.Close()
				var reader io.Reader = resp.Body
				if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
					if gz, err := GetGzipReader(resp.Body); err == nil {
						defer PutGzipReader(gz)
						reader = gz
					}
				}

				var readResp struct {
					Projects []struct {
						ID               string `json:"id"`
						Name             string `json:"name"`
						IsWorkspaceOnly  bool   `json:"isWorkspaceOnly"`
						ProjectResources *struct {
							Resources []struct {
								FolderURI string `json:"folderUri"`
								GitFolder *struct {
									FolderURI string `json:"folderUri"`
								} `json:"gitFolder"`
							} `json:"resources"`
						} `json:"projectResources"`
					} `json:"projects"`
				}

				if err := json.NewDecoder(reader).Decode(&readResp); err == nil && len(readResp.Projects) > 0 {
					projMap := make(map[string]struct {
						id   string
						name string
						uri  string
						isWs bool
					})

					for _, prj := range readResp.Projects {
						uri := ""
						if prj.ProjectResources != nil {
							for _, r := range prj.ProjectResources.Resources {
								if r.FolderURI != "" {
									uri = r.FolderURI
									break
								}
								if r.GitFolder != nil && r.GitFolder.FolderURI != "" {
									uri = r.GitFolder.FolderURI
									break
								}
							}
						}
						projMap[prj.ID] = struct {
							id   string
							name string
							uri  string
							isWs bool
						}{
							id:   prj.ID,
							name: prj.Name,
							uri:  uri,
							isWs: prj.IsWorkspaceOnly,
						}
					}

					var orderedItems []ProjectItem
					for _, id := range order {
						if pInfo, exists := projMap[id]; exists {
							norm := normalizeURI(pInfo.uri)
							path := uriToPath(norm)
							var lastActive *time.Time
							sessionCount := 0
							if stat, ok := sessionStats[norm]; ok {
								sessionCount = stat.count
								if !stat.lastActive.IsZero() {
									t := stat.lastActive
									lastActive = &t
								}
							}

							orderedItems = append(orderedItems, ProjectItem{
								ID:           pInfo.id,
								Name:         pInfo.name,
								URI:          norm,
								Path:         path,
								IsWorkspace:  pInfo.isWs,
								SessionCount: sessionCount,
								LastActive:   lastActive,
							})
						}
					}

					if len(orderedItems) > 0 {
						return orderedItems, nil
					}
				}
			}
		}
	}

	// Fallback to mac-workspace.code-workspace folders
	home, _ := os.UserHomeDir()
	wsFile := filepath.Join(home, "Projects", "mac-workspace.code-workspace")
	if wsBytes, err := os.ReadFile(wsFile); err == nil {
		var wsData struct {
			Folders []struct {
				Name string `json:"name"`
				Path string `json:"path"`
			} `json:"folders"`
		}
		if err := json.Unmarshal(wsBytes, &wsData); err == nil && len(wsData.Folders) > 0 {
			var fallbackItems []ProjectItem
			baseDir := filepath.Dir(wsFile)
			for _, f := range wsData.Folders {
				absPath := f.Path
				if !filepath.IsAbs(absPath) {
					absPath = filepath.Clean(filepath.Join(baseDir, absPath))
				}
				uri := "file://" + absPath
				norm := normalizeURI(uri)
				var lastActive *time.Time
				sessionCount := 0
				if stat, ok := sessionStats[norm]; ok {
					sessionCount = stat.count
					if !stat.lastActive.IsZero() {
						t := stat.lastActive
						lastActive = &t
					}
				}
				fallbackItems = append(fallbackItems, ProjectItem{
					Name:         f.Name,
					URI:          norm,
					Path:         absPath,
					IsWorkspace:  false,
					SessionCount: sessionCount,
					LastActive:   lastActive,
				})
			}
			return fallbackItems, nil
		}
	}

	return nil, fmt.Errorf("could not fetch official projects")
}

func getAntigravityWorkspaceStoragePaths() []string {
	var paths []string
	if appData := os.Getenv("APPDATA"); appData != "" {
		paths = append(paths,
			filepath.Join(appData, "Antigravity", "User", "workspaceStorage"),
			filepath.Join(appData, "Antigravity IDE", "User", "workspaceStorage"),
		)
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(home, "Library", "Application Support", "Antigravity", "User", "workspaceStorage"),
			filepath.Join(home, "Library", "Application Support", "Antigravity IDE", "User", "workspaceStorage"),
			filepath.Join(home, "AppData", "Roaming", "Antigravity", "User", "workspaceStorage"),
			filepath.Join(home, "AppData", "Roaming", "Antigravity IDE", "User", "workspaceStorage"),
			filepath.Join(home, ".config", "Antigravity", "User", "workspaceStorage"),
			filepath.Join(home, ".config", "Antigravity IDE", "User", "workspaceStorage"),
		)
	}
	return paths
}

// fetchProjectsFromWorkspaceStorage reads all workspace.json files from User/workspaceStorage.
// This is pure JSON on disk without requiring SQLite or Python.
func fetchProjectsFromWorkspaceStorage() []ProjectItem {
	var items []ProjectItem
	seenRoots := make(map[string]bool)
	seenURIs := make(map[string]bool)

	for _, wsRoot := range getAntigravityWorkspaceStoragePaths() {
		cleanRoot := filepath.Clean(wsRoot)
		if seenRoots[cleanRoot] {
			continue
		}
		seenRoots[cleanRoot] = true

		entries, err := os.ReadDir(cleanRoot)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			wsFile := filepath.Join(cleanRoot, entry.Name(), "workspace.json")
			fi, err := os.Stat(wsFile)
			if err != nil {
				continue
			}

			data, err := os.ReadFile(wsFile)
			if err != nil || len(data) == 0 {
				continue
			}

			var wsObj struct {
				Folder    string `json:"folder"`
				Workspace string `json:"workspace"`
			}
			if err := json.Unmarshal(data, &wsObj); err != nil {
				continue
			}

			rawURI := strings.TrimSpace(wsObj.Folder)
			isWorkspace := false
			if rawURI == "" && wsObj.Workspace != "" {
				rawURI = strings.TrimSpace(wsObj.Workspace)
				isWorkspace = true
			}
			if rawURI == "" {
				continue
			}

			norm := normalizeURI(rawURI)
			if norm == "" || seenURIs[norm] {
				continue
			}

			parsedPath := uriToPath(norm)
			if parsedPath == "" {
				continue
			}

			if !isRemoteURI(norm) {
				if fiPath, err := os.Stat(parsedPath); err != nil {
					continue
				} else if !isWorkspace && !fiPath.IsDir() {
					continue
				}
			}

			name := filepath.Base(parsedPath)
			if isWorkspace {
				name = strings.TrimSuffix(name, ".code-workspace")
			}
			if name == "" || name == "/" || name == "\\" || name == "." {
				name = filepath.Base(parsedPath)
			}
			if isRemoteURI(norm) {
				name += " (Remote)"
			}

			modTime := fi.ModTime()
			seenURIs[norm] = true
			items = append(items, ProjectItem{
				Name:        name,
				URI:         norm,
				Path:        parsedPath,
				IsWorkspace: isWorkspace,
				LastActive:  &modTime,
			})
		}
	}

	return items
}

func getAntigravityStateDBPaths() []string {
	var paths []string
	if appData := os.Getenv("APPDATA"); appData != "" {
		paths = append(paths,
			filepath.Join(appData, "Antigravity", "User", "globalStorage", "state.vscdb"),
			filepath.Join(appData, "Antigravity IDE", "User", "globalStorage", "state.vscdb"),
		)
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(home, "Library", "Application Support", "Antigravity", "User", "globalStorage", "state.vscdb"),
			filepath.Join(home, "Library", "Application Support", "Antigravity IDE", "User", "globalStorage", "state.vscdb"),
			filepath.Join(home, "AppData", "Roaming", "Antigravity", "User", "globalStorage", "state.vscdb"),
			filepath.Join(home, "AppData", "Roaming", "Antigravity IDE", "User", "globalStorage", "state.vscdb"),
		)
	}
	return paths
}

// fetchProjectsFromStateDB queries recentlyOpenedPathsList from Antigravity's state.vscdb.
func fetchProjectsFromStateDB() []ProjectItem {
	var dbPath string
	for _, p := range getAntigravityStateDBPaths() {
		if _, err := os.Stat(p); err == nil {
			dbPath = p
			break
		}
	}
	if dbPath == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	var out []byte
	var err error

	// 1. Try sqlite3 CLI if available
	if _, lookErr := exec.LookPath("sqlite3"); lookErr == nil {
		cmd := exec.CommandContext(ctx, "sqlite3", dbPath, "SELECT value FROM ItemTable WHERE key = 'history.recentlyOpenedPathsList';")
		out, err = cmd.Output()
	}

	// 2. Fallback to Python's built-in sqlite3 module if sqlite3 CLI is absent (e.g. Windows)
	if len(out) == 0 {
		pyScript := "import sqlite3, sys\n" +
			"try:\n" +
			"    conn = sqlite3.connect('file:' + sys.argv[1] + '?mode=ro', uri=True)\n" +
			"except Exception:\n" +
			"    conn = sqlite3.connect(sys.argv[1])\n" +
			"cur = conn.cursor()\n" +
			"cur.execute('SELECT value FROM ItemTable WHERE key = ?', (sys.argv[2],))\n" +
			"row = cur.fetchone()\n" +
			"sys.stdout.write(row[0] if row and row[0] else '')\n"
		for _, pyExe := range []string{"python", "python3"} {
			if _, lookErr := exec.LookPath(pyExe); lookErr == nil {
				cmd := exec.CommandContext(ctx, pyExe, "-c", pyScript, dbPath, "history.recentlyOpenedPathsList")
				if pyOut, pyErr := cmd.Output(); pyErr == nil && len(pyOut) > 0 {
					out = pyOut
					err = nil
					break
				}
			}
		}
	}

	if err != nil || len(out) == 0 {
		return nil
	}

	var hist vscdbHistory
	if err := json.Unmarshal(out, &hist); err != nil {
		return nil
	}

	var items []ProjectItem
	for _, entry := range hist.Entries {
		rawURI := ""
		isWorkspace := false

		if entry.FolderURI != "" {
			rawURI = entry.FolderURI
		} else if entry.Workspace != nil && entry.Workspace.ConfigPath != "" {
			rawURI = entry.Workspace.ConfigPath
			isWorkspace = true
		}

		if rawURI == "" {
			continue
		}

		path := uriToPath(rawURI)
		if path == "" {
			continue
		}

		if !isRemoteURI(rawURI) {
			if fi, err := os.Stat(path); err != nil {
				continue
			} else if !isWorkspace && !fi.IsDir() {
				continue
			}
		}

		name := filepath.Base(path)
		if isWorkspace {
			name = strings.TrimSuffix(name, ".code-workspace")
		}
		if isRemoteURI(rawURI) {
			name += " (Remote)"
		}

		items = append(items, ProjectItem{
			Name:        name,
			URI:         normalizeURI(rawURI),
			Path:        path,
			IsWorkspace: isWorkspace,
		})
	}

	return items
}

type upstreamTrajectorySummaryItem struct {
	LastModifiedTime   string `json:"lastModifiedTime"`
	TrajectoryMetadata *struct {
		WorkspaceUris        []string    `json:"workspaceUris"`
		ParentConversationID string      `json:"parentConversationId,omitempty"`
		SubagentSpec         interface{} `json:"subagentSpec,omitempty"`
		AgentScript          interface{} `json:"agentScript,omitempty"`
		NestingDepth         int         `json:"nestingDepth,omitempty"`
		RootConversationID   string      `json:"rootConversationId,omitempty"`
	} `json:"trajectoryMetadata"`
	Workspaces []struct {
		WorkspaceFolderAbsoluteUri string `json:"workspaceFolderAbsoluteUri"`
	} `json:"workspaces"`
}

type upstreamTrajectoriesResp struct {
	TrajectorySummaries map[string]upstreamTrajectorySummaryItem `json:"trajectorySummaries"`
}

func (p *Proxy) fetchTrajectoriesSummary(port int, token string) (map[string]upstreamTrajectorySummaryItem, error) {
	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", port)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
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

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	var reader io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		if gz, err := GetGzipReader(resp.Body); err == nil {
			defer PutGzipReader(gz)
			reader = gz
		}
	}

	var data upstreamTrajectoriesResp
	if err := json.NewDecoder(reader).Decode(&data); err != nil {
		return nil, err
	}

	if data.TrajectorySummaries != nil {
		for cid := range data.TrajectorySummaries {
			if IsDeletedCascade(cid) {
				delete(data.TrajectorySummaries, cid)
			}
		}
	}

	return data.TrajectorySummaries, nil
}

// HandleProjects handles GET /gateway/projects.
func (p *Proxy) HandleProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projects, err := p.GetProjects()
	if err != nil {
		errBytes, _ := json.Marshal(map[string]string{"error": err.Error()})
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(errBytes)))
		w.WriteHeader(http.StatusInternalServerError)
		w.Write(errBytes)
		return
	}

	data, err := json.Marshal(projects)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// CreateCascadeRequest represents payload to start a new cascade.
type CreateCascadeRequest struct {
	WorkspaceURI string `json:"workspaceUri"`
	Prompt       string `json:"prompt"`
	Model        string `json:"model,omitempty"`
	ProjectID    string `json:"projectId,omitempty"`
}

// CreateCascadeResponse represents result of starting a new cascade.
type CreateCascadeResponse struct {
	CascadeID string `json:"cascadeId"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}

// HandleCreateCascade handles POST /gateway/cascade/new.
func (p *Proxy) HandleCreateCascade(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// CSRF Protection: Validate Origin or Referer if present
	origin := r.Header.Get("Origin")
	if origin == "" {
		if ref := r.Header.Get("Referer"); ref != "" {
			if u, err := url.Parse(ref); err == nil {
				origin = fmt.Sprintf("%s://%s", u.Scheme, u.Host)
			}
		}
	}
	if origin != "" && !IsAllowedOrigin(origin, r.Host) {
		log.Printf("[Proxy] Rejected CreateCascade from untrusted origin: %s (host: %s)", origin, r.Host)
		http.Error(w, "Forbidden: untrusted origin", http.StatusForbidden)
		return
	}

	p.mu.RLock()
	port := p.activePort
	token := p.activeToken
	p.mu.RUnlock()

	if port == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(CreateCascadeResponse{
			Status: "error",
			Error:  "Antigravity language_server not connected",
		})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024) // 1MB limit to prevent DoS
	var req CreateCascadeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	clientMsgID := strings.TrimSpace(r.Header.Get("X-Client-Message-Id"))
	if clientMsgID == "" {
		clientMsgID = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	if clientMsgID != "" {
		if cachedID := p.getCascadeDedup(clientMsgID, 60*time.Second); cachedID != "" {
			log.Printf("[Proxy] Deduplicated repeat CreateCascade via clientMsgID %s -> cascade %s", clientMsgID, cachedID)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(CreateCascadeResponse{
				CascadeID: cachedID,
				Status:    "ok",
			})
			return
		}
	}

	wsURI := req.WorkspaceURI
	if wsURI != "" && !strings.HasPrefix(wsURI, "file://") {
		wsURI = "file://" + filepath.Clean(wsURI)
	}

	// Determine projectId: prefer explicitly provided projectId, otherwise match against known projects
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" && wsURI != "" {
		if projects, err := p.GetProjects(); err == nil {
			targetNorm := normalizeURI(wsURI)
			targetPath := uriToPath(targetNorm)
			for _, prj := range projects {
				if prj.ID == "" {
					continue
				}
				if normalizeURI(prj.URI) == targetNorm || uriToPath(prj.URI) == targetPath || filepath.Clean(prj.Path) == filepath.Clean(targetPath) {
					projectID = prj.ID
					break
				}
			}
		}
	}

	// 1. Call StartCascade RPC upstream
	startPayload := map[string]interface{}{}
	if projectID != "" {
		// When project environment config is provided, language_server strictly requires
		// workspaceUris to be empty ([]), otherwise it errors with invalid_argument.
		startPayload["source"] = "CORTEX_TRAJECTORY_SOURCE_CASCADE_CLIENT"
		startPayload["workspaceUris"] = []string{}
		startPayload["projectEnvConfig"] = map[string]interface{}{
			"projectId":                 projectID,
			"defaultProjectEnvironment": map[string]interface{}{},
		}
	} else if wsURI == "" || wsURI == "file://" {
		// Pure conversation (Chat / outside of project)
		startPayload["source"] = "CORTEX_TRAJECTORY_SOURCE_CASCADE_CLIENT"
		startPayload["workspaceUris"] = []string{}
		startPayload["projectEnvConfig"] = map[string]interface{}{
			"projectId":                 "outside-of-project",
			"defaultProjectEnvironment": map[string]interface{}{},
		}
	} else {
		startPayload["source"] = "CORTEX_TRAJECTORY_SOURCE_INTERACTIVE_CASCADE"
		startPayload["workspaceUris"] = []string{wsURI}
	}

	modelEnum := resolveModelEnum(req.Model)
	if modelEnum != "" {
		startPayload["requestedModel"] = modelEnum
	}

	startBytes, _ := json.Marshal(startPayload)
	startURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/StartCascade", port)

	httpReq, err := http.NewRequest(http.MethodPost, startURL, bytes.NewReader(startBytes))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		httpReq.Header.Set("x-codeium-csrf-token", token)
	}

	startResp, err := p.mediumClient.Do(httpReq)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(CreateCascadeResponse{
			Status: "error",
			Error:  fmt.Sprintf("Failed to call StartCascade: %v", err),
		})
		return
	}
	defer startResp.Body.Close()

	if startResp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(startResp.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(startResp.StatusCode)
		json.NewEncoder(w).Encode(CreateCascadeResponse{
			Status: "error",
			Error:  fmt.Sprintf("Upstream StartCascade error (%d): %s", startResp.StatusCode, string(b)),
		})
		return
	}

	var startResult struct {
		CascadeID string `json:"cascadeId"`
	}
	if err := json.NewDecoder(startResp.Body).Decode(&startResult); err != nil || startResult.CascadeID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(CreateCascadeResponse{
			Status: "error",
			Error:  "Upstream StartCascade returned empty cascadeId",
		})
		return
	}

	cascadeID := startResult.CascadeID
	if clientMsgID != "" {
		p.setCascadeDedup(clientMsgID, cascadeID)
	}
	log.Printf("[Proxy] Created new cascade: %s (projectId: %s) for workspace: %s", cascadeID, projectID, wsURI)

	// Update lastUserViewTime annotation upstream so desktop client recognizes it immediately
	annPayload := map[string]interface{}{
		"cascadeId": cascadeID,
		"annotations": map[string]interface{}{
			"lastUserViewTime": time.Now().UTC().Format("2006-01-02T15:04:05.999Z"),
		},
	}
	if annBytes, err := json.Marshal(annPayload); err == nil {
		annURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/UpdateConversationAnnotations", port)
		if annReq, err := http.NewRequest(http.MethodPost, annURL, bytes.NewReader(annBytes)); err == nil {
			annReq.Header.Set("Content-Type", "application/json")
			annReq.Header.Set("Connect-Protocol-Version", "1")
			if token != "" {
				annReq.Header.Set("x-codeium-csrf-token", token)
			}
			if annResp, err := p.mediumClient.Do(annReq); err == nil {
				annResp.Body.Close()
			}
		}
	}

	// 2. If prompt is provided, dispatch the initial user message. If empty, simply return cascadeId.
	if prompt := strings.TrimSpace(req.Prompt); prompt != "" {
		msgPayload := map[string]interface{}{
			"cascadeId": cascadeID,
			"items": []map[string]string{
				{"text": prompt},
			},
		}

		var cfgObj interface{}
		if cfg := p.GetCascadeConfig(cascadeID, port, token); len(cfg) > 0 {
			_ = json.Unmarshal(cfg, &cfgObj)
		}
		if modelEnum != "" {
			canonicalName := canonicalModelName(req.Model)
			if canonicalName == "" {
				canonicalName = req.Model
			}
			cfgObj = applyModelToCascadeConfig(cfgObj, modelEnum, canonicalName)
			if cfgBytes, err := json.Marshal(cfgObj); err == nil {
				SetCascadeModel(cascadeID, canonicalName, cfgBytes)
			}
		}
		if cfgObj != nil {
			msgPayload["cascadeConfig"] = cfgObj
		}

		msgBytes, _ := json.Marshal(msgPayload)
		msgURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", port)

		msgReq, err := http.NewRequest(http.MethodPost, msgURL, bytes.NewReader(msgBytes))
		if err == nil {
			msgReq.Header.Set("Content-Type", "application/json")
			msgReq.Header.Set("Connect-Protocol-Version", "1")
			if token != "" {
				msgReq.Header.Set("x-codeium-csrf-token", token)
			}
			if msgResp, err := p.mediumClient.Do(msgReq); err == nil {
				msgResp.Body.Close()
				ClearTrajectoryCache(cascadeID)
				log.Printf("[Proxy] Dispatched initial prompt to cascade %s", cascadeID)
			} else {
				log.Printf("[Proxy] Warning: failed to dispatch initial prompt: %v", err)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CreateCascadeResponse{
		CascadeID: cascadeID,
		Status:    "ok",
	})
}

func isRemoteURI(uri string) bool {
	return strings.HasPrefix(uri, "vscode-remote://") || strings.HasPrefix(uri, "ssh://")
}

func normalizeURI(uri string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return ""
	}

	if isRemoteURI(uri) {
		return strings.TrimSuffix(uri, "/")
	}

	// Fix 2-slash file://d:/... -> file:///d:/...
	if strings.HasPrefix(uri, "file://") && !strings.HasPrefix(uri, "file:///") {
		rest := strings.TrimPrefix(uri, "file://")
		if len(rest) >= 2 && isWindowsDriveLetter(rest[0]) && rest[1] == ':' {
			uri = "file:///" + rest
		}
	}

	// Unescape %3A or any percent-encoded characters (like %20 or %E9...)
	if strings.Contains(uri, "%") {
		if unescaped, err := url.PathUnescape(uri); err == nil {
			uri = unescaped
		}
	}

	if strings.HasPrefix(uri, "file://") {
		clean := strings.TrimPrefix(uri, "file://")
		clean = filepath.ToSlash(clean)
		if runtime.GOOS == "windows" {
			if len(clean) > 2 && clean[0] == '/' && isWindowsDriveLetter(clean[1]) && clean[2] == ':' {
				clean = clean[1:]
			}
			if len(clean) >= 2 && isWindowsDriveLetter(clean[0]) && clean[1] == ':' {
				clean = strings.ToLower(string(clean[0])) + clean[1:]
				return "file:///" + strings.TrimSuffix(clean, "/")
			}
		}
		return strings.TrimSuffix("file://"+clean, "/")
	}

	// Direct Windows drive path like D:\Projects...
	if runtime.GOOS == "windows" && len(uri) >= 2 && isWindowsDriveLetter(uri[0]) && uri[1] == ':' {
		uri = strings.ToLower(string(uri[0])) + uri[1:]
		return "file:///" + filepath.ToSlash(strings.TrimSuffix(uri, "\\/"))
	}

	if strings.HasPrefix(uri, "/") {
		uri = "file://" + uri
	}
	return strings.TrimSuffix(uri, "/")
}

func uriToPath(rawURI string) string {
	rawURI = strings.TrimSpace(rawURI)
	if rawURI == "" {
		return ""
	}

	if isRemoteURI(rawURI) {
		u, err := url.Parse(rawURI)
		if err == nil && u.Path != "" {
			if unescaped, uErr := url.PathUnescape(u.Path); uErr == nil {
				return unescaped
			}
			return u.Path
		}
		return rawURI
	}

	// Fix 2-slash file://d:/... -> file:///d:/...
	if strings.HasPrefix(rawURI, "file://") && !strings.HasPrefix(rawURI, "file:///") {
		rest := strings.TrimPrefix(rawURI, "file://")
		if len(rest) >= 2 && isWindowsDriveLetter(rest[0]) && rest[1] == ':' {
			rawURI = "file:///" + rest
		}
	}

	if !strings.HasPrefix(rawURI, "file://") {
		if strings.Contains(rawURI, "%") {
			if unescaped, err := url.PathUnescape(rawURI); err == nil {
				rawURI = unescaped
			}
		}
		return filepath.Clean(rawURI)
	}

	u, err := url.Parse(rawURI)
	var path string
	if err != nil {
		path = strings.TrimPrefix(rawURI, "file://")
	} else {
		p, uErr := url.PathUnescape(u.Path)
		if uErr == nil {
			path = p
		} else {
			path = u.Path
		}
	}

	if strings.Contains(path, "%") {
		if unescaped, err := url.PathUnescape(path); err == nil {
			path = unescaped
		}
	}

	if runtime.GOOS == "windows" {
		if len(path) > 2 && (path[0] == '/' || path[0] == '\\') && isWindowsDriveLetter(path[1]) && path[2] == ':' {
			path = path[1:]
		}
		if len(path) >= 2 && isWindowsDriveLetter(path[0]) && path[1] == ':' {
			path = strings.ToLower(string(path[0])) + path[1:]
		}
		return filepath.FromSlash(path)
	}
	return filepath.Clean(path)
}

// modelEnumMap maps friendly model IDs or aliases to upstream Protobuf enum names.
var modelEnumMap = map[string]string{
	"gemini":                   "MODEL_PLACEHOLDER_M318",
	"gemini-flash":             "MODEL_PLACEHOLDER_M318",
	"gemini-3.8-flash":         "MODEL_PLACEHOLDER_M318",
	"gemini-3.8-flash-high":    "MODEL_PLACEHOLDER_M318",
	"gemini-3.8-flash-medium":  "MODEL_PLACEHOLDER_M319",
	"gemini-3.8-flash-low":     "MODEL_PLACEHOLDER_M320",
	"gemini-3.7-flash-high":    "MODEL_PLACEHOLDER_M298",
	"gemini-3.7-flash-medium":  "MODEL_PLACEHOLDER_M299",
	"gemini-3.7-flash-low":     "MODEL_PLACEHOLDER_M300",
	"gemini-3.6-flash-high":    "MODEL_PLACEHOLDER_M71",
	"gemini-3.6-flash-medium":  "MODEL_PLACEHOLDER_M72",
	"gemini-3.6-flash-low":     "MODEL_PLACEHOLDER_M73",
	"gemini-pro-agent":         "MODEL_PLACEHOLDER_M16",
	"gemini-3.1-pro-low":       "MODEL_PLACEHOLDER_M36",
	"gemini-3.1-pro-high":      "MODEL_PLACEHOLDER_M37",
	"gemini-2.5-pro":           "MODEL_PLACEHOLDER_M318",
	"gemini-2.5-flash":         "MODEL_PLACEHOLDER_M318",
	"claude":                   "MODEL_PLACEHOLDER_M26",
	"claude-opus":              "MODEL_PLACEHOLDER_M26",
	"claude-opus-4-6-thinking": "MODEL_PLACEHOLDER_M26",
	"claude-sonnet-4-6":        "MODEL_PLACEHOLDER_M35",
	"claude-3-7-sonnet":        "MODEL_PLACEHOLDER_M26",
	"claude-3-5-sonnet":        "MODEL_PLACEHOLDER_M26",
	"gpt-oss-120b-medium":      "MODEL_OPENAI_GPT_OSS_120B_MEDIUM",
}

// resolveModelEnum resolves a user or client provided model name to its protobuf enum string.
func resolveModelEnum(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if strings.HasPrefix(model, "MODEL_") {
		if model == "MODEL_GOOGLE_GEMINI_2_5_PRO" || model == "MODEL_GOOGLE_GEMINI_2_5_FLASH" {
			return "MODEL_PLACEHOLDER_M318"
		}
		return model
	}
	if enum, ok := modelEnumMap[strings.ToLower(model)]; ok {
		return enum
	}
	return ""
}

// enumToCanonicalMap maps upstream Protobuf enum names to canonical model identifiers.
var enumToCanonicalMap = map[string]string{
	"MODEL_PLACEHOLDER_M26":            "claude-opus-4-6-thinking",
	"MODEL_PLACEHOLDER_M35":            "claude-sonnet-4-6",
	"MODEL_PLACEHOLDER_M318":           "gemini-3.8-flash-high",
	"MODEL_PLACEHOLDER_M319":           "gemini-3.8-flash-medium",
	"MODEL_PLACEHOLDER_M320":           "gemini-3.8-flash-low",
	"MODEL_PLACEHOLDER_M298":           "gemini-3.7-flash-high",
	"MODEL_PLACEHOLDER_M299":           "gemini-3.7-flash-medium",
	"MODEL_PLACEHOLDER_M300":           "gemini-3.7-flash-low",
	"MODEL_PLACEHOLDER_M71":            "gemini-3.6-flash-high",
	"MODEL_PLACEHOLDER_M72":            "gemini-3.6-flash-medium",
	"MODEL_PLACEHOLDER_M73":            "gemini-3.6-flash-low",
	"MODEL_PLACEHOLDER_M16":            "gemini-pro-agent",
	"MODEL_PLACEHOLDER_M36":            "gemini-3.1-pro-low",
	"MODEL_PLACEHOLDER_M37":            "gemini-3.1-pro-high",
	"MODEL_GOOGLE_GEMINI_2_5_PRO":      "gemini-3.8-flash-high",
	"MODEL_GOOGLE_GEMINI_2_5_FLASH":    "gemini-3.8-flash-high",
	"MODEL_OPENAI_GPT_OSS_120B_MEDIUM": "gpt-oss-120b-medium",
}

// canonicalModelName converts a model enum or friendly alias into its canonical model name.
func canonicalModelName(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if name, ok := enumToCanonicalMap[model]; ok {
		return name
	}
	enum := resolveModelEnum(model)
	if name, ok := enumToCanonicalMap[enum]; ok {
		return name
	}
	return model
}

// applyModelToCascadeConfig patches or injects the given modelEnum and modelName into cfgObj,
// and ensures checkpointConfig limits comply with model context window constraints.
func applyModelToCascadeConfig(cfgObj interface{}, modelEnum string, modelName string) interface{} {
	if modelEnum == "" {
		return cfgObj
	}
	if modelName == "" {
		modelName = canonicalModelName(modelEnum)
	}

	cfgMap, ok := cfgObj.(map[string]interface{})
	if !ok || cfgMap == nil {
		cfgMap = make(map[string]interface{})
	}

	var plannerConfig map[string]interface{}
	if p, ok := cfgMap["plannerConfig"].(map[string]interface{}); ok && p != nil {
		plannerConfig = p
	} else {
		plannerConfig = make(map[string]interface{})
	}

	plannerConfig["planModel"] = modelEnum
	plannerConfig["requestedModel"] = map[string]interface{}{
		"model": modelEnum,
	}
	plannerConfig["modelName"] = modelName
	cfgMap["plannerConfig"] = plannerConfig

	// Adjust checkpointConfig to match model's context window constraints.
	// Gemini: 1M context, allows maxTokenLimit: 256000, tokenThreshold: 140000.
	// Claude Opus 4.6 Thinking: 250k context, maxOutputTokens: 64000.
	// LanguageServer asserts: maxTokenLimit <= ContextWindow (250000) - MaxOutputTokens (64000) = 186000.
	// Desktop Antigravity uses maxTokenLimit: 160000, tokenThreshold: 50000 for Claude.
	isClaude := strings.Contains(strings.ToLower(modelEnum), "claude") ||
		strings.Contains(strings.ToLower(modelName), "claude") ||
		modelEnum == "MODEL_PLACEHOLDER_M26"

	var checkpointConfig map[string]interface{}
	if cp, ok := cfgMap["checkpointConfig"].(map[string]interface{}); ok && cp != nil {
		checkpointConfig = cp
	} else {
		checkpointConfig = make(map[string]interface{})
	}

	if isClaude {
		checkpointConfig["maxTokenLimit"] = 160000
		checkpointConfig["tokenThreshold"] = 50000
		checkpointConfig["isSync"] = false
		checkpointConfig["useLastPlannerModel"] = false
	} else {
		// Restore Gemini limits if previously clamped
		if limit, ok := getNumberAsInt(checkpointConfig["maxTokenLimit"]); ok && limit <= 160000 {
			checkpointConfig["maxTokenLimit"] = 256000
		}
		if thresh, ok := getNumberAsInt(checkpointConfig["tokenThreshold"]); ok && thresh <= 50000 {
			checkpointConfig["tokenThreshold"] = 140000
		}
		checkpointConfig["isSync"] = true
		checkpointConfig["useLastPlannerModel"] = true
	}
	cfgMap["checkpointConfig"] = checkpointConfig

	return cfgMap
}

func getNumberAsInt(val interface{}) (int, bool) {
	if val == nil {
		return 0, false
	}
	switch v := val.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

