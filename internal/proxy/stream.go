package proxy

import (
	"fmt"
	"log"
	"net/http"
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
	Duration           string               `json:"duration"`
	TotalSteps         int                  `json:"totalSteps"`
	TotalTools         int                  `json:"totalTools"`
	WorkspaceURI       string               `json:"workspaceUri"`
	Steps              []TrajectoryStep     `json:"steps"`
	Messages           []CascadeMessageItem `json:"messages"`
	IsFullSnapshot     bool                 `json:"isFullSnapshot"`
	CascadeConfigRaw   string               `json:"cascadeConfigRaw,omitempty"`
	CanProceed         bool                 `json:"canProceed"`
	ProceedArtifactURI string               `json:"proceedArtifactUri,omitempty"`
	PendingInteraction *PendingInteraction  `json:"pendingInteraction,omitempty"`
}

// Fingerprint computes a fast signature to detect changes and prevent redundant pushes.
func (p *StreamUpdatePayload) Fingerprint() string {
	piKey := "none"
	if p.PendingInteraction != nil {
		piKey = fmt.Sprintf("%s:%d:%s", p.PendingInteraction.Type, p.PendingInteraction.StepIndex, p.PendingInteraction.DefaultOptionID)
	}
	if len(p.Steps) == 0 {
		return fmt.Sprintf("%s:0:0:%t:%s", p.Status, p.CanProceed, piKey)
	}
	last := p.Steps[len(p.Steps)-1]
	lastLen := 0
	if last.PlannerResponse != nil {
		lastLen = len(last.PlannerResponse.Response) + len(last.PlannerResponse.Thinking)
	}
	return fmt.Sprintf("%s:%d:%d:%s:%s:%d:%t:%s", p.Status, p.TotalSteps, p.TotalTools, last.Type, last.Status, lastLen, p.CanProceed, piKey)
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

			if sink := p.NotificationSink(); sink != nil {
				sink.OnTrajectoryUpdate(&details)
			}
			payload := StreamUpdatePayload{
				Type:               "update",
				CascadeID:          details.CascadeID,
				Title:              details.Title,
				Status:             details.Status,
				Duration:           details.Duration,
				TotalSteps:         details.TotalSteps,
				TotalTools:         details.TotalTools,
				WorkspaceURI:       details.WorkspaceURI,
				Steps:              details.Steps,
				Messages:           details.AllMessages,
				IsFullSnapshot:     true,
				CascadeConfigRaw:   details.CascadeConfigRaw,
				CanProceed:         details.CanProceed,
				ProceedArtifactURI: details.ProceedArtifactURI,
				PendingInteraction: details.PendingInteraction,
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

			// Adjust poll interval dynamically: fast when executing, slower when idle
			if details.Status == "CASCADE_RUN_STATUS_RUNNING" {
				ticker.Reset(250 * time.Millisecond)
			} else {
				ticker.Reset(1200 * time.Millisecond)
			}
		}
	}
}
