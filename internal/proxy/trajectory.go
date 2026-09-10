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
	Duration           string               `json:"duration"`
	TotalSteps         int                  `json:"totalSteps"`
	TotalTools         int                  `json:"totalTools"`
	TotalMessages      int                  `json:"totalMessages"`
	HasMore            bool                 `json:"hasMore"`
	NextOffset         int                  `json:"nextOffset"`
	Messages           []CascadeMessageItem `json:"messages"`
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

var (
	trajCache              = make(map[string]*trajectoryCacheEntry)
	trajCacheMu            sync.Mutex
	cascadeTitlesCache     = make(map[string]string)
	cascadeTitlesCacheMu   sync.RWMutex
	lastCascadeTitlesFetch time.Time
	lastKnownCascadeConfig json.RawMessage
	lastKnownConfigMu      sync.RWMutex
	imgRegex               = regexp.MustCompile(`!\[.*?\]\((https?://[^\s\)]+|/static/[^\s\)]+)\)`)
	titleRegex             = regexp.MustCompile(`title:\s*"([^"]+)"`)
	loadedCascadesMu       sync.Mutex
	loadedCascades         = make(map[string]bool)
	hasSyncedHistMu        sync.Mutex
	hasSyncedHist          bool
)

type TrajectoryStep struct {
	Type     string `json:"type"`
	Status   string `json:"status"`
	Metadata struct {
		CreatedAt string `json:"createdAt"`
	} `json:"metadata"`
	UserInput *struct {
		UserResponse string `json:"userResponse"`
		Items        []struct {
			Text string `json:"text"`
		} `json:"items"`
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
	Status string `json:"status"`
}

func (p *Proxy) handleCascadeMessages(w http.ResponseWriter, r *http.Request) {
	cascadeID := r.URL.Query().Get("cascadeId")
	if cascadeID == "" {
		http.Error(w, `{"error":"missing cascadeId"}`, http.StatusBadRequest)
		return
	}

	limit := 10
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
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
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
	json.NewEncoder(w).Encode(CascadeMessagesResponse{
		CascadeID:          cascadeID,
		Title:              details.Title,
		Status:             details.Status,
		Duration:           details.Duration,
		TotalSteps:         details.TotalSteps,
		TotalTools:         details.TotalTools,
		TotalMessages:      totalMsgs,
		HasMore:            hasMore,
		NextOffset:         nextOffset,
		Messages:           sliced,
		CascadeConfig:      details.CascadeConfig,
		CascadeConfigRaw:   details.CascadeConfigRaw,
		CanProceed:         details.CanProceed,
		ProceedArtifactURI: details.ProceedArtifactURI,
		PendingInteraction: details.PendingInteraction,
	})
}

// TrajectoryDetails represents parsed and processed trajectory information.
type TrajectoryDetails struct {
	CascadeID          string               `json:"cascadeId"`
	Title              string               `json:"title,omitempty"`
	Status             string               `json:"status"`
	Duration           string               `json:"duration"`
	TotalSteps         int                  `json:"totalSteps"`
	TotalTools         int                  `json:"totalTools"`
	WorkspaceURI       string               `json:"workspaceUri"`
	Steps              []TrajectoryStep     `json:"steps"`
	AllMessages        []CascadeMessageItem `json:"allMessages"`
	CascadeConfig      json.RawMessage      `json:"cascadeConfig,omitempty"`
	CascadeConfigRaw   string               `json:"cascadeConfigRaw,omitempty"`
	CanProceed         bool                 `json:"canProceed"`
	ProceedArtifactURI string               `json:"proceedArtifactUri,omitempty"`
	PendingInteraction *PendingInteraction  `json:"pendingInteraction,omitempty"`
}

// ParseTrajectoryDetails extracts messages, tools count, duration and metadata from raw response.
func (p *Proxy) ParseTrajectoryDetails(rawResp *upstreamTrajectoryResp) TrajectoryDetails {
	steps := rawResp.Trajectory.Steps
	totalSteps := len(steps)

	var allMessages []CascadeMessageItem
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
			if s.UserInput != nil && len(s.UserInput.Media) > 0 {
				for _, m := range s.UserInput.Media {
					if m.Thumbnail != "" {
						mediaList = append(mediaList, m.Thumbnail)
					} else if m.InlineData != "" {
						mediaList = append(mediaList, m.InlineData)
					}
				}
			}

			if text != "" || len(mediaList) > 0 {
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
			} else {
				// Intermediate planner thought step: count as internal step
				pendingTools++
			}
		} else if strings.HasPrefix(stepType, "CORTEX_STEP_TYPE_") && stepType != "CORTEX_STEP_TYPE_SYSTEM_MESSAGE" {
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
	for i := len(rawResp.Trajectory.ExecutorMetadatas) - 1; i >= 0; i-- {
		cfg := rawResp.Trajectory.ExecutorMetadatas[i].CascadeConfig
		if len(cfg) > 0 && string(cfg) != "null" && string(cfg) != "{}" {
			activeConfig = cfg
			SetLastKnownCascadeConfig(cfg)
			break
		}
	}
	var activeConfigStr string
	if len(activeConfig) > 0 {
		activeConfigStr = string(activeConfig)
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

				isArtifact := ca.IsArtifactFile ||
					strings.Contains(uri, "/brain/") ||
					strings.Contains(uri, ".gemini/antigravity/brain") ||
					strings.Contains(uri, "implementation_plan.md")

				if isArtifact {
					reqFeedback := false
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

					// Also fallback check ~/.gemini/antigravity/brain/<cascadeId>/implementation_plan.md.metadata.json
					if !reqFeedback && rawResp.Trajectory.CascadeID != "" {
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

					if reqFeedback && uri != "" {
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

	return TrajectoryDetails{
		CascadeID:          rawResp.Trajectory.CascadeID,
		Title:              title,
		Status:             rawResp.Status,
		Duration:           duration,
		TotalSteps:         totalSteps,
		TotalTools:         totalToolsCount,
		WorkspaceURI:       wsURI,
		Steps:              steps,
		AllMessages:        allMessages,
		CascadeConfig:      activeConfig,
		CascadeConfigRaw:   activeConfigStr,
		CanProceed:         canProceed,
		ProceedArtifactURI: proceedArtifactURI,
		PendingInteraction: pendingInteraction,
	}
}

// SetLastKnownCascadeConfig caches the latest known valid cascade config.
func SetLastKnownCascadeConfig(cfg json.RawMessage) {
	if len(cfg) == 0 || string(cfg) == "null" || string(cfg) == "{}" {
		return
	}
	lastKnownConfigMu.Lock()
	lastKnownCascadeConfig = cfg
	lastKnownConfigMu.Unlock()
}

// GetCascadeConfig retrieves the cascade config for the given conversation or falls back to last known.
func (p *Proxy) GetCascadeConfig(cascadeID string, port int, token string) json.RawMessage {
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
	lastKnownConfigMu.RLock()
	defer lastKnownConfigMu.RUnlock()
	if len(lastKnownCascadeConfig) > 0 {
		return lastKnownCascadeConfig
	}
	return nil
}

// ClearTrajectoryCache invalidates cached trajectory for an updated cascade.
func ClearTrajectoryCache(cascadeID string) {
	if cascadeID == "" {
		return
	}
	trajCacheMu.Lock()
	delete(trajCache, cascadeID)
	trajCacheMu.Unlock()

	cascadeTitlesCacheMu.Lock()
	delete(cascadeTitlesCache, cascadeID)
	lastCascadeTitlesFetch = time.Time{}
	cascadeTitlesCacheMu.Unlock()
}

func (p *Proxy) fetchUpstreamTrajectory(cascadeID string, port int, token string) (*upstreamTrajectoryResp, error) {
	// Status-aware TTL: completed sessions rarely change, so cache them longer.
	// But if title is missing or session has few/no steps, keep TTL short (1.5s)
	// so newly generated titles/summaries are quickly discovered.
	maxAge := 800 * time.Millisecond
	trajCacheMu.Lock()
	if cached, ok := trajCache[cascadeID]; ok {
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
	trajCacheMu.Unlock()

	resp, err := p.fetchUpstreamTrajectoryWithMaxAge(cascadeID, port, token, maxAge)
	// Fallback: If not found or empty steps, try loading from disk via LoadTrajectory and retry once
	if (err != nil || (resp != nil && len(resp.Trajectory.Steps) == 0)) && port > 0 {
		if loadErr := p.LoadTrajectory(cascadeID, port, token); loadErr == nil {
			trajCacheMu.Lock()
			delete(trajCache, cascadeID)
			trajCacheMu.Unlock()
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

	cascadeTitlesCacheMu.RLock()
	cachedTitle, ok := cascadeTitlesCache[cascadeID]
	cacheFresh := time.Since(lastCascadeTitlesFetch) < 3*time.Second
	cascadeTitlesCacheMu.RUnlock()

	if ok && cachedTitle != "" && cachedTitle != "未命名会话" && cacheFresh {
		return cachedTitle
	}

	if !cacheFresh {
		summaries, err := p.fetchTrajectoriesSummaryWithTitles(port, token)
		if err == nil && len(summaries) > 0 {
			cascadeTitlesCacheMu.Lock()
			lastCascadeTitlesFetch = time.Now()
			for cid, t := range summaries {
				if t != "" {
					cascadeTitlesCache[cid] = t
				}
			}
			newTitle := cascadeTitlesCache[cascadeID]
			cascadeTitlesCacheMu.Unlock()
			if newTitle != "" && newTitle != "未命名会话" {
				return newTitle
			}
		}
	}

	if t := readAnnotationTitle(cascadeID); t != "" {
		cascadeTitlesCacheMu.Lock()
		cascadeTitlesCache[cascadeID] = t
		cascadeTitlesCacheMu.Unlock()
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
	loadedCascadesMu.Lock()
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".db") {
			continue
		}
		cascadeID := strings.TrimSuffix(name, ".db")
		if loadedCascades[cascadeID] {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Empty session schemas are exactly 48KB (49152 bytes) with 0 steps.
		// If older than 15 minutes and <= 49152 bytes, skip loading old empty drafts.
		if info.Size() <= 49152 && time.Since(info.ModTime()) > 15*time.Minute {
			loadedCascades[cascadeID] = true
			continue
		}

		candidates = append(candidates, cascadeID)
	}
	loadedCascadesMu.Unlock()

	if len(candidates) == 0 {
		hasSyncedHistMu.Lock()
		hasSyncedHist = true
		hasSyncedHistMu.Unlock()
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
					loadedCascadesMu.Lock()
					loadedCascades[cid] = true
					loadedCascadesMu.Unlock()
				} else {
					log.Printf("[Proxy] Failed to load historical trajectory %s: %v", cid, err)
				}
			}
		}()
	}
	wg.Wait()

	hasSyncedHistMu.Lock()
	hasSyncedHist = true
	hasSyncedHistMu.Unlock()

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
	trajCacheMu.Lock()
	if cached, ok := trajCache[cascadeID]; ok {
		if time.Since(cached.fetchedAt) < maxAge {
			trajCacheMu.Unlock()
			return cached.data, nil
		}
	}
	trajCacheMu.Unlock()

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

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: p.transport,
	}
	resp, err := client.Do(req)
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

	trajCacheMu.Lock()
	trajCache[cascadeID] = &trajectoryCacheEntry{
		fetchedAt: time.Now(),
		data:      &data,
	}
	trajCacheMu.Unlock()

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
