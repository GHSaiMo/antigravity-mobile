package notifier

import (
	"log/slog"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/i18n"
)

// FCMNotification represents the visible notification card in Android notification center.
type FCMNotification struct {
	Title       string `json:"title"`
	Body        string `json:"body"`
	Sound       string `json:"sound,omitempty"`
	Icon        string `json:"icon,omitempty"`
	ChannelID   string `json:"android_channel_id,omitempty"`
	ClickAction string `json:"click_action,omitempty"`
}

// FCMPayload represents the JSON body sent to FCM push endpoint.
type FCMPayload struct {
	To           string            `json:"to"`
	Priority     string            `json:"priority"` // "high" or "normal"
	Notification *FCMNotification  `json:"notification,omitempty"`
	Data         map[string]string `json:"data,omitempty"`
}

// FCMClient handles sending push notifications to Android devices running Multigravity.
type FCMClient struct {
	mu          sync.RWMutex
	endpoint    string
	serverKey   string
	deviceToken string
	deviceLang  string
	iconURL     string
	store       *PushDeviceStore
	client      *http.Client
}

// SetPushStore attaches a shared PushDeviceStore for multi-device Android FCM fan-out.
func (f *FCMClient) SetPushStore(store *PushDeviceStore) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.store = store
}

// NewFCMClient creates an FCMClient from configuration.
func NewFCMClient(cfg config.NotificationConfig) *FCMClient {
	endpoint := strings.TrimSpace(cfg.FCMEndpoint)
	if endpoint == "" {
		endpoint = "https://fcm.googleapis.com/fcm/send"
	}
	return &FCMClient{
		endpoint:    endpoint,
		serverKey:   strings.TrimSpace(cfg.FCMServerKey),
		deviceToken: strings.TrimSpace(cfg.FCMDeviceToken),
		iconURL:     cfg.IconURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SetDeviceToken updates the target Android device registration token dynamically.
func (f *FCMClient) SetDeviceToken(token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deviceToken = strings.TrimSpace(token)
}

// DeviceToken returns the current device registration token.
func (f *FCMClient) DeviceToken() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.deviceToken
}

// SetDeviceLang records the preferred notification language ("zh", "en", or "ja") for the Android device.
// Any explicit non-empty locale other than Chinese or Japanese is stored as "en" so it never falls back to Chinese.
func (f *FCMClient) SetDeviceLang(lang string) {
	rawLang := strings.TrimSpace(lang)
	if rawLang == "" {
		return
	}
	norm := i18n.NormalizeLang(rawLang)
	if norm == "" {
		norm = i18n.LangEn
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deviceLang = norm
}

// DeviceLang returns the registered Android device's preferred notification language (or "" if unset).
func (f *FCMClient) DeviceLang() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.deviceLang
}

// ServerKey returns the configured server key.
func (f *FCMClient) ServerKey() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.serverKey
}

// Endpoint returns the target FCM HTTP endpoint.
func (f *FCMClient) Endpoint() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.endpoint
}

// Send dispatches a push notification to all registered FCM Android devices (or the legacy single token).
func (f *FCMClient) Send(ctx context.Context, payload PushPayload) error {
	f.mu.RLock()
	endpoint := f.endpoint
	serverKey := f.serverKey
	legacyToken := f.deviceToken
	legacyLang := f.deviceLang
	iconURL := f.iconURL
	store := f.store
	f.mu.RUnlock()

	if err := validatePushEndpointURL(endpoint); err != nil {
		return err
	}

	var targets []FCMAlertTarget
	if store != nil {
		targets = store.ListFCMTargets()
	}
	if len(targets) == 0 && legacyToken != "" {
		targets = []FCMAlertTarget{{
			Token: legacyToken,
			Lang:  legacyLang,
		}}
	}
	if len(targets) == 0 {
		return fmt.Errorf("fcm device token not configured")
	}

	if len(targets) == 1 {
		return f.sendOne(ctx, endpoint, serverKey, iconURL, store, targets[0], payload)
	}

	var (
		wg    sync.WaitGroup
		errCh = make(chan error, len(targets))
	)
	for _, target := range targets {
		wg.Add(1)
		go func(t FCMAlertTarget) {
			defer wg.Done()
			if err := f.sendOne(ctx, endpoint, serverKey, iconURL, store, t, payload); err != nil {
				errCh <- err
			}
		}(target)
	}
	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) > 0 && len(errs) == len(targets) {
		return errs[0]
	}
	return nil
}

