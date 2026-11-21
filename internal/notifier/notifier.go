package notifier

import (
	"log/slog"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/proxy"
)

// PushPayload is the unified notification envelope dispatched across APNs and FCM channels.
type PushPayload struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	Icon      string `json:"icon,omitempty"`
	Group     string `json:"group,omitempty"`
	URL       string `json:"url,omitempty"`
	Level     string `json:"level,omitempty"` // "active", "timeSensitive", "passive"
	Sound     string `json:"sound,omitempty"`
	Badge     int    `json:"badge,omitempty"`
	Category  string `json:"category,omitempty"`
	CascadeID string `json:"cascade_id,omitempty"`
}

// NotificationSender defines the interface for dispatching notification payloads.
type NotificationSender interface {
	Send(ctx context.Context, payload PushPayload) error
}

// Notifier dispatches alerts to push notification channels (APNs, FCM, etc.).
type Notifier struct {
	mu           sync.RWMutex
	cfg          config.NotificationConfig
	senders      []NotificationSender
	apns         *APNsRelayClient
	liveActivity *LiveActivityDispatcher
	dedup        *DedupCache
}

// NewNotifier creates an initialized Notifier instance with configured push senders (e.g. FCM).
// Native iOS APNs is attached via AttachAPNsClient.
func NewNotifier(cfg config.NotificationConfig) *Notifier {
	n := &Notifier{
		cfg:   cfg,
		dedup: NewDedupCache(),
	}
	if cfg.FCMEnabled || (cfg.FCMServerKey != "" && cfg.FCMDeviceToken != "") {
		n.senders = append(n.senders, NewFCMClient(cfg))
	}
	return n
}

// NewNotifierWithSender creates an initialized Notifier instance with a custom sender.
func NewNotifierWithSender(cfg config.NotificationConfig, sender NotificationSender) *Notifier {
	return &Notifier{
		cfg:     cfg,
		senders: []NotificationSender{sender},
		dedup:   NewDedupCache(),
	}
}

// SetSender updates the notification sender (e.g. for testing or alternative channels).
func (n *Notifier) SetSender(sender NotificationSender) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.senders = []NotificationSender{sender}
}

// AddSender appends an additional notification sender to the dispatch chain.
func (n *Notifier) AddSender(sender NotificationSender) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.senders = append(n.senders, sender)
}

// Senders returns a snapshot of all active notification senders.
func (n *Notifier) Senders() []NotificationSender {
	n.mu.RLock()
	defer n.mu.RUnlock()
	cp := make([]NotificationSender, len(n.senders))
	copy(cp, n.senders)
	return cp
}

// UpdateFCMDeviceToken updates or registers an FCM client with the given Android registration token.
func (n *Notifier) UpdateFCMDeviceToken(token string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	for _, s := range n.senders {
		if fcm, ok := s.(*FCMClient); ok {
			fcm.SetDeviceToken(token)
			return
		}
	}
	// No FCM client currently registered, create and register one dynamically
	fcmCfg := n.cfg
	fcmCfg.FCMDeviceToken = token
	fcmCfg.FCMEnabled = true
	n.cfg.Enabled = true
	n.senders = append(n.senders, NewFCMClient(fcmCfg))
}

// AttachAPNsClient attaches an APNs relay client (and Live Activity dispatcher) for native iOS push delivery.
func (n *Notifier) AttachAPNsClient(client *APNsRelayClient, liveActivityEnabled bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.apns = client
	if client != nil {
		n.liveActivity = NewLiveActivityDispatcher(client, liveActivityEnabled)
	} else {
		n.liveActivity = nil
	}
}

// APNsClient returns the attached APNs relay client, if any.
func (n *Notifier) APNsClient() *APNsRelayClient {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.apns
}

// LiveActivity returns the Live Activity dispatcher, if any.
func (n *Notifier) LiveActivity() *LiveActivityDispatcher {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.liveActivity
}

// HasLiveActivityTargets reports whether at least one iOS device is subscribed to Live Activity pushes.
func (n *Notifier) HasLiveActivityTargets() bool {
	if n == nil {
		return false
	}
	n.mu.RLock()
	la := n.liveActivity
	n.mu.RUnlock()
	return la != nil && la.IsActive()
}

