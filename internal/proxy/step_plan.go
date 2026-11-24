package proxy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	stepPlanMarkdownLineRe = regexp.MustCompile(`^\s*[-*]\s+\[([ >xX\-!])\]\s+(.+?)\s*$`)
	stepPlanMultiSpaceRe   = regexp.MustCompile(`\s+`)
)

func isValidStepPlanStatus(status string) bool {
	switch status {
	case "pending", "in_progress", "done", "skipped", "failed":
		return true
	default:
		return false
	}
}

func isUpdateStepsStep(s TrajectoryStep) bool {
	return extractToolNameFromStep(s) == "update_steps"
}

func isUpdateFocusStep(s TrajectoryStep) bool {
	return extractToolNameFromStep(s) == "update_focus"
}

func hasStepError(s TrajectoryStep) bool {
	if s.Error != nil && (strings.TrimSpace(s.Error.ShortError) != "" || strings.TrimSpace(s.Error.FullError) != "") {
		return true
	}
	if s.ErrorMessage != nil && (strings.TrimSpace(s.ErrorMessage.Error.ShortError) != "" || strings.TrimSpace(s.ErrorMessage.Error.UserErrorMessage) != "") {
		return true
	}
	return s.Status == "CORTEX_STEP_STATUS_ERROR"
}

func parseStepPlanMarkdown(md string) []StepPlanItemEntry {
	if strings.TrimSpace(md) == "" {
		return nil
	}
	var items []StepPlanItemEntry
	for _, line := range strings.Split(md, "\n") {
		m := stepPlanMarkdownLineRe.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		title := strings.TrimSpace(m[2])
		if title == "" {
			continue
		}
		status := "pending"
		switch m[1] {
		case ">":
			status = "in_progress"
		case "x", "X":
			status = "done"
		case "-":
			status = "skipped"
		case "!":
			status = "failed"
		}
		items = append(items, StepPlanItemEntry{
			Title:  title,
			Status: status,
		})
	}
	if len(items) == 0 {
		return nil
	}
	return items
}

