package cockpit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/i18n"
)

const (
	AutoSwitchBucketGemini5h     = "gemini_5h"
	AutoSwitchBucketGeminiWeekly = "gemini_weekly"
	AutoSwitchBucketClaude5h     = "claude_5h"
	AutoSwitchBucketClaudeWeekly = "claude_weekly"

	AutoSwitchStrategyHighestQuota   = "highest_quota"
	AutoSwitchStrategyCandidateOrder = "candidate_order"

	autoSwitchConfigFilename = "auto_switch_config.json"
	autoSwitchStateFilename  = "auto_switch_state.json"
	autoSwitchFloatEps       = 1e-6
)

// AutoSwitchConfig stores user-configured threshold auto-switching preferences (default disabled).
type AutoSwitchConfig struct {
	Enabled          bool     `json:"enabled"`
	TargetBucket     string   `json:"target_bucket"`     // gemini_5h | gemini_weekly | claude_5h | claude_weekly
	ThresholdPercent float64  `json:"threshold_percent"` // 1..50, default 10
	WaitUntilIdle    bool     `json:"wait_until_idle"`   // default true
	Strategy         string   `json:"strategy"`          // highest_quota | candidate_order
	Candidates       []string `json:"candidates"`        // optional list of account IDs or emails; empty = all backups
	DebounceSec      int      `json:"debounce_sec"`      // default 15
	CooldownSec      int      `json:"cooldown_sec"`      // default 180
}

// AutoSwitchConfigPatch supports partial updates from mobile/web settings.
type AutoSwitchConfigPatch struct {
	Enabled          *bool     `json:"enabled,omitempty"`
	TargetBucket     *string   `json:"target_bucket,omitempty"`
	ThresholdPercent *float64  `json:"threshold_percent,omitempty"`
	WaitUntilIdle    *bool     `json:"wait_until_idle,omitempty"`
	Strategy         *string   `json:"strategy,omitempty"`
	Candidates       *[]string `json:"candidates,omitempty"`
	DebounceSec      *int      `json:"debounce_sec,omitempty"`
	CooldownSec      *int      `json:"cooldown_sec,omitempty"`
}

// AutoSwitchStatus reflects the live evaluation state of the auto-switch engine.
type AutoSwitchStatus struct {
	State              string  `json:"state"` // disabled | watching | pending_debounce | waiting_idle | cooldown | no_candidates | switched | error
	Reason             string  `json:"reason,omitempty"`
	LastCheckAt        int64   `json:"last_check_at,omitempty"`
	LastSwitchAt       int64   `json:"last_switch_at,omitempty"`
	LastFromEmail      string  `json:"last_from_email,omitempty"`
	LastToEmail        string  `json:"last_to_email,omitempty"`
	LastToRemainingPct float64 `json:"last_to_remaining_pct,omitempty"`
	LastSwitchNoticeID string  `json:"last_switch_notice_id,omitempty"`
}

// AutoSwitchResponse is returned by GET and POST /api/v1/cockpit/auto-switch.
type AutoSwitchResponse struct {
	Config           AutoSwitchConfig `json:"config"`
	Status           AutoSwitchStatus `json:"status"`
	CockpitInstalled bool             `json:"cockpit_installed"`
}

// AutoSwitchRuntime injects side-effect dependencies (switching, busy check, push notification).
type AutoSwitchRuntime struct {
	IsBusy        func() bool
	SwitchAccount func(accountID string) error
	Notify        func(QuotaAlert)
	Now           func() time.Time
}

type autoSwitchStore struct {
	mu            sync.Mutex
	loaded        bool
	cfgPath       string
	statePath     string
	cfg           AutoSwitchConfig
	status        AutoSwitchStatus
	debounceKey   string
	debounceSince time.Time
	switching     bool
}

var defaultAutoSwitch = &autoSwitchStore{}

