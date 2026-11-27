package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// BtwSideQuestionEntry stores metadata about a /btw side-question cascade linked to a parent chat.
type BtwSideQuestionEntry struct {
	CascadeID       string `json:"cascadeId"`
	ParentCascadeID string `json:"parentCascadeId"`
	Title           string `json:"title"`
	Question        string `json:"question"`
	Quote           string `json:"quote,omitempty"`
	Status          string `json:"status,omitempty"`
	CreatedAt       string `json:"createdAt,omitempty"`
	ForkStepIndex   *int   `json:"forkStepIndex,omitempty"`
	AnswerPreview   string `json:"answerPreview,omitempty"`
	TurnCount       int    `json:"turnCount,omitempty"`
	cachedStepCount int
	cachedStatus    string
}

type btwTracker struct {
	mu       sync.RWMutex
	byParent map[string][]BtwSideQuestionEntry
	bySide   map[string]BtwSideQuestionEntry
}

var defaultBtwTracker = &btwTracker{
	byParent: make(map[string][]BtwSideQuestionEntry),
	bySide:   make(map[string]BtwSideQuestionEntry),
}

const maxBtwTrackedSideQuestions = 512

func (t *btwTracker) record(entry BtwSideQuestionEntry) {
	if entry.CascadeID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if prev, exists := t.bySide[entry.CascadeID]; exists {
		if entry.ParentCascadeID == "" {
			entry.ParentCascadeID = prev.ParentCascadeID
		}
		if entry.Title == "" {
			entry.Title = prev.Title
		}
		if entry.Question == "" {
			entry.Question = prev.Question
		}
		if entry.Quote == "" {
			entry.Quote = prev.Quote
		}
		if entry.CreatedAt == "" {
			entry.CreatedAt = prev.CreatedAt
		}
		if entry.ForkStepIndex == nil {
			entry.ForkStepIndex = prev.ForkStepIndex
		}
		if entry.AnswerPreview == "" && entry.cachedStepCount == 0 {
			entry.AnswerPreview = prev.AnswerPreview
			entry.TurnCount = prev.TurnCount
			entry.cachedStepCount = prev.cachedStepCount
			entry.cachedStatus = prev.cachedStatus
		}
	} else if len(t.bySide) >= maxBtwTrackedSideQuestions {
		// Evict ~25% of entries when bounded cap is reached.
		evicted := 0
		for sid, old := range t.bySide {
			delete(t.bySide, sid)
			if old.ParentCascadeID != "" {
				list := t.byParent[old.ParentCascadeID]
				filtered := list[:0]
				for _, it := range list {
					if it.CascadeID != sid {
						filtered = append(filtered, it)
					}
				}
				if len(filtered) == 0 {
					delete(t.byParent, old.ParentCascadeID)
				} else {
					t.byParent[old.ParentCascadeID] = filtered
				}
			}
			evicted++
			if evicted >= maxBtwTrackedSideQuestions/4 {
				break
			}
		}
	}
	t.bySide[entry.CascadeID] = entry
	if entry.ParentCascadeID != "" {
		list := t.byParent[entry.ParentCascadeID]
		replaced := false
		for i := range list {
			if list[i].CascadeID == entry.CascadeID {
				list[i] = entry
				replaced = true
				break
			}
		}
		if !replaced {
			list = append(list, entry)
		}
		t.byParent[entry.ParentCascadeID] = list
	}
}

func (t *btwTracker) remove(cascadeID string) string {
	if cascadeID == "" {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	old, ok := t.bySide[cascadeID]
	if !ok {
		return ""
	}
	delete(t.bySide, cascadeID)
	if old.ParentCascadeID != "" {
		list := t.byParent[old.ParentCascadeID]
		filtered := list[:0]
		for _, it := range list {
			if it.CascadeID != cascadeID {
				filtered = append(filtered, it)
			}
		}
		if len(filtered) == 0 {
			delete(t.byParent, old.ParentCascadeID)
		} else {
			t.byParent[old.ParentCascadeID] = filtered
		}
	}
	return old.ParentCascadeID
}

func (t *btwTracker) isSideQuestion(cascadeID string) bool {
	if cascadeID == "" {
		return false
	}
	t.mu.RLock()
	_, ok := t.bySide[cascadeID]
	t.mu.RUnlock()
	return ok
}

func (t *btwTracker) listForParent(parentID string) []BtwSideQuestionEntry {
	if parentID == "" {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	src := t.byParent[parentID]
	out := make([]BtwSideQuestionEntry, len(src))
	copy(out, src)
	return out
}

func (t *btwTracker) getSide(cascadeID string) (BtwSideQuestionEntry, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	e, ok := t.bySide[cascadeID]
	return e, ok
}

func (t *btwTracker) snapshotAll() map[string]BtwSideQuestionEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]BtwSideQuestionEntry, len(t.bySide))
	for k, v := range t.bySide {
		out[k] = v
	}
	return out
}

// isSideQuestionTrajectoryMap returns true when a GetAllCascadeTrajectories entry is a /btw side question.
func isSideQuestionTrajectoryMap(s map[string]interface{}, id string) bool {
	if defaultBtwTracker.isSideQuestion(id) {
		return true
	}
	if tt, _ := s["trajectoryType"].(string); tt == "CORTEX_TRAJECTORY_TYPE_SIDE_QUESTION" {
		return true
	}
	if meta, ok := s["trajectoryMetadata"].(map[string]interface{}); ok && meta != nil {
		if tt, _ := meta["trajectoryType"].(string); tt == "CORTEX_TRAJECTORY_TYPE_SIDE_QUESTION" {
			return true
		}
	}
	return false
}

// isSideQuestionBoundaryStep reports whether a raw step is the upstream system message inserted
// by Antigravity LS immediately after the inherited parent history of a /btw side question.
func isSideQuestionBoundaryStep(s TrajectoryStep) bool {
	if s.Type != "CORTEX_STEP_TYPE_SYSTEM_MESSAGE" || s.SystemMessage == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(firstNonEmpty(s.SystemMessage.Message, s.SystemMessage.Content)))
	return strings.HasPrefix(msg, "above is the conversation history from the parent conversation")
}

