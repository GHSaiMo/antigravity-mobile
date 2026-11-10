package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultTaskLogTailBytes = 16 * 1024
	maxTaskLogTailBytes     = 64 * 1024
)

// GlobalBackgroundTaskItem represents a running background task across any conversation,
// returned by LanguageServerService/ListBackgroundTasks and enriched with conversation metadata.
type GlobalBackgroundTaskItem struct {
	ConversationID    string `json:"conversationId"`
	ConversationTitle string `json:"conversationTitle,omitempty"`
	WorkspaceName     string `json:"workspaceName,omitempty"`
	TaskID            string `json:"taskId"`
	StepIndex         int    `json:"stepIndex"`
	ToolName          string `json:"toolName,omitempty"`
	ToolSummary       string `json:"toolSummary,omitempty"`
	CommandLine       string `json:"commandLine,omitempty"`
	StartTime         string `json:"startTime,omitempty"`
	LogURI            string `json:"logUri,omitempty"`
	IsDaemon          bool   `json:"isDaemon,omitempty"`
}

type upstreamListBackgroundTasksResp struct {
	Tasks []struct {
		ConversationID string `json:"conversationId"`
		Conversation2  string `json:"conversation_id"`
		Task           struct {
			TaskID      string `json:"taskId"`
			TaskID2     string `json:"task_id"`
			ToolName    string `json:"toolName"`
			ToolName2   string `json:"tool_name"`
			ToolSummary string `json:"toolSummary"`
			ToolSum2    string `json:"tool_summary"`
			Description string `json:"description"`
			StartTime   string `json:"startTime"`
			StartTime2  string `json:"start_time"`
			StepIndex   int    `json:"stepIndex"`
			StepIndex2  int    `json:"step_index"`
			LogURI      string `json:"logUri"`
			LogURI2     string `json:"log_uri"`
			IsDaemon    bool   `json:"isDaemon"`
			IsDaemon2   bool   `json:"is_daemon"`
		} `json:"task"`
	} `json:"tasks"`
}

