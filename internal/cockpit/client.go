package cockpit

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"
)

// CockpitServerInfo contains runtime server details from ~/.antigravity_cockpit/server.json.
type CockpitServerInfo struct {
	WsPort    int    `json:"ws_port"`
	Version   string `json:"version"`
	PID       int    `json:"pid"`
	AuthToken string `json:"auth_token"`
}

type wsMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type switchAccountPayload struct {
	RequestID string `json:"request_id"`
	AccountID string `json:"account_id"`
}

type eventSwitchErrorPayload struct {
	Message string `json:"message"`
}

type responseErrorPayload struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error"`
}

type responseSuccessPayload struct {
	RequestID string `json:"request_id"`
	Message   string `json:"message"`
}

type eventAccountSwitchedPayload struct {
	AccountID string `json:"account_id"`
	Email     string `json:"email"`
}

// GetCockpitServerInfo retrieves server info including the dynamic ws_port.
func GetCockpitServerInfo() (*CockpitServerInfo, error) {
	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return nil, err
	}
	serverFile := filepath.Join(dataDir, "server.json")
	bytes, err := os.ReadFile(serverFile)
	if err != nil {
		return nil, fmt.Errorf("cockpit server.json not found: %w", err)
	}
	var info CockpitServerInfo
	if err := json.Unmarshal(bytes, &info); err != nil {
		return nil, fmt.Errorf("failed to parse server.json: %w", err)
	}
	if info.WsPort <= 0 {
		return nil, fmt.Errorf("invalid ws_port in server.json: %d", info.WsPort)
	}
	return &info, nil
}

func generateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "req-" + hex.EncodeToString(b)
}

// SwitchAccount connects to Cockpit Tools' WebSocket server and requests an account switch.
func SwitchAccount(accountID string) error {
	if accountID == "" {
		return errors.New("account_id is required")
	}

	serverInfo, err := GetCockpitServerInfo()
	if err != nil {
		return fmt.Errorf("cannot connect to Cockpit Tools: %w", err)
	}

	wsURL := url.URL{
		Scheme: "ws",
		Host:   fmt.Sprintf("127.0.0.1:%d", serverInfo.WsPort),
		Path:   "/",
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 3 * time.Second,
	}

	conn, _, err := dialer.Dial(wsURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect to Cockpit Tools ws (%s): %w", wsURL.String(), err)
	}
	defer conn.Close()

	reqID := generateRequestID()
	reqPayload, _ := json.Marshal(switchAccountPayload{
		RequestID: reqID,
		AccountID: accountID,
	})

	switchReq := wsMessage{
		Type:    "request.switch_account",
		Payload: reqPayload,
	}

	reqBytes, err := json.Marshal(switchReq)
	if err != nil {
		return fmt.Errorf("failed to marshal switch request: %w", err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, reqBytes); err != nil {
		return fmt.Errorf("failed to send switch request: %w", err)
	}

	// Read messages until success or failure or timeout
	deadline := time.Now().Add(6 * time.Second)
	_ = conn.SetReadDeadline(deadline)

	var lastErr string
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if lastErr != "" {
				return errors.New(lastErr)
			}
			return fmt.Errorf("read from Cockpit Tools timed out or failed: %w", err)
		}

		var msg wsMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		switch msg.Type {
		case "event.account_switched":
			var p eventAccountSwitchedPayload
			_ = json.Unmarshal(msg.Payload, &p)
			if p.AccountID == accountID {
				return nil
			}
		case "response.success":
			var p responseSuccessPayload
			_ = json.Unmarshal(msg.Payload, &p)
			if p.RequestID == reqID {
				return nil
			}
		case "event.switch_error":
			var p eventSwitchErrorPayload
			_ = json.Unmarshal(msg.Payload, &p)
			if p.Message != "" {
				lastErr = p.Message
			}
		case "response.error":
			var p responseErrorPayload
			_ = json.Unmarshal(msg.Payload, &p)
			if p.RequestID == reqID && p.Error != "" {
				return errors.New(p.Error)
			}
		}
	}
}
