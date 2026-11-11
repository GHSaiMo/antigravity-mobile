package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// SwarmBrief is a lightweight snapshot of active swarm state embedded in StreamUpdatePayload.
type SwarmBrief struct {
	Active         bool   `json:"active"`
	CoordinatorID  string `json:"coordinatorId"`
	Title          string `json:"title"`
	Phase          string `json:"phase,omitempty"`
	Headline       string `json:"headline,omitempty"`
	WorkingCount   int    `json:"workingCount"`
	TotalMembers   int    `json:"totalMembers"`
	LatestSeq      int    `json:"latestSeq"`
	AttentionCount int    `json:"attentionCount"`
}

// SwarmMemberInfo represents a member in members.json enriched with runtime activity.
type SwarmMemberInfo struct {
	Name          string `json:"name"`
	ConversationID string `json:"conversationId"`
	Model         string `json:"model"`
	Role          string `json:"role"`
	WakeMode      string `json:"wakeMode"`
	WaitSince     string `json:"waitSince,omitempty"`
	LastReadSeq   int    `json:"lastReadSeq"`
	LastNudgedSeq int    `json:"lastNudgedSeq"`
	Killed        bool   `json:"killed"`
	Status        string `json:"status"` // "working", "waiting", "killed", "error", "stuck"
	LastActivity  string `json:"lastActivity,omitempty"`
	UnreadCount   int    `json:"unreadCount"`
	IsCurrent     bool   `json:"isCurrent"`
}

// SwarmBoardMessage is one line from board.jsonl.
type SwarmBoardMessage struct {
	Seq      int      `json:"seq"`
	Agent    string   `json:"agent"`
	Text     string   `json:"text"`
	Time     string   `json:"time"`
	Channel  string   `json:"channel,omitempty"`
	Mentions []string `json:"mentions,omitempty"`
}

// SwarmViewAttention represents an actionable item in view.json.
type SwarmViewAttention struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`     // decision, failure, review, stuck, quota, error, fyi
	Severity string `json:"severity"` // high, med, low
	Title    string `json:"title"`
	Why      string `json:"why,omitempty"`
}

// SwarmViewWorkstream represents a stream of work in view.json.
type SwarmViewWorkstream struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Owners []string `json:"owners,omitempty"`
	State  string   `json:"state"` // queued, active, blocked, review, done, failed
	Note   string   `json:"note,omitempty"`
}

// SwarmView mirrors the structured summary in view.json written by the swarm narrator.
type SwarmView struct {
	Version   string `json:"version,omitempty"`
	AsOfSeq   int    `json:"as_of_seq,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Headline  string `json:"headline,omitempty"`
	Phase     *struct {
		Label string `json:"label"`
		Index int    `json:"index"`
		Total int    `json:"total"`
	} `json:"phase,omitempty"`
	Attention   []SwarmViewAttention   `json:"attention,omitempty"`
	Workstreams []SwarmViewWorkstream  `json:"workstreams,omitempty"`
	Milestones  []struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"milestones,omitempty"`
	Risks []struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"risks,omitempty"`
}

// SwarmSnapshot is returned by GET /gateway/swarm.
type SwarmSnapshot struct {
	CascadeID     string              `json:"cascadeId"`
	CoordinatorID string              `json:"coordinatorId"`
	Title         string              `json:"title"`
	ETag          string              `json:"etag"`
	Brief         SwarmBrief          `json:"brief"`
	Members       []SwarmMemberInfo   `json:"members"`
	Messages      []SwarmBoardMessage `json:"messages"`
	View          *SwarmView          `json:"view,omitempty"`
}

var (
	mentionRegex = regexp.MustCompile(`@([a-zA-Z0-9_\-]+)`)
	swarmCoordMu sync.RWMutex
	// memberToCoordCache maps member conversationId -> coordinator conversationId
	memberToCoordCache = make(map[string]string)
)

