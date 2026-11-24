package proxy

import (
	"encoding/json"
	"strings"
)

// ArtifactItem represents a Markdown document or previewable artifact generated in a turn.
type ArtifactItem struct {
	URI             string `json:"uri"`
	Title           string `json:"title"`
	Summary         string `json:"summary,omitempty"`
	RequestFeedback bool   `json:"requestFeedback,omitempty"`
	UserFacing      bool   `json:"userFacing,omitempty"`
}

// SlashCommandMention represents a slash command or skill invoked in a USER_INPUT step.
type SlashCommandMention struct {
	Name           string `json:"name"`
	Icon           string `json:"icon,omitempty"`
	Kind           string `json:"kind,omitempty"` // "system" | "skill"
	DefinitionPath string `json:"definitionPath,omitempty"`
}

// ToolDetailItem describes a single tool call or step inside a folded tools/thinking capsule.
type ToolDetailItem struct {
	Name      string `json:"name"`
	Summary   string `json:"summary,omitempty"`
	Status    string `json:"status,omitempty"` // "running", "done", "error"
	Duration  string `json:"duration,omitempty"`
	StepIndex int    `json:"stepIndex,omitempty"`
}

// TaskProgressItem represents the latest CORTEX_STEP_TYPE_TASK_BOUNDARY task state.
type TaskProgressItem struct {
	TaskName    string `json:"taskName,omitempty"`
	TaskStatus  string `json:"taskStatus,omitempty"`
	TaskSummary string `json:"taskSummary,omitempty"`
	Mode        string `json:"mode,omitempty"`
}

// StepPlanItemEntry represents one checklist step emitted by update_steps.
type StepPlanItemEntry struct {
	Key    string `json:"key"`
	Title  string `json:"title"`
	Status string `json:"status"` // "pending" | "in_progress" | "done" | "skipped" | "failed"
}

// StepPlanItem represents the live checklist of steps for a turn (update_steps).
type StepPlanItem struct {
	AnchorStepIndex int                 `json:"anchorStepIndex"`
	Items           []StepPlanItemEntry `json:"items"`
	DoneCount       int                 `json:"doneCount"`
	TotalCount      int                 `json:"totalCount"` // excludes skipped, matching desktop
	SkippedCount    int                 `json:"skippedCount,omitempty"`
	FailedCount     int                 `json:"failedCount,omitempty"`
}

type CascadeMessageItem struct {
	ID               string                `json:"id"`
	Type             string                `json:"type"` // "user", "agent", "tools", "error", "subagent", "workflow", "step_plan"
	Role             string                `json:"role"`
	Text             string                `json:"text"`
	Content          string                `json:"content"`
	StepIndex        *int                  `json:"stepIndex,omitempty"`
	ToolCount        int                   `json:"toolCount,omitempty"`
	ToolNames        []string              `json:"toolNames,omitempty"`
	ToolDetails      []ToolDetailItem      `json:"toolDetails,omitempty"`
	Thinking         string                `json:"thinking,omitempty"`
	ThinkingDuration string                `json:"thinkingDuration,omitempty"`
	// FocusText is the latest live status summary from update_focus for a tools batch.
	FocusText string `json:"focusText,omitempty"`
	// StartedAt is the RFC3339 timestamp when a tools batch started (drives live elapsed timer).
	StartedAt     string                `json:"startedAt,omitempty"`
	Media         []string              `json:"media,omitempty"`         // Base64 thumbnails
	ImageURLs     []string              `json:"imageUrls,omitempty"`     // Markdown image URLs
	Artifacts     []ArtifactItem        `json:"artifacts,omitempty"`     // Linked/generated artifact cards
	SlashCommands []SlashCommandMention `json:"slashCommands,omitempty"` // Slash commands / skills invoked in this user message
	AttemptCount  int                   `json:"attemptCount,omitempty"`
	MaxAttempts   int                   `json:"maxAttempts,omitempty"`
	// Model / ModelName identify the model that actually generated an agent reply (from the step's
	// generatorModel): Model is the model id, ModelName its display name. Empty when unknown.
	Model     string `json:"model,omitempty"`
	ModelName string `json:"modelName,omitempty"`
	// Subagent is set for Type "subagent": one card per subagent a invoke_subagent step dispatched,
	// shown inline at that step's position (like the desktop client). Status is filled live before sending.
	Subagent *SubagentItem `json:"subagent,omitempty"`
	// Workflow is set for Type "workflow": represents dynamic multi-agent workflows (run_workflow) in Antigravity 2.26+.
	Workflow *WorkflowItem `json:"workflow,omitempty"`
	// StepPlan is set for Type "step_plan": live checklist card emitted by update_steps.
	StepPlan *StepPlanItem `json:"stepPlan,omitempty"`
	// Timestamp is the RFC3339 timestamp when this step or turn was created.
	Timestamp string `json:"timestamp,omitempty"`
	// ClientMessageID is the client-supplied UUID (Antigravity 2.30+ native field or mgy-client tag) for user messages.
	ClientMessageID string `json:"clientMessageId,omitempty"`
	// ErrorKind indicates structured error type: "context_overflow", "rate_limited", etc.
	ErrorKind string `json:"errorKind,omitempty"`
	// IsOverflow is true when the message indicates context length overflow
	IsOverflow bool `json:"isOverflow,omitempty"`
}

