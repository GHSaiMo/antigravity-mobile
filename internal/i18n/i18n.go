package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

const (
	LangZh = "zh"
	LangEn = "en"
	LangJa = "ja"
)

var (
	mu                 sync.RWMutex
	activeLang         = LangEn
	activeNativeLocale = LangEn
)

// Init initializes the language setting according to the following precedence:
// 1. Explicit argument `override` (e.g. from --lang flag)
// 2. Environment variables: `MGY_LANG` or `MULTIGRAVITY_LANG`
// 3. System locale auto-detection (macOS / Linux / Windows)
// UI language defaults to LangEn unless explicitly configured or detected as Chinese (zh) or Japanese (ja),
// while GetNativeLocale preserves the host's real BCP-47 locale (e.g. "fr-FR", "ko-KR", "zh-TW") for AI Assist.
func Init(override string) {
	mu.Lock()
	defer mu.Unlock()

	if raw := strings.TrimSpace(override); raw != "" && !strings.EqualFold(raw, "auto") && !strings.EqualFold(raw, "system") {
		if native := cleanNativeLocaleTag(raw); native != "" {
			activeNativeLocale = native
		} else {
			activeNativeLocale = LangEn
		}
		if norm := normalizeLang(raw); norm != "" {
			activeLang = norm
		} else {
			activeLang = LangEn
		}
		return
	}

	for _, envKey := range []string{"MGY_LANG", "MULTIGRAVITY_LANG"} {
		if raw := strings.TrimSpace(os.Getenv(envKey)); raw != "" && !strings.EqualFold(raw, "auto") && !strings.EqualFold(raw, "system") {
			if native := cleanNativeLocaleTag(raw); native != "" {
				activeNativeLocale = native
			} else {
				activeNativeLocale = LangEn
			}
			if norm := normalizeLang(raw); norm != "" {
				activeLang = norm
			} else {
				activeLang = LangEn
			}
			return
		}
	}

	native := DetectSystemNativeLocale()
	if native != "" {
		activeNativeLocale = native
	} else {
		activeNativeLocale = LangEn
	}

	detected := DetectSystemLanguage()
	if detected != "" {
		activeLang = detected
		return
	}

	activeLang = LangEn
}

// SetLang explicitly sets the active UI language ("zh", "en", or "ja") and aligns native locale.
func SetLang(lang string) {
	mu.Lock()
	defer mu.Unlock()
	normalized := normalizeLang(lang)
	if normalized != "" {
		activeLang = normalized
		if native := cleanNativeLocaleTag(lang); native != "" {
			activeNativeLocale = native
		} else {
			activeNativeLocale = normalized
		}
	}
}

// SetNativeLocale explicitly sets the host native BCP-47 locale (e.g. "fr-FR", "ko-KR", "zh-TW")
// and derives activeLang ("zh", "ja", or "en" fallback) from it.
func SetNativeLocale(locale string) {
	mu.Lock()
	defer mu.Unlock()
	if native := cleanNativeLocaleTag(locale); native != "" {
		activeNativeLocale = native
	} else {
		activeNativeLocale = LangEn
	}
	if norm := normalizeLang(locale); norm != "" {
		activeLang = norm
	} else {
		activeLang = LangEn
	}
}

// GetLang returns the current active UI language code ("zh", "en", or "ja").
func GetLang() string {
	mu.RLock()
	defer mu.RUnlock()
	return activeLang
}

// GetNativeLocale returns the preserved host BCP-47 locale (e.g. "fr-FR", "ko-KR", "zh-CN", "ja-JP", "en").
func GetNativeLocale() string {
	mu.RLock()
	defer mu.RUnlock()
	if activeNativeLocale != "" {
		return activeNativeLocale
	}
	if activeLang != "" {
		return activeLang
	}
	return LangEn
}

// IsZh reports whether Chinese is the active language.
func IsZh() bool {
	return GetLang() == LangZh
}

// IsEn reports whether English is the active language.
func IsEn() bool {
	return GetLang() == LangEn
}

// IsJa reports whether Japanese is the active language.
func IsJa() bool {
	return GetLang() == LangJa
}

// T returns zh if Chinese is active, or en otherwise.
func T(zh, en string) string {
	if IsZh() {
		return zh
	}
	return en
}

// T3 returns zh, en, or ja according to the active language.
// If ja is active and the Japanese string is empty, it falls back to English.
func T3(zh, en, ja string) string {
	switch GetLang() {
	case LangZh:
		return zh
	case LangJa:
		if ja != "" {
			return ja
		}
		return en
	default:
		return en
	}
}

// T3For returns zh, en, or ja according to an explicit language code.
// Non-zh/ja codes and missing Japanese strings fall back to English.
func T3For(lang, zh, en, ja string) string {
	switch normalizeLang(lang) {
	case LangZh:
		return zh
	case LangJa:
		if ja != "" {
			return ja
		}
		return en
	default:
		return en
	}
}

