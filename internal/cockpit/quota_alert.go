package cockpit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/i18n"
)

// 全池智能额度守护（当前账号：Gemini 5h / Gemini Weekly / Claude 5h / Claude Weekly / Gemini 4 Argon）。
//
// 口径：
//   - 低水位预警：两档阈值（默认 ≤20% 提醒、≤5% 紧急，可由 QUOTA_ALERT_*_PERCENT 覆盖）；
//     每个「账号 × 额度池 × 重置周期」每档仅推一次并落盘；5h/Argon 池附带近 60 分钟耗尽 ETA（仅早于重置时显示），
//     周池仅显示剩余与重置时间；同轮多池同时跌破时合并为一条通知；
//   - 官方临时重置喜报（Bonus Reset）：同账号下明显早于原定重置时间（5h/Argon 提前 ≥20m、周池提前 ≥2h）
//     且大幅跃升并接近回满（≥95%）时推送；同轮多池回满合并为一条；
//   - 异常骤降与空闲静默掉额（Anomaly）：本机确认空闲 ≥20m 且前后采样均在空闲后却明显掉额（5h/Argon ≥15%、周池 ≥5%），
//     或活跃期 15m 内剧烈突降（周池 ≥15%、5h/Argon ≥45%）时推送，同池同类 4h 冷却；若同轮已触发低水位告警则不重复推；
//   - 文案按网关原生语言（zh / en / ja）输出。

const (
	defaultWarnFraction     = 0.20
	defaultCriticalFraction = 0.05

	// 预测窗口与可信度门槛：样本太少、跨度太短或几乎没消耗时宁可不预测。
	predictWindow     = 60 * time.Minute
	predictMinSamples = 3
	predictMinSpan    = 15 * time.Minute
	predictMinDrop    = 0.005 // 窗口内至少掉了 0.5 个百分点

	// 官方提前回满（Bonus Reset）防误报门槛。
	bonusMinFinalFraction   = 0.95
	bonusJumpMinShort       = 0.30
	bonusJumpMinWeekly      = 0.25
	bonusEarlyMarginShort   = 20 * time.Minute
	bonusEarlyMarginWeekly  = 2 * time.Hour
	bonusMaxPrevAgeShort    = 24 * time.Hour
	bonusMaxPrevAgeWeekly   = 7 * 24 * time.Hour

	// 异常骤降 / 空闲期静默扣额防误报门槛。
	anomalyRapidWindow = 15 * time.Minute
	anomalyCooldown    = 4 * time.Hour
	idleGraceDuration  = 20 * time.Minute
	idleSampleMargin   = 2 * time.Minute
	rapidDropMinShort  = 0.45
	rapidDropMinWeekly = 0.15
	idleDropMinShort   = 0.15
	idleDropMinWeekly  = 0.05

	// 采样保留时长与默认轮询节奏。
	sampleRetention = 3 * time.Hour
	alertPollEvery  = 30 * time.Second
)

// QuotaAlert is one notification to deliver.
type QuotaAlert struct {
	Level    string // "warn" | "critical" | "bonus_reset" | "anomaly"
	Title    string
	Body     string
	StateKey string // 唯一去重键，通知端可据此再去重
}

// QuotaAlertConfig configures the alerter.
type QuotaAlertConfig struct {
	WarnFraction      float64
	CriticalFraction  float64
	DisableBonusReset bool
	DisableAnomaly    bool
	// StatePath persists alert dedup state across gateway restarts ("" = memory only).
	StatePath string
	// LiveEmail returns the account the running language_server is really using ("" = trust Cockpit's current account).
	LiveEmail func() string
	// FetchQuota optionally provides quota snapshots directly (e.g. live language_server + Cockpit overlay),
	// enabling alerts even when Cockpit Tools is not installed.
	FetchQuota func() (*CockpitQuotaResponse, string, error)
	// HostIdleSince returns (idleSince, true) when no Antigravity session on this host is running.
	HostIdleSince func() (time.Time, bool)
	// Lang optionally overrides the active UI language ("zh", "en", "ja"); defaults to i18n.GetLang.
	Lang         func() string
	PollInterval time.Duration
	Notify       func(QuotaAlert)
	Now          func() time.Time
}

// QuotaAlertConfigFromEnv reads QUOTA_ALERT_WARN_PERCENT / QUOTA_ALERT_CRITICAL_PERCENT /
// QUOTA_ALERT_BONUS_RESET_ENABLED / QUOTA_ALERT_ANOMALY_ENABLED.
// Invalid or inconsistent values fall back to the defaults (20% / 5%, bonus reset & anomaly enabled).
func QuotaAlertConfigFromEnv() QuotaAlertConfig {
	cfg := QuotaAlertConfig{
		WarnFraction:      defaultWarnFraction,
		CriticalFraction:  defaultCriticalFraction,
		DisableBonusReset: envBoolDisabled("QUOTA_ALERT_BONUS_RESET_ENABLED"),
		DisableAnomaly:    envBoolDisabled("QUOTA_ALERT_ANOMALY_ENABLED"),
	}
	w, wOK := envPercent("QUOTA_ALERT_WARN_PERCENT")
	c, cOK := envPercent("QUOTA_ALERT_CRITICAL_PERCENT")
	switch {
	case wOK && cOK && w > c:
		cfg.WarnFraction, cfg.CriticalFraction = w, c
	case wOK && !cOK && w > cfg.CriticalFraction:
		cfg.WarnFraction = w
	case cOK && !wOK && c < cfg.WarnFraction:
		cfg.CriticalFraction = c
	case wOK || cOK:
		slog.Warn("[QuotaAlert] 阈值环境变量无效（需 0 < 紧急 < 提醒 < 100），使用默认 20% / 5%")
	}
	return cfg
}

