package proxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CascadeMessageItem struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"` // "user", "agent", "tools"
	Text      string   `json:"text"`
	ToolCount int      `json:"toolCount,omitempty"`
	ToolNames []string `json:"toolNames,omitempty"`
	Media     []string `json:"media,omitempty"`     // Base64 thumbnails
	ImageURLs []string `json:"imageUrls,omitempty"` // Markdown image URLs
}

// QueuedMessageItem represents a pending follow-up user message queued for execution.
type QueuedMessageItem struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	CreatedAt string `json:"createdAt,omitempty"`
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

type PendingInteraction struct {
	Type               string              `json:"type"` // "permission", "ask_question", "file_permission", "run_command"
	TrajectoryID       string              `json:"trajectoryId"`
	StepIndex          int                 `json:"stepIndex"`
	Title              string              `json:"title"`
	Target             string              `json:"target,omitempty"`
	Action             string              `json:"action,omitempty"`
	Description        string              `json:"description,omitempty"`
	Options            []InteractionOption `json:"options"`
	IsMultiSelect      bool                `json:"isMultiSelect,omitempty"`
	DefaultOptionID    string              `json:"defaultOptionId,omitempty"`
	HasWriteIn         bool                `json:"hasWriteIn"`
	WriteInLabel       string              `json:"writeInLabel,omitempty"`
	WriteInPlaceholder string              `json:"writeInPlaceholder,omitempty"`
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
	QueuedMessages     []QueuedMessageItem  `json:"queuedMessages,omitempty"`
	RunningTasks       []RunningTaskItem    `json:"runningTasks,omitempty"`
	ActiveModel        string               `json:"activeModel,omitempty"`
	ModelDisplayName   string               `json:"modelDisplayName,omitempty"`
	CascadeConfig      json.RawMessage      `json:"cascadeConfig,omitempty"`
	CascadeConfigRaw   string               `json:"cascadeConfigRaw,omitempty"`
	CanProceed         bool                 `json:"canProceed"`
	ProceedArtifactURI string               `json:"proceedArtifactUri,omitempty"`
	PendingInteraction *PendingInteraction  `json:"pendingInteraction,omitempty"`
}

type trajectoryCacheEntry struct {
	fetchedAt time.Time
	data      *upstreamTrajectoryResp
}

const maxTrajCacheSize = 50 // Maximum number of cached trajectory entries to prevent unbounded memory growth

// TrajectoryCache encapsulates all mutable trajectory-related state with proper synchronization.
// It replaces scattered package-level vars, enabling clean lifecycle management and testability.
type TrajectoryCache struct {
	trajCache   map[string]*trajectoryCacheEntry
	trajCacheMu sync.Mutex

	cascadeTitles       map[string]string
	cascadeTitlesMu     sync.RWMutex
	lastTitlesFetchTime time.Time

	lastKnownConfig   json.RawMessage
	lastKnownConfigMu sync.RWMutex

	cascadeConfigs   map[string]json.RawMessage
	cascadeConfigsMu sync.RWMutex

	cascadeModels   map[string]string
	cascadeModelsMu sync.RWMutex

	loadedCascades   map[string]bool
	loadedCascadesMu sync.Mutex
	lastSyncedPort   int

	deletedCascades   map[string]time.Time
	deletedCascadesMu sync.RWMutex
}

// NewTrajectoryCache creates a new TrajectoryCache with initialized maps.
func NewTrajectoryCache() *TrajectoryCache {
	return &TrajectoryCache{
		trajCache:       make(map[string]*trajectoryCacheEntry),
		cascadeTitles:   make(map[string]string),
		cascadeConfigs:  make(map[string]json.RawMessage),
		cascadeModels:   make(map[string]string),
		loadedCascades:  make(map[string]bool),
		deletedCascades: make(map[string]time.Time),
	}
}

var (
	imgRegex   = regexp.MustCompile(`!\[.*?\]\((https?://[^\s\)]+|/static/[^\s\)]+)\)`)
	titleRegex = regexp.MustCompile(`title:\s*"([^"]+)"`)
)

// RecordDeletedCascade marks a cascade as recently deleted with a TTL.
func RecordDeletedCascade(cascadeID string) {
	if cascadeID == "" {
		return
	}
	defaultTrajCache.deletedCascadesMu.Lock()
	defer defaultTrajCache.deletedCascadesMu.Unlock()
	defaultTrajCache.deletedCascades[cascadeID] = time.Now()
}

// RemoveDeletedCascadeTombstone removes a tombstone if deletion failed upstream.
func RemoveDeletedCascadeTombstone(cascadeID string) {
	if cascadeID == "" {
		return
	}
	defaultTrajCache.deletedCascadesMu.Lock()
	defer defaultTrajCache.deletedCascadesMu.Unlock()
	delete(defaultTrajCache.deletedCascades, cascadeID)
}

// IsDeletedCascade returns true if the cascade was recently deleted within tombstone TTL (60s).
func IsDeletedCascade(cascadeID string) bool {
	if cascadeID == "" {
		return false
	}
	defaultTrajCache.deletedCascadesMu.RLock()
	deletedAt, exists := defaultTrajCache.deletedCascades[cascadeID]
	defaultTrajCache.deletedCascadesMu.RUnlock()
	if !exists {
		return false
	}
	if time.Since(deletedAt) < 60*time.Second {
		return true
	}
	// Expired tombstone, clean it up
	defaultTrajCache.deletedCascadesMu.Lock()
	delete(defaultTrajCache.deletedCascades, cascadeID)
	defaultTrajCache.deletedCascadesMu.Unlock()
	return false
}

// ResetHistoricalSyncState clears the loaded cascades map and resets the last synced port,
// allowing a fresh sync of all historical sessions from disk.
func ResetHistoricalSyncState() {
	defaultTrajCache.loadedCascadesMu.Lock()
	defer defaultTrajCache.loadedCascadesMu.Unlock()
	defaultTrajCache.loadedCascades = make(map[string]bool)
	defaultTrajCache.lastSyncedPort = 0
}

// HasSyncedHistoricalTrajectories returns whether historical trajectories have already been synced for this port.
func HasSyncedHistoricalTrajectories(port int) bool {
	defaultTrajCache.loadedCascadesMu.Lock()
	defer defaultTrajCache.loadedCascadesMu.Unlock()
	return port > 0 && port == defaultTrajCache.lastSyncedPort
}

