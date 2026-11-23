//go:build windows

package i18n

import (
	"syscall"
)

var (
	modKernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetUserDefaultUILanguage = modKernel32.NewProc("GetUserDefaultUILanguage")
)

// DetectSystemLanguage detects the system UI language on Windows systems.
// It checks explicit locale environment variables first, deferring neutral POSIX placeholders
// (such as LANG=C.UTF-8 injected by Git Bash / VS Code) to the Win32 GetUserDefaultUILanguage API.
func DetectSystemLanguage() string {
	lang, sawPosixFallback := detectEnvLocale()
	if lang != "" {
		return lang
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

	if sawPosixFallback {
		return LangEn
	}

	return LangZh
}
