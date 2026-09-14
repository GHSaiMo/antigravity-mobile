package notifier

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/proxy"
)

type sessionTrackingState struct {
	lastStatus           string
	lastSteps            int
	title                string
	waitingForBackground bool
	fetchRetryCount      int
}

// Watcher continuously monitors Antigravity sessions in the background
// to detect when tasks complete or require user approvals.
type Watcher struct {
	mu             sync.RWMutex
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

	scanTicker := time.NewTicker(3500 * time.Millisecond)
	defer scanTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Watcher] Session watcher stopped")
			return
		case <-cleanupTicker.C:
			w.notifier.Dedup().Cleanup(4 * time.Hour)
			w.cleanupKnownSessions()
		case <-scanTicker.C:
			runningCount := w.scanOnce()
			// Adjust scan frequency: faster when tasks are running
			if runningCount > 0 {
				scanTicker.Reset(1500 * time.Millisecond)
			} else {
				scanTicker.Reset(3500 * time.Millisecond)
			}
		}
	}
}

func (w *Watcher) scanOnce() int {
	port, _ := w.proxy.ActiveUpstream()
	if port == 0 {
		return 0
	}

	summaries, runningSubagents, err := w.proxy.FetchRawCascadeSummaries()
	if err != nil || len(summaries) == 0 {
		return 0
	}

	// 1. Initial baseline synchronization:
	// Record already-completed historical sessions so we don't spam notifications on startup.
	w.mu.Lock()
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
		w.mu.Unlock()
		log.Printf("[Watcher] ✅ Initial baseline sync complete: %d sessions tracked", len(summaries))
		return 0
	}
	w.mu.Unlock()

	runningCount := 0

	// 2. Continuous delta inspection
	for id, s := range summaries {
		status, _ := s["status"].(string)
		steps := extractStepCount(s["stepCount"])
		title := extractTitle(s)
		hasSubagent := runningSubagents[id]

		w.mu.Lock()
		prev, exists := w.knownSessions[id]
		if !exists {
			prev = &sessionTrackingState{
				lastStatus: "CASCADE_RUN_STATUS_INITIAL",
				lastSteps:  0,
				title:      title,
			}
			w.knownSessions[id] = prev
		}
		w.mu.Unlock()

		if status == "CASCADE_RUN_STATUS_RUNNING" {
			runningCount++
			prev.waitingForBackground = false
			prev.fetchRetryCount = 0
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
			if err != nil || details == nil {
				prev.fetchRetryCount++
				if prev.fetchRetryCount <= 3 {
					log.Printf("[Watcher] ⚠️ Session %s status transitioned from RUNNING to %s, but FetchTrajectoryDetails failed (retry %d/3): %v", id, status, prev.fetchRetryCount, err)
					runningCount++
					// Do not advance prev.lastStatus yet so we retry on the next tick!
					continue
				}
				log.Printf("[Watcher] ⚠️ Session %s transition retries exhausted, notifying based on summary status", id)
				if steps > 0 {
					_ = w.notifier.NotifyCompleted(id, title, steps)
				}
				prev.fetchRetryCount = 0
			} else {
				prev.fetchRetryCount = 0
				if details.PendingInteraction != nil {
					_ = w.notifier.NotifyAction(id, details.Title, details.PendingInteraction)
				} else if details.CanProceed {
					_ = w.notifier.NotifyProceed(id, details.Title, details.TotalSteps)
				} else if (details.Status == "CASCADE_RUN_STATUS_FAILED" || details.Status == "CASCADE_RUN_STATUS_ERROR" || details.HasError) && details.TotalSteps > 0 {
					_ = w.notifier.NotifyFailed(id, details.Title, details.TotalSteps)
				} else if (details.Status == "CASCADE_RUN_STATUS_COMPLETED" || details.Status == "CASCADE_RUN_STATUS_IDLE") && details.TotalSteps > 0 {
					inProgress, reason := IsCascadeInProgress(details, hasSubagent)
					if inProgress {
						log.Printf("[Watcher] ⏳ Session %s entered IDLE but still in progress (%s); suppressing completion notification", id, reason)
						prev.waitingForBackground = true
						runningCount++
					} else {
						prev.waitingForBackground = false
						_ = w.notifier.NotifyCompleted(id, details.Title, details.TotalSteps)
					}
				}
			}
		} else if prev.waitingForBackground {
			// Session was waiting for background task or transitional step to finish while in IDLE
			details, err := w.proxy.FetchTrajectoryDetails(id, 250*time.Millisecond)
			if err == nil && details != nil {
				inProgress, _ := IsCascadeInProgress(details, hasSubagent)
				if !inProgress {
					prev.waitingForBackground = false
					log.Printf("[Watcher] ✅ Session %s background work completed! Triggering completion notification", id)
					if details.PendingInteraction != nil {
						_ = w.notifier.NotifyAction(id, details.Title, details.PendingInteraction)
					} else if details.CanProceed {
						_ = w.notifier.NotifyProceed(id, details.Title, details.TotalSteps)
					} else if (details.Status == "CASCADE_RUN_STATUS_FAILED" || details.Status == "CASCADE_RUN_STATUS_ERROR" || details.HasError) && details.TotalSteps > 0 {
						_ = w.notifier.NotifyFailed(id, details.Title, details.TotalSteps)
					} else if details.TotalSteps > 0 {
						_ = w.notifier.NotifyCompleted(id, details.Title, details.TotalSteps)
					}
				} else {
					runningCount++ // Still waiting, keep scan frequency fast (1500ms)
				}
			}
		}

		prev.lastStatus = status
		prev.lastSteps = steps
		if title != "" && title != MsgUntitledSession {
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

// cleanupKnownSessions enforces a soft cap on tracked sessions to prevent unbounded memory growth.
func (w *Watcher) cleanupKnownSessions() {
	w.mu.Lock()
	defer w.mu.Unlock()

	const maxTrackedSessions = 300
	if len(w.knownSessions) <= maxTrackedSessions {
		return
	}

	// Evict completed/non-running sessions first
	for id, s := range w.knownSessions {
		if s.lastStatus != "CASCADE_RUN_STATUS_RUNNING" && !s.waitingForBackground {
			delete(w.knownSessions, id)
			if len(w.knownSessions) <= maxTrackedSessions {
				break
			}
		}
	}
}

// TrackedSessionsCount returns the number of currently monitored sessions.
func (w *Watcher) TrackedSessionsCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.knownSessions)
}

