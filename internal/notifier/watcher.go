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
	pendingApprovalPollTTL        = 2 * time.Hour
	watcherAlertQueueSize         = 64
	watcherAlertWorkers           = 4
	watcherDetailFetchConcurrency = 4
	defaultCacheGuardInterval     = 15 * time.Second
)

type sessionTrackingState struct {
	lastStatus            string
	lastSteps             int
	title                 string
	waitingForBackground  bool
	pendingApprovalKey    string
	pendingApprovalSince  time.Time
	fetchRetryCount       int
	actionRetryAfter      time.Time
	lastCacheGuardCheckAt time.Time
	lastProgressAt        time.Time
	lastProgressSig       string
	lastUpdated           time.Time
}

// Watcher continuously monitors Antigravity sessions in the background
// to detect when tasks complete or require user approvals.
type Watcher struct {
	mu                 sync.RWMutex
	proxy              *proxy.Proxy
	notifier           *Notifier
	knownSessions      map[string]*sessionTrackingState
	hasInitialSync     bool
	initialSyncAt      time.Time
	lastActivityAt     time.Time
	onActivity         func(active bool)
	alertCh            chan func()
	cacheGuardInterval time.Duration
	stallThreshold     time.Duration
	stallBgThreshold   time.Duration
	nowFunc            func() time.Time
}

func (w *Watcher) now() time.Time {
	if w != nil && w.nowFunc != nil {
		return w.nowFunc()
	}
	return time.Now()
}

func (w *Watcher) effectiveCacheGuardInterval() time.Duration {
	if w.cacheGuardInterval > 0 {
		return w.cacheGuardInterval
	}
	return defaultCacheGuardInterval
}

func (w *Watcher) effectiveStallThreshold() time.Duration {
	if w.stallThreshold > 0 {
		return w.stallThreshold
	}
	mins := DefaultStallAlertMinutes()
	if mins <= 0 {
		return 0
	}
	return time.Duration(mins) * time.Minute
}

func (w *Watcher) effectiveStallBgThreshold() time.Duration {
	if w.stallBgThreshold > 0 {
		return w.stallBgThreshold
	}
	mins := DefaultStallBgAlertMinutes()
	if mins <= 0 {
		return 0
	}
	return time.Duration(mins) * time.Minute
}

func hasActiveBackgroundCommand(tasks []proxy.RunningTaskItem) bool {
	for _, t := range tasks {
		if t.IsDaemon {
			continue
		}
		if strings.TrimSpace(t.CommandLine) != "" ||
			strings.TrimSpace(t.LogURI) != "" ||
			strings.TrimSpace(t.ToolSummary) != "" ||
			strings.TrimSpace(t.ToolAction) != "" {
			return true
		}
	}
	return false
}

func progressSignatureForDetails(steps int, details *proxy.TrajectoryDetails) string {
	if details == nil {
		return fmt.Sprintf("s:%d", steps)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "s:%d/%d/t:%d|f:%s@%s", steps, details.TotalSteps, details.TotalTools, details.ActiveFocus, details.ActiveFocusStarted)
	if details.ActivePlan != nil {
		fmt.Fprintf(&b, "|p:%d/%d/%d", details.ActivePlan.DoneCount, details.ActivePlan.TotalCount, len(details.ActivePlan.Items))
		for _, ps := range details.ActivePlan.Items {
			b.WriteByte(',')
			b.WriteString(ps.Status)
		}
	}
	fmt.Fprintf(&b, "|m:%d", len(details.AllMessages))
	if n := len(details.AllMessages); n > 0 {
		last := details.AllMessages[n-1]
		fmt.Fprintf(&b, "(%s:%d:%d:%d)", last.ID, len(last.Text), len(last.Thinking), len(last.ToolDetails))
	}
	if len(details.RunningTasks) > 0 {
		fmt.Fprintf(&b, "|rt:%d", len(details.RunningTasks))
		for _, rt := range details.RunningTasks {
			b.WriteByte(',')
			b.WriteString(rt.ID)
			fmt.Fprintf(&b, ":%d", rt.StepIndex)
		}
	}
	return b.String()
}

