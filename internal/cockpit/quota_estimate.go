package cockpit

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/config"
)

const (
	poolUsageRetention      = 8 * 24 * time.Hour
	poolUsageMaxEvents      = 8000
	poolUsageFlushDebounce  = 25 * time.Second
	poolUsageEventsFilename = "quota_usage_events.json"
	quotaCalFilename        = "quota_calibration.json"
	quotaCalTTL5h           = 14 * 24 * time.Hour
	quotaCalTTLWeekly       = 21 * 24 * time.Hour
	quotaMinUsedFracForCal  = 0.05
)

type poolUsageEvent struct {
	TS         int64  `json:"ts"`
	Pool       string `json:"pool"` // "gemini_main" | "gemini_4_argon" | "claude"
	Tokens     int64  `json:"tokens"`
	ResponseID string `json:"rid,omitempty"`
}

type poolUsageDiskState struct {
	Version         int              `json:"version"`
	CoverageStartMs int64            `json:"coverage_start_ms,omitempty"`
	Events          []poolUsageEvent `json:"events"`
}

type bucketCalibrationEntry struct {
	LastResetMs          int64   `json:"last_reset_ms,omitempty"`
	LastFraction         float64 `json:"last_fraction"`
	ObservedFromStart    bool    `json:"observed_from_start,omitempty"`
	FullStartResetMs     int64   `json:"full_start_reset_ms,omitempty"`
	CalibratedTotalToks  int64   `json:"calibrated_total_tokens,omitempty"`
	CalibratedAtMs       int64   `json:"calibrated_at_ms,omitempty"`
	CalibratedSampleCnt  int     `json:"calibrated_sample_count,omitempty"`
}

type quotaCalibrationDiskState struct {
	Version int                               `json:"version"`
	Entries map[string]bucketCalibrationEntry `json:"entries"`
}

var (
	poolUsageMu        sync.Mutex
	poolUsageLoaded    bool
	poolUsagePath      string
	poolUsageState     poolUsageDiskState
	poolUsageSeenRIDs  map[string]struct{}
	poolUsageLastFlush time.Time

	quotaCalMu        sync.Mutex
	quotaCalLoaded    bool
	quotaCalPath      string
	quotaCalEntries   map[string]bucketCalibrationEntry
)

// ResetQuotaEstimateCaches clears in-memory usage events & calibration caches (used by tests).
func ResetQuotaEstimateCaches() {
	poolUsageMu.Lock()
	poolUsageLoaded = false
	poolUsagePath = ""
	poolUsageState = poolUsageDiskState{}
	poolUsageSeenRIDs = nil
	poolUsageLastFlush = time.Time{}
	poolUsageMu.Unlock()

	quotaCalMu.Lock()
	quotaCalLoaded = false
	quotaCalPath = ""
	quotaCalEntries = nil
	quotaCalMu.Unlock()
}

// PoolKeyForModel classifies a model display name / slug / enum into its Antigravity quota pool.
// Orchestrator (Argon) is explicitly routed to "gemini_main" so it never contaminates the standalone
// Gemini 4 Argon pool.
func PoolKeyForModel(rawModel string) string {
	resolved := resolveGenMetadataModelName("", rawModel, rawModel)
	lower := strings.ToLower(strings.TrimSpace(resolved))
	rawLower := strings.ToLower(strings.TrimSpace(rawModel))
	if lower == "" {
		lower = rawLower
	}
	if lower == "" || lower == "default" {
		return ""
	}
	if strings.Contains(lower, "claude") || strings.Contains(lower, "gpt") {
		return "claude"
	}
	if strings.Contains(lower, "orchestrator") || strings.Contains(rawLower, "orchestrator") {
		return "gemini_main"
	}
	if isArgonModelOrBucket(lower) || isArgonModelOrBucket(rawLower) {
		return "gemini_4_argon"
	}
	if strings.Contains(lower, "gemini") || strings.Contains(lower, "neon") ||
		strings.Contains(lower, "carbon") || strings.Contains(lower, "hydrogen") ||
		strings.Contains(lower, "flash") {
		return "gemini_main"
	}
	return ""
}

