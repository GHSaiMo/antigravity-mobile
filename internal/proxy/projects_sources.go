package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

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

// getGeminiConfigProjectsDirs returns directories where Antigravity 2.0 / Gemini project configs reside.
func getGeminiConfigProjectsDirs() []string {
	var dirs []string
	if envAppDir := os.Getenv("ANTIGRAVITY_APP_DATA_DIR"); envAppDir != "" {
		parent := filepath.Dir(envAppDir)
		dirs = append(dirs, filepath.Join(parent, "config", "projects"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs,
			filepath.Join(home, ".gemini", "config", "projects"),
		)
	}
	if appData := os.Getenv("APPDATA"); appData != "" {
		dirs = append(dirs,
			filepath.Join(appData, "Gemini", "config", "projects"),
			filepath.Join(appData, ".gemini", "config", "projects"),
			filepath.Join(appData, "Antigravity", "config", "projects"),
		)
	}
	return dirs
}

type geminiProjectConfigFile struct {
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
}

// fetchProjectsFromGeminiConfig reads Antigravity 2.0 project JSON configs from ~/.gemini/config/projects/.
func fetchProjectsFromGeminiConfig() []ProjectItem {
	items := make([]ProjectItem, 0, 16)
	seenIDs := make(map[string]bool)
	seenURIs := make(map[string]bool)

	for _, dir := range getGeminiConfigProjectsDirs() {
		cleanDir := filepath.Clean(dir)
		entries, err := os.ReadDir(cleanDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
				continue
			}
			if strings.EqualFold(entry.Name(), "outside-of-project.json") {
				continue
			}

			filePath := filepath.Join(cleanDir, entry.Name())
			fi, err := os.Stat(filePath)
			if err != nil {
				continue
			}

			data, err := os.ReadFile(filePath)
			if err != nil || len(data) == 0 {
				continue
			}

			var prj geminiProjectConfigFile
			if err := json.Unmarshal(data, &prj); err != nil || prj.ID == "" || prj.ID == "outside-of-project" {
				continue
			}

			if seenIDs[prj.ID] {
				continue
			}

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

			if uri == "" {
				continue
			}

			norm := normalizeURI(uri)
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
				} else if !prj.IsWorkspaceOnly && !fiPath.IsDir() {
					continue
				}
			}

			name := prj.Name
			if name == "" {
				name = filepath.Base(parsedPath)
			}
			if isRemoteURI(norm) && !strings.Contains(name, "(Remote)") {
				name += " (Remote)"
			}

			modTime := fi.ModTime()
			seenIDs[prj.ID] = true
			seenURIs[norm] = true

			items = append(items, ProjectItem{
				ID:          prj.ID,
				Name:        name,
				URI:         norm,
				Path:        parsedPath,
				IsWorkspace: prj.IsWorkspaceOnly,
				LastActive:  &modTime,
			})
		}
	}

	return items
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

	var storageMap map[string]interface{}
	if len(storageBytes) > 0 {
		_ = json.Unmarshal(storageBytes, &storageMap)
	}

	var order []string
	if storageMap != nil {
		if rawOrder, ok := storageMap["projectsOrder"].(string); ok && rawOrder != "" {
			_ = json.Unmarshal([]byte(rawOrder), &order)
		}
	}

	seenOrder := make(map[string]bool)
	for _, id := range order {
		seenOrder[id] = true
	}

	// Also discover project IDs from other app_storage.json keys (e.g. lastCreatedProjectId, new-convo-last-selected-project)
	if storageMap != nil {
		for k, v := range storageMap {
			strVal, ok := v.(string)
			if !ok || strVal == "" || strVal == "outside-of-project" {
				continue
			}
			if strings.Contains(strings.ToLower(k), "project") && len(strVal) == 36 && strings.Count(strVal, "-") == 4 {
				if !seenOrder[strVal] {
					seenOrder[strVal] = true
					order = append(order, strVal)
				}
			}
		}
	}

	// Also collect IDs from Gemini config files
	for _, cfgPrj := range fetchProjectsFromGeminiConfig() {
		if cfgPrj.ID != "" && cfgPrj.ID != "outside-of-project" && !seenOrder[cfgPrj.ID] {
			seenOrder[cfgPrj.ID] = true
			order = append(order, cfgPrj.ID)
		}
	}

	if len(order) == 0 {
		return nil, fmt.Errorf("no project IDs found")
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
	items := make([]ProjectItem, 0, 16)
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

var (
	cachedStateDBMu    sync.RWMutex
	cachedStateDBPath  string
	cachedStateDBMTime time.Time
	cachedStateDBItems []ProjectItem
)

// fetchProjectsFromStateDB queries recentlyOpenedPathsList from Antigravity's state.vscdb.
func fetchProjectsFromStateDB() []ProjectItem {
	var dbPath string
	var dbFi os.FileInfo
	for _, p := range getAntigravityStateDBPaths() {
		if fi, err := os.Stat(p); err == nil {
			dbPath = p
			dbFi = fi
			break
		}
	}
	if dbPath == "" || dbFi == nil {
		return nil
	}

	cachedStateDBMu.RLock()
	if cachedStateDBPath == dbPath && cachedStateDBMTime.Equal(dbFi.ModTime()) && cachedStateDBItems != nil {
		items := make([]ProjectItem, len(cachedStateDBItems))
		copy(items, cachedStateDBItems)
		cachedStateDBMu.RUnlock()
		return items
	}
	cachedStateDBMu.RUnlock()

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

	items := make([]ProjectItem, 0, len(hist.Entries))
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

	cachedStateDBMu.Lock()
	cachedStateDBPath = dbPath
	cachedStateDBMTime = dbFi.ModTime()
	cachedStateDBItems = make([]ProjectItem, len(items))
	copy(cachedStateDBItems, items)
	cachedStateDBMu.Unlock()

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
