package notifier

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/i18n"
	"antigravity-mobile/internal/proxy"
)

const (
	liveActivityMinRoutineInterval  = 18 * time.Second
	liveActivityFailBaseBackoff     = 15 * time.Second
	liveActivityFailMaxBackoff      = 5 * time.Minute
	liveActivityStaleTTL            = 30 * time.Minute
	liveActivityEndDismissDelaySec  = 15
	maxLiveActivityCoalesceWorkers  = 4
	// MaxLiveActivityCards caps concurrent Live Activity cards (WAITING_APPROVAL > most recently active).
	MaxLiveActivityCards  = 3
	liveActivityMemberTTL = 90 * time.Second
)

type liveActivityMember struct {
	pending  bool
	lastSeen time.Time
	title    string
	steps    int
}

// LiveActivityAttributes mirrors Swift AgentActivityAttributes static fields for push-to-start.
type LiveActivityAttributes struct {
	ConversationTitle string `json:"conversationTitle"`
	CascadeID         string `json:"cascadeId"`
	GatewayID         string `json:"gatewayId,omitempty"`
	GatewayLabel      string `json:"gatewayLabel,omitempty"`
}

// LiveActivityTaskSnapshot mirrors Swift AgentTaskSnapshot inside AgentActivityAttributes.ContentState.
type LiveActivityTaskSnapshot struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Command   string `json:"command,omitempty"`
	IsWaiting bool   `json:"isWaiting"`
}

// LiveActivityContentState mirrors Swift AgentActivityAttributes.ContentState field-for-field.
// LastUpdated is encoded as Unix epoch seconds (1970); Swift ContentState.init(from:) decodes it safely.
type LiveActivityContentState struct {
	ConversationTitle string                     `json:"conversationTitle"`
	Status            string                     `json:"status"` // "RUNNING", "TASK_RUNNING", "WAITING_APPROVAL", "COMPLETED", "FAILED", "CANCELLED"
	StepCount         int                        `json:"stepCount"`
	LatestAction      string                     `json:"latestAction"`
	RunningTaskCount  int                        `json:"runningTaskCount"`
	RunningTasks      []LiveActivityTaskSnapshot `json:"runningTasks"`
	ActiveTaskTitle   string                     `json:"activeTaskTitle,omitempty"`
	ActiveTaskCommand string                     `json:"activeTaskCommand,omitempty"`
	HasPendingAction  bool                       `json:"hasPendingAction"`
	PendingStepIndex  *int                       `json:"pendingStepIndex,omitempty"`
	PendingActionKind string                     `json:"pendingActionKind,omitempty"`
	LastUpdated       float64                    `json:"lastUpdated"`
}

type liveActivityTrack struct {
	lastHashByDevice map[string]string    // deviceID -> last pushed ContentState hash
	lastStatus       string
	lastPending      bool
	lastSentAt       time.Time
	startedAt        map[string]time.Time // deviceID -> last push-to-start time for this cascade
	retryAfter       map[string]time.Time // deviceID -> earliest retry timestamp after push failure
	failCount        map[string]int       // deviceID -> consecutive failure count
}

// LiveActivityDispatcher translates Antigravity trajectory states into throttled, budget-aware APNs Live Activity pushes.
type LiveActivityDispatcher struct {
	mu               sync.Mutex
	client           *APNsRelayClient
	enabled          bool
	redact           bool
	gatewayID        string
	gatewayName      string
	nowFunc          func() time.Time
	tracks           map[string]*liveActivityTrack  // cascadeID -> tracking state
	endedAt          map[string]time.Time           // cascadeID -> recent EndSession timestamp
	cascadeStripes   [64]sync.Mutex                 // fixed striped locks for per-cascade send serialization
	members          map[string]*liveActivityMember // cascades admitted to hold a card (cap MaxLiveActivityCards)
	coalesceMu          sync.Mutex
	pendingLatest       map[string]*proxy.TrajectoryDetails
	detailsPreprocessor func(ctx context.Context, details *proxy.TrajectoryDetails, fallbackLang string)
	coalesceInFlight map[string]bool
	coalesceWorkers  int
}

// SetRedact enables or disables strict push redaction for Live Activity updates and Push-to-Start payloads.
func (d *LiveActivityDispatcher) SetRedact(enabled bool) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.redact = enabled
	d.mu.Unlock()
}

