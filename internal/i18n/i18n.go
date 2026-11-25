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
	mu         sync.RWMutex
	activeLang = LangEn
)

// Init initializes the language setting according to the following precedence:
// 1. Explicit argument `override` (e.g. from --lang flag)
// 2. Environment variables: `MGY_LANG` or `MULTIGRAVITY_LANG`
// 3. System locale auto-detection (macOS / Linux / Windows)
// Defaults to LangEn unless explicitly configured or detected as Chinese (zh) or Japanese (ja).
func Init(override string) {
	mu.Lock()
	defer mu.Unlock()

	if raw := strings.TrimSpace(override); raw != "" && !strings.EqualFold(raw, "auto") && !strings.EqualFold(raw, "system") {
		if norm := normalizeLang(raw); norm != "" {
			activeLang = norm
		} else {
			activeLang = LangEn
		}
		return
	}

	for _, envKey := range []string{"MGY_LANG", "MULTIGRAVITY_LANG"} {
		if raw := strings.TrimSpace(os.Getenv(envKey)); raw != "" && !strings.EqualFold(raw, "auto") && !strings.EqualFold(raw, "system") {
			if norm := normalizeLang(raw); norm != "" {
				activeLang = norm
			} else {
				activeLang = LangEn
			}
			return
		}
	}

	detected := DetectSystemLanguage()
	if detected != "" {
		activeLang = detected
		return
	}

	activeLang = LangEn
}

// SetLang explicitly sets the active language ("zh", "en", or "ja").
func SetLang(lang string) {
	mu.Lock()
	defer mu.Unlock()
	normalized := normalizeLang(lang)
	if normalized != "" {
		activeLang = normalized
	}
}

// GetLang returns the current active language code ("zh", "en", or "ja").
func GetLang() string {
	mu.RLock()
	defer mu.RUnlock()
	return activeLang
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

// detectEnvLocale inspects standard POSIX locale environment variables in priority order.
// Neutral encoding/POSIX placeholders (UTF-8, C, POSIX, C.UTF-8) do not short-circuit
// before remaining env vars or OS-native UI language APIs (e.g. macOS AppleLanguages,
// Windows GetUserDefaultUILanguage), because IDE terminals (VS Code, tmux, Git Bash)
// frequently inject LANG=C.UTF-8 even on Chinese/Japanese desktops.
func detectEnvLocale() (lang string, sawPosixFallback bool) {
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
			if norm := normalizeLang(lower); norm != "" {
				return norm, false
			}
			// Any other explicit non-Chinese/non-Japanese locale defaults to English.
			return LangEn, false
		}
	}
	return "", sawPosixFallback
}