// resolveSideQuestionForkStepIndex snaps a user-selected message stepIndex in the parent cascade
// to a valid turn boundary required by Antigravity LS (fork.requireForkAtTurnBoundary rejects user
// steps and mid-turn steps). Returns (snappedStep, useForkFrom):
//   - If the selected message belongs to an earlier completed turn, useForkFrom is true and
//     snappedStep is that turn's final non-user step (immediately preceding the next USER_INPUT).
//   - If the selected message belongs to the latest turn (or no valid earlier boundary exists),
//     useForkFrom is false so StartCascade uses baseTrajectoryIdentifier.cascadeId (matching desktop).
func resolveSideQuestionForkStepIndex(steps []TrajectoryStep, requested int) (int, bool) {
	n := len(steps)
	if n == 0 {
		return requested, requested >= 0
	}
	if requested < 0 {
		return n - 1, false
	}
	if requested >= n-1 {
		return n - 1, false
	}
	end := requested
	for end+1 < n && steps[end+1].Type != "CORTEX_STEP_TYPE_USER_INPUT" {
		end++
	}
	// If the chosen turn has no assistant/tool response at all (a bare USER_INPUT followed directly
	// by another USER_INPUT), walk back to the preceding completed turn boundary.
	for end >= 0 && steps[end].Type == "CORTEX_STEP_TYPE_USER_INPUT" {
		end--
	}
	if end < 0 || end >= n-1 {
		return n - 1, false
	}
	return end, true
}

// extractBranchOrigin inspects upstream parentReferences, side-question boundary system messages,
// and in-memory tracker metadata to determine whether a trajectory is a /btw side question or fork branch.
func extractBranchOrigin(rawResp *upstreamTrajectoryResp, steps []TrajectoryStep) *SideQuestionOrigin {
	if rawResp == nil {
		return nil
	}
	cid := strings.TrimSpace(rawResp.Trajectory.CascadeID)
	isSide := rawResp.Trajectory.TrajectoryType == "CORTEX_TRAJECTORY_TYPE_SIDE_QUESTION" || defaultBtwTracker.isSideQuestion(cid)

	var parentID string
	forkStepIndex := -1
	hasForkRef := false
	for i := len(rawResp.Trajectory.ParentReferences) - 1; i >= 0; i-- {
		ref := rawResp.Trajectory.ParentReferences[i]
		isExplicitFork := ref.ReferenceType == "CORTEX_TRAJECTORY_REFERENCE_TYPE_FORK" ||
			ref.ReferenceType == "CORTEX_TRAJECTORY_REFERENCE_TYPE_THOUGHT_REWRITE"
		isSideFallbackRef := isSide && (ref.ReferenceType == "" || ref.ReferenceType == "CORTEX_TRAJECTORY_REFERENCE_TYPE_UNSPECIFIED")
		if (isExplicitFork || isSideFallbackRef) &&
			strings.TrimSpace(ref.ConversationID) != "" &&
			strings.TrimSpace(ref.ConversationID) != cid {
			parentID = strings.TrimSpace(ref.ConversationID)
			forkStepIndex = ref.StepIndex
			hasForkRef = true
			break
		}
	}

	if entry, ok := defaultBtwTracker.getSide(cid); ok {
		isSide = true
		if parentID == "" && entry.ParentCascadeID != "" {
			parentID = entry.ParentCascadeID
		}
		if !hasForkRef && entry.ForkStepIndex != nil && *entry.ForkStepIndex >= 0 {
			forkStepIndex = *entry.ForkStepIndex
			hasForkRef = true
		}
	}

	// Fallback: Antigravity LS automatically inserts a CORTEX_STEP_TYPE_SYSTEM_MESSAGE
	// right after forkStepIndex starting with "Above is the conversation history from the parent conversation".
	// Only match this exact boundary prefix (and never misclassify an explicit CASCADE trajectory whose
	// context summary happens to discuss side questions).
	if !hasForkRef && (isSide || rawResp.Trajectory.TrajectoryType == "") {
		for idx, s := range steps {
			if isSideQuestionBoundaryStep(s) {
				isSide = true
				forkStepIndex = idx - 1
				hasForkRef = true
				break
			}
		}
	}

	if !hasForkRef && !isSide {
		return nil
	}

	forkTurnNumber := 0
	if forkStepIndex >= 0 {
		limit := forkStepIndex
		if limit >= len(steps) {
			limit = len(steps) - 1
		}
		for i := 0; i <= limit; i++ {
			if steps[i].Type == "CORTEX_STEP_TYPE_USER_INPUT" {
				forkTurnNumber++
			}
		}
	}

	parentTitle := ""
	if parentID != "" {
		parentTitle = getCachedCascadeTitleAny(parentID)
	}

	return &SideQuestionOrigin{
		ParentCascadeID: parentID,
		ParentTitle:     parentTitle,
		ForkStepIndex:   forkStepIndex,
		ForkTurnNumber:  forkTurnNumber,
		IsSideQuestion:  isSide,
	}
}

func getCachedCascadeTitleAny(cascadeID string) string {
	if cascadeID == "" {
		return ""
	}
	if t := getAuthoritativeCachedCascadeTitle(cascadeID); t != "" {
		return t
	}
	if t := readAnnotationTitle(cascadeID); t != "" && t != "未命名会话" {
		return SanitizeTitle(t)
	}
	defaultTrajCache.cascadeTitlesMu.RLock()
	t := defaultTrajCache.cascadeTitles[cascadeID]
	defaultTrajCache.cascadeTitlesMu.RUnlock()
	if t != "" && t != "未命名会话" {
		return t
	}
	return ""
}

func messageBranchStepIndex(msg CascadeMessageItem) (int, bool) {
	if msg.StepIndex != nil {
		return *msg.StepIndex, true
	}
	if strings.HasPrefix(msg.ID, "step-") {
		if n, err := strconv.Atoi(strings.TrimPrefix(msg.ID, "step-")); err == nil {
			return n, true
		}
	}
	if len(msg.ToolDetails) > 0 || len(msg.ThinkingBlocks) > 0 {
		maxStep := -1
		for _, td := range msg.ToolDetails {
			if td.StepIndex > maxStep {
				maxStep = td.StepIndex
			}
		}
		for _, tb := range msg.ThinkingBlocks {
			if tb.StepIndex > maxStep {
				maxStep = tb.StepIndex
			}
		}
		if maxStep >= 0 {
			return maxStep, true
		}
	}
	if msg.StepPlan != nil && msg.StepPlan.AnchorStepIndex >= 0 {
		return msg.StepPlan.AnchorStepIndex, true
	}
	return 0, false
}