func envPercent(name string) (float64, bool) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 || v >= 100 {
		return 0, false
	}
	return v / 100, true
}

func envBoolDisabled(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return v == "0" || v == "false" || v == "no" || v == "off"
}

type bucketSpec struct {
	id           string
	isWeekly     bool
	predictETA   bool
	earlyMargin  time.Duration
	bonusJumpMin float64
	maxPrevAge   time.Duration
	rapidDropMin float64
	idleDropMin  float64
	get          func(*AccountQuota) *QuotaMetric
	label        func(lang string) string
}

var monitoredBuckets = []bucketSpec{
	{
		id:           "gemini_5h",
		isWeekly:     false,
		predictETA:   true,
		earlyMargin:  bonusEarlyMarginShort,
		bonusJumpMin: bonusJumpMinShort,
		maxPrevAge:   bonusMaxPrevAgeShort,
		rapidDropMin: rapidDropMinShort,
		idleDropMin:  idleDropMinShort,
		get:          func(a *AccountQuota) *QuotaMetric { return a.Gemini5h },
		label:        func(lang string) string { return "Gemini 5h" },
	},
	{
		id:           "gemini_weekly",
		isWeekly:     true,
		predictETA:   false,
		earlyMargin:  bonusEarlyMarginWeekly,
		bonusJumpMin: bonusJumpMinWeekly,
		maxPrevAge:   bonusMaxPrevAgeWeekly,
		rapidDropMin: rapidDropMinWeekly,
		idleDropMin:  idleDropMinWeekly,
		get:          func(a *AccountQuota) *QuotaMetric { return a.GeminiWeekly },
		label: func(lang string) string {
			return i18n.T3For(lang, "Gemini 周额度", "Gemini Weekly", "Gemini 週間")
		},
	},
	{
		id:           "claude_5h",
		isWeekly:     false,
		predictETA:   true,
		earlyMargin:  bonusEarlyMarginShort,
		bonusJumpMin: bonusJumpMinShort,
		maxPrevAge:   bonusMaxPrevAgeShort,
		rapidDropMin: rapidDropMinShort,
		idleDropMin:  idleDropMinShort,
		get:          func(a *AccountQuota) *QuotaMetric { return a.Claude5h },
		label:        func(lang string) string { return "Claude 5h" },
	},
	{
		id:           "claude_weekly",
		isWeekly:     true,
		predictETA:   false,
		earlyMargin:  bonusEarlyMarginWeekly,
		bonusJumpMin: bonusJumpMinWeekly,
		maxPrevAge:   bonusMaxPrevAgeWeekly,
		rapidDropMin: rapidDropMinWeekly,
		idleDropMin:  idleDropMinWeekly,
		get:          func(a *AccountQuota) *QuotaMetric { return a.ClaudeWeekly },
		label: func(lang string) string {
			return i18n.T3For(lang, "Claude 周额度", "Claude Weekly", "Claude 週間")
		},
	},
	{
		id:           "gemini_4_argon",
		isWeekly:     false,
		predictETA:   true,
		earlyMargin:  bonusEarlyMarginShort,
		bonusJumpMin: bonusJumpMinShort,
		maxPrevAge:   bonusMaxPrevAgeShort,
		rapidDropMin: rapidDropMinShort,
		idleDropMin:  idleDropMinShort,
		get:          func(a *AccountQuota) *QuotaMetric { return a.Gemini4Argon },
		label:        func(lang string) string { return "Gemini 4 Argon" },
	},
}

type quotaSample struct {
	at       time.Time // when Cockpit/Live captured this reading
	fraction float64
}

type bucketTracker struct {
	samples   []quotaSample
	sampleKey string // cycleKey the samples belong to
	hasPrev   bool
	prevFrac  float64
	prevReset time.Time
	prevRaw   string
	prevAt    time.Time
}

type alertState struct {
	Level     string `json:"level"`                // "warn" | "critical" | "bonus_reset" | "anomaly"
	ResetTime string `json:"reset_time,omitempty"` // RFC3339 for pruning
	FiredAt   int64  `json:"fired_at,omitempty"`   // unix seconds for cooldown & pruning
}

