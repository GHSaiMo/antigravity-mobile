package notifier

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/proxy"
)

const (
	liveActivityMinRoutineInterval = 18 * time.Second
	liveActivityStaleTTL           = 30 * time.Minute
	liveActivityEndDismissDelaySec = 15
	liveActivityStartCooldown      = 10 * time.Minute
)

// LiveActivityAttributes mirrors Swift AgentActivityAttributes static fields for push-to-start.
type LiveActivityAttributes struct {
	ConversationTitle string `json:"conversationTitle"`
	CascadeID         string `json:"cascadeId"`
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
	lastHash     string
	lastStatus   string
	lastPending  bool
	lastSentAt   time.Time
	startedAt    map[string]time.Time // deviceID -> last push-to-start time for this cascade
}

// LiveActivityDispatcher translates Antigravity trajectory states into throttled, budget-aware APNs Live Activity pushes.
type LiveActivityDispatcher struct {
	mu      sync.Mutex
	client  *APNsRelayClient
	enabled bool
	nowFunc func() time.Time
	tracks  map[string]*liveActivityTrack // cascadeID -> tracking state
}

// NewLiveActivityDispatcher constructs a LiveActivityDispatcher backed by client.
func NewLiveActivityDispatcher(client *APNsRelayClient, enabled bool) *LiveActivityDispatcher {
	return &LiveActivityDispatcher{
		client:  client,
		enabled: enabled,
		nowFunc: time.Now,
		tracks:  make(map[string]*liveActivityTrack),
	}
}

// IsActive reports whether Live Activity pushing is enabled and at least one iOS device is subscribed.
func (d *LiveActivityDispatcher) IsActive() bool {
	return d != nil && d.enabled && d.client != nil && d.client.HasLiveActivityTargets()
}

