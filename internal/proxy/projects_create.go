package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

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

	// CSRF Protection: Validate Origin or Referer if present
	origin := r.Header.Get("Origin")
	if origin == "" {
		if ref := r.Header.Get("Referer"); ref != "" {
			if u, err := url.Parse(ref); err == nil {
				origin = fmt.Sprintf("%s://%s", u.Scheme, u.Host)
			}
		}
	}
	if origin != "" && !IsAllowedOrigin(origin, r.Host) {
		slog.Warn(fmt.Sprintf("[Proxy] Rejected CreateCascade from untrusted origin: %s (host: %s)", origin, r.Host))
		http.Error(w, "Forbidden: untrusted origin", http.StatusForbidden)
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

	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024) // 1MB limit to prevent DoS
	var req CreateCascadeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	clientMsgID := strings.TrimSpace(r.Header.Get("X-Client-Message-Id"))
	if clientMsgID == "" {
		clientMsgID = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	if clientMsgID != "" {
		if cachedID := p.getCascadeDedup(clientMsgID, 60*time.Second); cachedID != "" {
			slog.Info(fmt.Sprintf("[Proxy] Deduplicated repeat CreateCascade via clientMsgID %s -> cascade %s", clientMsgID, cachedID))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(CreateCascadeResponse{
				CascadeID: cachedID,
				Status:    "ok",
			})
			return
		}
	}

	wsURI := req.WorkspaceURI
	if wsURI != "" && !strings.HasPrefix(wsURI, "file://") {
		wsURI = "file://" + filepath.Clean(wsURI)
	}

	// Determine projectId: prefer explicitly provided projectId, otherwise match against known projects
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" && wsURI != "" {
		projectID = p.FindProjectIDForWorkspaces([]string{wsURI})
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
	} else if wsURI == "" || wsURI == "file://" {
		// Pure conversation (Chat / outside of project)
		startPayload["source"] = "CORTEX_TRAJECTORY_SOURCE_CASCADE_CLIENT"
		startPayload["workspaceUris"] = []string{}
		startPayload["projectEnvConfig"] = map[string]interface{}{
			"projectId":                 "outside-of-project",
			"defaultProjectEnvironment": map[string]interface{}{},
		}
	} else {
		startPayload["source"] = "CORTEX_TRAJECTORY_SOURCE_INTERACTIVE_CASCADE"
		startPayload["workspaceUris"] = []string{wsURI}
	}

	modelEnum := resolveModelEnum(req.Model)
	if modelEnum != "" {
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

	startResp, err := p.mediumClient.Do(httpReq)
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
	if clientMsgID != "" {
		p.setCascadeDedup(clientMsgID, cascadeID)
	}
	slog.Info(fmt.Sprintf("[Proxy] ✨ 新建会话: %s", shortCascadeID(cascadeID)))

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
			if annResp, err := p.mediumClient.Do(annReq); err == nil {
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

		var cfgObj interface{}
		if cfg := p.GetCascadeConfig(cascadeID, port, token); len(cfg) > 0 {
			_ = json.Unmarshal(cfg, &cfgObj)
		}
		if modelEnum != "" {
			canonicalName := canonicalModelName(req.Model)
			if canonicalName == "" {
				canonicalName = req.Model
			}
			cfgObj = applyModelToCascadeConfig(cfgObj, modelEnum, canonicalName)
			if cfgBytes, err := json.Marshal(cfgObj); err == nil {
				SetCascadeModel(cascadeID, canonicalName, cfgBytes)
			}
		}
		if cfgObj != nil {
			msgPayload["cascadeConfig"] = cfgObj
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
			if msgResp, err := p.mediumClient.Do(msgReq); err == nil {
				msgResp.Body.Close()
				ClearTrajectoryCache(cascadeID)
				if verboseRPC {
					slog.Info(fmt.Sprintf("[Proxy] Dispatched initial prompt to cascade %s", shortCascadeID(cascadeID)))
				}
			} else {
				slog.Warn("⚠️  [Proxy] Warning: failed to dispatch initial prompt", "err", err)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CreateCascadeResponse{
		CascadeID: cascadeID,
		Status:    "ok",
	})
}

// resolveModelEnum resolves a user or client provided model name to its protobuf enum string.
func resolveModelEnum(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if strings.HasPrefix(model, "MODEL_") {
		if model == "MODEL_GOOGLE_GEMINI_2_5_PRO" || model == "MODEL_GOOGLE_GEMINI_2_5_FLASH" {
			return "MODEL_PLACEHOLDER_M318"
		}
		return model
	}
	if enum, ok := modelEnumMap[strings.ToLower(model)]; ok {
		return enum
	}
	return ""
}

// canonicalModelName converts a model enum or friendly alias into its canonical model name.
func canonicalModelName(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if name, ok := enumToCanonicalMap[model]; ok {
		return name
	}
	enum := resolveModelEnum(model)
	if name, ok := enumToCanonicalMap[enum]; ok {
		return name
	}
	return model
}

// applyModelToCascadeConfig patches or injects the given modelEnum and modelName into cfgObj,
// and ensures checkpointConfig limits comply with model context window constraints.
func applyModelToCascadeConfig(cfgObj interface{}, modelEnum string, modelName string) interface{} {
	if modelEnum == "" {
		return cfgObj
	}
	if modelName == "" {
		modelName = canonicalModelName(modelEnum)
	}

	cfgMap, ok := cfgObj.(map[string]interface{})
	if !ok || cfgMap == nil {
		cfgMap = make(map[string]interface{})
	}

	var plannerConfig map[string]interface{}
	if p, ok := cfgMap["plannerConfig"].(map[string]interface{}); ok && p != nil {
		plannerConfig = p
	} else {
		plannerConfig = make(map[string]interface{})
	}

	plannerConfig["planModel"] = modelEnum
	plannerConfig["requestedModel"] = map[string]interface{}{
		"model": modelEnum,
	}
	plannerConfig["modelName"] = modelName
	cfgMap["plannerConfig"] = plannerConfig

	// Adjust checkpointConfig to match model's context window constraints.
	// Gemini: 1M context, allows maxTokenLimit: 256000, tokenThreshold: 140000.
	// Claude Opus 4.6 Thinking: 250k context, maxOutputTokens: 64000.
	// LanguageServer asserts: maxTokenLimit <= ContextWindow (250000) - MaxOutputTokens (64000) = 186000.
	// Desktop Antigravity uses maxTokenLimit: 160000, tokenThreshold: 50000 for Claude.
	isClaude := strings.Contains(strings.ToLower(modelEnum), "claude") ||
		strings.Contains(strings.ToLower(modelName), "claude") ||
		modelEnum == "MODEL_PLACEHOLDER_M26"

	var checkpointConfig map[string]interface{}
	if cp, ok := cfgMap["checkpointConfig"].(map[string]interface{}); ok && cp != nil {
		checkpointConfig = cp
	} else {
		checkpointConfig = make(map[string]interface{})
	}

	if isClaude {
		checkpointConfig["maxTokenLimit"] = 160000
		checkpointConfig["tokenThreshold"] = 50000
		checkpointConfig["isSync"] = false
		checkpointConfig["useLastPlannerModel"] = false
	} else {
		// Restore Gemini limits if previously clamped
		if limit, ok := getNumberAsInt(checkpointConfig["maxTokenLimit"]); ok && limit <= 160000 {
			checkpointConfig["maxTokenLimit"] = 256000
		}
		if thresh, ok := getNumberAsInt(checkpointConfig["tokenThreshold"]); ok && thresh <= 50000 {
			checkpointConfig["tokenThreshold"] = 140000
		}
		checkpointConfig["isSync"] = true
		checkpointConfig["useLastPlannerModel"] = true
	}
	cfgMap["checkpointConfig"] = checkpointConfig

	return cfgMap
}

func getNumberAsInt(val interface{}) (int, bool) {
	if val == nil {
		return 0, false
	}
	switch v := val.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}
