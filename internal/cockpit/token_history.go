package cockpit

import (
	"bufio"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DailyTokenRecord represents the aggregated token usage and cost for a single calendar day.
type DailyTokenRecord struct {
	Date      string           `json:"date"`
	Total     int64            `json:"total"`
	Input     int64            `json:"input"`
	Output    int64            `json:"output"`
	CacheRead int64            `json:"cache_read"`
	Thinking  int64            `json:"thinking"`
	Steps     int              `json:"steps"`
	CostUSD   float64          `json:"cost_usd"`
	Models    map[string]int64 `json:"models,omitempty"`
}

// TokenHistorySummary represents overall high-level statistics across all sessions.
type TokenHistorySummary struct {
	TotalTokens      int64   `json:"total_tokens"`
	TodayTokens      int64   `json:"today_tokens"`
	Last7dTokens     int64   `json:"last_7d_tokens"`
	DesktopTokens    int64   `json:"desktop_tokens"`
	CLITokens        int64   `json:"cli_tokens"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	EstimatedCostCNY float64 `json:"estimated_cost_cny"`
	ActiveDays       int     `json:"active_days"`
	TotalSteps       int     `json:"total_steps"`
}

// TokenHistoryResponse is the full payload returned to the frontend.
type TokenHistoryResponse struct {
	UpdatedAt   int64              `json:"updated_at"`
	Summary     TokenHistorySummary `json:"summary"`
	DailyMatrix []DailyTokenRecord `json:"daily_matrix"`
}

type fileCacheItem struct {
	size    int64
	modTime time.Time
	records []parsedTranscriptRecord
}

type parsedTranscriptRecord struct {
	dateKey   string
	input     int64
	output    int64
	cacheRead int64
	thinking  int64
	total     int64
	model     string
	source    string // "desktop" or "cli"
	callID    string
}

var (
	historyMu          sync.RWMutex
	cachedHistoryResp  *TokenHistoryResponse
	cachedHistoryAt    time.Time
	transcriptFileMap  = make(map[string]fileCacheItem)
	transcriptFileMu   sync.Mutex
)

// CalculateCostUSD calculates reference pay-as-you-go cost according to standard model rate cards.
func CalculateCostUSD(model string, input, cacheRead, output int64) float64 {
	m := strings.ToLower(model)
	var inRate, cacheRate, outRate float64

	switch {
	case strings.Contains(m, "claude-3-5-sonnet") || strings.Contains(m, "claude-3-7-sonnet") || strings.Contains(m, "claude-opus"):
		inRate = 3.0 / 1e6
		cacheRate = 0.3 / 1e6
		outRate = 15.0 / 1e6
	case strings.Contains(m, "gpt-4o"):
		inRate = 2.5 / 1e6
		cacheRate = 1.25 / 1e6
		outRate = 10.0 / 1e6
	case strings.Contains(m, "flash"):
		inRate = 0.075 / 1e6
		cacheRate = 0.01875 / 1e6
		outRate = 0.30 / 1e6
	case strings.Contains(m, "pro") || strings.Contains(m, "gemini"):
		inRate = 1.25 / 1e6
		cacheRate = 0.3125 / 1e6
		outRate = 5.00 / 1e6
	default:
		// Default blended rate
		inRate = 1.0 / 1e6
		cacheRate = 0.25 / 1e6
		outRate = 4.0 / 1e6
	}

	cost := float64(input)*inRate + float64(cacheRead)*cacheRate + float64(output)*outRate
	return cost
}

func parseTranscriptFile(filePath, source string) []parsedTranscriptRecord {
	f, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var records []parsedTranscriptRecord
	reader := bufio.NewReaderSize(f, 64*1024)

	lineIdx := 0
	currentModel := ""

	for {
		line, isPrefix, err := reader.ReadLine()
		if err != nil {
			if err != io.EOF {
				slog.Debug("[TokenScanner] ReadLine error", "path", filePath, "err", err)
			}
			break
		}
		// If line exceeds 64KB, discard rest of line for memory protection
		if isPrefix {
			for isPrefix && err == nil {
				_, isPrefix, err = reader.ReadLine()
			}
			continue
		}
		lineIdx++
		if len(line) == 0 {
			continue
		}

		// Quick check if line might contain token counts or model before full JSON unmarshal
		if !strings.Contains(string(line), "token") && !strings.Contains(string(line), "Model") {
			continue
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		// Check for model selection setting
		if content, ok := raw["content"].(string); ok && strings.Contains(content, "Model Selection") {
			if idx := strings.Index(content, "Model Selection` from None to "); idx != -1 {
				rem := content[idx+len("Model Selection` from None to "):]
				if endIdx := strings.Index(rem, "."); endIdx != -1 {
					currentModel = strings.TrimSpace(rem[:endIdx])
				}
			}
		}

		var inp, outp, capTokens, thinking int64
		var callID, dateKey string

		if createdAt, ok := raw["created_at"].(string); ok && len(createdAt) >= 10 {
			if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
				dateKey = t.Format("2006-01-02")
			} else {
				dateKey = createdAt[:10]
			}
		}

		if v, ok := raw["input_tokens"].(float64); ok {
			inp = int64(v)
		}
		if v, ok := raw["output_tokens"].(float64); ok {
			outp = int64(v)
		}
		if v, ok := raw["cache_read_tokens"].(float64); ok {
			capTokens = int64(v)
		}
		if v, ok := raw["thinking_output_tokens"].(float64); ok {
			thinking = int64(v)
		}

		// Check chatModel usage
		if inp == 0 && outp == 0 && capTokens == 0 {
			if cm, ok := raw["chatModel"].(map[string]interface{}); ok {
				if mName, ok := cm["model"].(string); ok && mName != "" {
					currentModel = mName
				}
				if u, ok := cm["usage"].(map[string]interface{}); ok {
					if v, ok := u["inputTokens"].(float64); ok {
						inp = int64(v)
					}
					if v, ok := u["outputTokens"].(float64); ok {
						outp = int64(v)
					}
					if v, ok := u["cacheReadTokens"].(float64); ok {
						capTokens = int64(v)
					}
					if v, ok := u["thinkingOutputTokens"].(float64); ok {
						thinking = int64(v)
					}
					if respID, ok := u["responseId"].(string); ok {
						callID = respID
					}
				}
			}
		}

		if cid, ok := raw["callId"].(string); ok && cid != "" {
			callID = cid
		} else if cid, ok := raw["responseId"].(string); ok && cid != "" {
			callID = cid
		}

		tot := inp + outp + capTokens
		if tot > 0 && dateKey != "" {
			records = append(records, parsedTranscriptRecord{
				dateKey:   dateKey,
				input:     inp,
				output:    outp,
				cacheRead: capTokens,
				thinking:  thinking,
				total:     tot,
				model:     currentModel,
				source:    source,
				callID:    callID,
			})
		}
	}

	return records
}

