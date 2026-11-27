package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"antigravity-mobile/internal/proxy"
)

const (
	nodeProjectIDPrefix = "mgy-node:"
	nodeURIQueryParam   = "mgy_node"
)

var brainCascadeRe = regexp.MustCompile(`(?i)(?:/brain/|/static/artifacts/)([0-9a-fA-F-]{8,64})(?:/|$)`)

// EncodeNodeProjectID wraps a remote node's project ID so colliding IDs across nodes stay unique
// and carry lossless routing metadata.
func EncodeNodeProjectID(nodeID, origID string) string {
	nodeID = strings.TrimSpace(nodeID)
	origID = strings.TrimSpace(origID)
	if nodeID == "" || nodeID == LocalNodeID {
		return origID
	}
	return nodeProjectIDPrefix + nodeID + ":" + origID
}

// DecodeNodeProjectID extracts (nodeID, origID, ok) from an encoded Hub project ID.
func DecodeNodeProjectID(raw string) (string, string, bool) {
	if !strings.HasPrefix(raw, nodeProjectIDPrefix) {
		return "", raw, false
	}
	rest := strings.TrimPrefix(raw, nodeProjectIDPrefix)
	idx := strings.Index(rest, ":")
	if idx <= 0 {
		return "", raw, false
	}
	return rest[:idx], rest[idx+1:], true
}

// AppendNodeHintToURI attaches ?mgy_node=<nodeID> onto a workspace/file URI without mutating its path.
func AppendNodeHintToURI(rawURI, nodeID string) string {
	rawURI = strings.TrimSpace(rawURI)
	nodeID = strings.TrimSpace(nodeID)
	if rawURI == "" || nodeID == "" || nodeID == LocalNodeID {
		return rawURI
	}
	u, err := url.Parse(rawURI)
	if err != nil {
		return rawURI
	}
	q := u.Query()
	q.Set(nodeURIQueryParam, nodeID)
	u.RawQuery = q.Encode()
	return u.String()
}

// StripNodeHintFromURI extracts and strips ?mgy_node=<nodeID> from a URI.
func StripNodeHintFromURI(rawURI string) (cleanURI string, nodeID string, found bool) {
	rawURI = strings.TrimSpace(rawURI)
	if rawURI == "" || !strings.Contains(rawURI, nodeURIQueryParam) {
		return rawURI, "", false
	}
	u, err := url.Parse(rawURI)
	if err != nil {
		return rawURI, "", false
	}
	q := u.Query()
	nid := strings.TrimSpace(q.Get(nodeURIQueryParam))
	if nid == "" {
		return rawURI, "", false
	}
	q.Del(nodeURIQueryParam)
	u.RawQuery = q.Encode()
	return u.String(), nid, true
}

func normalizeWorkspaceKey(raw string) string {
	s, _, _ := StripNodeHintFromURI(raw)
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(s), "file://") {
		if u, err := url.Parse(s); err == nil && u.Path != "" {
			s = u.Path
		}
	}
	s = strings.ReplaceAll(s, "\\", "/")
	s = strings.TrimRight(s, "/")
	return s
}

// =========================================================================
// Phase 2A: Cross-Node Projects & New Conversation Creation
// =========================================================================

func (h *Hub) handleAggregatedProjects(w http.ResponseWriter, r *http.Request, local http.Handler) {
	multiNode := h.isMultiNode()
	remoteNodes := h.filteredRemoteNodes()
	includeLocal := h.includeLocalInAggregation()

	type projResult struct {
		nodeID   string
		nodeName string
		items    []proxy.ProjectItem
	}
	var results []projResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	if includeLocal {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			local.ServeHTTP(rec, cloneLocalRequest(r, nil))
			if rec.Code != http.StatusOK {
				return
			}
			var items []proxy.ProjectItem
			if json.Unmarshal(rec.Body.Bytes(), &items) == nil {
				mu.Lock()
				results = append(results, projResult{
					nodeID:   LocalNodeID,
					nodeName: h.localNodeDisplayName(),
					items:    items,
				})
				mu.Unlock()
			}
		}()
	}

	for _, n := range remoteNodes {
		wg.Add(1)
		go func(node NodeConfig) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(r.Context(), h.listTimeout)
			defer cancel()
			code, _, body, err := h.doNodeRequest(ctx, h.listClient, node, http.MethodGet, "/gateway/projects", "", nil, "")
			if err != nil || code != http.StatusOK {
				return
			}
			var items []proxy.ProjectItem
			if json.Unmarshal(body, &items) == nil {
				mu.Lock()
				results = append(results, projResult{
					nodeID:   node.ID,
					nodeName: node.Name,
					items:    items,
				})
				mu.Unlock()
			}
		}(n)
	}
	wg.Wait()

	merged := make([]proxy.ProjectItem, 0, 32)
	for _, res := range results {
		for _, item := range res.items {
			if item.URI != "" {
				h.indexWorkspace(item.URI, res.nodeID)
			} else if item.Path != "" {
				h.indexWorkspace(item.Path, res.nodeID)
			}
			if item.ID != "" {
				h.indexProjectID(item.ID, res.nodeID)
			}

			if multiNode {
				prefix := "[" + res.nodeName + "] "
				if !strings.HasPrefix(item.Name, prefix) {
					item.Name = prefix + item.Name
				}
				if item.Alias != "" && !strings.HasPrefix(item.Alias, prefix) {
					item.Alias = prefix + item.Alias
				}
			}
			if res.nodeID != LocalNodeID {
				item.URI = AppendNodeHintToURI(item.URI, res.nodeID)
				item.ID = EncodeNodeProjectID(res.nodeID, item.ID)
			}
			merged = append(merged, item)
		}
	}

	sort.SliceStable(merged, func(i, j int) bool {
		ti, tj := merged[i].LastActive, merged[j].LastActive
		if ti != nil && tj != nil && !ti.Equal(*tj) {
			return ti.After(*tj)
		}
		if ti != nil && tj == nil {
			return true
		}
		if ti == nil && tj != nil {
			return false
		}
		if merged[i].SessionCount != merged[j].SessionCount {
			return merged[i].SessionCount > merged[j].SessionCount
		}
		return merged[i].Name < merged[j].Name
	})

	writeJSON(w, http.StatusOK, merged)
}

