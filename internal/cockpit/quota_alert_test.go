package cockpit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"antigravity-mobile/internal/i18n"
)

var t0 = time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)

func samples(start time.Time, step time.Duration, fractions ...float64) []quotaSample {
	out := make([]quotaSample, len(fractions))
	for i, f := range fractions {
		out[i] = quotaSample{at: start.Add(time.Duration(i) * step), fraction: f}
	}
	return out
}

func TestPredictExhaustion(t *testing.T) {
	reset := t0.Add(5 * time.Hour)

	// 每 10 分钟掉 5 个点（0.5%/分钟）：剩 40% → 还能撑 80 分钟
	s := samples(t0, 10*time.Minute, 0.60, 0.55, 0.50, 0.45)
	now := t0.Add(30 * time.Minute)
	eta, ok := predictExhaustion(s, now, reset)
	if !ok {
		t.Fatal("steady consumption must produce a prediction")
	}
	want := t0.Add(30 * time.Minute).Add(90 * time.Minute) // 最后样本 +30min 时 0.45，再撑 90 分钟
	if d := eta.Sub(want); d < -time.Minute || d > time.Minute {
		t.Errorf("eta = %v, want ≈ %v", eta, want)
	}

	cases := map[string][]quotaSample{
		"too few samples": samples(t0, 10*time.Minute, 0.60, 0.50),
		"span too short":  samples(t0, 3*time.Minute, 0.60, 0.55, 0.50),
		"no consumption":  samples(t0, 10*time.Minute, 0.50, 0.50, 0.50, 0.50),
		"negligible drop": samples(t0, 10*time.Minute, 0.500, 0.499, 0.498),
		"quota rising":    samples(t0, 10*time.Minute, 0.30, 0.40, 0.50, 0.60),
		"empty":           nil,
	}
	for name, sm := range cases {
		if _, ok := predictExhaustion(sm, t0.Add(30*time.Minute), reset); ok {
			t.Errorf("%s: must not predict", name)
		}
	}
}

func TestPredictExhaustionOnlyWhenBeforeReset(t *testing.T) {
	s := samples(t0, 10*time.Minute, 0.60, 0.58, 0.56, 0.54) // 0.2%/分钟，剩 54% 要 270 分钟
	now := t0.Add(30 * time.Minute)
	if _, ok := predictExhaustion(s, now, now.Add(60*time.Minute)); ok {
		t.Error("quota outlasts the reset: warning about running out would be wrong")
	}
	if _, ok := predictExhaustion(s, now, now.Add(6*time.Hour)); !ok {
		t.Error("runs out before a distant reset: should predict")
	}
	if _, ok := predictExhaustion(s, now, time.Time{}); !ok {
		t.Error("unknown reset time should not block the prediction")
	}
}

func TestPredictExhaustionIgnoresSamplesOlderThanWindow(t *testing.T) {
	// 90 分钟前有一次剧烈消耗，最近 60 分钟几乎没动：不应被旧数据带偏
	old := samples(t0, 10*time.Minute, 0.90, 0.60, 0.30)
	recent := samples(t0.Add(100*time.Minute), 10*time.Minute, 0.30, 0.30, 0.30, 0.30)
	now := t0.Add(140 * time.Minute)
	if _, ok := predictExhaustion(append(old, recent...), now, time.Time{}); ok {
		t.Error("stale samples must not drive the prediction")
	}
}

func TestFriendlyDuration(t *testing.T) {
	cases := map[time.Duration]string{
		35 * time.Minute:                "35m",
		60 * time.Minute:                "1h 0m",
		101 * time.Minute:               "1h 41m",
		4*time.Hour + 29*time.Second:    "4h 0m",
		89*time.Minute + 40*time.Second: "1h 30m",
	}
	for d, want := range cases {
		if got := friendlyDuration(d); got != want {
			t.Errorf("friendlyDuration(%v) = %q, want %q", d, got, want)
		}
	}
}

// ---- alerter ------------------------------------------------------------------

func quotaFor(id, email string, frac float64, updated time.Time, reset time.Time) *CockpitQuotaResponse {
	acc := AccountQuota{
		ID: id, Email: email, IsCurrent: true, UpdatedAt: updated.UnixMilli(),
		Gemini5h: &QuotaMetric{RemainingFraction: frac, RemainingPercent: frac * 100, ResetTime: reset.Format(time.RFC3339), ResetFriendly: "1h 47m"},
	}
	return &CockpitQuotaResponse{CurrentAccount: &acc, Accounts: []AccountQuota{acc}}
}

type harness struct {
	a     *quotaAlerter
	sent  []QuotaAlert
	now   time.Time
	reset time.Time
}

func newHarness(t *testing.T, statePath string) *harness {
	h := &harness{now: t0, reset: t0.Add(4 * time.Hour)}
	h.a = newQuotaAlerter(QuotaAlertConfig{
		WarnFraction: 0.20, CriticalFraction: 0.05, StatePath: statePath,
		Now:    func() time.Time { return h.now },
		Notify: func(a QuotaAlert) { h.sent = append(h.sent, a) },
	})
	return h
}

// feed delivers one Cockpit refresh: time advances by `step`, and UpdatedAt moves with it.
func (h *harness) feed(frac float64, step time.Duration) {
	h.now = h.now.Add(step)
	h.a.Observe(quotaFor("acc-1", "taojiuzhen@gmail.com", frac, h.now, h.reset), "")
}

