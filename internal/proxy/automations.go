package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AutomationSchedule represents a scheduled automation task (from schedule sidecar).
type AutomationSchedule struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Status         string `json:"status"` // READY, STOPPED, ERROR
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

	port, token := p.ActiveUpstream()
	if cur := p.insp.Current(); cur != nil && cur.IsHealthy && cur.Port > 0 {
		port = cur.Port
		token = cur.CSRFToken
	}
	if port <= 0 {
		writeJSONError(w, "upstream language_server unavailable", http.StatusServiceUnavailable)
		return
	}

	resp := AutomationsResponse{
		Automations: make([]AutomationSchedule, 0),
		Events:      make([]AutomationEvent, 0),
	}

	// Query GetSidecars
	sidecarsURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetSidecars", port)
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, sidecarsURL, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	res, err := p.mediumClient.Do(req)
	if err == nil {
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		var raw struct {
			Sidecars []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Builtin     string `json:"builtin"`
				Status      string `json:"status"`
				Description string `json:"description"`
				Metadata    map[string]string `json:"metadata"`
			} `json:"sidecars"`
		}
		if json.Unmarshal(body, &raw) == nil {
			for _, sc := range raw.Sidecars {
				if sc.Builtin == "schedule" || strings.Contains(strings.ToLower(sc.Name), "schedule") || strings.Contains(strings.ToLower(sc.Name), "automation") {
					cron := ""
					prompt := ""
					if sc.Metadata != nil {
						cron = sc.Metadata["cron"]
						prompt = sc.Metadata["prompt"]
					}
					resp.Automations = append(resp.Automations, AutomationSchedule{
						ID:             sc.ID,
						Name:           sc.Name,
						Description:    sc.Description,
						Status:         sc.Status,
						CronExpression: cron,
						Prompt:         prompt,
						Enabled:        sc.Status == "READY" || sc.Status == "RUNNING",
					})
				}
			}
		}
	}

	// Query GetSidecarEvents
	eventsURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetSidecarEvents", port)
	req2, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, eventsURL, strings.NewReader(`{"limit":20}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req2.Header.Set("x-codeium-csrf-token", token)
	}
	res2, err := p.mediumClient.Do(req2)
	if err == nil {
		defer res2.Body.Close()
		body, _ := io.ReadAll(res2.Body)
		var rawEvents struct {
			Events []struct {
				EventID        string `json:"eventId"`
				SidecarID      string `json:"sidecarId"`
				ConversationID string `json:"conversationId"`
				Timestamp      string `json:"timestamp"`
				Status         string `json:"status"`
				Message        string `json:"message"`
			} `json:"events"`
		}
		if json.Unmarshal(body, &rawEvents) == nil {
			for _, ev := range rawEvents.Events {
				resp.Events = append(resp.Events, AutomationEvent{
					ID:             ev.EventID,
					ScheduleID:     ev.SidecarID,
					ConversationID: ev.ConversationID,
					Timestamp:      ev.Timestamp,
					Status:         ev.Status,
					Message:        ev.Message,
				})
			}
		}
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
		Action       string `json:"action"` // "start", "stop", "restart"
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
	if strings.TrimSpace(req.SidecarID) == "" {
		writeJSONError(w, "sidecarId cannot be empty", http.StatusBadRequest)
		return
	}

	port, token := p.ActiveUpstream()
	if cur := p.insp.Current(); cur != nil && cur.IsHealthy && cur.Port > 0 {
		port = cur.Port
		token = cur.CSRFToken
	}
	if port <= 0 {
		writeJSONError(w, "upstream language_server unavailable", http.StatusServiceUnavailable)
		return
	}

	manageURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/ManageSidecar", port)
	bodyBytes, _ := json.Marshal(map[string]interface{}{
		"sidecarId": req.SidecarID,
		"action":    req.Action,
	})
	upReq, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, manageURL, bytes.NewReader(bodyBytes))
	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		upReq.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.mediumClient.Do(upReq)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"sidecarId": req.SidecarID,
		"action":    req.Action,
	})
}
