package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	nextPromptTimeout       = 12 * time.Second
	nextPromptMaxCacheSize  = 48
	nextPromptStaleCutoff   = 30 * time.Minute
	nextPromptMaxUserRunes  = 600
	nextPromptMaxAsstRunes  = 1200
	nextPromptAsstTailRunes = 420
	nextPromptMaxSugRunes   = 90
)

const (
	SuggestionSourceNative    = "native"
	SuggestionSourcePredicted = "predicted"
)

type nextPromptCacheEntry struct {
	TurnKey    string
	Suggestion string
	Abstained  bool
	UpdatedAt  time.Time
}

type nextPromptEngine struct {
	mu       sync.RWMutex
	cache    map[string]nextPromptCacheEntry
	inFlight map[string]string // cascadeID -> turnKey currently predicting
}

var (
	reNextPromptAbstain = regexp.MustCompile(`(?i)^(?:no_suggestion|none|n/a|null|empty|skip|\(none\)|无建议|留空|なし)$`)
	reNextPromptFiller  = regexp.MustCompile(`(?i)^(?:继续|继续吧|请继续|好的|好|行|收到|谢谢|感谢|明白|没问题|ok|okay|continue|go ahead|proceed|thanks|thank you|looks good|sounds good|yes|no|done|got it|はい|了解|ありがとう|続けて|続行)\s*[。.!！?？]*$`)
	reNextPromptBullet  = regexp.MustCompile(`^[-*•\d.)]+\s+`)
	reTaskMdLine        = regexp.MustCompile(`^\s*[-*]\s*\[( |x|X|/)\]\s+(.+?)\s*$`)
	reBGNotifPrefix     = regexp.MustCompile(`(?i)^(?:\[(?:后台任务|cron|scheduled|background|system|notification|timer)\]|<system_message>|<task-notification>)`)
	rePunctNorm         = regexp.MustCompile(`[\s，。,.!！?？、:;：；"'` + "`" + `()（）\[\]]+`)
)

func (e *nextPromptEngine) clearAll() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	ids := make([]string, 0, len(e.cache))
	for id := range e.cache {
		ids = append(ids, id)
	}
	e.cache = make(map[string]nextPromptCacheEntry)
	e.inFlight = make(map[string]string)
	return ids
}

func (e *nextPromptEngine) get(cascadeID, turnKey string) (nextPromptCacheEntry, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.cache == nil {
		return nextPromptCacheEntry{}, false
	}
	entry, ok := e.cache[cascadeID]
	if !ok || entry.TurnKey != turnKey {
		return nextPromptCacheEntry{}, false
	}
	return entry, true
}

func (e *nextPromptEngine) signature(cascadeID string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.cache == nil {
		return ""
	}
	entry, ok := e.cache[cascadeID]
	if !ok || entry.Suggestion == "" {
		return ""
	}
	return entry.TurnKey + ":" + entry.Suggestion
}

func (e *nextPromptEngine) put(cascadeID string, entry nextPromptCacheEntry) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cache == nil {
		e.cache = make(map[string]nextPromptCacheEntry)
	}
	if len(e.cache) >= nextPromptMaxCacheSize {
		var oldestID string
		var oldestAt time.Time
		for k, v := range e.cache {
			if oldestID == "" || v.UpdatedAt.Before(oldestAt) {
				oldestID = k
				oldestAt = v.UpdatedAt
			}
		}
		if oldestID != "" {
			delete(e.cache, oldestID)
		}
	}
	e.cache[cascadeID] = entry
}

func (p *Proxy) clearAllNextPromptCache() {
	ids := p.nextPrompt.clearAll()
	for _, id := range ids {
		p.notifyStreamTouch(id)
	}
}

func (p *Proxy) nextPromptSignature(cascadeID string) string {
	if p == nil || !p.AIAssistPrefs().NextPromptEnabled {
		return ""
	}
	return p.nextPrompt.signature(cascadeID)
}

