package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var gitHubSlugSegmentRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)

// normalizeGitHubSlug validates and extracts a strict "owner/repo" slug from a GitHub URL or slug.
// Non-GitHub URLs, option-like flags ("-..."), traversal segments, and malformed inputs return "".
func normalizeGitHubSlug(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || strings.HasPrefix(s, "-") {
		return ""
	}
	fromURL := false
	switch {
	case strings.HasPrefix(s, "git@github.com:"):
		s = strings.TrimPrefix(s, "git@github.com:")
		fromURL = true
	case strings.HasPrefix(s, "https://github.com/"):
		s = strings.TrimPrefix(s, "https://github.com/")
		fromURL = true
	case strings.HasPrefix(s, "http://github.com/"):
		s = strings.TrimPrefix(s, "http://github.com/")
		fromURL = true
	default:
		if strings.Contains(s, "://") || strings.Contains(s, "@") {
			return ""
		}
	}
	if idx := strings.IndexAny(s, "?#"); idx >= 0 {
		s = s[:idx]
	}
	s = strings.Trim(s, "/")
	parts := strings.Split(s, "/")
	if len(parts) < 2 || (!fromURL && len(parts) != 2) {
		return ""
	}
	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")
	if owner == "." || owner == ".." || repo == "." || repo == ".." {
		return ""
	}
	if !gitHubSlugSegmentRe.MatchString(owner) || !gitHubSlugSegmentRe.MatchString(repo) {
		return ""
	}
	return owner + "/" + repo
}

