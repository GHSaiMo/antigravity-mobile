package notifier

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/proxy"
)

const (
	actionNotifyCooldown          = 20 * time.Second
	watcherAlertQueueSize         = 64
	watcherAlertWorkers           = 4
	watcherDetailFetchConcurrency = 4
)

type sessionTrackingState struct {
	lastStatus           string
	lastSteps            int
	title                string
	waitingForBackground bool
	fetchRetryCount      int
	actionRetryAfter     time.Time
	lastUpdated          time.Time
}

// Watcher continuously monitors Antigravity sessions in the background
// to detect when tasks complete or require user approvals.
type Watcher struct {
	mu             sync.RWMutex
	proxy          *proxy.Proxy
	notifier       *Notifier
	knownSessions  map[string]*sessionTrackingState
	hasInitialSync bool
	alertCh        chan func()
}

// NewWatcher creates a new background Watcher instance.
func NewWatcher(p *proxy.Proxy, n *Notifier) *Watcher {
	return &Watcher{
		proxy:         p,
		notifier:      n,
		knownSessions: make(map[string]*sessionTrackingState),
	}
}

// dispatchAlert dispatches high-priority state-transition notifications (Action / Proceed / Completed / Failed / EndSession)
// to the dedicated multi-worker alert pool, isolated from routine Live Activity updates.
// Returns false if the bounded queue is full so callers can retain previous state and retry on the next tick.
func (w *Watcher) dispatchAlert(fn func()) bool {
	w.mu.RLock()
	ch := w.alertCh
	w.mu.RUnlock()
	if ch == nil {
		// Synchronous fallback for direct unit-test scanOnce() calls
		fn()
		return true
	}
	select {
	case ch <- fn:
		return true
	default:
		slog.Warn("[Watcher] Critical alert dispatch queue full, deferring state transition for next tick retry")
		return false
	}
}

// dispatchLiveUpdate routes routine Live Activity updates to the per-cascade single-slot latest-wins coalescer.
func (w *Watcher) dispatchLiveUpdate(details *proxy.TrajectoryDetails) {
	if details == nil {
		return
	}
	la := w.notifier.LiveActivity()
	if la == nil {
		return
	}
	w.mu.RLock()
	ch := w.alertCh
	w.mu.RUnlock()
	if ch == nil {
		snap := *details
		la.SyncFromDetails(context.Background(), &snap)
		return
	}
	la.EnqueueLatest(details)
}

// Start initiates the background monitoring loop in a separate goroutine.
func (w *Watcher) Start(ctx context.Context) {
	if !w.notifier.IsEnabled() && !w.notifier.HasLiveActivityTargets() && !w.notifier.CanActivateDynamically() {
		return
	}
	go w.run(ctx)
}

func (w *Watcher) run(ctx context.Context) {
	alertCh := make(chan func(), watcherAlertQueueSize)
	w.mu.Lock()
	w.alertCh = alertCh
	w.mu.Unlock()
	for i := 0; i < watcherAlertWorkers; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case fn := <-alertCh:
					if fn != nil {
						fn()
					}
				}
			}
		}()
	}

	cleanupTicker := time.NewTicker(15 * time.Minute)
	defer cleanupTicker.Stop()

	scanTicker := time.NewTicker(3500 * time.Millisecond)
	defer scanTicker.Stop()

	sumWake, unsubSum := w.proxy.SubscribeSummaryUpdates()
	defer unsubSum()

	maxAge := 1500 * time.Millisecond
	lastReactiveScan := time.Time{}
	adjustScanCadence := func(runningCount int) {
		if runningCount > 0 {
			maxAge = 400 * time.Millisecond
			if w.proxy.IsReactiveSummariesLive() {
				scanTicker.Reset(2500 * time.Millisecond)
			} else {
				scanTicker.Reset(1500 * time.Millisecond)
			}
		} else {
			maxAge = 1500 * time.Millisecond
			scanTicker.Reset(5000 * time.Millisecond)
		}
	}
	for {
		select {
		case <-ctx.Done():
			slog.Info("[Watcher] Session watcher stopped")
			return
		case <-cleanupTicker.C:
			w.notifier.Dedup().Cleanup(4 * time.Hour)
			if la := w.notifier.LiveActivity(); la != nil {
				la.CleanupStaleTracks(4 * time.Hour)
			}
			w.cleanupKnownSessions()
		case <-sumWake:
			if wait := 80*time.Millisecond - time.Since(lastReactiveScan); wait > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(wait):
				}
			}
			lastReactiveScan = time.Now()
			// Reactive summary stream already updated allTrajectories in-place; read cached snapshot without forcing extra RPC.
			adjustScanCadence(w.scanOnceWithMaxAge(1500 * time.Millisecond))
		case <-scanTicker.C:
			adjustScanCadence(w.scanOnceWithMaxAge(maxAge))
		}
	}
}

