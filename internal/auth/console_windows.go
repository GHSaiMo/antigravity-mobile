//go:build windows

package auth

import (
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var (
	consoleVTPUnsupported atomic.Bool
	consoleLegacyCJKFont  atomic.Bool
)

func consoleSupportsANSI() bool {
	return !consoleVTPUnsupported.Load()
}

func consoleUsesDistinctSpaceBG() bool {
	return consoleSupportsANSI()
}

func consoleHasLegacyCJKFont() bool {
	return consoleLegacyCJKFont.Load()
}

type consoleCoord struct {
	X int16
	Y int16
}

// consoleFontInfoEx mirrors Win32 CONSOLE_FONT_INFOEX (84 bytes on 32/64-bit Windows).
type consoleFontInfoEx struct {
	cbSize     uint32
	nFont      uint32
	dwFontSize consoleCoord
	fontFamily uint32
	fontWeight uint32
	faceName   [32]uint16
}

func isModernWindowsTerminalEnv() bool {
	for _, key := range []string{"WT_SESSION", "TERM_PROGRAM", "ConEmuPID", "ANSICON"} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

func detectLegacyCJKConhostFont(kernel32 *syscall.LazyDLL, stdOutHandle uintptr) bool {
	if isModernWindowsTerminalEnv() {
		return false
	}
	getCurrentConsoleFontEx := kernel32.NewProc("GetCurrentConsoleFontEx")
	if err := getCurrentConsoleFontEx.Find(); err != nil {
		return false
	}
	var info consoleFontInfoEx
	info.cbSize = uint32(unsafe.Sizeof(info))
	r, _, _ := getCurrentConsoleFontEx.Call(stdOutHandle, 0, uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return false
	}
	face := syscall.UTF16ToString(info.faceName[:])
	return isLegacyCJKConsoleFontName(face)
}

func initConsoleOS() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setConsoleOutputCP := kernel32.NewProc("SetConsoleOutputCP")
	setConsoleCP := kernel32.NewProc("SetConsoleCP")

	// Set console code page to UTF-8 (65001)
	_, _, _ = setConsoleOutputCP.Call(65001)
	_, _, _ = setConsoleCP.Call(65001)

	// Enable Virtual Terminal Processing (ENABLE_VIRTUAL_TERMINAL_PROCESSING = 0x0004)
	// on standard output and standard error so ANSI colors, QR codes, and UTF-8 render properly.
	getStdHandle := kernel32.NewProc("GetStdHandle")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")

	const (
		stdOutputHandle = uint32(0xFFFFFFF5) // (DWORD)-11
		stdErrorHandle  = uint32(0xFFFFFFF4) // (DWORD)-12
		enableVTP       = 0x0004
	)

	for _, handleID := range []uint32{stdOutputHandle, stdErrorHandle} {
		h, _, _ := getStdHandle.Call(uintptr(handleID))
		if h != 0 && h != uintptr(syscall.InvalidHandle) {
			var mode uint32
			r, _, _ := getConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode)))
			if r != 0 {
				ok, _, _ := setConsoleMode.Call(h, uintptr(mode|enableVTP))
				if handleID == stdOutputHandle {
					consoleVTPUnsupported.Store(ok == 0)
					consoleLegacyCJKFont.Store(detectLegacyCJKConhostFont(kernel32, h))
				}
			}
		}
	}
}