// extractSideQuestionBranchMessages slices a side-question trajectory's parsed messages
// so ONLY steps after forkStepIndex are returned (never leaking shared parent history).
func extractSideQuestionBranchMessages(rawResp *upstreamTrajectoryResp, details TrajectoryDetails, entry BtwSideQuestionEntry) (forkStepIndex int, branchMsgs []CascadeMessageItem, question, thinking, answer, answerPreview string, turnCount int) {
	forkStepIndex = -1
	if origin := details.BranchOrigin; origin != nil && origin.ForkStepIndex >= 0 {
		forkStepIndex = origin.ForkStepIndex
	} else if origin := extractBranchOrigin(rawResp, details.Steps); origin != nil && origin.ForkStepIndex >= 0 {
		forkStepIndex = origin.ForkStepIndex
	} else if entry.ForkStepIndex != nil && *entry.ForkStepIndex >= 0 {
		forkStepIndex = *entry.ForkStepIndex
	}

	startIdx := 0
	if forkStepIndex >= 0 {
		firstBranchIdx := -1
		lastSharedIdx := -1
		for idx, msg := range details.AllMessages {
			if si, ok := messageBranchStepIndex(msg); ok {
				if si > forkStepIndex {
					if firstBranchIdx < 0 {
						firstBranchIdx = idx
					}
				} else {
					lastSharedIdx = idx
				}
			}
		}
		if firstBranchIdx >= 0 {
			startIdx = firstBranchIdx
		} else if lastSharedIdx >= 0 {
			startIdx = lastSharedIdx + 1
		}
	}

	for _, msg := range details.AllMessages[startIdx:] {
		if forkStepIndex >= 0 {
			if si, ok := messageBranchStepIndex(msg); ok && si <= forkStepIndex {
				continue
			}
		}
		branchMsgs = append(branchMsgs, msg)
		switch msg.Type {
		case "user":
			turnCount++
			if question == "" {
				question = strings.TrimSpace(msg.Content)
			}
		case "agent":
			if msg.Thinking != "" {
				if thinking != "" {
					thinking += "\n\n"
				}
				thinking += strings.TrimSpace(msg.Thinking)
			}
			if content := strings.TrimSpace(msg.Content); content != "" {
				if answer != "" {
					answer += "\n\n"
				}
				answer += content
				answerPreview = content
			}
		case "thinking":
			t := firstNonEmpty(msg.Thinking, msg.Content)
			if t != "" {
				if thinking != "" {
					thinking += "\n\n"
				}
				thinking += strings.TrimSpace(t)
			}
		}
	}

	if question == "" && entry.Question != "" {
		question = entry.Question
	}
	if answerPreview != "" {
		answerPreview = compactSidePreview(answerPreview, 140)
	}
	return
}

var (
	sidePreviewHRRe         = regexp.MustCompile(`^([-*_]\s*){3,}$`)
	sidePreviewTableSepRe   = regexp.MustCompile(`^\|?[\s:]*-{3,}[\s:]*(?:\|[\s:]*-{3,}[\s:]*)+\|?$`)
	sidePreviewQuoteRe      = regexp.MustCompile(`^(?:>\s*)+`)
	sidePreviewHeadingRe    = regexp.MustCompile(`^#{1,6}\s+`)
	sidePreviewBulletRe     = regexp.MustCompile(`^[-*+]\s+`)
	sidePreviewImageRe      = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	sidePreviewHtmlBrRe     = regexp.MustCompile(`(?i)<br\s*/?>`)
	sidePreviewBrokenImgRe  = regexp.MustCompile(`!\[[^\]]*(?:\]\([^)]*)?$`)
	sidePreviewBrokenLinkRe = regexp.MustCompile(`\[([^\[\]]+)\]\([^\)]*$`)
)

func compactSidePreview(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	normalized := strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	rawLines := strings.Split(normalized, "\n")
	cleanedLines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			continue
		}
		if sidePreviewHRRe.MatchString(t) || sidePreviewTableSepRe.MatchString(t) {
			continue
		}
		t = sidePreviewQuoteRe.ReplaceAllString(t, "")
		t = strings.TrimSpace(t)
		t = sidePreviewHeadingRe.ReplaceAllString(t, "")
		t = sidePreviewBulletRe.ReplaceAllString(t, "• ")
		if strings.HasPrefix(t, "|") && strings.HasSuffix(t, "|") && strings.Count(t, "|") >= 2 {
			cells := strings.Split(strings.Trim(t, "|"), "|")
			nonEmpty := make([]string, 0, len(cells))
			for _, cell := range cells {
				if c := strings.TrimSpace(cell); c != "" {
					nonEmpty = append(nonEmpty, c)
				}
			}
			t = strings.Join(nonEmpty, " · ")
		}
		t = sidePreviewImageRe.ReplaceAllStringFunc(t, func(m string) string {
			sub := sidePreviewImageRe.FindStringSubmatch(m)
			if len(sub) > 1 {
				return strings.TrimSpace(sub[1])
			}
			return ""
		})
		t = sidePreviewHtmlBrRe.ReplaceAllString(t, " ")
		if t = strings.TrimSpace(t); t != "" {
			cleanedLines = append(cleanedLines, t)
		}
	}
	if len(cleanedLines) == 0 {
		return ""
	}
	// Flatten newlines and redundant spaces for compact 1-2 line card preview.
	flat := strings.Join(strings.Fields(strings.Join(cleanedLines, " ")), " ")
	return repairTruncatedInlineMarkdown(truncateRunes(flat, maxRunes))
}