// Redact reports whether Live Activity redaction is active.
func (d *LiveActivityDispatcher) Redact() bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.redact
}

// admitLocked decides whether cascadeID may hold a card. Caller holds d.mu.
// Returns the evicted cascade (if any) that must be ended.
func (d *LiveActivityDispatcher) admitLocked(cascadeID string, pending bool, title string, steps int, now time.Time) (bool, string, *liveActivityMember) {
	for cid, m := range d.members {
		if cid != cascadeID && now.Sub(m.lastSeen) > liveActivityMemberTTL {
			delete(d.members, cid)
		}
	}
	if m, ok := d.members[cascadeID]; ok {
		m.pending, m.lastSeen, m.title, m.steps = pending, now, title, steps
		return true, "", nil
	}
	nm := &liveActivityMember{pending: pending, lastSeen: now, title: title, steps: steps}
	if len(d.members) < MaxLiveActivityCards {
		d.members[cascadeID] = nm
		return true, "", nil
	}
	if !pending {
		return false, "", nil
	}
	victim := ""
	for cid, m := range d.members {
		if m.pending {
			continue
		}
		if victim == "" || m.lastSeen.Before(d.members[victim].lastSeen) {
			victim = cid
		}
	}
	if victim == "" {
		return false, "", nil
	}
	vm := d.members[victim]
	delete(d.members, victim)
	d.members[cascadeID] = nm
	return true, victim, vm
}

// NewLiveActivityDispatcher constructs a LiveActivityDispatcher backed by client.
func NewLiveActivityDispatcher(client *APNsRelayClient, enabled bool) *LiveActivityDispatcher {
	return &LiveActivityDispatcher{
		client:           client,
		enabled:          enabled,
		nowFunc:          time.Now,
		tracks:           make(map[string]*liveActivityTrack),
		endedAt:          make(map[string]time.Time),
		members:          make(map[string]*liveActivityMember),
		pendingLatest:    make(map[string]*proxy.TrajectoryDetails),
		coalesceInFlight: make(map[string]bool),
	}
}

// SetGatewayIdentity configures the stable gateway ID and display name embedded in Live Activity Push-to-Start attributes.
func (d *LiveActivityDispatcher) SetGatewayIdentity(gatewayID, gatewayName string) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.gatewayID = strings.TrimSpace(gatewayID)
	d.gatewayName = strings.TrimSpace(gatewayName)
	d.mu.Unlock()
}

func (d *LiveActivityDispatcher) lockCascade(cascadeID string) func() {
	h := fnv.New32a()
	_, _ = h.Write([]byte(cascadeID))
	idx := h.Sum32() & 63
	d.cascadeStripes[idx].Lock()
	return d.cascadeStripes[idx].Unlock
}

// EnqueueLatest coalesces high-frequency trajectory updates per cascade (single-slot latest-wins)
// and dispatches them asynchronously using a bounded, self-draining worker pool.
func (d *LiveActivityDispatcher) EnqueueLatest(details *proxy.TrajectoryDetails) {
	if !d.IsActive() || details == nil || strings.TrimSpace(details.CascadeID) == "" {
		return
	}
	if strings.TrimSpace(details.ParentConversation) != "" {
		return
	}
	snap := *details
	if len(details.RunningTasks) > 0 {
		snap.RunningTasks = append([]proxy.RunningTaskItem(nil), details.RunningTasks...)
	}
	if details.PendingInteraction != nil {
		pi := *details.PendingInteraction
		snap.PendingInteraction = &pi
	}

	d.coalesceMu.Lock()
	if d.pendingLatest == nil {
		d.pendingLatest = make(map[string]*proxy.TrajectoryDetails)
	}
	if d.coalesceInFlight == nil {
		d.coalesceInFlight = make(map[string]bool)
	}
	d.pendingLatest[snap.CascadeID] = &snap
	if !d.coalesceInFlight[snap.CascadeID] && d.coalesceWorkers < maxLiveActivityCoalesceWorkers {
		d.coalesceWorkers++
		go d.runCoalesceWorker()
	}
	d.coalesceMu.Unlock()
}

