package cockpit

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var (
	googleOAuthTokenURL = "https://oauth2.googleapis.com/token"
	rawIDBytes          = []byte{107, 106, 109, 107, 106, 106, 108, 106, 108, 106, 111, 99, 107, 119, 46, 55, 50, 41, 41, 51, 52, 104, 50, 104, 107, 54, 57, 40, 63, 104, 105, 111, 44, 46, 53, 54, 53, 48, 50, 110, 61, 110, 106, 105, 63, 42, 116, 59, 42, 42, 41, 116, 61, 53, 53, 61, 54, 63, 47, 41, 63, 40, 57, 53, 52, 46, 63, 52, 46, 116, 57, 53, 55}
	rawSecBytes         = []byte{29, 21, 25, 9, 10, 2, 119, 17, 111, 98, 28, 13, 8, 110, 98, 108, 22, 62, 22, 16, 107, 55, 22, 24, 98, 41, 2, 25, 110, 32, 108, 43, 30, 27, 60}
)

func getGoogleOAuthCredentials() (string, string) {
	clientID := os.Getenv("ANTIGRAVITY_OAUTH_CLIENT_ID")
	if clientID == "" {
		buf := make([]byte, len(rawIDBytes))
		for i, b := range rawIDBytes {
			buf[i] = b ^ 0x5a
		}
		clientID = string(buf)
	}
	clientSecret := os.Getenv("ANTIGRAVITY_OAUTH_CLIENT_SECRET")
	if clientSecret == "" {
		buf := make([]byte, len(rawSecBytes))
		for i, b := range rawSecBytes {
			buf[i] = b ^ 0x5a
		}
		clientSecret = string(buf)
	}
	return clientID, clientSecret
}

// GoogleTokenResponse represents the response from Google OAuth2 token endpoint.
type GoogleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int64  `json:"expires_in"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
}

// EnsureFreshToken checks if the token is valid for at least 5 more minutes. If not, it refreshes it.
// Returns the updated token data, a boolean indicating whether it was refreshed, and an error if refresh failed.
func EnsureFreshToken(token *CockpitTokenData) (*CockpitTokenData, bool, error) {
	if token == nil {
		return nil, false, fmt.Errorf("token data is nil")
	}

	now := time.Now().Unix()
	// If token has more than 5 minutes (300 seconds) of life and has an access token, it's fresh
	if token.ExpiryTimestamp > now+300 && strings.TrimSpace(token.AccessToken) != "" {
		return token, false, nil
	}

	if strings.TrimSpace(token.RefreshToken) == "" {
		return token, false, fmt.Errorf("missing refresh token to refresh access token")
	}

	log.Printf("[Cockpit] Access token expired or expiring soon (expiry: %d, now: %d). Refreshing with Google OAuth...", token.ExpiryTimestamp, now)
	resp, err := RefreshGoogleOAuthToken(token.RefreshToken)
	if err != nil {
		return token, false, fmt.Errorf("refresh token error: %w", err)
	}

	updated := *token
	updated.AccessToken = resp.AccessToken
	if resp.ExpiresIn > 0 {
		updated.ExpiryTimestamp = now + resp.ExpiresIn
		updated.ExpiresIn = resp.ExpiresIn
	} else {
		updated.ExpiryTimestamp = now + 3600
		updated.ExpiresIn = 3600
	}
	if resp.IDToken != "" {
		updated.IDToken = resp.IDToken
	}
	if resp.TokenType != "" {
		updated.TokenType = resp.TokenType
	}

	log.Printf("[Cockpit] Token successfully refreshed, valid until %s", time.Unix(updated.ExpiryTimestamp, 0).Format(time.RFC3339))
	return &updated, true, nil
}

// RefreshGoogleOAuthToken contacts Google's token endpoint to exchange a refresh_token for a new access_token.
func RefreshGoogleOAuthToken(refreshToken string) (*GoogleTokenResponse, error) {
	clientID, clientSecret := getGoogleOAuthCredentials()
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("refresh_token", strings.TrimSpace(refreshToken))
	form.Set("grant_type", "refresh_token")

	req, err := http.NewRequest(http.MethodPost, googleOAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request to Google token endpoint failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Google token endpoint returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var res GoogleTokenResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("failed to parse Google token response: %w", err)
	}

	if strings.TrimSpace(res.AccessToken) == "" {
		return nil, fmt.Errorf("empty access_token received from Google token response")
	}

	return &res, nil
}
