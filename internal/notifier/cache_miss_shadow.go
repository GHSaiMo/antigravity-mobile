package notifier

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/config"
)

const (
	// CacheMissModeShadow runs cache-miss detection silently in the background,
	// persists audit telemetry, suppresses user-facing push notifications and
	// client-facing orange miss_streak badges, and refreshes the Private Docs report.
	CacheMissModeShadow = config.CacheMissModeShadow
	// CacheMissModeLive sends real push notifications (subject to AlertPreferences.CacheMiss)
	// and surfaces client-facing miss_streak badges while still recording audit telemetry.
	CacheMissModeLive = config.CacheMissModeLive
	// CacheMissModeOff disables push notifications, client miss_streak badges, and background auditing.
	CacheMissModeOff = config.CacheMissModeOff

	cacheMissAuditReportFilename = "cache_miss_shadow_audit_report.md"
	cacheMissAuditStateFilename  = "cache_miss_shadow_audit.json"
	cacheMissAuditMaxEpisodes    = 500
	cacheMissAuditMaxSessions    = 2000
	cacheMissAuditFlushDebounce  = 30 * time.Second
	cacheMissAuditStaleAge       = 2 * time.Hour
	cacheMissRecoveryMinHitRate  = 20.0
)

// CacheMissAlertMode delegates to config.CacheMissAlertMode so proxy and notifier share one source of truth.
func CacheMissAlertMode() string {
	return config.CacheMissAlertMode()
}

func isCacheMissAlertEnabled() bool {
	return CacheMissAlertMode() != CacheMissModeOff
}

func isCacheMissAlertLive() bool {
	return config.IsCacheMissAlertLive()
}

// ResolveCacheMissAuditStatePath resolves the JSON persistence path for shadow audit state.
func ResolveCacheMissAuditStatePath(dataDir string) string {
	if custom := strings.TrimSpace(os.Getenv("CACHE_MISS_AUDIT_STATE_PATH")); custom != "" {
		return custom
	}
	if strings.TrimSpace(dataDir) == "" {
		return ""
	}
	return filepath.Join(dataDir, cacheMissAuditStateFilename)
}

// ResolveCacheMissAuditReportPath resolves where the background Markdown observation report
// should be written. Prefers explicit CACHE_MISS_AUDIT_REPORT_PATH, then a repo-local
// private-docs directory (relative to CWD or executable), and finally falls back to dataDir.
func ResolveCacheMissAuditReportPath(dataDir string) string {
	if custom := strings.TrimSpace(os.Getenv("CACHE_MISS_AUDIT_REPORT_PATH")); custom != "" {
		return custom
	}
	if wd, err := os.Getwd(); err == nil && wd != "" {
		candidateDir := filepath.Join(wd, "private-docs")
		if fi, statErr := os.Stat(candidateDir); statErr == nil && fi.IsDir() {
			return filepath.Join(candidateDir, cacheMissAuditReportFilename)
		}
	}
	if execPath, err := os.Executable(); err == nil && execPath != "" {
		execDir := filepath.Dir(execPath)
		for _, candidateDir := range []string{
			filepath.Join(execDir, "private-docs"),
			filepath.Join(execDir, "..", "private-docs"),
		} {
			if fi, statErr := os.Stat(candidateDir); statErr == nil && fi.IsDir() {
				if abs, absErr := filepath.Abs(filepath.Join(candidateDir, cacheMissAuditReportFilename)); absErr == nil {
					return abs
				}
				return filepath.Join(candidateDir, cacheMissAuditReportFilename)
			}
		}
	}
	if strings.TrimSpace(dataDir) == "" {
		return ""
	}
	return filepath.Join(dataDir, cacheMissAuditReportFilename)
}

// CacheMissObservation is a single prompt-cache telemetry sample from Watcher or streaming updates.
type CacheMissObservation struct {
	CascadeID       string
	SessionTitle    string
	Model           string
	Source          string // "watcher" | "trajectory_update" | "direct"
	Mode            string // "shadow" | "live"
	StepCount       int
	Streak          int
	MissTokens      int64
	UsedTokens      int64
	MaxTokens       int64
	ContextPct      float64
	InputTokens     int64
	CacheReadTokens int64
	CacheHitRate    *float64
	TTFTSeconds     float64
	Now             time.Time
}

// CacheMissShadowEpisode tracks the full lifecycle of one triggered cache-miss streak episode,
// including peak miss metrics and whether the session self-recovered without user intervention.
type CacheMissShadowEpisode struct {
	EpisodeID         string     `json:"episode_id"`
	CascadeID         string     `json:"cascade_id"`
	SessionTitle      string     `json:"session_title"`
	Model             string     `json:"model,omitempty"`
	Source            string     `json:"source,omitempty"`
	Mode              string     `json:"mode"`
	Status            string     `json:"status"` // "ongoing" | "self_recovered" | "session_ended" | "stale_expired"
	TriggeredAt       time.Time  `json:"triggered_at"`
	LastObservedAt    time.Time  `json:"last_observed_at"`
	ResolvedAt        *time.Time `json:"resolved_at,omitempty"`
	InitialStreak     int        `json:"initial_streak"`
	PeakStreak        int        `json:"peak_streak"`
	InitialMissTokens int64      `json:"initial_miss_tokens"`
	PeakMissTokens    int64      `json:"peak_miss_tokens"`
	InitialHitRate    float64    `json:"initial_hit_rate"`
	LowestHitRate     float64    `json:"lowest_hit_rate"`
	RecoveryHitRate   float64    `json:"recovery_hit_rate,omitempty"`
	UsedTokens        int64      `json:"used_tokens,omitempty"`
	MaxTokens         int64      `json:"max_tokens,omitempty"`
	ContextPct        float64    `json:"context_pct,omitempty"`
	TTFTSeconds       float64    `json:"ttft_sec,omitempty"`
	StepCount         int        `json:"step_count,omitempty"`
	WouldHaveNotified bool       `json:"would_have_notified"`
	SampleCount       int        `json:"sample_count"`
	TerminalStatus    string     `json:"terminal_status,omitempty"`
}