func (h *Hub) handleAggregatedReadProjects(w http.ResponseWriter, r *http.Request, local http.Handler) {
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 256*1024))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	if len(bytes.TrimSpace(bodyBytes)) == 0 {
		bodyBytes = []byte("{}")
	}

	remoteNodes := h.filteredRemoteNodes()
	includeLocal := h.includeLocalInAggregation()
	multiNode := h.isMultiNode()

	var allProjects []map[string]any
	var mu sync.Mutex
	var wg sync.WaitGroup

	appendRaw := func(nodeID, nodeName string, raw []byte) {
		var env struct {
			Projects []map[string]any `json:"projects"`
		}
		if json.Unmarshal(raw, &env) != nil || len(env.Projects) == 0 {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, prj := range env.Projects {
			if prj == nil {
				continue
			}
			if pid, _ := prj["projectId"].(string); pid != "" {
				h.indexProjectID(pid, nodeID)
				if nodeID != LocalNodeID {
					prj["projectId"] = EncodeNodeProjectID(nodeID, pid)
				}
			}
			if uris, ok := prj["workspaceUris"].([]any); ok {
				for idx, u := range uris {
					if us, _ := u.(string); us != "" {
						h.indexWorkspace(us, nodeID)
						if nodeID != LocalNodeID {
							uris[idx] = AppendNodeHintToURI(us, nodeID)
						}
					}
				}
			}
			if multiNode {
				if name, _ := prj["name"].(string); name != "" {
					prefix := "[" + nodeName + "] "
					if !strings.HasPrefix(name, prefix) {
						prj["name"] = prefix + name
					}
				}
			}
			allProjects = append(allProjects, prj)
		}
	}

	if includeLocal {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			local.ServeHTTP(rec, cloneLocalRequest(r, bodyBytes))
			if rec.Code == http.StatusOK {
				appendRaw(LocalNodeID, h.localNodeDisplayName(), rec.Body.Bytes())
			}
		}()
	}

	for _, n := range remoteNodes {
		wg.Add(1)
		go func(node NodeConfig) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(r.Context(), h.listTimeout)
			defer cancel()
			code, _, respBody, reqErr := h.doNodeRequest(
				ctx, h.listClient, node, http.MethodPost,
				"/api/exa.language_server_pb.LanguageServerService/ReadProjects",
				"", bodyBytes, "application/json",
			)
			if reqErr == nil && code == http.StatusOK {
				appendRaw(node.ID, node.Name, respBody)
			}
		}(n)
	}
	wg.Wait()

	if allProjects == nil {
		allProjects = []map[string]any{}
	}
	respBytes, _ := json.Marshal(map[string]any{"projects": allProjects})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Connect-Protocol-Version", "1")
	w.Header().Set("Content-Length", strconv.Itoa(len(respBytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBytes)
}

// handleWorkspaceOrCreationRoute handles StartCascade, /gateway/cascade/new, /gateway/worktree/*,
// /gateway/git/*, /gateway/projects/alias|trust, and /gateway/search/code|files.
// It resolves the target node from encoded projectId / ?mgy_node= URI hints / cascadeId / workspaceIndex,
// strips Hub-internal markers before forwarding, and indexes newly created cascadeIds immediately.
func (h *Hub) handleWorkspaceOrCreationRoute(w http.ResponseWriter, r *http.Request, local http.Handler) {
	var bodyBytes []byte
	var rawMap map[string]any
	if r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch) {
		var err error
		bodyBytes, err = io.ReadAll(io.LimitReader(r.Body, 5<<20))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "failed to read request body")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		r.ContentLength = int64(len(bodyBytes))
		if len(bodyBytes) > 0 {
			_ = json.Unmarshal(bodyBytes, &rawMap)
		}
	}

	targetNodeID := ""
	modifiedBody := false

	// 1. Explicit query parameter (?node= / ?mgy_node=)
	if explicit := firstNonEmpty(r.URL.Query().Get("node"), r.URL.Query().Get(nodeURIQueryParam)); explicit != "" {
		targetNodeID = explicit
	}

	// 2. Query parameters cascadeId / projectId / workspace / uri
	q := r.URL.Query()
	queryModified := false
	if pid := strings.TrimSpace(q.Get("projectId")); pid != "" {
		if nid, orig, ok := DecodeNodeProjectID(pid); ok {
			if targetNodeID == "" {
				targetNodeID = nid
			}
			q.Set("projectId", orig)
			queryModified = true
		} else if targetNodeID == "" {
			if nid, ok := h.lookupProjectIDNode(pid); ok {
				targetNodeID = nid
			}
		}
	}
	for _, key := range []string{"workspaceUri", "workspace", "uri", "repoUri", "path"} {
		if rawVal := strings.TrimSpace(q.Get(key)); rawVal != "" {
			if clean, nid, ok := StripNodeHintFromURI(rawVal); ok {
				if targetNodeID == "" {
					targetNodeID = nid
				}
				q.Set(key, clean)
				queryModified = true
			} else if targetNodeID == "" {
				if nid, ok := h.lookupUniqueWorkspaceNode(rawVal); ok {
					targetNodeID = nid
				}
			}
		}
	}
	if targetNodeID == "" {
		if cid := extractCascadeIDFromQuery(q); cid != "" {
			if nid, ok := h.lookupCascadeNode(cid); ok {
				targetNodeID = nid
			}
		}
	}

	// 3. JSON body fields (explicit nodeId, encoded projectId, URI hints, cascadeId, workspaceIndex)
	if rawMap != nil {
		if explicit, _ := rawMap["nodeId"].(string); explicit != "" && targetNodeID == "" {
			targetNodeID = strings.TrimSpace(explicit)
			delete(rawMap, "nodeId")
			modifiedBody = true
		}
		if pid, _ := rawMap["projectId"].(string); pid != "" {
			if nid, orig, ok := DecodeNodeProjectID(pid); ok {
				if targetNodeID == "" {
					targetNodeID = nid
				}
				rawMap["projectId"] = orig
				modifiedBody = true
			} else if targetNodeID == "" {
				if nid, ok := h.lookupProjectIDNode(pid); ok {
					targetNodeID = nid
				}
			}
		}
		for _, field := range []string{"workspaceUri", "repoUri", "worktreeDirUri", "targetWorkspaceUri", "workspacePath", "path"} {
			if rawVal, ok := rawMap[field].(string); ok && strings.TrimSpace(rawVal) != "" {
				if clean, nid, ok := StripNodeHintFromURI(rawVal); ok {
					if targetNodeID == "" {
						targetNodeID = nid
					}
					rawMap[field] = clean
					modifiedBody = true
				} else if targetNodeID == "" {
					if nid, ok := h.lookupUniqueWorkspaceNode(rawVal); ok {
						targetNodeID = nid
					}
				}
			}
		}
		if uris, ok := rawMap["workspaceUris"].([]any); ok {
			for i, u := range uris {
				if us, ok := u.(string); ok && strings.TrimSpace(us) != "" {
					if clean, nid, ok := StripNodeHintFromURI(us); ok {
						if targetNodeID == "" {
							targetNodeID = nid
						}
						uris[i] = clean
						modifiedBody = true
					} else if targetNodeID == "" {
						if nid, ok := h.lookupUniqueWorkspaceNode(us); ok {
							targetNodeID = nid
						}
					}
				}
			}
		}
		if targetNodeID == "" {
			if cid := extractCascadeIDFromMap(rawMap); cid != "" {
				if nid, ok := h.lookupCascadeNode(cid); ok {
					targetNodeID = nid
				}
			}
		}
	}

	if modifiedBody && rawMap != nil {
		if patched, err := json.Marshal(rawMap); err == nil {
			bodyBytes = patched
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
			r.ContentLength = int64(len(bodyBytes))
		}
	}

	var overrideQuery *string
	if queryModified {
		q.Del("node")
		q.Del(nodeURIQueryParam)
		enc := q.Encode()
		overrideQuery = &enc
		r.URL.RawQuery = enc
	}

	var remoteNode NodeConfig
	useRemote := false
	if targetNodeID != "" {
		if !strings.EqualFold(targetNodeID, LocalNodeID) {
			if n, ok := h.store.GetNode(targetNodeID); ok && n.Enabled {
				remoteNode = n
				useRemote = true
			}
		}
	} else {
		// Pure chat or unscoped workspace call -> active filtered node or local/default remote node.
		remoteNode, useRemote = h.pickDefaultTarget()
	}

	isCreation := r.URL.Path == "/gateway/cascade/new" ||
		isRPCMethod(r.URL.Path, "StartCascade") ||
		r.URL.Path == "/gateway/worktree/create"

	if !useRemote {
		if isCreation {
			rec := httptest.NewRecorder()
			local.ServeHTTP(rec, cloneLocalRequest(r, bodyBytes))
			if rec.Code >= 200 && rec.Code < 300 {
				if cid := extractNewCascadeIDFromResponse(rec.Body.Bytes()); cid != "" {
					h.indexCascade(cid, LocalNodeID)
				}
				h.BumpEvents()
			}
			copyResponseHeaders(w.Header(), rec.Header(), true)
			w.Header().Set("Content-Length", strconv.Itoa(rec.Body.Len()))
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
			return
		}
		local.ServeHTTP(w, r)
		return
	}

	var inspect func(statusCode int, respBody []byte)
	if isCreation {
		inspect = func(statusCode int, respBody []byte) {
			if statusCode >= 200 && statusCode < 300 {
				if cid := extractNewCascadeIDFromResponse(respBody); cid != "" {
					h.indexCascade(cid, remoteNode.ID)
				}
				h.BumpEvents()
			}
		}
	}
	h.forwardHTTPToNode(w, r, remoteNode, "", overrideQuery, bodyBytes, inspect)
}

