package cockpit

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestPricingBuiltIn2026AndDynamicCatalog(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MGY_DISABLE_PRICING_SYNC", "1")
	ResetDynamicPricingCache()
	t.Cleanup(ResetDynamicPricingCache)

	// Built-in 2026 Gemini 4 & Claude 5.5 & GPT-OSS rates
	neon := CalculateCostUSD("Gemini 4 Neon (High)", 1_000_000, 1_000_000, 1_000_000)
	if math.Abs(neon-14.2) > 0.01 {
		t.Fatalf("Gemini 4 Neon cost = %v, want 14.2", neon)
	}
	argon := CalculateCostUSD("Gemini 4 Argon", 1_000_000, 1_000_000, 1_000_000)
	if math.Abs(argon-10.65) > 0.01 {
		t.Fatalf("Gemini 4 Argon cost = %v, want 10.65", argon)
	}
	hydrogen := CalculateCostUSD("Gemini 4 Hydrogen", 1_000_000, 1_000_000, 1_000_000)
	if math.Abs(hydrogen-3.55) > 0.01 {
		t.Fatalf("Gemini 4 Hydrogen cost = %v, want 3.55", hydrogen)
	}
	opus := CalculateCostUSD("Claude Opus 5.5 (Thinking)", 1_000_000, 1_000_000, 1_000_000)
	if math.Abs(opus-30.5) > 0.01 {
		t.Fatalf("Claude Opus 5.5 cost = %v, want 30.5", opus)
	}

	// Plan subscription ROI helpers
	if label, price := PlanMonthlyUSDForTier("ULTRA"); label != "Ultra" || math.Abs(price-249.99) > 0.01 {
		t.Fatalf("ULTRA tier = (%q, %v), want (Ultra, 249.99)", label, price)
	}
	if label, price := PlanMonthlyUSDForTier("PRO"); label != "Pro" || math.Abs(price-19.99) > 0.01 {
		t.Fatalf("PRO tier = (%q, %v), want (Pro, 19.99)", label, price)
	}
	if label, price := PlanMonthlyUSDForTier("FREE"); label != "Free" || price != 0 {
		t.Fatalf("FREE tier = (%q, %v), want (Free, 0)", label, price)
	}

	// models.dev JSON parser prefers primary provider & synthesizes 10% cache_read fallback
	sampleJSON := []byte(`{
		"reseller": {
			"models": {
				"gemini-4-neon": {"cost": {"input": 99.0, "output": 199.0}}
			}
		},
		"google": {
			"models": {
				"gemini-4-neon": {"cost": {"input": 2.5, "output": 15.0}}
			}
		}
	}`)
	parsed := ParseModelsDevCatalog(sampleJSON)
	rate, ok := parsed["gemini-4-neon"]
	if !ok {
		t.Fatalf("expected gemini-4-neon in parsed catalog, got %+v", parsed)
	}
	if math.Abs(rate.InputPerM-2.5) > 0.001 || math.Abs(rate.OutputPerM-15.0) > 0.001 || math.Abs(rate.CacheReadPerM-0.25) > 0.001 {
		t.Fatalf("unexpected parsed rate: %+v", rate)
	}

	// Dynamic sync persists & overlays lookup
	origFetcher := dynamicPricingFetcher
	dynamicPricingFetcher = func(ctx context.Context) ([]byte, error) {
		return sampleJSON, nil
	}
	t.Cleanup(func() { dynamicPricingFetcher = origFetcher })

	if err := SyncDynamicPricingCatalog(context.Background()); err != nil {
		t.Fatalf("SyncDynamicPricingCatalog failed: %v", err)
	}
	overlaid := CalculateCostUSD("Gemini 4 Neon", 1_000_000, 1_000_000, 1_000_000)
	// 2.5 + 0.25 + 15.0 = 17.75
	if math.Abs(overlaid-17.75) > 0.01 {
		t.Fatalf("overlaid Gemini 4 Neon cost = %v, want 17.75", overlaid)
	}
}

func TestModelSpeedStatsMedianAndDedup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ResetModelSpeedStats()
	t.Cleanup(ResetModelSpeedStats)

	now := time.Now()
	// Out-of-window sample (10 days ago) should be pruned
	RecordModelSpeedSample("Gemini 4 Neon", 500.0, 0.1, "old-resp", now.Add(-10*24*time.Hour))
	// Valid samples
	RecordModelSpeedSample("Gemini 4 Neon", 40.0, 1.20, "r1", now.Add(-3*time.Minute))
	RecordModelSpeedSample("Gemini 4 Neon", 80.0, 0.80, "r2", now.Add(-2*time.Minute))
	RecordModelSpeedSample("Gemini 4 Neon", 60.0, 1.00, "r3", now.Add(-1*time.Minute))
	// Duplicate responseID should be ignored
	RecordModelSpeedSample("Gemini 4 Neon", 999.0, 9.99, "r3", now)

	snap := SnapshotModelSpeedStats()
	st, ok := snap["Gemini 4 Neon"]
	if !ok {
		t.Fatalf("expected Gemini 4 Neon in snapshot, got %+v", snap)
	}
	if st.Samples != 3 {
		t.Fatalf("samples = %d, want 3", st.Samples)
	}
	if math.Abs(st.MedianTPS-60.0) > 0.1 {
		t.Fatalf("median TPS = %v, want 60.0", st.MedianTPS)
	}
	if math.Abs(st.MedianTTFT-1.00) > 0.02 {
		t.Fatalf("median TTFT = %v, want 1.00", st.MedianTTFT)
	}
}

