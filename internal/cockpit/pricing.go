package cockpit

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/config"
)

// ModelPriceRate defines reference pay-as-you-go API pricing in USD per 1M tokens.
type ModelPriceRate struct {
	InputPerM     float64 `json:"input_per_m"`
	CacheReadPerM float64 `json:"cache_read_per_m"`
	OutputPerM    float64 `json:"output_per_m"`
}

type dynamicPricingCatalog struct {
	UpdatedAt int64                     `json:"updated_at"`
	Source    string                    `json:"source"`
	Models    map[string]ModelPriceRate `json:"models"`
}

const (
	modelsDevPricingURL       = "https://models.dev/api.json"
	dynamicPricingCacheTTL    = 24 * time.Hour
	dynamicPricingRetryCool   = 10 * time.Minute
	dynamicPricingFilename    = "pricing_catalog_cache.json"
	dynamicPricingHTTPTimeout = 12 * time.Second
)

var (
	pricingMu               sync.RWMutex
	dynamicPricingLoaded    bool
	dynamicPricingCachePath string
	dynamicPricingData      *dynamicPricingCatalog
	dynamicPricingLastTry   time.Time
	dynamicPricingSyncing   bool
	dynamicPricingFetcher   = defaultFetchModelsDevJSON

	lastTierMu   sync.RWMutex
	lastSubTier  string
)

// SetLastKnownSubscriptionTier caches the latest observed subscription tier ("ULTRA", "PRO", "FREE")
// so TokenHistorySummary can compute monthly ROI without changing GetTokenHistory's signature.
func SetLastKnownSubscriptionTier(tier string) {
	norm := strings.ToUpper(strings.TrimSpace(tier))
	if norm == "" {
		return
	}
	lastTierMu.Lock()
	lastSubTier = norm
	lastTierMu.Unlock()
}

// LastKnownSubscriptionTier returns the latest observed subscription tier.
func LastKnownSubscriptionTier() string {
	lastTierMu.RLock()
	defer lastTierMu.RUnlock()
	return lastSubTier
}

// PlanMonthlyUSDForTier returns the canonical display label and standard USD monthly reference price.
func PlanMonthlyUSDForTier(tier string) (string, float64) {
	u := strings.ToUpper(strings.TrimSpace(tier))
	switch {
	case strings.Contains(u, "ULTRA"):
		return "Ultra", 249.99
	case strings.Contains(u, "PRO"):
		return "Pro", 19.99
	case strings.Contains(u, "FREE"):
		return "Free", 0
	default:
		return "", 0
	}
}

func pricingCacheFilePath() string {
	dir := strings.TrimSpace(config.GetDataDir())
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, dynamicPricingFilename)
}

// ResetDynamicPricingCache clears in-memory dynamic pricing state (used by tests).
func ResetDynamicPricingCache() {
	pricingMu.Lock()
	dynamicPricingLoaded = false
	dynamicPricingCachePath = ""
	dynamicPricingData = nil
	dynamicPricingLastTry = time.Time{}
	dynamicPricingSyncing = false
	pricingMu.Unlock()
}

func ensureDynamicPricingLoadedLocked() {
	wantPath := pricingCacheFilePath()
	if dynamicPricingLoaded && dynamicPricingCachePath == wantPath {
		return
	}
	dynamicPricingLoaded = true
	dynamicPricingCachePath = wantPath
	dynamicPricingData = nil
	if wantPath == "" {
		return
	}
	raw, err := os.ReadFile(wantPath)
	if err != nil {
		return
	}
	var cat dynamicPricingCatalog
	if err := json.Unmarshal(raw, &cat); err != nil || len(cat.Models) == 0 {
		return
	}
	dynamicPricingData = &cat
}

func defaultFetchModelsDevJSON(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsDevPricingURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Multigravity-Gateway/pricing-sync")
	client := &http.Client{Timeout: dynamicPricingHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, io.ErrUnexpectedEOF
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20))
}