// HandleListBackgroundTasks handles GET/POST /gateway/tasks/background.
func (p *Proxy) HandleListBackgroundTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	port, token := p.ActiveUpstream()
	if port == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tasks":[]}`))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()

	tasks, err := p.FetchGlobalBackgroundTasks(ctx, port, token)
	if err != nil {
		// Graceful degradation on older Antigravity versions without ListBackgroundTasks
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"tasks":[]}`))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"tasks": tasks,
	})
}

// FetchGlobalBackgroundTasks calls LanguageServerService/ListBackgroundTasks and enriches
// each task with its conversation title and workspace display name.
func (p *Proxy) FetchGlobalBackgroundTasks(ctx context.Context, port int, token string) ([]GlobalBackgroundTaskItem, error) {
	rawTasks, err := p.callListBackgroundTasksUpstream(ctx, port, token)
	if err != nil {
		return nil, err
	}
	if len(rawTasks) == 0 {
		return []GlobalBackgroundTaskItem{}, nil
	}

	metaMap := p.lookupConversationMetaForTasks(ctx, port, token)
	for i := range rawTasks {
		if m, ok := metaMap[rawTasks[i].ConversationID]; ok {
			rawTasks[i].ConversationTitle = m.title
			rawTasks[i].WorkspaceName = m.workspace
		}
		if rawTasks[i].ConversationTitle == "" {
			rawTasks[i].ConversationTitle = "未命名会话"
		}
	}

	sort.SliceStable(rawTasks, func(i, j int) bool {
		if rawTasks[i].StartTime != rawTasks[j].StartTime {
			return rawTasks[i].StartTime > rawTasks[j].StartTime
		}
		if rawTasks[i].ConversationID != rawTasks[j].ConversationID {
			return rawTasks[i].ConversationID < rawTasks[j].ConversationID
		}
		return rawTasks[i].StepIndex < rawTasks[j].StepIndex
	})

	return rawTasks, nil
}

func (p *Proxy) callListBackgroundTasksUpstream(ctx context.Context, port int, token string) ([]GlobalBackgroundTaskItem, error) {
	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/ListBackgroundTasks", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader("{}"))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.shortClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		if gz, gzErr := GetGzipReader(resp.Body); gzErr == nil {
			defer PutGzipReader(gz)
			reader = gz
		}
	}

	body, err := io.ReadAll(io.LimitReader(reader, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ListBackgroundTasks status %d: %s", resp.StatusCode, string(body))
	}

	var parsed upstreamListBackgroundTasksResp
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(parsed.Tasks))
	out := make([]GlobalBackgroundTaskItem, 0, len(parsed.Tasks))
	for _, entry := range parsed.Tasks {
		cid := strings.TrimSpace(firstNonEmptyStr(entry.ConversationID, entry.Conversation2))
		if cid == "" || IsDeletedCascade(cid) {
			continue
		}
		t := entry.Task
		taskID := strings.TrimSpace(firstNonEmptyStr(t.TaskID, t.TaskID2))
		stepIdx := t.StepIndex
		if stepIdx <= 0 {
			stepIdx = t.StepIndex2
		}
		if stepIdx <= 0 && taskID != "" {
			if idx := strings.LastIndex(taskID, "task-"); idx != -1 {
				if num, nErr := strconv.Atoi(taskID[idx+5:]); nErr == nil && num > 0 {
					stepIdx = num
				}
			}
		}
		if taskID == "" && stepIdx > 0 {
			taskID = fmt.Sprintf("%s/task-%d", cid, stepIdx)
		}
		dedupKey := fmt.Sprintf("%s:%s:%d", cid, taskID, stepIdx)
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true

		toolName := strings.TrimSpace(firstNonEmptyStr(t.ToolName, t.ToolName2))
		toolSummary := strings.TrimSpace(firstNonEmptyStr(t.ToolSummary, t.ToolSum2))
		cmdLine := strings.TrimSpace(t.Description)
		startTime := strings.TrimSpace(firstNonEmptyStr(t.StartTime, t.StartTime2))
		logURI := strings.TrimSpace(firstNonEmptyStr(t.LogURI, t.LogURI2))
		isDaemon := t.IsDaemon || t.IsDaemon2

		out = append(out, GlobalBackgroundTaskItem{
			ConversationID: cid,
			TaskID:         taskID,
			StepIndex:      stepIdx,
			ToolName:       toolName,
			ToolSummary:    toolSummary,
			CommandLine:    cmdLine,
			StartTime:      startTime,
			LogURI:         logURI,
			IsDaemon:       isDaemon,
		})
	}
	return out, nil
}

func firstNonEmptyStr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

type taskConvMeta struct {
	title     string
	workspace string
}

func (p *Proxy) lookupConversationMetaForTasks(ctx context.Context, port int, token string) map[string]taskConvMeta {
	out := make(map[string]taskConvMeta)
	raw, err := p.fetchAllTrajectoriesRaw(ctx, port, token)
	if err != nil {
		return out
	}
	var env struct {
		TrajectorySummaries map[string]struct {
			Summary            string `json:"summary"`
			TrajectoryMetadata *struct {
				WorkspaceUris []string `json:"workspaceUris"`
			} `json:"trajectoryMetadata"`
			Workspaces []struct {
				WorkspaceFolderAbsoluteUri string `json:"workspaceFolderAbsoluteUri"`
			} `json:"workspaces"`
		} `json:"trajectorySummaries"`
	}
	if json.Unmarshal(raw, &env) != nil {
		return out
	}

	aliases := loadWorkspaceAliases()
	for cid, s := range env.TrajectorySummaries {
		var wsURI string
		if len(s.Workspaces) > 0 {
			wsURI = strings.TrimSpace(s.Workspaces[0].WorkspaceFolderAbsoluteUri)
		}
		if wsURI == "" && s.TrajectoryMetadata != nil && len(s.TrajectoryMetadata.WorkspaceUris) > 0 {
			wsURI = strings.TrimSpace(s.TrajectoryMetadata.WorkspaceUris[0])
		}
		var wsName string
		if wsURI != "" {
			localPath := uriToPath(wsURI)
			normKey := normalizeWorkspaceKey(localPath)
			if alias, ok := aliases[normKey]; ok && strings.TrimSpace(alias) != "" {
				wsName = strings.TrimSpace(alias)
			} else if localPath != "" && !isDriveRoot(localPath) {
				wsName = filepath.Base(localPath)
			}
		}
		out[cid] = taskConvMeta{
			title:     strings.TrimSpace(s.Summary),
			workspace: wsName,
		}
	}
	return out
}

// HandleBackgroundTaskLog handles POST /gateway/tasks/background/log to read the tail of a running task's log file.
func (p *Proxy) HandleBackgroundTaskLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ConversationID string `json:"conversationId"`
		TaskID         string `json:"taskId"`
		LogURI         string `json:"logUri"`
		MaxBytes       int    `json:"maxBytes"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	logURI := strings.TrimSpace(req.LogURI)
	taskID := strings.TrimSpace(req.TaskID)
	cid := strings.TrimSpace(req.ConversationID)

	// Verify against active background tasks or strict task-log path pattern
	verifiedByUpstream := false
	port, token := p.ActiveUpstream()
	if port > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		tasks, err := p.callListBackgroundTasksUpstream(ctx, port, token)
		cancel()
		if err == nil {
			for _, t := range tasks {
				if (taskID != "" && t.TaskID == taskID) || (logURI != "" && t.LogURI == logURI) || (cid != "" && t.ConversationID == cid && logURI == "") {
					if logURI == "" && t.LogURI != "" {
						logURI = t.LogURI
					}
					if t.LogURI != "" && t.LogURI == logURI {
						verifiedByUpstream = true
					}
					if taskID == "" {
						taskID = t.TaskID
					}
					break
				}
			}
		}
	}

	if logURI == "" {
		writeJSONError(w, "logUri is not available for this task", http.StatusNotFound)
		return
	}

	localPath := uriToPath(logURI)
	if !verifiedByUpstream && !isAllowedBackgroundTaskLogPath(localPath) {
		writeJSONError(w, "forbidden log path", http.StatusForbidden)
		return
	}

	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultTaskLogTailBytes
	} else if maxBytes > maxTaskLogTailBytes {
		maxBytes = maxTaskLogTailBytes
	}

	content, sizeBytes, truncated, err := readLogFileTail(localPath, maxBytes)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"taskId":    taskID,
				"logUri":    logURI,
				"content":   "",
				"truncated": false,
				"sizeBytes": 0,
			})
			return
		}
		writeJSONError(w, "failed to read log: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"taskId":    taskID,
		"logUri":    logURI,
		"content":   content,
		"truncated": truncated,
		"sizeBytes": sizeBytes,
	})
}