// BuildContentState builds a compact (< 1.5 KB) LiveActivityContentState from TrajectoryDetails.
func BuildContentState(details *proxy.TrajectoryDetails, now time.Time) LiveActivityContentState {
	title := strings.TrimSpace(details.Title)
	if title == "" {
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
			tTitle = "后台任务"
		}
		tTitle = truncateString(tTitle, 40)
		tCmd := truncateString(strings.TrimSpace(t.CommandLine), 60)
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

	latestAction := buildLatestActionSummary(details, activeTaskTitle)

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

func buildLatestActionSummary(details *proxy.TrajectoryDetails, activeTaskTitle string) string {
	if pi := details.PendingInteraction; pi != nil {
		target := strings.TrimSpace(pi.Target)
		summary := target
		if summary == "" {
			summary = strings.TrimSpace(pi.Title)
		}
		if summary == "" {
			return "等待用户审批操作"
		}
		return "待审批: " + truncateString(summary, 80)
	}
	if details.CanProceed {
		return "方案待确认，可点击 Proceed"
	}
	if activeTaskTitle != "" {
		return activeTaskTitle
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
		return fmt.Sprintf("正在执行第 %d 步...", details.TotalSteps)
	}
	return "Agent 正在执行..."
}

// SyncFromDetails dispatches a Live Activity start or update push when warranted by state change and throttling rules.
func (d *LiveActivityDispatcher) SyncFromDetails(ctx context.Context, details *proxy.TrajectoryDetails) {
	if !d.IsActive() || details == nil || strings.TrimSpace(details.CascadeID) == "" {
		return
	}

	cascadeID := details.CascadeID
	targets := d.client.Store().ListLiveActivityTargets(cascadeID, true)
	if len(targets) == 0 {
		return
	}

	now := d.nowFunc()
	state := BuildContentState(details, now)
	stateHash := hashContentState(state)

	d.mu.Lock()
	tr, exists := d.tracks[cascadeID]
	if !exists {
		tr = &liveActivityTrack{startedAt: make(map[string]time.Time)}
		d.tracks[cascadeID] = tr
	}

	isCriticalTransition := !exists ||
		tr.lastStatus != state.Status ||
		tr.lastPending != state.HasPendingAction

	// Filter which targets actually need a push right now
	var toSend []APNsLiveActivityTarget
	for _, t := range targets {
		if t.IsStart {
			// Only remote-start once per cascade cooldown window so we never spam duplicate activities
			if lastStart, ok := tr.startedAt[t.DeviceID]; ok && now.Sub(lastStart) < liveActivityStartCooldown {
				continue
			}
			tr.startedAt[t.DeviceID] = now
			toSend = append(toSend, t)
		} else {
			if stateHash == tr.lastHash {
				continue
			}
			if !isCriticalTransition && now.Sub(tr.lastSentAt) < liveActivityMinRoutineInterval {
				continue
			}
			toSend = append(toSend, t)
		}
	}

	if len(toSend) == 0 {
		d.mu.Unlock()
		return
	}

	tr.lastHash = stateHash
	tr.lastStatus = state.Status
	tr.lastPending = state.HasPendingAction
	tr.lastSentAt = now
	d.mu.Unlock()

	priority := 5
	if isCriticalTransition {
		priority = 10
	}

	for _, target := range toSend {
		req := APNsLiveActivityRequest{
			Event:        "update",
			Priority:     priority,
			Timestamp:    now.Unix(),
			StaleDate:    now.Add(liveActivityStaleTTL).Unix(),
			ContentState: state,
		}
		if target.IsStart {
			req.Event = "start"
			req.Priority = 10
			req.AttributesType = "AgentActivityAttributes"
			req.Attributes = &LiveActivityAttributes{
				ConversationTitle: state.ConversationTitle,
				CascadeID:         cascadeID,
			}
			req.InputPushToken = true
		}

		if err := d.client.SendLiveActivity(ctx, target, req); err != nil {
			slog.Warn("[APNs] Live Activity push failed",
				"event", req.Event,
				"device_id", target.DeviceID,
				"cascade_id", shortID(cascadeID),
				"err", err,
			)
		}
	}
}

// EndSession sends an APNs Live Activity "end" event for all devices holding an active update token for cascadeID.
func (d *LiveActivityDispatcher) EndSession(ctx context.Context, cascadeID, title, finalStatus string, stepCount int) {
	if !d.IsActive() || strings.TrimSpace(cascadeID) == "" {
		return
	}

	// Only send "end" to devices that already hold an active update token (never push-to-start an ended session).
	targets := d.client.Store().ListLiveActivityTargets(cascadeID, false)

	d.mu.Lock()
	delete(d.tracks, cascadeID)
	d.mu.Unlock()

	if len(targets) == 0 {
		return
	}

	now := d.nowFunc()
	summary := "任务已完成"
	switch finalStatus {
	case "FAILED":
		summary = "执行遇到错误"
	case "CANCELLED":
		summary = "任务已终止"
	}
	if strings.TrimSpace(title) == "" {
		title = "Multigravity Agent"
	}
	if stepCount < 1 {
		stepCount = 1
	}

	finalState := LiveActivityContentState{
		ConversationTitle: truncateString(title, 40),
		Status:            finalStatus,
		StepCount:         stepCount,
		LatestAction:      summary,
		RunningTaskCount:  0,
		RunningTasks:      []LiveActivityTaskSnapshot{},
		HasPendingAction:  false,
		LastUpdated:       float64(now.Unix()),
	}

	for _, target := range targets {
		req := APNsLiveActivityRequest{
			Event:         "end",
			Priority:      10,
			Timestamp:     now.Unix(),
			DismissalDate: now.Add(liveActivityEndDismissDelaySec * time.Second).Unix(),
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
}

func hashContentState(s LiveActivityContentState) string {
	// Zero out LastUpdated so identical content states hash identically
	s.LastUpdated = 0
	b, _ := json.Marshal(s)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:12])
}
