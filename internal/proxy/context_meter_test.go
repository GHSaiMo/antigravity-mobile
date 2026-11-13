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