func (d *LiveActivityDispatcher) runCoalesceWorker() {
	for {
		d.coalesceMu.Lock()
		var pickedID string
		var pickedSnap *proxy.TrajectoryDetails
		for cid, s := range d.pendingLatest {
			if !d.coalesceInFlight[cid] {
				pickedID = cid
				pickedSnap = s
				delete(d.pendingLatest, cid)
				d.coalesceInFlight[cid] = true
				break
			}
		}
		if pickedSnap == nil {
			d.coalesceWorkers--
			d.coalesceMu.Unlock()
			return
		}
		d.coalesceMu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		d.SyncFromDetails(ctx, pickedSnap)
		cancel()

		d.coalesceMu.Lock()
		delete(d.coalesceInFlight, pickedID)
		d.coalesceMu.Unlock()
	}
}

// HasStateForCascade reports whether any device holds a Live Activity update token or Push-to-Start marker for cascadeID.
func (d *LiveActivityDispatcher) HasStateForCascade(cascadeID string) bool {
	if !d.IsActive() || strings.TrimSpace(cascadeID) == "" {
		return false
	}
	return d.client.Store().HasLiveActivityStateForCascade(cascadeID)
}

// IsActive reports whether Live Activity pushing is enabled and at least one iOS device is subscribed.
func (d *LiveActivityDispatcher) IsActive() bool {
	return d != nil && d.enabled && d.client != nil && d.client.HasLiveActivityTargets()
}

// SetDetailsPreprocessor registers an optional callback run asynchronously right before building
// non-redacted Live Activity ContentState (used to apply/wait briefly for AI Assist translations).
func (d *LiveActivityDispatcher) SetDetailsPreprocessor(fn func(ctx context.Context, details *proxy.TrajectoryDetails, fallbackLang string)) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.detailsPreprocessor = fn
	d.mu.Unlock()
}

// BuildContentState builds a compact (< 1.5 KB) LiveActivityContentState from TrajectoryDetails
// using the gateway's active language.
func BuildContentState(details *proxy.TrajectoryDetails, now time.Time) LiveActivityContentState {
	return BuildContentStateForLang(details, now, i18n.GetLang())
}

func isLiveActivityUntitledTitle(title string) bool {
	t := strings.TrimSpace(title)
	return t == "" ||
		t == MsgUntitledSession ||
		t == "当前会话" ||
		t == "無題のセッション" ||
		t == "現在のセッション" ||
		strings.EqualFold(t, "Untitled") ||
		strings.EqualFold(t, "Untitled Session") ||
		strings.EqualFold(t, "Current Session") ||
		strings.EqualFold(t, "New Chat")
}

// BuildContentStateForLang builds a compact (< 1.5 KB) LiveActivityContentState localized for lang ("zh"/"en"/"ja").
func BuildContentStateForLang(details *proxy.TrajectoryDetails, now time.Time, lang string) LiveActivityContentState {
	resolvedLang := i18n.ResolveLang(lang)
	title := strings.TrimSpace(SanitizePushText(details.Title))
	if isLiveActivityUntitledTitle(title) {
		title = "Multigravity Agent"
	}
	title = truncateString(title, 40)

	var blockingTasks []proxy.RunningTaskItem
	for _, t := range details.RunningTasks {
		if !t.IsDaemon {
			blockingTasks = append(blockingTasks, t)
		}
	}

	hasPending := details.CanProceed || details.PendingInteraction != nil
	isRunning := details.Status == "CASCADE_RUN_STATUS_RUNNING"

	status := "RUNNING"
	if hasPending {
		status = "WAITING_APPROVAL"
	} else if len(blockingTasks) > 0 && !isRunning {
		status = "TASK_RUNNING"
	} else if details.Status == "CASCADE_RUN_STATUS_FAILED" || details.Status == "CASCADE_RUN_STATUS_ERROR" || details.HasError {
		status = "FAILED"
	} else if !isRunning {
		status = "COMPLETED"
	}

	var pendingStep *int
	var pendingKind string
	if details.PendingInteraction != nil {
		idx := details.PendingInteraction.StepIndex
		pendingStep = &idx
		pendingKind = details.PendingInteraction.Type
	} else if details.CanProceed {
		idx := details.TotalSteps
		pendingStep = &idx
		pendingKind = "proceed"
	}

	snapshots := make([]LiveActivityTaskSnapshot, 0, 2)
	var activeTaskTitle, activeTaskCmd string
	for i, t := range blockingTasks {
		tTitle := strings.TrimSpace(t.ToolSummary)
		if tTitle == "" {
			tTitle = strings.TrimSpace(t.ToolAction)
		}
		if tTitle == "" {
			tTitle = strings.TrimSpace(t.ToolName)
		}
		if tTitle == "" {
			tTitle = msgDefaultTaskFor(resolvedLang)
		}
		tTitle = truncateString(SanitizePushText(tTitle), 40)
		tCmd := truncateString(SanitizePushText(strings.TrimSpace(t.CommandLine)), 60)
		if i == 0 {
			activeTaskTitle = tTitle
			activeTaskCmd = tCmd
		}
		if i < 2 {
			snapshots = append(snapshots, LiveActivityTaskSnapshot{
				ID:        truncateString(t.ID, 32),
				Title:     tTitle,
				Command:   tCmd,
				IsWaiting: false,
			})
		}
	}

	latestAction := SanitizePushText(buildLatestActionSummaryForLang(details, activeTaskTitle, resolvedLang))

	stepCount := details.TotalSteps
	if stepCount < 1 {
		stepCount = 1
	}

	return LiveActivityContentState{
		ConversationTitle: title,
		Status:            status,
		StepCount:         stepCount,
		LatestAction:      truncateString(latestAction, 100),
		RunningTaskCount:  len(blockingTasks),
		RunningTasks:      snapshots,
		ActiveTaskTitle:   activeTaskTitle,
		ActiveTaskCommand: activeTaskCmd,
		HasPendingAction:  hasPending,
		PendingStepIndex:  pendingStep,
		PendingActionKind: pendingKind,
		LastUpdated:       float64(now.Unix()),
	}
}