func poolUsageFilePath() string {
	dir := strings.TrimSpace(config.GetDataDir())
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, poolUsageEventsFilename)
}

func quotaCalFilePath() string {
	dir := strings.TrimSpace(config.GetDataDir())
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, quotaCalFilename)
}

func ensurePoolUsageLoadedLocked() {
	want := poolUsageFilePath()
	if poolUsageLoaded && poolUsagePath == want {
		return
	}
	poolUsageLoaded = true
	poolUsagePath = want
	poolUsageState = poolUsageDiskState{Version: 1}
	poolUsageSeenRIDs = make(map[string]struct{})
	if want == "" {
		return
	}
	raw, err := os.ReadFile(want)
	if err != nil {
		return
	}
	var st poolUsageDiskState
	if err := json.Unmarshal(raw, &st); err != nil {
		return
	}
	cutoff := time.Now().Add(-poolUsageRetention).UnixMilli()
	kept := make([]poolUsageEvent, 0, len(st.Events))
	for _, ev := range st.Events {
		if ev.TS >= cutoff && ev.Tokens > 0 && ev.Pool != "" {
			kept = append(kept, ev)
			if ev.ResponseID != "" {
				poolUsageSeenRIDs[ev.ResponseID] = struct{}{}
			}
		}
	}
	st.Events = kept
	poolUsageState = st
}

// RecordPoolUsageEvent records a timestamped token consumption event for quota token estimation.
func RecordPoolUsageEvent(ts time.Time, model string, input, cacheRead, output int64, responseID string) {
	total := input + cacheRead + output
	if total <= 0 {
		return
	}
	pool := PoolKeyForModel(model)
	if pool == "" {
		return
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	tsMs := ts.UnixMilli()
	rid := strings.TrimSpace(responseID)

	poolUsageMu.Lock()
	defer poolUsageMu.Unlock()
	ensurePoolUsageLoadedLocked()

	if rid != "" {
		if _, dup := poolUsageSeenRIDs[rid]; dup {
			return
		}
		poolUsageSeenRIDs[rid] = struct{}{}
	}

	if poolUsageState.CoverageStartMs == 0 || tsMs < poolUsageState.CoverageStartMs {
		poolUsageState.CoverageStartMs = tsMs
	}
	poolUsageState.Events = append(poolUsageState.Events, poolUsageEvent{
		TS:         tsMs,
		Pool:       pool,
		Tokens:     total,
		ResponseID: rid,
	})

	cutoff := time.Now().Add(-poolUsageRetention).UnixMilli()
	if len(poolUsageState.Events) > poolUsageMaxEvents {
		trimmed := make([]poolUsageEvent, 0, poolUsageMaxEvents)
		for _, ev := range poolUsageState.Events {
			if ev.TS >= cutoff {
				trimmed = append(trimmed, ev)
			}
		}
		if len(trimmed) > poolUsageMaxEvents {
			trimmed = trimmed[len(trimmed)-poolUsageMaxEvents:]
		}
		poolUsageState.Events = trimmed
	}

	if poolUsagePath != "" && (poolUsageLastFlush.IsZero() || time.Since(poolUsageLastFlush) >= poolUsageFlushDebounce) {
		if b, err := json.Marshal(poolUsageState); err == nil {
			_ = os.MkdirAll(filepath.Dir(poolUsagePath), 0700)
			if writeFileAtomic0600(poolUsagePath, b) == nil {
				poolUsageLastFlush = time.Now()
			}
		}
	}
}

func snapshotPoolUsageEvents() (events []poolUsageEvent, coverageStartMs int64) {
	poolUsageMu.Lock()
	defer poolUsageMu.Unlock()
	ensurePoolUsageLoadedLocked()
	events = append([]poolUsageEvent(nil), poolUsageState.Events...)
	coverageStartMs = poolUsageState.CoverageStartMs
	return events, coverageStartMs
}

func ensureQuotaCalLoadedLocked() {
	want := quotaCalFilePath()
	if quotaCalLoaded && quotaCalPath == want {
		return
	}
	quotaCalLoaded = true
	quotaCalPath = want
	quotaCalEntries = make(map[string]bucketCalibrationEntry)
	if want == "" {
		return
	}
	raw, err := os.ReadFile(want)
	if err != nil {
		return
	}
	var st quotaCalibrationDiskState
	if err := json.Unmarshal(raw, &st); err != nil || st.Entries == nil {
		return
	}
	quotaCalEntries = st.Entries
}

func saveQuotaCalLocked() {
	if quotaCalPath == "" {
		return
	}
	b, err := json.Marshal(quotaCalibrationDiskState{
		Version: 1,
		Entries: quotaCalEntries,
	})
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(quotaCalPath), 0700)
	_ = writeFileAtomic0600(quotaCalPath, b)
}