func repairTruncatedInlineMarkdown(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	suffix := ""
	body := s
	if strings.HasSuffix(body, "…") {
		suffix = "…"
		body = strings.TrimSuffix(body, "…")
	} else if strings.HasSuffix(body, "...") {
		suffix = "..."
		body = strings.TrimSuffix(body, "...")
	}

	body = sidePreviewBrokenImgRe.ReplaceAllString(body, "")
	body = sidePreviewBrokenLinkRe.ReplaceAllString(body, "$1")

	// Close or trim an unbalanced trailing inline code backtick.
	inCode := false
	lastBacktick := -1
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' && i+1 < len(body) {
			i++
			continue
		}
		if body[i] == '`' {
			inCode = !inCode
			if inCode {
				lastBacktick = i
			}
		}
	}
	if inCode && lastBacktick >= 0 {
		after := strings.TrimSpace(body[lastBacktick+1:])
		if after == "" {
			body = strings.TrimRight(body[:lastBacktick], " \t")
		} else {
			body = strings.TrimRight(body, " \t") + "`"
		}
	}

	type delimEntry struct {
		delim string
		pos   int
	}
	var stack []delimEntry
	inCode = false
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' && i+1 < len(body) {
			i++
			continue
		}
		if body[i] == '`' {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}
		if strings.HasPrefix(body[i:], "***") {
			if n := len(stack); n > 0 && stack[n-1].delim == "***" {
				stack = stack[:n-1]
			} else {
				stack = append(stack, delimEntry{delim: "***", pos: i})
			}
			i += 2
			continue
		}
		if strings.HasPrefix(body[i:], "**") {
			if n := len(stack); n > 0 && stack[n-1].delim == "**" {
				stack = stack[:n-1]
			} else if n > 0 && stack[n-1].delim == "***" {
				stack[n-1].delim = "*"
			} else {
				stack = append(stack, delimEntry{delim: "**", pos: i})
			}
			i++
			continue
		}
		if strings.HasPrefix(body[i:], "~~") {
			if n := len(stack); n > 0 && stack[n-1].delim == "~~" {
				stack = stack[:n-1]
			} else {
				stack = append(stack, delimEntry{delim: "~~", pos: i})
			}
			i++
			continue
		}
		if body[i] == '*' {
			prevSpace := i == 0 || body[i-1] == ' ' || body[i-1] == '\t'
			nextSpace := i+1 >= len(body) || body[i+1] == ' ' || body[i+1] == '\t'
			if prevSpace && nextSpace {
				continue
			}
			if n := len(stack); n > 0 && stack[n-1].delim == "*" {
				stack = stack[:n-1]
			} else if n > 0 && stack[n-1].delim == "***" {
				stack[n-1].delim = "**"
			} else {
				stack = append(stack, delimEntry{delim: "*", pos: i})
			}
		}
	}

	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if top.pos+len(top.delim) <= len(body) && strings.TrimSpace(body[top.pos+len(top.delim):]) == "" {
			body = strings.TrimRight(body[:top.pos], " \t")
		} else {
			body = strings.TrimRight(body, " \t") + top.delim
		}
	}

	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	return body + suffix
}

func formatBtwPrompt(question, quote string) string {
	q := strings.TrimSpace(question)
	qt := strings.TrimSpace(quote)
	if qt == "" {
		return q
	}
	lines := strings.Split(qt, "\n")
	for i := range lines {
		lines[i] = "> " + lines[i]
	}
	quoted := strings.Join(lines, "\n")
	if q == "" {
		return "请解释以下内容：\n" + quoted
	}
	return "关于以下内容：\n" + quoted + "\n\n" + q
}

func formatBtwInsertText(cascadeID, title, question, answer string) string {
	headerTitle := strings.TrimSpace(title)
	if headerTitle == "" {
		headerTitle = strings.TrimSpace(question)
	}
	body := strings.TrimSpace(answer)
	if body == "" {
		return ""
	}
	header := fmt.Sprintf("Side question (%s): %s", shortCascadeID(cascadeID), headerTitle)
	if headerTitle == "" {
		header = fmt.Sprintf("Side question (%s)", shortCascadeID(cascadeID))
	}
	lines := strings.Split(body, "\n")
	for i := range lines {
		lines[i] = "> " + lines[i]
	}
	return "**" + header + "**\n" + strings.Join(lines, "\n")
}