// BuildRedactedContentStateForLang builds a privacy-redacted LiveActivityContentState that strips
// conversation titles, commands, paths, focus text, and agent prose, retaining only status,
// risk/approval category, and numeric progress counts (e.g. "3/5" or step counts).
func BuildRedactedContentStateForLang(details *proxy.TrajectoryDetails, now time.Time, lang string) LiveActivityContentState {
	resolvedLang := i18n.ResolveLang(lang)

	var blockingCount int
	for _, t := range details.RunningTasks {
		if !t.IsDaemon {
			blockingCount++
		}
	}

	hasPending := details.CanProceed || details.PendingInteraction != nil
	isRunning := details.Status == "CASCADE_RUN_STATUS_RUNNING"

	status := "RUNNING"
	if hasPending {
		status = "WAITING_APPROVAL"
	} else if blockingCount > 0 && !isRunning {
		status = "TASK_RUNNING"
	} else if details.Status == "CASCADE_RUN_STATUS_FAILED" || details.Status == "CASCADE_RUN_STATUS_ERROR" || details.HasError {
		status = "FAILED"
	} else if !isRunning {
		status = "COMPLETED"
	}

	var pendingStep *int
	var pendingKind string
	if details.PendingInteraction != nil {
		idx := details.PendingInteraction.StepIndex
		pendingStep = &idx
		pendingKind = details.PendingInteraction.Type
	} else if details.CanProceed {
		idx := details.TotalSteps
		pendingStep = &idx
		pendingKind = "proceed"
	}

	latestAction := buildRedactedLatestActionForLang(details, blockingCount, resolvedLang)
	stepCount := details.TotalSteps
	if stepCount < 1 {
		stepCount = 1
	}

	return LiveActivityContentState{
		ConversationTitle: msgRedactedSessionTitleFor(resolvedLang),
		Status:            status,
		StepCount:         stepCount,
		LatestAction:      truncateString(latestAction, 100),
		RunningTaskCount:  blockingCount,
		RunningTasks:      []LiveActivityTaskSnapshot{},
		HasPendingAction:  hasPending,
		PendingStepIndex:  pendingStep,
		PendingActionKind: pendingKind,
		LastUpdated:       float64(now.Unix()),
	}
}