// CreateCascadeRequest represents payload to start a new cascade.
type CreateCascadeRequest struct {
	WorkspaceURI  string `json:"workspaceUri"`
	Prompt        string `json:"prompt"`
	Model         string `json:"model,omitempty"`
	ProjectID     string `json:"projectId,omitempty"`
	EnvironmentID string `json:"environmentId,omitempty"`
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

	port, token, _ := p.ensureActiveUpstream(r.Context())
	if port == 0 {
		p.writeUpstreamUnavailable(w)
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

	wsURI := strings.TrimSpace(req.WorkspaceURI)
	projectID := strings.TrimSpace(req.ProjectID)
	var validWSPath string
	if wsURI != "" && wsURI != "file://" {
		if isRemoteURI(wsURI) {
			if projectID == "" {
				projectID = p.FindProjectIDForWorkspaces([]string{wsURI})
			}
			if projectID == "" {
				http.Error(w, "Forbidden: unregistered remote workspace URI", http.StatusForbidden)
				return
			}
		} else {
			resolvedWS, wsErr := validateWorkspaceDir(wsURI, false)
			if wsErr != nil && !hasExplicitAllowedWorkspaceRoots() {
				// Cold-start fallback: refresh discovered projects (ReadProjects RPC / state.vscdb / trajectories)
				// so a registered workspace outside $HOME is recognized even before GET /gateway/projects.
				if projects, err := p.GetProjects(); err == nil && len(projects) > 0 {
					paths := make([]string, 0, len(projects))
					for _, prj := range projects {
						if isRemoteURI(prj.URI) {
							continue
						}
						if prj.Path != "" {
							paths = append(paths, prj.Path)
						} else if prj.URI != "" {
							paths = append(paths, prj.URI)
						}
					}
					syncDiscoveredWorkspaceRoots(paths)
				}
				resolvedWS, wsErr = validateWorkspaceDir(wsURI, false)
			}
			if wsErr != nil {
				http.Error(w, wsErr.Error(), http.StatusForbidden)
				return
			}
			validWSPath = resolvedWS
			if !strings.HasPrefix(wsURI, "file://") {
				wsURI = filePathToURI(resolvedWS)
			}
		}
	}

	// Determine projectId: prefer explicitly provided projectId, otherwise match against known projects
	if projectID == "" && wsURI != "" {
		candidates := []string{wsURI}
		if validWSPath != "" {
			if canonURI := filePathToURI(validWSPath); canonURI != wsURI {
				candidates = append(candidates, canonURI)
			}
		}
		projectID = p.FindProjectIDForWorkspaces(candidates)
	}
	if projectID == "" && validWSPath != "" {
		wsURI = filePathToURI(validWSPath)
	}

	// 1. Call StartCascade RPC upstream
	startPayload := map[string]interface{}{}
	envID := strings.TrimSpace(req.EnvironmentID)
	if projectID != "" {
		// When project environment config is provided, language_server strictly requires
		// workspaceUris to be empty ([]), otherwise it errors with invalid_argument.
		startPayload["source"] = "CORTEX_TRAJECTORY_SOURCE_CASCADE_CLIENT"
		startPayload["workspaceUris"] = []string{}
		envCfg := map[string]interface{}{
			"projectId": projectID,
		}
		if envID != "" {
			envCfg["environmentId"] = envID
		} else {
			envCfg["defaultProjectEnvironment"] = map[string]interface{}{}
		}
		startPayload["projectEnvConfig"] = envCfg
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

	p.ensureLiveModels(r.Context())
	if strings.TrimSpace(req.Model) == "" {
		req.Model = "gemini-3.8-flash-high"
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

	// Update lastUserViewTime annotation upstream so desktop client recognizes it immediately.
	// For Orchestrator cascades, also mark planExecute: true so desktop shows Orchestrator mode.
	annotations := map[string]interface{}{
		"lastUserViewTime": time.Now().UTC().Format("2006-01-02T15:04:05.999Z"),
	}
	if _, isOrchSpec := resolveOrchestratorSpec(req.Model); isOrchSpec || isBareOrchestratorAnchor(req.Model) {
		annotations["planExecute"] = true
		SetCascadePlanExecute(cascadeID, true)
	}
	annPayload := map[string]interface{}{
		"cascadeId":   cascadeID,
		"annotations": annotations,
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
		if clientMessageIDRe.MatchString(clientMsgID) {
			msgPayload["clientMessageId"] = clientMsgID
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
			recordedName := effectiveCascadeModelName(cfgObj, canonicalName)
			if cfgBytes, err := json.Marshal(cfgObj); err == nil {
				SetCascadeModel(cascadeID, recordedName, cfgBytes)
			}
		}
		if cfgObj != nil {
			msgPayload["cascadeConfig"] = cfgObj
			applyOrchestratorAgentSpec(msgPayload, cfgObj)
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
	} else if modelEnum != "" {
		// Record the requested model & config even when creating an empty cascade (e.g. before
		// uploading media/attachments or starting a voice call) so subsequent requests inherit
		// the exact Orchestrator spec rather than falling back to bare anchors.
		var cfgObj interface{}
		if cfg := p.GetCascadeConfig(cascadeID, port, token); len(cfg) > 0 {
			_ = json.Unmarshal(cfg, &cfgObj)
		}
		canonicalName := canonicalModelName(req.Model)
		if canonicalName == "" {
			canonicalName = req.Model
		}
		cfgObj = applyModelToCascadeConfig(cfgObj, modelEnum, canonicalName)
		recordedName := effectiveCascadeModelName(cfgObj, canonicalName)
		if cfgBytes, err := json.Marshal(cfgObj); err == nil {
			SetCascadeModel(cascadeID, recordedName, cfgBytes)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(CreateCascadeResponse{
		CascadeID: cascadeID,
		Status:    "ok",
	})
}

// bareOrchestratorAnchorFamily maps a legacy/anchor single-variant Orchestrator ID or enum
// to its planner family slug ("neon", "carbon", "argon"). Returns "" if not a bare anchor.
func bareOrchestratorAnchorFamily(model string) string {
	s := strings.TrimSpace(model)
	switch strings.ToUpper(s) {
	case "MODEL_PLACEHOLDER_M537":
		return "neon"
	case "MODEL_PLACEHOLDER_M667":
		return "carbon"
	case "MODEL_PLACEHOLDER_M681":
		return "argon"
	}
	switch strings.ToLower(s) {
	case "gemini-pro-orchestrator-neon", "orchestrator-neon":
		return "neon"
	case "gemini-pro-orchestrator", "gemini-pro-coordinator", "orchestrator-carbon":
		return "carbon"
	case "gemini-pro-orchestrator-argon", "orchestrator-argon":
		return "argon"
	}
	return ""
}

// isBareOrchestratorAnchor reports legacy/anchor single-variant Orchestrator IDs or enums
// that do not encode a thinking-effort tier.
func isBareOrchestratorAnchor(model string) bool {
	return bareOrchestratorAnchorFamily(model) != ""
}

// resolveBareOrchestratorAnchorSpec resolves a bare Orchestrator anchor ID or enum (such as
// "MODEL_PLACEHOLDER_M537" or "gemini-pro-orchestrator-neon") to a default effort variant
// with a valid PlannerEnum and ExecutionEnum so Plan & Execute cascades never lose
// planExecuteConfig.executionModel.
func resolveBareOrchestratorAnchorSpec(model string) (orchestratorSpec, bool) {
	family := bareOrchestratorAnchorFamily(model)
	if family == "" {
		return orchestratorSpec{}, false
	}
	if spec, ok := liveModels.defaultOrchestratorSpec(family); ok {
		return spec, true
	}
	for _, eff := range []string{"high", "medium", "xhigh", "low"} {
		if spec, ok := resolveStaticOrchestratorSpec("gemini-pro-orchestrator-" + family + "-" + eff); ok {
			return spec, true
		}
	}
	return liveModels.defaultOrchestratorSpec("")
}

// resolveStaticOrchestratorSpec resolves a synthetic Orchestrator+effort ID before liveModels
// has finished refreshing (cold-start safety net).
func resolveStaticOrchestratorSpec(id string) (orchestratorSpec, bool) {
	lower := strings.ToLower(strings.TrimSpace(id))
	const prefix = "gemini-pro-orchestrator-"
	if !strings.HasPrefix(lower, prefix) {
		return orchestratorSpec{}, false
	}
	rest := strings.TrimPrefix(lower, prefix)
	basePart, effort := splitModelBaseAndEffort(rest)
	if basePart == "" || effort == "" {
		return orchestratorSpec{}, false
	}
	normEffort := effort
	if normEffort == "extra-high" {
		normEffort = "xhigh"
	}
	var plannerID string
	switch basePart {
	case "neon", "carbon", "argon", "hydrogen":
		plannerID = fmt.Sprintf("gemini-4-%s-%s", basePart, normEffort)
	case "opus-5-5", "sonnet-5-5":
		plannerID = fmt.Sprintf("claude-%s-%s", basePart, normEffort)
	default:
		return orchestratorSpec{}, false
	}
	plannerEnum := liveModels.enumForID(plannerID)
	if plannerEnum == "" {
		plannerEnum = modelEnumMap[plannerID]
	}
	if plannerEnum == "" {
		return orchestratorSpec{}, false
	}
	execID := "gemini-4-hydrogen-high"
	execEnum := liveModels.enumForID(execID)
	if execEnum == "" {
		execEnum = modelEnumMap[execID]
	}
	if execEnum == "" || execEnum == plannerEnum {
		execID = "claude-sonnet-5-5-high"
		execEnum = modelEnumMap[execID]
	}
	if execEnum == "" || execEnum == plannerEnum {
		return orchestratorSpec{}, false
	}
	return orchestratorSpec{
		ID:               lower,
		PlannerModelID:   plannerID,
		PlannerEnum:      normalizeLegacyEnum(plannerEnum),
		ExecutionModelID: execID,
		ExecutionEnum:    normalizeLegacyEnum(execEnum),
		AnchorEnum:       "MODEL_PLACEHOLDER_M537",
	}, true
}

// resolveSyntheticOrchestratorSpec resolves only explicit synthetic Orchestrator+effort IDs
// (not bare single-variant anchors like MODEL_PLACEHOLDER_M537).
func resolveSyntheticOrchestratorSpec(id string) (orchestratorSpec, bool) {
	if spec, ok := liveModels.orchestratorSpecForID(id); ok {
		return spec, true
	}
	return resolveStaticOrchestratorSpec(id)
}

// resolveOrchestratorSpec resolves either a synthetic Orchestrator+effort ID or a bare
// Orchestrator anchor to a complete orchestratorSpec with valid PlannerEnum and ExecutionEnum.
func resolveOrchestratorSpec(id string) (orchestratorSpec, bool) {
	if spec, ok := resolveSyntheticOrchestratorSpec(id); ok {
		return spec, true
	}
	return resolveBareOrchestratorAnchorSpec(id)
}

// staticOrchestratorIDForPlannerEnum reverse-maps a planner model enum to its synthetic
// Orchestrator+effort ID when liveModels has not yet populated orchByPlannerEnum (cold start).
// Excludes executor-only families such as hydrogen so alternating planner/executor metadata
// entries do not misclassify an executor sub-step as the planner.
func staticOrchestratorIDForPlannerEnum(enum string) string {
	trimmed := strings.TrimSpace(enum)
	if trimmed == "" {
		return ""
	}
	if orchID := liveModels.orchestratorIDForPlannerEnum(trimmed); orchID != "" {
		return orchID
	}
	if isBareOrchestratorAnchor(trimmed) {
		if spec, ok := resolveBareOrchestratorAnchorSpec(trimmed); ok {
			return spec.ID
		}
		return ""
	}
	canon := liveModels.idForEnum(trimmed)
	if canon == "" {
		canon = enumToCanonicalMap[trimmed]
	}
	if canon == "" {
		return ""
	}
	base, effort := splitModelBaseAndEffort(canon)
	if base == "" || effort == "" {
		return ""
	}
	shortSlug := orchestratorShortSlug(base)
	switch shortSlug {
	case "neon", "carbon", "argon", "opus-5-5", "sonnet-5-5":
	default:
		return ""
	}
	if effort == "extra-high" {
		effort = "xhigh"
	}
	candidate := fmt.Sprintf("gemini-pro-orchestrator-%s-%s", shortSlug, effort)
	if spec, ok := resolveSyntheticOrchestratorSpec(candidate); ok {
		return spec.ID
	}
	return ""
}

// defaultOrchestratorModelID returns a valid synthetic Orchestrator+effort ID fallback for
// Plan & Execute cascades whose exact planner enum cannot be mapped.
func defaultOrchestratorModelID() string {
	if spec, ok := liveModels.defaultOrchestratorSpec(""); ok && spec.ID != "" {
		return spec.ID
	}
	if spec, ok := resolveStaticOrchestratorSpec("gemini-pro-orchestrator-neon-high"); ok {
		return spec.ID
	}
	return "gemini-pro-orchestrator-neon-high"
}

func isSyntheticOrchestratorID(id string) bool {
	_, ok := resolveSyntheticOrchestratorSpec(id)
	return ok
}

// effectiveCascadeModelName extracts plannerConfig.modelName from cfgObj if present,
// falling back to fallback when unavailable.
func effectiveCascadeModelName(cfgObj interface{}, fallback string) string {
	if cfgMap, ok := cfgObj.(map[string]interface{}); ok && cfgMap != nil {
		if pCfg, ok := cfgMap["plannerConfig"].(map[string]interface{}); ok && pCfg != nil {
			if mn, ok := pCfg["modelName"].(string); ok && strings.TrimSpace(mn) != "" {
				return strings.TrimSpace(mn)
			}
		}
	}
	return fallback
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
	// 实时列表是事实来源：客户端发来的 id 正是从它里面选的
	if enum := liveModels.enumForID(model); enum != "" {
		return normalizeLegacyEnum(enum)
	}
	if spec, ok := resolveStaticOrchestratorSpec(model); ok && spec.PlannerEnum != "" {
		return normalizeLegacyEnum(spec.PlannerEnum)
	}
	if enum, ok := modelEnumMap[strings.ToLower(model)]; ok {
		return enum
	}
	return ""
}

// normalizeLegacyEnum keeps the historical remap of the retired Gemini 2.5 enums.
func normalizeLegacyEnum(enum string) string {
	if enum == "MODEL_GOOGLE_GEMINI_2_5_PRO" || enum == "MODEL_GOOGLE_GEMINI_2_5_FLASH" {
		return "MODEL_PLACEHOLDER_M318"
	}
	return enum
}

// canonicalModelName converts a model enum or friendly alias into its canonical model name.
func canonicalModelName(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	// Preserve synthetic Orchestrator+effort IDs verbatim; their underlying PlannerEnum
	// intentionally shares the regular model's enum (e.g. MODEL_PLACEHOLDER_M532) and must
	// not collapse back to the non-orchestrator ID.
	if spec, ok := resolveSyntheticOrchestratorSpec(model); ok {
		return spec.ID
	}
	if name, ok := enumToCanonicalMap[model]; ok {
		return name
	}
	enum := resolveModelEnum(model)
	if name, ok := enumToCanonicalMap[enum]; ok {
		return name
	}
	// 静态表没有的新枚举：用实时列表反查出 id，而不是把裸枚举名当模型名
	if id := liveModels.idForEnum(model); id != "" {
		return id
	}
	if id := liveModels.idForEnum(enum); id != "" {
		return id
	}
	return model
}

// extractOrchestratorIDFromConfig infers a synthetic Orchestrator+effort ID from an existing
// cascadeConfig that already carries planExecuteConfig.executionModel.
func extractOrchestratorIDFromConfig(cfgObj interface{}) string {
	if !cascadeConfigHasPlanExecute(cfgObj) {
		return ""
	}
	cfgMap, ok := cfgObj.(map[string]interface{})
	if !ok || cfgMap == nil {
		return ""
	}
	pCfg, ok := cfgMap["plannerConfig"].(map[string]interface{})
	if !ok || pCfg == nil {
		return ""
	}
	if mn, _ := pCfg["modelName"].(string); mn != "" && isSyntheticOrchestratorID(mn) {
		return canonicalModelName(mn)
	}
	pm, _ := pCfg["planModel"].(string)
	if pm == "" {
		if rm, ok := pCfg["requestedModel"].(map[string]interface{}); ok {
			pm, _ = rm["model"].(string)
		}
	}
	if pm != "" {
		if orchID := staticOrchestratorIDForPlannerEnum(pm); orchID != "" {
			return orchID
		}
	}
	return ""
}

// applyModelToCascadeConfig patches or injects the given modelEnum and modelName into cfgObj,
// and ensures checkpointConfig limits comply with model context window constraints.
func applyModelToCascadeConfig(cfgObj interface{}, modelEnum string, modelName string) interface{} {
	if modelEnum == "" && strings.TrimSpace(modelName) == "" {
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

	spec, isOrchSpec := resolveOrchestratorSpec(modelName)
	if !isOrchSpec && modelEnum != "" {
		spec, isOrchSpec = resolveOrchestratorSpec(modelEnum)
	}
	if isOrchSpec && spec.PlannerEnum != "" && spec.ExecutionEnum != "" {
		modelEnum = spec.PlannerEnum
		modelName = spec.ID
		plannerConfig["planModel"] = spec.PlannerEnum
		plannerConfig["requestedModel"] = map[string]interface{}{
			"model": spec.PlannerEnum,
		}
		plannerConfig["modelName"] = spec.ID
		plannerConfig["planExecuteConfig"] = map[string]interface{}{
			"executionModel": spec.ExecutionEnum,
		}
	} else if modelEnum != "" {
		plannerConfig["planModel"] = modelEnum
		plannerConfig["requestedModel"] = map[string]interface{}{
			"model": modelEnum,
		}
		plannerConfig["modelName"] = modelName
		// Switching away from a dynamic Orchestrator turn must clear planExecuteConfig
		// (matching Antigravity Desktop's jKc).
		delete(plannerConfig, "planExecuteConfig")
	} else {
		return cfgObj
	}
	cfgMap["plannerConfig"] = plannerConfig

	// Adjust checkpointConfig to match model's context window constraints.
	// Gemini: 1M context, allows maxTokenLimit: 256000, tokenThreshold: 140000.
	// Claude Opus 4.6 Thinking: 250k context, maxOutputTokens: 64000.
	// LanguageServer asserts: maxTokenLimit <= ContextWindow (250000) - MaxOutputTokens (64000) = 186000.
	// Desktop Antigravity uses maxTokenLimit: 160000, tokenThreshold: 50000 for Claude.
	isClaude := isClaudeModelOrEnum(modelEnum) || isClaudeModelOrEnum(modelName)

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

// cascadeConfigHasPlanExecute reports whether cfgObj configures plannerConfig.planExecuteConfig.executionModel.
func cascadeConfigHasPlanExecute(cfgObj interface{}) bool {
	cfgMap, ok := cfgObj.(map[string]interface{})
	if !ok || cfgMap == nil {
		return false
	}
	pCfg, ok := cfgMap["plannerConfig"].(map[string]interface{})
	if !ok || pCfg == nil {
		return false
	}
	pec, ok := pCfg["planExecuteConfig"].(map[string]interface{})
	if !ok || pec == nil {
		return false
	}
	execModel, _ := pec["executionModel"].(string)
	return strings.TrimSpace(execModel) != "" && !strings.EqualFold(strings.TrimSpace(execModel), "MODEL_UNSPECIFIED")
}

// applyOrchestratorAgentSpec mirrors Antigravity Desktop's mob()+nob(): whenever
// cascadeConfig carries plannerConfig.planExecuteConfig.executionModel, ensure
// customAgentSpec.builtinAgent.defaultAgent.planExecute is true on SendUserCascadeMessage;
// otherwise clear any stale planExecute=true already present in msgPayload.customAgentSpec.
func applyOrchestratorAgentSpec(msgPayload map[string]interface{}, cfgObj interface{}) {
	if msgPayload == nil {
		return
	}
	if !cascadeConfigHasPlanExecute(cfgObj) {
		if specMap, ok := msgPayload["customAgentSpec"].(map[string]interface{}); ok && specMap != nil {
			if builtin, ok := specMap["builtinAgent"].(map[string]interface{}); ok && builtin != nil {
				if defaultAgent, ok := builtin["defaultAgent"].(map[string]interface{}); ok && defaultAgent != nil {
					defaultAgent["planExecute"] = false
				}
			}
		}
		return
	}
	specMap, ok := msgPayload["customAgentSpec"].(map[string]interface{})
	if !ok || specMap == nil {
		msgPayload["customAgentSpec"] = map[string]interface{}{
			"builtinAgent": map[string]interface{}{
				"defaultAgent": map[string]interface{}{
					"isInteractive": true,
					"surface":       "AGENT_SURFACE_HUB",
					"planExecute":   true,
				},
			},
		}
		return
	}
	builtin, ok := specMap["builtinAgent"].(map[string]interface{})
	if !ok || builtin == nil {
		return
	}
	defaultAgent, ok := builtin["defaultAgent"].(map[string]interface{})
	if !ok || defaultAgent == nil {
		return
	}
	defaultAgent["planExecute"] = true
}

// clearOrchestratorAgentSpec explicitly sets customAgentSpec.builtinAgent.defaultAgent.planExecute = false
// when switching an existing Plan & Execute cascade back to a regular non-Orchestrator model so upstream
// does not keep constructing Plan & Execute tools/hooks without planExecuteConfig.executionModel.
func clearOrchestratorAgentSpec(msgPayload map[string]interface{}) {
	if msgPayload == nil {
		return
	}
	specMap, ok := msgPayload["customAgentSpec"].(map[string]interface{})
	if !ok || specMap == nil {
		msgPayload["customAgentSpec"] = map[string]interface{}{
			"builtinAgent": map[string]interface{}{
				"defaultAgent": map[string]interface{}{
					"isInteractive": true,
					"surface":       "AGENT_SURFACE_HUB",
					"planExecute":   false,
				},
			},
		}
		return
	}
	builtin, ok := specMap["builtinAgent"].(map[string]interface{})
	if !ok || builtin == nil {
		specMap["builtinAgent"] = map[string]interface{}{
			"defaultAgent": map[string]interface{}{
				"isInteractive": true,
				"surface":       "AGENT_SURFACE_HUB",
				"planExecute":   false,
			},
		}
		return
	}
	defaultAgent, ok := builtin["defaultAgent"].(map[string]interface{})
	if !ok || defaultAgent == nil {
		builtin["defaultAgent"] = map[string]interface{}{
			"isInteractive": true,
			"surface":       "AGENT_SURFACE_HUB",
			"planExecute":   false,
		}
		return
	}
	defaultAgent["planExecute"] = false
}

func isClaudeModelOrEnum(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	if strings.Contains(lower, "claude") {
		return true
	}
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "MODEL_PLACEHOLDER_M26", "MODEL_PLACEHOLDER_M35",
		"MODEL_PLACEHOLDER_M400", "MODEL_PLACEHOLDER_M401", "MODEL_PLACEHOLDER_M402",
		"MODEL_PLACEHOLDER_M403", "MODEL_PLACEHOLDER_M404", "MODEL_PLACEHOLDER_M405":
		return true
	}
	return false
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
