package proxy

import (
	"strings"
	"testing"

	"antigravity-mobile/internal/cockpit"
)

func TestParseGeneratorMetadataProtoJSONStrings(t *testing.T) {
	payload := `{
		"generatorMetadata": [
			{
				"chatModel": {
					"model": "MODEL_PLACEHOLDER_M526",
					"modelDisplayName": "Gemini 4 Hydrogen (Medium)",
					"timeToFirstToken": "5.102348375s",
					"streamingDuration": "0.506033083s",
					"chatStartMetadata": {
						"contextWindowMetadata": {
							"estimatedTokensUsed": 39537
						}
					},
					"usage": {
						"inputTokens": "2904",
						"outputTokens": "401",
						"thinkingOutputTokens": "123",
						"cacheReadTokens": "27768"
					},
					"retryInfos": [
						{"traceId": "d510ee77c075353e"}
					]
				}
			},
			{
				"chatModel": {}
			}
		]
	}`

	item := parseGeneratorMetadata(strings.NewReader(payload))
	if item == nil {
		t.Fatal("expected non-nil ContextMeterItem")
	}
	if item.UsedTokens != 39537 {
		t.Fatalf("expected UsedTokens=39537, got %d", item.UsedTokens)
	}
	if item.MaxTokens != 200000 {
		t.Fatalf("expected default MaxTokens=200000, got %d", item.MaxTokens)
	}
	if item.InputTokens != 2904 || item.OutputTokens != 401 || item.CacheReadTokens != 27768 || item.ThinkingTokens != 123 {
		t.Fatalf("unexpected token breakdown: %+v", item)
	}
	if item.TokensPerSecond <= 0 {
		t.Fatalf("expected positive TokensPerSecond, got %v", item.TokensPerSecond)
	}
	if item.IsOverflow {
		t.Fatalf("expected IsOverflow=false")
	}
}

func TestParseGeneratorMetadataQuotedContextWindowAndOverflow(t *testing.T) {
	payload := `{
		"generatorMetadata": [
			{
				"chatModel": {
					"model": "MODEL_CLAUDE",
					"chatStartMetadata": {
						"contextWindowMetadata": {
							"maxContextTokens": "1000000",
							"estimatedTokensUsed": "950000"
						}
					},
					"usage": {
						"inputTokens": "900000",
						"outputTokens": "50000"
					},
					"retryInfos": [
						{"error": "prompt is too long: 1005000 tokens > 1000000 maximum"}
					]
				}
			}
		]
	}`

	item := parseGeneratorMetadata(strings.NewReader(payload))
	if item == nil {
		t.Fatal("expected non-nil ContextMeterItem")
	}
	if item.MaxTokens != 1000000 || item.UsedTokens != 950000 {
		t.Fatalf("unexpected context window: %+v", item)
	}
	if item.Percentage != 95.0 {
		t.Fatalf("expected Percentage=95.0, got %v", item.Percentage)
	}
	if !item.IsOverflow || item.ErrorKind != "context_overflow" {
		t.Fatalf("expected context_overflow, got IsOverflow=%v ErrorKind=%q", item.IsOverflow, item.ErrorKind)
	}
}