// resolveSwarmDir locates the .system_generated/swarm directory for a conversation ID.
// If the conversation is itself the coordinator, returns its own swarm dir.
// If it is a worker member, finds the coordinator owning this member.
func resolveSwarmDir(cascadeID string) (string, string) {
	if cascadeID == "" {
		return "", ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ""
	}
	brainBase := filepath.Join(home, ".gemini", "antigravity", "brain")

	// 1. Direct check: is cascadeID the coordinator?
	directDir := filepath.Join(brainBase, cascadeID, ".system_generated", "swarm")
	if fi, err := os.Stat(filepath.Join(directDir, "members.json")); err == nil && !fi.IsDir() {
		return directDir, cascadeID
	}

	// 2. Cache check
	swarmCoordMu.RLock()
	cachedCoord, ok := memberToCoordCache[cascadeID]
	swarmCoordMu.RUnlock()
	if ok && cachedCoord != "" {
		cachedDir := filepath.Join(brainBase, cachedCoord, ".system_generated", "swarm")
		if fi, err := os.Stat(filepath.Join(cachedDir, "members.json")); err == nil && !fi.IsDir() {
			return cachedDir, cachedCoord
		}
	}

	// 3. Scan recent brain dirs for members.json
	entries, err := os.ReadDir(brainBase)
	if err != nil {
		return "", ""
	}
	// Limit to the most recent 15 directories
	for i := len(entries) - 1; i >= 0 && i >= len(entries)-25; i-- {
		e := entries[i]
		if !e.IsDir() || e.Name() == cascadeID {
			continue
		}
		candCoord := e.Name()
		candDir := filepath.Join(brainBase, candCoord, ".system_generated", "swarm")
		membersPath := filepath.Join(candDir, "members.json")
		data, err := os.ReadFile(membersPath)
		if err != nil {
			continue
		}
		var raw struct {
			Members map[string]struct {
				ConversationID string `json:"conversation_id"`
			} `json:"members"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			continue
		}
		for _, m := range raw.Members {
			if m.ConversationID != "" {
				swarmCoordMu.Lock()
				memberToCoordCache[m.ConversationID] = candCoord
				swarmCoordMu.Unlock()
			}
			if m.ConversationID == cascadeID {
				return candDir, candCoord
			}
		}
	}

	return "", ""
}

// GetSwarmBrief extracts a lightweight brief of the swarm for streaming.
func GetSwarmBrief(cascadeID string) *SwarmBrief {
	swarmDir, coordID := resolveSwarmDir(cascadeID)
	if swarmDir == "" {
		return nil
	}

	membersData, err := os.ReadFile(filepath.Join(swarmDir, "members.json"))
	if err != nil {
		return nil
	}

	var rawMembers struct {
		Title   string `json:"title"`
		Members map[string]struct {
			Killed bool   `json:"killed"`
			Role   string `json:"role"`
		} `json:"members"`
	}
	if err := json.Unmarshal(membersData, &rawMembers); err != nil {
		return nil
	}

	total := len(rawMembers.Members)
	working := 0
	for _, m := range rawMembers.Members {
		if !m.Killed {
			working++
		}
	}

	brief := &SwarmBrief{
		Active:        true,
		CoordinatorID: coordID,
		Title:         rawMembers.Title,
		TotalMembers:  total,
		WorkingCount:  working,
	}
	if brief.Title == "" {
		brief.Title = "蜂群协作"
	}

	// Read view.json for headline/phase/attention count
	if viewData, err := os.ReadFile(filepath.Join(swarmDir, "view.json")); err == nil && len(viewData) > 0 {
		var v SwarmView
		if err := json.Unmarshal(viewData, &v); err == nil {
			brief.Headline = v.Headline
			if v.Phase != nil {
				brief.Phase = v.Phase.Label
			}
			brief.AttentionCount = len(v.Attention)
		}
	}

	// Read tail seq of board.jsonl
	if boardFile, err := os.Open(filepath.Join(swarmDir, "board.jsonl")); err == nil {
		defer boardFile.Close()
		scanner := bufio.NewScanner(boardFile)
		latestSeq := 0
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var m struct {
				Seq int `json:"seq"`
			}
			if err := json.Unmarshal(line, &m); err == nil && m.Seq > latestSeq {
				latestSeq = m.Seq
			}
		}
		brief.LatestSeq = latestSeq
	}

	return brief
}

// GetSwarmSnapshot returns full details of the swarm, checking ETag.
func GetSwarmSnapshot(cascadeID string, afterSeq int, clientETag string) (*SwarmSnapshot, bool, error) {
	swarmDir, coordID := resolveSwarmDir(cascadeID)
	if swarmDir == "" {
		return nil, false, os.ErrNotExist
	}

	membersPath := filepath.Join(swarmDir, "members.json")
	boardPath := filepath.Join(swarmDir, "board.jsonl")
	viewPath := filepath.Join(swarmDir, "view.json")

	fiMembers, err := os.Stat(membersPath)
	if err != nil {
		return nil, false, err
	}
	var boardMod int64
	var boardSize int64
	if fi, err := os.Stat(boardPath); err == nil {
		boardMod = fi.ModTime().UnixNano()
		boardSize = fi.Size()
	}
	var viewMod int64
	if fi, err := os.Stat(viewPath); err == nil {
		viewMod = fi.ModTime().UnixNano()
	}

	currentETag := fmt.Sprintf(`"%d-%d-%d-%d-%d"`, fiMembers.ModTime().UnixNano(), fiMembers.Size(), boardMod, boardSize, viewMod)
	if clientETag != "" && clientETag == currentETag {
		return nil, true, nil
	}

	membersData, err := os.ReadFile(membersPath)
	if err != nil {
		return nil, false, err
	}
	var rawMembers struct {
		Title   string `json:"title"`
		Members map[string]struct {
			ConversationID string `json:"conversation_id"`
			Model          string `json:"model"`
			Role           string `json:"role"`
			WakeMode       string `json:"wake_mode"`
			WaitSince      string `json:"wait_since"`
			LastReadSeq    int    `json:"last_read_seq"`
			LastNudgedSeq  int    `json:"last_nudged_seq"`
			Killed         bool   `json:"killed"`
		} `json:"members"`
	}
	if err := json.Unmarshal(membersData, &rawMembers); err != nil {
		return nil, false, err
	}

	// Read board.jsonl
	messages := make([]SwarmBoardMessage, 0)
	latestSeq := 0
	if boardFile, err := os.Open(boardPath); err == nil {
		defer boardFile.Close()
		scanner := bufio.NewScanner(boardFile)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var msg SwarmBoardMessage
			if err := json.Unmarshal(line, &msg); err == nil {
				if msg.Seq > latestSeq {
					latestSeq = msg.Seq
				}
				if msg.Seq > afterSeq {
					// extract mentions
					found := mentionRegex.FindAllStringSubmatch(msg.Text, -1)
					mentions := make([]string, 0, len(found))
					for _, m := range found {
						if len(m) > 1 {
							mentions = append(mentions, m[1])
						}
					}
					msg.Mentions = mentions
					messages = append(messages, msg)
				}
			}
		}
	}

	home, _ := os.UserHomeDir()
	brainBase := filepath.Join(home, ".gemini", "antigravity", "brain")

	members := make([]SwarmMemberInfo, 0, len(rawMembers.Members))
	workingCount := 0
	for name, rm := range rawMembers.Members {
		status := "waiting"
		if rm.Killed {
			status = "killed"
		} else {
			workingCount++
			status = "working"
		}

		lastActivity := ""
		// Inspect worker's transcript.jsonl tail if available
		if rm.ConversationID != "" {
			tPath := filepath.Join(brainBase, rm.ConversationID, ".system_generated", "logs", "transcript.jsonl")
			if tFile, err := os.Open(tPath); err == nil {
				defer tFile.Close()
				// Read last non-empty line
				scanner := bufio.NewScanner(tFile)
				var lastLine []byte
				for scanner.Scan() {
					b := scanner.Bytes()
					if len(b) > 0 {
						lastLine = append(lastLine[:0], b...)
					}
				}
				if len(lastLine) > 0 {
					var step struct {
						Type            string `json:"type"`
						Status          string `json:"status"`
						ToolCall        *struct {
							Name        string `json:"name"`
							ToolSummary string `json:"toolSummary"`
						} `json:"toolCall"`
						PlannerResponse *struct {
							Response string `json:"response"`
						} `json:"plannerResponse"`
						ErrorMessage    *struct {
							Error struct {
								ShortError string `json:"shortError"`
							} `json:"error"`
						} `json:"errorMessage"`
					}
					if err := json.Unmarshal(lastLine, &step); err == nil {
						if step.ErrorMessage != nil && step.ErrorMessage.Error.ShortError != "" {
							status = "error"
							lastActivity = step.ErrorMessage.Error.ShortError
						} else if step.ToolCall != nil {
							if step.ToolCall.ToolSummary != "" {
								lastActivity = step.ToolCall.ToolSummary
							} else {
								lastActivity = "调用工具 " + step.ToolCall.Name
							}
						} else if step.PlannerResponse != nil && step.PlannerResponse.Response != "" {
							lastActivity = "回复中..."
						}
					}
				}
			}
		}

		unread := latestSeq - rm.LastReadSeq
		if unread < 0 {
			unread = 0
		}

		members = append(members, SwarmMemberInfo{
			Name:           name,
			ConversationID: rm.ConversationID,
			Model:          rm.Model,
			Role:           rm.Role,
			WakeMode:       rm.WakeMode,
			WaitSince:      rm.WaitSince,
			LastReadSeq:    rm.LastReadSeq,
			LastNudgedSeq:  rm.LastNudgedSeq,
			Killed:         rm.Killed,
			Status:         status,
			LastActivity:   lastActivity,
			UnreadCount:    unread,
			IsCurrent:      rm.ConversationID == cascadeID,
		})
	}

	var view *SwarmView
	if viewData, err := os.ReadFile(viewPath); err == nil && len(viewData) > 0 {
		var v SwarmView
		if err := json.Unmarshal(viewData, &v); err == nil {
			view = &v
		}
	}

	title := rawMembers.Title
	if title == "" {
		title = "蜂群协作"
	}

	brief := SwarmBrief{
		Active:        true,
		CoordinatorID: coordID,
		Title:         title,
		WorkingCount:  workingCount,
		TotalMembers:  len(members),
		LatestSeq:     latestSeq,
	}
	if view != nil {
		brief.Headline = view.Headline
		if view.Phase != nil {
			brief.Phase = view.Phase.Label
		}
		brief.AttentionCount = len(view.Attention)
	}

	snap := &SwarmSnapshot{
		CascadeID:     cascadeID,
		CoordinatorID: coordID,
		Title:         title,
		ETag:          currentETag,
		Brief:         brief,
		Members:       members,
		Messages:      messages,
		View:          view,
	}

	return snap, false, nil
}

// HandleGetSwarm serves GET /gateway/swarm.
func (p *Proxy) HandleGetSwarm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	cascadeID := r.URL.Query().Get("cascadeId")
	if cascadeID == "" {
		cascadeID = r.URL.Query().Get("cascade_id")
	}
	if cascadeID == "" {
		writeJSONError(w, "missing cascadeId", http.StatusBadRequest)
		return
	}

	afterSeq := 0
	if s := r.URL.Query().Get("afterSeq"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			afterSeq = n
		}
	} else if s := r.URL.Query().Get("after_seq"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			afterSeq = n
		}
	}
	clientETag := r.Header.Get("If-None-Match")

	snap, notModified, err := GetSwarmSnapshot(cascadeID, afterSeq, clientETag)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, "no active swarm found for this conversation", http.StatusNotFound)
			return
		}
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if notModified {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", snap.ETag)
	_ = json.NewEncoder(w).Encode(snap)
}

// HandleSwarmFeedback serves POST /gateway/swarm/feedback.
func (p *Proxy) HandleSwarmFeedback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		CascadeID     string `json:"cascadeId"`
		CascadeIDAlt  string `json:"cascade_id"`
		CoordinatorID string `json:"coordinatorId"`
		Feedback      string `json:"feedback"`
		Content       string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.CascadeID == "" {
		req.CascadeID = req.CascadeIDAlt
	}
	feedback := strings.TrimSpace(req.Feedback)
	if feedback == "" {
		feedback = strings.TrimSpace(req.Content)
	}
	if feedback == "" {
		writeJSONError(w, "feedback text cannot be empty", http.StatusBadRequest)
		return
	}

	targetConv := req.CoordinatorID
	if targetConv == "" {
		_, coord := resolveSwarmDir(req.CascadeID)
		targetConv = coord
	}
	if targetConv == "" {
		targetConv = req.CascadeID
	}

	msg := fmt.Sprintf("@narrator user feedback: %s", feedback)
	if err := p.sendCascadeTextMessage(targetConv, msg); err != nil {
		slog.Warn("[Swarm] Failed to send feedback to coordinator", "conv", targetConv, "err", err)
		writeJSONError(w, "failed to dispatch feedback: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "dispatchedTo": targetConv})
}

// HandleSwarmNudge serves POST /gateway/swarm/nudge.
func (p *Proxy) HandleSwarmNudge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		CascadeID     string `json:"cascadeId"`
		CascadeIDAlt  string `json:"cascade_id"`
		CoordinatorID string `json:"coordinatorId"`
		WorkerName    string `json:"workerName"`
		Member        string `json:"member"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.CascadeID == "" {
		req.CascadeID = req.CascadeIDAlt
	}
	worker := strings.TrimSpace(req.WorkerName)
	if worker == "" {
		worker = strings.TrimSpace(req.Member)
	}
	if worker == "" {
		writeJSONError(w, "workerName cannot be empty", http.StatusBadRequest)
		return
	}

	targetConv := req.CoordinatorID
	if targetConv == "" {
		_, coord := resolveSwarmDir(req.CascadeID)
		targetConv = coord
	}
	if targetConv == "" {
		targetConv = req.CascadeID
	}

	msg := fmt.Sprintf("@%s 请汇报当前进展与卡点", worker)
	if err := p.sendCascadeTextMessage(targetConv, msg); err != nil {
		slog.Warn("[Swarm] Failed to send nudge to worker", "worker", worker, "conv", targetConv, "err", err)
		writeJSONError(w, "failed to dispatch nudge: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "worker": worker, "dispatchedTo": targetConv})
}

// sendCascadeTextMessage sends a single text prompt to the upstream language_server for a cascade.
func (p *Proxy) sendCascadeTextMessage(cascadeID, text string) error {
	port, token := p.ActiveUpstream()
	if cur := p.insp.Current(); cur != nil && cur.IsHealthy && cur.Port > 0 {
		port = cur.Port
		token = cur.CSRFToken
	}
	if port <= 0 {
		return fmt.Errorf("upstream language_server unavailable")
	}

	msgPayload := map[string]interface{}{
		"cascadeId": cascadeID,
		"items": []map[string]interface{}{
			{"text": text},
		},
	}
	msgBytes, err := json.Marshal(msgPayload)
	if err != nil {
		return err
	}

	msgURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", port)
	msgReq, err := http.NewRequest(http.MethodPost, msgURL, strings.NewReader(string(msgBytes)))
	if err != nil {
		return err
	}
	msgReq.Header.Set("Content-Type", "application/json")
	msgReq.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		msgReq.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.mediumClient.Do(msgReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	ClearTrajectoryCache(cascadeID)
	p.notifyStreamTouch(cascadeID)
	return nil
}
