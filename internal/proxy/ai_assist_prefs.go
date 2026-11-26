package proxy

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"antigravity-mobile/internal/config"
	"antigravity-mobile/internal/i18n"
)

const (
	NextPromptTapFill = "fill"
	NextPromptTapSend = "send"
)

// AIAssistPrefs holds gateway-persisted, cross-device AI Assist preferences.
// All capabilities default to opt-in (false) so no extra model calls are issued unexpectedly.
type AIAssistPrefs struct {
	TitleTranslateEnabled  bool   `json:"titleTranslateEnabled"`
	TitleTranslateLang     string `json:"titleTranslateLang,omitempty"`     // BCP-47 tag (e.g. "zh-Hans", "ja", "pt-BR"), empty = auto
	TitleTranslateLangName string `json:"titleTranslateLangName,omitempty"` // English display name (e.g. "Simplified Chinese", "Portuguese")
	NextPromptEnabled      bool   `json:"nextPromptEnabled"`
	NextPromptTapAction    string `json:"nextPromptTapAction"` // "fill" | "send"
}

// AIAssistPrefsPatch supports partial updates from mobile/web settings clients.
type AIAssistPrefsPatch struct {
	TitleTranslateEnabled  *bool   `json:"titleTranslateEnabled,omitempty"`
	TitleTranslateLang     *string `json:"titleTranslateLang,omitempty"`
	TitleTranslateLangName *string `json:"titleTranslateLangName,omitempty"`
	NextPromptEnabled      *bool   `json:"nextPromptEnabled,omitempty"`
	NextPromptTapAction    *string `json:"nextPromptTapAction,omitempty"`
}

// AIAssistStatusResponse is returned by GET and POST /gateway/ai-assist/prefs.
type AIAssistStatusResponse struct {
	Prefs            AIAssistPrefs             `json:"prefs"`
	ModelID          string                    `json:"modelId"`
	ModelName        string                    `json:"modelName"`
	ResolvedLang     string                    `json:"resolvedLang"`
	ResolvedLangName string                    `json:"resolvedLangName"`
	TranslatedCount  int                       `json:"translatedCount"`
	BatchStatus      *TitleTranslateBatchState `json:"batchStatus,omitempty"`
	Available        bool                      `json:"available"`
}

type aiAssistPrefsStore struct {
	mu     sync.RWMutex
	loaded bool
	path   string
	prefs  AIAssistPrefs
}

func envBoolDefaultFalse(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func defaultAIAssistPrefs() AIAssistPrefs {
	tapAction := strings.ToLower(strings.TrimSpace(os.Getenv("MGY_AI_NEXT_PROMPT_TAP")))
	if tapAction != NextPromptTapSend {
		tapAction = NextPromptTapFill
	}
	return AIAssistPrefs{
		TitleTranslateEnabled: envBoolDefaultFalse("MGY_AI_TITLE_TRANSLATE"),
		NextPromptEnabled:     envBoolDefaultFalse("MGY_AI_NEXT_PROMPT"),
		NextPromptTapAction:   tapAction,
	}
}

func aiAssistPrefsFilePath() string {
	return filepath.Join(config.GetDataDir(), "ai_assist_prefs.json")
}

func normalizeTapAction(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), NextPromptTapSend) {
		return NextPromptTapSend
	}
	return NextPromptTapFill
}

func (s *aiAssistPrefsStore) ensureLoadedLocked() {
	wantPath := aiAssistPrefsFilePath()
	if s.loaded && s.path == wantPath {
		return
	}
	s.path = wantPath
	s.prefs = defaultAIAssistPrefs()
	s.loaded = true

	data, err := os.ReadFile(wantPath)
	if err != nil {
		return
	}
	var patch AIAssistPrefsPatch
	if err := json.Unmarshal(data, &patch); err != nil {
		slog.Warn("[AIAssist] Failed to parse ai_assist_prefs.json, using defaults", "err", err)
		return
	}
	applyAIAssistPrefsPatch(&s.prefs, patch)
}

