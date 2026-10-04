package proxy

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
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
		ActiveModel:        details.ActiveModel,
		ModelDisplayName:   details.ModelDisplayName,
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

// ParseTrajectoryDetails extracts messages, tools count, duration and metadata from raw response.
func (p *Proxy) ParseTrajectoryDetails(rawResp *upstreamTrajectoryResp) TrajectoryDetails {
	steps := rawResp.Trajectory.Steps
	totalSteps := len(steps)

	var allMessages []CascadeMessageItem
	var lastErrorText string
	pendingTools := 0
	var toolNames []string
	totalToolsCount := 0

	flushTools := func() {
		if pendingTools == 0 {
			return
		}
		item := CascadeMessageItem{
			ID:        fmt.Sprintf("tools-%d", len(allMessages)),
			Type:      "tools",
			Role:      "tools",
			Text:      fmt.Sprintf("已思考并执行 %d 项操作", pendingTools),
			Content:   fmt.Sprintf("已思考并执行 %d 项操作", pendingTools),
			ToolCount: pendingTools,
			ToolNames: toolNames,
		}
		allMessages = append(allMessages, item)
		pendingTools = 0
		toolNames = nil
	}

	for idx, s := range steps {
		stepType := s.Type

		if stepType == "CORTEX_STEP_TYPE_USER_INPUT" {
			flushTools()
			text := ""
			if s.UserInput != nil {
				text = s.UserInput.UserResponse
				if text == "" && len(s.UserInput.Items) > 0 {
					text = s.UserInput.Items[0].Text
				}
			}

			var mediaList []string
			var userImageURLs []string
			seenMedia := make(map[string]bool)
			seenURL := make(map[string]bool)

			addMedia := func(m string) {
				m = strings.TrimSpace(m)
				if m != "" && !seenMedia[m] {
					seenMedia[m] = true
					mediaList = append(mediaList, m)
				}
			}
			addImageURL := func(u string) {
				u = strings.TrimSpace(u)
				if u != "" && !seenURL[u] {
					seenURL[u] = true
					userImageURLs = append(userImageURLs, u)
				}
			}

			if s.UserInput != nil {
				if len(s.UserInput.Media) > 0 {
					for _, m := range s.UserInput.Media {
						if m.URI != "" {
							addImageURL(m.URI)
						}
						if m.Thumbnail != "" {
							addMedia(m.Thumbnail)
						} else if m.InlineData != "" {
							addMedia(m.InlineData)
						}
					}
				}
				if len(s.UserInput.Images) > 0 {
					for _, img := range s.UserInput.Images {
						if img.Base64Data != "" {
							addMedia(img.Base64Data)
						}
					}
				}
			}

			// Also extract any image URLs or MEDIA: tags from user input text
			for _, u := range extractImageURLsFromText(text) {
				addImageURL(u)
			}

			trimmed := strings.TrimSpace(text)
			isSystemApproval := strings.HasPrefix(trimmed, "Comments on artifact URI:") || strings.Contains(trimmed, "The user has approved this document")

			if (trimmed != "" && !isSystemApproval) || len(mediaList) > 0 || len(userImageURLs) > 0 {
				stepIdx := idx
				allMessages = append(allMessages, CascadeMessageItem{
					ID:        fmt.Sprintf("step-%d", idx),
					Type:      "user",
					Role:      "user",
					Text:      text,
					Content:   text,
					StepIndex: &stepIdx,
					Media:     mediaList,
					ImageURLs: userImageURLs,
				})
			}
		} else if stepType == "CORTEX_STEP_TYPE_PLANNER_RESPONSE" {
			respText := ""
			if s.PlannerResponse != nil {
				respText = strings.TrimSpace(s.PlannerResponse.Response)
			}

			if respText != "" {
				flushTools()

				// Extract image URLs
				imgURLs := extractImageURLsFromText(respText)

				stepIdx := idx
				allMessages = append(allMessages, CascadeMessageItem{
					ID:        fmt.Sprintf("step-%d", idx),
					Type:      "agent",
					Role:      "assistant",
					Text:      respText,
					Content:   respText,
					StepIndex: &stepIdx,
					ImageURLs: imgURLs,
				})
			}
		} else if stepType == "CORTEX_STEP_TYPE_ERROR_MESSAGE" {
			if !isUserVisibleError(s) {
				continue
			}
			flushTools()
			lastErrorText = extractErrorText(s)
			allMessages = append(allMessages, CascadeMessageItem{
				ID:      fmt.Sprintf("step-%d", idx),
				Type:    "error",
				Role:    "error",
				Text:    lastErrorText,
				Content: lastErrorText,
			})
		} else if strings.HasPrefix(stepType, "CORTEX_STEP_TYPE_") && stepType != "CORTEX_STEP_TYPE_SYSTEM_MESSAGE" && stepType != "CORTEX_STEP_TYPE_ERROR_MESSAGE" {
			pendingTools++
			totalToolsCount++
			name := extractToolNameFromStep(s)
			toolNames = append(toolNames, name)
		}
	}

	flushTools()

	// Calculate total duration
	duration := "0秒"
	if len(steps) > 0 {
		var firstTime, lastTime time.Time
		for _, s := range steps {
			if s.Metadata.CreatedAt != "" {
				if t, err := parseTime(s.Metadata.CreatedAt); err == nil {
					firstTime = t
					break
				}
			}
		}
		for i := len(steps) - 1; i >= 0; i-- {
			if steps[i].Metadata.CreatedAt != "" {
				if t, err := parseTime(steps[i].Metadata.CreatedAt); err == nil {
					lastTime = t
					break
				}
			}
		}
		if !firstTime.IsZero() && !lastTime.IsZero() {
			diff := int(lastTime.Sub(firstTime).Seconds())
			duration = formatDuration(diff)
		}
	}

	var activeConfig json.RawMessage
	var activeModel string
	if cid := rawResp.Trajectory.CascadeID; cid != "" {
		if recModel, recCfg := GetCascadeModel(cid); recModel != "" {
			activeModel = canonicalModelName(recModel)
			if len(recCfg) > 0 {
				activeConfig = recCfg
			}
		}
	}
	for i := len(rawResp.Trajectory.ExecutorMetadatas) - 1; i >= 0; i-- {
		cfg := rawResp.Trajectory.ExecutorMetadatas[i].CascadeConfig
		if len(cfg) > 0 && string(cfg) != "null" && string(cfg) != "{}" {
			if activeModel == "" {
				var cfgMap map[string]interface{}
				if err := json.Unmarshal(cfg, &cfgMap); err == nil {
					if p, ok := cfgMap["plannerConfig"].(map[string]interface{}); ok {
						if mn, ok := p["modelName"].(string); ok && mn != "" {
							activeModel = canonicalModelName(mn)
						} else if pm, ok := p["planModel"].(string); ok && pm != "" {
							activeModel = canonicalModelName(pm)
						}
					}
				}
			}
			if len(activeConfig) == 0 {
				activeConfig = cfg
				SetLastKnownCascadeConfig(cfg)
			}
			if activeModel != "" && len(activeConfig) > 0 {
				break
			}
		}
	}
	var activeConfigStr string
	if len(activeConfig) > 0 {
		activeConfigStr = string(activeConfig)
	}

	modelDisplayName := ""
	if activeModel != "" {
		lower := strings.ToLower(activeModel)
		if strings.Contains(lower, "claude") {
			modelDisplayName = "Claude"
		} else if strings.Contains(lower, "gemini") {
			modelDisplayName = "Gemini"
		} else if strings.Contains(lower, "gpt") {
			modelDisplayName = "GPT"
		} else {
			modelDisplayName = activeModel
		}
	}

	wsURI := ""
	if len(rawResp.Trajectory.WorkspaceUris) > 0 {
		wsURI = rawResp.Trajectory.WorkspaceUris[0]
	}

	title := ""
	if rawResp.Trajectory.Annotations != nil && rawResp.Trajectory.Annotations.Title != "" {
		title = rawResp.Trajectory.Annotations.Title
	} else if rawResp.Trajectory.Summary != "" {
		title = rawResp.Trajectory.Summary
	}

	if title == "" || title == "未命名会话" {
		if t := readAnnotationTitle(rawResp.Trajectory.CascadeID); t != "" && t != "未命名会话" {
			title = t
		}
	}

	if title == "" || title == "未命名会话" {
		for _, s := range steps {
			if s.Type == "CORTEX_STEP_TYPE_USER_INPUT" {
				if t := extractTitleFromUserInput(s); t != "" && t != "未命名会话" {
					title = t
					break
				}
			}
		}
	}

	if title != "" && title != "未命名会话" && rawResp.Trajectory.CascadeID != "" {
		defaultTrajCache.cascadeTitlesMu.Lock()
		defaultTrajCache.cascadeTitles[rawResp.Trajectory.CascadeID] = title
		defaultTrajCache.cascadeTitlesMu.Unlock()
	}

	// Detect if latest turn contains an artifact pending user feedback (Proceed)
	// Desktop Antigravity (Bjb) parity:
	// 1. Must be in the latest turn (after last CORTEX_STEP_TYPE_USER_INPUT).
	// 2. Status must not be RUNNING.
	// 3. No non-artifact code files have been modified after the plan artifact in this turn.
	// 4. An artifact in this turn has requestFeedback == true.
	// 5. The artifact has NOT already been approved by user in conversation history.
	canProceed := false
	proceedArtifactURI := ""
	hasModifiedNonArtifactFilesAfterPlan := false

	// Track all approved artifact URIs from user input steps throughout the trajectory
	approvedArtifacts := make(map[string]bool)
	for _, s := range steps {
		if s.Type == "CORTEX_STEP_TYPE_USER_INPUT" && s.UserInput != nil {
			for _, ac := range s.UserInput.ArtifactComments {
				if ac.ArtifactURI != "" && isArtifactApproval(ac.ApprovalStatus) {
					approvedArtifacts[ac.ArtifactURI] = true
					approvedArtifacts[normalizeURI(ac.ArtifactURI)] = true
				}
			}
			text := s.UserInput.UserResponse
			if text == "" && len(s.UserInput.Items) > 0 {
				text = s.UserInput.Items[0].Text
			}
			trimmed := strings.TrimSpace(text)
			if strings.Contains(trimmed, "The user has approved this document") && strings.HasPrefix(trimmed, "Comments on artifact URI:") {
				firstLine := strings.Split(trimmed, "\n")[0]
				uri := strings.TrimSpace(strings.TrimPrefix(firstLine, "Comments on artifact URI:"))
				if uri != "" {
					approvedArtifacts[uri] = true
					approvedArtifacts[normalizeURI(uri)] = true
				}
			}
		}
	}

	lastUserInputIdx := -1
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Type == "CORTEX_STEP_TYPE_USER_INPUT" {
			lastUserInputIdx = i
			break
		}
	}

	for i := lastUserInputIdx + 1; i < len(steps); i++ {
		s := steps[i]
		if s.Type == "CORTEX_STEP_TYPE_CODE_ACTION" {
			if s.CodeAction != nil {
				ca := s.CodeAction
				uri := ""
				if ca.ActionResult != nil {
					if ca.ActionResult.Edit != nil && ca.ActionResult.Edit.AbsoluteURI != "" {
						uri = ca.ActionResult.Edit.AbsoluteURI
					} else if ca.ActionResult.AbsoluteURI != "" {
						uri = ca.ActionResult.AbsoluteURI
					}
				}
				if uri == "" && ca.ActionSpec != nil && ca.ActionSpec.CreateFile != nil && ca.ActionSpec.CreateFile.Path != nil {
					uri = ca.ActionSpec.CreateFile.Path.AbsoluteURI
				}

				isScratch := strings.Contains(uri, "/scratch/")
				isWalkthrough := strings.Contains(uri, "walkthrough.md")
				isPlan := strings.Contains(uri, "implementation_plan.md")

				isArtifact := (ca.IsArtifactFile ||
					strings.Contains(uri, "/brain/") ||
					strings.Contains(uri, ".gemini/antigravity/brain") ||
					isPlan) && !isScratch

				if isArtifact {
					reqFeedback := false
					if !isWalkthrough {
						if ca.ArtifactMetadata != nil && ca.ArtifactMetadata.RequestFeedback {
							reqFeedback = true
						}

						filePath := uri
						if strings.HasPrefix(filePath, "file://") {
							filePath = strings.TrimPrefix(filePath, "file://")
						}

						isCreateFile := (ca.ActionSpec != nil && ca.ActionSpec.CreateFile != nil) ||
							(ca.ActionResult != nil && ca.ActionResult.Edit != nil && ca.ActionResult.Edit.CreateFile)
						isApproved := approvedArtifacts[uri] || approvedArtifacts[normalizeURI(uri)]

						// If step metadata was missing but this is an artifact step in the current turn, check its specific metadata file.
						// Guard: ONLY check disk .metadata.json if:
						// 1. The step is actually creating/overwriting a file (isCreateFile), not a partial replace_file_content edit.
						// 2. The artifact was NOT already approved earlier.
						if !reqFeedback && !isApproved && isCreateFile {
							if filePath != "" && readMetadataRequestFeedback(filePath+".metadata.json") {
								reqFeedback = true
							}

							// Fallback: ONLY check implementation_plan.md.metadata.json if this step actually targets implementation_plan.md
							if !reqFeedback && isPlan && rawResp.Trajectory.CascadeID != "" {
								if home, err := os.UserHomeDir(); err == nil && home != "" {
									planMetaPath := filepath.Join(home, ".gemini/antigravity/brain", rawResp.Trajectory.CascadeID, "implementation_plan.md.metadata.json")
									if readMetadataRequestFeedback(planMetaPath) {
										reqFeedback = true
										if uri == "" {
											planAbs := filepath.Join(home, ".gemini", "antigravity", "brain", rawResp.Trajectory.CascadeID, "implementation_plan.md")
											uri = normalizeURI("file:///" + filepath.ToSlash(planAbs))
										}
									}
								}
							}
						}

						// If the artifact was already approved and this step did NOT explicitly request feedback via ArtifactMetadata,
						// do not trigger Proceed.
						if isApproved && (ca.ArtifactMetadata == nil || !ca.ArtifactMetadata.RequestFeedback) {
							reqFeedback = false
						}
					}

					if reqFeedback && uri != "" && !isWalkthrough {
						canProceed = true
						proceedArtifactURI = uri
						hasModifiedNonArtifactFilesAfterPlan = false
					}
				} else {
					if canProceed {
						hasModifiedNonArtifactFilesAfterPlan = true
					}
				}
			}
		}
	}

	// Desktop parity: if non-artifact files were modified after the plan was created,
	// or if agent is actively running, do not show proceed
	if hasModifiedNonArtifactFilesAfterPlan || rawResp.Status == "CASCADE_RUN_STATUS_RUNNING" {
		canProceed = false
		proceedArtifactURI = ""
	}

	var pendingInteraction *PendingInteraction
	for idx := len(steps) - 1; idx >= 0; idx-- {
		s := steps[idx]
		if s.Status == "CORTEX_STEP_STATUS_WAITING" && s.RequestedInteraction != nil {
			pi := &PendingInteraction{
				TrajectoryID: rawResp.Trajectory.TrajectoryID,
				StepIndex:    idx,
			}
			req := s.RequestedInteraction
			if req.Permission != nil {
				pi.Type = "permission"
				pi.Action = req.Permission.Resource.Action
				pi.Target = req.Permission.Resource.Target
				pi.Description = req.Permission.ActionDescription

				actionLower := strings.ToLower(pi.Action)
				if strings.Contains(actionLower, "read") {
					pi.Title = "Allow read access to this path?"
				} else if strings.Contains(actionLower, "command") {
					pi.Title = "Allow executing this command?"
				} else if strings.Contains(actionLower, "write") || strings.Contains(actionLower, "edit") {
					pi.Title = "Allow write access to this path?"
				} else if pi.Description != "" {
					pi.Title = "Allow: " + pi.Description + "?"
				} else {
					pi.Title = fmt.Sprintf("Allow %s access?", pi.Action)
				}

				pi.Options = []InteractionOption{
					{ID: "1", Text: "Yes, allow this time", Scope: 1},
					{ID: "2", Text: "Yes, and always allow in this conversation", Scope: 2},
					{ID: "3", Text: "Yes, and always allow in this project", Scope: 3},
					{ID: "4", Text: "Yes, and always allow", Scope: 4},
					{ID: "5", Text: "No", IsDeny: true},
				}
				pi.DefaultOptionID = "1"
				pi.HasWriteIn = true
				pi.WriteInLabel = "No"
				pi.WriteInPlaceholder = "(tell the agent what to do instead)"
			} else if req.AskQuestion != nil && len(req.AskQuestion.Questions) > 0 {
				pi.Type = "ask_question"
				var questions []InteractionQuestion
				for _, qItem := range req.AskQuestion.Questions {
					iq := InteractionQuestion{
						Question:           qItem.Question,
						IsMultiSelect:      qItem.IsMultiSelect,
						HasWriteIn:         true,
						WriteInLabel:       "Other",
						WriteInPlaceholder: "(write in your response)",
					}
					for _, opt := range qItem.Options {
						iq.Options = append(iq.Options, InteractionOption{
							ID:   opt.ID,
							Text: opt.Text,
						})
					}
					if len(iq.Options) > 0 {
						iq.DefaultOptionID = iq.Options[0].ID
					}
					questions = append(questions, iq)
				}
				pi.Questions = questions

				// For backward compatibility with clients that only inspect top-level single question:
				q0 := questions[0]
				pi.Title = q0.Question
				pi.IsMultiSelect = q0.IsMultiSelect
				pi.Options = q0.Options
				pi.DefaultOptionID = q0.DefaultOptionID
				pi.HasWriteIn = true
				pi.WriteInLabel = "Other"
				pi.WriteInPlaceholder = "(write in your response)"
			} else if req.FilePermission != nil {
				pi.Type = "file_permission"
				pi.Target = req.FilePermission.AbsolutePathURI
				pi.Title = "Allow access to this file outside workspace?"
				pi.Options = []InteractionOption{
					{ID: "1", Text: "Yes, allow this time", Scope: 1},
					{ID: "2", Text: "Yes, and always allow in this conversation", Scope: 2},
					{ID: "3", Text: "Yes, and always allow in this project", Scope: 3},
					{ID: "4", Text: "Yes, and always allow", Scope: 4},
					{ID: "5", Text: "No", IsDeny: true},
				}
				pi.DefaultOptionID = "1"
				pi.HasWriteIn = true
				pi.WriteInLabel = "No"
				pi.WriteInPlaceholder = "(tell the agent what to do instead)"
			} else if req.RunCommand != nil {
				pi.Type = "run_command"
				pi.Target = req.RunCommand.CommandLine
				pi.Title = "Confirm command execution"
				pi.Options = []InteractionOption{
					{ID: "1", Text: "Yes, run command", Scope: 1},
					{ID: "2", Text: "No", IsDeny: true},
				}
				pi.DefaultOptionID = "1"
				pi.HasWriteIn = true
				pi.WriteInLabel = "No"
				pi.WriteInPlaceholder = "(tell the agent what to do instead)"
			}
			pendingInteraction = pi
			break
		}
	}
	if rawResp.Status != "CASCADE_RUN_STATUS_RUNNING" {
		pendingInteraction = nil
	}

	queuedMessages := []QueuedMessageItem{}
	for _, pam := range rawResp.PendingAgentMessages {
		if isInternalAgentMessage(pam.HideFromUser, pam.Sender, pam.SourceMetadata, pam.Content) {
			continue
		}
		if pam.DeliveryStrategy != nil && !isQueuedDeliveryStrategy(pam.DeliveryStrategy) {
			continue
		}
		uMsg := upstreamAgentMessage{
			ID:               pam.ID,
			Sender:           pam.Sender,
			Timestamp:        pam.Timestamp,
			HideFromUser:     pam.HideFromUser,
			Content:          pam.Content,
			StepPayload:      pam.StepPayload,
			DeliveryStrategy: pam.DeliveryStrategy,
			SourceMetadata:   pam.SourceMetadata,
		}
		text := extractQueuedMessageText(uMsg)
		media, imageUrls := extractQueuedMessageMedia(uMsg)
		if (text != "" || len(media) > 0 || len(imageUrls) > 0) && !isInternalAgentMessage(false, "", nil, text) {
			queuedMessages = append(queuedMessages, QueuedMessageItem{
				ID:        pam.ID,
				Text:      text,
				CreatedAt: parseAgentMessageTimestamp(pam.Timestamp),
				Media:     media,
				ImageURLs: imageUrls,
			})
		}
	}
	queuedMessages = p.FilterQueuedMessagesAgainstTrajectory(rawResp.Trajectory.CascadeID, queuedMessages, steps, allMessages)

	var runningTasks []RunningTaskItem
	for idx, s := range steps {
		stepIdx := idx
		if s.Metadata.SourceTrajectoryStepInfo != nil && s.Metadata.SourceTrajectoryStepInfo.StepIndex > 0 {
			stepIdx = s.Metadata.SourceTrajectoryStepInfo.StepIndex
		}

		if s.Status == "CORTEX_STEP_STATUS_RUNNING" {
			cmdLine := ""
			toolName := "run_command"
			taskID := fmt.Sprintf("task-%d", stepIdx)
			logURI := ""

			if s.TaskDetails != nil {
				if s.TaskDetails.ID != "" {
					taskID = s.TaskDetails.ID
				}
				if s.TaskDetails.Description != "" {
					cmdLine = s.TaskDetails.Description
				}
				if s.TaskDetails.LogURI != "" {
					logURI = s.TaskDetails.LogURI
				}
			}
			if cmdLine == "" && s.RunCommand != nil {
				if s.RunCommand.CommandLine != "" {
					cmdLine = s.RunCommand.CommandLine
				} else if s.RunCommand.ProposedCommandLine != "" {
					cmdLine = s.RunCommand.ProposedCommandLine
				}
			}
			if s.Metadata.ToolCall != nil && s.Metadata.ToolCall.Name != "" {
				toolName = s.Metadata.ToolCall.Name
			}

			if cmdLine == "" && s.Metadata.ToolCall != nil {
				cmdLine = s.Metadata.ToolCall.ArgumentsJson
			}
			if cmdLine == "" {
				cmdLine = s.Metadata.ToolAction
			}

			runningTasks = append(runningTasks, RunningTaskItem{
				ID:          taskID,
				StepIndex:   stepIdx,
				ToolName:    toolName,
				CommandLine: cmdLine,
				ToolSummary: s.Metadata.ToolSummary,
				ToolAction:  s.Metadata.ToolAction,
				LogURI:      logURI,
				StartedAt:   s.Metadata.CreatedAt,
			})
		}
	}

	// Determine error state scoped strictly to the latest turn (after lastUserInputIdx).
	// Historical errors in previous turns that were subsequently recovered must not mark the session as an error.
	latestTurnHasError := false
	var latestTurnErrorText string
	for i := len(steps) - 1; i > lastUserInputIdx; i-- {
		s := steps[i]
		if s.Type == "CORTEX_STEP_TYPE_ERROR_MESSAGE" {
			if isUserVisibleError(s) {
				latestTurnHasError = true
				latestTurnErrorText = extractErrorText(s)
			}
			break
		} else if s.Type == "CORTEX_STEP_TYPE_PLANNER_RESPONSE" || s.RunCommand != nil || s.CodeAction != nil || s.TaskDetails != nil {
			// A subsequent step was executed after any earlier error; not terminated by error
			break
		}
	}

	finalStatus := rawResp.Status
	if rawResp.Status == "CASCADE_RUN_STATUS_RUNNING" {
		if len(steps) > 0 && steps[len(steps)-1].Type == "CORTEX_STEP_TYPE_ERROR_MESSAGE" && latestTurnHasError {
			finalStatus = "CASCADE_RUN_STATUS_ERROR"
		}
	} else if latestTurnHasError {
		finalStatus = "CASCADE_RUN_STATUS_ERROR"
	}

	hasError := finalStatus == "CASCADE_RUN_STATUS_ERROR"

	return TrajectoryDetails{
		CascadeID:          rawResp.Trajectory.CascadeID,
		Title:              title,
		Status:             finalStatus,
		HasError:           hasError,
		ErrorMessage:       latestTurnErrorText,
		Duration:           duration,
		TotalSteps:         totalSteps,
		TotalTools:         totalToolsCount,
		WorkspaceURI:       wsURI,
		Steps:              steps,
		AllMessages:        allMessages,
		QueuedMessages:     queuedMessages,
		RunningTasks:       runningTasks,
		ActiveModel:        activeModel,
		ModelDisplayName:   modelDisplayName,
		CascadeConfig:      activeConfig,
		CascadeConfigRaw:   activeConfigStr,
		CanProceed:         canProceed,
		ProceedArtifactURI: proceedArtifactURI,
		PendingInteraction: pendingInteraction,
	}
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
