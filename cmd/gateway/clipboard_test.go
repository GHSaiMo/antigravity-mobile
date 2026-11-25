package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestSelectClipboardCommand(t *testing.T) {
	mockLookPath := func(avail ...string) func(string) (string, error) {
		set := make(map[string]bool, len(avail))
		for _, a := range avail {
			set[a] = true
		}
		return func(bin string) (string, error) {
			if set[bin] {
				return "/usr/bin/" + bin, nil
			}
			return "", errors.New("not found")
		}
	}
	mockEnv := func(kv map[string]string) func(string) string {
		return func(k string) string { return kv[k] }
	}

	t.Run("darwin uses pbcopy", func(t *testing.T) {
		spec, ok := selectClipboardCommand("darwin", mockEnv(nil), mockLookPath("pbcopy"))
		if !ok || spec.name != "/usr/bin/pbcopy" || len(spec.args) != 0 {
			t.Fatalf("unexpected darwin clipboard spec: ok=%v spec=%+v", ok, spec)
		}
	})

	t.Run("windows prefers clip.exe", func(t *testing.T) {
		spec, ok := selectClipboardCommand("windows", mockEnv(nil), mockLookPath("clip.exe", "powershell.exe"))
		if !ok || spec.name != "/usr/bin/clip.exe" {
			t.Fatalf("unexpected windows clipboard spec: ok=%v spec=%+v", ok, spec)
		}
	})

	t.Run("linux wayland prefers wl-copy", func(t *testing.T) {
		spec, ok := selectClipboardCommand(
			"linux",
			mockEnv(map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}),
			mockLookPath("wl-copy", "xclip"),
		)
		if !ok || spec.name != "/usr/bin/wl-copy" {
			t.Fatalf("unexpected wayland clipboard spec: ok=%v spec=%+v", ok, spec)
		}
	})

	t.Run("linux x11 falls back to xclip then xsel", func(t *testing.T) {
		spec, ok := selectClipboardCommand(
			"linux",
			mockEnv(map[string]string{"DISPLAY": ":0"}),
			mockLookPath("xclip"),
		)
		if !ok || spec.name != "/usr/bin/xclip" || strings.Join(spec.args, " ") != "-selection clipboard" {
			t.Fatalf("unexpected xclip spec: ok=%v spec=%+v", ok, spec)
		}

		spec2, ok2 := selectClipboardCommand(
			"linux",
			mockEnv(map[string]string{"DISPLAY": ":0"}),
			mockLookPath("xsel"),
		)
		if !ok2 || spec2.name != "/usr/bin/xsel" {
			t.Fatalf("unexpected xsel spec: ok=%v spec=%+v", ok2, spec2)
		}
	})

	t.Run("linux headless returns unavailable without touching tools", func(t *testing.T) {
		if _, ok := selectClipboardCommand("linux", mockEnv(nil), mockLookPath("xclip", "wl-copy")); ok {
			t.Fatalf("expected headless linux session to report clipboard unavailable")
		}
	})

	t.Run("wsl2 uses clip.exe", func(t *testing.T) {
		spec, ok := selectClipboardCommand(
			"linux",
			mockEnv(map[string]string{"WSL_DISTRO_NAME": "Ubuntu"}),
			mockLookPath("clip.exe"),
		)
		if !ok || spec.name != "/usr/bin/clip.exe" {
			t.Fatalf("unexpected wsl2 clipboard spec: ok=%v spec=%+v", ok, spec)
		}
	})
}

func TestOutputPairingSession(t *testing.T) {
	const sampleURI = "agy://pair?code=deadbeef&host=r-123456&platform=darwin&pk=abc123"

	t.Run("default does not write clipboard", func(t *testing.T) {
		var out, errOut bytes.Buffer
		copied := false
		outputPairingSession(&out, &errOut, "deadbeef", sampleURI, false, false, func(string) error {
			copied = true
			return nil
		})
		if copied {
			t.Fatalf("clipboard must not be written when -copy is false")
		}
		if !strings.Contains(out.String(), sampleURI) {
			t.Fatalf("expected stdout to include uri, got %q", out.String())
		}
		if errOut.Len() != 0 {
			t.Fatalf("expected empty stderr, got %q", errOut.String())
		}
	})

	t.Run("uri-only prints clean single line and sends copy status to stderr", func(t *testing.T) {
		var out, errOut bytes.Buffer
		var gotClip string
		outputPairingSession(&out, &errOut, "deadbeef", sampleURI, true, true, func(s string) error {
			gotClip = s
			return nil
		})
		if out.String() != sampleURI+"\n" {
			t.Fatalf("expected pure single-line URI on stdout, got %q", out.String())
		}
		if gotClip != sampleURI {
			t.Fatalf("expected clipboard text %q, got %q", sampleURI, gotClip)
		}
		if errOut.Len() == 0 {
			t.Fatalf("expected copy confirmation on stderr in -uri -copy mode")
		}
	})

	t.Run("copy failure degrades gracefully to stderr warning", func(t *testing.T) {
		var out, errOut bytes.Buffer
		outputPairingSession(&out, &errOut, "deadbeef", sampleURI, false, true, func(string) error {
			return errors.New("no display")
		})
		if !strings.Contains(errOut.String(), "no display") {
			t.Fatalf("expected stderr warning with error message, got %q", errOut.String())
		}
	})
}