func TestAlertFiresOncePerLevelPerCycle(t *testing.T) {
	h := newHarness(t, "")
	h.feed(0.90, 10*time.Minute)
	h.feed(0.50, 10*time.Minute)
	if len(h.sent) != 0 {
		t.Fatalf("no alert above thresholds, got %d", len(h.sent))
	}
	h.feed(0.20, 10*time.Minute) // 恰好等于阈值也要提醒
	if len(h.sent) != 1 || h.sent[0].Level != "warn" {
		t.Fatalf("expected one warn at exactly 20%%, got %+v", h.sent)
	}
	h.feed(0.15, 10*time.Minute)
	h.feed(0.10, 10*time.Minute)
	if len(h.sent) != 1 {
		t.Errorf("warn must not repeat within the cycle, got %d", len(h.sent))
	}
	h.feed(0.05, 10*time.Minute)
	if len(h.sent) != 2 || h.sent[1].Level != "critical" {
		t.Fatalf("expected critical at 5%%, got %+v", h.sent)
	}
	h.feed(0.02, 10*time.Minute)
	h.feed(0.00, 10*time.Minute)
	if len(h.sent) != 2 {
		t.Errorf("critical must not repeat within the cycle, got %d", len(h.sent))
	}
}

func TestAlertSkipsWarnWhenQuotaJumpsStraightToCritical(t *testing.T) {
	h := newHarness(t, "")
	h.feed(0.60, 10*time.Minute)
	h.feed(0.03, 10*time.Minute) // 一个采样间隔内从 60% 掉到 3%
	if len(h.sent) != 1 || h.sent[0].Level != "critical" {
		t.Fatalf("expected a single critical alert, got %+v", h.sent)
	}
	h.feed(0.18, 10*time.Minute) // 不应再补发已被覆盖的「提醒」档
	if len(h.sent) != 1 {
		t.Errorf("lower level after critical must not fire, got %d", len(h.sent))
	}
}

func TestAlertRearmsAfterReset(t *testing.T) {
	h := newHarness(t, "")
	h.feed(0.15, 10*time.Minute)
	if len(h.sent) != 1 {
		t.Fatalf("first cycle warn missing: %+v", h.sent)
	}
	// 正常到期重置（推进到原定 reset 之后）：不应误报提前回满，新周期用到 18% 应再次提醒
	h.reset = h.reset.Add(5 * time.Hour)
	h.feed(1.00, 4*time.Hour)
	if len(h.sent) != 1 {
		t.Fatalf("on-time reset must not trigger bonus_reset, got %+v", h.sent)
	}
	h.feed(0.18, 10*time.Minute)
	if len(h.sent) != 2 {
		t.Errorf("a new reset cycle must be able to alert again, got %d", len(h.sent))
	}
}

func TestAlertDoesNotRepeatAfterRestartWithinSameCycle(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state", "quota_alert_state.json")
	h1 := newHarness(t, state)
	h1.feed(0.15, 10*time.Minute)
	if len(h1.sent) != 1 {
		t.Fatal("setup: warn expected")
	}

	// 网关重启：新的 alerter 从磁盘读回已推送状态
	h2 := newHarness(t, state)
	h2.now, h2.reset = h1.now, h1.reset
	h2.feed(0.14, 10*time.Minute)
	if len(h2.sent) != 0 {
		t.Errorf("restart must not re-send the same level, got %+v", h2.sent)
	}
	h2.feed(0.04, 10*time.Minute)
	if len(h2.sent) != 1 || h2.sent[0].Level != "critical" {
		t.Errorf("a higher level after restart must still alert, got %+v", h2.sent)
	}
}

func TestAlertIgnoresUnchangedReadingAndOtherAccounts(t *testing.T) {
	h := newHarness(t, "")
	q := quotaFor("acc-1", "a@x.com", 0.10, t0, h.reset)
	h.a.Observe(q, "")
	h.a.Observe(q, "") // Cockpit 还没刷新：同一读数
	if len(h.sent) != 1 {
		t.Fatalf("same reading must be processed once, got %d", len(h.sent))
	}

	// 备用账号额度很低，但当前账号正常：不打扰
	other := &CockpitQuotaResponse{
		CurrentAccount: &AccountQuota{ID: "acc-1", Email: "a@x.com", UpdatedAt: t0.Add(time.Hour).UnixMilli(),
			Gemini5h: &QuotaMetric{RemainingFraction: 0.9, ResetTime: h.reset.Format(time.RFC3339)}},
		Accounts: []AccountQuota{
			{ID: "acc-2", Email: "b@x.com", Gemini5h: &QuotaMetric{RemainingFraction: 0.01, ResetTime: h.reset.Format(time.RFC3339)}},
		},
	}
	h2 := newHarness(t, "")
	h2.a.Observe(other, "")
	if len(h2.sent) != 0 {
		t.Errorf("only the current account is monitored, got %+v", h2.sent)
	}
}

