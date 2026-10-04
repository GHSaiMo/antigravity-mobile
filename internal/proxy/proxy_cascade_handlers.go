package proxy

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (p *Proxy) checkAndRecordMessageDedup(key string, ttl time.Duration) bool {
	p.msgDedupMu.Lock()
	defer p.msgDedupMu.Unlock()
	if p.msgDedup == nil {
		p.msgDedup = make(map[string]time.Time)
	}
	now := time.Now()
	if lastTime, exists := p.msgDedup[key]; exists {
		if now.Sub(lastTime) < ttl {
			return true // is duplicate within TTL
		}
		// Lazy expiry: remove stale key immediately
		delete(p.msgDedup, key)
	}
	// Periodic cleanup of stale entries if map expands
	if len(p.msgDedup) > 128 {
		for k, t := range p.msgDedup {
			if now.Sub(t) > 120*time.Second {
				delete(p.msgDedup, k)
			}
		}
	}
	p.msgDedup[key] = now
	return false
}

func (p *Proxy) getCascadeDedup(key string, ttl time.Duration) string {
	p.cascadeDedupMu.Lock()
	defer p.cascadeDedupMu.Unlock()
	if p.cascadeDedup == nil {
		p.cascadeDedup = make(map[string]cascadeDedupEntry)
		return ""
	}
	now := time.Now()
	if entry, exists := p.cascadeDedup[key]; exists {
		if now.Sub(entry.createdAt) < ttl {
			return entry.cascadeID
		}
		// Lazy expiry
		delete(p.cascadeDedup, key)
	}
	if len(p.cascadeDedup) > 64 {
		for k, v := range p.cascadeDedup {
			if now.Sub(v.createdAt) > 120*time.Second {
				delete(p.cascadeDedup, k)
			}
		}
	}
	return ""
}

func (p *Proxy) setCascadeDedup(key string, cascadeID string) {
	p.cascadeDedupMu.Lock()
	defer p.cascadeDedupMu.Unlock()
	if p.cascadeDedup == nil {
		p.cascadeDedup = make(map[string]cascadeDedupEntry)
	}
	p.cascadeDedup[key] = cascadeDedupEntry{
		cascadeID: cascadeID,
		createdAt: time.Now(),
	}
}