type quotaAlerter struct {
	cfg QuotaAlertConfig

	mu          sync.Mutex
	buckets     map[string]*bucketTracker
	lastSeenAt  int64  // last metric UpdatedAt processed
	lastAccount string // normalized account identity (email or ID)
	state       map[string]alertState
}

func newQuotaAlerter(cfg QuotaAlertConfig) *quotaAlerter {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Lang == nil {
		cfg.Lang = i18n.GetLang
	}
	if cfg.WarnFraction <= 0 || cfg.CriticalFraction <= 0 || cfg.CriticalFraction >= cfg.WarnFraction {
		cfg.WarnFraction, cfg.CriticalFraction = defaultWarnFraction, defaultCriticalFraction
	}
	a := &quotaAlerter{
		cfg:     cfg,
		buckets: make(map[string]*bucketTracker),
		state:   map[string]alertState{},
	}
	a.loadState()
	return a
}

func (a *quotaAlerter) loadState() {
	if a.cfg.StatePath == "" {
		return
	}
	data, err := os.ReadFile(a.cfg.StatePath)
	if err != nil {
		return
	}
	var st map[string]alertState
	if json.Unmarshal(data, &st) == nil && st != nil {
		a.state = st
	}
}

func (a *quotaAlerter) saveState() {
	if a.cfg.StatePath == "" {
		return
	}
	// 清理早已过期的周期与冷却记录，文件不会无限增长
	cutoff := a.cfg.Now().Add(-48 * time.Hour)
	for k, v := range a.state {
		if t, err := time.Parse(time.RFC3339, v.ResetTime); err == nil {
			if t.Before(cutoff) {
				delete(a.state, k)
			}
			continue
		}
		if v.FiredAt > 0 && time.Unix(v.FiredAt, 0).Before(cutoff) {
			delete(a.state, k)
		}
	}
	data, err := json.Marshal(a.state)
	if err != nil {
		return
	}
	_ = writeFileAtomic0600(a.cfg.StatePath, data)
}

// pickMonitoredAccount returns the account to watch: the one the running language_server uses when known,
// otherwise Cockpit's current account.
func pickMonitoredAccount(q *CockpitQuotaResponse, liveEmail string) *AccountQuota {
	if q == nil {
		return nil
	}
	if live := strings.ToLower(strings.TrimSpace(liveEmail)); live != "" {
		for i := range q.Accounts {
			if strings.ToLower(q.Accounts[i].Email) == live {
				return &q.Accounts[i]
			}
		}
		if q.CurrentAccount != nil && strings.ToLower(q.CurrentAccount.Email) == live {
			return q.CurrentAccount
		}
	}
	return q.CurrentAccount
}

func accountIdentity(acc *AccountQuota) string {
	if acc == nil {
		return ""
	}
	if email := strings.ToLower(strings.TrimSpace(acc.Email)); email != "" {
		return email
	}
	return strings.TrimSpace(acc.ID)
}

func bucketCycleKey(accID string, spec bucketSpec, resetRaw string) string {
	// Keep legacy key format for gemini_5h so existing quota_alert_state.json files stay 100% compatible.
	if spec.id == "gemini_5h" {
		return accID + "|" + resetRaw
	}
	return accID + "|" + spec.id + "|" + resetRaw
}

type lowQuotaHit struct {
	spec     bucketSpec
	metric   *QuotaMetric
	level    string
	cycleKey string
	reset    time.Time
	eta      time.Time
	hasEta   bool
}

type bonusResetHit struct {
	spec      bucketSpec
	bonusKey  string
	prevFrac  float64
	curFrac   float64
	prevReset time.Time
	prevRaw   string
}

type anomalyHit struct {
	spec       bucketSpec
	kind       string // "idle" | "rapid"
	anomalyKey string
	fromFrac   float64
	toFrac     float64
	span       time.Duration
	resetRaw   string
}