func computeNextPromptTurnKey(details *TrajectoryDetails) string {
	if details == nil {
		return ""
	}
	lastID := ""
	lastLen := 0
	if n := len(details.AllMessages); n > 0 {
		last := details.AllMessages[n-1]
		lastID = last.ID
		lastLen = len(last.Text)
	}
	return fmt.Sprintf("%d:%d:%s:%d", details.TotalSteps, len(details.AllMessages), lastID, lastLen)
}

type nextPromptChecklist struct {
	Completed  []string
	InProgress []string
	Pending    []string
}

func collectNextPromptChecklist(cascadeID string, details *TrajectoryDetails) nextPromptChecklist {
	var out nextPromptChecklist
	seen := make(map[string]bool)
	add := func(dst *[]string, text string) {
		t := redactSensitiveForAI(text)
		if t == "" {
			return
		}
		t = truncateRunesTail(t, 120)
		key := strings.ToLower(t)
		if seen[key] {
			return
		}
		seen[key] = true
		*dst = append(*dst, t)
	}

	// 1. Primary source: structured ActivePlan parsed from update_steps.
	if details != nil && details.ActivePlan != nil {
		for _, item := range details.ActivePlan.Items {
			switch item.Status {
			case "done", "skipped":
				add(&out.Completed, item.Title)
			case "in_progress":
				add(&out.InProgress, item.Title)
			case "pending", "failed":
				add(&out.Pending, item.Title)
			}
		}
	}

	// 2. Secondary source: read-only brain/<cascadeId>/task.md checkboxes.
	if cascadeID != "" && cascadeIDRe.MatchString(cascadeID) {
		if taskPath := resolveAntigravitySubPath("brain", cascadeID, "task.md"); taskPath != "" {
			if raw, err := os.ReadFile(taskPath); err == nil && len(raw) > 0 && len(raw) <= 256*1024 {
				for _, line := range strings.Split(string(raw), "\n") {
					m := reTaskMdLine.FindStringSubmatch(line)
					if len(m) != 3 {
						continue
					}
					switch strings.ToLower(m[1]) {
					case "x":
						add(&out.Completed, m[2])
					case "/":
						add(&out.InProgress, m[2])
					default:
						add(&out.Pending, m[2])
					}
				}
			}
		}
	}
	return out
}

func truncateRunesHeadAndTail(s string, maxRunes, tailRunes int) string {
	clean := redactSensitiveForAI(s)
	runes := []rune(clean)
	if len(runes) <= maxRunes {
		return clean
	}
	if tailRunes > maxRunes/2 {
		tailRunes = maxRunes / 2
	}
	if tailRunes < 60 {
		tailRunes = 60
	}
	headRunes := maxRunes - tailRunes - 8
	if headRunes < 40 {
		headRunes = 40
	}
	return strings.TrimSpace(string(runes[:headRunes])) + "\n…[omitted]…\n" + strings.TrimSpace(string(runes[len(runes)-tailRunes:]))
}

func truncateRunesTail(s string, maxRunes int) string {
	clean := redactSensitiveForAI(s)
	runes := []rune(clean)
	if len(runes) <= maxRunes {
		return clean
	}
	return "…" + strings.TrimSpace(string(runes[len(runes)-maxRunes:]))
}

func detectPromptLanguageLabel(text string) string {
	var han, kana, latin int
	for _, r := range text {
		switch {
		case (r >= 0x3040 && r <= 0x30ff):
			kana++
		case unicode.Is(unicode.Han, r):
			han++
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			latin++
		}
	}
	if kana >= 2 {
		return "Japanese"
	}
	if han >= 3 || (han > 0 && han*2 >= latin) {
		return "Simplified Chinese"
	}
	return "English"
}

func isBackgroundNotificationMessage(text string) bool {
	t := strings.TrimSpace(text)
	return t == "" || reBGNotifPrefix.MatchString(t)
}

