package cockpit

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gorilla/websocket"
)

func TestSwitchAccountMockWS(t *testing.T) {
	origQuit := quitAntigravityBeforeSwitch
	quitCalled := 0
	quitAntigravityBeforeSwitch = func() error {
		quitCalled++
		return nil
	}
	t.Cleanup(func() { quitAntigravityBeforeSwitch = origQuit })

	origApply := applyLanguageServerOAuth
	applyLanguageServerOAuth = func(string) error { return nil }
	t.Cleanup(func() { applyLanguageServerOAuth = origApply })

	upgrader := websocket.Upgrader{}

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// 1. Send ready
		_ = conn.WriteJSON(map[string]any{
			"type":    "event.ready",
			"payload": map[string]any{"version": "1.3.47"},
		})

		// 2. Read request
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var req wsMessage
		_ = json.Unmarshal(msg, &req)
		if req.Type != "request.switch_account" {
			t.Errorf("unexpected req type: %s", req.Type)
			return
		}

		var p switchAccountPayload
		_ = json.Unmarshal(req.Payload, &p)
		if p.RuntimeTarget != "antigravity" && p.RuntimeTargetCamel != "antigravity" {
			t.Errorf("expected runtime_target=antigravity, got %q / %q", p.RuntimeTarget, p.RuntimeTargetCamel)
		}

		if p.AccountID == "valid-id" {
			_ = conn.WriteJSON(map[string]any{
				"type": "event.account_switched",
				"payload": map[string]any{
					"account_id": "valid-id",
					"email":      "test@example.com",
				},
			})
			_ = conn.WriteJSON(map[string]any{
				"type": "response.success",
				"payload": map[string]any{
					"request_id": p.RequestID,
					"message":    "切换账号成功",
				},
			})
		} else {
			_ = conn.WriteJSON(map[string]any{
				"type": "event.switch_error",
				"payload": map[string]any{
					"message": "ACCOUNT_NOT_FOUND",
				},
			})
			_ = conn.WriteJSON(map[string]any{
				"type": "response.error",
				"payload": map[string]any{
					"request_id": p.RequestID,
					"error":      "ACCOUNT_NOT_FOUND",
				},
			})
		}
	}))
	defer s.Close()

	u, _ := url.Parse(s.URL)
	port, _ := strconv.Atoi(u.Port())

	// Create temp home dir for server.json
	tmpDir := t.TempDir()
	cockpitDir := filepath.Join(tmpDir, ".antigravity_cockpit")
	_ = os.MkdirAll(cockpitDir, 0755)

	serverInfo := CockpitServerInfo{
		WsPort:    port,
		Version:   "1.3.47",
		PID:       1234,
		AuthToken: "test",
	}
	bytes, _ := json.Marshal(serverInfo)
	_ = os.WriteFile(filepath.Join(cockpitDir, "server.json"), bytes, 0644)

	// Override HOME, USERPROFILE, and COCKPIT_DATA_DIR for cross-platform isolation
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
	t.Setenv("COCKPIT_DATA_DIR", cockpitDir)

	// Test success
	err := SwitchAccount("valid-id")
	if err != nil {
		t.Fatalf("expected nil error on valid switch, got: %v", err)
	}
	if quitCalled == 0 {
		t.Fatal("expected Antigravity to be quit before switch")
	}

	// Test failure with rollback verification
	legacyPath := filepath.Join(cockpitDir, "antigravity_legacy_instances.json")
	_ = os.WriteFile(legacyPath, []byte(`{
		"instances": [],
		"defaultSettings": {
			"bindAccountId": "original-id"
		}
	}`), 0644)

	err = SwitchAccount("invalid-id")
	if err == nil || err.Error() != "ACCOUNT_NOT_FOUND" {
		t.Fatalf("expected ACCOUNT_NOT_FOUND, got: %v", err)
	}

	// Verify legacy instance bindAccountId was rolled back to original-id
	if got := getLegacyBindAccount(); got != "original-id" {
		t.Fatalf("expected rollback to original-id, got %q", got)
	}
}

