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
)

const (
	maxActivityTokensPerDevice = 20
	apnsEnvProduction          = "production"
	apnsEnvSandbox             = "sandbox"
)

// PushDeviceRecord persists APNs push tokens and preferences for a paired iOS device.
type PushDeviceRecord struct {
	DeviceID              string            `json:"device_id"`
	Platform              string            `json:"platform"` // "ios"
	APNSDeviceToken       string            `json:"apns_device_token,omitempty"`
	APNSEnv               string            `json:"apns_env,omitempty"` // "production" | "sandbox"
	PushToStartToken      string            `json:"push_to_start_token,omitempty"`
	ActivityTokens        map[string]string `json:"activity_tokens,omitempty"` // cascadeID -> update token
	ActivityUpdatedAt     map[string]int64  `json:"activity_updated_at,omitempty"`
	NotificationsEnabled  bool              `json:"notifications_enabled"`
	LiveActivitiesEnabled bool              `json:"live_activities_enabled"`
	SoundComplete         string            `json:"sound_complete,omitempty"`
	SoundAction           string            `json:"sound_action,omitempty"`
	UpdatedAt             time.Time         `json:"updated_at"`
}

// APNsAlertTarget identifies a registered iOS device eligible for standard APNs alert notifications.
type APNsAlertTarget struct {
	DeviceID      string
	Token         string
	Env           string
	SoundComplete string
	SoundAction   string
}

// APNsLiveActivityTarget identifies a registered iOS device Live Activity token (update or push-to-start).
type APNsLiveActivityTarget struct {
	DeviceID  string
	CascadeID string
	Token     string
	Env       string
	IsStart   bool
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
			NotificationsEnabled:  true,
			LiveActivitiesEnabled: true,
		}
		if rec.Platform == "" {
			rec.Platform = "ios"
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
	pruneActivityTokensLocked(rec)
	rec.UpdatedAt = time.Now()
	return s.saveLocked()
}

func pruneActivityTokensLocked(rec *PushDeviceRecord) {
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
				DeviceID:      rec.DeviceID,
				Token:         rec.APNSDeviceToken,
				Env:           normalizeAPNSEnv(rec.APNSEnv),
				SoundComplete: rec.SoundComplete,
				SoundAction:   rec.SoundAction,
			})
		}
	}
	return out
}

// ListLiveActivityTargets returns per-device targets for a cascade:
// if a device has an active update token for cascadeID, it returns that update target (IsStart=false);
// otherwise, if the device has a push-to-start token and includeStart is true, it returns the start target (IsStart=true).
func (s *PushDeviceStore) ListLiveActivityTargets(cascadeID string, includeStart bool) []APNsLiveActivityTarget {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []APNsLiveActivityTarget
	for _, rec := range s.devices {
		if !rec.LiveActivitiesEnabled {
			continue
		}
		env := normalizeAPNSEnv(rec.APNSEnv)
		if tok := rec.ActivityTokens[cascadeID]; tok != "" {
			out = append(out, APNsLiveActivityTarget{
				DeviceID:  rec.DeviceID,
				CascadeID: cascadeID,
				Token:     tok,
				Env:       env,
				IsStart:   false,
			})
		} else if includeStart && rec.PushToStartToken != "" {
			out = append(out, APNsLiveActivityTarget{
				DeviceID:  rec.DeviceID,
				CascadeID: cascadeID,
				Token:     rec.PushToStartToken,
				Env:       env,
				IsStart:   true,
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
	return os.Rename(tmp, s.filePath)
}