func (f *FCMClient) sendOne(
	ctx context.Context,
	endpoint, serverKey, iconURL string,
	store *PushDeviceStore,
	target FCMAlertTarget,
	payload PushPayload,
) error {
	deviceToken := target.Token
	deviceLang := target.Lang
	gwID := strings.TrimSpace(payload.GatewayID)
	gwLabel := ""
	if target.ShowGatewayLabel {
		lbl := strings.TrimSpace(target.GatewayLabel)
		if lbl == "" {
			lbl = strings.TrimSpace(payload.GatewayName)
		}
		if lbl != "" {
			gwLabel = truncateString(SanitizePushText(lbl), 32)
		}
	}

	var fcmMsg FCMPayload
	var logLabel string

	if payload.Silent {
		action := strings.TrimSpace(payload.Action)
		if action == "" {
			action = strings.TrimSpace(payload.Category)
		}
		data := map[string]string{
			"action":    action,
			"category":  payload.Category,
			"cascadeId": payload.CascadeID,
			"url":       payload.URL,
			"deeplink":  payload.URL,
			"silent":    "true",
		}
		if gwID != "" {
			data["gatewayId"] = gwID
		}
		fcmMsg = FCMPayload{
			To:       deviceToken,
			Priority: "high",
			Data:     data,
		}
		logLabel = "silent:" + action
	} else {
		priority := "normal"
		channelID := "antigravity_alerts"
		if payload.Level == "timeSensitive" || strings.Contains(payload.Category, "action") || strings.Contains(payload.Category, "proceed") || strings.Contains(payload.Category, "error") {
			priority = "high"
		}

		icon := payload.Icon
		if icon == "" {
			icon = iconURL
		}

		rawTitle := payload.Title
		rawBody := payload.Body
		if payload.Localize != nil && deviceLang != "" {
			if locTitle, locBody := payload.Localize(deviceLang); locTitle != "" || locBody != "" {
				if locTitle != "" {
					rawTitle = locTitle
				}
				if locBody != "" {
					rawBody = locBody
				}
			}
		}
		cleanTitle := truncateString(SanitizePushText(rawTitle), 120)
		cleanBody := truncateString(SanitizePushText(rawBody), 240)
		logLabel = cleanTitle

		data := map[string]string{
			"title":     cleanTitle,
			"body":      cleanBody,
			"url":       payload.URL,
			"deeplink":  payload.URL,
			"level":     payload.Level,
			"sound":     payload.Sound,
			"group":     payload.Group,
			"category":  payload.Category,
			"cascadeId": payload.CascadeID,
			"icon":      icon,
		}
		if gwID != "" {
			data["gatewayId"] = gwID
		}
		if gwLabel != "" {
			data["gatewayLabel"] = gwLabel
		}

		fcmMsg = FCMPayload{
			To:       deviceToken,
			Priority: priority,
			Notification: &FCMNotification{
				Title:       cleanTitle,
				Body:        cleanBody,
				Sound:       "default",
				Icon:        "ic_stat_antigravity",
				ChannelID:   channelID,
				ClickAction: "FLUTTER_NOTIFICATION_CLICK", // standard Android notification intent flag
			},
			Data: data,
		}
	}

	bodyBytes, err := json.Marshal(fcmMsg)
	if err != nil {
		return fmt.Errorf("failed to marshal fcm payload: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1 * time.Second):
			}
			slog.Info(fmt.Sprintf("[FCM] 🔄 Retrying notification send (attempt %d)...", attempt+1))
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("failed to create http request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		if serverKey != "" {
			req.Header.Set("Authorization", "key="+serverKey)
		}

		resp, err := f.client.Do(req)
		if err != nil {
			lastErr = err
			slog.Error(fmt.Sprintf("[FCM] ❌ Failed to send notification (attempt %d)", attempt+1), "err", err)
			continue
		}

		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			snippet := truncateString(SanitizePushText(string(respBody)), 240)
			if store != nil && target.DeviceID != "" &&
				(strings.Contains(snippet, "NotRegistered") || strings.Contains(snippet, "InvalidRegistration")) {
				slog.Info("[FCM] 🧹 Clearing unregistered FCM device token", "device_id", target.DeviceID)
				store.ClearFCMToken(target.DeviceID, deviceToken)
				return nil
			}
			lastErr = fmt.Errorf("fcm server error %d: %s", resp.StatusCode, snippet)
			slog.Warn(fmt.Sprintf("[FCM] ⚠️ Server returned status %d: %s", resp.StatusCode, snippet))
			continue
		}

		if store != nil && target.DeviceID != "" &&
			(strings.Contains(string(respBody), `"NotRegistered"`) || strings.Contains(string(respBody), `"InvalidRegistration"`)) {
			slog.Info("[FCM] 🧹 Clearing unregistered FCM device token", "device_id", target.DeviceID)
			store.ClearFCMToken(target.DeviceID, deviceToken)
			return nil
		}

		slog.Info(fmt.Sprintf("[FCM] 🚀 推送成功: %s", logLabel))
		return nil
	}

	return lastErr
}
