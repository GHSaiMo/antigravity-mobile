//go:build windows

package i18n

import (
	"os"
	"strings"
	"syscall"
)

var (
	modKernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetUserDefaultUILanguage = modKernel32.NewProc("GetUserDefaultUILanguage")
)

// DetectSystemLanguage detects the system UI language on Windows systems.
// It checks environment variables first, then falls back to the Win32 GetUserDefaultUILanguage API.
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
		if norm := normalizeLang(lower); norm != "" {
			return norm
		}
		return LangEn
	}

	if procGetUserDefaultUILanguage.Find() == nil {
		r, _, _ := procGetUserDefaultUILanguage.Call()
		langID := uint16(r)
		primaryLangID := langID & 0x03FF
		// 0x04 = LANG_CHINESE, 0x11 = LANG_JAPANESE in Windows WinNls.h
		switch primaryLangID {
		case 0x04:
			return LangZh
		case 0x11:
			return LangJa
		default:
			return LangEn
		}
	}

	return LangZh
}