// HandleBtw handles:
//   - POST /gateway/btw/start
//   - POST /gateway/btw/reply
//   - POST /gateway/btw/cancel
//   - GET  /gateway/btw/list?cascadeId=...
//   - GET  /gateway/btw/status?cascadeId=...
func (p *Proxy) HandleBtw(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/gateway/btw/start":
		p.handleBtwStart(w, r)
	case "/gateway/btw/reply":
		p.handleBtwReply(w, r)
	case "/gateway/btw/cancel":
		p.handleBtwCancel(w, r)
	case "/gateway/btw/list":
		p.handleBtwList(w, r)
	case "/gateway/btw/status":
		p.handleBtwStatus(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (p *Proxy) handleBtwStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ParentCascadeID string        `json:"parentCascadeId"`
		Question        string        `json:"question"`
		Quote           string        `json:"quote"`
		Model           string        `json:"model"`
		StepIndex       *int          `json:"stepIndex,omitempty"`
		Attachments     []interface{} `json:"attachments"`
		Media           []interface{} `json:"media"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	promptText := formatBtwPrompt(req.Question, req.Quote)
	if strings.TrimSpace(promptText) == "" && len(req.Attachments) == 0 && len(req.Media) == 0 {
		writeRPCError(w, http.StatusBadRequest, "question or quote is required")
		return
	}

	parentID := strings.TrimSpace(req.ParentCascadeID)
	port, token := p.ActiveUpstream()
	startPayload := map[string]interface{}{
		"source":         "CORTEX_TRAJECTORY_SOURCE_CASCADE_CLIENT",
		"trajectoryType": "CORTEX_TRAJECTORY_TYPE_SIDE_QUESTION",
	}
	var requestedForkStep *int
	if parentID != "" {
		upstreamParentID := ResolveActiveCascadeID(parentID)
		if req.StepIndex != nil && *req.StepIndex >= 0 {
			forkTargetID := upstreamParentID
			si := *req.StepIndex
			if len(GetStitchChainSegments(parentID)) > 1 {
				forkTargetID, si = p.ResolveGlobalStepToSegment(r.Context(), parentID, si, port, token)
			}
			useForkFrom := true
			if port > 0 {
				var parentRaw *upstreamTrajectoryResp
				var pErr error
				if len(GetStitchChainSegments(parentID)) > 1 {
					parentRaw, pErr = p.fetchSingleSegmentTrajectory(r.Context(), forkTargetID, port, token, 2*time.Second)
				} else {
					parentRaw, pErr = p.fetchUpstreamTrajectoryWithMaxAgeContext(r.Context(), forkTargetID, port, token, 2*time.Second)
				}
				if pErr == nil && parentRaw != nil && len(parentRaw.Trajectory.Steps) > 0 {
					si, useForkFrom = resolveSideQuestionForkStepIndex(parentRaw.Trajectory.Steps, si)
				}
			}
			if len(GetStitchChainSegments(parentID)) > 1 {
				// Preserve the client-visible (global) stepIndex for local side-question anchor mapping
				globalStep := *req.StepIndex
				requestedForkStep = &globalStep
			} else {
				requestedForkStep = &si
			}
			if useForkFrom {
				startPayload["baseTrajectoryIdentifier"] = map[string]interface{}{
					"forkFrom": map[string]interface{}{
						"cascadeId":     forkTargetID,
						"stepIndex":     si,
						"referenceType": "CORTEX_TRAJECTORY_REFERENCE_TYPE_FORK",
					},
				}
			} else {
				startPayload["baseTrajectoryIdentifier"] = map[string]interface{}{"cascadeId": forkTargetID}
			}
		} else {
			startPayload["baseTrajectoryIdentifier"] = map[string]interface{}{"cascadeId": upstreamParentID}
		}
	}

	startRaw, code, err := p.callUpstreamRPC(r.Context(), "StartCascade", startPayload)
	if err != nil {
		writeRPCError(w, code, err.Error())
		return
	}
	if code != http.StatusOK {
		writeRPCError(w, code, strings.TrimSpace(string(startRaw)))
		return
	}
	var startRes struct {
		CascadeID string `json:"cascadeId"`
	}
	if err := json.Unmarshal(startRaw, &startRes); err != nil || strings.TrimSpace(startRes.CascadeID) == "" {
		writeRPCError(w, http.StatusBadGateway, "upstream StartCascade returned empty cascadeId")
		return
	}
	sideID := strings.TrimSpace(startRes.CascadeID)
	RecordCascadeCreatedAtCurrentEpoch(sideID)

	// Best-effort resolve forkStepIndex before sending when stepIndex was not explicitly specified.
	if requestedForkStep == nil && parentID != "" && port > 0 {
		if preRaw, preErr := p.fetchUpstreamTrajectoryWithMaxAgeContext(r.Context(), sideID, port, token, 0); preErr == nil && preRaw != nil {
			if origin := extractBranchOrigin(preRaw, preRaw.Trajectory.Steps); origin != nil && origin.ForkStepIndex >= 0 {
				si := origin.ForkStepIndex
				requestedForkStep = &si
			}
		}
	}

	msgMap := map[string]interface{}{
		"cascadeId": sideID,
		"items":     []interface{}{map[string]interface{}{"text": promptText}},
	}
	if len(req.Attachments) > 0 {
		msgMap["attachments"] = req.Attachments
		if _, err := applyAttachmentsToMessage(msgMap); err != nil {
			writeRPCError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if len(req.Media) > 0 {
		if existing, ok := msgMap["media"].([]interface{}); ok {
			msgMap["media"] = append(existing, req.Media...)
		} else {
			msgMap["media"] = req.Media
		}
	}

	// Inherit model & cascadeConfig from parent cascade or requested model
	p.ensureLiveModels(r.Context())
	targetModel := strings.TrimSpace(req.Model)
	var cfgObj interface{}
	if parentID != "" && port > 0 {
		if cfg := p.GetCascadeConfig(parentID, port, token); len(cfg) > 0 {
			_ = json.Unmarshal(cfg, &cfgObj)
		}
		if targetModel == "" {
			targetModel, _ = GetCascadeModel(parentID)
		}
	}
	if targetModel == "" {
		targetModel = "gemini-3.8-flash-high"
	}
	if modelEnum := resolveModelEnum(targetModel); modelEnum != "" {
		canonicalName := canonicalModelName(targetModel)
		if canonicalName == "" {
			canonicalName = targetModel
		}
		cfgObj = applyModelToCascadeConfig(cfgObj, modelEnum, canonicalName)
		recordedName := effectiveCascadeModelName(cfgObj, canonicalName)
		if cfgBytes, err := json.Marshal(cfgObj); err == nil {
			SetCascadeModel(sideID, recordedName, cfgBytes)
		}
	}
	if cfgObj != nil {
		msgMap["cascadeConfig"] = cfgObj
		applyOrchestratorAgentSpec(msgMap, cfgObj)
	}

	sendRaw, sendCode, sendErr := p.callUpstreamRPC(r.Context(), "SendUserCascadeMessage", msgMap)
	if sendErr != nil {
		writeRPCError(w, sendCode, sendErr.Error())
		return
	}
	if sendCode != http.StatusOK {
		writeRPCError(w, sendCode, strings.TrimSpace(string(sendRaw)))
		return
	}

	title := strings.TrimSpace(req.Question)
	if title == "" {
		title = strings.TrimSpace(req.Quote)
	}
	if rTitle := []rune(title); len(rTitle) > 60 {
		title = string(rTitle[:60]) + "..."
	}
	entry := BtwSideQuestionEntry{
		CascadeID:       sideID,
		ParentCascadeID: parentID,
		Title:           title,
		Question:        strings.TrimSpace(req.Question),
		Quote:           strings.TrimSpace(req.Quote),
		Status:          "CASCADE_RUN_STATUS_RUNNING",
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		ForkStepIndex:   requestedForkStep,
		TurnCount:       1,
	}
	defaultBtwTracker.record(entry)
	if title != "" {
		setCachedCascadeTitle(sideID, title)
	}
	ClearTrajectoryCache(sideID)
	p.invalidateAllTrajectories()
	if parentID != "" {
		p.notifyStreamTouch(parentID)
	}

	// Best-effort annotation sync onto parent cascade so Antigravity desktop shows this side question too.
	if parentID != "" {
		entries := defaultBtwTracker.listForParent(parentID)
		sideIDs := make([]string, 0, len(entries))
		titles := make(map[string]string, len(entries))
		for _, e := range entries {
			sideIDs = append(sideIDs, e.CascadeID)
			if e.Title != "" {
				titles[e.CascadeID] = e.Title
			}
		}
		go func(pid string, ids []string, tmap map[string]string) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, _, _ = p.callUpstreamRPC(ctx, "UpdateConversationAnnotations", map[string]interface{}{
				"cascadeId":        pid,
				"mergeAnnotations": true,
				"annotations": map[string]interface{}{
					"sideQuestionCascadeIds": ids,
					"sideQuestionTitles":     tmap,
				},
			})
		}(parentID, sideIDs, titles)
	}

	resp := map[string]interface{}{
		"ok":              true,
		"cascadeId":       sideID,
		"parentCascadeId": parentID,
		"title":           title,
		"question":        entry.Question,
		"quote":           entry.Quote,
		"createdAt":       entry.CreatedAt,
	}
	if requestedForkStep != nil {
		resp["forkStepIndex"] = *requestedForkStep
	}
	writeJSONStatus(w, http.StatusOK, resp)
}

func (p *Proxy) handleBtwReply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		CascadeID   string        `json:"cascadeId"`
		Message     string        `json:"message"`
		Question    string        `json:"question"`
		Model       string        `json:"model"`
		Attachments []interface{} `json:"attachments"`
		Media       []interface{} `json:"media"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	sideID := strings.TrimSpace(req.CascadeID)
	if sideID == "" {
		writeRPCError(w, http.StatusBadRequest, "cascadeId is required")
		return
	}
	text := strings.TrimSpace(firstNonEmpty(req.Message, req.Question))
	if text == "" && len(req.Attachments) == 0 && len(req.Media) == 0 {
		writeRPCError(w, http.StatusBadRequest, "message is required")
		return
	}

	port, token := p.ActiveUpstream()
	msgMap := map[string]interface{}{
		"cascadeId": sideID,
		"items":     []interface{}{map[string]interface{}{"text": text}},
	}
	if len(req.Attachments) > 0 {
		msgMap["attachments"] = req.Attachments
		if _, err := applyAttachmentsToMessage(msgMap); err != nil {
			writeRPCError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if len(req.Media) > 0 {
		if existing, ok := msgMap["media"].([]interface{}); ok {
			msgMap["media"] = append(existing, req.Media...)
		} else {
			msgMap["media"] = req.Media
		}
	}

	p.ensureLiveModels(r.Context())
	targetModel := strings.TrimSpace(req.Model)
	var cfgObj interface{}
	if port > 0 {
		if cfg := p.GetCascadeConfig(sideID, port, token); len(cfg) > 0 {
			_ = json.Unmarshal(cfg, &cfgObj)
		}
		if targetModel == "" {
			targetModel, _ = GetCascadeModel(sideID)
		}
		if entry, ok := defaultBtwTracker.getSide(sideID); ok && entry.ParentCascadeID != "" {
			if cfgObj == nil {
				if cfg := p.GetCascadeConfig(entry.ParentCascadeID, port, token); len(cfg) > 0 {
					_ = json.Unmarshal(cfg, &cfgObj)
				}
			}
			if targetModel == "" {
				targetModel, _ = GetCascadeModel(entry.ParentCascadeID)
			}
		}
	}
	if targetModel == "" {
		targetModel = "gemini-3.8-flash-high"
	}
	if modelEnum := resolveModelEnum(targetModel); modelEnum != "" {
		canonicalName := canonicalModelName(targetModel)
		if canonicalName == "" {
			canonicalName = targetModel
		}
		cfgObj = applyModelToCascadeConfig(cfgObj, modelEnum, canonicalName)
		recordedName := effectiveCascadeModelName(cfgObj, canonicalName)
		if cfgBytes, err := json.Marshal(cfgObj); err == nil {
			SetCascadeModel(sideID, recordedName, cfgBytes)
		}
	}
	if cfgObj != nil {
		msgMap["cascadeConfig"] = cfgObj
		applyOrchestratorAgentSpec(msgMap, cfgObj)
	}

	sendRaw, sendCode, sendErr := p.callUpstreamRPC(r.Context(), "SendUserCascadeMessage", msgMap)
	if sendErr != nil {
		writeRPCError(w, sendCode, sendErr.Error())
		return
	}
	if sendCode != http.StatusOK {
		writeRPCError(w, sendCode, strings.TrimSpace(string(sendRaw)))
		return
	}

	ClearTrajectoryCache(sideID)
	p.invalidateAllTrajectories()
	p.notifyStreamTouch(sideID)
	if entry, ok := defaultBtwTracker.getSide(sideID); ok {
		entry.Status = "CASCADE_RUN_STATUS_RUNNING"
		entry.cachedStepCount = 0
		defaultBtwTracker.record(entry)
		if entry.ParentCascadeID != "" {
			p.notifyStreamTouch(entry.ParentCascadeID)
		}
	}

	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"cascadeId": sideID,
	})
}

func (p *Proxy) handleBtwCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		CascadeID string `json:"cascadeId"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	sideID := strings.TrimSpace(req.CascadeID)
	if sideID == "" {
		writeRPCError(w, http.StatusBadRequest, "cascadeId is required")
		return
	}
	raw, code, err := p.callUpstreamRPC(r.Context(), "CancelCascadeInvocation", map[string]interface{}{
		"cascadeId": sideID,
	})
	if err != nil {
		writeRPCError(w, code, err.Error())
		return
	}
	if code != http.StatusOK {
		writeRPCError(w, code, strings.TrimSpace(string(raw)))
		return
	}
	ClearTrajectoryCache(sideID)
	p.invalidateAllTrajectories()
	p.notifyStreamTouch(sideID)
	if entry, ok := defaultBtwTracker.getSide(sideID); ok && entry.ParentCascadeID != "" {
		p.notifyStreamTouch(entry.ParentCascadeID)
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"cascadeId": sideID,
	})
}

// notifySideQuestionDeleted cleans up tracker state and refreshes the parent's stream when a side question is deleted.
func (p *Proxy) notifySideQuestionDeleted(sideID string) {
	if sideID == "" {
		return
	}
	parentID := defaultBtwTracker.remove(sideID)
	if parentID != "" {
		p.notifyStreamTouch(parentID)
		entries := defaultBtwTracker.listForParent(parentID)
		ids := make([]string, 0, len(entries))
		tmap := make(map[string]string, len(entries))
		for _, e := range entries {
			if e.CascadeID == sideID || IsDeletedCascade(e.CascadeID) {
				continue
			}
			ids = append(ids, e.CascadeID)
			if e.Title != "" {
				tmap[e.CascadeID] = e.Title
			}
		}
		go func(pid string, keepIDs []string, titles map[string]string) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, _, _ = p.callUpstreamRPC(ctx, "UpdateConversationAnnotations", map[string]interface{}{
				"cascadeId":        pid,
				"mergeAnnotations": true,
				"annotations": map[string]interface{}{
					"sideQuestionCascadeIds": keepIDs,
					"sideQuestionTitles":     titles,
				},
			})
		}(parentID, ids, tmap)
	}
}

func (p *Proxy) handleBtwList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	parentID := strings.TrimSpace(r.URL.Query().Get("cascadeId"))
	if parentID == "" {
		writeRPCError(w, http.StatusBadRequest, "cascadeId is required")
		return
	}

	port, token := p.ActiveUpstream()
	items := p.collectSideQuestionEntries(r.Context(), parentID, port, token)

	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"parentCascadeId": parentID,
		"items":           items,
	})
}

type parsedSummaryForBtw struct {
	Status      string `json:"status"`
	Summary     string `json:"summary"`
	StepCount   int    `json:"stepCount"`
	CreatedTime string `json:"createdTime"`
	Annotations struct {
		SideQuestionCascadeIDs []string          `json:"sideQuestionCascadeIds"`
		SideQuestionTitles     map[string]string `json:"sideQuestionTitles"`
	} `json:"annotations"`
}

func (p *Proxy) collectSideQuestionEntries(ctx context.Context, parentID string, port int, token string) []BtwSideQuestionEntry {
	tracked := defaultBtwTracker.listForParent(parentID)
	items := make([]BtwSideQuestionEntry, 0, len(tracked))
	seen := make(map[string]int, len(tracked))
	for _, it := range tracked {
		if IsDeletedCascade(it.CascadeID) {
			continue
		}
		seen[it.CascadeID] = len(items)
		items = append(items, it)
	}

	var parsed struct {
		Summaries map[string]parsedSummaryForBtw `json:"trajectorySummaries"`
	}
	hasSummaries := false
	if port > 0 {
		if raw, err := p.fetchAllTrajectoriesRaw(ctx, port, token); err == nil {
			if json.Unmarshal(raw, &parsed) == nil {
				hasSummaries = true
			}
		}
	}

	if hasSummaries {
		if parentSum, ok := parsed.Summaries[parentID]; ok {
			for _, sid := range parentSum.Annotations.SideQuestionCascadeIDs {
				if sid == "" || IsDeletedCascade(sid) {
					continue
				}
				title := strings.TrimSpace(parentSum.Annotations.SideQuestionTitles[sid])
				sideSum, hasSum := parsed.Summaries[sid]
				if idx, exists := seen[sid]; exists {
					if hasSum && sideSum.Status != "" {
						items[idx].Status = sideSum.Status
					}
					if title != "" && (items[idx].Title == "" || items[idx].Title == "Side question") {
						items[idx].Title = title
					}
					if items[idx].CreatedAt == "" && sideSum.CreatedTime != "" {
						items[idx].CreatedAt = sideSum.CreatedTime
					}
				} else {
					e := BtwSideQuestionEntry{
						CascadeID:       sid,
						ParentCascadeID: parentID,
						Title:           firstNonEmpty(title, sideSum.Summary, "旁路快问"),
						Question:        firstNonEmpty(title, sideSum.Summary),
						Status:          sideSum.Status,
						CreatedAt:       sideSum.CreatedTime,
					}
					defaultBtwTracker.record(e)
					seen[sid] = len(items)
					items = append(items, e)
				}
			}
		}
		for i := range items {
			sideSum, hasSum := parsed.Summaries[items[i].CascadeID]
			if hasSum && sideSum.Status != "" {
				items[i].Status = sideSum.Status
			}
			// Enrich forkStepIndex + branch-only preview when uncached or stepCount/status changed.
			needHydrate := items[i].ForkStepIndex == nil ||
				(hasSum && (items[i].cachedStepCount != sideSum.StepCount || items[i].cachedStatus != sideSum.Status)) ||
				(items[i].AnswerPreview == "" && items[i].Status != "CASCADE_RUN_STATUS_RUNNING")
			if needHydrate && port > 0 {
				if rawResp, err := p.fetchUpstreamTrajectoryWithMaxAgeContext(ctx, items[i].CascadeID, port, token, 2*time.Second); err == nil && rawResp != nil {
					det := p.ParseTrajectoryDetailsCached(rawResp)
					forkIdx, _, q, _, _, preview, turns := extractSideQuestionBranchMessages(rawResp, det, items[i])
					if forkIdx >= 0 {
						fi := forkIdx
						items[i].ForkStepIndex = &fi
					}
					if q != "" {
						items[i].Question = q
						if items[i].Title == "" || items[i].Title == "旁路快问" || items[i].Title == "Side question" {
							items[i].Title = truncateRunes(q, 60)
						}
					}
					if preview != "" {
						items[i].AnswerPreview = preview
					}
					if turns > 0 {
						items[i].TurnCount = turns
					}
					if det.Status != "" {
						items[i].Status = det.Status
					}
					if hasSum {
						items[i].cachedStepCount = sideSum.StepCount
						items[i].cachedStatus = sideSum.Status
					} else {
						items[i].cachedStepCount = det.TotalSteps
						items[i].cachedStatus = det.Status
					}
					defaultBtwTracker.record(items[i])
				}
			}
		}
	}
	return items
}

// buildSideQuestionCards builds inline branch cards anchored to parent messages for HTTP & WebSocket payloads.
func (p *Proxy) buildSideQuestionCards(ctx context.Context, parentID string, allMessages []CascadeMessageItem, port int, token string) []SideQuestionCardItem {
	if parentID == "" {
		return nil
	}
	entries := p.collectSideQuestionEntries(ctx, parentID, port, token)
	if len(entries) == 0 {
		return nil
	}
	out := make([]SideQuestionCardItem, 0, len(entries))
	for _, e := range entries {
		forkIdx := -1
		if e.ForkStepIndex != nil {
			forkIdx = *e.ForkStepIndex
		}
		anchorID := resolveSideQuestionAnchorMessageID(allMessages, forkIdx)
		status := "done"
		switch e.Status {
		case "CASCADE_RUN_STATUS_RUNNING":
			status = "running"
		case "CASCADE_RUN_STATUS_ERROR":
			status = "error"
		}
		out = append(out, SideQuestionCardItem{
			CascadeID:       e.CascadeID,
			ParentCascadeID: parentID,
			Title:           firstNonEmpty(e.Title, e.Question, "旁路快问"),
			Question:        firstNonEmpty(e.Question, e.Title),
			AnswerPreview:   e.AnswerPreview,
			Status:          status,
			ForkStepIndex:   forkIdx,
			AnchorMessageID: anchorID,
			TurnCount:       e.TurnCount,
			CreatedAt:       e.CreatedAt,
		})
	}
	return out
}

// resolveSideQuestionAnchorMessageID finds the parent message bubble that a side question branched from.
func resolveSideQuestionAnchorMessageID(allMessages []CascadeMessageItem, forkStepIndex int) string {
	if len(allMessages) == 0 {
		return ""
	}
	if forkStepIndex >= 0 {
		bestID := ""
		bestFallbackID := ""
		for _, m := range allMessages {
			if m.StepIndex == nil {
				continue
			}
			if *m.StepIndex <= forkStepIndex {
				if m.Type == "agent" || m.Type == "user" {
					bestID = m.ID
				}
				if m.Type != "tools" && m.Type != "step_plan" {
					bestFallbackID = m.ID
				}
			}
		}
		if bestID != "" {
			return bestID
		}
		if bestFallbackID != "" {
			return bestFallbackID
		}
	}
	for i := len(allMessages) - 1; i >= 0; i-- {
		m := allMessages[i]
		if m.Type == "agent" || m.Type == "user" {
			return m.ID
		}
	}
	return allMessages[len(allMessages)-1].ID
}

// sideQuestionLiveSignature folds side-question live state into parent stream change detection.
func (p *Proxy) sideQuestionLiveSignature(parentID string) string {
	if parentID == "" {
		return ""
	}
	port, token := p.ActiveUpstream()
	if port == 0 {
		return ""
	}
	tracked := defaultBtwTracker.listForParent(parentID)
	raw, err := p.fetchAllTrajectoriesRaw(context.Background(), port, token)
	if err != nil && len(tracked) == 0 {
		return ""
	}
	var sideIDs []string
	seen := map[string]bool{}
	var parsed struct {
		Summaries map[string]parsedSummaryForBtw `json:"trajectorySummaries"`
	}
	if err == nil && json.Unmarshal(raw, &parsed) == nil {
		if parentSum, ok := parsed.Summaries[parentID]; ok {
			for _, sid := range parentSum.Annotations.SideQuestionCascadeIDs {
				if sid != "" && !IsDeletedCascade(sid) && !seen[sid] {
					seen[sid] = true
					sideIDs = append(sideIDs, sid)
				}
			}
		}
	}
	for _, it := range tracked {
		if it.CascadeID != "" && !IsDeletedCascade(it.CascadeID) && !seen[it.CascadeID] {
			seen[it.CascadeID] = true
			sideIDs = append(sideIDs, it.CascadeID)
		}
	}
	if len(sideIDs) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, sid := range sideIDs {
		sb.WriteString(sid)
		sb.WriteByte(':')
		if sum, ok := parsed.Summaries[sid]; ok {
			fmt.Fprintf(&sb, "%s:%d:%s;", sum.Status, sum.StepCount, sum.Summary)
		} else if e, ok := defaultBtwTracker.getSide(sid); ok {
			fmt.Fprintf(&sb, "%s:%d:%s;", e.Status, e.TurnCount, e.Title)
		} else {
			sb.WriteString("?;")
		}
	}
	return sb.String()
}

func (p *Proxy) handleBtwStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sideID := strings.TrimSpace(r.URL.Query().Get("cascadeId"))
	if sideID == "" {
		writeRPCError(w, http.StatusBadRequest, "cascadeId is required")
		return
	}

	port, token := p.ActiveUpstream()
	if port <= 0 {
		writeRPCError(w, http.StatusServiceUnavailable, "no active Antigravity upstream")
		return
	}

	rawResp, err := p.fetchUpstreamTrajectory(sideID, port, token)
	if err != nil {
		writeRPCError(w, http.StatusBadGateway, err.Error())
		return
	}
	details := p.ParseTrajectoryDetailsCached(rawResp)
	entry, _ := defaultBtwTracker.getSide(sideID)

	forkStepIndex, branchMsgs, question, thinking, answer, answerPreview, turnCount := extractSideQuestionBranchMessages(rawResp, details, entry)
	if branchMsgs == nil {
		branchMsgs = []CascadeMessageItem{}
	}

	parentID := entry.ParentCascadeID
	parentTitle := ""
	forkTurnNumber := 0
	if origin := details.BranchOrigin; origin != nil {
		if parentID == "" {
			parentID = origin.ParentCascadeID
		}
		parentTitle = origin.ParentTitle
		forkTurnNumber = origin.ForkTurnNumber
	}

	isRunning := details.Status == "CASCADE_RUN_STATUS_RUNNING"
	title := firstNonEmpty(entry.Title, question, details.Title)
	insertText := formatBtwInsertText(sideID, title, question, answer)

	if forkStepIndex >= 0 || answerPreview != "" || parentID != "" {
		updated := entry
		updated.CascadeID = sideID
		updated.ParentCascadeID = parentID
		updated.Title = firstNonEmpty(updated.Title, title)
		updated.Question = firstNonEmpty(updated.Question, question)
		updated.Status = details.Status
		if forkStepIndex >= 0 {
			fi := forkStepIndex
			updated.ForkStepIndex = &fi
		}
		if answerPreview != "" {
			updated.AnswerPreview = answerPreview
		}
		if turnCount > 0 {
			updated.TurnCount = turnCount
		}
		updated.cachedStepCount = details.TotalSteps
		updated.cachedStatus = details.Status
		defaultBtwTracker.record(updated)
	}

	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"cascadeId":       sideID,
		"parentCascadeId": parentID,
		"parentTitle":     parentTitle,
		"forkStepIndex":   forkStepIndex,
		"forkTurnNumber":  forkTurnNumber,
		"status":          details.Status,
		"isRunning":       isRunning,
		"hasError":        details.HasError,
		"errorMessage":    details.ErrorMessage,
		"title":           title,
		"question":        question,
		"quote":           entry.Quote,
		"thinking":        thinking,
		"answer":          answer,
		"answerPreview":   answerPreview,
		"turnCount":       turnCount,
		"messages":        branchMsgs,
		"insertText":      insertText,
		"totalSteps":      details.TotalSteps,
	})
}
