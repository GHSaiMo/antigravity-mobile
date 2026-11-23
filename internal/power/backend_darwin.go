//go:build darwin

package power

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

type darwinBackend struct {
	binPath string
	cmd     *exec.Cmd
	done    chan struct{}
}

func newPlatformBackend() Backend {
	bin, _ := exec.LookPath("caffeinate")
	return &darwinBackend{binPath: bin}
}

func (b *darwinBackend) Name() string {
	return "caffeinate(-i)"
}

func (b *darwinBackend) Available() bool {
	return b.binPath != ""
}

func (b *darwinBackend) Acquire() error {
	if b.binPath == "" {
		return fmt.Errorf("caffeinate binary not found")
	}
	// If already running and alive, keep existing assertion.
	if b.cmd != nil && b.done != nil {
		select {
		case <-b.done:
			b.cmd = nil
			b.done = nil
		default:
			return nil
		}
	}

	// -i prevents idle system sleep while still allowing display sleep.
	// -w <pid> guarantees macOS automatically terminates caffeinate if mgy crashes or is SIGKILLed.
	cmd := exec.Command(b.binPath, "-i", "-w", strconv.Itoa(os.Getpid()))
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start caffeinate: %w", err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	b.cmd = cmd
	b.done = done
	return nil
}

func (b *darwinBackend) Release() error {
	if b.cmd == nil {
		return nil
	}
	cmd := b.cmd
	done := b.done
	b.cmd = nil
	b.done = nil

	if cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
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
