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
	Type             string               `json:"type"` // "init", "update", "error"
	CascadeID        string               `json:"cascadeId"`
	Status           string               `json:"status"`
	Duration         string               `json:"duration"`
	TotalSteps       int                  `json:"totalSteps"`
	TotalTools       int                  `json:"totalTools"`
	WorkspaceURI     string               `json:"workspaceUri"`
	Steps            []TrajectoryStep     `json:"steps"`
	Messages         []CascadeMessageItem `json:"messages"`
	CascadeConfigRaw string               `json:"cascadeConfigRaw,omitempty"`
}

// Fingerprint computes a fast signature to detect changes and prevent redundant pushes.
func (p *StreamUpdatePayload) Fingerprint() string {
	if len(p.Steps) == 0 {
		return fmt.Sprintf("%s:0:0", p.Status)
	}
	last := p.Steps[len(p.Steps)-1]
	lastLen := 0
	if last.PlannerResponse != nil {
		lastLen = len(last.PlannerResponse.Response) + len(last.PlannerResponse.Thinking)
	}
	return fmt.Sprintf("%s:%d:%d:%s:%s:%d", p.Status, p.TotalSteps, p.TotalTools, last.Type, last.Status, lastLen)
}

// HandleCascadeStream serves a WebSocket connection for continuous real-time trajectory updates.
func (p *Proxy) HandleCascadeStream(w http.ResponseWriter, r *http.Request) {
	cascadeID := r.URL.Query().Get("cascadeId")
	if cascadeID == "" {
		http.Error(w, "missing cascadeId", http.StatusBadRequest)
		return
	}

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
	go func() {
		defer close(closeCh)
		for {
			_, _, err := clientConn.ReadMessage()
			if err != nil {
				return
			}
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
			payload := StreamUpdatePayload{
				Type:             "update",
				CascadeID:        details.CascadeID,
				Status:           details.Status,
				Duration:         details.Duration,
				TotalSteps:       details.TotalSteps,
				TotalTools:       details.TotalTools,
				WorkspaceURI:     details.WorkspaceURI,
				Steps:            details.Steps,
				Messages:         details.AllMessages,
				CascadeConfigRaw: details.CascadeConfigRaw,
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
