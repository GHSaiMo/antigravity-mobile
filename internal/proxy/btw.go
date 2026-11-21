package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	if _, exists := t.bySide[entry.CascadeID]; !exists && len(t.bySide) >= maxBtwTrackedSideQuestions {
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

// isSideQuestionTrajectoryMap returns true when a GetAllCascadeTrajectories entry is a /btw side question
// so it does not pollute the main conversation list or get misclassified as a tool subagent.
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
//   - GET  /gateway/btw/list?cascadeId=...
//   - GET  /gateway/btw/status?cascadeId=...
func (p *Proxy) HandleBtw(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/gateway/btw/start":
		p.handleBtwStart(w, r)
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
	startPayload := map[string]interface{}{
		"source":         "CORTEX_TRAJECTORY_SOURCE_CASCADE_CLIENT",
		"trajectoryType": "CORTEX_TRAJECTORY_TYPE_SIDE_QUESTION",
	}
	if parentID != "" {
		startPayload["baseTrajectoryIdentifier"] = map[string]interface{}{"cascadeId": parentID}
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

	port, token := p.ActiveUpstream()
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
		if cfgBytes, err := json.Marshal(cfgObj); err == nil {
			SetCascadeModel(sideID, canonicalName, cfgBytes)
		}
	}
	if cfgObj != nil {
		msgMap["cascadeConfig"] = cfgObj
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
	}
	defaultBtwTracker.record(entry)

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

	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"ok":              true,
		"cascadeId":       sideID,
		"parentCascadeId": parentID,
		"title":           title,
		"question":        entry.Question,
		"quote":           entry.Quote,
		"createdAt":       entry.CreatedAt,
	})
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

	items := defaultBtwTracker.listForParent(parentID)
	seen := make(map[string]int, len(items))
	for i, it := range items {
		seen[it.CascadeID] = i
	}

	port, token := p.ActiveUpstream()
	if port > 0 {
		if raw, err := p.fetchAllTrajectoriesRaw(r.Context(), port, token); err == nil {
			var parsed struct {
				Summaries map[string]struct {
					Status      string `json:"status"`
					Summary     string `json:"summary"`
					CreatedTime string `json:"createdTime"`
					Annotations struct {
						SideQuestionCascadeIDs []string          `json:"sideQuestionCascadeIds"`
						SideQuestionTitles     map[string]string `json:"sideQuestionTitles"`
					} `json:"annotations"`
				} `json:"trajectorySummaries"`
			}
			if json.Unmarshal(raw, &parsed) == nil {
				if parentSum, ok := parsed.Summaries[parentID]; ok {
					for _, sid := range parentSum.Annotations.SideQuestionCascadeIDs {
						title := parentSum.Annotations.SideQuestionTitles[sid]
						sideSum, hasSum := parsed.Summaries[sid]
						if idx, exists := seen[sid]; exists {
							if hasSum && sideSum.Status != "" {
								items[idx].Status = sideSum.Status
							}
							if items[idx].Title == "" && title != "" {
								items[idx].Title = title
							}
						} else {
							e := BtwSideQuestionEntry{
								CascadeID:       sid,
								ParentCascadeID: parentID,
								Title:           firstNonEmpty(title, sideSum.Summary, "Side question"),
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
					if sideSum, ok := parsed.Summaries[items[i].CascadeID]; ok && sideSum.Status != "" {
						items[i].Status = sideSum.Status
					}
				}
			}
		}
	}

	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"parentCascadeId": parentID,
		"items":           items,
	})
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

	var question, thinking, answer string
	for _, msg := range details.AllMessages {
		switch msg.Type {
		case "user":
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
			if msg.Content != "" {
				if answer != "" {
					answer += "\n\n"
				}
				answer += strings.TrimSpace(msg.Content)
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
	if entry.Question != "" {
		question = entry.Question
	}
	isRunning := details.Status == "CASCADE_RUN_STATUS_RUNNING"
	title := firstNonEmpty(entry.Title, details.Title, question)
	insertText := formatBtwInsertText(sideID, title, question, answer)

	writeJSONStatus(w, http.StatusOK, map[string]interface{}{
		"cascadeId":       sideID,
		"parentCascadeId": entry.ParentCascadeID,
		"status":          details.Status,
		"isRunning":       isRunning,
		"hasError":        details.HasError,
		"errorMessage":    details.ErrorMessage,
		"title":           title,
		"question":        question,
		"quote":           entry.Quote,
		"thinking":        thinking,
		"answer":          answer,
		"insertText":      insertText,
		"totalSteps":      details.TotalSteps,
	})
}
