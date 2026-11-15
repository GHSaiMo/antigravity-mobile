package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func buildConnectFrame(payload interface{}) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	frame := make([]byte, 5+len(raw))
	frame[0] = 0
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(raw)))
	copy(frame[5:], raw)
	return frame, nil
}

// HandleTerminals handles listing, creating, sending input to, closing, and streaming interactive terminals.
func (p *Proxy) HandleTerminals(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	switch r.URL.Path {
	case "/gateway/terminals":
		body, status, err := p.callUpstreamRPC(ctx, "ListTerminals", map[string]interface{}{})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, strings.TrimSpace(string(body)))
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	case "/gateway/terminal/create":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req map[string]interface{}
		_ = json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req)
		if req == nil {
			req = map[string]interface{}{}
		}
		body, status, err := p.callUpstreamRPC(ctx, "CreateTerminal", req)
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write(body)
	case "/gateway/terminal/input":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			TerminalID     string `json:"terminalId"`
			Input          string `json:"input"`
			ConversationID string `json:"conversationId"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
			writeRPCError(w, http.StatusBadRequest, "无效的请求体")
			return
		}
		if req.TerminalID == "" {
			writeRPCError(w, http.StatusBadRequest, "缺少 terminalId")
			return
		}
		body, status, err := p.callUpstreamRPC(ctx, "SendTerminalInput", map[string]interface{}{
			"terminalId":     req.TerminalID,
			"input":          req.Input,
			"conversationId": req.ConversationID,
		})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, strings.TrimSpace(string(body)))
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})
	case "/gateway/terminal/close":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			TerminalID string `json:"terminalId"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req)
		body, status, err := p.callUpstreamRPC(ctx, "CloseTerminal", map[string]interface{}{
			"terminalId": req.TerminalID,
		})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, strings.TrimSpace(string(body)))
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

// HandleTerminalStream bridges StreamTerminalOutput Connect frames onto a WebSocket connection.
func (p *Proxy) HandleTerminalStream(w http.ResponseWriter, r *http.Request) {
	terminalID := strings.TrimSpace(r.URL.Query().Get("terminalId"))
	if terminalID == "" {
		http.Error(w, "Missing terminalId", http.StatusBadRequest)
		return
	}
	port, token := p.ActiveUpstream()
	if port <= 0 {
		http.Error(w, "No upstream", http.StatusServiceUnavailable)
		return
	}

	wsUpgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	frame, _ := buildConnectFrame(map[string]interface{}{
		"terminalId": terminalID,
	})
	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/StreamTerminalOutput", port)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, url, bytes.NewReader(frame))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.streamingHTTPClient().Do(req)
	if err != nil {
		_ = conn.WriteJSON(map[string]interface{}{"type": "error", "error": err.Error()})
		return
	}
	defer resp.Body.Close()

	for {
		flag, body, err := readConnectEnvelope(resp.Body)
		if err != nil {
			break
		}
		if flag == 2 {
			_ = conn.WriteJSON(map[string]interface{}{"type": "end", "raw": json.RawMessage(body)})
			break
		}
		_ = conn.WriteMessage(websocket.TextMessage, body)
	}
}

