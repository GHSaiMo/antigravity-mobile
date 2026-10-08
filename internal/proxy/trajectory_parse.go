package proxy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	pendingTools := 0
	var toolNames []string
	totalToolsCount := 0

	var turnArtifacts []ArtifactItem
	seenArtifactURIs := make(map[string]bool)

	addTurnArtifact := func(art ArtifactItem) {
		norm := strings.TrimSpace(art.URI)
		if norm == "" {
			return
		}
		if !strings.HasPrefix(norm, "file://") && filepath.IsAbs(norm) {
			norm = "file://" + norm
		}
		if seenArtifactURIs[norm] {
			return
		}
		seenArtifactURIs[norm] = true
		art.URI = norm
		turnArtifacts = append(turnArtifacts, art)
	}

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

		// Extract any artifacts created or edited in this step
		for _, art := range extractArtifactsFromStep(s) {
			addTurnArtifact(art)
		}

		if stepType == "CORTEX_STEP_TYPE_USER_INPUT" {
			flushTools()
			turnArtifacts = nil
			seenArtifactURIs = make(map[string]bool)
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

				// Extract any referenced artifacts in text
				for _, art := range extractArtifactsFromText(respText) {
					addTurnArtifact(art)
				}

				var msgArtifacts []ArtifactItem
				if len(turnArtifacts) > 0 {
					msgArtifacts = append([]ArtifactItem(nil), turnArtifacts...)
					turnArtifacts = nil
					seenArtifactURIs = make(map[string]bool)
				}

				stepIdx := idx
				allMessages = append(allMessages, CascadeMessageItem{
					ID:        fmt.Sprintf("step-%d", idx),
					Type:      "agent",
					Role:      "assistant",
					Text:      respText,
					Content:   respText,
					StepIndex: &stepIdx,
					ImageURLs: imgURLs,
					Artifacts: msgArtifacts,
				})
			}
		} else if stepType == "CORTEX_STEP_TYPE_ERROR_MESSAGE" {
			if !isUserVisibleError(s) {
				continue
			}
			flushTools()
			rawErrText := extractErrorText(s)

			if len(allMessages) > 0 && allMessages[len(allMessages)-1].Type == "error" {
				if merged, ok := tryMergeAttemptErrors(allMessages[len(allMessages)-1], rawErrText); ok {
					allMessages[len(allMessages)-1] = merged
					continue
				}
			}

			parsed := parseAttemptError(rawErrText)
			formatted := rawErrText
			if parsed.isAttempt {
				formatted = formatAttemptError(parsed.prefix, parsed.attempt, parsed.maxAttempts, parsed.baseError)
			}

			allMessages = append(allMessages, CascadeMessageItem{
				ID:           fmt.Sprintf("step-%d", idx),
				Type:         "error",
				Role:         "error",
				Text:         formatted,
				Content:      formatted,
				AttemptCount: parsed.attempt,
				MaxAttempts:  parsed.maxAttempts,
			})
		} else if strings.HasPrefix(stepType, "CORTEX_STEP_TYPE_") && stepType != "CORTEX_STEP_TYPE_SYSTEM_MESSAGE" && stepType != "CORTEX_STEP_TYPE_ERROR_MESSAGE" {
			pendingTools++
			totalToolsCount++
			name := extractToolNameFromStep(s)
			toolNames = append(toolNames, name)
		}
	}

	flushTools()

	// If there are still unconsumed artifacts in this turn, attach them to the last agent message
	if len(turnArtifacts) > 0 && len(allMessages) > 0 {
		for i := len(allMessages) - 1; i >= 0; i-- {
			if allMessages[i].Type == "agent" {
				existingURIs := make(map[string]bool)
				for _, ea := range allMessages[i].Artifacts {
					existingURIs[ea.URI] = true
				}
				for _, art := range turnArtifacts {
					if !existingURIs[art.URI] {
						allMessages[i].Artifacts = append(allMessages[i].Artifacts, art)
						existingURIs[art.URI] = true
					}
				}
				break
			}
		}
	}

	return allMessages, totalToolsCount
}

