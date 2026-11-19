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
)

var (
	mu         sync.RWMutex
	activeLang = LangZh
)

// Init initializes the language setting according to the following precedence:
// 1. Explicit argument `override` (e.g. from --lang flag)
// 2. Environment variables: `MGY_LANG` or `MULTIGRAVITY_LANG`
// 3. System locale auto-detection (macOS / Linux / Windows)
// Defaults to LangZh if detection is indeterminate or Chinese, else LangEn.
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

// SetLang explicitly sets the active language ("zh" or "en").
func SetLang(lang string) {
	mu.Lock()
	defer mu.Unlock()
	normalized := normalizeLang(lang)
	if normalized != "" {
		activeLang = normalized
	}
}

// GetLang returns the current active language code ("zh" or "en").
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

// T returns zh if Chinese is active, or en otherwise.
func T(zh, en string) string {
	if IsZh() {
		return zh
	}
	return en
}

// Sprintf formats according to the active language's format string.
func Sprintf(zhFmt, enFmt string, a ...any) string {
	if IsZh() {
		return fmt.Sprintf(zhFmt, a...)
	}
	return fmt.Sprintf(enFmt, a...)
}

func normalizeLang(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.HasPrefix(s, "zh"):
		return LangZh
	case strings.HasPrefix(s, "en"):
		return LangEn
	default:
		return ""
	}
}
