package procutil

import (
	"os/exec"
	"testing"
)

func TestHideWindowNilSafe(t *testing.T) {
	if got := HideWindow(nil); got != nil {
		t.Fatalf("HideWindow(nil) = %v, want nil", got)
	}
}

func TestHideWindowReturnsSameCmd(t *testing.T) {
	cmd := exec.Command("echo", "ok")
	if got := HideWindow(cmd); got != cmd {
		t.Fatalf("HideWindow returned different *exec.Cmd pointer")
	}
}
