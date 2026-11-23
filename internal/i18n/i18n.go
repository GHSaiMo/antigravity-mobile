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
	activeLang = LangZh
)

// Init initializes the language setting according to the following precedence:
// 1. Explicit argument `override` (e.g. from --lang flag)
// 2. Environment variables: `MGY_LANG` or `MULTIGRAVITY_LANG`
// 3. System locale auto-detection (macOS / Linux / Windows)
// Defaults to LangZh if detection is indeterminate or Chinese, else LangJa or LangEn.
func Init(override string) {
	mu.Lock()
	defer mu.Unlock()

	override = normalizeLang(override)
	if override != "" {
		activeLang = override
		return
	}

	for _, envKey := range []string{"MGY_LANG", "MULTIGRAVITY_LANG"} {
		if val := normalizeLang(os.Getenv(envKey)); val != "" {
			activeLang = val
			return
		}
	}

	detected := DetectSystemLanguage()
	if detected != "" {
		activeLang = detected
		return
	}

	activeLang = LangZh
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
func T3(zh, en, ja string) string {
	switch GetLang() {
	case LangZh:
		return zh
	case LangJa:
		return ja
	default:
		return en
	}
}

// T3For returns zh, en, or ja according to an explicit language code.
func T3For(lang, zh, en, ja string) string {
	switch normalizeLang(lang) {
	case LangZh:
		return zh
	case LangJa:
		return ja
	default:
		return en
	}
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