// HandleSendStepsToBackground sends running steps to background via SendStepsToBackground.
func (p *Proxy) HandleSendStepsToBackground(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		CascadeID   string `json:"cascadeId"`
		StepIndices []int  `json:"stepIndices"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	body, status, err := p.callUpstreamRPC(ctx, "SendStepsToBackground", map[string]interface{}{
		"cascadeId":   req.CascadeID,
		"stepIndices": req.StepIndices,
	})
	if err != nil {
		writeRPCError(w, status, err.Error())
		return
	}
	if status != http.StatusOK {
		msg := strings.TrimSpace(string(body))
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":          false,
			"unsupported": strings.Contains(strings.ToLower(msg), "deprecated") || strings.Contains(strings.ToLower(msg), "unimplemented"),
			"message":     msg,
		})
		return
	}
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// HandleSubagentMessage forwards a direct mid-run message to an active subagent via SendAgentMessage.
func (p *Proxy) HandleSubagentMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Recipient    string `json:"recipient"`
		CascadeID    string `json:"cascadeId"`
		Content      string `json:"content"`
		SenderID     string `json:"senderId"`
		DisplayTitle string `json:"displayTitle"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 256*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	recipient := firstNonEmpty(req.Recipient, req.CascadeID)
	if recipient == "" || strings.TrimSpace(req.Content) == "" {
		writeRPCError(w, http.StatusBadRequest, "缺少 recipient 或 content")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	body, status, err := p.callUpstreamRPC(ctx, "SendAgentMessage", map[string]interface{}{
		"recipient":    recipient,
		"content":      req.Content,
		"senderId":     firstNonEmpty(req.SenderID, "user"),
		"displayTitle": req.DisplayTitle,
	})
	if err != nil {
		writeRPCError(w, status, err.Error())
		return
	}
	if status != http.StatusOK {
		writeRPCError(w, status, strings.TrimSpace(string(body)))
		return
	}
	p.notifyStreamTouch(recipient)
	writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// HandleBrowserSubagent exposes live browser subagent URL, latest screenshot URI, and whitelist/skip actions.
func (p *Proxy) HandleBrowserSubagent(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/gateway/subagent/browser":
		cid := strings.TrimSpace(r.URL.Query().Get("cascadeId"))
		if cid == "" {
			cid = strings.TrimSpace(r.URL.Query().Get("conversationId"))
		}
		port, token := p.ActiveUpstream()
		if port <= 0 {
			writeRPCError(w, http.StatusServiceUnavailable, "no upstream")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var urlStr, pageTitle, screenshotURI, screenshotB64 string
		if cid != "" {
			if raw, err := p.fetchUpstreamTrajectoryWithMaxAgeContext(ctx, cid, port, token, 500*time.Millisecond); err == nil && raw != nil {
				for i := len(raw.Trajectory.Steps) - 1; i >= 0; i-- {
					s := raw.Trajectory.Steps[i]
					if s.CaptureBrowserScreenshot != nil {
						screenshotURI = firstNonEmpty(screenshotURI, s.CaptureBrowserScreenshot.ScreenshotURI)
						screenshotB64 = firstNonEmpty(screenshotB64, s.CaptureBrowserScreenshot.ImageBase64)
						urlStr = firstNonEmpty(urlStr, s.CaptureBrowserScreenshot.URL)
						pageTitle = firstNonEmpty(pageTitle, s.CaptureBrowserScreenshot.PageTitle)
						break
					}
					if args := extractStepToolArgsJSON(s); args != "" {
						var parsed struct {
							URL           string `json:"url"`
							UrlUpper      string `json:"Url"`
							ScreenshotURI string `json:"screenshotUri"`
						}
						if json.Unmarshal([]byte(args), &parsed) == nil {
							urlStr = firstNonEmpty(urlStr, parsed.URL, parsed.UrlUpper)
							screenshotURI = firstNonEmpty(screenshotURI, parsed.ScreenshotURI)
						}
					}
				}
			}
		}

		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":            true,
			"cascadeId":     cid,
			"url":           urlStr,
			"pageTitle":     pageTitle,
			"screenshotUri": screenshotURI,
			"imageBase64":   screenshotB64,
		})
	case "/gateway/subagent/browser/skip":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			CascadeID string `json:"cascadeId"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req)
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		body, status, err := p.callUpstreamRPC(ctx, "SkipBrowserSubagent", map[string]interface{}{
			"cascadeId": req.CascadeID,
		})
		if err != nil {
			writeRPCError(w, status, err.Error())
			return
		}
		if status != http.StatusOK {
			writeRPCError(w, status, strings.TrimSpace(string(body)))
			return
		}
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{"ok": true})
	case "/gateway/subagent/browser/whitelist":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			URL string `json:"url"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req)
		targetURL := strings.TrimSpace(req.URL)
		if targetURL == "" {
			writeRPCError(w, http.StatusBadRequest, "缺少 url")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		_, status, _ := p.callUpstreamRPC(ctx, "AddToBrowserWhitelist", map[string]interface{}{"url": targetURL})
		cancel()
		// Persist in local ~/.gemini/config/config.json browser allowlist as durable local fallback.
		_ = appendLocalBrowserWhitelist(targetURL)
		writeJSONStatus(w, http.StatusOK, map[string]interface{}{
			"ok":     true,
			"url":    targetURL,
			"remote": status == http.StatusOK,
		})
	default:
		http.NotFound(w, r)
	}
}

func appendLocalBrowserWhitelist(targetURL string) error {
	permissionConfigMu.Lock()
	defer permissionConfigMu.Unlock()

	cfgPath := resolveLocalGeminiConfigPath()
	_ = os.MkdirAll(filepath.Dir(cfgPath), 0o755)
	doc := make(map[string]interface{})
	if raw, err := os.ReadFile(cfgPath); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &doc)
	}
	us, _ := doc["userSettings"].(map[string]interface{})
	if us == nil {
		us = make(map[string]interface{})
	}
	existing, _ := us["browserUrlAllowlist"].([]interface{})
	for _, it := range existing {
		if s, ok := it.(string); ok && s == targetURL {
			return nil
		}
	}
	us["browserUrlAllowlist"] = append(existing, targetURL)
	doc["userSettings"] = us
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, append(encoded, '\n'), 0o600)
}

