package cockpit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 低额度预警（Gemini 5h，当前账号）。
//
// 口径（与产品侧确认）：
//   - 只监控当前账号的 Gemini 5h 窗口；
//   - 两档固定阈值：≤20% 提醒、≤5% 紧急，可用环境变量覆盖；
//   - 每个「账号 × 重置周期」每一档只推送一次；已推状态落盘，重启网关不会重复推；
//   - 推送正文附「按最近 60 分钟消耗速度预计几点用完」，仅当预计早于重置时间才写，数据不足不写。

const (
	defaultWarnFraction     = 0.20
	defaultCriticalFraction = 0.05

	// 预测窗口与可信度门槛：样本太少、跨度太短或几乎没消耗时宁可不预测。
	predictWindow     = 60 * time.Minute
	predictMinSamples = 3
	predictMinSpan    = 15 * time.Minute
	predictMinDrop    = 0.005 // 窗口内至少掉了 0.5 个百分点

	// 采样保留时长（只需覆盖预测窗口，多留一点余量）。
	sampleRetention = 3 * time.Hour
	alertPollEvery  = 30 * time.Second
)

// QuotaAlert is one notification to deliver.
type QuotaAlert struct {
	Level    string // "warn" | "critical"
	Title    string
	Body     string
	StateKey string // 账号 + 重置周期，通知端可据此再去重
}

// QuotaAlertConfig configures the alerter.
type QuotaAlertConfig struct {
	WarnFraction     float64
	CriticalFraction float64
	// StatePath persists which levels were already pushed per reset cycle ("" = memory only).
	StatePath string
	// LiveEmail returns the account the running language_server is really using ("" = trust Cockpit's current account).
	LiveEmail func() string
	Notify    func(QuotaAlert)
	Now       func() time.Time
}

// QuotaAlertConfigFromEnv reads QUOTA_ALERT_WARN_PERCENT / QUOTA_ALERT_CRITICAL_PERCENT.
// Invalid or inconsistent values fall back to the defaults (20% / 5%).
func QuotaAlertConfigFromEnv() QuotaAlertConfig {
	cfg := QuotaAlertConfig{WarnFraction: defaultWarnFraction, CriticalFraction: defaultCriticalFraction}
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

type quotaSample struct {
	at       time.Time // when Cockpit captured this reading
	fraction float64
}

type alertState struct {
	Level     string `json:"level"`      // highest level already pushed in this cycle
	ResetTime string `json:"reset_time"` // for pruning
}

type quotaAlerter struct {
	cfg QuotaAlertConfig

	mu          sync.Mutex
	samples     []quotaSample
	sampleKey   string // account|resetTime the samples belong to
	lastSeenAt  int64  // last metric UpdatedAt processed
	lastAccount string
	state       map[string]alertState
}

func newQuotaAlerter(cfg QuotaAlertConfig) *quotaAlerter {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.WarnFraction <= 0 || cfg.CriticalFraction <= 0 || cfg.CriticalFraction >= cfg.WarnFraction {
		cfg.WarnFraction, cfg.CriticalFraction = defaultWarnFraction, defaultCriticalFraction
	}
	a := &quotaAlerter{cfg: cfg, state: map[string]alertState{}}
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
	// 清理早已过期的周期，文件不会无限增长
	cutoff := a.cfg.Now().Add(-48 * time.Hour)
	for k, v := range a.state {
		if t, err := time.Parse(time.RFC3339, v.ResetTime); err == nil && t.Before(cutoff) {
			delete(a.state, k)
		}
	}
	data, err := json.Marshal(a.state)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(a.cfg.StatePath), 0o700); err != nil {
		return
	}
	tmp := a.cfg.StatePath + ".tmp"
	if os.WriteFile(tmp, data, 0o600) == nil {
		_ = os.Rename(tmp, a.cfg.StatePath)
	}
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
	}
	return q.CurrentAccount
}