func TestAlertFollowsLiveAccount(t *testing.T) {
	h := newHarness(t, "")
	a1 := AccountQuota{ID: "acc-1", Email: "a@x.com", UpdatedAt: t0.UnixMilli(), Gemini5h: &QuotaMetric{RemainingFraction: 0.9, ResetTime: h.reset.Format(time.RFC3339)}}
	a2 := AccountQuota{ID: "acc-2", Email: "b@x.com", UpdatedAt: t0.UnixMilli(), Gemini5h: &QuotaMetric{RemainingFraction: 0.1, ResetTime: h.reset.Format(time.RFC3339)}}
	q := &CockpitQuotaResponse{CurrentAccount: &a1, Accounts: []AccountQuota{a1, a2}}
	// Cockpit 记录的当前账号是 a，但 language_server 实际在用 b：以实际在用的为准
	h.a.Observe(q, "B@x.com")
	if len(h.sent) != 1 || !strings.Contains(h.sent[0].Body, "Account b***@x.com") || strings.Contains(h.sent[0].Body, "a***@x.com") {
		t.Errorf("expected an English-default alert for the live account b@x.com only, got %+v", h.sent)
	}
}

func TestAlertMissingGemini5hIsIgnored(t *testing.T) {
	h := newHarness(t, "")
	h.a.Observe(nil, "")
	h.a.Observe(&CockpitQuotaResponse{}, "")
	h.a.Observe(&CockpitQuotaResponse{CurrentAccount: &AccountQuota{ID: "x"}}, "")
	if len(h.sent) != 0 {
		t.Errorf("no data must not alert: %+v", h.sent)
	}
}

func TestAlertMessageContent(t *testing.T) {
	prevLang := i18n.GetLang()
	defer i18n.SetLang(prevLang)

	// 1. Default English alert
	i18n.SetLang(i18n.LangEn)
	hEn := newHarness(t, "")
	hEn.feed(0.40, 10*time.Minute)
	hEn.feed(0.32, 10*time.Minute)
	hEn.feed(0.24, 10*time.Minute)
	hEn.feed(0.18, 10*time.Minute)
	if len(hEn.sent) != 1 {
		t.Fatalf("expected one English alert, got %+v", hEn.sent)
	}
	aEn := hEn.sent[0]
	if aEn.Title != "⚠️ Gemini 5h quota down to 18%" {
		t.Errorf("en title = %q", aEn.Title)
	}
	if !strings.Contains(aEn.Body, "Account ta***@gmail.com") || strings.Contains(aEn.Body, "taojiuzhen@gmail.com") {
		t.Errorf("email prefix must be masked in push body: %q", aEn.Body)
	}
	if !strings.Contains(aEn.Body, "resets in 3h 20m (12:00)") {
		t.Errorf("en body should state reset countdown: %q", aEn.Body)
	}
	if !strings.Contains(aEn.Body, "runs out around") {
		t.Errorf("en body should carry exhaustion prediction: %q", aEn.Body)
	}

	// 2. Explicit Chinese alert
	i18n.SetLang(i18n.LangZh)
	h := newHarness(t, "")
	// 先有 3 个稳定消耗的样本，再触发 20%：正文应带预测
	h.feed(0.40, 10*time.Minute)
	h.feed(0.32, 10*time.Minute)
	h.feed(0.24, 10*time.Minute)
	h.feed(0.18, 10*time.Minute)
	if len(h.sent) != 1 {
		t.Fatalf("expected one alert, got %+v", h.sent)
	}
	a := h.sent[0]
	if a.Title != "⚠️ Gemini 5h 额度仅剩 18%" {
		t.Errorf("title = %q", a.Title)
	}
	if !strings.Contains(a.Body, "账号 ta***@gmail.com") || strings.Contains(a.Body, "taojiuzhen@gmail.com") {
		t.Errorf("email prefix must be masked in push body: %q", a.Body)
	}
	// 最后一次读数在 t0+40min，重置在 t0+4h → 还剩 3h 20m，且按当前时间现算而不是用快照里的文本
	if !strings.Contains(a.Body, "3h 20m后重置（12:00）") {
		t.Errorf("body should state the reset countdown computed from now: %q", a.Body)
	}
	if !strings.Contains(a.Body, "预计") || !strings.Contains(a.Body, "用完（早于重置）") {
		t.Errorf("body should carry the exhaustion prediction: %q", a.Body)
	}
}

func TestAlertMessageOmitsPredictionWithoutEnoughData(t *testing.T) {
	h := newHarness(t, "")
	h.feed(0.15, 10*time.Minute) // 刚启动就已经低于阈值：只有一个样本
	if len(h.sent) != 1 {
		t.Fatalf("expected one alert, got %d", len(h.sent))
	}
	if strings.Contains(h.sent[0].Body, "预计") || strings.Contains(h.sent[0].Body, "runs out") {
		t.Errorf("a single sample must not produce a prediction: %q", h.sent[0].Body)
	}
}

func TestAlertTitleWhenExhausted(t *testing.T) {
	prevLang := i18n.GetLang()
	defer i18n.SetLang(prevLang)

	i18n.SetLang(i18n.LangEn)
	if got := alertTitle("critical", 0); got != "🚨 Gemini 5h quota exhausted" {
		t.Errorf("en title = %q", got)
	}
	if got := alertTitle("critical", 0.04); got != "🚨 Gemini 5h quota down to 4%" {
		t.Errorf("en title = %q", got)
	}

	i18n.SetLang(i18n.LangZh)
	if got := alertTitle("critical", 0); got != "🚨 Gemini 5h 额度已用完" {
		t.Errorf("zh title = %q", got)
	}
	if got := alertTitle("critical", 0.04); got != "🚨 Gemini 5h 额度仅剩 4%" {
		t.Errorf("zh title = %q", got)
	}
}