func buildRedactedLatestActionForLang(details *proxy.TrajectoryDetails, blockingCount int, lang string) string {
	resolvedLang := i18n.ResolveLang(lang)
	if pi := details.PendingInteraction; pi != nil {
		if pi.ApprovalPreview != nil && pi.ApprovalPreview.RiskLevel == "high" && pi.Type != "ask_question" {
			return msgRedactedLiveActivityHighRiskFor(resolvedLang)
		}
		return i18n.T3For(resolvedLang, "等待用户审批操作", "Waiting for approval", "承認待ちです")
	}
	if details.CanProceed {
		return i18n.T3For(resolvedLang, "方案待确认，可点击 Proceed", "Plan ready — tap Proceed", "計画を確認して Proceed")
	}
	if details.ActivePlan != nil && details.ActivePlan.TotalCount > 0 {
		return fmt.Sprintf("%d/%d", details.ActivePlan.DoneCount, details.ActivePlan.TotalCount)
	}
	if blockingCount > 0 {
		return msgRedactedLiveActivityRunningTasksFor(resolvedLang, blockingCount)
	}
	if details.TotalSteps > 0 {
		return i18n.Sprintf3For(resolvedLang, "正在执行第 %d 步...", "Running step %d...", "ステップ %d を実行中...", details.TotalSteps)
	}
	return i18n.T3For(resolvedLang, "Agent 正在执行...", "Agent is working...", "Agent が実行中...")
}

func buildLatestActionSummaryForLang(details *proxy.TrajectoryDetails, activeTaskTitle, lang string) string {
	resolvedLang := i18n.ResolveLang(lang)
	if pi := details.PendingInteraction; pi != nil {
		target := strings.TrimSpace(SanitizePushText(pi.Target))
		summary := target
		if summary == "" {
			summary = strings.TrimSpace(SanitizePushText(pi.Title))
		}
		if summary == "" {
			return i18n.T3For(resolvedLang, "等待用户审批操作", "Waiting for approval", "承認待ちです")
		}
		return i18n.Sprintf3For(resolvedLang, "待审批: %s", "Approval: %s", "承認待ち: %s", truncateString(summary, 80))
	}
	if details.CanProceed {
		return i18n.T3For(resolvedLang, "方案待确认，可点击 Proceed", "Plan ready — tap Proceed", "計画を確認して Proceed")
	}
	if activeTaskTitle != "" {
		return activeTaskTitle
	}
	if summary := buildPlanOrFocusSummary(details); summary != "" {
		return summary
	}
	if details.TaskProgress != nil {
		if s := strings.TrimSpace(details.TaskProgress.TaskStatus); s != "" {
			return s
		}
		if s := strings.TrimSpace(details.TaskProgress.TaskName); s != "" {
			return s
		}
	}
	if latestText, _ := ExtractLatestAgentResponse(details.Steps); latestText != "" {
		// Use first non-empty line to keep Lock Screen / Dynamic Island crisp
		for _, line := range strings.Split(latestText, "\n") {
			clean := strings.TrimSpace(strings.TrimLeft(line, "#*- >`"))
			if clean != "" {
				return clean
			}
		}
	}
	if details.TotalSteps > 0 {
		return i18n.Sprintf3For(resolvedLang, "正在执行第 %d 步...", "Running step %d...", "ステップ %d を実行中...", details.TotalSteps)
	}
	return i18n.T3For(resolvedLang, "Agent 正在执行...", "Agent is working...", "Agent が実行中...")
}

func buildPlanOrFocusSummary(details *proxy.TrajectoryDetails) string {
	if details == nil {
		return ""
	}
	focus := strings.TrimSpace(details.ActiveFocus)
	plan := details.ActivePlan
	if plan != nil && len(plan.Items) > 0 {
		prefix := ""
		if plan.TotalCount > 0 {
			prefix = fmt.Sprintf("%d/%d", plan.DoneCount, plan.TotalCount)
		}
		if focus != "" {
			if prefix != "" {
				return prefix + " · " + focus
			}
			return focus
		}
		for _, it := range plan.Items {
			if it.Status == "in_progress" && strings.TrimSpace(it.Title) != "" {
				if prefix != "" {
					return prefix + " · " + strings.TrimSpace(it.Title)
				}
				return strings.TrimSpace(it.Title)
			}
		}
	}
	return focus
}