func isAllowedBackgroundTaskLogPath(localPath string) bool {
	if localPath == "" {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(localPath))
	if !strings.HasSuffix(clean, ".log") && !strings.HasSuffix(clean, ".txt") {
		return false
	}
	return strings.Contains(clean, "/.system_generated/tasks/")
}

func readLogFileTail(localPath string, maxBytes int) (content string, totalSize int64, truncated bool, err error) {
	f, err := os.Open(localPath)
	if err != nil {
		return "", 0, false, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return "", 0, false, err
	}
	totalSize = st.Size()
	if totalSize == 0 {
		return "", 0, false, nil
	}

	var offset int64
	if totalSize > int64(maxBytes) {
		offset = totalSize - int64(maxBytes)
		truncated = true
	}

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return "", totalSize, false, err
		}
	}

	buf, err := io.ReadAll(io.LimitReader(f, int64(maxBytes)))
	if err != nil {
		return "", totalSize, false, err
	}

	// Trim incomplete leading UTF-8 rune if we seeked into the middle of a multi-byte character
	if truncated {
		for len(buf) > 0 && !utf8.Valid(buf) {
			r, size := utf8.DecodeRune(buf)
			if r == utf8.RuneError && size == 1 {
				buf = buf[1:]
			} else {
				break
			}
		}
		if idx := bytes.IndexByte(buf, '\n'); idx >= 0 && idx < 120 {
			buf = buf[idx+1:]
		}
	}

	return string(buf), totalSize, truncated, nil
}
