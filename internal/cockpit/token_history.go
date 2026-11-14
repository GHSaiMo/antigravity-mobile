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

type ledgerSession struct {
	Source      string                      `json:"source"`
	Size        int64                       `json:"size"`
	ModTimeNano int64                       `json:"mod_time_nano"`
	Days        map[string]DailyTokenRecord `json:"days"`
}

type tokenHistoryLedger struct {
	Version  int                      `json:"version"`
	Sessions map[string]ledgerSession `json:"sessions"`
}

type transcriptJob struct {
	convID string
	path   string
	source string
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
	historyMu         sync.RWMutex
	cachedHistoryResp *TokenHistoryResponse
	cachedHistoryAt   time.Time
	cachedLedger      *tokenHistoryLedger
	cachedLedgerPath  string
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

// parseModelFromUserSettings extracts the model display name from <USER_SETTINGS_CHANGE> in USER_INPUT steps.
func parseModelFromUserSettings(raw map[string]interface{}) string {
	rawType, _ := raw["type"].(string)
	if rawType != "USER_INPUT" {
		return ""
	}
	content, ok := raw["content"].(string)
	if !ok || !strings.Contains(content, "`Model Selection`") {
		return ""
	}

	startTag := "<USER_SETTINGS_CHANGE>"
	endTag := "</USER_SETTINGS_CHANGE>"
	sIdx := strings.Index(content, startTag)
	if sIdx == -1 {
		return ""
	}
	block := content[sIdx+len(startTag):]
	if eIdx := strings.Index(block, endTag); eIdx != -1 {
		block = block[:eIdx]
	}

	needle := "`Model Selection`"
	mIdx := strings.Index(block, needle)
	if mIdx == -1 {
		return ""
	}
	block = block[mIdx+len(needle):]

	toIdx := strings.Index(block, " to ")
	if toIdx == -1 {
		return ""
	}
	rem := strings.TrimSpace(block[toIdx+len(" to "):])

	for _, delim := range []string{". No need", ".\n", ".\r", "\n", "</"} {
		if idx := strings.Index(rem, delim); idx != -1 {
			rem = rem[:idx]
		}
	}
	rem = strings.TrimSuffix(rem, ".")
	model := strings.TrimSpace(rem)

	// Guard against code injection, excessive length, or malformed strings
	if len(model) == 0 || len(model) > 60 || strings.ContainsAny(model, "{};\"\n\r") {
		return ""
	}
	return model
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
		if m := parseModelFromUserSettings(raw); m != "" {
			currentModel = m
		}

		var inp, outp, capTokens, thinking int64
		var callID, dateKey string

		if createdAt, ok := raw["created_at"].(string); ok && len(createdAt) >= 10 {
			var t time.Time
			var err error
			if t, err = time.Parse(time.RFC3339Nano, createdAt); err == nil {
				dateKey = t.Local().Format("2006-01-02")
			} else if t, err = time.Parse(time.RFC3339, createdAt); err == nil {
				dateKey = t.Local().Format("2006-01-02")
			} else if t, err = time.Parse("2006-01-02T15:04:05Z07:00", createdAt); err == nil {
				dateKey = t.Local().Format("2006-01-02")
			} else if t, err = time.ParseInLocation("2006-01-02T15:04:05", createdAt, time.Local); err == nil {
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

// collectTranscriptJobs scans both desktop and CLI brain directories for transcript.jsonl files.
func collectTranscriptJobs() []transcriptJob {
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

	var jobs []transcriptJob

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
			convID := entry.Name()
			convDir := filepath.Join(dj.dir, convID)
			// 1. Direct standard log path: .system_generated/logs/transcript.jsonl
			stdPath := filepath.Join(convDir, ".system_generated", "logs", "transcript.jsonl")
			if st, err := os.Stat(stdPath); err == nil && !st.IsDir() {
				jobs = append(jobs, transcriptJob{convID: convID, path: stdPath, source: dj.source})
				continue
			}
			// 2. Fallback root transcript.jsonl
			rootPath := filepath.Join(convDir, "transcript.jsonl")
			if st, err := os.Stat(rootPath); err == nil && !st.IsDir() {
				jobs = append(jobs, transcriptJob{convID: convID, path: rootPath, source: dj.source})
			}
		}
	}

	return jobs
}

func tokenLedgerFilePath() (string, error) {
	dataDir, err := GetCockpitDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "token_history_ledger.json"), nil
}

func getOrLoadTokenLedgerLocked(ledgerPath string) *tokenHistoryLedger {
	if cachedLedger != nil && cachedLedgerPath == ledgerPath {
		return cachedLedger
	}

	ledger := &tokenHistoryLedger{
		Version:  1,
		Sessions: make(map[string]ledgerSession),
	}
	if ledgerPath != "" {
		if data, err := os.ReadFile(ledgerPath); err == nil && len(data) > 0 {
			var parsed tokenHistoryLedger
			if err := json.Unmarshal(data, &parsed); err == nil && parsed.Sessions != nil {
				ledger = &parsed
				if ledger.Version == 0 {
					ledger.Version = 1
				}
			}
		}
	}
	cachedLedger = ledger
	cachedLedgerPath = ledgerPath
	return ledger
}

func saveTokenHistoryLedger(ledgerPath string, ledger *tokenHistoryLedger) error {
	if ledgerPath == "" || ledger == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(ledgerPath), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(ledger)
	if err != nil {
		return err
	}
	tmpPath := ledgerPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, ledgerPath)
}