// =========================================================================
// Phase 2B: Cross-Node Attachment Upload & Lazy Replication
// =========================================================================

func (h *Hub) handleHubAttachmentUpload(w http.ResponseWriter, r *http.Request, local http.Handler) {
	// If the caller explicitly hints a remote node or known remote cascadeId, stream directly to that node!
	targetNodeID := firstNonEmpty(r.URL.Query().Get("node"), r.URL.Query().Get(nodeURIQueryParam))
	if targetNodeID == "" {
		if cid := extractCascadeIDFromQuery(r.URL.Query()); cid != "" {
			if nid, ok := h.lookupCascadeNode(cid); ok {
				targetNodeID = nid
			}
		}
	}
	if targetNodeID != "" && !strings.EqualFold(targetNodeID, LocalNodeID) {
		if node, ok := h.store.GetNode(targetNodeID); ok && node.Enabled {
			h.forwardHTTPToNode(w, r, node, "", nil, nil, func(statusCode int, respBody []byte) {
				if statusCode >= 200 && statusCode < 300 {
					var att proxy.Attachment
					if json.Unmarshal(respBody, &att) == nil && att.ID != "" {
						h.indexMu.Lock()
						entry := h.attachments[att.ID]
						if entry == nil {
							entry = &localAttachmentEntry{replicated: make(map[string]proxy.Attachment)}
							h.attachments[att.ID] = entry
						}
						entry.replicated[node.ID] = att
						h.indexMu.Unlock()
					}
				}
			})
			return
		}
	}

	// Default path: stage attachment in Hub's local inbox (works even in headless mode without local LS!)
	// and index its local path so SendUserCascadeMessage can lazily replicate it to whichever remote node
	// ultimately owns the target conversation.
	rec := httptest.NewRecorder()
	local.ServeHTTP(rec, cloneLocalRequest(r, nil))
	respBytes := rec.Body.Bytes()
	if rec.Code >= 200 && rec.Code < 300 {
		var att proxy.Attachment
		if json.Unmarshal(respBytes, &att) == nil && att.ID != "" {
			h.indexMu.Lock()
			entry := h.attachments[att.ID]
			if entry == nil {
				entry = &localAttachmentEntry{replicated: make(map[string]proxy.Attachment)}
				h.attachments[att.ID] = entry
			}
			entry.local = att
			h.indexMu.Unlock()
		}
	}
	copyResponseHeaders(w.Header(), rec.Header(), true)
	w.Header().Set("Content-Length", strconv.Itoa(len(respBytes)))
	w.WriteHeader(rec.Code)
	_, _ = w.Write(respBytes)
}

