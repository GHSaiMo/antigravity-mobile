package notifier

import (
	"context"
	"fmt"
	"strings"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/proxy"
)

// Notifier dispatches alerts to push notification channels (Bark, etc.)
type Notifier struct {
	cfg   config.NotificationConfig
	bark  *BarkClient
	dedup *DedupCache
}

// NewNotifier creates an initialized Notifier instance.
func NewNotifier(cfg config.NotificationConfig) *Notifier {
	return &Notifier{
		cfg:   cfg,
		bark:  NewBarkClient(cfg),
		dedup: NewDedupCache(),
	}
}

// IsEnabled reports whether notifications are actively configured.
func (n *Notifier) IsEnabled() bool {
	return n != nil && n.cfg.Enabled && n.bark != nil && n.cfg.BarkEndpoint != ""
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

	if err := n.bark.Send(context.Background(), payload); err != nil {
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

	if err := n.bark.Send(context.Background(), payload); err != nil {
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

	if err := n.bark.Send(context.Background(), payload); err != nil {
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

	if err := n.bark.Send(context.Background(), payload); err != nil {
		n.dedup.Remove(dedupKey)
		return err
	}
	return nil
}

// OnTrajectoryUpdate handles a real-time trajectory snapshot from WebSocket or polling.
func (n *Notifier) OnTrajectoryUpdate(details *proxy.TrajectoryDetails) {
	if !n.IsEnabled() || details == nil || details.CascadeID == "" {
		return
	}

	// 1. Check for Pending Interaction
	if details.PendingInteraction != nil {
		_ = n.NotifyAction(details.CascadeID, details.Title, details.PendingInteraction)
		return
	}

	// 2. Check for Plan Proceed
	if details.CanProceed {
		_ = n.NotifyProceed(details.CascadeID, details.Title, details.TotalSteps)
		return
	}

	// 3. Check for Terminal Completion
	if (details.Status == "CASCADE_RUN_STATUS_COMPLETED" || details.Status == "CASCADE_RUN_STATUS_IDLE") && details.TotalSteps > 0 {
		_ = n.NotifyCompleted(details.CascadeID, details.Title, details.TotalSteps)
	} else if details.Status == "CASCADE_RUN_STATUS_FAILED" && details.TotalSteps > 0 {
		_ = n.NotifyFailed(details.CascadeID, details.Title, details.TotalSteps)
	}
}

func truncateString(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}