// CacheMissModelStats aggregates baseline cache-hit checks and shadow alert outcomes per model.
type CacheMissModelStats struct {
	Checks           int `json:"checks"`
	HitChecks        int `json:"hit_checks"`
	MissStreak2Plus  int `json:"miss_streak_2_plus"`
	ShadowTriggers   int `json:"shadow_triggers"`
	WouldNotifyCount int `json:"would_notify_count"`
	SelfRecovered    int `json:"self_recovered"`
	SessionEnded     int `json:"session_ended"`
}

type cacheMissAuditState struct {
	Version               int                             `json:"version"`
	StartedAt             time.Time                       `json:"started_at"`
	UpdatedAt             time.Time                       `json:"updated_at"`
	NextSeq               int                             `json:"next_seq"`
	TotalChecks           int                             `json:"total_checks"`
	TotalHitChecks        int                             `json:"total_hit_checks"`
	TotalStreak2Plus      int                             `json:"total_streak_2_plus"`
	TotalEpisodes         int                             `json:"total_episodes"`
	TotalWouldNotify      int                             `json:"total_would_notify"`
	TotalSelfRecovered    int                             `json:"total_self_recovered"`
	TotalSessionEnded     int                             `json:"total_session_ended"`
	TotalStaleExpired     int                             `json:"total_stale_expired"`
	TotalPeakMissTokens   int64                           `json:"total_peak_miss_tokens"`
	ObservedSessions      map[string]time.Time            `json:"observed_sessions,omitempty"`
	LastNotifiedAt        map[string]time.Time            `json:"last_notified_at,omitempty"`
	HourlyWouldNotify     map[string]int                  `json:"hourly_would_notify,omitempty"`
	ModelStats            map[string]*CacheMissModelStats `json:"model_stats,omitempty"`
	Episodes              []CacheMissShadowEpisode        `json:"episodes,omitempty"`
}

// CacheMissAuditSummary is the JSON snapshot exposed by GET /api/v1/audit/cache-miss.
type CacheMissAuditSummary struct {
	Mode                 string                         `json:"mode"`
	ClientBadgeSilenced  bool                           `json:"client_badge_silenced"`
	StatePath            string                         `json:"state_path,omitempty"`
	ReportPath           string                         `json:"report_path,omitempty"`
	StartedAt            time.Time                      `json:"started_at"`
	UpdatedAt            time.Time                      `json:"updated_at"`
	ObservedSessions     int                            `json:"observed_sessions"`
	TotalChecks          int                            `json:"total_checks"`
	OverallHitRatePct    float64                        `json:"overall_hit_rate_pct"`
	TotalEpisodes        int                            `json:"total_episodes"`
	TotalWouldNotify     int                            `json:"total_would_notify"`
	OngoingEpisodes      int                            `json:"ongoing_episodes"`
	SelfRecoveredCount   int                            `json:"self_recovered_count"`
	ResolvedCount        int                            `json:"resolved_count"`
	SelfRecoveryRatePct  float64                        `json:"self_recovery_rate_pct"`
	TotalPeakMissTokens  int64                          `json:"total_peak_miss_tokens"`
	RecentEpisodes       []CacheMissShadowEpisode       `json:"recent_episodes,omitempty"`
}

// CacheMissShadowAuditor records shadow cache-miss telemetry, detects self-recovery,
// persists JSON audit state, and automatically maintains a Markdown observation report.
type CacheMissShadowAuditor struct {
	mu           sync.Mutex
	statePath    string
	reportPath   string
	nowFunc      func() time.Time
	state        cacheMissAuditState
	lastSample   map[string]string
	lastFlushAt  time.Time
	dirty        bool
}

// NewCacheMissShadowAuditor constructs an auditor. Passing empty paths keeps it memory-only (ideal for tests).
func NewCacheMissShadowAuditor(statePath, reportPath string) *CacheMissShadowAuditor {
	now := time.Now()
	a := &CacheMissShadowAuditor{
		statePath:  strings.TrimSpace(statePath),
		reportPath: strings.TrimSpace(reportPath),
		state: cacheMissAuditState{
			Version:           1,
			StartedAt:         now,
			UpdatedAt:         now,
			ObservedSessions:  make(map[string]time.Time),
			LastNotifiedAt:    make(map[string]time.Time),
			HourlyWouldNotify: make(map[string]int),
			ModelStats:        make(map[string]*CacheMissModelStats),
		},
		lastSample: make(map[string]string),
	}
	a.loadFromDisk()
	return a
}

func (a *CacheMissShadowAuditor) now(override time.Time) time.Time {
	if !override.IsZero() {
		return override
	}
	if a != nil && a.nowFunc != nil {
		return a.nowFunc()
	}
	return time.Now()
}

// ReportPath returns the target Markdown report path (if configured).
func (a *CacheMissShadowAuditor) ReportPath() string {
	if a == nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.reportPath
}

