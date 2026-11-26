package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/i18n"
)

const (
	titleTranslateBatchSize     = 12
	titleTranslateDebounce      = 2 * time.Second
	titleTranslateFailBackoff   = 10 * time.Minute
	titleAutoMaxAge             = 24 * time.Hour
	titleTranslateSingleTimeout = 15 * time.Second
	titleTranslateBatchTimeout  = 25 * time.Second
	maxTranslatedTitleRunes     = 80
)

type titleTranslationRecord struct {
	OriginalTitle   string `json:"originalTitle"`
	TranslatedTitle string `json:"translatedTitle"`
	TargetLang      string `json:"targetLang"`
	UserEdited      bool   `json:"userEdited,omitempty"`
	UpdatedAt       string `json:"updatedAt"`
}

// TitleTranslateBatchState tracks progress/result of an explicit batch translation run.
type TitleTranslateBatchState struct {
	Running        bool   `json:"running"`
	Scanned        int    `json:"scanned"`
	Translated     int    `json:"translated"`
	Skipped        int    `json:"skipped"`
	Failed         int    `json:"failed"`
	TargetLang     string `json:"targetLang,omitempty"`
	TargetLangName string `json:"targetLangName,omitempty"`
	FinishedAt     string `json:"finishedAt,omitempty"`
	Error          string `json:"error,omitempty"`
}

type titleCandidate struct {
	CascadeID     string
	SourceTitle   string
	CurrentTitle  string
	HasUserTitle  bool
	LastModTime   time.Time
	Archived      bool
	OriginalSaved string
}

type titleTranslatorState struct {
	mu           sync.Mutex
	loaded       bool
	ledgerPath   string
	ledger       map[string]titleTranslationRecord
	failBackoff  map[string]time.Time
	pendingAuto  map[string]titleCandidate
	debounceTmr  *time.Timer
	autoRunning  bool
	batchRunning bool
	lastBatch    *TitleTranslateBatchState
}

func titleLedgerFilePath() string {
	return filepath.Join(config.GetDataDir(), "title_translations.json")
}

func (ts *titleTranslatorState) ensureLoadedLocked() {
	want := titleLedgerFilePath()
	if ts.loaded && ts.ledgerPath == want {
		return
	}
	ts.ledgerPath = want
	ts.ledger = make(map[string]titleTranslationRecord)
	if ts.failBackoff == nil {
		ts.failBackoff = make(map[string]time.Time)
	}
	if ts.pendingAuto == nil {
		ts.pendingAuto = make(map[string]titleCandidate)
	}
	ts.loaded = true

	data, err := os.ReadFile(want)
	if err != nil {
		return
	}
	var parsed map[string]titleTranslationRecord
	if err := json.Unmarshal(data, &parsed); err != nil {
		slog.Warn("[TitleTranslate] Failed to parse title_translations.json, starting fresh", "err", err)
		return
	}
	if parsed != nil {
		ts.ledger = parsed
	}
}

func (ts *titleTranslatorState) saveLedgerLocked() {
	if ts.ledgerPath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(ts.ledgerPath), 0700); err != nil {
		return
	}
	data, err := json.MarshalIndent(ts.ledger, "", "  ")
	if err != nil {
		return
	}
	tmp := ts.ledgerPath + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0600); err != nil {
		return
	}
	if err := os.Rename(tmp, ts.ledgerPath); err != nil {
		_ = os.Remove(tmp)
	}
}

func (ts *titleTranslatorState) RestorableCount() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.ensureLoadedLocked()
	count := 0
	for _, rec := range ts.ledger {
		if !rec.UserEdited && rec.OriginalTitle != "" && rec.TranslatedTitle != "" && rec.OriginalTitle != rec.TranslatedTitle {
			count++
		}
	}
	return count
}

func (ts *titleTranslatorState) BatchSnapshot() *TitleTranslateBatchState {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.lastBatch == nil {
		return nil
	}
	cp := *ts.lastBatch
	return &cp
}