// Observe records the newest reading across all quota buckets and pushes alerts when warranted.
// liveEmail is the account the running language_server uses ("" when unknown).
func (a *quotaAlerter) Observe(q *CockpitQuotaResponse, liveEmail string) {
	acc := pickMonitoredAccount(q, liveEmail)
	if acc == nil {
		return
	}
	now := a.cfg.Now()
	capturedAt := now
	if acc.UpdatedAt > 0 {
		capturedAt = time.UnixMilli(acc.UpdatedAt)
	}
	accIdent := accountIdentity(acc)
	if accIdent == "" {
		return
	}
	lang := i18n.NormalizeLang(a.cfg.Lang())
	if lang == "" {
		lang = i18n.LangZh
	}

	var hostIdleSince time.Time
	var hostIdleOK bool
	if a.cfg.HostIdleSince != nil {
		hostIdleSince, hostIdleOK = a.cfg.HostIdleSince()
	}

	a.mu.Lock()

	// 同一账号且快照未更新时跳过，避免重复入样或重复评估
	if accIdent == a.lastAccount && acc.UpdatedAt > 0 && acc.UpdatedAt == a.lastSeenAt {
		a.mu.Unlock()
		return
	}
	// 切换账号时重置内存态采样与前值，避免跨账号误判为跳升/骤降
	if a.lastAccount != "" && accIdent != a.lastAccount {
		a.buckets = make(map[string]*bucketTracker)
	}
	a.lastAccount, a.lastSeenAt = accIdent, acc.UpdatedAt

	var lowHits []lowQuotaHit
	var bonusHits []bonusResetHit
	var idleHits []anomalyHit
	var rapidHits []anomalyHit
	stateDirty := false

	for _, spec := range monitoredBuckets {
		m := spec.get(acc)
		if m == nil {
			continue
		}
		frac := m.RemainingFraction
		if frac < 0 {
			frac = 0
		} else if frac > 1 {
			frac = 1
		}
		var reset time.Time
		if t, err := time.Parse(time.RFC3339, m.ResetTime); err == nil {
			reset = t
		}
		cycleKey := bucketCycleKey(acc.ID, spec, m.ResetTime)

		tr := a.buckets[spec.id]
		if tr == nil {
			tr = &bucketTracker{}
			a.buckets[spec.id] = tr
		}

		// 1. 官方提前回满（Bonus Reset）检测：必须在更新 prev 之前比对
		bonusTriggeredThisTick := false
		if !a.cfg.DisableBonusReset && tr.hasPrev {
			jump := frac - tr.prevFrac
			prevAge := capturedAt.Sub(tr.prevAt)
			if jump >= spec.bonusJumpMin &&
				frac >= bonusMinFinalFraction &&
				!tr.prevReset.IsZero() &&
				now.Before(tr.prevReset.Add(-spec.earlyMargin)) &&
				prevAge >= 0 && prevAge <= spec.maxPrevAge {
				bonusKey := acc.ID + "|bonus|" + spec.id + "|" + tr.prevRaw
				if a.state[bonusKey].Level == "" {
					bonusHits = append(bonusHits, bonusResetHit{
						spec:      spec,
						bonusKey:  bonusKey,
						prevFrac:  tr.prevFrac,
						curFrac:   frac,
						prevReset: tr.prevReset,
						prevRaw:   tr.prevRaw,
					})
					bonusTriggeredThisTick = true
				}
			}
		}

		// 2. 更新本周期采样窗口（若发生提前回满，清空旧采样并重新布防低水位告警以兼容 resetTime 不变的原地回满）
		if bonusTriggeredThisTick {
			tr.samples = nil
			delete(a.state, cycleKey)
		}
		if tr.sampleKey != cycleKey {
			tr.samples, tr.sampleKey = nil, cycleKey
		}
		tr.samples = append(tr.samples, quotaSample{at: capturedAt, fraction: frac})
		cutoff := now.Add(-sampleRetention)
		kept := tr.samples[:0]
		for _, s := range tr.samples {
			if s.at.After(cutoff) {
				kept = append(kept, s)
			}
		}
		tr.samples = kept

		// 3. 低水位阈值预警（每周期每档仅推一次）
		lowTriggeredThisTick := false
		level := ""
		switch {
		case frac <= a.cfg.CriticalFraction:
			level = "critical"
		case frac <= a.cfg.WarnFraction:
			level = "warn"
		}
		if level != "" {
			fired := a.state[cycleKey].Level
			if fired != "critical" && fired != level {
				var eta time.Time
				var hasEta bool
				if spec.predictETA {
					eta, hasEta = predictExhaustion(tr.samples, now, reset)
				}
				lowHits = append(lowHits, lowQuotaHit{
					spec:     spec,
					metric:   m,
					level:    level,
					cycleKey: cycleKey,
					reset:    reset,
					eta:      eta,
					hasEta:   hasEta,
				})
				lowTriggeredThisTick = true
			}
		}

		// 4. 异常骤降 / 空闲期静默掉额检测（同池本轮已触发低水位时不再重复叠加推送）
		if !a.cfg.DisableAnomaly && tr.hasPrev && !lowTriggeredThisTick {
			idleConfirmed := hostIdleOK &&
				!hostIdleSince.IsZero() &&
				now.Sub(hostIdleSince) >= idleGraceDuration &&
				!tr.prevAt.Before(hostIdleSince.Add(idleSampleMargin))

			if idleConfirmed && tr.prevFrac-frac >= spec.idleDropMin {
				anomalyKey := acc.ID + "|anomaly|idle|" + spec.id
				if !a.inCooldown(anomalyKey, now, anomalyCooldown) {
					idleHits = append(idleHits, anomalyHit{
						spec:       spec,
						kind:       "idle",
						anomalyKey: anomalyKey,
						fromFrac:   tr.prevFrac,
						toFrac:     frac,
						span:       now.Sub(hostIdleSince),
						resetRaw:   m.ResetTime,
					})
				}
			} else if peak, ok := recentCyclePeak(tr.samples, capturedAt, anomalyRapidWindow); ok {
				if peak.fraction-frac >= spec.rapidDropMin {
					anomalyKey := acc.ID + "|anomaly|rapid|" + spec.id
					if !a.inCooldown(anomalyKey, now, anomalyCooldown) {
						rapidHits = append(rapidHits, anomalyHit{
							spec:       spec,
							kind:       "rapid",
							anomalyKey: anomalyKey,
							fromFrac:   peak.fraction,
							toFrac:     frac,
							span:       capturedAt.Sub(peak.at),
							resetRaw:   m.ResetTime,
						})
					}
				}
			}
		}

		// 5. 更新前值快照
		tr.hasPrev = true
		tr.prevFrac = frac
		tr.prevReset = reset
		tr.prevRaw = m.ResetTime
		tr.prevAt = capturedAt
	}

	var toSend []QuotaAlert

	// 组装低水位推送（多池同轮触发时自动聚合为 1 条）
	if len(lowHits) > 0 {
		for _, h := range lowHits {
			a.state[h.cycleKey] = alertState{
				Level:     h.level,
				ResetTime: h.metric.ResetTime,
				FiredAt:   now.Unix(),
			}
		}
		stateDirty = true
		toSend = append(toSend, buildLowQuotaAlert(lang, acc, lowHits, now))
	}

	// 组装官方提前回满喜报（多池同轮回满强制聚合为 1 条）
	if len(bonusHits) > 0 {
		for _, h := range bonusHits {
			a.state[h.bonusKey] = alertState{
				Level:     "bonus_reset",
				ResetTime: h.prevRaw,
				FiredAt:   now.Unix(),
			}
		}
		stateDirty = true
		toSend = append(toSend, buildBonusResetAlert(lang, acc, bonusHits, now))
	}

	// 组装空闲期掉额告警（同轮多池聚合为 1 条）
	if len(idleHits) > 0 {
		for _, h := range idleHits {
			a.state[h.anomalyKey] = alertState{
				Level:     "anomaly",
				ResetTime: h.resetRaw,
				FiredAt:   now.Unix(),
			}
		}
		stateDirty = true
		toSend = append(toSend, buildAnomalyAlert(lang, acc, "idle", idleHits, now))
	}

	// 组装短时异常骤降告警（同轮多池聚合为 1 条）
	if len(rapidHits) > 0 {
		for _, h := range rapidHits {
			a.state[h.anomalyKey] = alertState{
				Level:     "anomaly",
				ResetTime: h.resetRaw,
				FiredAt:   now.Unix(),
			}
		}
		stateDirty = true
		toSend = append(toSend, buildAnomalyAlert(lang, acc, "rapid", rapidHits, now))
	}

	if stateDirty {
		a.saveState()
	}
	notifyFn := a.cfg.Notify
	a.mu.Unlock()

	if notifyFn != nil {
		for _, alert := range toSend {
			notifyFn(alert)
		}
	}
}