// QueuedMessageItem represents a pending follow-up user message queued for execution.
type QueuedMessageItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	// ClientMessageID is the X-Client-Message-Id the sender used, recovered from the message's tags.
	// Empty for messages not sent through the gateway. Lets clients match their own optimistic item
	// to the server id exactly instead of comparing text.
	ClientMessageID string   `json:"clientMessageId,omitempty"`
	CreatedAt       string   `json:"createdAt,omitempty"`
	Media           []string `json:"media,omitempty"`     // Base64 thumbnails or image data
	ImageURLs       []string `json:"imageUrls,omitempty"` // Image URLs
}

// RunningTaskItem represents an asynchronous background task currently running in Antigravity.
type RunningTaskItem struct {
	ID          string `json:"id"`
	StepIndex   int    `json:"stepIndex"`
	ToolName    string `json:"toolName,omitempty"`
	CommandLine string `json:"commandLine"`
	ToolSummary string `json:"toolSummary,omitempty"`
	ToolAction  string `json:"toolAction,omitempty"`
	LogURI      string `json:"logUri,omitempty"`
	StartedAt   string `json:"startedAt,omitempty"`
	IsDaemon    bool   `json:"isDaemon,omitempty"`
}

// HasActiveNonDaemonTasks reports whether tasks contains at least one running task that is NOT
// a standing daemon (IsDaemon=true). Daemon tasks stay RUNNING indefinitely across turns even
// when the cascade itself is IDLE, so they must not force the session into an active turn state.
func HasActiveNonDaemonTasks(tasks []RunningTaskItem) bool {
	for _, t := range tasks {
		if !t.IsDaemon {
			return true
		}
	}
	return false
}

// SubagentItem is one subagent a conversation dispatched through invoke_subagent.
// Static fields come from the INVOKE_SUBAGENT step; Status / StepCount / Title are filled from the live list snapshot.
type SubagentItem struct {
	ConversationID string `json:"conversationId"`
	TypeName       string `json:"typeName,omitempty"`
	Role           string `json:"role,omitempty"`
	Prompt         string `json:"prompt,omitempty"`
	ModelTier      string `json:"modelTier,omitempty"`
	// RuntimeModel is the actual model observed in the child conversation's gen_metadata (e.g.
	// "Gemini 4 Hydrogen (Orchestrator)"), revealing silent downgrades when ModelTier is "inherit".
	RuntimeModel string `json:"runtimeModel,omitempty"`
	StepIndex    int    `json:"stepIndex"`
	// Status: "running" | "done" | "gone" (no longer known to the language_server). "" when not yet resolved.
	Status    string `json:"status,omitempty"`
	StepCount int    `json:"stepCount,omitempty"`
	Title     string `json:"title,omitempty"`
}

type InteractionOption struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Scope  int    `json:"scope,omitempty"`
	IsDeny bool   `json:"isDeny,omitempty"`
}

type InteractionQuestion struct {
	Question           string              `json:"question"`
	IsMultiSelect      bool                `json:"isMultiSelect,omitempty"`
	Options            []InteractionOption `json:"options"`
	DefaultOptionID    string              `json:"defaultOptionId,omitempty"`
	HasWriteIn         bool                `json:"hasWriteIn"`
	WriteInLabel       string              `json:"writeInLabel,omitempty"`
	WriteInPlaceholder string              `json:"writeInPlaceholder,omitempty"`
}

// WorkflowPhaseItem represents a staged execution phase within a dynamic workflow.
type WorkflowPhaseItem struct {
	Name      string         `json:"name"`
	Logs      []string       `json:"logs,omitempty"`
	Subagents []SubagentItem `json:"subagents,omitempty"`
	Status    string         `json:"status,omitempty"` // "pending", "running", "done", "failed"
	Duration  string         `json:"duration,omitempty"`
}

// WorkflowApprovalDetails represents safety check details when run_workflow needs user confirmation.
type WorkflowApprovalDetails struct {
	Description     string   `json:"description,omitempty"`
	Script          string   `json:"script,omitempty"`
	ScriptPath      string   `json:"scriptPath,omitempty"`
	EstimatedPhases []string `json:"estimatedPhases,omitempty"`
	MaxConcurrency  int      `json:"maxConcurrency,omitempty"`
}