// MarkConversationTitleUserEdited records that the user manually renamed cascadeID so
// auto-translation and batch translation will never overwrite their custom title.
func (p *Proxy) MarkConversationTitleUserEdited(cascadeID, customTitle string) {
	if p == nil || strings.TrimSpace(cascadeID) == "" {
		return
	}
	ts := &p.titleTranslator
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.ensureLoadedLocked()
	rec := ts.ledger[cascadeID]
	rec.UserEdited = true
	rec.TranslatedTitle = strings.TrimSpace(customTitle)
	rec.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	ts.ledger[cascadeID] = rec
	delete(ts.pendingAuto, cascadeID)
	ts.saveLedgerLocked()
}

var commonLangEnglishNames = map[string]string{
	"zh":      "Simplified Chinese",
	"zh-cn":   "Simplified Chinese",
	"zh-hans": "Simplified Chinese",
	"zh-sg":   "Simplified Chinese",
	"zh-tw":   "Traditional Chinese",
	"zh-hk":   "Traditional Chinese",
	"zh-mo":   "Traditional Chinese",
	"zh-hant": "Traditional Chinese",
	"ja":      "Japanese",
	"en":      "English",
	"ko":      "Korean",
	"pt":      "Portuguese",
	"es":      "Spanish",
	"fr":      "French",
	"de":      "German",
	"ru":      "Russian",
	"it":      "Italian",
	"nl":      "Dutch",
	"pl":      "Polish",
	"tr":      "Turkish",
	"vi":      "Vietnamese",
	"th":      "Thai",
	"id":      "Indonesian",
	"ms":      "Malay",
	"ar":      "Arabic",
	"he":      "Hebrew",
	"hi":      "Hindi",
	"uk":      "Ukrainian",
	"sv":      "Swedish",
	"da":      "Danish",
	"fi":      "Finnish",
	"no":      "Norwegian",
	"nb":      "Norwegian",
	"cs":      "Czech",
	"el":      "Greek",
	"hu":      "Hungarian",
	"ro":      "Romanian",
}

// normalizeTitleTargetLang maps a BCP-47 locale tag and optional English display name into a clean pair.
func normalizeTitleTargetLang(rawCode, rawName string) (string, string) {
	code := strings.ReplaceAll(strings.TrimSpace(rawCode), "_", "-")
	name := strings.TrimSpace(rawName)
	if code == "" || strings.EqualFold(code, "auto") || strings.EqualFold(code, "system") {
		code = i18n.GetLang()
	}
	lower := strings.ToLower(code)
	switch {
	case strings.HasPrefix(lower, "zh-hant") || strings.HasPrefix(lower, "zh-tw") || strings.HasPrefix(lower, "zh-hk") || strings.HasPrefix(lower, "zh-mo"):
		if name == "" {
			name = "Traditional Chinese"
		}
		return "zh-Hant", name
	case lower == "zh" || strings.HasPrefix(lower, "zh-hans") || strings.HasPrefix(lower, "zh-cn") || strings.HasPrefix(lower, "zh-sg"):
		if name == "" {
			name = "Simplified Chinese"
		}
		return "zh-Hans", name
	case lower == "ja" || strings.HasPrefix(lower, "ja-"):
		if name == "" {
			name = "Japanese"
		}
		return "ja", name
	case lower == "en" || strings.HasPrefix(lower, "en-"):
		return "en", "English"
	}

	if name == "" {
		if n, ok := commonLangEnglishNames[lower]; ok {
			name = n
		} else if idx := strings.IndexByte(lower, '-'); idx > 0 {
			if n, ok := commonLangEnglishNames[lower[:idx]]; ok {
				name = n
			}
		}
	}
	if name == "" {
		name = code
	}
	return code, name
}

func isEnglishLangTag(code string) bool {
	lower := strings.ToLower(strings.TrimSpace(code))
	return lower == "en" || strings.HasPrefix(lower, "en-")
}