// replicateAttachmentsForRemoteNode ensures any attachment IDs referenced by a SendUserCascadeMessage
// (or /gateway/btw/start) payload exist on the target remote node, uploading missing files from Hub's
// local inbox and rewriting any embedded Hub disk paths to the remote node's disk paths.
func (h *Hub) replicateAttachmentsForRemoteNode(ctx context.Context, node NodeConfig, rawMap map[string]any) ([]byte, bool, error) {
	ids := collectAttachmentIDsFromMessageMap(rawMap)
	if len(ids) == 0 {
		return nil, false, nil
	}

	pathReplacements := make(map[string]string)
	for _, id := range ids {
		localAtt, remoteAtt, alreadyOnNode := h.lookupAttachmentState(id, node.ID)
		if alreadyOnNode {
			if localAtt.Path != "" && remoteAtt.Path != "" && localAtt.Path != remoteAtt.Path {
				pathReplacements[localAtt.Path] = remoteAtt.Path
			}
			continue
		}

		// Locate attachment file on Hub disk (from memory index or inbox scan).
		if localAtt.Path == "" {
			if found, ok := proxy.FindAttachmentByID(id); ok {
				localAtt = found
			} else {
				// Attachment may have been uploaded directly to the remote node previously; let remote resolve it.
				continue
			}
		}

		uploaded, err := h.uploadFileToRemoteNode(ctx, node, localAtt)
		if err != nil {
			return nil, false, err
		}
		h.recordReplicatedAttachment(id, localAtt, node.ID, uploaded)
		if localAtt.Path != "" && uploaded.Path != "" && localAtt.Path != uploaded.Path {
			pathReplacements[localAtt.Path] = uploaded.Path
		}
	}

	if len(pathReplacements) == 0 {
		return nil, false, nil
	}

	rewriteStrings := func(s string) (string, bool) {
		orig := s
		for oldPath, newPath := range pathReplacements {
			if strings.Contains(s, oldPath) {
				s = strings.ReplaceAll(s, oldPath, newPath)
			}
		}
		return s, s != orig
	}

	changed := false
	if t, ok := rawMap["text"].(string); ok {
		if nt, mod := rewriteStrings(t); mod {
			rawMap["text"] = nt
			changed = true
		}
	}
	if items, ok := rawMap["items"].([]any); ok {
		for _, it := range items {
			if m, ok := it.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					if nt, mod := rewriteStrings(t); mod {
						m["text"] = nt
						changed = true
					}
				}
			}
		}
	}
	if !changed {
		return nil, false, nil
	}
	out, err := json.Marshal(rawMap)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

