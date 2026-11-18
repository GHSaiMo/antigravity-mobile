package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// TrustFolderItem mirrors one folder entry inside Antigravity ProjectTrustStatus.
type TrustFolderItem struct {
	FolderURI  string `json:"folderUri,omitempty"`
	Path       string `json:"path,omitempty"`
	Trusted    bool   `json:"trusted"`
	TrustedVia string `json:"trustedVia,omitempty"`
}

// ProjectTrustResponse is the normalized gateway response for GET/POST /gateway/projects/trust.
type ProjectTrustResponse struct {
	OK               bool              `json:"ok"`
	ProjectID        string            `json:"projectId,omitempty"`
	IsProjectTrusted bool              `json:"isProjectTrusted"`
	HasBlockedItems  bool              `json:"hasBlockedItems"`
	Folders          []TrustFolderItem `json:"folders,omitempty"`
	Message          string            `json:"message,omitempty"`
}

type upstreamTrustStatusEnvelope struct {
	Status *struct {
		IsProjectTrusted bool              `json:"isProjectTrusted"`
		HasBlockedItems  bool              `json:"hasBlockedItems"`
		Folders          []TrustFolderItem `json:"folders"`
	} `json:"status"`
	IsProjectTrusted *bool             `json:"isProjectTrusted"`
	HasBlockedItems  *bool             `json:"hasBlockedItems"`
	Folders          []TrustFolderItem `json:"folders"`
}

func parseUpstreamTrustStatus(projectID string, raw []byte) ProjectTrustResponse {
	res := ProjectTrustResponse{
		OK:        true,
		ProjectID: projectID,
	}
	var env upstreamTrustStatusEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		res.IsProjectTrusted = true
		return res
	}
	if env.Status != nil {
		res.IsProjectTrusted = env.Status.IsProjectTrusted
		res.HasBlockedItems = env.Status.HasBlockedItems
		res.Folders = env.Status.Folders
		return res
	}
	if env.IsProjectTrusted != nil {
		res.IsProjectTrusted = *env.IsProjectTrusted
	} else {
		res.IsProjectTrusted = true
	}
	if env.HasBlockedItems != nil {
		res.HasBlockedItems = *env.HasBlockedItems
	}
	res.Folders = env.Folders
	return res
}

func (p *Proxy) resolveProjectIDForTrust(ctx context.Context, projectID, cascadeID string, port int, token string) string {
	pid := strings.TrimSpace(projectID)
	if pid != "" {
		return pid
	}
	cid := strings.TrimSpace(cascadeID)
	if cid == "" {
		return ""
	}
	if port <= 0 {
		return ""
	}
	raw, err := p.fetchAllTrajectoriesRaw(ctx, port, token)
	if err != nil {
		return ""
	}
	var parsed struct {
		Summaries map[string]struct {
			Metadata struct {
				ProjectID     string   `json:"projectId"`
				WorkspaceURIs []string `json:"workspaceUris"`
			} `json:"trajectoryMetadata"`
			Workspaces []struct {
				URI string `json:"workspaceFolderAbsoluteUri"`
			} `json:"workspaces"`
		} `json:"trajectorySummaries"`
	}
	if json.Unmarshal(raw, &parsed) != nil {
		return ""
	}
	entry, ok := parsed.Summaries[cid]
	if !ok {
		return ""
	}
	if pid = strings.TrimSpace(entry.Metadata.ProjectID); pid != "" && pid != "outside-of-project" {
		return pid
	}
	var wsURIs []string
	wsURIs = append(wsURIs, entry.Metadata.WorkspaceURIs...)
	for _, w := range entry.Workspaces {
		if w.URI != "" {
			wsURIs = append(wsURIs, w.URI)
		}
	}
	if len(wsURIs) > 0 {
		resolvedPID, _ := p.BuildWorkspaceDisplayNameResolver()("", wsURIs)
		if resolvedPID != "" {
			return resolvedPID
		}
	}
	return ""
}

// HandleProjectTrust handles GET/POST /gateway/projects/trust (GetProjectTrustStatus / SetWorkspaceTrust).
func (p *Proxy) HandleProjectTrust(w http.ResponseWriter, r *http.Request) {
	port, token := p.ActiveUpstream()
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		projectID := p.resolveProjectIDForTrust(r.Context(), q.Get("projectId"), q.Get("cascadeId"), port, token)
		if projectID == "" || projectID == "outside-of-project" {
			writeJSONStatus(w, http.StatusOK, ProjectTrustResponse{
				OK:               true,
				ProjectID:        projectID,
				IsProjectTrusted: true,
			})
			return
		}
		raw, code, err := p.callUpstreamRPC(r.Context(), "GetProjectTrustStatus", map[string]interface{}{
			"projectId": projectID,
		})
		if err != nil {
			writeRPCError(w, code, err.Error())
			return
		}
		if code != http.StatusOK {
			writeRPCError(w, code, strings.TrimSpace(string(raw)))
			return
		}
		writeJSONStatus(w, http.StatusOK, parseUpstreamTrustStatus(projectID, raw))

	case http.MethodPost:
		var req struct {
			ProjectID string   `json:"projectId"`
			CascadeID string   `json:"cascadeId"`
			Paths     []string `json:"paths"`
			Trusted   *bool    `json:"trusted"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		trusted := true
		if req.Trusted != nil {
			trusted = *req.Trusted
		}
		projectID := p.resolveProjectIDForTrust(r.Context(), req.ProjectID, req.CascadeID, port, token)
		if projectID == "" || projectID == "outside-of-project" {
			writeJSONStatus(w, http.StatusOK, ProjectTrustResponse{
				OK:               true,
				ProjectID:        projectID,
				IsProjectTrusted: true,
			})
			return
		}
		paths := req.Paths
		if len(paths) == 0 {
			if statusRaw, code, err := p.callUpstreamRPC(r.Context(), "GetProjectTrustStatus", map[string]interface{}{
				"projectId": projectID,
			}); err == nil && code == http.StatusOK {
				current := parseUpstreamTrustStatus(projectID, statusRaw)
				for _, f := range current.Folders {
					pStr := strings.TrimSpace(f.Path)
					if pStr == "" && f.FolderURI != "" {
						pStr = uriToFilePath(f.FolderURI)
					}
					if pStr != "" {
						paths = append(paths, pStr)
					}
				}
			}
		}
		raw, code, err := p.callUpstreamRPC(r.Context(), "SetWorkspaceTrust", map[string]interface{}{
			"projectId": projectID,
			"paths":     paths,
			"trusted":   trusted,
		})
		if err != nil {
			writeRPCError(w, code, err.Error())
			return
		}
		if code != http.StatusOK {
			writeRPCError(w, code, strings.TrimSpace(string(raw)))
			return
		}
		writeJSONStatus(w, http.StatusOK, parseUpstreamTrustStatus(projectID, raw))

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