// SyncFromDetails dispatches a Live Activity start or update push when warranted by state change and throttling rules.
func (d *LiveActivityDispatcher) SyncFromDetails(ctx context.Context, details *proxy.TrajectoryDetails) {
	if !d.IsActive() || details == nil || strings.TrimSpace(details.CascadeID) == "" {
		return
	}
	// Subagents share the parent conversation's Live Activity and never spawn separate cards.
	if strings.TrimSpace(details.ParentConversation) != "" {
		return
	}

	cascadeID := details.CascadeID
	defer d.lockCascade(cascadeID)()

	now := d.nowFunc()
	d.mu.Lock()
	if ended, ok := d.endedAt[cascadeID]; ok && now.Sub(ended) < 3*time.Second {
		d.mu.Unlock()
		return
	}
	d.mu.Unlock()

	targets := d.client.Store().ListLiveActivityTargetsAt(cascadeID, true, now)
	if len(targets) == 0 {
		return
	}

	redact := d.Redact()
	d.mu.Lock()
	preproc := d.detailsPreprocessor
	d.mu.Unlock()

	var baseState LiveActivityContentState
	if redact {
		baseState = BuildRedactedContentStateForLang(details, now, i18n.GetLang())
	} else {
		baseState = BuildContentState(details, now)
	}

	d.mu.Lock()
	allowed, victim, vm := d.admitLocked(cascadeID, baseState.HasPendingAction, baseState.ConversationTitle, baseState.StepCount, now)
	if !allowed {
		d.mu.Unlock()
		return
	}
	if victim != "" {
		delete(d.tracks, victim)
		d.endedAt[victim] = now
	}
	d.mu.Unlock()
	if victim != "" {
		d.client.Store().ClearLiveActivityStartedForCascade(victim)
		d.sendEnd(ctx, victim, vm.title, "COMPLETED", vm.steps, "", now, true)
	}

	d.mu.Lock()
	tr, exists := d.tracks[cascadeID]
	if !exists {
		tr = &liveActivityTrack{
			lastHashByDevice: make(map[string]string),
			startedAt:        make(map[string]time.Time),
			retryAfter:       make(map[string]time.Time),
			failCount:        make(map[string]int),
		}
		d.tracks[cascadeID] = tr
	} else if tr.lastHashByDevice == nil {
		tr.lastHashByDevice = make(map[string]string)
	}

	isCriticalTransition := !exists ||
		tr.lastStatus != baseState.Status ||
		tr.lastPending != baseState.HasPendingAction

	// Pre-filter targets by backoff, push-to-start TTL, and routine throttle before running per-device translation.
	var candidates []APNsLiveActivityTarget
	prevHashes := make(map[string]string, len(targets))
	for _, t := range targets {
		if until, ok := tr.retryAfter[t.DeviceID]; ok && now.Before(until) {
			continue
		}
		if t.IsStart {
			// Only remote-start once per working run (backed by both in-memory track and persisted store).
			if lastStart, ok := tr.startedAt[t.DeviceID]; ok && now.Sub(lastStart) < liveActivityStartRunTTL {
				continue
			}
			candidates = append(candidates, t)
		} else {
			if !isCriticalTransition && now.Sub(tr.lastSentAt) < liveActivityMinRoutineInterval {
				continue
			}
			prevHashes[t.DeviceID] = tr.lastHashByDevice[t.DeviceID]
			candidates = append(candidates, t)
		}
	}
	d.mu.Unlock()

	if len(candidates) == 0 {
		return
	}

	// Prepare per-device localized & translated ContentState outside d.mu, caching preprocessing by native locale.
	type preparedTarget struct {
		target APNsLiveActivityTarget
		state  LiveActivityContentState
		hash   string
	}
	preprocCache := make(map[string]*proxy.TrajectoryDetails)
	prepared := make([]preparedTarget, 0, len(candidates))
	for _, target := range candidates {
		eff := details
		if !redact && preproc != nil {
			devLocale := strings.TrimSpace(target.RawLocale)
			if devLocale == "" {
				devLocale = strings.TrimSpace(target.Lang)
			}
			cached, ok := preprocCache[devLocale]
			if !ok {
				work := *details
				preproc(ctx, &work, devLocale)
				cached = &work
				preprocCache[devLocale] = cached
			}
			eff = cached
		}

		uiLang := i18n.ResolveLang(target.Lang)
		var targetState LiveActivityContentState
		if redact {
			targetState = BuildRedactedContentStateForLang(eff, now, uiLang)
		} else {
			targetState = BuildContentStateForLang(eff, now, uiLang)
		}
		targetHash := hashContentState(targetState)
		if !target.IsStart && targetHash == prevHashes[target.DeviceID] {
			continue
		}
		prepared = append(prepared, preparedTarget{
			target: target,
			state:  targetState,
			hash:   targetHash,
		})
	}

	if len(prepared) == 0 {
		return
	}

	d.mu.Lock()
	if curTr := d.tracks[cascadeID]; curTr != nil {
		if curTr.lastHashByDevice == nil {
			curTr.lastHashByDevice = make(map[string]string)
		}
		for _, item := range prepared {
			if item.target.IsStart {
				curTr.startedAt[item.target.DeviceID] = now
			}
			curTr.lastHashByDevice[item.target.DeviceID] = item.hash
		}
		curTr.lastStatus = baseState.Status
		curTr.lastPending = baseState.HasPendingAction
		curTr.lastSentAt = now
	}
	d.mu.Unlock()

	priority := 5
	if isCriticalTransition {
		priority = 10
	}

	for _, item := range prepared {
		target := item.target
		targetState := item.state
		req := APNsLiveActivityRequest{
			Event:        "update",
			Priority:     priority,
			Timestamp:    now.Unix(),
			StaleDate:    now.Add(liveActivityStaleTTL).Unix(),
			ContentState: targetState,
		}
		if target.IsStart {
			// Persist Push-to-Start guard before network send so a concurrent gateway restart never re-starts this run.
			d.client.Store().MarkLiveActivityStarted(target.DeviceID, cascadeID, now)
			req.Event = "start"
			req.Priority = 10
			req.AttributesType = "AgentActivityAttributes"
			d.mu.Lock()
			gwID := d.gatewayID
			gwName := d.gatewayName
			d.mu.Unlock()
			gwLabel := ""
			if target.ShowGatewayLabel && !redact {
				gwLabel = strings.TrimSpace(target.GatewayLabel)
				if gwLabel == "" {
					gwLabel = gwName
				}
				gwLabel = truncateString(SanitizePushText(gwLabel), 24)
			}
			req.Attributes = &LiveActivityAttributes{
				ConversationTitle: targetState.ConversationTitle,
				CascadeID:         cascadeID,
				GatewayID:         gwID,
				GatewayLabel:      gwLabel,
			}
			// Apple liveactivitiesd strictly requires an aps.alert dictionary on event="start" (Push-to-Start);
			// omit Sound so starting a Live Activity stays silent and only completion/action alerts chime.
			req.Alert = &APNsLiveActivityAlert{
				Title: targetState.ConversationTitle,
				Body:  targetState.LatestAction,
			}
			req.InputPushToken = true
		}

		if err := d.client.SendLiveActivity(ctx, target, req); err != nil {
			if target.IsStart {
				d.client.Store().ClearLiveActivityStarted(target.DeviceID, cascadeID)
			}
			d.mu.Lock()
			if curTr := d.tracks[cascadeID]; curTr != nil {
				if target.IsStart {
					delete(curTr.startedAt, target.DeviceID)
				}
				delete(curTr.lastHashByDevice, target.DeviceID)
				fc := curTr.failCount[target.DeviceID] + 1
				curTr.failCount[target.DeviceID] = fc
				shift := fc - 1
				if shift > 4 {
					shift = 4
				}
				backoff := liveActivityFailBaseBackoff * time.Duration(1<<shift)
				if backoff > liveActivityFailMaxBackoff {
					backoff = liveActivityFailMaxBackoff
				}
				curTr.retryAfter[target.DeviceID] = now.Add(backoff)
			}
			d.mu.Unlock()
			slog.Warn("[APNs] Live Activity push failed",
				"event", req.Event,
				"device_id", target.DeviceID,
				"cascade_id", shortID(cascadeID),
				"err", err,
			)
		} else {
			d.mu.Lock()
			if curTr := d.tracks[cascadeID]; curTr != nil {
				delete(curTr.failCount, target.DeviceID)
				delete(curTr.retryAfter, target.DeviceID)
			}
			d.mu.Unlock()
		}
	}
}

