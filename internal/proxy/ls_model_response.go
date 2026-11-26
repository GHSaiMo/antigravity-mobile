package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// AIAssistModelInfo describes the lightweight model used for stateless GetModelResponse calls.
type AIAssistModelInfo struct {
	ID   string `json:"id"`
	Enum string `json:"enum"`
	Name string `json:"name"`
}

// preferredAIAssistModels orders candidate lightweight models from fastest/cheapest to heavier fallback.
var preferredAIAssistModels = []AIAssistModelInfo{
	{ID: "gemini-3.5-flash-lite", Enum: "MODEL_PLACEHOLDER_M198", Name: "Gemini 3.5 Flash Lite"},
	{ID: "gemini-3.1-flash-lite", Enum: "MODEL_PLACEHOLDER_M50", Name: "Gemini 3.1 Flash Lite"},
	{ID: "gemini-3-flash", Enum: "MODEL_PLACEHOLDER_M18", Name: "Gemini 3 Flash"},
	{ID: "gemini-3.8-flash-low", Enum: "MODEL_PLACEHOLDER_M320", Name: "Gemini 3.8 Flash (Low)"},
	{ID: "gemini-4-hydrogen-low", Enum: "MODEL_PLACEHOLDER_M527", Name: "Gemini 4 Hydrogen (Low)"},
}

// preferredAIAssistFallbackModels orders stronger non-Lite fallback models used when a Flash Lite
// model echoes untranslated source text or returns malformed batch JSON.
var preferredAIAssistFallbackModels = []AIAssistModelInfo{
	{ID: "gemini-4-hydrogen-low", Enum: "MODEL_PLACEHOLDER_M527", Name: "Gemini 4 Hydrogen (Low)"},
	{ID: "gemini-3.8-flash-low", Enum: "MODEL_PLACEHOLDER_M320", Name: "Gemini 3.8 Flash (Low)"},
	{ID: "gemini-3-flash", Enum: "MODEL_PLACEHOLDER_M18", Name: "Gemini 3 Flash"},
}

var (
	// Bound concurrent stateless GetModelResponse calls so bursts never saturate the local language_server.
	lsModelResponseSem = make(chan struct{}, 2)

	reBearerSecret = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._~+/=-]{12,}`)
	reTokenSecret  = regexp.MustCompile(`\b(?:ya29\.[A-Za-z0-9_-]{16,}|AIza[A-Za-z0-9_-]{20,}|sk-[A-Za-z0-9_-]{16,}|gh[pousr]_[A-Za-z0-9]{16,}|xox[baprs]-[A-Za-z0-9-]{12,}|1//[A-Za-z0-9_-]{16,})\b`)
	reEmailSecret  = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
	reControlChars = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
)

// redactSensitiveForAI strips common credentials, tokens, emails, and control characters
// before sending prompt excerpts to GetModelResponse.
func redactSensitiveForAI(raw string) string {
	if raw == "" {
		return ""
	}
	s := strings.ReplaceAll(raw, "\r\n", "\n")
	s = reControlChars.ReplaceAllString(s, " ")
	s = reBearerSecret.ReplaceAllString(s, "Bearer [REDACTED]")
	s = reTokenSecret.ReplaceAllString(s, "[REDACTED_SECRET]")
	s = reEmailSecret.ReplaceAllString(s, "[REDACTED_EMAIL]")
	return strings.TrimSpace(s)
}

