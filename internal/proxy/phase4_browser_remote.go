package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"antigravity-mobile/internal/i18n"
)

// Phase 4: Sibling ConnectRPC services exposed by Antigravity 2.33-2.35 language_server:
//   - exa.browser_viewer_pb.BrowserViewerService (live browser tab viewing & touch takeover)
//   - exa.remoting.RemotingService (SSH remote host management & interactive prompts)
//   - LanguageServerService/ListEnvironments (project execution environments)

const (
	svcBrowserViewer = "exa.browser_viewer_pb.BrowserViewerService"
	svcRemoting      = "exa.remoting.RemotingService"
	svcConversation  = "product.v1.ConversationService"
)

// callServiceRPC performs a unary ConnectRPC JSON call against any service hosted on language_server.
// Method names are passed without a leading slash so contract tests only track LanguageServerService RPCs.
func (p *Proxy) callServiceRPC(ctx context.Context, client *http.Client, service, method string, payload interface{}) ([]byte, int, error) {
	port, token := p.ActiveUpstream()
	if port <= 0 {
		return nil, http.StatusServiceUnavailable, fmt.Errorf("no active Antigravity upstream")
	}
	if client == nil {
		client = p.upstreamClient()
	}

	var bodyBytes []byte
	switch v := payload.(type) {
	case nil:
		bodyBytes = []byte("{}")
	case []byte:
		bodyBytes = v
	case json.RawMessage:
		bodyBytes = v
	default:
		var err error
		bodyBytes, err = json.Marshal(v)
		if err != nil {
			return nil, http.StatusBadRequest, err
		}
	}

	endpoint := fmt.Sprintf("https://127.0.0.1:%d/%s/%s", port, strings.Trim(service, "/"), strings.TrimLeft(method, "/"))
	doOnce := func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Connect-Protocol-Version", "1")
		if token != "" {
			req.Header.Set("x-codeium-csrf-token", token)
			req.Header.Set("x-antigravity-csrf-token", token)
		}
		return client.Do(req)
	}

	resp, err := doOnce()
	if err != nil && isTransientPooledConnErr(err) && ctx.Err() == nil {
		if p.transport != nil {
			p.transport.CloseIdleConnections()
		}
		resp, err = doOnce()
	}
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return nil, http.StatusBadGateway, err
	}
	return respBody, resp.StatusCode, nil
}

func extractConnectErrorMessage(body []byte, fallback string) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return fallback
	}
	var ce struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(body, &ce) == nil {
		if msg := strings.TrimSpace(ce.Message); msg != "" {
			return msg
		}
		if msg := strings.TrimSpace(ce.Error); msg != "" {
			return msg
		}
		if ce.Code != "" {
			return ce.Code
		}
	}
	if len(trimmed) > 300 {
		return trimmed[:300]
	}
	return trimmed
}

// ===========================================================================
// 1. BrowserViewerService — Remote Live Browser Frame & Touch Takeover
// ===========================================================================

func normalizeBrowserControlAction(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if strings.HasPrefix(s, "BROWSER_VIEWER_CONTROL_ACTION_") {
		return s
	}
	switch s {
	case "NAVIGATE", "GOTO", "OPEN":
		return "BROWSER_VIEWER_CONTROL_ACTION_NAVIGATE"
	case "BACK":
		return "BROWSER_VIEWER_CONTROL_ACTION_BACK"
	case "FORWARD":
		return "BROWSER_VIEWER_CONTROL_ACTION_FORWARD"
	case "RELOAD", "REFRESH":
		return "BROWSER_VIEWER_CONTROL_ACTION_RELOAD"
	case "STOP":
		return "BROWSER_VIEWER_CONTROL_ACTION_STOP"
	case "INPUT_EVENT", "INPUT", "TAP", "CLICK", "KEY", "SCROLL", "PASTE":
		return "BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT"
	default:
		return "BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT"
	}
}

func normalizeBrowserInputKind(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if strings.HasPrefix(s, "KIND_") {
		return s
	}
	switch s {
	case "POINTER_DOWN", "DOWN", "MOUSEDOWN":
		return "KIND_POINTER_DOWN"
	case "POINTER_UP", "UP", "MOUSEUP":
		return "KIND_POINTER_UP"
	case "POINTER_MOVE", "MOVE", "MOUSEMOVE":
		return "KIND_POINTER_MOVE"
	case "WHEEL", "SCROLL":
		return "KIND_WHEEL"
	case "KEY_DOWN", "KEYDOWN":
		return "KIND_KEY_DOWN"
	case "KEY_UP", "KEYUP":
		return "KIND_KEY_UP"
	case "PASTE", "TEXT":
		return "KIND_PASTE"
	default:
		return s
	}
}

