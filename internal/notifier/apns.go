package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/config"
)

// APNsRelayAlertRequest is the JSON contract accepted by /v1/push/alert on the Cloudflare APNs relay.
type APNsRelayAlertRequest struct {
	DeviceToken       string `json:"device_token"`
	Env               string `json:"env"` // "production" | "sandbox"
	Title             string `json:"title"`
	Subtitle          string `json:"subtitle,omitempty"`
	Body              string `json:"body"`
	Sound             string `json:"sound,omitempty"`
	InterruptionLevel string `json:"interruption_level,omitempty"` // "active" | "time-sensitive" | "passive"
	ThreadID          string `json:"thread_id,omitempty"`
	CollapseID        string `json:"collapse_id,omitempty"`
	Category          string `json:"category,omitempty"`
	CascadeID         string `json:"cascade_id,omitempty"`
	URL               string `json:"url,omitempty"`
}

// APNsLiveActivityAlert carries the alert dictionary required by iOS for Push-to-Start ("start") events.
type APNsLiveActivityAlert struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Sound string `json:"sound,omitempty"`
}

// APNsLiveActivityRequest is the JSON contract accepted by /v1/push/live-activity on the Cloudflare APNs relay.
type APNsLiveActivityRequest struct {
	PushToken      string                   `json:"push_token"`
	Env            string                   `json:"env"`   // "production" | "sandbox"
	Event          string                   `json:"event"` // "start" | "update" | "end"
	Priority       int                      `json:"priority"`
	Timestamp      int64                    `json:"timestamp"`
	StaleDate      int64                    `json:"stale_date,omitempty"`
	DismissalDate  int64                    `json:"dismissal_date,omitempty"`
	AttributesType string                   `json:"attributes_type,omitempty"`
	Attributes     *LiveActivityAttributes  `json:"attributes,omitempty"`
	ContentState   LiveActivityContentState `json:"content_state"`
	Alert          *APNsLiveActivityAlert   `json:"alert,omitempty"`
	InputPushToken bool                     `json:"input_push_token,omitempty"`
}

