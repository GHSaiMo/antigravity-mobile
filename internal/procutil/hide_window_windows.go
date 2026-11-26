//go:build windows

package procutil

import (
	"os/exec"
	"syscall"
)

// createNoWindow is the Win32 CREATE_NO_WINDOW process creation flag (0x08000000).
// Combined with HideWindow, it prevents console-subsystem child processes
// (powershell.exe, netstat.exe, git.exe, cloudflared.exe, etc.) from flashing
// transient console windows on the user's desktop when mgy runs in the background.
const createNoWindow = 0x08000000

// HideWindow configures cmd so spawning child processes on Windows does not
// display a console window. Returns cmd for convenient chaining.
func HideWindow(cmd *exec.Cmd) *exec.Cmd {
	if cmd == nil {
		return nil
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
	return cmd
}
