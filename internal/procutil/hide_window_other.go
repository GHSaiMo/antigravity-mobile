//go:build !windows

package procutil

import "os/exec"

// HideWindow is a no-op on non-Windows platforms. Returns cmd for convenient chaining.
func HideWindow(cmd *exec.Cmd) *exec.Cmd {
	return cmd
}