func normalizeBrowserInputEvent(raw map[string]interface{}) map[string]interface{} {
	if raw == nil {
		return nil
	}
	out := make(map[string]interface{}, len(raw))
	for k, v := range raw {
		out[k] = v
	}
	if kind, ok := out["kind"].(string); ok && strings.TrimSpace(kind) != "" {
		out["kind"] = normalizeBrowserInputKind(kind)
	}
	return out
}

// HandleBrowserViewer handles /gateway/browser/{tabs,open,frame,takeover,control,release,renew,close}.
func (p *Proxy) HandleBrowserViewer(w http.ResponseWriter, r *http.Request) {
	uiLang := resolveRequestUILang(r)
	switch r.URL.Path {
	case "/gateway/browser/tabs":
		rootID := strings.TrimSpace(r.URL.Query().Get("cascadeId"))
		if rootID == "" {
			rootID = strings.TrimSpace(r.URL.Query().Get("rootConversationId"))
		}
		if r.Method == http.MethodPost && r.Body != nil {
			var req struct {
				CascadeID          string `json:"cascadeId"`
				RootConversationID string `json:"rootConversationId"`
			}
			_ = json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req)
			rootID = firstNonEmpty(rootID, strings.TrimSpace(req.RootConversationID), strings.TrimSpace(req.CascadeID))
		}
		payload := map[string]interface{}{}
		if rootID != "" {
			payload["rootConversationId"] = rootID
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		body, status, err := p.callServiceRPC(ctx, p.upstreamClient(), svcBrowserViewer, "ListBrowserTabs", payload)
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "获取浏览器标签失败", "Failed to list browser tabs", "ブラウザタブの取得に失敗しました")))
			return
		}
		var up map[string]interface{}
		if json.Unmarshal(body, &up) != nil {
			up = map[string]interface{}{}
		}
		up["ok"] = true
		if _, has := up["tabs"]; !has {
			up["tabs"] = []interface{}{}
		}
		writeJSONStatus(w, http.StatusOK, up)

	case "/gateway/browser/open":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			CascadeID          string `json:"cascadeId"`
			RootConversationID string `json:"rootConversationId"`
			TabID              string `json:"tabId"`
			OfferSDP           string `json:"offerSdp"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		payload := map[string]interface{}{}
		if rootID := firstNonEmpty(strings.TrimSpace(req.RootConversationID), strings.TrimSpace(req.CascadeID)); rootID != "" {
			payload["rootConversationId"] = rootID
		}
		if tid := strings.TrimSpace(req.TabID); tid != "" {
			payload["tabId"] = tid
		}
		if sdp := strings.TrimSpace(req.OfferSDP); sdp != "" {
			payload["offerSdp"] = sdp
		}
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		body, status, err := p.callServiceRPC(ctx, p.upstreamClient(), svcBrowserViewer, "OpenBrowserView", payload)
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "打开浏览器视图失败", "Failed to open browser view", "ブラウザビューを開けませんでした")))
			return
		}
		var up map[string]interface{}
		if json.Unmarshal(body, &up) != nil {
			up = map[string]interface{}{}
		}
		up["ok"] = true
		writeJSONStatus(w, http.StatusOK, up)

	case "/gateway/browser/frame":
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			CascadeID          string `json:"cascadeId"`
			RootConversationID string `json:"rootConversationId"`
			SessionID          string `json:"sessionId"`
			TabID              string `json:"tabId"`
			MaxWidthPx         int    `json:"maxWidthPx"`
			MaxHeightPx        int    `json:"maxHeightPx"`
		}
		if r.Method == http.MethodPost {
			_ = json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req)
		} else {
			q := r.URL.Query()
			req.CascadeID = q.Get("cascadeId")
			req.RootConversationID = q.Get("rootConversationId")
			req.SessionID = q.Get("sessionId")
			req.TabID = q.Get("tabId")
		}
		maxW := req.MaxWidthPx
		if maxW <= 0 {
			maxW = 960
		} else if maxW > 1920 {
			maxW = 1920
		}
		maxH := req.MaxHeightPx
		if maxH <= 0 {
			maxH = 1440
		} else if maxH > 1920 {
			maxH = 1920
		}
		payload := map[string]interface{}{
			"maxWidthPx":  maxW,
			"maxHeightPx": maxH,
		}
		if rootID := firstNonEmpty(strings.TrimSpace(req.RootConversationID), strings.TrimSpace(req.CascadeID)); rootID != "" {
			payload["rootConversationId"] = rootID
		}
		if sid := strings.TrimSpace(req.SessionID); sid != "" {
			payload["sessionId"] = sid
		}
		if tid := strings.TrimSpace(req.TabID); tid != "" {
			payload["tabId"] = tid
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		body, status, err := p.callServiceRPC(ctx, p.upstreamClient(), svcBrowserViewer, "ReadBrowserFrame", payload)
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "读取浏览器画面失败", "Failed to read browser frame", "ブラウザフレームの取得に失敗しました")))
			return
		}
		var up map[string]interface{}
		if json.Unmarshal(body, &up) != nil {
			up = map[string]interface{}{}
		}
		up["ok"] = true
		writeJSONStatus(w, http.StatusOK, up)

	case "/gateway/browser/takeover":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			CascadeID          string `json:"cascadeId"`
			RootConversationID string `json:"rootConversationId"`
			SessionID          string `json:"sessionId"`
			TabID              string `json:"tabId"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		payload := map[string]interface{}{}
		if rootID := firstNonEmpty(strings.TrimSpace(req.RootConversationID), strings.TrimSpace(req.CascadeID)); rootID != "" {
			payload["rootConversationId"] = rootID
		}
		if sid := strings.TrimSpace(req.SessionID); sid != "" {
			payload["sessionId"] = sid
		}
		if tid := strings.TrimSpace(req.TabID); tid != "" {
			payload["tabId"] = tid
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		body, status, err := p.callServiceRPC(ctx, p.upstreamClient(), svcBrowserViewer, "TakeBrowserControl", payload)
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "接管浏览器控制权失败", "Failed to take browser control", "ブラウザ操作権の取得に失敗しました")))
			return
		}
		var up map[string]interface{}
		if json.Unmarshal(body, &up) != nil {
			up = map[string]interface{}{}
		}
		up["ok"] = true
		writeJSONStatus(w, http.StatusOK, up)

	case "/gateway/browser/control":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			CascadeID          string                   `json:"cascadeId"`
			RootConversationID string                   `json:"rootConversationId"`
			SessionID          string                   `json:"sessionId"`
			TabID              string                   `json:"tabId"`
			ControlToken       string                   `json:"controlToken"`
			Action             string                   `json:"action"`
			URL                string                   `json:"url"`
			InputEvent         map[string]interface{}   `json:"inputEvent"`
			InputEvents        []map[string]interface{} `json:"inputEvents"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		rootID := firstNonEmpty(strings.TrimSpace(req.RootConversationID), strings.TrimSpace(req.CascadeID))

		// Support batched input events (e.g. pointer_down + pointer_up for a tap) in one mobile round-trip.
		if len(req.InputEvents) > 0 {
			if len(req.InputEvents) > 32 {
				writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "单次最多发送 32 个输入事件", "At most 32 input events per request", "1回のリクエストで送信できる入力イベントは最大32件です"))
				return
			}
			var lastUp map[string]interface{}
			for _, ev := range req.InputEvents {
				payload := map[string]interface{}{
					"action":     "BROWSER_VIEWER_CONTROL_ACTION_INPUT_EVENT",
					"inputEvent": normalizeBrowserInputEvent(ev),
				}
				if rootID != "" {
					payload["rootConversationId"] = rootID
				}
				if sid := strings.TrimSpace(req.SessionID); sid != "" {
					payload["sessionId"] = sid
				}
				if tid := strings.TrimSpace(req.TabID); tid != "" {
					payload["tabId"] = tid
				}
				if tok := strings.TrimSpace(req.ControlToken); tok != "" {
					payload["controlToken"] = tok
				}
				ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
				body, status, err := p.callServiceRPC(ctx, p.upstreamClient(), svcBrowserViewer, "ControlBrowserTab", payload)
				cancel()
				if err != nil {
					writeRPCError(w, status, err.Error())
					return
				}
				if status != http.StatusOK {
					writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "浏览器控制操作失败", "Browser control action failed", "ブラウザ操作に失敗しました")))
					return
				}
				_ = json.Unmarshal(body, &lastUp)
			}
			if lastUp == nil {
				lastUp = map[string]interface{}{}
			}
			lastUp["ok"] = true
			writeJSONStatus(w, http.StatusOK, lastUp)
			return
		}

		payload := map[string]interface{}{
			"action": normalizeBrowserControlAction(req.Action),
		}
		if rootID != "" {
			payload["rootConversationId"] = rootID
		}
		if sid := strings.TrimSpace(req.SessionID); sid != "" {
			payload["sessionId"] = sid
		}
		if tid := strings.TrimSpace(req.TabID); tid != "" {
			payload["tabId"] = tid
		}
		if tok := strings.TrimSpace(req.ControlToken); tok != "" {
			payload["controlToken"] = tok
		}
		if u := strings.TrimSpace(req.URL); u != "" {
			payload["url"] = u
		}
		if req.InputEvent != nil {
			payload["inputEvent"] = normalizeBrowserInputEvent(req.InputEvent)
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		body, status, err := p.callServiceRPC(ctx, p.upstreamClient(), svcBrowserViewer, "ControlBrowserTab", payload)
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "浏览器控制操作失败", "Browser control action failed", "ブラウザ操作に失敗しました")))
			return
		}
		var up map[string]interface{}
		if json.Unmarshal(body, &up) != nil {
			up = map[string]interface{}{}
		}
		up["ok"] = true
		writeJSONStatus(w, http.StatusOK, up)

	case "/gateway/browser/release":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			CascadeID          string `json:"cascadeId"`
			RootConversationID string `json:"rootConversationId"`
			SessionID          string `json:"sessionId"`
			TabID              string `json:"tabId"`
			ControlToken       string `json:"controlToken"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req)
		payload := map[string]interface{}{}
		if rootID := firstNonEmpty(strings.TrimSpace(req.RootConversationID), strings.TrimSpace(req.CascadeID)); rootID != "" {
			payload["rootConversationId"] = rootID
		}
		if sid := strings.TrimSpace(req.SessionID); sid != "" {
			payload["sessionId"] = sid
		}
		if tid := strings.TrimSpace(req.TabID); tid != "" {
			payload["tabId"] = tid
		}
		if tok := strings.TrimSpace(req.ControlToken); tok != "" {
			payload["controlToken"] = tok
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		body, status, err := p.callServiceRPC(ctx, p.upstreamClient(), svcBrowserViewer, "ReleaseBrowserControl", payload)
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "释放控制权失败", "Failed to release browser control", "ブラウザ操作権の解放に失敗しました")))
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})

	case "/gateway/browser/renew":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			SessionID string `json:"sessionId"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req)
		if strings.TrimSpace(req.SessionID) == "" {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "缺少 sessionId", "Missing sessionId", "sessionId が指定されていません"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		body, status, err := p.callServiceRPC(ctx, p.upstreamClient(), svcBrowserViewer, "RenewBrowserView", map[string]interface{}{
			"sessionId": strings.TrimSpace(req.SessionID),
		})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "续期浏览器会话失败", "Failed to renew browser view", "ブラウザセッションの更新に失敗しました")))
			return
		}
		var up map[string]interface{}
		if json.Unmarshal(body, &up) != nil {
			up = map[string]interface{}{}
		}
		up["ok"] = true
		writeJSONStatus(w, http.StatusOK, up)

	case "/gateway/browser/close":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			SessionID string `json:"sessionId"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req)
		if strings.TrimSpace(req.SessionID) == "" {
			writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
		defer cancel()
		_, _, _ = p.callServiceRPC(ctx, p.upstreamClient(), svcBrowserViewer, "CloseBrowserView", map[string]interface{}{
			"sessionId": strings.TrimSpace(req.SessionID),
		})
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})

	default:
		http.NotFound(w, r)
	}
}

