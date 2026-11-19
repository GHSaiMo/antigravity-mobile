//go:build !windows

package i18n

import (
	"os"
	"strings"
)

// DetectSystemLanguage detects the system UI language on POSIX systems (macOS, Linux).
// It checks standard POSIX locale variables in order: LC_ALL -> LC_MESSAGES -> LANG.
func DetectSystemLanguage() string {
	for _, envKey := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		val := strings.TrimSpace(os.Getenv(envKey))
		if val == "" {
			continue
		}
		val = strings.ToLower(val)
		if strings.HasPrefix(val, "zh") {
			return LangZh
		}
		// If explicitly set to English, return LangEn
		if strings.HasPrefix(val, "en") {
			return LangEn
		}
		// For standard POSIX / C minimal environments, default to LangEn for international safety
		if val == "c" || val == "posix" {
			return LangEn
		}
		// Any other non-Chinese language environment defaults to English
		return LangEn
	}
	// When nothing is set in the environment, default to Chinese on localized systems
	return LangZh
}