func TestParseGeneratorMetadataClaudeCacheCreation(t *testing.T) {
	payload := `{
		"generatorMetadata": [
			{
				"chatModel": {
					"model": "MODEL_PLACEHOLDER_M402",
					"modelDisplayName": "Claude Opus 5.5 (High)",
					"usage": {
						"inputTokens": "500",
						"cacheCreationTokens": "18000",
						"cacheReadTokens": "42000",
						"outputTokens": "1200",
						"thinkingOutputTokens": "450"
					}
				}
			}
		]
	}`

	item := parseGeneratorMetadata(strings.NewReader(payload))
	if item == nil {
		t.Fatal("expected non-nil ContextMeterItem")
	}
	if item.InputTokens != 18500 || item.CacheReadTokens != 42000 || item.OutputTokens != 1200 || item.ThinkingTokens != 450 {
		t.Fatalf("unexpected Claude usage breakdown: %+v", item)
	}
	if item.UsedTokens != 61700 {
		t.Fatalf("expected UsedTokens=61700, got %d", item.UsedTokens)
	}
	if item.CacheHitRate == nil || *item.CacheHitRate != 69.4 {
		t.Fatalf("expected CacheHitRate=69.4, got %v", item.CacheHitRate)
	}
	if item.CacheStatus != "healthy" || item.CacheMissStreak != 0 {
		t.Fatalf("expected healthy cache status, got status=%q streak=%d", item.CacheStatus, item.CacheMissStreak)
	}
}

func TestParseGeneratorMetadataCacheMissStreakAndGuards(t *testing.T) {
	t.Run("first cold start step never alarms", func(t *testing.T) {
		payload := `{
			"generatorMetadata": [
				{"chatModel": {"usage": {"inputTokens": "50000", "cacheReadTokens": "0", "outputTokens": "500"}}}
			]
		}`
		item := parseGeneratorMetadata(strings.NewReader(payload))
		if item == nil {
			t.Fatal("expected non-nil item")
		}
		if item.CacheHitRate == nil || *item.CacheHitRate != 0.0 {
			t.Fatalf("expected CacheHitRate=0.0, got %v", item.CacheHitRate)
		}
		if item.CacheMissStreak != 0 || item.CacheStatus != "cold" {
			t.Fatalf("expected cold with streak=0, got status=%q streak=%d", item.CacheStatus, item.CacheMissStreak)
		}
	})

	t.Run("single miss after cold start does not trigger miss_streak", func(t *testing.T) {
		payload := `{
			"generatorMetadata": [
				{"chatModel": {"usage": {"inputTokens": "30000", "cacheReadTokens": "0", "outputTokens": "500"}}},
				{"chatModel": {"usage": {"inputTokens": "35000", "cacheReadTokens": "0", "outputTokens": "600"}}}
			]
		}`
		item := parseGeneratorMetadata(strings.NewReader(payload))
		if item.CacheMissStreak != 1 || item.CacheMissTokens != 35000 || item.CacheStatus != "healthy" {
			t.Fatalf("unexpected single-miss result: %+v", item)
		}
	})

	t.Run("three consecutive large cache misses trigger miss_streak and sum tokens", func(t *testing.T) {
		payload := `{
			"generatorMetadata": [
				{"chatModel": {"usage": {"inputTokens": "30000", "cacheReadTokens": "0", "outputTokens": "500"}}},
				{"chatModel": {"usage": {"inputTokens": "40000", "cacheReadTokens": "0", "outputTokens": "500"}}},
				{"chatModel": {"usage": {"inputTokens": "45000", "cacheReadTokens": "1000", "outputTokens": "500"}}},
				{"chatModel": {"usage": {"inputTokens": "50000", "cacheReadTokens": "0", "outputTokens": "500"}}}
			]
		}`
		item := parseGeneratorMetadata(strings.NewReader(payload))
		if item.CacheMissStreak != 3 {
			t.Fatalf("expected CacheMissStreak=3, got %d", item.CacheMissStreak)
		}
		if item.CacheMissTokens != 135000 {
			t.Fatalf("expected CacheMissTokens=135000, got %d", item.CacheMissTokens)
		}
		if item.CacheStatus != "miss_streak" {
			t.Fatalf("expected CacheStatus=miss_streak, got %q", item.CacheStatus)
		}
	})

	t.Run("small context never triggers miss_streak", func(t *testing.T) {
		payload := `{
			"generatorMetadata": [
				{"chatModel": {"usage": {"inputTokens": "10000", "cacheReadTokens": "0", "outputTokens": "200"}}},
				{"chatModel": {"usage": {"inputTokens": "12000", "cacheReadTokens": "0", "outputTokens": "200"}}},
				{"chatModel": {"usage": {"inputTokens": "15000", "cacheReadTokens": "0", "outputTokens": "200"}}}
			]
		}`
		item := parseGeneratorMetadata(strings.NewReader(payload))
		if item.CacheMissStreak != 0 || item.CacheStatus != "healthy" {
			t.Fatalf("expected no streak on small context, got %+v", item)
		}
	})

	t.Run("recovery on latest step clears miss_streak", func(t *testing.T) {
		payload := `{
			"generatorMetadata": [
				{"chatModel": {"usage": {"inputTokens": "40000", "cacheReadTokens": "0", "outputTokens": "300"}}},
				{"chatModel": {"usage": {"inputTokens": "45000", "cacheReadTokens": "0", "outputTokens": "300"}}},
				{"chatModel": {"usage": {"inputTokens": "5000", "cacheReadTokens": "45000", "outputTokens": "300"}}}
			]
		}`
		item := parseGeneratorMetadata(strings.NewReader(payload))
		if item.CacheMissStreak != 0 || item.CacheStatus != "healthy" {
			t.Fatalf("expected recovered healthy state, got %+v", item)
		}
		if item.CacheHitRate == nil || *item.CacheHitRate != 90.0 {
			t.Fatalf("expected CacheHitRate=90.0, got %v", item.CacheHitRate)
		}
	})

	t.Run("incremental offset counts index 0 in tail window", func(t *testing.T) {
		payload := `{
			"generatorMetadata": [
				{"chatModel": {"usage": {"inputTokens": "40000", "cacheReadTokens": "0", "outputTokens": "300"}}},
				{"chatModel": {"usage": {"inputTokens": "45000", "cacheReadTokens": "0", "outputTokens": "300"}}}
			]
		}`
		item, _ := parseGeneratorMetadataDetailedWithOffset(strings.NewReader(payload), 5)
		if item.CacheMissStreak != 2 || item.CacheMissTokens != 85000 || item.CacheStatus != "miss_streak" {
			t.Fatalf("expected streak=2 with offset=5, got %+v", item)
		}
	})
}