// ResolveLang resolves a caller/device language string into "zh", "en", or "ja".
// Explicit non-empty locales other than Chinese or Japanese (e.g. "ko-KR", "fr-FR")
// always fall back to English ("en") rather than the gateway host language.
// Only an empty string falls back to GetLang() (which itself defaults to "en").
func ResolveLang(lang string) string {
	trimmed := strings.TrimSpace(lang)
	if trimmed == "" {
		return GetLang()
	}
	if n := normalizeLang(trimmed); n != "" {
		return n
	}
	return LangEn
}

// Sprintf formats according to the active language's format string.
func Sprintf(zhFmt, enFmt string, a ...any) string {
	if IsZh() {
		return fmt.Sprintf(zhFmt, a...)
	}
	return fmt.Sprintf(enFmt, a...)
}

// Sprintf3 formats according to the active language's trilingual format string.
func Sprintf3(zhFmt, enFmt, jaFmt string, a ...any) string {
	return fmt.Sprintf(T3(zhFmt, enFmt, jaFmt), a...)
}

// Sprintf3For formats according to an explicit language code ("zh", "en", or "ja").
func Sprintf3For(lang, zhFmt, enFmt, jaFmt string, a ...any) string {
	return fmt.Sprintf(T3For(lang, zhFmt, enFmt, jaFmt), a...)
}

// NormalizeLang normalizes a locale string (e.g. "ja_JP.UTF-8", "zh-CN", "en_US") into "zh", "en", "ja", or "".
func NormalizeLang(s string) string {
	return normalizeLang(s)
}

func normalizeLang(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.HasPrefix(s, "zh"), s == "cn", s == "chinese":
		return LangZh
	case strings.HasPrefix(s, "ja"), s == "jp", s == "jpn", s == "japanese":
		return LangJa
	case strings.HasPrefix(s, "en"), s == "english":
		return LangEn
	default:
		return ""
	}
}

func isEncodingOnlyLocale(lower string) bool {
	return lower == "" || lower == "utf-8" || lower == "utf8"
}

func isNeutralPosixLocale(lower string) bool {
	return lower == "c" || lower == "posix" ||
		strings.HasPrefix(lower, "c.") || strings.HasPrefix(lower, "posix.")
}

// CleanLocaleTag normalizes a raw OS or HTTP locale tag (e.g. "fr_FR.UTF-8", "zh_Hant_TW", "ko-kr")
// into a clean BCP-47 tag (e.g. "fr-FR", "zh-Hant-TW", "ko-KR"), returning "" for neutral/placeholder values.
func CleanLocaleTag(raw string) string {
	return cleanNativeLocaleTag(raw)
}

func cleanNativeLocaleTag(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if idx := strings.IndexAny(s, ".@"); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	if lower == "auto" || lower == "system" || lower == "*" || isEncodingOnlyLocale(lower) || isNeutralPosixLocale(lower) {
		return ""
	}
	s = strings.ReplaceAll(s, "_", "-")
	parts := strings.Split(s, "-")
	var cleaned []string
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch {
		case i == 0:
			cleaned = append(cleaned, strings.ToLower(part))
		case len(part) == 4:
			// BCP-47 script subtag (e.g. Hans, Hant, Latn)
			cleaned = append(cleaned, strings.ToUpper(part[:1])+strings.ToLower(part[1:]))
		case len(part) == 2:
			// ISO 3166-1 alpha-2 region subtag (e.g. FR, KR, CN, TW, US, JP, BR)
			cleaned = append(cleaned, strings.ToUpper(part))
		default:
			cleaned = append(cleaned, part)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	out := strings.Join(cleaned, "-")
	if len(out) > 32 {
		return ""
	}
	return out
}

// detectEnvNativeLocale inspects standard POSIX locale environment variables in priority order
// and preserves the raw BCP-47 locale tag (e.g. "fr-FR", "ko-KR", "zh-TW", "ja-JP", "en-US").
func detectEnvNativeLocale() (nativeTag string, sawPosixFallback bool) {
	for _, envKey := range []string{"LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
		raw := strings.TrimSpace(os.Getenv(envKey))
		if raw == "" {
			continue
		}
		for _, part := range strings.Split(raw, ":") {
			lower := strings.ToLower(strings.TrimSpace(part))
			if isEncodingOnlyLocale(lower) {
				continue
			}
			if isNeutralPosixLocale(lower) {
				sawPosixFallback = true
				continue
			}
			if tag := cleanNativeLocaleTag(part); tag != "" {
				return tag, false
			}
		}
	}
	return "", sawPosixFallback
}

// detectEnvLocale inspects standard POSIX locale environment variables in priority order.
// Neutral encoding/POSIX placeholders (UTF-8, C, POSIX, C.UTF-8) do not short-circuit
// before remaining env vars or OS-native UI language APIs (e.g. macOS AppleLanguages,
// Windows GetUserDefaultUILanguage), because IDE terminals (VS Code, tmux, Git Bash)
// frequently inject LANG=C.UTF-8 even on Chinese/Japanese desktops.
func detectEnvLocale() (lang string, sawPosixFallback bool) {
	native, sawPosix := detectEnvNativeLocale()
	if native == "" {
		return "", sawPosix
	}
	if norm := normalizeLang(native); norm != "" {
		return norm, false
	}
	// Any other explicit non-Chinese/non-Japanese locale defaults to English for UI chrome.
	return LangEn, false
}
