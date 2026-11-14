package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/config"
)

// WorkflowTemplate represents one reusable workflow/prompt template shown in the Workflow panel.
type WorkflowTemplate struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Subtitle       string `json:"subtitle,omitempty"`
	Icon           string `json:"icon,omitempty"`
	Color          string `json:"color,omitempty"`
	TemplatePrompt string `json:"templatePrompt"`
}

// WorkflowTemplatesStore is the persisted JSON structure in workflow_templates.json.
type WorkflowTemplatesStore struct {
	Version   int                `json:"version"`
	UpdatedAt string             `json:"updatedAt,omitempty"`
	Templates []WorkflowTemplate `json:"templates"`
}

// WorkflowTemplatesResponse is returned by /gateway/workflow-templates.
type WorkflowTemplatesResponse struct {
	Templates []WorkflowTemplate `json:"templates"`
	UpdatedAt string             `json:"updatedAt,omitempty"`
}

var workflowTemplatesMu sync.RWMutex

// DefaultWorkflowTemplates returns a fresh slice of the built-in workflow presets.
func DefaultWorkflowTemplates() []WorkflowTemplate {
	return []WorkflowTemplate{
		{
			ID:             "code_review",
			Title:          "并发代码审查",
			Subtitle:       "并行启动安全、性能、类型规范多角度 Sub-Agents 进行深度审计",
			Icon:           "security",
			Color:          "indigo",
			TemplatePrompt: "/workflow 请使用 run_workflow 动态编排工作流，以 parallel 并发对 [模块/文件路径] 进行多维度审计（安全/性能/类型规范），并在最终阶段输出结构化修复建议。",
		},
		{
			ID:             "test_and_fix",
			Title:          "并发单测与故障修复",
			Subtitle:       "并行运行各模块单元测试，定位失败用例并尝试由子代理生成补丁",
			Icon:           "bug",
			Color:          "green",
			TemplatePrompt: "/workflow 请使用 run_workflow 编排自动化单测，以 parallel 同时运行 [测试目标或路径] 的单元测试，聚合失败信息并生成修复方案。",
		},
		{
			ID:             "research",
			Title:          "多维度综合调研",
			Subtitle:       "分派多个子代理从技术实现、竞品方案、生产实践分头研究并归纳",
			Icon:           "search",
			Color:          "blue",
			TemplatePrompt: "/workflow 请使用 run_workflow 编排调研工作流，分派子代理分别调查 [调研主题] 的核心技术方案、竞品对比与最佳实践，并汇总为结构化报告。",
		},
		{
			ID:             "custom",
			Title:          "自定义动态工作流",
			Subtitle:       "自由描述任务，让 Agent 依据依赖自主规划 parallel / pipeline 阶段",
			Icon:           "workflow",
			Color:          "purple",
			TemplatePrompt: "/workflow 请使用 run_workflow 编写 Python 动态工作流脚本，分阶段执行以下任务：[描述具体任务]，要求使用 parallel 并发加速并给出 finish 契约结果。",
		},
	}
}

func getWorkflowTemplatesFilePath() string {
	return filepath.Join(config.GetDataDir(), "workflow_templates.json")
}

func loadWorkflowTemplates() ([]WorkflowTemplate, string, error) {
	workflowTemplatesMu.RLock()
	defer workflowTemplatesMu.RUnlock()

	filePath := getWorkflowTemplatesFilePath()
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultWorkflowTemplates(), "", nil
		}
		return nil, "", fmt.Errorf("failed to read workflow templates: %w", err)
	}

	var store WorkflowTemplatesStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, "", fmt.Errorf("failed to parse workflow templates: %w", err)
	}
	if store.Templates == nil {
		store.Templates = []WorkflowTemplate{}
	}
	return store.Templates, store.UpdatedAt, nil
}

