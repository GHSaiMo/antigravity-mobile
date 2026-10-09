package proxy

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
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

	if verboseRPC {
		slog.Info("[Proxy] CascadeMessages", "cascadeId", cascadeID, "limit", limit, "offset", offset)
	}

	p.mu.RLock()
	port := p.activePort
	token := p.activeToken
	p.mu.RUnlock()

	if port == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":"Antigravity upstream not connected"}`))
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

	details := p.ParseTrajectoryDetails(rawResp)
	details.Subagents = p.enrichSubagents(details.Subagents)
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

	totalMsgs := len(details.AllMessages)
	var sliced []CascadeMessageItem
	hasMore := false
	nextOffset := 0

	if offset < 0 {
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

	respBytes, err := json.Marshal(CascadeMessagesResponse{
		CascadeID:          cascadeID,
		Title:              details.Title,
		Status:             details.Status,
		HasError:           details.HasError,
		ErrorMessage:       details.ErrorMessage,
		Duration:           details.Duration,
		TotalSteps:         details.TotalSteps,
		TotalTools:         details.TotalTools,
		TotalMessages:      totalMsgs,
		HasMore:            hasMore,
		NextOffset:         nextOffset,
		Messages:           sliced,
		QueuedMessages:     details.QueuedMessages,
		RunningTasks:       details.RunningTasks,
		Subagents:          details.Subagents,
		ParentConversation: details.ParentConversation,
		SubagentRole:       details.SubagentRole,
		ActiveModel:        details.ActiveModel,
		ActiveModelName:    details.ActiveModelName,
		ModelDisplayName:   details.ModelDisplayName,
		StartedAt:          details.StartedAt,
		CascadeConfig:      details.CascadeConfig,
		CascadeConfigRaw:   details.CascadeConfigRaw,
		CanProceed:         details.CanProceed,
		ProceedArtifactURI: details.ProceedArtifactURI,
		PendingInteraction: details.PendingInteraction,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(respBytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBytes)
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
