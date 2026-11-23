//go:build !darwin && !windows

package power

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type linuxBackend struct {
	binPath           string
	mu                sync.Mutex
	permanentlyFailed bool
	cmd               *exec.Cmd
	stdin             io.WriteCloser
	done              chan struct{}
}

func newPlatformBackend() Backend {
	bin, _ := exec.LookPath("systemd-inhibit")
	return &linuxBackend{binPath: bin}
}

func (b *linuxBackend) Name() string {
	return "systemd-inhibit"
}

func (b *linuxBackend) Available() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.binPath != "" && !b.permanentlyFailed
}

func (b *linuxBackend) Acquire() error {
	if b.binPath == "" {
		return fmt.Errorf("systemd-inhibit not found")
	}
	if b.cmd != nil && b.done != nil {
		select {
		case <-b.done:
			b.cleanupLocked()
		default:
			return nil
		}
	}

	// Block on `cat` reading our stdin pipe: when mgy releases or crashes, EOF closes
	// `cat` immediately and systemd-inhibit releases the idle lock with zero orphan risk.
	cmd := exec.Command(
		b.binPath,
		"--what=idle",
		"--who=Multigravity",
		"--why=Antigravity agent running",
		"--mode=block",
		"cat",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("systemd-inhibit stdin pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start systemd-inhibit: %w", err)
	}

	done := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(done)
	}()

	// Headless servers / Docker containers often have systemd-inhibit installed without
	// D-Bus / systemd-logind running, in which case it exits immediately with an error.
	select {
	case <-done:
		_ = stdin.Close()
		msg := strings.TrimSpace(stderr.String())
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "bus") || strings.Contains(lower, "logind") || strings.Contains(lower, "permission") || strings.Contains(lower, "system has not been booted") {
			b.mu.Lock()
			b.permanentlyFailed = true
			b.mu.Unlock()
		}
		if msg != "" {
			return fmt.Errorf("systemd-inhibit exited immediately: %s", msg)
		}
		return fmt.Errorf("systemd-inhibit exited immediately: %v", waitErr)
	case <-time.After(120 * time.Millisecond):
		b.cmd = cmd
		b.stdin = stdin
		b.done = done
		return nil
	}
}

func (b *linuxBackend) Release() error {
	if b.cmd == nil {
		return nil
	}
	cmd := b.cmd
	stdin := b.stdin
	done := b.done
	b.cmd = nil
	b.stdin = nil
	b.done = nil

	if stdin != nil {
		_ = stdin.Close()
	}
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-time.After(500 * time.Millisecond):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(1 * time.Second):
		}
		return nil
	}
}

func (b *linuxBackend) cleanupLocked() {
	if b.stdin != nil {
		_ = b.stdin.Close()
	}
	b.cmd = nil
	b.stdin = nil
	b.done = nil
}