// DefaultAutoSwitchConfig returns the safe opt-in defaults.
func DefaultAutoSwitchConfig() AutoSwitchConfig {
	return AutoSwitchConfig{
		Enabled:          false,
		TargetBucket:     AutoSwitchBucketGemini5h,
		ThresholdPercent: 10,
		WaitUntilIdle:    true,
		Strategy:         AutoSwitchStrategyHighestQuota,
		Candidates:       []string{},
		DebounceSec:      15,
		CooldownSec:      180,
	}
}

// IsValidAutoSwitchBucket validates one of the 4 supported quota pool windows.
func IsValidAutoSwitchBucket(b string) bool {
	switch strings.TrimSpace(strings.ToLower(b)) {
	case AutoSwitchBucketGemini5h, AutoSwitchBucketGeminiWeekly,
		AutoSwitchBucketClaude5h, AutoSwitchBucketClaudeWeekly:
		return true
	default:
		return false
	}
}

func normalizeAutoSwitchConfig(c AutoSwitchConfig) AutoSwitchConfig {
	def := DefaultAutoSwitchConfig()
	b := strings.TrimSpace(strings.ToLower(c.TargetBucket))
	if !IsValidAutoSwitchBucket(b) {
		b = def.TargetBucket
	}
	c.TargetBucket = b
	if c.ThresholdPercent < 1 || c.ThresholdPercent > 50 {
		c.ThresholdPercent = def.ThresholdPercent
	}
	st := strings.TrimSpace(strings.ToLower(c.Strategy))
	if st != AutoSwitchStrategyCandidateOrder {
		st = AutoSwitchStrategyHighestQuota
	}
	c.Strategy = st
	if c.DebounceSec < 0 || c.DebounceSec > 600 {
		c.DebounceSec = def.DebounceSec
	}
	if c.CooldownSec < 30 || c.CooldownSec > 3600 {
		c.CooldownSec = def.CooldownSec
	}
	if c.Candidates == nil {
		c.Candidates = []string{}
	} else {
		seen := make(map[string]bool, len(c.Candidates))
		cleaned := make([]string, 0, len(c.Candidates))
		for _, item := range c.Candidates {
			t := strings.TrimSpace(item)
			if t == "" {
				continue
			}
			key := strings.ToLower(t)
			if !seen[key] {
				seen[key] = true
				cleaned = append(cleaned, t)
			}
		}
		c.Candidates = cleaned
	}
	return c
}

func (s *autoSwitchStore) ensureLoadedLocked() {
	dir := strings.TrimSpace(config.GetDataDir())
	wantCfg := ""
	wantState := ""
	if dir != "" {
		wantCfg = filepath.Join(dir, autoSwitchConfigFilename)
		wantState = filepath.Join(dir, autoSwitchStateFilename)
	}
	if s.loaded && s.cfgPath == wantCfg && s.statePath == wantState {
		return
	}
	s.loaded = true
	s.cfgPath = wantCfg
	s.statePath = wantState
	s.cfg = DefaultAutoSwitchConfig()
	s.status = AutoSwitchStatus{State: "disabled"}
	s.debounceKey = ""
	s.debounceSince = time.Time{}

	if wantCfg != "" {
		if raw, err := os.ReadFile(wantCfg); err == nil {
			var disk AutoSwitchConfig
			if json.Unmarshal(raw, &disk) == nil {
				s.cfg = normalizeAutoSwitchConfig(disk)
			}
		}
	}
	if wantState != "" {
		if raw, err := os.ReadFile(wantState); err == nil {
			var st AutoSwitchStatus
			if json.Unmarshal(raw, &st) == nil && st.State != "" {
				s.status = st
			}
		}
	}
	if !s.cfg.Enabled {
		s.status.State = "disabled"
	} else if s.status.State == "" || s.status.State == "disabled" {
		s.status.State = "watching"
	}
}

func (s *autoSwitchStore) saveConfigLocked() error {
	if s.cfgPath == "" {
		return nil
	}
	b, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(s.cfgPath), 0700)
	return writeFileAtomic0600(s.cfgPath, append(b, '\n'))
}

func (s *autoSwitchStore) saveStateLocked() {
	if s.statePath == "" {
		return
	}
	b, err := json.MarshalIndent(s.status, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(s.statePath), 0700)
	_ = writeFileAtomic0600(s.statePath, append(b, '\n'))
}

