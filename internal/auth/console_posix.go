//go:build !windows

package auth

func initConsoleOS() {
	// No-op on POSIX systems (UTF-8 is standard)
}