func TestExtractLastActivityAtAndImmutableCopy(t *testing.T) {
	var s1, s2 TrajectoryStep
	s1.Metadata.CreatedAt = "2026-11-24T01:00:00Z"
	s2.Metadata.CompletedAt = "2026-11-24T02:15:30.123456Z"
	got := extractLastActivityAt([]TrajectoryStep{s1, s2})
	if got != "2026-11-24T02:15:30Z" {
		t.Fatalf("expected 2026-11-24T02:15:30Z, got %q", got)
	}

	orig := &ContextMeterItem{UsedTokens: 50000}
	annotated := withLastActivityAt(orig, got)
	if orig.LastActivityAt != "" {
		t.Fatalf("expected original cached ContextMeterItem untouched, got %q", orig.LastActivityAt)
	}
	if annotated == nil || annotated.LastActivityAt != got {
		t.Fatalf("expected annotated copy with LastActivityAt=%q, got %+v", got, annotated)
	}
}

func TestSanitizeContextMeterForClientShadowVsLive(t *testing.T) {
	hr := 5.0
	raw := &ContextMeterItem{
		UsedTokens:      250000,
		MaxTokens:       512000,
		Percentage:      48.8,
		InputTokens:     240000,
		CacheReadTokens: 12000,
		CacheHitRate:    &hr,
		CacheMissStreak: 3,
		CacheMissTokens: 720000,
		CacheStatus:     "miss_streak",
		LastActivityAt:  "2026-11-24T04:50:00Z",
	}

	// Default shadow mode silences client miss_streak warning without mutating raw or stripping read-only stats
	t.Setenv("CACHE_MISS_ALERT_MODE", "")
	t.Setenv("CACHE_MISS_ALERT_ENABLED", "")
	silenced := sanitizeContextMeterForClient(raw)
	if silenced == raw {
		t.Fatalf("expected immutable shallow copy when sanitizing")
	}
	if silenced.CacheMissStreak != 0 || silenced.CacheMissTokens != 0 || silenced.CacheStatus != "healthy" {
		t.Fatalf("expected miss_streak badge fields silenced in shadow mode, got %+v", silenced)
	}
	if silenced.UsedTokens != 250000 || silenced.CacheHitRate == nil || *silenced.CacheHitRate != 5.0 || silenced.LastActivityAt != raw.LastActivityAt {
		t.Fatalf("expected passive read-only meter fields preserved, got %+v", silenced)
	}
	if raw.CacheMissStreak != 3 || raw.CacheStatus != "miss_streak" {
		t.Fatalf("original cached ContextMeterItem must not be mutated: %+v", raw)
	}

	// Off mode also silences client badge
	t.Setenv("CACHE_MISS_ALERT_MODE", "off")
	offCopy := sanitizeContextMeterForClient(raw)
	if offCopy.CacheMissStreak != 0 || offCopy.CacheStatus != "healthy" {
		t.Fatalf("expected miss_streak badge silenced in off mode, got %+v", offCopy)
	}

	// Explicit live mode passes miss_streak through untouched
	t.Setenv("CACHE_MISS_ALERT_MODE", "live")
	live := sanitizeContextMeterForClient(raw)
	if live.CacheMissStreak != 3 || live.CacheMissTokens != 720000 || live.CacheStatus != "miss_streak" {
		t.Fatalf("expected miss_streak passed through in live mode, got %+v", live)
	}
}