// StatePath returns the JSON audit state file path (if configured).
func (a *CacheMissShadowAuditor) StatePath() string {
	if a == nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.statePath
}

// HasOngoingEpisode reports whether cascadeID currently has an open shadow episode.
func (a *CacheMissShadowAuditor) HasOngoingEpisode(cascadeID string) bool {
	if a == nil {
		return false
	}
	cascadeID = strings.TrimSpace(cascadeID)
	if cascadeID == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.findOngoingEpisodeIndexLocked(cascadeID) >= 0
}

func (a *CacheMissShadowAuditor) loadFromDisk() {
	if a.statePath == "" {
		return
	}
	raw, err := os.ReadFile(a.statePath)
	if err != nil {
		return
	}
	var loaded cacheMissAuditState
	if err := json.Unmarshal(raw, &loaded); err != nil {
		slog.Warn("[CacheMissShadow] Failed to parse existing audit state, starting fresh", "path", a.statePath, "err", err)
		return
	}
	if loaded.StartedAt.IsZero() {
		loaded.StartedAt = time.Now()
	}
	if loaded.ObservedSessions == nil {
		loaded.ObservedSessions = make(map[string]time.Time)
	}
	if loaded.LastNotifiedAt == nil {
		loaded.LastNotifiedAt = make(map[string]time.Time)
	}
	if loaded.HourlyWouldNotify == nil {
		loaded.HourlyWouldNotify = make(map[string]int)
	}
	if loaded.ModelStats == nil {
		loaded.ModelStats = make(map[string]*CacheMissModelStats)
	}
	a.state = loaded
}

// EnsureReport guarantees the Markdown observation report exists on disk immediately on startup.
func (a *CacheMissShadowAuditor) EnsureReport() error {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.flushLocked(true)
}

