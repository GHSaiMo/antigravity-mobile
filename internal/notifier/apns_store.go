package notifier

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/i18n"
)

const (
	maxStoredPushDevices       = 32
	maxActivityTokensPerDevice = 20
	liveActivityStartRunTTL    = 2 * time.Hour
	apnsEnvProduction          = "production"
	apnsEnvSandbox             = "sandbox"
)

// PushDeviceRecord persists APNs/FCM push tokens and preferences for a paired mobile device.
type PushDeviceRecord struct {
	DeviceID              string            `json:"device_id"`
	Platform              string            `json:"platform"` // "ios" | "android"
	APNSDeviceToken       string            `json:"apns_device_token,omitempty"`
	APNSEnv               string            `json:"apns_env,omitempty"` // "production" | "sandbox"
	FCMDeviceToken        string            `json:"fcm_device_token,omitempty"`
	PushToStartToken      string            `json:"push_to_start_token,omitempty"`
	ActivityTokens        map[string]string `json:"activity_tokens,omitempty"` // cascadeID -> update token
	ActivityUpdatedAt     map[string]int64  `json:"activity_updated_at,omitempty"`
	ActivityStartedAt     map[string]int64  `json:"activity_started_at,omitempty"` // cascadeID -> push-to-start unix ts for active working run
	NotificationsEnabled  bool              `json:"notifications_enabled"`
	LiveActivitiesEnabled bool              `json:"live_activities_enabled"`
	SoundComplete         string            `json:"sound_complete,omitempty"`
	SoundAction           string            `json:"sound_action,omitempty"`
	Lang                  string            `json:"lang,omitempty"`
	ShowGatewayLabel      bool              `json:"show_gateway_label,omitempty"`
	GatewayLabel          string            `json:"gateway_label,omitempty"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

// APNsAlertTarget identifies a registered iOS device eligible for standard APNs alert notifications.
type APNsAlertTarget struct {
	DeviceID         string
	Token            string
	Env              string
	SoundComplete    string
	SoundAction      string
	Lang             string
	ShowGatewayLabel bool
	GatewayLabel     string
}

// FCMAlertTarget identifies a registered Android device eligible for FCM push notifications.
type FCMAlertTarget struct {
	DeviceID         string
	Token            string
	Lang             string
	ShowGatewayLabel bool
	GatewayLabel     string
}

// APNsLiveActivityTarget identifies a registered iOS device Live Activity token (update or push-to-start).
type APNsLiveActivityTarget struct {
	DeviceID         string
	CascadeID        string
	Token            string
	Env              string
	Lang             string
	ShowGatewayLabel bool
	GatewayLabel     string
	IsStart          bool
}

// PushDeviceStore persists APNs push registrations to disk atomically (~/.multigravity/push_devices.json).
type PushDeviceStore struct {
	mu       sync.RWMutex
	filePath string
	devices  map[string]*PushDeviceRecord
}

// NewPushDeviceStore initializes a PushDeviceStore and loads existing entries from filePath if present.
// An empty filePath runs in memory-only mode (useful for unit tests).
func NewPushDeviceStore(filePath string) (*PushDeviceStore, error) {
	s := &PushDeviceStore{
		filePath: strings.TrimSpace(filePath),
		devices:  make(map[string]*PushDeviceRecord),
	}
	if s.filePath != "" {
		if err := s.load(); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("load push device store %s: %w", s.filePath, err)
		}
		if err := os.Chmod(s.filePath, 0600); err != nil && !os.IsNotExist(err) {
			slog.Warn("[PushStore] failed to enforce 0600 permissions", "path", s.filePath, "err", err)
		}
	}
	return s, nil
}

func normalizeAPNSEnv(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case apnsEnvSandbox, "dev", "development", "debug":
		return apnsEnvSandbox
	default:
		return apnsEnvProduction
	}
}

func isValidHexPushToken(tok string) bool {
	if len(tok) < 64 || len(tok) > 256 {
		return false
	}
	for i := 0; i < len(tok); i++ {
		c := tok[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func (s *PushDeviceStore) getOrCreateLocked(deviceID, platform, apnsEnv string) *PushDeviceRecord {
	rec, ok := s.devices[deviceID]
	if !ok {
		rec = &PushDeviceRecord{
			DeviceID:              deviceID,
			Platform:              strings.ToLower(strings.TrimSpace(platform)),
			APNSEnv:               normalizeAPNSEnv(apnsEnv),
			ActivityTokens:        make(map[string]string),
			ActivityUpdatedAt:     make(map[string]int64),
			ActivityStartedAt:     make(map[string]int64),
			NotificationsEnabled:  true,
			LiveActivitiesEnabled: true,
		}
		if rec.Platform == "" {
			rec.Platform = "ios"
		}
		if len(s.devices) >= maxStoredPushDevices {
			var oldestID string
			var oldestAt time.Time
			for id, existing := range s.devices {
				if oldestID == "" || existing.UpdatedAt.Before(oldestAt) {
					oldestID = id
					oldestAt = existing.UpdatedAt
				}
			}
			if oldestID != "" {
				delete(s.devices, oldestID)
			}
		}
		s.devices[deviceID] = rec
	} else {
		if strings.TrimSpace(apnsEnv) != "" {
			rec.APNSEnv = normalizeAPNSEnv(apnsEnv)
		}
		if rec.ActivityTokens == nil {
			rec.ActivityTokens = make(map[string]string)
		}
		if rec.ActivityUpdatedAt == nil {
			rec.ActivityUpdatedAt = make(map[string]int64)
		}
		if rec.ActivityStartedAt == nil {
			rec.ActivityStartedAt = make(map[string]int64)
		}
	}
	return rec
}

// UpsertAlertToken registers or updates an iOS device's standard APNs alert token.
func (s *PushDeviceStore) UpsertAlertToken(deviceID, platform, token, apnsEnv string, notifEnabled, liveEnabled *bool) error {
	deviceID = strings.TrimSpace(deviceID)
	token = strings.TrimSpace(token)
	if deviceID == "" {
		return fmt.Errorf("device_id is required")
	}
	if !isValidHexPushToken(token) {
		return fmt.Errorf("invalid APNs hex device token")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rec := s.getOrCreateLocked(deviceID, platform, apnsEnv)
	rec.APNSDeviceToken = token
	// Deduplicate across device records (e.g. re-pairing or switching between loopback and tunnel)
	for otherID, other := range s.devices {
		if otherID != deviceID && other.APNSDeviceToken == token {
			other.APNSDeviceToken = ""
			if other.PushToStartToken == "" && len(other.ActivityTokens) == 0 {
				delete(s.devices, otherID)
			}
		}
	}
	if notifEnabled != nil {
		rec.NotificationsEnabled = *notifEnabled
	}
	if liveEnabled != nil {
		rec.LiveActivitiesEnabled = *liveEnabled
	}
	rec.UpdatedAt = time.Now()
	return s.saveLocked()
}

// UpsertLiveActivityStartToken registers or updates an iOS device's ActivityKit push-to-start token.
func (s *PushDeviceStore) UpsertLiveActivityStartToken(deviceID, platform, token, apnsEnv string, liveEnabled *bool) error {
	deviceID = strings.TrimSpace(deviceID)
	token = strings.TrimSpace(token)
	if deviceID == "" {
		return fmt.Errorf("device_id is required")
	}
	if !isValidHexPushToken(token) {
		return fmt.Errorf("invalid Live Activity push-to-start token")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rec := s.getOrCreateLocked(deviceID, platform, apnsEnv)
	rec.PushToStartToken = token
	for otherID, other := range s.devices {
		if otherID != deviceID && other.PushToStartToken == token {
			other.PushToStartToken = ""
			if other.APNSDeviceToken == "" && len(other.ActivityTokens) == 0 {
				delete(s.devices, otherID)
			}
		}
	}
	if liveEnabled != nil {
		rec.LiveActivitiesEnabled = *liveEnabled
	}
	rec.UpdatedAt = time.Now()
	return s.saveLocked()
}

// UpsertLiveActivityToken registers or updates an active Live Activity's per-cascade update token.
func (s *PushDeviceStore) UpsertLiveActivityToken(deviceID, platform, cascadeID, token, apnsEnv string) error {
	deviceID = strings.TrimSpace(deviceID)
	cascadeID = strings.TrimSpace(cascadeID)
	token = strings.TrimSpace(token)
	if deviceID == "" || cascadeID == "" {
		return fmt.Errorf("device_id and cascade_id are required")
	}
	if !isValidHexPushToken(token) {
		return fmt.Errorf("invalid Live Activity update token")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rec := s.getOrCreateLocked(deviceID, platform, apnsEnv)
	rec.ActivityTokens[cascadeID] = token
	rec.ActivityUpdatedAt[cascadeID] = time.Now().Unix()
	for otherID, other := range s.devices {
		if otherID != deviceID && other.ActivityTokens[cascadeID] == token {
			delete(other.ActivityTokens, cascadeID)
			delete(other.ActivityUpdatedAt, cascadeID)
			delete(other.ActivityStartedAt, cascadeID)
			if other.APNSDeviceToken == "" && other.PushToStartToken == "" && len(other.ActivityTokens) == 0 {
				delete(s.devices, otherID)
			}
		}
	}
	pruneActivityTokensLocked(rec)
	rec.UpdatedAt = time.Now()
	return s.saveLocked()
}

func pruneActivityTokensLocked(rec *PushDeviceRecord) {
	if len(rec.ActivityStartedAt) > 0 {
		cutoff := time.Now().Add(-24 * time.Hour).Unix()
		for cid, ts := range rec.ActivityStartedAt {
			if ts < cutoff {
				delete(rec.ActivityStartedAt, cid)
			}
		}
	}
	if len(rec.ActivityTokens) <= maxActivityTokensPerDevice {
		return
	}
	type pair struct {
		cid string
		ts  int64
	}
	items := make([]pair, 0, len(rec.ActivityTokens))
	for cid := range rec.ActivityTokens {
		items = append(items, pair{cid: cid, ts: rec.ActivityUpdatedAt[cid]})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ts < items[j].ts })
	excess := len(items) - maxActivityTokensPerDevice
	for i := 0; i < excess; i++ {
		delete(rec.ActivityTokens, items[i].cid)
		delete(rec.ActivityUpdatedAt, items[i].cid)
	}
}

func normalizeSoundPreference(raw string) string {
	clean := strings.ToLower(strings.TrimSpace(raw))
	clean = strings.TrimSuffix(clean, ".caf")
	if clean == "" {
		return ""
	}
	for i := 0; i < len(clean); i++ {
		c := clean[i]
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return ""
		}
	}
	return clean
}

// UpdatePreferences updates notification/live-activity opt-in toggles without requiring a new token.
func (s *PushDeviceStore) UpdatePreferences(deviceID string, notifEnabled, liveEnabled *bool) error {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.devices[deviceID]
	if !ok {
		return nil
	}
	if notifEnabled != nil {
		rec.NotificationsEnabled = *notifEnabled
	}
	if liveEnabled != nil {
		rec.LiveActivitiesEnabled = *liveEnabled
	}
	rec.UpdatedAt = time.Now()
	return s.saveLocked()
}

// UpdateSoundPreferences updates per-device custom notification sound preferences.
func (s *PushDeviceStore) UpdateSoundPreferences(deviceID, soundComplete, soundAction string) error {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil
	}
	sc := normalizeSoundPreference(soundComplete)
	sa := normalizeSoundPreference(soundAction)
	if sc == "" && sa == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.devices[deviceID]
	if !ok {
		return nil
	}
	changed := false
	if sc != "" && rec.SoundComplete != sc {
		rec.SoundComplete = sc
		changed = true
	}
	if sa != "" && rec.SoundAction != sa {
		rec.SoundAction = sa
		changed = true
	}
	if !changed {
		return nil
	}
	rec.UpdatedAt = time.Now()
	return s.saveLocked()
}

// UpdateLanguage records the preferred notification language ("zh", "en", or "ja") for a paired iOS device.
// Any explicit non-empty locale other than Chinese or Japanese is stored as "en" so it never falls back to Chinese.
func (s *PushDeviceStore) UpdateLanguage(deviceID, lang string) error {
	deviceID = strings.TrimSpace(deviceID)
	rawLang := strings.TrimSpace(lang)
	if deviceID == "" || rawLang == "" {
		return nil
	}
	norm := i18n.NormalizeLang(rawLang)
	if norm == "" {
		norm = i18n.LangEn
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.devices[deviceID]
	if !ok {
		return nil
	}
	if rec.Lang == norm {
		return nil
	}
	rec.Lang = norm
	rec.UpdatedAt = time.Now()
	return s.saveLocked()
}

// UpdateGatewayLabel updates whether notifications sent to deviceID should include a gateway label
// and optionally overrides the display label with the user's custom alias on that device.
func (s *PushDeviceStore) UpdateGatewayLabel(deviceID string, show *bool, label string) error {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil
	}
	cleanLabel := truncateString(SanitizePushText(label), 24)
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.devices[deviceID]
	if !ok {
		return nil
	}
	changed := false
	if show != nil && rec.ShowGatewayLabel != *show {
		rec.ShowGatewayLabel = *show
		changed = true
	}
	if strings.TrimSpace(label) != "" && rec.GatewayLabel != cleanLabel {
		rec.GatewayLabel = cleanLabel
		changed = true
	}
	if !changed {
		return nil
	}
	rec.UpdatedAt = time.Now()
	return s.saveLocked()
}

// UpsertFCMToken registers or updates an Android device's FCM registration token in the shared store.
func (s *PushDeviceStore) UpsertFCMToken(deviceID, token, lang string, notifEnabled *bool) error {
	deviceID = strings.TrimSpace(deviceID)
	token = strings.TrimSpace(token)
	if deviceID == "" {
		return fmt.Errorf("device_id is required")
	}
	if token == "" || len(token) > 4096 {
		return fmt.Errorf("invalid FCM device token")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rec := s.getOrCreateLocked(deviceID, "android", "")
	rec.Platform = "android"
	rec.FCMDeviceToken = token
	for otherID, other := range s.devices {
		if otherID != deviceID && other.FCMDeviceToken == token {
			other.FCMDeviceToken = ""
			if other.APNSDeviceToken == "" && other.PushToStartToken == "" && len(other.ActivityTokens) == 0 {
				delete(s.devices, otherID)
			}
		}
	}
	if notifEnabled != nil {
		rec.NotificationsEnabled = *notifEnabled
	}
	if rawLang := strings.TrimSpace(lang); rawLang != "" {
		norm := i18n.NormalizeLang(rawLang)
		if norm == "" {
			norm = i18n.LangEn
		}
		rec.Lang = norm
	}
	rec.UpdatedAt = time.Now()
	return s.saveLocked()
}

// ClearFCMToken removes an invalid/unregistered FCM token for a device.
func (s *PushDeviceStore) ClearFCMToken(deviceID, badToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.devices[deviceID]
	if !ok || rec.FCMDeviceToken == "" {
		return
	}
	if badToken != "" && rec.FCMDeviceToken != badToken {
		return
	}
	rec.FCMDeviceToken = ""
	rec.UpdatedAt = time.Now()
	_ = s.saveLocked()
}

// ListFCMTargets returns all Android devices with an active FCM token and notifications enabled.
func (s *PushDeviceStore) ListFCMTargets() []FCMAlertTarget {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []FCMAlertTarget
	for _, rec := range s.devices {
		if rec.NotificationsEnabled && rec.FCMDeviceToken != "" {
			out = append(out, FCMAlertTarget{
				DeviceID:         rec.DeviceID,
				Token:            rec.FCMDeviceToken,
				Lang:             i18n.NormalizeLang(rec.Lang),
				ShowGatewayLabel: rec.ShowGatewayLabel,
				GatewayLabel:     rec.GatewayLabel,
			})
		}
	}
	return out
}

// HasFCMTargets reports whether at least one Android device is subscribed to FCM notifications.
func (s *PushDeviceStore) HasFCMTargets() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, rec := range s.devices {
		if rec.NotificationsEnabled && rec.FCMDeviceToken != "" {
			return true
		}
	}
	return false
}

// RemoveLiveActivityToken deletes a specific cascade's Live Activity update token (e.g. after end or 410).
func (s *PushDeviceStore) RemoveLiveActivityToken(deviceID, cascadeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.devices[deviceID]
	if !ok || rec.ActivityTokens == nil {
		return
	}
	if _, exists := rec.ActivityTokens[cascadeID]; !exists {
		return
	}
	delete(rec.ActivityTokens, cascadeID)
	delete(rec.ActivityUpdatedAt, cascadeID)
	rec.UpdatedAt = time.Now()
	_ = s.saveLocked()
}

// MarkLiveActivityStarted persists that a Push-to-Start was sent for (deviceID, cascadeID)
// so a gateway restart or cooldown lapse will not spawn duplicate Live Activities for the same working run.
func (s *PushDeviceStore) MarkLiveActivityStarted(deviceID, cascadeID string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.devices[deviceID]
	if !ok {
		return
	}
	if rec.ActivityStartedAt == nil {
		rec.ActivityStartedAt = make(map[string]int64)
	}
	rec.ActivityStartedAt[cascadeID] = at.Unix()
	pruneActivityTokensLocked(rec)
	rec.UpdatedAt = time.Now()
	_ = s.saveLocked()
}

// ClearLiveActivityStarted clears the Push-to-Start guard for (deviceID, cascadeID).
func (s *PushDeviceStore) ClearLiveActivityStarted(deviceID, cascadeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.devices[deviceID]
	if !ok || rec.ActivityStartedAt == nil {
		return
	}
	if _, exists := rec.ActivityStartedAt[cascadeID]; !exists {
		return
	}
	delete(rec.ActivityStartedAt, cascadeID)
	rec.UpdatedAt = time.Now()
	_ = s.saveLocked()
}

// ClearLiveActivityStartedForCascade clears the Push-to-Start guard for cascadeID across all devices
// once the working run terminates, allowing future runs in the same conversation to Push-to-Start again.
func (s *PushDeviceStore) ClearLiveActivityStartedForCascade(cascadeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for _, rec := range s.devices {
		if rec.ActivityStartedAt != nil {
			if _, exists := rec.ActivityStartedAt[cascadeID]; exists {
				delete(rec.ActivityStartedAt, cascadeID)
				rec.UpdatedAt = time.Now()
				changed = true
			}
		}
	}
	if changed {
		_ = s.saveLocked()
	}
}

// HasLiveActivityStateForCascade reports whether any device holds an update token or active Push-to-Start marker for cascadeID.
func (s *PushDeviceStore) HasLiveActivityStateForCascade(cascadeID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, rec := range s.devices {
		if rec.ActivityTokens[cascadeID] != "" || rec.ActivityStartedAt[cascadeID] > 0 {
			return true
		}
	}
	return false
}

// ClearAlertToken removes an invalid/unregistered alert token for a device (e.g. after 410 Unregistered).
func (s *PushDeviceStore) ClearAlertToken(deviceID, badToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.devices[deviceID]
	if !ok || rec.APNSDeviceToken == "" {
		return
	}
	if badToken != "" && rec.APNSDeviceToken != badToken {
		return
	}
	rec.APNSDeviceToken = ""
	rec.UpdatedAt = time.Now()
	_ = s.saveLocked()
}

// ClearLiveActivityStartToken removes an invalid/unregistered push-to-start token.
func (s *PushDeviceStore) ClearLiveActivityStartToken(deviceID, badToken string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.devices[deviceID]
	if !ok || rec.PushToStartToken == "" {
		return
	}
	if badToken != "" && rec.PushToStartToken != badToken {
		return
	}
	rec.PushToStartToken = ""
	rec.UpdatedAt = time.Now()
	_ = s.saveLocked()
}

// RemoveDevice purges all push tokens associated with a paired device.
func (s *PushDeviceStore) RemoveDevice(deviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[deviceID]; !ok {
		return
	}
	delete(s.devices, deviceID)
	_ = s.saveLocked()
}

// ClearAll removes all push device records.
func (s *PushDeviceStore) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.devices) == 0 {
		return
	}
	s.devices = make(map[string]*PushDeviceRecord)
	_ = s.saveLocked()
}

// ListAlertTargets returns all iOS devices with an active alert device token and notifications enabled.
func (s *PushDeviceStore) ListAlertTargets() []APNsAlertTarget {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []APNsAlertTarget
	for _, rec := range s.devices {
		if rec.NotificationsEnabled && rec.APNSDeviceToken != "" {
			out = append(out, APNsAlertTarget{
				DeviceID:         rec.DeviceID,
				Token:            rec.APNSDeviceToken,
				Env:              normalizeAPNSEnv(rec.APNSEnv),
				SoundComplete:    rec.SoundComplete,
				SoundAction:      rec.SoundAction,
				Lang:             i18n.NormalizeLang(rec.Lang),
				ShowGatewayLabel: rec.ShowGatewayLabel,
				GatewayLabel:     rec.GatewayLabel,
			})
		}
	}
	return out
}

// ListLiveActivityTargets returns per-device targets for a cascade:
// if a device has an active update token for cascadeID, it returns that update target (IsStart=false);
// otherwise, if the device has a push-to-start token and includeStart is true (and no active run start was already sent),
// it returns the start target (IsStart=true).
func (s *PushDeviceStore) ListLiveActivityTargets(cascadeID string, includeStart bool) []APNsLiveActivityTarget {
	return s.ListLiveActivityTargetsAt(cascadeID, includeStart, time.Now())
}

// ListLiveActivityTargetsAt behaves like ListLiveActivityTargets using an explicit reference clock.
func (s *PushDeviceStore) ListLiveActivityTargetsAt(cascadeID string, includeStart bool, now time.Time) []APNsLiveActivityTarget {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []APNsLiveActivityTarget
	for _, rec := range s.devices {
		if !rec.LiveActivitiesEnabled {
			continue
		}
		env := normalizeAPNSEnv(rec.APNSEnv)
		lang := i18n.NormalizeLang(rec.Lang)
		if tok := rec.ActivityTokens[cascadeID]; tok != "" {
			out = append(out, APNsLiveActivityTarget{
				DeviceID:         rec.DeviceID,
				CascadeID:        cascadeID,
				Token:            tok,
				Env:              env,
				Lang:             lang,
				ShowGatewayLabel: rec.ShowGatewayLabel,
				GatewayLabel:     rec.GatewayLabel,
				IsStart:          false,
			})
		} else if includeStart && rec.PushToStartToken != "" {
			if startedTs := rec.ActivityStartedAt[cascadeID]; startedTs > 0 && now.Sub(time.Unix(startedTs, 0)) < liveActivityStartRunTTL {
				continue
			}
			out = append(out, APNsLiveActivityTarget{
				DeviceID:         rec.DeviceID,
				CascadeID:        cascadeID,
				Token:            rec.PushToStartToken,
				Env:              env,
				Lang:             lang,
				ShowGatewayLabel: rec.ShowGatewayLabel,
				GatewayLabel:     rec.GatewayLabel,
				IsStart:          true,
			})
		}
	}
	return out
}

// HasAlertTargets reports whether at least one iOS device is ready to receive alert pushes.
func (s *PushDeviceStore) HasAlertTargets() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, rec := range s.devices {
		if rec.NotificationsEnabled && rec.APNSDeviceToken != "" {
			return true
		}
	}
	return false
}

// HasLiveActivityTargets reports whether at least one iOS device can receive Live Activity updates or push-to-start.
func (s *PushDeviceStore) HasLiveActivityTargets() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, rec := range s.devices {
		if !rec.LiveActivitiesEnabled {
			continue
		}
		if rec.PushToStartToken != "" || len(rec.ActivityTokens) > 0 {
			return true
		}
	}
	return false
}

// ActiveDeviceCount returns the count of registered iOS devices holding at least one valid push token.
func (s *PushDeviceStore) ActiveDeviceCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, rec := range s.devices {
		if rec.APNSDeviceToken != "" || rec.PushToStartToken != "" || len(rec.ActivityTokens) > 0 {
			count++
		}
	}
	return count
}

func (s *PushDeviceStore) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	var list []*PushDeviceRecord
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices = make(map[string]*PushDeviceRecord, len(list))
	for _, rec := range list {
		if rec == nil || rec.DeviceID == "" {
			continue
		}
		if rec.ActivityTokens == nil {
			rec.ActivityTokens = make(map[string]string)
		}
		if rec.ActivityUpdatedAt == nil {
			rec.ActivityUpdatedAt = make(map[string]int64)
		}
		if rec.ActivityStartedAt == nil {
			rec.ActivityStartedAt = make(map[string]int64)
		}
		s.devices[rec.DeviceID] = rec
	}
	return nil
}

func (s *PushDeviceStore) saveLocked() error {
	if s.filePath == "" {
		return nil
	}
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create push store dir: %w", err)
	}
	list := make([]*PushDeviceRecord, 0, len(s.devices))
	for _, rec := range s.devices {
		list = append(list, rec)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].DeviceID < list[j].DeviceID })
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp.%d", s.filePath, time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	_ = os.Chmod(tmp, 0600)
	if err := os.Rename(tmp, s.filePath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Chmod(s.filePath, 0600)
	return nil
}