// evictTrajCacheLocked removes the oldest entries when cache exceeds maxTrajCacheSize.
// MUST be called while holding tc.trajCacheMu.
// Uses sort-based batch removal: O(n log n) instead of O(n²).
func (tc *TrajectoryCache) evictTrajCacheLocked() {
	if len(tc.trajCache) <= maxTrajCacheSize {
		return
	}
	type keyTime struct {
		key string
		t   time.Time
	}
	items := make([]keyTime, 0, len(tc.trajCache))
	for k, v := range tc.trajCache {
		items = append(items, keyTime{key: k, t: v.fetchedAt})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].t.Before(items[j].t)
	})
	// Remove oldest entries until we're at the limit
	removeCount := len(tc.trajCache) - maxTrajCacheSize
	for i := 0; i < removeCount; i++ {
		delete(tc.trajCache, items[i].key)
	}
}

// defaultTrajCache is the package-level TrajectoryCache instance used by Proxy.
// It is initialized here and assigned as a field of Proxy in NewProxy.
var defaultTrajCache = NewTrajectoryCache()

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
			ID   string `json:"id"`
			Name string `json:"name"`
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
	UserInput *struct {
		UserResponse string `json:"userResponse"`
		Items        []struct {
			Text string `json:"text"`
		} `json:"items"`
		Images []struct {
			Base64Data string `json:"base64Data"`
			MimeType   string `json:"mimeType"`
		} `json:"images"`
		Media []struct {
			MimeType    string `json:"mimeType"`
			Description string `json:"description"`
			Thumbnail   string `json:"thumbnail"`
			InlineData  string `json:"inlineData"`
		} `json:"media"`
	} `json:"userInput"`
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
}

type upstreamPendingAgentMessage struct {
	ID               string          `json:"id"`
	DeliveryStrategy int             `json:"deliveryStrategy"`
	StepPayload      json.RawMessage `json:"stepPayload"`
	Content          string          `json:"content"`
	Timestamp        string          `json:"timestamp"`
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

func (p *Proxy) handleCascadeMessages(w http.ResponseWriter, r *http.Request) {
	cascadeID := r.URL.Query().Get("cascadeId")
	if cascadeID == "" {
		http.Error(w, `{"error":"missing cascadeId"}`, http.StatusBadRequest)
		return
	}

	limit := 15
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
			if limit > 50 {
				limit = 50
			}
		}
	}

	offset := -1
	if o := r.URL.Query().Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	log.Printf("[Proxy] CascadeMessages: cascadeId=%s limit=%d offset=%d", cascadeID, limit, offset)

	p.mu.RLock()
	port := p.activePort
	token := p.activeToken
	p.mu.RUnlock()

	if port == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":"Antigravity upstream not connected"}`))
		return
	}

	// Fetch or use cached trajectory
	rawResp, err := p.fetchUpstreamTrajectory(cascadeID, port, token)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		if encErr := json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}); encErr != nil {
			log.Printf("[Proxy] HandleGetCascadeMessages: failed to encode error response: %v", encErr)
		}
		return
	}

	details := p.ParseTrajectoryDetails(rawResp)
	if details.Title == "" || details.Title == "未命名会话" {
		if t := p.lookupCascadeTitle(cascadeID, port, token); t != "" {
			details.Title = t
		}
	}
	totalMsgs := len(details.AllMessages)
	var sliced []CascadeMessageItem
	hasMore := false
	nextOffset := 0

	if offset < 0 {
		// Initial fetch: return latest 'limit' messages
		start := totalMsgs - limit
		if start < 0 {
			start = 0
		}
		sliced = details.AllMessages[start:totalMsgs]
		hasMore = (start > 0)
		nextOffset = start
	} else {
		// Paging back: return 'limit' messages before 'offset'
		targetEnd := offset
		if targetEnd > totalMsgs {
			targetEnd = totalMsgs
		}
		targetStart := targetEnd - limit
		if targetStart < 0 {
			targetStart = 0
		}
		sliced = details.AllMessages[targetStart:targetEnd]
		hasMore = (targetStart > 0)
		nextOffset = targetStart
	}

	w.Header().Set("Content-Type", "application/json")
	if encErr := json.NewEncoder(w).Encode(CascadeMessagesResponse{
		CascadeID:          cascadeID,
		Title:              details.Title,
		Status:             details.Status,
		HasError:           details.HasError,
		ErrorMessage:       details.ErrorMessage,
		Duration:           details.Duration,
		TotalSteps:         details.TotalSteps,
		TotalTools:         details.TotalTools,
		TotalMessages:      totalMsgs,
		HasMore:            hasMore,
		NextOffset:         nextOffset,
		Messages:           sliced,
		QueuedMessages:     details.QueuedMessages,
		RunningTasks:       details.RunningTasks,
		ActiveModel:        details.ActiveModel,
		ModelDisplayName:   details.ModelDisplayName,
		CascadeConfig:      details.CascadeConfig,
		CascadeConfigRaw:   details.CascadeConfigRaw,
		CanProceed:         details.CanProceed,
		ProceedArtifactURI: details.ProceedArtifactURI,
		PendingInteraction: details.PendingInteraction,
	}); encErr != nil {
		log.Printf("[Proxy] HandleGetCascadeMessages: failed to encode response: %v", encErr)
	}
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
	QueuedMessages     []QueuedMessageItem  `json:"queuedMessages,omitempty"`
	RunningTasks       []RunningTaskItem    `json:"runningTasks,omitempty"`
	ActiveModel        string               `json:"activeModel,omitempty"`
	ModelDisplayName   string               `json:"modelDisplayName,omitempty"`
	CascadeConfig      json.RawMessage      `json:"cascadeConfig,omitempty"`
	CascadeConfigRaw   string               `json:"cascadeConfigRaw,omitempty"`
	CanProceed         bool                 `json:"canProceed"`
	ProceedArtifactURI string               `json:"proceedArtifactUri,omitempty"`
	PendingInteraction *PendingInteraction  `json:"pendingInteraction,omitempty"`
}