func collectAttachmentIDsFromMessageMap(rawMap map[string]any) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}

	if raw, ok := rawMap["attachments"].([]any); ok {
		for _, item := range raw {
			switch v := item.(type) {
			case string:
				add(v)
			case map[string]any:
				if id, ok := v["id"].(string); ok {
					add(id)
				}
			}
		}
	}
	if t, ok := rawMap["text"].(string); ok {
		for _, id := range proxy.ExtractAttachmentIDsFromText(t) {
			add(id)
		}
	}
	if items, ok := rawMap["items"].([]any); ok {
		for _, it := range items {
			if m, ok := it.(map[string]any); ok {
				if t, ok := m["text"].(string); ok {
					for _, id := range proxy.ExtractAttachmentIDsFromText(t) {
						add(id)
					}
				}
			}
		}
	}
	return out
}

func (h *Hub) lookupAttachmentState(id, nodeID string) (localAtt proxy.Attachment, remoteAtt proxy.Attachment, alreadyOnNode bool) {
	h.indexMu.RLock()
	defer h.indexMu.RUnlock()
	entry := h.attachments[id]
	if entry == nil {
		return proxy.Attachment{}, proxy.Attachment{}, false
	}
	localAtt = entry.local
	if r, ok := entry.replicated[nodeID]; ok {
		return localAtt, r, true
	}
	return localAtt, proxy.Attachment{}, false
}

func (h *Hub) recordReplicatedAttachment(id string, localAtt proxy.Attachment, nodeID string, remoteAtt proxy.Attachment) {
	h.indexMu.Lock()
	defer h.indexMu.Unlock()
	entry := h.attachments[id]
	if entry == nil {
		entry = &localAttachmentEntry{replicated: make(map[string]proxy.Attachment)}
		h.attachments[id] = entry
	}
	if localAtt.Path != "" {
		entry.local = localAtt
	}
	entry.replicated[nodeID] = remoteAtt
}