// ResetAutoSwitchForTest resets in-memory auto-switch state for isolated unit tests.
func ResetAutoSwitchForTest() {
	defaultAutoSwitch.mu.Lock()
	defaultAutoSwitch.loaded = false
	defaultAutoSwitch.cfgPath = ""
	defaultAutoSwitch.statePath = ""
	defaultAutoSwitch.debounceKey = ""
	defaultAutoSwitch.debounceSince = time.Time{}
	defaultAutoSwitch.switching = false
	defaultAutoSwitch.mu.Unlock()
}

// GetAutoSwitchResponse returns the current auto-switch configuration and status snapshot.
func GetAutoSwitchResponse() AutoSwitchResponse {
	defaultAutoSwitch.mu.Lock()
	defer defaultAutoSwitch.mu.Unlock()
	defaultAutoSwitch.ensureLoadedLocked()
	return AutoSwitchResponse{
		Config:           defaultAutoSwitch.cfg,
		Status:           defaultAutoSwitch.status,
		CockpitInstalled: IsCockpitInstalled(),
	}
}

// UpdateAutoSwitchConfig applies a partial patch, persists to disk, and returns the updated snapshot.
func UpdateAutoSwitchConfig(patch AutoSwitchConfigPatch) (AutoSwitchResponse, error) {
	defaultAutoSwitch.mu.Lock()
	defer defaultAutoSwitch.mu.Unlock()
	defaultAutoSwitch.ensureLoadedLocked()

	cfg := defaultAutoSwitch.cfg
	if patch.Enabled != nil {
		cfg.Enabled = *patch.Enabled
	}
	if patch.TargetBucket != nil {
		cfg.TargetBucket = *patch.TargetBucket
	}
	if patch.ThresholdPercent != nil {
		cfg.ThresholdPercent = *patch.ThresholdPercent
	}
	if patch.WaitUntilIdle != nil {
		cfg.WaitUntilIdle = *patch.WaitUntilIdle
	}
	if patch.Strategy != nil {
		cfg.Strategy = *patch.Strategy
	}
	if patch.Candidates != nil {
		cfg.Candidates = append([]string(nil), (*patch.Candidates)...)
	}
	if patch.DebounceSec != nil {
		cfg.DebounceSec = *patch.DebounceSec
	}
	if patch.CooldownSec != nil {
		cfg.CooldownSec = *patch.CooldownSec
	}
	cfg = normalizeAutoSwitchConfig(cfg)
	defaultAutoSwitch.cfg = cfg

	// Reset debounce when rule changes.
	defaultAutoSwitch.debounceKey = ""
	defaultAutoSwitch.debounceSince = time.Time{}
	if !cfg.Enabled {
		defaultAutoSwitch.status.State = "disabled"
		defaultAutoSwitch.status.Reason = ""
	} else {
		defaultAutoSwitch.status.State = "watching"
		defaultAutoSwitch.status.Reason = ""
	}

	if err := defaultAutoSwitch.saveConfigLocked(); err != nil {
		return AutoSwitchResponse{}, err
	}
	defaultAutoSwitch.saveStateLocked()
	return AutoSwitchResponse{
		Config:           defaultAutoSwitch.cfg,
		Status:           defaultAutoSwitch.status,
		CockpitInstalled: IsCockpitInstalled(),
	}, nil
}

func quotaMetricForBucketKey(acc *AccountQuota, bucketKey string) *QuotaMetric {
	if acc == nil {
		return nil
	}
	switch bucketKey {
	case AutoSwitchBucketGemini5h:
		return acc.Gemini5h
	case AutoSwitchBucketGeminiWeekly:
		return acc.GeminiWeekly
	case AutoSwitchBucketClaude5h:
		return acc.Claude5h
	case AutoSwitchBucketClaudeWeekly:
		return acc.ClaudeWeekly
	default:
		return nil
	}
}