func (a *quotaAlerter) inCooldown(key string, now time.Time, cooldown time.Duration) bool {
	st, ok := a.state[key]
	if !ok || st.FiredAt <= 0 {
		return false
	}
	return now.Sub(time.Unix(st.FiredAt, 0)) < cooldown
}

func recentCyclePeak(samples []quotaSample, capturedAt time.Time, window time.Duration) (quotaSample, bool) {
	if len(samples) < 2 {
		return quotaSample{}, false
	}
	cutoff := capturedAt.Add(-window)
	var peak quotaSample
	found := false
	// Exclude the last sample (which is the current reading itself).
	for _, s := range samples[:len(samples)-1] {
		if s.at.Before(cutoff) || !s.at.Before(capturedAt) {
			continue
		}
		if !found || s.fraction > peak.fraction {
			peak = s
			found = true
		}
	}
	return peak, found
}

func pctInt(fraction float64) int {
	p := int(math.Floor(fraction*100 + 0.5))
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// alertTitle preserves backward compatibility for existing unit tests targeting Gemini 5h.
func alertTitle(level string, fraction float64) string {
	return lowQuotaSingleTitle(i18n.GetLang(), monitoredBuckets[0], level, fraction)
}

func quotaSubject(lang string, spec bucketSpec) string {
	name := spec.label(lang)
	switch i18n.NormalizeLang(lang) {
	case i18n.LangZh:
		if strings.HasSuffix(name, "额度") {
			return name
		}
		return name + " 额度"
	case i18n.LangJa:
		if strings.HasSuffix(name, "週間") {
			return name + "クォータ"
		}
		return name + " クォータ"
	default:
		return name + " quota"
	}
}

func lowQuotaSingleTitle(lang string, spec bucketSpec, level string, fraction float64) string {
	subj := quotaSubject(lang, spec)
	pct := pctInt(fraction)
	switch {
	case fraction <= 0:
		return fmt.Sprintf(
			i18n.T3For(lang, "🚨 %s已用完", "🚨 %s exhausted", "🚨 %sを使い切りました"),
			subj,
		)
	case level == "critical":
		return fmt.Sprintf(
			i18n.T3For(lang, "🚨 %s仅剩 %d%%", "🚨 %s down to %d%%", "🚨 %s残り %d%%"),
			subj, pct,
		)
	default:
		return fmt.Sprintf(
			i18n.T3For(lang, "⚠️ %s仅剩 %d%%", "⚠️ %s down to %d%%", "⚠️ %s残り %d%%"),
			subj, pct,
		)
	}
}

func buildLowQuotaAlert(lang string, acc *AccountQuota, hits []lowQuotaHit, now time.Time) QuotaAlert {
	if len(hits) == 1 {
		h := hits[0]
		return QuotaAlert{
			Level:    h.level,
			Title:    lowQuotaSingleTitle(lang, h.spec, h.level, h.metric.RemainingFraction),
			Body:     lowQuotaSingleBody(lang, acc, h.reset, h.eta, h.hasEta, now),
			StateKey: h.cycleKey,
		}
	}

	level := "warn"
	keys := make([]string, 0, len(hits))
	items := make([]string, 0, len(hits))
	for _, h := range hits {
		if h.level == "critical" {
			level = "critical"
		}
		keys = append(keys, h.cycleKey)
		pct := pctInt(h.metric.RemainingFraction)
		items = append(items, fmt.Sprintf(
			i18n.T3For(lang, "%s %d%%", "%s %d%%", "%s 残り%d%%"),
			h.spec.label(lang), pct,
		))
	}
	var title string
	if level == "critical" {
		title = fmt.Sprintf(
			i18n.T3For(lang, "🚨 多项额度告急（%d 项）", "🚨 Multiple quotas critical (%d)", "🚨 複数のクォータが逼迫（%d件）"),
			len(hits),
		)
	} else {
		title = fmt.Sprintf(
			i18n.T3For(lang, "⚠️ 多项额度偏低（%d 项）", "⚠️ Multiple quotas running low (%d)", "⚠️ 複数のクォータが低下（%d件）"),
			len(hits),
		)
	}
	sep := i18n.T3For(lang, "、", ", ", "・")
	parts := accountPrefixParts(lang, acc)
	parts = append(parts, strings.Join(items, sep))
	return QuotaAlert{
		Level:    level,
		Title:    title,
		Body:     strings.Join(parts, " · "),
		StateKey: strings.Join(keys, "+"),
	}
}

func buildBonusResetAlert(lang string, acc *AccountQuota, hits []bonusResetHit, now time.Time) QuotaAlert {
	parts := accountPrefixParts(lang, acc)
	if len(hits) == 1 {
		h := hits[0]
		subj := quotaSubject(lang, h.spec)
		title := fmt.Sprintf(
			i18n.T3For(lang, "🎁 %s已提前回满", "🎁 %s refilled early", "🎁 %sが予定より早く回復"),
			subj,
		)
		detail := fmt.Sprintf("%d%% → %d%%", pctInt(h.prevFrac), pctInt(h.curFrac))
		if !h.prevReset.IsZero() {
			resetStr := formatResetClock(h.prevReset, now)
			detail += fmt.Sprintf(
				i18n.T3For(lang, "（原定 %s 重置）", " (was due %s)", "（本来のリセット %s）"),
				resetStr,
			)
		}
		parts = append(parts, detail)
		return QuotaAlert{
			Level:    "bonus_reset",
			Title:    title,
			Body:     strings.Join(parts, " · "),
			StateKey: h.bonusKey,
		}
	}

	keys := make([]string, 0, len(hits))
	items := make([]string, 0, len(hits))
	for _, h := range hits {
		keys = append(keys, h.bonusKey)
		items = append(items, fmt.Sprintf("%s (%d%%→%d%%)", h.spec.label(lang), pctInt(h.prevFrac), pctInt(h.curFrac)))
	}
	sep := i18n.T3For(lang, "、", ", ", "・")
	summary := fmt.Sprintf(
		i18n.T3For(lang, "%s 已早于原定时间重置", "%s reset ahead of schedule", "%s が予定より早くリセットされました"),
		strings.Join(items, sep),
	)
	parts = append(parts, summary)
	return QuotaAlert{
		Level:    "bonus_reset",
		Title:    i18n.T3For(lang, "🎁 Antigravity 额度已提前回满", "🎁 Antigravity quotas refilled early", "🎁 Antigravity クォータが予定より早く回復"),
		Body:     strings.Join(parts, " · "),
		StateKey: strings.Join(keys, "+"),
	}
}

func buildAnomalyAlert(lang string, acc *AccountQuota, kind string, hits []anomalyHit, now time.Time) QuotaAlert {
	parts := accountPrefixParts(lang, acc)
	cooldownBucket := now.Unix() / int64(anomalyCooldown.Seconds())
	keys := make([]string, 0, len(hits))
	for _, h := range hits {
		keys = append(keys, fmt.Sprintf("%s|%d", h.anomalyKey, cooldownBucket))
	}

	if kind == "idle" {
		idleDur := friendlyDuration(hits[0].span)
		if len(hits) == 1 {
			h := hits[0]
			subj := quotaSubject(lang, h.spec)
			title := fmt.Sprintf(
				i18n.T3For(lang, "🔍 空闲期检测到 %s下降", "🔍 %s dropped while idle", "🔍 アイドル中に %sが減少"),
				subj,
			)
			detail := fmt.Sprintf(
				i18n.T3For(
					lang,
					"%d%% → %d%%（本机已空闲 %s，若无其他设备使用请检查后台任务）",
					"%d%% → %d%% (host idle %s; check background tasks if no other device is active)",
					"%d%% → %d%%（%s アイドル中・他端末で利用中でなければバックグラウンドを確認）",
				),
				pctInt(h.fromFrac), pctInt(h.toFrac), idleDur,
			)
			parts = append(parts, detail)
			return QuotaAlert{
				Level:    "anomaly",
				Title:    title,
				Body:     strings.Join(parts, " · "),
				StateKey: keys[0],
			}
		}

		items := make([]string, 0, len(hits))
		for _, h := range hits {
			items = append(items, fmt.Sprintf("%s (%d%%→%d%%)", h.spec.label(lang), pctInt(h.fromFrac), pctInt(h.toFrac)))
		}
		sep := i18n.T3For(lang, "、", ", ", "・")
		parts = append(parts, fmt.Sprintf(
			i18n.T3For(
				lang,
				"%s（本机已空闲 %s，若无其他设备使用请检查后台任务）",
				"%s (host idle %s; check background tasks if no other device is active)",
				"%s（%s アイドル中・他端末で利用中でなければバックグラウンドを確認）",
			),
			strings.Join(items, sep), idleDur,
		))
		return QuotaAlert{
			Level:    "anomaly",
			Title:    i18n.T3For(lang, "🔍 空闲期检测到多项额度下降", "🔍 Multiple quotas dropped while idle", "🔍 アイドル中に複数のクォータが減少"),
			Body:     strings.Join(parts, " · "),
			StateKey: strings.Join(keys, "+"),
		}
	}

	// rapid drop
	if len(hits) == 1 {
		h := hits[0]
		subj := quotaSubject(lang, h.spec)
		dropPct := pctInt(h.fromFrac - h.toFrac)
		spanStr := friendlyDuration(h.span)
		if h.span < time.Minute {
			spanStr = "1m"
		}
		title := fmt.Sprintf(
			i18n.T3For(lang, "⚡ %s短时骤降 %d%%", "⚡ %s dropped %d%% rapidly", "⚡ %sが短時間で %d%% 急減"),
			subj, dropPct,
		)
		detail := fmt.Sprintf(
			i18n.T3For(
				lang,
				"%d%% → %d%%（约 %s 内），请留意当前模型档位或缓存命中",
				"%d%% → %d%% (~%s); check active model tier or prompt cache",
				"%d%% → %d%%（約 %s）・モデル設定やキャッシュを確認してください",
			),
			pctInt(h.fromFrac), pctInt(h.toFrac), spanStr,
		)
		parts = append(parts, detail)
		return QuotaAlert{
			Level:    "anomaly",
			Title:    title,
			Body:     strings.Join(parts, " · "),
			StateKey: keys[0],
		}
	}

	items := make([]string, 0, len(hits))
	for _, h := range hits {
		items = append(items, fmt.Sprintf("%s (%d%%→%d%%)", h.spec.label(lang), pctInt(h.fromFrac), pctInt(h.toFrac)))
	}
	sep := i18n.T3For(lang, "、", ", ", "・")
	parts = append(parts, fmt.Sprintf(
		i18n.T3For(
			lang,
			"%s，请留意当前模型档位或缓存命中",
			"%s; check active model tier or prompt cache",
			"%s・モデル設定やキャッシュを確認してください",
		),
		strings.Join(items, sep),
	))
	return QuotaAlert{
		Level:    "anomaly",
		Title:    i18n.T3For(lang, "⚡ 多项额度短时骤降", "⚡ Multiple quotas dropped rapidly", "⚡ 複数のクォータが短時間で急減"),
		Body:     strings.Join(parts, " · "),
		StateKey: strings.Join(keys, "+"),
	}
}

func accountPrefixParts(lang string, acc *AccountQuota) []string {
	if acc == nil || acc.Email == "" {
		return nil
	}
	masked := maskEmailForAlert(acc.Email)
	return []string{fmt.Sprintf(i18n.T3For(lang, "账号 %s", "Account %s", "アカウント %s"), masked)}
}

func formatResetClock(reset, now time.Time) string {
	localReset := reset.In(now.Location())
	if reset.Sub(now) >= 24*time.Hour {
		return localReset.Format("01-02 15:04")
	}
	return localReset.Format("15:04")
}

func maskEmailForAlert(email string) string {
	email = strings.TrimSpace(email)
	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return email
	}
	local := email[:at]
	domain := email[at+1:]
	runes := []rune(local)
	if len(runes) > 2 {
		return string(runes[:2]) + "***@" + domain
	}
	return string(runes[:1]) + "***@" + domain
}