// extractErrorText retrieves the most descriptive error message from a trajectory step.
func extractErrorText(s TrajectoryStep) string {
	if s.ErrorMessage != nil {
		e := s.ErrorMessage.Error
		userMsg := strings.TrimSpace(e.UserErrorMessage)
		shortErr := strings.TrimSpace(e.ShortError)
		modelErr := strings.TrimSpace(e.ModelErrorMessage)
		fullErr := strings.TrimSpace(e.FullError)

		if userMsg != "" && shortErr != "" && userMsg != shortErr && !strings.Contains(shortErr, userMsg) {
			return fmt.Sprintf("%s\n%s", userMsg, shortErr)
		}
		if shortErr != "" {
			return shortErr
		}
		if userMsg != "" {
			return userMsg
		}
		if modelErr != "" {
			return modelErr
		}
		if fullErr != "" {
			return fullErr
		}
	}
	if s.Error != nil {
		if s.Error.ShortError != "" {
			return s.Error.ShortError
		}
		if s.Error.FullError != "" {
			return s.Error.FullError
		}
	}
	return "Agent execution terminated due to error."
}

// isUserVisibleError determines whether an error step should be shown to the user.
// In the official Antigravity IDE, only errors with shouldShowUser == true are rendered in normal mode.
// Internal stream interruption retries and continuation errors (shouldShowModel == true) are hidden.
func isUserVisibleError(s TrajectoryStep) bool {
	if s.ErrorMessage != nil {
		if s.ErrorMessage.ShouldShowUser {
			return true
		}
		if s.ErrorMessage.ShouldShowModel {
			return false
		}
		// If neither flag was set, check for known transient retry errors
		short := strings.ToLower(s.ErrorMessage.Error.ShortError)
		userMsg := strings.ToLower(s.ErrorMessage.Error.UserErrorMessage)
		if strings.Contains(short, "stream was interrupted") || strings.Contains(userMsg, "stream was interrupted") ||
			strings.Contains(short, "model produced invalid output") || strings.Contains(userMsg, "model produced invalid output") {
			return false
		}
		return true
	}
	if s.Error != nil {
		short := strings.ToLower(s.Error.ShortError)
		if strings.Contains(short, "stream was interrupted") || strings.Contains(short, "model produced invalid output") {
			return false
		}
		return true
	}
	return false
}