func shouldSkipNextPromptPrediction(details *TrajectoryDetails, rawResp *upstreamTrajectoryResp) (bool, string) {
	if details == nil {
		return true, "nil_details"
	}
	if len(details.SuggestedResponses) > 0 {
		return true, "native_suggestions_present"
	}
	if details.Status == "CASCADE_RUN_STATUS_RUNNING" || details.HasRunningSubagents {
		return true, "running"
	}
	if details.HasError {
		return true, "has_error"
	}
	if details.CanProceed {
		return true, "can_proceed"
	}
	if details.PendingInteraction != nil {
		return true, "pending_interaction"
	}
	if len(details.QueuedMessages) > 0 {
		return true, "queued_messages"
	}
	if details.ParentConversation != "" {
		return true, "subagent"
	}
	if details.BranchOrigin != nil && details.BranchOrigin.IsSideQuestion {
		return true, "side_question"
	}
	if len(details.AllMessages) < 2 {
		return true, "too_few_messages"
	}

	// Verify last message is a completed assistant reply, and there is a real non-background user message.
	lastMsg := details.AllMessages[len(details.AllMessages)-1]
	if lastMsg.Role != "agent" && lastMsg.Role != "assistant" {
		return true, "last_not_assistant"
	}
	if strings.TrimSpace(lastMsg.Text) == "" {
		return true, "empty_assistant_reply"
	}

	// Check staleness: avoid spending calls when browsing old historical sessions.
	if rawResp != nil {
		if lastAct := extractLastActivityAt(rawResp.Trajectory.Steps); lastAct != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, lastAct); err == nil {
				if time.Since(parsed) > nextPromptStaleCutoff {
					return true, "stale_session"
				}
			} else if parsed, err := time.Parse(time.RFC3339, lastAct); err == nil {
				if time.Since(parsed) > nextPromptStaleCutoff {
					return true, "stale_session"
				}
			}
		}
	}

	// Check last user message is real (not a cron/background notification).
	var lastUserText string
	for i := len(details.AllMessages) - 2; i >= 0; i-- {
		m := details.AllMessages[i]
		if m.Role == "user" {
			lastUserText = strings.TrimSpace(m.Text)
			break
		}
	}
	if isBackgroundNotificationMessage(lastUserText) {
		return true, "background_or_missing_user"
	}
	return false, ""
}