func lowQuotaSingleBody(lang string, acc *AccountQuota, reset time.Time, eta time.Time, hasEta bool, now time.Time) string {
	parts := accountPrefixParts(lang, acc)
	if !reset.IsZero() {
		// 倒计时按当前时间现算：快照里的 reset_friendly 可能滞后一个刷新周期
		clock := formatResetClock(reset, now)
		if d := reset.Sub(now); d > time.Minute {
			dur := friendlyDuration(d)
			parts = append(parts, fmt.Sprintf(
				i18n.T3For(lang, "%s后重置（%s）", "resets in %s (%s)", "%s後にリセット（%s）"),
				dur, clock,
			))
		} else {
			parts = append(parts, fmt.Sprintf(
				i18n.T3For(lang, "即将重置（%s）", "resets soon (%s)", "まもなくリセット（%s）"),
				clock,
			))
		}
	}
	body := strings.Join(parts, " · ")
	if hasEta {
		etaClock := eta.In(now.Location()).Format("15:04")
		line := fmt.Sprintf(
			i18n.T3For(
				lang,
				"按最近 60 分钟的速度，预计 %s 用完（早于重置）",
				"At the last 60m pace, runs out around %s (before reset)",
				"直近60分のペースでは %s 頃に枯渇見込み（リセット前）",
			),
			etaClock,
		)
		if body != "" {
			body += "\n"
		}
		body += line
	}
	return body
}