func TestQuotaAlertConfigFromEnv(t *testing.T) {
	cases := []struct {
		warn, crit   string
		wantW, wantC float64
	}{
		{"", "", 0.20, 0.05},
		{"30", "10", 0.30, 0.10},
		{"25", "", 0.25, 0.05},
		{"", "2", 0.20, 0.02},
		{"5", "20", 0.20, 0.05}, // 反了：回退默认
		{"abc", "", 0.20, 0.05}, // 非法：回退默认
		{"0", "0", 0.20, 0.05},
		{"150", "", 0.20, 0.05},
		{"10", "10", 0.20, 0.05}, // 相等无意义
	}
	for _, c := range cases {
		t.Setenv("QUOTA_ALERT_WARN_PERCENT", c.warn)
		t.Setenv("QUOTA_ALERT_CRITICAL_PERCENT", c.crit)
		cfg := QuotaAlertConfigFromEnv()
		if cfg.WarnFraction != c.wantW || cfg.CriticalFraction != c.wantC {
			t.Errorf("warn=%q crit=%q => %.2f/%.2f, want %.2f/%.2f", c.warn, c.crit, cfg.WarnFraction, cfg.CriticalFraction, c.wantW, c.wantC)
		}
	}

	t.Setenv("QUOTA_ALERT_BONUS_RESET_ENABLED", "0")
	t.Setenv("QUOTA_ALERT_ANOMALY_ENABLED", "false")
	cfg := QuotaAlertConfigFromEnv()
	if !cfg.DisableBonusReset || !cfg.DisableAnomaly {
		t.Errorf("expected bonus reset and anomaly disabled via env, got %+v", cfg)
	}
}

func makeMultiBucketQuota(id, email string, updated time.Time, mutate func(*AccountQuota)) *CockpitQuotaResponse {
	acc := AccountQuota{
		ID:        id,
		Email:     email,
		IsCurrent: true,
		UpdatedAt: updated.UnixMilli(),
	}
	if mutate != nil {
		mutate(&acc)
	}
	return &CockpitQuotaResponse{CurrentAccount: &acc, Accounts: []AccountQuota{acc}}
}

func metricAt(frac float64, reset time.Time) *QuotaMetric {
	return &QuotaMetric{
		RemainingFraction: frac,
		RemainingPercent:  frac * 100,
		ResetTime:         reset.Format(time.RFC3339),
	}
}

func TestAlertAllFiveBucketsAndWeeklyOmitsETA(t *testing.T) {
	now := t0
	reset5h := t0.Add(4 * time.Hour)
	resetWeekly := t0.Add(3*24*time.Hour + 5*time.Hour)
	var sent []QuotaAlert

	a := newQuotaAlerter(QuotaAlertConfig{
		WarnFraction: 0.20, CriticalFraction: 0.05,
		Lang:   func() string { return "zh" },
		Now:    func() time.Time { return now },
		Notify: func(al QuotaAlert) { sent = append(sent, al) },
	})

	// 1. Weekly bucket drops steadily (4 samples) down to 18% -> must alert WITHOUT 60m ETA prediction, with day countdown
	for _, f := range []float64{0.30, 0.26, 0.22, 0.18} {
		now = now.Add(10 * time.Minute)
		a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
			acc.Gemini5h = metricAt(0.80, reset5h)
			acc.GeminiWeekly = metricAt(f, resetWeekly)
			acc.Claude5h = metricAt(0.90, reset5h)
			acc.ClaudeWeekly = metricAt(0.90, resetWeekly)
			acc.Gemini4Argon = metricAt(0.90, reset5h)
		}), "")
	}
	if len(sent) != 1 || sent[0].Level != "warn" {
		t.Fatalf("expected 1 weekly warn alert, got %+v", sent)
	}
	if !strings.Contains(sent[0].Title, "Gemini 周额度") || !strings.Contains(sent[0].Title, "18%") {
		t.Errorf("unexpected weekly title: %q", sent[0].Title)
	}
	if strings.Contains(sent[0].Body, "预计") {
		t.Errorf("weekly alert must not include 60m linear ETA: %q", sent[0].Body)
	}
	if !strings.Contains(sent[0].Body, "3d 4h") {
		t.Errorf("weekly alert should display day+hour reset countdown: %q", sent[0].Body)
	}

	// 2. Claude 5h & Gemini 4 Argon simultaneously cross low thresholds -> aggregated into ONE push
	now = now.Add(10 * time.Minute)
	a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(0.80, reset5h)
		acc.GeminiWeekly = metricAt(0.17, resetWeekly)
		acc.Claude5h = metricAt(0.15, reset5h)
		acc.ClaudeWeekly = metricAt(0.90, resetWeekly)
		acc.Gemini4Argon = metricAt(0.04, reset5h)
	}), "")
	if len(sent) != 2 {
		t.Fatalf("expected 1 aggregated alert for simultaneous Claude 5h + Argon drop, got %+v", sent)
	}
	if sent[1].Level != "critical" || !strings.Contains(sent[1].Body, "Claude 5h") || !strings.Contains(sent[1].Body, "Gemini 4 Argon") {
		t.Errorf("aggregated alert should escalate to critical and list both buckets: %+v", sent[1])
	}

	// 3. Claude Weekly on a FREE tier account (nil 5h pools)
	now = now.Add(10 * time.Minute)
	a.Observe(makeMultiBucketQuota("acc-free", "free@example.com", now, func(acc *AccountQuota) {
		acc.SubscriptionTier = "FREE"
		acc.ClaudeWeekly = metricAt(0.12, resetWeekly)
	}), "")
	if len(sent) != 3 || !strings.Contains(sent[2].Title, "Claude 周额度") {
		t.Fatalf("expected Claude Weekly alert for FREE tier account, got %+v", sent)
	}
}