func autoSwitchBucketDisplayTitle(bucketKey string) string {
	switch bucketKey {
	case AutoSwitchBucketGemini5h:
		return "Gemini · 5h"
	case AutoSwitchBucketGeminiWeekly:
		return "Gemini · Weekly"
	case AutoSwitchBucketClaude5h:
		return "Claude · 5h"
	case AutoSwitchBucketClaudeWeekly:
		return "Claude · Weekly"
	default:
		return bucketKey
	}
}

func selectAutoSwitchCandidate(resp *CockpitQuotaResponse, cfg AutoSwitchConfig, curRemPct float64) (*AccountQuota, float64) {
	if resp == nil || resp.CurrentAccount == nil {
		return nil, 0
	}
	cur := resp.CurrentAccount
	minRequired := cfg.ThresholdPercent + 10.0
	if curRemPct+10.0 > minRequired {
		minRequired = curRemPct + 10.0
	}

	byKey := make(map[string]*AccountQuota, len(resp.Accounts)*2)
	eligible := make([]AccountQuota, 0, len(resp.Accounts))
	for i := range resp.Accounts {
		acc := resp.Accounts[i]
		if acc.IsCurrent || acc.ID == cur.ID || (cur.Email != "" && strings.EqualFold(acc.Email, cur.Email)) {
			continue
		}
		m := quotaMetricForBucketKey(&acc, cfg.TargetBucket)
		if m == nil {
			continue
		}
		if m.RemainingPercent+autoSwitchFloatEps < minRequired {
			continue
		}
		eligible = append(eligible, acc)
		accCopy := acc
		byKey[strings.ToLower(strings.TrimSpace(acc.ID))] = &accCopy
		if acc.Email != "" {
			byKey[strings.ToLower(strings.TrimSpace(acc.Email))] = &accCopy
		}
	}
	if len(eligible) == 0 {
		return nil, 0
	}

	// Filter to explicit candidates when user configured a non-empty list.
	var pool []AccountQuota
	if len(cfg.Candidates) > 0 {
		for _, want := range cfg.Candidates {
			if matched, ok := byKey[strings.ToLower(strings.TrimSpace(want))]; ok && matched != nil {
				pool = append(pool, *matched)
			}
		}
		if len(pool) == 0 {
			return nil, 0
		}
	} else {
		pool = eligible
	}

	if cfg.Strategy == AutoSwitchStrategyCandidateOrder && len(cfg.Candidates) > 0 {
		chosen := pool[0]
		m := quotaMetricForBucketKey(&chosen, cfg.TargetBucket)
		return &chosen, m.RemainingPercent
	}

	// Default strategy: highest remaining quota on the target bucket, tie-break by tier rank (ULTRA > PRO > FREE).
	sort.SliceStable(pool, func(i, j int) bool {
		mi := quotaMetricForBucketKey(&pool[i], cfg.TargetBucket)
		mj := quotaMetricForBucketKey(&pool[j], cfg.TargetBucket)
		if mi.RemainingPercent != mj.RemainingPercent {
			return mi.RemainingPercent > mj.RemainingPercent
		}
		return accountTierSortRank(pool[i]) < accountTierSortRank(pool[j])
	})
	chosen := pool[0]
	return &chosen, quotaMetricForBucketKey(&chosen, cfg.TargetBucket).RemainingPercent
}

func buildAutoSwitchAlert(lang, bucketKey, fromEmail, toEmail string, toRemPct float64, noticeID string) QuotaAlert {
	bucketTitle := autoSwitchBucketDisplayTitle(bucketKey)
	formatFor := func(l string) (string, string) {
		switch l {
		case "en":
			return fmt.Sprintf("Switched Account · %s", bucketTitle),
				fmt.Sprintf("Quota dropped below threshold on %s; switched cleanly to %s (%.0f%% remaining).", fromEmail, toEmail, toRemPct)
		case "ja":
			return fmt.Sprintf("アカウント自動切替 · %s", bucketTitle),
				fmt.Sprintf("%s の枠が閾値以下になったため、%s（残り %.0f%%）へ自動切替しました。", fromEmail, toEmail, toRemPct)
		default:
			return fmt.Sprintf("已自动切换账号 · %s", bucketTitle),
				fmt.Sprintf("%s 额度低于阈值，已在空闲时自动切至 %s（剩余 %.0f%%）。", fromEmail, toEmail, toRemPct)
		}
	}
	title, body := formatFor(lang)
	return QuotaAlert{
		Level:    "info",
		Title:    title,
		Body:     body,
		StateKey: "auto_switch:" + noticeID,
		Localize: func(targetLang string) (string, string) {
			return formatFor(i18n.ResolveLang(targetLang))
		},
	}
}

