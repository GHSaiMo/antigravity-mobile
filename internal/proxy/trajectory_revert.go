package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"antigravity-mobile/internal/localtls"
)

// RevertDiffLine represents a single line in a unified diff with its change type.
type RevertDiffLine struct {
	Text string `json:"text"`
	Type string `json:"type"` // "INSERT", "DELETE", "UNCHANGED"
}

// RevertPreviewFile represents a file modified by steps being reverted.
type RevertPreviewFile struct {
	FileURI    string           `json:"fileUri"`
	FileName   string           `json:"fileName"`
	ActionType string           `json:"actionType"` // "MODIFY", "CREATE", "DELETE"
	Additions  int              `json:"additions"`
	Deletions  int              `json:"deletions"`
	DiffLines  []RevertDiffLine `json:"diffLines"`
}

// RevertPreviewResponse is the payload returned to mobile clients previewing an undo action.
type RevertPreviewResponse struct {
	CascadeID       string              `json:"cascadeId"`
	StepIndex       int                 `json:"stepIndex"`
	TargetStepIndex int                 `json:"targetStepIndex"`
	Files           []RevertPreviewFile `json:"files"`
	HasCodeChanges  bool                `json:"hasCodeChanges"`
}

// RevertPreviewRequest specifies the target cascade and step to preview reverting.
type RevertPreviewRequest struct {
	CascadeID       string `json:"cascadeId"`
	StepIndex       int    `json:"stepIndex"`
	TargetStepIndex *int   `json:"targetStepIndex,omitempty"`
}

// RevertExecuteRequest specifies the target cascade and step to execute reverting.
type RevertExecuteRequest struct {
	CascadeID        string `json:"cascadeId"`
	StepIndex        int    `json:"stepIndex"`
	TargetStepIndex  *int   `json:"targetStepIndex,omitempty"`
	ConversationOnly bool   `json:"conversationOnly"`
}