func TestPoolKeyForModelAndQuotaTokenEstimates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ResetQuotaEstimateCaches()
	t.Cleanup(ResetQuotaEstimateCaches)

	if got := PoolKeyForModel("Orchestrator (Argon)"); got != "gemini_main" {
		t.Fatalf("Orchestrator (Argon) pool = %q, want gemini_main", got)
	}
	if got := PoolKeyForModel("Gemini 4 Argon"); got != "gemini_4_argon" {
		t.Fatalf("Gemini 4 Argon pool = %q, want gemini_4_argon", got)
	}
	if got := PoolKeyForModel("Claude Sonnet 5.5"); got != "claude" {
		t.Fatalf("Claude Sonnet 5.5 pool = %q, want claude", got)
	}

	now := time.Now().UTC().Truncate(time.Second)
	resetAt := now.Add(3 * time.Hour).Format(time.RFC3339)
	cycleStart := now.Add(-2 * time.Hour)

	// Record an initial observation at ~100% remaining so the bucket is observed from cycle start.
	respStart := &CockpitQuotaResponse{
		CurrentAccount: &AccountQuota{
			ID:               "acc-1",
			Email:            "user@example.com",
			SubscriptionTier: "PRO",
			Gemini5h: &QuotaMetric{
				RemainingFraction: 1.0,
				RemainingPercent:  100.0,
				ResetTime:         resetAt,
			},
		},
	}
	ApplyQuotaTokenEstimates(respStart, cycleStart.Add(2*time.Minute))

	// Simulate 400K tokens consumed on gemini_main inside this 5h window.
	RecordPoolUsageEvent(cycleStart.Add(30*time.Minute), "Gemini 4 Neon", 100_000, 200_000, 100_000, "ev-1")
	// Separate Argon standalone usage must not contaminate gemini_main.
	RecordPoolUsageEvent(cycleStart.Add(40*time.Minute), "Gemini 4 Argon", 500_000, 0, 0, "ev-argon")

	// Now quota drops to 60% remaining (40% used => calibrated total = 1,000,000, remaining ~600,000).
	respMid := &CockpitQuotaResponse{
		CurrentAccount: &AccountQuota{
			ID:               "acc-1",
			Email:            "user@example.com",
			SubscriptionTier: "PRO",
			Gemini5h: &QuotaMetric{
				RemainingFraction: 0.60,
				RemainingPercent:  60.0,
				ResetTime:         resetAt,
			},
		},
	}
	ApplyQuotaTokenEstimates(respMid, now)

	g5 := respMid.CurrentAccount.Gemini5h
	if g5.EstCycleUsedTokens != 400_000 {
		t.Fatalf("EstCycleUsedTokens = %d, want 400000", g5.EstCycleUsedTokens)
	}
	if !g5.EstReliable {
		t.Fatalf("expected EstReliable = true after full-cycle calibration, got %+v", g5)
	}
	if g5.EstTotalTokens < 950_000 || g5.EstTotalTokens > 1_050_000 {
		t.Fatalf("EstTotalTokens = %d, want ~1000000", g5.EstTotalTokens)
	}
	if g5.EstRemainingTokens < 550_000 || g5.EstRemainingTokens > 650_000 {
		t.Fatalf("EstRemainingTokens = %d, want ~600000", g5.EstRemainingTokens)
	}
}

