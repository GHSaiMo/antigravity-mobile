package proxy

import (
	"context"
	"encoding/json"
	"fmt"
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
	liveTextCacheMaxEntries       = 1000
	liveTextFailBackoff           = 5 * time.Minute
	liveTextBatchTimeout          = 15 * time.Second
	liveTextSingleTimeout         = 12 * time.Second
	liveTextLiveActivityWaitCap   = 1500 * time.Millisecond
	liveTextDiskFlushDebounce     = 2 * time.Second
	liveTextMaxStepBatchItems     = 24
	liveTextMaxStepOutputRunes    = 160
	liveTextMaxFocusOutputRunes   = 200
	liveTextMaxInputRunes         = 240
	liveTextRecentPlansToSchedule = 2
)

const (
	liveTextKindStep  = "step"
	liveTextKindFocus = "focus"
)

type liveTextCacheEntry struct {
	Translated string `json:"translated"`
	UpdatedAt  string `json:"updatedAt"`
}

type liveTextTranslatorState struct {
	mu             sync.Mutex
	loaded         bool
	cachePath      string
	cache          map[string]liveTextCacheEntry
	inFlight       map[string]chan struct{}
	failBackoff    map[string]time.Time
	cascadeVersion map[string]uint64
	prefsVersion   uint64
	dirty          bool
	flushTimer     *time.Timer
}

func liveTextCacheFilePath() string {
	return filepath.Join(config.GetDataDir(), "live_text_translations.json")
}

func (lt *liveTextTranslatorState) ensureLoadedLocked() {
	want := liveTextCacheFilePath()
	if lt.loaded && lt.cachePath == want {
		return
	}
	lt.cachePath = want
	lt.cache = make(map[string]liveTextCacheEntry)
	if lt.inFlight == nil {
		lt.inFlight = make(map[string]chan struct{})
	}
	if lt.failBackoff == nil {
		lt.failBackoff = make(map[string]time.Time)
	}
	if lt.cascadeVersion == nil {
		lt.cascadeVersion = make(map[string]uint64)
	}
	lt.loaded = true

	data, err := os.ReadFile(want)
	if err != nil {
		return
	}
	var parsed map[string]liveTextCacheEntry
	if err := json.Unmarshal(data, &parsed); err != nil {
		slog.Warn("[LiveTextTranslate] Failed to parse live_text_translations.json, starting fresh", "err", err)
		return
	}
	if parsed != nil {
		lt.cache = parsed
	}
}

func (lt *liveTextTranslatorState) scheduleDiskFlushLocked() {
	lt.dirty = true
	if lt.flushTimer != nil {
		return
	}
	lt.flushTimer = time.AfterFunc(liveTextDiskFlushDebounce, func() {
		lt.flushToDisk()
	})
}

func (lt *liveTextTranslatorState) flushToDisk() {
	lt.mu.Lock()
	lt.flushTimer = nil
	if !lt.dirty || lt.cachePath == "" {
		lt.mu.Unlock()
		return
	}
	lt.dirty = false
	path := lt.cachePath
	snap := make(map[string]liveTextCacheEntry, len(lt.cache))
	for k, v := range lt.cache {
		snap[k] = v
	}
	lt.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0600); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
}

