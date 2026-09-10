package proxy

import (
	"bytes"
	"compress/gzip"
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
	"sort"
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
// Prioritizes official Projects order from app_storage.json and ReadProjects RPC.
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

	// 2. Try fetching official ordered Projects from app_storage.json + ReadProjects RPC
	if officialProjects, err := p.fetchOfficialProjects(port, token, sessionStats); err == nil && len(officialProjects) > 0 {
		return officialProjects, nil
	}

	// 3. Fallback: discover from workspace file or state.vscdb
	dbProjects := fetchProjectsFromStateDB()
	projectMap := make(map[string]*ProjectItem)

	for _, prj := range dbProjects {
		norm := normalizeURI(prj.URI)
		if stat, ok := sessionStats[norm]; ok {
			prj.SessionCount = stat.count
			if !stat.lastActive.IsZero() {
				t := stat.lastActive
				prj.LastActive = &t
			}
		}
		itemCopy := prj
		projectMap[norm] = &itemCopy
	}

	for norm, stat := range sessionStats {
		if _, exists := projectMap[norm]; !exists {
			parsedPath := uriToPath(norm)
			if parsedPath == "" {
				continue
			}
			if _, err := os.Stat(parsedPath); err != nil {
				continue
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

			projectMap[norm] = &ProjectItem{
				Name:         name,
				URI:          norm,
				Path:         parsedPath,
				IsWorkspace:  isWs,
				SessionCount: stat.count,
				LastActive:   lastActive,
			}
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

// fetchOfficialProjects loads projectsOrder and calls upstream ReadProjects to get the exact 21 projects in order.
func (p *Proxy) fetchOfficialProjects(port int, token string, sessionStats map[string]struct {
	count      int
	lastActive time.Time
}) ([]ProjectItem, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	appStoragePath := filepath.Join(home, "Library", "Application Support", "Antigravity", "app_storage.json")
	storageBytes, err := os.ReadFile(appStoragePath)
	if err != nil {
		return nil, err
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
		req, err := http.NewRequest(http.MethodPost, readURL, bytes.NewReader(reqBody))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Connect-Protocol-Version", "1")
			if token != "" {
				req.Header.Set("x-codeium-csrf-token", token)
			}
			client := &http.Client{Timeout: 5 * time.Second, Transport: p.transport}
			resp, err := client.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				defer resp.Body.Close()
				var reader io.Reader = resp.Body
				if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
					if gz, err := gzip.NewReader(resp.Body); err == nil {
						defer gz.Close()
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

// fetchProjectsFromStateDB queries recentlyOpenedPathsList from Antigravity's state.vscdb.
func fetchProjectsFromStateDB() []ProjectItem {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	dbPath := filepath.Join(home, "Library", "Application Support", "Antigravity", "User", "globalStorage", "state.vscdb")
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sqlite3", dbPath, "SELECT value FROM ItemTable WHERE key = 'history.recentlyOpenedPathsList';")
	out, err := cmd.Output()
	if err != nil {
		log.Printf("[Projects] Warning: failed to read state.vscdb: %v", err)
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

		if fi, err := os.Stat(path); err != nil {
			continue
		} else if !isWorkspace && !fi.IsDir() {
			continue
		}

		name := filepath.Base(path)
		if isWorkspace {
			name = strings.TrimSuffix(name, ".code-workspace")
		}

		items = append(items, ProjectItem{
			Name:        name,
			URI:         rawURI,
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
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: p.transport,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	var reader io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err == nil {
			defer gz.Close()
			reader = gz
		}
	}

	var data upstreamTrajectoriesResp
	if err := json.NewDecoder(reader).Decode(&data); err != nil {
		return nil, err
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
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

	var req CreateCascadeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
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
	} else {
		startPayload["source"] = "CORTEX_TRAJECTORY_SOURCE_INTERACTIVE_CASCADE"
		startPayload["workspaceUris"] = []string{wsURI}
	}

	if modelEnum := resolveModelEnum(req.Model); modelEnum != "" {
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

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: p.transport,
	}

	startResp, err := client.Do(httpReq)
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
			if annResp, err := client.Do(annReq); err == nil {
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

		if cfg := p.GetCascadeConfig(cascadeID, port, token); len(cfg) > 0 {
			var cfgObj interface{}
			if err := json.Unmarshal(cfg, &cfgObj); err == nil {
				msgPayload["cascadeConfig"] = cfgObj
			}
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
			if msgResp, err := client.Do(msgReq); err == nil {
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

func normalizeURI(uri string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return ""
	}
	if !strings.HasPrefix(uri, "file://") && strings.HasPrefix(uri, "/") {
		uri = "file://" + uri
	}
	return strings.TrimSuffix(uri, "/")
}

func uriToPath(rawURI string) string {
	if !strings.HasPrefix(rawURI, "file://") {
		return rawURI
	}
	u, err := url.Parse(rawURI)
	if err != nil {
		return strings.TrimPrefix(rawURI, "file://")
	}
	path, err := url.PathUnescape(u.Path)
	if err != nil {
		return u.Path
	}
	return path
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
	"gemini-2.5-pro":           "MODEL_GOOGLE_GEMINI_2_5_PRO",
	"gemini-2.5-flash":         "MODEL_GOOGLE_GEMINI_2_5_FLASH",
	"claude":                   "MODEL_PLACEHOLDER_M26",
	"claude-opus":              "MODEL_PLACEHOLDER_M26",
	"claude-opus-4-6-thinking": "MODEL_PLACEHOLDER_M26",
	"claude-sonnet-4-6":        "MODEL_PLACEHOLDER_M35",
	"gpt-oss-120b-medium":      "MODEL_OPENAI_GPT_OSS_120B_MEDIUM",
}

// resolveModelEnum resolves a user or client provided model name to its protobuf enum string.
func resolveModelEnum(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if strings.HasPrefix(model, "MODEL_") {
		return model
	}
	if enum, ok := modelEnumMap[strings.ToLower(model)]; ok {
		return enum
	}
	return ""
}