func TestAutoSwitchStateMachineIdleSafe(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ResetAutoSwitchForTest()
	t.Cleanup(ResetAutoSwitchForTest)

	enabled := true
	bucket := AutoSwitchBucketGemini5h
	thresh := 15.0
	waitIdle := true
	debounce := 15
	cooldown := 180
	if _, err := UpdateAutoSwitchConfig(AutoSwitchConfigPatch{
		Enabled:          &enabled,
		TargetBucket:     &bucket,
		ThresholdPercent: &thresh,
		WaitUntilIdle:    &waitIdle,
		DebounceSec:      &debounce,
		CooldownSec:      &cooldown,
	}); err != nil {
		t.Fatalf("UpdateAutoSwitchConfig failed: %v", err)
	}

	makeResp := func(curPct, b1Pct, b2Pct float64) *CockpitQuotaResponse {
		cur := AccountQuota{
			ID: "a1", Email: "a@example.com", IsCurrent: true, SubscriptionTier: "PRO",
			Gemini5h: &QuotaMetric{RemainingPercent: curPct, RemainingFraction: curPct / 100.0},
		}
		b1 := AccountQuota{
			ID: "b1", Email: "b@example.com", SubscriptionTier: "PRO",
			Gemini5h: &QuotaMetric{RemainingPercent: b1Pct, RemainingFraction: b1Pct / 100.0},
		}
		b2 := AccountQuota{
			ID: "b2", Email: "c@example.com", SubscriptionTier: "ULTRA",
			Gemini5h: &QuotaMetric{RemainingPercent: b2Pct, RemainingFraction: b2Pct / 100.0},
		}
		return &CockpitQuotaResponse{
			CockpitInstalled: true,
			CurrentAccount:   &cur,
			Accounts:         []AccountQuota{cur, b1, b2},
		}
	}

	baseTime := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)
	now := baseTime
	busy := true
	switchedTo := ""
	var alerts []QuotaAlert

	rt := AutoSwitchRuntime{
		IsBusy:        func() bool { return busy },
		SwitchAccount: func(id string) error { switchedTo = id; return nil },
		Notify:        func(a QuotaAlert) { alerts = append(alerts, a) },
		Now:           func() time.Time { return now },
	}

	// 1. Healthy quota -> watching
	st := EvaluateAutoSwitch(makeResp(45, 90, 60), "a@example.com", rt)
	if st.State != "watching" || switchedTo != "" {
		t.Fatalf("expected watching above threshold, got %+v", st)
	}

	// 2. Email mismatch guard -> watching with active_email_mismatch
	st = EvaluateAutoSwitch(makeResp(5, 90, 60), "other@example.com", rt)
	if st.State != "watching" || st.Reason != "active_email_mismatch" {
		t.Fatalf("expected active_email_mismatch, got %+v", st)
	}

	// 3. Drop below threshold -> pending_debounce first
	st = EvaluateAutoSwitch(makeResp(8, 90, 60), "a@example.com", rt)
	if st.State != "pending_debounce" || switchedTo != "" {
		t.Fatalf("expected pending_debounce, got %+v", st)
	}

	// 4. Advance past debounce while busy -> waiting_idle (no interruption!)
	now = baseTime.Add(20 * time.Second)
	st = EvaluateAutoSwitch(makeResp(8, 90, 60), "a@example.com", rt)
	if st.State != "waiting_idle" || switchedTo != "" {
		t.Fatalf("expected waiting_idle while busy, got %+v", st)
	}

	// 5. Become idle -> switches to highest quota backup (b1 @ 90%) & fires localized alert
	busy = false
	now = baseTime.Add(25 * time.Second)
	st = EvaluateAutoSwitch(makeResp(8, 90, 60), "a@example.com", rt)
	if st.State != "switched" || switchedTo != "b1" {
		t.Fatalf("expected switched to b1, got state=%+v switchedTo=%q", st, switchedTo)
	}
	if len(alerts) != 1 || alerts[0].Localize == nil {
		t.Fatalf("expected 1 localized alert, got %+v", alerts)
	}
	enTitle, _ := alerts[0].Localize("en")
	jaTitle, _ := alerts[0].Localize("ja")
	if enTitle == "" || jaTitle == "" {
		t.Fatalf("expected non-empty localized alert titles, got en=%q ja=%q", enTitle, jaTitle)
	}

	// 6. Within cooldown -> cooldown (no second switch)
	switchedTo = ""
	now = baseTime.Add(60 * time.Second)
	st = EvaluateAutoSwitch(makeResp(5, 90, 60), "a@example.com", rt)
	if st.State != "cooldown" || switchedTo != "" {
		t.Fatalf("expected cooldown, got %+v", st)
	}

	// 7. candidate_order strategy picks explicit first eligible candidate even if lower than another
	strategy := AutoSwitchStrategyCandidateOrder
	cands := []string{"c@example.com", "b@example.com"}
	debounceZero := 0
	if _, err := UpdateAutoSwitchConfig(AutoSwitchConfigPatch{
		Strategy:    &strategy,
		Candidates:  &cands,
		DebounceSec: &debounceZero,
	}); err != nil {
		t.Fatalf("UpdateAutoSwitchConfig candidate_order failed: %v", err)
	}
	now = baseTime.Add(300 * time.Second)
	st = EvaluateAutoSwitch(makeResp(5, 90, 60), "a@example.com", rt)
	if st.State != "switched" || switchedTo != "b2" {
		t.Fatalf("expected candidate_order to switch to b2 (c@example.com), got state=%+v switchedTo=%q", st, switchedTo)
	}
}
