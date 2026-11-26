package cockpit

import (
	"errors"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

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

func TestParseLinuxAntigravityPIDs(t *testing.T) {
	psOut := strings.Join([]string{
		"1001 /usr/local/bin/agy remote-control serve",
		"1002 /usr/local/bin/agy --csrf_token=5aacbcc0-cf40-4b94-8f17-986485a8d0d5",
		"1003 /usr/bin/proxychains4 -q /usr/local/bin/antigravity-cli remote-control serve",
		"1004 /opt/antigravity/language_server_linux_x64 --standalone --http_server_port=50005",
		"1005 /opt/Antigravity/antigravity --no-sandbox",
		"1006 /usr/local/bin/agy --version",
		"1007 /usr/local/bin/agy status",
		"1008 /usr/bin/python3 helper.py agy remote-control serve",
		"1009 grep agy remote-control serve",
		"9999 /usr/local/bin/agy remote-control serve",
	}, "\n")

	got := parseLinuxAntigravityPIDs(psOut, 9999)
	want := []int{1001, 1002, 1003, 1004, 1005}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseLinuxAntigravityPIDs() = %v, want %v", got, want)
	}
}

func TestLaunchLinuxAntigravitySystemdServices(t *testing.T) {
	origRunner := linuxSystemctlRunner
	origLookPath := linuxLookPath
	origStartLS := linuxStartStandaloneLS
	t.Cleanup(func() {
		linuxSystemctlRunner = origRunner
		linuxLookPath = origLookPath
		linuxStartStandaloneLS = origStartLS
		clearStoppedLinuxServices()
	})

	t.Run("restarts_cli_daemon_when_only_cli_unit_exists", func(t *testing.T) {
		clearStoppedLinuxServices()
		var restarted []string
		linuxSystemctlRunner = func(_ time.Duration, args ...string) error {
			if len(args) >= 2 && args[0] == "cat" && args[1] == "antigravity-cli-daemon.service" {
				return nil
			}
			if len(args) >= 2 && args[0] == "restart" {
				restarted = append(restarted, args[1])
				return nil
			}
			return errors.New("not found")
		}
		linuxLookPath = func(string) (string, error) { return "", errors.New("not found") }

		if err := launchLinuxAntigravity(); err != nil {
			t.Fatalf("launchLinuxAntigravity() error = %v", err)
		}
		want := []string{"antigravity-cli-daemon.service"}
		if !reflect.DeepEqual(restarted, want) {
			t.Fatalf("restarted = %v, want %v", restarted, want)
		}
	})

	t.Run("prefers_previously_stopped_service_when_both_units_exist", func(t *testing.T) {
		clearStoppedLinuxServices()
		recordStoppedLinuxServices([]string{"antigravity-cli-daemon.service"})
		var restarted []string
		linuxSystemctlRunner = func(_ time.Duration, args ...string) error {
			if len(args) >= 2 && args[0] == "cat" {
				return nil
			}
			if len(args) >= 2 && args[0] == "restart" {
				restarted = append(restarted, args[1])
				return nil
			}
			return nil
		}

		if err := launchLinuxAntigravity(); err != nil {
			t.Fatalf("launchLinuxAntigravity() error = %v", err)
		}
		want := []string{"antigravity-cli-daemon.service"}
		if !reflect.DeepEqual(restarted, want) {
			t.Fatalf("restarted = %v, want %v", restarted, want)
		}
	})

	t.Run("prefers_enabled_unit_when_both_units_exist_cold", func(t *testing.T) {
		clearStoppedLinuxServices()
		var restarted []string
		linuxSystemctlRunner = func(_ time.Duration, args ...string) error {
			if len(args) >= 2 && args[0] == "cat" {
				return nil
			}
			if len(args) >= 3 && args[0] == "is-enabled" && args[2] == "antigravity-ls.service" {
				return nil
			}
			if len(args) >= 2 && args[0] == "restart" {
				restarted = append(restarted, args[1])
				return nil
			}
			return errors.New("inactive")
		}

		if err := launchLinuxAntigravity(); err != nil {
			t.Fatalf("launchLinuxAntigravity() error = %v", err)
		}
		want := []string{"antigravity-ls.service"}
		if !reflect.DeepEqual(restarted, want) {
			t.Fatalf("restarted = %v, want %v", restarted, want)
		}
	})

	t.Run("falls_back_to_standalone_language_server", func(t *testing.T) {
		clearStoppedLinuxServices()
		linuxSystemctlRunner = func(_ time.Duration, _ ...string) error {
			return errors.New("no systemd")
		}
		linuxLookPath = func(file string) (string, error) {
			if file == "language_server" {
				return "/usr/local/bin/language_server", nil
			}
			return "", errors.New("not found")
		}
		var launched string
		linuxStartStandaloneLS = func(lsPath string) error {
			launched = lsPath
			return nil
		}

		if err := launchLinuxAntigravity(); err != nil {
			t.Fatalf("launchLinuxAntigravity() error = %v", err)
		}
		if launched != "/usr/local/bin/language_server" {
			t.Fatalf("launched = %q, want /usr/local/bin/language_server", launched)
		}
	})
}

func TestQuitAndRelaunchLinuxCliDaemonService(t *testing.T) {
	origRunner := linuxSystemctlRunner
	origFindPIDs := linuxFindPIDsFn
	origWait := linuxWaitExitedFn
	origSignal := linuxSignalFn
	t.Cleanup(func() {
		linuxSystemctlRunner = origRunner
		linuxFindPIDsFn = origFindPIDs
		linuxWaitExitedFn = origWait
		linuxSignalFn = origSignal
		clearStoppedLinuxServices()
	})

	clearStoppedLinuxServices()
	cliActive := true
	var calls []string
	var signals []syscall.Signal

	linuxSystemctlRunner = func(_ time.Duration, args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		switch {
		case len(args) >= 3 && args[0] == "is-active" && args[2] == "antigravity-cli-daemon.service":
			if cliActive {
				return nil
			}
			return errors.New("inactive")
		case len(args) >= 2 && args[0] == "stop" && args[1] == "antigravity-cli-daemon.service":
			cliActive = false
			return nil
		case len(args) >= 2 && args[0] == "restart" && args[1] == "antigravity-cli-daemon.service":
			cliActive = true
			return nil
		default:
			return errors.New("not active")
		}
	}
	linuxFindPIDsFn = func() []int { return nil }
	linuxWaitExitedFn = func(time.Duration) bool { return !cliActive }
	linuxSignalFn = func(sig syscall.Signal) { signals = append(signals, sig) }

	if err := quitLinuxAntigravity(); err != nil {
		t.Fatalf("quitLinuxAntigravity() error = %v", err)
	}
	if cliActive {
		t.Fatalf("expected antigravity-cli-daemon.service to be stopped")
	}
	if len(signals) != 0 {
		t.Fatalf("expected no direct signals when systemd stop succeeded, got %v", signals)
	}

	if err := launchLinuxAntigravity(); err != nil {
		t.Fatalf("launchLinuxAntigravity() error = %v", err)
	}
	if !cliActive {
		t.Fatalf("expected antigravity-cli-daemon.service to be restarted")
	}

	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "stop antigravity-cli-daemon.service") {
		t.Fatalf("expected stop call in %q", joined)
	}
	if !strings.Contains(joined, "restart antigravity-cli-daemon.service") {
		t.Fatalf("expected restart call in %q", joined)
	}
}
