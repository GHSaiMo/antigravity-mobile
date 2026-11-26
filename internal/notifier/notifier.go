package notifier

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/proxy"
)

const (
	cacheMissAlertMinStreak       = 3
	cacheMissAlertMinTokens int64 = 120000
	cacheMissAlertCooldown        = 1 * time.Hour
)

// PushPayload is the unified notification envelope dispatched across APNs and FCM channels.
type PushPayload struct {
	Title       string `json:"title"`
	Body        string `json:"body"`
	Icon        string `json:"icon,omitempty"`
	Group       string `json:"group,omitempty"`
	URL         string `json:"url,omitempty"`
	Level       string `json:"level,omitempty"` // "active", "timeSensitive", "passive"
	Sound       string `json:"sound,omitempty"`
	Badge       int    `json:"badge,omitempty"`
	Category    string `json:"category,omitempty"`
	CascadeID   string `json:"cascade_id,omitempty"`
	GatewayID   string `json:"gateway_id,omitempty"`
	GatewayName string `json:"gateway_name,omitempty"`
	Silent      bool   `json:"silent,omitempty"`
	Action      string `json:"action,omitempty"`
	// Localize optionally renders Title and Body for a specific device language ("zh"/"en"/"ja").
	// Ignored by JSON serialization and legacy senders that read Title/Body directly.
	Localize func(lang string) (title, body string) `json:"-"`
}

// NotificationSender defines the interface for dispatching notification payloads.
type NotificationSender interface {
	Send(ctx context.Context, payload PushPayload) error
}

// Notifier dispatches alerts to push notification channels (APNs, FCM, etc.).
type Notifier struct {
	mu               sync.RWMutex
	cfg              config.NotificationConfig
	senders          []NotificationSender
	pushStore        *PushDeviceStore
	apns             *APNsRelayClient
	liveActivity     *LiveActivityDispatcher
	dedup            *DedupCache
	alertPrefs       *AlertPreferencesStore
	cacheMissAuditor *CacheMissShadowAuditor
	gatewayID        string
	gatewayName      string
	hubRelaySender   NotificationSender
	hubLiveUpdate    func(details *proxy.TrajectoryDetails)
	hubLiveEnd       func(cascadeID, title, state string, totalSteps int)
}

// NewNotifier creates an initialized Notifier instance with configured push senders (e.g. FCM).
// Native iOS APNs is attached via AttachAPNsClient.
func NewNotifier(cfg config.NotificationConfig) *Notifier {
	n := &Notifier{
		cfg:              cfg,
		dedup:            NewDedupCache(),
		alertPrefs:       NewAlertPreferencesStore(""),
		cacheMissAuditor: NewCacheMissShadowAuditor("", ""),
	}
	if cfg.FCMEnabled || (cfg.FCMServerKey != "" && cfg.FCMDeviceToken != "") {
		n.senders = append(n.senders, NewFCMClient(cfg))
	}
	return n
}

// NewNotifierWithSender creates an initialized Notifier instance with a custom sender.
func NewNotifierWithSender(cfg config.NotificationConfig, sender NotificationSender) *Notifier {
	return &Notifier{
		cfg:              cfg,
		senders:          []NotificationSender{sender},
		dedup:            NewDedupCache(),
		alertPrefs:       NewAlertPreferencesStore(""),
		cacheMissAuditor: NewCacheMissShadowAuditor("", ""),
	}
}

// SetCacheMissAuditor attaches a persistent background shadow auditor for prompt cache miss monitoring.
func (n *Notifier) SetCacheMissAuditor(auditor *CacheMissShadowAuditor) {
	if n == nil || auditor == nil {
		return
	}
	n.mu.Lock()
	n.cacheMissAuditor = auditor
	n.mu.Unlock()
}