// Observe ingests a prompt-cache observation, updates baseline statistics, opens/updates
// a shadow episode when alert thresholds are met, or marks an active episode as self-recovered.
// Returns (isNewEpisode, wouldHaveNotified).
func (a *CacheMissShadowAuditor) Observe(obs CacheMissObservation) (bool, bool) {
	if a == nil {
		return false, false
	}
	cascadeID := strings.TrimSpace(obs.CascadeID)
	if cascadeID == "" {
		return false, false
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	now := a.now(obs.Now)
	if a.state.StartedAt.IsZero() {
		a.state.StartedAt = now
	}
	a.state.UpdatedAt = now
	a.recordObservedSessionLocked(cascadeID, now)

	modelKey := normalizeAuditModel(obs.Model)
	title := sanitizeAuditTitle(obs.SessionTitle)
	hitRate := extractObservationHitRate(obs)
	meetsTrigger := obs.Streak >= cacheMissAlertMinStreak && obs.MissTokens >= cacheMissAlertMinTokens

	// Deduplicate identical consecutive snapshots (especially from high-frequency WS trajectory updates)
	// so baseline check counts reflect real LLM steps instead of polling cadence.
	sampleSig := fmt.Sprintf("%d:%d:%d:%d:%d", obs.InputTokens, obs.CacheReadTokens, obs.Streak, obs.MissTokens, obs.StepCount)
	isNewUsageSample := obs.Source == "direct" || a.lastSample[cascadeID] != sampleSig
	if isNewUsageSample {
		a.lastSample[cascadeID] = sampleSig
		if obs.InputTokens > 0 || obs.CacheReadTokens > 0 || obs.CacheHitRate != nil || meetsTrigger {
			a.state.TotalChecks++
			ms := a.modelStatsLocked(modelKey)
			ms.Checks++
			if hitRate >= cacheMissRecoveryMinHitRate {
				a.state.TotalHitChecks++
				ms.HitChecks++
			}
			if obs.Streak >= 2 {
				a.state.TotalStreak2Plus++
				ms.MissStreak2Plus++
			}
			a.dirty = true
		}
	}

	activeIdx := a.findOngoingEpisodeIndexLocked(cascadeID)

	if meetsTrigger {
		if activeIdx >= 0 {
			ep := &a.state.Episodes[activeIdx]
			ep.LastObservedAt = now
			if isNewUsageSample {
				ep.SampleCount++
			}
			peakChanged := false
			if obs.Streak > ep.PeakStreak {
				ep.PeakStreak = obs.Streak
				peakChanged = true
			}
			if obs.MissTokens > ep.PeakMissTokens {
				a.state.TotalPeakMissTokens += obs.MissTokens - ep.PeakMissTokens
				ep.PeakMissTokens = obs.MissTokens
				peakChanged = true
			}
			if hitRate < ep.LowestHitRate {
				ep.LowestHitRate = hitRate
			}
			if obs.UsedTokens > ep.UsedTokens {
				ep.UsedTokens = obs.UsedTokens
			}
			if obs.MaxTokens > 0 {
				ep.MaxTokens = obs.MaxTokens
			}
			if obs.ContextPct > ep.ContextPct {
				ep.ContextPct = obs.ContextPct
			}
			if obs.TTFTSeconds > ep.TTFTSeconds {
				ep.TTFTSeconds = obs.TTFTSeconds
			}
			if obs.StepCount > ep.StepCount {
				ep.StepCount = obs.StepCount
			}
			if (ep.SessionTitle == "" || ep.SessionTitle == MsgUntitledSession) && title != "" {
				ep.SessionTitle = title
			}
			if (ep.Model == "" || ep.Model == "Unknown") && modelKey != "Unknown" {
				ep.Model = modelKey
			}
			a.dirty = true
			if peakChanged || now.Sub(a.lastFlushAt) >= cacheMissAuditFlushDebounce {
				_ = a.flushLocked(false)
			}
			return false, false
		}

		// New episode triggered
		a.state.NextSeq++
		wouldNotify := true
		if lastNotif, ok := a.state.LastNotifiedAt[cascadeID]; ok && now.Sub(lastNotif) < cacheMissAlertCooldown {
			wouldNotify = false
		}
		if wouldNotify {
			a.state.LastNotifiedAt[cascadeID] = now
			a.state.TotalWouldNotify++
			hourKey := fmt.Sprintf("%02d", now.Local().Hour())
			a.state.HourlyWouldNotify[hourKey]++
		}
		mode := strings.TrimSpace(obs.Mode)
		if mode == "" {
			mode = CacheMissAlertMode()
		}
		ep := CacheMissShadowEpisode{
			EpisodeID:         fmt.Sprintf("%s#%d", shortID(cascadeID), a.state.NextSeq),
			CascadeID:         cascadeID,
			SessionTitle:      title,
			Model:             modelKey,
			Source:            obs.Source,
			Mode:              mode,
			Status:            "ongoing",
			TriggeredAt:       now,
			LastObservedAt:    now,
			InitialStreak:     obs.Streak,
			PeakStreak:        obs.Streak,
			InitialMissTokens: obs.MissTokens,
			PeakMissTokens:    obs.MissTokens,
			InitialHitRate:    roundOneDecimal(hitRate),
			LowestHitRate:     roundOneDecimal(hitRate),
			UsedTokens:        obs.UsedTokens,
			MaxTokens:         obs.MaxTokens,
			ContextPct:        roundOneDecimal(obs.ContextPct),
			TTFTSeconds:       roundOneDecimal(obs.TTFTSeconds),
			StepCount:         obs.StepCount,
			WouldHaveNotified: wouldNotify,
			SampleCount:       1,
		}
		a.state.Episodes = append(a.state.Episodes, ep)
		a.state.TotalEpisodes++
		a.state.TotalPeakMissTokens += obs.MissTokens
		ms := a.modelStatsLocked(modelKey)
		ms.ShadowTriggers++
		if wouldNotify {
			ms.WouldNotifyCount++
		}
		a.trimEpisodesLocked()
		a.dirty = true
		_ = a.flushLocked(true)
		return true, wouldNotify
	}

	// Not currently meeting trigger: if there is an active ongoing episode and cache recovered, resolve it!
	if activeIdx >= 0 {
		ep := &a.state.Episodes[activeIdx]
		ep.LastObservedAt = now
		if obs.StepCount > ep.StepCount {
			ep.StepCount = obs.StepCount
		}
		recovered := obs.Streak == 0 && (hitRate >= cacheMissRecoveryMinHitRate || (obs.CacheReadTokens > 0 && obs.CacheReadTokens >= obs.InputTokens))
		if recovered {
			resolvedAt := now
			ep.Status = "self_recovered"
			ep.ResolvedAt = &resolvedAt
			ep.RecoveryHitRate = roundOneDecimal(hitRate)
			a.state.TotalSelfRecovered++
			a.modelStatsLocked(normalizeAuditModel(ep.Model)).SelfRecovered++
			a.dirty = true
			_ = a.flushLocked(true)
			slog.Info("[CacheMissShadow] ✅ 影子审计事件自动恢复（疑似偶发缓存漂移）",
				"episode", ep.EpisodeID,
				"cascade", shortID(cascadeID),
				"peak_streak", ep.PeakStreak,
				"peak_tokens", formatCompactTokens(ep.PeakMissTokens),
				"recovery_hit_rate", fmt.Sprintf("%.1f%%", ep.RecoveryHitRate),
			)
		}
	}

	if a.dirty && now.Sub(a.lastFlushAt) >= cacheMissAuditFlushDebounce {
		_ = a.flushLocked(false)
	}
	return false, false
}

// FinalizeSession closes any ongoing shadow episode when a session leaves RUNNING state.
func (a *CacheMissShadowAuditor) FinalizeSession(cascadeID, terminalStatus string, totalSteps int, nowOverride time.Time) {
	if a == nil {
		return
	}
	cascadeID = strings.TrimSpace(cascadeID)
	if cascadeID == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.lastSample, cascadeID)
	idx := a.findOngoingEpisodeIndexLocked(cascadeID)
	if idx < 0 {
		return
	}
	now := a.now(nowOverride)
	ep := &a.state.Episodes[idx]
	ep.Status = "session_ended"
	ep.LastObservedAt = now
	ep.ResolvedAt = &now
	ep.TerminalStatus = strings.TrimSpace(terminalStatus)
	if totalSteps > ep.StepCount {
		ep.StepCount = totalSteps
	}
	a.state.UpdatedAt = now
	a.state.TotalSessionEnded++
	a.modelStatsLocked(normalizeAuditModel(ep.Model)).SessionEnded++
	a.dirty = true
	_ = a.flushLocked(true)
}