func (h *Hub) uploadFileToRemoteNode(ctx context.Context, node NodeConfig, localAtt proxy.Attachment) (proxy.Attachment, error) {
	f, err := os.Open(localAtt.Path)
	if err != nil {
		return proxy.Attachment{}, fmt.Errorf("open hub attachment %s: %w", localAtt.ID, err)
	}
	defer f.Close()

	name := localAtt.Name
	if name == "" {
		name = filepath.Base(localAtt.Path)
	}
	targetURL, err := buildNodeURL(node.BaseURL, "/api/v1/attachments", "")
	if err != nil {
		return proxy.Attachment{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, f)
	if err != nil {
		return proxy.Attachment{}, err
	}
	h.applyNodeHeaders(req.Header, nil, node)
	req.Header.Set("X-File-Name", url.PathEscape(name))
	if localAtt.Mime != "" {
		req.Header.Set("Content-Type", localAtt.Mime)
	} else {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	if st, err := f.Stat(); err == nil {
		req.ContentLength = st.Size()
	}

	resp, err := h.streamClient.Do(req)
	if err != nil {
		return proxy.Attachment{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return proxy.Attachment{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return proxy.Attachment{}, fmt.Errorf("remote attachment upload HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var uploaded proxy.Attachment
	if err := json.Unmarshal(respBody, &uploaded); err != nil {
		return proxy.Attachment{}, fmt.Errorf("decode remote attachment response: %w", err)
	}
	return uploaded, nil
}

// =========================================================================
// Phase 2C: Cross-Node File & Artifact Reading
// =========================================================================

func (h *Hub) handleCrossNodeFile(w http.ResponseWriter, r *http.Request, local http.Handler) {
	q := r.URL.Query()
	queryModified := false

	targetNodeID := firstNonEmpty(q.Get("node"), q.Get(nodeURIQueryParam))
	if targetNodeID != "" {
		q.Del("node")
		q.Del(nodeURIQueryParam)
		queryModified = true
	}

	rawURI := strings.TrimSpace(q.Get("uri"))
	if rawURI != "" {
		if clean, nid, ok := StripNodeHintFromURI(rawURI); ok {
			if targetNodeID == "" {
				targetNodeID = nid
			}
			rawURI = clean
			q.Set("uri", clean)
			queryModified = true
		}
	}

	// Resolve by explicit cascade_id / cascadeId query param
	if targetNodeID == "" {
		if cid := extractCascadeIDFromQuery(q); cid != "" {
			if nid, ok := h.lookupCascadeNode(cid); ok {
				targetNodeID = nid
			}
		}
	}

	// Resolve by /brain/<cascadeId>/ or /static/artifacts/<cascadeId>/ pattern
	if targetNodeID == "" {
		for _, candidate := range []string{rawURI, r.URL.Path} {
			if m := brainCascadeRe.FindStringSubmatch(candidate); len(m) >= 2 {
				if nid, ok := h.lookupCascadeNode(m[1]); ok {
					targetNodeID = nid
					break
				}
			}
		}
	}

	// Resolve by unique workspace prefix match
	if targetNodeID == "" && rawURI != "" {
		if nid, ok := h.lookupUniqueWorkspaceNode(rawURI); ok {
			targetNodeID = nid
		}
	}

	var overrideQuery *string
	if queryModified {
		enc := q.Encode()
		overrideQuery = &enc
		r.URL.RawQuery = enc
	}

	// Known remote node -> forward directly!
	if targetNodeID != "" && !strings.EqualFold(targetNodeID, LocalNodeID) {
		if node, ok := h.store.GetNode(targetNodeID); ok && node.Enabled {
			h.forwardHTTPToNode(w, r, node, "", overrideQuery, nil, nil)
			return
		}
	}

	// Known local node -> serve locally.
	if strings.EqualFold(targetNodeID, LocalNodeID) && !h.opts.Headless {
		local.ServeHTTP(w, r)
		return
	}

	// Ambiguous file path: if non-headless, check local first; if missing locally (or headless),
	// probe online remote nodes concurrently and serve the first 200/206 response!
	if !h.opts.Headless {
		if r.URL.Path == "/api/v1/files/raw" {
			// Avoid buffering large raw files when present locally: check path existence directly.
			if localPath, err := proxy.ResolveLocalFilePath(rawURI, q.Get("cascade_id")); err == nil {
				if st, sErr := os.Stat(localPath); sErr == nil && st.Mode().IsRegular() {
					local.ServeHTTP(w, r)
					return
				}
			}
		} else {
			rec := httptest.NewRecorder()
			local.ServeHTTP(rec, cloneLocalRequest(r, nil))
			if rec.Code == http.StatusOK || rec.Code == http.StatusPartialContent {
				copyResponseHeaders(w.Header(), rec.Header(), true)
				w.Header().Set("Content-Length", strconv.Itoa(rec.Body.Len()))
				w.WriteHeader(rec.Code)
				_, _ = w.Write(rec.Body.Bytes())
				return
			}
		}
	}

	// Fallback parallel probe across online remote nodes
	remoteNodes := h.filteredRemoteNodes()
	if len(remoteNodes) == 0 {
		local.ServeHTTP(w, r)
		return
	}

	probeCtx, cancel := context.WithTimeout(r.Context(), h.listTimeout)
	defer cancel()

	hitCh := make(chan NodeConfig, len(remoteNodes))
	var wg sync.WaitGroup
	for _, n := range remoteNodes {
		wg.Add(1)
		go func(node NodeConfig) {
			defer wg.Done()
			probeMethod := http.MethodGet
			if r.URL.Path == "/api/v1/files/raw" {
				probeMethod = http.MethodHead
			}
			code, _, _, err := h.doNodeRequest(probeCtx, h.listClient, node, probeMethod, r.URL.Path, r.URL.RawQuery, nil, "")
			if err == nil && (code == http.StatusOK || code == http.StatusPartialContent) {
				select {
				case hitCh <- node:
					cancel()
				default:
				}
			}
		}(n)
	}
	go func() {
		wg.Wait()
		close(hitCh)
	}()

	if hitNode, ok := <-hitCh; ok {
		h.forwardHTTPToNode(w, r, hitNode, "", overrideQuery, nil, nil)
		return
	}

	if !h.opts.Headless {
		local.ServeHTTP(w, r)
		return
	}
	writeJSONError(w, http.StatusNotFound, "file not found on any online node")
}

// =========================================================================
// Phase 2D: Cross-Node Search, Background Tasks, & Permission Broadcast
// =========================================================================

func (h *Hub) handleAggregatedSearch(w http.ResponseWriter, r *http.Request, local http.Handler) {
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read search body")
		return
	}
	var req struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	_ = json.Unmarshal(bodyBytes, &req)

	multiNode := h.isMultiNode()
	remoteNodes := h.filteredRemoteNodes()
	includeLocal := h.includeLocalInAggregation()

	var merged []proxy.ConversationSearchResult
	seen := make(map[string]bool)
	var mu sync.Mutex
	var wg sync.WaitGroup

	appendResults := func(nodeID, nodeName string, raw []byte) {
		var parsed proxy.ConversationSearchResponse
		if json.Unmarshal(raw, &parsed) != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, item := range parsed.Results {
			if item.CascadeID == "" || seen[item.CascadeID] {
				continue
			}
			seen[item.CascadeID] = true
			h.indexCascade(item.CascadeID, nodeID)
			if item.ParentConversationID != "" {
				h.indexCascade(item.ParentConversationID, nodeID)
			}
			if multiNode {
				item.WorkspaceName = prefixWorkspaceWithNode(nodeName, item.WorkspaceName)
			}
			merged = append(merged, item)
		}
	}

	if includeLocal {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			local.ServeHTTP(rec, cloneLocalRequest(r, bodyBytes))
			if rec.Code == http.StatusOK {
				appendResults(LocalNodeID, h.localNodeDisplayName(), rec.Body.Bytes())
			}
		}()
	}

	for _, n := range remoteNodes {
		wg.Add(1)
		go func(node NodeConfig) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(r.Context(), h.listTimeout)
			defer cancel()
			code, _, respBody, reqErr := h.doNodeRequest(
				ctx, h.listClient, node, http.MethodPost,
				"/api/exa.language_server_pb.LanguageServerService/SearchConversations",
				"", bodyBytes, "application/json",
			)
			if reqErr == nil && code == http.StatusOK {
				appendResults(node.ID, node.Name, respBody)
			}
		}(n)
	}
	wg.Wait()

	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].LastModifiedTime > merged[j].LastModifiedTime
	})
	if req.Limit > 0 && len(merged) > req.Limit {
		merged = merged[:req.Limit]
	}
	if merged == nil {
		merged = []proxy.ConversationSearchResult{}
	}

	respBytes, _ := json.Marshal(proxy.ConversationSearchResponse{Results: merged})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Connect-Protocol-Version", "1")
	w.Header().Set("Content-Length", strconv.Itoa(len(respBytes)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respBytes)
}

func (h *Hub) handleAggregatedBackgroundTasks(w http.ResponseWriter, r *http.Request, local http.Handler) {
	multiNode := h.isMultiNode()
	remoteNodes := h.filteredRemoteNodes()
	includeLocal := h.includeLocalInAggregation()

	var merged []proxy.GlobalBackgroundTaskItem
	var mu sync.Mutex
	var wg sync.WaitGroup

	appendTasks := func(nodeID, nodeName string, raw []byte) {
		var env struct {
			Tasks []proxy.GlobalBackgroundTaskItem `json:"tasks"`
		}
		if json.Unmarshal(raw, &env) != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		for _, t := range env.Tasks {
			if t.ConversationID != "" {
				h.indexCascade(t.ConversationID, nodeID)
			}
			if t.TaskID != "" {
				h.indexTask(t.TaskID, nodeID)
			}
			if t.LogURI != "" {
				h.indexTask(t.LogURI, nodeID)
			}
			if multiNode {
				t.WorkspaceName = prefixWorkspaceWithNode(nodeName, t.WorkspaceName)
			}
			merged = append(merged, t)
		}
	}

	if includeLocal {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			local.ServeHTTP(rec, cloneLocalRequest(r, nil))
			if rec.Code == http.StatusOK {
				appendTasks(LocalNodeID, h.localNodeDisplayName(), rec.Body.Bytes())
			}
		}()
	}

	for _, n := range remoteNodes {
		wg.Add(1)
		go func(node NodeConfig) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(r.Context(), h.listTimeout)
			defer cancel()
			code, _, respBody, reqErr := h.doNodeRequest(ctx, h.listClient, node, http.MethodGet, "/gateway/tasks/background", "", nil, "")
			if reqErr == nil && code == http.StatusOK {
				appendTasks(node.ID, node.Name, respBody)
			}
		}(n)
	}
	wg.Wait()

	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].StartTime != merged[j].StartTime {
			return merged[i].StartTime > merged[j].StartTime
		}
		return merged[i].ConversationID < merged[j].ConversationID
	})
	if merged == nil {
		merged = []proxy.GlobalBackgroundTaskItem{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": merged})
}

func (h *Hub) handleBackgroundTaskLogRoute(w http.ResponseWriter, r *http.Request, local http.Handler) {
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 16*1024))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	r.ContentLength = int64(len(bodyBytes))

	var req struct {
		ConversationID string `json:"conversationId"`
		TaskID         string `json:"taskId"`
		LogURI         string `json:"logUri"`
	}
	_ = json.Unmarshal(bodyBytes, &req)

	var targetNodeID string
	if req.ConversationID != "" {
		if nid, ok := h.lookupCascadeNode(req.ConversationID); ok {
			targetNodeID = nid
		}
	}
	if targetNodeID == "" && req.TaskID != "" {
		if nid, ok := h.lookupTaskNode(req.TaskID); ok {
			targetNodeID = nid
		}
	}
	if targetNodeID == "" && req.LogURI != "" {
		if nid, ok := h.lookupTaskNode(req.LogURI); ok {
			targetNodeID = nid
		}
	}

	if targetNodeID != "" && targetNodeID != LocalNodeID {
		if node, ok := h.store.GetNode(targetNodeID); ok && node.Enabled {
			h.forwardHTTPToNode(w, r, node, "", nil, bodyBytes, nil)
			return
		}
	}
	local.ServeHTTP(w, r)
}