// ParseTrajectoryDetails extracts messages, tools count, duration and metadata from raw response.
func (p *Proxy) ParseTrajectoryDetails(rawResp *upstreamTrajectoryResp) TrajectoryDetails {
	steps := rawResp.Trajectory.Steps
	totalSteps := len(steps)

	var allMessages []CascadeMessageItem
	var lastErrorText string
	pendingTools := 0
	toolNamesMap := make(map[string]bool)
	var toolNames []string
	totalToolsCount := 0

	flushTools := func() {
		if pendingTools == 0 {
			return
		}
		item := CascadeMessageItem{
			ID:        fmt.Sprintf("tools-%d", len(allMessages)),
			Type:      "tools",
			Text:      fmt.Sprintf("已思考并执行 %d 项操作", pendingTools),
			ToolCount: pendingTools,
			ToolNames: toolNames,
		}
		allMessages = append(allMessages, item)
		pendingTools = 0
		toolNames = nil
		toolNamesMap = make(map[string]bool)
	}

	for idx, s := range steps {
		stepType := s.Type

		if stepType == "CORTEX_STEP_TYPE_USER_INPUT" {
			flushTools()
			text := ""
			if s.UserInput != nil {
				text = s.UserInput.UserResponse
				if text == "" && len(s.UserInput.Items) > 0 {
					text = s.UserInput.Items[0].Text
				}
			}

			var mediaList []string
			if s.UserInput != nil {
				if len(s.UserInput.Media) > 0 {
					for _, m := range s.UserInput.Media {
						if m.Thumbnail != "" {
							mediaList = append(mediaList, m.Thumbnail)
						} else if m.InlineData != "" {
							mediaList = append(mediaList, m.InlineData)
						}
					}
				}
				if len(s.UserInput.Images) > 0 {
					for _, img := range s.UserInput.Images {
						if img.Base64Data != "" {
							mediaList = append(mediaList, img.Base64Data)
						}
					}
				}
			}

			trimmed := strings.TrimSpace(text)
			isSystemApproval := strings.HasPrefix(trimmed, "Comments on artifact URI:") || strings.Contains(trimmed, "The user has approved this document")

			if (trimmed != "" && !isSystemApproval) || len(mediaList) > 0 {
				allMessages = append(allMessages, CascadeMessageItem{
					ID:    fmt.Sprintf("step-%d", idx),
					Type:  "user",
					Text:  text,
					Media: mediaList,
				})
			}
		} else if stepType == "CORTEX_STEP_TYPE_PLANNER_RESPONSE" {
			respText := ""
			if s.PlannerResponse != nil {
				respText = strings.TrimSpace(s.PlannerResponse.Response)
			}

			if respText != "" {
				flushTools()

				// Extract image URLs
				var imgURLs []string
				matches := imgRegex.FindAllStringSubmatch(respText, -1)
				for _, m := range matches {
					if len(m) > 1 {
						imgURLs = append(imgURLs, m[1])
					}
				}

				allMessages = append(allMessages, CascadeMessageItem{
					ID:        fmt.Sprintf("step-%d", idx),
					Type:      "agent",
					Text:      respText,
					ImageURLs: imgURLs,
				})
			}
		} else if stepType == "CORTEX_STEP_TYPE_ERROR_MESSAGE" {
			if !isUserVisibleError(s) {
				continue
			}
			flushTools()
			lastErrorText = extractErrorText(s)
			allMessages = append(allMessages, CascadeMessageItem{
				ID:   fmt.Sprintf("step-%d", idx),
				Type: "error",
				Text: lastErrorText,
			})
		} else if strings.HasPrefix(stepType, "CORTEX_STEP_TYPE_") && stepType != "CORTEX_STEP_TYPE_SYSTEM_MESSAGE" && stepType != "CORTEX_STEP_TYPE_ERROR_MESSAGE" {
			pendingTools++
			totalToolsCount++
			name := strings.ToLower(strings.TrimPrefix(stepType, "CORTEX_STEP_TYPE_"))
			if !toolNamesMap[name] && len(toolNames) < 3 {
				toolNamesMap[name] = true
				toolNames = append(toolNames, name)
			}
		}
	}

	flushTools()

	// Calculate total duration
	duration := "0秒"
	if len(steps) > 0 {
		var firstTime, lastTime time.Time
		for _, s := range steps {
			if s.Metadata.CreatedAt != "" {
				if t, err := parseTime(s.Metadata.CreatedAt); err == nil {
					firstTime = t
					break
				}
			}
		}
		for i := len(steps) - 1; i >= 0; i-- {
			if steps[i].Metadata.CreatedAt != "" {
				if t, err := parseTime(steps[i].Metadata.CreatedAt); err == nil {
					lastTime = t
					break
				}
			}
		}
		if !firstTime.IsZero() && !lastTime.IsZero() {
			diff := int(lastTime.Sub(firstTime).Seconds())
			duration = formatDuration(diff)
		}
	}

	var activeConfig json.RawMessage
	var activeModel string
	if cid := rawResp.Trajectory.CascadeID; cid != "" {
		if recModel, recCfg := GetCascadeModel(cid); recModel != "" {
			activeModel = canonicalModelName(recModel)
			if len(recCfg) > 0 {
				activeConfig = recCfg
			}
		}
	}
	for i := len(rawResp.Trajectory.ExecutorMetadatas) - 1; i >= 0; i-- {
		cfg := rawResp.Trajectory.ExecutorMetadatas[i].CascadeConfig
		if len(cfg) > 0 && string(cfg) != "null" && string(cfg) != "{}" {
			if activeModel == "" {
				var cfgMap map[string]interface{}
				if err := json.Unmarshal(cfg, &cfgMap); err == nil {
					if p, ok := cfgMap["plannerConfig"].(map[string]interface{}); ok {
						if mn, ok := p["modelName"].(string); ok && mn != "" {
							activeModel = canonicalModelName(mn)
						} else if pm, ok := p["planModel"].(string); ok && pm != "" {
							activeModel = canonicalModelName(pm)
						}
					}
				}
			}
			if len(activeConfig) == 0 {
				activeConfig = cfg
				SetLastKnownCascadeConfig(cfg)
			}
			if activeModel != "" && len(activeConfig) > 0 {
				break
			}
		}
	}
	var activeConfigStr string
	if len(activeConfig) > 0 {
		activeConfigStr = string(activeConfig)
	}

	modelDisplayName := ""
	if activeModel != "" {
		lower := strings.ToLower(activeModel)
		if strings.Contains(lower, "claude") {
			modelDisplayName = "Claude"
		} else if strings.Contains(lower, "gemini") {
			modelDisplayName = "Gemini"
		} else if strings.Contains(lower, "gpt") {
			modelDisplayName = "GPT"
		} else {
			modelDisplayName = activeModel
		}
	}

	wsURI := ""
	if len(rawResp.Trajectory.WorkspaceUris) > 0 {
		wsURI = rawResp.Trajectory.WorkspaceUris[0]
	}

	title := ""
	if rawResp.Trajectory.Annotations != nil && rawResp.Trajectory.Annotations.Title != "" {
		title = rawResp.Trajectory.Annotations.Title
	} else if rawResp.Trajectory.Summary != "" {
		title = rawResp.Trajectory.Summary
	}

	// Detect if latest turn contains an artifact pending user feedback (Proceed)
	// Desktop Antigravity (Bjb) parity:
	// 1. Must be in the latest turn (after last CORTEX_STEP_TYPE_USER_INPUT).
	// 2. Status must not be RUNNING.
	// 3. No non-artifact code files have been modified in this turn.
	// 4. An artifact in this turn has requestFeedback == true.
	canProceed := false
	proceedArtifactURI := ""
	hasModifiedNonArtifactFiles := false

	lastUserInputIdx := -1
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].Type == "CORTEX_STEP_TYPE_USER_INPUT" {
			lastUserInputIdx = i
			break
		}
	}

	for i := lastUserInputIdx + 1; i < len(steps); i++ {
		s := steps[i]
		if s.Type == "CORTEX_STEP_TYPE_CODE_ACTION" {
			if s.CodeAction != nil {
				ca := s.CodeAction
				uri := ""
				if ca.ActionResult != nil {
					if ca.ActionResult.Edit != nil && ca.ActionResult.Edit.AbsoluteURI != "" {
						uri = ca.ActionResult.Edit.AbsoluteURI
					} else if ca.ActionResult.AbsoluteURI != "" {
						uri = ca.ActionResult.AbsoluteURI
					}
				}
				if uri == "" && ca.ActionSpec != nil && ca.ActionSpec.CreateFile != nil && ca.ActionSpec.CreateFile.Path != nil {
					uri = ca.ActionSpec.CreateFile.Path.AbsoluteURI
				}

				isScratch := strings.Contains(uri, "/scratch/")
				isWalkthrough := strings.Contains(uri, "walkthrough.md")
				isPlan := strings.Contains(uri, "implementation_plan.md")

				isArtifact := (ca.IsArtifactFile ||
					strings.Contains(uri, "/brain/") ||
					strings.Contains(uri, ".gemini/antigravity/brain") ||
					isPlan) && !isScratch

				if isArtifact {
					reqFeedback := false
					if !isWalkthrough {
						if ca.ArtifactMetadata != nil && ca.ArtifactMetadata.RequestFeedback {
							reqFeedback = true
						}

						filePath := uri
						if strings.HasPrefix(filePath, "file://") {
							filePath = strings.TrimPrefix(filePath, "file://")
						}

						// If step metadata was missing but this is an artifact step in the current turn, check its specific metadata file
						if !reqFeedback && filePath != "" {
							if metaData, err := os.ReadFile(filePath + ".metadata.json"); err == nil {
								var meta struct {
									RequestFeedback bool `json:"requestFeedback"`
								}
								if err := json.Unmarshal(metaData, &meta); err == nil && meta.RequestFeedback {
									reqFeedback = true
								}
							}
						}

						// Fallback: ONLY check implementation_plan.md.metadata.json if this step actually targets implementation_plan.md
						if !reqFeedback && isPlan && rawResp.Trajectory.CascadeID != "" {
							if home, err := os.UserHomeDir(); err == nil && home != "" {
								planMetaPath := filepath.Join(home, ".gemini/antigravity/brain", rawResp.Trajectory.CascadeID, "implementation_plan.md.metadata.json")
								if metaData, err := os.ReadFile(planMetaPath); err == nil {
									var meta struct {
										RequestFeedback bool `json:"requestFeedback"`
									}
									if err := json.Unmarshal(metaData, &meta); err == nil && meta.RequestFeedback {
										reqFeedback = true
										if uri == "" {
											uri = "file://" + filepath.Join(home, ".gemini/antigravity/brain", rawResp.Trajectory.CascadeID, "implementation_plan.md")
										}
									}
								}
							}
						}
					}

					if reqFeedback && uri != "" && !isWalkthrough {
						canProceed = true
						proceedArtifactURI = uri
					}
				} else {
					hasModifiedNonArtifactFiles = true
				}
			}
		}
	}

	// Desktop parity: if non-artifact files were modified in this turn, or if agent is actively running, do not show proceed
	if hasModifiedNonArtifactFiles || rawResp.Status == "CASCADE_RUN_STATUS_RUNNING" {
		canProceed = false
		proceedArtifactURI = ""
	}

	var pendingInteraction *PendingInteraction
	for idx := len(steps) - 1; idx >= 0; idx-- {
		s := steps[idx]
		if s.Status == "CORTEX_STEP_STATUS_WAITING" && s.RequestedInteraction != nil {
			pi := &PendingInteraction{
				TrajectoryID: rawResp.Trajectory.TrajectoryID,
				StepIndex:    idx,
			}
			req := s.RequestedInteraction
			if req.Permission != nil {
				pi.Type = "permission"
				pi.Action = req.Permission.Resource.Action
				pi.Target = req.Permission.Resource.Target
				pi.Description = req.Permission.ActionDescription

				actionLower := strings.ToLower(pi.Action)
				if strings.Contains(actionLower, "read") {
					pi.Title = "Allow read access to this path?"
				} else if strings.Contains(actionLower, "command") {
					pi.Title = "Allow executing this command?"
				} else if strings.Contains(actionLower, "write") || strings.Contains(actionLower, "edit") {
					pi.Title = "Allow write access to this path?"
				} else if pi.Description != "" {
					pi.Title = "Allow: " + pi.Description + "?"
				} else {
					pi.Title = fmt.Sprintf("Allow %s access?", pi.Action)
				}

				pi.Options = []InteractionOption{
					{ID: "1", Text: "Yes, allow this time", Scope: 1},
					{ID: "2", Text: "Yes, and always allow in this conversation", Scope: 2},
					{ID: "3", Text: "Yes, and always allow in this project", Scope: 3},
					{ID: "4", Text: "Yes, and always allow", Scope: 4},
					{ID: "5", Text: "No", IsDeny: true},
				}
				pi.DefaultOptionID = "1"
				pi.HasWriteIn = true
				pi.WriteInLabel = "No"
				pi.WriteInPlaceholder = "(tell the agent what to do instead)"
			} else if req.AskQuestion != nil && len(req.AskQuestion.Questions) > 0 {
				pi.Type = "ask_question"
				q := req.AskQuestion.Questions[0]
				pi.Title = q.Question
				pi.IsMultiSelect = q.IsMultiSelect
				for _, opt := range q.Options {
					pi.Options = append(pi.Options, InteractionOption{
						ID:   opt.ID,
						Text: opt.Text,
					})
				}
				if len(pi.Options) > 0 {
					pi.DefaultOptionID = pi.Options[0].ID
				}
				pi.HasWriteIn = true
				pi.WriteInLabel = "Other"
				pi.WriteInPlaceholder = "(write in your response)"
			} else if req.FilePermission != nil {
				pi.Type = "file_permission"
				pi.Target = req.FilePermission.AbsolutePathURI
				pi.Title = "Allow access to this file outside workspace?"
				pi.Options = []InteractionOption{
					{ID: "1", Text: "Yes, allow this time", Scope: 1},
					{ID: "2", Text: "Yes, and always allow in this conversation", Scope: 2},
					{ID: "3", Text: "Yes, and always allow in this project", Scope: 3},
					{ID: "4", Text: "Yes, and always allow", Scope: 4},
					{ID: "5", Text: "No", IsDeny: true},
				}
				pi.DefaultOptionID = "1"
				pi.HasWriteIn = true
				pi.WriteInLabel = "No"
				pi.WriteInPlaceholder = "(tell the agent what to do instead)"
			} else if req.RunCommand != nil {
				pi.Type = "run_command"
				pi.Target = req.RunCommand.CommandLine
				pi.Title = "Confirm command execution"
				pi.Options = []InteractionOption{
					{ID: "1", Text: "Yes, run command", Scope: 1},
					{ID: "2", Text: "No", IsDeny: true},
				}
				pi.DefaultOptionID = "1"
				pi.HasWriteIn = true
				pi.WriteInLabel = "No"
				pi.WriteInPlaceholder = "(tell the agent what to do instead)"
			}
			pendingInteraction = pi
			break
		}
	}
	if rawResp.Status != "CASCADE_RUN_STATUS_RUNNING" {
		pendingInteraction = nil
	}

	var queuedMessages []QueuedMessageItem
	for _, pam := range rawResp.PendingAgentMessages {
		text := pam.Content
		if text == "" && len(pam.StepPayload) > 0 {
			var parsedPayload struct {
				Step struct {
					Case  string `json:"case"`
					Value struct {
						Items []struct {
							Text  string `json:"text"`
							Chunk *struct {
								Case  string `json:"case"`
								Value string `json:"value"`
							} `json:"chunk"`
						} `json:"items"`
					} `json:"value"`
				} `json:"step"`
			}
			if err := json.Unmarshal(pam.StepPayload, &parsedPayload); err == nil {
				for _, it := range parsedPayload.Step.Value.Items {
					if it.Text != "" {
						text += it.Text
					} else if it.Chunk != nil && it.Chunk.Value != "" {
						text += it.Chunk.Value
					}
				}
			}
		}
		if text != "" {
			queuedMessages = append(queuedMessages, QueuedMessageItem{
				ID:        pam.ID,
				Text:      text,
				CreatedAt: pam.Timestamp,
			})
		}
	}

	var runningTasks []RunningTaskItem
	for idx, s := range steps {
		stepIdx := idx
		if s.Metadata.SourceTrajectoryStepInfo != nil && s.Metadata.SourceTrajectoryStepInfo.StepIndex > 0 {
			stepIdx = s.Metadata.SourceTrajectoryStepInfo.StepIndex
		}

		if s.Status == "CORTEX_STEP_STATUS_RUNNING" {
			cmdLine := ""
			toolName := "run_command"
			taskID := fmt.Sprintf("task-%d", stepIdx)
			logURI := ""

			if s.TaskDetails != nil {
				if s.TaskDetails.ID != "" {
					taskID = s.TaskDetails.ID
				}
				if s.TaskDetails.Description != "" {
					cmdLine = s.TaskDetails.Description
				}
				if s.TaskDetails.LogURI != "" {
					logURI = s.TaskDetails.LogURI
				}
			}
			if cmdLine == "" && s.RunCommand != nil {
				if s.RunCommand.CommandLine != "" {
					cmdLine = s.RunCommand.CommandLine
				} else if s.RunCommand.ProposedCommandLine != "" {
					cmdLine = s.RunCommand.ProposedCommandLine
				}
			}
			if s.Metadata.ToolCall != nil && s.Metadata.ToolCall.Name != "" {
				toolName = s.Metadata.ToolCall.Name
			}

			if cmdLine != "" || s.TaskDetails != nil {
				runningTasks = append(runningTasks, RunningTaskItem{
					ID:          taskID,
					StepIndex:   stepIdx,
					ToolName:    toolName,
					CommandLine: cmdLine,
					ToolSummary: s.Metadata.ToolSummary,
					ToolAction:  s.Metadata.ToolAction,
					LogURI:      logURI,
					StartedAt:   s.Metadata.CreatedAt,
				})
			}
		}
	}

	// Determine error state scoped strictly to the latest turn (after lastUserInputIdx).
	// Historical errors in previous turns that were subsequently recovered must not mark the session as an error.
	latestTurnHasError := false
	var latestTurnErrorText string
	for i := lastUserInputIdx + 1; i < len(steps); i++ {
		s := steps[i]
		if s.Type == "CORTEX_STEP_TYPE_ERROR_MESSAGE" {
			if isUserVisibleError(s) {
				latestTurnHasError = true
				latestTurnErrorText = extractErrorText(s)
			}
		} else if s.Type == "CORTEX_STEP_TYPE_PLANNER_RESPONSE" {
			if s.PlannerResponse != nil && strings.TrimSpace(s.PlannerResponse.Response) != "" {
				// If planner succeeded with response in this turn, error was resolved
				latestTurnHasError = false
				latestTurnErrorText = ""
			}
		}
	}

	finalStatus := rawResp.Status
	if (rawResp.Status != "CASCADE_RUN_STATUS_RUNNING" || (len(steps) > 0 && steps[len(steps)-1].Type == "CORTEX_STEP_TYPE_ERROR_MESSAGE")) && latestTurnHasError {
		finalStatus = "CASCADE_RUN_STATUS_ERROR"
	}

	return TrajectoryDetails{
		CascadeID:          rawResp.Trajectory.CascadeID,
		Title:              title,
		Status:             finalStatus,
		HasError:           latestTurnHasError,
		ErrorMessage:       latestTurnErrorText,
		Duration:           duration,
		TotalSteps:         totalSteps,
		TotalTools:         totalToolsCount,
		WorkspaceURI:       wsURI,
		Steps:              steps,
		AllMessages:        allMessages,
		QueuedMessages:     queuedMessages,
		RunningTasks:       runningTasks,
		ActiveModel:        activeModel,
		ModelDisplayName:   modelDisplayName,
		CascadeConfig:      activeConfig,
		CascadeConfigRaw:   activeConfigStr,
		CanProceed:         canProceed,
		ProceedArtifactURI: proceedArtifactURI,
		PendingInteraction: pendingInteraction,
	}
}

