package proxy

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"antigravity-mobile/internal/i18n"
)

// DesktopLangCookieName stores an explicit language preference ("zh" | "en" | "ja")
// for the Desktop Web Console. When absent or "auto", the gateway negotiates from Accept-Language.
const DesktopLangCookieName = "mgy_lang"

// ResolveDesktopLangPref returns the explicit Desktop Web Console language preference
// from ?lang= or the mgy_lang cookie ("zh", "en", "ja", or "auto").
func ResolveDesktopLangPref(r *http.Request) string {
	if r == nil {
		return "auto"
	}
	if q := strings.TrimSpace(r.URL.Query().Get("lang")); q != "" {
		if strings.EqualFold(q, "auto") || strings.EqualFold(q, "system") {
			return "auto"
		}
		if norm := i18n.NormalizeLang(q); norm != "" {
			return norm
		}
	}
	if c, err := r.Cookie(DesktopLangCookieName); err == nil && c != nil {
		v := strings.TrimSpace(c.Value)
		if strings.EqualFold(v, "auto") || strings.EqualFold(v, "system") {
			return "auto"
		}
		if norm := i18n.NormalizeLang(v); norm != "" {
			return norm
		}
	}
	return "auto"
}

// ResolveDesktopLang resolves the effective Desktop Web Console language ("zh", "en", or "ja")
// using precedence: ?lang= > mgy_lang cookie > browser Accept-Language > gateway i18n.GetLang().
func ResolveDesktopLang(r *http.Request) string {
	if r == nil {
		return i18n.GetLang()
	}
	if q := strings.TrimSpace(r.URL.Query().Get("lang")); q != "" {
		if !strings.EqualFold(q, "auto") && !strings.EqualFold(q, "system") {
			if norm := i18n.NormalizeLang(q); norm != "" {
				return norm
			}
			return i18n.LangEn
		} else {
			if al := parseAcceptLanguage(r.Header.Get("Accept-Language")); al != "" {
				return al
			}
			return i18n.GetLang()
		}
	}
	if c, err := r.Cookie(DesktopLangCookieName); err == nil && c != nil {
		v := strings.TrimSpace(c.Value)
		if v != "" && !strings.EqualFold(v, "auto") && !strings.EqualFold(v, "system") {
			if norm := i18n.NormalizeLang(v); norm != "" {
				return norm
			}
			return i18n.LangEn
		}
	}
	if al := parseAcceptLanguage(r.Header.Get("Accept-Language")); al != "" {
		return al
	}
	return i18n.GetLang()
}

// parseAcceptLanguage parses an HTTP Accept-Language header and maps the highest-priority
// language tag into "zh", "ja", or "en" (any other non-wildcard tag maps to "en").
func parseAcceptLanguage(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	type candidate struct {
		tag string
		q   float64
		idx int
	}
	var cands []candidate
	for idx, rawPart := range strings.Split(header, ",") {
		part := strings.TrimSpace(rawPart)
		if part == "" {
			continue
		}
		segs := strings.Split(part, ";")
		tag := strings.ToLower(strings.TrimSpace(segs[0]))
		if tag == "" || tag == "*" {
			continue
		}
		q := 1.0
		for _, param := range segs[1:] {
			param = strings.TrimSpace(param)
			if strings.HasPrefix(strings.ToLower(param), "q=") {
				if parsed, err := strconv.ParseFloat(strings.TrimSpace(param[2:]), 64); err == nil {
					q = parsed
				}
			}
		}
		if q <= 0 {
			continue
		}
		cands = append(cands, candidate{tag: tag, q: q, idx: idx})
	}
	if len(cands) == 0 {
		return ""
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].q != cands[j].q {
			return cands[i].q > cands[j].q
		}
		return cands[i].idx < cands[j].idx
	})
	top := cands[0].tag
	if norm := i18n.NormalizeLang(top); norm != "" {
		return norm
	}
	return i18n.LangEn
}