// HandleAudioDictate bridges browser/mobile PCM16 audio chunks via StreamAudioTranscription -> SendAudioChunk -> EndAudioSession.
func (p *Proxy) HandleAudioDictate(w http.ResponseWriter, r *http.Request) {
	port, token := p.ActiveUpstream()
	if port <= 0 {
		http.Error(w, "No upstream", http.StatusServiceUnavailable)
		return
	}

	wsUpgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	cascadeID := strings.TrimSpace(r.URL.Query().Get("cascadeId"))
	mimeType := firstNonEmpty(r.URL.Query().Get("mimeType"), "audio/pcm;rate=16000")
	log.Printf("[Dictate] 🎙️ 原生流式听写开始 (cascadeId=%s, mime=%s)", cascadeID, mimeType)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	frame, _ := buildConnectFrame(map[string]interface{}{
		"cascadeId":  cascadeID,
		"mimeType":   mimeType,
		"continuous": true,
	})
	streamURL := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/StreamAudioTranscription", port)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, streamURL, bytes.NewReader(frame))
	if err != nil {
		log.Printf("[Dictate] ❌ 创建上游流式请求失败: %v", err)
		return
	}
	httpReq.Header.Set("Content-Type", "application/connect+json")
	httpReq.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		httpReq.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.streamingHTTPClient().Do(httpReq)
	if err != nil {
		log.Printf("[Dictate] ❌ 请求上游 StreamAudioTranscription 失败: %v", err)
		_ = conn.WriteJSON(map[string]interface{}{"type": "error", "error": err.Error()})
		return
	}
	defer resp.Body.Close()

	sessionCh := make(chan string, 1)
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		defer cancel()
		sentSession := false
		for {
			flag, body, readErr := readConnectEnvelope(resp.Body)
			if readErr != nil {
				break
			}
			if flag == 2 {
				_ = conn.WriteJSON(map[string]interface{}{"type": "complete"})
				break
			}
			var msg map[string]interface{}
			if json.Unmarshal(body, &msg) == nil {
				if ready, ok := msg["ready"].(map[string]interface{}); ok {
					if sid, ok := ready["sessionId"].(string); ok && sid != "" && !sentSession {
						sentSession = true
						sessionCh <- sid
						log.Printf("[Dictate] ⚡ 上游听写会话建立: %s", sid)
					}
				}
				if sid, ok := msg["sessionId"].(string); ok && sid != "" && !sentSession {
					sentSession = true
					sessionCh <- sid
					log.Printf("[Dictate] ⚡ 上游听写会话建立: %s", sid)
				}
				// 关键协议归一化：上游返回格式为 {"transcription":{"text":"...","isFinal":...}}
				// 解包并在顶层注入 text, transcript, isFinal，兼容所有端（iOS、Android、Web）
				if tr, ok := msg["transcription"].(map[string]interface{}); ok {
					if txt, ok := tr["text"].(string); ok {
						msg["text"] = txt
						msg["transcript"] = txt
						msg["isFinal"] = tr["isFinal"]
						if norm, err := json.Marshal(msg); err == nil {
							body = norm
						}
						isFinal, _ := tr["isFinal"].(bool)
						log.Printf("[Dictate] 📝 听写识别: %q (isFinal=%v)", txt, isFinal)
					}
				}
			}
			_ = conn.WriteMessage(websocket.TextMessage, body)
		}
	}()

	var sessionID string
	var seq int64
	for {
		msgType, raw, readErr := conn.ReadMessage()
		if readErr != nil {
			break
		}
		if sessionID == "" {
			select {
			case sid := <-sessionCh:
				sessionID = sid
			case <-time.After(3 * time.Second):
				log.Printf("[Dictate] ⚠️ 上游听写会话建立超时")
				_ = conn.WriteJSON(map[string]interface{}{"type": "error", "error": "上游听写会话建立超时"})
				return
			}
		}

		if msgType == websocket.BinaryMessage {
			seq++
			_, _, _ = p.callUpstreamRPC(ctx, "SendAudioChunk", map[string]interface{}{
				"sessionId":      sessionID,
				"data":           base64.StdEncoding.EncodeToString(raw),
				"sequenceNumber": seq,
			})
			continue
		}

		var ctrl struct {
			Type string `json:"type"`
			Data string `json:"data"`
		}
		if json.Unmarshal(raw, &ctrl) == nil {
			switch ctrl.Type {
			case "chunk":
				seq++
				_, _, _ = p.callUpstreamRPC(ctx, "SendAudioChunk", map[string]interface{}{
					"sessionId":      sessionID,
					"data":           ctrl.Data,
					"sequenceNumber": seq,
				})
			case "end":
				log.Printf("[Dictate] 🛑 客户端结束听写 (sessionId=%s, chunks=%d)", sessionID, seq)
				endCtx, endCancel := context.WithTimeout(context.Background(), 3*time.Second)
				_, _, _ = p.callUpstreamRPC(endCtx, "EndAudioSession", map[string]interface{}{
					"sessionId":          sessionID,
					"lastSequenceNumber": seq,
				})
				endCancel()
				// 等待上游响应流接收完毕（获取最终识别结果与完成标志），最长等待 2 秒
				select {
				case <-doneCh:
				case <-time.After(2 * time.Second):
				}
				return
			}
		}
	}
	if sessionID != "" {
		log.Printf("[Dictate] 🛑 客户端连接断开，终止上游会话 (sessionId=%s, chunks=%d)", sessionID, seq)
		endCtx, endCancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, _, _ = p.callUpstreamRPC(endCtx, "EndAudioSession", map[string]interface{}{
			"sessionId":          sessionID,
			"lastSequenceNumber": seq,
		})
		endCancel()
		select {
		case <-doneCh:
		case <-time.After(1 * time.Second):
		}
	}
}