// APNsRelayResponse is the structured response returned by the Cloudflare APNs relay Worker.
type APNsRelayResponse struct {
	OK           bool   `json:"ok"`
	Status       int    `json:"status"`
	APNsID       string `json:"apns_id,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Unregistered bool   `json:"unregistered,omitempty"`
	Error        string `json:"error,omitempty"`
}

// APNsRelayClient dispatches native iOS alerts and Live Activity updates through the stateless APNs relay Worker.
type APNsRelayClient struct {
	relayURL       string
	store          *PushDeviceStore
	client         *http.Client
	nowFunc        func() time.Time
	rateMu         sync.Mutex
	rateLimitUntil time.Time
}

// NewAPNsRelayClient creates an APNsRelayClient bound to relayURL and store.
func NewAPNsRelayClient(relayURL string, store *PushDeviceStore) *APNsRelayClient {
	relayURL = strings.TrimRight(strings.TrimSpace(relayURL), "/")
	if relayURL == "" {
		relayURL = config.DefaultAPNSRelayURL
	}
	return &APNsRelayClient{
		relayURL: relayURL,
		store:    store,
		nowFunc:  time.Now,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				MaxIdleConns:        16,
				MaxIdleConnsPerHost: 8,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Store returns the underlying PushDeviceStore.
func (c *APNsRelayClient) Store() *PushDeviceStore {
	return c.store
}

// RelayURL returns the configured relay base URL.
func (c *APNsRelayClient) RelayURL() string {
	return c.relayURL
}

// HasAlertTargets reports whether at least one iOS device is subscribed to APNs alerts.
func (c *APNsRelayClient) HasAlertTargets() bool {
	return c != nil && c.store != nil && c.store.HasAlertTargets()
}

// HasLiveActivityTargets reports whether at least one iOS device is subscribed to Live Activity pushes.
func (c *APNsRelayClient) HasLiveActivityTargets() bool {
	return c != nil && c.store != nil && c.store.HasLiveActivityTargets()
}

// Send implements NotificationSender, broadcasting an alert notification to all registered iOS devices.
func (c *APNsRelayClient) Send(ctx context.Context, payload PushPayload) error {
	if c == nil || c.store == nil {
		return nil
	}
	targets := c.store.ListAlertTargets()
	if len(targets) == 0 {
		return nil
	}

	cascadeID := strings.TrimSpace(payload.CascadeID)
	if cascadeID == "" {
		cascadeID = extractCascadeIDFromURL(payload.URL)
	}

	interruption := "active"
	if strings.EqualFold(payload.Level, "timeSensitive") || strings.EqualFold(payload.Level, "time-sensitive") {
		interruption = "time-sensitive"
	} else if strings.EqualFold(payload.Level, "passive") {
		interruption = "passive"
	}

	collapseID := ""
	if cascadeID != "" {
		collapseID = "cascade-" + cascadeID
		if len(collapseID) > 64 {
			collapseID = collapseID[:64]
		}
	}

	var (
		wg    sync.WaitGroup
		errCh = make(chan error, len(targets))
	)

	for _, target := range targets {
		wg.Add(1)
		go func(t APNsAlertTarget) {
			defer wg.Done()
			req := APNsRelayAlertRequest{
				DeviceToken:       t.Token,
				Env:               t.Env,
				Title:             truncateString(payload.Title, 80),
				Body:              truncateString(payload.Body, 240),
				Sound:             resolveAlertSound(t, payload),
				InterruptionLevel: interruption,
				ThreadID:          "multigravity",
				CollapseID:        collapseID,
				Category:          payload.Category,
				CascadeID:         cascadeID,
				URL:               payload.URL,
			}
			resp, err := c.postRelay(ctx, "/v1/push/alert", req)
			if err != nil {
				errCh <- err
				return
			}
			if resp.Unregistered {
				slog.Info("[APNs] 🧹 Clearing unregistered alert device token", "device_id", t.DeviceID, "reason", resp.Reason)
				c.store.ClearAlertToken(t.DeviceID, t.Token)
				return
			}
			if !resp.OK {
				errCh <- fmt.Errorf("apns alert rejected (%d %s)", resp.Status, resp.Reason)
				return
			}
			slog.Info(fmt.Sprintf("[APNs] 🚀 推送成功: %s", payload.Title), "device_id", t.DeviceID, "env", t.Env)
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

func (c *APNsRelayClient) now() time.Time {
	if c != nil && c.nowFunc != nil {
		return c.nowFunc()
	}
	return time.Now()
}

func (c *APNsRelayClient) isRateLimited() bool {
	c.rateMu.Lock()
	defer c.rateMu.Unlock()
	return !c.rateLimitUntil.IsZero() && c.now().Before(c.rateLimitUntil)
}

func (c *APNsRelayClient) markRateLimited(d time.Duration) {
	c.rateMu.Lock()
	defer c.rateMu.Unlock()
	until := c.now().Add(d)
	if until.After(c.rateLimitUntil) {
		c.rateLimitUntil = until
	}
}

// SendLiveActivity sends a single Live Activity start/update/end push to a specific target and cleans up stale tokens on 410.
func (c *APNsRelayClient) SendLiveActivity(ctx context.Context, target APNsLiveActivityTarget, req APNsLiveActivityRequest) error {
	if c == nil || c.store == nil {
		return nil
	}
	if req.Event != "end" && c.isRateLimited() {
		return fmt.Errorf("apns relay rate-limited, backing off")
	}
	req.PushToken = target.Token
	req.Env = target.Env

	resp, err := c.postRelay(ctx, "/v1/push/live-activity", req)
	if err != nil {
		return err
	}
	if resp.Unregistered {
		if target.IsStart {
			slog.Info("[APNs] 🧹 Clearing unregistered push-to-start token", "device_id", target.DeviceID, "reason", resp.Reason)
			c.store.ClearLiveActivityStartToken(target.DeviceID, target.Token)
		} else {
			slog.Info("[APNs] 🧹 Clearing stale Live Activity token", "device_id", target.DeviceID, "cascade_id", shortID(target.CascadeID), "reason", resp.Reason)
			c.store.RemoveLiveActivityToken(target.DeviceID, target.CascadeID)
		}
		return nil
	}
	if !resp.OK {
		return fmt.Errorf("apns live-activity %s rejected (%d %s)", req.Event, resp.Status, resp.Reason)
	}
	return nil
}

func (c *APNsRelayClient) postRelay(ctx context.Context, path string, body any) (*APNsRelayResponse, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal apns relay payload: %w", err)
	}

	endpoint := c.relayURL + path
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(700 * time.Millisecond):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("create apns relay request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			slog.Warn("[APNs] relay request failed", "path", path, "attempt", attempt+1, "err", err)
			continue
		}

		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		resp.Body.Close()

		var parsed APNsRelayResponse
		_ = json.Unmarshal(respBody, &parsed)
		if parsed.Status == 0 {
			parsed.Status = resp.StatusCode
		}

		if resp.StatusCode >= 500 && attempt == 0 {
			lastErr = fmt.Errorf("apns relay server error %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
			continue
		}

		if resp.StatusCode == http.StatusOK {
			parsed.OK = true
			return &parsed, nil
		}
		if resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode == http.StatusForbidden && parsed.Reason == "") {
			c.markRateLimited(15 * time.Second)
			if parsed.Reason == "" {
				parsed.Reason = "TooManyRequests"
			}
		}
		if resp.StatusCode == http.StatusGone || isPermanentTokenRejection(parsed.Reason) {
			parsed.Unregistered = true
		}
		return &parsed, nil
	}
	return nil, lastErr
}

func isPermanentTokenRejection(reason string) bool {
	switch strings.TrimSpace(reason) {
	case "Unregistered", "BadDeviceToken", "DeviceTokenNotForTopic", "BadEnvironmentKeyInToken", "TopicDisallowed", "ExpiredToken":
		return true
	default:
		return false
	}
}

func extractCascadeIDFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	// Expected format: antigravity://cascade/<cascadeID>?action=review
	const marker = "://cascade/"
	idx := strings.Index(rawURL, marker)
	if idx == -1 {
		return ""
	}
	rest := rawURL[idx+len(marker):]
	if q := strings.IndexAny(rest, "?#/"); q != -1 {
		rest = rest[:q]
	}
	return strings.TrimSpace(rest)
}

func resolveAlertSound(target APNsAlertTarget, payload PushPayload) string {
	var raw string
	switch payload.Category {
	case "antigravity_action", "antigravity_proceed":
		raw = target.SoundAction
		if raw == "" {
			raw = payload.Sound
		}
		if raw == "" {
			raw = "alarm"
		}
	case "antigravity_complete":
		raw = target.SoundComplete
		if raw == "" {
			raw = payload.Sound
		}
		if raw == "" {
			raw = "glass"
		}
	case "quota_alert":
		if strings.EqualFold(payload.Level, "timeSensitive") || strings.EqualFold(payload.Level, "time-sensitive") {
			raw = target.SoundAction
			if raw == "" {
				raw = payload.Sound
			}
			if raw == "" {
				raw = "alarm"
			}
		} else {
			raw = payload.Sound
		}
	default:
		raw = payload.Sound
	}
	return formatAPNSSound(raw)
}

func formatAPNSSound(raw string) string {
	clean := normalizeSoundPreference(raw)
	switch clean {
	case "", "default":
		return "default"
	case "silence", "none":
		return "silence.caf"
	case "failure":
		return "suspense.caf"
	default:
		return clean + ".caf"
	}
}
