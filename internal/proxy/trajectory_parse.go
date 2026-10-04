package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ParseTrajectoryDetails extracts messages, tools count, duration and metadata from raw response.
//
// It is an orchestrator over small stage functions; stage order matters because some stages
// have side effects (config/title caches) and some read on-disk metadata.
func (p *Proxy) ParseTrajectoryDetails(rawResp *upstreamTrajectoryResp) TrajectoryDetails {
	steps := rawResp.Trajectory.Steps
	totalSteps := len(steps)

	allMessages, totalToolsCount := buildTrajectoryMessages(steps)
	duration := trajectoryDuration(steps)

	activeModel, activeConfig := resolveActiveModel(rawResp)
	var activeConfigStr string
	if len(activeConfig) > 0 {
		activeConfigStr = string(activeConfig)
	}
	modelDisplayName := modelDisplayNameFor(activeModel)

	wsURI := ""
	if len(rawResp.Trajectory.WorkspaceUris) > 0 {
		wsURI = rawResp.Trajectory.WorkspaceUris[0]
	}

	title := resolveTrajectoryTitle(rawResp, steps)

	lastUserInputIdx := lastUserInputIndex(steps)
	canProceed, proceedArtifactURI := detectProceed(rawResp, steps, lastUserInputIdx)
	pendingInteraction := buildPendingInteraction(rawResp, steps)
	queuedMessages := p.buildQueuedMessages(rawResp, steps, allMessages)
	runningTasks := buildRunningTasks(steps)
	finalStatus, latestTurnErrorText := resolveFinalStatus(rawResp.Status, steps, lastUserInputIdx)
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

// buildTrajectoryMessages folds raw steps into chat messages: user/agent/error messages, with
// consecutive tool steps collapsed into a single "tools" capsule. It also returns the tool total.
func buildTrajectoryMessages(steps []TrajectoryStep) ([]CascadeMessageItem, int) {
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

	return allMessages, totalToolsCount
}

// trajectoryDuration formats the span between the first and last parseable step timestamps.
func trajectoryDuration(steps []TrajectoryStep) string {
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
	return duration
}

// resolveActiveModel determines the active model name and cascade config. An explicitly
// recorded model (SetCascadeModel) wins; otherwise the newest valid executor config is used.
// Side effect: remembers the newest valid config via SetLastKnownCascadeConfig.
func resolveActiveModel(rawResp *upstreamTrajectoryResp) (string, json.RawMessage) {
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
	return activeModel, activeConfig
}

// modelDisplayNameFor maps a model name to its short vendor label (Claude/Gemini/GPT) or the name itself.
func modelDisplayNameFor(activeModel string) string {
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
	return modelDisplayName
}

// resolveTrajectoryTitle picks the title from annotations, summary, the on-disk annotation file,
// or the first user prompt, and remembers it in the cascade title cache.
func resolveTrajectoryTitle(rawResp *upstreamTrajectoryResp, steps []TrajectoryStep) string {
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
	return title
}

// collectApprovedArtifacts gathers artifact URIs the user already approved anywhere in the
// conversation (via structured artifact comments or the system approval text).
func collectApprovedArtifacts(steps []TrajectoryStep) map[string]bool {
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
	return approvedArtifacts
}

// lastUserInputIndex returns the index of the latest USER_INPUT step, or -1 if there is none.
func lastUserInputIndex(steps []TrajectoryStep) int {
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Type == "CORTEX_STEP_TYPE_USER_INPUT" {
			return i
		}
	}
	return -1
}

// detectProceed reports whether the latest turn holds an artifact awaiting user feedback.
// Desktop Antigravity (Bjb) parity:
//  1. Must be in the latest turn (after the last USER_INPUT step).
//  2. Status must not be RUNNING.
//  3. No non-artifact code files modified after the plan artifact in this turn.
//  4. An artifact in this turn has requestFeedback == true (step metadata or on-disk metadata).
//  5. The artifact has NOT already been approved in conversation history.
func detectProceed(rawResp *upstreamTrajectoryResp, steps []TrajectoryStep, lastUserInputIdx int) (bool, string) {
	canProceed := false
	proceedArtifactURI := ""
	hasModifiedNonArtifactFilesAfterPlan := false
	approvedArtifacts := collectApprovedArtifacts(steps)

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
									}
								}
							}
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

	return canProceed, proceedArtifactURI
}

// buildPendingInteraction returns the newest step waiting on user input (permission, question,
// file access or command approval). Interactions only matter while the cascade is RUNNING.
func buildPendingInteraction(rawResp *upstreamTrajectoryResp, steps []TrajectoryStep) *PendingInteraction {
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

				applyPermissionOptions(pi)
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
				applyPermissionOptions(pi)
			} else if req.RunCommand != nil {
				pi.Type = "run_command"
				pi.Target = req.RunCommand.CommandLine
				pi.Title = "Confirm command execution"
				pi.Options = []InteractionOption{
					{ID: "1", Text: "Yes, run command", Scope: 1},
					{ID: "2", Text: "No", IsDeny: true},
				}
				applyDenyWriteIn(pi)
			}
			pendingInteraction = pi
			break
		}
	}
	if rawResp.Status != "CASCADE_RUN_STATUS_RUNNING" {
		pendingInteraction = nil
	}
	return pendingInteraction
}

// buildQueuedMessages lists user messages queued while the agent is busy, dropping internal
// agent traffic and entries already reflected in the trajectory.
func (p *Proxy) buildQueuedMessages(rawResp *upstreamTrajectoryResp, steps []TrajectoryStep, allMessages []CascadeMessageItem) []QueuedMessageItem {
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
	return queuedMessages
}

// buildRunningTasks lists steps still RUNNING (background commands) with their best-effort command line.
func buildRunningTasks(steps []TrajectoryStep) []RunningTaskItem {
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
	return runningTasks
}

// resolveFinalStatus derives the effective run status and error text, scoped strictly to the
// latest turn: errors from earlier turns that were recovered must not mark the session as errored.
func resolveFinalStatus(status string, steps []TrajectoryStep, lastUserInputIdx int) (string, string) {
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

	finalStatus := status
	if status == "CASCADE_RUN_STATUS_RUNNING" {
		if len(steps) > 0 && steps[len(steps)-1].Type == "CORTEX_STEP_TYPE_ERROR_MESSAGE" && latestTurnHasError {
			finalStatus = "CASCADE_RUN_STATUS_ERROR"
		}
	} else if latestTurnHasError {
		finalStatus = "CASCADE_RUN_STATUS_ERROR"
	}

	return finalStatus, latestTurnErrorText
}

// applyDenyWriteIn marks the first option as default and turns "No" into a write-in that lets
// the user tell the agent what to do instead.
func applyDenyWriteIn(pi *PendingInteraction) {
	pi.DefaultOptionID = "1"
	pi.HasWriteIn = true
	pi.WriteInLabel = "No"
	pi.WriteInPlaceholder = "(tell the agent what to do instead)"
}

// applyPermissionOptions installs the standard allow-once / allow-always / deny choices.
func applyPermissionOptions(pi *PendingInteraction) {
	pi.Options = []InteractionOption{
		{ID: "1", Text: "Yes, allow this time", Scope: 1},
		{ID: "2", Text: "Yes, and always allow in this conversation", Scope: 2},
		{ID: "3", Text: "Yes, and always allow in this project", Scope: 3},
		{ID: "4", Text: "Yes, and always allow", Scope: 4},
		{ID: "5", Text: "No", IsDeny: true},
	}
	applyDenyWriteIn(pi)
}