// WorkflowItem represents dynamic multi-agent workflow orchestration via run_workflow in Antigravity 2.26+.
type WorkflowItem struct {
	Description string              `json:"description,omitempty"`
	ScriptPath  string              `json:"scriptPath,omitempty"`
	Script      string              `json:"script,omitempty"`
	Phases      []WorkflowPhaseItem `json:"phases,omitempty"`
	Logs        []string            `json:"logs,omitempty"`
	Output      string              `json:"output,omitempty"`
	ResultJSON  string              `json:"resultJson,omitempty"` // finish(schema) structured return
	Status      string              `json:"status"`               // "running", "done", "error", "cancelled"
	Duration    string              `json:"duration,omitempty"`
	Error       string              `json:"error,omitempty"`
}

// PermissionGrantDetails represents structured permission declarations under PERMISSION_GRANTS_V2 (Antigravity 2.26+).
type PermissionGrantDetails struct {
	Protocol string `json:"protocol,omitempty"` // "PERMISSION_GRANTS_V2"
	Binary   string `json:"binary,omitempty"`   // "git", "gh", etc.
	Action   string `json:"action,omitempty"`   // "create", "update", "push", "merge", "dispatch", etc.
	Org      string `json:"org,omitempty"`
	Repo     string `json:"repo,omitempty"`
	Branch   string `json:"branch,omitempty"`
	PR       string `json:"pr,omitempty"`
	Path     string `json:"path,omitempty"`
	Ref      string `json:"ref,omitempty"`
	RawScope string `json:"rawScope,omitempty"`
}

// ApprovalPreviewDetails provides structured risk classification and a bounded compact diff
// so mobile users can verify destructive commands, sensitive paths, and file edits within seconds.
type ApprovalPreviewDetails struct {
	RiskLevel     string   `json:"riskLevel"` // "high", "medium", "low"
	RiskReasons   []string `json:"riskReasons,omitempty"`
	DisplayPath   string   `json:"displayPath,omitempty"`
	Cwd           string   `json:"cwd,omitempty"`
	AddedLines    int      `json:"addedLines,omitempty"`
	RemovedLines  int      `json:"removedLines,omitempty"`
	IsNewFile     bool     `json:"isNewFile,omitempty"`
	DiffPreview   string   `json:"diffPreview,omitempty"`
	DiffTruncated bool     `json:"diffTruncated,omitempty"`
}

type PendingInteraction struct {
	Type               string                   `json:"type"` // "permission", "ask_question", "file_permission", "run_command"
	TrajectoryID       string                   `json:"trajectoryId"`
	StepIndex          int                      `json:"stepIndex"`
	Title              string                   `json:"title"`
	Target             string                   `json:"target,omitempty"`
	Action             string                   `json:"action,omitempty"`
	Description        string                   `json:"description,omitempty"`
	PermissionGrant    *PermissionGrantDetails  `json:"permissionGrant,omitempty"`
	WorkflowApproval   *WorkflowApprovalDetails `json:"workflowApproval,omitempty"`
	ApprovalPreview    *ApprovalPreviewDetails  `json:"approvalPreview,omitempty"`
	Options            []InteractionOption      `json:"options"`
	IsMultiSelect      bool                    `json:"isMultiSelect,omitempty"`
	DefaultOptionID    string                  `json:"defaultOptionId,omitempty"`
	HasWriteIn         bool                    `json:"hasWriteIn"`
	WriteInLabel       string                  `json:"writeInLabel,omitempty"`
	WriteInPlaceholder string                  `json:"writeInPlaceholder,omitempty"`
	Questions          []InteractionQuestion   `json:"questions,omitempty"`
}

// ContextMeterItem represents the real-time context window usage and generation speed for the session.
type ContextMeterItem struct {
	UsedTokens        int64    `json:"usedTokens"`
	MaxTokens         int64    `json:"maxTokens"`
	Percentage        float64  `json:"percentage"`
	InputTokens       int64    `json:"inputTokens"`
	OutputTokens      int64    `json:"outputTokens"`
	CacheReadTokens   int64    `json:"cacheReadTokens"`
	ThinkingTokens    int64    `json:"thinkingTokens"`
	TimeToFirstToken  float64  `json:"timeToFirstTokenSec,omitempty"`
	StreamingDuration float64  `json:"streamingDurationSec,omitempty"`
	TokensPerSecond   float64  `json:"tokensPerSecond,omitempty"`
	IsOverflow        bool     `json:"isOverflow"`
	ErrorKind         string   `json:"errorKind,omitempty"`
	CacheHitRate      *float64 `json:"cacheHitRate,omitempty"`
	CacheMissStreak   int      `json:"cacheMissStreak,omitempty"`
	CacheMissTokens   int64    `json:"cacheMissTokens,omitempty"`
	CacheStatus            string   `json:"cacheStatus,omitempty"`
	ModelName              string   `json:"modelName,omitempty"`
	LastActivityAt         string   `json:"lastActivityAt,omitempty"`
	SessionSteps           int      `json:"sessionSteps,omitempty"`
	SessionInputTokens     int64    `json:"sessionInputTokens,omitempty"`
	SessionOutputTokens    int64    `json:"sessionOutputTokens,omitempty"`
	SessionCacheReadTokens int64    `json:"sessionCacheReadTokens,omitempty"`
	SessionThinkingTokens  int64    `json:"sessionThinkingTokens,omitempty"`
	SessionTotalTokens     int64    `json:"sessionTotalTokens,omitempty"`
	SessionCacheHitRate    *float64 `json:"sessionCacheHitRate,omitempty"`
	SessionCostUSD         float64  `json:"sessionCostUsd,omitempty"`
}