// formatArtifactTitle formats a markdown file URI into a human-friendly Title Case title.
func formatArtifactTitle(uri string) string {
	clean := uri
	if strings.HasPrefix(clean, "file://") {
		clean = strings.TrimPrefix(clean, "file://")
	}
	base := filepath.Base(clean)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)

	// Normalize underscores and hyphens to spaces
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")

	lower := strings.ToLower(name)
	if lower == "implementation plan" {
		return "Implementation Plan"
	} else if lower == "walkthrough" {
		return "Walkthrough"
	}

	// Title-case words
	words := strings.Fields(name)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	res := strings.Join(words, " ")
	if res == "" {
		return "文档详情"
	}
	return res
}

// isBrainArtifactURI reports whether a file URI points to an artifact within the brain directory.
func isBrainArtifactURI(uri string) bool {
	lower := strings.ToLower(strings.TrimSpace(uri))
	if lower == "" || strings.Contains(lower, "/scratch/") {
		return false
	}
	return strings.Contains(lower, "/brain/") ||
		strings.Contains(lower, ".gemini/antigravity/brain") ||
		strings.Contains(lower, "/static/artifacts/")
}

// extractArtifactsFromStep parses any artifact files created/edited in a trajectory step.
func extractArtifactsFromStep(s TrajectoryStep) []ArtifactItem {
	var results []ArtifactItem

	addArtifact := func(uri, summary string, reqFeedback, userFacing bool, isExplicitArtifact bool) {
		uri = strings.TrimSpace(uri)
		if uri == "" {
			return
		}
		if !strings.HasPrefix(uri, "file://") && filepath.IsAbs(uri) {
			uri = "file://" + uri
		}
		lower := strings.ToLower(uri)
		if strings.Contains(lower, "/scratch/") {
			return
		}

		isMd := strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
		if !isMd {
			return
		}

		isBrain := isBrainArtifactURI(uri)
		// 严禁将非 brain 目录且非 upstream 明确标记的常规工作区工程代码/文档文件当作 Artifact！
		if !isBrain && !isExplicitArtifact {
			return
		}

		filePath := strings.TrimPrefix(uri, "file://")
		hasDiskMeta := false
		if filePath != "" {
			metaPath := filePath + ".metadata.json"
			if metaBytes, err := os.ReadFile(metaPath); err == nil {
				var meta ArtifactMetadata
				if err := json.Unmarshal(metaBytes, &meta); err == nil {
					hasDiskMeta = true
					if summary == "" {
						summary = strings.TrimSpace(meta.Summary)
					}
					userFacing = meta.IsUserFacing()
					if meta.RequestFeedback {
						reqFeedback = true
					}
				}
			}
		}

		// 标准内置产物（如 implementation_plan.md 或 walkthrough.md）若位于 brain 目录亦视作产物
		isStandardPlan := strings.HasSuffix(lower, "implementation_plan.md") || strings.HasSuffix(lower, "walkthrough.md")
		if !isExplicitArtifact && !hasDiskMeta && !isStandardPlan {
			return
		}

		// 桌面端核心规范：仅用户可见（UserFacing == true）的产物在界面中渲染气泡
		if !userFacing {
			return
		}

		title := formatArtifactTitle(uri)
		results = append(results, ArtifactItem{
			URI:             uri,
			Title:           title,
			Summary:         summary,
			RequestFeedback: reqFeedback,
			UserFacing:      userFacing,
		})
	}

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
		summary := ""
		userFacing := true
		reqFeedback := false
		hasExplicitMeta := false
		if ca.ArtifactMetadata != nil {
			hasExplicitMeta = true
			summary = strings.TrimSpace(ca.ArtifactMetadata.Summary)
			if ca.ArtifactMetadata.UserFacing != nil {
				userFacing = *ca.ArtifactMetadata.UserFacing
			}
			reqFeedback = ca.ArtifactMetadata.RequestFeedback
		}
		isExplicit := ca.IsArtifactFile || hasExplicitMeta
		if isExplicit || isBrainArtifactURI(uri) {
			addArtifact(uri, summary, reqFeedback, userFacing, isExplicit)
		}
	}

	tcName := ""
	tcArgs := ""
	if s.ToolCall != nil {
		tcName = s.ToolCall.Name
		tcArgs = s.ToolCall.ArgumentsJson
	} else if s.Metadata.ToolCall != nil {
		tcName = s.Metadata.ToolCall.Name
		tcArgs = s.Metadata.ToolCall.ArgumentsJson
	}

	if (tcName == "write_to_file" || tcName == "replace_file_content") && tcArgs != "" {
		var args struct {
			TargetFile       string `json:"TargetFile"`
			ArtifactMetadata *struct {
				Summary         string `json:"Summary"`
				RequestFeedback bool   `json:"RequestFeedback"`
				UserFacing      *bool  `json:"UserFacing"`
			} `json:"ArtifactMetadata"`
		}
		if err := json.Unmarshal([]byte(tcArgs), &args); err == nil && args.TargetFile != "" {
			summary := ""
			userFacing := true
			reqFeedback := false
			hasExplicitMeta := false
			if args.ArtifactMetadata != nil {
				hasExplicitMeta = true
				summary = strings.TrimSpace(args.ArtifactMetadata.Summary)
				if args.ArtifactMetadata.UserFacing != nil {
					userFacing = *args.ArtifactMetadata.UserFacing
				}
				reqFeedback = args.ArtifactMetadata.RequestFeedback
			}
			if hasExplicitMeta || isBrainArtifactURI(args.TargetFile) {
				addArtifact(args.TargetFile, summary, reqFeedback, userFacing, hasExplicitMeta)
			}
		}
	}

	return results
}