func (p *Proxy) handleSendUserCascadeMessage(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string, port int, token string) {
	clientMsgID := strings.TrimSpace(r.Header.Get("X-Client-Message-Id"))
	if clientMsgID == "" {
		clientMsgID = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	if clientMsgID != "" {
		dedupKey := "client_msg:" + clientMsgID
		if p.checkAndRecordMessageDedup(dedupKey, 60*time.Second) {
			slog.Info(fmt.Sprintf("[Proxy] Deduplicated repeat SendUserCascadeMessage via clientMsgID: %s", clientMsgID))
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", "2")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{}"))
			return
		}
	}

	// P6: fast-reject oversized requests using Content-Length before reading any bytes.
	const maxBodySize = 50 * 1024 * 1024
	if cl := r.ContentLength; cl > maxBodySize {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}

	// Protect against OOM for extremely large requests by limiting to 50MB
	// PERF: for large known payloads (>256KB), allocate directly instead of using the pool
	// to avoid growing the pooled buffer past its retention threshold.
	var bodyBytes []byte
	var cleanup func()
	var err error
	if r.ContentLength > 256*1024 {
		data, readErr := io.ReadAll(io.LimitReader(r.Body, maxBodySize+1))
		if readErr != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		if int64(len(data)) > maxBodySize {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		bodyBytes = data
		cleanup = func() {} // no pool to return to
	} else {
		bodyBytes, cleanup, err = readBodyToPool(r.Body, maxBodySize)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
	}
	defer cleanup()

	var rawMap map[string]interface{}
	cascadeID := ""
	if err := json.Unmarshal(bodyBytes, &rawMap); err == nil {
		cascadeID, _ = rawMap["cascadeId"].(string)
		if strings.TrimSpace(cascadeID) == "" {
			http.Error(w, `{"error":"invalid_argument: cascadeId is required"}`, http.StatusBadRequest)
			return
		}

		// Short-window idempotency check: prevent duplicate triggers within 15 seconds
		// PERF: use streaming hasher to avoid full string copy for sha256
		contentHasher := sha256.New()
		var contentLen int
		if items, ok := rawMap["items"].([]interface{}); ok {
			for _, it := range items {
				if itemMap, ok := it.(map[string]interface{}); ok {
					if t, ok := itemMap["text"].(string); ok {
						contentHasher.Write([]byte(t))
						contentLen += len(t)
					}
				}
			}
		}
		// If items is missing or empty, synthesize from "text" parameter
		if items, ok := rawMap["items"].([]interface{}); !ok || len(items) == 0 {
			if txt, ok := rawMap["text"].(string); ok && strings.TrimSpace(txt) != "" {
				rawMap["items"] = []interface{}{map[string]interface{}{"text": txt}}
				contentHasher.Write([]byte(txt))
				contentLen += len(txt)
			}
		}
		if comments, ok := rawMap["artifactComments"].([]interface{}); ok {
			for _, ac := range comments {
				if acMap, ok := ac.(map[string]interface{}); ok {
					if uri, ok := acMap["artifactUri"].(string); ok {
						contentHasher.Write([]byte(uri))
						contentLen += len(uri)
					}
				}
			}
		}
		if images, ok := rawMap["images"].([]interface{}); ok {
			for _, img := range images {
				if imgMap, ok := img.(map[string]interface{}); ok {
					if b64, ok := imgMap["base64Data"].(string); ok && len(b64) > 0 {
						prefix := b64
						if len(prefix) > 1024 {
							prefix = prefix[:1024]
						}
						h := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", len(b64), prefix)))
						imgTag := fmt.Sprintf(":img:%x", h[:8])
						contentHasher.Write([]byte(imgTag))
						contentLen += len(imgTag)
					}
				}
			}
		}

		if clientMsgID == "" && cascadeID != "" && contentLen > 0 {
			strategyKey := fmt.Sprintf("%v", rawMap["deliveryStrategy"])
			dedupKey := fmt.Sprintf("%s:%s:%x", cascadeID, strategyKey, contentHasher.Sum(nil))
			if p.checkAndRecordMessageDedup(dedupKey, 15*time.Second) {
				slog.Info(fmt.Sprintf("[Proxy] Deduplicated repeat SendUserCascadeMessage for cascade %s (textLen=%d, hashDedup)", shortCascadeID(cascadeID), contentLen))
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Content-Length", "2")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("{}"))
				return
			}
		}

		targetModel := strings.TrimSpace(r.Header.Get("X-Antigravity-Model"))
		if targetModel == "" {
			targetModel = strings.TrimSpace(r.Header.Get("X-Model"))
		}
		if targetModel == "" {
			if m, ok := rawMap["model"].(string); ok {
				targetModel = strings.TrimSpace(m)
			}
		}

		// PERF-1: detect upfront whether the body actually needs modification.
		// If no model override and no cascadeConfigRaw cleanup needed, skip
		// unmarshal→modify→remarshal which is expensive for large payloads.
		_, hasModel := rawMap["model"]
		_, hasCfgRaw := rawMap["cascadeConfigRaw"]
		_, hasText := rawMap["text"]
		needsModification := targetModel != "" || hasModel || hasCfgRaw || hasText

		if !needsModification {
			// No changes required — forward original bytes as-is.
			if cascadeID != "" {
				ClearTrajectoryCache(cascadeID)
				ClearPendingMessagesCache(cascadeID)
			}
		} else {
			delete(rawMap, "model")

			var configToUse json.RawMessage
			// 1. Check if cascadeConfig already exists in payload
			if cfg, exists := rawMap["cascadeConfig"]; exists && cfg != nil {
				if cfgBytes, err := json.Marshal(cfg); err == nil && len(cfgBytes) > 2 && string(cfgBytes) != "{}" && string(cfgBytes) != "null" {
					configToUse = cfgBytes
				}
			}

			// 2. Check if cascadeConfigRaw was provided
			if len(configToUse) == 0 {
				if rawStr, ok := rawMap["cascadeConfigRaw"].(string); ok && len(rawStr) > 0 {
					configToUse = json.RawMessage(rawStr)
				}
			}

			// 3. Fallback to trajectory metadata or last known cascade config
			if len(configToUse) == 0 {
				configToUse = p.GetCascadeConfig(cascadeID, port, token)
			}

			modelEnum := resolveModelEnum(targetModel)
			canonicalName := canonicalModelName(targetModel)
			if canonicalName == "" {
				canonicalName = targetModel
			}

			if len(configToUse) > 0 {
				var cfgObj interface{}
				if err := json.Unmarshal(configToUse, &cfgObj); err == nil {
					if modelEnum != "" {
						cfgObj = applyModelToCascadeConfig(cfgObj, modelEnum, canonicalName)
						if updatedBytes, err := json.Marshal(cfgObj); err == nil {
							configToUse = updatedBytes
						}
						if verboseRPC {
							slog.Info(fmt.Sprintf("[Proxy] SendUserCascadeMessage: applied model %s (%s) to cascade %s", targetModel, modelEnum, shortCascadeID(cascadeID)))
						}
					}
					rawMap["cascadeConfig"] = cfgObj
				}
				SetLastKnownCascadeConfig(configToUse)
				if canonicalName != "" {
					SetCascadeModel(cascadeID, canonicalName, configToUse)
				}
			} else if modelEnum != "" {
				cfgObj := applyModelToCascadeConfig(nil, modelEnum, canonicalName)
				rawMap["cascadeConfig"] = cfgObj
				if updatedBytes, err := json.Marshal(cfgObj); err == nil {
					SetLastKnownCascadeConfig(updatedBytes)
					SetCascadeModel(cascadeID, canonicalName, updatedBytes)
				}
				if verboseRPC {
					slog.Info(fmt.Sprintf("[Proxy] SendUserCascadeMessage: synthesized cascadeConfig with model %s (%s) for cascade %s", targetModel, modelEnum, shortCascadeID(cascadeID)))
				}
			}

			delete(rawMap, "cascadeConfigRaw")

			if modifiedBytes, err := json.Marshal(rawMap); err == nil {
				bodyBytes = modifiedBytes
			}

			if cascadeID != "" {
				ClearTrajectoryCache(cascadeID)
				ClearPendingMessagesCache(cascadeID)
			}
		}
	}

	if verboseRPC {
		slog.Info("[Proxy] SendUserCascadeMessage", "cascadeId", shortCascadeID(cascadeID), "payloadLen", len(bodyBytes))
	} else {
		slog.Info(fmt.Sprintf("[Proxy] 💬 发送消息 -> 会话 %s", shortCascadeID(cascadeID)))
	}

	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}
	r.ContentLength = int64(len(bodyBytes))
	r.Header.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
	r.URL.Path = reqPath

	rw := newBufferedResponseWriter()
	defer rw.release()
	rp.ServeHTTP(rw, r)

	// Copy headers from upstream
	for k, vv := range rw.header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}

	respBody := rw.body.Bytes()
	if rw.statusCode >= 200 && rw.statusCode < 300 {
		// Ensure ConnectRPC empty responses always return valid JSON "{}"
		// to prevent any client JSONDecoder from crashing on 0-byte data
		if isSpaceOnly(respBody) {
			respBody = []byte("{}")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(respBody)))
		w.WriteHeader(rw.statusCode)
		w.Write(respBody)
		if verboseRPC {
			slog.Info("[Proxy] SendUserCascadeMessage upstream success", "status", rw.statusCode, "bodyLen", len(respBody))
		}
	} else {
		w.WriteHeader(rw.statusCode)
		w.Write(respBody)
		slog.Warn("⚠️  [Proxy] SendUserCascadeMessage upstream error", "status", rw.statusCode, "body", string(respBody))
	}
}

