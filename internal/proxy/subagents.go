package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// 子代理可见性。
//
// 父会话每次 invoke_subagent 会产生一个 CORTEX_STEP_TYPE_INVOKE_SUBAGENT 步骤：
//   - invokeSubagent.subagents[i]  描述第 i 个子代理（typeName / role / initialPrompt / modelTier）
//   - invokeSubagent.results[i]    给出它的 conversationId（子会话本身就是一个普通会话，可用 GetCascadeTrajectory 读取）
//
// 子代理的实时状态不在父会话里，要从会话列表快照（GetAllCascadeTrajectories）按 conversationId 取，
// 因此补全状态发生在推送/返回之前，而不是缓存的解析结果里。
// 关停用 ForceStopCascadeTree：它只停该会话及其下属，不会波及父会话。

const (
	stepTypeInvokeSubagent = "CORTEX_STEP_TYPE_INVOKE_SUBAGENT"

	subagentStatusRunning = "running"
	subagentStatusDone    = "done"
	subagentStatusGone    = "gone"

	subagentPromptMaxRunes = 600
)

// buildSubagents lists the subagents a conversation dispatched, in dispatch order. Entries whose
// conversation id is not known yet (result not delivered) are skipped.
func (p *Proxy) buildSubagents(steps []TrajectoryStep) []SubagentItem {
	var out []SubagentItem
	for idx, s := range steps {
		if s.Type != stepTypeInvokeSubagent || s.InvokeSubagent == nil {
			continue
		}
		stepIdx := idx
		if s.Metadata.SourceTrajectoryStepInfo != nil && s.Metadata.SourceTrajectoryStepInfo.StepIndex > 0 {
			stepIdx = s.Metadata.SourceTrajectoryStepInfo.StepIndex
		}
		inv := s.InvokeSubagent
		for i, res := range inv.Results {
			id := strings.TrimSpace(res.ConversationID)
			if id == "" {
				continue
			}
			item := SubagentItem{ConversationID: id, StepIndex: stepIdx}
			if i < len(inv.Subagents) {
				spec := inv.Subagents[i]
				item.TypeName = spec.TypeName
				item.Role = spec.Role
				item.ModelTier = spec.ModelTier
				item.Prompt = truncateRunes(spec.InitialPrompt, subagentPromptMaxRunes)
			}
			out = append(out, item)
		}
	}
	return out
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

// subagentIdentity reports whether the trajectory itself is a subagent and, if so, its parent and role.
func subagentIdentity(raw *upstreamTrajectoryResp) (parentID, role string) {
	if raw == nil || raw.Trajectory.Metadata == nil {
		return "", ""
	}
	m := raw.Trajectory.Metadata
	parentID = strings.TrimSpace(m.ParentConversationID)
	if m.SubagentSpec != nil {
		role = m.SubagentSpec.Role
		if role == "" {
			role = m.SubagentSpec.TypeName
		}
	}
	return parentID, role
}

// subagentLive is the slice of a list-snapshot entry we need.
type subagentLive struct {
	Status    string
	StepCount int
	Title     string
	Parent    string
}

// liveSummaries looks the given conversations up in the shared list snapshot.
// Missing ids are absent from the result.
func (p *Proxy) liveSummaries(ids []string) map[string]subagentLive {
	out := map[string]subagentLive{}
	if len(ids) == 0 {
		return out
	}
	port, token := p.ActiveUpstream()
	if port == 0 {
		return out
	}
	raw, err := p.fetchAllTrajectoriesRaw(context.Background(), port, token)
	if err != nil {
		return out
	}
	var env struct {
		TrajectorySummaries map[string]json.RawMessage `json:"trajectorySummaries"`
	}
	if json.Unmarshal(raw, &env) != nil {
		return out
	}
	for _, id := range ids {
		b, ok := env.TrajectorySummaries[id]
		if !ok {
			continue
		}
		var s struct {
			Status             string `json:"status"`
			StepCount          int    `json:"stepCount"`
			Summary            string `json:"summary"`
			TrajectoryMetadata struct {
				ParentConversationID string `json:"parentConversationId"`
			} `json:"trajectoryMetadata"`
		}
		if json.Unmarshal(b, &s) != nil {
			continue
		}
		out[id] = subagentLive{Status: s.Status, StepCount: s.StepCount, Title: s.Summary, Parent: strings.TrimSpace(s.TrajectoryMetadata.ParentConversationID)}
	}
	return out
}

// enrichSubagents returns a copy of items with live status / step count / title filled in.
func (p *Proxy) enrichSubagents(items []SubagentItem) []SubagentItem {
	if len(items) == 0 {
		return items
	}
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ConversationID
	}
	live := p.liveSummaries(ids)
	out := make([]SubagentItem, len(items))
	for i, it := range items {
		if l, ok := live[it.ConversationID]; ok {
			it.StepCount = l.StepCount
			it.Title = l.Title
			if l.Status == "CASCADE_RUN_STATUS_RUNNING" {
				it.Status = subagentStatusRunning
			} else {
				it.Status = subagentStatusDone
			}
		} else {
			it.Status = subagentStatusGone
		}
		out[i] = it
	}
	return out
}