func (w *Watcher) scanOnce() int {
	return w.scanOnceWithMaxAge(0)
}

type watcherDetailJob struct {
	id            string
	status        string
	steps         int
	title         string
	hasSubagent   bool
	prev          *sessionTrackingState
	existed       bool
	prevStatus    string
	prevSteps     int
	wasWaitingBg  bool
	prevRetries   int
	cooldownUntil time.Time
}

type watcherDetailResult struct {
	details *proxy.TrajectoryDetails
	err     error
}

func (w *Watcher) scanOnceWithMaxAge(summaryMaxAge time.Duration) int {
	port, _ := w.proxy.ActiveUpstream()
	if port == 0 {
		return 0
	}

	summaries, runningSubagents, err := w.proxy.FetchRawCascadeSummariesWithMaxAge(summaryMaxAge)
	if err != nil || len(summaries) == 0 {
		return 0
	}

	now := time.Now()

	// 1. Initial baseline synchronization:
	// Record already-completed historical sessions so we don't spam notifications on startup.
	w.mu.Lock()
	if !w.hasInitialSync {
		var deferred []func()
		for id, s := range summaries {
			status, _ := s["status"].(string)
			steps := extractStepCount(s["stepCount"])
			title := extractTitle(s)

			w.knownSessions[id] = &sessionTrackingState{
				lastStatus:  status,
				lastSteps:   steps,
				title:       title,
				lastUpdated: now,
			}

			// Pre-mark completed sessions in dedup cache
			if status != "CASCADE_RUN_STATUS_RUNNING" {
				w.notifier.Dedup().Record(fmt.Sprintf("done:%s:%d", id, steps))
				// If a session finished while the gateway was restarting, reconcile any leftover Live Activity state.
				if !runningSubagents[id] {
					if la := w.notifier.LiveActivity(); la != nil && la.HasStateForCascade(id) {
						cid, dTitle, st := id, title, steps
						deferred = append(deferred, func() {
							details, err := w.proxy.FetchTrajectoryDetails(cid, 250*time.Millisecond)
							if err == nil && details != nil {
								if details.PendingInteraction != nil || details.CanProceed {
									la.SyncFromDetails(context.Background(), details)
									return
								}
								if inProg, _ := IsCascadeInProgress(details, false); inProg {
									la.SyncFromDetails(context.Background(), details)
									return
								}
								finalStatus := "COMPLETED"
								if details.Status == "CASCADE_RUN_STATUS_FAILED" || details.Status == "CASCADE_RUN_STATUS_ERROR" || details.HasError {
									finalStatus = "FAILED"
								}
								la.EndSession(context.Background(), cid, details.Title, finalStatus, details.TotalSteps)
								return
							}
							la.EndSession(context.Background(), cid, dTitle, "COMPLETED", st)
						})
					}
				}
			}
		}
		w.hasInitialSync = true
		w.mu.Unlock()
		for _, fn := range deferred {
			_ = w.dispatchAlert(fn)
		}
		return 0
	}
	w.mu.Unlock()

	runningCount := 0
	hasAlert := w.notifier.IsEnabled()
	hasLive := w.notifier.HasLiveActivityTargets()

	var jobs []watcherDetailJob

	// 2. Pass 1: fast state scan & collect active/transitioning sessions requiring detail fetch
	for id, s := range summaries {
		status, _ := s["status"].(string)
		steps := extractStepCount(s["stepCount"])
		title := extractTitle(s)
		hasSubagent := runningSubagents[id]

		w.mu.Lock()
		prev, exists := w.knownSessions[id]
		if !exists {
			prev = &sessionTrackingState{
				lastStatus:  "CASCADE_RUN_STATUS_INITIAL",
				lastSteps:   0,
				title:       title,
				lastUpdated: now,
			}
			w.knownSessions[id] = prev
		}
		prevStatus := prev.lastStatus
		prevSteps := prev.lastSteps
		wasWaitingBg := prev.waitingForBackground
		prevRetries := prev.fetchRetryCount
		cooldownUntil := prev.actionRetryAfter

		needsDetail := (hasAlert || hasLive) &&
			(status == "CASCADE_RUN_STATUS_RUNNING" ||
				(prevStatus == "CASCADE_RUN_STATUS_RUNNING" && status != "CASCADE_RUN_STATUS_RUNNING") ||
				wasWaitingBg)

		if !needsDetail {
			if status == "CASCADE_RUN_STATUS_RUNNING" {
				runningCount++
			}
			if !exists || prevStatus != status || prevSteps != steps || status == "CASCADE_RUN_STATUS_RUNNING" {
				prev.lastUpdated = now
			}
			prev.lastStatus = status
			prev.lastSteps = steps
			if title != "" && title != MsgUntitledSession {
				prev.title = title
			}
			w.mu.Unlock()
			continue
		}
		w.mu.Unlock()

		jobs = append(jobs, watcherDetailJob{
			id:            id,
			status:        status,
			steps:         steps,
			title:         title,
			hasSubagent:   hasSubagent,
			prev:          prev,
			existed:       exists,
			prevStatus:    prevStatus,
			prevSteps:     prevSteps,
			wasWaitingBg:  wasWaitingBg,
			prevRetries:   prevRetries,
			cooldownUntil: cooldownUntil,
		})
	}

	if len(jobs) == 0 {
		return runningCount
	}

	// 3. Pass 2: bounded concurrent FetchTrajectoryDetails (cap = 4)
	results := make([]watcherDetailResult, len(jobs))
	if len(jobs) == 1 {
		d, fErr := w.proxy.FetchTrajectoryDetails(jobs[0].id, 250*time.Millisecond)
		results[0] = watcherDetailResult{details: d, err: fErr}
	} else {
		sem := make(chan struct{}, watcherDetailFetchConcurrency)
		var wg sync.WaitGroup
		for idx := range jobs {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				d, fErr := w.proxy.FetchTrajectoryDetails(jobs[i].id, 250*time.Millisecond)
				results[i] = watcherDetailResult{details: d, err: fErr}
			}(idx)
		}
		wg.Wait()
	}

	// 4. Pass 3: evaluate state transitions & dispatch notifications
	for idx, job := range jobs {
		id := job.id
		status := job.status
		steps := job.steps
		title := job.title
		prev := job.prev
		details := results[idx].details
		fetchErr := results[idx].err

		nextWaitingBg := job.wasWaitingBg
		nextRetries := job.prevRetries

		if status == "CASCADE_RUN_STATUS_RUNNING" {
			runningCount++
			nextWaitingBg = false
			nextRetries = 0
			if fetchErr == nil && details != nil {
				if hasLive {
					w.dispatchLiveUpdate(details)
				}
				if hasAlert && now.After(job.cooldownUntil) {
					if details.PendingInteraction != nil {
						cid, dTitle, pi := id, details.Title, details.PendingInteraction
						pState := prev
						_ = w.dispatchAlert(func() {
							if nErr := w.notifier.NotifyAction(cid, dTitle, pi); nErr != nil {
								w.mu.Lock()
								pState.actionRetryAfter = time.Now().Add(actionNotifyCooldown)
								w.mu.Unlock()
							}
						})
					}
					if details.CanProceed {
						cid, dTitle, totalSteps := id, details.Title, details.TotalSteps
						pState := prev
						_ = w.dispatchAlert(func() {
							if nErr := w.notifier.NotifyProceed(cid, dTitle, totalSteps); nErr != nil {
								w.mu.Lock()
								pState.actionRetryAfter = time.Now().Add(actionNotifyCooldown)
								w.mu.Unlock()
							}
						})
					}
				}
			}
		} else if job.prevStatus == "CASCADE_RUN_STATUS_RUNNING" && status != "CASCADE_RUN_STATUS_RUNNING" {
			// Transition: Running -> Finished / Waiting / Idle
			if fetchErr != nil || details == nil {
				nextRetries++
				if nextRetries <= 3 {
					slog.Warn(fmt.Sprintf("[Watcher] ⚠️ Session %s status transitioned from RUNNING to %s, but FetchTrajectoryDetails failed (retry %d/3)", id, status, nextRetries), "err", fetchErr)
					runningCount++
					w.mu.Lock()
					prev.fetchRetryCount = nextRetries
					prev.lastUpdated = now
					w.mu.Unlock()
					// Do not advance prev.lastStatus yet so we retry on the next tick!
					continue
				}
				slog.Warn(fmt.Sprintf("[Watcher] ⚠️ Session %s transition retries exhausted, notifying based on summary status", id))
				if steps > 0 {
					cid, dTitle, st := id, title, steps
					enqueued := true
					if hasAlert {
						if !w.dispatchAlert(func() { _ = w.notifier.NotifyCompleted(cid, dTitle, st) }) {
							enqueued = false
						}
					}
					if hasLive {
						if la := w.notifier.LiveActivity(); la != nil {
							if !w.dispatchAlert(func() { la.EndSession(context.Background(), cid, dTitle, "COMPLETED", st) }) {
								enqueued = false
							}
						}
					}
					if !enqueued {
						runningCount++
						continue
					}
				}
				nextRetries = 0
			} else {
				nextRetries = 0
				enqueued := true
				if details.PendingInteraction != nil {
					cid, dTitle, pi := id, details.Title, details.PendingInteraction
					if hasAlert && !w.dispatchAlert(func() { _ = w.notifier.NotifyAction(cid, dTitle, pi) }) {
						enqueued = false
					}
					if hasLive {
						w.dispatchLiveUpdate(details)
					}
				} else if details.CanProceed {
					cid, dTitle, totalSteps := id, details.Title, details.TotalSteps
					if hasAlert && !w.dispatchAlert(func() { _ = w.notifier.NotifyProceed(cid, dTitle, totalSteps) }) {
						enqueued = false
					}
					if hasLive {
						w.dispatchLiveUpdate(details)
					}
				} else if (details.Status == "CASCADE_RUN_STATUS_FAILED" || details.Status == "CASCADE_RUN_STATUS_ERROR" || details.HasError) && details.TotalSteps > 0 {
					cid, dTitle, totalSteps := id, details.Title, details.TotalSteps
					if hasAlert && !w.dispatchAlert(func() { _ = w.notifier.NotifyFailed(cid, dTitle, totalSteps) }) {
						enqueued = false
					}
					if hasLive {
						if la := w.notifier.LiveActivity(); la != nil {
							if !w.dispatchAlert(func() { la.EndSession(context.Background(), cid, dTitle, "FAILED", totalSteps) }) {
								enqueued = false
							}
						}
					}
				} else if (details.Status == "CASCADE_RUN_STATUS_COMPLETED" || details.Status == "CASCADE_RUN_STATUS_IDLE") && details.TotalSteps > 0 {
					inProgress, _ := IsCascadeInProgress(details, job.hasSubagent)
					if inProgress {
						nextWaitingBg = true
						runningCount++
						if hasLive {
							w.dispatchLiveUpdate(details)
						}
					} else {
						nextWaitingBg = false
						cid, dTitle, totalSteps := id, details.Title, details.TotalSteps
						if hasAlert && !w.dispatchAlert(func() { _ = w.notifier.NotifyCompleted(cid, dTitle, totalSteps) }) {
							enqueued = false
						}
						if hasLive {
							if la := w.notifier.LiveActivity(); la != nil {
								if !w.dispatchAlert(func() { la.EndSession(context.Background(), cid, dTitle, "COMPLETED", totalSteps) }) {
									enqueued = false
								}
							}
						}
					}
				}
				if !enqueued {
					// Queue full: retain RUNNING state so next tick retries instead of dropping terminal alert.
					runningCount++
					continue
				}
			}
		} else if job.wasWaitingBg {
			// Session was waiting for background task or transitional step to finish while in IDLE
			if fetchErr == nil && details != nil {
				inProgress, _ := IsCascadeInProgress(details, job.hasSubagent)
				if !inProgress {
					enqueued := true
					slog.Info(fmt.Sprintf("[Watcher] ✅ 会话 %s 后台任务已完成", shortID(id)))
					if details.PendingInteraction != nil {
						cid, dTitle, pi := id, details.Title, details.PendingInteraction
						if hasAlert && !w.dispatchAlert(func() { _ = w.notifier.NotifyAction(cid, dTitle, pi) }) {
							enqueued = false
						}
						if hasLive {
							w.dispatchLiveUpdate(details)
						}
					} else if details.CanProceed {
						cid, dTitle, totalSteps := id, details.Title, details.TotalSteps
						if hasAlert && !w.dispatchAlert(func() { _ = w.notifier.NotifyProceed(cid, dTitle, totalSteps) }) {
							enqueued = false
						}
						if hasLive {
							w.dispatchLiveUpdate(details)
						}
					} else if (details.Status == "CASCADE_RUN_STATUS_FAILED" || details.Status == "CASCADE_RUN_STATUS_ERROR" || details.HasError) && details.TotalSteps > 0 {
						cid, dTitle, totalSteps := id, details.Title, details.TotalSteps
						if hasAlert && !w.dispatchAlert(func() { _ = w.notifier.NotifyFailed(cid, dTitle, totalSteps) }) {
							enqueued = false
						}
						if hasLive {
							if la := w.notifier.LiveActivity(); la != nil {
								if !w.dispatchAlert(func() { la.EndSession(context.Background(), cid, dTitle, "FAILED", totalSteps) }) {
									enqueued = false
								}
							}
						}
					} else if details.TotalSteps > 0 {
						cid, dTitle, totalSteps := id, details.Title, details.TotalSteps
						if hasAlert && !w.dispatchAlert(func() { _ = w.notifier.NotifyCompleted(cid, dTitle, totalSteps) }) {
							enqueued = false
						}
						if hasLive {
							if la := w.notifier.LiveActivity(); la != nil {
								if !w.dispatchAlert(func() { la.EndSession(context.Background(), cid, dTitle, "COMPLETED", totalSteps) }) {
									enqueued = false
								}
							}
						}
					}
					if !enqueued {
						runningCount++
						continue
					}
					nextWaitingBg = false
				} else {
					runningCount++ // Still waiting, keep scan frequency fast (1500ms)
					if hasLive {
						w.dispatchLiveUpdate(details)
					}
				}
			}
		}

		w.mu.Lock()
		if !job.existed || prev.lastStatus != status || prev.lastSteps != steps || status == "CASCADE_RUN_STATUS_RUNNING" || nextWaitingBg || prev.waitingForBackground != nextWaitingBg {
			prev.lastUpdated = now
		}
		prev.waitingForBackground = nextWaitingBg
		prev.fetchRetryCount = nextRetries
		prev.lastStatus = status
		prev.lastSteps = steps
		if title != "" && title != MsgUntitledSession {
			prev.title = title
		}
		w.mu.Unlock()
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
			if clean := proxy.SanitizeTitle(t); clean != "" {
				return clean
			}
		}
	}
	if sm, ok := s["summary"].(string); ok && strings.TrimSpace(sm) != "" {
		if clean := proxy.SanitizeTitle(sm); clean != "" {
			return clean
		}
	}
	return ""
}

// cleanupKnownSessions enforces a soft cap on tracked sessions to prevent unbounded memory growth.
// Only evicts sessions that are non-running AND have been in a terminal state for over 24 hours.
func (w *Watcher) cleanupKnownSessions() {
	w.mu.Lock()
	defer w.mu.Unlock()

	const maxTrackedSessions = 300
	if len(w.knownSessions) <= maxTrackedSessions {
		return
	}

	const staleAge = 24 * time.Hour
	now := time.Now()

	// Evict completed/non-running sessions that have been stale for over 24 hours
	for id, s := range w.knownSessions {
		if s.lastStatus != "CASCADE_RUN_STATUS_RUNNING" && !s.waitingForBackground && now.Sub(s.lastUpdated) > staleAge {
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

func shortID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 12 {
		return id
	}
	return id[:8] + "..."
}
