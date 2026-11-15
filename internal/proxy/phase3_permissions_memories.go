package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var permissionConfigMu sync.Mutex

// PermissionState represents the resolved permission preset and raw Antigravity userSettings flags.
type PermissionState struct {
	OK                       bool   `json:"ok"`
	Preset                   string `json:"preset"` // "strict" | "sandbox" | "auto"
	Label                    string `json:"label"`
	IsUnlocked               bool   `json:"isUnlocked"`
	PermissionPreset         string `json:"permissionPreset"`
	AutoExecutionPolicy      string `json:"autoExecutionPolicy"`
	NonWorkspaceFilePolicy   string `json:"nonWorkspaceFileAccessPolicy"`
	EnableTerminalSandbox    bool   `json:"enableTerminalSandbox"`
	Source                   string `json:"source"`
}

func resolveLocalGeminiConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".gemini", "config", "config.json")
}

func permissionPresetProfile(preset string) (label, upPreset, execPolicy, filePolicy string, sandbox bool) {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "sandbox", "agent_permission_preset_default":
		return "离席沙箱放行", "AGENT_PERMISSION_PRESET_DEFAULT", "CASCADE_COMMANDS_AUTO_EXECUTION_AUTO", "FILE_ACCESS_POLICY_ASK", true
	case "auto", "turbo", "agent_permission_preset_turbo", "agent_permission_preset_auto":
		return "全自动", "AGENT_PERMISSION_PRESET_TURBO", "CASCADE_COMMANDS_AUTO_EXECUTION_TURBO", "FILE_ACCESS_POLICY_ALLOW", false
	default:
		return "严格审批", "AGENT_PERMISSION_PRESET_REQUEST_REVIEW", "CASCADE_COMMANDS_AUTO_EXECUTION_OFF", "FILE_ACCESS_POLICY_ASK", true
	}
}

func classifyPermissionPreset(upPreset, execPolicy, filePolicy string) (preset, label string, unlocked bool) {
	p := strings.ToUpper(strings.TrimSpace(upPreset))
	e := strings.ToUpper(strings.TrimSpace(execPolicy))
	f := strings.ToUpper(strings.TrimSpace(filePolicy))
	if p == "AGENT_PERMISSION_PRESET_TURBO" || p == "AGENT_PERMISSION_PRESET_AUTO" || e == "CASCADE_COMMANDS_AUTO_EXECUTION_TURBO" || f == "FILE_ACCESS_POLICY_ALLOW" {
		return "auto", "全自动", true
	}
	if p == "AGENT_PERMISSION_PRESET_DEFAULT" || e == "CASCADE_COMMANDS_AUTO_EXECUTION_AUTO" {
		return "sandbox", "离席沙箱放行", true
	}
	return "strict", "严格审批", false
}

func readLocalPermissionState() PermissionState {
	state := PermissionState{
		OK:                     true,
		Preset:                 "strict",
		Label:                  "严格审批",
		IsUnlocked:             false,
		PermissionPreset:       "AGENT_PERMISSION_PRESET_REQUEST_REVIEW",
		AutoExecutionPolicy:    "CASCADE_COMMANDS_AUTO_EXECUTION_OFF",
		NonWorkspaceFilePolicy: "FILE_ACCESS_POLICY_ASK",
		EnableTerminalSandbox:  true,
		Source:                 "local",
	}

	cfgPath := resolveLocalGeminiConfigPath()
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return state
	}
	var doc map[string]interface{}
	if json.Unmarshal(raw, &doc) != nil {
		return state
	}
	us, ok := doc["userSettings"].(map[string]interface{})
	if !ok {
		return state
	}
	if v, ok := us["permissionPreset"].(string); ok && v != "" {
		state.PermissionPreset = v
	}
	if v, ok := us["autoExecutionPolicy"].(string); ok && v != "" {
		state.AutoExecutionPolicy = v
	}
	if v, ok := us["nonWorkspaceFileAccessPolicy"].(string); ok && v != "" {
		state.NonWorkspaceFilePolicy = v
	}
	if v, ok := us["enableTerminalSandbox"].(bool); ok {
		state.EnableTerminalSandbox = v
	}
	state.Preset, state.Label, state.IsUnlocked = classifyPermissionPreset(
		state.PermissionPreset,
		state.AutoExecutionPolicy,
		state.NonWorkspaceFilePolicy,
	)
	return state
}

