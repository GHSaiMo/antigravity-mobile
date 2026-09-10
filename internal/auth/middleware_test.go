package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestAuthMiddleware_And_Handler(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "auth_store.json")
	store, err := NewAuthStore(storePath)
	if err != nil {
		t.Fatalf("failed to create auth store: %v", err)
	}

	pm := NewPairingManager()
	authHandler := NewAuthHandler(store, pm, "mac.local", 58900, false)

	// Setup a dummy protected handler wrapped in AuthMiddleware
	protectedHit := false
	protectedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protectedHit = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/pair", authHandler.HandlePair)
	mux.HandleFunc("/api/v1/devices", authHandler.HandleDevices)
	mux.HandleFunc("/api/v1/devices/", authHandler.HandleDevices)
	mux.Handle("/codeium.cascade.test", protectedHandler)

	wrappedRouter := AuthMiddleware(store, mux)

	// 1. Access protected route without token -> 401
	req1 := httptest.NewRequest(http.MethodGet, "/codeium.cascade.test", nil)
	rr1 := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr1.Code)
	}

	// 2. Generate pairing code and pair
	session, err := pm.GenerateSession(5 * time.Minute)
	if err != nil {
		t.Fatalf("failed to generate session: %v", err)
	}

	pairBody, _ := json.Marshal(PairRequest{
		PairingCode: session.Code,
		DeviceName:  "Test Phone",
		Platform:    "ios",
	})
	pairReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/pair", bytes.NewReader(pairBody))
	pairReq.Header.Set("Content-Type", "application/json")
	pairRR := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(pairRR, pairReq)

	if pairRR.Code != http.StatusOK {
		t.Fatalf("expected 200 from pair, got %d (body: %s)", pairRR.Code, pairRR.Body.String())
	}

	var pairResp PairResponse
	if err := json.NewDecoder(pairRR.Body).Decode(&pairResp); err != nil {
		t.Fatalf("failed to parse pair response: %v", err)
	}
	if pairResp.DeviceToken == "" || pairResp.DeviceID == "" {
		t.Fatalf("empty token or device id in response: %+v", pairResp)
	}

	// 3. Re-using pairing code should fail
	pairReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/pair", bytes.NewReader(pairBody))
	pairReq2.Header.Set("Content-Type", "application/json")
	pairRR2 := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(pairRR2, pairReq2)
	if pairRR2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on re-used code, got %d", pairRR2.Code)
	}

	// 4. Access protected route with Bearer Header -> 200
	req2 := httptest.NewRequest(http.MethodGet, "/codeium.cascade.test", nil)
	req2.Header.Set("Authorization", "Bearer "+pairResp.DeviceToken)
	rr2 := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("expected 200 with Bearer token, got %d", rr2.Code)
	}
	if !protectedHit {
		t.Errorf("expected protected handler to be hit")
	}

	// 5. Access with Query Param (?auth_token=...) -> 200
	req3 := httptest.NewRequest(http.MethodGet, "/codeium.cascade.test?auth_token="+pairResp.DeviceToken, nil)
	rr3 := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Fatalf("expected 200 with query token, got %d", rr3.Code)
	}

	// 6. Whitelisted path (/gateway/status) without token -> 200
	statusHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/gateway/status", statusHandler)
	reqStatus := httptest.NewRequest(http.MethodGet, "/gateway/status", nil)
	rrStatus := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rrStatus, reqStatus)
	if rrStatus.Code != http.StatusOK {
		t.Fatalf("expected 200 for whitelisted /gateway/status, got %d", rrStatus.Code)
	}

	// 7. Device list via Loopback
	reqList := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	reqList.RemoteAddr = "127.0.0.1:12345"
	rrList := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rrList, reqList)
	if rrList.Code != http.StatusOK {
		t.Fatalf("expected 200 from devices list, got %d", rrList.Code)
	}

	var devices []PairedDevice
	if err := json.NewDecoder(rrList.Body).Decode(&devices); err != nil {
		t.Fatalf("failed to decode devices: %v", err)
	}
	if len(devices) != 1 || devices[0].DeviceID != pairResp.DeviceID {
		t.Fatalf("expected 1 device with id %s, got %+v", pairResp.DeviceID, devices)
	}

	// 8. Delete device
	reqDel := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/"+pairResp.DeviceID, nil)
	reqDel.RemoteAddr = "127.0.0.1:12345"
	rrDel := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rrDel, reqDel)
	if rrDel.Code != http.StatusOK {
		t.Fatalf("expected 200 on delete, got %d", rrDel.Code)
	}

	// 9. Token revoked after delete -> 401
	reqRevoked := httptest.NewRequest(http.MethodGet, "/codeium.cascade.test", nil)
	reqRevoked.Header.Set("Authorization", "Bearer "+pairResp.DeviceToken)
	rrRevoked := httptest.NewRecorder()
	wrappedRouter.ServeHTTP(rrRevoked, reqRevoked)
	if rrRevoked.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after revocation, got %d", rrRevoked.Code)
	}
}