// friendlyDuration renders 35m / 1h 41m / 3d 5h.
func friendlyDuration(d time.Duration) string {
	mins := int(d.Round(time.Minute) / time.Minute)
	if mins >= 24*60 {
		days := mins / (24 * 60)
		hours := (mins % (24 * 60)) / 60
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if mins >= 60 {
		return fmt.Sprintf("%dh %dm", mins/60, mins%60)
	}
	return fmt.Sprintf("%dm", mins)
}

// predictExhaustion extrapolates the consumption speed of the last predictWindow with a least-squares fit.
// It returns false when there is too little data, nothing is being consumed, or the quota would outlast the
// reset time (in which case warning about "running out" would be wrong).
func predictExhaustion(samples []quotaSample, now time.Time, reset time.Time) (time.Time, bool) {
	cutoff := now.Add(-predictWindow)
	var win []quotaSample
	for _, s := range samples {
		if !s.at.Before(cutoff) {
			win = append(win, s)
		}
	}
	if len(win) < predictMinSamples {
		return time.Time{}, false
	}
	first, last := win[0], win[len(win)-1]
	if last.at.Sub(first.at) < predictMinSpan || first.fraction-last.fraction < predictMinDrop {
		return time.Time{}, false
	}

	// least squares: fraction = a + b·t (t in seconds since first)
	var sumT, sumF, sumTT, sumTF float64
	n := float64(len(win))
	for _, s := range win {
		t := s.at.Sub(first.at).Seconds()
		sumT += t
		sumF += s.fraction
		sumTT += t * t
		sumTF += t * s.fraction
	}
	den := n*sumTT - sumT*sumT
	if den == 0 {
		return time.Time{}, false
	}
	slope := (n*sumTF - sumT*sumF) / den // 每秒变化量，消耗时为负
	if slope >= -1e-9 {
		return time.Time{}, false
	}
	secondsLeft := last.fraction / -slope
	eta := last.at.Add(time.Duration(secondsLeft * float64(time.Second)))
	if !eta.After(now) {
		// 读数略有滞后，估算已经「过去」：说明正在见底，按当前时刻处理
		eta = now
	}
	if !reset.IsZero() && !eta.Before(reset) {
		return time.Time{}, false
	}
	return eta, true
}

// StartQuotaAlerter polls quota snapshots (via cfg.FetchQuota when provided, otherwise Cockpit snapshots)
// and pushes low-quota, early bonus-reset, and abnormal-drop alerts.
func StartQuotaAlerter(ctx context.Context, cfg QuotaAlertConfig) {
	if cfg.Notify == nil {
		return
	}
	pollEvery := cfg.PollInterval
	if pollEvery <= 0 {
		pollEvery = alertPollEvery
	}
	a := newQuotaAlerter(cfg)
	go func() {
		observe := func() {
			if a.cfg.FetchQuota != nil {
				if q, email, err := a.cfg.FetchQuota(); err == nil && q != nil {
					a.Observe(q, email)
				}
				return
			}
			email := ""
			if a.cfg.LiveEmail != nil {
				email = a.cfg.LiveEmail()
			}
			if q, err := GetQuotas(email); err == nil {
				a.Observe(q, email)
			}
		}
		observe()
		t := time.NewTicker(pollEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				observe()
			}
		}
	}()
	slog.Info(fmt.Sprintf(
		"[QuotaAlert] 全池额度守护已启用 (5 池)：低水位 ≤%.0f%%/≤%.0f%%，提前回满=%v，异常骤降=%v",
		a.cfg.WarnFraction*100, a.cfg.CriticalFraction*100, !a.cfg.DisableBonusReset, !a.cfg.DisableAnomaly,
	))
}