func isPlaceholderOrRawPromptTitle(title string) bool {
	t := strings.TrimSpace(title)
	if t == "" || t == "未命名会话" || t == "当前会话" || strings.EqualFold(t, "Untitled") || strings.EqualFold(t, "New Chat") {
		return true
	}
	// Raw unsummarized prompt fragments often start with slash commands/paths or contain filesystem paths/backticks.
	if strings.HasPrefix(t, "/") || strings.Contains(t, "/Users/") || strings.Contains(t, "/home/") || strings.Contains(t, `C:\`) {
		return true
	}
	if utf8.RuneCountInString(t) > 90 {
		return true
	}
	return false
}

// titleNeedsTranslation determines whether title should be sent to the model for targetLang.
// For non-Latin scripts (Chinese, Japanese, Korean, Cyrillic, Arabic, Thai, Hebrew), fast Unicode script
// checks skip titles that are already in the target language with zero model calls.
// For Latin-script languages (Portuguese, Spanish, French, German, etc.), unseen titles are sent to the
// model (with an instruction to return already-localized titles unchanged) and recorded in the ledger.
func titleNeedsTranslation(title, targetLang string) bool {
	t := strings.TrimSpace(title)
	if isPlaceholderOrRawPromptTitle(t) {
		return false
	}

	var hanCount, kanaCount, hangulCount, latinCount, cyrillicCount, arabicCount, thaiCount, hebrewCount int
	for _, r := range t {
		switch {
		case unicode.Is(unicode.Han, r):
			hanCount++
		case (r >= 0x3040 && r <= 0x30ff) || (r >= 0x31f0 && r <= 0x31ff):
			kanaCount++
		case unicode.Is(unicode.Hangul, r):
			hangulCount++
		case unicode.Is(unicode.Cyrillic, r):
			cyrillicCount++
		case unicode.Is(unicode.Arabic, r):
			arabicCount++
		case unicode.Is(unicode.Thai, r):
			thaiCount++
		case unicode.Is(unicode.Hebrew, r):
			hebrewCount++
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			latinCount++
		}
	}

	lower := strings.ToLower(strings.TrimSpace(targetLang))
	baseLang := lower
	if idx := strings.IndexByte(lower, '-'); idx > 0 {
		baseLang = lower[:idx]
	}

	switch baseLang {
	case "en":
		return hanCount+kanaCount+hangulCount+cyrillicCount+arabicCount+thaiCount+hebrewCount > 0
	case "zh":
		// Already Chinese if it contains meaningful Han characters and no Japanese Kana.
		if kanaCount > 0 {
			return true
		}
		if hanCount >= 2 && (latinCount == 0 || hanCount*5 >= (hanCount+latinCount)) {
			return false
		}
		return latinCount >= 3 || hangulCount > 0 || cyrillicCount > 0 || arabicCount > 0 || thaiCount > 0
	case "ja":
		// Japanese titles typically contain Hiragana/Katakana.
		if kanaCount >= 1 {
			return false
		}
		return latinCount >= 3 || hanCount >= 2 || hangulCount > 0 || cyrillicCount > 0
	case "ko":
		if hangulCount >= 2 {
			return false
		}
		return latinCount >= 3 || hanCount >= 2 || kanaCount > 0
	case "ru", "uk", "bg", "sr":
		if cyrillicCount >= 2 {
			return false
		}
		return latinCount >= 3 || hanCount >= 2
	case "ar", "fa":
		if arabicCount >= 2 {
			return false
		}
		return latinCount >= 3 || hanCount >= 2
	case "th":
		if thaiCount >= 2 {
			return false
		}
		return latinCount >= 3 || hanCount >= 2
	case "he", "iw":
		if hebrewCount >= 2 {
			return false
		}
		return latinCount >= 3 || hanCount >= 2
	default:
		// Latin-script languages (pt, es, fr, de, it, nl, vi, id, pl, tr, ...):
		// require at least some alphabetic content; ledger guarantees one-time processing.
		return latinCount >= 3 || hanCount >= 2 || kanaCount > 0 || hangulCount > 0
	}
}

func extractTitleCandidate(id string, s map[string]interface{}, ledger map[string]titleTranslationRecord, targetLang string, includeArchived bool) (titleCandidate, bool) {
	if id == "" || !cascadeIDRe.MatchString(id) || IsDeletedCascade(id) {
		return titleCandidate{}, false
	}
	if isSubagentTrajectoryMap(s, id) || isSideQuestionTrajectoryMap(s, id) {
		return titleCandidate{}, false
	}

	archived := false
	var annTitle string
	if ann, ok := s["annotations"].(map[string]interface{}); ok && ann != nil {
		if a, ok := ann["archived"].(bool); ok {
			archived = a
		}
		if t, ok := ann["title"].(string); ok {
			annTitle = strings.TrimSpace(t)
		}
	}
	if !archived {
		if diskArch, found := readAnnotationArchived(id); found && diskArch {
			archived = true
		}
	}
	if archived && !includeArchived {
		return titleCandidate{}, false
	}

	summaryTitle := ""
	if sm, ok := s["summary"].(string); ok {
		summaryTitle = strings.TrimSpace(sm)
	}
	effectiveTitle := annTitle
	if effectiveTitle == "" {
		effectiveTitle = summaryTitle
	}
	if isPlaceholderOrRawPromptTitle(effectiveTitle) {
		return titleCandidate{}, false
	}

	rec, hasRec := ledger[id]
	if hasRec && rec.UserEdited {
		return titleCandidate{}, false
	}

	// Detect pre-existing manual rename: upstream annotations.title differs from upstream summary,
	// AND does not match a translation we previously wrote.
	if annTitle != "" && summaryTitle != "" && annTitle != summaryTitle {
		if !hasRec || rec.TranslatedTitle != annTitle {
			return titleCandidate{}, false
		}
	}

	// Determine the clean original title to translate from (avoid double-translation across language switches).
	sourceTitle := effectiveTitle
	if hasRec && rec.OriginalTitle != "" && rec.TranslatedTitle == effectiveTitle {
		if strings.EqualFold(rec.TargetLang, targetLang) {
			return titleCandidate{}, false // already processed for this target language
		}
		sourceTitle = rec.OriginalTitle
	} else if hasRec && rec.OriginalTitle == effectiveTitle && strings.EqualFold(rec.TargetLang, targetLang) {
		return titleCandidate{}, false // idempotent skip (e.g. proper noun unchanged after translation)
	}

	if !titleNeedsTranslation(sourceTitle, targetLang) {
		return titleCandidate{}, false
	}

	var modTime time.Time
	if rawMod, ok := s["lastModifiedTime"].(string); ok && rawMod != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, rawMod); err == nil {
			modTime = parsed
		} else if parsed, err := time.Parse(time.RFC3339, rawMod); err == nil {
			modTime = parsed
		}
	}

	return titleCandidate{
		CascadeID:     id,
		SourceTitle:   sourceTitle,
		CurrentTitle:  effectiveTitle,
		LastModTime:   modTime,
		Archived:      archived,
		OriginalSaved: rec.OriginalTitle,
	}, true
}

// MaybeAutoTranslateSummaries inspects visible conversation summaries from GetAllCascadeTrajectories
// and asynchronously schedules debounced translation for newly generated recent titles when enabled.
func (p *Proxy) MaybeAutoTranslateSummaries(summaries map[string]map[string]interface{}) {
	if p == nil || len(summaries) == 0 {
		return
	}
	prefs := p.AIAssistPrefs()
	if !prefs.TitleTranslateEnabled {
		return
	}
	targetLang, targetLangName := normalizeTitleTargetLang(prefs.TitleTranslateLang, prefs.TitleTranslateLangName)
	if isEnglishLangTag(targetLang) {
		return
	}
	if !p.IsAIAssistAvailable() {
		return
	}

	now := time.Now()
	cutoff := now.Add(-titleAutoMaxAge)

	ts := &p.titleTranslator
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.ensureLoadedLocked()

	added := 0
	for id, s := range summaries {
		if retryAt, backedOff := ts.failBackoff[id]; backedOff && now.Before(retryAt) {
			continue
		}
		if _, queued := ts.pendingAuto[id]; queued {
			continue
		}
		cand, ok := extractTitleCandidate(id, s, ts.ledger, targetLang, false)
		if !ok {
			continue
		}
		// Auto-translation only touches recent conversations so enabling the toggle never
		// surprises the user by mass-rewriting ancient history (history uses the explicit button).
		if !cand.LastModTime.IsZero() && cand.LastModTime.Before(cutoff) {
			continue
		}
		ts.pendingAuto[id] = cand
		added++
	}

	if added > 0 && !ts.autoRunning {
		if ts.debounceTmr != nil {
			ts.debounceTmr.Stop()
		}
		langCopy, nameCopy := targetLang, targetLangName
		ts.debounceTmr = time.AfterFunc(titleTranslateDebounce, func() {
			p.drainAutoTitleQueue(langCopy, nameCopy)
		})
	}
}

func (p *Proxy) drainAutoTitleQueue(targetLang, targetLangName string) {
	ts := &p.titleTranslator
	ts.mu.Lock()
	if ts.autoRunning || len(ts.pendingAuto) == 0 {
		ts.mu.Unlock()
		return
	}
	if !p.AIAssistPrefs().TitleTranslateEnabled {
		ts.pendingAuto = make(map[string]titleCandidate)
		ts.mu.Unlock()
		return
	}
	ts.autoRunning = true
	batch := make([]titleCandidate, 0, len(ts.pendingAuto))
	for _, c := range ts.pendingAuto {
		batch = append(batch, c)
	}
	ts.pendingAuto = make(map[string]titleCandidate)
	ts.mu.Unlock()

	defer func() {
		ts.mu.Lock()
		ts.autoRunning = false
		hasMore := len(ts.pendingAuto) > 0
		ts.mu.Unlock()
		if hasMore {
			prefs := p.AIAssistPrefs()
			if prefs.TitleTranslateEnabled {
				l, n := normalizeTitleTargetLang(prefs.TitleTranslateLang, prefs.TitleTranslateLangName)
				if !isEnglishLangTag(l) {
					go p.drainAutoTitleQueue(l, n)
				}
			}
		}
	}()

	sort.Slice(batch, func(i, j int) bool {
		return batch[i].LastModTime.After(batch[j].LastModTime)
	})
	if len(batch) > titleTranslateBatchSize {
		batch = batch[:titleTranslateBatchSize]
	}

	ctx, cancel := context.WithTimeout(context.Background(), titleTranslateBatchTimeout)
	defer cancel()

	translated, _, err := p.translateAndApplyCandidates(ctx, batch, targetLang, targetLangName)
	if err != nil {
		slog.Warn("[TitleTranslate] Auto-translate batch failed", "err", err)
		return
	}
	if translated > 0 {
		p.invalidateAllTrajectories()
	}
}

func buildBatchTitleTranslatePrompt(items []titleCandidate, targetLang, targetLangName string) string {
	type promptItem struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	arr := make([]promptItem, len(items))
	for i, it := range items {
		arr[i] = promptItem{
			ID:    strconv.Itoa(i + 1),
			Title: redactSensitiveForAI(it.SourceTitle),
		}
	}
	rawJSON, _ := json.Marshal(arr)
	return fmt.Sprintf(`Translate each conversation title into concise, natural %s (%s).
Rules:
- Keep proper nouns, project names, acronyms, and technical terms as-is (e.g. Antigravity, Multigravity, iOS, Android, Xcode, Git, Docker, RPC, API, CLI).
- If a title is already naturally written in %s, return it unchanged.
- Keep each translated title concise and clean for a chat sidebar list (no trailing period, no quotes).
- Return ONLY a valid JSON object mapping each id string to its translated title. No markdown code fences, no commentary.

Input JSON:
%s`, targetLangName, targetLang, targetLangName, string(rawJSON))
}

func buildSingleTitleTranslatePrompt(title, targetLang, targetLangName string) string {
	return fmt.Sprintf(`Translate this conversation title into concise, natural %s (%s).
Rules:
- Keep proper nouns, project names, acronyms, and technical terms as-is (e.g. Antigravity, Multigravity, iOS, Android, Xcode, Git, RPC, API).
- If the title is already naturally written in %s, return it unchanged.
- Output ONLY the translated title with no quotes, no markdown, no trailing period, and no explanation.

Title: %s`, targetLangName, targetLang, targetLangName, redactSensitiveForAI(title))
}

func cleanTranslatedTitleOutput(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	s = strings.Trim(s, "\"'`「」『』")
	s = strings.TrimRight(s, "。.")
	s = strings.TrimSpace(s)
	s = SanitizeTitle(s)
	if s == "" || s == "未命名会话" || utf8.RuneCountInString(s) > maxTranslatedTitleRunes {
		return ""
	}
	return s
}

func parseBatchTitleJSON(raw string) (map[string]string, error) {
	s := strings.TrimSpace(raw)
	if idx := strings.IndexByte(s, '{'); idx >= 0 {
		if end := strings.LastIndexByte(s, '}'); end > idx {
			s = s[idx : end+1]
		}
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *Proxy) translateAndApplyCandidates(ctx context.Context, candidates []titleCandidate, targetLang, targetLangName string) (translated int, failed int, firstErr error) {
	if len(candidates) == 0 {
		return 0, 0, nil
	}

	results := make(map[int]string, len(candidates))
	if len(candidates) == 1 {
		prompt := buildSingleTitleTranslatePrompt(candidates[0].SourceTitle, targetLang, targetLangName)
		resp, _, err := p.CallLSModelResponse(ctx, prompt, nil)
		if err != nil {
			p.markTitleBackoff(candidates[0].CascadeID)
			return 0, 1, err
		}
		if cleaned := cleanTranslatedTitleOutput(resp); cleaned != "" {
			results[0] = cleaned
		}
	} else {
		prompt := buildBatchTitleTranslatePrompt(candidates, targetLang, targetLangName)
		resp, _, err := p.CallLSModelResponse(ctx, prompt, nil)
		if err != nil {
			for _, c := range candidates {
				p.markTitleBackoff(c.CascadeID)
			}
			return 0, len(candidates), err
		}
		parsed, parseErr := parseBatchTitleJSON(resp)
		if parseErr != nil {
			// Fallback: translate individually when batch JSON was malformed.
			for i, c := range candidates {
				sCtx, cancel := context.WithTimeout(ctx, titleTranslateSingleTimeout)
				oneResp, _, oneErr := p.CallLSModelResponse(sCtx, buildSingleTitleTranslatePrompt(c.SourceTitle, targetLang, targetLangName), nil)
				cancel()
				if oneErr != nil {
					p.markTitleBackoff(c.CascadeID)
					failed++
					if firstErr == nil {
						firstErr = oneErr
					}
					continue
				}
				if cleaned := cleanTranslatedTitleOutput(oneResp); cleaned != "" {
					results[i] = cleaned
				}
			}
		} else {
			for i := range candidates {
				key := strconv.Itoa(i + 1)
				if val, ok := parsed[key]; ok {
					if cleaned := cleanTranslatedTitleOutput(val); cleaned != "" {
						results[i] = cleaned
					}
				}
			}
		}
	}

	for i, c := range candidates {
		newTitle, ok := results[i]
		if !ok || newTitle == "" {
			p.markTitleBackoff(c.CascadeID)
			failed++
			continue
		}
		applied, err := p.applyTranslatedTitle(ctx, c, newTitle, targetLang)
		if err != nil {
			p.markTitleBackoff(c.CascadeID)
			failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if applied {
			translated++
		}
	}
	return translated, failed, firstErr
}

func (p *Proxy) markTitleBackoff(cascadeID string) {
	ts := &p.titleTranslator
	ts.mu.Lock()
	ts.ensureLoadedLocked()
	ts.failBackoff[cascadeID] = time.Now().Add(titleTranslateFailBackoff)
	ts.mu.Unlock()
}

func (p *Proxy) applyTranslatedTitle(ctx context.Context, cand titleCandidate, translatedTitle, targetLang string) (bool, error) {
	ts := &p.titleTranslator
	ts.mu.Lock()
	ts.ensureLoadedLocked()
	if existing, ok := ts.ledger[cand.CascadeID]; ok && existing.UserEdited {
		ts.mu.Unlock()
		return false, nil
	}
	ts.mu.Unlock()

	original := cand.OriginalSaved
	if original == "" {
		original = cand.SourceTitle
	}

	didUpdateTitle := translatedTitle != cand.CurrentTitle && translatedTitle != ""
	if didUpdateTitle {
		if err := p.writeUpstreamConversationTitle(ctx, cand.CascadeID, translatedTitle); err != nil {
			return false, err
		}
	}

	ts.mu.Lock()
	ts.ensureLoadedLocked()
	// Re-check userEdited in case a manual rename raced while the RPC was in-flight.
	if existing, ok := ts.ledger[cand.CascadeID]; !ok || !existing.UserEdited {
		ts.ledger[cand.CascadeID] = titleTranslationRecord{
			OriginalTitle:   original,
			TranslatedTitle: translatedTitle,
			TargetLang:      targetLang,
			UserEdited:      false,
			UpdatedAt:       time.Now().UTC().Format(time.RFC3339),
		}
		delete(ts.failBackoff, cand.CascadeID)
		ts.saveLedgerLocked()
	}
	ts.mu.Unlock()
	return didUpdateTitle, nil
}

func (p *Proxy) writeUpstreamConversationTitle(ctx context.Context, cascadeID, title string) error {
	title = strings.TrimSpace(title)
	if cascadeID == "" || title == "" || title == "未命名会话" {
		return fmt.Errorf("invalid cascadeId or title")
	}
	if p.titleWriteHook != nil {
		return p.titleWriteHook(ctx, cascadeID, title)
	}
	p.SuppressDesktopFocus(AntiReflectionDuration)
	rpcCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	raw, status, err := p.callUpstreamRPC(rpcCtx, "UpdateConversationAnnotations", map[string]interface{}{
		"cascadeIds":       []string{cascadeID},
		"annotations":      map[string]interface{}{"title": title},
		"mergeAnnotations": true,
	})
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("UpdateConversationAnnotations HTTP %d: %s", status, strings.TrimSpace(string(raw)))
	}
	setCachedCascadeTitle(cascadeID, title)
	writeAnnotationTitle(cascadeID, title)
	return nil
}

func (p *Proxy) fetchRawTrajectorySummariesMap(ctx context.Context) (map[string]map[string]interface{}, error) {
	if p.summariesFetchHook != nil {
		return p.summariesFetchHook(ctx)
	}
	port, token := p.ActiveUpstream()
	if port <= 0 {
		return nil, fmt.Errorf("no active Antigravity upstream")
	}
	raw, err := p.fetchAllTrajectoriesRaw(ctx, port, token)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		TrajectorySummaries map[string]map[string]interface{} `json:"trajectorySummaries"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	if parsed.TrajectorySummaries == nil {
		return map[string]map[string]interface{}{}, nil
	}
	return parsed.TrajectorySummaries, nil
}

// HandleTranslateTitles serves POST /gateway/ai-assist/translate-titles.
// It scans non-archived main conversations, translates titles needing localization in batches of 12,
// and writes translated titles natively back via UpdateConversationAnnotations.
func (p *Proxy) HandleTranslateTitles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TargetLang      string `json:"targetLang,omitempty"`
		TargetLangName  string `json:"targetLangName,omitempty"`
		IncludeArchived bool   `json:"includeArchived,omitempty"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req)
	}

	prefs := p.AIAssistPrefs()
	rawLang := strings.TrimSpace(req.TargetLang)
	rawName := strings.TrimSpace(req.TargetLangName)
	if rawLang == "" {
		rawLang = prefs.TitleTranslateLang
		rawName = prefs.TitleTranslateLangName
	}
	targetLang, targetLangName := normalizeTitleTargetLang(rawLang, rawName)
	if isEnglishLangTag(targetLang) {
		writeJSONError(w, "target language is English; translation not needed", http.StatusBadRequest)
		return
	}

	ts := &p.titleTranslator
	ts.mu.Lock()
	if ts.batchRunning {
		snap := ts.lastBatch
		ts.mu.Unlock()
		writeJSONOK(w, map[string]interface{}{
			"ok":             true,
			"alreadyRunning": true,
			"batchStatus":    snap,
		})
		return
	}
	ts.batchRunning = true
	ts.lastBatch = &TitleTranslateBatchState{
		Running:        true,
		TargetLang:     targetLang,
		TargetLangName: targetLangName,
	}
	ts.mu.Unlock()

	defer func() {
		ts.mu.Lock()
		ts.batchRunning = false
		if ts.lastBatch != nil {
			ts.lastBatch.Running = false
			ts.lastBatch.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		}
		ts.mu.Unlock()
	}()

	summaries, err := p.fetchRawTrajectorySummariesMap(r.Context())
	if err != nil {
		ts.mu.Lock()
		ts.lastBatch.Error = err.Error()
		ts.mu.Unlock()
		writeJSONError(w, "failed to load conversations: "+err.Error(), http.StatusBadGateway)
		return
	}

	ts.mu.Lock()
	ts.ensureLoadedLocked()
	ledgerSnap := make(map[string]titleTranslationRecord, len(ts.ledger))
	for k, v := range ts.ledger {
		ledgerSnap[k] = v
	}
	ts.mu.Unlock()

	var candidates []titleCandidate
	scanned := 0
	skipped := 0
	for id, s := range summaries {
		if IsDeletedCascade(id) || isSubagentTrajectoryMap(s, id) || isSideQuestionTrajectoryMap(s, id) {
			continue
		}
		scanned++
		cand, ok := extractTitleCandidate(id, s, ledgerSnap, targetLang, req.IncludeArchived)
		if !ok {
			skipped++
			continue
		}
		candidates = append(candidates, cand)
	}

	// Process newest conversations first.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].LastModTime.After(candidates[j].LastModTime)
	})

	totalTranslated := 0
	totalFailed := 0
	var lastErr error

	for start := 0; start < len(candidates); start += titleTranslateBatchSize {
		end := start + titleTranslateBatchSize
		if end > len(candidates) {
			end = len(candidates)
		}
		bCtx, cancel := context.WithTimeout(r.Context(), titleTranslateBatchTimeout)
		tr, fl, bErr := p.translateAndApplyCandidates(bCtx, candidates[start:end], targetLang, targetLangName)
		cancel()
		totalTranslated += tr
		totalFailed += fl
		if bErr != nil {
			lastErr = bErr
		}
		ts.mu.Lock()
		ts.lastBatch.Scanned = scanned
		ts.lastBatch.Translated = totalTranslated
		ts.lastBatch.Skipped = skipped
		ts.lastBatch.Failed = totalFailed
		ts.mu.Unlock()
	}

	ts.mu.Lock()
	ts.lastBatch.Scanned = scanned
	ts.lastBatch.Translated = totalTranslated
	ts.lastBatch.Skipped = skipped
	ts.lastBatch.Failed = totalFailed
	if lastErr != nil && totalTranslated == 0 {
		ts.lastBatch.Error = lastErr.Error()
	}
	ts.mu.Unlock()

	if totalTranslated > 0 {
		p.invalidateAllTrajectories()
	}

	resp := map[string]interface{}{
		"ok":              lastErr == nil || totalTranslated > 0,
		"scanned":         scanned,
		"translated":      totalTranslated,
		"skipped":         skipped,
		"failed":          totalFailed,
		"targetLang":      targetLang,
		"targetLangName":  targetLangName,
		"translatedCount": ts.RestorableCount(),
	}
	if lastErr != nil {
		resp["error"] = lastErr.Error()
	}
	writeJSONOK(w, resp)
}

