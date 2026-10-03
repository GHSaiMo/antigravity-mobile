package cockpit

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	RequestID          string `json:"request_id"`
	AccountID          string `json:"account_id"`
	AuthToken          string `json:"auth_token,omitempty"`
	RuntimeTarget      string `json:"runtime_target,omitempty"`
	RuntimeTargetCamel string `json:"runtimeTarget,omitempty"`
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

type accountsWithTokensPayload struct {
	RequestID string                `json:"request_id"`
	Accounts  []cockpitAccountToken `json:"accounts"`
}

type cockpitAccountToken struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
}

// ReadRawCockpitServerInfo reads server.json directly without performing port probing or recursion.
func ReadRawCockpitServerInfo() (*CockpitServerInfo, error) {
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

// GetCockpitServerInfo retrieves server info including the dynamic ws_port.
// If server.json is missing or its port is not responding, it attempts auto-detection.
func GetCockpitServerInfo() (*CockpitServerInfo, error) {
	rawInfo, err := ReadRawCockpitServerInfo()
	if err == nil && rawInfo != nil && rawInfo.WsPort > 0 {
		if IsCockpitListening(rawInfo.WsPort, 300*time.Millisecond) {
			return rawInfo, nil
		}
		// Port in server.json not listening: attempt auto-resolve
		if activePort, authToken, rErr := ResolveActiveWsPort(); rErr == nil && activePort > 0 {
			log.Printf("[Cockpit] Port %d in server.json not responding, auto-switched to active ws_port: %d", rawInfo.WsPort, activePort)
			rawInfo.WsPort = activePort
			if authToken != "" {
				rawInfo.AuthToken = authToken
			}
			return rawInfo, nil
		}
		return rawInfo, nil
	}

	// server.json missing or invalid: attempt auto-resolve
	if activePort, authToken, rErr := ResolveActiveWsPort(); rErr == nil && activePort > 0 {
		log.Printf("[Cockpit] server.json missing/invalid, auto-detected active ws_port: %d", activePort)
		return &CockpitServerInfo{
			WsPort:    activePort,
			AuthToken: authToken,
		}, nil
	}

	if err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("invalid ws_port in server.json")
}

func generateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "req-" + hex.EncodeToString(b)
}

func switchAccountDirect(accountID string) (bool, error) {
	detail, err := DecryptAccountDetail(accountID)
	if err != nil || detail == nil || detail.Token.RefreshToken == "" {
		return false, nil
	}

	log.Printf("[Cockpit] Found local encrypted account for %s (ID: %s). Executing direct native switch...", detail.Email, detail.ID)

	// Ensure token is fresh before injection
	freshTok, refreshed, rErr := EnsureFreshToken(&detail.Token)
	if rErr != nil {
		log.Printf("[Cockpit] Warning: token refresh failed (%v), proceeding with existing token", rErr)
	} else if refreshed {
		detail.Token = *freshTok
		if sErr := SaveAccountDetail(detail); sErr != nil {
			log.Printf("[Cockpit] Warning: failed to save refreshed account: %v", sErr)
		}
	}

	if err := quitAntigravityBeforeSwitch(); err != nil {
		return true, fmt.Errorf("quit Antigravity before switch: %w", err)
	}

	// Sync Cockpit Tools local state
	if err := SyncCockpitLocalState(detail.ID, detail.Email); err != nil {
		log.Printf("[Cockpit] Warning: sync Cockpit local state failed: %v", err)
	}
	afterProfilePrepare()

	// Inject unified OAuth token into Antigravity SQLite state.vscdb
	if err := InjectAccountToAntigravityStateDB(detail); err != nil {
		log.Printf("[Cockpit] Warning: SQLite state.vscdb injection error: %v", err)
	}

	// Apply Keychain, Jetski token file, relaunch IDE and verify with Language Server
	InvalidateQuotaCache()
	if err := applyLanguageServerOAuth(detail.ID); err != nil {
		return true, fmt.Errorf("apply language server oauth and relaunch: %w", err)
	}

	log.Printf("[Cockpit] Direct native switch completed successfully for %s", detail.Email)
	return true, nil
}