// TurnOutlineItem is a lightweight, full-session index entry for a single conversation turn.
// It carries no message bodies, thinking traces, or base64 media so clients can render the complete
// 1..N outline and true turn counts without eagerly downloading full message history on session open.
type TurnOutlineItem struct {
	TurnIndex      int            `json:"turnIndex"`
	FirstMessageID string         `json:"firstMessageId"`
	MessageOffset  int            `json:"messageOffset"`
	MessageCount   int            `json:"messageCount"`
	StartStepIndex *int           `json:"startStepIndex,omitempty"`
	EndStepIndex   *int           `json:"endStepIndex,omitempty"`
	PromptSummary  string         `json:"promptSummary"`
	TotalTools     int            `json:"totalTools,omitempty"`
	Artifacts      []ArtifactItem `json:"artifacts,omitempty"`
	HasError       bool           `json:"hasError,omitempty"`
	ModelName      string         `json:"modelName,omitempty"`
	Timestamp      string         `json:"timestamp,omitempty"`
	IsVoiceTurn    bool           `json:"isVoiceTurn,omitempty"`
}

type TrajectoryParentReference struct {
	TrajectoryID   string `json:"trajectoryId,omitempty"`
	TrajectoryType string `json:"trajectoryType,omitempty"`
	StepIndex      int    `json:"stepIndex"`
	StepType       string `json:"stepType,omitempty"`
	ReferenceType  string `json:"referenceType,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
}

// SideQuestionOrigin identifies the parent conversation and fork step when viewing a /btw side question
// or a forked conversation branch, allowing clients to collapse shared prefix history cleanly.
type SideQuestionOrigin struct {
	ParentCascadeID string `json:"parentCascadeId"`
	ParentTitle     string `json:"parentTitle,omitempty"`
	ForkStepIndex   int    `json:"forkStepIndex"`
	ForkTurnNumber  int    `json:"forkTurnNumber,omitempty"`
	IsSideQuestion  bool   `json:"isSideQuestion,omitempty"`
}

// SideQuestionCardItem represents an inline /btw side-question branch anchored to a parent message.
type SideQuestionCardItem struct {
	CascadeID       string `json:"cascadeId"`
	ParentCascadeID string `json:"parentCascadeId,omitempty"`
	Title           string `json:"title,omitempty"`
	Question        string `json:"question,omitempty"`
	AnswerPreview   string `json:"answerPreview,omitempty"`
	Status          string `json:"status,omitempty"` // "running", "done", "error"
	ForkStepIndex   int    `json:"forkStepIndex"`
	AnchorMessageID string `json:"anchorMessageId,omitempty"`
	TurnCount       int    `json:"turnCount,omitempty"`
	CreatedAt       string `json:"createdAt,omitempty"`
}

type CascadeMessagesResponse struct {
	CascadeID          string                 `json:"cascadeId"`
	Title              string                 `json:"title,omitempty"`
	Status             string                 `json:"status"`
	HasError           bool                   `json:"hasError"`
	ErrorMessage       string                 `json:"errorMessage,omitempty"`
	Duration           string                 `json:"duration"`
	TotalSteps         int                    `json:"totalSteps"`
	TotalTools         int                    `json:"totalTools"`
	TotalMessages      int                    `json:"totalMessages"`
	HasMore            bool                   `json:"hasMore"`
	NextOffset         int                    `json:"nextOffset"`
	Messages           []CascadeMessageItem   `json:"messages"`
	TurnOutline        []TurnOutlineItem      `json:"turnOutline,omitempty"`
	QueuedMessages     []QueuedMessageItem    `json:"queuedMessages"`
	RunningTasks       []RunningTaskItem      `json:"runningTasks,omitempty"`
	Subagents          []SubagentItem         `json:"subagents,omitempty"`
	HasRunningSubagents bool                  `json:"hasRunningSubagents,omitempty"`
	ParentConversation string                 `json:"parentConversationId,omitempty"`
	SubagentRole       string                 `json:"subagentRole,omitempty"`
	BranchOrigin       *SideQuestionOrigin    `json:"branchOrigin,omitempty"`
	SideQuestions      []SideQuestionCardItem `json:"sideQuestions,omitempty"`
	ActiveModel            string             `json:"activeModel,omitempty"`
	ActiveModelName        string             `json:"activeModelName,omitempty"` // full display name, e.g. "Gemini 3.8 Flash (High)"
	ModelDisplayName       string             `json:"modelDisplayName,omitempty"`
	DesktopActiveModel     string             `json:"desktopActiveModel,omitempty"`
	DesktopActiveModelName string             `json:"desktopActiveModelName,omitempty"`
	StartedAt          string                 `json:"startedAt,omitempty"` // when the conversation started (first step)
	CascadeConfig      json.RawMessage        `json:"cascadeConfig,omitempty"`
	CascadeConfigRaw   string                 `json:"cascadeConfigRaw,omitempty"`
	CanProceed         bool                   `json:"canProceed"`
	ProceedArtifactURI string                 `json:"proceedArtifactUri,omitempty"`
	PendingInteraction *PendingInteraction    `json:"pendingInteraction,omitempty"`
	Swarm              *SwarmBrief            `json:"swarm,omitempty"`
	ContextMeter       *ContextMeterItem      `json:"contextMeter,omitempty"`
	SuggestedResponses []string               `json:"suggestedResponses,omitempty"`
	TaskProgress       *TaskProgressItem      `json:"taskProgress,omitempty"`
	ActiveFocus        string                 `json:"activeFocus,omitempty"`
	ActiveFocusStarted string                 `json:"activeFocusStartedAt,omitempty"`
	ActivePlan         *StepPlanItem          `json:"activePlan,omitempty"`
}

type TrajectoryMediaItem struct {
	MimeType    string `json:"mimeType"`
	Description string `json:"description"`
	Thumbnail   string `json:"thumbnail"`
	InlineData  string `json:"inlineData"`
	URI         string `json:"uri"`
}

type TrajectoryUserInput struct {
	UserResponse string `json:"userResponse"`
	Items        []struct {
		Text string `json:"text"`
		Item *struct {
			SlashCommand *struct {
				Info *struct {
					Name           string `json:"name"`
					Type           string `json:"type,omitempty"`
					Icon           string `json:"icon,omitempty"`
					DefinitionPath string `json:"definitionPath,omitempty"`
				} `json:"info"`
			} `json:"slashCommand"`
		} `json:"item,omitempty"`
	} `json:"items"`
	Images []struct {
		Base64Data string `json:"base64Data"`
		MimeType   string `json:"mimeType"`
	} `json:"images"`
	Media            []TrajectoryMediaItem `json:"media"`
	ArtifactComments []struct {
		ArtifactURI    string      `json:"artifactUri"`
		ApprovalStatus interface{} `json:"approvalStatus"`
		Comment        string      `json:"comment,omitempty"`
	} `json:"artifactComments,omitempty"`
	ClientMessageID string   `json:"clientMessageId,omitempty"`
	Tags            []string `json:"tags,omitempty"`
}

// ResolvedClientMessageID returns the client message ID from the native 2.30+ field or fallback mgy-client tag.
func (ui *TrajectoryUserInput) ResolvedClientMessageID() string {
	if ui == nil {
		return ""
	}
	if cid := strings.TrimSpace(ui.ClientMessageID); clientMessageIDRe.MatchString(cid) {
		return cid
	}
	for _, t := range ui.Tags {
		if strings.HasPrefix(t, clientTagPrefix) {
			if id := strings.TrimPrefix(t, clientTagPrefix); clientMessageIDRe.MatchString(id) {
				return id
			}
		}
	}
	return ""
}

var defaultSystemSlashIcons = map[string]string{
	"grill-me":   "forum",
	"plan":       "ballot",
	"goal":       "timer",
	"compact":    "collapse_all",
	"schedule":   "schedule",
	"automation": "schedule",
	"browser":    "language",
	"teamwork":   "graph_2",
	"learn":      "lightbulb_2",
	"boost":      "bolt",
	"btw":        "chat_add_on",
	"swarm":      "hub",
}

// ResolveSlashCommandIcon returns the desktop-aligned icon name for a slash command or skill.
func ResolveSlashCommandIcon(name, icon, kind string) string {
	icon = strings.TrimSpace(icon)
	if icon != "" {
		return icon
	}
	cleanName := strings.TrimPrefix(strings.TrimSpace(name), "/")
	if sysIcon, ok := defaultSystemSlashIcons[cleanName]; ok {
		return sysIcon
	}
	if kind == "skill" {
		return "code_blocks"
	}
	return "code"
}

// ExtractTextAndSlashCommands returns the user input text and any structured slash-command mentions from Items.
func (ui *TrajectoryUserInput) ExtractTextAndSlashCommands() (string, []SlashCommandMention) {
	if ui == nil {
		return "", nil
	}
	var mentions []SlashCommandMention
	seen := make(map[string]bool)
	var sb strings.Builder
	for _, it := range ui.Items {
		if it.Item != nil && it.Item.SlashCommand != nil && it.Item.SlashCommand.Info != nil {
			info := it.Item.SlashCommand.Info
			name := strings.TrimPrefix(strings.TrimSpace(info.Name), "/")
			if name != "" {
				kind := "system"
				if strings.Contains(info.Type, "SKILL") {
					kind = "skill"
				}
				icon := ResolveSlashCommandIcon(name, info.Icon, kind)
				if !seen[name] {
					seen[name] = true
					mentions = append(mentions, SlashCommandMention{
						Name:           name,
						Icon:           icon,
						Kind:           kind,
						DefinitionPath: strings.TrimSpace(info.DefinitionPath),
					})
				}
				sb.WriteString("/")
				sb.WriteString(name)
			}
			continue
		}
		if it.Text != "" {
			sb.WriteString(it.Text)
		}
	}
	text := ui.UserResponse
	if text == "" && sb.Len() > 0 {
		text = sb.String()
	}
	return text, mentions
}

type TrajectoryStep struct {
	Type     string `json:"type"`
	Status   string `json:"status"`
	Metadata struct {
		CreatedAt                string `json:"createdAt"`
		StartedAt                string `json:"startedAt,omitempty"`
		CompletedAt              string `json:"completedAt,omitempty"`
		GeneratorModel           string `json:"generatorModel,omitempty"`
		ToolSummary              string `json:"toolSummary,omitempty"`
		ToolAction               string `json:"toolAction,omitempty"`
		SourceTrajectoryStepInfo *struct {
			StepIndex int `json:"stepIndex"`
		} `json:"sourceTrajectoryStepInfo,omitempty"`
		ToolCall *struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ArgumentsJson string `json:"argumentsJson,omitempty"`
		} `json:"toolCall,omitempty"`
	} `json:"metadata"`
	TaskDetails *struct {
		ID          string `json:"id"`
		LogURI      string `json:"logUri"`
		Description string `json:"description"`
		IsDaemon    bool   `json:"isDaemon,omitempty"`
	} `json:"taskDetails,omitempty"`
	RunCommand *struct {
		CommandLine         string `json:"commandLine"`
		ProposedCommandLine string `json:"proposedCommandLine"`
		Cwd                 string `json:"cwd"`
		WaitMsBeforeAsync   string `json:"waitMsBeforeAsync"`
		IsDaemon            bool   `json:"isDaemon,omitempty"`
	} `json:"runCommand,omitempty"`
	InvokeSubagent *struct {
		Subagents []struct {
			TypeName      string `json:"typeName"`
			Role          string `json:"role"`
			InitialPrompt string `json:"initialPrompt"`
			ModelTier     string `json:"modelTier"`
		} `json:"subagents"`
		Results []struct {
			ConversationID string `json:"conversationId"`
		} `json:"results"`
	} `json:"invokeSubagent,omitempty"`
	UserInput       *TrajectoryUserInput `json:"userInput"`
	PlannerResponse *struct {
		Response         string `json:"response"`
		Thinking         string `json:"thinking"`
		ThinkingDuration string `json:"thinkingDuration,omitempty"`
	} `json:"plannerResponse"`
	SuggestedResponses *struct {
		Suggestions []string `json:"suggestions"`
	} `json:"suggestedResponses,omitempty"`
	TaskBoundary *struct {
		TaskName    string `json:"taskName"`
		TaskStatus  string `json:"taskStatus"`
		TaskSummary string `json:"taskSummary"`
		Mode        string `json:"mode"`
	} `json:"taskBoundary,omitempty"`
	CaptureBrowserScreenshot *struct {
		ScreenshotURI string `json:"screenshotUri,omitempty"`
		ImageBase64   string `json:"imageBase64,omitempty"`
		URL           string `json:"url,omitempty"`
		PageTitle     string `json:"pageTitle,omitempty"`
	} `json:"captureBrowserScreenshot,omitempty"`
	CodeAction *struct {
		IsArtifactFile   bool `json:"isArtifactFile"`
		ArtifactMetadata *struct {
			Summary         string `json:"summary"`
			RequestFeedback bool   `json:"requestFeedback"`
			UserFacing      *bool  `json:"userFacing"`
		} `json:"artifactMetadata"`
		ActionResult *struct {
			Edit *struct {
				AbsoluteURI string `json:"absoluteUri"`
				CreateFile  bool   `json:"createFile"`
			} `json:"edit"`
			AbsoluteURI string `json:"absoluteUri"`
		} `json:"actionResult"`
		ActionSpec *struct {
			CreateFile *struct {
				Path *struct {
					AbsoluteURI string `json:"absoluteUri"`
				} `json:"path"`
			} `json:"createFile"`
		} `json:"actionSpec"`
	} `json:"codeAction"`
	RequestedInteraction *struct {
		Permission *struct {
			Resource struct {
				Action string `json:"action"`
				Target string `json:"target"`
			} `json:"resource"`
			ActionDescription string `json:"actionDescription"`
		} `json:"permission"`
		AskQuestion *struct {
			Questions []struct {
				Question      string `json:"question"`
				IsMultiSelect bool   `json:"isMultiSelect"`
				Options       []struct {
					ID   string `json:"id"`
					Text string `json:"text"`
				} `json:"options"`
			} `json:"questions"`
		} `json:"askQuestion"`
		RunCommand *struct {
			CommandLine string `json:"commandLine"`
		} `json:"runCommand"`
		FilePermission *struct {
			AbsolutePathURI string `json:"absolutePathUri"`
		} `json:"filePermission"`
		ApprovalInteraction *struct {
			Title       string `json:"title,omitempty"`
			Description string `json:"description,omitempty"`
			Confirm     bool   `json:"confirm,omitempty"`
		} `json:"approvalInteraction,omitempty"`
		Mcp *struct {
			Confirm bool `json:"confirm,omitempty"`
		} `json:"mcp,omitempty"`
		Elicitation *struct {
			ServerName          string `json:"serverName,omitempty"`
			Mode                string `json:"mode,omitempty"`
			Message             string `json:"message,omitempty"`
			RequestedSchemaJSON string `json:"requestedSchemaJson,omitempty"`
			URL                 string `json:"url,omitempty"`
		} `json:"elicitation,omitempty"`
	} `json:"requestedInteraction"`
	McpTool *struct {
		ServerName    string `json:"serverName,omitempty"`
		ToolName      string `json:"toolName,omitempty"`
		ArgumentsJson string `json:"argumentsJson,omitempty"`
	} `json:"mcpTool,omitempty"`
	ErrorMessage *struct {
		Error struct {
			UserErrorMessage  string `json:"userErrorMessage"`
			ModelErrorMessage string `json:"modelErrorMessage"`
			ShortError        string `json:"shortError"`
			FullError         string `json:"fullError"`
			ErrorCode         int    `json:"errorCode"`
			ErrorID           string `json:"errorId"`
			IsBenign          bool   `json:"isBenign"`
		} `json:"error"`
		ShouldShowUser  bool `json:"shouldShowUser"`
		ShouldShowModel bool `json:"shouldShowModel"`
	} `json:"errorMessage"`
	Error *struct {
		ShortError string `json:"shortError"`
		FullError  string `json:"fullError"`
	} `json:"error"`
	SystemMessage *struct {
		Content string `json:"content"`
		Message string `json:"message,omitempty"`
	} `json:"systemMessage,omitempty"`
	ToolCall *struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		ToolSummary   string `json:"toolSummary,omitempty"`
		ToolAction    string `json:"toolAction,omitempty"`
		ArgumentsJson string `json:"argumentsJson,omitempty"`
	} `json:"toolCall,omitempty"`
	Generic *struct {
		Args   map[string]json.RawMessage `json:"args,omitempty"`
		Result *struct {
			StepRenderInfo *struct {
				Title       string `json:"title,omitempty"`
				TitlePrefix string `json:"titlePrefix,omitempty"`
				Markdown    string `json:"markdown,omitempty"`
				Hidden      bool   `json:"hidden,omitempty"`
			} `json:"stepRenderInfo,omitempty"`
		} `json:"result,omitempty"`
	} `json:"generic,omitempty"`
	Content string `json:"content,omitempty"`
}

func extractToolNameFromStep(s TrajectoryStep) string {
	if s.Metadata.ToolCall != nil && s.Metadata.ToolCall.Name != "" {
		return s.Metadata.ToolCall.Name
	}
	if s.ToolCall != nil && s.ToolCall.Name != "" {
		return s.ToolCall.Name
	}
	if s.Metadata.ToolAction != "" {
		return s.Metadata.ToolAction
	}
	return strings.ToLower(strings.TrimPrefix(s.Type, "CORTEX_STEP_TYPE_"))
}

// extractStepToolArgsJSON returns the raw JSON tool arguments attached to a step, if any.
func extractStepToolArgsJSON(s TrajectoryStep) string {
	if s.Metadata.ToolCall != nil && s.Metadata.ToolCall.ArgumentsJson != "" {
		return s.Metadata.ToolCall.ArgumentsJson
	}
	if s.ToolCall != nil && s.ToolCall.ArgumentsJson != "" {
		return s.ToolCall.ArgumentsJson
	}
	return ""
}

// IsDaemonStep reports whether a trajectory step was launched with IsDaemon=true (e.g. a dev
// server via run_command or a standing cron job via schedule).
func IsDaemonStep(s TrajectoryStep) bool {
	if s.TaskDetails != nil && s.TaskDetails.IsDaemon {
		return true
	}
	if s.RunCommand != nil && s.RunCommand.IsDaemon {
		return true
	}
	rawArgs := extractStepToolArgsJSON(s)
	if rawArgs == "" || (!strings.Contains(rawArgs, "IsDaemon") && !strings.Contains(rawArgs, "isDaemon")) {
		return false
	}
	var parsed struct {
		IsDaemonUpper *bool `json:"IsDaemon"`
		IsDaemonLower *bool `json:"isDaemon"`
	}
	if err := json.Unmarshal([]byte(rawArgs), &parsed); err == nil {
		if parsed.IsDaemonUpper != nil && *parsed.IsDaemonUpper {
			return true
		}
		if parsed.IsDaemonLower != nil && *parsed.IsDaemonLower {
			return true
		}
	}
	return false
}

type upstreamPendingAgentMessage struct {
	ID               string          `json:"id"`
	DeliveryStrategy interface{}     `json:"deliveryStrategy"`
	Sender           string          `json:"sender"`
	HideFromUser     bool            `json:"hideFromUser"`
	SourceMetadata   json.RawMessage `json:"sourceMetadata"`
	StepPayload      json.RawMessage `json:"stepPayload"`
	Content          string          `json:"content"`
	Timestamp        interface{}     `json:"timestamp"`
}

type upstreamTrajectoryResp struct {
	Trajectory struct {
		TrajectoryID     string                      `json:"trajectoryId"`
		CascadeID        string                      `json:"cascadeId"`
		TrajectoryType   string                      `json:"trajectoryType,omitempty"`
		WorkspaceUris    []string                    `json:"workspaceUris"`
		Steps            []TrajectoryStep            `json:"steps"`
		ParentReferences []TrajectoryParentReference `json:"parentReferences,omitempty"`
		Annotations      *struct {
			Title                  string            `json:"title"`
			LastUserViewTime       string            `json:"lastUserViewTime"`
			SideQuestionCascadeIDs []string          `json:"sideQuestionCascadeIds,omitempty"`
			SideQuestionTitles     map[string]string `json:"sideQuestionTitles,omitempty"`
		} `json:"annotations"`
		Summary  string `json:"summary"`
		Metadata *struct {
			ParentConversationID string `json:"parentConversationId"`
			SubagentSpec         *struct {
				TypeName string `json:"typeName"`
				Role     string `json:"role"`
			} `json:"subagentSpec"`
		} `json:"metadata"`
		ExecutorMetadatas []struct {
			CascadeConfig json.RawMessage `json:"cascadeConfig"`
		} `json:"executorMetadatas"`
	} `json:"trajectory"`
	Status               string                        `json:"status"`
	PendingAgentMessages []upstreamPendingAgentMessage `json:"pendingAgentMessages"`
}

// TrajectoryDetails represents parsed and processed trajectory information.
type TrajectoryDetails struct {
	CascadeID          string                 `json:"cascadeId"`
	Title              string                 `json:"title,omitempty"`
	Status             string                 `json:"status"`
	HasError           bool                   `json:"hasError"`
	ErrorMessage       string                 `json:"errorMessage,omitempty"`
	Duration           string                 `json:"duration"`
	TotalSteps         int                    `json:"totalSteps"`
	TotalTools         int                    `json:"totalTools"`
	WorkspaceURI       string                 `json:"workspaceUri"`
	Steps              []TrajectoryStep       `json:"steps"`
	AllMessages        []CascadeMessageItem   `json:"allMessages"`
	TurnOutline        []TurnOutlineItem      `json:"-"`
	QueuedMessages     []QueuedMessageItem    `json:"queuedMessages"`
	RunningTasks       []RunningTaskItem      `json:"runningTasks,omitempty"`
	Subagents          []SubagentItem         `json:"subagents,omitempty"`
	HasRunningSubagents bool                  `json:"hasRunningSubagents,omitempty"`
	ParentConversation string                 `json:"parentConversationId,omitempty"`
	SubagentRole       string                 `json:"subagentRole,omitempty"`
	BranchOrigin       *SideQuestionOrigin    `json:"branchOrigin,omitempty"`
	SideQuestions      []SideQuestionCardItem `json:"sideQuestions,omitempty"`
	ActiveModel        string                 `json:"activeModel,omitempty"`
	ActiveModelName    string               `json:"activeModelName,omitempty"`
	ModelDisplayName   string               `json:"modelDisplayName,omitempty"`
	StartedAt          string               `json:"startedAt,omitempty"`
	CascadeConfig      json.RawMessage      `json:"cascadeConfig,omitempty"`
	CascadeConfigRaw   string               `json:"cascadeConfigRaw,omitempty"`
	CanProceed         bool                 `json:"canProceed"`
	ProceedArtifactURI string               `json:"proceedArtifactUri,omitempty"`
	PendingInteraction *PendingInteraction  `json:"pendingInteraction,omitempty"`
	ContextMeter       *ContextMeterItem    `json:"contextMeter,omitempty"`
	SuggestedResponses []string             `json:"suggestedResponses,omitempty"`
	TaskProgress       *TaskProgressItem    `json:"taskProgress,omitempty"`
	ActiveFocus        string               `json:"activeFocus,omitempty"`
	ActiveFocusStarted string               `json:"activeFocusStartedAt,omitempty"`
	ActivePlan         *StepPlanItem        `json:"activePlan,omitempty"`
}
