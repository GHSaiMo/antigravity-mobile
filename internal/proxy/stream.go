package proxy

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// StreamUpdatePayload represents a real-time event pushed over WebSocket.
type StreamUpdatePayload struct {
	Type               string               `json:"type"` // "init", "update", "error"
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
	Steps              []TrajectoryStep     `json:"steps"`
	Messages           []CascadeMessageItem `json:"messages"`
	QueuedMessages     []QueuedMessageItem  `json:"queuedMessages"`
	RunningTasks       []RunningTaskItem    `json:"runningTasks,omitempty"`
	IsFullSnapshot     bool                 `json:"isFullSnapshot"`
	CascadeConfigRaw   string               `json:"cascadeConfigRaw,omitempty"`
	CanProceed         bool                 `json:"canProceed"`
	ProceedArtifactURI string               `json:"proceedArtifactUri,omitempty"`
	PendingInteraction *PendingInteraction  `json:"pendingInteraction,omitempty"`
	ActiveModel        string               `json:"activeModel,omitempty"`
	ModelDisplayName   string               `json:"modelDisplayName,omitempty"`
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
			qb.WriteString(fmt.Sprintf("%d", len(qm.Text)))
		}
		queuedKey = qb.String()
	}
	tasksKey := fmt.Sprintf("%d", len(p.RunningTasks))
	if len(p.RunningTasks) > 0 {
		lastTask := p.RunningTasks[len(p.RunningTasks)-1]
		tasksKey = fmt.Sprintf("%d:%s:%d", len(p.RunningTasks), lastTask.ID, lastTask.StepIndex)
	}
	if len(p.Steps) == 0 {
		return fmt.Sprintf("%s:%t:0:0:%t:%s:%s:%s:%s", p.Status, p.HasError, p.CanProceed, piKey, queuedKey, tasksKey, p.ActiveModel)
	}
	last := p.Steps[len(p.Steps)-1]
	lastLen := 0
	if last.PlannerResponse != nil {
		lastLen = len(last.PlannerResponse.Response) + len(last.PlannerResponse.Thinking)
	}
	return fmt.Sprintf("%s:%t:%d:%d:%s:%s:%d:%t:%s:%s:%s:%s", p.Status, p.HasError, p.TotalSteps, p.TotalTools, last.Type, last.Status, lastLen, p.CanProceed, piKey, queuedKey, tasksKey, p.ActiveModel)
}

// HandleCascadeStream serves a WebSocket connection for continuous real-time trajectory updates.
func (p *Proxy) HandleCascadeStream(w http.ResponseWriter, r *http.Request) {
	cascadeID := r.URL.Query().Get("cascadeId")
	if cascadeID == "" {
		http.Error(w, "missing cascadeId", http.StatusBadRequest)
		return
	}

	sanitizeWebSocketHeaders(r)

	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Stream] WS upgrade failed: %v", err)
		return
	}
	defer clientConn.Close()

	var writeMu sync.Mutex
	writeJSON := func(v interface{}) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		clientConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return clientConn.WriteJSON(v)
	}

	closeCh := make(chan struct{})
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

	ticker := time.NewTicker(50 * time.Millisecond) // immediate first check
	defer ticker.Stop()

	pingTicker := time.NewTicker(20 * time.Second)
	defer pingTicker.Stop()

	lastFingerprint := ""
	firstPush := true

	for {
		select {
		case <-closeCh:
			return
		case <-pingTicker.C:
			writeMu.Lock()
			clientConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			_ = clientConn.WriteMessage(websocket.PingMessage, []byte{})
			writeMu.Unlock()
		case <-ticker.C:
			p.mu.RLock()
			port := p.activePort
			token := p.activeToken
			p.mu.RUnlock()

			if port == 0 {
				ticker.Reset(1000 * time.Millisecond)
				continue
			}

			maxAge := 200 * time.Millisecond
			rawResp, err := p.fetchUpstreamTrajectoryWithMaxAge(cascadeID, port, token, maxAge)
			if err != nil {
				ticker.Reset(1000 * time.Millisecond)
				continue
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
				Type:               "update",
				CascadeID:          details.CascadeID,
				Title:              details.Title,
				Status:             details.Status,
				HasError:           details.HasError,
				ErrorMessage:       details.ErrorMessage,
				Duration:           details.Duration,
				TotalSteps:         details.TotalSteps,
				TotalTools:         details.TotalTools,
				TotalMessages:      totalMsgs,
				HasMore:            hasMore,
				NextOffset:         streamStart,
				WorkspaceURI:       details.WorkspaceURI,
				Steps:              details.Steps,
				Messages:           slicedMessages,
				QueuedMessages:     details.QueuedMessages,
				RunningTasks:       details.RunningTasks,
				IsFullSnapshot:     !hasMore,
				CascadeConfigRaw:   details.CascadeConfigRaw,
				CanProceed:         details.CanProceed,
				ProceedArtifactURI: details.ProceedArtifactURI,
				PendingInteraction: details.PendingInteraction,
				ActiveModel:        details.ActiveModel,
				ModelDisplayName:   details.ModelDisplayName,
			}

			if firstPush {
				payload.Type = "init"
			}

			fp := payload.Fingerprint()
			if firstPush || fp != lastFingerprint {
				lastFingerprint = fp
				firstPush = false
				if err := writeJSON(payload); err != nil {
					return
				}
			}

			// Adjust poll interval dynamically: fast when executing or queued messages exist, slower when idle
			if details.Status == "CASCADE_RUN_STATUS_RUNNING" || len(details.QueuedMessages) > 0 {
				ticker.Reset(250 * time.Millisecond)
			} else {
				ticker.Reset(1200 * time.Millisecond)
			}
		}
	}
}
