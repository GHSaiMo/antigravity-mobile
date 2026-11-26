package proxy

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	imgRegex     = regexp.MustCompile(`!\[.*?\]\((https?://[^\s\)]+|/static/[^\s\)]+)\)`)
	titleRegex   = regexp.MustCompile(`title:\s*"([^"]+)"`)
	xmlMetaRegex = regexp.MustCompile(`(?s)<(?:ADDITIONAL_METADATA|USER_SETTINGS_CHANGE)>.*?</(?:ADDITIONAL_METADATA|USER_SETTINGS_CHANGE)>`)
	xmlTagRegex  = regexp.MustCompile(`</?[a-zA-Z0-9_-]+(\s+[^>]*)?>`)

	// P8: single combined regex replaces 4 sequential scans. Groups:
	//   1 = badge-style image link    [![...](...)](/url)
	//   2 = standard markdown image   ![...](url)
	//   3 = MEDIA: prefix             MEDIA: url
	//   4 = link to image file ext    [...](url.png)
	combinedImgRegex = regexp.MustCompile(
		`\[!\[.*?\]\([^\s\)]+\)\]\((https?://[^\s\)]+|/static/[^\s\)]+|file://[^\s\)]+|/[^\s\)]+)\)` +
			`|!\[.*?\]\((https?://[^\s\)]+|/static/[^\s\)]+|file://[^\s\)]+|/[^\s\)]+)\)` +
			`|(?:^|\s|<br\s*/?>)MEDIA:\s*([^\s)<>"'` + "`" + `]+)` +
			`|\[.*?\]\((https?://[^\s\)]+\.(?:png|jpg|jpeg|webp|gif|svg|bmp|heic|ico)|file://[^\s\)]+\.(?:png|jpg|jpeg|webp|gif|svg|bmp|heic|ico)|/[^\s\)]+\.(?:png|jpg|jpeg|webp|gif|svg|bmp|heic|ico))\)`)
)

func extractImageURLsFromText(text string) []string {
	if text == "" {
		return nil
	}
	if !strings.Contains(text, "![") && !strings.Contains(text, "](") && !strings.Contains(text, "MEDIA:") {
		return nil
	}
	var imgURLs []string
	seen := make(map[string]bool)

	// P8: single combined pass over the text instead of 4 separate regex scans.
	matches := combinedImgRegex.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		// Find the first non-empty capture group (groups 1–4 correspond to the 4 alternatives).
		u := ""
		for i := 1; i < len(m); i++ {
			if m[i] != "" {
				u = strings.TrimSpace(m[i])
				u = strings.Trim(u, "`\"'()[]<>")
				break
			}
		}
		if u != "" && !seen[u] {
			seen[u] = true
			imgURLs = append(imgURLs, u)
		}
	}
	return imgURLs
}

// isImageMedia determines whether a given URI or mimeType represents an image file.
func isImageMedia(uri, mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if strings.HasPrefix(mimeType, "image/") {
		return true
	}
	clean := strings.ToLower(strings.TrimSpace(uri))
	if i := strings.IndexAny(clean, "?#"); i >= 0 {
		clean = clean[:i]
	}
	ext := filepath.Ext(clean)
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".svg", ".bmp", ".heic", ".heif", ".ico", ".tiff", ".tif":
		return true
	}
	return false
}

func isArtifactApproval(status interface{}) bool {
	if status == nil {
		return false
	}
	switch v := status.(type) {
	case string:
		u := strings.ToUpper(v)
		return strings.Contains(u, "APPROVED") || u == "1"
	case float64:
		return int(v) == 1
	case int:
		return v == 1
	}
	return false
}