// ExpireStale marks long-unupdated ongoing episodes as stale_expired and refreshes the report.
func (a *CacheMissShadowAuditor) ExpireStale(maxAge time.Duration, nowOverride time.Time) int {
	if a == nil {
		return 0
	}
	if maxAge <= 0 {
		maxAge = cacheMissAuditStaleAge
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	now := a.now(nowOverride)
	expired := 0
	for i := range a.state.Episodes {
		ep := &a.state.Episodes[i]
		if ep.Status == "ongoing" && now.Sub(ep.LastObservedAt) >= maxAge {
			resolvedAt := now
			ep.Status = "stale_expired"
			ep.ResolvedAt = &resolvedAt
			a.state.TotalStaleExpired++
			expired++
		}
	}
	if expired > 0 || a.dirty {
		a.state.UpdatedAt = now
		a.dirty = true
		_ = a.flushLocked(true)
	}
	return expired
}

// Summary returns a thread-safe snapshot for HTTP API inspection.
func (a *CacheMissShadowAuditor) Summary() CacheMissAuditSummary {
	if a == nil {
		return CacheMissAuditSummary{Mode: CacheMissAlertMode()}
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	ongoing := 0
	for _, ep := range a.state.Episodes {
		if ep.Status == "ongoing" {
			ongoing++
		}
	}
	resolved := a.state.TotalSelfRecovered + a.state.TotalSessionEnded + a.state.TotalStaleExpired
	var recRate float64
	if resolved > 0 {
		recRate = roundOneDecimal(float64(a.state.TotalSelfRecovered) / float64(resolved) * 100.0)
	}
	var overallHit float64
	if a.state.TotalChecks > 0 {
		overallHit = roundOneDecimal(float64(a.state.TotalHitChecks) / float64(a.state.TotalChecks) * 100.0)
	}
	const maxRecent = 20
	recent := make([]CacheMissShadowEpisode, 0, minInt(len(a.state.Episodes), maxRecent))
	for i := len(a.state.Episodes) - 1; i >= 0 && len(recent) < maxRecent; i-- {
		recent = append(recent, a.state.Episodes[i])
	}
	return CacheMissAuditSummary{
		Mode:                CacheMissAlertMode(),
		ClientBadgeSilenced: !isCacheMissAlertLive(),
		StatePath:           a.statePath,
		ReportPath:          a.reportPath,
		StartedAt:           a.state.StartedAt,
		UpdatedAt:           a.state.UpdatedAt,
		ObservedSessions:    len(a.state.ObservedSessions),
		TotalChecks:         a.state.TotalChecks,
		OverallHitRatePct:   overallHit,
		TotalEpisodes:       a.state.TotalEpisodes,
		TotalWouldNotify:    a.state.TotalWouldNotify,
		OngoingEpisodes:     ongoing,
		SelfRecoveredCount:  a.state.TotalSelfRecovered,
		ResolvedCount:       resolved,
		SelfRecoveryRatePct: recRate,
		TotalPeakMissTokens: a.state.TotalPeakMissTokens,
		RecentEpisodes:      recent,
	}
}

// RenderMarkdownReport renders the complete observation report in Markdown.
func (a *CacheMissShadowAuditor) RenderMarkdownReport() string {
	if a == nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.renderMarkdownLocked(a.now(time.Time{}))
}

func (a *CacheMissShadowAuditor) findOngoingEpisodeIndexLocked(cascadeID string) int {
	for i := len(a.state.Episodes) - 1; i >= 0; i-- {
		if a.state.Episodes[i].CascadeID == cascadeID && a.state.Episodes[i].Status == "ongoing" {
			return i
		}
	}
	return -1
}

func (a *CacheMissShadowAuditor) modelStatsLocked(model string) *CacheMissModelStats {
	if a.state.ModelStats == nil {
		a.state.ModelStats = make(map[string]*CacheMissModelStats)
	}
	ms, ok := a.state.ModelStats[model]
	if !ok {
		ms = &CacheMissModelStats{}
		a.state.ModelStats[model] = ms
	}
	return ms
}

func (a *CacheMissShadowAuditor) recordObservedSessionLocked(cascadeID string, now time.Time) {
	if a.state.ObservedSessions == nil {
		a.state.ObservedSessions = make(map[string]time.Time)
	}
	a.state.ObservedSessions[cascadeID] = now
	if len(a.state.ObservedSessions) > cacheMissAuditMaxSessions {
		// Evict oldest 10%
		type pair struct {
			id string
			ts time.Time
		}
		list := make([]pair, 0, len(a.state.ObservedSessions))
		for k, v := range a.state.ObservedSessions {
			list = append(list, pair{id: k, ts: v})
		}
		sort.Slice(list, func(i, j int) bool { return list[i].ts.Before(list[j].ts) })
		drop := len(list) / 10
		if drop < 1 {
			drop = 1
		}
		for i := 0; i < drop; i++ {
			delete(a.state.ObservedSessions, list[i].id)
		}
	}
}

func (a *CacheMissShadowAuditor) trimEpisodesLocked() {
	if len(a.state.Episodes) <= cacheMissAuditMaxEpisodes {
		return
	}
	excess := len(a.state.Episodes) - cacheMissAuditMaxEpisodes
	a.state.Episodes = append([]CacheMissShadowEpisode(nil), a.state.Episodes[excess:]...)
}

func (a *CacheMissShadowAuditor) flushLocked(force bool) error {
	now := a.now(time.Time{})
	if !force && !a.dirty {
		return nil
	}
	var firstErr error
	if a.statePath != "" {
		raw, err := json.MarshalIndent(a.state, "", "  ")
		if err != nil {
			firstErr = err
		} else if err := writeFileAtomic(a.statePath, raw, 0600); err != nil {
			firstErr = err
			slog.Warn("[CacheMissShadow] Failed to persist audit state", "path", a.statePath, "err", err)
		}
	}
	if a.reportPath != "" {
		md := a.renderMarkdownLocked(now)
		if err := writeFileAtomic(a.reportPath, []byte(md), 0644); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			slog.Warn("[CacheMissShadow] Failed to write shadow audit report", "path", a.reportPath, "err", err)
		}
	}
	a.lastFlushAt = now
	a.dirty = false
	return firstErr
}

func (a *CacheMissShadowAuditor) renderMarkdownLocked(now time.Time) string {
	var b strings.Builder

	mode := CacheMissAlertMode()
	modeLabel := "影子审计模式 (`shadow` · 仅后台记录，**不发送推送且静默前端胶囊角标**)"
	clientBadgeStatus := "观察期已静默（客户端隐藏橙色 `⚠️` 未命中角标与强显提示，仅保留只读缓存命中率；后台照常采样）"
	switch mode {
	case CacheMissModeLive:
		modeLabel = "实推模式 (`live` · 正常发送推送并同步审计)"
		clientBadgeStatus = "已启用（随 `live` 模式向 iOS / Android / Web 下发 `miss_streak` 橙色角标）"
	case CacheMissModeOff:
		modeLabel = "已关闭 (`off` · 停止审计、推送与前端胶囊角标)"
		clientBadgeStatus = "已静默（不向客户端下发 `miss_streak` 角标）"
	}

	started := a.state.StartedAt
	if started.IsZero() {
		started = now
	}
	span := now.Sub(started)
	if span < 0 {
		span = 0
	}
	spanDays := span.Hours() / 24.0
	perDayWouldNotify := 0.0
	if spanDays >= 0.04 { // >= ~1 hour
		perDayWouldNotify = float64(a.state.TotalWouldNotify) / spanDays
	}

	ongoingCount := 0
	for _, ep := range a.state.Episodes {
		if ep.Status == "ongoing" {
			ongoingCount++
		}
	}
	resolvedCount := a.state.TotalSelfRecovered + a.state.TotalSessionEnded + a.state.TotalStaleExpired
	overallHitRate := 0.0
	if a.state.TotalChecks > 0 {
		overallHitRate = float64(a.state.TotalHitChecks) / float64(a.state.TotalChecks) * 100.0
	}
	selfRecRate := 0.0
	if resolvedCount > 0 {
		selfRecRate = float64(a.state.TotalSelfRecovered) / float64(resolvedCount) * 100.0
	}

	b.WriteString("# Prompt Cache 连续未命中预警 — 影子审计观察报告\n\n")
	b.WriteString("> 本文档由本机 Multigravity 网关后台影子审计器自动生成并实时增量刷新，用于在不打扰用户的前提下评估「缓存连续未命中预警」的真实信噪比与误报率。\n\n")

	b.WriteString("## 1. 审计配置与观察窗口\n\n")
	b.WriteString("| 项目 | 当前值 |\n")
	b.WriteString("| :--- | :--- |\n")
	fmt.Fprintf(&b, "| 当前运行模式 | %s |\n", modeLabel)
	fmt.Fprintf(&b, "| 前端胶囊未命中角标 | %s |\n", clientBadgeStatus)
	fmt.Fprintf(&b, "| 审计起始时间 | `%s` |\n", started.Local().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "| 最近刷新时间 | `%s` |\n", now.Local().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "| 累计观察时长 | `%s` |\n", formatAuditDuration(span))
	fmt.Fprintf(&b, "| 影子评估基线规则 | 连续 `≥ %d` 步命中率 `< %.0f%%` 且累计未命中输入 `≥ %s`（单会话冷却 `%v`） |\n",
		cacheMissAlertMinStreak, cacheMissRecoveryMinHitRate, formatCompactTokens(cacheMissAlertMinTokens), cacheMissAlertCooldown)
	if a.statePath != "" {
		fmt.Fprintf(&b, "| 原始审计数据 | `%s` |\n", a.statePath)
	}
	b.WriteString("\n")

	b.WriteString("## 2. 核心指标摘要与自动建议\n\n")
	b.WriteString("| 指标 | 统计值 | 说明 |\n")
	b.WriteString("| :--- | ---: | :--- |\n")
	fmt.Fprintf(&b, "| 累计观察会话数 | %d | 后台监测到的活跃非子代理会话数 |\n", len(a.state.ObservedSessions))
	fmt.Fprintf(&b, "| 去重有效采样步数 | %d | 整体缓存命中率（≥20%%）：**%.1f%%** |\n", a.state.TotalChecks, overallHitRate)
	fmt.Fprintf(&b, "| 触发 ≥2 步未命中采样 | %d | 应用内 Context 胶囊橙色角标口径（观察期已对客户端静默） |\n", a.state.TotalStreak2Plus)
	fmt.Fprintf(&b, "| 影子预警总事件数 | %d | 当前仍在持续观察中：%d |\n", a.state.TotalEpisodes, ongoingCount)
	if spanDays >= 0.04 {
		fmt.Fprintf(&b, "| **原本会推送打扰次数** | **%d** | 按 1h 冷却计，折合日均约 **%.1f 次/天** |\n", a.state.TotalWouldNotify, perDayWouldNotify)
	} else {
		fmt.Fprintf(&b, "| **原本会推送打扰次数** | **%d** | 按 1h 冷却计（观察满 1 小时后自动折算日均频次） |\n", a.state.TotalWouldNotify)
	}
	fmt.Fprintf(&b, "| 已结案事件自愈数 | %d / %d | **自愈率（疑似偶发误报率）：%.1f%%** |\n", a.state.TotalSelfRecovered, resolvedCount, selfRecRate)
	fmt.Fprintf(&b, "| 会话自然结束 / 超时结案 | %d / %d | 触发后直至会话停止仍未恢复或超时结案 |\n", a.state.TotalSessionEnded, a.state.TotalStaleExpired)
	fmt.Fprintf(&b, "| 影子事件累计峰值重算 | %s | 全部影子事件峰值未命中输入 Token 总和 |\n", formatCompactTokens(a.state.TotalPeakMissTokens))
	b.WriteString("\n")

	b.WriteString("### 自动评估结论\n\n")
	switch {
	case resolvedCount < 10:
		fmt.Fprintf(&b, "- ⏳ **样本积累期**：当前已结案影子事件仅 `%d` 起（建议至少积累 `≥ 10` 起已结案事件或持续观察 `3~7` 天），请继续保持 `shadow` 静默审计，暂不建议开启实时推送。\n\n", resolvedCount)
	case selfRecRate >= 60.0 || (spanDays >= 1.0 && perDayWouldNotify >= 3.0):
		fmt.Fprintf(&b, "- 🚫 **不建议按当前阈值开启推送**：已结案事件中 **%.1f%%** 在无人工干预的情况下自动恢复命中（折合日均拟推送 **%.1f 次/天**），属于典型隐式缓存副本漂移噪声。建议维持关闭/影子模式，或参考下方反事实矩阵仅考虑极高阈值。\n\n", selfRecRate, perDayWouldNotify)
	case selfRecRate >= 35.0:
		fmt.Fprintf(&b, "- ⚠️ **信噪比偏弱，建议上调阈值再观察**：自愈率为 **%.1f%%**，现行 `3步/120K` 门槛仍偏敏感，建议结合下方反事实表评估 `5步/500K` 或 `6步/1.0M` 档位。\n\n", selfRecRate)
	default:
		fmt.Fprintf(&b, "- ✅ **信号较稳定（自愈率 %.1f%%）**：多数触发事件未自行恢复，具备进一步评估转为实推（`CACHE_MISS_ALERT_MODE=live`）的基础。\n\n", selfRecRate)
	}

	b.WriteString("## 3. 阈值反事实模拟（What-If 降噪评估）\n\n")
	if len(a.state.Episodes) == 0 {
		b.WriteString("_尚无已触发的影子事件，等待后台持续采集。_\n\n")
	} else {
		type thresholdTier struct {
			name      string
			minStreak int
			minTokens int64
		}
		tiers := []thresholdTier{
			{"现行基线 (≥3步 & ≥120K)", 3, 120_000},
			{"中档过滤 (≥4步 & ≥300K)", 4, 300_000},
			{"较严过滤 (≥5步 & ≥500K)", 5, 500_000},
			{"严格过滤 (≥6步 & ≥1.0M)", 6, 1_000_000},
			{"极严过滤 (≥8步 & ≥1.5M)", 8, 1_500_000},
		}
		baseCount := len(a.state.Episodes)
		b.WriteString("| 候选阈值档位 | 命中事件数 | 较基线降噪 | 其中自动恢复 | 未恢复结案 | 观察中 |\n")
		b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: |\n")
		for _, tier := range tiers {
			matched, rec, unrec, ong := 0, 0, 0, 0
			for _, ep := range a.state.Episodes {
				if ep.PeakStreak >= tier.minStreak && ep.PeakMissTokens >= tier.minTokens {
					matched++
					switch ep.Status {
					case "self_recovered":
						rec++
					case "ongoing":
						ong++
					default:
						unrec++
					}
				}
			}
			reduction := 0.0
			if baseCount > 0 {
				reduction = (1.0 - float64(matched)/float64(baseCount)) * 100.0
			}
			fmt.Fprintf(&b, "| %s | %d | -%.1f%% | %d | %d | %d |\n",
				tier.name, matched, reduction, rec, unrec, ong)
		}
		b.WriteString("\n")
	}

	b.WriteString("## 4. 按模型维度统计\n\n")
	if len(a.state.ModelStats) == 0 {
		b.WriteString("_尚无模型采样记录。_\n\n")
	} else {
		type modelRow struct {
			name string
			st   *CacheMissModelStats
		}
		rows := make([]modelRow, 0, len(a.state.ModelStats))
		for k, v := range a.state.ModelStats {
			rows = append(rows, modelRow{name: k, st: v})
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].st.ShadowTriggers != rows[j].st.ShadowTriggers {
				return rows[i].st.ShadowTriggers > rows[j].st.ShadowTriggers
			}
			return rows[i].st.Checks > rows[j].st.Checks
		})
		b.WriteString("| 模型 | 采样步数 | 命中率 (≥20%) | 影子触发 | 拟推送数 | 自动恢复 | 自愈率 |\n")
		b.WriteString("| :--- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
		for _, r := range rows {
			hr := 0.0
			if r.st.Checks > 0 {
				hr = float64(r.st.HitChecks) / float64(r.st.Checks) * 100.0
			}
			closed := r.st.SelfRecovered + r.st.SessionEnded
			rrText := "—"
			if closed > 0 {
				rrText = fmt.Sprintf("%.1f%%", float64(r.st.SelfRecovered)/float64(closed)*100.0)
			}
			fmt.Fprintf(&b, "| %s | %d | %.1f%% | %d | %d | %d | %s |\n",
				escapeMDCell(r.name), r.st.Checks, hr, r.st.ShadowTriggers, r.st.WouldNotifyCount, r.st.SelfRecovered, rrText)
		}
		b.WriteString("\n")
	}

	if len(a.state.HourlyWouldNotify) > 0 {
		b.WriteString("## 5. 拟推送时段分布（本地小时）\n\n")
		hours := make([]string, 0, len(a.state.HourlyWouldNotify))
		for h := range a.state.HourlyWouldNotify {
			hours = append(hours, h)
		}
		sort.Strings(hours)
		b.WriteString("| 时段 | 拟推送次数 |\n")
		b.WriteString("| :--- | ---: |\n")
		for _, h := range hours {
			fmt.Fprintf(&b, "| `%s:00 ~ %s:59` | %d |\n", h, h, a.state.HourlyWouldNotify[h])
		}
		b.WriteString("\n")
	}

	b.WriteString("## 6. 近期影子触发事件明细（最多展示最近 50 条）\n\n")
	if len(a.state.Episodes) == 0 {
		b.WriteString("_暂未捕获到达到阈值的连续未命中事件。后台正在静默运行中，有事件后将自动更新此表。_\n\n")
	} else {
		b.WriteString("| 触发时间 | 会话 | 模型 | 连错步数 | 峰值重算 | 上下文 | 原本推送 | 结局 |\n")
		b.WriteString("| :--- | :--- | :--- | ---: | ---: | ---: | :---: | :--- |\n")
		shown := 0
		for i := len(a.state.Episodes) - 1; i >= 0 && shown < 50; i-- {
			ep := a.state.Episodes[i]
			shown++
			streakStr := fmt.Sprintf("%d", ep.PeakStreak)
			if ep.InitialStreak != ep.PeakStreak {
				streakStr = fmt.Sprintf("%d→%d", ep.InitialStreak, ep.PeakStreak)
			}
			ctxStr := "—"
			if ep.ContextPct > 0 {
				ctxStr = fmt.Sprintf("%.0f%%", ep.ContextPct)
			}
			notifyMark := "冷却跳过"
			if ep.WouldHaveNotified {
				notifyMark = "会推送"
			}
			outcome := formatEpisodeOutcome(ep)
			title := ep.SessionTitle
			if title == "" {
				title = shortID(ep.CascadeID)
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s | %s | %s | %s |\n",
				ep.TriggeredAt.Local().Format("01-02 15:04:05"),
				escapeMDCell(truncateString(title, 34)),
				escapeMDCell(truncateString(ep.Model, 24)),
				streakStr,
				formatCompactTokens(ep.PeakMissTokens),
				ctxStr,
				notifyMark,
				escapeMDCell(outcome),
			)
		}
		b.WriteString("\n")
	}

	b.WriteString("## 7. 模式控制说明\n\n")
	b.WriteString("- **默认影子审计（当前推荐）**：无需额外配置（或 `CACHE_MISS_ALERT_MODE=shadow`），同时静默**手机推送弹窗**与**聊天页顶部 Context 胶囊的橙色未命中角标**（仅保留只读缓存命中率明细），后台持续收集样本并自动刷新本报告。\n")
	b.WriteString("- **彻底静默关闭**：在 `~/.multigravity/.env` 设置 `CACHE_MISS_ALERT_MODE=off`（或在移动端「设置 → 提醒内容」关闭「缓存连续未命中」开关暂停后台审计）。\n")
	b.WriteString("- **仅在观察达标后转为实推**：设置 `CACHE_MISS_ALERT_MODE=live` 并重启网关，即可同步恢复推送与客户端橙色胶囊未命中角标。\n")

	return b.String()
}