func writeLocalPermissionPreset(preset string) (PermissionState, error) {
	permissionConfigMu.Lock()
	defer permissionConfigMu.Unlock()

	label, upPreset, execPolicy, filePolicy, sandbox := permissionPresetProfile(preset)
	cfgPath := resolveLocalGeminiConfigPath()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return PermissionState{}, err
	}

	doc := make(map[string]interface{})
	if raw, err := os.ReadFile(cfgPath); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &doc)
	}
	us, _ := doc["userSettings"].(map[string]interface{})
	if us == nil {
		us = make(map[string]interface{})
	}
	us["permissionPreset"] = upPreset
	us["autoExecutionPolicy"] = execPolicy
	us["nonWorkspaceFileAccessPolicy"] = filePolicy
	us["enableTerminalSandbox"] = sandbox
	doc["userSettings"] = us

	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return PermissionState{}, err
	}
	tmp := cfgPath + ".tmp"
	if err := os.WriteFile(tmp, append(encoded, '\n'), 0o600); err != nil {
		return PermissionState{}, err
	}
	if err := os.Rename(tmp, cfgPath); err != nil {
		return PermissionState{}, err
	}

	canonical, _, unlocked := classifyPermissionPreset(upPreset, execPolicy, filePolicy)
	return PermissionState{
		OK:                     true,
		Preset:                 canonical,
		Label:                  label,
		IsUnlocked:             unlocked,
		PermissionPreset:       upPreset,
		AutoExecutionPolicy:    execPolicy,
		NonWorkspaceFilePolicy: filePolicy,
		EnableTerminalSandbox:  sandbox,
		Source:                 "local",
	}, nil
}