func (p *Proxy) handleJetboxWriteState(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string) {
	// Protect against OOM for excessively large state requests by limiting to 5MB
	bodyBytes, cleanup, err := readBodyToPool(r.Body, 5*1024*1024)
	if err != nil {
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}
	defer cleanup()
	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	var stateReq struct {
		AppState struct {
			LastSelectedAgentModel string `json:"lastSelectedAgentModel"`
		} `json:"appState"`
	}
	if err := json.Unmarshal(bodyBytes, &stateReq); err == nil {
		model := stateReq.AppState.LastSelectedAgentModel
		if model != "" {
			modelEnum := resolveModelEnum(model)
			canonicalName := canonicalModelName(model)
			if canonicalName == "" {
				canonicalName = model
			}
			cascadeID := r.Header.Get("X-Cascade-Id")
			if cascadeID == "" {
				cascadeID = r.URL.Query().Get("cascade_id")
			}
			lastCfg := p.GetCascadeConfig(cascadeID, 0, "")
			patched := applyModelToCascadeConfig(lastCfg, modelEnum, canonicalName)
			if patchedBytes, err := json.Marshal(patched); err == nil {
				SetLastKnownCascadeConfig(patchedBytes)
				if cascadeID != "" {
					SetCascadeModel(cascadeID, canonicalName, patchedBytes)
					ClearTrajectoryCache(cascadeID)
				}
			}
			if verboseRPC {
				slog.Info(fmt.Sprintf("[Proxy] JetboxWriteState: cached active model %s (%s)", canonicalName, modelEnum), "cascadeId", shortCascadeID(cascadeID))
			}
		}
	}

	fwdReq := r.Clone(r.Context())
	fwdReq.URL.Path = reqPath
	fwdReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	rp.ServeHTTP(w, fwdReq)
}