// ===========================================================================
// 2. RemotingService & Environments — SSH Remote Hosts & Interactive Prompts
// ===========================================================================

type RemoteConnectPromptState struct {
	Message  string `json:"message"`
	IsSecret bool   `json:"isSecret,omitempty"`
}

type RemoteConnectJobSnapshot struct {
	JobID     string                    `json:"jobId"`
	Name      string                    `json:"name,omitempty"`
	RemoteID  string                    `json:"remoteId,omitempty"`
	Status    string                    `json:"status"` // "connecting" | "prompt" | "connected" | "error"
	Message   string                    `json:"message,omitempty"`
	Error     string                    `json:"error,omitempty"`
	Prompt    *RemoteConnectPromptState `json:"prompt,omitempty"`
	Done      bool                      `json:"done"`
	UpdatedAt string                    `json:"updatedAt,omitempty"`
}

type remoteConnectJob struct {
	mu       sync.Mutex
	snapshot RemoteConnectJobSnapshot
	msgKind  string // "connecting" | "auth_submitted" | ""
	errKind  string // "ssh_failed" | ""
	cancel   context.CancelFunc
	created  time.Time
}

func (j *remoteConnectJob) getSnapshot() RemoteConnectJobSnapshot {
	return j.getSnapshotForLang(i18n.GetLang())
}

