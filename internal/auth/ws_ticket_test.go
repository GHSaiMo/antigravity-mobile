package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWSTicketStore_IssueAndValidate(t *testing.T) {
	store := NewWSTicketStore()

	ticket, err := store.Issue("device-123")
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}
	if len(ticket) != 40 {
		t.Fatalf("expected 40-char hex ticket, got %q", ticket)
	}

	// First validation: should succeed and return deviceID
	deviceID, ok := store.Validate(ticket)
	if !ok || deviceID != "device-123" {
		t.Fatalf("expected validation success for device-123, got ok=%v deviceID=%q", ok, deviceID)
	}

	// Second validation: should fail (one-time use)
	deviceID, ok = store.Validate(ticket)
	if ok || deviceID != "" {
		t.Fatalf("expected one-time ticket to be consumed, got ok=%v deviceID=%q", ok, deviceID)
	}
}

func TestWSTicketStore_Expiration(t *testing.T) {
	store := NewWSTicketStore()

	ticket, err := store.Issue("device-expired")
	if err != nil {
		t.Fatalf("Issue failed: %v", err)
	}

	// Artificially age the ticket past TTL
	store.mu.Lock()
	store.tickets[ticket].createdAt = time.Now().Add(-1 * time.Hour)
	store.mu.Unlock()

	deviceID, ok := store.Validate(ticket)
	if ok || deviceID != "" {
		t.Fatalf("expected expired ticket to fail validation, got ok=%v deviceID=%q", ok, deviceID)
	}
}

func TestAuthMiddleware_WSTicketAuth(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewAuthStore(tmpDir + "/auth.json")
	if err != nil {
		t.Fatal(err)
	}

	deviceToken := "valid-device-token-12345"
	dev := PairedDevice{
		DeviceID:   "dev-abc",
		DeviceName: "iPhone 15",
		Platform:   "ios",
		TokenHash:  HashToken(deviceToken),
		CreatedAt:  time.Now(),
	}
	if err := store.AddDevice(dev); err != nil {
		t.Fatal(err)
	}

	pm := NewPairingManager()
	authHandler := NewAuthHandler(store, pm, "127.0.0.1", 58900, false)

	// Step 1: Exchange device token for a WS ticket via /api/v1/auth/ws-ticket
	ticketReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/ws-ticket", nil)
	ticketReq.Header.Set("Authorization", "Bearer "+deviceToken)
	ticketRec := httptest.NewRecorder()
	authHandler.HandleWSTicket(ticketRec, ticketReq)

	if ticketRec.Code != http.StatusOK {
		t.Fatalf("HandleWSTicket failed with code %d: %s", ticketRec.Code, ticketRec.Body.String())
	}

	var ticketResp struct {
		Ticket    string `json:"ticket"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := json.NewDecoder(ticketRec.Body).Decode(&ticketResp); err != nil {
		t.Fatalf("failed to parse ticket response: %v", err)
	}
	if ticketResp.Ticket == "" {
		t.Fatal("empty ticket returned")
	}

	// Protected handler
	var authedDevice *PairedDevice
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authedDevice, _ = DeviceFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	router := AuthMiddlewareWithPolicy(store, next, AuthPolicy{
		TunnelEnabled:  false,
		ListenLoopback: false,
	})

	// Step 2: Use ticket to access WebSocket route /gateway/cascade/stream?cascadeId=test&ticket=...
	wsReq := httptest.NewRequest(http.MethodGet, "/gateway/cascade/stream?cascadeId=test&ticket="+ticketResp.Ticket, nil)
	wsReq.Header.Set("Upgrade", "websocket")
	wsReq.Header.Set("Connection", "Upgrade")
	wsRec := httptest.NewRecorder()

	router.ServeHTTP(wsRec, wsReq)

	if wsRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK using valid ticket, got %d: %s", wsRec.Code, wsRec.Body.String())
	}
	if authedDevice == nil || authedDevice.DeviceID != "dev-abc" {
		t.Fatalf("expected authenticated device dev-abc, got %+v", authedDevice)
	}

	// Step 3: Reusing the same ticket must fail (one-time use)
	wsReq2 := httptest.NewRequest(http.MethodGet, "/gateway/cascade/stream?cascadeId=test&ticket="+ticketResp.Ticket, nil)
	wsReq2.Header.Set("Upgrade", "websocket")
	wsReq2.Header.Set("Connection", "Upgrade")
	wsRec2 := httptest.NewRecorder()

	router.ServeHTTP(wsRec2, wsReq2)

	if wsRec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized when reusing ticket, got %d", wsRec2.Code)
	}

	// Give background worker a moment to finish any async last-seen updates before TempDir cleanup
	time.Sleep(30 * time.Millisecond)
}