func aggregateRecordsToDays(records []parsedTranscriptRecord) map[string]DailyTokenRecord {
	days := make(map[string]DailyTokenRecord)
	seenCalls := make(map[string]bool)

	for _, rec := range records {
		if rec.callID != "" {
			if seenCalls[rec.callID] {
				continue
			}
			seenCalls[rec.callID] = true
		}

		day, ok := days[rec.dateKey]
		if !ok {
			day = DailyTokenRecord{
				Date:   rec.dateKey,
				Models: make(map[string]int64),
			}
		}

		day.Total += rec.total
		day.Input += rec.input
		day.Output += rec.output
		day.CacheRead += rec.cacheRead
		day.Thinking += rec.thinking
		day.Steps++
		day.CostUSD += CalculateCostUSD(rec.model, rec.input, rec.cacheRead, rec.output)

		modelKey := rec.model
		if modelKey == "" {
			modelKey = "default"
		}
		day.Models[modelKey] += rec.total
		days[rec.dateKey] = day
	}

	return days
}

func upsertSessionFromFile(ledger *tokenHistoryLedger, job transcriptJob, forceReparse bool) bool {
	st, err := os.Stat(job.path)
	if err != nil || st.IsDir() {
		return false
	}

	key := job.source + ":" + job.convID
	modNano := st.ModTime().UnixNano()
	if !forceReparse {
		if existing, ok := ledger.Sessions[key]; ok && existing.Size == st.Size() && existing.ModTimeNano == modNano {
			return false
		}
	}

	records := parseTranscriptFile(job.path, job.source)
	days := aggregateRecordsToDays(records)

	ledger.Sessions[key] = ledgerSession{
		Source:      job.source,
		Size:        st.Size(),
		ModTimeNano: modNano,
		Days:        days,
	}
	return true
}

// ArchiveConversationTokens persists a conversation's token usage into the local ledger
// before its brain directory is deleted, so historical stats remain intact.
func ArchiveConversationTokens(cascadeID string) {
	if strings.TrimSpace(cascadeID) == "" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	convDir := filepath.Join(home, ".gemini", "antigravity", "brain", cascadeID)
	candidates := []string{
		filepath.Join(convDir, ".system_generated", "logs", "transcript.jsonl"),
		filepath.Join(convDir, "transcript.jsonl"),
	}

	var targetPath string
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			targetPath = p
			break
		}
	}
	if targetPath == "" {
		return
	}

	historyMu.Lock()
	defer historyMu.Unlock()

	ledgerPath, err := tokenLedgerFilePath()
	if err != nil {
		return
	}
	ledger := getOrLoadTokenLedgerLocked(ledgerPath)
	if upsertSessionFromFile(ledger, transcriptJob{
		convID: cascadeID,
		path:   targetPath,
		source: "desktop",
	}, false) {
		if err := saveTokenHistoryLedger(ledgerPath, ledger); err != nil {
			slog.Warn("[TokenLedger] failed to save archived session", "cascadeID", cascadeID, "err", err)
		}
		cachedHistoryResp = nil
	}
}

// GetTokenHistory collects and aggregates token history across all live and archived sessions.
func GetTokenHistory(forceRefresh bool) (*TokenHistoryResponse, error) {
	historyMu.Lock()
	defer historyMu.Unlock()

	now := time.Now()
	ledgerPath, _ := tokenLedgerFilePath()
	if !forceRefresh && cachedHistoryResp != nil && cachedLedgerPath == ledgerPath && now.Sub(cachedHistoryAt) < 15*time.Second {
		return cachedHistoryResp, nil
	}

	ledger := getOrLoadTokenLedgerLocked(ledgerPath)
	jobs := collectTranscriptJobs()

	dirty := false
	for _, job := range jobs {
		if upsertSessionFromFile(ledger, job, forceRefresh) {
			dirty = true
		}
	}

	if dirty && ledgerPath != "" {
		if err := saveTokenHistoryLedger(ledgerPath, ledger); err != nil {
			slog.Warn("[TokenLedger] failed to persist token history ledger", "err", err)
		}
	}

	dailyMap := make(map[string]*DailyTokenRecord)
	var totalTokens, desktopTokens, cliTokens int64
	var totalCostUSD float64
	var totalSteps int

	todayKey := now.Format("2006-01-02")
	sevenDaysAgo := now.AddDate(0, 0, -7).Format("2006-01-02")
	var todayTokens, last7dTokens int64

	for _, sess := range ledger.Sessions {
		for dateKey, dayRec := range sess.Days {
			day, ok := dailyMap[dateKey]
			if !ok {
				day = &DailyTokenRecord{
					Date:   dateKey,
					Models: make(map[string]int64),
				}
				dailyMap[dateKey] = day
			}

			day.Total += dayRec.Total
			day.Input += dayRec.Input
			day.Output += dayRec.Output
			day.CacheRead += dayRec.CacheRead
			day.Thinking += dayRec.Thinking
			day.Steps += dayRec.Steps
			day.CostUSD += dayRec.CostUSD
			for modelKey, cnt := range dayRec.Models {
				day.Models[modelKey] += cnt
			}

			totalTokens += dayRec.Total
			totalCostUSD += dayRec.CostUSD
			totalSteps += dayRec.Steps
			if sess.Source == "cli" {
				cliTokens += dayRec.Total
			} else {
				desktopTokens += dayRec.Total
			}

			if dateKey == todayKey {
				todayTokens += dayRec.Total
			}
			if dateKey >= sevenDaysAgo {
				last7dTokens += dayRec.Total
			}
		}
	}

	// Build sorted active days matrix
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