// HandlePermissions gets or updates Antigravity autonomous permission settings (syncing local config.json and upstream RPC when enabled).
func (p *Proxy) HandlePermissions(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		state := readLocalPermissionState()
		// Best-effort upstream GetPermissionSettings if cloud settings are active.
		if port, _ := p.ActiveUpstream(); port > 0 {
			ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
			body, status, err := p.callUpstreamRPC(ctx, "GetPermissionSettings", map[string]interface{}{})
			cancel()
			if err == nil && status == http.StatusOK && len(body) > 2 {
				var up struct {
					Settings *struct {
						PermissionPreset         string `json:"permissionPreset"`
						AutoExecutionPolicy      string `json:"autoExecutionPolicy"`
						NonWorkspaceFilePolicy   string `json:"nonWorkspaceFileAccessPolicy"`
						EnableTerminalSandbox    *bool  `json:"enableTerminalSandbox"`
					} `json:"settings"`
				}
				if json.Unmarshal(body, &up) == nil && up.Settings != nil && up.Settings.PermissionPreset != "" {
					state.PermissionPreset = up.Settings.PermissionPreset
					state.AutoExecutionPolicy = firstNonEmpty(up.Settings.AutoExecutionPolicy, state.AutoExecutionPolicy)
					state.NonWorkspaceFilePolicy = firstNonEmpty(up.Settings.NonWorkspaceFilePolicy, state.NonWorkspaceFilePolicy)
					if up.Settings.EnableTerminalSandbox != nil {
						state.EnableTerminalSandbox = *up.Settings.EnableTerminalSandbox
					}
					state.Preset, state.Label, state.IsUnlocked = classifyPermissionPreset(
						state.PermissionPreset, state.AutoExecutionPolicy, state.NonWorkspaceFilePolicy,
					)
					state.Source = "upstream+local"
				}
			}
		}
		writeJSONStatus(w, http.StatusOK, state)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Preset string `json:"preset"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	state, err := writeLocalPermissionPreset(req.Preset)
	if err != nil {
		writeRPCError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if port, _ := p.ActiveUpstream(); port > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
		_, status, rpcErr := p.callUpstreamRPC(ctx, "UpdatePermissionSettings", map[string]interface{}{
			"settings": map[string]interface{}{
				"permissionPreset":             state.PermissionPreset,
				"autoExecutionPolicy":          state.AutoExecutionPolicy,
				"nonWorkspaceFileAccessPolicy": state.NonWorkspaceFilePolicy,
				"enableTerminalSandbox":        state.EnableTerminalSandbox,
			},
		})
		cancel()
		if rpcErr == nil && status == http.StatusOK {
			state.Source = "upstream+local"
		}
	}

	writeJSONStatus(w, http.StatusOK, state)
}

// MemoryEntry represents a unified rule, GEMINI.md instruction, or cascade memory item.
type MemoryEntry struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Scope    string `json:"scope"` // "global", "workspace", "memory"
	Path     string `json:"path"`
	Content  string `json:"content"`
	Editable bool   `json:"editable"`
}

func allowedMemoryRoots(workspacePath string) []string {
	home, _ := os.UserHomeDir()
	roots := []string{
		filepath.Join(home, ".gemini", "GEMINI.md"),
		filepath.Join(home, ".gemini", "config", "GEMINI.md"),
		filepath.Join(home, ".agents", "rules"),
		filepath.Join(home, ".gemini", "antigravity", "memories"),
	}
	if workspacePath != "" {
		roots = append(roots,
			filepath.Join(workspacePath, "GEMINI.md"),
			filepath.Join(workspacePath, ".agents", "rules"),
		)
	}
	return roots
}

func isWithinMemoryRoots(target string, roots []string) bool {
	cleanTarget, err := filepath.Abs(filepath.Clean(target))
	if err != nil {
		return false
	}
	for _, root := range roots {
		cleanRoot, err := filepath.Abs(filepath.Clean(root))
		if err != nil {
			continue
		}
		if cleanTarget == cleanRoot {
			return true
		}
		if strings.HasSuffix(cleanRoot, ".md") {
			continue
		}
		rel, err := filepath.Rel(cleanRoot, cleanTarget)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// HandleMemories manages Antigravity rules/instructions/memories via GetAllRules + sandboxed local files.
func (p *Proxy) HandleMemories(w http.ResponseWriter, r *http.Request) {
	home, _ := os.UserHomeDir()
	workspacePath := ""
	if ws := r.URL.Query().Get("workspacePath"); ws != "" {
		workspacePath = ws
	} else if wsURI := r.URL.Query().Get("workspaceUri"); wsURI != "" {
		workspacePath = uriToFilePath(wsURI)
	}

	if r.Method == http.MethodGet {
		items := collectLocalMemories(home, workspacePath)
		// Merge any upstream GetAllRules entries not yet in local list.
		if port, _ := p.ActiveUpstream(); port > 0 {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			body, status, err := p.callUpstreamRPC(ctx, "GetAllRules", map[string]interface{}{})
			cancel()
			if err == nil && status == http.StatusOK {
				var up struct {
					Rules []struct {
						Name        string `json:"name"`
						Path        string `json:"path"`
						Content     string `json:"content"`
						Description string `json:"description"`
					} `json:"rules"`
				}
				if json.Unmarshal(body, &up) == nil {
					seen := make(map[string]bool)
					for _, it := range items {
						seen[it.Path] = true
					}
					for _, rl := range up.Rules {
						if rl.Path != "" && seen[rl.Path] {
							continue
						}
						content := rl.Content
						if content == "" && rl.Path != "" {
							if raw, err := os.ReadFile(rl.Path); err == nil {
								content = string(raw)
							}
						}
						items = append(items, MemoryEntry{
							ID:       firstNonEmpty(rl.Path, rl.Name),
							Title:    firstNonEmpty(rl.Name, filepath.Base(rl.Path), "Rule"),
							Scope:    "global",
							Path:     rl.Path,
							Content:  content,
							Editable: isWithinMemoryRoots(rl.Path, allowedMemoryRoots(workspacePath)),
						})
					}
				}
			}
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":    true,
			"items": items,
		})
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Action        string `json:"action"` // "upsert" | "delete"
		Scope         string `json:"scope"`  // "global" | "workspace" | "memory"
		WorkspacePath string `json:"workspacePath"`
		Path          string `json:"path"`
		Title         string `json:"title"`
		Content       string `json:"content"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1024*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	if req.WorkspacePath != "" {
		workspacePath = uriToFilePath(req.WorkspacePath)
	}
	roots := allowedMemoryRoots(workspacePath)

	targetPath := strings.TrimSpace(req.Path)
	if targetPath == "" {
		slug := sanitizeFileSlug(req.Title)
		if slug == "" {
			slug = "custom-memory"
		}
		switch req.Scope {
		case "workspace":
			if workspacePath == "" {
				writeRPCError(w, http.StatusBadRequest, "未指定工作区路径")
				return
			}
			targetPath = filepath.Join(workspacePath, ".agents", "rules", slug+".md")
		case "memory":
			targetPath = filepath.Join(home, ".gemini", "antigravity", "memories", slug+".md")
		default:
			targetPath = filepath.Join(home, ".agents", "rules", slug+".md")
		}
	}

	if !isWithinMemoryRoots(targetPath, roots) {
		writeRPCError(w, http.StatusForbidden, "路径不在允许的记忆与规则目录内")
		return
	}

	if strings.EqualFold(req.Action, "delete") {
		if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
			writeRPCError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":    true,
			"items": collectLocalMemories(home, workspacePath),
		})
		return
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		writeRPCError(w, http.StatusInternalServerError, err.Error())
		return
	}
	tmp := targetPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(req.Content), 0o644); err != nil {
		writeRPCError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := os.Rename(tmp, targetPath); err != nil {
		writeRPCError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"ok":    true,
		"path":  targetPath,
		"items": collectLocalMemories(home, workspacePath),
	})
}