// EndSession sends an APNs Live Activity "end" event for all devices holding an active update token for cascadeID.
func (d *LiveActivityDispatcher) EndSession(ctx context.Context, cascadeID, title, finalStatus string, stepCount int) {
	if !d.IsActive() || strings.TrimSpace(cascadeID) == "" {
		return
	}

	d.coalesceMu.Lock()
	delete(d.pendingLatest, cascadeID)
	d.coalesceMu.Unlock()

	defer d.lockCascade(cascadeID)()

	now := d.nowFunc()

	d.mu.Lock()
	delete(d.tracks, cascadeID)
	delete(d.members, cascadeID)
	d.endedAt[cascadeID] = now
	d.mu.Unlock()

	// Clear persisted Push-to-Start run markers so a future working run in this conversation can Push-to-Start cleanly.
	d.client.Store().ClearLiveActivityStartedForCascade(cascadeID)

	summary := ""
	d.sendEnd(ctx, cascadeID, title, finalStatus, stepCount, summary, now, false)
}

func liveActivityEndSummary(lang, finalStatus string, quiet bool) string {
	resolved := i18n.ResolveLang(lang)
	if quiet {
		return i18n.T3For(resolved, "已让位给需要处理的会话", "Yielded to a session needing attention", "対応が必要なセッションに切替え")
	}
	switch finalStatus {
	case "FAILED":
		return i18n.T3For(resolved, "执行遇到错误", "Execution error", "実行エラー")
	case "CANCELLED":
		return i18n.T3For(resolved, "任务已终止", "Task stopped", "タスク停止")
	default:
		return i18n.T3For(resolved, "任务已完成", "Task completed", "タスク完了")
	}
}

