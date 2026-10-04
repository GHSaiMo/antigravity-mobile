package proxy

import (
	"bytes"
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

	apiURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/RevertToCascadeStep", port)

	// Build overrideConfig to supply plannerConfig.planModel and plannerConfig.requestedModel
	// which upstream RevertToCascadeStep strictly requires.
	var cfgObj interface{}
	configBytes := p.GetCascadeConfig(cascadeID, port, token)
	if len(configBytes) > 0 {
		_ = json.Unmarshal(configBytes, &cfgObj)
	}
	modelEnum := "MODEL_PLACEHOLDER_M318"
	modelName := "gemini-2.5-flash"
	if lastModel, _ := GetCascadeModel(cascadeID); lastModel != "" {
		if enum := resolveModelEnum(lastModel); enum != "" {
			modelEnum = enum
			modelName = canonicalModelName(lastModel)
		}
	}
	cfgObj = applyModelToCascadeConfig(cfgObj, modelEnum, modelName)

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