func (h *Hub) handleHubPermissions(w http.ResponseWriter, r *http.Request, local http.Handler) {
	if r.Method == http.MethodGet {
		if remote, useRemote := h.pickDefaultTarget(); useRemote {
			h.forwardHTTPToNode(w, r, remote, "", nil, nil, nil)
			return
		}
		local.ServeHTTP(w, r)
		return
	}
	if r.Method != http.MethodPost {
		local.ServeHTTP(w, r)
		return
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read permissions body")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	r.ContentLength = int64(len(bodyBytes))

	// Broadcast permission update to all enabled remote nodes concurrently.
	remoteNodes := h.store.ListNodes()
	var firstRemoteBody []byte
	var firstRemoteStatus int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, n := range remoteNodes {
		if !n.Enabled {
			continue
		}
		wg.Add(1)
		go func(node NodeConfig) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(r.Context(), h.listTimeout)
			defer cancel()
			code, _, respBody, reqErr := h.doNodeRequest(ctx, h.listClient, node, http.MethodPost, "/gateway/permissions", "", bodyBytes, "application/json")
			if reqErr != nil {
				slog.Warn("[Hub] Failed to sync permissions to node", "node", node.ID, "err", reqErr)
				return
			}
			mu.Lock()
			if firstRemoteStatus == 0 && code >= 200 && code < 300 {
				firstRemoteStatus = code
				firstRemoteBody = respBody
			}
			mu.Unlock()
		}(n)
	}

	if !h.opts.Headless {
		rec := httptest.NewRecorder()
		local.ServeHTTP(rec, cloneLocalRequest(r, bodyBytes))
		wg.Wait()
		copyResponseHeaders(w.Header(), rec.Header(), true)
		w.Header().Set("Content-Length", strconv.Itoa(rec.Body.Len()))
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
		return
	}

	wg.Wait()
	if firstRemoteStatus > 0 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(firstRemoteBody)))
		w.WriteHeader(firstRemoteStatus)
		_, _ = w.Write(firstRemoteBody)
		return
	}
	writeJSONError(w, http.StatusBadGateway, "no online nodes accepted permission update")
}