func (j *remoteConnectJob) getSnapshotForLang(lang string) RemoteConnectJobSnapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	cp := j.snapshot
	switch j.msgKind {
	case "connecting":
		cp.Message = i18n.T3For(lang, "正在建立 SSH 连接...", "Establishing SSH connection...", "SSH 接続を確立中...")
	case "auth_submitted":
		cp.Message = i18n.T3For(lang, "已提交认证，正在继续连接...", "Authentication submitted, continuing connection...", "認証を送信しました。接続を続行中...")
	}
	if j.errKind == "ssh_failed" {
		cp.Error = i18n.T3For(lang, "SSH 连接失败", "SSH connection failed", "SSH 接続に失敗しました")
	}
	if j.snapshot.Prompt != nil {
		p := *j.snapshot.Prompt
		cp.Prompt = &p
	}
	return cp
}

func newRandomJobID(prefix string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return prefix + "-" + hex.EncodeToString(b[:])
}

func (p *Proxy) cancelAllRemoteJobs() {
	p.remoteJobsMu.Lock()
	jobs := make([]*remoteConnectJob, 0, len(p.remoteJobs))
	for _, j := range p.remoteJobs {
		jobs = append(jobs, j)
	}
	p.remoteJobs = nil
	p.remoteJobsMu.Unlock()
	for _, j := range jobs {
		j.mu.Lock()
		if j.cancel != nil {
			j.cancel()
		}
		j.mu.Unlock()
	}
}

