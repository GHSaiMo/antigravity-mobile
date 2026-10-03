package cockpit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEnsureFreshToken_AlreadyFresh(t *testing.T) {
	tok := &CockpitTokenData{
		AccessToken:     "fresh-access-token",
		RefreshToken:    "valid-refresh-token",
		ExpiryTimestamp: time.Now().Unix() + 1000,
	}

	result, refreshed, err := EnsureFreshToken(tok)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if refreshed {
		t.Fatalf("expected refreshed=false, got true")
	}
	if result.AccessToken != "fresh-access-token" {
		t.Fatalf("access token altered: %s", result.AccessToken)
	}
}

func TestEnsureFreshToken_MockRefresh(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		_ = r.ParseForm()
		expectedClientID, _ := getGoogleOAuthCredentials()
		if r.FormValue("client_id") != expectedClientID {
			t.Errorf("unexpected client_id: %s", r.FormValue("client_id"))
		}
		if r.FormValue("refresh_token") != "test-refresh-token" {
			t.Errorf("unexpected refresh_token: %s", r.FormValue("refresh_token"))
		}

		resp := GoogleTokenResponse{
			AccessToken: "new-fresh-access-token",
			ExpiresIn:   3600,
			TokenType:   "Bearer",
			IDToken:     "new-id-token",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	origURL := googleOAuthTokenURL
	googleOAuthTokenURL = mockServer.URL
	defer func() { googleOAuthTokenURL = origURL }()

	tok := &CockpitTokenData{
		AccessToken:     "expired-token",
		RefreshToken:    "test-refresh-token",
		ExpiryTimestamp: time.Now().Unix() - 100, // Expired
	}

	result, refreshed, err := EnsureFreshToken(tok)
	if err != nil {
		t.Fatalf("EnsureFreshToken failed: %v", err)
	}
	if !refreshed {
		t.Fatalf("expected refreshed=true, got false")
	}
	if result.AccessToken != "new-fresh-access-token" {
		t.Fatalf("unexpected access token: %s", result.AccessToken)
	}
	if result.IDToken != "new-id-token" {
		t.Fatalf("unexpected ID token: %s", result.IDToken)
	}
}