// SwitchAccount switches active account using direct local storage if available,
// falling back to Cockpit Tools WebSocket server only if local storage is missing.
func SwitchAccount(accountID string) (err error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return errors.New("account_id is required")
	}

	// Auto-align Cockpit config to prevent APP_PATH_NOT_FOUND during switch
	_ = EnsureCockpitAntigravityConfig()

	// If accountID is an email or alias, resolve it to canonical UUID in accounts.json
	if resolved, rErr := ResolveAccountID(accountID); rErr == nil && resolved != "" {
		accountID = resolved
	}

	prevBindID := getLegacyBindAccount()
	defer func() {
		if err != nil && prevBindID != "" && prevBindID != accountID {
			syncLegacyBindAccount(prevBindID)
			log.Printf("[Cockpit] SwitchAccount failed; rolled back antigravity_legacy_instances bindAccountId to %s", prevBindID)
		}
	}()

	// Fast path: Direct local native switch without Cockpit Tools desktop process
	if handled, sErr := switchAccountDirect(accountID); handled {
		if sErr != nil {
			return sErr
		}
		return nil
	}

	// Fallback path: WebSocket switch via running Cockpit Tools desktop process
	if err = quitAntigravityBeforeSwitch(); err != nil {
		return fmt.Errorf("quit Antigravity before switch: %w", err)
	}
	prepareAntigravityProfileForSwitch(accountID)
	afterProfilePrepare()

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
		RequestID:          reqID,
		AccountID:          accountID,
		AuthToken:          serverInfo.AuthToken,
		RuntimeTarget:      "antigravity",
		RuntimeTargetCamel: "antigravity",
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
	log.Printf("[Cockpit] Sent request.switch_account account_id=%s runtime_target=antigravity", accountID)

	// Token refresh + inject + relaunch commonly takes >6s.
	deadline := time.Now().Add(90 * time.Second)
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
			if p.AccountID == accountID || (p.Email != "" && strings.EqualFold(p.Email, accountID)) {
				InvalidateQuotaCache()
				return applyLanguageServerOAuth(accountID)
			}
		case "response.success":
			var p responseSuccessPayload
			_ = json.Unmarshal(msg.Payload, &p)
			if p.RequestID == reqID {
				InvalidateQuotaCache()
				return applyLanguageServerOAuth(accountID)
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

func fetchAccountOAuth(accountID string) (*parsedOAuth, error) {
	// Fast path: direct from local storage without connecting to WebSocket
	if detail, err := DecryptAccountDetail(accountID); err == nil && detail != nil && detail.Token.RefreshToken != "" {
		freshTok, refreshed, _ := EnsureFreshToken(&detail.Token)
		if refreshed {
			detail.Token = *freshTok
			_ = SaveAccountDetail(detail)
		}
		exp := time.Now().Add(50 * time.Minute)
		if freshTok.ExpiryTimestamp > 0 {
			exp = time.Unix(freshTok.ExpiryTimestamp, 0)
		}
		return &parsedOAuth{
			AccessToken:  freshTok.AccessToken,
			RefreshToken: freshTok.RefreshToken,
			IDToken:      freshTok.IDToken,
			Email:        strings.ToLower(strings.TrimSpace(detail.Email)),
			Expiry:       exp,
		}, nil
	}

	serverInfo, err := GetCockpitServerInfo()
	if err != nil {
		return nil, err
	}
	wsURL := url.URL{Scheme: "ws", Host: fmt.Sprintf("127.0.0.1:%d", serverInfo.WsPort), Path: "/"}
	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	conn, _, err := dialer.Dial(wsURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("cockpit ws for tokens: %w", err)
	}
	defer conn.Close()
	reqID := generateRequestID()
	payload, _ := json.Marshal(map[string]string{
		"request_id": reqID,
		"auth_token": serverInfo.AuthToken,
	})
	msg, _ := json.Marshal(wsMessage{Type: "request.get_accounts_with_tokens", Payload: payload})
	if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
		return nil, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(8 * time.Second))
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("read accounts_with_tokens: %w", err)
		}
		var envelope wsMessage
		if json.Unmarshal(raw, &envelope) != nil {
			continue
		}
		if envelope.Type != "response.accounts_with_tokens" {
			continue
		}
		var p accountsWithTokensPayload
		if json.Unmarshal(envelope.Payload, &p) != nil {
			return nil, fmt.Errorf("parse accounts_with_tokens payload")
		}
		want := strings.TrimSpace(accountID)
		for _, acc := range p.Accounts {
			if acc.ID != want && !strings.EqualFold(acc.Email, want) {
				continue
			}
			if acc.AccessToken == "" || acc.RefreshToken == "" {
				return nil, fmt.Errorf("account %s missing tokens", acc.Email)
			}
			exp := time.Now().Add(50 * time.Minute)
			if acc.ExpiresAt > 0 {
				exp = time.Unix(acc.ExpiresAt, 0)
			}
			return &parsedOAuth{
				AccessToken:  acc.AccessToken,
				RefreshToken: acc.RefreshToken,
				Email:        strings.ToLower(strings.TrimSpace(acc.Email)),
				Expiry:       exp,
			}, nil
		}
		return nil, fmt.Errorf("account %s not in cockpit token list", accountID)
	}
}