func TestSwitchAccount_DirectNativeSwitch(t *testing.T) {
	origQuit := quitAntigravityBeforeSwitch
	quitCalled := 0
	quitAntigravityBeforeSwitch = func() error {
		quitCalled++
		return nil
	}
	t.Cleanup(func() { quitAntigravityBeforeSwitch = origQuit })

	origApply := applyLanguageServerOAuth
	applyCalled := 0
	var appliedID string
	applyLanguageServerOAuth = func(id string) error {
		applyCalled++
		appliedID = id
		return nil
	}
	t.Cleanup(func() { applyLanguageServerOAuth = origApply })

	tmpDir := t.TempDir()
	cockpitDir := filepath.Join(tmpDir, ".antigravity_cockpit")
	_ = os.MkdirAll(cockpitDir, 0755)

	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)
	t.Setenv("COCKPIT_DATA_DIR", cockpitDir)

	// Setup AES key
	rawKey := make([]byte, 32)
	for i := range rawKey {
		rawKey[i] = byte(i + 1)
	}
	_ = os.WriteFile(filepath.Join(cockpitDir, storageKeyFile), []byte(base64.StdEncoding.EncodeToString(rawKey)), 0600)

	// Setup accounts.json
	idx := accountsIndex{
		CurrentAccountID: "acc-old",
		Accounts: []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Name  string `json:"name"`
		}{
			{ID: "acc-old", Email: "old@example.com", Name: "Old User"},
			{ID: "acc-native-1", Email: "native@example.com", Name: "Native User"},
		},
	}
	idxBytes, _ := json.Marshal(idx)
	_ = os.WriteFile(filepath.Join(cockpitDir, "accounts.json"), idxBytes, 0644)

	// Save target account details in encrypted format
	targetAcc := &CockpitAccountDetail{
		ID:        "acc-native-1",
		Email:     "native@example.com",
		CreatedAt: 1791000000,
		Token: CockpitTokenData{
			AccessToken:     "ya29.native-access",
			RefreshToken:    "1//native-refresh",
			ExpiryTimestamp: 9999999999, // Fresh
		},
	}
	if err := SaveAccountDetail(targetAcc); err != nil {
		t.Fatalf("SaveAccountDetail failed: %v", err)
	}

	// NO server.json or mock WS server created! Direct native switch must work completely offline.
	if err := SwitchAccount("native@example.com"); err != nil {
		t.Fatalf("SwitchAccount failed on direct native path: %v", err)
	}

	if quitCalled != 1 {
		t.Fatalf("expected quitAntigravityBeforeSwitch to be called once, got %d", quitCalled)
	}
	if applyCalled != 1 || appliedID != "acc-native-1" {
		t.Fatalf("expected applyLanguageServerOAuth to be called for acc-native-1, got count=%d id=%q", applyCalled, appliedID)
	}

	// Verify local state was synchronized
	var curDoc struct {
		Email string `json:"email"`
	}
	curBytes, err := os.ReadFile(filepath.Join(cockpitDir, "current_account.json"))
	if err != nil {
		t.Fatalf("current_account.json not written: %v", err)
	}
	_ = json.Unmarshal(curBytes, &curDoc)
	if curDoc.Email != "native@example.com" {
		t.Fatalf("current_account.json email mismatch: got %q", curDoc.Email)
	}

	// Verify accounts.json was updated
	updatedAccBytes, _ := os.ReadFile(filepath.Join(cockpitDir, "accounts.json"))
	var updatedIdx accountsIndex
	_ = json.Unmarshal(updatedAccBytes, &updatedIdx)
	if updatedIdx.CurrentAccountID != "acc-native-1" {
		t.Fatalf("accounts.json current_account_id mismatch: got %q", updatedIdx.CurrentAccountID)
	}

	// Verify legacy bind account was updated
	if got := getLegacyBindAccount(); got != "acc-native-1" {
		t.Fatalf("legacy bind account mismatch: got %q", got)
	}
}