func (p *Proxy) handleArtifactProxy(w http.ResponseWriter, r *http.Request) {
	p.mu.RLock()
	rp := p.activeProxy
	p.mu.RUnlock()

	if rp == nil {
		if fileRes, err := GetFileContent(r.URL.Path, ""); err == nil {
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fileRes.Content))
			return
		}
		http.Error(w, "Antigravity instance unavailable", http.StatusServiceUnavailable)
		return
	}

	rp.ServeHTTP(w, r)
}

func (p *Proxy) handleStartCascadeProxy(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string) {
	bodyBytes, cleanup, err := readBodyToPool(r.Body, 5*1024*1024)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer cleanup()

	var rawMap map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &rawMap); err == nil {
		if src, ok := rawMap["source"].(string); !ok || src == "" || src == "CORTEX_TRAJECTORY_SOURCE_UNSPECIFIED" {
			rawMap["source"] = "CORTEX_TRAJECTORY_SOURCE_INTERACTIVE_CASCADE"
		}
		if modifiedBytes, err := json.Marshal(rawMap); err == nil {
			bodyBytes = modifiedBytes
		}
	}

	r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	r.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}
	r.ContentLength = int64(len(bodyBytes))
	r.Header.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
	r.URL.Path = reqPath

	rp.ServeHTTP(w, r)
}