func TestAlertLegacyStateFileCompatibility(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "quota_alert_state.json")
	reset := t0.Add(4 * time.Hour)
	legacy := `{"acc-1|` + reset.Format(time.RFC3339) + `":{"level":"warn","reset_time":"` + reset.Format(time.RFC3339) + `"}}`
	if err := os.WriteFile(statePath, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}

	var sent []QuotaAlert
	now := t0.Add(10 * time.Minute)
	a := newQuotaAlerter(QuotaAlertConfig{
		WarnFraction: 0.20, CriticalFraction: 0.05,
		StatePath: statePath,
		Lang:      func() string { return "zh" },
		Now:       func() time.Time { return now },
		Notify:    func(al QuotaAlert) { sent = append(sent, al) },
	})
	a.Observe(quotaFor("acc-1", "a@x.com", 0.15, now, reset), "")
	if len(sent) != 0 {
		t.Fatalf("legacy state entry must suppress duplicate gemini_5h warn, got %+v", sent)
	}
}

func TestAlertBonusResetEarlyRefillAndGuards(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	now := t0
	reset5h := t0.Add(4 * time.Hour)
	resetWeekly := t0.Add(5 * 24 * time.Hour)
	var sent []QuotaAlert

	a := newQuotaAlerter(QuotaAlertConfig{
		WarnFraction: 0.20, CriticalFraction: 0.05,
		StatePath: statePath,
		Lang:      func() string { return "zh" },
		Now:       func() time.Time { return now },
		Notify:    func(al QuotaAlert) { sent = append(sent, al) },
	})

	// Initial reading: 5h at 18% (fires warn), weekly at 45%
	now = now.Add(5 * time.Minute)
	a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(0.18, reset5h)
		acc.GeminiWeekly = metricAt(0.45, resetWeekly)
	}), "")
	if len(sent) != 1 || sent[0].Level != "warn" {
		t.Fatalf("expected initial warn, got %+v", sent)
	}

	// Official bonus reset 3h+ BEFORE scheduled reset: both pools jump to 100% -> ONE aggregated bonus_reset push!
	now = now.Add(10 * time.Minute)
	a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(1.00, reset5h) // even if resetTime stays identical (in-place refill)
		acc.GeminiWeekly = metricAt(1.00, resetWeekly.Add(24*time.Hour))
	}), "")
	if len(sent) != 2 || sent[1].Level != "bonus_reset" {
		t.Fatalf("expected 1 aggregated bonus_reset alert, got %+v", sent)
	}
	if !strings.Contains(sent[1].Body, "Gemini 5h") || !strings.Contains(sent[1].Body, "Gemini 周额度") {
		t.Errorf("aggregated bonus reset should list both pools: %q", sent[1].Body)
	}

	// After in-place refill (same resetTime), dropping to 18% again must re-arm warn!
	now = now.Add(30 * time.Minute)
	a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(0.18, reset5h)
		acc.GeminiWeekly = metricAt(0.90, resetWeekly.Add(24*time.Hour))
	}), "")
	if len(sent) != 3 || sent[2].Level != "warn" {
		t.Fatalf("in-place refill must re-arm low-quota warn, got %+v", sent)
	}

	// Guard 1: Switching accounts (acc-1 18% -> acc-2 100%) must NOT fire bonus_reset
	now = now.Add(5 * time.Minute)
	a.Observe(makeMultiBucketQuota("acc-2", "other@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(1.00, reset5h)
	}), "")
	if len(sent) != 3 {
		t.Fatalf("switching accounts must never fire bonus_reset, got %+v", sent)
	}

	// Guard 2: Small jump (+15% from 0.85 -> 1.00) must NOT fire bonus_reset
	now = now.Add(5 * time.Minute)
	a.Observe(makeMultiBucketQuota("acc-2", "other@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(0.85, reset5h)
	}), "")
	now = now.Add(5 * time.Minute)
	a.Observe(makeMultiBucketQuota("acc-2", "other@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(1.00, reset5h)
	}), "")
	if len(sent) != 3 {
		t.Fatalf("small refill (+15%%) must not trigger bonus_reset, got %+v", sent)
	}

	// Guard 3: Refill within 20m of scheduled reset (on-time reset) must NOT fire bonus_reset
	now = reset5h.Add(-15 * time.Minute)
	a.Observe(makeMultiBucketQuota("acc-2", "other@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(0.40, reset5h)
	}), "")
	now = reset5h.Add(-10 * time.Minute)
	a.Observe(makeMultiBucketQuota("acc-2", "other@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(1.00, reset5h.Add(5*time.Hour))
	}), "")
	if len(sent) != 3 {
		t.Fatalf("refill near scheduled reset must not fire bonus_reset, got %+v", sent)
	}
}