func applyAIAssistPrefsPatch(dst *AIAssistPrefs, patch AIAssistPrefsPatch) {
	if patch.TitleTranslateEnabled != nil {
		dst.TitleTranslateEnabled = *patch.TitleTranslateEnabled
	}
	if patch.TitleTranslateLang != nil {
		lang := strings.TrimSpace(*patch.TitleTranslateLang)
		if strings.EqualFold(lang, "auto") || strings.EqualFold(lang, "system") {
			lang = ""
		}
		if len(lang) <= 32 {
			dst.TitleTranslateLang = lang
		}
	}
	if patch.TitleTranslateLangName != nil {
		name := strings.TrimSpace(*patch.TitleTranslateLangName)
		if len(name) <= 64 {
			dst.TitleTranslateLangName = name
		}
	}
	if patch.NextPromptEnabled != nil {
		dst.NextPromptEnabled = *patch.NextPromptEnabled
	}
	if patch.NextPromptTapAction != nil {
		dst.NextPromptTapAction = normalizeTapAction(*patch.NextPromptTapAction)
	}
	dst.NextPromptTapAction = normalizeTapAction(dst.NextPromptTapAction)
}

func (s *aiAssistPrefsStore) Get() AIAssistPrefs {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	return s.prefs
}

func (s *aiAssistPrefsStore) Update(patch AIAssistPrefsPatch) (AIAssistPrefs, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	applyAIAssistPrefsPatch(&s.prefs, patch)
	out := s.prefs

	if s.path != "" {
		if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
			return out, err
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return out, err
		}
		tmp := s.path + ".tmp"
		if err := os.WriteFile(tmp, append(data, '\n'), 0600); err != nil {
			return out, err
		}
		if err := os.Rename(tmp, s.path); err != nil {
			_ = os.Remove(tmp)
			return out, err
		}
	}
	return out, nil
}

// AIAssistPrefs returns the current gateway AI Assist preferences snapshot.
func (p *Proxy) AIAssistPrefs() AIAssistPrefs {
	if p == nil {
		return defaultAIAssistPrefs()
	}
	return p.aiAssistPrefs.Get()
}

func (p *Proxy) buildAIAssistStatus(r *http.Request) AIAssistStatusResponse {
	prefs := p.AIAssistPrefs()
	model := p.ResolveAIAssistModel(r.Context())
	resolvedLang, resolvedLangName := normalizeTitleTargetLang(prefs.TitleTranslateLang, prefs.TitleTranslateLangName)
	if prefs.TitleTranslateLang == "" {
		if gl := i18n.GetLang(); gl != "" {
			resolvedLang, resolvedLangName = normalizeTitleTargetLang(gl, "")
		}
	}
	return AIAssistStatusResponse{
		Prefs:            prefs,
		ModelID:          model.ID,
		ModelName:        model.Name,
		ResolvedLang:     resolvedLang,
		ResolvedLangName: resolvedLangName,
		TranslatedCount:  p.titleTranslator.RestorableCount(),
		BatchStatus:      p.titleTranslator.BatchSnapshot(),
		Available:        p.IsAIAssistAvailable(),
	}
}

// HandleAIAssistPrefs serves GET/POST /gateway/ai-assist/prefs.
func (p *Proxy) HandleAIAssistPrefs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSONOK(w, p.buildAIAssistStatus(r))
	case http.MethodPost, http.MethodPatch, http.MethodPut:
		var patch AIAssistPrefsPatch
		if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&patch); err != nil {
			writeJSONError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		prev := p.AIAssistPrefs()
		updated, err := p.aiAssistPrefs.Update(patch)
		if err != nil {
			writeJSONError(w, "failed to save preferences: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// If next-prompt suggestions were disabled, immediately clear cached suggestions and notify active streams.
		if prev.NextPromptEnabled && !updated.NextPromptEnabled {
			p.clearAllNextPromptCache()
		}
		writeJSONOK(w, p.buildAIAssistStatus(r))
	default:
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func writeJSONOK(w http.ResponseWriter, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		writeJSONError(w, "marshal error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