// NewWatcher creates a new background Watcher instance.
func NewWatcher(p *proxy.Proxy, n *Notifier) *Watcher {
	return &Watcher{
		proxy:         p,
		notifier:      n,
		knownSessions: make(map[string]*sessionTrackingState),
	}
}

// SetOnActivityChange registers a callback invoked after each scan with whether any
// Antigravity session on this host is currently running or waiting on a background task.
func (w *Watcher) SetOnActivityChange(fn func(active bool)) {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.onActivity = fn
	w.mu.Unlock()
}

func (w *Watcher) notifyActivity(active bool) {
	w.mu.RLock()
	fn := w.onActivity
	w.mu.RUnlock()
	if fn != nil {
		fn(active)
	}
}

// IdleSince reports the timestamp since which no Antigravity session on this host has been
// running or advancing steps. It returns (_, false) if the watcher has not completed its
// initial sync or if any session is currently running / waiting on a background task.
func (w *Watcher) IdleSince() (time.Time, bool) {
	if w == nil {
		return time.Time{}, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if !w.hasInitialSync {
		return time.Time{}, false
	}
	for _, st := range w.knownSessions {
		if st.lastStatus == "CASCADE_RUN_STATUS_RUNNING" || st.waitingForBackground {
			return time.Time{}, false
		}
	}
	since := w.initialSyncAt
	if w.lastActivityAt.After(since) {
		since = w.lastActivityAt
	}
	if since.IsZero() {
		return time.Time{}, false
	}
	return since, true
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

// dispatchLiveUpdate routes routine Live Activity updates to the per-cascade single-slot latest-wins coalescer
// and any attached Hub relay hook.
func (w *Watcher) dispatchLiveUpdate(details *proxy.TrajectoryDetails) {
	if details == nil || w.notifier == nil {
		return
	}
	w.mu.RLock()
	ch := w.alertCh
	w.mu.RUnlock()
	la := w.notifier.LiveActivity()
	if ch == nil {
		snap := *details
		if la != nil && la.IsActive() {
			la.SyncFromDetails(context.Background(), &snap)
		}
		w.notifier.mu.RLock()
		hook := w.notifier.hubLiveUpdate
		w.notifier.mu.RUnlock()
		if hook != nil {
			hook(&snap)
		}
		return
	}
	w.notifier.EmitLiveUpdate(details)
}

// Start initiates the background monitoring loop in a separate goroutine.
func (w *Watcher) Start(ctx context.Context) {
	w.mu.RLock()
	hasActivityHook := w.onActivity != nil
	w.mu.RUnlock()
	if !w.notifier.IsEnabled() && !w.notifier.HasLiveActivityTargets() && !w.notifier.CanActivateDynamically() && !w.hasPersistentCacheAudit() && !hasActivityHook {
		return
	}
	go w.run(ctx)
}

func (w *Watcher) hasPersistentCacheAudit() bool {
	if w == nil || w.notifier == nil || !w.notifier.IsCacheMissMonitoringActive() {
		return false
	}
	aud := w.notifier.CacheMissAuditor()
	return aud != nil && (aud.ReportPath() != "" || aud.StatePath() != "")
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
			if aud := w.notifier.CacheMissAuditor(); aud != nil {
				aud.ExpireStale(cacheMissAuditStaleAge, w.now())
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
	id                       string
	status                   string
	steps                    int
	title                    string
	hasSubagent              bool
	prev                     *sessionTrackingState
	existed                  bool
	prevStatus               string
	prevSteps                int
	wasWaitingBg             bool
	prevPendingApprovalKey   string
	prevPendingApprovalSince time.Time
	prevRetries              int
	cooldownUntil            time.Time
	lastCacheGuardCheckAt    time.Time
	lastProgressAt           time.Time
	lastProgressSig          string
}

func approvalKeyForDetails(details *proxy.TrajectoryDetails) string {
	if details == nil {
		return ""
	}
	if details.PendingInteraction != nil {
		return fmt.Sprintf("pi:%d:%s", details.PendingInteraction.StepIndex, details.PendingInteraction.Type)
	}
	if details.CanProceed && !details.ProceedDismissed {
		return fmt.Sprintf("proceed:%d", details.TotalSteps)
	}
	return ""
}

type watcherDetailResult struct {
	details *proxy.TrajectoryDetails
	err     error
}

func (w *Watcher) scanOnceWithMaxAge(summaryMaxAge time.Duration) int {
	port, _ := w.proxy.ActiveUpstream()
	if port == 0 {
		w.notifyActivity(false)
		return 0
	}

	summaries, runningSubagents, err := w.proxy.FetchRawCascadeSummariesWithMaxAge(summaryMaxAge)
	if err != nil {
		return 0
	}
	if len(summaries) == 0 {
		w.notifyActivity(false)
		return 0
	}

	now := w.now()

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
			if status != "CASCADE_RUN_STATUS_RUNNING" || proxy.IsCascadeArchived(id, s) {
				w.notifier.Dedup().Record(fmt.Sprintf("done:%s:%d", id, steps))
				// If a session finished or was archived while the gateway was restarting, reconcile any leftover Live Activity state.
				if proxy.IsCascadeArchived(id, s) {
					if la := w.notifier.LiveActivity(); la != nil && la.HasStateForCascade(id) {
						cid, dTitle, st := id, title, steps
						deferred = append(deferred, func() {
							la.EndSessionImmediate(context.Background(), cid, dTitle, "COMPLETED", st)
						})
					}
				} else if !runningSubagents[id] {
					if la := w.notifier.LiveActivity(); la != nil && la.HasStateForCascade(id) {
						cid, dTitle, st := id, title, steps
						deferred = append(deferred, func() {
							details, err := w.proxy.FetchTrajectoryDetails(cid, 250*time.Millisecond)
							if err == nil && details != nil {
								if details.PendingInteraction != nil || (details.CanProceed && !details.ProceedDismissed) {
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
		w.initialSyncAt = now
		initialRunning := false
		for _, st := range w.knownSessions {
			if st.lastStatus == "CASCADE_RUN_STATUS_RUNNING" {
				w.lastActivityAt = now
				initialRunning = true
				break
			}
		}
		w.mu.Unlock()
		w.notifyActivity(initialRunning)
		for _, fn := range deferred {
			_ = w.dispatchAlert(fn)
		}
		return 0
	}
	w.mu.Unlock()

	runningCount := 0
	activitySeen := false
	hasAlert := w.notifier.IsEnabled()
	hasLive := w.notifier.HasLiveActivityTargets()
	hasCacheAudit := w.notifier.IsCacheMissMonitoringActive() && (hasAlert || hasLive || w.hasPersistentCacheAudit())

	var jobs []watcherDetailJob

	// 2. Pass 1: fast state scan & collect active/transitioning sessions requiring detail fetch
	for id, s := range summaries {
		status, _ := s["status"].(string)
		steps := extractStepCount(s["stepCount"])
		title := extractTitle(s)
		hasSubagent := runningSubagents[id]

		if proxy.IsCascadeArchived(id, s) {
			w.mu.Lock()
			prev, exists := w.knownSessions[id]
			if !exists {
				prev = &sessionTrackingState{
					lastStatus:  status,
					lastSteps:   steps,
					title:       title,
					lastUpdated: now,
				}
				w.knownSessions[id] = prev
			}
			prevPendingKey := prev.pendingApprovalKey
			prev.pendingApprovalKey = ""
			prev.pendingApprovalSince = time.Time{}
			prev.waitingForBackground = false
			prev.lastStatus = status
			prev.lastSteps = steps
			if title != "" && title != MsgUntitledSession {
				prev.title = title
			}
			w.mu.Unlock()

			if prevPendingKey != "" && hasAlert {
				cid, dismissKey := id, prevPendingKey
				_ = w.dispatchAlert(func() {
					_ = w.notifier.NotifyDismissApproval(cid, dismissKey)
				})
			}
			if hasLive {
				if la := w.notifier.LiveActivity(); la != nil && la.HasStateForCascade(id) {
					cid, dTitle, st := id, title, steps
					_ = w.dispatchAlert(func() {
						la.EndSessionImmediate(context.Background(), cid, dTitle, "COMPLETED", st)
					})
				}
			}
			continue
		}

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
		prevPendingKey := prev.pendingApprovalKey
		prevPendingSince := prev.pendingApprovalSince
		prevRetries := prev.fetchRetryCount
		cooldownUntil := prev.actionRetryAfter
		lastCacheGuardCheckAt := prev.lastCacheGuardCheckAt
		lastProgressAt := prev.lastProgressAt
		lastProgressSig := prev.lastProgressSig

		if status == "CASCADE_RUN_STATUS_RUNNING" ||
			(exists && steps > prevSteps) ||
			(prevStatus == "CASCADE_RUN_STATUS_RUNNING" && status != "CASCADE_RUN_STATUS_RUNNING") ||
			wasWaitingBg || hasSubagent {
			activitySeen = true
		}

		watchPendingApproval := prevPendingKey != "" &&
			(prevPendingSince.IsZero() || now.Sub(prevPendingSince) <= pendingApprovalPollTTL || status != prevStatus || steps != prevSteps)

		needsDetail := (hasAlert || hasLive || hasCacheAudit) &&
			(status == "CASCADE_RUN_STATUS_RUNNING" ||
				(prevStatus == "CASCADE_RUN_STATUS_RUNNING" && status != "CASCADE_RUN_STATUS_RUNNING") ||
				wasWaitingBg ||
				watchPendingApproval)

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
			id:                       id,
			status:                   status,
			steps:                    steps,
			title:                    title,
			hasSubagent:              hasSubagent,
			prev:                     prev,
			existed:                  exists,
			prevStatus:               prevStatus,
			prevSteps:                prevSteps,
			wasWaitingBg:             wasWaitingBg,
			prevPendingApprovalKey:   prevPendingKey,
			prevPendingApprovalSince: prevPendingSince,
			prevRetries:              prevRetries,
			cooldownUntil:            cooldownUntil,
			lastCacheGuardCheckAt:    lastCacheGuardCheckAt,
			lastProgressAt:           lastProgressAt,
			lastProgressSig:          lastProgressSig,
		})
	}

	if len(jobs) == 0 {
		if runningCount > 0 || activitySeen {
			w.mu.Lock()
			if now.After(w.lastActivityAt) {
				w.lastActivityAt = now
			}
			w.mu.Unlock()
		}
		w.notifyActivity(runningCount > 0)
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
		nextPendingKey := job.prevPendingApprovalKey
		nextPendingSince := job.prevPendingApprovalSince
		nextCacheGuardCheckAt := job.lastCacheGuardCheckAt
		nextProgressAt := job.lastProgressAt
		nextProgressSig := job.lastProgressSig
		setPending := func(key string) {
			nextPendingKey = key
			if key == "" {
				nextPendingSince = time.Time{}
			} else if key != job.prevPendingApprovalKey || nextPendingSince.IsZero() {
				nextPendingSince = now
			}
		}

		if status == "CASCADE_RUN_STATUS_RUNNING" {
			runningCount++
			nextWaitingBg = false
			nextRetries = 0
			if fetchErr == nil && details != nil {
				curPendingKey := approvalKeyForDetails(details)
				if curPendingKey == "" && job.prevPendingApprovalKey != "" {
					if hasAlert {
						cid, dismissKey := id, job.prevPendingApprovalKey
						_ = w.dispatchAlert(func() {
							_ = w.notifier.NotifyDismissApproval(cid, dismissKey)
						})
					}
				}
				setPending(curPendingKey)
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
					if details.CanProceed && !details.ProceedDismissed {
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
				if hasCacheAudit &&
					strings.TrimSpace(details.ParentConversation) == "" &&
					(job.lastCacheGuardCheckAt.IsZero() || now.Sub(job.lastCacheGuardCheckAt) >= w.effectiveCacheGuardInterval()) {
					nextCacheGuardCheckAt = now
					if meter := w.proxy.FetchContextMeter(id); meter != nil {
						dTitle := details.Title
						if dTitle == "" || dTitle == MsgUntitledSession {
							dTitle = title
						}
						totalSteps := details.TotalSteps
						if totalSteps <= 0 {
							totalSteps = steps
						}
						obs := buildCacheMissObservation(
							id,
							dTitle,
							pickModelName(details, meter),
							"watcher",
							totalSteps,
							meter,
							now,
						)
						_ = w.dispatchAlert(func() {
							_ = w.notifier.ObserveCacheMiss(obs)
						})
					}
				}
				// Long-running stall detection (exempts subagents & approval waits; relaxes threshold during active background commands)
				if strings.TrimSpace(details.ParentConversation) == "" {
					curSig := progressSignatureForDetails(steps, details)
					resumedFromApproval := curPendingKey == "" && job.prevPendingApprovalKey != ""
					if details.PendingInteraction != nil || (details.CanProceed && !details.ProceedDismissed) || resumedFromApproval || nextProgressAt.IsZero() || curSig != nextProgressSig {
						nextProgressAt = now
						nextProgressSig = curSig
					} else if hasAlert && w.notifier.AlertPreferences().TaskStall {
						threshold := w.effectiveStallThreshold()
						if hasActiveBackgroundCommand(details.RunningTasks) || job.hasSubagent {
							threshold = w.effectiveStallBgThreshold()
						}
						if threshold > 0 {
							stalled := now.Sub(nextProgressAt)
							if stalled >= threshold {
								stalledMins := int(stalled.Minutes())
								if stalledMins < 1 {
									stalledMins = 1
								}
								cid := id
								dTitle := details.Title
								if dTitle == "" || dTitle == MsgUntitledSession {
									dTitle = title
								}
								totalSteps := details.TotalSteps
								if totalSteps <= 0 {
									totalSteps = steps
								}
								_ = w.dispatchAlert(func() {
									_ = w.notifier.NotifyStallAlert(cid, dTitle, stalledMins, totalSteps)
								})
							}
						}
					}
				}
			}
		} else {
			nextProgressAt = time.Time{}
			nextProgressSig = ""
			if job.prevStatus == "CASCADE_RUN_STATUS_RUNNING" && status != "CASCADE_RUN_STATUS_RUNNING" {
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
						if !w.dispatchAlert(func() { w.notifier.EmitLiveEnd(context.Background(), cid, dTitle, "COMPLETED", st) }) {
							enqueued = false
						}
					}
					if !enqueued {
						runningCount++
						continue
					}
				}
				setPending("")
				nextRetries = 0
			} else {
				nextRetries = 0
				enqueued := true
				if details.PendingInteraction != nil {
					setPending(approvalKeyForDetails(details))
					cid, dTitle, pi := id, details.Title, details.PendingInteraction
					if hasAlert && !w.dispatchAlert(func() { _ = w.notifier.NotifyAction(cid, dTitle, pi) }) {
						enqueued = false
					}
					if hasLive {
						w.dispatchLiveUpdate(details)
					}
				} else if details.CanProceed && !details.ProceedDismissed {
					setPending(approvalKeyForDetails(details))
					cid, dTitle, totalSteps := id, details.Title, details.TotalSteps
					if hasAlert && !w.dispatchAlert(func() { _ = w.notifier.NotifyProceed(cid, dTitle, totalSteps) }) {
						enqueued = false
					}
					if hasLive {
						w.dispatchLiveUpdate(details)
					}
				} else if (details.Status == "CASCADE_RUN_STATUS_FAILED" || details.Status == "CASCADE_RUN_STATUS_ERROR" || details.HasError) && details.TotalSteps > 0 {
					setPending("")
					cid, dTitle, totalSteps := id, details.Title, details.TotalSteps
					if hasAlert && !w.dispatchAlert(func() { _ = w.notifier.NotifyFailed(cid, dTitle, totalSteps) }) {
						enqueued = false
					}
					if hasLive {
						if !w.dispatchAlert(func() { w.notifier.EmitLiveEnd(context.Background(), cid, dTitle, "FAILED", totalSteps) }) {
							enqueued = false
						}
					}
				} else if (details.Status == "CASCADE_RUN_STATUS_COMPLETED" || details.Status == "CASCADE_RUN_STATUS_IDLE") && details.TotalSteps > 0 {
					inProgress, _ := IsCascadeInProgress(details, job.hasSubagent)
					if inProgress {
						if job.prevPendingApprovalKey != "" {
							if hasAlert {
								cid, dismissKey := id, job.prevPendingApprovalKey
								_ = w.dispatchAlert(func() { _ = w.notifier.NotifyDismissApproval(cid, dismissKey) })
							}
							setPending("")
						}
						nextWaitingBg = true
						runningCount++
						if hasLive {
							w.dispatchLiveUpdate(details)
						}
					} else {
						setPending("")
						nextWaitingBg = false
						cid, dTitle, totalSteps := id, details.Title, details.TotalSteps
						if hasAlert && !w.dispatchAlert(func() { _ = w.notifier.NotifyCompleted(cid, dTitle, totalSteps) }) {
							enqueued = false
						}
						if hasLive {
							if !w.dispatchAlert(func() { w.notifier.EmitLiveEnd(context.Background(), cid, dTitle, "COMPLETED", totalSteps) }) {
								enqueued = false
							}
						}
					}
				} else if job.prevPendingApprovalKey != "" {
					if hasAlert {
						cid, dismissKey := id, job.prevPendingApprovalKey
						_ = w.dispatchAlert(func() { _ = w.notifier.NotifyDismissApproval(cid, dismissKey) })
					}
					setPending("")
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
						setPending(approvalKeyForDetails(details))
						cid, dTitle, pi := id, details.Title, details.PendingInteraction
						if hasAlert && !w.dispatchAlert(func() { _ = w.notifier.NotifyAction(cid, dTitle, pi) }) {
							enqueued = false
						}
						if hasLive {
							w.dispatchLiveUpdate(details)
						}
					} else if details.CanProceed && !details.ProceedDismissed {
						setPending(approvalKeyForDetails(details))
						cid, dTitle, totalSteps := id, details.Title, details.TotalSteps
						if hasAlert && !w.dispatchAlert(func() { _ = w.notifier.NotifyProceed(cid, dTitle, totalSteps) }) {
							enqueued = false
						}
						if hasLive {
							w.dispatchLiveUpdate(details)
						}
					} else if (details.Status == "CASCADE_RUN_STATUS_FAILED" || details.Status == "CASCADE_RUN_STATUS_ERROR" || details.HasError) && details.TotalSteps > 0 {
						setPending("")
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
						setPending("")
						cid, dTitle, totalSteps := id, details.Title, details.TotalSteps
						if hasAlert && !w.dispatchAlert(func() { _ = w.notifier.NotifyCompleted(cid, dTitle, totalSteps) }) {
							enqueued = false
						}
						if hasLive {
							if !w.dispatchAlert(func() { w.notifier.EmitLiveEnd(context.Background(), cid, dTitle, "COMPLETED", totalSteps) }) {
								enqueued = false
							}
						}
					}
					if !enqueued {
						runningCount++
						continue
					}
					nextWaitingBg = false
				} else {
					if job.prevPendingApprovalKey != "" && approvalKeyForDetails(details) == "" {
						if hasAlert {
							cid, dismissKey := id, job.prevPendingApprovalKey
							_ = w.dispatchAlert(func() { _ = w.notifier.NotifyDismissApproval(cid, dismissKey) })
						}
						setPending("")
					}
					runningCount++ // Still waiting, keep scan frequency fast (1500ms)
					if hasLive {
						w.dispatchLiveUpdate(details)
					}
				}
			}
		} else if job.prevPendingApprovalKey != "" {
			// Session was idle/waiting with a tracked approval; detect external resolution (e.g., rejected/cancelled on desktop).
			if fetchErr == nil && details != nil {
				curPendingKey := approvalKeyForDetails(details)
				if curPendingKey == "" {
					if hasAlert {
						cid, dismissKey := id, job.prevPendingApprovalKey
						_ = w.dispatchAlert(func() { _ = w.notifier.NotifyDismissApproval(cid, dismissKey) })
					}
					if hasLive {
						if inProg, _ := IsCascadeInProgress(details, job.hasSubagent); inProg {
							w.dispatchLiveUpdate(details)
						} else if la := w.notifier.LiveActivity(); la != nil && la.HasStateForCascade(id) {
							finalStatus := "COMPLETED"
							if details.Status == "CASCADE_RUN_STATUS_FAILED" || details.Status == "CASCADE_RUN_STATUS_ERROR" || details.HasError {
								finalStatus = "FAILED"
							}
							cid, dTitle, st := id, details.Title, details.TotalSteps
							_ = w.dispatchAlert(func() { la.EndSession(context.Background(), cid, dTitle, finalStatus, st) })
						}
					}
					setPending("")
				} else if curPendingKey != job.prevPendingApprovalKey {
					setPending(curPendingKey)
					if hasLive {
						w.dispatchLiveUpdate(details)
					}
					if hasAlert && now.After(job.cooldownUntil) {
						if details.PendingInteraction != nil {
							cid, dTitle, pi := id, details.Title, details.PendingInteraction
							_ = w.dispatchAlert(func() { _ = w.notifier.NotifyAction(cid, dTitle, pi) })
						} else if details.CanProceed && !details.ProceedDismissed {
							cid, dTitle, st := id, details.Title, details.TotalSteps
							_ = w.dispatchAlert(func() { _ = w.notifier.NotifyProceed(cid, dTitle, st) })
						}
					}
				}
			}
		}
			if !nextWaitingBg && nextPendingKey == "" {
				if aud := w.notifier.CacheMissAuditor(); aud != nil && aud.HasOngoingEpisode(id) {
					cid := id
					finalStatus := status
					finalSteps := steps
					if fetchErr == nil && details != nil {
						if details.Status != "" {
							finalStatus = details.Status
						}
						if details.TotalSteps > finalSteps {
							finalSteps = details.TotalSteps
						}
					}
					ts := now
					_ = w.dispatchAlert(func() {
						w.notifier.FinalizeCacheMissSession(cid, finalStatus, finalSteps, ts)
					})
				}
			}
		}

		w.mu.Lock()
		if !job.existed || prev.lastStatus != status || prev.lastSteps != steps || status == "CASCADE_RUN_STATUS_RUNNING" || nextWaitingBg || prev.waitingForBackground != nextWaitingBg {
			prev.lastUpdated = now
		}
		prev.waitingForBackground = nextWaitingBg
		prev.pendingApprovalKey = nextPendingKey
		prev.pendingApprovalSince = nextPendingSince
		prev.fetchRetryCount = nextRetries
		prev.lastCacheGuardCheckAt = nextCacheGuardCheckAt
		prev.lastProgressAt = nextProgressAt
		prev.lastProgressSig = nextProgressSig
		prev.lastStatus = status
		prev.lastSteps = steps
		if title != "" && title != MsgUntitledSession {
			prev.title = title
		}
		w.mu.Unlock()
	}

	if runningCount > 0 || activitySeen {
		w.mu.Lock()
		if now.After(w.lastActivityAt) {
			w.lastActivityAt = now
		}
		w.mu.Unlock()
	}
	w.notifyActivity(runningCount > 0)

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
	now := w.now()

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