func TestAlertRapidDropAndIdleSilentBurn(t *testing.T) {
	now := t0
	reset5h := t0.Add(4 * time.Hour)
	resetWeekly := t0.Add(5 * 24 * time.Hour)
	var idleSince time.Time
	idleOK := false
	var sent []QuotaAlert

	a := newQuotaAlerter(QuotaAlertConfig{
		WarnFraction: 0.20, CriticalFraction: 0.05,
		Lang:          func() string { return "zh" },
		Now:           func() time.Time { return now },
		HostIdleSince: func() (time.Time, bool) { return idleSince, idleOK },
		Notify:        func(al QuotaAlert) { sent = append(sent, al) },
	})

	// 1. Active mode: Weekly drops 18% in 10 minutes (82% -> 64%) -> fires rapid drop anomaly
	now = now.Add(5 * time.Minute)
	a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(0.95, reset5h)
		acc.GeminiWeekly = metricAt(0.82, resetWeekly)
	}), "")
	now = now.Add(10 * time.Minute)
	a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(0.65, reset5h) // 30% drop on 5h is below 45% rapid threshold -> silent
		acc.GeminiWeekly = metricAt(0.64, resetWeekly)
	}), "")
	if len(sent) != 1 || sent[0].Level != "anomaly" || !strings.Contains(sent[0].Title, "短时骤降") {
		t.Fatalf("expected rapid drop anomaly for weekly -18%%, got %+v", sent)
	}

	// Cooldown guard: another rapid weekly drop 10m later must be suppressed (4h cooldown)
	now = now.Add(10 * time.Minute)
	a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(0.60, reset5h)
		acc.GeminiWeekly = metricAt(0.45, resetWeekly)
	}), "")
	if len(sent) != 1 {
		t.Fatalf("rapid drop must obey 4h cooldown, got %+v", sent)
	}

	// 2. Idle guard: task ended at `now`, prev reading was captured BEFORE idleSince+2m -> must NOT false-alarm as idle burn
	idleSince = now
	idleOK = true
	now = now.Add(25 * time.Minute) // host idle 25m, but prev sample was taken right at idleSince (not after idleSince+2m)
	a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(0.42, reset5h) // -18% from pre-idle sample
		acc.GeminiWeekly = metricAt(0.45, resetWeekly)
	}), "")
	if len(sent) != 1 {
		t.Fatalf("drop from pre-idle sample must not false-alarm as idle burn, got %+v", sent)
	}

	// 3. Confirmed idle silent burn: prev sample was ALREADY captured at idleSince+25m, now another 10m later 5h drops 18% -> fires!
	now = now.Add(10 * time.Minute)
	a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(0.24, reset5h)
		acc.GeminiWeekly = metricAt(0.45, resetWeekly)
	}), "")
	if len(sent) != 2 || sent[1].Level != "anomaly" || !strings.Contains(sent[1].Title, "空闲期") {
		t.Fatalf("expected confirmed idle burn alert, got %+v", sent)
	}
}

func TestAlertTrilingualLocalization(t *testing.T) {
	reset5h := t0.Add(3 * time.Hour)
	for _, tc := range []struct {
		lang       string
		wantWarn   string
		wantBonus  string
		wantRapid  string
		wantIdle   string
	}{
		{"zh", "⚠️ Gemini 5h 额度仅剩 18%", "🎁 Gemini 5h 额度已提前回满", "⚡ Gemini 周额度短时骤降 20%", "🔍 空闲期检测到 Gemini 5h 额度下降"},
		{"en", "⚠️ Gemini 5h quota down to 18%", "🎁 Gemini 5h refilled early", "⚡ Gemini Weekly quota dropped 20% rapidly", "🔍 Gemini 5h quota dropped while idle"},
		{"ja", "⚠️ Gemini 5h クォータ残り 18%", "🎁 Gemini 5h 枠が早期回復", "⚡ Gemini 週間クォータが短時間で 20% 急減", "🔍 アイドル中に Gemini 5h クォータが減少"},
	} {
		now := t0
		idleSince := time.Time{}
		idleOK := false
		var sent []QuotaAlert
		a := newQuotaAlerter(QuotaAlertConfig{
			WarnFraction: 0.20, CriticalFraction: 0.05,
			Lang:          func() string { return tc.lang },
			Now:           func() time.Time { return now },
			HostIdleSince: func() (time.Time, bool) { return idleSince, idleOK },
			Notify:        func(al QuotaAlert) { sent = append(sent, al) },
		})

		// Warn
		now = now.Add(5 * time.Minute)
		a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
			acc.Gemini5h = metricAt(0.18, reset5h)
			acc.GeminiWeekly = metricAt(0.80, t0.Add(5*24*time.Hour))
		}), "")

		// Bonus reset
		now = now.Add(5 * time.Minute)
		a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
			acc.Gemini5h = metricAt(1.00, reset5h.Add(5*time.Hour))
			acc.GeminiWeekly = metricAt(0.80, t0.Add(5*24*time.Hour))
		}), "")

		// Rapid drop on weekly (-20%)
		now = now.Add(5 * time.Minute)
		a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
			acc.Gemini5h = metricAt(0.90, reset5h.Add(5*time.Hour))
			acc.GeminiWeekly = metricAt(0.60, t0.Add(5*24*time.Hour))
		}), "")

		// Idle burn on 5h (-20% after host idle 30m)
		idleSince = now
		idleOK = true
		now = now.Add(25 * time.Minute)
		a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
			acc.Gemini5h = metricAt(0.90, reset5h.Add(5*time.Hour))
			acc.GeminiWeekly = metricAt(0.60, t0.Add(5*24*time.Hour))
		}), "")
		now = now.Add(10 * time.Minute)
		a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
			acc.Gemini5h = metricAt(0.70, reset5h.Add(5*time.Hour))
			acc.GeminiWeekly = metricAt(0.60, t0.Add(5*24*time.Hour))
		}), "")

		if len(sent) != 4 {
			t.Fatalf("[%s] expected 4 alerts, got %d: %+v", tc.lang, len(sent), sent)
		}
		if sent[0].Title != tc.wantWarn {
			t.Errorf("[%s] warn title = %q, want %q", tc.lang, sent[0].Title, tc.wantWarn)
		}
		if sent[1].Title != tc.wantBonus {
			t.Errorf("[%s] bonus title = %q, want %q", tc.lang, sent[1].Title, tc.wantBonus)
		}
		if sent[2].Title != tc.wantRapid {
			t.Errorf("[%s] rapid title = %q, want %q", tc.lang, sent[2].Title, tc.wantRapid)
		}
		if sent[3].Title != tc.wantIdle {
			t.Errorf("[%s] idle title = %q, want %q", tc.lang, sent[3].Title, tc.wantIdle)
		}
	}
}

