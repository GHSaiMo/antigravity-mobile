package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// PluginItem represents an installed or available plugin.
type PluginItem struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Version     string   `json:"version,omitempty"`
	Enabled     bool     `json:"enabled"`
	Installed   bool     `json:"installed"`
	Kind        string   `json:"kind,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	AuthStatus  string   `json:"authStatus,omitempty"`
	Accounts    []string `json:"accounts,omitempty"`
}

// PluginListResponse is returned by GET /gateway/plugins.
type PluginListResponse struct {
	Plugins   []PluginItem `json:"plugins"`
	Providers []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		SignedIn bool   `json:"signedIn"`
	} `json:"providers,omitempty"`
}

// HandleGetPlugins serves GET /gateway/plugins.
func (p *Proxy) HandleGetPlugins(w http.ResponseWriter, r *http.Request) {
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

	resp := PayloadPluginsResp{
		Plugins: make([]PluginItem, 0),
	}
	// Query GetAllPlugins
	allURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetAllPlugins", port)
	req1, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, allURL, strings.NewReader("{}"))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req1.Header.Set("x-codeium-csrf-token", token)
	}

	res1, err := p.mediumClient.Do(req1)
	if err == nil {
		defer res1.Body.Close()
		body, _ := io.ReadAll(res1.Body)
		var raw struct {
			Plugins []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				Description string `json:"description"`
				Version     string `json:"version"`
				Enabled     bool   `json:"enabled"`
				Kind        string `json:"kind"`
			} `json:"plugins"`
		}
		if json.Unmarshal(body, &raw) == nil {
			for _, pl := range raw.Plugins {
				pid := strings.TrimSpace(pl.ID)
				if pid == "" {
					pid = pl.Name
				}
				resp.Plugins = append(resp.Plugins, PluginItem{
					ID:          pid,
					Name:        pl.Name,
					Description: pl.Description,
					Version:     pl.Version,
					Enabled:     pl.Enabled,
					Installed:   true,
					Kind:        pl.Kind,
				})
			}
		}
	}

	// Query ListPluginAuthProviders
	authURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/ListPluginAuthProviders", port)
	req2, _ := http.NewRequestWithContext(r.Context(), http.MethodPost, authURL, strings.NewReader("{}"))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req2.Header.Set("x-codeium-csrf-token", token)
	}
	res2, err := p.mediumClient.Do(req2)
	if err == nil {
		defer res2.Body.Close()
		body, _ := io.ReadAll(res2.Body)
		var rawAuth struct {
			Providers []struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				SignedIn bool   `json:"signedIn"`
			} `json:"providers"`
		}
		if json.Unmarshal(body, &rawAuth) == nil {
			resp.Providers = rawAuth.Providers
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

type PayloadPluginsResp = PluginListResponse

// HandleInstallPlugin serves POST /gateway/plugins/install.
func (p *Proxy) HandleInstallPlugin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PluginID          string `json:"pluginId"`
		PluginIDAlt       string `json:"id"`
		CascadeID         string `json:"cascadeId"`
		CascadeIDAlt      string `json:"conversation_id"`
		CascadeIDAlt2     string `json:"conversationId"`
		AutoResume        *bool  `json:"autoResume"`
		AutoResumeAlt     *bool  `json:"auto_resume"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	pluginID := strings.TrimSpace(req.PluginID)
	if pluginID == "" {
		pluginID = strings.TrimSpace(req.PluginIDAlt)
	}
	if req.CascadeID == "" {
		if req.CascadeIDAlt2 != "" {
			req.CascadeID = req.CascadeIDAlt2
		} else {
			req.CascadeID = req.CascadeIDAlt
		}
	}
	autoResume := false
	if req.AutoResume != nil {
		autoResume = *req.AutoResume
	} else if req.AutoResumeAlt != nil {
		autoResume = *req.AutoResumeAlt
	}
	if pluginID == "" {
		writeJSONError(w, "pluginId cannot be empty", http.StatusBadRequest)
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

	installPayload := map[string]interface{}{
		"pluginId": pluginID,
	}
	if req.CascadeID != "" {
		installPayload["cascadeId"] = req.CascadeID
	}
	bodyBytes, _ := json.Marshal(installPayload)

	installURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/InstallCascadePlugin", port)
	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, installURL, bytes.NewReader(bodyBytes))
	if err != nil {
		writeJSONError(w, "failed to build request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		upReq.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.mediumClient.Do(upReq)
	if err != nil {
		writeJSONError(w, "install request failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		writeJSONError(w, fmt.Sprintf("install failed (%d): %s", resp.StatusCode, string(respBody)), resp.StatusCode)
		return
	}

	resumed := false
	if autoResume && req.CascadeID != "" {
		time.Sleep(200 * time.Millisecond)
		resumeText := fmt.Sprintf("已成功安装插件 %s，请继续执行。", pluginID)
		if err := p.sendCascadeTextMessage(req.CascadeID, resumeText); err == nil {
			resumed = true
			slog.Info("[Plugins] Auto-resumed conversation after installing plugin", "cascadeId", shortCascadeID(req.CascadeID), "pluginId", pluginID)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"success":   true,
		"id":        pluginID,
		"pluginId":  pluginID,
		"installed": true,
		"resumed":   resumed,
		"message":   "插件安装成功",
	})
}
