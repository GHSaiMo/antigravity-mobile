//go:build !windows

package cockpit

import "os/exec"

func setHideWindow(cmd *exec.Cmd) {
	// No-op on POSIX/Darwin platforms
}
