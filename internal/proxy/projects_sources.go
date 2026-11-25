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
	"runtime"
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
			filepath.Join(home, ".config", "Antigravity", "app_storage.json"),
			filepath.Join(home, ".config", "Antigravity IDE", "app_storage.json"),
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

var (
	geminiCfgCacheMu           sync.RWMutex
	cachedSidecarProjStamp     string
	cachedSidecarProjAt        time.Time
	cachedSidecarProjIDs       map[string]bool
	cachedGeminiCfgProjStamp   string
	cachedGeminiCfgProjAt      time.Time
	cachedGeminiCfgProjects    []ProjectItem
	cachedGeminiAutoRootStamp  string
	cachedGeminiAutoRootAt     time.Time
	cachedGeminiAutoRoots      []string
	cachedWSStorageRootStamp   string
	cachedWSStorageRootAt      time.Time
	cachedWSStorageAutoRoots   []string
)

func invalidateGeminiConfigCaches() {
	geminiCfgCacheMu.Lock()
	cachedSidecarProjStamp = ""
	cachedSidecarProjIDs = nil
	cachedGeminiCfgProjStamp = ""
	cachedGeminiCfgProjects = nil
	cachedGeminiAutoRootStamp = ""
	cachedGeminiAutoRoots = nil
	cachedWSStorageRootStamp = ""
	cachedWSStorageAutoRoots = nil
	geminiCfgCacheMu.Unlock()
}

// fetchSidecarProjectIDs reads Antigravity 2.0 config.json files to identify shadow project IDs
// automatically created for scheduled tasks / sidecars so they can be excluded from workspace lists.
func fetchSidecarProjectIDs() map[string]bool {
	dirs := getGeminiConfigProjectsDirs()
	var cfgFiles []string
	seenFiles := make(map[string]bool, len(dirs))
	var stampBuf strings.Builder
	for _, projDir := range dirs {
		cfgFile := filepath.Clean(filepath.Join(filepath.Dir(projDir), "config.json"))
		if seenFiles[cfgFile] {
			continue
		}
		seenFiles[cfgFile] = true
		cfgFiles = append(cfgFiles, cfgFile)
		stampBuf.WriteString(cfgFile)
		if fi, err := os.Stat(cfgFile); err == nil {
			fmt.Fprintf(&stampBuf, ":%d:%d|", fi.ModTime().UnixNano(), fi.Size())
		} else {
			stampBuf.WriteString(":missing|")
		}
	}
	stamp := stampBuf.String()
	now := time.Now()

	geminiCfgCacheMu.RLock()
	if cachedSidecarProjIDs != nil && cachedSidecarProjStamp == stamp && now.Sub(cachedSidecarProjAt) < 5*time.Second {
		cp := make(map[string]bool, len(cachedSidecarProjIDs))
		for k, v := range cachedSidecarProjIDs {
			cp[k] = v
		}
		geminiCfgCacheMu.RUnlock()
		return cp
	}
	geminiCfgCacheMu.RUnlock()

	ids := make(map[string]bool)
	for _, cfgFile := range cfgFiles {
		data, err := os.ReadFile(cfgFile)
		if err != nil || len(data) == 0 {
			continue
		}

		var cfg struct {
			Sidecars map[string]struct {
				ProjectID string `json:"projectId"`
			} `json:"sidecars"`
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			continue
		}

		for _, sc := range cfg.Sidecars {
			if pid := strings.TrimSpace(sc.ProjectID); pid != "" {
				ids[pid] = true
			}
		}
	}

	geminiCfgCacheMu.Lock()
	cachedSidecarProjStamp = stamp
	cachedSidecarProjAt = now
	cachedSidecarProjIDs = make(map[string]bool, len(ids))
	for k, v := range ids {
		cachedSidecarProjIDs[k] = v
	}
	geminiCfgCacheMu.Unlock()

	return ids
}

