package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
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

// PluginAuthAccountInfo represents one connected account under a plugin auth provider.
type PluginAuthAccountInfo struct {
	AccountID       string `json:"accountId"`
	Alias           string `json:"alias,omitempty"`
	Identity        string `json:"identity,omitempty"`
	Label           string `json:"label,omitempty"`
	IsDefault       bool   `json:"isDefault,omitempty"`
	NeedsAuth       bool   `json:"needsAuth,omitempty"`
	NeedsAuthReason string `json:"needsAuthReason,omitempty"`
}

// PluginAuthProvider represents a normalised plugin authentication provider entry.
type PluginAuthProvider struct {
	ID                string                  `json:"id"`
	Name              string                  `json:"name"`
	SignedIn          bool                    `json:"signedIn"`
	Plugin            string                  `json:"plugin,omitempty"`
	Provider          string                  `json:"provider,omitempty"`
	Identity          string                  `json:"identity,omitempty"`
	NeedsAuth         bool                    `json:"needsAuth,omitempty"`
	NeedsAuthReason   string                  `json:"needsAuthReason,omitempty"`
	LastSignInError   string                  `json:"lastSignInError,omitempty"`
	SetupURL          string                  `json:"setupUrl,omitempty"`
	SetupInstructions string                  `json:"setupInstructions,omitempty"`
	SignInPending     bool                    `json:"signInPending,omitempty"`
	Accounts          []PluginAuthAccountInfo `json:"accounts,omitempty"`
}

// PluginPermissionGrant represents one persisted plugin permission grant (Antigravity 2.35+).
type PluginPermissionGrant struct {
	Grant        string `json:"grant"`
	PluginID     string `json:"pluginId,omitempty"`
	Plugin       string `json:"plugin,omitempty"`
	Description  string `json:"description,omitempty"`
	Action       string `json:"action,omitempty"`
	ActionLabel  string `json:"actionLabel,omitempty"`
	ResourceName string `json:"resourceName,omitempty"`
	AnyResource  bool   `json:"anyResource,omitempty"`
	Scope        string `json:"scope,omitempty"`
	ProjectID    string `json:"projectId,omitempty"`
	ProjectName  string `json:"projectName,omitempty"`
}

// PluginListResponse is returned by GET /gateway/plugins.
type PluginListResponse struct {
	Plugins          []PluginItem            `json:"plugins"`
	Providers        []PluginAuthProvider    `json:"providers,omitempty"`
	PermissionGrants []PluginPermissionGrant `json:"permissionGrants,omitempty"`
}