// parseStepPlanJSON mirrors upstream desktop BSc: accepts a JSON array of {title, status},
// an object {"steps": ...}, or a JSON-encoded string wrapping either.
func parseStepPlanJSON(raw []byte) []StepPlanItemEntry {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	// Unwrap JSON-encoded string (e.g. generic.args["steps"] is a protobuf map<string,string> value)
	if strings.HasPrefix(trimmed, `"`) {
		var inner string
		if err := json.Unmarshal([]byte(trimmed), &inner); err == nil {
			return parseStepPlanJSON([]byte(inner))
		}
		return nil
	}
	// Object wrapper {"steps": ...}
	if strings.HasPrefix(trimmed, "{") {
		var wrapper struct {
			Steps json.RawMessage `json:"steps"`
		}
		if err := json.Unmarshal([]byte(trimmed), &wrapper); err == nil && len(wrapper.Steps) > 0 {
			return parseStepPlanJSON(wrapper.Steps)
		}
		return nil
	}
	// Direct array [{title, status}, ...]
	if strings.HasPrefix(trimmed, "[") {
		var arr []struct {
			Title  string `json:"title"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(trimmed), &arr); err != nil {
			return nil
		}
		items := make([]StepPlanItemEntry, 0, len(arr))
		for _, it := range arr {
			title := strings.TrimSpace(it.Title)
			if title == "" {
				continue
			}
			status := strings.TrimSpace(it.Status)
			if !isValidStepPlanStatus(status) {
				status = "pending"
			}
			items = append(items, StepPlanItemEntry{
				Title:  title,
				Status: status,
			})
		}
		if len(items) > 0 {
			return items
		}
	}
	return nil
}

func finalizeStepPlan(anchorStepIndex int, raw []StepPlanItemEntry) *StepPlanItem {
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(raw))
	items := make([]StepPlanItemEntry, 0, len(raw))
	doneCount := 0
	skippedCount := 0
	failedCount := 0
	for _, it := range raw {
		baseKey := stepPlanMultiSpaceRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(it.Title)), " ")
		key := baseKey
		for k := 2; seen[key]; k++ {
			key = fmt.Sprintf("%s#%d", baseKey, k)
		}
		seen[key] = true
		items = append(items, StepPlanItemEntry{
			Key:    key,
			Title:  it.Title,
			Status: it.Status,
		})
		switch it.Status {
		case "done":
			doneCount++
		case "skipped":
			skippedCount++
		case "failed":
			failedCount++
		}
	}
	return &StepPlanItem{
		AnchorStepIndex: anchorStepIndex,
		Items:           items,
		DoneCount:       doneCount,
		TotalCount:      len(items) - skippedCount,
		SkippedCount:    skippedCount,
		FailedCount:     failedCount,
	}
}

// extractStepPlanFromStep parses an update_steps step matching desktop ESc semantics:
// 1) stepRenderInfo.markdown (survives CLEARED compaction)
// 2) generic.args["steps"]
// 3) metadata.toolCall.argumentsJson / toolCall.argumentsJson
func extractStepPlanFromStep(s TrajectoryStep, stepIdx int) *StepPlanItem {
	if !isUpdateStepsStep(s) || hasStepError(s) {
		return nil
	}
	var parsed []StepPlanItemEntry
	if s.Generic != nil && s.Generic.Result != nil && s.Generic.Result.StepRenderInfo != nil {
		parsed = parseStepPlanMarkdown(s.Generic.Result.StepRenderInfo.Markdown)
	}
	if s.Status == "CORTEX_STEP_STATUS_CLEARED" {
		return finalizeStepPlan(stepIdx, parsed)
	}
	if len(parsed) == 0 && s.Generic != nil && len(s.Generic.Args) > 0 {
		if rawSteps, ok := s.Generic.Args["steps"]; ok {
			parsed = parseStepPlanJSON(rawSteps)
		}
	}
	if len(parsed) == 0 {
		if argsJSON := extractStepToolArgsJSON(s); argsJSON != "" {
			parsed = parseStepPlanJSON([]byte(argsJSON))
		}
	}
	return finalizeStepPlan(stepIdx, parsed)
}

// extractFocusFromStep parses an update_focus step matching desktop eea semantics
// with safe fallbacks to generic.args["focus"] and toolCall.argumentsJson.
func extractFocusFromStep(s TrajectoryStep) string {
	if !isUpdateFocusStep(s) || hasStepError(s) {
		return ""
	}
	if s.Generic != nil && s.Generic.Result != nil && s.Generic.Result.StepRenderInfo != nil {
		if title := strings.TrimSpace(s.Generic.Result.StepRenderInfo.Title); title != "" {
			return title
		}
	}
	if s.Generic != nil && len(s.Generic.Args) > 0 {
		if rawFocus, ok := s.Generic.Args["focus"]; ok {
			var str string
			if err := json.Unmarshal(rawFocus, &str); err == nil {
				if trimmed := strings.TrimSpace(str); trimmed != "" {
					return trimmed
				}
			} else if trimmed := strings.Trim(strings.TrimSpace(string(rawFocus)), `"`); trimmed != "" {
				return trimmed
			}
		}
	}
	if argsJSON := extractStepToolArgsJSON(s); argsJSON != "" {
		var obj struct {
			Focus string `json:"focus"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &obj); err == nil {
			if trimmed := strings.TrimSpace(obj.Focus); trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func stepPlanFallbackText(plan *StepPlanItem) string {
	if plan == nil || len(plan.Items) == 0 {
		return ""
	}
	parts := make([]string, 0, 3)
	if plan.TotalCount > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d steps", plan.DoneCount, plan.TotalCount))
	}
	if plan.FailedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", plan.FailedCount))
	}
	if plan.SkippedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", plan.SkippedCount))
	}
	return strings.Join(parts, " · ")
}

// extractActiveTurnPlanAndFocus extracts the latest step plan and active focus summary
// for the current (last) user turn, used by Live Activity / Dynamic Island and active thinking bubbles.
func extractActiveTurnPlanAndFocus(steps []TrajectoryStep, lastUserIdx int) (*StepPlanItem, string, string) {
	start := lastUserIdx + 1
	if start < 0 {
		start = 0
	}
	var activePlan *StepPlanItem
	anchorIdx := -1
	activeFocus := ""
	activeFocusStartedAt := ""

	for i := start; i < len(steps); i++ {
		s := steps[i]
		if isUpdateStepsStep(s) {
			if p := extractStepPlanFromStep(s, i); p != nil {
				if anchorIdx < 0 {
					anchorIdx = i
				}
				p.AnchorStepIndex = anchorIdx
				activePlan = p
			}
			continue
		}
		if isUpdateFocusStep(s) {
			if f := extractFocusFromStep(s); f != "" {
				activeFocus = f
				if activeFocusStartedAt == "" {
					activeFocusStartedAt = stepStartTimestamp(s)
				}
			}
			continue
		}
		if s.Type == "CORTEX_STEP_TYPE_PLANNER_RESPONSE" && s.PlannerResponse != nil {
			if CleanInternalSystemMessage(s.PlannerResponse.Response) != "" {
				// Mirror desktop j1b: an assistant text message resets the trailing focus & timer.
				activeFocus = ""
				activeFocusStartedAt = ""
			} else if strings.TrimSpace(s.PlannerResponse.Thinking) != "" && activeFocusStartedAt == "" {
				activeFocusStartedAt = stepStartTimestamp(s)
			}
			continue
		}
		if strings.HasPrefix(s.Type, "CORTEX_STEP_TYPE_") &&
			s.Type != "CORTEX_STEP_TYPE_SYSTEM_MESSAGE" &&
			s.Type != "CORTEX_STEP_TYPE_ERROR_MESSAGE" &&
			s.Type != "CORTEX_STEP_TYPE_SUGGESTED_RESPONSES" {
			if activeFocusStartedAt == "" {
				activeFocusStartedAt = stepStartTimestamp(s)
			}
		}
	}
	return activePlan, activeFocus, activeFocusStartedAt
}

func stepStartTimestamp(s TrajectoryStep) string {
	if ts := strings.TrimSpace(s.Metadata.CreatedAt); ts != "" {
		return ts
	}
	return strings.TrimSpace(s.Metadata.StartedAt)
}

func hashStepPlanItem(h *fnv64, p *StepPlanItem) {
	if p == nil {
		h.u8(0)
		return
	}
	h.u8(1)
	h.num(p.AnchorStepIndex)
	h.num(p.DoneCount)
	h.num(p.TotalCount)
	h.num(p.SkippedCount)
	h.num(p.FailedCount)
	h.num(len(p.Items))
	for i := range p.Items {
		h.str(p.Items[i].Key)
		h.str(p.Items[i].Title)
		h.str(p.Items[i].Status)
	}
}