func (p *Proxy) handleDeleteCascadeTrajectory(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string) {
	bodyBytes, cleanup, err := readBodyToPool(r.Body, 5*1024*1024)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer cleanup()

	var reqData struct {
		CascadeID string `json:"cascadeId"`
	}
	_ = json.Unmarshal(bodyBytes, &reqData)

	// SEC-7: Strict allowlist — cascade IDs are UUID-like alphanumeric strings.
	// Rejects any ID containing path separators, null bytes, or shell metacharacters.
	isSafeCascadeID := cascadeIDRe.MatchString(reqData.CascadeID)

	// S3 fix: only set the in-memory tombstone pre-emptively (so stream/list filters
	// hide the cascade immediately). File deletion is deferred to the success branch
	// below — doing it here would permanently destroy local files if upstream rejects.
	if isSafeCascadeID {
		RecordDeletedCascade(reqData.CascadeID)
	}

	fwdReq := r.Clone(r.Context())
	fwdReq.URL.Path = reqPath
	fwdReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	rec := newBufferedResponseWriter()
	defer rec.release()
	rp.ServeHTTP(rec, fwdReq)

	if rec.statusCode >= 200 && rec.statusCode < 300 {
		if isSafeCascadeID {
			RecordDeletedCascade(reqData.CascadeID)
			ClearTrajectoryCache(reqData.CascadeID)
			// Clean up local files only after upstream confirms deletion.
			if home, err := os.UserHomeDir(); err == nil {
				annPath := filepath.Join(home, ".gemini", "antigravity", "annotations", reqData.CascadeID+".pbtxt")
				_ = os.Remove(annPath)

				convDir := filepath.Join(home, ".gemini", "antigravity", "conversations")
				_ = os.Remove(filepath.Join(convDir, reqData.CascadeID+".db"))
				_ = os.Remove(filepath.Join(convDir, reqData.CascadeID+".db-wal"))
				_ = os.Remove(filepath.Join(convDir, reqData.CascadeID+".db-shm"))

				brainDir := filepath.Join(home, ".gemini", "antigravity", "brain", reqData.CascadeID)
				_ = os.RemoveAll(brainDir)
			}
			slog.Info(fmt.Sprintf("[Proxy] 🗑️ 删除会话: %s", shortCascadeID(reqData.CascadeID)))
		}
	} else if rec.statusCode >= 400 {
		// Upstream explicitly rejected deletion; release the tombstone so the cascade reappears.
		if isSafeCascadeID {
			RemoveDeletedCascadeTombstone(reqData.CascadeID)
		}
	}

	for k, v := range rec.header {
		w.Header()[k] = v
	}
	w.WriteHeader(rec.statusCode)
	w.Write(rec.body.Bytes())
}

func (p *Proxy) handleDeleteAgentMessage(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string) {
	bodyBytes, cleanup, err := readBodyToPool(r.Body, 5*1024*1024)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer cleanup()

	var reqData struct {
		MessageID string `json:"messageId"`
		Recipient string `json:"recipient"`
	}
	_ = json.Unmarshal(bodyBytes, &reqData)

	cascadeID := reqData.Recipient
	messageID := reqData.MessageID

	if messageID != "" {
		RecordDeletedMessage(cascadeID, messageID)
		RemovePendingMessageFromCache(cascadeID, messageID)
	}
	ClearPendingMessagesCache(cascadeID)

	fwdReq := r.Clone(r.Context())
	fwdReq.URL.Path = reqPath
	fwdReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	rec := newBufferedResponseWriter()
	defer rec.release()
	rp.ServeHTTP(rec, fwdReq)

	if rec.statusCode >= 200 && rec.statusCode < 300 {
		if messageID != "" {
			RemovePendingMessageFromCache(cascadeID, messageID)
		}
		ClearPendingMessagesCache(cascadeID)
	} else if rec.statusCode >= 400 {
		if messageID != "" {
			RemoveDeletedMessageTombstone(cascadeID, messageID)
		}
	}

	for k, v := range rec.header {
		w.Header()[k] = v
	}
	w.WriteHeader(rec.statusCode)
	w.Write(rec.body.Bytes())
}

