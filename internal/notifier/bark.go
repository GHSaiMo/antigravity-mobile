package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"antigravity-mobile/internal/config"
)

// BarkPayload represents the JSON body sent to Bark's push notification API.
type BarkPayload struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	Icon     string `json:"icon,omitempty"`
	Group    string `json:"group,omitempty"`
	URL      string `json:"url,omitempty"`
	Level    string `json:"level,omitempty"` // "active", "timeSensitive", "passive"
	Sound    string `json:"sound,omitempty"`
	Badge    int    `json:"badge,omitempty"`
	Category string `json:"category,omitempty"`
}

// BarkClient handles sending push notifications to a device running Bark.
type BarkClient struct {
	endpoint      string // e.g. "https://api.day.app/8CTWiePgCJqiZNsJBaaDX9"
	iconURL       string
	group         string
	soundAction   string
	soundComplete string
	client        *http.Client
}

// NewBarkClient creates a BarkClient from configuration.
func NewBarkClient(cfg config.NotificationConfig) *BarkClient {
	return &BarkClient{
		endpoint:      cfg.BarkEndpoint,
		iconURL:       cfg.IconURL,
		group:         cfg.Group,
		soundAction:   cfg.SoundAction,
		soundComplete: cfg.SoundComplete,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Send dispatches a push notification to Bark with automatic retry on failure.
func (b *BarkClient) Send(ctx context.Context, payload BarkPayload) error {
	if b.endpoint == "" {
		return fmt.Errorf("bark endpoint not configured")
	}

	// Apply defaults from client configuration if not specified in payload
	if payload.Icon == "" {
		payload.Icon = b.iconURL
	}
	if payload.Group == "" {
		payload.Group = b.group
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal bark payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1 * time.Second):
			}
			log.Printf("[Bark] 🔄 Retrying notification send (attempt %d)...", attempt+1)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("failed to create http request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")

		resp, err := b.client.Do(req)
		if err != nil {
			lastErr = err
			log.Printf("[Bark] ❌ Failed to send notification (attempt %d): %v", attempt+1, err)
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("bark server error %d: %s", resp.StatusCode, string(respBody))
			log.Printf("[Bark] ⚠️ Server returned status %d: %s", resp.StatusCode, string(respBody))
			continue
		}

		log.Printf("[Bark] 🚀 Notification sent successfully: %q - %q (level=%s, sound=%s)",
			payload.Title, payload.Body, payload.Level, payload.Sound)
		return nil
	}

	return lastErr
}
