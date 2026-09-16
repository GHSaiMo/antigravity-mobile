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

func TestQuitAppDarwinAllowlist(t *testing.T) {
	// Whitelisted apps should pass check (even if osascript fails or succeeds depending on runtime)
	if !allowedQuitAppNames[antigravityAppName] {
		t.Errorf("expected %q to be in allowedQuitAppNames", antigravityAppName)
	}
	if !allowedQuitAppNames[antigravityIDEAppName] {
		t.Errorf("expected %q to be in allowedQuitAppNames", antigravityIDEAppName)
	}

	// Malicious / injected / non-whitelisted app names must be rejected with error
	dangerousNames := []string{
		`Antigravity" with administrator privileges --`,
		`Finder" and delay 5 --`,
		`Terminal`,
		`System Events`,
		`Calculator`,
		``,
	}
	for _, dangerous := range dangerousNames {
		err := quitAppDarwin(dangerous)
		if err == nil {
			t.Errorf("expected error for non-whitelisted app name %q, got nil", dangerous)
		}
	}
}
