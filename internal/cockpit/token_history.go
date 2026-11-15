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

const currentTokenHistoryLedgerVersion = 2

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
	return parseTranscriptFileWithConvID(filePath, "", source)
}

func parseTranscriptFileWithConvID(filePath, convID, source string) []parsedTranscriptRecord {
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

	lineIdx := 0
	currentModel := ""
	parentConvID := ""

	for scanner.Scan() {
		line := scanner.Bytes()
		lineIdx++
		if len(line) == 0 {
			continue
		}

		// Quick check if line might contain token counts, model, or subagent sender before full JSON unmarshal
		hasTokenOrModel := bytes.Contains(line, []byte("token")) || bytes.Contains(line, []byte("Model"))
		maybeSubagentHeader := parentConvID == "" && bytes.Contains(line, []byte("SYSTEM_MESSAGE")) && bytes.Contains(line, []byte("sender="))
		if !hasTokenOrModel && !maybeSubagentHeader {
			continue
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		if parentConvID == "" {
			if p := parseParentConvIDFromSystemMessage(raw); p != "" {
				parentConvID = p
			}
		}

		// Check for model selection setting
		if m := parseModelFromUserSettings(raw); m != "" {
			currentModel = m
		}

		var inp, outp, capTokens, thinking int64
		var callID, dateKey string
		stepIndex := -1
		if si, ok := raw["step_index"].(float64); ok {
			stepIndex = int(si)
		}

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
					currentModel = resolveGenMetadataModelName("", mName, mName)
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

	if hasUnresolvedModel(records) && convID != "" {
		resolveMissingRecordModels(records, convID, parentConvID, source)
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
	if rawType, _ := raw["type"].(string); rawType != "SYSTEM_MESSAGE" {
		return ""
	}
	content, ok := raw["content"].(string)
	if !ok {
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
	// Subagent parent senders are UUIDs (e.g. 36 chars); ignore non-UUID task/system senders.
	if len(rem) < 16 || strings.Contains(rem, "/") {
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
	if strings.TrimSpace(convID) == "" {
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

func queryGenMetadataBlobs(dbPath string) ([][]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var out []byte
	var err error
	if sqliteBin, lookErr := exec.LookPath("sqlite3"); lookErr == nil && isExecutableValid(sqliteBin) {
		cmd := exec.CommandContext(ctx, sqliteBin, "-batch", "-noheader", "-cmd", ".timeout 2000", dbPath, "SELECT hex(data) FROM gen_metadata ORDER BY idx;")
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
		pyScript := `import sqlite3, sys
conn = sqlite3.connect('file:' + sys.argv[1] + '?mode=ro', uri=True, timeout=2.0)
for (val,) in conn.execute('SELECT hex(data) FROM gen_metadata ORDER BY idx'):
    if val:
        print(val)
`
		cmd := exec.CommandContext(ctx, pyBin, "-c", pyScript, dbPath)
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

func resolveMissingRecordModels(records []parsedTranscriptRecord, convID, parentConvID, source string) {
	if entries := loadGenMetadataModels(convID, source); len(entries) > 0 {
		for i := range records {
			if records[i].model == "" {
				records[i].model = matchGenMetadataModel(entries, records[i].stepIndex)
			}
		}
	}
	if !hasUnresolvedModel(records) || parentConvID == "" {
		return
	}

	// Fallback if child .db is absent: check parent's gen_metadata when subagent inherited parent's model.
	if parentModel := resolveSubagentModelFromParent(convID, parentConvID, source); parentModel != "" {
		for i := range records {
			if records[i].model == "" {
				records[i].model = parentModel
			}
		}
	}
}

func resolveSubagentModelFromParent(convID, parentConvID, source string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	baseDir := filepath.Join(home, ".gemini", "antigravity", "brain")
	if source == "cli" {
		baseDir = filepath.Join(home, ".gemini", "antigravity-cli", "brain")
	}

	// Check parent transcript_full.jsonl for explicit Model override in invoke_subagent;
	// only inherit parent's active model when Model was omitted or "inherit".
	fullPath := filepath.Join(baseDir, parentConvID, ".system_generated", "logs", "transcript_full.jsonl")
	if data, err := os.ReadFile(fullPath); err == nil {
		lines := bytes.Split(data, []byte("\n"))
		spawnLineIdx := -1
		for i, ln := range lines {
			if bytes.Contains(ln, []byte(convID)) {
				spawnLineIdx = i
				break
			}
		}
		if spawnLineIdx > 0 {
			for i := spawnLineIdx - 1; i >= 0 && i >= spawnLineIdx-5; i-- {
				if !bytes.Contains(lines[i], []byte("invoke_subagent")) {
					continue
				}
				var raw struct {
					ToolCalls []struct {
						Name string                 `json:"name"`
						Args map[string]interface{} `json:"args"`
					} `json:"tool_calls"`
				}
				if json.Unmarshal(lines[i], &raw) == nil {
					for _, tc := range raw.ToolCalls {
						if tc.Name == "invoke_subagent" {
							if m, _ := tc.Args["Model"].(string); m != "" && m != "inherit" {
								switch m {
								case "flash":
									return "Gemini 3.8 Flash"
								case "flash_lite":
									return "Gemini 3.5 Flash Lite"
								default:
									return resolveGenMetadataModelName("", m, m)
								}
							}
						}
					}
				}
				break
			}
		}
	}

	if entries := loadGenMetadataModels(parentConvID, source); len(entries) > 0 {
		return entries[len(entries)-1].modelName
	}
	return ""
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

func sessionHasDefaultModel(sess ledgerSession) bool {
	for _, day := range sess.Days {
		if day.Models["default"] > 0 {
			return true
		}
	}
	return false
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
			needsBackfill := sessionHasDefaultModel(existing) &&
				(ledger.Version < currentTokenHistoryLedgerVersion || conversationDBExists(job.convID, job.source))
			if !needsBackfill {
				return false
			}
		}
	}

	records := parseTranscriptFileWithConvID(job.path, job.convID, job.source)
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
