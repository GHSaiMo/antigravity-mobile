package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// 本会话改动（正向 diff）。
//
// language_server 没有直接给出「会话累计净改动」的接口，但 GetRevertPreview(stepIndex=-1)
// 返回的是「把整个会话的代码改动全部撤销」的预览，即累计净改动的反向 diff。
// 把 INSERT/DELETE 对调、CREATE/DELETE 对调，就得到正向的累计 diff，且天然按文件合并了多次编辑。
// fromStepIndex 非空时回滚目标为 fromStepIndex-1，得到「从该步骤起」的改动（例如某一轮对话）。

// CascadeChangesRequest is the request body of POST /gateway/cascade/changes.
type CascadeChangesRequest struct {
	CascadeID string `json:"cascadeId"`
	// FromStepIndex limits the result to changes made at or after this step. Nil means the whole conversation.
	FromStepIndex *int `json:"fromStepIndex,omitempty"`
}

// CascadeChangesFile is one file changed by the agent, expressed as a forward diff.
type CascadeChangesFile struct {
	FileURI    string           `json:"fileUri"`
	FileName   string           `json:"fileName"`
	ActionType string           `json:"actionType"` // "CREATE", "MODIFY", "DELETE"
	Additions  int              `json:"additions"`
	Deletions  int              `json:"deletions"`
	DiffLines  []RevertDiffLine `json:"diffLines"`
}

// CascadeChangesResponse is the response body of POST /gateway/cascade/changes.
type CascadeChangesResponse struct {
	CascadeID     string               `json:"cascadeId"`
	FromStepIndex *int                 `json:"fromStepIndex,omitempty"`
	Files         []CascadeChangesFile `json:"files"`
	Additions     int                  `json:"additions"`
	Deletions     int                  `json:"deletions"`
	HasChanges    bool                 `json:"hasChanges"`
}

// invertRevertPreview converts a revert preview (the diff that undoes the agent's work) into the
// forward diff of what the agent changed.
func invertRevertPreview(files []RevertPreviewFile) []CascadeChangesFile {
	out := make([]CascadeChangesFile, 0, len(files))
	for _, f := range files {
		action := "MODIFY"
		switch f.ActionType {
		case "DELETE": // 撤销时要删除 = Agent 新建了它
			action = "CREATE"
		case "CREATE": // 撤销时要重建 = Agent 删除了它
			action = "DELETE"
		}

		lines := make([]RevertDiffLine, len(f.DiffLines))
		for i, l := range f.DiffLines {
			switch l.Type {
			case "INSERT":
				l.Type = "DELETE"
			case "DELETE":
				l.Type = "INSERT"
			}
			lines[i] = l
		}
		lines = deletesBeforeInserts(lines)

		out = append(out, CascadeChangesFile{
			FileURI:    f.FileURI,
			FileName:   f.FileName,
			ActionType: action,
			Additions:  f.Deletions, // 反向 diff 的删除行就是 Agent 新增的行
			Deletions:  f.Additions,
			DiffLines:  lines,
		})
	}
	return out
}

// deletesBeforeInserts reorders every contiguous run of changed lines so that removed lines come
// first, matching the conventional unified-diff layout after the INSERT/DELETE swap.
func deletesBeforeInserts(lines []RevertDiffLine) []RevertDiffLine {
	out := make([]RevertDiffLine, 0, len(lines))
	for i := 0; i < len(lines); {
		if lines[i].Type == "UNCHANGED" {
			out = append(out, lines[i])
			i++
			continue
		}
		j := i
		for j < len(lines) && lines[j].Type != "UNCHANGED" {
			j++
		}
		for _, l := range lines[i:j] {
			if l.Type == "DELETE" {
				out = append(out, l)
			}
		}
		for _, l := range lines[i:j] {
			if l.Type == "INSERT" {
				out = append(out, l)
			}
		}
		i = j
	}
	return out
}

// GetCascadeChanges returns the cumulative forward diff of a conversation (or of the steps from fromStepIndex on).
func (p *Proxy) GetCascadeChanges(cascadeID string, fromStepIndex *int, port int, token string) (*CascadeChangesResponse, error) {
	target := -1
	if fromStepIndex != nil && *fromStepIndex > 0 {
		target = *fromStepIndex - 1
	}
	preview, err := p.GetRevertPreview(cascadeID, 0, &target, port, token)
	if err != nil {
		return nil, err
	}

	res := &CascadeChangesResponse{
		CascadeID:     cascadeID,
		FromStepIndex: fromStepIndex,
		Files:         invertRevertPreview(preview.Files),
	}
	for _, f := range res.Files {
		res.Additions += f.Additions
		res.Deletions += f.Deletions
	}
	res.HasChanges = len(res.Files) > 0
	return res, nil
}

// HandleCascadeChanges handles POST /gateway/cascade/changes.
func (p *Proxy) HandleCascadeChanges(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req CascadeChangesRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 65536)).Decode(&req); err != nil {
		writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	req.CascadeID = strings.TrimSpace(req.CascadeID)
	if req.CascadeID == "" || !cascadeIDRe.MatchString(req.CascadeID) {
		writeJSONError(w, "cascadeId is required", http.StatusBadRequest)
		return
	}

	port, token := p.ActiveUpstream()
	if port == 0 {
		writeJSONError(w, "No active Antigravity upstream", http.StatusServiceUnavailable)
		return
	}

	res, err := p.GetCascadeChanges(req.CascadeID, req.FromStepIndex, port, token)
	if err != nil {
		slog.Warn(fmt.Sprintf("[Proxy] Cascade changes failed for cascade %s", shortCascadeID(req.CascadeID)), "err", err)
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := json.Marshal(res)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