// HandleRestoreTitles serves POST /gateway/ai-assist/restore-titles.
// It restores previously translated (non-user-edited) titles back to their original titles.
func (p *Proxy) HandleRestoreTitles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	ts := &p.titleTranslator
	ts.mu.Lock()
	ts.ensureLoadedLocked()
	targets := make([]struct {
		id       string
		original string
	}, 0, len(ts.ledger))
	for id, rec := range ts.ledger {
		if rec.UserEdited || rec.OriginalTitle == "" || rec.OriginalTitle == rec.TranslatedTitle {
			continue
		}
		if IsDeletedCascade(id) {
			continue
		}
		targets = append(targets, struct {
			id       string
			original string
		}{id: id, original: rec.OriginalTitle})
	}
	ts.mu.Unlock()

	restored := 0
	failed := 0
	var restoredIDs []string
	for _, item := range targets {
		if err := p.writeUpstreamConversationTitle(r.Context(), item.id, item.original); err != nil {
			failed++
			continue
		}
		restored++
		restoredIDs = append(restoredIDs, item.id)
	}

	if len(restoredIDs) > 0 {
		ts.mu.Lock()
		ts.ensureLoadedLocked()
		for _, id := range restoredIDs {
			delete(ts.ledger, id)
		}
		ts.saveLedgerLocked()
		ts.mu.Unlock()
		p.invalidateAllTrajectories()
	}

	writeJSONOK(w, map[string]interface{}{
		"ok":              failed == 0 || restored > 0,
		"restored":        restored,
		"failed":          failed,
		"translatedCount": ts.RestorableCount(),
	})
}
