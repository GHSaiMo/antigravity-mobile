package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"antigravity-mobile/internal/i18n"
)

const (
	titleTranslateBatchSize     = 12
	titleTranslateFailBackoff   = 10 * time.Minute
	titleTranslateSingleTimeout = 15 * time.Second
	titleTranslateBatchTimeout  = 25 * time.Second
	maxTranslatedTitleRunes     = 120
)

var titleTranslateDebounce = 2 * time.Second

type titleCandidate struct {
	CascadeID    string
	SourceTitle  string
	CurrentTitle string
	LastModTime  time.Time
}

type titleTranslatorState struct {
	mu           sync.Mutex
	baselineIDs  map[string]bool // existing cascade IDs snapshot when enabled; never modified
	handled      map[string]bool // cascade IDs already translated or manually renamed
	failBackoff  map[string]time.Time
	pendingAuto  map[string]titleCandidate
	inFlightAuto map[string]bool
	debounceTmr  *time.Timer
	autoRunning  bool
}

func (ts *titleTranslatorState) ensureInitLocked() {
	if ts.baselineIDs == nil {
		ts.baselineIDs = make(map[string]bool)
	}
	if ts.handled == nil {
		ts.handled = make(map[string]bool)
	}
	if ts.failBackoff == nil {
		ts.failBackoff = make(map[string]time.Time)
	}
	if ts.pendingAuto == nil {
		ts.pendingAuto = make(map[string]titleCandidate)
	}
	if ts.inFlightAuto == nil {
		ts.inFlightAuto = make(map[string]bool)
	}
}

// onTitleTranslateEnabled snapshots currently known cascade IDs so previously existing
// conversations are never touched, and only future conversations get auto-translated.
func (p *Proxy) onTitleTranslateEnabled() {
	if p == nil {
		return
	}
	var existingIDs []string
	p.allTrajectories.mu.Lock()
	if p.allTrajectories.parsed != nil && len(p.allTrajectories.parsed.sortedIDs) > 0 {
		existingIDs = append(existingIDs, p.allTrajectories.parsed.sortedIDs...)
	} else if len(p.allTrajectories.entries) > 0 {
		existingIDs = make([]string, 0, len(p.allTrajectories.entries))
		for id := range p.allTrajectories.entries {
			existingIDs = append(existingIDs, id)
		}
	}
	p.allTrajectories.mu.Unlock()

	ts := &p.titleTranslator
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.ensureInitLocked()
	ts.baselineIDs = make(map[string]bool, len(existingIDs))
	for _, id := range existingIDs {
		ts.baselineIDs[id] = true
	}
	ts.pendingAuto = make(map[string]titleCandidate)
	ts.failBackoff = make(map[string]time.Time)
	if ts.debounceTmr != nil {
		ts.debounceTmr.Stop()
		ts.debounceTmr = nil
	}
}

func (p *Proxy) onTitleTranslateDisabled() {
	if p == nil {
		return
	}
	ts := &p.titleTranslator
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.pendingAuto = make(map[string]titleCandidate)
	if ts.debounceTmr != nil {
		ts.debounceTmr.Stop()
		ts.debounceTmr = nil
	}
}

func (p *Proxy) clearTitleTranslateBackoff() {
	if p == nil {
		return
	}
	ts := &p.titleTranslator
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.failBackoff = make(map[string]time.Time)
}