func buildNextPromptInput(cascadeID string, details *TrajectoryDetails) (string, nextPromptChecklist) {
	checklist := collectNextPromptChecklist(cascadeID, details)

	var userTexts []string
	var lastUser string
	var lastAssistant CascadeMessageItem
	msgs := details.AllMessages
	startIdx := len(msgs) - 8
	if startIdx < 0 {
		startIdx = 0
	}
	window := msgs[startIdx:]
	for _, m := range window {
		if m.Role == "user" && !isBackgroundNotificationMessage(m.Text) {
			userTexts = append(userTexts, m.Text)
			lastUser = m.Text
		} else if m.Role == "agent" || m.Role == "assistant" {
			lastAssistant = m
		}
	}

	langLabel := detectPromptLanguageLabel(lastUser)
	if lastUser == "" {
		langLabel = detectPromptLanguageLabel(lastAssistant.Text)
	}

	maxHint := "max 55 characters"
	if langLabel == "Simplified Chinese" || langLabel == "Japanese" {
		maxHint = "max 26 characters"
	}

	var sb strings.Builder
	sb.WriteString("You predict the single most likely next short message the USER will type into a mobile chat box after a completed assistant turn.\n")
	sb.WriteString(fmt.Sprintf("Output language MUST match the user's language (%s) in a concise, natural, direct tone.\n", langLabel))
	sb.WriteString("Strict selection priority:\n")
	sb.WriteString("1. First priority: the smallest concrete next step the user explicitly asked for in recent goals or remaining pending task items that has NOT been done yet.\n")
	sb.WriteString("2. Second priority: a concrete verification, test run, or follow-up check strictly grounded in current task evidence and not yet completed.\n")
	sb.WriteString("3. Third priority: a natural user answer to a question in the assistant reply ONLY when earlier user goals already provide clear clues.\n")
	sb.WriteString("Strict abstention rules (output ONLY the exact token NO_SUGGESTION in all these cases):\n")
	sb.WriteString("- The task and user goals are already verified/complete with no remaining pending items.\n")
	sb.WriteString("- The assistant asks an open-ended question or offers choices (e.g. A vs B) where the user gave no prior preference.\n")
	sb.WriteString("- Predicting would guess unstated requirements, expand scope, or produce generic filler like \"continue\", \"ok\", \"thanks\", \"继续\", or \"好的\".\n")
	sb.WriteString("Hard constraints:\n")
	sb.WriteString(fmt.Sprintf("- Output ONLY ONE short, directly sendable user-voiced sentence (%s, no quotes, no bullets, no markdown), OR the exact token NO_SUGGESTION.\n", maxHint))
	sb.WriteString("- Never repeat items already marked completed [x].\n\n")

	if len(userTexts) > 0 {
		goalStart := len(userTexts) - 3
		if goalStart < 0 {
			goalStart = 0
		}
		sb.WriteString("Confirmed recent user goals:\n")
		for _, g := range userTexts[goalStart:] {
			sb.WriteString("- ")
			sb.WriteString(truncateRunesHeadAndTail(g, 240, 100))
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}

	if len(checklist.Completed) > 0 || len(checklist.InProgress) > 0 || len(checklist.Pending) > 0 {
		sb.WriteString("Task checklist state:\n")
		if len(checklist.Completed) > 0 {
			sb.WriteString("Already completed [x] (DO NOT suggest again): ")
			sb.WriteString(strings.Join(checklist.Completed, "; "))
			sb.WriteByte('\n')
		}
		if len(checklist.InProgress) > 0 {
			sb.WriteString("In progress [/]: ")
			sb.WriteString(strings.Join(checklist.InProgress, "; "))
			sb.WriteByte('\n')
		}
		if len(checklist.Pending) > 0 {
			sb.WriteString("Remaining pending [ ]: ")
			sb.WriteString(strings.Join(checklist.Pending, "; "))
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}

	if len(window) > 2 {
		sb.WriteString("Earlier trimmed turns:\n")
		for _, m := range window[:len(window)-2] {
			tag := "[ASSISTANT]"
			if m.Role == "user" {
				if isBackgroundNotificationMessage(m.Text) {
					tag = "[BACKGROUND_NOTIFICATION]"
				} else {
					tag = "[REAL_USER]"
				}
			}
			sb.WriteString(tag)
			sb.WriteByte(' ')
			sb.WriteString(truncateRunesHeadAndTail(m.Text, 200, 80))
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}

	sb.WriteString("[REAL_USER] Last user request:\n")
	sb.WriteString(truncateRunesTail(lastUser, nextPromptMaxUserRunes))
	sb.WriteString("\n\n")

	if len(lastAssistant.ToolDetails) > 0 {
		var toolLines []string
		startTool := len(lastAssistant.ToolDetails) - 6
		if startTool < 0 {
			startTool = 0
		}
		for _, td := range lastAssistant.ToolDetails[startTool:] {
			line := strings.TrimSpace(td.Name)
			if td.Summary != "" {
				line += ": " + truncateRunesTail(td.Summary, 100)
			}
			if td.Status != "" && strings.Contains(strings.ToLower(td.Status), "error") {
				line += " [ERROR]"
			}
			if line != "" {
				toolLines = append(toolLines, line)
			}
		}
		if len(toolLines) > 0 {
			sb.WriteString("[TOOL_SUMMARY]\n")
			sb.WriteString(strings.Join(toolLines, "\n"))
			sb.WriteString("\n\n")
		}
	}

	sb.WriteString("[ASSISTANT] Last assistant reply tail:\n")
	sb.WriteString(truncateRunesHeadAndTail(lastAssistant.Text, nextPromptMaxAsstRunes, nextPromptAsstTailRunes))

	return sb.String(), checklist
}

func sanitizeNextPromptSuggestion(raw string, checklist nextPromptChecklist) string {
	s := strings.TrimSpace(redactSensitiveForAI(raw))
	if s == "" {
		return ""
	}
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	s = reNextPromptBullet.ReplaceAllString(s, "")
	s = strings.Trim(s, "\"'`「」『』")
	s = strings.TrimSpace(s)

	if s == "" || reNextPromptAbstain.MatchString(s) || reNextPromptFiller.MatchString(s) {
		return ""
	}
	if utf8.RuneCountInString(s) > nextPromptMaxSugRunes {
		return ""
	}

	// Reject suggestions that duplicate an already-completed checklist item.
	normSug := strings.ToLower(rePunctNorm.ReplaceAllString(s, ""))
	if normSug == "" {
		return ""
	}
	for _, done := range checklist.Completed {
		normDone := strings.ToLower(rePunctNorm.ReplaceAllString(done, ""))
		if utf8.RuneCountInString(normDone) >= 6 && (normSug == normDone || strings.Contains(normSug, normDone) || strings.Contains(normDone, normSug)) {
			return ""
		}
	}
	return s
}

// ResolveOrScheduleNextPrompt returns effective (suggestedResponses, source) for an active stream frame.
// When native suggestions exist, they are returned untouched ("native").
// Otherwise, if NextPromptEnabled is on, cached predictions are returned immediately ("predicted") or
// an asynchronous prediction is scheduled once per turnKey and pushes back via notifyStreamTouch.
func (p *Proxy) ResolveOrScheduleNextPrompt(cascadeID string, details *TrajectoryDetails, rawResp *upstreamTrajectoryResp, allowSchedule bool) ([]string, string) {
	if details == nil {
		return nil, ""
	}
	if len(details.SuggestedResponses) > 0 {
		return details.SuggestedResponses, SuggestionSourceNative
	}
	if p == nil || !p.AIAssistPrefs().NextPromptEnabled || !p.IsAIAssistAvailable() {
		return nil, ""
	}

	if skip, _ := shouldSkipNextPromptPrediction(details, rawResp); skip {
		return nil, ""
	}

	turnKey := computeNextPromptTurnKey(details)
	if entry, ok := p.nextPrompt.get(cascadeID, turnKey); ok {
		if entry.Abstained || entry.Suggestion == "" {
			return nil, ""
		}
		return []string{entry.Suggestion}, SuggestionSourcePredicted
	}

	if !allowSchedule {
		return nil, ""
	}

	// Single-flight guard per (cascadeID, turnKey).
	p.nextPrompt.mu.Lock()
	if p.nextPrompt.inFlight == nil {
		p.nextPrompt.inFlight = make(map[string]string)
	}
	if p.nextPrompt.inFlight[cascadeID] == turnKey {
		p.nextPrompt.mu.Unlock()
		return nil, ""
	}
	p.nextPrompt.inFlight[cascadeID] = turnKey
	p.nextPrompt.mu.Unlock()

	prompt, checklist := buildNextPromptInput(cascadeID, details)

	go func() {
		defer func() {
			p.nextPrompt.mu.Lock()
			if p.nextPrompt.inFlight[cascadeID] == turnKey {
				delete(p.nextPrompt.inFlight, cascadeID)
			}
			p.nextPrompt.mu.Unlock()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), nextPromptTimeout)
		defer cancel()

		rawText, _, err := p.CallLSModelResponse(ctx, prompt, nil)
		if err != nil {
			slog.Debug("[NextPrompt] prediction failed", "cascade_id", shortCascadeID(cascadeID), "err", err)
			p.nextPrompt.put(cascadeID, nextPromptCacheEntry{
				TurnKey:   turnKey,
				Abstained: true,
				UpdatedAt: time.Now(),
			})
			return
		}

		cleaned := sanitizeNextPromptSuggestion(rawText, checklist)
		p.nextPrompt.put(cascadeID, nextPromptCacheEntry{
			TurnKey:    turnKey,
			Suggestion: cleaned,
			Abstained:  cleaned == "",
			UpdatedAt:  time.Now(),
		})
		if cleaned != "" {
			p.notifyStreamTouch(cascadeID)
		}
	}()

	return nil, ""
}