func (p *Proxy) handleUpdateConversationAnnotations(w http.ResponseWriter, r *http.Request, rp http.Handler, reqPath string) {
	bodyBytes, cleanup, err := readBodyToPool(r.Body, 5*1024*1024)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer cleanup()

	var payload struct {
		CascadeIDs  []string `json:"cascadeIds"`
		Annotations struct {
			Title string `json:"title"`
		} `json:"annotations"`
	}
	if err := json.Unmarshal(bodyBytes, &payload); err == nil {
		var activeIDs []string
		for _, cid := range payload.CascadeIDs {
			if !IsDeletedCascade(cid) {
				activeIDs = append(activeIDs, cid)
			}
		}
		if len(activeIDs) == 0 && len(payload.CascadeIDs) > 0 {
			// All requested cascades are deleted tombstones; absorb without reviving upstream
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{}"))
			return
		}

		title := strings.TrimSpace(payload.Annotations.Title)
		if title != "" && title != "未命名会话" {
			defaultTrajCache.cascadeTitlesMu.Lock()
			for _, cid := range activeIDs {
				if cid != "" {
					defaultTrajCache.cascadeTitles[cid] = title
					writeAnnotationTitle(cid, title)
				}
			}
			defaultTrajCache.cascadeTitlesMu.Unlock()
		}
	}

	p.SuppressDesktopFocus(AntiReflectionDuration)

	fwdReq := r.Clone(r.Context())
	fwdReq.URL.Path = reqPath
	fwdReq.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	rp.ServeHTTP(w, fwdReq)
}

// QuestionResponse represents an answer to a single question in a multi-question ask_question interaction.
type QuestionResponse struct {
	QuestionIndex     int      `json:"questionIndex"`
	SelectedOptionIDs []string `json:"selectedOptionIds"`
	WriteInResponse   string   `json:"writeInResponse,omitempty"`
	Skipped           bool     `json:"skipped,omitempty"`
}

// InteractionSubmitRequest represents user decision submitted from mobile client.
type InteractionSubmitRequest struct {
	CascadeID         string             `json:"cascadeId"`
	TrajectoryID      string             `json:"trajectoryId"`
	StepIndex         int                `json:"stepIndex"`
	Type              string             `json:"type"` // "permission", "ask_question", "file_permission", "run_command"
	OptionID          string             `json:"optionId,omitempty"`
	Scope             int                `json:"scope,omitempty"`
	Allow             bool               `json:"allow"`
	WriteInResponse   string             `json:"writeInResponse,omitempty"`
	Skipped           bool               `json:"skipped"`
	Target            string             `json:"target,omitempty"`
	QuestionResponses []QuestionResponse `json:"questionResponses,omitempty"`
}