func parseQuotaResetTime(raw string) (time.Time, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ApplyQuotaTokenEstimates computes estimated cycle-used and remaining tokens for the current account's
// quota buckets using observed pool token consumption and calibrated total pool capacity.
func ApplyQuotaTokenEstimates(resp *CockpitQuotaResponse, now time.Time) {
	if resp == nil || resp.CurrentAccount == nil {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	acc := resp.CurrentAccount
	if acc.SubscriptionTier != "" {
		SetLastKnownSubscriptionTier(acc.SubscriptionTier)
	}
	acctKey := strings.ToLower(strings.TrimSpace(acc.Email))
	if acctKey == "" {
		acctKey = strings.ToLower(strings.TrimSpace(acc.ID))
	}
	if acctKey == "" {
		acctKey = "current"
	}

	events, coverageStartMs := snapshotPoolUsageEvents()
	isUltra := strings.Contains(strings.ToUpper(acc.SubscriptionTier), "ULTRA")

	type bucketSpec struct {
		key       string
		pool      string
		windowDur time.Duration
		calTTL    time.Duration
		metric    **QuotaMetric
	}
	argonDur := 168 * time.Hour
	if isUltra {
		argonDur = 5 * time.Hour
	}

	specs := []bucketSpec{
		{key: "gemini_5h", pool: "gemini_main", windowDur: 5 * time.Hour, calTTL: quotaCalTTL5h, metric: &acc.Gemini5h},
		{key: "gemini_weekly", pool: "gemini_main", windowDur: 168 * time.Hour, calTTL: quotaCalTTLWeekly, metric: &acc.GeminiWeekly},
		{key: "claude_5h", pool: "claude", windowDur: 5 * time.Hour, calTTL: quotaCalTTL5h, metric: &acc.Claude5h},
		{key: "claude_weekly", pool: "claude", windowDur: 168 * time.Hour, calTTL: quotaCalTTLWeekly, metric: &acc.ClaudeWeekly},
		{key: "gemini_4_argon", pool: "gemini_4_argon", windowDur: argonDur, calTTL: quotaCalTTLWeekly, metric: &acc.Gemini4Argon},
	}

	nowMs := now.UnixMilli()
	calDirty := false

	quotaCalMu.Lock()
	ensureQuotaCalLoadedLocked()

	for _, sp := range specs {
		m := *sp.metric
		if m == nil {
			continue
		}
		// Copy metric so shared cache pointers are never mutated in place.
		cp := *m
		frac := cp.RemainingFraction
		if frac == 0 && cp.RemainingPercent > 0 {
			frac = cp.RemainingPercent / 100.0
		}
		if frac < 0 {
			frac = 0
		}
		if frac > 1 {
			frac = 1
		}

		resetAt, hasReset := parseQuotaResetTime(cp.ResetTime)
		var resetMs int64
		var cycleStartMs int64
		if hasReset {
			resetMs = resetAt.UnixMilli()
			if resetMs > nowMs && time.Duration(resetMs-nowMs)*time.Millisecond <= sp.windowDur+5*time.Minute {
				cycleStartMs = resetAt.Add(-sp.windowDur).UnixMilli()
			}
		}
		if cycleStartMs == 0 {
			cycleStartMs = now.Add(-sp.windowDur).UnixMilli()
		}

		calKey := acctKey + "|" + sp.key
		prev, hasPrev := quotaCalEntries[calKey]

		resetShifted := hasReset && hasPrev && prev.LastResetMs > 0 && (resetMs-prev.LastResetMs) > int64(10*time.Minute/time.Millisecond)
		jumpedUp := hasPrev && (frac-prev.LastFraction) >= 0.04
		isNewCycle := resetShifted || jumpedUp

		prevSameFullCycle := !isNewCycle && prev.ObservedFromStart &&
			(!hasReset || prev.FullStartResetMs == resetMs)
		curObservedFull := hasReset && ((isNewCycle) || (!hasPrev && frac >= 0.98) || prevSameFullCycle)
		observedFromStart := prevSameFullCycle || curObservedFull

		hasCollectorCoverage := hasReset && coverageStartMs > 0 && coverageStartMs <= cycleStartMs+int64(5*time.Minute/time.Millisecond)
		hasFullCoverage := hasReset && (observedFromStart || hasCollectorCoverage)

		var cycleUsed int64
		for _, ev := range events {
			if ev.Pool == sp.pool && ev.TS >= cycleStartMs && ev.TS <= nowMs+60000 {
				cycleUsed += ev.Tokens
			}
		}
		cp.EstCycleUsedTokens = cycleUsed

		nextEntry := prev
		nextEntry.LastFraction = frac
		if hasReset {
			nextEntry.LastResetMs = resetMs
		}
		nextEntry.ObservedFromStart = observedFromStart
		if observedFromStart && hasReset {
			nextEntry.FullStartResetMs = resetMs
		}

		usedFrac := 1.0 - frac
		if hasFullCoverage && usedFrac >= quotaMinUsedFracForCal && cycleUsed > 0 {
			estTotal := int64(math.Round(float64(cycleUsed) / usedFrac))
			if estTotal > 0 {
				// Blend gently with prior calibration if available to dampen single-step jitter.
				if prev.CalibratedTotalToks > 0 && prev.CalibratedAtMs > 0 && time.Duration(nowMs-prev.CalibratedAtMs)*time.Millisecond <= sp.calTTL {
					estTotal = int64(math.Round(float64(estTotal)*0.65 + float64(prev.CalibratedTotalToks)*0.35))
				}
				estRem := int64(math.Round(float64(estTotal) * frac))
				cp.EstTotalTokens = estTotal
				cp.EstRemainingTokens = estRem
				cp.EstReliable = true

				nextEntry.CalibratedTotalToks = estTotal
				nextEntry.CalibratedAtMs = nowMs
				nextEntry.CalibratedSampleCnt++
			}
		} else if prev.CalibratedTotalToks > 0 && prev.CalibratedAtMs > 0 && time.Duration(nowMs-prev.CalibratedAtMs)*time.Millisecond <= sp.calTTL {
			estTotal := prev.CalibratedTotalToks
			estRem := int64(math.Round(float64(estTotal) * frac))
			cp.EstTotalTokens = estTotal
			cp.EstRemainingTokens = estRem
			cp.EstReliable = true
		}

		if nextEntry != prev || !hasPrev {
			quotaCalEntries[calKey] = nextEntry
			calDirty = true
		}
		*sp.metric = &cp
	}

	if calDirty {
		saveQuotaCalLocked()
	}
	quotaCalMu.Unlock()

	// Keep resp.Accounts in sync with updated resp.CurrentAccount.
	for i := range resp.Accounts {
		if resp.Accounts[i].ID == acc.ID || (acc.Email != "" && strings.EqualFold(resp.Accounts[i].Email, acc.Email)) {
			resp.Accounts[i] = *acc
		}
	}
}
