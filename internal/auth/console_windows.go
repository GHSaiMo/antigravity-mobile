//go:build windows

package auth

import (
	"syscall"
	"unsafe"
)

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
				_, _, _ = setConsoleMode.Call(h, uintptr(mode|enableVTP))
			}
		}
	}
}
