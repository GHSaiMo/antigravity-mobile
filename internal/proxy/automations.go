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
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Status         string `json:"status"` // READY, RUNNING, STOPPED, ERROR
	CronExpression string `json:"cronExpression,omitempty"`
	Prompt         string `json:"prompt,omitempty"`
	LastRunAt      string `json:"lastRunAt,omitempty"`
	NextRunAt      string `json:"nextRunAt,omitempty"`
	Enabled        bool   `json:"enabled"`
	WorkspaceURI   string `json:"workspaceUri,omitempty"`
}

// AutomationEvent represents a historical run of an automation.
type AutomationEvent struct {
	ID             string `json:"id"`
	ScheduleID     string `json:"scheduleId"`
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
					SidecarID string `json:"sidecarId"`
					ID        string `json:"id"`
					Status    string `json:"status"`
					Config    struct {
						Builtin       string   `json:"builtin"`
						Args          []string `json:"args"`
						DisplayName   string   `json:"displayName"`
						Description   string   `json:"description"`
						RestartPolicy string   `json:"restartPolicy"`
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

					cron := ""
					prompt := ""
					if len(sc.Config.Args) > 0 {
						cron = sc.Config.Args[0]
					}
					if len(sc.Config.Args) > 3 {
						prompt = sc.Config.Args[3]
					}
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
					if sc.UserConfig != nil && sc.UserConfig.Enabled != nil {
						enabled = *sc.UserConfig.Enabled
						if !enabled {
							status = "STOPPED"
						}
					}

					existing, found := schedulesMap[sid]
					if found {
						existing.Status = status
						existing.Enabled = enabled
						if cron != "" {
							existing.CronExpression = cron
						}
						if prompt != "" {
							existing.Prompt = prompt
						}
						if name != "" && existing.Name == sid {
							existing.Name = name
						}
						if desc != "" && existing.Description == "" {
							existing.Description = desc
						}
					} else {
						schedulesMap[sid] = &AutomationSchedule{
							ID:             sid,
							Name:           name,
							Description:    desc,
							Status:         status,
							CronExpression: cron,
							Prompt:         prompt,
							Enabled:        enabled,
						}
					}
				}
			}
		}
	}

	// 3. Collect events and enrich LastRunAt / NextRunAt
	allEvents := make([]AutomationEvent, 0)
	for sid, sc := range schedulesMap {
		// Read NextRunAt from log file
		if nextRun := extractNextRunFromLog(sid); nextRun != "" {
			sc.NextRunAt = nextRun
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

	// Local state synchronization: if enable/disable, persist to ~/.gemini/config/config.json
	if mappedAction == "SIDECAR_ACTION_ENABLE" {
		_ = updateLocalSidecarEnabled(req.SidecarID, true)
	} else if mappedAction == "SIDECAR_ACTION_DISABLE" {
		_ = updateLocalSidecarEnabled(req.SidecarID, false)
	}

	// Upstream ManageSidecar RPC call
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
	} else if mappedAction == "SIDECAR_ACTION_RUN_NOW" {
		writeJSONError(w, "upstream language_server unavailable for run_now", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"sidecarId": req.SidecarID,
		"action":    req.Action,
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

	// Read config.json to check enabled status
	enabledMap := make(map[string]bool)
	if data, err := os.ReadFile(configJSONPath); err == nil {
		var cfg struct {
			Sidecars map[string]struct {
				Enabled *bool `json:"enabled"`
			} `json:"sidecars"`
		}
		if json.Unmarshal(data, &cfg) == nil {
			for k, v := range cfg.Sidecars {
				if v.Enabled != nil {
					enabledMap[k] = *v.Enabled
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
			Builtin        string   `json:"builtin"`
			Args           []string `json:"args"`
			DisplayName    string   `json:"display_name"`
			DisplayNameAlt string   `json:"displayName"`
			Description    string   `json:"description"`
		}
		if json.Unmarshal(scBytes, &sc) != nil {
			continue
		}

		if sc.Builtin != "schedule" && !strings.Contains(strings.ToLower(sid), "schedule") {
			if len(sc.Args) < 3 || sc.Args[1] != "agentapi" || sc.Args[2] != "new-conversation" {
				continue
			}
		}

		cron := ""
		prompt := ""
		if len(sc.Args) > 0 {
			cron = sc.Args[0]
		}
		if len(sc.Args) > 3 {
			prompt = sc.Args[3]
		}

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

		result[sid] = &AutomationSchedule{
			ID:             sid,
			Name:           name,
			Description:    sc.Description,
			Status:         status,
			CronExpression: cron,
			Prompt:         prompt,
			Enabled:        enabled,
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
