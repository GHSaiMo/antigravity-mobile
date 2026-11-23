package power

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	// DefaultReleaseGrace keeps the host awake briefly after all tasks become idle so
	// immediate follow-up prompts from mobile don't race against sudden system sleep.
	DefaultReleaseGrace = 2 * time.Minute

	acquireErrorLogCooldown = 5 * time.Minute
)

// Backend is a platform-specific idle-sleep inhibitor (macOS caffeinate, Windows
// SetThreadExecutionState, Linux systemd-inhibit).
type Backend interface {
	Name() string
	Available() bool
	Acquire() error
	Release() error
}

// Guard dynamically prevents host idle sleep ONLY while Antigravity has active
// sessions running, releasing the assertion after a short idle grace period.
// Display sleep is never blocked; gateway exit always releases the assertion.
type Guard struct {
	backend      Backend
	releaseGrace time.Duration

	mu           sync.Mutex
	held         bool
	active       bool
	closed       bool
	releaseTimer *time.Timer
	timerGen     uint64
	lastErrLog   time.Time
}

// NewGuard creates a sleep guard backed by backend (or the host's default platform
// backend when nil). If releaseGrace <= 0, DefaultReleaseGrace is used.
func NewGuard(backend Backend, releaseGrace time.Duration) *Guard {
	if backend == nil {
		backend = newPlatformBackend()
	}
	if releaseGrace <= 0 {
		releaseGrace = DefaultReleaseGrace
	}
	return &Guard{
		backend:      backend,
		releaseGrace: releaseGrace,
	}
}

// Available reports whether the underlying OS backend can hold a sleep assertion.
func (g *Guard) Available() bool {
	if g == nil || g.backend == nil {
		return false
	}
	return g.backend.Available()
}

// BackendName returns the human-readable platform inhibitor name.
func (g *Guard) BackendName() string {
	if g == nil || g.backend == nil {
		return "none"
	}
	return g.backend.Name()
}

// IsHeld reports whether a host sleep assertion is currently held.
func (g *Guard) IsHeld() bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.held
}

// SetActive updates whether Antigravity currently has at least one running session.
// Becoming active acquires the assertion immediately; becoming inactive schedules
// release after releaseGrace.
func (g *Guard) SetActive(active bool) {
	if g == nil || g.backend == nil {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if g.closed {
		return
	}
	if !g.backend.Available() {
		return
	}

	if active {
		g.active = true
		g.timerGen++
		if g.releaseTimer != nil {
			g.releaseTimer.Stop()
			g.releaseTimer = nil
		}
		if g.held {
			return
		}
		if err := g.backend.Acquire(); err != nil {
			now := time.Now()
			if g.lastErrLog.IsZero() || now.Sub(g.lastErrLog) >= acquireErrorLogCooldown {
				g.lastErrLog = now
				slog.Warn("[Power] failed to acquire idle-sleep guard", "backend", g.backend.Name(), "err", err)
			}
			return
		}
		g.held = true
		slog.Info("[Power] 🔋 检测到活跃任务，已启用任务期防休眠", "backend", g.backend.Name())
		return
	}

	// Inactive
	g.active = false
	if !g.held || g.releaseTimer != nil {
		return
	}
	gen := g.timerGen
	g.releaseTimer = time.AfterFunc(g.releaseGrace, func() {
		g.onReleaseTimer(gen)
	})
}

func (g *Guard) onReleaseTimer(gen uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.closed || g.active || !g.held || g.timerGen != gen {
		return
	}
	g.releaseTimer = nil
	if err := g.backend.Release(); err != nil {
		slog.Warn("[Power] failed to release idle-sleep guard", "backend", g.backend.Name(), "err", err)
	}
	g.held = false
	slog.Info("[Power] 💤 任务已空闲，已释放防休眠锁", "backend", g.backend.Name())
}

// WatchContext automatically releases any held assertion when ctx is canceled.
func (g *Guard) WatchContext(ctx context.Context) {
	if g == nil || ctx == nil {
		return
	}
	go func() {
		<-ctx.Done()
		g.Close()
	}()
}

// Close permanently stops the guard and releases any active sleep assertion.
func (g *Guard) Close() {
	if g == nil || g.backend == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.closed {
		return
	}
	g.closed = true
	g.active = false
	g.timerGen++
	if g.releaseTimer != nil {
		g.releaseTimer.Stop()
		g.releaseTimer = nil
	}
	if g.held {
		if err := g.backend.Release(); err != nil {
			slog.Warn("[Power] failed to release idle-sleep guard on shutdown", "backend", g.backend.Name(), "err", err)
		}
		g.held = false
	}
}