func collectLocalMemories(home, workspacePath string) []MemoryEntry {
	var out []MemoryEntry
	addFile := func(title, scope, path string) {
		raw, err := os.ReadFile(path)
		if err != nil {
			return
		}
		out = append(out, MemoryEntry{
			ID:       path,
			Title:    title,
			Scope:    scope,
			Path:     path,
			Content:  string(raw),
			Editable: true,
		})
	}

	globalCandidates := []string{
		filepath.Join(home, ".gemini", "GEMINI.md"),
		filepath.Join(home, ".gemini", "config", "GEMINI.md"),
	}
	for _, p := range globalCandidates {
		addFile("全局指令 ("+filepath.Base(filepath.Dir(p))+"/GEMINI.md)", "global", p)
	}

	scanRuleDir := func(dir, scope string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() || (!strings.HasSuffix(e.Name(), ".md") && !strings.HasSuffix(e.Name(), ".txt")) {
				continue
			}
			addFile(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())), scope, filepath.Join(dir, e.Name()))
		}
	}

	scanRuleDir(filepath.Join(home, ".agents", "rules"), "global")
	scanRuleDir(filepath.Join(home, ".gemini", "antigravity", "memories"), "memory")
	if workspacePath != "" {
		addFile("项目指令 (GEMINI.md)", "workspace", filepath.Join(workspacePath, "GEMINI.md"))
		scanRuleDir(filepath.Join(workspacePath, ".agents", "rules"), "workspace")
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Scope != out[j].Scope {
			return out[i].Scope < out[j].Scope
		}
		return out[i].Title < out[j].Title
	})
	return out
}

func sanitizeFileSlug(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == ' ':
			b.WriteByte('-')
		case r > 127:
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "-")
}
