package notifier

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"antigravity-mobile/internal/proxy"
)

type sessionTrackingState struct {
	lastStatus string
	lastSteps  int
	title      string
}

// Watcher continuously monitors Antigravity sessions in the background
// to detect when tasks complete or require user approvals.
type Watcher struct {
	proxy          *proxy.Proxy
	notifier       *Notifier
	knownSessions  map[string]*sessionTrackingState
	hasInitialSync bool
}

// NewWatcher creates a new background Watcher instance.
func NewWatcher(p *proxy.Proxy, n *Notifier) *Watcher {
	return &Watcher{
		proxy:         p,
		notifier:      n,
		knownSessions: make(map[string]*sessionTrackingState),
	}
}

// Start initiates the background monitoring loop in a separate goroutine.
func (w *Watcher) Start(ctx context.Context) {
	if !w.notifier.IsEnabled() {
		return
	}
	log.Printf("[Watcher] 👁️ Background Antigravity session watcher started")
	go w.run(ctx)
}

func (w *Watcher) run(ctx context.Context) {
	cleanupTicker := time.NewTicker(15 * time.Minute)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Watcher] Session watcher stopped")
			return
		case <-cleanupTicker.C:
			w.notifier.Dedup().Cleanup(4 * time.Hour)
		default:
			runningCount := w.scanOnce()
			var sleepDuration time.Duration
			if runningCount > 0 {
				sleepDuration = 1500 * time.Millisecond
			} else {
				sleepDuration = 3500 * time.Millisecond
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(sleepDuration):
			}
		}
	}
}

func (w *Watcher) scanOnce() int {
	port, _ := w.proxy.ActiveUpstream()
	if port == 0 {
		return 0
	}

	summaries, err := w.proxy.FetchRawCascadeSummaries()
	if err != nil || len(summaries) == 0 {
		return 0
	}

	// 1. Initial baseline synchronization:
	// Record already-completed historical sessions so we don't spam notifications on startup.
	if !w.hasInitialSync {
		for id, s := range summaries {
			status, _ := s["status"].(string)
			steps := extractStepCount(s["stepCount"])
			title := extractTitle(s)

			w.knownSessions[id] = &sessionTrackingState{
				lastStatus: status,
				lastSteps:  steps,
				title:      title,
			}

			// Pre-mark completed sessions in dedup cache
			if status != "CASCADE_RUN_STATUS_RUNNING" {
				w.notifier.Dedup().Record(fmt.Sprintf("done:%s:%d", id, steps))
			}
		}
		w.hasInitialSync = true
		log.Printf("[Watcher] ✅ Initial baseline sync complete: %d sessions tracked", len(summaries))
		return 0
	}

	runningCount := 0

	// 2. Continuous delta inspection
	for id, s := range summaries {
		status, _ := s["status"].(string)
		steps := extractStepCount(s["stepCount"])
		title := extractTitle(s)

		prev, exists := w.knownSessions[id]
		if !exists {
			prev = &sessionTrackingState{
				lastStatus: "CASCADE_RUN_STATUS_INITIAL",
				lastSteps:  0,
				title:      title,
			}
			w.knownSessions[id] = prev
		}

		if status == "CASCADE_RUN_STATUS_RUNNING" {
			runningCount++
			// Fetch real-time trajectory to check for PendingInteraction or CanProceed
			details, err := w.proxy.FetchTrajectoryDetails(id, 250*time.Millisecond)
			if err == nil && details != nil {
				if details.PendingInteraction != nil {
					_ = w.notifier.NotifyAction(id, details.Title, details.PendingInteraction)
				}
				if details.CanProceed {
					_ = w.notifier.NotifyProceed(id, details.Title, details.TotalSteps)
				}
			}
		} else if prev.lastStatus == "CASCADE_RUN_STATUS_RUNNING" && status != "CASCADE_RUN_STATUS_RUNNING" {
			// Transition: Running -> Finished / Waiting / Idle
			details, err := w.proxy.FetchTrajectoryDetails(id, 250*time.Millisecond)
			if err == nil && details != nil {
				if details.PendingInteraction != nil {
					_ = w.notifier.NotifyAction(id, details.Title, details.PendingInteraction)
				} else if details.CanProceed {
					_ = w.notifier.NotifyProceed(id, details.Title, details.TotalSteps)
				} else if details.Status == "CASCADE_RUN_STATUS_COMPLETED" && details.TotalSteps > 0 {
					_ = w.notifier.NotifyCompleted(id, details.Title, details.TotalSteps)
				} else if details.Status == "CASCADE_RUN_STATUS_FAILED" {
					_ = w.notifier.NotifyFailed(id, details.Title, details.TotalSteps)
				}
			}
		}

		prev.lastStatus = status
		prev.lastSteps = steps
		if title != "" && title != "未命名会话" {
			prev.title = title
		}
	}

	return runningCount
}

func extractStepCount(v interface{}) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	case int64:
		return int(n)
	default:
		return 0
	}
}

func extractTitle(s map[string]interface{}) string {
	if ann, ok := s["annotations"].(map[string]interface{}); ok {
		if t, ok := ann["title"].(string); ok && strings.TrimSpace(t) != "" {
			return strings.TrimSpace(t)
		}
	}
	if sm, ok := s["summary"].(string); ok && strings.TrimSpace(sm) != "" {
		return strings.TrimSpace(sm)
	}
	return ""
}