// GetRevertPreview queries the upstream language_server for code modifications that would occur upon reverting to step.
func (p *Proxy) GetRevertPreview(cascadeID string, messageStepIndex int, targetIndexOverride *int, port int, token string) (*RevertPreviewResponse, error) {
	if port == 0 {
		return nil, fmt.Errorf("no active Antigravity upstream")
	}

	targetIndex := messageStepIndex - 1
	if messageStepIndex <= 0 {
		targetIndex = -1
	}
	if targetIndexOverride != nil {
		targetIndex = *targetIndexOverride
	}

	apiURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/GetRevertPreview", port)
	reqPayload, _ := json.Marshal(map[string]interface{}{
		"cascadeId": cascadeID,
		"stepIndex": targetIndex,
	})

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(reqPayload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	client := p.mediumClient
	if client == nil {
		client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: localtls.ClientConfig(),
				DialTLSContext:  localtls.DialTLSContext,
			},
			Timeout: 10 * time.Second,
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call upstream GetRevertPreview: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("upstream GetRevertPreview returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var upstreamResp struct {
		CodeEditPreviews []struct {
			FileURI    string `json:"fileUri"`
			ActionType string `json:"actionType"`
			Diff       struct {
				Lines []struct {
					Text string      `json:"text"`
					Type interface{} `json:"type"`
				} `json:"lines"`
			} `json:"diff"`
		} `json:"codeEditPreviews"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&upstreamResp); err != nil {
		return nil, fmt.Errorf("failed to decode upstream GetRevertPreview response: %w", err)
	}

	result := &RevertPreviewResponse{
		CascadeID:       cascadeID,
		StepIndex:       messageStepIndex,
		TargetStepIndex: targetIndex,
		Files:           make([]RevertPreviewFile, 0),
		HasCodeChanges:  false,
	}

	for _, cp := range upstreamResp.CodeEditPreviews {
		action := "MODIFY"
		actStr := strings.ToUpper(cp.ActionType)
		if strings.Contains(actStr, "CREATE") || actStr == "2" {
			action = "CREATE"
		} else if strings.Contains(actStr, "DELETE") || actStr == "3" {
			action = "DELETE"
		}

		fileName := cp.FileURI
		if u, err := url.Parse(cp.FileURI); err == nil && u.Path != "" {
			fileName = filepath.Base(u.Path)
		} else {
			fileName = filepath.Base(cp.FileURI)
		}

		var additions, deletions int
		diffLines := make([]RevertDiffLine, 0, len(cp.Diff.Lines))

		for _, l := range cp.Diff.Lines {
			lineType := "UNCHANGED"
			switch v := l.Type.(type) {
			case string:
				vUpper := strings.ToUpper(v)
				if strings.Contains(vUpper, "INSERT") || v == "1" {
					lineType = "INSERT"
					additions++
				} else if strings.Contains(vUpper, "DELETE") || v == "2" {
					lineType = "DELETE"
					deletions++
				}
			case float64:
				if int(v) == 1 {
					lineType = "INSERT"
					additions++
				} else if int(v) == 2 {
					lineType = "DELETE"
					deletions++
				}
			}
			diffLines = append(diffLines, RevertDiffLine{
				Text: l.Text,
				Type: lineType,
			})
		}

		result.Files = append(result.Files, RevertPreviewFile{
			FileURI:    cp.FileURI,
			FileName:   fileName,
			ActionType: action,
			Additions:  additions,
			Deletions:  deletions,
			DiffLines:  diffLines,
		})
	}

	result.HasCodeChanges = len(result.Files) > 0
	return result, nil
}

// ExecuteRevert sends RevertToCascadeStep to language_server, clearing downstream steps and rolling back changes.
func (p *Proxy) ExecuteRevert(cascadeID string, messageStepIndex int, targetIndexOverride *int, conversationOnly bool, port int, token string) (int, error) {
	if port == 0 {
		return 0, fmt.Errorf("no active Antigravity upstream")
	}

	targetIndex := messageStepIndex - 1
	if messageStepIndex <= 0 {
		targetIndex = -1
	}
	if targetIndexOverride != nil {
		targetIndex = *targetIndexOverride
	}

	client := p.mediumClient
	if client == nil {
		client = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: localtls.ClientConfig(),
				DialTLSContext:  localtls.DialTLSContext,
			},
			Timeout: 10 * time.Second,
		}
	}

	// Stop any active invocation first so reverting a just-sent / RUNNING turn is not rejected upstream.
	p.cancelBeforeRevert(client, cascadeID, port, token)

	apiURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/RevertToCascadeStep", port)

	// Build overrideConfig to supply plannerConfig.planModel and plannerConfig.requestedModel
	// which upstream RevertToCascadeStep strictly requires, without clobbering an existing model
	// when the in-memory cascade model cache is empty (e.g. after gateway restart or desktop-initiated sessions).
	var cfgObj interface{}
	configBytes := p.GetCascadeConfig(cascadeID, port, token)
	if len(configBytes) > 0 {
		_ = json.Unmarshal(configBytes, &cfgObj)
	}
	if lastModel, _ := GetCascadeModel(cascadeID); lastModel != "" {
		if enum := resolveModelEnum(lastModel); enum != "" {
			cfgObj = applyModelToCascadeConfig(cfgObj, enum, canonicalModelName(lastModel))
		}
	}
	if !hasValidPlanModel(cfgObj) {
		defaultModel := "gemini-3.8-flash-high"
		defaultEnum := resolveModelEnum(defaultModel)
		if defaultEnum == "" {
			defaultEnum = "MODEL_PLACEHOLDER_M318"
		}
		cfgObj = applyModelToCascadeConfig(cfgObj, defaultEnum, defaultModel)
	} else {
		cfgObj = ensureRequestedModelFromPlanModel(cfgObj)
	}

	reqPayload, _ := json.Marshal(map[string]interface{}{
		"cascadeId":        cascadeID,
		"stepIndex":        targetIndex,
		"conversationOnly": conversationOnly,
		"overrideConfig":   cfgObj,
	})

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewReader(reqPayload))
	if err != nil {
		return targetIndex, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return targetIndex, fmt.Errorf("failed to call upstream RevertToCascadeStep: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return targetIndex, fmt.Errorf("upstream RevertToCascadeStep returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Revert succeeded: Invalidate all caches and notify stream listeners
	ClearTrajectoryCache(cascadeID)
	ClearPendingMessagesCache(cascadeID)
	p.notifyStreamTouch(cascadeID)

	slog.Info(fmt.Sprintf("[Proxy] Reverted cascade %s to step %d (message step %d, conversationOnly=%v)", shortCascadeID(cascadeID), targetIndex, messageStepIndex, conversationOnly))

	return targetIndex, nil
}

func (p *Proxy) cancelBeforeRevert(client *http.Client, cascadeID string, port int, token string) {
	if cascadeID == "" || port <= 0 || client == nil {
		return
	}
	hasDaemon := false
	defaultTrajCache.trajCacheMu.RLock()
	entry := defaultTrajCache.trajCache[cascadeID]
	defaultTrajCache.trajCacheMu.RUnlock()
	if entry != nil && entry.data != nil {
		for _, t := range buildRunningTasks(entry.data.Trajectory.Steps) {
			if t.IsDaemon {
				hasDaemon = true
				break
			}
		}
	}

	cancelPayload, _ := json.Marshal(map[string]interface{}{
		"cascadeId":           cascadeID,
		"killBackgroundTasks": !hasDaemon,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()

	cancelURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/CancelCascadeInvocation", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cancelURL, bytes.NewReader(cancelPayload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		time.Sleep(25 * time.Millisecond)
	}
}

func hasValidPlanModel(cfgObj interface{}) bool {
	cfgMap, ok := cfgObj.(map[string]interface{})
	if !ok || cfgMap == nil {
		return false
	}
	plannerCfg, ok := cfgMap["plannerConfig"].(map[string]interface{})
	if !ok || plannerCfg == nil {
		return false
	}
	if pm, ok := plannerCfg["planModel"].(string); ok && strings.TrimSpace(pm) != "" {
		return true
	}
	if reqModel, ok := plannerCfg["requestedModel"].(map[string]interface{}); ok && reqModel != nil {
		if m, ok := reqModel["model"].(string); ok && strings.TrimSpace(m) != "" {
			return true
		}
	}
	return false
}

func ensureRequestedModelFromPlanModel(cfgObj interface{}) interface{} {
	cfgMap, ok := cfgObj.(map[string]interface{})
	if !ok || cfgMap == nil {
		return cfgObj
	}
	plannerCfg, ok := cfgMap["plannerConfig"].(map[string]interface{})
	if !ok || plannerCfg == nil {
		return cfgObj
	}
	pm, _ := plannerCfg["planModel"].(string)
	pm = strings.TrimSpace(pm)
	reqModel, _ := plannerCfg["requestedModel"].(map[string]interface{})
	var rm string
	if reqModel != nil {
		rm, _ = reqModel["model"].(string)
		rm = strings.TrimSpace(rm)
	}
	if pm == "" && rm != "" {
		plannerCfg["planModel"] = rm
	} else if pm != "" && rm == "" {
		plannerCfg["requestedModel"] = map[string]interface{}{"model": pm}
	}
	cfgMap["plannerConfig"] = plannerCfg
	return cfgMap
}