func formatEpisodeOutcome(ep CacheMissShadowEpisode) string {
	switch ep.Status {
	case "self_recovered":
		dur := ""
		if ep.ResolvedAt != nil {
			d := ep.ResolvedAt.Sub(ep.TriggeredAt)
			if d < 0 {
				d = 0
			}
			dur = ", " + formatAuditDuration(d)
		}
		return fmt.Sprintf("✅ 自动恢复 (命中 %.0f%%%s)", ep.RecoveryHitRate, dur)
	case "session_ended":
		st := strings.TrimPrefix(ep.TerminalStatus, "CASCADE_RUN_STATUS_")
		if st == "" {
			st = "ENDED"
		}
		return fmt.Sprintf("⏹ 会话结束 (%s)", st)
	case "stale_expired":
		return "⌛ 超时结案"
	default:
		return "⏳ 持续观察中"
	}
}

func formatAuditDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if hours < 24 {
		return fmt.Sprintf("%dh%dm", hours, mins)
	}
	days := hours / 24
	remHours := hours % 24
	return fmt.Sprintf("%dd%dh", days, remHours)
}

func extractObservationHitRate(obs CacheMissObservation) float64 {
	if obs.CacheHitRate != nil {
		return *obs.CacheHitRate
	}
	total := obs.InputTokens + obs.CacheReadTokens
	if total <= 0 {
		return 0
	}
	return float64(obs.CacheReadTokens) / float64(total) * 100.0
}

func normalizeAuditModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "Unknown"
	}
	return model
}

func sanitizeAuditTitle(title string) string {
	clean := strings.TrimSpace(SanitizePushText(title))
	if clean == "" || clean == MsgUntitledSession {
		return ""
	}
	return clean
}

func escapeMDCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return strings.TrimSpace(s)
}

func roundOneDecimal(v float64) float64 {
	return float64(int(v*10+0.5)) / 10.0
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmpPath := path + ".tmp." + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmpPath, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
