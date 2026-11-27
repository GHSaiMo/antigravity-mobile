//go:build windows

package i18n

import (
	"syscall"
	"unsafe"
)

var (
	modKernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetUserDefaultUILanguage = modKernel32.NewProc("GetUserDefaultUILanguage")
	procGetUserDefaultLocaleName = modKernel32.NewProc("GetUserDefaultLocaleName")
)

// DetectSystemNativeLocale detects the real system BCP-47 locale on Windows (e.g. "fr-FR", "ko-KR", "zh-TW").
func DetectSystemNativeLocale() string {
	if native, _ := detectEnvNativeLocale(); native != "" {
		return native
	}
	if procGetUserDefaultLocaleName.Find() == nil {
		// LOCALE_NAME_MAX_LENGTH is 85 in WinNls.h.
		var buf [85]uint16
		r, _, _ := procGetUserDefaultLocaleName.Call(
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
		)
		if r > 0 {
			if tag := cleanNativeLocaleTag(syscall.UTF16ToString(buf[:])); tag != "" {
				return tag
			}
		}
	}
	return DetectSystemLanguage()
}

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

	return LangEn
}