func (p *Proxy) handleCascadeMessages(w http.ResponseWriter, r *http.Request) {
	cascadeID := r.URL.Query().Get("cascadeId")
	if cascadeID == "" {
		http.Error(w, `{"error":"missing cascadeId"}`, http.StatusBadRequest)
		return
	}

	limit := 15
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
			if limit > 50 {
				limit = 50
			}
		}
	}

	offset := -1
	if o := r.URL.Query().Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	fromOffset := -1
	if fo := r.URL.Query().Get("fromOffset"); fo != "" {
		if val, err := strconv.Atoi(fo); err == nil && val >= 0 {
			fromOffset = val
		}
	}

	if verboseRPC {
		slog.Info("[Proxy] CascadeMessages", "cascadeId", cascadeID, "limit", limit, "offset", offset, "fromOffset", fromOffset)
	}

	port, token, _ := p.ensureActiveUpstream(r.Context())
	if port == 0 {
		p.writeUpstreamUnavailable(w)
		return
	}

	if IsDeletedCascade(cascadeID) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"cascade trajectory has been deleted"}`))
		return
	}

	// Fetch or use cached trajectory
	rawResp, err := p.fetchUpstreamTrajectory(cascadeID, port, token)
	if err != nil {
		errBytes, _ := json.Marshal(map[string]string{"error": err.Error()})
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(errBytes)))
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write(errBytes)
		return
	}

	details := p.ParseTrajectoryDetailsCached(rawResp)
	details.Subagents = p.enrichSubagents(details.Subagents)
	for _, sa := range details.Subagents {
		if sa.Status == subagentStatusRunning {
			details.Status = "CASCADE_RUN_STATUS_RUNNING"
			details.HasRunningSubagents = true
			break
		}
	}
	details.AllMessages = p.enrichSubagentMessages(details.AllMessages)
	if details.BranchOrigin != nil && details.BranchOrigin.ParentCascadeID != "" && details.BranchOrigin.ParentTitle == "" {
		originCopy := *details.BranchOrigin
		if pt := p.lookupCascadeTitle(originCopy.ParentCascadeID, port, token); pt != "" {
			originCopy.ParentTitle = pt
		}
		details.BranchOrigin = &originCopy
	}
	if details.BranchOrigin == nil || !details.BranchOrigin.IsSideQuestion {
		details.SideQuestions = p.buildSideQuestionCards(r.Context(), cascadeID, details.AllMessages, port, token)
	}
	if details.Title == "" || details.Title == "未命名会话" {
		if t := p.lookupCascadeTitle(cascadeID, port, token); t != "" {
			details.Title = t
		}
	}
	if qm := p.GetCachedOrFetchPendingMessages(cascadeID, port, token); qm != nil {
		details.QueuedMessages = qm
	} else if details.QueuedMessages == nil {
		details.QueuedMessages = []QueuedMessageItem{}
	}
	details.QueuedMessages = p.FilterQueuedMessagesAgainstTrajectory(cascadeID, details.QueuedMessages, rawResp.Trajectory.Steps, details.AllMessages)
	details.ContextMeter = withLastActivityAt(
		p.fetchUpstreamGeneratorMetadata(cascadeID, port, token),
		extractLastActivityAt(rawResp.Trajectory.Steps),
	)
	liveTranslations := p.ApplyLiveTextTranslations(cascadeID, &details, extractRequestLangHint(r), true)

	totalMsgs := len(details.AllMessages)
	sliced := make([]CascadeMessageItem, 0)
	hasMore := false
	nextOffset := 0

	const maxRangeMessages = 500
	if fromOffset >= 0 {
		// Range catch-up fetch for jumping directly to an earlier turn in the outline:
		// returns [targetStart : targetEnd), capped at maxRangeMessages per request.
		targetEnd := offset
		if targetEnd < 0 || targetEnd > totalMsgs {
			targetEnd = totalMsgs
		}
		targetStart := fromOffset
		if targetStart > targetEnd {
			targetStart = targetEnd
		}
		if targetEnd-targetStart > maxRangeMessages {
			targetStart = targetEnd - maxRangeMessages
		}
		sliced = details.AllMessages[targetStart:targetEnd]
		hasMore = (targetStart > 0)
		nextOffset = targetStart
	} else if offset < 0 {
		// Initial fetch: return latest 'limit' messages
		start := totalMsgs - limit
		if start < 0 {
			start = 0
		}
		sliced = details.AllMessages[start:totalMsgs]
		hasMore = (start > 0)
		nextOffset = start
	} else {
		// Paging back: return 'limit' messages before 'offset'
		targetEnd := offset
		if targetEnd > totalMsgs {
			targetEnd = totalMsgs
		}
		targetStart := targetEnd - limit
		if targetStart < 0 {
			targetStart = 0
		}
		sliced = details.AllMessages[targetStart:targetEnd]
		hasMore = (targetStart > 0)
		nextOffset = targetStart
	}
	if sliced == nil {
		sliced = []CascadeMessageItem{}
	}

	sugResponses, sugSource := p.ResolveOrScheduleNextPrompt(cascadeID, &details, nil, false)
	desktopModel, desktopModelName := readDesktopSelectedModel()
	respBytes, err := json.Marshal(CascadeMessagesResponse{
		CascadeID:               cascadeID,
		Title:                   details.Title,
		Status:                  details.Status,
		HasError:                details.HasError,
		ErrorMessage:            details.ErrorMessage,
		Duration:                details.Duration,
		TotalSteps:              details.TotalSteps,
		TotalTools:              details.TotalTools,
		TotalMessages:           totalMsgs,
		HasMore:                 hasMore,
		NextOffset:              nextOffset,
		Messages:                sliced,
		TurnOutline:             details.TurnOutline,
		QueuedMessages:          details.QueuedMessages,
		RunningTasks:            details.RunningTasks,
		Subagents:               details.Subagents,
		ParentConversation:      details.ParentConversation,
		SubagentRole:            details.SubagentRole,
		BranchOrigin:            details.BranchOrigin,
		SideQuestions:           details.SideQuestions,
		ActiveModel:             details.ActiveModel,
		ActiveModelName:         details.ActiveModelName,
		ModelDisplayName:        details.ModelDisplayName,
		DesktopActiveModel:      desktopModel,
		DesktopActiveModelName:  desktopModelName,
		StartedAt:               details.StartedAt,
		CascadeConfig:           details.CascadeConfig,
		CascadeConfigRaw:        details.CascadeConfigRaw,
		CanProceed:              details.CanProceed,
		ProceedArtifactURI:      details.ProceedArtifactURI,
		PendingInteraction:      details.PendingInteraction,
		Swarm:                   GetSwarmBrief(cascadeID),
		ContextMeter:            sanitizeContextMeterForClient(details.ContextMeter),
		SuggestedResponses:      sugResponses,
		SuggestedResponseSource: sugSource,
		TaskProgress:            details.TaskProgress,
		ActiveFocus:             details.ActiveFocus,
		ActiveFocusStarted:      details.ActiveFocusStarted,
		ActivePlan:              details.ActivePlan,
		LiveTextTranslations:    liveTranslations,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sum := sha256.Sum256(respBytes)
	etag := fmt.Sprintf(`"mgy-%x"`, sum[:12])

	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")
	if ifNoneMatchMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(respBytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBytes)
}

// ifNoneMatchMatches implements weak ETag comparison (RFC 9110 §13.1.2) so Cloudflare's
// automatic W/"..." conversion under gzip/zstd edge compression still hits 304 Not Modified.
func ifNoneMatchMatches(headerVal, currentETag string) bool {
	headerVal = strings.TrimSpace(headerVal)
	if headerVal == "" || currentETag == "" {
		return false
	}
	if headerVal == "*" {
		return true
	}
	normTarget := normalizeWeakETag(currentETag)
	for _, part := range strings.Split(headerVal, ",") {
		if normalizeWeakETag(part) == normTarget {
			return true
		}
	}
	return false
}

func normalizeWeakETag(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "W/") || strings.HasPrefix(s, "w/") {
		s = strings.TrimSpace(s[2:])
	}
	return s
}

// extractErrorText retrieves the most descriptive error message from a trajectory step.
func extractErrorText(s TrajectoryStep) string {
	if s.ErrorMessage != nil {
		e := s.ErrorMessage.Error
		userMsg := strings.TrimSpace(e.UserErrorMessage)
		shortErr := strings.TrimSpace(e.ShortError)
		modelErr := strings.TrimSpace(e.ModelErrorMessage)
		fullErr := strings.TrimSpace(e.FullError)

		if userMsg != "" && shortErr != "" && userMsg != shortErr && !strings.Contains(shortErr, userMsg) {
			return fmt.Sprintf("%s\n%s", userMsg, shortErr)
		}
		if shortErr != "" {
			return shortErr
		}
		if userMsg != "" {
			return userMsg
		}
		if modelErr != "" {
			return modelErr
		}
		if fullErr != "" {
			return fullErr
		}
	}
	if s.Error != nil {
		if s.Error.ShortError != "" {
			return s.Error.ShortError
		}
		if s.Error.FullError != "" {
			return s.Error.FullError
		}
	}
	return "Agent execution terminated due to error."
}

// isUserVisibleError determines whether an error step should be shown to the user.
// In the official Antigravity IDE, only errors with shouldShowUser == true are rendered in normal mode.
// Internal stream interruption retries and continuation errors (shouldShowModel == true) are hidden.
func isUserVisibleError(s TrajectoryStep) bool {
	if s.ErrorMessage != nil {
		if s.ErrorMessage.ShouldShowUser {
			return true
		}
		if s.ErrorMessage.ShouldShowModel {
			return false
		}
		// If neither flag was set, check for known transient retry errors
		short := strings.ToLower(s.ErrorMessage.Error.ShortError)
		userMsg := strings.ToLower(s.ErrorMessage.Error.UserErrorMessage)
		if strings.Contains(short, "stream was interrupted") || strings.Contains(userMsg, "stream was interrupted") ||
			strings.Contains(short, "model produced invalid output") || strings.Contains(userMsg, "model produced invalid output") {
			return false
		}
		return true
	}
	if s.Error != nil {
		short := strings.ToLower(s.Error.ShortError)
		if strings.Contains(short, "stream was interrupted") || strings.Contains(short, "model produced invalid output") {
			return false
		}
		return true
	}
	return false
}

// SanitizeTitle cleans and formats any raw prompt or title to a concise, single-line title (max 36 runes).
func SanitizeTitle(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || s == "未命名会话" {
		return ""
	}

	// Fast-path: strip XML metadata blocks if present
	if strings.Contains(s, "<ADDITIONAL_METADATA>") || strings.Contains(s, "<USER_SETTINGS_CHANGE>") {
		s = xmlMetaRegex.ReplaceAllString(s, "")
		s = strings.TrimSpace(s)
	}

	// Bound input length before regex/line scanning since the maximum title is only 36 runes
	if len(s) > 512 {
		r := []rune(s)
		if len(r) > 150 {
			s = string(r[:150])
		}
	}

	// Fast-path: only execute tag-stripping regex if '<' character exists
	if strings.IndexByte(s, '<') >= 0 {
		s = xmlTagRegex.ReplaceAllString(s, "")
		s = strings.TrimSpace(s)
	}

	lines := strings.Split(s, "\n")
	for _, l := range lines {
		line := strings.TrimSpace(l)
		if line != "" {
			runes := []rune(line)
			if len(runes) > 36 {
				return string(runes[:36]) + "..."
			}
			return line
		}
	}
	return ""
}

func extractTitleFromUserInput(s TrajectoryStep) string {
	if s.UserInput == nil {
		return ""
	}
	prompt := strings.TrimSpace(s.UserInput.UserResponse)
	if prompt == "" && len(s.UserInput.Items) > 0 {
		prompt = strings.TrimSpace(s.UserInput.Items[0].Text)
	}
	return SanitizeTitle(prompt)
}

// SetLastKnownCascadeConfig caches the latest known valid cascade config.
func SetLastKnownCascadeConfig(cfg json.RawMessage) {
	if len(cfg) == 0 || string(cfg) == "null" || string(cfg) == "{}" {
		return
	}
	defaultTrajCache.lastKnownConfigMu.Lock()
	defaultTrajCache.lastKnownConfig = cfg
	defaultTrajCache.lastKnownConfigMu.Unlock()
}

// SetCascadeModel records the active model name and cascadeConfig explicitly chosen/applied for a cascade.
func SetCascadeModel(cascadeID, modelName string, cfg json.RawMessage) {
	if cascadeID == "" {
		return
	}
	if modelName != "" {
		defaultTrajCache.cascadeModelsMu.Lock()
		if defaultTrajCache.cascadeModels == nil {
			defaultTrajCache.cascadeModels = make(map[string]string)
		}
		if len(defaultTrajCache.cascadeModels) > maxMetadataMapSize {
			// Trim half of entries when bound is reached
			count := 0
			for k := range defaultTrajCache.cascadeModels {
				delete(defaultTrajCache.cascadeModels, k)
				count++
				if count >= maxMetadataMapSize/2 {
					break
				}
			}
		}
		defaultTrajCache.cascadeModels[cascadeID] = modelName
		defaultTrajCache.cascadeModelsMu.Unlock()
	}
	if len(cfg) > 0 && string(cfg) != "null" && string(cfg) != "{}" {
		defaultTrajCache.cascadeConfigsMu.Lock()
		if defaultTrajCache.cascadeConfigs == nil {
			defaultTrajCache.cascadeConfigs = make(map[string]json.RawMessage)
		}
		if len(defaultTrajCache.cascadeConfigs) > maxMetadataMapSize {
			count := 0
			for k := range defaultTrajCache.cascadeConfigs {
				delete(defaultTrajCache.cascadeConfigs, k)
				count++
				if count >= maxMetadataMapSize/2 {
					break
				}
			}
		}
		defaultTrajCache.cascadeConfigs[cascadeID] = cfg
		defaultTrajCache.cascadeConfigsMu.Unlock()
		SetLastKnownCascadeConfig(cfg)
	}
}

// GetCascadeModel retrieves any explicitly recorded model name and config for a cascade.
func GetCascadeModel(cascadeID string) (string, json.RawMessage) {
	if cascadeID == "" {
		return "", nil
	}
	defaultTrajCache.cascadeModelsMu.RLock()
	model := ""
	if defaultTrajCache.cascadeModels != nil {
		model = defaultTrajCache.cascadeModels[cascadeID]
	}
	defaultTrajCache.cascadeModelsMu.RUnlock()

	defaultTrajCache.cascadeConfigsMu.RLock()
	var cfg json.RawMessage
	if defaultTrajCache.cascadeConfigs != nil {
		cfg = defaultTrajCache.cascadeConfigs[cascadeID]
	}
	defaultTrajCache.cascadeConfigsMu.RUnlock()

	return model, cfg
}

// SetCascadePlanExecute records whether a cascade is known to be running in Plan & Execute
// (Orchestrator) mode so switching back to a regular model reliably clears planExecute even
// if the client's cascadeConfigRaw omitted planExecuteConfig.
func SetCascadePlanExecute(cascadeID string, enabled bool) {
	if cascadeID == "" {
		return
	}
	defaultTrajCache.cascadeModelsMu.Lock()
	defer defaultTrajCache.cascadeModelsMu.Unlock()
	if !enabled {
		if defaultTrajCache.cascadePlanExecute != nil {
			delete(defaultTrajCache.cascadePlanExecute, cascadeID)
		}
		return
	}
	if defaultTrajCache.cascadePlanExecute == nil {
		defaultTrajCache.cascadePlanExecute = make(map[string]bool)
	}
	if len(defaultTrajCache.cascadePlanExecute) > maxMetadataMapSize {
		count := 0
		for k := range defaultTrajCache.cascadePlanExecute {
			delete(defaultTrajCache.cascadePlanExecute, k)
			count++
			if count >= maxMetadataMapSize/2 {
				break
			}
		}
	}
	defaultTrajCache.cascadePlanExecute[cascadeID] = true
}

// IsCascadePlanExecute reports whether cascadeID was observed in Plan & Execute mode.
func IsCascadePlanExecute(cascadeID string) bool {
	if cascadeID == "" {
		return false
	}
	defaultTrajCache.cascadeModelsMu.RLock()
	defer defaultTrajCache.cascadeModelsMu.RUnlock()
	if defaultTrajCache.cascadePlanExecute == nil {
		return false
	}
	return defaultTrajCache.cascadePlanExecute[cascadeID]
}

// GetCascadeConfig retrieves the cascade config for the given conversation or falls back to last known.
func (p *Proxy) GetCascadeConfig(cascadeID string, port int, token string) json.RawMessage {
	if cascadeID != "" {
		if _, recordedCfg := GetCascadeModel(cascadeID); len(recordedCfg) > 0 {
			return recordedCfg
		}
	}
	if cascadeID != "" && port > 0 {
		if rawResp, err := p.fetchUpstreamTrajectory(cascadeID, port, token); err == nil && rawResp != nil {
			metas := rawResp.Trajectory.ExecutorMetadatas
			for i := len(metas) - 1; i >= 0; i-- {
				cfg := metas[i].CascadeConfig
				if len(cfg) > 0 && string(cfg) != "null" && string(cfg) != "{}" {
					SetLastKnownCascadeConfig(cfg)
					return cfg
				}
			}
		}
	}
	if cascadeID == "" {
		defaultTrajCache.lastKnownConfigMu.RLock()
		defer defaultTrajCache.lastKnownConfigMu.RUnlock()
		if len(defaultTrajCache.lastKnownConfig) > 0 {
			return defaultTrajCache.lastKnownConfig
		}
	}
	return nil
}

func parseTime(str string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, str); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, str)
}

func formatDuration(sec int) string {
	if sec <= 0 {
		return "0秒"
	}
	hours := sec / 3600
	minutes := (sec % 3600) / 60
	seconds := sec % 60
	if hours > 0 {
		return fmt.Sprintf("%d小时%d分", hours, minutes)
	} else if minutes > 0 {
		return fmt.Sprintf("%d分%d秒", minutes, seconds)
	}
	return fmt.Sprintf("%d秒", seconds)
}