// HandleNarration synthesizes speech for an Agent bubble via StartNarration, falling back to local TTS when unavailable.
func (p *Proxy) HandleNarration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Text  string `json:"text"`
		Voice string `json:"voice"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 512*1024)).Decode(&req); err != nil {
		writeRPCError(w, http.StatusBadRequest, "无效的请求体")
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeRPCError(w, http.StatusBadRequest, "缺少朗读文本")
		return
	}
	if len([]rune(text)) > 4000 {
		runes := []rune(text)
		text = string(runes[:4000])
	}

	// 1. Try upstream Antigravity StartNarration -> SendNarrationTurn -> EndNarration.
	if audioBytes, mimeType := p.tryUpstreamNarration(r.Context(), text); len(audioBytes) > 0 {
		w.Header().Set("Content-Type", firstNonEmpty(mimeType, "audio/mpeg"))
		w.Header().Set("X-Narration-Source", "upstream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(audioBytes)
		return
	}

	// 2. Fallback to macOS `say` synthesizing AAC (.m4a) cleanly when upstream narration service is unavailable locally.
	if _, err := exec.LookPath("say"); err == nil {
		tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("mgy-narr-%d.m4a", time.Now().UnixNano()))
		defer os.Remove(tmpFile)
		sayCtx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		cmd := exec.CommandContext(sayCtx, "say", "-o", tmpFile, "--data-format=aac", text)
		runErr := cmd.Run()
		cancel()
		if runErr == nil {
			if data, readErr := os.ReadFile(tmpFile); readErr == nil && len(data) > 0 {
				w.Header().Set("Content-Type", "audio/mp4")
				w.Header().Set("X-Narration-Source", "macos-say")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(data)
				return
			}
		}
	}

	writeRPCError(w, http.StatusServiceUnavailable, "当前环境暂无可用语音播报引擎")
}

func (p *Proxy) tryUpstreamNarration(parent context.Context, text string) ([]byte, string) {
	port, token := p.ActiveUpstream()
	if port <= 0 {
		return nil, ""
	}
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()

	frame, err := buildConnectFrame(map[string]interface{}{"userText": text})
	if err != nil {
		return nil, ""
	}
	url := fmt.Sprintf("https://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/StartNarration", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(frame))
	if err != nil {
		return nil, ""
	}
	req.Header.Set("Content-Type", "application/connect+json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("x-codeium-csrf-token", token)
	}

	resp, err := p.upstreamClient().Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return nil, ""
	}
	defer resp.Body.Close()

	var audioBuf bytes.Buffer
	var mimeType string
	var sessionID string
	for {
		flag, body, err := readConnectEnvelope(resp.Body)
		if err != nil {
			break
		}
		if flag == 2 {
			break
		}
		var chunk map[string]interface{}
		if json.Unmarshal(body, &chunk) != nil {
			continue
		}
		if ready, ok := chunk["ready"].(map[string]interface{}); ok {
			if sid, ok := ready["sessionId"].(string); ok && sid != "" && sessionID == "" {
				sessionID = sid
				_, _, _ = p.callUpstreamRPC(ctx, "SendNarrationTurn", map[string]interface{}{
					"sessionId": sessionID,
					"text":      text,
				})
				_, _, _ = p.callUpstreamRPC(ctx, "EndNarration", map[string]interface{}{
					"sessionId": sessionID,
				})
			}
		}
		if audioChunk, ok := chunk["audioChunk"].(map[string]interface{}); ok {
			if mt, ok := audioChunk["mimeType"].(string); ok && mt != "" {
				mimeType = mt
			}
			if b64, ok := audioChunk["data"].(string); ok && b64 != "" {
				if raw, decErr := base64.StdEncoding.DecodeString(b64); decErr == nil {
					audioBuf.Write(raw)
				}
			}
		}
	}
	return audioBuf.Bytes(), mimeType
}