func (p *Proxy) cleanupStaleRemoteJobsLocked() {
	now := time.Now()
	for id, j := range p.remoteJobs {
		j.mu.Lock()
		done := j.snapshot.Done
		age := now.Sub(j.created)
		j.mu.Unlock()
		if (done && age > 10*time.Minute) || age > 30*time.Minute {
			delete(p.remoteJobs, id)
		}
	}
}

// buildRemoteMultiHopFromReq normalizes either raw remoteMultiHop or a simple {host, user, port} descriptor.
func buildRemoteMultiHopFromReq(rawMultiHop interface{}, name, host, user string, port int) interface{} {
	if rawMultiHop != nil {
		return rawMultiHop
	}
	targetHost := firstNonEmpty(strings.TrimSpace(host), strings.TrimSpace(name))
	if targetHost == "" {
		return nil
	}
	hop := map[string]interface{}{
		"host": targetHost,
	}
	if u := strings.TrimSpace(user); u != "" {
		hop["user"] = u
	}
	if port > 0 && port <= 65535 {
		hop["port"] = port
	}
	return map[string]interface{}{
		"hops": []interface{}{hop},
	}
}

func (p *Proxy) runRemoteConnectStream(ctx context.Context, job *remoteConnectJob, multiHop interface{}) {
	defer func() {
		job.mu.Lock()
		if !job.snapshot.Done {
			job.snapshot.Done = true
			if job.snapshot.Status == "connecting" || job.snapshot.Status == "prompt" {
				if job.snapshot.Error != "" {
					job.snapshot.Status = "error"
				} else {
					job.snapshot.Status = "connected"
				}
			}
			job.snapshot.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		}
		job.mu.Unlock()
	}()

	port, token := p.ActiveUpstream()
	if port <= 0 {
		job.mu.Lock()
		job.snapshot.Status = "error"
		job.snapshot.Error = "no active Antigravity upstream"
		job.snapshot.Done = true
		job.snapshot.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		job.mu.Unlock()
		return
	}

	reqPayload := map[string]interface{}{
		"remoteMultiHop": multiHop,
	}
	var envBuf bytes.Buffer
	var header [5]byte
	envBuf.Write(header[:])
	if err := json.NewEncoder(&envBuf).Encode(reqPayload); err != nil {
		job.mu.Lock()
		job.snapshot.Status = "error"
		job.snapshot.Error = err.Error()
		job.snapshot.Done = true
		job.mu.Unlock()
		return
	}
	binary.BigEndian.PutUint32(envBuf.Bytes()[1:5], uint32(envBuf.Len()-5))

	endpoint := fmt.Sprintf("https://127.0.0.1:%d/%s/ConnectRemote", port, svcRemoting)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(envBuf.Bytes()))
	if err != nil {
		job.mu.Lock()
		job.snapshot.Status = "error"
		job.snapshot.Error = err.Error()
		job.snapshot.Done = true
		job.mu.Unlock()
		return
	}
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
		req.Header.Set("x-antigravity-csrf-token", token)
	}

	client := p.streamingHTTPClient()
	if client == nil {
		client = p.upstreamClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		job.mu.Lock()
		job.snapshot.Status = "error"
		job.snapshot.Error = err.Error()
		job.snapshot.Done = true
		job.snapshot.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		job.mu.Unlock()
		return
	}
	defer resp.Body.Close()

	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		job.mu.Lock()
		job.snapshot.Status = "error"
		job.snapshot.Error = extractConnectErrorMessage(raw, fmt.Sprintf("HTTP %d", resp.StatusCode))
		job.snapshot.Done = true
		job.snapshot.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		job.mu.Unlock()
		return
	}

	applyFrame := func(body []byte) {
		var frame map[string]interface{}
		if json.Unmarshal(body, &frame) != nil {
			return
		}
		job.mu.Lock()
		defer job.mu.Unlock()
		if rid, ok := frame["remoteId"].(string); ok && strings.TrimSpace(rid) != "" {
			job.snapshot.RemoteID = strings.TrimSpace(rid)
		}
		if upd, ok := frame["update"].(string); ok && strings.TrimSpace(upd) != "" {
			job.msgKind = ""
			job.snapshot.Message = strings.TrimSpace(upd)
		} else if updMap, ok := frame["update"].(map[string]interface{}); ok {
			if msg, ok := updMap["message"].(string); ok && msg != "" {
				job.msgKind = ""
				job.snapshot.Message = msg
			}
		}
		if pr, ok := frame["prompt"].(map[string]interface{}); ok && len(pr) > 0 {
			msg, _ := pr["message"].(string)
			sec, _ := pr["isSecret"].(bool)
			if strings.TrimSpace(msg) != "" {
				job.snapshot.Status = "prompt"
				job.snapshot.Prompt = &RemoteConnectPromptState{
					Message:  strings.TrimSpace(msg),
					IsSecret: sec,
				}
			}
		}
		if st, ok := frame["status"].(string); ok && st != "" {
			upper := strings.ToUpper(st)
			switch {
			case strings.Contains(upper, "ERROR") || strings.Contains(upper, "FAIL"):
				job.snapshot.Status = "error"
				if job.snapshot.Error == "" {
					job.snapshot.Error = firstNonEmpty(job.snapshot.Message, st)
				}
				job.snapshot.Done = true
			case strings.Contains(upper, "CONNECTED") || strings.Contains(upper, "READY") || strings.Contains(upper, "SUCCESS"):
				job.snapshot.Status = "connected"
				job.snapshot.Prompt = nil
				job.snapshot.Done = true
			}
		}
		job.snapshot.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	if !strings.Contains(ct, "application/connect") {
		// Graceful fallback for unit tests or unary JSON responses
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		applyFrame(raw)
		return
	}

	for {
		flag, body, err := readConnectEnvelope(resp.Body)
		if err != nil {
			if err != io.EOF && ctx.Err() == nil {
				job.mu.Lock()
				if !job.snapshot.Done && job.snapshot.Status != "connected" {
					job.snapshot.Status = "error"
					job.snapshot.Error = err.Error()
					job.snapshot.Done = true
				}
				job.mu.Unlock()
			}
			return
		}
		if flag != 0 {
			// End-of-stream trailer frame
			var trailer struct {
				Error *struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal(body, &trailer) == nil && trailer.Error != nil {
				job.mu.Lock()
				job.snapshot.Status = "error"
				if msg := firstNonEmpty(trailer.Error.Message, trailer.Error.Code); msg != "" {
					job.errKind = ""
					job.snapshot.Error = msg
				} else {
					job.errKind = "ssh_failed"
					job.snapshot.Error = "SSH connection failed"
				}
				job.snapshot.Prompt = nil
				job.snapshot.Done = true
				job.snapshot.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
				job.mu.Unlock()
			}
			return
		}
		applyFrame(body)
	}
}

// HandleRemotes serves /gateway/remotes* and /gateway/environments.
func (p *Proxy) HandleRemotes(w http.ResponseWriter, r *http.Request) {
	uiLang := resolveRequestUILang(r)
	switch r.URL.Path {
	case "/gateway/remotes":
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		var (
			remBody, connBody, envBody []byte
			remStatus, connStatus, envStatus int
			wg sync.WaitGroup
		)
		wg.Add(3)
		go func() {
			defer wg.Done()
			remBody, remStatus, _ = p.callServiceRPC(ctx, p.upstreamClient(), svcRemoting, "ListRemotes", map[string]interface{}{
				"includeAutodetected": true,
				"includeSaved":        true,
			})
		}()
		go func() {
			defer wg.Done()
			connBody, connStatus, _ = p.callServiceRPC(ctx, p.upstreamClient(), svcRemoting, "ListConnectedRemotes", map[string]interface{}{})
		}()
		go func() {
			defer wg.Done()
			envBody, envStatus, _ = p.callUpstreamRPC(ctx, "ListEnvironments", map[string]interface{}{})
		}()
		wg.Wait()

		remotes := make([]interface{}, 0)
		if remStatus == http.StatusOK {
			var parsed struct {
				Remotes []interface{} `json:"remotes"`
			}
			if json.Unmarshal(remBody, &parsed) == nil && parsed.Remotes != nil {
				remotes = parsed.Remotes
			}
		}
		connected := make([]interface{}, 0)
		if connStatus == http.StatusOK {
			var parsed struct {
				Remotes []interface{} `json:"remotes"`
			}
			if json.Unmarshal(connBody, &parsed) == nil && parsed.Remotes != nil {
				connected = parsed.Remotes
			}
		}
		environments := make([]interface{}, 0)
		if envStatus == http.StatusOK {
			var parsed struct {
				Environments []interface{} `json:"environments"`
			}
			if json.Unmarshal(envBody, &parsed) == nil && parsed.Environments != nil {
				environments = parsed.Environments
			}
		}

		p.remoteJobsMu.Lock()
		p.cleanupStaleRemoteJobsLocked()
		jobs := make([]RemoteConnectJobSnapshot, 0, len(p.remoteJobs))
		for _, j := range p.remoteJobs {
			jobs = append(jobs, j.getSnapshotForLang(uiLang))
		}
		p.remoteJobsMu.Unlock()

		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":           true,
			"remotes":      remotes,
			"connected":    connected,
			"environments": environments,
			"jobs":         jobs,
		})

	case "/gateway/remotes/connect":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Name           string      `json:"name"`
			Host           string      `json:"host"`
			User           string      `json:"user"`
			Port           int         `json:"port"`
			RemoteMultiHop interface{} `json:"remoteMultiHop"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 128*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		multiHop := buildRemoteMultiHopFromReq(req.RemoteMultiHop, req.Name, req.Host, req.User, req.Port)
		if multiHop == nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "缺少远端主机配置", "Missing remote host configuration", "リモートホスト設定が不足しています"))
			return
		}
		jobID := newRandomJobID("ssh")
		jobCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		job := &remoteConnectJob{
			cancel:  cancel,
			created: time.Now(),
			msgKind: "connecting",
			snapshot: RemoteConnectJobSnapshot{
				JobID:     jobID,
				Name:      firstNonEmpty(strings.TrimSpace(req.Name), strings.TrimSpace(req.Host)),
				Status:    "connecting",
				Message:   i18n.T3For(uiLang, "正在建立 SSH 连接...", "Establishing SSH connection...", "SSH 接続を確立中..."),
				UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			},
		}
		p.remoteJobsMu.Lock()
		if p.remoteJobs == nil {
			p.remoteJobs = make(map[string]*remoteConnectJob)
		}
		p.cleanupStaleRemoteJobsLocked()
		p.remoteJobs[jobID] = job
		p.remoteJobsMu.Unlock()

		go p.runRemoteConnectStream(jobCtx, job, multiHop)

		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":  true,
			"job": job.getSnapshotForLang(uiLang),
		})

	case "/gateway/remotes/connect/status":
		jobID := strings.TrimSpace(r.URL.Query().Get("jobId"))
		if jobID == "" {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "缺少 jobId", "Missing jobId", "jobId が指定されていません"))
			return
		}
		p.remoteJobsMu.Lock()
		job := p.remoteJobs[jobID]
		p.remoteJobsMu.Unlock()
		if job == nil {
			writeRPCError(w, http.StatusNotFound, i18n.T3For(uiLang, "连接任务不存在或已过期", "Connection job not found or expired", "接続ジョブが存在しないか期限切れです"))
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":  true,
			"job": job.getSnapshotForLang(uiLang),
		})

	case "/gateway/remotes/prompt":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			JobID    string `json:"jobId"`
			RemoteID string `json:"remoteId"`
			Response string `json:"response"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		remoteID := strings.TrimSpace(req.RemoteID)
		if remoteID == "" && strings.TrimSpace(req.JobID) != "" {
			p.remoteJobsMu.Lock()
			if job := p.remoteJobs[strings.TrimSpace(req.JobID)]; job != nil {
				remoteID = job.getSnapshotForLang(uiLang).RemoteID
			}
			p.remoteJobsMu.Unlock()
		}
		if remoteID == "" {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "缺少 remoteId", "Missing remoteId", "remoteId が指定されていません"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		body, status, err := p.callServiceRPC(ctx, p.upstreamClient(), svcRemoting, "RespondToPrompt", map[string]interface{}{
			"remoteId": remoteID,
			"response": req.Response,
		})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "提交认证响应失败", "Failed to submit authentication response", "認証応答の送信に失敗しました")))
			return
		}
		if strings.TrimSpace(req.JobID) != "" {
			p.remoteJobsMu.Lock()
			if job := p.remoteJobs[strings.TrimSpace(req.JobID)]; job != nil {
				job.mu.Lock()
				job.snapshot.Prompt = nil
				if job.snapshot.Status == "prompt" {
					job.snapshot.Status = "connecting"
					job.msgKind = "auth_submitted"
					job.snapshot.Message = i18n.T3For(uiLang, "已提交认证，正在继续连接...", "Authentication submitted, continuing connection...", "認証を送信しました。接続を続行中...")
				}
				job.snapshot.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
				job.mu.Unlock()
			}
			p.remoteJobsMu.Unlock()
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})

	case "/gateway/remotes/disconnect":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			RemoteID string `json:"remoteId"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		if strings.TrimSpace(req.RemoteID) == "" {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "缺少 remoteId", "Missing remoteId", "remoteId が指定されていません"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		body, status, err := p.callServiceRPC(ctx, p.upstreamClient(), svcRemoting, "DisconnectRemote", map[string]interface{}{
			"remoteId": strings.TrimSpace(req.RemoteID),
		})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "断开远端连接失败", "Failed to disconnect remote host", "リモート接続の切断に失敗しました")))
			return
		}
		p.invalidateProjectsCache()
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})

	case "/gateway/remotes/save":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Name           string      `json:"name"`
			Description    string      `json:"description"`
			Type           string      `json:"type"`
			Host           string      `json:"host"`
			User           string      `json:"user"`
			Port           int         `json:"port"`
			RemoteMultiHop interface{} `json:"remoteMultiHop"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 128*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		multiHop := buildRemoteMultiHopFromReq(req.RemoteMultiHop, req.Name, req.Host, req.User, req.Port)
		if multiHop == nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "缺少主机名或地址", "Missing host name or address", "ホスト名またはアドレスが指定されていません"))
			return
		}
		desc := map[string]interface{}{
			"name":           firstNonEmpty(strings.TrimSpace(req.Name), strings.TrimSpace(req.Host)),
			"description":    strings.TrimSpace(req.Description),
			"remoteMultiHop": multiHop,
		}
		if t := strings.TrimSpace(req.Type); t != "" {
			desc["type"] = t
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		body, status, err := p.callServiceRPC(ctx, p.upstreamClient(), svcRemoting, "SaveRemote", map[string]interface{}{
			"descriptor": desc,
		})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "保存远端主机失败", "Failed to save remote host", "リモートホストの保存に失敗しました")))
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})

	case "/gateway/remotes/delete":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			RemoteID string `json:"remoteId"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 32*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "无效的请求体", "Invalid request body", "リクエストボディが無効です"))
			return
		}
		if strings.TrimSpace(req.RemoteID) == "" {
			writeRPCError(w, http.StatusBadRequest, i18n.T3For(uiLang, "缺少 remoteId", "Missing remoteId", "remoteId が指定されていません"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		body, status, err := p.callServiceRPC(ctx, p.upstreamClient(), svcRemoting, "DeleteRemote", map[string]interface{}{
			"remoteId": strings.TrimSpace(req.RemoteID),
		})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "删除远端主机失败", "Failed to delete remote host", "リモートホストの削除に失敗しました")))
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})

	case "/gateway/environments":
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		body, status, err := p.callUpstreamRPC(ctx, "ListEnvironments", map[string]interface{}{})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, extractConnectErrorMessage(body, i18n.T3For(uiLang, "获取环境列表失败", "Failed to list environments", "環境一覧の取得に失敗しました")))
			return
		}
		var up map[string]interface{}
		if json.Unmarshal(body, &up) != nil {
			up = map[string]interface{}{}
		}
		up["ok"] = true
		if _, has := up["environments"]; !has {
			up["environments"] = []interface{}{}
		}
		writeJSONStatus(w, http.StatusOK, up)

	default:
		http.NotFound(w, r)
	}
}
