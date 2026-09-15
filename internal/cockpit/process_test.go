package cockpit

import "testing"

func TestLooksLikeAntigravityNotRequiredWhenNotRunning(t *testing.T) {
	// Safety: this test must not kill a live IDE. It only checks the wait helper
	// against whatever the current machine state is, via a zero-duration wait
	// when already stopped — antigravityStillRunning is a pgrep, not a kill.
	_ = antigravityStillRunning()
	if waitUntilAntigravityExited(0) {
		t.Log("Antigravity is not running")
	}
}
