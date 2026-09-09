package proxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

type CascadeMessagesResponse struct {
	CascadeID        string               `json:"cascadeId"`
	Status           string               `json:"status"`
	Duration         string               `json:"duration"`
	TotalSteps       int                  `json:"totalSteps"`
	TotalTools       int                  `json:"totalTools"`
	TotalMessages    int                  `json:"totalMessages"`
	HasMore          bool                 `json:"hasMore"`
	NextOffset       int                  `json:"nextOffset"`
	Messages         []CascadeMessageItem `json:"messages"`
	CascadeConfig    json.RawMessage      `json:"cascadeConfig,omitempty"`
	CascadeConfigRaw string               `json:"cascadeConfigRaw,omitempty"`
}

type trajectoryCacheEntry struct {
	fetchedAt time.Time
	data      *upstreamTrajectoryResp
}

var (
	trajCache              = make(map[string]*trajectoryCacheEntry)
	trajCacheMu            sync.Mutex
	lastKnownCascadeConfig json.RawMessage
	lastKnownConfigMu      sync.RWMutex
	imgRegex               = regexp.MustCompile(`!\[.*?\]\((https?://[^\s\)]+|/static/[^\s\)]+)\)`)
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
}

type upstreamTrajectoryResp struct {
	Trajectory struct {
		TrajectoryID  string           `json:"trajectoryId"`
		CascadeID     string           `json:"cascadeId"`
		WorkspaceUris []string         `json:"workspaceUris"`
		Steps         []TrajectoryStep `json:"steps"`
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
		CascadeID:        cascadeID,
		Status:           details.Status,
		Duration:         details.Duration,
		TotalSteps:       details.TotalSteps,
		TotalTools:       details.TotalTools,
		TotalMessages:    totalMsgs,
		HasMore:          hasMore,
		NextOffset:       nextOffset,
		Messages:         sliced,
		CascadeConfig:    details.CascadeConfig,
		CascadeConfigRaw: details.CascadeConfigRaw,
	})
}

// TrajectoryDetails represents parsed and processed trajectory information.
type TrajectoryDetails struct {
	CascadeID        string               `json:"cascadeId"`
	Status           string               `json:"status"`
	Duration         string               `json:"duration"`
	TotalSteps       int                  `json:"totalSteps"`
	TotalTools       int                  `json:"totalTools"`
	WorkspaceURI     string               `json:"workspaceUri"`
	Steps            []TrajectoryStep     `json:"steps"`
	AllMessages      []CascadeMessageItem `json:"allMessages"`
	CascadeConfig    json.RawMessage      `json:"cascadeConfig,omitempty"`
	CascadeConfigRaw string               `json:"cascadeConfigRaw,omitempty"`
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

	return TrajectoryDetails{
		CascadeID:        rawResp.Trajectory.CascadeID,
		Status:           rawResp.Status,
		Duration:         duration,
		TotalSteps:       totalSteps,
		TotalTools:       totalToolsCount,
		WorkspaceURI:     wsURI,
		Steps:            steps,
		AllMessages:      allMessages,
		CascadeConfig:    activeConfig,
		CascadeConfigRaw: activeConfigStr,
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
}

func (p *Proxy) fetchUpstreamTrajectory(cascadeID string, port int, token string) (*upstreamTrajectoryResp, error) {
	// Status-aware TTL: completed sessions rarely change, so cache them much longer
	// to avoid redundant upstream requests and speed up client rendering.
	maxAge := 800 * time.Millisecond
	trajCacheMu.Lock()
	if cached, ok := trajCache[cascadeID]; ok {
		if cached.data.Status != "" && cached.data.Status != "CASCADE_RUN_STATUS_RUNNING" {
			maxAge = 60 * time.Second
		}
	}
	trajCacheMu.Unlock()
	return p.fetchUpstreamTrajectoryWithMaxAge(cascadeID, port, token, maxAge)
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
