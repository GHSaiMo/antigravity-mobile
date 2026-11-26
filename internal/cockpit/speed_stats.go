package cockpit

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/config"
)

const (
	modelSpeedRetention     = 7 * 24 * time.Hour
	modelSpeedMaxPerModel   = 200
	modelSpeedFlushDebounce = 20 * time.Second
	modelSpeedFilename      = "model_speed_stats.json"
)

type modelSpeedSample struct {
	TS         int64   `json:"ts"`
	TPS        float64 `json:"tps"`
	TTFT       float64 `json:"ttft,omitempty"`
	ResponseID string  `json:"rid,omitempty"`
}

// ModelSpeedSummary aggregates 7-day median throughput and TTFT for a single model.
type ModelSpeedSummary struct {
	MedianTPS  float64 `json:"median_tps"`
	MedianTTFT float64 `json:"median_ttft_sec"`
	Samples    int     `json:"speed_samples"`
}

type modelSpeedDiskState struct {
	Version int                           `json:"version"`
	Models  map[string][]modelSpeedSample `json:"models"`
}

var (
	speedMu        sync.Mutex
	speedLoaded    bool
	speedStorePath string
	speedByModel   map[string][]modelSpeedSample
	speedLastFlush time.Time
)

func modelSpeedFilePath() string {
	dir := strings.TrimSpace(config.GetDataDir())
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, modelSpeedFilename)
}

// ResetModelSpeedStats clears in-memory speed samples (used in tests).
func ResetModelSpeedStats() {
	speedMu.Lock()
	speedLoaded = false
	speedStorePath = ""
	speedByModel = nil
	speedLastFlush = time.Time{}
	speedMu.Unlock()
}

func ensureSpeedLoadedLocked() {
	want := modelSpeedFilePath()
	if speedLoaded && speedStorePath == want {
		return
	}
	speedLoaded = true
	speedStorePath = want
	speedByModel = make(map[string][]modelSpeedSample)
	if want == "" {
		return
	}
	raw, err := os.ReadFile(want)
	if err != nil {
		return
	}
	var st modelSpeedDiskState
	if err := json.Unmarshal(raw, &st); err != nil || st.Models == nil {
		return
	}
	cutoff := time.Now().Add(-modelSpeedRetention).UnixMilli()
	for model, list := range st.Models {
		kept := pruneSpeedSamples(list, cutoff)
		if len(kept) > 0 {
			speedByModel[model] = kept
		}
	}
}

func pruneSpeedSamples(list []modelSpeedSample, cutoffMs int64) []modelSpeedSample {
	if len(list) == 0 {
		return nil
	}
	out := make([]modelSpeedSample, 0, len(list))
	for _, s := range list {
		if s.TS >= cutoffMs && s.TPS > 0.5 && s.TPS < 2000 {
			out = append(out, s)
		}
	}
	if len(out) > modelSpeedMaxPerModel {
		out = out[len(out)-modelSpeedMaxPerModel:]
	}
	return out
}

func saveSpeedLocked(force bool) {
	if speedStorePath == "" {
		return
	}
	if !force && !speedLastFlush.IsZero() && time.Since(speedLastFlush) < modelSpeedFlushDebounce {
		return
	}
	st := modelSpeedDiskState{
		Version: 1,
		Models:  speedByModel,
	}
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(speedStorePath), 0700)
	if err := writeFileAtomic0600(speedStorePath, b); err == nil {
		speedLastFlush = time.Now()
	}
}

// RecordModelSpeedSample records a validated generation throughput / TTFT sample for a model.
func RecordModelSpeedSample(rawModel string, tps float64, ttftSec float64, responseID string, ts time.Time) {
	if tps <= 0.5 || tps >= 2000 || math.IsNaN(tps) || math.IsInf(tps, 0) {
		return
	}
	if ttftSec < 0 || ttftSec > 300 || math.IsNaN(ttftSec) || math.IsInf(ttftSec, 0) {
		ttftSec = 0
	}
	model := resolveGenMetadataModelName("", rawModel, rawModel)
	model = normalizeModelDisplayName(model)
	if model == "" || strings.EqualFold(model, "default") {
		return
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	tsMs := ts.UnixMilli()
	rid := strings.TrimSpace(responseID)

	speedMu.Lock()
	defer speedMu.Unlock()
	ensureSpeedLoadedLocked()

	cutoff := time.Now().Add(-modelSpeedRetention).UnixMilli()
	list := pruneSpeedSamples(speedByModel[model], cutoff)
	if rid != "" {
		for _, existing := range list {
			if existing.ResponseID == rid {
				return
			}
		}
	} else if len(list) > 0 {
		last := list[len(list)-1]
		// Avoid duplicate polls recording the exact same step every second when responseID is absent.
		if math.Abs(last.TPS-tps) < 0.05 && math.Abs(last.TTFT-ttftSec) < 0.02 && tsMs-last.TS < 15000 {
			return
		}
	}

	list = append(list, modelSpeedSample{
		TS:         tsMs,
		TPS:        math.Round(tps*10) / 10,
		TTFT:       math.Round(ttftSec*100) / 100,
		ResponseID: rid,
	})
	if len(list) > modelSpeedMaxPerModel {
		list = list[len(list)-modelSpeedMaxPerModel:]
	}
	speedByModel[model] = list
	saveSpeedLocked(false)
}

// FlushModelSpeedStats forces persistence of in-memory speed samples to disk.
func FlushModelSpeedStats() {
	speedMu.Lock()
	defer speedMu.Unlock()
	if !speedLoaded {
		return
	}
	saveSpeedLocked(true)
}

func computeFloatMedian(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	cp := append([]float64(nil), vals...)
	sort.Float64s(cp)
	mid := len(cp) / 2
	if len(cp)%2 == 1 {
		return cp[mid]
	}
	return (cp[mid-1] + cp[mid]) / 2.0
}

// SnapshotModelSpeedStats returns 7-day median TPS and TTFT keyed by normalized model display name.
func SnapshotModelSpeedStats() map[string]ModelSpeedSummary {
	speedMu.Lock()
	defer speedMu.Unlock()
	ensureSpeedLoadedLocked()

	cutoff := time.Now().Add(-modelSpeedRetention).UnixMilli()
	out := make(map[string]ModelSpeedSummary, len(speedByModel))
	for model, list := range speedByModel {
		valid := pruneSpeedSamples(list, cutoff)
		if len(valid) == 0 {
			continue
		}
		tpsVals := make([]float64, 0, len(valid))
		ttftVals := make([]float64, 0, len(valid))
		for _, s := range valid {
			tpsVals = append(tpsVals, s.TPS)
			if s.TTFT > 0 {
				ttftVals = append(ttftVals, s.TTFT)
			}
		}
		medTPS := math.Round(computeFloatMedian(tpsVals)*10) / 10
		medTTFT := 0.0
		if len(ttftVals) > 0 {
			medTTFT = math.Round(computeFloatMedian(ttftVals)*100) / 100
		}
		out[model] = ModelSpeedSummary{
			MedianTPS:  medTPS,
			MedianTTFT: medTTFT,
			Samples:    len(valid),
		}
	}
	return out
}