// HandleGetPlugins serves GET /gateway/plugins.
func (p *Proxy) HandleGetPlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	port, token := p.ActiveUpstream()
	if p.insp != nil {
		if cur := p.insp.Current(); cur != nil && cur.IsHealthy && cur.Port > 0 {
			port = cur.Port
			token = cur.CSRFToken
		}
	}
	if port <= 0 {
		writeJSONError(w, "upstream language_server unavailable", http.StatusServiceUnavailable)
		return
	}

	resp := PayloadPluginsResp{
		Plugins: make([]PluginItem, 0),
	}

	// Query GetSidecars first (or concurrently) to resolve runtime enabled/running state
	// for sidecar-backed builtin/global plugins such as swarm.
	type sidecarState struct {
		sidecarID  string
		pluginPath string
		status     string
		enabled    bool
		hasEnabled bool
	}
	fetchUpstreamJSON := func(method string) ([]byte, bool) {
		u := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/%s", port, method)
		reqUp, err := http.NewRequestWithContext(r.Context(), http.MethodPost, u, strings.NewReader("{}"))
		if err != nil {
			return nil, false
		}
		reqUp.Header.Set("Content-Type", "application/json")
		reqUp.Header.Set("Connect-Protocol-Version", "1")
		if token != "" {
			reqUp.Header.Set("x-codeium-csrf-token", token)
		}
		resUp, err := p.mediumClient.Do(reqUp)
		if err != nil {
			return nil, false
		}
		defer resUp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resUp.Body, 4<<20))
		return b, resUp.StatusCode >= 200 && resUp.StatusCode < 300
	}

	var (
		scBody, allPluginsBody, authProvidersBody, grantsBody []byte
		okSc, okAll, okAuth, okGrants                         bool
		wg                                                    sync.WaitGroup
	)
	wg.Add(4)
	go func() {
		defer wg.Done()
		scBody, okSc = fetchUpstreamJSON("GetSidecars")
	}()
	go func() {
		defer wg.Done()
		allPluginsBody, okAll = fetchUpstreamJSON("GetAllPlugins")
	}()
	go func() {
		defer wg.Done()
		authProvidersBody, okAuth = fetchUpstreamJSON("ListPluginAuthProviders")
	}()
	go func() {
		defer wg.Done()
		grantsBody, okGrants = fetchUpstreamJSON("ListPluginPermissionGrants")
	}()
	wg.Wait()

	var sidecars []sidecarState
	if okSc {
		var rawSc struct {
			Sidecars []struct {
				SidecarID  string `json:"sidecarId"`
				Status     string `json:"status"`
				PluginPath string `json:"pluginPath"`
				UserConfig *struct {
					Enabled *bool `json:"enabled"`
				} `json:"userConfig"`
			} `json:"sidecars"`
		}
		if json.Unmarshal(scBody, &rawSc) == nil {
			for _, sc := range rawSc.Sidecars {
				st := sidecarState{
					sidecarID:  strings.TrimSpace(sc.SidecarID),
					pluginPath: strings.TrimSpace(sc.PluginPath),
					status:     strings.TrimSpace(sc.Status),
				}
				if sc.UserConfig != nil && sc.UserConfig.Enabled != nil {
					st.hasEnabled = true
					st.enabled = *sc.UserConfig.Enabled
				}
				sidecars = append(sidecars, st)
			}
		}
	}

	// Parse GetAllPlugins
	if okAll {
		body := allPluginsBody
		var raw struct {
			Plugins []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				DisplayName string `json:"displayName"`
				Path        string `json:"path"`
				Description string `json:"description"`
				Version     string `json:"version"`
				Enabled     *bool  `json:"enabled"`
				Disabled    bool   `json:"disabled"`
				IsGlobal    bool   `json:"isGlobal"`
				IsBuiltin   bool   `json:"isBuiltin"`
				Kind        string `json:"kind"`
				Sidecars    []struct {
					SidecarID string `json:"sidecarId"`
				} `json:"sidecars"`
			} `json:"plugins"`
		}
		if json.Unmarshal(body, &raw) == nil {
			seenPlugins := make(map[string]bool, len(raw.Plugins))
			for idx, pl := range raw.Plugins {
				pid := strings.TrimSpace(pl.ID)
				if pid == "" {
					pid = strings.TrimSpace(pl.Name)
				}
				if pid == "" {
					pid = fmt.Sprintf("plugin-%d", idx)
				}
				if seenPlugins[pid] {
					base := pid
					for n := 2; seenPlugins[pid]; n++ {
						pid = fmt.Sprintf("%s-%d", base, n)
					}
				}
				seenPlugins[pid] = true

				displayName := strings.TrimSpace(pl.DisplayName)
				if displayName == "" {
					displayName = strings.TrimSpace(pl.Name)
				}
				if displayName == "" {
					displayName = pid
				}
				kind := strings.TrimSpace(pl.Kind)
				if kind == "" {
					if pl.IsBuiltin {
						kind = "builtin"
					} else if pl.IsGlobal {
						kind = "global"
					}
				}

				// Determine enabled state: GetAllPlugins omits "enabled" when active
				// and exposes sidecar runtime/userConfig state via GetSidecars.
				enabled := !pl.Disabled
				if !pl.Disabled && pl.Enabled != nil {
					enabled = *pl.Enabled
				} else if !pl.Disabled && len(sidecars) > 0 {
					pluginSidecarIDs := make(map[string]bool, len(pl.Sidecars))
					for _, ps := range pl.Sidecars {
						if ps.SidecarID != "" {
							pluginSidecarIDs[ps.SidecarID] = true
						}
					}
					matchedAny := false
					anyActive := false
					for _, sc := range sidecars {
						matches := pluginSidecarIDs[sc.sidecarID] ||
							(pl.Path != "" && sc.pluginPath == pl.Path) ||
							(pl.Name != "" && strings.HasPrefix(sc.sidecarID, pl.Name+"/"))
						if !matches {
							continue
						}
						matchedAny = true
						if (sc.hasEnabled && sc.enabled) ||
							sc.status == "SIDECAR_STATUS_RUNNING" ||
							sc.status == "RUNNING" ||
							sc.status == "READY" {
							anyActive = true
							break
						}
					}
					if matchedAny {
						enabled = anyActive
					}
				}

				resp.Plugins = append(resp.Plugins, PluginItem{
					ID:          pid,
					Name:        displayName,
					Description: pl.Description,
					Version:     pl.Version,
					Enabled:     enabled,
					Installed:   true,
					Kind:        kind,
				})
			}
		}
	}

	// Parse ListPluginAuthProviders
	if okAuth {
		body := authProvidersBody
		// Antigravity 2.28.1+ returns {plugin, provider, connected, identity, accounts}.
		// Older builds used {id, name, signedIn}. Accept both shapes, then normalise:
		// the mobile clients treat the provider id as a list key (Android LazyColumn
		// keys must be unique), so it has to be non-empty and unique even if upstream
		// omits every identifying field.
		var rawAuth struct {
			Providers []struct {
				ID                string                  `json:"id"`
				Name              string                  `json:"name"`
				DisplayName       string                  `json:"displayName"`
				SignedIn          bool                    `json:"signedIn"`
				Plugin            string                  `json:"plugin"`
				PluginID          string                  `json:"pluginId"`
				PluginName        string                  `json:"pluginName"`
				Provider          string                  `json:"provider"`
				Connected         bool                    `json:"connected"`
				Identity          string                  `json:"identity"`
				NeedsAuth         bool                    `json:"needsAuth"`
				NeedsAuthReason   string                  `json:"needsAuthReason"`
				LastSignInError   string                  `json:"lastSignInError"`
				SetupURL          string                  `json:"setupUrl"`
				SetupInstructions string                  `json:"setupInstructions"`
				SignInPending     bool                    `json:"signInPending"`
				Accounts          []PluginAuthAccountInfo `json:"accounts"`
			} `json:"providers"`
		}
		if json.Unmarshal(body, &rawAuth) == nil {
			seen := make(map[string]bool, len(rawAuth.Providers))
			for i, pv := range rawAuth.Providers {
				plugin := strings.TrimSpace(pv.Plugin)
				if plugin == "" {
					plugin = strings.TrimSpace(pv.PluginID)
				}
				provider := strings.TrimSpace(pv.Provider)
				legacyID := strings.TrimSpace(pv.ID)
				legacyName := strings.TrimSpace(pv.Name)
				displayName := strings.TrimSpace(pv.DisplayName)
				pluginName := strings.TrimSpace(pv.PluginName)

				var id, display string
				if plugin != "" || provider != "" {
					if plugin != "" && provider != "" && !strings.EqualFold(plugin, provider) {
						id = plugin + ":" + provider
					} else if plugin != "" {
						id = plugin
					} else {
						id = provider
					}

					pluginLabel := pluginName
					if pluginLabel == "" {
						pluginLabel = plugin
					}
					if displayName != "" {
						display = displayName
					} else if pluginLabel != "" && provider != "" && !strings.EqualFold(pluginLabel, provider) {
						display = fmt.Sprintf("%s (%s)", pluginLabel, provider)
					} else if pluginLabel != "" {
						display = pluginLabel
					} else {
						display = provider
					}
				} else {
					id = legacyID
					if id == "" {
						id = legacyName
					}
					display = displayName
					if display == "" {
						display = legacyName
					}
					if display == "" {
						display = legacyID
					}
				}

				if id == "" {
					id = fmt.Sprintf("provider-%d", i)
				}
				if seen[id] {
					base := id
					for n := 2; seen[id]; n++ {
						id = fmt.Sprintf("%s-%d", base, n)
					}
				}
				seen[id] = true

				if display == "" {
					display = id
				}

				resp.Providers = append(resp.Providers, PluginAuthProvider{
					ID:                id,
					Name:              display,
					SignedIn:          pv.Connected || pv.SignedIn,
					Plugin:            plugin,
					Provider:          provider,
					Identity:          strings.TrimSpace(pv.Identity),
					NeedsAuth:         pv.NeedsAuth,
					NeedsAuthReason:   strings.TrimSpace(pv.NeedsAuthReason),
					LastSignInError:   strings.TrimSpace(pv.LastSignInError),
					SetupURL:          strings.TrimSpace(pv.SetupURL),
					SetupInstructions: strings.TrimSpace(pv.SetupInstructions),
					SignInPending:     pv.SignInPending,
					Accounts:          pv.Accounts,
				})
			}
		}
	}

	// Parse ListPluginPermissionGrants (Antigravity 2.35+; absent on older servers)
	if okGrants && len(grantsBody) > 2 {
		var rawGrants struct {
			Grants []PluginPermissionGrant `json:"grants"`
		}
		if json.Unmarshal(grantsBody, &rawGrants) == nil && len(rawGrants.Grants) > 0 {
			for _, g := range rawGrants.Grants {
				g.Grant = strings.TrimSpace(g.Grant)
				if g.Grant == "" {
					continue
				}
				g.PluginID = strings.TrimSpace(g.PluginID)
				g.Plugin = strings.TrimSpace(g.Plugin)
				if g.Plugin == "" {
					g.Plugin = g.PluginID
				}
				g.Description = strings.TrimSpace(g.Description)
				g.Action = strings.TrimSpace(g.Action)
				g.ActionLabel = strings.TrimSpace(g.ActionLabel)
				g.ResourceName = strings.TrimSpace(g.ResourceName)
				g.Scope = strings.TrimSpace(g.Scope)
				g.ProjectID = strings.TrimSpace(g.ProjectID)
				g.ProjectName = strings.TrimSpace(g.ProjectName)
				resp.PermissionGrants = append(resp.PermissionGrants, g)
			}
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
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
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
	if p.insp != nil {
		if cur := p.insp.Current(); cur != nil && cur.IsHealthy && cur.Port > 0 {
			port = cur.Port
			token = cur.CSRFToken
		}
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

// HandleRevokePluginPermissionGrant serves POST /gateway/plugins/grants/revoke (Antigravity 2.35+).
func (p *Proxy) HandleRevokePluginPermissionGrant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Grant     string `json:"grant"`
		Scope     string `json:"scope"`
		ProjectID string `json:"projectId"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.Grant = strings.TrimSpace(req.Grant)
	req.Scope = strings.TrimSpace(req.Scope)
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	if req.Grant == "" {
		writeJSONError(w, "grant cannot be empty", http.StatusBadRequest)
		return
	}

	port, token := p.ActiveUpstream()
	if p.insp != nil {
		if cur := p.insp.Current(); cur != nil && cur.IsHealthy && cur.Port > 0 {
			port = cur.Port
			token = cur.CSRFToken
		}
	}
	if port <= 0 {
		writeJSONError(w, "upstream language_server unavailable", http.StatusServiceUnavailable)
		return
	}

	payload := map[string]interface{}{
		"grant": req.Grant,
	}
	if req.Scope != "" {
		payload["scope"] = req.Scope
	}
	if req.ProjectID != "" {
		payload["projectId"] = req.ProjectID
	}
	bodyBytes, _ := json.Marshal(payload)

	revokeURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/RevokePluginPermissionGrant", port)
	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, revokeURL, bytes.NewReader(bodyBytes))
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
		writeJSONError(w, "revoke request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		writeJSONError(w, fmt.Sprintf("revoke failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody))), resp.StatusCode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":    true,
		"grant": req.Grant,
	})
}