// EvaluateAutoSwitch runs one safe evaluation tick against a fresh CockpitQuotaResponse.
func EvaluateAutoSwitch(resp *CockpitQuotaResponse, liveEmail string, rt AutoSwitchRuntime) AutoSwitchStatus {
	nowFn := rt.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn()
	nowMs := now.UnixMilli()

	defaultAutoSwitch.mu.Lock()
	defaultAutoSwitch.ensureLoadedLocked()
	cfg := defaultAutoSwitch.cfg

	if !cfg.Enabled {
		defaultAutoSwitch.status.State = "disabled"
		defaultAutoSwitch.status.Reason = ""
		st := defaultAutoSwitch.status
		defaultAutoSwitch.mu.Unlock()
		return st
	}
	if defaultAutoSwitch.switching {
		st := defaultAutoSwitch.status
		defaultAutoSwitch.mu.Unlock()
		return st
	}

	defaultAutoSwitch.status.LastCheckAt = nowMs

	if resp == nil || resp.CurrentAccount == nil || !resp.CockpitInstalled {
		defaultAutoSwitch.status.State = "watching"
		defaultAutoSwitch.status.Reason = "cockpit_or_snapshot_unavailable"
		defaultAutoSwitch.debounceKey = ""
		defaultAutoSwitch.debounceSince = time.Time{}
		defaultAutoSwitch.saveStateLocked()
		st := defaultAutoSwitch.status
		defaultAutoSwitch.mu.Unlock()
		return st
	}

	cur := resp.CurrentAccount
	if strings.TrimSpace(liveEmail) != "" && cur.Email != "" && !strings.EqualFold(strings.TrimSpace(liveEmail), strings.TrimSpace(cur.Email)) {
		defaultAutoSwitch.status.State = "watching"
		defaultAutoSwitch.status.Reason = "active_email_mismatch"
		defaultAutoSwitch.debounceKey = ""
		defaultAutoSwitch.debounceSince = time.Time{}
		defaultAutoSwitch.saveStateLocked()
		st := defaultAutoSwitch.status
		defaultAutoSwitch.mu.Unlock()
		return st
	}

	metric := quotaMetricForBucketKey(cur, cfg.TargetBucket)
	if metric == nil {
		defaultAutoSwitch.status.State = "watching"
		defaultAutoSwitch.status.Reason = "target_bucket_missing"
		defaultAutoSwitch.debounceKey = ""
		defaultAutoSwitch.debounceSince = time.Time{}
		defaultAutoSwitch.saveStateLocked()
		st := defaultAutoSwitch.status
		defaultAutoSwitch.mu.Unlock()
		return st
	}

	if metric.RemainingPercent-autoSwitchFloatEps > cfg.ThresholdPercent {
		defaultAutoSwitch.debounceKey = ""
		defaultAutoSwitch.debounceSince = time.Time{}
		defaultAutoSwitch.status.State = "watching"
		defaultAutoSwitch.status.Reason = ""
		defaultAutoSwitch.saveStateLocked()
		st := defaultAutoSwitch.status
		defaultAutoSwitch.mu.Unlock()
		return st
	}

	// Cooldown guard
	if defaultAutoSwitch.status.LastSwitchAt > 0 && now.Sub(time.UnixMilli(defaultAutoSwitch.status.LastSwitchAt)) < time.Duration(cfg.CooldownSec)*time.Second {
		defaultAutoSwitch.status.State = "cooldown"
		defaultAutoSwitch.status.Reason = "cooldown_active"
		defaultAutoSwitch.saveStateLocked()
		st := defaultAutoSwitch.status
		defaultAutoSwitch.mu.Unlock()
		return st
	}

	// Debounce guard
	dbKey := strings.ToLower(cur.Email) + "|" + cfg.TargetBucket
	if defaultAutoSwitch.debounceKey != dbKey || defaultAutoSwitch.debounceSince.IsZero() {
		defaultAutoSwitch.debounceKey = dbKey
		defaultAutoSwitch.debounceSince = now
		if cfg.DebounceSec > 0 {
			defaultAutoSwitch.status.State = "pending_debounce"
			defaultAutoSwitch.status.Reason = "threshold_debounce"
			defaultAutoSwitch.saveStateLocked()
			st := defaultAutoSwitch.status
			defaultAutoSwitch.mu.Unlock()
			return st
		}
	} else if cfg.DebounceSec > 0 && now.Sub(defaultAutoSwitch.debounceSince) < time.Duration(cfg.DebounceSec)*time.Second {
		defaultAutoSwitch.status.State = "pending_debounce"
		defaultAutoSwitch.status.Reason = "threshold_debounce"
		defaultAutoSwitch.saveStateLocked()
		st := defaultAutoSwitch.status
		defaultAutoSwitch.mu.Unlock()
		return st
	}

	// Idle-safe guard: never interrupt an in-flight generation.
	if cfg.WaitUntilIdle && rt.IsBusy != nil && rt.IsBusy() {
		defaultAutoSwitch.status.State = "waiting_idle"
		defaultAutoSwitch.status.Reason = "generation_in_progress"
		defaultAutoSwitch.saveStateLocked()
		st := defaultAutoSwitch.status
		defaultAutoSwitch.mu.Unlock()
		return st
	}

	target, targetRem := selectAutoSwitchCandidate(resp, cfg, metric.RemainingPercent)
	if target == nil {
		defaultAutoSwitch.status.State = "no_candidates"
		defaultAutoSwitch.status.Reason = "no_healthy_backup_account"
		defaultAutoSwitch.saveStateLocked()
		st := defaultAutoSwitch.status
		defaultAutoSwitch.mu.Unlock()
		return st
	}

	switchFn := rt.SwitchAccount
	if switchFn == nil {
		switchFn = SwitchAccount
	}

	defaultAutoSwitch.switching = true
	fromEmail := cur.Email
	toID := target.ID
	toEmail := target.Email
	defaultAutoSwitch.mu.Unlock()

	err := switchFn(toID)

	defaultAutoSwitch.mu.Lock()
	defaultAutoSwitch.switching = false
	defaultAutoSwitch.debounceKey = ""
	defaultAutoSwitch.debounceSince = time.Time{}

	if err != nil {
		slog.Warn("[AutoSwitch] account switch failed", "from", fromEmail, "to", toEmail, "err", err)
		defaultAutoSwitch.status.State = "error"
		defaultAutoSwitch.status.Reason = err.Error()
		defaultAutoSwitch.saveStateLocked()
		st := defaultAutoSwitch.status
		defaultAutoSwitch.mu.Unlock()
		return st
	}

	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d", fromEmail, toEmail, nowMs)))
	noticeID := hex.EncodeToString(sum[:8])

	defaultAutoSwitch.status = AutoSwitchStatus{
		State:              "switched",
		Reason:             "",
		LastCheckAt:        nowMs,
		LastSwitchAt:       nowMs,
		LastFromEmail:      fromEmail,
		LastToEmail:        toEmail,
		LastToRemainingPct: targetRem,
		LastSwitchNoticeID: noticeID,
	}
	defaultAutoSwitch.saveStateLocked()
	st := defaultAutoSwitch.status
	defaultAutoSwitch.mu.Unlock()

	slog.Info("[AutoSwitch] switched account automatically",
		"bucket", cfg.TargetBucket,
		"from", fromEmail,
		"to", toEmail,
		"to_remaining_pct", targetRem,
	)

	if rt.Notify != nil {
		rt.Notify(buildAutoSwitchAlert(i18n.GetLang(), cfg.TargetBucket, fromEmail, toEmail, targetRem, noticeID))
	}
	return st
}