// CollectTranscriptsJobs scans both desktop and CLI brain directories for transcript.jsonl files.
func collectTranscriptJobs() []struct {
	path   string
	source string
} {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	type dirJob struct {
		dir    string
		source string
	}

	dirs := []dirJob{
		{dir: filepath.Join(home, ".gemini", "antigravity", "brain"), source: "desktop"},
		{dir: filepath.Join(home, ".gemini", "antigravity-cli", "brain"), source: "cli"},
	}

	var jobs []struct {
		path   string
		source string
	}

	for _, dj := range dirs {
		if _, err := os.Stat(dj.dir); err != nil {
			continue
		}

		entries, err := os.ReadDir(dj.dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			convDir := filepath.Join(dj.dir, entry.Name())
			// 1. Direct standard log path: .system_generated/logs/transcript.jsonl
			stdPath := filepath.Join(convDir, ".system_generated", "logs", "transcript.jsonl")
			if st, err := os.Stat(stdPath); err == nil && !st.IsDir() {
				jobs = append(jobs, struct {
					path   string
					source string
				}{path: stdPath, source: dj.source})
				continue
			}
			// 2. Fallback root transcript.jsonl
			rootPath := filepath.Join(convDir, "transcript.jsonl")
			if st, err := os.Stat(rootPath); err == nil && !st.IsDir() {
				jobs = append(jobs, struct {
					path   string
					source string
				}{path: rootPath, source: dj.source})
			}
		}
	}

	return jobs
}