func TestStartQuotaAlerterUsesCustomFetchQuotaWithoutCockpit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var notified atomic.Int32
	done := make(chan QuotaAlert, 1)
	reset := time.Now().Add(3 * time.Hour)

	StartQuotaAlerter(ctx, QuotaAlertConfig{
		WarnFraction:     0.20,
		CriticalFraction: 0.05,
		PollInterval:     20 * time.Millisecond,
		Lang:             func() string { return "en" },
		FetchQuota: func() (*CockpitQuotaResponse, string, error) {
			return makeMultiBucketQuota("live:u@example.com", "u@example.com", time.Now(), func(acc *AccountQuota) {
				acc.Gemini5h = metricAt(0.12, reset)
			}), "u@example.com", nil
		},
		Notify: func(a QuotaAlert) {
			if notified.Add(1) == 1 {
				done <- a
			}
		},
	})

	select {
	case a := <-done:
		if a.Level != "warn" || !strings.Contains(a.Title, "Gemini 5h quota down to 12%") {
			t.Errorf("unexpected alert from custom FetchQuota: %+v", a)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for StartQuotaAlerter custom FetchQuota alert")
	}
}

func TestBonusResetCompactTitlesForLockScreen(t *testing.T) {
	acc := &AccountQuota{ID: "acc-1", Email: "kenji@gmail.com"}
	weeklySpec := monitoredBuckets[1] // gemini_weekly
	fiveHSpec := monitoredBuckets[0]  // gemini_5h
	resetAt := time.Date(2026, 11, 28, 17, 40, 3, 0, time.UTC)
	now := time.Date(2026, 11, 23, 20, 7, 42, 0, time.UTC)

	singleHit := []bonusResetHit{{
		spec:      weeklySpec,
		bonusKey:  "k1",
		prevFrac:  0.52,
		curFrac:   0.99,
		prevReset: resetAt,
	}}
	multiHits := []bonusResetHit{
		singleHit[0],
		{spec: fiveHSpec, bonusKey: "k2", prevFrac: 0.40, curFrac: 1.00, prevReset: now.Add(3 * time.Hour)},
	}

	for _, tc := range []struct {
		lang       string
		wantSingle string
		wantMulti  string
	}{
		{"zh", "🎁 Gemini 周额度已提前回满", "🎁 Antigravity 额度已提前回满"},
		{"en", "🎁 Gemini Weekly refilled early", "🎁 Antigravity refilled early"},
		{"ja", "🎁 Gemini 週間枠が早期回復", "🎁 Antigravity 枠が早期回復"},
	} {
		gotSingle := buildBonusResetAlert(tc.lang, acc, singleHit, now)
		if gotSingle.Title != tc.wantSingle {
			t.Errorf("[%s] single weekly bonus title = %q, want %q", tc.lang, gotSingle.Title, tc.wantSingle)
		}
		gotMulti := buildBonusResetAlert(tc.lang, acc, multiHits, now)
		if gotMulti.Title != tc.wantMulti {
			t.Errorf("[%s] multi bonus title = %q, want %q", tc.lang, gotMulti.Title, tc.wantMulti)
		}
	}
}