// ResolveAIAssistModel selects the lightest live Flash model available from GetAvailableModels,
// falling back to the built-in Gemini 3.5 Flash Lite enum when the catalog has not been fetched yet.
func (p *Proxy) ResolveAIAssistModel(ctx context.Context) AIAssistModelInfo {
	// Ensure liveModels has had a chance to populate if an upstream is active.
	if p != nil {
		port, _ := p.ActiveUpstream()
		if port > 0 {
			liveModels.mu.RLock()
			hasCatalog := len(liveModels.byID) > 0
			liveModels.mu.RUnlock()
			if !hasCatalog {
				rCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				p.ensureLiveModels(rCtx)
				cancel()
			}
		}
	}

	liveModels.mu.RLock()
	defer liveModels.mu.RUnlock()
	if len(liveModels.byID) > 0 {
		for _, cand := range preferredAIAssistModels {
			if opt, ok := liveModels.byID[strings.ToLower(cand.ID)]; ok && strings.TrimSpace(opt.Enum) != "" {
				if isUnavailableModelTag(opt.Tag) {
					continue
				}
				name := strings.TrimSpace(opt.Name)
				if name == "" {
					name = cand.Name
				}
				return AIAssistModelInfo{
					ID:   opt.ID,
					Enum: opt.Enum,
					Name: name,
				}
			}
		}
		// Secondary dynamic scan: any non-unavailable Gemini Flash Lite / Flash Low model in catalog.
		for _, opt := range liveModels.byID {
			if opt.Provider != "gemini" || strings.TrimSpace(opt.Enum) == "" || isUnavailableModelTag(opt.Tag) || isRetiredEnum(opt.Enum) {
				continue
			}
			lower := strings.ToLower(opt.ID)
			if strings.Contains(lower, "flash-lite") && !strings.Contains(lower, "tab_") && !strings.Contains(lower, "image") {
				name := strings.TrimSpace(opt.Name)
				if name == "" {
					name = opt.ID
				}
				return AIAssistModelInfo{ID: opt.ID, Enum: opt.Enum, Name: name}
			}
		}
	}
	return preferredAIAssistModels[0]
}

// ResolveAIAssistFallbackModel picks a stronger non-Lite model (distinct from excludeEnum) for
// automatic retry when the primary Flash Lite model echoes untranslated input or outputs bad JSON.
func (p *Proxy) ResolveAIAssistFallbackModel(ctx context.Context, excludeEnum string) *AIAssistModelInfo {
	liveModels.mu.RLock()
	defer liveModels.mu.RUnlock()
	if len(liveModels.byID) > 0 {
		for _, cand := range preferredAIAssistFallbackModels {
			if cand.Enum == excludeEnum {
				continue
			}
			if opt, ok := liveModels.byID[strings.ToLower(cand.ID)]; ok && strings.TrimSpace(opt.Enum) != "" {
				if isUnavailableModelTag(opt.Tag) || opt.Enum == excludeEnum {
					continue
				}
				name := strings.TrimSpace(opt.Name)
				if name == "" {
					name = cand.Name
				}
				return &AIAssistModelInfo{
					ID:   opt.ID,
					Enum: opt.Enum,
					Name: name,
				}
			}
		}
	}
	for _, cand := range preferredAIAssistFallbackModels {
		if cand.Enum != excludeEnum {
			cp := cand
			return &cp
		}
	}
	return nil
}

// IsAIAssistAvailable reports whether the connected language_server supports GetModelResponse.
func (p *Proxy) IsAIAssistAvailable() bool {
	if p == nil {
		return false
	}
	c := p.currentCompatibility(nil)
	if c == nil || !c.Checked {
		return true
	}
	for _, u := range c.Unavailable {
		if u == "ai_assist" {
			return false
		}
	}
	return true
}

// CallLSModelResponse invokes the local Antigravity language_server's stateless GetModelResponse RPC.
// It runs without creating any cascade trajectory or invoking tools.
func (p *Proxy) CallLSModelResponse(ctx context.Context, prompt string, modelOverride *AIAssistModelInfo) (string, AIAssistModelInfo, error) {
	model := preferredAIAssistModels[0]
	if modelOverride != nil && strings.TrimSpace(modelOverride.Enum) != "" {
		model = *modelOverride
	} else {
		model = p.ResolveAIAssistModel(ctx)
	}

	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", model, fmt.Errorf("empty prompt")
	}

	if p != nil && p.lsModelResponseHook != nil {
		text, err := p.lsModelResponseHook(ctx, prompt, model.Enum)
		return strings.TrimSpace(text), model, err
	}

	if p == nil {
		return "", model, fmt.Errorf("proxy is nil")
	}

	select {
	case lsModelResponseSem <- struct{}{}:
		defer func() { <-lsModelResponseSem }()
	case <-ctx.Done():
		return "", model, ctx.Err()
	}

	reqBody := map[string]interface{}{
		"prompt": prompt,
		"model":  model.Enum,
	}
	rawResp, status, err := p.callUpstreamRPCLong(ctx, "GetModelResponse", reqBody)
	if err != nil {
		return "", model, err
	}
	if status != http.StatusOK {
		return "", model, fmt.Errorf("GetModelResponse HTTP %d: %s", status, strings.TrimSpace(string(rawResp)))
	}

	var parsed struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(rawResp, &parsed); err != nil {
		return "", model, fmt.Errorf("parse GetModelResponse: %w", err)
	}
	return strings.TrimSpace(parsed.Response), model, nil
}