// CanActivateDynamically reports whether this Notifier can receive device push registrations at runtime
// (e.g. APNs iOS clients pairing after gateway startup).
func (n *Notifier) CanActivateDynamically() bool {
	if n == nil {
		return false
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.apns != nil
}

// IsEnabled reports whether notifications are actively configured and ready to send.
func (n *Notifier) IsEnabled() bool {
	if n == nil {
		return false
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.cfg.Enabled && len(n.senders) > 0 {
		return true
	}
	return n.apns != nil && n.apns.HasAlertTargets()
}

// sendToAll dispatches payload to all active notification senders in parallel.
func (n *Notifier) sendToAll(ctx context.Context, payload PushPayload) error {
	payload.Title = SanitizePushText(payload.Title)
	payload.Body = SanitizePushText(payload.Body)

	n.mu.RLock()
	senders := make([]NotificationSender, 0, len(n.senders)+1)
	if n.cfg.Enabled {
		senders = append(senders, n.senders...)
	}
	if n.apns != nil && n.apns.HasAlertTargets() {
		senders = append(senders, n.apns)
	}
	n.mu.RUnlock()

	if len(senders) == 0 {
		return fmt.Errorf("no notification senders configured")
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(senders))

	for _, s := range senders {
		wg.Add(1)
		go func(sender NotificationSender) {
			defer wg.Done()
			if err := sender.Send(ctx, payload); err != nil {
				errCh <- err
			}
		}(s)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 && len(errs) == len(senders) {
		return fmt.Errorf("all notification senders failed: %w", errs[0])
	}
	return nil
}

// Dedup returns the deduplication cache.
func (n *Notifier) Dedup() *DedupCache {
	return n.dedup
}

// NotifyAction sends a high-priority alert when the agent requires user permission or answers.
func (n *Notifier) NotifyAction(cascadeID, title string, pi *proxy.PendingInteraction) error {
	if !n.IsEnabled() || pi == nil {
		return nil
	}

	dedupKey := fmt.Sprintf("action:%s:%d:%s", cascadeID, pi.StepIndex, pi.Type)
	if !n.dedup.TryNotify(dedupKey, 2*time.Hour) {
		return nil
	}

	notifTitle := MsgTitleApproval
	var notifBody string

	switch pi.Type {
	case "workflow_approval":
		notifTitle = "⚠️ 动态工作流需审批"
		if pi.WorkflowApproval != nil && pi.WorkflowApproval.Description != "" {
			notifBody = fmt.Sprintf("编排工作流申请执行：%s", truncateString(SanitizePushText(pi.WorkflowApproval.Description), 90))
		} else {
			notifBody = fmt.Sprintf("动态工作流申请执行：%s", truncateString(SanitizePushText(pi.Title), 90))
		}
	case "permission":
		actionLower := strings.ToLower(pi.Action)
		cleanTarget := truncateString(SanitizePushText(pi.Target), 90)
		if strings.Contains(actionLower, "command") || strings.Contains(actionLower, "run") {
			notifBody = fmt.Sprintf(MsgBodyCommand, cleanTarget)
		} else if strings.Contains(actionLower, "write") || strings.Contains(actionLower, "edit") {
			notifBody = fmt.Sprintf(MsgBodyWriteFile, cleanTarget)
		} else if strings.Contains(actionLower, "read") {
			notifBody = fmt.Sprintf(MsgBodyReadFile, cleanTarget)
		} else if pi.Description != "" {
			notifBody = fmt.Sprintf(MsgBodyApproval, truncateString(SanitizePushText(pi.Description), 90))
		} else {
			notifBody = fmt.Sprintf(MsgBodyGenericAction, SanitizePushText(pi.Action), cleanTarget)
		}
	case "ask_question":
		notifTitle = MsgTitleQuestion
		notifBody = fmt.Sprintf(MsgBodyQuestion, truncateString(SanitizePushText(pi.Title), 90))
	case "run_command":
		notifTitle = MsgTitleRunCommand
		notifBody = fmt.Sprintf(MsgBodyCommand, truncateString(SanitizePushText(pi.Target), 90))
	case "file_permission":
		notifTitle = MsgTitleFileAccess
		notifBody = fmt.Sprintf(MsgBodyAccessFile, truncateString(SanitizePushText(pi.Target), 90))
	default:
		notifBody = fmt.Sprintf(MsgBodyWaiting, truncateString(SanitizePushText(pi.Title), 90))
	}

	cleanSessionTitle := SanitizePushText(title)
	if cleanSessionTitle != "" && cleanSessionTitle != MsgUntitledSession {
		notifBody = fmt.Sprintf("【%s】%s", truncateString(cleanSessionTitle, 60), notifBody)
	}

	payload := PushPayload{
		Title:     notifTitle,
		Body:      notifBody,
		Icon:      n.cfg.IconURL,
		Group:     n.cfg.Group,
		URL:       fmt.Sprintf("antigravity://cascade/%s?action=review", cascadeID),
		Level:     "timeSensitive",
		Sound:     n.cfg.SoundAction,
		Category:  "antigravity_action",
		CascadeID: cascadeID,
	}

	if err := n.sendToAll(context.Background(), payload); err != nil {
		n.dedup.Remove(dedupKey)
		return err
	}
	return nil
}

// NotifyProceed sends an alert when an implementation plan has completed and waits for Proceed.
func (n *Notifier) NotifyProceed(cascadeID, title string, totalSteps int) error {
	if !n.IsEnabled() {
		return nil
	}

	dedupKey := fmt.Sprintf("proceed:%s:%d", cascadeID, totalSteps)
	if !n.dedup.TryNotify(dedupKey, 2*time.Hour) {
		return nil
	}

	notifTitle := MsgTitleProceed
	displayTitle := title
	if displayTitle == "" || displayTitle == MsgUntitledSession {
		displayTitle = MsgDefaultPlan
	}
	notifBody := fmt.Sprintf(MsgBodyProceed, displayTitle)

	payload := PushPayload{
		Title:     notifTitle,
		Body:      notifBody,
		Icon:      n.cfg.IconURL,
		Group:     n.cfg.Group,
		URL:       fmt.Sprintf("antigravity://cascade/%s", cascadeID),
		Level:     "timeSensitive",
		Sound:     n.cfg.SoundAction,
		Category:  "antigravity_proceed",
		CascadeID: cascadeID,
	}

	if err := n.sendToAll(context.Background(), payload); err != nil {
		n.dedup.Remove(dedupKey)
		return err
	}
	return nil
}

// NotifyCompleted sends a notification when a cascade completes all steps successfully.
func (n *Notifier) NotifyCompleted(cascadeID, title string, totalSteps int) error {
	if !n.IsEnabled() {
		return nil
	}

	dedupKey := fmt.Sprintf("done:%s:%d", cascadeID, totalSteps)
	if !n.dedup.TryNotify(dedupKey, 2*time.Hour) {
		return nil
	}

	notifTitle := MsgTitleCompleted
	displayTitle := title
	if displayTitle == "" || displayTitle == MsgUntitledSession {
		displayTitle = MsgDefaultTask
	}
	notifBody := fmt.Sprintf(MsgBodyCompleted, displayTitle, totalSteps)

	payload := PushPayload{
		Title:     notifTitle,
		Body:      notifBody,
		Icon:      n.cfg.IconURL,
		Group:     n.cfg.Group,
		URL:       fmt.Sprintf("antigravity://cascade/%s", cascadeID),
		Level:     "active",
		Sound:     n.cfg.SoundComplete,
		Category:  "antigravity_complete",
		CascadeID: cascadeID,
	}

	if err := n.sendToAll(context.Background(), payload); err != nil {
		n.dedup.Remove(dedupKey)
		return err
	}
	return nil
}

// NotifyFailed sends a notification when a cascade fails or terminates abnormally.
func (n *Notifier) NotifyFailed(cascadeID, title string, totalSteps int) error {
	if !n.IsEnabled() {
		return nil
	}

	dedupKey := fmt.Sprintf("fail:%s:%d", cascadeID, totalSteps)
	if !n.dedup.TryNotify(dedupKey, 2*time.Hour) {
		return nil
	}

	notifTitle := MsgTitleFailed
	displayTitle := title
	if displayTitle == "" || displayTitle == MsgUntitledSession {
		displayTitle = MsgDefaultTask
	}
	notifBody := fmt.Sprintf(MsgBodyFailed, displayTitle)

	payload := PushPayload{
		Title:     notifTitle,
		Body:      notifBody,
		Icon:      n.cfg.IconURL,
		Group:     n.cfg.Group,
		URL:       fmt.Sprintf("antigravity://cascade/%s", cascadeID),
		Level:     "timeSensitive",
		Sound:     "failure",
		Category:  "antigravity_error",
		CascadeID: cascadeID,
	}

	if err := n.sendToAll(context.Background(), payload); err != nil {
		n.dedup.Remove(dedupKey)
		return err
	}
	return nil
}

// NotifyCockpitAlert sends a notification when Cockpit Tools fails to respond after launch retries.
func (n *Notifier) NotifyCockpitAlert(title, message string) error {
	if !n.IsEnabled() {
		return nil
	}

	dedupKey := "cockpit:offline"
	if !n.dedup.TryNotify(dedupKey, 1*time.Hour) {
		return nil
	}

	if title == "" {
		title = MsgTitleCockpitOffline
	}
	if message == "" {
		message = MsgBodyCockpitOffline
	}

	payload := PushPayload{
		Title:    title,
		Body:     message,
		Icon:     n.cfg.IconURL,
		Group:    n.cfg.Group,
		Level:    "timeSensitive",
		Sound:    "failure",
		Category: "cockpit_alert",
	}

	if err := n.sendToAll(context.Background(), payload); err != nil {
		n.dedup.Remove(dedupKey)
		return err
	}
	return nil
}

// NotifyQuotaAlert pushes a low-quota alert. key identifies the (account, reset cycle) so a flaky sender retry
// cannot double-send; the alerter itself already guarantees one push per level per cycle.
func (n *Notifier) NotifyQuotaAlert(key, title, body string, critical bool) error {
	if !n.IsEnabled() {
		return nil
	}
	dedupKey := "quota:" + key
	if critical {
		dedupKey += ":critical"
	}
	if !n.dedup.TryNotify(dedupKey, 6*time.Hour) {
		return nil
	}

	payload := PushPayload{
		Title:    title,
		Body:     body,
		Icon:     n.cfg.IconURL,
		Group:    n.cfg.Group,
		Level:    "active",
		Category: "quota_alert",
	}
	if critical {
		// 紧急一档：时效性通知 + 告警音，能穿透专注模式（但不用 critical，那会无视静音开关）
		payload.Level = "timeSensitive"
		payload.Sound = n.cfg.SoundAction
	}
	if err := n.sendToAll(context.Background(), payload); err != nil {
		n.dedup.Remove(dedupKey)
		return err
	}
	return nil
}

// OnTrajectoryUpdate handles a real-time trajectory snapshot from WebSocket or polling asynchronously
// to prevent blocking streaming connections if the push server experiences latency.
func (n *Notifier) OnTrajectoryUpdate(details *proxy.TrajectoryDetails) {
	if details == nil || details.CascadeID == "" {
		return
	}

	// Sync Live Activity state via per-cascade single-slot latest-wins coalescer (excluding subagents).
	if n.HasLiveActivityTargets() && details.Status == "CASCADE_RUN_STATUS_RUNNING" && strings.TrimSpace(details.ParentConversation) == "" {
		if la := n.LiveActivity(); la != nil {
			la.EnqueueLatest(details)
		}
	}

	if !n.IsEnabled() {
		return
	}

	// 1. Check for Pending Interaction (only while the cascade is actively RUNNING)
	if details.Status == "CASCADE_RUN_STATUS_RUNNING" && details.PendingInteraction != nil {
		dedupKey := fmt.Sprintf("action:%s:%d:%s", details.CascadeID, details.PendingInteraction.StepIndex, details.PendingInteraction.Type)
		if n.dedup.IsNotified(dedupKey, 2*time.Hour) {
			return
		}
		cascadeID := details.CascadeID
		title := details.Title
		pi := *details.PendingInteraction
		go func() {
			if err := n.NotifyAction(cascadeID, title, &pi); err != nil {
				slog.Warn(fmt.Sprintf("[Notifier] Async NotifyAction error for cascade %s", cascadeID), "err", err)
			}
		}()
		return
	}

	// Terminal completion (Completed/Failed) and Plan Proceed notifications are handled
	// strictly by the background Watcher's state transition engine (detecting RUNNING -> IDLE/COMPLETED).
	// This avoids spurious notifications when users open or view historical/idle conversations.
}

func truncateString(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}
