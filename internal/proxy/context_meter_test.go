package proxy

import (
	"strings"
	"testing"
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
