package cockpit

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestMaskEmail(t *testing.T) {
	cases := map[string]string{
		"taojiuzhen@gmail.com": "t***n@gmail.com",
		"ab@x.io":              "a***@x.io",
		"a@x.io":               "a***@x.io",
		"no-at-sign":           "no-at-sign",
	}
	for in, want := range cases {
		if got := maskEmail(in); got != want {
			t.Errorf("maskEmail(%q) = %q, want %q", in, got, want)
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
	// 重置：新的重置时间点，额度回满，随后又用到 18%
	h.reset = h.reset.Add(5 * time.Hour)
	h.feed(1.00, 10*time.Minute)
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
	if len(h.sent) != 1 || !strings.Contains(h.sent[0].Body, "b***@x.com") || strings.Contains(h.sent[0].Body, "a***") {
		t.Errorf("expected an alert for the live account b@x.com only, got %+v", h.sent)
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
	if !strings.Contains(a.Body, "t***n@gmail.com") || strings.Contains(a.Body, "taojiuzhen") {
		t.Errorf("email must be masked in push body: %q", a.Body)
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
	if strings.Contains(h.sent[0].Body, "预计") {
		t.Errorf("a single sample must not produce a prediction: %q", h.sent[0].Body)
	}
}

func TestAlertTitleWhenExhausted(t *testing.T) {
	if got := alertTitle("critical", 0); got != "🚨 Gemini 5h 额度已用完" {
		t.Errorf("title = %q", got)
	}
	if got := alertTitle("critical", 0.04); got != "🚨 Gemini 5h 额度仅剩 4%" {
		t.Errorf("title = %q", got)
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
}
