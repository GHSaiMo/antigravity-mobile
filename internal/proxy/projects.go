package proxy

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProjectItem represents a discovered upstream project or workspace.
type ProjectItem struct {
	ID           string     `json:"id,omitempty"`
	Name         string     `json:"name"`
	Alias        string     `json:"alias,omitempty"`
	URI          string     `json:"uri"`
	Path         string     `json:"path"`
	IsWorkspace  bool       `json:"isWorkspace"`
	SessionCount int        `json:"sessionCount"`
	LastActive   *time.Time `json:"lastActive,omitempty"`
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
	projectIDStats := make(map[string]struct {
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

				if sum.TrajectoryMetadata != nil {
					if pid := strings.TrimSpace(sum.TrajectoryMetadata.ProjectID); pid != "" && pid != "outside-of-project" {
						pstat := projectIDStats[pid]
						pstat.count++
						if activeTime.After(pstat.lastActive) {
							pstat.lastActive = activeTime
						}
						projectIDStats[pid] = pstat
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
		if prj.Path != "" && isDriveRoot(prj.Path) {
			return
		}
		if prj.Name == "" || prj.Name == "/" || prj.Name == "\\" || prj.Name == "." {
			return
		}
		if existing, exists := projectMap[norm]; exists {
			if existing.ID == "" && prj.ID != "" {
				existing.ID = prj.ID
				if prj.Name != "" {
					existing.Name = prj.Name
				}
			} else if (existing.ID == prj.ID || existing.Name == "") && prj.Name != "" {
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
		if itemCopy.ID != "" {
			if pstat, ok := projectIDStats[itemCopy.ID]; ok {
				if pstat.count > itemCopy.SessionCount {
					itemCopy.SessionCount = pstat.count
				}
				if !pstat.lastActive.IsZero() && (itemCopy.LastActive == nil || pstat.lastActive.After(*itemCopy.LastActive)) {
					t := pstat.lastActive
					itemCopy.LastActive = &t
				}
			}
		}
		projectMap[norm] = &itemCopy
	}

	// 2. Load Antigravity 2.0 projects directly from ~/.gemini/config/projects/
	for _, prj := range fetchProjectsFromGeminiConfig() {
		mergeProject(prj)
	}

	// 3. Load official ordered Projects from app_storage.json + ReadProjects RPC
	if officialProjects, err := p.fetchOfficialProjects(port, token, sessionStats); err == nil {
		for _, prj := range officialProjects {
			mergeProject(prj)
		}
	}

	// Strictly align with official Projects managed in the Windows/macOS Antigravity client.
	// Only fall back to local workspace caches (workspaceStorage / stateDB / trajectories)
	// when no official projects are configured at all, or if MGY_INCLUDE_WORKSPACE_CACHE is explicitly enabled.
	includeWorkspaceCache := strings.EqualFold(os.Getenv("MGY_INCLUDE_WORKSPACE_CACHE"), "true")
	if len(projectMap) == 0 || includeWorkspaceCache {
		// 4. Load workspace projects from workspaceStorage (plain JSON, works on all OSes without SQLite/Python)
		for _, prj := range fetchProjectsFromWorkspaceStorage() {
			mergeProject(prj)
		}

		// 5. Load from state.vscdb (history.recentlyOpenedPathsList)
		for _, prj := range fetchProjectsFromStateDB() {
			mergeProject(prj)
		}

		// 6. Load any remaining active workspace URIs from trajectories
		for norm, stat := range sessionStats {
			if _, exists := projectMap[norm]; !exists {
				parsedPath := uriToPath(norm)
				if parsedPath == "" || isDriveRoot(parsedPath) {
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
				if name == "" || name == "/" || name == "\\" || name == "." {
					continue
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
	}

	result := make([]ProjectItem, 0, len(projectMap))
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

	aliases := loadWorkspaceAliases()
	if len(aliases) > 0 {
		for i := range result {
			normPath := normalizeWorkspaceKey(result[i].Path)
			normURI := normalizeWorkspaceKey(result[i].URI)
			if a, ok := aliases[normPath]; ok && a != "" {
				result[i].Alias = a
			} else if a, ok := aliases[normURI]; ok && a != "" {
				result[i].Alias = a
			}
		}
	}

	return result, nil
}

// BuildWorkspaceDisplayNameResolver returns a fast lookup closure that resolves
// a trajectory's projectID and official project display name (Alias or Name)
// from its projectId and/or workspaceURIs without per-trajectory RPC calls.
func (p *Proxy) BuildWorkspaceDisplayNameResolver() func(projectID string, workspaceURIs []string) (string, string) {
	cfgProjects := fetchProjectsFromGeminiConfig()
	aliases := loadWorkspaceAliases()

	byID := make(map[string]string, len(cfgProjects))
	type uriEntry struct {
		id   string
		name string
	}
	byURI := make(map[string]uriEntry, len(cfgProjects))
	byPath := make(map[string]uriEntry, len(cfgProjects))

	register := func(id, name, uri, path string) {
		disp := strings.TrimSpace(name)
		normURI := normalizeURI(uri)
		cleanPath := ""
		if path != "" {
			cleanPath = filepath.Clean(path)
		} else if normURI != "" {
			if parsed := uriToPath(normURI); parsed != "" {
				cleanPath = filepath.Clean(parsed)
			}
		}
		if len(aliases) > 0 {
			if cleanPath != "" {
				if a, ok := aliases[normalizeWorkspaceKey(cleanPath)]; ok && a != "" {
					disp = a
				}
			}
			if normURI != "" {
				if a, ok := aliases[normalizeWorkspaceKey(normURI)]; ok && a != "" {
					disp = a
				}
			}
		}
		if id != "" && disp != "" {
			if _, exists := byID[id]; !exists {
				byID[id] = disp
			}
		}
		if normURI != "" && disp != "" {
			if _, exists := byURI[normURI]; !exists {
				byURI[normURI] = uriEntry{id: id, name: disp}
			}
		}
		if cleanPath != "" && disp != "" {
			if _, exists := byPath[cleanPath]; !exists {
				byPath[cleanPath] = uriEntry{id: id, name: disp}
			}
		}
	}

	for _, prj := range cfgProjects {
		register(prj.ID, prj.Name, prj.URI, prj.Path)
	}

	p.projectsCacheMu.RLock()
	cachedReadResp := p.lastReadProjectsResp
	p.projectsCacheMu.RUnlock()
	if len(cachedReadResp) > 0 {
		var readResp struct {
			Projects []geminiProjectConfigFile `json:"projects"`
		}
		if err := json.Unmarshal(cachedReadResp, &readResp); err == nil {
			for _, prj := range readResp.Projects {
				if prj.ID == "" || prj.ID == "outside-of-project" {
					continue
				}
				if prj.ProjectResources != nil {
					for _, r := range prj.ProjectResources.Resources {
						u := r.FolderURI
						if u == "" && r.GitFolder != nil {
							u = r.GitFolder.FolderURI
						}
						if u != "" {
							register(prj.ID, prj.Name, u, "")
						}
					}
				} else if prj.Name != "" {
					register(prj.ID, prj.Name, "", "")
				}
			}
		}
	}

	return func(projectID string, workspaceURIs []string) (string, string) {
		resolvedID := strings.TrimSpace(projectID)
		if resolvedID != "" && resolvedID != "outside-of-project" {
			if disp, ok := byID[resolvedID]; ok && disp != "" {
				return resolvedID, disp
			}
		}
		for _, wsURI := range workspaceURIs {
			if wsURI == "" {
				continue
			}
			norm := normalizeURI(wsURI)
			if e, ok := byURI[norm]; ok {
				if resolvedID == "" || resolvedID == "outside-of-project" {
					resolvedID = e.id
				}
				return resolvedID, e.name
			}
			if tPath := uriToPath(norm); tPath != "" {
				cleanT := filepath.Clean(tPath)
				if e, ok := byPath[cleanT]; ok {
					if resolvedID == "" || resolvedID == "outside-of-project" {
						resolvedID = e.id
					}
					return resolvedID, e.name
				}
				if len(aliases) > 0 {
					if a, ok := aliases[normalizeWorkspaceKey(cleanT)]; ok && a != "" {
						return resolvedID, a
					}
				}
			}
			if len(aliases) > 0 && norm != "" {
				if a, ok := aliases[normalizeWorkspaceKey(norm)]; ok && a != "" {
					return resolvedID, a
				}
			}
		}
		return resolvedID, ""
	}
}

// FindProjectIDForWorkspaces matches workspace URIs against known projects and returns the project ID.
func (p *Proxy) FindProjectIDForWorkspaces(workspaceURIs []string) string {
	if len(workspaceURIs) == 0 {
		return ""
	}
	projects, err := p.GetProjects()
	if err != nil || len(projects) == 0 {
		projects = fetchProjectsFromGeminiConfig()
	}
	for _, wsURI := range workspaceURIs {
		if wsURI == "" {
			continue
		}
		targetNorm := normalizeURI(wsURI)
		targetPath := uriToPath(targetNorm)
		for _, prj := range projects {
			if prj.ID == "" {
				continue
			}
			if normalizeURI(prj.URI) == targetNorm || uriToPath(prj.URI) == targetPath || (prj.Path != "" && filepath.Clean(prj.Path) == filepath.Clean(targetPath)) {
				return prj.ID
			}
		}
		for _, prj := range fetchProjectsFromGeminiConfig() {
			if prj.ID == "" {
				continue
			}
			if normalizeURI(prj.URI) == targetNorm || uriToPath(prj.URI) == targetPath || (prj.Path != "" && filepath.Clean(prj.Path) == filepath.Clean(targetPath)) {
				return prj.ID
			}
		}
	}
	return ""
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

var (
	workspaceAliasesMu sync.RWMutex
)

// modelEnumMap maps friendly model IDs or aliases to upstream Protobuf enum names.
var modelEnumMap = map[string]string{
	"gemini":                   "MODEL_PLACEHOLDER_M318",
	"gemini-flash":             "MODEL_PLACEHOLDER_M318",
	"gemini-4-argon":           "MODEL_PLACEHOLDER_M417",
	"gemini-4-argon-high":      "MODEL_PLACEHOLDER_M417",
	"gemini-4-argon-medium":    "MODEL_PLACEHOLDER_M418",
	"gemini-4-argon-low":       "MODEL_PLACEHOLDER_M419",
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
	"claude-opus-5-5":          "MODEL_PLACEHOLDER_M402",
	"claude-opus-5-5-high":     "MODEL_PLACEHOLDER_M402",
	"claude-opus-5-5-medium":   "MODEL_PLACEHOLDER_M401",
	"claude-opus-5-5-low":      "MODEL_PLACEHOLDER_M400",
	"claude-sonnet-5-5":        "MODEL_PLACEHOLDER_M405",
	"claude-sonnet-5-5-high":   "MODEL_PLACEHOLDER_M405",
	"claude-sonnet-5-5-medium": "MODEL_PLACEHOLDER_M404",
	"claude-sonnet-5-5-low":    "MODEL_PLACEHOLDER_M403",
	"claude-opus-4-6-thinking": "MODEL_PLACEHOLDER_M26",
	"claude-sonnet-4-6":        "MODEL_PLACEHOLDER_M35",
	"claude-3-7-sonnet":        "MODEL_PLACEHOLDER_M26",
	"claude-3-5-sonnet":        "MODEL_PLACEHOLDER_M26",
	"gpt-oss-120b-medium":      "MODEL_OPENAI_GPT_OSS_120B_MEDIUM",
}

// enumToCanonicalMap maps upstream Protobuf enum names to canonical model identifiers.
var enumToCanonicalMap = map[string]string{
	"MODEL_PLACEHOLDER_M402":           "claude-opus-5-5-high",
	"MODEL_PLACEHOLDER_M401":           "claude-opus-5-5-medium",
	"MODEL_PLACEHOLDER_M400":           "claude-opus-5-5-low",
	"MODEL_PLACEHOLDER_M405":           "claude-sonnet-5-5-high",
	"MODEL_PLACEHOLDER_M404":           "claude-sonnet-5-5-medium",
	"MODEL_PLACEHOLDER_M403":           "claude-sonnet-5-5-low",
	"MODEL_PLACEHOLDER_M26":            "claude-opus-4-6-thinking",
	"MODEL_PLACEHOLDER_M35":            "claude-sonnet-4-6",
	"MODEL_PLACEHOLDER_M417":           "gemini-4-argon-high",
	"MODEL_PLACEHOLDER_M418":           "gemini-4-argon-medium",
	"MODEL_PLACEHOLDER_M419":           "gemini-4-argon-low",
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