// CancelCascadeStep invokes Antigravity LanguageServer's CancelCascadeSteps RPC to terminate a specific background step.
func (p *Proxy) CancelCascadeStep(cascadeID string, stepIndex int, port int, token string) error {
	if port == 0 {
		return fmt.Errorf("no active Antigravity upstream")
	}

	bodyData, err := json.Marshal(map[string]interface{}{
		"cascadeId":   cascadeID,
		"stepIndices": []int{stepIndex},
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/CancelCascadeSteps", port)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.mediumClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("CancelCascadeSteps returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// SetLastKnownCascadeConfig caches the latest known valid cascade config.
func SetLastKnownCascadeConfig(cfg json.RawMessage) {
	if len(cfg) == 0 || string(cfg) == "null" || string(cfg) == "{}" {
		return
	}
	defaultTrajCache.lastKnownConfigMu.Lock()
	defaultTrajCache.lastKnownConfig = cfg
	defaultTrajCache.lastKnownConfigMu.Unlock()
}

// SetCascadeModel records the active model name and cascadeConfig explicitly chosen/applied for a cascade.
func SetCascadeModel(cascadeID, modelName string, cfg json.RawMessage) {
	if cascadeID == "" {
		return
	}
	if modelName != "" {
		defaultTrajCache.cascadeModelsMu.Lock()
		if defaultTrajCache.cascadeModels == nil {
			defaultTrajCache.cascadeModels = make(map[string]string)
		}
		defaultTrajCache.cascadeModels[cascadeID] = modelName
		defaultTrajCache.cascadeModelsMu.Unlock()
	}
	if len(cfg) > 0 && string(cfg) != "null" && string(cfg) != "{}" {
		defaultTrajCache.cascadeConfigsMu.Lock()
		if defaultTrajCache.cascadeConfigs == nil {
			defaultTrajCache.cascadeConfigs = make(map[string]json.RawMessage)
		}
		defaultTrajCache.cascadeConfigs[cascadeID] = cfg
		defaultTrajCache.cascadeConfigsMu.Unlock()
		SetLastKnownCascadeConfig(cfg)
	}
}

// GetCascadeModel retrieves any explicitly recorded model name and config for a cascade.
func GetCascadeModel(cascadeID string) (string, json.RawMessage) {
	if cascadeID == "" {
		return "", nil
	}
	defaultTrajCache.cascadeModelsMu.RLock()
	model := ""
	if defaultTrajCache.cascadeModels != nil {
		model = defaultTrajCache.cascadeModels[cascadeID]
	}
	defaultTrajCache.cascadeModelsMu.RUnlock()

	defaultTrajCache.cascadeConfigsMu.RLock()
	var cfg json.RawMessage
	if defaultTrajCache.cascadeConfigs != nil {
		cfg = defaultTrajCache.cascadeConfigs[cascadeID]
	}
	defaultTrajCache.cascadeConfigsMu.RUnlock()

	return model, cfg
}

// GetCascadeConfig retrieves the cascade config for the given conversation or falls back to last known.
func (p *Proxy) GetCascadeConfig(cascadeID string, port int, token string) json.RawMessage {
	if cascadeID != "" {
		if _, recordedCfg := GetCascadeModel(cascadeID); len(recordedCfg) > 0 {
			return recordedCfg
		}
	}
	if cascadeID != "" && port > 0 {
		if rawResp, err := p.fetchUpstreamTrajectory(cascadeID, port, token); err == nil && rawResp != nil {
			metas := rawResp.Trajectory.ExecutorMetadatas
			for i := len(metas) - 1; i >= 0; i-- {
				cfg := metas[i].CascadeConfig
				if len(cfg) > 0 && string(cfg) != "null" && string(cfg) != "{}" {
					SetLastKnownCascadeConfig(cfg)
					return cfg
				}
			}
		}
	}
	defaultTrajCache.lastKnownConfigMu.RLock()
	defer defaultTrajCache.lastKnownConfigMu.RUnlock()
	if len(defaultTrajCache.lastKnownConfig) > 0 {
		return defaultTrajCache.lastKnownConfig
	}
	return nil
}

// ClearTrajectoryCache invalidates cached trajectory for an updated cascade.
func ClearTrajectoryCache(cascadeID string) {
	if cascadeID == "" {
		return
	}
	defaultTrajCache.trajCacheMu.Lock()
	delete(defaultTrajCache.trajCache, cascadeID)
	defaultTrajCache.trajCacheMu.Unlock()

	defaultTrajCache.cascadeTitlesMu.Lock()
	delete(defaultTrajCache.cascadeTitles, cascadeID)
	defaultTrajCache.lastTitlesFetchTime = time.Time{}
	defaultTrajCache.cascadeTitlesMu.Unlock()

	defaultTrajCache.loadedCascadesMu.Lock()
	delete(defaultTrajCache.loadedCascades, cascadeID)
	defaultTrajCache.loadedCascadesMu.Unlock()
}

func (p *Proxy) fetchUpstreamTrajectory(cascadeID string, port int, token string) (*upstreamTrajectoryResp, error) {
	// Status-aware TTL: completed sessions rarely change, so cache them longer.
	// But if title is missing or session has few/no steps, keep TTL short (1.5s)
	// so newly generated titles/summaries are quickly discovered.
	maxAge := 800 * time.Millisecond
	defaultTrajCache.trajCacheMu.Lock()
	if cached, ok := defaultTrajCache.trajCache[cascadeID]; ok {
		if cached.data.Status != "" && cached.data.Status != "CASCADE_RUN_STATUS_RUNNING" {
			hasTitle := (cached.data.Trajectory.Annotations != nil && cached.data.Trajectory.Annotations.Title != "") ||
				cached.data.Trajectory.Summary != ""
			if hasTitle && len(cached.data.Trajectory.Steps) > 0 {
				maxAge = 30 * time.Second
			} else {
				maxAge = 1500 * time.Millisecond
			}
		}
	}
	defaultTrajCache.trajCacheMu.Unlock()

	resp, err := p.fetchUpstreamTrajectoryWithMaxAge(cascadeID, port, token, maxAge)
	// Fallback: If not found or empty steps, try loading from disk via LoadTrajectory and retry once
	if (err != nil || (resp != nil && len(resp.Trajectory.Steps) == 0)) && port > 0 {
		if loadErr := p.LoadTrajectory(cascadeID, port, token); loadErr == nil {
			defaultTrajCache.trajCacheMu.Lock()
			delete(defaultTrajCache.trajCache, cascadeID)
			defaultTrajCache.trajCacheMu.Unlock()
			if retryResp, retryErr := p.fetchUpstreamTrajectoryWithMaxAge(cascadeID, port, token, 0); retryErr == nil && retryResp != nil {
				return retryResp, nil
			}
		}
	}
	return resp, err
}

func readAnnotationTitle(cascadeID string) string {
	if cascadeID == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	p := filepath.Join(home, ".gemini", "antigravity", "annotations", cascadeID+".pbtxt")
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	m := titleRegex.FindSubmatch(b)
	if len(m) > 1 {
		return string(m[1])
	}
	return ""
}

func (p *Proxy) lookupCascadeTitle(cascadeID string, port int, token string) string {
	if cascadeID == "" || port == 0 {
		return ""
	}

	defaultTrajCache.cascadeTitlesMu.RLock()
	cachedTitle, ok := defaultTrajCache.cascadeTitles[cascadeID]
	cacheFresh := time.Since(defaultTrajCache.lastTitlesFetchTime) < 3*time.Second
	defaultTrajCache.cascadeTitlesMu.RUnlock()

	if ok && cachedTitle != "" && cachedTitle != "未命名会话" && cacheFresh {
		return cachedTitle
	}

	if !cacheFresh {
		summaries, err := p.fetchTrajectoriesSummaryWithTitles(port, token)
		if err == nil && len(summaries) > 0 {
			defaultTrajCache.cascadeTitlesMu.Lock()
			defaultTrajCache.lastTitlesFetchTime = time.Now()
			for cid, t := range summaries {
				if t != "" {
					defaultTrajCache.cascadeTitles[cid] = t
				}
			}
			newTitle := defaultTrajCache.cascadeTitles[cascadeID]
			defaultTrajCache.cascadeTitlesMu.Unlock()
			if newTitle != "" && newTitle != "未命名会话" {
				return newTitle
			}
		}
	}

	if t := readAnnotationTitle(cascadeID); t != "" {
		defaultTrajCache.cascadeTitlesMu.Lock()
		defaultTrajCache.cascadeTitles[cascadeID] = t
		defaultTrajCache.cascadeTitlesMu.Unlock()
		return t
	}

	return cachedTitle
}

// LoadTrajectory asks upstream language_server to load a historical cascade into memory.
func (p *Proxy) LoadTrajectory(cascadeID string, port int, token string) error {
	if cascadeID == "" || port == 0 {
		return fmt.Errorf("invalid cascadeId or port")
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/LoadTrajectory", port)
	payload, _ := json.Marshal(map[string]string{"cascadeId": cascadeID})
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	client := &http.Client{
		Timeout:   3 * time.Second,
		Transport: p.transport,
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upstream LoadTrajectory returned %d: %s", resp.StatusCode, string(b))
	}

	return nil
}

// SyncHistoricalTrajectories scans ~/.gemini/antigravity/conversations for historical session DBs
// and loads valid sessions into upstream language_server memory so GetAllCascadeTrajectories returns them.
func (p *Proxy) SyncHistoricalTrajectories(port int, token string) error {
	if port == 0 {
		return fmt.Errorf("upstream port not set")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	convDir := filepath.Join(home, ".gemini", "antigravity", "conversations")
	entries, err := os.ReadDir(convDir)
	if err != nil {
		return err
	}

	var candidates []string
	defaultTrajCache.loadedCascadesMu.Lock()
	if port != defaultTrajCache.lastSyncedPort {
		defaultTrajCache.loadedCascades = make(map[string]bool)
		defaultTrajCache.lastSyncedPort = port
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".db") {
			continue
		}
		cascadeID := strings.TrimSuffix(name, ".db")
		if defaultTrajCache.loadedCascades[cascadeID] {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.Size() == 0 {
			continue
		}

		// Empty session schemas are exactly 48KB (49152 bytes) with 0 steps.
		// If older than 15 minutes and <= 49152 bytes, skip loading old empty drafts.
		if info.Size() <= 49152 && time.Since(info.ModTime()) > 15*time.Minute {
			defaultTrajCache.loadedCascades[cascadeID] = true
			continue
		}

		candidates = append(candidates, cascadeID)
	}
	defaultTrajCache.loadedCascadesMu.Unlock()

	if len(candidates) == 0 {
		return nil
	}

	log.Printf("[Proxy] Syncing %d historical trajectories into upstream language_server...", len(candidates))

	concurrency := 8
	if concurrency > len(candidates) {
		concurrency = len(candidates)
	}

	workCh := make(chan string, len(candidates))
	for _, cid := range candidates {
		workCh <- cid
	}
	close(workCh)

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for cid := range workCh {
				err := p.LoadTrajectory(cid, port, token)
				if err == nil {
					defaultTrajCache.loadedCascadesMu.Lock()
					defaultTrajCache.loadedCascades[cid] = true
					defaultTrajCache.loadedCascadesMu.Unlock()
				} else {
					log.Printf("[Proxy] Failed to load historical trajectory %s: %v", cid, err)
				}
			}
		}()
	}
	wg.Wait()

	log.Printf("[Proxy] Finished syncing historical trajectories")
	return nil
}

func (p *Proxy) fetchTrajectoriesSummaryWithTitles(port int, token string) (map[string]string, error) {
	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", port)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	client := &http.Client{
		Timeout:   3 * time.Second,
		Transport: p.transport,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	var data struct {
		TrajectorySummaries map[string]struct {
			Summary     string `json:"summary"`
			Annotations *struct {
				Title string `json:"title"`
			} `json:"annotations"`
		} `json:"trajectorySummaries"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for id, sum := range data.TrajectorySummaries {
		t := ""
		if sum.Annotations != nil && sum.Annotations.Title != "" {
			t = sum.Annotations.Title
		} else if sum.Summary != "" {
			t = sum.Summary
		}
		if t != "" {
			result[id] = t
		}
	}
	return result, nil
}

func (p *Proxy) fetchUpstreamTrajectoryWithMaxAge(cascadeID string, port int, token string, maxAge time.Duration) (*upstreamTrajectoryResp, error) {
	defaultTrajCache.trajCacheMu.Lock()
	if cached, ok := defaultTrajCache.trajCache[cascadeID]; ok {
		if time.Since(cached.fetchedAt) < maxAge {
			defaultTrajCache.trajCacheMu.Unlock()
			return cached.data, nil
		}
	}
	defaultTrajCache.trajCacheMu.Unlock()

	bodyBytes, _ := json.Marshal(map[string]string{"cascadeId": cascadeID})
	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetCascadeTrajectory", port)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.mediumClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upstream returned %d: %s", resp.StatusCode, string(b))
	}

	var reader io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip reader failed: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	var data upstreamTrajectoryResp
	if err := json.NewDecoder(reader).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode upstream response: %w", err)
	}

	defaultTrajCache.trajCacheMu.Lock()
	defaultTrajCache.trajCache[cascadeID] = &trajectoryCacheEntry{
		fetchedAt: time.Now(),
		data:      &data,
	}
	defaultTrajCache.evictTrajCacheLocked()
	defaultTrajCache.trajCacheMu.Unlock()

	return &data, nil
}

func parseTime(str string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, str); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, str)
}

func formatDuration(sec int) string {
	if sec <= 0 {
		return "0秒"
	}
	hours := sec / 3600
	minutes := (sec % 3600) / 60
	seconds := sec % 60
	if hours > 0 {
		return fmt.Sprintf("%d小时%d分", hours, minutes)
	} else if minutes > 0 {
		return fmt.Sprintf("%d分%d秒", minutes, seconds)
	}
	return fmt.Sprintf("%d秒", seconds)
}

// FetchTrajectoryDetails fetches and parses full trajectory details for a cascade.
func (p *Proxy) FetchTrajectoryDetails(cascadeID string, maxAge time.Duration) (*TrajectoryDetails, error) {
	port, token := p.ActiveUpstream()
	if port == 0 {
		return nil, fmt.Errorf("antigravity upstream not connected")
	}
	rawResp, err := p.fetchUpstreamTrajectoryWithMaxAge(cascadeID, port, token, maxAge)
	if err != nil {
		return nil, err
	}
	details := p.ParseTrajectoryDetails(rawResp)
	if details.Title == "" || details.Title == "未命名会话" {
		if t := p.lookupCascadeTitle(cascadeID, port, token); t != "" {
			details.Title = t
		}
	}
	return &details, nil
}

// FetchRawCascadeSummaries queries upstream GetAllCascadeTrajectories and returns non-subagent summaries.
func (p *Proxy) FetchRawCascadeSummaries() (map[string]map[string]interface{}, error) {
	port, token := p.ActiveUpstream()
	if port == 0 {
		return nil, fmt.Errorf("antigravity upstream not connected")
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories", port)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	client := &http.Client{
		Timeout:   4 * time.Second,
		Transport: p.transport,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream status %d", resp.StatusCode)
	}

	var rawMap map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&rawMap); err != nil {
		return nil, err
	}

	summariesRaw, ok := rawMap["trajectorySummaries"]
	if !ok {
		return make(map[string]map[string]interface{}), nil
	}

	var summaries map[string]map[string]interface{}
	if err := json.Unmarshal(summariesRaw, &summaries); err != nil {
		return nil, err
	}

	// Filter out internal subagent sessions
	for id, s := range summaries {
		if isSubagentTrajectoryMap(s, id) {
			delete(summaries, id)
		}
	}

	return summaries, nil
}
