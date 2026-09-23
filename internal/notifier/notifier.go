package notifier

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/proxy"
)

// NotificationSender defines the interface for dispatching notification payloads.
type NotificationSender interface {
	Send(ctx context.Context, payload BarkPayload) error
}

// Notifier dispatches alerts to push notification channels (Bark, FCM, Webhook, etc.)
type Notifier struct {
	mu      sync.RWMutex
	cfg     config.NotificationConfig
	senders []NotificationSender
	dedup   *DedupCache
}

// NewNotifier creates an initialized Notifier instance with Bark and/or FCM clients.
func NewNotifier(cfg config.NotificationConfig) *Notifier {
	n := &Notifier{
		cfg:   cfg,
		dedup: NewDedupCache(),
	}
	if cfg.BarkEndpoint != "" {
		n.senders = append(n.senders, NewBarkClient(cfg))
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
	n.senders = append(n.senders, NewFCMClient(fcmCfg))
}

// IsEnabled reports whether notifications are actively configured and ready to send.
func (n *Notifier) IsEnabled() bool {
	if n == nil || !n.cfg.Enabled {
		return false
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.senders) > 0
}

// sendToAll dispatches payload to all active notification senders in parallel.
func (n *Notifier) sendToAll(ctx context.Context, payload BarkPayload) error {
	n.mu.RLock()
	senders := make([]NotificationSender, len(n.senders))
	copy(senders, n.senders)
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
	case "permission":
		actionLower := strings.ToLower(pi.Action)
		if strings.Contains(actionLower, "command") || strings.Contains(actionLower, "run") {
			notifBody = fmt.Sprintf(MsgBodyCommand, truncateString(pi.Target, 90))
		} else if strings.Contains(actionLower, "write") || strings.Contains(actionLower, "edit") {
			notifBody = fmt.Sprintf(MsgBodyWriteFile, truncateString(pi.Target, 90))
		} else if strings.Contains(actionLower, "read") {
			notifBody = fmt.Sprintf(MsgBodyReadFile, truncateString(pi.Target, 90))
		} else if pi.Description != "" {
			notifBody = fmt.Sprintf(MsgBodyApproval, truncateString(pi.Description, 90))
		} else {
			notifBody = fmt.Sprintf(MsgBodyGenericAction, pi.Action, truncateString(pi.Target, 90))
		}
	case "ask_question":
		notifTitle = MsgTitleQuestion
		notifBody = fmt.Sprintf(MsgBodyQuestion, truncateString(pi.Title, 90))
	case "run_command":
		notifTitle = MsgTitleRunCommand
		notifBody = fmt.Sprintf(MsgBodyCommand, truncateString(pi.Target, 90))
	case "file_permission":
		notifTitle = MsgTitleFileAccess
		notifBody = fmt.Sprintf(MsgBodyAccessFile, truncateString(pi.Target, 90))
	default:
		notifBody = fmt.Sprintf(MsgBodyWaiting, truncateString(pi.Title, 90))
	}

	if title != "" && title != MsgUntitledSession {
		notifBody = fmt.Sprintf("【%s】%s", title, notifBody)
	}

	payload := BarkPayload{
		Title:    notifTitle,
		Body:     notifBody,
		Icon:     n.cfg.IconURL,
		Group:    n.cfg.Group,
		URL:      fmt.Sprintf("antigravity://cascade/%s?action=review", cascadeID),
		Level:    "timeSensitive",
		Sound:    n.cfg.SoundAction,
		Category: "antigravity_action",
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

	payload := BarkPayload{
		Title:    notifTitle,
		Body:     notifBody,
		Icon:     n.cfg.IconURL,
		Group:    n.cfg.Group,
		URL:      fmt.Sprintf("antigravity://cascade/%s", cascadeID),
		Level:    "timeSensitive",
		Sound:    n.cfg.SoundAction,
		Category: "antigravity_proceed",
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

	payload := BarkPayload{
		Title:    notifTitle,
		Body:     notifBody,
		Icon:     n.cfg.IconURL,
		Group:    n.cfg.Group,
		URL:      fmt.Sprintf("antigravity://cascade/%s", cascadeID),
		Level:    "active",
		Sound:    n.cfg.SoundComplete,
		Category: "antigravity_complete",
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

	payload := BarkPayload{
		Title:    notifTitle,
		Body:     notifBody,
		Icon:     n.cfg.IconURL,
		Group:    n.cfg.Group,
		URL:      fmt.Sprintf("antigravity://cascade/%s", cascadeID),
		Level:    "timeSensitive",
		Sound:    "failure",
		Category: "antigravity_error",
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

	payload := BarkPayload{
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

// OnTrajectoryUpdate handles a real-time trajectory snapshot from WebSocket or polling asynchronously
// to prevent blocking streaming connections if the push server experiences latency.
func (n *Notifier) OnTrajectoryUpdate(details *proxy.TrajectoryDetails) {
	if !n.IsEnabled() || details == nil || details.CascadeID == "" {
		return
	}

	// 1. Check for Pending Interaction (only while the cascade is actively RUNNING)
	if details.Status == "CASCADE_RUN_STATUS_RUNNING" && details.PendingInteraction != nil {
		cascadeID := details.CascadeID
		title := details.Title
		pi := *details.PendingInteraction
		go func() {
			if err := n.NotifyAction(cascadeID, title, &pi); err != nil {
				log.Printf("[Notifier] Async NotifyAction error for cascade %s: %v", cascadeID, err)
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