// HandleCascadeInteraction submits user choice to upstream HandleCascadeUserInteraction.
func (p *Proxy) HandleCascadeInteraction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// SEC-3: Limit request body to prevent OOM from oversized payloads.
	body, cleanup, err := readBodyToPool(r.Body, 1*1024*1024)
	if err != nil {
		writeJSONError(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer cleanup()
	var req InteractionSubmitRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	p.mu.RLock()
	port := p.activePort
	token := p.activeToken
	p.mu.RUnlock()

	if port == 0 {
		http.Error(w, `{"error":"upstream not connected"}`, http.StatusServiceUnavailable)
		return
	}

	type userInteractionPayload struct {
		TrajectoryID string                 `json:"trajectoryId"`
		StepIndex    int                    `json:"stepIndex"`
		Permission   map[string]interface{} `json:"permission,omitempty"`
		AskQuestion  map[string]interface{} `json:"askQuestion,omitempty"`
		RunCommand   map[string]interface{} `json:"runCommand,omitempty"`
	}

	type rpcRequest struct {
		CascadeID   string                 `json:"cascadeId"`
		Interaction userInteractionPayload `json:"interaction"`
	}

	interaction := userInteractionPayload{
		TrajectoryID: req.TrajectoryID,
		StepIndex:    req.StepIndex,
	}

	switch req.Type {
	case "permission", "file_permission":
		allow := req.Allow
		scope := req.Scope
		denyReason := ""
		if req.Skipped {
			allow = false
		} else if req.OptionID == "5" || !req.Allow {
			allow = false
			denyReason = req.WriteInResponse
		} else {
			allow = true
			if scope <= 0 {
				scope = 1
			}
		}
		interaction.Permission = map[string]interface{}{
			"allow":               allow,
			"scope":               scope,
			"userDenyInstruction": denyReason,
			"editedTarget":        req.Target,
		}

	case "ask_question":
		if req.Skipped {
			numQuestions := len(req.QuestionResponses)
			if numQuestions == 0 {
				numQuestions = 1
			}
			responses := make([]map[string]interface{}, numQuestions)
			for i := 0; i < numQuestions; i++ {
				responses[i] = map[string]interface{}{
					"selectedOptionIds": []string{},
					"writeInResponse":   "",
					"skipped":           true,
				}
			}
			interaction.AskQuestion = map[string]interface{}{
				"responses": responses,
				"cancelled": true,
			}
		} else if len(req.QuestionResponses) > 0 {
			responses := make([]map[string]interface{}, len(req.QuestionResponses))
			for i, qr := range req.QuestionResponses {
				opts := []string{}
				for _, oid := range qr.SelectedOptionIDs {
					if oid != "" && oid != "5" && oid != "__write_in__" {
						opts = append(opts, oid)
					}
				}
				responses[i] = map[string]interface{}{
					"selectedOptionIds": opts,
					"writeInResponse":   qr.WriteInResponse,
					"skipped":           qr.Skipped,
				}
			}
			interaction.AskQuestion = map[string]interface{}{
				"responses": responses,
				"cancelled": false,
			}
		} else {
			opts := []string{}
			if req.OptionID != "" && req.OptionID != "5" && req.OptionID != "__write_in__" {
				opts = append(opts, req.OptionID)
			}
			interaction.AskQuestion = map[string]interface{}{
				"responses": []map[string]interface{}{
					{
						"selectedOptionIds": opts,
						"writeInResponse":   req.WriteInResponse,
						"skipped":           false,
					},
				},
				"cancelled": false,
			}
		}

	case "run_command":
		interaction.RunCommand = map[string]interface{}{
			"confirm":              req.Allow && !req.Skipped,
			"submittedCommandLine": req.Target,
		}

	default:
		interaction.Permission = map[string]interface{}{
			"allow":               req.Allow && !req.Skipped,
			"scope":               req.Scope,
			"userDenyInstruction": req.WriteInResponse,
		}
	}

	rpcReq := rpcRequest{
		CascadeID:   req.CascadeID,
		Interaction: interaction,
	}

	bodyBytes, err := json.Marshal(rpcReq)
	if err != nil {
		writeJSONError(w, "failed to encode: "+err.Error(), http.StatusInternalServerError)
		return
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/HandleCascadeUserInteraction", port)
	upstreamReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		writeJSONError(w, "failed to create upstream request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		upstreamReq.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.mediumClient.Do(upstreamReq)
	if err != nil {
		writeJSONError(w, "upstream call failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		slog.Warn(fmt.Sprintf("[Proxy] HandleCascadeUserInteraction error (%d): %s", resp.StatusCode, string(respBody)))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		w.Write(respBody)
		return
	}

	// Invalidate cache immediately so next poll and stream capture the new active state
	ClearTrajectoryCache(req.CascadeID)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success":true}`))
}

// handleCascadeTaskStop terminates a specific running background step via CancelCascadeSteps.
func (p *Proxy) handleCascadeTaskStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CascadeID  string `json:"cascadeId"`
		CascadeID2 string `json:"cascade_id"`
		StepIndex  *int   `json:"stepIndex"`
		StepIndex2 *int   `json:"step_index"`
		TaskID     string `json:"taskId"`
		TaskID2    string `json:"task_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	cascadeID := strings.TrimSpace(req.CascadeID)
	if cascadeID == "" {
		cascadeID = strings.TrimSpace(req.CascadeID2)
	}
	if cascadeID == "" {
		http.Error(w, `{"error":"missing cascadeId"}`, http.StatusBadRequest)
		return
	}

	taskID := strings.TrimSpace(req.TaskID)
	if taskID == "" {
		taskID = strings.TrimSpace(req.TaskID2)
	}

	stepIndex := -1
	if req.StepIndex != nil {
		stepIndex = *req.StepIndex
	} else if req.StepIndex2 != nil {
		stepIndex = *req.StepIndex2
	}

	if stepIndex <= 0 && taskID != "" {
		if idx := strings.LastIndex(taskID, "task-"); idx != -1 {
			if num, err := strconv.Atoi(taskID[idx+5:]); err == nil && num > 0 {
				stepIndex = num
			}
		}
	}

	p.mu.RLock()
	port := p.activePort
	token := p.activeToken
	p.mu.RUnlock()

	if port == 0 {
		http.Error(w, `{"error":"upstream not connected"}`, http.StatusServiceUnavailable)
		return
	}

	if stepIndex <= 0 {
		rawResp, err := p.fetchUpstreamTrajectory(cascadeID, port, token)
		if err == nil && rawResp != nil {
			details := p.ParseTrajectoryDetails(rawResp)
			for _, t := range details.RunningTasks {
				if taskID == "" || t.ID == taskID || strings.HasSuffix(t.ID, "/"+taskID) || strings.HasSuffix(taskID, "/"+t.ID) {
					stepIndex = t.StepIndex
					break
				}
			}
			if stepIndex <= 0 && len(details.RunningTasks) == 1 {
				stepIndex = details.RunningTasks[0].StepIndex
			}
		}
	}

	if stepIndex < 0 {
		stepIndex = 0
	}

	if err := p.CancelCascadeStep(cascadeID, stepIndex, port, token); err != nil {
		slog.Warn(fmt.Sprintf("[Proxy] CancelCascadeStep failed (cascade: %s, step: %d)", cascadeID, stepIndex), "err", err)
		writeJSONError(w, "cancel step failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Immediately invalidate trajectory cache so next stream tick picks up the changed status
	ClearTrajectoryCache(cascadeID)
	ClearPendingMessagesCache(cascadeID)
	p.notifyStreamTouch(cascadeID)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"success":true}`))
}

// HandleCascadeRevertPreview handles POST /gateway/cascade/revert/preview.
func (p *Proxy) HandleCascadeRevertPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req RevertPreviewRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 65536)).Decode(&req); err != nil {
		writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	req.CascadeID = strings.TrimSpace(req.CascadeID)
	if req.CascadeID == "" {
		writeJSONError(w, "cascadeId is required", http.StatusBadRequest)
		return
	}

	port, token := p.ActiveUpstream()
	if port == 0 {
		writeJSONError(w, "No active Antigravity upstream", http.StatusServiceUnavailable)
		return
	}

	res, err := p.GetRevertPreview(req.CascadeID, req.StepIndex, req.TargetStepIndex, port, token)
	if err != nil {
		slog.Warn(fmt.Sprintf("[Proxy] Revert preview failed for cascade %s (step %d)", shortCascadeID(req.CascadeID), req.StepIndex), "err", err)
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

// HandleCascadeRevertExecute handles POST /gateway/cascade/revert/execute.
func (p *Proxy) HandleCascadeRevertExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req RevertExecuteRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 65536)).Decode(&req); err != nil {
		writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	req.CascadeID = strings.TrimSpace(req.CascadeID)
	if req.CascadeID == "" {
		writeJSONError(w, "cascadeId is required", http.StatusBadRequest)
		return
	}

	port, token := p.ActiveUpstream()
	if port == 0 {
		writeJSONError(w, "No active Antigravity upstream", http.StatusServiceUnavailable)
		return
	}

	targetIndex, err := p.ExecuteRevert(req.CascadeID, req.StepIndex, req.TargetStepIndex, req.ConversationOnly, port, token)
	if err != nil {
		slog.Warn(fmt.Sprintf("[Proxy] Revert execute failed for cascade %s (step %d)", shortCascadeID(req.CascadeID), req.StepIndex), "err", err)
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ClearTrajectoryCache(req.CascadeID)

	data, err := json.Marshal(map[string]interface{}{
		"status":          "ok",
		"cascadeId":       req.CascadeID,
		"targetStepIndex": targetIndex,
	})
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