func TestAlertSuppressesMirroredArgonAcrossAllAlertTypes(t *testing.T) {
	now := t0
	reset5h := t0.Add(4 * time.Hour)
	reset5hJitter := reset5h.Add(-1 * time.Second) // real 1s model-vs-bucket jitter
	resetWeekly := t0.Add(5 * 24 * time.Hour)

	var sent []QuotaAlert
	a := newQuotaAlerter(QuotaAlertConfig{
		WarnFraction:     0.20,
		CriticalFraction: 0.05,
		Lang:             func() string { return "zh" },
		Now:              func() time.Time { return now },
		Notify:           func(al QuotaAlert) { sent = append(sent, al) },
	})

	// 1. Steady drop to 18%: both Gemini5h and mirrored Gemini4Argon cross warn threshold.
	// Only Gemini 5h should alert (single-bucket title, no phantom "Gemini 4 Argon").
	for _, f := range []float64{0.60, 0.45, 0.18} {
		now = now.Add(5 * time.Minute)
		a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
			acc.Gemini5h = metricAt(f, reset5h)
			acc.GeminiWeekly = metricAt(0.85, resetWeekly)
			acc.Gemini4Argon = metricAt(f, reset5hJitter)
		}), "")
	}
	if len(sent) != 1 {
		t.Fatalf("expected 1 low-quota alert (not duplicated for Argon), got %d: %+v", len(sent), sent)
	}
	if strings.Contains(sent[0].Title, "Argon") || strings.Contains(sent[0].Body, "Argon") {
		t.Errorf("mirrored Argon must not appear in low-quota alert: title=%q body=%q", sent[0].Title, sent[0].Body)
	}

	// 2. Official early refill (18% -> 100%) on both Gemini5h and mirrored Argon:
	// must emit single-bucket Gemini 5h bonus reset, NOT multi-bucket with Argon.
	now = now.Add(10 * time.Minute)
	a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(1.0, reset5h)
		acc.GeminiWeekly = metricAt(0.85, resetWeekly)
		acc.Gemini4Argon = metricAt(1.0, reset5hJitter)
	}), "")
	if len(sent) != 2 || sent[1].Level != "bonus_reset" {
		t.Fatalf("expected 1 bonus_reset alert, got %+v", sent)
	}
	if strings.Contains(sent[1].Title, "Argon") || strings.Contains(sent[1].Body, "Argon") || !strings.Contains(sent[1].Title, "Gemini 5h") {
		t.Errorf("expected single-bucket Gemini 5h bonus reset without Argon, got title=%q body=%q", sent[1].Title, sent[1].Body)
	}

	// 3. Rapid 15m drop (100% -> 50%, >=45% threshold) on both Gemini5h and mirrored Argon:
	// must emit single-bucket anomaly without Argon.
	now = now.Add(5 * time.Minute)
	a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(0.50, reset5h)
		acc.GeminiWeekly = metricAt(0.85, resetWeekly)
		acc.Gemini4Argon = metricAt(0.50, reset5hJitter)
	}), "")
	if len(sent) != 3 || sent[2].Level != "anomaly" {
		t.Fatalf("expected 1 rapid anomaly alert, got %+v", sent)
	}
	if strings.Contains(sent[2].Title, "Argon") || strings.Contains(sent[2].Body, "Argon") {
		t.Errorf("mirrored Argon must not appear in anomaly alert: title=%q body=%q", sent[2].Title, sent[2].Body)
	}
}

func TestAlertIgnoresPlaceholderAndNormalizesMissingFraction(t *testing.T) {
	p := placeholderReadyMetric()
	if p.RemainingFraction != 1.0 || p.RemainingPercent != 100 || !p.IsPlaceholder {
		t.Fatalf("unexpected placeholderReadyMetric: %+v", p)
	}

	now := t0
	reset5h := t0.Add(4 * time.Hour)
	resetWeekly := t0.Add(5 * 24 * time.Hour)

	var sent []QuotaAlert
	a := newQuotaAlerter(QuotaAlertConfig{
		WarnFraction:     0.20,
		CriticalFraction: 0.05,
		Lang:             func() string { return "zh" },
		Now:              func() time.Time { return now },
		Notify:           func(al QuotaAlert) { sent = append(sent, al) },
	})

	// 1. Real reading at 40% weekly.
	a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(0.95, reset5h)
		acc.GeminiWeekly = metricAt(0.40, resetWeekly)
		acc.ClaudeWeekly = metricAt(0.50, resetWeekly)
	}), "")

	// 2. Transient Cockpit fallback omits weekly buckets -> synthetic placeholders must NOT trigger
	// false critical (0%), false bonus_reset (40% -> 100%), or pollute previous-value tracker.
	now = now.Add(30 * time.Second)
	a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(0.95, reset5h)
		acc.GeminiWeekly = placeholderReadyMetric()
		acc.ClaudeWeekly = placeholderReadyMetric()
	}), "")

	// 3. Next live poll restores real readings -> must NOT trigger false rapid anomaly drop (100% -> 39%).
	now = now.Add(30 * time.Second)
	a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = metricAt(0.94, reset5h)
		acc.GeminiWeekly = metricAt(0.39, resetWeekly)
		acc.ClaudeWeekly = metricAt(0.49, resetWeekly)
	}), "")

	// 4. Defensive normalization: a non-placeholder metric with RemainingPercent=100 but unset
	// RemainingFraction=0 must normalize to 1.0 and never fire a false critical exhausted alert.
	now = now.Add(30 * time.Second)
	a.Observe(makeMultiBucketQuota("acc-1", "dev@example.com", now, func(acc *AccountQuota) {
		acc.Gemini5h = &QuotaMetric{RemainingPercent: 100, ResetFriendly: "就绪"}
		acc.GeminiWeekly = metricAt(0.39, resetWeekly)
		acc.ClaudeWeekly = metricAt(0.49, resetWeekly)
	}), "")

	if len(sent) != 0 {
		t.Fatalf("expected 0 alerts across placeholder & missing-fraction snapshots, got %d: %+v", len(sent), sent)
	}
}