// subagentLiveSignature folds the subagents' live state into the stream's change detection, because a
// child making progress does not alter the parent's own trajectory. Returns "" when there are none.
func (p *Proxy) subagentLiveSignature(raw *upstreamTrajectoryResp) string {
	if raw == nil {
		return ""
	}
	var ids []string
	for _, s := range raw.Trajectory.Steps {
		if s.Type != stepTypeInvokeSubagent || s.InvokeSubagent == nil {
			continue
		}
		for _, r := range s.InvokeSubagent.Results {
			if id := strings.TrimSpace(r.ConversationID); id != "" {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return ""
	}
	live := p.liveSummaries(ids)
	var b strings.Builder
	for _, id := range ids {
		l, ok := live[id]
		fmt.Fprintf(&b, "%s:%t:%s:%d;", id, ok, l.Status, l.StepCount)
	}
	return b.String()
}

// callForceStopCascadeTree stops a conversation and everything it spawned, returning the ids actually stopped.
func (p *Proxy) callForceStopCascadeTree(conversationID string, port int, token string) ([]string, error) {
	body, _ := json.Marshal(map[string]string{"conversationId": conversationID})
	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/ForceStopCascadeTree", port)
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(body)))
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
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ForceStopCascadeTree returned status %d: %s", resp.StatusCode, string(respBody))
	}
	var parsed struct {
		Stopped []string `json:"stoppedConversationIds"`
	}
	_ = json.Unmarshal(respBody, &parsed)
	return parsed.Stopped, nil
}

// HandleSubagentStop handles POST /gateway/subagent/stop {"conversationId": "<subagent id>"}.
// It refuses ids that are not subagents, so this endpoint can never stop a user's main conversation.
func (p *Proxy) HandleSubagentStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ConversationID string `json:"conversationId"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(req.ConversationID)
	if id == "" {
		writeJSONError(w, "conversationId is required", http.StatusBadRequest)
		return
	}
	port, token := p.ActiveUpstream()
	if port == 0 {
		writeJSONError(w, "No active Antigravity upstream", http.StatusServiceUnavailable)
		return
	}

	// 先刷新一次快照再判断，避免拿到过期的父子关系
	p.invalidateAllTrajectories()
	live, ok := p.liveSummaries([]string{id})[id]
	if !ok {
		writeJSONError(w, "subagent not found", http.StatusNotFound)
		return
	}
	if live.Parent == "" {
		writeJSONError(w, "conversation is not a subagent", http.StatusBadRequest)
		return
	}

	stopped, err := p.callForceStopCascadeTree(id, port, token)
	if err != nil {
		slog.Warn(fmt.Sprintf("[Proxy] Stop subagent failed (%s)", shortCascadeID(id)), "err", err)
		writeJSONError(w, "stop subagent failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info(fmt.Sprintf("[Proxy] Stopped subagent %s (parent %s), %d conversation(s)", shortCascadeID(id), shortCascadeID(live.Parent), len(stopped)))

	// 让父、子会话的推送流立刻重算（子代理状态不在父会话的轨迹里）
	p.invalidateAllTrajectories()
	for _, c := range []string{id, live.Parent} {
		ClearTrajectoryCache(c)
		p.notifyStreamTouch(c)
	}

	out, _ := json.Marshal(map[string]interface{}{"success": true, "stopped": stopped})
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}
