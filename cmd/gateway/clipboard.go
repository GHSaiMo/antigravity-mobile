package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const clipboardCommandTimeout = 3 * time.Second

type clipboardCmdSpec struct {
	name string
	args []string
}

func selectClipboardCommand(
	goos string,
	getenv func(string) string,
	lookPath func(string) (string, error),
) (clipboardCmdSpec, bool) {
	has := func(bin string) (string, bool) {
		p, err := lookPath(bin)
		if err != nil || strings.TrimSpace(p) == "" {
			return "", false
		}
		return p, true
	}

	switch goos {
	case "darwin":
		if p, ok := has("pbcopy"); ok {
			return clipboardCmdSpec{name: p}, true
		}
	case "windows":
		if p, ok := has("clip.exe"); ok {
			return clipboardCmdSpec{name: p}, true
		}
		if p, ok := has("clip"); ok {
			return clipboardCmdSpec{name: p}, true
		}
		if p, ok := has("powershell.exe"); ok {
			return clipboardCmdSpec{
				name: p,
				args: []string{"-NoProfile", "-NonInteractive", "-Command", "Set-Clipboard"},
			}, true
		}
	default:
		// Wayland session
		if strings.TrimSpace(getenv("WAYLAND_DISPLAY")) != "" {
			if p, ok := has("wl-copy"); ok {
				return clipboardCmdSpec{name: p}, true
			}
		}
		// X11 session
		if strings.TrimSpace(getenv("DISPLAY")) != "" {
			if p, ok := has("xclip"); ok {
				return clipboardCmdSpec{name: p, args: []string{"-selection", "clipboard"}}, true
			}
			if p, ok := has("xsel"); ok {
				return clipboardCmdSpec{name: p, args: []string{"--clipboard", "--input"}}, true
			}
		}
		// WSL2 fallback to Windows clipboard
		if strings.TrimSpace(getenv("WSL_DISTRO_NAME")) != "" || strings.TrimSpace(getenv("WSL_INTEROP")) != "" {
			if p, ok := has("clip.exe"); ok {
				return clipboardCmdSpec{name: p}, true
			}
		}
	}
	return clipboardCmdSpec{}, false
}

// copyTextToClipboard writes text to the host system clipboard when an interactive
// clipboard utility is available. Callers must only invoke this on explicit user opt-in.
func copyTextToClipboard(text string) error {
	spec, ok := selectClipboardCommand(runtime.GOOS, os.Getenv, exec.LookPath)
	if !ok {
		return errors.New("no system clipboard tool available (headless or unsupported session)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), clipboardCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, spec.name, spec.args...)
	cmd.Stdin = strings.NewReader(text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("%s: %w (%s)", spec.name, err, msg)
		}
		return fmt.Errorf("%s: %w", spec.name, err)
	}
	return nil
}