// CacheMissAuditor returns the active cache-miss shadow auditor, if any.
func (n *Notifier) CacheMissAuditor() *CacheMissShadowAuditor {
	if n == nil {
		return nil
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.cacheMissAuditor
}

// IsCacheMissMonitoringActive reports whether prompt cache miss monitoring (shadow audit or live alert) is active.
func (n *Notifier) IsCacheMissMonitoringActive() bool {
	if n == nil {
		return false
	}
	switch CacheMissAlertMode() {
	case CacheMissModeShadow:
		return true
	case CacheMissModeLive:
		return n.AlertPreferences().CacheMiss
	default:
		return false
	}
}

// SetAlertPreferencesStore attaches a persistent preference store to the notifier.
func (n *Notifier) SetAlertPreferencesStore(store *AlertPreferencesStore) {
	if n == nil || store == nil {
		return
	}
	n.mu.Lock()
	n.alertPrefs = store
	n.mu.Unlock()
}

// AlertPreferences returns the active push alert category switches.
func (n *Notifier) AlertPreferences() AlertPreferences {
	if n == nil {
		return DefaultAlertPreferences()
	}
	n.mu.RLock()
	store := n.alertPrefs
	n.mu.RUnlock()
	if store == nil {
		return DefaultAlertPreferences()
	}
	return store.Get()
}

// UpdateAlertPreferences applies a partial preference update and persists it.
func (n *Notifier) UpdateAlertPreferences(patch AlertPreferencesPatch) (AlertPreferences, error) {
	if n == nil {
		return DefaultAlertPreferences(), nil
	}
	n.mu.Lock()
	if n.alertPrefs == nil {
		n.alertPrefs = NewAlertPreferencesStore("")
	}
	store := n.alertPrefs
	n.mu.Unlock()
	return store.Update(patch)
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
	n.UpdateFCMDeviceTokenWithLang(token, "")
}

// UpdateFCMDeviceTokenWithLang updates or registers an FCM client with the given Android registration token and language.
func (n *Notifier) UpdateFCMDeviceTokenWithLang(token, lang string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	for _, s := range n.senders {
		if fcm, ok := s.(*FCMClient); ok {
			fcm.SetDeviceToken(token)
			fcm.SetDeviceLang(lang)
			return
		}
	}
	// No FCM client currently registered, create and register one dynamically
	fcmCfg := n.cfg
	fcmCfg.FCMDeviceToken = token
	fcmCfg.FCMEnabled = true
	n.cfg.Enabled = true
	client := NewFCMClient(fcmCfg)
	client.SetDeviceLang(lang)
	n.senders = append(n.senders, client)
}

// UpdateFCMDeviceLang updates the preferred notification language on all registered FCM clients.
func (n *Notifier) UpdateFCMDeviceLang(lang string) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	for _, s := range n.senders {
		if fcm, ok := s.(*FCMClient); ok {
			fcm.SetDeviceLang(lang)
		}
	}
}

// SetGatewayIdentity configures the stable gateway_id and display gateway_name attached to all outgoing pushes.
func (n *Notifier) SetGatewayIdentity(gatewayID, gatewayName string) {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.gatewayID = strings.TrimSpace(gatewayID)
	n.gatewayName = strings.TrimSpace(gatewayName)
	la := n.liveActivity
	n.mu.Unlock()
	if la != nil {
		la.SetGatewayIdentity(n.gatewayID, n.gatewayName)
	}
}

