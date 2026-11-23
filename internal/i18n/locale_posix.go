//go:build !windows

package i18n

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// macOSLocaleReader can be overridden in unit tests to avoid reading host defaults.
var macOSLocaleReader = readMacOSLocale

// DetectSystemLanguage detects the system UI language on POSIX systems (macOS, Linux).
// It checks standard POSIX locale variables in order: LC_ALL -> LC_MESSAGES -> LANG -> LANGUAGE,
// falling back to macOS AppleLanguages when launched without POSIX locale env vars (e.g. LaunchAgent).
func DetectSystemLanguage() string {
	for _, envKey := range []string{"LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
		val := strings.TrimSpace(os.Getenv(envKey))
		if val == "" {
			continue
		}
		if idx := strings.IndexByte(val, ':'); idx >= 0 {
			val = strings.TrimSpace(val[:idx])
		}
		lower := strings.ToLower(val)
		if lower == "" || lower == "utf-8" || lower == "utf8" {
			continue
		}
		if lower == "c" || lower == "posix" || strings.HasPrefix(lower, "c.") {
			return LangEn
		}
		if norm := normalizeLang(lower); norm != "" {
			return norm
		}
		// Any other non-Chinese/non-Japanese locale defaults to English.
		return LangEn
	}
	if runtime.GOOS == "darwin" && macOSLocaleReader != nil {
		if norm := normalizeLang(macOSLocaleReader()); norm != "" {
			return norm
		}
	}
	// When nothing is set in the environment, default to Chinese on localized systems
	return LangZh
}

func readMacOSLocale() string {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleLanguages").Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			trimmed := strings.Trim(strings.TrimSpace(line), "\",()")
			if trimmed == "" {
				continue
			}
			return trimmed
		}
	}
	out, err = exec.CommandContext(ctx, "defaults", "read", "-g", "AppleLocale").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}