// Observe records the newest reading and pushes an alert when a threshold is crossed.
// liveEmail is the account the running language_server uses ("" when unknown).
func (a *quotaAlerter) Observe(q *CockpitQuotaResponse, liveEmail string) {
	acc := pickMonitoredAccount(q, liveEmail)
	if acc == nil || acc.Gemini5h == nil {
		return
	}
	m := acc.Gemini5h
	now := a.cfg.Now()

	capturedAt := now
	if acc.UpdatedAt > 0 {
		capturedAt = time.UnixMilli(acc.UpdatedAt)
	}

	var reset time.Time
	if t, err := time.Parse(time.RFC3339, m.ResetTime); err == nil {
		reset = t
	}
	cycleKey := acc.ID + "|" + m.ResetTime

	a.mu.Lock()
	defer a.mu.Unlock()

	// Cockpit 每约 10 分钟才刷新一次；同一读数不重复入样、不重复评估
	if acc.ID == a.lastAccount && acc.UpdatedAt > 0 && acc.UpdatedAt == a.lastSeenAt {
		return
	}
	a.lastAccount, a.lastSeenAt = acc.ID, acc.UpdatedAt

	// 换了账号或进入新的重置周期：旧样本不能参与新周期的消耗速度
	if a.sampleKey != cycleKey {
		a.samples, a.sampleKey = nil, cycleKey
	}
	a.samples = append(a.samples, quotaSample{at: capturedAt, fraction: m.RemainingFraction})
	cutoff := now.Add(-sampleRetention)
	kept := a.samples[:0]
	for _, s := range a.samples {
		if s.at.After(cutoff) {
			kept = append(kept, s)
		}
	}
	a.samples = kept

	level := ""
	switch {
	case m.RemainingFraction <= a.cfg.CriticalFraction:
		level = "critical"
	case m.RemainingFraction <= a.cfg.WarnFraction:
		level = "warn"
	}
	if level == "" {
		return
	}
	if fired := a.state[cycleKey].Level; fired == "critical" || fired == level {
		return // 这一档（或更严重的一档）本周期已经推过
	}

	eta, ok := predictExhaustion(a.samples, now, reset)
	alert := QuotaAlert{
		Level:    level,
		Title:    alertTitle(level, m.RemainingFraction),
		Body:     alertBody(acc, m, reset, eta, ok, now),
		StateKey: cycleKey,
	}
	a.state[cycleKey] = alertState{Level: level, ResetTime: m.ResetTime}
	a.saveState()
	if a.cfg.Notify != nil {
		a.cfg.Notify(alert)
	}
}

func alertTitle(level string, fraction float64) string {
	pct := int(math.Floor(fraction*100 + 0.5))
	switch {
	case fraction <= 0:
		return "🚨 Gemini 5h 额度已用完"
	case level == "critical":
		return fmt.Sprintf("🚨 Gemini 5h 额度仅剩 %d%%", pct)
	default:
		return fmt.Sprintf("⚠️ Gemini 5h 额度仅剩 %d%%", pct)
	}
}

func alertBody(acc *AccountQuota, m *QuotaMetric, reset time.Time, eta time.Time, hasEta bool, now time.Time) string {
	var parts []string
	if acc.Email != "" {
		parts = append(parts, "账号 "+maskEmail(acc.Email))
	}
	if !reset.IsZero() {
		// 倒计时按当前时间现算：Cockpit 快照里的 reset_friendly 最多滞后一个刷新周期
		left := "即将"
		if d := reset.Sub(now); d > time.Minute {
			left = friendlyDuration(d) + "后"
		}
		parts = append(parts, fmt.Sprintf("%s重置（%s）", left, reset.In(now.Location()).Format("15:04")))
	}
	body := strings.Join(parts, " · ")
	if hasEta {
		line := fmt.Sprintf("按最近 60 分钟的速度，预计 %s 用完（早于重置）", eta.In(now.Location()).Format("15:04"))
		if body != "" {
			body += "\n"
		}
		body += line
	}
	return body
}

// friendlyDuration renders 1h 41m / 35m.
func friendlyDuration(d time.Duration) string {
	mins := int(d.Round(time.Minute) / time.Minute)
	if mins >= 60 {
		return fmt.Sprintf("%dh %dm", mins/60, mins%60)
	}
	return fmt.Sprintf("%dm", mins)
}

// maskEmail keeps the push (which passes through third-party push services) free of full addresses.
func maskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return email
	}
	local, domain := email[:at], email[at:]
	r := []rune(local)
	switch len(r) {
	case 1:
		return string(r) + "***" + domain
	case 2:
		return string(r[:1]) + "***" + domain
	default:
		return string(r[:1]) + "***" + string(r[len(r)-1:]) + domain
	}
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

// StartQuotaAlerter polls the Cockpit quota snapshot and pushes low-quota alerts. It never triggers a refresh
// itself; it only reacts to readings the existing auto-refresher produces.
func StartQuotaAlerter(ctx context.Context, cfg QuotaAlertConfig) {
	if cfg.Notify == nil {
		return
	}
	a := newQuotaAlerter(cfg)
	go func() {
		observe := func() {
			email := ""
			if a.cfg.LiveEmail != nil {
				email = a.cfg.LiveEmail()
			}
			if q, err := GetQuotas(email); err == nil {
				a.Observe(q, email)
			}
		}
		observe()
		t := time.NewTicker(alertPollEvery)
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
	slog.Info(fmt.Sprintf("[QuotaAlert] Gemini 5h 低额度预警已启用：≤%.0f%% 提醒，≤%.0f%% 紧急", a.cfg.WarnFraction*100, a.cfg.CriticalFraction*100))
}