// MarkConversationTitleUserEdited records that the user manually renamed cascadeID so
// auto-translation will never overwrite their custom title.
func (p *Proxy) MarkConversationTitleUserEdited(cascadeID, _ string) {
	if p == nil || strings.TrimSpace(cascadeID) == "" {
		return
	}
	ts := &p.titleTranslator
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.ensureInitLocked()
	ts.handled[cascadeID] = true
	delete(ts.pendingAuto, cascadeID)
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
		code = i18n.GetNativeLocale()
		name = ""
	}
	lower := strings.ToLower(code)
	switch {
	case strings.HasPrefix(lower, "zh-hant") || strings.HasPrefix(lower, "zh-tw") || strings.HasPrefix(lower, "zh-hk") || strings.HasPrefix(lower, "zh-mo"):
		return "zh-Hant", "Traditional Chinese"
	case lower == "zh" || strings.HasPrefix(lower, "zh-hans") || strings.HasPrefix(lower, "zh-cn") || strings.HasPrefix(lower, "zh-sg"):
		return "zh-Hans", "Simplified Chinese"
	case lower == "ja" || strings.HasPrefix(lower, "ja-"):
		return "ja", "Japanese"
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

func parseSummaryTimestamp(raw interface{}) (time.Time, bool) {
	s, ok := raw.(string)
	if !ok {
		return time.Time{}, false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return parsed, true
	}
	if parsed, err := time.Parse(time.RFC3339, s); err == nil {
		return parsed, true
	}
	return time.Time{}, false
}

func isUntitledPlaceholderTitle(title string) bool {
	t := strings.TrimSpace(title)
	if t == "" ||
		t == "未命名会话" ||
		t == "当前会话" ||
		t == "無題のセッション" ||
		t == "現在のセッション" ||
		strings.EqualFold(t, "Untitled") ||
		strings.EqualFold(t, "Untitled Session") ||
		strings.EqualFold(t, "Current Session") ||
		strings.EqualFold(t, "New Chat") {
		return true
	}
	return false
}

func isPlaceholderOrRawPromptTitle(title string) bool {
	t := strings.TrimSpace(title)
	if isUntitledPlaceholderTitle(t) {
		return true
	}
	// Raw unsummarized prompt fragments often start with slash commands/paths or contain filesystem paths.
	if strings.HasPrefix(t, "/") || strings.Contains(t, "/Users/") || strings.Contains(t, "/home/") || strings.Contains(t, `C:\`) {
		return true
	}
	if utf8.RuneCountInString(t) > 90 {
		return true
	}
	return false
}

type scriptCounts struct {
	han      int
	kana     int
	hangul   int
	latin    int
	cyrillic int
	arabic   int
	thai     int
	hebrew   int
}

func countScripts(s string) scriptCounts {
	var c scriptCounts
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r):
			c.han++
		case (r >= 0x3040 && r <= 0x30ff) || (r >= 0x31f0 && r <= 0x31ff):
			c.kana++
		case unicode.Is(unicode.Hangul, r):
			c.hangul++
		case unicode.Is(unicode.Cyrillic, r):
			c.cyrillic++
		case unicode.Is(unicode.Arabic, r):
			c.arabic++
		case unicode.Is(unicode.Thai, r):
			c.thai++
		case unicode.Is(unicode.Hebrew, r):
			c.hebrew++
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			c.latin++
		}
	}
	return c
}

func isTruncatedEcho(source, translated string) bool {
	src := strings.TrimSpace(source)
	tr := strings.TrimSpace(translated)
	if src == "" || tr == "" {
		return false
	}
	for _, suffix := range []string{"...", "…"} {
		if strings.HasSuffix(tr, suffix) && !strings.HasSuffix(src, suffix) {
			prefix := strings.TrimSpace(strings.TrimSuffix(tr, suffix))
			if utf8.RuneCountInString(prefix) >= 8 && strings.HasPrefix(strings.ToLower(src), strings.ToLower(prefix)) {
				return true
			}
		}
	}
	return false
}

func looksLikeEnglishPhrase(s string) bool {
	lower := " " + strings.ToLower(strings.TrimSpace(s)) + " "
	for _, w := range []string{
		" the ", " and ", " with ", " from ", " into ", " for ", " your ",
		" this ", " that ", " without ", " before ", " after ", " during ",
		" when ", " where ", " while ", " across ", " between ", " should ", " must ",
	} {
		if strings.Contains(lower, w) {
			return true
		}
	}
	trimmed := strings.TrimSpace(lower)
	for _, prefix := range []string{
		"updating ", "checking ", "inspecting ", "running ", "fixing ", "adding ",
		"building ", "testing ", "deploying ", "installing ", "refactoring ",
		"debugging ", "searching ", "reading ", "writing ", "creating ", "deleting ",
		"removing ", "verifying ", "analyzing ", "investigating ", "implementing ",
		"optimizing ", "reviewing ", "migrating ", "preparing ", "configuring ",
		"fetching ", "loading ", "syncing ", "waiting ",
		"fix ", "update ", "add ", "check ", "run ", "build ", "verify ", "inspect ",
		"debug ", "refactor ", "deploy ", "install ", "implement ", "optimize ",
		"remove ", "delete ", "create ", "review ", "migrate ", "draft ", "analyze ",
		"investigate ", "prepare ", "configure ",
	} {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

// isTranslationAcceptable verifies that a model translation actually produced text in targetLang
// rather than echoing or truncating the untranslated source text.
func isTranslationAcceptable(source, translated, targetLang string) bool {
	src := strings.TrimSpace(source)
	tr := strings.TrimSpace(translated)
	if tr == "" {
		return false
	}
	if isTruncatedEcho(src, tr) {
		return false
	}

	lower := strings.ToLower(strings.TrimSpace(targetLang))
	baseLang := lower
	if idx := strings.IndexByte(lower, '-'); idx > 0 {
		baseLang = lower[:idx]
	}

	srcC := countScripts(src)
	trC := countScripts(tr)

	switch baseLang {
	case "en":
		nonLatinSrc := srcC.han + srcC.kana + srcC.hangul + srcC.cyrillic + srcC.arabic + srcC.thai + srcC.hebrew
		nonLatinTr := trC.han + trC.kana + trC.hangul + trC.cyrillic + trC.arabic + trC.thai + trC.hebrew
		if nonLatinSrc > 0 && trC.latin == 0 && nonLatinTr > 0 {
			return false
		}
		return true
	case "zh":
		if trC.kana > 0 || trC.han == 0 {
			return false
		}
		if strings.EqualFold(src, tr) && srcC.latin >= 3 {
			return false
		}
		if srcC.han > 0 && trC.han <= srcC.han && trC.latin >= srcC.latin && srcC.latin >= 4 {
			return false
		}
		return true
	case "ja":
		if trC.kana == 0 && trC.han < 2 {
			return false
		}
		if strings.EqualFold(src, tr) && ((srcC.han > 0 && srcC.kana == 0) || (srcC.latin >= 3 && (srcC.kana == 0 || hasCommonEnglishGlueWord(src)))) {
			return false
		}
		if (srcC.kana > 0 || srcC.han > 0) && trC.kana <= srcC.kana && trC.han <= srcC.han && trC.latin >= srcC.latin && srcC.latin >= 4 && hasCommonEnglishGlueWord(src) {
			return false
		}
		if trC.kana >= 1 {
			return true
		}
		// Allow Kanji-only Japanese translations (e.g. "API設計" or Chinese->Japanese kanji conversion) while rejecting echoes.
		return trC.han >= 2 && (srcC.han == 0 || !strings.EqualFold(src, tr))
	case "ko":
		if trC.hangul == 0 {
			return false
		}
		if strings.EqualFold(src, tr) && srcC.latin >= 3 && hasCommonEnglishGlueWord(src) {
			return false
		}
		if srcC.hangul > 0 && trC.hangul <= srcC.hangul && trC.latin >= srcC.latin && srcC.latin >= 4 && hasCommonEnglishGlueWord(src) {
			return false
		}
		return true
	case "ru", "uk", "bg", "sr":
		return trC.cyrillic >= 1
	case "ar", "fa":
		return trC.arabic >= 1
	case "th":
		return trC.thai >= 1
	case "he", "iw":
		return trC.hebrew >= 1
	default:
		// Latin-script non-English targets (fr, de, es, pt, it, nl, vi, id, pl, tr, ...):
		// Reject untranslated non-Latin echoes and obvious verbatim English echoes,
		// while preserving already-localized Latin titles (e.g. "Configurar Docker" for pt-BR).
		nonLatinSrc := srcC.han + srcC.kana + srcC.hangul + srcC.cyrillic + srcC.arabic + srcC.thai + srcC.hebrew
		nonLatinTr := trC.han + trC.kana + trC.hangul + trC.cyrillic + trC.arabic + trC.thai + trC.hebrew
		if nonLatinSrc > 0 && trC.latin == 0 && nonLatinTr > 0 {
			return false
		}
		if strings.EqualFold(src, tr) && looksLikeEnglishPhrase(src) {
			return false
		}
		return true
	}
}

// titleNeedsTranslation determines whether title should be sent to the model for targetLang.
// For non-Latin scripts (Chinese, Japanese, Korean, Cyrillic, Arabic, Thai, Hebrew), fast Unicode script
// checks skip titles that are already in the target language with zero model calls.
func titleNeedsTranslation(title, targetLang string) bool {
	t := strings.TrimSpace(title)
	if isPlaceholderOrRawPromptTitle(t) {
		return false
	}

	c := countScripts(t)
	hanCount, kanaCount, hangulCount, latinCount := c.han, c.kana, c.hangul, c.latin
	cyrillicCount, arabicCount, thaiCount, hebrewCount := c.cyrillic, c.arabic, c.thai, c.hebrew

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
		if hanCount >= 2 && (latinCount == 0 || hanCount*5 >= (hanCount+latinCount) || (hanCount >= 4 && !hasCommonEnglishGlueWord(t))) {
			return false
		}
		return latinCount >= 3 || hangulCount > 0 || cyrillicCount > 0 || arabicCount > 0 || thaiCount > 0
	case "ja":
		if kanaCount >= 1 {
			if latinCount >= 4 && hasCommonEnglishGlueWord(t) && (kanaCount+hanCount)*3 < (kanaCount+hanCount+latinCount) {
				return true
			}
			return false
		}
		return latinCount >= 3 || hanCount >= 2 || hangulCount > 0 || cyrillicCount > 0
	case "ko":
		if hangulCount >= 2 {
			if latinCount >= 4 && hasCommonEnglishGlueWord(t) && hangulCount*3 < (hangulCount+latinCount) {
				return true
			}
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
		return latinCount >= 3 || hanCount >= 2 || kanaCount > 0 || hangulCount > 0
	}
}

func extractFutureTitleCandidate(id string, s map[string]interface{}, enabledAt time.Time, targetLang string) (titleCandidate, bool) {
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
	if archived {
		return titleCandidate{}, false
	}

	// Only translate conversations created AFTER title translation was enabled.
	// Never touch pre-existing conversations even if they receive new messages later.
	createdAt, hasCreated := parseSummaryTimestamp(s["createdTime"])
	modTime, hasMod := parseSummaryTimestamp(s["lastModifiedTime"])
	if !enabledAt.IsZero() {
		if hasCreated {
			if createdAt.Before(enabledAt) {
				return titleCandidate{}, false
			}
		} else if hasMod {
			if modTime.Before(enabledAt) {
				return titleCandidate{}, false
			}
		} else {
			return titleCandidate{}, false
		}
	}

	summaryTitle := ""
	if sm, ok := s["summary"].(string); ok {
		summaryTitle = strings.TrimSpace(sm)
	}
	// If annotations.title already differs from upstream summary, the title was either
	// manually renamed by the user or already overwritten by a prior translation.
	if annTitle != "" && summaryTitle != "" && annTitle != summaryTitle {
		return titleCandidate{}, false
	}

	effectiveTitle := annTitle
	if effectiveTitle == "" {
		effectiveTitle = summaryTitle
	}
	if !titleNeedsTranslation(effectiveTitle, targetLang) {
		return titleCandidate{}, false
	}

	return titleCandidate{
		CascadeID:    id,
		SourceTitle:  effectiveTitle,
		CurrentTitle: effectiveTitle,
		LastModTime:  modTime,
	}, true
}

// MaybeAutoTranslateSummaries inspects conversation summaries from GetAllCascadeTrajectories
// and asynchronously overwrites English titles of newly created conversations (created after
// the option was enabled) with translated titles directly in Antigravity.
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
	enabledAt, ok := parseSummaryTimestamp(prefs.TitleTranslateEnabledAt)
	if !ok {
		return
	}

	now := time.Now()
	ts := &p.titleTranslator
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.ensureInitLocked()

	added := 0
	for id, s := range summaries {
		if ts.baselineIDs[id] || ts.handled[id] || ts.inFlightAuto[id] {
			continue
		}
		if retryAt, backedOff := ts.failBackoff[id]; backedOff && now.Before(retryAt) {
			continue
		}
		if _, queued := ts.pendingAuto[id]; queued {
			continue
		}
		cand, eligible := extractFutureTitleCandidate(id, s, enabledAt, targetLang)
		if !eligible {
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
	ts.ensureInitLocked()
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
	sort.Slice(batch, func(i, j int) bool {
		return batch[i].LastModTime.After(batch[j].LastModTime)
	})
	if len(batch) > titleTranslateBatchSize {
		batch = batch[:titleTranslateBatchSize]
	}
	for _, c := range batch {
		delete(ts.pendingAuto, c.CascadeID)
		ts.inFlightAuto[c.CascadeID] = true
	}
	ts.mu.Unlock()

	defer func() {
		ts.mu.Lock()
		for _, c := range batch {
			delete(ts.inFlightAuto, c.CascadeID)
		}
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
	return fmt.Sprintf(`You are a precise UI translator. Translate EVERY conversation title in the Input JSON into concise, natural %s (%s).
Rules:
- You MUST translate all non-%s words into %s. Do NOT leave English titles untranslated.
- Keep only specific proper nouns, product/project names, acronyms, and technical terms as-is (e.g. Antigravity, Multigravity, iOS, Android, Xcode, Git, Docker, RPC, API, CLI).
- Only return a title unchanged if it is ALREADY written in %s.
- Keep each translated title concise and clean for a chat sidebar list (no trailing period, no quotes).
- Return ONLY a valid JSON object mapping each id string to its translated title. No markdown code fences, no commentary.

Input JSON:
%s`, targetLangName, targetLang, targetLangName, targetLangName, targetLangName, string(rawJSON))
}

func buildSingleTitleTranslatePrompt(title, targetLang, targetLangName string) string {
	return fmt.Sprintf(`You are a precise UI translator. Translate this conversation title into concise, natural %s (%s).
Rules:
- You MUST translate all non-%s words into %s. Do NOT leave English titles untranslated.
- Keep only specific proper nouns, product/project names, acronyms, and technical terms as-is (e.g. Antigravity, Multigravity, iOS, Android, Xcode, Git, RPC, API).
- Only return the title unchanged if it is ALREADY written in %s.
- Output ONLY the translated title with no quotes, no markdown, no trailing period, and no explanation.

Title: %s`, targetLangName, targetLang, targetLangName, targetLangName, targetLangName, redactSensitiveForAI(title))
}

func cleanTranslatedTitleOutput(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	if strings.Contains(s, "<ADDITIONAL_METADATA>") || strings.Contains(s, "<USER_SETTINGS_CHANGE>") {
		s = xmlMetaRegex.ReplaceAllString(s, "")
		s = strings.TrimSpace(s)
	}
	if strings.IndexByte(s, '<') >= 0 {
		s = xmlTagRegex.ReplaceAllString(s, "")
		s = strings.TrimSpace(s)
	}
	s = reControlChars.ReplaceAllString(s, " ")
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			s = trimmed
			break
		}
	}
	s = strings.Trim(s, "\"'`「」『』«»“”")
	s = strings.TrimRight(s, "。.")
	s = strings.TrimSpace(s)
	if isUntitledPlaceholderTitle(s) || utf8.RuneCountInString(s) > maxTranslatedTitleRunes {
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
	var primaryModel AIAssistModelInfo
	var retryIndices []int

	if len(candidates) == 1 {
		prompt := buildSingleTitleTranslatePrompt(candidates[0].SourceTitle, targetLang, targetLangName)
		resp, usedModel, err := p.CallLSModelResponse(ctx, prompt, nil)
		primaryModel = usedModel
		if err != nil {
			p.markTitleBackoff(candidates[0].CascadeID)
			return 0, 1, err
		}
		if cleaned := cleanTranslatedTitleOutput(resp); isTranslationAcceptable(candidates[0].SourceTitle, cleaned, targetLang) {
			results[0] = cleaned
		} else {
			retryIndices = append(retryIndices, 0)
		}
	} else {
		prompt := buildBatchTitleTranslatePrompt(candidates, targetLang, targetLangName)
		resp, usedModel, err := p.CallLSModelResponse(ctx, prompt, nil)
		primaryModel = usedModel
		if err != nil {
			for _, c := range candidates {
				p.markTitleBackoff(c.CascadeID)
			}
			return 0, len(candidates), err
		}
		parsed, parseErr := parseBatchTitleJSON(resp)
		if parseErr != nil {
			slog.Warn("[TitleTranslate] Batch JSON malformed, scheduling retry", "model", usedModel.ID, "count", len(candidates), "err", parseErr)
			for i := range candidates {
				retryIndices = append(retryIndices, i)
			}
		} else {
			for i, c := range candidates {
				key := strconv.Itoa(i + 1)
				if val, ok := parsed[key]; ok {
					if cleaned := cleanTranslatedTitleOutput(val); isTranslationAcceptable(c.SourceTitle, cleaned, targetLang) {
						results[i] = cleaned
						continue
					}
				}
				retryIndices = append(retryIndices, i)
			}
		}
	}

	// Retry any missing, malformed, or untranslated-echo items using a stronger fallback model.
	if len(retryIndices) > 0 {
		fbModel := p.ResolveAIAssistFallbackModel(ctx, primaryModel.Enum)
		fbName := primaryModel.ID
		if fbModel != nil {
			fbName = fbModel.ID
		}
		slog.Warn("[TitleTranslate] Primary model returned untranslated/incomplete items, retrying with fallback",
			"primary_model", primaryModel.ID, "fallback_model", fbName, "retry_count", len(retryIndices), "total", len(candidates))

		var singleRetryIndices []int
		if len(retryIndices) >= 2 && fbModel != nil {
			sub := make([]titleCandidate, len(retryIndices))
			for j, origIdx := range retryIndices {
				sub[j] = candidates[origIdx]
			}
			fbCtx, cancel := context.WithTimeout(ctx, titleTranslateBatchTimeout)
			fbResp, _, fbErr := p.CallLSModelResponse(fbCtx, buildBatchTitleTranslatePrompt(sub, targetLang, targetLangName), fbModel)
			cancel()
			if fbErr == nil {
				if parsed, pErr := parseBatchTitleJSON(fbResp); pErr == nil {
					for j, origIdx := range retryIndices {
						if val, ok := parsed[strconv.Itoa(j+1)]; ok {
							if cleaned := cleanTranslatedTitleOutput(val); isTranslationAcceptable(candidates[origIdx].SourceTitle, cleaned, targetLang) {
								results[origIdx] = cleaned
								continue
							}
						}
						singleRetryIndices = append(singleRetryIndices, origIdx)
					}
				} else {
					singleRetryIndices = retryIndices
				}
			} else {
				if firstErr == nil {
					firstErr = fbErr
				}
				singleRetryIndices = retryIndices
			}
		} else {
			singleRetryIndices = retryIndices
		}

		for _, origIdx := range singleRetryIndices {
			c := candidates[origIdx]
			sCtx, cancel := context.WithTimeout(ctx, titleTranslateSingleTimeout)
			oneResp, _, oneErr := p.CallLSModelResponse(sCtx, buildSingleTitleTranslatePrompt(c.SourceTitle, targetLang, targetLangName), fbModel)
			cancel()
			if oneErr != nil {
				if firstErr == nil {
					firstErr = oneErr
				}
				continue
			}
			if cleaned := cleanTranslatedTitleOutput(oneResp); isTranslationAcceptable(c.SourceTitle, cleaned, targetLang) {
				results[origIdx] = cleaned
			}
		}
	}

	for i, c := range candidates {
		newTitle, ok := results[i]
		if !ok || !isTranslationAcceptable(c.SourceTitle, newTitle, targetLang) {
			p.markTitleBackoff(c.CascadeID)
			failed++
			continue
		}
		applied, err := p.applyTranslatedTitle(ctx, c, newTitle)
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
	slog.Info("[TitleTranslate] Auto-translate batch processed",
		"model", primaryModel.ID, "target_lang", targetLang,
		"candidates", len(candidates), "translated", translated, "failed", failed, "retried", len(retryIndices))
	return translated, failed, firstErr
}

func (p *Proxy) markTitleBackoff(cascadeID string) {
	ts := &p.titleTranslator
	ts.mu.Lock()
	ts.ensureInitLocked()
	ts.failBackoff[cascadeID] = time.Now().Add(titleTranslateFailBackoff)
	ts.mu.Unlock()
}

func (p *Proxy) applyTranslatedTitle(ctx context.Context, cand titleCandidate, translatedTitle string) (bool, error) {
	ts := &p.titleTranslator
	ts.mu.Lock()
	ts.ensureInitLocked()
	if ts.handled[cand.CascadeID] {
		ts.mu.Unlock()
		return false, nil
	}
	ts.mu.Unlock()

	didUpdateTitle := translatedTitle != cand.CurrentTitle && translatedTitle != ""
	if didUpdateTitle {
		if err := p.writeUpstreamConversationTitle(ctx, cand.CascadeID, translatedTitle); err != nil {
			return false, err
		}
	}

	ts.mu.Lock()
	ts.ensureInitLocked()
	ts.handled[cand.CascadeID] = true
	delete(ts.failBackoff, cand.CascadeID)
	ts.mu.Unlock()
	return didUpdateTitle, nil
}

func (p *Proxy) writeUpstreamConversationTitle(ctx context.Context, cascadeID, title string) error {
	title = strings.TrimSpace(title)
	if cascadeID == "" || isUntitledPlaceholderTitle(title) {
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