func (h *Hub) handleHubAIAssistPrefs(w http.ResponseWriter, r *http.Request, local http.Handler) {
	if r.Method == http.MethodGet {
		if remote, useRemote := h.pickDefaultTarget(); useRemote {
			h.forwardHTTPToNode(w, r, remote, "", nil, nil, nil)
			return
		}
		local.ServeHTTP(w, r)
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodPatch && r.Method != http.MethodPut {
		local.ServeHTTP(w, r)
		return
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 32*1024))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read ai-assist prefs body")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	r.ContentLength = int64(len(bodyBytes))

	// Broadcast AI Assist preference changes to all enabled remote nodes concurrently,
	// preserving Accept-Language so each node resolves auto-locale cleanly.
	remoteNodes := h.store.ListNodes()
	var firstRemoteBody []byte
	var firstRemoteStatus int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, n := range remoteNodes {
		if !n.Enabled {
			continue
		}
		wg.Add(1)
		go func(node NodeConfig) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(r.Context(), h.listTimeout)
			defer cancel()
			targetURL, uErr := buildNodeURL(node.BaseURL, "/gateway/ai-assist/prefs", r.URL.RawQuery)
			if uErr != nil {
				return
			}
			req, reqErr := http.NewRequestWithContext(ctx, r.Method, targetURL, bytes.NewReader(bodyBytes))
			if reqErr != nil {
				return
			}
			h.applyNodeHeaders(req.Header, r.Header, node)
			req.Header.Set("Content-Type", "application/json")
			resp, doErr := h.listClient.Do(req)
			if doErr != nil {
				slog.Warn("[Hub] Failed to sync AI Assist prefs to node", "node", node.ID, "err", doErr)
				return
			}
			defer resp.Body.Close()
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
			mu.Lock()
			if firstRemoteStatus == 0 && resp.StatusCode >= 200 && resp.StatusCode < 300 {
				firstRemoteStatus = resp.StatusCode
				firstRemoteBody = respBody
			}
			mu.Unlock()
		}(n)
	}

	if !h.opts.Headless {
		rec := httptest.NewRecorder()
		local.ServeHTTP(rec, cloneLocalRequest(r, bodyBytes))
		wg.Wait()
		copyResponseHeaders(w.Header(), rec.Header(), true)
		w.Header().Set("Content-Length", strconv.Itoa(rec.Body.Len()))
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
		return
	}

	wg.Wait()
	if firstRemoteStatus > 0 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(firstRemoteBody)))
		w.WriteHeader(firstRemoteStatus)
		_, _ = w.Write(firstRemoteBody)
		return
	}
	writeJSONError(w, http.StatusBadGateway, "no online nodes accepted AI Assist preference update")
}