var markdownFileLinkRegex = regexp.MustCompile(`(?i)(?:\[([^\]]*)\]\()?((?:file://)?(/[^\s)\]]+\.(?:md|markdown)))\)?`)

// extractArtifactsFromText scans response text for markdown links pointing to markdown artifact files.
// Desktop client parity: only true brain artifacts with valid on-disk user-facing metadata are extracted.
// Regular workspace code/doc links (e.g. docs/foo.md) are strictly excluded from artifact cards.
func extractArtifactsFromText(text string) []ArtifactItem {
	var results []ArtifactItem
	matches := markdownFileLinkRegex.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		rawPath := m[2]
		uri := rawPath
		if !strings.HasPrefix(uri, "file://") && filepath.IsAbs(rawPath) {
			uri = "file://" + uri
		}
		lower := strings.ToLower(uri)
		if strings.Contains(lower, "/scratch/") {
			continue
		}

		// 必须是 brain 目录下的产物路径，严禁将项目工作区普通文档当做 Artifact
		if !isBrainArtifactURI(uri) {
			continue
		}

		filePath := strings.TrimPrefix(uri, "file://")
		metaPath := filePath + ".metadata.json"
		metaBytes, err := os.ReadFile(metaPath)
		if err != nil {
			// 只有标准内置 plan/walkthrough 允许在无独立 metadata 文件时被识别
			isStandardPlan := strings.HasSuffix(lower, "implementation_plan.md") || strings.HasSuffix(lower, "walkthrough.md")
			if !isStandardPlan {
				continue
			}
			title := formatArtifactTitle(uri)
			results = append(results, ArtifactItem{
				URI:        uri,
				Title:      title,
				UserFacing: true,
			})
			continue
		}

		var meta ArtifactMetadata
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			continue
		}
		if !meta.IsUserFacing() {
			continue
		}

		title := formatArtifactTitle(uri)
		results = append(results, ArtifactItem{
			URI:             uri,
			Title:           title,
			Summary:         strings.TrimSpace(meta.Summary),
			RequestFeedback: meta.RequestFeedback,
			UserFacing:      meta.IsUserFacing(),
		})
	}
	return results
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
				raw := extractErrorText(s)
				parsed := parseAttemptError(raw)
				if parsed.isAttempt {
					latestTurnErrorText = formatAttemptError(parsed.prefix, parsed.attempt, parsed.maxAttempts, parsed.baseError)
				} else {
					latestTurnErrorText = raw
				}
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
