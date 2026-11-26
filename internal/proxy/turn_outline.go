package proxy

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	voiceContextBlockRe      = regexp.MustCompile(`(?s)<talker_agent_context>(.*?)</talker_agent_context>`)
	voiceInstructionsBlockRe = regexp.MustCompile(`(?s)<voice_instructions>.*?</voice_instructions>`)
	voiceXMLTagRe            = regexp.MustCompile(`</?[a-zA-Z_][a-zA-Z0-9_-]*(?:\s+[^>]*)?>`)
	voiceSpeakerPrefixRe     = regexp.MustCompile(`(?i)^(我|user|human|me|talker|assistant|ai)\s*[:：]\s*`)
	voiceTimestampLineRe     = regexp.MustCompile(`^\[\d{4}-\d{2}-\d{2}[^\]]*\]\s*`)
)

// buildTurnOutline builds a compact, full-session turn index from all parsed messages.
// Clients use this lightweight outline to show true 1..N turn counts and jump directly to
// any earlier turn without downloading the full message history on session open.
func buildTurnOutline(allMessages []CascadeMessageItem) []TurnOutlineItem {
	if len(allMessages) == 0 {
		return nil
	}

	turns := make([]TurnOutlineItem, 0, max(1, len(allMessages)/3))
	turnStartOffset := 0
	var currentMsgs []CascadeMessageItem
	var currentUser *CascadeMessageItem
	turnIndex := 1

	finalize := func() {
		if len(currentMsgs) == 0 {
			return
		}
		firstID := ""
		if currentUser != nil && currentUser.ID != "" {
			firstID = currentUser.ID
		} else if currentMsgs[0].ID != "" {
			firstID = currentMsgs[0].ID
		} else {
			firstID = fmt.Sprintf("turn-%d", turnIndex)
		}

		summary, isVoice := extractTurnPromptSummary(currentUser)

		totalTools := 0
		hasError := false
		var artifacts []ArtifactItem
		seenArtifacts := make(map[string]bool)
		var startStep *int
		var endStep *int
		modelName := ""
		timestamp := ""

		if currentUser != nil {
			if currentUser.StepIndex != nil {
				v := *currentUser.StepIndex
				startStep = &v
			}
			timestamp = currentUser.Timestamp
		}

		for i := range currentMsgs {
			m := &currentMsgs[i]
			totalTools += m.ToolCount
			if m.Type == "error" || m.Role == "error" {
				hasError = true
			}
			for _, art := range m.Artifacts {
				if art.URI == "" || seenArtifacts[art.URI] {
					continue
				}
				seenArtifacts[art.URI] = true
				artifacts = append(artifacts, ArtifactItem{
					URI:             art.URI,
					Title:           art.Title,
					RequestFeedback: art.RequestFeedback,
					UserFacing:      art.UserFacing,
				})
			}
			if m.StepIndex != nil {
				v := *m.StepIndex
				if startStep == nil || v < *startStep {
					cp := v
					startStep = &cp
				}
				if endStep == nil || v > *endStep {
					cp := v
					endStep = &cp
				}
			}
			if timestamp == "" && m.Timestamp != "" {
				timestamp = m.Timestamp
			}
		}

		for i := len(currentMsgs) - 1; i >= 0; i-- {
			m := &currentMsgs[i]
			if m.ModelName != "" {
				modelName = m.ModelName
				break
			}
			if m.Model != "" {
				modelName = liveModels.displayName(m.Model)
				if modelName == "" {
					modelName = m.Model
				}
				break
			}
		}

		turns = append(turns, TurnOutlineItem{
			TurnIndex:       turnIndex,
			FirstMessageID:  firstID,
			MessageOffset:   turnStartOffset,
			MessageCount:    len(currentMsgs),
			StartStepIndex:  startStep,
			EndStepIndex:    endStep,
			PromptSummary:   summary,
			TotalTools:      totalTools,
			Artifacts:       artifacts,
			HasError:        hasError,
			ModelName:       modelName,
			Timestamp:       timestamp,
			IsVoiceTurn:     isVoice,
			IsScheduledTask: currentUser != nil && currentUser.IsScheduledTask,
		})

		turnIndex++
		currentMsgs = nil
		currentUser = nil
	}

	for idx := range allMessages {
		msg := allMessages[idx]
		isUser := msg.Type == "user" || msg.Role == "user"
		if isUser {
			if len(currentMsgs) > 0 {
				finalize()
			}
			turnStartOffset = idx
			msgCopy := msg
			currentUser = &msgCopy
			currentMsgs = append(currentMsgs, msg)
		} else {
			if len(currentMsgs) == 0 {
				turnStartOffset = idx
			}
			currentMsgs = append(currentMsgs, msg)
		}
	}
	if len(currentMsgs) > 0 {
		finalize()
	}

	return turns
}