// GetTokenHistory collects and aggregates token history across all sessions.
func GetTokenHistory(forceRefresh bool) (*TokenHistoryResponse, error) {
	historyMu.Lock()
	defer historyMu.Unlock()

	now := time.Now()
	if !forceRefresh && cachedHistoryResp != nil && now.Sub(cachedHistoryAt) < 15*time.Second {
		return cachedHistoryResp, nil
	}

	jobs := collectTranscriptJobs()

	transcriptFileMu.Lock()
	defer transcriptFileMu.Unlock()

	dailyMap := make(map[string]*DailyTokenRecord)
	seenCalls := make(map[string]bool)

	var totalTokens, desktopTokens, cliTokens int64
	var totalCostUSD float64
	var totalSteps int

	todayKey := now.Format("2006-01-02")
	sevenDaysAgo := now.AddDate(0, 0, -7).Format("2006-01-02")
	var todayTokens, last7dTokens int64

	for _, job := range jobs {
		st, err := os.Stat(job.path)
		if err != nil {
			continue
		}

		cached, ok := transcriptFileMap[job.path]
		var records []parsedTranscriptRecord
		if ok && cached.size == st.Size() && cached.modTime.Equal(st.ModTime()) {
			records = cached.records
		} else {
			records = parseTranscriptFile(job.path, job.source)
			transcriptFileMap[job.path] = fileCacheItem{
				size:    st.Size(),
				modTime: st.ModTime(),
				records: records,
			}
		}

		for _, rec := range records {
			if rec.callID != "" {
				if seenCalls[rec.callID] {
					continue
				}
				seenCalls[rec.callID] = true
			}

			day, ok := dailyMap[rec.dateKey]
			if !ok {
				day = &DailyTokenRecord{
					Date:   rec.dateKey,
					Models: make(map[string]int64),
				}
				dailyMap[rec.dateKey] = day
			}

			day.Total += rec.total
			day.Input += rec.input
			day.Output += rec.output
			day.CacheRead += rec.cacheRead
			day.Thinking += rec.thinking
			day.Steps++

			cost := CalculateCostUSD(rec.model, rec.input, rec.cacheRead, rec.output)
			day.CostUSD += cost
			totalCostUSD += cost

			modelKey := rec.model
			if modelKey == "" {
				modelKey = "default"
			}
			day.Models[modelKey] += rec.total

			totalTokens += rec.total
			totalSteps++
			if rec.source == "cli" {
				cliTokens += rec.total
			} else {
				desktopTokens += rec.total
			}

			if rec.dateKey == todayKey {
				todayTokens += rec.total
			}
			if rec.dateKey >= sevenDaysAgo {
				last7dTokens += rec.total
			}
		}
	}

	// Build 52-week (365 days) continuous matrix or sorted active days
	var dates []string
	for d := range dailyMap {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	var dailyMatrix []DailyTokenRecord
	for _, d := range dates {
		rec := *dailyMap[d]
		// Round cost to 4 decimal places
		rec.CostUSD = float64(int(rec.CostUSD*10000+0.5)) / 10000
		dailyMatrix = append(dailyMatrix, rec)
	}

	summary := TokenHistorySummary{
		TotalTokens:      totalTokens,
		TodayTokens:      todayTokens,
		Last7dTokens:     last7dTokens,
		DesktopTokens:    desktopTokens,
		CLITokens:        cliTokens,
		EstimatedCostUSD: float64(int(totalCostUSD*100+0.5)) / 100,
		EstimatedCostCNY: float64(int(totalCostUSD*7.25*100+0.5)) / 100,
		ActiveDays:       len(dailyMatrix),
		TotalSteps:       totalSteps,
	}

	cachedHistoryResp = &TokenHistoryResponse{
		UpdatedAt:   now.UnixMilli(),
		Summary:     summary,
		DailyMatrix: dailyMatrix,
	}
	cachedHistoryAt = now

	return cachedHistoryResp, nil
}