// ParseModelsDevCatalog extracts normalized per-1M USD rates from a models.dev /api.json payload.
// models.dev schema: { "<provider>": { "models": { "<model_id>": { "cost": { "input": ..., "output": ..., "cache_read": ... } } } } }
func ParseModelsDevCatalog(raw []byte) map[string]ModelPriceRate {
	var providers map[string]struct {
		Models map[string]struct {
			Cost *struct {
				Input     float64 `json:"input"`
				Output    float64 `json:"output"`
				CacheRead float64 `json:"cache_read"`
			} `json:"cost"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &providers); err != nil {
		return nil
	}

	out := make(map[string]ModelPriceRate, 64)
	for provName, prov := range providers {
		pl := strings.ToLower(provName)
		// Prioritize first-party providers so resellers don't overwrite official rates.
		isPrimary := pl == "google" || pl == "anthropic" || pl == "openai" || pl == "google-vertex"
		for modelID, m := range prov.Models {
			if m.Cost == nil || (m.Cost.Input <= 0 && m.Cost.Output <= 0) {
				continue
			}
			key := canonicalPricingKey(modelID)
			if key == "" {
				continue
			}
			cacheRate := m.Cost.CacheRead
			if cacheRate <= 0 && m.Cost.Input > 0 {
				cacheRate = m.Cost.Input * 0.1
			}
			rate := ModelPriceRate{
				InputPerM:     m.Cost.Input,
				CacheReadPerM: cacheRate,
				OutputPerM:    m.Cost.Output,
			}
			if _, exists := out[key]; !exists || isPrimary {
				out[key] = rate
			}
		}
	}
	return out
}

// SyncDynamicPricingCatalog fetches and persists the dynamic pricing catalog from models.dev.
func SyncDynamicPricingCatalog(ctx context.Context) error {
	pricingMu.RLock()
	fetcher := dynamicPricingFetcher
	pricingMu.RUnlock()
	if fetcher == nil {
		return nil
	}
	raw, err := fetcher(ctx)
	if err != nil {
		return err
	}
	models := ParseModelsDevCatalog(raw)
	if len(models) == 0 {
		return nil
	}
	cat := &dynamicPricingCatalog{
		UpdatedAt: time.Now().UnixMilli(),
		Source:    "models.dev",
		Models:    models,
	}

	pricingMu.Lock()
	dynamicPricingData = cat
	dynamicPricingLoaded = true
	path := pricingCacheFilePath()
	dynamicPricingCachePath = path
	pricingMu.Unlock()

	if path != "" {
		if b, mErr := json.Marshal(cat); mErr == nil {
			_ = os.MkdirAll(filepath.Dir(path), 0700)
			_ = writeFileAtomic0600(path, b)
		}
	}
	return nil
}

// MaybeRefreshDynamicPricingAsync kicks off a non-blocking refresh if the cache is missing or > 24h old.
func MaybeRefreshDynamicPricingAsync() {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("MGY_DISABLE_PRICING_SYNC")), "1") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("MGY_DISABLE_PRICING_SYNC")), "true") {
		return
	}
	pricingMu.Lock()
	ensureDynamicPricingLoadedLocked()
	now := time.Now()
	stale := dynamicPricingData == nil ||
		time.Since(time.UnixMilli(dynamicPricingData.UpdatedAt)) >= dynamicPricingCacheTTL
	if !stale || dynamicPricingSyncing || (!dynamicPricingLastTry.IsZero() && now.Sub(dynamicPricingLastTry) < dynamicPricingRetryCool) {
		pricingMu.Unlock()
		return
	}
	dynamicPricingSyncing = true
	dynamicPricingLastTry = now
	pricingMu.Unlock()

	go func() {
		defer func() {
			pricingMu.Lock()
			dynamicPricingSyncing = false
			pricingMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), dynamicPricingHTTPTimeout)
		defer cancel()
		if err := SyncDynamicPricingCatalog(ctx); err != nil {
			slog.Debug("[Pricing] dynamic catalog sync skipped/failed, keeping built-in rates", "err", err)
		}
	}()
}

// canonicalPricingKey normalizes display names, slugs, and placeholder enums to a clean pricing lookup key.
func canonicalPricingKey(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	if mapped, ok := knownModelDisplayNames[lower]; ok {
		lower = strings.ToLower(mapped)
	}
	// Strip provider prefix like "google/" or "anthropic/"
	if idx := strings.LastIndex(lower, "/"); idx >= 0 {
		lower = lower[idx+1:]
	}
	// Strip parenthetical effort/thinking suffixes like "(high)", "(xhigh)", "(thinking)"
	if idx := strings.Index(lower, "("); idx >= 0 {
		// Preserve orchestrator target hint if present: "orchestrator (neon)" -> "orchestrator-neon"
		prefix := strings.TrimSpace(lower[:idx])
		inside := strings.Trim(strings.TrimSpace(lower[idx:]), "()")
		if strings.Contains(prefix, "orchestrator") && inside != "" {
			lower = prefix + "-" + inside
		} else {
			lower = prefix
		}
	}
	replacer := strings.NewReplacer(" ", "-", "_", "-")
	lower = replacer.Replace(lower)
	for strings.Contains(lower, "--") {
		lower = strings.ReplaceAll(lower, "--", "-")
	}
	lower = strings.Trim(lower, "-")
	// Strip trailing effort suffixes
	for _, suf := range []string{"-xhigh", "-extra", "-high", "-medium", "-low", "-thinking", "-tiered", "-preview"} {
		if strings.HasSuffix(lower, suf) && len(lower) > len(suf)+3 {
			lower = strings.TrimSuffix(lower, suf)
		}
	}
	return lower
}

func lookupBuiltInPriceRate(model string) ModelPriceRate {
	k := canonicalPricingKey(model)
	m := strings.ToLower(strings.TrimSpace(model))

	switch {
	// Claude 5.5 / 4.x family
	case strings.Contains(k, "claude") && strings.Contains(k, "opus"):
		return ModelPriceRate{InputPerM: 5.0, CacheReadPerM: 0.5, OutputPerM: 25.0}
	case strings.Contains(k, "claude") && strings.Contains(k, "haiku"):
		return ModelPriceRate{InputPerM: 1.0, CacheReadPerM: 0.1, OutputPerM: 5.0}
	case strings.Contains(k, "claude") || strings.Contains(m, "claude"):
		// Claude Sonnet (3.5 / 4.5 / 4.6 / 5.5)
		return ModelPriceRate{InputPerM: 3.0, CacheReadPerM: 0.3, OutputPerM: 15.0}

	// Gemini 4 family (Antigravity 2.31-2.35+)
	case strings.Contains(k, "neon"):
		return ModelPriceRate{InputPerM: 2.0, CacheReadPerM: 0.2, OutputPerM: 12.0}
	case strings.Contains(k, "carbon"):
		return ModelPriceRate{InputPerM: 2.0, CacheReadPerM: 0.2, OutputPerM: 12.0}
	case strings.Contains(k, "argon"):
		return ModelPriceRate{InputPerM: 1.5, CacheReadPerM: 0.15, OutputPerM: 9.0}
	case strings.Contains(k, "hydrogen"):
		return ModelPriceRate{InputPerM: 0.5, CacheReadPerM: 0.05, OutputPerM: 3.0}

	// Gemini 3.x family
	case strings.Contains(k, "3.8-flash") || strings.Contains(k, "3-8-flash"):
		return ModelPriceRate{InputPerM: 0.5, CacheReadPerM: 0.05, OutputPerM: 3.0}
	case strings.Contains(k, "flash-lite"):
		return ModelPriceRate{InputPerM: 0.10, CacheReadPerM: 0.01, OutputPerM: 0.40}
	case strings.Contains(k, "3.1-pro") || strings.Contains(k, "3-1-pro"):
		return ModelPriceRate{InputPerM: 2.0, CacheReadPerM: 0.2, OutputPerM: 12.0}

	// Gemini 2.5 / legacy Flash & Pro
	case strings.Contains(k, "flash") || strings.Contains(m, "flash"):
		return ModelPriceRate{InputPerM: 0.075, CacheReadPerM: 0.01875, OutputPerM: 0.30}
	case strings.Contains(k, "2.5-pro") || strings.Contains(k, "2-5-pro"):
		return ModelPriceRate{InputPerM: 1.25, CacheReadPerM: 0.3125, OutputPerM: 10.0}
	case strings.Contains(k, "pro") || strings.Contains(k, "gemini") || strings.Contains(m, "gemini"):
		return ModelPriceRate{InputPerM: 1.25, CacheReadPerM: 0.3125, OutputPerM: 5.0}

	// OpenAI / GPT family
	case strings.Contains(k, "gpt-oss"):
		return ModelPriceRate{InputPerM: 0.15, CacheReadPerM: 0.075, OutputPerM: 0.60}
	case strings.Contains(k, "gpt-4o") || strings.Contains(m, "gpt-4o"):
		return ModelPriceRate{InputPerM: 2.5, CacheReadPerM: 1.25, OutputPerM: 10.0}
	case strings.Contains(k, "gpt-6") || strings.Contains(k, "gpt-5"):
		return ModelPriceRate{InputPerM: 2.5, CacheReadPerM: 0.25, OutputPerM: 15.0}

	default:
		return ModelPriceRate{InputPerM: 1.0, CacheReadPerM: 0.25, OutputPerM: 4.0}
	}
}

// ResolveModelPriceRate returns the effective per-1M USD rate for model (dynamic models.dev cache first,
// falling back to the built-in 2026 rate card).
func ResolveModelPriceRate(model string) ModelPriceRate {
	key := canonicalPricingKey(model)
	if key != "" {
		pricingMu.Lock()
		ensureDynamicPricingLoadedLocked()
		cat := dynamicPricingData
		pricingMu.Unlock()

		if cat != nil && len(cat.Models) > 0 {
			if rate, ok := cat.Models[key]; ok && (rate.InputPerM > 0 || rate.OutputPerM > 0) {
				return rate
			}
			// Try dot-vs-dash minor version variant (e.g. claude-opus-5.5 <-> claude-opus-5-5)
			altDash := strings.ReplaceAll(key, ".", "-")
			if rate, ok := cat.Models[altDash]; ok && (rate.InputPerM > 0 || rate.OutputPerM > 0) {
				return rate
			}
			altDot := strings.ReplaceAll(key, "-5-5", "-5.5")
			if rate, ok := cat.Models[altDot]; ok && (rate.InputPerM > 0 || rate.OutputPerM > 0) {
				return rate
			}
		}
	}
	return lookupBuiltInPriceRate(model)
}

// CalculateCostUSD calculates reference pay-as-you-go cost according to dynamic + built-in model rate cards.
func CalculateCostUSD(model string, input, cacheRead, output int64) float64 {
	rate := ResolveModelPriceRate(model)
	cost := (float64(input)*rate.InputPerM +
		float64(cacheRead)*rate.CacheReadPerM +
		float64(output)*rate.OutputPerM) / 1e6
	if math.IsNaN(cost) || math.IsInf(cost, 0) || cost < 0 {
		return 0
	}
	return cost
}
