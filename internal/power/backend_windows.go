//go:build windows

package power

import (
	"fmt"
	"runtime"
	"syscall"
)

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

var (
	modKernel32Power            = syscall.NewLazyDLL("kernel32.dll")
	procSetThreadExecutionState = modKernel32Power.NewProc("SetThreadExecutionState")
)

type windowsBackend struct {
	releaseCh chan struct{}
	doneCh    chan struct{}
}

func newPlatformBackend() Backend {
	return &windowsBackend{}
}

func (b *windowsBackend) Name() string {
	return "SetThreadExecutionState"
}

func (b *windowsBackend) Available() bool {
	return procSetThreadExecutionState.Find() == nil
}

func (b *windowsBackend) Acquire() error {
	if b.releaseCh != nil {
		return nil
	}
	if !b.Available() {
		return fmt.Errorf("SetThreadExecutionState not available")
	}

	initCh := make(chan error, 1)
	releaseCh := make(chan struct{})
	doneCh := make(chan struct{})

	// SetThreadExecutionState is thread-affine: both acquire and release MUST run
	// on the exact same OS thread via runtime.LockOSThread().
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(doneCh)

		r, _, callErr := procSetThreadExecutionState.Call(uintptr(esContinuous | esSystemRequired))
		if r == 0 {
			initCh <- fmt.Errorf("SetThreadExecutionState acquire failed: %v", callErr)
			return
		}
		initCh <- nil

		<-releaseCh
		_, _, _ = procSetThreadExecutionState.Call(uintptr(esContinuous))
	}()

	if err := <-initCh; err != nil {
		return err
	}
	b.releaseCh = releaseCh
	b.doneCh = doneCh
	return nil
}

func (b *windowsBackend) Release() error {
	if b.releaseCh == nil {
		return nil
	}
	close(b.releaseCh)
	if b.doneCh != nil {
		<-b.doneCh
	}
	b.releaseCh = nil
	b.doneCh = nil
	return nil
}
