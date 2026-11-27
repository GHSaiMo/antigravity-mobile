//go:build !windows

package i18n

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// macOSLocaleReader can be overridden in unit tests to avoid reading host defaults.
var macOSLocaleReader = readMacOSLocale

// DetectSystemLanguage detects the system UI language on POSIX systems (macOS, Linux).
// It checks standard POSIX locale variables in order: LC_ALL -> LC_MESSAGES -> LANG -> LANGUAGE.
// Neutral POSIX placeholders (e.g. C, POSIX, C.UTF-8, UTF-8) are deferred so macOS can still
// resolve the real desktop UI language via AppleLanguages (important under VS Code / tmux / LaunchAgent);
// on Linux/minimal POSIX environments where no OS UI language is available, they fall back to English.
func DetectSystemLanguage() string {
	return detectSystemLanguagePosix(runtime.GOOS == "darwin", macOSLocaleReader)
}

// DetectSystemNativeLocale detects the real system BCP-47 locale (e.g. "fr-FR", "ko-KR", "zh-TW", "ja-JP")
// without collapsing unsupported UI languages to "en".
func DetectSystemNativeLocale() string {
	return detectSystemNativeLocalePosix(runtime.GOOS == "darwin", macOSLocaleReader)
}

func detectSystemNativeLocalePosix(isDarwin bool, macReader func() string) string {
	if native, sawPosixFallback := detectEnvNativeLocale(); native != "" {
		return native
	} else {
		if isDarwin && macReader != nil {
			if macLocale := cleanNativeLocaleTag(macReader()); macLocale != "" {
				return macLocale
			}
		}
		if sawPosixFallback {
			return LangEn
		}
	}
	return LangEn
}

func detectSystemLanguagePosix(isDarwin bool, macReader func() string) string {
	if lang, sawPosixFallback := detectEnvLocale(); lang != "" {
		return lang
	} else {
		if isDarwin && macReader != nil {
			if macLocale := strings.TrimSpace(macReader()); macLocale != "" {
				if norm := normalizeLang(macLocale); norm != "" {
					return norm
				}
				return LangEn
			}
		}
		if sawPosixFallback {
			return LangEn
		}
	}
	// When nothing is set in the environment and no OS UI language is available, default to English.
	return LangEn
}

func readMacOSLocale() string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleLanguages").Output()
	if err == nil {
		if first := parseMacOSDefaultsList(string(out)); first != "" {
			return first
		}
	}
	out, err = exec.CommandContext(ctx, "defaults", "read", "-g", "AppleLocale").Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

func parseMacOSDefaultsList(out string) string {
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.Trim(strings.TrimSpace(line), "\",()")
		if trimmed == "" {
			continue
		}
		return trimmed
	}
	return ""
}
