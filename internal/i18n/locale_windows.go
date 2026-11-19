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
	for _, envKey := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		val := strings.TrimSpace(os.Getenv(envKey))
		if val == "" {
			continue
		}
		val = strings.ToLower(val)
		if strings.HasPrefix(val, "zh") {
			return LangZh
		}
		return LangEn
	}

	if procGetUserDefaultUILanguage.Find() == nil {
		r, _, _ := procGetUserDefaultUILanguage.Call()
		langID := uint16(r)
		primaryLangID := langID & 0x03FF
		// 0x04 corresponds to LANG_CHINESE in Windows header WinNls.h
		if primaryLangID == 0x04 {
			return LangZh
		}
		return LangEn
	}

	return LangZh
}