func TestContextMeterSessionUsageAccumulationAndIncrementalMerge(t *testing.T) {
	fullPayload := `{
		"generatorMetadata": [
			{
				"chatModel": {
					"model": "claude-opus-5-5-high",
					"modelDisplayName": "Claude Opus 5.5 (High)",
					"usage": {
						"inputTokens": "10000",
						"cacheCreationTokens": "2000",
						"cacheReadTokens": "48000",
						"outputTokens": "1000",
						"thinkingOutputTokens": "400",
						"responseId": "r-1"
					}
				}
			},
			{
				"chatModel": {
					"model": "claude-opus-5-5-high",
					"modelDisplayName": "Claude Opus 5.5 (High)",
					"usage": {
						"inputTokens": "10000",
						"cacheCreationTokens": "2000",
						"cacheReadTokens": "48000",
						"outputTokens": "1000",
						"thinkingOutputTokens": "400",
						"responseId": "r-1"
					}
				}
			},
			{
				"chatModel": {
					"model": "claude-opus-5-5-high",
					"modelDisplayName": "Claude Opus 5.5 (High)",
					"usage": {
						"inputTokens": "8000",
						"cacheReadTokens": "72000",
						"outputTokens": "2000",
						"thinkingOutputTokens": "600",
						"responseId": "r-2"
					}
				}
			}
		]
	}`

	item, sum := parseGeneratorMetadataDetailedWithOffset(strings.NewReader(fullPayload), 0)
	if item == nil {
		t.Fatal("expected non-nil ContextMeterItem")
	}
	if item.SessionSteps != 2 || item.SessionInputTokens != 20000 || item.SessionCacheReadTokens != 120000 ||
		item.SessionOutputTokens != 3000 || item.SessionThinkingTokens != 1000 || item.SessionTotalTokens != 143000 {
		t.Fatalf("unexpected offset=0 session totals: %+v", item)
	}
	if item.SessionCacheHitRate == nil || *item.SessionCacheHitRate != 85.7 {
		t.Fatalf("expected SessionCacheHitRate=85.7, got %v", item.SessionCacheHitRate)
	}
	// Opus: 20k*5/1e6 ($0.10) + 120k*0.5/1e6 ($0.06) + 3k*25/1e6 ($0.075) = $0.235
	if item.SessionCostUSD < 0.234 || item.SessionCostUSD > 0.236 {
		t.Fatalf("expected SessionCostUSD≈0.235, got %.4f", item.SessionCostUSD)
	}

	// Incremental tail fetch starting at offset=2 re-sending step 2 plus a brand-new step 3
	tailPayload := `{
		"generatorMetadata": [
			{
				"chatModel": {
					"model": "claude-opus-5-5-high",
					"modelDisplayName": "Claude Opus 5.5 (High)",
					"usage": {
						"inputTokens": "8000",
						"cacheReadTokens": "72000",
						"outputTokens": "2000",
						"thinkingOutputTokens": "600",
						"responseId": "r-2"
					}
				}
			},
			{
				"chatModel": {
					"model": "claude-opus-5-5-high",
					"modelDisplayName": "Claude Opus 5.5 (High)",
					"usage": {
						"inputTokens": "5000",
						"cacheReadTokens": "95000",
						"outputTokens": "1500",
						"thinkingOutputTokens": "500",
						"responseId": "r-3"
					}
				}
			}
		]
	}`
	tailItem, tailSum := parseGeneratorMetadataDetailedWithOffset(strings.NewReader(tailPayload), 2)
	if tailItem == nil {
		t.Fatal("expected non-nil tail ContextMeterItem")
	}
	if tailItem.SessionSteps != 0 {
		t.Fatalf("expected offset>0 parse not to emit truncated SessionSteps before merge, got %d", tailItem.SessionSteps)
	}
	merged := mergeSessionStepUsages(sum.stepUsages, 2, tailSum.stepUsages)
	applySessionUsageTotals(tailItem, merged)
	if tailItem.SessionSteps != 3 || tailItem.SessionInputTokens != 25000 || tailItem.SessionCacheReadTokens != 215000 ||
		tailItem.SessionOutputTokens != 4500 || tailItem.SessionThinkingTokens != 1500 {
		t.Fatalf("unexpected merged session totals: %+v", tailItem)
	}

	// Fallback SQLite overlay must not mutate base and must not overwrite existing SessionSteps > 0
	origHook := resolveSessionTokenUsage
	t.Cleanup(func() { resolveSessionTokenUsage = origHook })
	rate := 90.0
	resolveSessionTokenUsage = func(convID string) *cockpit.ConversationSessionUsage {
		if convID != "conv-fallback" {
			return nil
		}
		return &cockpit.ConversationSessionUsage{
			InputTokens:      10000,
			CacheReadTokens:  90000,
			OutputTokens:     2500,
			ThinkingTokens:   700,
			Steps:            4,
			CacheHitRate:     &rate,
			EstimatedCostUSD: 0.18,
		}
	}

	baseEmpty := &ContextMeterItem{UsedTokens: 60000, MaxTokens: 200000, Percentage: 30.0}
	overlaid := overlaySessionTokenUsage("conv-fallback", baseEmpty)
	if baseEmpty.SessionSteps != 0 {
		t.Fatalf("overlaySessionTokenUsage must not mutate base pointer")
	}
	if overlaid == nil || overlaid.SessionSteps != 4 || overlaid.SessionCacheReadTokens != 90000 ||
		overlaid.SessionTotalTokens != 102500 || overlaid.SessionCostUSD != 0.18 ||
		overlaid.SessionCacheHitRate == nil || *overlaid.SessionCacheHitRate != 90.0 {
		t.Fatalf("unexpected overlaid session usage: %+v", overlaid)
	}

	// Already-populated SessionSteps stays untouched
	untouched := overlaySessionTokenUsage("conv-fallback", tailItem)
	if untouched != tailItem || untouched.SessionSteps != 3 {
		t.Fatalf("expected existing session totals preserved, got %+v", untouched)
	}
}
