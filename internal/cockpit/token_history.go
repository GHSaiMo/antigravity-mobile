package cockpit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
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

const currentTokenHistoryLedgerVersion = 3

type parsedTranscriptRecord struct {
	stepIndex int
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
	case strings.Contains(m, "claude"):
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

// isSafeConvID validates conversation/cascade identifiers before joining paths or querying SQLite.
func isSafeConvID(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 128 || strings.HasPrefix(s, "-") || !IsSafeAccountIdentifier(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isAlphaNum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if !isAlphaNum && c != '-' && c != '_' && c != '.' {
			return false
		}
	}
	return true
}

type transcriptChatModelUsageLite struct {
	InputTokens          float64 `json:"inputTokens"`
	OutputTokens         float64 `json:"outputTokens"`
	CacheReadTokens      float64 `json:"cacheReadTokens"`
	ThinkingOutputTokens float64 `json:"thinkingOutputTokens"`
	ResponseID           string  `json:"responseId"`
}

type transcriptChatModelLite struct {
	Model string                        `json:"model"`
	Usage *transcriptChatModelUsageLite `json:"usage"`
}

// transcriptStepLite decodes only token/model metadata fields and intentionally omits
// multi-MB "content", "thinking", and "tool_calls" payloads to avoid heap amplification (P1-3).
type transcriptStepLite struct {
	Type                 string                   `json:"type"`
	StepIndex            *float64                 `json:"step_index"`
	CreatedAt            string                   `json:"created_at"`
	InputTokens          float64                  `json:"input_tokens"`
	OutputTokens         float64                  `json:"output_tokens"`
	CacheReadTokens      float64                  `json:"cache_read_tokens"`
	ThinkingOutputTokens float64                  `json:"thinking_output_tokens"`
	CallID               string                   `json:"callId"`
	ResponseID           string                   `json:"responseId"`
	ChatModel            *transcriptChatModelLite `json:"chatModel"`
}

type transcriptContentEnvelope struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

// parseModelFromUserSettings extracts the model display name from <USER_SETTINGS_CHANGE> in USER_INPUT steps.
func parseModelFromUserSettings(raw map[string]interface{}) string {
	rawType, _ := raw["type"].(string)
	content, _ := raw["content"].(string)
	return parseModelFromUserSettingsText(rawType, content)
}

func parseModelFromUserSettingsText(rawType, content string) string {
	if rawType != "USER_INPUT" {
		return ""
	}
	if !strings.Contains(content, "`Model Selection`") {
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
	return parseTranscriptFileWithConvID(filePath, "", source)
}

func parseTranscriptFileWithConvID(filePath, convID, source string) []parsedTranscriptRecord {
	var cache *tokenHistoryScanCache
	if isSafeConvID(convID) {
		cache = newTokenHistoryScanCache(false)
		cache.registerCandidateConvID(source, convID)
	}
	return parseTranscriptFileCached(filePath, convID, source, cache)
}

func parseTranscriptFileCached(filePath, convID, source string, cache *tokenHistoryScanCache) []parsedTranscriptRecord {
	if convID != "" && !isSafeConvID(convID) {
		return nil
	}
	f, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var records []parsedTranscriptRecord
	scanner := bufio.NewScanner(f)
	// Allow JSONL lines up to 16MB so large turns with embedded tool output or thoughts
	// are never dropped during token history aggregation.
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	currentModel := ""
	parentConvID := ""

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Tight field-name pre-filter before any JSON decoding so prose mentioning "token" or "Model"
		// never unmarshals multi-MB step content.
		hasUsageOrChatModel := bytes.Contains(line, []byte(`"input_tokens"`)) ||
			bytes.Contains(line, []byte(`"output_tokens"`)) ||
			bytes.Contains(line, []byte(`"cache_read_tokens"`)) ||
			bytes.Contains(line, []byte(`"chatModel"`))
		maybeUserSettings := bytes.Contains(line, []byte(`"USER_INPUT"`)) && bytes.Contains(line, []byte("`Model Selection`"))
		maybeSubagentHeader := parentConvID == "" && bytes.Contains(line, []byte(`"SYSTEM_MESSAGE"`)) && bytes.Contains(line, []byte("sender="))
		if !hasUsageOrChatModel && !maybeUserSettings && !maybeSubagentHeader {
			continue
		}

		if maybeSubagentHeader || maybeUserSettings {
			var env transcriptContentEnvelope
			if err := json.Unmarshal(line, &env); err == nil {
				if parentConvID == "" && maybeSubagentHeader {
					if p := parseParentConvIDFromSystemMessageText(env.Type, env.Content); p != "" {
						parentConvID = p
					}
				}
				if maybeUserSettings {
					if m := parseModelFromUserSettingsText(env.Type, env.Content); m != "" && !strings.EqualFold(m, "default") {
						currentModel = m
					}
				}
			}
			if !hasUsageOrChatModel {
				continue
			}
		}

		var step transcriptStepLite
		if err := json.Unmarshal(line, &step); err != nil {
			continue
		}

		var inp, outp, capTokens, thinking int64
		var callID, dateKey string
		stepIndex := -1
		if step.StepIndex != nil {
			stepIndex = int(*step.StepIndex)
		}

		if createdAt := step.CreatedAt; len(createdAt) >= 10 {
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

		inp = int64(step.InputTokens)
		outp = int64(step.OutputTokens)
		capTokens = int64(step.CacheReadTokens)
		thinking = int64(step.ThinkingOutputTokens)

		// Check chatModel metadata (extract model even if top-level token fields are present)
		if cm := step.ChatModel; cm != nil {
			if cm.Model != "" {
				if resolved := resolveGenMetadataModelName("", cm.Model, cm.Model); resolved != "" && !strings.EqualFold(resolved, "default") {
					currentModel = resolved
				}
			}
			if inp == 0 && outp == 0 && capTokens == 0 && cm.Usage != nil {
				inp = int64(cm.Usage.InputTokens)
				outp = int64(cm.Usage.OutputTokens)
				capTokens = int64(cm.Usage.CacheReadTokens)
				thinking = int64(cm.Usage.ThinkingOutputTokens)
				callID = cm.Usage.ResponseID
			}
		}

		if step.CallID != "" {
			callID = step.CallID
		} else if step.ResponseID != "" {
			callID = step.ResponseID
		}

		tot := inp + outp + capTokens
		if tot > 0 && dateKey != "" {
			records = append(records, parsedTranscriptRecord{
				stepIndex: stepIndex,
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

	if currentModel != "" {
		for i := range records {
			if records[i].model == "" {
				records[i].model = currentModel
			}
		}
	}

	if hasUnresolvedModel(records) && convID != "" {
		resolveMissingRecordModels(records, convID, parentConvID, source, cache)
	}

	return records
}

func hasUnresolvedModel(records []parsedTranscriptRecord) bool {
	for _, r := range records {
		if r.model == "" {
			return true
		}
	}
	return false
}

// parseParentConvIDFromSystemMessage extracts sender=<uuid> from subagent SYSTEM_MESSAGE steps.
func parseParentConvIDFromSystemMessage(raw map[string]interface{}) string {
	rawType, _ := raw["type"].(string)
	content, _ := raw["content"].(string)
	return parseParentConvIDFromSystemMessageText(rawType, content)
}

func parseParentConvIDFromSystemMessageText(rawType, content string) string {
	if rawType != "SYSTEM_MESSAGE" || content == "" {
		return ""
	}
	const needle = "sender="
	idx := strings.Index(content, needle)
	if idx == -1 {
		return ""
	}
	rem := content[idx+len(needle):]
	end := strings.IndexAny(rem, " \t\r\n")
	if end != -1 {
		rem = rem[:end]
	}
	rem = strings.TrimSpace(rem)
	// Subagent parent senders are UUIDs/safe IDs (>=16 chars); reject non-UUID or traversal strings.
	if len(rem) < 16 || !isSafeConvID(rem) {
		return ""
	}
	return rem
}

type genMetadataModelEntry struct {
	lastStepIndex int
	modelName     string
}

var knownModelDisplayNames = map[string]string{
	"model_placeholder_m26":     "Claude Opus 4.6 (Thinking)",
	"claude-opus-4-6-thinking":  "Claude Opus 4.6 (Thinking)",
	"model_placeholder_m35":     "Claude Sonnet 4.6 (Thinking)",
	"claude-sonnet-4-6":         "Claude Sonnet 4.6 (Thinking)",
	"model_placeholder_m36":     "Gemini 3.1 Pro (Low)",
	"gemini-3.1-pro-low":        "Gemini 3.1 Pro (Low)",
	"model_placeholder_m37":     "Gemini 3.1 Pro (High)",
	"gemini-3.1-pro-high":       "Gemini 3.1 Pro (High)",
	"model_placeholder_m318":    "Gemini 3.8 Flash (High)",
	"gemini-3.8-flash-high":     "Gemini 3.8 Flash (High)",
	"model_placeholder_m319":    "Gemini 3.8 Flash (Medium)",
	"gemini-3.8-flash-medium":   "Gemini 3.8 Flash (Medium)",
	"model_placeholder_m320":    "Gemini 3.8 Flash (Low)",
	"gemini-3.8-flash-low":      "Gemini 3.8 Flash (Low)",
	"model_placeholder_m322":    "Gemini 3.8 Flash",
	"gemini-3.8-flash-tiered":   "Gemini 3.8 Flash",
	"gemini-3.5-flash-lite":     "Gemini 3.5 Flash Lite",
	"model_placeholder_m400":    "Claude Opus 5.5 (Low)",
	"claude-opus-5-5-low":       "Claude Opus 5.5 (Low)",
	"model_placeholder_m401":    "Claude Opus 5.5 (Medium)",
	"claude-opus-5-5-medium":    "Claude Opus 5.5 (Medium)",
	"model_placeholder_m402":    "Claude Opus 5.5 (High)",
	"claude-opus-5-5-high":      "Claude Opus 5.5 (High)",
	"model_placeholder_m403":    "Claude Sonnet 5.5 (Low)",
	"claude-sonnet-5-5-low":     "Claude Sonnet 5.5 (Low)",
	"model_placeholder_m404":    "Claude Sonnet 5.5 (Medium)",
	"claude-sonnet-5-5-medium":  "Claude Sonnet 5.5 (Medium)",
	"model_placeholder_m405":    "Claude Sonnet 5.5 (High)",
	"claude-sonnet-5-5-high":    "Claude Sonnet 5.5 (High)",
	"model_placeholder_m417":    "Gemini 4 Argon (High)",
	"gemini-4-argon-high":       "Gemini 4 Argon (High)",
	"model_placeholder_m418":    "Gemini 4 Argon (Medium)",
	"gemini-4-argon-medium":     "Gemini 4 Argon (Medium)",
	"model_placeholder_m419":    "Gemini 4 Argon (Low)",
	"gemini-4-argon-low":        "Gemini 4 Argon (Low)",
	"model_placeholder_m420":    "Gemini 4 Carbon (High)",
	"model_placeholder_m427":    "Gemini 4 Carbon (High)",
	"gemini-4-carbon-high":      "Gemini 4 Carbon (High)",
	"model_placeholder_m421":    "Gemini 4 Carbon (Medium)",
	"gemini-4-carbon-medium":    "Gemini 4 Carbon (Medium)",
	"model_placeholder_m422":    "Gemini 4 Carbon (Low)",
	"gemini-4-carbon-low":       "Gemini 4 Carbon (Low)",
	"model_placeholder_m525":    "Gemini 4 Hydrogen (High)",
	"gemini-4-hydrogen-high":    "Gemini 4 Hydrogen (High)",
	"model_placeholder_m526":    "Gemini 4 Hydrogen (Medium)",
	"gemini-4-hydrogen-medium":  "Gemini 4 Hydrogen (Medium)",
	"model_placeholder_m527":    "Gemini 4 Hydrogen (Low)",
	"gemini-4-hydrogen-low":     "Gemini 4 Hydrogen (Low)",
	"model_placeholder_m528":    "Gemini 4 Hydrogen (Extra)",
	"gemini-4-hydrogen-xhigh":   "Gemini 4 Hydrogen (Extra)",
	"model_placeholder_m530":    "Gemini 4 Hydrogen (High)",
	"model_placeholder_m531":    "Gemini 4 Hydrogen (Extra)",
	"model_placeholder_m532":    "Gemini 4 Neon (High)",
	"gemini-4-neon-high":        "Gemini 4 Neon (High)",
	"model_placeholder_m533":    "Gemini 4 Neon (Medium)",
	"gemini-4-neon-medium":      "Gemini 4 Neon (Medium)",
	"model_placeholder_m534":    "Gemini 4 Neon (Low)",
	"gemini-4-neon-low":         "Gemini 4 Neon (Low)",
	"model_placeholder_m535":    "Gemini 4 Neon (Extra)",
	"gemini-4-neon-xhigh":       "Gemini 4 Neon (Extra)",
	"model_placeholder_m537":    "Orchestrator (Neon)",
	"gemini-pro-orchestrator-neon": "Orchestrator (Neon)",
	"model_placeholder_m667":    "Orchestrator (Carbon)",
	"gemini-pro-orchestrator":   "Orchestrator (Carbon)",
	"model_placeholder_m681":    "Orchestrator (Argon)",
	"gemini-pro-orchestrator-argon": "Orchestrator (Argon)",
	"model_placeholder_m669":    "Gemini 4 Hydrogen (Orchestrator)",
	"gemini-flash-worker-agy":   "Gemini 4 Hydrogen (Orchestrator)",
}

func resolveGenMetadataModelName(displayName, slug, modelEnum string) string {
	if dn := strings.TrimSpace(displayName); dn != "" {
		return dn
	}
	if enumKey := strings.ToLower(strings.TrimSpace(modelEnum)); enumKey != "" {
		if mapped, ok := knownModelDisplayNames[enumKey]; ok {
			return mapped
		}
	}
	if slugKey := strings.ToLower(strings.TrimSpace(slug)); slugKey != "" {
		if mapped, ok := knownModelDisplayNames[slugKey]; ok {
			return mapped
		}
		return prettifyModelSlug(slug)
	}
	return strings.TrimSpace(modelEnum)
}

func prettifyModelSlug(slug string) string {
	s := strings.TrimSpace(slug)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "-") {
		return s
	}
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		if strings.EqualFold(p, "gpt") {
			parts[i] = "GPT"
		} else {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

func conversationDBCandidates(convID, source string) []string {
	convID = strings.TrimSpace(convID)
	if !isSafeConvID(convID) {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	desktopDB := filepath.Join(home, ".gemini", "antigravity", "conversations", convID+".db")
	cliDB := filepath.Join(home, ".gemini", "antigravity-cli", "conversations", convID+".db")
	if source == "cli" {
		return []string{cliDB, desktopDB}
	}
	return []string{desktopDB, cliDB}
}

func conversationDBExists(convID, source string) bool {
	for _, p := range conversationDBCandidates(convID, source) {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return true
		}
	}
	return false
}

func loadGenMetadataModels(convID, source string) []genMetadataModelEntry {
	for _, dbPath := range conversationDBCandidates(convID, source) {
		st, err := os.Stat(dbPath)
		if err != nil || st.IsDir() {
			continue
		}
		blobs, err := queryGenMetadataBlobs(dbPath)
		if err != nil || len(blobs) == 0 {
			continue
		}
		var entries []genMetadataModelEntry
		for _, blob := range blobs {
			lastStep, modelName := parseGenMetadataBlob(blob)
			if modelName != "" {
				entries = append(entries, genMetadataModelEntry{
					lastStepIndex: lastStep,
					modelName:     modelName,
				})
			}
		}
		if len(entries) > 0 {
			return entries
		}
	}
	return nil
}

func loadGenMetadataModelsCached(convID, source string, cache *tokenHistoryScanCache) []genMetadataModelEntry {
	if cache == nil {
		return loadGenMetadataModels(convID, source)
	}
	cacheKey := source + ":" + convID
	if entries, ok := cache.genMetaBySession[cacheKey]; ok {
		return entries
	}
	entries := loadGenMetadataModels(convID, source)
	cache.genMetaBySession[cacheKey] = entries
	return entries
}

func queryGenMetadataBlobs(dbPath string) ([][]byte, error) {
	cleanDB, err := safeSQLitePathArg(dbPath)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var out []byte
	if sqliteBin, lookErr := exec.LookPath("sqlite3"); lookErr == nil && isExecutableValid(sqliteBin) {
		cmd := exec.CommandContext(ctx, sqliteBin, "-readonly", "-batch", "-noheader", "-cmd", ".timeout 2000", cleanDB, "SELECT hex(data) FROM gen_metadata ORDER BY idx;")
		out, err = cmd.Output()
	} else {
		var pyBin string
		for _, candidate := range []string{"python3", "python"} {
			if p, pErr := exec.LookPath(candidate); pErr == nil && isExecutableValid(p) {
				pyBin = p
				break
			}
		}
		if pyBin == "" {
			return nil, os.ErrNotExist
		}
		pyScript := `import pathlib, sqlite3, sys
uri = pathlib.Path(sys.argv[1]).resolve().as_uri() + '?mode=ro'
conn = sqlite3.connect(uri, uri=True, timeout=2.0)
try:
    for (val,) in conn.execute('SELECT hex(data) FROM gen_metadata ORDER BY idx'):
        if val:
            print(val)
finally:
    conn.close()
`
		cmd := exec.CommandContext(ctx, pyBin, "-c", pyScript, cleanDB)
		out, err = cmd.Output()
	}
	if err != nil {
		return nil, err
	}

	var blobs [][]byte
	for _, line := range strings.Split(string(out), "\n") {
		h := strings.TrimSpace(line)
		if h == "" {
			continue
		}
		decoded, decErr := hex.DecodeString(h)
		if decErr != nil || len(decoded) == 0 {
			continue
		}
		blobs = append(blobs, decoded)
	}
	return blobs, nil
}

func readProtobufVarint(b []byte, pos int) (uint64, int, bool) {
	var v uint64
	var shift uint
	for pos < len(b) && shift < 64 {
		byt := b[pos]
		pos++
		v |= uint64(byt&0x7f) << shift
		if byt&0x80 == 0 {
			return v, pos, true
		}
		shift += 7
	}
	return 0, pos, false
}

func skipProtobufWireField(b []byte, pos int, wireType int) (int, bool) {
	switch wireType {
	case 0:
		_, next, ok := readProtobufVarint(b, pos)
		return next, ok
	case 1:
		if pos+8 > len(b) {
			return pos, false
		}
		return pos + 8, true
	case 2:
		ln, next, ok := readProtobufVarint(b, pos)
		if !ok || ln > uint64(len(b)-next) {
			return pos, false
		}
		return next + int(ln), true
	case 5:
		if pos+4 > len(b) {
			return pos, false
		}
		return pos + 4, true
	default:
		return pos, false
	}
}

// parseGenMetadataBlob decodes GeneratorMetadata protobuf bytes from conversations/*.db
// to extract last_step_index and the resolved human-readable model name.
func parseGenMetadataBlob(blob []byte) (int, string) {
	pos := 0
	for pos < len(blob) {
		tag, next, ok := readProtobufVarint(blob, pos)
		if !ok {
			break
		}
		pos = next
		fieldNum := int(tag >> 3)
		wireType := int(tag & 7)
		if fieldNum == 1 && wireType == 2 {
			ln, valStart, ok := readProtobufVarint(blob, pos)
			if !ok || ln > uint64(len(blob)-valStart) {
				break
			}
			return parseChatModelMetaFields(blob[valStart : valStart+int(ln)])
		}
		nextPos, ok := skipProtobufWireField(blob, pos, wireType)
		if !ok {
			break
		}
		pos = nextPos
	}
	return -1, ""
}

func parseChatModelMetaFields(msg []byte) (int, string) {
	lastStep := -1
	var slug, displayName, modelEnum string

	pos := 0
	for pos < len(msg) {
		tag, next, ok := readProtobufVarint(msg, pos)
		if !ok {
			break
		}
		pos = next
		fieldNum := int(tag >> 3)
		wireType := int(tag & 7)
		if wireType != 2 {
			nextPos, ok := skipProtobufWireField(msg, pos, wireType)
			if !ok {
				break
			}
			pos = nextPos
			continue
		}
		ln, valStart, ok := readProtobufVarint(msg, pos)
		if !ok || ln > uint64(len(msg)-valStart) {
			break
		}
		chunk := msg[valStart : valStart+int(ln)]
		pos = valStart + int(ln)

		switch fieldNum {
		case 19:
			slug = string(chunk)
		case 21:
			displayName = string(chunk)
		case 20:
			k, v := parseProtobufKV(chunk)
			switch k {
			case "model_enum":
				modelEnum = v
			case "last_step_index":
				if idx, convErr := strconv.Atoi(v); convErr == nil {
					lastStep = idx
				}
			}
		}
	}
	return lastStep, resolveGenMetadataModelName(displayName, slug, modelEnum)
}

func parseProtobufKV(msg []byte) (string, string) {
	var key, val string
	pos := 0
	for pos < len(msg) {
		tag, next, ok := readProtobufVarint(msg, pos)
		if !ok {
			break
		}
		pos = next
		fieldNum := int(tag >> 3)
		wireType := int(tag & 7)
		if wireType != 2 {
			nextPos, ok := skipProtobufWireField(msg, pos, wireType)
			if !ok {
				break
			}
			pos = nextPos
			continue
		}
		ln, valStart, ok := readProtobufVarint(msg, pos)
		if !ok || ln > uint64(len(msg)-valStart) {
			break
		}
		chunk := msg[valStart : valStart+int(ln)]
		pos = valStart + int(ln)
		switch fieldNum {
		case 1:
			key = string(chunk)
		case 2:
			val = string(chunk)
		}
	}
	return key, val
}

func matchGenMetadataModel(entries []genMetadataModelEntry, stepIndex int) string {
	if len(entries) == 0 {
		return ""
	}
	if stepIndex < 0 {
		return entries[0].modelName
	}
	bestIdx := -1
	bestStep := -1
	for i, e := range entries {
		if e.lastStepIndex >= 0 && e.lastStepIndex < stepIndex && e.lastStepIndex >= bestStep {
			bestStep = e.lastStepIndex
			bestIdx = i
		}
	}
	if bestIdx >= 0 {
		return entries[bestIdx].modelName
	}
	return entries[0].modelName
}

func backfillRecordsFromSiblings(records []parsedTranscriptRecord) {
	firstModel := ""
	for i := range records {
		if m := strings.TrimSpace(records[i].model); m != "" && !strings.EqualFold(m, "default") {
			firstModel = m
			break
		}
	}
	if firstModel == "" {
		return
	}
	lastSeen := firstModel
	for i := range records {
		m := strings.TrimSpace(records[i].model)
		if m == "" || strings.EqualFold(m, "default") {
			records[i].model = lastSeen
		} else {
			lastSeen = m
		}
	}
}

type parentTranscriptSummary struct {
	childOverrides map[string]string
	childSeen      map[string]bool
	fallbackModel  string
}

type tokenHistoryScanCache struct {
	batchParentDB          bool
	genMetaBySession       map[string][]genMetadataModelEntry
	parentsLoaded          map[string]bool
	parentByChild          map[string]map[string]string
	parentSummaries        map[string]*parentTranscriptSummary
	parentTranscriptModels map[string]string
	candidateConvIDs       map[string][]string
	candidateConvIDSeen    map[string]bool
}

func newTokenHistoryScanCache(batchParentDB bool) *tokenHistoryScanCache {
	return &tokenHistoryScanCache{
		batchParentDB:          batchParentDB,
		genMetaBySession:       make(map[string][]genMetadataModelEntry),
		parentsLoaded:          make(map[string]bool),
		parentByChild:          make(map[string]map[string]string),
		parentSummaries:        make(map[string]*parentTranscriptSummary),
		parentTranscriptModels: make(map[string]string),
		candidateConvIDs:       make(map[string][]string),
		candidateConvIDSeen:    make(map[string]bool),
	}
}

func (c *tokenHistoryScanCache) registerCandidateConvID(source, convID string) {
	if c == nil || !isSafeConvID(convID) {
		return
	}
	if source != "cli" {
		source = "desktop"
	}
	key := source + ":" + convID
	if c.candidateConvIDSeen[key] {
		return
	}
	c.candidateConvIDSeen[key] = true
	c.candidateConvIDs[source] = append(c.candidateConvIDs[source], convID)
}

func resolveMissingRecordModels(records []parsedTranscriptRecord, convID, parentConvID, source string, cache *tokenHistoryScanCache) {
	backfillRecordsFromSiblings(records)
	if !hasUnresolvedModel(records) {
		return
	}

	if entries := loadGenMetadataModelsCached(convID, source, cache); len(entries) > 0 {
		for i := range records {
			if records[i].model == "" {
				if m := matchGenMetadataModel(entries, records[i].stepIndex); m != "" && !strings.EqualFold(m, "default") {
					records[i].model = m
				}
			}
		}
	}
	if !hasUnresolvedModel(records) {
		return
	}

	if parentConvID == "" {
		parentConvID = lookupParentConvIDCached(convID, source, cache)
	}
	if parentConvID == "" {
		return
	}

	// Fallback if child .db is absent: check parent's gen_metadata / transcript when subagent inherited parent's model.
	if parentModel := resolveSubagentModelFromParentCached(convID, parentConvID, source, cache); parentModel != "" && !strings.EqualFold(parentModel, "default") {
		for i := range records {
			if records[i].model == "" {
				records[i].model = parentModel
			}
		}
	}
}

func summariesDBPathForSource(source string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	subDir := "antigravity"
	if source == "cli" {
		subDir = "antigravity-cli"
	}
	dbPath := filepath.Join(home, ".gemini", subDir, "conversation_summaries.db")
	st, err := os.Stat(dbPath)
	if err != nil || st.IsDir() {
		return ""
	}
	return dbPath
}

func lookupParentConvIDCached(convID, source string, cache *tokenHistoryScanCache) string {
	convID = strings.TrimSpace(convID)
	if !isSafeConvID(convID) {
		return ""
	}
	srcKey := "desktop"
	if source == "cli" {
		srcKey = "cli"
	}
	if cache == nil {
		return lookupParentConvIDFromSummariesDB(convID, srcKey)
	}
	if cache.batchParentDB {
		if !cache.parentsLoaded[srcKey] {
			cache.parentByChild[srcKey] = loadAllParentConvIDsFromSummariesDB(srcKey)
			cache.parentsLoaded[srcKey] = true
		}
		return cache.parentByChild[srcKey][convID]
	}
	if m, ok := cache.parentByChild[srcKey]; ok {
		if parent, exists := m[convID]; exists {
			return parent
		}
	} else {
		cache.parentByChild[srcKey] = make(map[string]string)
	}
	parent := lookupParentConvIDFromSummariesDB(convID, srcKey)
	cache.parentByChild[srcKey][convID] = parent
	return parent
}

func loadAllParentConvIDsFromSummariesDB(source string) map[string]string {
	result := make(map[string]string)
	dbPath := summariesDBPathForSource(source)
	if dbPath == "" {
		return result
	}
	cleanDB, err := safeSQLitePathArg(dbPath)
	if err != nil {
		return result
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var out []byte
	if sqliteBin, lookErr := exec.LookPath("sqlite3"); lookErr == nil && isExecutableValid(sqliteBin) {
		sqlStdin := ".timeout 1500\n.mode list\n.separator \"\\t\"\n" +
			"SELECT conversation_id, parent_id FROM conversation_parents WHERE parent_id IS NOT NULL AND parent_id != '';\n" +
			"SELECT conversation_id, parent_conversation_id FROM conversation_summaries WHERE parent_conversation_id IS NOT NULL AND parent_conversation_id != '';\n"
		cmd := exec.CommandContext(ctx, sqliteBin, "-readonly", "-batch", "-noheader", cleanDB)
		cmd.Stdin = strings.NewReader(sqlStdin)
		out, _ = cmd.Output()
	} else {
		for _, candidate := range []string{"python3", "python"} {
			if pyBin, pErr := exec.LookPath(candidate); pErr == nil && isExecutableValid(pyBin) {
				pyScript := `import pathlib, sqlite3, sys
uri = pathlib.Path(sys.argv[1]).resolve().as_uri() + '?mode=ro'
conn = sqlite3.connect(uri, uri=True, timeout=1.5)
try:
    for q in (
        "SELECT conversation_id, parent_id FROM conversation_parents WHERE parent_id IS NOT NULL AND parent_id != ''",
        "SELECT conversation_id, parent_conversation_id FROM conversation_summaries WHERE parent_conversation_id IS NOT NULL AND parent_conversation_id != ''",
    ):
        try:
            for cid, pid in conn.execute(q):
                if cid and pid:
                    print(f"{cid}\t{pid}")
        except sqlite3.Error:
            pass
finally:
    conn.close()
`
				cmd := exec.CommandContext(ctx, pyBin, "-c", pyScript, cleanDB)
				out, _ = cmd.Output()
				break
			}
		}
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		cid, pid, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		cid = strings.TrimSpace(cid)
		pid = strings.TrimSpace(pid)
		if !isSafeConvID(cid) || !isSafeConvID(pid) || cid == pid {
			continue
		}
		// Preserve COALESCE(conversation_parents, conversation_summaries) precedence.
		if _, exists := result[cid]; !exists {
			result[cid] = pid
		}
	}
	return result
}

func lookupParentConvIDFromSummariesDB(convID, source string) string {
	convID = strings.TrimSpace(convID)
	if !isSafeConvID(convID) {
		return ""
	}
	dbPath := summariesDBPathForSource(source)
	if dbPath == "" {
		return ""
	}
	cleanDB, err := safeSQLitePathArg(dbPath)
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var out []byte
	if sqliteBin, lookErr := exec.LookPath("sqlite3"); lookErr == nil && isExecutableValid(sqliteBin) {
		escaped := strings.ReplaceAll(convID, "'", "''")
		sqlStdin := ".timeout 1000\n" +
			"SELECT parent_id FROM conversation_parents WHERE conversation_id = '" + escaped + "' LIMIT 1;\n" +
			"SELECT parent_conversation_id FROM conversation_summaries WHERE conversation_id = '" + escaped + "' LIMIT 1;\n"
		cmd := exec.CommandContext(ctx, sqliteBin, "-readonly", "-batch", "-noheader", cleanDB)
		cmd.Stdin = strings.NewReader(sqlStdin)
		out, _ = cmd.Output()
	} else {
		for _, candidate := range []string{"python3", "python"} {
			if pyBin, pErr := exec.LookPath(candidate); pErr == nil && isExecutableValid(pyBin) {
				pyScript := `import pathlib, sqlite3, sys
uri = pathlib.Path(sys.argv[1]).resolve().as_uri() + '?mode=ro'
conn = sqlite3.connect(uri, uri=True, timeout=1.0)
try:
    for q in (
        "SELECT parent_id FROM conversation_parents WHERE conversation_id = ?1 LIMIT 1",
        "SELECT parent_conversation_id FROM conversation_summaries WHERE conversation_id = ?1 LIMIT 1",
    ):
        try:
            row = conn.execute(q, (sys.argv[2],)).fetchone()
            if row and row[0]:
                print(row[0])
                break
        except sqlite3.Error:
            pass
finally:
    conn.close()
`
				cmd := exec.CommandContext(ctx, pyBin, "-c", pyScript, cleanDB, convID)
				out, _ = cmd.Output()
				break
			}
		}
	}
	for _, line := range strings.Split(string(out), "\n") {
		parent := strings.TrimSpace(line)
		if parent == "" {
			continue
		}
		if !isSafeConvID(parent) || parent == convID {
			return ""
		}
		return parent
	}
	return ""
}

func mapInvokeSubagentOverrideModel(m string) string {
	m = strings.TrimSpace(m)
	if m == "" || m == "inherit" {
		return ""
	}
	switch m {
	case "flash":
		return "Gemini 3.8 Flash"
	case "flash_lite":
		return "Gemini 3.5 Flash Lite"
	default:
		return resolveGenMetadataModelName("", m, m)
	}
}

func parseInvokeSubagentLine(line []byte) (hasInvoke bool, overrideModel string) {
	if !bytes.Contains(line, []byte("invoke_subagent")) || !bytes.Contains(line, []byte(`"tool_calls"`)) {
		return false, ""
	}
	var step struct {
		ToolCalls []struct {
			Name string          `json:"name"`
			Args json.RawMessage `json:"args"`
		} `json:"tool_calls"`
	}
	if json.Unmarshal(line, &step) != nil {
		return false, ""
	}
	for _, tc := range step.ToolCalls {
		if tc.Name != "invoke_subagent" {
			continue
		}
		hasInvoke = true
		if len(tc.Args) == 0 {
			continue
		}
		var args struct {
			Model      string `json:"Model"`
			ModelLower string `json:"model"`
		}
		if json.Unmarshal(tc.Args, &args) == nil {
			m := args.Model
			if m == "" {
				m = args.ModelLower
			}
			if mapped := mapInvokeSubagentOverrideModel(m); mapped != "" {
				return true, mapped
			}
		}
	}
	return hasInvoke, ""
}

// scanConversationIDTokens extracts conversationId / conversation_id string values from raw or
// JSON-escaped JSONL lines without unmarshaling full step payloads.
func scanConversationIDTokens(line []byte) []string {
	var ids []string
	for _, key := range [][]byte{[]byte("conversationId"), []byte("conversation_id")} {
		search := line
		for {
			idx := bytes.Index(search, key)
			if idx < 0 {
				break
			}
			rest := search[idx+len(key):]
			search = rest

			pos := 0
			sawColon := false
			for pos < len(rest) && pos < 16 {
				c := rest[pos]
				if c == ':' {
					sawColon = true
					pos++
					break
				}
				if c == '\\' || c == '"' || c == '\'' || c == ' ' || c == '\t' {
					pos++
					continue
				}
				break
			}
			if !sawColon {
				continue
			}
			for pos < len(rest) && pos < 24 {
				c := rest[pos]
				if c == ' ' || c == '\t' || c == '\\' || c == '"' || c == '\'' {
					pos++
					continue
				}
				break
			}
			start := pos
			for pos < len(rest) && pos-start <= 128 {
				c := rest[pos]
				isAlphaNum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
				if !isAlphaNum && c != '-' && c != '_' && c != '.' {
					break
				}
				pos++
			}
			if pos-start >= 8 {
				candidate := string(rest[start:pos])
				if isSafeConvID(candidate) {
					ids = append(ids, candidate)
				}
			}
		}
	}
	return ids
}

type invokeWindowEntry struct {
	lineIdx       int
	hasInvoke     bool
	overrideModel string
}

// scanParentTranscriptFull streams parent transcript_full.jsonl once via bufio.Scanner (never os.ReadFile)
// and memoizes explicit invoke_subagent Model overrides for all spawned sibling subagents (P1-3).
func scanParentTranscriptFull(fullPath string, candidates []string) *parentTranscriptSummary {
	summary := &parentTranscriptSummary{
		childOverrides: make(map[string]string),
		childSeen:      make(map[string]bool),
	}
	f, err := os.Open(fullPath)
	if err != nil {
		return summary
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var ring [5]invokeWindowEntry
	for i := range ring {
		ring[i].lineIdx = -1
	}
	lastInvokeLine := -100
	lineIdx := -1

	for scanner.Scan() {
		line := scanner.Bytes()
		lineIdx++
		if len(line) == 0 {
			continue
		}

		// Track parent fallback model in the same streaming pass.
		if bytes.Contains(line, []byte(`"USER_INPUT"`)) && bytes.Contains(line, []byte("`Model Selection`")) {
			var env transcriptContentEnvelope
			if json.Unmarshal(line, &env) == nil {
				if m := parseModelFromUserSettingsText(env.Type, env.Content); m != "" && !strings.EqualFold(m, "default") {
					summary.fallbackModel = m
				}
			}
		} else if bytes.Contains(line, []byte(`"chatModel"`)) {
			var cmWrap struct {
				ChatModel *transcriptChatModelLite `json:"chatModel"`
			}
			if json.Unmarshal(line, &cmWrap) == nil && cmWrap.ChatModel != nil && cmWrap.ChatModel.Model != "" {
				if resolved := resolveGenMetadataModelName("", cmWrap.ChatModel.Model, cmWrap.ChatModel.Model); resolved != "" && !strings.EqualFold(resolved, "default") {
					summary.fallbackModel = resolved
				}
			}
		}

		// Match spawned subagent IDs against any invoke_subagent step in [lineIdx-5, lineIdx-1].
		if lineIdx > 0 && lineIdx-lastInvokeLine <= 5 {
			foundInvoke := false
			nearestOverride := ""
			for prevIdx := lineIdx - 1; prevIdx >= 0 && prevIdx >= lineIdx-5; prevIdx-- {
				slot := ring[prevIdx%len(ring)]
				if slot.lineIdx == prevIdx && slot.hasInvoke {
					foundInvoke = true
					nearestOverride = slot.overrideModel
					break
				}
			}
			if foundInvoke {
				matched := scanConversationIDTokens(line)
				if len(matched) == 0 {
					for _, cid := range candidates {
						if !summary.childSeen[cid] && bytes.Contains(line, []byte(cid)) {
							matched = append(matched, cid)
						}
					}
				}
				for _, cid := range matched {
					if summary.childSeen[cid] {
						continue
					}
					summary.childSeen[cid] = true
					if nearestOverride != "" {
						summary.childOverrides[cid] = nearestOverride
					}
				}
			}
		}

		hasInvoke, overrideModel := parseInvokeSubagentLine(line)
		ring[lineIdx%len(ring)] = invokeWindowEntry{
			lineIdx:       lineIdx,
			hasInvoke:     hasInvoke,
			overrideModel: overrideModel,
		}
		if hasInvoke {
			lastInvokeLine = lineIdx
		}
	}
	return summary
}

func resolveSubagentModelFromParent(convID, parentConvID, source string) string {
	cache := newTokenHistoryScanCache(false)
	cache.registerCandidateConvID(source, convID)
	return resolveSubagentModelFromParentCached(convID, parentConvID, source, cache)
}

func resolveSubagentModelFromParentCached(convID, parentConvID, source string, cache *tokenHistoryScanCache) string {
	convID = strings.TrimSpace(convID)
	parentConvID = strings.TrimSpace(parentConvID)
	if !isSafeConvID(convID) || !isSafeConvID(parentConvID) {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	srcKey := "desktop"
	baseDir := filepath.Join(home, ".gemini", "antigravity", "brain")
	if source == "cli" {
		srcKey = "cli"
		baseDir = filepath.Join(home, ".gemini", "antigravity-cli", "brain")
	}

	parentKey := srcKey + ":" + parentConvID
	var summary *parentTranscriptSummary
	if cache != nil {
		cache.registerCandidateConvID(srcKey, convID)
		summary = cache.parentSummaries[parentKey]
	}
	if summary == nil {
		fullPath := filepath.Join(baseDir, parentConvID, ".system_generated", "logs", "transcript_full.jsonl")
		var candidates []string
		if cache != nil {
			candidates = cache.candidateConvIDs[srcKey]
		} else {
			candidates = []string{convID}
		}
		summary = scanParentTranscriptFull(fullPath, candidates)
		if cache != nil {
			cache.parentSummaries[parentKey] = summary
		}
	}

	// 1. Explicit Model override in parent's invoke_subagent call.
	if override := summary.childOverrides[convID]; override != "" {
		return override
	}

	// 2. Inherited model from parent's conversation DB gen_metadata.
	if entries := loadGenMetadataModelsCached(parentConvID, srcKey, cache); len(entries) > 0 {
		return entries[len(entries)-1].modelName
	}

	// 3. Parent's own transcript.jsonl (memoized per parent within scanCache).
	if cache != nil {
		if m, seen := cache.parentTranscriptModels[parentKey]; seen {
			if m != "" {
				return m
			}
			return summary.fallbackModel
		}
	}
	resolvedParentTranscriptModel := ""
	for _, p := range []string{
		filepath.Join(baseDir, parentConvID, ".system_generated", "logs", "transcript.jsonl"),
		filepath.Join(baseDir, parentConvID, "transcript.jsonl"),
	} {
		parentRecs := parseTranscriptFileCached(p, "", srcKey, cache)
		for i := len(parentRecs) - 1; i >= 0; i-- {
			if m := strings.TrimSpace(parentRecs[i].model); m != "" && !strings.EqualFold(m, "default") {
				resolvedParentTranscriptModel = m
				break
			}
		}
		if resolvedParentTranscriptModel != "" {
			break
		}
	}
	if cache != nil {
		cache.parentTranscriptModels[parentKey] = resolvedParentTranscriptModel
	}
	if resolvedParentTranscriptModel != "" {
		return resolvedParentTranscriptModel
	}
	return summary.fallbackModel
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
			if !isSafeConvID(convID) {
				continue
			}
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
		Version:  currentTokenHistoryLedgerVersion,
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
	data, err := json.Marshal(ledger)
	if err != nil {
		return err
	}
	return writeFileAtomic0600(ledgerPath, data)
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

		modelKey := strings.TrimSpace(rec.model)
		if modelKey == "" || strings.EqualFold(modelKey, "default") {
			// Never emit a synthetic "default" bucket; unattributable turns are omitted from stats.
			continue
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
		day.Models[modelKey] += rec.total
		days[rec.dateKey] = day
	}

	return days
}

func sessionHasDefaultModel(sess ledgerSession) bool {
	for _, day := range sess.Days {
		for k, cnt := range day.Models {
			if strings.EqualFold(k, "default") && cnt > 0 {
				return true
			}
		}
	}
	return false
}

func resolveLedgerSessionFallbackModel(convID, source string, cache *tokenHistoryScanCache) string {
	if !isSafeConvID(convID) {
		return ""
	}
	if entries := loadGenMetadataModelsCached(convID, source, cache); len(entries) > 0 {
		for i := len(entries) - 1; i >= 0; i-- {
			if m := strings.TrimSpace(entries[i].modelName); m != "" && !strings.EqualFold(m, "default") {
				return m
			}
		}
	}
	if parentConvID := lookupParentConvIDCached(convID, source, cache); parentConvID != "" {
		if m := resolveSubagentModelFromParentCached(convID, parentConvID, source, cache); m != "" && !strings.EqualFold(m, "default") {
			return m
		}
	}
	return ""
}

// sanitizeLedgerDefaultEntries reclassifies any legacy "default" entries when a concrete model
// can be recovered from local DB/parent metadata, or drops them if unrecoverable.
func sanitizeLedgerDefaultEntries(ledger *tokenHistoryLedger, cache *tokenHistoryScanCache) bool {
	if ledger == nil || len(ledger.Sessions) == 0 {
		return false
	}
	changed := false
	for key, sess := range ledger.Sessions {
		if !sessionHasDefaultModel(sess) {
			continue
		}
		source, convID, hasSep := strings.Cut(key, ":")
		if !hasSep {
			source = sess.Source
			convID = key
		}
		resolvedModel := resolveLedgerSessionFallbackModel(convID, source, cache)

		for dateKey, day := range sess.Days {
			var defaultCnt int64
			for mKey, cnt := range day.Models {
				if strings.EqualFold(mKey, "default") {
					defaultCnt += cnt
					delete(day.Models, mKey)
					changed = true
				}
			}
			if defaultCnt <= 0 {
				sess.Days[dateKey] = day
				continue
			}
			if resolvedModel != "" {
				if day.Models == nil {
					day.Models = make(map[string]int64)
				}
				day.Models[resolvedModel] += defaultCnt
				sess.Days[dateKey] = day
				continue
			}
			// Unrecoverable default tokens: remove them cleanly so totals match remaining models.
			if len(day.Models) == 0 || day.Total <= defaultCnt {
				delete(sess.Days, dateKey)
				changed = true
				continue
			}
			ratio := float64(day.Total-defaultCnt) / float64(day.Total)
			day.Total -= defaultCnt
			day.Input = int64(float64(day.Input) * ratio)
			day.Output = int64(float64(day.Output) * ratio)
			day.CacheRead = int64(float64(day.CacheRead) * ratio)
			day.Thinking = int64(float64(day.Thinking) * ratio)
			day.CostUSD *= ratio
			sess.Days[dateKey] = day
			changed = true
		}
		if len(sess.Days) == 0 {
			delete(ledger.Sessions, key)
		} else {
			ledger.Sessions[key] = sess
		}
	}
	return changed
}

func upsertSessionFromFile(ledger *tokenHistoryLedger, job transcriptJob, forceReparse bool, cache *tokenHistoryScanCache) bool {
	if !isSafeConvID(job.convID) {
		return false
	}
	st, err := os.Stat(job.path)
	if err != nil || st.IsDir() {
		return false
	}

	key := job.source + ":" + job.convID
	modNano := st.ModTime().UnixNano()
	if !forceReparse {
		if existing, ok := ledger.Sessions[key]; ok && existing.Size == st.Size() && existing.ModTimeNano == modNano {
			needsBackfill := sessionHasDefaultModel(existing) &&
				(ledger.Version < currentTokenHistoryLedgerVersion || conversationDBExists(job.convID, job.source))
			if !needsBackfill {
				return false
			}
		}
	}

	records := parseTranscriptFileCached(job.path, job.convID, job.source, cache)
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
	cascadeID = strings.TrimSpace(cascadeID)
	if !isSafeConvID(cascadeID) {
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
	scanCache := newTokenHistoryScanCache(false)
	scanCache.registerCandidateConvID("desktop", cascadeID)
	if upsertSessionFromFile(ledger, transcriptJob{
		convID: cascadeID,
		path:   targetPath,
		source: "desktop",
	}, false, scanCache) {
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
	scanCache := newTokenHistoryScanCache(true)
	for _, job := range jobs {
		scanCache.registerCandidateConvID(job.source, job.convID)
	}
	for key, sess := range ledger.Sessions {
		src, cid, hasSep := strings.Cut(key, ":")
		if !hasSep {
			src = sess.Source
			cid = key
		}
		scanCache.registerCandidateConvID(src, cid)
	}

	dirty := false
	for _, job := range jobs {
		if upsertSessionFromFile(ledger, job, forceRefresh, scanCache) {
			dirty = true
		}
	}
	if sanitizeLedgerDefaultEntries(ledger, scanCache) {
		dirty = true
	}
	if ledger.Version < currentTokenHistoryLedgerVersion {
		ledger.Version = currentTokenHistoryLedgerVersion
		dirty = true
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
				if strings.EqualFold(modelKey, "default") || cnt <= 0 {
					continue
				}
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