func extractTurnPromptSummary(userMsg *CascadeMessageItem) (string, bool) {
	if userMsg == nil {
		return "会话开场与初始化", false
	}
	rawText := userMsg.Text
	if rawText == "" {
		rawText = userMsg.Content
	}
	body, fileNames := splitAttachmentBlock(rawText)
	isVoice := isVoiceTurnMessage(body, fileNames)

	if isVoice {
		cleaned := cleanVoiceTranscriptSummary(body)
		if summary := firstTwoLinesTruncated(cleaned, 48); summary != "" {
			return summary, true
		}
		return "", true
	}

	if summary := firstTwoLinesTruncated(body, 48); summary != "" {
		return summary, false
	}
	if len(fileNames) > 0 {
		firstFile := fileNames[0]
		if len(fileNames) == 1 {
			return fmt.Sprintf("[附件: %s]", firstFile), false
		}
		return fmt.Sprintf("[附件: %s 等 %d 个文件]", firstFile, len(fileNames)), false
	}
	imgCount := max(len(userMsg.Media), len(userMsg.ImageURLs))
	if imgCount > 0 {
		return fmt.Sprintf("[包含 %d 张图片]", imgCount), false
	}
	return "用户消息", false
}

func splitAttachmentBlock(text string) (string, []string) {
	idx := strings.LastIndex(text, attachmentHeader)
	if idx < 0 {
		return strings.TrimSpace(text), nil
	}
	body := strings.TrimSpace(text[:idx])
	block := strings.TrimSpace(text[idx+len(attachmentHeader):])
	var files []string
	for _, rawLine := range strings.Split(block, "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		entry := strings.TrimSpace(strings.TrimPrefix(line, "- "))
		if p := strings.LastIndex(entry, " ("); p > 0 {
			entry = strings.TrimSpace(entry[:p])
		}
		if entry != "" {
			files = append(files, filepath.Base(entry))
		}
	}
	return body, files
}

func isVoiceTurnMessage(body string, fileNames []string) bool {
	trimmed := strings.TrimSpace(body)
	if strings.Contains(trimmed, "<talker_agent_context>") || strings.Contains(trimmed, "</talker_agent_context>") {
		return true
	}
	if strings.Contains(trimmed, "<voice_instructions>") || strings.Contains(trimmed, "</voice_instructions>") {
		return true
	}
	if strings.Contains(trimmed, "talker:") && (strings.Contains(trimmed, "user:") || strings.Contains(trimmed, "[202")) {
		return true
	}
	if len(fileNames) > 0 && trimmed == "" {
		allAudio := true
		for _, fn := range fileNames {
			ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(fn)), ".")
			switch ext {
			case "wav", "m4a", "aac", "mp3", "caf", "opus":
			default:
				allAudio = false
			}
		}
		if allAudio {
			return true
		}
	}
	return false
}

func cleanVoiceTranscriptSummary(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	text := trimmed
	if m := voiceContextBlockRe.FindStringSubmatch(trimmed); len(m) > 1 && strings.TrimSpace(m[1]) != "" {
		text = strings.TrimSpace(m[1])
	} else {
		text = voiceInstructionsBlockRe.ReplaceAllString(text, "")
		text = voiceXMLTagRe.ReplaceAllString(text, "")
	}
	var userLines []string
	var fallbackLines []string
	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(voiceTimestampLineRe.ReplaceAllString(strings.TrimSpace(rawLine), ""))
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		isUserLine := strings.HasPrefix(lower, "user:") || strings.HasPrefix(lower, "user：") ||
			strings.HasPrefix(lower, "human:") || strings.HasPrefix(lower, "human：") ||
			strings.HasPrefix(lower, "me:") || strings.HasPrefix(lower, "me：") ||
			strings.HasPrefix(line, "我:") || strings.HasPrefix(line, "我：")
		stripped := strings.TrimSpace(voiceSpeakerPrefixRe.ReplaceAllString(line, ""))
		if stripped == "" {
			continue
		}
		if isUserLine {
			userLines = append(userLines, stripped)
		} else {
			fallbackLines = append(fallbackLines, stripped)
		}
	}
	if len(userLines) > 0 {
		return strings.Join(userLines, "\n")
	}
	return strings.Join(fallbackLines, "\n")
}

func firstTwoLinesTruncated(text string, maxRunes int) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	var lines []string
	for _, l := range strings.Split(trimmed, "\n") {
		if s := strings.TrimSpace(l); s != "" {
			lines = append(lines, s)
			if len(lines) == 2 {
				break
			}
		}
	}
	combined := strings.Join(lines, " ")
	runes := []rune(combined)
	if len(runes) > maxRunes {
		cut := maxRunes - 3
		if cut < 1 {
			cut = maxRunes
		}
		return string(runes[:cut]) + "..."
	}
	return combined
}