// sendEnd pushes an "end" event to every device holding an update token for cascadeID.
// quiet=true dismisses immediately (used when a card is evicted by the cap).
func (d *LiveActivityDispatcher) sendEnd(ctx context.Context, cascadeID, title, finalStatus string, stepCount int, _ string, now time.Time, quiet bool) {
	targets := d.client.Store().ListLiveActivityTargetsAt(cascadeID, false, now)
	if len(targets) == 0 {
		return
	}

	dismissDelay := time.Duration(liveActivityEndDismissDelaySec) * time.Second
	if quiet {
		dismissDelay = 0
	}
	title = SanitizePushText(title)
	if isLiveActivityUntitledTitle(title) {
		title = "Multigravity Agent"
	}
	if stepCount < 1 {
		stepCount = 1
	}

	redact := d.Redact()
	truncatedTitle := truncateString(title, 40)
	for _, target := range targets {
		convTitle := truncatedTitle
		if redact {
			convTitle = msgRedactedSessionTitleFor(target.Lang)
		}
		finalState := LiveActivityContentState{
			ConversationTitle: convTitle,
			Status:            finalStatus,
			StepCount:         stepCount,
			LatestAction:      liveActivityEndSummary(target.Lang, finalStatus, quiet),
			RunningTaskCount:  0,
			RunningTasks:      []LiveActivityTaskSnapshot{},
			HasPendingAction:  false,
			LastUpdated:       float64(now.Unix()),
		}
		req := APNsLiveActivityRequest{
			Event:         "end",
			Priority:      10,
			Timestamp:     now.Unix(),
			DismissalDate: now.Add(dismissDelay).Unix(),
			ContentState:  finalState,
		}
		if err := d.client.SendLiveActivity(ctx, target, req); err != nil {
			slog.Warn("[APNs] Live Activity end push failed",
				"device_id", target.DeviceID,
				"cascade_id", shortID(cascadeID),
				"err", err,
			)
		} else {
			d.client.Store().RemoveLiveActivityToken(target.DeviceID, cascadeID)
		}
	}
}

// CleanupStaleTracks evicts old in-memory throttle records.
func (d *LiveActivityDispatcher) CleanupStaleTracks(maxAge time.Duration) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.nowFunc()
	for cid, tr := range d.tracks {
		if now.Sub(tr.lastSentAt) > maxAge {
			delete(d.tracks, cid)
		}
	}
	for cid, ended := range d.endedAt {
		if now.Sub(ended) > maxAge {
			delete(d.endedAt, cid)
		}
	}
}

func hashContentState(s LiveActivityContentState) string {
	// Zero out LastUpdated so identical content states hash identically
	s.LastUpdated = 0
	b, _ := json.Marshal(s)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:12])
}
