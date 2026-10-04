package proxy

import (
	"encoding/json"
	"strings"
)

type CascadeMessageItem struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"` // "user", "agent", "tools", "error"
	Role      string   `json:"role"`
	Text      string   `json:"text"`
	Content   string   `json:"content"`
	StepIndex *int     `json:"stepIndex,omitempty"`
	ToolCount int      `json:"toolCount,omitempty"`
	ToolNames []string `json:"toolNames,omitempty"`
	Media     []string `json:"media,omitempty"`     // Base64 thumbnails
	ImageURLs []string `json:"imageUrls,omitempty"` // Markdown image URLs
}

// QueuedMessageItem represents a pending follow-up user message queued for execution.
type QueuedMessageItem struct {
	ID        string   `json:"id"`
	Text      string   `json:"text"`
	CreatedAt string   `json:"createdAt,omitempty"`
	Media     []string `json:"media,omitempty"`     // Base64 thumbnails or image data
	ImageURLs []string `json:"imageUrls,omitempty"` // Image URLs
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

type PendingInteraction struct {
	Type               string                `json:"type"` // "permission", "ask_question", "file_permission", "run_command"
	TrajectoryID       string                `json:"trajectoryId"`
	StepIndex          int                   `json:"stepIndex"`
	Title              string                `json:"title"`
	Target             string                `json:"target,omitempty"`
	Action             string                `json:"action,omitempty"`
	Description        string                `json:"description,omitempty"`
	Options            []InteractionOption   `json:"options"`
	IsMultiSelect      bool                  `json:"isMultiSelect,omitempty"`
	DefaultOptionID    string                `json:"defaultOptionId,omitempty"`
	HasWriteIn         bool                  `json:"hasWriteIn"`
	WriteInLabel       string                `json:"writeInLabel,omitempty"`
	WriteInPlaceholder string                `json:"writeInPlaceholder,omitempty"`
	Questions          []InteractionQuestion `json:"questions,omitempty"`
}

type CascadeMessagesResponse struct {
	CascadeID          string               `json:"cascadeId"`
	Title              string               `json:"title,omitempty"`
	Status             string               `json:"status"`
	HasError           bool                 `json:"hasError"`
	ErrorMessage       string               `json:"errorMessage,omitempty"`
	Duration           string               `json:"duration"`
	TotalSteps         int                  `json:"totalSteps"`
	TotalTools         int                  `json:"totalTools"`
	TotalMessages      int                  `json:"totalMessages"`
	HasMore            bool                 `json:"hasMore"`
	NextOffset         int                  `json:"nextOffset"`
	Messages           []CascadeMessageItem `json:"messages"`
	QueuedMessages     []QueuedMessageItem  `json:"queuedMessages"`
	RunningTasks       []RunningTaskItem    `json:"runningTasks,omitempty"`
	ActiveModel        string               `json:"activeModel,omitempty"`
	ModelDisplayName   string               `json:"modelDisplayName,omitempty"`
	CascadeConfig      json.RawMessage      `json:"cascadeConfig,omitempty"`
	CascadeConfigRaw   string               `json:"cascadeConfigRaw,omitempty"`
	CanProceed         bool                 `json:"canProceed"`
	ProceedArtifactURI string               `json:"proceedArtifactUri,omitempty"`
	PendingInteraction *PendingInteraction  `json:"pendingInteraction,omitempty"`
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
}

type TrajectoryStep struct {
	Type     string `json:"type"`
	Status   string `json:"status"`
	Metadata struct {
		CreatedAt                string `json:"createdAt"`
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
	} `json:"taskDetails,omitempty"`
	RunCommand *struct {
		CommandLine         string `json:"commandLine"`
		ProposedCommandLine string `json:"proposedCommandLine"`
		Cwd                 string `json:"cwd"`
		WaitMsBeforeAsync   string `json:"waitMsBeforeAsync"`
	} `json:"runCommand,omitempty"`
	UserInput       *TrajectoryUserInput `json:"userInput"`
	PlannerResponse *struct {
		Response string `json:"response"`
		Thinking string `json:"thinking"`
	} `json:"plannerResponse"`
	CodeAction *struct {
		IsArtifactFile   bool `json:"isArtifactFile"`
		ArtifactMetadata *struct {
			Summary         string `json:"summary"`
			RequestFeedback bool   `json:"requestFeedback"`
			UserFacing      bool   `json:"userFacing"`
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
	} `json:"requestedInteraction"`
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
	} `json:"systemMessage,omitempty"`
	ToolCall *struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		ToolSummary   string `json:"toolSummary,omitempty"`
		ToolAction    string `json:"toolAction,omitempty"`
		ArgumentsJson string `json:"argumentsJson,omitempty"`
	} `json:"toolCall,omitempty"`
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
		TrajectoryID  string           `json:"trajectoryId"`
		CascadeID     string           `json:"cascadeId"`
		WorkspaceUris []string         `json:"workspaceUris"`
		Steps         []TrajectoryStep `json:"steps"`
		Annotations   *struct {
			Title            string `json:"title"`
			LastUserViewTime string `json:"lastUserViewTime"`
		} `json:"annotations"`
		Summary           string `json:"summary"`
		ExecutorMetadatas []struct {
			CascadeConfig json.RawMessage `json:"cascadeConfig"`
		} `json:"executorMetadatas"`
	} `json:"trajectory"`
	Status               string                        `json:"status"`
	PendingAgentMessages []upstreamPendingAgentMessage `json:"pendingAgentMessages"`
}

// TrajectoryDetails represents parsed and processed trajectory information.
type TrajectoryDetails struct {
	CascadeID          string               `json:"cascadeId"`
	Title              string               `json:"title,omitempty"`
	Status             string               `json:"status"`
	HasError           bool                 `json:"hasError"`
	ErrorMessage       string               `json:"errorMessage,omitempty"`
	Duration           string               `json:"duration"`
	TotalSteps         int                  `json:"totalSteps"`
	TotalTools         int                  `json:"totalTools"`
	WorkspaceURI       string               `json:"workspaceUri"`
	Steps              []TrajectoryStep     `json:"steps"`
	AllMessages        []CascadeMessageItem `json:"allMessages"`
	QueuedMessages     []QueuedMessageItem  `json:"queuedMessages"`
	RunningTasks       []RunningTaskItem    `json:"runningTasks,omitempty"`
	ActiveModel        string               `json:"activeModel,omitempty"`
	ModelDisplayName   string               `json:"modelDisplayName,omitempty"`
	CascadeConfig      json.RawMessage      `json:"cascadeConfig,omitempty"`
	CascadeConfigRaw   string               `json:"cascadeConfigRaw,omitempty"`
	CanProceed         bool                 `json:"canProceed"`
	ProceedArtifactURI string               `json:"proceedArtifactUri,omitempty"`
	PendingInteraction *PendingInteraction  `json:"pendingInteraction,omitempty"`
}