// fetchProjectsFromGeminiConfig reads Antigravity 2.0 project JSON configs from ~/.gemini/config/projects/.
func fetchProjectsFromGeminiConfig() []ProjectItem {
	dirs := getGeminiConfigProjectsDirs()
	sidecarIDs := fetchSidecarProjectIDs()

	var stampBuf strings.Builder
	for _, dir := range dirs {
		cleanDir := filepath.Clean(dir)
		stampBuf.WriteString(cleanDir)
		if fi, err := os.Stat(cleanDir); err == nil {
			fmt.Fprintf(&stampBuf, ":%d|", fi.ModTime().UnixNano())
		} else {
			stampBuf.WriteString(":missing|")
		}
	}
	fmt.Fprintf(&stampBuf, "#sc=%d", len(sidecarIDs))
	stamp := stampBuf.String()
	now := time.Now()

	geminiCfgCacheMu.RLock()
	if cachedGeminiCfgProjects != nil && cachedGeminiCfgProjStamp == stamp && now.Sub(cachedGeminiCfgProjAt) < 2*time.Second {
		cp := cloneProjectItems(cachedGeminiCfgProjects)
		geminiCfgCacheMu.RUnlock()
		return cp
	}
	geminiCfgCacheMu.RUnlock()

	items := make([]ProjectItem, 0, 16)
	seenIDs := make(map[string]bool)
	seenURIs := make(map[string]bool)

	for _, dir := range dirs {
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

			if sidecarIDs[prj.ID] || seenIDs[prj.ID] {
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
			if parsedPath == "" || isDriveRoot(parsedPath) {
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
			if name == "" || name == "/" || name == "\\" || name == "." {
				continue
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

	geminiCfgCacheMu.Lock()
	cachedGeminiCfgProjStamp = stamp
	cachedGeminiCfgProjAt = now
	cachedGeminiCfgProjects = cloneProjectItems(items)
	geminiCfgCacheMu.Unlock()

	return items
}

// fetchAutoWorkspaceRootsFromGeminiConfig collects every local folder path referenced by
// non-sidecar Antigravity project config JSONs (including multi-folder projects).
func fetchAutoWorkspaceRootsFromGeminiConfig() []string {
	dirs := getGeminiConfigProjectsDirs()
	sidecarIDs := fetchSidecarProjectIDs()

	var stampBuf strings.Builder
	for _, dir := range dirs {
		cleanDir := filepath.Clean(dir)
		stampBuf.WriteString(cleanDir)
		if fi, err := os.Stat(cleanDir); err == nil {
			fmt.Fprintf(&stampBuf, ":%d|", fi.ModTime().UnixNano())
		} else {
			stampBuf.WriteString(":missing|")
		}
	}
	fmt.Fprintf(&stampBuf, "#sc=%d", len(sidecarIDs))
	stamp := stampBuf.String()
	now := time.Now()

	geminiCfgCacheMu.RLock()
	if cachedGeminiAutoRoots != nil && cachedGeminiAutoRootStamp == stamp && now.Sub(cachedGeminiAutoRootAt) < 2*time.Second {
		cp := make([]string, len(cachedGeminiAutoRoots))
		copy(cp, cachedGeminiAutoRoots)
		geminiCfgCacheMu.RUnlock()
		return cp
	}
	geminiCfgCacheMu.RUnlock()

	roots := make([]string, 0, 8)
	seen := make(map[string]bool)
	addURI := func(rawURI string) {
		rawURI = strings.TrimSpace(rawURI)
		if rawURI == "" || isRemoteURI(rawURI) {
			return
		}
		norm := normalizeURI(rawURI)
		if norm == "" || isRemoteURI(norm) {
			return
		}
		p := uriToPath(norm)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		roots = append(roots, p)
	}

	for _, dir := range dirs {
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
			data, err := os.ReadFile(filepath.Join(cleanDir, entry.Name()))
			if err != nil || len(data) == 0 {
				continue
			}
			var prj geminiProjectConfigFile
			if err := json.Unmarshal(data, &prj); err != nil || prj.ID == "" || prj.ID == "outside-of-project" || sidecarIDs[prj.ID] {
				continue
			}
			if prj.ProjectResources == nil {
				continue
			}
			for _, r := range prj.ProjectResources.Resources {
				if r.FolderURI != "" {
					addURI(r.FolderURI)
				}
				if r.GitFolder != nil && r.GitFolder.FolderURI != "" {
					addURI(r.GitFolder.FolderURI)
				}
			}
		}
	}

	geminiCfgCacheMu.Lock()
	cachedGeminiAutoRootStamp = stamp
	cachedGeminiAutoRootAt = now
	cachedGeminiAutoRoots = make([]string, len(roots))
	copy(cachedGeminiAutoRoots, roots)
	geminiCfgCacheMu.Unlock()
	return roots
}

// fetchAutoWorkspaceRootsFromWorkspaceStorage collects local folder paths from
// User/workspaceStorage/*/workspace.json with a lightweight directory mtime cache.
func fetchAutoWorkspaceRootsFromWorkspaceStorage() []string {
	wsRoots := getAntigravityWorkspaceStoragePaths()

	var stampBuf strings.Builder
	seenDirs := make(map[string]bool, len(wsRoots))
	cleanRoots := make([]string, 0, len(wsRoots))
	for _, wsRoot := range wsRoots {
		cleanRoot := filepath.Clean(wsRoot)
		if seenDirs[cleanRoot] {
			continue
		}
		seenDirs[cleanRoot] = true
		cleanRoots = append(cleanRoots, cleanRoot)
		stampBuf.WriteString(cleanRoot)
		if fi, err := os.Stat(cleanRoot); err == nil {
			fmt.Fprintf(&stampBuf, ":%d|", fi.ModTime().UnixNano())
		} else {
			stampBuf.WriteString(":missing|")
		}
	}
	stamp := stampBuf.String()
	now := time.Now()

	geminiCfgCacheMu.RLock()
	if cachedWSStorageAutoRoots != nil && cachedWSStorageRootStamp == stamp && now.Sub(cachedWSStorageRootAt) < 5*time.Second {
		cp := make([]string, len(cachedWSStorageAutoRoots))
		copy(cp, cachedWSStorageAutoRoots)
		geminiCfgCacheMu.RUnlock()
		return cp
	}
	geminiCfgCacheMu.RUnlock()

	roots := make([]string, 0, 8)
	seen := make(map[string]bool)
	for _, cleanRoot := range cleanRoots {
		entries, err := os.ReadDir(cleanRoot)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(cleanRoot, entry.Name(), "workspace.json"))
			if err != nil || len(data) == 0 {
				continue
			}
			var wsObj struct {
				Folder string `json:"folder"`
			}
			if err := json.Unmarshal(data, &wsObj); err != nil {
				continue
			}
			rawURI := strings.TrimSpace(wsObj.Folder)
			if rawURI == "" || isRemoteURI(rawURI) {
				continue
			}
			norm := normalizeURI(rawURI)
			if norm == "" || isRemoteURI(norm) {
				continue
			}
			p := uriToPath(norm)
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			roots = append(roots, p)
		}
	}

	geminiCfgCacheMu.Lock()
	cachedWSStorageRootStamp = stamp
	cachedWSStorageRootAt = now
	cachedWSStorageAutoRoots = make([]string, len(roots))
	copy(cachedWSStorageAutoRoots, roots)
	geminiCfgCacheMu.Unlock()
	return roots
}

// syncDiscoveredWorkspaceRootsFromReadProjectsJSON extracts local project folder URIs
// from an upstream ReadProjects JSON response and registers them as discovered workspace roots.
func syncDiscoveredWorkspaceRootsFromReadProjectsJSON(raw []byte) {
	if len(raw) == 0 {
		return
	}
	var readResp struct {
		Projects []geminiProjectConfigFile `json:"projects"`
	}
	if err := json.Unmarshal(raw, &readResp); err != nil || len(readResp.Projects) == 0 {
		return
	}
	sidecarIDs := fetchSidecarProjectIDs()
	paths := make([]string, 0, len(readResp.Projects))
	for _, prj := range readResp.Projects {
		if prj.ID == "" || prj.ID == "outside-of-project" || sidecarIDs[prj.ID] || prj.ProjectResources == nil {
			continue
		}
		for _, r := range prj.ProjectResources.Resources {
			if r.FolderURI != "" && !isRemoteURI(r.FolderURI) {
				paths = append(paths, r.FolderURI)
			}
			if r.GitFolder != nil && r.GitFolder.FolderURI != "" && !isRemoteURI(r.GitFolder.FolderURI) {
				paths = append(paths, r.GitFolder.FolderURI)
			}
		}
	}
	syncDiscoveredWorkspaceRoots(paths)
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

	sidecarIDs := fetchSidecarProjectIDs()

	var rawOrderList []string
	if storageMap != nil {
		if rawOrder, ok := storageMap["projectsOrder"].(string); ok && rawOrder != "" {
			_ = json.Unmarshal([]byte(rawOrder), &rawOrderList)
		}
	}

	var order []string
	seenOrder := make(map[string]bool)
	for _, id := range rawOrderList {
		if id == "" || id == "outside-of-project" || sidecarIDs[id] || seenOrder[id] {
			continue
		}
		seenOrder[id] = true
		order = append(order, id)
	}

	// Also discover project IDs from other app_storage.json keys (e.g. lastCreatedProjectId, new-convo-last-selected-project)
	if storageMap != nil {
		for k, v := range storageMap {
			strVal, ok := v.(string)
			if !ok || strVal == "" || strVal == "outside-of-project" || sidecarIDs[strVal] {
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
		if cfgPrj.ID != "" && cfgPrj.ID != "outside-of-project" && !sidecarIDs[cfgPrj.ID] && !seenOrder[cfgPrj.ID] {
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
			if err == nil && resp != nil && resp.StatusCode != http.StatusOK {
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
				_ = resp.Body.Close()
			}
			if err == nil && resp != nil && resp.StatusCode == http.StatusOK {
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

					var allLocalResourceURIs []string
					for _, prj := range readResp.Projects {
						uri := ""
						if prj.ProjectResources != nil {
							for _, r := range prj.ProjectResources.Resources {
								if r.FolderURI != "" {
									if !isRemoteURI(r.FolderURI) {
										allLocalResourceURIs = append(allLocalResourceURIs, r.FolderURI)
									}
									if uri == "" {
										uri = r.FolderURI
									}
								}
								if r.GitFolder != nil && r.GitFolder.FolderURI != "" {
									if !isRemoteURI(r.GitFolder.FolderURI) {
										allLocalResourceURIs = append(allLocalResourceURIs, r.GitFolder.FolderURI)
									}
									if uri == "" {
										uri = r.GitFolder.FolderURI
									}
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
					syncDiscoveredWorkspaceRoots(allLocalResourceURIs)

					var orderedItems []ProjectItem
					for _, id := range order {
						if pInfo, exists := projMap[id]; exists {
							norm := normalizeURI(pInfo.uri)
							path := uriToPath(norm)
							if path == "" || isDriveRoot(path) {
								continue
							}
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
			if parsedPath == "" || isDriveRoot(parsedPath) {
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
				continue
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
	seen := make(map[string]bool)
	addPath := func(p string) {
		clean := filepath.Clean(p)
		if !seen[clean] {
			seen[clean] = true
			paths = append(paths, clean)
		}
	}

	if appData := os.Getenv("APPDATA"); appData != "" {
		addPath(filepath.Join(appData, "Antigravity", "User", "globalStorage", "state.vscdb"))
		addPath(filepath.Join(appData, "Antigravity IDE", "User", "globalStorage", "state.vscdb"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		if runtime.GOOS == "darwin" {
			addPath(filepath.Join(home, "Library", "Application Support", "Antigravity", "User", "globalStorage", "state.vscdb"))
			addPath(filepath.Join(home, "Library", "Application Support", "Antigravity IDE", "User", "globalStorage", "state.vscdb"))
		} else if runtime.GOOS == "windows" {
			addPath(filepath.Join(home, "AppData", "Roaming", "Antigravity", "User", "globalStorage", "state.vscdb"))
			addPath(filepath.Join(home, "AppData", "Roaming", "Antigravity IDE", "User", "globalStorage", "state.vscdb"))
		} else {
			configDir := os.Getenv("XDG_CONFIG_HOME")
			if configDir == "" {
				configDir = filepath.Join(home, ".config")
			}
			addPath(filepath.Join(configDir, "Antigravity", "User", "globalStorage", "state.vscdb"))
			addPath(filepath.Join(configDir, "Antigravity IDE", "User", "globalStorage", "state.vscdb"))
		}
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
	if safeDB, pathErr := safeSQLiteDBArg(dbPath); pathErr == nil {
		if bin, roFlag, lookErr := findSQLiteExecutable(); lookErr == nil {
			args := make([]string, 0, 4)
			if roFlag {
				args = append(args, "-readonly")
			}
			args = append(args, safeDB, "SELECT value FROM ItemTable WHERE key = 'history.recentlyOpenedPathsList';")
			cmd := exec.CommandContext(ctx, bin, args...)
			out, err = cmd.Output()
		} else if p, lookErr2 := exec.LookPath("sqlite3"); lookErr2 == nil && isExecutableValid(p) {
			cmd := exec.CommandContext(ctx, p, safeDB, "SELECT value FROM ItemTable WHERE key = 'history.recentlyOpenedPathsList';")
			out, err = cmd.Output()
		}
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
		for _, pyExe := range pythonCandidates() {
			p, lookErr := exec.LookPath(pyExe)
			if lookErr != nil || !isExecutableValid(p) {
				continue
			}
			cmd := exec.CommandContext(ctx, pyExe, "-c", pyScript, dbPath, "history.recentlyOpenedPathsList")
			if pyOut, pyErr := cmd.Output(); pyErr == nil && len(pyOut) > 0 {
				out = pyOut
				err = nil
				break
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
		if path == "" || isDriveRoot(path) {
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
		if name == "" || name == "/" || name == "\\" || name == "." {
			continue
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
		ProjectID            string      `json:"projectId,omitempty"`
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := p.fetchAllTrajectoriesRaw(ctx, port, token)
	if err != nil {
		return nil, err
	}

	var data upstreamTrajectoriesResp
	if err := json.Unmarshal(raw, &data); err != nil {
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