func (lt *liveTextTranslatorState) putLocked(cacheKey, translated string) {
	lt.ensureLoadedLocked()
	if len(lt.cache) >= liveTextCacheMaxEntries {
		// Evict oldest ~10% entries to keep cache bounded cleanly.
		type kv struct {
			k  string
			ts string
		}
		all := make([]kv, 0, len(lt.cache))
		for k, v := range lt.cache {
			all = append(all, kv{k: k, ts: v.UpdatedAt})
		}
		sort.Slice(all, func(i, j int) bool {
			return all[i].ts < all[j].ts
		})
		evictCount := len(all) / 10
		if evictCount < 1 {
			evictCount = 1
		}
		for i := 0; i < evictCount && i < len(all); i++ {
			delete(lt.cache, all[i].k)
		}
	}
	lt.cache[cacheKey] = liveTextCacheEntry{
		Translated: translated,
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	delete(lt.failBackoff, cacheKey)
	lt.scheduleDiskFlushLocked()
}

func (lt *liveTextTranslatorState) BumpPrefsVersion() {
	lt.mu.Lock()
	lt.prefsVersion++
	lt.mu.Unlock()
}

func (lt *liveTextTranslatorState) bumpCascadeVersionLocked(cascadeID string) {
	if cascadeID == "" {
		return
	}
	if lt.cascadeVersion == nil {
		lt.cascadeVersion = make(map[string]uint64)
	}
	lt.cascadeVersion[cascadeID]++
}

func (p *Proxy) liveTextTranslateSignature(cascadeID string) string {
	if p == nil {
		return ""
	}
	lt := &p.liveTextTranslator
	lt.mu.Lock()
	pv := lt.prefsVersion
	cv := lt.cascadeVersion[cascadeID]
	lt.mu.Unlock()
	return fmt.Sprintf("%d:%d", pv, cv)
}

func normalizeLiveTextKey(raw string) string {
	return stepPlanMultiSpaceRe.ReplaceAllString(strings.TrimSpace(raw), " ")
}

func makeLiveTextCacheKey(kind, targetLang, original string) string {
	return kind + "|" + strings.ToLower(strings.TrimSpace(targetLang)) + "|" + normalizeLiveTextKey(original)
}

// extractRequestLangHint extracts a raw BCP-47 language tag hint from ?lang= or Accept-Language.
func extractRequestLangHint(r *http.Request) string {
	if r == nil {
		return ""
	}
	if q := strings.TrimSpace(r.URL.Query().Get("lang")); q != "" && !strings.EqualFold(q, "auto") && !strings.EqualFold(q, "system") {
		if len(q) <= 32 {
			return q
		}
	}
	return parseFirstAcceptLanguageTag(r.Header.Get("Accept-Language"))
}

func parseFirstAcceptLanguageTag(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	for _, part := range strings.Split(header, ",") {
		tag := strings.TrimSpace(part)
		if idx := strings.IndexByte(tag, ';'); idx >= 0 {
			tag = strings.TrimSpace(tag[:idx])
		}
		if tag == "" || tag == "*" || len(tag) > 32 {
			continue
		}
		return tag
	}
	return ""
}

// resolveLiveTranslateTargetLang resolves the effective non-English target language for live
// step-plan and focus translation. Returns ok=false when target language is English.
func resolveLiveTranslateTargetLang(prefs AIAssistPrefs, fallbackLang string) (string, string, bool) {
	rawCode := strings.TrimSpace(prefs.TitleTranslateLang)
	rawName := strings.TrimSpace(prefs.TitleTranslateLangName)
	if rawCode == "" {
		rawCode = strings.TrimSpace(fallbackLang)
	}
	if rawCode == "" {
		rawCode = i18n.GetLang()
	}
	targetLang, targetLangName := normalizeTitleTargetLang(rawCode, rawName)
	if isEnglishLangTag(targetLang) {
		return "", "", false
	}
	return targetLang, targetLangName, true
}

func hasCommonEnglishGlueWord(s string) bool {
	lower := " " + strings.ToLower(s) + " "
	for _, w := range []string{" to ", " and ", " for ", " with ", " from ", " into ", " the ", " in ", " on ", " of "} {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

// liveTextNeedsTranslation checks whether a step-plan item or focus phrase needs translation
// into targetLang. Compared to titleNeedsTranslation, it handles mixed-language technical items
// such as "Install to 九仔 and restart mgy" or "Deploy to 九仔" cleanly without skipping them.
func liveTextNeedsTranslation(raw, targetLang string) bool {
	t := strings.TrimSpace(raw)
	if t == "" || utf8.RuneCountInString(t) > liveTextMaxInputRunes {
		return false
	}
	if strings.HasPrefix(t, "/") || strings.Contains(t, "/Users/") || strings.Contains(t, "/home/") || strings.Contains(t, `C:\`) {
		return false
	}

	var hanCount, kanaCount, hangulCount, latinCount, cyrillicCount, arabicCount, thaiCount, hebrewCount int
	var firstLetter rune
	for _, r := range t {
		isLatin := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isHan := unicode.Is(unicode.Han, r)
		isKana := (r >= 0x3040 && r <= 0x30ff) || (r >= 0x31f0 && r <= 0x31ff)
		isHangul := unicode.Is(unicode.Hangul, r)
		isCyrillic := unicode.Is(unicode.Cyrillic, r)
		isArabic := unicode.Is(unicode.Arabic, r)
		isThai := unicode.Is(unicode.Thai, r)
		isHebrew := unicode.Is(unicode.Hebrew, r)
		if firstLetter == 0 && (isLatin || isHan || isKana || isHangul || isCyrillic || isArabic || isThai || isHebrew) {
			firstLetter = r
		}
		switch {
		case isHan:
			hanCount++
		case isKana:
			kanaCount++
		case isHangul:
			hangulCount++
		case isCyrillic:
			cyrillicCount++
		case isArabic:
			arabicCount++
		case isThai:
			thaiCount++
		case isHebrew:
			hebrewCount++
		case isLatin:
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
		return false
	case "zh":
		if kanaCount > 0 {
			return true
		}
		if hanCount >= 2 {
			if latinCount == 0 {
				return false
			}
			// If the phrase begins with a Latin letter and contains English glue words ("to", "and", ...)
			// with sparse Han (e.g. "Deploy to 九仔", "Install to 九仔 and restart mgy"), translate it.
			startsLatin := (firstLetter >= 'a' && firstLetter <= 'z') || (firstLetter >= 'A' && firstLetter <= 'Z')
			if startsLatin && hasCommonEnglishGlueWord(t) && hanCount*3 < (hanCount+latinCount) {
				return true
			}
			// If it starts with Han or has meaningful Han ratio (e.g. "更新 iOS 与 Android 布局"), skip.
			if unicode.Is(unicode.Han, firstLetter) || hanCount*5 >= (hanCount+latinCount) {
				return false
			}
		}
		return latinCount >= 3 || hangulCount > 0 || cyrillicCount > 0 || arabicCount > 0 || thaiCount > 0
	case "ja":
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
		// Latin-script non-English languages (pt, es, fr, de, it, nl, vi, id, pl, tr, ...):
		// cache guarantees each unique phrase is evaluated once.
		return latinCount >= 3 || hanCount >= 2 || kanaCount > 0 || hangulCount > 0
	}
}

func buildStepPlanBatchTranslatePrompt(items []string, targetLang, targetLangName string) string {
	type promptItem struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	arr := make([]promptItem, len(items))
	for i, text := range items {
		arr[i] = promptItem{
			ID:   strconv.Itoa(i + 1),
			Text: redactSensitiveForAI(text),
		}
	}
	rawJSON, _ := json.Marshal(arr)
	return fmt.Sprintf(`You are a precise UI translator. Translate EVERY short task checklist item in the Input JSON into concise, natural %s (%s) imperative phrasing for a mobile progress card.
Rules:
- You MUST translate all non-%s words into %s. Do NOT leave English items untranslated.
- Keep only specific proper nouns, code identifiers, filenames, model names, device/host names, acronyms, and technical terms as-is (e.g. iOS, Android, Web, HUD, API, RPC, Git, Xcode, Docker, mgy).
- Only return an item unchanged if it is ALREADY written in %s.
- Keep each item concise and clean (no numbering, no bullet prefix, no quotes, no trailing period).
- Return ONLY a valid JSON object mapping each id string to its translated text. No markdown code fences, no commentary.

Input JSON:
%s`, targetLangName, targetLang, targetLangName, targetLangName, targetLangName, string(rawJSON))
}

func buildFocusTranslatePrompt(focus, targetLang, targetLangName string) string {
	return fmt.Sprintf(`You are a precise UI translator. Translate this short live progress status phrase into concise, natural %s (%s) for a mobile status bar and Dynamic Island.
Rules:
- You MUST translate all non-%s words into %s. Do NOT leave English phrases untranslated.
- Keep only specific proper nouns, code identifiers, filenames, model names, device/host names, acronyms, and technical terms as-is (e.g. iOS, Android, Web, HUD, API, RPC, Git, Xcode, mgy).
- Only return the phrase unchanged if it is ALREADY written in %s.
- Output ONLY the translated status phrase with no quotes, no markdown, no trailing period, and no commentary.

Status: %s`, targetLangName, targetLang, targetLangName, targetLangName, targetLangName, redactSensitiveForAI(focus))
}

func cleanLiveTranslatedOutput(raw string, maxRunes int) string {
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
	if s == "" || utf8.RuneCountInString(s) > maxRunes {
		return ""
	}
	return s
}

func (p *Proxy) lookupLiveTextTranslation(kind, targetLang, original string) (string, bool) {
	key := makeLiveTextCacheKey(kind, targetLang, original)
	lt := &p.liveTextTranslator
	lt.mu.Lock()
	lt.ensureLoadedLocked()
	entry, ok := lt.cache[key]
	lt.mu.Unlock()
	if !ok || strings.TrimSpace(entry.Translated) == "" {
		return "", false
	}
	return entry.Translated, true
}

func cloneStepPlanWithTranslations(plan *StepPlanItem, lookup func(orig string) (string, bool), record func(orig, tr string)) (*StepPlanItem, bool) {
	if plan == nil || len(plan.Items) == 0 {
		return plan, false
	}
	changed := false
	var itemsCopy []StepPlanItemEntry
	for i, it := range plan.Items {
		if tr, ok := lookup(it.Title); ok && tr != "" {
			if record != nil {
				record(it.Title, tr)
			}
			if tr != it.Title {
				if !changed {
					itemsCopy = make([]StepPlanItemEntry, len(plan.Items))
					copy(itemsCopy, plan.Items)
					changed = true
				}
				itemsCopy[i].Title = tr
				// Preserve itemsCopy[i].Key untouched so SwiftUI/Compose/Web keep stable row identity.
			}
		}
	}
	if !changed {
		return plan, false
	}
	planCopy := *plan
	planCopy.Items = itemsCopy
	return &planCopy, true
}

// ApplyLiveTextTranslations overlays cached StepPlan (update_steps) and Focus (update_focus)
// translations onto a copy-on-write view of details (never mutating shared trajectory cache slices)
// and optionally schedules asynchronous GetModelResponse translations for uncached recent items.
// Returns a map of original -> translated texts applied (used by Web clients rendering raw Steps).
func (p *Proxy) ApplyLiveTextTranslations(cascadeID string, details *TrajectoryDetails, fallbackLang string, allowSchedule bool) map[string]string {
	if p == nil || details == nil {
		return nil
	}
	prefs := p.AIAssistPrefs()
	if !prefs.StepPlanTranslateEnabled && !prefs.FocusTranslateEnabled {
		return nil
	}
	if !p.IsAIAssistAvailable() {
		return nil
	}
	targetLang, targetLangName, ok := resolveLiveTranslateTargetLang(prefs, fallbackLang)
	if !ok {
		return nil
	}

	applied := make(map[string]string)
	recordApplied := func(orig, tr string) {
		orig = strings.TrimSpace(orig)
		tr = strings.TrimSpace(tr)
		if orig != "" && tr != "" && tr != orig {
			applied[orig] = tr
		}
	}

	// Copy-on-write AllMessages slice so shared ParseTrajectoryDetailsCached buffers are never mutated.
	var msgsCopy []CascadeMessageItem
	ensureMsgsCopy := func() []CascadeMessageItem {
		if msgsCopy == nil {
			msgsCopy = make([]CascadeMessageItem, len(details.AllMessages))
			copy(msgsCopy, details.AllMessages)
		}
		return msgsCopy
	}

	// 1. Apply cached StepPlan translations.
	var stepPlanIndices []int
	if prefs.StepPlanTranslateEnabled {
		stepLookup := func(orig string) (string, bool) {
			return p.lookupLiveTextTranslation(liveTextKindStep, targetLang, orig)
		}
		for i := range details.AllMessages {
			if details.AllMessages[i].StepPlan == nil {
				continue
			}
			stepPlanIndices = append(stepPlanIndices, i)
			if updatedPlan, changed := cloneStepPlanWithTranslations(details.AllMessages[i].StepPlan, stepLookup, recordApplied); changed {
				mc := ensureMsgsCopy()
				mc[i].StepPlan = updatedPlan
			}
		}
		if details.ActivePlan != nil {
			if updatedPlan, changed := cloneStepPlanWithTranslations(details.ActivePlan, stepLookup, recordApplied); changed {
				details.ActivePlan = updatedPlan
			}
		}
	}

	// 2. Apply cached Focus translations.
	lastToolsFocus := ""
	if prefs.FocusTranslateEnabled {
		for i := range details.AllMessages {
			ft := strings.TrimSpace(details.AllMessages[i].FocusText)
			if ft == "" {
				continue
			}
			lastToolsFocus = ft
			if tr, ok := p.lookupLiveTextTranslation(liveTextKindFocus, targetLang, ft); ok && tr != "" {
				recordApplied(ft, tr)
				if tr != ft {
					mc := ensureMsgsCopy()
					mc[i].FocusText = tr
				}
			}
		}
		if af := strings.TrimSpace(details.ActiveFocus); af != "" {
			if tr, ok := p.lookupLiveTextTranslation(liveTextKindFocus, targetLang, af); ok && tr != "" {
				recordApplied(af, tr)
				if tr != af {
					details.ActiveFocus = tr
				}
			}
		}
	}

	if msgsCopy != nil {
		details.AllMessages = msgsCopy
	}

	// 3. Schedule asynchronous translations for uncached recent items (bounded to prevent history storms).
	if allowSchedule {
		if prefs.StepPlanTranslateEnabled {
			var missingSteps []string
			seenStep := make(map[string]bool)
			collectPlan := func(plan *StepPlanItem) {
				if plan == nil {
					return
				}
				for _, it := range plan.Items {
					orig := strings.TrimSpace(it.Title)
					if orig == "" || len(missingSteps) >= liveTextMaxStepBatchItems {
						continue
					}
					norm := normalizeLiveTextKey(orig)
					if seenStep[norm] {
						continue
					}
					seenStep[norm] = true
					if !liveTextNeedsTranslation(orig, targetLang) {
						continue
					}
					if _, cached := p.lookupLiveTextTranslation(liveTextKindStep, targetLang, orig); cached {
						continue
					}
					missingSteps = append(missingSteps, orig)
				}
			}
			collectPlan(details.ActivePlan)
			startIdx := len(stepPlanIndices) - liveTextRecentPlansToSchedule
			if startIdx < 0 {
				startIdx = 0
			}
			for _, msgIdx := range stepPlanIndices[startIdx:] {
				collectPlan(details.AllMessages[msgIdx].StepPlan)
			}
			if len(missingSteps) > 0 {
				p.scheduleStepPlanTranslationBatch(cascadeID, missingSteps, targetLang, targetLangName)
			}
		}

		if prefs.FocusTranslateEnabled {
			var missingFocus []string
			seenFocus := make(map[string]bool)
			addFocus := func(raw string) {
				orig := strings.TrimSpace(raw)
				if orig == "" {
					return
				}
				norm := normalizeLiveTextKey(orig)
				if seenFocus[norm] {
					return
				}
				seenFocus[norm] = true
				if !liveTextNeedsTranslation(orig, targetLang) {
					return
				}
				if _, cached := p.lookupLiveTextTranslation(liveTextKindFocus, targetLang, orig); cached {
					return
				}
				missingFocus = append(missingFocus, orig)
			}
			addFocus(details.ActiveFocus)
			addFocus(lastToolsFocus)
			for _, f := range missingFocus {
				p.scheduleFocusTranslation(cascadeID, f, targetLang, targetLangName)
			}
		}
	}

	if len(applied) == 0 {
		return nil
	}
	return applied
}

// scheduleStepPlanTranslationBatch starts a single-flight batched GetModelResponse call for uncached step items
// and returns wait channels for all keys (both newly started and already in-flight).
func (p *Proxy) scheduleStepPlanTranslationBatch(cascadeID string, items []string, targetLang, targetLangName string) []chan struct{} {
	if len(items) == 0 {
		return nil
	}
	now := time.Now()
	lt := &p.liveTextTranslator
	lt.mu.Lock()
	lt.ensureLoadedLocked()

	var toTranslate []string
	var toKeys []string
	var waitChs []chan struct{}
	batchDone := make(chan struct{})

	for _, orig := range items {
		key := makeLiveTextCacheKey(liveTextKindStep, targetLang, orig)
		if _, cached := lt.cache[key]; cached {
			continue
		}
		if retryAt, backedOff := lt.failBackoff[key]; backedOff && now.Before(retryAt) {
			continue
		}
		if ch, busy := lt.inFlight[key]; busy {
			waitChs = append(waitChs, ch)
			continue
		}
		lt.inFlight[key] = batchDone
		toTranslate = append(toTranslate, orig)
		toKeys = append(toKeys, key)
	}
	lt.mu.Unlock()

	if len(toTranslate) == 0 {
		return waitChs
	}
	waitChs = append(waitChs, batchDone)

	go func() {
		defer func() {
			lt.mu.Lock()
			for _, k := range toKeys {
				if lt.inFlight[k] == batchDone {
					delete(lt.inFlight, k)
				}
			}
			lt.mu.Unlock()
			close(batchDone)
		}()

		ctx, cancel := context.WithTimeout(context.Background(), liveTextBatchTimeout)
		defer cancel()

		prompt := buildStepPlanBatchTranslatePrompt(toTranslate, targetLang, targetLangName)
		rawResp, usedModel, err := p.CallLSModelResponse(ctx, prompt, nil)
		if err != nil {
			slog.Debug("[LiveTextTranslate] step batch failed", "cascade_id", shortCascadeID(cascadeID), "count", len(toTranslate), "err", err)
			lt.mu.Lock()
			backoffUntil := time.Now().Add(liveTextFailBackoff)
			for _, k := range toKeys {
				lt.failBackoff[k] = backoffUntil
			}
			lt.mu.Unlock()
			return
		}

		accepted := make(map[int]string, len(toTranslate))
		extractBatch := func(resp string, indices []int) {
			parsed, parseErr := parseBatchTitleJSON(resp)
			if parseErr != nil {
				if len(indices) == 1 {
					idx := indices[0]
					if cleaned := cleanLiveTranslatedOutput(resp, liveTextMaxStepOutputRunes); isTranslationAcceptable(toTranslate[idx], cleaned, targetLang) {
						accepted[idx] = cleaned
					}
				}
				return
			}
			for pos, idx := range indices {
				val, ok := parsed[strconv.Itoa(pos+1)]
				if !ok {
					continue
				}
				if cleaned := cleanLiveTranslatedOutput(val, liveTextMaxStepOutputRunes); isTranslationAcceptable(toTranslate[idx], cleaned, targetLang) {
					accepted[idx] = cleaned
				}
			}
		}

		allIdx := make([]int, len(toTranslate))
		for i := range toTranslate {
			allIdx[i] = i
		}
		extractBatch(rawResp, allIdx)

		var missingIdx []int
		for i := range toTranslate {
			if _, ok := accepted[i]; !ok {
				missingIdx = append(missingIdx, i)
			}
		}
		if len(missingIdx) > 0 {
			if fbModel := p.ResolveAIAssistFallbackModel(ctx, usedModel.Enum); fbModel != nil {
				subItems := make([]string, len(missingIdx))
				for j, idx := range missingIdx {
					subItems[j] = toTranslate[idx]
				}
				fbCtx, fbCancel := context.WithTimeout(context.Background(), liveTextBatchTimeout)
				fbResp, _, fbErr := p.CallLSModelResponse(fbCtx, buildStepPlanBatchTranslatePrompt(subItems, targetLang, targetLangName), fbModel)
				fbCancel()
				if fbErr == nil {
					extractBatch(fbResp, missingIdx)
				}
			}
		}

		updated := false
		lt.mu.Lock()
		backoffUntil := time.Now().Add(liveTextFailBackoff)
		for i, k := range toKeys {
			if cleaned, ok := accepted[i]; ok && cleaned != "" {
				lt.putLocked(k, cleaned)
				updated = true
			} else {
				lt.failBackoff[k] = backoffUntil
			}
		}
		if updated {
			lt.bumpCascadeVersionLocked(cascadeID)
		}
		lt.mu.Unlock()

		if updated && cascadeID != "" {
			p.notifyStreamTouch(cascadeID)
		}
	}()

	return waitChs
}

// scheduleFocusTranslation starts a single-flight GetModelResponse call for a single focus phrase
// and returns a channel that closes when the translation completes (or nil if already cached/backed-off).
func (p *Proxy) scheduleFocusTranslation(cascadeID, focus, targetLang, targetLangName string) chan struct{} {
	orig := strings.TrimSpace(focus)
	if orig == "" {
		return nil
	}
	key := makeLiveTextCacheKey(liveTextKindFocus, targetLang, orig)
	now := time.Now()
	lt := &p.liveTextTranslator
	lt.mu.Lock()
	lt.ensureLoadedLocked()
	if _, cached := lt.cache[key]; cached {
		lt.mu.Unlock()
		return nil
	}
	if retryAt, backedOff := lt.failBackoff[key]; backedOff && now.Before(retryAt) {
		lt.mu.Unlock()
		return nil
	}
	if ch, busy := lt.inFlight[key]; busy {
		lt.mu.Unlock()
		return ch
	}
	done := make(chan struct{})
	lt.inFlight[key] = done
	lt.mu.Unlock()

	go func() {
		defer func() {
			lt.mu.Lock()
			if lt.inFlight[key] == done {
				delete(lt.inFlight, key)
			}
			lt.mu.Unlock()
			close(done)
		}()

		ctx, cancel := context.WithTimeout(context.Background(), liveTextSingleTimeout)
		defer cancel()

		prompt := buildFocusTranslatePrompt(orig, targetLang, targetLangName)
		rawResp, usedModel, err := p.CallLSModelResponse(ctx, prompt, nil)
		if err != nil {
			slog.Debug("[LiveTextTranslate] focus failed", "cascade_id", shortCascadeID(cascadeID), "err", err)
			lt.mu.Lock()
			lt.failBackoff[key] = time.Now().Add(liveTextFailBackoff)
			lt.mu.Unlock()
			return
		}
		cleaned := cleanLiveTranslatedOutput(rawResp, liveTextMaxFocusOutputRunes)
		if !isTranslationAcceptable(orig, cleaned, targetLang) {
			cleaned = ""
			if fbModel := p.ResolveAIAssistFallbackModel(ctx, usedModel.Enum); fbModel != nil {
				fbCtx, fbCancel := context.WithTimeout(context.Background(), liveTextSingleTimeout)
				fbResp, _, fbErr := p.CallLSModelResponse(fbCtx, prompt, fbModel)
				fbCancel()
				if fbErr == nil {
					if fbClean := cleanLiveTranslatedOutput(fbResp, liveTextMaxFocusOutputRunes); isTranslationAcceptable(orig, fbClean, targetLang) {
						cleaned = fbClean
					}
				}
			}
		}
		lt.mu.Lock()
		if cleaned != "" {
			lt.putLocked(key, cleaned)
			lt.bumpCascadeVersionLocked(cascadeID)
		} else {
			lt.failBackoff[key] = time.Now().Add(liveTextFailBackoff)
		}
		lt.mu.Unlock()

		if cleaned != "" && cascadeID != "" {
			p.notifyStreamTouch(cascadeID)
		}
	}()

	return done
}

// EnsureLiveTextTranslatedForLiveActivity translates details.ActiveFocus and details.ActivePlan
// in-place (on the caller's snapshot copy) before an APNs Live Activity push is sent.
// Because Live Activity routine pushes are throttled (18s), this waits briefly (up to 1.5s)
// on any in-flight or newly scheduled Flash Lite call so the very first Dynamic Island / Lock Screen
// update already renders in the user's language, falling back seamlessly to original text on timeout.
func (p *Proxy) EnsureLiveTextTranslatedForLiveActivity(ctx context.Context, details *TrajectoryDetails, fallbackLang string) {
	if p == nil || details == nil {
		return
	}
	prefs := p.AIAssistPrefs()
	if !prefs.StepPlanTranslateEnabled && !prefs.FocusTranslateEnabled {
		return
	}
	if !p.IsAIAssistAvailable() {
		return
	}
	targetLang, targetLangName, ok := resolveLiveTranslateTargetLang(prefs, fallbackLang)
	if !ok {
		return
	}

	var waitChs []chan struct{}
	if prefs.FocusTranslateEnabled {
		if af := strings.TrimSpace(details.ActiveFocus); af != "" && liveTextNeedsTranslation(af, targetLang) {
			if _, cached := p.lookupLiveTextTranslation(liveTextKindFocus, targetLang, af); !cached {
				if ch := p.scheduleFocusTranslation(details.CascadeID, af, targetLang, targetLangName); ch != nil {
					waitChs = append(waitChs, ch)
				}
			}
		}
	}
	if prefs.StepPlanTranslateEnabled && details.ActivePlan != nil {
		var missing []string
		seen := make(map[string]bool)
		for _, it := range details.ActivePlan.Items {
			orig := strings.TrimSpace(it.Title)
			if orig == "" || len(missing) >= liveTextMaxStepBatchItems {
				continue
			}
			norm := normalizeLiveTextKey(orig)
			if seen[norm] {
				continue
			}
			seen[norm] = true
			if !liveTextNeedsTranslation(orig, targetLang) {
				continue
			}
			if _, cached := p.lookupLiveTextTranslation(liveTextKindStep, targetLang, orig); !cached {
				missing = append(missing, orig)
			}
		}
		if len(missing) > 0 {
			waitChs = append(waitChs, p.scheduleStepPlanTranslationBatch(details.CascadeID, missing, targetLang, targetLangName)...)
		}
	}

	if len(waitChs) > 0 {
		waitCtx, cancel := context.WithTimeout(ctx, liveTextLiveActivityWaitCap)
	waitLoop:
		for _, ch := range waitChs {
			select {
			case <-ch:
			case <-waitCtx.Done():
				break waitLoop
			}
		}
		cancel()
	}

	// Overlay any newly populated cached translations onto ActiveFocus & ActivePlan.
	if prefs.FocusTranslateEnabled {
		if af := strings.TrimSpace(details.ActiveFocus); af != "" {
			if tr, ok := p.lookupLiveTextTranslation(liveTextKindFocus, targetLang, af); ok && tr != "" {
				details.ActiveFocus = tr
			}
		}
	}
	if prefs.StepPlanTranslateEnabled && details.ActivePlan != nil {
		stepLookup := func(orig string) (string, bool) {
			return p.lookupLiveTextTranslation(liveTextKindStep, targetLang, orig)
		}
		if updatedPlan, changed := cloneStepPlanWithTranslations(details.ActivePlan, stepLookup, nil); changed {
			details.ActivePlan = updatedPlan
		}
	}
}
