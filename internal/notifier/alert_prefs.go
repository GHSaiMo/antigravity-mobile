package notifier

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultStallAlertMinutes   = 8
	defaultStallBgAlertMinutes = 15
	stallAlertCooldown         = 1 * time.Hour
)

// AlertPreferences controls which optional push notification categories are enabled on the gateway.
// Interactive approval requests and agent questions always follow the master push switch.
type AlertPreferences struct {
	TaskCompletion  bool   `json:"task_completion"`
	TaskStall       bool   `json:"task_stall"`
	CacheMiss       bool   `json:"cache_miss"`
	CacheMissMode   string `json:"cache_miss_mode,omitempty"` // "shadow" | "live" | "off"
	QuotaLow        bool   `json:"quota_low"`
	QuotaBonusReset bool   `json:"quota_bonus_reset"`
	QuotaAnomaly    bool   `json:"quota_anomaly"`
	StallMinutes    int    `json:"stall_minutes,omitempty"`
}

// AlertPreferencesPatch supports partial updates from mobile/web settings clients.
type AlertPreferencesPatch struct {
	TaskCompletion  *bool `json:"task_completion,omitempty"`
	TaskStall       *bool `json:"task_stall,omitempty"`
	CacheMiss       *bool `json:"cache_miss,omitempty"`
	QuotaLow        *bool `json:"quota_low,omitempty"`
	QuotaBonusReset *bool `json:"quota_bonus_reset,omitempty"`
	QuotaAnomaly    *bool `json:"quota_anomaly,omitempty"`
}

// DefaultStallAlertMinutes reads STALL_ALERT_MINUTES (default 8; <=0 disables).
func DefaultStallAlertMinutes() int {
	raw := strings.TrimSpace(os.Getenv("STALL_ALERT_MINUTES"))
	if raw == "" {
		return defaultStallAlertMinutes
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return defaultStallAlertMinutes
	}
	if v <= 0 {
		return 0
	}
	return v
}

// DefaultStallBgAlertMinutes returns the relaxed threshold when background terminal tasks are active.
func DefaultStallBgAlertMinutes() int {
	base := DefaultStallAlertMinutes()
	if base <= 0 {
		return 0
	}
	if base*2 > defaultStallBgAlertMinutes {
		return base * 2
	}
	return defaultStallBgAlertMinutes
}

func envBoolDefaultTrue(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

// DefaultAlertPreferences returns the baseline preferences honoring legacy env toggles.
func DefaultAlertPreferences() AlertPreferences {
	stallMins := DefaultStallAlertMinutes()
	cacheMissMode := CacheMissAlertMode()
	return AlertPreferences{
		TaskCompletion:  true,
		TaskStall:       stallMins > 0,
		CacheMiss:       cacheMissMode != CacheMissModeOff,
		CacheMissMode:   cacheMissMode,
		QuotaLow:        envBoolDefaultTrue("QUOTA_ALERT_ENABLED"),
		QuotaBonusReset: envBoolDefaultTrue("QUOTA_ALERT_ENABLED") && envBoolDefaultTrue("QUOTA_ALERT_BONUS_RESET_ENABLED"),
		QuotaAnomaly:    envBoolDefaultTrue("QUOTA_ALERT_ENABLED") && envBoolDefaultTrue("QUOTA_ALERT_ANOMALY_ENABLED"),
		StallMinutes:    stallMins,
	}
}

// AlertPreferencesStore persists user-configured notification preferences atomically on disk.
type AlertPreferencesStore struct {
	mu    sync.RWMutex
	path  string
	prefs AlertPreferences
}

// NewAlertPreferencesStore creates a store backed by filePath (empty path = in-memory only).
func NewAlertPreferencesStore(filePath string) *AlertPreferencesStore {
	s := &AlertPreferencesStore{
		path:  strings.TrimSpace(filePath),
		prefs: DefaultAlertPreferences(),
	}
	s.loadFromDisk()
	return s
}

func (s *AlertPreferencesStore) loadFromDisk() {
	if s.path == "" {
		return
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var patch AlertPreferencesPatch
	if err := json.Unmarshal(data, &patch); err != nil {
		slog.Warn("[AlertPrefs] Failed to parse saved notification_prefs.json, falling back to defaults", "err", err)
		return
	}
	applyAlertPreferencesPatch(&s.prefs, patch)
}

func applyAlertPreferencesPatch(dst *AlertPreferences, p AlertPreferencesPatch) {
	if p.TaskCompletion != nil {
		dst.TaskCompletion = *p.TaskCompletion
	}
	if p.TaskStall != nil {
		dst.TaskStall = *p.TaskStall
	}
	if p.CacheMiss != nil {
		dst.CacheMiss = *p.CacheMiss
	}
	if p.QuotaLow != nil {
		dst.QuotaLow = *p.QuotaLow
	}
	if p.QuotaBonusReset != nil {
		dst.QuotaBonusReset = *p.QuotaBonusReset
	}
	if p.QuotaAnomaly != nil {
		dst.QuotaAnomaly = *p.QuotaAnomaly
	}
}

// Get returns a snapshot of current preferences including the active stall alert threshold in minutes.
func (s *AlertPreferencesStore) Get() AlertPreferences {
	if s == nil {
		return DefaultAlertPreferences()
	}
	s.mu.RLock()
	out := s.prefs
	s.mu.RUnlock()
	out.StallMinutes = DefaultStallAlertMinutes()
	out.CacheMissMode = CacheMissAlertMode()
	return out
}

// Update applies a partial patch and persists atomically if backed by a file path.
func (s *AlertPreferencesStore) Update(patch AlertPreferencesPatch) (AlertPreferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	applyAlertPreferencesPatch(&s.prefs, patch)
	out := s.prefs
	out.StallMinutes = DefaultStallAlertMinutes()
	out.CacheMissMode = CacheMissAlertMode()
	if s.path == "" {
		return out, nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return out, err
	}
	diskPayload := AlertPreferences{
		TaskCompletion:  s.prefs.TaskCompletion,
		TaskStall:       s.prefs.TaskStall,
		CacheMiss:       s.prefs.CacheMiss,
		QuotaLow:        s.prefs.QuotaLow,
		QuotaBonusReset: s.prefs.QuotaBonusReset,
		QuotaAnomaly:    s.prefs.QuotaAnomaly,
	}
	raw, err := json.MarshalIndent(diskPayload, "", "  ")
	if err != nil {
		return out, err
	}
	tmpPath := s.path + ".tmp." + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmpPath, raw, 0600); err != nil {
		return out, err
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return out, err
	}
	return out, nil
}