func normalizeAndValidateWorkflowTemplates(raw []WorkflowTemplate) ([]WorkflowTemplate, error) {
	out := make([]WorkflowTemplate, 0, len(raw))
	seen := make(map[string]bool, len(raw))
	nowMs := time.Now().UnixMilli()

	for i, item := range raw {
		title := strings.TrimSpace(item.Title)
		prompt := strings.TrimSpace(item.TemplatePrompt)
		if title == "" {
			return nil, fmt.Errorf("template at index %d has empty title", i)
		}
		if prompt == "" {
			return nil, fmt.Errorf("template %q has empty templatePrompt", title)
		}

		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = fmt.Sprintf("wf_%d_%d", nowMs, i)
		}
		baseID := id
		suffix := 1
		for seen[id] {
			suffix++
			id = fmt.Sprintf("%s_%d", baseID, suffix)
		}
		seen[id] = true

		icon := strings.TrimSpace(item.Icon)
		if icon == "" {
			icon = "workflow"
		}
		color := strings.TrimSpace(item.Color)
		if color == "" {
			color = "indigo"
		}

		out = append(out, WorkflowTemplate{
			ID:             id,
			Title:          title,
			Subtitle:       strings.TrimSpace(item.Subtitle),
			Icon:           icon,
			Color:          color,
			TemplatePrompt: prompt,
		})
	}
	return out, nil
}

func saveWorkflowTemplates(raw []WorkflowTemplate) (WorkflowTemplatesResponse, error) {
	normalized, err := normalizeAndValidateWorkflowTemplates(raw)
	if err != nil {
		return WorkflowTemplatesResponse{}, err
	}

	workflowTemplatesMu.Lock()
	defer workflowTemplatesMu.Unlock()

	filePath := getWorkflowTemplatesFilePath()
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return WorkflowTemplatesResponse{}, fmt.Errorf("failed to create data directory: %w", err)
	}

	updatedAt := time.Now().UTC().Format(time.RFC3339)
	store := WorkflowTemplatesStore{
		Version:   1,
		UpdatedAt: updatedAt,
		Templates: normalized,
	}

	buf, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return WorkflowTemplatesResponse{}, fmt.Errorf("failed to encode workflow templates: %w", err)
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", filePath, time.Now().UnixNano())
	if err := os.WriteFile(tmpFile, buf, 0644); err != nil {
		return WorkflowTemplatesResponse{}, fmt.Errorf("failed to write temp workflow templates file: %w", err)
	}

	if err := os.Rename(tmpFile, filePath); err != nil {
		_ = os.Remove(tmpFile)
		return WorkflowTemplatesResponse{}, fmt.Errorf("failed to replace workflow templates file: %w", err)
	}

	return WorkflowTemplatesResponse{
		Templates: normalized,
		UpdatedAt: updatedAt,
	}, nil
}

func resetWorkflowTemplates() (WorkflowTemplatesResponse, error) {
	workflowTemplatesMu.Lock()
	defer workflowTemplatesMu.Unlock()

	filePath := getWorkflowTemplatesFilePath()
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return WorkflowTemplatesResponse{}, fmt.Errorf("failed to remove workflow templates file: %w", err)
	}

	return WorkflowTemplatesResponse{
		Templates: DefaultWorkflowTemplates(),
	}, nil
}

func writeWorkflowTemplatesJSON(w http.ResponseWriter, status int, v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		writeJSONError(w, "failed to encode response: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

// HandleWorkflowTemplates handles GET, PUT/POST, and DELETE /gateway/workflow-templates.
func (p *Proxy) HandleWorkflowTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		templates, updatedAt, err := loadWorkflowTemplates()
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeWorkflowTemplatesJSON(w, http.StatusOK, WorkflowTemplatesResponse{
			Templates: templates,
			UpdatedAt: updatedAt,
		})

	case http.MethodPut, http.MethodPost:
		var req struct {
			Templates []WorkflowTemplate `json:"templates"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if req.Templates == nil {
			writeJSONError(w, "templates array is required", http.StatusBadRequest)
			return
		}
		resp, err := saveWorkflowTemplates(req.Templates)
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeWorkflowTemplatesJSON(w, http.StatusOK, resp)

	case http.MethodDelete:
		resp, err := resetWorkflowTemplates()
		if err != nil {
			writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeWorkflowTemplatesJSON(w, http.StatusOK, resp)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