// GatewayIdentity returns the configured (gatewayID, gatewayName).
func (n *Notifier) GatewayIdentity() (string, string) {
	if n == nil {
		return "", ""
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.gatewayID, n.gatewayName
}

// AttachPushStore attaches a persistent PushDeviceStore shared across APNs and multi-device FCM delivery.
func (n *Notifier) AttachPushStore(store *PushDeviceStore) {
	if n == nil || store == nil {
		return
	}
	n.mu.Lock()
	n.pushStore = store
	var fcmClient *FCMClient
	for _, s := range n.senders {
		if fc, ok := s.(*FCMClient); ok {
			fcmClient = fc
			break
		}
	}
	if fcmClient == nil && (n.cfg.FCMEnabled || store.HasFCMTargets()) {
		fcmCfg := n.cfg
		fcmCfg.FCMEnabled = true
		fcmClient = NewFCMClient(fcmCfg)
		n.senders = append(n.senders, fcmClient)
		if store.HasFCMTargets() {
			n.cfg.Enabled = true
		}
	}
	n.mu.Unlock()
	if fcmClient != nil {
		fcmClient.SetPushStore(store)
	}
}

// PushStore returns the attached persistent PushDeviceStore, if any.
func (n *Notifier) PushStore() *PushDeviceStore {
	if n == nil {
		return nil
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.pushStore != nil {
		return n.pushStore
	}
	if n.apns != nil {
		return n.apns.Store()
	}
	return nil
}

// UpsertFCMDevice registers or updates a paired Android device's FCM token and preferences in the multi-device store.
func (n *Notifier) UpsertFCMDevice(deviceID, token, lang string, notifEnabled *bool, showGatewayLabel *bool, gatewayLabel string) error {
	n.mu.Lock()
	store := n.pushStore
	if store == nil && n.apns != nil {
		store = n.apns.Store()
		n.pushStore = store
	}
	if store == nil {
		store, _ = NewPushDeviceStore("")
		n.pushStore = store
	}
	var fcmClient *FCMClient
	for _, s := range n.senders {
		if fc, ok := s.(*FCMClient); ok {
			fcmClient = fc
			break
		}
	}
	if fcmClient == nil {
		fcmCfg := n.cfg
		fcmCfg.FCMEnabled = true
		n.cfg.Enabled = true
		fcmClient = NewFCMClient(fcmCfg)
		n.senders = append(n.senders, fcmClient)
	} else {
		n.cfg.Enabled = true
	}
	n.mu.Unlock()

	fcmClient.SetPushStore(store)
	if strings.TrimSpace(token) != "" {
		if err := store.UpsertFCMToken(deviceID, token, lang, notifEnabled); err != nil {
			return err
		}
		// Keep legacy single-token field updated as well for fallback diagnostics.
		fcmClient.SetDeviceToken(token)
		fcmClient.SetDeviceLang(lang)
	} else {
		if notifEnabled != nil {
			_ = store.UpdatePreferences(deviceID, notifEnabled, nil)
		}
		if strings.TrimSpace(lang) != "" {
			_ = store.UpdateLanguage(deviceID, lang)
		}
	}
	if showGatewayLabel != nil || strings.TrimSpace(gatewayLabel) != "" {
		_ = store.UpdateGatewayLabel(deviceID, showGatewayLabel, gatewayLabel)
	}
	return nil
}

// AttachAPNsClient attaches an APNs relay client (and Live Activity dispatcher) for native iOS push delivery.
func (n *Notifier) AttachAPNsClient(client *APNsRelayClient, liveActivityEnabled bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.apns = client
	if client != nil {
		client.SetRedact(n.cfg.PushRedact)
		if n.pushStore == nil && client.Store() != nil {
			n.pushStore = client.Store()
		}
		la := NewLiveActivityDispatcher(client, liveActivityEnabled)
		la.SetGatewayIdentity(n.gatewayID, n.gatewayName)
		la.SetRedact(n.cfg.PushRedact)
		n.liveActivity = la
	} else {
		n.liveActivity = nil
	}
}

// Redacted reports whether strict push redaction (MGY_PUSH_REDACT / PUSH_REDACT / APNS_REDACT) is enabled.
func (n *Notifier) Redacted() bool {
	if n == nil {
		return false
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.cfg.PushRedact
}

// SetPushRedact updates strict push redaction dynamically across Notifier, APNs, Live Activity, and FCM senders.
func (n *Notifier) SetPushRedact(enabled bool) {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.cfg.PushRedact = enabled
	apns := n.apns
	la := n.liveActivity
	senders := append([]NotificationSender(nil), n.senders...)
	n.mu.Unlock()
	if apns != nil {
		apns.SetRedact(enabled)
	}
	if la != nil {
		la.SetRedact(enabled)
	}
	for _, s := range senders {
		if fc, ok := s.(*FCMClient); ok {
			fc.SetRedact(enabled)
		}
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

// EnableHubRelay configures a node-side relay sink so alerts and Live Activity events
// generated by this node's Watcher are forwarded upstream to a Multigravity Hub.
func (n *Notifier) EnableHubRelay(
	sender NotificationSender,
	onLiveUpdate func(details *proxy.TrajectoryDetails),
	onLiveEnd func(cascadeID, title, state string, totalSteps int),
) {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.hubRelaySender = sender
	n.hubLiveUpdate = onLiveUpdate
	n.hubLiveEnd = onLiveEnd
	n.mu.Unlock()
}

// EmitLiveUpdate forwards a live trajectory snapshot to the local Live Activity dispatcher
// and, if configured, to the upstream Hub relay hook.
func (n *Notifier) EmitLiveUpdate(details *proxy.TrajectoryDetails) {
	if n == nil || details == nil {
		return
	}
	n.mu.RLock()
	la := n.liveActivity
	hook := n.hubLiveUpdate
	n.mu.RUnlock()
	if la != nil && la.IsActive() {
		la.EnqueueLatest(details)
	}
	if hook != nil {
		snap := *details
		hook(&snap)
	}
}

// EmitLiveEnd notifies the local Live Activity dispatcher and any upstream Hub relay hook
// that a session has ended.
func (n *Notifier) EmitLiveEnd(ctx context.Context, cascadeID, title, state string, totalSteps int) {
	if n == nil {
		return
	}
	n.mu.RLock()
	la := n.liveActivity
	hook := n.hubLiveEnd
	n.mu.RUnlock()
	if la != nil && la.IsActive() {
		la.EndSession(ctx, cascadeID, title, state, totalSteps)
	}
	if hook != nil {
		hook(cascadeID, title, state, totalSteps)
	}
}

// DispatchForwardedPush delivers a push notification relayed from a remote LAN node
// through this Hub's paired APNs/FCM senders, tagging it with the Hub's gateway identity.
func (n *Notifier) DispatchForwardedPush(ctx context.Context, payload PushPayload) error {
	if n == nil || !n.IsEnabled() {
		return nil
	}
	// Always attach the Hub's own gateway identity so tapping the push routes via Hub.
	n.mu.RLock()
	hubGWID := n.gatewayID
	hubGWName := n.gatewayName
	n.mu.RUnlock()
	if hubGWID != "" {
		payload.GatewayID = hubGWID
	}
	if payload.GatewayName == "" {
		payload.GatewayName = hubGWName
	}
	return n.sendToAll(ctx, payload)
}

// HasLiveActivityTargets reports whether at least one iOS device is subscribed to Live Activity pushes
// or a Hub relay hook is active to forward Live Activity updates upstream.
func (n *Notifier) HasLiveActivityTargets() bool {
	if n == nil {
		return false
	}
	n.mu.RLock()
	la := n.liveActivity
	hasHook := n.hubLiveUpdate != nil
	n.mu.RUnlock()
	return (la != nil && la.IsActive()) || hasHook
}

// CanActivateDynamically reports whether this Notifier can receive device push registrations at runtime
// (e.g. APNs iOS or FCM Android clients pairing after gateway startup).
func (n *Notifier) CanActivateDynamically() bool {
	if n == nil {
		return false
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.apns != nil || n.pushStore != nil || n.hubRelaySender != nil
}

// IsEnabled reports whether notifications are actively configured and ready to send.
func (n *Notifier) IsEnabled() bool {
	if n == nil {
		return false
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.hubRelaySender != nil {
		return true
	}
	if n.cfg.Enabled && len(n.senders) > 0 {
		return true
	}
	if n.pushStore != nil && n.pushStore.HasFCMTargets() && len(n.senders) > 0 {
		return true
	}
	return n.apns != nil && n.apns.HasAlertTargets()
}

func appendGatewayQueryParam(rawURL, gatewayID string) string {
	rawURL = strings.TrimSpace(rawURL)
	gatewayID = strings.TrimSpace(gatewayID)
	if rawURL == "" || gatewayID == "" {
		return rawURL
	}
	if strings.Contains(rawURL, "gw=") {
		return rawURL
	}
	if strings.Contains(rawURL, "?") {
		return rawURL + "&gw=" + gatewayID
	}
	return rawURL + "?gw=" + gatewayID
}

// sendToAll dispatches payload to all active notification senders in parallel.
func (n *Notifier) sendToAll(ctx context.Context, payload PushPayload) error {
	payload.Title = SanitizePushText(payload.Title)
	payload.Body = SanitizePushText(payload.Body)

	n.mu.RLock()
	gwID := n.gatewayID
	gwName := n.gatewayName
	redacted := n.cfg.PushRedact
	senders := make([]NotificationSender, 0, len(n.senders)+1)
	if n.cfg.Enabled || (n.pushStore != nil && n.pushStore.HasFCMTargets()) {
		senders = append(senders, n.senders...)
	}
	if n.apns != nil && n.apns.HasAlertTargets() {
		senders = append(senders, n.apns)
	}
	if n.hubRelaySender != nil {
		senders = append(senders, n.hubRelaySender)
	}
	n.mu.RUnlock()

	if payload.GatewayID == "" {
		payload.GatewayID = gwID
	}
	if redacted {
		payload.GatewayName = ""
	} else if payload.GatewayName == "" {
		payload.GatewayName = gwName
	}
	if payload.GatewayID != "" && payload.URL != "" {
		payload.URL = appendGatewayQueryParam(payload.URL, payload.GatewayID)
	}

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

	piCopy := *pi
	if pi.WorkflowApproval != nil {
		wa := *pi.WorkflowApproval
		piCopy.WorkflowApproval = &wa
	}
	if pi.ApprovalPreview != nil {
		ap := *pi.ApprovalPreview
		piCopy.ApprovalPreview = &ap
	}
	cleanSessionTitle := SanitizePushText(title)
	isHighRisk := piCopy.ApprovalPreview != nil && piCopy.ApprovalPreview.RiskLevel == "high" && piCopy.Type != "ask_question"
	redacted := n.Redacted()
	buildActionCopy := func(lang string) (string, string) {
		nt := msgTitleApprovalFor(lang)
		var nb string
		redactedKind := piCopy.Type
		switch piCopy.Type {
		case "workflow_approval":
			nt = msgTitleWorkflowApprovalFor(lang)
			if !redacted {
				if piCopy.WorkflowApproval != nil && piCopy.WorkflowApproval.Description != "" {
					nb = msgBodyWorkflowDescFor(lang, truncateString(SanitizePushText(piCopy.WorkflowApproval.Description), 90))
				} else {
					nb = msgBodyWorkflowTitleFor(lang, truncateString(SanitizePushText(piCopy.Title), 90))
				}
			}
		case "permission":
			actionLower := strings.ToLower(piCopy.Action)
			if strings.Contains(actionLower, "command") || strings.Contains(actionLower, "run") {
				redactedKind = "command"
				if !redacted {
					nb = msgBodyCommandFor(lang, truncateString(SanitizePushText(piCopy.Target), 90))
				}
			} else if strings.Contains(actionLower, "write") || strings.Contains(actionLower, "edit") {
				redactedKind = "file"
				if !redacted {
					nb = msgBodyWriteFileFor(lang, truncateString(SanitizePushText(piCopy.Target), 90))
				}
			} else if strings.Contains(actionLower, "read") {
				redactedKind = "file"
				if !redacted {
					nb = msgBodyReadFileFor(lang, truncateString(SanitizePushText(piCopy.Target), 90))
				}
			} else if !redacted {
				if piCopy.Description != "" {
					nb = msgBodyApprovalFor(lang, truncateString(SanitizePushText(piCopy.Description), 90))
				} else {
					nb = msgBodyGenericActionFor(lang, SanitizePushText(piCopy.Action), truncateString(SanitizePushText(piCopy.Target), 90))
				}
			}
		case "ask_question":
			nt = msgTitleQuestionFor(lang)
			if !redacted {
				nb = msgBodyQuestionFor(lang, truncateString(SanitizePushText(piCopy.Title), 90))
			}
		case "run_command":
			nt = msgTitleRunCommandFor(lang)
			if !redacted {
				nb = msgBodyCommandFor(lang, truncateString(SanitizePushText(piCopy.Target), 90))
			}
		case "file_permission":
			nt = msgTitleFileAccessFor(lang)
			if !redacted {
				nb = msgBodyAccessFileFor(lang, truncateString(SanitizePushText(piCopy.Target), 90))
			}
		default:
			if !redacted {
				nb = msgBodyWaitingFor(lang, truncateString(SanitizePushText(piCopy.Title), 90))
			}
		}
		if isHighRisk {
			nt = msgTitleHighRiskApprovalFor(lang)
		}
		if redacted {
			return nt, msgRedactedActionBodyFor(lang, redactedKind, isHighRisk)
		}
		if cleanSessionTitle != "" && cleanSessionTitle != MsgUntitledSession {
			nb = formatSessionPrefixFor(lang, truncateString(cleanSessionTitle, 60), nb)
		}
		return nt, nb
	}

	category := "antigravity_action"
	if isHighRisk {
		category = "antigravity_action_high"
	}
	notifTitle, notifBody := buildActionCopy("")

	payload := PushPayload{
		Title:     notifTitle,
		Body:      notifBody,
		Icon:      n.cfg.IconURL,
		Group:     n.cfg.Group,
		URL:       fmt.Sprintf("antigravity://cascade/%s?action=review", cascadeID),
		Level:     "timeSensitive",
		Sound:     n.cfg.SoundAction,
		Category:  category,
		CascadeID: cascadeID,
		Localize:  buildActionCopy,
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

	redacted := n.Redacted()
	buildProceedCopy := func(lang string) (string, string) {
		if redacted {
			return msgTitleProceedFor(lang), msgRedactedProceedBodyFor(lang, totalSteps)
		}
		displayTitle := title
		if displayTitle == "" || displayTitle == MsgUntitledSession {
			displayTitle = msgDefaultPlanFor(lang)
		}
		return msgTitleProceedFor(lang), msgBodyProceedFor(lang, displayTitle)
	}
	notifTitle, notifBody := buildProceedCopy("")

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
		Localize:  buildProceedCopy,
	}

	if err := n.sendToAll(context.Background(), payload); err != nil {
		n.dedup.Remove(dedupKey)
		return err
	}
	return nil
}

// NotifyDismissApproval sends a silent background push instructing paired mobile clients
// to clear already-delivered approval/proceed banners for cascadeID after resolution on desktop or another device.
func (n *Notifier) NotifyDismissApproval(cascadeID, dismissKey string) error {
	cascadeID = strings.TrimSpace(cascadeID)
	if !n.IsEnabled() || cascadeID == "" {
		return nil
	}

	keySuffix := strings.TrimSpace(dismissKey)
	if keySuffix == "" {
		keySuffix = "latest"
	}
	dedupKey := fmt.Sprintf("dismiss:%s:%s", cascadeID, keySuffix)
	if !n.dedup.TryNotify(dedupKey, 2*time.Hour) {
		return nil
	}

	payload := PushPayload{
		URL:       fmt.Sprintf("antigravity://cascade/%s", cascadeID),
		Category:  "dismiss_approval",
		CascadeID: cascadeID,
		Silent:    true,
		Action:    "dismiss_approval",
	}

	if err := n.sendToAll(context.Background(), payload); err != nil {
		n.dedup.Remove(dedupKey)
		return err
	}
	return nil
}

// NotifyCompleted sends a notification when a cascade completes all steps successfully.
func (n *Notifier) NotifyCompleted(cascadeID, title string, totalSteps int) error {
	if !n.IsEnabled() || !n.AlertPreferences().TaskCompletion {
		return nil
	}

	dedupKey := fmt.Sprintf("done:%s:%d", cascadeID, totalSteps)
	if !n.dedup.TryNotify(dedupKey, 2*time.Hour) {
		return nil
	}

	redacted := n.Redacted()
	buildCompletedCopy := func(lang string) (string, string) {
		if redacted {
			return msgTitleCompletedFor(lang), msgRedactedCompletedBodyFor(lang, totalSteps)
		}
		displayTitle := title
		if displayTitle == "" || displayTitle == MsgUntitledSession {
			displayTitle = msgDefaultTaskFor(lang)
		}
		return msgTitleCompletedFor(lang), msgBodyCompletedFor(lang, displayTitle, totalSteps)
	}
	notifTitle, notifBody := buildCompletedCopy("")

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
		Localize:  buildCompletedCopy,
	}

	if err := n.sendToAll(context.Background(), payload); err != nil {
		n.dedup.Remove(dedupKey)
		return err
	}
	return nil
}

// NotifyFailed sends a notification when a cascade fails or terminates abnormally.
func (n *Notifier) NotifyFailed(cascadeID, title string, totalSteps int) error {
	if !n.IsEnabled() || !n.AlertPreferences().TaskCompletion {
		return nil
	}

	dedupKey := fmt.Sprintf("fail:%s:%d", cascadeID, totalSteps)
	if !n.dedup.TryNotify(dedupKey, 2*time.Hour) {
		return nil
	}

	redacted := n.Redacted()
	buildFailedCopy := func(lang string) (string, string) {
		if redacted {
			return msgTitleFailedFor(lang), msgRedactedFailedBodyFor(lang)
		}
		displayTitle := title
		if displayTitle == "" || displayTitle == MsgUntitledSession {
			displayTitle = msgDefaultTaskFor(lang)
		}
		return msgTitleFailedFor(lang), msgBodyFailedFor(lang, displayTitle)
	}
	notifTitle, notifBody := buildFailedCopy("")

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
		Localize:  buildFailedCopy,
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

	rawTitle, rawMessage := title, message
	redacted := n.Redacted()
	buildCockpitCopy := func(lang string) (string, string) {
		if redacted {
			return msgTitleCockpitOfflineFor(lang), msgRedactedCockpitBodyFor(lang)
		}
		t := rawTitle
		if t == "" {
			t = msgTitleCockpitOfflineFor(lang)
		}
		m := rawMessage
		if m == "" {
			m = msgBodyCockpitOfflineFor(lang)
		}
		return t, m
	}
	notifTitle, notifBody := buildCockpitCopy("")

	payload := PushPayload{
		Title:    notifTitle,
		Body:     notifBody,
		Icon:     n.cfg.IconURL,
		Group:    n.cfg.Group,
		Level:    "timeSensitive",
		Sound:    "failure",
		Category: "cockpit_alert",
		Localize: buildCockpitCopy,
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
	level := "warn"
	if critical {
		level = "critical"
	}
	return n.notifyQuotaAlertWithLevel(level, key, title, body, critical, nil)
}

func (n *Notifier) notifyQuotaAlert(key, title, body string, critical bool, localize func(lang string) (string, string)) error {
	level := "warn"
	if critical {
		level = "critical"
	}
	return n.notifyQuotaAlertWithLevel(level, key, title, body, critical, localize)
}

func (n *Notifier) notifyQuotaAlertWithLevel(level, key, title, body string, critical bool, localize func(lang string) (string, string)) error {
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
	if n.Redacted() {
		normLevel := strings.ToLower(strings.TrimSpace(level))
		title, body = msgRedactedQuotaCopyFor("", normLevel)
		localize = func(lang string) (string, string) {
			return msgRedactedQuotaCopyFor(lang, normLevel)
		}
	}

	payload := PushPayload{
		Title:    title,
		Body:     body,
		Icon:     n.cfg.IconURL,
		Group:    n.cfg.Group,
		URL:      "antigravity://quota",
		Level:    "active",
		Category: "quota_alert",
		Localize: localize,
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

// NotifyQuotaAlertWithLevel filters a quota alert against user-configured category preferences
// ("warn"/"critical" -> quota_low, "bonus_reset" -> quota_bonus_reset, "anomaly" -> quota_anomaly).
func (n *Notifier) NotifyQuotaAlertWithLevel(level, key, title, body string) error {
	return n.NotifyQuotaAlertLocalized(level, key, title, body, nil)
}

// NotifyQuotaAlertLocalized behaves like NotifyQuotaAlertWithLevel and optionally localizes title/body per recipient device.
func (n *Notifier) NotifyQuotaAlertLocalized(level, key, title, body string, localize func(lang string) (string, string)) error {
	prefs := n.AlertPreferences()
	normLevel := strings.ToLower(strings.TrimSpace(level))
	switch normLevel {
	case "bonus_reset":
		if !prefs.QuotaBonusReset {
			return nil
		}
	case "anomaly":
		if !prefs.QuotaAnomaly {
			return nil
		}
	default:
		if !prefs.QuotaLow {
			return nil
		}
	}
	return n.notifyQuotaAlertWithLevel(normLevel, key, title, body, normLevel == "critical", localize)
}

func pickModelName(details *proxy.TrajectoryDetails, meter *proxy.ContextMeterItem) string {
	if meter != nil && strings.TrimSpace(meter.ModelName) != "" {
		return strings.TrimSpace(meter.ModelName)
	}
	if details != nil {
		for _, candidate := range []string{details.ActiveModelName, details.ModelDisplayName, details.ActiveModel} {
			if s := strings.TrimSpace(candidate); s != "" {
				return s
			}
		}
	}
	return ""
}

func buildCacheMissObservation(
	cascadeID, title, model, source string,
	stepCount int,
	meter *proxy.ContextMeterItem,
	now time.Time,
) CacheMissObservation {
	obs := CacheMissObservation{
		CascadeID:    strings.TrimSpace(cascadeID),
		SessionTitle: title,
		Model:        model,
		Source:       source,
		StepCount:    stepCount,
		Now:          now,
	}
	if meter != nil {
		obs.Streak = meter.CacheMissStreak
		obs.MissTokens = meter.CacheMissTokens
		obs.UsedTokens = meter.UsedTokens
		obs.MaxTokens = meter.MaxTokens
		obs.ContextPct = meter.Percentage
		obs.InputTokens = meter.InputTokens
		obs.CacheReadTokens = meter.CacheReadTokens
		obs.TTFTSeconds = meter.TimeToFirstToken
		if meter.CacheHitRate != nil {
			hr := *meter.CacheHitRate
			obs.CacheHitRate = &hr
		}
	}
	return obs
}

// ObserveCacheMiss feeds a real-time prompt-cache telemetry sample into the shadow auditor.
// In default "shadow" mode, it records triggers, peak miss metrics, and self-recovery outcomes
// to background storage & the Private Docs report without sending any user-facing push notification.
// In explicit "live" mode (CACHE_MISS_ALERT_MODE=live), it also dispatches deduplicated push alerts.
func (n *Notifier) ObserveCacheMiss(obs CacheMissObservation) error {
	if n == nil {
		return nil
	}
	obs.CascadeID = strings.TrimSpace(obs.CascadeID)
	mode := CacheMissAlertMode()
	if mode == CacheMissModeOff || obs.CascadeID == "" {
		return nil
	}
	if mode == CacheMissModeLive && !n.AlertPreferences().CacheMiss {
		return nil
	}
	obs.Mode = mode

	if aud := n.CacheMissAuditor(); aud != nil {
		isNew, wouldNotify := aud.Observe(obs)
		if isNew && mode == CacheMissModeShadow {
			slog.Info("[CacheMissShadow] 🕵️ 影子审计命中（仅后台记录，不向用户推送）",
				"cascade", shortID(obs.CascadeID),
				"title", truncateString(SanitizePushText(obs.SessionTitle), 40),
				"model", normalizeAuditModel(obs.Model),
				"streak", obs.Streak,
				"miss_tokens", formatCompactTokens(obs.MissTokens),
				"would_have_notified", wouldNotify,
				"report", aud.ReportPath(),
			)
		}
	}

	if mode != CacheMissModeLive {
		return nil
	}
	if obs.Streak < cacheMissAlertMinStreak || obs.MissTokens < cacheMissAlertMinTokens {
		return nil
	}
	return n.sendCacheMissPush(obs.CascadeID, obs.SessionTitle, obs.Streak, obs.MissTokens)
}

// FinalizeCacheMissSession closes any active shadow audit episode when a session leaves RUNNING.
func (n *Notifier) FinalizeCacheMissSession(cascadeID, terminalStatus string, totalSteps int, now time.Time) {
	if n == nil {
		return
	}
	if aud := n.CacheMissAuditor(); aud != nil {
		aud.FinalizeSession(cascadeID, terminalStatus, totalSteps, now)
	}
}

// NotifyCacheMissAlert handles a cache-miss alert trigger.
// Under the default "shadow" mode, it records the trigger in the background shadow auditor
// and suppresses all user-facing push notifications. Real pushes only fire in explicit "live" mode.
func (n *Notifier) NotifyCacheMissAlert(cascadeID, title string, streak int, missTokens int64) error {
	cascadeID = strings.TrimSpace(cascadeID)
	mode := CacheMissAlertMode()
	if mode == CacheMissModeOff || cascadeID == "" {
		return nil
	}
	if mode == CacheMissModeLive && !n.AlertPreferences().CacheMiss {
		return nil
	}
	if streak < cacheMissAlertMinStreak || missTokens < cacheMissAlertMinTokens {
		return nil
	}

	return n.ObserveCacheMiss(CacheMissObservation{
		CascadeID:    cascadeID,
		SessionTitle: title,
		Source:       "direct",
		Mode:         mode,
		Streak:       streak,
		MissTokens:   missTokens,
	})
}

func (n *Notifier) sendCacheMissPush(cascadeID, title string, streak int, missTokens int64) error {
	if !n.IsEnabled() {
		return nil
	}

	dedupKey := fmt.Sprintf("cache_miss:%s", cascadeID)
	if !n.dedup.TryNotify(dedupKey, cacheMissAlertCooldown) {
		return nil
	}

	compactTokens := formatCompactTokens(missTokens)
	cleanSessionTitle := SanitizePushText(title)
	redacted := n.Redacted()
	buildCacheMissCopy := func(lang string) (string, string) {
		nb := msgBodyCacheMissAlertFor(lang, streak, compactTokens)
		if !redacted && cleanSessionTitle != "" && cleanSessionTitle != MsgUntitledSession {
			nb = formatSessionPrefixFor(lang, truncateString(cleanSessionTitle, 60), nb)
		}
		return msgTitleCacheMissAlertFor(lang), nb
	}
	notifTitle, notifBody := buildCacheMissCopy("")

	payload := PushPayload{
		Title:     notifTitle,
		Body:      notifBody,
		Icon:      n.cfg.IconURL,
		Group:     n.cfg.Group,
		URL:       fmt.Sprintf("antigravity://cascade/%s", cascadeID),
		Level:     "timeSensitive",
		Sound:     n.cfg.SoundAction,
		Category:  "cache_miss_alert",
		CascadeID: cascadeID,
		Localize:  buildCacheMissCopy,
	}

	if err := n.sendToAll(context.Background(), payload); err != nil {
		n.dedup.Remove(dedupKey)
		return err
	}
	return nil
}

// NotifyStallAlert sends a deduplicated alert when a running session has produced no step,
// text, thinking, or plan progress for an extended period.
func (n *Notifier) NotifyStallAlert(cascadeID, title string, stalledMinutes, totalSteps int) error {
	cascadeID = strings.TrimSpace(cascadeID)
	if !n.IsEnabled() || !n.AlertPreferences().TaskStall || cascadeID == "" || stalledMinutes <= 0 {
		return nil
	}

	dedupKey := fmt.Sprintf("stall:%s:%d", cascadeID, totalSteps)
	if !n.dedup.TryNotify(dedupKey, stallAlertCooldown) {
		return nil
	}

	cleanSessionTitle := SanitizePushText(title)
	redacted := n.Redacted()
	buildStallCopy := func(lang string) (string, string) {
		nb := msgBodyStallAlertFor(lang, stalledMinutes)
		if !redacted && cleanSessionTitle != "" && cleanSessionTitle != MsgUntitledSession {
			nb = formatSessionPrefixFor(lang, truncateString(cleanSessionTitle, 60), nb)
		}
		return msgTitleStallAlertFor(lang), nb
	}
	notifTitle, notifBody := buildStallCopy("")

	payload := PushPayload{
		Title:     notifTitle,
		Body:      notifBody,
		Icon:      n.cfg.IconURL,
		Group:     n.cfg.Group,
		URL:       fmt.Sprintf("antigravity://cascade/%s", cascadeID),
		Level:     "timeSensitive",
		Sound:     n.cfg.SoundAction,
		Category:  "task_stall_alert",
		CascadeID: cascadeID,
		Localize:  buildStallCopy,
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
		n.EmitLiveUpdate(details)
	}

	// 2. Shadow-audit (or live-alert when explicitly opted in) repeated large-context prompt cache misses.
	// Runs even when push delivery is inactive so background shadow reports remain complete.
	if details.Status == "CASCADE_RUN_STATUS_RUNNING" &&
		strings.TrimSpace(details.ParentConversation) == "" &&
		details.ContextMeter != nil &&
		n.IsCacheMissMonitoringActive() {
		meetsTrigger := details.ContextMeter.CacheMissStreak >= cacheMissAlertMinStreak &&
			details.ContextMeter.CacheMissTokens >= cacheMissAlertMinTokens
		aud := n.CacheMissAuditor()
		hasOngoing := aud != nil && aud.HasOngoingEpisode(details.CascadeID)
		if meetsTrigger || hasOngoing {
			obs := buildCacheMissObservation(
				details.CascadeID,
				details.Title,
				pickModelName(details, details.ContextMeter),
				"trajectory_update",
				details.TotalSteps,
				details.ContextMeter,
				time.Now(),
			)
			go func() {
				if err := n.ObserveCacheMiss(obs); err != nil {
					slog.Warn(fmt.Sprintf("[Notifier] Async ObserveCacheMiss error for cascade %s", obs.CascadeID), "err", err)
				}
			}()
		}
	}

	if !n.IsEnabled() {
		return
	}

	// 1. Check for Pending Interaction (only while the cascade is actively RUNNING)
	if details.Status == "CASCADE_RUN_STATUS_RUNNING" && details.PendingInteraction != nil {
		dedupKey := fmt.Sprintf("action:%s:%d:%s", details.CascadeID, details.PendingInteraction.StepIndex, details.PendingInteraction.Type)
		if !n.dedup.IsNotified(dedupKey, 2*time.Hour) {
			cascadeID := details.CascadeID
			title := details.Title
			pi := *details.PendingInteraction
			go func() {
				if err := n.NotifyAction(cascadeID, title, &pi); err != nil {
					slog.Warn(fmt.Sprintf("[Notifier] Async NotifyAction error for cascade %s", cascadeID), "err", err)
				}
			}()
		}
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
