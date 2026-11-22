package proxy

import (
	"compress/flate"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// streamRunningKeepaliveInterval bounds silent periods while RUNNING so proxies/tunnels stay alive across >5 min thinking.
var streamRunningKeepaliveInterval = 25 * time.Second

// StreamUpdatePayload represents a real-time event pushed over WebSocket.
type StreamUpdatePayload struct {
	Type               string               `json:"type"` // "init", "update", "noop", "error"
	Rev                uint64               `json:"rev,omitempty"`
	CascadeID          string               `json:"cascadeId"`
	Title              string               `json:"title,omitempty"`
	Status             string               `json:"status"`
	HasError           bool                 `json:"hasError"`
	ErrorMessage       string               `json:"errorMessage,omitempty"`
	Duration           string               `json:"duration"`
	TotalSteps         int                  `json:"totalSteps"`
	TotalTools         int                  `json:"totalTools"`
	TotalMessages      int                  `json:"totalMessages"`
	HasMore            bool                 `json:"hasMore"`
	NextOffset         int                  `json:"nextOffset"`
	WorkspaceURI       string               `json:"workspaceUri"`
	Steps              []TrajectoryStep     `json:"steps,omitempty"`
	Messages           []CascadeMessageItem `json:"messages,omitempty"`
	TurnOutline        []TurnOutlineItem    `json:"turnOutline,omitempty"`
	QueuedMessages     []QueuedMessageItem  `json:"queuedMessages"`
	RunningTasks       []RunningTaskItem    `json:"runningTasks,omitempty"`
	Subagents          []SubagentItem       `json:"subagents,omitempty"`
	HasRunningSubagents bool                `json:"hasRunningSubagents,omitempty"`
	ParentConversation string               `json:"parentConversationId,omitempty"`
	SubagentRole       string               `json:"subagentRole,omitempty"`
	IsFullSnapshot     bool                 `json:"isFullSnapshot"`
	CascadeConfigRaw   string               `json:"cascadeConfigRaw,omitempty"`
	CanProceed         bool                 `json:"canProceed"`
	ProceedArtifactURI string               `json:"proceedArtifactUri,omitempty"`
	PendingInteraction *PendingInteraction  `json:"pendingInteraction,omitempty"`
	ActiveModel            string               `json:"activeModel,omitempty"`
	ActiveModelName        string               `json:"activeModelName,omitempty"`
	ModelDisplayName       string               `json:"modelDisplayName,omitempty"`
	DesktopActiveModel     string               `json:"desktopActiveModel,omitempty"`
	DesktopActiveModelName string               `json:"desktopActiveModelName,omitempty"`
	StartedAt          string               `json:"startedAt,omitempty"`
	Swarm              *SwarmBrief          `json:"swarm,omitempty"`
	ContextMeter       *ContextMeterItem    `json:"contextMeter,omitempty"`
	SuggestedResponses []string             `json:"suggestedResponses,omitempty"`
	TaskProgress       *TaskProgressItem    `json:"taskProgress,omitempty"`

	// Delta framing, only set for clients that connect with delta=1 (see streamDeltaState).
	Delta      bool     `json:"delta,omitempty"`
	MessageIDs []string `json:"messageIds,omitempty"`
	StepsFrom  *int     `json:"stepsFrom,omitempty"`
}

// Fingerprint computes a fast signature to detect changes and prevent redundant pushes.
func (p *StreamUpdatePayload) Fingerprint() string {
	piKey := "none"
	if p.PendingInteraction != nil {
		piKey = fmt.Sprintf("%s:%d:%s", p.PendingInteraction.Type, p.PendingInteraction.StepIndex, p.PendingInteraction.DefaultOptionID)
	}
	queuedKey := fmt.Sprintf("%d", len(p.QueuedMessages))
	if len(p.QueuedMessages) > 0 {
		var qb strings.Builder
		qb.WriteString(queuedKey)
		for _, qm := range p.QueuedMessages {
			qb.WriteString(";")
			qb.WriteString(qm.ID)
			qb.WriteString(":")
			qb.WriteString(fmt.Sprintf("%d:%d", len(qm.Text), len(qm.Media)))
		}
		queuedKey = qb.String()
	}
	tasksKey := fmt.Sprintf("%d", len(p.RunningTasks))
	if len(p.RunningTasks) > 0 {
		lastTask := p.RunningTasks[len(p.RunningTasks)-1]
		tasksKey = fmt.Sprintf("%d:%s:%d", len(p.RunningTasks), lastTask.ID, lastTask.StepIndex)
	}
	if len(p.Subagents) > 0 {
		var sb strings.Builder
		sb.WriteString(tasksKey)
		for _, sa := range p.Subagents {
			fmt.Fprintf(&sb, ";%s:%s:%d", sa.ConversationID, sa.Status, sa.StepCount)
		}
		tasksKey = sb.String()
	}
	msgsKey := fmt.Sprintf("%d:%d", p.TotalMessages, len(p.TurnOutline))
	if len(p.Messages) > 0 {
		lastMsg := p.Messages[len(p.Messages)-1]
		msgsKey = fmt.Sprintf("%d:%d:%s:%d:%x", p.TotalMessages, len(p.TurnOutline), lastMsg.ID, len(lastMsg.Text), hashStrings(lastMsg.Text))
	}
	swarmKey := "none"
	if p.Swarm != nil && p.Swarm.Active {
		swarmKey = fmt.Sprintf("%s:%d:%d:%d:%d:%s", p.Swarm.CoordinatorID, p.Swarm.WorkingCount, p.Swarm.TotalMembers, p.Swarm.LatestSeq, p.Swarm.AttentionCount, p.Swarm.Phase)
	}
	cmKey := "none"
	if p.ContextMeter != nil {
		cmKey = fmt.Sprintf("%d:%d:%.1f", p.ContextMeter.UsedTokens, p.ContextMeter.OutputTokens, p.ContextMeter.TokensPerSecond)
	}
	sugKey := fmt.Sprintf("%d:%s", len(p.SuggestedResponses), strings.Join(p.SuggestedResponses, "|"))
	tpKey := ""
	if p.TaskProgress != nil {
		tpKey = p.TaskProgress.TaskName + ":" + p.TaskProgress.TaskStatus + ":" + p.TaskProgress.TaskSummary
	}
	if len(p.Steps) == 0 {
		return fmt.Sprintf("%s:%t:%s:%t:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s", p.Status, p.HasError, msgsKey, p.CanProceed, piKey, queuedKey, tasksKey, p.ActiveModel, p.DesktopActiveModel, p.Title, swarmKey, cmKey, sugKey, tpKey)
	}
	last := p.Steps[len(p.Steps)-1]
	lastLen := 0
	if last.PlannerResponse != nil {
		lastLen += len(last.PlannerResponse.Response) + len(last.PlannerResponse.Thinking)
	}
	if last.Content != "" {
		lastLen += len(last.Content)
	}
	if last.ToolCall != nil {
		lastLen += len(last.ToolCall.Name) + len(last.ToolCall.ToolSummary) + len(last.ToolCall.ArgumentsJson)
	}
	if last.ErrorMessage != nil {
		lastLen += len(last.ErrorMessage.Error.ShortError) + len(last.ErrorMessage.Error.UserErrorMessage)
	}
	// Length alone misses same-length rewrites of the tail step, so also fold in a content hash.
	var contentHash uint64
	if last.PlannerResponse != nil {
		contentHash = hashStrings(last.PlannerResponse.Response, last.PlannerResponse.Thinking, last.Content)
	} else {
		contentHash = hashStrings(last.Content)
	}
	return fmt.Sprintf("%s:%t:%d:%d:%s:%s:%d:%s:%t:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%x", p.Status, p.HasError, p.TotalSteps, p.TotalTools, last.Type, last.Status, lastLen, msgsKey, p.CanProceed, piKey, queuedKey, tasksKey, p.ActiveModel, p.DesktopActiveModel, p.Title, swarmKey, cmKey, sugKey, tpKey, contentHash)
}

// rawTrajectorySignature computes an O(1) lightweight fingerprint of the raw upstream response
// to skip expensive ParseTrajectoryDetails and regex scans during 250ms stream polling when unchanged.
func rawTrajectorySignature(raw *upstreamTrajectoryResp) string {
	if raw == nil {
		return ""
	}
	steps := raw.Trajectory.Steps
	n := len(steps)
	title := ""
	if raw.Trajectory.Annotations != nil {
		title = raw.Trajectory.Annotations.Title
	} else {
		title = raw.Trajectory.Summary
	}
	pamCount := len(raw.PendingAgentMessages)
	lastPamID := ""
	lastPamLen := 0
	if pamCount > 0 {
		lastPam := raw.PendingAgentMessages[pamCount-1]
		lastPamID = lastPam.ID
		lastPamLen = len(lastPam.Content)
	}

	if n == 0 {
		return fmt.Sprintf("%s:%d:%s:%d:%s:%d", raw.Status, n, title, pamCount, lastPamID, lastPamLen)
	}

	last := steps[n-1]
	lastContentLen := len(last.Content)
	if last.PlannerResponse != nil {
		lastContentLen += len(last.PlannerResponse.Response) + len(last.PlannerResponse.Thinking)
	}
	if last.ToolCall != nil {
		lastContentLen += len(last.ToolCall.Name) + len(last.ToolCall.ArgumentsJson)
	}
	if last.TaskDetails != nil {
		lastContentLen += len(last.TaskDetails.Description)
	}
	hasPI := (last.RequestedInteraction != nil)

	var contentHash uint64
	if last.PlannerResponse != nil {
		contentHash = hashStrings(last.PlannerResponse.Response, last.PlannerResponse.Thinking, last.Content)
	} else {
		contentHash = hashStrings(last.Content)
	}
	return fmt.Sprintf("%s:%d:%s:%s:%d:%t:%s:%d:%s:%d:%x",
		raw.Status, n, last.Type, last.Status, lastContentLen, hasPI, title, pamCount, lastPamID, lastPamLen, contentHash)
}

// HandleCascadeStream serves a WebSocket connection for continuous real-time trajectory updates.
func (p *Proxy) HandleCascadeStream(w http.ResponseWriter, r *http.Request) {
	cascadeID := r.URL.Query().Get("cascadeId")
	if cascadeID == "" {
		http.Error(w, "missing cascadeId", http.StatusBadRequest)
		return
	}

	format := r.URL.Query().Get("format")
	clientType := r.URL.Query().Get("client")
	ua := r.UserAgent()
	isMessagesOnly := format == "messages" || clientType == "ios" ||
		((strings.Contains(ua, "CFNetwork") || strings.Contains(ua, "Darwin") || strings.Contains(ua, "Antigravity")) && !strings.Contains(ua, "Mozilla"))

	wantDelta := r.URL.Query().Get("delta") == "1"
	sinceRev, _ := strconv.ParseUint(r.URL.Query().Get("sinceRev"), 10, 64)
	// Step-rendering clients (the web app) never read the messages window; messages=0 drops it so
	// every frame (and the large init frame) is not sent twice in two shapes.
	omitMessages := !isMessagesOnly && r.URL.Query().Get("messages") == "0"
	var deltaState streamDeltaState

	sanitizeWebSocketHeaders(r)

	clientConn, err := streamUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("[Stream] WS upgrade failed", "err", err)
		return
	}
	_ = clientConn.SetCompressionLevel(flate.BestSpeed)
	defer clientConn.Close()
	defer p.trackWSConn(clientConn)()

	releaseReactive := p.agentStateHub.Acquire(p, cascadeID)
	defer releaseReactive()

	if verboseRPC {
		slog.Info("[Stream] WS client connected", "cascadeId", cascadeID, "remote", r.RemoteAddr, "client", clientType, "format", format)
		defer slog.Info("[Stream] WS client disconnected", "cascadeId", cascadeID, "remote", r.RemoteAddr)
	}

	var curPort int
	var curToken string
	if p.insp != nil {
		if cur := p.insp.Current(); cur != nil {
			curPort = cur.Port
			curToken = cur.CSRFToken
		}
	}
	streamTitle := p.lookupCascadeTitle(cascadeID, curPort, curToken)
	if streamTitle == "" {
		streamTitle = "当前会话"
	}
	p.SetActiveStream(cascadeID, streamTitle)
	defer p.ClearActiveStream(cascadeID)

	touchCh, cleanupTouch := p.registerStreamTouchListener(cascadeID)
	defer cleanupTouch()

	var writeMu sync.Mutex
	writeJSON := func(v interface{}) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		clientConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return clientConn.WriteJSON(v)
	}

	closeCh := make(chan struct{})
	clientConn.SetReadLimit(64 * 1024)
	// Set initial read deadline and register pong handler to keep connection alive
	clientConn.SetReadDeadline(time.Now().Add(45 * time.Second))
	clientConn.SetPongHandler(func(appData string) error {
		clientConn.SetReadDeadline(time.Now().Add(45 * time.Second))
		return nil
	})
	go func() {
		defer close(closeCh)
		for {
			_, _, err := clientConn.ReadMessage()
			if err != nil {
				return
			}
			// Reset read deadline on any client message
			clientConn.SetReadDeadline(time.Now().Add(45 * time.Second))
		}
	}()

	// P7: do the first fetch synchronously before entering the select loop to
	// eliminate the 50ms latency on initial stream push.
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	pingTicker := time.NewTicker(20 * time.Second)
	defer pingTicker.Stop()

	lastFingerprint := ""
	lastRawSig := ""
	firstPush := true
	lastJSONWriteAt := time.Now()
	var connStreamRev uint64
	resumedFromRev := false
	if wantDelta && sinceRev > 0 {
		if savedDelta, savedFP, ok := p.streamResume.lookup(sinceRev, cascadeID, isMessagesOnly, omitMessages); ok {
			deltaState = savedDelta
			lastFingerprint = savedFP
			connStreamRev = sinceRev
			resumedFromRev = true
		}
	}
	cachedStreamTitle := streamTitle
	lastTitleLookupTime := time.Time{}
	var lastDetails TrajectoryDetails
	var lastSummarySig uint64
	haveSummarySig := false
	var lastFetchAt time.Time
	const streamTouchCoalesceWindow = 65 * time.Millisecond

	emitJSON := func(v interface{}) error {
		lastJSONWriteAt = time.Now()
		return writeJSON(v)
	}

	// fetchAndSend does one poll + send cycle. Returns false if the connection should close.
	fetchAndSend := func() bool {
		lastFetchAt = time.Now()
		p.mu.RLock()
		port := p.activePort
		token := p.activeToken
		p.mu.RUnlock()

		if port == 0 {
			ticker.Reset(1000 * time.Millisecond)
			return true
		}

		maxAge := 340 * time.Millisecond
		if firstPush {
			// Baseline for the idle change detector below, taken before the first full fetch.
			lastSummarySig, haveSummarySig = p.cascadeSummarySignature(cascadeID, port, token)
		} else if lastDetails.Status != "" && lastDetails.Status != "CASCADE_RUN_STATUS_RUNNING" && len(lastDetails.QueuedMessages) == 0 {
			// Idle: the full trajectory is only refetched every 15s, but a run started elsewhere (e.g. on
			// the desktop) must show up promptly. The shared conversation list (1s TTL, also used by list
			// clients and the events hub) is a cheap change detector: refetch as soon as this cascade's
			// entry changes.
			maxAge = 15 * time.Second
			if sig, ok := p.cascadeSummarySignature(cascadeID, port, token); ok {
				if haveSummarySig && sig != lastSummarySig {
					maxAge = 0
				}
				lastSummarySig, haveSummarySig = sig, true
			}
		}
		t1 := time.Now()
		rawResp, err := p.fetchUpstreamTrajectoryWithMaxAge(cascadeID, port, token, maxAge)
		if time.Since(t1) > 100*time.Millisecond {
			slog.Debug("stream perf: slow fetchUpstream", "cascade_id", cascadeID, "took", time.Since(t1))
		}
		desktopModel, desktopModelName := readDesktopSelectedModelCached(time.Second)
		if err != nil {
			if firstPush {
				// Send empty init payload immediately so client never hangs spinning on load
				_ = emitJSON(StreamUpdatePayload{
					Type:                   "init",
					CascadeID:              cascadeID,
					Title:                  streamTitle,
					Status:                 "CASCADE_RUN_STATUS_IDLE",
					TotalSteps:             0,
					Steps:                  []TrajectoryStep{},
					Messages:               []CascadeMessageItem{},
					DesktopActiveModel:     desktopModel,
					DesktopActiveModelName: desktopModelName,
				})
				firstPush = false
			}
			ticker.Reset(1000 * time.Millisecond)
			return true
		}

		rawSig := rawTrajectorySignature(rawResp) + "|" + p.subagentLiveSignature(rawResp) + "|" + GetSwarmSignature(cascadeID) + "|" + desktopModel + "|" + p.liveStreamAuxSignature(cascadeID)
		if !firstPush && rawSig == lastRawSig && (time.Since(lastTitleLookupTime) < 2*time.Second || cachedStreamTitle != "") {
			// Fast path: Upstream trajectory has not changed at all.
			// Keep L7 stream alive across >5 minute silent thinking via lightweight JSON noop every 25s.
			if lastDetails.Status == "CASCADE_RUN_STATUS_RUNNING" && time.Since(lastJSONWriteAt) >= streamRunningKeepaliveInterval {
				if errKeepalive := emitJSON(StreamUpdatePayload{
					Type:      "noop",
					CascadeID: cascadeID,
					Status:    lastDetails.Status,
					Rev:       connStreamRev,
				}); errKeepalive != nil {
					return false
				}
			}
			// Skip full steps scan, regex image extraction, and payload construction.
			if lastDetails.Status == "CASCADE_RUN_STATUS_RUNNING" || len(lastDetails.QueuedMessages) > 0 {
				if p.agentStateHub.IsLive(cascadeID) {
					ticker.Reset(900 * time.Millisecond)
				} else {
					ticker.Reset(350 * time.Millisecond)
				}
			} else {
				ticker.Reset(2000 * time.Millisecond)
			}
			return true
		}
		lastRawSig = rawSig

		t2 := time.Now()
		prevQueuedCount := len(lastDetails.QueuedMessages)
		details := p.ParseTrajectoryDetailsCached(rawResp)
		if time.Since(t2) > 100*time.Millisecond {
			slog.Debug("stream perf: slow ParseTrajectoryDetails", "cascade_id", cascadeID, "took", time.Since(t2))
		}
		details.Subagents = p.enrichSubagents(details.Subagents)
		for _, sa := range details.Subagents {
			if sa.Status == subagentStatusRunning {
				details.Status = "CASCADE_RUN_STATUS_RUNNING"
				details.HasRunningSubagents = true
				break
			}
		}
		details.AllMessages = p.enrichSubagentMessages(details.AllMessages)
		if details.Title != "" && details.Title != "未命名会话" {
			cachedStreamTitle = details.Title
		} else if cachedStreamTitle != "" && cachedStreamTitle != "未命名会话" && cachedStreamTitle != "当前会话" {
			details.Title = cachedStreamTitle
		} else if firstPush || time.Since(lastTitleLookupTime) >= 2*time.Second {
			lastTitleLookupTime = time.Now()
			if t := p.lookupCascadeTitle(cascadeID, port, token); t != "" {
				details.Title = t
				cachedStreamTitle = t
			}
		}
		fetchedLivePending := false
		if firstPush || details.Status == "CASCADE_RUN_STATUS_RUNNING" || len(details.QueuedMessages) > 0 || prevQueuedCount > 0 {
			if qm := p.GetCachedOrFetchPendingMessages(cascadeID, port, token); qm != nil {
				details.QueuedMessages = qm
				fetchedLivePending = true
			}
		}
		if details.QueuedMessages == nil {
			details.QueuedMessages = []QueuedMessageItem{}
		} else if fetchedLivePending && len(details.QueuedMessages) > 0 {
			details.QueuedMessages = p.FilterQueuedMessagesAgainstTrajectory(cascadeID, details.QueuedMessages, rawResp.Trajectory.Steps, details.AllMessages)
		}
		details.ContextMeter = p.fetchUpstreamGeneratorMetadata(cascadeID, port, token)
		lastDetails = details

		if sink := p.NotificationSink(); sink != nil {
			sink.OnTrajectoryUpdate(&details)
		}
		totalMsgs := len(details.AllMessages)
		streamLimit := 15
		streamStart := totalMsgs - streamLimit
		if streamStart < 0 {
			streamStart = 0
		}
		slicedMessages := details.AllMessages[streamStart:totalMsgs]
		hasMore := streamStart > 0

		payload := StreamUpdatePayload{
			Type:                   "update",
			CascadeID:              details.CascadeID,
			Title:                  details.Title,
			Status:                 details.Status,
			HasError:               details.HasError,
			ErrorMessage:           details.ErrorMessage,
			Duration:               details.Duration,
			TotalSteps:             details.TotalSteps,
			TotalTools:             details.TotalTools,
			TotalMessages:          totalMsgs,
			HasMore:                hasMore,
			NextOffset:             streamStart,
			WorkspaceURI:           details.WorkspaceURI,
			Messages:               slicedMessages,
			TurnOutline:            details.TurnOutline,
			QueuedMessages:         details.QueuedMessages,
			RunningTasks:           details.RunningTasks,
			Subagents:              details.Subagents,
			HasRunningSubagents:    details.HasRunningSubagents,
			ParentConversation:     details.ParentConversation,
			SubagentRole:           details.SubagentRole,
			IsFullSnapshot:         !hasMore,
			CascadeConfigRaw:       details.CascadeConfigRaw,
			CanProceed:             details.CanProceed,
			ProceedArtifactURI:     details.ProceedArtifactURI,
			PendingInteraction:     details.PendingInteraction,
			ActiveModel:            details.ActiveModel,
			ActiveModelName:        details.ActiveModelName,
			ModelDisplayName:       details.ModelDisplayName,
			DesktopActiveModel:     desktopModel,
			DesktopActiveModelName: desktopModelName,
			StartedAt:              details.StartedAt,
			Swarm:              GetSwarmBrief(cascadeID),
			ContextMeter:       details.ContextMeter,
			SuggestedResponses: details.SuggestedResponses,
			TaskProgress:       details.TaskProgress,
		}

		if !isMessagesOnly {
			payload.Steps = details.Steps
		}
		if omitMessages {
			payload.Messages = nil
			payload.TurnOutline = nil
		}

		if firstPush && !resumedFromRev {
			payload.Type = "init"
		}

		fp := payload.Fingerprint()
		if firstPush && resumedFromRev && fp == lastFingerprint {
			firstPush = false
			resumedFromRev = false
			if errWrite := emitJSON(StreamUpdatePayload{
				Type:      "noop",
				CascadeID: cascadeID,
				Status:    payload.Status,
				Rev:       connStreamRev,
			}); errWrite != nil {
				return false
			}
		} else if firstPush || fp != lastFingerprint {
			lastFingerprint = fp
			if wantDelta {
				deltaState.apply(&payload)
				connStreamRev = p.streamResume.save(connStreamRev, cascadeID, isMessagesOnly, omitMessages, fp, &deltaState)
				payload.Rev = connStreamRev
			}
			firstPush = false
			resumedFromRev = false
			t3 := time.Now()
			errWrite := emitJSON(payload)
			if time.Since(t3) > 100*time.Millisecond {
				slog.Debug("stream perf: slow writeJSON", "cascade_id", cascadeID, "took", time.Since(t3))
			}
			if errWrite != nil {
				return false
			}
		}

		// Adjust poll interval dynamically: reactive Connect stream wakes touchCh in <2ms, so relax fallback ticker when live.
		if details.Status == "CASCADE_RUN_STATUS_RUNNING" || len(details.QueuedMessages) > 0 {
			if p.agentStateHub.IsLive(cascadeID) {
				ticker.Reset(900 * time.Millisecond)
			} else {
				ticker.Reset(350 * time.Millisecond)
			}
		} else {
			ticker.Reset(2000 * time.Millisecond)
		}
		return true
	}

	// Immediate first push — no ticker wait needed.
	if !fetchAndSend() {
		return
	}

	for {
		select {
		case <-closeCh:
			return
		case <-touchCh:
			// Coalesce bursty reactive delta wakeups within a 65ms window while preserving lastRawSig
			// so unchanged frames hit the fast-path short-circuit cleanly.
			wait := streamTouchCoalesceWindow - time.Since(lastFetchAt)
			if wait < 10*time.Millisecond {
				wait = 10 * time.Millisecond
			}
			ticker.Reset(wait)
		case <-pingTicker.C:
			writeMu.Lock()
			clientConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_ = clientConn.WriteMessage(websocket.PingMessage, []byte{})
			writeMu.Unlock()
		case <-ticker.C:
			if !fetchAndSend() {
				return
			}
		}
	}
}
