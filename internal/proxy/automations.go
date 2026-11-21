package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AutomationSchedule represents a scheduled automation task (from schedule sidecar).
type AutomationSchedule struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description,omitempty"`
	Status         string   `json:"status"` // READY, RUNNING, STOPPED, ERROR
	CronExpression string   `json:"cronExpression,omitempty"`
	TimeZone       string   `json:"timeZone,omitempty"`
	Model          string   `json:"model,omitempty"`
	Prompt         string   `json:"prompt,omitempty"`
	LastRunAt      string   `json:"lastRunAt,omitempty"`
	NextRunAt      string   `json:"nextRunAt,omitempty"`
	LastError      string   `json:"lastError,omitempty"`
	StartTimeMs    string   `json:"startTimeMs,omitempty"`
	Enabled        bool     `json:"enabled"`
	WorkspaceURI   string   `json:"workspaceUri,omitempty"`
	WorkspaceURIs  []string `json:"workspaceUris,omitempty"`
	ProjectID      string   `json:"projectId,omitempty"`
	AccessGrants   []string `json:"accessGrants,omitempty"`
}

// AutomationEvent represents a historical run of an automation.
type AutomationEvent struct {
	ID             string `json:"id"`
	ScheduleID     string `json:"scheduleId"`
	SidecarID      string `json:"sidecarId,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
	Timestamp      string `json:"timestamp"`
	Status         string `json:"status"` // success, error, running
	Message        string `json:"message,omitempty"`
}

// AutomationsResponse is returned by GET /gateway/automations.
type AutomationsResponse struct {
	Automations []AutomationSchedule `json:"automations"`
	Events      []AutomationEvent    `json:"events"`
}

var cronTZPrefixRe = regexp.MustCompile(`^(?:CRON_TZ|TZ)=(\S+)\s+(.+)$`)

// flagsWithValue lists agentapi new-conversation flags that take a separate next-token value.
var scheduleFlagsWithValue = map[string]bool{
	"--model":     true,
	"-model":      true,
	"-m":          true,
	"--workspace": true,
	"-w":          true,
	"--project":   true,
}

// parseScheduleArgs mirrors Antigravity desktop's frd/grd/d3 parsing for schedule sidecar args.
func parseScheduleArgs(args []string) (cronPattern, timeZone, model, prompt string, flags []string) {
	if len(args) == 0 {
		return "", "", "", "", nil
	}
	cronPattern = strings.TrimSpace(args[0])
	if m := cronTZPrefixRe.FindStringSubmatch(cronPattern); len(m) == 3 {
		timeZone = m[1]
	}

	if len(args) <= 3 {
		return cronPattern, timeZone, "", "", nil
	}

	// Look for explicit "--" separator at index >= 3.
	sepIdx := -1
	for i := 3; i < len(args); i++ {
		if args[i] == "--" {
			sepIdx = i
			break
		}
	}
	if sepIdx != -1 {
		flags = append(flags, args[3:sepIdx]...)
		if sepIdx+1 < len(args) {
			prompt = strings.Join(args[sepIdx+1:], " ")
		}
	} else {
		i := 3
		for i < len(args) {
			tok := args[i]
			if strings.HasPrefix(tok, "-") {
				if strings.Contains(tok, "=") {
					flags = append(flags, tok)
					i++
				} else if scheduleFlagsWithValue[tok] && i+1 < len(args) {
					flags = append(flags, tok, args[i+1])
					i += 2
				} else {
					flags = append(flags, tok)
					i++
				}
			} else {
				break
			}
		}
		if i < len(args) {
			prompt = strings.Join(args[i:], " ")
		}
	}

	// Extract --model / -model value from flags.
	for i := 0; i < len(flags); i++ {
		f := flags[i]
		if f == "--model" || f == "-model" || f == "-m" {
			if i+1 < len(flags) {
				model = flags[i+1]
				break
			}
		} else if strings.HasPrefix(f, "--model=") {
			model = strings.TrimPrefix(f, "--model=")
			break
		} else if strings.HasPrefix(f, "-model=") {
			model = strings.TrimPrefix(f, "-model=")
			break
		}
	}
	return cronPattern, timeZone, model, prompt, flags
}

// buildScheduleArgs mirrors Antigravity desktop's krd builder, preserving non-model flags.
func buildScheduleArgs(origArgs []string, cronPattern string, newModel *string, prompt string) []string {
	_, _, _, _, origFlags := parseScheduleArgs(origArgs)
	var updatedFlags []string
	if newModel != nil {
		trimmedModel := strings.TrimSpace(*newModel)
		modelReplaced := false
		for i := 0; i < len(origFlags); i++ {
			f := origFlags[i]
			if f == "--model" || f == "-model" || f == "-m" {
				if i+1 < len(origFlags) {
					i++
				}
				if trimmedModel != "" && !modelReplaced {
					updatedFlags = append(updatedFlags, "--model="+trimmedModel)
					modelReplaced = true
				}
				continue
			}
			if strings.HasPrefix(f, "--model=") || strings.HasPrefix(f, "-model=") {
				if trimmedModel != "" && !modelReplaced {
					updatedFlags = append(updatedFlags, "--model="+trimmedModel)
					modelReplaced = true
				}
				continue
			}
			updatedFlags = append(updatedFlags, f)
		}
		if trimmedModel != "" && !modelReplaced {
			updatedFlags = append(updatedFlags, "--model="+trimmedModel)
		}
	} else {
		updatedFlags = append(updatedFlags, origFlags...)
	}

	cmd1 := "agentapi"
	cmd2 := "new-conversation"
	if len(origArgs) > 1 && strings.TrimSpace(origArgs[1]) != "" {
		cmd1 = origArgs[1]
	}
	if len(origArgs) > 2 && strings.TrimSpace(origArgs[2]) != "" {
		cmd2 = origArgs[2]
	}

	out := make([]string, 0, 5+len(updatedFlags))
	out = append(out, strings.TrimSpace(cronPattern), cmd1, cmd2)
	out = append(out, updatedFlags...)
	out = append(out, "--", strings.TrimSpace(prompt))
	return out
}

func isSafeSidecarID(id string) bool {
	s := strings.TrimSpace(id)
	if s == "" || s == "." || s == ".." {
		return false
	}
	if strings.Contains(s, "..") || strings.ContainsRune(s, '/') || strings.ContainsRune(s, '\\') {
		return false
	}
	return true
}

// HandleGetAutomations serves GET /gateway/automations.
func (p *Proxy) HandleGetAutomations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// 1. First, load all local sidecar definitions and configurations from disk
	schedulesMap := readLocalSidecars()

	// 2. Query upstream language_server if available to enrich live runtime status
	port, token := p.ActiveUpstream()
	if p.insp != nil {
		if cur := p.insp.Current(); cur != nil && cur.IsHealthy && cur.Port > 0 {
			port = cur.Port
			token = cur.CSRFToken
		}
	}

	if port > 0 {
		sidecarsURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetSidecars", port)
		req, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, sidecarsURL, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Connect-Protocol-Version", "1")
		if token != "" {
			req.Header.Set("x-codeium-csrf-token", token)
		}

		if res, err := p.mediumClient.Do(req); err == nil {
			defer res.Body.Close()
			body, _ := io.ReadAll(res.Body)
			var raw struct {
				Sidecars []struct {
					SidecarID   string `json:"sidecarId"`
					ID          string `json:"id"`
					Status      string `json:"status"`
					LastError   string `json:"lastError"`
					StartTimeMs string `json:"startTimeMs"`
					Config      struct {
						Builtin          string   `json:"builtin"`
						Args             []string `json:"args"`
						DisplayName      string   `json:"displayName"`
						Description      string   `json:"description"`
						RestartPolicy    string   `json:"restartPolicy"`
						AgentPermissions *struct {
							WorkspaceURIs []string `json:"workspaceUris"`
							AccessGrants  []string `json:"accessGrants"`
						} `json:"agentPermissions"`
					} `json:"config"`
					Builtin     string            `json:"builtin"`
					Name        string            `json:"name"`
					Description string            `json:"description"`
					Metadata    map[string]string `json:"metadata"`
					UserConfig  *struct {
						Enabled   *bool  `json:"enabled"`
						ProjectID string `json:"projectId"`
					} `json:"userConfig"`
				} `json:"sidecars"`
			}

			if json.Unmarshal(body, &raw) == nil {
				for _, sc := range raw.Sidecars {
					sid := sc.SidecarID
					if sid == "" {
						sid = sc.ID
					}
					if sid == "" {
						continue
					}

					builtin := sc.Config.Builtin
					if builtin == "" {
						builtin = sc.Builtin
					}

					// Only consider schedule sidecars
					isSchedule := builtin == "schedule" ||
						strings.Contains(strings.ToLower(sid), "schedule") ||
						strings.Contains(strings.ToLower(sc.Name), "schedule") ||
						strings.Contains(strings.ToLower(sc.Config.DisplayName), "schedule")

					if !isSchedule && len(sc.Config.Args) > 0 {
						if len(sc.Config.Args) >= 3 && sc.Config.Args[1] == "agentapi" && sc.Config.Args[2] == "new-conversation" {
							isSchedule = true
						}
					}

					if !isSchedule {
						continue
					}

					cron, tz, model, prompt, _ := parseScheduleArgs(sc.Config.Args)
					if cron == "" && sc.Metadata != nil {
						cron = sc.Metadata["cron"]
					}
					if prompt == "" && sc.Metadata != nil {
						prompt = sc.Metadata["prompt"]
					}

					name := sc.Config.DisplayName
					if name == "" {
						name = sc.Name
					}
					if name == "" {
						name = sid
					}

					desc := sc.Config.Description
					if desc == "" {
						desc = sc.Description
					}

					status := "READY"
					stUpper := strings.ToUpper(sc.Status)
					if strings.Contains(stUpper, "RUNNING") {
						status = "RUNNING"
					} else if strings.Contains(stUpper, "STOPPED") {
						status = "STOPPED"
					} else if strings.Contains(stUpper, "ERROR") {
						status = "ERROR"
					}

					enabled := status == "RUNNING" || status == "READY"
					projectID := ""
					if sc.UserConfig != nil {
						projectID = sc.UserConfig.ProjectID
						if sc.UserConfig.Enabled != nil {
							enabled = *sc.UserConfig.Enabled
							if !enabled {
								status = "STOPPED"
							}
						}
					}

					var wsURIs, grants []string
					var primaryWS string
					if sc.Config.AgentPermissions != nil {
						wsURIs = sc.Config.AgentPermissions.WorkspaceURIs
						grants = sc.Config.AgentPermissions.AccessGrants
						if len(wsURIs) > 0 {
							primaryWS = wsURIs[0]
						}
					}

					existing, found := schedulesMap[sid]
					if found {
						existing.Status = status
						existing.Enabled = enabled
						existing.LastError = sc.LastError
						existing.StartTimeMs = sc.StartTimeMs
						if cron != "" {
							existing.CronExpression = cron
						}
						if tz != "" {
							existing.TimeZone = tz
						}
						if model != "" {
							existing.Model = model
						}
						if prompt != "" {
							existing.Prompt = prompt
						}
						if name != "" {
							existing.Name = name
						}
						if desc != "" {
							existing.Description = desc
						}
						if projectID != "" {
							existing.ProjectID = projectID
						}
						if primaryWS != "" {
							existing.WorkspaceURI = primaryWS
						}
						if len(wsURIs) > 0 {
							existing.WorkspaceURIs = wsURIs
						}
						if len(grants) > 0 {
							existing.AccessGrants = grants
						}
					} else {
						schedulesMap[sid] = &AutomationSchedule{
							ID:             sid,
							Name:           name,
							Description:    desc,
							Status:         status,
							CronExpression: cron,
							TimeZone:       tz,
							Model:          model,
							Prompt:         prompt,
							LastError:      sc.LastError,
							StartTimeMs:    sc.StartTimeMs,
							Enabled:        enabled,
							WorkspaceURI:   primaryWS,
							WorkspaceURIs:  wsURIs,
							ProjectID:      projectID,
							AccessGrants:   grants,
						}
					}
				}
			}
		}
	}

	// 3. Collect events and enrich LastRunAt / NextRunAt
	allEvents := make([]AutomationEvent, 0)
	for sid, sc := range schedulesMap {
		// Only report NextRunAt when the schedule is enabled (avoid stale log timestamps when paused)
		if sc.Enabled {
			if nextRun := extractNextRunFromLog(sid); nextRun != "" {
				sc.NextRunAt = nextRun
			}
		} else {
			sc.NextRunAt = ""
		}

		// Read local historical events
		events := readLocalSidecarEvents(sid, sc.Name, 20)
		if len(events) > 0 {
			// Latest event timestamp is LastRunAt
			sc.LastRunAt = events[0].Timestamp
			allEvents = append(allEvents, events...)
		}
	}

	// Sort events by timestamp descending (newest first)
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].Timestamp > allEvents[j].Timestamp
	})
	if len(allEvents) > 50 {
		allEvents = allEvents[:50]
	}

	// Convert schedulesMap to slice and sort
	automations := make([]AutomationSchedule, 0, len(schedulesMap))
	for _, sc := range schedulesMap {
		automations = append(automations, *sc)
	}
	sort.Slice(automations, func(i, j int) bool {
		return automations[i].Name < automations[j].Name
	})

	resp := AutomationsResponse{
		Automations: automations,
		Events:      allEvents,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleManageAutomation serves POST /gateway/automations/manage.
func (p *Proxy) HandleManageAutomation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SidecarID    string `json:"sidecarId"`
		SidecarIDAlt string `json:"sidecar_id"`
		Name         string `json:"name"`
		Action       string `json:"action"` // "start", "stop", "restart", "run_now"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SidecarID == "" {
		if req.SidecarIDAlt != "" {
			req.SidecarID = req.SidecarIDAlt
		} else {
			req.SidecarID = req.Name
		}
	}
	req.SidecarID = strings.TrimSpace(req.SidecarID)
	if req.SidecarID == "" {
		writeJSONError(w, "sidecarId cannot be empty", http.StatusBadRequest)
		return
	}

	// Normalize action to upstream SIDECAR_ACTION enum
	mappedAction := "SIDECAR_ACTION_UNSPECIFIED"
	actionLower := strings.ToLower(strings.TrimSpace(req.Action))
	switch actionLower {
	case "start", "enable", "sidecar_action_enable":
		mappedAction = "SIDECAR_ACTION_ENABLE"
	case "stop", "disable", "sidecar_action_disable":
		mappedAction = "SIDECAR_ACTION_DISABLE"
	case "restart", "sidecar_action_restart":
		mappedAction = "SIDECAR_ACTION_RESTART"
	case "run_now", "run", "trigger", "execute", "sidecar_action_run_now":
		mappedAction = "SIDECAR_ACTION_RUN_NOW"
	default:
		if strings.HasPrefix(strings.ToUpper(req.Action), "SIDECAR_ACTION_") {
			mappedAction = strings.ToUpper(req.Action)
		} else {
			mappedAction = req.Action
		}
	}

	// Upstream ManageSidecar RPC call (let language_server own config.json when online)
	port, token := p.ActiveUpstream()
	if p.insp != nil {
		if cur := p.insp.Current(); cur != nil && cur.IsHealthy && cur.Port > 0 {
			port = cur.Port
			token = cur.CSRFToken
		}
	}

	if port > 0 {
		manageURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/ManageSidecar", port)
		bodyBytes, _ := json.Marshal(map[string]interface{}{
			"sidecarId": req.SidecarID,
			"action":    mappedAction,
		})
		upReq, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, manageURL, bytes.NewReader(bodyBytes))
		upReq.Header.Set("Content-Type", "application/json")
		upReq.Header.Set("Connect-Protocol-Version", "1")
		if token != "" {
			upReq.Header.Set("x-codeium-csrf-token", token)
		}

		resp, err := p.mediumClient.Do(upReq)
		if err != nil {
			writeJSONError(w, "upstream call failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			writeJSONError(w, fmt.Sprintf("upstream error (HTTP %d): %s", resp.StatusCode, string(respBody)), resp.StatusCode)
			return
		}
	} else {
		switch mappedAction {
		case "SIDECAR_ACTION_ENABLE":
			_ = updateLocalSidecarEnabled(req.SidecarID, true)
		case "SIDECAR_ACTION_DISABLE":
			_ = updateLocalSidecarEnabled(req.SidecarID, false)
		case "SIDECAR_ACTION_RUN_NOW", "SIDECAR_ACTION_RESTART":
			writeJSONError(w, "upstream language_server unavailable for "+actionLower, http.StatusServiceUnavailable)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"sidecarId": req.SidecarID,
		"action":    req.Action,
	})
}

// HandleUpdateAutomation serves POST /gateway/automations/update.
// Updates title, description, cron expression, model, and/or prompt while preserving
// existing sidecar permissions, restart policy, and other flags.
func (p *Proxy) HandleUpdateAutomation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SidecarID   string  `json:"sidecarId"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Cron        *string `json:"cron"`
		Model       *string `json:"model"`
		Prompt      *string `json:"prompt"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 512*1024)).Decode(&req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	sid := strings.TrimSpace(req.SidecarID)
	if !isSafeSidecarID(sid) {
		writeJSONError(w, "invalid sidecarId", http.StatusBadRequest)
		return
	}

	port, token := p.ActiveUpstream()
	if p.insp != nil {
		if cur := p.insp.Current(); cur != nil && cur.IsHealthy && cur.Port > 0 {
			port = cur.Port
			token = cur.CSRFToken
		}
	}

	// 1. Load existing sidecar config (prefer live GetSidecars to preserve all upstream proto fields)
	var cfgMap map[string]interface{}
	if port > 0 && p.mediumClient != nil {
		sidecarsURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetSidecars", port)
		upReq, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, sidecarsURL, strings.NewReader("{}"))
		upReq.Header.Set("Content-Type", "application/json")
		upReq.Header.Set("Connect-Protocol-Version", "1")
		if token != "" {
			upReq.Header.Set("x-codeium-csrf-token", token)
		}
		if res, err := p.mediumClient.Do(upReq); err == nil {
			defer res.Body.Close()
			if res.StatusCode == http.StatusOK {
				body, _ := io.ReadAll(res.Body)
				var raw struct {
					Sidecars []struct {
						SidecarID string                 `json:"sidecarId"`
						ID        string                 `json:"id"`
						Config    map[string]interface{} `json:"config"`
					} `json:"sidecars"`
				}
				if json.Unmarshal(body, &raw) == nil {
					for _, sc := range raw.Sidecars {
						id := sc.SidecarID
						if id == "" {
							id = sc.ID
						}
						if id == sid && sc.Config != nil {
							cfgMap = sc.Config
							break
						}
					}
				}
			}
		}
	}

	// Fallback to reading local sidecar.json
	var localPath string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		localPath = filepath.Join(home, ".gemini", "config", "sidecars", sid, "sidecar.json")
	}
	if cfgMap == nil && localPath != "" {
		if data, err := os.ReadFile(localPath); err == nil {
			var localMap map[string]interface{}
			if json.Unmarshal(data, &localMap) == nil {
				cfgMap = normalizeSidecarConfigToCamel(localMap)
			}
		}
	}
	if cfgMap == nil {
		writeJSONError(w, "automation not found: "+sid, http.StatusNotFound)
		return
	}

	// Extract current args slice
	var origArgs []string
	if rawArgs, ok := cfgMap["args"].([]interface{}); ok {
		for _, item := range rawArgs {
			if s, ok := item.(string); ok {
				origArgs = append(origArgs, s)
			}
		}
	}
	curCron, _, _, curPrompt, _ := parseScheduleArgs(origArgs)

	newCron := curCron
	if req.Cron != nil {
		newCron = strings.TrimSpace(*req.Cron)
	}
	newPrompt := curPrompt
	if req.Prompt != nil {
		newPrompt = strings.TrimSpace(*req.Prompt)
	}
	if newCron == "" {
		writeJSONError(w, "cron expression cannot be empty", http.StatusBadRequest)
		return
	}
	if newPrompt == "" {
		writeJSONError(w, "prompt cannot be empty", http.StatusBadRequest)
		return
	}

	if req.Name != nil {
		trimmedName := strings.TrimSpace(*req.Name)
		if trimmedName != "" {
			cfgMap["displayName"] = trimmedName
		}
	}
	if req.Description != nil {
		cfgMap["description"] = strings.TrimSpace(*req.Description)
	}
	if _, hasBuiltin := cfgMap["builtin"]; !hasBuiltin {
		cfgMap["builtin"] = "schedule"
	}
	if req.Cron != nil || req.Model != nil || req.Prompt != nil {
		cfgMap["args"] = buildScheduleArgs(origArgs, newCron, req.Model, newPrompt)
	}

	// 2. Persist via upstream ManageSidecar(SIDECAR_ACTION_UPDATE) when online
	if port > 0 && p.mediumClient != nil {
		manageURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/ManageSidecar", port)
		payload, _ := json.Marshal(map[string]interface{}{
			"sidecarId": sid,
			"action":    "SIDECAR_ACTION_UPDATE",
			"config":    cfgMap,
		})
		upReq, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, manageURL, bytes.NewReader(payload))
		upReq.Header.Set("Content-Type", "application/json")
		upReq.Header.Set("Connect-Protocol-Version", "1")
		if token != "" {
			upReq.Header.Set("x-codeium-csrf-token", token)
		}
		resp, err := p.mediumClient.Do(upReq)
		if err != nil {
			writeJSONError(w, "upstream update failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			writeJSONError(w, fmt.Sprintf("upstream error (HTTP %d): %s", resp.StatusCode, string(respBody)), resp.StatusCode)
			return
		}
	} else if localPath != "" {
		// Offline fallback: update sidecar.json in snake_case format
		snakeMap := normalizeSidecarConfigToSnake(cfgMap)
		out, err := json.MarshalIndent(snakeMap, "", "  ")
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpFile := localPath + ".tmp"
		if err := os.WriteFile(tmpFile, append(out, '\n'), 0o644); err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := os.Rename(tmpFile, localPath); err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":        true,
		"sidecarId": sid,
	})
}

func normalizeSidecarConfigToCamel(src map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(src))
	for k, v := range src {
		switch k {
		case "display_name":
			out["displayName"] = v
		case "restart_policy":
			out["restartPolicy"] = v
		case "agent_permissions":
			if pm, ok := v.(map[string]interface{}); ok {
				permOut := make(map[string]interface{}, len(pm))
				for pk, pv := range pm {
					switch pk {
					case "workspace_uris":
						permOut["workspaceUris"] = pv
					case "access_grants":
						permOut["accessGrants"] = pv
					default:
						permOut[pk] = pv
					}
				}
				out["agentPermissions"] = permOut
			} else {
				out["agentPermissions"] = v
			}
		default:
			out[k] = v
		}
	}
	return out
}

func normalizeSidecarConfigToSnake(src map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(src))
	for k, v := range src {
		switch k {
		case "displayName":
			out["display_name"] = v
		case "restartPolicy":
			out["restart_policy"] = v
		case "agentPermissions":
			if pm, ok := v.(map[string]interface{}); ok {
				permOut := make(map[string]interface{}, len(pm))
				for pk, pv := range pm {
					switch pk {
					case "workspaceUris":
						permOut["workspace_uris"] = pv
					case "accessGrants":
						permOut["access_grants"] = pv
					default:
						permOut[pk] = pv
					}
				}
				out["agent_permissions"] = permOut
			} else {
				out["agent_permissions"] = v
			}
		default:
			out[k] = v
		}
	}
	return out
}

// HandleGetAutomationLogs serves GET /gateway/automations/logs?sidecarId=<id>&tail=200&file=sidecar.log.
func (p *Proxy) HandleGetAutomationLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	sid := strings.TrimSpace(r.URL.Query().Get("sidecarId"))
	if !isSafeSidecarID(sid) {
		writeJSONError(w, "invalid sidecarId", http.StatusBadRequest)
		return
	}

	tailLines := 200
	if rawTail := strings.TrimSpace(r.URL.Query().Get("tail")); rawTail != "" {
		if n, err := strconv.Atoi(rawTail); err == nil && n > 0 {
			if n > 1000 {
				n = 1000
			}
			tailLines = n
		}
	}

	reqFile := strings.TrimSpace(r.URL.Query().Get("file"))
	if reqFile == "" {
		reqFile = "sidecar.log"
	}
	if !isSafeSidecarID(reqFile) {
		writeJSONError(w, "invalid log filename", http.StatusBadRequest)
		return
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		writeJSONError(w, "cannot determine user home", http.StatusInternalServerError)
		return
	}
	logsDir := filepath.Join(home, ".gemini", "antigravity", "sidecar_data", sid, "logs")

	var files []string
	if entries, err := os.ReadDir(logsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				files = append(files, e.Name())
			}
		}
		sort.Strings(files)
	}

	logPath := filepath.Join(logsDir, reqFile)
	content := ""
	truncated := false
	if data, err := os.ReadFile(logPath); err == nil {
		// Cap raw tail inspection at 512KB from the end to stay fast on large logs.
		const maxBytes = 512 * 1024
		if len(data) > maxBytes {
			data = data[len(data)-maxBytes:]
			truncated = true
		}
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		if len(lines) > tailLines {
			lines = lines[len(lines)-tailLines:]
			truncated = true
		}
		content = strings.Join(lines, "\n")
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"sidecarId": sid,
		"filename":  reqFile,
		"files":     files,
		"logs":      content,
		"truncated": truncated,
	})
}

// --- Helper Functions for Local Disk Reading & Synchronization ---

// readLocalSidecars scans ~/.gemini/config/sidecars/ for schedule sidecars.
func readLocalSidecars() map[string]*AutomationSchedule {
	result := make(map[string]*AutomationSchedule)

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return result
	}

	configDir := filepath.Join(home, ".gemini", "config")
	sidecarsDir := filepath.Join(configDir, "sidecars")
	configJSONPath := filepath.Join(configDir, "config.json")

	// Read config.json to check enabled status & projectId
	enabledMap := make(map[string]bool)
	projectIDMap := make(map[string]string)
	if data, err := os.ReadFile(configJSONPath); err == nil {
		var cfg struct {
			Sidecars map[string]struct {
				Enabled   *bool  `json:"enabled"`
				ProjectID string `json:"projectId"`
			} `json:"sidecars"`
		}
		if json.Unmarshal(data, &cfg) == nil {
			for k, v := range cfg.Sidecars {
				if v.Enabled != nil {
					enabledMap[k] = *v.Enabled
				}
				if v.ProjectID != "" {
					projectIDMap[k] = v.ProjectID
				}
			}
		}
	}

	entries, err := os.ReadDir(sidecarsDir)
	if err != nil {
		return result
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sid := entry.Name()
		sidecarJSONPath := filepath.Join(sidecarsDir, sid, "sidecar.json")
		scBytes, err := os.ReadFile(sidecarJSONPath)
		if err != nil {
			continue
		}

		var sc struct {
			Builtin          string   `json:"builtin"`
			Args             []string `json:"args"`
			DisplayName      string   `json:"display_name"`
			DisplayNameAlt   string   `json:"displayName"`
			Description      string   `json:"description"`
			AgentPermissions *struct {
				WorkspaceURIs    []string `json:"workspace_uris"`
				WorkspaceURIsAlt []string `json:"workspaceUris"`
				AccessGrants     []string `json:"access_grants"`
				AccessGrantsAlt  []string `json:"accessGrants"`
			} `json:"agent_permissions"`
			AgentPermissionsAlt *struct {
				WorkspaceURIs []string `json:"workspaceUris"`
				AccessGrants  []string `json:"accessGrants"`
			} `json:"agentPermissions"`
		}
		if json.Unmarshal(scBytes, &sc) != nil {
			continue
		}

		if sc.Builtin != "schedule" && !strings.Contains(strings.ToLower(sid), "schedule") {
			if len(sc.Args) < 3 || sc.Args[1] != "agentapi" || sc.Args[2] != "new-conversation" {
				continue
			}
		}

		cron, tz, model, prompt, _ := parseScheduleArgs(sc.Args)

		name := sc.DisplayName
		if name == "" {
			name = sc.DisplayNameAlt
		}
		if name == "" {
			name = sid
		}

		enabled := true
		if en, ok := enabledMap[sid]; ok {
			enabled = en
		}

		status := "STOPPED"
		if enabled {
			status = "READY"
		}

		var wsURIs, grants []string
		if sc.AgentPermissions != nil {
			wsURIs = sc.AgentPermissions.WorkspaceURIs
			if len(wsURIs) == 0 {
				wsURIs = sc.AgentPermissions.WorkspaceURIsAlt
			}
			grants = sc.AgentPermissions.AccessGrants
			if len(grants) == 0 {
				grants = sc.AgentPermissions.AccessGrantsAlt
			}
		} else if sc.AgentPermissionsAlt != nil {
			wsURIs = sc.AgentPermissionsAlt.WorkspaceURIs
			grants = sc.AgentPermissionsAlt.AccessGrants
		}
		primaryWS := ""
		if len(wsURIs) > 0 {
			primaryWS = wsURIs[0]
		}

		result[sid] = &AutomationSchedule{
			ID:             sid,
			Name:           name,
			Description:    sc.Description,
			Status:         status,
			CronExpression: cron,
			TimeZone:       tz,
			Model:          model,
			Prompt:         prompt,
			Enabled:        enabled,
			WorkspaceURI:   primaryWS,
			WorkspaceURIs:  wsURIs,
			ProjectID:      projectIDMap[sid],
			AccessGrants:   grants,
		}
	}

	return result
}

// readLocalSidecarEvents scans ~/.gemini/antigravity/sidecar_data/<sidecarID>/events/
func readLocalSidecarEvents(sidecarID, displayName string, limit int) []AutomationEvent {
	events := make([]AutomationEvent, 0)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return events
	}

	eventsDir := filepath.Join(home, ".gemini", "antigravity", "sidecar_data", sidecarID, "events")
	entries, err := os.ReadDir(eventsDir)
	if err != nil {
		return events
	}

	// Filter and sort entries by filename descending (filenames start with YYYYMMDD_HHMMSS)
	var jsonFiles []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			jsonFiles = append(jsonFiles, e)
		}
	}

	sort.Slice(jsonFiles, func(i, j int) bool {
		return jsonFiles[i].Name() > jsonFiles[j].Name()
	})

	if limit > 0 && len(jsonFiles) > limit {
		jsonFiles = jsonFiles[:limit]
	}

	friendlyName := displayName
	if friendlyName == "" {
		friendlyName = sidecarID
	}

	for _, f := range jsonFiles {
		filePath := filepath.Join(eventsDir, f.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var raw struct {
			TimestampMs string `json:"timestampMs"`
			Error       string `json:"error"`
			Payload     struct {
				NewConversation struct {
					ConversationID string `json:"conversationId"`
					Prompt         string `json:"prompt"`
				} `json:"newConversation"`
			} `json:"payload"`
		}
		if json.Unmarshal(data, &raw) != nil {
			continue
		}

		tsStr := ""
		if raw.TimestampMs != "" {
			if ms, err := strconv.ParseInt(raw.TimestampMs, 10, 64); err == nil && ms > 0 {
				tsStr = time.UnixMilli(ms).In(time.Local).Format("2006-01-02 15:04:05")
			}
		}
		if tsStr == "" {
			if info, err := f.Info(); err == nil {
				tsStr = info.ModTime().In(time.Local).Format("2006-01-02 15:04:05")
			}
		}

		status := "success"
		msg := "自动化执行完成"
		if raw.Error != "" {
			status = "error"
			msg = raw.Error
		} else if raw.Payload.NewConversation.Prompt != "" {
			msg = raw.Payload.NewConversation.Prompt
			if len([]rune(msg)) > 100 {
				msg = string([]rune(msg)[:97]) + "..."
			}
		}

		events = append(events, AutomationEvent{
			ID:             f.Name(),
			ScheduleID:     friendlyName,
			SidecarID:      sidecarID,
			ConversationID: raw.Payload.NewConversation.ConversationID,
			Timestamp:      tsStr,
			Status:         status,
			Message:        msg,
		})
	}

	return events
}

var nextRunRegex = regexp.MustCompile(`\(at\s+(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})\s+UTC\)`)

// extractNextRunFromLog searches ~/.gemini/antigravity/sidecar_data/<sidecarID>/logs/sidecar.log
// for the latest "Next execution scheduled ... (at YYYY-MM-DD HH:MM:SS UTC)" line.
func extractNextRunFromLog(sidecarID string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}

	logPath := filepath.Join(home, ".gemini", "antigravity", "sidecar_data", sidecarID, "logs", "sidecar.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if strings.Contains(line, "Next execution scheduled") {
			matches := nextRunRegex.FindStringSubmatch(line)
			if len(matches) > 1 {
				utcStr := matches[1]
				if t, err := time.Parse("2006-01-02 15:04:05", utcStr); err == nil {
					return t.In(time.Local).Format("2006-01-02 15:04:05")
				}
			}
		}
	}
	return ""
}

// updateLocalSidecarEnabled updates the enabled state of a sidecar in ~/.gemini/config/config.json.
func updateLocalSidecarEnabled(sidecarID string, enabled bool) error {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return err
	}

	configJSONPath := filepath.Join(home, ".gemini", "config", "config.json")
	data, err := os.ReadFile(configJSONPath)
	if err != nil {
		return err
	}

	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return err
	}

	sidecars, ok := root["sidecars"].(map[string]interface{})
	if !ok {
		sidecars = make(map[string]interface{})
		root["sidecars"] = sidecars
	}

	scRaw, ok := sidecars[sidecarID]
	if ok {
		if scMap, ok := scRaw.(map[string]interface{}); ok {
			scMap["enabled"] = enabled
		}
	} else {
		sidecars[sidecarID] = map[string]interface{}{
			"enabled": enabled,
		}
	}

	newBytes, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := configJSONPath + ".tmp"
	if err := os.WriteFile(tmpFile, newBytes, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, configJSONPath)
}
